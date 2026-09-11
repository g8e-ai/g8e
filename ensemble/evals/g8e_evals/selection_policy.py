# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""D8/D11/D17 Phase B selection policy for role-competition ranking.

Encodes the frozen Phase B selection policy that ranks model variants
for the Primary, Assistant, and Light roles after accepted Phase A
evidence exists. The policy is frozen before any Phase A evidence is
inspected and produces byte-identical ordered outputs from identical
accepted evidence.

D8 category normalization: use category-normalized role-selection
scores so task count does not let IFEval dominate. Each of the nine
categories contributes equally to the role-selection macro-average
regardless of task count.

D11 ranking: require complete terminal coverage, disqualify
unresolved infrastructure or security-policy failures, rank by
role-specific category macro-average, then tie-break by missed
escalations, median warm latency, peak memory, and stable variant ID.

D17 finalist count: advance exactly five candidates per role. One
variant may appear in multiple role lists.

Every model is frozen with ``extra="forbid"``. Content hashes are
SHA-256 over canonical JSON (sorted keys, no extra whitespace).
"""

from __future__ import annotations

import hashlib
import json
import math
from enum import StrEnum
from typing import Self

from pydantic import BaseModel, ConfigDict, Field, model_validator


PHASE_B_SELECTION_POLICY_VERSION = "1.0.0"

FINALIST_COUNT_PER_ROLE = 5

VALID_ROLES: tuple[str, ...] = ("primary", "assistant", "lite")


def _sha256(data: str) -> str:
    return hashlib.sha256(data.encode()).hexdigest()


class SelectionCategory(StrEnum):
    """The nine evaluation categories for D8 category normalization.

    Each category contributes equally to the role-selection
    macro-average regardless of task count. IFEval
    (instruction_adherence) does not dominate despite having 120 tasks
    while other categories have 1-4 tasks.
    """

    INSTRUCTION_ADHERENCE = "instruction_adherence"
    TOOL_SELECTION = "tool_selection"
    TOOL_ARGUMENTS = "tool_arguments"
    TECHNICAL_ANALYSIS = "technical_analysis"
    ROUTING_DELEGATION = "routing_delegation"
    VERIFICATION = "verification"
    SECURITY_POLICY = "security_policy"
    RECOVERY = "recovery"
    FINAL_RESPONSE = "final_response"


class DisqualificationReason(StrEnum):
    """Typed disqualification reason for a variant in a role.

    ``INFRASTRUCTURE_FAILURE``: Unresolved infrastructure failure
    prevents the variant from being ranked.
    ``SECURITY_POLICY_FAILURE``: Security or policy failure
    disqualifies the variant.
    ``INCOMPLETE_TERMINAL_COVERAGE``: The variant does not have
    complete terminal coverage (one or more assignments lack a
    terminal outcome).
    """

    INFRASTRUCTURE_FAILURE = "infrastructure_failure"
    SECURITY_POLICY_FAILURE = "security_policy_failure"
    INCOMPLETE_TERMINAL_COVERAGE = "incomplete_terminal_coverage"


class MissingnessPolicy(StrEnum):
    DISQUALIFY = "disqualify"


class FinalistShortfallPolicy(StrEnum):
    FAIL_CLOSED = "fail_closed"


class CategoryWeight(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    category: SelectionCategory
    weight: float = Field(gt=0.0, le=1.0)


class RoleSelectionFormula(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    role: str = Field(min_length=1)
    category_weights: list[CategoryWeight] = Field(min_length=1)

    @model_validator(mode="after")
    def _validate_formula(self) -> Self:
        if self.role not in VALID_ROLES:
            raise ValueError(f"role must be one of {sorted(VALID_ROLES)}: got {self.role!r}")
        categories = [weight.category for weight in self.category_weights]
        if len(categories) != len(set(categories)):
            raise ValueError(f"duplicate category in role formula: {categories}")
        if set(categories) != set(SelectionCategory):
            raise ValueError("role formula must include all selection categories")
        if not math.isclose(sum(weight.weight for weight in self.category_weights), 1.0):
            raise ValueError("role formula category weights must sum to 1")
        return self


class TieBreakerKey(StrEnum):
    """Ordered tie-breaker keys for D11 Phase B ranking.

    Tie-breakers are applied in the order listed: missed escalations
    (fewer is better), median warm latency (lower is better), peak
    memory (lower is better), stable variant ID (lexicographic
    ascending).
    """

    MISSED_ESCALATIONS = "missed_escalations"
    MEDIAN_WARM_LATENCY = "median_warm_latency"
    PEAK_MEMORY = "peak_memory"
    VARIANT_ID = "variant_id"


class CategoryScore(BaseModel):
    """One category's macro-average score for a variant in a role.

    The score is the unweighted mean of all task-level scores in
    this category for this variant and role. The denominator is
    the number of tasks in the category with terminal outcomes for
    this variant.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    category: SelectionCategory = Field(description="Evaluation category.")
    score: float = Field(ge=0.0, le=1.0, description="Macro-average score for this category.")
    task_count: int = Field(ge=0, description="Number of tasks with terminal outcomes in this category.")
    numerator: int = Field(ge=0, description="Number of passing tasks in this category.")
    denominator: int = Field(ge=1, description="Number of terminal tasks in this category (the denominator).")

    @model_validator(mode="after")
    def _validate_counts(self) -> Self:
        if self.task_count != self.denominator:
            raise ValueError("task_count must equal denominator")
        if self.numerator > self.denominator:
            raise ValueError("numerator must not exceed denominator")
        expected_score = self.numerator / self.denominator
        if not math.isclose(self.score, expected_score):
            raise ValueError(f"score must equal numerator / denominator: {expected_score}")
        return self


class VariantRoleScore(BaseModel):
    """Complete scoring record for one variant in one role.

    Binds the variant ID, role, per-category scores, macro-average
    across categories, tie-breaker values, and disqualification status.
    The macro-average is the unweighted mean of per-category scores
    across all nine categories (D8 normalization).
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    variant_id: str = Field(min_length=1, description="Model variant ID from the registry.")
    role: str = Field(min_length=1, description="Role: primary, assistant, or lite.")
    category_scores: list[CategoryScore] = Field(
        min_length=1,
        description="Per-category macro-average scores.",
    )
    macro_average: float = Field(
        ge=0.0, le=1.0,
        description="Unweighted mean of per-category scores (D8 normalization).",
    )
    missed_escalations: int = Field(
        ge=0,
        description="Number of missed escalations (tie-breaker 1, fewer is better).",
    )
    median_warm_latency_ms: float = Field(
        ge=0.0,
        description="Median warm latency in milliseconds (tie-breaker 2, lower is better).",
    )
    peak_memory_mb: float = Field(
        ge=0.0,
        description="Peak memory in MB (tie-breaker 3, lower is better).",
    )
    disqualified: bool = Field(
        default=False,
        description="Whether this variant is disqualified for this role.",
    )
    disqualification_reason: DisqualificationReason | None = Field(
        default=None,
        description="Typed disqualification reason, None when not disqualified.",
    )

    @model_validator(mode="after")
    def _validate_score(self) -> Self:
        if self.role not in VALID_ROLES:
            raise ValueError(
                f"role must be one of {sorted(VALID_ROLES)}: got {self.role!r}"
            )
        if self.disqualified and self.disqualification_reason is None:
            raise ValueError(
                f"disqualified variant {self.variant_id!r} must have a disqualification_reason"
            )
        if not self.disqualified and self.disqualification_reason is not None:
            raise ValueError(
                f"non-disqualified variant {self.variant_id!r} must have disqualification_reason=None"
            )
        categories = [cs.category for cs in self.category_scores]
        if len(categories) != len(set(categories)):
            raise ValueError(
                f"duplicate category in category_scores: {categories}"
            )
        if set(categories) != set(SelectionCategory):
            raise ValueError("category_scores must include all selection categories")
        expected_macro_average = compute_macro_average(self.category_scores)
        if not math.isclose(self.macro_average, expected_macro_average):
            raise ValueError(
                f"macro_average must equal the category macro-average: {expected_macro_average}"
            )
        return self


def _default_role_formulas() -> list[RoleSelectionFormula]:
    category_weight = 1.0 / len(SelectionCategory)
    return [
        RoleSelectionFormula(
            role=role,
            category_weights=[
                CategoryWeight(category=category, weight=category_weight)
                for category in SelectionCategory
            ],
        )
        for role in VALID_ROLES
    ]


class PhaseBSelectionPolicy(BaseModel):
    """Frozen Phase B selection policy encoding D8, D11, and D17.

    Binds the policy version, finalist count, valid roles, category
    set, tie-breaker order, and a content hash. The policy is frozen
    before any Phase A evidence is inspected and produces byte-identical
    ordered outputs from identical accepted evidence.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    policy_id: str = Field(min_length=1, description="Unique policy identifier.")
    policy_version: str = Field(min_length=1, description="Policy schema version.")
    finalist_count: int = Field(
        ge=1,
        description="Number of finalists to advance per role (D17).",
    )
    valid_roles: list[str] = Field(
        min_length=1,
        description="Valid role names (primary, assistant, lite).",
    )
    categories: list[SelectionCategory] = Field(
        min_length=1,
        description="Evaluation categories for D8 normalization.",
    )
    tie_breaker_order: list[TieBreakerKey] = Field(
        min_length=1,
        description="Ordered tie-breaker keys for D11 ranking.",
    )
    require_complete_terminal_coverage: bool = True
    missingness_policy: MissingnessPolicy = MissingnessPolicy.DISQUALIFY
    finalist_shortfall_policy: FinalistShortfallPolicy = FinalistShortfallPolicy.FAIL_CLOSED
    role_formulas: list[RoleSelectionFormula] = Field(default_factory=_default_role_formulas)
    content_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 over canonical JSON of the policy.",
    )

    @model_validator(mode="after")
    def _validate_policy(self) -> Self:
        if self.finalist_count != FINALIST_COUNT_PER_ROLE:
            raise ValueError(
                f"finalist_count must be {FINALIST_COUNT_PER_ROLE} (D17): "
                f"got {self.finalist_count}"
            )
        if self.valid_roles != list(VALID_ROLES):
            raise ValueError(
                f"valid_roles must be {list(VALID_ROLES)}: got {self.valid_roles}"
            )
        if len(self.categories) != len(set(self.categories)):
            raise ValueError(
                f"duplicate category in categories: {self.categories}"
            )
        if len(self.tie_breaker_order) != len(set(self.tie_breaker_order)):
            raise ValueError(
                f"duplicate tie-breaker in tie_breaker_order: {self.tie_breaker_order}"
            )
        if not self.require_complete_terminal_coverage:
            raise ValueError("complete terminal coverage is required")
        formula_roles = [formula.role for formula in self.role_formulas]
        if formula_roles != list(VALID_ROLES):
            raise ValueError(f"role_formulas must be ordered as {list(VALID_ROLES)}")
        expected = compute_selection_policy_hash(
            policy_id=self.policy_id,
            policy_version=self.policy_version,
            finalist_count=self.finalist_count,
            valid_roles=self.valid_roles,
            categories=self.categories,
            tie_breaker_order=self.tie_breaker_order,
            require_complete_terminal_coverage=self.require_complete_terminal_coverage,
            missingness_policy=self.missingness_policy,
            finalist_shortfall_policy=self.finalist_shortfall_policy,
            role_formulas=self.role_formulas,
        )
        if self.content_hash != expected:
            raise ValueError(
                f"Phase B selection policy content_hash mismatch: "
                f"declared {self.content_hash!r}, computed {expected!r}"
            )
        return self


def compute_selection_policy_hash(
    *,
    policy_id: str,
    policy_version: str,
    finalist_count: int,
    valid_roles: list[str],
    categories: list[SelectionCategory],
    tie_breaker_order: list[TieBreakerKey],
    require_complete_terminal_coverage: bool = True,
    missingness_policy: MissingnessPolicy = MissingnessPolicy.DISQUALIFY,
    finalist_shortfall_policy: FinalistShortfallPolicy = FinalistShortfallPolicy.FAIL_CLOSED,
    role_formulas: list[RoleSelectionFormula] | None = None,
) -> str:
    """Compute the content hash for a Phase B selection policy."""
    if role_formulas is None:
        role_formulas = _default_role_formulas()
    payload = json.dumps(
        {
            "policy_id": policy_id,
            "policy_version": policy_version,
            "finalist_count": finalist_count,
            "valid_roles": sorted(valid_roles),
            "categories": sorted(c.value for c in categories),
            "tie_breaker_order": [t.value for t in tie_breaker_order],
            "require_complete_terminal_coverage": require_complete_terminal_coverage,
            "missingness_policy": missingness_policy.value,
            "finalist_shortfall_policy": finalist_shortfall_policy.value,
            "role_formulas": [
                formula.model_dump(mode="json")
                for formula in role_formulas
            ],
        },
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


def build_phase_b_selection_policy(
    *,
    policy_id: str = "phase-b-selection-policy",
) -> PhaseBSelectionPolicy:
    """Build the frozen Phase B selection policy encoding D8, D11, and D17.

    The policy is frozen with:
    - D17: exactly five finalists per role.
    - D8: nine categories with unweighted macro-average normalization.
    - D11: tie-breaker order of missed escalations, median warm
      latency, peak memory, variant ID.
    """
    categories = list(SelectionCategory)
    tie_breakers = list(TieBreakerKey)
    content_hash = compute_selection_policy_hash(
        policy_id=policy_id,
        policy_version=PHASE_B_SELECTION_POLICY_VERSION,
        finalist_count=FINALIST_COUNT_PER_ROLE,
        valid_roles=list(VALID_ROLES),
        categories=categories,
        tie_breaker_order=tie_breakers,
    )
    return PhaseBSelectionPolicy(
        policy_id=policy_id,
        policy_version=PHASE_B_SELECTION_POLICY_VERSION,
        finalist_count=FINALIST_COUNT_PER_ROLE,
        valid_roles=list(VALID_ROLES),
        categories=categories,
        tie_breaker_order=tie_breakers,
        content_hash=content_hash,
    )


def compute_macro_average(category_scores: list[CategoryScore]) -> float:
    """Compute the D8 unweighted macro-average across category scores.

    Each category contributes equally regardless of task count. The
    macro-average is the unweighted mean of per-category scores.
    """
    if not category_scores:
        return 0.0
    return sum(cs.score for cs in category_scores) / len(category_scores)


def rank_variants_for_role(
    scores: list[VariantRoleScore],
    finalist_count: int = FINALIST_COUNT_PER_ROLE,
) -> list[VariantRoleScore]:
    """Rank variants for one role and return the leading finalists.

    Disqualified variants are excluded from ranking. Eligible variants
    are ranked by macro-average descending, then by the D11 tie-breaker
    order: missed escalations ascending, median warm latency ascending,
    peak memory ascending, variant ID lexicographic ascending.

    Returns exactly ``finalist_count`` variants and fails closed when
    fewer eligible variants exist.
    """
    eligible = [s for s in scores if not s.disqualified]
    eligible.sort(
        key=lambda s: (
            -s.macro_average,
            s.missed_escalations,
            s.median_warm_latency_ms,
            s.peak_memory_mb,
            s.variant_id,
        )
    )
    if len(eligible) < finalist_count:
        raise ValueError(
            f"exactly {finalist_count} eligible finalists are required: got {len(eligible)}"
        )
    return eligible[:finalist_count]


__all__ = [
    "FINALIST_COUNT_PER_ROLE",
    "PHASE_B_SELECTION_POLICY_VERSION",
    "VALID_ROLES",
    "CategoryScore",
    "CategoryWeight",
    "DisqualificationReason",
    "FinalistShortfallPolicy",
    "MissingnessPolicy",
    "PhaseBSelectionPolicy",
    "RoleSelectionFormula",
    "SelectionCategory",
    "TieBreakerKey",
    "VariantRoleScore",
    "build_phase_b_selection_policy",
    "compute_macro_average",
    "compute_selection_policy_hash",
    "rank_variants_for_role",
]
