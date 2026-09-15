# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed operation configuration models for the unified eval tooling.

This module replaces the provisional flat ``CampaignRunConfig`` with
versioned operation documents that bind the entire lifecycle without
embedding implementation-location fields. Platform-owned fields
(``auth_project_root``, ``g8e_cli``, ``operator_url``, ``g8ee_url``,
canonical trust-bundle paths, default Gateway URLs) are injected by the
Go facade from the selected repository/runtime context and never appear
in user-authored config.

Authority paths are repository-relative. The Go facade resolves them to
absolute paths when invoking the engine. Absolute or escaping paths are
rejected at validation time.

Every config carries a content hash over canonical JSON excluding the
``content_hash`` field itself. The hash is verified on load; a mismatch
indicates tampering or drift.
"""

from __future__ import annotations

import hashlib
import json
from enum import StrEnum
from pathlib import Path
from typing import Literal, Self

from pydantic import BaseModel, ConfigDict, Field, model_validator


OPERATION_CONFIG_SCHEMA_VERSION = "1.0.0"


class OperationKind(StrEnum):
    """The kind of operation a config binds."""

    DIAGNOSTIC = "diagnostic"
    CAMPAIGN = "campaign"


class AuthorityRef(BaseModel):
    """A repository-relative authority file reference with its SHA-256
    content hash. The path is relative to the repository root; the Go
    facade resolves it to an absolute path when invoking the engine.
    Absolute and escaping paths are rejected.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    path: str = Field(min_length=1)
    sha256: str = Field(min_length=64, max_length=64)

    @model_validator(mode="after")
    def reject_absolute_or_escaping(self) -> Self:
        p = Path(self.path)
        if p.is_absolute():
            raise ValueError(
                f"authority path must be repository-relative, got absolute: {self.path}"
            )
        if ".." in p.parts:
            raise ValueError(
                f"authority path must not escape repository root: {self.path}"
            )
        return self


class EvidenceKeyRef(BaseModel):
    """Owner-local evidence key reference by path and key ID. Key bytes
    are never placed in config; the Go facade injects the key file path
    from the platform context and the engine reads it at runtime.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    path: str = Field(min_length=1)
    key_id: str = Field(min_length=1)

    @model_validator(mode="after")
    def reject_absolute_or_escaping(self) -> Self:
        p = Path(self.path)
        if p.is_absolute():
            raise ValueError(
                f"evidence key path must be repository-relative, got absolute: {self.path}"
            )
        if ".." in p.parts:
            raise ValueError(
                f"evidence key path must not escape repository root: {self.path}"
            )
        return self


class ProviderEndpointRef(BaseModel):
    """Provider endpoint class and reference without credentials. The
    provider name identifies the adapter family; the endpoint class
    identifies the configured endpoint group. Credentials remain in the
    supported environment or OS secret mechanism and are inherited by
    the child process environment.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    provider: str = Field(min_length=1)
    endpoint_class: str = Field(min_length=1)


class StopConditions(BaseModel):
    """Stop conditions for an operation. Idle timeout is the maximum
    seconds without an SSE event before declaring a task idle. Max
    duration is an optional whole-stream deadline.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    idle_timeout_s: float = Field(gt=0)
    max_duration_s: float | None = Field(default=None, gt=0)


class SamplingPolicy(BaseModel):
    """Sampling policy for model generation. All fields are optional;
    ``None`` means the provider default applies.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    temperature: float | None = Field(default=None, ge=0.0, le=2.0)
    top_p: float | None = Field(default=None, gt=0.0, le=1.0)
    max_output_tokens: int | None = Field(default=None, gt=0)


class BudgetCeilings(BaseModel):
    """Provider budget ceilings. All bounds are strict; zero USD is
    valid for local-only providers.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    max_requests: int = Field(gt=0)
    max_tokens: int = Field(gt=0)
    max_usd: float = Field(ge=0)
    max_retries: int = Field(default=1, ge=0)
    concurrency: int = Field(default=1, gt=0)
    min_free_disk_gb: float | None = Field(default=None, ge=0)


class OperationConfigBase(BaseModel):
    """Common fields shared by every operation config. Specialized
    configs extend this base with operation-specific fields.

    The ``content_hash`` field is ``None`` during construction and set
    by ``finalize()``. When loading a persisted config, the validator
    checks that the hash matches the canonical JSON of all other fields.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    schema_version: Literal["1.0.0"] = OPERATION_CONFIG_SCHEMA_VERSION
    operation_kind: OperationKind
    operation_id: str = Field(min_length=1)
    revision: str = Field(min_length=1)
    suite: str = Field(min_length=1)
    seed: int = Field(ge=0)
    report_root: str = Field(min_length=1)
    gold_set: AuthorityRef
    evidence_key: EvidenceKeyRef
    provider_endpoint: ProviderEndpointRef
    budget: BudgetCeilings
    stop_conditions: StopConditions
    content_hash: str | None = Field(default=None, min_length=64, max_length=64)

    @model_validator(mode="after")
    def reject_absolute_or_escaping_report_root(self) -> Self:
        p = Path(self.report_root)
        if p.is_absolute():
            raise ValueError(
                f"report_root must be repository-relative, got absolute: {self.report_root}"
            )
        if ".." in p.parts:
            raise ValueError(
                f"report_root must not escape repository root: {self.report_root}"
            )
        return self

    @model_validator(mode="after")
    def verify_content_hash(self) -> Self:
        if self.content_hash is None:
            return self
        expected = self.compute_content_hash()
        if self.content_hash != expected:
            raise ValueError(
                f"content_hash mismatch: expected {expected}, got {self.content_hash}"
            )
        return self

    def canonical_json_bytes(self) -> bytes:
        """Return the canonical JSON encoding of all fields except
        ``content_hash``, with sorted keys and compact separators.
        """
        data = self.model_dump(exclude={"content_hash"}, exclude_none=True)
        return json.dumps(data, sort_keys=True, separators=(",", ":"), ensure_ascii=True).encode("utf-8")

    def compute_content_hash(self) -> str:
        """Return the SHA-256 hex digest over the canonical JSON."""
        return hashlib.sha256(self.canonical_json_bytes()).hexdigest()

    def finalized(self) -> Self:
        """Return a copy with ``content_hash`` set to the canonical hash."""
        return self.model_copy(update={"content_hash": self.compute_content_hash()})


class DiagnosticConfig(OperationConfigBase):
    """Typed configuration for a single-arm diagnostic run. A diagnostic
    executes one arm against one model and produces one run. It does not
    create a campaign identity, assignment manifest, or randomized
    schedule.
    """

    operation_kind: Literal[OperationKind.DIAGNOSTIC] = OperationKind.DIAGNOSTIC
    model_variant_id: str = Field(min_length=1)
    arm: str = Field(min_length=1)
    sampling: SamplingPolicy = Field(default_factory=SamplingPolicy)
    preregistration: AuthorityRef | None = None
    task_limit: int | None = Field(default=None, gt=0)
    task_offset: int = Field(default=0, ge=0)

    @model_validator(mode="after")
    def validate_task_slice(self) -> Self:
        if self.task_offset > 0 and self.task_limit is not None and self.task_limit <= 0:
            raise ValueError("task_limit must be positive when task_offset is set")
        return self


class CampaignConfig(OperationConfigBase):
    """Typed configuration for an authoritative multi-arm, multi-cohort
    campaign. A campaign creates one campaign identity, one report
    directory, one assignment manifest, one randomized schedule, and one
    final canonical analysis.
    """

    operation_kind: Literal[OperationKind.CAMPAIGN] = OperationKind.CAMPAIGN
    campaign_id: str = Field(min_length=1)
    release_version: str = Field(min_length=1)
    preregistration: AuthorityRef
    profile: AuthorityRef
    model_registry: AuthorityRef
    model_tags: AuthorityRef | None = None
    arms: list[str] = Field(min_length=1)
    cohort_ids: list[str] = Field(min_length=1)
    repetitions: int = Field(default=1, gt=0)
    task_offset: int = Field(default=0, ge=0)
    task_limit: int | None = Field(default=None, gt=0)
    campaign_set_plan: AuthorityRef | None = None
    replacement_rule: AuthorityRef | None = None
    publication_eligible: bool = True

    @model_validator(mode="after")
    def validate_replacement_requires_campaign_set(self) -> Self:
        if self.replacement_rule is not None and self.campaign_set_plan is None:
            raise ValueError("replacement_rule requires campaign_set_plan")
        return self


class OperationConfigError(ValueError):
    """Raised when an operation config fails to load or validate."""


def load_operation_config(path: Path) -> OperationConfigBase:
    """Load and validate an operation config from a JSON file. The
    content hash is verified on load. Returns the specialized config
    instance (``DiagnosticConfig`` or ``CampaignConfig``) selected by
    ``operation_kind``.
    """
    raw = path.read_bytes()
    try:
        probe = json.loads(raw)
    except json.JSONDecodeError as exc:
        raise OperationConfigError(f"operation config is not valid JSON: {exc}") from exc
    if not isinstance(probe, dict):
        raise OperationConfigError("operation config must be a JSON object")
    kind = probe.get("operation_kind")
    if kind == OperationKind.DIAGNOSTIC.value:
        return DiagnosticConfig.model_validate_json(raw)
    if kind == OperationKind.CAMPAIGN.value:
        return CampaignConfig.model_validate_json(raw)
    raise OperationConfigError(f"unknown operation_kind: {kind!r}")


def write_operation_config(config: OperationConfigBase, path: Path) -> None:
    """Write a finalized config to a JSON file. The config is finalized
    (content hash computed) before writing. The output is canonical JSON
    with sorted keys and a trailing newline.
    """
    finalized = config.finalized()
    data = finalized.model_dump(exclude_none=True)
    text = json.dumps(data, sort_keys=True, indent=2, ensure_ascii=True)
    path.write_text(text + "\n")
