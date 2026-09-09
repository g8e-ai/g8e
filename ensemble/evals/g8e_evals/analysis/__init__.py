# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Canonical eval analysis package: versioned model, computation engine, and paired statistical estimators."""

from __future__ import annotations

from g8e_evals.analysis import statistics
from g8e_evals.analysis.canonical import (
    ANALYSIS_COMPUTATION_VERSION,
    ANALYSIS_SCHEMA_VERSION,
    AnalysisInputSummary,
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
    PairedComparison,
    ReceiptCoverageAnalysis,
)
from g8e_evals.analysis.engine import compute_canonical_analysis

__all__ = [
    "ANALYSIS_COMPUTATION_VERSION",
    "ANALYSIS_SCHEMA_VERSION",
    "AnalysisInputSummary",
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
    "PairedComparison",
    "ReceiptCoverageAnalysis",
    "compute_canonical_analysis",
    "statistics",
]
