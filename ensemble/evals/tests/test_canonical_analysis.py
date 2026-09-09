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
    BridgeRunManifest,
    CanonicalEvalAnalysis,
    ComparisonDirection,
    ConfusionMatrix,
    ContinuousTestPolicy,
    GateDecisionStatus,
    MetricAnalysisResult,
    MissingnessBreakdown,
    MissingnessReason,
    PreregistrationConfig,
    ReceiptCoverageAnalysis,
    ReplicateAggregationPolicy,
    SecondaryFamily,
    canonical_model_json,
)
from g8e_evals.analysis.engine import (
    BridgeManifestValidationError,
    compute_bridge_run_comparison,
    compute_canonical_analysis,
)
from g8e_evals.arms import Arm
from g8e_evals.metrics import DEFAULT_METRIC_REGISTRY, MetricDirection, UnregisteredMetricError
from g8e_evals.release_metric_set import MetricDomain, RELEASE_METRIC_SET
from g8e.operator.v1.operator_pb2 import (
    ActionReceipt,
    DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
    DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
)
from g8e_evals.schema import (
    AttemptRecord,
    CanaryScrubbingAssertion,
    FactualQAAssertion,
    FactualQAMatchType,
    GraderReference,
    MetricObservation,
    PolicyOutcome,
    ReceiptObservation,
    RejectionLayer,
    SecretDetectionAssertion,
    StageKind,
    StageObservation,
    StateCollectionBoundary,
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
    graders: list[GraderReference] | None = None,
    sensitive_canary_annotations: list[CanaryScrubbingAssertion] | None = None,
    secret_detection_assertions: list[SecretDetectionAssertion] | None = None,
    factual_qa_assertions: list[FactualQAAssertion] | None = None,
) -> TaskDefinition:
    return TaskDefinition(
        task_id=task_id,
        suite_id="utility",
        suite_version="1.0.0",
        prompt_hash="abc123",
        prompt_length=10,
        expected_action_class="TEST_ACTION",
        compatible_arms=[Arm.DOCTRINE],
        graders=graders if graders is not None else [GraderReference(grader_id="receipt_integrity", grader_version="1.0.0")],
        expected_allow_block_outcome=expected_allow_block,
        expected_rejection_layer=RejectionLayer.L1_DOCTRINE if expected_allow_block == PolicyOutcome.BLOCK else None,
        sensitive_canary_annotations=sensitive_canary_annotations if sensitive_canary_annotations is not None else [],
        secret_detection_assertions=secret_detection_assertions if secret_detection_assertions is not None else [],
        factual_qa_assertions=factual_qa_assertions if factual_qa_assertions is not None else [],
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
    value: float | None = 1.0,
    eligible: bool = True,
    verification_status: VerificationStatus = VerificationStatus.VERIFIED,
) -> MetricObservation:
    definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
    return MetricObservation(
        metric_id=metric_id,
        metric_version="1.0.0",
        attempt_id=attempt_id,
        run_id=_RUN_ID,
        arm_id=arm_id,
        task_id=task_id,
        value=value,
        unit=definition.unit,
        eligible=eligible,
        verification_status=verification_status,
        grader_class=definition.grader_class,
    )


def _canary_assertion(assertion_id: str = "canary-1") -> CanaryScrubbingAssertion:
    return CanaryScrubbingAssertion(
        assertion_id=assertion_id,
        canary_sha256="a" * 64,
        source="prompt",
        input_artifact_sha256="b" * 64,
        expected_output_artifact_sha256="c" * 64,
        expected_scrub_type="api_key",
        expected_occurrences=1,
    )


def _secret_detection_assertion(assertion_id: str = "sd-1") -> SecretDetectionAssertion:
    return SecretDetectionAssertion(
        assertion_id=assertion_id,
        source="prompt",
        input_artifact_sha256="d" * 64,
        expected_sensitive_occurrences=1,
        expected_benign_occurrences=0,
        expected_sensitive_types=["api_key"],
    )


def _factual_qa_assertion(assertion_id: str = "fqa-1") -> FactualQAAssertion:
    return FactualQAAssertion(
        assertion_id=assertion_id,
        match_type=FactualQAMatchType.EXACT_MATCH,
        expected_answer="42",
        collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
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
        assert ANALYSIS_SCHEMA_VERSION == "1.1.0"
        assert ANALYSIS_COMPUTATION_VERSION == "1.2.0"

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
        assert "\n" not in json1
        assert json1.startswith('{"analysis_computation_version"')


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

    def test_partial_missingness_cannot_pass_release_gate(self) -> None:
        task = _make_task()
        first_attempt = _make_attempt(attempt_id="attempt-1")
        second_attempt = _make_attempt(attempt_id="attempt-2")
        observation = _make_metric_obs(attempt_id=first_attempt.attempt_id)

        analysis = compute_canonical_analysis(
            tasks=[task],
            attempts=[first_attempt, second_attempt],
            metric_observations=[observation],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )

        result = next(result for result in analysis.metric_results if result.metric_id == "receipt_integrity")
        decision = next(decision for decision in analysis.gate_decisions if decision.metric_id == "receipt_integrity")
        assert result.denominator == 2
        assert result.missing_count == 1
        assert decision.status == GateDecisionStatus.INSUFFICIENT_DATA

    def test_unrelated_release_metrics_are_not_added_to_task_denominators(self) -> None:
        analysis = compute_canonical_analysis(
            tasks=[_make_task()],
            attempts=[_make_attempt()],
            metric_observations=[],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        # receipt_integrity is eligible because the task declares an expected
        # action class and the receipt_integrity grader. Telemetry metrics
        # are eligible for all attempts (eligibility is observation-dependent);
        # with no source records they appear as missing. Other task-assertion
        # metrics are not eligible because the task has no assertions.
        assert {result.metric_id for result in analysis.metric_results} == {
            "receipt_integrity",
            "stage_latency_seconds",
            "provider_usage_tokens",
            "provider_cost_usd",
            "local_resource_peak_memory_bytes",
            "local_resource_cpu_seconds",
            "human_wait_seconds",
        }

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
        task = _make_task(
            graders=[GraderReference(grader_id="secret_detection_precision", grader_version="1.0.0")],
            secret_detection_assertions=[_secret_detection_assertion()],
        )
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

    def test_pooled_confusion_matrix_sums_arm_level_matrices(self) -> None:
        """Pooled confusion matrices sum arm-level TP/FP/TN/FN across arms."""
        task_allow = _make_task(task_id="task-allow", expected_allow_block=PolicyOutcome.ALLOW)
        task_block = _make_task(task_id="task-block", expected_allow_block=PolicyOutcome.BLOCK)
        attempt_allow_doctrine = _make_attempt(attempt_id="attempt-allow-d", task_id="task-allow", arm_id=Arm.DOCTRINE)
        attempt_block_doctrine = _make_attempt(attempt_id="attempt-block-d", task_id="task-block", arm_id=Arm.DOCTRINE)
        attempt_allow_direct = _make_attempt(attempt_id="attempt-allow-r", task_id="task-allow", arm_id=Arm.DIRECT)
        attempt_block_direct = _make_attempt(attempt_id="attempt-block-r", task_id="task-block", arm_id=Arm.DIRECT)
        obs_allow_d = _make_metric_obs(metric_id="policy_outcome", attempt_id="attempt-allow-d", task_id="task-allow", arm_id=Arm.DOCTRINE, value=1.0)
        obs_block_d = _make_metric_obs(metric_id="policy_outcome", attempt_id="attempt-block-d", task_id="task-block", arm_id=Arm.DOCTRINE, value=1.0)
        obs_allow_r = _make_metric_obs(metric_id="policy_outcome", attempt_id="attempt-allow-r", task_id="task-allow", arm_id=Arm.DIRECT, value=0.0)
        obs_block_r = _make_metric_obs(metric_id="policy_outcome", attempt_id="attempt-block-r", task_id="task-block", arm_id=Arm.DIRECT, value=0.0)

        analysis = compute_canonical_analysis(
            tasks=[task_allow, task_block],
            attempts=[attempt_allow_doctrine, attempt_block_doctrine, attempt_allow_direct, attempt_block_direct],
            metric_observations=[obs_allow_d, obs_block_d, obs_allow_r, obs_block_r],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        pooled = [p for p in analysis.pooled_confusion_matrices if p.metric_id == "policy_outcome"]
        assert len(pooled) == 1
        p = pooled[0]
        # Doctrine: TP=1, TN=1, FP=0, FN=0
        # Direct: TP=0, TN=0, FP=1, FN=1
        # Pooled: TP=1, TN=1, FP=1, FN=1
        assert p.true_positive == 1
        assert p.true_negative == 1
        assert p.false_positive == 1
        assert p.false_negative == 1
        assert p.arm_count == 2
        assert p.arm_ids == ["direct", "doctrine"]
        assert p.pooling_method == "preregistered_simple_summation_across_arms"

    def test_pooled_confusion_matrix_properties(self) -> None:
        """Pooled confusion matrix derived properties match arm-level computation."""
        task_allow = _make_task(task_id="task-allow", expected_allow_block=PolicyOutcome.ALLOW)
        task_block = _make_task(task_id="task-block", expected_allow_block=PolicyOutcome.BLOCK)
        attempt_allow = _make_attempt(attempt_id="attempt-allow", task_id="task-allow")
        attempt_block = _make_attempt(attempt_id="attempt-block", task_id="task-block")
        obs_allow = _make_metric_obs(metric_id="policy_outcome", attempt_id="attempt-allow", task_id="task-allow", value=1.0)
        obs_block = _make_metric_obs(metric_id="policy_outcome", attempt_id="attempt-block", task_id="task-block", value=1.0)

        analysis = compute_canonical_analysis(
            tasks=[task_allow, task_block],
            attempts=[attempt_allow, attempt_block],
            metric_observations=[obs_allow, obs_block],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        pooled = next(p for p in analysis.pooled_confusion_matrices if p.metric_id == "policy_outcome")
        assert pooled.total == 2
        assert pooled.accuracy == 1.0
        assert pooled.balanced_accuracy == 1.0
        assert pooled.matthews_correlation_coefficient == 1.0

    def test_pooled_confusion_matrix_empty_when_no_confusion_metrics(self) -> None:
        """No confusion metrics produces no pooled confusion matrices."""
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
        assert analysis.pooled_confusion_matrices == []

    def test_pooled_confusion_matrix_sorted_by_metric_id(self) -> None:
        """Pooled confusion matrices are sorted by (metric_id, metric_version)."""
        task_allow = _make_task(task_id="task-allow", expected_allow_block=PolicyOutcome.ALLOW)
        task_block = _make_task(task_id="task-block", expected_allow_block=PolicyOutcome.BLOCK)
        attempt_allow = _make_attempt(attempt_id="attempt-allow", task_id="task-allow")
        attempt_block = _make_attempt(attempt_id="attempt-block", task_id="task-block")
        obs_allow_po = _make_metric_obs(metric_id="policy_outcome", attempt_id="attempt-allow", task_id="task-allow", value=1.0)
        obs_block_po = _make_metric_obs(metric_id="policy_outcome", attempt_id="attempt-block", task_id="task-block", value=1.0)
        obs_allow_pa = _make_metric_obs(metric_id="policy_attack", attempt_id="attempt-allow", task_id="task-allow", value=1.0)
        obs_block_pa = _make_metric_obs(metric_id="policy_attack", attempt_id="attempt-block", task_id="task-block", value=1.0)

        analysis = compute_canonical_analysis(
            tasks=[task_allow, task_block],
            attempts=[attempt_allow, attempt_block],
            metric_observations=[obs_allow_po, obs_block_po, obs_allow_pa, obs_block_pa],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        metric_ids = [p.metric_id for p in analysis.pooled_confusion_matrices]
        assert metric_ids == sorted(metric_ids)
        assert "policy_attack" in metric_ids
        assert "policy_outcome" in metric_ids

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
        attempt_consensus = _make_attempt(attempt_id="attempt-consensus", arm_id=Arm.CONSENSUS)
        obs_doctrine = _make_metric_obs(
            metric_id="receipt_integrity",
            attempt_id="attempt-doctrine",
            arm_id=Arm.DOCTRINE,
            value=1.0,
        )
        obs_consensus = _make_metric_obs(
            metric_id="receipt_integrity",
            attempt_id="attempt-consensus",
            arm_id=Arm.CONSENSUS,
            value=0.0,
        )

        analysis = compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt_doctrine, attempt_consensus],
            metric_observations=[obs_doctrine, obs_consensus],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        assert sorted(analysis.arm_ids) == ["consensus", "doctrine"]
        ri_results = [r for r in analysis.metric_results if r.metric_id == "receipt_integrity"]
        assert len(ri_results) == 2
        arm_values = {r.arm_id: r.value for r in ri_results}
        assert arm_values["doctrine"] == 1.0
        assert arm_values["consensus"] == 0.0

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
        task = _make_task(
            graders=[GraderReference(grader_id="model_boundary_raw_secret_rate", grader_version="1.0.0")],
            sensitive_canary_annotations=[_canary_assertion()],
        )
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
        task = _make_task(
            graders=[GraderReference(grader_id="model_boundary_raw_secret_rate", grader_version="1.0.0")],
            sensitive_canary_annotations=[_canary_assertion()],
        )
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


def _make_multi_arm_scenario(
    arm_values: dict[Arm, list[float]],
    task_ids: list[str],
    metric_id: str = "receipt_integrity",
    state_snapshot_hash: str = "",
) -> tuple[list[TaskDefinition], list[AttemptRecord], list[MetricObservation]]:
    """Build a multi-arm scenario with paired task instances.

    Creates one task per task_id, one attempt per (arm, task_id), and one
    metric observation per attempt. All attempts share the same
    state_snapshot_hash so they pair on (task_id, state_snapshot_hash).
    """
    tasks = [_make_task(task_id=tid) for tid in task_ids]
    attempts: list[AttemptRecord] = []
    observations: list[MetricObservation] = []
    for arm, values in arm_values.items():
        for tid, val in zip(task_ids, values, strict=True):
            att_id = f"att-{arm.value}-{tid}"
            attempt = _make_attempt(
                attempt_id=att_id,
                task_id=tid,
                arm_id=arm,
            )
            attempt = attempt.model_copy(update={"state_snapshot_hash": state_snapshot_hash})
            attempts.append(attempt)
            observations.append(_make_metric_obs(
                metric_id=metric_id,
                attempt_id=att_id,
                task_id=tid,
                arm_id=arm,
                value=val,
            ))
    return tasks, attempts, observations


def _make_preregistration(
    arm_values: dict[Arm, list[float]],
    metric_id: str = "receipt_integrity",
    primary_metric_ids: list[str] | None = None,
    secondary_families: list | None = None,
    continuous_test_policy: ContinuousTestPolicy = ContinuousTestPolicy.PAIRED_T,
) -> PreregistrationConfig:
    """Build a preregistration config for the arms in a multi-arm scenario.

    The baseline arm is the lexicographically first arm, and all other
    arms are comparison arms. Model cohort IDs default to empty strings
    (matching the default on ``AttemptRecord``).
    """
    arm_ids = sorted(arm.value for arm in arm_values)
    return PreregistrationConfig(
        config_id="test-config-1",
        config_version="1.0.0",
        baseline_arm_id=arm_ids[0],
        comparison_arm_ids=arm_ids[1:],
        model_cohort_ids=[""],
        task_assignment_id="test-assignment-1",
        initial_state_assignment_id="test-snapshot-1",
        required_replicate_ids=["1"],
        required_replicate_count=1,
        primary_metric_ids=primary_metric_ids or [],
        secondary_families=secondary_families or [],
        continuous_test_policy=continuous_test_policy,
        bootstrap_count=10000,
        bootstrap_confidence=0.95,
        bootstrap_seed=0,
        significance_level=0.05,
    )


class TestPairedComparisons:
    """Tests for _compute_paired_comparisons in the canonical analysis engine."""

    def test_paired_comparisons_empty_when_single_arm(self) -> None:
        """Only one arm produces no comparisons (need at least 2 arms)."""
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values={Arm.DOCTRINE: [1.0, 1.0, 1.0]},
            task_ids=["task-1", "task-2", "task-3"],
        )
        analysis = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
        )
        assert analysis.comparisons == []

    def test_paired_comparisons_empty_when_no_common_tasks(self) -> None:
        """Two arms with disjoint task sets produce no comparisons."""
        task_d1 = _make_task(task_id="task-d1")
        task_d2 = _make_task(task_id="task-d2")
        task_r1 = _make_task(task_id="task-r1")
        task_r2 = _make_task(task_id="task-r2")
        att_d1 = _make_attempt(attempt_id="att-d1", task_id="task-d1", arm_id=Arm.DIRECT)
        att_d2 = _make_attempt(attempt_id="att-d2", task_id="task-d2", arm_id=Arm.DIRECT)
        att_r1 = _make_attempt(attempt_id="att-r1", task_id="task-r1", arm_id=Arm.DOCTRINE)
        att_r2 = _make_attempt(attempt_id="att-r2", task_id="task-r2", arm_id=Arm.DOCTRINE)
        obs_d1 = _make_metric_obs(attempt_id="att-d1", task_id="task-d1", arm_id=Arm.DIRECT, value=1.0)
        obs_d2 = _make_metric_obs(attempt_id="att-d2", task_id="task-d2", arm_id=Arm.DIRECT, value=1.0)
        obs_r1 = _make_metric_obs(attempt_id="att-r1", task_id="task-r1", arm_id=Arm.DOCTRINE, value=0.0)
        obs_r2 = _make_metric_obs(attempt_id="att-r2", task_id="task-r2", arm_id=Arm.DOCTRINE, value=0.0)
        prereg = _make_preregistration({Arm.DIRECT: [1.0], Arm.DOCTRINE: [0.0]})

        analysis = compute_canonical_analysis(
            tasks=[task_d1, task_d2, task_r1, task_r2],
            attempts=[att_d1, att_d2, att_r1, att_r2],
            metric_observations=[obs_d1, obs_d2, obs_r1, obs_r2],
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        assert analysis.comparisons == []

    def test_paired_comparisons_empty_when_insufficient_pairs(self) -> None:
        """Only 1 common task instance produces no comparisons (need >= 2)."""
        arm_values = {Arm.DIRECT: [1.0], Arm.DOCTRINE: [0.0]}
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values=arm_values,
            task_ids=["task-1"],
        )
        prereg = _make_preregistration(arm_values)
        analysis = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        assert analysis.comparisons == []

    def test_paired_comparisons_produce_deltas_for_two_arms(self) -> None:
        """Two arms with 3 common tasks produce a comparison with correct deltas."""
        arm_values = {Arm.DIRECT: [1.0, 1.0, 1.0], Arm.DOCTRINE: [0.0, 0.0, 0.0]}
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values=arm_values,
            task_ids=["task-1", "task-2", "task-3"],
        )
        prereg = _make_preregistration(arm_values)
        analysis = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        ri_comparisons = [c for c in analysis.comparisons if c.metric_id == "receipt_integrity"]
        assert len(ri_comparisons) == 1
        cmp = ri_comparisons[0]
        # baseline is the declared baseline arm: "direct"
        assert cmp.baseline_arm_id == "direct"
        assert cmp.comparison_arm_id == "doctrine"
        assert cmp.paired_count == 3
        assert cmp.baseline_value == 1.0
        assert cmp.comparison_value == 0.0
        assert cmp.absolute_delta == -1.0
        assert cmp.relative_delta == -1.0

    def test_paired_comparisons_direction_regression_for_binary_pass_fail(self) -> None:
        """BINARY_PASS_FAIL with negative delta produces REGRESSION."""
        arm_values = {Arm.DIRECT: [1.0, 1.0, 1.0], Arm.DOCTRINE: [0.0, 0.0, 0.0]}
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values=arm_values,
            task_ids=["task-1", "task-2", "task-3"],
        )
        prereg = _make_preregistration(arm_values)
        analysis = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        cmp = next(c for c in analysis.comparisons if c.metric_id == "receipt_integrity")
        assert cmp.direction == ComparisonDirection.REGRESSION

    def test_paired_comparisons_direction_improvement_for_higher_is_better(self) -> None:
        """HIGHER_IS_BETTER with positive delta produces IMPROVEMENT."""
        arm_values = {Arm.DIRECT: [0.5, 0.5, 0.5], Arm.DOCTRINE: [1.0, 1.0, 1.0]}
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values=arm_values,
            task_ids=["task-1", "task-2", "task-3"],
            metric_id="canary_scrubbing",
        )
        prereg = _make_preregistration(arm_values, metric_id="canary_scrubbing")
        analysis = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        cmp = next(c for c in analysis.comparisons if c.metric_id == "canary_scrubbing")
        assert cmp.direction == ComparisonDirection.IMPROVEMENT
        assert cmp.absolute_delta == 0.5

    def test_paired_comparisons_direction_regression_for_higher_is_better(self) -> None:
        """HIGHER_IS_BETTER with negative delta produces REGRESSION."""
        arm_values = {Arm.DIRECT: [1.0, 1.0, 1.0], Arm.DOCTRINE: [0.5, 0.5, 0.5]}
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values=arm_values,
            task_ids=["task-1", "task-2", "task-3"],
            metric_id="canary_scrubbing",
        )
        prereg = _make_preregistration(arm_values, metric_id="canary_scrubbing")
        analysis = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        cmp = next(c for c in analysis.comparisons if c.metric_id == "canary_scrubbing")
        assert cmp.direction == ComparisonDirection.REGRESSION
        assert cmp.absolute_delta == -0.5

    def test_paired_comparisons_direction_improvement_for_lower_is_better(self) -> None:
        """LOWER_IS_BETTER with negative delta produces IMPROVEMENT."""
        arm_values = {Arm.DIRECT: [0.5, 0.5, 0.5], Arm.DOCTRINE: [0.0, 0.0, 0.0]}
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values=arm_values,
            task_ids=["task-1", "task-2", "task-3"],
            metric_id="model_boundary_raw_secret_rate",
        )
        prereg = _make_preregistration(arm_values, metric_id="model_boundary_raw_secret_rate")
        analysis = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        cmp = next(c for c in analysis.comparisons if c.metric_id == "model_boundary_raw_secret_rate")
        assert cmp.direction == ComparisonDirection.IMPROVEMENT
        assert cmp.absolute_delta == -0.5

    def test_paired_comparisons_direction_regression_for_lower_is_better(self) -> None:
        """LOWER_IS_BETTER with positive delta produces REGRESSION."""
        arm_values = {Arm.DIRECT: [0.0, 0.0, 0.0], Arm.DOCTRINE: [0.5, 0.5, 0.5]}
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values=arm_values,
            task_ids=["task-1", "task-2", "task-3"],
            metric_id="model_boundary_raw_secret_rate",
        )
        prereg = _make_preregistration(arm_values, metric_id="model_boundary_raw_secret_rate")
        analysis = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        cmp = next(c for c in analysis.comparisons if c.metric_id == "model_boundary_raw_secret_rate")
        assert cmp.direction == ComparisonDirection.REGRESSION
        assert cmp.absolute_delta == 0.5

    def test_paired_comparisons_direction_neutral_for_zero_delta(self) -> None:
        """Zero absolute delta produces NEUTRAL direction."""
        arm_values = {Arm.DIRECT: [1.0, 1.0, 1.0], Arm.DOCTRINE: [1.0, 1.0, 1.0]}
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values=arm_values,
            task_ids=["task-1", "task-2", "task-3"],
        )
        prereg = _make_preregistration(arm_values)
        analysis = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        cmp = next(c for c in analysis.comparisons if c.metric_id == "receipt_integrity")
        assert cmp.direction == ComparisonDirection.NEUTRAL
        assert cmp.absolute_delta == 0.0

    def test_paired_comparisons_direction_neutral_for_neutral_metric(self) -> None:
        """NEUTRAL direction metric always produces NEUTRAL comparison direction."""
        arm_values = {Arm.DIRECT: [1.0, 2.0, 3.0], Arm.DOCTRINE: [3.0, 2.0, 1.0]}
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values=arm_values,
            task_ids=["task-1", "task-2", "task-3"],
            metric_id="stage_latency_seconds",
        )
        prereg = _make_preregistration(arm_values, metric_id="stage_latency_seconds")
        analysis = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        cmp = next(c for c in analysis.comparisons if c.metric_id == "stage_latency_seconds")
        assert cmp.direction == ComparisonDirection.NEUTRAL

    def test_paired_comparisons_mcnemar_for_binary_pass_fail(self) -> None:
        """BINARY_PASS_FAIL metrics use the McNemar p-value as the comparison p-value."""
        arm_values = {Arm.DIRECT: [1.0, 1.0, 1.0], Arm.DOCTRINE: [0.0, 0.0, 0.0]}
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values=arm_values,
            task_ids=["task-1", "task-2", "task-3"],
        )
        prereg = _make_preregistration(arm_values)
        analysis = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        cmp = next(c for c in analysis.comparisons if c.metric_id == "receipt_integrity")
        # All 3 pairs are discordant in the same direction (baseline pass, comparison fail)
        assert cmp.mcnemar_statistic is not None
        assert cmp.mcnemar_p_value is not None
        assert 0.0 <= cmp.mcnemar_p_value <= 1.0

    def test_paired_comparisons_t_test_for_continuous_metric(self) -> None:
        """Non-binary metrics use the paired t-test p-value as the comparison p-value."""
        arm_values = {Arm.DIRECT: [0.5, 0.6, 0.7], Arm.DOCTRINE: [0.8, 0.8, 1.0]}
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values=arm_values,
            task_ids=["task-1", "task-2", "task-3"],
            metric_id="canary_scrubbing",
        )
        prereg = _make_preregistration(arm_values, metric_id="canary_scrubbing")
        analysis = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        cmp = next(c for c in analysis.comparisons if c.metric_id == "canary_scrubbing")
        # Continuous metrics publish both paired t-test and Wilcoxon signed-rank results.
        # canary_scrubbing has a non-inferiority margin of 0.0 (zero leakage tolerance).
        # The positive bootstrap CI remains within that strict margin, so the comparison passes.
        assert cmp.paired_t_statistic is not None
        assert cmp.paired_t_p_value is not None
        assert cmp.wilcoxon_statistic is not None
        assert cmp.wilcoxon_p_value is not None
        assert cmp.non_inferiority_margin == 0.0
        assert cmp.gate_decision == GateDecisionStatus.PASS

    def test_paired_comparisons_bootstrap_ci_deterministic(self) -> None:
        """Bootstrap CI is deterministic for identical inputs."""
        arm_values = {Arm.DIRECT: [0.5, 0.6, 0.7, 0.8], Arm.DOCTRINE: [0.8, 0.9, 1.0, 0.95]}
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values=arm_values,
            task_ids=["task-1", "task-2", "task-3", "task-4"],
            metric_id="canary_scrubbing",
        )
        prereg = _make_preregistration(arm_values, metric_id="canary_scrubbing")
        analysis1 = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        analysis2 = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        cmp1 = next(c for c in analysis1.comparisons if c.metric_id == "canary_scrubbing")
        cmp2 = next(c for c in analysis2.comparisons if c.metric_id == "canary_scrubbing")
        assert cmp1.bootstrap_ci_lower == cmp2.bootstrap_ci_lower
        assert cmp1.bootstrap_ci_upper == cmp2.bootstrap_ci_upper
        assert cmp1.bootstrap_ci_lower is not None
        assert cmp1.bootstrap_ci_upper is not None

    def test_paired_comparisons_holm_correction_single_comparison(self) -> None:
        """Holm correction with a single comparison preserves the p-value and assigns rank 1.

        Uses McNemar as the preregistered continuous-test policy because the
        data is binary. A metric not in any secondary family receives no
        secondary-family correction: rank 1 and the raw selected p-value.
        """
        arm_values = {Arm.DIRECT: [1.0, 1.0, 1.0], Arm.DOCTRINE: [0.0, 0.0, 0.0]}
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values=arm_values,
            task_ids=["task-1", "task-2", "task-3"],
        )
        prereg = _make_preregistration(
            arm_values,
            continuous_test_policy=ContinuousTestPolicy.MCNEMAR,
        )
        analysis = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        cmp = next(c for c in analysis.comparisons if c.metric_id == "receipt_integrity")
        assert cmp.holm_rank == 1
        assert cmp.family_name is None
        # Single comparison with no family: Holm correction preserves the original p-value
        if cmp.mcnemar_p_value is not None:
            assert cmp.holm_corrected_p_value == cmp.mcnemar_p_value

    def test_paired_comparisons_holm_correction_multiple_comparisons(self) -> None:
        """Three arms produce 2 comparisons per metric with family-wide Holm correction applied.

        With preregistration, the baseline is the lexicographically first arm
        and each comparison arm is paired against it. When the metric is in a
        named secondary family, Holm correction is applied across all
        comparisons in that family. Two comparisons receive ranks 1 and 2.
        """
        task_ids = ["task-1", "task-2", "task-3"]
        tasks = [_make_task(task_id=tid) for tid in task_ids]
        arms_data = {
            Arm.CONSENSUS: [1.0, 1.0, 1.0],
            Arm.DIRECT: [0.0, 0.0, 0.0],
            Arm.DOCTRINE: [1.0, 0.0, 1.0],
        }
        attempts: list[AttemptRecord] = []
        observations: list[MetricObservation] = []
        for arm, values in arms_data.items():
            for tid, val in zip(task_ids, values, strict=True):
                att_id = f"att-{arm.value}-{tid}"
                attempts.append(_make_attempt(attempt_id=att_id, task_id=tid, arm_id=arm))
                observations.append(_make_metric_obs(attempt_id=att_id, task_id=tid, arm_id=arm, value=val))

        prereg = _make_preregistration(
            arms_data,
            secondary_families=[
                SecondaryFamily(family_name="integrity", metric_ids=["receipt_integrity"]),
            ],
            continuous_test_policy=ContinuousTestPolicy.MCNEMAR,
        )
        analysis = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        ri_comparisons = [c for c in analysis.comparisons if c.metric_id == "receipt_integrity"]
        # With preregistration, baseline is consensus and comparison arms are direct and doctrine.
        # That produces 2 pairs: (consensus, direct), (consensus, doctrine).
        assert len(ri_comparisons) == 2
        # All comparisons are in the same family
        assert all(c.family_name == "integrity" for c in ri_comparisons)
        # Holm ranks should be 1, 2 (each comparison gets a unique rank within the family)
        ranks = sorted(c.holm_rank for c in ri_comparisons if c.holm_rank is not None)
        assert ranks == [1, 2]
        # Corrected p-values should be monotonically non-decreasing by rank
        by_rank = sorted(ri_comparisons, key=lambda c: c.holm_rank or 0)
        corrected_ps = [c.holm_corrected_p_value for c in by_rank if c.holm_corrected_p_value is not None]
        if len(corrected_ps) > 1:
            for i in range(1, len(corrected_ps)):
                assert corrected_ps[i] >= corrected_ps[i - 1]

    def test_paired_comparisons_gate_decision_fail_when_inferior(self) -> None:
        """Gate decision is FAIL when the comparison is inferior (worse beyond the margin).

        receipt_integrity has a non-inferiority margin of 0.0 (any evidence
        failure is a release blocker). When the comparison arm is worse than
        the baseline by more than 0.0, the bootstrap CI lower bound crosses
        the margin and the gate fails.
        """
        # 7 tasks with all discordant pairs: baseline all pass, comparison all fail
        # Diffs = [-1.0]*7, bootstrap CI = [-1.0, -1.0]
        # CI lower bound (-1.0) < -margin (0.0) → FAIL
        arm_values = {Arm.DIRECT: [1.0] * 7, Arm.DOCTRINE: [0.0] * 7}
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values=arm_values,
            task_ids=[f"task-{i}" for i in range(1, 8)],
        )
        prereg = _make_preregistration(arm_values)
        analysis = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        cmp = next(c for c in analysis.comparisons if c.metric_id == "receipt_integrity")
        assert cmp.non_inferiority_margin == 0.0
        assert cmp.gate_decision == GateDecisionStatus.FAIL
        assert cmp.absolute_delta == -1.0

    def test_paired_comparisons_gate_decision_pass_when_non_inferior(self) -> None:
        """Gate decision is PASS when the comparison is non-inferior (within the margin).

        When the comparison arm is better than or equal to the baseline, the
        bootstrap CI lower bound does not cross the margin and the gate passes.
        """
        # 7 tasks: comparison all pass, baseline all fail
        # Diffs = [1.0]*7, bootstrap CI = [1.0, 1.0]
        # CI lower bound (1.0) >= -margin (0.0) → PASS
        arm_values = {Arm.DIRECT: [0.0] * 7, Arm.DOCTRINE: [1.0] * 7}
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values=arm_values,
            task_ids=[f"task-{i}" for i in range(1, 8)],
        )
        prereg = _make_preregistration(arm_values)
        analysis = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        cmp = next(c for c in analysis.comparisons if c.metric_id == "receipt_integrity")
        assert cmp.non_inferiority_margin == 0.0
        assert cmp.gate_decision == GateDecisionStatus.PASS

    def test_paired_comparisons_gate_decision_pass_when_no_degradation(self) -> None:
        """Gate decision is PASS when there is no degradation (all same values).

        When both arms produce identical results, the bootstrap CI is [0.0, 0.0]
        and the lower bound (0.0) >= -margin (0.0), so the gate passes.
        The p-value may be None (no discordant pairs) but the non-inferiority
        gate uses the CI, not the p-value.
        """
        # All same values: no discordant pairs, McNemar returns (None, None)
        arm_values = {Arm.DIRECT: [1.0, 1.0, 1.0], Arm.DOCTRINE: [1.0, 1.0, 1.0]}
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values=arm_values,
            task_ids=["task-1", "task-2", "task-3"],
        )
        prereg = _make_preregistration(arm_values)
        analysis = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        cmp = next(c for c in analysis.comparisons if c.metric_id == "receipt_integrity")
        assert cmp.mcnemar_p_value is None
        assert cmp.non_inferiority_margin == 0.0
        assert cmp.gate_decision == GateDecisionStatus.PASS

    def test_paired_comparisons_gate_decision_fail_when_ci_crosses_margin(self) -> None:
        """Gate decision is FAIL when the bootstrap CI crosses the non-inferiority margin.

        When the comparison is worse on average and the CI lower bound is
        below the margin, the gate fails even if the p-value is not
        significant. The non-inferiority gate uses the CI, not the p-value.
        """
        # 3 tasks with mixed: DIRECT=[1.0, 1.0, 0.0], DOCTRINE=[0.0, 0.0, 1.0]
        # Diffs = [-1.0, -1.0, 1.0], mean = -0.333
        # Bootstrap CI lower bound will be negative, crossing the 0.0 margin
        arm_values = {Arm.DIRECT: [1.0, 1.0, 0.0], Arm.DOCTRINE: [0.0, 0.0, 1.0]}
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values=arm_values,
            task_ids=["task-1", "task-2", "task-3"],
        )
        prereg = _make_preregistration(arm_values)
        analysis = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        cmp = next(c for c in analysis.comparisons if c.metric_id == "receipt_integrity")
        assert cmp.mcnemar_p_value is not None
        assert cmp.holm_corrected_p_value is not None
        assert cmp.non_inferiority_margin == 0.0
        assert cmp.gate_decision == GateDecisionStatus.FAIL

    def test_paired_comparisons_sorted_by_metric_and_arms(self) -> None:
        """Comparisons are sorted by (metric_id, baseline_arm_id, comparison_arm_id)."""
        task_ids = ["task-1", "task-2", "task-3"]
        tasks = [_make_task(task_id=tid) for tid in task_ids]
        arms_data = {
            Arm.CONSENSUS: [1.0, 1.0, 1.0],
            Arm.DIRECT: [0.0, 0.0, 0.0],
            Arm.DOCTRINE: [1.0, 0.0, 1.0],
        }
        attempts: list[AttemptRecord] = []
        observations: list[MetricObservation] = []
        for arm, values in arms_data.items():
            for tid, val in zip(task_ids, values, strict=True):
                att_id = f"att-{arm.value}-{tid}"
                attempts.append(_make_attempt(attempt_id=att_id, task_id=tid, arm_id=arm))
                observations.append(_make_metric_obs(
                    metric_id="receipt_integrity",
                    attempt_id=att_id,
                    task_id=tid,
                    arm_id=arm,
                    value=val,
                ))
                observations.append(_make_metric_obs(
                    metric_id="canary_scrubbing",
                    attempt_id=att_id,
                    task_id=tid,
                    arm_id=arm,
                    value=val,
                ))

        prereg = _make_preregistration(arms_data)
        analysis = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        keys = [(c.metric_id, c.baseline_arm_id, c.comparison_arm_id) for c in analysis.comparisons]
        assert keys == sorted(keys)

    def test_paired_comparisons_excludes_none_value_observations(self) -> None:
        """Observations with None values are excluded from pairing."""
        task_ids = ["task-1", "task-2", "task-3"]
        tasks = [_make_task(task_id=tid) for tid in task_ids]
        attempts = [
            _make_attempt(attempt_id="att-d1", task_id="task-1", arm_id=Arm.DIRECT),
            _make_attempt(attempt_id="att-d2", task_id="task-2", arm_id=Arm.DIRECT),
            _make_attempt(attempt_id="att-d3", task_id="task-3", arm_id=Arm.DIRECT),
            _make_attempt(attempt_id="att-r1", task_id="task-1", arm_id=Arm.DOCTRINE),
            _make_attempt(attempt_id="att-r2", task_id="task-2", arm_id=Arm.DOCTRINE),
            _make_attempt(attempt_id="att-r3", task_id="task-3", arm_id=Arm.DOCTRINE),
        ]
        observations = [
            _make_metric_obs(attempt_id="att-d1", task_id="task-1", arm_id=Arm.DIRECT, value=1.0),
            _make_metric_obs(attempt_id="att-d2", task_id="task-2", arm_id=Arm.DIRECT, value=1.0),
            _make_metric_obs(attempt_id="att-d3", task_id="task-3", arm_id=Arm.DIRECT, value=1.0),
            _make_metric_obs(attempt_id="att-r1", task_id="task-1", arm_id=Arm.DOCTRINE, value=0.0),
            # task-2 doctrine observation has None value (excluded)
            _make_metric_obs(attempt_id="att-r2", task_id="task-2", arm_id=Arm.DOCTRINE, value=None),
            _make_metric_obs(attempt_id="att-r3", task_id="task-3", arm_id=Arm.DOCTRINE, value=0.0),
        ]
        prereg = _make_preregistration({Arm.DIRECT: [1.0], Arm.DOCTRINE: [0.0]})

        analysis = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        cmp = [c for c in analysis.comparisons if c.metric_id == "receipt_integrity"]
        assert len(cmp) == 1
        # Only 2 common pairs (task-1 and task-3; task-2 excluded due to None value)
        assert cmp[0].paired_count == 2

    def test_paired_comparisons_pairs_by_state_snapshot_hash(self) -> None:
        """Attempts with different state_snapshot_hash on the same task are not paired."""
        task_ids = ["task-1", "task-2"]
        tasks = [_make_task(task_id=tid) for tid in task_ids]
        # Direct arm: state_snapshot_hash="snap-a"
        att_d1 = _make_attempt(attempt_id="att-d1", task_id="task-1", arm_id=Arm.DIRECT).model_copy(
            update={"state_snapshot_hash": "snap-a"}
        )
        att_d2 = _make_attempt(attempt_id="att-d2", task_id="task-2", arm_id=Arm.DIRECT).model_copy(
            update={"state_snapshot_hash": "snap-a"}
        )
        # Doctrine arm: state_snapshot_hash="snap-b" (different from direct)
        att_r1 = _make_attempt(attempt_id="att-r1", task_id="task-1", arm_id=Arm.DOCTRINE).model_copy(
            update={"state_snapshot_hash": "snap-b"}
        )
        att_r2 = _make_attempt(attempt_id="att-r2", task_id="task-2", arm_id=Arm.DOCTRINE).model_copy(
            update={"state_snapshot_hash": "snap-b"}
        )
        observations = [
            _make_metric_obs(attempt_id="att-d1", task_id="task-1", arm_id=Arm.DIRECT, value=1.0),
            _make_metric_obs(attempt_id="att-d2", task_id="task-2", arm_id=Arm.DIRECT, value=1.0),
            _make_metric_obs(attempt_id="att-r1", task_id="task-1", arm_id=Arm.DOCTRINE, value=0.0),
            _make_metric_obs(attempt_id="att-r2", task_id="task-2", arm_id=Arm.DOCTRINE, value=0.0),
        ]
        prereg = _make_preregistration({Arm.DIRECT: [1.0], Arm.DOCTRINE: [0.0]})

        analysis = compute_canonical_analysis(
            tasks=tasks,
            attempts=[att_d1, att_d2, att_r1, att_r2],
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        # No common pairing keys because state_snapshot_hash differs
        assert analysis.comparisons == []

    def test_paired_comparisons_pairs_with_matching_state_snapshot_hash(self) -> None:
        """Attempts with the same state_snapshot_hash on the same task are paired."""
        task_ids = ["task-1", "task-2", "task-3"]
        tasks = [_make_task(task_id=tid) for tid in task_ids]
        snap = "shared-snap"
        attempts: list[AttemptRecord] = []
        observations: list[MetricObservation] = []
        for arm, values in [(Arm.DIRECT, [1.0, 1.0, 1.0]), (Arm.DOCTRINE, [0.0, 0.0, 0.0])]:
            for tid, val in zip(task_ids, values, strict=True):
                att_id = f"att-{arm.value}-{tid}"
                attempts.append(
                    _make_attempt(attempt_id=att_id, task_id=tid, arm_id=arm).model_copy(
                        update={"state_snapshot_hash": snap}
                    )
                )
                observations.append(_make_metric_obs(attempt_id=att_id, task_id=tid, arm_id=arm, value=val))

        prereg = _make_preregistration({Arm.DIRECT: [1.0], Arm.DOCTRINE: [0.0]})
        analysis = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        cmp = [c for c in analysis.comparisons if c.metric_id == "receipt_integrity"]
        assert len(cmp) == 1
        assert cmp[0].paired_count == 3

    def test_paired_comparisons_three_arms_produce_two_pairs(self) -> None:
        """Three arms with preregistration produce 2 pairs (baseline vs each comparison arm)."""
        arm_values = {
            Arm.CONSENSUS: [1.0, 1.0, 1.0],
            Arm.DIRECT: [0.0, 0.0, 0.0],
            Arm.DOCTRINE: [1.0, 0.0, 1.0],
        }
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values=arm_values,
            task_ids=["task-1", "task-2", "task-3"],
        )
        prereg = _make_preregistration(arm_values)
        analysis = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        ri_comparisons = [c for c in analysis.comparisons if c.metric_id == "receipt_integrity"]
        # With preregistration, baseline is consensus and comparison arms are direct and doctrine.
        assert len(ri_comparisons) == 2
        pairs = {(c.baseline_arm_id, c.comparison_arm_id) for c in ri_comparisons}
        assert pairs == {("consensus", "direct"), ("consensus", "doctrine")}

    def test_paired_comparisons_deterministic_output(self) -> None:
        """Identical inputs produce identical comparison records."""
        arm_values = {Arm.DIRECT: [0.5, 0.6, 0.7, 0.8], Arm.DOCTRINE: [0.8, 0.9, 1.0, 0.95]}
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values=arm_values,
            task_ids=["task-1", "task-2", "task-3", "task-4"],
            metric_id="canary_scrubbing",
        )
        prereg = _make_preregistration(arm_values, metric_id="canary_scrubbing")
        analysis1 = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        analysis2 = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        assert analysis1.canonical_json() == analysis2.canonical_json()

    def test_paired_comparisons_replicates_averaged(self) -> None:
        """Multiple observations per pairing key are averaged before comparison."""
        task_ids = ["task-1", "task-2"]
        tasks = [_make_task(task_id=tid) for tid in task_ids]
        # Direct: 2 observations on task-1 (0.5, 1.0 -> avg 0.75), 1 on task-2 (1.0)
        # Doctrine: 1 observation on task-1 (0.0), 1 on task-2 (0.0)
        attempts = [
            _make_attempt(attempt_id="att-d1a", task_id="task-1", arm_id=Arm.DIRECT),
            _make_attempt(attempt_id="att-d1b", task_id="task-1", arm_id=Arm.DIRECT),
            _make_attempt(attempt_id="att-d2", task_id="task-2", arm_id=Arm.DIRECT),
            _make_attempt(attempt_id="att-r1", task_id="task-1", arm_id=Arm.DOCTRINE),
            _make_attempt(attempt_id="att-r2", task_id="task-2", arm_id=Arm.DOCTRINE),
        ]
        observations = [
            _make_metric_obs(attempt_id="att-d1a", task_id="task-1", arm_id=Arm.DIRECT, value=0.5),
            _make_metric_obs(attempt_id="att-d1b", task_id="task-1", arm_id=Arm.DIRECT, value=1.0),
            _make_metric_obs(attempt_id="att-d2", task_id="task-2", arm_id=Arm.DIRECT, value=1.0),
            _make_metric_obs(attempt_id="att-r1", task_id="task-1", arm_id=Arm.DOCTRINE, value=0.0),
            _make_metric_obs(attempt_id="att-r2", task_id="task-2", arm_id=Arm.DOCTRINE, value=0.0),
        ]
        prereg = _make_preregistration({Arm.DIRECT: [1.0], Arm.DOCTRINE: [0.0]})

        analysis = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        cmp = [c for c in analysis.comparisons if c.metric_id == "receipt_integrity"]
        assert len(cmp) == 1
        # baseline (direct) values: [avg(0.5, 1.0), 1.0] = [0.75, 1.0] -> mean 0.875
        assert cmp[0].baseline_value == round((0.75 + 1.0) / 2, 10)
        assert cmp[0].paired_count == 2

    def test_paired_comparisons_reject_unregistered_metrics(self) -> None:
        """Unregistered metric observations fail closed before comparison."""
        task = _make_task()
        attempt = _make_attempt()
        observation = MetricObservation(
            metric_id="nonexistent_metric",
            metric_version="1.0.0",
            attempt_id=attempt.attempt_id,
            run_id=attempt.run_id,
            arm_id=attempt.arm_id,
            task_id=attempt.task_id,
            value=1.0,
            unit="boolean",
            verification_status=VerificationStatus.VERIFIED,
        )

        with pytest.raises(UnregisteredMetricError, match="unregistered metric"):
            compute_canonical_analysis(
                tasks=[task],
                attempts=[attempt],
                metric_observations=[observation],
                receipts=[],
                stages=[],
                run_id=_RUN_ID,
            )

    def test_paired_comparisons_relative_delta_none_for_zero_baseline(self) -> None:
        """Relative delta is None when the baseline value is zero."""
        arm_values = {Arm.DIRECT: [0.0, 0.0, 0.0], Arm.DOCTRINE: [1.0, 1.0, 1.0]}
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values=arm_values,
            task_ids=["task-1", "task-2", "task-3"],
            metric_id="canary_scrubbing",
        )
        prereg = _make_preregistration(arm_values, metric_id="canary_scrubbing")
        analysis = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        cmp = next(c for c in analysis.comparisons if c.metric_id == "canary_scrubbing")
        assert cmp.relative_delta is None
        assert cmp.absolute_delta == 1.0

    def test_paired_comparisons_effect_size_computed(self) -> None:
        """Cohen's d standardized effect size is computed for non-constant diffs."""
        arm_values = {Arm.DIRECT: [0.5, 0.6, 0.7, 0.8], Arm.DOCTRINE: [0.8, 0.9, 1.0, 0.95]}
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values=arm_values,
            task_ids=["task-1", "task-2", "task-3", "task-4"],
            metric_id="canary_scrubbing",
        )
        prereg = _make_preregistration(arm_values, metric_id="canary_scrubbing")
        analysis = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        cmp = next(c for c in analysis.comparisons if c.metric_id == "canary_scrubbing")
        # Diffs are [0.3, 0.3, 0.3, 0.15] which has non-zero variance, so Cohen's d is computed
        assert cmp.standardized_effect_size is not None

    def test_paired_comparisons_effect_size_none_for_constant_diffs(self) -> None:
        """Cohen's d is None when all diffs are identical (zero variance)."""
        arm_values = {Arm.DIRECT: [1.0, 1.0, 1.0], Arm.DOCTRINE: [0.0, 0.0, 0.0]}
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values=arm_values,
            task_ids=["task-1", "task-2", "task-3"],
        )
        prereg = _make_preregistration(arm_values)
        analysis = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        cmp = next(c for c in analysis.comparisons if c.metric_id == "receipt_integrity")
        # All diffs are -1.0 (constant), so variance is 0 and Cohen's d is None
        assert cmp.standardized_effect_size is None

    def test_non_inferiority_gate_pass_when_within_margin(self) -> None:
        """Non-inferiority gate passes when degradation is within the margin.

        factual_qa has a non-inferiority margin of 0.05 (HIGHER_IS_BETTER).
        When the comparison arm scores 0.03 lower than the baseline, the
        bootstrap CI lower bound (-0.03) >= -margin (-0.05), so the gate passes.
        """
        # Baseline (DIRECT) scores 0.90, comparison (DOCTRINE) scores 0.87
        # Diffs = [-0.03]*7, bootstrap CI = [-0.03, -0.03]
        # CI lower bound (-0.03) >= -margin (-0.05) → PASS
        arm_values = {Arm.DIRECT: [0.90] * 7, Arm.DOCTRINE: [0.87] * 7}
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values=arm_values,
            task_ids=[f"task-{i}" for i in range(1, 8)],
            metric_id="factual_qa",
        )
        prereg = _make_preregistration(arm_values, metric_id="factual_qa")
        analysis = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        cmp = next(c for c in analysis.comparisons if c.metric_id == "factual_qa")
        assert cmp.non_inferiority_margin == 0.05
        assert cmp.gate_decision == GateDecisionStatus.PASS

    def test_non_inferiority_gate_fail_when_outside_margin(self) -> None:
        """Non-inferiority gate fails when degradation exceeds the margin.

        factual_qa has a non-inferiority margin of 0.05 (HIGHER_IS_BETTER).
        When the comparison arm scores 0.10 lower than the baseline, the
        bootstrap CI lower bound (-0.10) < -margin (-0.05), so the gate fails.
        """
        # Baseline (DIRECT) scores 0.90, comparison (DOCTRINE) scores 0.80
        # Diffs = [-0.10]*7, bootstrap CI = [-0.10, -0.10]
        # CI lower bound (-0.10) < -margin (-0.05) → FAIL
        arm_values = {Arm.DIRECT: [0.90] * 7, Arm.DOCTRINE: [0.80] * 7}
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values=arm_values,
            task_ids=[f"task-{i}" for i in range(1, 8)],
            metric_id="factual_qa",
        )
        prereg = _make_preregistration(arm_values, metric_id="factual_qa")
        analysis = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        cmp = next(c for c in analysis.comparisons if c.metric_id == "factual_qa")
        assert cmp.non_inferiority_margin == 0.05
        assert cmp.gate_decision == GateDecisionStatus.FAIL

    def test_non_inferiority_gate_none_for_metrics_without_margin(self) -> None:
        """Metrics without a non-inferiority margin use the default superiority gate."""
        # eval_judge has no non-inferiority margin
        arm_values = {Arm.DIRECT: [3.0, 3.0, 3.0], Arm.DOCTRINE: [4.0, 4.0, 4.0]}
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values=arm_values,
            task_ids=["task-1", "task-2", "task-3"],
            metric_id="eval_judge",
        )
        prereg = _make_preregistration(arm_values, metric_id="eval_judge")
        analysis = compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=_RUN_ID,
            preregistration=prereg,
        )
        cmp = next(c for c in analysis.comparisons if c.metric_id == "eval_judge")
        assert cmp.non_inferiority_margin is None

    def test_preregistration_is_frozen_and_forbids_extra(self) -> None:
        """PreregistrationConfig is frozen and rejects extra fields."""
        from pydantic import ValidationError
        prereg = _make_preregistration({Arm.DIRECT: [1.0], Arm.DOCTRINE: [0.0]})
        # Frozen models reject direct attribute mutation
        with pytest.raises(ValidationError):
            prereg.baseline_arm_id = "doctrine"  # type: ignore[misc]
        # Extra fields are rejected at construction
        with pytest.raises(ValidationError):
            PreregistrationConfig.model_validate({
                "config_id": "test",
                "config_version": "1.0.0",
                "baseline_arm_id": "direct",
                "comparison_arm_ids": ["doctrine"],
                "model_cohort_ids": [""],
                "task_assignment_id": "ta",
                "initial_state_assignment_id": "isa",
                "required_replicate_count": 1,
                "bootstrap_count": 100,
                "bootstrap_confidence": 0.95,
                "bootstrap_seed": 0,
                "significance_level": 0.05,
                "extra_field": "rejected",
            })

    def test_preregistration_deterministic_serialization_and_hash(self) -> None:
        """Identical preregistration configs produce identical canonical JSON."""
        arm_values = {Arm.DIRECT: [1.0, 1.0], Arm.DOCTRINE: [0.0, 0.0]}
        prereg1 = _make_preregistration(arm_values)
        prereg2 = _make_preregistration(arm_values)
        assert canonical_model_json(prereg1) == canonical_model_json(prereg2)

    def test_changed_preregistration_changes_hash(self) -> None:
        """Changing the preregistration config changes the input content hash."""
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values={Arm.DIRECT: [1.0, 1.0], Arm.DOCTRINE: [0.0, 0.0]},
            task_ids=["task-1", "task-2"],
        )
        prereg1 = _make_preregistration({Arm.DIRECT: [1.0], Arm.DOCTRINE: [0.0]})
        prereg2 = PreregistrationConfig(
            config_id="test-config-2",
            config_version="1.0.0",
            baseline_arm_id="direct",
            comparison_arm_ids=["doctrine"],
            model_cohort_ids=[""],
            task_assignment_id="test-assignment-2",
            initial_state_assignment_id="test-snapshot-1",
            required_replicate_ids=["1"],
            required_replicate_count=1,
            bootstrap_count=10000,
            bootstrap_confidence=0.95,
            bootstrap_seed=0,
            significance_level=0.05,
        )
        analysis1 = compute_canonical_analysis(
            tasks=tasks, attempts=attempts, metric_observations=observations,
            receipts=[], stages=[], run_id=_RUN_ID, preregistration=prereg1,
        )
        analysis2 = compute_canonical_analysis(
            tasks=tasks, attempts=attempts, metric_observations=observations,
            receipts=[], stages=[], run_id=_RUN_ID, preregistration=prereg2,
        )
        assert analysis1.input_summary.input_content_hash != analysis2.input_summary.input_content_hash

    def test_undeclared_comparison_arm_rejected(self) -> None:
        """An attempt with an arm not declared in preregistration fails closed."""
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values={Arm.DIRECT: [1.0, 1.0], Arm.DOCTRINE: [0.0, 0.0], Arm.CONSENSUS: [1.0, 1.0]},
            task_ids=["task-1", "task-2"],
        )
        prereg = _make_preregistration({Arm.DIRECT: [1.0], Arm.DOCTRINE: [0.0]})
        with pytest.raises(ValueError, match="not declared in preregistration"):
            compute_canonical_analysis(
                tasks=tasks, attempts=attempts, metric_observations=observations,
                receipts=[], stages=[], run_id=_RUN_ID, preregistration=prereg,
            )

    def test_no_lexicographic_baseline_inference(self) -> None:
        """The declared baseline arm is used, not the lexicographically first arm."""
        task_ids = ["task-1", "task-2", "task-3"]
        tasks = [_make_task(task_id=tid) for tid in task_ids]
        arms_data = {
            Arm.DIRECT: [1.0, 1.0, 1.0],
            Arm.DOCTRINE: [0.0, 0.0, 0.0],
        }
        attempts: list[AttemptRecord] = []
        observations: list[MetricObservation] = []
        for arm, values in arms_data.items():
            for tid, val in zip(task_ids, values, strict=True):
                att_id = f"att-{arm.value}-{tid}"
                attempts.append(_make_attempt(attempt_id=att_id, task_id=tid, arm_id=arm))
                observations.append(_make_metric_obs(attempt_id=att_id, task_id=tid, arm_id=arm, value=val))
        # Declare doctrine as baseline (not lexicographically first)
        prereg = PreregistrationConfig(
            config_id="test-config-1",
            config_version="1.0.0",
            baseline_arm_id="doctrine",
            comparison_arm_ids=["direct"],
            model_cohort_ids=[""],
            task_assignment_id="test-assignment-1",
            initial_state_assignment_id="test-snapshot-1",
            required_replicate_ids=["1"],
            required_replicate_count=1,
            bootstrap_count=10000,
            bootstrap_confidence=0.95,
            bootstrap_seed=0,
            significance_level=0.05,
        )
        analysis = compute_canonical_analysis(
            tasks=tasks, attempts=attempts, metric_observations=observations,
            receipts=[], stages=[], run_id=_RUN_ID, preregistration=prereg,
        )
        ri = [c for c in analysis.comparisons if c.metric_id == "receipt_integrity"]
        assert len(ri) == 1
        assert ri[0].baseline_arm_id == "doctrine"
        assert ri[0].comparison_arm_id == "direct"

    def test_mixed_model_cohort_rejected(self) -> None:
        """Attempts with a model cohort not declared in preregistration fail closed."""
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values={Arm.DIRECT: [1.0, 1.0], Arm.DOCTRINE: [0.0, 0.0]},
            task_ids=["task-1", "task-2"],
        )
        # Add a model cohort to one attempt
        attempts[0] = attempts[0].model_copy(update={"model_cohort_id": "undeclared-cohort"})
        prereg = _make_preregistration({Arm.DIRECT: [1.0], Arm.DOCTRINE: [0.0]})
        with pytest.raises(ValueError, match="model cohort"):
            compute_canonical_analysis(
                tasks=tasks, attempts=attempts, metric_observations=observations,
                receipts=[], stages=[], run_id=_RUN_ID, preregistration=prereg,
            )

    def test_undeclared_replicate_id_rejected(self) -> None:
        """An attempt with a replicate_id not declared in preregistration fails closed."""
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values={Arm.DIRECT: [1.0, 1.0], Arm.DOCTRINE: [0.0, 0.0]},
            task_ids=["task-1", "task-2"],
        )
        # Change one attempt's replicate_id to an undeclared value
        attempts[0] = attempts[0].model_copy(update={"replicate_id": "undeclared-replicate"})
        prereg = _make_preregistration({Arm.DIRECT: [1.0], Arm.DOCTRINE: [0.0]})
        with pytest.raises(ValueError, match="replicate_id"):
            compute_canonical_analysis(
                tasks=tasks, attempts=attempts, metric_observations=observations,
                receipts=[], stages=[], run_id=_RUN_ID, preregistration=prereg,
            )

    def test_attempt_referencing_unknown_task_rejected(self) -> None:
        """An attempt referencing a task not in the declared task set fails closed."""
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values={Arm.DIRECT: [1.0, 1.0], Arm.DOCTRINE: [0.0, 0.0]},
            task_ids=["task-1", "task-2"],
        )
        # Change one attempt's task_id to a task not in the set
        attempts[0] = attempts[0].model_copy(update={"task_id": "unknown-task"})
        prereg = _make_preregistration({Arm.DIRECT: [1.0], Arm.DOCTRINE: [0.0]})
        with pytest.raises(ValueError, match="unknown task"):
            compute_canonical_analysis(
                tasks=tasks, attempts=attempts, metric_observations=observations,
                receipts=[], stages=[], run_id=_RUN_ID, preregistration=prereg,
            )

    def test_selected_continuous_test_policy_determines_decision_p_value(self) -> None:
        """The preregistered continuous-test policy selects which p-value drives the gate."""
        arm_values = {Arm.DIRECT: [1.0, 1.0, 1.0, 1.0], Arm.DOCTRINE: [0.0, 0.0, 0.0, 0.0]}
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values=arm_values,
            task_ids=["task-1", "task-2", "task-3", "task-4"],
        )
        # With WILCOXON policy, the selected test should be wilcoxon
        prereg_w = _make_preregistration(
            arm_values,
            continuous_test_policy=ContinuousTestPolicy.WILCOXON,
        )
        analysis_w = compute_canonical_analysis(
            tasks=tasks, attempts=attempts, metric_observations=observations,
            receipts=[], stages=[], run_id=_RUN_ID, preregistration=prereg_w,
        )
        cmp_w = next(c for c in analysis_w.comparisons if c.metric_id == "receipt_integrity")
        assert cmp_w.selected_test == "wilcoxon"
        if cmp_w.wilcoxon_p_value is not None:
            # With no family, corrected p = raw selected p
            assert cmp_w.holm_corrected_p_value == cmp_w.wilcoxon_p_value

        # With PAIRED_T policy, the selected test should be paired_t
        prereg_t = _make_preregistration(
            arm_values,
            continuous_test_policy=ContinuousTestPolicy.PAIRED_T,
        )
        analysis_t = compute_canonical_analysis(
            tasks=tasks, attempts=attempts, metric_observations=observations,
            receipts=[], stages=[], run_id=_RUN_ID, preregistration=prereg_t,
        )
        cmp_t = next(c for c in analysis_t.comparisons if c.metric_id == "receipt_integrity")
        assert cmp_t.selected_test == "paired_t"

    def test_named_secondary_family_holm_correction_across_metrics(self) -> None:
        """Holm correction is applied across all comparisons for all metrics in a named family."""
        task_ids = ["task-1", "task-2", "task-3", "task-4"]
        tasks = [_make_task(task_id=tid) for tid in task_ids]
        arms_data = {
            Arm.CONSENSUS: [1.0, 1.0, 1.0, 1.0],
            Arm.DIRECT: [0.0, 0.0, 0.0, 0.0],
            Arm.DOCTRINE: [1.0, 0.0, 1.0, 0.0],
        }
        attempts: list[AttemptRecord] = []
        observations: list[MetricObservation] = []
        for arm, values in arms_data.items():
            for tid, val in zip(task_ids, values, strict=True):
                att_id = f"att-{arm.value}-{tid}"
                attempts.append(_make_attempt(attempt_id=att_id, task_id=tid, arm_id=arm))
                observations.append(_make_metric_obs(attempt_id=att_id, task_id=tid, arm_id=arm, value=val))

        # Put two metrics in the same family
        prereg = _make_preregistration(
            arms_data,
            secondary_families=[
                SecondaryFamily(
                    family_name="integrity",
                    metric_ids=["receipt_integrity", "canary_scrubbing"],
                ),
            ],
            continuous_test_policy=ContinuousTestPolicy.MCNEMAR,
        )
        # Add canary_scrubbing observations
        for arm, values in arms_data.items():
            for tid, val in zip(task_ids, values, strict=True):
                att_id = f"att-{arm.value}-{tid}"
                observations.append(_make_metric_obs(
                    metric_id="canary_scrubbing",
                    attempt_id=att_id, task_id=tid, arm_id=arm, value=val,
                ))

        analysis = compute_canonical_analysis(
            tasks=tasks, attempts=attempts, metric_observations=observations,
            receipts=[], stages=[], run_id=_RUN_ID, preregistration=prereg,
        )
        family_comparisons = [c for c in analysis.comparisons if c.family_name == "integrity"]
        # 2 metrics x 2 comparison arms = 4 comparisons in the family
        assert len(family_comparisons) == 4
        # Ranks should be 1, 2, 3, 4 (Holm across all 4 comparisons in the family)
        ranks = sorted(c.holm_rank for c in family_comparisons if c.holm_rank is not None)
        assert ranks == [1, 2, 3, 4]

    def test_primary_metric_not_in_family_correction(self) -> None:
        """Primary metrics do not undergo secondary-family correction."""
        task_ids = ["task-1", "task-2", "task-3"]
        tasks = [_make_task(task_id=tid) for tid in task_ids]
        arms_data = {
            Arm.CONSENSUS: [1.0, 1.0, 1.0],
            Arm.DIRECT: [0.0, 0.0, 0.0],
            Arm.DOCTRINE: [1.0, 0.0, 1.0],
        }
        attempts: list[AttemptRecord] = []
        observations: list[MetricObservation] = []
        for arm, values in arms_data.items():
            for tid, val in zip(task_ids, values, strict=True):
                att_id = f"att-{arm.value}-{tid}"
                attempts.append(_make_attempt(attempt_id=att_id, task_id=tid, arm_id=arm))
                observations.append(_make_metric_obs(attempt_id=att_id, task_id=tid, arm_id=arm, value=val))

        prereg = _make_preregistration(
            arms_data,
            primary_metric_ids=["receipt_integrity"],
            continuous_test_policy=ContinuousTestPolicy.MCNEMAR,
        )
        analysis = compute_canonical_analysis(
            tasks=tasks, attempts=attempts, metric_observations=observations,
            receipts=[], stages=[], run_id=_RUN_ID, preregistration=prereg,
        )
        ri_comparisons = [c for c in analysis.comparisons if c.metric_id == "receipt_integrity"]
        # Primary metric: no family correction, rank 1, raw p-value preserved
        for cmp in ri_comparisons:
            assert cmp.holm_rank == 1
            assert cmp.family_name is None

    def test_deterministic_output_regardless_of_input_list_order(self) -> None:
        """Permuting input observation order produces identical comparison records."""
        task_ids = ["task-1", "task-2", "task-3"]
        tasks = [_make_task(task_id=tid) for tid in task_ids]
        arms_data = {
            Arm.DIRECT: [1.0, 0.0, 1.0],
            Arm.DOCTRINE: [0.0, 1.0, 0.0],
        }
        attempts: list[AttemptRecord] = []
        observations: list[MetricObservation] = []
        for arm, values in arms_data.items():
            for tid, val in zip(task_ids, values, strict=True):
                att_id = f"att-{arm.value}-{tid}"
                attempts.append(_make_attempt(attempt_id=att_id, task_id=tid, arm_id=arm))
                observations.append(_make_metric_obs(attempt_id=att_id, task_id=tid, arm_id=arm, value=val))

        prereg = _make_preregistration(arms_data, continuous_test_policy=ContinuousTestPolicy.MCNEMAR)

        analysis1 = compute_canonical_analysis(
            tasks=tasks, attempts=attempts, metric_observations=observations,
            receipts=[], stages=[], run_id=_RUN_ID, preregistration=prereg,
        )
        # Reverse observation order
        reversed_obs = list(reversed(observations))
        analysis2 = compute_canonical_analysis(
            tasks=tasks, attempts=attempts, metric_observations=reversed_obs,
            receipts=[], stages=[], run_id=_RUN_ID, preregistration=prereg,
        )
        assert analysis1.canonical_json() == analysis2.canonical_json()

    def test_invalid_preregistration_values_fail_closed(self) -> None:
        """Invalid preregistration values (bad significance level, bad bootstrap) fail closed."""
        from pydantic import ValidationError
        with pytest.raises(ValidationError):
            PreregistrationConfig(
                config_id="test",
                config_version="1.0.0",
                baseline_arm_id="direct",
                comparison_arm_ids=["doctrine"],
                model_cohort_ids=[""],
                task_assignment_id="ta",
                initial_state_assignment_id="isa",
                required_replicate_count=1,
                bootstrap_count=0,  # invalid: must be >= 1
                bootstrap_confidence=0.95,
                bootstrap_seed=0,
                significance_level=0.05,
            )
        with pytest.raises(ValidationError):
            PreregistrationConfig(
                config_id="test",
                config_version="1.0.0",
                baseline_arm_id="direct",
                comparison_arm_ids=["doctrine"],
                model_cohort_ids=[""],
                task_assignment_id="ta",
                initial_state_assignment_id="isa",
                required_replicate_count=1,
                bootstrap_count=100,
                bootstrap_confidence=1.5,  # invalid: must be < 1.0
                bootstrap_seed=0,
                significance_level=0.05,
            )
        with pytest.raises(ValidationError):
            PreregistrationConfig(
                config_id="test",
                config_version="1.0.0",
                baseline_arm_id="direct",
                comparison_arm_ids=["doctrine"],
                model_cohort_ids=[""],
                task_assignment_id="ta",
                initial_state_assignment_id="isa",
                required_replicate_count=1,
                bootstrap_count=100,
                bootstrap_confidence=0.95,
                bootstrap_seed=0,
                significance_level=0.0,  # invalid: must be > 0.0
            )

    def test_multiple_declared_comparison_arms(self) -> None:
        """Multiple declared comparison arms produce one comparison per arm pair."""
        task_ids = ["task-1", "task-2", "task-3"]
        tasks = [_make_task(task_id=tid) for tid in task_ids]
        arms_data = {
            Arm.CONSENSUS: [1.0, 1.0, 1.0],
            Arm.DIRECT: [0.0, 0.0, 0.0],
            Arm.DOCTRINE: [1.0, 0.0, 1.0],
        }
        attempts: list[AttemptRecord] = []
        observations: list[MetricObservation] = []
        for arm, values in arms_data.items():
            for tid, val in zip(task_ids, values, strict=True):
                att_id = f"att-{arm.value}-{tid}"
                attempts.append(_make_attempt(attempt_id=att_id, task_id=tid, arm_id=arm))
                observations.append(_make_metric_obs(attempt_id=att_id, task_id=tid, arm_id=arm, value=val))

        prereg = _make_preregistration(arms_data, continuous_test_policy=ContinuousTestPolicy.MCNEMAR)
        analysis = compute_canonical_analysis(
            tasks=tasks, attempts=attempts, metric_observations=observations,
            receipts=[], stages=[], run_id=_RUN_ID, preregistration=prereg,
        )
        ri = [c for c in analysis.comparisons if c.metric_id == "receipt_integrity"]
        # 2 comparison arms (direct, doctrine) paired against baseline (consensus)
        assert len(ri) == 2
        pairs = {(c.baseline_arm_id, c.comparison_arm_id) for c in ri}
        assert pairs == {("consensus", "direct"), ("consensus", "doctrine")}

    def test_unauthorized_arm_not_in_preregistration_produces_no_comparison(self) -> None:
        """A preregistration with no comparison arms fails closed at construction."""
        from pydantic import ValidationError
        # Preregistration only declares direct as baseline with no comparison arms
        with pytest.raises(ValidationError):
            PreregistrationConfig(
                config_id="test",
                config_version="1.0.0",
                baseline_arm_id="direct",
                comparison_arm_ids=[],  # invalid: min_length=1
                model_cohort_ids=[""],
                task_assignment_id="ta",
                initial_state_assignment_id="isa",
                required_replicate_count=1,
                bootstrap_count=100,
                bootstrap_confidence=0.95,
                bootstrap_seed=0,
                significance_level=0.05,
            )

    def test_duplicate_metric_in_secondary_families_rejected(self) -> None:
        """A metric in two secondary families fails closed."""
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values={Arm.DIRECT: [1.0, 1.0], Arm.DOCTRINE: [0.0, 0.0]},
            task_ids=["task-1", "task-2"],
        )
        prereg = _make_preregistration(
            {Arm.DIRECT: [1.0], Arm.DOCTRINE: [0.0]},
            secondary_families=[
                SecondaryFamily(family_name="fam1", metric_ids=["receipt_integrity"]),
                SecondaryFamily(family_name="fam2", metric_ids=["receipt_integrity"]),
            ],
        )
        with pytest.raises(ValueError, match="belongs to multiple secondary families"):
            compute_canonical_analysis(
                tasks=tasks, attempts=attempts, metric_observations=observations,
                receipts=[], stages=[], run_id=_RUN_ID, preregistration=prereg,
            )

    def test_primary_metric_in_secondary_family_rejected(self) -> None:
        """A metric that is both primary and in a secondary family fails closed."""
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values={Arm.DIRECT: [1.0, 1.0], Arm.DOCTRINE: [0.0, 0.0]},
            task_ids=["task-1", "task-2"],
        )
        prereg = _make_preregistration(
            {Arm.DIRECT: [1.0], Arm.DOCTRINE: [0.0]},
            primary_metric_ids=["receipt_integrity"],
            secondary_families=[
                SecondaryFamily(family_name="fam1", metric_ids=["receipt_integrity"]),
            ],
        )
        with pytest.raises(ValueError, match="both primary and in secondary family"):
            compute_canonical_analysis(
                tasks=tasks, attempts=attempts, metric_observations=observations,
                receipts=[], stages=[], run_id=_RUN_ID, preregistration=prereg,
            )


class TestBridgeRunComparisons:
    """Tests for bridge-run comparison computation between two analysis versions."""

    @staticmethod
    def _make_analysis_with_metric(
        metric_id: str,
        arm_id: Arm,
        value: float,
        run_id: str = "run-bridge-old",
    ) -> CanonicalEvalAnalysis:
        """Build a minimal canonical analysis with one metric result."""
        # Set up the task with the right grader and assertions for the metric.
        graders = [GraderReference(grader_id=metric_id, grader_version="1.0.0")]
        task = _make_task(
            graders=graders,
            sensitive_canary_annotations=[_canary_assertion()] if metric_id == "model_boundary_raw_secret_rate" else None,
            secret_detection_assertions=[_secret_detection_assertion()] if metric_id == "secret_detection_precision" else None,
            factual_qa_assertions=[_factual_qa_assertion()] if metric_id == "factual_qa" else None,
        )
        attempt = _make_attempt(
            attempt_id=f"attempt-{run_id}",
            arm_id=arm_id,
        ).model_copy(update={"run_id": run_id})
        obs = _make_metric_obs(
            metric_id=metric_id,
            attempt_id=f"attempt-{run_id}",
            arm_id=arm_id,
            value=value,
        ).model_copy(update={"run_id": run_id})
        return compute_canonical_analysis(
            tasks=[task],
            attempts=[attempt],
            metric_observations=[obs],
            receipts=[],
            stages=[],
            run_id=run_id,
        )

    @staticmethod
    def _make_bridge_manifest(
        old: CanonicalEvalAnalysis,
        new: CanonicalEvalAnalysis,
        *,
        bridge_id: str = "bridge-1",
    ) -> BridgeRunManifest:
        """Build a manifest bound to the actual analysis hashes and task counts."""
        return BridgeRunManifest(
            bridge_id=bridge_id,
            bridge_version="1.0.0",
            old_version_label="v2.1.7",
            new_version_label="v2.1.8",
            old_suite_hash="old-suite-hash",
            new_suite_hash="new-suite-hash",
            old_grader_hash="old-grader-hash",
            new_grader_hash="new-grader-hash",
            old_metric_hash="old-metric-hash",
            new_metric_hash="new-metric-hash",
            old_doctrine_hash="old-doctrine-hash",
            new_doctrine_hash="new-doctrine-hash",
            old_protocol_descriptor_hash="old-proto-hash",
            new_protocol_descriptor_hash="new-proto-hash",
            old_analysis_hash=old.input_summary.input_content_hash,
            new_analysis_hash=new.input_summary.input_content_hash,
            model_cohort_id="cohort-1",
            task_assignment_id="assignment-1",
            task_count=old.input_summary.task_count,
            task_ids=sorted({t.task_id for t in []}),
            initial_state_snapshot_hashes=[],
            replicate_aggregation_policy=ReplicateAggregationPolicy.MEAN,
            required_replicate_ids=["1"],
        )

    def test_bridge_run_comparison_pass_when_no_change(self) -> None:
        """Bridge comparison passes when old and new values are identical."""
        old = self._make_analysis_with_metric("receipt_integrity", Arm.DOCTRINE, 1.0, "run-old")
        new = self._make_analysis_with_metric("receipt_integrity", Arm.DOCTRINE, 1.0, "run-new")
        manifest = self._make_bridge_manifest(old, new)
        comparisons = compute_bridge_run_comparison(manifest, old, new)
        ri = next(c for c in comparisons if c.metric_id == "receipt_integrity")
        assert ri.old_value == 1.0
        assert ri.new_value == 1.0
        assert ri.absolute_delta == 0.0
        assert ri.gate_decision == GateDecisionStatus.PASS

    def test_bridge_run_comparison_fail_when_blocker_regresses(self) -> None:
        """Bridge comparison fails when a release-blocker metric regresses beyond margin."""
        old = self._make_analysis_with_metric("receipt_integrity", Arm.DOCTRINE, 1.0, "run-old")
        new = self._make_analysis_with_metric("receipt_integrity", Arm.DOCTRINE, 0.0, "run-new")
        manifest = self._make_bridge_manifest(old, new)
        comparisons = compute_bridge_run_comparison(manifest, old, new)
        ri = next(c for c in comparisons if c.metric_id == "receipt_integrity")
        assert ri.old_value == 1.0
        assert ri.new_value == 0.0
        assert ri.absolute_delta == -1.0
        assert ri.gate_decision == GateDecisionStatus.FAIL

    def test_bridge_run_comparison_pass_when_utility_within_margin(self) -> None:
        """Bridge comparison passes when utility metric stays within the 0.05 margin."""
        old = self._make_analysis_with_metric("factual_qa", Arm.DOCTRINE, 0.90, "run-old")
        new = self._make_analysis_with_metric("factual_qa", Arm.DOCTRINE, 0.87, "run-new")
        manifest = self._make_bridge_manifest(old, new)
        comparisons = compute_bridge_run_comparison(manifest, old, new)
        qa = next(c for c in comparisons if c.metric_id == "factual_qa")
        assert qa.old_value == 0.90
        assert qa.new_value == 0.87
        assert qa.absolute_delta == -0.03
        assert qa.gate_decision == GateDecisionStatus.PASS

    def test_bridge_run_comparison_fail_when_utility_outside_margin(self) -> None:
        """Bridge comparison fails when utility metric regresses beyond the 0.05 margin."""
        old = self._make_analysis_with_metric("factual_qa", Arm.DOCTRINE, 0.90, "run-old")
        new = self._make_analysis_with_metric("factual_qa", Arm.DOCTRINE, 0.80, "run-new")
        manifest = self._make_bridge_manifest(old, new)
        comparisons = compute_bridge_run_comparison(manifest, old, new)
        qa = next(c for c in comparisons if c.metric_id == "factual_qa")
        assert qa.absolute_delta == -0.10
        assert qa.gate_decision == GateDecisionStatus.FAIL

    def test_bridge_run_comparison_insufficient_data_when_metric_missing(self) -> None:
        """Bridge comparison is INSUFFICIENT_DATA when a metric is in only one version."""
        old = self._make_analysis_with_metric("receipt_integrity", Arm.DOCTRINE, 1.0, "run-old")
        # New analysis has a different metric
        new = self._make_analysis_with_metric("factual_qa", Arm.DOCTRINE, 0.90, "run-new")
        manifest = self._make_bridge_manifest(old, new)
        comparisons = compute_bridge_run_comparison(manifest, old, new)
        ri = next(c for c in comparisons if c.metric_id == "receipt_integrity")
        assert ri.old_value == 1.0
        assert ri.new_value is None
        assert ri.gate_decision == GateDecisionStatus.INSUFFICIENT_DATA

    def test_bridge_run_comparison_sorted_by_metric_id(self) -> None:
        """Bridge comparisons are sorted by (bridge_id, metric_id, metric_version)."""
        old = self._make_analysis_with_metric("receipt_integrity", Arm.DOCTRINE, 1.0, "run-old")
        new = self._make_analysis_with_metric("receipt_integrity", Arm.DOCTRINE, 1.0, "run-new")
        manifest = self._make_bridge_manifest(old, new)
        comparisons = compute_bridge_run_comparison(manifest, old, new)
        metric_ids = [c.metric_id for c in comparisons]
        assert metric_ids == sorted(metric_ids)

    def test_bridge_run_comparison_deterministic(self) -> None:
        """Bridge comparison is deterministic for identical inputs."""
        old = self._make_analysis_with_metric("receipt_integrity", Arm.DOCTRINE, 1.0, "run-old")
        new = self._make_analysis_with_metric("receipt_integrity", Arm.DOCTRINE, 0.5, "run-new")
        manifest = self._make_bridge_manifest(old, new)
        c1 = compute_bridge_run_comparison(manifest, old, new)
        c2 = compute_bridge_run_comparison(manifest, old, new)
        assert [c.model_dump() for c in c1] == [c.model_dump() for c in c2]

    def test_bridge_run_comparison_model_is_frozen(self) -> None:
        """BridgeRunComparison model is frozen."""
        old = self._make_analysis_with_metric("receipt_integrity", Arm.DOCTRINE, 1.0, "run-old")
        new = self._make_analysis_with_metric("receipt_integrity", Arm.DOCTRINE, 1.0, "run-new")
        manifest = self._make_bridge_manifest(old, new)
        comparisons = compute_bridge_run_comparison(manifest, old, new)
        with pytest.raises(Exception, match="frozen"):
            comparisons[0].gate_decision = GateDecisionStatus.PASS

    def test_bridge_run_comparison_carries_manifest_bridge_id(self) -> None:
        """Bridge comparison records carry the manifest bridge_id, not a free string."""
        old = self._make_analysis_with_metric("receipt_integrity", Arm.DOCTRINE, 1.0, "run-old")
        new = self._make_analysis_with_metric("receipt_integrity", Arm.DOCTRINE, 1.0, "run-new")
        manifest = self._make_bridge_manifest(old, new, bridge_id="manifest-bridge-42")
        comparisons = compute_bridge_run_comparison(manifest, old, new)
        assert all(c.bridge_id == "manifest-bridge-42" for c in comparisons)

    def test_canonical_analysis_has_empty_bridge_run_comparisons_by_default(self) -> None:
        """A fresh canonical analysis has empty bridge_run_comparisons."""
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
        assert analysis.bridge_run_comparisons == []
        assert analysis.bridge_runs == []


class TestBridgeManifestValidation:
    """A bridge comparison cannot run unless old and new analyses satisfy one manifest."""

    @staticmethod
    def _make_analysis_with_metric(
        metric_id: str,
        arm_id: Arm,
        value: float,
        run_id: str = "run-bridge-old",
        task_count: int = 1,
    ) -> CanonicalEvalAnalysis:
        """Build a canonical analysis with one metric result per task."""
        tasks = []
        attempts = []
        observations = []
        for i in range(task_count):
            tid = f"task-bridge-{i}"
            graders = [GraderReference(grader_id=metric_id, grader_version="1.0.0")]
            task = _make_task(
                task_id=tid,
                graders=graders,
                sensitive_canary_annotations=[_canary_assertion()] if metric_id == "model_boundary_raw_secret_rate" else None,
                secret_detection_assertions=[_secret_detection_assertion()] if metric_id == "secret_detection_precision" else None,
                factual_qa_assertions=[_factual_qa_assertion()] if metric_id == "factual_qa" else None,
            )
            attempt = _make_attempt(
                attempt_id=f"attempt-{run_id}-{i}",
                task_id=tid,
                arm_id=arm_id,
            ).model_copy(update={"run_id": run_id})
            obs = _make_metric_obs(
                metric_id=metric_id,
                attempt_id=f"attempt-{run_id}-{i}",
                task_id=tid,
                arm_id=arm_id,
                value=value,
            ).model_copy(update={"run_id": run_id})
            tasks.append(task)
            attempts.append(attempt)
            observations.append(obs)
        return compute_canonical_analysis(
            tasks=tasks,
            attempts=attempts,
            metric_observations=observations,
            receipts=[],
            stages=[],
            run_id=run_id,
        )

    @staticmethod
    def _make_manifest(
        old: CanonicalEvalAnalysis,
        new: CanonicalEvalAnalysis,
        *,
        old_analysis_hash: str | None = None,
        new_analysis_hash: str | None = None,
        task_count: int | None = None,
    ) -> BridgeRunManifest:
        return BridgeRunManifest(
            bridge_id="bridge-1",
            bridge_version="1.0.0",
            old_version_label="v2.1.7",
            new_version_label="v2.1.8",
            old_suite_hash="old-suite-hash",
            new_suite_hash="new-suite-hash",
            old_grader_hash="old-grader-hash",
            new_grader_hash="new-grader-hash",
            old_metric_hash="old-metric-hash",
            new_metric_hash="new-metric-hash",
            old_doctrine_hash="old-doctrine-hash",
            new_doctrine_hash="new-doctrine-hash",
            old_protocol_descriptor_hash="old-proto-hash",
            new_protocol_descriptor_hash="new-proto-hash",
            old_analysis_hash=old_analysis_hash if old_analysis_hash is not None else old.input_summary.input_content_hash,
            new_analysis_hash=new_analysis_hash if new_analysis_hash is not None else new.input_summary.input_content_hash,
            model_cohort_id="cohort-1",
            task_assignment_id="assignment-1",
            task_count=task_count if task_count is not None else old.input_summary.task_count,
            task_ids=[],
            initial_state_snapshot_hashes=[],
            replicate_aggregation_policy=ReplicateAggregationPolicy.MEAN,
            required_replicate_ids=["1"],
        )

    def test_rejects_old_analysis_hash_mismatch(self) -> None:
        """A manifest whose old_analysis_hash does not match the old analysis is rejected."""
        old = self._make_analysis_with_metric("receipt_integrity", Arm.DOCTRINE, 1.0, "run-old")
        new = self._make_analysis_with_metric("receipt_integrity", Arm.DOCTRINE, 1.0, "run-new")
        manifest = self._make_manifest(old, new, old_analysis_hash="0" * 64)
        with pytest.raises(BridgeManifestValidationError, match="old analysis hash mismatch"):
            compute_bridge_run_comparison(manifest, old, new)

    def test_rejects_new_analysis_hash_mismatch(self) -> None:
        """A manifest whose new_analysis_hash does not match the new analysis is rejected."""
        old = self._make_analysis_with_metric("receipt_integrity", Arm.DOCTRINE, 1.0, "run-old")
        new = self._make_analysis_with_metric("receipt_integrity", Arm.DOCTRINE, 1.0, "run-new")
        manifest = self._make_manifest(old, new, new_analysis_hash="0" * 64)
        with pytest.raises(BridgeManifestValidationError, match="new analysis hash mismatch"):
            compute_bridge_run_comparison(manifest, old, new)

    def test_rejects_old_task_count_mismatch(self) -> None:
        """A manifest whose task_count does not match the old analysis is rejected."""
        old = self._make_analysis_with_metric("receipt_integrity", Arm.DOCTRINE, 1.0, "run-old", task_count=2)
        new = self._make_analysis_with_metric("receipt_integrity", Arm.DOCTRINE, 1.0, "run-new", task_count=2)
        manifest = self._make_manifest(old, new, task_count=1)
        with pytest.raises(BridgeManifestValidationError, match="old analysis task count mismatch"):
            compute_bridge_run_comparison(manifest, old, new)

    def test_rejects_new_task_count_mismatch(self) -> None:
        """A manifest whose task_count does not match the new analysis is rejected."""
        old = self._make_analysis_with_metric("receipt_integrity", Arm.DOCTRINE, 1.0, "run-old", task_count=1)
        new = self._make_analysis_with_metric("receipt_integrity", Arm.DOCTRINE, 1.0, "run-new", task_count=2)
        manifest = self._make_manifest(old, new, task_count=1)
        with pytest.raises(BridgeManifestValidationError, match="new analysis task count mismatch"):
            compute_bridge_run_comparison(manifest, old, new)

    def test_rejects_arm_set_mismatch(self) -> None:
        """Old and new analyses with different arm sets are rejected."""
        old = self._make_analysis_with_metric("receipt_integrity", Arm.DOCTRINE, 1.0, "run-old")
        new = self._make_analysis_with_metric("receipt_integrity", Arm.CONSENSUS, 1.0, "run-new")
        manifest = self._make_manifest(old, new)
        with pytest.raises(BridgeManifestValidationError, match="arm set mismatch"):
            compute_bridge_run_comparison(manifest, old, new)

    def test_rejects_release_version_mismatch(self) -> None:
        """Old and new analyses with different release versions are rejected."""
        old = self._make_analysis_with_metric("receipt_integrity", Arm.DOCTRINE, 1.0, "run-old")
        new = self._make_analysis_with_metric("receipt_integrity", Arm.DOCTRINE, 1.0, "run-new")
        new = new.model_copy(update={"release_version": "v2.1.9"})
        manifest = self._make_manifest(old, new)
        with pytest.raises(BridgeManifestValidationError, match="release version mismatch"):
            compute_bridge_run_comparison(manifest, old, new)

    def test_valid_manifest_produces_deterministic_bridge_vector(self) -> None:
        """A manifest-bound valid bridge comparison is deterministic across repeated calls."""
        old = self._make_analysis_with_metric("receipt_integrity", Arm.DOCTRINE, 1.0, "run-old")
        new = self._make_analysis_with_metric("receipt_integrity", Arm.DOCTRINE, 0.9, "run-new")
        manifest = self._make_manifest(old, new)
        c1 = compute_bridge_run_comparison(manifest, old, new)
        c2 = compute_bridge_run_comparison(manifest, old, new)
        assert [c.model_dump() for c in c1] == [c.model_dump() for c in c2]
        assert all(c.bridge_id == "bridge-1" for c in c1)

    def test_manifest_model_is_frozen_and_forbids_extra(self) -> None:
        """BridgeRunManifest is frozen and forbids extra fields."""
        old = self._make_analysis_with_metric("receipt_integrity", Arm.DOCTRINE, 1.0, "run-old")
        new = self._make_analysis_with_metric("receipt_integrity", Arm.DOCTRINE, 1.0, "run-new")
        manifest = self._make_manifest(old, new)
        with pytest.raises(Exception, match="frozen"):
            manifest.bridge_id = "other"  # type: ignore[misc]
        with pytest.raises(Exception, match="extra"):
            BridgeRunManifest(
                bridge_id="bridge-1",
                bridge_version="1.0.0",
                old_version_label="v2.1.7",
                new_version_label="v2.1.8",
                old_suite_hash="old-suite-hash",
                new_suite_hash="new-suite-hash",
                old_grader_hash="old-grader-hash",
                new_grader_hash="new-grader-hash",
                old_metric_hash="old-metric-hash",
                new_metric_hash="new-metric-hash",
                old_doctrine_hash="old-doctrine-hash",
                new_doctrine_hash="new-doctrine-hash",
                old_protocol_descriptor_hash="old-proto-hash",
                new_protocol_descriptor_hash="new-proto-hash",
                old_analysis_hash=old.input_summary.input_content_hash,
                new_analysis_hash=new.input_summary.input_content_hash,
                model_cohort_id="cohort-1",
                task_assignment_id="assignment-1",
                task_count=1,
                task_ids=[],
                initial_state_snapshot_hashes=[],
                replicate_aggregation_policy=ReplicateAggregationPolicy.MEAN,
                required_replicate_ids=["1"],
                extra_field="bad",  # type: ignore[call-arg]
            )



class TestCanonicalAnalysisInputValidation:
    @staticmethod
    def _compute(
        *,
        tasks: list[TaskDefinition] | None = None,
        attempts: list[AttemptRecord] | None = None,
        observations: list[MetricObservation] | None = None,
        receipts: list[ReceiptObservation] | None = None,
        stages: list[StageObservation] | None = None,
    ) -> CanonicalEvalAnalysis:
        return compute_canonical_analysis(
            tasks=tasks if tasks is not None else [_make_task()],
            attempts=attempts if attempts is not None else [_make_attempt()],
            metric_observations=observations if observations is not None else [],
            receipts=receipts if receipts is not None else [],
            stages=stages if stages is not None else [],
            run_id=_RUN_ID,
        )

    @pytest.mark.parametrize(
        ("attempts", "error"),
        [
            ([_make_attempt(), _make_attempt()], "duplicate attempt ID"),
            ([_make_attempt().model_copy(update={"run_id": "other-run"})], "attempt run does not match"),
            ([_make_attempt(task_id="other-task")], "attempt references unknown task"),
        ],
    )
    def test_rejects_invalid_attempt_bindings(
        self,
        attempts: list[AttemptRecord],
        error: str,
    ) -> None:
        with pytest.raises(ValueError, match=error):
            self._compute(attempts=attempts)

    @pytest.mark.parametrize(
        ("updates", "error"),
        [
            ({"run_id": "other-run"}, "metric observation run does not match"),
            ({"task_id": "other-task"}, "metric observation task does not match"),
            ({"arm_id": Arm.DIRECT}, "metric observation arm does not match"),
            ({"attempt_id": "other-attempt"}, "metric observation references unknown attempt"),
            ({"unit": "wrong-unit"}, "unit mismatch"),
        ],
    )
    def test_rejects_invalid_metric_observation_bindings(
        self,
        updates: dict[str, object],
        error: str,
    ) -> None:
        observation = _make_metric_obs().model_copy(update=updates)
        with pytest.raises(ValueError, match=error):
            self._compute(observations=[observation])

    def test_rejects_duplicate_metric_observations(self) -> None:
        observation = _make_metric_obs()
        with pytest.raises(ValueError, match="duplicate metric observation"):
            self._compute(observations=[observation, observation])

    @pytest.mark.parametrize(
        ("receipt", "error"),
        [
            (_make_receipt_observation().model_copy(update={"run_id": "other-run"}), "receipt run does not match"),
            (_make_receipt_observation(attempt_id="other-attempt"), "receipt references unknown attempt"),
        ],
    )
    def test_rejects_invalid_receipt_bindings(
        self,
        receipt: ReceiptObservation,
        error: str,
    ) -> None:
        with pytest.raises(ValueError, match=error):
            self._compute(receipts=[receipt])

    @pytest.mark.parametrize(
        ("updates", "error"),
        [
            ({"run_id": "other-run"}, "stage run does not match"),
            ({"attempt_id": "other-attempt"}, "stage references unknown attempt"),
            ({"task_id": "other-task"}, "stage task does not match"),
        ],
    )
    def test_rejects_invalid_stage_bindings(
        self,
        updates: dict[str, object],
        error: str,
    ) -> None:
        stage = StageObservation(
            stage_id="stage-1",
            attempt_id=_ATTEMPT_ID,
            run_id=_RUN_ID,
            task_id=_TASK_ID,
            kind=StageKind.GRADING,
        ).model_copy(update=updates)
        with pytest.raises(ValueError, match=error):
            self._compute(stages=[stage])

    def test_input_hash_is_independent_of_mapping_insertion_order(self) -> None:
        first_task = _make_task().model_copy(update={"metadata": {"z": 1, "a": 2}})
        second_task = _make_task().model_copy(update={"metadata": {"a": 2, "z": 1}})
        first = self._compute(tasks=[first_task])
        second = self._compute(tasks=[second_task])
        assert first.input_summary.input_content_hash == second.input_summary.input_content_hash
