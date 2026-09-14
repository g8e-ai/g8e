#!/usr/bin/env python3

from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path
from typing import Any

from bridge import ReportBridge


class RecordingPublisher:
    def __init__(self, failures: int = 0) -> None:
        self.failures = failures
        self.records: list[dict[str, Any]] = []

    def __call__(self, batch_path: Path) -> None:
        if self.failures:
            self.failures -= 1
            raise RuntimeError("publisher unavailable")
        for line in batch_path.read_text(encoding="utf-8").splitlines():
            envelope = json.loads(line)
            self.records.append({"record_type": envelope["record_type"], **json.loads(envelope["record_bytes"])})


class ReportBridgeTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        self.report = self.root / "reports" / "ifeval_subset-run"
        self.state = self.root / "state"
        self.report.mkdir(parents=True)
        self.manifest = {
            "run_id": "run-test-1",
            "suite_id": "ifeval_subset",
            "created_at": "2026-09-14T18:00:00Z",
            "arms": [{"arm_id": "ensemble_ungoverned"}],
            "role_to_model": {
                "primary": {"model": "gemma4:e4b", "provider": "ollama"},
                "assistant": {"model": "gemma4:e2b", "provider": "ollama"},
                "lite": {"model": "qwen2.5:0.5b", "provider": "ollama"},
            },
        }
        self.write_json("manifest.json", self.manifest)
        self.write_jsonl("tasks.jsonl", [{"task_id": "1001"}, {"task_id": "1019"}])

    def tearDown(self) -> None:
        self.temp.cleanup()

    def write_json(self, name: str, value: dict[str, Any]) -> None:
        (self.report / name).write_text(json.dumps(value), encoding="utf-8")

    def write_jsonl(self, name: str, values: list[dict[str, Any]], final_newline: bool = True) -> None:
        text = "\n".join(json.dumps(value) for value in values)
        if final_newline:
            text += "\n"
        (self.report / name).write_text(text, encoding="utf-8")

    def committed_attempt(self) -> dict[str, Any]:
        return {
            "attempt_id": "run-test-1:1001:ensemble_ungoverned:1",
            "run_id": "run-test-1",
            "task_id": "1001",
            "terminal_status": "completed",
            "started_at": "2026-09-14T18:00:01Z",
            "ended_at": "2026-09-14T18:00:03Z",
        }

    def test_manifest_partial_commit_restart_and_terminal_state(self) -> None:
        publisher = RecordingPublisher()
        bridge = ReportBridge(self.root / "reports", self.state, publisher)
        first_count = bridge.scan()
        self.assertGreaterEqual(first_count, 7)
        self.assertIn("evaluation_started", {record["kind"] for record in publisher.records})
        self.assertTrue(all("192.168" not in json.dumps(record) for record in publisher.records))

        attempt = self.committed_attempt()
        self.write_jsonl("attempts.jsonl", [attempt], final_newline=False)
        self.assertEqual(bridge.scan(), 0)

        self.write_jsonl("attempts.jsonl", [attempt])
        self.write_jsonl("metrics.jsonl", [{
            "attempt_id": attempt["attempt_id"],
            "metric_id": "ifeval_subset_verifier",
            "unit": "boolean",
            "eligible": True,
            "value": 1.0,
            "verification_status": "verified",
        }])
        self.write_jsonl("stages.jsonl", [{
            "attempt_id": attempt["attempt_id"],
            "stage_id": "stage-1",
            "kind": "model_inference",
            "agent_role": "g8e.bound",
            "model": "gemma4:e4b",
        }])
        self.write_jsonl("resource-observations.jsonl", [{
            "attempt_id": attempt["attempt_id"],
            "provider_call_latency_seconds": 0.5,
            "input_tokens": 10,
            "output_tokens": 20,
            "retry_count": 0,
        }])
        committed_count = bridge.scan()
        self.assertGreaterEqual(committed_count, 5)
        assignment = next(record for record in publisher.records if record.get("kind") == "assignment_result")
        self.assertEqual(assignment["schema_version"], "1.1.0")
        self.assertEqual(assignment["variant_id"], "gemma-4-e4b-it")
        self.assertEqual(assignment["scenario_category"], "instruction_adherence")
        self.assertEqual(assignment["evaluation_unit"], "system")
        self.assertTrue(assignment["stack_id"].startswith("stack-"))
        self.assertTrue(assignment["benchmark_observations"]["unavailable_reasons"])
        self.assertEqual(assignment["metric_values"]["pass"]["value"], 1.0)

        before_restart = len(publisher.records)
        restarted = ReportBridge(self.root / "reports", self.state, publisher)
        self.assertEqual(restarted.scan(), 0)
        self.assertEqual(len(publisher.records), before_restart)

        self.write_json("summary.json", {"numerator": 1, "denominator": 1})
        self.assertGreater(restarted.scan(), 0)
        terminal = [record for record in publisher.records if record.get("kind") == "evaluation_completed"]
        self.assertEqual(len(terminal), 1)
        self.assertEqual(terminal[0]["completed"], 1)

    def test_outbox_recovers_in_order_after_publisher_failure(self) -> None:
        failing = RecordingPublisher(failures=1)
        bridge = ReportBridge(self.root / "reports", self.state, failing)
        with self.assertRaisesRegex(RuntimeError, "publisher unavailable"):
            bridge.scan()
        self.assertEqual(len(list((self.state / "outbox").glob("batch-*.jsonl"))), 1)

        recovered = RecordingPublisher()
        restarted = ReportBridge(self.root / "reports", self.state, recovered)
        self.assertEqual(restarted.scan(), 0)
        self.assertEqual(len(list((self.state / "outbox").glob("batch-*.jsonl"))), 0)
        kinds = [record["kind"] for record in recovered.records]
        self.assertEqual(kinds[:2], ["catalog_snapshot", "evaluation_summary"])
        self.assertEqual(kinds.count("evaluation_started"), 1)

    def test_explicit_producer_failure_maps_to_dead_evidence(self) -> None:
        publisher = RecordingPublisher()
        bridge = ReportBridge(self.root / "reports", self.state, publisher, final_status="failed")
        bridge.scan()
        terminal = next(record for record in publisher.records if record.get("kind") == "evaluation_failed")
        self.assertEqual(terminal["lifecycle_status"], "failed")
        self.assertEqual(terminal["quality_state"], "dead_evidence")

    def test_campaign_failure_maps_to_terminal_failure(self) -> None:
        self.write_json("campaign-status.json", {
            "status": "stopped",
            "stop_reason": "non_retryable_failure",
            "updated_at": "2026-09-14T18:00:05Z",
        })
        publisher = RecordingPublisher()
        bridge = ReportBridge(self.root / "reports", self.state, publisher)
        bridge.scan()
        terminal = next(record for record in publisher.records if record.get("kind") == "evaluation_failed")
        self.assertEqual(terminal["lifecycle_status"], "failed")
        self.assertEqual(terminal["quality_state"], "terminal_failed")


if __name__ == "__main__":
    unittest.main()
