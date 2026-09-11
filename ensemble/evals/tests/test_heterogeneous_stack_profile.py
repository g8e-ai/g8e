# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for the heterogeneous-stack campaign profile builder.

Verifies that ``build_heterogeneous_stack_profile`` constructs a frozen
``CampaignProfile`` and ``RoleCombinationTrack`` from a heterogeneous
stack composition where all three roles (primary, assistant, lite) are
explicitly assigned to specific model variants. Validates all
constraints (exactly three role keys, distinct variant IDs, all
variant IDs in the registry), produces the correct
``model_tier_assignments`` (one per role), empty
``baseline_tier_mappings`` (no baselines), a single
``RoleCombination`` representing the complete stack, and
``TaskRoleBinding`` records for the Cartesian product of tasks and the
one combination. Both frozen artifacts validate and round-trip through
JSON. No external dependencies (no files, network, or DB).
"""

from __future__ import annotations

import json

import pytest

from g8e_evals.analysis_contract import RoleCombinationTrack
from g8e_evals.heterogeneous_stack import (
    HeterogeneousStackProfileBundle,
    build_heterogeneous_stack_profile,
)
from g8e_evals.profile import (
    CAMPAIGN_PROFILE_VERSION,
    CampaignLifecycleStatus,
    CampaignProfile,
    ClaimBoundary,
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


def _make_variant(
    variant_id: str,
    served_model_tag: str,
    weight_class: WeightClass = WeightClass.HEAVY_SLM,
) -> ModelVariant:
    return ModelVariant(
        variant_id=variant_id,
        canonical_display_name=variant_id,
        source_list_alias=variant_id,
        hf_repo=f"org/{variant_id}",
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
        weight_class=weight_class,
        backend_name="ollama",
        backend_version="0.1.48",
        served_model_tag=served_model_tag,
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


def _default_variants() -> list[ModelVariant]:
    return [
        _make_variant("qwen3-8b", "qwen3:8b", WeightClass.HEAVY_SLM),
        _make_variant("qwen3-4b", "qwen3:4b", WeightClass.HEAVY_SLM),
        _make_variant("qwen3-0.6b", "qwen3:0.6b", WeightClass.TINY_GENERATIVE),
        _make_variant("phi4-mini", "phi4:mini", WeightClass.HEAVY_SLM),
        _make_variant("smollm2-360m", "smollm2:360m", WeightClass.TINY_GENERATIVE),
        _make_variant("granite-8b", "granite:8b", WeightClass.HEAVY_SLM),
        _make_variant("granite-3b", "granite:3b", WeightClass.SMALL_REASONING),
        _make_variant("smollm3-3b", "smollm3:3b", WeightClass.SMALL_REASONING),
    ]


def _default_stack_composition() -> dict[str, str]:
    return {
        "primary": "qwen3-8b",
        "assistant": "phi4-mini",
        "lite": "smollm2-360m",
    }


def _build_bundle(
    variants: list[ModelVariant] | None = None,
    stack_composition: dict[str, str] | None = None,
    stack_label: str = "heterogeneous-1",
    task_ids: list[str] | None = None,
    repetitions: int = 5,
    role_exercise_declarations: dict[str, list[str]] | None = None,
    track_id: str | None = None,
    track_description: str | None = None,
    routing_policy: str = "default",
    claim_boundary: ClaimBoundary = ClaimBoundary.DESCRIPTIVE_ONLY,
) -> HeterogeneousStackProfileBundle:
    if variants is None:
        variants = _default_variants()
    if stack_composition is None:
        stack_composition = _default_stack_composition()
    if task_ids is None:
        task_ids = ["task-001", "task-002", "task-003"]
    registry = _make_registry(variants)
    return build_heterogeneous_stack_profile(
        campaign_id="stack-campaign-v1",
        campaign_revision="rev-1",
        purpose="Heterogeneous stack campaign for evaluation framework v2",
        created_at="2026-09-10T00:00:00Z",
        stack_composition=stack_composition,
        stack_label=stack_label,
        registry=registry,
        benchmark_ids=["ifeval_subset", "tool_selection", "tool_arguments"],
        dataset_hashes=[_VALID_HASH],
        grader_hashes=[_VALID_HASH],
        prompt_serialization_hash=_VALID_HASH,
        task_ids=task_ids,
        repetitions=repetitions,
        primary_metrics=["ifeval_subset_verifier"],
        hardware_identity="linux/amd64/rtx-4090",
        environment_stratum="single-machine",
        role_exercise_declarations=role_exercise_declarations,
        track_id=track_id,
        track_description=track_description,
        routing_policy=routing_policy,
        claim_boundary=claim_boundary,
    )


class TestBuildHeterogeneousStackProfileSuccess:
    def test_returns_bundle_with_profile_and_track(self):
        bundle = _build_bundle()
        assert isinstance(bundle, HeterogeneousStackProfileBundle)
        assert bundle.campaign_profile is not None
        assert bundle.role_combination_track is not None

    def test_campaign_profile_is_frozen(self):
        bundle = _build_bundle()
        assert bundle.campaign_profile.lifecycle_status == CampaignLifecycleStatus.FROZEN

    def test_campaign_profile_has_content_hash(self):
        bundle = _build_bundle()
        assert len(bundle.campaign_profile.content_hash) == 64

    def test_role_combination_track_has_content_hash(self):
        bundle = _build_bundle()
        assert len(bundle.role_combination_track.content_hash) == 64

    def test_generative_variant_ids_are_all_stack_variants(self):
        bundle = _build_bundle()
        expected = sorted(["qwen3-8b", "phi4-mini", "smollm2-360m"])
        assert bundle.campaign_profile.generative_variant_ids == expected

    def test_model_tier_assignments_one_per_role(self):
        bundle = _build_bundle()
        assignments = bundle.campaign_profile.model_tier_assignments
        assert len(assignments) == 3
        by_variant = {a.variant_id: a.target_tier for a in assignments}
        assert by_variant["qwen3-8b"] == "primary"
        assert by_variant["phi4-mini"] == "assistant"
        assert by_variant["smollm2-360m"] == "lite"

    def test_baseline_tier_mappings_are_empty(self):
        bundle = _build_bundle()
        assert bundle.campaign_profile.baseline_tier_mappings == {}

    def test_track_arm_assignment_is_tier_fitness_ensemble(self):
        bundle = _build_bundle()
        assert len(bundle.campaign_profile.track_arm_assignments) == 1
        taa = bundle.campaign_profile.track_arm_assignments[0]
        assert taa.track == CampaignTrack.TIER_FITNESS
        assert taa.arm_id == "ensemble_ungoverned"

    def test_claim_boundary_defaults_to_descriptive_only(self):
        bundle = _build_bundle()
        assert bundle.campaign_profile.claim_boundary == ClaimBoundary.DESCRIPTIVE_ONLY

    def test_role_combination_track_has_exactly_one_combination(self):
        bundle = _build_bundle()
        assert len(bundle.role_combination_track.combinations) == 1

    def test_combination_has_correct_variant_ids_for_all_roles(self):
        bundle = _build_bundle()
        combo = bundle.role_combination_track.combinations[0]
        assert combo.combination_id.primary_variant_id == "qwen3-8b"
        assert combo.combination_id.assistant_variant_id == "phi4-mini"
        assert combo.combination_id.lite_variant_id == "smollm2-360m"

    def test_all_combination_variant_ids_are_distinct(self):
        bundle = _build_bundle()
        combo = bundle.role_combination_track.combinations[0]
        ids = [
            combo.combination_id.primary_variant_id,
            combo.combination_id.assistant_variant_id,
            combo.combination_id.lite_variant_id,
        ]
        assert len(ids) == len(set(ids))

    def test_combination_id_includes_stack_label(self):
        bundle = _build_bundle(stack_label="best-accuracy")
        combo = bundle.role_combination_track.combinations[0]
        assert "best-accuracy" in combo.combination_id.combination_id

    def test_combination_rationale_includes_stack_label(self):
        bundle = _build_bundle(stack_label="all-qwen")
        combo = bundle.role_combination_track.combinations[0]
        assert "all-qwen" in combo.rationale

    def test_task_role_bindings_are_cartesian_product(self):
        task_ids = ["task-001", "task-002", "task-003"]
        bundle = _build_bundle(task_ids=task_ids)
        num_combos = len(bundle.role_combination_track.combinations)
        expected_bindings = len(task_ids) * num_combos
        assert len(bundle.role_combination_track.task_role_bindings) == expected_bindings

    def test_task_role_binding_variant_ids_match_combination(self):
        bundle = _build_bundle()
        combo = bundle.role_combination_track.combinations[0]
        for binding in bundle.role_combination_track.task_role_bindings:
            assert binding.combination_id == combo.combination_id.combination_id
            assert binding.primary_variant_id == combo.combination_id.primary_variant_id
            assert binding.assistant_variant_id == combo.combination_id.assistant_variant_id
            assert binding.lite_variant_id == combo.combination_id.lite_variant_id

    def test_role_exercise_declarations_default_to_all_roles(self):
        task_ids = ["task-001", "task-002"]
        bundle = _build_bundle(task_ids=task_ids)
        declarations = bundle.role_combination_track.role_exercise_declarations
        assert len(declarations) == 2
        for d in declarations:
            assert d.exercised_roles == ["assistant", "lite", "primary"]

    def test_role_exercise_declarations_from_input(self):
        task_ids = ["task-001", "task-002"]
        declarations_input = {
            "task-001": ["lite"],
            "task-002": ["assistant", "primary"],
        }
        bundle = _build_bundle(
            task_ids=task_ids,
            role_exercise_declarations=declarations_input,
        )
        declarations = bundle.role_combination_track.role_exercise_declarations
        decl_map = {d.task_id: d.exercised_roles for d in declarations}
        assert decl_map["task-001"] == ["lite"]
        assert decl_map["task-002"] == ["assistant", "primary"]

    def test_campaign_profile_validates_against_registry(self):
        variants = _default_variants()
        registry = _make_registry(variants)
        bundle = _build_bundle(variants=variants)
        bundle.campaign_profile.validate_against_registry(registry)

    def test_campaign_profile_round_trips_through_json(self):
        bundle = _build_bundle()
        data = json.loads(bundle.campaign_profile.model_dump_json())
        restored = CampaignProfile.model_validate(data)
        assert restored.content_hash == bundle.campaign_profile.content_hash
        assert restored.generative_variant_ids == bundle.campaign_profile.generative_variant_ids

    def test_role_combination_track_round_trips_through_json(self):
        bundle = _build_bundle()
        data = json.loads(bundle.role_combination_track.model_dump_json())
        restored = RoleCombinationTrack.model_validate(data)
        assert restored.content_hash == bundle.role_combination_track.content_hash
        assert len(restored.combinations) == len(bundle.role_combination_track.combinations)

    def test_profile_schema_version_matches_campaign_profile_version(self):
        bundle = _build_bundle()
        assert bundle.campaign_profile.schema_version == CAMPAIGN_PROFILE_VERSION

    def test_model_registry_hash_matches_registry(self):
        variants = _default_variants()
        registry = _make_registry(variants)
        bundle = _build_bundle(variants=variants)
        assert bundle.campaign_profile.model_registry_hash == registry.content_hash

    def test_repetitions_set_on_profile(self):
        bundle = _build_bundle(repetitions=5)
        assert bundle.campaign_profile.repetitions == 5

    def test_routing_policy_defaults_to_default(self):
        bundle = _build_bundle()
        assert bundle.campaign_profile.routing_policy == "default"

    def test_custom_routing_policy(self):
        bundle = _build_bundle(routing_policy="cost-aware")
        assert bundle.campaign_profile.routing_policy == "cost-aware"

    def test_custom_claim_boundary(self):
        bundle = _build_bundle(claim_boundary=ClaimBoundary.CONFIRMATORY)
        assert bundle.campaign_profile.claim_boundary == ClaimBoundary.CONFIRMATORY

    def test_default_track_id_includes_stack_label(self):
        bundle = _build_bundle(stack_label="best-efficiency")
        assert bundle.role_combination_track.track_id == "stack-best-efficiency-track"

    def test_custom_track_id(self):
        bundle = _build_bundle(track_id="custom-stack-track")
        assert bundle.role_combination_track.track_id == "custom-stack-track"

    def test_custom_track_description(self):
        bundle = _build_bundle(track_description="Custom description for stack")
        assert bundle.role_combination_track.description == "Custom description for stack"

    def test_default_track_description_includes_stack_label(self):
        bundle = _build_bundle(stack_label="smallest-possible")
        assert "smallest-possible" in bundle.role_combination_track.description

    def test_two_different_stacks_produce_different_profiles(self):
        variants = _default_variants()
        registry = _make_registry(variants)
        bundle_a = build_heterogeneous_stack_profile(
            campaign_id="stack-campaign-v1",
            campaign_revision="rev-1",
            purpose="Stack campaign",
            created_at="2026-09-10T00:00:00Z",
            stack_composition={
                "primary": "qwen3-8b",
                "assistant": "phi4-mini",
                "lite": "smollm2-360m",
            },
            stack_label="heterogeneous-1",
            registry=registry,
            benchmark_ids=["ifeval_subset"],
            dataset_hashes=[_VALID_HASH],
            grader_hashes=[_VALID_HASH],
            prompt_serialization_hash=_VALID_HASH,
            task_ids=["task-001"],
            repetitions=1,
            primary_metrics=["ifeval_subset_verifier"],
            hardware_identity="linux/amd64/rtx-4090",
            environment_stratum="single-machine",
        )
        bundle_b = build_heterogeneous_stack_profile(
            campaign_id="stack-campaign-v1",
            campaign_revision="rev-1",
            purpose="Stack campaign",
            created_at="2026-09-10T00:00:00Z",
            stack_composition={
                "primary": "granite-8b",
                "assistant": "granite-3b",
                "lite": "qwen3-0.6b",
            },
            stack_label="heterogeneous-2",
            registry=registry,
            benchmark_ids=["ifeval_subset"],
            dataset_hashes=[_VALID_HASH],
            grader_hashes=[_VALID_HASH],
            prompt_serialization_hash=_VALID_HASH,
            task_ids=["task-001"],
            repetitions=1,
            primary_metrics=["ifeval_subset_verifier"],
            hardware_identity="linux/amd64/rtx-4090",
            environment_stratum="single-machine",
        )
        assert bundle_a.campaign_profile.content_hash != bundle_b.campaign_profile.content_hash
        assert bundle_a.role_combination_track.content_hash != bundle_b.role_combination_track.content_hash

    def test_same_stack_different_label_produces_different_tracks(self):
        variants = _default_variants()
        registry = _make_registry(variants)
        common_kwargs = {
            "campaign_id": "stack-campaign-v1",
            "campaign_revision": "rev-1",
            "purpose": "Stack campaign",
            "created_at": "2026-09-10T00:00:00Z",
            "stack_composition": _default_stack_composition(),
            "registry": registry,
            "benchmark_ids": ["ifeval_subset"],
            "dataset_hashes": [_VALID_HASH],
            "grader_hashes": [_VALID_HASH],
            "prompt_serialization_hash": _VALID_HASH,
            "task_ids": ["task-001"],
            "repetitions": 1,
            "primary_metrics": ["ifeval_subset_verifier"],
            "hardware_identity": "linux/amd64/rtx-4090",
            "environment_stratum": "single-machine",
        }
        bundle_a = build_heterogeneous_stack_profile(
            stack_label="best-accuracy", **common_kwargs
        )
        bundle_b = build_heterogeneous_stack_profile(
            stack_label="best-efficiency", **common_kwargs
        )
        assert bundle_a.role_combination_track.content_hash != bundle_b.role_combination_track.content_hash
        assert bundle_a.campaign_profile.content_hash == bundle_b.campaign_profile.content_hash


class TestBuildHeterogeneousStackProfileValidation:
    def test_rejects_empty_stack_label(self):
        with pytest.raises(ValueError, match="stack_label must be non-empty"):
            _build_bundle(stack_label="")

    def test_rejects_missing_role_key_in_stack_composition(self):
        with pytest.raises(ValueError, match="stack_composition must have exactly keys"):
            _build_bundle(stack_composition={
                "primary": "qwen3-8b",
                "assistant": "phi4-mini",
            })

    def test_rejects_extra_role_key_in_stack_composition(self):
        with pytest.raises(ValueError, match="stack_composition must have exactly keys"):
            _build_bundle(stack_composition={
                "primary": "qwen3-8b",
                "assistant": "phi4-mini",
                "lite": "smollm2-360m",
                "extra": "foo",
            })

    def test_rejects_non_distinct_variant_ids(self):
        with pytest.raises(ValueError, match="stack composition variant IDs must be distinct"):
            _build_bundle(stack_composition={
                "primary": "qwen3-8b",
                "assistant": "qwen3-8b",
                "lite": "smollm2-360m",
            })

    def test_rejects_all_three_same_variant(self):
        with pytest.raises(ValueError, match="stack composition variant IDs must be distinct"):
            _build_bundle(stack_composition={
                "primary": "qwen3-8b",
                "assistant": "qwen3-8b",
                "lite": "qwen3-8b",
            })

    def test_rejects_variant_not_in_registry(self):
        with pytest.raises(ValueError, match="not found in registry"):
            _build_bundle(stack_composition={
                "primary": "nonexistent-model",
                "assistant": "phi4-mini",
                "lite": "smollm2-360m",
            })

    def test_rejects_role_exercise_declaration_missing_task(self):
        with pytest.raises(ValueError, match="role_exercise_declarations missing task"):
            _build_bundle(
                task_ids=["task-001", "task-002"],
                role_exercise_declarations={"task-001": ["lite"]},
            )


class TestBuildHeterogeneousStackProfileAssignmentMatrix:
    def test_total_assignments_matches_variants_times_tasks_times_repetitions(self):
        task_ids = ["task-001", "task-002", "task-003"]
        repetitions = 5
        bundle = _build_bundle(task_ids=task_ids, repetitions=repetitions)
        profile = bundle.campaign_profile
        num_variants = len(profile.generative_variant_ids)
        num_tasks = len(profile.task_ids)
        num_tracks = len(profile.track_arm_assignments)
        total = num_variants * num_tasks * num_tracks * repetitions
        assert total == 3 * 3 * 1 * 5

    def test_each_variant_has_exactly_one_tier_assignment(self):
        bundle = _build_bundle()
        profile = bundle.campaign_profile
        for vid in profile.generative_variant_ids:
            assignments = [a for a in profile.model_tier_assignments if a.variant_id == vid]
            assert len(assignments) == 1

    def test_all_three_roles_covered_in_tier_assignments(self):
        bundle = _build_bundle()
        roles_used = {a.target_tier for a in bundle.campaign_profile.model_tier_assignments}
        assert roles_used == {"primary", "assistant", "lite"}

    def test_combination_count_is_one(self):
        bundle = _build_bundle()
        assert len(bundle.role_combination_track.combinations) == 1

    def test_task_binding_count_equals_tasks_times_one_combination(self):
        task_ids = ["t1", "t2", "t3", "t4", "t5"]
        bundle = _build_bundle(task_ids=task_ids)
        num_combos = len(bundle.role_combination_track.combinations)
        expected = len(task_ids) * num_combos
        assert len(bundle.role_combination_track.task_role_bindings) == expected
