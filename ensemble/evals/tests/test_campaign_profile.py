# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for the typed campaign profile contract.

Verifies that the CampaignProfile typed model and its sub-models
(TrackArmAssignment, ModelTierAssignment, CampaignLifecycleStatus,
ClaimBoundary) enforce strict field validation, reject unknown fields,
round-trip through JSON, validate content hashes, and reject
inconsistent configurations (e.g. variant IDs not in the registry,
missing track assignments, duplicate tier assignments). No external
dependencies (no files, network, or DB).
"""

# pyright: reportCallIssue=false
# This file intentionally constructs models with missing required fields
# and unknown extra fields to verify pydantic validation rejects them.

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
    temp = CampaignProfile.model_construct(
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
        primary_metrics=["ifeval_subset_verifier"],
        unit_of_analysis="task",
        claim_boundary=ClaimBoundary.DESCRIPTIVE_ONLY,
        model_registry_hash=model_registry_hash,
        content_hash="0" * 64,
    )
    ch = compute_campaign_profile_hash(temp)
    return CampaignProfile(
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
        primary_metrics=["ifeval_subset_verifier"],
        unit_of_analysis="task",
        claim_boundary=ClaimBoundary.DESCRIPTIVE_ONLY,
        model_registry_hash=model_registry_hash,
        content_hash=ch,
    )


class TestCampaignLifecycleStatus:
    def test_draft_value(self):
        assert CampaignLifecycleStatus.DRAFT.value == "draft"

    def test_frozen_value(self):
        assert CampaignLifecycleStatus.FROZEN.value == "frozen"

    def test_active_value(self):
        assert CampaignLifecycleStatus.ACTIVE.value == "active"

    def test_completed_value(self):
        assert CampaignLifecycleStatus.COMPLETED.value == "completed"

    def test_superseded_value(self):
        assert CampaignLifecycleStatus.SUPERSEDED.value == "superseded"


class TestClaimBoundary:
    def test_descriptive_only_value(self):
        assert ClaimBoundary.DESCRIPTIVE_ONLY.value == "descriptive_only"

    def test_confirmatory_value(self):
        assert ClaimBoundary.CONFIRMATORY.value == "confirmatory"


class TestTrackArmAssignment:
    def test_round_trip_preserves_all_fields(self):
        assignment = TrackArmAssignment(track=CampaignTrack.DIRECT, arm_id="direct")
        restored = TrackArmAssignment.model_validate_json(assignment.model_dump_json())
        assert restored == assignment

    def test_rejects_unknown_fields(self):
        with pytest.raises(ValidationError):
            TrackArmAssignment(
                track=CampaignTrack.DIRECT,
                arm_id="direct",
                extra_field="bad",
            )

    def test_requires_track(self):
        with pytest.raises(ValidationError):
            TrackArmAssignment(arm_id="direct")

    def test_requires_arm_id(self):
        with pytest.raises(ValidationError):
            TrackArmAssignment(track=CampaignTrack.DIRECT)

    def test_frozen_model(self):
        assignment = TrackArmAssignment(track=CampaignTrack.DIRECT, arm_id="direct")
        with pytest.raises(ValidationError):
            assignment.arm_id = "changed"  # type: ignore[misc]


class TestModelTierAssignment:
    def test_round_trip_preserves_all_fields(self):
        assignment = ModelTierAssignment(variant_id="qwen3-8b-q4_0", target_tier="primary")
        restored = ModelTierAssignment.model_validate_json(assignment.model_dump_json())
        assert restored == assignment

    def test_rejects_unknown_fields(self):
        with pytest.raises(ValidationError):
            ModelTierAssignment(
                variant_id="qwen3-8b-q4_0",
                target_tier="primary",
                extra_field="bad",
            )

    def test_requires_variant_id(self):
        with pytest.raises(ValidationError):
            ModelTierAssignment(target_tier="primary")

    def test_requires_target_tier(self):
        with pytest.raises(ValidationError):
            ModelTierAssignment(variant_id="qwen3-8b-q4_0")

    def test_rejects_empty_target_tier(self):
        with pytest.raises(ValidationError):
            ModelTierAssignment(variant_id="qwen3-8b-q4_0", target_tier="")

    def test_frozen_model(self):
        assignment = ModelTierAssignment(variant_id="qwen3-8b-q4_0", target_tier="primary")
        with pytest.raises(ValidationError):
            assignment.target_tier = "changed"  # type: ignore[misc]


class TestCampaignProfile:
    def test_round_trip_preserves_all_fields(self):
        profile = _make_profile()
        restored = CampaignProfile.model_validate_json(profile.model_dump_json())
        assert restored == profile

    def test_rejects_unknown_fields(self):
        data = _make_profile().model_dump()
        data["extra_field"] = "bad"
        with pytest.raises(ValidationError):
            CampaignProfile(**data)

    def test_requires_campaign_id(self):
        data = _make_profile().model_dump()
        del data["campaign_id"]
        with pytest.raises(ValidationError):
            CampaignProfile(**data)

    def test_requires_generative_variant_ids(self):
        data = _make_profile().model_dump()
        del data["generative_variant_ids"]
        with pytest.raises(ValidationError):
            CampaignProfile(**data)

    def test_rejects_empty_generative_variant_ids(self):
        data = _make_profile().model_dump()
        data["generative_variant_ids"] = []
        with pytest.raises(ValidationError):
            CampaignProfile(**data)

    def test_rejects_duplicate_generative_variant_ids(self):
        data = _make_profile().model_dump()
        data["generative_variant_ids"] = ["v1", "v1"]
        with pytest.raises(ValueError, match="duplicate generative_variant_id"):
            CampaignProfile(**data)

    def test_rejects_duplicate_track_arm_assignments(self):
        assignment = TrackArmAssignment(track=CampaignTrack.DIRECT, arm_id="direct")
        data = _make_profile().model_dump()
        data["track_arm_assignments"] = [assignment, assignment]
        with pytest.raises(ValueError, match="duplicate track in track_arm_assignments"):
            CampaignProfile(**data)

    def test_rejects_duplicate_model_tier_assignments(self):
        assignment = ModelTierAssignment(variant_id="v1", target_tier="primary")
        data = _make_profile().model_dump()
        data["model_tier_assignments"] = [assignment, assignment]
        with pytest.raises(ValueError, match="duplicate variant_id in model_tier_assignments"):
            CampaignProfile(**data)

    def test_content_hash_mismatch_raises(self):
        data = _make_profile().model_dump()
        data["content_hash"] = "0" * 64
        with pytest.raises(ValueError, match="content_hash mismatch"):
            CampaignProfile(**data)

    def test_frozen_model(self):
        profile = _make_profile()
        with pytest.raises(ValidationError):
            profile.campaign_id = "changed"  # type: ignore[misc]

    def test_validate_against_registry_rejects_unknown_variant(self):
        variant = _make_variant("qwen3-8b-q4_0")
        registry = _make_registry([variant])
        profile = _make_profile(
            generative_variant_ids=["qwen3-8b-q4_0", "nonexistent"],
            model_registry_hash=registry.content_hash,
        )
        with pytest.raises(ValueError, match="not found in registry"):
            profile.validate_against_registry(registry)

    def test_validate_against_registry_rejects_hash_mismatch(self):
        registry = _make_registry([_make_variant("qwen3-8b-q4_0")])
        profile = _make_profile(model_registry_hash="b" * 64)
        with pytest.raises(ValueError, match="model_registry_hash mismatch"):
            profile.validate_against_registry(registry)

    def test_validate_against_registry_passes_when_consistent(self):
        variant = _make_variant("qwen3-8b-q4_0")
        registry = _make_registry([variant])
        profile = _make_profile(
            generative_variant_ids=["qwen3-8b-q4_0"],
            model_registry_hash=registry.content_hash,
        )
        profile.validate_against_registry(registry)

    def test_validate_against_registry_rejects_tier_assignment_for_unknown_variant(self):
        variant = _make_variant("qwen3-8b-q4_0")
        registry = _make_registry([variant])
        profile = _make_profile(
            generative_variant_ids=["qwen3-8b-q4_0"],
            model_registry_hash=registry.content_hash,
            model_tier_assignments=[
                ModelTierAssignment(variant_id="nonexistent", target_tier="primary"),
            ],
        )
        with pytest.raises(ValueError, match="not found in registry"):
            profile.validate_against_registry(registry)
