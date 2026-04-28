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
Typed g8ed publish events.

All events published to g8ed are represented as one of two typed envelopes:

- SessionEvent: routes to a specific connected browser session. web_session_id,
  case_id, and investigation_id are all required — the caller must have all
  three or the event cannot be constructed.

- BackgroundEvent: system-initiated, no browser session. g8ed fans the event
  out to every active SSE session owned by user_id. investigation_id and
  case_id are optional correlation hints carried inside the payload.

The EventService.publish() method accepts the union type and dispatches
accordingly. publish_event_to_g8ed() no longer exists.
"""

from typing import Any

from app.constants import EventType
from app.models.base import G8eBaseModel, Field


class SessionEvent(G8eBaseModel):
    """Event that must reach a specific connected browser session.

    Use this when the triggering request arrived on a known web_session_id
    (approval requests, command broadcasts, AI stream events, etc.).
    """

    event_type: EventType = Field(description="g8ed event type")
    payload: G8eBaseModel = Field(description="Typed event-specific payload")
    web_session_id: str = Field(description="Browser session to deliver to")
    user_id: str = Field(description="User ID associated with the session")
    case_id: str | None = Field(default=None, description="Case correlation ID")
    investigation_id: str | None = Field(default=None, description="Investigation correlation ID")
    task_id: str | None = Field(default=None, description="AI task ID for routing")


class BackgroundEvent(G8eBaseModel):
    """System-initiated event with no connected browser session.

    g8ed fans the event out to every active SSE session owned by user_id.
    investigation_id and case_id are optional correlation hints carried inside
    the payload — they do not drive routing.
    """

    event_type: EventType = Field(description="g8ed event type")
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
