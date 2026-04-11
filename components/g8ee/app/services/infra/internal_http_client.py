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
from typing import Optional

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
from app.models.events import BackgroundEvent, SessionEvent
from app.models.http_context import G8eHttpContext
from app.models.g8ed_client import (
    GrantIntentResponse,
    IntentOperationResult,
    IntentRequestPayload,
    RevokeIntentResponse,
    SSEPushResponse,
)

logger = logging.getLogger(__name__)


def get_g8ed_url(settings: G8eePlatformSettings) -> str:
    return settings.service_urls.g8ed_url


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
            ca_cert_path=settings.ca_cert_path,
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
    ) -> bool:
        try:
            wire = event.flatten_for_wire()
            web_session_id = wire.get("web_session_id")
            event_type = wire.get("event", {}).get("type") or "None"

            logger.info(
                "[HTTP-G8ED] Pushing SSE event",
                extra={
                    "web_session_id": (web_session_id[:8] + "...") if web_session_id else None,
                    "event_type": event_type,
                }
            )

            response = await self._http.post(
                InternalApiPaths.PREFIX + InternalApiPaths.G8ED_SSE_PUSH,
                json_data=wire,
                headers=self._auth_headers(),
            )
            if response.is_success:
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
                return result.success
            logger.error(
                "[HTTP-G8ED] Failed to deliver SSE event",
                extra={"status": response.status_code, "error": response.text}
            )
            return False

        except Exception as e:
            raise NetworkError(
                f"[HTTP-G8ED] HTTP request failed: {e}",
                component=ComponentName.G8EE,
                cause=e,
            )

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
                (InternalApiPaths.PREFIX + InternalApiPaths.G8ED_GRANT_INTENT).format(operator_id=operator_id),
                json_data=request_payload.flatten_for_wire(),
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
                (InternalApiPaths.PREFIX + InternalApiPaths.G8ED_REVOKE_INTENT).format(operator_id=operator_id),
                json_data=request_payload.flatten_for_wire(),
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

    async def bind_operators(
        self,
        operator_ids: list[str],
        web_session_id: str,
        context: G8eHttpContext,
    ) -> bool:
        """Delegate to OperatorDataService for proper domain separation."""
        from app.main import app  # Import here to avoid circular dependency
        
        return await app.state.operator_data_service.bind_operators(
            operator_ids=operator_ids,
            web_session_id=web_session_id,
            context=context,
        )

    
