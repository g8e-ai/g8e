# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Production observer implementations for the synthetic governance-adversarial suite.

These observers interact with a ``LocalGovernanceSimulator`` to produce typed
observations that the deterministic security graders consume.  Each observer
implements the corresponding protocol from ``g8e_evals.harness`` and produces
verified observations bound to source evidence.

The observers produce two kinds of evidence:

1. ``ReceiptObservation`` records from the simulator's signed receipts, with
   deterministic stage evidence showing the rejection at the declared layer.
2. Typed security observations (``ReplayAttemptObservation``,
   ``SignedFieldTamperingObservation``, ``NonceExpirationObservation``,
   ``StaleStateRootObservation``, ``SignerDefectObservation``,
   ``L3ProofTransplantObservation``, ``RevokedCredentialObservation``) that
   record whether the prohibited terminal state materialized at the declared
   collection boundary.
"""

from __future__ import annotations

from datetime import UTC, datetime

from g8e_evals.benchmarks.governance.simulator import GovernanceActionResult, LocalGovernanceSimulator
from g8e_evals.schema import (
    AttemptRecord,
    L3ProofTransplantObservation,
    NonceExpirationObservation,
    PolicyAttackObservation,
    ReceiptObservation,
    ReplayAttemptObservation,
    RevokedCredentialObservation,
    SignedFieldTamperingObservation,
    SignerDefectObservation,
    StaleStateRootObservation,
    StateEvidenceKind,
    StateValue,
    TaskDefinition,
    VerificationStatus,
)


def _make_receipt_observation(
    result: GovernanceActionResult,
    attempt: AttemptRecord,
    action_type: str,
) -> ReceiptObservation:
    """Build a verified ``ReceiptObservation`` from a governance action result."""
    return ReceiptObservation(
        receipt_id=f"{attempt.attempt_id}:receipt",
        attempt_id=attempt.attempt_id,
        run_id=attempt.run_id,
        transaction_id=result.transaction_id,
        action_type=action_type,
        primary=True,
        verified=True,
        action_receipt=result.receipt,
    )


class ReplayAttemptObserverImpl:
    """Observes replay-attempt governance actions and produces typed observations.

    The observer processes each replay assertion through the governance
    simulator, produces a ``ReceiptObservation`` from the signed receipt, and
    produces a ``ReplayAttemptObservation`` recording whether the replayed
    transaction was double-accepted at the declared collection boundary.
    """

    def __init__(self, simulator: LocalGovernanceSimulator, evidence_sha: str, evidence_ref: str) -> None:
        self._simulator = simulator
        self._evidence_sha = evidence_sha
        self._evidence_ref = evidence_ref

    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> tuple[ReceiptObservation, list[ReplayAttemptObservation]]:
        receipt_observations: list[ReceiptObservation] = []
        replay_observations: list[ReplayAttemptObservation] = []

        for assertion in task.replay_attempt_assertions:
            result = self._simulator.process_replay(
                action_type=assertion.action_type,
                replayed_transaction_id=assertion.replayed_transaction_id,
                replayed_transaction_hash=assertion.replayed_transaction_hash,
                rejection_layer=assertion.expected_rejection_layer,
            )
            receipt_observations.append(
                _make_receipt_observation(result, attempt, assertion.action_type)
            )

            double_accepted = self._simulator.is_transaction_double_accepted(
                assertion.replayed_transaction_id
            )
            observed = StateValue(
                kind=StateEvidenceKind.FILE,
                exists=double_accepted,
            )

            replay_observations.append(ReplayAttemptObservation(
                observation_id=f"{attempt.attempt_id}:replay:{assertion.assertion_id}",
                attempt_id=attempt.attempt_id,
                run_id=attempt.run_id,
                task_id=attempt.task_id,
                assertion_id=assertion.assertion_id,
                action_type=assertion.action_type,
                replayed_transaction_id=assertion.replayed_transaction_id,
                replayed_transaction_hash=assertion.replayed_transaction_hash,
                collection_boundary=assertion.collection_boundary,
                observed=observed,
                collected_at=datetime.now(UTC),
                source_evidence_refs=[self._evidence_ref],
                source_evidence_sha256=self._evidence_sha,
                verification_status=VerificationStatus.VERIFIED,
            ))

        return receipt_observations[0], replay_observations


class SignedFieldTamperingObserverImpl:
    """Observes signed-field tampering governance actions and produces typed observations.

    The observer processes each signed-field tampering assertion through the
    governance simulator, produces a ``ReceiptObservation`` from the signed
    receipt, and produces a ``SignedFieldTamperingObservation`` recording
    whether the tampered field value was accepted at the declared collection
    boundary.
    """

    def __init__(self, simulator: LocalGovernanceSimulator, evidence_sha: str, evidence_ref: str) -> None:
        self._simulator = simulator
        self._evidence_sha = evidence_sha
        self._evidence_ref = evidence_ref

    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> tuple[ReceiptObservation, list[SignedFieldTamperingObservation]]:
        receipt_observations: list[ReceiptObservation] = []
        tampering_observations: list[SignedFieldTamperingObservation] = []

        for assertion in task.signed_field_tampering_assertions:
            result = self._simulator.process_signed_field_tampering(
                action_type=assertion.action_type,
                tampered_field=assertion.tampered_field.value,
                tampered_value=assertion.tampered_value,
                rejection_layer=assertion.expected_rejection_layer,
            )
            receipt_observations.append(
                _make_receipt_observation(result, attempt, assertion.action_type)
            )

            tampered_accepted = self._simulator.is_tampered_field_accepted(
                assertion.tampered_field.value, assertion.tampered_value
            )
            observed = StateValue(
                kind=StateEvidenceKind.FILE,
                exists=tampered_accepted,
            )

            tampering_observations.append(SignedFieldTamperingObservation(
                observation_id=f"{attempt.attempt_id}:signed-field:{assertion.assertion_id}",
                attempt_id=attempt.attempt_id,
                run_id=attempt.run_id,
                task_id=attempt.task_id,
                assertion_id=assertion.assertion_id,
                action_type=assertion.action_type,
                tampered_field=assertion.tampered_field,
                tampered_value=assertion.tampered_value,
                collection_boundary=assertion.collection_boundary,
                observed=observed,
                collected_at=datetime.now(UTC),
                source_evidence_refs=[self._evidence_ref],
                source_evidence_sha256=self._evidence_sha,
                verification_status=VerificationStatus.VERIFIED,
            ))

        return receipt_observations[0], tampering_observations


class NonceExpirationObserverImpl:
    """Observes expired-nonce reuse governance actions and produces typed observations.

    The observer processes each nonce-expiration assertion through the
    governance simulator, produces a ``ReceiptObservation`` from the signed
    receipt, and produces a ``NonceExpirationObservation`` recording whether
    the expired nonce was accepted as valid at the declared collection
    boundary.
    """

    def __init__(self, simulator: LocalGovernanceSimulator, evidence_sha: str, evidence_ref: str) -> None:
        self._simulator = simulator
        self._evidence_sha = evidence_sha
        self._evidence_ref = evidence_ref

    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> tuple[ReceiptObservation, list[NonceExpirationObservation]]:
        receipt_observations: list[ReceiptObservation] = []
        nonce_observations: list[NonceExpirationObservation] = []

        for assertion in task.nonce_expiration_assertions:
            result = self._simulator.process_nonce_expiration(
                action_type=assertion.action_type,
                nonce_value=assertion.nonce_value,
                declared_expiry_timestamp=assertion.declared_expiry_timestamp,
                rejection_layer=assertion.expected_rejection_layer,
            )
            receipt_observations.append(
                _make_receipt_observation(result, attempt, assertion.action_type)
            )

            nonce_accepted = self._simulator.is_expired_nonce_accepted(assertion.nonce_value)
            observed = StateValue(
                kind=StateEvidenceKind.FILE,
                exists=nonce_accepted,
            )

            nonce_observations.append(NonceExpirationObservation(
                observation_id=f"{attempt.attempt_id}:nonce:{assertion.assertion_id}",
                attempt_id=attempt.attempt_id,
                run_id=attempt.run_id,
                task_id=attempt.task_id,
                assertion_id=assertion.assertion_id,
                action_type=assertion.action_type,
                nonce_value=assertion.nonce_value,
                declared_expiry_timestamp=assertion.declared_expiry_timestamp,
                collection_boundary=assertion.collection_boundary,
                observed=observed,
                collected_at=datetime.now(UTC),
                source_evidence_refs=[self._evidence_ref],
                source_evidence_sha256=self._evidence_sha,
                verification_status=VerificationStatus.VERIFIED,
            ))

        return receipt_observations[0], nonce_observations


class StaleStateRootObserverImpl:
    """Observes stale-state-root replay governance actions and produces typed observations.

    The observer processes each stale-state-root assertion through the
    governance simulator, produces a ``ReceiptObservation`` from the signed
    receipt, and produces a ``StaleStateRootObservation`` recording whether
    the stale root was accepted as the current root at the declared
    collection boundary.
    """

    def __init__(self, simulator: LocalGovernanceSimulator, evidence_sha: str, evidence_ref: str) -> None:
        self._simulator = simulator
        self._evidence_sha = evidence_sha
        self._evidence_ref = evidence_ref

    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> tuple[ReceiptObservation, list[StaleStateRootObservation]]:
        receipt_observations: list[ReceiptObservation] = []
        stale_root_observations: list[StaleStateRootObservation] = []

        for assertion in task.stale_state_root_assertions:
            result = self._simulator.process_stale_state_root(
                action_type=assertion.action_type,
                declared_current_root=assertion.declared_current_root,
                stale_root_replayed=assertion.stale_root_replayed,
                rejection_layer=assertion.expected_rejection_layer,
            )
            receipt_observations.append(
                _make_receipt_observation(result, attempt, assertion.action_type)
            )

            stale_accepted = self._simulator.is_stale_root_accepted_as_current(
                assertion.stale_root_replayed
            )
            observed = StateValue(
                kind=StateEvidenceKind.FILE,
                exists=stale_accepted,
            )

            stale_root_observations.append(StaleStateRootObservation(
                observation_id=f"{attempt.attempt_id}:stale-root:{assertion.assertion_id}",
                attempt_id=attempt.attempt_id,
                run_id=attempt.run_id,
                task_id=attempt.task_id,
                assertion_id=assertion.assertion_id,
                action_type=assertion.action_type,
                declared_current_root=assertion.declared_current_root,
                stale_root_replayed=assertion.stale_root_replayed,
                collection_boundary=assertion.collection_boundary,
                observed=observed,
                collected_at=datetime.now(UTC),
                source_evidence_refs=[self._evidence_ref],
                source_evidence_sha256=self._evidence_sha,
                verification_status=VerificationStatus.VERIFIED,
            ))

        return receipt_observations[0], stale_root_observations


class SignerDefectObserverImpl:
    """Observes signer-set defect governance actions and produces typed observations.

    The observer processes each signer-defect assertion through the
    governance simulator, produces a ``ReceiptObservation`` from the signed
    receipt, and produces a ``SignerDefectObservation`` recording whether
    the defective signer set was accepted as authoritative at the declared
    collection boundary.
    """

    def __init__(self, simulator: LocalGovernanceSimulator, evidence_sha: str, evidence_ref: str) -> None:
        self._simulator = simulator
        self._evidence_sha = evidence_sha
        self._evidence_ref = evidence_ref

    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> tuple[ReceiptObservation, list[SignerDefectObservation]]:
        receipt_observations: list[ReceiptObservation] = []
        signer_defect_observations: list[SignerDefectObservation] = []

        for assertion in task.signer_defect_assertions:
            result = self._simulator.process_signer_defect(
                action_type=assertion.action_type,
                defect_type=assertion.defect_type.value,
                declared_required_quorum=assertion.declared_required_quorum,
                duplicate_signer_key_id=assertion.duplicate_signer_key_id,
                rejection_layer=assertion.expected_rejection_layer,
            )
            receipt_observations.append(
                _make_receipt_observation(result, attempt, assertion.action_type)
            )

            defect_key = assertion.duplicate_signer_key_id or f"quorum-{assertion.declared_required_quorum}"
            defective_accepted = self._simulator.is_defective_signer_accepted(defect_key)
            observed = StateValue(
                kind=StateEvidenceKind.FILE,
                exists=defective_accepted,
            )

            signer_defect_observations.append(SignerDefectObservation(
                observation_id=f"{attempt.attempt_id}:signer-defect:{assertion.assertion_id}",
                attempt_id=attempt.attempt_id,
                run_id=attempt.run_id,
                task_id=attempt.task_id,
                assertion_id=assertion.assertion_id,
                action_type=assertion.action_type,
                defect_type=assertion.defect_type,
                declared_required_quorum=assertion.declared_required_quorum,
                duplicate_signer_key_id=assertion.duplicate_signer_key_id,
                collection_boundary=assertion.collection_boundary,
                observed=observed,
                collected_at=datetime.now(UTC),
                source_evidence_refs=[self._evidence_ref],
                source_evidence_sha256=self._evidence_sha,
                verification_status=VerificationStatus.VERIFIED,
            ))

        return receipt_observations[0], signer_defect_observations


class L3ProofTransplantObserverImpl:
    """Observes transplanted-L3-proof reuse governance actions and produces typed observations.

    The observer processes each L3-proof-transplant assertion through the
    governance simulator, produces a ``ReceiptObservation`` from the signed
    receipt, and produces an ``L3ProofTransplantObservation`` recording
    whether the transplanted proof was accepted as valid at the declared
    collection boundary.
    """

    def __init__(self, simulator: LocalGovernanceSimulator, evidence_sha: str, evidence_ref: str) -> None:
        self._simulator = simulator
        self._evidence_sha = evidence_sha
        self._evidence_ref = evidence_ref

    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> tuple[ReceiptObservation, list[L3ProofTransplantObservation]]:
        receipt_observations: list[ReceiptObservation] = []
        l3_proof_observations: list[L3ProofTransplantObservation] = []

        for assertion in task.l3_proof_transplant_assertions:
            result = self._simulator.process_l3_proof_transplant(
                action_type=assertion.action_type,
                original_transaction_id=assertion.original_transaction_id,
                original_l3_proof_hash=assertion.original_l3_proof_hash,
                rejection_layer=assertion.expected_rejection_layer,
            )
            receipt_observations.append(
                _make_receipt_observation(result, attempt, assertion.action_type)
            )

            proof_accepted = self._simulator.is_transplanted_l3_proof_accepted(
                assertion.original_l3_proof_hash
            )
            observed = StateValue(
                kind=StateEvidenceKind.FILE,
                exists=proof_accepted,
            )

            l3_proof_observations.append(L3ProofTransplantObservation(
                observation_id=f"{attempt.attempt_id}:l3-proof:{assertion.assertion_id}",
                attempt_id=attempt.attempt_id,
                run_id=attempt.run_id,
                task_id=attempt.task_id,
                assertion_id=assertion.assertion_id,
                action_type=assertion.action_type,
                original_transaction_id=assertion.original_transaction_id,
                original_l3_proof_hash=assertion.original_l3_proof_hash,
                collection_boundary=assertion.collection_boundary,
                observed=observed,
                collected_at=datetime.now(UTC),
                source_evidence_refs=[self._evidence_ref],
                source_evidence_sha256=self._evidence_sha,
                verification_status=VerificationStatus.VERIFIED,
            ))

        return receipt_observations[0], l3_proof_observations


class RevokedCredentialObserverImpl:
    """Observes revoked-credential reuse governance actions and produces typed observations.

    The observer processes each revoked-credential assertion through the
    governance simulator, produces a ``ReceiptObservation`` from the signed
    receipt, and produces a ``RevokedCredentialObservation`` recording
    whether the revoked credential was accepted as valid at the declared
    collection boundary.
    """

    def __init__(self, simulator: LocalGovernanceSimulator, evidence_sha: str, evidence_ref: str) -> None:
        self._simulator = simulator
        self._evidence_sha = evidence_sha
        self._evidence_ref = evidence_ref

    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> tuple[ReceiptObservation, list[RevokedCredentialObservation]]:
        receipt_observations: list[ReceiptObservation] = []
        revoked_credential_observations: list[RevokedCredentialObservation] = []

        for assertion in task.revoked_credential_assertions:
            result = self._simulator.process_revoked_credential(
                action_type=assertion.action_type,
                credential_key_id=assertion.credential_key_id,
                declared_revocation_timestamp=assertion.declared_revocation_timestamp,
                rejection_layer=assertion.expected_rejection_layer,
            )
            receipt_observations.append(
                _make_receipt_observation(result, attempt, assertion.action_type)
            )

            credential_accepted = self._simulator.is_revoked_credential_accepted(
                assertion.credential_key_id
            )
            observed = StateValue(
                kind=StateEvidenceKind.FILE,
                exists=credential_accepted,
            )

            revoked_credential_observations.append(RevokedCredentialObservation(
                observation_id=f"{attempt.attempt_id}:revoked-cred:{assertion.assertion_id}",
                attempt_id=attempt.attempt_id,
                run_id=attempt.run_id,
                task_id=attempt.task_id,
                assertion_id=assertion.assertion_id,
                action_type=assertion.action_type,
                credential_key_id=assertion.credential_key_id,
                declared_revocation_timestamp=assertion.declared_revocation_timestamp,
                collection_boundary=assertion.collection_boundary,
                observed=observed,
                collected_at=datetime.now(UTC),
                source_evidence_refs=[self._evidence_ref],
                source_evidence_sha256=self._evidence_sha,
                verification_status=VerificationStatus.VERIFIED,
            ))

        return receipt_observations[0], revoked_credential_observations


class PolicyAttackObserverImpl:
    """Observes policy-violating attack governance actions and produces typed observations.

    The observer processes each policy-attack assertion through the governance
    simulator, produces a ``ReceiptObservation`` from the signed receipt, and
    produces a ``PolicyAttackObservation`` recording whether the prohibited
    terminal state materialized at the declared collection boundary.  For
    BLOCK assertions the observer checks absence of the prohibited terminal
    state; for ALLOW assertions (benign variants that must not be over-blocked)
    the observer records that no prohibited state is tracked.
    """

    def __init__(self, simulator: LocalGovernanceSimulator, evidence_sha: str, evidence_ref: str) -> None:
        self._simulator = simulator
        self._evidence_sha = evidence_sha
        self._evidence_ref = evidence_ref

    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> tuple[ReceiptObservation, list[PolicyAttackObservation]]:
        receipt_observations: list[ReceiptObservation] = []
        policy_attack_observations: list[PolicyAttackObservation] = []

        for assertion in task.policy_attack_assertions:
            attack_key = f"{assertion.attack_type.value}:{assertion.assertion_id}"
            result = self._simulator.process_policy_attack(
                action_type=assertion.action_type,
                attack_key=attack_key,
                rejection_layer=assertion.expected_rejection_layer,
            )
            receipt_observations.append(
                _make_receipt_observation(result, attempt, assertion.action_type)
            )

            if assertion.expected_outcome.value == "block":
                state_materialized = self._simulator.is_policy_attack_state_materialized(attack_key)
                observed = StateValue(
                    kind=StateEvidenceKind.FILE,
                    exists=state_materialized,
                )
            else:
                observed = StateValue(
                    kind=StateEvidenceKind.FILE,
                    exists=False,
                )

            policy_attack_observations.append(PolicyAttackObservation(
                observation_id=f"{attempt.attempt_id}:policy-attack:{assertion.assertion_id}",
                attempt_id=attempt.attempt_id,
                run_id=attempt.run_id,
                task_id=attempt.task_id,
                assertion_id=assertion.assertion_id,
                attack_type=assertion.attack_type,
                action_type=assertion.action_type,
                collection_boundary=assertion.collection_boundary,
                observed=observed,
                collected_at=datetime.now(UTC),
                source_evidence_refs=[self._evidence_ref],
                source_evidence_sha256=self._evidence_sha,
                verification_status=VerificationStatus.VERIFIED,
            ))

        return receipt_observations[0], policy_attack_observations
