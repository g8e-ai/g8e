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

from datetime import datetime, UTC
from enum import StrEnum
from typing import Any

from pydantic import BaseModel, ConfigDict, Field, field_serializer, field_validator, model_validator

from g8e.operator.v1.operator_pb2 import ActionReceipt
from g8e.receipts import action_receipt_to_dict, parse_action_receipt
from g8e_evals.arms import Arm, GovernancePosture
from g8e_evals.receipts.verify import receipt_action_type


SCHEMA_VERSION = "1.17.0"


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


class GraderReference(BaseModel):
    """Typed reference to a registered grader by ID, version, and class.

    Replaces the parallel ``grader_ids``/``grader_versions`` string lists.
    Every task declares its graders as typed references so that identity,
    version, and class are bound together and cannot drift out of sync.
    """

    model_config = ConfigDict(extra="forbid")

    grader_id: str = Field(min_length=1)
    grader_version: str = Field(min_length=1)
    grader_class: GraderClass = GraderClass.DETERMINISTIC


class VerificationStatus(StrEnum):
    PENDING = "pending"
    VERIFIED = "verified"
    FAILED = "failed"
    NOT_APPLICABLE = "not_applicable"


class StateAssertionPredicate(StrEnum):
    STATE_ROOT_CHANGED = "state_root_changed"
    STATE_ROOT_UNCHANGED = "state_root_unchanged"


class StateEvidenceKind(StrEnum):
    FILE = "file"
    DOCUMENT = "document"
    WORKLOAD_SIDE_EFFECT = "workload_side_effect"
    LEDGER_CONSISTENCY = "ledger_consistency"


class StateCollectionBoundary(StrEnum):
    OPERATOR_WORKLOAD = "operator_workload"
    GOVERNED_DOCUMENT_STORE = "governed_document_store"
    GOVERNANCE_LEDGER = "governance_ledger"


class RehydrationBoundary(StrEnum):
    LOCAL_RUNTIME = "local_runtime"


class PolicyOutcome(StrEnum):
    ALLOW = "allow"
    BLOCK = "block"


class RejectionLayer(StrEnum):
    L1_DOCTRINE = "l1_doctrine"
    L2_CONSENSUS = "l2_consensus"
    L3_NOTARY = "l3_notary"
    L4_VERIFICATION = "l4_verification"


class EvidenceEncryptionAlgorithm(StrEnum):
    AES_256_GCM = "aes-256-gcm"


class EvidenceAccessPolicy(StrEnum):
    NAMED_KEY_HOLDERS = "named_key_holders"


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
    judge: ModelIdentity | None = None


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


class FinalStateAssertion(BaseModel):
    model_config = ConfigDict(extra="forbid")

    assertion_id: str = Field(min_length=1)
    predicate: StateAssertionPredicate
    action_type: str = Field(min_length=1)


class FinalStateObservation(BaseModel):
    model_config = ConfigDict(extra="forbid")

    schema_version: str = SCHEMA_VERSION
    observation_id: str
    attempt_id: str
    run_id: str
    task_id: str
    assertion_id: str
    action_type: str
    state_root_before: str | None = None
    state_root_after: str | None = None
    source_receipt_id: str | None = None
    verification_status: VerificationStatus = VerificationStatus.PENDING


class StateValue(BaseModel):
    model_config = ConfigDict(extra="forbid")

    kind: StateEvidenceKind
    exists: bool | None = None
    content_sha256: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    byte_length: int | None = Field(default=None, ge=0)
    mode: str | None = None
    version: str | None = None
    consistent: bool | None = None
    entry_count: int | None = Field(default=None, ge=0)
    head_sha256: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")

    @model_validator(mode="after")
    def _validate_kind(self) -> StateValue:
        if self.kind == StateEvidenceKind.LEDGER_CONSISTENCY:
            if self.consistent is None or self.exists is not None:
                raise ValueError("ledger state requires consistency and cannot declare existence")
        elif self.exists is None or self.consistent is not None:
            raise ValueError("file, document, and side-effect state require existence")
        if self.kind != StateEvidenceKind.FILE and self.mode is not None:
            raise ValueError("file mode is valid only for file state")
        if self.kind != StateEvidenceKind.DOCUMENT and self.version is not None:
            raise ValueError("document version is valid only for document state")
        if self.kind != StateEvidenceKind.LEDGER_CONSISTENCY and (
            self.entry_count is not None or self.head_sha256 is not None
        ):
            raise ValueError("ledger fields are valid only for ledger state")
        return self


class StateAssertion(BaseModel):
    model_config = ConfigDict(extra="forbid")

    assertion_id: str = Field(min_length=1)
    action_type: str = Field(min_length=1)
    collection_boundary: StateCollectionBoundary
    target: str = Field(min_length=1)
    expected: StateValue


class StateFixtureDefinition(BaseModel):
    model_config = ConfigDict(extra="forbid")

    fixture_id: str = Field(min_length=1)
    fixture_sha256: str = Field(pattern=r"^[0-9a-f]{64}$")
    assertions: list[StateAssertion] = Field(min_length=1)

    @model_validator(mode="after")
    def _validate_assertions(self) -> StateFixtureDefinition:
        assertion_ids = [assertion.assertion_id for assertion in self.assertions]
        if len(assertion_ids) != len(set(assertion_ids)):
            raise ValueError("state assertion IDs must be unique")
        return self


class StateObservation(BaseModel):
    model_config = ConfigDict(extra="forbid")

    schema_version: str = SCHEMA_VERSION
    observation_id: str = Field(min_length=1)
    attempt_id: str = Field(min_length=1)
    run_id: str = Field(min_length=1)
    task_id: str = Field(min_length=1)
    assertion_id: str = Field(min_length=1)
    action_type: str = Field(min_length=1)
    fixture_sha256: str = Field(pattern=r"^[0-9a-f]{64}$")
    collection_boundary: StateCollectionBoundary
    target: str = Field(min_length=1)
    observed: StateValue
    collected_at: datetime
    source_evidence_refs: list[str] = Field(default_factory=list)
    source_evidence_sha256: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    verification_status: VerificationStatus = VerificationStatus.PENDING

    @model_validator(mode="after")
    def _validate_evidence_binding(self) -> StateObservation:
        if self.verification_status == VerificationStatus.VERIFIED and (
            not self.source_evidence_refs or self.source_evidence_sha256 is None
        ):
            raise ValueError("verified state observation requires source evidence")
        return self


class ModelBoundaryPrivacyAttestation(BaseModel):
    model_config = ConfigDict(extra="forbid")

    scanner_version: str = Field(min_length=1)
    input_artifact_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    raw_sensitive_occurrences: int = Field(ge=0)
    raw_sensitive_types: list[str] = Field(default_factory=list)


class CanaryScrubbingAssertion(BaseModel):
    model_config = ConfigDict(extra="forbid")

    assertion_id: str = Field(min_length=1)
    canary_sha256: str = Field(pattern=r"^[0-9a-f]{64}$")
    source: str = Field(min_length=1)
    input_artifact_sha256: str = Field(pattern=r"^[0-9a-f]{64}$")
    expected_output_artifact_sha256: str = Field(pattern=r"^[0-9a-f]{64}$")
    expected_scrub_type: str = Field(min_length=1)
    expected_occurrences: int = Field(ge=1)


class RehydrationAssertion(BaseModel):
    model_config = ConfigDict(extra="forbid")

    assertion_id: str = Field(min_length=1)
    source: str = Field(min_length=1)
    input_artifact_sha256: str = Field(pattern=r"^[0-9a-f]{64}$")
    expected_output_artifact_sha256: str = Field(pattern=r"^[0-9a-f]{64}$")
    expected_token_count: int = Field(ge=1)
    expected_sensitive_types: list[str] = Field(min_length=1)

    @model_validator(mode="after")
    def _validate_sensitive_types(self) -> RehydrationAssertion:
        if len(self.expected_sensitive_types) != len(set(self.expected_sensitive_types)):
            raise ValueError("expected rehydration sensitive types must be unique")
        if self.expected_token_count < len(self.expected_sensitive_types):
            raise ValueError("expected rehydration tokens cannot be fewer than sensitive types")
        return self


class RehydrationObservation(BaseModel):
    model_config = ConfigDict(extra="forbid")

    schema_version: str = SCHEMA_VERSION
    observation_id: str = Field(min_length=1)
    attempt_id: str = Field(min_length=1)
    run_id: str = Field(min_length=1)
    task_id: str = Field(min_length=1)
    assertion_id: str = Field(min_length=1)
    source: str = Field(min_length=1)
    input_artifact_sha256: str = Field(pattern=r"^[0-9a-f]{64}$")
    output_artifact_sha256: str = Field(pattern=r"^[0-9a-f]{64}$")
    rehydrator_version: str = Field(min_length=1)
    execution_boundary: RehydrationBoundary
    collected_at: datetime
    restored_token_count: int = Field(ge=0)
    unresolved_token_count: int = Field(ge=0)
    restored_sensitive_types: list[str] = Field(default_factory=list)
    unresolved_sensitive_types: list[str] = Field(default_factory=list)
    source_evidence_refs: list[str] = Field(default_factory=list)
    source_evidence_sha256: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    verification_status: VerificationStatus = VerificationStatus.PENDING

    @model_validator(mode="after")
    def _validate_rehydration_evidence(self) -> RehydrationObservation:
        if len(self.restored_sensitive_types) != len(set(self.restored_sensitive_types)):
            raise ValueError("restored sensitive types must be unique")
        if len(self.unresolved_sensitive_types) != len(set(self.unresolved_sensitive_types)):
            raise ValueError("unresolved sensitive types must be unique")
        if bool(self.restored_token_count) != bool(self.restored_sensitive_types):
            raise ValueError("restored token count and sensitive types must agree")
        if bool(self.unresolved_token_count) != bool(self.unresolved_sensitive_types):
            raise ValueError("unresolved token count and sensitive types must agree")
        if len(self.source_evidence_refs) != len(set(self.source_evidence_refs)):
            raise ValueError("rehydration source evidence references must be unique")
        if self.verification_status == VerificationStatus.VERIFIED and (
            not self.source_evidence_refs or self.source_evidence_sha256 is None
        ):
            raise ValueError("verified rehydration observation requires source evidence")
        return self


class SecretDetectionAssertion(BaseModel):
    model_config = ConfigDict(extra="forbid")

    assertion_id: str = Field(min_length=1)
    source: str = Field(min_length=1)
    input_artifact_sha256: str = Field(pattern=r"^[0-9a-f]{64}$")
    expected_sensitive_occurrences: int = Field(ge=1)
    expected_benign_occurrences: int = Field(ge=0)
    expected_sensitive_types: list[str] = Field(min_length=1)

    @model_validator(mode="after")
    def _validate_sensitive_types(self) -> SecretDetectionAssertion:
        if len(self.expected_sensitive_types) != len(set(self.expected_sensitive_types)):
            raise ValueError("expected sensitive types must be unique")
        if self.expected_sensitive_occurrences < len(self.expected_sensitive_types):
            raise ValueError("expected sensitive occurrences cannot be fewer than sensitive types")
        return self


class SecretDetectionObservation(BaseModel):
    model_config = ConfigDict(extra="forbid")

    schema_version: str = SCHEMA_VERSION
    observation_id: str = Field(min_length=1)
    attempt_id: str = Field(min_length=1)
    run_id: str = Field(min_length=1)
    task_id: str = Field(min_length=1)
    assertion_id: str = Field(min_length=1)
    source: str = Field(min_length=1)
    input_artifact_sha256: str = Field(pattern=r"^[0-9a-f]{64}$")
    scanner_version: str = Field(min_length=1)
    collected_at: datetime
    true_positive_count: int = Field(ge=0)
    false_positive_count: int = Field(ge=0)
    false_negative_count: int = Field(ge=0)
    true_negative_count: int = Field(ge=0)
    detected_sensitive_types: list[str] = Field(default_factory=list)
    missed_sensitive_types: list[str] = Field(default_factory=list)
    source_evidence_refs: list[str] = Field(default_factory=list)
    source_evidence_sha256: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    verification_status: VerificationStatus = VerificationStatus.PENDING

    @model_validator(mode="after")
    def _validate_detection_evidence(self) -> SecretDetectionObservation:
        if len(self.detected_sensitive_types) != len(set(self.detected_sensitive_types)):
            raise ValueError("detected sensitive types must be unique")
        if len(self.missed_sensitive_types) != len(set(self.missed_sensitive_types)):
            raise ValueError("missed sensitive types must be unique")
        if bool(self.true_positive_count) != bool(self.detected_sensitive_types):
            raise ValueError("true-positive count and detected sensitive types must agree")
        if bool(self.false_negative_count) != bool(self.missed_sensitive_types):
            raise ValueError("false-negative count and missed sensitive types must agree")
        if self.verification_status == VerificationStatus.VERIFIED and (
            not self.source_evidence_refs or self.source_evidence_sha256 is None
        ):
            raise ValueError("verified secret-detection observation requires source evidence")
        return self


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
    state_fixture: StateFixtureDefinition | None = None
    expected_final_state_assertions: list[FinalStateAssertion] = Field(default_factory=list)

    expected_allow_block_outcome: PolicyOutcome | None = None
    expected_rejection_layer: RejectionLayer | None = None

    sensitive_canary_annotations: list[CanaryScrubbingAssertion] = Field(default_factory=list)
    rehydration_assertions: list[RehydrationAssertion] = Field(default_factory=list)
    secret_detection_assertions: list[SecretDetectionAssertion] = Field(default_factory=list)

    graders: list[GraderReference] = Field(default_factory=list)

    metadata: dict[str, Any] = Field(default_factory=dict)

    @model_validator(mode="after")
    def _validate_expectations(self) -> TaskDefinition:
        assertion_ids = [assertion.assertion_id for assertion in self.expected_final_state_assertions]
        if len(assertion_ids) != len(set(assertion_ids)):
            raise ValueError("final-state assertion IDs must be unique")
        canary_assertion_ids = [
            assertion.assertion_id for assertion in self.sensitive_canary_annotations
        ]
        if len(canary_assertion_ids) != len(set(canary_assertion_ids)):
            raise ValueError("canary assertion IDs must be unique")
        rehydration_assertion_ids = [
            assertion.assertion_id for assertion in self.rehydration_assertions
        ]
        if len(rehydration_assertion_ids) != len(set(rehydration_assertion_ids)):
            raise ValueError("rehydration assertion IDs must be unique")
        secret_detection_assertion_ids = [
            assertion.assertion_id for assertion in self.secret_detection_assertions
        ]
        if len(secret_detection_assertion_ids) != len(set(secret_detection_assertion_ids)):
            raise ValueError("secret-detection assertion IDs must be unique")
        grader_keys = [
            (grader.grader_id, grader.grader_version) for grader in self.graders
        ]
        if len(grader_keys) != len(set(grader_keys)):
            raise ValueError("grader references must be unique")
        if self.state_fixture is not None:
            if self.initial_state_fixture_hash != self.state_fixture.fixture_sha256:
                raise ValueError("state fixture hash does not match the initial-state fixture hash")
        if self.expected_allow_block_outcome is not None and not self.expected_action_class:
            raise ValueError("expected policy outcome requires an expected action class")
        if self.expected_allow_block_outcome == PolicyOutcome.BLOCK and self.expected_rejection_layer is None:
            raise ValueError("blocked policy outcome requires an expected rejection layer")
        if self.expected_allow_block_outcome != PolicyOutcome.BLOCK and self.expected_rejection_layer is not None:
            raise ValueError("expected rejection layer requires a blocked policy outcome")
        return self


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
    reported_cache_tokens: int = 0
    observed_input_tokens: int = 0
    observed_output_tokens: int = 0
    observed_thinking_tokens: int = 0
    observed_cache_tokens: int = 0
    observed_call_count: int = 0
    expected_call_count: int = 0
    missing_provider_usage_call_count: int = 0

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
    def cache_token_delta(self) -> int:
        return self.observed_cache_tokens - self.reported_cache_tokens

    @property
    def exact_reconciliation_possible(self) -> bool:
        return self.missing_provider_usage_call_count == 0

    @property
    def reconciled(self) -> bool:
        return (
            self.exact_reconciliation_possible
            and self.observed_call_count == self.expected_call_count
            and self.input_token_delta == 0
            and self.output_token_delta == 0
            and self.thinking_token_delta == 0
            and self.cache_token_delta == 0
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
    final_state_observation_refs: list[str] = Field(default_factory=list)
    state_observation_refs: list[str] = Field(default_factory=list)
    rehydration_observation_refs: list[str] = Field(default_factory=list)
    secret_detection_observation_refs: list[str] = Field(default_factory=list)
    receipt_refs: list[str] = Field(default_factory=list)
    grade_refs: list[str] = Field(default_factory=list)

    missingness_or_failure: str | None = None
    usage_reconciliation: UsageReconciliation | None = None

    parent_attempt_id: str | None = None


class ReceiptObservation(BaseModel):
    model_config = ConfigDict(arbitrary_types_allowed=True, extra="forbid")

    schema_version: str = SCHEMA_VERSION
    receipt_id: str
    attempt_id: str
    run_id: str
    transaction_id: str
    action_type: str
    primary: bool = False
    verified: bool = False
    action_receipt: ActionReceipt

    @field_validator("action_receipt", mode="before")
    @classmethod
    def _parse_action_receipt(cls, value: object) -> ActionReceipt:
        if isinstance(value, ActionReceipt):
            return value
        if isinstance(value, dict):
            return parse_action_receipt(value)
        raise ValueError("action_receipt must be an ActionReceipt or canonical protojson object")

    @model_validator(mode="after")
    def _validate_receipt_binding(self) -> ReceiptObservation:
        if self.action_receipt.transaction_id != self.transaction_id:
            raise ValueError("receipt transaction ID does not match observation")
        if receipt_action_type(self.action_receipt) != self.action_type:
            raise ValueError("receipt action type does not match observation")
        return self

    @field_serializer("action_receipt")
    def _serialize_action_receipt(self, receipt: ActionReceipt) -> dict[str, object]:
        return action_receipt_to_dict(receipt)


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
    usage_reported: bool = False
    usage_estimated: bool = False

    retry_count: int = 0
    finish_reason: str | None = None

    input_artifact_hash: str | None = None
    output_artifact_hash: str | None = None
    model_boundary_privacy: ModelBoundaryPrivacyAttestation | None = None

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

    value: float | None
    unit: str = ""
    eligible: bool = True
    denominator_contribution: int = 1

    verification_status: VerificationStatus = VerificationStatus.PENDING
    grader_class: GraderClass = GraderClass.DETERMINISTIC

    evidence_refs: list[str] = Field(default_factory=list)


class EvidenceEncryption(BaseModel):
    model_config = ConfigDict(extra="forbid")

    algorithm: EvidenceEncryptionAlgorithm
    key_id: str
    aad_sha256: str
    ciphertext_sha256: str
    ciphertext_byte_length: int


class EvidenceAccessControl(BaseModel):
    model_config = ConfigDict(extra="forbid")

    policy: EvidenceAccessPolicy
    authorization_scope: str


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
    encryption: EvidenceEncryption | None = None
    access_control: EvidenceAccessControl | None = None

    parent_evidence_refs: list[str] = Field(default_factory=list)
