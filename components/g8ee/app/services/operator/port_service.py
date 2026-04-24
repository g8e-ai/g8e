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

"""Operator Port Service

Port check operations via g8eo operators. No approval required.
Replaces PortOperationsMixin. Uses pubsub_service.wait_for_result().
"""

import asyncio
import logging
from typing import cast

from app.services.protocols import (
    ExecutionServiceProtocol,
    PubSubServiceProtocol,
)

from app.constants.events import EventType
from app.constants.status import (
    AITaskId,
    CommandErrorType,
    ComponentName,
    ExecutionStatus,
    NetworkProtocol,
)
from app.constants.settings import (
    OPERATOR_COMMAND_WAIT_TIMEOUT_SECONDS,
)
from app.errors import BusinessLogicError, ValidationError
from app.models.command_request_payloads import CheckPortRequestPayload
from app.models.http_context import G8eHttpContext
from app.models.investigations import EnrichedInvestigationContext
from app.models.operators import CommandExecutingBroadcastEvent, CommandResultBroadcastEvent
from app.models.pubsub_messages import PortCheckResultPayload, G8eoResultEnvelope, G8eMessage
from app.models.tool_results import PortCheckToolResult
from app.utils.timestamp import now

logger = logging.getLogger(__name__)


class OperatorPortService:
    """Port check diagnostic operations via g8eo operators."""

    def __init__(
        self,
        pubsub_service: PubSubServiceProtocol,
        execution_service: ExecutionServiceProtocol,
    ) -> None:
        self.pubsub_service = pubsub_service
        self.execution_service = execution_service

    async def execute_port_check(
        self,
        args: CheckPortRequestPayload,
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
    ) -> PortCheckToolResult:
        """Execute a single port check.

        ``execution_id`` is extracted from args.execution_id; it
        is used as the Future correlation key on ``PubSubService`` and appears
        in the STARTED / COMPLETED / FAILED UI lifecycle events. Using a
        per-call id (rather than ``g8e_context.execution_id``) ensures that
        concurrent port checks in a single chat turn do not collide on the
        pending-Future registry.
        """
        exec_id = args.execution_id
        logger.info("[PORT_CHECK] Starting port check operation (execution_id=%s)", exec_id)

        case_id = g8e_context.case_id
        user_id = g8e_context.user_id
        web_session_id = g8e_context.web_session_id

        host = args.host.strip()
        port = args.port
        try:
            protocol = NetworkProtocol(args.protocol.strip().lower())
        except ValueError:
            error_msg = f"Invalid network protocol: {args.protocol}"
            logger.error("[PORT_CHECK] %s", error_msg)
            return PortCheckToolResult(success=False, error=error_msg, error_type=CommandErrorType.EXECUTION_ERROR)

        logger.info("[PORT_CHECK] Host: '%s', Port: %d, Protocol: '%s'", host, port, protocol)

        if port < 1 or port > 65535:
            error_msg = f"Invalid port number: {port} (must be 1-65535)"
            logger.error("[PORT_CHECK] %s", error_msg)
            return PortCheckToolResult(success=False, error=error_msg, error_type=CommandErrorType.VALIDATION_ERROR)

        operator_documents = investigation.operator_documents if investigation else []
        try:
            resolved_operator = self.execution_service.resolve_target_operator(
                operator_documents=operator_documents,
                target_operator=args.target_operator,
                tool_name="check_port",
            )
        except (ValidationError, BusinessLogicError, ValueError) as e:
            logger.error("[PORT_CHECK] Operator resolution failed: %s", e, exc_info=True)
            return PortCheckToolResult(
                success=False,
                error=f"Operator resolution failed: {e}. Ensure at least one operator is online and has a valid session, then retry.",
                error_type=CommandErrorType.OPERATOR_RESOLUTION_ERROR,
            )

        operator_id = resolved_operator.id
        operator_session_id = resolved_operator.operator_session_id
        if not operator_session_id:
            return PortCheckToolResult(
                success=False,
                error="Operator session not found",
                error_type=CommandErrorType.NO_OPERATORS_AVAILABLE,
            )

        _hn = resolved_operator.current_hostname or (
            resolved_operator.system_info.hostname if resolved_operator.system_info else None
        ) or "unknown"
        logger.info("[PORT_CHECK] Resolved operator: %s (hostname: %s)", operator_id, _hn)

        if not self.pubsub_service.is_ready:
            error_msg = "Pub/sub pattern subscription not ready"
            logger.error("[PUBSUB-PATTERN] %s", error_msg)
            return PortCheckToolResult(success=False, error=error_msg, error_type=CommandErrorType.PUBSUB_SUBSCRIPTION_NOT_READY)

        try:
            command_data = G8eMessage(
                id=exec_id,
                source_component=ComponentName.G8EE,
                event_type=EventType.OPERATOR_NETWORK_PORT_CHECK_REQUESTED,
                case_id=case_id,
                investigation_id=investigation.id if investigation else "",
                task_id=AITaskId.PORT_CHECK,
                web_session_id=web_session_id,
                operator_session_id=operator_session_id,
                operator_id=operator_id,
                payload=CheckPortRequestPayload(
                    execution_id=exec_id,
                    host=host,
                    port=port,
                    protocol=protocol.value,
                ),
            )

            logger.info("[PORT_CHECK] Publishing port check request via g8es pub/sub")
            await self.pubsub_service.register_operator_session(operator_id, operator_session_id)

            # Notify start
            await self.execution_service.g8ed_event_service.publish_command_event(
                EventType.OPERATOR_NETWORK_PORT_CHECK_STARTED,
                CommandExecutingBroadcastEvent(
                    command=f"port_check {host}:{port} ({protocol})",
                    execution_id=exec_id,
                    operator_session_id=operator_session_id,
                ),
                g8e_context,
                task_id=AITaskId.PORT_CHECK,
            )

            internal_result, envelope = await self.execution_service.execute(
                g8e_message=command_data,
                g8e_context=g8e_context,
                timeout_seconds=OPERATOR_COMMAND_WAIT_TIMEOUT_SECONDS,
            )

            if not envelope:
                timeout_error = f"Port check timed out after {OPERATOR_COMMAND_WAIT_TIMEOUT_SECONDS} seconds"
                logger.warning("[PORT_CHECK] %s", timeout_error)

                # Notify failure (timeout)
                await self.execution_service.g8ed_event_service.publish_command_event(
                    EventType.OPERATOR_NETWORK_PORT_CHECK_FAILED,
                    CommandResultBroadcastEvent(
                        execution_id=exec_id,
                        command=f"port_check {host}:{port} ({protocol})",
                        status=ExecutionStatus.TIMEOUT,
                        error=timeout_error,
                        operator_id=operator_id,
                        operator_session_id=operator_session_id,
                    ),
                    g8e_context,
                    task_id=AITaskId.PORT_CHECK,
                )

                return PortCheckToolResult(
                    success=False,
                    error=timeout_error,
                    error_type=CommandErrorType.OPERATION_TIMEOUT,
                )

            if not hasattr(envelope, "payload"):
                logger.error("[PORT_CHECK] Unexpected envelope type: %s", type(envelope))
                return PortCheckToolResult(
                    success=False,
                    error="Unexpected result format from operator",
                    error_type=CommandErrorType.EXECUTION_ERROR,
                )

            if isinstance(envelope.payload, PortCheckResultPayload):
                payload = envelope.payload
                failed = envelope.event_type == EventType.OPERATOR_NETWORK_PORT_CHECK_FAILED
                
                # Notify completion/failure
                completion_event_type = (
                    EventType.OPERATOR_NETWORK_PORT_CHECK_COMPLETED 
                    if not failed 
                    else EventType.OPERATOR_NETWORK_PORT_CHECK_FAILED
                )

                await self.execution_service.g8ed_event_service.publish_command_event(
                    completion_event_type,
                    CommandResultBroadcastEvent(
                        execution_id=exec_id,
                        command=f"port_check {host}:{port} ({protocol})",
                        status=ExecutionStatus.COMPLETED if not failed else ExecutionStatus.FAILED,
                        output=f"Port {port} on {host} is {'OPEN' if payload.is_open else 'CLOSED'}" if not failed else None,
                        error=payload.error if failed else None,
                        operator_id=operator_id,
                        operator_session_id=operator_session_id,
                    ),
                    g8e_context,
                    task_id=AITaskId.PORT_CHECK,
                )

                if failed:
                    return PortCheckToolResult(
                        success=False,
                        error=payload.error or "Port check failed",
                        error_type=CommandErrorType.PORT_CHECK_FAILED,
                    )
                return PortCheckToolResult(
                    success=True,
                    host=payload.host or host,
                    port=payload.port or port,
                    protocol=cast(NetworkProtocol, payload.protocol or protocol),
                    is_open=payload.is_open,
                    latency_ms=payload.latency_ms,
                    error=payload.error,
                )

            return PortCheckToolResult(
                success=False,
                error="Unexpected result payload from operator",
                error_type=CommandErrorType.EXECUTION_ERROR,
            )
        except (ValidationError, BusinessLogicError):
            raise
        except Exception as e:
            logger.error("[PORT_CHECK] Unexpected error: %s", e, exc_info=True)
            return PortCheckToolResult(success=False, error=f"Port check execution failed: {e}. Check operator status and retry.", error_type=CommandErrorType.EXECUTION_ERROR)
