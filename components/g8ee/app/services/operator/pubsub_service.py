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

"""Operator PubSub Service

Owns all g8eo pub/sub lifecycle state:
  - pubsub_client reference
  - _pubsub_ready flag
  - _active_operator_sessions_set

Exposes wait_for_result(execution_id, timeout) to replace every hand-rolled
polling loop that previously lived across every mixin.
"""

import json
import logging
from collections.abc import Callable, Coroutine
from uuid import uuid4

from app.clients.pubsub_client import PubSubClient
from app.constants.events import EventType
from app.constants.channels import PubSubChannel
from pydantic import ValidationError as PydanticValidationError

from app.errors import ValidationError
from app.models.pubsub_messages import (
    CancellationResultPayload,
    ExecutionResultsPayload,
    ExecutionStatusPayload,
    FetchFileDiffResultPayload,
    FetchFileHistoryResultPayload,
    FetchHistoryResultPayload,
    FetchLogsResultPayload,
    FileEditResultPayload,
    FsListResultPayload,
    FsReadResultPayload,
    PortCheckResultPayload,
    RestoreFileResultPayload,
    ShutdownAckPayload,
    G8eoResultEnvelope,
    G8eoResultPayload,
    G8eMessage,
)
from app.services.mcp.adapter import parse_tool_call_result
from app.constants.status import ExecutionStatus

logger = logging.getLogger(__name__)

_FILE_EDIT_EVENTS = frozenset({
    EventType.OPERATOR_FILE_EDIT_COMPLETED,
    EventType.OPERATOR_FILE_EDIT_FAILED,
})


# Map event types to payload models for reconstruction
_PAYLOAD_MODELS = {
    EventType.OPERATOR_FILE_EDIT_COMPLETED: FileEditResultPayload,
    EventType.OPERATOR_FILE_EDIT_FAILED: FileEditResultPayload,
    EventType.OPERATOR_NETWORK_PORT_CHECK_COMPLETED: PortCheckResultPayload,
    EventType.OPERATOR_NETWORK_PORT_CHECK_FAILED: PortCheckResultPayload,
    EventType.OPERATOR_FILESYSTEM_LIST_COMPLETED: FsListResultPayload,
    EventType.OPERATOR_FILESYSTEM_LIST_FAILED: FsListResultPayload,
    EventType.OPERATOR_FILESYSTEM_READ_COMPLETED: FsReadResultPayload,
    EventType.OPERATOR_FILESYSTEM_READ_FAILED: FsReadResultPayload,
    EventType.OPERATOR_FILE_HISTORY_FETCH_COMPLETED: FetchFileHistoryResultPayload,
    EventType.OPERATOR_FILE_HISTORY_FETCH_FAILED: FetchFileHistoryResultPayload,
    EventType.OPERATOR_FILE_RESTORE_COMPLETED: RestoreFileResultPayload,
    EventType.OPERATOR_FILE_RESTORE_FAILED: RestoreFileResultPayload,
    EventType.OPERATOR_FILE_DIFF_FETCH_COMPLETED: FetchFileDiffResultPayload,
    EventType.OPERATOR_FILE_DIFF_FETCH_FAILED: FetchFileDiffResultPayload,
    EventType.OPERATOR_LOGS_FETCH_COMPLETED: FetchLogsResultPayload,
    EventType.OPERATOR_LOGS_FETCH_FAILED: FetchLogsResultPayload,
    EventType.OPERATOR_HISTORY_FETCH_COMPLETED: FetchHistoryResultPayload,
    EventType.OPERATOR_HISTORY_FETCH_FAILED: FetchHistoryResultPayload,
}


def _parse_g8eo_payload(event_type_raw: object, payload_raw: dict[str, object]) -> G8eoResultPayload:
    event_type = EventType(event_type_raw) if isinstance(event_type_raw, str) else event_type_raw

    if event_type == EventType.OPERATOR_SHUTDOWN_ACKNOWLEDGED:
        return ShutdownAckPayload.model_validate(payload_raw)
    if event_type == EventType.OPERATOR_COMMAND_STATUS_UPDATED_RUNNING:
        return ExecutionStatusPayload.model_validate(payload_raw)
    if event_type == EventType.OPERATOR_COMMAND_CANCELLED:
        return CancellationResultPayload.model_validate(payload_raw)

    if event_type == EventType.OPERATOR_MCP_TOOLS_RESULT:
        # MCP results contain structured data. Reconstruct the original typed payload.
        mcp_result = parse_tool_call_result(payload_raw)
        
        # Metadata check (Smell #1 Fix)
        if mcp_result.metadata and "original_payload" in mcp_result.metadata:
            original_payload = mcp_result.metadata["original_payload"]
            mcp_event_type_raw = mcp_result.metadata.get("event_type")

            if mcp_event_type_raw:
                try:
                    mcp_event_type = EventType(mcp_event_type_raw)
                    if model_cls := _PAYLOAD_MODELS.get(mcp_event_type):
                        return model_cls.model_validate(original_payload)
                except (ValueError, PydanticValidationError) as exc:
                    logger.warning(
                        "[MCP] Failed to reconstruct typed payload from metadata, "
                        "falling back to ExecutionResultsPayload: %s",
                        exc,
                    )

        # Command execution results have no _metadata (standard stdout/stderr output)
        stdout = "\n".join(c.text for c in mcp_result.content if c.type == "text" and c.text and not mcp_result.isError)
        stderr = "\n".join(c.text for c in mcp_result.content if c.type == "text" and c.text and mcp_result.isError)
        
        # execution_id: try MCP metadata first, then the JSON-RPC envelope ID, finally a fresh UUID
        execution_id = mcp_result.execution_id or payload_raw.get("id") or str(uuid4())

        return ExecutionResultsPayload(
            execution_id=execution_id,
            status=ExecutionStatus.COMPLETED if not mcp_result.isError else ExecutionStatus.FAILED,
            stdout=stdout.strip(),
            stderr=stderr.strip(),
        )

    if event_type in _PAYLOAD_MODELS:
        return _PAYLOAD_MODELS[event_type].model_validate(payload_raw)

    return ExecutionResultsPayload.model_validate(payload_raw)


class OperatorPubSubService:
    """Owns all g8eo pub/sub lifecycle state and result dispatch.

    Consumers call wait_for_result(execution_id, timeout) instead of spinning
    on asyncio.sleep(). Before publishing a command, call allocate_event(id).
    After consuming the result, call release_event(id).

    Result routing is delegated to an injectable result_handler callback so
    the service that owns the pending command store can update it without
    creating a circular dependency.
    """

    def __init__(self) -> None:
        self.pubsub_client: PubSubClient
        self._pubsub_ready: bool = False
        self._active_operator_sessions_set: set[tuple[str, str]] = set()
        self._result_handlers: list[Callable[[G8eoResultEnvelope], Coroutine[object, object, None]]] = []

    def subscribe_results(self, handler: Callable[[G8eoResultEnvelope], Coroutine[object, object, None]]) -> None:
        """Register a callback for inbound g8eo result messages."""
        if handler not in self._result_handlers:
            self._result_handlers.append(handler)

    def set_pubsub_client(self, client: PubSubClient) -> None:
        if not client:
            raise ValidationError("client is required for g8eo command communication", component="g8ee")
        self.pubsub_client = client
        logger.info("[PUBSUB] Pub/sub client configured")

    def _install_msg_capture(self) -> list[G8eMessage]:
        """Capture all published commands for verification in tests."""
        if not hasattr(self, "_captured_publish_commands"):
            self._captured_publish_commands: list[G8eMessage] = []

        async def _capture_publish(operator_id, operator_session_id, command_data):
            self._captured_publish_commands.append(command_data)
            if self.pubsub_client:
                return await self.pubsub_client.publish_command(
                    operator_id=operator_id,
                    operator_session_id=operator_session_id,
                    command_data=command_data
                )
            return 1

        self._capture_publish_internal = _capture_publish
        return self._captured_publish_commands

    async def start(self) -> None:
        if self._pubsub_ready:
            logger.info("[PUBSUB] Pub/sub client already ready")
            return
        if self.pubsub_client is None:
            raise ValidationError("pubsub_client not initialized — call set_pubsub_client() first", component="g8ee")
        await self.pubsub_client.ensure_connected()
        self._pubsub_ready = True
        logger.info("[PUBSUB] Pub/sub client ready — awaiting operator session registrations")

    async def stop(self) -> None:
        for operator_id, operator_session_id in list(self._active_operator_sessions_set):
            await self._unsubscribe_results_channel(operator_id, operator_session_id)
        self._pubsub_ready = False
        logger.info("[PUBSUB] All operator result channel subscriptions stopped")

    @property
    def is_ready(self) -> bool:
        return self._pubsub_ready

    async def register_operator_session(self, operator_id: str, operator_session_id: str) -> None:
        key = (operator_id, operator_session_id)
        if key in self._active_operator_sessions_set:
            return
        if self.pubsub_client is None:
            raise ValidationError("pubsub_client not initialized — call set_pubsub_client() first", component="g8ee")
        results_ch = PubSubChannel.results(operator_id, operator_session_id)
        self.pubsub_client.on_channel_message(results_ch, self._dispatch_results_message)
        await self.pubsub_client.subscribe(results_ch)
        self._active_operator_sessions_set.add(key)
        logger.info(
            "[PUBSUB] Registered operator session results channel",
            extra={"operator_id": operator_id, "operator_session_id": operator_session_id},
        )

    async def deregister_operator_session(self, operator_id: str, operator_session_id: str) -> None:
        await self._unsubscribe_results_channel(operator_id, operator_session_id)

    async def publish_command(self, operator_id: str, operator_session_id: str, command_data: G8eMessage) -> int:
        if hasattr(self, "_capture_publish_internal"):
            return await self._capture_publish_internal(operator_id, operator_session_id, command_data)
        if self.pubsub_client is None:
            raise ValidationError("pubsub_client not initialized — call set_pubsub_client() first", component="g8ee")
        return await self.pubsub_client.publish_command(
            operator_id=operator_id,
            operator_session_id=operator_session_id,
            command_data=command_data,
        )

    async def _unsubscribe_results_channel(self, operator_id: str, operator_session_id: str) -> None:
        key = (operator_id, operator_session_id)
        self._active_operator_sessions_set.discard(key)
        if self.pubsub_client is None:
            return
        results_ch = PubSubChannel.results(operator_id, operator_session_id)
        self.pubsub_client.off_channel_message(results_ch, self._dispatch_results_message)
        await self.pubsub_client.unsubscribe(results_ch)
        logger.info(
            "[PUBSUB] Deregistered operator session results channel",
            extra={"operator_id": operator_id, "operator_session_id": operator_session_id},
        )

    async def _dispatch_results_message(self, channel: str, data: str | dict[str, object]) -> None:
        try:
            # channel format: results:operator_id:operator_session_id
            parts = channel.split(":")
            if len(parts) != 3:
                logger.warning("[PUBSUB] Failed to parse results channel: %s", channel)
                return
            
            operator_id = parts[1]
            operator_session_id = parts[2]
            
            if not operator_id or not operator_session_id:
                logger.warning("[PUBSUB] Failed to parse results channel: %s", channel)
                return
            raw = data if isinstance(data, dict) else json.loads(str(data))
            await self._handle_pubsub_result_message(operator_id, operator_session_id, raw)
        except Exception:
            logger.error("[PUBSUB] _dispatch_results_message error", exc_info=True)

    async def _handle_pubsub_result_message(
        self,
        operator_id: str,
        operator_session_id: str,
        raw: dict[str, object],
    ) -> None:
        try:
            event_type_raw = raw.get("event_type")
            if not event_type_raw:
                logger.warning("[PUBSUB] Received message without event_type; ignoring")
                return
            _raw_payload = raw.get("payload")
            payload_raw: dict[str, object] = _raw_payload if isinstance(_raw_payload, dict) else {}
            for id_field in ("case_id", "investigation_id", "task_id"):
                if not raw.get(id_field) and payload_raw.get(id_field):
                    raw[id_field] = payload_raw[id_field]
            payload = _parse_g8eo_payload(event_type_raw, payload_raw)
            envelope = G8eoResultEnvelope.model_validate({
                **raw,
                "operator_id": operator_id,
                "operator_session_id": operator_session_id,
                "payload": payload_raw,
            })
            envelope = envelope.model_copy(update={"payload": payload})
            logger.info(
                "[PUBSUB] Received message from Operator",
                extra={"operator_id": operator_id, "event_type": envelope.event_type},
            )
            for handler in self._result_handlers:
                try:
                    await handler(envelope)
                except Exception:
                    logger.error("[PUBSUB] Result handler failed", exc_info=True)
        except Exception:
            logger.error("[PUBSUB] _handle_pubsub_result_message error", exc_info=True)
