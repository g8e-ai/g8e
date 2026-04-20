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

from datetime import UTC, datetime

import pytest

from app.models.settings import AuthSettings, G8eePlatformSettings
from app.constants import (
    INTERNAL_AUTH_HEADER,
    ComponentName,
    EventType,
    OperatorStatus,
    G8eHeaders,
)
from app.errors import ConfigurationError, NetworkError
from app.models.events import SessionEvent
from app.models.g8ed_client import (
    ChatResponseChunkPayload,
    GrantIntentResponse,
    IntentOperationResult,
    IntentRequestPayload,
    RevokeIntentResponse,
    SSEPushResponse,
)
from app.services.infra.internal_http_client import InternalHttpClient

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]


class MockG8eHTTPResponse:
    """Mock response matching the HTTPClient  response interface."""

    def __init__(self, status: int, json_data: dict, text_data: str = ""):
        self.status_code = status
        self._json_data = json_data
        self._text_data = text_data or str(json_data)

    @property
    def is_success(self) -> bool:
        return 200 <= self.status_code < 300

    def json(self) -> dict | None:
        return self._json_data

    @property
    def text(self) -> str | None:
        return self._text_data


class MockG8eHTTPClient:
    """Mock HTTPClient  used by InternalHttpClient._http."""

    def __init__(self, response: MockG8eHTTPResponse = None, side_effect: Exception = None):
        self._response = response
        self._side_effect = side_effect
        self._captured_method: str
        self._captured_url: str = ""
        self._captured_json_data: dict = {}
        self._captured_headers: dict = {}
        self._captured_params: dict = {}

    async def post(self, url: str, json_data: dict, headers: dict, context=None, **kwargs):
        self._captured_method = "POST"
        self._captured_url = url
        self._captured_json_data = json_data
        self._captured_headers = headers or {}
        if self._side_effect:
            raise self._side_effect
        return self._response

    async def get(self, url: str, headers: dict, params: dict, context=None, **kwargs):
        self._captured_method = "GET"
        self._captured_url = url
        self._captured_headers = headers or {}
        self._captured_params = params
        if self._side_effect:
            raise self._side_effect
        return self._response

    async def close(self):
        pass


def _make_operator_doc_dict(**overrides):
    base = {
        "operator_id": "op-abc123",
        "user_id": "user-1",
        "status": OperatorStatus.BOUND,
        "web_session_id": "sess-999",
        "operator_session_id": "op-sess-111",
        "last_heartbeat": datetime.now(UTC).isoformat(),
    }
    base.update(overrides)
    return base

# =============================================================================
# SETTINGS PROPERTY
# =============================================================================

class TestG8edHttpClientSettings:

    async def test_settings_returns_configured_settings(self, mock_settings):
        """Accessing settings returns the configured Settings object."""
        client = InternalHttpClient(settings=mock_settings)
        assert client.settings is mock_settings

    async def test_configure_updates_settings(self, mock_settings):
        """configure() replaces settings after construction."""
        initial = mock_settings.model_copy(update={"auth": AuthSettings(internal_auth_token="initial-token")})
        client = InternalHttpClient(settings=initial)
        client.configure(mock_settings)
        assert client.settings is mock_settings


# =============================================================================
# AUTH HEADERS
# =============================================================================

class TestG8edHttpClientAuthHeaders:

    async def test_auth_headers_uses_g8eheaders_constant(self, mock_settings):
        """_auth_headers uses G8eHeaders.SOURCE_COMPONENT, not a hardcoded string."""
        mock_settings.auth.internal_auth_token = "test-token"
        client = InternalHttpClient(settings=mock_settings)
        headers = client._auth_headers()
        assert G8eHeaders.SOURCE_COMPONENT in headers
        assert headers[G8eHeaders.SOURCE_COMPONENT] == ComponentName.G8EE

    async def test_auth_headers_includes_internal_auth_token(self, mock_settings):
        """_auth_headers includes the internal auth_token."""
        mock_settings.auth.internal_auth_token = "test-token"
        client = InternalHttpClient(settings=mock_settings)
        headers = client._auth_headers()
        expected_token = "test-token"
        assert headers[INTERNAL_AUTH_HEADER] == expected_token

    async def test_auth_headers_raises_configuration_error_when_token_is_none(self, mock_settings):
        """_auth_headers raises ConfigurationError when internal_auth_token is None."""
        mock_settings.auth.internal_auth_token = None
        client = InternalHttpClient(settings=mock_settings)
        with pytest.raises(ConfigurationError):
            client._auth_headers()

    async def test_auth_headers_raises_configuration_error_when_token_is_empty(self, mock_settings):
        """_auth_headers raises ConfigurationError when internal_auth_token is empty string."""
        mock_settings.auth.internal_auth_token = ""
        client = InternalHttpClient(settings=mock_settings)
        with pytest.raises(ConfigurationError):
            client._auth_headers()


# =============================================================================
# PUSH SSE EVENT
# =============================================================================

class TestG8edHttpClientPushSSEEvent:

    @staticmethod
    def _make_test_event(event_type: EventType = EventType.CASE_UPDATED) -> SessionEvent:
        return SessionEvent(
            event_type=event_type,
            payload=ChatResponseChunkPayload(),
            web_session_id="sess-abc",
            user_id="user-1",
            case_id="case-1",
            investigation_id="inv-1",
        )

    async def test_push_sse_event_success(self, mock_settings):
        """Returns typed SSEPushResponse with success=True and delivered count."""
        mock_settings.auth.internal_auth_token = "test-token"
        client = InternalHttpClient(settings=mock_settings)
        client._http = MockG8eHTTPClient(
            response=MockG8eHTTPResponse(200, {"success": True, "delivered": 3})
        )
        result = await client.push_sse_event(self._make_test_event())
        assert isinstance(result, SSEPushResponse)
        assert result.success is True
        assert result.delivered == 3

    async def test_push_sse_event_zero_delivered_is_success(self, mock_settings):
        """BackgroundEvent fan-out to zero sessions is a legitimate success — not an error."""
        mock_settings.auth.internal_auth_token = "test-token"
        client = InternalHttpClient(settings=mock_settings)
        client._http = MockG8eHTTPClient(
            response=MockG8eHTTPResponse(200, {"success": True, "delivered": 0})
        )
        result = await client.push_sse_event(self._make_test_event())
        assert result.success is True
        assert result.delivered == 0

    async def test_push_sse_event_http_error_raises_with_status(self, mock_settings):
        """Non-2xx responses raise NetworkError with the originating status code preserved."""
        mock_settings.auth.internal_auth_token = "test-token"
        client = InternalHttpClient(settings=mock_settings)
        client._http = MockG8eHTTPClient(
            response=MockG8eHTTPResponse(503, {"error": "unavailable"})
        )
        with pytest.raises(NetworkError) as exc_info:
            await client.push_sse_event(self._make_test_event())
        assert exc_info.value.error_detail.details["status_code"] == 503

    async def test_push_sse_event_http_500_raises_not_returns_false(self, mock_settings):
        """Regression: a genuine 500 from g8ed must not collapse into a success-False shape.

        Previously push_sse_event silently returned False on non-2xx, which
        EventService.publish then re-raised as a generic NetworkError with no
        status code — indistinguishable from a zero-delivery fan-out. The fix
        raises NetworkError with the HTTP status preserved in details.
        """
        mock_settings.auth.internal_auth_token = "test-token"
        client = InternalHttpClient(settings=mock_settings)
        client._http = MockG8eHTTPClient(
            response=MockG8eHTTPResponse(500, {"error": "crash"})
        )
        with pytest.raises(NetworkError) as exc_info:
            await client.push_sse_event(self._make_test_event())
        assert exc_info.value.error_detail.details["status_code"] == 500

    async def test_push_sse_event_exception(self, mock_settings):
        """Raises NetworkError on network exception."""
        mock_settings.auth.internal_auth_token = "test-token"
        client = InternalHttpClient(settings=mock_settings)
        client._http = MockG8eHTTPClient(side_effect=OSError("Connection refused"))
        with pytest.raises(NetworkError):
            await client.push_sse_event(self._make_test_event())

    async def test_push_sse_event_sends_wire_payload(self, mock_settings):
        """Payload sent to g8ed matches SessionEvent.flatten_for_wire() shape."""
        mock_settings.auth.internal_auth_token = "test-token"
        mock_http = MockG8eHTTPClient(
            response=MockG8eHTTPResponse(200, {"success": True})
        )
        client = InternalHttpClient(settings=mock_settings)
        client._http = mock_http

        event = self._make_test_event(EventType.CASE_CREATED)
        result = await client.push_sse_event(event)
        assert result.success is True

        assert mock_http._captured_url == "/api/internal/sse/push"
        wire = mock_http._captured_json_data
        assert wire is not None
        assert wire["web_session_id"] == "sess-abc"
        assert wire["event"]["type"] == EventType.CASE_CREATED

    async def test_push_sse_event_sends_auth_headers(self, mock_settings):
        """push_sse_event sends internal auth headers to g8es."""
        test_token = "dynamic-test-token-789"
        mock_settings.auth.internal_auth_token = test_token
        client = InternalHttpClient(settings=mock_settings)
        mock_http = MockG8eHTTPClient(
            response=MockG8eHTTPResponse(200, {"success": True})
        )
        client._http = mock_http
        
        event = self._make_test_event(EventType.LLM_CHAT_ITERATION_STARTED)
        await client.push_sse_event(event)
        
        assert mock_http._captured_headers[INTERNAL_AUTH_HEADER] == test_token
        assert mock_http._captured_headers[G8eHeaders.SOURCE_COMPONENT] == ComponentName.G8EE


# =============================================================================
# GRANT INTENT
# =============================================================================

class TestG8edHttpClientGrantIntent:

    async def test_success_returns_typed_result(self, mock_settings):
        """Returns IntentOperationResult with success=True and granted_intents."""
        from app.models.http_context import G8eHttpContext
        mock_settings.auth.internal_auth_token = "test-token"
        client = InternalHttpClient(settings=mock_settings)
        client._http = MockG8eHTTPClient(
            response=MockG8eHTTPResponse(200, {
                "success": True,
                "granted_intents": ["ec2_discovery", "s3_read"],
            })
        )
        context = G8eHttpContext(
            web_session_id="s1",
            user_id="u1",
            case_id="c1",
            investigation_id="i1",
            source_component=ComponentName.G8EE,
        )
        result = await client.grant_intent("op-123", "ec2_discovery", context)
        assert isinstance(result, IntentOperationResult)
        assert result.success is True
        assert "ec2_discovery" in result.granted_intents

    async def test_server_error_returns_failure(self, mock_settings):
        """Returns failure result on 500."""
        from app.models.http_context import G8eHttpContext
        mock_settings.auth.internal_auth_token = "test-token"
        client = InternalHttpClient(settings=mock_settings)
        client._http = MockG8eHTTPClient(
            response=MockG8eHTTPResponse(500, {"success": False, "error": "server error"})
        )
        context = G8eHttpContext(
            web_session_id="s1",
            user_id="u1",
            case_id="c1",
            investigation_id="i1",
            source_component=ComponentName.G8EE,
        )
        result = await client.grant_intent("op-123", "ec2_discovery", context)
        assert result.success is False

    async def test_network_exception_raises_network_error(self, mock_settings):
        """Raises NetworkError on network exception."""
        from app.models.http_context import G8eHttpContext
        mock_settings.auth.internal_auth_token = "test-token"
        client = InternalHttpClient(settings=mock_settings)
        client._http = MockG8eHTTPClient(side_effect=Exception("Network error"))
        context = G8eHttpContext(
            web_session_id="s1",
            user_id="u1",
            case_id="c1",
            investigation_id="i1",
            source_component=ComponentName.G8EE,
        )
        with pytest.raises(NetworkError) as exc_info:
            await client.grant_intent("op-123", "ec2_discovery", context)
        assert "Network error" in str(exc_info.value)

    async def test_sends_correct_url_and_payload(self, mock_settings):
        """Sends POST to the correct URL with intent payload."""
        from app.models.http_context import G8eHttpContext
        mock_settings.auth.internal_auth_token = "test-token"
        mock_http = MockG8eHTTPClient(
            response=MockG8eHTTPResponse(200, {"success": True, "granted_intents": []})
        )
        client = InternalHttpClient(settings=mock_settings)
        client._http = mock_http

        context = G8eHttpContext(
            web_session_id="s1",
            user_id="u1",
            case_id="c1",
            investigation_id="i1",
            source_component=ComponentName.G8EE,
        )
        await client.grant_intent("op-test-456", "s3_read", context)

        assert mock_http._captured_url == "/api/internal/operators/op-test-456/grant-intent"
        assert mock_http._captured_json_data == {"intent": "s3_read"}

    async def test_sends_correct_auth_headers(self, mock_settings):
        """grant_intent sends internal auth headers to g8es."""
        from app.models.http_context import G8eHttpContext
        test_token = "grant-token-123"
        mock_settings.auth.internal_auth_token = test_token
        mock_http = MockG8eHTTPClient(
            response=MockG8eHTTPResponse(200, {"success": True, "granted": True})
        )
        client = InternalHttpClient(settings=mock_settings)
        client._http = mock_http
        
        context = G8eHttpContext(
            web_session_id="s1",
            user_id="u1",
            case_id="c1",
            investigation_id="i1",
            source_component=ComponentName.G8EE,
        )
        await client.grant_intent("op-1", "intent-1", context)
        
        assert mock_http._captured_headers[INTERNAL_AUTH_HEADER] == test_token
        assert mock_http._captured_headers[G8eHeaders.SOURCE_COMPONENT] == ComponentName.G8EE


# =============================================================================
# REVOKE INTENT
# =============================================================================

class TestG8edHttpClientRevokeIntent:

    async def test_success_returns_typed_result(self, mock_settings):
        """Returns IntentOperationResult with success=True and remaining granted_intents."""
        from app.models.http_context import G8eHttpContext
        mock_settings.auth.internal_auth_token = "test-token"
        client = InternalHttpClient(settings=mock_settings)
        client._http = MockG8eHTTPClient(
            response=MockG8eHTTPResponse(200, {
                "success": True,
                "granted_intents": ["s3_read"],
            })
        )
        context = G8eHttpContext(
            web_session_id="s1",
            user_id="u1",
            case_id="c1",
            investigation_id="i1",
            source_component=ComponentName.G8EE,
        )
        result = await client.revoke_intent("op-123", "ec2_discovery", context)
        assert isinstance(result, IntentOperationResult)
        assert result.success is True
        assert result.granted_intents == ["s3_read"]

    async def test_failure_response_returns_error(self, mock_settings):
        """Returns IntentOperationResult with success=False and error."""
        from app.models.http_context import G8eHttpContext
        mock_settings.auth.internal_auth_token = "test-token"
        client = InternalHttpClient(settings=mock_settings)
        client._http = MockG8eHTTPClient(
            response=MockG8eHTTPResponse(400, {
                "success": False,
                "error": "Intent not found",
            })
        )
        context = G8eHttpContext(
            web_session_id="s1",
            user_id="u1",
            case_id="c1",
            investigation_id="i1",
            source_component=ComponentName.G8EE,
        )
        result = await client.revoke_intent("op-123", "unknown_intent", context)
        assert result.success is False
        assert result.error is not None
        assert "Intent not found" in result.error

    async def test_server_error_returns_failure(self, mock_settings):
        """Returns failure result on 500."""
        from app.models.http_context import G8eHttpContext
        mock_settings.auth.internal_auth_token = "test-token"
        client = InternalHttpClient(settings=mock_settings)
        client._http = MockG8eHTTPClient(
            response=MockG8eHTTPResponse(500, {"success": False, "error": "server error"})
        )
        context = G8eHttpContext(
            web_session_id="s1",
            user_id="u1",
            case_id="c1",
            investigation_id="i1",
            source_component=ComponentName.G8EE,
        )
        result = await client.revoke_intent("op-123", "ec2_discovery", context)
        assert result.success is False

    async def test_network_exception_raises_network_error(self, mock_settings):
        """Raises NetworkError on network exception."""
        from app.models.http_context import G8eHttpContext
        mock_settings.auth.internal_auth_token = "test-token"
        client = InternalHttpClient(settings=mock_settings)
        client._http = MockG8eHTTPClient(side_effect=OSError("Connection refused"))
        context = G8eHttpContext(
            web_session_id="s1",
            user_id="u1",
            case_id="c1",
            investigation_id="i1",
            source_component=ComponentName.G8EE,
        )
        with pytest.raises(NetworkError) as exc_info:
            await client.revoke_intent("op-123", "ec2_discovery", context)
        assert "Connection refused" in str(exc_info.value)

    async def test_timeout_exception_raises_network_error(self, mock_settings):
        """Raises NetworkError on timeout exception."""
        from app.models.http_context import G8eHttpContext
        mock_settings.auth.internal_auth_token = "test-token"
        client = InternalHttpClient(settings=mock_settings)
        client._http = MockG8eHTTPClient(side_effect=Exception("Timeout"))
        context = G8eHttpContext(
            web_session_id="s1",
            user_id="u1",
            case_id="c1",
            investigation_id="i1",
            source_component=ComponentName.G8EE,
        )
        with pytest.raises(NetworkError) as exc_info:
            await client.revoke_intent("op-123", "ec2_discovery", context)
        assert "Timeout" in str(exc_info.value)

    async def test_sends_correct_url_and_payload(self, mock_settings):
        """Sends POST to the correct URL with intent payload."""
        from app.models.http_context import G8eHttpContext
        mock_settings.auth.internal_auth_token = "test-token"
        mock_http = MockG8eHTTPClient(
            response=MockG8eHTTPResponse(200, {"success": True})
        )
        client = InternalHttpClient(settings=mock_settings)
        client._http = mock_http

        context = G8eHttpContext(
            web_session_id="s1",
            user_id="u1",
            case_id="c1",
            investigation_id="i1",
            source_component=ComponentName.G8EE,
        )
        await client.revoke_intent("op-test-456", "s3_write", context)

        assert mock_http._captured_url == "/api/internal/operators/op-test-456/revoke-intent"
        assert mock_http._captured_json_data == {"intent": "s3_write"}

    async def test_sends_correct_auth_headers(self, mock_settings):
        """Sends correct auth headers using G8eHeaders constant."""
        from app.models.http_context import G8eHttpContext
        test_token = "revoke-token-456"
        mock_settings.auth.internal_auth_token = test_token
        mock_http = MockG8eHTTPClient(
            response=MockG8eHTTPResponse(200, {"success": True})
        )
        client = InternalHttpClient(settings=mock_settings)
        client._http = mock_http

        context = G8eHttpContext(
            web_session_id="s1",
            user_id="u1",
            case_id="c1",
            investigation_id="i1",
            source_component=ComponentName.G8EE,
        )
        await client.revoke_intent("op-123", "ec2_discovery", context)

        assert mock_http._captured_headers[INTERNAL_AUTH_HEADER] == test_token
        assert mock_http._captured_headers[G8eHeaders.SOURCE_COMPONENT] == ComponentName.G8EE



# =============================================================================
# TYPED REQUEST/RESPONSE MODELS
# =============================================================================

class TestG8edHttpClientTypedModels:

    async def test_grant_intent_sends_typed_request_payload(self, mock_settings):
        """grant_intent sends IntentRequestPayload via flatten_for_wire."""
        from app.models.http_context import G8eHttpContext
        mock_settings.auth.internal_auth_token = "test-token"
        mock_http = MockG8eHTTPClient(
            response=MockG8eHTTPResponse(200, {"success": True, "granted_intents": ["ec2_discovery"]})
        )
        client = InternalHttpClient(settings=mock_settings)
        client._http = mock_http

        context = G8eHttpContext(
            web_session_id="s1",
            user_id="u1",
            case_id="c1",
            investigation_id="i1",
            source_component=ComponentName.G8EE,
        )
        await client.grant_intent("op-123", "ec2_discovery", context)

        expected = IntentRequestPayload(intent="ec2_discovery").model_dump(mode="json")
        assert mock_http._captured_json_data == expected

    async def test_revoke_intent_sends_typed_request_payload(self, mock_settings):
        """revoke_intent sends IntentRequestPayload via flatten_for_wire."""
        from app.models.http_context import G8eHttpContext
        mock_settings.auth.internal_auth_token = "test-token"
        mock_http = MockG8eHTTPClient(
            response=MockG8eHTTPResponse(200, {"success": True, "granted_intents": []})
        )
        client = InternalHttpClient(settings=mock_settings)
        client._http = mock_http

        context = G8eHttpContext(
            web_session_id="s1",
            user_id="u1",
            case_id="c1",
            investigation_id="i1",
            source_component=ComponentName.G8EE,
        )
        await client.revoke_intent("op-123", "s3_write", context)

        expected = IntentRequestPayload(intent="s3_write").model_dump(mode="json")
        assert mock_http._captured_json_data == expected

    async def test_grant_intent_response_parses_granted_intents(self):
        """GrantIntentResponse correctly parses granted_intents."""
        raw = {"success": True, "operator_id": "op-1", "granted_intents": ["ec2_discovery", "s3_read"]}
        result = GrantIntentResponse.model_validate(raw)
        assert result.success is True
        assert result.granted_intents == ["ec2_discovery", "s3_read"]

    async def test_revoke_intent_response_defaults_empty_intents(self):
        """RevokeIntentResponse defaults granted_intents to empty list."""
        raw = {"success": False, "error": "denied"}
        result = RevokeIntentResponse.model_validate(raw)
        assert result.success is False
        assert result.granted_intents == []
        assert result.error == "denied"

    async def test_sse_push_response_parses_delivered(self):
        """SSEPushResponse correctly parses the delivered count."""
        raw = {"success": True, "delivered": 2}
        result = SSEPushResponse.model_validate(raw)
        assert result.success is True
        assert result.delivered == 2

    async def test_sse_push_response_defaults_delivered_to_zero(self):
        """SSEPushResponse defaults delivered to 0 when absent — legitimate empty fan-out."""
        raw = {"success": True}
        result = SSEPushResponse.model_validate(raw)
        assert result.success is True
        assert result.delivered == 0


# =============================================================================
# bind_operators idempotent path uses OperatorStatus.BOUND
# =============================================================================


class TestG8edHttpClientBindOperatorUsesEnum:

    def _make_client(self, mock_http, settings):
        client = InternalHttpClient (settings=settings)
        client._http = mock_http
        return client

    async def test_already_bound_returns_true_via_operator_status_enum(self, mock_settings):
        """400 with current_status=OperatorStatus.BOUND is treated as idempotent success."""
        from app.models.http_context import G8eHttpContext
        from unittest.mock import AsyncMock, MagicMock
        class _MockHTTP:
            async def post(self, url, json_data=None, headers=None, **kwargs):
                err = NetworkError(
                    "Operator already bound",
                    component="test",
                )
                err.error_detail.details = {
                    "status_code": 400,
                    "response": {"current_status": OperatorStatus.BOUND},
                }
                raise err

        client = self._make_client(_MockHTTP(), mock_settings)
        
        # Mock operator_data_service and inject it
        mock_operator_service = MagicMock()
        mock_operator_service.bind_operators = AsyncMock(return_value=True)
        client.set_operator_data_service(mock_operator_service)
        
        context = G8eHttpContext(
            web_session_id="web-session-test",
            user_id="user-1",
            case_id="case-1",
            investigation_id="inv-1",
            source_component=ComponentName.G8EE,
        )
        result = await client.bind_operators(["op-already-bound"], "web-session-test", context)
        assert result is True

    async def test_already_bound_check_uses_enum_not_literal(self):
        """Verify the comparison value equals OperatorStatus.BOUND string."""
        assert OperatorStatus.BOUND.value == "bound"
        assert str(OperatorStatus.BOUND.value) == "bound"
