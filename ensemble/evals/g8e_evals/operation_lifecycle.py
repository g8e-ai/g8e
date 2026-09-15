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
import shutil
import signal
from datetime import UTC, datetime
from pathlib import Path

from pydantic import BaseModel, ConfigDict

from g8e_evals.constants import (
    CAMPAIGN_PROGRESS_JSON,
    CAMPAIGN_STATUS_JSON,
    EVAL_LAUNCH_STATE_JSON,
    EVAL_STOP_REQUEST_JSON,
)
from g8e_evals.evidence import load_evidence_encryption_key
from g8e_evals.operation_config import (
    AuthorityRef,
    CampaignConfig,
    DiagnosticConfig,
    OperationConfigBase,
    load_operation_config,
)


class OperationPlan(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    operation_kind: str
    operation_id: str
    revision: str
    selected_models: list[str]
    arms: list[str]
    task_count: int
    repetitions: int
    assignment_count: int
    warmup_calls: int
    maximum_provider_calls: int
    maximum_tokens: int
    maximum_usd: float
    maximum_duration_s: float | None
    minimum_free_disk_gb: float | None
    schedule_identity: str
    stop_conditions: dict[str, float | None]


class OperationCheck(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    check_id: str
    status: str
    safe_detail: str


class OperationCheckResult(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    ok: bool
    operation_kind: str
    operation_id: str
    checks: list[OperationCheck]


def _resolve_owned_path(repository_root: Path, relative_path: str, label: str) -> Path:
    root = repository_root.resolve()
    path = root / relative_path
    if path.is_symlink():
        raise ValueError(f"{label} must not be a symlink: {relative_path}")
    resolved = path.resolve()
    try:
        resolved.relative_to(root)
    except ValueError as exc:
        raise ValueError(f"{label} escapes repository root: {relative_path}") from exc
    if not resolved.is_file():
        raise ValueError(f"{label} file not found: {relative_path}")
    return resolved


def _verify_authority(repository_root: Path, authority: AuthorityRef, label: str) -> Path:
    path = _resolve_owned_path(repository_root, authority.path, label)
    actual = hashlib.sha256(path.read_bytes()).hexdigest()
    if actual != authority.sha256:
        raise ValueError(f"{label} SHA-256 mismatch: expected {authority.sha256}, got {actual}")
    return path


def _authorities(config: OperationConfigBase) -> list[tuple[str, AuthorityRef]]:
    refs = [("gold_set", config.gold_set)]
    if isinstance(config, DiagnosticConfig):
        if config.preregistration is not None:
            refs.append(("preregistration", config.preregistration))
        return refs
    if isinstance(config, CampaignConfig):
        refs.extend([
            ("preregistration", config.preregistration),
            ("profile", config.profile),
            ("model_registry", config.model_registry),
        ])
        for label, ref in (
            ("model_tags", config.model_tags),
            ("campaign_set_plan", config.campaign_set_plan),
            ("replacement_rule", config.replacement_rule),
        ):
            if ref is not None:
                refs.append((label, ref))
    return refs


def _task_count(config: OperationConfigBase, repository_root: Path) -> int:
    if isinstance(config, DiagnosticConfig):
        if config.task_limit is not None:
            return config.task_limit
        from g8e_evals.suites import assert_model_comparison_eligible

        gold_path = _verify_authority(repository_root, config.gold_set, "gold_set")
        tasks = list(assert_model_comparison_eligible(config.suite).loader_factory(gold_path).load())
        return max(0, len(tasks) - config.task_offset)
    if isinstance(config, CampaignConfig):
        from g8e_evals.profile import CampaignProfile

        profile_path = _verify_authority(repository_root, config.profile, "profile")
        profile = CampaignProfile.model_validate_json(profile_path.read_text())
        selected = profile.task_ids[config.task_offset:]
        if config.task_limit is not None:
            selected = selected[:config.task_limit]
        return len(selected)
    raise ValueError(f"unsupported operation config: {type(config).__name__}")


def _campaign_models(config: CampaignConfig, repository_root: Path) -> list[str]:
    from g8e_evals.profile import CampaignProfile
    from g8e_evals.registry import ModelRegistry
    from g8e_evals.runner import derive_cohorts_from_registry

    profile_path = _verify_authority(repository_root, config.profile, "profile")
    registry_path = _verify_authority(repository_root, config.model_registry, "model_registry")
    profile = CampaignProfile.model_validate_json(profile_path.read_text())
    registry = ModelRegistry.model_validate_json(registry_path.read_text())
    profile.validate_against_registry(registry)
    cohorts, variant_by_cohort = derive_cohorts_from_registry(profile, registry)
    available = {cohort.cohort_id for cohort in cohorts}
    selected = set(config.cohort_ids)
    if selected != available:
        raise ValueError(f"campaign cohort IDs must exactly match the derived profile: expected {sorted(available)}, got {sorted(selected)}")
    profile_arms = {assignment.arm_id for assignment in profile.track_arm_assignments}
    if set(config.arms) != profile_arms:
        raise ValueError(f"campaign arms must exactly match the profile: expected {sorted(profile_arms)}, got {sorted(config.arms)}")
    if config.repetitions != profile.repetitions:
        raise ValueError(f"campaign repetitions must match the profile: expected {profile.repetitions}, got {config.repetitions}")
    return [variant_by_cohort[cohort_id] for cohort_id in config.cohort_ids]


def plan_operation(config_path: Path, repository_root: Path) -> OperationPlan:
    config = load_operation_config(config_path)
    task_count = _task_count(config, repository_root)
    if isinstance(config, DiagnosticConfig):
        selected_models = [config.model_variant_id]
        arms = [config.arm]
        repetitions = 1
        warmup_calls = 0
    elif isinstance(config, CampaignConfig):
        selected_models = _campaign_models(config, repository_root)
        arms = config.arms
        repetitions = config.repetitions
        warmup_calls = len(selected_models) * len(arms)
    else:
        raise ValueError(f"unsupported operation config: {type(config).__name__}")
    assignment_count = len(selected_models) * len(arms) * task_count * repetitions
    schedule_payload = {
        "content_hash": config.content_hash,
        "models": selected_models,
        "arms": arms,
        "tasks": task_count,
        "repetitions": repetitions,
        "seed": config.seed,
    }
    schedule_identity = hashlib.sha256(
        json.dumps(schedule_payload, sort_keys=True, separators=(",", ":")).encode()
    ).hexdigest()
    return OperationPlan(
        operation_kind=config.operation_kind.value,
        operation_id=config.operation_id,
        revision=config.revision,
        selected_models=selected_models,
        arms=arms,
        task_count=task_count,
        repetitions=repetitions,
        assignment_count=assignment_count,
        warmup_calls=warmup_calls,
        maximum_provider_calls=config.budget.max_requests,
        maximum_tokens=config.budget.max_tokens,
        maximum_usd=config.budget.max_usd,
        maximum_duration_s=config.stop_conditions.max_duration_s,
        minimum_free_disk_gb=config.budget.min_free_disk_gb,
        schedule_identity=schedule_identity,
        stop_conditions=config.stop_conditions.model_dump(),
    )


def check_operation(config_path: Path, repository_root: Path) -> OperationCheckResult:
    config = load_operation_config(config_path)
    checks = [OperationCheck(check_id="config", status="pass", safe_detail="typed config and content hash valid")]
    for label, authority in _authorities(config):
        _verify_authority(repository_root, authority, label)
    checks.append(OperationCheck(check_id="authorities", status="pass", safe_detail="all authority hashes match"))
    key_path = _resolve_owned_path(repository_root, config.evidence_key.path, "evidence_key")
    key = load_evidence_encryption_key(key_path)
    if key.key_id != config.evidence_key.key_id:
        raise ValueError(f"evidence key ID mismatch: expected {config.evidence_key.key_id}, got {key.key_id}")
    checks.append(OperationCheck(check_id="evidence_key", status="pass", safe_detail="evidence key identity matches"))
    report_root = (repository_root.resolve() / config.report_root).resolve()
    if report_root.exists():
        raise FileExistsError(f"report root already exists: {report_root}")
    checks.append(OperationCheck(check_id="report_root", status="pass", safe_detail="report root is fresh"))
    disk_path = report_root.parent
    while not disk_path.exists() and disk_path != disk_path.parent:
        disk_path = disk_path.parent
    free_gb = shutil.disk_usage(disk_path).free / (1024 ** 3)
    required_gb = config.budget.min_free_disk_gb or 0
    if free_gb < required_gb:
        raise ValueError(f"minimum free disk not met: required {required_gb} GiB, available {free_gb:.2f} GiB")
    checks.append(OperationCheck(check_id="disk", status="pass", safe_detail=f"{free_gb:.2f} GiB free"))
    return OperationCheckResult(ok=True, operation_kind=config.operation_kind.value, operation_id=config.operation_id, checks=checks)


class OperationStatus(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    operation_kind: str
    operation_id: str
    revision: str
    status: str
    process_state: str
    report_root: str
    completed_assignments: int = 0
    total_assignments: int = 0
    provider_requests: int = 0
    tokens: int = 0
    spent_usd: float = 0.0
    publication_state: str = "not_published"
    safe_detail: str = ""


class OperationStopResult(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    operation_kind: str
    operation_id: str
    status: str
    immediate: bool
    request_path: str


class OperationVerifyResult(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    operation_kind: str
    operation_id: str
    ok: bool
    checked_layers: list[str]
    failures: list[str]


def resolve_report_root(config: OperationConfigBase, repository_root: Path) -> Path:
    root = repository_root.resolve()
    report_root = (root / config.report_root).resolve()
    try:
        report_root.relative_to(root)
    except ValueError as exc:
        raise ValueError(f"report root escapes repository root: {config.report_root}") from exc
    return report_root


def _load_json_object(path: Path) -> dict[str, object]:
    if not path.is_file():
        return {}
    value = json.loads(path.read_text())
    if not isinstance(value, dict):
        raise ValueError(f"expected JSON object: {path.name}")
    return value


def _process_is_running(pid: int) -> bool:
    if pid <= 0:
        return False
    try:
        os.kill(pid, 0)
    except (OSError, ValueError):
        return False
    return True


def operation_status(config_path: Path, repository_root: Path) -> OperationStatus:
    config = load_operation_config(config_path)
    report_root = resolve_report_root(config, repository_root)
    if not report_root.is_dir():
        return OperationStatus(
            operation_kind=config.operation_kind.value,
            operation_id=config.operation_id,
            revision=config.revision,
            status="not_started",
            process_state="absent",
            report_root=str(report_root),
        )
    launch = _load_json_object(report_root / EVAL_LAUNCH_STATE_JSON)
    if launch and (launch.get("operation_id") != config.operation_id or launch.get("revision") != config.revision):
        raise ValueError("launch state identity does not match operation config")
    pid = launch.get("pid", 0)
    running = _process_is_running(pid) if isinstance(pid, int) else False
    process_state = "running" if running else ("stale" if launch.get("status") == "running" else "terminal")
    status_record = _load_json_object(report_root / CAMPAIGN_STATUS_JSON)
    progress = _load_json_object(report_root / CAMPAIGN_PROGRESS_JSON)
    status = status_record.get("status") or launch.get("status") or "report_present"
    if not isinstance(status, str):
        raise ValueError("operation status must be a string")
    return OperationStatus(
        operation_kind=config.operation_kind.value,
        operation_id=config.operation_id,
        revision=config.revision,
        status=status,
        process_state=process_state,
        report_root=str(report_root),
        completed_assignments=int(progress.get("completed_assignments", 0)),
        total_assignments=int(progress.get("total_assignments", 0)),
        provider_requests=int(progress.get("provider_requests", 0)),
        tokens=int(progress.get("tokens", 0)),
        spent_usd=float(progress.get("spent_usd", 0.0)),
        publication_state=str(progress.get("publication_state", "not_published")),
        safe_detail="producer metadata is stale" if process_state == "stale" else "",
    )


def stop_operation(config_path: Path, repository_root: Path, *, immediate: bool) -> OperationStopResult:
    config = load_operation_config(config_path)
    report_root = resolve_report_root(config, repository_root)
    if not report_root.is_dir():
        raise ValueError("operation has not started")
    launch = _load_json_object(report_root / EVAL_LAUNCH_STATE_JSON)
    if launch.get("operation_id") != config.operation_id or launch.get("revision") != config.revision:
        raise ValueError("launch state identity does not match operation config")
    request_path = report_root / EVAL_STOP_REQUEST_JSON
    if request_path.exists():
        raise FileExistsError("stop request already exists")
    pid = launch.get("pid")
    if immediate and (not isinstance(pid, int) or not _process_is_running(pid)):
        raise ValueError("cannot force-stop a process that is not running")
    requested_at = datetime.now(UTC).isoformat()
    payload = {
        "operation_id": config.operation_id,
        "revision": config.revision,
        "immediate": immediate,
        "requested_at": requested_at,
    }
    payload["content_hash"] = hashlib.sha256(
        json.dumps(payload, sort_keys=True, separators=(",", ":")).encode()
    ).hexdigest()
    request_path.write_text(json.dumps(payload, sort_keys=True, indent=2) + "\n")
    if immediate:
        os.kill(pid, signal.SIGTERM)
    return OperationStopResult(
        operation_kind=config.operation_kind.value,
        operation_id=config.operation_id,
        status="force_stop_requested" if immediate else "graceful_stop_requested",
        immediate=immediate,
        request_path=str(request_path),
    )


def verify_operation(config_path: Path, repository_root: Path) -> OperationVerifyResult:
    config = load_operation_config(config_path)
    report_root = resolve_report_root(config, repository_root)
    if isinstance(config, CampaignConfig):
        from g8e_evals.campaign_verify import verify_campaign

        verification = verify_campaign(report_root)
        return OperationVerifyResult(
            operation_kind=config.operation_kind.value,
            operation_id=config.operation_id,
            ok=verification.ok,
            checked_layers=list(verification.checked_layers),
            failures=list(verification.failures),
        )
    from g8e_evals.report.validate import validate_standalone_report

    verification = validate_standalone_report(report_root)
    return OperationVerifyResult(
        operation_kind=config.operation_kind.value,
        operation_id=config.operation_id,
        ok=verification.ok,
        checked_layers=["standalone_report"],
        failures=list(verification.failures),
    )
