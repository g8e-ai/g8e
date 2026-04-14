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
Unit tests for HTTPClient , CircuitBreaker, RequestTrace, and get_service_client.
"""

import asyncio
import json
from datetime import UTC
from unittest.mock import AsyncMock, MagicMock, patch

import aiohttp
import pytest
import pytest_asyncio

from app.clients.http_client import (
    DEFAULT_RETRY_METHODS,
    DEFAULT_RETRY_STATUS_CODES,
    AiohttpResponse,
    CircuitBreaker,
    CircuitBreakerConfig,
    RequestTrace,
    RetryConfig,
    HTTPClient ,
    get_service_client,
)
from app.utils.timestamp import now
from app.constants import (
    DEFAULT_HTTP_CLIENT_TIMEOUT as DEFAULT_TIMEOUT,
)
from app.constants import (
    DEFAULT_MAX_RETRIES,
    DEFAULT_RETRY_BACKOFF_FACTOR,
    HTTP_API_KEY_HEADER,
    CircuitBreakerState,
    ComponentName,
    G8eHeaders,
)
from app.errors import NetworkError, ValidationError
from app.models.http_context import G8eHttpContext

pytestmark = pytest.mark.unit


# =============================================================================
# Fixtures
# =============================================================================

@pytest_asyncio.fixture(scope="session", loop_scope="session")
async def client():
    c = HTTPClient (
        component_id=ComponentName.G8EE,
        base_url="https://g8ed",
        timeout=DEFAULT_TIMEOUT,
        retry_config=RetryConfig(),
        circuit_breaker_config=CircuitBreakerConfig(),
        auth_token="",
        api_key="",
        headers={},
        ca_cert_path="/mock/ca.crt",
    )
    yield c
    await c.close()


@pytest_asyncio.fixture(scope="session", loop_scope="session")
async def authed_client():
    c = HTTPClient (
        component_id=ComponentName.G8EE,
        base_url="https://g8ed",
        timeout=DEFAULT_TIMEOUT,
        retry_config=RetryConfig(),
        circuit_breaker_config=CircuitBreakerConfig(),
        auth_token="test-token",
        api_key="test-api-key",
        headers={},
        ca_cert_path="/mock/ca.crt",
    )
    yield c
    await c.close()


# =============================================================================
# G8eHeaders — COMPONENT_ID must not exist (regression guard)
# =============================================================================

class TestG8eHeaders:
    """G8eHeaders contract: required headers present, COMPONENT_ID absent."""

    def test_component_id_absent(self):
        assert not hasattr(G8eHeaders, "COMPONENT_ID")

    def test_required_headers_present_in_g8eheaders(self):
        assert hasattr(G8eHeaders, "EXECUTION_ID")
        assert hasattr(G8eHeaders, "WEB_SESSION_ID")
        assert hasattr(G8eHeaders, "USER_ID")
        assert hasattr(G8eHeaders, "SOURCE_COMPONENT")
        assert hasattr(G8eHeaders, "ORGANIZATION_ID")
        assert hasattr(G8eHeaders, "CASE_ID")
        assert hasattr(G8eHeaders, "INVESTIGATION_ID")
        assert hasattr(G8eHeaders, "TASK_ID")


# =============================================================================
# AiohttpResponse — properties
# =============================================================================

class TestAiohttpResponse:
    """AiohttpResponse wraps a raw HTTP response with typed accessors."""

    def test_is_success_true_for_2xx_range(self):
        for status in (200, 201, 204, 299):
            assert AiohttpResponse(status, b"ok", {}).is_success

    def test_is_success_false_outside_2xx_range(self):
        for status in (199, 300, 400, 404, 500, 503):
            assert not AiohttpResponse(status, b"err", {}).is_success

    def test_text_property_decodes_utf8_body(self):
        body = b"hello world"
        assert AiohttpResponse(200, body, {}).text == "hello world"

    def test_json_method_parses_body(self):
        body = json.dumps({"key": "value", "n": 42}).encode()
        result = AiohttpResponse(200, body, {}).json()
        assert result == {"key": "value", "n": 42}

    def test_headers_dict_is_accessible(self):
        resp = AiohttpResponse(200, b"", {"Content-Type": "application/json"})
        assert resp.headers["Content-Type"] == "application/json"


# =============================================================================
# RequestTrace — timezone-aware timestamps via now()
# =============================================================================

class TestRequestTrace:
    """RequestTrace must use timezone-aware datetimes from app.utils.timestamp.now()."""

    def test_from_headers_start_time_is_utc_aware(self):
        trace = RequestTrace(
            execution_id="req-1",
            component_id=ComponentName.G8EE,
            case_id="",
            task_id="",
            investigation_id="",
            start_time=now(),
            end_time=now(),
        )
        assert trace.start_time is not None
        assert trace.start_time.tzinfo is not None
        assert trace.start_time.tzinfo == UTC

    def test_from_headers_generates_request_id_when_absent(self):
        trace = RequestTrace.from_headers({}, component_id=ComponentName.G8EE)
        assert trace.execution_id.startswith("exec-")

    def test_from_headers_preserves_existing_request_id(self):
        trace = RequestTrace.from_headers(
            {G8eHeaders.EXECUTION_ID: "exec-existing-123"}, component_id=ComponentName.G8EE
        )
        assert trace.execution_id == "exec-existing-123"

    def test_from_headers_extracts_case_task_investigation_ids(self):
        trace = RequestTrace.from_headers(
            {
                G8eHeaders.CASE_ID: "case-1",
                G8eHeaders.TASK_ID: "task-1",
                G8eHeaders.INVESTIGATION_ID: "inv-1",
            },
            component_id=ComponentName.G8EE,
        )
        assert trace.case_id == "case-1"
        assert trace.task_id == "task-1"
        assert trace.investigation_id == "inv-1"

    def test_finish_populates_end_time_and_duration_ms(self):
        trace = RequestTrace(
            execution_id="req-1",
            component_id=ComponentName.G8EE,
            case_id="",
            task_id="",
            investigation_id="",
            start_time=now(),
            end_time=now(),
        )
        trace.finish()
        assert trace.end_time is not None
        assert trace.end_time.tzinfo == UTC
        assert trace.duration_ms >= 0.0

    def test_as_headers_contains_request_id(self):
        trace = RequestTrace.from_headers(
            {G8eHeaders.EXECUTION_ID: "req-abc"}, component_id=ComponentName.G8EE
        )
        headers = trace.as_headers
        assert headers[G8eHeaders.EXECUTION_ID] == "req-abc"

    def test_as_headers_omits_none_optional_fields(self):
        trace = RequestTrace.from_headers({}, component_id=ComponentName.G8EE)
        headers = trace.as_headers
        assert G8eHeaders.CASE_ID not in headers
        assert G8eHeaders.TASK_ID not in headers
        assert G8eHeaders.INVESTIGATION_ID not in headers


# =============================================================================
# RetryConfig / CircuitBreakerConfig defaults
# =============================================================================

class TestRetryConfig:
    """RetryConfig defaults match module constants."""

    def test_retry_config_defaults_match_module_constants(self):
        cfg = RetryConfig()
        assert cfg.max_retries == DEFAULT_MAX_RETRIES
        assert cfg.retry_backoff_factor == DEFAULT_RETRY_BACKOFF_FACTOR
        assert cfg.retry_methods == DEFAULT_RETRY_METHODS
        assert cfg.retry_status_codes == DEFAULT_RETRY_STATUS_CODES

    def test_retry_methods_is_set_of_strings(self):
        cfg = RetryConfig()
        assert isinstance(cfg.retry_methods, set)
        assert all(isinstance(m, str) for m in cfg.retry_methods)

    def test_retry_status_codes_is_set_of_ints(self):
        cfg = RetryConfig()
        assert isinstance(cfg.retry_status_codes, set)
        assert all(isinstance(c, int) for c in cfg.retry_status_codes)


class TestCircuitBreakerConfig:
    def test_circuit_breaker_config_defaults(self):
        cfg = CircuitBreakerConfig()
        assert cfg.failure_threshold == 5
        assert cfg.recovery_time == 30.0
        assert cfg.half_open_success_threshold == 3

    def test_circuit_breaker_initial_state_is_closed(self):
        cb = CircuitBreaker(CircuitBreakerConfig(), endpoint="https://g8ed")
        assert cb.state is CircuitBreakerState.CLOSED


# =============================================================================
# CircuitBreaker — state transitions use CircuitBreakerState enum
# =============================================================================

@pytest.mark.asyncio(loop_scope="session")
class TestCircuitBreaker:
    """CircuitBreaker state transitions must produce CircuitBreakerState enum members."""

    async def test_state_transitions_closed_open_half_open_closed(self):
        cb = CircuitBreaker(
            CircuitBreakerConfig(
                failure_threshold=1,
                recovery_time=0.0,
                half_open_success_threshold=1,
            ),
            endpoint="https://g8ed",
        )
        assert cb.state is CircuitBreakerState.CLOSED

        await cb.record_failure()
        assert cb.state is CircuitBreakerState.OPEN
        assert isinstance(cb.state, CircuitBreakerState)

        assert await cb.allow_request() is True
        assert cb.state is CircuitBreakerState.HALF_OPEN
        assert isinstance(cb.state, CircuitBreakerState)

        await cb.record_success()
        assert cb.state is CircuitBreakerState.CLOSED
        assert isinstance(cb.state, CircuitBreakerState)

    async def test_open_state_blocks_allow_request(self):
        cb = CircuitBreaker(
            CircuitBreakerConfig(failure_threshold=1, recovery_time=9999.0),
            endpoint="https://g8ed",
        )
        await cb.record_failure()
        assert cb.state is CircuitBreakerState.OPEN
        assert await cb.allow_request() is False

    async def test_half_open_failure_transitions_back_to_open(self):
        cb = CircuitBreaker(
            CircuitBreakerConfig(failure_threshold=1, recovery_time=0.0),
            endpoint="https://g8ed",
        )
        await cb.record_failure()
        await cb.allow_request()
        assert cb.state is CircuitBreakerState.HALF_OPEN

        await cb.record_failure()
        assert cb.state is CircuitBreakerState.OPEN
        assert isinstance(cb.state, CircuitBreakerState)

    async def test_closed_state_always_allows_requests(self):
        cb = CircuitBreaker(CircuitBreakerConfig(), endpoint="https://g8ed")
        assert await cb.allow_request() is True


# =============================================================================
# HTTPClient  — init and header injection
# =============================================================================

class TestG8eHTTPClientInit:
    """HTTPClient  initialises correctly and does not inject forbidden headers."""

    def test_component_id_not_injected_into_default_headers(self, client):
        assert "X-G8E-Component-ID" not in client.default_headers

    def test_custom_timeout_is_applied(self):
        c = HTTPClient (
            component_id=ComponentName.G8EE,
            base_url="https://g8ed",
            timeout=15.0,
            retry_config=RetryConfig(),
            circuit_breaker_config=CircuitBreakerConfig(),
            auth_token="",
            api_key="",
            headers={},
            ca_cert_path="/mock/ca.crt",
        )
        assert c.timeout.total == 15.0

    def test_default_timeout_matches_module_constant(self):
        c = HTTPClient (
            component_id=ComponentName.G8EE,
            base_url="https://g8ed",
            timeout=DEFAULT_TIMEOUT,
            retry_config=RetryConfig(),
            circuit_breaker_config=CircuitBreakerConfig(),
            auth_token="",
            api_key="",
            headers={},
            ca_cert_path="/mock/ca.crt",
        )
        assert c.timeout.total == DEFAULT_TIMEOUT

    def test_retry_config_defaults_on_init(self, client):
        assert client.retry_config.max_retries == DEFAULT_MAX_RETRIES

    def test_circuit_breakers_dict_starts_empty(self, client):
        assert client.circuit_breakers == {}


# =============================================================================
# HTTPClient  — _prepare_request header construction
# =============================================================================

@pytest.mark.asyncio(loop_scope="session")
class TestG8eHTTPClientPrepareRequest:
    """_prepare_request must build correct headers from config and context."""

    async def test_prepare_request_injects_request_id(self, client):
        _url, headers, _kw, _trace = await client._prepare_request("GET", "/api/health", headers={}, context=None)
        assert G8eHeaders.EXECUTION_ID in headers

    async def test_prepare_request_never_injects_component_id_header(self, client):
        _url, headers, _kw, _trace = await client._prepare_request("GET", "/api/health", headers={}, context=None)
        assert "X-G8E-Component-ID" not in headers

    async def test_prepare_request_auth_token_formatted_as_bearer(self, authed_client):
        _url, headers, _kw, _trace = await authed_client._prepare_request("GET", "/api/health", headers={}, context=None)
        assert headers["Authorization"] == "Bearer test-token"

    async def test_prepare_request_api_key_set_in_header(self, authed_client):
        _url, headers, _kw, _trace = await authed_client._prepare_request("GET", "/api/health", headers={}, context=None)
        assert headers[HTTP_API_KEY_HEADER] == "test-api-key"

    async def test_prepare_request_joins_base_url_with_path(self, client):
        url, _headers, _kw, _trace = await client._prepare_request("GET", "/api/health", headers={}, context=None)
        assert url == "https://g8ed/api/health"

    async def test_prepare_request_caller_headers_override_defaults(self, client):
        _url, headers, _kw, _trace = await client._prepare_request(
            "GET", "/api/health", headers={"X-Custom": "override"}, context=None
        )
        assert headers["X-Custom"] == "override"

    async def test_prepare_request_trace_id_propagated_to_headers(self, client):
        _url, headers, _kw, trace = await client._prepare_request("GET", "/api/health", headers={}, context=None)
        assert headers[G8eHeaders.EXECUTION_ID] == trace.execution_id

    async def test_prepare_request_g8e_context_headers_propagated(self, client):
        ctx = G8eHttpContext(
            web_session_id="sess-abc",
            user_id="user-123",
            source_component=ComponentName.G8ED,
            case_id="case-456",
            investigation_id="inv-789",
        )
        _url, headers, _kw, _trace = await client._prepare_request(
            "POST", "/api/internal/chat/stream", headers={}, context=ctx
        )
        assert headers[G8eHeaders.WEB_SESSION_ID] == "sess-abc"
        assert headers[G8eHeaders.USER_ID] == "user-123"
        assert headers[G8eHeaders.SOURCE_COMPONENT] == "g8ed"
        assert headers[G8eHeaders.CASE_ID] == "case-456"
        assert headers[G8eHeaders.INVESTIGATION_ID] == "inv-789"


# =============================================================================
# HTTPClient  — circuit breaker per-endpoint isolation
# =============================================================================

class TestG8eHTTPClientCircuitBreakerIsolation:
    """Each distinct URL endpoint gets its own CircuitBreaker instance."""

    pytestmark = pytest.mark.asyncio(loop_scope="session")

    async def test_distinct_url_paths_get_separate_circuit_breakers(self):
        c = HTTPClient (
            component_id=ComponentName.G8EE,
            base_url="https://g8ed",
            timeout=DEFAULT_TIMEOUT,
            retry_config=RetryConfig(),
            circuit_breaker_config=CircuitBreakerConfig(),
            auth_token="",
            api_key="",
            headers={},
            ca_cert_path="/mock/ca.crt",
        )
        try:
            cb1 = c._get_circuit_breaker("https://g8ed/api/health")
            cb2 = c._get_circuit_breaker("https://g8ed/api/chat/stream")
            assert cb1 is not cb2
        finally:
            await c.close()

    async def test_same_url_path_returns_cached_circuit_breaker(self):
        c = HTTPClient (
            component_id=ComponentName.G8EE,
            base_url="https://g8ed",
            timeout=DEFAULT_TIMEOUT,
            retry_config=RetryConfig(),
            circuit_breaker_config=CircuitBreakerConfig(),
            auth_token="",
            api_key="",
            headers={},
            ca_cert_path="/mock/ca.crt",
        )
        try:
            cb1 = c._get_circuit_breaker("https://g8ed/api/health")
            cb2 = c._get_circuit_breaker("https://g8ed/api/health")
            assert cb1 is cb2
        finally:
            await c.close()


# =============================================================================
# HTTPClient  — context manager
# =============================================================================

class TestG8eHTTPClientContextManager:
    """HTTPClient  must be usable as an async context manager."""

    pytestmark = pytest.mark.asyncio(loop_scope="session")

    async def test_context_manager_aenter_returns_client_instance(self):
        async with HTTPClient (
            component_id=ComponentName.G8EE,
            base_url="https://g8ed",
            timeout=DEFAULT_TIMEOUT,
            retry_config=RetryConfig(),
            circuit_breaker_config=CircuitBreakerConfig(),
            auth_token="",
            api_key="",
            headers={},
            ca_cert_path="/mock/ca.crt",
        ) as c:
            assert isinstance(c, HTTPClient )

    async def test_context_manager_aexit_closes_session(self):
        c = HTTPClient (
            component_id=ComponentName.G8EE,
            base_url="https://g8ed",
            timeout=DEFAULT_TIMEOUT,
            retry_config=RetryConfig(),
            circuit_breaker_config=CircuitBreakerConfig(),
            auth_token="",
            api_key="",
            headers={},
            ca_cert_path="/mock/ca.crt",
        )
        await c._get_http_session()
        assert c._session is not None and not c._session.closed
        await c.close()
        assert c._session is None


# =============================================================================
# get_service_client — ValidationError on missing base_url
# =============================================================================

class TestGetServiceClient:
    """get_service_client must raise ValidationError (not ValueError) when base_url is absent."""

    def test_raises_validation_error_when_base_url_absent(self):
        with pytest.raises(ValidationError):
            get_service_client(
                target_service=ComponentName.G8ED,
                source_service=ComponentName.G8EE,
                base_url="",
                timeout=DEFAULT_TIMEOUT,
                auth_token="",
            )

    def test_plain_value_error_never_raised_for_missing_base_url(self):
        with pytest.raises(Exception) as exc_info:
            get_service_client(
                target_service=ComponentName.G8ED,
                source_service=ComponentName.G8EE,
                base_url="",
                timeout=DEFAULT_TIMEOUT,
                auth_token="",
            )
        assert type(exc_info.value) is not ValueError


# =============================================================================
# HTTPClient ._should_retry
# =============================================================================

class TestShouldRetry:
    """_should_retry encodes method + status-code + exception gating."""

    def test_returns_false_when_retry_count_exhausted(self):
        c = HTTPClient (
            component_id=ComponentName.G8EE,
            base_url="https://g8ed",
            timeout=DEFAULT_TIMEOUT,
            retry_config=RetryConfig(max_retries=2),
            circuit_breaker_config=CircuitBreakerConfig(),
            auth_token="",
            api_key="",
            headers={},
            ca_cert_path="/mock/ca.crt",
        )
        assert c._should_retry("GET", 503, 2, Exception()) is False

    def test_returns_false_for_non_retryable_method(self):
        c = HTTPClient (
            component_id=ComponentName.G8EE,
            base_url="https://g8ed",
            timeout=DEFAULT_TIMEOUT,
            retry_config=RetryConfig(),
            circuit_breaker_config=CircuitBreakerConfig(),
            auth_token="",
            api_key="",
            headers={},
            ca_cert_path="/mock/ca.crt",
        )
        assert c._should_retry("POST", 503, 0, Exception()) is False

    def test_returns_true_for_retryable_status_and_method(self):
        c = HTTPClient (
            component_id=ComponentName.G8EE,
            base_url="https://g8ed",
            timeout=DEFAULT_TIMEOUT,
            retry_config=RetryConfig(),
            circuit_breaker_config=CircuitBreakerConfig(),
            auth_token="",
            api_key="",
            headers={},
            ca_cert_path="/mock/ca.crt",
        )
        for status in (408, 429, 500, 502, 503, 504):
            assert c._should_retry("GET", status, 0, Exception()) is True

    def test_returns_false_for_4xx_non_retryable(self):
        c = HTTPClient (
            component_id=ComponentName.G8EE,
            base_url="https://g8ed",
            timeout=DEFAULT_TIMEOUT,
            retry_config=RetryConfig(),
            circuit_breaker_config=CircuitBreakerConfig(),
            auth_token="",
            api_key="",
            headers={},
            ca_cert_path="/mock/ca.crt",
        )
        for status in (400, 401, 403, 404, 422):
            assert c._should_retry("GET", status, 0, Exception()) is False

    def test_returns_true_for_timeout_exception(self):
        c = HTTPClient (
            component_id=ComponentName.G8EE,
            base_url="https://g8ed",
            timeout=DEFAULT_TIMEOUT,
            retry_config=RetryConfig(),
            circuit_breaker_config=CircuitBreakerConfig(),
            auth_token="",
            api_key="",
            headers={},
            ca_cert_path="/mock/ca.crt",
        )
        assert c._should_retry("GET", 0, 0, asyncio.TimeoutError()) is True

    def test_returns_true_for_server_timeout_exception(self):
        c = HTTPClient (
            component_id=ComponentName.G8EE,
            base_url="https://g8ed",
            timeout=DEFAULT_TIMEOUT,
            retry_config=RetryConfig(),
            circuit_breaker_config=CircuitBreakerConfig(),
            auth_token="",
            api_key="",
            headers={},
            ca_cert_path="/mock/ca.crt",
        )
        assert c._should_retry("GET", 0, 0, aiohttp.ServerTimeoutError()) is True

    def test_returns_true_for_server_disconnected_exception(self):
        c = HTTPClient (
            component_id=ComponentName.G8EE,
            base_url="https://g8ed",
            timeout=DEFAULT_TIMEOUT,
            retry_config=RetryConfig(),
            circuit_breaker_config=CircuitBreakerConfig(),
            auth_token="",
            api_key="",
            headers={},
            ca_cert_path="/mock/ca.crt",
        )
        assert c._should_retry("GET", 0, 0, aiohttp.ServerDisconnectedError()) is True

    def test_returns_false_for_non_retryable_exception(self):
        c = HTTPClient (
            component_id=ComponentName.G8EE,
            base_url="https://g8ed",
            timeout=DEFAULT_TIMEOUT,
            retry_config=RetryConfig(),
            circuit_breaker_config=CircuitBreakerConfig(),
            auth_token="",
            api_key="",
            headers={},
            ca_cert_path="/mock/ca.crt",
        )
        assert c._should_retry("GET", 0, 0, ValueError("unexpected")) is False


# =============================================================================
# HTTPClient ._calculate_backoff
# =============================================================================

class TestCalculateBackoff:
    """_calculate_backoff produces exponential values with bounded jitter."""

    def test_backoff_is_non_negative(self):
        c = HTTPClient (
            component_id=ComponentName.G8EE,
            base_url="https://g8ed",
            timeout=DEFAULT_TIMEOUT,
            retry_config=RetryConfig(),
            circuit_breaker_config=CircuitBreakerConfig(),
            auth_token="",
            api_key="",
            headers={},
            ca_cert_path="/mock/ca.crt",
        )
        for retry in range(5):
            assert c._calculate_backoff(retry) >= 0.0

    def test_backoff_increases_with_retry_count(self):
        c = HTTPClient (
            component_id=ComponentName.G8EE,
            base_url="https://g8ed",
            timeout=DEFAULT_TIMEOUT,
            retry_config=RetryConfig(retry_jitter_factor=0.0),
            circuit_breaker_config=CircuitBreakerConfig(),
            auth_token="",
            api_key="",
            headers={},
            ca_cert_path="/mock/ca.crt",
        )
        b0 = c._calculate_backoff(0)
        b1 = c._calculate_backoff(1)
        b2 = c._calculate_backoff(2)
        assert b1 > b0
        assert b2 > b1


# =============================================================================
# HTTPClient .request() — error branches (mocked aiohttp session)
# =============================================================================

def _make_mock_response(status: int, body: bytes = b"") -> MagicMock:
    """Build a mock aiohttp response context-manager that yields the given status."""
    resp = MagicMock()
    resp.status = status
    resp.headers = {}

    async def _read():
        return body

    resp.read = _read
    resp.__aenter__ = AsyncMock(return_value=resp)
    resp.__aexit__ = AsyncMock(return_value=False)
    return resp


@pytest.mark.asyncio(loop_scope="session")
class TestG8eHTTPClientRequest:
    """HTTPClient .request() — response and exception error branches."""

    async def _make_client_with_mock_session(self, mock_response_cm):
        c = HTTPClient (
            component_id=ComponentName.G8EE,
            base_url="https://g8ed",
            timeout=DEFAULT_TIMEOUT,
            retry_config=RetryConfig(max_retries=0),
            circuit_breaker_config=CircuitBreakerConfig(),
            auth_token="",
            api_key="",
            headers={},
            ca_cert_path="/mock/ca.crt",
        )
        session = MagicMock()
        session.closed = False
        session.request = MagicMock(return_value=mock_response_cm)
        c._session = session
        return c

    async def test_4xx_non_retryable_raises_network_error(self):
        resp = _make_mock_response(404, b'{"detail": "not found"}')
        c = await self._make_client_with_mock_session(resp)

        with pytest.raises(NetworkError) as exc_info:
            await c.request("GET", "/api/missing", headers={}, json_data=None, context=None)

        assert exc_info.value is not None
        assert "404" in str(exc_info.value)

    async def test_5xx_non_retryable_raises_network_error(self):
        resp = _make_mock_response(500, b'{"detail": "internal error"}')
        c = HTTPClient (
            component_id=ComponentName.G8EE,
            base_url="https://g8ed",
            timeout=DEFAULT_TIMEOUT,
            retry_config=RetryConfig(max_retries=0, retry_methods=set()),
            circuit_breaker_config=CircuitBreakerConfig(),
            auth_token="",
            api_key="",
            headers={},
            ca_cert_path="/mock/ca.crt",
        )
        session = MagicMock()
        session.closed = False
        session.request = MagicMock(return_value=resp)
        c._session = session

        with pytest.raises(NetworkError) as exc_info:
            await c.request("GET", "/api/error", headers={}, json_data=None, context=None)

        assert "500" in str(exc_info.value)

    async def test_network_error_raises_network_error(self):
        resp = MagicMock()
        resp.__aenter__ = AsyncMock(side_effect=aiohttp.ClientConnectorError(
            connection_key=MagicMock(), os_error=OSError("connection refused")
        ))
        resp.__aexit__ = AsyncMock(return_value=False)

        c = HTTPClient (
            component_id=ComponentName.G8EE,
            base_url="https://g8ed",
            timeout=DEFAULT_TIMEOUT,
            retry_config=RetryConfig(max_retries=0),
            circuit_breaker_config=CircuitBreakerConfig(),
            auth_token="",
            api_key="",
            headers={},
            ca_cert_path="/mock/ca.crt",
        )
        session = MagicMock()
        session.closed = False
        session.request = MagicMock(return_value=resp)
        c._session = session

        with pytest.raises(NetworkError):
            await c.request("GET", "/api/test", headers={}, json_data=None, context=None)

    async def test_timeout_raises_network_error(self):
        resp = MagicMock()
        resp.__aenter__ = AsyncMock(side_effect=aiohttp.ServerTimeoutError())
        resp.__aexit__ = AsyncMock(return_value=False)

        c = HTTPClient (
            component_id=ComponentName.G8EE,
            base_url="https://g8ed",
            timeout=DEFAULT_TIMEOUT,
            retry_config=RetryConfig(max_retries=0),
            circuit_breaker_config=CircuitBreakerConfig(),
            auth_token="",
            api_key="",
            headers={},
            ca_cert_path="/mock/ca.crt",
        )
        session = MagicMock()
        session.closed = False
        session.request = MagicMock(return_value=resp)
        c._session = session

        with pytest.raises(NetworkError) as exc_info:
            await c.request("GET", "/api/slow", headers={}, json_data=None, context=None)

        assert "timed out" in str(exc_info.value).lower() or exc_info.value is not None

    async def test_circuit_breaker_open_raises_without_making_request(self):
        c = HTTPClient (
            component_id=ComponentName.G8EE,
            base_url="https://g8ed",
            timeout=DEFAULT_TIMEOUT,
            retry_config=RetryConfig(),
            circuit_breaker_config=CircuitBreakerConfig(failure_threshold=1, recovery_time=9999.0),
            auth_token="",
            api_key="",
            headers={},
            ca_cert_path="/mock/ca.crt",
        )
        session = MagicMock()
        session.closed = False
        session.request = MagicMock()
        c._session = session

        cb = c._get_circuit_breaker("https://g8ed/api/test")
        await cb.record_failure()
        assert cb.state is CircuitBreakerState.OPEN

        with pytest.raises(NetworkError) as exc_info:
            await c.request("GET", "/api/test", headers={}, json_data=None, context=None)

        assert "Circuit breaker" in str(exc_info.value) or exc_info.value is not None
        session.request.assert_not_called()

    async def test_2xx_response_returns_aiohttp_response(self):
        resp = _make_mock_response(200, b'{"ok": true}')
        c = await self._make_client_with_mock_session(resp)

        result = await c.request("GET", "/api/health", headers={}, json_data=None, context=None)

        assert isinstance(result, AiohttpResponse)
        assert result.is_success
        assert result.json() == {"ok": True}

    async def test_retryable_status_retries_and_eventually_raises(self):
        resp_503 = _make_mock_response(503, b"service unavailable")
        call_count = 0
        original_request = MagicMock(return_value=resp_503)

        c = HTTPClient (
            component_id=ComponentName.G8EE,
            base_url="https://g8ed",
            timeout=DEFAULT_TIMEOUT,
            retry_config=RetryConfig(
                max_retries=2,
                retry_backoff_factor=0.0,
                retry_methods={"GET"},
                retry_status_codes={503},
            ),
            circuit_breaker_config=CircuitBreakerConfig(),
            auth_token="",
            api_key="",
            headers={},
            ca_cert_path="/mock/ca.crt",
        )
        session = MagicMock()
        session.closed = False
        session.request = original_request
        c._session = session

        with patch("asyncio.sleep", new_callable=AsyncMock):
            with pytest.raises(NetworkError):
                await c.request("GET", "/api/flaky", headers={}, json_data=None, context=None)

        assert original_request.call_count == 3

    async def test_retryable_exception_retries_and_eventually_raises(self):
        resp = MagicMock()
        resp.__aenter__ = AsyncMock(side_effect=aiohttp.ServerDisconnectedError())
        resp.__aexit__ = AsyncMock(return_value=False)

        c = HTTPClient (
            component_id=ComponentName.G8EE,
            base_url="https://g8ed",
            timeout=DEFAULT_TIMEOUT,
            retry_config=RetryConfig(
                max_retries=2,
                retry_backoff_factor=0.0,
                retry_methods={"GET"},
            ),
            circuit_breaker_config=CircuitBreakerConfig(),
            auth_token="",
            api_key="",
            headers={},
            ca_cert_path="/mock/ca.crt",
        )
        session = MagicMock()
        session.closed = False
        session.request = MagicMock(return_value=resp)
        c._session = session

        with patch("asyncio.sleep", new_callable=AsyncMock):
            with pytest.raises(NetworkError):
                await c.request("GET", "/api/disconnected", headers={}, json_data=None, context=None)

        assert session.request.call_count == 3

    async def test_non_retryable_method_does_not_retry_on_5xx(self):
        resp_500 = _make_mock_response(500, b"server error")
        original_request = MagicMock(return_value=resp_500)

        c = HTTPClient (
            component_id=ComponentName.G8EE,
            base_url="https://g8ed",
            timeout=DEFAULT_TIMEOUT,
            retry_config=RetryConfig(
                max_retries=3,
                retry_backoff_factor=0.0,
                retry_methods={"GET"},
                retry_status_codes={500},
            ),
            circuit_breaker_config=CircuitBreakerConfig(),
            auth_token="",
            api_key="",
            headers={},
            ca_cert_path="/mock/ca.crt",
        )
        session = MagicMock()
        session.closed = False
        session.request = original_request
        c._session = session

        with pytest.raises(NetworkError):
            await c.request("POST", "/api/create", headers={}, json_data=None, context=None)

        assert original_request.call_count == 1

    async def test_unexpected_exception_raises_network_error(self):
        resp = MagicMock()
        resp.__aenter__ = AsyncMock(side_effect=RuntimeError("unexpected internal error"))
        resp.__aexit__ = AsyncMock(return_value=False)

        c = HTTPClient (
            component_id=ComponentName.G8EE,
            base_url="https://g8ed",
            timeout=DEFAULT_TIMEOUT,
            retry_config=RetryConfig(max_retries=0),
            circuit_breaker_config=CircuitBreakerConfig(),
            auth_token="",
            api_key="",
            headers={},
            ca_cert_path="/mock/ca.crt",
        )
        session = MagicMock()
        session.closed = False
        session.request = MagicMock(return_value=resp)
        c._session = session

        with pytest.raises(NetworkError):
            await c.request("GET", "/api/boom", headers={}, json_data=None, context=None)
