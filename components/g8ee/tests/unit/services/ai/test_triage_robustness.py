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

import pytest
from unittest.mock import patch

from app.constants import (
    TriageComplexityClassification,
    AgentMode,
)
from app.services.ai.triage import TriageAgent
from app.models.agents.triage import TriageRequest
from tests.fakes.fake_llm_provider import FakeLLMProvider

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
            primary_provider=LLMProvider.OLLAMA,
            primary_model="main-model",
            lite_provider=LLMProvider.OLLAMA,
            lite_model="lite-model"
        )
    )

async def test_triage_handles_unclosed_json(fake_provider, mock_settings):
    # Missing closing brace
    fake_provider.add_response('''{
        "intent_summary": "factual question",
        "intent": "information",
        "intent_confidence": "high",
        "complexity": "simple",
        "complexity_confidence": "high",
        "follow_up_question": null
    ''')

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

    # Should be SIMPLE after robust parsing
    assert result.complexity == TriageComplexityClassification.SIMPLE
    assert result.intent_summary == "factual question"

async def test_triage_handles_json_with_preamble(fake_provider, mock_settings):
    fake_provider.add_response('''Sure, here is the analysis:
    {
        "intent_summary": "factual question",
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

    # Should be SIMPLE after robust parsing
    assert result.complexity == TriageComplexityClassification.SIMPLE
    assert result.intent_summary == "factual question"
