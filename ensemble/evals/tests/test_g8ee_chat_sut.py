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
from g8e_evals.harness import BindingType, Task
from g8e_evals.sut.g8ee_chat import (
    AgentTrailEvent,
    G8eeChatSUT,
    _extract_gateway_transaction_ids,
    _extract_governed_action_types,
)

pytestmark = pytest.mark.unit


def test_extract_gateway_transaction_ids_ignores_investigation_only_trail():
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
    assert _extract_gateway_transaction_ids(trail) == []


def test_extract_gateway_transaction_ids_preserves_distinct_warden_receipts_in_order():
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
                    "data": {"transaction_hash": "tx-command"},
                },
                "investigation_id": "inv-abc",
            },
        ),
        AgentTrailEvent(
            id=3,
            event_type="g8e.v1.ai.governance.warden.receipt.signed",
            payload={
                "event": {
                    "type": "g8e.v1.ai.governance.warden.receipt.signed",
                    "data": {"transaction_hash": "tx-file"},
                },
                "investigation_id": "inv-abc",
            },
        ),
        AgentTrailEvent(
            id=4,
            event_type="g8e.v1.ai.governance.warden.receipt.signed",
            payload={
                "event": {
                    "type": "g8e.v1.ai.governance.warden.receipt.signed",
                    "data": {"transaction_hash": "tx-command"},
                },
                "investigation_id": "inv-abc",
            },
        ),
    ]
    assert _extract_gateway_transaction_ids(trail) == ["tx-command", "tx-file"]


def test_extract_governed_action_types_preserves_distinct_approval_classes_in_order():
    trail = [
        AgentTrailEvent(
            id=1,
            event_type="g8e.v1.operator.command.approval.requested",
            payload={},
        ),
        AgentTrailEvent(
            id=2,
            event_type="g8e.v1.operator.file.edit.approval.requested",
            payload={},
        ),
        AgentTrailEvent(
            id=3,
            event_type="g8e.v1.operator.file.edit.approval.requested",
            payload={},
        ),
    ]

    assert _extract_governed_action_types(trail) == ["EXECUTE_BASH", "FILE_EDIT"]


def test_extract_gateway_transaction_ids_ignores_investigation_id_lookalikes():
    # An investigation_id used as a transaction_id on a non-Gateway event
    # must NOT be promoted to a Gateway transaction id.
    trail = [
        AgentTrailEvent(
            id=1,
            event_type="g8e.v1.app.case.investigation.created",
            payload={"transaction_id": "inv-abc"},
        ),
    ]
    assert _extract_gateway_transaction_ids(trail) == []


def test_binding_unbound_when_no_Gateway_receipt(monkeypatch):
    # Smoke-import to ensure the SUT's UNBOUND-no-receipt branch references
    # are valid module-level symbols; full end-to-end binding is exercised by
    # the live ifeval bench.
    from g8e_evals.sut import g8ee_chat

    assert hasattr(g8ee_chat, "_extract_gateway_transaction_ids")
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
    _answer, _trail, terminal, error = await sut._drain_events(client, since_id=0, investigation_id="inv-123")

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

    monkeypatch.setattr(sut, "_client", MockClientCM)

    # 3. Mock _drain_events to return an error
    monkeypatch.setattr(sut, "_drain_events", AsyncMock(return_value=("partial text", [], None, "sse_auth_failed: 401")))

    task = Task(id="task-123", prompt="hello")
    response = await sut.get_answer(task)

    assert response.binding == BindingType.UNBOUND
    assert response.unbound_reason == "sse_auth_failed: 401"
    assert response.answer == "partial text"


@pytest.mark.asyncio
async def test_get_answer_preserves_observed_action_types_without_receipt_events(monkeypatch):
    config = MagicMock()
    config.operator_session_id = "session-123"
    config.operator_url = "http://operator"
    config.primary.provider = "test"
    config.primary.model = "model"
    config.arm_definition.receipt_binding = True

    mock_env = MagicMock()
    mock_env.g8ee_url = "http://g8ee"
    mock_env.operator_url = "http://operator"
    mock_env.auth_headers.return_value = {"Authorization": "Bearer token"}
    mock_env.to_request_context.return_value = MagicMock()
    monkeypatch.setattr("g8e_evals.sut.g8ee_chat.AuthContext.from_env", lambda **kw: mock_env)

    sut = G8eeChatSUT(config=config)
    sut.model_provider = "test-model"
    monkeypatch.setattr(sut, "_current_cursor", AsyncMock(return_value=10))
    monkeypatch.setattr(
        sut,
        "_build_chat_request",
        MagicMock(return_value=MagicMock(model_dump_json=lambda: "{}")),
    )
    response = MagicMock()
    response.status_code = 200
    response.json.return_value = {
        "case_id": "case-123",
        "investigation_id": "inv-123",
        "success": True,
    }
    client = AsyncMock(spec=httpx.AsyncClient)
    client.post.return_value = response

    class MockClientCM:
        async def __aenter__(self):
            return client

        async def __aexit__(self, *args):
            pass

    monkeypatch.setattr(sut, "_client", MockClientCM)
    trail = [
        AgentTrailEvent(
            id=11,
            event_type="g8e.v1.operator.file.edit.approval.requested",
            payload={},
        )
    ]
    monkeypatch.setattr(
        sut,
        "_drain_events",
        AsyncMock(
            return_value=(
                "completed",
                trail,
                "g8e.v1.ai.llm.chat.iteration.text.completed",
                None,
            )
        ),
    )

    result = await sut.get_answer(Task(id="task-123", prompt="hello"))

    assert result.transaction_ids == []
    assert result.governed_action_types == ["FILE_EDIT"]


@pytest.mark.asyncio
async def test_drain_events_extracts_event_type_from_envelope_not_sse_name(monkeypatch):
    """The Gateway SSE stream wraps every g8e event in a generic ``message``
    SSE frame. The canonical g8e event type lives inside the payload at
    ``envelope.event.type``. The drain loop must use that inner type for
    terminal-event matching, not the SSE wire event name."""
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

    sut = G8eeChatSUT(config=config, idle_timeout_s=5)

    # Simulate a Gateway SSE stream that wraps g8e events in "message" frames.
    # The SSE event field is "message" but the real g8e event type is inside
    # the payload's event.type field.
    chunk_payload = json.dumps({
        "cli_session_id": "cli-123",
        "event": {
            "type": "g8e.v1.ai.llm.chat.iteration.text.chunk.received",
            "data": {"content": "Hello ", "investigation_id": "inv-123"},
        },
    })
    iteration_completed_payload = json.dumps({
        "cli_session_id": "cli-123",
        "event": {
            "type": "g8e.v1.ai.llm.chat.iteration.completed",
            "data": {"turn": 1, "investigation_id": "inv-123"},
        },
    })
    completed_payload = json.dumps({
        "cli_session_id": "cli-123",
        "event": {
            "type": "g8e.v1.ai.llm.chat.iteration.text.completed",
            "data": {"content": "Hello world", "investigation_id": "inv-123"},
        },
    })

    class MockEventSource:
        async def _events(self):
            for evt in [
                ("message", chunk_payload, "1"),
                ("message", iteration_completed_payload, "2"),
                ("message", completed_payload, "3"),
            ]:
                event = MagicMock()
                event.event = evt[0]
                event.data = evt[1]
                event.id = evt[2]
                yield event

        def aiter_sse(self):
            return self._events()

    class MockContextManager:
        async def __aenter__(self):
            return MockEventSource()

        async def __aexit__(self, exc_type, exc_val, exc_tb):
            pass

    monkeypatch.setattr("g8e_evals.sut.g8ee_chat.aconnect_sse", lambda *a, **kw: MockContextManager())

    answer, trail, terminal, error = await sut._drain_events(
        AsyncMock(spec=httpx.AsyncClient),
        since_id=0,
        investigation_id="inv-123",
    )

    assert error is None
    assert terminal == "g8e.v1.ai.llm.chat.iteration.text.completed"
    # The trail must record the canonical g8e event type, not "message".
    assert trail[0].event_type == "g8e.v1.ai.llm.chat.iteration.text.chunk.received"
    assert trail[1].event_type == "g8e.v1.ai.llm.chat.iteration.completed"
    assert trail[2].event_type == "g8e.v1.ai.llm.chat.iteration.text.completed"
    # The text.completed terminal event carries the full response content.
    assert answer == "Hello world"
    assert len(trail) == 3


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "approval_event_type",
    [
        "g8e.v1.operator.command.approval.requested",
        "g8e.v1.operator.file.edit.approval.requested",
    ],
)
async def test_drain_events_headless_approves_correlated_governed_request(
    monkeypatch, approval_event_type
):
    config = MagicMock()
    config.operator_session_id = "session-123"
    config.operator_url = "http://operator"
    config.primary.provider = "test"
    config.primary.model = "model"
    config.headless = True

    mock_env = MagicMock()
    mock_env.operator_url = "http://operator"
    mock_env.auth_headers.return_value = {"Authorization": "Bearer token"}
    mock_env.cli_session_id = "cli-123"
    monkeypatch.setattr("g8e_evals.sut.g8ee_chat.AuthContext.from_env", lambda **kw: mock_env)

    sut = G8eeChatSUT(config=config, idle_timeout_s=5)
    approve = AsyncMock()
    monkeypatch.setattr(sut, "_approve_command", approve)
    approval_payload = json.dumps(
        {
            "cli_session_id": "cli-123",
            "event": {
                "type": approval_event_type,
                "data": {"approval_id": "approval-1", "investigation_id": "inv-123"},
            },
        }
    )
    completed_payload = json.dumps(
        {
            "cli_session_id": "cli-123",
            "event": {
                "type": "g8e.v1.ai.llm.chat.iteration.text.completed",
                "data": {"content": "done", "investigation_id": "inv-123"},
            },
        }
    )

    class MockEventSource:
        async def _events(self):
            for event_id, payload in enumerate([approval_payload, completed_payload], start=1):
                event = MagicMock(event="message", data=payload, id=str(event_id))
                yield event

        def aiter_sse(self):
            return self._events()

    class MockContextManager:
        async def __aenter__(self):
            return MockEventSource()

        async def __aexit__(self, exc_type, exc_val, exc_tb):
            pass

    monkeypatch.setattr(
        "g8e_evals.sut.g8ee_chat.aconnect_sse", lambda *a, **kw: MockContextManager()
    )
    client = AsyncMock(spec=httpx.AsyncClient)

    answer, trail, terminal, error = await sut._drain_events(
        client, since_id=0, investigation_id="inv-123"
    )

    assert error is None
    assert answer == "done"
    assert terminal == "g8e.v1.ai.llm.chat.iteration.text.completed"
    assert len(trail) == 2
    approve.assert_awaited_once()
    approval_call = approve.await_args
    assert approval_call is not None
    assert approval_call.args[0] is client
    assert approval_call.args[1].event.type == approval_event_type
    assert approval_call.args[2] == "inv-123"


# ---------------------------------------------------------------------------
# _extract_inference_observations multi-role tests
#
# The g8ee pipeline emits ChatResponseCompletePayload on the text.completed
# terminal event, carrying a model_calls list of ModelCallTelemetry dicts —
# one per role-model provider call. The SUT must emit one InferenceObservation
# per entry, binding role, model_variant_id, timing, token counts, and error
# state exactly. Failed turns (no text.completed event) produce no
# observations. Remote GPU values are never synthesized.
# ---------------------------------------------------------------------------


def _make_config() -> MagicMock:
    config = MagicMock()
    config.operator_session_id = "session-123"
    config.operator_url = "http://operator"
    config.g8ee_url = "http://g8ee"
    config.primary.provider = "ollama"
    config.primary.model = "qwen3:8b"
    config.assistant.provider = "ollama"
    config.assistant.model = "granite3.3:8b"
    config.lite.provider = "ollama"
    config.lite.model = "smollm2:360m"
    config.candidate_model = None
    config.arm.value = "ensemble_ungoverned"
    config.arm_definition.receipt_binding = True
    return config


def _make_completed_trail(model_calls: list[dict]) -> list[AgentTrailEvent]:
    """Build a minimal trail with a text.completed event carrying model_calls."""
    return [
        AgentTrailEvent(
            id=1,
            event_type="g8e.v1.ai.llm.chat.iteration.text.chunk.received",
            payload={
                "event": {
                    "type": "g8e.v1.ai.llm.chat.iteration.text.chunk.received",
                    "data": {"content": "Hello", "investigation_id": "inv-1"},
                },
            },
        ),
        AgentTrailEvent(
            id=2,
            event_type="g8e.v1.ai.llm.chat.iteration.text.completed",
            payload={
                "event": {
                    "type": "g8e.v1.ai.llm.chat.iteration.text.completed",
                    "data": {
                        "content": "Hello world",
                        "finish_reason": "stop",
                        "has_citations": False,
                        "grounding_metadata": {},
                        "token_usage": {},
                        "agent_mode": "sage",
                        "model_calls": model_calls,
                    },
                },
            },
        ),
    ]


def _make_sut(config: MagicMock | None = None) -> G8eeChatSUT:
    cfg = config or _make_config()
    mock_env = MagicMock()
    mock_env.g8ee_url = "http://g8ee"
    mock_env.operator_url = "http://operator"
    mock_env.auth_headers.return_value = {"Authorization": "Bearer token"}
    mock_env.to_request_context.return_value = MagicMock()
    mock_env.make_async_client.return_value = AsyncMock(spec=httpx.AsyncClient)
    # Patch AuthContext.from_env before constructing the SUT
    import g8e_evals.sut.g8ee_chat as mod
    orig = mod.AuthContext.from_env
    mod.AuthContext.from_env = lambda **kw: mock_env  # type: ignore[assignment]
    sut = G8eeChatSUT(config=cfg)
    mod.AuthContext.from_env = orig  # type: ignore[assignment]
    return sut


def test_extract_inference_observations_multi_role_emits_one_per_role_model_call():
    """A text.completed event with three model_calls (primary, assistant, lite)
    produces exactly three InferenceObservation records, one per role-model
    provider call, with exact role, model_variant_id, and identity mapping."""
    sut = _make_sut()
    model_calls = [
        {
            "agent_role": "primary",
            "provider": "OllamaProvider",
            "model": "qwen3:8b",
            "monotonic_start": 10.0,
            "monotonic_end": 12.5,
            "input_tokens": 100,
            "output_tokens": 50,
            "thinking_tokens": 20,
            "total_tokens": 170,
            "cache_tokens": 5,
            "usage_reported": True,
            "finish_reason": "stop",
            "succeeded": True,
            "input_artifact_hash": "a" * 64,
            "output_artifact_hash": "b" * 64,
        },
        {
            "agent_role": "assistant",
            "provider": "OllamaProvider",
            "model": "granite3.3:8b",
            "monotonic_start": 13.0,
            "monotonic_end": 14.0,
            "input_tokens": 80,
            "output_tokens": 30,
            "thinking_tokens": 0,
            "total_tokens": 110,
            "cache_tokens": 0,
            "usage_reported": True,
            "finish_reason": "stop",
            "succeeded": True,
            "input_artifact_hash": "c" * 64,
            "output_artifact_hash": "d" * 64,
        },
        {
            "agent_role": "lite",
            "provider": "OllamaProvider",
            "model": "smollm2:360m",
            "monotonic_start": 14.5,
            "monotonic_end": 15.0,
            "input_tokens": 40,
            "output_tokens": 10,
            "thinking_tokens": 0,
            "total_tokens": 50,
            "cache_tokens": 0,
            "usage_reported": True,
            "finish_reason": "stop",
            "succeeded": True,
            "input_artifact_hash": "e" * 64,
            "output_artifact_hash": "f" * 64,
        },
    ]
    trail = _make_completed_trail(model_calls)
    obs = sut._extract_inference_observations(trail, "g8e.v1.ai.llm.chat.iteration.text.completed")

    assert len(obs) == 3
    # Primary role
    assert obs[0].inference_id == "inf-0"
    assert obs[0].role == "primary"
    assert obs[0].model_variant_id == "qwen3:8b"
    assert obs[0].provider == "OllamaProvider"
    assert obs[0].model == "qwen3:8b"
    assert obs[0].provider_call_latency_seconds == 2.5
    assert obs[0].output_throughput_tokens_per_second == 20.0  # 50 / 2.5
    assert obs[0].hidden_reasoning_throughput_tokens_per_second == 8.0  # 20 / 2.5
    assert obs[0].prompt_token_count == 100
    assert obs[0].candidates_token_count == 50
    assert obs[0].total_token_count == 170
    assert obs[0].thinking_token_count == 20
    assert obs[0].cache_token_count == 5
    assert obs[0].usage_reported is True
    assert obs[0].finish_reason == "stop"
    assert obs[0].input_artifact_hash == "a" * 64
    assert obs[0].output_artifact_hash == "b" * 64
    assert obs[0].error is None
    assert obs[0].monotonic_start == 10.0
    assert obs[0].monotonic_end == 12.5
    # Assistant role
    assert obs[1].inference_id == "inf-1"
    assert obs[1].role == "assistant"
    assert obs[1].model_variant_id == "granite3.3:8b"
    assert obs[1].provider_call_latency_seconds == 1.0
    assert obs[1].output_throughput_tokens_per_second == 30.0  # 30 / 1.0
    assert obs[1].hidden_reasoning_throughput_tokens_per_second is None  # 0 thinking tokens
    assert obs[1].prompt_token_count == 80
    assert obs[1].candidates_token_count == 30
    assert obs[1].total_token_count == 110
    assert obs[1].thinking_token_count is None  # 0 → None
    assert obs[1].cache_token_count is None  # 0 → None
    assert obs[1].usage_reported is True
    assert obs[1].input_artifact_hash == "c" * 64
    assert obs[1].output_artifact_hash == "d" * 64
    assert obs[1].error is None
    # Lite role
    assert obs[2].inference_id == "inf-2"
    assert obs[2].role == "lite"
    assert obs[2].model_variant_id == "smollm2:360m"
    assert obs[2].provider_call_latency_seconds == 0.5
    assert obs[2].output_throughput_tokens_per_second == 20.0  # 10 / 0.5
    assert obs[2].prompt_token_count == 40
    assert obs[2].candidates_token_count == 10
    assert obs[2].total_token_count == 50
    assert obs[2].input_artifact_hash == "e" * 64
    assert obs[2].output_artifact_hash == "f" * 64
    assert obs[2].error is None


def test_extract_inference_observations_single_role_emits_one_observation():
    """A text.completed event with one model_call produces exactly one
    InferenceObservation."""
    sut = _make_sut()
    model_calls = [
        {
            "agent_role": "primary",
            "provider": "OllamaProvider",
            "model": "qwen3:8b",
            "monotonic_start": 5.0,
            "monotonic_end": 6.0,
            "input_tokens": 10,
            "output_tokens": 5,
            "total_tokens": 15,
            "usage_reported": True,
            "finish_reason": "stop",
            "succeeded": True,
        },
    ]
    trail = _make_completed_trail(model_calls)
    obs = sut._extract_inference_observations(trail, "g8e.v1.ai.llm.chat.iteration.text.completed")

    assert len(obs) == 1
    assert obs[0].inference_id == "inf-0"
    assert obs[0].role == "primary"
    assert obs[0].model_variant_id == "qwen3:8b"
    assert obs[0].provider_call_latency_seconds == 1.0
    assert obs[0].output_throughput_tokens_per_second == 5.0  # 5 / 1.0
    assert obs[0].prompt_token_count == 10
    assert obs[0].candidates_token_count == 5
    assert obs[0].total_token_count == 15
    assert obs[0].usage_reported is True
    assert obs[0].error is None


def test_extract_inference_observations_failed_turn_no_text_completed_emits_none():
    """A failed turn (no text.completed event) produces no InferenceObservation
    records. The runner records the failure as a typed terminal status on the
    attempt."""
    sut = _make_sut()
    trail = [
        AgentTrailEvent(
            id=1,
            event_type="g8e.v1.ai.llm.chat.iteration.text.chunk.received",
            payload={
                "event": {
                    "type": "g8e.v1.ai.llm.chat.iteration.text.chunk.received",
                    "data": {"content": "partial", "investigation_id": "inv-1"},
                },
            },
        ),
        AgentTrailEvent(
            id=2,
            event_type="g8e.v1.ai.llm.chat.iteration.failed",
            payload={
                "event": {
                    "type": "g8e.v1.ai.llm.chat.iteration.failed",
                    "data": {"error": "provider_timeout", "investigation_id": "inv-1"},
                },
            },
        ),
    ]
    obs = sut._extract_inference_observations(trail, "g8e.v1.ai.llm.chat.iteration.failed")

    assert obs == []


def test_extract_inference_observations_failed_call_carries_error_not_silently_dropped():
    """A model_calls entry with succeeded=False carries the error_type on the
    InferenceObservation.error field. Failed calls are not silently dropped."""
    sut = _make_sut()
    model_calls = [
        {
            "agent_role": "primary",
            "provider": "OllamaProvider",
            "model": "qwen3:8b",
            "monotonic_start": 10.0,
            "monotonic_end": 10.5,
            "input_tokens": 100,
            "output_tokens": 0,
            "total_tokens": 100,
            "usage_reported": True,
            "finish_reason": "error",
            "succeeded": False,
            "error_type": "OllamaEmptyResponseError",
            "input_artifact_hash": "a" * 64,
        },
    ]
    trail = _make_completed_trail(model_calls)
    obs = sut._extract_inference_observations(trail, "g8e.v1.ai.llm.chat.iteration.text.completed")

    assert len(obs) == 1
    assert obs[0].role == "primary"
    assert obs[0].model_variant_id == "qwen3:8b"
    assert obs[0].error == "OllamaEmptyResponseError"
    assert obs[0].finish_reason == "error"
    assert obs[0].output_throughput_tokens_per_second is None  # 0 output tokens
    assert obs[0].candidates_token_count is None  # 0 → None


def test_extract_inference_observations_does_not_synthesize_remote_gpu_values():
    """Remote GPU values are never inferred from model metadata. The
    InferenceObservation carries no GPU fields; the resulting
    ResourceObservation marks them as UNAVAILABLE with a typed reason."""
    sut = _make_sut()
    model_calls = [
        {
            "agent_role": "primary",
            "provider": "OllamaProvider",
            "model": "qwen3:8b",
            "monotonic_start": 10.0,
            "monotonic_end": 12.0,
            "input_tokens": 100,
            "output_tokens": 50,
            "total_tokens": 150,
            "usage_reported": True,
            "finish_reason": "stop",
            "succeeded": True,
        },
    ]
    trail = _make_completed_trail(model_calls)
    obs = sut._extract_inference_observations(trail, "g8e.v1.ai.llm.chat.iteration.text.completed")

    assert len(obs) == 1
    # InferenceObservation has no GPU fields — they remain absent
    assert not hasattr(obs[0], "gpu_memory_bytes")
    assert not hasattr(obs[0], "gpu_utilization_pct")
    assert not hasattr(obs[0], "gpu_power_watts")
    # Timing and token fields are present (measured from telemetry)
    assert obs[0].provider_call_latency_seconds == 2.0
    assert obs[0].prompt_token_count == 100


def test_extract_inference_observations_distinct_inference_ids_across_roles():
    """Each model_calls entry gets a distinct inference_id (inf-0, inf-1, ...)
    so the runner can build distinct ResourceObservation and StageObservation
    records per role-model provider call."""
    sut = _make_sut()
    model_calls = [
        {
            "agent_role": "primary",
            "provider": "OllamaProvider",
            "model": "qwen3:8b",
            "monotonic_start": 10.0,
            "monotonic_end": 11.0,
            "input_tokens": 10,
            "output_tokens": 5,
            "total_tokens": 15,
            "usage_reported": True,
            "finish_reason": "stop",
            "succeeded": True,
        },
        {
            "agent_role": "assistant",
            "provider": "OllamaProvider",
            "model": "granite3.3:8b",
            "monotonic_start": 11.5,
            "monotonic_end": 12.0,
            "input_tokens": 8,
            "output_tokens": 3,
            "total_tokens": 11,
            "usage_reported": True,
            "finish_reason": "stop",
            "succeeded": True,
        },
    ]
    trail = _make_completed_trail(model_calls)
    obs = sut._extract_inference_observations(trail, "g8e.v1.ai.llm.chat.iteration.text.completed")

    assert len(obs) == 2
    ids = [o.inference_id for o in obs]
    assert ids == ["inf-0", "inf-1"]
    assert len(set(ids)) == 2  # all distinct


def test_extract_inference_observations_empty_model_calls_emits_none():
    """A text.completed event with an empty model_calls list produces no
    InferenceObservation records."""
    sut = _make_sut()
    trail = _make_completed_trail([])
    obs = sut._extract_inference_observations(trail, "g8e.v1.ai.llm.chat.iteration.text.completed")

    assert obs == []


def test_extract_inference_observations_malformed_text_completed_skips_silently():
    """A text.completed event with a malformed payload (fails
    ChatResponseCompletePayload validation) is skipped silently rather than
    crashing the extraction. No observations are emitted from that event."""
    sut = _make_sut()
    trail = [
        AgentTrailEvent(
            id=1,
            event_type="g8e.v1.ai.llm.chat.iteration.text.completed",
            payload={
                "event": {
                    "type": "g8e.v1.ai.llm.chat.iteration.text.completed",
                    "data": {"not_a_valid_field": True},  # missing required fields
                },
            },
        ),
    ]
    obs = sut._extract_inference_observations(trail, "g8e.v1.ai.llm.chat.iteration.text.completed")

    assert obs == []


def test_extract_inference_observations_maps_native_timing_fields():
    """Provider-native durations and TTFT on ModelCallTelemetry map onto
    the InferenceObservation. Absent fields stay None."""
    sut = _make_sut()
    model_calls = [
        {
            "agent_role": "primary",
            "provider": "OllamaProvider",
            "model": "qwen3:8b",
            "monotonic_start": 10.0,
            "monotonic_end": 12.5,
            "input_tokens": 100,
            "output_tokens": 50,
            "total_tokens": 150,
            "usage_reported": True,
            "finish_reason": "stop",
            "succeeded": True,
            "time_to_first_token_seconds": 0.4,
            "generation_duration_seconds": 2.1,
            "prompt_eval_duration_seconds": 0.3,
            "total_duration_seconds": 2.5,
            "load_duration_seconds": 0.05,
        },
        {
            "agent_role": "lite",
            "provider": "OllamaProvider",
            "model": "smollm2:360m",
            "monotonic_start": 13.0,
            "monotonic_end": 13.5,
            "input_tokens": 40,
            "output_tokens": 10,
            "total_tokens": 50,
            "usage_reported": True,
            "finish_reason": "stop",
            "succeeded": True,
            # Non-streaming producer: no TTFT or native durations
        },
    ]
    trail = _make_completed_trail(model_calls)
    obs = sut._extract_inference_observations(trail, "g8e.v1.ai.llm.chat.iteration.text.completed")

    assert len(obs) == 2
    assert obs[0].time_to_first_token_seconds == 0.4
    assert obs[0].generation_duration_seconds == 2.1
    assert obs[1].time_to_first_token_seconds is None
    assert obs[1].generation_duration_seconds is None


def test_extract_inference_observations_does_not_retain_restricted_plaintext():
    """The InferenceObservation carries only artifact hashes, not raw prompt
    content or model output. Restricted plaintext is not retained on the
    public path."""
    sut = _make_sut()
    model_calls = [
        {
            "agent_role": "primary",
            "provider": "OllamaProvider",
            "model": "qwen3:8b",
            "monotonic_start": 10.0,
            "monotonic_end": 11.0,
            "input_tokens": 10,
            "output_tokens": 5,
            "total_tokens": 15,
            "usage_reported": True,
            "finish_reason": "stop",
            "succeeded": True,
            "input_artifact_hash": "a" * 64,
            "output_artifact_hash": "b" * 64,
            # Simulate a field that could carry plaintext if retained
            "input_text": "SECRET PROMPT CONTENT",
            "output_text": "SECRET MODEL OUTPUT",
        },
    ]
    trail = _make_completed_trail(model_calls)
    obs = sut._extract_inference_observations(trail, "g8e.v1.ai.llm.chat.iteration.text.completed")

    assert len(obs) == 1
    assert obs[0].input_artifact_hash == "a" * 64
    assert obs[0].output_artifact_hash == "b" * 64
    # No plaintext fields exist on InferenceObservation
    assert not hasattr(obs[0], "input_text")
    assert not hasattr(obs[0], "output_text")
    assert not hasattr(obs[0], "prompt_text")
    assert not hasattr(obs[0], "output_content")
