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

"""
Function call execution — tool display metadata, grounding merge, single
tool call dispatch, and sequential turn-level execution loop.
"""

import asyncio
import logging
from collections.abc import AsyncGenerator
from dataclasses import dataclass, field

from pydantic import ValidationError

from app.constants import (
    CommandErrorType,
    EventType,
    OperatorToolName,
)
from app.services.ai.tool_registry import OPERATOR_TOOLS, get_tool_spec
from app.constants.settings import (
    DEFAULT_OS_NAME,
    DEFAULT_SHELL,
    DEFAULT_WORKING_DIRECTORY,
    ToolDisplayCategory,
    StreamChunkFromModelType,
)
from app.llm.llm_types import ToolCall
from app.models.agent import (
    ExecutorCommandArgs,
    SageOperatorRequest,
    ToolCallResponse,
    StreamChunkData,
    StreamChunkFromModel,
)

from app.services.ai.generator import generate_command
from app.models.grounding import GroundingMetadata
from app.models.http_context import G8eHttpContext
from app.models.investigations import EnrichedInvestigationContext
from app.models.reputation import StakeResolutionPayload
from app.models.tool_results import (
    CommandExecutionResult,
    ToolResult,
    SearchWebResult,
)
from app.models.settings import G8eeUserSettings

from app.models.agents.tribunal import (
    CommandGenerationResult,
    TribunalError,
)
from app.services.investigation.investigation_service import extract_operator_context_by_target, extract_single_operator_context
from app.services.ai.tool_service import AIToolService
from app.services.infra.event_service import EventService
from app.utils.ids import generate_command_execution_id
from app.utils.safety import map_os_string_to_platform
from app.utils.csv_commands import parse_command_csv
from app.models.whitelist import WhitelistedCommand

class TribunalInvoker:
    """Encapsulates Tribunal invocation logic for run_commands_with_operator.

    Converts a Sage-facing SageOperatorRequest into the executor-facing
    ExecutorCommandArgs by running the Tribunal pipeline to produce a command.
    """

    @staticmethod
    def _fetch_command_constraints(
        request_settings: G8eeUserSettings,
        investigation: EnrichedInvestigationContext,
        tool_executor: AIToolService,
    ) -> tuple[bool, bool, list[WhitelistedCommand], list[dict[str, str]]]:
        """Fetch command validation constraints from request settings.
        
        Returns metadata-rich command list with safe_options and validation patterns.
        """
        whitelisting_enabled = False
        blacklisting_enabled = False
        whitelisted_commands: list[WhitelistedCommand] = []
        blacklisted_commands: list[dict[str, str]] = []

        cv = request_settings
        if cv:
            whitelisting_enabled = cv.command_validation.enable_whitelisting
            blacklisting_enabled = cv.command_validation.enable_blacklisting
            if whitelisting_enabled:
                # Map OS string to Platform enum using centralized function
                primary_operator = investigation.operator_documents[0] if investigation.operator_documents else None
                operator_context = extract_single_operator_context(primary_operator) if primary_operator else None
                os_name = operator_context.os if operator_context else DEFAULT_OS_NAME
                platform = map_os_string_to_platform(os_name)

                # Check if CSV override is active
                csv_override = cv.command_validation.whitelisted_commands
                csv_commands = parse_command_csv(csv_override) if csv_override else []
                if csv_commands:
                    # CSV mode: construct simple metadata from CSV commands (no rich safe_options/validation)
                    whitelisted_commands = [WhitelistedCommand(command=cmd) for cmd in csv_commands]
                else:
                    # JSON mode: use rich metadata from validator
                    whitelisted_commands = tool_executor.whitelist_validator.get_available_commands_with_metadata(platform)
            if blacklisting_enabled:
                blacklisted_commands = tool_executor.blacklist_validator.get_forbidden_commands()

        return whitelisting_enabled, blacklisting_enabled, whitelisted_commands, blacklisted_commands

    @staticmethod
    async def run(
        sage_request: SageOperatorRequest,
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
        event_service: EventService,
        request_settings: G8eeUserSettings,
        tool_executor: AIToolService,
    ) -> tuple[ExecutorCommandArgs, CommandGenerationResult]:
        """Invoke Tribunal pipeline and return executor args with generated command.

        Raises TribunalError subclasses when the Tribunal cannot produce a command;
        the caller is responsible for converting these into a failed ToolCallResult.
        """
        request = (sage_request.request or "").strip()
        guidelines = (sage_request.guidelines or "").strip()
        # Use first target operator as context anchor for Tribunal
        target_anchor = sage_request.target_operators[0] if sage_request.target_operators else None
        op_context = extract_operator_context_by_target(
            investigation, target_anchor,
        )

        logger.info(
            "[TRIBUNAL-INVOKE] Operator context: os=%s shell=%s working_dir=%s username=%s uid=%s hostname=%s arch=%s",
            (op_context.os if op_context else None) or DEFAULT_OS_NAME,
            (op_context.shell if op_context else None) or DEFAULT_SHELL,
            (op_context.working_directory if op_context else None) or DEFAULT_WORKING_DIRECTORY,
            op_context.username if op_context else None,
            op_context.uid if op_context else None,
            op_context.hostname if op_context else None,
            op_context.architecture if op_context else None,
        )

        whitelisting_enabled, blacklisting_enabled, whitelisted_commands, blacklisted_commands = (
            TribunalInvoker._fetch_command_constraints(request_settings, investigation, tool_executor)
        )

        logger.info(
            "[TRIBUNAL-INVOKE] Command constraints: whitelisting=%s blacklisting=%s whitelist_count=%d blacklist_count=%d",
            whitelisting_enabled, blacklisting_enabled,
            len(whitelisted_commands), len(blacklisted_commands),
        )

        investigation_context_parts = [p for p in [investigation.case_title, investigation.case_description] if p]
        investigation_context = " | ".join(investigation_context_parts)

        gen_result = await generate_command(
            request=request,
            guidelines=guidelines,
            operator_context=op_context,
            event_service=event_service,
            web_session_id=g8e_context.web_session_id or "",
            user_id=g8e_context.user_id or "",
            case_id=g8e_context.case_id or "",
            investigation_id=investigation.id,
            settings=request_settings,
            reputation_data_service=tool_executor.reputation_data_service,
            auditor_hmac_key=tool_executor.auditor_hmac_key,
            ai_response_analyzer=tool_executor.ai_response_analyzer,
            investigation_state=investigation.current_state,
            investigation_context=investigation_context,
            whitelisting_enabled=whitelisting_enabled,
            blacklisting_enabled=blacklisting_enabled,
            whitelisted_commands=whitelisted_commands,
            blacklisted_commands=blacklisted_commands,
        )
        logger.info(
            "[CMD_GEN] Tribunal produced command: outcome=%s request=%r final=%r",
            gen_result.outcome, request[:80], gen_result.final_command[:80] if gen_result.final_command else None,
        )

        executor_args = ExecutorCommandArgs(
            command=gen_result.final_command,
            request=request,
            guidelines=guidelines,
            target_operators=sage_request.target_operators,
            expected_output_lines=sage_request.expected_output_lines,
            timeout_seconds=sage_request.timeout_seconds,
            correlation_id=gen_result.correlation_id,
            risk_analysis=gen_result.warden_risk_analysis, # Pass risk analysis to executor
        )
        return executor_args, gen_result


@dataclass
class ToolCallResult:
    """Internal pipeline carrier for a single dispatched tool call."""
    tool_name: str
    call_info: StreamChunkData
    result_info: StreamChunkData
    result: ToolResult
    grounding: GroundingMetadata | None = field(default=None)
    tribunal_result: CommandGenerationResult | None = field(default=None)


logger = logging.getLogger(__name__)


_UNKNOWN_TOOL_DISPLAY: tuple[str, str, ToolDisplayCategory] = (
    "Processing", "sync", ToolDisplayCategory.GENERAL,
)


def tool_display_metadata(
    tool_name: str,
    display_detail: str,
) -> tuple[str, str, str, ToolDisplayCategory]:
    """Return ``(display_label, display_icon, display_detail, category)`` for a tool call.

    Display fields live on the tool's ``ToolSpec`` (the single-source registry)
    so adding a new tool cannot silently miss display metadata. Unknown names
    (e.g. dynamically-registered grounding stubs) fall back to a generic entry.
    """
    spec = get_tool_spec(tool_name)
    if spec is None:
        label, icon, category = _UNKNOWN_TOOL_DISPLAY
    else:
        label, icon, category = spec.display_label, spec.display_icon, spec.display_category
    return label, icon, display_detail, category


def merge_grounding(
    existing: GroundingMetadata | None,
    new: GroundingMetadata,
) -> GroundingMetadata:
    """
    Merge two GroundingMetadata instances, combining all sources from both.

    When multiple search_web calls happen in the same turn each produces its
    own GroundingMetadata. This function accumulates all web_search_queries,
    grounding_chunks, grounding_supports, and sources so that the final
    CITATIONS chunk reflects every search call made in that turn.
    """
    if existing is None:
        return new
    return GroundingMetadata(
        grounding_used=existing.grounding_used or new.grounding_used,
        source=new.source,
        web_search_queries=existing.web_search_queries + new.web_search_queries,
        search_queries_count=existing.search_queries_count + new.search_queries_count,
        grounding_chunks=existing.grounding_chunks + new.grounding_chunks,
        sources_count=existing.sources_count + new.sources_count,
        grounding_supports=existing.grounding_supports + new.grounding_supports,
        citations_count=existing.citations_count + new.citations_count,
        search_entry_point=new.search_entry_point or existing.search_entry_point,
        sources=existing.sources + new.sources,
        error=new.error or existing.error,
    )


def _tribunal_error_result(
    tool_name: str,
    request: str,
    error_msg: str,
) -> ToolCallResult:
    """Build a failed ToolCallResult when the Tribunal cannot produce a command.

    Sage never proposes a command, so we surface the original request string
    (truncated if long) as the display detail so the UI and the LLM can see
    what Sage asked for.
    """
    error_result = CommandExecutionResult(
        success=False,
        error=error_msg,
        error_type=CommandErrorType.EXECUTION_ERROR,
    )
    display_detail = request if len(request) <= 200 else request[:200] + "..."
    return ToolCallResult(
        tool_name=tool_name,
        call_info=StreamChunkData(
            tool_name=tool_name,
            execution_id=None,
            command=display_detail,
            is_operator_tool=True,
        ),
        result_info=StreamChunkData(
            execution_id=None,
            success=False,
            result=error_result,
            error_type=CommandErrorType.EXECUTION_ERROR,
        ),
        result=error_result,
    )


async def orchestrate_tool_execution(
    tool_call: ToolCall,
    tool_executor: AIToolService,
    investigation: EnrichedInvestigationContext,
    g8e_context: G8eHttpContext,
    event_service: EventService,
    request_settings: G8eeUserSettings,
) -> ToolCallResult:
    """
    Dispatch a single tool call through the Tribunal refinement pipeline and
    AIToolExecutor, returning a fully typed ToolCallResult.

    Type safety is enforced at the tool_service boundary which validates
    raw dict args to typed models internally.
    """
    tool_name = tool_call.name or ""
    raw_args: dict[str, object] = dict(tool_call.args) if tool_call.args else {}

    logger.info(
        "[AGENT] Dispatching function: name=%s args_keys=%s",
        tool_name, list(raw_args.keys()),
    )

    is_operator_tool = tool_name in OPERATOR_TOOLS
    sage_request: SageOperatorRequest | None = None
    gen_result: CommandGenerationResult | None = None

    try:
        if tool_name == OperatorToolName.RUN_COMMANDS:
            sage_request = SageOperatorRequest.model_validate(raw_args)
            request = (sage_request.request or "").strip()

            if request:
                logger.info(
                    "[TRIBUNAL-INVOKE] run_commands_with_operator detected: request_len=%d guidelines_len=%d target_operators=%s",
                    len(request), len(sage_request.guidelines or ""), sage_request.target_operators,
                )
                logger.info(
                    "[TRIBUNAL-INVOKE] Request settings: llm_command_gen_enabled=%s llm_command_gen_auditor=%s llm_command_gen_passes=%d assistant_model=%s eval_judge_model=%s",
                    request_settings.llm.llm_command_gen_enabled,
                    request_settings.llm.llm_command_gen_auditor,
                    request_settings.llm.llm_command_gen_passes,
                    request_settings.llm.assistant_model,
                    request_settings.eval_judge.model,
                )

                try:
                    executor_args, gen_result = await TribunalInvoker.run(
                        sage_request=sage_request,
                        investigation=investigation,
                        g8e_context=g8e_context,
                        event_service=event_service,
                        request_settings=request_settings,
                        tool_executor=tool_executor,
                    )
                except (TribunalError, ValidationError) as exc:
                    error_msg = exc.user_message if isinstance(exc, TribunalError) else str(exc)
                    logger.error(
                        "[TRIBUNAL-ERROR] %s (%s): %s",
                        type(exc).__name__, tool_name, error_msg,
                    )
                    return _tribunal_error_result(
                        tool_name=tool_name,
                        request=request,
                        error_msg=error_msg,
                    )

                raw_args = executor_args.model_dump(by_alias=True)

        execution_id = generate_command_execution_id()

        result = await tool_executor.execute_tool_call(
            tool_name,
            raw_args,
            investigation,
            g8e_context,
            request_settings=request_settings,
            execution_id=execution_id,
        )
    except asyncio.CancelledError:
        logger.info(
            "[ORCHESTRATE-TOOL] Tool execution cancelled for %s",
            tool_name
        )
        raise

    if (
        gen_result is not None
        and tool_name == OperatorToolName.RUN_COMMANDS
        and isinstance(result, CommandExecutionResult)
    ):
        # Reset warden block count on successful command (new turn starts fresh)
        if result.success and investigation and investigation.current_state:
            investigation.current_state.warden_block_count = 0
            logger.info("[WARDEN-CIRCUIT-BREAKER] Reset warden block count for investigation=%s after successful command", investigation.id)

        # Schedule fire-and-forget reputation resolution
        async def _resolve_and_emit():
            try:
                warden_blocked = result.error_type == CommandErrorType.RISK_ANALYSIS_BLOCKED
                res = await tool_executor.reputation_service.resolve_stakes(
                    tribunal_command_id=gen_result.correlation_id,
                    investigation_id=investigation.id,
                    gen_result=gen_result,
                    execution_result=result,
                    warden_risk=result.warden_risk,
                    warden_blocked=warden_blocked,
                )

                for outcome in res.resolutions:
                    payload = StakeResolutionPayload.model_validate(outcome.model_dump())
                    await event_service.publish_reputation_event(
                        EventType.AI_REPUTATION_STATE_UPDATED,
                        payload,
                        g8e_context
                    )

                    if outcome.slash_tier:
                        slash_event = getattr(EventType, f"AI_REPUTATION_SLASH_TIER{outcome.slash_tier.value}")
                        await event_service.publish_reputation_event(
                            slash_event,
                            payload,
                            g8e_context
                        )
            except Exception as e:
                logger.error("[REPUTATION] Failed to resolve stakes: %s", e, exc_info=True)

        task_id = f"reputation_resolution_{execution_id}"
        task = asyncio.create_task(_resolve_and_emit(), name=task_id)
        tool_executor.chat_task_manager.track_detached(task_id, task)

    logger.info(
        "[AGENT] Function result: name=%s success=%s execution_id=%s error_type=%s",
        tool_name, result.success, execution_id, result.error_type,
    )

    command_display = gen_result.final_command if gen_result else (
        sage_request.request if sage_request else ""
    )

    display_label, display_icon, display_detail, category = tool_display_metadata(
        tool_name, command_display or ""
    )

    return ToolCallResult(
        tool_name=tool_name,
        call_info=StreamChunkData(
            tool_name=tool_name,
            execution_id=execution_id,
            command=command_display,
            is_operator_tool=is_operator_tool,
            display_label=display_label,
            display_icon=display_icon,
            display_detail=display_detail or "",
            category=category,
        ),
        result_info=StreamChunkData(
            execution_id=execution_id,
            success=result.success,
            result=result,
            error_type=result.error_type if not result.success and hasattr(result, "error_type") else None,
        ),
        result=result,
        tribunal_result=gen_result,
    )


async def execute_turn_tool_calls(
    pending_tool_calls: list[ToolCall],
    tool_executor: AIToolService,
    investigation: EnrichedInvestigationContext,
    g8e_context: G8eHttpContext,
    result_out: list[list[ToolCallResponse]],
    request_settings: G8eeUserSettings,
    event_service: EventService,
) -> AsyncGenerator[StreamChunkFromModel]:
    """
    Execute all tool calls from one turn.

    If parallel execution is enabled in settings, tool calls are dispatched
    concurrently. Otherwise, they are executed sequentially.

    Yields TOOL_CALL and TOOL_RESULT StreamChunkFromModel chunks for
    each call. On completion appends the list of ToolCallResponse records
    to result_out (always exactly one item — a list of responses for the turn).
    """
    num_calls = len(pending_tool_calls)
    use_parallel = request_settings.llm.llm_parallel_tool_calls

    if use_parallel:
        logger.info("[PARALLEL_EXEC] Executing %d tool call(s) in parallel", num_calls)
        async for chunk in _execute_parallel(
            pending_tool_calls,
            tool_executor,
            investigation,
            g8e_context,
            result_out,
            request_settings,
            event_service,
        ):
            yield chunk
    else:
        logger.info("[SEQ_EXEC] Executing %d tool call(s) sequentially", num_calls)
        async for chunk in _execute_sequential(
            pending_tool_calls,
            tool_executor,
            investigation,
            g8e_context,
            result_out,
            request_settings,
            event_service,
        ):
            yield chunk


async def _process_single_tool_call(
    idx: int,
    fc: ToolCall,
    tool_executor: AIToolService,
    investigation: EnrichedInvestigationContext,
    g8e_context: G8eHttpContext,
    request_settings: G8eeUserSettings,
    event_service: EventService,
) -> ToolCallResult:
    """Helper to execute a single tool call with error handling."""
    try:
        tool_result = await orchestrate_tool_execution(
            tool_call=fc,
            tool_executor=tool_executor,
            investigation=investigation,
            g8e_context=g8e_context,
            event_service=event_service,
            request_settings=request_settings,
        )
    except asyncio.CancelledError:
        raise
    except Exception as exc:
        logger.error("[TOOL_EXEC] Function call %d (%s) failed: %s", idx, fc.name, exc)
        _exc_result = CommandExecutionResult(
            success=False,
            error=str(exc),
        )
        tool_result = ToolCallResult(
            tool_name=fc.name or "",
            call_info=StreamChunkData(
                tool_name=fc.name,
                execution_id=None,
                command="",
                is_operator_tool=False,
            ),
            result_info=StreamChunkData(
                execution_id=None,
                success=False,
                result=_exc_result,
                error_type=CommandErrorType.EXECUTION_ERROR,
            ),
            result=_exc_result,
        )

    if (
        fc.name == OperatorToolName.G8E_SEARCH_WEB
        and isinstance(tool_result.result, SearchWebResult)
        and tool_executor.web_search_provider is not None
    ):
        g8e_web_search_grounding = tool_executor.web_search_provider.build_g8e_web_search_grounding(
            tool_result.result
        )
        if g8e_web_search_grounding.grounding_used:
            tool_result = ToolCallResult(
                tool_name=tool_result.tool_name,
                call_info=tool_result.call_info,
                result_info=tool_result.result_info,
                result=tool_result.result,
                grounding=g8e_web_search_grounding,
            )

    return tool_result


async def _execute_sequential(
    pending_tool_calls: list[ToolCall],
    tool_executor: AIToolService,
    investigation: EnrichedInvestigationContext,
    g8e_context: G8eHttpContext,
    result_out: list[list[ToolCallResponse]],
    request_settings: G8eeUserSettings,
    event_service: EventService,
) -> AsyncGenerator[StreamChunkFromModel]:
    """Execute tool calls one by one."""
    responses: list[ToolCallResponse] = []
    for i, fc in enumerate(pending_tool_calls):
        tool_result = await _process_single_tool_call(
            i, fc, tool_executor, investigation, g8e_context, request_settings, event_service
        )

        yield StreamChunkFromModel(
            type=StreamChunkFromModelType.TOOL_CALL,
            data=tool_result.call_info,
        )
        yield StreamChunkFromModel(
            type=StreamChunkFromModelType.TOOL_RESULT,
            data=tool_result.result_info,
        )

        flattened = tool_result.result.model_dump(mode="json")
        responses.append(ToolCallResponse(
            tool_name=tool_result.tool_name,
            flattened_response=flattened,
            grounding=tool_result.grounding,
            tool_call_id=fc.id,
        ))

    result_out.append(responses)


async def _execute_parallel(
    pending_tool_calls: list[ToolCall],
    tool_executor: AIToolService,
    investigation: EnrichedInvestigationContext,
    g8e_context: G8eHttpContext,
    result_out: list[list[ToolCallResponse]],
    request_settings: G8eeUserSettings,
    event_service: EventService,
) -> AsyncGenerator[StreamChunkFromModel]:
    """Execute tool calls concurrently using asyncio.gather."""
    tasks = [
        _process_single_tool_call(
            i, fc, tool_executor, investigation, g8e_context, request_settings, event_service
        )
        for i, fc in enumerate(pending_tool_calls)
    ]

    # Execute all tool calls in parallel
    results: list[ToolCallResult] = await asyncio.gather(*tasks)

    responses: list[ToolCallResponse] = []
    for fc, tool_result in zip(pending_tool_calls, results):
        # Yield status chunks for UI
        yield StreamChunkFromModel(
            type=StreamChunkFromModelType.TOOL_CALL,
            data=tool_result.call_info,
        )
        yield StreamChunkFromModel(
            type=StreamChunkFromModelType.TOOL_RESULT,
            data=tool_result.result_info,
        )

        flattened = tool_result.result.model_dump(mode="json")
        responses.append(ToolCallResponse(
            tool_name=tool_result.tool_name,
            flattened_response=flattened,
            grounding=tool_result.grounding,
            tool_call_id=fc.id,
        ))

    result_out.append(responses)
