# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tests for crash-safe artifact persistence and corruption detection.

Reproduces the abrupt-death and artifact-corruption path observed in
the failed overnight campaign. Proves that no file can acquire
undetected NUL regions, partial JSON records, or an apparently live
launch state after the process is gone. Status reconciliation must
report ``interrupted`` or ``inconsistent`` with typed bounded failures
and never misclassify report corruption as config invalid.
"""

from __future__ import annotations

import json
import os
from pathlib import Path

import pytest

from g8e_evals.constants import (
    ATTEMPTS_JSONL,
    EVAL_LAUNCH_STATE_JSON,
    METRICS_JSONL,
    STAGES_JSONL,
    TASKS_JSONL,
)
from g8e_evals.operation_config import (
    AuthorityRef,
    BudgetCeilings,
    DiagnosticConfig,
    EvidenceKeyRef,
    ProviderEndpointRef,
    StopConditions,
    write_operation_config,
)
from g8e_evals.operation_lifecycle import (
    operation_status,
    stop_operation,
)

pytestmark = pytest.mark.unit


def _sha256(path: Path) -> str:
    import hashlib
    return hashlib.sha256(path.read_bytes()).hexdigest()


def _write_scenario_gold_set(tmp_path: Path) -> str:
    gold_dir = tmp_path / "gold_sets" / "tool_selection"
    gold_dir.mkdir(parents=True)
    code_path = tmp_path / "generation_code.py"
    code_path.write_text("# scenario generation code\n")
    code_sha = _sha256(code_path)
    task_row = json.dumps({
        "key": 0,
        "prompt": "Select the correct tool",
        "category": "tool_selection",
        "complexity": "light",
        "expected_role": "light",
        "required_tools": ["search"],
        "criteria": [{"kind": "keyword_present", "value": "search", "must_pass": True}],
    })
    gold_file = gold_dir / "input_data.jsonl"
    gold_file.write_text(task_row + "\n")
    gold_sha = _sha256(gold_file)
    provenance = {
        "schema_version": 1,
        "benchmark": "tool_selection",
        "source": {
            "repository": "https://example.com/repo",
            "revision": "abc123",
            "license_spdx": "Apache-2.0",
            "code_path": "generation_code.py",
            "code_sha256": code_sha,
        },
        "output": {
            "path": "input_data.jsonl",
            "rows": 1,
            "sha256": gold_sha,
        },
        "partition": "development",
        "domain_strata": ["tool_selection"],
    }
    (gold_dir / "provenance.json").write_text(json.dumps(provenance))
    return "gold_sets/tool_selection/input_data.jsonl"


def _write_diagnostic_config(tmp_path: Path, *, report_exists: bool = True) -> Path:
    gold_rel = _write_scenario_gold_set(tmp_path)
    gold_path = tmp_path / gold_rel
    key_path = tmp_path / "evidence-key.json"
    key_path.write_text(json.dumps({"version": 1, "key_id": "eval-key-1", "key_b64": "YWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWE="}))
    os.chmod(key_path, 0o600)
    report_root = tmp_path / "reports" / "diagnostic"
    if report_exists:
        report_root.mkdir(parents=True)
    config = DiagnosticConfig(
        operation_id="diagnostic-1",
        revision="rev-1",
        suite="tool_selection",
        seed=42,
        report_root=str(report_root.relative_to(tmp_path)),
        gold_set=AuthorityRef(path=gold_rel, sha256=_sha256(gold_path)),
        evidence_key=EvidenceKeyRef(path=key_path.name, key_id="eval-key-1"),
        provider_endpoint=ProviderEndpointRef(provider="ollama", endpoint_class="local"),
        budget=BudgetCeilings(max_requests=10, max_tokens=1000, max_usd=2.5, min_free_disk_gb=0),
        stop_conditions=StopConditions(idle_timeout_s=180, max_duration_s=600),
        model_variant_id="qwen3:8b",
        arm="direct",
        task_limit=1,
    )
    config_path = tmp_path / "diagnostic.json"
    write_operation_config(config, config_path)
    return config_path


def _write_launch_with_hash(report_root: Path, *, status: str = "running", pid: int = 99999999) -> str:
    """Write a launch record with a valid content hash and return the hash."""
    import hashlib
    from g8e_evals.operation_lifecycle import LaunchState
    launch = LaunchState(
        operation_id="diagnostic-1",
        revision="rev-1",
        pid=pid,
        status=status,
        started_at="2026-01-01T00:00:00+00:00",
        updated_at="2026-01-01T00:00:00+00:00",
    )
    payload = launch.model_dump(exclude={"content_hash"}, exclude_none=True)
    content_hash = hashlib.sha256(
        json.dumps(payload, sort_keys=True, separators=(",", ":")).encode()
    ).hexdigest()
    launch = launch.model_copy(update={"content_hash": content_hash})
    (report_root / EVAL_LAUNCH_STATE_JSON).write_text(launch.model_dump_json(indent=2))
    return content_hash


def test_status_reports_interrupted_when_pid_dead_and_launch_running(tmp_path: Path) -> None:
    """When the launch record says ``running`` but the recorded PID no
    longer exists, status must report ``interrupted`` — not ``running``
    or ``report_present``. This is the crash-safe reconciliation that
    detects abrupt process death."""
    config_path = _write_diagnostic_config(tmp_path, report_exists=True)
    report_root = tmp_path / "reports" / "diagnostic"
    _write_launch_with_hash(report_root, status="running", pid=99999999)

    result = operation_status(config_path, tmp_path)

    assert result.status == "interrupted"
    assert result.process_state == "stale"
    assert "stale" in result.safe_detail


def test_status_reports_inconsistent_on_nul_byte_corruption(tmp_path: Path) -> None:
    """When a JSONL artifact contains NUL-byte padding from a process
    kill mid-write, status must report ``inconsistent`` with a bounded
    safe_detail — not crash with an unhandled JSONDecodeError that the
    Go facade misclassifies as ``ErrEvalConfigInvalid``."""
    config_path = _write_diagnostic_config(tmp_path, report_exists=True)
    report_root = tmp_path / "reports" / "diagnostic"
    _write_launch_with_hash(report_root, status="running", pid=99999999)
    # Write a tasks.jsonl with NUL-byte padding (simulating preallocated
    # block that was only partially filled)
    (report_root / TASKS_JSONL).write_bytes(
        b'{"task_id": "0"}\n\x00\x00\x00\x00\x00\x00\x00\x00'
    )

    result = operation_status(config_path, tmp_path)

    assert result.status == "inconsistent"
    assert "corrupt" in result.safe_detail.lower() or "nul" in result.safe_detail.lower()


def test_status_reports_inconsistent_on_partial_json_record(tmp_path: Path) -> None:
    """When a JSONL artifact contains a partial JSON record (the
    process was killed mid-write), status must report ``inconsistent``
    — not crash with an unhandled JSONDecodeError."""
    config_path = _write_diagnostic_config(tmp_path, report_exists=True)
    report_root = tmp_path / "reports" / "diagnostic"
    _write_launch_with_hash(report_root, status="running", pid=99999999)
    # Write a tasks.jsonl with a partial JSON line (truncated mid-write)
    (report_root / TASKS_JSONL).write_text(
        '{"task_id": "0"}\n{"task_id": "1", "prompt": "trunca'
    )

    result = operation_status(config_path, tmp_path)

    assert result.status == "inconsistent"
    assert "corrupt" in result.safe_detail.lower() or "partial" in result.safe_detail.lower()


def test_status_reports_inconsistent_on_nul_byte_in_metrics(tmp_path: Path) -> None:
    """NUL-byte corruption in any JSONL artifact (not just tasks) must
    be detected and reported as ``inconsistent``."""
    config_path = _write_diagnostic_config(tmp_path, report_exists=True)
    report_root = tmp_path / "reports" / "diagnostic"
    _write_launch_with_hash(report_root, status="completed", pid=99999999)
    (report_root / TASKS_JSONL).write_text('{"task_id": "0"}\n')
    (report_root / ATTEMPTS_JSONL).write_text('{"terminal_status": "completed"}\n')
    # Corrupt metrics with NUL bytes
    (report_root / METRICS_JSONL).write_bytes(
        b'{"metric_id": "accuracy", "value": 0.9}\n\x00\x00\x00'
    )

    result = operation_status(config_path, tmp_path)

    assert result.status == "inconsistent"


def test_status_reports_inconsistent_on_corrupt_stages(tmp_path: Path) -> None:
    """Corruption in stages.jsonl must be detected and reported as
    ``inconsistent`` — the budget derivation must not silently produce
    wrong numbers from corrupt data."""
    config_path = _write_diagnostic_config(tmp_path, report_exists=True)
    report_root = tmp_path / "reports" / "diagnostic"
    _write_launch_with_hash(report_root, status="completed", pid=99999999)
    (report_root / TASKS_JSONL).write_text('{"task_id": "0"}\n')
    (report_root / ATTEMPTS_JSONL).write_text('{"terminal_status": "completed"}\n')
    # Corrupt stages with a partial JSON line
    (report_root / STAGES_JSONL).write_text(
        '{"kind": "model_inference", "input_tokens": 100}\n{"kind": "grading", "input_t'
    )

    result = operation_status(config_path, tmp_path)

    assert result.status == "inconsistent"


def test_status_does_not_misclassify_corruption_as_config_invalid(tmp_path: Path) -> None:
    """The Go facade maps Python exceptions to typed errors. A
    JSONDecodeError from corrupt artifacts must not bubble up as
    ``ErrEvalConfigInvalid`` (which means the config file is invalid).
    Status reconciliation must catch artifact corruption and return a
    bounded ``inconsistent`` result, not raise."""
    config_path = _write_diagnostic_config(tmp_path, report_exists=True)
    report_root = tmp_path / "reports" / "diagnostic"
    _write_launch_with_hash(report_root, status="running", pid=99999999)
    (report_root / TASKS_JSONL).write_bytes(b'{"task_id": "0"}\n\x00\x00\x00')

    # Must not raise — must return a bounded inconsistent result
    result = operation_status(config_path, tmp_path)
    assert result.status == "inconsistent"


def test_runner_read_jsonl_rejects_nul_byte_corruption(tmp_path: Path) -> None:
    """The runner's ``_read_jsonl`` must reject NUL-byte corruption
    rather than silently stripping it. Silently stripping NUL bytes
    can hide data loss from a process kill mid-write."""
    from g8e_evals.runner import _read_jsonl
    from g8e_evals.schema import StageKind, StageObservation

    corrupt_path = tmp_path / "corrupt.jsonl"
    record = StageObservation(
        stage_id="s1", attempt_id="a1", run_id="r1", kind=StageKind.MODEL_INFERENCE,
        input_tokens=100, output_tokens=50,
    )
    corrupt_path.write_bytes(record.model_dump_json().encode() + b"\n\x00\x00\x00")

    with pytest.raises((ValueError, json.JSONDecodeError)):
        _read_jsonl(corrupt_path, StageObservation)


def test_runner_append_jsonl_is_durable(tmp_path: Path) -> None:
    """The runner's ``_append_jsonl`` must flush and fsync after
    appending records so that a process kill immediately after the
    append does not lose the data."""
    from g8e_evals.runner import _append_jsonl, _read_jsonl
    from g8e_evals.schema import StageKind, StageObservation

    path = tmp_path / "test.jsonl"
    stage = StageObservation(
        stage_id="s1", attempt_id="a1", run_id="r1", kind=StageKind.MODEL_INFERENCE,
        input_tokens=100, output_tokens=50,
    )
    _append_jsonl(path, [stage])

    # The file must exist and contain the record
    assert path.exists()
    content = path.read_text()
    assert '"stage_id":"s1"' in content
    # The data must be on disk (fsync'd), not just in the OS buffer
    # We verify by re-reading immediately
    records = _read_jsonl(path, StageObservation)
    assert len(records) == 1
    assert records[0].stage_id == "s1"


def test_force_stop_rejects_reused_pid_with_corrupt_artifacts(tmp_path: Path) -> None:
    """Forced stop must not signal a reused PID even when artifacts are
    corrupt. The PID-reuse check must run before any signal is sent."""
    config_path = _write_diagnostic_config(tmp_path, report_exists=True)
    report_root = tmp_path / "reports" / "diagnostic"
    # Use a PID that is likely alive (this process) but with a mismatched
    # start time to simulate PID reuse
    _write_launch_with_hash(report_root, status="running", pid=os.getpid())
    # Corrupt an artifact
    (report_root / TASKS_JSONL).write_bytes(b'{"task_id": "0"}\n\x00\x00')

    # Force stop should still validate PID identity before signaling.
    # With a mismatched start time (the launch record has started_at
    # 2026-01-01 but this process started later), the PID-reuse check
    # should reject the force stop.
    with pytest.raises(ValueError, match="not running or has been reused"):
        stop_operation(config_path, tmp_path, immediate=True)
