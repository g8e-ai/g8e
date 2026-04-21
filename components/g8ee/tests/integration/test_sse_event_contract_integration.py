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
SSE Event Contract Integration Tests

Ensures g8ee and g8ed emit/consume SSE events that match the shared fixture
definitions in shared/test-fixtures/sse-events.json. This prevents drift
between components and ensures wire compatibility.
"""

import json
import pytest
from pathlib import Path

from app.constants import EventType
from app.models.agent import StreamChunkData, StreamChunkFromModel, StreamChunkFromModelType
from app.models.g8ed_client import ChatProcessingStartedPayload
from app.services.ai.agent_sse import deliver_via_sse
from tests.fakes.agent_helpers import (
    make_agent_run_args,
    make_g8ed_event_service,
)

pytestmark = [pytest.mark.integration, pytest.mark.asyncio(loop_scope="session")]


def _load_shared_sse_fixtures():
    """Load shared SSE event fixtures from the workspace root."""
    # Navigate from components/g8ee/tests/integration to shared/test-fixtures/
    fixtures_path = Path(__file__).resolve().parent.parent.parent.parent.parent / "shared" / "test-fixtures" / "sse-events.json"
    with open(fixtures_path) as f:
        return json.load(f)


# Load fixtures once for all tests
SHARED_SSE_EVENTS = _load_shared_sse_fixtures()


@pytest.mark.asyncio(loop_scope="session")
class TestSSEEventContract:
    """Verify g8ee SSE events match shared fixture contracts."""

    async def test_processing_started_matches_shared_fixture(self):
        """LLM_CHAT_ITERATION_STARTED event matches shared structure."""
        inputs, state = make_agent_run_args(
            case_id="contract-test-case-007",
            investigation_id="contract-test-inv-007",
            web_session_id="contract-test-sess-007",
            user_id="contract-test-user-007",
            agent_mode="g8e.not.bound",
        )
        event_svc = make_g8ed_event_service()

        async def _simple_stream():
            yield StreamChunkFromModel(
                type=StreamChunkFromModelType.TEXT,
                data=StreamChunkData(content="Test content"),
            )
            yield StreamChunkFromModel(
                type=StreamChunkFromModelType.COMPLETE,
                data=StreamChunkData(finish_reason="STOP"),
            )

        await deliver_via_sse(
            stream=_simple_stream(),
            inputs=inputs,
            state=state,
            g8ed_event_service=event_svc,
        )

        published = event_svc._published_events
        started_events = [e for e in published if e.event_type == EventType.LLM_CHAT_ITERATION_STARTED]

        assert len(started_events) == 1
        actual_event = started_events[0]

        # Compare against shared fixture
        expected_fixture = SHARED_SSE_EVENTS["llm_chat_iteration_started"]

        # Verify payload is typed ChatProcessingStartedPayload with agent_mode
        assert isinstance(actual_event.payload, ChatProcessingStartedPayload)
        assert actual_event.event_type == expected_fixture["type"]
        assert actual_event.payload.agent_mode == expected_fixture["data"]["agent_mode"]
        assert actual_event.investigation_id == inputs.investigation_id
        assert actual_event.case_id == inputs.case_id
        assert actual_event.web_session_id == inputs.web_session_id

    async def test_text_chunk_received_matches_shared_fixture(self):
        """LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED event matches shared structure."""
        # Use content from fixture to satisfy contract test
        expected_fixture = SHARED_SSE_EVENTS["text_chunk_received"]
        fixture_content = expected_fixture["data"]["content"]

        inputs, state = make_agent_run_args(
            case_id="contract-test-case-001",
            investigation_id="contract-test-inv-001",
            web_session_id="contract-test-sess-001",
            user_id="contract-test-user-001",
        )
        event_svc = make_g8ed_event_service()

        # Create a text chunk stream
        async def _text_stream():
            yield StreamChunkFromModel(
                type=StreamChunkFromModelType.TEXT,
                data=StreamChunkData(content=fixture_content),
            )
            yield StreamChunkFromModel(
                type=StreamChunkFromModelType.COMPLETE,
                data=StreamChunkData(finish_reason="STOP"),
            )

        await deliver_via_sse(
            stream=_text_stream(),
            inputs=inputs,
            state=state,
            g8ed_event_service=event_svc,
        )

        # Get published events
        published = event_svc._published_events
        text_chunk_events = [e for e in published if e.event_type == EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED]

        assert len(text_chunk_events) >= 1
        actual_event = text_chunk_events[0]

        # Compare against shared fixture
        expected_fixture = SHARED_SSE_EVENTS["text_chunk_received"]

        # Verify structure matches (using actual EventType constants)
        assert actual_event.event_type == EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED
        assert actual_event.payload.content == expected_fixture["data"]["content"]
        assert actual_event.investigation_id == inputs.investigation_id
        assert actual_event.case_id == inputs.case_id
        assert actual_event.web_session_id == inputs.web_session_id

    async def test_text_completed_matches_shared_fixture(self):
        """LLM_CHAT_ITERATION_TEXT_COMPLETED event matches shared structure."""
        # Use content from fixture to satisfy contract test
        expected_fixture = SHARED_SSE_EVENTS["text_completed"]
        fixture_content = expected_fixture["data"]["content"]

        inputs, state = make_agent_run_args(
            case_id="contract-test-case-002",
            investigation_id="contract-test-inv-002",
            web_session_id="contract-test-sess-002",
            user_id="contract-test-user-002",
        )
        event_svc = make_g8ed_event_service()

        async def _completed_stream():
            yield StreamChunkFromModel(
                type=StreamChunkFromModelType.TEXT,
                data=StreamChunkData(content=fixture_content),
            )
            yield StreamChunkFromModel(
                type=StreamChunkFromModelType.COMPLETE,
                data=StreamChunkData(finish_reason="STOP"),
            )

        await deliver_via_sse(
            stream=_completed_stream(),
            inputs=inputs,
            state=state,
            g8ed_event_service=event_svc,
        )

        published = event_svc._published_events
        completed_events = [e for e in published if e.event_type == EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED]
        
        assert len(completed_events) >= 1
        actual_event = completed_events[0]

        # Compare against shared fixture
        expected_fixture = SHARED_SSE_EVENTS["text_completed"]
        
        assert actual_event.event_type == expected_fixture["type"]
        assert actual_event.payload.content == expected_fixture["data"]["content"]
        assert actual_event.investigation_id == inputs.investigation_id
        assert actual_event.case_id == inputs.case_id
        assert actual_event.web_session_id == inputs.web_session_id

    async def test_chat_iteration_failed_matches_shared_fixture(self):
        """LLM_CHAT_ITERATION_FAILED event matches shared structure."""
        inputs, state = make_agent_run_args(
            case_id="contract-test-case-003",
            investigation_id="contract-test-inv-003",
            web_session_id="contract-test-sess-003",
            user_id="contract-test-user-003",
        )
        event_svc = make_g8ed_event_service()

        async def _failing_stream():
            yield StreamChunkFromModel(
                type=StreamChunkFromModelType.TEXT,
                data=StreamChunkData(content="Before failure"),
            )
            raise Exception("Contract test failure")

        await deliver_via_sse(
            stream=_failing_stream(),
            inputs=inputs,
            state=state,
            g8ed_event_service=event_svc,
        )

        published = event_svc._published_events
        failed_events = [e for e in published if e.event_type == EventType.LLM_CHAT_ITERATION_FAILED]

        assert len(failed_events) >= 1
        actual_event = failed_events[0]

        # Compare against shared fixture
        expected_fixture = SHARED_SSE_EVENTS["chat_iteration_failed"]

        assert actual_event.event_type == expected_fixture["type"]
        assert actual_event.payload.error == "Contract test failure"
        assert actual_event.investigation_id == inputs.investigation_id
        assert actual_event.case_id == inputs.case_id
        assert actual_event.web_session_id == inputs.web_session_id

    async def test_error_chunk_skips_completion_event(self):
        """ERROR chunk prevents completion event from being published."""
        inputs, state = make_agent_run_args(
            case_id="contract-test-case-006",
            investigation_id="contract-test-inv-006",
            web_session_id="contract-test-sess-006",
            user_id="contract-test-user-006",
        )
        event_svc = make_g8ed_event_service()

        async def _error_chunk_stream():
            yield StreamChunkFromModel(
                type=StreamChunkFromModelType.TEXT,
                data=StreamChunkData(content="Before error"),
            )
            yield StreamChunkFromModel(
                type=StreamChunkFromModelType.ERROR,
                data=StreamChunkData(error="LLM provider error"),
            )

        await deliver_via_sse(
            stream=_error_chunk_stream(),
            inputs=inputs,
            state=state,
            g8ed_event_service=event_svc,
        )

        published = event_svc._published_events

        # Verify ERROR event was published
        error_events = [e for e in published if e.event_type == EventType.LLM_CHAT_ITERATION_FAILED]
        assert len(error_events) == 1
        assert error_events[0].payload.error == "LLM provider error"

        # Verify completion event was NOT published (should be skipped when error occurs)
        completion_events = [e for e in published if e.event_type == EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED]
        assert len(completion_events) == 0, "Completion event should not be published when ERROR chunk is received"

    async def test_search_web_events_match_shared_fixtures(self):
        """Search web tool events match shared structures."""
        from app.constants import OperatorToolName
        from app.models.agent import StreamChunkData, StreamChunkFromModel

        inputs, state = make_agent_run_args(
            case_id="contract-test-case-004",
            investigation_id="contract-test-inv-004",
            web_session_id="contract-test-sess-004",
            user_id="contract-test-user-004",
        )
        event_svc = make_g8ed_event_service()

        # Create search web tool call stream
        async def _search_web_stream():
            yield StreamChunkFromModel(
                type=StreamChunkFromModelType.TOOL_CALL,
                data=StreamChunkData(
                    tool_name=OperatorToolName.G8E_SEARCH_WEB,
                    execution_id="contract-search-001",
                    display_detail="contract test query",
                    is_operator_tool=True,
                ),
            )
            yield StreamChunkFromModel(
                type=StreamChunkFromModelType.COMPLETE,
                data=StreamChunkData(finish_reason="STOP"),
            )

        await deliver_via_sse(
            stream=_search_web_stream(),
            inputs=inputs,
            state=state,
            g8ed_event_service=event_svc,
        )

        published = event_svc._published_events
        search_requested_events = [e for e in published if e.event_type == EventType.LLM_TOOL_G8E_WEB_SEARCH_REQUESTED]
        
        assert len(search_requested_events) >= 1
        actual_event = search_requested_events[0]

        # Compare against shared fixture
        expected_fixture = SHARED_SSE_EVENTS["g8e_web_search_requested"]
        
        assert actual_event.event_type == expected_fixture["type"]
        assert actual_event.payload.query == "contract test query"
        assert actual_event.payload.execution_id == "contract-search-001"
        assert actual_event.investigation_id == inputs.investigation_id
        assert actual_event.case_id == inputs.case_id
        assert actual_event.web_session_id == inputs.web_session_id

    async def test_all_required_routing_fields_present(self):
        """All events have required routing fields matching shared fixtures."""
        inputs, state = make_agent_run_args(
            case_id="contract-test-case-005",
            investigation_id="contract-test-inv-005",
            web_session_id="contract-test-sess-005",
            user_id="contract-test-user-005",
        )
        event_svc = make_g8ed_event_service()

        async def _multi_event_stream():
            yield StreamChunkFromModel(
                type=StreamChunkFromModelType.TEXT,
                data=StreamChunkData(content="Multi event test"),
            )
            yield StreamChunkFromModel(
                type=StreamChunkFromModelType.COMPLETE,
                data=StreamChunkData(finish_reason="STOP"),
            )

        await deliver_via_sse(
            stream=_multi_event_stream(),
            inputs=inputs,
            state=state,
            g8ed_event_service=event_svc,
        )

        published = event_svc._published_events

        # Every published event should have the required routing fields
        for event in published:
            # Check that the event has the expected routing fields
            assert hasattr(event, 'investigation_id')
            assert hasattr(event, 'case_id') 
            assert hasattr(event, 'web_session_id')
            
            # Check that values match the inputs
            assert event.investigation_id == inputs.investigation_id
            assert event.case_id == inputs.case_id
            assert event.web_session_id == inputs.web_session_id


async def test_shared_fixtures_contain_all_required_event_types():
    """Shared fixtures file contains all required event types."""
    required_event_types = [
        "text_chunk_received",
        "text_completed", 
        "chat_iteration_failed",
        "g8e_web_search_requested",
        "g8e_web_search_completed",
        "g8e_web_search_failed",
        "port_check_requested",
        "port_check_completed",
        "port_check_failed",
        "citations_received",
        "operator_command_requested",
        "operator_command_started",
        "operator_command_completed",
        "operator_command_failed",
        "llm_lifecycle_started",
        "llm_lifecycle_completed",
        "platform_sse_connection_established",
        "platform_sse_keepalive_sent"
    ]

    for event_type in required_event_types:
        assert event_type in SHARED_SSE_EVENTS, f"Missing required event type: {event_type}"
        
        # Each fixture should have the required structure
        fixture = SHARED_SSE_EVENTS[event_type]
        assert "type" in fixture, f"Event {event_type} missing 'type' field"
        assert "data" in fixture, f"Event {event_type} missing 'data' field"
        
        # Data should have routing fields (except for platform events which only have web_session_id)
        data = fixture["data"]
        assert "web_session_id" in data, f"Event {event_type} missing 'web_session_id' in data"
        if not event_type.startswith("platform_sse_"):
            assert "investigation_id" in data, f"Event {event_type} missing 'investigation_id' in data"
            assert "case_id" in data, f"Event {event_type} missing 'case_id' in data"


async def test_shared_fixture_event_types_match_constants():
    """Shared fixture event types match g8ee EventType constants."""
    # Map fixture keys to EventType constants
    fixture_to_constant_mapping = {
        "text_chunk_received": EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
        "text_completed": EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED,
        "chat_iteration_failed": EventType.LLM_CHAT_ITERATION_FAILED,
        "g8e_web_search_requested": EventType.LLM_TOOL_G8E_WEB_SEARCH_REQUESTED,
        "g8e_web_search_completed": EventType.LLM_TOOL_G8E_WEB_SEARCH_COMPLETED,
        "g8e_web_search_failed": EventType.LLM_TOOL_G8E_WEB_SEARCH_FAILED,
        "port_check_completed": EventType.OPERATOR_NETWORK_PORT_CHECK_COMPLETED,
        "port_check_failed": EventType.OPERATOR_NETWORK_PORT_CHECK_FAILED,
        "citations_received": EventType.LLM_CHAT_ITERATION_CITATIONS_RECEIVED,
        "operator_command_requested": EventType.OPERATOR_COMMAND_REQUESTED,
        "operator_command_started": EventType.OPERATOR_COMMAND_STARTED,
        "operator_command_completed": EventType.OPERATOR_COMMAND_COMPLETED,
        "operator_command_failed": EventType.OPERATOR_COMMAND_FAILED,
        "llm_lifecycle_started": EventType.LLM_LIFECYCLE_STARTED,
        "llm_lifecycle_completed": EventType.LLM_LIFECYCLE_COMPLETED,
        "platform_sse_connection_established": EventType.PLATFORM_SSE_CONNECTION_ESTABLISHED,
        "platform_sse_keepalive_sent": EventType.PLATFORM_SSE_KEEPALIVE_SENT,
    }

    for fixture_key, expected_constant in fixture_to_constant_mapping.items():
        assert fixture_key in SHARED_SSE_EVENTS, f"Missing fixture: {fixture_key}"
        fixture_event_type = SHARED_SSE_EVENTS[fixture_key]["type"]
        assert fixture_event_type == expected_constant, f"Fixture {fixture_key} type {fixture_event_type} doesn't match constant {expected_constant}"
