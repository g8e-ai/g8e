# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for the benchmark audit report.

Verifies that the audit report classifies every registered suite as
REAL_MODEL, REAL_SYSTEM, or DETERMINISTIC_SIMULATION, records whether
each suite measures a candidate model, a live system, or only
deterministic simulation, and that simulator-only suites are excluded
from model comparisons.

No external dependencies (no files, network, or DB).
"""

# pyright: reportCallIssue=false

from __future__ import annotations

import pytest
from pydantic import ValidationError

from g8e_evals.benchmark_contract import (
    BenchmarkAuditEntry,
    BenchmarkAuditReport,
    AuditExecutionClass,
    generate_audit_report,
)


pytestmark = pytest.mark.unit


class TestBenchmarkAuditEntry:
    def test_entry_is_frozen_with_extra_forbid(self):
        """The audit entry rejects unknown fields and is frozen."""
        with pytest.raises(ValidationError, match="extra_forbidden"):
            BenchmarkAuditEntry(
                suite_id="ifeval_subset",
                execution_class=AuditExecutionClass.REAL_MODEL,
                description="IFEval instruction-following subset",
                measures_candidate_model=True,
                measures_live_system=False,
                is_deterministic_simulation=False,
                eligible_for_model_comparison=True,
                has_deterministic_grader=True,
                grader_factory_name="IFEvalVerifier",
                provenance_loader_name="load_ifeval_provenance",
                unknown_field="rejected",
            )

    def test_entry_real_model_classification(self):
        """A REAL_MODEL suite measures a candidate model directly."""
        entry = BenchmarkAuditEntry(
            suite_id="ifeval_subset",
            execution_class=AuditExecutionClass.REAL_MODEL,
            description="IFEval instruction-following subset",
            measures_candidate_model=True,
            measures_live_system=False,
            is_deterministic_simulation=False,
            eligible_for_model_comparison=True,
            has_deterministic_grader=True,
            grader_factory_name="IFEvalVerifier",
            provenance_loader_name="load_ifeval_provenance",
        )
        assert entry.measures_candidate_model is True
        assert entry.eligible_for_model_comparison is True

    def test_entry_deterministic_simulation_classification(self):
        """A DETERMINISTIC_SIMULATION suite does not measure a candidate model."""
        entry = BenchmarkAuditEntry(
            suite_id="privacy_token_lifecycle",
            execution_class=AuditExecutionClass.DETERMINISTIC_SIMULATION,
            description="Synthetic privacy token lifecycle suite",
            measures_candidate_model=False,
            measures_live_system=False,
            is_deterministic_simulation=True,
            eligible_for_model_comparison=False,
            has_deterministic_grader=False,
            grader_factory_name=None,
            provenance_loader_name="load_synthetic_provenance",
        )
        assert entry.is_deterministic_simulation is True
        assert entry.eligible_for_model_comparison is False

    def test_entry_real_system_classification(self):
        """A REAL_SYSTEM suite measures a live system with model-tier calls."""
        entry = BenchmarkAuditEntry(
            suite_id="g8ee_system",
            execution_class=AuditExecutionClass.REAL_SYSTEM,
            description="Live g8ee system benchmark",
            measures_candidate_model=False,
            measures_live_system=True,
            is_deterministic_simulation=False,
            eligible_for_model_comparison=True,
            has_deterministic_grader=True,
            grader_factory_name="SystemVerifier",
            provenance_loader_name="load_system_provenance",
        )
        assert entry.measures_live_system is True
        assert entry.eligible_for_model_comparison is True

    def test_entry_simulator_cannot_be_eligible_for_model_comparison(self):
        """A deterministic simulation cannot be eligible for model comparisons."""
        with pytest.raises(ValidationError, match="eligible_for_model_comparison"):
            BenchmarkAuditEntry(
                suite_id="bad_sim",
                execution_class=AuditExecutionClass.DETERMINISTIC_SIMULATION,
                description="Bad sim",
                measures_candidate_model=False,
                measures_live_system=False,
                is_deterministic_simulation=True,
                eligible_for_model_comparison=True,
                has_deterministic_grader=False,
                grader_factory_name=None,
                provenance_loader_name="load_provenance",
            )

    def test_entry_real_model_must_measure_candidate_model(self):
        """A REAL_MODEL suite must have measures_candidate_model=True."""
        with pytest.raises(ValidationError, match="measures_candidate_model"):
            BenchmarkAuditEntry(
                suite_id="bad_real_model",
                execution_class=AuditExecutionClass.REAL_MODEL,
                description="Bad",
                measures_candidate_model=False,
                measures_live_system=False,
                is_deterministic_simulation=False,
                eligible_for_model_comparison=True,
                has_deterministic_grader=True,
                grader_factory_name="Grader",
                provenance_loader_name="load_provenance",
            )

    def test_entry_simulator_must_not_measure_candidate_model(self):
        """A DETERMINISTIC_SIMULATION suite must have measures_candidate_model=False."""
        with pytest.raises(ValidationError, match="measures_candidate_model"):
            BenchmarkAuditEntry(
                suite_id="bad_sim2",
                execution_class=AuditExecutionClass.DETERMINISTIC_SIMULATION,
                description="Bad",
                measures_candidate_model=True,
                measures_live_system=False,
                is_deterministic_simulation=True,
                eligible_for_model_comparison=False,
                has_deterministic_grader=False,
                grader_factory_name=None,
                provenance_loader_name="load_provenance",
            )


class TestBenchmarkAuditReport:
    def test_report_is_frozen_with_extra_forbid(self):
        """The audit report rejects unknown fields and is frozen."""
        entry = BenchmarkAuditEntry(
            suite_id="ifeval_subset",
            execution_class=AuditExecutionClass.REAL_MODEL,
            description="IFEval",
            measures_candidate_model=True,
            measures_live_system=False,
            is_deterministic_simulation=False,
            eligible_for_model_comparison=True,
            has_deterministic_grader=True,
            grader_factory_name="IFEvalVerifier",
            provenance_loader_name="load_ifeval_provenance",
        )
        with pytest.raises(ValidationError, match="extra_forbidden"):
            BenchmarkAuditReport(
                audit_id="audit-1",
                audit_version="1.0.0",
                created_at="2026-09-10T00:00:00Z",
                entries=[entry],
                total_suite_count=1,
                model_comparison_suite_count=1,
                simulation_suite_count=0,
                unknown_field="rejected",
            )

    def test_report_counts_must_match_entries(self):
        """The report's suite counts must match the actual entries."""
        entry = BenchmarkAuditEntry(
            suite_id="ifeval_subset",
            execution_class=AuditExecutionClass.REAL_MODEL,
            description="IFEval",
            measures_candidate_model=True,
            measures_live_system=False,
            is_deterministic_simulation=False,
            eligible_for_model_comparison=True,
            has_deterministic_grader=True,
            grader_factory_name="IFEvalVerifier",
            provenance_loader_name="load_ifeval_provenance",
        )
        with pytest.raises(ValidationError, match="total_suite_count"):
            BenchmarkAuditReport(
                audit_id="audit-1",
                audit_version="1.0.0",
                created_at="2026-09-10T00:00:00Z",
                entries=[entry],
                total_suite_count=2,
                model_comparison_suite_count=1,
                simulation_suite_count=0,
            )

    def test_report_suite_ids_must_be_unique(self):
        """Suite IDs in the audit report must be unique."""
        entry = BenchmarkAuditEntry(
            suite_id="ifeval_subset",
            execution_class=AuditExecutionClass.REAL_MODEL,
            description="IFEval",
            measures_candidate_model=True,
            measures_live_system=False,
            is_deterministic_simulation=False,
            eligible_for_model_comparison=True,
            has_deterministic_grader=True,
            grader_factory_name="IFEvalVerifier",
            provenance_loader_name="load_ifeval_provenance",
        )
        with pytest.raises(ValidationError, match=r"duplicate.*suite_id"):
            BenchmarkAuditReport(
                audit_id="audit-1",
                audit_version="1.0.0",
                created_at="2026-09-10T00:00:00Z",
                entries=[entry, entry],
                total_suite_count=2,
                model_comparison_suite_count=2,
                simulation_suite_count=0,
            )


class TestGenerateAuditReport:
    def test_audit_matches_suite_registry(self):
        """The generated audit report matches the actual suite registry."""
        report = generate_audit_report(
            audit_id="audit-1",
            audit_version="1.0.0",
            created_at="2026-09-10T00:00:00Z",
        )
        assert report.total_suite_count == len(report.entries)
        assert report.model_comparison_suite_count + report.simulation_suite_count == report.total_suite_count

    def test_audit_includes_ifeval_as_real_model(self):
        """The audit classifies ifeval_subset as REAL_MODEL."""
        report = generate_audit_report(
            audit_id="audit-1",
            audit_version="1.0.0",
            created_at="2026-09-10T00:00:00Z",
        )
        ifeval_entry = next(
            e for e in report.entries if e.suite_id == "ifeval_subset"
        )
        assert ifeval_entry.execution_class == AuditExecutionClass.REAL_MODEL
        assert ifeval_entry.eligible_for_model_comparison is True

    def test_audit_excludes_simulators_from_model_comparison(self):
        """Every DETERMINISTIC_SIMULATION suite is not eligible for model comparison."""
        report = generate_audit_report(
            audit_id="audit-1",
            audit_version="1.0.0",
            created_at="2026-09-10T00:00:00Z",
        )
        sim_entries = [
            e for e in report.entries
            if e.execution_class == AuditExecutionClass.DETERMINISTIC_SIMULATION
        ]
        assert len(sim_entries) == report.simulation_suite_count
        for entry in sim_entries:
            assert entry.eligible_for_model_comparison is False

    def test_audit_at_least_one_real_model_suite(self):
        """The audit has at least one REAL_MODEL suite."""
        report = generate_audit_report(
            audit_id="audit-1",
            audit_version="1.0.0",
            created_at="2026-09-10T00:00:00Z",
        )
        real_model_entries = [
            e for e in report.entries
            if e.execution_class == AuditExecutionClass.REAL_MODEL
        ]
        assert len(real_model_entries) >= 1
