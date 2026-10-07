# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import logging

from app.constants import EventType
from app.models.agents.tribunal import TribunalObserver
from app.models.base import G8eBaseModel
from app.models.events import SessionEvent
from app.models.http_context import G8eHttpContext
from app.models.tool_results import CommandRiskAnalysis
from app.services.protocols import EventServiceProtocol

logger = logging.getLogger(__name__)

_TERMINAL_TRIBUNAL_EVENTS = {
    EventType.AI_CONSENSUS_SESSION_STARTED,
    EventType.AI_CONSENSUS_SESSION_COMPLETED,
    EventType.AI_CONSENSUS_SESSION_DISABLED,
    EventType.AI_CONSENSUS_SESSION_MODEL_NOT_CONFIGURED,
    EventType.AI_CONSENSUS_SESSION_PROVIDER_UNAVAILABLE,
    EventType.AI_CONSENSUS_SESSION_SYSTEM_ERROR,
    EventType.AI_CONSENSUS_SESSION_GENERATION_FAILED,
    EventType.AI_CONSENSUS_SESSION_AUDITOR_FAILED,
}


class TribunalEmitter:
    """Handles emission of Tribunal SSE events via EventService."""

    def __init__(
        self,
        event_service: EventServiceProtocol | None,
        g8e_context: G8eHttpContext | None,
        correlation_id: str | None = None,
        observer: TribunalObserver | None = None,
    ):
        self.event_service = event_service
        self.g8e_context = g8e_context
        self.correlation_id = correlation_id
        self.observer = observer

    def observe_marshal_risk(self, command: str, analysis: CommandRiskAnalysis | None) -> None:
        """Tell the observer what Marshal classified. Marshal publishes no event unless it blocks."""
        if self.observer is None:
            return
        try:
            self.observer.observe_marshal_risk(command, analysis)
        except Exception as exc:
            logger.warning("[TRIBUNAL-EMIT] Observer failed on marshal risk (ignored): %s", exc)

    async def emit(
        self, event_type: EventType, payload: G8eBaseModel, correlation_id: str | None = None
    ) -> None:
        """Emit an SSE event. Re-raises if event_type is terminal."""
        if self.observer is not None:
            try:
                self.observer.observe(event_type, payload)
            except Exception as exc:
                logger.warning(
                    "[TRIBUNAL-EMIT] Observer failed on %s (ignored): %s", event_type, exc
                )
        try:
            if self.event_service is None or self.g8e_context is None:
                return

            # Inject correlation_id if provided and supported by the payload
            # If not provided to emit, try to use the one stored on the emitter
            corr_id = correlation_id or getattr(self, "correlation_id", None)
            if corr_id and "correlation_id" in type(payload).model_fields:
                payload = payload.model_copy(update={"correlation_id": corr_id})

            event = SessionEvent.from_context(
                context=self.g8e_context,
                event_type=event_type,
                payload=payload,
            )
            await self.event_service.publish(event)
        except Exception as exc:
            if event_type in _TERMINAL_TRIBUNAL_EVENTS:
                logger.error("[TRIBUNAL-EMIT] Terminal event %s failed: %s", event_type, exc)
                raise
            logger.warning(
                "[TRIBUNAL-EMIT] Progress event %s failed (swallowed): %s", event_type, exc
            )
