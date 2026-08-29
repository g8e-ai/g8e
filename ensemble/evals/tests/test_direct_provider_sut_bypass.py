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

from unittest.mock import AsyncMock, MagicMock

import pytest

from g8e_evals.arms import Arm
from g8e_evals.harness import BindingType, LLMRoleConfig, SUTConfig, Task
from g8e_evals.sut.direct_provider import DirectCallEvidence, DirectProviderSUT

pytestmark = pytest.mark.unit


def _config() -> SUTConfig:
    return SUTConfig(
        g8ee_url="http://g8ee:8000",
        primary=LLMRoleConfig(provider="ollama", model="test-model"),
        arm=Arm.DIRECT,
    )


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

    mock_response = MagicMock()
    mock_response.text = "A direct answer."
    mock_response.candidates = [MagicMock(finish_reason="STOP")]
    mock_response.usage_metadata = MagicMock(
        prompt_token_count=10,
        candidates_token_count=5,
        total_token_count=15,
        thinking_token_count=0,
    )

    sut._provider = MagicMock()
    sut._provider.generate_content_primary = AsyncMock(return_value=mock_response)
    sut._provider.force_close = AsyncMock()

    task = Task(id="1001", prompt="Write a sentence.")
    response = await sut.get_answer(task)

    assert response.arm == Arm.DIRECT
    assert response.binding == BindingType.UNBOUND
    assert response.answer == "A direct answer."
    assert response.transaction_id is None
    assert response.action_receipt is None
    assert response.receipt_verified is False


@pytest.mark.asyncio
async def test_direct_provider_sut_produces_no_sse_trail(monkeypatch):
    """The direct arm must not produce an SSE trail or agent events."""
    sut = DirectProviderSUT(_config())

    mock_response = MagicMock()
    mock_response.text = "Direct answer."
    mock_response.candidates = [MagicMock(finish_reason="STOP")]
    mock_response.usage_metadata = MagicMock(
        prompt_token_count=10,
        candidates_token_count=5,
        total_token_count=15,
        thinking_token_count=0,
    )

    sut._provider = MagicMock()
    sut._provider.generate_content_primary = AsyncMock(return_value=mock_response)
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

    mock_response = MagicMock()
    mock_response.text = "Answer."
    mock_response.candidates = [MagicMock(finish_reason="STOP")]
    mock_response.usage_metadata = MagicMock(
        prompt_token_count=5,
        candidates_token_count=3,
        total_token_count=8,
        thinking_token_count=0,
    )

    sut._provider = MagicMock()
    sut._provider.generate_content_primary = AsyncMock(return_value=mock_response)
    sut._provider.force_close = AsyncMock()

    task = Task(id="1001", prompt="Write a sentence.")
    response = await sut.get_answer(task)

    assert response.action_receipt is None
    assert response.receipt_verified is False
    assert response.binding == BindingType.UNBOUND
    assert "direct arm" in (response.unbound_reason or "")


@pytest.mark.asyncio
async def test_direct_provider_sut_handles_provider_error(monkeypatch):
    """A provider error produces an empty answer with error evidence."""
    sut = DirectProviderSUT(_config())

    sut._provider = MagicMock()
    sut._provider.generate_content_primary = AsyncMock(side_effect=RuntimeError("connection refused"))
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

    mock_response = MagicMock()
    mock_response.text = "Answer with tokens."
    mock_response.candidates = [MagicMock(finish_reason="STOP")]
    mock_response.usage_metadata = MagicMock(
        prompt_token_count=100,
        candidates_token_count=50,
        total_token_count=150,
        thinking_token_count=10,
    )

    sut._provider = MagicMock()
    sut._provider.generate_content_primary = AsyncMock(return_value=mock_response)
    sut._provider.force_close = AsyncMock()

    task = Task(id="1001", prompt="Write a sentence.")
    response = await sut.get_answer(task)

    dump = response.chat_evidence.model_dump()
    assert dump["prompt_token_count"] == 100
    assert dump["candidates_token_count"] == 50
    assert dump["total_token_count"] == 150
    assert dump["thinking_token_count"] == 10
    assert dump["finish_reason"] == "STOP"
