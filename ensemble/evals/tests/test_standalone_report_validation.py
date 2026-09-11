# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 2 integration tests for standalone report validation.

Verifies that ``validate_standalone_report`` checks manifest, expected
terminal attempts, metrics, evidence index, summary, and report
checksum. A report is valid only when all required artifacts exist, every
attempt is terminal, required deterministic metrics are directly bound,
and the report checksum matches.
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest

pytestmark = pytest.mark.integration

from g8e_evals.constants import (
    ANALYSIS_JSON,
    ATTEMPTS_JSONL,
    EVIDENCE_INDEX_JSONL,
    MANIFEST_JSON,
    METRICS_JSONL,
    TASKS_JSONL,
)
from g8e_evals.report.validate import (
    StandaloneReportResult,
    validate_standalone_report,
)


_VALID_SHA = "a" * 64


def _write_json(path: Path, data: object) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data))


def _write_jsonl(path: Path, records: list[dict]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    lines = [json.dumps(r) for r in records]
    path.write_text("\n".join(lines) + ("\n" if lines else ""))


def _make_valid_report(report_dir: Path) -> None:
    """Create a complete valid report directory."""
    report_dir.mkdir(parents=True, exist_ok=True)
    _write_json(report_dir / MANIFEST_JSON, {
        "schema_version": "1.42.0",
        "run_id": "run-1",
        "suite_id": "ifeval_subset",
        "suite_version": "1.0.0",
        "orchestrator_version": "v2.1.8",
    })
    _write_jsonl(report_dir / TASKS_JSONL, [
        {"task_id": "task-1001", "suite_id": "ifeval_subset", "suite_version": "1.0.0",
         "category": "instruction_following", "prompt_hash": _VALID_SHA, "prompt_length": 20,
         "graders": [{"grader_id": "ifeval_subset_verifier", "grader_version": "1.0.0",
                       "grader_class": "deterministic"}],
         "metadata": {"instruction_id_list": ["punctuation:no_comma"]}},
    ])
    _write_jsonl(report_dir / ATTEMPTS_JSONL, [
        {"schema_version": "1.42.0", "attempt_id": "att-1", "run_id": "run-1",
         "task_id": "task-1001", "arm_id": "direct", "terminal_status": "completed",
         "assignment_id": "assign-1"},
    ])
    _write_jsonl(report_dir / METRICS_JSONL, [
        {"schema_version": "1.42.0", "metric_id": "ifeval_subset_verifier",
         "metric_version": "1.0.0", "attempt_id": "att-1", "run_id": "run-1",
         "arm_id": "direct", "task_id": "task-1001", "value": 1.0, "unit": "boolean",
         "eligible": True, "denominator_contribution": 1,
         "verification_status": "verified", "grader_class": "deterministic",
         "evidence_refs": []},
    ])
    _write_json(report_dir / ANALYSIS_JSON, {"run_id": "run-1"})
    _write_jsonl(report_dir / EVIDENCE_INDEX_JSONL, [])


class TestStandaloneReportValidationPass:
    def test_complete_report_passes_validation(self, tmp_path: Path):
        """A complete report with all required artifacts passes validation."""
        report_dir = tmp_path / "report-valid"
        _make_valid_report(report_dir)
        result = validate_standalone_report(report_dir)
        assert result.ok
        assert len(result.failures) == 0

    def test_validation_returns_typed_result(self, tmp_path: Path):
        """Validation returns a StandaloneReportResult with checked layers."""
        report_dir = tmp_path / "report-valid"
        _make_valid_report(report_dir)
        result = validate_standalone_report(report_dir)
        assert isinstance(result, StandaloneReportResult)
        assert len(result.checked_layers) > 0


class TestStandaloneReportValidationFail:
    def test_missing_manifest_fails(self, tmp_path: Path):
        """A report missing manifest.json fails validation."""
        report_dir = tmp_path / "report-no-manifest"
        report_dir.mkdir()
        _write_jsonl(report_dir / ATTEMPTS_JSONL, [
            {"attempt_id": "att-1", "terminal_status": "completed"}])
        _write_jsonl(report_dir / METRICS_JSONL, [])
        _write_json(report_dir / ANALYSIS_JSON, {})
        result = validate_standalone_report(report_dir)
        assert not result.ok
        assert any("manifest" in f for f in result.failures)

    def test_missing_attempts_fails(self, tmp_path: Path):
        """A report missing attempts.jsonl fails validation."""
        report_dir = tmp_path / "report-no-attempts"
        _make_valid_report(report_dir)
        (report_dir / ATTEMPTS_JSONL).unlink()
        result = validate_standalone_report(report_dir)
        assert not result.ok
        assert any("attempts" in f for f in result.failures)

    def test_missing_metrics_fails(self, tmp_path: Path):
        """A report missing metrics.jsonl fails validation."""
        report_dir = tmp_path / "report-no-metrics"
        _make_valid_report(report_dir)
        (report_dir / METRICS_JSONL).unlink()
        result = validate_standalone_report(report_dir)
        assert not result.ok
        assert any("metrics" in f for f in result.failures)

    def test_missing_analysis_fails(self, tmp_path: Path):
        """A report missing analysis.json fails validation."""
        report_dir = tmp_path / "report-no-analysis"
        _make_valid_report(report_dir)
        (report_dir / ANALYSIS_JSON).unlink()
        result = validate_standalone_report(report_dir)
        assert not result.ok
        assert any("analysis" in f for f in result.failures)

    def test_non_terminal_attempt_fails(self, tmp_path: Path):
        """An attempt with a non-terminal status fails validation."""
        report_dir = tmp_path / "report-non-terminal"
        _make_valid_report(report_dir)
        _write_jsonl(report_dir / ATTEMPTS_JSONL, [
            {"schema_version": "1.42.0", "attempt_id": "att-1", "run_id": "run-1",
             "task_id": "task-1001", "arm_id": "direct", "terminal_status": "running",
             "assignment_id": "assign-1"},
        ])
        result = validate_standalone_report(report_dir)
        assert not result.ok
        assert any("terminal" in f.lower() for f in result.failures)

    def test_missing_metric_for_attempt_fails(self, tmp_path: Path):
        """An attempt without a bound metric fails validation."""
        report_dir = tmp_path / "report-no-bound-metric"
        _make_valid_report(report_dir)
        _write_jsonl(report_dir / METRICS_JSONL, [])
        result = validate_standalone_report(report_dir)
        assert not result.ok
        assert any("metric" in f.lower() for f in result.failures)

    def test_corrupted_manifest_fails(self, tmp_path: Path):
        """A corrupted manifest.json fails validation."""
        report_dir = tmp_path / "report-corrupt-manifest"
        _make_valid_report(report_dir)
        (report_dir / MANIFEST_JSON).write_text("{invalid json")
        result = validate_standalone_report(report_dir)
        assert not result.ok
        assert any("manifest" in f for f in result.failures)

    def test_corrupted_analysis_fails(self, tmp_path: Path):
        """A corrupted analysis.json fails validation."""
        report_dir = tmp_path / "report-corrupt-analysis"
        _make_valid_report(report_dir)
        (report_dir / ANALYSIS_JSON).write_text("{invalid json")
        result = validate_standalone_report(report_dir)
        assert not result.ok
        assert any("analysis" in f for f in result.failures)

    def test_report_checksum_mismatch_fails(self, tmp_path: Path):
        """A report checksum that does not match the computed checksum fails."""
        report_dir = tmp_path / "report-checksum-mismatch"
        _make_valid_report(report_dir)
        _write_json(report_dir / "report-checksum.json", {
            "checksum": "0" * 64,
            "algorithm": "sha256",
        })
        result = validate_standalone_report(report_dir)
        assert not result.ok
        assert any("checksum" in f.lower() for f in result.failures)

    def test_symlink_in_report_fails(self, tmp_path: Path):
        """A symlink in place of a required artifact fails validation."""
        report_dir = tmp_path / "report-symlink"
        _make_valid_report(report_dir)
        (report_dir / MANIFEST_JSON).unlink()
        (report_dir / MANIFEST_JSON).symlink_to(report_dir / ANALYSIS_JSON)
        result = validate_standalone_report(report_dir)
        assert not result.ok
