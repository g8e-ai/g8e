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
   ``SignedFieldTamperingObservation``, ``NonceExpirationObservation``) that
   record whether the prohibited terminal state materialized at the declared
   collection boundary.
"""

from __future__ import annotations

from datetime import UTC, datetime

from g8e_evals.benchmarks.governance.simulator import GovernanceActionResult, LocalGovernanceSimulator
from g8e_evals.schema import (
    AttemptRecord,
    NonceExpirationObservation,
    ReceiptObservation,
    ReplayAttemptObservation,
    SignedFieldTamperingObservation,
    StateEvidenceKind,
    StateValue,
    TaskDefinition,
    VerificationStatus,
)


def _make_receipt_observation(
    result: GovernanceActionResult,
    attempt: AttemptRecord,
    action_type: str,
    evidence_sha: str,
    evidence_ref: str,
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


def _absent_file_state() -> StateValue:
    return StateValue(kind=StateEvidenceKind.FILE, exists=False)


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
                _make_receipt_observation(result, attempt, assertion.action_type, self._evidence_sha, self._evidence_ref)
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
                _make_receipt_observation(result, attempt, assertion.action_type, self._evidence_sha, self._evidence_ref)
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
                _make_receipt_observation(result, attempt, assertion.action_type, self._evidence_sha, self._evidence_ref)
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
