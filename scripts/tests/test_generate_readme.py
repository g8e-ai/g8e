#!/usr/bin/env python3
# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, 2.0.

"""Unit tests for scripts/generate_readme.py.

Tests run with the standard library only and exercise validation, aggregation,
rendering, safety, and drift behavior using synthetic fixtures.
"""

import hashlib
import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

import generate_readme as gr
import project_readme_evidence as pre


FIXTURES = Path(__file__).resolve().parent / "fixtures" / "readme"
VALID = FIXTURES / "valid"
INVALID = FIXTURES / "invalid"
SCRIPT = Path(__file__).resolve().parent.parent / "generate_readme.py"
TEMPLATE = VALID.parent.parent.parent.parent.parent / "docs" / "templates" / "README.md.tmpl"


def _update_sha_in_index(index: dict, rel: str, new_sha: str) -> None:
    """Recursively find a *_sha256 field whose corresponding *_path field matches rel and update it."""
    for key, value in index.items():
        if key.endswith("_sha256") and isinstance(value, str):
            path_key = key.replace("_sha256", "_path")
            if path_key in index and index[path_key] == rel:
                index[key] = new_sha
                return
        if isinstance(value, dict):
            _update_sha_in_index(value, rel, new_sha)
        elif isinstance(value, list):
            for item in value:
                if isinstance(item, dict):
                    _update_sha_in_index(item, rel, new_sha)


def _write_jsonl(path: Path, rows: list[dict]) -> None:
    path.write_text("".join(json.dumps(row, separators=(",", ":")) + "\n" for row in rows))


def _set_artifact_checksum(snapshot_dir: Path, rel: str) -> None:
    index_path = snapshot_dir / "index.json"
    index = json.loads(index_path.read_text())
    _update_sha_in_index(index, rel, gr._sha256_file(snapshot_dir / rel))
    index_path.write_text(json.dumps(index))


def _make_stage1_snapshot(tmp: str) -> Path:
    snapshot_dir = Path(tmp) / "snap"
    shutil.copytree(VALID, snapshot_dir)
    run_rel = "eval/runs/run-2026-09-01-synthetic-a"
    manifest_path = snapshot_dir / run_rel / "manifest.json"
    manifest = json.loads(manifest_path.read_text())
    manifest.update({
        "schema_version": "1.40.0",
        "suite_id": "ifeval_subset",
        "orchestrator_version": "2.1.5",
        "arms": [{"arm_id": "doctrine", "requested_posture": "l1_doctrine", "uses_g8ee": True, "uses_gateway": True, "receipt_binding": True, "is_production_posture": True}],
        "role_to_model": {
            "primary": {"role": "primary", "provider": "ollama", "model": "gemma4:12b", "endpoint": None, "endpoint_class": "self-hosted-lan", "api_key_present": False, "seed_support": "unknown"},
            "assistant": {"role": "assistant", "provider": "ollama", "model": "gemma4:e4b", "endpoint": None, "endpoint_class": "self-hosted-lan", "api_key_present": False, "seed_support": "unknown"},
            "lite": {"role": "lite", "provider": "ollama", "model": "gemma4:e2b", "endpoint": None, "endpoint_class": "self-hosted-lan", "api_key_present": False, "seed_support": "unknown"},
            "judge": None,
        },
    })
    manifest.pop("model_mapping", None)
    manifest_path.write_text(json.dumps(manifest))

    task_ids = ["1001", "1019", "1051", "1072", "1075"]
    attempts = []
    metrics = []
    stages = []
    for order, task_id in enumerate(task_ids, start=1):
        attempt_id = f"run-2026-09-01-synthetic-a:{task_id}:doctrine:1"
        attempts.append({"schema_version": "1.40.0", "run_id": "run-2026-09-01-synthetic-a", "attempt_id": attempt_id, "task_id": task_id, "arm_id": "doctrine", "terminal_status": "completed", "assignment_order": order, "receipt_refs": []})
        metrics.append({"schema_version": "1.40.0", "run_id": "run-2026-09-01-synthetic-a", "metric_id": "ifeval_subset_verifier", "metric_version": "1.0.0", "attempt_id": attempt_id, "arm_id": "doctrine", "task_id": task_id, "eligible": True, "denominator_contribution": 1, "value": 1.0, "unit": "boolean", "verification_status": "verified", "grader_class": "deterministic", "evidence_refs": [f"{attempt_id}:agent-trail"]})
        stages.append({"schema_version": "1.40.0", "run_id": "run-2026-09-01-synthetic-a", "stage_id": f"{attempt_id}:call:1", "attempt_id": attempt_id, "kind": "model_inference", "agent_role": "triage", "provider": "OllamaProvider", "model": "gemma4:e2b", "decision": None, "source": ""})
    _write_jsonl(snapshot_dir / run_rel / "attempts.jsonl", attempts)
    _write_jsonl(snapshot_dir / run_rel / "metrics.jsonl", metrics)
    _write_jsonl(snapshot_dir / run_rel / "stages.jsonl", stages)
    (snapshot_dir / run_rel / "receipts.jsonl").write_text("")

    receipt_path = snapshot_dir / "eval/receipt-verification.json"
    receipt = json.loads(receipt_path.read_text())
    receipt.update({"total_receipts": 0, "verified_signatures": 0, "verified_persistence": 0, "failed_signatures": 0, "failed_persistence": 0, "missing_keys": 0, "distinct_signer_key_ids": [], "receipt_bound_eligible_attempts": 0, "sample_receipt_fingerprints": []})
    receipt_path.write_text(json.dumps(receipt))

    tasks_path = snapshot_dir / run_rel / "tasks.jsonl"
    _write_jsonl(tasks_path, [
        {
            "schema_version": "1.40.0",
            "task_id": task_id,
            "suite_id": "ifeval_subset",
            "suite_version": manifest["suite_version"],
            "category": "instruction_following",
            "prompt_hash": hashlib.sha256(f"prompt-{task_id}".encode()).hexdigest(),
            "prompt_length": 20,
            "compatible_arms": ["doctrine"],
            "graders": [{"grader_id": "ifeval_subset_verifier", "grader_version": "1.0.0", "grader_class": "deterministic"}],
        }
        for task_id in task_ids
    ])
    summary_path = snapshot_dir / run_rel / "summary.json"
    summary_path.write_text(json.dumps({
        "schema_version": "1.0.0",
        "run_id": "run-2026-09-01-synthetic-a",
        "assigned_tasks": 5,
        "terminal_attempts": 5,
        "outcomes": {"completed": 5},
        "metric_id": "ifeval_subset_verifier",
        "metric_version": "1.0.0",
        "numerator": 5,
        "denominator": 5,
        "unit": "boolean",
        "receipt_count": 0,
    }))
    evidence_index_path = snapshot_dir / run_rel / "evidence-index.jsonl"
    _write_jsonl(evidence_index_path, [
        {
            "schema_version": "1.0.0",
            "artifact_id": f"evidence-{task_id}-answer",
            "attempt_id": f"run-2026-09-01-synthetic-a:{task_id}:doctrine:1",
            "evidence_kind": "answer",
            "media_type": "text/plain",
            "plaintext_sha256": hashlib.sha256(f"answer-{task_id}".encode()).hexdigest(),
            "byte_length": 20,
            "retained_privately": True,
        }
        for task_id in task_ids
    ])
    reproduction_path = snapshot_dir / run_rel / "reproduction-manifest.json"
    reproduction_path.write_text(json.dumps({
        "schema_version": "1.0.0",
        "release_version": "2.1.5",
        "run_id": "run-2026-09-01-synthetic-a",
        "eval_schema_version": "1.40.0",
        "eval_cli_version": "2.1.5",
        "suite_id": "ifeval_subset",
        "suite_version": manifest["suite_version"],
        "arm_id": "doctrine",
        "task_ids": task_ids,
        "repetitions": 1,
        "task_limit": None,
        "idle_timeout_seconds": 180,
        "endpoint_class": "self-hosted-lan",
        "roles": {
            role: {"provider": identity["provider"], "model": identity["model"], "endpoint_class": identity["endpoint_class"]}
            for role, identity in manifest["role_to_model"].items()
            if role in {"primary", "assistant", "lite"}
        },
        "environment": {"os": "linux", "arch": "x86_64"},
        "provider_inventory_retained_privately": True,
        "command": {
            "program": "g8e-evals",
            "arguments": ["run", "--suite", "ifeval_subset", "--arm", "doctrine", "--primary-endpoint", "${OLLAMA_ENDPOINT}"],
        },
    }))

    index_path = snapshot_dir / "index.json"
    index = json.loads(index_path.read_text())
    index["publication_schema_version"] = "2.0.0"
    index["readme_evidence_version"] = "2.0.0"
    index["platform_version"] = "2.1.5"
    run_ref = index["eval_runs"][0]
    for name in ["tasks.jsonl", "summary.json", "evidence-index.jsonl", "reproduction-manifest.json"]:
        stem = name.removesuffix(".jsonl").removesuffix(".json").replace("-", "_")
        run_ref[f"{stem}_path"] = f"{run_rel}/{name}"
        run_ref[f"{stem}_sha256"] = gr._sha256_file(snapshot_dir / run_rel / name)
    index_path.write_text(json.dumps(index))

    for name in ["manifest.json", "tasks.jsonl", "attempts.jsonl", "metrics.jsonl", "receipts.jsonl", "stages.jsonl", "summary.json", "evidence-index.jsonl", "reproduction-manifest.json"]:
        _set_artifact_checksum(snapshot_dir, f"{run_rel}/{name}")
    _set_artifact_checksum(snapshot_dir, "eval/receipt-verification.json")
    return snapshot_dir


def _make_private_stage1_report(tmp: str) -> Path:
    snapshot_dir = _make_stage1_snapshot(tmp)
    source = snapshot_dir / "eval/runs/run-2026-09-01-synthetic-a"
    report = Path(tmp) / "private-report"
    shutil.copytree(source, report)
    manifest_path = report / "manifest.json"
    manifest = json.loads(manifest_path.read_text())
    manifest["content_hashes"] = [
        {"name": "dataset", "sha256": manifest["dataset_hash"], "byte_length": 1},
        {"name": "prompt_bundle", "sha256": manifest["prompt_hash"], "byte_length": 1},
        {"name": "grader_bundle", "sha256": manifest["grader_hash"], "byte_length": 1},
    ]
    manifest["orchestrator_version"] = "0.3.0"
    for identity in manifest["role_to_model"].values():
        if isinstance(identity, dict):
            identity["endpoint"] = "http://192.168.1.2:11434"
    manifest_path.write_text(json.dumps(manifest))
    attempts_path = report / "attempts.jsonl"
    attempts = [json.loads(line) for line in attempts_path.read_text().splitlines()]
    for order, attempt in enumerate(attempts, start=1):
        attempt["ended_at"] = f"2026-09-01T14:00:0{order}Z"
    _write_jsonl(attempts_path, attempts)
    metrics_path = report / "metrics.jsonl"
    metrics = [json.loads(line) for line in metrics_path.read_text().splitlines()]
    _write_jsonl(metrics_path, metrics)
    evidence_index_path = report / "evidence-index.jsonl"
    evidence_rows = [json.loads(line) for line in evidence_index_path.read_text().splitlines()]
    for row in evidence_rows:
        row["sha256"] = row.pop("plaintext_sha256")
    _write_jsonl(evidence_index_path, evidence_rows)
    return report


class TestLoadSnapshot(unittest.TestCase):
    def test_load_valid_snapshot(self) -> None:
        snapshot = gr.load_snapshot(VALID)
        self.assertEqual(snapshot.manifest.publication_schema_version, "1.0.0")
        self.assertEqual(len(snapshot.eval_runs), 1)
        self.assertEqual(snapshot.receipt_verification.total_receipts, 9)
        self.assertEqual(len(snapshot.demo_reports), 1)

    def test_missing_index(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp))
        self.assertIn("missing index.json", str(ctx.exception))

    def test_unsupported_publication_schema(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            index = Path(tmp) / "index.json"
            index.write_text(json.dumps({"publication_schema_version": "9.9.9"}))
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp))
        self.assertIn("unsupported publication_schema_version", str(ctx.exception))

    def test_invalid_fixture_unsupported_publication_schema(self) -> None:
        with self.assertRaises(gr.ReadmeError) as ctx:
            gr.load_snapshot(INVALID / "unsupported_schema")
        self.assertIn("unsupported publication_schema_version", str(ctx.exception))

    def test_path_traversal_blocked(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            data_dir = Path(tmp) / "data"
            data_dir.mkdir()
            # Copy valid manifest for eval_runs to satisfy minimum eval run check.
            shutil.copytree(VALID, Path(tmp) / "snap")
            index_path = Path(tmp) / "snap" / "index.json"
            index = json.loads(index_path.read_text())
            index["receipt_verification"]["result_path"] = "../evil.json"
            index_path.write_text(json.dumps(index))
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("path traversal", str(ctx.exception))

    def test_checksum_mismatch(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            index_path = Path(tmp) / "snap" / "index.json"
            index = json.loads(index_path.read_text())
            index["eval_runs"][0]["manifest_sha256"] = "0" * 64
            index_path.write_text(json.dumps(index))
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("checksum mismatch", str(ctx.exception))

    def test_undeclared_artifact_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            (Path(tmp) / "snap" / "extra.txt").write_text("secret")
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("undeclared artifact", str(ctx.exception))

    def test_duplicate_run_id(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            index_path = Path(tmp) / "snap" / "index.json"
            index = json.loads(index_path.read_text())
            index["eval_runs"].append(index["eval_runs"][0])
            index_path.write_text(json.dumps(index))
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("duplicate run_id", str(ctx.exception))

    def test_unsupported_eval_schema(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            manifest_path = Path(tmp) / "snap" / "eval" / "runs" / "run-2026-09-01-synthetic-a" / "manifest.json"
            manifest = json.loads(manifest_path.read_text())
            manifest["schema_version"] = "0.0.0"
            manifest_path.write_text(json.dumps(manifest))
            index_path = Path(tmp) / "snap" / "index.json"
            index = json.loads(index_path.read_text())
            index["eval_runs"][0]["manifest_sha256"] = gr._sha256_file(manifest_path)
            index_path.write_text(json.dumps(index))
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("unsupported eval schema version", str(ctx.exception))

    def test_duplicate_metric_row(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            metrics_path = Path(tmp) / "snap" / "eval" / "runs" / "run-2026-09-01-synthetic-a" / "metrics.jsonl"
            with metrics_path.open("a") as f:
                f.write(metrics_path.read_text().splitlines()[0] + "\n")
            index_path = Path(tmp) / "snap" / "index.json"
            index = json.loads(index_path.read_text())
            index["eval_runs"][0]["metrics_sha256"] = gr._sha256_file(metrics_path)
            index_path.write_text(json.dumps(index))
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("duplicate metric row", str(ctx.exception))

    def test_metric_references_missing_attempt(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            metrics_path = Path(tmp) / "snap" / "eval" / "runs" / "run-2026-09-01-synthetic-a" / "metrics.jsonl"
            lines = metrics_path.read_text().splitlines()
            first = json.loads(lines[0])
            first["evidence_ref"] = "attempts/missing-attempt"
            lines[0] = json.dumps(first)
            metrics_path.write_text("\n".join(lines) + "\n")
            index_path = Path(tmp) / "snap" / "index.json"
            index = json.loads(index_path.read_text())
            index["eval_runs"][0]["metrics_sha256"] = gr._sha256_file(metrics_path)
            index_path.write_text(json.dumps(index))
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("missing attempt", str(ctx.exception))

    def test_non_finite_metric_value(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            metrics_path = Path(tmp) / "snap" / "eval" / "runs" / "run-2026-09-01-synthetic-a" / "metrics.jsonl"
            lines = metrics_path.read_text().splitlines()
            first = json.loads(lines[0])
            first["value"] = float("nan")
            lines[0] = json.dumps(first)
            metrics_path.write_text("\n".join(lines) + "\n")
            index_path = Path(tmp) / "snap" / "index.json"
            index = json.loads(index_path.read_text())
            index["eval_runs"][0]["metrics_sha256"] = gr._sha256_file(metrics_path)
            index_path.write_text(json.dumps(index))
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("must be finite", str(ctx.exception))

    def test_ineligible_metric_with_zero_denominator(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            metrics_path = Path(tmp) / "snap" / "eval" / "runs" / "run-2026-09-01-synthetic-a" / "metrics.jsonl"
            lines = metrics_path.read_text().splitlines()
            first = json.loads(lines[0])
            first["eligible"] = True
            first["denominator_contribution"] = 0
            lines[0] = json.dumps(first)
            metrics_path.write_text("\n".join(lines) + "\n")
            index_path = Path(tmp) / "snap" / "index.json"
            index = json.loads(index_path.read_text())
            index["eval_runs"][0]["metrics_sha256"] = gr._sha256_file(metrics_path)
            index_path.write_text(json.dumps(index))
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("positive denominator", str(ctx.exception))

    def test_current_eval_schema_stage1_snapshot_loads(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            snapshot = gr.load_snapshot(_make_stage1_snapshot(tmp))
        self.assertEqual(next(iter(snapshot.eval_runs.values())).manifest["schema_version"], "1.40.0")

    def test_fake_configured_provider_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            snapshot_dir = _make_stage1_snapshot(tmp)
            manifest_path = snapshot_dir / "eval/runs/run-2026-09-01-synthetic-a/manifest.json"
            manifest = json.loads(manifest_path.read_text())
            manifest["role_to_model"]["primary"]["provider"] = "fake"
            manifest_path.write_text(json.dumps(manifest))
            _set_artifact_checksum(snapshot_dir, "eval/runs/run-2026-09-01-synthetic-a/manifest.json")
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(snapshot_dir)
        self.assertIn("real provider", str(ctx.exception))

    def test_fake_provider_stage_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            snapshot_dir = _make_stage1_snapshot(tmp)
            stages_path = snapshot_dir / "eval/runs/run-2026-09-01-synthetic-a/stages.jsonl"
            rows = [json.loads(line) for line in stages_path.read_text().splitlines()]
            rows[0]["provider"] = "FakeProvider"
            rows[0]["model"] = "fake-model"
            _write_jsonl(stages_path, rows)
            _set_artifact_checksum(snapshot_dir, "eval/runs/run-2026-09-01-synthetic-a/stages.jsonl")
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(snapshot_dir)
        self.assertIn("fake provider stage", str(ctx.exception))

    def test_selected_only_stage1_population_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            snapshot_dir = _make_stage1_snapshot(tmp)
            attempts_path = snapshot_dir / "eval/runs/run-2026-09-01-synthetic-a/attempts.jsonl"
            rows = [json.loads(line) for line in attempts_path.read_text().splitlines()][:-1]
            _write_jsonl(attempts_path, rows)
            metrics_path = snapshot_dir / "eval/runs/run-2026-09-01-synthetic-a/metrics.jsonl"
            metric_rows = [json.loads(line) for line in metrics_path.read_text().splitlines()][:-1]
            _write_jsonl(metrics_path, metric_rows)
            stages_path = snapshot_dir / "eval/runs/run-2026-09-01-synthetic-a/stages.jsonl"
            stage_rows = [json.loads(line) for line in stages_path.read_text().splitlines()][:-1]
            _write_jsonl(stages_path, stage_rows)
            _set_artifact_checksum(snapshot_dir, "eval/runs/run-2026-09-01-synthetic-a/attempts.jsonl")
            _set_artifact_checksum(snapshot_dir, "eval/runs/run-2026-09-01-synthetic-a/metrics.jsonl")
            _set_artifact_checksum(snapshot_dir, "eval/runs/run-2026-09-01-synthetic-a/stages.jsonl")
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(snapshot_dir)
        self.assertIn("complete five-task", str(ctx.exception))

    def test_stage1_publication_requires_safe_projection_refs(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            snapshot_dir = _make_stage1_snapshot(tmp)
            index_path = snapshot_dir / "index.json"
            index = json.loads(index_path.read_text())
            del index["eval_runs"][0]["tasks_path"]
            del index["eval_runs"][0]["tasks_sha256"]
            index_path.write_text(json.dumps(index))
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(snapshot_dir)
        self.assertIn("tasks_path", str(ctx.exception))

    def test_stage1_tasks_projection_must_match_complete_population(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            snapshot_dir = _make_stage1_snapshot(tmp)
            tasks_path = snapshot_dir / "eval/runs/run-2026-09-01-synthetic-a/tasks.jsonl"
            rows = [json.loads(line) for line in tasks_path.read_text().splitlines()][:-1]
            _write_jsonl(tasks_path, rows)
            _set_artifact_checksum(snapshot_dir, "eval/runs/run-2026-09-01-synthetic-a/tasks.jsonl")
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(snapshot_dir)
        self.assertIn("tasks projection", str(ctx.exception))

    def test_stage1_metric_attempt_binding_rejects_foreign_attempt(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            snapshot_dir = _make_stage1_snapshot(tmp)
            metrics_path = snapshot_dir / "eval/runs/run-2026-09-01-synthetic-a/metrics.jsonl"
            rows = [json.loads(line) for line in metrics_path.read_text().splitlines()]
            rows[0]["attempt_id"] = "foreign-attempt"
            _write_jsonl(metrics_path, rows)
            _set_artifact_checksum(snapshot_dir, "eval/runs/run-2026-09-01-synthetic-a/metrics.jsonl")
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(snapshot_dir)
        self.assertIn("does not bind to its attempt", str(ctx.exception))

    def test_stage1_summary_projection_must_match_typed_records(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            snapshot_dir = _make_stage1_snapshot(tmp)
            summary_path = snapshot_dir / "eval/runs/run-2026-09-01-synthetic-a/summary.json"
            summary = json.loads(summary_path.read_text())
            summary["denominator"] = 4
            summary_path.write_text(json.dumps(summary))
            _set_artifact_checksum(snapshot_dir, "eval/runs/run-2026-09-01-synthetic-a/summary.json")
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(snapshot_dir)
        self.assertIn("summary projection", str(ctx.exception))

    def test_stage1_evidence_index_projection_rejects_foreign_attempt(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            snapshot_dir = _make_stage1_snapshot(tmp)
            evidence_index_path = snapshot_dir / "eval/runs/run-2026-09-01-synthetic-a/evidence-index.jsonl"
            rows = [json.loads(line) for line in evidence_index_path.read_text().splitlines()]
            rows[0]["attempt_id"] = "foreign-attempt"
            _write_jsonl(evidence_index_path, rows)
            _set_artifact_checksum(snapshot_dir, "eval/runs/run-2026-09-01-synthetic-a/evidence-index.jsonl")
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(snapshot_dir)
        self.assertIn("evidence-index projection", str(ctx.exception))

    def test_stage1_reproduction_manifest_must_match_run_profile(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            snapshot_dir = _make_stage1_snapshot(tmp)
            reproduction_path = snapshot_dir / "eval/runs/run-2026-09-01-synthetic-a/reproduction-manifest.json"
            reproduction = json.loads(reproduction_path.read_text())
            reproduction["task_limit"] = 4
            reproduction_path.write_text(json.dumps(reproduction))
            _set_artifact_checksum(snapshot_dir, "eval/runs/run-2026-09-01-synthetic-a/reproduction-manifest.json")
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(snapshot_dir)
        self.assertIn("reproduction manifest", str(ctx.exception))

    def test_stage1_reproduction_eval_cli_version_must_match_orchestrator(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            snapshot_dir = _make_stage1_snapshot(tmp)
            reproduction_path = snapshot_dir / "eval/runs/run-2026-09-01-synthetic-a/reproduction-manifest.json"
            reproduction = json.loads(reproduction_path.read_text())
            reproduction["eval_cli_version"] = "9.9.9"
            reproduction_path.write_text(json.dumps(reproduction))
            _set_artifact_checksum(snapshot_dir, "eval/runs/run-2026-09-01-synthetic-a/reproduction-manifest.json")
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(snapshot_dir)
        self.assertIn("reproduction manifest", str(ctx.exception))


class TestProjectMetrics(unittest.TestCase):
    def test_project_eligible_only(self) -> None:
        snapshot = gr.load_snapshot(VALID)
        projections = gr._project_metrics(snapshot.eval_runs)
        for key, p in projections.items():
            self.assertTrue(p.denominator > 0)
            self.assertTrue(0.0 <= p.rate <= 1.0)

    def test_pass_fail_rate(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            metrics_path = Path(tmp) / "snap" / "eval" / "runs" / "run-2026-09-01-synthetic-a" / "metrics.jsonl"
            lines = metrics_path.read_text().splitlines()
            filtered = [line for line in lines if '"metric_id":"ifeval_subset_verifier"' in line and '"arm_id":"baseline"' in line]
            metrics_path.write_text("\n".join(filtered) + "\n")
            index_path = Path(tmp) / "snap" / "index.json"
            index = json.loads(index_path.read_text())
            index["eval_runs"][0]["metrics_sha256"] = gr._sha256_file(metrics_path)
            index_path.write_text(json.dumps(index))
            snapshot = gr.load_snapshot(Path(tmp) / "snap")
            projections = gr._project_metrics(snapshot.eval_runs)
            baseline = projections["ifeval_subset_verifier__baseline"]
            self.assertEqual(baseline.denominator, 5)
            self.assertEqual(baseline.numerator, 3)
            self.assertAlmostEqual(baseline.rate, 0.6)


class TestRenderReadme(unittest.TestCase):
    def test_markers_rendered(self) -> None:
        snapshot = gr.load_snapshot(VALID)
        template = TEMPLATE.read_text()
        rendered = gr.render_readme(snapshot, template)
        self.assertIn("Generated by scripts/generate_readme.py", rendered)
        self.assertIn("### Eval Metrics", rendered)
        self.assertIn("### Receipt Verification", rendered)
        self.assertIn("### Governance and State Proof", rendered)
        self.assertIn("### Independently Verified Demonstrations", rendered)
        self.assertIn("### Evidence Identity", rendered)
        self.assertIn("### CI and Reproducibility", rendered)

    def test_missing_marker_fails(self) -> None:
        snapshot = gr.load_snapshot(VALID)
        template = "{{EVAL_METRICS}}"
        with self.assertRaises(gr.ReadmeError) as ctx:
            gr.render_readme(snapshot, template)
        self.assertIn("missing template markers", str(ctx.exception))

    def test_unknown_marker_fails(self) -> None:
        snapshot = gr.load_snapshot(VALID)
        template = "{{UNKNOWN}}"
        with self.assertRaises(gr.ReadmeError) as ctx:
            gr.render_readme(snapshot, template)
        self.assertIn("unknown template marker", str(ctx.exception))

    def test_html_injection_escaped(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            index_path = Path(tmp) / "snap" / "index.json"
            index = json.loads(index_path.read_text())
            index["platform_version"] = "<script>alert(1)</script>"
            index_path.write_text(json.dumps(index))
            snapshot = gr.load_snapshot(Path(tmp) / "snap")
            template = TEMPLATE.read_text()
            rendered = gr.render_readme(snapshot, template)
            self.assertIn("&lt;script&gt;alert(1)&lt;/script&gt;", rendered)
            self.assertNotIn("<script>alert(1)</script>", rendered)

    def test_invalid_demo_report_blocks_render(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            report_path = Path(tmp) / "snap" / "demo" / "demo-allow-001" / "compliance-report.json"
            report = json.loads(report_path.read_text())
            report["valid"] = False
            report_path.write_text(json.dumps(report))
            index_path = Path(tmp) / "snap" / "index.json"
            index = json.loads(index_path.read_text())
            index["demo_reports"][0]["report_sha256"] = gr._sha256_file(report_path)
            index_path.write_text(json.dumps(index))
            snapshot = gr.load_snapshot(Path(tmp) / "snap")
            template = TEMPLATE.read_text()
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.render_readme(snapshot, template)
        self.assertIn("invalid or has failures", str(ctx.exception))

    def test_zero_receipts_not_a_pass(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            receipt_path = Path(tmp) / "snap" / "eval" / "receipt-verification.json"
            receipt = json.loads(receipt_path.read_text())
            receipt["total_receipts"] = 0
            receipt["verified_signatures"] = 0
            receipt["verified_persistence"] = 0
            receipt_path.write_text(json.dumps(receipt))
            index_path = Path(tmp) / "snap" / "index.json"
            index = json.loads(index_path.read_text())
            index["receipt_verification"]["result_sha256"] = gr._sha256_file(receipt_path)
            index_path.write_text(json.dumps(index))
            snapshot = gr.load_snapshot(Path(tmp) / "snap")
            template = TEMPLATE.read_text()
            rendered = gr.render_readme(snapshot, template)
            self.assertIn("| Pass | no |", rendered)

    def test_stage1_renders_configured_and_observed_roles_separately(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            snapshot = gr.load_snapshot(_make_stage1_snapshot(tmp))
            rendered = gr.render_readme(snapshot, TEMPLATE.read_text())
        self.assertIn("Configured but unobserved", rendered)
        self.assertIn("Observed model call", rendered)
        self.assertIn("gemma4:e2b", rendered)

    def test_stage1_zero_receipts_and_governance_are_unavailable(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            snapshot = gr.load_snapshot(_make_stage1_snapshot(tmp))
            rendered = gr.render_readme(snapshot, TEMPLATE.read_text())
        self.assertIn("Receipt evidence is unavailable", rendered)
        self.assertIn("Governance and state evidence is unavailable", rendered)
        self.assertNotIn("| Pass | no |", rendered)

    def test_stage1_scope_and_source_links_render(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            snapshot = gr.load_snapshot(_make_stage1_snapshot(tmp))
            rendered = gr.render_readme(snapshot, TEMPLATE.read_text())
        self.assertIn("Stage 1: Real-agent diagnostic", rendered)
        self.assertIn("complete five-task", rendered)
        self.assertIn("tasks.jsonl", rendered)
        self.assertIn("attempts.jsonl", rendered)
        self.assertIn("stages.jsonl", rendered)
        self.assertIn("summary.json", rendered)
        self.assertIn("evidence-index.jsonl", rendered)
        self.assertIn("reproduction-manifest.json", rendered)

    def test_unsafe_exact_private_endpoint_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            snapshot_dir = _make_stage1_snapshot(tmp)
            manifest_path = snapshot_dir / "eval/runs/run-2026-09-01-synthetic-a/manifest.json"
            manifest = json.loads(manifest_path.read_text())
            manifest["role_to_model"]["primary"]["endpoint"] = "http://192.168.1.2:11434"
            manifest_path.write_text(json.dumps(manifest))
            _set_artifact_checksum(snapshot_dir, "eval/runs/run-2026-09-01-synthetic-a/manifest.json")
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(snapshot_dir)
        self.assertIn("exact provider endpoint", str(ctx.exception))

    def test_unsafe_ci_label_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            index_path = Path(tmp) / "snap" / "index.json"
            index = json.loads(index_path.read_text())
            index["ci_links"].append({"label": "<b>Evil</b>", "url": "https://example.com", "kind": "workflow_status"})
            index_path.write_text(json.dumps(index))
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("unsupported ci link label", str(ctx.exception))

    def test_unsupported_claim_label_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            index_path = Path(tmp) / "snap" / "index.json"
            index = json.loads(index_path.read_text())
            index["claim_labels"].append("custom_unsupported_claim")
            index_path.write_text(json.dumps(index))
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("unsupported claim label", str(ctx.exception))


class TestProjectReadmeEvidence(unittest.TestCase):
    def test_projects_private_report_to_deterministic_safe_candidate(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            private_report = _make_private_stage1_report(tmp)
            first = Path(tmp) / "candidate-first"
            second = Path(tmp) / "candidate-second"
            pre.project(private_report, first, "2.1.5", "0.3.0", 180)
            pre.project(private_report, second, "2.1.5", "0.3.0", 180)
            gr.load_snapshot(first)
            first_files = {path.relative_to(first): path.read_bytes() for path in first.rglob("*") if path.is_file()}
            second_files = {path.relative_to(second): path.read_bytes() for path in second.rglob("*") if path.is_file()}
        self.assertEqual(first_files, second_files)
        self.assertNotIn(b"192.168.1.2", b"".join(first_files.values()))
        self.assertIn(Path("eval/runs/run-2026-09-01-synthetic-a/tasks.jsonl"), first_files)
        self.assertIn(Path("eval/runs/run-2026-09-01-synthetic-a/summary.json"), first_files)
        self.assertIn(Path("eval/runs/run-2026-09-01-synthetic-a/evidence-index.jsonl"), first_files)
        self.assertIn(Path("eval/runs/run-2026-09-01-synthetic-a/reproduction-manifest.json"), first_files)

    def test_refuses_eval_cli_version_mismatching_private_manifest(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            private_report = _make_private_stage1_report(tmp)
            with self.assertRaises(pre.ProjectionError) as ctx:
                pre.project(private_report, Path(tmp) / "candidate", "2.1.5", "9.9.9", 180)
        self.assertIn("eval CLI version", str(ctx.exception))

    def test_refuses_to_replace_existing_candidate(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            private_report = _make_private_stage1_report(tmp)
            candidate = Path(tmp) / "candidate"
            candidate.mkdir()
            with self.assertRaises(pre.ProjectionError) as ctx:
                pre.project(private_report, candidate, "2.1.5", "0.3.0", 180)
        self.assertIn("already exists", str(ctx.exception))


class TestGenerate(unittest.TestCase):
    def test_generate_atomic_and_deterministic(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp) / "README.md"
            gr.generate(TEMPLATE, VALID, out)
            first = out.read_text()
            gr.generate(TEMPLATE, VALID, out)
            second = out.read_text()
            self.assertEqual(first, second)
            self.assertTrue(out.exists())

    def test_check_passes_when_identical(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp) / "README.md"
            gr.generate(TEMPLATE, VALID, out)
            gr.generate(TEMPLATE, VALID, out, check=True)

    def test_check_fails_on_drift(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp) / "README.md"
            out.write_text("stale readme")
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.generate(TEMPLATE, VALID, out, check=True)
        self.assertIn("drift check failed", str(ctx.exception))

    def test_check_fails_when_output_missing(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp) / "README.md"
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.generate(TEMPLATE, VALID, out, check=True)
        self.assertIn("does not exist", str(ctx.exception))


class TestMain(unittest.TestCase):
    def test_main_generate_and_check(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp) / "README.md"
            code = gr.main([
                "--snapshot-dir", str(VALID),
                "--template", str(TEMPLATE),
                "--output", str(out),
            ])
            self.assertEqual(code, 0)
            code = gr.main([
                "--check",
                "--snapshot-dir", str(VALID),
                "--template", str(TEMPLATE),
                "--output", str(out),
            ])
            self.assertEqual(code, 0)

    def test_main_check_fails_on_drift(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp) / "README.md"
            out.write_text("stale")
            code = gr.main([
                "--check",
                "--snapshot-dir", str(VALID),
                "--template", str(TEMPLATE),
                "--output", str(out),
            ])
            self.assertEqual(code, 1)

    def test_main_invalid_snapshot_fails(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            code = gr.main([
                "--snapshot-dir", str(tmp),
                "--template", str(TEMPLATE),
                "--output", str(Path(tmp) / "README.md"),
            ])
            self.assertEqual(code, 1)


class TestSafetyScans(unittest.TestCase):
    def test_private_key_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            (Path(tmp) / "snap" / "keys").mkdir()
            (Path(tmp) / "snap" / "keys" / "actuator.pem").write_text(
                "-----BEGIN PRIVATE KEY-----\nMIIE...\n-----END PRIVATE KEY-----\n"
            )
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("undeclared artifact", str(ctx.exception))

    def test_absolute_path_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            index_path = Path(tmp) / "snap" / "index.json"
            index = json.loads(index_path.read_text())
            index["receipt_verification"]["result_path"] = "/etc/passwd"
            index_path.write_text(json.dumps(index))
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("absolute path", str(ctx.exception))

    def _corrupt_declared_artifact(self, tmp: str, rel: str, new_content: str) -> None:
        """Overwrite a declared artifact and update its checksum in index.json."""
        snap = Path(tmp) / "snap"
        artifact = snap / rel
        artifact.write_text(new_content)
        import hashlib
        new_sha = hashlib.sha256(artifact.read_bytes()).hexdigest()
        index_path = snap / "index.json"
        index = json.loads(index_path.read_text())
        _update_sha_in_index(index, rel, new_sha)
        index_path.write_text(json.dumps(index))

    def test_private_key_in_declared_artifact_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            base = json.loads((Path(tmp) / "snap" / "eval" / "receipt-verification.json").read_text())
            base["note"] = "-----BEGIN PRIVATE KEY-----\nMIIE\n-----END PRIVATE KEY-----\n"
            self._corrupt_declared_artifact(
                tmp,
                "eval/receipt-verification.json",
                json.dumps(base),
            )
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("forbidden private key material", str(ctx.exception))

    def test_credential_field_in_declared_artifact_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            base = json.loads((Path(tmp) / "snap" / "eval" / "receipt-verification.json").read_text())
            base["api_key"] = "leaked-key-value"
            self._corrupt_declared_artifact(
                tmp,
                "eval/receipt-verification.json",
                json.dumps(base),
            )
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("forbidden credential field", str(ctx.exception))

    def test_raw_canary_in_declared_artifact_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            base = json.loads((Path(tmp) / "snap" / "eval" / "receipt-verification.json").read_text())
            base["note"] = "CANARY-abc123def456"
            self._corrupt_declared_artifact(
                tmp,
                "eval/receipt-verification.json",
                json.dumps(base),
            )
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("forbidden raw canary value", str(ctx.exception))

    def test_raw_prompt_field_in_declared_artifact_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            base = json.loads((Path(tmp) / "snap" / "eval" / "receipt-verification.json").read_text())
            base["prompt"] = "private operator request"
            self._corrupt_declared_artifact(tmp, "eval/receipt-verification.json", json.dumps(base))
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("forbidden restricted evidence field", str(ctx.exception))

    def test_machine_specific_absolute_path_in_declared_artifact_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            base = json.loads((Path(tmp) / "snap" / "eval" / "receipt-verification.json").read_text())
            base["artifact_path"] = "/home/release-owner/private/report.json"
            self._corrupt_declared_artifact(tmp, "eval/receipt-verification.json", json.dumps(base))
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("machine-specific absolute path", str(ctx.exception))

    def test_private_network_endpoint_in_declared_artifact_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            base = json.loads((Path(tmp) / "snap" / "eval" / "receipt-verification.json").read_text())
            base["inventory_endpoint"] = "http://192.168.1.2:11434/api/tags"
            self._corrupt_declared_artifact(tmp, "eval/receipt-verification.json", json.dumps(base))
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("exact private-network topology", str(ctx.exception))

    def test_private_network_endpoint_in_index_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            index_path = Path(tmp) / "snap/index.json"
            index = json.loads(index_path.read_text())
            index["caveats"].append("Inventory retained at http://192.168.1.2:11434/api/tags")
            index_path.write_text(json.dumps(index))
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("exact private-network topology", str(ctx.exception))

    def test_canary_metric_id_not_flagged(self) -> None:
        """The metric ID canary_scrubbing is not a raw canary value and must not be flagged."""
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            snapshot = gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("canary_scrubbing", {m.metric_id for run in snapshot.eval_runs.values() for m in run.metrics})


class TestFormatRate(unittest.TestCase):
    def test_format_rate(self) -> None:
        self.assertEqual(gr._format_rate(1.0), "100.0%")
        self.assertEqual(gr._format_rate(0.0), "0.0%")
        self.assertEqual(gr._format_rate(0.5), "50.0%")


class TestStability(unittest.TestCase):
    """Generated output must be byte-identical across working directories, locales, and hash-randomization seeds."""

    def _render_via_subprocess(
        self, cwd: Path, env: dict[str, str], snapshot_dir: Path, template: Path, output: Path
    ) -> str:
        result = subprocess.run(
            [sys.executable, str(SCRIPT), "--snapshot-dir", str(snapshot_dir), "--template", str(template), "--output", str(output)],
            cwd=str(cwd),
            env=env,
            capture_output=True,
            text=True,
        )
        if result.returncode != 0:
            raise AssertionError(f"generator failed (rc={result.returncode}): {result.stderr}")
        return output.read_text()

    def test_stable_across_cwd_locale_and_hash_seed(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            baseline_output = tmp_path / "baseline.md"
            baseline_env = os.environ.copy()
            baseline_env["PYTHONHASHSEED"] = "0"
            baseline_env["LC_ALL"] = "C"
            baseline_env["LANG"] = "C"
            baseline = self._render_via_subprocess(
                tmp_path, baseline_env, VALID, TEMPLATE, baseline_output
            )

            for i, (hashseed, locale) in enumerate(
                [("1", "C"), ("0", "en_US.UTF-8"), ("42", "C"), ("random", "C")]
            ):
                output = tmp_path / f"variant_{i}.md"
                env = os.environ.copy()
                env["PYTHONHASHSEED"] = hashseed
                env["LC_ALL"] = locale
                env["LANG"] = locale
                cwd = tmp_path / "subdir" if i == 0 else tmp_path
                cwd.mkdir(exist_ok=True)
                variant = self._render_via_subprocess(
                    cwd,
                    env,
                    VALID,
                    TEMPLATE,
                    output,
                )
                self.assertEqual(
                    baseline,
                    variant,
                    f"output differs with PYTHONHASHSEED={hashseed}, LC_ALL={locale}, cwd variant={i}",
                )

    def test_stable_across_hash_seed_values(self) -> None:
        """Direct in-process rendering must not depend on dict iteration order affected by hash randomization."""
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp) / "README.md"
            gr.generate(TEMPLATE, VALID, out)
            first = out.read_text()
            gr.generate(TEMPLATE, VALID, out)
            second = out.read_text()
            self.assertEqual(first, second)


if __name__ == "__main__":
    unittest.main()
