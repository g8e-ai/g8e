#!/usr/bin/env python3

from __future__ import annotations

import argparse
import hashlib
import json
import os
import subprocess
import sys
import time
from datetime import UTC, datetime
from pathlib import Path
from typing import Any, Callable

from project import scan_for_paths_and_endpoints, scan_for_prohibited_fields

SCHEMA_VERSION = "1.1.0"
SOURCE_REVISION = "live-evaluation"
TERMINAL_KEYS = ("completed", "model_failed", "grader_failed", "invalid_evidence", "stopped")
MODEL_VARIANTS = {
    "gemma4:e4b": "gemma-4-e4b-it",
    "gemma4:e2b": "gemma-4-e2b-it",
    "qwen2.5:0.5b": "qwen-2-5-0-5b-instruct",
}
TERMINAL_STATUS = {
    "completed": "completed",
    "model_failed": "model_failed",
    "timed_out": "model_failed",
    "infrastructure_failed": "model_failed",
    "governance_rejected": "invalid_evidence",
    "human_denied": "invalid_evidence",
    "invalid_evidence": "invalid_evidence",
}
SCENARIO_CATEGORY_BY_SUITE = {
    "final_response": "final_response",
    "recovery": "recovery",
    "routing_delegation": "routing_delegation",
    "security_policy": "security_policy",
    "technical_analysis": "technical_analysis",
    "tool_arguments": "tool_arguments",
    "tool_selection": "tool_selection",
    "verification": "verification",
    "ifeval_subset": "instruction_adherence",
    "ifeval": "instruction_adherence",
}


def utc_now() -> str:
    return datetime.now(UTC).isoformat().replace("+00:00", "Z")


def read_json(path: Path) -> dict[str, Any] | None:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (FileNotFoundError, json.JSONDecodeError, OSError):
        return None
    return value if isinstance(value, dict) else None


def read_jsonl(path: Path) -> list[dict[str, Any]]:
    try:
        content = path.read_bytes()
    except OSError:
        return []
    if content and not content.endswith(b"\n"):
        content = content[: content.rfind(b"\n") + 1]
    records: list[dict[str, Any]] = []
    for raw in content.splitlines():
        try:
            value = json.loads(raw)
        except json.JSONDecodeError:
            continue
        if isinstance(value, dict):
            records.append(value)
    return records


def variant_id(model: str) -> str:
    return MODEL_VARIANTS.get(model, model.replace(":", "-").replace(".", "-"))


def stack_id(model_mapping: dict[str, str]) -> str:
    encoded = json.dumps(model_mapping, sort_keys=True, separators=(",", ":")).encode()
    return f"stack-{hashlib.sha256(encoded).hexdigest()[:16]}"


def event_id(run_id: str, kind: str, identity: str) -> str:
    digest = hashlib.sha256(f"{run_id}:{kind}:{identity}".encode()).hexdigest()[:20]
    return f"evt-{kind}-{digest}"


def record_identity(record_type: str, record: dict[str, Any]) -> str:
    if record_type == "event":
        return f"event:{record['event_id']}"
    payload = json.dumps(record, sort_keys=True, separators=(",", ":"))
    return f"projection:{hashlib.sha256(payload.encode()).hexdigest()}"


def encode_line(record_type: str, record: dict[str, Any]) -> str:
    violations = scan_for_prohibited_fields(record) + scan_for_paths_and_endpoints(record)
    if violations:
        raise ValueError(f"public disclosure violation: {violations}")
    return json.dumps(
        {"record_type": record_type, "record_bytes": json.dumps(record, sort_keys=True, separators=(",", ":"))},
        sort_keys=True,
        separators=(",", ":"),
    )


def atomic_json(path: Path, value: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_suffix(path.suffix + ".tmp")
    temporary.write_text(json.dumps(value, sort_keys=True, indent=2), encoding="utf-8")
    os.replace(temporary, path)


class CommandPublisher:
    def __init__(self, g8e_bin: Path, cwd: Path) -> None:
        self.g8e_bin = g8e_bin
        self.cwd = cwd

    def _high_water_sequence(self) -> int:
        result = subprocess.run(
            [str(self.g8e_bin), "public", "status"],
            cwd=self.cwd,
            check=True,
            capture_output=True,
            text=True,
        )
        status = json.loads(result.stdout)
        high_water = status.get("high_water_sequence")
        if not isinstance(high_water, int) or high_water < 0:
            raise ValueError("public publisher returned an invalid high-water sequence")
        return high_water

    def __call__(self, batch_path: Path) -> None:
        admission_path = batch_path.with_suffix(".admission.json")
        admission = read_json(admission_path)
        if admission is None:
            record_count = len(read_jsonl(batch_path))
            if record_count == 0:
                raise ValueError("bridge batch is empty")
            admission = {
                "batch_sha256": hashlib.sha256(batch_path.read_bytes()).hexdigest(),
                "before_high_water_sequence": self._high_water_sequence(),
                "record_count": record_count,
            }
            atomic_json(admission_path, admission)
        expected_hash = admission.get("batch_sha256")
        before_sequence = admission.get("before_high_water_sequence")
        record_count = admission.get("record_count")
        if (
            expected_hash != hashlib.sha256(batch_path.read_bytes()).hexdigest()
            or not isinstance(before_sequence, int)
            or not isinstance(record_count, int)
            or record_count <= 0
        ):
            raise ValueError("bridge admission checkpoint does not match its batch")
        if self._high_water_sequence() < before_sequence + record_count:
            subprocess.run([str(self.g8e_bin), "public", "publish", str(batch_path)], cwd=self.cwd, check=True)
        subprocess.run([str(self.g8e_bin), "public", "push"], cwd=self.cwd, check=True)
        if self._high_water_sequence() < before_sequence + record_count:
            raise RuntimeError("public publisher did not durably admit the bridge batch")
        admission_path.unlink()


class ReportBridge:
    def __init__(self, report_root: Path, state_dir: Path, publisher: Callable[[Path], None], final_status: str | None = None) -> None:
        self.report_root = report_root.resolve()
        self.state_dir = state_dir.resolve()
        self.outbox = self.state_dir / "outbox"
        self.state_path = self.state_dir / "cursor.json"
        self.publisher = publisher
        self.final_status = final_status
        self.state = read_json(self.state_path) or {"schema_version": SCHEMA_VERSION, "emitted": [], "next_batch": 1}
        self.emitted = set(self.state.get("emitted", []))
        self.outbox.mkdir(parents=True, exist_ok=True)
        self._recover_outbox_identities()

    def _recover_outbox_identities(self) -> None:
        changed = False
        for batch in sorted(self.outbox.glob("batch-*.jsonl")):
            for envelope in read_jsonl(batch):
                try:
                    record = json.loads(envelope["record_bytes"])
                    identity = record_identity(envelope["record_type"], record)
                except (KeyError, TypeError, json.JSONDecodeError):
                    continue
                if identity not in self.emitted:
                    self.emitted.add(identity)
                    changed = True
        if changed:
            self._save_state()

    def _save_state(self) -> None:
        self.state["emitted"] = sorted(self.emitted)
        atomic_json(self.state_path, self.state)

    def flush(self) -> None:
        for batch in sorted(self.outbox.glob("batch-*.jsonl")):
            self.publisher(batch)
            batch.unlink()

    def emit(self, records: list[tuple[str, dict[str, Any]]]) -> int:
        pending: list[tuple[str, dict[str, Any], str]] = []
        for record_type, record in records:
            identity = record_identity(record_type, record)
            if identity not in self.emitted:
                pending.append((record_type, record, identity))
        emitted = 0
        for offset in range(0, len(pending), 100):
            chunk = pending[offset:offset + 100]
            batch_number = int(self.state.get("next_batch", 1))
            batch = self.outbox / f"batch-{batch_number:08d}.jsonl"
            temporary = batch.with_suffix(".jsonl.tmp")
            temporary.write_text("\n".join(encode_line(record_type, record) for record_type, record, _ in chunk) + "\n", encoding="utf-8")
            os.replace(temporary, batch)
            self.emitted.update(identity for _, _, identity in chunk)
            self.state["next_batch"] = batch_number + 1
            self._save_state()
            emitted += len(chunk)
        self.flush()
        return emitted

    def report_dirs(self) -> list[Path]:
        if (self.report_root / "manifest.json").is_file():
            return [self.report_root]
        return sorted(path.parent for path in self.report_root.glob("**/manifest.json") if path.parent.is_dir())

    def scan(self) -> int:
        self.flush()
        return sum(self._scan_report(report_dir) for report_dir in self.report_dirs())

    def _scan_report(self, report_dir: Path) -> int:
        manifest = read_json(report_dir / "manifest.json")
        if manifest is None or not isinstance(manifest.get("run_id"), str):
            return 0
        run_id = manifest["run_id"]
        dataset_id = f"ds-live-{run_id}"
        observed_at = manifest.get("created_at") or utc_now()
        tasks = read_jsonl(report_dir / "tasks.jsonl")
        attempts = read_jsonl(report_dir / "attempts.jsonl")
        stages = read_jsonl(report_dir / "stages.jsonl")
        metrics = read_jsonl(report_dir / "metrics.jsonl")
        resources = read_jsonl(report_dir / "resource-observations.jsonl")
        progress = read_json(report_dir / "campaign-progress.json") or {}
        status = read_json(report_dir / "campaign-status.json") or {}
        total = int(progress.get("total_assignments") or len(tasks) or len(attempts))
        completed = int(progress.get("completed_assignments") or len(attempts))
        role_models = manifest.get("role_to_model", {})
        model_mapping = {
            role: variant_id(info["model"])
            for role, info in role_models.items()
            if role in {"primary", "assistant", "lite"} and isinstance(info, dict) and isinstance(info.get("model"), str)
        }
        stack_identity = stack_id(model_mapping)
        terminal_kind, lifecycle, quality = self._terminal_state(report_dir, status)
        outcome_counts = {key: 0 for key in TERMINAL_KEYS}
        for attempt in attempts:
            mapped = TERMINAL_STATUS.get(str(attempt.get("terminal_status")), "invalid_evidence")
            outcome_counts[mapped] += 1
        failed = completed - outcome_counts["completed"]
        evaluation = {
            "schema_version": SCHEMA_VERSION,
            "kind": "evaluation_summary",
            "dataset_id": dataset_id,
            "quality_state": quality,
            "observed_at": status.get("updated_at") or progress.get("updated_at") or observed_at,
            "source_revision_label": SOURCE_REVISION,
            "run_id": run_id,
            "suite_id": manifest.get("suite_id", "unknown"),
            "arm": ((manifest.get("arms") or [{}])[0]).get("arm_id", "unknown"),
            "evaluation_unit": "system",
            "stack_id": stack_identity,
            "primary_invocation_share": {"unavailable_reason": "role-attributed inference call counts are not committed"},
            "correlated_failure_rate": {"unavailable_reason": "no comparable correlated-failure observations"},
            "benchmark_unavailable_reasons": ["Escalation, tool scorecard, security event, GPU, and correlated-failure observations are not committed by this report."],
            "model_role_mapping": model_mapping,
            "lifecycle_state": lifecycle,
            "assignment_total": total,
            "assignment_completed": completed,
            "assignment_failed": failed,
            "terminal_outcomes": outcome_counts,
            "started_at": observed_at,
            "verifier_state": "not_applicable",
            "headline_metrics": self._headline_metrics(metrics),
        }
        limitations = ["Live values are provisional until the report reaches a terminal state.", "Raw prompts, outputs, identities, endpoints, and private evidence are excluded."]
        catalog = {
            "schema_version": SCHEMA_VERSION,
            "kind": "catalog_snapshot",
            "dataset_id": dataset_id,
            "dataset_kind": "live_run",
            "quality_state": quality,
            "observed_at": observed_at,
            "source_revision_label": SOURCE_REVISION,
            "title": f"Live evaluation {run_id}",
            "description": "Public-safe committed evaluation progress from the local report bridge.",
            "limitations": limitations,
            "model_count": len(model_mapping),
            "evaluated_count": len({value for value in model_mapping.values()}),
            "suite_count": 1,
            "run_count": 1,
            "assignment_count": total,
            "provider_request_count": len([stage for stage in stages if stage.get("kind") == "model_inference"]),
            "provider_token_count": self._resource_total(resources, "input_tokens") + self._resource_total(resources, "output_tokens"),
            "retry_count": self._resource_total(resources, "retry_count"),
            "verifier_passed_count": 0,
            "verifier_failed_count": 0,
            "generated_at": observed_at,
        }
        records: list[tuple[str, dict[str, Any]]] = [("projection", catalog), ("projection", evaluation)]
        for role, info in role_models.items():
            if role not in {"primary", "assistant", "lite"} or not isinstance(info, dict) or not isinstance(info.get("model"), str):
                continue
            model = info["model"]
            records.append(("projection", {
                "schema_version": SCHEMA_VERSION,
                "kind": "model_summary",
                "dataset_id": dataset_id,
                "quality_state": quality,
                "observed_at": observed_at,
                "source_revision_label": SOURCE_REVISION,
                "variant_id": variant_id(model),
                "display_name": model,
                "served_model_tag": model,
                "role": role,
                "backend_provider_class": info.get("provider", "unknown"),
                "inventory_only": False,
                "evaluation_coverage": completed / total if total else 0,
                "unavailable_reasons": ["Terminal model metrics are not available yet."] if lifecycle == "running" else [],
            }))
        records.extend(self._initial_events(run_id, dataset_id, observed_at, completed, total))
        records.extend(self._assignment_records(manifest, run_id, dataset_id, stack_identity, attempts, stages, metrics, resources, total))
        if terminal_kind:
            records.append(("event", self._event(terminal_kind, run_id, dataset_id, terminal_kind, lifecycle, completed, total, evaluation["observed_at"], quality)))
        return self.emit(records)

    def _initial_events(self, run_id: str, dataset_id: str, observed_at: str, completed: int, total: int) -> list[tuple[str, dict[str, Any]]]:
        return [
            ("event", self._event("evaluation_queued", run_id, dataset_id, "manifest", "queued", 0, total, observed_at, "live_in_progress")),
            ("event", self._event("evaluation_started", run_id, dataset_id, "manifest", "running", completed, total, observed_at, "live_in_progress")),
        ]

    def _assignment_records(self, manifest: dict[str, Any], run_id: str, dataset_id: str, stack_identity: str, attempts: list[dict[str, Any]], stages: list[dict[str, Any]], metrics: list[dict[str, Any]], resources: list[dict[str, Any]], total: int) -> list[tuple[str, dict[str, Any]]]:
        records: list[tuple[str, dict[str, Any]]] = []
        metrics_by_attempt: dict[str, list[dict[str, Any]]] = {}
        stages_by_attempt: dict[str, list[dict[str, Any]]] = {}
        resources_by_attempt: dict[str, list[dict[str, Any]]] = {}
        for metric in metrics:
            metrics_by_attempt.setdefault(str(metric.get("attempt_id", "")), []).append(metric)
        for stage in stages:
            stages_by_attempt.setdefault(str(stage.get("attempt_id", "")), []).append(stage)
        for resource in resources:
            resources_by_attempt.setdefault(str(resource.get("attempt_id", "")), []).append(resource)
        role_models = manifest.get("role_to_model", {})
        scenario_category = SCENARIO_CATEGORY_BY_SUITE.get(str(manifest.get("suite_id", "")))
        for index, attempt in enumerate(attempts, 1):
            attempt_id_value = str(attempt.get("attempt_id", ""))
            attempt_stages = stages_by_attempt.get(attempt_id_value, [])
            bound_stage = next((stage for stage in attempt_stages if stage.get("agent_role") == "g8e.bound"), None)
            model = str((bound_stage or {}).get("model") or ((role_models.get("primary") or {}).get("model")) or "unknown")
            role = next((name for name, info in role_models.items() if isinstance(info, dict) and info.get("model") == model and name in {"primary", "assistant", "lite"}), "primary")
            source_status = str(attempt.get("terminal_status", "invalid_evidence"))
            terminal_status = TERMINAL_STATUS.get(source_status, "invalid_evidence")
            assignment_id = attempt_id_value
            observed_at = attempt.get("ended_at") or attempt.get("started_at") or manifest.get("created_at") or utc_now()
            pass_metric = self._pass_metric(metrics_by_attempt.get(attempt_id_value, []))
            metric_values = {"pass": {"value": float(pass_metric["value"])}} if pass_metric else {"pass": {"unavailable_reason": f"attempt ended {source_status}; no graded metric exists"}}
            attempt_resources = resources_by_attempt.get(attempt_id_value, [])
            resource = attempt_resources[0] if attempt_resources else {}
            resource_summary = {
                key: {"value": value}
                for key, value in {
                    "latency_ms": float(resource["provider_call_latency_seconds"]) * 1000 if resource.get("provider_call_latency_seconds") is not None else None,
                    "input_tokens": resource.get("input_tokens"),
                    "output_tokens": resource.get("output_tokens"),
                    "retries": resource.get("retry_count", 0) if resource else None,
                }.items()
                if value is not None
            }
            assignment = {
                "schema_version": SCHEMA_VERSION,
                "kind": "assignment_result",
                "dataset_id": dataset_id,
                "quality_state": "live_in_progress" if terminal_status == "completed" else "terminal_failed",
                "observed_at": observed_at,
                "source_revision_label": SOURCE_REVISION,
                "assignment_id": assignment_id,
                "run_id": run_id,
                "task_id": str(attempt.get("task_id", "unknown")),
                "variant_id": variant_id(model),
                "role": role,
                "repetition": 1,
                "scenario_category": scenario_category,
                "evaluation_unit": "system",
                "stack_id": stack_identity,
                "benchmark_observations": {
                    "timing": {
                        "model_load_ms": {"unavailable_reason": "not committed by this report"},
                        "time_to_first_token_ms": {"unavailable_reason": "not committed by this report"},
                        "generation_ms": {"unavailable_reason": "not observed separately from provider call latency"},
                        "whole_task_ms": {"unavailable_reason": "not committed by this report"},
                    },
                    "gpu": {
                        "vram_peak_bytes": {"unavailable_reason": "remote GPU observation unavailable"},
                        "power_watts": {"unavailable_reason": "remote GPU observation unavailable"},
                    },
                    "unavailable_reasons": ["Escalation, decomposed tool, security, cold-start, GPU, and correlated-failure observations were not committed."],
                },
                "terminal_status": terminal_status,
                "metric_values": metric_values,
                "missingness_reason": None if source_status == "completed" and resource else f"Source terminal status: {source_status}; provider resource observation unavailable.",
                "stage_summary": [],
                "resource_summary": resource_summary or None,
                "verification_disposition": "passed" if pass_metric and pass_metric.get("verification_status") == "verified" else "failed" if pass_metric and pass_metric.get("verification_status") == "failed" else "not_applicable",
            }
            assignment = {key: value for key, value in assignment.items() if value is not None}
            records.append(("projection", assignment))
            for stage in attempt_stages:
                stage_identity = str(stage.get("stage_id") or hashlib.sha256(json.dumps(stage, sort_keys=True).encode()).hexdigest())
                event = self._event("stage_updated", run_id, dataset_id, stage_identity, "running", index - 1, total, observed_at, "live_in_progress")
                event["assignment_id"] = assignment_id
                event["task_id"] = assignment["task_id"]
                event["variant_id"] = assignment["variant_id"]
                event["stage_label"] = str(stage.get("agent_role") or stage.get("kind") or "stage")
                records.append(("event", event))
            kind = "assignment_completed" if terminal_status == "completed" else "assignment_failed"
            event = self._event(kind, run_id, dataset_id, assignment_id, "completed" if terminal_status == "completed" else "failed", index, total, observed_at, assignment["quality_state"])
            event["assignment_id"] = assignment_id
            event["task_id"] = assignment["task_id"]
            event["variant_id"] = assignment["variant_id"]
            records.append(("event", event))
            if pass_metric:
                metric_event = self._event("metric_updated", run_id, dataset_id, assignment_id, "running", index, total, observed_at, "live_in_progress")
                metric_event["assignment_id"] = assignment_id
                metric_event["metric_delta"] = {"pass_rate": {"value": float(pass_metric["value"])}}
                records.append(("event", metric_event))
        return records

    def _event(self, kind: str, run_id: str, dataset_id: str, identity: str, lifecycle: str, completed: int, total: int, observed_at: str, quality: str) -> dict[str, Any]:
        return {
            "schema_version": SCHEMA_VERSION,
            "kind": kind,
            "dataset_id": dataset_id,
            "quality_state": quality,
            "observed_at": observed_at,
            "source_revision_label": SOURCE_REVISION,
            "event_id": event_id(run_id, kind, identity),
            "run_id": run_id,
            "lifecycle_status": lifecycle,
            "completed": completed,
            "total": total,
        }

    def _terminal_state(self, report_dir: Path, status: dict[str, Any]) -> tuple[str | None, str, str]:
        if self.final_status == "completed":
            return "evaluation_completed", "completed", "exploratory_verified"
        if self.final_status == "failed":
            return "evaluation_failed", "failed", "dead_evidence"
        if self.final_status == "stopped":
            return "evaluation_stopped", "stopped", "terminal_failed"
        source_status = status.get("status")
        stop_reason = status.get("stop_reason")
        if source_status in {"completed", "finalized"} and stop_reason in {None, "completed"}:
            return "evaluation_completed", "completed", "exploratory_verified"
        if source_status == "failed_integrity" or stop_reason in {"non_retryable_failure", "invalid_evidence"}:
            return "evaluation_failed", "failed", "dead_evidence" if source_status == "failed_integrity" or stop_reason == "invalid_evidence" else "terminal_failed"
        if source_status == "stopped":
            return "evaluation_stopped", "stopped", "terminal_failed"
        if (report_dir / "summary.json").is_file() or (report_dir / "analysis.json").is_file():
            return "evaluation_completed", "completed", "exploratory_verified"
        return None, "running", "live_in_progress"

    def _pass_metric(self, metrics: list[dict[str, Any]]) -> dict[str, Any] | None:
        candidates = [metric for metric in metrics if metric.get("unit") == "boolean" and metric.get("eligible", True) and isinstance(metric.get("value"), (int, float))]
        preferred = [metric for metric in candidates if "verifier" in str(metric.get("metric_id", ""))]
        return (preferred or candidates or [None])[-1]

    def _headline_metrics(self, metrics: list[dict[str, Any]]) -> dict[str, Any]:
        values = [float(metric["value"]) for metric in metrics if "verifier" in str(metric.get("metric_id", "")) and isinstance(metric.get("value"), (int, float))]
        return {"pass_rate": {"value": sum(values) / len(values)}} if values else {}

    def _resource_total(self, resources: list[dict[str, Any]], key: str) -> int:
        return sum(int(resource.get(key) or 0) for resource in resources)


def default_g8e_bin(project_root: Path) -> Path:
    candidate = project_root / "g8e"
    if candidate.is_file():
        return candidate
    raise FileNotFoundError("g8e binary not found; pass --g8e-bin")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--report-root", type=Path, required=True)
    parser.add_argument("--state-dir", type=Path, default=Path("scripts/.bridge"))
    parser.add_argument("--g8e-bin", type=Path)
    parser.add_argument("--g8e-cwd", type=Path, default=Path(__file__).resolve().parents[4])
    parser.add_argument("--poll-ms", type=int, default=200)
    parser.add_argument("--retry-ms", type=int, default=5000)
    parser.add_argument("--once", action="store_true")
    parser.add_argument("--final-status", choices=("completed", "failed", "stopped"))
    args = parser.parse_args()
    g8e_bin = args.g8e_bin or default_g8e_bin(args.g8e_cwd)
    bridge = ReportBridge(args.report_root, args.state_dir, CommandPublisher(g8e_bin, args.g8e_cwd), args.final_status)
    while True:
        try:
            emitted = bridge.scan()
        except (OSError, subprocess.CalledProcessError) as error:
            if args.once:
                raise
            print(f"Publication unavailable; retained durable outbox for retry: {error}", file=sys.stderr, flush=True)
            time.sleep(max(args.retry_ms, 1000) / 1000)
            continue
        if emitted:
            print(f"Published {emitted} public-safe report record(s)", flush=True)
        if args.once:
            return 0
        time.sleep(max(args.poll_ms, 50) / 1000)


if __name__ == "__main__":
    sys.exit(main())
