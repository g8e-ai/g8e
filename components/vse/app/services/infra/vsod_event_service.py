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

from __future__ import annotations

import logging

from app.constants import ErrorCode, EVENT_PUBLISH_SUCCESS, EventType
from app.errors import NetworkError
from app.models.base import VSOBaseModel
from app.models.events import BackgroundEvent, SessionEvent
from app.models.http_context import VSOHttpContext
from .internal_http_client import InternalHttpClient

logger = logging.getLogger(__name__)


class EventService:

    def __init__(self, internal_http_client: InternalHttpClient):
        self.vsod_client = internal_http_client

    async def publish(self, event: SessionEvent | BackgroundEvent) -> str:
        web_session_id = event.web_session_id if isinstance(event, SessionEvent) else None

        logger.info(
            "[HTTP-VSOD] Sending event '%s' to VSOD",
            event.event_type,
            extra={
                "case_id": event.case_id,
                "investigation_id": event.investigation_id,
                "web_session_id": web_session_id,
            },
        )

        success = await self.vsod_client.push_sse_event(event)

        if success:
            logger.info(
                "[HTTP-VSOD] Event delivered successfully",
                extra={"event_type": event.event_type, "web_session_id": web_session_id}
            )
            return EVENT_PUBLISH_SUCCESS

        raise NetworkError(
            f"Failed to deliver event '{event.event_type}' to VSOD",
            code=ErrorCode.API_RESPONSE_ERROR,
            component="vse",
        )

    async def publish_command_event(
        self,
        event_type: EventType,
        data: VSOBaseModel,
        vso_context: VSOHttpContext,
        *,
        task_id: str,
    ) -> None:
        logger.info(
            "[HTTP-VSOD] Publishing command event '%s' for task %s",
            event_type,
            task_id,
            extra={
                "case_id": vso_context.case_id,
                "investigation_id": vso_context.investigation_id,
                "web_session_id": vso_context.web_session_id,
                "task_id": task_id,
            },
        )
        
        try:
            await self.publish(
                SessionEvent(
                    event_type=event_type,
                    payload=data,
                    web_session_id=vso_context.web_session_id,
                    user_id=vso_context.user_id,
                    case_id=vso_context.case_id,
                    investigation_id=vso_context.investigation_id,
                    task_id=task_id,
                )
            )
        except Exception as e:
            logger.warning("Failed to broadcast command event %s: %s", event_type, e)

    async def publish_investigation_event(
        self,
        investigation_id: str,
        event_type: EventType,
        payload: dict[str, Any] | VSOBaseModel,
        web_session_id: str,
        case_id: str,
        user_id: str | None = None,
    ) -> None:
        """Publish a session event specifically for an investigation."""
        logger.info(
            "[HTTP-VSOD] Publishing investigation event '%s' for investigation %s",
            event_type,
            investigation_id,
            extra={
                "case_id": case_id,
                "investigation_id": investigation_id,
                "web_session_id": web_session_id,
                "user_id": user_id,
            },
        )
        
        try:
            await self.publish(
                SessionEvent(
                    event_type=event_type,
                    payload=payload,
                    investigation_id=investigation_id,
                    web_session_id=web_session_id,
                    case_id=case_id,
                    user_id=user_id,
                )
            )
        except Exception as e:
            logger.warning("Failed to publish investigation event %s for %s: %s", event_type, investigation_id, e)
