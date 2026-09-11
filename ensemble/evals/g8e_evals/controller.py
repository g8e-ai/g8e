# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Fixture-driven continuous controller core (CONT-CORE).

A small explicit controller that orchestrates existing commands over one
frozen cycle manifest. It does not recreate campaign, verifier, projector,
promoter, or outbox semantics; it composes pluggable handlers that own
those concerns. In Phase 2 the handlers are stubs; in Phase 7 (CONT-1) the
real command handlers are wired in.

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
import time
from enum import StrEnum
from pathlib import Path
from typing import Protocol, Self

from pydantic import BaseModel, ConfigDict, Field, model_validator

from g8e_evals.constants import (
    CONTROLLER_STATE_JSON,
    CYCLE_MANIFEST_JSON,
    OUTBOX_ENTRIES_DIR,
    OUTBOX_INDEX_JSONL,
)


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
        expected = compute_outbox_entry_hash(self)
        if self.content_hash != expected:
            raise ValueError(
                f"outbox entry content_hash mismatch: "
                f"declared {self.content_hash!r}, computed {expected!r}"
            )
        return self


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
        payload_path = self._entries_dir / f"{entry.entry_id}.json"
        if payload_path.exists():
            raise ValueError(f"outbox entry already exists: {entry.entry_id}")
        payload_path.write_text(_canonical_json(entry.model_dump(mode="json", by_alias=True)))
        with self._index_path.open("a") as f:
            f.write(_canonical_json({"entry_id": entry.entry_id, "action": "enqueue"}) + "\n")

    def mark_published(self, entry_id: str) -> None:
        """Append a publication-success record to the durable outbox index."""
        with self._index_path.open("a") as f:
            f.write(_canonical_json({"entry_id": entry_id, "action": "published"}) + "\n")

    def mark_attempt(self, entry_id: str) -> None:
        """Append a publication-attempt record to the durable outbox index."""
        with self._index_path.open("a") as f:
            f.write(_canonical_json({"entry_id": entry_id, "action": "attempt"}) + "\n")

    def recover(self) -> list[OutboxEntry]:
        """Read the full index and return unpublished entries in enqueue order.

        Parses the append-only index, tracks enqueue/publish/attempt
        actions per entry, and returns the unpublished entries in the
        order they were enqueued. Each returned entry has its
        ``publication_attempts`` updated from the index.
        """
        if not self._index_path.exists():
            return []
        enqueue_order: list[str] = []
        published: set[str] = set()
        attempts: dict[str, int] = {}
        for line in self._index_path.read_text().splitlines():
            line = line.strip()
            if not line:
                continue
            record = json.loads(line)
            entry_id = record["entry_id"]
            action = record["action"]
            if action == "enqueue":
                if entry_id not in enqueue_order:
                    enqueue_order.append(entry_id)
            elif action == "published":
                published.add(entry_id)
            elif action == "attempt":
                attempts[entry_id] = attempts.get(entry_id, 0) + 1
        unpublished_ids = [eid for eid in enqueue_order if eid not in published]
        entries: list[OutboxEntry] = []
        for eid in unpublished_ids:
            payload_path = self._entries_dir / f"{eid}.json"
            if not payload_path.exists():
                continue
            data = json.loads(payload_path.read_text())
            data["publication_attempts"] = attempts.get(eid, 0)
            data["published"] = False
            # Recompute hash since we mutated publication_attempts/published.
            data["content_hash"] = _sha256(_canonical_json({
                k: v for k, v in data.items() if k != "content_hash"
            }))
            entries.append(OutboxEntry.model_validate(data))
        return entries

    def all_entries(self) -> list[OutboxEntry]:
        """Return all entries in enqueue order, including published ones."""
        if not self._index_path.exists():
            return []
        enqueue_order: list[str] = []
        published: set[str] = set()
        attempts: dict[str, int] = {}
        for line in self._index_path.read_text().splitlines():
            line = line.strip()
            if not line:
                continue
            record = json.loads(line)
            entry_id = record["entry_id"]
            action = record["action"]
            if action == "enqueue":
                if entry_id not in enqueue_order:
                    enqueue_order.append(entry_id)
            elif action == "published":
                published.add(entry_id)
            elif action == "attempt":
                attempts[entry_id] = attempts.get(entry_id, 0) + 1
        entries: list[OutboxEntry] = []
        for eid in enqueue_order:
            payload_path = self._entries_dir / f"{eid}.json"
            if not payload_path.exists():
                continue
            data = json.loads(payload_path.read_text())
            data["publication_attempts"] = attempts.get(eid, 0)
            data["published"] = eid in published
            data["content_hash"] = _sha256(_canonical_json({
                k: v for k, v in data.items() if k != "content_hash"
            }))
            entries.append(OutboxEntry.model_validate(data))
        return entries


# ---------------------------------------------------------------------------
# Handler protocols (pluggable; stubs in Phase 2, real in Phase 7)
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


def make_controller_state(
    controller_id: str,
    cycle_id: str,
    status: ControllerStatus = ControllerStatus.IDLE,
    current_child_id: str | None = None,
    completed_children: list[str] | None = None,
    failed_children: list[str] | None = None,
    interrupted_children: list[str] | None = None,
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
    """Fixture-driven continuous controller core.

    Orchestrates one frozen cycle manifest through child execution,
    aggregate verification, strict validation, exact-digest approval,
    and durable outbox publication. Handlers are injected so Phase 2
    uses stubs and Phase 7 wires real commands.

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

        self._report_root.mkdir(parents=True, exist_ok=True)
        self._outbox = Outbox(self._outbox_dir)

        self._graceful_stop_requested = False
        self._safety_stop_requested = False
        self._started_at = _now_iso()

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

        report_dirs: dict[str, Path] = {}
        completed: list[str] = []
        failed: list[str] = []
        interrupted: list[str] = []

        for child in self._manifest.children:
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
        if conditions.max_budget_usd is not None:
            # In Phase 2, budget tracking is delegated to the child handler.
            # The controller checks the accumulated spend via a hook if
            # provided. For now, the stub handler does not track budget.
            # Real budget tracking is wired in Phase 7 (CONT-1).
            pass
        if conditions.max_disk_gb is not None:
            usage_gb = _directory_size_gb(self._report_root)
            if usage_gb > conditions.max_disk_gb:
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
            stop_reason=stop_reason,
            started_at=self._started_at,
            updated_at=_now_iso(),
        )
        self._persist_state()

    def _persist_state(self) -> None:
        save_controller_state(self._state, self._state_path)


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
    "load_cycle_manifest",
    "make_child_command",
    "make_controller_state",
    "make_cycle_manifest",
    "make_outbox_entry",
    "save_controller_state",
    "save_cycle_manifest",
]
