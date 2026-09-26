# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Operator Execution Service

Handles g8eo execution dispatch, validation, security checks, operator resolution,
and result assembly.
"""

import asyncio
import logging
from app.clients.gateway_operator_client import GatewayOperatorClient
from app.constants import EventType
from app.constants.generated_status import (
    AITaskId,
    CommandErrorType,
)
from app.constants.config import ExecutionStatus
from app.errors import BusinessLogicError, NetworkError, ValidationError
from app.services.protocols import (
    AIResponseAnalyzerProtocol,
    ApprovalServiceProtocol,
    EventServiceProtocol,
    ExecutionServiceProtocol,
    InvestigationServiceProtocol,
    OperatorDataServiceProtocol,
    PubSubServiceProtocol,
)

from app.models.tool_results import CommandExecutionResult
from app.models.command_request_payloads import CommandCancelRequestPayload, CommandRequestPayload
from app.models.internal_api import DirectCommandRequest
from app.models.operators import (
    CancelCommandResult,
    CommandFailedBroadcastEvent,
    CommandResultBroadcastEvent,
    DirectCommandResult,
    OperatorDocument,
    TargetSystem,
)
from app.models.pubsub_messages import G8eMessage, G8eoResultEnvelope, ExecutionResultsPayload
from app.models.tool_results import CommandInternalResult
from app.models.http_context import G8eHttpContext
from app.models.settings import G8eeAppSettings
from app.utils.validators import get_blacklist_validator, get_whitelist_validator
from app.utils.gateway_dispatch_result import envelope_from_gateway_dispatch

logger = logging.getLogger(__name__)


class OperatorExecutionService(ExecutionServiceProtocol):
    """Handles execution dispatch, validation, and result assembly."""

    def __init__(
        self,
        pubsub_service: PubSubServiceProtocol,
        approval_service: ApprovalServiceProtocol,
        event_service: EventServiceProtocol,
        settings: G8eeAppSettings,
        ai_response_analyzer: AIResponseAnalyzerProtocol,
        operator_data_service: OperatorDataServiceProtocol,
        investigation_service: InvestigationServiceProtocol,
        gateway_operator_client: GatewayOperatorClient | None = None,
    ) -> None:
        self._pubsub_service = pubsub_service
        self._approval_service = approval_service
        self._event_service = event_service
        self._settings = settings
        self._operator_data_service = operator_data_service
        self._ai_response_analyzer = ai_response_analyzer
        self._investigation_service = investigation_service
        self._gateway_operator_client = gateway_operator_client
        self._background_tasks: set[asyncio.Task[None]] = set()

        self.whitelist_validator = get_whitelist_validator()
        self.blacklist_validator = get_blacklist_validator()

    @property
    def pubsub_service(self) -> PubSubServiceProtocol:
        return self._pubsub_service

    @property
    def approval_service(self) -> ApprovalServiceProtocol:
        return self._approval_service

    @property
    def event_service(self) -> EventServiceProtocol:
        return self._event_service

    @property
    def operator_data_service(self) -> OperatorDataServiceProtocol:
        return self._operator_data_service

    @property
    def ai_response_analyzer(self) -> AIResponseAnalyzerProtocol:
        return self._ai_response_analyzer

    @property
    def investigation_service(self) -> InvestigationServiceProtocol:
        return self._investigation_service

    # -------------------------------------------------------------------------
    # Failure helper
    # -------------------------------------------------------------------------

    async def _fail_command(
        self,
        error_msg: str,
        error_type: CommandErrorType,
        command: str,
        g8e_context: G8eHttpContext,
        *,
        execution_id: str,
        operator_session_id: str,
        status: ExecutionStatus,
        approval_id: str,
        rule: str,
        violations: list[str],
        denial_reason: str,
        feedback_reason: str,
    ) -> CommandExecutionResult:
        try:
            await self.event_service.publish_command_event(
                EventType.OPERATOR_COMMAND_FAILED,
                CommandFailedBroadcastEvent(
                    command=command,
                    execution_id=execution_id,
                    operator_session_id=operator_session_id,
                    status=status or ExecutionStatus.FAILED,
                    error=error_msg,
                    stderr=feedback_reason or error_msg,
                    error_type=error_type,
                    denial_reason=denial_reason,
                    feedback_reason=feedback_reason,
                    rule=rule,
                    violations=violations,
                    approval_id=approval_id,
                ),
                g8e_context,
                task_id=AITaskId.COMMAND,
            )
        except Exception as e:
            logger.warning(
                "Failed to broadcast command event %s to client: %s",
                EventType.OPERATOR_COMMAND_FAILED,
                e,
            )
        return CommandExecutionResult(
            success=False,
            error=error_msg,
            error_type=error_type,
            execution_id=execution_id,
            rule=rule,
            denial_reason=denial_reason,
            feedback_reason=feedback_reason,
        )

    # -------------------------------------------------------------------------
    # Operator resolution
    # -------------------------------------------------------------------------

    def resolve_operators(
        self,
        operator_documents: list[OperatorDocument],
        target_operators: list[str],
    ) -> list[OperatorDocument]:
        """Resolve target_operators to a list of OperatorDocument.

        Supports sentinel values ('all', '*', 'fleet', 'every', 'everyone') for fleet-wide
        execution, and resolves individual targets by operator_id, hostname, or index.
        """
        if not operator_documents:
            raise BusinessLogicError("No operators bound to this session", component="g8ee")
        if not target_operators:
            raise ValidationError("target_operators list is empty", component="g8ee")

        # Single operator case: bypass resolution
        if len(operator_documents) == 1:
            return operator_documents

        # Lenient "all" handling: any sentinel in the list expands to the full fleet.
        # This rescues LLMs that pass e.g. ['all', 'web-1'] or ['*'] and makes whole-fleet
        # intent robust against enumeration mistakes.
        _fleet_sentinels = {"all", "*", "fleet", "every", "everyone"}
        if any(t.strip().lower() in _fleet_sentinels for t in target_operators):
            return operator_documents

        resolved: list[OperatorDocument] = []
        resolved_ids: set[str] = set()

        for target in target_operators:
            op = self._resolve_single_operator(operator_documents, target)
            if op and op.id and op.id not in resolved_ids:
                resolved.append(op)
                resolved_ids.add(op.id)

        if not resolved:
            raise ValidationError(
                "Could not resolve any operators from target_operators list",
                component="g8ee",
            )
        return resolved

    def _resolve_single_operator(
        self,
        operator_documents: list[OperatorDocument],
        target: str,
    ) -> OperatorDocument | None:
        """Resolve a single target string to an OperatorDocument.

        Matches by operator_id, hostname (case-insensitive), or numeric index.
        Returns None if no match found.
        """
        # Match by operator_id
        for op in operator_documents:
            if op.id == target:
                return op

        # Match by hostname (case-insensitive)
        for op in operator_documents:
            hostname = op.current_hostname or (
                op.latest_heartbeat_snapshot.system_identity.hostname
                if op.latest_heartbeat_snapshot
                else ""
            )
            if hostname and hostname.lower() == target.lower():
                return op

        # Match by index
        if target.isdigit():
            idx = int(target)
            if 0 <= idx < len(operator_documents):
                return operator_documents[idx]

        return None

    def build_target_systems_list(
        self, operator_documents: list[OperatorDocument]
    ) -> list[TargetSystem]:
        systems: list[TargetSystem] = []
        for op in operator_documents:
            hostname: str = (
                op.current_hostname
                or (
                    op.latest_heartbeat_snapshot.system_identity.hostname
                    if op.latest_heartbeat_snapshot
                    else None
                )
                or "None"
            )
            systems.append(
                TargetSystem(
                    operator_id=op.id,
                    hostname=hostname,
                    operator_type=op.operator_type,
                )
            )
        return systems

    # -------------------------------------------------------------------------
    # Generic Execution
    # -------------------------------------------------------------------------

    async def execute(
        self,
        g8e_message: G8eMessage,
        g8e_context: G8eHttpContext,
        timeout_seconds: int = 60,
    ) -> tuple[CommandInternalResult, G8eoResultEnvelope | None]:
        """Generic execution entry point for any G8eMessage."""
        return await self.dispatch_command(g8e_message, g8e_context, timeout_seconds)

    async def dispatch_command(
        self,
        g8e_message: G8eMessage,
        g8e_context: G8eHttpContext,
        timeout_seconds: int = 60,
    ) -> tuple[CommandInternalResult, G8eoResultEnvelope | None]:
        """Dispatch a governed operator command through the Gateway protocol surface."""
        payload = g8e_message.payload
        execution_id = getattr(payload, "execution_id", None) or g8e_message.id
        if not execution_id:
            raise ValidationError(
                "g8e_message must carry an execution_id (payload.execution_id or message.id)",
                component="g8ee",
            )
        operator_id = g8e_message.operator_id
        operator_session_id = g8e_message.operator_session_id

        if not operator_id or not operator_session_id:
            raise ValidationError(
                "operator_id and operator_session_id are required", component="g8ee"
            )

        if payload is None:
            raise ValidationError("g8e_message.payload is required", component="g8ee")

        if self._gateway_operator_client is None:
            return CommandInternalResult(
                execution_id=execution_id,
                status=ExecutionStatus.FAILED,
                error="Gateway operator client is not configured",
                error_type=CommandErrorType.PUBSUB_SUBSCRIPTION_NOT_READY,
            ), None

        payload_bytes = payload.to_protobuf().SerializeToString()
        target_resource = getattr(payload, "file_path", None) or getattr(payload, "path", None)

        try:
            dispatch_result = await asyncio.wait_for(
                self._gateway_operator_client.dispatch(
                    context=g8e_context,
                    operator_session_id=operator_session_id,
                    event_type=g8e_message.event_type,
                    payload=payload_bytes,
                    target_resource=target_resource,
                ),
                timeout=timeout_seconds,
            )
        except TimeoutError:
            return CommandInternalResult(
                execution_id=execution_id,
                status=ExecutionStatus.TIMEOUT,
                error=f"Execution timed out after {timeout_seconds}s",
                error_type=CommandErrorType.COMMAND_TIMEOUT,
            ), None
        except NetworkError as exc:
            return CommandInternalResult(
                execution_id=execution_id,
                status=ExecutionStatus.FAILED,
                error=str(exc),
                error_type=CommandErrorType.NO_OPERATORS_AVAILABLE,
            ), None

        if not dispatch_result.get("success", True):
            return CommandInternalResult(
                execution_id=execution_id,
                status=ExecutionStatus.FAILED,
                error=str(dispatch_result.get("error") or "Gateway dispatch failed"),
                error_type=CommandErrorType.EXECUTION_FAILED,
            ), None

        envelope = envelope_from_gateway_dispatch(
            dispatch_result,
            execution_id=execution_id,
            operator_id=operator_id,
            operator_session_id=operator_session_id,
            g8e_context=g8e_context,
        )

        if envelope is None or not isinstance(envelope.payload, ExecutionResultsPayload):
            return CommandInternalResult(
                execution_id=execution_id,
                status=ExecutionStatus.COMPLETED,
                output="",
                operator_id=operator_id,
            ), envelope

        result_payload = envelope.payload
        status = result_payload.status if result_payload.status else ExecutionStatus.COMPLETED
        return CommandInternalResult(
            execution_id=execution_id,
            status=status,
            output=result_payload.stdout or "",
            stderr=result_payload.stderr or "",
            error=result_payload.error_message or "",
            exit_code=result_payload.return_code,
            execution_time_seconds=result_payload.duration_seconds or 0,
            operator_id=operator_id,
            completed_at=result_payload.completed_at,
        ), envelope

    async def cancel_command(
        self,
        execution_id: str,
        operator_id: str,
        operator_session_id: str,
        g8e_context: G8eHttpContext,
    ) -> CancelCommandResult:
        if self._gateway_operator_client is None:
            return CancelCommandResult(
                execution_id=execution_id,
                status=ExecutionStatus.FAILED,
                error="Gateway operator client is not configured",
            )

        cancel_payload = CommandCancelRequestPayload(execution_id=execution_id)
        try:
            dispatch_result = await self._gateway_operator_client.dispatch(
                context=g8e_context,
                operator_session_id=operator_session_id,
                event_type=EventType.OPERATOR_COMMAND_CANCEL_REQUESTED,
                payload=cancel_payload.to_protobuf().SerializeToString(),
            )
        except NetworkError as exc:
            logger.error("[EXECUTION] Cancel command failed: %s", exc, exc_info=True)
            return CancelCommandResult(
                execution_id=execution_id,
                status=ExecutionStatus.FAILED,
                error=f"Command cancellation failed: {exc}. Check operator status and retry.",
            )
        except Exception as e:
            logger.error("[EXECUTION] Cancel command failed: %s", e, exc_info=True)
            return CancelCommandResult(
                execution_id=execution_id,
                status=ExecutionStatus.FAILED,
                error=f"Command cancellation failed: {e}. Check operator status and retry.",
            )

        if not dispatch_result.get("success", True):
            return CancelCommandResult(
                execution_id=execution_id,
                status=ExecutionStatus.FAILED,
                error=str(dispatch_result.get("error") or "Gateway cancel dispatch failed"),
            )

        return CancelCommandResult(execution_id=execution_id, status=ExecutionStatus.CANCELLED)

    async def send_command_to_operator(
        self,
        command_payload: DirectCommandRequest,
        g8e_context: G8eHttpContext,
    ) -> DirectCommandResult:
        if not g8e_context.bound_operators:
            raise ValidationError("No bound operators", component="g8ee")
        bound = g8e_context.bound_operators[0]
        if not bound.operator_session_id:
            raise ValidationError("Operator not bound", component="g8ee")

        if self._gateway_operator_client is None:
            return DirectCommandResult(
                execution_id=command_payload.execution_id,
                status=ExecutionStatus.FAILED,
                error="Gateway operator client is not configured",
            )

        execution_id = command_payload.execution_id
        operator_id = bound.operator_id
        operator_session_id = bound.operator_session_id

        _t = asyncio.create_task(
            self._dispatch_direct_command_and_broadcast(
                command_payload=command_payload,
                g8e_context=g8e_context,
                operator_id=operator_id,
                operator_session_id=operator_session_id,
            )
        )
        self._background_tasks.add(_t)
        _t.add_done_callback(self._background_tasks.discard)

        return DirectCommandResult(execution_id=execution_id, status=ExecutionStatus.EXECUTING)

    async def _dispatch_direct_command_and_broadcast(
        self,
        command_payload: DirectCommandRequest,
        g8e_context: G8eHttpContext,
        operator_id: str,
        operator_session_id: str,
    ) -> None:
        """Dispatch a direct terminal command through Gateway and broadcast the result."""
        execution_id = command_payload.execution_id
        command = command_payload.command
        timeout_seconds = 300
        request_payload = CommandRequestPayload(command=command, execution_id=execution_id)

        try:
            dispatch_result = await asyncio.wait_for(
                self._gateway_operator_client.dispatch(
                    context=g8e_context,
                    operator_session_id=operator_session_id,
                    event_type=EventType.OPERATOR_COMMAND_REQUESTED,
                    payload=request_payload.to_protobuf().SerializeToString(),
                ),
                timeout=timeout_seconds,
            )
        except TimeoutError:
            logger.warning(
                "[EXECUTION] Direct command timed out waiting for Gateway result for %s",
                execution_id,
            )
            await self._broadcast_direct_command_failure(
                execution_id=execution_id,
                command=command,
                g8e_context=g8e_context,
                operator_id=operator_id,
                operator_session_id=operator_session_id,
                hostname=command_payload.hostname,
                error=f"Execution timed out after {timeout_seconds}s",
            )
            return
        except NetworkError as exc:
            logger.error(
                "[EXECUTION] Direct command Gateway dispatch failed for %s: %s",
                execution_id,
                exc,
                exc_info=True,
            )
            await self._broadcast_direct_command_failure(
                execution_id=execution_id,
                command=command,
                g8e_context=g8e_context,
                operator_id=operator_id,
                operator_session_id=operator_session_id,
                hostname=command_payload.hostname,
                error=str(exc),
            )
            return
        except Exception as e:
            logger.error(
                "[EXECUTION] Direct command dispatch failed for %s: %s",
                execution_id,
                e,
                exc_info=True,
            )
            await self._broadcast_direct_command_failure(
                execution_id=execution_id,
                command=command,
                g8e_context=g8e_context,
                operator_id=operator_id,
                operator_session_id=operator_session_id,
                hostname=command_payload.hostname,
                error=str(e),
            )
            return

        if not dispatch_result.get("success", True):
            await self._broadcast_direct_command_failure(
                execution_id=execution_id,
                command=command,
                g8e_context=g8e_context,
                operator_id=operator_id,
                operator_session_id=operator_session_id,
                hostname=command_payload.hostname,
                error=str(dispatch_result.get("error") or "Gateway dispatch failed"),
            )
            return

        envelope = envelope_from_gateway_dispatch(
            dispatch_result,
            execution_id=execution_id,
            operator_id=operator_id,
            operator_session_id=operator_session_id,
            g8e_context=g8e_context,
        )
        await self._broadcast_direct_command_envelope(
            execution_id=execution_id,
            command=command,
            envelope=envelope,
            g8e_context=g8e_context,
            operator_id=operator_id,
            operator_session_id=operator_session_id,
            hostname=command_payload.hostname,
        )

    async def _broadcast_direct_command_envelope(
        self,
        execution_id: str,
        command: str,
        envelope: G8eoResultEnvelope | None,
        g8e_context: G8eHttpContext,
        operator_id: str,
        operator_session_id: str,
        hostname: str | None = None,
    ) -> None:
        """Broadcast a direct command result envelope to the client."""
        if envelope is None or not isinstance(envelope.payload, ExecutionResultsPayload):
            logger.warning(
                "[EXECUTION] Direct command result payload type mismatch for %s", execution_id
            )
            return

        payload = envelope.payload
        status = payload.status if payload.status else ExecutionStatus.COMPLETED

        completion_event_type = (
            EventType.OPERATOR_COMMAND_COMPLETED
            if status == ExecutionStatus.COMPLETED
            else EventType.OPERATOR_COMMAND_FAILED
        )

        await self.event_service.publish_command_event(
            completion_event_type,
            CommandResultBroadcastEvent(
                execution_id=execution_id,
                command=command,
                status=status,
                output=payload.stdout,
                error=payload.error_message,
                stderr=payload.stderr,
                exit_code=payload.return_code,
                execution_time_seconds=payload.duration_seconds or 0,
                operator_id=operator_id,
                operator_session_id=operator_session_id,
                hostname=hostname,
                direct_execution=True,
            ),
            g8e_context,
            task_id=AITaskId.DIRECT_COMMAND,
        )

        logger.info("[EXECUTION] Direct command result broadcasted for %s", execution_id)

    async def _broadcast_direct_command_failure(
        self,
        execution_id: str,
        command: str,
        g8e_context: G8eHttpContext,
        operator_id: str,
        operator_session_id: str,
        hostname: str | None,
        error: str,
    ) -> None:
        await self.event_service.publish_command_event(
            EventType.OPERATOR_COMMAND_FAILED,
            CommandResultBroadcastEvent(
                execution_id=execution_id,
                command=command,
                status=ExecutionStatus.FAILED,
                error=error,
                stderr=error,
                operator_id=operator_id,
                operator_session_id=operator_session_id,
                hostname=hostname,
                direct_execution=True,
            ),
            g8e_context,
            task_id=AITaskId.DIRECT_COMMAND,
        )
