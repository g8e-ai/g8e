# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests proving the threshold rendering field is presentation-only.

The ``MetricDefinition.threshold_rendering`` field (formerly
``release_threshold``) is a human-readable string that renders the typed
practical threshold or calibration status. It is NOT consumed by any
gate-decision, engine, renderer, or statistical code. The gate decision
status (PASS, FAIL, INSUFFICIENT_DATA, etc.) is computed exclusively from
the typed ``practical_threshold`` and ``non_inferiority_margin`` fields.

These tests prove that changing the presentation text cannot alter a gate
decision, satisfying the P1-08 step 1 requirement.
"""

from __future__ import annotations

import pytest

from g8e_evals.analysis import engine
from g8e_evals.analysis.canonical import MetricAnalysisResult, MetricDomain
from g8e_evals.metrics import (
    DEFAULT_METRIC_REGISTRY,
    MetricDirection,
    MetricRegistry,
)
from g8e_evals.release_metric_set import MetricDomain as ReleaseMetricDomain
from g8e_evals.release_metric_set import ReleaseMetricEntry

pytestmark = pytest.mark.unit


def _metric_result(metric_id: str, value: float, arm_id: str = "doctrine") -> MetricAnalysisResult:
    return MetricAnalysisResult(
        metric_id=metric_id,
        metric_version="1.0.0",
        arm_id=arm_id,
        domain=MetricDomain.GOVERNANCE,
        direction=MetricDirection.BINARY_PASS_FAIL,
        unit="boolean",
        numerator=value,
        denominator=1,
        value=value,
        eligible_count=1,
        not_eligible_count=0,
        missing_count=0,
        verification_status_counts={"verified": 1},
        evidence_ref_count=1,
        metric_observation_ids=[f"{metric_id}@1.0.0:att-1"],
    )


def _registry_with_renamed_threshold(new_text: str) -> MetricRegistry:
    original = DEFAULT_METRIC_REGISTRY.get("receipt_integrity", "1.0.0")
    modified = original.model_copy(update={"threshold_rendering": new_text})
    return MetricRegistry([modified])


def _release_entry_with_text(text: str) -> dict[tuple[str, str], ReleaseMetricEntry]:
    return {
        ("receipt_integrity", "1.0.0"): ReleaseMetricEntry(
            metric_id="receipt_integrity",
            metric_version="1.0.0",
            domain=ReleaseMetricDomain.GOVERNANCE,
            has_practical_threshold=True,
            threshold_description=text,
        ),
    }


class TestThresholdRenderingIsPresentationOnly:
    """The threshold_rendering field cannot alter a gate decision."""

    def test_pass_status_unchanged_when_presentation_text_differs(self, monkeypatch) -> None:
        """A PASS gate decision is identical regardless of the threshold_rendering text."""
        mr = _metric_result("receipt_integrity", 1.0)
        original_decisions = engine._compute_gate_decisions([mr])
        original_status = original_decisions[0].status
        assert original_status == engine.GateDecisionStatus.PASS

        monkeypatch.setattr(
            engine, "DEFAULT_METRIC_REGISTRY",
            _registry_with_renamed_threshold("Completely different presentation text."),
        )
        monkeypatch.setattr(
            engine, "_build_release_metric_lookup",
            lambda: _release_entry_with_text("Completely different presentation text."),
        )

        modified_decisions = engine._compute_gate_decisions([mr])
        assert modified_decisions[0].status == original_status
        assert modified_decisions[0].threshold_description == "Completely different presentation text."

    def test_fail_status_unchanged_when_presentation_text_differs(self, monkeypatch) -> None:
        """A FAIL gate decision is identical regardless of the threshold_rendering text."""
        mr = _metric_result("receipt_integrity", 0.0)
        original_decisions = engine._compute_gate_decisions([mr])
        original_status = original_decisions[0].status
        assert original_status == engine.GateDecisionStatus.FAIL

        monkeypatch.setattr(
            engine, "DEFAULT_METRIC_REGISTRY",
            _registry_with_renamed_threshold("Different text for failing case."),
        )
        monkeypatch.setattr(
            engine, "_build_release_metric_lookup",
            lambda: _release_entry_with_text("Different text for failing case."),
        )

        modified_decisions = engine._compute_gate_decisions([mr])
        assert modified_decisions[0].status == original_status

    def test_threshold_rendering_field_exists_and_is_str_or_none(self) -> None:
        """The threshold_rendering field is a str | None presentation field on MetricDefinition."""
        definition = DEFAULT_METRIC_REGISTRY.get("receipt_integrity", "1.0.0")
        assert hasattr(definition, "threshold_rendering")
        assert isinstance(definition.threshold_rendering, str | type(None))
        assert not hasattr(definition, "release_threshold")

    def test_telemetry_metrics_have_none_threshold_rendering(self) -> None:
        """Telemetry metrics have threshold_rendering set to None (no practical threshold)."""
        for metric_id in (
            "stage_latency_seconds", "provider_usage_tokens", "provider_cost_usd",
            "local_resource_peak_memory_bytes", "local_resource_cpu_seconds",
            "human_wait_seconds",
        ):
            definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
            assert definition.threshold_rendering is None, (
                f"telemetry metric {metric_id} has a threshold_rendering, expected None"
            )
