# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Unit tests for decision provider typed models."""

from __future__ import annotations

import pytest
from pydantic import TypeAdapter

from app.decision.types import (
    Answer,
    ChoiceAnswer,
    ChoiceQuestion,
    EvaluateResponse,
    EvaluateUsage,
    NoulAnswer,
    NoulQuestion,
    Question,
    ScoreAnswer,
    ScoreQuestion,
)

pytestmark = pytest.mark.unit

_QUESTION_ADAPTER: TypeAdapter[Question] = TypeAdapter(Question)
_ANSWER_ADAPTER: TypeAdapter[Answer] = TypeAdapter(Answer)


class TestDecisionQuestionSerialization:
    def test_choice_question_round_trip(self):
        question = ChoiceQuestion(
            instructions="What is the issue about?",
            criteria={"billing": "money problems", "bug": "broken product"},
        )
        wire = question.model_dump(mode="json", exclude_none=True)
        restored = _QUESTION_ADAPTER.validate_python(wire)
        assert restored == question

    def test_score_question_round_trip(self):
        question = ScoreQuestion(
            instructions="How urgent is this?",
            criteria=["routine", "today", "urgent", "critical"],
        )
        wire = question.model_dump(mode="json", exclude_none=True)
        restored = _QUESTION_ADAPTER.validate_python(wire)
        assert restored == question

    def test_noul_question_round_trip(self):
        question = NoulQuestion(instructions="Escalate to a human now?")
        wire = question.model_dump(mode="json", exclude_none=True)
        restored = _QUESTION_ADAPTER.validate_python(wire)
        assert restored == question


class TestDecisionAnswerSerialization:
    def test_choice_answer_round_trip(self):
        answer = ChoiceAnswer(
            choice="billing",
            confidence=1.0,
            probabilities={"billing": 1.0, "bug": 0.0},
        )
        wire = answer.model_dump(mode="json", exclude_none=True)
        restored = _ANSWER_ADAPTER.validate_python(wire)
        assert restored == answer

    def test_score_answer_round_trip(self):
        answer = ScoreAnswer(
            score=3.0,
            confidence=0.92,
            probabilities={"0": 0.0, "3": 1.0},
            legend={"0": "routine", "3": "critical"},
        )
        wire = answer.model_dump(mode="json", exclude_none=True)
        restored = _ANSWER_ADAPTER.validate_python(wire)
        assert restored == answer

    def test_noul_answer_round_trip(self):
        answer = NoulAnswer(noul=0.8)
        wire = answer.model_dump(mode="json", exclude_none=True)
        restored = _ANSWER_ADAPTER.validate_python(wire)
        assert restored == answer


class TestEvaluateResponseSerialization:
    def test_evaluate_response_round_trip(self):
        response = EvaluateResponse(
            model="jev-latest",
            answers={
                "topic": ChoiceAnswer(
                    choice="billing",
                    confidence=1.0,
                    probabilities={"billing": 1.0},
                ),
                "escalate": NoulAnswer(noul=0.8),
            },
            usage=EvaluateUsage(input_tokens=100, output_tokens=20),
        )
        wire = response.model_dump(mode="json", exclude_none=True)
        restored = EvaluateResponse.model_validate(wire)
        assert restored == response
