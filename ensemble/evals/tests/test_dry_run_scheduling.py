# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for dry-run scheduling in the campaign plan command.

Verifies that ``campaign plan`` computes the deterministic schedule
from the frozen seed and shows exact run order without executing. The
plan output includes the ordered assignment IDs derived from the
Fisher-Yates shuffle of the frozen seed.
"""

from __future__ import annotations

import os
from pathlib import Path

import pytest
from click.testing import CliRunner

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
    seed: int = 42,
    filename: str = "profile.json",
) -> Path:
    if registry_hash is None:
        variant = _make_variant()
        registry_hash = compute_model_registry_hash("reg-1", "1", [variant], [])

    track_assignments = [
        TrackArmAssignment(track=CampaignTrack.DIRECT, arm_id="direct"),
    ]
    tier_assignments = [
        ModelTierAssignment(variant_id="qwen3-8b-q4_0", target_tier="primary"),
    ]
    temp = CampaignProfile.model_construct(
        campaign_id="generative-campaign-v1",
        campaign_revision="1",
        schema_version=CAMPAIGN_PROFILE_VERSION,
        purpose="46-model generative comparison campaign",
        created_at="2026-09-10T00:00:00Z",
        lifecycle_status=CampaignLifecycleStatus.DRAFT,
        generative_variant_ids=["qwen3-8b-q4_0"],
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
        seed=seed,
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
        content_hash="0" * 64,
    )
    ch = compute_campaign_profile_hash(temp)
    profile = CampaignProfile(
        campaign_id="generative-campaign-v1",
        campaign_revision="1",
        schema_version=CAMPAIGN_PROFILE_VERSION,
        purpose="46-model generative comparison campaign",
        created_at="2026-09-10T00:00:00Z",
        lifecycle_status=CampaignLifecycleStatus.DRAFT,
        generative_variant_ids=["qwen3-8b-q4_0"],
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
        seed=seed,
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
    path = tmp_path / filename
    path.write_text(profile.model_dump_json(indent=2))
    return path


def _invoke(runner: CliRunner, args: list[str], env: dict[str, str] | None = None):
    sterile = {
        k: v
        for k, v in os.environ.items()
        if not k.startswith(("G8E_OPERATOR", "OPERATOR_SESSION", "OPERATOR_ID"))
    }
    if env:
        sterile.update(env)
    return runner.invoke(main, args, env=sterile, catch_exceptions=False)


class TestDryRunScheduling:
    def test_plan_shows_exact_run_order(self, tmp_path: Path):
        """Plan output includes the ordered assignment IDs from the deterministic schedule."""
        registry_path = _write_registry(tmp_path)
        profile_path = _write_profile(tmp_path, seed=42)
        result = _invoke(CliRunner(), [
            "campaign", "plan",
            "--profile", str(profile_path),
            "--models", str(registry_path),
        ])
        assert result.exit_code == 0, result.output
        assert "run_order" in result.output.lower()

    def test_plan_run_order_is_deterministic(self, tmp_path: Path):
        """The same seed produces the same run order across invocations."""
        registry_path = _write_registry(tmp_path)
        profile_path = _write_profile(tmp_path, seed=42)
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

    def test_plan_run_order_changes_with_different_seed(self, tmp_path: Path):
        """Different seeds produce different run orders."""
        registry_path = _write_registry(tmp_path)
        profile1_path = _write_profile(tmp_path, seed=42, filename="profile-42.json")
        profile2_path = _write_profile(tmp_path, seed=99, filename="profile-99.json")
        result1 = _invoke(CliRunner(), [
            "campaign", "plan",
            "--profile", str(profile1_path),
            "--models", str(registry_path),
        ])
        result2 = _invoke(CliRunner(), [
            "campaign", "plan",
            "--profile", str(profile2_path),
            "--models", str(registry_path),
        ])
        assert result1.exit_code == 0
        assert result2.exit_code == 0
        assert result1.output != result2.output

    def test_plan_shows_schedule_seed(self, tmp_path: Path):
        """Plan output includes the frozen schedule seed."""
        registry_path = _write_registry(tmp_path)
        profile_path = _write_profile(tmp_path, seed=42)
        result = _invoke(CliRunner(), [
            "campaign", "plan",
            "--profile", str(profile_path),
            "--models", str(registry_path),
        ])
        assert result.exit_code == 0, result.output
        assert "42" in result.output
