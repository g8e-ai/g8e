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

"""
Typed client publish events.

All events published to client are represented as one of two typed envelopes:

- SessionEvent: routes to a specific connected browser session. web_session_id,
  case_id, and investigation_id are all required — the caller must have all
  three or the event cannot be constructed.

- BackgroundEvent: system-initiated, no browser session. client fans the event
  out to every active SSE session owned by user_id. investigation_id and
  case_id are optional correlation hints carried inside the payload.

The EventService.publish() method accepts the union type and dispatches
accordingly. publish_event_to_client() no longer exists.
"""

from typing import Any

from app.constants import EventType, ToolCallStatus
from app.models.base import G8eBaseModel, Field, UTCDatetime


class SessionEvent(G8eBaseModel):
    """Event that must reach a specific connected browser session.

    Use this when the triggering request arrived on a known web_session_id
    (approval requests, command broadcasts, AI stream events, etc.).
    """

    event_type: EventType = Field(description="client event type")
    payload: G8eBaseModel = Field(description="Typed event-specific payload")
    web_session_id: str = Field(description="Browser session to deliver to")
    user_id: str = Field(description="User ID associated with the session")
    case_id: str | None = Field(default=None, description="Case correlation ID")
    investigation_id: str | None = Field(default=None, description="Investigation correlation ID")
    task_id: str | None = Field(default=None, description="AI task ID for routing")


class BackgroundEvent(G8eBaseModel):
    """System-initiated event with no connected browser session.

    client fans the event out to every active SSE session owned by user_id.
    investigation_id and case_id are optional correlation hints carried inside
    the payload — they do not drive routing.
    """

    event_type: EventType = Field(description="client event type")
    payload: G8eBaseModel = Field(description="Typed event-specific payload")
    user_id: str = Field(description="User ID to fan out to")
    investigation_id: str | None = Field(default=None, description="Investigation correlation ID")
    case_id: str | None = Field(default=None, description="Case correlation ID")
    task_id: str | None = Field(default=None, description="AI task ID for routing")


class _SSEEventBody(G8eBaseModel):
    type: EventType
    data: dict[str, Any]


class SessionEventWire(G8eBaseModel):
    web_session_id: str
    user_id: str
    event: _SSEEventBody

    @classmethod
    def from_session_event(cls, se: SessionEvent) -> "SessionEventWire":
        data = se.payload.model_dump(mode="json")
        data["web_session_id"] = se.web_session_id
        data["user_id"] = se.user_id
        if se.case_id is not None:
            data["case_id"] = se.case_id
        if se.investigation_id is not None:
            data["investigation_id"] = se.investigation_id
        if se.task_id is not None:
            data["task_id"] = se.task_id
        return cls(
            web_session_id=se.web_session_id,
            user_id=se.user_id,
            event=_SSEEventBody(type=se.event_type, data=data),
        )


class BackgroundEventWire(G8eBaseModel):
    user_id: str
    event: _SSEEventBody

    @classmethod
    def from_background_event(cls, be: BackgroundEvent) -> "BackgroundEventWire":
        data = be.payload.model_dump(mode="json")
        data["user_id"] = be.user_id
        if be.investigation_id is not None:
            data["investigation_id"] = be.investigation_id
        if be.case_id is not None:
            data["case_id"] = be.case_id
        if be.task_id is not None:
            data["task_id"] = be.task_id
        return cls(user_id=be.user_id, event=_SSEEventBody(type=be.event_type, data=data))


# AI SSE event payloads
class AiProcessingStoppedPayload(G8eBaseModel):
    reason: str
    timestamp: UTCDatetime


class AIToolLifecyclePayload(G8eBaseModel):
    tool_name: str
    display_label: str | None = None
    display_icon: str | None = None
    display_detail: str | None = None
    category: str | None = None
    execution_id: str
    status: ToolCallStatus
    
    # Optional tool-specific context (reconciled from shared fixtures)
    query: str | None = None
    content: str | None = None
    results: list[dict[str, Any]] | None = None
    error: str | None = None
    port: str | None = None
    host: str | None = None
    is_open: bool | None = None
    timestamp: str | None = None


class ChatCitationsReadyPayload(G8eBaseModel):
    grounding_metadata: dict[str, Any]
    timestamp: str | None = None


class ChatErrorPayload(G8eBaseModel):
    error: str
    timestamp: str | None = None


class ChatProcessingStartedPayload(G8eBaseModel):
    agent_mode: str
    timestamp: str | None = None


class ChatResponseChunkPayload(G8eBaseModel):
    content: str
    timestamp: str | None = None


class ChatResponseCompletePayload(G8eBaseModel):
    content: str
    finish_reason: str
    has_citations: bool
    grounding_metadata: dict[str, Any]
    token_usage: dict[str, Any]
    agent_mode: str
    timestamp: str | None = None


class ChatRetryPayload(G8eBaseModel):
    attempt: int
    max_attempts: int
    timestamp: str | None = None


class ChatThinkingPayload(G8eBaseModel):
    thinking: str | None
    action_type: str
    timestamp: str | None = None


class ChatTurnCompletePayload(G8eBaseModel):
    turn: int
    timestamp: str | None = None


class TriageClarificationQuestionsPayload(G8eBaseModel):
    questions: list[str]
    complexity: str
    complexity_confidence: str
    intent: str
    intent_confidence: str
    intent_summary: str
    request_posture: str
    posture_confidence: str
