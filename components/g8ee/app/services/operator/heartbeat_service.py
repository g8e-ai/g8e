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
from app.constants.channels import PubSubChannel
from app.models.events import BackgroundEvent, SessionEvent
from app.models.operators import (
    HeartbeatSSEEnvelope,
    OperatorDocument,
    HeartbeatSnapshot,
)
from app.models.pubsub_messages import G8eoHeartbeatPayload
from app.security.request_timestamp import RequestValidationResult, validate_timestamp

from ..protocols import OperatorDataServiceProtocol, EventServiceProtocol

logger = logging.getLogger(__name__)


class HeartbeatSnapshotService:

    def __init__(
        self,
        operator_data_service: OperatorDataServiceProtocol,
        event_service: EventServiceProtocol,
    ):
        self._operator_data_service = operator_data_service
        self._event_service = event_service
        self._pubsub_client: PubSubClient | None = None
        self._active_sessions: set[tuple[str, str]] = set()
        self._ready = False

    @property
    def operator_data_service(self) -> OperatorDataServiceProtocol:
        return self._operator_data_service

    @property
    def event_service(self) -> EventServiceProtocol:
        return self._event_service

    def set_pubsub_client(self, client: PubSubClient) -> None:
        if not client:
            raise ConfigurationError("client is required for heartbeat pub/sub", component="g8ee")
        self._pubsub_client = client
        self._pubsub_client.on_disconnect(self._on_ws_disconnect)
        logger.info("HeartbeatSnapshotService pubsub client configured")

    async def start(self) -> None:
        if self._ready:
            return
        if not self._pubsub_client:
            raise ConfigurationError("pubsub_client must be set before calling start()", component="g8ee")
        await self._pubsub_client.ensure_connected()

        # Pattern-subscribe to every operator heartbeat channel. g8eo publishes
        # its bootstrap heartbeat the instant authentication completes, so g8ee
        # MUST be listening before any specific operator session exists. A single
        # pattern subscription captures every operator's heartbeats from the
        # first packet onward without per-session register/deregister races.
        pattern = f"{PubSubChannel.HEARTBEAT_PREFIX.value}:*"
        self._pubsub_client.on_pmessage(pattern, self._on_pattern_heartbeat_message)
        await self._pubsub_client.psubscribe(pattern)
        logger.info("[HEARTBEAT] Pattern-subscribed to %s", pattern)

        self._ready = True
        logger.info("[HEARTBEAT] Pub/sub client ready")

    async def stop(self) -> None:
        if self._pubsub_client:
            pattern = f"{PubSubChannel.HEARTBEAT_PREFIX.value}:*"
            try:
                await self._pubsub_client.punsubscribe(pattern)
            except Exception:
                logger.warning("[HEARTBEAT] Failed to punsubscribe %s on stop", pattern, exc_info=True)
        self._active_sessions.clear()
        self._ready = False
        logger.info("[HEARTBEAT] Heartbeat pattern subscription stopped")

    async def _on_ws_disconnect(self) -> None:
        self._ready = False
        logger.warning("[HEARTBEAT] WebSocket disconnected — ready state reset, preserving active sessions for re-subscription")

    async def register_operator_session(self, operator_id: str, operator_session_id: str) -> None:
        """Track an operator session as active.

        Subscription is handled via a single ``heartbeat:*`` pattern set up in
        ``start()`` — this method only records the (operator, session) pair so
        callers can observe activity. It is idempotent and never opens a new
        per-session pubsub subscription.
        """
        self._active_sessions.add((operator_id, operator_session_id))
        logger.info(
            "[HEARTBEAT] Tracked operator session",
            extra={"operator_id": operator_id, "operator_session_id": operator_session_id},
        )

    async def deregister_operator_session(self, operator_id: str, operator_session_id: str) -> None:
        """Stop tracking an operator session. No pubsub state to release—
        the pattern subscription is shared across all operators.
        """
        self._active_sessions.discard((operator_id, operator_session_id))
        logger.info(
            "[HEARTBEAT] Untracked operator session",
            extra={"operator_id": operator_id, "operator_session_id": operator_session_id},
        )

    async def _on_pattern_heartbeat_message(
        self, pattern: str, channel: str, data: str | dict[str, object]
    ) -> None:
        """Pattern-message dispatcher for ``heartbeat:*`` channels."""
        await self._on_heartbeat_message(channel, data)

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
            self._active_sessions.add((operator_id, operator_session_id))
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
                "system_architecture": payload.system_identity.architecture,
                "system_cpu_count": payload.system_identity.cpu_count,
                "system_memory_mb": payload.system_identity.memory_mb,
                "system_user": payload.system_identity.current_user,
                "public_ip": payload.network_info.public_ip,
                "cpu_percent": payload.performance_metrics.cpu_percent,
                "memory_percent": payload.performance_metrics.memory_percent,
                "disk_percent": payload.performance_metrics.disk_percent,
                "network_latency": payload.performance_metrics.network_latency,
                "system_fingerprint": payload.system_fingerprint,
            }
        )

        if not self._validate_operator_identity(operator_id, payload, operator_session_id):
            return False

        operator = await self._get_and_validate_operator(operator_id, operator_session_id, payload)
        if not operator:
            return False

        heartbeat = HeartbeatSnapshot.from_wire(payload)

        db_success = await self.operator_data_service.update_operator_heartbeat(
            operator_id=operator_id,
            heartbeat=heartbeat,
            investigation_id=payload.investigation_id,
            case_id=payload.case_id,
        )

        if not db_success:
            return False

        envelope = HeartbeatSSEEnvelope.from_heartbeat(operator_id, operator.status, heartbeat)
        await self._push_heartbeat_sse(envelope, payload, operator)

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
                error=ts_result.error or "Timestamp validation failed",
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

        logger.info(
            "Operator %s validated for heartbeat",
            operator_id,
            extra={"operator_id": operator_id, "status": operator.status},
        )
        return operator

    async def _push_heartbeat_sse(
        self,
        envelope: HeartbeatSSEEnvelope,
        payload: G8eoHeartbeatPayload,
        operator: OperatorDocument,
    ) -> None:
        event = self._build_heartbeat_event(envelope, payload, operator)
        try:
            await self.event_service.publish(event)
        except Exception as e:
            logger.warning(
                "[HEARTBEAT] SSE push failed (non-blocking): %s",
                e,
                exc_info=True,
            )

    @staticmethod
    def _build_heartbeat_event(
        envelope: HeartbeatSSEEnvelope,
        payload: G8eoHeartbeatPayload,
        operator: OperatorDocument,
    ) -> SessionEvent | BackgroundEvent:
        """Build the routing event: SessionEvent when the operator is bound to a
        web session (targeted delivery), BackgroundEvent otherwise (fan-out by user_id).
        """
        if operator.bound_web_session_id:
            return SessionEvent(
                event_type=EventType.OPERATOR_HEARTBEAT_RECEIVED,
                payload=envelope,
                web_session_id=operator.bound_web_session_id,
                user_id=operator.user_id,
                case_id=payload.case_id,
                investigation_id=payload.investigation_id,
            )
        return BackgroundEvent(
            event_type=EventType.OPERATOR_HEARTBEAT_RECEIVED,
            payload=envelope,
            user_id=operator.user_id,
            investigation_id=payload.investigation_id,
            case_id=payload.case_id,
        )
