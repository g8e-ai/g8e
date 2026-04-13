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

from app.errors import ExternalServiceError, ValidationError
from app.constants import ComponentName

"""
g8e Agent — orchestrates the ReAct streaming loop.

Concerns handled here:
  - Invocation context lifecycle (start / reset via AIToolExecutor) — owned by
    run_with_sse, NOT by stream_response, because stream_response is an async
    generator and Python runs its finally block in a different asyncio Context
    during async-generator cleanup, which makes ContextVar.reset() raise ValueError.
  - Retry loop with backoff around _stream_with_tool_loop
  - ReAct loop: provider turn -> tool calls -> next turn contents -> repeat
  - SSE delivery via run_with_sse (delegates to agent_sse)

All other concerns live in dedicated modules:
  agent_turn.py          — thinking state machine, stream parsing, parts consolidation,
                           finish reason normalization, retry classification
  agent_tool_loop.py — tool call dispatch, sequential execution,
                           tool display metadata, grounding merge
  agent_sse.py           — SSE translation and g8ed event delivery
  investigation_service.py — operator context extraction
"""

import asyncio
import logging
from collections.abc import AsyncGenerator

import app.llm.llm_types as types
from app.constants import (
    AGENT_MAX_RETRIES,
    AGENT_RETRY_BACKOFF_MULTIPLIER,
    AGENT_RETRY_DELAY_SECONDS,
    DEFAULT_FINISH_REASON,
)
from app.llm.provider import LLMProvider
from app.models.agent import (
    AgentStreamContext,
    ToolCallResponse,
    StreamChunkData,
    StreamChunkFromModel,
    StreamChunkFromModelType,
    TokenUsage,
    TurnResult,
)
from app.models.grounding import GroundingMetadata
from app.services.ai.agent_tool_loop import (
    execute_turn_tool_calls,
    merge_grounding,
)
from app.services.ai.agent_sse import deliver_via_sse
from app.services.ai.agent_turn import (
    consolidate_model_parts,
    process_provider_turn,
    should_retry_error,
)
from app.services.ai.grounding.grounding_service import GroundingService
from app.services.ai.tool_service import AIToolService
from app.services.infra.g8ed_event_service import EventService

logger = logging.getLogger(__name__)


class g8eEngine:
    """
    Unified g8e AI Agent — orchestrates the ReAct streaming loop.

    Usage:
        async for chunk in agent.stream_response(message, contents, config, model, context):
            yield chunk

        await agent.run_with_sse(contents, config, model, agent_stream_context, context, event_service)
    """

    def __init__(
        self,
        tool_executor: AIToolService,
        grounding_service: GroundingService | None = None,
    ):
        self._tool_executor = tool_executor
        self._grounding_service = grounding_service or GroundingService()
        logger.info("g8eEngine initialized")

    @property
    def tool_executor(self) -> AIToolService:
        return self._tool_executor

    @property
    def g8e_web_search_available(self) -> bool:
        return self._tool_executor.g8e_web_search_available

    async def stream_response(
        self,
        message: str,
        contents: list[types.Content],
        generation_config: types.PrimaryLLMSettings,
        model_name: str,
        context: AgentStreamContext,
        g8ed_event_service: EventService,
        llm_provider: LLMProvider,
    ) -> AsyncGenerator[StreamChunkFromModel, None]:
        """
        Stream AI response with full tool support.

        Yields StreamChunkFromModel objects. Callers deliver them via HTTP SSE
        (through run_with_sse) or consume them directly for pub/sub delivery.

        IMPORTANT: The invocation context (ContextVar on AIToolService) must be
        set by the caller before iterating this generator and reset after. This
        generator must NOT own the ContextVar lifecycle: it is an async generator,
        so Python runs its finally block in a different asyncio Context during
        async-generator cleanup, which makes ContextVar.reset() raise ValueError.
        run_with_sse owns this lifecycle via a normal try/finally.
        """
        case_id = context.case_id
        investigation_id = context.investigation_id
        user_id = context.user_id
        g8e_context = context.g8e_context
        agent_mode = context.agent_mode

        _bound_count = len(g8e_context.bound_operators) if g8e_context else 0
        logger.info(
            "[AGENT] stream_response start: model=%s investigation_id=%s case_id=%s "
            "workflow=%s bound_operators=%d contents=%d user_id=%s",
            model_name, investigation_id, case_id,
            agent_mode, _bound_count, len(contents),
            (user_id[:8] + "...") if user_id else None,
        )

        max_attempts = AGENT_MAX_RETRIES + 1
        backoff_seconds = AGENT_RETRY_DELAY_SECONDS
        attempt = 1
        streaming_started = False
        logger.info(
            "[AGENT] Retry config: max_attempts=%d initial_delay=%.1fs",
            max_attempts, backoff_seconds,
        )

        while attempt <= max_attempts:
            try:
                # Emit RETRY chunk before retrying (not on first attempt)
                if attempt > 1:
                    logger.info("Retry attempt %d/%d", attempt, max_attempts)
                    yield StreamChunkFromModel(
                        type=StreamChunkFromModelType.RETRY,
                        data=StreamChunkData(attempt=attempt, max_attempts=max_attempts),
                    )

                async for chunk in self._stream_with_tool_loop(
                    contents=contents,
                    generation_config=generation_config,
                    model_name=model_name,
                    context=context,
                    llm_provider=llm_provider,
                    g8ed_event_service=g8ed_event_service,
                ):
                    if chunk.type == StreamChunkFromModelType.TEXT:
                        streaming_started = True
                    yield chunk

                return

            except Exception as e:
                can_retry = (
                    attempt < max_attempts
                    and not streaming_started
                    and should_retry_error(e)
                )
                if not can_retry:
                    if streaming_started:
                        logger.error("[AGENT] Fatal error after streaming started: %s", e)
                    yield StreamChunkFromModel(
                        type=StreamChunkFromModelType.ERROR,
                        data=StreamChunkData(error=str(e)),
                    )
                    return

                logger.warning("[AGENT] Attempt %d failed, retrying: %s", attempt, e)
                await asyncio.sleep(backoff_seconds)
                backoff_seconds *= AGENT_RETRY_BACKOFF_MULTIPLIER
                attempt += 1

    async def run_with_sse(
        self,
        contents: list[types.Content],
        generation_config: types.PrimaryLLMSettings,
        model_name: str,
        agent_streaming_context: AgentStreamContext,
        context: AgentStreamContext,
        g8ed_event_service: EventService,
        llm_provider: LLMProvider,
    ) -> None:
        """
        SSE chat path — runs stream_response and delivers events to the browser.

        Owns the invocation context lifecycle: starts it before iterating
        stream_response and resets it in finally. This must live here (a normal
        coroutine) rather than inside stream_response (an async generator), because
        Python dispatches async-generator cleanup in a new asyncio Context, which
        makes ContextVar.reset() raise ValueError if the token was created in the
        original request Context.

        Delegates all SSE translation to agent_sse.deliver_via_sse.
        """
        logger.info(
            "[AGENT] run_with_sse: investigation_id=%s case_id=%s model=%s "
            "workflow=%s sentinel_mode=%s contents=%d",
            agent_streaming_context.investigation_id, agent_streaming_context.case_id,
            model_name, agent_streaming_context.agent_mode,
            agent_streaming_context.sentinel_mode, len(contents),
        )
        if not context.g8e_context:
            raise ValidationError("G8eHttpContext is required for run_with_sse", field="g8e_context", constraint="required")
        context_token = self._tool_executor.start_invocation_context(
            g8e_context=context.g8e_context,
        )
        try:
            await deliver_via_sse(
                stream=self.stream_response(
                    message="",
                    contents=contents,
                    generation_config=generation_config,
                    model_name=model_name,
                    context=context,
                    llm_provider=llm_provider,
                    g8ed_event_service=g8ed_event_service,
                ),
                agent_streaming_context=agent_streaming_context,
                g8ed_event_service=g8ed_event_service,
            )
        finally:
            self._tool_executor.reset_invocation_context(context_token)

    async def _stream_with_tool_loop(
        self,
        contents: list[types.Content],
        generation_config: types.PrimaryLLMSettings,
        model_name: str,
        context: AgentStreamContext,
        llm_provider: LLMProvider,
        g8ed_event_service: EventService,
    ) -> AsyncGenerator[StreamChunkFromModel, None]:
        """
        ReAct function-calling loop.

        Each iteration:
          1. Calls process_provider_turn — consumes one provider stream,
             drives the thinking state machine, yields chunks, writes TurnResult.
          2. If the turn produced tool calls, calls execute_turn_tool_calls
             — executes them sequentially, yields TOOL_CALL/TOOL_RESULT chunks.
          3. Appends model response + tool responses to contents and loops.
          4. Breaks when the turn produces no tool calls.

        Yields CITATIONS and COMPLETE at the end.
        """
        case_id = context.case_id
        investigation_id = context.investigation_id

        total_input_tokens = 0
        total_output_tokens = 0
        total_tokens = 0
        grounding_metadata: GroundingMetadata | None = None
        final_finish_reason: str = DEFAULT_FINISH_REASON

        loop_turn = 0
        while True:
            loop_turn += 1
            logger.info(
                "[AGENT] Tool loop turn %d: contents=%d case_id=%s investigation_id=%s",
                loop_turn, len(contents), case_id, investigation_id,
            )

            stream_response = llm_provider.generate_content_stream_primary(
                model=model_name,
                contents=contents,
                primary_llm_settings=generation_config,
            )

            if not stream_response:
                raise ExternalServiceError("LLM provider returned None stream — provider contract violation", service_name="llm_provider", component=ComponentName.G8EE)

            turn_result_out: list[TurnResult] = []
            async for chunk in process_provider_turn(stream_response, model_name, turn_result_out):
                yield chunk

            turn_result = turn_result_out[0]

            total_input_tokens += turn_result.input_tokens
            total_output_tokens += turn_result.output_tokens
            total_tokens += turn_result.total_tokens
            if turn_result.finish_reason:
                final_finish_reason = turn_result.finish_reason

            if not turn_result.pending_tool_calls:
                logger.info(
                    "[AGENT] Tool loop breaking: turn=%d finish_reason=%s "
                    "input_tokens=%d output_tokens=%d total_tokens=%d",
                    loop_turn, turn_result.finish_reason,
                    total_input_tokens, total_output_tokens, total_tokens,
                )
                break

            fc_responses_out: list[list[ToolCallResponse]] = []
            async for chunk in execute_turn_tool_calls(
                pending_tool_calls=turn_result.pending_tool_calls,
                tool_executor=self._tool_executor,
                investigation=context.investigation,
                g8e_context=context.g8e_context,
                result_out=fc_responses_out,
                request_settings=context.request_settings,
                g8ed_event_service=g8ed_event_service,
            ):
                yield chunk

            fc_responses: list[ToolCallResponse] = fc_responses_out[0]

            for resp in fc_responses:
                if resp.grounding is not None:
                    grounding_metadata = merge_grounding(grounding_metadata, resp.grounding)

            if turn_result.model_response_parts:
                consolidated = consolidate_model_parts(
                    turn_result.model_response_parts, model_name=model_name
                )
                contents.append(types.Content(role=types.Role.MODEL, parts=consolidated))
                logger.info("[AGENT] Added model response: %d parts", len(consolidated))

            tool_response_parts = [
                types.Part.from_tool_response(
                    name=r.tool_name,
                    response=r.flattened_response,
                    id=r.tool_call_id,
                )
                for r in fc_responses
            ]
            contents.append(types.Content(role=types.Role.USER, parts=tool_response_parts))
            logger.info("[AGENT] Added %d tool responses, looping...", len(tool_response_parts))

        if grounding_metadata and grounding_metadata.grounding_used:
            yield StreamChunkFromModel(
                type=StreamChunkFromModelType.CITATIONS,
                data=StreamChunkData(grounding_metadata=grounding_metadata),
            )
        else:
            logger.info("[AGENT] No grounding metadata to emit for this turn")

        token_usage = TokenUsage(
            input_tokens=total_input_tokens,
            output_tokens=total_output_tokens,
            total_tokens=total_tokens,
        ) if (total_input_tokens or total_output_tokens or total_tokens) else None

        logger.info(
            "[AGENT] Yielding COMPLETE chunk: finish_reason=%s input_tokens=%d output_tokens=%d total_tokens=%d",
            final_finish_reason or DEFAULT_FINISH_REASON, total_input_tokens, total_output_tokens, total_tokens,
        )
        yield StreamChunkFromModel(
            type=StreamChunkFromModelType.COMPLETE,
            data=StreamChunkData(
                finish_reason=final_finish_reason or DEFAULT_FINISH_REASON,
                token_usage=token_usage,
            ),
        )
