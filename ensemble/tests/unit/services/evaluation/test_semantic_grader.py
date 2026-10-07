# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from unittest.mock import AsyncMock, MagicMock, patch

import pytest
from g8e.models.internal_api import (
    EvaluationGoldSummary,
    EvaluationInferenceContext,
    InferenceModelVariant,
)

from app.constants import JEV_DEFAULT_MODEL, LLMProvider
from app.llm.llm_types import GenerateContentResponse
from app.models.evaluation_trace import EvaluationToolCallRecord
from app.models.http_context import G8eHttpContext
from app.models.model_telemetry import ModelCallTelemetry
from app.models.settings import EvalJudgeSettings, G8eeUserSettings, LLMSettings
from app.services.ai.eval_judge import EvalGrade, EvalJudgeError
from app.services.evaluation.semantic_grader import (
    _resolve_eval_judge_model,
    grade_campaign_assignment_semantically,
)


def _gold_summary() -> EvaluationGoldSummary:
    return EvaluationGoldSummary(
        user_prompt="Identify the failing service.",
        expected_behavior="checkout-api is the failing service",
        required_concepts=["log-analysis"],
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
        gold_summary=_gold_summary(),
    )


@pytest.mark.asyncio
async def test_grade_campaign_assignment_semantically_records_passing_grade():
    evaluation_context = _evaluation_context()
    context = G8eHttpContext(user_id="user-1", evaluation_context=evaluation_context)
    settings = G8eeUserSettings(eval_judge=EvalJudgeSettings(eval_judge_model="judge-model"))
    judge_grade = EvalGrade(
        score=4,
        reasoning="checkout-api is identified",
        passed=True,
        model_calls=[
            ModelCallTelemetry(
                agent_role="judge",
                classification="grader",
                provider="GeminiProvider",
                model="judge-model",
                monotonic_start=1.0,
                monotonic_end=2.0,
                provider_attempt_id="judge-attempt-1",
            )
        ],
    )
    with (
        patch("app.services.evaluation.semantic_grader.get_llm_provider", return_value=object()),
        patch("app.services.evaluation.semantic_grader.EvalJudge") as judge_cls,
    ):
        judge_cls.return_value.grade_turn = AsyncMock(return_value=judge_grade)
        semantic_grades, grader_calls = await grade_campaign_assignment_semantically(
            evaluation_context=evaluation_context,
            g8e_context=context,
            judge_settings=settings,
            gold_summary=_gold_summary(),
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


def test_resolve_eval_judge_model_prefers_explicit_setting():
    settings = G8eeUserSettings(
        llm=LLMSettings(llm_lite_model="qwen3:0.6b"),
        eval_judge=EvalJudgeSettings(eval_judge_model="judge-model"),
    )
    assert _resolve_eval_judge_model(settings) == "judge-model"


def test_resolve_eval_judge_model_falls_back_to_lite_model():
    settings = G8eeUserSettings(llm=LLMSettings(llm_lite_model="qwen3:0.6b"))
    assert _resolve_eval_judge_model(settings) == "qwen3:0.6b"


@pytest.mark.asyncio
async def test_grade_campaign_assignment_semantically_uses_lite_model_fallback():
    evaluation_context = _evaluation_context()
    context = G8eHttpContext(user_id="user-1", evaluation_context=evaluation_context)
    settings = G8eeUserSettings(
        llm=LLMSettings(llm_lite_provider=LLMProvider.OLLAMA, llm_lite_model="qwen3:0.6b")
    )
    judge_grade = EvalGrade(score=3, reasoning="partial handoff", passed=False, model_calls=[])
    with (
        patch(
            "app.services.evaluation.semantic_grader.get_llm_provider", return_value=object()
        ) as provider_fn,
        patch("app.services.evaluation.semantic_grader.EvalJudge") as judge_cls,
    ):
        judge_cls.return_value.grade_turn = AsyncMock(return_value=judge_grade)
        semantic_grades, _ = await grade_campaign_assignment_semantically(
            evaluation_context=evaluation_context,
            g8e_context=context,
            judge_settings=settings,
            gold_summary=_gold_summary(),
            designated_role_output="delegating to assistant",
            tool_calls=[],
        )

    provider_fn.assert_called_once()
    assert provider_fn.call_args.kwargs.get("is_lite") is True
    judge_cls.assert_called_once()
    assert judge_cls.call_args.kwargs["model"] == "qwen3:0.6b"
    assert semantic_grades[0].judge_variant_id == "qwen3:0.6b"


@pytest.mark.asyncio
async def test_grade_campaign_assignment_semantically_uses_jev_decision_provider():
    evaluation_context = _evaluation_context()
    context = G8eHttpContext(user_id="user-1", evaluation_context=evaluation_context)
    settings = G8eeUserSettings(
        llm=LLMSettings(
            llm_lite_provider=LLMProvider.JEV,
            llm_lite_model=JEV_DEFAULT_MODEL,
        ),
        eval_judge=EvalJudgeSettings(eval_judge_model=JEV_DEFAULT_MODEL),
    )
    judge_grade = EvalGrade(
        score=4,
        reasoning="Jev rubric score: 4/5 (index 3.00, confidence 0.91).",
        passed=True,
        model_calls=[],
    )
    with (
        patch(
            "app.services.evaluation.semantic_grader.get_decision_provider",
            return_value=object(),
        ) as decision_fn,
        patch("app.services.evaluation.semantic_grader.get_llm_provider") as llm_fn,
        patch("app.services.evaluation.semantic_grader.EvalJudge") as judge_cls,
    ):
        judge_cls.return_value.grade_turn = AsyncMock(return_value=judge_grade)
        semantic_grades, _ = await grade_campaign_assignment_semantically(
            evaluation_context=evaluation_context,
            g8e_context=context,
            judge_settings=settings,
            gold_summary=_gold_summary(),
            designated_role_output="checkout-api failed",
            tool_calls=[],
        )

    decision_fn.assert_called_once_with(settings.llm)
    llm_fn.assert_not_called()
    judge_cls.assert_called_once()
    assert judge_cls.call_args.kwargs["decision_provider"] is not None
    assert judge_cls.call_args.kwargs.get("provider") is None
    assert semantic_grades[0].judge_variant_id == JEV_DEFAULT_MODEL


@pytest.mark.asyncio
async def test_empty_judge_response_is_unavailable_and_sends_no_output_cap():
    """Simulates the provider returning no candidates (the 2026-10-02 failure shape).

    Uses the real EvalJudge so the whole path runs: the judge call must carry no
    invented output limit, and an empty response must surface as an explicit
    `unavailable` grade, never a silent pass, fail, or zero score.
    """
    evaluation_context = _evaluation_context()
    context = G8eHttpContext(user_id="user-1", evaluation_context=evaluation_context)
    settings = G8eeUserSettings(
        llm=LLMSettings(llm_lite_provider=LLMProvider.OLLAMA, llm_lite_model="qwen3:0.6b"),
    )
    provider = MagicMock()
    provider.generate_content_lite = AsyncMock(return_value=GenerateContentResponse(candidates=[]))

    with patch("app.services.evaluation.semantic_grader.get_llm_provider", return_value=provider):
        semantic_grades, grader_calls = await grade_campaign_assignment_semantically(
            evaluation_context=evaluation_context,
            g8e_context=context,
            judge_settings=settings,
            gold_summary=_gold_summary(),
            designated_role_output="checkout-api failed",
            tool_calls=[],
        )

    sent = provider.generate_content_lite.call_args.kwargs["lite_llm_settings"]
    assert sent.max_output_tokens is None
    assert len(semantic_grades) == 1
    assert semantic_grades[0].status == "unavailable"
    assert semantic_grades[0].score is None
    assert "empty response" in semantic_grades[0].detail
    assert len(grader_calls) == 1
    assert grader_calls[0].judge_variant_id == "qwen3:0.6b"
    assert provider.generate_content_lite.await_count == 1


@pytest.mark.asyncio
async def test_grade_campaign_assignment_semantically_returns_unavailable_when_judge_missing():
    evaluation_context = _evaluation_context()
    context = G8eHttpContext(user_id="user-1", evaluation_context=evaluation_context)
    settings = G8eeUserSettings()
    semantic_grades, grader_calls = await grade_campaign_assignment_semantically(
        evaluation_context=evaluation_context,
        g8e_context=context,
        judge_settings=settings,
        gold_summary=_gold_summary(),
        designated_role_output="delegating to assistant",
        tool_calls=[],
    )

    assert grader_calls == []
    assert len(semantic_grades) == 1
    assert semantic_grades[0].status == "unavailable"
    assert semantic_grades[0].detail


@pytest.mark.asyncio
async def test_unavailable_grade_preserves_failed_judge_attempt_telemetry():
    evaluation_context = _evaluation_context()
    context = G8eHttpContext(user_id="user-1", evaluation_context=evaluation_context)
    settings = G8eeUserSettings(eval_judge=EvalJudgeSettings(eval_judge_model="judge-model"))
    failed_call = ModelCallTelemetry(
        agent_role="judge",
        classification="grader",
        provider="GeminiProvider",
        model="judge-model",
        monotonic_start=1.0,
        monotonic_end=2.0,
        provider_attempt_id="failed-judge-attempt",
    )
    with (
        patch("app.services.evaluation.semantic_grader.get_llm_provider", return_value=object()),
        patch("app.services.evaluation.semantic_grader.EvalJudge") as judge_cls,
    ):
        judge_cls.return_value.grade_turn = AsyncMock(
            side_effect=EvalJudgeError("judge provider failed", model_calls=[failed_call])
        )
        semantic_grades, grader_calls = await grade_campaign_assignment_semantically(
            evaluation_context=evaluation_context,
            g8e_context=context,
            judge_settings=settings,
            gold_summary=_gold_summary(),
            designated_role_output="checkout-api failed",
            tool_calls=[],
        )

    assert semantic_grades[0].status == "unavailable"
    assert len(grader_calls) == 1
    assert grader_calls[0].provider_attempt_id == "failed-judge-attempt"


@pytest.mark.asyncio
async def test_judge_is_called_with_grader_context_without_evaluation_attempt_id():
    """Verify that judge calls are NOT attributed to the scored turn's evaluation_attempt_id.

    W2 requirement: Grader context must be distinct from the scored chain.
    The judge should not set the judged assignment's evaluation_attempt_id
    as a scored-chain member, so grader calls are excluded from scored aggregates.
    """
    evaluation_context = _evaluation_context()
    context = G8eHttpContext(user_id="user-1", evaluation_context=evaluation_context)
    settings = G8eeUserSettings(eval_judge=EvalJudgeSettings(eval_judge_model="judge-model"))
    judge_grade = EvalGrade(
        score=4,
        reasoning="checkout-api is identified",
        passed=True,
        model_calls=[],
    )
    with (
        patch("app.services.evaluation.semantic_grader.get_llm_provider", return_value=object()),
        patch("app.services.evaluation.semantic_grader.EvalJudge") as judge_cls,
    ):
        judge_cls.return_value.grade_turn = AsyncMock(return_value=judge_grade)
        await grade_campaign_assignment_semantically(
            evaluation_context=evaluation_context,
            g8e_context=context,
            judge_settings=settings,
            gold_summary=_gold_summary(),
            designated_role_output="checkout-api failed",
            tool_calls=[],
        )

    # Verify the judge was instantiated with a grader context
    judge_instantiation_context = judge_cls.call_args.kwargs["g8e_context"]
    assert judge_instantiation_context is not None
    assert judge_instantiation_context.evaluation_context is None, (
        "Grader context must not carry evaluation_context (which includes evaluation_attempt_id)"
    )
    assert judge_instantiation_context.user_id == context.user_id
    assert judge_instantiation_context.operator_id == context.operator_id
