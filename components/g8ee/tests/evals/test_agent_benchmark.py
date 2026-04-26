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
from app.constants.status import OperatorToolName
from app.models.events import SessionEvent
from app.models.g8ed_client import AIToolLifecyclePayload
from app.models.operators import CommandExecutingBroadcastEvent
from app.models.agents.tribunal import TribunalSessionCompletedPayload
from app.models.http_context import G8eHttpContext
from app.models.investigations import InvestigationCreateRequest
from app.models.operators import ApprovalType
from tests.fakes.fake_event_service import FakeEventService
from tests.evals.shared import (
    BenchmarkTestResult,
    load_and_validate_benchmark_set,
    seed_operator_if_bound,
)
from tests.integration.conftest import auto_approve_inline_callback

logger = logging.getLogger(__name__)

def load_benchmark_set() -> list[dict[str, Any]]:
    return load_and_validate_benchmark_set(PATHS["g8ee"]["evals"]["benchmark_path"])


# 600s per scenario: multi-step investigations can run up to AGENT_MAX_TOOL_TURNS
# (25) before requesting an AGENT_CONTINUE approval; at real LLM latencies plus a
# post-continue continuation this exceeds the 180s we used before the continue
# mechanism existed. AGENT_CONTINUE_APPROVAL_TIMEOUT_SECONDS is itself 600s.
pytestmark = [
    pytest.mark.integration,
    pytest.mark.ai_integration,
    pytest.mark.agent_benchmark,
    pytest.mark.slow,
    pytest.mark.timeout(600),
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
                    final_command=payload.final_command,
                    outcome=payload.outcome,
                    vote_score=payload.vote_score,
                    auditor_passed=None,
                    auditor_revision=None,
                )

    return None


# Map native per-tool STARTED events to the canonical OperatorToolName each
# event family represents. The native event contract dedicates one event family
# per tool, so the event_type is the source of truth for which tool was called.
# Multi-tool families (e.g. OPERATOR_FILE_EDIT_STARTED covers create/write/update)
# are intentionally mapped to their representative AI-facing tool name — the
# benchmark only asserts tool-family-level expectations, not sub-operation.
_OPERATOR_STARTED_EVENT_TO_TOOL: dict[EventType, str] = {
    EventType.OPERATOR_COMMAND_STARTED: OperatorToolName.RUN_COMMANDS,
    EventType.OPERATOR_NETWORK_PORT_CHECK_STARTED: OperatorToolName.CHECK_PORT,
    EventType.OPERATOR_FILE_EDIT_STARTED: OperatorToolName.FILE_WRITE,
    EventType.OPERATOR_FILESYSTEM_LIST_STARTED: OperatorToolName.LIST_FILES,
    EventType.OPERATOR_FILESYSTEM_READ_STARTED: OperatorToolName.READ_FILE_CONTENT,
}

_UNIVERSAL_TOOL_REQUESTED_EVENT_TO_TOOL: dict[EventType, str] = {
    EventType.LLM_TOOL_G8E_WEB_SEARCH_REQUESTED: OperatorToolName.G8E_SEARCH_WEB,
    EventType.LLM_TOOL_G8E_INVESTIGATION_QUERY_REQUESTED: OperatorToolName.QUERY_INVESTIGATION_CONTEXT,
    EventType.LLM_TOOL_G8E_COMMAND_CONSTRAINTS_REQUESTED: OperatorToolName.GET_COMMAND_CONSTRAINTS,
}


def _extract_tool_calls_from_events(
    published: list[SessionEvent],
) -> list[ToolCallCapture]:
    """Extract tool calls from native per-tool STARTED/REQUESTED events.

    Tool calls are not persisted in conversation history, so we intercept the
    per-tool native lifecycle events published by each operator service
    (OPERATOR_*_STARTED) and each universal AI tool (LLM_TOOL_G8E_*_REQUESTED).
    """
    tool_calls: list[ToolCallCapture] = []

    for event in published:
        if not isinstance(event, SessionEvent):
            continue

        tool_name = _OPERATOR_STARTED_EVENT_TO_TOOL.get(event.event_type)
        if tool_name is not None:
            payload = event.payload
            command = payload.command if isinstance(payload, CommandExecutingBroadcastEvent) else ""
            tool_calls.append(ToolCallCapture(
                tool_name=tool_name,
                args={"command": command} if command else {},
            ))
            continue

        tool_name = _UNIVERSAL_TOOL_REQUESTED_EVENT_TO_TOOL.get(event.event_type)
        if tool_name is not None:
            payload = event.payload
            detail = payload.display_detail if isinstance(payload, AIToolLifecyclePayload) else None
            tool_calls.append(ToolCallCapture(
                tool_name=tool_name,
                args={"command": detail} if detail else {},
            ))

    return tool_calls


@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.parametrize("scenario_data", load_benchmark_set(), ids=lambda s: s["id"])
async def test_agent_benchmark(
    scenario_data: dict[str, Any],
    all_services,
    cache_aside_service,
    test_settings,
    user_settings,
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
    result_data = BenchmarkTestResult(
        scenario_id=scenario.id,
        category=scenario.category,
        dimension=scenario_data.get("dimension", "accuracy")
    )

    logger.info("[BENCH] ===== Starting test for scenario %s =====", scenario.id)

    investigation_service = all_services['investigation_service']
    investigation_data_service = all_services['investigation_data_service']
    operator_data_service = all_services['operator_data_service']
    chat_pipeline = all_services['chat_pipeline']

    response_text = ""

    try:
        llm_settings = user_settings.llm
        agent_mode = AgentMode.OPERATOR_BOUND if scenario.agent_mode == "OPERATOR_BOUND" else AgentMode.OPERATOR_NOT_BOUND

        logger.info("[BENCH-SETTINGS] Tribunal configuration:")
        logger.info("[BENCH-SETTINGS]   llm_command_gen_enabled=%s", llm_settings.llm_command_gen_enabled)
        logger.info("[BENCH-SETTINGS]   llm_command_gen_auditor=%s", llm_settings.llm_command_gen_auditor)
        logger.info("[BENCH-SETTINGS]   llm_command_gen_passes=%d", llm_settings.llm_command_gen_passes)
        logger.info("[BENCH-SETTINGS]   assistant_provider=%s", llm_settings.assistant_provider)
        logger.info("[BENCH-SETTINGS]   assistant_model=%s", llm_settings.assistant_model)
        logger.info("[BENCH-SETTINGS]   eval_judge_model=%s", user_settings.eval_judge.model)

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
        logger.info("[BENCH-SETTINGS] user_settings.llm.llm_command_gen_enabled=%s", user_settings.llm.llm_command_gen_enabled)
        logger.info("[BENCH-SETTINGS] user_settings.eval_judge.model=%s", user_settings.eval_judge.model)
        task_manager = all_services['chat_task_manager']

        logger.info("[BENCH] Running scenario %s", scenario.id)

        # Inline approval auto-approver handles every approval type uniformly,
        # including AGENT_CONTINUE requests emitted when the agent exceeds
        # AGENT_MAX_TOOL_TURNS. The post-hoc auto_approve_pending helper used
        # by other eval tests would deadlock here because chat_pipeline.run_chat
        # itself blocks on PendingApproval.wait() mid-invocation.
        try:
            with auto_approve_inline_callback(approval_service) as approval_tracker:
                await chat_pipeline.run_chat(
                    message=scenario.user_query,
                    g8e_context=g8e_context,
                    attachments=[],
                    sentinel_mode=False,
                    llm_primary_provider=None,
                    llm_assistant_provider=None,
                    llm_lite_provider=None,
                    llm_primary_model=llm_settings.primary_model,
                    llm_assistant_model=llm_settings.assistant_model,
                    llm_lite_model=llm_settings.lite_model,
                    _task_manager=task_manager,
                    user_settings=user_settings,
                    _track_task=False,
                )
        finally:
            chat_pipeline.g8ed_event_service = real_event_service

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
            result_data.tribunal_final_command = grade.tribunal_final_command
            result_data.tribunal_outcome = grade.tribunal_outcome

        result_data.agent_continue_approvals = approval_tracker.count(ApprovalType.AGENT_CONTINUE)
        result_data.approvals_by_type = {
            atype.value: count for atype, count in approval_tracker.counts.items()
        }

        end_time = datetime.now(timezone.utc)
        result_data.execution_time_ms = (end_time - start_time).total_seconds() * 1000

        benchmark_results_collector.add_result(result_data.to_dict())

        status = "PASS" if grade.passed else "FAIL"
        logger.info("=" * 60)
        logger.info("[BENCH_RESULT] %s %s", status, scenario.id)
        logger.info("[BENCH_RESULT] Matchers: %d/%d", grade.matchers_passed, grade.matchers_total)
        logger.info("[BENCH_RESULT] Execution Time: %.1fms", result_data.execution_time_ms)
        if result_data.agent_continue_approvals:
            logger.info(
                "[BENCH_RESULT] AGENT_CONTINUE approvals: %d (total approvals=%d, by_type=%s)",
                result_data.agent_continue_approvals,
                approval_tracker.total,
                result_data.approvals_by_type,
            )
        if grade.failures:
            for f in grade.failures:
                logger.info("[BENCH_RESULT] FAILURE: %s", f[:200])
        if grade.tribunal_outcome:
            logger.info("[BENCH_RESULT] Tribunal: %s -> %s",
                grade.tribunal_final_command, grade.tribunal_outcome)
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
