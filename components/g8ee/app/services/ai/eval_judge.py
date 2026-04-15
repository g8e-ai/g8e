# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

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
from typing import Any

from pydantic import BaseModel, Field, field_validator

from app.llm.llm_types import Content, Part, ResponseFormat, Role, LiteLLMSettings
from app.llm.provider import LLMProvider as LLMProviderBase
from app.models.settings import EvalJudgeSettings
from app.constants import LLM_DEFAULT_TEMPERATURE

logger = logging.getLogger(__name__)

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
    code = getattr(exc, "code", None) or getattr(exc, "status_code", None)
    if code in (429, 503):
        return True
    msg = str(exc).lower()
    return any(pattern in msg for pattern in _RETRYABLE_PATTERNS)


class EvalJudgeError(Exception):
    """Raised when the judge cannot produce a valid grade due to a system error.

    This is distinct from a low score — it means the evaluation itself failed
    (LLM unreachable, invalid response after retries, etc.).
    """


class EvalGrade(BaseModel):
    """Result of an evaluation grade."""
    score: int = Field(..., ge=1, le=5, description="Score from 1 to 5")
    reasoning: str = Field(..., description="Detailed explanation for the score")
    passed: bool = Field(..., description="Whether the score meets the passing threshold")

    @field_validator("reasoning", mode="before")
    @classmethod
    def _reasoning_must_be_nonempty(cls, v: str) -> str:
        if not v or not v.strip():
            raise ValueError("reasoning must not be empty")
        return v


_JUDGE_RESPONSE_SCHEMA = {
    "type": "object",
    "properties": {
        "score": {"type": "integer", "minimum": 1, "maximum": 5},
        "reasoning": {"type": "string"},
    },
    "required": ["score", "reasoning"],
}

_JUDGE_PROMPT_TEMPLATE = """\
You are an expert AI Agent Evaluator. Your task is to grade the performance of an AI Agent (the "Student") \
based on a User Query, the Student's Response/Actions, and a set of Gold Standard Criteria.

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
{student_interaction}
</student_interaction>

Grading Rubric (1-5):
1: Fails completely. Uses forbidden tools or completely ignores the intent.
2: Major issues. Misses required concepts or fails the primary intent.
3: Functional but flawed. Meets intent but with minor inaccuracies or sub-optimal tool use.
4: Good. Meets all criteria with clear, accurate responses.
5: Exceptional. Perfect tool use, hits all concepts, and provides a clear, concise response.

Response Format (JSON):
{{
  "score": integer (1-5),
  "reasoning": "string explaining why this score was assigned"
}}

Output ONLY the JSON object."""

_JSON_FENCE_RE = re.compile(r"```(?:json)?\s*(.*?)\s*```", re.DOTALL)


def _extract_json(text: str) -> dict[str, Any]:
    """Extract a JSON object from LLM text, handling markdown fences."""
    stripped = text.strip()
    fence_match = _JSON_FENCE_RE.search(stripped)
    if fence_match:
        stripped = fence_match.group(1).strip()
    return json.loads(stripped)


class EvalJudge:
    """Judge that uses an LLM to evaluate agent accuracy.

    Construction requires a concrete LLM provider instance and either
    an explicit model name or EvalJudgeSettings. The provider is the
    abstract LLMProvider from ``app.llm.provider``, not the LLMProvider
    enum from ``app.constants``.
    """

    def __init__(
        self,
        provider: LLMProviderBase | None = None,
        model: str | None = None,
        settings: EvalJudgeSettings | None = None,
    ):
        if provider is None:
            raise EvalJudgeError("EvalJudge requires a configured LLM provider instance")

        self._provider = provider
        self._settings = settings or EvalJudgeSettings(
            eval_judge_model=None,
            eval_judge_temperature=None,
            eval_judge_max_tokens=4096,
        )
        self._model = model or self._settings.model

        if not self._model:
            raise EvalJudgeError("EvalJudge requires an explicit model name (via model parameter or settings.model)")

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
        prompt = _JUDGE_PROMPT_TEMPLATE.format(
            user_query=user_query,
            expected_behavior=expected_behavior,
            required_concepts=", ".join(required_concepts),
            expected_tools=", ".join(expected_tools or []),
            forbidden_tools=", ".join(forbidden_tools or []),
            student_interaction=interaction_trace,
        )

        from app.models.model_configs import get_model_config

        effective_temperature = self._settings.temperature if self._settings.temperature is not None else None
        if effective_temperature is None:
            model_config = get_model_config(self._model)
            effective_temperature = model_config.default_temperature if model_config and model_config.default_temperature is not None else LLM_DEFAULT_TEMPERATURE
        settings = LiteLLMSettings(
            temperature=effective_temperature,
            max_output_tokens=self._settings.max_output_tokens,
            system_instructions="",
            response_format=ResponseFormat.from_pydantic_schema(  # type: ignore[arg-type]
                _JUDGE_RESPONSE_SCHEMA,
                name="EvalGradeResponse",
            ),
        )

        contents = [Content(role=Role.USER, parts=[Part.from_text(prompt)])]
        last_error: Exception | None = None
        delay = _INITIAL_RETRY_DELAY_SECONDS

        for attempt in range(_MAX_RETRIES):
            try:
                return await self._call_and_parse(contents, settings)
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

        logger.error(
            "EvalJudge failed after %d attempt(s)", _MAX_RETRIES, exc_info=True
        )
        raise EvalJudgeError(
            f"Judge could not produce a valid grade after {_MAX_RETRIES} "
            f"attempt(s): {last_error}"
        ) from last_error

    async def _call_and_parse(
        self,
        contents: list[Content],
        settings: LiteLLMSettings,
    ) -> EvalGrade:
        """Make the LLM call and parse the response into an EvalGrade."""
        if not self._model:
            raise EvalJudgeError("Model is not set")
        response = await self._provider.generate_content_lite(
            model=self._model,
            contents=contents,
            lite_llm_settings=settings,
        )

        if not response or not response.text:
            raise EvalJudgeError("Judge LLM returned an empty response")

        try:
            data = _extract_json(response.text)
        except json.JSONDecodeError as exc:
            raise EvalJudgeError(
                f"Judge LLM returned invalid JSON: {response.text[:200]}"
            ) from exc

        score = data.get("score")
        reasoning = data.get("reasoning")

        if score is None or reasoning is None:
            raise EvalJudgeError(
                f"Judge response missing required fields: {data}"
            )

        if not isinstance(score, int) or score < 1 or score > 5:
            raise EvalJudgeError(
                f"Judge returned out-of-range score: {score}"
            )

        return EvalGrade(
            score=score,
            reasoning=reasoning,
            passed=score >= PASSING_THRESHOLD,
        )
