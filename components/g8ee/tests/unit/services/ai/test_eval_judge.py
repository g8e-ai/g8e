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
Unit tests for the EvalJudge service.

Tests cover:
- Successful grading with deterministic pass/fail
- Score range validation (1-5 enforced)
- LLM API failure handling (raises EvalJudgeError)
- Transient failure retry with exponential backoff
- JSON parsing edge cases (markdown fences, missing fields)
- Response format configuration
- Prompt construction
"""

import json
import pytest
from unittest.mock import AsyncMock, MagicMock, patch

from app.services.ai.eval_judge import (
    EvalJudge,
    EvalGrade,
    EvalJudgeError,
    PASSING_THRESHOLD,
    _extract_json,
    _is_retryable,
    _MAX_RETRIES,
)
from app.llm.llm_types import GenerateContentResponse, Candidate, Content, Part
from app.models.settings import EvalJudgeSettings


def _build_response(text: str) -> GenerateContentResponse:
    """Build a GenerateContentResponse with the given text payload."""
    return GenerateContentResponse(
        candidates=[Candidate(
            content=Content(
                role="model",
                parts=[Part.from_text(text)],
            ),
            finish_reason="STOP",
        )]
    )


def _build_grade_json(score: int, reasoning: str) -> str:
    """Build a JSON string matching the judge response schema."""
    return json.dumps({"score": score, "reasoning": reasoning})


@pytest.fixture
def mock_provider():
    """Mock LLM provider with async generate_content_lite."""
    provider = MagicMock()
    provider.generate_content_lite = AsyncMock()
    return provider


@pytest.fixture
def judge(mock_provider):
    """EvalJudge with mocked provider."""
    return EvalJudge(provider=mock_provider, model="gemini-3.1-pro-preview")


GRADE_KWARGS = dict(
    user_query="Test query",
    interaction_trace="Test trace",
    expected_behavior="Test behavior",
    required_concepts=["test"],
)


class TestEvalGradeModel:
    """EvalGrade Pydantic model validation."""

    def test_valid_construction(self):
        g = EvalGrade(score=4, reasoning="Good", passed=True)
        assert g.score == 4
        assert g.reasoning == "Good"
        assert g.passed is True

    def test_serialization_roundtrip(self):
        g = EvalGrade(score=5, reasoning="Excellent", passed=True)
        data = g.model_dump()
        assert data == {"score": 5, "reasoning": "Excellent", "passed": True}
        assert EvalGrade(**data) == g

    def test_score_below_range_rejected(self):
        with pytest.raises(Exception):
            EvalGrade(score=0, reasoning="Too low", passed=False)

    def test_score_above_range_rejected(self):
        with pytest.raises(Exception):
            EvalGrade(score=6, reasoning="Too high", passed=True)

    def test_empty_reasoning_rejected(self):
        with pytest.raises(Exception):
            EvalGrade(score=3, reasoning="", passed=True)

    def test_whitespace_only_reasoning_rejected(self):
        with pytest.raises(Exception):
            EvalGrade(score=3, reasoning="   ", passed=True)


class TestExtractJson:
    """JSON extraction from LLM text, including markdown fences."""

    def test_plain_json(self):
        assert _extract_json('{"score": 3, "reasoning": "OK"}') == {"score": 3, "reasoning": "OK"}

    def test_json_in_code_fence(self):
        text = '```json\n{"score": 4, "reasoning": "Good"}\n```'
        assert _extract_json(text) == {"score": 4, "reasoning": "Good"}

    def test_json_in_bare_fence(self):
        text = '```\n{"score": 2, "reasoning": "Bad"}\n```'
        assert _extract_json(text) == {"score": 2, "reasoning": "Bad"}

    def test_json_with_surrounding_whitespace(self):
        text = '  \n  {"score": 5, "reasoning": "Top"}  \n  '
        assert _extract_json(text) == {"score": 5, "reasoning": "Top"}

    def test_invalid_json_raises(self):
        with pytest.raises(json.JSONDecodeError):
            _extract_json("not json at all")


class TestIsRetryable:
    """Transient error classification."""

    def test_rate_limit_string(self):
        assert _is_retryable(Exception("429 Too Many Requests")) is True

    def test_service_unavailable_string(self):
        assert _is_retryable(Exception("503 Service Unavailable")) is True

    def test_resource_exhausted(self):
        assert _is_retryable(Exception("RESOURCE_EXHAUSTED: quota exceeded")) is True

    def test_status_code_attribute(self):
        exc = Exception("fail")
        exc.status_code = 429
        assert _is_retryable(exc) is True

    def test_non_retryable_error(self):
        assert _is_retryable(Exception("401 Unauthorized")) is False

    def test_generic_error_not_retryable(self):
        assert _is_retryable(ValueError("bad value")) is False


class TestEvalJudgeConstruction:
    """Constructor validation."""

    def test_requires_model_name(self):
        provider = MagicMock()
        with pytest.raises(EvalJudgeError, match="explicit model name"):
            EvalJudge(provider=provider, model="")

    def test_none_model_rejected(self):
        provider = MagicMock()
        with pytest.raises(EvalJudgeError, match="explicit model name"):
            EvalJudge(provider=provider, model=None)

    def test_none_provider_rejected(self):
        with pytest.raises(EvalJudgeError, match="configured LLM provider"):
            EvalJudge(provider=None, model="some-model")

    def test_valid_construction(self):
        provider = MagicMock()
        judge = EvalJudge(provider=provider, model="some-model")
        assert judge._model == "some-model"

    def test_construction_with_settings(self):
        provider = MagicMock()
        settings = EvalJudgeSettings(
            model="settings-model",
            temperature=0.5,
            max_output_tokens=8192,
        )
        judge = EvalJudge(provider=provider, settings=settings)
        assert judge._model == "settings-model"
        assert judge._settings.temperature == 0.5
        assert judge._settings.max_output_tokens == 8192

    def test_construction_with_model_overrides_settings(self):
        provider = MagicMock()
        settings = EvalJudgeSettings(
            model="settings-model",
            temperature=0.5,
            max_output_tokens=8192,
        )
        judge = EvalJudge(provider=provider, model="override-model", settings=settings)
        assert judge._model == "override-model"
        assert judge._settings.temperature == 0.5

    def test_construction_with_default_settings(self):
        provider = MagicMock()
        judge = EvalJudge(provider=provider, model="some-model")
        assert judge._settings.temperature is None
        assert judge._settings.max_output_tokens == 4096


class TestGradeTurnHappyPath:
    """Successful grading scenarios."""

    async def test_high_score_passes(self, judge, mock_provider):
        mock_provider.generate_content_lite.return_value = _build_response(
            _build_grade_json(4, "Good response with proper tool usage")
        )
        result = await judge.grade_turn(
            user_query="List files in /tmp",
            interaction_trace="TOOL_CALLS: run_commands_with_operator\nRESPONSE: Files listed",
            expected_behavior="Execute ls command",
            required_concepts=["ls", "/tmp"],
            expected_tools=["run_commands_with_operator"],
        )
        assert result.score == 4
        assert result.passed is True
        assert result.reasoning == "Good response with proper tool usage"

    async def test_low_score_fails(self, judge, mock_provider):
        mock_provider.generate_content_lite.return_value = _build_response(
            _build_grade_json(2, "Major issues found")
        )
        result = await judge.grade_turn(**GRADE_KWARGS)
        assert result.score == 2
        assert result.passed is False

    async def test_threshold_score_passes(self, judge, mock_provider):
        mock_provider.generate_content_lite.return_value = _build_response(
            _build_grade_json(PASSING_THRESHOLD, "Meets minimum bar")
        )
        result = await judge.grade_turn(**GRADE_KWARGS)
        assert result.score == PASSING_THRESHOLD
        assert result.passed is True

    async def test_passed_is_deterministic_not_llm_driven(self, judge, mock_provider):
        """LLM cannot override the pass/fail decision regardless of what it returns."""
        mock_provider.generate_content_lite.return_value = _build_response(
            json.dumps({"score": 2, "reasoning": "Bad but LLM says pass", "passed": True})
        )
        result = await judge.grade_turn(**GRADE_KWARGS)
        assert result.score == 2
        assert result.passed is False

    async def test_response_format_config(self, judge, mock_provider):
        mock_provider.generate_content_lite.return_value = _build_response(
            _build_grade_json(5, "Perfect")
        )
        await judge.grade_turn(**GRADE_KWARGS)
        call_args = mock_provider.generate_content_lite.call_args
        settings = call_args.kwargs["lite_llm_settings"]
        assert settings.response_format is not None

    async def test_settings_used_in_grade_turn(self, mock_provider):
        custom_settings = EvalJudgeSettings(
            model="custom-model",
            temperature=0.7,
            max_output_tokens=2048,
        )
        judge = EvalJudge(provider=mock_provider, model="custom-model", settings=custom_settings)
        mock_provider.generate_content_lite.return_value = _build_response(
            _build_grade_json(4, "Good")
        )
        await judge.grade_turn(**GRADE_KWARGS)
        call_args = mock_provider.generate_content_lite.call_args
        settings = call_args.kwargs["lite_llm_settings"]
        assert settings.temperature == 0.7
        assert settings.max_output_tokens == 2048

    async def test_model_passed_to_provider(self, mock_provider):
        judge = EvalJudge(provider=mock_provider, model="custom-eval-model")
        mock_provider.generate_content_lite.return_value = _build_response(
            _build_grade_json(3, "OK")
        )
        await judge.grade_turn(**GRADE_KWARGS)
        call_args = mock_provider.generate_content_lite.call_args
        assert call_args.kwargs["model"] == "custom-eval-model"

    async def test_handles_markdown_fenced_response(self, judge, mock_provider):
        mock_provider.generate_content_lite.return_value = _build_response(
            '```json\n{"score": 4, "reasoning": "Fenced response"}\n```'
        )
        result = await judge.grade_turn(**GRADE_KWARGS)
        assert result.score == 4
        assert result.reasoning == "Fenced response"

    async def test_extra_fields_in_response_ignored(self, judge, mock_provider):
        mock_provider.generate_content_lite.return_value = _build_response(
            json.dumps({"score": 3, "reasoning": "With extras", "extra_field": "ignored"})
        )
        result = await judge.grade_turn(**GRADE_KWARGS)
        assert result.score == 3


class TestGradeTurnErrorPaths:
    """Error handling — all system errors raise EvalJudgeError."""

    async def test_empty_candidates_raises(self, judge, mock_provider):
        mock_provider.generate_content_lite.return_value = GenerateContentResponse(candidates=[])
        with pytest.raises(EvalJudgeError, match="empty response"):
            await judge.grade_turn(**GRADE_KWARGS)

    async def test_none_response_raises(self, judge, mock_provider):
        mock_provider.generate_content_lite.return_value = None
        with pytest.raises(EvalJudgeError, match="empty response"):
            await judge.grade_turn(**GRADE_KWARGS)

    async def test_invalid_json_raises(self, judge, mock_provider):
        mock_provider.generate_content_lite.return_value = _build_response("not valid json")
        with pytest.raises(EvalJudgeError, match="invalid JSON"):
            await judge.grade_turn(**GRADE_KWARGS)

    async def test_missing_score_raises(self, judge, mock_provider):
        mock_provider.generate_content_lite.return_value = _build_response(
            '{"reasoning": "No score field"}'
        )
        with pytest.raises(EvalJudgeError, match="missing required fields"):
            await judge.grade_turn(**GRADE_KWARGS)

    async def test_missing_reasoning_raises(self, judge, mock_provider):
        mock_provider.generate_content_lite.return_value = _build_response('{"score": 4}')
        with pytest.raises(EvalJudgeError, match="missing required fields"):
            await judge.grade_turn(**GRADE_KWARGS)

    async def test_out_of_range_score_raises(self, judge, mock_provider):
        mock_provider.generate_content_lite.return_value = _build_response(
            '{"score": 10, "reasoning": "Way too high"}'
        )
        with pytest.raises(EvalJudgeError, match="out-of-range score"):
            await judge.grade_turn(**GRADE_KWARGS)

    async def test_zero_score_raises(self, judge, mock_provider):
        mock_provider.generate_content_lite.return_value = _build_response(
            '{"score": 0, "reasoning": "Below minimum"}'
        )
        with pytest.raises(EvalJudgeError, match="out-of-range score"):
            await judge.grade_turn(**GRADE_KWARGS)

    async def test_non_retryable_api_error_raises_immediately(self, judge, mock_provider):
        mock_provider.generate_content_lite.side_effect = Exception("401 Unauthorized")
        with pytest.raises(EvalJudgeError, match="401 Unauthorized"):
            await judge.grade_turn(**GRADE_KWARGS)
        assert mock_provider.generate_content_lite.call_count == 1


class TestGradeTurnRetry:
    """Retry logic with exponential backoff for transient failures."""

    @patch("app.services.ai.eval_judge.asyncio.sleep", new_callable=AsyncMock)
    async def test_retries_on_rate_limit(self, mock_sleep, judge, mock_provider):
        mock_provider.generate_content_lite.side_effect = [
            Exception("429 Too Many Requests"),
            _build_response(_build_grade_json(4, "Recovered")),
        ]
        result = await judge.grade_turn(**GRADE_KWARGS)
        assert result.score == 4
        assert mock_provider.generate_content_lite.call_count == 2
        mock_sleep.assert_called_once()

    @patch("app.services.ai.eval_judge.asyncio.sleep", new_callable=AsyncMock)
    async def test_retries_on_503(self, mock_sleep, judge, mock_provider):
        mock_provider.generate_content_lite.side_effect = [
            Exception("503 Service Unavailable"),
            Exception("503 Service Unavailable"),
            _build_response(_build_grade_json(3, "Eventually worked")),
        ]
        result = await judge.grade_turn(**GRADE_KWARGS)
        assert result.score == 3
        assert mock_provider.generate_content_lite.call_count == 3
        assert mock_sleep.call_count == 2

    @patch("app.services.ai.eval_judge.asyncio.sleep", new_callable=AsyncMock)
    async def test_all_retries_exhausted_raises(self, mock_sleep, judge, mock_provider):
        mock_provider.generate_content_lite.side_effect = Exception("429 Rate Limited")
        with pytest.raises(EvalJudgeError, match="after 3 attempt"):
            await judge.grade_turn(**GRADE_KWARGS)
        assert mock_provider.generate_content_lite.call_count == _MAX_RETRIES

    @patch("app.services.ai.eval_judge.asyncio.sleep", new_callable=AsyncMock)
    async def test_backoff_delays_double(self, mock_sleep, judge, mock_provider):
        mock_provider.generate_content_lite.side_effect = Exception("429 Rate Limited")
        with pytest.raises(EvalJudgeError):
            await judge.grade_turn(**GRADE_KWARGS)
        delays = [call.args[0] for call in mock_sleep.call_args_list]
        assert delays[0] == 2.0
        assert delays[1] == 4.0

    async def test_eval_judge_error_not_retried(self, judge, mock_provider):
        """EvalJudgeError from _call_and_parse propagates immediately."""
        mock_provider.generate_content_lite.return_value = None
        with pytest.raises(EvalJudgeError, match="empty response"):
            await judge.grade_turn(**GRADE_KWARGS)
        assert mock_provider.generate_content_lite.call_count == 1


class TestPromptConstruction:
    """Prompt template population."""

    async def test_prompt_includes_all_criteria(self, judge, mock_provider):
        mock_provider.generate_content_lite.return_value = _build_response(
            _build_grade_json(4, "Good")
        )
        await judge.grade_turn(
            user_query="List files",
            interaction_trace="Some trace",
            expected_behavior="Execute command",
            required_concepts=["ls", "files"],
            expected_tools=["run_commands_with_operator"],
            forbidden_tools=["dangerous_tool"],
        )
        call_args = mock_provider.generate_content_lite.call_args
        prompt_text = call_args.kwargs["contents"][0].parts[0].text
        assert "List files" in prompt_text
        assert "Execute command" in prompt_text
        assert "ls" in prompt_text
        assert "run_commands_with_operator" in prompt_text
        assert "dangerous_tool" in prompt_text
        assert "Some trace" in prompt_text

    async def test_prompt_handles_none_tool_lists(self, judge, mock_provider):
        mock_provider.generate_content_lite.return_value = _build_response(
            _build_grade_json(3, "OK")
        )
        await judge.grade_turn(
            user_query="Test",
            interaction_trace="Test",
            expected_behavior="Test",
            required_concepts=["test"],
        )
        assert mock_provider.generate_content_lite.call_count == 1

    async def test_prompt_handles_empty_tool_lists(self, judge, mock_provider):
        mock_provider.generate_content_lite.return_value = _build_response(
            _build_grade_json(3, "OK")
        )
        await judge.grade_turn(
            user_query="Test",
            interaction_trace="Test",
            expected_behavior="Test",
            required_concepts=["test"],
            expected_tools=[],
            forbidden_tools=[],
        )
        assert mock_provider.generate_content_lite.call_count == 1


class TestScoreThresholds:
    """Deterministic pass/fail from score, independent of LLM opinion."""

    @pytest.mark.parametrize("score,expected_passed", [
        (1, False),
        (2, False),
        (3, True),
        (4, True),
        (5, True),
    ])
    async def test_score_determines_passed(self, score, expected_passed, judge, mock_provider):
        mock_provider.generate_content_lite.return_value = _build_response(
            _build_grade_json(score, f"Score {score} evaluation")
        )
        result = await judge.grade_turn(**GRADE_KWARGS)
        assert result.score == score
        assert result.passed == expected_passed

