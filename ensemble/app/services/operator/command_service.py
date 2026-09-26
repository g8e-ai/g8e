# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations
import logging

from app.clients.gateway_operator_client import GatewayOperatorClient
from app.models.settings import G8eeAppSettings, G8eeUserSettings
from app.constants.generated_status import CommandErrorType, RiskLevel
from app.constants.config import ExecutionStatus
from app.constants import EventType, G8EE_COMPONENT
from app.constants.generated_status import AITaskId
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
from app.models.pubsub_messages import G8eMessage
from app.models.tool_results import (
    CommandInternalResult,
    CommandExecutionResult,
    FetchFileHistoryToolResult,
    FetchFileDiffToolResult,
    FileEditResult,
    FsGrepToolResult,
    FsListToolResult,
    FsReadToolResult,
    IntentPermissionResult,
    PortCheckToolResult,
)
from app.services.protocols import (
    AIResponseAnalyzerProtocol,
    ApprovalServiceProtocol,
    ExecutionServiceProtocol,
    FileServiceProtocol,
    FilesystemServiceProtocol,
    InvestigationServiceProtocol,
    IntentServiceProtocol,
    LFAAServiceProtocol,
    PortServiceProtocol,
    G8eClientProtocol,
)
from app.services.investigation.investigation_service import extract_single_operator_context

from .execution_service import OperatorExecutionService
from .file_service import OperatorFileService
from .filesystem_service import OperatorFilesystemService
from .intent_service import OperatorIntentService
from .lfaa_service import OperatorLFAAService
from .port_service import OperatorPortService
from app.utils.safety import validate_command_safety
from app.utils.csv_commands import parse_command_csv
from app.utils.validators import (
    get_auto_approved_validator,
    get_blacklist_validator,
    get_whitelist_validator,
)
from app.utils.ids import generate_command_execution_id, generate_batch_id
from app.utils.whitelist_validator import CommandWhitelistValidator
from app.utils.blacklist_validator import CommandBlacklistValidator
from app.utils.auto_approved_validator import CommandAutoApprovedValidator
from app.errors import ValidationError, BusinessLogicError
import asyncio

logger = logging.getLogger(__name__)


class OperatorCommandService:
    """Orchestrates operator command execution by delegating to focused services."""

    def __init__(
        self,
        approval_service: ApprovalServiceProtocol,
        execution_service: ExecutionServiceProtocol,
        filesystem_service: FilesystemServiceProtocol,
        port_service: PortServiceProtocol,
        file_service: FileServiceProtocol,
        intent_service: IntentServiceProtocol,
        lfaa_service: LFAAServiceProtocol,
        investigation_service: InvestigationServiceProtocol,
        settings: G8eeAppSettings,
        whitelist_validator: CommandWhitelistValidator | None = None,
        blacklist_validator: CommandBlacklistValidator | None = None,
        auto_approved_validator: CommandAutoApprovedValidator | None = None,
    ) -> None:
        self._approval_service = approval_service
        self._execution_service = execution_service
        self._filesystem_service = filesystem_service
        self._port_service = port_service
        self._file_service = file_service
        self._intent_service = intent_service
        self._lfaa_service = lfaa_service
        self._investigation_service = investigation_service
        self._settings = settings

        self._whitelist_validator = (
            whitelist_validator if whitelist_validator is not None else get_whitelist_validator()
        )
        self._blacklist_validator = (
            blacklist_validator if blacklist_validator is not None else get_blacklist_validator()
        )
        self._auto_approved_validator = (
            auto_approved_validator
            if auto_approved_validator is not None
            else get_auto_approved_validator()
        )
        self._init_logic(settings)

    @property
    def investigation_service(self) -> InvestigationServiceProtocol:
        return self._investigation_service

    def _init_logic(self, settings: G8eeAppSettings) -> None:
        self._cv = settings.command_validation
        self._be = settings.batch_execution
        logger.info(
            "OperatorCommandService initialized with PLATFORM DEFAULTS (per-user overrides apply) - whitelisting: %s, blacklisting: %s, auto-approve: %s",
            "ENABLED" if self._cv.enable_whitelisting else "DISABLED",
            "ENABLED" if self._cv.enable_blacklisting else "DISABLED",
            "ENABLED" if self._cv.enable_auto_approve else "DISABLED",
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
        investigation_service: InvestigationServiceProtocol,
        settings: G8eeAppSettings,
        ai_response_analyzer: AIResponseAnalyzerProtocol,
        internal_http_client: G8eClientProtocol,
        approval_service: ApprovalServiceProtocol,
        gateway_operator_client: GatewayOperatorClient | None = None,
        whitelist_validator: CommandWhitelistValidator | None = None,
        blacklist_validator: CommandBlacklistValidator | None = None,
        auto_approved_validator: CommandAutoApprovedValidator | None = None,
    ) -> OperatorCommandService:
        """Construct, wire, and return a fully-initialised OperatorCommandService."""
        lfaa_service = OperatorLFAAService(
            gateway_operator_client=gateway_operator_client,
        )

        execution_service = OperatorExecutionService(
            approval_service=approval_service,
            settings=settings,
            ai_response_analyzer=ai_response_analyzer,
            investigation_service=investigation_service,
            gateway_operator_client=gateway_operator_client,
        )

        filesystem_service = OperatorFilesystemService(
            execution_service=execution_service,
            investigation_service=investigation_service,
        )

        port_service = OperatorPortService(
            execution_service=execution_service,
        )

        file_service = OperatorFileService(
            approval_service=approval_service,
            execution_service=execution_service,
            ai_response_analyzer=ai_response_analyzer,
            investigation_service=investigation_service,
        )

        intent_service = OperatorIntentService(
            approval_service=approval_service,
            execution_service=execution_service,
            investigation_service=investigation_service,
            internal_http_client=internal_http_client,
        )

        return cls(
            approval_service=approval_service,
            execution_service=execution_service,
            filesystem_service=filesystem_service,
            port_service=port_service,
            file_service=file_service,
            intent_service=intent_service,
            lfaa_service=lfaa_service,
            investigation_service=investigation_service,
            settings=settings,
            whitelist_validator=whitelist_validator,
            blacklist_validator=blacklist_validator,
            auto_approved_validator=auto_approved_validator,
        )

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
        dispatches correlated by a batch_id. For N==1 the return shape matches the single-operator response.
        """
        command = args.command.strip()
        justification_parts = [args.request.strip()] if args.request else []
        if args.guidelines and args.guidelines.strip():
            justification_parts.append(f"Guidelines: {args.guidelines.strip()}")
        justification = (
            " | ".join(justification_parts)
            if justification_parts
            else "(no justification provided)"
        )

        logger.info("[COMMAND] Starting execution: %s", command)

        # 1. Resolve target operator(s) - unified path for singular and batch.
        operator_documents = investigation.operator_documents if investigation else []
        try:
            target_operator_docs = self._resolve_targets(operator_documents, args)
        except (ValidationError, BusinessLogicError, ValueError) as e:
            logger.error("[COMMAND] Operator resolution failed: %s", e, exc_info=True)
            return CommandExecutionResult(
                success=False,
                error=f"Operator resolution failed: {e}. Ensure at least one operator is online and has a valid session, then retry.",
                error_type=CommandErrorType.G8E_RESOLUTION_ERROR,
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

        target_systems: list[TargetSystem] = self._execution_service.build_target_systems_list(
            target_operator_docs
        )
        is_batch = len(target_operator_docs) > 1

        primary = target_operator_docs[0]

        # 2. Command validation (L1Doctrine technical bedrock: whitelist/blacklist/forbidden patterns)
        cv = request_settings.command_validation if request_settings else self._cv
        whitelist_override = parse_command_csv(cv.whitelisted_commands)
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
            logger.warning(
                "[COMMAND] Technical safety validation failed: %s", safety_result.error_message
            )
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

        per_operator_exec_ids = [generate_command_execution_id() for _ in target_operator_docs]
        approval_execution_id = (
            per_operator_exec_ids[0] if not is_batch else generate_command_execution_id()
        )

        csv_auto_approve_override = parse_command_csv(cv.auto_approved_commands)
        base_command = command.split()[0] if command else ""
        auto_approve_result = self._auto_approved_validator.is_auto_approved(
            command, extra_commands=csv_auto_approve_override
        )
        is_auto_approved = (
            cv.enable_auto_approve and base_command != "" and auto_approve_result.is_auto_approved
        )
        if is_auto_approved:
            logger.info(
                "[COMMAND] Base command %r is auto-approved (rule=%s, reason=%s) - bypassing human approval: %s",
                base_command,
                auto_approve_result.rule,
                auto_approve_result.reason,
                command,
            )

        # 3. Approval gate - a single approval covers the whole batch.
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
                    risk_analysis=args.risk_analysis,
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
                error_type=CommandErrorType.APPROVAL_DENIED
                if not approval_result.feedback
                else CommandErrorType.USER_FEEDBACK,
                approval_id=approval_result.approval_id,
            )

        # 4. Fan-out dispatch - one execution_id per operator, bounded concurrency.
        max_concurrency = self._be.max_concurrency
        fail_fast = self._be.fail_fast
        semaphore = asyncio.Semaphore(max_concurrency)
        cancel_event = asyncio.Event()

        async def _dispatch(op: OperatorDocument, exec_id: str) -> BatchOperatorExecutionResult:
            op_id = op.id
            op_session_id = op.operator_session_id or ""
            hostname = (
                op.current_hostname
                or (
                    op.latest_heartbeat_snapshot.system_identity.hostname
                    if op.latest_heartbeat_snapshot
                    else None
                )
                or op_id
            )

            if cancel_event.is_set():
                return BatchOperatorExecutionResult(
                    hostname=hostname,
                    operator_id=op_id,
                    execution_id=exec_id,
                    success=False,
                    error="Cancelled by fail-fast",
                )

            async with semaphore:
                if cancel_event.is_set():
                    return BatchOperatorExecutionResult(
                        hostname=hostname,
                        operator_id=op_id,
                        execution_id=exec_id,
                        success=False,
                        error="Cancelled by fail-fast",
                    )

                g8e_message = G8eMessage(
                    id=exec_id,
                    source_component=G8EE_COMPONENT,
                    event_type=EventType.OPERATOR_COMMAND_REQUESTED,
                    case_id=g8e_context.case_id,
                    task_id=AITaskId.COMMAND,
                    investigation_id=g8e_context.investigation_id,
                    web_session_id=g8e_context.web_session_id,
                    user_id=g8e_context.user_id,
                    cli_session_id=g8e_context.cli_session_id,
                    operator_session_id=op_session_id,
                    operator_id=op_id,
                    payload=CommandRequestPayload(
                        command=command,
                        execution_id=exec_id,
                        justification=justification,
                        timeout_seconds=args.timeout_seconds,
                    ),
                )

                try:
                    internal_result, _ = await self._execution_service.execute(
                        g8e_message=g8e_message,
                        g8e_context=g8e_context,
                        timeout_seconds=args.timeout_seconds,
                    )
                    if internal_result is None:
                        raise BusinessLogicError(
                            "Execution service returned None for internal_result", component="g8ee"
                        )
                except Exception as e:
                    logger.exception("[COMMAND] Per-operator dispatch failed on %s: %s", op_id, e)
                    if fail_fast:
                        cancel_event.set()
                    return BatchOperatorExecutionResult(
                        hostname=hostname,
                        operator_id=op_id,
                        execution_id=exec_id,
                        success=False,
                        error=f"Command execution failed: {e}. Check operator status and retry.",
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

        logger.info(
            "[COMMAND] Dispatching to %d operator(s) (batch_id=%s)",
            len(target_operator_docs),
            batch_id,
        )
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
            marshal_risk=args.risk_analysis.risk_level if args.risk_analysis else None,
        )

    # ------------------------------------------------------------------
    # Helpers
    # ------------------------------------------------------------------

    def _resolve_targets(
        self,
        operator_documents: list[OperatorDocument],
        args: ExecutorCommandArgs,
    ) -> list[OperatorDocument]:
        """Resolve target_operators to operator documents."""
        return self._execution_service.resolve_operators(operator_documents, args.target_operators)

    def _assemble_result(
        self,
        *,
        command: str,
        justification: str,
        per_operator_results: list[BatchOperatorExecutionResult],
        approval_id: str | None,
        is_batch: bool,
        batch_id: str | None,
        marshal_risk: RiskLevel | None = None,
    ) -> CommandExecutionResult:
        """Collapse per-operator results into a single CommandExecutionResult.

        For N==1 we populate the single-operator fields (output/stderr/exit_code at top
        level). For N>1 we additionally populate batch fields and a combined output with
        per-host headers so the agent can reason about divergence.
        """
        successful = [r for r in per_operator_results if r.success]
        failed = [r for r in per_operator_results if not r.success]
        internal_results: list[CommandInternalResult] = [
            r.result for r in per_operator_results if r.result is not None
        ]

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
                marshal_risk=marshal_risk,
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
            marshal_risk=marshal_risk,
        )

    async def execute_file_edit(
        self,
        args: FileEditRequestPayload,
        g8e_context: G8eHttpContext,
        investigation: EnrichedInvestigationContext,
        request_settings: G8eeUserSettings | None = None,
    ) -> FileEditResult:
        return await self._file_service.execute_file_edit(
            args, g8e_context, investigation, request_settings=request_settings
        )

    async def execute_port_check(
        self,
        args: CheckPortRequestPayload,
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
    ) -> PortCheckToolResult:
        return await self._port_service.execute_port_check(
            args, investigation, g8e_context=g8e_context
        )

    async def execute_fs_list(
        self,
        args: FsListRequestPayload,
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
    ) -> FsListToolResult:
        return await self._filesystem_service.execute_fs_list(
            args, investigation, g8e_context=g8e_context
        )

    async def execute_fs_grep(
        self,
        args: FsGrepRequestPayload,
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
    ) -> FsGrepToolResult:
        return await self._filesystem_service.execute_fs_grep(
            args, investigation, g8e_context=g8e_context
        )

    async def execute_file_read(
        self,
        args: FsReadRequestPayload,
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
    ) -> FsReadToolResult:
        return await self._filesystem_service.execute_file_read(
            args, investigation, g8e_context=g8e_context
        )

    async def execute_intent_permission_request(
        self,
        args: GrantIntentArgs,
        g8e_context: G8eHttpContext,
        investigation: EnrichedInvestigationContext,
    ) -> IntentPermissionResult:
        return await self._intent_service.execute_intent_permission_request(
            args=args, g8e_context=g8e_context, investigation=investigation
        )

    async def execute_intent_revocation(
        self,
        args: RevokeIntentArgs,
        g8e_context: G8eHttpContext,
        investigation: EnrichedInvestigationContext,
    ) -> IntentPermissionResult:
        return await self._intent_service.execute_intent_revocation(
            args=args, g8e_context=g8e_context, investigation=investigation
        )

    async def execute_fetch_file_history(
        self,
        args: FetchFileHistoryRequestPayload,
        g8e_context: G8eHttpContext,
        investigation: EnrichedInvestigationContext,
    ) -> FetchFileHistoryToolResult:
        return await self._file_service.execute_fetch_file_history(args, g8e_context, investigation)

    async def execute_fetch_file_diff(
        self,
        args: FetchFileDiffRequestPayload,
        g8e_context: G8eHttpContext,
        investigation: EnrichedInvestigationContext,
    ) -> FetchFileDiffToolResult:
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
