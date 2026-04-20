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

"""Unit tests for agent_turn.py - provider turn processing functions."""

import pytest

from app.llm.llm_types import ToolCall, ThoughtSignature, UsageMetadata
from app.constants import (
    DEFAULT_FINISH_REASON,
    StreamChunkFromModelType,
)
from app.services.ai.agent_turn import (
    TurnState,
    consolidate_model_parts,
    extract_status_code,
    handle_finish_reason_chunk,
    handle_text_chunk,
    handle_thought_chunk,
    handle_tool_call_chunk,
    handle_usage_chunk,
    normalize_finish_reason,
    process_provider_turn,
    should_retry_error,
)
import app.llm.llm_types as types


class TestTurnState:
    """Test TurnState dataclass and methods."""

    def test_default_initialization(self):
        """Test TurnState initializes with correct defaults."""
        state = TurnState()
        assert state.model_response_parts == []
        assert state.pending_tool_calls == []
        assert state.thinking_active is False
        assert state.thinking_text_parts == []
        assert state.thinking_signature is None
        assert state.finish_reason == DEFAULT_FINISH_REASON
        assert state.input_tokens == 0
        assert state.output_tokens == 0
        assert state.total_tokens == 0

    def test_flush_thinking_block_with_content(self):
        """Test flush_thinking_block creates Part from accumulated text."""
        state = TurnState()
        state.thinking_text_parts = ["thinking ", "part 1", " part 2"]
        sig = ThoughtSignature(value="test-sig")
        state.thinking_signature = sig

        state.flush_thinking_block()

        assert len(state.model_response_parts) == 1
        part = state.model_response_parts[0]
        assert part.text == "thinking part 1 part 2"
        assert part.thought is True
        assert part.thought_signature == sig
        assert state.thinking_text_parts == []

    def test_flush_thinking_block_empty(self):
        """Test flush_thinking_block with no accumulated text does nothing."""
        state = TurnState()
        state.flush_thinking_block()
        assert len(state.model_response_parts) == 0

    def test_flush_thinking_block_clears_after_flush(self):
        """Test flush_thinking_block clears text_parts after creating Part."""
        state = TurnState()
        state.thinking_text_parts = ["some text"]
        state.flush_thinking_block()
        assert state.thinking_text_parts == []
        state.flush_thinking_block()
        assert len(state.model_response_parts) == 1


class TestHandleUsageChunk:
    """Test handle_usage_chunk token counting."""

    def test_handles_usage_metadata(self):
        """Test handle_usage_chunk accumulates token counts."""
        state = TurnState()
        chunk = types.StreamChunkFromModel(
            usage_metadata=UsageMetadata(
                prompt_token_count=10,
                candidates_token_count=20,
                total_token_count=30,
            )
        )

        handle_usage_chunk(chunk, state)

        assert state.input_tokens == 10
        assert state.output_tokens == 20
        assert state.total_tokens == 30

    def test_handles_none_values_in_usage(self):
        """Test handle_usage_chunk handles None values gracefully."""
        state = TurnState()
        chunk = types.StreamChunkFromModel(
            usage_metadata=UsageMetadata(
                prompt_token_count=None,
                candidates_token_count=None,
                total_token_count=None,
            )
        )

        handle_usage_chunk(chunk, state)

        assert state.input_tokens == 0
        assert state.output_tokens == 0
        assert state.total_tokens == 0

    def test_accumulates_across_chunks(self):
        """Test handle_usage_chunk accumulates across multiple chunks."""
        state = TurnState()
        chunk1 = types.StreamChunkFromModel(
            usage_metadata=UsageMetadata(
                prompt_token_count=10,
                candidates_token_count=20,
                total_token_count=30,
            )
        )
        chunk2 = types.StreamChunkFromModel(
            usage_metadata=UsageMetadata(
                prompt_token_count=5,
                candidates_token_count=10,
                total_token_count=15,
            )
        )

        handle_usage_chunk(chunk1, state)
        handle_usage_chunk(chunk2, state)

        assert state.input_tokens == 15
        assert state.output_tokens == 30
        assert state.total_tokens == 45

    def test_handles_missing_usage_metadata(self):
        """Test handle_usage_chunk with no usage_metadata."""
        state = TurnState()
        chunk = types.StreamChunkFromModel()
        handle_usage_chunk(chunk, state)
        assert state.input_tokens == 0
        assert state.output_tokens == 0
        assert state.total_tokens == 0


class TestHandleFinishReasonChunk:
    """Test handle_finish_reason_chunk normalization."""

    def test_normalizes_finish_reason(self):
        """Test handle_finish_reason_chunk normalizes finish reason."""
        state = TurnState()
        chunk = types.StreamChunkFromModel(finish_reason="stop")
        handle_finish_reason_chunk(chunk, state)
        assert state.finish_reason == "STOP"

    def test_handles_none_finish_reason(self):
        """Test handle_finish_reason_chunk with None."""
        state = TurnState()
        chunk = types.StreamChunkFromModel(finish_reason=None)
        handle_finish_reason_chunk(chunk, state)
        assert state.finish_reason == DEFAULT_FINISH_REASON

    def test_updates_finish_reason_on_subsequent_chunks(self):
        """Test finish_reason updates on subsequent chunks."""
        state = TurnState()
        chunk1 = types.StreamChunkFromModel(finish_reason="stop")
        chunk2 = types.StreamChunkFromModel(finish_reason="length")
        handle_finish_reason_chunk(chunk1, state)
        handle_finish_reason_chunk(chunk2, state)
        assert state.finish_reason == "LENGTH"


class TestHandleThoughtChunk:
    """Test handle_thought_chunk thinking state transitions."""

    def test_emits_thinking_chunk_on_first_thought(self):
        """Test first thought chunk emits THINKING event."""
        state = TurnState()
        chunk = types.StreamChunkFromModel(thought=True, text="thinking content")

        result = handle_thought_chunk(chunk, state)

        assert result is not None
        assert result.type == StreamChunkFromModelType.THINKING
        assert result.data.thinking == "thinking content"
        assert state.thinking_active is True
        assert state.thinking_text_parts == ["thinking content"]

    def test_accumulates_thinking_text(self):
        """Test thought chunks accumulate text."""
        state = TurnState()
        chunk1 = types.StreamChunkFromModel(thought=True, text="part1")
        chunk2 = types.StreamChunkFromModel(thought=True, text="part2")

        handle_thought_chunk(chunk1, state)
        handle_thought_chunk(chunk2, state)

        assert state.thinking_text_parts == ["part1", "part2"]

    def test_captures_thought_signature(self):
        """Test thought signature is captured."""
        state = TurnState()
        sig = ThoughtSignature(value="sig-value")
        chunk = types.StreamChunkFromModel(
            thought=True, text="text", thought_signature=sig
        )

        handle_thought_chunk(chunk, state)

        assert state.thinking_signature == sig

    def test_returns_none_for_non_thought_chunks(self):
        """Test non-thought chunks return None."""
        state = TurnState()
        chunk = types.StreamChunkFromModel(thought=False, text="regular text")

        result = handle_thought_chunk(chunk, state)

        assert result is None
        assert state.thinking_active is False

    def test_returns_none_when_thought_without_text(self):
        """Test thought=True without text returns None."""
        state = TurnState()
        chunk = types.StreamChunkFromModel(thought=True, text=None)

        result = handle_thought_chunk(chunk, state)

        assert result is None


class TestHandleToolCallChunk:
    """Test handle_tool_call_chunk tool call handling."""

    def test_emits_thinking_end_when_active(self):
        """Test tool call ends thinking and emits THINKING_END."""
        state = TurnState()
        state.thinking_active = True
        state.thinking_text_parts = ["prior thinking"]
        tool_call = ToolCall(name="test_func", args={"arg": "value"}, id="call-1")
        chunk = types.StreamChunkFromModel(tool_calls=[tool_call])

        result = handle_tool_call_chunk(chunk, state)

        assert result is not None
        assert result.type == StreamChunkFromModelType.THINKING_END
        assert state.thinking_active is False
        assert len(state.model_response_parts) == 1

    def test_adds_tool_calls_to_state(self):
        """Test tool calls are added to model_response_parts and pending_tool_calls."""
        state = TurnState()
        tool_call = ToolCall(name="test_func", args={"arg": "value"}, id="call-1")
        chunk = types.StreamChunkFromModel(tool_calls=[tool_call])

        handle_tool_call_chunk(chunk, state)

        assert len(state.model_response_parts) == 1
        assert state.model_response_parts[0].tool_call == tool_call
        assert len(state.pending_tool_calls) == 1
        assert state.pending_tool_calls[0] == tool_call

    def test_handles_multiple_tool_calls(self):
        """Test multiple tool calls are all added."""
        state = TurnState()
        tool_call1 = ToolCall(name="func1", args={}, id="call-1")
        tool_call2 = ToolCall(name="func2", args={}, id="call-2")
        chunk = types.StreamChunkFromModel(tool_calls=[tool_call1, tool_call2])

        handle_tool_call_chunk(chunk, state)

        assert len(state.model_response_parts) == 2
        assert len(state.pending_tool_calls) == 2

    def test_thought_signature_on_first_tool_call_only(self):
        """Test thought_signature applied only to first tool call."""
        state = TurnState()
        sig = ThoughtSignature(value="sig-value")
        tool_call1 = ToolCall(name="func1", args={}, id="call-1")
        tool_call2 = ToolCall(name="func2", args={}, id="call-2")
        chunk = types.StreamChunkFromModel(
            tool_calls=[tool_call1, tool_call2], thought_signature=sig
        )

        handle_tool_call_chunk(chunk, state)

        assert state.model_response_parts[0].thought_signature == sig
        assert state.model_response_parts[1].thought_signature is None

    def test_returns_none_when_thinking_not_active(self):
        """Test returns None when thinking is not active."""
        state = TurnState()
        tool_call = ToolCall(name="test_func", args={}, id="call-1")
        chunk = types.StreamChunkFromModel(tool_calls=[tool_call])

        result = handle_tool_call_chunk(chunk, state)

        assert result is None

    def test_returns_none_for_no_tool_calls(self):
        """Test returns None when chunk has no tool_calls."""
        state = TurnState()
        chunk = types.StreamChunkFromModel()

        result = handle_tool_call_chunk(chunk, state)

        assert result is None


class TestHandleTextChunk:
    """Test handle_text_chunk text handling."""

    def test_emits_thinking_end_when_active(self):
        """Test text chunk ends thinking and emits THINKING_END."""
        state = TurnState()
        state.thinking_active = True
        state.thinking_text_parts = ["prior thinking"]
        chunk = types.StreamChunkFromModel(text="regular text", thought=False)

        result = handle_text_chunk(chunk, state)

        assert result is not None
        assert result.type == StreamChunkFromModelType.THINKING_END
        assert state.thinking_active is False
        assert len(state.model_response_parts) == 1

    def test_emits_text_chunk_and_adds_to_parts(self):
        """Test text chunk emits TEXT and adds to model_response_parts."""
        state = TurnState()
        chunk = types.StreamChunkFromModel(text="hello world", thought=False)

        result = handle_text_chunk(chunk, state)

        assert result is not None
        assert result.type == StreamChunkFromModelType.TEXT
        assert result.data.content == "hello world"
        assert len(state.model_response_parts) == 1
        assert state.model_response_parts[0].text == "hello world"

    def test_captures_thought_signature(self):
        """Test thought_signature is captured on text chunk."""
        state = TurnState()
        sig = ThoughtSignature(value="sig-value")
        chunk = types.StreamChunkFromModel(
            text="text", thought=False, thought_signature=sig
        )

        handle_text_chunk(chunk, state)

        assert state.model_response_parts[0].thought_signature == sig

    def test_returns_none_for_thought_chunks(self):
        """Test thought=True chunks return None."""
        state = TurnState()
        chunk = types.StreamChunkFromModel(thought=True, text="thinking")

        result = handle_text_chunk(chunk, state)

        assert result is None

    def test_returns_none_for_no_text(self):
        """Test chunks with no text return None."""
        state = TurnState()
        chunk = types.StreamChunkFromModel(text=None, thought=False)

        result = handle_text_chunk(chunk, state)

        assert result is None


class TestProcessProviderTurn:
    """Test process_provider_turn async generator."""

    @pytest.mark.asyncio
    async def test_simple_text_stream(self):
        """Test simple text stream produces TEXT chunks and TurnResult."""
        async def stream():
            yield types.StreamChunkFromModel(text="hello ", thought=False)
            yield types.StreamChunkFromModel(text="world", thought=False)
            yield types.StreamChunkFromModel(finish_reason="stop")

        result_out = []
        chunks = []
        async for chunk in process_provider_turn(stream(), "test-model", result_out):
            chunks.append(chunk)

        assert len(chunks) == 2
        assert chunks[0].type == StreamChunkFromModelType.TEXT
        assert chunks[0].data.content == "hello "
        assert chunks[1].type == StreamChunkFromModelType.TEXT
        assert chunks[1].data.content == "world"
        assert len(result_out) == 1
        assert result_out[0].finish_reason == "STOP"

    @pytest.mark.asyncio
    async def test_thinking_to_text_transition(self):
        """Test thinking then text emits THINKING, THINKING_END, TEXT."""
        async def stream():
            yield types.StreamChunkFromModel(thought=True, text="thinking")
            yield types.StreamChunkFromModel(text="response", thought=False)
            yield types.StreamChunkFromModel(finish_reason="stop")

        result_out = []
        chunks = []
        async for chunk in process_provider_turn(stream(), "test-model", result_out):
            chunks.append(chunk)

        assert len(chunks) == 3
        assert chunks[0].type == StreamChunkFromModelType.THINKING
        assert chunks[1].type == StreamChunkFromModelType.THINKING_END
        assert chunks[2].type == StreamChunkFromModelType.TEXT
        assert result_out[0].finish_reason == "STOP"

    @pytest.mark.asyncio
    async def test_thinking_to_tool_call_transition(self):
        """Test thinking then tool call emits THINKING, THINKING_END."""
        async def stream():
            yield types.StreamChunkFromModel(thought=True, text="thinking")
            tool_call = ToolCall(name="func", args={}, id="call-1")
            yield types.StreamChunkFromModel(tool_calls=[tool_call])
            yield types.StreamChunkFromModel(finish_reason="stop")

        result_out = []
        chunks = []
        async for chunk in process_provider_turn(stream(), "test-model", result_out):
            chunks.append(chunk)

        assert len(chunks) == 2
        assert chunks[0].type == StreamChunkFromModelType.THINKING
        assert chunks[1].type == StreamChunkFromModelType.THINKING_END
        assert len(result_out[0].pending_tool_calls) == 1

    @pytest.mark.asyncio
    async def test_tool_call_chunks(self):
        """Test tool calls are accumulated in TurnResult."""
        async def stream():
            tool_call1 = ToolCall(name="func1", args={}, id="call-1")
            tool_call2 = ToolCall(name="func2", args={}, id="call-2")
            yield types.StreamChunkFromModel(tool_calls=[tool_call1, tool_call2])
            yield types.StreamChunkFromModel(finish_reason="stop")

        result_out = []
        async for _ in process_provider_turn(stream(), "test-model", result_out):
            pass

        assert len(result_out[0].pending_tool_calls) == 2
        assert len(result_out[0].model_response_parts) == 2

    @pytest.mark.asyncio
    async def test_token_accumulation(self):
        """Test token counts are accumulated in TurnResult."""
        async def stream():
            yield types.StreamChunkFromModel(
                usage_metadata=UsageMetadata(
                    prompt_token_count=10, candidates_token_count=5, total_token_count=15
                )
            )
            yield types.StreamChunkFromModel(
                usage_metadata=UsageMetadata(
                    prompt_token_count=5, candidates_token_count=5, total_token_count=10
                )
            )
            yield types.StreamChunkFromModel(finish_reason="stop")

        result_out = []
        async for _ in process_provider_turn(stream(), "test-model", result_out):
            pass

        assert result_out[0].input_tokens == 15
        assert result_out[0].output_tokens == 10
        assert result_out[0].total_tokens == 25

    @pytest.mark.asyncio
    async def test_thinking_flush_on_stream_end(self):
        """Test thinking is flushed at stream end if still active."""
        async def stream():
            yield types.StreamChunkFromModel(thought=True, text="thinking")

        result_out = []
        chunks = []
        async for chunk in process_provider_turn(stream(), "test-model", result_out):
            chunks.append(chunk)

        assert chunks[0].type == StreamChunkFromModelType.THINKING
        assert chunks[1].type == StreamChunkFromModelType.THINKING_END
        assert len(result_out[0].model_response_parts) == 1
        assert result_out[0].model_response_parts[0].thought is True

    @pytest.mark.asyncio
    async def test_thought_signature_without_text_or_tools(self):
        """Test thought_signature alone is handled correctly."""
        async def stream():
            sig = ThoughtSignature(value="sig-value")
            yield types.StreamChunkFromModel(thought_signature=sig)
            yield types.StreamChunkFromModel(finish_reason="stop")

        result_out = []
        async for _ in process_provider_turn(stream(), "test-model", result_out):
            pass

        assert len(result_out[0].model_response_parts) == 1
        assert result_out[0].model_response_parts[0].thought_signature.value == "sig-value"


class TestConsolidateModelParts:
    """Test consolidate_model_parts parts consolidation."""

    def test_consolidates_adjacent_plain_text_parts(self):
        """Test adjacent plain text parts are merged."""
        parts = [
            types.Part(text="hello "),
            types.Part(text="world "),
            types.Part(text="foo"),
        ]
        result = consolidate_model_parts(parts, "test-model")

        assert len(result) == 1
        assert result[0].text == "hello world foo"

    def test_preserves_thought_parts(self):
        """Test thought parts are preserved separately."""
        parts = [
            types.Part(text="plain", thought=False),
            types.Part(text="thinking", thought=True),
            types.Part(text="more plain", thought=False),
        ]
        result = consolidate_model_parts(parts, "test-model")

        assert len(result) == 3
        assert result[0].text == "plain"
        assert result[1].text == "thinking"
        assert result[1].thought is True
        assert result[2].text == "more plain"

    def test_preserves_tool_call_parts(self):
        """Test tool_call parts are preserved."""
        tool_call = ToolCall(name="func", args={}, id="call-1")
        parts = [
            types.Part(text="plain"),
            types.Part(tool_call=tool_call),
            types.Part(text="more plain"),
        ]
        result = consolidate_model_parts(parts, "test-model")

        assert len(result) == 3
        assert result[1].tool_call == tool_call

    def test_preserves_thought_signature_parts(self):
        """Test thought_signature parts are preserved."""
        sig = ThoughtSignature(value="sig-value")
        parts = [
            types.Part(text="plain"),
            types.Part(thought_signature=sig),
            types.Part(text="more plain"),
        ]
        result = consolidate_model_parts(parts, "test-model")

        assert len(result) == 3
        assert result[1].thought_signature == sig

    def test_handles_empty_list(self):
        """Test empty parts list returns empty list."""
        result = consolidate_model_parts([], "test-model")
        assert result == []

    def test_does_not_merge_non_adjacent_text(self):
        """Test non-adjacent text parts are not merged."""
        tool_call = ToolCall(name="func", args={}, id="call-1")
        parts = [
            types.Part(text="first"),
            types.Part(tool_call=tool_call),
            types.Part(text="second"),
        ]
        result = consolidate_model_parts(parts, "test-model")

        assert len(result) == 3
        assert result[0].text == "first"
        assert result[2].text == "second"


class TestNormalizeFinishReason:
    """Test normalize_finish_reason string normalization."""

    def test_uppercases_finish_reason(self):
        """Test finish reason is uppercased."""
        assert normalize_finish_reason("stop") == "STOP"
        assert normalize_finish_reason("STOP") == "STOP"

    def test_removes_finish_reason_prefix(self):
        """Test finish_reason_ prefix is removed."""
        assert normalize_finish_reason("finish_reason_stop") == "STOP"
        assert normalize_finish_reason("FINISH_REASON_STOP") == "STOP"

    def test_removes_stop_reason_prefix(self):
        """Test stop_reason_ prefix is removed."""
        assert normalize_finish_reason("stop_reason_max_tokens") == "MAX_TOKENS"
        assert normalize_finish_reason("STOP_REASON_LENGTH") == "LENGTH"

    def test_handles_dot_notation(self):
        """Test dot notation extracts last part."""
        assert normalize_finish_reason("stop.max_tokens") == "MAX_TOKENS"
        assert normalize_finish_reason("finish_reason.stop.length") == "LENGTH"

    def test_handles_none(self):
        """Test None returns None."""
        assert normalize_finish_reason(None) is None

    def test_handles_empty_string(self):
        """Test empty string returns None."""
        assert normalize_finish_reason("") is None
        assert normalize_finish_reason("   ") is None

    def test_strips_whitespace(self):
        """Test whitespace is stripped."""
        assert normalize_finish_reason("  stop  ") == "STOP"


class TestShouldRetryError:
    """Test should_retry_error retry classification."""

    def test_retryable_status_codes(self):
        """Test retryable status codes return True."""
        from app.constants import AGENT_RETRYABLE_STATUS_CODES

        for code in AGENT_RETRYABLE_STATUS_CODES:
            error = Exception(f"HTTP {code}")
            error.status_code = code
            assert should_retry_error(error) is True

    def test_non_retryable_status_codes(self):
        """Test non-retryable status codes return False."""
        error = Exception("HTTP 400")
        error.status_code = 400
        assert should_retry_error(error) is False

    def test_retryable_error_substrings(self):
        """Test retryable error substrings return True."""
        from app.constants import AGENT_RETRYABLE_ERROR_SUBSTRINGS

        for substring in AGENT_RETRYABLE_ERROR_SUBSTRINGS:
            error = Exception(f"Error: {substring} occurred")
            assert should_retry_error(error) is True

    def test_non_retryable_errors(self):
        """Test non-retryable errors return False."""
        error = Exception("Invalid API key")
        assert should_retry_error(error) is False

    def test_case_insensitive_substring_matching(self):
        """Test substring matching is case-insensitive."""
        error = Exception("MODEL IS OVERLOADED")
        assert should_retry_error(error) is True

    def test_status_code_takes_precedence(self):
        """Test status code check happens first."""
        error = Exception("Invalid API key")
        error.status_code = 429
        assert should_retry_error(error) is True


class TestExtractStatusCode:
    """Test extract_status_code status code extraction."""

    def test_extracts_from_httpx_error(self):
        """Test extracts status code from httpx.HTTPStatusError."""
        try:
            from httpx import HTTPStatusError, Response

            response = Response(status_code=429, request=None)
            error = HTTPStatusError("Rate limited", request=None, response=response)
            assert extract_status_code(error) == 429
        except ImportError:
            pytest.skip("httpx not available")

    def test_extracts_from_status_code_attribute(self):
        """Test extracts from status_code attribute."""
        error = Exception("Error")
        error.status_code = 500
        assert extract_status_code(error) == 500

    def test_returns_none_when_no_status_code(self):
        """Test returns None when no status code available."""
        error = Exception("Error")
        assert extract_status_code(error) is None

    def test_returns_none_for_non_int_status_code(self):
        """Test returns None when status_code is not an int."""
        error = Exception("Error")
        error.status_code = "500"
        assert extract_status_code(error) is None

    def test_handles_missing_response_attribute(self):
        """Test handles HTTPStatusError without response."""
        try:
            from httpx import HTTPStatusError

            error = HTTPStatusError("Error", request=None, response=None)
            assert extract_status_code(error) is None
        except ImportError:
            pytest.skip("httpx not available")
