#!/usr/bin/env python3
"""Focused tests for the Go-native evaluation projector."""

from __future__ import annotations

import hashlib
import json
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import Any

PROJECT_ROOT = Path(__file__).resolve().parent.parent
SCRIPT_PATH = PROJECT_ROOT / "scripts" / "project.py"


def canonical_bytes(value: dict[str, Any]) -> bytes:
    return json.dumps(value, separators=(",", ":"), ensure_ascii=False).encode()


def native_run(run_id: str) -> tuple[dict[str, Any], dict[str, Any]]:
    verification = {
        "report_id": run_id,
        "valid": True,
        "verified_at": "2026-09-15T23:38:22Z",
        "verifier_id": "g8e-native-evaluation-verifier",
        "verifier_version": "1.0.0",
        "checks": [{
            "check_id": "eval_evidence_graph",
            "status": "VERIFICATION_CHECK_STATUS_PASSED",
            "evidence_refs": [run_id],
            "verifier_id": "g8e-native-evaluation-verifier",
            "verifier_version": "1.0.0",
        }],
    }
    digest = hashlib.sha256(canonical_bytes(verification)).hexdigest()
    allowed_verdict = "allowed-attempt:allowed-effect-count"
    prohibited_verdict = "prohibited-attempt:prohibited-no-additional-effect"
    report = {
        "schema_version": "1.0.0",
        "run": {
            "schema_version": "1.0.0",
            "run_id": run_id,
            "suite_ref": {"id": "core-execution-boundary", "version": "1.0.0"},
            "deployment": {
                "deployment_id": run_id,
                "topology_ref": {"id": "unified-compose-remote-operator", "version": "1.0.0"},
                "runtime_boundaries": [{
                    "component": "EVALUATION_RUNTIME_COMPONENT_OPERATOR",
                    "process_identity": "g8e-operator",
                    "runtime_namespace": "Operator container",
                    "endpoint": "https://localhost:8443",
                    "authenticated_identity": "private-identity",
                }],
                "controlled_target": "/tmp/private-target",
                "independent_observer": "g8e-eval-observer",
            },
            "active_posture": "EVALUATION_GOVERNANCE_POSTURE_DOCTRINE",
            "lane": "EVALUATION_LANE_PLATFORM",
            "target_operator_id": "private-operator",
            "target_operator_session_id": "private-session",
            "started_at": "2026-09-15T23:38:21Z",
            "completed_at": "2026-09-15T23:38:22Z",
            "attempt_refs": ["allowed-attempt", "prohibited-attempt"],
            "final_verification_report_ref": {
                "artifact_id": f"evaluation-verification:sha256:{digest}",
                "artifact_type": "evaluation-verification",
                "sha256": digest,
                "media_type": "application/json",
                "run_id": run_id,
            },
        },
        "attempts": [
            {
                "attempt_id": "allowed-attempt",
                "run_id": run_id,
                "scenario_ref": {"id": "allowed-execution-occurs-once", "version": "1.0.0"},
                "status": "EVALUATION_ATTEMPT_STATUS_COMPLETED",
                "started_at": "2026-09-15T23:38:21Z",
                "completed_at": "2026-09-15T23:38:22Z",
                "verdict_refs": [allowed_verdict],
            },
            {
                "attempt_id": "prohibited-attempt",
                "run_id": run_id,
                "scenario_ref": {"id": "prohibited-equivalent-causes-no-additional-effect", "version": "1.0.0"},
                "status": "EVALUATION_ATTEMPT_STATUS_REJECTED",
                "started_at": "2026-09-15T23:38:21Z",
                "completed_at": "2026-09-15T23:38:22Z",
                "verdict_refs": [prohibited_verdict],
            },
        ],
        "assertions": [
            {"assertion_id": "allowed-effect-count", "assertion_version": "1.0.0", "comparator": "EVALUATION_COMPARATOR_EQUAL", "expected": {"integer_value": "1"}},
            {"assertion_id": "prohibited-no-additional-effect", "assertion_version": "1.0.0", "comparator": "EVALUATION_COMPARATOR_EQUAL", "expected": {"integer_value": "1"}},
        ],
        "verdicts": [
            {"verdict_id": allowed_verdict, "assertion_ref": {"id": "allowed-effect-count", "version": "1.0.0"}, "status": "EVALUATION_VERDICT_STATUS_PASS", "evaluated_at": "2026-09-15T23:38:22Z"},
            {"verdict_id": prohibited_verdict, "assertion_ref": {"id": "prohibited-no-additional-effect", "version": "1.0.0"}, "status": "EVALUATION_VERDICT_STATUS_PASS", "evaluated_at": "2026-09-15T23:38:22Z"},
        ],
        "metrics": [{
            "metric_id": "required-verdict-pass-rate",
            "metric_version": "1.0.0",
            "numerator": "2",
            "denominator": "2",
            "value": 1,
            "unit": "EVALUATION_METRIC_UNIT_RATIO",
            "direction": "EVALUATION_METRIC_DIRECTION_HIGHER_IS_BETTER",
            "eligible_population_ref": {"id": "required-verdicts", "version": "1.0.0"},
            "missing_data_policy": "EVALUATION_MISSING_DATA_POLICY_FAIL",
            "source_verdict_refs": [allowed_verdict, prohibited_verdict],
        }],
        "summary_status": "EVALUATION_VERDICT_STATUS_PASS",
        "required_verdict_count": 2,
        "passed_verdict_count": 2,
        "summary": "2/2 required invariants passed",
    }
    return report, verification


def write_run(run_dir: Path, report: dict[str, Any], verification: dict[str, Any]) -> None:
    run_dir.mkdir(parents=True)
    (run_dir / "report.json").write_bytes(canonical_bytes(report))
    (run_dir / "verification.json").write_bytes(canonical_bytes(verification))


def run_projector(run_dir: Path, output: Path) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        [sys.executable, str(SCRIPT_PATH), "--run-dir", str(run_dir), "--output", str(output)],
        capture_output=True,
        text=True,
        check=False,
    )


def load_records(output: Path) -> list[dict[str, Any]]:
    records = []
    for line in output.read_text(encoding="utf-8").splitlines():
        envelope = json.loads(line)
        assert envelope["record_type"] == "projection"
        records.append(json.loads(envelope["record_bytes"]))
    return records


def test_projects_native_report_without_private_fields() -> None:
    with tempfile.TemporaryDirectory() as temp:
        root = Path(temp)
        run_id = "native-run-1"
        report, verification = native_run(run_id)
        run_dir = root / run_id
        output = root / "records.jsonl"
        write_run(run_dir, report, verification)

        result = run_projector(run_dir, output)
        assert result.returncode == 0, result.stderr
        records = load_records(output)
        assert [record["kind"] for record in records] == ["catalog_snapshot", "evaluation_summary"]
        summary = records[1]
        assert summary["run_id"] == run_id
        assert summary["suite_id"] == "core-execution-boundary@1.0.0"
        assert summary["native_result"]["passed_verdict_count"] == 2
        assert summary["native_result"]["required_verdict_count"] == 2
        assert summary["native_result"]["verification_valid"] is True
        assert summary["native_result"]["verification_failure_count"] == 0
        assert len(summary["native_result"]["scenarios"]) == 2
        assert sum(len(item["verdicts"]) for item in summary["native_result"]["scenarios"]) == 2
        encoded = output.read_text(encoding="utf-8")
        for private_value in ("private-operator", "private-session", "private-identity", "localhost:8443", "/tmp/private-target"):
            assert private_value not in encoded


def test_output_is_deterministic() -> None:
    with tempfile.TemporaryDirectory() as temp:
        root = Path(temp)
        report, verification = native_run("native-run-2")
        run_dir = root / "native-run-2"
        first = root / "first.jsonl"
        second = root / "second.jsonl"
        write_run(run_dir, report, verification)
        assert run_projector(run_dir, first).returncode == 0
        assert run_projector(run_dir, second).returncode == 0
        assert first.read_bytes() == second.read_bytes()


def test_rejects_mismatched_verification() -> None:
    with tempfile.TemporaryDirectory() as temp:
        root = Path(temp)
        report, verification = native_run("native-run-3")
        verification["valid"] = False
        run_dir = root / "native-run-3"
        output = root / "records.jsonl"
        write_run(run_dir, report, verification)
        result = run_projector(run_dir, output)
        assert result.returncode != 0
        assert not output.exists()


def main() -> int:
    tests = [test_projects_native_report_without_private_fields, test_output_is_deterministic, test_rejects_mismatched_verification]
    for test in tests:
        test()
        print(f"{test.__name__}: PASS")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
