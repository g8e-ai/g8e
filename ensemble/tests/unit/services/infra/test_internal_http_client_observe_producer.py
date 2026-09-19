# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software
# is released under the Apache License, Version 2.0.

"""Unit tests for the low-level InternalHttpClient producer methods
(push_agent_state, push_run_state).

These tests verify exact path, typed request body, success parsing, non-2xx
NetworkError, transport-exception wrapping, and mTLS enforcement. They do
not touch the network; the underlying HTTPClient is stubbed.
"""

from __future__ import annotations

from unittest.mock import AsyncMock, MagicMock

import pytest

from app.constants import G8EE_COMPONENT
from app.errors import NetworkError
from app.models.internal_api import (
    ObserveProducerAgentStateRequest,
    ObserveProducerRunStateRequest,
    ObserveProducerResponse,
)
from app.services.infra.internal_http_client import InternalHttpClient


pytestmark = pytest.mark.unit


def _make_client() -> InternalHttpClient:
    """Build an InternalHttpClient with a stubbed HTTPClient and settings."""
    settings = MagicMock()
    settings.component_urls.client_url = "https://client.local"
    settings.ca_cert_path = None
    settings.client_cert_path = None
    settings.client_key_path = None
    settings.auth.internal_api_key = None
    client = InternalHttpClient(settings)
    return client


def _agent_request() -> ObserveProducerAgentStateRequest:
    return ObserveProducerAgentStateRequest(
        schema_version="1.0.0",
        agent_id="user-1:triage",
        display_name="Triage",
        role="classifier",
        status="running",
        run_id="inv-1",
        task_id="task-1",
        model="test-model",
        observed_at="2026-09-09T12:00:00Z",
        web_session_id="web-session-1",
    )


def _run_request() -> ObserveProducerRunStateRequest:
    return ObserveProducerRunStateRequest(
        schema_version="1.0.0",
        run_id="inv-1",
        run_kind="investigation",
        display_name="Investigation One",
        status="running",
        completed_tasks=0,
        total_tasks=0,
        observed_at="2026-09-09T12:00:00Z",
        web_session_id="web-session-1",
    )


@pytest.mark.asyncio
async def test_push_agent_state_posts_typed_request_to_producer_path_and_parses_response():
    client = _make_client()
    response = MagicMock()
    response.is_success = True
    response.json.return_value = {"accepted": True}
    client._http.post = AsyncMock(return_value=response)

    result = await client.push_agent_state(_agent_request())

    assert isinstance(result, ObserveProducerResponse)
    assert result.accepted is True
    client._http.post.assert_awaited_once()
    path = client._http.post.await_args.args[0]
    assert path == "/api/v1/observe/producer/agent-state"
    sent = client._http.post.await_args.kwargs["json_data"]
    assert isinstance(sent, ObserveProducerAgentStateRequest)
    assert sent.agent_id == "user-1:triage"
    assert sent.status == "running"


@pytest.mark.asyncio
async def test_push_run_state_posts_typed_request_to_producer_path_and_parses_response():
    client = _make_client()
    response = MagicMock()
    response.is_success = True
    response.json.return_value = {"accepted": True}
    client._http.post = AsyncMock(return_value=response)

    result = await client.push_run_state(_run_request())

    assert isinstance(result, ObserveProducerResponse)
    assert result.accepted is True
    path = client._http.post.await_args.args[0]
    assert path == "/api/v1/observe/producer/run-state"
    sent = client._http.post.await_args.kwargs["json_data"]
    assert isinstance(sent, ObserveProducerRunStateRequest)
    assert sent.run_id == "inv-1"
    assert sent.run_kind == "investigation"


@pytest.mark.asyncio
async def test_push_agent_state_raises_network_error_on_non_2xx_with_status_preserved():
    client = _make_client()
    response = MagicMock()
    response.is_success = False
    response.status_code = 400
    response.text = "bad request"
    client._http.post = AsyncMock(return_value=response)

    with pytest.raises(NetworkError) as exc_info:
        await client.push_agent_state(_agent_request())

    assert exc_info.value.error_detail.details["status_code"] == 400
    assert "agent_id" in exc_info.value.error_detail.details


@pytest.mark.asyncio
async def test_push_run_state_raises_network_error_on_non_2xx_with_status_preserved():
    client = _make_client()
    response = MagicMock()
    response.is_success = False
    response.status_code = 403
    response.text = "forbidden"
    client._http.post = AsyncMock(return_value=response)

    with pytest.raises(NetworkError) as exc_info:
        await client.push_run_state(_run_request())

    assert exc_info.value.error_detail.details["status_code"] == 403
    assert "run_id" in exc_info.value.error_detail.details


@pytest.mark.asyncio
async def test_push_agent_state_wraps_transport_exception_in_network_error():
    client = _make_client()
    client._http.post = AsyncMock(side_effect=ConnectionError("reset"))

    with pytest.raises(NetworkError) as exc_info:
        await client.push_agent_state(_agent_request())

    assert exc_info.value.component == G8EE_COMPONENT


@pytest.mark.asyncio
async def test_push_run_state_wraps_transport_exception_in_network_error():
    client = _make_client()
    client._http.post = AsyncMock(side_effect=ConnectionError("reset"))

    with pytest.raises(NetworkError):
        await client.push_run_state(_run_request())


@pytest.mark.asyncio
async def test_push_agent_state_ensures_mtls_before_request():
    client = _make_client()
    response = MagicMock()
    response.is_success = True
    response.json.return_value = {"accepted": True}
    client._http.post = AsyncMock(return_value=response)
    client._ensure_mtls = MagicMock()

    await client.push_agent_state(_agent_request())

    client._ensure_mtls.assert_called_once()


@pytest.mark.asyncio
async def test_push_run_state_ensures_mtls_before_request():
    client = _make_client()
    response = MagicMock()
    response.is_success = True
    response.json.return_value = {"accepted": True}
    client._http.post = AsyncMock(return_value=response)
    client._ensure_mtls = MagicMock()

    await client.push_run_state(_run_request())

    client._ensure_mtls.assert_called_once()
