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
from app.models.agent import ExecutorCommandArgs
from app.models.tool_args import (
    GrantIntentArgs,
    RevokeIntentArgs,
)
from app.models.command_request_payloads import (
    CheckPortRequestPayload,
    CommandRequestPayload,
    FetchFileHistoryRequestPayload,
    FetchFileDiffRequestPayload,
    FileEditRequestPayload,
    FsListRequestPayload,
    FsReadRequestPayload,
)
from app.models.http_context import G8eHttpContext
from app.models.investigations import EnrichedInvestigationContext
from app.models.operators import (
    ApprovalResult,
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
from app.services.protocols import (
    AIResponseAnalyzerProtocol,
    ApprovalServiceProtocol,
    EventServiceProtocol,
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
from app.services.investigation.investigation_service import extract_single_operator_context
from .operator_data_service import OperatorDataService

from .execution_service import OperatorExecutionService
from .file_service import OperatorFileService
from .filesystem_service import OperatorFilesystemService
from .intent_service import OperatorIntentService
from .lfaa_service import OperatorLFAAService
from .port_service import OperatorPortService
from .pubsub_service import OperatorPubSubService
from app.utils.safety import validate_command_safety
from app.utils.whitelist_validator import parse_whitelisted_commands_csv
from app.utils.validators import get_blacklist_validator, get_whitelist_validator
from app.utils.ids import generate_command_execution_id, generate_batch_id
from app.utils.whitelist_validator import CommandWhitelistValidator
from app.utils.blacklist_validator import CommandBlacklistValidator
from app.errors import ValidationError, BusinessLogicError
from app.models.operators import CommandResultBroadcastEvent, CommandExecutingBroadcastEvent
import asyncio

logger = logging.getLogger(__name__)


class OperatorCommandService:
    """Orchestrates operator command execution by delegating to focused services."""

    def __init__(
        self,
        pubsub_service: PubSubServiceProtocol,
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
        whitelist_validator: CommandWhitelistValidator | None = None,
        blacklist_validator: CommandBlacklistValidator | None = None,
    ) -> None:
        self._pubsub_service = pubsub_service
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

        self._whitelist_validator = whitelist_validator if whitelist_validator is not None else get_whitelist_validator()
        self._blacklist_validator = blacklist_validator if blacklist_validator is not None else get_blacklist_validator()

        self._CommandResultBroadcastEvent = CommandResultBroadcastEvent
        self._CommandExecutingBroadcastEvent = CommandExecutingBroadcastEvent

        self._cv = settings.command_validation
        logger.info(
            "OperatorCommandService initialized with PLATFORM DEFAULTS (per-user overrides apply) - whitelisting: %s, blacklisting: %s",
            "ENABLED" if self._cv.enable_whitelisting else "DISABLED",
            "ENABLED" if self._cv.enable_blacklisting else "DISABLED",
        )
        if self._cv.whitelisted_commands:
            logger.warning(
                "CSV whitelist override is globally active. "
                "Per-command safe_options and validation regexes from JSON are BYPASSED. "
                "Falling back to basic _is_safe_value checks for command arguments."
            )

    @classmethod
    def build(
        cls,
        cache_aside_service: CacheAsideService,
        operator_data_service: OperatorDataService,
        investigation_service: InvestigationServiceProtocol,
        g8ed_event_service: EventServiceProtocol,
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
            approval_service=approval_service,
            g8ed_event_service=g8ed_event_service,
            settings=settings,
            ai_response_analyzer=ai_response_analyzer,
            operator_data_service=operator_data_service,
            investigation_service=investigation_service,
        )

        filesystem_service = OperatorFilesystemService(
            pubsub_service=pubsub_service,
            execution_service=execution_service,
            investigation_service=investigation_service,
        )

        port_service = OperatorPortService(
            pubsub_service=pubsub_service,
            execution_service=execution_service,
        )

        file_service = OperatorFileService(
            pubsub_service=pubsub_service,
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

        return cls(
            pubsub_service=pubsub_service,
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

    async def execute_command(
        self,
        args: ExecutorCommandArgs,
        g8e_context: G8eHttpContext,
        investigation: EnrichedInvestigationContext,
        request_settings: G8eeUserSettings,
        execution_id: str | None = None,
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
            logger.error("[COMMAND] Operator resolution failed: %s", e, exc_info=True)
            return CommandExecutionResult(
                success=False, error=f"Operator resolution failed: {e}. Ensure at least one operator is online and has a valid session, then retry.", error_type=CommandErrorType.OPERATOR_RESOLUTION_ERROR,
            )

        # All resolved operators must have a live session.
        missing_session = [op for op in target_operator_docs if not op.operator_session_id]
        if missing_session:
            return CommandExecutionResult(
                success=False,
                error="Operator session not found for: "
                      + ", ".join(op.id for op in missing_session),
                error_type=CommandErrorType.NO_OPERATORS_AVAILABLE,
            )

        target_systems: list[TargetSystem] = self._execution_service.build_target_systems_list(target_operator_docs)
        is_batch = len(target_operator_docs) > 1

        # Primary operator is the first resolved — used for approval identity fields.
        primary = target_operator_docs[0]

        # 2. Command validation (L1 technical bedrock: whitelist/blacklist/forbidden patterns)
        # Prefer the per-request (user) command_validation settings — get_user_settings
        # already falls back to platform defaults when no user document exists.
        cv = request_settings.command_validation if request_settings else self._cv
        whitelist_override = parse_whitelisted_commands_csv(cv.whitelisted_commands)
        operator_context = extract_single_operator_context(primary) if primary else None
        safety_result = validate_command_safety(
            command,
            whitelisting_enabled=cv.enable_whitelisting,
            blacklisting_enabled=cv.enable_blacklisting,
            operator_context=operator_context,
            whitelisted_commands_override=whitelist_override,
            whitelist_validator=self._whitelist_validator,
            blacklist_validator=self._blacklist_validator,
        )
        if not safety_result.is_safe:
            logger.warning("[COMMAND] Technical safety validation failed: %s", safety_result.error_message)
            return CommandExecutionResult(
                success=False,
                error=safety_result.error_message,
                error_type=safety_result.error_type,
                blocked_command=command,
                validation_details={"reason": safety_result.error_message},
            )
        primary_operator_id = primary.id
        primary_session_id = primary.operator_session_id or ""
        batch_id = generate_batch_id() if is_batch else None

        # Generate per-operator execution IDs upfront so PREPARING can correlate with STARTED
        per_operator_exec_ids = [generate_command_execution_id() for _ in target_operator_docs]

        # For single operator, use its exec_id as the approval_execution_id to unify IDs
        # For batch, use a separate approval_execution_id but include per-operator IDs in PREPARING
        approval_execution_id = per_operator_exec_ids[0] if not is_batch else generate_command_execution_id()

        # Auto-approval gate (separate from whitelist hard-allow-list).
        # The human has rubber-stamped these base commands as benign, so
        # individual approval prompts are skipped. Auto-approve is independent
        # of whitelisting: a command must still pass ALL L1 hard gates above
        # (forbidden patterns, blacklist, and whitelist if enabled) before
        # auto-approve can apply.
        auto_approve_list = parse_whitelisted_commands_csv(cv.auto_approved_commands)
        base_command = command.split()[0] if command else ""
        is_auto_approved = (
            cv.enable_auto_approve
            and base_command != ""
            and base_command in auto_approve_list
        )
        if is_auto_approved:
            logger.info(
                "[COMMAND] Base command %r is in auto-approve list - bypassing human approval: %s",
                base_command, command,
            )

        # 3. Notify preparing (one event for the approval card).
        # Skip for auto-approved commands since they bypass the approval UI.
        if not is_auto_approved:
            await self.g8ed_event_service.publish_command_event(
                EventType.OPERATOR_COMMAND_APPROVAL_PREPARING,
                self._CommandExecutingBroadcastEvent(
                    command=command,
                    execution_id=approval_execution_id,
                    operator_session_id=primary_session_id,
                    operator_id=primary_operator_id,
                    batch_id=batch_id,
                    per_operator_execution_ids=per_operator_exec_ids if is_batch else [],
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
        # Auto-approved base commands skip the human approval prompt
        # (the human has rubber-stamped them via auto_approved_commands).
        if is_auto_approved:
            approval_result = ApprovalResult(
                approved=True,
                reason="Base command is in auto-approve list - human approval bypassed",
                approval_id=None,
            )
        else:
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
                correlation_id=args.correlation_id,
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
        max_concurrency = self._cv.max_batch_concurrency
        fail_fast = self._cv.batch_fail_fast
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

        async def _dispatch(op: OperatorDocument, exec_id: str) -> BatchOperatorExecutionResult:
            op_id = op.id
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

                g8e_message = G8eMessage(
                    id=exec_id,
                    source_component=ComponentName.G8EE,
                    event_type=EventType.OPERATOR_COMMAND_REQUESTED,
                    case_id=g8e_context.case_id,
                    task_id=AITaskId.COMMAND,
                    investigation_id=g8e_context.investigation_id,
                    web_session_id=g8e_context.web_session_id,
                    operator_session_id=op_session_id,
                    operator_id=op_id,
                    payload=CommandRequestPayload(
                        command=command,
                        execution_id=exec_id,
                        justification=justification,
                        timeout_seconds=args.timeout_seconds,
                    ),
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
                    internal_result, envelope = await self._execution_service.execute(
                        g8e_message=g8e_message,
                        g8e_context=g8e_context,
                        timeout_seconds=args.timeout_seconds,
                    )
                except Exception as e:  # noqa: BLE001 — isolate per-operator failure
                    logger.exception("[COMMAND] Per-operator dispatch failed on %s: %s", op_id, e)
                    if fail_fast:
                        cancel_event.set()
                    await _publish_failed(exec_id, op_id, op_session_id, hostname, f"Command execution failed: {e}")
                    return BatchOperatorExecutionResult(
                        hostname=hostname, operator_id=op_id, execution_id=exec_id,
                        success=False, error=f"Command execution failed: {e}. Check operator status and retry.",
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
            *[_dispatch(op, per_operator_exec_ids[i]) for i, op in enumerate(target_operator_docs)]
        )

        return self._assemble_result(
            command=command,
            justification=justification,
            per_operator_results=per_operator_results,
            approval_id=approval_result.approval_id,
            is_batch=is_batch,
            batch_id=batch_id,
            warden_risk=risk_analysis.risk_level if risk_analysis else None,
        )

    # ------------------------------------------------------------------
    # Helpers
    # ------------------------------------------------------------------

    def _resolve_targets(
        self,
        operator_documents: list[OperatorDocument],
        args: ExecutorCommandArgs,
    ) -> list[OperatorDocument]:
        """Unified resolution for singular (`target_operator`) and batch (`target_operators`)."""
        if args.target_operators:
            return self._execution_service.resolve_multiple_operators(
                operator_documents, args.target_operators
            )
        return [self._execution_service.resolve_target_operator(
            operator_documents=operator_documents,
            target_operator=args.target_operator or "",
            tool_name="run_commands_with_operator",
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
        warden_risk: RiskLevel | None = None,
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
                warden_risk=warden_risk,
            )

        # Batch: aggregate outputs with host headers for the agent.
        def _section(r: BatchOperatorExecutionResult) -> str:
            header = f"===== {r.hostname} ({r.operator_id}) ====="
            if r.result is not None:
                body_parts: list[str] = []
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
            warden_risk=warden_risk,
        )

    async def execute_file_edit(self, args: FileEditRequestPayload, g8e_context: G8eHttpContext, investigation: EnrichedInvestigationContext) -> FileEditResult:
        return await self._file_service.execute_file_edit(args, g8e_context, investigation)

    async def execute_port_check(self, args: CheckPortRequestPayload, investigation: EnrichedInvestigationContext, g8e_context: G8eHttpContext) -> PortCheckToolResult:
        return await self._port_service.execute_port_check(args, investigation, g8e_context=g8e_context)

    async def execute_fs_list(self, args: FsListRequestPayload, investigation: EnrichedInvestigationContext, g8e_context: G8eHttpContext) -> FsListToolResult:
        return await self._filesystem_service.execute_fs_list(args, investigation, g8e_context=g8e_context)

    async def execute_file_read(self, args: FsReadRequestPayload, investigation: EnrichedInvestigationContext, g8e_context: G8eHttpContext) -> FsReadToolResult:
        return await self._filesystem_service.execute_file_read(args, investigation, g8e_context=g8e_context)

    async def execute_intent_permission_request(self, args: GrantIntentArgs, g8e_context: G8eHttpContext, investigation: EnrichedInvestigationContext) -> IntentPermissionResult:
        return await self._intent_service.execute_intent_permission_request(
            args=args, g8e_context=g8e_context, investigation=investigation
        )

    async def execute_intent_revocation(self, args: RevokeIntentArgs, g8e_context: G8eHttpContext, investigation: EnrichedInvestigationContext) -> IntentPermissionResult:
        return await self._intent_service.execute_intent_revocation(
            args=args, g8e_context=g8e_context, investigation=investigation
        )

    async def execute_fetch_file_history(self, args: FetchFileHistoryRequestPayload, g8e_context: G8eHttpContext, investigation: EnrichedInvestigationContext) -> FetchFileHistoryToolResult:
        return await self._file_service.execute_fetch_file_history(args, g8e_context, investigation)

    async def execute_fetch_file_diff(self, args: FetchFileDiffRequestPayload, g8e_context: G8eHttpContext, investigation: EnrichedInvestigationContext) -> FetchFileDiffToolResult:
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
