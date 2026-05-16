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

import logging

from app.clients.http_client import CircuitBreakerConfig, RetryConfig, HTTPClient
from app.models.settings import G8eePlatformSettings
from app.constants import (
    UNKNOWN_ERROR_MESSAGE,
    DEFAULT_HTTP_CLIENT_TIMEOUT,
    DEFAULT_MAX_RETRIES,
    ComponentName,
    G8eHeaders,
    InternalApiPaths,
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


def get_client_url(settings: G8eePlatformSettings) -> str:
    return settings.component_urls.client_url


class InternalHttpClient:

    def __init__(self, settings: G8eePlatformSettings):
        self.client_url = get_client_url(settings)
        self._settings = settings
        self._http: HTTPClient = HTTPClient(
            component_id=ComponentName.G8EE,
            base_url=self.client_url,
            timeout=DEFAULT_HTTP_CLIENT_TIMEOUT,
            retry_config=RetryConfig(max_retries=DEFAULT_MAX_RETRIES),
            circuit_breaker_config=CircuitBreakerConfig(
                failure_threshold=5,
                recovery_time=60,
            ),
            auth_token="",
            api_key=settings.auth.g8e_api_key or "",
            headers={G8eHeaders.SOURCE_COMPONENT: ComponentName.G8EE},
            ca_cert_path=settings.ca_cert_path or "",
        )

        logger.info("InternalHttpClient initialized with URL: %s", self.client_url)

    def configure(self, settings: G8eePlatformSettings) -> None:
        self._settings = settings

    @property
    def settings(self) -> G8eePlatformSettings:
        return self._settings

    @property
    def client(self) -> HTTPClient:
        """Access the underlying HTTP client."""
        return self._http

    def _auth_headers(self) -> dict[str, str]:
        return {
            G8eHeaders.SOURCE_COMPONENT: ComponentName.G8EE,
        }

    async def close(self) -> None:
        await self._http.close()

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
        event_type: str = wire.get("event", {}).get("type") or "None"

        logger.info(
            "[HTTP-CLIENT] Pushing SSE event",
            extra={
                "web_session_id": (web_session_id[:8] + "...") if web_session_id else None,
                "event_type": event_type,
            }
        )

        try:
            response = await self._http.post(
                InternalApiPaths.CLIENT_SSE_PUSH,
                json_data=wire_model,
                headers=self._auth_headers(),
            )
        except Exception as e:
            raise NetworkError(
                f"[HTTP-CLIENT] HTTP request failed: {e}",
                component=ComponentName.G8EE,
                cause=e,
            ) from e

        if not response.is_success:
            logger.error(
                "[HTTP-CLIENT] Failed to deliver SSE event",
                extra={
                    "status": response.status_code,
                    "error": response.text,
                    "event_type": event_type,
                }
            )
            raise NetworkError(
                f"[HTTP-CLIENT] SSE push returned HTTP {response.status_code}",
                component=ComponentName.G8EE,
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
                "delivered": result.delivered,
            }
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
                extra={"operator_id": operator_id, "intent": intent}
            )

            request_payload = IntentRequestPayload(intent=intent)

            response = await self._http.post(
                InternalApiPaths.CLIENT_GRANT_INTENT.format(operator_id=operator_id),
                json_data=request_payload,
                headers=self._auth_headers(),
                context=context,
            )
            result = GrantIntentResponse.model_validate(response.json())
            if response.is_success and result.success:
                logger.info(
                    "[HTTP-CLIENT] Intent granted successfully",
                    extra={
                        "operator_id": operator_id,
                        "intent": intent,
                        "granted_intents": result.granted_intents,
                    }
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
                }
            )
            return IntentOperationResult(
                success=False,
                error=result.error or UNKNOWN_ERROR_MESSAGE,
            )

        except Exception as e:
            raise NetworkError(
                f"[HTTP-CLIENT] Failed to grant intent: {e}",
                component=ComponentName.G8EE,
                cause=e,
            ) from e

    async def revoke_intent(
        self,
        operator_id: str,
        intent: str,
        context: G8eHttpContext,
    ) -> IntentOperationResult:
        try:
            request_payload = IntentRequestPayload(intent=intent)

            response = await self._http.post(
                InternalApiPaths.CLIENT_REVOKE_INTENT.format(operator_id=operator_id),
                json_data=request_payload,
                headers=self._auth_headers(),
                context=context,
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
                component=ComponentName.G8EE,
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
                extra={"user_id": user_id, "operator_id": operator_id}
            )

            request_payload = OperatorLinkRequestPayload(
                user_id=user_id,
                organization_id=organization_id,
                operator_id=operator_id,
                web_session_id=web_session_id,
            )

            response = await self._http.post(
                InternalApiPaths.CLIENT_CREATE_OPERATOR_LINK,
                json_data=request_payload,
                headers=self._auth_headers(),
                context=context,
            )

            result = OperatorLinkResponse.model_validate(response.json())
            if response.is_success and result.success:
                logger.info(
                    "[HTTP-CLIENT] Operator device link generated successfully",
                    extra={"user_id": user_id, "operator_id": operator_id}
                )
                return result

            logger.warning(
                "[HTTP-CLIENT] Failed to generate operator device link",
                extra={
                    "user_id": user_id,
                    "operator_id": operator_id,
                    "status": response.status_code,
                    "error": result.error,
                }
            )
            return result

        except Exception as e:
            raise NetworkError(
                f"[HTTP-CLIENT] Failed to generate operator device link: {e}",
                component=ComponentName.G8EE,
                cause=e,
            ) from e


