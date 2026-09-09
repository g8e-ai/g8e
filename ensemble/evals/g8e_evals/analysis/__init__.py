# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Canonical eval analysis package: versioned model, computation engine, paired statistical estimators, and deterministic renderers."""

from __future__ import annotations

from g8e_evals.analysis import statistics
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
    MissingnessReason,
    NonInferiorityMargin,
    PairedComparison,
    PooledConfusionMatrix,
    ReceiptCoverageAnalysis,
)
from g8e_evals.analysis.engine import compute_bridge_run_comparison, compute_canonical_analysis, compute_canonical_analysis_from_record
from g8e_evals.analysis.input import AnalysisInputRecord
from g8e_evals.analysis.renderers import render_cli, render_html, render_markdown
from g8e_evals.analysis.telemetry import (
    DEFAULT_TELEMETRY_REGISTRY,
    DuplicateTelemetryProducerError,
    TelemetryObservationCollisionError,
    TelemetryProducerError,
    TelemetryProducerRegistry,
    TelemetryRegistryIncompleteError,
    run_all_telemetry_producers,
)
from g8e_evals.analysis import telemetry

__all__ = [
    "ANALYSIS_COMPUTATION_VERSION",
    "ANALYSIS_SCHEMA_VERSION",
    "DEFAULT_TELEMETRY_REGISTRY",
    "AnalysisInputRecord",
    "AnalysisInputSummary",
    "BridgeRunComparison",
    "BridgeRunManifest",
    "CanonicalEvalAnalysis",
    "ComparisonDirection",
    "ConfusionMatrix",
    "DomainStratifiedResult",
    "DuplicateTelemetryProducerError",
    "GateDecision",
    "GateDecisionStatus",
    "MetricAnalysisResult",
    "MissingnessBreakdown",
    "MissingnessReason",
    "NonInferiorityMargin",
    "PairedComparison",
    "PooledConfusionMatrix",
    "ReceiptCoverageAnalysis",
    "TelemetryObservationCollisionError",
    "TelemetryProducerError",
    "TelemetryProducerRegistry",
    "TelemetryRegistryIncompleteError",
    "compute_bridge_run_comparison",
    "compute_canonical_analysis",
    "compute_canonical_analysis_from_record",
    "render_cli",
    "render_html",
    "render_markdown",
    "run_all_telemetry_producers",
    "statistics",
    "telemetry",
]
