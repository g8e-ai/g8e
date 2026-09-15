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

from pydantic import BaseModel, ConfigDict, ValidationError

from g8e_evals.constants import (
    ATTEMPTS_JSONL,
    CAMPAIGN_PROGRESS_JSON,
    CAMPAIGN_STATUS_JSON,
    CAMPAIGN_VERIFICATION_REPORT_JSON,
    EVAL_LAUNCH_STATE_JSON,
    EVAL_STOP_REQUEST_JSON,
    METRICS_JSONL,
    STAGES_JSONL,
    TASKS_JSONL,
)
from g8e_evals.evidence import load_evidence_encryption_key
from g8e_evals.operation_config import (
    AuthorityRef,
    CampaignConfig,
    DiagnosticConfig,
    OperationConfigBase,
    load_operation_config,
)
from g8e_evals.stop_request import (
    StopRequestIdentity,
    build_stop_request,
)


class OperationPlan(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    operation_kind: str
    operation_id: str
    revision: str
    selected_models: list[str]
    arms: list[str]
    task_count: int
    task_identities: list[str]
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
    # Reject symlinks in any path component, not just the final path.
    # A symlink in a parent directory could redirect the resolution
    # outside the repository root without the final path being a symlink.
    current = root
    for part in Path(relative_path).parts:
        current = current / part
        if current.is_symlink():
            raise ValueError(f"{label} path component must not be a symlink: {current.relative_to(root)}")
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


def _verify_authority_identity(path: Path, label: str) -> tuple[BaseModel | None, str]:
    """Validate the canonical identity of an authority document beyond
    its file hash.

    Typed authority documents are validated with their authoritative
    model, which enforces the schema version, identity fields, and
    content-hash self-consistency declared by that document class.
    Data-file authorities (gold sets, model tags) carry no typed
    identity model; their file hash is the identity, so they are only
    required to parse as JSON or JSONL. Returns the loaded typed
    document (or ``None`` for data-file authorities) and a short detail
    string describing the verified identity. The loaded document is
    returned so callers can validate cross-authority bindings against
    the registry or expected identity each authority declares.
    """
    from g8e_evals.analysis.canonical import PreregistrationConfig
    from g8e_evals.campaign_set import CampaignSetPlan
    from g8e_evals.profile import CampaignProfile
    from g8e_evals.registry import ModelRegistry
    from g8e_evals.replacement_rule import ReplacementManifestRule

    typed_models: dict[str, type[BaseModel]] = {
        "preregistration": PreregistrationConfig,
        "profile": CampaignProfile,
        "model_registry": ModelRegistry,
        "campaign_set_plan": CampaignSetPlan,
        "replacement_rule": ReplacementManifestRule,
    }
    text = path.read_text()
    model = typed_models.get(label)
    if model is not None:
        try:
            document = model.model_validate_json(text)
        except ValidationError as exc:
            raise ValueError(f"{label} authority identity invalid: {exc}") from exc
        identity = (
            getattr(document, "authority_id", None)
            or getattr(document, "config_id", None)
            or getattr(document, "campaign_id", None)
            or getattr(document, "registry_id", None)
            or getattr(document, "set_id", None)
            or getattr(document, "rule_id", None)
            or "untyped"
        )
        schema_version = (
            getattr(document, "schema_version", None)
            or getattr(document, "authority_version", None)
            or getattr(document, "config_version", None)
            or getattr(document, "set_version", None)
            or getattr(document, "rule_version", None)
            or "unversioned"
        )
        return document, f"{label} {identity} (schema {schema_version})"
    try:
        json.loads(text)
    except json.JSONDecodeError:
        # JSONL data files (for example gold sets): every non-empty line
        # must be a JSON value.
        try:
            for line in text.splitlines():
                if line.strip():
                    json.loads(line)
        except json.JSONDecodeError as exc:
            raise ValueError(f"{label} data file is not parseable JSON/JSONL: {exc}") from exc
    return None, f"{label} data file hash-bound"


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


def _selected_diagnostic_tasks(config: DiagnosticConfig, repository_root: Path) -> list[str]:
    """Load the full ordered task population, apply offset and limit to
    the actual ordered task set, reject out-of-range or empty slices, and
    return the exact selected task identities. The count is never
    ceiling-derived from task_limit alone."""
    from g8e_evals.suites import assert_model_comparison_eligible

    gold_path = _verify_authority(repository_root, config.gold_set, "gold_set")
    tasks = list(assert_model_comparison_eligible(config.suite).loader_factory(gold_path).load())
    full_count = len(tasks)
    if config.task_offset > full_count:
        raise ValueError(
            f"task_offset {config.task_offset} exceeds task population {full_count}"
        )
    selected = tasks[config.task_offset:]
    if config.task_limit is not None:
        selected = selected[:config.task_limit]
    identities = [task.id for task in selected]
    if not identities:
        raise ValueError("task selection is empty after offset and limit")
    return identities


def _selected_campaign_tasks(config: CampaignConfig, repository_root: Path) -> list[str]:
    """Load the campaign profile's ordered task_ids, apply offset and
    limit to the actual ordered set, reject out-of-range or empty slices,
    and return the exact selected task identities."""
    from g8e_evals.profile import CampaignProfile

    profile_path = _verify_authority(repository_root, config.profile, "profile")
    profile = CampaignProfile.model_validate_json(profile_path.read_text())
    full_count = len(profile.task_ids)
    if config.task_offset > full_count:
        raise ValueError(
            f"task_offset {config.task_offset} exceeds profile task_ids {full_count}"
        )
    selected = profile.task_ids[config.task_offset:]
    if config.task_limit is not None:
        selected = selected[:config.task_limit]
    if not selected:
        raise ValueError("task selection is empty after offset and limit")
    return list(selected)


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
    if isinstance(config, DiagnosticConfig):
        task_identities = _selected_diagnostic_tasks(config, repository_root)
        selected_models = [config.model_variant_id]
        arms = [config.arm]
        repetitions = 1
        warmup_calls = 0
    elif isinstance(config, CampaignConfig):
        task_identities = _selected_campaign_tasks(config, repository_root)
        selected_models = _campaign_models(config, repository_root)
        arms = config.arms
        repetitions = config.repetitions
        warmup_calls = len(selected_models) * len(arms)
    else:
        raise ValueError(f"unsupported operation config: {type(config).__name__}")
    task_count = len(task_identities)
    assignment_count = len(selected_models) * len(arms) * task_count * repetitions
    schedule_payload = {
        "content_hash": config.content_hash,
        "models": selected_models,
        "arms": arms,
        "task_identities": task_identities,
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
        task_identities=task_identities,
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


def _validate_authority_bindings(
    config: OperationConfigBase,
    documents: dict[str, BaseModel | None],
) -> str:
    """Validate cross-authority bindings the config declares.

    Each typed authority document is already validated in isolation by
    ``_verify_authority_identity`` (schema version, identity fields,
    content-hash self-consistency). This validates the bindings between
    authorities: the profile's declared model-registry hash must match
    the loaded registry, every preregistration arm must be declared by
    the profile's ``track_arm_assignments``, and a bound replacement
    rule must bind the loaded campaign-set plan exactly. Returns a
    short detail string describing the verified bindings. Raises
    ``ValueError`` on the first binding failure so the preflight fails
    closed before report-root creation or engine launch.
    """
    from g8e_evals.analysis.canonical import PreregistrationConfig
    from g8e_evals.campaign_set import CampaignSetPlan
    from g8e_evals.profile import CampaignProfile
    from g8e_evals.registry import ModelRegistry
    from g8e_evals.replacement_rule import ReplacementManifestRule, validate_replacement_rule_plan_binding

    if not isinstance(config, CampaignConfig):
        return "no cross-authority bindings for diagnostic"
    profile = documents.get("profile")
    registry = documents.get("model_registry")
    preregistration = documents.get("preregistration")
    if not isinstance(profile, CampaignProfile) or not isinstance(registry, ModelRegistry):
        raise ValueError("campaign profile and model registry must be typed authorities")
    profile.validate_against_registry(registry)
    if not isinstance(preregistration, PreregistrationConfig):
        raise ValueError("campaign preregistration must be a typed authority")
    declared_arms = {a.arm_id for a in profile.track_arm_assignments}
    prereg_arms = {preregistration.baseline_arm_id, *preregistration.comparison_arm_ids}
    unauthorized = sorted(prereg_arms - declared_arms)
    if unauthorized:
        raise ValueError(
            f"preregistration declares arms {unauthorized} not present in the "
            f"profile's track_arm_assignments {sorted(declared_arms)}; "
            "a profile-bound campaign may only execute declared arms"
        )
    plan = documents.get("campaign_set_plan")
    rule = documents.get("replacement_rule")
    if rule is not None:
        if not isinstance(rule, ReplacementManifestRule):
            raise ValueError("campaign replacement_rule must be a typed authority")
        if not isinstance(plan, CampaignSetPlan):
            raise ValueError(
                "replacement_rule is bound but campaign_set_plan is not a typed authority; "
                "a replacement rule authorizes replacements only under the "
                "campaign-set plan it names"
            )
        failures = validate_replacement_rule_plan_binding(rule, plan)
        if failures:
            raise ValueError("; ".join(failures))
    return "profile binds registry; preregistration arms declared by profile" + (
        "; replacement rule binds campaign-set plan" if rule is not None else ""
    )


def check_operation(config_path: Path, repository_root: Path) -> OperationCheckResult:
    config = load_operation_config(config_path)
    checks = [OperationCheck(check_id="config", status="pass", safe_detail="typed config and content hash valid")]
    identity_details: list[str] = []
    documents: dict[str, BaseModel | None] = {}
    for label, authority in _authorities(config):
        auth_path = _verify_authority(repository_root, authority, label)
        document, detail = _verify_authority_identity(auth_path, label)
        documents[label] = document
        identity_details.append(detail)
    checks.append(OperationCheck(check_id="authorities", status="pass", safe_detail="all authority hashes match"))
    checks.append(OperationCheck(
        check_id="authority_identity",
        status="pass",
        safe_detail="; ".join(identity_details),
    ))
    binding_detail = _validate_authority_bindings(config, documents)
    checks.append(OperationCheck(
        check_id="authority_binding",
        status="pass",
        safe_detail=binding_detail,
    ))
    key_path = _resolve_owned_path(repository_root, config.evidence_key.path, "evidence_key")
    key = load_evidence_encryption_key(key_path)
    if key.key_id != config.evidence_key.key_id:
        raise ValueError(f"evidence key ID mismatch: expected {config.evidence_key.key_id}, got {key.key_id}")
    key_mode = key_path.stat().st_mode & 0o777
    if key_mode != 0o600:
        raise ValueError(f"evidence key permissions must be 0600, got {oct(key_mode)}")
    checks.append(OperationCheck(check_id="evidence_key", status="pass", safe_detail="evidence key identity and permissions match"))
    _SUPPORTED_PROVIDERS = frozenset({"openai", "anthropic", "gemini", "ollama", "llamacpp", "fake"})
    if config.provider_endpoint.provider not in _SUPPORTED_PROVIDERS:
        raise ValueError(
            f"unsupported provider: {config.provider_endpoint.provider}; "
            f"supported: {sorted(_SUPPORTED_PROVIDERS)}"
        )
    checks.append(OperationCheck(
        check_id="endpoint_identity",
        status="pass",
        safe_detail=f"provider {config.provider_endpoint.provider} endpoint_class {config.provider_endpoint.endpoint_class}",
    ))
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
    stop_reason: str = ""
    verification_state: str = "not_verified"
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


def _read_jsonl(path: Path) -> list[dict[str, object]]:
    """Read a JSONL file into a list of parsed objects. Returns an empty
    list when the file is absent. Raises ValueError on malformed lines."""
    if not path.is_file():
        return []
    records: list[dict[str, object]] = []
    for line in path.read_text().splitlines():
        if not line.strip():
            continue
        value = json.loads(line)
        if not isinstance(value, dict):
            raise ValueError(f"expected JSON object in {path.name}: {type(value).__name__}")
        records.append(value)
    return records


def _verify_launch_content_hash(launch: dict[str, object]) -> bool:
    """Verify the launch record's declared content hash matches the
    recomputed hash over canonical JSON excluding content_hash. Returns
    True when consistent (or when no hash is declared). Returns False when
    the declared hash does not match the recomputed hash (tampered)."""
    declared = launch.get("content_hash")
    if not isinstance(declared, str) or len(declared) != 64:
        return True  # No hash to verify; treat as consistent (legacy record)
    payload = {k: v for k, v in launch.items() if k != "content_hash"}
    recomputed = hashlib.sha256(
        json.dumps(payload, sort_keys=True, separators=(",", ":")).encode()
    ).hexdigest()
    return declared == recomputed


def _derive_budget(report_root: Path) -> tuple[int, int, float]:
    """Derive provider request count, token sum, and USD spend from the
    authoritative stage and metric records rather than from nonexistent
    campaign-progress fields.

    Provider requests are the count of model_inference stage records.
    Tokens are the sum of input_tokens + output_tokens + thinking_tokens
    over model_inference stages. USD spend is the sum of provider_cost_usd
    metric values in metrics.jsonl.
    """
    stages = _read_jsonl(report_root / STAGES_JSONL)
    provider_requests = 0
    tokens = 0
    for stage in stages:
        if stage.get("kind") != "model_inference":
            continue
        provider_requests += 1
        for field in ("input_tokens", "output_tokens", "thinking_tokens"):
            value = stage.get(field)
            if isinstance(value, int) and value > 0:
                tokens += value
    spent_usd = 0.0
    for metric in _read_jsonl(report_root / METRICS_JSONL):
        if metric.get("metric_id") != "provider_cost_usd":
            continue
        value = metric.get("value")
        if isinstance(value, (int, float)):
            spent_usd += float(value)
    return provider_requests, tokens, spent_usd


def _verification_state(report_root: Path, is_campaign: bool) -> str:
    """Derive the verification state from the authoritative verification
    report presence and ok flag."""
    if is_campaign:
        report_path = report_root / CAMPAIGN_VERIFICATION_REPORT_JSON
        if not report_path.is_file():
            return "not_verified"
        try:
            report = json.loads(report_path.read_text())
        except (OSError, json.JSONDecodeError):
            return "not_verified"
        if not isinstance(report, dict):
            return "not_verified"
        return "verified" if report.get("ok") is True else "failed"
    # Diagnostic standalone reports do not carry a typed verification
    # report file; verification is run on demand via verify_operation.
    return "not_verified"


def _publication_state(report_root: Path) -> str:
    """Derive the publication state from the presence of the published
    campaign artifacts."""
    from g8e_evals.constants import MODEL_CAMPAIGN_JSON

    if (report_root / MODEL_CAMPAIGN_JSON).is_file():
        return "published"
    return "not_published"


def _process_is_running(pid: int) -> bool:
    if pid <= 0:
        return False
    try:
        os.kill(pid, 0)
    except (OSError, ValueError):
        return False
    return True


def _process_start_epoch(pid: int) -> float | None:
    """Return the process start time as a Unix timestamp on Linux, or None.

    Reads ``/proc/<pid>/stat`` to get the process start time in clock ticks
    since boot and converts it to a Unix timestamp using the boot time from
    ``/proc/stat``. On non-Linux platforms or when the proc filesystem is
    unavailable, returns None so the caller falls back to PID-existence
    checks alone.
    """
    try:
        stat_text = Path(f"/proc/{pid}/stat").read_text()
        # /proc/<pid>/stat: field 22 (1-indexed) is starttime in clock ticks
        # since boot. The comm field (field 2) is in parentheses and may
        # contain spaces, so parse from the last ')' onward.
        comm_end = stat_text.rfind(")")
        fields = stat_text[comm_end + 1:].split()
        starttime_ticks = int(fields[19])  # 0-indexed: field 22 - 2 = 19 after comm
        clk_tck = os.sysconf(os.sysconf_names["SC_CLK_TCK"])
        with open("/proc/stat") as f:
            for line in f:
                if line.startswith("btime "):
                    boot_time = int(line.split()[1])
                    return boot_time + starttime_ticks / clk_tck
    except (OSError, ValueError, IndexError, KeyError):
        return None
    return None


def _process_matches_launch(pid: int, launch: dict[str, object]) -> bool:
    """Check whether the process at ``pid`` is the one recorded in ``launch``.

    Protects against PID reuse: if the process at the PID started at a
    different time than the launch record's ``started_at``, the PID has
    been reused by an unrelated process. On platforms where the process
    start time cannot be determined, falls back to PID-existence alone.
    """
    if not _process_is_running(pid):
        return False
    start_epoch = _process_start_epoch(pid)
    if start_epoch is None:
        return True  # Cannot verify start time; assume running
    started_at = launch.get("started_at")
    if not isinstance(started_at, str):
        return True  # No recorded start time; assume running
    try:
        record_epoch = datetime.fromisoformat(started_at).timestamp()
    except ValueError:
        return True  # Unparseable start time; assume running
    # 5-second tolerance for clock skew and rounding
    return abs(start_epoch - record_epoch) <= 5


def operation_status(config_path: Path, repository_root: Path) -> OperationStatus:
    config = load_operation_config(config_path)
    report_root = resolve_report_root(config, repository_root)
    is_campaign = isinstance(config, CampaignConfig)
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

    # Verify the launch record's declared content hash. A mismatch means
    # the launch state was tampered with after the engine wrote it.
    if launch and not _verify_launch_content_hash(launch):
        return OperationStatus(
            operation_kind=config.operation_kind.value,
            operation_id=config.operation_id,
            revision=config.revision,
            status="inconsistent",
            process_state="unknown",
            report_root=str(report_root),
            safe_detail="launch state content hash does not match the recorded state",
        )

    pid = launch.get("pid", 0)
    running = _process_matches_launch(pid, launch) if isinstance(pid, int) else False
    raw_launch_status = launch.get("status")
    launch_status = raw_launch_status if isinstance(raw_launch_status, str) else ""

    # Reconcile the process state against the recorded launch status.
    if launch_status == "running":
        process_state = "running" if running else "stale"
    elif launch_status:
        process_state = "terminal"
    else:
        process_state = "absent" if not running else "running"

    # Derive the reconciled lifecycle status from the launch record,
    # the campaign status file (campaigns only), and the process state.
    stop_reason = ""
    if is_campaign:
        status_record = _load_json_object(report_root / CAMPAIGN_STATUS_JSON)
        raw_campaign_status = status_record.get("status")
        campaign_status = raw_campaign_status if isinstance(raw_campaign_status, str) else ""
        campaign_stop_reason = status_record.get("stop_reason")
        if isinstance(campaign_stop_reason, str):
            stop_reason = campaign_stop_reason
    else:
        campaign_status = ""

    if launch_status == "running" and not running:
        status = "interrupted"
    elif launch_status in ("completed", "failed", "stopped", "interrupted"):
        status = launch_status
    elif campaign_status:
        status = campaign_status
    elif launch_status == "running" and running:
        status = "running"
    else:
        status = "report_present"

    # Assignment counts: campaigns read the real progress fields; diagnostics
    # derive total from tasks.jsonl and completed from terminal attempts.
    progress = _load_json_object(report_root / CAMPAIGN_PROGRESS_JSON)
    if is_campaign:
        raw_total = progress.get("total_assignments")
        raw_completed = progress.get("completed_assignments")
        total_assignments = raw_total if isinstance(raw_total, int) else 0
        completed_assignments = raw_completed if isinstance(raw_completed, int) else 0
    else:
        task_records = _read_jsonl(report_root / TASKS_JSONL)
        total_assignments = len(task_records)
        attempt_records = _read_jsonl(report_root / ATTEMPTS_JSONL)
        completed_assignments = sum(
            1 for a in attempt_records
            if a.get("terminal_status") in ("completed", "COMPLETED")
        )

    provider_requests, tokens, spent_usd = _derive_budget(report_root)
    verification_state = _verification_state(report_root, is_campaign)
    publication_state = _publication_state(report_root)

    # A completed run that has passed verification is "verified"; a
    # completed run pending publication is "finalized".
    if status == "completed" and verification_state == "verified":
        status = "verified"
    elif status == "completed" and publication_state == "not_published":
        status = "finalized"

    safe_detail = "producer metadata is stale" if process_state == "stale" else ""
    return OperationStatus(
        operation_kind=config.operation_kind.value,
        operation_id=config.operation_id,
        revision=config.revision,
        status=status,
        process_state=process_state,
        report_root=str(report_root),
        completed_assignments=completed_assignments,
        total_assignments=total_assignments,
        provider_requests=provider_requests,
        tokens=tokens,
        spent_usd=spent_usd,
        stop_reason=stop_reason,
        verification_state=verification_state,
        publication_state=publication_state,
        safe_detail=safe_detail,
    )


def stop_operation(config_path: Path, repository_root: Path, *, immediate: bool) -> OperationStopResult:
    config = load_operation_config(config_path)
    report_root = resolve_report_root(config, repository_root)
    if not report_root.is_dir():
        raise ValueError("operation has not started")
    launch = _load_json_object(report_root / EVAL_LAUNCH_STATE_JSON)
    if launch.get("operation_id") != config.operation_id or launch.get("revision") != config.revision:
        raise ValueError("launch state identity does not match operation config")
    launch_content_hash = launch.get("content_hash")
    if not isinstance(launch_content_hash, str) or len(launch_content_hash) != 64:
        raise ValueError("launch state is missing its content hash; cannot bind a stop request")
    request_path = report_root / EVAL_STOP_REQUEST_JSON
    if request_path.exists():
        raise FileExistsError("stop request already exists")
    pid = launch.get("pid")
    # All forced-stop validation must pass before the request is
    # persisted or any signal is sent. A reused or mismatched PID is
    # never signaled.
    kill_pid: int | None = None
    if immediate:
        if not isinstance(pid, int) or not _process_matches_launch(pid, launch):
            raise ValueError("cannot force-stop a process that is not running or has been reused")
        kill_pid = pid
    config_content_hash = config.content_hash or config.compute_content_hash()
    request = build_stop_request(
        StopRequestIdentity(
            operation_id=config.operation_id,
            revision=config.revision,
            config_content_hash=config_content_hash,
            launch_content_hash=launch_content_hash,
        ),
        immediate=immediate,
        requested_at=datetime.now(UTC).isoformat(),
    )
    request_path.write_text(request.model_dump_json(indent=2) + "\n")
    if kill_pid is not None:
        os.kill(kill_pid, signal.SIGTERM)
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
