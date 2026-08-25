# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

"""Typed fake for EventServiceProtocol."""

from unittest.mock import AsyncMock

from app.constants import EventType, G8EE_COMPONENT
from app.models.events import BackgroundEvent, SessionEvent


class FakeEventService:
    """Typed fake implementing EventServiceProtocol.

    Records all publish calls for assertion in tests. Does not perform any
    real I/O. Implements the protocol structurally - no inheritance required.
    """

    def __init__(self) -> None:
        self.published: list[SessionEvent | BackgroundEvent] = []
        self.command_events: list[dict] = []

        # Initialize as a proper AsyncMock for call assertions
        # We manually record to self.published in the side_effect
        self.publish = AsyncMock(side_effect=self._record_publish)

    async def _record_publish(self, event: SessionEvent | BackgroundEvent) -> str:
        """Internal functional implementation."""
        # Ensure it's a SessionEvent before appending to self.published
        # (Though in our fakes they usually are)
        if isinstance(event, (SessionEvent, BackgroundEvent)):
            self.published.append(event)
        return "fake-publish-id"

    async def publish_command_event(
        self,
        event_type: EventType,
        data: G8eBaseModel,
        g8e_context: G8eHttpContext,
        *,
        task_id: str,
    ) -> None:
        self.command_events.append(
            {
                "event_type": event_type,
                "data": data,
                "g8e_context": g8e_context,
                "task_id": task_id,
            }
        )
        # Converge command events into the main published list as SessionEvents
        from app.models.events import SessionEvent

        event = SessionEvent.from_context(
            context=g8e_context,
            event_type=event_type,
            payload=data,
        )
        # CRITICAL: We MUST call self.publish() which is an AsyncMock.
        # The AsyncMock's side_effect is _record_publish, which appends to self.published.
        await self.publish(event)

    async def publish_investigation_event(
        self,
        investigation_id: str,
        event_type: EventType,
        payload: dict[str, object] | G8eBaseModel,
        web_session_id: str | None,
        case_id: str,
        user_id: str,
        *,
        cli_session_id: str | None = None,
    ) -> None:
        """Typed fake for publish_investigation_event."""
        # We can just record this as a SessionEvent in self.published
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
