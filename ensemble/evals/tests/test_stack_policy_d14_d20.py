# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for the D14/D20 Phase D stack selector policy.

Verifies that the policy encodes exactly eight selectors (D14), the
D20 homogeneous family preference order (Qwen, Granite,
lexicographic), lineage-diversity requirements, tie-breaker order,
typed selection criteria and directions, lineage metadata source,
deduplication rule, content hash validation, and rejection of
invalid inputs. No external dependencies (no files, network, or DB).
"""

from __future__ import annotations

import pytest
from pydantic import ValidationError

from g8e_evals.stack_policy import (
    STACK_POLICY_VERSION,
    DeduplicationRule,
    FamilyPreference,
    HomogeneousFamilyEligibilityRule,
    HomogeneousFamilyRule,
    LineageMetadataSource,
    SelectorEligibilityRule,
    SelectionCriterion,
    SelectionDirection,
    StackPolicy,
    StackSelectorDefinition,
    StackSelectorId,
    StackTieBreakerKey,
    build_stack_policy,
)


pytestmark = pytest.mark.unit


class TestStackSelectorId:
    def test_has_eight_selectors(self):
        assert len(StackSelectorId) == 8

    def test_selector_ids_match_d14(self):
        expected = {
            "accuracy",
            "efficiency",
            "smallest",
            "tool_calling",
            "privacy",
            "throughput",
            "lineage_diverse_accuracy",
            "lineage_diverse_efficiency",
        }
        assert {s.value for s in StackSelectorId} == expected


class TestFamilyPreference:
    def test_has_three_preferences(self):
        assert len(FamilyPreference) == 3

    def test_qwen_first(self):
        assert FamilyPreference.QWEN == "qwen"

    def test_granite_second(self):
        assert FamilyPreference.GRANITE == "granite"

    def test_lexicographic_third(self):
        assert FamilyPreference.LEXICOGRAPHIC == "lexicographic"


class TestSelectionCriterion:
    def test_has_six_criteria(self):
        assert len(SelectionCriterion) == 6

    def test_criteria_are_typed_metrics(self):
        expected = {
            "macro_average",
            "median_warm_latency_ms",
            "parameter_count",
            "tool_calling_category_score",
            "privacy_category_score",
            "throughput_tokens_per_second",
        }
        assert {c.value for c in SelectionCriterion} == expected


class TestSelectionDirection:
    def test_has_two_directions(self):
        assert len(SelectionDirection) == 2

    def test_maximize(self):
        assert SelectionDirection.MAXIMIZE == "maximize"

    def test_minimize(self):
        assert SelectionDirection.MINIMIZE == "minimize"


class TestLineageMetadataSource:
    def test_has_one_source(self):
        assert len(LineageMetadataSource) == 1

    def test_model_registry_family_id(self):
        assert LineageMetadataSource.MODEL_REGISTRY_FAMILY_ID == "model_registry_family_id"


class TestDeduplicationRule:
    def test_has_one_rule(self):
        assert len(DeduplicationRule) == 1

    def test_keep_selector_label(self):
        assert DeduplicationRule.KEEP_SELECTOR_LABEL == "keep_selector_label"


class TestStackPolicy:
    def test_builds_valid_policy(self):
        policy = build_stack_policy()
        assert len(policy.selectors) == 8
        assert policy.homogeneous_family_rule.preference_order[0] == FamilyPreference.QWEN
        assert policy.homogeneous_family_rule.preference_order[1] == FamilyPreference.GRANITE
        assert policy.homogeneous_family_rule.preference_order[2] == FamilyPreference.LEXICOGRAPHIC

    def test_policy_is_deterministic(self):
        policy_a = build_stack_policy()
        policy_b = build_stack_policy()
        assert policy_a.content_hash == policy_b.content_hash

    def test_lineage_metadata_source_is_model_registry_family_id(self):
        policy = build_stack_policy()
        assert policy.lineage_metadata_source == LineageMetadataSource.MODEL_REGISTRY_FAMILY_ID

    def test_deduplication_rule_is_keep_selector_label(self):
        policy = build_stack_policy()
        assert policy.deduplication_rule == DeduplicationRule.KEEP_SELECTOR_LABEL

    def test_lineage_diverse_selectors_require_diversity(self):
        policy = build_stack_policy()
        for selector in policy.selectors:
            if selector.selector_id in (StackSelectorId.LINEAGE_DIVERSE_ACCURACY, StackSelectorId.LINEAGE_DIVERSE_EFFICIENCY):
                assert selector.requires_lineage_diversity is True
            else:
                assert selector.requires_lineage_diversity is False

    def test_all_selectors_have_tie_breakers(self):
        policy = build_stack_policy()
        for selector in policy.selectors:
            assert len(selector.tie_breaker_order) >= 1
            assert StackTieBreakerKey.VARIANT_ID in selector.tie_breaker_order

    def test_all_selectors_have_typed_criterion_and_direction(self):
        policy = build_stack_policy()
        for selector in policy.selectors:
            assert isinstance(selector.selection_criterion, SelectionCriterion)
            assert isinstance(selector.selection_direction, SelectionDirection)
            assert selector.eligibility_rule == SelectorEligibilityRule.PHASE_B_FINALIST

    def test_homogeneous_family_condition_is_typed(self):
        policy = build_stack_policy()
        assert policy.homogeneous_family_rule.eligibility_rule == HomogeneousFamilyEligibilityRule.ELIGIBLE_IN_ALL_ROLES

    def test_accuracy_selector_maximizes_macro_average(self):
        policy = build_stack_policy()
        sel = next(s for s in policy.selectors if s.selector_id == StackSelectorId.ACCURACY)
        assert sel.selection_criterion == SelectionCriterion.MACRO_AVERAGE
        assert sel.selection_direction == SelectionDirection.MAXIMIZE

    def test_efficiency_selector_minimizes_median_warm_latency(self):
        policy = build_stack_policy()
        sel = next(s for s in policy.selectors if s.selector_id == StackSelectorId.EFFICIENCY)
        assert sel.selection_criterion == SelectionCriterion.MEDIAN_WARM_LATENCY_MS
        assert sel.selection_direction == SelectionDirection.MINIMIZE

    def test_smallest_selector_minimizes_parameter_count(self):
        policy = build_stack_policy()
        sel = next(s for s in policy.selectors if s.selector_id == StackSelectorId.SMALLEST)
        assert sel.selection_criterion == SelectionCriterion.PARAMETER_COUNT
        assert sel.selection_direction == SelectionDirection.MINIMIZE

    def test_tool_calling_selector_maximizes_tool_calling_score(self):
        policy = build_stack_policy()
        sel = next(s for s in policy.selectors if s.selector_id == StackSelectorId.TOOL_CALLING)
        assert sel.selection_criterion == SelectionCriterion.TOOL_CALLING_CATEGORY_SCORE
        assert sel.selection_direction == SelectionDirection.MAXIMIZE

    def test_privacy_selector_maximizes_privacy_score(self):
        policy = build_stack_policy()
        sel = next(s for s in policy.selectors if s.selector_id == StackSelectorId.PRIVACY)
        assert sel.selection_criterion == SelectionCriterion.PRIVACY_CATEGORY_SCORE
        assert sel.selection_direction == SelectionDirection.MAXIMIZE

    def test_throughput_selector_maximizes_throughput(self):
        policy = build_stack_policy()
        sel = next(s for s in policy.selectors if s.selector_id == StackSelectorId.THROUGHPUT)
        assert sel.selection_criterion == SelectionCriterion.THROUGHPUT_TOKENS_PER_SECOND
        assert sel.selection_direction == SelectionDirection.MAXIMIZE

    def test_lineage_diverse_accuracy_maximizes_macro_average(self):
        policy = build_stack_policy()
        sel = next(s for s in policy.selectors if s.selector_id == StackSelectorId.LINEAGE_DIVERSE_ACCURACY)
        assert sel.selection_criterion == SelectionCriterion.MACRO_AVERAGE
        assert sel.selection_direction == SelectionDirection.MAXIMIZE

    def test_lineage_diverse_efficiency_minimizes_latency(self):
        policy = build_stack_policy()
        sel = next(s for s in policy.selectors if s.selector_id == StackSelectorId.LINEAGE_DIVERSE_EFFICIENCY)
        assert sel.selection_criterion == SelectionCriterion.MEDIAN_WARM_LATENCY_MS
        assert sel.selection_direction == SelectionDirection.MINIMIZE

    def test_no_description_contains_unspecified_best_or_top(self):
        policy = build_stack_policy()
        for selector in policy.selectors:
            desc_lower = selector.description.lower()
            assert "best" not in desc_lower, f"selector {selector.selector_id} description uses unspecified 'best': {selector.description!r}"
            assert "top" not in desc_lower, f"selector {selector.selector_id} description uses unspecified 'top': {selector.description!r}"
            assert "approximately" not in desc_lower, f"selector {selector.selector_id} description uses unspecified 'approximately': {selector.description!r}"

    def test_rejects_duplicate_selectors(self):
        sel = StackSelectorDefinition(
            selector_id=StackSelectorId.ACCURACY,
            description="test",
            selection_criterion=SelectionCriterion.MACRO_AVERAGE,
            selection_direction=SelectionDirection.MAXIMIZE,
            requires_lineage_diversity=False,
            tie_breaker_order=list(StackTieBreakerKey),
        )
        with pytest.raises(ValidationError, match="duplicate selector_id"):
            StackPolicy(
                policy_id="test",
                policy_version=STACK_POLICY_VERSION,
                selectors=[sel, sel],
                lineage_metadata_source=LineageMetadataSource.MODEL_REGISTRY_FAMILY_ID,
                deduplication_rule=DeduplicationRule.KEEP_SELECTOR_LABEL,
                homogeneous_family_rule=HomogeneousFamilyRule(
                    preference_order=[FamilyPreference.QWEN],
                    eligibility_rule=HomogeneousFamilyEligibilityRule.ELIGIBLE_IN_ALL_ROLES,
                ),
                content_hash="0" * 64,
            )

    def test_rejects_wrong_selector_count(self):
        with pytest.raises(ValidationError, match="expected 8 selectors"):
            StackPolicy(
                policy_id="test",
                policy_version=STACK_POLICY_VERSION,
                selectors=[
                    StackSelectorDefinition(
                        selector_id=StackSelectorId.ACCURACY,
                        description="test",
                        selection_criterion=SelectionCriterion.MACRO_AVERAGE,
                        selection_direction=SelectionDirection.MAXIMIZE,
                        requires_lineage_diversity=False,
                        tie_breaker_order=list(StackTieBreakerKey),
                    )
                ],
                lineage_metadata_source=LineageMetadataSource.MODEL_REGISTRY_FAMILY_ID,
                deduplication_rule=DeduplicationRule.KEEP_SELECTOR_LABEL,
                homogeneous_family_rule=HomogeneousFamilyRule(
                    preference_order=[FamilyPreference.QWEN],
                    eligibility_rule=HomogeneousFamilyEligibilityRule.ELIGIBLE_IN_ALL_ROLES,
                ),
                content_hash="0" * 64,
            )

    def test_rejects_content_hash_mismatch(self):
        policy = build_stack_policy()
        with pytest.raises(ValidationError, match="content_hash mismatch"):
            StackPolicy(
                policy_id=policy.policy_id,
                policy_version=policy.policy_version,
                selectors=policy.selectors,
                lineage_metadata_source=policy.lineage_metadata_source,
                deduplication_rule=policy.deduplication_rule,
                homogeneous_family_rule=policy.homogeneous_family_rule,
                content_hash="0" * 64,
            )

    def test_rejects_unknown_field(self):
        policy = build_stack_policy()
        with pytest.raises(ValidationError):
            StackPolicy(
                policy_id=policy.policy_id,
                policy_version=policy.policy_version,
                selectors=policy.selectors,
                lineage_metadata_source=policy.lineage_metadata_source,
                deduplication_rule=policy.deduplication_rule,
                homogeneous_family_rule=policy.homogeneous_family_rule,
                content_hash=policy.content_hash,
                unknown_field="bad",  # type: ignore
            )


class TestHomogeneousFamilyRule:
    def test_builds_valid_rule(self):
        rule = HomogeneousFamilyRule(
            preference_order=[
                FamilyPreference.QWEN,
                FamilyPreference.GRANITE,
                FamilyPreference.LEXICOGRAPHIC,
            ],
            eligibility_rule=HomogeneousFamilyEligibilityRule.ELIGIBLE_IN_ALL_ROLES,
        )
        assert len(rule.preference_order) == 3

    def test_rejects_unknown_field(self):
        with pytest.raises(ValidationError):
            HomogeneousFamilyRule(
                preference_order=[FamilyPreference.QWEN],
                eligibility_rule=HomogeneousFamilyEligibilityRule.ELIGIBLE_IN_ALL_ROLES,
                unknown_field="bad",  # type: ignore
            )

    def test_rejects_empty_preference_order(self):
        with pytest.raises(ValidationError):
            HomogeneousFamilyRule(
                preference_order=[],
                eligibility_rule=HomogeneousFamilyEligibilityRule.ELIGIBLE_IN_ALL_ROLES,
            )
