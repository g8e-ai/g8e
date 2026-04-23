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
g8ee HTTP Client

A robust HTTP client for inter-component communication with built-in retry,
circuit breaking, and error handling capabilities.
"""

import asyncio
import json
import logging
import random
import time
import uuid
from collections.abc import AsyncIterator, Mapping
from datetime import datetime
from typing import Any, Optional, Union
from urllib.parse import urljoin, urlparse

import aiohttp

from app.constants import (
    DEFAULT_MAX_RETRIES,
    DEFAULT_RETRY_BACKOFF_FACTOR,
    DEFAULT_RETRY_JITTER,
    HTTP_API_KEY_HEADER,
    HTTP_AUTHORIZATION_HEADER,
    HTTP_BEARER_PREFIX,
    CircuitBreakerState,
    ErrorCode,
    ErrorSeverity,
    G8eHeaders,
)
from app.errors import (
    NetworkError,
    ValidationError,
)
from app.models.base import G8eBaseModel
from app.models.http_context import G8eHttpContext
from app.utils.aiohttp_session import new_component_http_session
from app.utils.timestamp import now

logger = logging.getLogger(__name__)

JSONPayload = Union[Mapping[str, Any], G8eBaseModel]
"""Typed JSON request body: a Pydantic model or a dict-shaped mapping.

HTTPClient callers must pass a typed model or a mapping already serialized to
JSON-safe primitives (e.g. via ``model.model_dump(mode=\"json\")``). Raw
``Any`` is rejected per the developer guide (no ``Any`` in signatures)."""

QueryParams = Mapping[str, Union[str, int, float, bool]]

DEFAULT_RETRY_METHODS: set[str] = {"GET", "PUT", "DELETE", "HEAD", "OPTIONS"}
DEFAULT_RETRY_STATUS_CODES: set[int] = {408, 429, 500, 502, 503, 504}

EXECUTION_ID_HEADER = G8eHeaders.EXECUTION_ID

class RequestTrace(G8eBaseModel):
    """
    Tracking information for a request to provide context in logs
    and enable distributed tracing.
    """
    execution_id: str
    component_id: str
    case_id: Optional[str] = None
    task_id: Optional[str] = None
    investigation_id: Optional[str] = None
    start_time: datetime
    end_time: Optional[datetime] = None

    duration_ms: float = 0.0

    @property
    def as_headers(self) -> dict[str, str]:
        """Convert trace info to HTTP headers"""
        headers = {
            EXECUTION_ID_HEADER: self.execution_id,
        }
        if self.case_id:
            headers[G8eHeaders.CASE_ID] = self.case_id
        if self.task_id:
            headers[G8eHeaders.TASK_ID] = self.task_id
        if self.investigation_id:
            headers[G8eHeaders.INVESTIGATION_ID] = self.investigation_id
        return headers

    @classmethod
    def from_headers(cls, headers: dict[str, str], component_id: str) -> "RequestTrace":
        """Create trace info from incoming HTTP headers"""
        execution_id = headers.get(EXECUTION_ID_HEADER, f"exec-{uuid.uuid4()}")
        return cls(
            execution_id=execution_id,
            component_id=component_id,
            case_id=headers.get(G8eHeaders.CASE_ID),
            task_id=headers.get(G8eHeaders.TASK_ID),
            investigation_id=headers.get(G8eHeaders.INVESTIGATION_ID),
            start_time=now(),
        )

    def finish(self) -> None:
        """Mark the request as completed and calculate duration"""
        self.end_time = now()
        if self.start_time is not None:
            self.duration_ms = (self.end_time - self.start_time).total_seconds() * 1000


class RetryConfig(G8eBaseModel):
    """Configuration for HTTP client retry behavior"""
    max_retries: int = DEFAULT_MAX_RETRIES
    retry_backoff_factor: float = DEFAULT_RETRY_BACKOFF_FACTOR
    retry_jitter_factor: float = DEFAULT_RETRY_JITTER
    retry_methods: set[str] = DEFAULT_RETRY_METHODS
    retry_status_codes: set[int] = DEFAULT_RETRY_STATUS_CODES


class CircuitBreakerConfig(G8eBaseModel):
    """Configuration for circuit breaker behavior"""
    failure_threshold: int = 5
    recovery_time: float = 30.0
    half_open_success_threshold: int = 3


class CircuitBreaker:
    """Circuit breaker to fail fast when a service is down."""

    def __init__(self, config: CircuitBreakerConfig, endpoint: str):
        self.config = config
        self.endpoint = endpoint
        self.state: CircuitBreakerState = CircuitBreakerState.CLOSED
        self.failures = 0
        self.successes = 0
        self.last_failure_time = 0
        self.lock = asyncio.Lock()

    async def record_success(self) -> None:
        """Record a successful operation"""
        async with self.lock:
            if self.state == CircuitBreakerState.HALF_OPEN:
                self.successes += 1
                if self.successes >= self.config.half_open_success_threshold:
                    self.state = CircuitBreakerState.CLOSED
                    self.failures = 0
                    self.successes = 0
                    logger.info(f"Circuit breaker for {self.endpoint} is now CLOSED after successful recovery")

    async def record_failure(self) -> None:
        """Record a failed operation"""
        async with self.lock:
            self.last_failure_time = time.time()
            if self.state == CircuitBreakerState.CLOSED:
                self.failures += 1
                if self.failures >= self.config.failure_threshold:
                    self.state = CircuitBreakerState.OPEN
                    logger.error(f"Circuit breaker for {self.endpoint} is now OPEN after {self.failures} failures")
            elif self.state == CircuitBreakerState.HALF_OPEN:
                self.state = CircuitBreakerState.OPEN
                self.successes = 0
                logger.error(f"Circuit breaker for {self.endpoint} returned to OPEN state after failure in HALF_OPEN")

    async def allow_request(self) -> bool:
        """
        Check if a request should be allowed based on the circuit state.

        Returns:
            True if the request should proceed, False if it should be blocked
        """
        async with self.lock:
            if self.state == CircuitBreakerState.CLOSED:
                return True
            if self.state == CircuitBreakerState.OPEN:
                if time.time() - self.last_failure_time > self.config.recovery_time:
                    self.state = CircuitBreakerState.HALF_OPEN
                    self.successes = 0
                    logger.info(f"Circuit breaker for {self.endpoint} is now HALF_OPEN, testing service recovery")
                    return True
                return False
            return True


class AiohttpResponse:
    """Lightweight response wrapper over aiohttp (used by LLM providers).
    
    aiohttp responses must be consumed inside their context manager. This wrapper
    reads the body eagerly and exposes .status_code, .json(), .text, and .is_success.
    """

    def __init__(self, status: int, body: bytes, headers: dict[str, str]):
        self.status_code = status
        self._body = body
        self.headers = headers

    @property
    def is_success(self) -> bool:
        return 200 <= self.status_code < 300

    @property
    def text(self) -> str:
        return self._body.decode("utf-8", errors="replace")

    def json(self) -> object:
        """Parse response body as JSON.

        Returns ``object`` (not ``Any``) to force callers to validate the
        decoded payload through a typed model at the boundary.
        """
        return json.loads(self._body)


class HTTPClient:
    """
    A robust HTTP client for inter-component communication within the g8e ecosystem.
    
    Features:
    - Automatic retry for transient failures with exponential backoff
    - Circuit breaker pattern to fail fast when services are unavailable
    - Distributed tracing via request ID and correlation ID
    - Structured error handling that integrates with the g8e error system
    - Authentication between components
    """

    def __init__(
        self,
        component_id: str,
        base_url: str,
        timeout: float,
        retry_config: RetryConfig,
        circuit_breaker_config: CircuitBreakerConfig,
        auth_token: str,
        api_key: str,
        headers: dict[str, str],
        ca_cert_path: str,
    ):
        self.component_id = component_id
        self.base_url = base_url
        self.timeout = aiohttp.ClientTimeout(total=timeout)
        self.retry_config = retry_config
        self.circuit_breaker_config = circuit_breaker_config
        self.auth_token = auth_token
        self.api_key = api_key
        self.default_headers = headers
        self._ca_cert_path = ca_cert_path

        self._session: aiohttp.ClientSession | None = None
        self.circuit_breakers: dict[str, CircuitBreaker] = {}

    @property
    def is_session_closed(self) -> bool:
        """Check if the internal session is closed."""
        return self._session is None or self._session.closed

    async def _get_http_session(self) -> aiohttp.ClientSession:
        """Get or create a persistent aiohttp session."""
        self._session = new_component_http_session(
            self._session,
            timeout=self.timeout,
            ca_cert_path=self._ca_cert_path,
            headers=self.default_headers,
        )
        return self._session

    def _get_circuit_breaker(self, url: str) -> CircuitBreaker:
        """Get or create a circuit breaker for the given URL"""
        parsed = urlparse(url)
        endpoint = f"{parsed.netloc}{parsed.path}"

        if endpoint not in self.circuit_breakers:
            self.circuit_breakers[endpoint] = CircuitBreaker(
                self.circuit_breaker_config, endpoint
            )

        return self.circuit_breakers[endpoint]

    @staticmethod
    def _context_to_headers(context: "G8eHttpContext") -> dict[str, str]:
        """Convert a G8eHttpContext into the standard X-G8E-* outbound headers."""
        headers: dict[str, str] = {
            G8eHeaders.WEB_SESSION_ID: context.web_session_id,
            G8eHeaders.USER_ID: context.user_id,
            G8eHeaders.SOURCE_COMPONENT: context.source_component,
            G8eHeaders.EXECUTION_ID: context.execution_id,
        }
        if context.organization_id:
            headers[G8eHeaders.ORGANIZATION_ID] = context.organization_id
        if context.case_id:
            headers[G8eHeaders.CASE_ID] = context.case_id
        if context.investigation_id:
            headers[G8eHeaders.INVESTIGATION_ID] = context.investigation_id
        if context.task_id:
            headers[G8eHeaders.TASK_ID] = context.task_id
        return headers

    async def _prepare_request(
        self,
        method: str,
        url: str,
        headers: dict[str, str] | None,
        context: Optional["G8eHttpContext"],
    ) -> tuple[str, dict[str, str], RequestTrace]:
        """
        Prepare the request URL and headers.

        Args:
            method: HTTP method
            url: Request URL or path (will be joined with base_url if provided)
            headers: Optional headers to add to the request
            context: Optional G8eHttpContext to propagate as X-G8E-* headers

        Returns:
            Tuple of (final_url, headers, trace)
        """
        final_url = urljoin(self.base_url, url) if self.base_url else url

        request_headers = self.default_headers.copy()
        if self.auth_token:
            request_headers[HTTP_AUTHORIZATION_HEADER] = f"{HTTP_BEARER_PREFIX} {self.auth_token}"
        if self.api_key:
            request_headers[HTTP_API_KEY_HEADER] = self.api_key

        if context is not None:
            request_headers.update(self._context_to_headers(context))

        if headers:
            request_headers.update(headers)

        trace = RequestTrace.from_headers(request_headers, self.component_id)
        request_headers.update(trace.as_headers)

        return final_url, request_headers, trace

    @staticmethod
    def _serialize_json_payload(json_data: JSONPayload | None) -> Mapping[str, Any] | None:
        """Coerce a typed payload to a JSON-safe mapping for aiohttp.

        G8eBaseModel instances serialize through ``model_dump(mode=\"json\")`` to
        honour the application-boundary rule: models flatten to plain dicts
        only when crossing a wire boundary.
        """
        if json_data is None:
            return None
        if isinstance(json_data, G8eBaseModel):
            return json_data.model_dump(mode="json")
        return json_data

    def _should_retry(
        self,
        method: str,
        status_code: int,
        retry_count: int,
        exception: Optional[Exception] = None
    ) -> bool:
        if retry_count >= self.retry_config.max_retries:
            return False
        if method.upper() not in self.retry_config.retry_methods:
            return False
        if status_code in self.retry_config.retry_status_codes:
            return True
        if exception and isinstance(exception, (
            aiohttp.ClientConnectorError,
            aiohttp.ClientOSError,
            aiohttp.ServerDisconnectedError,
            aiohttp.ServerTimeoutError,
            asyncio.TimeoutError,
        )):
            return True
        return False

    def _calculate_backoff(self, retry_count: int) -> float:
        backoff = self.retry_config.retry_backoff_factor * (2 ** retry_count)
        jitter = random.uniform(
            -self.retry_config.retry_jitter_factor * backoff,
            self.retry_config.retry_jitter_factor * backoff
        )
        return max(0, backoff + jitter)

    async def request(
        self,
        method: str,
        url: str,
        headers: dict[str, str] | None = None,
        json_data: JSONPayload | None = None,
        context: Optional["G8eHttpContext"] = None,
        params: QueryParams | None = None,
    ) -> "AiohttpResponse":
        """
        Make an HTTP request with automatic retry and circuit breaking.

        Args:
            method: HTTP method (GET, POST, etc.)
            url: URL or path to request
            headers: Optional headers to include
            json_data: Optional typed JSON body (Pydantic model or JSON-safe mapping)
            context: Optional G8eHttpContext to propagate as X-G8E-* headers
            params: Optional query-string parameters

        Returns:
            AiohttpResponse wrapper with status_code, json(), text properties

        Raises:
            NetworkError: On connection or response errors
        """
        final_url, request_headers, trace = await self._prepare_request(
            method, url, headers, context=context
        )

        request_kwargs: dict[str, Any] = {}
        json_body = self._serialize_json_payload(json_data)
        if json_body is not None:
            request_kwargs["json"] = json_body
        if params is not None:
            request_kwargs["params"] = dict(params)

        circuit_breaker = self._get_circuit_breaker(final_url)

        if not await circuit_breaker.allow_request():
            error = NetworkError(
                message=f"Circuit breaker is open for {circuit_breaker.endpoint}, failing fast",
                code=ErrorCode.API_CONNECTION_ERROR,
                severity=ErrorSeverity.HIGH,
                details={
                    "url": final_url,
                    "method": method,
                    "circuit_state": circuit_breaker.state,
                    "failures": circuit_breaker.failures,
                    "last_failure_time": circuit_breaker.last_failure_time,
                },
                retry_suggested=False
            )
            logger.error(f"Circuit breaker prevented request: {error}")
            raise error

        retry_count = 0
        session = await self._get_http_session()

        while True:
            try:
                async with session.request(
                    method,
                    final_url,
                    headers=request_headers,
                    **request_kwargs
                ) as response:
                    body = await response.read()
                    status = response.status
                    resp_headers = dict(response.headers)

                wrapped = AiohttpResponse(status, body, resp_headers)

                if wrapped.is_success:
                    await circuit_breaker.record_success()
                    trace.finish()
                    logger.info(
                        f"HTTP request successful: {method} {final_url}",
                        extra={
                            "request_method": method,
                            "request_url": final_url,
                            "response_status": wrapped.status_code,
                            "duration_ms": trace.duration_ms,
                            "execution_id": trace.execution_id,
                            "case_id": trace.case_id,
                            "task_id": trace.task_id,
                            "investigation_id": trace.investigation_id,
                        }
                    )

                    return wrapped

                if self._should_retry(method, wrapped.status_code, retry_count, exception=None):
                    retry_count += 1
                    backoff = self._calculate_backoff(retry_count)
                    logger.error(
                        f"Retrying request due to status {wrapped.status_code}: {method} {final_url} (attempt {retry_count}/{self.retry_config.max_retries}, backoff {backoff:.2f}s)",
                        extra={
                            "request_method": method,
                            "request_url": final_url,
                            "response_status": wrapped.status_code,
                            "retry_count": retry_count,
                            "backoff": backoff,
                            "execution_id": trace.execution_id,
                        }
                    )

                    await asyncio.sleep(backoff)
                    continue

                try:
                    error_detail = wrapped.json()
                except (json.JSONDecodeError, ValueError):
                    error_detail = {"text": wrapped.text[:1000] if wrapped.text else "(empty response)"}

                await circuit_breaker.record_failure()
                trace.finish()

                error = NetworkError(
                    message=f"HTTP request failed with status {wrapped.status_code}",
                    code=ErrorCode.API_RESPONSE_ERROR,
                    details={
                        "url": final_url,
                        "method": method,
                        "status_code": wrapped.status_code,
                        "response": error_detail,
                        "duration_ms": trace.duration_ms,
                        "execution_id": trace.execution_id,
                    },
                    retry_suggested=False
                )

                logger.error(
                    f"HTTP request failed: {method} {final_url}",
                    extra={
                        "request_method": method,
                        "request_url": final_url,
                        "response_status": wrapped.status_code,
                        "duration_ms": trace.duration_ms,
                        "error": str(error),
                        "execution_id": trace.execution_id,
                    }
                )

                raise error

            except (TimeoutError, aiohttp.ServerTimeoutError) as e:
                if self._should_retry(method, 0, retry_count, e):
                    retry_count += 1
                    backoff = self._calculate_backoff(retry_count)
                    logger.error(
                        f"Retrying request due to timeout: {method} {final_url} (attempt {retry_count}/{self.retry_config.max_retries}, backoff {backoff:.2f}s)",
                        extra={
                            "request_method": method,
                            "request_url": final_url,
                            "retry_count": retry_count,
                            "backoff": backoff,
                            "error": str(e),
                            "execution_id": trace.execution_id,
                        }
                    )

                    await asyncio.sleep(backoff)
                    continue

                await circuit_breaker.record_failure()
                trace.finish()

                error = NetworkError(
                    message=f"HTTP request timed out after {trace.duration_ms:.2f}ms",
                    code=ErrorCode.API_TIMEOUT_ERROR,
                    details={
                        "url": final_url,
                        "method": method,
                        "timeout": self.timeout.total,
                        "duration_ms": trace.duration_ms,
                        "execution_id": trace.execution_id,
                    },
                    retry_suggested=True,
                    cause=e
                )

                logger.error(
                    f"HTTP request timed out: {method} {final_url}",
                    extra={
                        "request_method": method,
                        "request_url": final_url,
                        "duration_ms": trace.duration_ms,
                        "error": str(error),
                        "execution_id": trace.execution_id,
                    }
                )

                raise error

            except (aiohttp.ClientError, OSError) as e:
                if self._should_retry(method, 0, retry_count, e):
                    retry_count += 1
                    backoff = self._calculate_backoff(retry_count)
                    logger.error(
                        f"Retrying request due to connection error: {method} {final_url} (attempt {retry_count}/{self.retry_config.max_retries}, backoff {backoff:.2f}s)",
                        extra={
                            "request_method": method,
                            "request_url": final_url,
                            "retry_count": retry_count,
                            "backoff": backoff,
                            "error": str(e),
                            "execution_id": trace.execution_id,
                        }
                    )

                    await asyncio.sleep(backoff)
                    continue

                await circuit_breaker.record_failure()
                trace.finish()

                error = NetworkError(
                    message=f"HTTP request failed: {e!s}",
                    code=ErrorCode.API_CONNECTION_ERROR,
                    details={
                        "url": final_url,
                        "method": method,
                        "duration_ms": trace.duration_ms,
                        "execution_id": trace.execution_id,
                    },
                    retry_suggested=True,
                    cause=e
                )

                logger.error(
                    f"HTTP connection error: {method} {final_url}",
                    extra={
                        "request_method": method,
                        "request_url": final_url,
                        "duration_ms": trace.duration_ms,
                        "error": str(error),
                        "execution_id": trace.execution_id,
                    }
                )

                raise error

            except NetworkError:
                raise

            except Exception as e:
                await circuit_breaker.record_failure()
                trace.finish()

                error = NetworkError(
                    message=f"Unexpected error during HTTP request: {e!s}",
                    code=ErrorCode.API_REQUEST_ERROR,
                    details={
                        "url": final_url,
                        "method": method,
                        "duration_ms": trace.duration_ms,
                        "execution_id": trace.execution_id,
                    },
                    cause=e
                )

                logger.error(
                    f"Unexpected HTTP request error: {method} {final_url}",
                    extra={
                        "request_method": method,
                        "request_url": final_url,
                        "duration_ms": trace.duration_ms,
                        "error": str(error),
                        "execution_id": trace.execution_id,
                    },
                    exc_info=True
                )

                raise error

    async def post(
        self,
        url: str,
        json_data: JSONPayload | None = None,
        headers: dict[str, str] | None = None,
        context: Optional["G8eHttpContext"] = None,
    ) -> "AiohttpResponse":
        return await self.request(
            "POST", url, headers=headers, json_data=json_data, context=context
        )

    async def get(
        self,
        url: str,
        params: QueryParams | None = None,
        headers: dict[str, str] | None = None,
        context: Optional["G8eHttpContext"] = None,
    ) -> "AiohttpResponse":
        return await self.request(
            "GET", url, headers=headers, context=context, params=params
        )

    async def stream(
        self,
        method: str,
        url: str,
        headers: dict[str, str] | None = None,
        json_data: JSONPayload | None = None,
        context: Optional["G8eHttpContext"] = None,
        chunk_size: int = 8192,
    ) -> AsyncIterator[bytes]:
        """Stream response content chunk-by-chunk without buffering the full body.

        Raises:
            NetworkError: On connection or response errors
        """
        final_url, request_headers, trace = await self._prepare_request(
            method, url, headers, context=context
        )
        request_kwargs: dict[str, Any] = {}
        json_body = self._serialize_json_payload(json_data)
        if json_body is not None:
            request_kwargs["json"] = json_body

        circuit_breaker = self._get_circuit_breaker(final_url)
        if not await circuit_breaker.allow_request():
            raise NetworkError(
                message=f"Circuit breaker is open for {circuit_breaker.endpoint}, failing fast",
                code=ErrorCode.API_CONNECTION_ERROR,
                details={"url": final_url, "method": method},
                retry_suggested=False,
            )

        session = await self._get_http_session()
        try:
            async with session.request(
                method,
                final_url,
                headers=request_headers,
                **request_kwargs
            ) as response:
                if response.status >= 400:
                    body = await response.read()
                    await circuit_breaker.record_failure()
                    raise NetworkError(
                        message=f"Streaming request failed with status {response.status}",
                        code=ErrorCode.API_RESPONSE_ERROR,
                        details={
                            "url": final_url,
                            "method": method,
                            "status_code": response.status,
                            "response": body.decode("utf-8", errors="replace")[:500],
                        },
                    )
                await circuit_breaker.record_success()
                async for chunk in response.content.iter_chunked(chunk_size):
                    yield chunk
        except NetworkError:
            raise
        except (TimeoutError, aiohttp.ServerTimeoutError) as e:
            await circuit_breaker.record_failure()
            raise NetworkError(
                message="Streaming request timed out",
                code=ErrorCode.API_TIMEOUT_ERROR,
                details={"url": final_url, "method": method},
                retry_suggested=True,
                cause=e,
            )
        except (aiohttp.ClientError, OSError) as e:
            await circuit_breaker.record_failure()
            raise NetworkError(
                message=f"Streaming request failed: {e}",
                code=ErrorCode.API_CONNECTION_ERROR,
                details={"url": final_url, "method": method},
                retry_suggested=True,
                cause=e,
            )

    async def close(self) -> None:
        if self._session is not None and not self._session.closed:
            await self._session.close()
            self._session = None

    async def __aenter__(self) -> "HTTPClient ":
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb) -> None:
        await self.close()


def get_service_client(
    target_service: str,
    source_service: str,
    base_url: str,
    timeout: float,
    auth_token: str,
    api_key: str = "",
    headers: dict[str, str] | None = None,
    ca_cert_path: str = "",
) -> HTTPClient:
    """Get an HTTP client configured for inter-service communication.

    Args:
        target_service: The service being called (e.g., ComponentName.G8ED)
        source_service: The component making the request (e.g., ComponentName.G8EE)
        base_url: Optional explicit service URL
        timeout: Request timeout in seconds
        auth_token: Optional pre-loaded auth token (avoids inline Settings creation)
        api_key: Optional API key for authentication
        headers: Optional default headers
        ca_cert_path: Optional path to CA certificate

    Returns:
        Configured HTTP client for the service
    """
    if not base_url:
        raise ValidationError(
            f"No base_url provided for service '{target_service}'. "
            "Service URLs must be explicitly provided or loaded from database configuration.",
            component="g8ee",
        )

    client = HTTPClient(
        component_id=source_service,
        base_url=base_url,
        timeout=timeout,
        retry_config=RetryConfig(max_retries=3),
        circuit_breaker_config=CircuitBreakerConfig(),
        auth_token=auth_token,
        api_key=api_key,
        headers=headers or {},
        ca_cert_path=ca_cert_path,
    )

    logger.info(f"Created HTTP client for service {target_service} with base URL {base_url}")

    return client
