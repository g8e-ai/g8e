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
    BatchOperatorExecutionResult,
    CommandApprovalRequest,
    OperatorDocument,
    TargetSystem,
    DirectCommandResult,
)
from app.models.internal_api import DirectCommandRequest
from app.models.pubsub_messages import G8eMessage, G8eoResultEnvelope
from app.models.tool_results import (
    CommandInternalResult,
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
from app.utils.ids import generate_batch_id, generate_command_execution_id
from app.errors import ValidationError, BusinessLogicError
import asyncio

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
            # The transport-level correlation key is envelope.id, which g8eo echoes
            # verbatim from the outbound g8e_message.id (see publishLFAATypedResponseTo
            # in components/g8eo/services/pubsub/publish_helpers.go). Some typed
            # payloads also carry an execution_id field that mirrors the same id; for
            # LFAA payloads (FetchFileHistoryResultPayload, FetchHistoryResultPayload,
            # RestoreFileResultPayload, FetchFileDiffResultPayload) that field is
            # absent. We prefer payload.execution_id when set to preserve legacy
            # behaviour for command results and status updates, and fall back to
            # envelope.id so LFAA results actually complete their waiter instead of
            # timing out at 60 s.
            payload = envelope.payload
            if payload is None:
                return
            execution_id = getattr(payload, "execution_id", None) or envelope.id
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
        """Orchestrate command execution: resolve -> validate -> approve -> fan-out dispatch.

        Single-operator runs and multi-operator ("batch") runs share the same pipeline:
        one command validation, one risk analysis, one approval, then N parallel per-operator
        dispatches correlated by a batch_id. For N==1 the return shape matches legacy behavior.
        """
        command = args.command.strip()
        # Sage never writes `command` directly; the Tribunal produces it from
        # Sage's request+guidelines. The approval UI still shows a "justification"
        # line to the user, which in the new model is Sage's natural-language
        # request (plus optional guidelines).
        justification_parts = [args.request.strip()] if args.request else []
        if args.guidelines and args.guidelines.strip():
            justification_parts.append(f"Guidelines: {args.guidelines.strip()}")
        justification = " | ".join(justification_parts) if justification_parts else "(no justification provided)"

        logger.info("[COMMAND] Starting execution: %s", command)

        # 1. Resolve target operator(s) — unified path for singular and batch.
        operator_documents = investigation.operator_documents if investigation else []
        try:
            target_operator_docs = self._resolve_targets(operator_documents, args)
        except (ValidationError, BusinessLogicError, ValueError) as e:
            return CommandExecutionResult(
                success=False, error=str(e), error_type=CommandErrorType.OPERATOR_RESOLUTION_ERROR,
            )

        # All resolved operators must have a live session.
        missing_session = [op for op in target_operator_docs if not op.operator_session_id]
        if missing_session:
            return CommandExecutionResult(
                success=False,
                error="Operator session not found for: "
                      + ", ".join(op.operator_id or "<unknown>" for op in missing_session),
                error_type=CommandErrorType.NO_OPERATORS_AVAILABLE,
            )

        target_systems: list[TargetSystem] = self._execution_service.build_target_systems_list(target_operator_docs)
        is_batch = len(target_operator_docs) > 1

        # 2. Command validation (whitelist/blacklist enforcement) — once for the whole batch.
        cv = self._settings.command_validation
        if cv.enable_whitelisting:
            whitelist_result = self._execution_service.whitelist_validator.validate_command(command)
            if not whitelist_result.is_valid:
                logger.warning("[COMMAND] Whitelist validation failed: %s", whitelist_result.reason)
                return CommandExecutionResult(
                    success=False,
                    error=whitelist_result.reason,
                    error_type=CommandErrorType.WHITELIST_VIOLATION,
                    blocked_command=command,
                    validation_details={"reason": whitelist_result.reason, "violations": whitelist_result.violations or []},
                )

        if cv.enable_blacklisting:
            blacklist_result = self._execution_service.blacklist_validator.validate_command(command)
            if not blacklist_result.is_allowed:
                logger.warning("[COMMAND] Blacklist validation failed: %s - %s", blacklist_result.rule, blacklist_result.reason)
                return CommandExecutionResult(
                    success=False,
                    error=blacklist_result.reason or f"Command blocked by blacklist rule: {blacklist_result.rule}",
                    error_type=CommandErrorType.BLACKLIST_VIOLATION,
                    blocked_command=command,
                    rule=blacklist_result.rule,
                )

        # Primary operator is the first resolved — used for approval identity fields.
        primary = target_operator_docs[0]
        primary_operator_id = primary.operator_id or ""
        primary_session_id = primary.operator_session_id or ""
        batch_id = generate_batch_id() if is_batch else None
        approval_execution_id = generate_command_execution_id()

        # 3. Notify preparing (one event for the approval card).
        await self.g8ed_event_service.publish_command_event(
            EventType.OPERATOR_COMMAND_APPROVAL_PREPARING,
            self._CommandExecutingBroadcastEvent(
                command=command,
                execution_id=approval_execution_id,
                operator_session_id=primary_session_id,
                operator_id=primary_operator_id,
                batch_id=batch_id,
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

        # 4. Approval gate — a single approval covers the whole batch.
        approval_result = await self._approval_service.request_command_approval(
            CommandApprovalRequest(
                g8e_context=g8e_context,
                timeout_seconds=args.timeout_seconds,
                justification=justification,
                execution_id=approval_execution_id,
                operator_id=primary_operator_id,
                operator_session_id=primary_session_id,
                command=command,
                risk_analysis=risk_analysis,
                task_id=AITaskId.COMMAND,
                target_systems=target_systems,
                batch_id=batch_id,
            )
        )

        if not approval_result.approved:
            return CommandExecutionResult(
                success=False,
                error=approval_result.reason or "Command denied by user",
                error_type=CommandErrorType.APPROVAL_DENIED if not approval_result.feedback else CommandErrorType.USER_FEEDBACK,
                approval_id=approval_result.approval_id,
            )

        # 5. Fan-out dispatch — one execution_id per operator, bounded concurrency.
        max_concurrency = cv.max_batch_concurrency
        fail_fast = cv.batch_fail_fast
        semaphore = asyncio.Semaphore(max_concurrency)
        cancel_event = asyncio.Event()

        async def _publish_failed(exec_id: str, op_id: str, op_session_id: str, hostname: str, error_msg: str) -> None:
            """Emit OPERATOR_COMMAND_FAILED so the UI always reflects every operator in the batch."""
            try:
                await self.g8ed_event_service.publish_command_event(
                    EventType.OPERATOR_COMMAND_FAILED,
                    self._CommandResultBroadcastEvent(
                        execution_id=exec_id,
                        command=command,
                        status=ExecutionStatus.FAILED,
                        output=None,
                        error=error_msg,
                        stderr=None,
                        exit_code=None,
                        execution_time_seconds=None,
                        operator_id=op_id,
                        operator_session_id=op_session_id,
                        hostname=hostname,
                        approval_id=approval_result.approval_id,
                        batch_id=batch_id,
                    ),
                    g8e_context,
                    task_id=AITaskId.COMMAND,
                )
            except Exception as e:  # noqa: BLE001 — best-effort notification
                logger.warning("[COMMAND] Failed to publish FAILED event for %s: %s", op_id, e)

        async def _dispatch(op: OperatorDocument) -> BatchOperatorExecutionResult:
            exec_id = generate_command_execution_id()
            op_id = op.operator_id or ""
            op_session_id = op.operator_session_id or ""
            hostname = op.current_hostname or (op.system_info.hostname if op.system_info else None) or op_id

            if cancel_event.is_set():
                await _publish_failed(exec_id, op_id, op_session_id, hostname, "Cancelled by fail-fast")
                return BatchOperatorExecutionResult(
                    hostname=hostname, operator_id=op_id, execution_id=exec_id,
                    success=False, error="Cancelled by fail-fast",
                )

            async with semaphore:
                if cancel_event.is_set():
                    await _publish_failed(exec_id, op_id, op_session_id, hostname, "Cancelled by fail-fast")
                    return BatchOperatorExecutionResult(
                        hostname=hostname, operator_id=op_id, execution_id=exec_id,
                        success=False, error="Cancelled by fail-fast",
                    )

                mcp_payload = build_tool_call_request(
                    tool_name="run_commands_with_operator",
                    arguments={
                        "execution_id": exec_id,
                        "command": command,
                        "justification": justification,
                        "timeout_seconds": args.timeout_seconds,
                    },
                    request_id=exec_id,
                )
                g8e_message = G8eMessage(
                    id=exec_id,
                    source_component=ComponentName.G8EE,
                    event_type=EventType.OPERATOR_MCP_TOOLS_CALL,
                    case_id=g8e_context.case_id,
                    task_id=AITaskId.COMMAND,
                    investigation_id=g8e_context.investigation_id,
                    web_session_id=g8e_context.web_session_id,
                    operator_session_id=op_session_id,
                    operator_id=op_id,
                    payload=mcp_payload,
                )

                await self.g8ed_event_service.publish_command_event(
                    EventType.OPERATOR_COMMAND_STARTED,
                    self._CommandExecutingBroadcastEvent(
                        command=command,
                        execution_id=exec_id,
                        operator_session_id=op_session_id,
                        operator_id=op_id,
                        approval_id=approval_result.approval_id,
                        batch_id=batch_id,
                    ),
                    g8e_context,
                    task_id=AITaskId.COMMAND,
                )

                try:
                    internal_result = await self._execution_service.execute(
                        g8e_message=g8e_message,
                        g8e_context=g8e_context,
                        timeout_seconds=args.timeout_seconds,
                    )
                except Exception as e:  # noqa: BLE001 — isolate per-operator failure
                    logger.exception("[COMMAND] Per-operator dispatch failed on %s: %s", op_id, e)
                    if fail_fast:
                        cancel_event.set()
                    await _publish_failed(exec_id, op_id, op_session_id, hostname, str(e))
                    return BatchOperatorExecutionResult(
                        hostname=hostname, operator_id=op_id, execution_id=exec_id,
                        success=False, error=str(e),
                    )

                completion_event_type = (
                    EventType.OPERATOR_COMMAND_COMPLETED
                    if internal_result.status == ExecutionStatus.COMPLETED
                    else EventType.OPERATOR_COMMAND_FAILED
                )
                await self.g8ed_event_service.publish_command_event(
                    completion_event_type,
                    self._CommandResultBroadcastEvent(
                        execution_id=exec_id,
                        command=command,
                        status=internal_result.status,
                        output=internal_result.output,
                        error=internal_result.error,
                        stderr=internal_result.stderr,
                        exit_code=internal_result.exit_code,
                        execution_time_seconds=internal_result.execution_time_seconds,
                        operator_id=op_id,
                        operator_session_id=op_session_id,
                        hostname=hostname,
                        approval_id=approval_result.approval_id,
                        batch_id=batch_id,
                    ),
                    g8e_context,
                    task_id=AITaskId.COMMAND,
                )

                succeeded = internal_result.status == ExecutionStatus.COMPLETED
                if not succeeded and fail_fast:
                    cancel_event.set()
                return BatchOperatorExecutionResult(
                    hostname=hostname,
                    operator_id=op_id,
                    execution_id=exec_id,
                    success=succeeded,
                    result=internal_result,
                    error=internal_result.error if not succeeded else None,
                )

        logger.info("[COMMAND] Dispatching to %d operator(s) (batch_id=%s)", len(target_operator_docs), batch_id)
        per_operator_results: list[BatchOperatorExecutionResult] = await asyncio.gather(
            *[_dispatch(op) for op in target_operator_docs]
        )

        return self._assemble_result(
            command=command,
            justification=justification,
            per_operator_results=per_operator_results,
            approval_id=approval_result.approval_id,
            is_batch=is_batch,
            batch_id=batch_id,
        )

    # ------------------------------------------------------------------
    # Helpers
    # ------------------------------------------------------------------

    def _resolve_targets(
        self,
        operator_documents: list[OperatorDocument],
        args: OperatorCommandArgs,
    ) -> list[OperatorDocument]:
        """Unified resolution for singular (`target_operator`) and batch (`target_operators`)."""
        if args.target_operators:
            return self._execution_service.resolve_multiple_operators(
                operator_documents, args.target_operators
            )
        return [self._execution_service.resolve_target_operator(
            operator_documents=operator_documents,
            target_operator=args.target_operator or "",
        )]

    def _assemble_result(
        self,
        *,
        command: str,
        justification: str,
        per_operator_results: list[BatchOperatorExecutionResult],
        approval_id: str | None,
        is_batch: bool,
        batch_id: str | None,
    ) -> CommandExecutionResult:
        """Collapse per-operator results into a single CommandExecutionResult.

        For N==1 we preserve legacy field population (output/stderr/exit_code at top level)
        so downstream consumers keep working. For N>1 we additionally populate batch fields
        and a combined output with per-host headers so the agent can reason about divergence.
        """
        successful = [r for r in per_operator_results if r.success]
        failed = [r for r in per_operator_results if not r.success]
        internal_results: list[CommandInternalResult] = [r.result for r in per_operator_results if r.result is not None]

        if not is_batch:
            only = per_operator_results[0]
            res = only.result
            return CommandExecutionResult(
                success=only.success,
                command_executed=command,
                justification=justification,
                output=res.output if res else None,
                stderr=res.stderr if res else None,
                exit_code=res.exit_code if res else None,
                execution_status=res.status if res else None,
                execution_result=res,
                execution_id=only.execution_id,
                approval_id=approval_id,
                batch_id=batch_id,
                error=only.error,
            )

        # Batch: aggregate outputs with host headers for the agent.
        def _section(r: BatchOperatorExecutionResult) -> str:
            header = f"===== {r.hostname} ({r.operator_id}) ====="
            if r.result is not None:
                body_parts = []
                if r.result.output:
                    body_parts.append(r.result.output)
                if r.result.stderr:
                    body_parts.append(f"[stderr]\n{r.result.stderr}")
                status = f"[status={r.result.status} exit_code={r.result.exit_code}]"
                body_parts.append(status)
                return header + "\n" + "\n".join(body_parts)
            return header + f"\n[error] {r.error or 'unknown failure'}"

        combined_output = "\n\n".join(_section(r) for r in per_operator_results)
        all_succeeded = not failed
        aggregate_error: str | None = None
        if failed:
            aggregate_error = (
                f"{len(failed)}/{len(per_operator_results)} operator(s) failed: "
                + ", ".join(f"{r.hostname}={r.error or 'failed'}" for r in failed)
            )

        return CommandExecutionResult(
            success=all_succeeded,
            command_executed=command,
            justification=justification,
            output=combined_output,
            execution_status=ExecutionStatus.COMPLETED if all_succeeded else ExecutionStatus.FAILED,
            batch_execution=True,
            operators_used=len(per_operator_results),
            successful_count=len(successful),
            failed_count=len(failed),
            execution_results=internal_results or None,
            approval_id=approval_id,
            batch_id=batch_id,
            error=aggregate_error,
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
