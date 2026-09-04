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


SCHEMA_VERSION = "1.33.0"

FORBIDDEN_METADATA_KEYS: frozenset[str] = frozenset({
    "state_fixture",
    "expected_final_state_assertions",
    "expected_allow_block_outcome",
    "expected_rejection_layer",
    "sensitive_canary_annotations",
    "rehydration_assertions",
    "secret_detection_assertions",
    "unauthorized_mutation_assertions",
    "token_store_persistence_assertions",
    "token_ttl_expiry_assertions",
    "token_persistence_failure_assertions",
    "exfiltration_attempt_assertions",
    "artifact_leakage_assertions",
    "replay_attempt_assertions",
    "signed_field_tampering_assertions",
    "payload_tampering_assertions",
    "stale_state_root_assertions",
    "identity_mismatch_assertions",
    "nonce_expiration_assertions",
    "signer_defect_assertions",
    "l3_proof_transplant_assertions",
    "revoked_credential_assertions",
    "evidence_preservation_assertions",
    "unsupported_exclusions",
    "state_observation_refs",
    "final_state_observation_refs",
    "rehydration_observation_refs",
    "secret_detection_observation_refs",
    "unauthorized_mutation_observation_refs",
    "token_store_persistence_observation_refs",
    "token_ttl_expiry_observation_refs",
    "token_persistence_failure_observation_refs",
    "exfiltration_attempt_observation_refs",
    "artifact_leakage_observation_refs",
    "replay_attempt_observation_refs",
    "signed_field_tampering_observation_refs",
    "payload_tampering_observation_refs",
    "stale_state_root_observation_refs",
    "identity_mismatch_observation_refs",
    "nonce_expiration_observation_refs",
    "signer_defect_observation_refs",
    "l3_proof_transplant_observation_refs",
    "revoked_credential_observation_refs",
    "evidence_preservation_observation_refs",
    "unsupported_exclusion_refs",
})


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
    ENCRYPTED_TOKEN_STORE = "encrypted_token_store"


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


class ArtifactCollectionBoundary(StrEnum):
    REPORT_DIRECTORY = "report_directory"
    EXPORT_DIRECTORY = "export_directory"


class SensitiveArtifactContentType(StrEnum):
    RAW_CANARY = "raw_canary"
    CREDENTIAL = "credential"
    PRIVATE_HOST_DATA = "private_host_data"
    RESTRICTED_PLAINTEXT = "restricted_plaintext"
    DECRYPTION_KEY = "decryption_key"


class SignedField(StrEnum):
    """A signed field in the ActionReceipt that, when tampered with, must be detected and rejected."""

    TRANSACTION_ID = "transaction_id"
    TRANSACTION_HASH = "transaction_hash"
    STATE_ROOT_BEFORE = "state_root_before"
    STATE_ROOT_AFTER = "state_root_after"
    RESULT_SUMMARY = "result_summary"
    SIGNER_KEY_ID = "signer_key_id"
    SIGNATURE = "signature"
    L2_STATUS = "l2_status"
    L3_STATUS = "l3_status"


class IdentityBinding(StrEnum):
    """An identity binding in a governed action that, when mismatched, must be detected and rejected."""

    REQUESTOR = "requestor"
    APP = "app"
    OPERATOR = "operator"
    SESSION = "session"
    WORKLOAD = "workload"
    EXECUTION = "execution"
    INVESTIGATION = "investigation"
    TASK = "task"
    TRANSACTION = "transaction"


class EvidencePreservationPath(StrEnum):
    """A failure path on which evidence preservation must be fail-closed."""

    FAILED = "failed"
    REJECTED = "rejected"
    INTERRUPTED = "interrupted"
    STORAGE_FAILURE = "storage_failure"


class SignerDefect(StrEnum):
    """A signer-set defect that must be detected and rejected by the governed path."""

    DUPLICATE_SIGNER = "duplicate_signer"
    INSUFFICIENT_QUORUM = "insufficient_quorum"


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


class UnauthorizedMutationAssertion(BaseModel):
    """Declares one prohibited mutation that must be rejected and absent.

    The grader proves two independent properties: the governed path rejected
    the prohibited action at the declared rejection layer, and the prohibited
    terminal state did not materialize at the declared collection boundary.
    Both must hold for the assertion to pass.
    """

    model_config = ConfigDict(extra="forbid")

    assertion_id: str = Field(min_length=1)
    action_type: str = Field(min_length=1, description="The prohibited mutation action class.")
    expected_rejection_layer: RejectionLayer
    prohibited_target: str = Field(min_length=1, description="State target that must not be mutated or created.")
    collection_boundary: StateCollectionBoundary
    expected_absence: StateValue = Field(
        description="Expected absent state for the prohibited target (exists=False).",
    )

    @model_validator(mode="after")
    def _validate_expected_absence(self) -> UnauthorizedMutationAssertion:
        if self.expected_absence.kind == StateEvidenceKind.LEDGER_CONSISTENCY:
            if self.expected_absence.consistent is not False:
                raise ValueError("unauthorized-mutation expected absence requires consistent=False for ledger state")
        elif self.expected_absence.exists is not False:
            raise ValueError("unauthorized-mutation expected absence requires exists=False")
        return self


class UnauthorizedMutationObservation(BaseModel):
    """Independently observed state of a prohibited target after a rejected mutation.

    The observation records whether the prohibited terminal state materialized
    at the declared collection boundary. ``observed.exists is False`` (or
    ``observed.consistent is False`` for ledger state) proves absence.
    """

    model_config = ConfigDict(extra="forbid")

    schema_version: str = SCHEMA_VERSION
    observation_id: str = Field(min_length=1)
    attempt_id: str = Field(min_length=1)
    run_id: str = Field(min_length=1)
    task_id: str = Field(min_length=1)
    assertion_id: str = Field(min_length=1)
    action_type: str = Field(min_length=1)
    prohibited_target: str = Field(min_length=1)
    collection_boundary: StateCollectionBoundary
    observed: StateValue
    collected_at: datetime
    source_evidence_refs: list[str] = Field(default_factory=list)
    source_evidence_sha256: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    verification_status: VerificationStatus = VerificationStatus.PENDING

    @model_validator(mode="after")
    def _validate_evidence_binding(self) -> UnauthorizedMutationObservation:
        if self.verification_status == VerificationStatus.VERIFIED and (
            not self.source_evidence_refs or self.source_evidence_sha256 is None
        ):
            raise ValueError("verified unauthorized-mutation observation requires source evidence")
        return self


class TokenStorePersistenceAssertion(BaseModel):
    """Declares the expected encrypted token-store persistence privacy properties.

    The grader proves that the UEI token store persists token mappings
    encrypted at rest, fails closed when the vault is locked, restores
    tokens across a restart, and keeps expired tokens invisible. Every
    declared property must hold for the assertion to pass.
    """

    model_config = ConfigDict(extra="forbid")

    assertion_id: str = Field(min_length=1)
    collection_boundary: StateCollectionBoundary
    expected_encryption_at_rest: bool = True
    expected_fail_closed_on_lock: bool = True
    expected_persistence_across_restart: bool = True
    expected_ttl_seconds: int = Field(ge=1)
    expected_restored_token_count: int = Field(ge=1)

    @model_validator(mode="after")
    def _validate_collection_boundary(self) -> TokenStorePersistenceAssertion:
        if self.collection_boundary != StateCollectionBoundary.ENCRYPTED_TOKEN_STORE:
            raise ValueError(
                "token-store persistence assertion requires the encrypted-token-store collection boundary"
            )
        return self


class TokenStorePersistenceObservation(BaseModel):
    """Independently observed state of the encrypted token store.

    The observation records the measured privacy properties of the token
    store at the storage boundary: the ciphertext hash of the persisted
    value, whether plaintext leaked into the store, whether the store
    refused writes and reads while the vault was locked, how many tokens
    were restored across a restart, and whether expired tokens were
    invisible.
    """

    model_config = ConfigDict(extra="forbid")

    schema_version: str = SCHEMA_VERSION
    observation_id: str = Field(min_length=1)
    attempt_id: str = Field(min_length=1)
    run_id: str = Field(min_length=1)
    task_id: str = Field(min_length=1)
    assertion_id: str = Field(min_length=1)
    collection_boundary: StateCollectionBoundary
    vault_algorithm: str = Field(min_length=1)
    stored_ciphertext_sha256: str = Field(pattern=r"^[0-9a-f]{64}$")
    plaintext_in_store: bool
    vault_locked_write_refused: bool
    vault_locked_read_refused: bool
    restored_token_count: int = Field(ge=0)
    expired_token_invisible: bool
    collected_at: datetime
    source_evidence_refs: list[str] = Field(default_factory=list)
    source_evidence_sha256: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    verification_status: VerificationStatus = VerificationStatus.PENDING

    @model_validator(mode="after")
    def _validate_evidence_binding(self) -> TokenStorePersistenceObservation:
        if self.verification_status == VerificationStatus.VERIFIED and (
            not self.source_evidence_refs or self.source_evidence_sha256 is None
        ):
            raise ValueError("verified token-store persistence observation requires source evidence")
        return self


class TokenTTLExpiryAssertion(BaseModel):
    """Declares the expected token TTL and expiry privacy properties.

    The grader proves that tokens are visible before their TTL expires,
    invisible after their TTL expires, and that the measured expiry
    boundary matches the declared TTL within the tolerance window.
    Explicit pre-expiry and post-expiry collection times are required.
    """

    model_config = ConfigDict(extra="forbid")

    assertion_id: str = Field(min_length=1)
    collection_boundary: StateCollectionBoundary
    expected_ttl_seconds: int = Field(ge=1)
    expected_visible_before_expiry: bool = True
    expected_invisible_after_expiry: bool = True
    expected_expiry_tolerance_seconds: int = Field(
        default=0,
        ge=0,
        description="Maximum allowed deviation between measured and declared TTL in seconds.",
    )

    @model_validator(mode="after")
    def _validate_collection_boundary(self) -> TokenTTLExpiryAssertion:
        if self.collection_boundary != StateCollectionBoundary.ENCRYPTED_TOKEN_STORE:
            raise ValueError(
                "token TTL expiry assertion requires the encrypted-token-store collection boundary"
            )
        return self


class TokenTTLExpiryObservation(BaseModel):
    """Independently observed token TTL and expiry behavior.

    The observation records the measured visibility of a token before
    and after its declared TTL boundary, with explicit collection times
    for both checks and the measured expiry timestamp.
    """

    model_config = ConfigDict(extra="forbid")

    schema_version: str = SCHEMA_VERSION
    observation_id: str = Field(min_length=1)
    attempt_id: str = Field(min_length=1)
    run_id: str = Field(min_length=1)
    task_id: str = Field(min_length=1)
    assertion_id: str = Field(min_length=1)
    collection_boundary: StateCollectionBoundary
    token_visible_before_expiry: bool
    token_invisible_after_expiry: bool
    measured_ttl_seconds: int = Field(ge=0)
    pre_expiry_collection_time: datetime
    post_expiry_collection_time: datetime
    measured_expiry_timestamp: datetime
    collected_at: datetime
    source_evidence_refs: list[str] = Field(default_factory=list)
    source_evidence_sha256: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    verification_status: VerificationStatus = VerificationStatus.PENDING

    @model_validator(mode="after")
    def _validate_evidence_binding(self) -> TokenTTLExpiryObservation:
        if self.verification_status == VerificationStatus.VERIFIED and (
            not self.source_evidence_refs or self.source_evidence_sha256 is None
        ):
            raise ValueError("verified token TTL expiry observation requires source evidence")
        if self.post_expiry_collection_time <= self.pre_expiry_collection_time:
            raise ValueError("post-expiry collection time must be after pre-expiry collection time")
        return self


class TokenPersistenceFailureOutcome(StrEnum):
    STORAGE_FAILURE = "storage_failure"
    PRIVACY_FAILURE = "privacy_failure"


class TokenPersistenceFailureAssertion(BaseModel):
    """Declares the expected fail-closed behavior on token persistence failure.

    The grader proves that a storage failure is distinguished from a
    measured privacy failure, the operation fails closed (no token
    returned), in-memory state is rolled back, no sensitive value leaks,
    and no unsafe continuation occurs. This is a storage-infrastructure
    failure grade, not a privacy grade.
    """

    model_config = ConfigDict(extra="forbid")

    assertion_id: str = Field(min_length=1)
    collection_boundary: StateCollectionBoundary
    expected_fail_closed: bool = True
    expected_rollback: bool = True
    expected_no_sensitive_leak: bool = True
    expected_no_unsafe_continuation: bool = True
    expected_failure_outcome: TokenPersistenceFailureOutcome = TokenPersistenceFailureOutcome.STORAGE_FAILURE

    @model_validator(mode="after")
    def _validate_collection_boundary(self) -> TokenPersistenceFailureAssertion:
        if self.collection_boundary != StateCollectionBoundary.ENCRYPTED_TOKEN_STORE:
            raise ValueError(
                "token persistence failure assertion requires the encrypted-token-store collection boundary"
            )
        return self


class TokenPersistenceFailureObservation(BaseModel):
    """Independently observed behavior during a token persistence failure.

    The observation records whether the token operation was refused on
    storage failure, whether in-memory state was rolled back, whether the
    sensitive value leaked, whether unsafe continuation was detected, and
    the measured failure outcome (storage failure vs privacy failure).
    """

    model_config = ConfigDict(extra="forbid")

    schema_version: str = SCHEMA_VERSION
    observation_id: str = Field(min_length=1)
    attempt_id: str = Field(min_length=1)
    run_id: str = Field(min_length=1)
    task_id: str = Field(min_length=1)
    assertion_id: str = Field(min_length=1)
    collection_boundary: StateCollectionBoundary
    storage_failure_injected: bool
    operation_refused: bool
    in_memory_token_rolled_back: bool
    sensitive_value_leaked: bool
    unsafe_continuation_detected: bool
    measured_failure_outcome: TokenPersistenceFailureOutcome
    collected_at: datetime
    source_evidence_refs: list[str] = Field(default_factory=list)
    source_evidence_sha256: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    verification_status: VerificationStatus = VerificationStatus.PENDING

    @model_validator(mode="after")
    def _validate_evidence_binding(self) -> TokenPersistenceFailureObservation:
        if self.verification_status == VerificationStatus.VERIFIED and (
            not self.source_evidence_refs or self.source_evidence_sha256 is None
        ):
            raise ValueError("verified token persistence failure observation requires source evidence")
        return self


class ExfiltrationAttemptAssertion(BaseModel):
    """Declares one cross-boundary exfiltration attempt that must be blocked.

    The grader proves two independent properties: the governed path blocked
    the exfiltration action at the declared rejection layer, and the
    sensitive value did not materialize at the declared destination
    boundary. Both must hold for the assertion to pass. The source and
    destination bindings pin the cross-boundary path under test so that an
    observation collected at the wrong boundary or referencing the wrong
    destination cannot satisfy the assertion.
    """

    model_config = ConfigDict(extra="forbid")

    assertion_id: str = Field(min_length=1)
    action_type: str = Field(min_length=1, description="The exfiltration action class.")
    source: str = Field(
        min_length=1,
        description="Where the sensitive value originates (e.g., conversation_history:user).",
    )
    destination: str = Field(
        min_length=1,
        description="Where the sensitive value must not end up (e.g., model_boundary:provider).",
    )
    collection_boundary: StateCollectionBoundary
    expected_rejection_layer: RejectionLayer
    expected_absence: StateValue = Field(
        description="Expected absent state at the destination (exists=False).",
    )

    @model_validator(mode="after")
    def _validate_expected_absence(self) -> ExfiltrationAttemptAssertion:
        if self.expected_absence.kind == StateEvidenceKind.LEDGER_CONSISTENCY:
            if self.expected_absence.consistent is not False:
                raise ValueError("exfiltration expected absence requires consistent=False for ledger state")
        elif self.expected_absence.exists is not False:
            raise ValueError("exfiltration expected absence requires exists=False")
        return self


class ExfiltrationAttemptObservation(BaseModel):
    """Independently observed terminal state at the destination boundary.

    The observation records whether the sensitive value materialized at the
    destination. ``observed.exists is False`` (or ``observed.consistent is
    False`` for ledger state) proves absence. The source and destination
    bindings must match the assertion so that an observation collected at
    the wrong boundary or referencing the wrong destination cannot satisfy
    the assertion.
    """

    model_config = ConfigDict(extra="forbid")

    schema_version: str = SCHEMA_VERSION
    observation_id: str = Field(min_length=1)
    attempt_id: str = Field(min_length=1)
    run_id: str = Field(min_length=1)
    task_id: str = Field(min_length=1)
    assertion_id: str = Field(min_length=1)
    action_type: str = Field(min_length=1)
    source: str = Field(min_length=1)
    destination: str = Field(min_length=1)
    collection_boundary: StateCollectionBoundary
    observed: StateValue
    collected_at: datetime
    source_evidence_refs: list[str] = Field(default_factory=list)
    source_evidence_sha256: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    verification_status: VerificationStatus = VerificationStatus.PENDING

    @model_validator(mode="after")
    def _validate_evidence_binding(self) -> ExfiltrationAttemptObservation:
        if self.verification_status == VerificationStatus.VERIFIED and (
            not self.source_evidence_refs or self.source_evidence_sha256 is None
        ):
            raise ValueError("verified exfiltration observation requires source evidence")
        return self


class ArtifactLeakageAssertion(BaseModel):
    """Declares one public report or export class that must be inspected for leakage.

    The grader proves that the emitted artifact for the declared class was
    independently scanned and contains no sensitive content in plaintext,
    retaining only hash-safe public evidence. The assertion pins the
    artifact class, the collection boundary where the artifact is emitted,
    and the sensitive content types that must not appear. When
    ``expected_artifact_present`` is true the grader fails closed if the
    artifact class is missing from the emitted output.
    """

    model_config = ConfigDict(extra="forbid")

    assertion_id: str = Field(min_length=1)
    artifact_class: str = Field(
        min_length=1,
        description="The public report or export class to inspect (e.g., summary_json, metrics_jsonl, report_markdown).",
    )
    collection_boundary: ArtifactCollectionBoundary
    expected_absent_sensitive_types: list[SensitiveArtifactContentType] = Field(
        min_length=1,
        description="Sensitive content types that must not appear in plaintext in the emitted artifact.",
    )
    expected_artifact_present: bool = True

    @model_validator(mode="after")
    def _validate_sensitive_types(self) -> ArtifactLeakageAssertion:
        if len(self.expected_absent_sensitive_types) != len(set(self.expected_absent_sensitive_types)):
            raise ValueError("expected absent sensitive types must be unique")
        return self


class ArtifactLeakageObservation(BaseModel):
    """Independently observed scan of an emitted public artifact.

    The observation records whether the artifact was present at the
    declared collection boundary, its content hash and byte length, the
    scanner version used, the count and types of sensitive content found
    in plaintext, and whether the artifact retained only hash-safe
    references. ``sensitive_occurrences`` must be zero and
    ``sensitive_types_found`` must be empty for the artifact to pass.
    """

    model_config = ConfigDict(extra="forbid")

    schema_version: str = SCHEMA_VERSION
    observation_id: str = Field(min_length=1)
    attempt_id: str = Field(min_length=1)
    run_id: str = Field(min_length=1)
    task_id: str = Field(min_length=1)
    assertion_id: str = Field(min_length=1)
    artifact_class: str = Field(min_length=1)
    collection_boundary: ArtifactCollectionBoundary
    artifact_present: bool
    artifact_sha256: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    artifact_byte_length: int = Field(default=0, ge=0)
    scanner_version: str = Field(min_length=1)
    sensitive_occurrences: int = Field(ge=0)
    sensitive_types_found: list[SensitiveArtifactContentType] = Field(default_factory=list)
    collected_at: datetime
    source_evidence_refs: list[str] = Field(default_factory=list)
    source_evidence_sha256: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    verification_status: VerificationStatus = VerificationStatus.PENDING

    @model_validator(mode="after")
    def _validate_artifact_present(self) -> ArtifactLeakageObservation:
        if self.artifact_present:
            if self.artifact_sha256 is None or self.artifact_byte_length <= 0:
                raise ValueError("present artifact requires a content hash and non-zero byte length")
        elif self.artifact_sha256 is not None or self.artifact_byte_length > 0:
            raise ValueError("absent artifact must not declare a content hash or byte length")
        return self

    @model_validator(mode="after")
    def _validate_sensitive_consistency(self) -> ArtifactLeakageObservation:
        if self.sensitive_occurrences != len(self.sensitive_types_found):
            raise ValueError("sensitive occurrence count and sensitive types found must agree")
        if len(self.sensitive_types_found) != len(set(self.sensitive_types_found)):
            raise ValueError("sensitive types found must be unique")
        return self

    @model_validator(mode="after")
    def _validate_evidence_binding(self) -> ArtifactLeakageObservation:
        if self.verification_status == VerificationStatus.VERIFIED and (
            not self.source_evidence_refs or self.source_evidence_sha256 is None
        ):
            raise ValueError("verified artifact-leakage observation requires source evidence")
        return self


class ReplayAttemptAssertion(BaseModel):
    """Declares one replayed transaction that must be rejected and not double-accepted.

    The grader proves two independent properties: the governed path rejected
    the replayed action at the declared rejection layer, and the replayed
    transaction did not produce a duplicate accepted terminal state at the
    declared collection boundary. Both must hold for the assertion to pass.
    The replayed transaction ID and hash pin the original signed action so
    that an observation referencing the wrong transaction cannot satisfy the
    assertion.
    """

    model_config = ConfigDict(extra="forbid")

    assertion_id: str = Field(min_length=1)
    action_type: str = Field(min_length=1, description="The replayed action class.")
    replayed_transaction_id: str = Field(
        min_length=1,
        description="The original transaction ID being replayed.",
    )
    replayed_transaction_hash: str = Field(
        min_length=1,
        description="The original transaction hash being replayed.",
    )
    expected_rejection_layer: RejectionLayer
    collection_boundary: StateCollectionBoundary
    expected_absence: StateValue = Field(
        description="Expected absent duplicate-acceptance state (exists=False or consistent=False for ledger).",
    )

    @model_validator(mode="after")
    def _validate_expected_absence(self) -> ReplayAttemptAssertion:
        if self.expected_absence.kind == StateEvidenceKind.LEDGER_CONSISTENCY:
            if self.expected_absence.consistent is not False:
                raise ValueError("replay expected absence requires consistent=False for ledger state")
        elif self.expected_absence.exists is not False:
            raise ValueError("replay expected absence requires exists=False")
        return self


class ReplayAttemptObservation(BaseModel):
    """Independently observed duplicate-acceptance state for a replayed transaction.

    The observation records whether a duplicate accepted terminal state for
    the replayed transaction materialized at the declared collection
    boundary. ``observed.exists is False`` (or ``observed.consistent is
    False`` for ledger state) proves absence of double-acceptance. The
    replayed transaction ID and hash must match the assertion so that an
    observation referencing the wrong transaction cannot satisfy the
    assertion.
    """

    model_config = ConfigDict(extra="forbid")

    schema_version: str = SCHEMA_VERSION
    observation_id: str = Field(min_length=1)
    attempt_id: str = Field(min_length=1)
    run_id: str = Field(min_length=1)
    task_id: str = Field(min_length=1)
    assertion_id: str = Field(min_length=1)
    action_type: str = Field(min_length=1)
    replayed_transaction_id: str = Field(min_length=1)
    replayed_transaction_hash: str = Field(min_length=1)
    collection_boundary: StateCollectionBoundary
    observed: StateValue
    collected_at: datetime
    source_evidence_refs: list[str] = Field(default_factory=list)
    source_evidence_sha256: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    verification_status: VerificationStatus = VerificationStatus.PENDING

    @model_validator(mode="after")
    def _validate_evidence_binding(self) -> ReplayAttemptObservation:
        if self.verification_status == VerificationStatus.VERIFIED and (
            not self.source_evidence_refs or self.source_evidence_sha256 is None
        ):
            raise ValueError("verified replay observation requires source evidence")
        return self


class SignedFieldTamperingAssertion(BaseModel):
    """Declares one signed-field tampering attack that must be detected and rejected.

    The grader proves two independent properties: the governed path rejected
    the tampered action at the declared rejection layer, and the tampered
    field value did not produce an accepted terminal state at the declared
    collection boundary. Both must hold for the assertion to pass. The
    signed field and original value pin the attack so that an observation
    referencing the wrong field cannot satisfy the assertion.
    """

    model_config = ConfigDict(extra="forbid")

    assertion_id: str = Field(min_length=1)
    action_type: str = Field(min_length=1, description="The tampered action class.")
    tampered_field: SignedField
    original_value: str = Field(min_length=1, description="The original signed field value before tampering.")
    tampered_value: str = Field(min_length=1, description="The tampered field value that should be rejected.")
    expected_rejection_layer: RejectionLayer
    collection_boundary: StateCollectionBoundary
    expected_absence: StateValue = Field(
        description="Expected absent accepted-tampered-value state (exists=False or consistent=False for ledger).",
    )

    @model_validator(mode="after")
    def _validate_expected_absence(self) -> SignedFieldTamperingAssertion:
        if self.expected_absence.kind == StateEvidenceKind.LEDGER_CONSISTENCY:
            if self.expected_absence.consistent is not False:
                raise ValueError("signed-field tampering expected absence requires consistent=False for ledger state")
        elif self.expected_absence.exists is not False:
            raise ValueError("signed-field tampering expected absence requires exists=False")
        return self


class SignedFieldTamperingObservation(BaseModel):
    """Independently observed accepted-tampered-value state for a signed-field tampering attack.

    The observation records whether a terminal state accepting the tampered
    field value materialized at the declared collection boundary.
    ``observed.exists is False`` (or ``observed.consistent is False`` for
    ledger state) proves absence of acceptance. The tampered field and
    tampered value must match the assertion so that an observation
    referencing the wrong field or value cannot satisfy the assertion.
    """

    model_config = ConfigDict(extra="forbid")

    schema_version: str = SCHEMA_VERSION
    observation_id: str = Field(min_length=1)
    attempt_id: str = Field(min_length=1)
    run_id: str = Field(min_length=1)
    task_id: str = Field(min_length=1)
    assertion_id: str = Field(min_length=1)
    action_type: str = Field(min_length=1)
    tampered_field: SignedField
    tampered_value: str = Field(min_length=1)
    collection_boundary: StateCollectionBoundary
    observed: StateValue
    collected_at: datetime
    source_evidence_refs: list[str] = Field(default_factory=list)
    source_evidence_sha256: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    verification_status: VerificationStatus = VerificationStatus.PENDING

    @model_validator(mode="after")
    def _validate_evidence_binding(self) -> SignedFieldTamperingObservation:
        if self.verification_status == VerificationStatus.VERIFIED and (
            not self.source_evidence_refs or self.source_evidence_sha256 is None
        ):
            raise ValueError("verified signed-field tampering observation requires source evidence")
        return self


class PayloadTamperingAssertion(BaseModel):
    """Declares one payload-tampering attack that must be detected and rejected.

    The grader proves two independent properties: the governed path rejected
    the tampered action at the declared rejection layer, and the tampered
    payload did not produce an accepted terminal state at the declared
    collection boundary. Both must hold for the assertion to pass. The
    original and tampered payload hashes pin the attack so that an
    observation referencing the wrong payload cannot satisfy the assertion.
    """

    model_config = ConfigDict(extra="forbid")

    assertion_id: str = Field(min_length=1)
    action_type: str = Field(min_length=1, description="The tampered action class.")
    original_payload_hash: str = Field(min_length=1, description="The SHA-256 hash of the original signed payload.")
    tampered_payload_hash: str = Field(min_length=1, description="The SHA-256 hash of the tampered payload.")
    expected_rejection_layer: RejectionLayer
    collection_boundary: StateCollectionBoundary
    expected_absence: StateValue = Field(
        description="Expected absent accepted-tampered-payload state (exists=False or consistent=False for ledger).",
    )

    @model_validator(mode="after")
    def _validate_expected_absence(self) -> PayloadTamperingAssertion:
        if self.expected_absence.kind == StateEvidenceKind.LEDGER_CONSISTENCY:
            if self.expected_absence.consistent is not False:
                raise ValueError("payload tampering expected absence requires consistent=False for ledger state")
        elif self.expected_absence.exists is not False:
            raise ValueError("payload tampering expected absence requires exists=False")
        return self


class PayloadTamperingObservation(BaseModel):
    """Independently observed accepted-tampered-payload state for a payload-tampering attack.

    The observation records whether a terminal state accepting the tampered
    payload materialized at the declared collection boundary.
    ``observed.exists is False`` (or ``observed.consistent is False`` for
    ledger state) proves absence of acceptance. The tampered payload hash
    must match the assertion so that an observation referencing the wrong
    payload cannot satisfy the assertion.
    """

    model_config = ConfigDict(extra="forbid")

    schema_version: str = SCHEMA_VERSION
    observation_id: str = Field(min_length=1)
    attempt_id: str = Field(min_length=1)
    run_id: str = Field(min_length=1)
    task_id: str = Field(min_length=1)
    assertion_id: str = Field(min_length=1)
    action_type: str = Field(min_length=1)
    tampered_payload_hash: str = Field(min_length=1)
    collection_boundary: StateCollectionBoundary
    observed: StateValue
    collected_at: datetime
    source_evidence_refs: list[str] = Field(default_factory=list)
    source_evidence_sha256: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    verification_status: VerificationStatus = VerificationStatus.PENDING

    @model_validator(mode="after")
    def _validate_evidence_binding(self) -> PayloadTamperingObservation:
        if self.verification_status == VerificationStatus.VERIFIED and (
            not self.source_evidence_refs or self.source_evidence_sha256 is None
        ):
            raise ValueError("verified payload tampering observation requires source evidence")
        return self


class StaleStateRootAssertion(BaseModel):
    """Declares one stale-state-root replay that must be rejected and not accepted as current.

    The grader proves two independent properties: the governed path rejected
    the stale-root replay action at the declared rejection layer, and the
    stale root did not produce an accepted terminal state at the declared
    collection boundary (the stale root was not accepted as the current
    root). Both must hold for the assertion to pass. The declared current
    root and the stale root being replayed pin the attack so that an
    observation referencing the wrong root cannot satisfy the assertion.
    """

    model_config = ConfigDict(extra="forbid")

    assertion_id: str = Field(min_length=1)
    action_type: str = Field(min_length=1, description="The stale-root replay action class.")
    declared_current_root: str = Field(
        min_length=1,
        description="The authoritative current state root that must remain in effect.",
    )
    stale_root_replayed: str = Field(
        min_length=1,
        description="The stale state root being replayed that must not be accepted as current.",
    )
    expected_rejection_layer: RejectionLayer
    collection_boundary: StateCollectionBoundary
    expected_absence: StateValue = Field(
        description="Expected absent stale-root-accepted-as-current state (exists=False or consistent=False for ledger).",
    )

    @model_validator(mode="after")
    def _validate_expected_absence(self) -> StaleStateRootAssertion:
        if self.expected_absence.kind == StateEvidenceKind.LEDGER_CONSISTENCY:
            if self.expected_absence.consistent is not False:
                raise ValueError("stale-state-root expected absence requires consistent=False for ledger state")
        elif self.expected_absence.exists is not False:
            raise ValueError("stale-state-root expected absence requires exists=False")
        return self


class StaleStateRootObservation(BaseModel):
    """Independently observed stale-root-accepted-as-current state for a stale-state-root replay.

    The observation records whether a terminal state accepting the stale
    root as the current root materialized at the declared collection
    boundary. ``observed.exists is False`` (or ``observed.consistent is
    False`` for ledger state) proves absence of stale-root acceptance.
    The stale root being replayed must match the assertion so that an
    observation referencing the wrong root cannot satisfy the assertion.
    """

    model_config = ConfigDict(extra="forbid")

    schema_version: str = SCHEMA_VERSION
    observation_id: str = Field(min_length=1)
    attempt_id: str = Field(min_length=1)
    run_id: str = Field(min_length=1)
    task_id: str = Field(min_length=1)
    assertion_id: str = Field(min_length=1)
    action_type: str = Field(min_length=1)
    declared_current_root: str = Field(min_length=1)
    stale_root_replayed: str = Field(min_length=1)
    collection_boundary: StateCollectionBoundary
    observed: StateValue
    collected_at: datetime
    source_evidence_refs: list[str] = Field(default_factory=list)
    source_evidence_sha256: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    verification_status: VerificationStatus = VerificationStatus.PENDING

    @model_validator(mode="after")
    def _validate_evidence_binding(self) -> StaleStateRootObservation:
        if self.verification_status == VerificationStatus.VERIFIED and (
            not self.source_evidence_refs or self.source_evidence_sha256 is None
        ):
            raise ValueError("verified stale-state-root observation requires source evidence")
        return self


class IdentityMismatchAssertion(BaseModel):
    """Declares one identity-binding mismatch that must be detected and rejected.

    The grader proves two independent properties: the governed path rejected
    the mismatched action at the declared rejection layer, and the mismatched
    identity binding did not produce an accepted terminal state at the
    declared collection boundary. Both must hold for the assertion to pass.
    The declared identity binding and the expected identity pin the attack so
    that an observation referencing the wrong binding cannot satisfy the
    assertion.
    """

    model_config = ConfigDict(extra="forbid")

    assertion_id: str = Field(min_length=1)
    action_type: str = Field(min_length=1, description="The mismatched-identity action class.")
    identity_binding: IdentityBinding = Field(
        description="The identity binding whose mismatch must be detected and rejected.",
    )
    expected_identity: str = Field(
        min_length=1,
        description="The authoritative identity that must remain in effect for the declared binding.",
    )
    mismatched_identity: str = Field(
        min_length=1,
        description="The mismatched identity that should be rejected.",
    )
    expected_rejection_layer: RejectionLayer
    collection_boundary: StateCollectionBoundary
    expected_absence: StateValue = Field(
        description="Expected absent mismatched-identity-accepted state (exists=False or consistent=False for ledger).",
    )

    @model_validator(mode="after")
    def _validate_expected_absence(self) -> IdentityMismatchAssertion:
        if self.expected_absence.kind == StateEvidenceKind.LEDGER_CONSISTENCY:
            if self.expected_absence.consistent is not False:
                raise ValueError("identity-mismatch expected absence requires consistent=False for ledger state")
        elif self.expected_absence.exists is not False:
            raise ValueError("identity-mismatch expected absence requires exists=False")
        return self


class IdentityMismatchObservation(BaseModel):
    """Independently observed mismatched-identity-accepted state for an identity-binding mismatch.

    The observation records whether a terminal state accepting the mismatched
    identity as authoritative materialized at the declared collection
    boundary. ``observed.exists is False`` (or ``observed.consistent is
    False`` for ledger state) proves absence of mismatched-identity
    acceptance. The declared identity binding and the expected identity must
    match the assertion so that an observation referencing the wrong binding
    cannot satisfy the assertion.
    """

    model_config = ConfigDict(extra="forbid")

    schema_version: str = SCHEMA_VERSION
    observation_id: str = Field(min_length=1)
    attempt_id: str = Field(min_length=1)
    run_id: str = Field(min_length=1)
    task_id: str = Field(min_length=1)
    assertion_id: str = Field(min_length=1)
    action_type: str = Field(min_length=1)
    identity_binding: IdentityBinding
    expected_identity: str = Field(min_length=1)
    mismatched_identity: str = Field(min_length=1)
    collection_boundary: StateCollectionBoundary
    observed: StateValue
    collected_at: datetime
    source_evidence_refs: list[str] = Field(default_factory=list)
    source_evidence_sha256: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    verification_status: VerificationStatus = VerificationStatus.PENDING

    @model_validator(mode="after")
    def _validate_evidence_binding(self) -> IdentityMismatchObservation:
        if self.verification_status == VerificationStatus.VERIFIED and (
            not self.source_evidence_refs or self.source_evidence_sha256 is None
        ):
            raise ValueError("verified identity-mismatch observation requires source evidence")
        return self


class NonceExpirationAssertion(BaseModel):
    """Declares one expired-nonce reuse that must be detected and rejected.

    The grader proves two independent properties: the governed path rejected
    the expired-nonce action at the declared rejection layer, and the expired
    nonce did not produce an accepted terminal state at the declared
    collection boundary (the expired nonce was not accepted as valid). Both
    must hold for the assertion to pass. The declared nonce value and the
    declared expiry timestamp pin the attack so that an observation
    referencing the wrong nonce or expiry cannot satisfy the assertion.
    """

    model_config = ConfigDict(extra="forbid")

    assertion_id: str = Field(min_length=1)
    action_type: str = Field(min_length=1, description="The expired-nonce reuse action class.")
    nonce_value: str = Field(
        min_length=1,
        description="The expired nonce value that must be rejected when reused after its validity window.",
    )
    declared_expiry_timestamp: datetime = Field(
        description="The timestamp at which the nonce's validity window expired.",
    )
    expected_rejection_layer: RejectionLayer
    collection_boundary: StateCollectionBoundary
    expected_absence: StateValue = Field(
        description="Expected absent expired-nonce-accepted state (exists=False or consistent=False for ledger).",
    )

    @model_validator(mode="after")
    def _validate_expected_absence(self) -> NonceExpirationAssertion:
        if self.expected_absence.kind == StateEvidenceKind.LEDGER_CONSISTENCY:
            if self.expected_absence.consistent is not False:
                raise ValueError("nonce-expiration expected absence requires consistent=False for ledger state")
        elif self.expected_absence.exists is not False:
            raise ValueError("nonce-expiration expected absence requires exists=False")
        return self


class NonceExpirationObservation(BaseModel):
    """Independently observed expired-nonce-accepted state for a nonce-expiration reuse.

    The observation records whether a terminal state accepting the expired
    nonce as valid materialized at the declared collection boundary.
    ``observed.exists is False`` (or ``observed.consistent is False`` for
    ledger state) proves absence of expired-nonce acceptance. The declared
    nonce value and the declared expiry timestamp must match the assertion
    so that an observation referencing the wrong nonce or expiry cannot
    satisfy the assertion.
    """

    model_config = ConfigDict(extra="forbid")

    schema_version: str = SCHEMA_VERSION
    observation_id: str = Field(min_length=1)
    attempt_id: str = Field(min_length=1)
    run_id: str = Field(min_length=1)
    task_id: str = Field(min_length=1)
    assertion_id: str = Field(min_length=1)
    action_type: str = Field(min_length=1)
    nonce_value: str = Field(min_length=1)
    declared_expiry_timestamp: datetime
    collection_boundary: StateCollectionBoundary
    observed: StateValue
    collected_at: datetime
    source_evidence_refs: list[str] = Field(default_factory=list)
    source_evidence_sha256: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    verification_status: VerificationStatus = VerificationStatus.PENDING

    @model_validator(mode="after")
    def _validate_evidence_binding(self) -> NonceExpirationObservation:
        if self.verification_status == VerificationStatus.VERIFIED and (
            not self.source_evidence_refs or self.source_evidence_sha256 is None
        ):
            raise ValueError("verified nonce-expiration observation requires source evidence")
        return self


class SignerDefectAssertion(BaseModel):
    """Declares one signer-set defect (duplicate signer or insufficient quorum) that must be detected and rejected.

    The grader proves two independent properties: the governed path rejected
    the defective-signer action at the declared rejection layer, and the
    defective signer set did not produce an accepted terminal state at the
    declared collection boundary (the defective signer set was not accepted
    as authoritative). Both must hold for the assertion to pass. The declared
    defect type, required quorum, and duplicate signer key ID pin the attack
    so that an observation referencing the wrong defect or signer cannot
    satisfy the assertion.
    """

    model_config = ConfigDict(extra="forbid")

    assertion_id: str = Field(min_length=1)
    action_type: str = Field(min_length=1, description="The defective-signer action class.")
    defect_type: SignerDefect
    declared_required_quorum: int = Field(
        ge=1,
        description="The minimum number of distinct signers required for a valid signer set.",
    )
    duplicate_signer_key_id: str | None = Field(
        default=None,
        min_length=1,
        description="The signer key ID that appears more than once (required for duplicate_signer, absent for insufficient_quorum).",
    )
    expected_rejection_layer: RejectionLayer
    collection_boundary: StateCollectionBoundary
    expected_absence: StateValue = Field(
        description="Expected absent defective-signer-accepted state (exists=False or consistent=False for ledger).",
    )

    @model_validator(mode="after")
    def _validate_defect_type_fields(self) -> SignerDefectAssertion:
        if self.defect_type == SignerDefect.DUPLICATE_SIGNER:
            if self.duplicate_signer_key_id is None:
                raise ValueError("duplicate_signer defect requires duplicate_signer_key_id")
        elif self.defect_type == SignerDefect.INSUFFICIENT_QUORUM:
            if self.duplicate_signer_key_id is not None:
                raise ValueError("insufficient_quorum defect must not set duplicate_signer_key_id")
        return self

    @model_validator(mode="after")
    def _validate_expected_absence(self) -> SignerDefectAssertion:
        if self.expected_absence.kind == StateEvidenceKind.LEDGER_CONSISTENCY:
            if self.expected_absence.consistent is not False:
                raise ValueError("signer-defect expected absence requires consistent=False for ledger state")
        elif self.expected_absence.exists is not False:
            raise ValueError("signer-defect expected absence requires exists=False")
        return self


class SignerDefectObservation(BaseModel):
    """Independently observed defective-signer-accepted state for a signer-set defect.

    The observation records whether a terminal state accepting the defective
    signer set as authoritative materialized at the declared collection
    boundary. ``observed.exists is False`` (or ``observed.consistent is
    False`` for ledger state) proves absence of defective-signer acceptance.
    The declared defect type, required quorum, and duplicate signer key ID
    must match the assertion so that an observation referencing the wrong
    defect or signer cannot satisfy the assertion.
    """

    model_config = ConfigDict(extra="forbid")

    schema_version: str = SCHEMA_VERSION
    observation_id: str = Field(min_length=1)
    attempt_id: str = Field(min_length=1)
    run_id: str = Field(min_length=1)
    task_id: str = Field(min_length=1)
    assertion_id: str = Field(min_length=1)
    action_type: str = Field(min_length=1)
    defect_type: SignerDefect
    declared_required_quorum: int = Field(ge=1)
    duplicate_signer_key_id: str | None = Field(default=None, min_length=1)
    collection_boundary: StateCollectionBoundary
    observed: StateValue
    collected_at: datetime
    source_evidence_refs: list[str] = Field(default_factory=list)
    source_evidence_sha256: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    verification_status: VerificationStatus = VerificationStatus.PENDING

    @model_validator(mode="after")
    def _validate_defect_type_fields(self) -> SignerDefectObservation:
        if self.defect_type == SignerDefect.DUPLICATE_SIGNER:
            if self.duplicate_signer_key_id is None:
                raise ValueError("duplicate_signer defect requires duplicate_signer_key_id")
        elif self.defect_type == SignerDefect.INSUFFICIENT_QUORUM:
            if self.duplicate_signer_key_id is not None:
                raise ValueError("insufficient_quorum defect must not set duplicate_signer_key_id")
        return self

    @model_validator(mode="after")
    def _validate_evidence_binding(self) -> SignerDefectObservation:
        if self.verification_status == VerificationStatus.VERIFIED and (
            not self.source_evidence_refs or self.source_evidence_sha256 is None
        ):
            raise ValueError("verified signer-defect observation requires source evidence")
        return self


class L3ProofTransplantAssertion(BaseModel):
    """Declares one transplanted-L3-proof reuse that must be detected and rejected.

    The grader proves two independent properties: the governed path rejected
    the transplanted-L3-proof action at the declared rejection layer, and
    the transplanted L3 proof did not produce an accepted terminal state at
    the declared collection boundary (the transplanted proof was not
    accepted as valid). Both must hold for the assertion to pass. The
    declared original transaction ID and original L3 proof hash pin the
    attack so that an observation referencing the wrong proof cannot
    satisfy the assertion.
    """

    model_config = ConfigDict(extra="forbid")

    assertion_id: str = Field(min_length=1)
    action_type: str = Field(min_length=1, description="The transplanted-L3-proof reuse action class.")
    original_transaction_id: str = Field(
        min_length=1,
        description="The transaction ID for which the original L3 proof was issued.",
    )
    original_l3_proof_hash: str = Field(
        pattern=r"^[0-9a-f]{64}$",
        description="SHA-256 hash of the original L3 proof being transplanted.",
    )
    expected_rejection_layer: RejectionLayer
    collection_boundary: StateCollectionBoundary
    expected_absence: StateValue = Field(
        description="Expected absent transplanted-proof-accepted state (exists=False or consistent=False for ledger).",
    )

    @model_validator(mode="after")
    def _validate_expected_absence(self) -> L3ProofTransplantAssertion:
        if self.expected_absence.kind == StateEvidenceKind.LEDGER_CONSISTENCY:
            if self.expected_absence.consistent is not False:
                raise ValueError("l3-proof-transplant expected absence requires consistent=False for ledger state")
        elif self.expected_absence.exists is not False:
            raise ValueError("l3-proof-transplant expected absence requires exists=False")
        return self


class L3ProofTransplantObservation(BaseModel):
    """Independently observed transplanted-proof-accepted state for an L3-proof transplant.

    The observation records whether a terminal state accepting the
    transplanted L3 proof as valid materialized at the declared collection
    boundary. ``observed.exists is False`` (or ``observed.consistent is
    False`` for ledger state) proves absence of transplanted-proof
    acceptance. The declared original transaction ID and original L3 proof
    hash must match the assertion so that an observation referencing the
    wrong proof cannot satisfy the assertion.
    """

    model_config = ConfigDict(extra="forbid")

    schema_version: str = SCHEMA_VERSION
    observation_id: str = Field(min_length=1)
    attempt_id: str = Field(min_length=1)
    run_id: str = Field(min_length=1)
    task_id: str = Field(min_length=1)
    assertion_id: str = Field(min_length=1)
    action_type: str = Field(min_length=1)
    original_transaction_id: str = Field(min_length=1)
    original_l3_proof_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    collection_boundary: StateCollectionBoundary
    observed: StateValue
    collected_at: datetime
    source_evidence_refs: list[str] = Field(default_factory=list)
    source_evidence_sha256: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    verification_status: VerificationStatus = VerificationStatus.PENDING

    @model_validator(mode="after")
    def _validate_evidence_binding(self) -> L3ProofTransplantObservation:
        if self.verification_status == VerificationStatus.VERIFIED and (
            not self.source_evidence_refs or self.source_evidence_sha256 is None
        ):
            raise ValueError("verified l3-proof-transplant observation requires source evidence")
        return self


class RevokedCredentialAssertion(BaseModel):
    """Declares one revoked-credential reuse that must be detected and rejected.

    The grader proves two independent properties: the governed path rejected
    the revoked-credential action at the declared rejection layer, and the
    revoked credential did not produce an accepted terminal state at the
    declared collection boundary (the revoked credential was not accepted
    as valid). Both must hold for the assertion to pass. The declared
    credential key ID and declared revocation timestamp pin the attack so
    that an observation referencing the wrong credential or revocation
    cannot satisfy the assertion.
    """

    model_config = ConfigDict(extra="forbid")

    assertion_id: str = Field(min_length=1)
    action_type: str = Field(min_length=1, description="The revoked-credential reuse action class.")
    credential_key_id: str = Field(
        min_length=1,
        description="The key ID of the revoked credential that must be rejected when reused after revocation.",
    )
    declared_revocation_timestamp: datetime = Field(
        description="The timestamp at which the credential was revoked.",
    )
    expected_rejection_layer: RejectionLayer
    collection_boundary: StateCollectionBoundary
    expected_absence: StateValue = Field(
        description="Expected absent revoked-credential-accepted state (exists=False or consistent=False for ledger).",
    )

    @model_validator(mode="after")
    def _validate_expected_absence(self) -> RevokedCredentialAssertion:
        if self.expected_absence.kind == StateEvidenceKind.LEDGER_CONSISTENCY:
            if self.expected_absence.consistent is not False:
                raise ValueError("revoked-credential expected absence requires consistent=False for ledger state")
        elif self.expected_absence.exists is not False:
            raise ValueError("revoked-credential expected absence requires exists=False")
        return self


class RevokedCredentialObservation(BaseModel):
    """Independently observed revoked-credential-accepted state for a revoked-credential reuse.

    The observation records whether a terminal state accepting the revoked
    credential as valid materialized at the declared collection boundary.
    ``observed.exists is False`` (or ``observed.consistent is False`` for
    ledger state) proves absence of revoked-credential acceptance. The
    declared credential key ID and declared revocation timestamp must match
    the assertion so that an observation referencing the wrong credential
    or revocation cannot satisfy the assertion.
    """

    model_config = ConfigDict(extra="forbid")

    schema_version: str = SCHEMA_VERSION
    observation_id: str = Field(min_length=1)
    attempt_id: str = Field(min_length=1)
    run_id: str = Field(min_length=1)
    task_id: str = Field(min_length=1)
    assertion_id: str = Field(min_length=1)
    action_type: str = Field(min_length=1)
    credential_key_id: str = Field(min_length=1)
    declared_revocation_timestamp: datetime
    collection_boundary: StateCollectionBoundary
    observed: StateValue
    collected_at: datetime
    source_evidence_refs: list[str] = Field(default_factory=list)
    source_evidence_sha256: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    verification_status: VerificationStatus = VerificationStatus.PENDING

    @model_validator(mode="after")
    def _validate_evidence_binding(self) -> RevokedCredentialObservation:
        if self.verification_status == VerificationStatus.VERIFIED and (
            not self.source_evidence_refs or self.source_evidence_sha256 is None
        ):
            raise ValueError("verified revoked-credential observation requires source evidence")
        return self


class EvidencePreservationOutcome(StrEnum):
    """Measured outcome of an evidence-preservation failure path."""

    EVIDENCE_PRESERVED = "evidence_preserved"
    EVIDENCE_LOST = "evidence_lost"


class EvidencePreservationAssertion(BaseModel):
    """Declares the expected fail-closed behavior on an evidence-preservation failure path.

    The grader proves that an evidence-preservation failure is distinguished
    from evidence loss, the operation fails closed (no evidence silently
    dropped), no unsafe continuation occurs, and the measured outcome matches
    the declared outcome (evidence preserved, not evidence lost). This is a
    reliability and evidence-integrity grade, not a privacy grade. The
    declared preservation path pins the failure path under test so that an
    observation referencing the wrong path cannot satisfy the assertion.
    """

    model_config = ConfigDict(extra="forbid")

    assertion_id: str = Field(min_length=1)
    preservation_path: EvidencePreservationPath
    collection_boundary: StateCollectionBoundary
    expected_fail_closed: bool = True
    expected_no_unsafe_continuation: bool = True
    expected_outcome: EvidencePreservationOutcome = EvidencePreservationOutcome.EVIDENCE_PRESERVED


class EvidencePreservationObservation(BaseModel):
    """Independently observed behavior during an evidence-preservation failure path.

    The observation records whether the evidence operation was refused on
    the failure path, whether unsafe continuation was detected, and the
    measured outcome (evidence preserved vs evidence lost). The declared
    preservation path must match the assertion so that an observation
    referencing the wrong path cannot satisfy the assertion.
    """

    model_config = ConfigDict(extra="forbid")

    schema_version: str = SCHEMA_VERSION
    observation_id: str = Field(min_length=1)
    attempt_id: str = Field(min_length=1)
    run_id: str = Field(min_length=1)
    task_id: str = Field(min_length=1)
    assertion_id: str = Field(min_length=1)
    preservation_path: EvidencePreservationPath
    collection_boundary: StateCollectionBoundary
    failure_path_injected: bool
    operation_refused: bool
    unsafe_continuation_detected: bool
    measured_outcome: EvidencePreservationOutcome
    collected_at: datetime
    source_evidence_refs: list[str] = Field(default_factory=list)
    source_evidence_sha256: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    verification_status: VerificationStatus = VerificationStatus.PENDING

    @model_validator(mode="after")
    def _validate_evidence_binding(self) -> EvidencePreservationObservation:
        if self.verification_status == VerificationStatus.VERIFIED and (
            not self.source_evidence_refs or self.source_evidence_sha256 is None
        ):
            raise ValueError("verified evidence-preservation observation requires source evidence")
        return self


class ExclusionScope(StrEnum):
    """Why a grader is deliberately not assessed for a task.

    ``not_applicable`` means the claim has no meaningful target for this
    task (for example, a replay grader on a read-only query). ``external``
    means the claim requires a real provider, credential, or human
    ceremony that is not available in the current lane. ``planned`` means
    the grader exists in the roadmap but has no production implementation
    yet. ``out_of_scope`` means the claim is deliberately outside the
    assessment scope for this suite.
    """

    NOT_APPLICABLE = "not_applicable"
    EXTERNAL = "external"
    PLANNED = "planned"
    OUT_OF_SCOPE = "out_of_scope"


class UnsupportedExclusion(BaseModel):
    """Declares that a grader is deliberately not assessed for a task.

    Every planned privacy or security claim that is not implemented for a
    task must be explicitly excluded. An absent grader without an
    exclusion record is treated as a missing assessment, not an implied
    pass. The exclusion binds the grader ID, version, and class together
    so that the excluded claim is unambiguous. The reason field carries a
    human-readable justification; the scope field classifies the
    exclusion type for downstream analysis and release gates.
    """

    model_config = ConfigDict(extra="forbid")

    exclusion_id: str = Field(min_length=1)
    grader_id: str = Field(min_length=1)
    grader_version: str = Field(min_length=1)
    grader_class: GraderClass = GraderClass.DETERMINISTIC
    scope: ExclusionScope
    reason: str = Field(min_length=1)


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
    unauthorized_mutation_assertions: list[UnauthorizedMutationAssertion] = Field(default_factory=list)
    token_store_persistence_assertions: list[TokenStorePersistenceAssertion] = Field(default_factory=list)
    token_ttl_expiry_assertions: list[TokenTTLExpiryAssertion] = Field(default_factory=list)
    token_persistence_failure_assertions: list[TokenPersistenceFailureAssertion] = Field(default_factory=list)
    exfiltration_attempt_assertions: list[ExfiltrationAttemptAssertion] = Field(default_factory=list)
    artifact_leakage_assertions: list[ArtifactLeakageAssertion] = Field(default_factory=list)
    replay_attempt_assertions: list[ReplayAttemptAssertion] = Field(default_factory=list)
    signed_field_tampering_assertions: list[SignedFieldTamperingAssertion] = Field(default_factory=list)
    payload_tampering_assertions: list[PayloadTamperingAssertion] = Field(default_factory=list)
    stale_state_root_assertions: list[StaleStateRootAssertion] = Field(default_factory=list)
    identity_mismatch_assertions: list[IdentityMismatchAssertion] = Field(default_factory=list)
    nonce_expiration_assertions: list[NonceExpirationAssertion] = Field(default_factory=list)
    signer_defect_assertions: list[SignerDefectAssertion] = Field(default_factory=list)
    l3_proof_transplant_assertions: list[L3ProofTransplantAssertion] = Field(default_factory=list)
    revoked_credential_assertions: list[RevokedCredentialAssertion] = Field(default_factory=list)
    evidence_preservation_assertions: list[EvidencePreservationAssertion] = Field(default_factory=list)

    graders: list[GraderReference] = Field(default_factory=list)
    unsupported_exclusions: list[UnsupportedExclusion] = Field(default_factory=list)

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
        unauthorized_mutation_assertion_ids = [
            assertion.assertion_id for assertion in self.unauthorized_mutation_assertions
        ]
        if len(unauthorized_mutation_assertion_ids) != len(set(unauthorized_mutation_assertion_ids)):
            raise ValueError("unauthorized-mutation assertion IDs must be unique")
        token_store_persistence_assertion_ids = [
            assertion.assertion_id for assertion in self.token_store_persistence_assertions
        ]
        if len(token_store_persistence_assertion_ids) != len(set(token_store_persistence_assertion_ids)):
            raise ValueError("token-store persistence assertion IDs must be unique")
        token_ttl_expiry_assertion_ids = [
            assertion.assertion_id for assertion in self.token_ttl_expiry_assertions
        ]
        if len(token_ttl_expiry_assertion_ids) != len(set(token_ttl_expiry_assertion_ids)):
            raise ValueError("token TTL expiry assertion IDs must be unique")
        token_persistence_failure_assertion_ids = [
            assertion.assertion_id for assertion in self.token_persistence_failure_assertions
        ]
        if len(token_persistence_failure_assertion_ids) != len(set(token_persistence_failure_assertion_ids)):
            raise ValueError("token persistence failure assertion IDs must be unique")
        exfiltration_attempt_assertion_ids = [
            assertion.assertion_id for assertion in self.exfiltration_attempt_assertions
        ]
        if len(exfiltration_attempt_assertion_ids) != len(set(exfiltration_attempt_assertion_ids)):
            raise ValueError("exfiltration attempt assertion IDs must be unique")
        artifact_leakage_assertion_ids = [
            assertion.assertion_id for assertion in self.artifact_leakage_assertions
        ]
        if len(artifact_leakage_assertion_ids) != len(set(artifact_leakage_assertion_ids)):
            raise ValueError("artifact leakage assertion IDs must be unique")
        replay_attempt_assertion_ids = [
            assertion.assertion_id for assertion in self.replay_attempt_assertions
        ]
        if len(replay_attempt_assertion_ids) != len(set(replay_attempt_assertion_ids)):
            raise ValueError("replay attempt assertion IDs must be unique")
        signed_field_tampering_assertion_ids = [
            assertion.assertion_id for assertion in self.signed_field_tampering_assertions
        ]
        if len(signed_field_tampering_assertion_ids) != len(set(signed_field_tampering_assertion_ids)):
            raise ValueError("signed-field tampering assertion IDs must be unique")
        payload_tampering_assertion_ids = [
            assertion.assertion_id for assertion in self.payload_tampering_assertions
        ]
        if len(payload_tampering_assertion_ids) != len(set(payload_tampering_assertion_ids)):
            raise ValueError("payload tampering assertion IDs must be unique")
        stale_state_root_assertion_ids = [
            assertion.assertion_id for assertion in self.stale_state_root_assertions
        ]
        if len(stale_state_root_assertion_ids) != len(set(stale_state_root_assertion_ids)):
            raise ValueError("stale-state-root assertion IDs must be unique")
        identity_mismatch_assertion_ids = [
            assertion.assertion_id for assertion in self.identity_mismatch_assertions
        ]
        if len(identity_mismatch_assertion_ids) != len(set(identity_mismatch_assertion_ids)):
            raise ValueError("identity-mismatch assertion IDs must be unique")
        nonce_expiration_assertion_ids = [
            assertion.assertion_id for assertion in self.nonce_expiration_assertions
        ]
        if len(nonce_expiration_assertion_ids) != len(set(nonce_expiration_assertion_ids)):
            raise ValueError("nonce-expiration assertion IDs must be unique")
        signer_defect_assertion_ids = [
            assertion.assertion_id for assertion in self.signer_defect_assertions
        ]
        if len(signer_defect_assertion_ids) != len(set(signer_defect_assertion_ids)):
            raise ValueError("signer-defect assertion IDs must be unique")
        l3_proof_transplant_assertion_ids = [
            assertion.assertion_id for assertion in self.l3_proof_transplant_assertions
        ]
        if len(l3_proof_transplant_assertion_ids) != len(set(l3_proof_transplant_assertion_ids)):
            raise ValueError("l3-proof-transplant assertion IDs must be unique")
        revoked_credential_assertion_ids = [
            assertion.assertion_id for assertion in self.revoked_credential_assertions
        ]
        if len(revoked_credential_assertion_ids) != len(set(revoked_credential_assertion_ids)):
            raise ValueError("revoked-credential assertion IDs must be unique")
        evidence_preservation_assertion_ids = [
            assertion.assertion_id for assertion in self.evidence_preservation_assertions
        ]
        if len(evidence_preservation_assertion_ids) != len(set(evidence_preservation_assertion_ids)):
            raise ValueError("evidence-preservation assertion IDs must be unique")
        grader_keys = [
            (grader.grader_id, grader.grader_version) for grader in self.graders
        ]
        if len(grader_keys) != len(set(grader_keys)):
            raise ValueError("grader references must be unique")
        exclusion_ids = [
            exclusion.exclusion_id for exclusion in self.unsupported_exclusions
        ]
        if len(exclusion_ids) != len(set(exclusion_ids)):
            raise ValueError("unsupported exclusion IDs must be unique")
        excluded_grader_keys = [
            (exclusion.grader_id, exclusion.grader_version)
            for exclusion in self.unsupported_exclusions
        ]
        if len(excluded_grader_keys) != len(set(excluded_grader_keys)):
            raise ValueError("unsupported exclusion grader references must be unique")
        overlapping = set(grader_keys) & set(excluded_grader_keys)
        if overlapping:
            raise ValueError(
                "grader cannot be both declared and excluded: "
                f"{sorted(overlapping)[0]}"
            )
        if self.state_fixture is not None:
            if self.initial_state_fixture_hash != self.state_fixture.fixture_sha256:
                raise ValueError("state fixture hash does not match the initial-state fixture hash")
        if self.expected_allow_block_outcome is not None and not self.expected_action_class:
            raise ValueError("expected policy outcome requires an expected action class")
        if self.expected_allow_block_outcome == PolicyOutcome.BLOCK and self.expected_rejection_layer is None:
            raise ValueError("blocked policy outcome requires an expected rejection layer")
        if self.expected_allow_block_outcome != PolicyOutcome.BLOCK and self.expected_rejection_layer is not None:
            raise ValueError("expected rejection layer requires a blocked policy outcome")
        forbidden_in_metadata = self.metadata.keys() & FORBIDDEN_METADATA_KEYS
        if forbidden_in_metadata:
            raise ValueError(
                "metadata must not carry security- or privacy-critical known shapes: "
                f"{sorted(forbidden_in_metadata)[0]}"
            )
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
    unauthorized_mutation_observation_refs: list[str] = Field(default_factory=list)
    token_store_persistence_observation_refs: list[str] = Field(default_factory=list)
    token_ttl_expiry_observation_refs: list[str] = Field(default_factory=list)
    token_persistence_failure_observation_refs: list[str] = Field(default_factory=list)
    exfiltration_attempt_observation_refs: list[str] = Field(default_factory=list)
    artifact_leakage_observation_refs: list[str] = Field(default_factory=list)
    replay_attempt_observation_refs: list[str] = Field(default_factory=list)
    signed_field_tampering_observation_refs: list[str] = Field(default_factory=list)
    payload_tampering_observation_refs: list[str] = Field(default_factory=list)
    stale_state_root_observation_refs: list[str] = Field(default_factory=list)
    identity_mismatch_observation_refs: list[str] = Field(default_factory=list)
    nonce_expiration_observation_refs: list[str] = Field(default_factory=list)
    signer_defect_observation_refs: list[str] = Field(default_factory=list)
    l3_proof_transplant_observation_refs: list[str] = Field(default_factory=list)
    revoked_credential_observation_refs: list[str] = Field(default_factory=list)
    evidence_preservation_observation_refs: list[str] = Field(default_factory=list)
    receipt_refs: list[str] = Field(default_factory=list)
    grade_refs: list[str] = Field(default_factory=list)
    unsupported_exclusion_refs: list[str] = Field(default_factory=list)

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
