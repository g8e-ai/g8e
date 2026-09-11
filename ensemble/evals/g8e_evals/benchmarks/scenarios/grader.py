# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Deterministic grader for scenario-based REAL_SYSTEM suites.

The grader extracts ``ScenarioMetadata`` from the task's
``benchmark_specific`` field and checks each ``GraderCriterion``
against the response answer. All ``must_pass`` criteria must pass for
the scenario to pass. Criteria with ``must_pass=False`` are
informational and recorded in the score details but do not affect the
pass/fail result.

The grader supports the following criterion kinds:

- ``keyword_present``: The answer contains the keyword (case-insensitive).
- ``keyword_absent``: The answer does not contain the keyword.
- ``regex_match``: The answer matches the regex pattern.
- ``json_field_equals``: The answer contains JSON with a field matching
  the expected value. ``value`` is a JSON object with ``field`` and
  ``expected`` keys.
- ``json_field_present``: The answer contains JSON with the named field.
- ``min_length``: The answer is at least ``value`` characters.
- ``max_length``: The answer is at most ``value`` characters.
- ``starts_with``: The answer starts with the given prefix.
- ``ends_with``: The answer ends with the given suffix.
"""

from __future__ import annotations

import json
import re
from typing import Any

from pydantic import TypeAdapter

from g8e_evals.benchmarks.scenarios.metadata import (
    GraderCriterion,
    ScenarioMetadata,
)
from g8e_evals.harness import Response, Score, Task
from g8e_evals.models import InstructionResult, ScoreDetails


class ScenarioGrader:
    """Deterministic grader for scenario-based REAL_SYSTEM suites.

    Each instance is bound to a suite ID and grader identity (ID and
    version) so the runner can use ``grader.grader_id`` and
    ``grader.grader_version`` for metric observations and task
    definitions.
    """

    def __init__(self, *, grader_id: str, grader_version: str = "1.0.0") -> None:
        self.grader_id = grader_id
        self.grader_version = grader_version

    def grade(self, task: Task, response: Response) -> Score:
        """Grade a completed scenario task against its criteria."""
        answer = response.answer or ""
        scenario_meta = self._extract_scenario_metadata(task)
        if scenario_meta is None:
            return Score(
                task_id=task.id,
                passed=False,
                details=ScoreDetails(
                    error="Missing ScenarioMetadata in task benchmark_specific",
                ),
            )

        results: list[InstructionResult] = []
        for index, criterion in enumerate(scenario_meta.criteria):
            passed = self._check_criterion(criterion, answer)
            results.append(
                InstructionResult(
                    instruction=f"criterion-{index}:{criterion.kind}",
                    passed=passed,
                    kwargs={"value": criterion.value, "must_pass": criterion.must_pass},
                )
            )

        must_pass_results = [r for r, c in zip(results, scenario_meta.criteria, strict=True) if c.must_pass]
        all_passed = all(r.passed for r in must_pass_results) if must_pass_results else False

        return Score(
            task_id=task.id,
            passed=all_passed,
            details=ScoreDetails(instructions=results),
        )

    def _extract_scenario_metadata(self, task: Task) -> ScenarioMetadata | None:
        """Extract typed ScenarioMetadata from the task's benchmark_specific field."""
        raw = task.metadata.benchmark_specific
        if not raw:
            return None
        try:
            return TypeAdapter(ScenarioMetadata).validate_python(raw)
        except Exception:
            return None

    def _check_criterion(self, criterion: GraderCriterion, answer: str) -> bool:
        kind = criterion.kind
        value = criterion.value

        if kind == "keyword_present":
            return value.lower() in answer.lower()

        if kind == "keyword_absent":
            return value.lower() not in answer.lower()

        if kind == "regex_match":
            try:
                return re.search(value, answer, flags=re.IGNORECASE) is not None
            except re.error:
                return False

        if kind == "json_field_equals":
            spec = json.loads(value)
            field = spec["field"]
            expected = spec["expected"]
            return self._check_json_field(answer, field, expected)

        if kind == "json_field_present":
            return self._check_json_field_present(answer, value)

        if kind == "min_length":
            try:
                return len(answer) >= int(value)
            except ValueError:
                return False

        if kind == "max_length":
            try:
                return len(answer) <= int(value)
            except ValueError:
                return False

        if kind == "starts_with":
            return answer.strip().lower().startswith(value.lower())

        if kind == "ends_with":
            return answer.strip().lower().endswith(value.lower())

        return False

    def _extract_json(self, answer: str) -> dict[str, Any] | None:
        """Extract a JSON object from the answer, handling markdown fences."""
        value = (
            answer.strip()
            .removeprefix("```json")
            .removeprefix("```Json")
            .removeprefix("```JSON")
            .removeprefix("```")
            .removesuffix("```")
            .strip()
        )
        try:
            parsed = json.loads(value)
            if isinstance(parsed, dict):
                return parsed
        except ValueError:
            pass
        # Try to find a JSON block in the answer
        match = re.search(r"\{[^{}]*\}", answer, flags=re.DOTALL)
        if match:
            try:
                parsed = json.loads(match.group())
                if isinstance(parsed, dict):
                    return parsed
            except ValueError:
                pass
        return None

    def _check_json_field(self, answer: str, field: str, expected: Any) -> bool:
        parsed = self._extract_json(answer)
        if parsed is None:
            return False
        return parsed.get(field) == expected

    def _check_json_field_present(self, answer: str, field: str) -> bool:
        parsed = self._extract_json(answer)
        if parsed is None:
            return False
        return field in parsed


__all__ = ["ScenarioGrader"]
