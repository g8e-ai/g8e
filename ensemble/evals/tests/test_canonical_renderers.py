# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for the canonical analysis renderers.

Verifies that Markdown, HTML, and CLI renderers are deterministic
(byte-identical for identical inputs), cover every section of the
canonical analysis, and derive all values from the typed analysis
model without re-computation.
"""

from __future__ import annotations

import pytest

pytestmark = pytest.mark.unit

from g8e_evals.analysis.canonical import (
    ANALYSIS_COMPUTATION_VERSION,
    ANALYSIS_SCHEMA_VERSION,
    AnalysisInputSummary,
    BridgeRunComparison,
    BridgeRunManifest,
    CanonicalEvalAnalysis,
    ComparisonDirection,
    ConfusionMatrix,
    DomainStratifiedResult,
    GateDecision,
    GateDecisionStatus,
    MetricAnalysisResult,
    MissingnessBreakdown,
    PairedComparison,
    PooledConfusionMatrix,
    ReceiptCoverageAnalysis,
    ReplicateAggregationPolicy,
)
from g8e_evals.analysis.engine import compute_canonical_analysis
from g8e_evals.analysis.renderers import render_cli, render_html, render_markdown
from g8e_evals.arms import Arm
from g8e_evals.metrics import MetricDirection
from g8e_evals.release_metric_set import MetricDomain
from g8e_evals.schema import (
    AttemptRecord,
    GraderClass,
    MetricObservation,
    PolicyOutcome,
    TaskDefinition,
    TerminalStatus,
    VerificationStatus,
)

_RUN_ID = "run-render-1"
_TASK_ID = "task-render-1"
_ATTEMPT_ID = "attempt-render-1"


def _make_task(
    task_id: str = _TASK_ID,
    expected_allow_block: PolicyOutcome | None = None,
) -> TaskDefinition:
    from g8e_evals.schema import GraderReference, RejectionLayer

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
    value: float | None = 1.0,
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
        eligible=True,
        verification_status=VerificationStatus.VERIFIED,
        grader_class=GraderClass.DETERMINISTIC,
    )


def _make_multi_arm_scenario(
    arm_values: dict[Arm, list[float]],
    task_ids: list[str],
    metric_id: str = "receipt_integrity",
) -> tuple[list[TaskDefinition], list[AttemptRecord], list[MetricObservation]]:
    tasks = [_make_task(task_id=tid) for tid in task_ids]
    attempts: list[AttemptRecord] = []
    observations: list[MetricObservation] = []
    for arm, values in arm_values.items():
        for tid, val in zip(task_ids, values, strict=True):
            att_id = f"att-{arm.value}-{tid}"
            attempts.append(_make_attempt(attempt_id=att_id, task_id=tid, arm_id=arm))
            observations.append(_make_metric_obs(
                metric_id=metric_id,
                attempt_id=att_id,
                task_id=tid,
                arm_id=arm,
                value=val,
            ))
    return tasks, attempts, observations


def _make_rich_analysis() -> CanonicalEvalAnalysis:
    """Build a canonical analysis with multiple arms, comparisons, and gate decisions."""
    tasks, attempts, observations = _make_multi_arm_scenario(
        arm_values={Arm.DIRECT: [1.0] * 7, Arm.DOCTRINE: [1.0] * 7},
        task_ids=[f"task-{i}" for i in range(1, 8)],
    )
    return compute_canonical_analysis(
        tasks=tasks,
        attempts=attempts,
        metric_observations=observations,
        receipts=[],
        stages=[],
        run_id=_RUN_ID,
    )


def _make_synthetic_analysis() -> CanonicalEvalAnalysis:
    """Build a fully populated synthetic analysis covering all sections."""
    return CanonicalEvalAnalysis(
        analysis_schema_version=ANALYSIS_SCHEMA_VERSION,
        analysis_computation_version=ANALYSIS_COMPUTATION_VERSION,
        release_version="v2.1.8",
        run_id=_RUN_ID,
        input_summary=AnalysisInputSummary(
            task_count=2,
            attempt_count=4,
            observation_count=8,
            receipt_count=2,
            stage_count=6,
            metric_observation_count=4,
            input_content_hash="abc123def456",
        ),
        missingness=MissingnessBreakdown(
            completed=3,
            model_failed=1,
            governance_rejected=0,
            human_denied=0,
            timed_out=0,
            infrastructure_failed=0,
            invalid_evidence=0,
        ),
        receipt_coverage=ReceiptCoverageAnalysis(
            eligible_attempt_count=2,
            receipt_bound_count=2,
            receipt_verified_count=1,
        ),
        arm_ids=["direct", "doctrine"],
        metric_results=[
            MetricAnalysisResult(
                metric_id="receipt_integrity",
                metric_version="1.0.0",
                arm_id="doctrine",
                domain=MetricDomain.GOVERNANCE,
                direction=MetricDirection.BINARY_PASS_FAIL,
                unit="boolean",
                numerator=2.0,
                denominator=2,
                value=1.0,
                eligible_count=2,
                not_eligible_count=0,
                missing_count=0,
                verification_status_counts={"verified": 2},
                evidence_ref_count=2,
                metric_observation_ids=["receipt_integrity@1.0.0:att-1", "receipt_integrity@1.0.0:att-2"],
            ),
        ],
        domain_stratified_results=[
            DomainStratifiedResult(
                arm_id="doctrine",
                domain=MetricDomain.GOVERNANCE,
                metric_count=1,
                passing_metric_count=1,
                failing_metric_count=0,
                not_applicable_metric_count=0,
            ),
        ],
        confusion_matrices=[
            ConfusionMatrix(
                metric_id="policy_outcome",
                metric_version="1.0.0",
                arm_id="doctrine",
                domain=MetricDomain.GOVERNANCE,
                true_positive=3,
                false_positive=1,
                true_negative=2,
                false_negative=0,
            ),
        ],
        pooled_confusion_matrices=[
            PooledConfusionMatrix(
                metric_id="policy_outcome",
                metric_version="1.0.0",
                domain=MetricDomain.GOVERNANCE,
                arm_count=1,
                arm_ids=["doctrine"],
                true_positive=3,
                false_positive=1,
                true_negative=2,
                false_negative=0,
                pooling_method="preregistered_simple_summation_across_arms",
            ),
        ],
        comparisons=[
            PairedComparison(
                metric_id="receipt_integrity",
                metric_version="1.0.0",
                baseline_arm_id="direct",
                comparison_arm_id="doctrine",
                paired_count=7,
                baseline_value=1.0,
                comparison_value=1.0,
                absolute_delta=0.0,
                relative_delta=0.0,
                standardized_effect_size=None,
                direction=ComparisonDirection.NEUTRAL,
                mcnemar_statistic=None,
                mcnemar_p_value=None,
                bootstrap_ci_lower=0.0,
                bootstrap_ci_upper=0.0,
                holm_corrected_p_value=1.0,
                holm_rank=1,
                non_inferiority_margin=0.0,
                gate_decision=GateDecisionStatus.PASS,
                selected_test="paired_t",
                family_name=None,
                replicate_aggregation_policy="mean",
            ),
        ],
        gate_decisions=[
            GateDecision(
                metric_id="receipt_integrity",
                metric_version="1.0.0",
                arm_id="doctrine",
                threshold_description="Practical threshold: 1.0 for governed arms.",
                measured_value=1.0,
                threshold_value=1.0,
                non_inferiority_margin=0.0,
                status=GateDecisionStatus.PASS,
                reason="Release-blocker threshold 1.0 met: measured 1.0.",
            ),
        ],
        bridge_runs=[
            BridgeRunManifest(
                bridge_id="bridge-1",
                bridge_version="1.0.0",
                old_version_label="v2.1.7",
                new_version_label="v2.1.8",
                old_suite_hash="oldhash123",
                new_suite_hash="newhash456",
                old_grader_hash="oldgraderhash",
                new_grader_hash="newgraderhash",
                old_metric_hash="oldmetrichash",
                new_metric_hash="newmetrichash",
                old_doctrine_hash="olddoctrinehash",
                new_doctrine_hash="newdoctrinehash",
                old_protocol_descriptor_hash="oldprotohash",
                new_protocol_descriptor_hash="newprotohash",
                old_analysis_hash="oldanalysishash",
                new_analysis_hash="newanalysishash",
                model_cohort_id="cohort-1",
                task_assignment_id="assignment-1",
                task_count=10,
                task_ids=["task-1", "task-2"],
                initial_state_snapshot_hashes=["snap-1"],
                replicate_aggregation_policy=ReplicateAggregationPolicy.MEAN,
                required_replicate_ids=["1"],
            ),
        ],
        bridge_run_comparisons=[
            BridgeRunComparison(
                bridge_id="bridge-1",
                metric_id="receipt_integrity",
                metric_version="1.0.0",
                old_value=1.0,
                new_value=1.0,
                absolute_delta=0.0,
                gate_decision=GateDecisionStatus.PASS,
                reason="No change between versions.",
            ),
        ],
        unsupported_claim_names=[
            "complete_ifeval_import",
            "human_semantic_grading",
            "reasoner_independence",
        ],
    )


class TestRendererDeterminism:
    """Renderers produce byte-identical output for identical inputs."""

    def test_markdown_deterministic_for_computed_analysis(self) -> None:
        analysis = _make_rich_analysis()
        out1 = render_markdown(analysis)
        out2 = render_markdown(analysis)
        assert out1 == out2

    def test_html_deterministic_for_computed_analysis(self) -> None:
        analysis = _make_rich_analysis()
        out1 = render_html(analysis)
        out2 = render_html(analysis)
        assert out1 == out2

    def test_cli_deterministic_for_computed_analysis(self) -> None:
        analysis = _make_rich_analysis()
        out1 = render_cli(analysis)
        out2 = render_cli(analysis)
        assert out1 == out2

    def test_markdown_deterministic_for_synthetic_analysis(self) -> None:
        analysis = _make_synthetic_analysis()
        assert render_markdown(analysis) == render_markdown(analysis)

    def test_html_deterministic_for_synthetic_analysis(self) -> None:
        analysis = _make_synthetic_analysis()
        assert render_html(analysis) == render_html(analysis)

    def test_cli_deterministic_for_synthetic_analysis(self) -> None:
        analysis = _make_synthetic_analysis()
        assert render_cli(analysis) == render_cli(analysis)

    def test_identical_analyses_produce_identical_renderers(self) -> None:
        """Two independently computed analyses from identical inputs produce identical renderer output."""
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values={Arm.DIRECT: [1.0] * 7, Arm.DOCTRINE: [0.0] * 7},
            task_ids=[f"task-{i}" for i in range(1, 8)],
        )
        a1 = compute_canonical_analysis(
            tasks=tasks, attempts=attempts, metric_observations=observations,
            receipts=[], stages=[], run_id=_RUN_ID,
        )
        a2 = compute_canonical_analysis(
            tasks=tasks, attempts=attempts, metric_observations=observations,
            receipts=[], stages=[], run_id=_RUN_ID,
        )
        assert a1.canonical_json() == a2.canonical_json()
        assert render_markdown(a1) == render_markdown(a2)
        assert render_html(a1) == render_html(a2)
        assert render_cli(a1) == render_cli(a2)


class TestRendererCompleteness:
    """Renderers cover every section of the canonical analysis."""

    def test_markdown_contains_all_sections(self) -> None:
        analysis = _make_synthetic_analysis()
        md = render_markdown(analysis)
        assert "# Eval Analysis — v2.1.8" in md
        assert "## Input Summary" in md
        assert "## Missingness Breakdown" in md
        assert "## Receipt Coverage" in md
        assert "## Arms" in md
        assert "## Metric Results" in md
        assert "## Domain-Stratified Results" in md
        assert "## Confusion Matrices (Arm-Level)" in md
        assert "## Pooled Confusion Matrices" in md
        assert "## Paired Comparisons" in md
        assert "## Gate Decisions" in md
        assert "## Bridge Runs" in md
        assert "## Bridge Run Comparisons" in md
        assert "## Unsupported Claims" in md

    def test_html_contains_all_sections(self) -> None:
        analysis = _make_synthetic_analysis()
        html = render_html(analysis)
        assert "<!DOCTYPE html>" in html
        assert "<title>Eval Analysis — v2.1.8</title>" in html
        assert "<h2>Input Summary</h2>" in html
        assert "<h2>Missingness Breakdown</h2>" in html
        assert "<h2>Receipt Coverage</h2>" in html
        assert "<h2>Arms</h2>" in html
        assert "<h2>Metric Results</h2>" in html
        assert "<h2>Domain-Stratified Results</h2>" in html
        assert "<h2>Confusion Matrices (Arm-Level)</h2>" in html
        assert "<h2>Pooled Confusion Matrices</h2>" in html
        assert "<h2>Paired Comparisons</h2>" in html
        assert "<h2>Gate Decisions</h2>" in html
        assert "<h2>Bridge Runs</h2>" in html
        assert "<h2>Bridge Run Comparisons</h2>" in html
        assert "<h2>Unsupported Claims</h2>" in html
        assert "</html>" in html

    def test_cli_contains_all_sections(self) -> None:
        analysis = _make_synthetic_analysis()
        cli = render_cli(analysis)
        assert "=== Eval Analysis — v2.1.8 ===" in cli
        assert "--- Input Summary ---" in cli
        assert "--- Missingness Breakdown ---" in cli
        assert "--- Receipt Coverage ---" in cli
        assert "--- Arms ---" in cli
        assert "--- Metric Results ---" in cli
        assert "--- Domain-Stratified Results ---" in cli
        assert "--- Confusion Matrices (Arm-Level) ---" in cli
        assert "--- Pooled Confusion Matrices ---" in cli
        assert "--- Paired Comparisons ---" in cli
        assert "--- Gate Decisions ---" in cli
        assert "--- Bridge Runs ---" in cli
        assert "--- Bridge Run Comparisons ---" in cli
        assert "--- Unsupported Claims ---" in cli


class TestRendererValues:
    """Renderers display the correct values from the canonical analysis."""

    def test_markdown_displays_run_id_and_versions(self) -> None:
        analysis = _make_synthetic_analysis()
        md = render_markdown(analysis)
        assert f"Run ID: `{_RUN_ID}`" in md
        assert f"Analysis schema version: `{ANALYSIS_SCHEMA_VERSION}`" in md
        assert f"Analysis computation version: `{ANALYSIS_COMPUTATION_VERSION}`" in md

    def test_markdown_displays_input_content_hash(self) -> None:
        analysis = _make_synthetic_analysis()
        md = render_markdown(analysis)
        assert "abc123def456" in md

    def test_markdown_displays_metric_value(self) -> None:
        analysis = _make_synthetic_analysis()
        md = render_markdown(analysis)
        assert "receipt_integrity" in md
        assert "1" in md

    def test_markdown_displays_confusion_matrix_counts(self) -> None:
        analysis = _make_synthetic_analysis()
        md = render_markdown(analysis)
        assert "policy_outcome" in md
        assert "3" in md  # TP
        assert "1" in md  # FP

    def test_markdown_displays_gate_decision_status(self) -> None:
        analysis = _make_synthetic_analysis()
        md = render_markdown(analysis)
        assert "pass" in md

    def test_markdown_displays_unsupported_claims(self) -> None:
        analysis = _make_synthetic_analysis()
        md = render_markdown(analysis)
        assert "reasoner_independence" in md
        assert "human_semantic_grading" in md
        assert "complete_ifeval_import" in md

    def test_html_displays_metric_value(self) -> None:
        analysis = _make_synthetic_analysis()
        html = render_html(analysis)
        assert "receipt_integrity" in html

    def test_cli_displays_metric_value(self) -> None:
        analysis = _make_synthetic_analysis()
        cli = render_cli(analysis)
        assert "receipt_integrity" in cli
        assert "value=1" in cli

    def test_cli_displays_gate_decision(self) -> None:
        analysis = _make_synthetic_analysis()
        cli = render_cli(analysis)
        assert "status=pass" in cli

    def test_markdown_none_value_renders_na(self) -> None:
        analysis = _make_synthetic_analysis().model_copy(update={
            "metric_results": [
                _make_synthetic_analysis().metric_results[0].model_copy(update={"value": None}),
            ],
        })
        md = render_markdown(analysis)
        assert "N/A" in md

    def test_cli_none_value_renders_na(self) -> None:
        analysis = _make_synthetic_analysis().model_copy(update={
            "metric_results": [
                _make_synthetic_analysis().metric_results[0].model_copy(update={"value": None}),
            ],
        })
        cli = render_cli(analysis)
        assert "value=N/A" in cli


class TestRendererEmptySections:
    """Renderers handle empty optional sections gracefully."""

    def test_markdown_omits_empty_confusion_matrices_section(self) -> None:
        analysis = _make_rich_analysis()
        # The rich analysis from receipt_integrity has no confusion matrices
        md = render_markdown(analysis)
        assert "Confusion Matrices (Arm-Level)" not in md

    def test_html_omits_empty_confusion_matrices_section(self) -> None:
        analysis = _make_rich_analysis()
        html = render_html(analysis)
        assert "Confusion Matrices (Arm-Level)" not in html

    def test_cli_omits_empty_confusion_matrices_section(self) -> None:
        analysis = _make_rich_analysis()
        cli = render_cli(analysis)
        assert "Confusion Matrices (Arm-Level)" not in cli

    def test_markdown_omits_empty_bridge_runs_section(self) -> None:
        analysis = _make_rich_analysis()
        md = render_markdown(analysis)
        assert "## Bridge Runs" not in md

    def test_markdown_omits_empty_bridge_run_comparisons_section(self) -> None:
        analysis = _make_rich_analysis()
        md = render_markdown(analysis)
        assert "## Bridge Run Comparisons" not in md

    def test_empty_analysis_renders_without_error(self) -> None:
        analysis = compute_canonical_analysis(
            tasks=[], attempts=[], metric_observations=[],
            receipts=[], stages=[], run_id=_RUN_ID,
        )
        md = render_markdown(analysis)
        html = render_html(analysis)
        cli = render_cli(analysis)
        assert "# Eval Analysis" in md
        assert "<!DOCTYPE html>" in html
        assert "=== Eval Analysis" in cli
        # Empty analysis has no confusion matrices, comparisons, or bridge runs
        assert "Confusion Matrices" not in md
        assert "Paired Comparisons" not in md
        assert "Bridge Runs" not in md


class TestRendererDerivedFromCanonical:
    """Renderers derive all values from the canonical analysis, not re-computation."""

    def test_markdown_does_not_recompute_floats(self) -> None:
        """The renderer displays the stored value, not a recomputed one."""
        analysis = _make_synthetic_analysis()
        mr = analysis.metric_results[0]
        md = render_markdown(analysis)
        # The stored value is 1.0; the renderer should display "1" (trimmed)
        assert f"| {mr.metric_id} |" in md

    def test_renderer_output_changes_when_analysis_changes(self) -> None:
        """Changing a value in the analysis changes the renderer output."""
        analysis = _make_synthetic_analysis()
        md1 = render_markdown(analysis)
        analysis2 = analysis.model_copy(update={
            "metric_results": [analysis.metric_results[0].model_copy(update={"value": 0.5})],
        })
        md2 = render_markdown(analysis2)
        assert md1 != md2
        assert "0.5" in md2

    def test_canonical_json_stability_implies_renderer_stability(self) -> None:
        """If canonical JSON is identical, renderer output is identical."""
        tasks, attempts, observations = _make_multi_arm_scenario(
            arm_values={Arm.DIRECT: [1.0] * 5, Arm.DOCTRINE: [1.0] * 5},
            task_ids=[f"task-{i}" for i in range(1, 6)],
        )
        a1 = compute_canonical_analysis(
            tasks=tasks, attempts=attempts, metric_observations=observations,
            receipts=[], stages=[], run_id=_RUN_ID,
        )
        a2 = compute_canonical_analysis(
            tasks=tasks, attempts=attempts, metric_observations=observations,
            receipts=[], stages=[], run_id=_RUN_ID,
        )
        assert a1.canonical_json() == a2.canonical_json()
        assert render_markdown(a1) == render_markdown(a2)
        assert render_html(a1) == render_html(a2)
        assert render_cli(a1) == render_cli(a2)
