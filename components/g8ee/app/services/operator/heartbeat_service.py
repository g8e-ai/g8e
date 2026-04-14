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

import json
import logging

from app.clients.pubsub_client import PubSubClient
from app.errors import ConfigurationError
from app.constants.events import EventType
from app.constants.status import OperatorStatus
from app.constants.channels import PubSubChannel
from app.models.events import SessionEvent
from app.models.operators import (
    HeartbeatSSEPayload,
    OperatorDocument,
    OperatorHeartbeat,
    OperatorPanelListUpdatedPayload,
)
from app.models.pubsub_messages import G8eoHeartbeatPayload
from app.security.request_timestamp import RequestValidationResult, validate_timestamp

from typing import TYPE_CHECKING, cast

if TYPE_CHECKING:
    from ..protocols import OperatorDataServiceProtocol, EventServiceProtocol

logger = logging.getLogger(__name__)

VALID_HEARTBEAT_STATUSES = {
    OperatorStatus.ACTIVE,
    OperatorStatus.BOUND,
    OperatorStatus.STALE,
    OperatorStatus.OFFLINE,
}


class OperatorHeartbeatService:

    def __init__(
        self,
        operator_data_service: "OperatorDataServiceProtocol",
        event_service: "EventServiceProtocol",
    ):
        self.operator_data_service = operator_data_service
        self.event_service = event_service
        self._pubsub_client: PubSubClient | None = None
        self._active_sessions: set[tuple[str, str]] = set()
        self._ready = False

    def set_pubsub_client(self, client: PubSubClient) -> None:
        if not client:
            raise ConfigurationError("client is required for heartbeat pub/sub", component="g8ee")
        self._pubsub_client = client
        self._pubsub_client.on_disconnect(self._on_ws_disconnect)
        logger.info("OperatorHeartbeatService pubsub client configured")

    async def start(self) -> None:
        if self._ready:
            return
        if not self._pubsub_client:
            raise ConfigurationError("pubsub_client must be set before calling start()", component="g8ee")
        await self._pubsub_client.ensure_connected()
        self._ready = True
        logger.info("[HEARTBEAT] Pub/sub client ready")

    async def stop(self) -> None:
        for operator_id, operator_session_id in list(self._active_sessions):
            await self._unsubscribe(operator_id, operator_session_id)
        self._ready = False
        logger.info("[HEARTBEAT] All heartbeat channel subscriptions stopped")

    async def _on_ws_disconnect(self) -> None:
        self._ready = False
        self._active_sessions.clear()
        logger.warning("[HEARTBEAT] WebSocket disconnected — ready state reset, subscriptions cleared")

    async def register_operator_session(self, operator_id: str, operator_session_id: str) -> None:
        key = (operator_id, operator_session_id)
        if key in self._active_sessions:
            return
        if not self._pubsub_client:
            raise ConfigurationError("pubsub_client is not set", component="g8ee")
        channel = PubSubChannel.heartbeat(operator_id, operator_session_id)
        self._pubsub_client.on_channel_message(channel, self._on_heartbeat_message)
        await self._pubsub_client.subscribe(channel)
        self._active_sessions.add(key)
        logger.info(
            "[HEARTBEAT] Registered operator session",
            extra={"operator_id": operator_id, "operator_session_id": operator_session_id},
        )

    async def deregister_operator_session(self, operator_id: str, operator_session_id: str) -> None:
        await self._unsubscribe(operator_id, operator_session_id)

    async def _unsubscribe(self, operator_id: str, operator_session_id: str) -> None:
        key = (operator_id, operator_session_id)
        self._active_sessions.discard(key)
        if not self._pubsub_client:
            return
        channel = PubSubChannel.heartbeat(operator_id, operator_session_id)
        self._pubsub_client.off_channel_message(channel, self._on_heartbeat_message)
        await self._pubsub_client.unsubscribe(channel)
        logger.info(
            "[HEARTBEAT] Deregistered operator session",
            extra={"operator_id": operator_id, "operator_session_id": operator_session_id},
        )

    async def _on_heartbeat_message(self, channel: str, data: str | dict[str, object]) -> None:
        try:
            # channel format: heartbeat:operator_id:operator_session_id
            parts = channel.split(":")
            if len(parts) != 3:
                logger.warning("[HEARTBEAT] Invalid channel format: %s", channel)
                return
            
            operator_id = parts[1]
            operator_session_id = parts[2]
            
            if not operator_id or not operator_session_id:
                logger.warning("[HEARTBEAT] Missing operator_id or operator_session_id in channel: %s", channel)
                return
            raw = data if isinstance(data, dict) else json.loads(data)
            payload = G8eoHeartbeatPayload.model_validate(raw)
            if (operator_id, operator_session_id) not in self._active_sessions:
                await self.register_operator_session(operator_id, operator_session_id)
            success = await self.process_heartbeat_message(operator_id, operator_session_id, payload)
            if not success:
                logger.warning("[HEARTBEAT] Heartbeat processing failed for operator %s", operator_id)
        except Exception:
            logger.error("[HEARTBEAT] Failed to handle heartbeat message", exc_info=True)

    async def process_heartbeat_message(
        self,
        operator_id: str,
        operator_session_id: str,
        payload: G8eoHeartbeatPayload,
    ) -> bool:
        ts_result = self._validate_heartbeat_timestamp(payload)
        if not ts_result.is_valid:
            logger.warning(
                "Heartbeat rejected - timestamp validation failed: %s",
                ts_result.error,
                extra={
                    "error_code": ts_result.error_code,
                    "operator_id": operator_id,
                    "security_event": "heartbeat_timestamp_rejected",
                }
            )
            return False

        logger.info(
            "Heartbeat service received message from g8eo",
            extra={
                "event_type": payload.event_type,
                "operator_session_id": operator_session_id,
                "system_hostname": payload.system_identity.hostname,
                "system_os": payload.system_identity.os,
                "system_user": payload.system_identity.current_user,
                "public_ip": payload.network_info.public_ip,
                "system_fingerprint": payload.system_fingerprint,
            }
        )

        if not self._validate_operator_identity(operator_id, payload, operator_session_id):
            return False

        operator = await self._get_and_validate_operator(operator_id, operator_session_id, payload)
        if not operator:
            return False

        heartbeat = OperatorHeartbeat.from_wire(payload)

        db_success = await self.operator_data_service.update_operator_heartbeat(
            operator_id=operator_id,
            heartbeat=heartbeat,
            investigation_id=payload.investigation_id or "unknown",
            case_id=payload.case_id or "unknown",
        )

        if not db_success:
            return False

        sse_payload = heartbeat.to_sse_payload(operator_id)
        sse_payload.status = operator.status
        await self._push_heartbeat_sse(sse_payload, payload, operator)

        logger.info(
            "Heartbeat processed successfully for Operator %s",
            operator_id,
            extra={
                "operator_id": operator_id,
                "investigation_id": payload.investigation_id,
            }
        )
        return True

    def _validate_heartbeat_timestamp(self, payload: G8eoHeartbeatPayload) -> RequestValidationResult:
        ts_result = validate_timestamp(payload.timestamp)
        if not ts_result.is_valid:
            return RequestValidationResult(
                is_valid=False,
                error=ts_result.error,
                error_code=ts_result.error_code,
            )
        return RequestValidationResult(is_valid=True)

    def _validate_operator_identity(
        self,
        channel_operator_id: str,
        payload: G8eoHeartbeatPayload,
        operator_session_id: str,
    ) -> bool:
        if payload.operator_id and payload.operator_id != channel_operator_id:
            logger.error(
                "[SECURITY] Operator ID mismatch - payload claims different identity than channel",
                extra={
                    "channel_operator_id": channel_operator_id,
                    "payload_operator_id": payload.operator_id,
                    "operator_session_id": operator_session_id,
                    "security_event": "operator_identity_spoofing_attempt",
                }
            )
            return False
        return True

    async def _get_and_validate_operator(
        self,
        operator_id: str,
        operator_session_id: str,
        payload: G8eoHeartbeatPayload,
    ) -> OperatorDocument | None:
        operator: OperatorDocument | None = await self.operator_data_service.get_operator(operator_id)

        if not operator:
            logger.warning(
                "Ignoring heartbeat from unknown Operator %s (invalid API key?)",
                operator_id,
                extra={
                    "operator_id": operator_id,
                    "operator_session_id": operator_session_id,
                    "security_event": "unauthenticated_operator_heartbeat",
                    "system_hostname": payload.system_identity.hostname,
                    "system_os": payload.system_identity.os,
                    "system_user": payload.system_identity.current_user,
                    "public_ip": payload.network_info.public_ip,
                    "system_fingerprint": payload.system_fingerprint,
                },
            )
            return None

        if operator.status not in VALID_HEARTBEAT_STATUSES:
            logger.warning(
                "Ignoring heartbeat from Operator %s with invalid status: %s",
                operator_id,
                operator.status,
                extra={
                    "operator_id": operator_id,
                    "status": operator.status,
                    "valid_statuses": list(VALID_HEARTBEAT_STATUSES),
                    "operator_session_id": operator_session_id,
                    "security_event": "invalid_status_operator_heartbeat",
                }
            )
            return None

        logger.info(
            "Operator %s validated for heartbeat",
            operator_id,
            extra={"operator_id": operator_id, "status": operator.status},
        )
        return operator

    async def _push_heartbeat_sse(
        self,
        sse_payload: HeartbeatSSEPayload,
        payload: G8eoHeartbeatPayload,
        operator: OperatorDocument,
    ) -> None:
        web_session_id = operator.web_session_id
        user_id = operator.user_id
        if not web_session_id or not user_id:
            logger.error(
                "[HEARTBEAT] Cannot push SSE — operator %s has missing web_session_id or user_id (broken bind)",
                operator.operator_id,
                extra={
                    "operator_id": operator.operator_id,
                    "web_session_id": web_session_id,
                    "user_id": user_id,
                },
            )
            return
        try:
            await self.event_service.publish(
                SessionEvent(
                    event_type=EventType.OPERATOR_HEARTBEAT_RECEIVED,
                    payload=sse_payload,
                    web_session_id=web_session_id,
                    user_id=user_id,
                    case_id=payload.case_id,
                    investigation_id=payload.investigation_id,
                )
            )
            # Always send OPERATOR_PANEL_LIST_UPDATED to ensure Operator Panel reflects current status
            await self.event_service.publish(
                SessionEvent(
                    event_type=EventType.OPERATOR_PANEL_LIST_UPDATED,
                    payload=OperatorPanelListUpdatedPayload(
                        operator_id=operator.operator_id,
                        case_id=payload.case_id,
                        investigation_id=payload.investigation_id,
                    ),
                    web_session_id=web_session_id,
                    user_id=user_id,
                    case_id=payload.case_id,
                    investigation_id=payload.investigation_id,
                )
            )
        except Exception as e:
            logger.warning("[HEARTBEAT] SSE push failed (non-blocking): %s", e)
