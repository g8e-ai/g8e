# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import asyncio
import logging

from app.constants import EventType, G8EE_COMPONENT
from app.models.base import G8eBaseModel
from app.models.events import BackgroundEvent, SessionEvent
from app.models.http_context import G8eHttpContext, RequestContext
from app.models.internal_api import (
    ObserveProducerAgentStateRequest,
    ObserveProducerRunStateRequest,
)
from app.services.protocols import EventServiceProtocol, G8eClientProtocol

logger = logging.getLogger(__name__)


class EventService(EventServiceProtocol):
    """Event service for publishing session and background events."""

    def __init__(self, internal_http_client: G8eClientProtocol):
        self._internal_http_client = internal_http_client

    async def publish(self, event: SessionEvent | BackgroundEvent) -> str:
        """Publish a session or background event.

        Raises the underlying exception on failure so callers can propagate
        push errors to the user instead of silently logging success.

        Events with no routing target (no web_session_id and no cli_session_id)
        are skipped: the gateway's SSE push endpoint requires exactly one
        routing target and rejects targetless events with 400, which trips
        the circuit breaker and blocks all subsequent SSE pushes (including
        approval requests for file edits). There is no connected client to
        deliver to when there is no routing target, so skipping is correct.
        """
        if not getattr(event, "web_session_id", None) and not getattr(event, "cli_session_id", None):
            logger.debug(
                "Skipping SSE push for targetless %s (event_type=%s)",
                type(event).__name__,
                getattr(event, "event_type", "unknown"),
            )
            return getattr(event, "id", "event-id")
        await self._internal_http_client.push_sse_event(event)
        return getattr(event, "id", "event-id")

    async def publish_reputation_event(
        self,
        event_type: EventType,
        payload: G8eBaseModel,
        g8e_context: G8eHttpContext,
    ) -> None:
        event = SessionEvent.from_context(
            context=RequestContext.from_app_context(g8e_context),
            event_type=event_type,
            payload=payload,
        )
        await self.publish(event)

    async def publish_investigation_event(
        self,
        investigation_id: str,
        event_type: EventType,
        payload: object,
        web_session_id: str | None,
        case_id: str,
        user_id: str,
        *,
        cli_session_id: str | None = None,
    ) -> None:
        """Publish an investigation-related event."""
        from app.models.events import SessionEvent
        from app.models.http_context import RequestContext


        ctx = RequestContext(
            web_session_id=web_session_id,
            cli_session_id=cli_session_id,
            user_id=user_id,
            case_id=case_id,
            investigation_id=investigation_id,
            source_component=G8EE_COMPONENT,
        )
        event = SessionEvent.from_context(
            context=ctx,
            event_type=event_type,
            payload=payload,
        )
        await self.publish(event)

    async def publish_agent_state(
        self, request: ObserveProducerAgentStateRequest
    ) -> None:
        """Best-effort agent-state projection push to the gateway observe producer.

        Catches transport/network failures from the low-level client, logs one
        warning with safe identifiers, and returns without raising. Cancellation
        (asyncio.CancelledError) continues to propagate. Targetless requests
        (no web_session_id and no cli_session_id) are skipped, mirroring the
        SSE targetless-skip contract.
        """
        if not request.web_session_id and not request.cli_session_id:
            logger.debug(
                "Skipping observe agent-state push for targetless request "
                "(agent_id=%s, status=%s)",
                request.agent_id,
                request.status,
            )
            return
        try:
            await self._internal_http_client.push_agent_state(request)
        except asyncio.CancelledError:
            raise
        except Exception as exc:
            logger.warning(
                "observe agent-state push failed (non-blocking): %s",
                exc,
                extra={
                    "agent_id": request.agent_id,
                    "status": request.status,
                },
            )

    async def publish_run_state(
        self, request: ObserveProducerRunStateRequest
    ) -> None:
        """Best-effort run-state projection push to the gateway observe producer.

        Same best-effort and targetless-skip semantics as publish_agent_state.
        """
        if not request.web_session_id and not request.cli_session_id:
            logger.debug(
                "Skipping observe run-state push for targetless request "
                "(run_id=%s, status=%s)",
                request.run_id,
                request.status,
            )
            return
        try:
            await self._internal_http_client.push_run_state(request)
        except asyncio.CancelledError:
            raise
        except Exception as exc:
            logger.warning(
                "observe run-state push failed (non-blocking): %s",
                exc,
                extra={
                    "run_id": request.run_id,
                    "status": request.status,
                },
            )
