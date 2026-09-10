# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Standalone report validation for individual eval report directories.

A report is complete only when all required artifacts exist, are valid,
and are internally consistent: manifest.json (RunManifest), attempts.jsonl
(every attempt terminal), metrics.jsonl (required deterministic metrics
directly bound), evidence-index.jsonl (if present), analysis.json
(summary), and report checksum (if present, must match).

This module performs read-only checks on the report directory. It does
not mutate, create, or delete any files. A partial report (missing
artifacts) returns ``ok=False`` with typed failures. A corrupted
artifact (invalid JSON, schema violation) also returns ``ok=False``.
"""

from __future__ import annotations

import hashlib
import json
from pathlib import Path

from pydantic import BaseModel, ConfigDict, Field, ValidationError

from g8e_evals.constants import (
    ANALYSIS_JSON,
    ATTEMPTS_JSONL,
    EVIDENCE_INDEX_JSONL,
    MANIFEST_JSON,
    METRICS_JSONL,
    REPORT_CHECKSUM_JSON,
    TASKS_JSONL,
)
from g8e_evals.schema import AttemptRecord, MetricObservation, RunManifest, TerminalStatus


class StandaloneReportResult(BaseModel):
    """Typed result of standalone report validation.

    ``ok`` is True only when every checked layer passes. ``checked_layers``
    is a sorted list of verification layer names. ``failures`` is a sorted
    list of typed failure messages.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    report_dir: str = Field(description="Report directory path that was validated.")
    ok: bool = Field(description="True when all verification layers pass.")
    checked_layers: list[str] = Field(
        default_factory=list,
        description="Sorted list of verification layer names that were checked.",
    )
    failures: list[str] = Field(
        default_factory=list,
        description="Sorted list of typed verification failure messages.",
    )


def _read_jsonl(path: Path) -> list[dict]:
    """Read a JSONL file and return a list of parsed dicts."""
    records: list[dict] = []
    for line in path.read_text().splitlines():
        line = line.strip()
        if line:
            records.append(json.loads(line))
    return records


def _check_regular_file(path: Path, failures: list[str], label: str) -> bool:
    """Check that a path exists, is a regular file (not a symlink)."""
    if not path.exists():
        failures.append(f"missing {label}: {path.name}")
        return False
    if path.is_symlink():
        failures.append(f"symlink rejected for {label}: {path.name}")
        return False
    if not path.is_file():
        failures.append(f"{label} is not a regular file: {path.name}")
        return False
    return True


def _compute_report_checksum(report_dir: Path) -> str:
    """Compute SHA-256 over canonical JSON of attempts and metrics."""
    attempts_path = report_dir / ATTEMPTS_JSONL
    metrics_path = report_dir / METRICS_JSONL

    attempts_data: list[dict] = []
    if attempts_path.exists():
        for line in attempts_path.read_text().splitlines():
            line = line.strip()
            if line:
                attempts_data.append(json.loads(line))

    metrics_data: list[dict] = []
    if metrics_path.exists():
        for line in metrics_path.read_text().splitlines():
            line = line.strip()
            if line:
                metrics_data.append(json.loads(line))

    payload = json.dumps(
        {"attempts": attempts_data, "metrics": metrics_data},
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return hashlib.sha256(payload.encode()).hexdigest()


def validate_standalone_report(report_dir: Path) -> StandaloneReportResult:
    """Validate a standalone report directory.

    Checks the following layers in order:
    1. **manifest**: manifest.json exists, is a regular file, and is a valid RunManifest.
    2. **attempts**: attempts.jsonl exists, every attempt is terminal, and every task has at least one terminal attempt.
    3. **metrics**: metrics.jsonl exists and every attempt has at least one bound metric.
    4. **evidence_index**: if evidence-index.jsonl exists, it is valid JSONL.
    5. **analysis**: analysis.json exists, is a regular file, and is valid JSON.
    6. **checksum**: if report-checksum.json exists, its checksum matches the computed checksum.

    Returns a ``StandaloneReportResult`` with ``ok=True`` only when all
    layers pass. Missing or corrupted artifacts produce typed failures.
    """
    failures: list[str] = []
    checked_layers: list[str] = []

    # Layer 1: manifest
    checked_layers.append("manifest")
    manifest_path = report_dir / MANIFEST_JSON
    if _check_regular_file(manifest_path, failures, "manifest"):
        try:
            RunManifest.model_validate_json(manifest_path.read_text())
        except (ValidationError, json.JSONDecodeError) as e:
            failures.append(f"manifest validation failed: {e}")

    # Layer 2: attempts
    checked_layers.append("attempts")
    attempts_path = report_dir / ATTEMPTS_JSONL
    attempts: list[AttemptRecord] = []
    if _check_regular_file(attempts_path, failures, "attempts"):
        try:
            raw_records = _read_jsonl(attempts_path)
            attempts = [AttemptRecord.model_validate(r) for r in raw_records]
        except (ValidationError, json.JSONDecodeError) as e:
            failures.append(f"attempts validation failed: {e}")

    # All TerminalStatus values are terminal by definition. An attempt
    # with a non-terminal status (e.g. "running") fails at parse time
    # with a ValidationError, which is caught above. No additional
    # non-terminal check is needed here.

    # Check every task in tasks.jsonl has at least one terminal attempt
    tasks_path = report_dir / TASKS_JSONL
    if tasks_path.exists() and tasks_path.is_file() and not tasks_path.is_symlink():
        try:
            task_records = _read_jsonl(tasks_path)
            task_ids: set[str] = {
                t["task_id"] for t in task_records if t.get("task_id")
            }
            attempted_task_ids = {a.task_id for a in attempts}
            missing_tasks = task_ids - attempted_task_ids
            for tid in sorted(missing_tasks):
                failures.append(f"task {tid} has no terminal attempt")
        except json.JSONDecodeError as e:
            failures.append(f"tasks validation failed: {e}")

    # Layer 3: metrics
    checked_layers.append("metrics")
    metrics_path = report_dir / METRICS_JSONL
    metrics: list[MetricObservation] = []
    if _check_regular_file(metrics_path, failures, "metrics"):
        try:
            raw_records = _read_jsonl(metrics_path)
            metrics = [MetricObservation.model_validate(r) for r in raw_records]
        except (ValidationError, json.JSONDecodeError) as e:
            failures.append(f"metrics validation failed: {e}")

    # Check every completed attempt has at least one bound metric
    metrics_by_attempt: dict[str, list[MetricObservation]] = {}
    for m in metrics:
        metrics_by_attempt.setdefault(m.attempt_id, []).append(m)

    for attempt in attempts:
        if attempt.terminal_status == TerminalStatus.COMPLETED:
            if attempt.attempt_id not in metrics_by_attempt:
                failures.append(
                    f"attempt {attempt.attempt_id} has no bound metric"
                )

    # Layer 4: evidence index (optional)
    evidence_path = report_dir / EVIDENCE_INDEX_JSONL
    if evidence_path.exists():
        checked_layers.append("evidence_index")
        if evidence_path.is_symlink():
            failures.append(f"symlink rejected for evidence index: {evidence_path.name}")
        elif not evidence_path.is_file():
            failures.append(f"evidence index is not a regular file: {evidence_path.name}")
        else:
            try:
                _read_jsonl(evidence_path)
            except json.JSONDecodeError as e:
                failures.append(f"evidence index validation failed: {e}")

    # Layer 5: analysis (summary)
    checked_layers.append("analysis")
    analysis_path = report_dir / ANALYSIS_JSON
    if _check_regular_file(analysis_path, failures, "analysis"):
        try:
            json.loads(analysis_path.read_text())
        except json.JSONDecodeError as e:
            failures.append(f"analysis validation failed: {e}")

    # Layer 6: checksum (optional)
    checksum_path = report_dir / REPORT_CHECKSUM_JSON
    if checksum_path.exists():
        checked_layers.append("checksum")
        if checksum_path.is_symlink():
            failures.append(f"symlink rejected for checksum: {checksum_path.name}")
        elif not checksum_path.is_file():
            failures.append(f"checksum is not a regular file: {checksum_path.name}")
        else:
            try:
                checksum_data = json.loads(checksum_path.read_text())
                declared = checksum_data.get("checksum", "")
                if declared:
                    computed = _compute_report_checksum(report_dir)
                    if declared != computed:
                        failures.append(
                            f"report checksum mismatch: declared={declared}, computed={computed}"
                        )
            except json.JSONDecodeError as e:
                failures.append(f"checksum validation failed: {e}")

    ok = len(failures) == 0
    return StandaloneReportResult(
        report_dir=str(report_dir),
        ok=ok,
        checked_layers=sorted(set(checked_layers)),
        failures=sorted(failures),
    )


__all__ = [
    "StandaloneReportResult",
    "validate_standalone_report",
]
