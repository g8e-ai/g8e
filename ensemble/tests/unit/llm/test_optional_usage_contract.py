"""Contract packet for preserving absent versus explicitly-zero usage metrics.

These tests are intentionally strict expected failures until the cross-language
telemetry contract is versioned.  They make the required semantics executable
without contacting Ollama or changing generated protocol files.
"""

import pytest

from app.llm.llm_dataclasses import UsageMetadata
from app.models.agent import TurnResult
from app.models.tool_results import TokenUsage
from g8e.models.events import ModelCallTelemetry
from g8e.operator.v1.operator_pb2 import InferenceResult


_OPTIONALITY_REASON = (
    "Worker 6 packet: Python telemetry still defaults absent "
    "thinking/cache counts to zero"
)


@pytest.mark.unit
@pytest.mark.xfail(strict=True, reason=_OPTIONALITY_REASON)
def test_usage_metadata_keeps_absent_thinking_and_cache_counts_distinct():
    usage = UsageMetadata()

    assert usage.thinking_token_count is None
    assert usage.cache_token_count is None


@pytest.mark.unit
@pytest.mark.xfail(strict=True, reason=_OPTIONALITY_REASON)
def test_turn_result_keeps_absent_thinking_and_cache_counts_distinct():
    result = TurnResult(
        model_response_parts=[],
        pending_tool_calls=[],
        finish_reason=None,
        input_tokens=1,
        output_tokens=2,
        total_tokens=3,
    )

    assert result.thinking_tokens is None
    assert result.cache_tokens is None


@pytest.mark.unit
@pytest.mark.xfail(strict=True, reason=_OPTIONALITY_REASON)
def test_token_usage_keeps_absent_thinking_and_cache_counts_distinct():
    usage = TokenUsage(input_tokens=1, output_tokens=2, total_tokens=3)

    assert usage.thinking_tokens is None
    assert usage.cache_tokens is None


@pytest.mark.unit
@pytest.mark.xfail(strict=True, reason=_OPTIONALITY_REASON)
def test_model_call_telemetry_keeps_absent_thinking_and_cache_counts_distinct():
    telemetry = ModelCallTelemetry(
        agent_role="primary",
        provider="fake",
        model="test-model",
        monotonic_start=1.0,
        monotonic_end=2.0,
    )

    assert telemetry.thinking_tokens is None
    assert telemetry.cache_tokens is None


@pytest.mark.unit
def test_inference_result_proto_preserves_absent_and_explicit_zero_presence():
    absent = InferenceResult()
    explicit_zero = InferenceResult(thinking_tokens=0, cache_tokens=0)

    assert not absent.HasField("thinking_tokens")
    assert not absent.HasField("cache_tokens")
    assert explicit_zero.HasField("thinking_tokens")
    assert explicit_zero.HasField("cache_tokens")
    assert explicit_zero.thinking_tokens == 0
    assert explicit_zero.cache_tokens == 0
