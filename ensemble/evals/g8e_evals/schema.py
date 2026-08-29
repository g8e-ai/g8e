# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Canonical analytical data model for evaluation runs.

These typed Pydantic schemas replace ad hoc dictionaries with versioned,
schema-valid records. Analytical records contain references to immutable
evidence rather than nested ad hoc dictionaries.

The schema version is pinned per record type. Breaking changes increment
the major version; additive changes increment the minor version. A run
manifest declares its schema version and offline verification rejects
bundles whose records do not match the declared version.
"""

from __future__ import annotations

from datetime import datetime, timezone, UTC
from enum import StrEnum
from typing import Any

from pydantic import BaseModel, ConfigDict, Field

from g8e_evals.arms import Arm, GovernancePosture


SCHEMA_VERSION = "1.5.0"


class TerminalStatus(StrEnum):
    COMPLETED = "completed"
    MODEL_FAILED = "model_failed"
    GOVERNANCE_REJECTED = "governance_rejected"
    HUMAN_DENIED = "human_denied"
    TIMED_OUT = "timed_out"
    INFRASTRUCTURE_FAILED = "infrastructure_failed"
    INVALID_EVIDENCE = "invalid_evidence"


class StageKind(StrEnum):
    MODEL_INFERENCE = "model_inference"
    DETERMINISTIC_DOCTRINE = "deterministic_doctrine"
    TRIBUNAL_GENERATION = "tribunal_generation"
    TRIBUNAL_AUDITOR = "tribunal_auditor"
    PROTOCOL_L2 = "protocol_l2"
    L3_CEREMONY = "l3_ceremony"
    L4_VERIFICATION = "l4_verification"
    L5_EXECUTION = "l5_execution"
    SCRUBBING = "scrubbing"
    REHYDRATION = "rehydration"
    GRADING = "grading"
    RECEIPT_PERSISTENCE = "receipt_persistence"
    COMMITMENT_APPEND = "commitment_append"


class EvidenceMediaType(StrEnum):
    APPLICATION_JSON = "application/json"
    TEXT_PLAIN = "text/plain"
    APPLICATION_OCTET_STREAM = "application/octet-stream"


class PrivacyClassification(StrEnum):
    PUBLIC = "public"
    INTERNAL = "internal"
    RESTRICTED = "restricted"


class GraderClass(StrEnum):
    DETERMINISTIC = "deterministic"
    HUMAN = "human"
    LLM_JUDGE = "llm_judge"


class VerificationStatus(StrEnum):
    PENDING = "pending"
    VERIFIED = "verified"
    FAILED = "failed"
    NOT_APPLICABLE = "not_applicable"


class ModelIdentity(BaseModel):
    """Exact model identification for one role in a run."""

    model_config = ConfigDict(extra="forbid")

    role: str = Field(description="Agent role: primary, assistant, lite, etc.")
    provider: str = Field(description="Provider identifier: openai, anthropic, ollama, etc.")
    model: str = Field(description="Exact model name as passed to the provider API.")
    endpoint: str | None = Field(default=None, description="Provider endpoint URL, classified as local or remote.")
    endpoint_class: str = Field(default="remote", description="local or remote")
    api_key_present: bool = Field(default=False, description="Whether an API key was supplied. Never the key itself.")
    seed_support: str = Field(default="unknown", description="none, deterministic, or unknown")


class RoleToModelMapping(BaseModel):
    """Mapping from agent role to exact model identity."""

    model_config = ConfigDict(extra="forbid")

    primary: ModelIdentity | None = None
    assistant: ModelIdentity | None = None
    lite: ModelIdentity | None = None


class SamplingSettings(BaseModel):
    """Sampling configuration declared for the run."""

    model_config = ConfigDict(extra="forbid")

    max_output_tokens: int | None = None
    top_p: float | None = None
    top_k: int | None = None
    temperature: float | None = None
    stop_sequences: list[str] = Field(default_factory=list)
    seed: int | None = None


class StackEnvironment(BaseModel):
    """Hardware and runtime environment metadata."""

    model_config = ConfigDict(extra="forbid")

    os: str = ""
    arch: str = ""
    cpu: str = ""
    ram: str = ""
    gpu: str = ""
    runtime_version: str = ""
    network_mode: str = ""
    stack_image_digests: dict[str, str] = Field(default_factory=dict)


class ArmManifestEntry(BaseModel):
    """One arm's declaration in the run manifest."""

    model_config = ConfigDict(extra="forbid")

    arm_id: Arm
    requested_posture: GovernancePosture
    uses_g8ee: bool
    uses_gateway: bool
    receipt_binding: bool
    is_production_posture: bool


class ContentHash(BaseModel):
    """A named content hash for an immutable input or artifact."""

    model_config = ConfigDict(extra="forbid")

    name: str
    sha256: str
    byte_length: int = 0


class RunManifest(BaseModel):
    """Immutable run manifest written before execution begins.

    The manifest records every input, configuration, and identity that
    influences the run outcome. It is written to ``manifest.json`` in the
    report directory before any task executes. The runner refuses to start
    when a required hash or model identity is unavailable.
    """

    model_config = ConfigDict(extra="forbid")

    schema_version: str = SCHEMA_VERSION
    run_id: str
    suite_id: str
    suite_version: str
    created_at: datetime = Field(default_factory=lambda: datetime.now(UTC))
    orchestrator_version: str = ""
    experiment_preregistration_hash: str | None = None

    source_revision: str = ""
    source_tree_state_hash: str = ""

    content_hashes: list[ContentHash] = Field(default_factory=list)

    arms: list[ArmManifestEntry] = Field(default_factory=list)

    role_to_model: RoleToModelMapping = Field(default_factory=RoleToModelMapping)
    sampling: SamplingSettings = Field(default_factory=SamplingSettings)
    context_limits: dict[str, int] = Field(default_factory=dict)

    stack_environment: StackEnvironment = Field(default_factory=StackEnvironment)

    redacted_config: dict[str, Any] = Field(default_factory=dict)

    @property
    def dataset_hash(self) -> str | None:
        for h in self.content_hashes:
            if h.name == "dataset":
                return h.sha256
        return None

    @property
    def grader_bundle_hash(self) -> str | None:
        for h in self.content_hashes:
            if h.name == "grader_bundle":
                return h.sha256
        return None

    @property
    def prompt_bundle_hash(self) -> str | None:
        for h in self.content_hashes:
            if h.name == "prompt_bundle":
                return h.sha256
        return None


class TaskDefinition(BaseModel):
    """Immutable task definition with expected outcomes and grader references.

    A task definition is stable across runs. It references its prompt and
    initial-state fixture by hash so the same definition always produces
    the same starting conditions.
    """

    model_config = ConfigDict(extra="forbid")

    schema_version: str = SCHEMA_VERSION
    task_id: str
    suite_id: str
    suite_version: str
    category: str = ""
    domain: str = ""
    difficulty: str = ""
    risk: str = ""
    expected_action_class: str = ""
    compatible_arms: list[Arm] = Field(default_factory=list)

    prompt_hash: str
    prompt_length: int = 0

    initial_state_fixture_hash: str | None = None
    expected_final_state_assertions: list[str] = Field(default_factory=list)

    expected_allow_block_outcome: str | None = None
    expected_rejection_layer: str | None = None

    sensitive_canary_annotations: list[str] = Field(default_factory=list)
    privacy_assertions: list[str] = Field(default_factory=list)

    grader_ids: list[str] = Field(default_factory=list)
    grader_versions: list[str] = Field(default_factory=list)

    metadata: dict[str, Any] = Field(default_factory=dict)


class PostureObservation(BaseModel):
    """Requested and independently observed effective posture for one attempt.

    The runner records the requested posture from the arm definition and
    the observed effective posture from the gateway. It never infers
    posture from the CLI argument alone.
    """

    model_config = ConfigDict(extra="forbid")

    requested_posture: GovernancePosture
    observed_posture: GovernancePosture | None = None
    observation_source: str = ""
    observation_timestamp: datetime | None = None
    posture_match: bool | None = None


class UsageReconciliation(BaseModel):
    model_config = ConfigDict(extra="forbid")

    reported_input_tokens: int = 0
    reported_output_tokens: int = 0
    reported_thinking_tokens: int = 0
    observed_input_tokens: int = 0
    observed_output_tokens: int = 0
    observed_thinking_tokens: int = 0
    observed_call_count: int = 0
    expected_call_count: int = 0

    @property
    def input_token_delta(self) -> int:
        return self.observed_input_tokens - self.reported_input_tokens

    @property
    def output_token_delta(self) -> int:
        return self.observed_output_tokens - self.reported_output_tokens

    @property
    def thinking_token_delta(self) -> int:
        return self.observed_thinking_tokens - self.reported_thinking_tokens

    @property
    def reconciled(self) -> bool:
        return (
            self.observed_call_count == self.expected_call_count
            and self.input_token_delta == 0
            and self.output_token_delta == 0
            and self.thinking_token_delta == 0
        )


class AttemptRecord(BaseModel):
    """One immutable attempt: one task instance executed by one arm.

    The logical key is (task_id, arm_id, model_cohort_id,
    state_snapshot_hash, replicate_id). Infrastructure retries create a new
    attempt record linked to the failed attempt; they never overwrite it.
    """

    model_config = ConfigDict(extra="forbid")

    schema_version: str = SCHEMA_VERSION
    attempt_id: str
    run_id: str
    task_id: str
    arm_id: Arm
    model_cohort_id: str = ""
    state_snapshot_hash: str = ""
    replicate_id: str = "1"
    assignment_order: int = 0

    started_at: datetime | None = None
    ended_at: datetime | None = None

    terminal_status: TerminalStatus = TerminalStatus.COMPLETED
    posture: PostureObservation = Field(default_factory=lambda: PostureObservation(
        requested_posture=GovernancePosture.NONE
    ))

    state_root_before: str | None = None
    state_root_after: str | None = None

    correlation_ids: dict[str, str] = Field(default_factory=dict)

    answer_ref: str | None = None
    final_state_observation_ref: str | None = None
    grade_refs: list[str] = Field(default_factory=list)

    missingness_or_failure: str | None = None
    usage_reconciliation: UsageReconciliation | None = None

    parent_attempt_id: str | None = None


class StageObservation(BaseModel):
    """One observation for a deterministic stage or model call.

    Separate kinds cover model inference, deterministic doctrine, Tribunal
    generation, Tribunal Auditor, protocol L2, L3 ceremony, L4
    verification, L5 execution, scrubbing, rehydration, grading, receipt
    persistence, and commitment append.
    """

    model_config = ConfigDict(extra="forbid")

    schema_version: str = SCHEMA_VERSION
    stage_id: str
    attempt_id: str
    run_id: str
    kind: StageKind
    agent_role: str = ""
    provider: str = ""
    model: str = ""

    monotonic_start: float | None = None
    monotonic_end: float | None = None
    clock_domain: str = ""
    timing_source: str = ""
    cross_process_timing: bool = False

    input_tokens: int | None = None
    output_tokens: int | None = None
    thinking_tokens: int | None = None
    cache_tokens: int | None = None
    usage_estimated: bool = False

    retry_count: int = 0
    finish_reason: str | None = None

    input_artifact_hash: str | None = None
    output_artifact_hash: str | None = None

    decision: str | None = None
    confidence: float | None = None

    source: str = ""
    scrub_count: int = 0
    scrub_types: list[str] = Field(default_factory=list)

    transaction_id: str = ""
    transaction_hash: str = ""
    action_type: str = ""
    operator_id: str = ""
    operator_session_id: str = ""
    requestor_user_id: str = ""
    acting_app_id: str = ""
    case_id: str = ""
    investigation_id: str = ""
    task_id: str = ""
    state_root_before: str = ""
    state_root_after: str = ""
    signer_key_id: str = ""
    receipt_signature_digest: str = ""
    commitment_hash: str = ""
    prior_commitment_hash: str = ""
    l2_signature_digest: str = ""
    l3_signature_digest: str = ""
    audit_record_id: str = ""
    persisted_at_unix_ms: int | None = None
    doctrine_bundle_hash: str = ""
    doctrine_bundle_version: str = ""

    parent_stage_id: str | None = None
    child_stage_ids: list[str] = Field(default_factory=list)


class MetricObservation(BaseModel):
    """One measured metric value for one attempt.

    Every metric declares its ID, version, unit, eligibility, denominator
    contribution, verification status, grader class, and evidence
    references. The metric registry (Phase 4) owns the full definitions;
    this record carries the measured value and its provenance.
    """

    model_config = ConfigDict(extra="forbid")

    schema_version: str = SCHEMA_VERSION
    metric_id: str
    metric_version: str = "1.0.0"
    attempt_id: str
    run_id: str
    arm_id: Arm
    task_id: str

    value: float
    unit: str = ""
    eligible: bool = True
    denominator_contribution: int = 1

    verification_status: VerificationStatus = VerificationStatus.PENDING
    grader_class: GraderClass = GraderClass.DETERMINISTIC

    evidence_refs: list[str] = Field(default_factory=list)


class EvidenceIndex(BaseModel):
    """Index entry for one evidence artifact.

    Raw provider requests and responses are encrypted and access-controlled
    when retained. Public exports contain scrubbed projections and hashes.
    """

    model_config = ConfigDict(extra="forbid")

    schema_version: str = SCHEMA_VERSION
    artifact_id: str
    run_id: str
    attempt_id: str | None = None

    media_type: EvidenceMediaType
    schema_ref: str = Field(default="", description="Schema identifier or URL for the evidence artifact.")
    byte_length: int = 0
    sha256: str

    producer_identity: str = ""
    signature: str | None = None

    privacy_classification: PrivacyClassification = PrivacyClassification.INTERNAL
    storage_location: str = ""

    parent_evidence_refs: list[str] = Field(default_factory=list)
