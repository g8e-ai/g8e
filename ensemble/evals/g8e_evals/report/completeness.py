# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Report completeness checking for partial-report handling.

A report is complete only when all required artifacts exist and are
valid: manifest.json, attempts.jsonl, metrics.jsonl, and analysis.json.
A process-killed partial directory remains immutable evidence of
interruption but is never treated as complete.

This module performs read-only checks on the report directory. It does
not mutate, create, or delete any files.
"""

from __future__ import annotations

from pathlib import Path

from pydantic import ValidationError

from g8e_evals.analysis.canonical import CanonicalEvalAnalysis
from g8e_evals.constants import (
    ANALYSIS_JSON,
    ATTEMPTS_JSONL,
    MANIFEST_JSON,
    METRICS_JSONL,
)
from g8e_evals.schema import RunManifest


class ReportCompletenessError(ValueError):
    """Raised when a report directory contains corrupted or invalid artifacts."""


_REQUIRED_ARTIFACTS = (
    MANIFEST_JSON,
    ATTEMPTS_JSONL,
    METRICS_JSONL,
    ANALYSIS_JSON,
)


def is_report_complete(report_dir: Path) -> bool:
    """Check whether a report directory is complete.

    A report is complete only when all required artifacts exist and are
    parseable: manifest.json, attempts.jsonl, metrics.jsonl, and
    analysis.json. A partial report (missing artifacts) returns ``False``.
    A corrupted artifact (e.g. invalid JSON in manifest.json) raises
    ``ReportCompletenessError``.

    Returns ``False`` for nonexistent or empty report directories.
    """
    if not report_dir.exists() or not report_dir.is_dir():
        return False

    for artifact in _REQUIRED_ARTIFACTS:
        path = report_dir / artifact
        if not path.exists():
            return False
        if not path.is_file():
            return False

    # Validate that JSON artifacts are parseable as their typed models. A
    # corrupted manifest or analysis file is an error, not a partial report.
    manifest_path = report_dir / MANIFEST_JSON
    try:
        RunManifest.model_validate_json(manifest_path.read_text())
    except (ValidationError, ValueError, OSError) as e:
        raise ReportCompletenessError(
            f"corrupted {MANIFEST_JSON} in report {report_dir}: {e}"
        ) from e
    analysis_path = report_dir / ANALYSIS_JSON
    try:
        CanonicalEvalAnalysis.model_validate_json(analysis_path.read_text())
    except (ValidationError, ValueError, OSError) as e:
        raise ReportCompletenessError(
            f"corrupted {ANALYSIS_JSON} in report {report_dir}: {e}"
        ) from e

    return True


__all__ = [
    "ReportCompletenessError",
    "is_report_complete",
]
