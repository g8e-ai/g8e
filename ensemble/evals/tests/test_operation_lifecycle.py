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
from typing import TYPE_CHECKING

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

if TYPE_CHECKING:
    from g8e_evals.analysis.canonical import PreregistrationConfig
    from g8e_evals.campaign_set import CampaignSetPlan
    from g8e_evals.profile import CampaignProfile
    from g8e_evals.registry import ModelRegistry
    from g8e_evals.replacement_rule import ReplacementManifestRule

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
        "authority_identity",
        "authority_binding",
        "task_parity",
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


def test_check_rejects_unsupported_provider(tmp_path: Path) -> None:
    config_path = _write_diagnostic_config(tmp_path)
    from g8e_evals.operation_config import load_operation_config
    config = load_operation_config(config_path)
    config = config.model_copy(update={"provider_endpoint": ProviderEndpointRef(provider="closedai", endpoint_class="remote")})
    write_operation_config(config, config_path)

    with pytest.raises(ValueError, match="unsupported provider"):
        check_operation(config_path, tmp_path)


def test_check_fails_on_insufficient_disk(tmp_path: Path) -> None:
    config_path = _write_diagnostic_config(tmp_path)
    from g8e_evals.operation_config import load_operation_config
    config = load_operation_config(config_path)
    # Require more free disk than any realistic tmp_path filesystem has.
    config = config.model_copy(update={"budget": BudgetCeilings(max_requests=10, max_tokens=1000, max_usd=2.5, min_free_disk_gb=999999)})
    write_operation_config(config, config_path)

    with pytest.raises(ValueError, match="minimum free disk not met"):
        check_operation(config_path, tmp_path)


def test_status_distinguishes_live_and_stale_producer_metadata(tmp_path: Path) -> None:
    from datetime import UTC, datetime

    from g8e_evals.operation_lifecycle import LaunchState, _process_start_epoch

    config_path = _write_diagnostic_config(tmp_path, report_exists=True)
    report_root = tmp_path / "reports" / "diagnostic"
    launch_path = report_root / EVAL_LAUNCH_STATE_JSON
    # Live case: bind started_at to this process's actual start time so
    # the PID-reuse check classifies the producer as live.
    process_start = _process_start_epoch(os.getpid())
    now_iso = datetime.fromtimestamp(process_start, UTC).isoformat() if process_start else datetime.now(UTC).isoformat()
    live_launch = LaunchState(
        operation_id="diagnostic-1",
        revision="rev-1",
        pid=os.getpid(),
        status="running",
        started_at=now_iso,
        updated_at=now_iso,
    )
    launch_path.write_text(live_launch.model_dump_json(indent=2))

    live = operation_status(config_path, tmp_path)
    assert live.process_state == "running"

    stale_launch = LaunchState(
        operation_id="diagnostic-1",
        revision="rev-1",
        pid=99999999,
        status="running",
        started_at="2026-01-01T00:00:00+00:00",
        updated_at="2026-01-01T00:00:00+00:00",
    )
    launch_path.write_text(stale_launch.model_dump_json(indent=2))
    stale = operation_status(config_path, tmp_path)
    assert stale.process_state == "stale"
    assert stale.safe_detail == "producer metadata is stale"


def test_graceful_stop_writes_identity_bound_durable_request(tmp_path: Path) -> None:
    config_path = _write_diagnostic_config(tmp_path, report_exists=True)
    report_root = tmp_path / "reports" / "diagnostic"
    from g8e_evals.operation_config import load_operation_config
    config = load_operation_config(config_path)
    launch_content_hash = hashlib.sha256(b"launch-identity").hexdigest()
    from g8e_evals.operation_lifecycle import LaunchState
    launch = LaunchState(
        operation_id="diagnostic-1",
        revision="rev-1",
        pid=os.getpid(),
        status="running",
        started_at="2026-01-01T00:00:00+00:00",
        updated_at="2026-01-01T00:00:00+00:00",
        content_hash=launch_content_hash,
    )
    (report_root / EVAL_LAUNCH_STATE_JSON).write_text(launch.model_dump_json(indent=2))

    result = stop_operation(config_path, tmp_path, immediate=False)

    assert result.status == "graceful_stop_requested"
    request = json.loads((report_root / EVAL_STOP_REQUEST_JSON).read_text())
    assert request["operation_id"] == "diagnostic-1"
    assert request["revision"] == "rev-1"
    assert request["config_content_hash"] == config.content_hash
    assert request["launch_content_hash"] == launch_content_hash
    assert len(request["content_hash"]) == 64


def _write_launch_with_hash(report_root: Path, *, status: str = "running", pid: int = 99999999, operation_id: str = "diagnostic-1", revision: str = "rev-1") -> str:
    """Write a launch record with a valid content hash and return the hash."""
    from g8e_evals.operation_lifecycle import LaunchState
    launch = LaunchState(
        operation_id=operation_id,
        revision=revision,
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


def test_status_reports_inconsistent_when_launch_content_hash_tampered(tmp_path: Path) -> None:
    config_path = _write_diagnostic_config(tmp_path, report_exists=True)
    report_root = tmp_path / "reports" / "diagnostic"
    _write_launch_with_hash(report_root)
    # Tamper with the launch record after writing it
    launch_path = report_root / EVAL_LAUNCH_STATE_JSON
    record = json.loads(launch_path.read_text())
    record["status"] = "completed"
    launch_path.write_text(json.dumps(record))

    result = operation_status(config_path, tmp_path)
    assert result.status == "inconsistent"
    assert "content hash does not match" in result.safe_detail


def test_status_derives_budget_from_stages_and_metrics_not_progress_fields(tmp_path: Path) -> None:
    from g8e_evals.constants import STAGES_JSONL, METRICS_JSONL, TASKS_JSONL, ATTEMPTS_JSONL
    from g8e_evals.arms import Arm
    from g8e_evals.schema import (
        AttemptRecord,
        MetricObservation,
        StageKind,
        StageObservation,
        TaskDefinition,
        TerminalStatus,
    )

    config_path = _write_diagnostic_config(tmp_path, report_exists=True)
    report_root = tmp_path / "reports" / "diagnostic"
    _write_launch_with_hash(report_root, status="completed", pid=99999999)
    # Write typed stage records: 2 model_inference stages with token usage
    # plus a grading stage that must not count toward provider requests.
    stages = [
        StageObservation(
            stage_id="s1", attempt_id="a1", run_id="r1", kind=StageKind.MODEL_INFERENCE,
            input_tokens=100, output_tokens=50, thinking_tokens=0,
        ),
        StageObservation(
            stage_id="s2", attempt_id="a2", run_id="r1", kind=StageKind.MODEL_INFERENCE,
            input_tokens=200, output_tokens=100, thinking_tokens=10,
        ),
        StageObservation(
            stage_id="s3", attempt_id="a3", run_id="r1", kind=StageKind.GRADING,
            input_tokens=5, output_tokens=0, thinking_tokens=0,
        ),
    ]
    with open(report_root / STAGES_JSONL, "w") as f:
        for s in stages:
            f.write(s.model_dump_json() + "\n")
    metrics = [
        MetricObservation(metric_id="provider_cost_usd", attempt_id="a1", run_id="r1", arm_id=Arm.DIRECT, task_id="0", value=0.15),
        MetricObservation(metric_id="provider_cost_usd", attempt_id="a2", run_id="r1", arm_id=Arm.DIRECT, task_id="0", value=0.25),
        MetricObservation(metric_id="accuracy", attempt_id="a1", run_id="r1", arm_id=Arm.DIRECT, task_id="0", value=0.9),
    ]
    with open(report_root / METRICS_JSONL, "w") as f:
        for m in metrics:
            f.write(m.model_dump_json() + "\n")
    # Write a typed task definition and a completed attempt
    task_def = TaskDefinition(task_id="0", suite_id="tool_selection", suite_version="1.0.0", prompt_hash=hashlib.sha256(b"prompt").hexdigest())
    with open(report_root / TASKS_JSONL, "w") as f:
        f.write(task_def.model_dump_json() + "\n")
    attempt = AttemptRecord(
        attempt_id="a1", run_id="r1", task_id="0", arm_id=Arm.DIRECT,
        terminal_status=TerminalStatus.COMPLETED,
    )
    with open(report_root / ATTEMPTS_JSONL, "w") as f:
        f.write(attempt.model_dump_json() + "\n")

    result = operation_status(config_path, tmp_path)
    assert result.provider_requests == 2  # only model_inference stages
    assert result.tokens == 460  # 100+50 + 200+100+10
    assert abs(result.spent_usd - 0.4) < 1e-9  # 0.15 + 0.25
    assert result.completed_assignments == 1
    assert result.total_assignments == 1


def test_status_reports_verification_state_for_campaign(tmp_path: Path) -> None:
    from g8e_evals.constants import CAMPAIGN_VERIFICATION_REPORT_JSON
    from g8e_evals.index import INDEX_GENERATION_SCHEMA_VERSION, CampaignVerificationReport
    from g8e_evals.operation_config import (
        AuthorityRef,
        BudgetCeilings,
        CampaignConfig,
        EvidenceKeyRef,
        ProviderEndpointRef,
        StopConditions,
        write_operation_config,
    )
    gold_rel = _write_scenario_gold_set(tmp_path)
    gold_path = tmp_path / gold_rel
    key_path = tmp_path / "evidence-key.json"
    key_path.write_text(json.dumps({"version": 1, "key_id": "eval-key-1", "key_b64": "YWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWE="}))
    os.chmod(key_path, 0o600)
    # Authority files need not be valid for operation_status (it only loads
    # the config and resolves the report root); the content hash is the
    # config's own hash, not the authority file hashes.
    prereg_path = tmp_path / "prereg.json"
    prereg_path.write_text("{}")
    profile_path = tmp_path / "profile.json"
    profile_path.write_text("{}")
    registry_path = tmp_path / "registry.json"
    registry_path.write_text("{}")
    report_root = tmp_path / "reports" / "campaign"
    report_root.mkdir(parents=True)
    config = CampaignConfig(
        operation_id="campaign-1",
        revision="rev-1",
        suite="tool_selection",
        seed=42,
        report_root=str(report_root.relative_to(tmp_path)),
        gold_set=AuthorityRef(path=gold_rel, sha256=_sha256(gold_path)),
        evidence_key=EvidenceKeyRef(path=key_path.name, key_id="eval-key-1"),
        provider_endpoint=ProviderEndpointRef(provider="ollama", endpoint_class="local"),
        budget=BudgetCeilings(max_requests=10, max_tokens=1000, max_usd=2.5, min_free_disk_gb=0),
        stop_conditions=StopConditions(idle_timeout_s=180, max_duration_s=600),
        campaign_id="campaign-1",
        release_version="v1",
        preregistration=AuthorityRef(path=prereg_path.name, sha256=_sha256(prereg_path)),
        profile=AuthorityRef(path=profile_path.name, sha256=_sha256(profile_path)),
        model_registry=AuthorityRef(path=registry_path.name, sha256=_sha256(registry_path)),
        cohort_ids=["cohort-1"],
        arms=["direct"],
        repetitions=1,
    )
    config_path = tmp_path / "campaign.json"
    write_operation_config(config, config_path)
    _write_launch_with_hash(report_root, status="completed", pid=99999999, operation_id="campaign-1")
    # No verification report -> not_verified
    result = operation_status(config_path, tmp_path)
    assert result.verification_state == "not_verified"
    # Write a passing typed verification report -> verified, status becomes "verified"
    passing_report = CampaignVerificationReport(
        verification_schema_version=INDEX_GENERATION_SCHEMA_VERSION,
        campaign_id="campaign-1",
        campaign_revision="rev-1",
        ok=True,
        verified_index_generation_hash="0" * 64,
    )
    (report_root / CAMPAIGN_VERIFICATION_REPORT_JSON).write_text(passing_report.model_dump_json(indent=2))
    result = operation_status(config_path, tmp_path)
    assert result.verification_state == "verified"
    assert result.status == "verified"
    # Write a failing typed verification report -> failed
    failing_report = passing_report.model_copy(update={"ok": False})
    (report_root / CAMPAIGN_VERIFICATION_REPORT_JSON).write_text(failing_report.model_dump_json(indent=2))
    result = operation_status(config_path, tmp_path)
    assert result.verification_state == "failed"


def test_check_rejects_authority_parent_symlink(tmp_path: Path) -> None:
    config_path = _write_diagnostic_config(tmp_path)
    # Create a symlink in a parent directory of the gold set
    gold_dir = tmp_path / "gold_sets" / "tool_selection"
    link_dir = tmp_path / "gold_sets" / "link_dir"
    link_dir.symlink_to(gold_dir, target_is_directory=True)
    # Move the gold set into the symlinked directory path and update config
    from g8e_evals.operation_config import load_operation_config
    config = load_operation_config(config_path)
    new_gold_rel = "gold_sets/link_dir/input_data.jsonl"
    config = config.model_copy(update={"gold_set": AuthorityRef(path=new_gold_rel, sha256=config.gold_set.sha256)})
    write_operation_config(config, config_path)

    with pytest.raises(ValueError, match="path component must not be a symlink"):
        check_operation(config_path, tmp_path)


def test_stop_rejects_force_stop_on_reused_pid(tmp_path: Path) -> None:
    config_path = _write_diagnostic_config(tmp_path, report_exists=True)
    report_root = tmp_path / "reports" / "diagnostic"
    # Write a launch record with a PID that is not running
    _write_launch_with_hash(report_root, status="running", pid=99999999)

    with pytest.raises(ValueError, match="cannot force-stop"):
        stop_operation(config_path, tmp_path, immediate=True)
    # The stop request must not be persisted when validation fails
    assert not (report_root / EVAL_STOP_REQUEST_JSON).exists()


# ---------------------------------------------------------------------------
# Cross-authority binding validation (U7.3b)
# ---------------------------------------------------------------------------

_VALID_HASH = "a" * 64
_VARIANT_ID = "qwen3-8b-q4_0"
_COHORT_ID = "cohort-qwen3-8b-q4_0-role-primary"


def _make_registry() -> ModelRegistry:
    from g8e_evals.registry import (
        MODEL_REGISTRY_VERSION,
        ModelRegistry,
        ModelVariant,
        PublicationEligibility,
        WeightClass,
        compute_model_registry_hash,
    )

    variant = ModelVariant(
        variant_id=_VARIANT_ID,
        canonical_display_name="Qwen3 8B",
        source_list_alias="qwen3:8b",
        hf_repo="Qwen/Qwen3-8B",
        hf_sha="a" * 40,
        retrieval_date="2026-09-01",
        license_id="Apache-2.0",
        license_text_hash=_VALID_HASH,
        gated=False,
        publication_eligibility=PublicationEligibility.ELIGIBLE,
        parameter_count=8_000_000_000,
        parameter_count_display="8.0B",
        architecture="QwenForCausalLM",
        model_type="qwen3",
        dtype="BF16",
        format="gguf",
        quantization="q4_0",
        context_length=32768,
        supported_modalities=["text"],
        reasoning_mode="non-reasoning",
        tool_call_support=True,
        chat_template_family="qwen3",
        chat_template_hash="c" * 64,
        tokenizer_digest="d" * 64,
        weight_class=WeightClass.HEAVY_SLM,
        backend_name="ollama",
        backend_version="0.1.48",
        served_model_tag="qwen3:8b",
        artifact_digest="b" * 64,
        artifact_bytes=8_000_000_000,
        tensor_format="gguf",
        hidden_reasoning_tokens=False,
    )
    variants = [variant]
    content_hash = compute_model_registry_hash("registry-v1", "1", variants, [])
    return ModelRegistry(
        registry_id="registry-v1",
        registry_version="1",
        schema_version=MODEL_REGISTRY_VERSION,
        created_at="2026-09-01T00:00:00Z",
        variants=variants,
        qualification_records=[],
        content_hash=content_hash,
    )


def _make_profile(
    registry_hash: str,
    *,
    arm_ids: list[str] | None = None,
    task_ids: list[str] | None = None,
) -> CampaignProfile:
    from g8e_evals.profile import (
        CAMPAIGN_PROFILE_VERSION,
        CampaignLifecycleStatus,
        CampaignProfile,
        ClaimBoundary,
        TrackArmAssignment,
        compute_campaign_profile_hash,
    )
    from g8e_evals.schema import CampaignTrack

    if arm_ids is None:
        arm_ids = ["direct"]
    if task_ids is None:
        task_ids = ["0"]
    track_by_arm = {
        "direct": CampaignTrack.DIRECT,
        "ensemble_ungoverned": CampaignTrack.TIER_FITNESS,
    }
    track_arm_assignments = [
        TrackArmAssignment(track=track_by_arm[a], arm_id=a) for a in arm_ids
    ]
    fields = {
        "campaign_id": "binding-test-campaign",
        "campaign_revision": "1",
        "schema_version": CAMPAIGN_PROFILE_VERSION,
        "purpose": "Cross-authority binding test campaign",
        "created_at": "2026-09-01T00:00:00Z",
        "lifecycle_status": CampaignLifecycleStatus.FROZEN,
        "generative_variant_ids": [_VARIANT_ID],
        "benchmark_ids": ["ifeval_subset"],
        "dataset_hashes": [_VALID_HASH],
        "grader_hashes": ["g" * 64],
        "prompt_serialization_hash": _VALID_HASH,
        "task_ids": task_ids,
        "repetitions": 1,
        "track_arm_assignments": track_arm_assignments,
        "model_tier_assignments": [],
        "baseline_tier_mappings": {},
        "routing_policy": "default",
        "temperature": 0.0,
        "top_p": 1.0,
        "max_tokens": 4096,
        "seed": 42,
        "context_limit": 32768,
        "timeout_seconds": 120.0,
        "max_retries": 1,
        "warmup_excluded": True,
        "concurrency": 1,
        "hardware_identity": "linux/amd64/cpu",
        "environment_stratum": "single-machine",
        "primary_metrics": ["ifeval_subset_verifier"],
        "unit_of_analysis": "task",
        "claim_boundary": ClaimBoundary.DESCRIPTIVE_ONLY,
        "model_registry_hash": registry_hash,
    }
    temp = CampaignProfile.model_construct(**fields, content_hash="0" * 64)
    return CampaignProfile(**fields, content_hash=compute_campaign_profile_hash(temp))


def _make_preregistration(baseline: str = "direct", comparisons: list[str] | None = None) -> PreregistrationConfig:
    from g8e_evals.analysis.canonical import (
        ClaimPolicy,
        ContinuousTestPolicy,
        PreregistrationConfig,
    )

    return PreregistrationConfig(
        config_id="binding-test-prereg",
        config_version="1.0.0",
        baseline_arm_id=baseline,
        comparison_arm_ids=comparisons or [],
        model_cohort_ids=[_COHORT_ID],
        task_assignment_id="task-assignment-v1",
        initial_state_assignment_id="no-initial-state-v1",
        required_replicate_ids=["replicate-1"],
        required_replicate_count=1,
        primary_metric_ids=["ifeval_subset_verifier"],
        continuous_test_policy=ContinuousTestPolicy.PAIRED_T,
        bootstrap_count=10000,
        bootstrap_confidence=0.95,
        bootstrap_seed=0,
        significance_level=0.05,
        claim_policy=ClaimPolicy.DESCRIPTIVE_ONLY,
    )


def _write_campaign_config(
    tmp_path: Path,
    *,
    registry: ModelRegistry,
    profile: CampaignProfile,
    preregistration: PreregistrationConfig,
    campaign_set_plan: CampaignSetPlan | None = None,
    replacement_rule: ReplacementManifestRule | None = None,
) -> Path:
    """Write authority files and a CampaignConfig referencing them."""
    from g8e_evals.operation_config import CampaignConfig

    gold_rel = _write_scenario_gold_set(tmp_path)
    gold_path = tmp_path / gold_rel
    key_path = tmp_path / "evidence-key.json"
    key_path.write_text(json.dumps({"version": 1, "key_id": "eval-key-1", "key_b64": "YWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWE="}))
    os.chmod(key_path, 0o600)

    prereg_path = tmp_path / "prereg.json"
    prereg_path.write_text(preregistration.model_dump_json())
    profile_path = tmp_path / "profile.json"
    profile_path.write_text(profile.model_dump_json())
    registry_path = tmp_path / "registry.json"
    registry_path.write_text(registry.model_dump_json())

    report_root = tmp_path / "reports" / "campaign"
    kwargs: dict[str, object] = {
        "operation_id": "campaign-1",
        "revision": "rev-1",
        "suite": "tool_selection",
        "seed": 42,
        "report_root": str(report_root.relative_to(tmp_path)),
        "gold_set": AuthorityRef(path=gold_rel, sha256=_sha256(gold_path)),
        "evidence_key": EvidenceKeyRef(path=key_path.name, key_id="eval-key-1"),
        "provider_endpoint": ProviderEndpointRef(provider="ollama", endpoint_class="local"),
        "budget": BudgetCeilings(max_requests=10, max_tokens=1000, max_usd=2.5, min_free_disk_gb=0),
        "stop_conditions": StopConditions(idle_timeout_s=180, max_duration_s=600),
        "campaign_id": "campaign-1",
        "release_version": "v1",
        "preregistration": AuthorityRef(path=prereg_path.name, sha256=_sha256(prereg_path)),
        "profile": AuthorityRef(path=profile_path.name, sha256=_sha256(profile_path)),
        "model_registry": AuthorityRef(path=registry_path.name, sha256=_sha256(registry_path)),
        "cohort_ids": [_COHORT_ID],
        "arms": ["direct"],
        "repetitions": 1,
    }
    if campaign_set_plan is not None:
        plan_path = tmp_path / "plan.json"
        plan_path.write_text(campaign_set_plan.model_dump_json())
        kwargs["campaign_set_plan"] = AuthorityRef(path=plan_path.name, sha256=_sha256(plan_path))
    if replacement_rule is not None:
        rule_path = tmp_path / "rule.json"
        rule_path.write_text(replacement_rule.model_dump_json())
        kwargs["replacement_rule"] = AuthorityRef(path=rule_path.name, sha256=_sha256(rule_path))

    config = CampaignConfig.model_validate(kwargs)
    config_path = tmp_path / "campaign.json"
    write_operation_config(config, config_path)
    return config_path


def test_check_campaign_validates_cross_authority_bindings(tmp_path: Path) -> None:
    registry = _make_registry()
    profile = _make_profile(registry.content_hash)
    preregistration = _make_preregistration(baseline="direct")
    config_path = _write_campaign_config(tmp_path, registry=registry, profile=profile, preregistration=preregistration)

    result = check_operation(config_path, tmp_path)

    assert result.ok is True
    check_ids = {check.check_id for check in result.checks}
    assert "authority_binding" in check_ids
    binding = next(check for check in result.checks if check.check_id == "authority_binding")
    assert binding.status == "pass"
    assert "profile binds registry" in binding.safe_detail


def test_check_campaign_rejects_profile_registry_hash_mismatch(tmp_path: Path) -> None:
    registry = _make_registry()
    # Profile declares a registry hash that does not match the loaded registry.
    profile = _make_profile("b" * 64)
    preregistration = _make_preregistration(baseline="direct")
    config_path = _write_campaign_config(tmp_path, registry=registry, profile=profile, preregistration=preregistration)

    with pytest.raises(ValueError, match="model_registry_hash mismatch"):
        check_operation(config_path, tmp_path)


def test_check_campaign_rejects_preregistration_arm_not_in_profile(tmp_path: Path) -> None:
    registry = _make_registry()
    # Profile only declares "direct"; preregistration declares "ensemble_ungoverned".
    profile = _make_profile(registry.content_hash, arm_ids=["direct"])
    preregistration = _make_preregistration(baseline="direct", comparisons=["ensemble_ungoverned"])
    config_path = _write_campaign_config(tmp_path, registry=registry, profile=profile, preregistration=preregistration)

    with pytest.raises(ValueError, match="not present in the profile"):
        check_operation(config_path, tmp_path)


def test_check_campaign_rejects_replacement_rule_plan_binding_mismatch(tmp_path: Path) -> None:
    from _set_plan_fixture import make_campaign_set_plan, make_replacement_rule

    registry = _make_registry()
    profile = _make_profile(registry.content_hash)
    preregistration = _make_preregistration(baseline="direct")
    plan = make_campaign_set_plan()
    # Rule binds a wrong set_plan_hash (not the plan's content_hash).
    rule = make_replacement_rule(plan=plan, set_plan_hash="b" * 64)
    config_path = _write_campaign_config(
        tmp_path,
        registry=registry,
        profile=profile,
        preregistration=preregistration,
        campaign_set_plan=plan,
        replacement_rule=rule,
    )

    with pytest.raises(ValueError, match="set_plan_hash"):
        check_operation(config_path, tmp_path)


def test_plan_campaign_reports_gold_set_task_identities(tmp_path: Path) -> None:
    """Campaign plan must load task identities from the gold set (the
    same source execution uses), not from the profile's declared
    task_ids alone. The profile's task_ids are verified against the
    gold set, not trusted blindly."""
    registry = _make_registry()
    profile = _make_profile(registry.content_hash)
    preregistration = _make_preregistration(baseline="direct")
    config_path = _write_campaign_config(tmp_path, registry=registry, profile=profile, preregistration=preregistration)

    plan = plan_operation(config_path, tmp_path)

    assert plan.operation_kind == "campaign"
    assert plan.task_identities == ["0"]
    assert plan.task_count == 1
    assert len(plan.task_manifest_digest) == 64
    assert plan.assignment_count == 1


def test_plan_campaign_rejects_profile_task_ids_mismatch(tmp_path: Path) -> None:
    """When the profile's declared task_ids do not match the gold-set
    task identities, planning fails closed. This prevents the
    plan/execution task-identity divergence that caused the overnight
    campaign to plan one task set and execute another."""
    registry = _make_registry()
    profile = _make_profile(registry.content_hash, task_ids=["task-1"])
    preregistration = _make_preregistration(baseline="direct")
    config_path = _write_campaign_config(
        tmp_path, registry=registry, profile=profile, preregistration=preregistration
    )

    with pytest.raises(ValueError, match="do not match the campaign profile"):
        plan_operation(config_path, tmp_path)


def test_check_campaign_rejects_task_ids_mismatch(tmp_path: Path) -> None:
    """Preflight must reject a campaign whose profile task_ids do not
    match the gold-set task identities before report-root creation."""
    registry = _make_registry()
    profile = _make_profile(registry.content_hash, task_ids=["task-1"])
    preregistration = _make_preregistration(baseline="direct")
    config_path = _write_campaign_config(
        tmp_path, registry=registry, profile=profile, preregistration=preregistration
    )

    with pytest.raises(ValueError, match="do not match the campaign profile"):
        check_operation(config_path, tmp_path)


def test_plan_campaign_task_manifest_digest_is_stable(tmp_path: Path) -> None:
    """The task manifest digest is deterministic for the same gold set
    and config. Two plans over the same inputs produce the same digest."""
    registry = _make_registry()
    profile = _make_profile(registry.content_hash)
    preregistration = _make_preregistration(baseline="direct")
    config_path = _write_campaign_config(tmp_path, registry=registry, profile=profile, preregistration=preregistration)

    plan1 = plan_operation(config_path, tmp_path)
    plan2 = plan_operation(config_path, tmp_path)

    assert plan1.task_manifest_digest == plan2.task_manifest_digest
    assert len(plan1.task_manifest_digest) == 64


def test_check_campaign_includes_task_parity_check(tmp_path: Path) -> None:
    """The preflight check set includes a task_parity check that
    verifies gold-set task identities match the profile's declared
    task_ids and reports the manifest digest."""
    registry = _make_registry()
    profile = _make_profile(registry.content_hash)
    preregistration = _make_preregistration(baseline="direct")
    config_path = _write_campaign_config(tmp_path, registry=registry, profile=profile, preregistration=preregistration)

    result = check_operation(config_path, tmp_path)

    check_ids = {check.check_id for check in result.checks}
    assert "task_parity" in check_ids
    task_parity = next(check for check in result.checks if check.check_id == "task_parity")
    assert task_parity.status == "pass"
    assert "manifest digest" in task_parity.safe_detail


def test_plan_diagnostic_includes_task_manifest_digest(tmp_path: Path) -> None:
    """The diagnostic plan also carries a task manifest digest binding
    the plan to the exact task content."""
    config_path = _write_diagnostic_config(tmp_path)

    plan = plan_operation(config_path, tmp_path)

    assert len(plan.task_manifest_digest) == 64
    assert plan.task_manifest_digest != ""
