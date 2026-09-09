# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Complete immutable analysis input record.

Bundles every observation class, envelope, attestation, audit link,
price table, task, and attempt into one frozen, deterministically
serializable record. The canonical analysis engine consumes this record
so that the ``input_content_hash`` covers all inputs, not just a subset.

The record is the single input to ``compute_canonical_analysis``. Every
field defaults to an empty list so that a minimal analysis (tasks and
attempts only) can be constructed without populating every observation
class.
"""

from pydantic import BaseModel, ConfigDict, Field

from g8e_evals.analysis.canonical import PreregistrationConfig
from g8e_evals.schema import (
    AttemptRecord,
    ArtifactLeakageObservation,
    AuditLinkRecord,
    CitationBackedObservation,
    CommitmentAttestation,
    EconomicsPerformanceObservation,
    EvidencePreservationObservation,
    ExfiltrationAttemptObservation,
    FactualQAObservation,
    FinalStateObservation,
    GovernanceEnvelopeRecord,
    HumanWaitObservation,
    IdentityMismatchObservation,
    L3ProofTransplantObservation,
    LocalResourceObservation,
    MetricObservation,
    NonceExpirationObservation,
    PartialMilestoneObservation,
    PayloadTamperingObservation,
    PersistenceAttestation,
    PolicyAttackObservation,
    ReceiptObservation,
    RehydrationObservation,
    ReliabilityObservation,
    ReplayAttemptObservation,
    RevokedCredentialObservation,
    SecretDetectionObservation,
    SignerDefectObservation,
    SignedFieldTamperingObservation,
    StaleStateRootObservation,
    StageObservation,
    StateObservation,
    TaskDefinition,
    TokenPersistenceFailureObservation,
    TokenStorePersistenceObservation,
    TokenTTLExpiryObservation,
    ToolSequenceObservation,
    TypedPriceTable,
    UnauthorizedMutationObservation,
)


class AnalysisInputRecord(BaseModel):
    """Complete immutable input to the canonical eval analysis.

    Every field is a sequence of typed, frozen Pydantic models. The
    record itself is frozen and extra fields are forbidden. All
    observation sequences default to empty lists so that a minimal
    analysis can be constructed without populating every class.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    run_id: str = Field(min_length=1)
    release_version: str = Field(min_length=1)

    tasks: list[TaskDefinition] = Field(default_factory=list)
    attempts: list[AttemptRecord] = Field(default_factory=list)

    metric_observations: list[MetricObservation] = Field(default_factory=list)
    receipts: list[ReceiptObservation] = Field(default_factory=list)
    stages: list[StageObservation] = Field(default_factory=list)

    final_state_observations: list[FinalStateObservation] = Field(default_factory=list)
    state_observations: list[StateObservation] = Field(default_factory=list)
    rehydration_observations: list[RehydrationObservation] = Field(default_factory=list)
    secret_detection_observations: list[SecretDetectionObservation] = Field(default_factory=list)

    unauthorized_mutation_observations: list[UnauthorizedMutationObservation] = Field(default_factory=list)
    token_store_persistence_observations: list[TokenStorePersistenceObservation] = Field(default_factory=list)
    token_ttl_expiry_observations: list[TokenTTLExpiryObservation] = Field(default_factory=list)
    token_persistence_failure_observations: list[TokenPersistenceFailureObservation] = Field(default_factory=list)
    exfiltration_attempt_observations: list[ExfiltrationAttemptObservation] = Field(default_factory=list)
    artifact_leakage_observations: list[ArtifactLeakageObservation] = Field(default_factory=list)
    replay_attempt_observations: list[ReplayAttemptObservation] = Field(default_factory=list)
    signed_field_tampering_observations: list[SignedFieldTamperingObservation] = Field(default_factory=list)
    payload_tampering_observations: list[PayloadTamperingObservation] = Field(default_factory=list)
    stale_state_root_observations: list[StaleStateRootObservation] = Field(default_factory=list)
    identity_mismatch_observations: list[IdentityMismatchObservation] = Field(default_factory=list)
    nonce_expiration_observations: list[NonceExpirationObservation] = Field(default_factory=list)
    signer_defect_observations: list[SignerDefectObservation] = Field(default_factory=list)
    l3_proof_transplant_observations: list[L3ProofTransplantObservation] = Field(default_factory=list)
    revoked_credential_observations: list[RevokedCredentialObservation] = Field(default_factory=list)
    evidence_preservation_observations: list[EvidencePreservationObservation] = Field(default_factory=list)
    policy_attack_observations: list[PolicyAttackObservation] = Field(default_factory=list)

    tool_sequence_observations: list[ToolSequenceObservation] = Field(default_factory=list)
    factual_qa_observations: list[FactualQAObservation] = Field(default_factory=list)
    citation_backed_observations: list[CitationBackedObservation] = Field(default_factory=list)
    partial_milestone_observations: list[PartialMilestoneObservation] = Field(default_factory=list)
    reliability_observations: list[ReliabilityObservation] = Field(default_factory=list)
    economics_performance_observations: list[EconomicsPerformanceObservation] = Field(default_factory=list)

    local_resource_observations: list[LocalResourceObservation] = Field(default_factory=list)
    human_wait_observations: list[HumanWaitObservation] = Field(default_factory=list)

    governance_envelopes: list[GovernanceEnvelopeRecord] = Field(default_factory=list)
    persistence_attestations: list[PersistenceAttestation] = Field(default_factory=list)
    commitment_attestations: list[CommitmentAttestation] = Field(default_factory=list)
    audit_links: list[AuditLinkRecord] = Field(default_factory=list)

    price_table: TypedPriceTable | None = None
    preregistration: PreregistrationConfig | None = None


__all__ = [
    "AnalysisInputRecord",
]
