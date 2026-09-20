# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software
# is released under the Apache License, Version 2.0.

"""Unit tests for the best-effort EventService producer publish methods
(publish_agent_state, publish_run_state).

Verifies successful forwarding, warning-and-return on failure, cancellation
propagation, and targetless skip.
"""

from __future__ import annotations

import asyncio
from unittest.mock import AsyncMock

import pytest

from app.errors import NetworkError
from app.models.internal_api import (
    ObserveProducerAgentStateRequest,
    ObserveProducerRunStateRequest,
)
from app.services.infra.event_service import EventService


pytestmark = pytest.mark.unit


def _agent_request(*, web_session_id="web-1", cli_session_id=None):
    return ObserveProducerAgentStateRequest(
        schema_version="1.0.0",
        agent_id="user-1:triage",
        display_name="Triage",
        role="classifier",
        status="running",
        observed_at="2026-09-09T12:00:00Z",
        web_session_id=web_session_id,
        cli_session_id=cli_session_id,
    )


def _run_request(*, web_session_id="web-1", cli_session_id=None):
    return ObserveProducerRunStateRequest(
        schema_version="1.0.0",
        run_id="inv-1",
        run_kind="investigation",
        display_name="Investigation One",
        status="running",
        completed_tasks=0,
        total_tasks=0,
        observed_at="2026-09-09T12:00:00Z",
        web_session_id=web_session_id,
        cli_session_id=cli_session_id,
    )


@pytest.mark.asyncio
async def test_publish_agent_state_forwards_to_low_level_client_on_success():
    http_client = AsyncMock()
    service = EventService(http_client)

    await service.publish_agent_state(_agent_request())

    http_client.push_agent_state.assert_awaited_once()


@pytest.mark.asyncio
async def test_publish_run_state_forwards_to_low_level_client_on_success():
    http_client = AsyncMock()
    service = EventService(http_client)

    await service.publish_run_state(_run_request())

    http_client.push_run_state.assert_awaited_once()


@pytest.mark.asyncio
async def test_publish_agent_state_warns_and_returns_on_network_error():
    http_client = AsyncMock()
    http_client.push_agent_state = AsyncMock(side_effect=NetworkError("boom"))
    service = EventService(http_client)

    # Must not raise.
    await service.publish_agent_state(_agent_request())

    http_client.push_agent_state.assert_awaited_once()


@pytest.mark.asyncio
async def test_publish_run_state_warns_and_returns_on_network_error():
    http_client = AsyncMock()
    http_client.push_run_state = AsyncMock(side_effect=NetworkError("boom"))
    service = EventService(http_client)

    await service.publish_run_state(_run_request())

    http_client.push_run_state.assert_awaited_once()


@pytest.mark.asyncio
async def test_publish_agent_state_warns_and_returns_on_generic_exception():
    http_client = AsyncMock()
    http_client.push_agent_state = AsyncMock(side_effect=RuntimeError("oops"))
    service = EventService(http_client)

    await service.publish_agent_state(_agent_request())

    http_client.push_agent_state.assert_awaited_once()


@pytest.mark.asyncio
async def test_publish_run_state_warns_and_returns_on_generic_exception():
    http_client = AsyncMock()
    http_client.push_run_state = AsyncMock(side_effect=RuntimeError("oops"))
    service = EventService(http_client)

    await service.publish_run_state(_run_request())

    http_client.push_run_state.assert_awaited_once()


@pytest.mark.asyncio
async def test_publish_agent_state_propagates_cancellation():
    http_client = AsyncMock()

    async def raise_cancelled(_request):
        raise asyncio.CancelledError

    http_client.push_agent_state = AsyncMock(side_effect=raise_cancelled)
    service = EventService(http_client)

    with pytest.raises(asyncio.CancelledError):
        await service.publish_agent_state(_agent_request())


@pytest.mark.asyncio
async def test_publish_run_state_propagates_cancellation():
    http_client = AsyncMock()

    async def raise_cancelled(_request):
        raise asyncio.CancelledError

    http_client.push_run_state = AsyncMock(side_effect=raise_cancelled)
    service = EventService(http_client)

    with pytest.raises(asyncio.CancelledError):
        await service.publish_run_state(_run_request())


@pytest.mark.asyncio
async def test_publish_agent_state_skips_targetless_request_without_calling_client():
    http_client = AsyncMock()
    service = EventService(http_client)

    await service.publish_agent_state(_agent_request(web_session_id=None, cli_session_id=None))

    http_client.push_agent_state.assert_not_awaited()


@pytest.mark.asyncio
async def test_publish_run_state_skips_targetless_request_without_calling_client():
    http_client = AsyncMock()
    service = EventService(http_client)

    await service.publish_run_state(_run_request(web_session_id=None, cli_session_id=None))

    http_client.push_run_state.assert_not_awaited()


@pytest.mark.asyncio
async def test_publish_agent_state_forwards_cli_routed_request():
    http_client = AsyncMock()
    service = EventService(http_client)

    await service.publish_agent_state(_agent_request(web_session_id=None, cli_session_id="cli-1"))

    http_client.push_agent_state.assert_awaited_once()


@pytest.mark.asyncio
async def test_publish_run_state_forwards_cli_routed_request():
    http_client = AsyncMock()
    service = EventService(http_client)

    await service.publish_run_state(_run_request(web_session_id=None, cli_session_id="cli-1"))

    http_client.push_run_state.assert_awaited_once()
