# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Frozen paired model-comparison authority and engine.

Implements the D10, D18, and D19 model-comparison contract as a
separate typed authority from the arm-within-cohort analysis in
``analysis/engine.py``. The canonical inferential engine compares
arms within a model cohort; this module compares model cohorts
against preregistered anchors. The two engines never share a
denominator, family, or claim boundary.

The comparison preregistration binds the global anchor (Qwen3-8B),
class anchors (D19), family membership, pair keys, repetition
reduction policy, missingness policy, minimum population, tests,
effect estimands, confidence intervals, correction method, alpha,
and claim gates. The preregistration is frozen before the first
dependent provider call; any material change creates a new
identity and invalidates dependent collection.

Pairing matches raw cells by exact accepted ``(task_id,
repetition_id)`` identity, then reduces the three matched
repetition outcomes within each task to the preregistered
majority binary outcome. The 0/3 through 3/3 count is retained as
descriptive stability; task remains the independent inferential
unit. Missing or non-terminal repetition cells are treated
according to the frozen missingness policy: a pair is never
silently completed or dropped.

Inference uses exact McNemar for binary majority outcomes, paired
risk difference, deterministic task-cluster bootstrap intervals,
and Holm-Bonferroni correction over the global-anchor family and
separately within each approved class family. All-pairs
comparisons and anchor substitution not present in the authority
are rejected.

The canonical output binds to aggregate verification hash,
campaign-set plan/index hashes, profile, registry, benchmark
population, comparison authority, code/metric identity, and
environment strata. The output content hash covers every bound
authority so that a changed authority invalidates the output.

Every model is frozen with ``extra="forbid"``. Content hashes
are SHA-256 over canonical JSON (sorted keys, no extra
whitespace).
"""

from __future__ import annotations

import hashlib
import json
import math
from collections.abc import Sequence
from enum import StrEnum
from typing import Self

from pydantic import BaseModel, ConfigDict, Field, model_validator

from g8e_evals.analysis import statistics
from g8e_evals.registry import WeightClass


MODEL_COMPARISON_AUTHORITY_VERSION = "1.0.0"
MODEL_COMPARISON_ENGINE_VERSION = "1.0.0"

_FLOAT_PRECISION = 10


def _sha256(data: str) -> str:
    return hashlib.sha256(data.encode()).hexdigest()


def _round(value: float) -> float:
    return round(float(value), _FLOAT_PRECISION)


class RepetitionReductionPolicy(StrEnum):
    """Preregistered policy for reducing matched repetition outcomes.

    ``MAJORITY_BINARY``: Reduce the three matched repetition outcomes
    within each task to a single binary majority outcome. A task passes
    when at least two of three repetitions pass (majority of 3). The
    0/3 through 3/3 count is retained as descriptive stability. Task
    remains the independent inferential unit.
    """

    MAJORITY_BINARY = "majority_binary"


class MissingnessPolicy(StrEnum):
    """Preregistered policy for missing or non-terminal repetition cells.

    ``REJECT_PAIR``: A pair with any missing or non-terminal repetition
    cell is rejected from the inferential population. The pair is
    retained in the denominator as a missing contribution and the
    missingness reason is recorded. The pair is never silently
    completed or dropped.

    ``TREAT_AS_FAILURE``: A missing or non-terminal repetition cell is
    treated as a failure (binary 0) for the majority reduction. The
    pair remains in the inferential population. This policy is more
    permissive and must be explicitly authorized.
    """

    REJECT_PAIR = "reject_pair"
    TREAT_AS_FAILURE = "treat_as_failure"


class ComparisonTest(StrEnum):
    """Preregistered inferential test for paired binary majority outcomes.

    ``EXACT_MCNEMAR``: Exact McNemar test using the binomial
    distribution over discordant pairs. The test is two-sided.

    ``PAIRED_RISK_DIFFERENCE``: Paired risk difference with a
    task-cluster bootstrap confidence interval. The point estimate
    is the mean paired difference in binary majority outcomes.
    """

    EXACT_MCNEMAR = "exact_mcnemar"
    PAIRED_RISK_DIFFERENCE = "paired_risk_difference"


class CorrectionMethod(StrEnum):
    """Preregistered multiple-comparison correction method.

    ``HOLM_BONFERRONI``: Holm-Bonferroni step-down correction applied
    within each declared correction family. The global-anchor family
    and each class family are corrected separately.
    """

    HOLM_BONFERRONI = "holm_bonferroni"


class ClaimGate(StrEnum):
    """Preregistered claim gate for model-comparison conclusions.

    ``DESCRIPTIVE_ONLY``: Test statistics, p-values, and confidence
    intervals are retained as diagnostic output. No superiority or
    non-inferiority claim is produced.

    ``SUPERIORITY``: A Holm-corrected p-value below the significance
    level combined with a non-zero paired risk difference can produce
    a superiority claim.

    ``NON_INFERIORITY``: A bootstrap confidence interval within the
    declared non-inferiority margin can produce a non-inferiority
    claim.
    """

    DESCRIPTIVE_ONLY = "descriptive_only"
    SUPERIORITY = "superiority"
    NON_INFERIORITY = "non_inferiority"


class ComparisonFamilyKind(StrEnum):
    """Kind of correction family for Holm-Bonferroni grouping.

    ``GLOBAL_ANCHOR``: The family containing all comparisons against
    the global anchor (Qwen3-8B). Corrected as one family.

    ``CLASS_ANCHOR``: The family containing comparisons against a
    class anchor within one weight class. Each class is corrected
    separately.
    """

    GLOBAL_ANCHOR = "global_anchor"
    CLASS_ANCHOR = "class_anchor"


class ClassAnchorBinding(BaseModel):
    """Frozen binding of one D19 class anchor to its weight class.

    Binds the anchor variant ID, the weight class it anchors, and
    the family name used for Holm-Bonferroni correction within that
    class. The anchor must exist in the frozen registry with the
    expected weight class and a runnable backend identity.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    weight_class: WeightClass = Field(description="Weight class anchored by this variant.")
    anchor_variant_id: str = Field(min_length=1, description="D19 anchor variant ID from the registry.")
    family_name: str = Field(
        min_length=1,
        description="Holm-Bonferroni family name for within-class comparisons.",
    )


class ComparisonPairKey(BaseModel):
    """Frozen pair key for one authorized model comparison.

    Binds the candidate variant ID, the anchor variant ID, the
    family kind (global or class), the family name for correction,
    and the weight class for class-anchor comparisons. The pair
    key is the authority for one comparison; a comparison not
    declared here is rejected.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    candidate_variant_id: str = Field(min_length=1, description="Candidate variant ID from the registry.")
    anchor_variant_id: str = Field(min_length=1, description="Anchor variant ID (global or class anchor).")
    family_kind: ComparisonFamilyKind = Field(description="Global-anchor or class-anchor family.")
    family_name: str = Field(
        min_length=1,
        description="Holm-Bonferroni family name for this pair.",
    )
    weight_class: WeightClass | None = Field(
        default=None,
        description="Weight class for class-anchor comparisons. None for global-anchor comparisons.",
    )

    @model_validator(mode="after")
    def _validate_pair_key(self) -> Self:
        if self.family_kind == ComparisonFamilyKind.CLASS_ANCHOR and self.weight_class is None:
            raise ValueError(
                f"class-anchor pair key requires weight_class: candidate={self.candidate_variant_id!r}"
            )
        if self.family_kind == ComparisonFamilyKind.GLOBAL_ANCHOR and self.weight_class is not None:
            raise ValueError(
                f"global-anchor pair key must not set weight_class: candidate={self.candidate_variant_id!r}"
            )
        if self.candidate_variant_id == self.anchor_variant_id:
            raise ValueError(
                f"candidate variant cannot equal anchor variant: {self.candidate_variant_id!r}"
            )
        return self


class ModelComparisonPreregistration(BaseModel):
    """Frozen paired model-comparison preregistration for D10, D18, D19.

    Binds the global anchor, class anchors, family membership, pair
    keys, repetition reduction, missingness, minimum population,
    tests, effect estimands, confidence intervals, correction
    method, alpha, and claim gates. The preregistration is frozen
    before the first dependent provider call; any material change
    creates a new identity and invalidates dependent collection.

    The ``content_hash`` is SHA-256 over canonical JSON of the
    preregistration (sorted keys, no extra whitespace). Changing
    any bound authority changes the hash.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    authority_id: str = Field(min_length=1, description="Unique authority identifier.")
    authority_version: str = Field(min_length=1, description="Authority version (incremented when bindings change).")
    schema_version: str = Field(min_length=1, description="Schema version of the authority contract.")

    global_anchor_variant_id: str = Field(
        min_length=1,
        description="D10 global operational anchor variant ID (Qwen3-8B).",
    )
    global_anchor_family_name: str = Field(
        min_length=1,
        description="Holm-Bonferroni family name for global-anchor comparisons.",
    )

    class_anchors: list[ClassAnchorBinding] = Field(
        min_length=1,
        description="D19 class anchors, one per weight class. Each anchors comparisons within its class.",
    )

    pair_keys: list[ComparisonPairKey] = Field(
        min_length=1,
        description="Authorized comparison pair keys. A comparison not declared here is rejected.",
    )

    repetition_count: int = Field(ge=1, description="Number of matched repetitions per task per variant.")
    repetition_reduction_policy: RepetitionReductionPolicy = Field(
        description="Policy for reducing matched repetitions to a single outcome.",
    )
    missingness_policy: MissingnessPolicy = Field(
        description="Policy for missing or non-terminal repetition cells.",
    )

    minimum_population: int = Field(
        ge=1,
        description="Minimum number of paired tasks required for inferential procedures.",
    )

    primary_test: ComparisonTest = Field(
        description="Primary inferential test for binary majority outcomes.",
    )
    correction_method: CorrectionMethod = Field(
        description="Multiple-comparison correction method.",
    )
    significance_level: float = Field(
        gt=0.0, lt=1.0,
        description="Significance level for Holm-corrected p-values.",
    )

    bootstrap_count: int = Field(ge=1, description="Number of bootstrap resamples for confidence intervals.")
    bootstrap_confidence: float = Field(
        gt=0.0, lt=1.0,
        description="Confidence level for bootstrap intervals.",
    )
    bootstrap_seed: int = Field(ge=0, description="Seed for the deterministic bootstrap generator.")

    non_inferiority_margin: float | None = Field(
        default=None,
        ge=0.0,
        description="Non-inferiority margin for the paired risk difference. None when no margin is declared.",
    )

    claim_gate: ClaimGate = Field(
        description="Claim gate: descriptive-only, superiority, or non-inferiority.",
    )

    environment_stratum: str = Field(
        min_length=1,
        description="Environment stratum label bound to this authority (e.g. single-machine).",
    )

    content_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 over canonical JSON of the preregistration.",
    )

    @model_validator(mode="after")
    def _validate_preregistration(self) -> Self:
        anchor_classes = {ca.weight_class for ca in self.class_anchors}
        if len(anchor_classes) != len(self.class_anchors):
            raise ValueError(
                f"duplicate weight class in class_anchors: {[ca.weight_class for ca in self.class_anchors]}"
            )

        anchor_family_names = [ca.family_name for ca in self.class_anchors]
        if len(anchor_family_names) != len(set(anchor_family_names)):
            raise ValueError(
                f"duplicate class anchor family name: {anchor_family_names}"
            )

        if self.global_anchor_family_name in anchor_family_names:
            raise ValueError(
                f"global anchor family name {self.global_anchor_family_name!r} "
                f"collides with a class anchor family name"
            )

        anchor_variant_ids = {ca.anchor_variant_id for ca in self.class_anchors}
        if self.global_anchor_variant_id in anchor_variant_ids:
            raise ValueError(
                f"global anchor variant {self.global_anchor_variant_id!r} "
                f"is also a class anchor"
            )

        pair_keys_seen: set[tuple[str, str, str]] = set()
        for pk in self.pair_keys:
            key = (pk.candidate_variant_id, pk.anchor_variant_id, pk.family_name)
            if key in pair_keys_seen:
                raise ValueError(f"duplicate pair key: {key}")
            pair_keys_seen.add(key)

            if pk.family_kind == ComparisonFamilyKind.GLOBAL_ANCHOR:
                if pk.anchor_variant_id != self.global_anchor_variant_id:
                    raise ValueError(
                        f"global-anchor pair key references non-global anchor: "
                        f"{pk.anchor_variant_id!r} != {self.global_anchor_variant_id!r}"
                    )
                if pk.family_name != self.global_anchor_family_name:
                    raise ValueError(
                        f"global-anchor pair key family name {pk.family_name!r} "
                        f"!= global anchor family {self.global_anchor_family_name!r}"
                    )
            elif pk.family_kind == ComparisonFamilyKind.CLASS_ANCHOR:
                class_anchor_by_class = {ca.weight_class: ca for ca in self.class_anchors}
                ca = class_anchor_by_class.get(pk.weight_class)
                if ca is None:
                    raise ValueError(
                        f"class-anchor pair key references unknown weight class: {pk.weight_class!r}"
                    )
                if pk.anchor_variant_id != ca.anchor_variant_id:
                    raise ValueError(
                        f"class-anchor pair key anchor {pk.anchor_variant_id!r} "
                        f"!= D19 anchor for {pk.weight_class!r}: {ca.anchor_variant_id!r}"
                    )
                if pk.family_name != ca.family_name:
                    raise ValueError(
                        f"class-anchor pair key family name {pk.family_name!r} "
                        f"!= D19 family for {pk.weight_class!r}: {ca.family_name!r}"
                    )

        expected = compute_model_comparison_hash(self)
        if self.content_hash != expected:
            raise ValueError(
                f"model comparison content_hash mismatch: declared {self.content_hash!r}, "
                f"computed {expected!r}"
            )
        return self


def compute_model_comparison_hash(preregistration: ModelComparisonPreregistration) -> str:
    """Compute the content hash for a model comparison preregistration.

    Serializes every material field (excluding ``content_hash`` itself)
    into canonical JSON and returns SHA-256. The same function is used
    by the model validator and by builders so a field added to the
    preregistration is automatically included in the hash.
    """
    data = preregistration.model_dump(mode="json", by_alias=True)
    data.pop("content_hash", None)
    payload = json.dumps(
        data,
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


class RepetitionCell(BaseModel):
    """One raw repetition cell for model comparison pairing.

    Binds the variant ID, task ID, repetition ID, and the binary
    pass/fail outcome. A ``None`` outcome indicates a missing or
    non-terminal repetition cell; the missingness policy determines
    how it is treated.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    variant_id: str = Field(min_length=1, description="Model variant ID.")
    task_id: str = Field(min_length=1, description="Task ID.")
    repetition_id: str = Field(min_length=1, description="Repetition ID.")
    passed: bool | None = Field(
        description="Binary pass/fail outcome. None when missing or non-terminal.",
    )


class TaskMajorityOutcome(BaseModel):
    """Frozen majority reduction outcome for one task and variant.

    Records the 0/3 through 3/3 pass count, the majority binary
    outcome (True when at least 2 of 3 pass), and whether the pair
    was rejected due to missingness. Task remains the independent
    inferential unit.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    variant_id: str = Field(min_length=1, description="Model variant ID.")
    task_id: str = Field(min_length=1, description="Task ID.")
    pass_count: int = Field(ge=0, description="Number of passing repetitions (0 to repetition_count).")
    total_repetitions: int = Field(ge=1, description="Total number of matched repetitions.")
    majority_passed: bool | None = Field(
        description="Majority binary outcome. None when the pair was rejected for missingness.",
    )
    rejected_for_missingness: bool = Field(
        description="True when the missingness policy rejected this task for missing cells.",
    )
    missing_repetition_count: int = Field(
        ge=0, description="Number of missing or non-terminal repetition cells.",
    )


class PairedTaskOutcome(BaseModel):
    """Frozen paired task outcome for one candidate-anchor pair.

    Binds the candidate and anchor majority outcomes, the paired
    binary values used in inference, and whether the pair is in the
    inferential population or rejected for missingness.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    candidate_variant_id: str = Field(min_length=1)
    anchor_variant_id: str = Field(min_length=1)
    task_id: str = Field(min_length=1)
    candidate_majority: bool | None = Field(
        description="Candidate majority outcome. None when rejected for missingness.",
    )
    anchor_majority: bool | None = Field(
        description="Anchor majority outcome. None when rejected for missingness.",
    )
    in_inferential_population: bool = Field(
        description="True when both candidate and anchor majority outcomes are available.",
    )


class ClaimGateStatus(StrEnum):
    """Typed outcome of the claim gate for one model comparison.

    ``PASS``: The comparison passes the declared claim gate
    (superiority or non-inferiority).

    ``FAIL``: The comparison fails the declared claim gate.

    ``INSUFFICIENT_POPULATION``: The paired task count is below the
    minimum inferential population.

    ``DESCRIPTIVE_ONLY``: The claim gate is descriptive-only; no
    superiority or non-inferiority claim is produced. Test
    statistics and intervals remain diagnostic.

    ``NO_DISCORDANT_PAIRS``: The exact McNemar test has no discordant
    pairs; the p-value is undefined.
    """

    PASS = "pass"
    FAIL = "fail"
    INSUFFICIENT_POPULATION = "insufficient_population"
    DESCRIPTIVE_ONLY = "descriptive_only"
    NO_DISCORDANT_PAIRS = "no_discordant_pairs"


class ModelComparisonResult(BaseModel):
    """Frozen result of one paired model comparison.

    Records the pair key, paired task count, candidate and anchor
    pass rates, paired risk difference, exact McNemar statistic and
    p-value, bootstrap confidence interval, Holm-corrected p-value
    and rank, claim gate status, and the list of paired task outcomes
    for audit. Every value traces to the frozen preregistration and
    the accepted evidence.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    candidate_variant_id: str = Field(min_length=1)
    anchor_variant_id: str = Field(min_length=1)
    family_kind: ComparisonFamilyKind
    family_name: str = Field(min_length=1)
    weight_class: WeightClass | None = Field(default=None)

    paired_task_count: int = Field(ge=0, description="Number of tasks in the inferential population.")
    rejected_task_count: int = Field(ge=0, description="Number of tasks rejected for missingness.")
    total_task_count: int = Field(ge=0, description="Total number of matched tasks (inferential + rejected).")

    candidate_pass_count: int = Field(ge=0, description="Number of tasks where the candidate majority passed.")
    anchor_pass_count: int = Field(ge=0, description="Number of tasks where the anchor majority passed.")
    candidate_pass_rate: float | None = Field(
        default=None,
        description="Candidate pass rate in the inferential population. None when population is zero.",
    )
    anchor_pass_rate: float | None = Field(
        default=None,
        description="Anchor pass rate in the inferential population. None when population is zero.",
    )

    paired_risk_difference: float | None = Field(
        default=None,
        description="Candidate pass rate minus anchor pass rate. None when population is zero.",
    )

    mcnemar_statistic: float | None = Field(default=None, description="Exact McNemar statistic.")
    mcnemar_p_value: float | None = Field(default=None, description="Exact McNemar two-sided p-value.")

    bootstrap_ci_lower: float | None = Field(default=None, description="Task-cluster bootstrap CI lower bound.")
    bootstrap_ci_upper: float | None = Field(default=None, description="Task-cluster bootstrap CI upper bound.")

    holm_corrected_p_value: float | None = Field(default=None, description="Holm-corrected p-value within family.")
    holm_rank: int | None = Field(default=None, description="Holm rank within family (1-based).")

    claim_gate_status: ClaimGateStatus = Field(description="Typed claim gate outcome.")
    claim_gate_reason: str = Field(min_length=1, description="Reason for the claim gate outcome.")

    paired_task_outcomes: list[PairedTaskOutcome] = Field(
        default_factory=list,
        description="Per-task paired majority outcomes, sorted by task_id. Empty when no tasks are paired.",
    )


class ModelComparisonOutput(BaseModel):
    """Frozen canonical output of the model comparison engine.

    Binds the comparison preregistration hash, aggregate verification
    hash, campaign-set plan and index hashes, profile hash, registry
    hash, benchmark population hash, code/metric identity hash, and
    environment stratum. The output content hash covers every bound
    authority so that a changed authority invalidates the output.

    The results list is sorted by
    ``(family_kind, family_name, candidate_variant_id, anchor_variant_id)``.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    engine_version: str = Field(min_length=1, description="Model comparison engine version.")
    authority_hash: str = Field(
        min_length=64, max_length=64,
        description="Content hash of the frozen model comparison preregistration.",
    )
    aggregate_verification_hash: str = Field(
        min_length=64, max_length=64,
        description="Content hash of the accepted aggregate verification report.",
    )
    campaign_set_plan_hash: str = Field(
        min_length=64, max_length=64,
        description="Content hash of the frozen campaign-set plan.",
    )
    campaign_set_index_hash: str = Field(
        min_length=64, max_length=64,
        description="Content hash of the post-execution campaign-set index.",
    )
    profile_hash: str = Field(
        min_length=64, max_length=64,
        description="Content hash of the frozen campaign profile.",
    )
    registry_hash: str = Field(
        min_length=64, max_length=64,
        description="Content hash of the frozen model registry.",
    )
    benchmark_population_hash: str = Field(
        min_length=64, max_length=64,
        description="Content hash of the frozen benchmark population.",
    )
    code_metric_identity_hash: str = Field(
        min_length=64, max_length=64,
        description="Content hash of the code and metric identity (engine version + metric registry).",
    )
    environment_stratum: str = Field(
        min_length=1,
        description="Environment stratum label bound to this output.",
    )

    results: list[ModelComparisonResult] = Field(
        default_factory=list,
        description="Model comparison results, sorted by (family_kind, family_name, candidate, anchor).",
    )

    content_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 over canonical JSON of the output (excluding content_hash).",
    )

    @model_validator(mode="after")
    def _validate_output(self) -> Self:
        expected = compute_model_comparison_output_hash(self)
        if self.content_hash != expected:
            raise ValueError(
                f"model comparison output content_hash mismatch: declared {self.content_hash!r}, "
                f"computed {expected!r}"
            )
        return self


def compute_model_comparison_output_hash(output: ModelComparisonOutput) -> str:
    """Compute the content hash for a model comparison output.

    Serializes every material field (excluding ``content_hash`` itself)
    into canonical JSON and returns SHA-256.
    """
    data = output.model_dump(mode="json", by_alias=True)
    data.pop("content_hash", None)
    payload = json.dumps(
        data,
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


def compute_code_metric_identity_hash(
    engine_version: str,
    metric_registry_hash: str,
) -> str:
    """Compute the code/metric identity hash for binding into the output.

    Binds the engine version and the metric registry hash so that a
    changed engine or metric registry invalidates the output.
    """
    payload = json.dumps(
        {
            "engine_version": engine_version,
            "metric_registry_hash": metric_registry_hash,
        },
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


def reduce_repetitions_to_majority(
    cells: Sequence[RepetitionCell],
    repetition_count: int,
    missingness_policy: MissingnessPolicy,
) -> TaskMajorityOutcome:
    """Reduce matched repetition cells to a single majority binary outcome.

    The ``cells`` sequence contains one cell per matched repetition
    for one variant and one task. The function counts passing
    repetitions, applies the missingness policy, and returns the
    majority binary outcome. A task passes when at least
    ``ceil(repetition_count / 2)`` repetitions pass.

    Under ``REJECT_PAIR``, any missing cell rejects the task from
    the inferential population (``majority_passed=None``,
    ``rejected_for_missingness=True``).

    Under ``TREAT_AS_FAILURE``, a missing cell is treated as a
    failure (``passed=False``) and the task remains in the
    inferential population.
    """
    if len(cells) != repetition_count:
        raise ValueError(
            f"repetition cell count {len(cells)} != repetition_count {repetition_count} "
            f"for variant={cells[0].variant_id if cells else '?'!r} task={cells[0].task_id if cells else '?'!r}"
        )

    variant_id = cells[0].variant_id
    task_id = cells[0].task_id

    missing_count = sum(1 for c in cells if c.passed is None)
    pass_count = sum(1 for c in cells if c.passed is True)

    if missing_count > 0 and missingness_policy == MissingnessPolicy.REJECT_PAIR:
        return TaskMajorityOutcome(
            variant_id=variant_id,
            task_id=task_id,
            pass_count=pass_count,
            total_repetitions=repetition_count,
            majority_passed=None,
            rejected_for_missingness=True,
            missing_repetition_count=missing_count,
        )

    if missingness_policy == MissingnessPolicy.TREAT_AS_FAILURE:
        pass_count = sum(1 for c in cells if c.passed is True)

    majority_threshold = math.ceil(repetition_count / 2)
    majority_passed = pass_count >= majority_threshold

    return TaskMajorityOutcome(
        variant_id=variant_id,
        task_id=task_id,
        pass_count=pass_count,
        total_repetitions=repetition_count,
        majority_passed=majority_passed,
        rejected_for_missingness=False,
        missing_repetition_count=missing_count,
    )


def pair_task_majority_outcomes(
    candidate_outcomes: Sequence[TaskMajorityOutcome],
    anchor_outcomes: Sequence[TaskMajorityOutcome],
) -> list[PairedTaskOutcome]:
    """Pair candidate and anchor majority outcomes by task_id.

    Matches on exact ``task_id`` identity. A task present in one
    sequence but not the other is not paired. A task rejected for
    missingness in either sequence is not in the inferential
    population (``in_inferential_population=False``).
    """
    candidate_by_task = {o.task_id: o for o in candidate_outcomes}
    anchor_by_task = {o.task_id: o for o in anchor_outcomes}

    common_tasks = sorted(set(candidate_by_task.keys()) & set(anchor_by_task.keys()))

    paired: list[PairedTaskOutcome] = []
    for task_id in common_tasks:
        c = candidate_by_task[task_id]
        a = anchor_by_task[task_id]
        in_pop = not c.rejected_for_missingness and not a.rejected_for_missingness
        paired.append(PairedTaskOutcome(
            candidate_variant_id=c.variant_id,
            anchor_variant_id=a.variant_id,
            task_id=task_id,
            candidate_majority=c.majority_passed if not c.rejected_for_missingness else None,
            anchor_majority=a.majority_passed if not a.rejected_for_missingness else None,
            in_inferential_population=in_pop,
        ))

    return paired


def _exact_mcnemar(
    candidate_binary: Sequence[bool],
    anchor_binary: Sequence[bool],
) -> tuple[float | None, float | None]:
    """Compute exact McNemar statistic and two-sided p-value.

    Returns ``(statistic, p_value)``. The statistic is ``|b - c|``
    where ``b`` is the count of pairs where the anchor passed and
    the candidate failed, and ``c`` is the count where the candidate
    passed and the anchor failed. The p-value uses the exact
    binomial distribution when the discordant count is small.

    Returns ``(None, None)`` when there are no discordant pairs.
    """
    n = len(candidate_binary)
    if n == 0:
        return None, None
    if len(anchor_binary) != n:
        raise ValueError("candidate and anchor binary sequences must have equal length")

    b = sum(1 for i in range(n) if anchor_binary[i] and not candidate_binary[i])
    c = sum(1 for i in range(n) if not anchor_binary[i] and candidate_binary[i])
    discordant = b + c

    if discordant == 0:
        return None, None

    p_value = _binomial_two_sided(min(b, c), discordant, 0.5)
    statistic = float(abs(b - c))
    return _round(statistic), _round(p_value)


def _binomial_two_sided(successes: int, trials: int, p: float) -> float:
    """Two-sided p-value for a binomial test."""
    if trials == 0:
        return 1.0
    observed_prob = _binom_pmf(successes, trials, p)
    p_value = 0.0
    for k in range(trials + 1):
        pk = _binom_pmf(k, trials, p)
        if pk <= observed_prob + 1e-15:
            p_value += pk
    return min(p_value, 1.0)


def _binom_pmf(k: int, n: int, p: float) -> float:
    """Binomial probability mass function via log-space computation."""
    if k < 0 or k > n:
        return 0.0
    log_coeff = _log_binom_coeff(n, k)
    log_p = log_coeff + k * math.log(p) + (n - k) * math.log(1.0 - p)
    return math.exp(log_p)


def _log_binom_coeff(n: int, k: int) -> float:
    """Log of binomial coefficient C(n, k)."""
    if k < 0 or k > n:
        return float("-inf")
    k = min(k, n - k)
    result = 0.0
    for i in range(k):
        result += math.log(n - i) - math.log(i + 1)
    return result


def _task_cluster_bootstrap_ci(
    candidate_binary: Sequence[bool],
    anchor_binary: Sequence[bool],
    n_bootstrap: int,
    confidence: float,
    seed: int,
) -> tuple[float | None, float | None]:
    """Compute a task-cluster bootstrap CI for the paired risk difference.

    Resamples paired differences (candidate - anchor) with replacement
    using a seeded numpy ``Generator``. Returns ``(ci_lower, ci_upper)``.
    Returns ``(None, None)`` when fewer than 2 paired values.
    """
    import numpy as np

    n = len(candidate_binary)
    if n_bootstrap <= 0:
        raise ValueError("bootstrap sample count must be positive")
    if not 0.0 < confidence < 1.0:
        raise ValueError("bootstrap confidence must be between zero and one")
    if n < 2:
        return None, None

    diffs = np.array(
        [float(candidate_binary[i]) - float(anchor_binary[i]) for i in range(n)],
        dtype=np.float64,
    )
    rng = np.random.default_rng(seed)
    indices = rng.integers(0, n, size=(n_bootstrap, n))
    boot_means = diffs[indices].mean(axis=1)
    boot_means.sort()

    alpha = 1.0 - confidence
    lower_idx = math.floor(alpha / 2.0 * n_bootstrap)
    upper_idx = math.ceil((1.0 - alpha / 2.0) * n_bootstrap) - 1
    upper_idx = min(upper_idx, n_bootstrap - 1)

    return _round(float(boot_means[lower_idx])), _round(float(boot_means[upper_idx]))


def _evaluate_claim_gate(
    claim_gate: ClaimGate,
    corrected_p_value: float | None,
    paired_risk_difference: float | None,
    bootstrap_ci_lower: float | None,
    bootstrap_ci_upper: float | None,
    non_inferiority_margin: float | None,
    significance_level: float,
    paired_task_count: int,
    minimum_population: int,
    mcnemar_p_value: float | None,
) -> tuple[ClaimGateStatus, str]:
    """Evaluate the claim gate for one model comparison.

    Returns ``(status, reason)``. The gate is evaluated after Holm
    correction and bootstrap CI computation. A descriptive-only gate
    never produces PASS. A superiority gate requires a corrected
    p-value below alpha and a non-zero paired risk difference. A
    non-inferiority gate requires the bootstrap CI to be within the
    declared margin.
    """
    if paired_task_count < minimum_population:
        return ClaimGateStatus.INSUFFICIENT_POPULATION, (
            f"paired task count {paired_task_count} < minimum population {minimum_population}"
        )

    if claim_gate == ClaimGate.DESCRIPTIVE_ONLY:
        return ClaimGateStatus.DESCRIPTIVE_ONLY, (
            "claim gate is descriptive-only; no superiority or non-inferiority claim"
        )

    if claim_gate == ClaimGate.SUPERIORITY:
        if mcnemar_p_value is None:
            return ClaimGateStatus.NO_DISCORDANT_PAIRS, (
                "no discordant pairs; McNemar p-value is undefined"
            )
        if corrected_p_value is None or corrected_p_value > significance_level:
            return ClaimGateStatus.FAIL, (
                f"corrected p-value {corrected_p_value} > significance level {significance_level}"
            )
        if paired_risk_difference is None or paired_risk_difference == 0.0:
            return ClaimGateStatus.FAIL, (
                "paired risk difference is zero or undefined"
            )
        return ClaimGateStatus.PASS, (
            f"superiority: corrected p-value {corrected_p_value} <= {significance_level}, "
            f"risk difference {paired_risk_difference}"
        )

    if claim_gate == ClaimGate.NON_INFERIORITY:
        if non_inferiority_margin is None:
            return ClaimGateStatus.FAIL, (
                "non-inferiority gate requires a declared margin"
            )
        if bootstrap_ci_lower is None or bootstrap_ci_upper is None:
            return ClaimGateStatus.INSUFFICIENT_POPULATION, (
                "bootstrap CI is undefined; insufficient paired tasks"
            )
        if bootstrap_ci_lower >= -non_inferiority_margin:
            return ClaimGateStatus.PASS, (
                f"non-inferior: CI lower {bootstrap_ci_lower} >= -margin {-non_inferiority_margin}"
            )
        return ClaimGateStatus.FAIL, (
            f"not non-inferior: CI lower {bootstrap_ci_lower} < -margin {-non_inferiority_margin}"
        )

    return ClaimGateStatus.FAIL, f"unknown claim gate: {claim_gate}"


class ModelComparisonAuthorityHashes(BaseModel):
    """Frozen bundle of authority content hashes bound to the output.

    Every hash binds the output to a frozen authority. A changed
    authority invalidates the output. The hashes are supplied by
    the caller from the accepted authorities; the engine does not
    recompute them.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    aggregate_verification_hash: str = Field(
        min_length=64, max_length=64,
        description="Content hash of the accepted aggregate verification report.",
    )
    campaign_set_plan_hash: str = Field(
        min_length=64, max_length=64,
        description="Content hash of the frozen campaign-set plan.",
    )
    campaign_set_index_hash: str = Field(
        min_length=64, max_length=64,
        description="Content hash of the post-execution campaign-set index.",
    )
    profile_hash: str = Field(
        min_length=64, max_length=64,
        description="Content hash of the frozen campaign profile.",
    )
    registry_hash: str = Field(
        min_length=64, max_length=64,
        description="Content hash of the frozen model registry.",
    )
    benchmark_population_hash: str = Field(
        min_length=64, max_length=64,
        description="Content hash of the frozen benchmark population.",
    )
    metric_registry_hash: str = Field(
        min_length=64, max_length=64,
        description="Content hash of the metric registry (bound via code/metric identity).",
    )


def compute_model_comparison(
    preregistration: ModelComparisonPreregistration,
    repetition_cells: Sequence[RepetitionCell],
    authority_hashes: ModelComparisonAuthorityHashes,
) -> ModelComparisonOutput:
    """Compute the canonical model comparison from raw repetition cells.

    The entry point for the model comparison engine. Pairs raw cells
    by exact ``(task_id, repetition_id)`` identity, reduces matched
    repetitions to majority binary outcomes, computes exact McNemar
    and paired risk difference, applies Holm-Bonferroni correction
    within each family, evaluates the claim gate, and binds the
    output to every frozen authority by content hash.

    The computation is deterministic: identical inputs and engine
    version produce byte-identical output. Unauthorized comparisons
    (pair keys not in the preregistration) are rejected before any
    inference runs.
    """
    _validate_repetition_cells_against_preregistration(preregistration, repetition_cells)

    results_by_pair: dict[tuple[str, str, str], ModelComparisonResult] = {}

    family_groups: dict[str, list[tuple[str, str, str]]] = {}
    family_p_values: dict[str, list[float | None]] = {}

    for pk in preregistration.pair_keys:
        candidate_cells = [
            c for c in repetition_cells
            if c.variant_id == pk.candidate_variant_id
        ]
        anchor_cells = [
            c for c in repetition_cells
            if c.variant_id == pk.anchor_variant_id
        ]

        _validate_repetition_id_matching(candidate_cells, anchor_cells)

        candidate_outcomes = _reduce_cells_by_task(
            candidate_cells,
            preregistration.repetition_count,
            preregistration.missingness_policy,
        )
        anchor_outcomes = _reduce_cells_by_task(
            anchor_cells,
            preregistration.repetition_count,
            preregistration.missingness_policy,
        )

        paired = pair_task_majority_outcomes(candidate_outcomes, anchor_outcomes)

        inferential_paired = [p for p in paired if p.in_inferential_population]
        candidate_binary = [p.candidate_majority for p in inferential_paired if p.candidate_majority is not None]
        anchor_binary = [p.anchor_majority for p in inferential_paired if p.anchor_majority is not None]

        paired_task_count = len(inferential_paired)
        rejected_task_count = len(paired) - paired_task_count
        total_task_count = len(paired)

        candidate_pass_count = sum(1 for v in candidate_binary if v)
        anchor_pass_count = sum(1 for v in anchor_binary if v)

        if paired_task_count > 0:
            candidate_pass_rate = _round(candidate_pass_count / paired_task_count)
            anchor_pass_rate = _round(anchor_pass_count / paired_task_count)
            paired_risk_difference = _round(candidate_pass_rate - anchor_pass_rate)
        else:
            candidate_pass_rate = None
            anchor_pass_rate = None
            paired_risk_difference = None

        mcnemar_stat, mcnemar_p = _exact_mcnemar(candidate_binary, anchor_binary)

        ci_lower, ci_upper = _task_cluster_bootstrap_ci(
            candidate_binary,
            anchor_binary,
            n_bootstrap=preregistration.bootstrap_count,
            confidence=preregistration.bootstrap_confidence,
            seed=preregistration.bootstrap_seed,
        )

        pair_key_tuple = (pk.candidate_variant_id, pk.anchor_variant_id, pk.family_name)
        results_by_pair[pair_key_tuple] = ModelComparisonResult(
            candidate_variant_id=pk.candidate_variant_id,
            anchor_variant_id=pk.anchor_variant_id,
            family_kind=pk.family_kind,
            family_name=pk.family_name,
            weight_class=pk.weight_class,
            paired_task_count=paired_task_count,
            rejected_task_count=rejected_task_count,
            total_task_count=total_task_count,
            candidate_pass_count=candidate_pass_count,
            anchor_pass_count=anchor_pass_count,
            candidate_pass_rate=candidate_pass_rate,
            anchor_pass_rate=anchor_pass_rate,
            paired_risk_difference=paired_risk_difference,
            mcnemar_statistic=mcnemar_stat,
            mcnemar_p_value=mcnemar_p,
            bootstrap_ci_lower=ci_lower,
            bootstrap_ci_upper=ci_upper,
            holm_corrected_p_value=None,
            holm_rank=None,
            claim_gate_status=ClaimGateStatus.DESCRIPTIVE_ONLY,
            claim_gate_reason="pending Holm correction",
            paired_task_outcomes=paired,
        )

        family_groups.setdefault(pk.family_name, []).append(pair_key_tuple)
        family_p_values.setdefault(pk.family_name, []).append(mcnemar_p)

    for family_name, pair_keys_in_family in family_groups.items():
        p_values_raw = family_p_values[family_name]
        p_values_for_correction = [p if p is not None else 1.0 for p in p_values_raw]
        holm_results = statistics.holm_correction(p_values_for_correction)

        for pair_key_tuple, (rank, corrected_p) in zip(
            pair_keys_in_family, holm_results, strict=True,
        ):
            result = results_by_pair[pair_key_tuple]
            status, reason = _evaluate_claim_gate(
                preregistration.claim_gate,
                corrected_p,
                result.paired_risk_difference,
                result.bootstrap_ci_lower,
                result.bootstrap_ci_upper,
                preregistration.non_inferiority_margin,
                preregistration.significance_level,
                result.paired_task_count,
                preregistration.minimum_population,
                result.mcnemar_p_value,
            )
            results_by_pair[pair_key_tuple] = result.model_copy(update={
                "holm_corrected_p_value": corrected_p,
                "holm_rank": rank,
                "claim_gate_status": status,
                "claim_gate_reason": reason,
            })

    sorted_results = sorted(
        results_by_pair.values(),
        key=lambda r: (r.family_kind.value, r.family_name, r.candidate_variant_id, r.anchor_variant_id),
    )

    code_metric_hash = compute_code_metric_identity_hash(
        MODEL_COMPARISON_ENGINE_VERSION,
        authority_hashes.metric_registry_hash,
    )

    output = ModelComparisonOutput.model_construct(
        engine_version=MODEL_COMPARISON_ENGINE_VERSION,
        authority_hash=preregistration.content_hash,
        aggregate_verification_hash=authority_hashes.aggregate_verification_hash,
        campaign_set_plan_hash=authority_hashes.campaign_set_plan_hash,
        campaign_set_index_hash=authority_hashes.campaign_set_index_hash,
        profile_hash=authority_hashes.profile_hash,
        registry_hash=authority_hashes.registry_hash,
        benchmark_population_hash=authority_hashes.benchmark_population_hash,
        code_metric_identity_hash=code_metric_hash,
        environment_stratum=preregistration.environment_stratum,
        results=sorted_results,
        content_hash="0" * 64,
    )

    expected_hash = compute_model_comparison_output_hash(output)
    return output.model_copy(update={"content_hash": expected_hash})


def _reduce_cells_by_task(
    cells: Sequence[RepetitionCell],
    repetition_count: int,
    missingness_policy: MissingnessPolicy,
) -> list[TaskMajorityOutcome]:
    """Group repetition cells by task and reduce each task to a majority outcome.

    Each cell is identified by its ``(task_id, repetition_id)`` pair.
    Duplicate ``repetition_id`` values within a task are rejected so
    that the matched repetitions are unambiguous.
    """
    cells_by_task: dict[str, list[RepetitionCell]] = {}
    for cell in cells:
        cells_by_task.setdefault(cell.task_id, []).append(cell)

    outcomes: list[TaskMajorityOutcome] = []
    for task_id in sorted(cells_by_task.keys()):
        task_cells = cells_by_task[task_id]
        if len(task_cells) != repetition_count:
            raise ValueError(
                f"task {task_id!r} has {len(task_cells)} repetition cells, "
                f"expected {repetition_count}"
            )
        seen_repetition_ids: set[str] = set()
        for cell in task_cells:
            if cell.repetition_id in seen_repetition_ids:
                raise ValueError(
                    f"duplicate repetition_id {cell.repetition_id!r} "
                    f"for task {task_id!r} variant {cell.variant_id!r}"
                )
            seen_repetition_ids.add(cell.repetition_id)
        outcomes.append(
            reduce_repetitions_to_majority(task_cells, repetition_count, missingness_policy)
        )
    return outcomes


def _validate_repetition_id_matching(
    candidate_cells: Sequence[RepetitionCell],
    anchor_cells: Sequence[RepetitionCell],
) -> None:
    """Reject paired tasks whose repetition_id sets differ between variants.

    Raw cells are matched by exact accepted ``(task_id, repetition_id)``
    identity. A task present in both variants must have the same set of
    ``repetition_id`` values; a mismatch means the two variants did not
    run the same repetitions and the pair is not comparable.
    """
    candidate_by_task: dict[str, set[str]] = {}
    for cell in candidate_cells:
        candidate_by_task.setdefault(cell.task_id, set()).add(cell.repetition_id)
    anchor_by_task: dict[str, set[str]] = {}
    for cell in anchor_cells:
        anchor_by_task.setdefault(cell.task_id, set()).add(cell.repetition_id)

    common_tasks = set(candidate_by_task.keys()) & set(anchor_by_task.keys())
    for task_id in sorted(common_tasks):
        candidate_ids = candidate_by_task[task_id]
        anchor_ids = anchor_by_task[task_id]
        if candidate_ids != anchor_ids:
            raise ValueError(
                f"repetition_id mismatch for task {task_id!r}: "
                f"candidate={sorted(candidate_ids)}, anchor={sorted(anchor_ids)}"
            )


def _validate_repetition_cells_against_preregistration(
    preregistration: ModelComparisonPreregistration,
    cells: Sequence[RepetitionCell],
) -> None:
    """Reject repetition cells for variants not in the preregistration.

    Every cell's variant ID must be either a candidate or an anchor
    in the preregistration pair keys. A cell for an unauthorized
    variant is rejected before any inference runs.
    """
    authorized_variants: set[str] = {preregistration.global_anchor_variant_id}
    for ca in preregistration.class_anchors:
        authorized_variants.add(ca.anchor_variant_id)
    for pk in preregistration.pair_keys:
        authorized_variants.add(pk.candidate_variant_id)
        authorized_variants.add(pk.anchor_variant_id)

    for cell in cells:
        if cell.variant_id not in authorized_variants:
            raise ValueError(
                f"repetition cell for unauthorized variant {cell.variant_id!r} "
                f"task {cell.task_id!r} repetition {cell.repetition_id!r}"
            )


__all__ = [
    "MODEL_COMPARISON_AUTHORITY_VERSION",
    "MODEL_COMPARISON_ENGINE_VERSION",
    "ClaimGate",
    "ClaimGateStatus",
    "ClassAnchorBinding",
    "ComparisonFamilyKind",
    "ComparisonPairKey",
    "ComparisonTest",
    "CorrectionMethod",
    "MissingnessPolicy",
    "ModelComparisonAuthorityHashes",
    "ModelComparisonOutput",
    "ModelComparisonPreregistration",
    "ModelComparisonResult",
    "PairedTaskOutcome",
    "RepetitionCell",
    "RepetitionReductionPolicy",
    "TaskMajorityOutcome",
    "compute_code_metric_identity_hash",
    "compute_model_comparison",
    "compute_model_comparison_hash",
    "compute_model_comparison_output_hash",
    "pair_task_majority_outcomes",
    "reduce_repetitions_to_majority",
]
