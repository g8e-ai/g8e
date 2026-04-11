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
Unit tests for the AI Triage Service (triage_message).

Covers:
- Short-circuit for attachments (always COMPLEX)
- Short-circuit for empty message (always COMPLEX + follow-up)
- Successful triage with assistant model (SIMPLE vs COMPLEX)
- Handling of LOW confidence and follow-up questions
- Resilience to malformed JSON or empty model responses
"""

import pytest
from unittest.mock import AsyncMock, MagicMock, patch

from tests.fakes.fake_llm_provider import FakeLLMProvider
from app.constants import (
    TriageComplexityClassification,
    TriageConfidence,
    TriageIntentClassification,
    AgentMode,
    GeminiRole,
)

from app.llm import llm_types as types
from app.models.attachments import AttachmentMetadata
from app.models.agents.triage import TriageResult, TriageRequest
from app.services.ai.triage import TriageAgent

pytestmark = [pytest.mark.unit, pytest.mark.asyncio]


@pytest.fixture
def fake_provider():
    return FakeLLMProvider()


@pytest.fixture
def mock_settings():
    from app.models.settings import G8eeUserSettings, LLMSettings
    from app.constants import LLMProvider
    return G8eeUserSettings(
        llm=LLMSettings(
            provider=LLMProvider.OLLAMA,
            primary_model="main-model",
            assistant_model="lite-model"
        )
    )


# ---------------------------------------------------------------------------
# Short-circuit Tests
# ---------------------------------------------------------------------------

async def test_triage_escalates_immediately_if_attachments_present(mock_settings):
    att = AttachmentMetadata(filename="logs.txt", content_type="text/plain")
    agent = TriageAgent()
    request = TriageRequest(
        message="check logs",
        agent_mode=AgentMode.OPERATOR_NOT_BOUND,
        conversation_history=[],
        attachments=[att],
        settings=mock_settings,
    )
    result = await agent.triage(request)

    assert result.complexity == TriageComplexityClassification.COMPLEX
    assert result.complexity_confidence == TriageConfidence.HIGH
    assert "attachments" in result.intent_summary


async def test_triage_escalates_immediately_if_message_empty(mock_settings):
    agent = TriageAgent()
    request = TriageRequest(
        message="  ",
        agent_mode=AgentMode.OPERATOR_NOT_BOUND,
        conversation_history=[],
        attachments=[],
        settings=mock_settings,
    )
    result = await agent.triage(request)

    assert result.complexity == TriageComplexityClassification.COMPLEX
    assert result.intent == TriageIntentClassification.UNKNOWN
    assert result.follow_up_question is not None


# ---------------------------------------------------------------------------
# LLM Triage Tests
# ---------------------------------------------------------------------------

async def test_triage_returns_simple_classification_from_llm(fake_provider, mock_settings):
    fake_provider.add_response('''{
            "intent_summary": "factual question about DNS",
            "intent": "information",
            "intent_confidence": "high",
            "complexity": "simple",
            "complexity_confidence": "high",
            "follow_up_question": null
        }''')

    agent = TriageAgent()
    request = TriageRequest(
        message="What is DNS?",
        agent_mode=AgentMode.OPERATOR_NOT_BOUND,
        conversation_history=[],
        attachments=[],
        settings=mock_settings,
    )

    with patch("app.services.ai.triage.get_llm_provider", return_value=fake_provider):
        result = await agent.triage(request)

    assert result.complexity == TriageComplexityClassification.SIMPLE
    assert result.intent == TriageIntentClassification.INFORMATION
    assert result.complexity_confidence == TriageConfidence.HIGH


async def test_triage_returns_complex_classification_from_llm(fake_provider, mock_settings):
    fake_provider.add_response('''{
            "intent_summary": "request to debug nginx",
            "intent": "action",
            "intent_confidence": "high",
            "complexity": "complex",
            "complexity_confidence": "high",
            "follow_up_question": null
        }''')

    agent = TriageAgent()
    request = TriageRequest(
        message="Debug nginx errors",
        agent_mode=AgentMode.OPERATOR_NOT_BOUND,
        conversation_history=[],
        attachments=[],
        settings=mock_settings,
    )

    with patch("app.services.ai.triage.get_llm_provider", return_value=fake_provider):
        result = await agent.triage(request)

    assert result.complexity == TriageComplexityClassification.COMPLEX
    assert result.intent == TriageIntentClassification.ACTION


async def test_triage_handles_low_confidence_and_follow_up(fake_provider, mock_settings):
    fake_provider.add_response('''{
            "intent_summary": "ambiguous request",
            "intent": "unknown",
            "intent_confidence": "low",
            "complexity": "complex",
            "complexity_confidence": "low",
            "follow_up_question": "Could you clarify which system you are referring to?"
        }''')

    agent = TriageAgent()
    request = TriageRequest(
        message="Fix it",
        agent_mode=AgentMode.OPERATOR_NOT_BOUND,
        conversation_history=[],
        attachments=[],
        settings=mock_settings,
    )

    with patch("app.services.ai.triage.get_llm_provider", return_value=fake_provider):
        result = await agent.triage(request)

    assert result.intent_confidence == TriageConfidence.LOW
    assert result.complexity_confidence == TriageConfidence.LOW
    assert result.follow_up_question == "Could you clarify which system you are referring to?"


# ---------------------------------------------------------------------------
# Error Handling & Resilience
# ---------------------------------------------------------------------------

async def test_triage_defaults_to_complex_on_malformed_json(fake_provider, mock_settings):
    fake_provider.add_response("not json")

    agent = TriageAgent()
    request = TriageRequest(
        message="hello",
        agent_mode=AgentMode.OPERATOR_NOT_BOUND,
        conversation_history=[],
        attachments=[],
        settings=mock_settings,
    )

    with patch("app.services.ai.triage.get_llm_provider", return_value=fake_provider):
        result = await agent.triage(request)

    assert result.complexity == TriageComplexityClassification.COMPLEX
    assert result.complexity_confidence == TriageConfidence.LOW


async def test_triage_defaults_to_complex_on_empty_model_response(fake_provider, mock_settings):
    fake_provider.add_response("")

    agent = TriageAgent()
    request = TriageRequest(
        message="hello",
        agent_mode=AgentMode.OPERATOR_NOT_BOUND,
        conversation_history=[],
        attachments=[],
        settings=mock_settings,
    )

    with patch("app.services.ai.triage.get_llm_provider", return_value=fake_provider):
        result = await agent.triage(request)

    assert result.complexity == TriageComplexityClassification.COMPLEX
    assert result.complexity_confidence == TriageConfidence.LOW


async def test_triage_defaults_to_complex_on_llm_exception(fake_provider, mock_settings):
    agent = TriageAgent()
    request = TriageRequest(
        message="hello",
        agent_mode=AgentMode.OPERATOR_NOT_BOUND,
        conversation_history=[],
        attachments=[],
        settings=mock_settings,
    )
    with patch("app.services.ai.triage.get_llm_provider", side_effect=Exception("LLM Down")):
        result = await agent.triage(request)

    assert result.complexity == TriageComplexityClassification.COMPLEX
    assert result.intent_summary == "Error during triage: LLM Down"


async def test_triage_cleans_markdown_json_blocks(fake_provider, mock_settings):
    fake_provider.add_response('''```json
        {
            "intent_summary": "simple greeting",
            "intent": "information",
            "intent_confidence": "high",
            "complexity": "simple",
            "complexity_confidence": "high",
            "follow_up_question": null
        }
        ```''')

    agent = TriageAgent()
    request = TriageRequest(
        message="Hi",
        agent_mode=AgentMode.OPERATOR_NOT_BOUND,
        conversation_history=[],
        attachments=[],
        settings=mock_settings,
    )

    with patch("app.services.ai.triage.get_llm_provider", return_value=fake_provider):
        result = await agent.triage(request)

    assert result.complexity == TriageComplexityClassification.SIMPLE


async def test_triage_uses_provided_model_override(fake_provider, mock_settings):
    fake_provider.add_response('{"complexity": "simple", "intent": "information", "intent_confidence": "high", "complexity_confidence": "high", "intent_summary": "test"}')

    agent = TriageAgent()
    request = TriageRequest(
        message="Hi",
        agent_mode=AgentMode.OPERATOR_NOT_BOUND,
        conversation_history=[],
        attachments=[],
        settings=mock_settings,
        model_override="custom-model",
    )

    with patch("app.services.ai.triage.get_llm_provider", return_value=fake_provider):
        with patch("app.services.ai.triage.AIGenerationConfigBuilder.get_lite_generation_config") as mock_config:
            mock_config.return_value = MagicMock()
            await agent.triage(request)

            # Verify custom model was used in config builder
            args, kwargs = mock_config.call_args
            assert kwargs["model"] == "custom-model"
