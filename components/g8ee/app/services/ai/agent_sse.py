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
from collections.abc import AsyncGenerator

from app.constants import (
    DEFAULT_FINISH_REASON,
    EventType,
    ToolCallStatus,
    OperatorToolName,
    UNKNOWN_ERROR_MESSAGE,
    StreamChunkFromModelType,
    ThinkingActionType,
)
from app.models.agent import (
    AgentStreamContext,
    StreamChunkFromModel,
)
from app.models.g8ed_client import (
    AISearchWebPayload,
    ChatCitationsReadyPayload,
    ChatErrorPayload,
    ChatResponseChunkPayload,
    ChatResponseCompletePayload,
    ChatThinkingPayload,
    ChatTurnCompletePayload,
    OperatorNetworkPortCheckPayload,
)
from app.errors import ValidationError
from app.services.infra.g8ed_event_service import EventService

logger = logging.getLogger(__name__)


async def deliver_via_sse(
    stream: AsyncGenerator[StreamChunkFromModel, None],
    agent_streaming_context: AgentStreamContext,
    g8ed_event_service: EventService,
) -> None:
    """
    Consume a StreamChunkFromModel async generator and deliver each event to
    the browser via EventService HTTP push.

    TEXT chunks are pushed as CHAT_RESPONSE_CHUNK events immediately.
    COMPLETE pushes chat.response_complete.
    All other chunk types are translated to their corresponding SSE events.
    """
    if not agent_streaming_context.web_session_id or not agent_streaming_context.case_id:
        raise ValidationError(
            "deliver_via_sse requires web_session_id, case_id, and investigation_id",
            details={"case_id": agent_streaming_context.case_id, "web_session_id": agent_streaming_context.web_session_id}
        )
    if not agent_streaming_context.investigation_id:
        raise ValidationError("investigation_id is required for deliver_via_sse", field="investigation_id", constraint="required")
    if not agent_streaming_context.web_session_id:
        raise ValidationError("web_session_id is required for deliver_via_sse", field="web_session_id", constraint="required")
    case_id: str = agent_streaming_context.case_id
    investigation_id: str = agent_streaming_context.investigation_id
    web_session_id: str = agent_streaming_context.web_session_id
    user_id: str = agent_streaming_context.user_id or ""
    agent_mode = agent_streaming_context.agent_mode

    logger.info(
        "[SSE] Starting delivery: investigation_id=%s case_id=%s user_id=%s workflow=%s sentinel_mode=%s",
        investigation_id, case_id, user_id, agent_mode, agent_streaming_context.sentinel_mode,
    )
    logger.info("[SSE] Async generator iteration starting")

    _pending_search_calls: dict[str, str | None] = {}
    _turn = 0
    _thinking_started = False

    # Initialize grounding and token_usage to avoid UnboundLocalError
    grounding_metadata = None
    token_usage = None
    error_occurred = False

    try:
        async for chunk in stream:
            if chunk.type == StreamChunkFromModelType.TEXT:
                agent_streaming_context.response_text += chunk.data.content or ""
                await g8ed_event_service.publish_investigation_event(
                    investigation_id=investigation_id,
                    event_type=EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
                    payload=ChatResponseChunkPayload(content=chunk.data.content),
                    web_session_id=web_session_id,
                    case_id=case_id,
                    user_id=user_id,
                )

            elif chunk.type == StreamChunkFromModelType.THINKING:
                agent_streaming_context.set_thinking_started()
                # Determine action type based on whether this is the first thinking chunk
                action_type = ThinkingActionType.START if not _thinking_started else ThinkingActionType.UPDATE
                _thinking_started = True
                
                await g8ed_event_service.publish_investigation_event(
                    investigation_id=investigation_id,
                    event_type=EventType.LLM_CHAT_ITERATION_THINKING_STARTED,
                    payload=ChatThinkingPayload(
                        thinking=chunk.data.thinking,
                        action_type=action_type,
                    ),
                    web_session_id=web_session_id,
                    case_id=case_id,
                    user_id=user_id,
                )

            elif chunk.type == StreamChunkFromModelType.THINKING_END:
                agent_streaming_context.set_thinking_ended()
                _thinking_started = False
                # Emit thinking event with END action to indicate thinking phase is complete
                await g8ed_event_service.publish_investigation_event(
                    investigation_id=investigation_id,
                    event_type=EventType.LLM_CHAT_ITERATION_THINKING_STARTED,
                    payload=ChatThinkingPayload(
                        thinking=None,
                        action_type=ThinkingActionType.END,
                    ),
                    web_session_id=web_session_id,
                    case_id=case_id,
                    user_id=user_id,
                )

            elif chunk.type == StreamChunkFromModelType.TOOL_CALL:
                fn = chunk.data.tool_name or ""
                if fn == OperatorToolName.G8E_SEARCH_WEB:
                    exec_id = chunk.data.execution_id or ""
                    query = chunk.data.display_detail
                    _pending_search_calls[exec_id] = query
                    await g8ed_event_service.publish_investigation_event(
                        investigation_id=investigation_id,
                        event_type=EventType.LLM_TOOL_G8E_WEB_SEARCH_REQUESTED,
                        payload=AISearchWebPayload(
                            query=query,
                            execution_id=exec_id,
                            status=ToolCallStatus.STARTED,
                        ),
                        web_session_id=web_session_id,
                        case_id=case_id,
                        user_id=user_id,
                    )
                elif fn == OperatorToolName.CHECK_PORT:
                    await g8ed_event_service.publish_investigation_event(
                        investigation_id=investigation_id,
                        event_type=EventType.OPERATOR_NETWORK_PORT_CHECK_REQUESTED,
                        payload=OperatorNetworkPortCheckPayload(
                            port=chunk.data.display_detail,
                            execution_id=chunk.data.execution_id,
                            status=ToolCallStatus.STARTED,
                        ),
                        web_session_id=web_session_id,
                        case_id=case_id,
                        user_id=user_id,
                    )

            elif chunk.type == StreamChunkFromModelType.TOOL_RESULT:
                exec_id = chunk.data.execution_id or ""
                if exec_id in _pending_search_calls:
                    query = _pending_search_calls.pop(exec_id)
                    succeeded = chunk.data.success is not False
                    await g8ed_event_service.publish_investigation_event(
                        investigation_id=investigation_id,
                        event_type=(
                            EventType.LLM_TOOL_G8E_WEB_SEARCH_COMPLETED
                            if succeeded
                            else EventType.LLM_TOOL_G8E_WEB_SEARCH_FAILED
                        ),
                        payload=AISearchWebPayload(
                            query=query,
                            execution_id=exec_id,
                            status=ToolCallStatus.COMPLETED,
                        ),
                        web_session_id=web_session_id,
                        case_id=case_id,
                        user_id=user_id,
                    )

                _turn += 1
                await g8ed_event_service.publish_investigation_event(
                    investigation_id=investigation_id,
                    event_type=EventType.LLM_CHAT_ITERATION_COMPLETED,
                    payload=ChatTurnCompletePayload(turn=_turn),
                    web_session_id=web_session_id,
                    case_id=case_id,
                    user_id=user_id,
                )

            elif chunk.type == StreamChunkFromModelType.CITATIONS:
                agent_streaming_context.grounding_metadata = chunk.data.grounding_metadata
                grounding_metadata = chunk.data.grounding_metadata
                if grounding_metadata and grounding_metadata.grounding_used:
                    await g8ed_event_service.publish_investigation_event(
                        investigation_id=investigation_id,
                        event_type=EventType.LLM_CHAT_ITERATION_CITATIONS_RECEIVED,
                        payload=ChatCitationsReadyPayload(
                            grounding_metadata=grounding_metadata.flatten_for_wire(),
                        ),
                        web_session_id=web_session_id,
                        case_id=case_id,
                        user_id=user_id,
                    )

            elif chunk.type == StreamChunkFromModelType.COMPLETE:
                agent_streaming_context.token_usage = chunk.data.token_usage
                token_usage = chunk.data.token_usage
                agent_streaming_context.finish_reason = chunk.data.finish_reason
                logger.info(
                    "[SSE] COMPLETE chunk received: finish_reason=%s response_chars=%d",
                    chunk.data.finish_reason, len(agent_streaming_context.response_text),
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
                await g8ed_event_service.publish_investigation_event(
                    investigation_id=investigation_id,
                    event_type=EventType.LLM_CHAT_ITERATION_FAILED,
                    payload=ChatErrorPayload(error=error_message),
                    web_session_id=web_session_id,
                    case_id=case_id,
                    user_id=user_id,
                )
                error_occurred = True
                break  # Break instead of return to ensure post-loop code executes

        # Ensure we use the latest values from agent_streaming_context
        grounding_metadata = agent_streaming_context.grounding_metadata
        token_usage = agent_streaming_context.token_usage
        has_citations = bool(grounding_metadata and grounding_metadata.grounding_used)

        # Skip completion event if error already occurred
        if error_occurred:
            logger.info("[SSE] Skipping completion event due to prior error")
        else:
            await g8ed_event_service.publish_investigation_event(
                investigation_id=investigation_id,
                event_type=EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED,
                payload=ChatResponseCompletePayload(
                    content=agent_streaming_context.response_text,
                    finish_reason=agent_streaming_context.finish_reason or DEFAULT_FINISH_REASON,
                    has_citations=has_citations,
                    grounding_metadata=grounding_metadata.flatten_for_wire() if grounding_metadata else {},
                    token_usage=token_usage.flatten_for_wire() if token_usage else {},
                    agent_mode=agent_mode,
                ),
                web_session_id=web_session_id,
                case_id=case_id,
                user_id=user_id,
            )

        logger.info(
            "[SSE] Complete: investigation_id=%s has_citations=%s "
            "finish_reason=%s response_chars=%d",
            investigation_id, has_citations,
            agent_streaming_context.finish_reason, len(agent_streaming_context.response_text),
        )
        logger.info("[SSE] Async generator iteration completed")

    except asyncio.CancelledError:
        logger.info("[SSE] Cancelled for investigation %s", investigation_id)
        await g8ed_event_service.publish_investigation_event(
            investigation_id=investigation_id,
            event_type=EventType.LLM_CHAT_ITERATION_FAILED,
            payload=ChatErrorPayload(error="AI processing stopped"),
            web_session_id=web_session_id,
            case_id=case_id,
            user_id=user_id,
        )
        raise

    except Exception as e:
        logger.error("[SSE] Error: %s", e, exc_info=True)
        await g8ed_event_service.publish_investigation_event(
            investigation_id=investigation_id,
            event_type=EventType.LLM_CHAT_ITERATION_FAILED,
            payload=ChatErrorPayload(error=str(e)),
            web_session_id=web_session_id,
            case_id=case_id,
            user_id=user_id,
        )
