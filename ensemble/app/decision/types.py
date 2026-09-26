# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

from typing import Annotated, Any, Literal

from pydantic import Discriminator, Tag

from app.models.base import Field, G8eBaseModel

DecisionState = str | dict[str, Any] | list[Any]


class ChoiceQuestion(G8eBaseModel):
    type: Literal["choice"] = "choice"
    instructions: str
    criteria: dict[str, str]


class ScoreQuestion(G8eBaseModel):
    type: Literal["score"] = "score"
    instructions: str
    criteria: list[str]


class NoulQuestion(G8eBaseModel):
    type: Literal["noul"] = "noul"
    instructions: str


def _question_discriminator(value: Any) -> str:
    if isinstance(value, G8eBaseModel):
        return value.type
    if isinstance(value, dict):
        question_type = value.get("type")
        if isinstance(question_type, str):
            return question_type
    raise TypeError(f"Unsupported question payload: {type(value).__name__}")


Question = Annotated[
    Annotated[ChoiceQuestion, Tag("choice")]
    | Annotated[ScoreQuestion, Tag("score")]
    | Annotated[NoulQuestion, Tag("noul")],
    Discriminator(_question_discriminator),
]


class ChoiceAnswer(G8eBaseModel):
    type: Literal["choice"] = "choice"
    choice: str
    confidence: float
    probabilities: dict[str, float]


class ScoreAnswer(G8eBaseModel):
    type: Literal["score"] = "score"
    score: float
    confidence: float
    probabilities: dict[str, float]
    legend: dict[str, str] | None = None


class NoulAnswer(G8eBaseModel):
    type: Literal["noul"] = "noul"
    noul: float


def _answer_discriminator(value: Any) -> str:
    if isinstance(value, G8eBaseModel):
        return value.type
    if isinstance(value, dict):
        answer_type = value.get("type")
        if isinstance(answer_type, str):
            return answer_type
    raise TypeError(f"Unsupported answer payload: {type(value).__name__}")


Answer = Annotated[
    Annotated[ChoiceAnswer, Tag("choice")]
    | Annotated[ScoreAnswer, Tag("score")]
    | Annotated[NoulAnswer, Tag("noul")],
    Discriminator(_answer_discriminator),
]


class EvaluateUsage(G8eBaseModel):
    input_tokens: int = Field(ge=0)
    output_tokens: int = Field(ge=0)


class EvaluateResponse(G8eBaseModel):
    model: str
    answers: dict[str, Answer]
    usage: EvaluateUsage
