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
Industry-Standard Agent Benchmark Test Suite.

Grades the AI agent's tool call payloads against strict boolean criteria
using the BenchmarkJudge. No partial credit -- scenarios are binary pass/fail.

This test exercises the full ChatPipelineService.run_chat path and intercepts
the tool call arguments emitted during the agent's ReAct loop to grade
the actual command payload, not the text reasoning.

The Tribunal delta is tracked: for each run_commands_with_operator call, the
test records the pre-Tribunal and post-Tribunal command strings and measures
whether the Tribunal improved the syntactic accuracy.

Aggregate metrics:
  - Pass rate: passed_scenarios / total_scenarios (the "real percentage")
  - Tribunal improvement rate: how often the Tribunal corrected a Primary Agent error
"""

import os
import logging
import pytest
from typing import Any
from datetime import datetime, timezone
from unittest.mock import patch

from app.constants import AgentMode, EventType, OperatorStatus
from app.services.ai.chat_task_manager import ChatTaskManager
from app.services.ai.benchmark_judge import (
    BenchmarkJudge,
    BenchmarkScenario,
    PayloadMatcher,
    ToolCallCapture,
    TribunalCapture,
)
from app.models.settings import G8eeUserSettings
from app.models.http_context import G8eHttpContext
from app.models.investigations import InvestigationCreateRequest
from tests.integration.evals.shared import (
    BenchmarkTestResult,
    load_and_validate_benchmark_set,
    seed_operator_if_bound,
)

logger = logging.getLogger(__name__)

BENCHMARK_PATH = os.path.join(os.path.dirname(__file__), "benchmark_gold_set.json")


def load_benchmark_set() -> list[dict[str, Any]]:
    return load_and_validate_benchmark_set(BENCHMARK_PATH)


pytestmark = [
    pytest.mark.integration,
    pytest.mark.ai_integration,
    pytest.mark.agent_benchmark,
    pytest.mark.slow,
    pytest.mark.timeout(180),
]


def _build_scenario(raw: dict[str, Any]) -> BenchmarkScenario:
    """Convert a raw gold set dict into a typed BenchmarkScenario."""
    matchers = [
        PayloadMatcher(
            field=m["field"],
            pattern=m["pattern"],
            description=m.get("description", ""),
        )
        for m in raw["expected_payload"]
    ]
    return BenchmarkScenario(
        id=raw["id"],
        description=raw.get("description", ""),
        user_query=raw["user_query"],
        agent_mode=raw["agent_mode"],
        expected_tool=raw["expected_tool"],
        expected_payload=matchers,
        forbidden_tools=raw.get("forbidden_tools", []),
        category=raw.get("category", "general"),
    )


@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.parametrize("scenario_data", load_benchmark_set(), ids=lambda s: s["id"])
async def test_agent_benchmark(
    scenario_data: dict[str, Any],
    all_services,
    cache_aside_service,
    test_settings,
    test_user_settings,
    cleanup,
    unique_investigation_id,
    unique_case_id,
    unique_web_session_id,
    unique_user_id,
    unique_operator_id,
    unique_session_id,
    benchmark_results_collector,
):
    """
    Benchmark a single scenario against the full agent pipeline.

    1. Create investigation and operator context
    2. Patch orchestrate_tool_execution to capture tool call args + Tribunal data
    3. Run the full ChatPipelineService.run_chat
    4. Grade captured tool calls with BenchmarkJudge (deterministic, no LLM)
    5. Record results for aggregate reporting
    """
    start_time = datetime.now(timezone.utc)
    scenario = _build_scenario(scenario_data)
    result_data = BenchmarkTestResult(scenario_id=scenario.id, category=scenario.category)

    investigation_service = all_services['investigation_service']
    investigation_data_service = all_services['investigation_data_service']
    operator_data_service = all_services['operator_data_service']
    chat_pipeline = all_services['chat_pipeline']

    captured_tool_calls: list[ToolCallCapture] = []
    captured_tribunal: TribunalCapture | None = None
    response_text = ""

    try:
        llm_settings = test_user_settings.llm
        agent_mode = AgentMode.OPERATOR_BOUND if scenario.agent_mode == "OPERATOR_BOUND" else AgentMode.OPERATOR_NOT_BOUND

        bound_operators = await seed_operator_if_bound(
            agent_mode=agent_mode,
            operator_id=unique_operator_id,
            operator_session_id=unique_session_id,
            operator_data_service=operator_data_service,
            cleanup=cleanup,
            log_prefix="[BENCH]",
        )

        investigation_request = InvestigationCreateRequest(
            case_id=unique_case_id,
            case_title=f"Benchmark: {scenario.id}",
            case_description=scenario.description or "Benchmark evaluation",
            user_id=unique_user_id,
            web_session_id=unique_web_session_id,
        )
        created_investigation = await investigation_data_service.create_investigation(investigation_request)
        logger.info("[BENCH] Created investigation %s for %s", created_investigation.id, scenario.id)
        cleanup.track_investigation(created_investigation.id)

        g8e_context = G8eHttpContext(
            web_session_id=unique_web_session_id,
            user_id=unique_user_id,
            case_id=unique_case_id,
            investigation_id=created_investigation.id,
            organization_id="org-bench-001",
            source_component="g8ee",
            bound_operators=bound_operators,
        )

        from app.services.ai.agent_tool_loop import orchestrate_tool_execution as _real_orchestrate

        async def _capturing_orchestrate(tool_call, tool_executor, investigation, g8e_context, g8ed_event_service, request_settings):
            nonlocal captured_tribunal
            tool_name = tool_call.name or ""
            raw_args = dict(tool_call.args) if tool_call.args else {}

            result = await _real_orchestrate(
                tool_call=tool_call,
                tool_executor=tool_executor,
                investigation=investigation,
                g8e_context=g8e_context,
                g8ed_event_service=g8ed_event_service,
                request_settings=request_settings,
            )

            actual_args = dict(raw_args)
            tr = result.tribunal_result
            if tool_name == "run_commands_with_operator" and tr is not None:
                actual_args["command"] = tr.final_command

            captured_tool_calls.append(ToolCallCapture(
                tool_name=tool_name,
                args=actual_args,
            ))

            if tool_name == "run_commands_with_operator" and tr is not None:
                captured_tribunal = TribunalCapture(
                    original_command=tr.original_command,
                    final_command=tr.final_command,
                    outcome=tr.outcome,
                    vote_score=tr.vote_score,
                    verifier_passed=tr.verifier_passed,
                    verifier_revision=tr.verifier_revision,
                )

            return result

        user_settings = test_user_settings
        task_manager = ChatTaskManager()

        logger.info("[BENCH] Running scenario %s", scenario.id)

        with patch(
            "app.services.ai.agent_tool_loop.orchestrate_tool_execution",
            side_effect=_capturing_orchestrate,
        ):
            await chat_pipeline.run_chat(
                message=scenario.user_query,
                g8e_context=g8e_context,
                attachments=[],
                sentinel_mode=True,
                llm_primary_provider=None,
                llm_assistant_provider=None,
                llm_primary_model=llm_settings.primary_model,
                llm_assistant_model=llm_settings.assistant_model,
                _task_manager=task_manager,
                user_settings=user_settings,
                _track_task=False,
            )

        conversation_history = await investigation_service.get_chat_messages(
            investigation_id=created_investigation.id
        )
        for msg in reversed(conversation_history):
            if msg.sender == EventType.EVENT_SOURCE_AI_PRIMARY:
                response_text = msg.content
                break

        logger.info(
            "[BENCH] Captured %d tool call(s) for %s",
            len(captured_tool_calls), scenario.id,
        )
        for tc in captured_tool_calls:
            logger.info("[BENCH]   tool=%s args_keys=%s", tc.tool_name, list(tc.args.keys()))
            if "command" in tc.args:
                logger.info("[BENCH]   command=%s", str(tc.args["command"])[:200])

        judge = BenchmarkJudge()
        is_refusal = scenario.category == "security_refusal"

        if is_refusal:
            grade = judge.grade_refusal(
                scenario=scenario,
                tool_calls=captured_tool_calls,
                response_text=response_text,
            )
        else:
            grade = judge.grade_tool_call(
                scenario=scenario,
                tool_calls=captured_tool_calls,
                tribunal=captured_tribunal,
            )

        result_data.passed = grade.passed
        result_data.tool_called = grade.tool_called
        result_data.matchers_total = grade.matchers_total
        result_data.matchers_passed = grade.matchers_passed
        result_data.failures = grade.failures

        if grade.tribunal_outcome is not None:
            result_data.tribunal_original_command = grade.tribunal_original_command
            result_data.tribunal_final_command = grade.tribunal_final_command
            result_data.tribunal_outcome = grade.tribunal_outcome
            result_data.tribunal_improved = grade.tribunal_improved
            result_data.tribunal_pre_score = grade.tribunal_pre_score

        end_time = datetime.now(timezone.utc)
        result_data.execution_time_ms = (end_time - start_time).total_seconds() * 1000

        benchmark_results_collector.add_result(result_data.to_dict())

        status = "PASS" if grade.passed else "FAIL"
        logger.info("=" * 60)
        logger.info("[BENCH_RESULT] %s %s", status, scenario.id)
        logger.info("[BENCH_RESULT] Matchers: %d/%d", grade.matchers_passed, grade.matchers_total)
        logger.info("[BENCH_RESULT] Execution Time: %.1fms", result_data.execution_time_ms)
        if grade.failures:
            for f in grade.failures:
                logger.info("[BENCH_RESULT] FAILURE: %s", f[:200])
        if grade.tribunal_improved:
            logger.info("[BENCH_RESULT] Tribunal: %s -> %s",
                grade.tribunal_original_command, grade.tribunal_final_command)
        logger.info("=" * 60)

        assert grade.passed, (
            f"Benchmark failed for {scenario.id}: "
            f"{grade.matchers_passed}/{grade.matchers_total} matchers passed. "
            f"Failures: {'; '.join(grade.failures)}"
        )

    except Exception as e:
        result_data.error = str(e)
        result_data.passed = False
        logger.exception("[BENCH] Fatal error in scenario %s: %s", scenario.id, e)
        raise
