# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 4 external tests for eval judge via live Jev API calls."""

from __future__ import annotations

import os

import pytest

from app.constants import JEV_DEFAULT_MODEL, OLLAMA_DEFAULT_ENDPOINT, LLMProvider
from app.constants.env_vars import EnvVar
from app.decision import get_decision_provider
from app.models.settings import EvalJudgeSettings, G8eeUserSettings, LLMSettings
from app.services.ai.eval_judge import PASSING_THRESHOLD, EvalJudge

pytestmark = [pytest.mark.integration, pytest.mark.requires_system_one, pytest.mark.slow]

FROZEN_ASSIGNMENT = {
    "user_query": "Which service failed in these logs?",
    "interaction_trace": (
        "Designated role output:\nThe checkout-api service is failing with HTTP 503 errors."
    ),
    "expected_behavior": "Identify checkout-api as the failing service",
    "required_concepts": ["log-analysis", "checkout-api"],
    "expected_tools": [],
    "forbidden_tools": [],
}


@pytest.fixture
def jev_settings() -> G8eeUserSettings:
    return G8eeUserSettings(
        llm=LLMSettings(
            primary_provider=LLMProvider.OLLAMA,
            primary_model="main-model",
            lite_provider=LLMProvider.JEV,
            lite_model=JEV_DEFAULT_MODEL,
            ollama_endpoint=os.environ.get(EnvVar.LLM_OLLAMA_ENDPOINT) or OLLAMA_DEFAULT_ENDPOINT,
        ),
        eval_judge=EvalJudgeSettings(eval_judge_model=JEV_DEFAULT_MODEL),
    )


async def test_eval_judge_grades_frozen_assignment_via_live_jev(jev_settings: G8eeUserSettings):
    decision_provider = get_decision_provider(jev_settings.llm)
    judge = EvalJudge(
        decision_provider=decision_provider,
        model=jev_settings.eval_judge.model,
    )

    grade = await judge.grade_turn(**FROZEN_ASSIGNMENT)

    assert 1 <= grade.score <= 5
    assert grade.reasoning
    assert grade.passed == (grade.score >= PASSING_THRESHOLD)
    assert len(grade.model_calls) == 1
    assert grade.model_calls[0].succeeded is True
    assert "Jev rubric score" in grade.reasoning
