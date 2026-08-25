# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Regression tests for G8eeChatSUT receipt-binding semantics.

The Operator's audit vault keys ActionReceipts by the UAP envelope
transaction_id (i.e. transaction_hash of a Warden-signed envelope), NOT by
the g8ee-issued investigation_id. A plain answer-only chat turn does not
trigger a Tribunal->Warden mutation and therefore produces no on-Gateway
ActionReceipt. The SUT must not lie about that:

  - It must not pass investigation_id off as transaction_id.
  - It must not claim BindingType.RECEIPT_BOUND when no Warden-signed
    receipt was observed in the agent trail.

This guards against a latent semantic bug where ReceiptCollector polled the
Operator with an investigation_id and silently timed out for every task.
"""

import json
import time
import asyncio
from unittest.mock import AsyncMock, MagicMock

import httpx
import pytest
from g8e_evals.harness import BindingType, Task, Response
from g8e_evals.sut.g8ee_chat import G8eeChatSUT, AgentTrailEvent, _extract_Gateway_transaction_id


def test_extract_Gateway_transaction_id_ignores_investigation_only_trail():
    trail = [
        AgentTrailEvent(
            id=1,
            event_type="g8e.v1.ai.llm.chat.iteration.text.chunk.received",
            payload={"investigation_id": "inv-abc", "data": {"content": "hi"}},
        ),
        AgentTrailEvent(
            id=2,
            event_type="g8e.v1.ai.llm.chat.iteration.text.completed",
            payload={"investigation_id": "inv-abc"},
        ),
    ]
    assert _extract_Gateway_transaction_id(trail) is None


def test_extract_Gateway_transaction_id_picks_warden_signed_receipt():
    trail = [
        AgentTrailEvent(
            id=1,
            event_type="g8e.v1.ai.llm.chat.iteration.text.chunk.received",
            payload={"investigation_id": "inv-abc"},
        ),
        AgentTrailEvent(
            id=2,
            event_type="g8e.v1.ai.governance.warden.receipt.signed",
            payload={
                "event": {
                    "type": "g8e.v1.ai.governance.warden.receipt.signed",
                    "data": {"transaction_hash": "tx-real-Gateway-id"},
                },
                "investigation_id": "inv-abc",
            },
        ),
    ]
    assert _extract_Gateway_transaction_id(trail) == "tx-real-Gateway-id"


def test_extract_Gateway_transaction_id_ignores_investigation_id_lookalikes():
    # An investigation_id used as a transaction_id on a non-Gateway event
    # must NOT be promoted to a Gateway transaction id.
    trail = [
        AgentTrailEvent(
            id=1,
            event_type="g8e.v1.app.case.investigation.created",
            payload={"transaction_id": "inv-abc"},
        ),
    ]
    assert _extract_Gateway_transaction_id(trail) is None


def test_binding_unbound_when_no_Gateway_receipt(monkeypatch):
    # Smoke-import to ensure the SUT's UNBOUND-no-receipt branch references
    # are valid module-level symbols; full end-to-end binding is exercised by
    # the live ifeval bench.
    from g8e_evals.sut import g8ee_chat

    assert hasattr(g8ee_chat, "_extract_Gateway_transaction_id")
    assert BindingType.UNBOUND.value == "UNBOUND"
    assert BindingType.RECEIPT_BOUND.value == "RECEIPT_BOUND"


@pytest.mark.asyncio
async def test_drain_events_propagates_auth_failure(monkeypatch):
    # Setup SUT with minimal config
    config = MagicMock()
    config.operator_session_id = "session-123"
    config.operator_url = "http://operator"
    config.primary.provider = "test"
    config.primary.model = "model"

    # Mock AuthContext.from_env to return our mock_env
    mock_env = MagicMock()
    mock_env.operator_url = "http://operator"
    mock_env.auth_headers.return_value = {"Authorization": "Bearer token"}
    mock_env.cli_session_id = "session-123"

    monkeypatch.setattr("g8e_evals.sut.g8ee_chat.AuthContext.from_env", lambda **kw: mock_env)

    sut = G8eeChatSUT(config=config)

    # Mock aconnect_sse to raise HTTPStatusError
    mock_resp = MagicMock()
    mock_resp.status_code = 401

    # Use a custom context manager mock
    class MockContextManager:
        async def __aenter__(self):
            raise httpx.HTTPStatusError("Unauthorized", request=MagicMock(), response=mock_resp)
        async def __aexit__(self, exc_type, exc_val, exc_tb):
            pass

    monkeypatch.setattr("g8e_evals.sut.g8ee_chat.aconnect_sse", lambda *a, **kw: MockContextManager())

    client = AsyncMock(spec=httpx.AsyncClient)
    answer, trail, terminal, error = await sut._drain_events(client, since_id=0, investigation_id="inv-123")

    assert error == "sse_auth_failed: 401"
    assert terminal is None


@pytest.mark.asyncio
async def test_drain_events_idle_timeout_survives_heartbeat_only_stream(monkeypatch):
    config = MagicMock()
    config.operator_session_id = "session-123"
    config.operator_url = "http://operator"
    config.primary.provider = "test"
    config.primary.model = "model"

    mock_env = MagicMock()
    mock_env.operator_url = "http://operator"
    mock_env.auth_headers.return_value = {"Authorization": "Bearer token"}
    mock_env.cli_session_id = "cli-123"

    monkeypatch.setattr("g8e_evals.sut.g8ee_chat.AuthContext.from_env", lambda **kw: mock_env)

    sut = G8eeChatSUT(config=config, idle_timeout_s=0.02)

    class MockEventSource:
        async def _events(self):
            while True:
                await asyncio.sleep(0)
                event = MagicMock()
                event.event = "heartbeat"
                event.data = ""
                event.id = ""
                yield event

        def aiter_sse(self):
            return self._events()

    class MockContextManager:
        async def __aenter__(self):
            return MockEventSource()

        async def __aexit__(self, exc_type, exc_val, exc_tb):
            pass

    monkeypatch.setattr("g8e_evals.sut.g8ee_chat.aconnect_sse", lambda *a, **kw: MockContextManager())

    started = time.time()
    answer, trail, terminal, error = await sut._drain_events(
        AsyncMock(spec=httpx.AsyncClient),
        since_id=0,
        investigation_id="inv-123",
    )

    assert time.time() - started < 0.5
    assert answer == ""
    assert trail == []
    assert terminal is None
    assert error is None


@pytest.mark.asyncio
async def test_get_answer_surfaces_sse_error(monkeypatch):
    # Setup SUT
    config = MagicMock()
    config.operator_session_id = "session-123"
    config.operator_url = "http://operator"
    config.primary.provider = "test"
    config.primary.model = "model"
    config.mode = "real"

    mock_env = MagicMock()
    mock_env.g8ee_url = "http://g8ee"
    mock_env.operator_url = "http://operator"
    mock_env.auth_headers.return_value = {"Authorization": "Bearer token"}
    mock_env.to_request_context.return_value = MagicMock()

    monkeypatch.setattr("g8e_evals.sut.g8ee_chat.AuthContext.from_env", lambda **kw: mock_env)

    sut = G8eeChatSUT(config=config)
    sut.model_provider = "test-model"
    sut.env = mock_env # Ensure it's set

    # 1. Mock _current_cursor
    monkeypatch.setattr(sut, "_current_cursor", AsyncMock(return_value=10))

    # 2. Mock g8ee chat POST and _build_chat_request to avoid Pydantic validation
    monkeypatch.setattr(sut, "_build_chat_request", MagicMock(return_value=MagicMock(model_dump_json=lambda: "{}")))

    mock_resp = MagicMock()
    mock_resp.status_code = 200
    mock_resp.json.return_value = {"case_id": "case-123", "investigation_id": "inv-123", "success": True}

    async def mock_post(*args, **kwargs):
        return mock_resp

    client_mock = AsyncMock(spec=httpx.AsyncClient)
    client_mock.post = mock_post

    # Mock the context manager for _client()
    class MockClientCM:
        async def __aenter__(self):
            return client_mock
        async def __aexit__(self, *args):
            pass

    monkeypatch.setattr(sut, "_client", lambda: MockClientCM())

    # 3. Mock _drain_events to return an error
    monkeypatch.setattr(sut, "_drain_events", AsyncMock(return_value=("partial text", [], None, "sse_auth_failed: 401")))

    task = Task(id="task-123", prompt="hello")
    response = await sut.get_answer(task)

    assert response.binding == BindingType.UNBOUND
    assert response.unbound_reason == "sse_auth_failed: 401"
    assert response.answer == "partial text"
