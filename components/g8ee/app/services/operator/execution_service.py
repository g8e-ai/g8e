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

"""Operator Execution Service

Handles g8eo execution dispatch, validation, security checks, operator resolution,
and result assembly.
"""

import asyncio
import logging

from app.constants.events import EventType
from app.constants.status import (
    AITaskId,
    CommandErrorType,
    ComponentName,
    ExecutionStatus,
)
from app.errors import BusinessLogicError, ValidationError
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
    DirectCommandResult,
    OperatorDocument,
    TargetSystem,
)
from app.models.pubsub_messages import G8eMessage, G8eoResultEnvelope, ExecutionResultsPayload
from app.models.tool_results import CommandInternalResult
from app.models.http_context import G8eHttpContext
from app.models.settings import G8eePlatformSettings

logger = logging.getLogger(__name__)


class OperatorExecutionService(ExecutionServiceProtocol):
    """Handles execution dispatch, validation, and result assembly."""

    def __init__(
        self,
        pubsub_service: PubSubServiceProtocol,
        approval_service: ApprovalServiceProtocol,
        g8ed_event_service: EventServiceProtocol,
        settings: G8eePlatformSettings,
        ai_response_analyzer: AIResponseAnalyzerProtocol,
        operator_data_service: OperatorDataServiceProtocol,
        investigation_service: InvestigationServiceProtocol,
    ) -> None:
        self._pubsub_service = pubsub_service
        self._approval_service = approval_service
        self._g8ed_event_service = g8ed_event_service
        self._settings = settings
        self._operator_data_service = operator_data_service
        self._ai_response_analyzer = ai_response_analyzer
        self._investigation_service = investigation_service

        from app.security.operator_command_validator import OperatorCommandValidator
        from app.utils.validators import get_blacklist_validator, get_whitelist_validator
        self.command_validator = OperatorCommandValidator(operator_data_service)
        self.whitelist_validator = get_whitelist_validator()
        self.blacklist_validator = get_blacklist_validator()

    @property
    def pubsub_service(self) -> PubSubServiceProtocol:
        return self._pubsub_service

    @property
    def approval_service(self) -> ApprovalServiceProtocol:
        return self._approval_service

    @property
    def g8ed_event_service(self) -> EventServiceProtocol:
        return self._g8ed_event_service

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
            await self.g8ed_event_service.publish_command_event(
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
            logger.warning("Failed to broadcast command event %s to g8ed: %s", EventType.OPERATOR_COMMAND_FAILED, e)
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

    def resolve_target_operator(
        self,
        operator_documents: list[OperatorDocument],
        target_operator: str | None,
        tool_name: str | None = None,
    ) -> OperatorDocument:
        if not operator_documents:
            raise BusinessLogicError("No operators bound to this session", component="g8ee")

        if len(operator_documents) == 1:
            return operator_documents[0]

        if not target_operator:
            available = [
                f"  [{i}] {op.current_hostname or (op.system_info.hostname if op.system_info else 'None')} "
                f"({op.id}) - {op.operator_type}"
                for i, op in enumerate(operator_documents)
            ]
            error_msg = (
                f"Multiple operators ({len(operator_documents)}) are bound to this session. "
            )
            
            if tool_name and "command" in tool_name.lower():
                error_msg += (
                    f"You MUST specify either target_operator (single host: operator_id, hostname, or index) "
                    f"or target_operators (list of hosts for batch execution under one approval).\n"
                )
            else:
                error_msg += (
                    f"This tool only supports single-target execution. "
                    f"You MUST specify target_operator (operator_id or hostname).\n"
                    f"Note: target_operators (batch execution) is only supported by run_commands_with_operator.\n"
                )
            
            error_msg += f"Available operators:\n" + "\n".join(available)
            
            raise ValidationError(
                error_msg,
                component="g8ee",
            )

        for op in operator_documents:
            if op.id == target_operator:
                return op

        for op in operator_documents:
            hostname = op.current_hostname or (op.system_info.hostname if op.system_info else "")
            if hostname and hostname.lower() == target_operator.lower():
                return op

        if target_operator.isdigit():
            idx = int(target_operator)
            if 0 <= idx < len(operator_documents):
                return operator_documents[idx]
            raise ValidationError(
                f"Operator index {idx} out of range. Valid indices: 0-{len(operator_documents)-1}",
                component="g8ee",
            )

        available = [
            f"  [{i}] {op.current_hostname or (op.system_info.hostname if op.system_info else 'None')} ({op.id})"
            for i, op in enumerate(operator_documents)
        ]
        raise ValidationError(
            f"Could not resolve target_operator '{target_operator}'. "
            f"No match found by operator_id, hostname, or index.\n"
            f"Available operators:\n" + "\n".join(available),
            component="g8ee",
        )

    def resolve_multiple_operators(
        self,
        operator_documents: list[OperatorDocument],
        target_operators: list[str],
    ) -> list[OperatorDocument]:
        if not operator_documents:
            raise BusinessLogicError("No operators bound to this session", component="g8ee")
        if not target_operators:
            raise ValidationError("target_operators list is empty", component="g8ee")

        # Lenient "all" handling: any sentinel in the list expands to the full fleet.
        # This rescues LLMs that pass e.g. ['all', 'web-1'] or ['*'] and makes whole-fleet
        # intent robust against enumeration mistakes.
        _fleet_sentinels = {"all", "*", "fleet", "every", "everyone"}
        if any(t.strip().lower() in _fleet_sentinels for t in target_operators):
            return operator_documents

        resolved: list[OperatorDocument] = []
        resolved_ids: set[str] = set()

        for target in target_operators:
            try:
                op = self.resolve_target_operator(
                    operator_documents=operator_documents,
                    target_operator=target,
                )
                if op.id and op.id not in resolved_ids:
                    resolved.append(op)
                    resolved_ids.add(op.id)
            except (ValidationError, BusinessLogicError):
                continue

        if not resolved:
            raise ValidationError(
                "Could not resolve any operators from target_operators list",
                component="g8ee",
            )
        return resolved

    def build_target_systems_list(self, operator_documents: list[OperatorDocument]) -> list[TargetSystem]:
        systems: list[TargetSystem] = []
        for op in operator_documents:
            hostname: str = op.current_hostname or (op.system_info.hostname if op.system_info else None) or "None"
            systems.append(TargetSystem(
                operator_id=op.id,
                hostname=hostname,
                operator_type=op.operator_type,
            ))
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
        """Authors authoritative execution for any G8eMessage."""
        # Correlation key: always the per-message execution_id that g8eo echoes back
        # in the result payload. g8e_context.execution_id is the HTTP request id and
        # is NOT the same as the per-operator/per-message execution_id that the
        # pubsub result dispatcher uses to complete Futures. See the docstring on
        # G8eoResultEnvelope.id in app/models/pubsub_messages.py.
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
            raise ValidationError("operator_id and operator_session_id are required", component="g8ee")

        if not self.pubsub_service.is_ready:
            return CommandInternalResult(
                execution_id=execution_id,
                status=ExecutionStatus.FAILED,
                error="Pub/sub pattern subscription not ready",
                error_type=CommandErrorType.PUBSUB_SUBSCRIPTION_NOT_READY,
            ), None

        future = self.pubsub_service.register_future(execution_id)
        await self.pubsub_service.register_operator_session(operator_id, operator_session_id)

        subscribers = await self.pubsub_service.publish_command(
            operator_id=operator_id,
            operator_session_id=operator_session_id,
            command_data=g8e_message,
        )

        if subscribers == 0:
            channel = f"cmd:{operator_id}:{operator_session_id}"
            error_msg = f"No Operator listening on command channel '{channel}'"
            logger.error("[NO-SUBSCRIBERS] %s", error_msg, extra={"operator_id": operator_id, "operator_session_id": operator_session_id, "channel": channel})
            self.pubsub_service.release_future(execution_id)
            return CommandInternalResult(
                execution_id=execution_id,
                status=ExecutionStatus.FAILED,
                error=error_msg,
                error_type=CommandErrorType.NO_OPERATORS_AVAILABLE,
            ), None

        try:
            envelope = await asyncio.wait_for(future, timeout=timeout_seconds)
        except (asyncio.TimeoutError, TimeoutError):
            self.pubsub_service.release_future(execution_id)
            return CommandInternalResult(
                execution_id=execution_id,
                status=ExecutionStatus.TIMEOUT,
                error=f"Execution timed out after {timeout_seconds}s",
                error_type=CommandErrorType.COMMAND_TIMEOUT,
            ), None

        if not isinstance(envelope.payload, ExecutionResultsPayload):
            self.pubsub_service.release_future(execution_id)
            return CommandInternalResult(
                execution_id=execution_id,
                status=ExecutionStatus.COMPLETED,
                output="",
            ), envelope

        payload = envelope.payload
        status = payload.status if payload.status else ExecutionStatus.COMPLETED

        self.pubsub_service.release_future(execution_id)
        return CommandInternalResult(
            execution_id=execution_id,
            status=status,
            output=payload.stdout or "",
            stderr=payload.stderr or "",
            error=payload.error_message or "",
            exit_code=payload.return_code,
            execution_time_seconds=payload.duration_seconds or 0,
            operator_id=envelope.operator_id,
            completed_at=payload.completed_at,
        ), envelope

    async def cancel_command(
        self,
        execution_id: str,
        operator_id: str,
        operator_session_id: str,
        g8e_context: G8eHttpContext,
    ) -> CancelCommandResult:
        try:
            cancel_data = G8eMessage(
                id=f"cancel_{execution_id}",
                source_component=ComponentName.G8EE,
                event_type=EventType.OPERATOR_COMMAND_CANCEL_REQUESTED,
                case_id=g8e_context.case_id,
                task_id=AITaskId.COMMAND,
                operator_session_id=operator_session_id,
                operator_id=operator_id,
                investigation_id=g8e_context.investigation_id,
                web_session_id=g8e_context.web_session_id,
                payload=CommandCancelRequestPayload(execution_id=execution_id),
            )

            await self.pubsub_service.publish_command(
                operator_id=operator_id,
                operator_session_id=operator_session_id,
                command_data=cancel_data,
            )
            return CancelCommandResult(execution_id=execution_id, status=ExecutionStatus.CANCELLED)
        except Exception as e:
            logger.error("[EXECUTION] Cancel command failed: %s", e, exc_info=True)
            return CancelCommandResult(execution_id=execution_id, status=ExecutionStatus.FAILED, error=f"Command cancellation failed: {e}. Check operator status and retry.")

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

        execution_id = command_payload.execution_id
        command = command_payload.command
        operator_id = bound.operator_id
        operator_session_id = bound.operator_session_id

        command_data = G8eMessage(
            id=execution_id,
            source_component=ComponentName.G8EE,
            event_type=EventType.OPERATOR_COMMAND_REQUESTED,
            case_id=g8e_context.case_id,
            task_id=AITaskId.DIRECT_COMMAND,
            investigation_id=g8e_context.investigation_id,
            web_session_id=g8e_context.web_session_id,
            operator_session_id=operator_session_id,
            operator_id=operator_id,
            payload=CommandRequestPayload(
                command=command,
                execution_id=execution_id,
            ),
        )

        subscribers = await self.pubsub_service.publish_command(
            operator_id=operator_id,
            operator_session_id=operator_session_id,
            command_data=command_data,
        )

        if subscribers == 0:
            channel = f"cmd:{operator_id}:{operator_session_id}"
            logger.error("[NO-SUBSCRIBERS] No Operator listening on command channel '%s'", channel, extra={"operator_id": operator_id, "operator_session_id": operator_session_id, "channel": channel})
            return DirectCommandResult(execution_id=execution_id, status=ExecutionStatus.FAILED, error="No Operator listening")

        return DirectCommandResult(execution_id=execution_id, status=ExecutionStatus.EXECUTING)

