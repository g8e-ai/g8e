# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import UTC, datetime
from enum import StrEnum
from typing import TYPE_CHECKING, Any, Protocol

from app.models.model_telemetry import ModelCallTelemetry
from g8e.constants import PORTS
from g8e.operator.v1.operator_pb2 import ActionReceipt
from g8e_evals.arms import Arm, ArmDefinition, get_arm_definition
from g8e_evals.auth_bridge import CLIAuthContext
from g8e_evals.models import ScoreDetails, TaskMetadata

# Forward reference for ChatEvaluationReceipt to avoid circular import
if TYPE_CHECKING:
    from g8e_evals.schema import (
        AttemptRecord,
        ArtifactLeakageObservation,
        EvidencePreservationObservation,
        ExfiltrationAttemptObservation,
        IdentityMismatchObservation,
        NonceExpirationObservation,
        PayloadTamperingObservation,
        PolicyAttackObservation,
        SignerDefectObservation,
        L3ProofTransplantObservation,
        RevokedCredentialObservation,
        RehydrationObservation,
        ReplayAttemptObservation,
        SecretDetectionObservation,
        SignedFieldTamperingObservation,
        StaleStateRootObservation,
        StateObservation,
        TaskDefinition,
        TokenStorePersistenceObservation,
        TokenTTLExpiryObservation,
        TokenPersistenceFailureObservation,
        UnauthorizedMutationObservation,
    )


class EvidenceLike(Protocol):
    """Structural protocol for chat evidence attached to a Response.

    Both ``ChatEvaluationReceipt`` (g8ee arms) and ``_DirectEvidenceWrapper``
    (direct arm) satisfy this protocol. The CLI and report writer consume
    these attributes without forcing every arm into the SSE evidence schema.
    """

    @property
    def terminal_event(self) -> str | None: ...

    @property
    def event_count(self) -> int: ...

    def model_dump(self) -> dict[str, Any]: ...


class StateObserver(Protocol):
    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[StateObservation]: ...


class RehydrationObserver(Protocol):
    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[RehydrationObservation]: ...


class SecretDetectionObserver(Protocol):
    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[SecretDetectionObservation]: ...


class UnauthorizedMutationObserver(Protocol):
    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[UnauthorizedMutationObservation]: ...


class TokenStorePersistenceObserver(Protocol):
    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[TokenStorePersistenceObservation]: ...


class TokenTTLExpiryObserver(Protocol):
    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[TokenTTLExpiryObservation]: ...


class TokenPersistenceFailureObserver(Protocol):
    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[TokenPersistenceFailureObservation]: ...


class ExfiltrationAttemptObserver(Protocol):
    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[ExfiltrationAttemptObservation]: ...


class ArtifactLeakageObserver(Protocol):
    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[ArtifactLeakageObservation]: ...


class ReplayAttemptObserver(Protocol):
    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[ReplayAttemptObservation]: ...


class SignedFieldTamperingObserver(Protocol):
    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[SignedFieldTamperingObservation]: ...


class PayloadTamperingObserver(Protocol):
    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[PayloadTamperingObservation]: ...


class StaleStateRootObserver(Protocol):
    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[StaleStateRootObservation]: ...


class IdentityMismatchObserver(Protocol):
    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[IdentityMismatchObservation]: ...


class NonceExpirationObserver(Protocol):
    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[NonceExpirationObservation]: ...


class SignerDefectObserver(Protocol):
    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[SignerDefectObservation]: ...


class L3ProofTransplantObserver(Protocol):
    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[L3ProofTransplantObservation]: ...


class RevokedCredentialObserver(Protocol):
    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[RevokedCredentialObservation]: ...


class EvidencePreservationObserver(Protocol):
    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[EvidencePreservationObservation]: ...


class PolicyAttackObserver(Protocol):
    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[PolicyAttackObservation]: ...


class BindingType(StrEnum):
    RECEIPT_BOUND = "RECEIPT_BOUND"
    UNBOUND = "UNBOUND"


@dataclass
class LLMRoleConfig:
    provider: str | None = None
    model: str | None = None
    api_key: str | None = None
    endpoint: str | None = None

@dataclass
class SUTConfig:
    g8ee_url: str
    primary: LLMRoleConfig = field(default_factory=LLMRoleConfig)
    assistant: LLMRoleConfig = field(default_factory=LLMRoleConfig)
    lite: LLMRoleConfig = field(default_factory=LLMRoleConfig)
    judge: LLMRoleConfig = field(default_factory=LLMRoleConfig)

    web_search_project: str | None = None
    web_search_app: str | None = None
    web_search_api_key: str | None = None

    operator_url: str = f"https://localhost:{PORTS['ports']['OperatorHttps']['value']}"
    operator_session_id: str | None = None
    auth_context: CLIAuthContext | None = None
    state_root: str = "test-state-root-v1"

    l2_private_key: str | None = None
    l2_key_id: str | None = None
    arm: Arm = Arm.ENSEMBLE_UNGOVERNED
    headless: bool = False
    state_observer: StateObserver | None = None
    rehydration_observer: RehydrationObserver | None = None
    secret_detection_observer: SecretDetectionObserver | None = None
    unauthorized_mutation_observer: UnauthorizedMutationObserver | None = None
    token_store_persistence_observer: TokenStorePersistenceObserver | None = None
    token_ttl_expiry_observer: TokenTTLExpiryObserver | None = None
    token_persistence_failure_observer: TokenPersistenceFailureObserver | None = None
    exfiltration_attempt_observer: ExfiltrationAttemptObserver | None = None
    artifact_leakage_observer: ArtifactLeakageObserver | None = None
    replay_attempt_observer: ReplayAttemptObserver | None = None
    signed_field_tampering_observer: SignedFieldTamperingObserver | None = None
    payload_tampering_observer: PayloadTamperingObserver | None = None
    stale_state_root_observer: StaleStateRootObserver | None = None
    identity_mismatch_observer: IdentityMismatchObserver | None = None
    nonce_expiration_observer: NonceExpirationObserver | None = None
    signer_defect_observer: SignerDefectObserver | None = None
    l3_proof_transplant_observer: L3ProofTransplantObserver | None = None
    revoked_credential_observer: RevokedCredentialObserver | None = None
    evidence_preservation_observer: EvidencePreservationObserver | None = None
    policy_attack_observer: PolicyAttackObserver | None = None

    @property
    def arm_definition(self) -> ArmDefinition:
        """Return the static arm definition for this config's arm."""
        return get_arm_definition(self.arm)

@dataclass
class Task:
    id: str
    prompt: str
    metadata: TaskMetadata = field(default_factory=TaskMetadata)


@dataclass(frozen=True)
class ReceiptEvidence:
    action_receipt: ActionReceipt
    verified: bool


@dataclass
class Response:
    answer: str
    model: str
    arm: Arm = Arm.ENSEMBLE_UNGOVERNED
    transaction_ids: list[str] = field(default_factory=list)
    governed_action_types: list[str] = field(default_factory=list)
    chat_evidence: EvidenceLike | None = None
    receipts: list[ReceiptEvidence] = field(default_factory=list)
    primary_transaction_id: str | None = None
    binding: BindingType = BindingType.UNBOUND
    unbound_reason: str | None = None

    @property
    def receipts_verified(self) -> bool:
        return bool(self.receipts) and all(receipt.verified for receipt in self.receipts)


@dataclass
class Score:
    task_id: str
    passed: bool
    details: ScoreDetails = field(default_factory=ScoreDetails)
    model_calls: list[ModelCallTelemetry] = field(default_factory=list)


@dataclass
class RowResult:
    task: Task
    response: Response
    score: Score
    arm: Arm = Arm.ENSEMBLE_UNGOVERNED
    timestamp: datetime = field(default_factory=lambda: datetime.now(UTC))


@dataclass
class Aggregate:
    suite: str
    pass_rate: float
    total_tasks: int
    passed_tasks: int
    receipt_coverage_pct: float
    receipt_verification_pct: float
    metadata: dict[str, Any] = field(default_factory=dict)
