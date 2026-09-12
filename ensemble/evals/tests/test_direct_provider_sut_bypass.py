# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for DirectProviderSUT bypass semantics.

Proves the direct arm bypasses g8ee HTTP, SSE trails, receipt collection,
and governance events. The SUT calls the model directly through the
shared provider abstraction with a single user turn, no system
instructions, no tools, and no agent loop.
"""

from __future__ import annotations

from collections.abc import AsyncGenerator
from unittest.mock import AsyncMock, MagicMock

import pytest

from app.llm.llm_types import StreamChunkFromModel, UsageMetadata

from g8e_evals.arms import Arm
from g8e_evals.harness import BindingType, LLMRoleConfig, SUTConfig, Task
from g8e_evals.sut.direct_provider import DirectProviderSUT

pytestmark = pytest.mark.unit


def _config() -> SUTConfig:
    return SUTConfig(
        g8ee_url="http://g8ee:8000",
        primary=LLMRoleConfig(provider="ollama", model="test-model"),
        arm=Arm.DIRECT,
    )


def _stream_chunks(text: str, usage: UsageMetadata | None = None, finish_reason: str = "STOP"):
    """Return a callable producing an async chunk stream for the SUT."""
    def _factory(**_kwargs) -> AsyncGenerator[StreamChunkFromModel]:
        async def _gen():
            yield StreamChunkFromModel(text=text)
            yield StreamChunkFromModel(
                finish_reason=finish_reason,
                usage_metadata=usage or UsageMetadata(),
            )
        return _gen()
    return _factory


def _usage(**overrides) -> UsageMetadata:
    base = {
        "prompt_token_count": 10,
        "candidates_token_count": 5,
        "total_token_count": 15,
        "thinking_token_count": 0,
        "cache_token_count": 2,
        "usage_reported": True,
    }
    base.update(overrides)
    return UsageMetadata(**base)


def test_direct_provider_sut_requires_primary_provider_and_model():
    config = SUTConfig(
        g8ee_url="http://g8ee:8000",
        primary=LLMRoleConfig(),
        arm=Arm.DIRECT,
    )
    with pytest.raises(ValueError, match="requires a primary provider and model"):
        DirectProviderSUT(config)


def test_direct_provider_sut_sets_arm_to_direct():
    sut = DirectProviderSUT(_config())
    assert sut.config.arm == Arm.DIRECT


@pytest.mark.asyncio
async def test_direct_provider_sut_bypasses_g8ee_http(monkeypatch):
    """The direct arm must not make any HTTP call to g8ee."""
    sut = DirectProviderSUT(_config())

    sut._provider = MagicMock()
    sut._provider.generate_content_stream_primary = MagicMock(
        side_effect=_stream_chunks("A direct answer.", usage=_usage())
    )
    sut._provider.force_close = AsyncMock()

    task = Task(id="1001", prompt="Write a sentence.")
    response = await sut.get_answer(task)

    assert response.arm == Arm.DIRECT
    assert response.binding == BindingType.UNBOUND
    assert response.answer == "A direct answer."
    assert response.transaction_ids == []
    assert response.receipts == []
    assert response.receipts_verified is False


@pytest.mark.asyncio
async def test_direct_provider_sut_produces_no_sse_trail(monkeypatch):
    """The direct arm must not produce an SSE trail or agent events."""
    sut = DirectProviderSUT(_config())

    sut._provider = MagicMock()
    sut._provider.generate_content_stream_primary = MagicMock(
        side_effect=_stream_chunks("Direct answer.", usage=_usage())
    )
    sut._provider.force_close = AsyncMock()

    task = Task(id="1001", prompt="Write a sentence.")
    response = await sut.get_answer(task)

    evidence = response.chat_evidence
    assert evidence is not None
    assert evidence.event_count == 0
    assert evidence.terminal_event == "direct.provider.completed"


@pytest.mark.asyncio
async def test_direct_provider_sut_no_receipt_collected(monkeypatch):
    """The direct arm must not collect or verify any ActionReceipt."""
    sut = DirectProviderSUT(_config())

    sut._provider = MagicMock()
    sut._provider.generate_content_stream_primary = MagicMock(
        side_effect=_stream_chunks("Answer.", usage=_usage(
            prompt_token_count=5, candidates_token_count=3, total_token_count=8
        ))
    )
    sut._provider.force_close = AsyncMock()

    task = Task(id="1001", prompt="Write a sentence.")
    response = await sut.get_answer(task)

    assert response.receipts == []
    assert response.receipts_verified is False
    assert response.binding == BindingType.UNBOUND
    assert "direct arm" in (response.unbound_reason or "")


@pytest.mark.asyncio
async def test_direct_provider_sut_handles_provider_error(monkeypatch):
    """A provider error produces an empty answer with error evidence."""
    sut = DirectProviderSUT(_config())

    sut._provider = MagicMock()
    sut._provider.generate_content_stream_primary = MagicMock(
        side_effect=RuntimeError("connection refused")
    )
    sut._provider.force_close = AsyncMock()

    task = Task(id="1001", prompt="Write a sentence.")
    response = await sut.get_answer(task)

    assert response.answer == ""
    assert response.arm == Arm.DIRECT
    assert response.binding == BindingType.UNBOUND
    evidence = response.chat_evidence
    assert evidence is not None
    assert evidence.terminal_event == "direct.provider.failed"
    dump = evidence.model_dump()
    assert "connection refused" in dump.get("error", "")


@pytest.mark.asyncio
async def test_direct_provider_sut_check_settings_is_noop():
    """The direct arm has no remote settings endpoint to query."""
    sut = DirectProviderSUT(_config())
    result = await sut.check_settings()
    assert result is None


@pytest.mark.asyncio
async def test_direct_call_evidence_captures_token_usage():
    sut = DirectProviderSUT(_config())

    sut._provider = MagicMock()
    sut._provider.generate_content_stream_primary = MagicMock(
        side_effect=_stream_chunks("Answer with tokens.", usage=_usage(
            prompt_token_count=100,
            candidates_token_count=50,
            total_token_count=150,
            thinking_token_count=10,
            cache_token_count=20,
        ))
    )
    sut._provider.force_close = AsyncMock()

    task = Task(id="1001", prompt="Write a sentence.")
    response = await sut.get_answer(task)

    assert response.chat_evidence is not None
    dump = response.chat_evidence.model_dump()
    assert dump["prompt_token_count"] == 100
    assert dump["candidates_token_count"] == 50
    assert dump["total_token_count"] == 150
    assert dump["thinking_token_count"] == 10
    assert dump["cache_token_count"] == 20
    assert dump["usage_reported"] is True
    assert dump["finish_reason"] == "STOP"


@pytest.mark.asyncio
async def test_direct_call_evidence_uses_provider_outbound_payload_hash():
    sut = DirectProviderSUT(_config())
    sut._provider = MagicMock()
    sut._provider.input_artifact_hash = "provider-outbound-payload-hash"
    sut._provider.generate_content_stream_primary = MagicMock(
        side_effect=_stream_chunks("Answer.", usage=_usage(
            prompt_token_count=5, candidates_token_count=3, total_token_count=8
        ))
    )
    sut._provider.force_close = AsyncMock()

    response = await sut.get_answer(Task(id="1001", prompt="Write a sentence."))

    assert response.chat_evidence is not None
    assert response.chat_evidence.model_dump()["input_artifact_hash"] == (
        "provider-outbound-payload-hash"
    )


@pytest.mark.asyncio
async def test_direct_provider_sut_observation_carries_native_timing():
    """The collapsed stream surfaces provider-native durations on the
    InferenceObservation: TTFT from first-chunk arrival, generation
    duration from eval_duration, throughput from eval_count/eval_duration."""
    sut = DirectProviderSUT(_config())
    usage = _usage(
        time_to_first_token_seconds=0.4,
        prompt_eval_duration_seconds=0.01,
        eval_duration_seconds=0.04,
        total_duration_seconds=0.052,
        load_duration_seconds=0.002,
    )
    sut._provider = MagicMock()
    sut._provider.generate_content_stream_primary = MagicMock(
        side_effect=_stream_chunks("Answer.", usage=usage)
    )
    sut._provider.force_close = AsyncMock()

    response = await sut.get_answer(Task(id="1001", prompt="Write a sentence."))

    assert len(response.inference_observations) == 1
    obs = response.inference_observations[0]
    assert obs.time_to_first_token_seconds == 0.4
    assert obs.generation_duration_seconds == 0.04
    # 5 output tokens / 0.04 s eval duration
    assert obs.output_throughput_tokens_per_second == 125.0
    assert obs.provider_call_latency_seconds is not None
    assert obs.provider_call_latency_seconds > 0.0

    assert response.chat_evidence is not None
    dump = response.chat_evidence.model_dump()
    assert dump["time_to_first_token_seconds"] == 0.4
    assert dump["prompt_eval_duration_seconds"] == 0.01
    assert dump["eval_duration_seconds"] == 0.04
    assert dump["total_duration_seconds"] == 0.052
    assert dump["load_duration_seconds"] == 0.002


@pytest.mark.asyncio
async def test_direct_provider_sut_throughput_falls_back_to_elapsed():
    """When the provider does not report eval_duration, output throughput
    derives from the measured monotonic span instead."""
    sut = DirectProviderSUT(_config())
    usage = _usage(candidates_token_count=5)
    sut._provider = MagicMock()
    sut._provider.generate_content_stream_primary = MagicMock(
        side_effect=_stream_chunks("Answer.", usage=usage)
    )
    sut._provider.force_close = AsyncMock()

    response = await sut.get_answer(Task(id="1001", prompt="Write a sentence."))

    obs = response.inference_observations[0]
    assert obs.generation_duration_seconds is None
    assert obs.time_to_first_token_seconds is None
    assert obs.output_throughput_tokens_per_second is not None
    assert obs.output_throughput_tokens_per_second > 0.0


@pytest.mark.asyncio
async def test_direct_provider_sut_failed_call_measures_latency_only():
    """A failed provider call still reports the measured monotonic span;
    TTFT and generation duration remain unmeasured (None)."""
    sut = DirectProviderSUT(_config())
    sut._provider = MagicMock()
    sut._provider.generate_content_stream_primary = MagicMock(
        side_effect=RuntimeError("connection refused")
    )
    sut._provider.force_close = AsyncMock()

    response = await sut.get_answer(Task(id="1001", prompt="Write a sentence."))

    assert len(response.inference_observations) == 1
    obs = response.inference_observations[0]
    assert obs.error == "connection refused"
    assert obs.provider_call_latency_seconds is not None
    assert obs.provider_call_latency_seconds >= 0.0
    assert obs.time_to_first_token_seconds is None
    assert obs.generation_duration_seconds is None
    assert obs.output_throughput_tokens_per_second is None
