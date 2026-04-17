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

from app.llm.llm_types import Content, Part, ResponseFormat, ResponseJsonSchema, Role, LiteLLMSettings
from app.llm.provider import LLMProvider as LLMProviderBase
from app.models.settings import EvalJudgeSettings
from app.constants import LLM_DEFAULT_TEMPERATURE
from app.utils.agent_persona_loader import get_agent_persona

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
        persona = get_agent_persona("eval_judge")
        prompt_template = persona.get_system_prompt()
        prompt = f"{prompt_template}\n\n<user_query>\n{user_query}\n</user_query>\n\n<gold_standard_criteria>\nExpected Behavior: {expected_behavior}\nRequired Concepts: {', '.join(required_concepts)}\nExpected Tools: {', '.join(expected_tools or [])}\nForbidden Tools: {', '.join(forbidden_tools or [])}\n</gold_standard_criteria>\n\n<student_interaction>\n{interaction_trace}\n</student_interaction>"

        from app.models.model_configs import get_model_config
        from app.services.ai.command_generator import _resolve_temperature

        effective_temperature = self._settings.temperature if self._settings.temperature is not None else persona.temperature
        effective_temperature = _resolve_temperature(effective_temperature, self._model)
        model_config = get_model_config(self._model)
        settings = LiteLLMSettings(
            temperature=effective_temperature,
            max_output_tokens=self._settings.max_output_tokens,
            top_p_nucleus_sampling=model_config.top_p,
            top_k_filtering=model_config.top_k,
            stop_sequences=model_config.stop_sequences,
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
        from app.errors import OllamaEmptyResponseError

        if not self._model:
            raise EvalJudgeError("Model is not set")
        try:
            response = await self._provider.generate_content_lite(
                model=self._model,
                contents=contents,
                lite_llm_settings=settings,
            )
            if response is None or not hasattr(response, 'text') or response.text is None:
                raise EvalJudgeError("Judge LLM returned an empty response")
        except OllamaEmptyResponseError as exc:
            raise EvalJudgeError(f"Judge LLM returned an empty response: {exc}") from exc

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
