# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Unit tests for EvalJudge via the Jev decision provider."""

from __future__ import annotations

import pytest

from app.decision.types import (
    EvaluateResponse,
    EvaluateUsage,
    NoulAnswer,
    ScoreAnswer,
)
from app.errors import RateLimitError
from app.services.ai.eval_judge import PASSING_THRESHOLD, EvalJudge, EvalJudgeError
from tests.fakes.fake_decision_provider import FakeDecisionProvider

pytestmark = pytest.mark.unit

GRADE_KWARGS = {
    "user_query": "Identify the failing service in the logs.",
    "interaction_trace": "Designated role output:\ncheckout-api failed",
    "expected_behavior": "checkout-api is the failing service",
    "required_concepts": ["log-analysis"],
    "expected_tools": ["file_read_on_operator"],
    "forbidden_tools": [],
}


def _jev_grade_response(
    *,
    score_index: float = 3.0,
    confidence: float = 0.91,
    noul: float = 0.82,
) -> EvaluateResponse:
    return EvaluateResponse(
        model="jev-latest",
        answers={
            "rubric_score": ScoreAnswer(
                score=score_index,
                confidence=confidence,
                probabilities={
                    "0": 0.01,
                    "1": 0.02,
                    "2": 0.03,
                    "3": 0.91,
                    "4": 0.03,
                },
            ),
            "meets_passing_threshold": NoulAnswer(noul=noul),
        },
        usage=EvaluateUsage(input_tokens=220, output_tokens=18),
    )


@pytest.fixture
def fake_decision_provider() -> FakeDecisionProvider:
    return FakeDecisionProvider()


@pytest.fixture
def jev_judge(fake_decision_provider: FakeDecisionProvider) -> EvalJudge:
    return EvalJudge(
        decision_provider=fake_decision_provider,
        model="jev-latest",
    )


class TestEvalJudgeJevConstruction:
    def test_requires_provider_or_decision_provider(self):
        with pytest.raises(EvalJudgeError, match="LLM provider or decision provider"):
            EvalJudge(provider=None, decision_provider=None, model="jev-latest")

    def test_rejects_both_providers(self, fake_decision_provider: FakeDecisionProvider):
        with pytest.raises(EvalJudgeError, match="not both"):
            EvalJudge(
                provider=object(),
                decision_provider=fake_decision_provider,
                model="jev-latest",
            )


@pytest.mark.asyncio
class TestEvalJudgeJevHappyPath:
    async def test_maps_score_answer_to_eval_grade(
        self,
        jev_judge: EvalJudge,
        fake_decision_provider: FakeDecisionProvider,
    ):
        fake_decision_provider.add_response(_jev_grade_response(score_index=3.0))

        result = await jev_judge.grade_turn(**GRADE_KWARGS)

        assert result.score == 4
        assert result.passed is True
        assert result.score >= PASSING_THRESHOLD
        assert "Jev rubric score: 4/5" in result.reasoning
        assert "Passing-threshold noul: 0.82" in result.reasoning
        assert len(result.model_calls) == 1
        assert result.model_calls[0].agent_role == "judge"
        assert result.model_calls[0].succeeded is True

    async def test_low_score_fails(self, jev_judge: EvalJudge, fake_decision_provider: FakeDecisionProvider):
        fake_decision_provider.add_response(_jev_grade_response(score_index=1.0))

        result = await jev_judge.grade_turn(**GRADE_KWARGS)

        assert result.score == 2
        assert result.passed is False

    async def test_passed_is_deterministic_from_rubric_score(
        self,
        jev_judge: EvalJudge,
        fake_decision_provider: FakeDecisionProvider,
    ):
        fake_decision_provider.add_response(_jev_grade_response(score_index=2.0, noul=0.99))

        result = await jev_judge.grade_turn(**GRADE_KWARGS)

        assert result.score == PASSING_THRESHOLD
        assert result.passed is True

    async def test_state_includes_rubric_context(
        self,
        jev_judge: EvalJudge,
        fake_decision_provider: FakeDecisionProvider,
    ):
        fake_decision_provider.add_response(_jev_grade_response())

        await jev_judge.grade_turn(**GRADE_KWARGS)

        assert fake_decision_provider.last_request is not None
        state = fake_decision_provider.last_request["state"]
        assert isinstance(state, str)
        assert "Identify the failing service in the logs." in state
        assert "checkout-api is the failing service" in state
        assert "file_read_on_operator" in state
        questions = fake_decision_provider.last_request["questions"]
        assert "rubric_score" in questions
        assert "meets_passing_threshold" in questions


@pytest.mark.asyncio
class TestEvalJudgeJevErrorPaths:
    async def test_missing_score_answer_raises(
        self,
        jev_judge: EvalJudge,
        fake_decision_provider: FakeDecisionProvider,
    ):
        fake_decision_provider.add_response(
            EvaluateResponse(
                model="jev-latest",
                answers={"meets_passing_threshold": NoulAnswer(noul=0.5)},
                usage=EvaluateUsage(input_tokens=10, output_tokens=5),
            )
        )

        with pytest.raises(EvalJudgeError, match="missing rubric_score"):
            await jev_judge.grade_turn(**GRADE_KWARGS)

    async def test_out_of_range_score_raises(
        self,
        jev_judge: EvalJudge,
        fake_decision_provider: FakeDecisionProvider,
    ):
        fake_decision_provider.add_response(_jev_grade_response(score_index=9.0))

        with pytest.raises(EvalJudgeError, match="out-of-range score index"):
            await jev_judge.grade_turn(**GRADE_KWARGS)

    async def test_provider_failure_raises_eval_judge_error(
        self,
        jev_judge: EvalJudge,
        fake_decision_provider: FakeDecisionProvider,
    ):
        fake_decision_provider.responses.clear()

        with pytest.raises(EvalJudgeError, match="after 3 attempt"):
            await jev_judge.grade_turn(**GRADE_KWARGS)

    async def test_retries_on_rate_limit(
        self,
        jev_judge: EvalJudge,
        fake_decision_provider: FakeDecisionProvider,
    ):
        call_count = 0

        async def flaky_evaluate(**_kwargs):
            nonlocal call_count
            call_count += 1
            if call_count == 1:
                raise RateLimitError("Jev rate limit exceeded.", component="typesafe")
            return _jev_grade_response()

        fake_decision_provider.evaluate = flaky_evaluate  # type: ignore[method-assign]

        result = await jev_judge.grade_turn(**GRADE_KWARGS)

        assert result.score == 4
        assert call_count == 2
        assert len(result.model_calls) == 2
        assert result.model_calls[0].succeeded is False
        assert result.model_calls[1].succeeded is True
