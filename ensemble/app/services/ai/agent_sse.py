# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
SSE delivery - translates StreamChunkFromModel events produced by the agent
streaming loop into client EventService pub/sub calls for browser delivery.
"""

import asyncio
import logging
from collections.abc import AsyncGenerator, Awaitable, Callable
from typing import cast

from app.constants import (
    DEFAULT_FINISH_REASON,
    UNKNOWN_ERROR_MESSAGE,
    EventType,
    StreamChunkFromModelType,
    ThinkingPhase,
    ToolCallStatus,
)
from app.constants.generated_status import OperatorToolName
from app.errors import ValidationError
from app.models.agent import (
    AgentInputs,
    AgentStreamState,
    StreamChunkFromModel,
)
from app.models.base import G8eBaseModel
from app.models.evaluation_trace import EvaluationProviderToolRejection
from app.models.events import (
    AiProcessingStoppedPayload,
    AIToolLifecyclePayload,
    ChatCitationsReadyPayload,
    ChatErrorPayload,
    ChatProcessingStartedPayload,
    ChatResponseChunkPayload,
    ChatResponseCompletePayload,
    ChatRetryPayload,
    ChatThinkingPayload,
    ChatTurnCompletePayload,
)
from app.models.tool_results import (
    CommandConstraintsResult,
    CommandExecutionResult,
    FetchFileDiffToolResult,
    FetchLogsToolResult,
    FileEditResult,
    FsGrepToolResult,
    FsListToolResult,
    FsReadToolResult,
    InvestigationContextResult,
    PortCheckToolResult,
    SearchWebResult,
    SshInventoryToolResult,
)
from app.services.ai.tool_registry import AI_UNIVERSAL_TOOLS
from app.services.evaluation.tool_evidence import (
    record_tool_call_completed,
    record_tool_call_started,
)
from app.services.evaluation.trace_service import EvaluationTraceService
from app.services.observe.payloads import (
    agent_state_sequence,
    build_agent_state_request,
    build_investigation_run_state_request,
    resolve_chat_persona_id,
)
from app.services.protocols import EventServiceProtocol
from app.utils.time_ids.ids import generate_command_execution_id
from app.utils.time_ids.timestamp import now

logger = logging.getLogger(__name__)


def _make_event_publisher(event_service, has_sse, investigation_id, web_session_id, cli_session_id, case_id, user_id):
    """Bind stream routing values to a best-effort event publisher."""
    async def publish(event_type: EventType, payload: G8eBaseModel) -> None:
        if not has_sse:
            return
        try:
            await event_service.publish_investigation_event(
                investigation_id=investigation_id,
                event_type=event_type,
                payload=payload,
                web_session_id=web_session_id,
                cli_session_id=cli_session_id,
                case_id=case_id,
                user_id=user_id,
            )
        except Exception as exc:
            logger.warning("[SSE] Push failed for %s (non-blocking): %s", event_type, exc)
    return publish


def _make_state_pushers(event_service, inputs, investigation_id, run_display_name, user_id, web_session_id, cli_session_id):
    """Create best-effort publishers for agent and investigation run state."""
    persona_id = resolve_chat_persona_id(inputs.active_agent)

    async def push_agent_state(status: str) -> None:
        if persona_id is None:
            return
        for reported_status in agent_state_sequence(status):
            request = build_agent_state_request(
                user_id=user_id,
                persona_id=persona_id,
                status=reported_status,
                run_id=investigation_id,
                model=inputs.model_to_use,
                web_session_id=web_session_id,
                cli_session_id=cli_session_id,
            )
            if request is None:
                return
            try:
                await event_service.publish_agent_state(request)
            except asyncio.CancelledError:
                raise
            except Exception as exc:
                logger.warning("[SSE] observe agent-state push failed (non-blocking): %s", exc)
                return

    async def push_run_state(status: str) -> None:
        request = build_investigation_run_state_request(
            run_id=investigation_id,
            display_name=run_display_name,
            status=status,
            user_id=user_id,
            web_session_id=web_session_id,
            cli_session_id=cli_session_id,
        )
        if request is None:
            return
        try:
            await event_service.publish_run_state(request)
        except asyncio.CancelledError:
            raise
        except Exception as exc:
            logger.warning("[SSE] observe run-state push failed (non-blocking): %s", exc)

    return push_agent_state, push_run_state


async def deliver_via_sse(
    stream: AsyncGenerator[StreamChunkFromModel],
    inputs: AgentInputs,
    state: AgentStreamState,
    event_service: EventServiceProtocol,
    on_iteration_text: Callable[[str], Awaitable[None]] | None = None,
    evaluation_trace_service: EvaluationTraceService | None = None,
) -> None:
    """
    Consume a StreamChunkFromModel async generator and deliver each event to
    the browser via EventService HTTP push.

    TEXT chunks are pushed as CHAT_RESPONSE_CHUNK events immediately.
    COMPLETE pushes chat.response_complete.
    All other chunk types are translated to their corresponding SSE events.

    ``inputs`` carries the immutable request-scoped context (investigation,
    session, agent mode). ``state`` is the sole mutable sink: response text,
    token usage, finish reason, and grounding metadata are written here and
    read back by the chat pipeline after the run completes.

    If ``on_iteration_text`` is provided, it is awaited with the accumulated
    ``state.response_text`` each time a tool iteration ends (TOOL_RESULT chunk),
    before the buffer is cleared. This allows callers to persist intermediate
    AI commentary produced before each tool call, so conversation_history
    retains the agent's running narrative on restore.
    """
    if not inputs.investigation_id:
        raise ValidationError(
            "investigation_id is required for deliver_via_sse",
            field="investigation_id",
            constraint="required",
        )

    investigation_id: str = inputs.investigation_id
    web_session_id: str | None = inputs.web_session_id
    cli_session_id: str | None = inputs.g8e_context.cli_session_id if inputs.g8e_context else None
    user_id: str = inputs.user_id or ""
    agent_mode = inputs.agent_mode

    # Standard flows have web sessions or CLI sessions for SSE delivery.
    # If both session IDs are None, we still process the stream to populate
    # state.response_text but skip EventService publishing.
    has_sse = web_session_id is not None or cli_session_id is not None

    # For test/eval flows without a case ID, still allow event delivery.
    case_id = (inputs.case_id or "") if has_sse else ""

    _publish = _make_event_publisher(
        event_service, has_sse, investigation_id, web_session_id, cli_session_id, case_id, user_id
    )

    # Observe producer: resolve the active persona for agent-state projections.
    # If the persona cannot be identified from the request, agent updates are
    # omitted and that path is recorded as unsupported.
    _persona_id = resolve_chat_persona_id(inputs.active_agent)
    _run_display_name = ""
    if inputs.investigation:
        _run_display_name = inputs.investigation.case_title or ""

    _push_agent_state, _push_run_state = _make_state_pushers(
        event_service,
        inputs,
        investigation_id,
        _run_display_name,
        user_id,
        web_session_id,
        cli_session_id,
    )

    if has_sse:
        logger.info(
            "[SSE] Starting delivery: investigation_id=%s case_id=%s user_id=%s workflow=%s sentinel_mode=%s",
            investigation_id,
            case_id,
            user_id,
            agent_mode,
            inputs.sentinel_mode,
        )
    else:
        logger.info(
            "[SSE] Starting delivery (no-session flow, no SSE): investigation_id=%s user_id=%s workflow=%s sentinel_mode=%s",
            investigation_id,
            user_id,
            agent_mode,
            inputs.sentinel_mode,
        )
    logger.info("[SSE] Async generator iteration starting")

    # Emit iteration started event to signal AI processing has begun
    await _publish(
        EventType.AI_LLM_CHAT_ITERATION_STARTED,
        ChatProcessingStartedPayload(agent_mode=agent_mode),
    )
    # Agent and investigation run enter running state when the chat iteration starts.
    await _push_agent_state("running")
    await _push_run_state("running")

    await _run_sse_delivery(
        stream, inputs, state, _publish, _push_agent_state, _push_run_state,
        on_iteration_text, investigation_id, agent_mode, has_sse, case_id
    )


async def _run_sse_delivery(
    stream, inputs, state, publish, push_agent_state, push_run_state,
    on_iteration_text, investigation_id, agent_mode, has_sse, case_id
) -> None:
    """Consume a stream and publish its completion or terminal error event."""
    try:
        error_occurred = await _consume_sse_stream(
            stream, inputs, state, publish, push_agent_state, push_run_state,
            on_iteration_text, investigation_id, agent_mode, has_sse, case_id
        )

        # Read final aggregate values from the mutable stream state
        grounding_metadata = state.grounding_metadata
        token_usage = state.token_usage
        has_citations = bool(grounding_metadata and grounding_metadata.grounding_used)

        # Skip completion event if error already occurred
        if error_occurred:
            logger.info("[SSE] Skipping completion event due to prior error")
        else:
            await publish(
                EventType.AI_LLM_CHAT_ITERATION_TEXT_COMPLETED,
                ChatResponseCompletePayload(
                    content=state.response_text,
                    finish_reason=state.finish_reason or DEFAULT_FINISH_REASON,
                    has_citations=has_citations,
                    grounding_metadata=grounding_metadata.model_dump(mode="json")
                    if grounding_metadata
                    else {},
                    token_usage=token_usage.model_dump(mode="json") if token_usage else {},
                    model_calls=state.model_calls,
                    scrubbing_observations=inputs.scrubbing_observations,
                    agent_mode=agent_mode,
                ),
            )
            # Agent and run complete when the persona's turn work finishes.
            # The investigation run is NOT marked terminal here; a later chat
            # turn on the same investigation refreshes it as running.
            await push_agent_state("completed")
            await push_run_state("running")

        logger.info(
            "[SSE] Complete: investigation_id=%s has_citations=%s "
            "finish_reason=%s response_chars=%d",
            investigation_id,
            has_citations,
            state.finish_reason,
            len(state.response_text),
        )
        logger.info("[SSE] Async generator iteration completed")

    except asyncio.CancelledError:
        logger.info("[SSE] Cancelled for investigation %s", investigation_id)
        # Emit STOPPED event instead of FAILED when processing is cancelled
        await publish(
            EventType.AI_LLM_CHAT_ITERATION_STOPPED,
            AiProcessingStoppedPayload(
                reason="AI processing stopped",
                timestamp=now(),
            ),
        )
        # Agent enters idle on cancellation; the run stays non-terminal.
        await push_agent_state("idle")
        raise

    except Exception as e:
        logger.error("[SSE] Error: %s", e)
        await publish(
            EventType.AI_LLM_CHAT_ITERATION_FAILED,
            ChatErrorPayload(error=str(e)),
        )
        # Agent enters failed on an unexpected terminal exception.
        state.stream_failed = True
        state.error = str(e)
        await push_agent_state("failed")


async def _handle_tool_call_chunk(
    chunk: StreamChunkFromModel, state: AgentStreamState, inputs: AgentInputs,
    _publish, _push_agent_state, _push_run_state
) -> None:
    """Handle one tool call chunk."""
    fn = chunk.data.tool_name or ""
    exec_id = chunk.data.execution_id or generate_command_execution_id()
    chunk.data.execution_id = exec_id

    # Track tool call in state for metadata recording
    state.tool_call_count += 1
    if fn and fn not in state.tool_types_used:
        state.tool_types_used.append(fn)
    record_tool_call_started(state, inputs.g8e_context, chunk.data)

    # For universal tools, emit the new native lifecycle event.
    # Operator-gated tools are handled by their respective services.
    if fn in AI_UNIVERSAL_TOOLS:
        event_type = None
        query = None
        port = None
        host = None

        if fn == OperatorToolName.QUERY_INVESTIGATION_CONTEXT:
            event_type = EventType.AI_LLM_TOOL_G8E_INVESTIGATION_QUERY_REQUESTED
            if chunk.data.arguments and "query" in chunk.data.arguments:
                query = str(chunk.data.arguments["query"])
            elif (
                isinstance(chunk.data.result, SearchWebResult)
                and chunk.data.result.query
            ):
                query = chunk.data.result.query
        elif fn == OperatorToolName.GET_COMMAND_CONSTRAINTS:
            event_type = EventType.AI_LLM_TOOL_G8E_COMMAND_CONSTRAINTS_REQUESTED
        elif fn == OperatorToolName.G8E_SEARCH_WEB:
            event_type = EventType.AI_LLM_TOOL_G8E_WEB_SEARCH_REQUESTED
            if chunk.data.arguments and "query" in chunk.data.arguments:
                query = str(chunk.data.arguments["query"])
            elif (
                isinstance(chunk.data.result, SearchWebResult)
                and chunk.data.result.query
            ):
                query = chunk.data.result.query

        if event_type:
            await _publish(
                event_type,
                AIToolLifecyclePayload(
                    tool_name=fn,
                    display_label=chunk.data.display_label,
                    display_icon=chunk.data.display_icon,
                    display_detail=chunk.data.display_detail,
                    category=chunk.data.category,
                    execution_id=exec_id,
                    status=ToolCallStatus.STARTED,
                    query=query,
                    port=port,
                    host=host,
                    timestamp=now().isoformat(),
                ),
            )
            # Agent and run enter waiting while a universal tool is executing.
            await _push_agent_state("waiting")
            await _push_run_state("waiting")
    else:
        await _publish(
            EventType.OPERATOR_COMMAND_STARTED,
            AIToolLifecyclePayload(
                tool_name=fn,
                display_label=chunk.data.display_label,
                display_icon=chunk.data.display_icon,
                display_detail=chunk.data.display_detail,
                category=chunk.data.category,
                execution_id=exec_id,
                status=ToolCallStatus.STARTED,
                timestamp=now().isoformat(),
            ),
        )
        await _push_agent_state("waiting")
        await _push_run_state("waiting")


async def _handle_universal_tool_result(
    chunk: StreamChunkFromModel, fn: str, exec_id: str, publish, push_agent_state, push_run_state
) -> None:
    """Publish completion details for a universal tool result."""
    result = chunk.data.result
    event_type, content, results, error = _universal_tool_result_details(fn, result)
    error = str(chunk.data.error) if chunk.data.error else error
    is_failed = _tool_result_failed(chunk, error)
    if event_type:
        await publish(
            event_type,
            AIToolLifecyclePayload(
                tool_name=fn,
                display_label=chunk.data.display_label,
                display_icon=chunk.data.display_icon,
                display_detail=chunk.data.display_detail,
                category=chunk.data.category,
                execution_id=exec_id,
                status=ToolCallStatus.FAILED if is_failed else ToolCallStatus.COMPLETED,
                content=content,
                results=results,
                error=error,
                timestamp=now().isoformat(),
            ),
        )
        await push_agent_state("running")
        await push_run_state("running")


def _universal_tool_result_details(fn: str, result):
    """Return the event and result fields for a completed universal tool."""
    handlers = {
        OperatorToolName.QUERY_INVESTIGATION_CONTEXT: (
            EventType.AI_LLM_TOOL_G8E_INVESTIGATION_QUERY_COMPLETED,
            InvestigationContextResult,
            lambda value: str(value.data) if value.data is not None else None,
        ),
        OperatorToolName.GET_COMMAND_CONSTRAINTS: (
            EventType.AI_LLM_TOOL_G8E_COMMAND_CONSTRAINTS_COMPLETED,
            CommandConstraintsResult,
            lambda value: value.message,
        ),
        OperatorToolName.G8E_SEARCH_WEB: (
            EventType.AI_LLM_TOOL_G8E_WEB_SEARCH_COMPLETED,
            SearchWebResult,
            lambda value: [item.model_dump(mode="json") for item in value.results],
        ),
    }
    handler = next((handlers[key] for key in handlers if fn == key), None)
    if handler is None:
        return None, None, None, None
    event_type, result_type, content_for = handler
    if not isinstance(result, result_type):
        return event_type, None, None, None
    content = content_for(result) if result_type is not SearchWebResult else None
    results = content_for(result) if result_type is SearchWebResult else None
    error = result.error if not result.success else None
    return event_type, content, results, error


async def _handle_operator_tool_result(
    chunk: StreamChunkFromModel, fn: str, exec_id: str, publish, push_agent_state, push_run_state
) -> None:
    """Publish completion details for an operator tool result."""
    error = str(chunk.data.error) if chunk.data.error else None
    content = None if error or chunk.data.result is None else _operator_result_content(chunk.data.result)
    if not error and chunk.data.result is not None and not chunk.data.result.success:
        error = str(chunk.data.result.error) if chunk.data.result.error else None
    is_failed = _tool_result_failed(chunk, error)
    await publish(
        EventType.OPERATOR_COMMAND_FAILED if is_failed else EventType.OPERATOR_COMMAND_COMPLETED,
        AIToolLifecyclePayload(
            tool_name=fn,
            display_label=chunk.data.display_label,
            display_icon=chunk.data.display_icon,
            display_detail=chunk.data.display_detail,
            category=chunk.data.category,
            execution_id=exec_id,
            status=ToolCallStatus.FAILED if is_failed else ToolCallStatus.COMPLETED,
            content=content,
            error=error,
            timestamp=now().isoformat(),
        ),
    )
    await push_agent_state("running")
    await push_run_state("running")


def _operator_result_content(result):
    """Convert a typed operator result to the content shown in its SSE event."""
    if not result.success and result.error:
        return None
    return _operator_result_content_value(result)


_NO_OPERATOR_CONTENT = object()


def _simple_operator_result_content(result):
    handlers = (
        (CommandExecutionResult, lambda value: value.output),
        ((FileEditResult, FsReadToolResult), lambda value: value.content),
        (FetchLogsToolResult, lambda value: value.stdout),
        (FetchFileDiffToolResult, lambda value: value.diff.diff_content if value.diff else None),
        (InvestigationContextResult, lambda value: str(value.data) if value.data is not None else None),
        (CommandConstraintsResult, lambda value: value.message),
    )
    handler = next((render for types, render in handlers if isinstance(result, types)), None)
    return handler(result) if handler else _NO_OPERATOR_CONTENT


def _operator_result_content_value(result):
    content = _simple_operator_result_content(result)
    if content is not _NO_OPERATOR_CONTENT:
        return cast(str | None, content)
    if isinstance(result, PortCheckToolResult):
        return f"Port {result.port} on {result.host} is {'open' if result.is_open else 'closed'}"
    if isinstance(result, (FsListToolResult, FsGrepToolResult, SshInventoryToolResult, SearchWebResult)):
        return str(result.model_dump(mode="json"))
    return None


def _tool_result_failed(chunk: StreamChunkFromModel, error: str | None) -> bool:
    """Return whether the stream metadata marks a tool result as failed."""
    return bool(
        error
        or chunk.data.status == ToolCallStatus.FAILED
        or chunk.data.success is False
        or (chunk.data.result is not None and not chunk.data.result.success)
    )


async def _handle_tool_result_chunk(
    chunk: StreamChunkFromModel, state: AgentStreamState, inputs: AgentInputs,
    _publish, _push_agent_state, _push_run_state, on_iteration_text, turn: int
) -> int:
    """Handle one tool result chunk."""
    _turn = turn
    exec_id = chunk.data.execution_id or generate_command_execution_id()
    chunk.data.execution_id = exec_id
    fn = chunk.data.tool_name or ""
    record_tool_call_completed(state, inputs.g8e_context, chunk.data)

    if fn in AI_UNIVERSAL_TOOLS:
        await _handle_universal_tool_result(
            chunk, fn, exec_id, _publish, _push_agent_state, _push_run_state
        )
    else:
        await _handle_operator_tool_result(
            chunk, fn, exec_id, _publish, _push_agent_state, _push_run_state
        )

    _turn += 1

    await _publish(
        EventType.AI_LLM_CHAT_ITERATION_COMPLETED,
        ChatTurnCompletePayload(turn=_turn),
    )

    if on_iteration_text and state.response_text.strip():
        try:
            await on_iteration_text(state.response_text)
        except Exception as persist_err:
            logger.warning(
                "[SSE] on_iteration_text callback failed: %s",
                persist_err,
                exc_info=True,
            )
    state.response_text = ""

    return _turn


async def _handle_stream_update_chunk(chunk, inputs, state, publish, thinking_started: bool) -> bool:
    """Handle text, thinking, retry, citation, and completion chunks."""
    if chunk.type == StreamChunkFromModelType.TEXT:
        state.response_text += chunk.data.content or ""
        await publish(
            EventType.AI_LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
            ChatResponseChunkPayload(content=chunk.data.content or ""),
        )
    elif chunk.type == StreamChunkFromModelType.THINKING:
        phase = ThinkingPhase.START if not thinking_started else ThinkingPhase.UPDATE
        thinking_started = True
        await publish(
            EventType.AI_LLM_CHAT_ITERATION_THINKING_STARTED,
            ChatThinkingPayload(thinking=chunk.data.thinking, phase=phase),
        )
    elif chunk.type == StreamChunkFromModelType.THINKING_END:
        thinking_started = False
        await publish(
            EventType.AI_LLM_CHAT_ITERATION_THINKING_STARTED,
            ChatThinkingPayload(thinking=None, phase=ThinkingPhase.END),
        )
    elif chunk.type == StreamChunkFromModelType.RETRY:
        attempt = chunk.data.attempt or 0
        max_attempts = chunk.data.max_attempts or 0
        logger.info("[SSE] RETRY chunk: attempt=%d max_attempts=%d", attempt, max_attempts)
        await publish(
            EventType.AI_LLM_CHAT_ITERATION_RETRY,
            ChatRetryPayload(attempt=attempt, max_attempts=max_attempts),
        )
    elif chunk.type == StreamChunkFromModelType.CITATIONS:
        grounding_metadata = chunk.data.grounding_metadata
        state.grounding_metadata = grounding_metadata
        if grounding_metadata and grounding_metadata.grounding_used:
            await publish(
                EventType.AI_LLM_CHAT_ITERATION_CITATIONS_RECEIVED,
                ChatCitationsReadyPayload(
                    grounding_metadata=grounding_metadata.model_dump(mode="json")
                ),
            )
    elif chunk.type == StreamChunkFromModelType.COMPLETE:
        state.token_usage = chunk.data.token_usage
        state.model_calls = chunk.data.model_calls
        state.finish_reason = chunk.data.finish_reason
        state.tool_turn_limit_reached = bool(chunk.data.tool_turn_limit_reached)
        if chunk.data.tool_response_sizes:
            state.tool_response_sizes = chunk.data.tool_response_sizes
        logger.info(
            "[SSE] COMPLETE chunk received: finish_reason=%s response_chars=%d",
            chunk.data.finish_reason,
            len(state.response_text),
        )
        if chunk.data.token_usage:
            logger.info("[TOKEN_USAGE] SSE final: %s", chunk.data.token_usage)
    return thinking_started


async def _handle_model_error_chunk(
    chunk, inputs, state, publish, push_agent_state, investigation_id, agent_mode, has_sse, case_id
) -> None:
    """Record and publish a terminal model error chunk."""
    error_message = chunk.data.error or UNKNOWN_ERROR_MESSAGE
    error_extra = {"investigation_id": investigation_id, "agent_mode": agent_mode}
    if has_sse:
        error_extra["case_id"] = case_id
    logger.exception("[SSE] LLM provider error: %s", error_message, extra=error_extra)
    if chunk.data.model_calls:
        state.model_calls = chunk.data.model_calls
    if chunk.data.provider_tool_rejection and inputs.model_to_use:
        state.provider_tool_rejection = EvaluationProviderToolRejection(
            model=inputs.model_to_use, reason=error_message
        )
    await publish(EventType.AI_LLM_CHAT_ITERATION_FAILED, ChatErrorPayload(error=error_message))
    await push_agent_state("failed")
    state.stream_failed = True
    state.error = error_message


async def _consume_sse_stream(
    stream, inputs, state, publish, push_agent_state, push_run_state,
    on_iteration_text, investigation_id, agent_mode, has_sse, case_id
) -> bool:
    """Consume stream chunks, publish events, and return whether a model error occurred."""
    _turn = 0
    _thinking_started = False
    error_occurred = False
    async for chunk in stream:
        if chunk.type == StreamChunkFromModelType.TOOL_CALL:
            await _handle_tool_call_chunk(chunk, state, inputs, publish, push_agent_state, push_run_state)
        elif chunk.type == StreamChunkFromModelType.TOOL_RESULT:
            _turn = await _handle_tool_result_chunk(chunk, state, inputs, publish, push_agent_state, push_run_state, on_iteration_text, _turn)
        elif chunk.type == StreamChunkFromModelType.ERROR:
            await _handle_model_error_chunk(
                chunk, inputs, state, publish, push_agent_state,
                investigation_id, agent_mode, has_sse, case_id
            )
            error_occurred = True
            break  # Break instead of return to ensure post-loop code executes
        else:
            _thinking_started = await _handle_stream_update_chunk(
                chunk, inputs, state, publish, _thinking_started
            )

    return error_occurred
