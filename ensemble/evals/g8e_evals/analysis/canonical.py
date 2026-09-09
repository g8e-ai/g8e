# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Versioned canonical machine-readable eval analysis.

Replaces the descriptive ``Aggregate`` model and legacy ``summary.json``
output with a typed, versioned, byte-deterministic analysis built only
from immutable task, attempt, observation, receipt, stage, and
``MetricObservation`` records.

The canonical analysis is the single source of truth for release
reporting. Markdown, HTML, and CLI views are derived from it, never
computed independently. Identical immutable inputs and analysis version
produce byte-identical analysis and renderer output.

Key properties:

- **Versioned**: ``analysis_version`` pins the analysis schema and
  computation. Breaking changes increment the major version.
- **Deterministic**: all lists are sorted by stable keys. Floating-point
  values are rounded to a fixed precision. Canonical JSON serialization
  (``model_dump_json`` with sorted keys) produces byte-identical output
  for identical inputs.
- **Denominator-preserving**: every assigned attempt appears in its
  declared denominator regardless of outcome. Completed, model-failed,
  governance-rejected, human-denied, timed-out, infrastructure-failed,
  and invalid-evidence attempts are all retained.
- **Evidence-linked**: every metric result references the
  ``MetricObservation`` rows that fed it. No free-form metadata enters
  the analysis.
- **Gate-deciding**: every headline metric has a typed gate decision
  with a practical threshold. Statistical significance alone cannot pass
  a release.
"""

from __future__ import annotations

from enum import StrEnum

from pydantic import BaseModel, ConfigDict, Field

from g8e_evals.metrics import MetricDirection
from g8e_evals.release_metric_set import MetricDomain


ANALYSIS_SCHEMA_VERSION = "1.0.0"
ANALYSIS_COMPUTATION_VERSION = "1.0.0"

_FLOAT_PRECISION = 10


class GateDecisionStatus(StrEnum):
    """Typed outcome of a practical-threshold gate check."""

    PASS = "pass"
    FAIL = "fail"
    NOT_APPLICABLE = "not_applicable"
    INSUFFICIENT_DATA = "insufficient_data"
    UNSUPPORTED = "unsupported"


class ComparisonDirection(StrEnum):
    """Direction of a paired comparison delta."""

    IMPROVEMENT = "improvement"
    REGRESSION = "regression"
    NEUTRAL = "neutral"


class MissingnessReason(StrEnum):
    """Typed reason an attempt did not produce a passing result."""

    COMPLETED = "completed"
    MODEL_FAILED = "model_failed"
    GOVERNANCE_REJECTED = "governance_rejected"
    HUMAN_DENIED = "human_denied"
    TIMED_OUT = "timed_out"
    INFRASTRUCTURE_FAILED = "infrastructure_failed"
    INVALID_EVIDENCE = "invalid_evidence"
    NOT_ELIGIBLE = "not_eligible"
    NOT_APPLICABLE = "not_applicable"


class AnalysisInputSummary(BaseModel):
    """Summary of the immutable records that fed this analysis.

    Every field is a count or a content hash, never a free-form value.
    The content hash is computed over the canonical JSON of all input
    records sorted by their natural key, ensuring that identical inputs
    produce identical hashes.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    task_count: int = Field(ge=0)
    attempt_count: int = Field(ge=0)
    observation_count: int = Field(ge=0)
    receipt_count: int = Field(ge=0)
    stage_count: int = Field(ge=0)
    metric_observation_count: int = Field(ge=0)
    input_content_hash: str = Field(min_length=1, description="SHA-256 over canonical JSON of all sorted input records.")


class MissingnessBreakdown(BaseModel):
    """Breakdown of attempt outcomes by terminal status.

    Every assigned attempt appears in exactly one bucket. No attempt
    disappears from analysis because it lacks a passing result.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    completed: int = Field(ge=0)
    model_failed: int = Field(ge=0)
    governance_rejected: int = Field(ge=0)
    human_denied: int = Field(ge=0)
    timed_out: int = Field(ge=0)
    infrastructure_failed: int = Field(ge=0)
    invalid_evidence: int = Field(ge=0)

    @property
    def total(self) -> int:
        return (
            self.completed
            + self.model_failed
            + self.governance_rejected
            + self.human_denied
            + self.timed_out
            + self.infrastructure_failed
            + self.invalid_evidence
        )


class ReceiptCoverageAnalysis(BaseModel):
    """Receipt coverage computed only over receipt-eligible mutation attempts.

    An attempt is receipt-eligible when the arm supports receipt binding
    (governed arms: doctrine, consensus, notary) and the task declares an
    expected action class (mutation action). Ungoverned arms and
    answer-only turns are excluded from the denominator by design.

    Receipt coverage is the proportion of receipt-eligible attempts that
    produced a verified primary receipt. Receipt verification is the
    proportion of receipt-bound attempts whose receipts all verified.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    eligible_attempt_count: int = Field(ge=0, description="Attempts on governed arms with a declared expected action class.")
    receipt_bound_count: int = Field(ge=0, description="Eligible attempts that produced a primary receipt.")
    receipt_verified_count: int = Field(ge=0, description="Receipt-bound attempts whose receipts all verified.")

    @property
    def coverage_pct(self) -> float:
        if self.eligible_attempt_count == 0:
            return 0.0
        return round((self.receipt_bound_count / self.eligible_attempt_count) * 100.0, _FLOAT_PRECISION)

    @property
    def verification_pct(self) -> float:
        if self.receipt_bound_count == 0:
            return 0.0
        return round((self.receipt_verified_count / self.receipt_bound_count) * 100.0, _FLOAT_PRECISION)


class ConfusionMatrix(BaseModel):
    """Typed allow/block confusion matrix for one arm and metric.

    Used for policy_outcome, policy_attack, and any binary allow/block
    metric. Published arm-level before pooled scores.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    metric_id: str = Field(min_length=1)
    metric_version: str = Field(min_length=1)
    arm_id: str = Field(min_length=1)
    domain: MetricDomain

    true_positive: int = Field(ge=0, description="Expected allow, observed allow.")
    false_positive: int = Field(ge=0, description="Expected block, observed allow.")
    true_negative: int = Field(ge=0, description="Expected block, observed block.")
    false_negative: int = Field(ge=0, description="Expected allow, observed block.")

    @property
    def total(self) -> int:
        return self.true_positive + self.false_positive + self.true_negative + self.false_negative

    @property
    def accuracy(self) -> float:
        if self.total == 0:
            return 0.0
        return round((self.true_positive + self.true_negative) / self.total, _FLOAT_PRECISION)

    @property
    def balanced_accuracy(self) -> float:
        tpr = self.true_positive / (self.true_positive + self.false_negative) if (self.true_positive + self.false_negative) > 0 else 0.0
        tnr = self.true_negative / (self.true_negative + self.false_positive) if (self.true_negative + self.false_positive) > 0 else 0.0
        return round((tpr + tnr) / 2.0, _FLOAT_PRECISION)

    @property
    def matthews_correlation_coefficient(self) -> float:
        tp = self.true_positive
        fp = self.false_positive
        tn = self.true_negative
        fn = self.false_negative
        numerator = tp * tn - fp * fn
        denominator_sq = (tp + fp) * (tp + fn) * (tn + fp) * (tn + fn)
        if denominator_sq == 0:
            return 0.0
        return round(numerator / (denominator_sq ** 0.5), _FLOAT_PRECISION)


class PooledConfusionMatrix(BaseModel):
    """Preregistered pooled confusion matrix across all arms for one metric.

    Sums the arm-level ``ConfusionMatrix`` TP/FP/TN/FN counts into a
    single pooled estimate. This is the preregistered pooling approach:
    the pooling rule (simple summation across arms) is declared before
    analysis and applied uniformly. A hierarchical model that accounts
    for arm-level variance is not implemented in v2.1.8 and remains an
    explicit unsupported option.

    Published after arm-level confusion matrices so that readers can
    inspect model-specific results before the pooled score.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    metric_id: str = Field(min_length=1)
    metric_version: str = Field(min_length=1)
    domain: MetricDomain

    arm_count: int = Field(ge=0, description="Number of arm-level matrices pooled.")
    arm_ids: list[str] = Field(
        default_factory=list,
        description="Sorted arm IDs whose matrices were pooled.",
    )

    true_positive: int = Field(ge=0)
    false_positive: int = Field(ge=0)
    true_negative: int = Field(ge=0)
    false_negative: int = Field(ge=0)

    pooling_method: str = Field(
        min_length=1,
        description="Declared pooling rule applied before analysis.",
    )

    @property
    def total(self) -> int:
        return self.true_positive + self.false_positive + self.true_negative + self.false_negative

    @property
    def accuracy(self) -> float:
        if self.total == 0:
            return 0.0
        return round((self.true_positive + self.true_negative) / self.total, _FLOAT_PRECISION)

    @property
    def balanced_accuracy(self) -> float:
        tpr = self.true_positive / (self.true_positive + self.false_negative) if (self.true_positive + self.false_negative) > 0 else 0.0
        tnr = self.true_negative / (self.true_negative + self.false_positive) if (self.true_negative + self.false_positive) > 0 else 0.0
        return round((tpr + tnr) / 2.0, _FLOAT_PRECISION)

    @property
    def matthews_correlation_coefficient(self) -> float:
        tp = self.true_positive
        fp = self.false_positive
        tn = self.true_negative
        fn = self.false_negative
        numerator = tp * tn - fp * fn
        denominator_sq = (tp + fp) * (tp + fn) * (tn + fp) * (tn + fn)
        if denominator_sq == 0:
            return 0.0
        return round(numerator / (denominator_sq ** 0.5), _FLOAT_PRECISION)


class MetricAnalysisResult(BaseModel):
    """Aggregated result for one metric across one arm.

    Built only from ``MetricObservation`` rows. The denominator includes
    every eligible attempt regardless of outcome. Missing, denied, and
    failed attempts are retained in the denominator per the missingness
    policy.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    metric_id: str = Field(min_length=1)
    metric_version: str = Field(min_length=1)
    arm_id: str = Field(min_length=1)
    domain: MetricDomain

    direction: MetricDirection
    unit: str = Field(min_length=1)

    numerator: float = Field(ge=0.0)
    denominator: int = Field(ge=0)
    value: float | None = Field(default=None, description="Aggregate value (numerator/denominator for proportions, mean for continuous). None when denominator is zero.")

    eligible_count: int = Field(ge=0)
    not_eligible_count: int = Field(ge=0)
    missing_count: int = Field(ge=0, description="Eligible attempts with no MetricObservation or a None value.")

    verification_status_counts: dict[str, int] = Field(
        default_factory=dict,
        description="Counts by VerificationStatus value string. Sorted by key in canonical output.",
    )

    evidence_ref_count: int = Field(ge=0, description="Total evidence references across all MetricObservation rows for this metric and arm.")

    metric_observation_ids: list[str] = Field(
        default_factory=list,
        description="Sorted list of MetricObservation IDs that fed this result.",
    )


class DomainStratifiedResult(BaseModel):
    """Per-domain aggregated result across one arm.

    Groups metric results by ``MetricDomain`` for domain-stratified
    reporting. Published before pooled scores.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    arm_id: str = Field(min_length=1)
    domain: MetricDomain
    metric_count: int = Field(ge=0)
    passing_metric_count: int = Field(ge=0, description="Metrics where value meets the practical threshold.")
    failing_metric_count: int = Field(ge=0)
    not_applicable_metric_count: int = Field(ge=0)


class NonInferiorityMargin(BaseModel):
    """Typed non-inferiority margin for a release metric.

    Declares the maximum acceptable degradation when comparing a new arm
    against a reference arm. Non-inferiority is assessed via the bootstrap
    confidence interval of the paired delta: the comparison is
    non-inferior when the CI bound does not cross the margin.

    A margin of 0.0 means any degradation is unacceptable (equivalent to
    a strict release-blocker threshold in the paired setting).
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    metric_id: str = Field(min_length=1)
    metric_version: str = Field(min_length=1)
    margin: float = Field(ge=0.0, description="Maximum acceptable absolute degradation in the comparison direction.")
    description: str = Field(min_length=1)


class PairedComparison(BaseModel):
    """Paired comparison between two arms over identical task instances.

    Includes absolute and relative deltas, effect size, and a typed
    gate decision. Built only from paired ``MetricObservation`` rows
    on identical task instances and initial-state snapshots.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    metric_id: str = Field(min_length=1)
    metric_version: str = Field(min_length=1)

    baseline_arm_id: str = Field(min_length=1)
    comparison_arm_id: str = Field(min_length=1)

    paired_count: int = Field(ge=0, description="Number of task instances with observations in both arms.")

    baseline_value: float | None = None
    comparison_value: float | None = None

    absolute_delta: float | None = None
    relative_delta: float | None = None
    standardized_effect_size: float | None = None

    direction: ComparisonDirection = ComparisonDirection.NEUTRAL

    mcnemar_statistic: float | None = None
    mcnemar_p_value: float | None = None

    bootstrap_ci_lower: float | None = None
    bootstrap_ci_upper: float | None = None

    holm_corrected_p_value: float | None = None
    holm_rank: int | None = None

    non_inferiority_margin: float | None = Field(
        default=None,
        description="Non-inferiority margin for this metric, if defined. None when no margin is declared.",
    )

    gate_decision: GateDecisionStatus = GateDecisionStatus.INSUFFICIENT_DATA


class GateDecision(BaseModel):
    """Typed practical-threshold gate decision for one metric and arm.

    Statistical significance alone cannot pass a release. Every headline
    metric has a gate decision with a practical threshold or
    non-inferiority margin.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    metric_id: str = Field(min_length=1)
    metric_version: str = Field(min_length=1)
    arm_id: str = Field(min_length=1)

    threshold_description: str = Field(min_length=1)
    measured_value: float | None = None
    threshold_value: float | None = None
    non_inferiority_margin: float | None = None

    status: GateDecisionStatus
    reason: str = Field(min_length=1)


class BridgeRunManifest(BaseModel):
    """Immutable manifest for a bridge run between suite, grader, metric,
    doctrine, or analysis versions.

    Bridge runs execute old and new versions over the same model cohort
    before combining or comparing results. This manifest records the
    versions and content hashes that define the bridge.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    bridge_id: str = Field(min_length=1)
    bridge_version: str = Field(min_length=1)

    old_version_label: str = Field(min_length=1)
    new_version_label: str = Field(min_length=1)

    old_suite_hash: str = Field(min_length=1)
    new_suite_hash: str = Field(min_length=1)

    model_cohort_id: str = Field(min_length=1)
    task_count: int = Field(ge=0)

    old_analysis_hash: str | None = None
    new_analysis_hash: str | None = None


class BridgeRunComparison(BaseModel):
    """Typed comparison between old and new analysis versions over the same model cohort.

    Built from two ``CanonicalEvalAnalysis`` instances produced by
    executing old and new suite, grader, metric, doctrine, or analysis
    versions over the same model cohort. Records per-metric deltas and
    a gate decision that fails when any release-blocker metric regresses
    beyond its non-inferiority margin.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    bridge_id: str = Field(min_length=1)
    metric_id: str = Field(min_length=1)
    metric_version: str = Field(min_length=1)

    old_value: float | None = None
    new_value: float | None = None
    absolute_delta: float | None = None

    gate_decision: GateDecisionStatus = GateDecisionStatus.INSUFFICIENT_DATA
    reason: str = Field(min_length=1)


class CanonicalEvalAnalysis(BaseModel):
    """Versioned canonical machine-readable eval analysis.

    The single source of truth for release reporting. Built only from
    immutable task, attempt, observation, receipt, stage, and
    ``MetricObservation`` records. Identical inputs and analysis version
    produce byte-identical output.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    analysis_schema_version: str = Field(min_length=1)
    analysis_computation_version: str = Field(min_length=1)

    release_version: str = Field(min_length=1)
    run_id: str = Field(min_length=1)

    input_summary: AnalysisInputSummary
    missingness: MissingnessBreakdown
    receipt_coverage: ReceiptCoverageAnalysis

    arm_ids: list[str] = Field(
        default_factory=list,
        description="Sorted unique arm IDs in this analysis.",
    )

    metric_results: list[MetricAnalysisResult] = Field(
        default_factory=list,
        description="Per-metric per-arm aggregated results, sorted by (metric_id, arm_id).",
    )

    domain_stratified_results: list[DomainStratifiedResult] = Field(
        default_factory=list,
        description="Per-domain per-arm stratified results, sorted by (arm_id, domain).",
    )

    confusion_matrices: list[ConfusionMatrix] = Field(
        default_factory=list,
        description="Allow/block confusion matrices, sorted by (metric_id, arm_id).",
    )

    pooled_confusion_matrices: list[PooledConfusionMatrix] = Field(
        default_factory=list,
        description="Preregistered pooled confusion matrices across arms, sorted by (metric_id, metric_version).",
    )

    comparisons: list[PairedComparison] = Field(
        default_factory=list,
        description="Paired comparisons between arms, sorted by (metric_id, baseline_arm_id, comparison_arm_id).",
    )

    gate_decisions: list[GateDecision] = Field(
        default_factory=list,
        description="Practical-threshold gate decisions, sorted by (metric_id, arm_id).",
    )

    bridge_runs: list[BridgeRunManifest] = Field(
        default_factory=list,
        description="Bridge-run manifests, sorted by bridge_id.",
    )

    bridge_run_comparisons: list[BridgeRunComparison] = Field(
        default_factory=list,
        description="Bridge-run per-metric comparisons, sorted by (bridge_id, metric_id, metric_version).",
    )

    unsupported_claim_names: list[str] = Field(
        default_factory=list,
        description="Sorted names of unsupported claims carried from the release metric set.",
    )

    def canonical_json(self) -> str:
        """Return canonical JSON with sorted keys and no extra whitespace.

        This is the byte-deterministic serialization. Identical inputs
        and analysis version produce byte-identical output.
        """
        return self.model_dump_json(
            by_alias=True,
            exclude_none=False,
            indent=0,
        )


__all__ = [
    "ANALYSIS_COMPUTATION_VERSION",
    "ANALYSIS_SCHEMA_VERSION",
    "AnalysisInputSummary",
    "BridgeRunComparison",
    "BridgeRunManifest",
    "CanonicalEvalAnalysis",
    "ComparisonDirection",
    "ConfusionMatrix",
    "DomainStratifiedResult",
    "GateDecision",
    "GateDecisionStatus",
    "MetricAnalysisResult",
    "MissingnessBreakdown",
    "MissingnessReason",
    "NonInferiorityMargin",
    "PairedComparison",
    "PooledConfusionMatrix",
    "ReceiptCoverageAnalysis",
]
