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
SSE wire contract tests.

Verifies that SessionEventWire and BackgroundEventWire serialize correctly
for SSE transport, ensuring:
- model_dump(mode="json") produces wire-safe output
- datetime fields use UTCDatetime with Z suffix
- nested payloads are properly serialized
- conversion methods produce correct structure
"""

from datetime import UTC, datetime

import pytest

from app.constants import EventType
from app.models.base import G8eBaseModel, UTCDatetime
from app.models.events import BackgroundEvent, BackgroundEventWire, SessionEvent, SessionEventWire

pytestmark = pytest.mark.unit


class _SamplePayload(G8eBaseModel):
    message: str
    timestamp: UTCDatetime | None = None
    count: int | None = None


class TestSessionEventWireContract:
    """Contract tests for SessionEventWire serialization."""

    def test_from_session_event_creates_wire_structure(self):
        payload = _SamplePayload(message="test", timestamp=datetime(2026, 1, 15, 10, 30, 0, tzinfo=UTC))
        session_event = SessionEvent(
            event_type=EventType.LLM_CHAT_ITERATION_THINKING_STARTED,
            payload=payload,
            web_session_id="sess-123",
            user_id="user-abc",
            case_id="case-xyz",
            investigation_id="inv-def",
            task_id="task-ghi",
        )

        wire = SessionEventWire.from_session_event(session_event)

        assert wire.web_session_id == "sess-123"
        assert wire.user_id == "user-abc"
        assert wire.event.type == EventType.LLM_CHAT_ITERATION_THINKING_STARTED
        assert wire.event.data["message"] == "test"
        assert wire.event.data["web_session_id"] == "sess-123"
        assert wire.event.data["user_id"] == "user-abc"
        assert wire.event.data["case_id"] == "case-xyz"
        assert wire.event.data["investigation_id"] == "inv-def"
        assert wire.event.data["task_id"] == "task-ghi"

    def test_from_session_event_with_optional_fields_none(self):
        payload = _SamplePayload(message="test")
        session_event = SessionEvent(
            event_type=EventType.LLM_CHAT_ITERATION_THINKING_STARTED,
            payload=payload,
            web_session_id="sess-123",
            user_id="user-abc",
        )

        wire = SessionEventWire.from_session_event(session_event)

        assert wire.web_session_id == "sess-123"
        assert wire.user_id == "user-abc"
        assert wire.event.type == EventType.LLM_CHAT_ITERATION_THINKING_STARTED
        assert wire.event.data["message"] == "test"
        assert wire.event.data["web_session_id"] == "sess-123"
        assert wire.event.data["user_id"] == "user-abc"
        assert "case_id" not in wire.event.data
        assert "investigation_id" not in wire.event.data
        assert "task_id" not in wire.event.data

    def test_wire_dump_serializes_to_json_safe_dict(self):
        payload = _SamplePayload(message="test")
        session_event = SessionEvent(
            event_type=EventType.LLM_CHAT_ITERATION_THINKING_STARTED,
            payload=payload,
            web_session_id="sess-123",
            user_id="user-abc",
        )
        wire = SessionEventWire.from_session_event(session_event)

        dumped = wire.model_dump(mode="json")

        assert isinstance(dumped, dict)
        assert dumped["web_session_id"] == "sess-123"
        assert dumped["user_id"] == "user-abc"
        assert dumped["event"]["type"] == "g8e.v1.ai.llm.chat.iteration.thinking.started"
        assert isinstance(dumped["event"]["data"], dict)
        assert dumped["event"]["data"]["message"] == "test"

    def test_wire_dump_datetime_emits_z_suffix(self):
        dt = datetime(2026, 1, 15, 10, 30, 0, 123456, tzinfo=UTC)
        payload = _SamplePayload(message="test", timestamp=dt)
        session_event = SessionEvent(
            event_type=EventType.LLM_CHAT_ITERATION_THINKING_STARTED,
            payload=payload,
            web_session_id="sess-123",
            user_id="user-abc",
        )
        wire = SessionEventWire.from_session_event(session_event)

        dumped = wire.model_dump(mode="json")

        assert dumped["event"]["data"]["timestamp"].endswith("Z")
        assert "+" not in dumped["event"]["data"]["timestamp"]
        assert ".123456Z" in dumped["event"]["data"]["timestamp"]

    def test_wire_dump_omits_none_optional_fields(self):
        payload = _SamplePayload(message="test", timestamp=None, count=None)
        session_event = SessionEvent(
            event_type=EventType.LLM_CHAT_ITERATION_THINKING_STARTED,
            payload=payload,
            web_session_id="sess-123",
            user_id="user-abc",
        )
        wire = SessionEventWire.from_session_event(session_event)

        dumped = wire.model_dump(mode="json")

        assert "timestamp" not in dumped["event"]["data"]
        assert "count" not in dumped["event"]["data"]


class TestBackgroundEventWireContract:
    """Contract tests for BackgroundEventWire serialization."""

    def test_from_background_event_creates_wire_structure(self):
        payload = _SamplePayload(message="test", timestamp=datetime(2026, 1, 15, 10, 30, 0, tzinfo=UTC))
        background_event = BackgroundEvent(
            event_type=EventType.LLM_CHAT_ITERATION_THINKING_STARTED,
            payload=payload,
            user_id="user-abc",
            investigation_id="inv-def",
            case_id="case-xyz",
            task_id="task-ghi",
        )

        wire = BackgroundEventWire.from_background_event(background_event)

        assert wire.user_id == "user-abc"
        assert wire.event.type == EventType.LLM_CHAT_ITERATION_THINKING_STARTED
        assert wire.event.data["message"] == "test"
        assert wire.event.data["user_id"] == "user-abc"
        assert wire.event.data["investigation_id"] == "inv-def"
        assert wire.event.data["case_id"] == "case-xyz"
        assert wire.event.data["task_id"] == "task-ghi"

    def test_from_background_event_with_optional_fields_none(self):
        payload = _SamplePayload(message="test")
        background_event = BackgroundEvent(
            event_type=EventType.LLM_CHAT_ITERATION_THINKING_STARTED,
            payload=payload,
            user_id="user-abc",
        )

        wire = BackgroundEventWire.from_background_event(background_event)

        assert wire.user_id == "user-abc"
        assert wire.event.type == EventType.LLM_CHAT_ITERATION_THINKING_STARTED
        assert wire.event.data["message"] == "test"
        assert wire.event.data["user_id"] == "user-abc"
        assert "investigation_id" not in wire.event.data
        assert "case_id" not in wire.event.data
        assert "task_id" not in wire.event.data

    def test_wire_dump_serializes_to_json_safe_dict(self):
        payload = _SamplePayload(message="test")
        background_event = BackgroundEvent(
            event_type=EventType.LLM_CHAT_ITERATION_THINKING_STARTED,
            payload=payload,
            user_id="user-abc",
        )
        wire = BackgroundEventWire.from_background_event(background_event)

        dumped = wire.model_dump(mode="json")

        assert isinstance(dumped, dict)
        assert dumped["user_id"] == "user-abc"
        assert dumped["event"]["type"] == "g8e.v1.ai.llm.chat.iteration.thinking.started"
        assert isinstance(dumped["event"]["data"], dict)
        assert dumped["event"]["data"]["message"] == "test"

    def test_wire_dump_datetime_emits_z_suffix(self):
        dt = datetime(2026, 1, 15, 10, 30, 0, 123456, tzinfo=UTC)
        payload = _SamplePayload(message="test", timestamp=dt)
        background_event = BackgroundEvent(
            event_type=EventType.LLM_CHAT_ITERATION_THINKING_STARTED,
            payload=payload,
            user_id="user-abc",
        )
        wire = BackgroundEventWire.from_background_event(background_event)

        dumped = wire.model_dump(mode="json")

        assert dumped["event"]["data"]["timestamp"].endswith("Z")
        assert "+" not in dumped["event"]["data"]["timestamp"]
        assert ".123456Z" in dumped["event"]["data"]["timestamp"]

    def test_wire_dump_omits_none_optional_fields(self):
        payload = _SamplePayload(message="test", timestamp=None, count=None)
        background_event = BackgroundEvent(
            event_type=EventType.LLM_CHAT_ITERATION_THINKING_STARTED,
            payload=payload,
            user_id="user-abc",
        )
        wire = BackgroundEventWire.from_background_event(background_event)

        dumped = wire.model_dump(mode="json")

        assert "timestamp" not in dumped["event"]["data"]
        assert "count" not in dumped["event"]["data"]


class TestSSEWireContractInvariants:
    """Cross-cutting contract invariants for SSE wire models."""

    def test_both_wire_models_use_model_dump_json_mode(self):
        """Both wire models must serialize via model_dump(mode="json")."""
        payload = _SamplePayload(message="test")

        session_event = SessionEvent(
            event_type=EventType.LLM_CHAT_ITERATION_THINKING_STARTED,
            payload=payload,
            web_session_id="sess-123",
            user_id="user-abc",
        )
        session_wire = SessionEventWire.from_session_event(session_event)
        session_dumped = session_wire.model_dump(mode="json")

        background_event = BackgroundEvent(
            event_type=EventType.LLM_CHAT_ITERATION_THINKING_STARTED,
            payload=payload,
            user_id="user-abc",
        )
        background_wire = BackgroundEventWire.from_background_event(background_event)
        background_dumped = background_wire.model_dump(mode="json")

        assert isinstance(session_dumped, dict)
        assert isinstance(background_dumped, dict)

    def test_event_type_serializes_as_string(self):
        """EventType enum must serialize as string in wire format."""
        payload = _SamplePayload(message="test")

        session_event = SessionEvent(
            event_type=EventType.LLM_CHAT_ITERATION_THINKING_STARTED,
            payload=payload,
            web_session_id="sess-123",
            user_id="user-abc",
        )
        session_wire = SessionEventWire.from_session_event(session_event)
        dumped = session_wire.model_dump(mode="json")

        assert isinstance(dumped["event"]["type"], str)
        assert dumped["event"]["type"] == "g8e.v1.ai.llm.chat.iteration.thinking.started"

    def test_nested_payload_serializes_recursively(self):
        """Nested G8eBaseModel payloads must serialize recursively."""
        class _NestedPayload(G8eBaseModel):
            inner: _SamplePayload

        inner = _SamplePayload(message="inner-test")
        outer = _NestedPayload(inner=inner)

        session_event = SessionEvent(
            event_type=EventType.LLM_CHAT_ITERATION_THINKING_STARTED,
            payload=outer,
            web_session_id="sess-123",
            user_id="user-abc",
        )
        session_wire = SessionEventWire.from_session_event(session_event)
        dumped = session_wire.model_dump(mode="json")

        assert isinstance(dumped["event"]["data"]["inner"], dict)
        assert dumped["event"]["data"]["inner"]["message"] == "inner-test"
