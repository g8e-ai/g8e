# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import logging

from app.constants import EventType, G8EE_COMPONENT
from app.models.events import BackgroundEvent, SessionEvent
from app.services.protocols import EventServiceProtocol, G8eClientProtocol

logger = logging.getLogger(__name__)


class EventService(EventServiceProtocol):
    """Event service for publishing session and background events."""

    def __init__(self, internal_http_client: G8eClientProtocol):
        self._internal_http_client = internal_http_client

    async def publish(self, event: SessionEvent | BackgroundEvent) -> str:
        """Publish a session or background event."""
        try:
            await self._internal_http_client.push_sse_event(event)
            return getattr(event, "id", "event-id")
        except Exception as e:
            logger.error(f"Failed to publish event: {e}")
            return "error-id"

    async def publish_command_event(
        self,
        event_type: EventType,
        data: object,
        g8e_context: object,
        *,
        task_id: str,
    ) -> None:
        """Publish a command-related event."""
        # For now, wrap in a BackgroundEvent and publish
        from app.models.events import BackgroundEvent

        event = BackgroundEvent(
            event_type=event_type,
            payload=data,
            task_id=task_id,
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
