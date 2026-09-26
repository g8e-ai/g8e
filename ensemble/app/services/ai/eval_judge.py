# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
AI Agent Accuracy Evaluation Judge.

Uses a high-capability LLM (Primary Model) to grade the performance of an
agent turn against a gold standard reference.

Error contract:
  - Transient LLM failures (rate limits, 503s) are retried with exponential
    backoff (3 attempts, 2s initial delay, 2x multiplier).
  - Infrastructure errors that survive all retries raise EvalJudgeError
    so the caller sees a clear system failure, never a fake grade.
  - Only a valid LLM evaluation produces an EvalGrade.
"""

import asyncio
import json
import logging
import re
import time
from typing import Any

from app.models.base import BaseModel, Field, field_validator

from app.decision.provider import DecisionProvider
from app.decision.types import EvaluateResponse, NoulAnswer, NoulQuestion, ScoreAnswer, ScoreQuestion
from app.llm.llm_types import Content, GenerateContentResponse, Part, ResponseFormat, Role, LiteLLMSettings, UsageMetadata
from app.llm.model_evidence import model_boundary_hash
from app.llm.model_call_attribution import build_model_call_telemetry, prepare_provider_call
from app.models.http_context import G8eHttpContext
from app.llm.provider import LLMProvider as LLMProviderBase
from app.models.model_telemetry import ModelCallTelemetry
from app.models.settings import EvalJudgeSettings
from app.utils.agent_persona_loader import get_agent_persona
from app.errors import OllamaEmptyResponseError, RateLimitError
from app.models.model_configs import get_model_config

logger = logging.getLogger(__name__)

# Judge template scaffolding - holds the response format structure
# separate from the persona voice in agents.json

JUDGE_EVALUATION_TEMPLATE = """\
<user_query>
{user_query}
</user_query>

<gold_standard_criteria>
Expected Behavior: {expected_behavior}
Required Concepts: {required_concepts}
Expected Tools: {expected_tools}
Forbidden Tools: {forbidden_tools}
</gold_standard_criteria>

<student_interaction>
{interaction_trace}
</student_interaction>

<response_format>
Respond with ONLY a JSON object matching this exact schema, with no prose, no markdown fences, and no additional fields:
{{
  "score": integer (1-5),
  "reasoning": "string explaining why this score was assigned"
}}
</response_format>
"""

JEV_EVALUATION_STATE_TEMPLATE = """\
<user_query>
{user_query}
</user_query>

<gold_standard_criteria>
Expected Behavior: {expected_behavior}
Required Concepts: {required_concepts}
Expected Tools: {expected_tools}
Forbidden Tools: {forbidden_tools}
</gold_standard_criteria>

<student_interaction>
{interaction_trace}
</student_interaction>
"""

JEV_RUBRIC_CRITERIA = ["1", "2", "3", "4", "5"]

PASSING_THRESHOLD = 3
_MAX_RETRIES = 3
_INITIAL_RETRY_DELAY_SECONDS = 2.0
_RETRY_BACKOFF_MULTIPLIER = 2.0

_RETRYABLE_PATTERNS = (
    "429",
    "503",
    "rate limit",
    "too many requests",
    "service unavailable",
    "resource exhausted",
    "overloaded",
    "quota",
    "temporarily unavailable",
)


def _is_retryable(exc: BaseException) -> bool:
    """Return True for transient LLM errors that warrant a retry."""
    if isinstance(exc, RateLimitError):
        return True
    code = getattr(exc, "code", None) or getattr(exc, "status_code", None)
    if code in (429, 503):
        return True
    msg = str(exc).lower()
    return any(pattern in msg for pattern in _RETRYABLE_PATTERNS)


class EvalJudgeError(Exception):
    """Raised when the judge cannot produce a valid grade due to a system error.

    This is distinct from a low score - it means the evaluation itself failed
    (LLM unreachable, invalid response after retries, etc.).
    """

    def __init__(self, message: str, model_calls: list[ModelCallTelemetry] | None = None):
        super().__init__(message)
        self.model_calls = model_calls or []


class EvalGrade(BaseModel):
    """Result of an evaluation grade."""

    score: int = Field(..., ge=1, le=5, description="Score from 1 to 5")
    reasoning: str = Field(..., description="Detailed explanation for the score")
    passed: bool = Field(..., description="Whether the score meets the passing threshold")
    model_calls: list[ModelCallTelemetry] = Field(default_factory=list)

    @field_validator("reasoning", mode="before")
    @classmethod
    def _reasoning_must_be_nonempty(cls, v: str) -> str:
        if not v or not v.strip():
            raise ValueError("reasoning must not be empty")
        return v


_JUDGE_RESPONSE_SCHEMA: dict[str, Any] = {
    "type": "object",
    "properties": {
        "score": {"type": "integer", "minimum": 1, "maximum": 5},
        "reasoning": {"type": "string"},
    },
    "required": ["score", "reasoning"],
}

_JSON_FENCE_RE = re.compile(r"```(?:json)?\s*(.*?)\s*```", re.DOTALL)


def _extract_json(text: str) -> dict[str, Any]:
    """Extract a JSON object from LLM text, handling markdown fences."""
    stripped = text.strip()
    fence_match = _JSON_FENCE_RE.search(stripped)
    if fence_match:
        stripped = fence_match.group(1).strip()
    return json.loads(stripped)


class EvalJudge:
    """Judge that evaluates agent accuracy via an LLM or Jev decision provider.

    Construction requires either a concrete LLM provider instance or a
    DecisionProvider (when lite_provider is Jev), plus an explicit model
    name or EvalJudgeSettings. The LLM provider is the abstract LLMProvider
    from ``app.llm.provider``, not the LLMProvider enum from ``app.constants``.
    """

    def __init__(
        self,
        provider: LLMProviderBase | None = None,
        decision_provider: DecisionProvider | None = None,
        model: str | None = None,
        settings: EvalJudgeSettings | None = None,
        g8e_context: G8eHttpContext | None = None,
    ):
        if provider is None and decision_provider is None:
            raise EvalJudgeError(
                "EvalJudge requires a configured LLM provider or decision provider instance"
            )
        if provider is not None and decision_provider is not None:
            raise EvalJudgeError(
                "EvalJudge accepts either an LLM provider or a decision provider, not both"
            )

        self._provider = provider
        self._decision_provider = decision_provider
        self._g8e_context = g8e_context
        self._settings = settings or EvalJudgeSettings(
            eval_judge_model=None,
            eval_judge_max_tokens=4096,
        )
        self._model = model or self._settings.model

        if not self._model:
            raise EvalJudgeError(
                "EvalJudge requires an explicit model name (via model parameter or settings.model)"
            )

    async def grade_turn(
        self,
        user_query: str,
        interaction_trace: str,
        expected_behavior: str,
        required_concepts: list[str],
        expected_tools: list[str] | None = None,
        forbidden_tools: list[str] | None = None,
    ) -> EvalGrade:
        """Grade a single agent interaction turn.

        Retries transient LLM failures with exponential backoff.

        Raises:
            EvalJudgeError: When the judge cannot produce a valid grade after
                all retry attempts (infrastructure failure, not a low score).
        """
        if self._decision_provider is not None:
            return await self._grade_with_jev(
                user_query=user_query,
                interaction_trace=interaction_trace,
                expected_behavior=expected_behavior,
                required_concepts=required_concepts,
                expected_tools=expected_tools,
                forbidden_tools=forbidden_tools,
            )

        persona = get_agent_persona("judge")
        template = JUDGE_EVALUATION_TEMPLATE.format(
            user_query=user_query,
            expected_behavior=expected_behavior,
            required_concepts=", ".join(required_concepts),
            expected_tools=", ".join(expected_tools or []),
            forbidden_tools=", ".join(forbidden_tools or []),
            interaction_trace=interaction_trace,
        )
        prompt = f"{persona.get_system_prompt()}{template}"

        model_config = get_model_config(self._model)
        settings = LiteLLMSettings(
            max_output_tokens=self._settings.max_output_tokens,
            top_p_nucleus_sampling=model_config.top_p,
            top_k_filtering=model_config.top_k,
            stop_sequences=model_config.stop_sequences,
            system_instructions="",
            response_format=ResponseFormat.from_pydantic_schema(
                _JUDGE_RESPONSE_SCHEMA,
                name="EvalGradeResponse",
            ),
        )

        contents = [Content(role=Role.USER, parts=[Part.from_text(prompt)])]
        last_error: Exception | None = None
        model_calls: list[ModelCallTelemetry] = []
        delay = _INITIAL_RETRY_DELAY_SECONDS

        for attempt in range(_MAX_RETRIES):
            try:
                return await self._call_and_parse(contents, settings, attempt, model_calls)
            except EvalJudgeError:
                raise
            except Exception as exc:
                last_error = exc
                if _is_retryable(exc) and attempt < _MAX_RETRIES - 1:
                    logger.warning(
                        "EvalJudge transient failure (attempt %d/%d): %s",
                        attempt + 1,
                        _MAX_RETRIES,
                        exc,
                    )
                    await asyncio.sleep(delay)
                    delay *= _RETRY_BACKOFF_MULTIPLIER
                    continue
                break

        logger.error("EvalJudge failed after %d attempt(s)", _MAX_RETRIES, exc_info=True)
        raise EvalJudgeError(
            f"Judge could not produce a valid grade after {_MAX_RETRIES} attempt(s): {last_error}",
            model_calls=model_calls,
        ) from last_error

    async def _call_and_parse(
        self,
        contents: list[Content],
        settings: LiteLLMSettings,
        retry_count: int,
        model_calls: list[ModelCallTelemetry],
    ) -> EvalGrade:
        """Make the LLM call and parse the response into an EvalGrade."""
        if not self._model:
            raise EvalJudgeError("Model is not set", model_calls=model_calls)
        prepare_provider_call(
            self._provider,
            g8e_context=self._g8e_context,
            retry_count=retry_count,
        )
        input_artifact_hash = model_boundary_hash({
            "model": self._model,
            "contents": contents,
            "settings": settings,
        })
        monotonic_start = time.monotonic()
        try:
            response = await self._provider.generate_content_lite(
                model=self._model,
                contents=contents,
                lite_llm_settings=settings,
            )
        except Exception as exc:
            model_calls.append(self._model_call_telemetry(
                response=None,
                response_text="",
                monotonic_start=monotonic_start,
                retry_count=retry_count,
                input_artifact_hash=input_artifact_hash,
                error=exc,
            ))
            raise

        response_text = ""
        try:
            response_text = response.text or ""
            if not response_text:
                raise EvalJudgeError("Judge LLM returned an empty response")
            data = _extract_json(response_text)
            score = data.get("score")
            reasoning = data.get("reasoning")
            if score is None or reasoning is None:
                raise EvalJudgeError(f"Judge response missing required fields: {data}")
            if not isinstance(score, int) or score < 1 or score > 5:
                raise EvalJudgeError(f"Judge returned out-of-range score: {score}")
            telemetry = self._model_call_telemetry(
                response=response,
                response_text=response_text,
                monotonic_start=monotonic_start,
                retry_count=retry_count,
                input_artifact_hash=input_artifact_hash,
            )
            model_calls.append(telemetry)
            return EvalGrade(
                score=score,
                reasoning=reasoning,
                passed=score >= PASSING_THRESHOLD,
                model_calls=model_calls,
            )
        except OllamaEmptyResponseError as exc:
            error = EvalJudgeError(f"Judge LLM returned an empty response: {exc}")
        except json.JSONDecodeError as exc:
            error = EvalJudgeError(f"Judge LLM returned invalid JSON: {response_text[:200]}")
            error.__cause__ = exc
        except EvalJudgeError as exc:
            error = exc
        except Exception as exc:
            error = EvalJudgeError(str(exc))
            error.__cause__ = exc
        model_calls.append(self._model_call_telemetry(
            response=response,
            response_text=response_text,
            monotonic_start=monotonic_start,
            retry_count=retry_count,
            input_artifact_hash=input_artifact_hash,
            error=error,
        ))
        error.model_calls = list(model_calls)
        raise error

    async def _grade_with_jev(
        self,
        *,
        user_query: str,
        interaction_trace: str,
        expected_behavior: str,
        required_concepts: list[str],
        expected_tools: list[str] | None = None,
        forbidden_tools: list[str] | None = None,
    ) -> EvalGrade:
        """Grade via a batched Jev System One evaluate call."""
        if self._decision_provider is None:
            raise EvalJudgeError("Jev grading requested without a decision provider")
        if not self._model:
            raise EvalJudgeError("Model is not set")

        state = self._build_jev_state(
            user_query=user_query,
            interaction_trace=interaction_trace,
            expected_behavior=expected_behavior,
            required_concepts=required_concepts,
            expected_tools=expected_tools,
            forbidden_tools=forbidden_tools,
        )
        questions = self._build_jev_questions()

        last_error: Exception | None = None
        model_calls: list[ModelCallTelemetry] = []
        delay = _INITIAL_RETRY_DELAY_SECONDS

        for attempt in range(_MAX_RETRIES):
            try:
                return await self._evaluate_and_parse_jev(
                    state=state,
                    questions=questions,
                    retry_count=attempt,
                    model_calls=model_calls,
                )
            except EvalJudgeError:
                raise
            except Exception as exc:
                last_error = exc
                if _is_retryable(exc) and attempt < _MAX_RETRIES - 1:
                    logger.warning(
                        "EvalJudge Jev transient failure (attempt %d/%d): %s",
                        attempt + 1,
                        _MAX_RETRIES,
                        exc,
                    )
                    await asyncio.sleep(delay)
                    delay *= _RETRY_BACKOFF_MULTIPLIER
                    continue
                break

        logger.error("EvalJudge Jev failed after %d attempt(s)", _MAX_RETRIES, exc_info=True)
        raise EvalJudgeError(
            f"Judge could not produce a valid grade after {_MAX_RETRIES} attempt(s): {last_error}",
            model_calls=model_calls,
        ) from last_error

    async def _evaluate_and_parse_jev(
        self,
        *,
        state: str,
        questions: dict[str, ScoreQuestion | NoulQuestion],
        retry_count: int,
        model_calls: list[ModelCallTelemetry],
    ) -> EvalGrade:
        """Make the Jev evaluate call and parse the response into an EvalGrade."""
        provider = self._decision_provider
        if provider is None or not self._model:
            raise EvalJudgeError("Jev judge is not configured", model_calls=model_calls)

        provider.clear_input_artifact_hash()
        input_artifact_hash = model_boundary_hash({
            "model": self._model,
            "state": state,
            "questions": {
                name: question.model_dump(mode="json", exclude_none=True)
                for name, question in questions.items()
            },
        })
        monotonic_start = time.monotonic()

        try:
            response = await provider.evaluate(
                model=self._model,
                state=state,
                questions=questions,
            )
        except Exception as exc:
            model_calls.append(self._jev_model_call_telemetry(
                response=None,
                monotonic_start=monotonic_start,
                retry_count=retry_count,
                input_artifact_hash=input_artifact_hash,
                error=exc,
            ))
            raise

        try:
            grade = self._map_jev_response_to_grade(response)
            monotonic_end = time.monotonic()
            model_calls.append(self._jev_model_call_telemetry(
                response=response,
                monotonic_start=monotonic_start,
                monotonic_end=monotonic_end,
                retry_count=retry_count,
                input_artifact_hash=input_artifact_hash,
            ))
            return grade.model_copy(update={"model_calls": model_calls})
        except EvalJudgeError as exc:
            model_calls.append(self._jev_model_call_telemetry(
                response=response,
                monotonic_start=monotonic_start,
                retry_count=retry_count,
                input_artifact_hash=input_artifact_hash,
                error=exc,
            ))
            exc.model_calls = list(model_calls)
            raise

    def _build_jev_state(
        self,
        *,
        user_query: str,
        interaction_trace: str,
        expected_behavior: str,
        required_concepts: list[str],
        expected_tools: list[str] | None,
        forbidden_tools: list[str] | None,
    ) -> str:
        persona = get_agent_persona("judge")
        template = JEV_EVALUATION_STATE_TEMPLATE.format(
            user_query=user_query,
            expected_behavior=expected_behavior,
            required_concepts=", ".join(required_concepts),
            expected_tools=", ".join(expected_tools or []),
            forbidden_tools=", ".join(forbidden_tools or []),
            interaction_trace=interaction_trace,
        )
        return f"{persona.get_system_prompt()}{template}"

    @staticmethod
    def _build_jev_questions() -> dict[str, ScoreQuestion | NoulQuestion]:
        return {
            "rubric_score": ScoreQuestion(
                instructions=(
                    "Grade agent performance on a 1-5 rubric. "
                    "1=unacceptable, 2=below expectations, 3=meets minimum bar, "
                    "4=good, 5=excellent. Base the score strictly on the rubric and evidence."
                ),
                criteria=JEV_RUBRIC_CRITERIA,
            ),
            "meets_passing_threshold": NoulQuestion(
                instructions=(
                    f"Does the agent performance meet or exceed the passing threshold "
                    f"(rubric score >= {PASSING_THRESHOLD})?"
                ),
            ),
        }

    def _map_jev_response_to_grade(self, response: EvaluateResponse) -> EvalGrade:
        score_answer = response.answers.get("rubric_score")
        noul_answer = response.answers.get("meets_passing_threshold")

        if not isinstance(score_answer, ScoreAnswer):
            raise EvalJudgeError("Jev judge response missing rubric_score answer")
        if noul_answer is not None and not isinstance(noul_answer, NoulAnswer):
            raise EvalJudgeError("Jev judge response has malformed meets_passing_threshold answer")

        score = self._jev_score_to_rubric(score_answer)
        reasoning = self._format_jev_reasoning(score_answer, noul_answer)
        return EvalGrade(
            score=score,
            reasoning=reasoning,
            passed=score >= PASSING_THRESHOLD,
        )

    @staticmethod
    def _jev_score_to_rubric(score_answer: ScoreAnswer) -> int:
        index = round(score_answer.score)
        if index < 0 or index >= len(JEV_RUBRIC_CRITERIA):
            raise EvalJudgeError(
                f"Jev returned out-of-range score index: {score_answer.score}"
            )
        rubric_score = int(JEV_RUBRIC_CRITERIA[index])
        if rubric_score < 1 or rubric_score > 5:
            raise EvalJudgeError(f"Jev mapped to out-of-range rubric score: {rubric_score}")
        return rubric_score

    @staticmethod
    def _format_jev_reasoning(
        score_answer: ScoreAnswer,
        noul_answer: NoulAnswer | None,
    ) -> str:
        index = round(score_answer.score)
        selected = (
            JEV_RUBRIC_CRITERIA[index]
            if 0 <= index < len(JEV_RUBRIC_CRITERIA)
            else str(score_answer.score)
        )
        parts = [
            (
                f"Jev rubric score: {selected}/5 "
                f"(index {score_answer.score:.2f}, confidence {score_answer.confidence:.2f})"
            ),
        ]
        if score_answer.probabilities:
            distribution = ", ".join(
                f"{key}={value:.2f}"
                for key, value in sorted(
                    score_answer.probabilities.items(),
                    key=lambda item: int(item[0]) if item[0].isdigit() else 0,
                )
            )
            parts.append(f"Score distribution: {distribution}")
        if noul_answer is not None:
            parts.append(f"Passing-threshold noul: {noul_answer.noul:.2f}")
        return ". ".join(parts) + "."

    def _jev_model_call_telemetry(
        self,
        *,
        response: EvaluateResponse | None,
        monotonic_start: float,
        monotonic_end: float | None = None,
        retry_count: int,
        input_artifact_hash: str,
        error: Exception | None = None,
    ) -> ModelCallTelemetry:
        provider = self._decision_provider
        if provider is None or not self._model:
            raise EvalJudgeError("Jev judge is not configured")
        usage = response.usage if response else None
        output_hash = (
            model_boundary_hash(response.model_dump(mode="json", exclude_none=True))
            if response
            else ""
        )
        return build_model_call_telemetry(
            provider=provider,
            agent_role="judge",
            model_role="lite",
            model=response.model if response else self._model,
            monotonic_start=monotonic_start,
            monotonic_end=monotonic_end,
            input_artifact_hash=input_artifact_hash,
            input_tokens=usage.input_tokens if usage else 0,
            output_tokens=usage.output_tokens if usage else 0,
            total_tokens=(
                (usage.input_tokens + usage.output_tokens) if usage else 0
            ),
            usage_reported=usage is not None,
            retry_count=retry_count,
            succeeded=error is None,
            error_type=type(error).__name__ if error else None,
            output_artifact_hash=output_hash,
        )

    def _model_call_telemetry(
        self,
        response: GenerateContentResponse | None,
        response_text: str,
        monotonic_start: float,
        retry_count: int,
        input_artifact_hash: str,
        error: Exception | None = None,
    ) -> ModelCallTelemetry:
        usage = response.usage_metadata if response else UsageMetadata()
        finish_reason = response.candidates[0].finish_reason if response and response.candidates else None
        return build_model_call_telemetry(
            provider=self._provider,
            agent_role="judge",
            model_role="lite",
            model=self._model or "",
            monotonic_start=monotonic_start,
            input_artifact_hash=input_artifact_hash,
            input_tokens=usage.prompt_token_count,
            output_tokens=usage.candidates_token_count,
            thinking_tokens=usage.thinking_token_count,
            cache_tokens=usage.cache_token_count,
            total_tokens=usage.total_token_count,
            usage_reported=usage.usage_reported,
            finish_reason=finish_reason,
            generation_duration_seconds=usage.eval_duration_seconds,
            prompt_eval_duration_seconds=usage.prompt_eval_duration_seconds,
            total_duration_seconds=usage.total_duration_seconds,
            load_duration_seconds=usage.load_duration_seconds,
            retry_count=retry_count,
            succeeded=error is None,
            error_type=type(error).__name__ if error else None,
            output_artifact_hash=model_boundary_hash(response_text),
        )
