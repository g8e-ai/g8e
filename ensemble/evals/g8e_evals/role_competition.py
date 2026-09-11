# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Role-competition campaign profile builder.

Constructs a frozen ``CampaignProfile`` and ``RoleCombinationTrack``
from role candidate selections and fixed baselines. Each candidate
competes for exactly one role (primary, assistant, or lite) through
the ensemble (``G8eeChatSUT``) using the existing ``model_tier_assignments``
and ``baseline_tier_mappings`` tier-fitness infrastructure. Fixed
baseline models fill the non-target roles.

The builder validates three constraints before constructing any frozen
artifact:

1. The three baseline variant IDs must be distinct (they appear
   together in role combinations).
2. No candidate variant ID may duplicate a baseline variant ID (the
   candidate appears in a combination with two baselines, and
   ``RoleCombinationId`` requires all three variant IDs to be distinct).
3. No candidate may appear in more than one role group (each variant
   gets exactly one ``ModelTierAssignment``, and the
   ``model_tier_assignments`` validator rejects duplicate variant IDs).

The builder also validates that every candidate and baseline variant ID
exists in the model registry, resolves baseline variant IDs to served
model tags for ``baseline_tier_mappings``, and produces a
``RoleCombinationTrack`` with one ``RoleCombination`` per candidate and
``TaskRoleBinding`` records for the Cartesian product of tasks and
combinations.
"""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass

from g8e_evals.analysis_contract import (
    ROLE_COMBINATION_TRACK_VERSION,
    RoleCombination,
    RoleCombinationId,
    RoleCombinationTrack,
    RoleExerciseDeclaration,
    TaskRoleBinding,
    compute_role_track_hash,
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
from g8e_evals.registry import ModelRegistry
from g8e_evals.schema import CampaignTrack

_VALID_ROLES = ("primary", "assistant", "lite")


@dataclass(frozen=True)
class RoleCompetitionProfileBundle:
    """Frozen role-competition profile bundle.

    Contains the frozen ``CampaignProfile`` (with
    ``model_tier_assignments``, ``baseline_tier_mappings``, and claim
    boundary) and the frozen ``RoleCombinationTrack`` (with exact
    Primary/Assistant/Lite combinations for each candidate). Both
    artifacts are content-hashed and immutable.
    """

    campaign_profile: CampaignProfile
    role_combination_track: RoleCombinationTrack


def build_role_competition_profile(
    *,
    campaign_id: str,
    campaign_revision: str,
    purpose: str,
    created_at: str,
    role_candidates: dict[str, list[str]],
    baselines: dict[str, str],
    registry: ModelRegistry,
    benchmark_ids: list[str],
    dataset_hashes: list[str],
    grader_hashes: list[str],
    prompt_serialization_hash: str,
    task_ids: list[str],
    repetitions: int,
    primary_metrics: list[str],
    hardware_identity: str,
    environment_stratum: str,
    unit_of_analysis: str = "task",
    temperature: float = 0.0,
    top_p: float = 1.0,
    max_tokens: int = 1024,
    seed: int = 42,
    context_limit: int = 2048,
    timeout_seconds: float = 120.0,
    max_retries: int = 1,
    warmup_excluded: bool = True,
    concurrency: int = 1,
    claim_boundary: ClaimBoundary = ClaimBoundary.DESCRIPTIVE_ONLY,
    routing_policy: str = "default",
    track_id: str = "role-competition-track",
    track_description: str = (
        "Role-competition track for the evaluation framework v2 campaign"
    ),
    role_exercise_declarations: dict[str, list[str]] | None = None,
) -> RoleCompetitionProfileBundle:
    """Build a frozen role-competition campaign profile and role-combination track.

    Each candidate in ``role_candidates`` is assigned to its candidate
    role via ``model_tier_assignments``. Fixed baselines from
    ``baselines`` fill the non-target roles via
    ``baseline_tier_mappings`` (resolved to served model tags through
    the registry). The ``RoleCombinationTrack`` gets one
    ``RoleCombination`` per candidate and ``TaskRoleBinding`` records
    for every task-combination pair.

    Raises ``ValueError`` when: a role key is missing or unexpected,
    the three baseline variant IDs are not distinct, a candidate
    duplicates a baseline variant ID, a candidate appears in more than
    one role group, a candidate duplicates within a single role group,
    any variant ID is absent from the registry, or no candidates are
    provided.
    """
    _validate_role_keys(role_candidates, "role_candidates")
    _validate_role_keys(baselines, "baselines")

    baseline_ids = [baselines[r] for r in _VALID_ROLES]
    if len(baseline_ids) != len(set(baseline_ids)):
        raise ValueError(
            f"baseline variant IDs must be distinct: {baseline_ids}"
        )

    candidate_roles: dict[str, str] = {}
    all_candidates: list[str] = []
    for role in _VALID_ROLES:
        group = role_candidates[role]
        if len(group) != len(set(group)):
            seen: set[str] = set()
            dupes: list[str] = []
            for vid in group:
                if vid in seen:
                    dupes.append(vid)
                seen.add(vid)
            raise ValueError(
                f"duplicate candidate in {role} role group: "
                f"{sorted(set(dupes))}"
            )
        for vid in group:
            if vid in candidate_roles:
                raise ValueError(
                    f"candidate {vid!r} appears in multiple role groups: "
                    f"{candidate_roles[vid]!r} and {role!r}"
                )
            candidate_roles[vid] = role
            all_candidates.append(vid)

    if not all_candidates:
        raise ValueError("at least one role candidate is required")

    baseline_set = set(baseline_ids)
    for vid in all_candidates:
        if vid in baseline_set:
            raise ValueError(
                f"candidate {vid!r} duplicates a baseline variant ID"
            )

    registry_ids = {v.variant_id for v in registry.variants}
    for vid in all_candidates + baseline_ids:
        if vid not in registry_ids:
            raise ValueError(f"variant {vid!r} not found in registry")

    baseline_tier_mappings: dict[str, str] = {}
    for role in _VALID_ROLES:
        variant = registry.get_variant(baselines[role])
        baseline_tier_mappings[role] = variant.served_model_tag

    model_tier_assignments = [
        ModelTierAssignment(variant_id=vid, target_tier=role)
        for role in _VALID_ROLES
        for vid in role_candidates[role]
    ]

    combinations: list[RoleCombination] = []
    for role in _VALID_ROLES:
        for vid in role_candidates[role]:
            combinations.append(
                _make_combination(vid, role, baselines)
            )

    declarations = _build_role_exercise_declarations(
        task_ids, role_exercise_declarations
    )

    task_role_bindings: list[TaskRoleBinding] = []
    for tid in task_ids:
        for combo in combinations:
            task_role_bindings.append(
                TaskRoleBinding(
                    task_id=tid,
                    combination_id=combo.combination_id.combination_id,
                    primary_variant_id=combo.combination_id.primary_variant_id,
                    assistant_variant_id=combo.combination_id.assistant_variant_id,
                    lite_variant_id=combo.combination_id.lite_variant_id,
                )
            )

    track_hash = compute_role_track_hash(
        track_id=track_id,
        track_version=ROLE_COMBINATION_TRACK_VERSION,
        description=track_description,
        combinations=combinations,
        role_exercise_declarations=declarations,
        task_role_bindings=task_role_bindings,
    )
    role_combination_track = RoleCombinationTrack(
        track_id=track_id,
        track_version=ROLE_COMBINATION_TRACK_VERSION,
        description=track_description,
        combinations=combinations,
        role_exercise_declarations=declarations,
        task_role_bindings=task_role_bindings,
        content_hash=track_hash,
    )

    generative_variant_ids = sorted(set(all_candidates))
    track_arm_assignments = [
        TrackArmAssignment(
            track=CampaignTrack.TIER_FITNESS,
            arm_id="ensemble_ungoverned",
        ),
    ]

    temp_profile = CampaignProfile.model_construct(
        campaign_id=campaign_id,
        campaign_revision=campaign_revision,
        schema_version=CAMPAIGN_PROFILE_VERSION,
        purpose=purpose,
        created_at=created_at,
        lifecycle_status=CampaignLifecycleStatus.FROZEN,
        generative_variant_ids=generative_variant_ids,
        benchmark_ids=benchmark_ids,
        dataset_hashes=dataset_hashes,
        grader_hashes=grader_hashes,
        prompt_serialization_hash=prompt_serialization_hash,
        task_ids=task_ids,
        repetitions=repetitions,
        track_arm_assignments=track_arm_assignments,
        model_tier_assignments=model_tier_assignments,
        baseline_tier_mappings=baseline_tier_mappings,
        routing_policy=routing_policy,
        temperature=temperature,
        top_p=top_p,
        max_tokens=max_tokens,
        seed=seed,
        context_limit=context_limit,
        timeout_seconds=timeout_seconds,
        max_retries=max_retries,
        warmup_excluded=warmup_excluded,
        concurrency=concurrency,
        hardware_identity=hardware_identity,
        environment_stratum=environment_stratum,
        primary_metrics=primary_metrics,
        unit_of_analysis=unit_of_analysis,
        claim_boundary=claim_boundary,
        model_registry_hash=registry.content_hash,
        content_hash="0" * 64,
    )
    profile_hash = compute_campaign_profile_hash(temp_profile)

    campaign_profile = CampaignProfile(
        campaign_id=campaign_id,
        campaign_revision=campaign_revision,
        schema_version=CAMPAIGN_PROFILE_VERSION,
        purpose=purpose,
        created_at=created_at,
        lifecycle_status=CampaignLifecycleStatus.FROZEN,
        generative_variant_ids=generative_variant_ids,
        benchmark_ids=benchmark_ids,
        dataset_hashes=dataset_hashes,
        grader_hashes=grader_hashes,
        prompt_serialization_hash=prompt_serialization_hash,
        task_ids=task_ids,
        repetitions=repetitions,
        track_arm_assignments=track_arm_assignments,
        model_tier_assignments=model_tier_assignments,
        baseline_tier_mappings=baseline_tier_mappings,
        routing_policy=routing_policy,
        temperature=temperature,
        top_p=top_p,
        max_tokens=max_tokens,
        seed=seed,
        context_limit=context_limit,
        timeout_seconds=timeout_seconds,
        max_retries=max_retries,
        warmup_excluded=warmup_excluded,
        concurrency=concurrency,
        hardware_identity=hardware_identity,
        environment_stratum=environment_stratum,
        primary_metrics=primary_metrics,
        unit_of_analysis=unit_of_analysis,
        claim_boundary=claim_boundary,
        model_registry_hash=registry.content_hash,
        content_hash=profile_hash,
    )

    return RoleCompetitionProfileBundle(
        campaign_profile=campaign_profile,
        role_combination_track=role_combination_track,
    )


def _validate_role_keys(mapping: Mapping[str, object], name: str) -> None:
    expected = set(_VALID_ROLES)
    actual = set(mapping.keys())
    if actual != expected:
        raise ValueError(
            f"{name} must have exactly keys {sorted(expected)}: "
            f"got {sorted(actual)}"
        )


def _make_combination(
    candidate_vid: str,
    candidate_role: str,
    baselines: dict[str, str],
) -> RoleCombination:
    roles: dict[str, str] = {
        "primary": baselines["primary"],
        "assistant": baselines["assistant"],
        "lite": baselines["lite"],
    }
    roles[candidate_role] = candidate_vid
    combo_id = (
        f"combo-primary-{roles['primary']}"
        f"-assistant-{roles['assistant']}"
        f"-lite-{roles['lite']}"
    )
    return RoleCombination(
        combination_id=RoleCombinationId(
            combination_id=combo_id,
            primary_variant_id=roles["primary"],
            assistant_variant_id=roles["assistant"],
            lite_variant_id=roles["lite"],
        ),
        rationale=f"Candidate {candidate_vid} competing for {candidate_role} role",
    )


def _build_role_exercise_declarations(
    task_ids: list[str],
    role_exercise_declarations: dict[str, list[str]] | None,
) -> list[RoleExerciseDeclaration]:
    if role_exercise_declarations is not None:
        declarations: list[RoleExerciseDeclaration] = []
        for tid in task_ids:
            roles = role_exercise_declarations.get(tid)
            if roles is None:
                raise ValueError(
                    f"role_exercise_declarations missing task {tid!r}"
                )
            declarations.append(
                RoleExerciseDeclaration(
                    task_id=tid,
                    exercised_roles=sorted(roles),
                )
            )
        return declarations
    return [
        RoleExerciseDeclaration(
            task_id=tid,
            exercised_roles=sorted(_VALID_ROLES),
        )
        for tid in task_ids
    ]


__all__ = [
    "RoleCompetitionProfileBundle",
    "build_role_competition_profile",
]
