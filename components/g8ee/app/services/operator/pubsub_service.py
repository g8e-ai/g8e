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

Handles all inbound g8eo result payloads by mapping event types to their
corresponding Pydantic models for type-safe parsing.
"""

import asyncio
import json
import logging
from collections.abc import Callable, Coroutine

from pydantic import ValidationError as PydanticValidationError

from app.clients.pubsub_client import PubSubClient
from app.constants.events import EventType
from app.constants.channels import OperatorChannel, PubSubChannel

from app.errors import ValidationError
from app.models.pubsub_messages import (
    G8eoResultEnvelope,
    G8eoResultPayload,
    G8eoResultPayloadAdapter,
    G8eMessage,
)

logger = logging.getLogger(__name__)


def parse_inbound_g8eo_payload(payload_raw: dict[str, object]) -> G8eoResultPayload:
    """Parse inbound g8eo payload using discriminator-based union parsing.

    The payload models use a 'payload_type' discriminator field that Pydantic uses
    to automatically determine the correct model class. This matches the wire
    deserialization pattern used for outbound payloads.

    Args:
        payload_raw: The raw payload dict from the pub/sub message

    Returns:
        A validated G8eoResultPayload instance

    Raises:
        ValidationError: If the payload_type is invalid or payload validation fails
    """
    try:
        return G8eoResultPayloadAdapter.validate_python(payload_raw)
    except PydanticValidationError as e:
        raise ValidationError(
            f"Invalid g8eo result payload: {e}",
            component="g8ee",
        ) from e


class OperatorPubSubService:
    """Owns all g8eo pub/sub lifecycle state and result dispatch.

    Manages asyncio.Future objects for execution tracking:
    - register_future(execution_id) creates a Future
    - await_future(execution_id, timeout) waits for the result
    - complete_future(execution_id, envelope) sets the Future result
    - release_future(execution_id) removes the Future from tracking

    Result routing is handled internally by completing the registered Future
    when a result message arrives.
    """

    def __init__(self) -> None:
        self.pubsub_client: PubSubClient | None = None
        self._pubsub_ready: bool = False
        self._active_operator_sessions_set: set[tuple[str, str]] = set()
        self._pending_futures: dict[str, asyncio.Future[G8eoResultEnvelope]] = {}

    def register_future(self, execution_id: str) -> asyncio.Future[G8eoResultEnvelope]:
        """Register a Future for tracking execution results.

        Args:
            execution_id: Unique identifier for the execution

        Returns:
            The Future that will be completed when the result arrives
        """
        if not execution_id:
            raise ValueError("execution_id cannot be empty")
        if execution_id in self._pending_futures:
            logger.warning("[PUBSUB] Re-registering existing execution_id: %s", execution_id)
        future: asyncio.Future[G8eoResultEnvelope] = asyncio.Future()
        self._pending_futures[execution_id] = future
        return future

    def release_future(self, execution_id: str) -> None:
        """Remove a Future from tracking after use.

        Args:
            execution_id: Unique identifier for the execution
        """
        self._pending_futures.pop(execution_id, None)

    def set_pubsub_client(self, client: PubSubClient) -> None:
        if not client:
            raise ValidationError("client is required for g8eo command communication", component="g8ee")
        self.pubsub_client = client
        logger.info("[PUBSUB] Pub/sub client configured")

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
            _prefix, operator_id, operator_session_id = OperatorChannel.parse(channel)
        except ValueError:
            logger.warning("[PUBSUB] Failed to parse results channel: %s", channel)
            return
        if isinstance(data, dict):
            raw = data
        else:
            try:
                raw = json.loads(str(data))
            except json.JSONDecodeError:
                logger.warning("[PUBSUB] Non-JSON payload on results channel %s", channel)
                return
        await self._handle_pubsub_result_message(operator_id, operator_session_id, raw)

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
            payload_raw: dict[str, object] = _raw_payload if isinstance(_raw_payload, dict) else {}  # type: ignore[reportUnknownVariableType]
            for id_field in ("case_id", "investigation_id", "task_id"):
                if not raw.get(id_field) and payload_raw.get(id_field):
                    raw[id_field] = payload_raw[id_field]
            payload = parse_inbound_g8eo_payload(payload_raw)
            envelope = G8eoResultEnvelope.model_validate({
                **raw,
                "operator_id": operator_id,
                "operator_session_id": operator_session_id,
                "payload": payload,
            })
            logger.info(
                "[PUBSUB] Received message from Operator",
                extra={"operator_id": operator_id, "event_type": envelope.event_type},
            )
            execution_id = payload.execution_id if hasattr(payload, "execution_id") else None
            if execution_id:
                future = self._pending_futures.get(execution_id)
                if future and not future.done():
                    future.set_result(envelope)
                else:
                    logger.info("[PUBSUB] No pending Future for execution_id: %s", execution_id)
        except Exception:
            logger.error("[PUBSUB] _handle_pubsub_result_message error", exc_info=True)
