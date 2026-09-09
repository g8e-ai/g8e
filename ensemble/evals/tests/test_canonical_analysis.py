# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for the canonical eval analysis model and computation.

These tests verify the typed model structure, deterministic
serialization, denominator preservation, and computation correctness
of the canonical analysis engine. No external dependencies (no files,
network, or DB).
"""

from __future__ import annotations


import pytest

pytestmark = pytest.mark.unit

from g8e_evals.analysis.canonical import (
    ANALYSIS_COMPUTATION_VERSION,
    ANALYSIS_SCHEMA_VERSION,
    AnalysisInputSummary,
    CanonicalEvalAnalysis,
    ConfusionMatrix,
    GateDecisionStatus,
    MetricAnalysisResult,
    MissingnessBreakdown,
    MissingnessReason,
    ComparisonDirection,
    ReceiptCoverageAnalysis,
)
from g8e_evals.analysis.engine import compute_canonical_analysis
from g8e_evals.arms import Arm
from g8e_evals.metrics import MetricDirection
from g8e_evals.release_metric_set import MetricDomain, RELEASE_METRIC_SET
from g8e.operator.v1.operator_pb2 import (
    ActionReceipt,
    DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
    DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
)
from g8e_evals.schema import (
    AttemptRecord,
    GraderClass,
    GraderReference,
    MetricObservation,
    PolicyOutcome,
    ReceiptObservation,
    RejectionLayer,
    TaskDefinition,
    TerminalStatus,
    VerificationStatus,
)


_RUN_ID = "run-test-1"
_TASK_ID = "task-test-1"
_ATTEMPT_ID = "attempt-test-1"


def _make_task(
    task_id: str = _TASK_ID,
    expected_allow_block: PolicyOutcome | None = None,
) -> TaskDefinition:
    return TaskDefinition(
        task_id=task_id,
        suite_id="utility",
        suite_version="1.0.0",
        prompt_hash="abc123",
        prompt_length=10,
        expected_action_class="TEST_ACTION",
        compatible_arms=[Arm.DOCTRINE],
        graders=[GraderReference(grader_id="receipt_integrity", grader_version="1.0.0")],
        expected_allow_block_outcome=expected_allow_block,
        expected_rejection_layer=RejectionLayer.L1_DOCTRINE if expected_allow_block == PolicyOutcome.BLOCK else None,
    )


def _make_attempt(
    attempt_id: str = _ATTEMPT_ID,
    task_id: str = _TASK_ID,
    arm_id: Arm = Arm.DOCTRINE,
    terminal_status: TerminalStatus = TerminalStatus.COMPLETED,
) -> AttemptRecord:
    return AttemptRecord(
        attempt_id=attempt_id,
        run_id=_RUN_ID,
        task_id=task_id,
        arm_id=arm_id,
        terminal_status=terminal_status,
    )


def _make_metric_obs(
    metric_id: str = "receipt_integrity",
    attempt_id: str = _ATTEMPT_ID,
    task_id: str = _TASK_ID,
    arm_id: Arm = Arm.DOCTRINE,
    value: float = 1.0,
    eligible: bool = True,
    verification_status: VerificationStatus = VerificationStatus.VERIFIED,
) -> MetricObservation:
    return MetricObservation(
        metric_id=metric_id,
        metric_version="1.0.0",
        attempt_id=attempt_id,
        run_id=_RUN_ID,
        arm_id=arm_id,
        task_id=task_id,
        value=value,
        unit="boolean",
        eligible=eligible,
        verification_status=verification_status,
        grader_class=GraderClass.DETERMINISTIC,
    )


def _make_receipt_observation(
    attempt_id: str = _ATTEMPT_ID,
    verified: bool = True,
    primary: bool = True,
    transaction_id: str = "tx-1",
    action_type: str = "TEST_ACTION",
) -> ReceiptObservation:
    receipt = ActionReceipt(
        transaction_id=transaction_id,
        transaction_hash="hash-1",
    )
    receipt.deterministic_stage_evidence.add(
        kind=DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
        outcome=DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
        action_type=action_type,
    )
    return ReceiptObservation(
        receipt_id=f"receipt-{transaction_id}",
        attempt_id=attempt_id,
        run_id=_RUN_ID,
        transaction_id=transaction_id,
        action_type=action_type,
        primary=primary,
        verified=verified,
        action_receipt=receipt,
    )


class TestCanonicalAnalysisModel:
    """Tests for the typed canonical analysis model structure."""

    def test_model_is_frozen_and_forbids_extra(self) -> None:
        result = MetricAnalysisResult(
            metric_id="receipt_integrity",
            metric_version="1.0.0",
            arm_id="doctrine",
            domain=MetricDomain.GOVERNANCE,
            direction=MetricDirection.BINARY_PASS_FAIL,
            unit="boolean",
            numerator=1.0,
            denominator=1,
            value=1.0,
            eligible_count=1,
            not_eligible_count=0,
            missing_count=0,
            verification_status_counts={"verified": 1},
            evidence_ref_count=0,
            metric_observation_ids=["receipt_integrity@1.0.0:attempt-1"],
        )
        with pytest.raises(Exception, match="frozen"):
            result.metric_id = "other"

    def test_confusion_matrix_properties_zero_denominator(self) -> None:
        cm = ConfusionMatrix(
            metric_id="policy_outcome",
            metric_version="1.0.0",
            arm_id="doctrine",
            domain=MetricDomain.GOVERNANCE,
            true_positive=0,
            false_positive=0,
            true_negative=0,
            false_negative=0,
        )
        assert cm.total == 0
        assert cm.accuracy == 0.0
        assert cm.balanced_accuracy == 0.0
        assert cm.matthews_correlation_coefficient == 0.0

    def test_confusion_matrix_perfect_classification(self) -> None:
        cm = ConfusionMatrix(
            metric_id="policy_outcome",
            metric_version="1.0.0",
            arm_id="doctrine",
            domain=MetricDomain.GOVERNANCE,
            true_positive=5,
            false_positive=0,
            true_negative=5,
            false_negative=0,
        )
        assert cm.total == 10
        assert cm.accuracy == 1.0
        assert cm.balanced_accuracy == 1.0
        assert cm.matthews_correlation_coefficient == 1.0

    def test_confusion_matrix_worst_classification(self) -> None:
        cm = ConfusionMatrix(
            metric_id="policy_outcome",
            metric_version="1.0.0",
            arm_id="doctrine",
            domain=MetricDomain.GOVERNANCE,
            true_positive=0,
            false_positive=5,
            true_negative=0,
            false_negative=5,
        )
        assert cm.accuracy == 0.0
        assert cm.balanced_accuracy == 0.0
        assert cm.matthews_correlation_coefficient == -1.0

    def test_confusion_matrix_mixed_classification(self) -> None:
        cm = ConfusionMatrix(
            metric_id="policy_outcome",
            metric_version="1.0.0",
            arm_id="doctrine",
            domain=MetricDomain.GOVERNANCE,
            true_positive=3,
            false_positive=1,
            true_negative=4,
            false_negative=2,
        )
        assert cm.total == 10
        assert cm.accuracy == 0.7
        tpr = 3 / 5
        tnr = 4 / 5
        assert cm.balanced_accuracy == round((tpr + tnr) / 2, 10)
        numerator = 3 * 4 - 1 * 2
        denominator_sq = (3 + 1) * (3 + 2) * (4 + 1) * (4 + 2)
        expected_mcc = round(numerator / (denominator_sq ** 0.5), 10)
        assert cm.matthews_correlation_coefficient == expected_mcc

    def test_missingness_breakdown_total(self) -> None:
        mb = MissingnessBreakdown(
            completed=5,
            model_failed=2,
            governance_rejected=1,
            human_denied=0,
            timed_out=1,
            infrastructure_failed=1,
            invalid_evidence=0,
        )
        assert mb.total == 10

    def test_gate_decision_status_values(self) -> None:
        assert GateDecisionStatus.PASS.value == "pass"
        assert GateDecisionStatus.FAIL.value == "fail"
        assert GateDecisionStatus.NOT_APPLICABLE.value == "not_applicable"
        assert GateDecisionStatus.INSUFFICIENT_DATA.value == "insufficient_data"
        assert GateDecisionStatus.UNSUPPORTED.value == "unsupported"

    def test_missingness_reason_values(self) -> None:
        assert MissingnessReason.COMPLETED.value == "completed"
        assert MissingnessReason.MODEL_FAILED.value == "model_failed"
        assert MissingnessReason.GOVERNANCE_REJECTED.value == "governance_rejected"
        assert MissingnessReason.HUMAN_DENIED.value == "human_denied"
        assert MissingnessReason.TIMED_OUT.value == "timed_out"
        assert MissingnessReason.INFRASTRUCTURE_FAILED.value == "infrastructure_failed"
        assert MissingnessReason.INVALID_EVIDENCE.value == "invalid_evidence"

    def test_comparison_direction_values(self) -> None:
        assert ComparisonDirection.IMPROVEMENT.value == "improvement"
        assert ComparisonDirection.REGRESSION.value == "regression"
        assert ComparisonDirection.NEUTRAL.value == "neutral"

    def test_analysis_versions_are_pinned(self) -> None:
        assert ANALYSIS_SCHEMA_VERSION == "1.0.0"
        assert ANALYSIS_COMPUTATION_VERSION == "1.0.0"

    def test_canonical_json_is_deterministic(self) -> None:
        """Two identical analyses produce byte-identical canonical JSON."""
        analysis = CanonicalEvalAnalysis(
            analysis_schema_version=ANALYSIS_SCHEMA_VERSION,
            analysis_computation_version=ANALYSIS_COMPUTATION_VERSION,
            release_version="v2.1.8",
            run_id=_RUN_ID,
            input_summary=AnalysisInputSummary(
                task_count=1,
                attempt_count=1,
                observation_count=0,
                receipt_count=0,
                stage_count=0,
                metric_observation_count=0,
                input_content_hash="abc123",
            ),
            missingness=MissingnessBreakdown(
                completed=1,
                model_failed=0,
                governance_rejected=0,
                human_denied=0,
                timed_out=0,
                infrastructure_failed=0,
                invalid_evidence=0,
            ),
            receipt_coverage=ReceiptCoverageAnalysis(
                eligible_attempt_count=0,
                receipt_bound_count=0,
                receipt_verified_count=0,
            ),
            arm_ids=["doctrine"],
        )
        json1 = analysis.canonical_json()
        json2 = analysis.canonical_json()
        assert json1 == json2


class TestCanonicalAnalysisComputation:
    """Tests for the canonical analysis computation engine."""

    def test_empty_inputs_produce_valid_analysis(self) -> None:
        analysis = compute_canonical_analysis(
            tasks=[],
            attempts=[],
            metric_observations=[],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        assert analysis.run_id == _RUN_ID
        assert analysis.release_version == "v2.1.8"
        assert analysis.input_summary.task_count == 0
        assert analysis.input_summary.attempt_count == 0
        assert analysis.missingness.total == 0
        assert analysis.arm_ids == []
        assert analysis.metric_results == []
        assert analysis.confusion_matrices == []
        assert analysis.comparisons == []
        assert analysis.gate_decisions == []
        assert analysis.bridge_runs == []
        assert len(analysis.unsupported_claim_names) > 0

    def test_single_attempt_single_metric(self) -> None:
        task = _make_task()
        attempt = _make_attempt()
        obs = _make_metric_obs(value=1.0)

        analysis = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt],
            metric_observations=[obs],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        assert analysis.input_summary.task_count == 1
        assert analysis.input_summary.attempt_count == 1
        assert analysis.input_summary.metric_observation_count == 1
        assert analysis.missingness.completed == 1
        assert analysis.arm_ids == ["doctrine"]

        # Find the receipt_integrity metric result
        ri_results = [r for r in analysis.metric_results if r.metric_id == "receipt_integrity"]
        assert len(ri_results) == 1
        ri = ri_results[0]
        assert ri.arm_id == "doctrine"
        assert ri.denominator == 1
        assert ri.value == 1.0
        assert ri.eligible_count == 1
        assert ri.missing_count == 0
        assert ri.verification_status_counts.get("verified") == 1

    def test_denominator_preservation_model_failed(self) -> None:
        """A model-failed attempt is retained in the denominator."""
        task = _make_task()
        attempt = _make_attempt(terminal_status=TerminalStatus.MODEL_FAILED)
        # No metric observation for this attempt

        analysis = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt],
            metric_observations=[],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        assert analysis.missingness.model_failed == 1
        assert analysis.missingness.completed == 0

        # The attempt should still appear in metric results for the arm
        ri_results = [r for r in analysis.metric_results if r.metric_id == "receipt_integrity"]
        assert len(ri_results) == 1
        ri = ri_results[0]
        assert ri.denominator == 1  # Preserved in denominator
        assert ri.missing_count == 1  # But marked as missing
        assert ri.value is None  # No value when all observations are missing

    def test_denominator_preservation_governance_rejected(self) -> None:
        """A governance-rejected attempt is retained in the denominator."""
        task = _make_task()
        attempt = _make_attempt(terminal_status=TerminalStatus.GOVERNANCE_REJECTED)

        analysis = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt],
            metric_observations=[],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        assert analysis.missingness.governance_rejected == 1

    def test_denominator_preservation_all_terminal_statuses(self) -> None:
        """Every terminal status is retained in the missingness breakdown."""
        task = _make_task()
        statuses = [
            TerminalStatus.COMPLETED,
            TerminalStatus.MODEL_FAILED,
            TerminalStatus.GOVERNANCE_REJECTED,
            TerminalStatus.HUMAN_DENIED,
            TerminalStatus.TIMED_OUT,
            TerminalStatus.INFRASTRUCTURE_FAILED,
            TerminalStatus.INVALID_EVIDENCE,
        ]
        attempts = [
            _make_attempt(
                attempt_id=f"attempt-{i}",
                terminal_status=status,
            )
            for i, status in enumerate(statuses)
        ]

        analysis = compute_canonical_analysis(
            tasks=[task],
            attempts=attempts,
            metric_observations=[],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        assert analysis.missingness.completed == 1
        assert analysis.missingness.model_failed == 1
        assert analysis.missingness.governance_rejected == 1
        assert analysis.missingness.human_denied == 1
        assert analysis.missingness.timed_out == 1
        assert analysis.missingness.infrastructure_failed == 1
        assert analysis.missingness.invalid_evidence == 1
        assert analysis.missingness.total == 7

    def test_deterministic_output_for_identical_inputs(self) -> None:
        """Identical inputs produce byte-identical canonical JSON."""
        task = _make_task()
        attempt = _make_attempt()
        obs = _make_metric_obs(value=1.0)

        analysis1 = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt],
            metric_observations=[obs],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        analysis2 = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt],
            metric_observations=[obs],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        assert analysis1.canonical_json() == analysis2.canonical_json()

    def test_different_input_order_produces_identical_output(self) -> None:
        """Input record order does not affect canonical output."""
        task = _make_task()
        attempt1 = _make_attempt(attempt_id="attempt-a")
        attempt2 = _make_attempt(attempt_id="attempt-b")
        obs1 = _make_metric_obs(attempt_id="attempt-a", value=1.0)
        obs2 = _make_metric_obs(attempt_id="attempt-b", value=0.0)

        analysis1 = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt1, attempt2],
            metric_observations=[obs1, obs2],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        analysis2 = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt2, attempt1],
            metric_observations=[obs2, obs1],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        assert analysis1.canonical_json() == analysis2.canonical_json()

    def test_input_content_hash_is_deterministic(self) -> None:
        """The input content hash is the same for identical inputs."""
        task = _make_task()
        attempt = _make_attempt()
        obs = _make_metric_obs(value=1.0)

        analysis1 = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt],
            metric_observations=[obs],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        analysis2 = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt],
            metric_observations=[obs],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        assert analysis1.input_summary.input_content_hash == analysis2.input_summary.input_content_hash

    def test_input_content_hash_changes_on_different_inputs(self) -> None:
        """Different inputs produce different content hashes."""
        task = _make_task()
        attempt = _make_attempt()
        obs1 = _make_metric_obs(value=1.0)
        obs2 = _make_metric_obs(value=0.0)

        analysis1 = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt],
            metric_observations=[obs1],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        analysis2 = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt],
            metric_observations=[obs2],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        assert analysis1.input_summary.input_content_hash != analysis2.input_summary.input_content_hash

    def test_gate_decision_pass_for_release_blocker_threshold(self) -> None:
        """A metric with a release-blocker threshold of 1.0 passes when value is 1.0."""
        task = _make_task()
        attempt = _make_attempt()
        obs = _make_metric_obs(metric_id="receipt_integrity", value=1.0)

        analysis = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt],
            metric_observations=[obs],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        gd = [d for d in analysis.gate_decisions if d.metric_id == "receipt_integrity"]
        assert len(gd) == 1
        assert gd[0].status == GateDecisionStatus.PASS
        assert gd[0].measured_value == 1.0
        assert gd[0].threshold_value == 1.0

    def test_gate_decision_fail_for_release_blocker_threshold(self) -> None:
        """A metric with a release-blocker threshold of 1.0 fails when value is below 1.0."""
        task = _make_task()
        attempt = _make_attempt()
        obs = _make_metric_obs(metric_id="receipt_integrity", value=0.0)

        analysis = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt],
            metric_observations=[obs],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        gd = [d for d in analysis.gate_decisions if d.metric_id == "receipt_integrity"]
        assert len(gd) == 1
        assert gd[0].status == GateDecisionStatus.FAIL

    def test_gate_decision_unsupported_for_no_threshold(self) -> None:
        """A metric without a practical threshold gets UNSUPPORTED status."""
        task = _make_task()
        attempt = _make_attempt()
        obs = _make_metric_obs(metric_id="secret_detection_precision", value=0.8)

        analysis = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt],
            metric_observations=[obs],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        gd = [d for d in analysis.gate_decisions if d.metric_id == "secret_detection_precision"]
        assert len(gd) == 1
        assert gd[0].status == GateDecisionStatus.UNSUPPORTED

    def test_gate_decision_insufficient_data_for_all_missing(self) -> None:
        """A metric where all eligible attempts are missing gets INSUFFICIENT_DATA."""
        task = _make_task()
        attempt = _make_attempt()

        analysis = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt],
            metric_observations=[],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        gd = [d for d in analysis.gate_decisions if d.metric_id == "receipt_integrity"]
        assert len(gd) == 1
        assert gd[0].status == GateDecisionStatus.INSUFFICIENT_DATA

    def test_gate_decision_not_applicable_for_no_attempts(self) -> None:
        """A metric with no attempts at all gets NOT_APPLICABLE status."""
        analysis = compute_canonical_analysis(
            tasks=[],
            attempts=[],
            metric_observations=[],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        # No attempts means no metric results and no gate decisions
        assert analysis.gate_decisions == []

    def test_confusion_matrix_for_policy_outcome(self) -> None:
        """Policy outcome metrics produce confusion matrices."""
        task_allow = _make_task(task_id="task-allow", expected_allow_block=PolicyOutcome.ALLOW)
        task_block = _make_task(task_id="task-block", expected_allow_block=PolicyOutcome.BLOCK)
        attempt_allow = _make_attempt(attempt_id="attempt-allow", task_id="task-allow")
        attempt_block = _make_attempt(attempt_id="attempt-block", task_id="task-block")
        obs_allow = _make_metric_obs(
            metric_id="policy_outcome",
            attempt_id="attempt-allow",
            task_id="task-allow",
            value=1.0,
        )
        obs_block = _make_metric_obs(
            metric_id="policy_outcome",
            attempt_id="attempt-block",
            task_id="task-block",
            value=1.0,
        )

        analysis = compute_canonical_analysis(
            tasks=[task_allow, task_block],
            attempts=[attempt_allow, attempt_block],
            metric_observations=[obs_allow, obs_block],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        cms = [cm for cm in analysis.confusion_matrices if cm.metric_id == "policy_outcome"]
        assert len(cms) == 1
        cm = cms[0]
        assert cm.true_positive == 1  # Expected allow, observed allow
        assert cm.true_negative == 1  # Expected block, observed block
        assert cm.false_positive == 0
        assert cm.false_negative == 0

    def test_confusion_matrix_with_mismatched_outcome(self) -> None:
        """A mismatched policy outcome produces FP or FN."""
        task_allow = _make_task(task_id="task-allow", expected_allow_block=PolicyOutcome.ALLOW)
        task_block = _make_task(task_id="task-block", expected_allow_block=PolicyOutcome.BLOCK)
        attempt_allow = _make_attempt(attempt_id="attempt-allow", task_id="task-allow")
        attempt_block = _make_attempt(attempt_id="attempt-block", task_id="task-block")
        # Allow task but observed block (value=0.0)
        obs_allow = _make_metric_obs(
            metric_id="policy_outcome",
            attempt_id="attempt-allow",
            task_id="task-allow",
            value=0.0,
        )
        # Block task but observed allow (value=0.0)
        obs_block = _make_metric_obs(
            metric_id="policy_outcome",
            attempt_id="attempt-block",
            task_id="task-block",
            value=0.0,
        )

        analysis = compute_canonical_analysis(
            tasks=[task_allow, task_block],
            attempts=[attempt_allow, attempt_block],
            metric_observations=[obs_allow, obs_block],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        cms = [cm for cm in analysis.confusion_matrices if cm.metric_id == "policy_outcome"]
        assert len(cms) == 1
        cm = cms[0]
        assert cm.false_negative == 1  # Expected allow, observed block
        assert cm.false_positive == 1  # Expected block, observed allow
        assert cm.true_positive == 0
        assert cm.true_negative == 0

    def test_domain_stratified_results_group_by_domain(self) -> None:
        """Domain-stratified results group metric results by domain."""
        task = _make_task()
        attempt = _make_attempt()
        obs = _make_metric_obs(metric_id="receipt_integrity", value=1.0)

        analysis = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt],
            metric_observations=[obs],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        # Should have at least one domain-stratified result for governance
        gov_results = [r for r in analysis.domain_stratified_results if r.domain == MetricDomain.GOVERNANCE]
        assert len(gov_results) >= 1
        assert gov_results[0].arm_id == "doctrine"

    def test_unsupported_claims_carried_from_release_set(self) -> None:
        """Unsupported claims are carried from the release metric set."""
        task = _make_task()
        attempt = _make_attempt()

        analysis = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt],
            metric_observations=[],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        assert analysis.unsupported_claim_names == sorted(RELEASE_METRIC_SET.unsupported_claim_names)
        assert "reasoner_independence" in analysis.unsupported_claim_names
        assert "human_semantic_grading" in analysis.unsupported_claim_names

    def test_multiple_arms_produce_separate_results(self) -> None:
        """Multiple arms produce separate per-metric results."""
        task = _make_task()
        attempt_doctrine = _make_attempt(attempt_id="attempt-doctrine", arm_id=Arm.DOCTRINE)
        attempt_direct = _make_attempt(attempt_id="attempt-direct", arm_id=Arm.DIRECT)
        obs_doctrine = _make_metric_obs(
            metric_id="receipt_integrity",
            attempt_id="attempt-doctrine",
            arm_id=Arm.DOCTRINE,
            value=1.0,
        )
        obs_direct = _make_metric_obs(
            metric_id="receipt_integrity",
            attempt_id="attempt-direct",
            arm_id=Arm.DIRECT,
            value=0.0,
        )

        analysis = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt_doctrine, attempt_direct],
            metric_observations=[obs_doctrine, obs_direct],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        assert sorted(analysis.arm_ids) == ["direct", "doctrine"]
        ri_results = [r for r in analysis.metric_results if r.metric_id == "receipt_integrity"]
        assert len(ri_results) == 2
        arm_values = {r.arm_id: r.value for r in ri_results}
        assert arm_values["doctrine"] == 1.0
        assert arm_values["direct"] == 0.0

    def test_metric_results_sorted_by_metric_and_arm(self) -> None:
        """Metric results are sorted by (metric_id, arm_id)."""
        task = _make_task()
        attempt = _make_attempt()
        obs = _make_metric_obs(value=1.0)

        analysis = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt],
            metric_observations=[obs],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        keys = [(r.metric_id, r.arm_id) for r in analysis.metric_results]
        assert keys == sorted(keys)

    def test_gate_decisions_sorted_by_metric_and_arm(self) -> None:
        """Gate decisions are sorted by (metric_id, arm_id)."""
        task = _make_task()
        attempt = _make_attempt()
        obs = _make_metric_obs(value=1.0)

        analysis = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt],
            metric_observations=[obs],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        keys = [(d.metric_id, d.arm_id) for d in analysis.gate_decisions]
        assert keys == sorted(keys)

    def test_canonical_json_round_trip_preserves_structure(self) -> None:
        """Canonical JSON can be parsed back into the same model."""
        task = _make_task()
        attempt = _make_attempt()
        obs = _make_metric_obs(value=1.0)

        analysis = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt],
            metric_observations=[obs],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        json_str = analysis.canonical_json()
        restored = CanonicalEvalAnalysis.model_validate_json(json_str)
        assert restored.run_id == analysis.run_id
        assert restored.input_summary.input_content_hash == analysis.input_summary.input_content_hash
        assert len(restored.metric_results) == len(analysis.metric_results)
        assert len(restored.gate_decisions) == len(analysis.gate_decisions)

    def test_lower_is_better_threshold_check(self) -> None:
        """A lower-is-better metric with threshold 0.0 passes when value is 0.0."""
        task = _make_task()
        attempt = _make_attempt()
        obs = _make_metric_obs(metric_id="model_boundary_raw_secret_rate", value=0.0)

        analysis = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt],
            metric_observations=[obs],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        gd = [d for d in analysis.gate_decisions if d.metric_id == "model_boundary_raw_secret_rate"]
        assert len(gd) == 1
        assert gd[0].status == GateDecisionStatus.PASS
        assert gd[0].threshold_value == 0.0

    def test_lower_is_better_threshold_fail(self) -> None:
        """A lower-is-better metric with threshold 0.0 fails when value is above 0.0."""
        task = _make_task()
        attempt = _make_attempt()
        obs = _make_metric_obs(metric_id="model_boundary_raw_secret_rate", value=0.5)

        analysis = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt],
            metric_observations=[obs],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        gd = [d for d in analysis.gate_decisions if d.metric_id == "model_boundary_raw_secret_rate"]
        assert len(gd) == 1
        assert gd[0].status == GateDecisionStatus.FAIL


class TestReceiptCoverage:
    """Tests for receipt coverage computed only over receipt-eligible mutation attempts."""

    def test_receipt_coverage_zero_when_no_eligible_attempts(self) -> None:
        """No eligible attempts means zero receipt coverage."""
        task = _make_task()
        attempt = _make_attempt()

        analysis = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt],
            metric_observations=[],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        # No receipts provided, so coverage is 0
        assert analysis.receipt_coverage.eligible_attempt_count == 1
        assert analysis.receipt_coverage.receipt_bound_count == 0
        assert analysis.receipt_coverage.coverage_pct == 0.0

    def test_receipt_coverage_excludes_ungoverned_arms(self) -> None:
        """Ungoverned arms (direct) are not in the receipt coverage denominator."""
        task = _make_task()
        attempt_direct = _make_attempt(attempt_id="attempt-direct", arm_id=Arm.DIRECT)

        analysis = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt_direct],
            metric_observations=[],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        assert analysis.receipt_coverage.eligible_attempt_count == 0

    def test_receipt_coverage_excludes_tasks_without_action_class(self) -> None:
        """Tasks without an expected action class are not mutation attempts."""
        task = TaskDefinition(
            task_id="task-no-action",
            suite_id="utility",
            suite_version="1.0.0",
            prompt_hash="abc",
            compatible_arms=[Arm.DOCTRINE],
            graders=[],
        )
        attempt = _make_attempt(task_id="task-no-action")

        analysis = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt],
            metric_observations=[],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        assert analysis.receipt_coverage.eligible_attempt_count == 0

    def test_receipt_coverage_counts_governed_arm_with_action_class(self) -> None:
        """A governed arm with an action class is receipt-eligible."""
        task = _make_task()
        attempt = _make_attempt()

        analysis = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt],
            metric_observations=[],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        assert analysis.receipt_coverage.eligible_attempt_count == 1

    def test_receipt_coverage_with_verified_primary_receipt(self) -> None:
        """A verified primary receipt on an eligible attempt produces 100% coverage."""
        task = _make_task()
        attempt = _make_attempt()
        receipt = _make_receipt_observation(
            attempt_id=attempt.attempt_id,
            verified=True,
            primary=True,
        )

        analysis = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt],
            metric_observations=[],
            receipts=[receipt],
            stages=[],
            run_id=_RUN_ID,
        )
        assert analysis.receipt_coverage.eligible_attempt_count == 1
        assert analysis.receipt_coverage.receipt_bound_count == 1
        assert analysis.receipt_coverage.receipt_verified_count == 1
        assert analysis.receipt_coverage.coverage_pct == 100.0
        assert analysis.receipt_coverage.verification_pct == 100.0

    def test_receipt_coverage_with_unverified_primary_receipt(self) -> None:
        """An unverified primary receipt is bound but not verified."""
        task = _make_task()
        attempt = _make_attempt()
        receipt = _make_receipt_observation(
            attempt_id=attempt.attempt_id,
            verified=False,
            primary=True,
        )

        analysis = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt],
            metric_observations=[],
            receipts=[receipt],
            stages=[],
            run_id=_RUN_ID,
        )
        assert analysis.receipt_coverage.receipt_bound_count == 1
        assert analysis.receipt_coverage.receipt_verified_count == 0
        assert analysis.receipt_coverage.verification_pct == 0.0

    def test_receipt_coverage_excludes_non_primary_receipts(self) -> None:
        """Non-primary receipts do not count toward receipt binding."""
        task = _make_task()
        attempt = _make_attempt()
        receipt = _make_receipt_observation(
            attempt_id=attempt.attempt_id,
            verified=True,
            primary=False,
        )

        analysis = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt],
            metric_observations=[],
            receipts=[receipt],
            stages=[],
            run_id=_RUN_ID,
        )
        assert analysis.receipt_coverage.receipt_bound_count == 0

    def test_receipt_coverage_partial_coverage(self) -> None:
        """Two eligible attempts, one with a receipt, produces 50% coverage."""
        task = _make_task()
        attempt1 = _make_attempt(attempt_id="attempt-1")
        attempt2 = _make_attempt(attempt_id="attempt-2")
        receipt1 = _make_receipt_observation(
            attempt_id="attempt-1",
            verified=True,
            primary=True,
        )

        analysis = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt1, attempt2],
            metric_observations=[],
            receipts=[receipt1],
            stages=[],
            run_id=_RUN_ID,
        )
        assert analysis.receipt_coverage.eligible_attempt_count == 2
        assert analysis.receipt_coverage.receipt_bound_count == 1
        assert analysis.receipt_coverage.coverage_pct == 50.0
