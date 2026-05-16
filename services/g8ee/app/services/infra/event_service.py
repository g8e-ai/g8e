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

import logging

from app.constants import EventType
from app.models.events import BackgroundEvent, SessionEvent
from app.services.protocols import EventServiceProtocol

logger = logging.getLogger(__name__)


class EventService(EventServiceProtocol):
    """Event service for publishing session and background events."""

    def __init__(self, internal_http_client: object):
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
        event = SessionEvent(
            event_type=event_type,
            payload=payload,
            web_session_id=web_session_id,
            cli_session_id=cli_session_id,
            investigation_id=investigation_id,
            case_id=case_id,
            user_id=user_id,
        )
        await self.publish(event)
