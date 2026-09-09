# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed non-inferiority margin projection from the metric registry.

The metric-owned typed contract (``MetricDefinition.non_inferiority_margin``)
is the single source of truth for non-inferiority margins. This module
projects those margins into ``NonInferiorityMargin`` records for
consumers that need the structured (metric_id, metric_version, margin,
description) form.

A margin of 0.0 means any degradation is unacceptable (equivalent to a
strict release-blocker threshold in the paired setting). Non-zero
margins allow small, practically insignificant degradation for utility
metrics where exact parity is not required.

Metrics without a declared margin (e.g. eval_judge, derived analysis
metrics, telemetry metrics) return ``None`` from
``get_non_inferiority_margin`` and use the default superiority gate
(statistical significance plus non-zero delta).
"""

from __future__ import annotations

from g8e_evals.analysis.canonical import NonInferiorityMargin
from g8e_evals.metrics import DEFAULT_METRIC_REGISTRY

_GRADER_VERSION = "1.0.0"


def _build_registry() -> dict[tuple[str, str], NonInferiorityMargin]:
    """Build the typed non-inferiority margin registry from the metric registry."""
    margins: list[NonInferiorityMargin] = []
    for definition in DEFAULT_METRIC_REGISTRY.all_definitions():
        if definition.non_inferiority_margin is None:
            continue
        margins.append(NonInferiorityMargin(
            metric_id=definition.metric_id,
            metric_version=definition.metric_version,
            margin=definition.non_inferiority_margin,
            description=f"Non-inferiority margin for {definition.metric_id}: {definition.non_inferiority_margin}.",
        ))
    return {(m.metric_id, m.metric_version): m for m in margins}


_NON_INFERIORITY_MARGINS: dict[tuple[str, str], NonInferiorityMargin] = _build_registry()


def get_non_inferiority_margin(metric_id: str, metric_version: str) -> NonInferiorityMargin | None:
    """Return the typed non-inferiority margin for a metric, or None."""
    return _NON_INFERIORITY_MARGINS.get((metric_id, metric_version))


def all_non_inferiority_margins() -> list[NonInferiorityMargin]:
    """Return all registered non-inferiority margins, sorted by metric_id."""
    return sorted(_NON_INFERIORITY_MARGINS.values(), key=lambda m: (m.metric_id, m.metric_version))


__all__ = [
    "all_non_inferiority_margins",
    "get_non_inferiority_margin",
]
