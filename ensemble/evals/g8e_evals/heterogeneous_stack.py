# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Heterogeneous-stack campaign profile builder.

Constructs a frozen ``CampaignProfile`` and ``RoleCombinationTrack``
from a heterogeneous stack composition. Each stack assigns all three
roles (primary, assistant, lite) to specific model variants. There is
no tier replacement and no baseline filling — all three roles are
explicitly assigned. The stack runs the complete 25-scenario
campaign end to end through the ensemble (``G8eeChatSUT``).

The builder validates three constraints before constructing any frozen
artifact:

1. The stack composition must have exactly the three role keys
   (primary, assistant, lite).
2. The three variant IDs must be distinct (``RoleCombinationId``
   requires all three variant IDs to be distinct).
3. Every variant ID must exist in the model registry.

The role-competition profile builder (``role_competition.py``) runs
each candidate through one role with fixed baselines filling the other
two. This builder runs one complete stack with all three roles
explicitly assigned, producing a single ``RoleCombination`` for the
entire stack.
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
)
from g8e_evals.registry import ModelRegistry
from g8e_evals.schema import CampaignTrack
from g8e_evals.serialization import provisional_hash

_VALID_ROLES = ("primary", "assistant", "lite")


@dataclass(frozen=True)
class HeterogeneousStackProfileBundle:
    """Frozen heterogeneous-stack profile bundle.

    Contains the frozen ``CampaignProfile`` (with
    ``model_tier_assignments`` for all three roles and empty
    ``baseline_tier_mappings``) and the frozen ``RoleCombinationTrack``
    (with one combination representing the complete stack). Both
    artifacts are content-hashed and immutable.
    """

    campaign_profile: CampaignProfile
    role_combination_track: RoleCombinationTrack


def build_heterogeneous_stack_profile(
    *,
    campaign_id: str,
    campaign_revision: str,
    purpose: str,
    created_at: str,
    stack_composition: dict[str, str],
    stack_label: str,
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
    track_id: str | None = None,
    track_description: str | None = None,
    role_exercise_declarations: dict[str, list[str]] | None = None,
) -> HeterogeneousStackProfileBundle:
    """Build a frozen heterogeneous-stack campaign profile and role-combination track.

    All three roles (primary, assistant, lite) are explicitly assigned
    to specific model variants from the stack composition. There is no
    tier replacement and no baseline filling. The
    ``RoleCombinationTrack`` gets one ``RoleCombination`` representing
    the complete stack and ``TaskRoleBinding`` records for every
    task-combination pair.

    The ``stack_label`` identifies the stack (e.g. ``"best-accuracy"``,
    ``"all-qwen"``) and is embedded in the track ID and combination
    rationale for traceability.

    Raises ``ValueError`` when: ``stack_label`` is empty, a role key is
    missing or unexpected in the stack composition, the three variant
    IDs are not distinct, any variant ID is absent from the registry,
    or a role-exercise declaration is missing for a task.
    """
    if not stack_label:
        raise ValueError("stack_label must be non-empty")

    _validate_role_keys(stack_composition, "stack_composition")

    variant_ids = [stack_composition[r] for r in _VALID_ROLES]
    if len(variant_ids) != len(set(variant_ids)):
        raise ValueError(
            f"stack composition variant IDs must be distinct: {variant_ids}"
        )

    registry_ids = {v.variant_id for v in registry.variants}
    for vid in variant_ids:
        if vid not in registry_ids:
            raise ValueError(f"variant {vid!r} not found in registry")

    model_tier_assignments = [
        ModelTierAssignment(
            variant_id=stack_composition[role], target_tier=role
        )
        for role in _VALID_ROLES
    ]

    combination_id = (
        f"stack-{stack_label}"
        f"-primary-{variant_ids[0]}"
        f"-assistant-{variant_ids[1]}"
        f"-lite-{variant_ids[2]}"
    )
    combination = RoleCombination(
        combination_id=RoleCombinationId(
            combination_id=combination_id,
            primary_variant_id=variant_ids[0],
            assistant_variant_id=variant_ids[1],
            lite_variant_id=variant_ids[2],
        ),
        rationale=(
            f"Heterogeneous stack {stack_label!r}: "
            f"primary={variant_ids[0]}, "
            f"assistant={variant_ids[1]}, "
            f"lite={variant_ids[2]}"
        ),
    )
    combinations = [combination]

    declarations = _build_role_exercise_declarations(
        task_ids, role_exercise_declarations
    )

    task_role_bindings: list[TaskRoleBinding] = []
    for tid in task_ids:
        task_role_bindings.append(
            TaskRoleBinding(
                task_id=tid,
                combination_id=combination.combination_id.combination_id,
                primary_variant_id=combination.combination_id.primary_variant_id,
                assistant_variant_id=combination.combination_id.assistant_variant_id,
                lite_variant_id=combination.combination_id.lite_variant_id,
            )
        )

    resolved_track_id = track_id or f"stack-{stack_label}-track"
    resolved_track_description = track_description or (
        f"Heterogeneous stack {stack_label!r} track "
        f"for the evaluation framework v2 campaign"
    )

    track_hash = compute_role_track_hash(
        track_id=resolved_track_id,
        track_version=ROLE_COMBINATION_TRACK_VERSION,
        description=resolved_track_description,
        combinations=combinations,
        role_exercise_declarations=declarations,
        task_role_bindings=task_role_bindings,
    )
    role_combination_track = RoleCombinationTrack(
        track_id=resolved_track_id,
        track_version=ROLE_COMBINATION_TRACK_VERSION,
        description=resolved_track_description,
        combinations=combinations,
        role_exercise_declarations=declarations,
        task_role_bindings=task_role_bindings,
        content_hash=track_hash,
    )

    generative_variant_ids = sorted(set(variant_ids))
    track_arm_assignments = [
        TrackArmAssignment(
            track=CampaignTrack.TIER_FITNESS,
            arm_id="ensemble_ungoverned",
        ),
    ]

    profile_hash = provisional_hash(
        CampaignProfile,
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
        baseline_tier_mappings={},
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
    )

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
        baseline_tier_mappings={},
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

    return HeterogeneousStackProfileBundle(
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
    "HeterogeneousStackProfileBundle",
    "build_heterogeneous_stack_profile",
]
