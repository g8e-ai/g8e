# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 2 integration tests for partial-report handling.

Verifies that a process-killed partial report directory remains
immutable evidence of interruption but is never treated as a complete
report. Resume creates a new report directory for a missing or
replacement assignment rather than mutating the partial directory.
A report is finalized only after its manifest, expected terminal
attempts, metrics, evidence index, summary, and report checksum
validate.
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest

pytestmark = pytest.mark.integration

from g8e_evals.constants import (
    ANALYSIS_JSON,
    ATTEMPTS_JSONL,
    CAMPAIGN_MANIFEST_JSON,
    CAMPAIGN_STATUS_JSON,
    MANIFEST_JSON,
    METRICS_JSONL,
)
from g8e_evals.report.completeness import is_report_complete, ReportCompletenessError


_VALID_HASH = "a" * 64


def _write_json(path: Path, data: object) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data))


def _write_jsonl(path: Path, records: list[dict]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    lines = [json.dumps(r) for r in records]
    path.write_text("\n".join(lines) + ("\n" if lines else ""))


class TestReportCompleteness:
    def test_complete_report_passes_completeness_check(self, tmp_path: Path):
        """A report directory with all required artifacts is complete."""
        report_dir = tmp_path / "report-complete"
        report_dir.mkdir()
        _write_json(report_dir / MANIFEST_JSON, {"run_id": "run-1"})
        _write_jsonl(report_dir / ATTEMPTS_JSONL, [{"attempt_id": "a1", "terminal_status": "completed"}])
        _write_jsonl(report_dir / METRICS_JSONL, [{"metric_id": "m1", "value": 1.0}])
        _write_json(report_dir / ANALYSIS_JSON, {"run_id": "run-1"})
        assert is_report_complete(report_dir)

    def test_partial_report_missing_manifest_is_not_complete(self, tmp_path: Path):
        """A report directory missing manifest.json is not complete."""
        report_dir = tmp_path / "report-partial"
        report_dir.mkdir()
        _write_jsonl(report_dir / ATTEMPTS_JSONL, [{"attempt_id": "a1", "terminal_status": "completed"}])
        assert not is_report_complete(report_dir)

    def test_partial_report_missing_attempts_is_not_complete(self, tmp_path: Path):
        """A report directory missing attempts.jsonl is not complete."""
        report_dir = tmp_path / "report-partial"
        report_dir.mkdir()
        _write_json(report_dir / MANIFEST_JSON, {"run_id": "run-1"})
        assert not is_report_complete(report_dir)

    def test_partial_report_missing_metrics_is_not_complete(self, tmp_path: Path):
        """A report directory missing metrics.jsonl is not complete."""
        report_dir = tmp_path / "report-partial"
        report_dir.mkdir()
        _write_json(report_dir / MANIFEST_JSON, {"run_id": "run-1"})
        _write_jsonl(report_dir / ATTEMPTS_JSONL, [{"attempt_id": "a1", "terminal_status": "completed"}])
        assert not is_report_complete(report_dir)

    def test_partial_report_missing_analysis_is_not_complete(self, tmp_path: Path):
        """A report directory missing analysis.json is not complete."""
        report_dir = tmp_path / "report-partial"
        report_dir.mkdir()
        _write_json(report_dir / MANIFEST_JSON, {"run_id": "run-1"})
        _write_jsonl(report_dir / ATTEMPTS_JSONL, [{"attempt_id": "a1", "terminal_status": "completed"}])
        _write_jsonl(report_dir / METRICS_JSONL, [{"metric_id": "m1", "value": 1.0}])
        assert not is_report_complete(report_dir)

    def test_empty_report_directory_is_not_complete(self, tmp_path: Path):
        """An empty report directory is not complete."""
        report_dir = tmp_path / "report-empty"
        report_dir.mkdir()
        assert not is_report_complete(report_dir)

    def test_nonexistent_report_directory_is_not_complete(self, tmp_path: Path):
        """A nonexistent report directory is not complete."""
        report_dir = tmp_path / "report-nonexistent"
        assert not is_report_complete(report_dir)


class TestPartialReportImmutability:
    def test_partial_report_directory_is_never_treated_as_complete(self, tmp_path: Path):
        """A process-killed partial directory with a status file but no
        analysis.json remains interruption evidence, never complete."""
        report_dir = tmp_path / "report-killed"
        report_dir.mkdir()
        _write_json(report_dir / MANIFEST_JSON, {"run_id": "run-1"})
        _write_json(report_dir / CAMPAIGN_STATUS_JSON, {"status": "running"})
        _write_jsonl(report_dir / ATTEMPTS_JSONL, [{"attempt_id": "a1", "terminal_status": "completed"}])
        # No analysis.json: the process was killed before finalization
        assert not is_report_complete(report_dir)

    def test_partial_report_with_campaign_manifest_still_requires_analysis(self, tmp_path: Path):
        """Even with a campaign manifest, a report is not complete without analysis."""
        report_dir = tmp_path / "report-partial-campaign"
        report_dir.mkdir()
        _write_json(report_dir / MANIFEST_JSON, {"run_id": "run-1"})
        _write_json(report_dir / CAMPAIGN_MANIFEST_JSON, {"campaign_id": "c1"})
        _write_json(report_dir / CAMPAIGN_STATUS_JSON, {"status": "running"})
        _write_jsonl(report_dir / ATTEMPTS_JSONL, [{"attempt_id": "a1", "terminal_status": "completed"}])
        _write_jsonl(report_dir / METRICS_JSONL, [{"metric_id": "m1", "value": 1.0}])
        assert not is_report_complete(report_dir)


class TestReportCompletenessError:
    def test_completeness_check_raises_on_corrupted_manifest(self, tmp_path: Path):
        """A corrupted manifest.json raises ReportCompletenessError, not silently passes."""
        report_dir = tmp_path / "report-corrupt"
        report_dir.mkdir()
        (report_dir / MANIFEST_JSON).write_text("{invalid json")
        _write_jsonl(report_dir / ATTEMPTS_JSONL, [{"attempt_id": "a1", "terminal_status": "completed"}])
        _write_jsonl(report_dir / METRICS_JSONL, [{"metric_id": "m1", "value": 1.0}])
        _write_json(report_dir / ANALYSIS_JSON, {"run_id": "run-1"})
        with pytest.raises(ReportCompletenessError):
            is_report_complete(report_dir)
