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
from app.constants.events import EventType
from app.services.ai.tool_registry import OPERATOR_TOOLS, get_tool_spec
from app.constants.settings import (
    DEFAULT_OS_NAME,
    DEFAULT_SHELL,
    DEFAULT_WORKING_DIRECTORY,
    EXECUTION_ID_PREFIX,
    ToolDisplayCategory,
    TribunalFallbackReason,
    StreamChunkFromModelType,
)
from app.llm.llm_types import ToolCall
from app.models.agent import (
    OperatorCommandArgs,
    OperatorCommandToolSchema,
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
    CommandGenerationResult,
    TribunalFallbackPayload,
    TribunalDisabledError,
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
    tribunal_result: CommandGenerationResult | None = field(default=None)


logger = logging.getLogger(__name__)


_FALLBACK_DISPLAY: tuple[str, str, ToolDisplayCategory] = (
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
        label, icon, category = _FALLBACK_DISPLAY
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

    is_operator_tool = tool_name in OPERATOR_TOOLS
    tool_schema: OperatorCommandToolSchema | None = None
    gen_result: CommandGenerationResult | None = None

    if tool_name == OperatorToolName.RUN_COMMANDS:
        tool_schema = OperatorCommandToolSchema.model_validate(raw_args)
        request = (tool_schema.request or "").strip()
        guidelines = (tool_schema.guidelines or "").strip()

        if request:
            logger.info(
                "[TRIBUNAL-INVOKE] run_commands_with_operator detected: request_len=%d guidelines_len=%d target_operator=%s",
                len(request), len(guidelines), tool_schema.target_operator,
            )
            logger.info(
                "[TRIBUNAL-INVOKE] Request settings: llm_command_gen_enabled=%s llm_command_gen_verifier=%s llm_command_gen_passes=%d assistant_model=%s eval_judge_model=%s",
                request_settings.llm.llm_command_gen_enabled,
                request_settings.llm.llm_command_gen_verifier,
                request_settings.llm.llm_command_gen_passes,
                request_settings.llm.assistant_model,
                request_settings.eval_judge.model,
            )

            op_context = extract_operator_context_by_target(
                investigation,
                tool_schema.target_operator,
            )
            os_name = (op_context.os if op_context else None) or DEFAULT_OS_NAME
            shell = (op_context.shell if op_context else None) or DEFAULT_SHELL
            working_directory = (
                op_context.working_directory if op_context else None
            ) or DEFAULT_WORKING_DIRECTORY
            username = op_context.username if op_context else None
            uid = op_context.uid if op_context else None
            user_context = f"{username} (uid={uid})" if username and uid is not None else (username or "unknown")

            logger.info(
                "[TRIBUNAL-INVOKE] Operator context: os=%s shell=%s working_dir=%s user=%s username=%s uid=%s hostname=%s arch=%s",
                os_name, shell, working_directory, user_context, username, uid,
                op_context.hostname if op_context else None,
                op_context.architecture if op_context else None,
            )

            # Fetch command constraints for Tribunal
            whitelisting_enabled = False
            blacklisting_enabled = False
            whitelisted_commands: list[str] = []
            blacklisted_commands: list[dict[str, str]] = []

            cv = tool_executor.user_settings
            if cv:
                whitelisting_enabled = cv.command_validation.enable_whitelisting if cv else False
                blacklisting_enabled = cv.command_validation.enable_blacklisting if cv else False
                if whitelisting_enabled:
                    whitelisted_commands = sorted(tool_executor.whitelist_validator.all_commands)
                if blacklisting_enabled:
                    blacklisted_commands = tool_executor.blacklist_validator.get_forbidden_commands()

            logger.info(
                "[TRIBUNAL-INVOKE] Command constraints: whitelisting=%s blacklisting=%s whitelist_count=%d blacklist_count=%d",
                whitelisting_enabled, blacklisting_enabled,
                len(whitelisted_commands), len(blacklisted_commands),
            )

            try:
                gen_result = await generate_command(
                    request=request,
                    guidelines=guidelines,
                    operator_context=op_context,
                    g8ed_event_service=g8ed_event_service,
                    web_session_id=g8e_context.web_session_id,
                    user_id=g8e_context.user_id,
                    case_id=g8e_context.case_id,
                    investigation_id=investigation.id,
                    settings=request_settings,
                    whitelisting_enabled=whitelisting_enabled,
                    blacklisting_enabled=blacklisting_enabled,
                    whitelisted_commands=whitelisted_commands,
                    blacklisted_commands=blacklisted_commands,
                )
                logger.info(
                    "[TRIBUNAL-RESULT] generate_command completed: request=%r final=%r outcome=%s",
                    request[:80],
                    gen_result.final_command[:80] if gen_result else None,
                    gen_result.outcome if gen_result else None,
                )
            except TribunalDisabledError as exc:
                logger.error(
                    "[TRIBUNAL-ERROR] Tribunal is disabled — Sage cannot execute commands: %s",
                    exc,
                )
                return _tribunal_error_result(
                    tool_name=tool_name,
                    request=request,
                    error_msg=str(exc),
                )
            except TribunalSystemError as exc:
                logger.error(
                    "[TRIBUNAL-ERROR] Tribunal system error — halting command execution: %s",
                    exc.pass_errors,
                )
                return _tribunal_error_result(
                    tool_name=tool_name,
                    request=request,
                    error_msg=f"Tribunal system error: {'; '.join(exc.pass_errors)}",
                )
            except TribunalProviderUnavailableError as exc:
                logger.error(
                    "[TRIBUNAL-ERROR] Tribunal provider unavailable — halting command execution: %s",
                    exc.error,
                )
                await g8ed_event_service.publish_investigation_event(
                    investigation_id=investigation.id,
                    event_type=EventType.TRIBUNAL_SESSION_FALLBACK_TRIGGERED,
                    payload=TribunalFallbackPayload(
                        reason=TribunalFallbackReason.PROVIDER_UNAVAILABLE,
                        request=request,
                        error=f"Provider unavailable ({exc.provider}): {exc.error}",
                    ),
                    web_session_id=g8e_context.web_session_id,
                    case_id=g8e_context.case_id,
                    user_id=g8e_context.user_id,
                )
                return _tribunal_error_result(
                    tool_name=tool_name,
                    request=request,
                    error_msg=f"Tribunal provider unavailable ({exc.provider}): {exc.error}",
                )
            except TribunalGenerationFailedError as exc:
                logger.error(
                    "[TRIBUNAL-ERROR] Tribunal generation failed — halting command execution: %s",
                    exc.pass_errors,
                )
                return _tribunal_error_result(
                    tool_name=tool_name,
                    request=request,
                    error_msg=f"Tribunal generation failed: {'; '.join(exc.pass_errors)}",
                )
            except TribunalVerifierFailedError as exc:
                logger.error(
                    "[CMD_GEN] Tribunal verifier failed — halting command execution: %s",
                    exc.error,
                )
                return _tribunal_error_result(
                    tool_name=tool_name,
                    request=request,
                    error_msg=f"Tribunal verifier failed ({exc.reason}): {exc.error}",
                )

            logger.info(
                "[CMD_GEN] Tribunal produced command: outcome=%s request=%r final=%r",
                gen_result.outcome, request[:80], gen_result.final_command[:80],
            )
            # Inject the Tribunal-produced command into the args the executor sees.
            # Sage never writes `command` — it is the Tribunal's authoritative output.
            raw_args["command"] = gen_result.final_command
            raw_args["request"] = request
            raw_args["guidelines"] = guidelines

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

    command_display = gen_result.final_command if gen_result else (
        tool_schema.request if tool_schema else ""
    )

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
        tribunal_result=gen_result,
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
            tool_call_id=fc.id,
        ))

    logger.info("[SEQ_EXEC] Completed %d tool call(s)", num_calls)
    result_out.append(responses)
