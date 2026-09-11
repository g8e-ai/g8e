# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for the role-competition campaign profile builder.

Verifies that ``build_role_competition_profile`` constructs a frozen
``CampaignProfile`` and ``RoleCombinationTrack`` from role candidate
selections and fixed baselines, validates all constraints (distinct
baselines, no candidate-baseline collision, no candidate in multiple
role groups, all variant IDs in the registry), resolves baseline
variant IDs to served model tags, produces the correct
``model_tier_assignments``, ``baseline_tier_mappings``,
``RoleCombination`` entries, and ``TaskRoleBinding`` records, and
that both frozen artifacts validate and round-trip through JSON. No
external dependencies (no files, network, or DB).
"""

from __future__ import annotations

import pytest

from g8e_evals.profile import (
    CAMPAIGN_PROFILE_VERSION,
    CampaignLifecycleStatus,
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
from g8e_evals.role_competition import (
    RoleCompetitionProfileBundle,
    build_role_competition_profile,
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
    ]


def _default_role_candidates() -> dict[str, list[str]]:
    return {
        "primary": ["qwen3-8b", "phi4-mini"],
        "assistant": ["qwen3-4b"],
        "lite": ["qwen3-0.6b"],
    }


def _default_baselines() -> dict[str, str]:
    return {
        "primary": "granite-8b",
        "assistant": "granite-3b",
        "lite": "smollm2-360m",
    }


def _build_bundle(
    variants: list[ModelVariant] | None = None,
    role_candidates: dict[str, list[str]] | None = None,
    baselines: dict[str, str] | None = None,
    task_ids: list[str] | None = None,
    repetitions: int = 5,
    role_exercise_declarations: dict[str, list[str]] | None = None,
) -> RoleCompetitionProfileBundle:
    if variants is None:
        variants = _default_variants()
    if role_candidates is None:
        role_candidates = _default_role_candidates()
    if baselines is None:
        baselines = _default_baselines()
    if task_ids is None:
        task_ids = ["task-001", "task-002", "task-003"]
    registry = _make_registry(variants)
    return build_role_competition_profile(
        campaign_id="role-competition-v1",
        campaign_revision="rev-1",
        purpose="Role-competition campaign for evaluation framework v2",
        created_at="2026-09-10T00:00:00Z",
        role_candidates=role_candidates,
        baselines=baselines,
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
    )


class TestBuildRoleCompetitionProfileSuccess:
    def test_returns_bundle_with_profile_and_track(self):
        bundle = _build_bundle()
        assert isinstance(bundle, RoleCompetitionProfileBundle)
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

    def test_generative_variant_ids_are_candidates_only(self):
        bundle = _build_bundle()
        expected = sorted({"qwen3-8b", "phi4-mini", "qwen3-4b", "qwen3-0.6b"})
        assert bundle.campaign_profile.generative_variant_ids == expected

    def test_generative_variant_ids_exclude_baselines(self):
        bundle = _build_bundle()
        # Baselines are granite-8b, granite-3b, smollm2-360m — none are candidates.
        assert "granite-8b" not in bundle.campaign_profile.generative_variant_ids
        assert "granite-3b" not in bundle.campaign_profile.generative_variant_ids
        assert "smollm2-360m" not in bundle.campaign_profile.generative_variant_ids
        assert bundle.campaign_profile.baseline_tier_mappings["primary"] == "granite:8b"

    def test_model_tier_assignments_one_per_candidate(self):
        bundle = _build_bundle()
        assignments = bundle.campaign_profile.model_tier_assignments
        assert len(assignments) == 4
        by_variant = {a.variant_id: a.target_tier for a in assignments}
        assert by_variant["qwen3-8b"] == "primary"
        assert by_variant["phi4-mini"] == "primary"
        assert by_variant["qwen3-4b"] == "assistant"
        assert by_variant["qwen3-0.6b"] == "lite"

    def test_baseline_tier_mappings_resolved_to_served_model_tags(self):
        bundle = _build_bundle()
        mappings = bundle.campaign_profile.baseline_tier_mappings
        assert mappings["primary"] == "granite:8b"
        assert mappings["assistant"] == "granite:3b"
        assert mappings["lite"] == "smollm2:360m"

    def test_track_arm_assignment_is_tier_fitness_ensemble(self):
        bundle = _build_bundle()
        assert len(bundle.campaign_profile.track_arm_assignments) == 1
        taa = bundle.campaign_profile.track_arm_assignments[0]
        assert taa.track == CampaignTrack.TIER_FITNESS
        assert taa.arm_id == "ensemble_ungoverned"

    def test_claim_boundary_defaults_to_descriptive_only(self):
        bundle = _build_bundle()
        assert bundle.campaign_profile.claim_boundary == ClaimBoundary.DESCRIPTIVE_ONLY

    def test_role_combination_track_has_one_combination_per_candidate(self):
        bundle = _build_bundle()
        assert len(bundle.role_combination_track.combinations) == 4

    def test_role_combination_track_combination_ids_are_unique(self):
        bundle = _build_bundle()
        combo_ids = [c.combination_id.combination_id for c in bundle.role_combination_track.combinations]
        assert len(combo_ids) == len(set(combo_ids))

    def test_primary_candidate_combination_has_candidate_in_primary_role(self):
        bundle = _build_bundle()
        qwen_combo = None
        for c in bundle.role_combination_track.combinations:
            if c.combination_id.primary_variant_id == "qwen3-8b":
                qwen_combo = c
                break
        assert qwen_combo is not None
        assert qwen_combo.combination_id.primary_variant_id == "qwen3-8b"
        assert qwen_combo.combination_id.assistant_variant_id == "granite-3b"
        assert qwen_combo.combination_id.lite_variant_id == "smollm2-360m"

    def test_assistant_candidate_combination_has_candidate_in_assistant_role(self):
        bundle = _build_bundle()
        found = False
        for c in bundle.role_combination_track.combinations:
            if c.combination_id.assistant_variant_id == "qwen3-4b":
                assert c.combination_id.primary_variant_id == "granite-8b"
                assert c.combination_id.lite_variant_id == "smollm2-360m"
                found = True
                break
        assert found

    def test_lite_candidate_combination_has_candidate_in_lite_role(self):
        bundle = _build_bundle()
        found = False
        for c in bundle.role_combination_track.combinations:
            if c.combination_id.lite_variant_id == "qwen3-0.6b":
                assert c.combination_id.primary_variant_id == "granite-8b"
                assert c.combination_id.assistant_variant_id == "granite-3b"
                found = True
                break
        assert found

    def test_all_combination_variant_ids_are_distinct(self):
        bundle = _build_bundle()
        for c in bundle.role_combination_track.combinations:
            ids = [
                c.combination_id.primary_variant_id,
                c.combination_id.assistant_variant_id,
                c.combination_id.lite_variant_id,
            ]
            assert len(ids) == len(set(ids))

    def test_task_role_bindings_are_cartesian_product(self):
        task_ids = ["task-001", "task-002", "task-003"]
        bundle = _build_bundle(task_ids=task_ids)
        num_combos = len(bundle.role_combination_track.combinations)
        expected_bindings = len(task_ids) * num_combos
        assert len(bundle.role_combination_track.task_role_bindings) == expected_bindings

    def test_task_role_binding_variant_ids_match_combination(self):
        bundle = _build_bundle()
        combo_map = {
            c.combination_id.combination_id: c
            for c in bundle.role_combination_track.combinations
        }
        for binding in bundle.role_combination_track.task_role_bindings:
            combo = combo_map[binding.combination_id]
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
        import json
        from g8e_evals.profile import CampaignProfile
        data = json.loads(bundle.campaign_profile.model_dump_json())
        restored = CampaignProfile.model_validate(data)
        assert restored.content_hash == bundle.campaign_profile.content_hash
        assert restored.generative_variant_ids == bundle.campaign_profile.generative_variant_ids

    def test_role_combination_track_round_trips_through_json(self):
        bundle = _build_bundle()
        import json
        from g8e_evals.analysis_contract import RoleCombinationTrack
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
        variants = _default_variants()
        registry = _make_registry(variants)
        bundle = build_role_competition_profile(
            campaign_id="role-competition-v1",
            campaign_revision="rev-1",
            purpose="Role-competition campaign",
            created_at="2026-09-10T00:00:00Z",
            role_candidates=_default_role_candidates(),
            baselines=_default_baselines(),
            registry=registry,
            benchmark_ids=["ifeval_subset"],
            dataset_hashes=[_VALID_HASH],
            grader_hashes=[_VALID_HASH],
            prompt_serialization_hash=_VALID_HASH,
            task_ids=["task-001"],
            repetitions=3,
            primary_metrics=["ifeval_subset_verifier"],
            hardware_identity="linux/amd64/rtx-4090",
            environment_stratum="single-machine",
            routing_policy="cost-aware",
        )
        assert bundle.campaign_profile.routing_policy == "cost-aware"

    def test_custom_claim_boundary(self):
        variants = _default_variants()
        registry = _make_registry(variants)
        bundle = build_role_competition_profile(
            campaign_id="role-competition-v1",
            campaign_revision="rev-1",
            purpose="Role-competition campaign",
            created_at="2026-09-10T00:00:00Z",
            role_candidates=_default_role_candidates(),
            baselines=_default_baselines(),
            registry=registry,
            benchmark_ids=["ifeval_subset"],
            dataset_hashes=[_VALID_HASH],
            grader_hashes=[_VALID_HASH],
            prompt_serialization_hash=_VALID_HASH,
            task_ids=["task-001"],
            repetitions=3,
            primary_metrics=["ifeval_subset_verifier"],
            hardware_identity="linux/amd64/rtx-4090",
            environment_stratum="single-machine",
            claim_boundary=ClaimBoundary.CONFIRMATORY,
        )
        assert bundle.campaign_profile.claim_boundary == ClaimBoundary.CONFIRMATORY


class TestBuildRoleCompetitionProfileValidation:
    def test_rejects_missing_role_key_in_candidates(self):
        with pytest.raises(ValueError, match="role_candidates must have exactly keys"):
            _build_bundle(role_candidates={"primary": ["qwen3-8b"], "assistant": ["qwen3-4b"]})

    def test_rejects_extra_role_key_in_candidates(self):
        with pytest.raises(ValueError, match="role_candidates must have exactly keys"):
            _build_bundle(role_candidates={
                "primary": ["qwen3-8b"],
                "assistant": ["qwen3-4b"],
                "lite": ["qwen3-0.6b"],
                "extra": ["foo"],
            })

    def test_rejects_missing_role_key_in_baselines(self):
        with pytest.raises(ValueError, match="baselines must have exactly keys"):
            _build_bundle(baselines={"primary": "qwen3-8b", "assistant": "qwen3-4b"})

    def test_rejects_non_distinct_baselines(self):
        with pytest.raises(ValueError, match="baseline variant IDs must be distinct"):
            _build_bundle(baselines={
                "primary": "qwen3-8b",
                "assistant": "qwen3-8b",
                "lite": "qwen3-0.6b",
            })

    def test_rejects_candidate_equal_to_baseline(self):
        # qwen3-8b is both a primary candidate and the primary baseline
        with pytest.raises(ValueError, match="duplicates a baseline variant ID"):
            _build_bundle(
                role_candidates={
                    "primary": ["phi4-mini"],
                    "assistant": ["qwen3-4b"],
                    "lite": ["qwen3-0.6b"],
                },
                baselines={
                    "primary": "phi4-mini",
                    "assistant": "qwen3-4b",
                    "lite": "qwen3-0.6b",
                },
            )

    def test_rejects_candidate_in_multiple_role_groups(self):
        with pytest.raises(ValueError, match="appears in multiple role groups"):
            _build_bundle(
                role_candidates={
                    "primary": ["qwen3-8b", "phi4-mini"],
                    "assistant": ["qwen3-8b", "qwen3-4b"],
                    "lite": ["qwen3-0.6b"],
                },
                baselines={
                    "primary": "granite-8b",
                    "assistant": "granite-3b",
                    "lite": "smollm2-360m",
                },
            )

    def test_rejects_duplicate_candidate_within_role_group(self):
        with pytest.raises(ValueError, match="duplicate candidate in primary role group"):
            _build_bundle(
                role_candidates={
                    "primary": ["qwen3-8b", "qwen3-8b"],
                    "assistant": ["qwen3-4b"],
                    "lite": ["qwen3-0.6b"],
                },
                baselines={
                    "primary": "granite-8b",
                    "assistant": "granite-3b",
                    "lite": "smollm2-360m",
                },
            )

    def test_rejects_candidate_not_in_registry(self):
        with pytest.raises(ValueError, match="not found in registry"):
            _build_bundle(
                role_candidates={
                    "primary": ["nonexistent-model"],
                    "assistant": ["qwen3-4b"],
                    "lite": ["qwen3-0.6b"],
                },
                baselines=_default_baselines(),
            )

    def test_rejects_baseline_not_in_registry(self):
        with pytest.raises(ValueError, match="not found in registry"):
            _build_bundle(
                role_candidates={
                    "primary": ["qwen3-8b"],
                    "assistant": ["qwen3-4b"],
                    "lite": ["qwen3-0.6b"],
                },
                baselines={
                    "primary": "nonexistent-baseline",
                    "assistant": "nonexistent-baseline2",
                    "lite": "nonexistent-baseline3",
                },
            )

    def test_rejects_no_candidates(self):
        with pytest.raises(ValueError, match="at least one role candidate is required"):
            _build_bundle(
                role_candidates={
                    "primary": [],
                    "assistant": [],
                    "lite": [],
                },
                baselines=_default_baselines(),
            )

    def test_rejects_role_exercise_declaration_missing_task(self):
        with pytest.raises(ValueError, match="role_exercise_declarations missing task"):
            _build_bundle(
                task_ids=["task-001", "task-002"],
                role_exercise_declarations={"task-001": ["lite"]},
            )


class TestBuildRoleCompetitionProfileAssignmentMatrix:
    def test_total_assignments_matches_candidates_times_tasks_times_repetitions(self):
        task_ids = ["task-001", "task-002", "task-003"]
        repetitions = 5
        bundle = _build_bundle(task_ids=task_ids, repetitions=repetitions)
        profile = bundle.campaign_profile
        num_variants = len(profile.generative_variant_ids)
        num_tasks = len(profile.task_ids)
        num_tracks = len(profile.track_arm_assignments)
        total = num_variants * num_tasks * num_tracks * repetitions
        assert total == 4 * 3 * 1 * 5

    def test_each_candidate_has_exactly_one_tier_assignment(self):
        bundle = _build_bundle()
        profile = bundle.campaign_profile
        for vid in profile.generative_variant_ids:
            assignments = [a for a in profile.model_tier_assignments if a.variant_id == vid]
            assert len(assignments) == 1

    def test_candidate_role_mapping_covers_all_three_roles(self):
        bundle = _build_bundle()
        roles_used = {a.target_tier for a in bundle.campaign_profile.model_tier_assignments}
        assert roles_used == {"primary", "assistant", "lite"}

    def test_combination_count_equals_candidate_count(self):
        bundle = _build_bundle()
        num_candidates = len(bundle.campaign_profile.generative_variant_ids)
        num_combos = len(bundle.role_combination_track.combinations)
        assert num_combos == num_candidates

    def test_task_binding_count_equals_tasks_times_combinations(self):
        task_ids = ["t1", "t2", "t3", "t4", "t5"]
        bundle = _build_bundle(task_ids=task_ids)
        num_combos = len(bundle.role_combination_track.combinations)
        expected = len(task_ids) * num_combos
        assert len(bundle.role_combination_track.task_role_bindings) == expected
