# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for the campaign validate and plan subcommands.

Verifies that ``campaign validate`` performs side-effect-free validation
of a campaign profile against a model registry (no provider calls, no
file writes, no network), and that ``campaign plan`` produces a
deterministic dry-run output showing exact run, task, warm-up, measured-
call, disk, and declared remote-cost ceilings. Both subcommands reject
inconsistent inputs (hash mismatches, unknown variant IDs, missing
files) with typed errors.
"""

from __future__ import annotations

import os
from pathlib import Path

import pytest
from click.testing import CliRunner

from g8e_evals import cli
from g8e_evals.cli import main
from g8e_evals.profile import (
    CAMPAIGN_PROFILE_VERSION,
    CampaignLifecycleStatus,
    CampaignProfile,
    ClaimBoundary,
    ModelTierAssignment,
    TrackArmAssignment,
    compute_campaign_profile_hash,
)
from g8e_evals.registry import (
    MODEL_REGISTRY_VERSION,
    ModelRegistry,
    ModelVariant,
    PublicationEligibility,
    WeightClass,
    compute_model_registry_hash,
)
from g8e_evals.schema import CampaignTrack


pytestmark = pytest.mark.unit

_VALID_HASH = "a" * 64


def _make_variant(variant_id: str = "qwen3-8b-q4_0") -> ModelVariant:
    return ModelVariant(
        variant_id=variant_id,
        canonical_display_name="Qwen3-8B",
        source_list_alias="Qwen3-8B",
        hf_repo="Qwen/Qwen3-8B",
        hf_sha="e966e34a21f2cb17a6c3e8bc2209a2faed9268fb",
        retrieval_date="2026-09-09T00:00:00Z",
        license_id="apache-2.0",
        license_text_hash=None,
        gated=False,
        publication_eligibility=PublicationEligibility.ELIGIBLE,
        parameter_count=8030261248,
        parameter_count_display="8.0B",
        architecture="QwenForCausalLM",
        model_type="qwen2",
        dtype="BF16",
        format="safetensors",
        quantization="q4_0",
        base_model="",
        context_length=32768,
        supported_modalities=["text"],
        reasoning_mode="switchable",
        tool_call_support=True,
        chat_template_family="qwen3",
        chat_template_hash=_VALID_HASH,
        tokenizer_digest=_VALID_HASH,
        weight_class=WeightClass.HEAVY_SLM,
        backend_name="ollama",
        backend_version="0.1.48",
        served_model_tag="qwen3:8b",
        artifact_digest=_VALID_HASH,
        artifact_bytes=4_800_000_000,
        tensor_format="gguf",
        hidden_reasoning_tokens=False,
    )


def _write_registry(tmp_path: Path, variants: list[ModelVariant] | None = None) -> Path:
    if variants is None:
        variants = [_make_variant()]
    ch = compute_model_registry_hash("reg-1", "1", variants, [])
    registry = ModelRegistry(
        registry_id="reg-1",
        registry_version="1",
        schema_version=MODEL_REGISTRY_VERSION,
        created_at="2026-09-10T00:00:00Z",
        variants=variants,
        qualification_records=[],
        content_hash=ch,
    )
    path = tmp_path / "models.json"
    path.write_text(registry.model_dump_json(indent=2))
    return path


def _write_profile(
    tmp_path: Path,
    *,
    registry_hash: str | None = None,
    generative_variant_ids: list[str] | None = None,
) -> Path:
    if generative_variant_ids is None:
        generative_variant_ids = ["qwen3-8b-q4_0"]
    if registry_hash is None:
        # Compute the actual registry hash for the default variant
        variant = _make_variant()
        registry_hash = compute_model_registry_hash("reg-1", "1", [variant], [])

    track_assignments = [
        TrackArmAssignment(track=CampaignTrack.DIRECT, arm_id="direct"),
    ]
    tier_assignments = [
        ModelTierAssignment(variant_id=generative_variant_ids[0], target_tier="primary"),
    ]
    ch = compute_campaign_profile_hash(
        campaign_id="generative-campaign-v1",
        campaign_revision="1",
        schema_version=CAMPAIGN_PROFILE_VERSION,
        purpose="46-model generative comparison campaign",
        generative_variant_ids=generative_variant_ids,
        benchmark_ids=["ifeval_subset"],
        dataset_hashes=[_VALID_HASH],
        grader_hashes=[_VALID_HASH],
        track_arm_assignments=track_assignments,
        model_tier_assignments=tier_assignments,
        repetitions=3,
        model_registry_hash=registry_hash,
    )
    profile = CampaignProfile(
        campaign_id="generative-campaign-v1",
        campaign_revision="1",
        schema_version=CAMPAIGN_PROFILE_VERSION,
        purpose="46-model generative comparison campaign",
        created_at="2026-09-10T00:00:00Z",
        lifecycle_status=CampaignLifecycleStatus.DRAFT,
        generative_variant_ids=generative_variant_ids,
        benchmark_ids=["ifeval_subset"],
        dataset_hashes=[_VALID_HASH],
        grader_hashes=[_VALID_HASH],
        prompt_serialization_hash=_VALID_HASH,
        task_ids=["task-1", "task-2", "task-3"],
        repetitions=3,
        track_arm_assignments=track_assignments,
        model_tier_assignments=tier_assignments,
        baseline_tier_mappings={
            "primary": "qwen3:8b",
            "assistant": "granite3.3:8b",
            "lite": "qwen3:0.6b",
        },
        routing_policy="default",
        temperature=0.0,
        top_p=1.0,
        max_tokens=4096,
        seed=42,
        context_limit=32768,
        timeout_seconds=120.0,
        max_retries=1,
        warmup_excluded=True,
        concurrency=1,
        hardware_identity="linux/amd64/rtx-4090",
        environment_stratum="single-machine",
        primary_metrics=["ifeval_subset_verifier"],
        unit_of_analysis="task",
        claim_boundary=ClaimBoundary.DESCRIPTIVE_ONLY,
        model_registry_hash=registry_hash,
        content_hash=ch,
    )
    path = tmp_path / "profile.json"
    path.write_text(profile.model_dump_json(indent=2))
    return path


def _invoke(runner: CliRunner, args: list[str], env: dict[str, str] | None = None):
    """Invoke the CLI with a controlled environment."""
    sterile = {
        k: v
        for k, v in os.environ.items()
        if not k.startswith(("G8E_OPERATOR", "OPERATOR_SESSION", "OPERATOR_ID"))
    }
    if env:
        sterile.update(env)
    return runner.invoke(main, args, env=sterile, catch_exceptions=False)


class TestCampaignValidate:
    def test_validate_succeeds_with_consistent_profile_and_registry(self, tmp_path: Path):
        registry_path = _write_registry(tmp_path)
        profile_path = _write_profile(tmp_path)
        result = _invoke(CliRunner(), [
            "campaign", "validate",
            "--profile", str(profile_path),
            "--models", str(registry_path),
        ])
        assert result.exit_code == 0, result.output
        assert "valid" in result.output.lower()

    def test_validate_rejects_missing_profile(self, tmp_path: Path):
        registry_path = _write_registry(tmp_path)
        result = _invoke(CliRunner(), [
            "campaign", "validate",
            "--profile", str(tmp_path / "nonexistent.json"),
            "--models", str(registry_path),
        ])
        assert result.exit_code != 0

    def test_validate_rejects_missing_models(self, tmp_path: Path):
        profile_path = _write_profile(tmp_path)
        result = _invoke(CliRunner(), [
            "campaign", "validate",
            "--profile", str(profile_path),
            "--models", str(tmp_path / "nonexistent.json"),
        ])
        assert result.exit_code != 0

    def test_validate_rejects_hash_mismatch(self, tmp_path: Path):
        registry_path = _write_registry(tmp_path)
        profile_path = _write_profile(tmp_path, registry_hash="b" * 64)
        result = _invoke(CliRunner(), [
            "campaign", "validate",
            "--profile", str(profile_path),
            "--models", str(registry_path),
        ])
        assert result.exit_code != 0
        assert "mismatch" in result.output.lower()

    def test_validate_rejects_unknown_variant_id(self, tmp_path: Path):
        registry_path = _write_registry(tmp_path)
        profile_path = _write_profile(tmp_path, generative_variant_ids=["nonexistent"])
        result = _invoke(CliRunner(), [
            "campaign", "validate",
            "--profile", str(profile_path),
            "--models", str(registry_path),
        ])
        assert result.exit_code != 0
        assert "not found in registry" in result.output.lower()

    def test_validate_makes_no_provider_calls(self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch):
        """Validate must not start providers, download models, or make network calls."""
        registry_path = _write_registry(tmp_path)
        profile_path = _write_profile(tmp_path)

        def _no_provider(*args, **kwargs):
            raise AssertionError("validate must not call any provider")

        monkeypatch.setattr(cli, "get_llm_provider", _no_provider)
        result = _invoke(CliRunner(), [
            "campaign", "validate",
            "--profile", str(profile_path),
            "--models", str(registry_path),
        ])
        assert result.exit_code == 0, result.output


class TestCampaignPlan:
    def test_plan_succeeds_with_consistent_profile_and_registry(self, tmp_path: Path):
        registry_path = _write_registry(tmp_path)
        profile_path = _write_profile(tmp_path)
        result = _invoke(CliRunner(), [
            "campaign", "plan",
            "--profile", str(profile_path),
            "--models", str(registry_path),
        ])
        assert result.exit_code == 0, result.output
        assert "assignments" in result.output.lower()

    def test_plan_shows_assignment_count(self, tmp_path: Path):
        registry_path = _write_registry(tmp_path)
        profile_path = _write_profile(tmp_path)
        result = _invoke(CliRunner(), [
            "campaign", "plan",
            "--profile", str(profile_path),
            "--models", str(registry_path),
        ])
        assert result.exit_code == 0, result.output
        # 3 tasks x 1 variant x 1 arm x 3 repetitions = 9 assignments
        assert "9" in result.output

    def test_plan_shows_warmup_count(self, tmp_path: Path):
        registry_path = _write_registry(tmp_path)
        profile_path = _write_profile(tmp_path)
        result = _invoke(CliRunner(), [
            "campaign", "plan",
            "--profile", str(profile_path),
            "--models", str(registry_path),
        ])
        assert result.exit_code == 0, result.output
        assert "warm" in result.output.lower()

    def test_plan_shows_measured_call_count(self, tmp_path: Path):
        registry_path = _write_registry(tmp_path)
        profile_path = _write_profile(tmp_path)
        result = _invoke(CliRunner(), [
            "campaign", "plan",
            "--profile", str(profile_path),
            "--models", str(registry_path),
        ])
        assert result.exit_code == 0, result.output
        assert "measured" in result.output.lower()

    def test_plan_shows_task_count(self, tmp_path: Path):
        registry_path = _write_registry(tmp_path)
        profile_path = _write_profile(tmp_path)
        result = _invoke(CliRunner(), [
            "campaign", "plan",
            "--profile", str(profile_path),
            "--models", str(registry_path),
        ])
        assert result.exit_code == 0, result.output
        assert "task" in result.output.lower()

    def test_plan_is_deterministic(self, tmp_path: Path):
        """Plan output must be byte-identical for the same inputs."""
        registry_path = _write_registry(tmp_path)
        profile_path = _write_profile(tmp_path)
        result1 = _invoke(CliRunner(), [
            "campaign", "plan",
            "--profile", str(profile_path),
            "--models", str(registry_path),
        ])
        result2 = _invoke(CliRunner(), [
            "campaign", "plan",
            "--profile", str(profile_path),
            "--models", str(registry_path),
        ])
        assert result1.exit_code == 0
        assert result2.exit_code == 0
        assert result1.output == result2.output

    def test_plan_rejects_inconsistent_profile(self, tmp_path: Path):
        registry_path = _write_registry(tmp_path)
        profile_path = _write_profile(tmp_path, registry_hash="b" * 64)
        result = _invoke(CliRunner(), [
            "campaign", "plan",
            "--profile", str(profile_path),
            "--models", str(registry_path),
        ])
        assert result.exit_code != 0

    def test_plan_makes_no_provider_calls(self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch):
        """Plan must not start providers, download models, or make network calls."""
        registry_path = _write_registry(tmp_path)
        profile_path = _write_profile(tmp_path)

        def _no_provider(*args, **kwargs):
            raise AssertionError("plan must not call any provider")

        monkeypatch.setattr(cli, "get_llm_provider", _no_provider)
        result = _invoke(CliRunner(), [
            "campaign", "plan",
            "--profile", str(profile_path),
            "--models", str(registry_path),
        ])
        assert result.exit_code == 0, result.output


class TestCampaignSubcommandStructure:
    def test_campaign_help_lists_subcommands(self):
        result = _invoke(CliRunner(), ["campaign", "--help"])
        assert result.exit_code == 0
        assert "validate" in result.output
        assert "plan" in result.output

    def test_campaign_run_still_exists(self):
        result = _invoke(CliRunner(), ["campaign", "run", "--help"])
        assert result.exit_code == 0
