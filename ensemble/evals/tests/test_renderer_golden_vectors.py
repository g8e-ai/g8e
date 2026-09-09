# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 2 golden-vector tests for the canonical analysis renderers.

Pins the Markdown, HTML, and CLI renderer output to committed frozen
fixture files so that any unintended change to the renderer output is
detected. The golden files live under
``tests/fixtures/golden/renderers/`` and are regenerated deliberately
via the ``--regenerate-golden-vectors`` command-line option.

The golden vectors cover three scenarios:

- **full**: a synthetic analysis with every optional section populated
  (confusion matrices, pooled confusion matrices, paired comparisons,
  bridge runs, bridge run comparisons, unsupported claims).
- **empty**: a minimal analysis with no optional sections, verifying
  the base-case renderer output.
- **computed**: an analysis produced by the computation engine from
  immutable records, verifying the engine-to-renderer path.
"""

from __future__ import annotations

from pathlib import Path

import pytest

pytestmark = pytest.mark.integration

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

_FIXTURE_DIR = Path(__file__).resolve().parent / "fixtures" / "golden" / "renderers"

_RUN_ID = "run-golden-1"
_TASK_ID = "task-golden-1"
_ATTEMPT_ID = "attempt-golden-1"


def _make_full_synthetic_analysis() -> CanonicalEvalAnalysis:
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
                model_cohort_id="cohort-1",
                task_count=10,
                old_analysis_hash="oldanalysishash",
                new_analysis_hash="newanalysishash",
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


def _make_empty_synthetic_analysis() -> CanonicalEvalAnalysis:
    """Build a minimal analysis with no optional sections."""
    return CanonicalEvalAnalysis(
        analysis_schema_version=ANALYSIS_SCHEMA_VERSION,
        analysis_computation_version=ANALYSIS_COMPUTATION_VERSION,
        release_version="v2.1.8",
        run_id="run-empty-1",
        input_summary=AnalysisInputSummary(
            task_count=0,
            attempt_count=0,
            observation_count=0,
            receipt_count=0,
            stage_count=0,
            metric_observation_count=0,
            input_content_hash="0000000000000000000000000000000000000000000000000000000000000000",
        ),
        missingness=MissingnessBreakdown(
            completed=0,
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
        arm_ids=[],
        metric_results=[],
        domain_stratified_results=[],
        confusion_matrices=[],
        pooled_confusion_matrices=[],
        comparisons=[],
        gate_decisions=[],
        bridge_runs=[],
        bridge_run_comparisons=[],
        unsupported_claim_names=[],
    )


def _make_computed_analysis() -> CanonicalEvalAnalysis:
    """Build an analysis via the computation engine from immutable records."""
    from g8e_evals.schema import GraderReference, RejectionLayer

    task = TaskDefinition(
        task_id=_TASK_ID,
        suite_id="utility",
        suite_version="1.0.0",
        prompt_hash="abc123",
        prompt_length=10,
        expected_action_class="TEST_ACTION",
        compatible_arms=[Arm.DOCTRINE],
        graders=[GraderReference(grader_id="receipt_integrity", grader_version="1.0.0")],
        expected_allow_block_outcome=PolicyOutcome.BLOCK,
        expected_rejection_layer=RejectionLayer.L1_DOCTRINE,
    )
    attempt = AttemptRecord(
        attempt_id=_ATTEMPT_ID,
        run_id=_RUN_ID,
        task_id=_TASK_ID,
        arm_id=Arm.DOCTRINE,
        terminal_status=TerminalStatus.COMPLETED,
    )
    observation = MetricObservation(
        metric_id="receipt_integrity",
        metric_version="1.0.0",
        attempt_id=_ATTEMPT_ID,
        run_id=_RUN_ID,
        arm_id=Arm.DOCTRINE,
        task_id=_TASK_ID,
        value=1.0,
        unit="boolean",
        eligible=True,
        verification_status=VerificationStatus.VERIFIED,
        grader_class=GraderClass.DETERMINISTIC,
    )
    return compute_canonical_analysis(
        tasks=[task],
        attempts=[attempt],
        metric_observations=[observation],
        receipts=[],
        stages=[],
        run_id=_RUN_ID,
    )


_SCENARIOS: list[tuple[str, str, CanonicalEvalAnalysis]] = []


def _register_scenarios() -> None:
    global _SCENARIOS
    _SCENARIOS = [
        ("full", "synthetic_full", _make_full_synthetic_analysis()),
        ("empty", "synthetic_empty", _make_empty_synthetic_analysis()),
        ("computed", "computed_single", _make_computed_analysis()),
    ]


_register_scenarios()

_RENDERER_FUNCS = [
    ("markdown", ".md", render_markdown),
    ("html", ".html", render_html),
    ("cli", ".cli", render_cli),
]


def _golden_path(scenario_name: str, ext: str) -> Path:
    return _FIXTURE_DIR / f"{scenario_name}{ext}"


class TestGoldenVectors:
    """Renderer output matches committed golden-vector fixture files byte-for-byte."""

    @pytest.mark.parametrize(
        ("scenario_label", "scenario_name", "analysis"),
        [(label, name, analysis) for label, name, analysis in _SCENARIOS],
        ids=[name for _, name, _ in _SCENARIOS],
    )
    @pytest.mark.parametrize(
        ("renderer_label", "ext", "render"),
        _RENDERER_FUNCS,
        ids=[label for label, _, _ in _RENDERER_FUNCS],
    )
    def test_renderer_matches_golden_vector(
        self,
        request: pytest.FixtureRequest,
        scenario_label: str,
        scenario_name: str,
        analysis: CanonicalEvalAnalysis,
        renderer_label: str,
        ext: str,
        render: object,
    ) -> None:
        if request.config.getoption("--regenerate-golden-vectors"):
            pytest.skip("Golden vectors are being regenerated, not tested.")
        golden = _golden_path(scenario_name, ext)
        assert golden.exists(), (
            f"Golden vector file missing: {golden}. "
            "Run pytest with --regenerate-golden-vectors to create it."
        )
        expected = golden.read_text(encoding="utf-8")
        actual = render(analysis)  # type: ignore[operator]
        assert actual == expected, (
            f"Renderer '{renderer_label}' output for scenario '{scenario_name}' "
            f"does not match golden vector {golden}. "
            f"Run pytest with --regenerate-golden-vectors to update after an "
            f"intentional renderer or analysis model change."
        )

    @pytest.mark.parametrize(
        ("scenario_label", "scenario_name", "analysis"),
        [(label, name, analysis) for label, name, analysis in _SCENARIOS],
        ids=[name for _, name, _ in _SCENARIOS],
    )
    def test_golden_vector_non_empty(
        self,
        request: pytest.FixtureRequest,
        scenario_label: str,
        scenario_name: str,
        analysis: CanonicalEvalAnalysis,
    ) -> None:
        if request.config.getoption("--regenerate-golden-vectors"):
            pytest.skip("Golden vectors are being regenerated, not tested.")
        for _, ext, render in _RENDERER_FUNCS:
            golden = _golden_path(scenario_name, ext)
            assert golden.exists(), f"Golden vector file missing: {golden}"
            content = golden.read_text(encoding="utf-8")
            assert len(content) > 0, f"Golden vector {golden} is empty"
            actual = render(analysis)  # type: ignore[operator]
            assert len(actual) > 0, f"Renderer output for {golden} is empty"

    @pytest.mark.parametrize(
        ("scenario_label", "scenario_name", "analysis"),
        [(label, name, analysis) for label, name, analysis in _SCENARIOS],
        ids=[name for _, name, _ in _SCENARIOS],
    )
    def test_golden_vector_covers_all_sections(
        self,
        request: pytest.FixtureRequest,
        scenario_label: str,
        scenario_name: str,
        analysis: CanonicalEvalAnalysis,
    ) -> None:
        """The golden vector for the full scenario covers every section."""
        if request.config.getoption("--regenerate-golden-vectors"):
            pytest.skip("Golden vectors are being regenerated, not tested.")
        if scenario_label != "full":
            pytest.skip("Section coverage check only applies to the full scenario.")
        md = _golden_path(scenario_name, ".md").read_text(encoding="utf-8")
        html = _golden_path(scenario_name, ".html").read_text(encoding="utf-8")
        cli = _golden_path(scenario_name, ".cli").read_text(encoding="utf-8")
        for section in [
            "Input Summary",
            "Missingness Breakdown",
            "Receipt Coverage",
            "Arms",
            "Metric Results",
            "Domain-Stratified Results",
            "Confusion Matrices",
            "Pooled Confusion Matrices",
            "Paired Comparisons",
            "Gate Decisions",
            "Bridge Runs",
            "Bridge Run Comparisons",
            "Unsupported Claims",
        ]:
            assert section in md, f"Markdown golden vector missing section: {section}"
            assert section in html, f"HTML golden vector missing section: {section}"
            assert section in cli, f"CLI golden vector missing section: {section}"


def test_regenerate_golden_vectors(
    request: pytest.FixtureRequest,
    tmp_path: Path,
) -> None:
    """Regenerate golden-vector fixture files when --regenerate-golden-vectors is passed.

    This test writes the fixture files in place and passes when regeneration
    is requested. When the flag is not set, this test is skipped.
    """
    if not request.config.getoption("--regenerate-golden-vectors"):
        pytest.skip("Not regenerating golden vectors")
    _FIXTURE_DIR.mkdir(parents=True, exist_ok=True)
    for _, scenario_name, analysis in _SCENARIOS:
        for _, ext, render in _RENDERER_FUNCS:
            golden = _golden_path(scenario_name, ext)
            content = render(analysis)  # type: ignore[operator]
            golden.write_text(content, encoding="utf-8")
    # Verify all files were written
    for _, scenario_name, _ in _SCENARIOS:
        for _, ext, _ in _RENDERER_FUNCS:
            golden = _golden_path(scenario_name, ext)
            assert golden.exists(), f"Failed to write golden vector: {golden}"
            assert golden.stat().st_size > 0, f"Golden vector is empty: {golden}"
