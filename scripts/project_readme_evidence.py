#!/usr/bin/env python3

from __future__ import annotations

import argparse
import hashlib
import json
import os
import platform
import shutil
import tempfile
from collections import Counter
from pathlib import Path
from typing import Any

import generate_readme as readme


class ProjectionError(Exception):
    pass


def _load_json(path: Path) -> dict[str, Any]:
    try:
        value = json.loads(path.read_text())
    except (OSError, json.JSONDecodeError) as exc:
        raise ProjectionError(f"cannot read {path.name}: {exc}") from exc
    if not isinstance(value, dict):
        raise ProjectionError(f"{path.name} must contain an object")
    return value


def _load_jsonl(path: Path) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    try:
        lines = path.read_text().splitlines()
    except OSError as exc:
        raise ProjectionError(f"cannot read {path.name}: {exc}") from exc
    for index, line in enumerate(lines):
        if not line.strip():
            continue
        try:
            row = json.loads(line)
        except json.JSONDecodeError as exc:
            raise ProjectionError(f"invalid JSON in {path.name} line {index + 1}: {exc}") from exc
        if not isinstance(row, dict):
            raise ProjectionError(f"{path.name} line {index + 1} must contain an object")
        rows.append(row)
    return rows


def _canonical_json(value: Any) -> str:
    return json.dumps(value, sort_keys=True, separators=(",", ":")) + "\n"


def _canonical_jsonl(rows: list[dict[str, Any]]) -> str:
    return "".join(_canonical_json(row) for row in rows)


def _write(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content)


def _sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def _required_string(value: dict[str, Any], field: str, label: str) -> str:
    result = value.get(field)
    if not isinstance(result, str) or not result:
        raise ProjectionError(f"{label} requires non-empty {field}")
    return result


def _project_manifest(manifest: dict[str, Any]) -> dict[str, Any]:
    role_map = manifest.get("role_to_model")
    if not isinstance(role_map, dict):
        raise ProjectionError("manifest requires role_to_model")
    projected_roles: dict[str, Any] = {}
    for role in readme.STAGE1_CONFIGURED_ROLES:
        identity = role_map.get(role)
        if not isinstance(identity, dict):
            raise ProjectionError(f"manifest requires configured {role} role")
        provider = _required_string(identity, "provider", f"manifest {role} role")
        model = _required_string(identity, "model", f"manifest {role} role")
        if readme._is_fake_identity(provider, model):
            raise ProjectionError(f"manifest {role} role uses a forbidden provider identity")
        projected_roles[role] = {
            "role": role,
            "provider": provider,
            "model": model,
            "endpoint": None,
            "endpoint_class": "self-hosted-lan",
            "seed_support": str(identity.get("seed_support", "unknown")),
        }
    arms = manifest.get("arms")
    if not isinstance(arms, list) or len(arms) != 1 or not isinstance(arms[0], dict) or arms[0].get("arm_id") != readme.STAGE1_ARM:
        raise ProjectionError("manifest must contain only the doctrine arm")
    stack_environment = manifest.get("stack_environment")
    if not isinstance(stack_environment, dict):
        raise ProjectionError("manifest requires stack_environment")
    content_hashes = manifest.get("content_hashes")
    if not isinstance(content_hashes, list):
        raise ProjectionError("manifest requires content_hashes")
    return {
        "schema_version": _required_string(manifest, "schema_version", "manifest"),
        "run_id": _required_string(manifest, "run_id", "manifest"),
        "suite_id": _required_string(manifest, "suite_id", "manifest"),
        "suite_version": _required_string(manifest, "suite_version", "manifest"),
        "created_at": _required_string(manifest, "created_at", "manifest"),
        "orchestrator_version": _required_string(manifest, "orchestrator_version", "manifest"),
        "source_revision": str(manifest.get("source_revision", "")),
        "source_tree_state_hash": str(manifest.get("source_tree_state_hash", "")),
        "content_hashes": content_hashes,
        "arms": arms,
        "role_to_model": projected_roles,
        "sampling": manifest.get("sampling", {}),
        "context_limits": manifest.get("context_limits", {}),
        "stack_environment": {
            "os": str(stack_environment.get("os", platform.system())),
            "arch": str(stack_environment.get("arch", platform.machine())),
            "runtime_version": str(stack_environment.get("runtime_version", "")),
        },
        "redacted_config": {},
    }


def _project_tasks(rows: list[dict[str, Any]]) -> list[dict[str, Any]]:
    projected: list[dict[str, Any]] = []
    for row in rows:
        graders = row.get("graders")
        compatible_arms = row.get("compatible_arms")
        if not isinstance(graders, list) or not isinstance(compatible_arms, list):
            raise ProjectionError("task rows require graders and compatible_arms lists")
        projected.append({
            "schema_version": _required_string(row, "schema_version", "task"),
            "task_id": _required_string(row, "task_id", "task"),
            "suite_id": _required_string(row, "suite_id", "task"),
            "suite_version": _required_string(row, "suite_version", "task"),
            "category": str(row.get("category", "")),
            "prompt_hash": _required_string(row, "prompt_hash", "task"),
            "prompt_length": int(row.get("prompt_length", 0)),
            "compatible_arms": compatible_arms,
            "graders": [
                {
                    "grader_id": _required_string(grader, "grader_id", "task grader"),
                    "grader_version": _required_string(grader, "grader_version", "task grader"),
                    "grader_class": _required_string(grader, "grader_class", "task grader"),
                }
                for grader in graders
                if isinstance(grader, dict)
            ],
        })
    return sorted(projected, key=lambda row: row["task_id"])


def _project_attempts(rows: list[dict[str, Any]]) -> list[dict[str, Any]]:
    return sorted([{
        "schema_version": _required_string(row, "schema_version", "attempt"),
        "run_id": _required_string(row, "run_id", "attempt"),
        "attempt_id": _required_string(row, "attempt_id", "attempt"),
        "task_id": _required_string(row, "task_id", "attempt"),
        "arm_id": _required_string(row, "arm_id", "attempt"),
        "terminal_status": _required_string(row, "terminal_status", "attempt"),
        "assignment_order": int(row.get("assignment_order", 0)),
        "receipt_refs": row.get("receipt_refs", []),
    } for row in rows], key=lambda row: (row["assignment_order"], row["task_id"]))


def _project_metrics(rows: list[dict[str, Any]]) -> list[dict[str, Any]]:
    projected = []
    for row in rows:
        if row.get("metric_id") != "ifeval_subset_verifier":
            continue
        projected.append({
            "schema_version": _required_string(row, "schema_version", "metric"),
            "run_id": _required_string(row, "run_id", "metric"),
            "metric_id": "ifeval_subset_verifier",
            "metric_version": _required_string(row, "metric_version", "metric"),
            "attempt_id": _required_string(row, "attempt_id", "metric"),
            "arm_id": _required_string(row, "arm_id", "metric"),
            "task_id": _required_string(row, "task_id", "metric"),
            "eligible": bool(row.get("eligible")),
            "denominator_contribution": int(row.get("denominator_contribution", 0)),
            "value": row.get("value"),
            "unit": _required_string(row, "unit", "metric"),
            "verification_status": _required_string(row, "verification_status", "metric"),
            "grader_class": _required_string(row, "grader_class", "metric"),
            "evidence_refs": row.get("evidence_refs", []),
        })
    return sorted(projected, key=lambda row: row["task_id"])


def _project_stages(rows: list[dict[str, Any]]) -> list[dict[str, Any]]:
    projected = []
    for row in rows:
        if row.get("kind") != "model_inference":
            continue
        provider = _required_string(row, "provider", "model stage")
        model = _required_string(row, "model", "model stage")
        if readme._is_fake_identity(provider, model):
            raise ProjectionError("model stage uses a forbidden provider identity")
        projected.append({
            "schema_version": _required_string(row, "schema_version", "model stage"),
            "run_id": _required_string(row, "run_id", "model stage"),
            "stage_id": _required_string(row, "stage_id", "model stage"),
            "attempt_id": _required_string(row, "attempt_id", "model stage"),
            "kind": "model_inference",
            "agent_role": str(row.get("agent_role", "")),
            "provider": provider,
            "model": model,
            "decision": row.get("decision") if isinstance(row.get("decision"), str) else None,
            "source": "",
        })
    return sorted(projected, key=lambda row: (row["attempt_id"], row["stage_id"]))


def _project_evidence_index(rows: list[dict[str, Any]], attempt_ids: set[str]) -> list[dict[str, Any]]:
    projected = []
    for row in rows:
        attempt_id = row.get("attempt_id")
        if not isinstance(attempt_id, str) or attempt_id not in attempt_ids:
            continue
        projected.append({
            "schema_version": "1.0.0",
            "artifact_id": _required_string(row, "artifact_id", "evidence index"),
            "attempt_id": attempt_id,
            "evidence_kind": "restricted",
            "media_type": _required_string(row, "media_type", "evidence index"),
            "plaintext_sha256": _required_string(row, "sha256", "evidence index"),
            "byte_length": int(row.get("byte_length", 0)),
            "retained_privately": True,
        })
    return sorted(projected, key=lambda row: row["artifact_id"])


def project(private_report: Path, candidate_dir: Path, release_version: str, eval_cli_version: str, idle_timeout_seconds: int) -> None:
    if candidate_dir.exists():
        raise ProjectionError(f"candidate directory already exists: {candidate_dir}")
    if not candidate_dir.parent.is_dir():
        raise ProjectionError(f"candidate parent directory does not exist: {candidate_dir.parent}")
    manifest = _project_manifest(_load_json(private_report / "manifest.json"))
    if manifest["schema_version"] != readme.STAGE1_EVAL_SCHEMA or manifest["suite_id"] != readme.STAGE1_SUITE:
        raise ProjectionError("private report is not the supported Stage 1 eval profile")
    if eval_cli_version != manifest["orchestrator_version"]:
        raise ProjectionError("eval CLI version does not match private manifest orchestrator version")
    tasks = _project_tasks(_load_jsonl(private_report / "tasks.jsonl"))
    attempts = _project_attempts(_load_jsonl(private_report / "attempts.jsonl"))
    metrics = _project_metrics(_load_jsonl(private_report / "metrics.jsonl"))
    stages = _project_stages(_load_jsonl(private_report / "stages.jsonl"))
    receipts = _load_jsonl(private_report / "receipts.jsonl")
    if receipts:
        raise ProjectionError("Stage 1 answer-only publication requires zero receipts")
    task_ids = {row["task_id"] for row in tasks}
    attempt_ids = {row["attempt_id"] for row in attempts}
    if task_ids != readme.STAGE1_TASK_IDS or len(tasks) != len(readme.STAGE1_TASK_IDS):
        raise ProjectionError("private report does not contain the complete Stage 1 task population")
    if {row["task_id"] for row in attempts} != readme.STAGE1_TASK_IDS or len(attempts) != len(readme.STAGE1_TASK_IDS):
        raise ProjectionError("private report does not contain one attempt per Stage 1 task")
    evidence_index = _project_evidence_index(_load_jsonl(private_report / "evidence-index.jsonl"), attempt_ids)
    outcomes = dict(sorted(Counter(row["terminal_status"] for row in attempts).items()))
    numerator = sum(1 for row in metrics if row["eligible"] and row["value"] == 1.0)
    denominator = sum(row["denominator_contribution"] for row in metrics if row["eligible"])
    ended_at = [_required_string(row, "ended_at", "attempt") for row in _load_jsonl(private_report / "attempts.jsonl")]
    if not ended_at:
        raise ProjectionError("private report has no attempt end times")
    run_id = manifest["run_id"]
    run_rel = f"eval/runs/{run_id}"
    summary = {
        "schema_version": "1.0.0",
        "run_id": run_id,
        "assigned_tasks": len(tasks),
        "terminal_attempts": len(attempts),
        "outcomes": outcomes,
        "metric_id": "ifeval_subset_verifier",
        "metric_version": metrics[0]["metric_version"] if metrics else "",
        "numerator": numerator,
        "denominator": denominator,
        "unit": metrics[0]["unit"] if metrics else "",
        "receipt_count": 0,
    }
    roles = {
        role: {
            "provider": manifest["role_to_model"][role]["provider"],
            "model": manifest["role_to_model"][role]["model"],
            "endpoint_class": manifest["role_to_model"][role]["endpoint_class"],
        }
        for role in readme.STAGE1_CONFIGURED_ROLES
    }
    reproduction = {
        "schema_version": "1.0.0",
        "release_version": release_version,
        "run_id": run_id,
        "eval_schema_version": manifest["schema_version"],
        "eval_cli_version": eval_cli_version,
        "suite_id": manifest["suite_id"],
        "suite_version": manifest["suite_version"],
        "arm_id": readme.STAGE1_ARM,
        "task_ids": sorted(task_ids),
        "repetitions": 1,
        "task_limit": None,
        "idle_timeout_seconds": idle_timeout_seconds,
        "endpoint_class": "self-hosted-lan",
        "roles": roles,
        "environment": {"os": manifest["stack_environment"]["os"], "arch": manifest["stack_environment"]["arch"]},
        "provider_inventory_retained_privately": True,
        "command": {
            "program": "g8e-evals",
            "arguments": [
                "run", "--suite", "ifeval_subset", "--arm", "doctrine", "--provider", "ollama", "--model", "${PRIMARY_MODEL}",
                "--assistant-provider", "ollama", "--assistant-model", "${ASSISTANT_MODEL}", "--lite-provider", "ollama", "--lite-model", "${LITE_MODEL}",
                "--primary-endpoint", "${OLLAMA_ENDPOINT}", "--assistant-endpoint", "${OLLAMA_ENDPOINT}", "--lite-endpoint", "${OLLAMA_ENDPOINT}",
                "--idle-timeout", str(idle_timeout_seconds),
            ],
        },
    }
    receipt_verification = {
        "schema_version": "1.0.0",
        "run_id": run_id,
        "verified_at": max(ended_at),
        "verifier_version": "unavailable-zero-receipts",
        "scope": "canonical receipt signatures and final-persistence attestations",
        "total_receipts": 0,
        "verified_signatures": 0,
        "verified_persistence": 0,
        "failed_signatures": 0,
        "failed_persistence": 0,
        "missing_keys": 0,
        "distinct_signer_key_ids": [],
        "receipt_bound_eligible_attempts": 0,
        "sample_receipt_fingerprints": [],
    }
    with tempfile.TemporaryDirectory(prefix="readme-evidence-", dir=candidate_dir.parent) as temp:
        root = Path(temp) / candidate_dir.name
        run_dir = root / run_rel
        artifacts = {
            "manifest.json": _canonical_json(manifest),
            "tasks.jsonl": _canonical_jsonl(tasks),
            "attempts.jsonl": _canonical_jsonl(attempts),
            "metrics.jsonl": _canonical_jsonl(metrics),
            "receipts.jsonl": "",
            "stages.jsonl": _canonical_jsonl(stages),
            "summary.json": _canonical_json(summary),
            "evidence-index.jsonl": _canonical_jsonl(evidence_index),
            "reproduction-manifest.json": _canonical_json(reproduction),
        }
        for name, content in artifacts.items():
            _write(run_dir / name, content)
        receipt_path = root / "eval/receipt-verification.json"
        _write(receipt_path, _canonical_json(receipt_verification))
        run_ref: dict[str, Any] = {"run_id": run_id}
        for name in artifacts:
            stem = name.removesuffix(".jsonl").removesuffix(".json").replace("-", "_")
            run_ref[f"{stem}_path"] = f"{run_rel}/{name}"
            run_ref[f"{stem}_sha256"] = _sha256(run_dir / name)
        index = {
            "publication_schema_version": "2.0.0",
            "readme_evidence_version": "2.0.0",
            "evidence_cutoff": max(ended_at),
            "platform_version": release_version,
            "eval_runs": [run_ref],
            "receipt_verification": {
                "result_path": "eval/receipt-verification.json",
                "result_sha256": _sha256(receipt_path),
                "scope": receipt_verification["scope"],
            },
            "demo_reports": [],
            "ci_links": [
                {"label": "CI", "url": "https://github.com/g8e-ai/g8e/actions/workflows/build-and-test.yml", "kind": "workflow_status"},
                {"label": "Latest Release", "url": "https://github.com/g8e-ai/g8e/releases", "kind": "release_link"},
            ],
            "claim_labels": ["verified_metric_outcomes", "curated_ifeval_subset", "reproducible_local_verification"],
            "caveats": [
                "Stage 1 covers one complete five-task real-agent diagnostic and does not establish broad model quality or statistical significance.",
                "The answer-only campaign produced zero receipts and supports no receipt, mutation, persistence, state, governance, or compliance claim.",
                "Raw prompts, outputs, exact endpoints, local paths, credentials, and evidence keys remain private.",
            ],
        }
        _write(root / "index.json", _canonical_json(index))
        readme.load_snapshot(root)
        os.replace(root, candidate_dir)


def _comparison_task_rows(run_dir: Path) -> dict[str, tuple[str, str, float]]:
    attempts = {row["task_id"]: row for row in _load_jsonl(run_dir / "attempts.jsonl")}
    metrics = {row["task_id"]: row for row in _load_jsonl(run_dir / "metrics.jsonl") if row.get("metric_id") == "ifeval_subset_verifier"}
    if set(attempts) != readme.STAGE1_TASK_IDS or set(metrics) != readme.STAGE1_TASK_IDS:
        raise ProjectionError("comparison input does not contain the complete five-task population")
    return {task_id: (_required_string(attempts[task_id], "attempt_id", "comparison attempt"), _required_string(attempts[task_id], "terminal_status", "comparison attempt"), float(metrics[task_id]["value"])) for task_id in sorted(readme.STAGE1_TASK_IDS)}


def project_stage2(baseline_candidate: Path, private_report: Path, provenance: dict[str, Any], candidate_dir: Path, release_version: str, eval_cli_version: str, idle_timeout_seconds: int) -> None:
    if candidate_dir.exists():
        raise ProjectionError(f"candidate directory already exists: {candidate_dir}")
    if not candidate_dir.parent.is_dir():
        raise ProjectionError(f"candidate parent directory does not exist: {candidate_dir.parent}")
    baseline_snapshot = readme.load_snapshot(baseline_candidate)
    if baseline_snapshot.manifest.publication_schema_version != "2.0.0" or len(baseline_snapshot.manifest.eval_runs) != 1:
        raise ProjectionError("Stage 2 baseline must be one validated Stage 1 publication")
    campaign_profile = provenance.get("campaign_profile")
    if not isinstance(campaign_profile, dict) or campaign_profile.get("release_version") != release_version:
        raise ProjectionError("Stage 2 provenance campaign profile does not match the release version")
    if campaign_profile.get("idle_timeout_seconds") != idle_timeout_seconds:
        raise ProjectionError("Stage 2 provenance campaign profile does not match the idle timeout")
    with tempfile.TemporaryDirectory(prefix="readme-stage2-", dir=candidate_dir.parent) as temp:
        temporary = Path(temp)
        reproduction_candidate = temporary / "reproduction"
        project(private_report, reproduction_candidate, release_version, eval_cli_version, idle_timeout_seconds)
        reproduction_index = _load_json(reproduction_candidate / "index.json")
        reproduction_ref = reproduction_index["eval_runs"][0]
        reproduction_run_id = reproduction_ref["run_id"]
        baseline_ref = baseline_snapshot.manifest.eval_runs[0]
        if baseline_ref.run_id == reproduction_run_id:
            raise ProjectionError("Stage 2 reproduction requires a new run ID")
        reproduction_manifest_path = reproduction_candidate / reproduction_ref["reproduction_manifest_path"]
        reproduction_manifest = _load_json(reproduction_manifest_path)
        source_tree = provenance.get("source_tree")
        components = provenance.get("components")
        images = provenance.get("images")
        environment = provenance.get("environment")
        if not isinstance(source_tree, dict) or not isinstance(components, list) or not isinstance(images, list) or not isinstance(environment, dict):
            raise ProjectionError("Stage 2 provenance is incomplete")
        reproduction_manifest.update({
            "schema_version": "2.0.0",
            "campaign_profile_sha256": hashlib.sha256(_canonical_json(campaign_profile).encode()).hexdigest(),
            "source_state_sha256": _required_string(source_tree, "state_sha256", "source tree"),
            "component_sha256s": {str(row["name"]): str(row["sha256"]) for row in components},
            "image_sha256s": {str(row["service"]): str(row["sha256"]) for row in images},
            "environment": environment,
        })
        _write(reproduction_manifest_path, _canonical_json(reproduction_manifest))
        reproduction_ref["reproduction_manifest_sha256"] = _sha256(reproduction_manifest_path)

        root = temporary / "candidate"
        baseline_index = _load_json(baseline_candidate / "index.json")
        baseline_ref_raw = dict(baseline_index["eval_runs"][0])
        baseline_ref_raw["maturity"] = "stage1"
        for field, value in baseline_ref_raw.items():
            if not field.endswith("_path") or not isinstance(value, str):
                continue
            source = baseline_candidate / value
            destination = root / value
            destination.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(source, destination)
        reproduction_ref["maturity"] = "stage2"
        for field, value in reproduction_ref.items():
            if not field.endswith("_path") or not isinstance(value, str):
                continue
            source = reproduction_candidate / value
            destination = root / value
            destination.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(source, destination)

        baseline_run_dir = root / Path(baseline_ref_raw["manifest_path"]).parent
        reproduction_run_dir = root / Path(reproduction_ref["manifest_path"]).parent
        baseline_tasks = _comparison_task_rows(baseline_run_dir)
        reproduction_tasks = _comparison_task_rows(reproduction_run_dir)
        comparison = {
            "schema_version": "1.0.0",
            "baseline_run_id": baseline_ref.run_id,
            "baseline_maturity": "stage1",
            "reproduction_run_id": reproduction_run_id,
            "reproduction_maturity": "stage2",
            "same_operator_environment": True,
            "causal_attribution": False,
            "statistical_claim": False,
            "declared_differences": ["release_version", "source_state", "component_digests", "image_digests", "execution_state", "runtime_environment"],
            "tasks": [
                {
                    "task_id": task_id,
                    "baseline_attempt_id": baseline_tasks[task_id][0],
                    "baseline_status": baseline_tasks[task_id][1],
                    "baseline_value": baseline_tasks[task_id][2],
                    "reproduction_attempt_id": reproduction_tasks[task_id][0],
                    "reproduction_status": reproduction_tasks[task_id][1],
                    "reproduction_value": reproduction_tasks[task_id][2],
                }
                for task_id in sorted(readme.STAGE1_TASK_IDS)
            ],
        }
        stage2_dir = root / "stage2"
        campaign_profile_path = stage2_dir / "campaign-profile.json"
        provenance_path = stage2_dir / "provenance.json"
        comparison_path = stage2_dir / "comparison.json"
        _write(campaign_profile_path, _canonical_json(campaign_profile))
        _write(provenance_path, _canonical_json(provenance))
        _write(comparison_path, _canonical_json(comparison))
        receipt_source = reproduction_candidate / reproduction_index["receipt_verification"]["result_path"]
        receipt_destination = root / reproduction_index["receipt_verification"]["result_path"]
        receipt_destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(receipt_source, receipt_destination)
        index = {
            "publication_schema_version": "3.0.0",
            "readme_evidence_version": "3.0.0",
            "evidence_cutoff": reproduction_index["evidence_cutoff"],
            "platform_version": release_version,
            "eval_runs": [baseline_ref_raw, reproduction_ref],
            "stage2": {
                "campaign_profile_path": "stage2/campaign-profile.json",
                "campaign_profile_sha256": _sha256(campaign_profile_path),
                "provenance_path": "stage2/provenance.json",
                "provenance_sha256": _sha256(provenance_path),
                "comparison_path": "stage2/comparison.json",
                "comparison_sha256": _sha256(comparison_path),
            },
            "receipt_verification": reproduction_index["receipt_verification"],
            "demo_reports": [],
            "ci_links": reproduction_index["ci_links"],
            "claim_labels": reproduction_index["claim_labels"],
            "caveats": [
                "The original run remains Stage 1; stronger source, component, image, and environment provenance applies only to the fresh Stage 2 reproduction.",
                "The two complete five-task populations support a deterministic reproduction comparison, not causal attribution, statistical significance, broad model quality, or independent external validation.",
                "Both executions used the same local operator environment; independent execution state does not establish independent hardware, organizational control, trust roots, or audit.",
                "Both answer-only campaigns produced zero receipts and support no receipt, mutation, persistence, state, governance, or compliance claim.",
            ],
        }
        _write(root / "index.json", _canonical_json(index))
        readme.load_snapshot(root)
        os.replace(root, candidate_dir)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("private_report", type=Path)
    parser.add_argument("candidate_dir", type=Path)
    parser.add_argument("--release-version", required=True)
    parser.add_argument("--eval-cli-version", required=True)
    parser.add_argument("--idle-timeout", type=int, default=180)
    parser.add_argument("--baseline-candidate", type=Path)
    parser.add_argument("--provenance", type=Path)
    args = parser.parse_args(argv)
    try:
        if (args.baseline_candidate is None) != (args.provenance is None):
            raise ProjectionError("Stage 2 projection requires both --baseline-candidate and --provenance")
        if args.baseline_candidate is not None and args.provenance is not None:
            project_stage2(args.baseline_candidate, args.private_report, _load_json(args.provenance), args.candidate_dir, args.release_version, args.eval_cli_version, args.idle_timeout)
        else:
            project(args.private_report, args.candidate_dir, args.release_version, args.eval_cli_version, args.idle_timeout)
    except (ProjectionError, readme.ReadmeError) as exc:
        print(f"error: {exc}")
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
