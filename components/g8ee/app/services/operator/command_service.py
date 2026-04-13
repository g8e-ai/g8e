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

from __future__ import annotations
import logging

from app.clients.pubsub_client import PubSubClient
from app.models.settings import G8eePlatformSettings, G8eeUserSettings
from app.constants.status import ComponentName, CommandErrorType, ExecutionStatus
from app.constants.events import EventType
from app.constants.status import AITaskId
from app.models.agent import OperatorCommandArgs
from app.models.command_payloads import (
    CheckPortArgs,
    FileEditPayload,
    FsListArgs,
    FsReadArgs,
    GrantIntentArgs,
    RevokeIntentArgs,
)
from app.models.http_context import G8eHttpContext
from app.models.investigations import EnrichedInvestigationContext
from app.models.operators import (
    CommandApprovalRequest,
    TargetSystem,
    DirectCommandResult,
)
from app.models.internal_api import DirectCommandRequest
from app.models.pubsub_messages import G8eMessage, G8eoResultEnvelope
from app.models.tool_results import (
    CommandExecutionResult,
    FetchFileHistoryToolResult,
    FetchFileDiffToolResult,
    FileEditResult,
    FsListToolResult,
    FsReadToolResult,
    IntentPermissionResult,
    PortCheckToolResult,
    CommandRiskContext,
)
from app.models.command_payloads import (
    FetchFileHistoryArgs,
    FetchFileDiffArgs,
)
from app.services.protocols import (
    AIResponseAnalyzerProtocol,
    ApprovalServiceProtocol,
    EventServiceProtocol,
    ExecutionRegistryProtocol,
    ExecutionServiceProtocol,
    FileServiceProtocol,
    FilesystemServiceProtocol,
    InvestigationServiceProtocol,
    IntentServiceProtocol,
    LFAAServiceProtocol,
    PortServiceProtocol,
    PubSubServiceProtocol,
    G8edClientProtocol,
)
from app.services.cache.cache_aside import CacheAsideService
from .operator_data_service import OperatorDataService

from .execution_service import OperatorExecutionService
from .file_service import OperatorFileService
from .filesystem_service import OperatorFilesystemService
from .intent_service import OperatorIntentService
from .lfaa_service import OperatorLFAAService
from .port_service import OperatorPortService
from .pubsub_service import OperatorPubSubService
from app.services.mcp.adapter import build_tool_call_request
from app.utils.ids import generate_command_execution_id
from app.errors import ValidationError, BusinessLogicError

logger = logging.getLogger(__name__)


class OperatorCommandService:
    """Orchestrates operator command execution by delegating to focused services."""

    def __init__(
        self,
        pubsub_service: PubSubServiceProtocol,
        execution_registry: ExecutionRegistryProtocol,
        approval_service: ApprovalServiceProtocol,
        execution_service: ExecutionServiceProtocol,
        filesystem_service: FilesystemServiceProtocol,
        port_service: PortServiceProtocol,
        file_service: FileServiceProtocol,
        intent_service: IntentServiceProtocol,
        lfaa_service: LFAAServiceProtocol,
        cache_aside_service: CacheAsideService,
        operator_data_service: OperatorDataService,
        investigation_service: InvestigationServiceProtocol,
        settings: G8eePlatformSettings,
    ) -> None:
        self._pubsub_service = pubsub_service
        self._execution_registry = execution_registry
        self._approval_service = approval_service
        self._execution_service = execution_service
        self._filesystem_service = filesystem_service
        self._port_service = port_service
        self._file_service = file_service
        self._intent_service = intent_service
        self._lfaa_service = lfaa_service
        self.cache_aside_service = cache_aside_service
        self.operator_data_service = operator_data_service
        self.investigation_service = investigation_service
        self.g8ed_event_service = execution_service.g8ed_event_service
        self._settings = settings

        from app.models.operators import CommandResultBroadcastEvent, CommandExecutingBroadcastEvent
        self._CommandResultBroadcastEvent = CommandResultBroadcastEvent
        self._CommandExecutingBroadcastEvent = CommandExecutingBroadcastEvent

        _cv = settings.command_validation
        logger.info(
            "OperatorCommandService initialized - whitelisting: %s, blacklisting: %s",
            "ENABLED" if _cv.enable_whitelisting else "DISABLED",
            "ENABLED" if _cv.enable_blacklisting else "DISABLED",
        )

    @classmethod
    def build(
        cls,
        cache_aside_service: CacheAsideService,
        operator_data_service: OperatorDataService,
        investigation_service: InvestigationServiceProtocol,
        g8ed_event_service: EventServiceProtocol,
        execution_registry: ExecutionRegistryProtocol,
        settings: G8eePlatformSettings,
        ai_response_analyzer: AIResponseAnalyzerProtocol,
        internal_http_client: G8edClientProtocol,
        approval_service: ApprovalServiceProtocol,
    ) -> OperatorCommandService:
        """Construct, wire, and return a fully-initialised OperatorCommandService."""
        pubsub_service = OperatorPubSubService()
        
        lfaa_service = OperatorLFAAService(
            pubsub_service=pubsub_service,
        )

        execution_service = OperatorExecutionService(
            pubsub_service=pubsub_service,
            execution_registry=execution_registry,
            approval_service=approval_service,
            g8ed_event_service=g8ed_event_service,
            settings=settings,
            ai_response_analyzer=ai_response_analyzer,
            operator_data_service=operator_data_service,
            investigation_service=investigation_service,
        )

        filesystem_service = OperatorFilesystemService(
            pubsub_service=pubsub_service,
            execution_registry=execution_registry,
            execution_service=execution_service,
            investigation_service=investigation_service,
        )

        port_service = OperatorPortService(
            pubsub_service=pubsub_service,
            execution_registry=execution_registry,
            execution_service=execution_service,
        )

        file_service = OperatorFileService(
            pubsub_service=pubsub_service,
            execution_registry=execution_registry,
            approval_service=approval_service,
            g8ed_event_service=g8ed_event_service,
            execution_service=execution_service,
            ai_response_analyzer=ai_response_analyzer,
            investigation_service=investigation_service,
        )

        intent_service = OperatorIntentService(
            approval_service=approval_service,
            execution_service=execution_service,
            g8ed_event_service=g8ed_event_service,
            investigation_service=investigation_service,
            g8ed_client=internal_http_client,
        )

        async def _on_g8eo_result(envelope: G8eoResultEnvelope) -> None:
            payload = envelope.payload
            if payload is None:
                return
            execution_id = getattr(payload, "execution_id", None)
            if not execution_id:
                return
            execution_registry.complete(execution_id, envelope)

        pubsub_service.subscribe_results(_on_g8eo_result)

        return cls(
            pubsub_service=pubsub_service,
            execution_registry=execution_registry,
            approval_service=approval_service,
            execution_service=execution_service,
            filesystem_service=filesystem_service,
            port_service=port_service,
            file_service=file_service,
            intent_service=intent_service,
            lfaa_service=lfaa_service,
            cache_aside_service=cache_aside_service,
            operator_data_service=operator_data_service,
            investigation_service=investigation_service,
            settings=settings,
        )

    def set_pubsub_client(self, client: PubSubClient) -> None:
        self._pubsub_service.set_pubsub_client(client)

    async def start_pubsub_listeners(self) -> None:
        await self._pubsub_service.start()

    async def stop_pubsub_listeners(self) -> None:
        await self._pubsub_service.stop()

    def operator_service_available(self) -> bool:
        return self.operator_data_service is not None

    async def execute_command(
        self,
        args: OperatorCommandArgs,
        g8e_context: G8eHttpContext,
        investigation: EnrichedInvestigationContext,
        request_settings: G8eeUserSettings,
    ) -> CommandExecutionResult:
        """Orchestrate command execution: risk analysis -> approval -> dispatch."""
        command = args.command.strip()
        justification = args.justification.strip()
        
        logger.info("[COMMAND] Starting execution: %s", command)

        # 1. Resolve operator
        try:
            operator_documents = investigation.operator_documents if investigation else []
            resolved_operator = self._execution_service.resolve_target_operator(
                operator_documents=operator_documents,
                target_operator=args.target_operator or ""
            )
        except (ValidationError, BusinessLogicError, ValueError) as e:
            return CommandExecutionResult(success=False, error=str(e), error_type=CommandErrorType.OPERATOR_RESOLUTION_ERROR)

        operator_id = resolved_operator.operator_id
        operator_session_id = resolved_operator.operator_session_id
        if not operator_session_id:
            return CommandExecutionResult(success=False, error="Operator session not found", error_type=CommandErrorType.NO_OPERATORS_AVAILABLE)

        # Resolve target_systems for the approval UI
        if args.target_operators:
            try:
                target_operator_docs = self._execution_service.resolve_multiple_operators(
                    operator_documents, args.target_operators
                )
            except (ValidationError, BusinessLogicError):
                target_operator_docs = [resolved_operator]
        else:
            target_operator_docs = [resolved_operator]
        target_systems: list[TargetSystem] = self._execution_service.build_target_systems_list(target_operator_docs)

        execution_id = generate_command_execution_id()

        # Notify preparing (UI status update)
        await self.g8ed_event_service.publish_command_event(
            EventType.OPERATOR_COMMAND_APPROVAL_PREPARING,
            self._CommandExecutingBroadcastEvent(
                command=command,
                execution_id=execution_id,
                operator_session_id=operator_session_id,
            ),
            g8e_context,
            task_id=AITaskId.COMMAND,
        )

        risk_analysis = await self._execution_service.ai_response_analyzer.analyze_command_risk(
            command=command,
            justification=justification,
            context=CommandRiskContext(),
            settings=request_settings,
        )

        # 3. Approval gate
        approval_result = await self._approval_service.request_command_approval(
            CommandApprovalRequest(
                g8e_context=g8e_context,
                timeout_seconds=args.timeout_seconds,
                justification=justification,
                execution_id=execution_id,
                operator_id=operator_id,
                operator_session_id=operator_session_id,
                command=command,
                risk_analysis=risk_analysis,
                task_id=AITaskId.COMMAND,
                target_systems=target_systems,
            )
        )

        if not approval_result.approved:
            return CommandExecutionResult(
                success=False,
                error=approval_result.reason or "Command denied by user",
                error_type=CommandErrorType.APPROVAL_DENIED if not approval_result.feedback else CommandErrorType.USER_FEEDBACK,
                approval_id=approval_result.approval_id
            )

        # 4. Dispatch
        mcp_payload = build_tool_call_request(
            tool_name="run_commands_with_operator",
            arguments={
                "execution_id": execution_id,
                "command": command,
                "justification": justification,
                "timeout_seconds": args.timeout_seconds,
            },
            request_id=execution_id,
        )

        g8e_message = G8eMessage(
            id=execution_id,
            source_component=ComponentName.G8EE,
            event_type=EventType.OPERATOR_MCP_TOOLS_CALL,
            case_id=g8e_context.case_id,
            task_id=AITaskId.COMMAND,
            investigation_id=g8e_context.investigation_id,
            web_session_id=g8e_context.web_session_id,
            operator_session_id=operator_session_id,
            operator_id=operator_id,
            payload=mcp_payload,
        )

        # Notify start
        await self.g8ed_event_service.publish_command_event(
            EventType.OPERATOR_COMMAND_STARTED,
            self._CommandExecutingBroadcastEvent(
                command=command,
                execution_id=execution_id,
                operator_session_id=operator_session_id,
                approval_id=approval_result.approval_id,
            ),
            g8e_context,
            task_id=AITaskId.COMMAND,
        )

        logger.info("[COMMAND] Executing: %s", command)
        internal_result = await self._execution_service.execute(
            g8e_message=g8e_message,
            g8e_context=g8e_context,
            timeout_seconds=args.timeout_seconds,
        )
        logger.info("[COMMAND] Execution finished: status=%s", internal_result.status)

        # Notify completion/failure
        completion_event_type = (
            EventType.OPERATOR_COMMAND_COMPLETED 
            if internal_result.status == ExecutionStatus.COMPLETED 
            else EventType.OPERATOR_COMMAND_FAILED
        )
        
        await self.g8ed_event_service.publish_command_event(
            completion_event_type,
            self._CommandResultBroadcastEvent(
                execution_id=execution_id,
                command=command,
                status=internal_result.status,
                output=internal_result.output,
                error=internal_result.error,
                stderr=internal_result.stderr,
                exit_code=internal_result.exit_code,
                execution_time_seconds=internal_result.execution_time_seconds,
                operator_id=operator_id,
                operator_session_id=operator_session_id,
                approval_id=approval_result.approval_id,
            ),
            g8e_context,
            task_id=AITaskId.COMMAND,
        )

        return CommandExecutionResult(
            success=internal_result.status == ExecutionStatus.COMPLETED,
            command_executed=command,
            justification=justification,
            output=internal_result.output,
            stderr=internal_result.stderr,
            exit_code=internal_result.exit_code,
            execution_status=internal_result.status,
            execution_result=internal_result,
        )

    async def execute_file_edit(self, args: FileEditPayload, g8e_context: G8eHttpContext, investigation: EnrichedInvestigationContext, execution_id: str) -> FileEditResult:
        return await self._file_service.execute_file_edit(args, g8e_context, investigation, execution_id)

    async def execute_port_check(self, args: CheckPortArgs, investigation: EnrichedInvestigationContext, g8e_context: G8eHttpContext) -> PortCheckToolResult:
        return await self._port_service.execute_port_check(args, investigation, g8e_context=g8e_context)

    async def execute_fs_list(self, args: FsListArgs, investigation: EnrichedInvestigationContext, g8e_context: G8eHttpContext) -> FsListToolResult:
        return await self._filesystem_service.execute_fs_list(args, investigation, g8e_context=g8e_context)

    async def execute_fs_read(self, args: FsReadArgs, investigation: EnrichedInvestigationContext, g8e_context: G8eHttpContext) -> FsReadToolResult:
        return await self._filesystem_service.execute_fs_read(args, investigation, g8e_context=g8e_context)

    async def execute_intent_permission_request(self, args: GrantIntentArgs, g8e_context: G8eHttpContext, investigation: EnrichedInvestigationContext) -> IntentPermissionResult:
        return await self._intent_service.execute_intent_permission_request(
            args=args, g8e_context=g8e_context, investigation=investigation
        )

    async def execute_intent_revocation(self, args: RevokeIntentArgs, g8e_context: G8eHttpContext, investigation: EnrichedInvestigationContext) -> IntentPermissionResult:
        return await self._intent_service.execute_intent_revocation(
            args=args, g8e_context=g8e_context, investigation=investigation
        )

    async def execute_fetch_file_history(self, args: FetchFileHistoryArgs, g8e_context: G8eHttpContext, investigation: EnrichedInvestigationContext) -> FetchFileHistoryToolResult:
        return await self._file_service.execute_fetch_file_history(args, g8e_context, investigation)

    async def execute_fetch_file_diff(self, args: FetchFileDiffArgs, g8e_context: G8eHttpContext, investigation: EnrichedInvestigationContext) -> FetchFileDiffToolResult:
        return await self._file_service.execute_fetch_file_diff(args, g8e_context, investigation)

    async def send_command_to_operator(
        self,
        command_payload: DirectCommandRequest,
        g8e_context: G8eHttpContext,
    ) -> DirectCommandResult:
        """Delegate direct terminal command dispatch to the execution service."""
        return await self._execution_service.send_command_to_operator(
            command_payload=command_payload,
            g8e_context=g8e_context,
        )

    async def send_direct_exec_audit_event(
        self,
        command: str,
        execution_id: str,
        g8e_context: G8eHttpContext,
    ) -> bool:
        """Delegate LFAA audit event dispatch to the LFAA service."""
        return await self._lfaa_service.send_direct_exec_audit_event(
            command=command,
            execution_id=execution_id,
            g8e_context=g8e_context,
        )

__all__ = ["OperatorCommandService"]
