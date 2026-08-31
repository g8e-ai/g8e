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
    DETERMINISTIC_STAGE_KIND_COMMITMENT_APPEND,
    DETERMINISTIC_STAGE_KIND_L1_DOCTRINE,
    DETERMINISTIC_STAGE_KIND_L3_NOTARY,
    DETERMINISTIC_STAGE_KIND_L4_VERIFICATION,
    DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
    DETERMINISTIC_STAGE_KIND_PROTOCOL_L2,
    DETERMINISTIC_STAGE_KIND_RECEIPT_PERSISTENCE,
    DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
    DETERMINISTIC_STAGE_OUTCOME_FAILED,
    DETERMINISTIC_STAGE_OUTCOME_NOT_REQUIRED,
    DETERMINISTIC_STAGE_OUTCOME_VERIFIED,
    EXECUTION_STATUS_COMPLETED,
    EXECUTION_STATUS_FAILED,
    L2_STATUS_NOT_REQUIRED,
    L2_STATUS_REQUIRED_VALID,
    L3_STATUS_NOT_REQUIRED,
    L3_STATUS_REQUIRED_VALID,
)

from g8e_evals.arms import Arm, GovernancePosture
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
    PostureObservation,
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


def _protocol_context(
    posture: GovernancePosture = GovernancePosture.L1_DOCTRINE,
) -> DeterministicGradingContext:
    context = _context()
    receipt = context.receipts[0].action_receipt
    receipt.status = EXECUTION_STATUS_COMPLETED
    receipt.l2_status = (
        L2_STATUS_REQUIRED_VALID
        if posture in {GovernancePosture.L2_CONSENSUS, GovernancePosture.L3_NOTARY}
        else L2_STATUS_NOT_REQUIRED
    )
    receipt.l3_status = (
        L3_STATUS_REQUIRED_VALID
        if posture == GovernancePosture.L3_NOTARY
        else L3_STATUS_NOT_REQUIRED
    )
    del receipt.deterministic_stage_evidence[:]
    kinds_and_outcomes = [
        (DETERMINISTIC_STAGE_KIND_L1_DOCTRINE, DETERMINISTIC_STAGE_OUTCOME_VERIFIED),
        (
            DETERMINISTIC_STAGE_KIND_PROTOCOL_L2,
            DETERMINISTIC_STAGE_OUTCOME_VERIFIED
            if posture in {GovernancePosture.L2_CONSENSUS, GovernancePosture.L3_NOTARY}
            else DETERMINISTIC_STAGE_OUTCOME_NOT_REQUIRED,
        ),
        (
            DETERMINISTIC_STAGE_KIND_L3_NOTARY,
            DETERMINISTIC_STAGE_OUTCOME_VERIFIED
            if posture == GovernancePosture.L3_NOTARY
            else DETERMINISTIC_STAGE_OUTCOME_NOT_REQUIRED,
        ),
        (DETERMINISTIC_STAGE_KIND_L4_VERIFICATION, DETERMINISTIC_STAGE_OUTCOME_VERIFIED),
        (DETERMINISTIC_STAGE_KIND_RECEIPT_PERSISTENCE, DETERMINISTIC_STAGE_OUTCOME_COMPLETED),
        (DETERMINISTIC_STAGE_KIND_COMMITMENT_APPEND, DETERMINISTIC_STAGE_OUTCOME_COMPLETED),
        (DETERMINISTIC_STAGE_KIND_L5_EXECUTION, DETERMINISTIC_STAGE_OUTCOME_COMPLETED),
    ]
    l4_id = "tx-1:l4"
    l5_id = "tx-1:l5"
    for index, (kind, outcome) in enumerate(kinds_and_outcomes):
        stage = receipt.deterministic_stage_evidence.add(
            stage_id=l4_id if kind == DETERMINISTIC_STAGE_KIND_L4_VERIFICATION else l5_id if kind == DETERMINISTIC_STAGE_KIND_L5_EXECUTION else f"tx-1:stage:{index}",
            kind=kind,
            outcome=outcome,
            transaction_id="tx-1",
            transaction_hash="hash-1",
            action_type="FILE_EDIT",
            operator_id="operator-1",
        )
        if kind in {
            DETERMINISTIC_STAGE_KIND_L1_DOCTRINE,
            DETERMINISTIC_STAGE_KIND_PROTOCOL_L2,
            DETERMINISTIC_STAGE_KIND_L3_NOTARY,
        }:
            stage.parent_stage_id = l4_id
        elif kind in {
            DETERMINISTIC_STAGE_KIND_L4_VERIFICATION,
            DETERMINISTIC_STAGE_KIND_RECEIPT_PERSISTENCE,
            DETERMINISTIC_STAGE_KIND_COMMITMENT_APPEND,
        }:
            stage.parent_stage_id = l5_id
        if kind == DETERMINISTIC_STAGE_KIND_L5_EXECUTION:
            stage.state_root_before = "root-before"
            stage.state_root_after = "root-after"
    task = context.task.model_copy(update={
        "grader_ids": ["protocol_chain"],
        "grader_versions": ["1.0.0"],
    })
    attempt = context.attempt.model_copy(update={
        "posture": PostureObservation(
            requested_posture=posture,
            observed_posture=posture,
            posture_match=True,
        ),
    })
    return DeterministicGradingContext(
        task=task,
        attempt=attempt,
        receipts=context.receipts,
        stages=context.stages,
    )


def test_protocol_chain_grader_verifies_signed_stage_graph_for_observed_posture():
    result = grade_deterministically(
        "protocol_chain",
        "1.0.0",
        _protocol_context(GovernancePosture.L3_NOTARY),
    )

    assert result.value == 1.0
    assert result.verification_status == VerificationStatus.VERIFIED
    assert result.evidence_refs == ["receipt-1"]


@pytest.mark.parametrize(
    ("tamper", "failure"),
    [
        ("order", "deterministic stage order is invalid"),
        ("action", "deterministic stage action does not match the receipt"),
        ("parent", "deterministic stage parent relationship is invalid"),
        ("outcome", "L2 stage outcome does not match the signed receipt status"),
        ("identity", "deterministic stage identity fields are inconsistent"),
        ("missing", "verified protocol chain is missing required stages"),
    ],
)
def test_protocol_chain_grader_fails_closed_on_invalid_signed_chain(
    tamper: str,
    failure: str,
):
    context = _protocol_context()
    stages = context.receipts[0].action_receipt.deterministic_stage_evidence
    if tamper == "order":
        first = type(stages[0])()
        first.CopyFrom(stages[0])
        second = type(stages[1])()
        second.CopyFrom(stages[1])
        del stages[:2]
        stages.extend([second, first])
    elif tamper == "action":
        stages[0].action_type = "EXECUTE_BASH"
    elif tamper == "parent":
        stages[0].parent_stage_id = "tx-1:l5"
    elif tamper == "outcome":
        stages[1].outcome = DETERMINISTIC_STAGE_OUTCOME_VERIFIED
    elif tamper == "identity":
        stages[1].operator_id = "operator-2"
    else:
        del stages[5]

    result = grade_deterministically("protocol_chain", "1.0.0", context)

    assert result.value == 0.0
    assert result.verification_status == VerificationStatus.FAILED
    assert result.failure == failure


def test_protocol_chain_grader_verifies_signed_rejection_prefix():
    context = _protocol_context()
    receipt = context.receipts[0].action_receipt
    receipt.status = EXECUTION_STATUS_FAILED
    del receipt.deterministic_stage_evidence[4:]
    receipt.deterministic_stage_evidence[-1].outcome = DETERMINISTIC_STAGE_OUTCOME_FAILED
    receipt.deterministic_stage_evidence[-1].parent_stage_id = ""

    result = grade_deterministically("protocol_chain", "1.0.0", context)

    assert result.value == 1.0
    assert result.verification_status == VerificationStatus.VERIFIED


def test_protocol_chain_grader_fails_closed_when_observed_posture_does_not_match_arm():
    context = _protocol_context()
    context.attempt.posture.posture_match = False

    result = grade_deterministically("protocol_chain", "1.0.0", context)

    assert result.value == 0.0
    assert result.verification_status == VerificationStatus.FAILED
    assert result.failure == "observed governance posture does not match the requested posture"


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
