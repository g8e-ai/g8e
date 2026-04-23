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

"""Operator Filesystem Service

Handles non-mutating filesystem operations (list, read) on Operators.
"""

import logging

from app.services.protocols import (
    ExecutionRegistryProtocol,
    ExecutionServiceProtocol,
    InvestigationServiceProtocol,
    PubSubServiceProtocol,
)
from app.constants.status import (
    ComponentName,
    ExecutionStatus,
    AITaskId,
)
from app.constants.events import (
    EventType,
)
from app.models.http_context import G8eHttpContext
from app.models.command_request_payloads import FsListRequestPayload, FsReadRequestPayload
from app.models.operators import CommandExecutingBroadcastEvent, CommandResultBroadcastEvent
from app.models.investigations import EnrichedInvestigationContext
from app.models.tool_results import FsListToolResult, FsReadToolResult
from app.models.pubsub_messages import G8eMessage

logger = logging.getLogger(__name__)


class OperatorFilesystemService:
    """Handles filesystem list and read operations on Operators."""

    def __init__(
        self,
        pubsub_service: PubSubServiceProtocol,
        execution_registry: ExecutionRegistryProtocol,
        execution_service: ExecutionServiceProtocol,
        investigation_service: InvestigationServiceProtocol,
    ) -> None:
        self.pubsub_service = pubsub_service
        self.execution_registry = execution_registry
        self.execution_service = execution_service
        self.investigation_service = investigation_service

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
        resolved_operator = self.execution_service.resolve_target_operator(
            operator_documents=operator_documents,
            target_operator=args.target_operator,
        )

        self.execution_registry.allocate(exec_id)

        try:
            g8e_message = G8eMessage(
                id=exec_id,
                source_component=ComponentName.G8EE,
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
            await self.execution_service.g8ed_event_service.publish_command_event(
                EventType.OPERATOR_FILESYSTEM_LIST_STARTED,
                CommandExecutingBroadcastEvent(
                    command=f"ls {args.path}",
                    execution_id=exec_id,
                    operator_session_id=resolved_operator.operator_session_id,
                ),
                g8e_context,
                task_id=AITaskId.FS_LIST,
            )

            internal_result = await self.execution_service.execute(
                g8e_message=g8e_message,
                g8e_context=g8e_context,
                timeout_seconds=60,
            )

            # Extract typed payload data from execution registry
            from app.models.pubsub_messages import FsListResultPayload
            envelope = self.execution_registry.get_result(exec_id)
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

            await self.execution_service.g8ed_event_service.publish_command_event(
                completion_event_type,
                CommandResultBroadcastEvent(
                    execution_id=exec_id,
                    command=f"ls {args.path}",
                    status=status if status is not None else ExecutionStatus.FAILED,
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
        finally:
            self.execution_registry.release(exec_id)

    async def execute_fs_read(
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
        resolved_operator = self.execution_service.resolve_target_operator(
            operator_documents=operator_documents,
            target_operator=args.target_operator,
        )

        self.execution_registry.allocate(exec_id)

        try:
            g8e_message = G8eMessage(
                id=exec_id,
                source_component=ComponentName.G8EE,
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
            await self.execution_service.g8ed_event_service.publish_command_event(
                EventType.OPERATOR_FILESYSTEM_READ_STARTED,
                CommandExecutingBroadcastEvent(
                    command=f"cat {args.path}",
                    execution_id=exec_id,
                    operator_session_id=resolved_operator.operator_session_id,
                ),
                g8e_context,
                task_id=AITaskId.FS_READ,
            )

            internal_result = await self.execution_service.execute(
                g8e_message=g8e_message,
                g8e_context=g8e_context,
                timeout_seconds=60,
            )

            # Extract typed payload data from execution registry
            from app.models.pubsub_messages import FsReadResultPayload
            envelope = self.execution_registry.get_result(exec_id)
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

            await self.execution_service.g8ed_event_service.publish_command_event(
                completion_event_type,
                CommandResultBroadcastEvent(
                    execution_id=exec_id,
                    command=f"cat {args.path}",
                    status=status if status is not None else ExecutionStatus.FAILED,
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
        finally:
            self.execution_registry.release(exec_id)
