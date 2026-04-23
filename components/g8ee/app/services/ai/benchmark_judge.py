# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""
Deterministic Benchmark Judge for Industry-Standard Agent Evaluation.

Grades agent tool call payloads against expected patterns using strict
boolean pass/fail criteria. No LLM is involved -- grading is purely
deterministic regex matching on the actual TOOL_CALL arguments.

This complements the subjective EvalJudge (1-5 rubric) by providing a
hard, reproducible percentage metric:

    pass_rate = passed_scenarios / total_scenarios

Design principles (aligned with OSWorld / SWE-bench methodology):
  - Grade the syntactic payload, not the text reasoning.
  - Binary pass/fail per scenario. No partial credit.
  - Regex matching against expected command flags and arguments.

Note on Tribunal: Sage never proposes commands; the Tribunal produces
the final command string from Sage's natural-language request. The
benchmark grades the Tribunal's final command directly against the
scenario's expected payload. There is no "delta vs. Sage's proposal"
metric because Sage has no proposal to compare against.
"""

import logging
import re

from pydantic import BaseModel, Field, field_validator

logger = logging.getLogger(__name__)


class PayloadMatcher(BaseModel):
    """A single expected pattern to match against a tool call argument."""
    field: str = Field(description="Tool call argument field to match (e.g. 'command')")
    pattern: str = Field(description="Regex pattern the field value must match")
    description: str = Field(default="", description="Human-readable description of what this pattern checks")

    @field_validator("pattern", mode="before")
    @classmethod
    def _pattern_must_compile(cls, v: str) -> str:
        try:
            re.compile(v)
        except re.error as exc:
            raise ValueError(f"Invalid regex pattern: {exc}") from exc
        return v


class BenchmarkGrade(BaseModel):
    """Result of a deterministic benchmark evaluation.

    The Tribunal's final command (`tribunal_final_command`) and pipeline
    outcome (`tribunal_outcome`) are recorded for analysis when a Tribunal
    run produced the command. There is no pre-Tribunal delta because Sage
    does not propose commands.
    """
    passed: bool = Field(description="True if ALL payload matchers matched")
    tool_called: bool = Field(description="True if the expected tool was called at all")
    matchers_total: int = Field(ge=0, description="Total number of payload matchers")
    matchers_passed: int = Field(ge=0, description="Number of matchers that passed")
    failures: list[str] = Field(default_factory=list, description="List of matcher failure descriptions")

    tribunal_final_command: str | None = Field(default=None, description="Final command produced by the Tribunal")
    tribunal_outcome: str | None = Field(default=None, description="Tribunal pipeline outcome")


class ToolCallCapture(BaseModel):
    """Captured tool call from an agent interaction for benchmark grading."""
    tool_name: str = Field(description="Name of the tool that was called")
    args: dict[str, object] = Field(default_factory=dict, description="Arguments passed to the tool")


class TribunalCapture(BaseModel):
    """Captured Tribunal pipeline result attached to a benchmark grade.

    Sage never proposes commands; this records the Tribunal's output so the
    benchmark can surface `final_command` and the pipeline `outcome` in the
    grade for downstream analysis. There is no `original_command` field.
    """
    final_command: str = Field(description="Final command produced by the Tribunal")
    outcome: str = Field(description="Tribunal pipeline outcome (VERIFIED, CONSENSUS, VERIFICATION_FAILED)")
    vote_score: float | None = Field(default=None, ge=0.0, le=1.0)
    auditor_passed: bool | None = Field(default=None)
    auditor_revision: str | None = Field(default=None)


class BenchmarkScenario(BaseModel):
    """A single benchmark scenario definition from the gold set."""
    id: str
    description: str = ""
    user_query: str
    agent_mode: str
    expected_tool: str = Field(description="The tool name expected to be called")
    expected_payload: list[PayloadMatcher] = Field(
        description="Regex matchers for the tool call arguments",
    )
    forbidden_tools: list[str] = Field(default_factory=list)
    category: str = Field(default="general", description="Scenario category for reporting")


def _match_payload(
    tool_args: dict[str, object],
    matchers: list[PayloadMatcher],
) -> tuple[int, list[str]]:
    """Run all matchers against tool call args. Returns (passed_count, failure_descriptions)."""
    passed = 0
    failures: list[str] = []

    for matcher in matchers:
        value = tool_args.get(matcher.field)
        if value is None:
            failures.append(
                f"Field '{matcher.field}' not present in tool args"
                + (f" [{matcher.description}]" if matcher.description else "")
            )
            continue

        str_value = str(value)
        if re.search(matcher.pattern, str_value):
            passed += 1
        else:
            failures.append(
                f"Field '{matcher.field}' value '{str_value[:200]}' "
                f"did not match pattern '{matcher.pattern}'"
                + (f" [{matcher.description}]" if matcher.description else "")
            )

    return passed, failures


class BenchmarkJudge:
    """Deterministic judge that grades agent tool call payloads against expected patterns.

    Unlike EvalJudge, this class involves no LLM calls. Grading is pure regex
    matching on the captured tool call arguments, producing a strict boolean
    pass/fail per scenario.
    """

    def grade_tool_call(
        self,
        scenario: BenchmarkScenario,
        tool_calls: list[ToolCallCapture],
        tribunal: TribunalCapture | None = None,
    ) -> BenchmarkGrade:
        """Grade a single scenario against captured tool calls.

        A scenario passes only if:
        1. The expected tool was called.
        2. ALL payload matchers match against the tool call arguments.
        3. No forbidden tools were called.

        For ``multi_step_execution`` scenarios, matchers are evaluated across
        the **union** of all captured tool calls for the expected tool.  Each
        matcher passes if it matches on **at least one** call.  This correctly
        models multi-step resolution where the agent issues separate commands
        (e.g. identify process, kill it, restart service).
        """
        forbidden_called = [
            tc.tool_name for tc in tool_calls
            if tc.tool_name in scenario.forbidden_tools
        ]
        if forbidden_called:
            return BenchmarkGrade(
                passed=False,
                tool_called=False,
                matchers_total=len(scenario.expected_payload),
                matchers_passed=0,
                failures=[f"Forbidden tool(s) called: {', '.join(forbidden_called)}"],
            )

        matching_calls = [tc for tc in tool_calls if tc.tool_name == scenario.expected_tool]
        if not matching_calls:
            return BenchmarkGrade(
                passed=False,
                tool_called=False,
                matchers_total=len(scenario.expected_payload),
                matchers_passed=0,
                failures=[f"Expected tool '{scenario.expected_tool}' was not called"],
            )

        if scenario.category == "multi_step_execution":
            best_grade = self._grade_multi_step(scenario, matching_calls)
        else:
            best_grade = self._grade_single_call(scenario, matching_calls)

        if tribunal:
            best_grade.tribunal_final_command = tribunal.final_command
            best_grade.tribunal_outcome = tribunal.outcome

        return best_grade

    @staticmethod
    def _grade_single_call(
        scenario: BenchmarkScenario,
        matching_calls: list[ToolCallCapture],
    ) -> BenchmarkGrade:
        """Grade by finding the single best-matching tool call."""
        best_grade: BenchmarkGrade | None = None
        for tc in matching_calls:
            matchers_passed, failures = _match_payload(tc.args, scenario.expected_payload)
            grade = BenchmarkGrade(
                passed=(matchers_passed == len(scenario.expected_payload) and len(failures) == 0),
                tool_called=True,
                matchers_total=len(scenario.expected_payload),
                matchers_passed=matchers_passed,
                failures=failures,
            )
            if best_grade is None or grade.matchers_passed > best_grade.matchers_passed:
                best_grade = grade
        assert best_grade is not None
        return best_grade

    @staticmethod
    def _grade_multi_step(
        scenario: BenchmarkScenario,
        matching_calls: list[ToolCallCapture],
    ) -> BenchmarkGrade:
        """Grade by evaluating each matcher across *all* captured calls (union).

        A matcher passes if it matches on at least one tool call.  This handles
        scenarios where the agent issues separate commands for each step.
        A single compound command that satisfies all matchers also passes.
        """
        matchers_passed = 0
        failures: list[str] = []

        for matcher in scenario.expected_payload:
            matched = False
            for tc in matching_calls:
                value = tc.args.get(matcher.field)
                if value is not None and re.search(matcher.pattern, str(value)):
                    matched = True
                    break
            if matched:
                matchers_passed += 1
            else:
                best_value = None
                for tc in matching_calls:
                    v = tc.args.get(matcher.field)
                    if v is not None:
                        best_value = str(v)
                        break
                if best_value is None:
                    failures.append(
                        f"Field '{matcher.field}' not present in any tool call"
                        + (f" [{matcher.description}]" if matcher.description else "")
                    )
                else:
                    failures.append(
                        f"Field '{matcher.field}' did not match pattern '{matcher.pattern}' "
                        f"across {len(matching_calls)} tool call(s)"
                        + (f" [{matcher.description}]" if matcher.description else "")
                    )

        return BenchmarkGrade(
            passed=(matchers_passed == len(scenario.expected_payload) and len(failures) == 0),
            tool_called=True,
            matchers_total=len(scenario.expected_payload),
            matchers_passed=matchers_passed,
            failures=failures,
        )

    def grade_refusal(
        self,
        scenario: BenchmarkScenario,
        tool_calls: list[ToolCallCapture],
        response_text: str,
    ) -> BenchmarkGrade:
        """Grade a refusal scenario where the agent should NOT call forbidden tools.

        Passes if:
        1. No forbidden tools were called.
        2. All payload matchers match against the response text (treated as a
           single field named 'response').
        """
        forbidden_called = [
            tc.tool_name for tc in tool_calls
            if tc.tool_name in scenario.forbidden_tools
        ]
        if forbidden_called:
            return BenchmarkGrade(
                passed=False,
                tool_called=True,
                matchers_total=len(scenario.expected_payload),
                matchers_passed=0,
                failures=[f"Forbidden tool(s) called: {', '.join(forbidden_called)}"],
            )

        response_args: dict[str, object] = {"response": response_text}
        matchers_passed, failures = _match_payload(response_args, scenario.expected_payload)

        return BenchmarkGrade(
            passed=(matchers_passed == len(scenario.expected_payload) and len(failures) == 0),
            tool_called=len(tool_calls) > 0,
            matchers_total=len(scenario.expected_payload),
            matchers_passed=matchers_passed,
            failures=failures,
        )


def compute_benchmark_percentage(grades: list[BenchmarkGrade]) -> float:
    """Compute aggregate pass rate from a list of benchmark grades."""
    if not grades:
        return 0.0
    return sum(1 for g in grades if g.passed) / len(grades)


def compute_tribunal_stats(grades: list[BenchmarkGrade]) -> dict[str, object]:
    """Compute aggregate Tribunal pass-rate statistics.

    Sage does not propose commands, so there is no pre/post delta. This reports
    how the Tribunal's command fared against the scenario matchers:

      - total_with_tribunal: scenarios where a Tribunal run was captured
      - tribunal_passed_count: scenarios where the Tribunal's command passed all matchers
      - tribunal_pass_rate: tribunal_passed_count / total_with_tribunal
    """
    tribunal_grades = [g for g in grades if g.tribunal_outcome is not None]
    if not tribunal_grades:
        return {
            "total_with_tribunal": 0,
            "tribunal_passed_count": 0,
            "tribunal_pass_rate": 0.0,
        }

    passed_count = sum(1 for g in tribunal_grades if g.passed)
    return {
        "total_with_tribunal": len(tribunal_grades),
        "tribunal_passed_count": passed_count,
        "tribunal_pass_rate": passed_count / len(tribunal_grades),
    }
