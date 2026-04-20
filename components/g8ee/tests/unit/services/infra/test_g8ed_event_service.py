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

import pytest

from app.constants import EVENT_PUBLISH_SUCCESS, EventType
from app.errors import NetworkError
from app.models.base import G8eBaseModel, Field
from app.models.events import BackgroundEvent, SessionEvent
from app.services.infra.g8ed_event_service import EventService
from app.services.protocols import EventServiceProtocol
from tests.fakes.fake_g8ed_client import FakeG8edClient as create_mock_g8ed_http_client


class _SimplePayload(G8eBaseModel):
    message: str | None = Field(default=None)
    status: str | None = Field(default=None)
    data: str | None = Field(default=None)
    error: str | None = Field(default=None)
    key: str | None = Field(default=None)
    chunk: str | None = Field(default=None)


class _AnalysisResult(G8eBaseModel):
    status: str | None = Field(default=None)
    results: list[str] | None = Field(default=None)
    count: int | None = Field(default=None)


class _ComplexPayload(G8eBaseModel):
    analysis: _AnalysisResult | None = Field(default=None)
    timestamp: str | None = Field(default=None)


pytestmark = pytest.mark.unit


@pytest.fixture
def mock_g8ed_http_client():
    return create_mock_g8ed_http_client()


@pytest.fixture
def service(mock_g8ed_http_client):
    return EventService(internal_http_client=mock_g8ed_http_client)


class TestEventServiceInit:

    def test_initialization_with_client(self, mock_g8ed_http_client):
        service = EventService(internal_http_client=mock_g8ed_http_client)

        assert service.g8ed_client is not None
        assert service.g8ed_client == mock_g8ed_http_client


@pytest.mark.asyncio(loop_scope="session")
class TestPublishEventToG8ed:

    async def test_publish_with_web_session_id(self, service, mock_g8ed_http_client):
        event = SessionEvent(
            event_type=EventType.LLM_CHAT_MESSAGE_SENT,
            payload=_SimplePayload(message="Hello world"),
            web_session_id="web-session-123",
            user_id="user-123",
            case_id="case-456",
            investigation_id="inv-789",
        )
        result = await service.publish(event)

        assert result == EVENT_PUBLISH_SUCCESS
        mock_g8ed_http_client.assert_push_sse_event_called_with(event)

    async def test_publish_background_event(self, service, mock_g8ed_http_client):
        event = BackgroundEvent(
            event_type=EventType.OPERATOR_STATUS_UPDATED_ACTIVE,
            payload=_SimplePayload(status="active"),
            investigation_id="inv-789",
            user_id="user-123",
        )
        result = await service.publish(event)

        assert result == EVENT_PUBLISH_SUCCESS
        mock_g8ed_http_client.assert_push_sse_event_called_with(event)

    async def test_session_event_typed_event_passed_to_client(self, service, mock_g8ed_http_client):
        event = SessionEvent(
            event_type=EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
            payload=_SimplePayload(data="test"),
            web_session_id="web-123",
            user_id="user-123",
            case_id="case-abc",
            investigation_id="inv-abc",
        )
        result = await service.publish(event)

        assert result == EVENT_PUBLISH_SUCCESS
        mock_g8ed_http_client.assert_push_sse_event_called_with(event)

    async def test_publish_background_event_no_web_session(self, service, mock_g8ed_http_client):
        event = BackgroundEvent(
            event_type=EventType.LLM_CHAT_ITERATION_FAILED,
            payload=_SimplePayload(error="something went wrong"),
            investigation_id="inv-err-1",
            user_id="user-123",
        )
        result = await service.publish(event)

        assert result == EVENT_PUBLISH_SUCCESS
        mock_g8ed_http_client.assert_push_sse_event_called_with(event)

    async def test_publish_with_all_optional_fields(self, service, mock_g8ed_http_client):
        event = SessionEvent(
            event_type=EventType.INVESTIGATION_STATUS_UPDATED_CLOSED,
            payload=_SimplePayload(key="value"),
            web_session_id="web-123",
            user_id="user-123",
            case_id="case-456",
            investigation_id="inv-101",
            task_id="task-789",
        )
        result = await service.publish(event)

        assert result == EVENT_PUBLISH_SUCCESS
        mock_g8ed_http_client.assert_push_sse_event_called_with(event)
        assert event.case_id == "case-456"
        assert event.investigation_id == "inv-101"
        assert event.task_id == "task-789"

    async def test_web_session_id_on_session_event(self, service, mock_g8ed_http_client):
        event = SessionEvent(
            event_type=EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
            payload=_SimplePayload(chunk="hello"),
            web_session_id="web-abc",
            user_id="user-abc",
            case_id="case-abc",
            investigation_id="inv-1",
        )
        result = await service.publish(event)

        assert result == EVENT_PUBLISH_SUCCESS
        assert event.web_session_id == "web-abc"

    async def test_correlation_ids_on_session_event(self, service, mock_g8ed_http_client):
        event = SessionEvent(
            event_type=EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
            payload=_SimplePayload(chunk="token"),
            web_session_id="web-abc",
            user_id="user-abc",
            case_id="authoritative-case",
            investigation_id="authoritative-inv",
        )
        result = await service.publish(event)

        assert result == EVENT_PUBLISH_SUCCESS
        assert event.investigation_id == "authoritative-inv"
        assert event.case_id == "authoritative-case"

    async def test_publish_failure_raises_network_error(self, service, mock_g8ed_http_client):
        mock_g8ed_http_client.set_push_sse_event_return_value(False)

        with pytest.raises(NetworkError) as exc_info:
            await service.publish(
                BackgroundEvent(
                    event_type=EventType.LLM_CHAT_ITERATION_FAILED,
                    payload=_SimplePayload(data="test"),
                    investigation_id="inv-err",
                    user_id="user-err",
                )
            )

        assert exc_info.value.get_http_status() == 500

    async def test_publish_zero_delivered_is_success(self, service, mock_g8ed_http_client):
        """Regression: fan-out with zero active sessions must not raise.

        BackgroundEvent fan-out to a user with no connected sessions returns
        delivered=0 and success=True. That is documented behavior, not a
        failure - it must not be collapsed into NetworkError.
        """
        from app.models.g8ed_client import SSEPushResponse

        mock_g8ed_http_client.set_push_sse_event_response(
            SSEPushResponse(success=True, delivered=0)
        )

        result = await service.publish(
            BackgroundEvent(
                event_type=EventType.OPERATOR_HEARTBEAT_RECEIVED,
                payload=_SimplePayload(status="ok"),
                investigation_id="inv-bg",
                user_id="user-no-sessions",
            )
        )

        assert result == EVENT_PUBLISH_SUCCESS

    async def test_publish_transport_error_preserves_status(self, service, mock_g8ed_http_client):
        """Genuine transport failures propagate the NetworkError raised by the client.

        The HTTP status code set by InternalHttpClient is preserved in the
        error details - a real g8ed 500 must remain distinguishable from a
        zero-delivery success.
        """
        transport_err = NetworkError(
            "[HTTP-G8ED] SSE push returned HTTP 500",
            component="g8ee",
            details={"status_code": 500, "response": "crash"},
        )
        mock_g8ed_http_client.set_push_sse_event_exception(transport_err)

        with pytest.raises(NetworkError) as exc_info:
            await service.publish(
                BackgroundEvent(
                    event_type=EventType.LLM_CHAT_ITERATION_FAILED,
                    payload=_SimplePayload(data="test"),
                    investigation_id="inv-err",
                    user_id="user-err",
                )
            )

        assert exc_info.value.error_detail.details["status_code"] == 500

    async def test_publish_with_complex_payload(self, service, mock_g8ed_http_client):
        complex_payload = _ComplexPayload(
            analysis=_AnalysisResult(
                status="complete",
                results=["item1", "item2"],
                count=2,
            ),
            timestamp="2025-01-01T00:00:00Z"
        )
        event = SessionEvent(
            event_type=EventType.INVESTIGATION_UPDATED,
            payload=complex_payload,
            web_session_id="web-123",
            user_id="user-123",
            case_id="case-123",
            investigation_id="inv-456",
        )
        result = await service.publish(event)

        assert result == EVENT_PUBLISH_SUCCESS
        assert event.payload.analysis.status == "complete"
        assert len(event.payload.analysis.results) == 2
        assert event.payload.analysis.count == 2

    async def test_investigation_id_on_session_event(self, service, mock_g8ed_http_client):
        event = SessionEvent(
            event_type=EventType.LLM_CHAT_ITERATION_THINKING_STARTED,
            payload=_SimplePayload(message="test"),
            web_session_id="web-123",
            user_id="user-123",
            case_id="case-123",
            investigation_id="inv-critical-123",
        )
        result = await service.publish(event)

        assert result == EVENT_PUBLISH_SUCCESS
        assert event.investigation_id == "inv-critical-123"

    async def test_network_error_message_contains_event_type(self, service, mock_g8ed_http_client):
        mock_g8ed_http_client.set_push_sse_event_return_value(False)

        with pytest.raises(NetworkError) as exc_info:
            await service.publish(
                BackgroundEvent(
                    event_type=EventType.LLM_CHAT_ITERATION_FAILED,
                    payload=_SimplePayload(data="test"),
                    investigation_id="inv-err",
                    user_id="user-err",
                )
            )

        assert EventType.LLM_CHAT_ITERATION_FAILED in str(exc_info.value)

    async def test_background_event_with_optional_case_id_none(self, service, mock_g8ed_http_client):
        event = BackgroundEvent(
            event_type=EventType.OPERATOR_STATUS_UPDATED_ACTIVE,
            payload=_SimplePayload(status="active"),
            investigation_id="inv-no-case",
            user_id="user-123",
        )
        result = await service.publish(event)

        assert result == EVENT_PUBLISH_SUCCESS
        assert event.case_id is None

    async def test_background_event_with_explicit_case_id(self, service, mock_g8ed_http_client):
        event = BackgroundEvent(
            event_type=EventType.OPERATOR_STATUS_UPDATED_ACTIVE,
            payload=_SimplePayload(status="active"),
            investigation_id="inv-with-case",
            user_id="user-123",
            case_id="case-explicit",
        )
        result = await service.publish(event)

        assert result == EVENT_PUBLISH_SUCCESS
        assert event.case_id == "case-explicit"

    async def test_background_event_task_id_on_event(self, service, mock_g8ed_http_client):
        event = BackgroundEvent(
            event_type=EventType.OPERATOR_STATUS_UPDATED_ACTIVE,
            payload=_SimplePayload(status="active"),
            investigation_id="inv-task",
            user_id="user-123",
            task_id="task-bg-001",
        )
        result = await service.publish(event)

        assert result == EVENT_PUBLISH_SUCCESS
        assert event.task_id == "task-bg-001"

    async def test_session_event_task_id_none_when_omitted(self, service, mock_g8ed_http_client):
        event = SessionEvent(
            event_type=EventType.LLM_CHAT_MESSAGE_SENT,
            payload=_SimplePayload(message="hi"),
            web_session_id="web-x",
            user_id="user-x",
            case_id="case-x",
            investigation_id="inv-x",
        )
        result = await service.publish(event)

        assert result == EVENT_PUBLISH_SUCCESS
        assert event.task_id is None


class TestEventServiceProtocol:

    def test_event_service_satisfies_protocol(self, mock_g8ed_http_client):
        service = EventService(internal_http_client=mock_g8ed_http_client)
        assert isinstance(service, EventServiceProtocol)

    def test_plain_object_does_not_satisfy_protocol(self):
        assert not isinstance(object(), EventServiceProtocol)


