# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for the scenario metadata model.

Verifies that the ScenarioMetadata model validates role-complexity
matching, rejects unknown fields, is frozen, and that GraderCriterion
validates all nine criterion kinds.

No external dependencies (no files, network, or DB).
"""

from __future__ import annotations

import pytest
from pydantic import ValidationError

from g8e_evals.benchmarks.scenarios.metadata import (
    ExpectedRole,
    GraderCriterion,
    ScenarioCategory,
    ScenarioComplexity,
    ScenarioMetadata,
)


pytestmark = pytest.mark.unit


class TestScenarioCategory:
    def test_has_nine_categories(self):
        assert len(ScenarioCategory) == 9

    def test_categories_match_plan(self):
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
        assert {c.value for c in ScenarioCategory} == expected


class TestScenarioComplexity:
    def test_has_three_complexity_levels(self):
        assert len(ScenarioComplexity) == 3

    def test_complexity_values(self):
        assert ScenarioComplexity.LIGHT == "light"
        assert ScenarioComplexity.ASSISTANT == "assistant"
        assert ScenarioComplexity.PRIMARY == "primary"


class TestExpectedRole:
    def test_has_three_roles(self):
        assert len(ExpectedRole) == 3

    def test_role_values(self):
        assert ExpectedRole.PRIMARY == "primary"
        assert ExpectedRole.ASSISTANT == "assistant"
        assert ExpectedRole.LIGHT == "light"


class TestGraderCriterion:
    def test_criterion_is_frozen_with_extra_forbid(self):
        with pytest.raises(ValidationError, match="extra_forbidden"):
            GraderCriterion(
                kind="keyword_present",
                value="error",
                unknown_field="rejected",
            )

    def test_criterion_kind_must_be_nonempty(self):
        with pytest.raises(ValidationError):
            GraderCriterion(kind="", value="test")

    def test_criterion_must_pass_defaults_to_true(self):
        criterion = GraderCriterion(kind="keyword_present", value="test")
        assert criterion.must_pass is True

    def test_criterion_must_pass_can_be_false(self):
        criterion = GraderCriterion(kind="keyword_present", value="test", must_pass=False)
        assert criterion.must_pass is False


class TestScenarioMetadata:
    def _make_valid_criterion(self) -> GraderCriterion:
        return GraderCriterion(kind="keyword_present", value="pass")

    def test_metadata_is_frozen_with_extra_forbid(self):
        with pytest.raises(ValidationError, match="extra_forbidden"):
            ScenarioMetadata(
                category=ScenarioCategory.TOOL_SELECTION,
                complexity=ScenarioComplexity.LIGHT,
                expected_role=ExpectedRole.LIGHT,
                criteria=[self._make_valid_criterion()],
                unknown_field="rejected",
            )

    def test_metadata_requires_at_least_one_criterion(self):
        with pytest.raises(ValidationError):
            ScenarioMetadata(
                category=ScenarioCategory.TOOL_SELECTION,
                complexity=ScenarioComplexity.LIGHT,
                expected_role=ExpectedRole.LIGHT,
                criteria=[],
            )

    def test_light_complexity_matches_light_role(self):
        meta = ScenarioMetadata(
            category=ScenarioCategory.TOOL_SELECTION,
            complexity=ScenarioComplexity.LIGHT,
            expected_role=ExpectedRole.LIGHT,
            criteria=[self._make_valid_criterion()],
        )
        assert meta.complexity == ScenarioComplexity.LIGHT
        assert meta.expected_role == ExpectedRole.LIGHT

    def test_assistant_complexity_matches_assistant_role(self):
        meta = ScenarioMetadata(
            category=ScenarioCategory.TECHNICAL_ANALYSIS,
            complexity=ScenarioComplexity.ASSISTANT,
            expected_role=ExpectedRole.ASSISTANT,
            criteria=[self._make_valid_criterion()],
        )
        assert meta.complexity == ScenarioComplexity.ASSISTANT
        assert meta.expected_role == ExpectedRole.ASSISTANT

    def test_primary_complexity_matches_primary_role(self):
        meta = ScenarioMetadata(
            category=ScenarioCategory.ROUTING_DELEGATION,
            complexity=ScenarioComplexity.PRIMARY,
            expected_role=ExpectedRole.PRIMARY,
            criteria=[self._make_valid_criterion()],
        )
        assert meta.complexity == ScenarioComplexity.PRIMARY
        assert meta.expected_role == ExpectedRole.PRIMARY

    def test_light_complexity_rejects_assistant_role(self):
        with pytest.raises(ValidationError, match="expected_role.*does not match.*complexity"):
            ScenarioMetadata(
                category=ScenarioCategory.TOOL_SELECTION,
                complexity=ScenarioComplexity.LIGHT,
                expected_role=ExpectedRole.ASSISTANT,
                criteria=[self._make_valid_criterion()],
            )

    def test_primary_complexity_rejects_light_role(self):
        with pytest.raises(ValidationError, match="expected_role.*does not match.*complexity"):
            ScenarioMetadata(
                category=ScenarioCategory.ROUTING_DELEGATION,
                complexity=ScenarioComplexity.PRIMARY,
                expected_role=ExpectedRole.LIGHT,
                criteria=[self._make_valid_criterion()],
            )

    def test_required_tools_default_to_empty(self):
        meta = ScenarioMetadata(
            category=ScenarioCategory.TOOL_SELECTION,
            complexity=ScenarioComplexity.LIGHT,
            expected_role=ExpectedRole.LIGHT,
            criteria=[self._make_valid_criterion()],
        )
        assert meta.required_tools == []

    def test_required_tools_can_be_specified(self):
        meta = ScenarioMetadata(
            category=ScenarioCategory.TOOL_SELECTION,
            complexity=ScenarioComplexity.LIGHT,
            expected_role=ExpectedRole.LIGHT,
            required_tools=["logs.search", "packet.inspect"],
            criteria=[self._make_valid_criterion()],
        )
        assert meta.required_tools == ["logs.search", "packet.inspect"]

    def test_metadata_round_trip_serialization(self):
        import json

        meta = ScenarioMetadata(
            category=ScenarioCategory.SECURITY_POLICY,
            complexity=ScenarioComplexity.ASSISTANT,
            expected_role=ExpectedRole.ASSISTANT,
            required_tools=["policy.check"],
            criteria=[
                GraderCriterion(kind="keyword_present", value="REFUSED"),
                GraderCriterion(kind="keyword_absent", value="secret", must_pass=False),
            ],
        )
        data = json.loads(meta.model_dump_json())
        restored = ScenarioMetadata.model_validate(data)
        assert restored == meta
