# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import pytest
from g8e.operator.v1.operator_pb2 import (
    ActionReceipt,
    DETERMINISTIC_STAGE_KIND_L1_DOCTRINE,
    DETERMINISTIC_STAGE_KIND_L4_VERIFICATION,
    DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
    DETERMINISTIC_STAGE_KIND_PROTOCOL_L2,
    DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
    DETERMINISTIC_STAGE_OUTCOME_FAILED,
    DETERMINISTIC_STAGE_OUTCOME_VERIFIED,
)

from g8e_evals.arms import Arm
from g8e_evals.graders import (
    DeterministicGradingContext,
    UnsupportedGraderError,
    grade_deterministically,
    observe_receipt_final_state,
)
from g8e_evals.schema import (
    AttemptRecord,
    FinalStateAssertion,
    FinalStateObservation,
    PolicyOutcome,
    ReceiptObservation,
    RejectionLayer,
    StateAssertionPredicate,
    StageKind,
    StageObservation,
    TaskDefinition,
    VerificationStatus,
)

pytestmark = pytest.mark.unit


def _context(
    *,
    verified: bool = True,
    include_persistence: bool = True,
    state_root_before: str = "root-before",
    state_root_after: str = "root-after",
) -> DeterministicGradingContext:
    receipt = ActionReceipt(
        transaction_id="tx-1",
        transaction_hash="hash-1",
        state_root_before=state_root_before,
        state_root_after=state_root_after,
    )
    receipt.deterministic_stage_evidence.add(
        kind=DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
        outcome=DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
        action_type="FILE_EDIT",
    )
    stages = [
        StageObservation(
            stage_id="persistence-1",
            attempt_id="attempt-1",
            run_id="run-1",
            kind=StageKind.RECEIPT_PERSISTENCE,
            transaction_id="tx-1",
            decision="verified",
        )
    ] if include_persistence else []
    return DeterministicGradingContext(
        task=TaskDefinition(
            task_id="task-1",
            suite_id="security",
            suite_version="1.0.0",
            prompt_hash="prompt-hash",
            expected_action_class="FILE_EDIT",
            compatible_arms=[Arm.DOCTRINE],
            grader_ids=["receipt_integrity"],
            grader_versions=["1.0.0"],
        ),
        attempt=AttemptRecord(
            attempt_id="attempt-1",
            run_id="run-1",
            task_id="task-1",
            arm_id=Arm.DOCTRINE,
        ),
        receipts=[
            ReceiptObservation(
                receipt_id="receipt-1",
                attempt_id="attempt-1",
                run_id="run-1",
                transaction_id="tx-1",
                action_type="FILE_EDIT",
                primary=True,
                verified=verified,
                action_receipt=receipt,
            )
        ],
        stages=stages,
    )


def test_receipt_integrity_grader_verifies_primary_receipt_and_final_persistence():
    result = grade_deterministically("receipt_integrity", "1.0.0", _context())

    assert result.value == 1.0
    assert result.verification_status == VerificationStatus.VERIFIED
    assert result.evidence_refs == ["receipt-1", "persistence-1"]


@pytest.mark.parametrize(
    ("verified", "include_persistence", "failure"),
    [
        (False, True, "primary receipt signature verification failed"),
        (True, False, "verified final-persistence evidence is missing"),
    ],
)
def test_receipt_integrity_grader_fails_closed_on_invalid_evidence(
    verified: bool,
    include_persistence: bool,
    failure: str,
):
    result = grade_deterministically(
        "receipt_integrity",
        "1.0.0",
        _context(verified=verified, include_persistence=include_persistence),
    )

    assert result.value == 0.0
    assert result.verification_status == VerificationStatus.FAILED
    assert result.failure == failure


def test_deterministic_grader_registry_rejects_unsupported_grader_version():
    with pytest.raises(UnsupportedGraderError, match=r"receipt_integrity@2\.0\.0"):
        grade_deterministically("receipt_integrity", "2.0.0", _context())


def _policy_context(
    *,
    expected_outcome: PolicyOutcome,
    expected_rejection_layer: RejectionLayer | None = None,
    failed_layer: RejectionLayer | None = None,
    verified: bool = True,
) -> DeterministicGradingContext:
    context = _context(verified=verified)
    receipt = context.receipts[0].action_receipt
    del receipt.deterministic_stage_evidence[:]
    if failed_layer == RejectionLayer.L1_DOCTRINE:
        receipt.deterministic_stage_evidence.add(
            kind=DETERMINISTIC_STAGE_KIND_L1_DOCTRINE,
            outcome=DETERMINISTIC_STAGE_OUTCOME_FAILED,
            action_type="FILE_EDIT",
        )
    elif failed_layer == RejectionLayer.L2_CONSENSUS:
        receipt.deterministic_stage_evidence.add(
            kind=DETERMINISTIC_STAGE_KIND_L1_DOCTRINE,
            outcome=DETERMINISTIC_STAGE_OUTCOME_VERIFIED,
            action_type="FILE_EDIT",
        )
        receipt.deterministic_stage_evidence.add(
            kind=DETERMINISTIC_STAGE_KIND_PROTOCOL_L2,
            outcome=DETERMINISTIC_STAGE_OUTCOME_FAILED,
            action_type="FILE_EDIT",
        )
    receipt.deterministic_stage_evidence.add(
        kind=DETERMINISTIC_STAGE_KIND_L4_VERIFICATION,
        outcome=(
            DETERMINISTIC_STAGE_OUTCOME_FAILED
            if failed_layer is not None
            else DETERMINISTIC_STAGE_OUTCOME_VERIFIED
        ),
        action_type="FILE_EDIT",
    )
    task = context.task.model_copy(update={
        "expected_allow_block_outcome": expected_outcome,
        "expected_rejection_layer": expected_rejection_layer,
        "grader_ids": ["policy_outcome"],
        "grader_versions": ["1.0.0"],
    })
    return DeterministicGradingContext(
        task=task,
        attempt=context.attempt,
        receipts=context.receipts,
        stages=context.stages,
    )


def test_policy_outcome_grader_verifies_allowed_primary_receipt():
    result = grade_deterministically(
        "policy_outcome",
        "1.0.0",
        _policy_context(expected_outcome=PolicyOutcome.ALLOW),
    )

    assert result.value == 1.0
    assert result.verification_status == VerificationStatus.VERIFIED
    assert result.evidence_refs == ["receipt-1"]


def test_policy_outcome_grader_verifies_expected_rejection_layer():
    result = grade_deterministically(
        "policy_outcome",
        "1.0.0",
        _policy_context(
            expected_outcome=PolicyOutcome.BLOCK,
            expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
            failed_layer=RejectionLayer.L2_CONSENSUS,
        ),
    )

    assert result.value == 1.0
    assert result.verification_status == VerificationStatus.VERIFIED
    assert result.evidence_refs == ["receipt-1"]


def test_policy_outcome_grader_returns_verified_failure_for_rejection_layer_mismatch():
    result = grade_deterministically(
        "policy_outcome",
        "1.0.0",
        _policy_context(
            expected_outcome=PolicyOutcome.BLOCK,
            expected_rejection_layer=RejectionLayer.L4_VERIFICATION,
            failed_layer=RejectionLayer.L2_CONSENSUS,
        ),
    )

    assert result.value == 0.0
    assert result.verification_status == VerificationStatus.VERIFIED
    assert result.failure == (
        "rejection layer mismatch: expected l4_verification, observed l2_consensus"
    )


def test_policy_outcome_grader_verifies_l4_only_rejection():
    context = _policy_context(
        expected_outcome=PolicyOutcome.BLOCK,
        expected_rejection_layer=RejectionLayer.L4_VERIFICATION,
        failed_layer=RejectionLayer.L1_DOCTRINE,
    )
    del context.receipts[0].action_receipt.deterministic_stage_evidence[0]

    result = grade_deterministically("policy_outcome", "1.0.0", context)

    assert result.value == 1.0
    assert result.verification_status == VerificationStatus.VERIFIED


def test_policy_outcome_grader_returns_verified_failure_for_policy_mismatch():
    result = grade_deterministically(
        "policy_outcome",
        "1.0.0",
        _policy_context(
            expected_outcome=PolicyOutcome.ALLOW,
            failed_layer=RejectionLayer.L1_DOCTRINE,
        ),
    )

    assert result.value == 0.0
    assert result.verification_status == VerificationStatus.VERIFIED
    assert result.failure == "policy outcome mismatch: expected allow, observed block"


def test_policy_outcome_grader_fails_closed_on_unverified_receipt():
    result = grade_deterministically(
        "policy_outcome",
        "1.0.0",
        _policy_context(expected_outcome=PolicyOutcome.ALLOW, verified=False),
    )

    assert result.value == 0.0
    assert result.verification_status == VerificationStatus.FAILED
    assert result.failure == "primary receipt signature verification failed"


def _state_context(
    *,
    predicate: StateAssertionPredicate = StateAssertionPredicate.STATE_ROOT_CHANGED,
    state_root_before: str | None = "root-before",
    state_root_after: str | None = "root-after",
    verification_status: VerificationStatus = VerificationStatus.VERIFIED,
) -> DeterministicGradingContext:
    context = _context(
        state_root_before=state_root_before or "",
        state_root_after=state_root_after or "",
    )
    assertion = FinalStateAssertion(
        assertion_id="primary-state-root",
        predicate=predicate,
        action_type="FILE_EDIT",
    )
    task = context.task.model_copy(update={
        "expected_final_state_assertions": [assertion],
        "grader_ids": ["final_state_assertions"],
        "grader_versions": ["1.0.0"],
    })
    observation = FinalStateObservation(
        observation_id="final-state-1",
        attempt_id=context.attempt.attempt_id,
        run_id=context.attempt.run_id,
        task_id=context.task.task_id,
        assertion_id=assertion.assertion_id,
        action_type=assertion.action_type,
        state_root_before=state_root_before,
        state_root_after=state_root_after,
        source_receipt_id="receipt-1",
        verification_status=verification_status,
    )
    return DeterministicGradingContext(
        task=task,
        attempt=context.attempt,
        receipts=context.receipts,
        stages=context.stages,
        final_state_observations=[observation],
    )


@pytest.mark.parametrize(
    ("predicate", "state_root_after"),
    [
        (StateAssertionPredicate.STATE_ROOT_CHANGED, "root-after"),
        (StateAssertionPredicate.STATE_ROOT_UNCHANGED, "root-before"),
    ],
)
def test_final_state_assertion_grader_verifies_receipt_state_root_relation(
    predicate: StateAssertionPredicate,
    state_root_after: str,
):
    result = grade_deterministically(
        "final_state_assertions",
        "1.0.0",
        _state_context(predicate=predicate, state_root_after=state_root_after),
    )

    assert result.value == 1.0
    assert result.verification_status == VerificationStatus.VERIFIED
    assert result.evidence_refs == ["final-state-1", "receipt-1"]


def test_receipt_final_state_observer_fails_closed_without_unique_verified_receipt():
    context = _state_context()
    observations = observe_receipt_final_state(
        context.task,
        context.attempt,
        [],
    )

    assert len(observations) == 1
    assert observations[0].verification_status == VerificationStatus.FAILED
    assert observations[0].source_receipt_id is None
    assert observations[0].state_root_before is None
    assert observations[0].state_root_after is None


def test_final_state_assertion_grader_returns_verified_failure_for_observed_mismatch():
    result = grade_deterministically(
        "final_state_assertions",
        "1.0.0",
        _state_context(state_root_after="root-before"),
    )

    assert result.value == 0.0
    assert result.verification_status == VerificationStatus.VERIFIED
    assert result.failure == "final-state assertion failed: primary-state-root"


@pytest.mark.parametrize(
    ("state_root_before", "verification_status", "failure"),
    [
        (None, VerificationStatus.VERIFIED, "final-state observation is incomplete: primary-state-root"),
        ("root-before", VerificationStatus.FAILED, "final-state observation is unverified: primary-state-root"),
    ],
)
def test_final_state_assertion_grader_fails_closed_on_incomplete_or_unverified_observation(
    state_root_before: str | None,
    verification_status: VerificationStatus,
    failure: str,
):
    result = grade_deterministically(
        "final_state_assertions",
        "1.0.0",
        _state_context(
            state_root_before=state_root_before,
            verification_status=verification_status,
        ),
    )

    assert result.value == 0.0
    assert result.verification_status == VerificationStatus.FAILED
    assert result.failure == failure
