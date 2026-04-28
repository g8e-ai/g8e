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
    INTERNAL_AUTH_HEADER,
    UNKNOWN_ERROR_MESSAGE,
    G8ED_CLIENT_FAILURE_THRESHOLD,
    G8ED_CLIENT_MAX_RETRIES,
    G8ED_CLIENT_RECOVERY_TIME,
    G8ED_CLIENT_TIMEOUT,
    ComponentName,
    G8eHeaders,
    InternalApiPaths,
)
from app.errors import ConfigurationError, NetworkError
from app.models.events import BackgroundEvent, BackgroundEventWire, SessionEvent, SessionEventWire
from app.models.http_context import G8eHttpContext
from app.models.g8ed_client import (
    GrantIntentResponse,
    IntentOperationResult,
    IntentRequestPayload,
    RevokeIntentResponse,
    SSEPushResponse,
    OperatorLinkResponse,
    OperatorLinkRequestPayload,
)

logger = logging.getLogger(__name__)


def get_g8ed_url(settings: G8eePlatformSettings) -> str:
    return settings.component_urls.g8ed_url


class InternalHttpClient:

    def __init__(self, settings: G8eePlatformSettings):
        self.g8ed_url = get_g8ed_url(settings)
        self._settings = settings
        self._http: HTTPClient = HTTPClient(
            component_id=ComponentName.G8EE,
            base_url=self.g8ed_url,
            timeout=G8ED_CLIENT_TIMEOUT,
            retry_config=RetryConfig(max_retries=G8ED_CLIENT_MAX_RETRIES),
            circuit_breaker_config=CircuitBreakerConfig(
                failure_threshold=G8ED_CLIENT_FAILURE_THRESHOLD,
                recovery_time=G8ED_CLIENT_RECOVERY_TIME,
            ),
            auth_token=settings.auth.internal_auth_token or "",
            api_key=settings.auth.g8e_api_key or "",
            headers={G8eHeaders.SOURCE_COMPONENT: ComponentName.G8EE},
            ca_cert_path=settings.ca_cert_path or "",
        )

        logger.info("InternalHttpClient initialized with URL: %s", self.g8ed_url)

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
        token = self.settings.auth.internal_auth_token
        if not token:
            raise ConfigurationError(
                "INTERNAL_AUTH_TOKEN is not configured — g8ee cannot authenticate with g8ed",
                component=ComponentName.G8EE,
            )
        return {
            INTERNAL_AUTH_HEADER: token,
            G8eHeaders.SOURCE_COMPONENT: ComponentName.G8EE,
        }

    async def close(self) -> None:
        await self._http.close()

    async def push_sse_event(
        self,
        event: SessionEvent | BackgroundEvent,
    ) -> SSEPushResponse:
        """POST an event to g8ed for SSE delivery.

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
            "[HTTP-G8ED] Pushing SSE event",
            extra={
                "web_session_id": (web_session_id[:8] + "...") if web_session_id else None,
                "event_type": event_type,
            }
        )

        try:
            response = await self._http.post(
                InternalApiPaths.G8ED_SSE_PUSH,
                json_data=wire_model,
                headers=self._auth_headers(),
            )
        except Exception as e:
            raise NetworkError(
                f"[HTTP-G8ED] HTTP request failed: {e}",
                component=ComponentName.G8EE,
                cause=e,
            )

        if not response.is_success:
            logger.error(
                "[HTTP-G8ED] Failed to deliver SSE event",
                extra={
                    "status": response.status_code,
                    "error": response.text,
                    "event_type": event_type,
                }
            )
            raise NetworkError(
                f"[HTTP-G8ED] SSE push returned HTTP {response.status_code}",
                component=ComponentName.G8EE,
                details={
                    "status_code": response.status_code,
                    "response": response.text,
                    "event_type": event_type,
                },
            )

        result = SSEPushResponse.model_validate(response.json())
        logger.info(
            "[HTTP-G8ED] SSE event delivered",
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
                "[HTTP-G8ED] Granting intent to operator",
                extra={"operator_id": operator_id, "intent": intent}
            )

            request_payload = IntentRequestPayload(intent=intent)

            response = await self._http.post(
                InternalApiPaths.G8ED_GRANT_INTENT.format(operator_id=operator_id),
                json_data=request_payload,
                headers=self._auth_headers(),
                context=context,
            )
            result = GrantIntentResponse.model_validate(response.json())
            if response.is_success and result.success:
                logger.info(
                    "[HTTP-G8ED] Intent granted successfully",
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
                "[HTTP-G8ED] Failed to grant intent",
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
                f"[HTTP-G8ED] Failed to grant intent: {e}",
                component=ComponentName.G8EE,
                cause=e,
            )

    async def revoke_intent(
        self,
        operator_id: str,
        intent: str,
        context: G8eHttpContext,
    ) -> IntentOperationResult:
        try:
            request_payload = IntentRequestPayload(intent=intent)

            response = await self._http.post(
                InternalApiPaths.G8ED_REVOKE_INTENT.format(operator_id=operator_id),
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
                f"[HTTP-G8ED] Failed to revoke intent: {e}",
                component=ComponentName.G8EE,
                cause=e,
            )

    async def generate_operator_link(
        self,
        user_id: str,
        operator_id: str,
        web_session_id: str,
        organization_id: str | None = None,
        context: G8eHttpContext | None = None,
    ) -> OperatorLinkResponse:
        """Generate a single-operator handshake link (dlk_ token) via g8ed.
        
        This is a prerequisite for the 'stream_operator' tool (Phase 4).
        """
        try:
            logger.info(
                "[HTTP-G8ED] Generating operator device link",
                extra={"user_id": user_id, "operator_id": operator_id}
            )

            request_payload = OperatorLinkRequestPayload(
                user_id=user_id,
                organization_id=organization_id,
                operator_id=operator_id,
                web_session_id=web_session_id,
            )

            response = await self._http.post(
                InternalApiPaths.G8ED_CREATE_OPERATOR_LINK,
                json_data=request_payload,
                headers=self._auth_headers(),
                context=context,
            )
            
            result = OperatorLinkResponse.model_validate(response.json())
            if response.is_success and result.success:
                logger.info(
                    "[HTTP-G8ED] Operator device link generated successfully",
                    extra={"user_id": user_id, "operator_id": operator_id}
                )
                return result

            logger.warning(
                "[HTTP-G8ED] Failed to generate operator device link",
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
                f"[HTTP-G8ED] Failed to generate operator device link: {e}",
                component=ComponentName.G8EE,
                cause=e,
            )

    
