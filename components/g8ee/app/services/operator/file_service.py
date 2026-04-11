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

"""Operator File Service

Handles file-related operations (edit, read, write, update) on Operators.
"""

import logging

from app.services.protocols import (
    ApprovalServiceProtocol,
    EventServiceProtocol,
    ExecutionRegistryProtocol,
    ExecutionServiceProtocol,
    AIResponseAnalyzerProtocol,
    InvestigationServiceProtocol,
    PubSubServiceProtocol,
)
from app.constants.status import (
    AITaskId,
    CommandErrorType,
    ComponentName,
    ExecutionStatus,
    FileOperation,
    OperatorToolName,
)
from app.services.mcp.adapter import build_tool_call_request
from app.constants.events import (
    EventType,
)
from app.constants.settings import (
    ApprovalErrorType,
)
from app.models.command_payloads import FileEditPayload, FetchFileHistoryArgs, FetchFileDiffArgs
from app.models.http_context import VSOHttpContext
from app.models.investigations import EnrichedInvestigationContext
from app.models.tool_results import FileEditResult, FileOperationRiskAnalysis, FetchFileHistoryToolResult, FetchFileDiffToolResult
from app.models.operators import FileEditApprovalRequest, OperatorDocument, CommandFailedBroadcastEvent, FileEditBroadcastEvent, CommandExecutingBroadcastEvent, CommandResultBroadcastEvent
from app.models.pubsub_messages import VSOMessage
from app.utils.ids import generate_command_execution_id
from app.utils.timestamp import now

logger = logging.getLogger(__name__)


class OperatorFileService:
    """Handles file operations on Operators with risk analysis and approval."""

    def __init__(
        self,
        pubsub_service: PubSubServiceProtocol,
        execution_registry: ExecutionRegistryProtocol,
        approval_service: ApprovalServiceProtocol,
        vsod_event_service: EventServiceProtocol,
        execution_service: ExecutionServiceProtocol,
        ai_response_analyzer: AIResponseAnalyzerProtocol,
        investigation_service: InvestigationServiceProtocol,
    ) -> None:
        self.pubsub_service = pubsub_service
        self.execution_registry = execution_registry
        self.approval_service = approval_service
        self.vsod_event_service = vsod_event_service
        self.execution_service = execution_service
        self.ai_response_analyzer = ai_response_analyzer
        self.investigation_service = investigation_service

    async def execute_file_edit(
        self,
        args: FileEditPayload,
        vso_context: VSOHttpContext,
        investigation: EnrichedInvestigationContext,
        execution_id: str,
    ) -> FileEditResult:
        """Orchestrate file operation: resolution -> risk -> approval -> execution."""
        try:
            file_path = args.file_path
            operation = args.operation
            justification = args.justification.strip() if args.justification else ""
            
            op_name = getattr(operation, "value", operation)
            logger.info("[FILE] Starting %s on %s", op_name, file_path)

            # 1. Validation
            if not file_path or not file_path.strip():
                return FileEditResult(success=False, error="File path parameter is required", error_type=CommandErrorType.VALIDATION_ERROR)
            
            if not justification:
                return FileEditResult(success=False, error="Justification parameter is required", error_type=CommandErrorType.VALIDATION_ERROR)

            if operation == FileOperation.REPLACE:
                if not args.old_content:
                    return FileEditResult(success=False, error="old_content is required for replace operation", error_type=CommandErrorType.VALIDATION_ERROR)
                if not args.new_content:
                    return FileEditResult(success=False, error="new_content is required for replace operation", error_type=CommandErrorType.VALIDATION_ERROR)
            elif operation == FileOperation.WRITE:
                if not args.content:
                    return FileEditResult(success=False, error="content is required for write operation", error_type=CommandErrorType.VALIDATION_ERROR)
            elif operation == FileOperation.INSERT:
                if not args.insert_content:
                    return FileEditResult(success=False, error="insert_content is required for insert operation", error_type=CommandErrorType.VALIDATION_ERROR)
                if args.insert_position is None:
                    return FileEditResult(success=False, error="insert_position is required for insert operation", error_type=CommandErrorType.VALIDATION_ERROR)
            elif operation == FileOperation.DELETE:
                if args.start_line is None and args.end_line is None:
                    return FileEditResult(success=False, error="At least one of start_line or end_line must be provided for delete operation", error_type=CommandErrorType.VALIDATION_ERROR)

            # 2. Resolve operator
            try:
                operator_documents = investigation.operator_documents if investigation else []
                resolved_operator = self.execution_service.resolve_target_operator(
                    operator_documents=operator_documents,
                    target_operator=args.target_operator,
                )
            except Exception as e:
                return FileEditResult(
                    success=False, 
                    error=str(e),
                    error_type=CommandErrorType.OPERATOR_RESOLUTION_ERROR if operator_documents else CommandErrorType.NO_OPERATORS_AVAILABLE,
                )

            operator_id = resolved_operator.operator_id
            operator_session_id = resolved_operator.operator_session_id
            if not operator_session_id:
                return FileEditResult(success=False, error="Operator offline", error_type=CommandErrorType.NO_OPERATORS_AVAILABLE)

            exec_id = execution_id or generate_command_execution_id()

            # 3. Risk analysis (only for write/update)
            risk_analysis: FileOperationRiskAnalysis | None = None
            if op_name in (FileOperation.WRITE, FileOperation.REPLACE, "write", "replace"):
                try:
                    risk_analysis = await self.ai_response_analyzer.analyze_file_operation_risk(
                        operation=operation,
                        file_path=file_path,
                        content=args.content or args.new_content,
                    )
                    if risk_analysis and not risk_analysis.safe_to_proceed:
                        return FileEditResult(
                            success=False,
                            error="Risk analysis blocked operation",
                            error_type=CommandErrorType.RISK_ANALYSIS_BLOCKED,
                            blocking_issues=risk_analysis.blocking_issues
                        )
                except Exception as e:
                    logger.error("[FILE-RISK] Failed to analyze risk: %s", e)

            # 4. Approval gate (only for write/update)
            if op_name in (FileOperation.WRITE, FileOperation.REPLACE, "write", "replace"):
                approval_result = await self.approval_service.request_file_edit_approval(
                    FileEditApprovalRequest(
                        vso_context=vso_context,
                        timeout_seconds=600,
                        justification=justification or f"Requested {op_name} on {file_path}",
                        execution_id=exec_id,
                        operator_id=operator_id,
                        operator_session_id=operator_session_id,
                        file_path=file_path,
                        operation=operation,
                        risk_analysis=risk_analysis,
                    )
                )

                if not approval_result.approved:
                    # Broadcast failure
                    await self.vsod_event_service.publish_command_event(
                        EventType.OPERATOR_FILE_EDIT_FAILED,
                        CommandFailedBroadcastEvent(
                            command=f"file_edit {op_name} {file_path}",
                            execution_id=exec_id,
                            operator_session_id=operator_session_id,
                            status=ExecutionStatus.DENIED,
                            error=approval_result.reason or "Denied by user",
                            error_type=CommandErrorType.APPROVAL_DENIED,
                            approval_id=approval_result.approval_id,
                        ),
                        vso_context,
                    )
                    return FileEditResult(
                        success=False, 
                        approved=False,
                        error=approval_result.reason or "Denied by user",
                        error_type=CommandErrorType.APPROVAL_DENIED if approval_result.error_type != ApprovalErrorType.APPROVAL_TIMEOUT else CommandErrorType.APPROVAL_TIMEOUT
                    )

            # 5. Dispatch
            mcp_payload = build_tool_call_request(
                tool_name=OperatorToolName.FILE_READ if op_name == FileOperation.READ else (OperatorToolName.FILE_CREATE if op_name == FileOperation.WRITE and getattr(args, "create_if_missing", False) else (OperatorToolName.FILE_WRITE if op_name == FileOperation.WRITE else OperatorToolName.FILE_UPDATE)),
                arguments={
                    "file_path": args.file_path,
                    "content": args.content,
                    "operation": args.operation,
                    "create_if_missing": getattr(args, "create_if_missing", False),
                    "target_operator": args.target_operator,
                },
                request_id=exec_id,
            )

            vso_message = VSOMessage(
                id=exec_id,
                source_component=ComponentName.G8EE,
                event_type=EventType.OPERATOR_MCP_TOOLS_CALL,
                case_id=vso_context.case_id,
                task_id=AITaskId.FILE_EDIT,
                investigation_id=vso_context.investigation_id,
                web_session_id=vso_context.web_session_id,
                operator_session_id=operator_session_id,
                operator_id=operator_id,
                payload=mcp_payload,
            )

            # Notify start
            await self.vsod_event_service.publish_command_event(
                EventType.OPERATOR_FILE_EDIT_STARTED,
                CommandExecutingBroadcastEvent(
                    command=f"file_edit {op_name} {file_path}",
                    execution_id=exec_id,
                    operator_session_id=operator_session_id,
                    approval_id=getattr(approval_result, "approval_id", None) if "approval_result" in locals() else None,
                ),
                vso_context,
                task_id=AITaskId.FILE_EDIT,
            )

            internal_result = await self.execution_service.execute(
                vso_message=vso_message,
                vso_context=vso_context,
                timeout_seconds=60,
            )

            # Notify completion/failure
            completion_event_type = (
                EventType.OPERATOR_FILE_EDIT_COMPLETED 
                if internal_result.status == ExecutionStatus.COMPLETED 
                else EventType.OPERATOR_FILE_EDIT_FAILED
            )

            await self.vsod_event_service.publish_command_event(
                completion_event_type,
                FileEditBroadcastEvent(
                    file_path=file_path,
                    operation=op_name,
                    execution_id=exec_id,
                    operator_session_id=operator_session_id,
                    status=internal_result.status,
                    error=internal_result.error,
                    stderr=internal_result.stderr,
                    content=getattr(args, "content", None) or getattr(args, "new_content", None),
                    approval_id=getattr(approval_result, "approval_id", None) if "approval_result" in locals() else None,
                ),
                vso_context,
                task_id=AITaskId.FILE_EDIT,
            )

            return FileEditResult(
                success=internal_result.status == ExecutionStatus.COMPLETED,
                file_path=file_path,
                operation=operation,
                error=internal_result.error,
            )
        except Exception as e:
            logger.error("[FILE-ERROR] Unexpected error in execute_file_edit: %s", e, exc_info=True)
            return FileEditResult(success=False, error=str(e), error_type=CommandErrorType.EXECUTION_ERROR)

    async def execute_fetch_file_history(
        self,
        args: FetchFileHistoryArgs,
        vso_context: VSOHttpContext,
        investigation: EnrichedInvestigationContext,
    ) -> FetchFileHistoryToolResult:
        """Fetch file history from operator ledger."""
        try:
            file_path = args.file_path
            logger.info("[FILE] Fetching history for %s", file_path)

            # 1. Resolve operator
            operator_documents = investigation.operator_documents if investigation else []
            try:
                resolved_operator = self.execution_service.resolve_target_operator(
                    operator_documents=operator_documents,
                    target_operator=args.target_operator,
                )
            except Exception as e:
                return FetchFileHistoryToolResult(
                    success=False,
                    error=str(e),
                    error_type=CommandErrorType.OPERATOR_RESOLUTION_ERROR if operator_documents else CommandErrorType.NO_OPERATORS_AVAILABLE,
                )

            operator_id = resolved_operator.operator_id
            operator_session_id = resolved_operator.operator_session_id
            if not operator_session_id:
                return FetchFileHistoryToolResult(success=False, error="Operator offline", error_type=CommandErrorType.NO_OPERATORS_AVAILABLE)

            exec_id = generate_command_execution_id()
            self.execution_registry.allocate(exec_id)

            try:
                mcp_payload = build_tool_call_request(
                    tool_name=OperatorToolName.FETCH_FILE_HISTORY,
                    arguments={
                        "file_path": args.file_path,
                        "target_operator": args.target_operator,
                    },
                    request_id=exec_id,
                )

                vso_message = VSOMessage(
                    id=exec_id,
                    source_component=ComponentName.G8EE,
                    event_type=EventType.OPERATOR_MCP_TOOLS_CALL,
                    case_id=vso_context.case_id,
                    task_id=AITaskId.FETCH_FILE_HISTORY,
                    investigation_id=vso_context.investigation_id,
                    web_session_id=vso_context.web_session_id,
                    operator_session_id=operator_session_id,
                    operator_id=operator_id,
                    payload=mcp_payload,
                )

                # Notify start
                await self.vsod_event_service.publish_command_event(
                    EventType.OPERATOR_FILE_HISTORY_FETCH_STARTED,
                    CommandExecutingBroadcastEvent(
                        command=f"file_history {file_path}",
                        execution_id=exec_id,
                        operator_session_id=operator_session_id,
                    ),
                    vso_context,
                    task_id=AITaskId.FETCH_FILE_HISTORY,
                )

                internal_result = await self.execution_service.execute(
                    vso_message=vso_message,
                    vso_context=vso_context,
                    timeout_seconds=60,
                )

                # Notify completion/failure
                completion_event_type = (
                    EventType.OPERATOR_FILE_HISTORY_FETCH_COMPLETED 
                    if internal_result.status == ExecutionStatus.COMPLETED 
                    else EventType.OPERATOR_FILE_HISTORY_FETCH_FAILED
                )

                await self.vsod_event_service.publish_command_event(
                    completion_event_type,
                    CommandResultBroadcastEvent(
                        execution_id=exec_id,
                        command=f"file_history {file_path}",
                        status=internal_result.status,
                        output=internal_result.output,
                        error=internal_result.error,
                        operator_id=operator_id,
                        operator_session_id=operator_session_id,
                    ),
                    vso_context,
                    task_id=AITaskId.FETCH_FILE_HISTORY,
                )

                from app.models.pubsub_messages import FetchFileHistoryResultPayload
                envelope = self.execution_registry.get_result(exec_id)
                if envelope and isinstance(envelope.payload, FetchFileHistoryResultPayload):
                    history = envelope.payload.history or []
                    return FetchFileHistoryToolResult(
                        success=internal_result.status == ExecutionStatus.COMPLETED,
                        file_path=file_path,
                        history=history,
                        error=internal_result.error,
                    )

                return FetchFileHistoryToolResult(
                    success=internal_result.status == ExecutionStatus.COMPLETED,
                    file_path=file_path,
                    error=internal_result.error,
                )
            finally:
                self.execution_registry.release(exec_id)
        except Exception as e:
            logger.error("[FILE-ERROR] Unexpected error in execute_fetch_file_history: %s", e, exc_info=True)
            return FetchFileHistoryToolResult(success=False, error=str(e), error_type=CommandErrorType.EXECUTION_ERROR)

    async def execute_fetch_file_diff(
        self,
        args: FetchFileDiffArgs,
        vso_context: VSOHttpContext,
        investigation: EnrichedInvestigationContext,
    ) -> FetchFileDiffToolResult:
        """Fetch file diff from operator ledger."""
        try:
            file_path = args.file_path
            logger.info("[FILE] Fetching diff for %s", file_path)

            # 1. Resolve operator
            operator_documents = investigation.operator_documents if investigation else []
            try:
                resolved_operator = self.execution_service.resolve_target_operator(
                    operator_documents=operator_documents,
                    target_operator=args.target_operator,
                )
            except Exception as e:
                return FetchFileDiffToolResult(
                    success=False,
                    error=str(e),
                    error_type=CommandErrorType.OPERATOR_RESOLUTION_ERROR if operator_documents else CommandErrorType.NO_OPERATORS_AVAILABLE,
                )

            operator_id = resolved_operator.operator_id
            operator_session_id = resolved_operator.operator_session_id
            if not operator_session_id:
                return FetchFileDiffToolResult(success=False, error="Operator offline", error_type=CommandErrorType.NO_OPERATORS_AVAILABLE)

            exec_id = generate_command_execution_id()
            self.execution_registry.allocate(exec_id)

            try:
                mcp_payload = build_tool_call_request(
                    tool_name=OperatorToolName.FETCH_FILE_DIFF,
                    arguments={
                        "file_path": args.file_path,
                        "target_operator": args.target_operator,
                    },
                    request_id=exec_id,
                )

                vso_message = VSOMessage(
                    id=exec_id,
                    source_component=ComponentName.G8EE,
                    event_type=EventType.OPERATOR_MCP_TOOLS_CALL,
                    case_id=vso_context.case_id,
                    task_id=AITaskId.FETCH_FILE_DIFF,
                    investigation_id=vso_context.investigation_id,
                    web_session_id=vso_context.web_session_id,
                    operator_session_id=operator_session_id,
                    operator_id=operator_id,
                    payload=mcp_payload,
                )

                # Notify start
                await self.vsod_event_service.publish_command_event(
                    EventType.OPERATOR_FILE_DIFF_FETCH_STARTED,
                    CommandExecutingBroadcastEvent(
                        command=f"file_diff {file_path}",
                        execution_id=exec_id,
                        operator_session_id=operator_session_id,
                    ),
                    vso_context,
                    task_id=AITaskId.FETCH_FILE_DIFF,
                )

                internal_result = await self.execution_service.execute(
                    vso_message=vso_message,
                    vso_context=vso_context,
                    timeout_seconds=60,
                )

                # Notify completion/failure
                completion_event_type = (
                    EventType.OPERATOR_FILE_DIFF_FETCH_COMPLETED 
                    if internal_result.status == ExecutionStatus.COMPLETED 
                    else EventType.OPERATOR_FILE_DIFF_FETCH_FAILED
                )

                await self.vsod_event_service.publish_command_event(
                    completion_event_type,
                    CommandResultBroadcastEvent(
                        execution_id=exec_id,
                        command=f"file_diff {file_path}",
                        status=internal_result.status,
                        output=internal_result.output,
                        error=internal_result.error,
                        operator_id=operator_id,
                        operator_session_id=operator_session_id,
                    ),
                    vso_context,
                    task_id=AITaskId.FETCH_FILE_DIFF,
                )

                from app.models.pubsub_messages import FetchFileDiffResultPayload
                envelope = self.execution_registry.get_result(exec_id)
                if envelope and isinstance(envelope.payload, FetchFileDiffResultPayload):
                    diff = envelope.payload.diff
                    return FetchFileDiffToolResult(
                        success=internal_result.status == ExecutionStatus.COMPLETED,
                        diff=diff,
                        total=1 if diff else 0,
                        error=internal_result.error,
                        operator_session_id=operator_session_id,
                    )

                return FetchFileDiffToolResult(
                    success=internal_result.status == ExecutionStatus.COMPLETED,
                    error=internal_result.error,
                    operator_session_id=operator_session_id,
                )
            finally:
                self.execution_registry.release(exec_id)
        except Exception as e:
            logger.error("[FILE-ERROR] Unexpected error in execute_fetch_file_diff: %s", e, exc_info=True)
            return FetchFileDiffToolResult(success=False, error=str(e), error_type=CommandErrorType.EXECUTION_ERROR)
