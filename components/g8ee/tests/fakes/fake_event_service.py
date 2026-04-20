# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

from __future__ import annotations

"""Typed fake for EventServiceProtocol."""

from unittest.mock import AsyncMock

from app.constants import EventType
from app.models.events import BackgroundEvent, SessionEvent
from app.services.protocols import EventServiceProtocol


class FakeEventService:
    """Typed fake implementing EventServiceProtocol.

    Records all publish calls for assertion in tests. Does not perform any
    real I/O. Implements the protocol structurally — no inheritance required.
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
        data: "G8eBaseModel",
        g8e_context: "G8eHttpContext",
        *,
        task_id: str,
    ) -> None:
        self.command_events.append({
            "event_type": event_type,
            "data": data,
            "g8e_context": g8e_context,
            "task_id": task_id,
        })
        # Converge command events into the main published list as SessionEvents
        from app.models.events import SessionEvent
        event = SessionEvent(
            event_type=event_type,
            payload=data,
            web_session_id=g8e_context.web_session_id,
            user_id=g8e_context.user_id,
            case_id=g8e_context.case_id,
            investigation_id=g8e_context.investigation_id,
            task_id=task_id,
        )
        # CRITICAL: We MUST call self.publish() which is an AsyncMock.
        # The AsyncMock's side_effect is _record_publish, which appends to self.published.
        await self.publish(event)

    async def publish_investigation_event(
        self,
        investigation_id: str,
        event_type: EventType,
        payload: dict[str, object] | "G8eBaseModel",
        web_session_id: str,
        case_id: str,
        user_id: str,
    ) -> None:
        """Typed fake for publish_investigation_event."""
        # We can just record this as a SessionEvent in self.published
        event = SessionEvent(
            event_type=event_type,
            payload=payload,
            investigation_id=investigation_id,
            web_session_id=web_session_id,
            case_id=case_id,
            user_id=user_id,
        )
        await self.publish(event)


_: EventServiceProtocol = FakeEventService()
