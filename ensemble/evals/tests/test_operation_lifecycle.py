# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path

import pytest

from g8e_evals.operation_config import (
    AuthorityRef,
    BudgetCeilings,
    DiagnosticConfig,
    EvidenceKeyRef,
    ProviderEndpointRef,
    StopConditions,
    write_operation_config,
)
from g8e_evals.constants import EVAL_LAUNCH_STATE_JSON, EVAL_STOP_REQUEST_JSON
from g8e_evals.operation_lifecycle import (
    check_operation,
    operation_status,
    plan_operation,
    stop_operation,
)

pytestmark = pytest.mark.unit


def _sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def _write_scenario_gold_set(tmp_path: Path) -> str:
    """Create a valid tool_selection scenario gold set directory with provenance.

    Returns the repository-relative path to the gold set file.
    """
    gold_dir = tmp_path / "gold_sets" / "tool_selection"
    gold_dir.mkdir(parents=True)
    code_path = tmp_path / "generation_code.py"
    code_path.write_text("# scenario generation code\n")
    code_sha = hashlib.sha256(code_path.read_bytes()).hexdigest()
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
    gold_sha = hashlib.sha256(gold_file.read_bytes()).hexdigest()
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


def _write_diagnostic_config(tmp_path: Path, *, report_exists: bool = False) -> Path:
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


def test_plan_diagnostic_reports_exact_bounded_dimensions(tmp_path: Path) -> None:
    config_path = _write_diagnostic_config(tmp_path)

    plan = plan_operation(config_path, tmp_path)

    assert plan.operation_kind == "diagnostic"
    assert plan.selected_models == ["qwen3:8b"]
    assert plan.task_count == 1
    assert plan.task_identities == ["0"]
    assert plan.assignment_count == 1
    assert plan.maximum_provider_calls == 10
    assert plan.maximum_tokens == 1000
    assert plan.maximum_usd == 2.5
    assert plan.maximum_duration_s == 600
    assert len(plan.schedule_identity) == 64


def test_plan_diagnostic_rejects_offset_beyond_population(tmp_path: Path) -> None:
    config_path = _write_diagnostic_config(tmp_path)
    from g8e_evals.operation_config import load_operation_config
    config = load_operation_config(config_path)
    config = config.model_copy(update={"task_offset": 99})
    write_operation_config(config, config_path)

    with pytest.raises(ValueError, match="exceeds task population"):
        plan_operation(config_path, tmp_path)


def test_plan_diagnostic_rejects_empty_selection(tmp_path: Path) -> None:
    config_path = _write_diagnostic_config(tmp_path)
    from g8e_evals.operation_config import load_operation_config
    config = load_operation_config(config_path)
    config = config.model_copy(update={"task_offset": 1})
    write_operation_config(config, config_path)

    with pytest.raises(ValueError, match="task selection is empty"):
        plan_operation(config_path, tmp_path)


def test_check_diagnostic_validates_authorities_key_disk_and_fresh_root(tmp_path: Path) -> None:
    config_path = _write_diagnostic_config(tmp_path)

    result = check_operation(config_path, tmp_path)

    assert result.ok is True
    assert {check.check_id for check in result.checks} == {
        "config",
        "authorities",
        "evidence_key",
        "endpoint_identity",
        "report_root",
        "disk",
    }
    assert all(check.status == "pass" for check in result.checks)


def test_check_rejects_authority_hash_drift(tmp_path: Path) -> None:
    config_path = _write_diagnostic_config(tmp_path)
    (tmp_path / "gold_sets" / "tool_selection" / "input_data.jsonl").write_text("[]")

    with pytest.raises(ValueError, match="gold_set SHA-256 mismatch"):
        check_operation(config_path, tmp_path)


def test_check_rejects_reused_report_root(tmp_path: Path) -> None:
    config_path = _write_diagnostic_config(tmp_path, report_exists=True)

    with pytest.raises(FileExistsError, match="report root already exists"):
        check_operation(config_path, tmp_path)


def test_check_rejects_evidence_key_identity_mismatch(tmp_path: Path) -> None:
    config_path = _write_diagnostic_config(tmp_path)
    key_path = tmp_path / "evidence-key.json"
    payload = json.loads(key_path.read_text())
    payload["key_id"] = "different-key"
    key_path.write_text(json.dumps(payload))

    with pytest.raises(ValueError, match="evidence key ID mismatch"):
        check_operation(config_path, tmp_path)


def test_status_distinguishes_live_and_stale_producer_metadata(tmp_path: Path) -> None:
    config_path = _write_diagnostic_config(tmp_path, report_exists=True)
    report_root = tmp_path / "reports" / "diagnostic"
    launch_path = report_root / EVAL_LAUNCH_STATE_JSON
    launch_path.write_text(json.dumps({"operation_id": "diagnostic-1", "revision": "rev-1", "pid": os.getpid(), "status": "running"}))

    live = operation_status(config_path, tmp_path)
    assert live.process_state == "running"

    launch_path.write_text(json.dumps({"operation_id": "diagnostic-1", "revision": "rev-1", "pid": 99999999, "status": "running"}))
    stale = operation_status(config_path, tmp_path)
    assert stale.process_state == "stale"
    assert stale.safe_detail == "producer metadata is stale"


def test_graceful_stop_writes_identity_bound_durable_request(tmp_path: Path) -> None:
    config_path = _write_diagnostic_config(tmp_path, report_exists=True)
    report_root = tmp_path / "reports" / "diagnostic"
    (report_root / EVAL_LAUNCH_STATE_JSON).write_text(json.dumps({"operation_id": "diagnostic-1", "revision": "rev-1", "pid": os.getpid(), "status": "running"}))

    result = stop_operation(config_path, tmp_path, immediate=False)

    assert result.status == "graceful_stop_requested"
    request = json.loads((report_root / EVAL_STOP_REQUEST_JSON).read_text())
    assert request["operation_id"] == "diagnostic-1"
    assert request["revision"] == "rev-1"
    assert len(request["content_hash"]) == 64
