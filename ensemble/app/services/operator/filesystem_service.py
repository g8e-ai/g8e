# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Operator Filesystem Service

Handles non-mutating filesystem operations (list, read) on Operators.
"""

import logging

from app.services.protocols import (
    ExecutionServiceProtocol,
    InvestigationServiceProtocol,
    PubSubServiceProtocol,
)
from app.constants import EventType, G8EE_COMPONENT
from app.constants.generated_status import AITaskId
from app.constants.config import ExecutionStatus
from app.models.http_context import G8eHttpContext
from app.models.command_request_payloads import (
    FsListRequestPayload,
    FsReadRequestPayload,
    FsGrepRequestPayload,
)
from app.models.operators import CommandExecutingBroadcastEvent, CommandResultBroadcastEvent
from app.models.investigations import EnrichedInvestigationContext
from app.models.tool_results import FsListToolResult, FsReadToolResult, FsGrepToolResult
from app.models.pubsub_messages import (
    FsListResultPayload,
    FsReadResultPayload,
    FsGrepResultPayload,
    G8eMessage,
)

logger = logging.getLogger(__name__)


class OperatorFilesystemService:
    """Handles filesystem list and read operations on Operators."""

    def __init__(
        self,
        pubsub_service: PubSubServiceProtocol,
        execution_service: ExecutionServiceProtocol,
        investigation_service: InvestigationServiceProtocol,
    ) -> None:
        self._pubsub_service = pubsub_service
        self._execution_service = execution_service
        self._investigation_service = investigation_service

    @property
    def pubsub_service(self) -> PubSubServiceProtocol:
        return self._pubsub_service

    @property
    def execution_service(self) -> ExecutionServiceProtocol:
        return self._execution_service

    @property
    def investigation_service(self) -> InvestigationServiceProtocol:
        return self._investigation_service

    async def execute_fs_list(
        self,
        args: FsListRequestPayload,
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
    ) -> FsListToolResult:
        """List files on an operator.

        ``execution_id`` is extracted from args.execution_id and
        is used as the registry key and in UI lifecycle events.
        """
        exec_id = args.execution_id
        operator_documents = investigation.operator_documents if investigation else []
        resolved_operators = self.execution_service.resolve_operators(
            operator_documents=operator_documents,
            target_operators=args.target_operators,
        )
        resolved_operator = resolved_operators[0]

        g8e_message = G8eMessage(
            id=exec_id,
            source_component=G8EE_COMPONENT,
            event_type=EventType.OPERATOR_FILESYSTEM_LIST_REQUESTED,
            case_id=g8e_context.case_id,
            task_id=AITaskId.FS_LIST,
            investigation_id=g8e_context.investigation_id,
            web_session_id=g8e_context.web_session_id,
            operator_session_id=resolved_operator.operator_session_id,
            operator_id=resolved_operator.id,
            payload=args,
        )

        # Notify start
        await self.execution_service.event_service.publish_command_event(
            EventType.OPERATOR_FILESYSTEM_LIST_STARTED,
            CommandExecutingBroadcastEvent(
                command=f"ls {args.path}",
                execution_id=exec_id,
                operator_session_id=resolved_operator.operator_session_id,
            ),
            g8e_context,
            task_id=AITaskId.FS_LIST,
        )

        internal_result, envelope = await self.execution_service.execute(
            g8e_message=g8e_message,
            g8e_context=g8e_context,
            timeout_seconds=60,
        )

        # Extract typed payload data from envelope
        entries = []
        if envelope and isinstance(envelope.payload, FsListResultPayload):
            entries = envelope.payload.entries or []

        # Notify completion/failure
        status = internal_result.status
        output = internal_result.output
        error = internal_result.error

        completion_event_type = (
            EventType.OPERATOR_FILESYSTEM_LIST_COMPLETED
            if status == ExecutionStatus.COMPLETED
            else EventType.OPERATOR_FILESYSTEM_LIST_FAILED
        )

        await self.execution_service.event_service.publish_command_event(
            completion_event_type,
            CommandResultBroadcastEvent(
                execution_id=exec_id,
                command=f"ls {args.path}",
                status=status,
                output=output,
                error=error,
                operator_id=resolved_operator.id,
                operator_session_id=resolved_operator.operator_session_id,
            ),
            g8e_context,
            task_id=AITaskId.FS_LIST,
        )

        return FsListToolResult(
            success=status == ExecutionStatus.COMPLETED,
            path=args.path,
            entries=entries,
            error=error,
        )

    async def execute_fs_grep(
        self,
        args: FsGrepRequestPayload,
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
    ) -> FsGrepToolResult:
        """Search for a pattern on an operator.

        ``execution_id`` is extracted from args.execution_id and
        is used as the registry key and in UI lifecycle events.
        """
        exec_id = args.execution_id
        operator_documents = investigation.operator_documents if investigation else []
        resolved_operators = self.execution_service.resolve_operators(
            operator_documents=operator_documents,
            target_operators=args.target_operators,
        )
        resolved_operator = resolved_operators[0]

        g8e_message = G8eMessage(
            id=exec_id,
            source_component=G8EE_COMPONENT,
            event_type=EventType.OPERATOR_FILESYSTEM_GREP_REQUESTED,
            case_id=g8e_context.case_id,
            task_id=AITaskId.RECURSIVE_GREP,
            investigation_id=g8e_context.investigation_id,
            web_session_id=g8e_context.web_session_id,
            operator_session_id=resolved_operator.operator_session_id,
            operator_id=resolved_operator.id,
            payload=args,
        )

        # Notify start
        await self.execution_service.event_service.publish_command_event(
            EventType.OPERATOR_FILESYSTEM_GREP_STARTED,
            CommandExecutingBroadcastEvent(
                command=f"grep -r {args.pattern} {args.path}",
                execution_id=exec_id,
                operator_session_id=resolved_operator.operator_session_id,
            ),
            g8e_context,
            task_id=AITaskId.RECURSIVE_GREP,
        )

        internal_result, envelope = await self.execution_service.execute(
            g8e_message=g8e_message,
            g8e_context=g8e_context,
            timeout_seconds=60,
        )

        # Extract typed payload data from envelope
        matches = []
        total_matches = 0
        truncated = False
        if envelope and isinstance(envelope.payload, FsGrepResultPayload):
            matches = envelope.payload.matches or []
            total_matches = envelope.payload.total_matches
            truncated = envelope.payload.truncated

        # Notify completion/failure
        status = internal_result.status
        output = internal_result.output
        error = internal_result.error

        completion_event_type = (
            EventType.OPERATOR_FILESYSTEM_GREP_COMPLETED
            if status == ExecutionStatus.COMPLETED
            else EventType.OPERATOR_FILESYSTEM_GREP_FAILED
        )

        await self.execution_service.event_service.publish_command_event(
            completion_event_type,
            CommandResultBroadcastEvent(
                execution_id=exec_id,
                command=f"grep -r {args.pattern} {args.path}",
                status=status,
                output=output,
                error=error,
                operator_id=resolved_operator.id,
                operator_session_id=resolved_operator.operator_session_id,
            ),
            g8e_context,
            task_id=AITaskId.RECURSIVE_GREP,
        )

        return FsGrepToolResult(
            success=status == ExecutionStatus.COMPLETED,
            path=args.path,
            pattern=args.pattern,
            matches=matches,
            total_matches=total_matches,
            truncated=truncated,
            error=error,
        )

    async def execute_file_read(
        self,
        args: FsReadRequestPayload,
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
    ) -> FsReadToolResult:
        """Read a file from an operator.

        ``execution_id`` is extracted from args.execution_id and
        is used as the registry key and in UI lifecycle events.
        """
        exec_id = args.execution_id
        operator_documents = investigation.operator_documents if investigation else []
        resolved_operators = self.execution_service.resolve_operators(
            operator_documents=operator_documents,
            target_operators=args.target_operators,
        )
        resolved_operator = resolved_operators[0]

        g8e_message = G8eMessage(
            id=exec_id,
            source_component=G8EE_COMPONENT,
            event_type=EventType.OPERATOR_FILESYSTEM_READ_REQUESTED,
            case_id=g8e_context.case_id,
            task_id=AITaskId.FS_READ,
            investigation_id=g8e_context.investigation_id,
            web_session_id=g8e_context.web_session_id,
            operator_session_id=resolved_operator.operator_session_id,
            operator_id=resolved_operator.id,
            payload=args,
        )

        # Notify start
        await self.execution_service.event_service.publish_command_event(
            EventType.OPERATOR_FILESYSTEM_READ_STARTED,
            CommandExecutingBroadcastEvent(
                command=f"cat {args.path}",
                execution_id=exec_id,
                operator_session_id=resolved_operator.operator_session_id,
            ),
            g8e_context,
            task_id=AITaskId.FS_READ,
        )

        internal_result, envelope = await self.execution_service.execute(
            g8e_message=g8e_message,
            g8e_context=g8e_context,
            timeout_seconds=60,
        )

        # Extract typed payload data from envelope
        content = None
        if envelope and isinstance(envelope.payload, FsReadResultPayload):
            content = envelope.payload.content

        # Notify completion/failure
        status = internal_result.status
        output = internal_result.output
        error = internal_result.error

        completion_event_type = (
            EventType.OPERATOR_FILESYSTEM_READ_COMPLETED
            if status == ExecutionStatus.COMPLETED
            else EventType.OPERATOR_FILESYSTEM_READ_FAILED
        )

        await self.execution_service.event_service.publish_command_event(
            completion_event_type,
            CommandResultBroadcastEvent(
                execution_id=exec_id,
                command=f"cat {args.path}",
                status=status,
                output=output,
                error=error,
                operator_id=resolved_operator.id,
                operator_session_id=resolved_operator.operator_session_id,
            ),
            g8e_context,
            task_id=AITaskId.FS_READ,
        )

        return FsReadToolResult(
            success=status == ExecutionStatus.COMPLETED,
            path=args.path,
            content=content,
            error=error,
        )
