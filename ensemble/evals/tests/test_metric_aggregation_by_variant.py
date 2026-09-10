# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for metric aggregation by model variant.

Verifies that metrics are aggregated per model variant and never pooled
across distinct variants or combinations. The aggregation preserves
variant identity, produces per-variant numerators, denominators, and
rates, and rejects attempts to merge results from different variants.
"""

from __future__ import annotations

import pytest

from g8e_evals.index import (
    aggregate_metrics_by_variant,
)


pytestmark = pytest.mark.unit


def _metric(
    *,
    variant_id: str = "qwen3-8b-q4_0",
    task_id: str = "task-1",
    value: float = 1.0,
    unit: str = "boolean",
) -> dict:
    return {
        "model_variant_id": variant_id,
        "task_id": task_id,
        "value": value,
        "unit": unit,
    }


class TestMetricAggregationByVariant:
    def test_single_variant_aggregation(self):
        """Metrics for one variant aggregate to one result."""
        metrics = [
            _metric(variant_id="v1", task_id="t1", value=1.0),
            _metric(variant_id="v1", task_id="t2", value=0.0),
            _metric(variant_id="v1", task_id="t3", value=1.0),
        ]
        results = aggregate_metrics_by_variant(metrics)
        assert len(results) == 1
        assert results[0].variant_id == "v1"
        assert results[0].numerator == 2
        assert results[0].denominator == 3
        assert results[0].rate == pytest.approx(2 / 3)

    def test_two_variants_not_pooled(self):
        """Metrics for two variants produce two separate results, not pooled."""
        metrics = [
            _metric(variant_id="v1", task_id="t1", value=1.0),
            _metric(variant_id="v1", task_id="t2", value=1.0),
            _metric(variant_id="v2", task_id="t1", value=0.0),
            _metric(variant_id="v2", task_id="t2", value=0.0),
        ]
        results = aggregate_metrics_by_variant(metrics)
        assert len(results) == 2
        v1 = next(r for r in results if r.variant_id == "v1")
        v2 = next(r for r in results if r.variant_id == "v2")
        assert v1.numerator == 2
        assert v1.denominator == 2
        assert v1.rate == pytest.approx(1.0)
        assert v2.numerator == 0
        assert v2.denominator == 2
        assert v2.rate == pytest.approx(0.0)

    def test_aggregation_preserves_variant_identity(self):
        """Each aggregate result carries its variant_id."""
        metrics = [
            _metric(variant_id="alpha", task_id="t1", value=1.0),
            _metric(variant_id="beta", task_id="t1", value=0.0),
        ]
        results = aggregate_metrics_by_variant(metrics)
        variant_ids = {r.variant_id for r in results}
        assert variant_ids == {"alpha", "beta"}

    def test_empty_metrics_produces_empty_results(self):
        """No metrics produces no aggregates."""
        results = aggregate_metrics_by_variant([])
        assert results == []

    def test_aggregation_includes_unit(self):
        """Each aggregate result carries the metric unit."""
        metrics = [
            _metric(variant_id="v1", task_id="t1", value=1.0, unit="boolean"),
        ]
        results = aggregate_metrics_by_variant(metrics)
        assert results[0].unit == "boolean"

    def test_mixed_units_for_same_variant_rejected(self):
        """Metrics with different units for the same variant are rejected."""
        metrics = [
            _metric(variant_id="v1", task_id="t1", value=1.0, unit="boolean"),
            _metric(variant_id="v1", task_id="t2", value=0.5, unit="ratio"),
        ]
        with pytest.raises(ValueError, match=r"unit.*mismatch"):
            aggregate_metrics_by_variant(metrics)

    def test_results_are_sorted_by_variant_id(self):
        """Aggregate results are sorted by variant_id for deterministic output."""
        metrics = [
            _metric(variant_id="zeta", task_id="t1", value=1.0),
            _metric(variant_id="alpha", task_id="t1", value=1.0),
            _metric(variant_id="mid", task_id="t1", value=1.0),
        ]
        results = aggregate_metrics_by_variant(metrics)
        variant_ids = [r.variant_id for r in results]
        assert variant_ids == sorted(variant_ids)
