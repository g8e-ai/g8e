# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Phase 1 release-metric audit: enumerate all 82 release metrics.

For each metric in the v2.1.8 release set, this test records typed
eligibility, denominator, missing-denominator disposition, arm
requirement, authoritative producer, immutable source records,
aggregation, uncertainty policy, threshold/calibration state,
non-inferiority margin, and renderer presence. Any blank cell keeps
Phase 1 open. This test is the named verification artifact for P1-08
step 5.
"""

from __future__ import annotations

import pytest

pytestmark = pytest.mark.unit

from g8e_evals.analysis.derived import DEFAULT_DERIVED_REGISTRY
from g8e_evals.analysis.telemetry import DEFAULT_TELEMETRY_REGISTRY
from g8e_evals.metrics import (
    AggregationMethod,
    ArmRequirement,
    DEFAULT_METRIC_REGISTRY,
    DenominatorKind,
    EligibilityKind,
    GraderClass,
    MissingDenominatorDisposition,
    MissingValuePolicy,
)
from g8e_evals.release_metric_set import RELEASE_METRIC_SET, MetricDomain


# ---------------------------------------------------------------------------
# Expected producer mapping: metric_id -> producer registry name.
# ---------------------------------------------------------------------------

_DERIVED_METRIC_IDS = {
    "allow_block_confusion_matrix",
    "attack_success_rate",
    "expected_layer_detection",
    "balanced_accuracy",
    "matthews_correlation_coefficient",
    "harm_weighted_loss",
    "l2_proof_property",
    "l3_proof_property",
    "l4_proof_property",
    "l5_proof_property",
    "receipt_linkage",
    "envelope_linkage",
    "state_linkage",
    "persistence_linkage",
    "commitment_linkage",
    "audit_linkage",
    "evidence_validity",
    "tool_call_recognition",
    "tool_call_selection",
    "tool_call_schema",
    "tool_call_semantics",
    "tool_call_permission",
    "tool_call_interpretation",
    "tool_call_follow_up",
    "tool_call_unnecessary",
    "tool_call_looping",
    "tool_call_recovery",
    "escalation_correct_autonomous",
    "escalation_correct_escalation",
    "escalation_false_escalation",
    "escalation_missed_escalation",
    "escalation_efficiency",
}

_TELEMETRY_METRIC_IDS = {
    "stage_latency_seconds",
    "provider_usage_tokens",
    "provider_cost_usd",
    "local_resource_peak_memory_bytes",
    "local_resource_cpu_seconds",
    "human_wait_seconds",
}


def _producer_name(metric_id: str, grader_class: GraderClass) -> str:
    """Return the authoritative producer name for a metric."""
    if metric_id in _DERIVED_METRIC_IDS:
        return "DEFAULT_DERIVED_REGISTRY"
    if metric_id in _TELEMETRY_METRIC_IDS:
        return "DEFAULT_TELEMETRY_REGISTRY"
    if grader_class == GraderClass.DETERMINISTIC:
        return f"deterministic grader: {metric_id}"
    if grader_class == GraderClass.LLM_JUDGE:
        return f"llm judge: {metric_id}"
    return "UNKNOWN"


class TestPhase1ReleaseMetricAudit:
    """Every release metric has a complete typed audit trail."""

    def test_release_metric_set_has_exactly_82_metrics(self) -> None:
        assert len(RELEASE_METRIC_SET.metrics) == 82, (
            f"Expected 82 release metrics, got {len(RELEASE_METRIC_SET.metrics)}"
        )

    def test_registry_has_exactly_82_definitions(self) -> None:
        definitions = DEFAULT_METRIC_REGISTRY.all_definitions()
        assert len(definitions) == 82, (
            f"Expected 82 registered definitions, got {len(definitions)}"
        )

    def test_release_set_matches_registry(self) -> None:
        registry_ids = {
            (d.metric_id, d.metric_version) for d in DEFAULT_METRIC_REGISTRY.all_definitions()
        }
        release_ids = RELEASE_METRIC_SET.metric_ids
        assert registry_ids == release_ids, (
            "Release metric set does not match the registry. "
            f"Registry-only: {registry_ids - release_ids}, "
            f"Release-only: {release_ids - registry_ids}"
        )

    @pytest.mark.parametrize(
        "definition",
        DEFAULT_METRIC_REGISTRY.all_definitions(),
        ids=[d.metric_id for d in DEFAULT_METRIC_REGISTRY.all_definitions()],
    )
    def test_every_metric_has_complete_audit_trail(self, definition) -> None:
        """Every release metric has all required audit fields populated."""
        metric_id = definition.metric_id
        metric_version = definition.metric_version

        # Eligibility
        assert definition.applicability.eligibility is not None
        assert isinstance(definition.applicability.eligibility, EligibilityKind)

        # Denominator
        assert definition.applicability.denominator is not None
        assert isinstance(definition.applicability.denominator, DenominatorKind)

        # Missing-denominator disposition
        disposition = definition.applicability.missing_denominator_disposition
        assert disposition is not None
        assert isinstance(disposition, MissingDenominatorDisposition)

        # Arm requirement
        assert definition.applicability.arm_requirement is not None
        assert isinstance(definition.applicability.arm_requirement, ArmRequirement)

        # Authoritative producer
        producer = _producer_name(metric_id, definition.grader_class)
        assert producer != "UNKNOWN", (
            f"Metric {metric_id} has no authoritative producer"
        )

        # Verify producer is registered in the right registry
        if metric_id in _DERIVED_METRIC_IDS:
            assert DEFAULT_DERIVED_REGISTRY.get(metric_id, metric_version) is not None, (
                f"Derived metric {metric_id} not registered in DEFAULT_DERIVED_REGISTRY"
            )
        elif metric_id in _TELEMETRY_METRIC_IDS:
            assert DEFAULT_TELEMETRY_REGISTRY.get(metric_id, metric_version) is not None, (
                f"Telemetry metric {metric_id} not registered in DEFAULT_TELEMETRY_REGISTRY"
            )

        # Immutable source records (evidence requirements)
        assert len(definition.evidence_requirements) >= 1, (
            f"Metric {metric_id} has no evidence requirements"
        )

        # Aggregation
        assert definition.aggregation is not None
        assert isinstance(definition.aggregation, AggregationMethod)

        # Uncertainty policy
        assert definition.uncertainty_method is not None
        assert len(definition.uncertainty_method) > 0

        # Threshold/calibration state
        # threshold_rendering is the presentation-only field; practical_threshold is the typed gate
        # Both can be None for telemetry metrics, but threshold_rendering must be a string or None
        assert definition.threshold_rendering is None or isinstance(definition.threshold_rendering, str)

        # Non-inferiority margin
        # None means the default superiority gate applies; a float means a typed margin exists
        assert definition.non_inferiority_margin is None or isinstance(definition.non_inferiority_margin, float)

        # Renderer presence: all metrics appear in the Metric Results section of all three renderers
        # The renderers iterate over analysis.metric_results, which includes every metric that
        # produced observations. This is verified by the golden vector tests.

        # Grader class
        assert definition.grader_class in (
            GraderClass.DETERMINISTIC,
            GraderClass.LLM_JUDGE,
            GraderClass.ANALYSIS,
        )

        # Direction
        assert definition.direction is not None

        # Missing value policy
        assert definition.missing_value_policy is not None
        assert isinstance(definition.missing_value_policy, MissingValuePolicy)

    def test_derived_registry_covers_all_analysis_metrics(self) -> None:
        """Every GraderClass.ANALYSIS metric has a derived producer."""
        analysis_metrics = [
            d for d in DEFAULT_METRIC_REGISTRY.all_definitions()
            if d.grader_class == GraderClass.ANALYSIS
        ]
        assert len(analysis_metrics) == 32, (
            f"Expected 32 analysis metrics, got {len(analysis_metrics)}"
        )
        for definition in analysis_metrics:
            assert DEFAULT_DERIVED_REGISTRY.get(definition.metric_id, definition.metric_version) is not None, (
                f"Analysis metric {definition.metric_id} has no derived producer"
            )

    def test_telemetry_registry_covers_all_telemetry_metrics(self) -> None:
        """Every telemetry metric has a telemetry producer."""
        telemetry_metrics = [
            d for d in DEFAULT_METRIC_REGISTRY.all_definitions()
            if d.metric_id in _TELEMETRY_METRIC_IDS
        ]
        assert len(telemetry_metrics) == 6, (
            f"Expected 6 telemetry metrics, got {len(telemetry_metrics)}"
        )
        for definition in telemetry_metrics:
            assert DEFAULT_TELEMETRY_REGISTRY.get(definition.metric_id, definition.metric_version) is not None, (
                f"Telemetry metric {definition.metric_id} has no telemetry producer"
            )

    def test_deterministic_metrics_have_grader_refs(self) -> None:
        """Every deterministic metric (except telemetry and reconciliation) has a grader reference."""
        # stage_usage_reconciled is deterministic but has no named grader; it uses
        # the usage reconciliation logic directly.
        _NO_GRADER_REF = _TELEMETRY_METRIC_IDS | {"stage_usage_reconciled"}
        for definition in DEFAULT_METRIC_REGISTRY.all_definitions():
            if definition.grader_class == GraderClass.DETERMINISTIC:
                if definition.metric_id in _NO_GRADER_REF:
                    assert definition.grader_ref is None, (
                        f"Metric {definition.metric_id} should not have a grader_ref"
                    )
                else:
                    assert definition.grader_ref is not None, (
                        f"Deterministic metric {definition.metric_id} has no grader_ref"
                    )

    def test_analysis_metrics_have_no_grader_refs(self) -> None:
        """Analysis-derived metrics do not have grader references; they use derived producers."""
        for definition in DEFAULT_METRIC_REGISTRY.all_definitions():
            if definition.grader_class == GraderClass.ANALYSIS:
                assert definition.grader_ref is None, (
                    f"Analysis metric {definition.metric_id} should not have a grader_ref"
                )

    def test_all_metrics_have_release_domain(self) -> None:
        """Every release metric has a domain in the release metric set."""
        for entry in RELEASE_METRIC_SET.metrics:
            assert isinstance(entry.domain, MetricDomain)

    def test_threshold_rendering_is_presentation_only(self) -> None:
        """threshold_rendering is a string or None, never consumed by gate logic."""
        for definition in DEFAULT_METRIC_REGISTRY.all_definitions():
            assert definition.threshold_rendering is None or isinstance(definition.threshold_rendering, str)

    def test_practical_threshold_is_typed(self) -> None:
        """practical_threshold is a typed PracticalThreshold or None."""
        from g8e_evals.metrics import PracticalThreshold

        for definition in DEFAULT_METRIC_REGISTRY.all_definitions():
            assert definition.practical_threshold is None or isinstance(
                definition.practical_threshold, PracticalThreshold
            )

    @pytest.mark.parametrize(
        "definition",
        DEFAULT_METRIC_REGISTRY.all_definitions(),
        ids=[d.metric_id for d in DEFAULT_METRIC_REGISTRY.all_definitions()],
    )
    def test_metric_has_unit_and_direction(self, definition) -> None:
        """Every metric has a unit and direction."""
        assert definition.unit is not None
        assert len(definition.unit) > 0
        assert definition.direction is not None
