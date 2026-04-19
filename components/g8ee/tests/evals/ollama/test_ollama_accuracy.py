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
Ollama Model Accuracy Evaluation Test Suite.

Tests raw Ollama model quality without the ChatPipeline. This is a direct
LLM evaluation that tests the model's ability to respond to scenarios in the
gold set without any pipeline overhead, investigation context, or g8es I/O.

Test flow:
1. Build system prompt via build_modular_system_prompt
2. Build PrimaryLLMSettings via AIGenerationConfigBuilder.build_primary_settings
3. Call llm_provider.generate_content directly (no pipeline, no g8es)
4. Extract response text from GenerateContentResponse
5. Grade with EvalJudge

Skips if settings.llm.primary_provider != LLMProvider.OLLAMA.
"""

import pytest
import logging
from typing import Any
from datetime import datetime, timezone

import httpx
import app.llm.llm_types as types
from app.constants import LLMProvider
from app.constants.paths import PATHS
from app.llm.prompts import build_modular_system_prompt
from app.llm.factory import get_llm_settings
from app.services.ai.generation_config_builder import AIGenerationConfigBuilder
from app.services.ai.eval_judge import EvalJudge, EvalGrade, EvalJudgeError
from tests.evals.shared import AccuracyTestResult, load_and_validate_gold_set

logger = logging.getLogger(__name__)

def load_gold_set() -> list[dict[str, Any]]:
    return load_and_validate_gold_set(PATHS["g8ee"]["evals"]["gold_set_path"])


# Use ai_integration marker to ensure this only runs when LLM is configured
# Use agent_eval marker for dedicated evaluation runs
pytestmark = [pytest.mark.integration, pytest.mark.ai_integration, pytest.mark.agent_eval, pytest.mark.slow, pytest.mark.timeout(180)]


@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.parametrize("scenario", load_gold_set(), ids=lambda s: s["id"])
async def test_ollama_accuracy(
    scenario: dict[str, Any],
    llm_provider,
    test_settings,
    eval_results_collector,
):
    """
    Evaluate raw Ollama model accuracy for a specific scenario using a Judge model.

    This test bypasses the ChatPipeline entirely and calls the LLM provider directly:
    1. Build system prompt (no investigation context, no operator context)
    2. Build GenerateContentConfig (no tools)
    3. Call llm_provider.generate_content directly
    4. Extract response text from GenerateContentResponse
    5. Grade with EvalJudge

    This isolates the model's raw quality from pipeline behavior.
    """
    start_time = datetime.now(timezone.utc)
    result_data = AccuracyTestResult(scenario_id=scenario["id"], dimension=scenario.get("dimension", "accuracy"))

    try:
        llm_settings = get_llm_settings()
        if not llm_settings or not llm_settings.primary_model:
            pytest.skip("LLM provider is not configured")

        if llm_settings.primary_provider != LLMProvider.OLLAMA:
            pytest.skip(f"This test only runs with Ollama provider, current provider: {llm_settings.primary_provider}")

        # Use the primary model for this test (raw model quality)
        model_name = llm_settings.primary_model

        logger.info(f"[OLLAMA_EVAL] Running scenario {scenario['id']} with model {model_name}")

        # Step 1: Build system prompt (minimal, no investigation context)
        system_prompt = build_modular_system_prompt(
            operator_bound=False,
            system_context=None,
            user_memories=[],
            case_memories=[],
            investigation=None,
            g8e_web_search_available=False,
        )

        # Step 2: Build PrimaryLLMSettings (no tools)
        generation_settings = AIGenerationConfigBuilder.build_primary_settings(
            model=model_name,
            max_tokens=llm_settings.llm_max_tokens or 4096,
            system_instructions=system_prompt,
            tools=[],
        )

        # Step 3: Call llm_provider.generate_content_primary directly
        user_query = scenario["user_query"]
        contents = [types.Content(role="user", parts=[types.Part.from_text(text=user_query)])]

        try:
            response = await llm_provider.generate_content_primary(
                model=model_name,
                contents=contents,
                primary_llm_settings=generation_settings,
            )
        except (httpx.ConnectTimeout, httpx.ConnectError, httpx.RemoteProtocolError) as e:
            pytest.skip(
                f"Ollama endpoint at {llm_settings.ollama_endpoint} is not reachable. "
                f"Ensure Ollama is running and accessible. Error: {e}"
            )

        # Step 4: Extract response text using canonical response.text property
        response_text = response.text if response.text else ""

        if not response_text:
            pytest.fail(f"No response text found for scenario {scenario['id']}")

        result_data.response_text = response_text
        logger.info(f"[OLLAMA_EVAL] Response length: {len(response_text)} chars")

        # Step 5: Grade with EvalJudge
        judge = EvalJudge(provider=llm_provider, model=llm_settings.primary_model)

        # Build interaction trace for the judge
        trace_lines = [
            f"USER_QUERY: {user_query}",
            f"MODEL: {model_name}",
            f"RESPONSE: {response_text}",
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
        logger.info(f"[OLLAMA_EVAL_RESULT] Scenario: {scenario['id']}")
        logger.info(f"[OLLAMA_EVAL_RESULT] Score: {grade.score}/5")
        logger.info(f"[OLLAMA_EVAL_RESULT] Passed: {grade.passed}")
        logger.info(f"[OLLAMA_EVAL_RESULT] Execution Time: {result_data.execution_time_ms:.1f}ms")
        logger.info(f"[OLLAMA_EVAL_RESULT] Reasoning: {grade.reasoning}")
        logger.info("=" * 60)

        # Assert that the evaluation passed
        assert grade.passed, (
            f"Ollama accuracy evaluation failed for {scenario['id']}: "
            f"{grade.reasoning} (Score: {grade.score})"
        )

    except Exception as e:
        result_data.error = str(e)
        result_data.passed = False
        logger.exception(f"[OLLAMA_EVAL] Fatal error in scenario {scenario['id']}: {e}")
        raise
