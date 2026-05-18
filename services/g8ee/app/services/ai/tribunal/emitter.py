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

import logging
from app.constants import ComponentName, EventType
from app.models.base import G8eBaseModel
from app.models.events import SessionEvent
from app.models.http_context import G8eHttpContext
from app.services.protocols import EventServiceProtocol

logger = logging.getLogger(__name__)

_TERMINAL_TRIBUNAL_EVENTS = {
    EventType.AI_TRIBUNAL_SESSION_STARTED,
    EventType.AI_TRIBUNAL_SESSION_COMPLETED,
    EventType.AI_TRIBUNAL_SESSION_DISABLED,
    EventType.AI_TRIBUNAL_SESSION_MODEL_NOT_CONFIGURED,
    EventType.AI_TRIBUNAL_SESSION_PROVIDER_UNAVAILABLE,
    EventType.AI_TRIBUNAL_SESSION_SYSTEM_ERROR,
    EventType.AI_TRIBUNAL_SESSION_GENERATION_FAILED,
    EventType.AI_TRIBUNAL_SESSION_AUDITOR_FAILED,
}

class TribunalEmitter:
    """Handles emission of Tribunal SSE events via EventService."""

    def __init__(
        self,
        event_service: EventServiceProtocol,
        g8e_context: G8eHttpContext,
        correlation_id: str | None = None,
    ):
        self.event_service = event_service
        self.g8e_context = g8e_context
        self.correlation_id = correlation_id

    async def emit(self, event_type: EventType, payload: G8eBaseModel, correlation_id: str | None = None) -> None:
        """Emit an SSE event. Re-raises if event_type is terminal."""
        try:
            if self.event_service is None or self.g8e_context is None:
                return

            # Inject correlation_id if provided and supported by the payload
            # If not provided to emit, try to use the one stored on the emitter
            corr_id = correlation_id or getattr(self, "correlation_id", None)
            if corr_id and hasattr(payload, "correlation_id"):
                payload.correlation_id = corr_id

            event = SessionEvent(
                event_type=event_type,
                payload=payload,
                web_session_id=self.g8e_context.web_session_id,
                user_id=self.g8e_context.user_id,
                case_id=self.g8e_context.case_id,
                investigation_id=self.g8e_context.investigation_id,
                source_component=ComponentName.G8EE,
            )
            await self.event_service.publish(event)
        except Exception as exc:
            if event_type in _TERMINAL_TRIBUNAL_EVENTS:
                logger.error("[TRIBUNAL-EMIT] Terminal event %s failed: %s", event_type, exc)
                raise
            logger.warning("[TRIBUNAL-EMIT] Progress event %s failed (swallowed): %s", event_type, exc)
