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

IMPORTANT: If test fails with 0 tool calls captured, run with:
    pytest -s --log-cli-level=INFO tests/evals/test_agent_benchmark.py

Check the [AGENT] "Loading model:" log line which includes tools=N. If tools=0,
operator tools are not being declared to the LLM - investigate get_generation_config
and tool_service.get_tools() to understand why.

Aggregate metrics:
  - Pass rate: passed_scenarios / total_scenarios (the "real percentage")
  - Tribunal improvement rate: how often the Tribunal corrected a Primary Agent error
"""

import logging
import pytest
from typing import Any
from datetime import datetime, timezone

from app.constants import AgentMode, EventType
from app.constants.paths import PATHS
from app.services.ai.chat_task_manager import ChatTaskManager
from app.services.ai.benchmark_judge import (
    BenchmarkJudge,
    BenchmarkScenario,
    PayloadMatcher,
    ToolCallCapture,
    TribunalCapture,
)
from app.models.events import SessionEvent
from app.models.g8ed_client import ChatToolCallPayload
from app.models.agents.tribunal import TribunalSessionCompletedPayload
from app.models.settings import G8eeUserSettings
from app.models.http_context import G8eHttpContext
from app.models.investigations import InvestigationCreateRequest
from app.services.operator.approval_service import PendingApproval
from tests.fakes.fake_event_service import FakeEventService
from tests.evals.shared import (
    BenchmarkTestResult,
    load_and_validate_benchmark_set,
    seed_operator_if_bound,
)
from app.utils.timestamp import now

logger = logging.getLogger(__name__)

def load_benchmark_set() -> list[dict[str, Any]]:
    return load_and_validate_benchmark_set(PATHS["g8ee"]["evals"]["benchmark_path"])


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


def _extract_tribunal_from_events(
    published: list[SessionEvent],
) -> TribunalCapture | None:
    """Extract Tribunal data from TRIBUNAL_SESSION_COMPLETED events.

    Tribunal delta tracking requires event interception since Tribunal
    results are not persisted in conversation history.
    """
    for event in published:
        if not isinstance(event, SessionEvent):
            continue

        if event.event_type == EventType.TRIBUNAL_SESSION_COMPLETED:
            payload = event.payload
            if isinstance(payload, TribunalSessionCompletedPayload):
                return TribunalCapture(
                    original_command=payload.original_command,
                    final_command=payload.final_command,
                    outcome=payload.outcome,
                    vote_score=payload.vote_score,
                    verifier_passed=None,
                    verifier_revision=None,
                )

    return None


def _extract_tool_calls_from_events(
    published: list[SessionEvent],
) -> list[ToolCallCapture]:
    """Extract tool calls from TOOL_CALL_STARTED events.

    Tool calls are not persisted in conversation history, so we must
    intercept events to capture them.
    """
    tool_calls: list[ToolCallCapture] = []

    for event in published:
        if not isinstance(event, SessionEvent):
            continue

        if event.event_type == EventType.LLM_CHAT_ITERATION_TOOL_CALL_STARTED:
            payload = event.payload
            if isinstance(payload, ChatToolCallPayload) and payload.tool_name:
                command = payload.display_detail or ""
                tool_calls.append(ToolCallCapture(
                    tool_name=payload.tool_name,
                    args={"command": command} if command else {},
                ))

    return tool_calls


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
    2. Inject FakeEventService to capture pipeline events
    3. Run the full ChatPipelineService.run_chat
    4. Extract tool calls and Tribunal data from captured events
    5. Grade with BenchmarkJudge (deterministic, no LLM)
    6. Record results for aggregate reporting

    Note: Both tool calls and Tribunal data use event interception since
    neither are persisted in conversation history. Run with -s --log-cli-level=INFO
    to see [AGENT] logs including tools=N which indicates if tools were declared to the LLM.
    """
    start_time = datetime.now(timezone.utc)
    scenario = _build_scenario(scenario_data)
    result_data = BenchmarkTestResult(scenario_id=scenario.id, category=scenario.category)

    logger.info("[BENCH] ===== Starting test for scenario %s =====", scenario.id)

    investigation_service = all_services['investigation_service']
    investigation_data_service = all_services['investigation_data_service']
    operator_data_service = all_services['operator_data_service']
    chat_pipeline = all_services['chat_pipeline']

    response_text = ""

    try:
        llm_settings = test_user_settings.llm
        agent_mode = AgentMode.OPERATOR_BOUND if scenario.agent_mode == "OPERATOR_BOUND" else AgentMode.OPERATOR_NOT_BOUND

        logger.info("[BENCH-SETTINGS] Tribunal configuration:")
        logger.info("[BENCH-SETTINGS]   llm_command_gen_enabled=%s", llm_settings.llm_command_gen_enabled)
        logger.info("[BENCH-SETTINGS]   llm_command_gen_verifier=%s", llm_settings.llm_command_gen_verifier)
        logger.info("[BENCH-SETTINGS]   llm_command_gen_passes=%d", llm_settings.llm_command_gen_passes)
        logger.info("[BENCH-SETTINGS]   assistant_provider=%s", llm_settings.assistant_provider)
        logger.info("[BENCH-SETTINGS]   assistant_model=%s", llm_settings.assistant_model)
        logger.info("[BENCH-SETTINGS]   eval_judge_model=%s", test_user_settings.eval_judge.model)

        logger.info("[BENCH] agent_mode=%s expected_tool=%s", agent_mode, scenario.expected_tool)
        bound_operators = await seed_operator_if_bound(
            agent_mode=agent_mode,
            operator_id=unique_operator_id,
            operator_data_service=operator_data_service,
            cleanup=cleanup,
            log_prefix="[BENCH]",
        )

        logger.info("[BENCH] bound_operators=%d", len(bound_operators))

        investigation_request = InvestigationCreateRequest(
            case_id=unique_case_id,
            case_title=f"Benchmark: {scenario.id}",
            case_description=scenario.description or "Benchmark evaluation",
            user_id=unique_user_id,
            web_session_id=unique_web_session_id,
        )
        created_investigation = await investigation_data_service.create_investigation(investigation_request)
        logger.info("[BENCH] investigation_id=%s", created_investigation.id)
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

        fake_event_service = FakeEventService()
        real_event_service = chat_pipeline.g8ed_event_service
        chat_pipeline.g8ed_event_service = fake_event_service

        approval_service = all_services['approval_service']

        def _auto_approve_callback(approval_id: str, pending: PendingApproval):
            pending.resolve(
                approved=True,
                reason="Auto-approved by benchmark test",
                responded_at=now(),
            )
            logger.info("[AUTO-APPROVE] Approved %s", approval_id)

        approval_service.set_on_approval_requested(_auto_approve_callback)

        user_settings = test_user_settings
        logger.info("[BENCH-SETTINGS] user_settings.llm.llm_command_gen_enabled=%s", user_settings.llm.llm_command_gen_enabled)
        logger.info("[BENCH-SETTINGS] user_settings.eval_judge.model=%s", user_settings.eval_judge.model)
        task_manager = ChatTaskManager()

        logger.info("[BENCH] Running scenario %s", scenario.id)

        try:
            await chat_pipeline.run_chat(
                message=scenario.user_query,
                g8e_context=g8e_context,
                attachments=[],
                sentinel_mode=False,
                llm_primary_provider=None,
                llm_assistant_provider=None,
                llm_primary_model=llm_settings.primary_model,
                llm_assistant_model=llm_settings.assistant_model,
                _task_manager=task_manager,
                user_settings=user_settings,
                _track_task=False,
            )
        finally:
            chat_pipeline.g8ed_event_service = real_event_service
            approval_service.set_on_approval_requested(None)

        captured_tribunal = _extract_tribunal_from_events(fake_event_service.published)
        captured_tool_calls = _extract_tool_calls_from_events(fake_event_service.published)

        conversation_history = await investigation_service.get_chat_messages(
            investigation_id=created_investigation.id
        )
        for msg in reversed(conversation_history):
            if msg.sender == EventType.EVENT_SOURCE_AI_PRIMARY:
                response_text = msg.content
                break

        logger.info(
            "[BENCH] tool_calls_captured=%d tribunal_captured=%s",
            len(captured_tool_calls),
            captured_tribunal is not None,
        )
        for tc in captured_tool_calls:
            logger.info("[BENCH] tool=%s", tc.tool_name)
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
