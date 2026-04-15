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
ChatPipeline Accuracy Evaluation Test Suite.

Tests the real ChatPipelineService.run_chat end-to-end. This is the full agent path:
user message in, investigation context assembly, triage, system prompt, LLM call,
response persistence.

The test creates a real investigation in g8es, builds G8eHttpContext, calls
chat_pipeline.run_chat() with real services, reads the persisted AI response from
g8es, and grades it with EvalJudge (Primary Model grades Assistant Model).
"""

import os
import pytest
import logging
from typing import Any
from datetime import datetime, timezone

from app.constants import AgentMode, EventType, OperatorStatus
from app.services.ai.chat_task_manager import ChatTaskManager
from app.services.ai.eval_judge import EvalJudge, EvalJudgeError
from app.llm.factory import get_llm_provider
from app.models.settings import G8eeUserSettings, SearchSettings
from app.models.http_context import G8eHttpContext, BoundOperator
from app.models.investigations import InvestigationCreateRequest
from app.models.model_configs import get_model_config
from tests.fakes.factories import build_g8e_http_context
from tests.integration.evals.shared import (
    AccuracyTestResult,
    load_and_validate_gold_set,
    seed_operator_if_bound,
)

logger = logging.getLogger(__name__)

GOLD_SET_PATH = os.path.join(os.path.dirname(__file__), "gold_set.json")


def load_gold_set() -> list[dict[str, Any]]:
    return load_and_validate_gold_set(GOLD_SET_PATH)


# Use ai_integration marker to ensure this only runs when LLM is configured
# Use agent_eval marker for dedicated evaluation runs
pytestmark = [pytest.mark.integration, pytest.mark.ai_integration, pytest.mark.agent_eval, pytest.mark.slow, pytest.mark.timeout(180)]


@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.parametrize("scenario", load_gold_set(), ids=lambda s: s["id"])
async def test_agent_accuracy(
    scenario: dict[str, Any],
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
    eval_results_collector,
):
    """
    Evaluate the AI agent's accuracy for a specific scenario using a Judge model.

    This test uses the real ChatPipelineService.run_chat end-to-end:
    1. Creates a real investigation in g8es
    2. Builds G8eHttpContext with the investigation
    3. Calls chat_pipeline.run_chat() with real services
    4. Reads conversation history from g8es to extract AI response
    5. Grades with EvalJudge (Primary Model grades Assistant Model)
    6. Cleanup: deletes investigation from g8es
    """
    start_time = datetime.now(timezone.utc)
    result_data = AccuracyTestResult(scenario_id=scenario["id"])

    investigation_service = all_services['investigation_service']
    investigation_data_service = all_services['investigation_data_service']
    chat_pipeline = all_services['chat_pipeline']

    try:
        llm_settings = test_user_settings.llm

        # The Assistant Model is being tested
        model_name = llm_settings.assistant_model

        # Prepare the context based on the scenario's agent mode
        agent_mode_str = scenario["agent_mode"]
        agent_mode = AgentMode.OPERATOR_BOUND if agent_mode_str == "operator_bound" else AgentMode.OPERATOR_NOT_BOUND

        # Build G8E context with bound operators for operator_bound scenarios
        operator_data_service = all_services['operator_data_service']
        bound_operators = await seed_operator_if_bound(
            agent_mode=agent_mode,
            operator_id=unique_operator_id,
            operator_data_service=operator_data_service,
            cleanup=cleanup,
            log_prefix="[EVAL]",
        )

        logger.info(f"[EVAL] Scenario agent_mode={scenario['agent_mode']}, Bound operators count: {len(bound_operators)}")
        if bound_operators:
            logger.info(f"[EVAL] First bound operator: id={bound_operators[0].operator_id} session_id={bound_operators[0].operator_session_id} status={bound_operators[0].status}")

        # Step 1: Create a real investigation in g8es
        investigation_request = InvestigationCreateRequest(
            case_id=unique_case_id,
            case_title=f"Accuracy Test: {scenario['id']}",
            case_description=scenario.get("description", "Accuracy evaluation test"),
            user_id=unique_user_id,
            web_session_id=unique_web_session_id,
        )
        created_investigation = await investigation_data_service.create_investigation(investigation_request)
        logger.info(f"[EVAL] Created investigation {created_investigation.id} for scenario {scenario['id']}")
        cleanup.track_investigation(created_investigation.id)

        # Step 2: Build G8eHttpContext with the investigation
        g8e_context = G8eHttpContext(
            web_session_id=unique_web_session_id,
            user_id=unique_user_id,
            case_id=unique_case_id,
            investigation_id=created_investigation.id,
            organization_id="org-eval-001",
            source_component="g8ee",
            bound_operators=bound_operators,
        )

        # Step 3: Call chat_pipeline.run_chat() with real services
        # Enable web search for scenarios that expect it
        search_settings = SearchSettings(enabled="g8e_web_search" in scenario.get("expected_tools", []))
        user_settings = G8eeUserSettings(llm=test_user_settings.llm, search=search_settings)
        task_manager = ChatTaskManager()

        logger.info(f"[EVAL] Running scenario {scenario['id']} with model {model_name}")
        logger.info(f"[EVAL] Agent mode: {agent_mode}")
        logger.info(f"[EVAL] Investigation ID: {created_investigation.id}")

        user_query = scenario["user_query"]

        # Call run_chat with the correct signature
        await chat_pipeline.run_chat(
            message=user_query,
            g8e_context=g8e_context,
            attachments=[],
            sentinel_mode=False,
            llm_primary_provider=None,
            llm_assistant_provider=None,
            llm_primary_model=llm_settings.primary_model,
            llm_assistant_model=llm_settings.assistant_model,
            _task_manager=task_manager,
            user_settings=user_settings,
            _track_task=False,  # Don't track task for eval tests
        )

        # Step 4: Read conversation history from g8es to extract AI response
        conversation_history = await investigation_service.get_chat_messages(
            investigation_id=created_investigation.id
        )

        # Find the last AI response (should be the most recent EVENT_SOURCE_AI_PRIMARY message)
        ai_response_text = ""
        for msg in reversed(conversation_history):
            if msg.sender == EventType.EVENT_SOURCE_AI_PRIMARY:
                ai_response_text = msg.content
                break

        if not ai_response_text:
            pytest.fail(f"No AI response found in conversation history for scenario {scenario['id']}")

        result_data.response_text = ai_response_text
        logger.info(f"[EVAL] AI response length: {len(ai_response_text)} chars")

        # Step 5: Grade with EvalJudge (Primary Model grades Assistant Model)
        judge = EvalJudge(provider=get_llm_provider(llm_settings), model=llm_settings.primary_model)

        # Build interaction trace for the judge
        trace_lines = [
            f"USER_QUERY: {user_query}",
            f"AGENT_MODE: {agent_mode}",
            f"RESPONSE: {ai_response_text}",
        ]
        interaction_trace = "\n".join(trace_lines)

        try:
            grade = await judge.grade_turn(
                user_query=user_query,
                interaction_trace=interaction_trace,
                expected_behavior=scenario["expected_behavior"],
                required_concepts=scenario["required_concepts"],
                expected_tools=scenario.get("expected_tools", []),
                forbidden_tools=scenario.get("forbidden_tools", []),
            )
        except EvalJudgeError as judge_err:
            pytest.fail(f"Judge system error for {scenario['id']}: {judge_err}")

        result_data.score = grade.score
        result_data.reasoning = grade.reasoning
        result_data.passed = grade.passed

        # Calculate execution time
        end_time = datetime.now(timezone.utc)
        result_data.execution_time_ms = (end_time - start_time).total_seconds() * 1000

        # Add result to collector for end-of-test summary
        eval_results_collector.add_result(result_data.to_dict())

        # Output structured result
        logger.info("=" * 60)
        logger.info(f"[EVAL_RESULT] Scenario: {scenario['id']}")
        logger.info(f"[EVAL_RESULT] Score: {grade.score}/5")
        logger.info(f"[EVAL_RESULT] Passed: {grade.passed}")
        logger.info(f"[EVAL_RESULT] Execution Time: {result_data.execution_time_ms:.1f}ms")
        logger.info(f"[EVAL_RESULT] Reasoning: {grade.reasoning}")
        logger.info("=" * 60)

        # Assert that the evaluation passed
        assert grade.passed, (
            f"Accuracy evaluation failed for {scenario['id']}: "
            f"{grade.reasoning} (Score: {grade.score})"
        )

    except Exception as e:
        result_data.error = str(e)
        result_data.passed = False
        logger.exception(f"[EVAL] Fatal error in scenario {scenario['id']}: {e}")
        raise
