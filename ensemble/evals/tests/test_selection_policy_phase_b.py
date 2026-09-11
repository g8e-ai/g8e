# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for the D8/D11/D17 Phase B selection policy.

Verifies that the policy encodes exactly five finalists per role
(D17), nine categories for D8 normalization, the D11 tie-breaker
order, macro-average computation, variant ranking, disqualification
handling, and content hash validation. No external dependencies (no
files, network, or DB).
"""

from __future__ import annotations

import pytest
from pydantic import ValidationError

from g8e_evals.selection_policy import (
    FINALIST_COUNT_PER_ROLE,
    PHASE_B_SELECTION_POLICY_VERSION,
    VALID_ROLES,
    CategoryScore,
    DisqualificationReason,
    PhaseBSelectionPolicy,
    SelectionCategory,
    TieBreakerKey,
    VariantRoleScore,
    build_phase_b_selection_policy,
    compute_macro_average,
    rank_variants_for_role,
)


pytestmark = pytest.mark.unit


def _make_category_score(
    category: SelectionCategory = SelectionCategory.INSTRUCTION_ADHERENCE,
    score: float = 0.8,
    task_count: int = 10,
    numerator: int = 8,
    denominator: int = 10,
) -> CategoryScore:
    return CategoryScore(
        category=category,
        score=score,
        task_count=task_count,
        numerator=numerator,
        denominator=denominator,
    )


def _make_variant_score(
    variant_id: str = "qwen3-8b",
    role: str = "primary",
    macro_average: float = 0.8,
    missed_escalations: int = 0,
    median_warm_latency_ms: float = 100.0,
    peak_memory_mb: float = 4096.0,
    disqualified: bool = False,
    disqualification_reason: DisqualificationReason | None = None,
) -> VariantRoleScore:
    categories = [
        _make_category_score(SelectionCategory.INSTRUCTION_ADHERENCE, score=macro_average),
        _make_category_score(SelectionCategory.TOOL_SELECTION, score=macro_average),
        _make_category_score(SelectionCategory.TOOL_ARGUMENTS, score=macro_average),
        _make_category_score(SelectionCategory.TECHNICAL_ANALYSIS, score=macro_average),
        _make_category_score(SelectionCategory.ROUTING_DELEGATION, score=macro_average),
        _make_category_score(SelectionCategory.VERIFICATION, score=macro_average),
        _make_category_score(SelectionCategory.SECURITY_POLICY, score=macro_average),
        _make_category_score(SelectionCategory.RECOVERY, score=macro_average),
        _make_category_score(SelectionCategory.FINAL_RESPONSE, score=macro_average),
    ]
    return VariantRoleScore(
        variant_id=variant_id,
        role=role,
        category_scores=categories,
        macro_average=macro_average,
        missed_escalations=missed_escalations,
        median_warm_latency_ms=median_warm_latency_ms,
        peak_memory_mb=peak_memory_mb,
        disqualified=disqualified,
        disqualification_reason=disqualification_reason,
    )


class TestPhaseBSelectionPolicy:
    def test_builds_valid_policy(self):
        policy = build_phase_b_selection_policy()
        assert policy.finalist_count == FINALIST_COUNT_PER_ROLE
        assert policy.finalist_count == 5
        assert policy.valid_roles == list(VALID_ROLES)
        assert len(policy.categories) == 9
        assert policy.tie_breaker_order[0] == TieBreakerKey.MISSED_ESCALATIONS
        assert policy.tie_breaker_order[1] == TieBreakerKey.MEDIAN_WARM_LATENCY
        assert policy.tie_breaker_order[2] == TieBreakerKey.PEAK_MEMORY
        assert policy.tie_breaker_order[3] == TieBreakerKey.VARIANT_ID

    def test_policy_is_deterministic(self):
        policy_a = build_phase_b_selection_policy()
        policy_b = build_phase_b_selection_policy()
        assert policy_a.content_hash == policy_b.content_hash

    def test_has_nine_categories(self):
        assert len(SelectionCategory) == 9

    def test_categories_match_d8(self):
        expected = {
            "instruction_adherence",
            "tool_selection",
            "tool_arguments",
            "technical_analysis",
            "routing_delegation",
            "verification",
            "security_policy",
            "recovery",
            "final_response",
        }
        assert {c.value for c in SelectionCategory} == expected

    def test_rejects_wrong_finalist_count(self):
        with pytest.raises(ValidationError, match="finalist_count must be 5"):
            PhaseBSelectionPolicy(
                policy_id="test",
                policy_version=PHASE_B_SELECTION_POLICY_VERSION,
                finalist_count=3,
                valid_roles=list(VALID_ROLES),
                categories=list(SelectionCategory),
                tie_breaker_order=list(TieBreakerKey),
                content_hash="0" * 64,
            )

    def test_rejects_content_hash_mismatch(self):
        with pytest.raises(ValidationError, match="content_hash mismatch"):
            PhaseBSelectionPolicy(
                policy_id="test",
                policy_version=PHASE_B_SELECTION_POLICY_VERSION,
                finalist_count=FINALIST_COUNT_PER_ROLE,
                valid_roles=list(VALID_ROLES),
                categories=list(SelectionCategory),
                tie_breaker_order=list(TieBreakerKey),
                content_hash="0" * 64,
            )

    def test_rejects_unknown_field(self):
        policy = build_phase_b_selection_policy()
        with pytest.raises(ValidationError):
            PhaseBSelectionPolicy(
                policy_id=policy.policy_id,
                policy_version=policy.policy_version,
                finalist_count=policy.finalist_count,
                valid_roles=policy.valid_roles,
                categories=policy.categories,
                tie_breaker_order=policy.tie_breaker_order,
                content_hash=policy.content_hash,
                unknown_field="bad",
            )


class TestMacroAverage:
    def test_uniform_scores(self):
        scores = [
            _make_category_score(SelectionCategory.INSTRUCTION_ADHERENCE, score=0.8),
            _make_category_score(SelectionCategory.TOOL_SELECTION, score=0.8),
        ]
        assert compute_macro_average(scores) == 0.8

    def test_ifeval_does_not_dominate(self):
        scores = [
            _make_category_score(SelectionCategory.INSTRUCTION_ADHERENCE, score=1.0, task_count=120),
            _make_category_score(SelectionCategory.TOOL_SELECTION, score=0.0, task_count=4),
        ]
        avg = compute_macro_average(scores)
        assert avg == 0.5, (
            "D8 normalization: IFEval with 120 tasks should not dominate "
            "over tool_selection with 4 tasks"
        )

    def test_empty_scores(self):
        assert compute_macro_average([]) == 0.0


class TestRankVariants:
    def test_ranks_by_macro_average_descending(self):
        scores = [
            _make_variant_score("low", macro_average=0.5),
            _make_variant_score("high", macro_average=0.9),
            _make_variant_score("mid", macro_average=0.7),
        ]
        ranked = rank_variants_for_role(scores, finalist_count=2)
        assert ranked[0].variant_id == "high"
        assert ranked[1].variant_id == "mid"

    def test_tie_break_by_missed_escalations(self):
        scores = [
            _make_variant_score("a", macro_average=0.8, missed_escalations=3),
            _make_variant_score("b", macro_average=0.8, missed_escalations=1),
        ]
        ranked = rank_variants_for_role(scores, finalist_count=2)
        assert ranked[0].variant_id == "b"

    def test_tie_break_by_median_warm_latency(self):
        scores = [
            _make_variant_score("a", macro_average=0.8, median_warm_latency_ms=200.0),
            _make_variant_score("b", macro_average=0.8, median_warm_latency_ms=100.0),
        ]
        ranked = rank_variants_for_role(scores, finalist_count=2)
        assert ranked[0].variant_id == "b"

    def test_tie_break_by_peak_memory(self):
        scores = [
            _make_variant_score("a", macro_average=0.8, peak_memory_mb=8192.0),
            _make_variant_score("b", macro_average=0.8, peak_memory_mb=4096.0),
        ]
        ranked = rank_variants_for_role(scores, finalist_count=2)
        assert ranked[0].variant_id == "b"

    def test_tie_break_by_variant_id(self):
        scores = [
            _make_variant_score("zeta", macro_average=0.8),
            _make_variant_score("alpha", macro_average=0.8),
        ]
        ranked = rank_variants_for_role(scores, finalist_count=2)
        assert ranked[0].variant_id == "alpha"

    def test_excludes_disqualified(self):
        scores = [
            _make_variant_score("good", macro_average=0.7),
            _make_variant_score(
                "bad", macro_average=0.9,
                disqualified=True,
                disqualification_reason=DisqualificationReason.INFRASTRUCTURE_FAILURE,
            ),
        ]
        ranked = rank_variants_for_role(scores, finalist_count=5)
        assert len(ranked) == 1
        assert ranked[0].variant_id == "good"

    def test_returns_exactly_finalist_count(self):
        scores = [
            _make_variant_score(f"v{i}", macro_average=0.1 * i)
            for i in range(10)
        ]
        ranked = rank_variants_for_role(scores, finalist_count=FINALIST_COUNT_PER_ROLE)
        assert len(ranked) == FINALIST_COUNT_PER_ROLE

    def test_returns_fewer_when_insufficient(self):
        scores = [
            _make_variant_score("a", macro_average=0.9),
            _make_variant_score("b", macro_average=0.8),
        ]
        ranked = rank_variants_for_role(scores, finalist_count=5)
        assert len(ranked) == 2


class TestVariantRoleScore:
    def test_rejects_invalid_role(self):
        with pytest.raises(ValidationError, match="role must be one of"):
            _make_variant_score(role="invalid_role")

    def test_disqualified_requires_reason(self):
        with pytest.raises(ValidationError, match="must have a disqualification_reason"):
            VariantRoleScore(
                variant_id="test",
                role="primary",
                category_scores=[_make_category_score()],
                macro_average=0.5,
                missed_escalations=0,
                median_warm_latency_ms=100.0,
                peak_memory_mb=4096.0,
                disqualified=True,
                disqualification_reason=None,
            )

    def test_not_disqualified_rejects_reason(self):
        with pytest.raises(ValidationError, match="must have disqualification_reason=None"):
            VariantRoleScore(
                variant_id="test",
                role="primary",
                category_scores=[_make_category_score()],
                macro_average=0.5,
                missed_escalations=0,
                median_warm_latency_ms=100.0,
                peak_memory_mb=4096.0,
                disqualified=False,
                disqualification_reason=DisqualificationReason.INFRASTRUCTURE_FAILURE,
            )

    def test_rejects_duplicate_categories(self):
        with pytest.raises(ValidationError, match="duplicate category"):
            VariantRoleScore(
                variant_id="test",
                role="primary",
                category_scores=[
                    _make_category_score(SelectionCategory.INSTRUCTION_ADHERENCE),
                    _make_category_score(SelectionCategory.INSTRUCTION_ADHERENCE),
                ],
                macro_average=0.5,
                missed_escalations=0,
                median_warm_latency_ms=100.0,
                peak_memory_mb=4096.0,
                disqualified=False,
                disqualification_reason=None,
            )
