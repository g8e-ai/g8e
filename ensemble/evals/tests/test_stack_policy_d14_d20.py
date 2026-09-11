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
content hash validation, and rejection of invalid inputs. No
external dependencies (no files, network, or DB).
"""

from __future__ import annotations

import pytest
from pydantic import ValidationError

from g8e_evals.stack_policy import (
    STACK_POLICY_VERSION,
    FamilyPreference,
    HomogeneousFamilyRule,
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

    def test_rejects_duplicate_selectors(self):
        sel = StackSelectorDefinition(
            selector_id=StackSelectorId.ACCURACY,
            description="test",
            requires_lineage_diversity=False,
            tie_breaker_order=list(StackTieBreakerKey),
        )
        with pytest.raises(ValidationError, match="duplicate selector_id"):
            StackPolicy(
                policy_id="test",
                policy_version=STACK_POLICY_VERSION,
                selectors=[sel, sel],
                homogeneous_family_rule=HomogeneousFamilyRule(
                    preference_order=[FamilyPreference.QWEN],
                    condition="test",
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
                        requires_lineage_diversity=False,
                        tie_breaker_order=list(StackTieBreakerKey),
                    )
                ],
                homogeneous_family_rule=HomogeneousFamilyRule(
                    preference_order=[FamilyPreference.QWEN],
                    condition="test",
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
                homogeneous_family_rule=policy.homogeneous_family_rule,
                content_hash=policy.content_hash,
                unknown_field="bad",
            )


class TestHomogeneousFamilyRule:
    def test_builds_valid_rule(self):
        rule = HomogeneousFamilyRule(
            preference_order=[
                FamilyPreference.QWEN,
                FamilyPreference.GRANITE,
                FamilyPreference.LEXICOGRAPHIC,
            ],
            condition="at least one family eligible in all roles",
        )
        assert len(rule.preference_order) == 3

    def test_rejects_unknown_field(self):
        with pytest.raises(ValidationError):
            HomogeneousFamilyRule(
                preference_order=[FamilyPreference.QWEN],
                condition="test",
                unknown_field="bad",
            )

    def test_rejects_empty_preference_order(self):
        with pytest.raises(ValidationError):
            HomogeneousFamilyRule(
                preference_order=[],
                condition="test",
            )
