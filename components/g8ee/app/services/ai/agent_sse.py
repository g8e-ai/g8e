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

"""
SSE delivery — translates StreamChunkFromModel events produced by the agent
streaming loop into g8ed EventService pub/sub calls for browser delivery.
"""

import asyncio
import logging
from collections.abc import AsyncGenerator, Awaitable, Callable

from app.constants import (
    DEFAULT_FINISH_REASON,
    EventType,
    ToolCallStatus,
    UNKNOWN_ERROR_MESSAGE,
    StreamChunkFromModelType,
    ThinkingActionType,
)
from app.models.agent import (
    AgentInputs,
    AgentStreamState,
    StreamChunkFromModel,
)
from app.models.base import G8eBaseModel
from app.models.g8ed_client import (
    ChatCitationsReadyPayload,
    ChatErrorPayload,
    ChatProcessingStartedPayload,
    ChatResponseChunkPayload,
    ChatResponseCompletePayload,
    ChatRetryPayload,
    ChatThinkingPayload,
    ChatToolCallPayload,
    ChatTurnCompletePayload,
)
from app.errors import ValidationError
from app.services.infra.g8ed_event_service import EventService

logger = logging.getLogger(__name__)


async def deliver_via_sse(
    stream: AsyncGenerator[StreamChunkFromModel, None],
    inputs: AgentInputs,
    state: AgentStreamState,
    g8ed_event_service: EventService,
    on_iteration_text: Callable[[str], Awaitable[None]] | None = None,
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
            field="investigation_id", constraint="required",
        )
    if not inputs.web_session_id:
        raise ValidationError(
            "web_session_id is required for deliver_via_sse",
            field="web_session_id", constraint="required",
        )
    if not inputs.case_id:
        raise ValidationError(
            "case_id is required for deliver_via_sse",
            field="case_id", constraint="required",
        )
    case_id: str = inputs.case_id
    investigation_id: str = inputs.investigation_id
    web_session_id: str = inputs.web_session_id
    user_id: str = inputs.user_id or ""
    agent_mode = inputs.agent_mode

    async def _publish(event_type: EventType, payload: G8eBaseModel) -> None:
        """Publish an investigation event with the stream's fixed routing tuple.

        Centralizes the (investigation_id, web_session_id, case_id, user_id)
        binding so new events can't accidentally drop a routing field.
        """
        await g8ed_event_service.publish_investigation_event(
            investigation_id=investigation_id,
            event_type=event_type,
            payload=payload,
            web_session_id=web_session_id,
            case_id=case_id,
            user_id=user_id,
        )

    logger.info(
        "[SSE] Starting delivery: investigation_id=%s case_id=%s user_id=%s workflow=%s sentinel_mode=%s",
        investigation_id, case_id, user_id, agent_mode, inputs.sentinel_mode,
    )
    logger.info("[SSE] Async generator iteration starting")

    # Emit iteration started event to signal AI processing has begun
    await _publish(
        EventType.LLM_CHAT_ITERATION_STARTED,
        ChatProcessingStartedPayload(agent_mode=agent_mode),
    )

    _turn = 0
    _thinking_started = False

    # Initialize grounding and token_usage to avoid UnboundLocalError
    grounding_metadata = None
    token_usage = None
    error_occurred = False

    try:
        async for chunk in stream:
            if chunk.type == StreamChunkFromModelType.TEXT:
                state.response_text += chunk.data.content or ""
                await _publish(
                    EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
                    ChatResponseChunkPayload(content=chunk.data.content),
                )

            elif chunk.type == StreamChunkFromModelType.THINKING:
                # Determine action type based on whether this is the first thinking chunk
                action_type = ThinkingActionType.START if not _thinking_started else ThinkingActionType.UPDATE
                _thinking_started = True

                await _publish(
                    EventType.LLM_CHAT_ITERATION_THINKING_STARTED,
                    ChatThinkingPayload(
                        thinking=chunk.data.thinking,
                        action_type=action_type,
                    ),
                )

            elif chunk.type == StreamChunkFromModelType.THINKING_END:
                _thinking_started = False
                # Emit thinking event with END action to indicate thinking phase is complete
                await _publish(
                    EventType.LLM_CHAT_ITERATION_THINKING_STARTED,
                    ChatThinkingPayload(
                        thinking=None,
                        action_type=ThinkingActionType.END,
                    ),
                )

            elif chunk.type == StreamChunkFromModelType.RETRY:
                attempt = chunk.data.attempt or 0
                max_attempts = chunk.data.max_attempts or 0
                logger.info(
                    "[SSE] RETRY chunk: attempt=%d max_attempts=%d",
                    attempt, max_attempts,
                )
                await _publish(
                    EventType.LLM_CHAT_ITERATION_RETRY,
                    ChatRetryPayload(
                        attempt=attempt,
                        max_attempts=max_attempts,
                    ),
                )

            elif chunk.type == StreamChunkFromModelType.TOOL_CALL:
                fn = chunk.data.tool_name or ""
                exec_id = chunk.data.execution_id

                # Track tool call in state for metadata recording
                state.tool_call_count += 1
                if fn and fn not in state.tool_types_used:
                    state.tool_types_used.append(fn)

                # Emit generic tool call started event for all tools
                await _publish(
                    EventType.LLM_CHAT_ITERATION_TOOL_CALL_STARTED,
                    ChatToolCallPayload(
                        tool_name=fn,
                        display_label=chunk.data.display_label,
                        display_icon=chunk.data.display_icon,
                        display_detail=chunk.data.display_detail,
                        category=chunk.data.category,
                        execution_id=exec_id,
                        status=ToolCallStatus.STARTED,
                    ),
                )

                # No per-tool sidecar REQUESTED events are emitted here. The generic
                # LLM_CHAT_ITERATION_TOOL_CALL_STARTED event (above) carries the
                # display metadata for every tool and owns the UI indicator
                # lifecycle. Port check STARTED / COMPLETED / FAILED are emitted
                # separately by port_service and keep their dedicated sidecar path.

            elif chunk.type == StreamChunkFromModelType.TOOL_RESULT:
                exec_id = chunk.data.execution_id
                fn = chunk.data.tool_name or ""

                # Emit generic tool call completed event for all tools
                await _publish(
                    EventType.LLM_CHAT_ITERATION_TOOL_CALL_COMPLETED,
                    ChatToolCallPayload(
                        tool_name=fn,
                        display_label=chunk.data.display_label,
                        display_icon=chunk.data.display_icon,
                        display_detail=chunk.data.display_detail,
                        category=chunk.data.category,
                        execution_id=exec_id,
                        status=ToolCallStatus.COMPLETED,
                    ),
                )

                _turn += 1
                await _publish(
                    EventType.LLM_CHAT_ITERATION_COMPLETED,
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

            elif chunk.type == StreamChunkFromModelType.CITATIONS:
                state.grounding_metadata = chunk.data.grounding_metadata
                grounding_metadata = chunk.data.grounding_metadata
                if grounding_metadata and grounding_metadata.grounding_used:
                    await _publish(
                        EventType.LLM_CHAT_ITERATION_CITATIONS_RECEIVED,
                        ChatCitationsReadyPayload(
                            grounding_metadata=grounding_metadata.model_dump(mode="json"),
                        ),
                    )

            elif chunk.type == StreamChunkFromModelType.COMPLETE:
                state.token_usage = chunk.data.token_usage
                token_usage = chunk.data.token_usage
                state.finish_reason = chunk.data.finish_reason
                logger.info(
                    "[SSE] COMPLETE chunk received: finish_reason=%s response_chars=%d",
                    chunk.data.finish_reason, len(state.response_text),
                )
                if chunk.data.token_usage:
                    logger.info("[TOKEN_USAGE] SSE final: %s", chunk.data.token_usage)

            elif chunk.type == StreamChunkFromModelType.ERROR:
                # Handle LLM provider errors gracefully instead of raising exceptions
                error_message = chunk.data.error or UNKNOWN_ERROR_MESSAGE
                logger.error(
                    "[SSE] LLM provider error: %s",
                    error_message,
                    extra={
                        "investigation_id": investigation_id,
                        "case_id": case_id,
                        "agent_mode": agent_mode,
                    }
                )
                
                # Publish the error event and continue - don't raise exception
                await _publish(
                    EventType.LLM_CHAT_ITERATION_FAILED,
                    ChatErrorPayload(error=error_message),
                )
                error_occurred = True
                break  # Break instead of return to ensure post-loop code executes

        # Read final aggregate values from the mutable stream state
        grounding_metadata = state.grounding_metadata
        token_usage = state.token_usage
        has_citations = bool(grounding_metadata and grounding_metadata.grounding_used)

        # Skip completion event if error already occurred
        if error_occurred:
            logger.info("[SSE] Skipping completion event due to prior error")
        else:
            await _publish(
                EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED,
                ChatResponseCompletePayload(
                    content=state.response_text,
                    finish_reason=state.finish_reason or DEFAULT_FINISH_REASON,
                    has_citations=has_citations,
                    grounding_metadata=grounding_metadata.model_dump(mode="json") if grounding_metadata else {},
                    token_usage=token_usage.model_dump(mode="json") if token_usage else {},
                    agent_mode=agent_mode,
                ),
            )

        logger.info(
            "[SSE] Complete: investigation_id=%s has_citations=%s "
            "finish_reason=%s response_chars=%d",
            investigation_id, has_citations,
            state.finish_reason, len(state.response_text),
        )
        logger.info("[SSE] Async generator iteration completed")

    except asyncio.CancelledError:
        logger.info("[SSE] Cancelled for investigation %s", investigation_id)
        await _publish(
            EventType.LLM_CHAT_ITERATION_FAILED,
            ChatErrorPayload(error="AI processing stopped"),
        )
        raise

    except Exception as e:
        logger.error("[SSE] Error: %s", e, exc_info=True)
        await _publish(
            EventType.LLM_CHAT_ITERATION_FAILED,
            ChatErrorPayload(error=str(e)),
        )
