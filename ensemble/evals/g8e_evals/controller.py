# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Finite attended evaluation controller.

The controller orchestrates existing commands over one owner-approved,
content-addressed cycle manifest. It does not recreate campaign, verifier,
projector, promoter, or public outbox semantics; the attended CLI composes
closed command adapters for those concerns.

The controller lifecycle for one cycle:

1. Load and validate one frozen ``CycleManifest``.
2. For each immutable child command, invoke the child-command handler to
   produce a report directory. An interrupted child is dead evidence; the
   controller never resumes a partial report.
3. After all children complete, require passing aggregate verification.
4. Require strict validation and exact digest approval.
5. Enqueue the accepted candidate into the durable outbox.
6. Publish from the durable outbox. Publication retries read the outbox
   and never rerun inference.

Stop conditions are explicit and fail-closed. The controller stops on
budget, disk, authority, verifier, disclosure, trust, or integrity
failure. A graceful stop completes the current child and then stops. A
safety stop halts immediately and marks the current child as interrupted.

State ownership is unambiguous: one ``ControllerState`` record identifies
the controller instance, current status, current child, completed
children, failed children, and timestamps. The state is persisted to
``controller-state.json`` after every transition.

Every model is frozen with ``extra="forbid"``. Content hashes are
SHA-256 over canonical JSON (sorted keys, no extra whitespace).
"""

from __future__ import annotations

import hashlib
import json
import os
import shutil
import time
from enum import StrEnum
from pathlib import Path, PurePosixPath
from typing import Literal, Protocol, Self

from pydantic import BaseModel, ConfigDict, Field, model_validator

from g8e_evals.candidate_digest import CandidateDigestError, compute_candidate_tree_digest
from g8e_evals.constants import (
    CONTROLLER_STATE_JSON,
    CONTROLLER_STOP_REQUEST_JSON,
    CONTROLLER_TRANSITIONS_JSONL,
    CYCLE_MANIFEST_JSON,
    OUTBOX_ENTRIES_DIR,
    OUTBOX_INDEX_JSONL,
)
from g8e_evals.replacement_rule import ReplacementManifestRule, compute_replacement_child_id


CONTROLLER_SCHEMA_VERSION = "1.0.0"

_ZERO_HASH = "0" * 64


def _sha256(data: str) -> str:
    return hashlib.sha256(data.encode()).hexdigest()


def _canonical_json(data: object) -> str:
    return json.dumps(
        data,
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )


def _now_iso() -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def _validate_relative_path(value: str) -> None:
    path = PurePosixPath(value)
    if path.is_absolute() or value != path.as_posix() or any(part in {"", ".", ".."} for part in path.parts):
        raise ValueError(f"controller path must be a safe relative path: {value!r}")


# ---------------------------------------------------------------------------
# Stop conditions
# ---------------------------------------------------------------------------


class StopReason(StrEnum):
    """The typed reason the controller stopped.

    ``COMPLETED``: All children passed and publication succeeded.
    ``GRACEFUL_STOP``: Owner requested graceful stop after the current child.
    ``SAFETY_STOP``: Owner requested immediate safety stop.
    ``BUDGET_EXHAUSTED``: A budget ceiling was exceeded.
    ``DISK_FULL``: The disk ceiling was exceeded.
    ``AUTHORITY_FAILURE``: An authority mismatch or invalid authority was detected.
    ``VERIFIER_FAILURE``: Aggregate verification failed.
    ``DISCLOSURE_FAILURE``: A disclosure scan rejected the candidate.
    ``TRUST_FAILURE``: A trust check failed.
    ``INTEGRITY_FAILURE``: A content hash or digest mismatch was detected.
    ``CHILD_FAILURE``: A child command returned a failure result.
    ``DIGEST_REJECTED``: The candidate digest did not match the approved digest.
    ``MIRROR_OUTAGE``: The publication mirror was unavailable.
    """

    COMPLETED = "completed"
    GRACEFUL_STOP = "graceful_stop"
    SAFETY_STOP = "safety_stop"
    BUDGET_EXHAUSTED = "budget_exhausted"
    DISK_FULL = "disk_full"
    AUTHORITY_FAILURE = "authority_failure"
    VERIFIER_FAILURE = "verifier_failure"
    DISCLOSURE_FAILURE = "disclosure_failure"
    TRUST_FAILURE = "trust_failure"
    INTEGRITY_FAILURE = "integrity_failure"
    CHILD_FAILURE = "child_failure"
    DIGEST_REJECTED = "digest_rejected"
    MIRROR_OUTAGE = "mirror_outage"


# Stop reasons that trigger an immediate safety stop (not graceful).
_SAFETY_STOP_REASONS: frozenset[StopReason] = frozenset({
    StopReason.SAFETY_STOP,
    StopReason.BUDGET_EXHAUSTED,
    StopReason.DISK_FULL,
    StopReason.AUTHORITY_FAILURE,
    StopReason.VERIFIER_FAILURE,
    StopReason.DISCLOSURE_FAILURE,
    StopReason.TRUST_FAILURE,
    StopReason.INTEGRITY_FAILURE,
    StopReason.DIGEST_REJECTED,
})


class StopConditions(BaseModel):
    """Frozen stop conditions for a cycle.

    All ceilings are checked before each child starts. A ``None`` ceiling
    is not enforced. The boolean conditions control whether the
    corresponding failure class triggers a safety stop.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    max_budget_usd: float | None = Field(
        default=None,
        ge=0,
        description="Maximum total USD spend across the cycle. None disables the check.",
    )
    max_disk_gb: float | None = Field(
        default=None,
        ge=0,
        description="Maximum disk usage in GB across the report root. None disables the check.",
    )
    min_free_disk_bytes: int | None = Field(
        default=None,
        ge=0,
        description="Minimum free bytes required on the report filesystem. None disables the check.",
    )
    stop_on_verifier_failure: bool = Field(
        default=True,
        description="Safety-stop when aggregate verification fails.",
    )
    stop_on_disclosure_failure: bool = Field(
        default=True,
        description="Safety-stop when a disclosure scan rejects the candidate.",
    )
    stop_on_trust_failure: bool = Field(
        default=True,
        description="Safety-stop when a trust check fails.",
    )
    stop_on_integrity_failure: bool = Field(
        default=True,
        description="Safety-stop when a content hash or digest mismatch is detected.",
    )
    stop_on_authority_failure: bool = Field(
        default=True,
        description="Safety-stop when an authority mismatch or invalid authority is detected.",
    )


# ---------------------------------------------------------------------------
# Cycle manifest
# ---------------------------------------------------------------------------


class ChildCommand(BaseModel):
    """One immutable child command within a cycle.

    The ``command_name`` identifies the command to execute (e.g.
    ``campaign_run``). The ``args`` are typed arguments passed to the
    command handler. The ``report_dir`` is a relative path under the
    cycle's report root where the child writes its report.

    The ``content_hash`` is SHA-256 over canonical JSON of the command
    (excluding ``content_hash`` itself). The same command always
    produces the same hash.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    child_id: str = Field(min_length=1, description="Unique child identity within the cycle.")
    command_name: str = Field(min_length=1, description="Command name to execute.")
    args: dict[str, str] = Field(
        default_factory=dict,
        description="Typed arguments passed to the command handler.",
    )
    report_dir: str = Field(
        min_length=1,
        description="Relative report directory under the cycle's report root.",
    )
    content_hash: str = Field(
        min_length=64,
        max_length=64,
        description="SHA-256 over canonical JSON of the command.",
    )

    @model_validator(mode="after")
    def _validate_command(self) -> Self:
        _validate_relative_path(self.report_dir)
        expected = compute_child_command_hash(self)
        if self.content_hash != expected:
            raise ValueError(
                f"child command content_hash mismatch: "
                f"declared {self.content_hash!r}, computed {expected!r}"
            )
        return self


class CycleManifest(BaseModel):
    """Frozen manifest defining one continuous-operation cycle.

    Binds the cycle identity, schema version, ordered immutable child
    commands, stop conditions, aggregate verification and strict
    validation requirements, the approved candidate digest for
    publication, and the report root and outbox directory.

    The manifest is the frozen authority for one cycle. Changing any
    bound field changes the content hash and creates a new cycle
    identity. An interrupted cycle cannot be resumed; a replacement
    cycle manifest with a fresh child identity and report root
    authorizes the restart.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    cycle_id: str = Field(min_length=1, description="Cycle identity.")
    cycle_revision: str = Field(min_length=1, description="Cycle revision identifier.")
    schema_version: str = Field(
        default=CONTROLLER_SCHEMA_VERSION,
        description="Controller schema version.",
    )
    children: list[ChildCommand] = Field(
        min_length=1,
        description="Ordered immutable child commands for this cycle.",
    )
    stop_conditions: StopConditions = Field(
        default_factory=StopConditions,
        description="Frozen stop conditions for this cycle.",
    )
    require_aggregate_verification: bool = Field(
        default=True,
        description="Require passing aggregate verification before publication.",
    )
    require_strict_validation: bool = Field(
        default=True,
        description="Require passing strict validation before publication.",
    )
    approved_digest: str | None = Field(
        default=None,
        min_length=64,
        max_length=64,
        description="Approved candidate digest for exact-digest publication. None skips the check.",
    )
    report_root: str = Field(
        min_length=1,
        description="Relative report root directory for this cycle.",
    )
    outbox_dir: str = Field(
        min_length=1,
        description="Relative outbox directory for durable publication entries.",
    )
    content_hash: str = Field(
        min_length=64,
        max_length=64,
        description="SHA-256 over canonical JSON of the manifest.",
    )

    @model_validator(mode="after")
    def _validate_manifest(self) -> Self:
        _validate_relative_path(self.report_root)
        _validate_relative_path(self.outbox_dir)
        child_ids = [c.child_id for c in self.children]
        if len(child_ids) != len(set(child_ids)):
            raise ValueError(f"duplicate child_id in cycle manifest: {child_ids}")
        report_dirs = [c.report_dir for c in self.children]
        if len(report_dirs) != len(set(report_dirs)):
            raise ValueError(f"duplicate report_dir in cycle manifest: {report_dirs}")
        if self.schema_version != CONTROLLER_SCHEMA_VERSION:
            raise ValueError(
                f"schema_version must be {CONTROLLER_SCHEMA_VERSION!r}: "
                f"got {self.schema_version!r}"
            )
        expected = compute_cycle_manifest_hash(self)
        if self.content_hash != expected:
            raise ValueError(
                f"cycle manifest content_hash mismatch: "
                f"declared {self.content_hash!r}, computed {expected!r}"
            )
        return self


# ---------------------------------------------------------------------------
# Controller state
# ---------------------------------------------------------------------------


class ControllerStatus(StrEnum):
    """Lifecycle status of the controller."""

    IDLE = "idle"
    RUNNING = "running"
    GRACEFUL_STOPPING = "graceful_stopping"
    SAFETY_STOPPED = "safety_stopped"
    COMPLETED = "completed"
    FAILED = "failed"


class ControllerState(BaseModel):
    """Explicit state ownership for the controller.

    One ``ControllerState`` record identifies the controller instance,
    current status, current child, completed children, failed children,
    interrupted children, stop reason, and timestamps. The state is
    persisted to ``controller-state.json`` after every transition.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    controller_id: str = Field(min_length=1, description="Unique controller instance identity.")
    cycle_id: str = Field(min_length=1, description="Cycle identity from the manifest.")
    status: ControllerStatus = Field(description="Current controller status.")
    current_child_id: str | None = Field(
        default=None,
        description="Child currently being executed, if any.",
    )
    completed_children: list[str] = Field(
        default_factory=list,
        description="Children that completed successfully (in order).",
    )
    failed_children: list[str] = Field(
        default_factory=list,
        description="Children that failed (in order).",
    )
    interrupted_children: list[str] = Field(
        default_factory=list,
        description="Children interrupted by a safety stop (dead evidence).",
    )
    budget_spent_usd: float = Field(
        default=0.0,
        ge=0,
        description="Observed provider cost accumulated across completed child attempts.",
    )
    stop_reason: StopReason | None = Field(
        default=None,
        description="Reason the controller stopped, if stopped.",
    )
    started_at: str = Field(description="ISO timestamp when the cycle started.")
    updated_at: str = Field(description="ISO timestamp of the last state transition.")
    content_hash: str = Field(
        min_length=64,
        max_length=64,
        description="SHA-256 over canonical JSON of the state.",
    )

    @model_validator(mode="after")
    def _validate_state(self) -> Self:
        expected = compute_controller_state_hash(self)
        if self.content_hash != expected:
            raise ValueError(
                f"controller state content_hash mismatch: "
                f"declared {self.content_hash!r}, computed {expected!r}"
            )
        return self


class ControllerTransition(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    sequence: int = Field(ge=1)
    controller_id: str = Field(min_length=1)
    cycle_id: str = Field(min_length=1)
    previous_transition_hash: str = Field(min_length=64, max_length=64)
    state_hash: str = Field(min_length=64, max_length=64)
    status: ControllerStatus
    current_child_id: str | None = None
    recorded_at: str = Field(min_length=1)
    content_hash: str = Field(min_length=64, max_length=64)

    @model_validator(mode="after")
    def _validate_transition(self) -> Self:
        data = self.model_dump(mode="json")
        data.pop("content_hash", None)
        expected = _sha256(_canonical_json(data))
        if self.content_hash != expected:
            raise ValueError("controller transition content_hash mismatch")
        return self


class ControllerStopRequest(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    controller_id: str = Field(min_length=1)
    cycle_id: str = Field(min_length=1)
    immediate: bool
    requested_at: str = Field(min_length=1)
    content_hash: str = Field(min_length=64, max_length=64)

    @model_validator(mode="after")
    def _validate_request(self) -> Self:
        data = self.model_dump(mode="json")
        data.pop("content_hash", None)
        expected = _sha256(_canonical_json(data))
        if self.content_hash != expected:
            raise ValueError("controller stop request content_hash mismatch")
        return self


# ---------------------------------------------------------------------------
# Outbox
# ---------------------------------------------------------------------------


class OutboxEntry(BaseModel):
    """One durable outbox entry for publication.

    The entry binds the child identity, report directory, candidate
    digest, publication attempt count, published status, and content
    hash. The outbox is append-only: entries are enqueued after
    acceptance and never removed. Publication retries read the outbox
    and mark entries as published; they never rerun inference.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    entry_id: str = Field(min_length=1, description="Unique outbox entry identity.")
    cycle_id: str = Field(min_length=1, description="Cycle identity from the manifest.")
    child_id: str = Field(min_length=1, description="Child identity that produced the candidate.")
    report_dir: str = Field(min_length=1, description="Relative report directory of the candidate.")
    digest: str = Field(
        min_length=64,
        max_length=64,
        description="SHA-256 candidate digest for exact-digest publication.",
    )
    publication_attempts: int = Field(
        default=0,
        ge=0,
        description="Number of publication attempts for this entry.",
    )
    published: bool = Field(
        default=False,
        description="True when publication succeeded.",
    )
    enqueued_at: str = Field(description="ISO timestamp when the entry was enqueued.")
    content_hash: str = Field(
        min_length=64,
        max_length=64,
        description="SHA-256 over canonical JSON of the entry.",
    )

    @model_validator(mode="after")
    def _validate_entry(self) -> Self:
        if PurePosixPath(self.entry_id).name != self.entry_id or self.entry_id in {".", ".."}:
            raise ValueError("outbox entry_id must be a safe filename")
        expected = compute_outbox_entry_hash(self)
        if self.content_hash != expected:
            raise ValueError(
                f"outbox entry content_hash mismatch: "
                f"declared {self.content_hash!r}, computed {expected!r}"
            )
        return self


class OutboxIndexRecord(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    entry_id: str = Field(min_length=1)
    action: Literal["enqueue", "attempt", "published"]


class Outbox:
    """Durable outbox for publication retries.

    The outbox persists entries to ``outbox-index.jsonl`` (one JSON
    object per line, append-only) and entry payloads to
    ``outbox-entries/<entry_id>.json``. Publication retries read the
    index, find unpublished entries in enqueue order, and attempt
    publication. A successful publication marks the entry as published
    in a new index line; the original entry is never mutated.

    Recovery reads the full index and reconstructs the ordered list of
    entries. Entries that were published are skipped; unpublished
    entries are retried in enqueue order.
    """

    def __init__(self, outbox_dir: Path) -> None:
        self._dir = Path(outbox_dir)
        self._index_path = self._dir / OUTBOX_INDEX_JSONL
        self._entries_dir = self._dir / OUTBOX_ENTRIES_DIR
        self._dir.mkdir(parents=True, exist_ok=True)
        self._entries_dir.mkdir(parents=True, exist_ok=True)
        if not self._index_path.exists():
            self._index_path.touch()

    def enqueue(self, entry: OutboxEntry) -> None:
        """Append one entry to the durable outbox index and payload."""
        enqueue_order, _, _ = self._read_index()
        payload_path = self._entries_dir / f"{entry.entry_id}.json"
        if entry.entry_id in enqueue_order or payload_path.exists():
            raise ValueError(f"outbox entry already exists: {entry.entry_id}")
        payload_path.write_text(_canonical_json(entry.model_dump(mode="json", by_alias=True)))
        self._append_index(entry.entry_id, "enqueue")

    def mark_published(self, entry_id: str) -> None:
        """Append a publication-success record to the durable outbox index."""
        enqueue_order, published, _ = self._read_index()
        if entry_id not in enqueue_order:
            raise ValueError(f"outbox entry is not enqueued: {entry_id}")
        if entry_id in published:
            raise ValueError(f"outbox entry is already published: {entry_id}")
        self._append_index(entry_id, "published")

    def mark_attempt(self, entry_id: str) -> None:
        """Append a publication-attempt record to the durable outbox index."""
        enqueue_order, published, _ = self._read_index()
        if entry_id not in enqueue_order:
            raise ValueError(f"outbox entry is not enqueued: {entry_id}")
        if entry_id in published:
            raise ValueError(f"published outbox entry cannot be attempted: {entry_id}")
        self._append_index(entry_id, "attempt")

    def recover(self) -> list[OutboxEntry]:
        """Read the full index and return unpublished entries in enqueue order.

        Parses the append-only index, tracks enqueue/publish/attempt
        actions per entry, and returns the unpublished entries in the
        order they were enqueued. Each returned entry has its
        ``publication_attempts`` updated from the index.
        """
        return [entry for entry in self._load_entries() if not entry.published]

    def all_entries(self) -> list[OutboxEntry]:
        """Return all entries in enqueue order, including published ones."""
        return self._load_entries()

    def _append_index(self, entry_id: str, action: Literal["enqueue", "attempt", "published"]) -> None:
        record = OutboxIndexRecord(entry_id=entry_id, action=action)
        with self._index_path.open("a") as stream:
            stream.write(_canonical_json(record.model_dump(mode="json")) + "\n")

    def _read_index(self) -> tuple[list[str], set[str], dict[str, int]]:
        enqueue_order: list[str] = []
        published: set[str] = set()
        attempts: dict[str, int] = {}
        for line in self._index_path.read_text().splitlines():
            if not line.strip():
                continue
            record = OutboxIndexRecord.model_validate_json(line)
            if record.action == "enqueue":
                if record.entry_id in enqueue_order:
                    raise ValueError(f"duplicate outbox enqueue action: {record.entry_id}")
                enqueue_order.append(record.entry_id)
                attempts[record.entry_id] = 0
                continue
            if record.entry_id not in enqueue_order:
                raise ValueError(f"outbox action precedes enqueue: {record.entry_id}")
            if record.entry_id in published:
                raise ValueError(f"outbox action follows publication: {record.entry_id}")
            if record.action == "attempt":
                attempts[record.entry_id] += 1
            else:
                published.add(record.entry_id)
        return enqueue_order, published, attempts

    def _load_entries(self) -> list[OutboxEntry]:
        enqueue_order, published, attempts = self._read_index()
        expected_payloads = {f"{entry_id}.json" for entry_id in enqueue_order}
        actual_payloads: set[str] = set()
        for path in self._entries_dir.iterdir():
            if path.is_symlink() or not path.is_file():
                raise ValueError(f"outbox payload is not a regular file: {path.name}")
            actual_payloads.add(path.name)
        if actual_payloads != expected_payloads:
            missing = sorted(expected_payloads - actual_payloads)
            unexpected = sorted(actual_payloads - expected_payloads)
            if missing:
                raise ValueError(f"outbox entry payload is missing: {missing}")
            raise ValueError(f"outbox contains unexpected payloads: {unexpected}")
        entries: list[OutboxEntry] = []
        for entry_id in enqueue_order:
            payload = OutboxEntry.model_validate_json((self._entries_dir / f"{entry_id}.json").read_text())
            if payload.entry_id != entry_id:
                raise ValueError(f"outbox payload identity mismatch: {entry_id}")
            if payload.publication_attempts != 0 or payload.published:
                raise ValueError(f"outbox payload contains mutable status: {entry_id}")
            data = payload.model_dump(mode="json")
            data["publication_attempts"] = attempts[entry_id]
            data["published"] = entry_id in published
            data["content_hash"] = _sha256(_canonical_json({key: value for key, value in data.items() if key != "content_hash"}))
            entries.append(OutboxEntry.model_validate(data))
        return entries


# ---------------------------------------------------------------------------
# Handler protocols
# ---------------------------------------------------------------------------


class ChildResult(BaseModel):
    """Result of executing one child command.

    ``ok`` is True when the child completed and produced a valid report
    directory. ``report_dir`` is the absolute path to the report. ``error``
    is a typed failure message when ``ok`` is False.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    child_id: str = Field(min_length=1, description="Child identity.")
    ok: bool = Field(description="True when the child completed successfully.")
    report_dir: str | None = Field(
        default=None,
        description="Absolute report directory path when ok.",
    )
    error: str | None = Field(
        default=None,
        description="Typed failure message when not ok.",
    )
    provider_cost_usd: float = Field(
        default=0.0,
        ge=0,
        description="Observed provider cost recorded by the completed child report.",
    )


class VerificationResult(BaseModel):
    """Result of aggregate verification across children."""

    model_config = ConfigDict(extra="forbid", frozen=True)

    ok: bool = Field(description="True when aggregate verification passed.")
    failures: list[str] = Field(
        default_factory=list,
        description="Sorted failure messages when not ok.",
    )


class ValidationResult(BaseModel):
    """Result of strict validation of the candidate."""

    model_config = ConfigDict(extra="forbid", frozen=True)

    ok: bool = Field(description="True when strict validation passed.")
    digest: str = Field(
        description="SHA-256 candidate digest computed by the validator.",
    )
    failures: list[str] = Field(
        default_factory=list,
        description="Sorted failure messages when not ok.",
    )


class PublicationResult(BaseModel):
    """Result of one publication attempt from the outbox."""

    model_config = ConfigDict(extra="forbid", frozen=True)

    ok: bool = Field(description="True when publication succeeded.")
    error: str | None = Field(
        default=None,
        description="Typed failure message when not ok.",
    )


class ChildCommandHandler(Protocol):
    """Handler for executing one immutable child command."""

    def __call__(self, child: ChildCommand, report_dir: Path) -> ChildResult: ...


class AggregateVerifierHandler(Protocol):
    """Handler for aggregate verification across children."""

    def __call__(
        self,
        children: list[ChildCommand],
        report_dirs: dict[str, Path],
    ) -> VerificationResult: ...


class ValidationHandler(Protocol):
    """Handler for strict validation and digest computation of the candidate."""

    def __call__(self, report_dirs: dict[str, Path]) -> ValidationResult: ...


class PublicationHandler(Protocol):
    """Handler for publishing one outbox entry to the mirror."""

    def __call__(self, entry: OutboxEntry, report_dirs: dict[str, Path]) -> PublicationResult: ...


# ---------------------------------------------------------------------------
# Hash computation functions
# ---------------------------------------------------------------------------


def compute_child_command_hash(command: ChildCommand) -> str:
    """Compute the content hash for a child command."""
    data = command.model_dump(mode="json", by_alias=True)
    data.pop("content_hash", None)
    return _sha256(_canonical_json(data))


def compute_cycle_manifest_hash(manifest: CycleManifest) -> str:
    """Compute the content hash for a cycle manifest."""
    data = manifest.model_dump(mode="json", by_alias=True)
    data.pop("content_hash", None)
    return _sha256(_canonical_json(data))


def compute_controller_state_hash(state: ControllerState) -> str:
    """Compute the content hash for a controller state."""
    data = state.model_dump(mode="json", by_alias=True)
    data.pop("content_hash", None)
    return _sha256(_canonical_json(data))


def compute_outbox_entry_hash(entry: OutboxEntry) -> str:
    """Compute the content hash for an outbox entry."""
    data = entry.model_dump(mode="json", by_alias=True)
    data.pop("content_hash", None)
    return _sha256(_canonical_json(data))


# ---------------------------------------------------------------------------
# Manifest and state I/O
# ---------------------------------------------------------------------------


def load_cycle_manifest(path: Path) -> CycleManifest:
    """Load and validate a cycle manifest from a JSON file."""
    return CycleManifest.model_validate_json(Path(path).read_text())


def save_cycle_manifest(manifest: CycleManifest, path: Path) -> None:
    """Persist a cycle manifest to a JSON file."""
    Path(path).write_text(_canonical_json(manifest.model_dump(mode="json", by_alias=True)))


def load_controller_state(path: Path) -> ControllerState:
    """Load and validate a controller state from a JSON file."""
    return ControllerState.model_validate_json(Path(path).read_text())


def save_controller_state(state: ControllerState, path: Path) -> None:
    """Persist a controller state to a JSON file."""
    Path(path).write_text(_canonical_json(state.model_dump(mode="json", by_alias=True)))


def load_controller_transitions(path: Path) -> list[ControllerTransition]:
    transitions: list[ControllerTransition] = []
    if path.exists():
        transitions = [
            ControllerTransition.model_validate_json(line)
            for line in path.read_text().splitlines()
            if line.strip()
        ]
    for index, transition in enumerate(transitions):
        if transition.sequence != index + 1:
            raise ControllerError("controller transition sequence is not contiguous")
        expected_previous = _ZERO_HASH if index == 0 else transitions[index - 1].content_hash
        if transition.previous_transition_hash != expected_previous:
            raise ControllerError("controller transition hash chain is invalid")
    return transitions


def _append_controller_transition(path: Path, state: ControllerState) -> ControllerTransition:
    transitions = load_controller_transitions(path)
    data = {
        "sequence": len(transitions) + 1,
        "controller_id": state.controller_id,
        "cycle_id": state.cycle_id,
        "previous_transition_hash": transitions[-1].content_hash if transitions else _ZERO_HASH,
        "state_hash": state.content_hash,
        "status": state.status,
        "current_child_id": state.current_child_id,
        "recorded_at": state.updated_at,
    }
    transition = ControllerTransition(**data, content_hash=_sha256(_canonical_json(data)))
    with path.open("a") as stream:
        stream.write(_canonical_json(transition.model_dump(mode="json")) + "\n")
    return transition


def request_controller_stop(work_dir: Path, *, immediate: bool) -> ControllerStopRequest:
    root = Path(work_dir)
    state = load_controller_state(root / CONTROLLER_STATE_JSON)
    if state.status not in {ControllerStatus.IDLE, ControllerStatus.RUNNING, ControllerStatus.GRACEFUL_STOPPING}:
        raise ControllerError(f"controller is not active: {state.status.value}")
    path = root / CONTROLLER_STOP_REQUEST_JSON
    if path.exists():
        raise ControllerError("controller stop request already exists")
    data = {
        "controller_id": state.controller_id,
        "cycle_id": state.cycle_id,
        "immediate": immediate,
        "requested_at": _now_iso(),
    }
    request = ControllerStopRequest(**data, content_hash=_sha256(_canonical_json(data)))
    path.write_text(_canonical_json(request.model_dump(mode="json")))
    return request


def recover_interrupted_controller(work_dir: Path) -> ControllerState:
    root = Path(work_dir)
    state_path = root / CONTROLLER_STATE_JSON
    state = load_controller_state(state_path)
    if state.status not in {ControllerStatus.IDLE, ControllerStatus.RUNNING, ControllerStatus.GRACEFUL_STOPPING}:
        raise ControllerError(f"controller is not recoverable: {state.status.value}")
    interrupted = list(state.interrupted_children)
    if state.current_child_id is not None and state.current_child_id not in interrupted:
        interrupted.append(state.current_child_id)
    recovered = make_controller_state(
        controller_id=state.controller_id,
        cycle_id=state.cycle_id,
        status=ControllerStatus.SAFETY_STOPPED,
        completed_children=list(state.completed_children),
        failed_children=list(state.failed_children),
        interrupted_children=interrupted,
        budget_spent_usd=state.budget_spent_usd,
        stop_reason=StopReason.SAFETY_STOP,
        started_at=state.started_at,
    )
    save_controller_state(recovered, state_path)
    _append_controller_transition(root / CONTROLLER_TRANSITIONS_JSONL, recovered)
    return recovered


def retry_controller_publication(
    work_dir: Path,
    publication_handler: PublicationHandler,
) -> ControllerState:
    root = Path(work_dir)
    manifest = load_cycle_manifest(root / CYCLE_MANIFEST_JSON)
    state_path = root / CONTROLLER_STATE_JSON
    state = load_controller_state(state_path)
    if state.cycle_id != manifest.cycle_id:
        raise ControllerError("controller state and cycle manifest identity mismatch")
    if state.status != ControllerStatus.SAFETY_STOPPED or state.stop_reason != StopReason.MIRROR_OUTAGE:
        raise ControllerError("controller publication is not recoverable")
    if state.current_child_id is not None or state.failed_children or state.interrupted_children:
        raise ControllerError("controller publication recovery requires completed child execution")
    expected_children = [child.child_id for child in manifest.children]
    if state.completed_children != expected_children:
        raise ControllerError("controller publication recovery child set mismatch")

    transitions_path = root / CONTROLLER_TRANSITIONS_JSONL
    transitions = load_controller_transitions(transitions_path)
    if not transitions:
        raise ControllerError("controller transition history is missing")
    transition = transitions[-1]
    if (
        transition.controller_id != state.controller_id
        or transition.cycle_id != state.cycle_id
        or transition.state_hash != state.content_hash
        or transition.status != state.status
        or transition.current_child_id != state.current_child_id
    ):
        raise ControllerError("controller transition tip does not match durable state")

    if manifest.approved_digest is None:
        raise ControllerError("controller publication recovery requires an approved digest")
    report_root = root / manifest.report_root
    if report_root.is_symlink():
        raise ControllerError("controller report root must not be a symlink")
    report_dirs: dict[str, Path] = {}
    for child in manifest.children:
        report_dir = report_root / child.report_dir
        if report_dir.is_symlink() or not report_dir.is_dir():
            raise ControllerError(f"controller report directory is invalid: {child.child_id}")
        report_dirs[child.child_id] = report_dir
    try:
        digest = compute_candidate_tree_digest(report_root)
    except CandidateDigestError as error:
        raise ControllerError(f"controller candidate digest validation failed: {error}") from error
    if digest != manifest.approved_digest:
        raise ControllerError("controller candidate digest does not match approved digest")

    outbox_dir = root / manifest.outbox_dir
    if not outbox_dir.is_dir():
        raise ControllerError("controller publication outbox is missing")
    outbox = Outbox(outbox_dir)
    entries = outbox.all_entries()
    if not entries:
        raise ControllerError("controller publication outbox is empty")
    for entry in entries:
        if (
            entry.cycle_id != manifest.cycle_id
            or entry.child_id != "aggregate"
            or entry.report_dir != manifest.report_root
            or entry.digest != digest
        ):
            raise ControllerError("controller publication outbox binding mismatch")

    for entry in outbox.recover():
        outbox.mark_attempt(entry.entry_id)
        result = publication_handler(entry, report_dirs)
        if not result.ok:
            retry_state = make_controller_state(
                controller_id=state.controller_id,
                cycle_id=state.cycle_id,
                status=ControllerStatus.SAFETY_STOPPED,
                completed_children=list(state.completed_children),
                budget_spent_usd=state.budget_spent_usd,
                stop_reason=StopReason.MIRROR_OUTAGE,
                started_at=state.started_at,
            )
            save_controller_state(retry_state, state_path)
            _append_controller_transition(transitions_path, retry_state)
            return retry_state
        outbox.mark_published(entry.entry_id)

    completed = make_controller_state(
        controller_id=state.controller_id,
        cycle_id=state.cycle_id,
        status=ControllerStatus.COMPLETED,
        completed_children=list(state.completed_children),
        budget_spent_usd=state.budget_spent_usd,
        stop_reason=StopReason.COMPLETED,
        started_at=state.started_at,
    )
    save_controller_state(completed, state_path)
    _append_controller_transition(transitions_path, completed)
    return completed


def make_controller_state(
    controller_id: str,
    cycle_id: str,
    status: ControllerStatus = ControllerStatus.IDLE,
    current_child_id: str | None = None,
    completed_children: list[str] | None = None,
    failed_children: list[str] | None = None,
    interrupted_children: list[str] | None = None,
    budget_spent_usd: float = 0.0,
    stop_reason: StopReason | None = None,
    started_at: str | None = None,
    updated_at: str | None = None,
) -> ControllerState:
    """Construct a ControllerState with a computed content hash."""
    now = _now_iso()
    partial = ControllerState.model_construct(
        controller_id=controller_id,
        cycle_id=cycle_id,
        status=status,
        current_child_id=current_child_id,
        completed_children=completed_children or [],
        failed_children=failed_children or [],
        interrupted_children=interrupted_children or [],
        budget_spent_usd=budget_spent_usd,
        stop_reason=stop_reason,
        started_at=started_at or now,
        updated_at=updated_at or now,
        content_hash=_ZERO_HASH,
    )
    content_hash = compute_controller_state_hash(partial)
    return ControllerState(
        controller_id=controller_id,
        cycle_id=cycle_id,
        status=status,
        current_child_id=current_child_id,
        completed_children=completed_children or [],
        failed_children=failed_children or [],
        interrupted_children=interrupted_children or [],
        budget_spent_usd=budget_spent_usd,
        stop_reason=stop_reason,
        started_at=started_at or now,
        updated_at=updated_at or now,
        content_hash=content_hash,
    )


def make_outbox_entry(
    entry_id: str,
    cycle_id: str,
    child_id: str,
    report_dir: str,
    digest: str,
    enqueued_at: str | None = None,
) -> OutboxEntry:
    """Construct an OutboxEntry with a computed content hash."""
    now = _now_iso()
    partial = OutboxEntry.model_construct(
        entry_id=entry_id,
        cycle_id=cycle_id,
        child_id=child_id,
        report_dir=report_dir,
        digest=digest,
        publication_attempts=0,
        published=False,
        enqueued_at=enqueued_at or now,
        content_hash=_ZERO_HASH,
    )
    content_hash = compute_outbox_entry_hash(partial)
    return OutboxEntry(
        entry_id=entry_id,
        cycle_id=cycle_id,
        child_id=child_id,
        report_dir=report_dir,
        digest=digest,
        publication_attempts=0,
        published=False,
        enqueued_at=enqueued_at or now,
        content_hash=content_hash,
    )


def make_child_command(
    child_id: str,
    command_name: str,
    args: dict[str, str] | None = None,
    report_dir: str | None = None,
) -> ChildCommand:
    """Construct a ChildCommand with a computed content hash."""
    resolved_args = args or {}
    resolved_report_dir = report_dir or f"reports/{child_id}"
    partial = ChildCommand.model_construct(
        child_id=child_id,
        command_name=command_name,
        args=resolved_args,
        report_dir=resolved_report_dir,
        content_hash=_ZERO_HASH,
    )
    content_hash = compute_child_command_hash(partial)
    return ChildCommand(
        child_id=child_id,
        command_name=command_name,
        args=resolved_args,
        report_dir=resolved_report_dir,
        content_hash=content_hash,
    )


def make_replacement_child_command(
    original: ChildCommand,
    rule: ReplacementManifestRule,
    attempt_number: int,
    replacement_rule_path: str,
    report_dir: str,
) -> ChildCommand:
    if original.command_name != "campaign_run":
        raise ValueError("replacement rule applies only to campaign_run children")
    if original.child_id not in rule.replaceable_child_ids:
        raise ValueError("replacement rule does not authorize the original child")
    if attempt_number < 1 or attempt_number > rule.max_attempts_per_child:
        raise ValueError("replacement attempt exceeds the rule ceiling")
    if report_dir == original.report_dir:
        raise ValueError("replacement requires a fresh report directory")
    _validate_relative_path(replacement_rule_path)
    args = dict(original.args)
    replacement_id = compute_replacement_child_id(rule.rule_id, original.child_id, attempt_number)
    args["campaign-id"] = replacement_id
    args["replacement-rule"] = replacement_rule_path
    return make_child_command(
        child_id=replacement_id,
        command_name=original.command_name,
        args=args,
        report_dir=report_dir,
    )


def make_cycle_manifest(
    cycle_id: str,
    cycle_revision: str,
    children: list[ChildCommand],
    stop_conditions: StopConditions | None = None,
    require_aggregate_verification: bool = True,
    require_strict_validation: bool = True,
    approved_digest: str | None = None,
    report_root: str = "reports",
    outbox_dir: str = "outbox",
) -> CycleManifest:
    """Construct a CycleManifest with a computed content hash."""
    partial = CycleManifest.model_construct(
        cycle_id=cycle_id,
        cycle_revision=cycle_revision,
        schema_version=CONTROLLER_SCHEMA_VERSION,
        children=children,
        stop_conditions=stop_conditions or StopConditions(),
        require_aggregate_verification=require_aggregate_verification,
        require_strict_validation=require_strict_validation,
        approved_digest=approved_digest,
        report_root=report_root,
        outbox_dir=outbox_dir,
        content_hash=_ZERO_HASH,
    )
    content_hash = compute_cycle_manifest_hash(partial)
    return CycleManifest(
        cycle_id=cycle_id,
        cycle_revision=cycle_revision,
        schema_version=CONTROLLER_SCHEMA_VERSION,
        children=children,
        stop_conditions=stop_conditions or StopConditions(),
        require_aggregate_verification=require_aggregate_verification,
        require_strict_validation=require_strict_validation,
        approved_digest=approved_digest,
        report_root=report_root,
        outbox_dir=outbox_dir,
        content_hash=content_hash,
    )


# ---------------------------------------------------------------------------
# Controller
# ---------------------------------------------------------------------------


class ControllerError(Exception):
    """Raised when the controller encounters an unrecoverable error."""


class Controller:
    """Finite controller over one frozen cycle authority.

    Orchestrates one frozen cycle manifest through child execution,
    aggregate verification, strict validation, exact-digest approval,
    and durable outbox publication. The production CLI supplies attended
    command, verifier, validator, and public-push adapters.

    The controller owns one ``ControllerState`` record, persisted to
    ``controller-state.json`` after every transition. State ownership
    is unambiguous: the ``controller_id`` identifies the instance.

    Stop conditions are checked before each child starts. A graceful
    stop completes the current child and then stops. A safety stop
    halts immediately and marks the current child as interrupted (dead
    evidence). A replacement cycle manifest with a fresh child
    identity and report root authorizes the restart.
    """

    def __init__(
        self,
        manifest: CycleManifest,
        work_dir: Path,
        controller_id: str | None = None,
        child_handler: ChildCommandHandler | None = None,
        verifier_handler: AggregateVerifierHandler | None = None,
        validation_handler: ValidationHandler | None = None,
        publication_handler: PublicationHandler | None = None,
    ) -> None:
        self._manifest = manifest
        self._work_dir = Path(work_dir)
        self._controller_id = controller_id or f"ctrl-{os.getpid()}-{int(time.time())}"
        self._child_handler = child_handler
        self._verifier_handler = verifier_handler
        self._validation_handler = validation_handler
        self._publication_handler = publication_handler

        self._report_root = self._work_dir / manifest.report_root
        self._outbox_dir = self._work_dir / manifest.outbox_dir
        self._state_path = self._work_dir / CONTROLLER_STATE_JSON
        self._manifest_path = self._work_dir / CYCLE_MANIFEST_JSON
        self._transitions_path = self._work_dir / CONTROLLER_TRANSITIONS_JSONL
        self._stop_request_path = self._work_dir / CONTROLLER_STOP_REQUEST_JSON

        if self._state_path.exists():
            existing_state = load_controller_state(self._state_path)
            if existing_state.status in {ControllerStatus.IDLE, ControllerStatus.RUNNING, ControllerStatus.GRACEFUL_STOPPING}:
                raise ControllerError(f"active controller already exists: {existing_state.controller_id}")
            if existing_state.cycle_id == manifest.cycle_id:
                raise ControllerError("replacement requires a fresh cycle identity")
            if not self._manifest_path.exists():
                raise ControllerError("prior cycle manifest is missing")
            existing_manifest = load_cycle_manifest(self._manifest_path)
            if manifest.report_root == existing_manifest.report_root:
                raise ControllerError("replacement requires a fresh report root")
            if manifest.outbox_dir == existing_manifest.outbox_dir:
                raise ControllerError("replacement requires a fresh outbox directory")

        self._report_root.mkdir(parents=True, exist_ok=True)
        self._outbox = Outbox(self._outbox_dir)

        self._graceful_stop_requested = False
        self._safety_stop_requested = False
        self._started_at = _now_iso()
        self._budget_spent_usd = 0.0

        self._state = make_controller_state(
            controller_id=self._controller_id,
            cycle_id=manifest.cycle_id,
            status=ControllerStatus.IDLE,
            started_at=self._started_at,
        )
        save_cycle_manifest(manifest, self._manifest_path)
        self._persist_state()

    @property
    def state(self) -> ControllerState:
        return self._state

    @property
    def outbox(self) -> Outbox:
        return self._outbox

    def status(self) -> ControllerState:
        """Return the current controller state (owner-only read)."""
        return self._state

    def request_graceful_stop(self) -> None:
        """Request a graceful stop after the current child completes."""
        self._graceful_stop_requested = True

    def request_safety_stop(self) -> None:
        """Request an immediate safety stop."""
        self._safety_stop_requested = True

    def run_cycle(self) -> ControllerState:
        """Execute one cycle: children, verification, validation, publication.

        Returns the final controller state. The controller stops on
        any frozen stop condition, graceful stop request, or safety
        stop request. An interrupted child is dead evidence; the
        controller never resumes a partial report.
        """
        self._transition(ControllerStatus.RUNNING, current_child_id=None)
        self._consume_stop_request()

        report_dirs: dict[str, Path] = {}
        completed: list[str] = []
        failed: list[str] = []
        interrupted: list[str] = []

        for child in self._manifest.children:
            self._consume_stop_request()
            # Check safety stop before starting the next child.
            if self._safety_stop_requested:
                self._transition(
                    ControllerStatus.SAFETY_STOPPED,
                    completed_children=completed,
                    failed_children=failed,
                    interrupted_children=interrupted,
                    stop_reason=StopReason.SAFETY_STOP,
                )
                return self._state

            # Check graceful stop before starting the next child.
            if self._graceful_stop_requested:
                self._transition(
                    ControllerStatus.GRACEFUL_STOPPING,
                    completed_children=completed,
                    failed_children=failed,
                    interrupted_children=interrupted,
                    stop_reason=StopReason.GRACEFUL_STOP,
                )
                return self._state

            # Check stop conditions (budget, disk).
            stop_reason = self._check_stop_conditions()
            if stop_reason is not None:
                self._transition(
                    ControllerStatus.SAFETY_STOPPED,
                    current_child_id=child.child_id,
                    completed_children=completed,
                    failed_children=failed,
                    interrupted_children=[*interrupted, child.child_id],
                    stop_reason=stop_reason,
                )
                return self._state

            # Execute the child.
            self._transition(
                ControllerStatus.RUNNING,
                current_child_id=child.child_id,
                completed_children=completed,
                failed_children=failed,
                interrupted_children=interrupted,
            )

            child_report_dir = self._report_root / child.report_dir
            child_report_dir.mkdir(parents=True, exist_ok=True)

            result = self._execute_child(child, child_report_dir)
            self._consume_stop_request()

            # Check safety stop during child execution.
            if self._safety_stop_requested:
                interrupted.append(child.child_id)
                self._transition(
                    ControllerStatus.SAFETY_STOPPED,
                    current_child_id=None,
                    completed_children=completed,
                    failed_children=failed,
                    interrupted_children=interrupted,
                    stop_reason=StopReason.SAFETY_STOP,
                )
                return self._state

            if not result.ok:
                failed.append(child.child_id)
                if self._manifest.stop_conditions.stop_on_authority_failure:
                    self._transition(
                        ControllerStatus.SAFETY_STOPPED,
                        current_child_id=None,
                        completed_children=completed,
                        failed_children=failed,
                        interrupted_children=interrupted,
                        stop_reason=StopReason.CHILD_FAILURE,
                    )
                    return self._state
                # If not stopping on child failure, continue to next child.
                self._transition(
                    ControllerStatus.RUNNING,
                    current_child_id=None,
                    completed_children=completed,
                    failed_children=failed,
                    interrupted_children=interrupted,
                )
                continue

            self._budget_spent_usd += result.provider_cost_usd
            budget_limit = self._manifest.stop_conditions.max_budget_usd
            if budget_limit is not None and self._budget_spent_usd > budget_limit:
                completed.append(child.child_id)
                report_dirs[child.child_id] = Path(result.report_dir) if result.report_dir is not None else child_report_dir
                self._transition(
                    ControllerStatus.SAFETY_STOPPED,
                    completed_children=completed,
                    failed_children=failed,
                    interrupted_children=interrupted,
                    stop_reason=StopReason.BUDGET_EXHAUSTED,
                )
                return self._state

            completed.append(child.child_id)
            if result.report_dir is not None:
                report_dirs[child.child_id] = Path(result.report_dir)
            else:
                report_dirs[child.child_id] = child_report_dir

            self._transition(
                ControllerStatus.RUNNING,
                current_child_id=None,
                completed_children=completed,
                failed_children=failed,
                interrupted_children=interrupted,
            )

        # All children complete. Run aggregate verification if required.
        if self._manifest.require_aggregate_verification:
            verification = self._run_aggregate_verification(report_dirs)
            if not verification.ok:
                self._transition(
                    ControllerStatus.SAFETY_STOPPED,
                    completed_children=completed,
                    failed_children=failed,
                    interrupted_children=interrupted,
                    stop_reason=StopReason.VERIFIER_FAILURE,
                )
                return self._state

        # Run strict validation if required.
        digest = _ZERO_HASH
        if self._manifest.require_strict_validation:
            validation = self._run_validation(report_dirs)
            if not validation.ok:
                self._transition(
                    ControllerStatus.SAFETY_STOPPED,
                    completed_children=completed,
                    failed_children=failed,
                    interrupted_children=interrupted,
                    stop_reason=StopReason.INTEGRITY_FAILURE,
                )
                return self._state
            digest = validation.digest

            # Exact digest approval.
            if self._manifest.approved_digest is not None:
                if digest != self._manifest.approved_digest:
                    self._transition(
                        ControllerStatus.SAFETY_STOPPED,
                        completed_children=completed,
                        failed_children=failed,
                        interrupted_children=interrupted,
                        stop_reason=StopReason.DIGEST_REJECTED,
                    )
                    return self._state

        # Enqueue the accepted candidate into the durable outbox.
        entry = make_outbox_entry(
            entry_id=f"entry-{self._manifest.cycle_id}",
            cycle_id=self._manifest.cycle_id,
            child_id="aggregate",
            report_dir=self._manifest.report_root,
            digest=digest,
        )
        self._outbox.enqueue(entry)

        # Publish from the durable outbox.
        publication_result = self._publish_from_outbox(report_dirs)
        if not publication_result.ok:
            stop_reason = self._classify_publication_failure(publication_result)
            self._transition(
                ControllerStatus.SAFETY_STOPPED,
                completed_children=completed,
                failed_children=failed,
                interrupted_children=interrupted,
                stop_reason=stop_reason,
            )
            return self._state

        self._transition(
            ControllerStatus.COMPLETED,
            completed_children=completed,
            failed_children=failed,
            interrupted_children=interrupted,
            stop_reason=StopReason.COMPLETED,
        )
        return self._state

    def publish_pending(self, report_dirs: dict[str, Path]) -> PublicationResult:
        """Retry publication of unpublished outbox entries.

        Reads the durable outbox, finds unpublished entries in enqueue
        order, and attempts publication. Never reruns inference; the
        report directories are the same immutable artifacts from the
        original cycle.
        """
        entries = self._outbox.recover()
        if not entries:
            return PublicationResult(ok=True)
        for entry in entries:
            self._outbox.mark_attempt(entry.entry_id)
            result = self._publish_entry(entry, report_dirs)
            if result.ok:
                self._outbox.mark_published(entry.entry_id)
            else:
                return result
        return PublicationResult(ok=True)

    # -----------------------------------------------------------------------
    # Internal helpers
    # -----------------------------------------------------------------------

    def _consume_stop_request(self) -> None:
        if not self._stop_request_path.exists():
            return
        request = ControllerStopRequest.model_validate_json(self._stop_request_path.read_text())
        if request.controller_id != self._controller_id or request.cycle_id != self._manifest.cycle_id:
            raise ControllerError("controller stop request identity mismatch")
        if request.immediate:
            self._safety_stop_requested = True
        else:
            self._graceful_stop_requested = True
        self._stop_request_path.unlink()

    def _execute_child(self, child: ChildCommand, report_dir: Path) -> ChildResult:
        if self._child_handler is None:
            raise ControllerError(
                f"no child command handler configured for child {child.child_id}"
            )
        return self._child_handler(child, report_dir)

    def _run_aggregate_verification(
        self,
        report_dirs: dict[str, Path],
    ) -> VerificationResult:
        if self._verifier_handler is None:
            raise ControllerError("no aggregate verifier handler configured")
        return self._verifier_handler(self._manifest.children, report_dirs)

    def _run_validation(self, report_dirs: dict[str, Path]) -> ValidationResult:
        if self._validation_handler is None:
            raise ControllerError("no validation handler configured")
        return self._validation_handler(report_dirs)

    def _publish_from_outbox(
        self,
        report_dirs: dict[str, Path],
    ) -> PublicationResult:
        entries = self._outbox.recover()
        if not entries:
            return PublicationResult(ok=True)
        for entry in entries:
            self._outbox.mark_attempt(entry.entry_id)
            result = self._publish_entry(entry, report_dirs)
            if result.ok:
                self._outbox.mark_published(entry.entry_id)
            else:
                return result
        return PublicationResult(ok=True)

    def _publish_entry(
        self,
        entry: OutboxEntry,
        report_dirs: dict[str, Path],
    ) -> PublicationResult:
        if self._publication_handler is None:
            raise ControllerError("no publication handler configured")
        return self._publication_handler(entry, report_dirs)

    def _check_stop_conditions(self) -> StopReason | None:
        """Check budget and disk ceilings. Returns a stop reason if exceeded."""
        conditions = self._manifest.stop_conditions
        if conditions.max_budget_usd is not None and self._budget_spent_usd > conditions.max_budget_usd:
            return StopReason.BUDGET_EXHAUSTED
        if conditions.max_disk_gb is not None:
            usage_gb = _directory_size_gb(self._report_root)
            if usage_gb > conditions.max_disk_gb:
                return StopReason.DISK_FULL
        if conditions.min_free_disk_bytes is not None and shutil.disk_usage(self._report_root).free < conditions.min_free_disk_bytes:
            return StopReason.DISK_FULL
        return None

    def _classify_publication_failure(
        self,
        result: PublicationResult,
    ) -> StopReason:
        """Classify a publication failure into a typed stop reason."""
        if result.error and "mirror" in result.error.lower():
            return StopReason.MIRROR_OUTAGE
        if result.error and "trust" in result.error.lower():
            return StopReason.TRUST_FAILURE
        if result.error and "disclosure" in result.error.lower():
            return StopReason.DISCLOSURE_FAILURE
        if result.error and "integrity" in result.error.lower():
            return StopReason.INTEGRITY_FAILURE
        return StopReason.MIRROR_OUTAGE

    def _transition(
        self,
        status: ControllerStatus,
        current_child_id: str | None = None,
        completed_children: list[str] | None = None,
        failed_children: list[str] | None = None,
        interrupted_children: list[str] | None = None,
        stop_reason: StopReason | None = None,
    ) -> None:
        """Transition to a new state and persist it."""
        self._state = make_controller_state(
            controller_id=self._controller_id,
            cycle_id=self._manifest.cycle_id,
            status=status,
            current_child_id=current_child_id,
            completed_children=completed_children if completed_children is not None else self._state.completed_children,
            failed_children=failed_children if failed_children is not None else self._state.failed_children,
            interrupted_children=interrupted_children if interrupted_children is not None else self._state.interrupted_children,
            budget_spent_usd=self._budget_spent_usd,
            stop_reason=stop_reason,
            started_at=self._started_at,
            updated_at=_now_iso(),
        )
        self._persist_state()

    def _persist_state(self) -> None:
        save_controller_state(self._state, self._state_path)
        _append_controller_transition(self._transitions_path, self._state)


def _directory_size_gb(path: Path) -> float:
    """Compute the total size of a directory in GB."""
    if not path.exists():
        return 0.0
    total = 0
    for p in path.rglob("*"):
        if p.is_file():
            try:
                total += p.stat().st_size
            except OSError:
                continue
    return total / (1024 ** 3)


__all__ = [
    "CONTROLLER_SCHEMA_VERSION",
    "AggregateVerifierHandler",
    "ChildCommand",
    "ChildCommandHandler",
    "ChildResult",
    "Controller",
    "ControllerError",
    "ControllerState",
    "ControllerStatus",
    "ControllerStopRequest",
    "ControllerTransition",
    "CycleManifest",
    "Outbox",
    "OutboxEntry",
    "PublicationHandler",
    "PublicationResult",
    "StopConditions",
    "StopReason",
    "ValidationHandler",
    "ValidationResult",
    "VerificationResult",
    "compute_child_command_hash",
    "compute_controller_state_hash",
    "compute_cycle_manifest_hash",
    "compute_outbox_entry_hash",
    "load_controller_state",
    "load_controller_transitions",
    "load_cycle_manifest",
    "make_child_command",
    "make_controller_state",
    "make_cycle_manifest",
    "make_outbox_entry",
    "make_replacement_child_command",
    "recover_interrupted_controller",
    "request_controller_stop",
    "retry_controller_publication",
    "save_controller_state",
    "save_cycle_manifest",
]
