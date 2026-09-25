# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Unit tests for triage via the Jev decision provider."""

from __future__ import annotations

from unittest.mock import patch

import pytest

from app.constants import (
    AgentMode,
    JEV_DEFAULT_MODEL,
    LLMProvider,
    TriageComplexityClassification,
    TriageConfidence,
    TriageIntentClassification,
    TriageRequestPosture,
)
from app.decision.types import (
    ChoiceAnswer,
    EvaluateResponse,
    EvaluateUsage,
)
from app.models.agents.triage import TriageRequest
from app.models.settings import G8eeUserSettings, LLMSettings
from app.services.ai.triage import JEV_TRIAGE_HIGH_CONFIDENCE_THRESHOLD, TriageAgent
from tests.fakes.fake_decision_provider import FakeDecisionProvider

pytestmark = [pytest.mark.unit, pytest.mark.asyncio]


@pytest.fixture
def jev_settings() -> G8eeUserSettings:
    return G8eeUserSettings(
        llm=LLMSettings(
            primary_provider=LLMProvider.OLLAMA,
            primary_model="main-model",
            lite_provider=LLMProvider.JEV,
            lite_model=JEV_DEFAULT_MODEL,
            jev_api_key="ts_test_key",
        )
    )


def _jev_response(**overrides) -> EvaluateResponse:
    answers = {
        "complexity": ChoiceAnswer(
            choice="simple",
            confidence=0.95,
            probabilities={"simple": 0.95, "complex": 0.05},
        ),
        "intent": ChoiceAnswer(
            choice="information",
            confidence=0.91,
            probabilities={"information": 0.91, "action": 0.09},
        ),
        "request_posture": ChoiceAnswer(
            choice="normal",
            confidence=0.88,
            probabilities={"normal": 0.88, "escalated": 0.12},
        ),
    }
    answers.update(overrides.get("answers", {}))
    return EvaluateResponse(
        model=overrides.get("model", "jev-latest"),
        answers=answers,
        usage=EvaluateUsage(input_tokens=120, output_tokens=30),
    )


async def test_triage_jev_maps_decision_answers_to_triage_result(
    jev_settings: G8eeUserSettings,
):
    fake_provider = FakeDecisionProvider()
    fake_provider.add_response(_jev_response())

    agent = TriageAgent()
    request = TriageRequest(
        message="What is DNS?",
        agent_mode=AgentMode.G8E_NOT_BOUND,
        conversation_history=[],
        attachments=[],
        settings=jev_settings,
    )

    with patch("app.services.ai.triage.get_decision_provider", return_value=fake_provider):
        result = await agent.triage(request)

    assert result.complexity == TriageComplexityClassification.SIMPLE
    assert result.intent == TriageIntentClassification.INFORMATION
    assert result.request_posture == TriageRequestPosture.NORMAL
    assert result.complexity_confidence == TriageConfidence.HIGH
    assert result.intent_confidence == TriageConfidence.HIGH
    assert result.posture_confidence == TriageConfidence.HIGH
    assert "information intent" in result.intent_summary
    assert result.model_call is not None
    assert result.model_call.provider == "FakeDecisionProvider"
    assert result.model_call.input_tokens == 120
    assert result.model_call.output_tokens == 30


async def test_triage_jev_batches_questions_in_one_evaluate_call(
    jev_settings: G8eeUserSettings,
):
    fake_provider = FakeDecisionProvider()
    fake_provider.add_response(_jev_response())

    agent = TriageAgent()
    request = TriageRequest(
        message="Reset my password",
        agent_mode=AgentMode.G8E_BOUND,
        conversation_history=[],
        attachments=[],
        settings=jev_settings,
    )

    with patch("app.services.ai.triage.get_decision_provider", return_value=fake_provider):
        await agent.triage(request)

    assert fake_provider.last_request is not None
    questions = fake_provider.last_request["questions"]
    assert set(questions.keys()) == {"complexity", "intent", "request_posture"}
    assert fake_provider.last_request["model"] == JEV_DEFAULT_MODEL
    assert "Reset my password" in str(fake_provider.last_request["state"])


async def test_triage_jev_maps_low_confidence_from_probability_threshold(
    jev_settings: G8eeUserSettings,
):
    fake_provider = FakeDecisionProvider()
    fake_provider.add_response(
        _jev_response(
            answers={
                "complexity": ChoiceAnswer(
                    choice="complex",
                    confidence=JEV_TRIAGE_HIGH_CONFIDENCE_THRESHOLD - 0.01,
                    probabilities={"complex": 0.84, "simple": 0.16},
                )
            }
        )
    )

    agent = TriageAgent()
    request = TriageRequest(
        message="Fix it",
        agent_mode=AgentMode.G8E_NOT_BOUND,
        conversation_history=[],
        attachments=[],
        settings=jev_settings,
    )

    with patch("app.services.ai.triage.get_decision_provider", return_value=fake_provider):
        result = await agent.triage(request)

    assert result.complexity == TriageComplexityClassification.COMPLEX
    assert result.complexity_confidence == TriageConfidence.LOW


async def test_triage_jev_escalates_on_provider_failure(jev_settings: G8eeUserSettings):
    fake_provider = FakeDecisionProvider()

    agent = TriageAgent()
    request = TriageRequest(
        message="hello",
        agent_mode=AgentMode.G8E_NOT_BOUND,
        conversation_history=[],
        attachments=[],
        settings=jev_settings,
    )

    with patch("app.services.ai.triage.get_decision_provider", return_value=fake_provider):
        result = await agent.triage(request)

    assert result.complexity == TriageComplexityClassification.COMPLEX
    assert result.complexity_confidence == TriageConfidence.LOW
    assert result.error_code == "CLASSIFICATION_ERROR"
    assert result.model_call is not None
    assert result.model_call.succeeded is False


async def test_triage_jev_short_circuits_attachments_without_decision_provider(
    jev_settings: G8eeUserSettings,
):
    from app.models.attachments import AttachmentMetadata

    agent = TriageAgent()
    request = TriageRequest(
        message="check logs",
        agent_mode=AgentMode.G8E_NOT_BOUND,
        conversation_history=[],
        attachments=[AttachmentMetadata(filename="logs.txt", content_type="text/plain")],
        settings=jev_settings,
    )

    with patch("app.services.ai.triage.get_decision_provider") as mock_get_provider:
        result = await agent.triage(request)

    mock_get_provider.assert_not_called()
    assert result.complexity == TriageComplexityClassification.COMPLEX
    assert "attachments" in result.intent_summary
