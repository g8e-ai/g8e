# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for the tier-fitness SUT factory.

Verifies that ``build_tier_fitness_sut_config`` constructs a
``SUTConfig`` with the candidate model in the target tier and baseline
models in non-target tiers from ``baseline_tier_mappings``. Also
verifies that ``build_campaign_sut_factory`` dispatches to
``DirectProviderSUT`` for the direct arm and produces a
``G8eeChatSUT``-compatible config for ensemble arms.
"""

from __future__ import annotations

import pytest

from g8e_evals.arms import Arm
from g8e_evals.campaign import (
    ModelCohort,
    RoleModelBinding,
    SamplingSettings,
    compute_model_cohort_hash,
)
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
_OLLAMA_ENDPOINT = "http://192.168.1.2:11434"


def _make_variant(
    variant_id: str = "qwen3-8b",
    served_model_tag: str = "qwen3:8b",
    hf_repo: str = "Qwen/Qwen3-8B",
    hf_sha: str = "e966e34a21f2cb17a6c3e8bc2209a2faed9268fb",
) -> ModelVariant:
    return ModelVariant(
        variant_id=variant_id,
        canonical_display_name=variant_id,
        source_list_alias=variant_id,
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


def _make_cohort(variant_id: str, model_tag: str) -> ModelCohort:
    cohort_id = f"cohort-{variant_id}"
    bindings = [RoleModelBinding(
        role="primary",
        model_id=model_tag,
        provider="ollama",
        endpoint=_OLLAMA_ENDPOINT,
        sampling_settings=SamplingSettings(temperature=0.0, top_p=1.0, max_tokens=1024, seed=42),
        timeout_seconds=120.0,
        seed_capable=True,
    )]
    ch = compute_model_cohort_hash(cohort_id, bindings)
    return ModelCohort(cohort_id=cohort_id, role_bindings=bindings, content_hash=ch)


def _make_tier_fitness_profile(
    variant_ids: list[str],
    tier_assignments: list[ModelTierAssignment],
    baseline_mappings: dict[str, str],
    registry_hash: str,
) -> CampaignProfile:
    track_assignments = [
        TrackArmAssignment(track=CampaignTrack.TIER_FITNESS, arm_id="ensemble_ungoverned"),
    ]
    ch = compute_campaign_profile_hash(
        campaign_id="tier-fitness-v1",
        campaign_revision="rev-1",
        schema_version=CAMPAIGN_PROFILE_VERSION,
        purpose="Tier-fitness campaign",
        generative_variant_ids=variant_ids,
        benchmark_ids=["ifeval_subset"],
        dataset_hashes=[_VALID_HASH],
        grader_hashes=[_VALID_HASH],
        track_arm_assignments=track_assignments,
        model_tier_assignments=tier_assignments,
        repetitions=3,
        model_registry_hash=registry_hash,
    )
    return CampaignProfile(
        campaign_id="tier-fitness-v1",
        campaign_revision="rev-1",
        schema_version=CAMPAIGN_PROFILE_VERSION,
        purpose="Tier-fitness campaign",
        created_at="2026-09-10T00:00:00Z",
        lifecycle_status=CampaignLifecycleStatus.FROZEN,
        generative_variant_ids=variant_ids,
        benchmark_ids=["ifeval_subset"],
        dataset_hashes=[_VALID_HASH],
        grader_hashes=[_VALID_HASH],
        prompt_serialization_hash=_VALID_HASH,
        task_ids=["1001", "1019"],
        repetitions=3,
        track_arm_assignments=track_assignments,
        model_tier_assignments=tier_assignments,
        baseline_tier_mappings=baseline_mappings,
        routing_policy="default",
        temperature=0.0,
        top_p=1.0,
        max_tokens=1024,
        seed=42,
        context_limit=2048,
        timeout_seconds=120.0,
        max_retries=1,
        warmup_excluded=True,
        concurrency=1,
        hardware_identity="linux/amd64/cpu",
        environment_stratum="single-machine",
        primary_metrics=["ifeval_subset_verifier"],
        unit_of_analysis="task",
        claim_boundary=ClaimBoundary.DESCRIPTIVE_ONLY,
        model_registry_hash=registry_hash,
        content_hash=ch,
    )


def _make_registry(variants: list[ModelVariant]) -> ModelRegistry:
    ch = compute_model_registry_hash("reg-1", "1", variants, [])
    return ModelRegistry(
        registry_id="reg-1",
        registry_version="1",
        schema_version=MODEL_REGISTRY_VERSION,
        created_at="2026-09-10T00:00:00Z",
        variants=variants,
        qualification_records=[],
        content_hash=ch,
    )


class TestBuildTierFitnessSutConfig:
    def test_candidate_model_replaces_primary_tier(self):
        from g8e_evals.runner import build_tier_fitness_sut_config

        v = _make_variant("qwen3-8b", "qwen3:8b")
        registry = _make_registry([v])
        profile = _make_tier_fitness_profile(
            variant_ids=["qwen3-8b"],
            tier_assignments=[ModelTierAssignment(variant_id="qwen3-8b", target_tier="primary")],
            baseline_mappings={"assistant": "qwen3:4b", "lite": "qwen3:0.6b"},
            registry_hash=registry.content_hash,
        )
        cohort = _make_cohort("qwen3-8b", "qwen3:8b")
        cohort_variant_map = {"cohort-qwen3-8b": "qwen3-8b"}

        config = build_tier_fitness_sut_config(
            cohort=cohort,
            arm=Arm.ENSEMBLE_UNGOVERNED,
            campaign_profile=profile,
            cohort_variant_map=cohort_variant_map,
            g8ee_url="https://localhost:8443",
        )

        assert config.primary.model == "qwen3:8b"
        assert config.primary.provider == "ollama"
        assert config.assistant.model == "qwen3:4b"
        assert config.assistant.provider == "ollama"
        assert config.lite.model == "qwen3:0.6b"
        assert config.lite.provider == "ollama"
        assert config.candidate_model == "qwen3:8b"

    def test_candidate_model_replaces_assistant_tier(self):
        from g8e_evals.runner import build_tier_fitness_sut_config

        v = _make_variant("qwen3-4b", "qwen3:4b")
        registry = _make_registry([v])
        profile = _make_tier_fitness_profile(
            variant_ids=["qwen3-4b"],
            tier_assignments=[ModelTierAssignment(variant_id="qwen3-4b", target_tier="assistant")],
            baseline_mappings={"primary": "qwen3:8b", "lite": "qwen3:0.6b"},
            registry_hash=registry.content_hash,
        )
        cohort = _make_cohort("qwen3-4b", "qwen3:4b")
        cohort_variant_map = {"cohort-qwen3-4b": "qwen3-4b"}

        config = build_tier_fitness_sut_config(
            cohort=cohort,
            arm=Arm.ENSEMBLE_UNGOVERNED,
            campaign_profile=profile,
            cohort_variant_map=cohort_variant_map,
            g8ee_url="https://localhost:8443",
        )

        assert config.primary.model == "qwen3:8b"
        assert config.assistant.model == "qwen3:4b"
        assert config.lite.model == "qwen3:0.6b"
        assert config.candidate_model == "qwen3:4b"

    def test_candidate_model_replaces_lite_tier(self):
        from g8e_evals.runner import build_tier_fitness_sut_config

        v = _make_variant("qwen3-06b", "qwen3:0.6b")
        registry = _make_registry([v])
        profile = _make_tier_fitness_profile(
            variant_ids=["qwen3-06b"],
            tier_assignments=[ModelTierAssignment(variant_id="qwen3-06b", target_tier="lite")],
            baseline_mappings={"primary": "qwen3:8b", "assistant": "qwen3:4b"},
            registry_hash=registry.content_hash,
        )
        cohort = _make_cohort("qwen3-06b", "qwen3:0.6b")
        cohort_variant_map = {"cohort-qwen3-06b": "qwen3-06b"}

        config = build_tier_fitness_sut_config(
            cohort=cohort,
            arm=Arm.ENSEMBLE_UNGOVERNED,
            campaign_profile=profile,
            cohort_variant_map=cohort_variant_map,
            g8ee_url="https://localhost:8443",
        )

        assert config.primary.model == "qwen3:8b"
        assert config.assistant.model == "qwen3:4b"
        assert config.lite.model == "qwen3:0.6b"
        assert config.candidate_model == "qwen3:0.6b"

    def test_g8ee_url_is_set_on_config(self):
        from g8e_evals.runner import build_tier_fitness_sut_config

        v = _make_variant("qwen3-8b", "qwen3:8b")
        registry = _make_registry([v])
        profile = _make_tier_fitness_profile(
            variant_ids=["qwen3-8b"],
            tier_assignments=[ModelTierAssignment(variant_id="qwen3-8b", target_tier="primary")],
            baseline_mappings={"assistant": "qwen3:4b", "lite": "qwen3:0.6b"},
            registry_hash=registry.content_hash,
        )
        cohort = _make_cohort("qwen3-8b", "qwen3:8b")
        cohort_variant_map = {"cohort-qwen3-8b": "qwen3-8b"}

        config = build_tier_fitness_sut_config(
            cohort=cohort,
            arm=Arm.ENSEMBLE_UNGOVERNED,
            campaign_profile=profile,
            cohort_variant_map=cohort_variant_map,
            g8ee_url="https://localhost:8443",
        )

        assert config.g8ee_url == "https://localhost:8443"

    def test_arm_is_set_on_config(self):
        from g8e_evals.runner import build_tier_fitness_sut_config

        v = _make_variant("qwen3-8b", "qwen3:8b")
        registry = _make_registry([v])
        profile = _make_tier_fitness_profile(
            variant_ids=["qwen3-8b"],
            tier_assignments=[ModelTierAssignment(variant_id="qwen3-8b", target_tier="primary")],
            baseline_mappings={"assistant": "qwen3:4b", "lite": "qwen3:0.6b"},
            registry_hash=registry.content_hash,
        )
        cohort = _make_cohort("qwen3-8b", "qwen3:8b")
        cohort_variant_map = {"cohort-qwen3-8b": "qwen3-8b"}

        config = build_tier_fitness_sut_config(
            cohort=cohort,
            arm=Arm.DOCTRINE,
            campaign_profile=profile,
            cohort_variant_map=cohort_variant_map,
            g8ee_url="https://localhost:8443",
        )

        assert config.arm == Arm.DOCTRINE

    def test_endpoint_from_cohort_for_candidate_model(self):
        from g8e_evals.runner import build_tier_fitness_sut_config

        v = _make_variant("qwen3-8b", "qwen3:8b")
        registry = _make_registry([v])
        profile = _make_tier_fitness_profile(
            variant_ids=["qwen3-8b"],
            tier_assignments=[ModelTierAssignment(variant_id="qwen3-8b", target_tier="primary")],
            baseline_mappings={"assistant": "qwen3:4b", "lite": "qwen3:0.6b"},
            registry_hash=registry.content_hash,
        )
        cohort = _make_cohort("qwen3-8b", "qwen3:8b")
        cohort_variant_map = {"cohort-qwen3-8b": "qwen3-8b"}

        config = build_tier_fitness_sut_config(
            cohort=cohort,
            arm=Arm.ENSEMBLE_UNGOVERNED,
            campaign_profile=profile,
            cohort_variant_map=cohort_variant_map,
            g8ee_url="https://localhost:8443",
        )

        assert config.primary.endpoint == _OLLAMA_ENDPOINT

    def test_baseline_models_use_ollama_provider(self):
        from g8e_evals.runner import build_tier_fitness_sut_config

        v = _make_variant("qwen3-8b", "qwen3:8b")
        registry = _make_registry([v])
        profile = _make_tier_fitness_profile(
            variant_ids=["qwen3-8b"],
            tier_assignments=[ModelTierAssignment(variant_id="qwen3-8b", target_tier="primary")],
            baseline_mappings={"assistant": "qwen3:4b", "lite": "qwen3:0.6b"},
            registry_hash=registry.content_hash,
        )
        cohort = _make_cohort("qwen3-8b", "qwen3:8b")
        cohort_variant_map = {"cohort-qwen3-8b": "qwen3-8b"}

        config = build_tier_fitness_sut_config(
            cohort=cohort,
            arm=Arm.ENSEMBLE_UNGOVERNED,
            campaign_profile=profile,
            cohort_variant_map=cohort_variant_map,
            g8ee_url="https://localhost:8443",
        )

        assert config.assistant.provider == "ollama"
        assert config.lite.provider == "ollama"

    def test_raises_when_variant_not_in_tier_assignments(self):
        from g8e_evals.runner import build_tier_fitness_sut_config

        v = _make_variant("qwen3-8b", "qwen3:8b")
        registry = _make_registry([v])
        profile = _make_tier_fitness_profile(
            variant_ids=["qwen3-8b"],
            tier_assignments=[],
            baseline_mappings={"assistant": "qwen3:4b", "lite": "qwen3:0.6b"},
            registry_hash=registry.content_hash,
        )
        cohort = _make_cohort("qwen3-8b", "qwen3:8b")
        cohort_variant_map = {"cohort-qwen3-8b": "qwen3-8b"}

        with pytest.raises(ValueError, match="no model_tier_assignment found"):
            build_tier_fitness_sut_config(
                cohort=cohort,
                arm=Arm.ENSEMBLE_UNGOVERNED,
                campaign_profile=profile,
                cohort_variant_map=cohort_variant_map,
                g8ee_url="https://localhost:8443",
            )

    def test_raises_when_baseline_mapping_missing_for_non_target_tier(self):
        from g8e_evals.runner import build_tier_fitness_sut_config

        v = _make_variant("qwen3-8b", "qwen3:8b")
        registry = _make_registry([v])
        profile = _make_tier_fitness_profile(
            variant_ids=["qwen3-8b"],
            tier_assignments=[ModelTierAssignment(variant_id="qwen3-8b", target_tier="primary")],
            baseline_mappings={"assistant": "qwen3:4b"},
            registry_hash=registry.content_hash,
        )
        cohort = _make_cohort("qwen3-8b", "qwen3:8b")
        cohort_variant_map = {"cohort-qwen3-8b": "qwen3-8b"}

        with pytest.raises(ValueError, match="missing baseline_tier_mapping for lite"):
            build_tier_fitness_sut_config(
                cohort=cohort,
                arm=Arm.ENSEMBLE_UNGOVERNED,
                campaign_profile=profile,
                cohort_variant_map=cohort_variant_map,
                g8ee_url="https://localhost:8443",
            )

    def test_raises_when_cohort_not_in_variant_map(self):
        from g8e_evals.runner import build_tier_fitness_sut_config

        v = _make_variant("qwen3-8b", "qwen3:8b")
        registry = _make_registry([v])
        profile = _make_tier_fitness_profile(
            variant_ids=["qwen3-8b"],
            tier_assignments=[ModelTierAssignment(variant_id="qwen3-8b", target_tier="primary")],
            baseline_mappings={"assistant": "qwen3:4b", "lite": "qwen3:0.6b"},
            registry_hash=registry.content_hash,
        )
        cohort = _make_cohort("qwen3-8b", "qwen3:8b")
        cohort_variant_map = {"cohort-other": "other"}

        with pytest.raises(ValueError, match=r"cohort .* not found in cohort_variant_map"):
            build_tier_fitness_sut_config(
                cohort=cohort,
                arm=Arm.ENSEMBLE_UNGOVERNED,
                campaign_profile=profile,
                cohort_variant_map=cohort_variant_map,
                g8ee_url="https://localhost:8443",
            )


class TestBuildCampaignSutFactory:
    def test_direct_arm_creates_direct_provider_sut(self):
        from g8e_evals.runner import build_campaign_sut_factory
        from g8e_evals.sut.direct_provider import DirectProviderSUT

        v = _make_variant("qwen3-8b", "qwen3:8b")
        registry = _make_registry([v])
        profile = _make_tier_fitness_profile(
            variant_ids=["qwen3-8b"],
            tier_assignments=[ModelTierAssignment(variant_id="qwen3-8b", target_tier="primary")],
            baseline_mappings={"assistant": "qwen3:4b", "lite": "qwen3:0.6b"},
            registry_hash=registry.content_hash,
        )
        cohort = _make_cohort("qwen3-8b", "qwen3:8b")
        cohort_variant_map = {"cohort-qwen3-8b": "qwen3-8b"}

        factory = build_campaign_sut_factory(
            campaign_profile=profile,
            cohort_variant_map=cohort_variant_map,
            g8ee_url="https://localhost:8443",
        )
        sut = factory(cohort, Arm.DIRECT)
        assert isinstance(sut, DirectProviderSUT)

    def test_direct_arm_does_not_set_candidate_model(self):
        from g8e_evals.runner import build_campaign_sut_factory
        from g8e_evals.sut.direct_provider import DirectProviderSUT

        v = _make_variant("qwen3-4b", "qwen3:4b")
        registry = _make_registry([v])
        profile = _make_tier_fitness_profile(
            variant_ids=["qwen3-4b"],
            tier_assignments=[ModelTierAssignment(variant_id="qwen3-4b", target_tier="assistant")],
            baseline_mappings={"primary": "qwen3:8b", "lite": "qwen3:0.6b"},
            registry_hash=registry.content_hash,
        )
        cohort = _make_cohort("qwen3-4b", "qwen3:4b")
        cohort_variant_map = {"cohort-qwen3-4b": "qwen3-4b"}

        factory = build_campaign_sut_factory(
            campaign_profile=profile,
            cohort_variant_map=cohort_variant_map,
            g8ee_url="https://localhost:8443",
        )
        sut = factory(cohort, Arm.DIRECT)
        assert isinstance(sut, DirectProviderSUT)
        # DirectProviderSUT uses primary.model for model_provider, which
        # is the candidate's served_model_tag. candidate_model is not set
        # because the direct arm always uses the primary as the candidate.
        assert sut.model_provider == "ollama:qwen3:4b"

    def test_ensemble_arm_creates_g8ee_chat_sut_config(self):
        from unittest.mock import patch, MagicMock

        from g8e_evals.runner import build_campaign_sut_factory

        v = _make_variant("qwen3-8b", "qwen3:8b")
        registry = _make_registry([v])
        profile = _make_tier_fitness_profile(
            variant_ids=["qwen3-8b"],
            tier_assignments=[ModelTierAssignment(variant_id="qwen3-8b", target_tier="primary")],
            baseline_mappings={"assistant": "qwen3:4b", "lite": "qwen3:0.6b"},
            registry_hash=registry.content_hash,
        )
        cohort = _make_cohort("qwen3-8b", "qwen3:8b")
        cohort_variant_map = {"cohort-qwen3-8b": "qwen3-8b"}

        factory = build_campaign_sut_factory(
            campaign_profile=profile,
            cohort_variant_map=cohort_variant_map,
            g8ee_url="https://localhost:8443",
        )
        # G8eeChatSUT.__init__ calls AuthContext.from_env() which requires
        # real auth. Mock the class to capture the config without invoking
        # the auth chain.
        mock_sut = MagicMock()
        mock_sut.config = MagicMock()
        with patch("g8e_evals.sut.g8ee_chat.G8eeChatSUT", return_value=mock_sut) as mock_cls:
            factory(cohort, Arm.ENSEMBLE_UNGOVERNED)

        assert mock_cls.call_count == 1
        captured_config = mock_cls.call_args[0][0]
        assert captured_config.g8ee_url == "https://localhost:8443"
        assert captured_config.arm == Arm.ENSEMBLE_UNGOVERNED
        assert captured_config.primary.model == "qwen3:8b"
        assert captured_config.assistant.model == "qwen3:4b"
        assert captured_config.lite.model == "qwen3:0.6b"
        assert captured_config.candidate_model == "qwen3:8b"
