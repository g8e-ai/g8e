# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from unittest.mock import patch

import pytest

from app.constants import (
    AgentMode,
    TriageComplexityClassification,
)
from app.models.agents.triage import TriageRequest
from app.services.ai.triage import TriageAgent
from tests.fakes.fake_llm_provider import FakeLLMProvider

pytestmark = [pytest.mark.unit, pytest.mark.asyncio]


@pytest.fixture
def fake_provider():
    return FakeLLMProvider()


@pytest.fixture
def mock_settings():
    from app.constants import LLMProvider
    from app.models.settings import G8eeUserSettings, LLMSettings

    return G8eeUserSettings(
        llm=LLMSettings(
            primary_provider=LLMProvider.OLLAMA,
            primary_model="main-model",
            lite_provider=LLMProvider.OLLAMA,
            lite_model="lite-model",
        )
    )


async def test_triage_handles_unclosed_json(fake_provider, mock_settings):
    # Missing closing brace
    fake_provider.add_response("""{
        "intent_summary": "factual question",
        "intent": "information",
        "intent_confidence": "high",
        "complexity": "simple",
        "complexity_confidence": "high"
    """)

    agent = TriageAgent()
    request = TriageRequest(
        message="What is DNS?",
        agent_mode=AgentMode.G8E_NOT_BOUND,
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
    fake_provider.add_response("""Sure, here is the analysis:
    {
        "intent_summary": "factual question",
        "intent": "information",
        "intent_confidence": "high",
        "complexity": "simple",
        "complexity_confidence": "high"
    }""")

    agent = TriageAgent()
    request = TriageRequest(
        message="What is DNS?",
        agent_mode=AgentMode.G8E_NOT_BOUND,
        conversation_history=[],
        attachments=[],
        settings=mock_settings,
    )

    with patch("app.services.ai.triage.get_llm_provider", return_value=fake_provider):
        result = await agent.triage(request)

    # Should be SIMPLE after robust parsing
    assert result.complexity == TriageComplexityClassification.SIMPLE
    assert result.intent_summary == "factual question"
