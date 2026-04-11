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
from app.models.settings import VSEPlatformSettings
from app.constants import (
    INTERNAL_AUTH_HEADER,
    UNKNOWN_ERROR_MESSAGE,
    VSOD_CLIENT_FAILURE_THRESHOLD,
    VSOD_CLIENT_MAX_RETRIES,
    VSOD_CLIENT_RECOVERY_TIME,
    VSOD_CLIENT_TIMEOUT,
    ComponentName,
    VSOHeaders,
    InternalApiPaths,
)
from app.errors import ConfigurationError, NetworkError
from app.models.events import BackgroundEvent, SessionEvent
from app.models.http_context import VSOHttpContext
from app.models.vsod_client import (
    GrantIntentResponse,
    IntentOperationResult,
    IntentRequestPayload,
    RevokeIntentResponse,
    SSEPushResponse,
)

logger = logging.getLogger(__name__)


def get_vsod_url(settings: VSEPlatformSettings) -> str:
    return settings.service_urls.vsod_url


class InternalHttpClient:

    def __init__(self, settings: VSEPlatformSettings):
        self.vsod_url = get_vsod_url(settings)
        self._settings = settings
        self._http: HTTPClient = HTTPClient(
            component_id=ComponentName.VSE,
            base_url=self.vsod_url,
            timeout=VSOD_CLIENT_TIMEOUT,
            retry_config=RetryConfig(max_retries=VSOD_CLIENT_MAX_RETRIES),
            circuit_breaker_config=CircuitBreakerConfig(
                failure_threshold=VSOD_CLIENT_FAILURE_THRESHOLD,
                recovery_time=VSOD_CLIENT_RECOVERY_TIME,
            ),
            auth_token=settings.auth.internal_auth_token or "",
            api_key=settings.auth.g8e_api_key or "",
            headers={VSOHeaders.SOURCE_COMPONENT: ComponentName.VSE},
            ca_cert_path=settings.ca_cert_path,
        )

        logger.info("InternalHttpClient initialized with URL: %s", self.vsod_url)

    def configure(self, settings: VSEPlatformSettings) -> None:
        self._settings = settings

    @property
    def settings(self) -> VSEPlatformSettings:
        return self._settings

    @property
    def client(self) -> HTTPClient:
        """Access the underlying HTTP client."""
        return self._http

    def _auth_headers(self) -> dict[str, str]:
        token = self.settings.auth.internal_auth_token
        if not token:
            raise ConfigurationError(
                "INTERNAL_AUTH_TOKEN is not configured — VSE cannot authenticate with VSOD",
                component=ComponentName.VSE,
            )
        return {
            INTERNAL_AUTH_HEADER: token,
            VSOHeaders.SOURCE_COMPONENT: ComponentName.VSE,
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
                "[HTTP-VSOD] Pushing SSE event",
                extra={
                    "web_session_id": (web_session_id[:8] + "...") if web_session_id else None,
                    "event_type": event_type,
                }
            )

            response = await self._http.post(
                InternalApiPaths.PREFIX + InternalApiPaths.VSOD_SSE_PUSH,
                json_data=wire,
                headers=self._auth_headers(),
            )
            if response.is_success:
                result = SSEPushResponse.model_validate(response.json())
                logger.info(
                    "[HTTP-VSOD] SSE event delivered",
                    extra={
                        "web_session_id": (web_session_id[:8] + "...") if web_session_id else None,
                        "event_type": event_type,
                        "success": result.success,
                        "delivered": result.delivered,
                    }
                )
                return result.success
            logger.error(
                "[HTTP-VSOD] Failed to deliver SSE event",
                extra={"status": response.status_code, "error": response.text}
            )
            return False

        except Exception as e:
            raise NetworkError(
                f"[HTTP-VSOD] HTTP request failed: {e}",
                component=ComponentName.VSE,
                cause=e,
            )

    async def grant_intent(
        self,
        operator_id: str,
        intent: str,
        context: VSOHttpContext,
    ) -> IntentOperationResult:
        try:
            logger.info(
                "[HTTP-VSOD] Granting intent to operator",
                extra={"operator_id": operator_id, "intent": intent}
            )

            request_payload = IntentRequestPayload(intent=intent)

            response = await self._http.post(
                (InternalApiPaths.PREFIX + InternalApiPaths.VSOD_GRANT_INTENT).format(operator_id=operator_id),
                json_data=request_payload.flatten_for_wire(),
                headers=self._auth_headers(),
                context=context,
            )
            result = GrantIntentResponse.model_validate(response.json())
            if response.is_success and result.success:
                logger.info(
                    "[HTTP-VSOD] Intent granted successfully",
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
                "[HTTP-VSOD] Failed to grant intent",
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
                f"[HTTP-VSOD] Failed to grant intent: {e}",
                component=ComponentName.VSE,
                cause=e,
            )

    async def revoke_intent(
        self,
        operator_id: str,
        intent: str,
        context: VSOHttpContext,
    ) -> IntentOperationResult:
        try:
            request_payload = IntentRequestPayload(intent=intent)

            response = await self._http.post(
                (InternalApiPaths.PREFIX + InternalApiPaths.VSOD_REVOKE_INTENT).format(operator_id=operator_id),
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
                f"[HTTP-VSOD] Failed to revoke intent: {e}",
                component=ComponentName.VSE,
                cause=e,
            )

    async def bind_operators(
        self,
        operator_ids: list[str],
        web_session_id: str,
        context: VSOHttpContext,
    ) -> bool:
        """Delegate to OperatorDataService for proper domain separation."""
        from app.main import app  # Import here to avoid circular dependency
        
        return await app.state.operator_data_service.bind_operators(
            operator_ids=operator_ids,
            web_session_id=web_session_id,
            context=context,
        )

    
