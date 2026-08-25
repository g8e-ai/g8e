# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import logging

from app.clients.http_client import CircuitBreakerConfig, RetryConfig, HTTPClient
from app.models.settings import G8eeAppSettings, TLSConfig
from app.constants import (
    DEFAULT_HTTP_CLIENT_TIMEOUT,
    DEFAULT_MAX_RETRIES,
    G8EE_COMPONENT,
    GatewayAPIPaths,
    InternalAPIPaths,
    UNKNOWN_ERROR_MESSAGE,
)
from app.errors import NetworkError
from app.models.events import BackgroundEvent, BackgroundEventWire, SessionEvent, SessionEventWire
from app.models.http_context import G8eHttpContext
from app.models.internal_api import (
    GrantIntentResponse,
    IntentOperationResult,
    IntentRequestPayload,
    RevokeIntentResponse,
    SSEPushResponse,
    OperatorLinkResponse,
    OperatorLinkRequestPayload,
)

logger = logging.getLogger(__name__)


def get_client_url(settings: G8eeAppSettings) -> str:
    return settings.component_urls.client_url


class InternalHttpClient:
    def __init__(self, settings: G8eeAppSettings):
        self.client_url = get_client_url(settings)
        self._settings = settings
        self._cached_cert_path: str | None = None
        self._cached_key_path: str | None = None

        tls_config = TLSConfig(
            ca_cert_path=settings.ca_cert_path,
            client_cert_path=settings.client_cert_path,
            client_key_path=settings.client_key_path,
        )

        self._http: HTTPClient = HTTPClient(
            component_id=G8EE_COMPONENT,
            base_url=self.client_url,
            timeout=DEFAULT_HTTP_CLIENT_TIMEOUT,
            retry_config=RetryConfig(max_retries=DEFAULT_MAX_RETRIES),
            circuit_breaker_config=CircuitBreakerConfig(
                failure_threshold=5,
                recovery_time=60,
            ),
            auth_token="",
            api_key=settings.auth.internal_api_key or "",
            headers={},
            tls_config=tls_config,
        )

        self._cached_cert_path = settings.client_cert_path
        self._cached_key_path = settings.client_key_path
        logger.info("InternalHttpClient initialized with URL: %s", self.client_url)

    def configure(self, settings: G8eeAppSettings) -> None:
        self._settings = settings

    @property
    def settings(self) -> G8eeAppSettings:
        return self._settings

    @property
    def client(self) -> HTTPClient:
        """Access the underlying HTTP client."""
        return self._http

    async def close(self) -> None:
        await self._http.close()

    def _ensure_mtls(self) -> None:
        """Ensure mTLS credentials are up to date from settings.

        Caches cert/key paths to avoid redundant refresh calls on every request.
        Only refreshes when paths actually change.
        """
        current_cert_path = self._settings.client_cert_path
        current_key_path = self._settings.client_key_path

        if current_cert_path != self._cached_cert_path or current_key_path != self._cached_key_path:
            self._http.refresh_mtls_credentials(
                current_cert_path,
                current_key_path,
            )
            self._cached_cert_path = current_cert_path
            self._cached_key_path = current_key_path

    async def push_sse_event(
        self,
        event: SessionEvent | BackgroundEvent,
    ) -> SSEPushResponse:
        """POST an event to client for SSE delivery.

        Returns the typed SSEPushResponse so callers can distinguish "accepted,
        delivered to N sessions" from "accepted, fan-out had zero listeners"
        (both legitimate success cases). Raises NetworkError only for genuine
        transport/server failures (non-2xx); the originating HTTP status code
        is preserved in the error details so real outages are never collapsed
        into the empty-fan-out success shape.
        """
        wire_model = (
            SessionEventWire.from_session_event(event)
            if isinstance(event, SessionEvent)
            else BackgroundEventWire.from_background_event(event)
        )
        wire = wire_model.model_dump(mode="json")
        web_session_id: str | None = wire.get("web_session_id")
        cli_session_id: str | None = wire.get("cli_session_id")
        event_type: str = wire.get("event", {}).get("type") or "None"

        logger.info(
            "[HTTP-CLIENT] Pushing SSE event",
            extra={
                "web_session_id": (web_session_id[:8] + "...") if web_session_id else None,
                "cli_session_id": (cli_session_id[:8] + "...") if cli_session_id else None,
                "event_type": event_type,
            },
        )

        self._ensure_mtls()
        try:
            response = await self._http.post(
                GatewayAPIPaths.SSE_PUSH,
                json_data=wire_model,
            )
        except Exception as e:
            raise NetworkError(
                f"[HTTP-CLIENT] HTTP request failed: {e}",
                component=G8EE_COMPONENT,
                cause=e,
            ) from e

        if not response.is_success:
            logger.error(
                "[HTTP-CLIENT] Failed to deliver SSE event",
                extra={
                    "status": response.status_code,
                    "error": response.text,
                    "event_type": event_type,
                },
            )
            raise NetworkError(
                f"[HTTP-CLIENT] SSE push returned HTTP {response.status_code}",
                component=G8EE_COMPONENT,
                details={
                    "status_code": response.status_code,
                    "response": response.text,
                    "event_type": event_type,
                },
            )

        result = SSEPushResponse.model_validate(response.json())
        logger.info(
            "[HTTP-CLIENT] SSE event delivered",
            extra={
                "web_session_id": (web_session_id[:8] + "...") if web_session_id else None,
                "event_type": event_type,
                "success": result.success,
                "listeners": result.listeners,
            },
        )
        return result

    async def grant_intent(
        self,
        operator_id: str,
        intent: str,
        context: G8eHttpContext,
    ) -> IntentOperationResult:
        try:
            logger.info(
                "[HTTP-CLIENT] Granting intent to operator",
                extra={"operator_id": operator_id, "intent": intent},
            )

            self._ensure_mtls()
            from app.models.http_context import RequestContext

            request_payload = IntentRequestPayload(
                context=RequestContext.from_app_context(context),
                operator_id=operator_id,
                intent=intent,
            )

            response = await self._http.post(
                InternalAPIPaths.CLIENT_GRANT_INTENT.format(operator_id=operator_id),
                json_data=request_payload,
                context=None,  # Context now in request body
            )
            result = GrantIntentResponse.model_validate(response.json())
            if response.is_success and result.success:
                logger.info(
                    "[HTTP-CLIENT] Intent granted successfully",
                    extra={
                        "operator_id": operator_id,
                        "intent": intent,
                        "granted_intents": result.granted_intents,
                    },
                )
                return IntentOperationResult(
                    success=True,
                    granted_intents=result.granted_intents,
                )
            logger.warning(
                "[HTTP-CLIENT] Failed to grant intent",
                extra={
                    "operator_id": operator_id,
                    "intent": intent,
                    "status": response.status_code,
                    "error": result.error,
                },
            )
            return IntentOperationResult(
                success=False,
                error=result.error or UNKNOWN_ERROR_MESSAGE,
            )

        except Exception as e:
            raise NetworkError(
                f"[HTTP-CLIENT] Failed to grant intent: {e}",
                component=G8EE_COMPONENT,
                cause=e,
            ) from e

    async def revoke_intent(
        self,
        operator_id: str,
        intent: str,
        context: G8eHttpContext,
    ) -> IntentOperationResult:
        try:
            self._ensure_mtls()
            from app.models.http_context import RequestContext

            request_payload = IntentRequestPayload(
                context=RequestContext.from_app_context(context),
                operator_id=operator_id,
                intent=intent,
            )

            response = await self._http.post(
                InternalAPIPaths.CLIENT_REVOKE_INTENT.format(operator_id=operator_id),
                json_data=request_payload,
                context=None,  # Context now in request body
            )
            result = RevokeIntentResponse.model_validate(response.json())
            if response.is_success and result.success:
                return IntentOperationResult(
                    success=True,
                    granted_intents=result.granted_intents,
                )
            return IntentOperationResult(
                success=False,
                error=result.error or UNKNOWN_ERROR_MESSAGE,
            )

        except Exception as e:
            raise NetworkError(
                f"[HTTP-CLIENT] Failed to revoke intent: {e}",
                component=G8EE_COMPONENT,
                cause=e,
            ) from e

    async def generate_operator_link(
        self,
        user_id: str,
        operator_id: str,
        web_session_id: str,
        organization_id: str | None = None,
        context: G8eHttpContext | None = None,
    ) -> OperatorLinkResponse:
        """Generate a single-operator handshake link (dlk_ token) via client.

        This is a prerequisite for the 'stream_operator' tool (Phase 4).
        """
        try:
            logger.info(
                "[HTTP-CLIENT] Generating operator device link",
                extra={"user_id": user_id, "operator_id": operator_id},
            )

            self._ensure_mtls()
            from app.models.http_context import RequestContext

            if context:
                request_context = RequestContext.from_app_context(context)
            else:
                request_context = RequestContext(
                    web_session_id=web_session_id,
                    user_id=user_id,
                    organization_id=organization_id,
                    source_component=G8EE_COMPONENT,
                )

            request_payload = OperatorLinkRequestPayload(
                context=request_context,
                operator_id=operator_id,
                user_id=user_id,
            )

            response = await self._http.post(
                InternalAPIPaths.CLIENT_CREATE_OPERATOR_LINK,
                json_data=request_payload,
                context=None,  # Context now in request body
            )

            result = OperatorLinkResponse.model_validate(response.json())
            if response.is_success and result.success:
                logger.info(
                    "[HTTP-CLIENT] Operator device link generated successfully",
                    extra={"user_id": user_id, "operator_id": operator_id},
                )
                return result

            logger.warning(
                "[HTTP-CLIENT] Failed to generate operator device link",
                extra={
                    "user_id": user_id,
                    "operator_id": operator_id,
                    "status": response.status_code,
                    "error": result.error,
                },
            )
            return result

        except Exception as e:
            raise NetworkError(
                f"[HTTP-CLIENT] Failed to generate operator device link: {e}",
                component=G8EE_COMPONENT,
                cause=e,
            ) from e
