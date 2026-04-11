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

import logging
import uuid
from collections.abc import AsyncGenerator
from dataclasses import dataclass, field

from app.constants.status import (
    CommandErrorType,
    OperatorToolName,
)
from app.constants.settings import (
    DEFAULT_OS_NAME,
    DEFAULT_SHELL,
    DEFAULT_WORKING_DIRECTORY,
    EXECUTION_ID_PREFIX,
    ToolDisplayCategory,
    StreamChunkFromModelType,
)
from app.llm.llm_types import ToolCall
from app.models.agent import (
    OperatorCommandArgs,
    ToolCallResponse,
    StreamChunkData,
    StreamChunkFromModel,
)

from app.services.ai.command_generator import generate_command
from app.models.grounding import GroundingMetadata
from app.models.http_context import G8eHttpContext
from app.models.investigations import EnrichedInvestigationContext
from app.models.tool_results import CommandExecutionResult, ToolResult, SearchWebResult
from app.models.settings import G8eeUserSettings

from app.models.agents.tribunal import (
    TribunalSystemError,
    TribunalProviderUnavailableError,
    TribunalGenerationFailedError,
    TribunalVerifierFailedError,
)
from app.services.investigation.investigation_service import extract_operator_context_by_target
from app.services.ai.tool_service import AIToolService
from app.services.infra.g8ed_event_service import EventService
from app.utils.timestamp import now, to_timestamp


@dataclass
class ToolCallResult:
    """Internal pipeline carrier for a single dispatched tool call."""
    tool_name: str
    call_info: StreamChunkData
    result_info: StreamChunkData
    result: ToolResult
    grounding: GroundingMetadata | None = field(default=None)


logger = logging.getLogger(__name__)


_TOOL_DISPLAY_METADATA: dict[str, tuple[str, str, ToolDisplayCategory]] = {
    OperatorToolName.RUN_COMMANDS:           ("Executing command",     "terminal",   ToolDisplayCategory.EXECUTION),
    OperatorToolName.FILE_CREATE:            ("Creating file",         "file-plus",  ToolDisplayCategory.FILE),
    OperatorToolName.FILE_WRITE:             ("Writing file",          "file-edit",  ToolDisplayCategory.FILE),
    OperatorToolName.FILE_READ:              ("Reading file",          "file-text",  ToolDisplayCategory.FILE),
    OperatorToolName.FILE_UPDATE:            ("Updating file",         "file-edit",  ToolDisplayCategory.FILE),
    OperatorToolName.LIST_FILES:             ("Listing directory",     "folder",     ToolDisplayCategory.FILE),
    OperatorToolName.READ_FILE_CONTENT:      ("Reading file",          "file-text",  ToolDisplayCategory.FILE),
    OperatorToolName.RESTORE_FILE:           ("Restoring file",        "file-check", ToolDisplayCategory.FILE),
    OperatorToolName.FETCH_FILE_HISTORY:     ("Fetching file history", "history",    ToolDisplayCategory.FILE),
    OperatorToolName.FETCH_FILE_DIFF:        ("Fetching file diff",    "git-diff",   ToolDisplayCategory.FILE),
    OperatorToolName.G8E_SEARCH_WEB:             ("Searching the web",     "search",     ToolDisplayCategory.SEARCH),
    OperatorToolName.CHECK_PORT:             ("Checking port",         "network",    ToolDisplayCategory.NETWORK),
    OperatorToolName.GRANT_INTENT:           ("Requesting permission", "shield",     ToolDisplayCategory.GENERAL),
    OperatorToolName.REVOKE_INTENT:          ("Revoking permission",   "shield-off", ToolDisplayCategory.GENERAL),
    OperatorToolName.FETCH_EXECUTION_OUTPUT: ("Fetching output",       "terminal",   ToolDisplayCategory.GENERAL),
    OperatorToolName.FETCH_SESSION_HISTORY:  ("Fetching history",      "clock",      ToolDisplayCategory.GENERAL),
}


def tool_display_metadata(
    tool_name: str,
    display_detail: str,
) -> tuple[str, str, str, ToolDisplayCategory]:
    """Return (display_label, display_icon, display_detail, category) for a tool call."""
    label, icon, category = _TOOL_DISPLAY_METADATA.get(
        tool_name,
        ("Processing", "sync", ToolDisplayCategory.GENERAL),
    )
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


async def orchestrate_tool_execution(
    tool_call: ToolCall,
    tool_executor: AIToolService,
    investigation: EnrichedInvestigationContext,
    g8e_context: G8eHttpContext,
    g8ed_event_service: EventService,
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

    operator_tools = {member for member in OperatorToolName}
    is_operator_tool = tool_name in operator_tools
    typed_args: OperatorCommandArgs | None = None

    if tool_name == OperatorToolName.RUN_COMMANDS:
        typed_args = OperatorCommandArgs.model_validate(raw_args)
        if typed_args.command:
            original_command = typed_args.command
            intent = typed_args.justification

            op_context = extract_operator_context_by_target(
                investigation,
                typed_args.target_operator,
            )
            os_name = (op_context.os if op_context else None) or DEFAULT_OS_NAME
            shell = (op_context.shell if op_context else None) or DEFAULT_SHELL
            working_directory = (
                op_context.working_directory if op_context else None
            ) or DEFAULT_WORKING_DIRECTORY

            try:
                gen_result = await generate_command(
                    original_command=original_command,
                    intent=intent,
                    os_name=os_name,
                    shell=shell,
                    working_directory=working_directory,
                    g8ed_event_service=g8ed_event_service,
                    web_session_id=g8e_context.web_session_id,
                    user_id=g8e_context.user_id,
                    case_id=g8e_context.case_id,
                    investigation_id=investigation.id,
                    settings=request_settings,
                )
            except TribunalSystemError as exc:
                logger.error(
                    "[CMD_GEN] Tribunal system error — halting command execution: %s",
                    exc.pass_errors,
                )
                return ToolCallResult(
                    tool_name=tool_name,
                    call_info=StreamChunkData(
                        tool_name=tool_name,
                        execution_id=None,
                        command=original_command,
                        is_operator_tool=True,
                    ),
                    result_info=StreamChunkData(
                        execution_id=None,
                        success=False,
                        result=CommandExecutionResult(
                            success=False,
                            error=f"Tribunal system error: {'; '.join(exc.pass_errors)}",
                            error_type=CommandErrorType.EXECUTION_ERROR,
                        ),
                        error_type=CommandErrorType.EXECUTION_ERROR,
                    ),
                    result=CommandExecutionResult(
                        success=False,
                        error=f"Tribunal system error: {'; '.join(exc.pass_errors)}",
                        error_type=CommandErrorType.EXECUTION_ERROR,
                    ),
                )
            except TribunalProviderUnavailableError as exc:
                logger.error(
                    "[CMD_GEN] Tribunal provider unavailable — halting command execution: %s",
                    exc.error,
                )
                return ToolCallResult(
                    tool_name=tool_name,
                    call_info=StreamChunkData(
                        tool_name=tool_name,
                        execution_id=None,
                        command=original_command,
                        is_operator_tool=True,
                    ),
                    result_info=StreamChunkData(
                        execution_id=None,
                        success=False,
                        result=CommandExecutionResult(
                            success=False,
                            error=f"Tribunal provider unavailable ({exc.provider}): {exc.error}",
                            error_type=CommandErrorType.EXECUTION_ERROR,
                        ),
                        error_type=CommandErrorType.EXECUTION_ERROR,
                    ),
                    result=CommandExecutionResult(
                        success=False,
                        error=f"Tribunal provider unavailable ({exc.provider}): {exc.error}",
                        error_type=CommandErrorType.EXECUTION_ERROR,
                    ),
                )
            except TribunalGenerationFailedError as exc:
                logger.error(
                    "[CMD_GEN] Tribunal generation failed — halting command execution: %s",
                    exc.pass_errors,
                )
                return ToolCallResult(
                    tool_name=tool_name,
                    call_info=StreamChunkData(
                        tool_name=tool_name,
                        execution_id=None,
                        command=original_command,
                        is_operator_tool=True,
                    ),
                    result_info=StreamChunkData(
                        execution_id=None,
                        success=False,
                        result=CommandExecutionResult(
                            success=False,
                            error=f"Tribunal generation failed: {'; '.join(exc.pass_errors)}",
                            error_type=CommandErrorType.EXECUTION_ERROR,
                        ),
                        error_type=CommandErrorType.EXECUTION_ERROR,
                    ),
                    result=CommandExecutionResult(
                        success=False,
                        error=f"Tribunal generation failed: {'; '.join(exc.pass_errors)}",
                        error_type=CommandErrorType.EXECUTION_ERROR,
                    ),
                )
            except TribunalVerifierFailedError as exc:
                logger.error(
                    "[CMD_GEN] Tribunal verifier failed — halting command execution: %s",
                    exc.error,
                )
                return ToolCallResult(
                    tool_name=tool_name,
                    call_info=StreamChunkData(
                        tool_name=tool_name,
                        execution_id=None,
                        command=original_command,
                        is_operator_tool=True,
                    ),
                    result_info=StreamChunkData(
                        execution_id=None,
                        success=False,
                        result=CommandExecutionResult(
                            success=False,
                            error=f"Tribunal verifier failed ({exc.reason}): {exc.error}",
                            error_type=CommandErrorType.EXECUTION_ERROR,
                        ),
                        error_type=CommandErrorType.EXECUTION_ERROR,
                    ),
                    result=CommandExecutionResult(
                        success=False,
                        error=f"Tribunal verifier failed ({exc.reason}): {exc.error}",
                        error_type=CommandErrorType.EXECUTION_ERROR,
                    ),
                )

            if gen_result.final_command != original_command:
                logger.info(
                    "[CMD_GEN] Command refined: outcome=%s original=%r final=%r",
                    gen_result.outcome, original_command, gen_result.final_command,
                )
                raw_args["command"] = gen_result.final_command
            else:
                logger.info(
                    "[CMD_GEN] Command unchanged: outcome=%s command=%r",
                    gen_result.outcome, original_command,
                )

    execution_id: str | None = None

    if is_operator_tool:
        execution_id = f"{EXECUTION_ID_PREFIX}_{uuid.uuid4().hex[:12]}_{int(to_timestamp(now()))}"

    tool_args_with_id = {**raw_args}
    if is_operator_tool and execution_id:
        tool_args_with_id["execution_id"] = execution_id
        tool_args_with_id["_web_session_id"] = g8e_context.web_session_id

    result = await tool_executor.execute_tool_call(
        tool_name,
        tool_args_with_id,
        investigation,
        g8e_context,
        request_settings=request_settings,
    )

    logger.info(
        "[AGENT] Function result: name=%s success=%s execution_id=%s error_type=%s",
        tool_name, result.success, execution_id, result.error_type,
    )

    command_display = typed_args.command if typed_args else ""

    display_label, display_icon, display_detail, category = tool_display_metadata(
        tool_name, command_display
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
            display_detail=display_detail,
            category=category,
        ),
        result_info=StreamChunkData(
            execution_id=execution_id,
            success=result.success,
            result=result,
            error_type=result.error_type if not result.success and hasattr(result, "error_type") else None,
        ),
        result=result,
    )


async def execute_turn_tool_calls(
    pending_tool_calls: list[ToolCall],
    tool_executor: AIToolService,
    investigation: EnrichedInvestigationContext,
    g8e_context: G8eHttpContext,
    result_out: list[list[ToolCallResponse]],
    request_settings: G8eeUserSettings,
    g8ed_event_service: EventService,
) -> AsyncGenerator[StreamChunkFromModel, None]:
    """
    Execute all tool calls from one turn sequentially.

    Yields TOOL_CALL and TOOL_RESULT StreamChunkFromModel chunks for
    each call. On completion appends the list of ToolCallResponse records
    to result_out (always exactly one item — a list of responses for the turn).
    """
    responses: list[ToolCallResponse] = []
    num_calls = len(pending_tool_calls)
    logger.info("[SEQ_EXEC] Executing %d tool call(s) sequentially", num_calls)

    for i, fc in enumerate(pending_tool_calls):
        try:
            tool_result = await orchestrate_tool_execution(
                tool_call=fc,
                tool_executor=tool_executor,
                investigation=investigation,
                g8e_context=g8e_context,
                g8ed_event_service=g8ed_event_service,
                request_settings=request_settings,
            )
        except Exception as exc:
            logger.error("[SEQ_EXEC] Function call %d (%s) failed: %s", i, fc.name, exc)
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

        yield StreamChunkFromModel(
            type=StreamChunkFromModelType.TOOL_CALL,
            data=tool_result.call_info,
        )
        yield StreamChunkFromModel(
            type=StreamChunkFromModelType.TOOL_RESULT,
            data=tool_result.result_info,
        )

        flattened = tool_result.result.flatten_for_llm()
        logger.info(
            "[FUNCTION_RESPONSE] %s: success=%s output_len=%d exit_code=%s",
            tool_result.tool_name,
            flattened.get("success"),
            len(flattened.get("output", "")),
            flattened.get("exit_code"),
        )
        responses.append(ToolCallResponse(
            tool_name=tool_result.tool_name,
            flattened_response=flattened,
            grounding=tool_result.grounding,
        ))

    logger.info("[SEQ_EXEC] Completed %d tool call(s)", num_calls)
    result_out.append(responses)
