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
Provider turn processing — thinking state machine, stream chunk parsing,
model parts accumulation, token counting, parts consolidation, finish reason
normalization, and retry error classification.

All functions are stateless and accept typed inputs. Side-channel instance
variables are eliminated: callers pass a mutable result_out list to receive
the TurnResult alongside streamed chunks.
"""

from typing import Optional
from dataclasses import dataclass, field
import logging
from collections.abc import AsyncGenerator

import app.llm.llm_types as types
from app.constants import (
    AGENT_RETRYABLE_ERROR_SUBSTRINGS,
    AGENT_RETRYABLE_STATUS_CODES,
    DEFAULT_FINISH_REASON,
    StreamChunkFromModelType,
)
from app.models.agent import (
    StreamChunkData,
    StreamChunkFromModel,
    TurnResult,
)

logger = logging.getLogger(__name__)


@dataclass
class TurnState:
    """Mutable state for a single LLM stream turn."""
    model_response_parts: list[types.Part] = field(default_factory=list[types.Part])
    pending_tool_calls: list[types.ToolCall] = field(default_factory=list[types.ToolCall])
    thinking_active: bool = False
    thinking_text_parts: list[str] = field(default_factory=list[str])
    thinking_signature: Optional[types.ThoughtSignature] = None
    finish_reason: str = DEFAULT_FINISH_REASON
    input_tokens: int = 0
    output_tokens: int = 0
    total_tokens: int = 0

    def flush_thinking_block(self) -> None:
        combined = "".join(self.thinking_text_parts)
        if combined:
            self.model_response_parts.append(types.Part(
                text=combined,
                thought=True,
                thought_signature=self.thinking_signature,
            ))
        self.thinking_text_parts.clear()


def handle_usage_chunk(chunk: types.StreamChunkFromModel, state: TurnState) -> None:
    if chunk.usage_metadata:
        usage = chunk.usage_metadata
        chunk_in = usage.prompt_token_count or 0
        chunk_out = usage.candidates_token_count or 0
        chunk_total = usage.total_token_count or 0
        state.input_tokens += chunk_in
        state.output_tokens += chunk_out
        state.total_tokens += chunk_total
        logger.info("[TOKEN_USAGE] Chunk: in=%d out=%d total=%d", chunk_in, chunk_out, chunk_total)


def handle_finish_reason_chunk(chunk: types.StreamChunkFromModel, state: TurnState) -> None:
    if chunk.finish_reason:
        normalized = normalize_finish_reason(chunk.finish_reason)
        if normalized:
            logger.info("[TURN] Setting finish_reason: raw=%s normalized=%s previous=%s", chunk.finish_reason, normalized, state.finish_reason)
            state.finish_reason = normalized


def handle_thought_chunk(chunk: types.StreamChunkFromModel, state: TurnState) -> Optional[StreamChunkFromModel]:
    if chunk.thought and chunk.text:
        if not state.thinking_active:
            logger.info("[TURN] Thinking started")
        state.thinking_active = True
        state.thinking_text_parts.append(chunk.text)
        if chunk.thought_signature:
            state.thinking_signature = chunk.thought_signature
        return StreamChunkFromModel(
            type=StreamChunkFromModelType.THINKING,
            data=StreamChunkData(thinking=chunk.text),
        )
    return None


def handle_tool_call_chunk(chunk: types.StreamChunkFromModel, state: TurnState) -> Optional[StreamChunkFromModel]:
    if chunk.tool_calls:
        if state.thinking_active:
            thinking_char_count = sum(len(t) for t in state.thinking_text_parts)
            state.flush_thinking_block()
            logger.info("[TURN] Thinking ended: %d chars accumulated", thinking_char_count)
            state.thinking_active = False
            return StreamChunkFromModel(
                type=StreamChunkFromModelType.THINKING_END,
                data=StreamChunkData(),
            )
        
        logger.info("[TURN] Function call(s) received: count=%d names=%s", len(chunk.tool_calls), [fc.name for fc in chunk.tool_calls])
        for i, fc in enumerate(chunk.tool_calls):
            state.model_response_parts.append(types.Part(
                tool_call=fc,
                thought_signature=chunk.thought_signature if i == 0 else None,
            ))
            state.pending_tool_calls.append(fc)
        return None
    return None


def handle_text_chunk(chunk: types.StreamChunkFromModel, state: TurnState) -> Optional[StreamChunkFromModel]:
    if chunk.text and not chunk.thought:
        if state.thinking_active:
            state.flush_thinking_block()
            state.thinking_active = False
            return StreamChunkFromModel(
                type=StreamChunkFromModelType.THINKING_END,
                data=StreamChunkData(),
            )
        
        state.model_response_parts.append(types.Part(
            text=chunk.text,
            thought_signature=chunk.thought_signature,
        ))
        return StreamChunkFromModel(
            type=StreamChunkFromModelType.TEXT,
            data=StreamChunkData(content=chunk.text),
        )
    return None


async def process_provider_turn(
    stream_response: AsyncGenerator[types.StreamChunkFromModel, None],
    model_name: str,
    result_out: list[TurnResult],
) -> AsyncGenerator[StreamChunkFromModel, None]:
    """
    Consume one provider stream turn.

    Drives all thinking state transitions for a single LLM call.
    Yields StreamChunkFromModel chunks as they arrive.
    On completion, appends the TurnResult to result_out (always exactly one item).

    Thinking state machine:
      INACTIVE -> (thought chunk) -> ACTIVE -> (text or FC chunk) -> INACTIVE
    THINKING_END is emitted exactly once per transition out of ACTIVE.
    """
    state = TurnState()

    async for chunk in stream_response:
        handle_finish_reason_chunk(chunk, state)
        handle_usage_chunk(chunk, state)

        # 1. Thought chunks (Thinking content)
        if chunk.thought and chunk.text:
            yield StreamChunkFromModel(
                type=StreamChunkFromModelType.THINKING,
                data=StreamChunkData(thinking=chunk.text),
            )
            handle_thought_chunk(chunk, state)
            continue

        # 2. Tool calls
        if chunk.tool_calls:
            if state.thinking_active:
                state.flush_thinking_block()
                yield StreamChunkFromModel(type=StreamChunkFromModelType.THINKING_END, data=StreamChunkData())
                state.thinking_active = False
            
            logger.info("[TURN] Function call(s) received: count=%d names=%s", len(chunk.tool_calls), [fc.name for fc in chunk.tool_calls])
            for i, fc in enumerate(chunk.tool_calls):
                state.model_response_parts.append(types.Part(
                    tool_call=fc,
                    thought_signature=chunk.thought_signature if i == 0 else None,
                ))
                state.pending_tool_calls.append(fc)
            continue

        # 3. Thought signatures (without text/tools)
        if chunk.thought_signature and not chunk.text and not chunk.tool_calls:
            if state.thinking_active:
                state.thinking_signature = chunk.thought_signature
            else:
                state.model_response_parts.append(types.Part(thought_signature=chunk.thought_signature))
            continue

        # 4. Normal text chunks
        if chunk.text:
            if state.thinking_active:
                state.flush_thinking_block()
                yield StreamChunkFromModel(type=StreamChunkFromModelType.THINKING_END, data=StreamChunkData())
                state.thinking_active = False
            
            state.model_response_parts.append(types.Part(
                text=chunk.text,
                thought_signature=chunk.thought_signature,
            ))
            yield StreamChunkFromModel(
                type=StreamChunkFromModelType.TEXT,
                data=StreamChunkData(content=chunk.text),
            )

    if state.thinking_active:
        state.flush_thinking_block()
        yield StreamChunkFromModel(
            type=StreamChunkFromModelType.THINKING_END,
            data=StreamChunkData(),
        )

    logger.info(
        "[TURN] Complete: finish_reason=%s input_tokens=%d output_tokens=%d "
        "total_tokens=%d response_parts=%d pending_fc=%d thinking_used=%s",
        state.finish_reason, state.input_tokens, state.output_tokens, state.total_tokens,
        len(state.model_response_parts), len(state.pending_tool_calls),
        any(p.thought for p in state.model_response_parts),
    )

    result_out.append(TurnResult(
        model_response_parts=state.model_response_parts,
        pending_tool_calls=state.pending_tool_calls,
        finish_reason=state.finish_reason,
        input_tokens=state.input_tokens,
        output_tokens=state.output_tokens,
        total_tokens=state.total_tokens,
    ))



def consolidate_model_parts(
    parts: list[types.Part],
    model_name: str,
) -> list[types.Part]:
    """
    Consolidate model response parts for the tool calling loop.

    Preserves part ordering and special fields (thought, tool_call).
    Only adjacent plain text parts are merged into one.

    Provider-specific: thought_signature parts are preserved in order when
    present (some providers require them for tool calling context).
    """
    def _is_plain_text(p: types.Part) -> bool:
        return (
            p.text is not None and 
            not p.thought and 
            p.tool_call is None and 
            p.thought_signature is None
        )

    consolidated: list[types.Part] = []
    pending_text: list[str] = []
    sig_count = 0

    def _flush_pending_text() -> None:
        if pending_text:
            combined = "".join(pending_text)
            if combined:
                consolidated.append(types.Part.from_text(text=combined))
            pending_text.clear()

    for p in parts:
        if _is_plain_text(p):
            pending_text.append(p.text or "")
        else:
            _flush_pending_text()
            consolidated.append(p)
            if p.thought_signature:
                sig_count += 1

    _flush_pending_text()

    if sig_count:
        logger.info(
            "Consolidated %d -> %d parts, preserved %d thought signature(s) in order",
            len(parts), len(consolidated), sig_count,
        )

    return consolidated


def normalize_finish_reason(reason: str | None) -> str | None:
    """Normalize finish reason string to an uppercase string."""
    if not reason:
        return None

    text = reason.strip()
    if not text:
        return None

    text = text.replace("finish_reason_", "").replace("FINISH_REASON_", "")
    text = text.replace("stop_reason_", "").replace("STOP_REASON_", "")
    if "." in text:
        text = text.rsplit(".", 1)[-1]
    return text.upper()


def should_retry_error(error: Exception) -> bool:
    """Return True if the error is transient and the request should be retried.

    Covers transient HTTP errors, rate-limits, timeouts from any provider SDK
    (httpx.TimeoutException, stdlib TimeoutError / asyncio.TimeoutError), and
    known retryable error message patterns.
    """
    # Non-retryable errors - these should fail immediately
    if isinstance(error, (PermissionError, ValueError)):
        return False
    
    try:
        from httpx import TimeoutException as HttpxTimeout
        if isinstance(error, HttpxTimeout):
            return True
    except ImportError:
        pass
    if isinstance(error, (TimeoutError, OSError)):
        return True
    status_code = extract_status_code(error)
    if status_code and status_code in AGENT_RETRYABLE_STATUS_CODES:
        return True
    message = str(error).lower()
    return any(fragment in message for fragment in AGENT_RETRYABLE_ERROR_SUBSTRINGS)


def extract_status_code(error: Exception) -> int | None:
    """Extract an HTTP status code from an exception, if present."""
    # Handle known library exceptions explicitly
    try:
        from httpx import HTTPStatusError
        if isinstance(error, HTTPStatusError):
            if hasattr(error, "response") and hasattr(error.response, "status_code"):
                return error.response.status_code
    except ImportError:
        pass

    # For other exceptions with status_code attribute
    status_code = getattr(error, "status_code", None)
    if isinstance(status_code, int):
        return status_code
    
    return None
