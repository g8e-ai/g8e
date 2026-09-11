# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for the ScenarioGrader.

Verifies that the grader checks all nine criterion kinds correctly,
handles missing metadata, distinguishes must_pass from informational
criteria, and returns a Score with per-criterion InstructionResult
records.

No external dependencies (no files, network, or DB).
"""

from __future__ import annotations

import pytest

from g8e_evals.benchmarks.scenarios.grader import ScenarioGrader
from g8e_evals.benchmarks.scenarios.metadata import (
    ExpectedRole,
    GraderCriterion,
    ScenarioCategory,
    ScenarioComplexity,
    ScenarioMetadata,
)
from g8e_evals.harness import Response, Task
from g8e_evals.models import TaskMetadata


pytestmark = pytest.mark.unit


def _make_task(criteria: list[GraderCriterion], *, suite_id: str = "tool_selection") -> Task:
    meta = ScenarioMetadata(
        category=ScenarioCategory.TOOL_SELECTION,
        complexity=ScenarioComplexity.LIGHT,
        expected_role=ExpectedRole.LIGHT,
        criteria=criteria,
    )
    return Task(
        id="TS-001",
        prompt="Select the correct tool.",
        metadata=TaskMetadata(
            benchmark=suite_id,
            category="tool_selection",
            benchmark_specific=meta.model_dump(mode="json"),
        ),
    )


class TestScenarioGraderIdentity:
    def test_grader_carries_grader_id(self):
        grader = ScenarioGrader(grader_id="tool_selection")
        assert grader.grader_id == "tool_selection"

    def test_grader_default_version(self):
        grader = ScenarioGrader(grader_id="tool_selection")
        assert grader.grader_version == "1.0.0"

    def test_grader_custom_version(self):
        grader = ScenarioGrader(grader_id="tool_selection", grader_version="2.0.0")
        assert grader.grader_version == "2.0.0"


class TestKeywordPresent:
    def test_keyword_present_passes_when_found(self):
        task = _make_task([GraderCriterion(kind="keyword_present", value="error")])
        grader = ScenarioGrader(grader_id="tool_selection")
        score = grader.grade(task, Response(model="test-model", answer="The error was found."))
        assert score.passed is True

    def test_keyword_present_fails_when_absent(self):
        task = _make_task([GraderCriterion(kind="keyword_present", value="missing")])
        grader = ScenarioGrader(grader_id="tool_selection")
        score = grader.grade(task, Response(model="test-model", answer="Everything looks fine."))
        assert score.passed is False

    def test_keyword_present_is_case_insensitive(self):
        task = _make_task([GraderCriterion(kind="keyword_present", value="ERROR")])
        grader = ScenarioGrader(grader_id="tool_selection")
        score = grader.grade(task, Response(model="test-model", answer="the error was found"))
        assert score.passed is True


class TestKeywordAbsent:
    def test_keyword_absent_passes_when_not_found(self):
        task = _make_task([GraderCriterion(kind="keyword_absent", value="secret")])
        grader = ScenarioGrader(grader_id="tool_selection")
        score = grader.grade(task, Response(model="test-model", answer="Everything is public."))
        assert score.passed is True

    def test_keyword_absent_fails_when_present(self):
        task = _make_task([GraderCriterion(kind="keyword_absent", value="secret")])
        grader = ScenarioGrader(grader_id="tool_selection")
        score = grader.grade(task, Response(model="test-model", answer="The secret key was leaked."))
        assert score.passed is False


class TestRegexMatch:
    def test_regex_match_passes_when_matched(self):
        task = _make_task([GraderCriterion(kind="regex_match", value=r"\d{3}-\d{4}")])
        grader = ScenarioGrader(grader_id="tool_selection")
        score = grader.grade(task, Response(model="test-model", answer="Phone: 555-1234"))
        assert score.passed is True

    def test_regex_match_fails_when_not_matched(self):
        task = _make_task([GraderCriterion(kind="regex_match", value=r"\d{3}-\d{4}")])
        grader = ScenarioGrader(grader_id="tool_selection")
        score = grader.grade(task, Response(model="test-model", answer="No phone here."))
        assert score.passed is False

    def test_regex_match_fails_on_invalid_pattern(self):
        task = _make_task([GraderCriterion(kind="regex_match", value=r"[invalid")])
        grader = ScenarioGrader(grader_id="tool_selection")
        score = grader.grade(task, Response(model="test-model", answer="anything"))
        assert score.passed is False


class TestJsonFieldEquals:
    def test_json_field_equals_passes_with_valid_json(self):
        spec = '{"field": "status", "expected": "ok"}'
        task = _make_task([GraderCriterion(kind="json_field_equals", value=spec)])
        grader = ScenarioGrader(grader_id="tool_selection")
        score = grader.grade(task, Response(model="test-model", answer='{"status": "ok", "code": 200}'))
        assert score.passed is True

    def test_json_field_equals_fails_with_wrong_value(self):
        spec = '{"field": "status", "expected": "ok"}'
        task = _make_task([GraderCriterion(kind="json_field_equals", value=spec)])
        grader = ScenarioGrader(grader_id="tool_selection")
        score = grader.grade(task, Response(model="test-model", answer='{"status": "error"}'))
        assert score.passed is False

    def test_json_field_equals_passes_with_markdown_fence(self):
        spec = '{"field": "result", "expected": "pass"}'
        task = _make_task([GraderCriterion(kind="json_field_equals", value=spec)])
        grader = ScenarioGrader(grader_id="tool_selection")
        score = grader.grade(task, Response(model="test-model", answer='```json\n{"result": "pass"}\n```'))
        assert score.passed is True

    def test_json_field_equals_fails_on_non_json(self):
        spec = '{"field": "status", "expected": "ok"}'
        task = _make_task([GraderCriterion(kind="json_field_equals", value=spec)])
        grader = ScenarioGrader(grader_id="tool_selection")
        score = grader.grade(task, Response(model="test-model", answer="not json at all"))
        assert score.passed is False


class TestJsonFieldPresent:
    def test_json_field_present_passes_when_field_exists(self):
        task = _make_task([GraderCriterion(kind="json_field_present", value="diagnosis")])
        grader = ScenarioGrader(grader_id="tool_selection")
        score = grader.grade(task, Response(model="test-model", answer='{"diagnosis": "network failure"}'))
        assert score.passed is True

    def test_json_field_present_fails_when_field_missing(self):
        task = _make_task([GraderCriterion(kind="json_field_present", value="diagnosis")])
        grader = ScenarioGrader(grader_id="tool_selection")
        score = grader.grade(task, Response(model="test-model", answer='{"cause": "unknown"}'))
        assert score.passed is False


class TestMinLength:
    def test_min_length_passes_when_long_enough(self):
        task = _make_task([GraderCriterion(kind="min_length", value="10")])
        grader = ScenarioGrader(grader_id="tool_selection")
        score = grader.grade(task, Response(model="test-model", answer="This is a long enough answer."))
        assert score.passed is True

    def test_min_length_fails_when_too_short(self):
        task = _make_task([GraderCriterion(kind="min_length", value="100")])
        grader = ScenarioGrader(grader_id="tool_selection")
        score = grader.grade(task, Response(model="test-model", answer="short"))
        assert score.passed is False


class TestMaxLength:
    def test_max_length_passes_when_short_enough(self):
        task = _make_task([GraderCriterion(kind="max_length", value="100")])
        grader = ScenarioGrader(grader_id="tool_selection")
        score = grader.grade(task, Response(model="test-model", answer="short answer"))
        assert score.passed is True

    def test_max_length_fails_when_too_long(self):
        task = _make_task([GraderCriterion(kind="max_length", value="5")])
        grader = ScenarioGrader(grader_id="tool_selection")
        score = grader.grade(task, Response(model="test-model", answer="this is way too long"))
        assert score.passed is False


class TestStartsWith:
    def test_starts_with_passes_when_correct_prefix(self):
        task = _make_task([GraderCriterion(kind="starts_with", value="YES")])
        grader = ScenarioGrader(grader_id="tool_selection")
        score = grader.grade(task, Response(model="test-model", answer="YES, the task is complete."))
        assert score.passed is True

    def test_starts_with_fails_when_wrong_prefix(self):
        task = _make_task([GraderCriterion(kind="starts_with", value="YES")])
        grader = ScenarioGrader(grader_id="tool_selection")
        score = grader.grade(task, Response(model="test-model", answer="NO, the task failed."))
        assert score.passed is False


class TestEndsWith:
    def test_ends_with_passes_when_correct_suffix(self):
        task = _make_task([GraderCriterion(kind="ends_with", value="done")])
        grader = ScenarioGrader(grader_id="tool_selection")
        score = grader.grade(task, Response(model="test-model", answer="The task is done"))
        assert score.passed is True

    def test_ends_with_fails_when_wrong_suffix(self):
        task = _make_task([GraderCriterion(kind="ends_with", value="done")])
        grader = ScenarioGrader(grader_id="tool_selection")
        score = grader.grade(task, Response(model="test-model", answer="The task is incomplete"))
        assert score.passed is False


class TestMustPassVsInformational:
    def test_informational_criterion_does_not_affect_pass(self):
        task = _make_task([
            GraderCriterion(kind="keyword_present", value="pass", must_pass=True),
            GraderCriterion(kind="keyword_present", value="bonus", must_pass=False),
        ])
        grader = ScenarioGrader(grader_id="tool_selection")
        score = grader.grade(task, Response(model="test-model", answer="This passes."))
        assert score.passed is True
        assert len(score.details.instructions) == 2

    def test_informational_criterion_failure_does_not_fail(self):
        task = _make_task([
            GraderCriterion(kind="keyword_present", value="pass", must_pass=True),
            GraderCriterion(kind="keyword_present", value="missing", must_pass=False),
        ])
        grader = ScenarioGrader(grader_id="tool_selection")
        score = grader.grade(task, Response(model="test-model", answer="This passes."))
        assert score.passed is True

    def test_all_must_pass_criteria_must_pass(self):
        task = _make_task([
            GraderCriterion(kind="keyword_present", value="alpha", must_pass=True),
            GraderCriterion(kind="keyword_present", value="beta", must_pass=True),
        ])
        grader = ScenarioGrader(grader_id="tool_selection")
        score = grader.grade(task, Response(model="test-model", answer="alpha only"))
        assert score.passed is False


class TestMissingMetadata:
    def test_grader_returns_fail_on_missing_metadata(self):
        task = Task(
            id="TS-001",
            prompt="Select the correct tool.",
            metadata=TaskMetadata(benchmark="tool_selection", category="tool_selection"),
        )
        grader = ScenarioGrader(grader_id="tool_selection")
        score = grader.grade(task, Response(model="test-model", answer="anything"))
        assert score.passed is False
        assert "Missing ScenarioMetadata" in (score.details.error or "")

    def test_grader_returns_fail_on_empty_answer(self):
        task = _make_task([GraderCriterion(kind="keyword_present", value="found")])
        grader = ScenarioGrader(grader_id="tool_selection")
        score = grader.grade(task, Response(model="test-model", answer=""))
        assert score.passed is False


class TestScoreDetails:
    def test_score_has_per_criterion_instruction_results(self):
        task = _make_task([
            GraderCriterion(kind="keyword_present", value="alpha"),
            GraderCriterion(kind="keyword_present", value="beta"),
        ])
        grader = ScenarioGrader(grader_id="tool_selection")
        score = grader.grade(task, Response(model="test-model", answer="alpha and beta"))
        assert score.task_id == "TS-001"
        assert score.details.instructions is not None
        assert len(score.details.instructions) == 2
        assert score.details.instructions[0].passed is True
        assert score.details.instructions[1].passed is True
