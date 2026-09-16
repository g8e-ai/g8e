# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from unittest.mock import AsyncMock, patch

import pytest

from app.models.evaluation_trace import EvaluationToolCallRecord
from app.models.http_context import G8eHttpContext
from app.models.model_telemetry import ModelCallTelemetry
from app.models.settings import EvalJudgeSettings, G8eeUserSettings
from app.services.ai.eval_judge import EvalGrade
from app.services.evaluation.semantic_grader import grade_campaign_assignment_semantically
from g8e.models.internal_api import (
    EvaluationGoldSummary,
    EvaluationInferenceContext,
    InferenceModelVariant,
)


def _evaluation_context() -> EvaluationInferenceContext:
    return EvaluationInferenceContext(
        campaign_id="campaign-1",
        run_id="run-1",
        assignment_id="assignment-1",
        evaluation_attempt_id="attempt-1",
        scenario_id="tech-log-parse",
        model_registry_digest="d" * 64,
        model_registry=[InferenceModelVariant(model="model-a", digest="a" * 64)],
        target_operator_session_id="session-1",
        evaluation_lane="model_role",
        designated_model_role="primary",
        grading_method="semantic_judge",
        gold_summary=EvaluationGoldSummary(
            user_prompt="Identify the failing service.",
            expected_behavior="checkout-api is the failing service",
            required_concepts=["log-analysis"],
        ),
    )


@pytest.mark.asyncio
async def test_grade_campaign_assignment_semantically_records_passing_grade():
    context = G8eHttpContext(user_id="user-1", evaluation_context=_evaluation_context())
    settings = G8eeUserSettings(
        eval_judge=EvalJudgeSettings(eval_judge_model="judge-model", eval_judge_max_tokens=1024)
    )
    judge_grade = EvalGrade(
        score=4,
        reasoning="checkout-api is identified",
        passed=True,
        model_calls=[
            ModelCallTelemetry(
                agent_role="judge",
                provider="GeminiProvider",
                model="judge-model",
                monotonic_start=1.0,
                monotonic_end=2.0,
                provider_attempt_id="judge-attempt-1",
            )
        ],
    )
    with patch("app.services.evaluation.semantic_grader.get_llm_provider", return_value=object()), patch(
        "app.services.evaluation.semantic_grader.EvalJudge"
    ) as judge_cls:
        judge_cls.return_value.grade_turn = AsyncMock(return_value=judge_grade)
        semantic_grades, grader_calls = await grade_campaign_assignment_semantically(
            evaluation_context=context.evaluation_context,
            g8e_context=context,
            request_settings=settings,
            gold_summary=context.evaluation_context.gold_summary,
            designated_role_output="checkout-api failed",
            tool_calls=[
                EvaluationToolCallRecord(
                    call_id="call-1",
                    tool_name="file_read_on_operator",
                    success=True,
                )
            ],
        )

    assert len(semantic_grades) == 1
    assert semantic_grades[0].status == "pass"
    assert semantic_grades[0].score == 4
    assert len(grader_calls) == 1
    assert grader_calls[0].provider_attempt_id == "judge-attempt-1"
