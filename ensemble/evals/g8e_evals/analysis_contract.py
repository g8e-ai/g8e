# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Frozen statistical analysis contract and role-combination track for
the campaign.

The statistical analysis contract freezes the analysis approach before
measured collection: task as the primary independent sampling unit,
paired task-level comparisons, the exact paired binary procedure
(McNemar) for pass/fail outcomes, the paired task-cluster bootstrap for
continuous or repeated outcomes, preregistered multiple-comparison
correction per declared family, paired effect sizes and uncertainty
intervals, Pareto views for quality versus latency/memory/artifact size,
and descriptive labeling when the population is too small for
inferential procedures.

The role-combination track defines exact Primary/Assistant/Lite
assignments, freezes a tractable combination matrix and rationale rather
than silently attempting an unbounded Cartesian product, declares which
roles each benchmark task exercises, and binds metrics to both the exact
three-role combination and the individual invoked variant. Results from
different combinations never pool implicitly.

Every model is frozen with ``extra="forbid"``. Content hashes are
SHA-256 over canonical JSON (sorted keys, no extra whitespace).
"""

from __future__ import annotations

import hashlib
import json
from enum import StrEnum
from typing import Self

from pydantic import BaseModel, ConfigDict, Field, model_validator


ANALYSIS_CONTRACT_VERSION = "1.0.0"
ROLE_COMBINATION_TRACK_VERSION = "1.0.0"

_VALID_TIERS = frozenset({"primary", "assistant", "lite"})


def _sha256(data: str) -> str:
    return hashlib.sha256(data.encode()).hexdigest()


class ComparisonFamily(StrEnum):
    """Preregistered comparison family for the campaign.

    ``PAIRED_TASK_LEVEL``: Each task is a paired observation. Comparisons
    are made at the task level between variants or role combinations.
    This is the default and only supported family for the campaign.
    """

    PAIRED_TASK_LEVEL = "paired_task_level"


class EstimandType(StrEnum):
    """Preregistered estimand for the paired comparison.

    ``PAIRED_DELTA``: The paired difference between two variants or
    role combinations on the same task. The estimand is the mean paired
    delta across the task population.

    ``PAIRED_RATE_DIFFERENCE``: The difference in pass rates between two
    variants or role combinations on the same binary task set.
    """

    PAIRED_DELTA = "paired_delta"
    PAIRED_RATE_DIFFERENCE = "paired_rate_difference"


class ParetoDimension(StrEnum):
    """Dimension for a Pareto view.

    Quality dimensions are the vertical axis (higher is better). Cost
    dimensions are the horizontal axis (lower is better). A Pareto view
    plots quality against cost so a reader can inspect trade-offs
    without a single scalar score.
    """

    ACCURACY = "accuracy"
    PASS_RATE = "pass_rate"
    LATENCY_SECONDS = "latency_seconds"
    PEAK_MEMORY_BYTES = "peak_memory_bytes"
    ARTIFACT_BYTES = "artifact_bytes"
    THROUGHPUT_TOKENS_PER_SECOND = "throughput_tokens_per_second"


class DescriptiveFallbackPolicy(StrEnum):
    """Preregistered policy for when the population is too small for inference.

    ``BELOW_MIN_POPULATION``: When the task population is below
    ``minimum_inferential_population``, results are labeled descriptive
    only. No p-values, confidence intervals, or superiority claims are
    produced. Point estimates and Pareto views remain available.

    ``ALWAYS_DESCRIPTIVE``: All results are descriptive only regardless
    of population size. No inferential procedures are applied.
    """

    BELOW_MIN_POPULATION = "below_min_population"
    ALWAYS_DESCRIPTIVE = "always_descriptive"


class ParetoView(BaseModel):
    """One Pareto view plotting a quality dimension against a cost dimension.

    The quality dimension is the vertical axis (higher is better). The
    cost dimension is the horizontal axis (lower is better). A Pareto
    view lets a reader inspect quality versus efficiency trade-offs
    without reducing them to a single scalar score.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    view_id: str = Field(min_length=1, description="Unique view identifier within the contract.")
    quality_dimension: ParetoDimension = Field(description="Vertical axis dimension (higher is better).")
    cost_dimension: ParetoDimension = Field(description="Horizontal axis dimension (lower is better).")

    @model_validator(mode="after")
    def _validate_dimensions_differ(self) -> Self:
        if self.quality_dimension == self.cost_dimension:
            raise ValueError(
                f"quality_dimension and cost_dimension must differ: "
                f"both are {self.quality_dimension.value!r}"
            )
        return self


class StatisticalAnalysisContract(BaseModel):
    """Frozen statistical analysis contract for the campaign.

    Declares the analysis approach before measured collection. The
    contract is hashed and bound to the campaign profile. Changing any
    field changes the hash and creates a new campaign revision.

    The contract freezes: task as the primary independent sampling unit,
    paired task-level comparisons, the exact paired binary procedure
    (McNemar) for pass/fail outcomes, the paired task-cluster bootstrap
    for continuous or repeated outcomes, preregistered multiple-comparison
    correction per declared family (Holm-Bonferroni), paired effect sizes
    and uncertainty intervals, Pareto views for quality versus
    latency/memory/artifact size, and descriptive labeling when the
    population is too small for inferential procedures.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    contract_id: str = Field(min_length=1, description="Unique contract identifier.")
    contract_version: str = Field(min_length=1, description="Contract schema version.")

    independent_unit: str = Field(
        default="task",
        description="Primary independent sampling unit. Must be 'task'.",
    )

    comparison_method: ComparisonFamily = Field(
        default=ComparisonFamily.PAIRED_TASK_LEVEL,
        description="Preregistered comparison family.",
    )

    binary_test_policy: str = Field(
        min_length=1,
        description="Exact paired binary procedure for pass/fail outcomes (e.g. mcnemar).",
    )
    continuous_test_policy: str = Field(
        min_length=1,
        description="Paired task-cluster bootstrap policy for continuous or repeated outcomes (e.g. paired_bootstrap).",
    )

    bootstrap_count: int = Field(ge=1, description="Number of bootstrap resamples for confidence intervals.")
    bootstrap_confidence: float = Field(gt=0.0, lt=1.0, description="Confidence level for bootstrap intervals.")
    bootstrap_seed: int = Field(ge=0, description="Seed for the deterministic bootstrap generator.")

    correction_family: str = Field(
        min_length=1,
        description="Multiple-comparison correction family (e.g. holm_bonferroni, none).",
    )
    significance_level: float = Field(gt=0.0, lt=1.0, description="Significance level for gate decisions.")

    estimand: EstimandType = Field(description="Preregistered estimand for the paired comparison.")
    effect_size_measure: str = Field(
        min_length=1,
        description="Effect size measure (e.g. cohens_d, hedges_g, paired_delta_mean).",
    )

    pareto_views: list[ParetoView] = Field(
        default_factory=list,
        description="Pareto views for quality versus cost trade-offs.",
    )

    descriptive_fallback: DescriptiveFallbackPolicy = Field(
        description="Policy for when the population is too small for inferential procedures.",
    )
    minimum_inferential_population: int = Field(
        ge=1,
        description="Minimum task population for inferential procedures. Below this, results are descriptive only.",
    )

    content_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 over canonical JSON of the contract.",
    )

    @model_validator(mode="after")
    def _validate_contract(self) -> Self:
        if self.independent_unit != "task":
            raise ValueError(
                f"independent_unit must be 'task': got {self.independent_unit!r}"
            )

        view_ids = [v.view_id for v in self.pareto_views]
        if len(view_ids) != len(set(view_ids)):
            seen: set[str] = set()
            dupes: list[str] = []
            for vid in view_ids:
                if vid in seen:
                    dupes.append(vid)
                seen.add(vid)
            raise ValueError(f"duplicate pareto view_id: {sorted(set(dupes))}")

        expected = compute_contract_hash(
            contract_id=self.contract_id,
            contract_version=self.contract_version,
            independent_unit=self.independent_unit,
            comparison_method=self.comparison_method,
            binary_test_policy=self.binary_test_policy,
            continuous_test_policy=self.continuous_test_policy,
            bootstrap_count=self.bootstrap_count,
            bootstrap_confidence=self.bootstrap_confidence,
            bootstrap_seed=self.bootstrap_seed,
            correction_family=self.correction_family,
            significance_level=self.significance_level,
            estimand=self.estimand,
            effect_size_measure=self.effect_size_measure,
            pareto_views=self.pareto_views,
            descriptive_fallback=self.descriptive_fallback,
            minimum_inferential_population=self.minimum_inferential_population,
        )
        if self.content_hash != expected:
            raise ValueError(
                f"statistical analysis contract content_hash mismatch: "
                f"declared {self.content_hash!r}, computed {expected!r}"
            )
        return self


def compute_contract_hash(
    *,
    contract_id: str,
    contract_version: str,
    independent_unit: str,
    comparison_method: ComparisonFamily,
    binary_test_policy: str,
    continuous_test_policy: str,
    bootstrap_count: int,
    bootstrap_confidence: float,
    bootstrap_seed: int,
    correction_family: str,
    significance_level: float,
    estimand: EstimandType,
    effect_size_measure: str,
    pareto_views: list[ParetoView],
    descriptive_fallback: DescriptiveFallbackPolicy,
    minimum_inferential_population: int,
) -> str:
    """Compute the content hash for a statistical analysis contract."""
    payload = json.dumps(
        {
            "contract_id": contract_id,
            "contract_version": contract_version,
            "independent_unit": independent_unit,
            "comparison_method": comparison_method.value,
            "binary_test_policy": binary_test_policy,
            "continuous_test_policy": continuous_test_policy,
            "bootstrap_count": bootstrap_count,
            "bootstrap_confidence": bootstrap_confidence,
            "bootstrap_seed": bootstrap_seed,
            "correction_family": correction_family,
            "significance_level": significance_level,
            "estimand": estimand.value,
            "effect_size_measure": effect_size_measure,
            "pareto_views": [
                json.loads(v.model_dump_json())
                for v in sorted(pareto_views, key=lambda v: v.view_id)
            ],
            "descriptive_fallback": descriptive_fallback.value,
            "minimum_inferential_population": minimum_inferential_population,
        },
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


class RoleCombinationId(BaseModel):
    """Stable identity binding all three exact model-variant IDs.

    A stable role-combination ID binds the exact Primary, Assistant, and
    Lite ``ModelVariant`` identities. The three variant IDs must be
    distinct; a combination with the same variant in two roles is
    rejected.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    combination_id: str = Field(min_length=1, description="Stable role-combination identifier.")
    primary_variant_id: str = Field(min_length=1, description="Primary role model variant ID.")
    assistant_variant_id: str = Field(min_length=1, description="Assistant role model variant ID.")
    lite_variant_id: str = Field(min_length=1, description="Lite role model variant ID.")

    @model_validator(mode="after")
    def _validate_distinct_variants(self) -> Self:
        ids = [self.primary_variant_id, self.assistant_variant_id, self.lite_variant_id]
        if len(ids) != len(set(ids)):
            raise ValueError(
                f"role combination variant_id values must be distinct: {ids}"
            )
        return self


class RoleCombination(BaseModel):
    """One exact Primary/Assistant/Lite role combination.

    Binds the stable role-combination ID to the three exact variant IDs
    and records the rationale for including this combination in the
    tractable matrix rather than silently attempting an unbounded
    Cartesian product.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    combination_id: RoleCombinationId = Field(description="Stable identity binding all three variant IDs.")
    rationale: str = Field(min_length=1, description="Rationale for including this combination.")


class RoleExerciseDeclaration(BaseModel):
    """Declaration of which roles a benchmark task exercises.

    Every benchmark task declares which roles it exercises. Each
    exercised role must have matching provider-boundary telemetry. Non-
    invoked roles are preserved as explicit non-observations.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    task_id: str = Field(min_length=1, description="Task ID.")
    exercised_roles: list[str] = Field(
        min_length=1,
        description="Sorted tier names this task exercises (subset of primary, assistant, lite).",
    )

    @model_validator(mode="after")
    def _validate_roles(self) -> Self:
        for role in self.exercised_roles:
            if role not in _VALID_TIERS:
                raise ValueError(
                    f"exercised_roles must be from {sorted(_VALID_TIERS)}: got {role!r}"
                )
        if self.exercised_roles != sorted(self.exercised_roles):
            raise ValueError(
                f"exercised_roles must be sorted: {self.exercised_roles}"
            )
        if len(self.exercised_roles) != len(set(self.exercised_roles)):
            raise ValueError(
                f"exercised_roles must not contain duplicates: {self.exercised_roles}"
            )
        return self


class TaskRoleBinding(BaseModel):
    """Binding of a task to an exact role combination with variant IDs.

    Metrics bind both the exact three-role combination and the
    individual invoked variant. Results from different combinations
    never pool implicitly. The binding's variant IDs must match the
    referenced combination's variant IDs.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    task_id: str = Field(min_length=1, description="Task ID.")
    combination_id: str = Field(min_length=1, description="Role-combination ID from the track's combinations.")
    primary_variant_id: str = Field(min_length=1, description="Primary role variant ID for this binding.")
    assistant_variant_id: str = Field(min_length=1, description="Assistant role variant ID for this binding.")
    lite_variant_id: str = Field(min_length=1, description="Lite role variant ID for this binding.")


class RoleCombinationTrack(BaseModel):
    """Frozen role-combination track for the campaign.

    Defines exact Primary/Assistant/Lite assignments, freezes a
    tractable combination matrix and rationale, declares which roles each
    benchmark task exercises, and binds tasks to exact role
    combinations. Metrics bind both the exact three-role combination and
    the individual invoked variant; results from different combinations
    never pool implicitly.

    The ``content_hash`` is SHA-256 over canonical JSON of the track.
    Changing any field changes the hash and invalidates downstream
    campaign profile hashes.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    track_id: str = Field(min_length=1, description="Unique track identifier.")
    track_version: str = Field(min_length=1, description="Track schema version.")
    description: str = Field(min_length=1, description="Human-readable track description.")

    combinations: list[RoleCombination] = Field(
        min_length=1,
        description="Exact role combinations in the tractable matrix.",
    )
    role_exercise_declarations: list[RoleExerciseDeclaration] = Field(
        default_factory=list,
        description="Per-task declarations of which roles are exercised.",
    )
    task_role_bindings: list[TaskRoleBinding] = Field(
        default_factory=list,
        description="Bindings of tasks to exact role combinations with variant IDs.",
    )

    content_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 over canonical JSON of the track.",
    )

    @model_validator(mode="after")
    def _validate_track(self) -> Self:
        combo_ids = [c.combination_id.combination_id for c in self.combinations]
        if len(combo_ids) != len(set(combo_ids)):
            seen: set[str] = set()
            dupes: list[str] = []
            for cid in combo_ids:
                if cid in seen:
                    dupes.append(cid)
                seen.add(cid)
            raise ValueError(f"duplicate combination_id: {sorted(set(dupes))}")

        combo_map = {c.combination_id.combination_id: c.combination_id for c in self.combinations}

        for binding in self.task_role_bindings:
            if binding.combination_id not in combo_map:
                raise ValueError(
                    f"task_role_binding references unknown combination_id: "
                    f"{binding.combination_id!r}"
                )
            combo = combo_map[binding.combination_id]
            if (
                binding.primary_variant_id != combo.primary_variant_id
                or binding.assistant_variant_id != combo.assistant_variant_id
                or binding.lite_variant_id != combo.lite_variant_id
            ):
                raise ValueError(
                    f"task_role_binding variant_id mismatch for combination "
                    f"{binding.combination_id!r}: binding has "
                    f"({binding.primary_variant_id!r}, "
                    f"{binding.assistant_variant_id!r}, "
                    f"{binding.lite_variant_id!r}), combination has "
                    f"({combo.primary_variant_id!r}, "
                    f"{combo.assistant_variant_id!r}, "
                    f"{combo.lite_variant_id!r})"
                )

        expected = compute_role_track_hash(
            track_id=self.track_id,
            track_version=self.track_version,
            description=self.description,
            combinations=self.combinations,
            role_exercise_declarations=self.role_exercise_declarations,
            task_role_bindings=self.task_role_bindings,
        )
        if self.content_hash != expected:
            raise ValueError(
                f"role-combination track content_hash mismatch: "
                f"declared {self.content_hash!r}, computed {expected!r}"
            )
        return self


def compute_role_track_hash(
    *,
    track_id: str,
    track_version: str,
    description: str,
    combinations: list[RoleCombination],
    role_exercise_declarations: list[RoleExerciseDeclaration],
    task_role_bindings: list[TaskRoleBinding],
) -> str:
    """Compute the content hash for a role-combination track."""
    payload = json.dumps(
        {
            "track_id": track_id,
            "track_version": track_version,
            "description": description,
            "combinations": [
                json.loads(c.model_dump_json())
                for c in sorted(combinations, key=lambda c: c.combination_id.combination_id)
            ],
            "role_exercise_declarations": [
                json.loads(d.model_dump_json())
                for d in sorted(role_exercise_declarations, key=lambda d: d.task_id)
            ],
            "task_role_bindings": [
                json.loads(b.model_dump_json())
                for b in sorted(task_role_bindings, key=lambda b: (b.task_id, b.combination_id))
            ],
        },
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


__all__ = [
    "ANALYSIS_CONTRACT_VERSION",
    "ROLE_COMBINATION_TRACK_VERSION",
    "ComparisonFamily",
    "DescriptiveFallbackPolicy",
    "EstimandType",
    "ParetoDimension",
    "ParetoView",
    "RoleCombination",
    "RoleCombinationId",
    "RoleCombinationTrack",
    "RoleExerciseDeclaration",
    "StatisticalAnalysisContract",
    "TaskRoleBinding",
    "compute_contract_hash",
    "compute_role_track_hash",
]
