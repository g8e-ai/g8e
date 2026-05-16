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
Unit tests for SSE wire envelope models.

Coverage:
- SessionEventWire.from_session_event produces correct SSE wire envelope structure
- BackgroundEventWire.from_background_event produces correct SSE wire envelope structure
- Wire envelope structure matches expected SSE payload shape
"""

import pytest

from app.constants import EventType
from app.models.base import Field, G8eBaseModel
from app.models.events import BackgroundEvent, BackgroundEventWire, SessionEvent, SessionEventWire

pytestmark = pytest.mark.unit


class _SimplePayload(G8eBaseModel):
    message: str | None = Field(default=None)
    status: str | None = Field(default=None)
    data: str | None = Field(default=None)


class _ComplexPayload(G8eBaseModel):
    nested: dict[str, str] | None = Field(default=None)
    items: list[str] | None = Field(default=None)
    count: int | None = Field(default=None)


class _PayloadWithId(G8eBaseModel):
    id: str
    name: str


class TestSessionEventWireStructure:
    """Assert SessionEventWire.from_session_event produces correct SSE wire envelope structure."""

    def test_wire_structure_top_level_keys(self):
        se = SessionEvent(
            event_type=EventType.LLM_CHAT_MESSAGE_SENT,
            payload=_SimplePayload(message="test"),
            web_session_id="web-session-123",
            user_id="user-456",
        )

        wire = SessionEventWire.from_session_event(se).model_dump(mode="json")

        assert set(wire.keys()) == {"web_session_id", "user_id", "event"}
        assert set(wire["event"].keys()) == {"type", "data"}
        assert isinstance(wire["event"]["type"], str)

    def test_wire_structure_with_optional_ids(self):
        se = SessionEvent(
            event_type=EventType.CASE_CREATED,
            payload=_PayloadWithId(id="case-123", name="Test Case"),
            web_session_id="web-session-123",
            user_id="user-456",
            case_id="case-789",
            investigation_id="inv-101",
        )

        wire = SessionEventWire.from_session_event(se).model_dump(mode="json")

        assert "case_id" in wire["event"]["data"]
        assert "investigation_id" in wire["event"]["data"]
        assert wire["event"]["data"]["case_id"] == "case-789"
        assert wire["event"]["data"]["investigation_id"] == "inv-101"


class TestBackgroundEventWireStructure:
    """Assert BackgroundEventWire.from_background_event produces correct SSE wire envelope structure."""

    def test_wire_structure_top_level_keys(self):
        be = BackgroundEvent(
            event_type=EventType.OPERATOR_HEARTBEAT_SENT,
            payload=_SimplePayload(status="heartbeat"),
            user_id="user-456",
        )

        wire = BackgroundEventWire.from_background_event(be).model_dump(mode="json")

        assert set(wire.keys()) == {"user_id", "event"}
        assert set(wire["event"].keys()) == {"type", "data"}
        assert isinstance(wire["event"]["type"], str)

    def test_wire_structure_with_optional_ids(self):
        be = BackgroundEvent(
            event_type=EventType.CASE_ESCALATED,
            payload=_PayloadWithId(id="case-123", name="Escalated"),
            user_id="user-456",
            investigation_id="inv-101",
            case_id="case-789",
        )

        wire = BackgroundEventWire.from_background_event(be).model_dump(mode="json")

        assert "case_id" in wire["event"]["data"]
        assert "investigation_id" in wire["event"]["data"]
        assert wire["event"]["data"]["case_id"] == "case-789"
        assert wire["event"]["data"]["investigation_id"] == "inv-101"
