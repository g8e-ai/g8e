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

- BackgroundEvent: system-initiated, no browser session. g8ed routes by
  investigation_id. investigation_id is required; case_id is optional.

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

    def flatten_for_wire(self) -> dict[str, Any]:
        data: dict[str, Any] = self.payload.flatten_for_wire()
        data["web_session_id"] = self.web_session_id
        data["user_id"] = self.user_id
        if self.case_id is not None:
            data["case_id"] = self.case_id
        if self.investigation_id is not None:
            data["investigation_id"] = self.investigation_id
        return {
            "web_session_id": self.web_session_id,
            "user_id": self.user_id,
            "event": {
                "type": self.event_type,
                "data": data,
            },
        }


class BackgroundEvent(G8eBaseModel):
    """System-initiated event with no connected browser session.

    g8ed routes by investigation_id. Use this for background/async events
    where no web_session_id is available at publish time.
    """

    event_type: EventType = Field(description="g8ed event type")
    payload: G8eBaseModel = Field(description="Typed event-specific payload")
    investigation_id: str = Field(description="Investigation correlation ID")
    user_id: str = Field(description="User ID associated with the investigation")
    case_id: str | None = Field(default=None, description="Case correlation ID")
    task_id: str | None = Field(default=None, description="AI task ID for routing")

    def flatten_for_wire(self) -> dict[str, Any]:
        data: dict[str, Any] = self.payload.flatten_for_wire()
        data["investigation_id"] = self.investigation_id
        data["user_id"] = self.user_id
        if self.case_id is not None:
            data["case_id"] = self.case_id
        return {
            "user_id": self.user_id,
            "event": {
                "type": self.event_type,
                "data": data,
            },
        }
