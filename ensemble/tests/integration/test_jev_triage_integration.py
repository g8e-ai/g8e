# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 4 external tests for triage via live Jev API calls."""

from __future__ import annotations

import os

import pytest

from app.constants import AgentMode, JEV_DEFAULT_MODEL, LLMProvider
from app.constants.env_vars import EnvVar
from app.models.agents.triage import TriageRequest, TriageResult
from app.models.settings import G8eeUserSettings, LLMSettings
from app.services.ai.triage import TriageAgent

pytestmark = [pytest.mark.integration, pytest.mark.requires_typesafe, pytest.mark.slow]


@pytest.fixture
def jev_settings() -> G8eeUserSettings:
    api_key = os.environ.get(EnvVar.LLM_JEV_API_KEY) or os.environ.get(EnvVar.TYPESAFE_API_KEY)
    if not api_key:
        pytest.skip("TYPESAFE_API_KEY or G8E_LLM_JEV_API_KEY required")

    return G8eeUserSettings(
        llm=LLMSettings(
            primary_provider=LLMProvider.OLLAMA,
            primary_model="main-model",
            lite_provider=LLMProvider.JEV,
            lite_model=JEV_DEFAULT_MODEL,
            jev_api_key=api_key,
        )
    )


async def test_triage_classifies_simple_question_via_live_jev(jev_settings: G8eeUserSettings):
    agent = TriageAgent()
    request = TriageRequest(
        message="What is DNS?",
        agent_mode=AgentMode.G8E_NOT_BOUND,
        conversation_history=[],
        attachments=[],
        settings=jev_settings,
    )

    result = await agent.triage(request)

    assert isinstance(result, TriageResult)
    assert result.model_call is not None
    assert result.model_call.succeeded is True
    assert result.intent_summary
