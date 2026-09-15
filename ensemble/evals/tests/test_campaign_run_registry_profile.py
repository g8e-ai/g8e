# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for cohort derivation from the model registry and
campaign profile.

Verifies that ``derive_cohorts_from_registry`` builds cohorts from the
registry's runnable variants (filtered by the profile's
``generative_variant_ids``), that each cohort's role binding carries the
variant's served model tag and backend name, and that each cohort
carries its candidate variant ID and role as typed fields. Also
verifies that the ``campaign run`` CLI accepts ``--profile`` and
``--models`` options.
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


def _make_variant(variant_id: str = "qwen3-8b-q4_0", served_model_tag: str = "qwen3:8b", hf_repo: str = "Qwen/Qwen3-8B", hf_sha: str = "e966e34a21f2cb17a6c3e8bc2209a2faed9268fb") -> ModelVariant:
    return ModelVariant(
        variant_id=variant_id,
        canonical_display_name="Qwen3-8B",
        source_list_alias="Qwen3-8B",
        hf_repo=hf_repo,
        hf_sha=hf_sha,
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
        served_model_tag=served_model_tag,
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
    tier_assignments: list[ModelTierAssignment] | None = None,
) -> Path:
    if generative_variant_ids is None:
        generative_variant_ids = ["qwen3-8b-q4_0"]
    if registry_hash is None:
        variant = _make_variant()
        registry_hash = compute_model_registry_hash("reg-1", "1", [variant], [])

    track_assignments = [
        TrackArmAssignment(track=CampaignTrack.DIRECT, arm_id="direct"),
    ]
    if tier_assignments is None:
        tier_assignments = [
            ModelTierAssignment(variant_id=generative_variant_ids[0], target_tier="primary"),
        ]
    temp = CampaignProfile.model_construct(
        campaign_id="generative-campaign-v1",
        campaign_revision="1",
        schema_version=CAMPAIGN_PROFILE_VERSION,
        purpose="46-model generative comparison campaign",
        created_at="2026-09-10T00:00:00Z",
        lifecycle_status=CampaignLifecycleStatus.FROZEN,
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
        content_hash="0" * 64,
    )
    ch = compute_campaign_profile_hash(temp)
    profile = CampaignProfile(
        campaign_id="generative-campaign-v1",
        campaign_revision="1",
        schema_version=CAMPAIGN_PROFILE_VERSION,
        purpose="46-model generative comparison campaign",
        created_at="2026-09-10T00:00:00Z",
        lifecycle_status=CampaignLifecycleStatus.FROZEN,
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
    sterile = {
        k: v
        for k, v in os.environ.items()
        if not k.startswith(("G8E_OPERATOR", "OPERATOR_SESSION", "OPERATOR_ID"))
    }
    if env:
        sterile.update(env)
    return runner.invoke(main, args, env=sterile, catch_exceptions=False)


class TestCampaignRunAcceptsProfileAndModels:
    def test_campaign_run_help_shows_profile_option(self):
        result = _invoke(CliRunner(), ["campaign", "run", "--help"])
        assert result.exit_code == 0
        assert "--profile" in result.output

    def test_campaign_run_help_shows_models_option(self):
        result = _invoke(CliRunner(), ["campaign", "run", "--help"])
        assert result.exit_code == 0
        assert "--models" in result.output


class TestDeriveCohortsFromRegistry:
    def test_derives_one_cohort_per_runnable_variant(self, tmp_path: Path):
        from g8e_evals.runner import derive_cohorts_from_registry

        variant = _make_variant()
        registry_path = _write_registry(tmp_path, [variant])
        registry = ModelRegistry.model_validate_json(registry_path.read_text())
        profile_path = _write_profile(tmp_path, registry_hash=registry.content_hash)
        profile = CampaignProfile.model_validate_json(profile_path.read_text())

        cohorts = derive_cohorts_from_registry(profile, registry)
        assert len(cohorts) == 1
        assert cohorts[0].candidate_variant_id == "qwen3-8b-q4_0"

    def test_derives_one_cohort_per_model_role_assignment(self, tmp_path: Path):
        from g8e_evals.index import ModelRole
        from g8e_evals.runner import derive_cohorts_from_registry

        variant = _make_variant()
        registry_path = _write_registry(tmp_path, [variant])
        registry = ModelRegistry.model_validate_json(registry_path.read_text())
        assignments = [
            ModelTierAssignment(variant_id=variant.variant_id, target_tier=role)
            for role in (ModelRole.PRIMARY, ModelRole.ASSISTANT, ModelRole.LITE)
        ]
        profile_path = _write_profile(
            tmp_path,
            registry_hash=registry.content_hash,
            tier_assignments=assignments,
        )
        profile = CampaignProfile.model_validate_json(profile_path.read_text())

        cohorts = derive_cohorts_from_registry(profile, registry)

        assert len(cohorts) == 3
        assert all(
            {binding.role for binding in cohort.role_bindings} == {ModelRole.PRIMARY, ModelRole.ASSISTANT, ModelRole.LITE}
            for cohort in cohorts
        )
        assert {cohort.candidate_role for cohort in cohorts} == {ModelRole.PRIMARY, ModelRole.ASSISTANT, ModelRole.LITE}
        assert {cohort.candidate_variant_id for cohort in cohorts} == {variant.variant_id}
        for cohort in cohorts:
            target_role = cohort.candidate_role
            binding_by_role = {binding.role: binding for binding in cohort.role_bindings}
            assert binding_by_role[target_role].model_id == variant.served_model_tag
        assert {cohort.cohort_id for cohort in cohorts} == {
            f"cohort-{variant.variant_id}-role-primary",
            f"cohort-{variant.variant_id}-role-assistant",
            f"cohort-{variant.variant_id}-role-lite",
        }

    def test_cohort_role_binding_carries_served_model_tag(self, tmp_path: Path):
        from g8e_evals.runner import derive_cohorts_from_registry

        variant = _make_variant(served_model_tag="qwen3:8b")
        registry_path = _write_registry(tmp_path, [variant])
        registry = ModelRegistry.model_validate_json(registry_path.read_text())
        profile_path = _write_profile(tmp_path, registry_hash=registry.content_hash)
        profile = CampaignProfile.model_validate_json(profile_path.read_text())

        cohorts = derive_cohorts_from_registry(profile, registry)
        assert len(cohorts) == 1
        rb = cohorts[0].role_bindings[0]
        assert rb.model_id == "qwen3:8b"
        assert rb.provider == "ollama"

    def test_derives_multiple_cohorts_for_multiple_variants(self, tmp_path: Path):
        from g8e_evals.runner import derive_cohorts_from_registry

        v1 = _make_variant("qwen3-8b-q4_0", "qwen3:8b")
        v2 = _make_variant("granite-3.3-8b-q4_0", "granite3.3:8b", "ibm-granite/granite-3.3-8b", "f1d2a3c4b5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0")
        registry_path = _write_registry(tmp_path, [v1, v2])
        registry = ModelRegistry.model_validate_json(registry_path.read_text())
        profile_path = _write_profile(
            tmp_path,
            registry_hash=registry.content_hash,
            generative_variant_ids=["qwen3-8b-q4_0", "granite-3.3-8b-q4_0"],
        )
        profile = CampaignProfile.model_validate_json(profile_path.read_text())

        cohorts = derive_cohorts_from_registry(profile, registry)
        assert len(cohorts) == 2
        variant_ids = sorted(cohort.candidate_variant_id for cohort in cohorts)
        assert variant_ids == ["granite-3.3-8b-q4_0", "qwen3-8b-q4_0"]

    def test_excludes_non_runnable_variants(self, tmp_path: Path):
        from g8e_evals.registry import QualificationOutcome, QualificationRecord
        from g8e_evals.runner import derive_cohorts_from_registry

        v1 = _make_variant("qwen3-8b-q4_0", "qwen3:8b")
        v2 = _make_variant("granite-3.3-8b-q4_0", "granite3.3:8b", "ibm-granite/granite-3.3-8b", "f1d2a3c4b5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0")
        qual_record = QualificationRecord(
            variant_id="granite-3.3-8b-q4_0",
            outcome=QualificationOutcome.OUT_OF_MEMORY,
            reason="Does not fit available hardware",
            evidence_hash=_VALID_HASH,
        )
        ch = compute_model_registry_hash("reg-1", "1", [v1, v2], [qual_record])
        registry = ModelRegistry(
            registry_id="reg-1",
            registry_version="1",
            schema_version=MODEL_REGISTRY_VERSION,
            created_at="2026-09-10T00:00:00Z",
            variants=[v1, v2],
            qualification_records=[qual_record],
            content_hash=ch,
        )
        profile_path = _write_profile(
            tmp_path,
            registry_hash=registry.content_hash,
            generative_variant_ids=["qwen3-8b-q4_0", "granite-3.3-8b-q4_0"],
        )
        profile = CampaignProfile.model_validate_json(profile_path.read_text())

        cohorts = derive_cohorts_from_registry(profile, registry)
        assert len(cohorts) == 1
        assert cohorts[0].candidate_variant_id == "qwen3-8b-q4_0"

    def test_cohort_id_is_derived_from_variant_id(self, tmp_path: Path):
        from g8e_evals.runner import derive_cohorts_from_registry

        variant = _make_variant("qwen3-8b-q4_0", "qwen3:8b")
        registry_path = _write_registry(tmp_path, [variant])
        registry = ModelRegistry.model_validate_json(registry_path.read_text())
        profile_path = _write_profile(tmp_path, registry_hash=registry.content_hash)
        profile = CampaignProfile.model_validate_json(profile_path.read_text())

        cohorts = derive_cohorts_from_registry(profile, registry)
        assert cohorts[0].cohort_id == "cohort-qwen3-8b-q4_0-role-primary"

    def test_cohort_content_hash_is_computed(self, tmp_path: Path):
        from g8e_evals.runner import derive_cohorts_from_registry

        variant = _make_variant("qwen3-8b-q4_0", "qwen3:8b")
        registry_path = _write_registry(tmp_path, [variant])
        registry = ModelRegistry.model_validate_json(registry_path.read_text())
        profile_path = _write_profile(tmp_path, registry_hash=registry.content_hash)
        profile = CampaignProfile.model_validate_json(profile_path.read_text())

        cohorts = derive_cohorts_from_registry(profile, registry)
        assert len(cohorts[0].content_hash) == 64

    def test_sampling_settings_from_profile(self, tmp_path: Path):
        from g8e_evals.runner import derive_cohorts_from_registry

        variant = _make_variant("qwen3-8b-q4_0", "qwen3:8b")
        registry_path = _write_registry(tmp_path, [variant])
        registry = ModelRegistry.model_validate_json(registry_path.read_text())
        profile_path = _write_profile(tmp_path, registry_hash=registry.content_hash)
        profile = CampaignProfile.model_validate_json(profile_path.read_text())

        cohorts = derive_cohorts_from_registry(profile, registry)
        rb = cohorts[0].role_bindings[0]
        assert rb.sampling_settings.temperature == profile.temperature
        assert rb.sampling_settings.top_p == profile.top_p
        assert rb.sampling_settings.max_tokens == profile.max_tokens
        assert rb.sampling_settings.seed == profile.seed
        assert rb.timeout_seconds == profile.timeout_seconds
