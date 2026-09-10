# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for extended campaign profile and registry validation.

Verifies validation rules beyond the existing round-trip, unknown-field,
and content-hash tests: tier-assignment variants must be in the profile's
generative set, target tiers come from a closed vocabulary, track arm IDs
are valid, and benchmark/dataset/grader/task lists are internally unique
and consistent. No external dependencies (no files, network, or DB).
"""

# pyright: reportCallIssue=false
# This file intentionally constructs models with inconsistent configurations
# to verify validation rejects them.

from __future__ import annotations

import pytest
from pydantic import ValidationError

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


def _make_profile(
    *,
    generative_variant_ids: list[str] | None = None,
    track_arm_assignments: list[TrackArmAssignment] | None = None,
    model_tier_assignments: list[ModelTierAssignment] | None = None,
    model_registry_hash: str = _VALID_HASH,
    benchmark_ids: list[str] | None = None,
    dataset_hashes: list[str] | None = None,
    grader_hashes: list[str] | None = None,
    task_ids: list[str] | None = None,
    primary_metrics: list[str] | None = None,
) -> CampaignProfile:
    if generative_variant_ids is None:
        generative_variant_ids = ["qwen3-8b-q4_0"]
    if track_arm_assignments is None:
        track_arm_assignments = [
            TrackArmAssignment(track=CampaignTrack.DIRECT, arm_id="direct"),
            TrackArmAssignment(track=CampaignTrack.TIER_FITNESS, arm_id="ensemble_ungoverned"),
        ]
    if model_tier_assignments is None:
        model_tier_assignments = [
            ModelTierAssignment(variant_id="qwen3-8b-q4_0", target_tier="primary"),
        ]
    if benchmark_ids is None:
        benchmark_ids = ["ifeval_subset"]
    if dataset_hashes is None:
        dataset_hashes = [_VALID_HASH]
    if grader_hashes is None:
        grader_hashes = [_VALID_HASH]
    if task_ids is None:
        task_ids = ["task-1", "task-2", "task-3"]
    if primary_metrics is None:
        primary_metrics = ["ifeval_subset_verifier"]
    ch = compute_campaign_profile_hash(
        campaign_id="generative-campaign-v1",
        campaign_revision="1",
        schema_version=CAMPAIGN_PROFILE_VERSION,
        purpose="46-model generative comparison campaign",
        generative_variant_ids=generative_variant_ids,
        benchmark_ids=benchmark_ids,
        dataset_hashes=dataset_hashes,
        grader_hashes=grader_hashes,
        track_arm_assignments=track_arm_assignments,
        model_tier_assignments=model_tier_assignments,
        repetitions=3,
        model_registry_hash=model_registry_hash,
    )
    return CampaignProfile(
        campaign_id="generative-campaign-v1",
        campaign_revision="1",
        schema_version=CAMPAIGN_PROFILE_VERSION,
        purpose="46-model generative comparison campaign",
        created_at="2026-09-10T00:00:00Z",
        lifecycle_status=CampaignLifecycleStatus.DRAFT,
        generative_variant_ids=generative_variant_ids,
        benchmark_ids=benchmark_ids,
        dataset_hashes=dataset_hashes,
        grader_hashes=grader_hashes,
        prompt_serialization_hash=_VALID_HASH,
        task_ids=task_ids,
        repetitions=3,
        track_arm_assignments=track_arm_assignments,
        model_tier_assignments=model_tier_assignments,
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
        primary_metrics=primary_metrics,
        unit_of_analysis="task",
        claim_boundary=ClaimBoundary.DESCRIPTIVE_ONLY,
        model_registry_hash=model_registry_hash,
        content_hash=ch,
    )


class TestProfileTierAssignmentValidation:
    def test_tier_assignment_variant_must_be_in_generative_set(self):
        """A model_tier_assignment for a variant not in generative_variant_ids is invalid."""
        variant1 = _make_variant("qwen3-8b-q4_0")
        variant2 = _make_variant("other-variant")
        variant2 = variant2.model_copy(update={"quantization": "fp16"})
        registry = _make_registry([variant1, variant2])
        profile = _make_profile(
            generative_variant_ids=["qwen3-8b-q4_0"],
            model_tier_assignments=[
                ModelTierAssignment(variant_id="other-variant", target_tier="primary"),
            ],
            model_registry_hash=registry.content_hash,
        )
        with pytest.raises(ValueError, match="not in generative_variant_ids"):
            profile.validate_against_registry(registry)

    def test_tier_assignment_target_tier_must_be_known(self):
        """target_tier must be one of primary, assistant, lite."""
        with pytest.raises(ValidationError, match="target_tier"):
            _make_profile(
                model_tier_assignments=[
                    ModelTierAssignment(variant_id="qwen3-8b-q4_0", target_tier="unknown_tier"),
                ],
            )


class TestProfileListUniqueness:
    def test_benchmark_ids_must_be_unique(self):
        with pytest.raises(ValueError, match="duplicate benchmark_id"):
            _make_profile(benchmark_ids=["ifeval_subset", "ifeval_subset"])

    def test_dataset_hashes_must_be_unique(self):
        with pytest.raises(ValueError, match="duplicate dataset_hash"):
            _make_profile(dataset_hashes=[_VALID_HASH, _VALID_HASH])

    def test_grader_hashes_must_be_unique(self):
        with pytest.raises(ValueError, match="duplicate grader_hash"):
            _make_profile(grader_hashes=[_VALID_HASH, _VALID_HASH])

    def test_task_ids_must_be_unique(self):
        with pytest.raises(ValueError, match="duplicate task_id"):
            _make_profile(task_ids=["task-1", "task-1", "task-3"])


class TestProfileTrackArmValidation:
    def test_track_arm_assignment_arm_id_must_be_valid(self):
        """arm_id in track_arm_assignments must be a known arm."""
        with pytest.raises(ValueError, match="unknown arm_id"):
            _make_profile(
                track_arm_assignments=[
                    TrackArmAssignment(track=CampaignTrack.DIRECT, arm_id="nonexistent_arm"),
                ],
            )


class TestRegistryValidation:
    def test_registry_rejects_duplicate_hf_sha_with_different_variant_id(self):
        """Two variants with the same hf_repo and hf_sha but different variant_ids
        are allowed only if they represent genuinely different quantizations."""
        variant1 = _make_variant("qwen3-8b-q4_0")
        variant2 = _make_variant("qwen3-8b-fp16")
        variant2 = variant2.model_copy(update={"quantization": "fp16"})
        # Same repo and sha, different quantization: allowed
        ch = compute_model_registry_hash("reg-1", "1", [variant1, variant2], [])
        registry = ModelRegistry(
            registry_id="reg-1",
            registry_version="1",
            schema_version=MODEL_REGISTRY_VERSION,
            created_at="2026-09-10T00:00:00Z",
            variants=[variant1, variant2],
            qualification_records=[],
            content_hash=ch,
        )
        assert len(registry.variants) == 2

    def test_registry_rejects_duplicate_variant_with_same_quantization(self):
        """Two variants with the same hf_repo, hf_sha, and quantization are duplicates."""
        variant1 = _make_variant("qwen3-8b-q4_0")
        variant2 = _make_variant("qwen3-8b-q4_0-dup")
        # Same repo, sha, and quantization: should be rejected as duplicate
        ch = compute_model_registry_hash("reg-1", "1", [variant1, variant2], [])
        with pytest.raises(ValueError, match=r"duplicate.*artifact"):
            ModelRegistry(
                registry_id="reg-1",
                registry_version="1",
                schema_version=MODEL_REGISTRY_VERSION,
                created_at="2026-09-10T00:00:00Z",
                variants=[variant1, variant2],
                qualification_records=[],
                content_hash=ch,
            )
