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
Unit tests for g8eEngine (agent.py).

Tests:
- Constructor initialization with required and optional dependencies
- Property accessors (tool_executor, g8e_web_search_available)
- stream_response retry loop with backoff
- stream_response error handling (retryable vs non-retryable)
- stream_response streaming_started guard
- run_with_sse ContextVar lifecycle ownership
- run_with_sse G8eHttpContext validation
- _stream_with_tool_loop ReAct loop termination
- _stream_with_tool_loop token accumulation
- _stream_with_tool_loop grounding metadata merge
- _stream_with_tool_loop CITATIONS emission
- _stream_with_tool_loop COMPLETE emission with token usage

Run with:
    ./scripts/testing/run_tests.sh g8ee -- tests/unit/services/ai/test_agent.py
"""

from unittest.mock import MagicMock, patch

import pytest

from app.constants import (
    AGENT_MAX_RETRIES,
    AGENT_RETRY_BACKOFF_MULTIPLIER,
    AGENT_RETRY_DELAY_SECONDS,
    DEFAULT_FINISH_REASON,
)
from app.errors import ValidationError
from app.models.agent import (
    StreamChunkData,
    StreamChunkFromModel,
    StreamChunkFromModelType,
)
from app.models.tool_results import SearchWebResult
from app.models.grounding import GroundingMetadata
from app.services.ai.agent import g8eEngine
from app.services.ai.agent_tool_loop import ToolCallResponse
from tests.fakes.agent_helpers import (
    make_agent_inputs,
    make_agent_stream_state,
    make_g8e_agent,
    make_gen_config,
    make_provider_chunk,
    make_g8ed_event_service,
)

pytestmark = pytest.mark.unit


# =============================================================================
# TEST: Constructor
# =============================================================================

class Testg8eEngineConstructor:

    def test_constructor_with_required_dependencies(self):
        tool_executor = MagicMock()
        agent = g8eEngine(tool_executor=tool_executor)

        assert agent._tool_executor is tool_executor
        assert agent._grounding_service is not None

    def test_constructor_with_optional_grounding_service(self):
        tool_executor = MagicMock()
        grounding_service = MagicMock()
        agent = g8eEngine(
            tool_executor=tool_executor,
            grounding_service=grounding_service,
        )

        assert agent._grounding_service is grounding_service

    def test_constructor_creates_default_grounding_service_when_none(self):
        tool_executor = MagicMock()
        agent = g8eEngine(
            tool_executor=tool_executor,
            grounding_service=None,
        )

        from app.services.ai.grounding.grounding_service import GroundingService
        assert isinstance(agent._grounding_service, GroundingService)


# =============================================================================
# TEST: Property Accessors
# =============================================================================

class Testg8eEngineProperties:

    def test_tool_executor_property_returns_executor(self):
        tool_executor = MagicMock()
        agent = make_g8e_agent(fn_handler=tool_executor)

        assert agent.tool_executor is tool_executor

    def test_g8e_web_search_available_delegates_to_executor(self):
        tool_executor = MagicMock()
        tool_executor.g8e_web_search_available = True
        agent = make_g8e_agent(fn_handler=tool_executor)

        assert agent.g8e_web_search_available is True

    def test_g8e_web_search_available_false_when_executor_false(self):
        tool_executor = MagicMock()
        tool_executor.g8e_web_search_available = False
        agent = make_g8e_agent(fn_handler=tool_executor)

        assert agent.g8e_web_search_available is False


# =============================================================================
# TEST: stream_response - Retry Loop
# =============================================================================

@pytest.mark.asyncio(loop_scope="session")
class TestStreamResponseRetryLoop:

    async def test_retry_on_retryable_error_before_streaming_starts(self):
        tool_executor = MagicMock()
        tool_executor.start_invocation_context = MagicMock(return_value="token")
        tool_executor.reset_invocation_context = MagicMock()

        provider = MagicMock()
        call_count = 0

        def failing_stream(**kwargs):
            nonlocal call_count
            call_count += 1
            if call_count < 2:
                raise ConnectionError("service unavailable")
            async def _gen():
                yield make_provider_chunk(text="Success")
                yield make_provider_chunk(finish_reason="STOP")
            return _gen()

        provider.generate_content_stream_primary = failing_stream

        agent = make_g8e_agent(fn_handler=tool_executor)
        context = make_agent_inputs()
        gen_config = make_gen_config()
        g8ed_event_service = make_g8ed_event_service()

        context.model_to_use = "test-model"
        context.generation_config = gen_config
        chunks = []
        async for chunk in agent.stream_response(
            inputs=context,
            g8ed_event_service=g8ed_event_service,
            llm_provider=provider,
        ):
            chunks.append(chunk)

        assert len(chunks) == 3
        assert chunks[0].type == StreamChunkFromModelType.RETRY
        assert chunks[0].data.attempt == 2
        assert chunks[0].data.max_attempts == AGENT_MAX_RETRIES + 1
        assert chunks[1].type == StreamChunkFromModelType.TEXT
        assert call_count == 2

    async def test_no_retry_after_streaming_starts(self):
        tool_executor = MagicMock()
        tool_executor.start_invocation_context = MagicMock(return_value="token")
        tool_executor.reset_invocation_context = MagicMock()

        provider = MagicMock()

        def stream_then_fail(**kwargs):
            async def _gen():
                yield make_provider_chunk(text="Hello")
                raise ConnectionError("Network error after text")
                yield
            return _gen()

        provider.generate_content_stream_primary = stream_then_fail

        agent = make_g8e_agent(fn_handler=tool_executor)
        context = make_agent_inputs()
        gen_config = make_gen_config()
        g8ed_event_service = make_g8ed_event_service()

        context.model_to_use = "test-model"
        context.generation_config = gen_config
        chunks = []
        async for chunk in agent.stream_response(
            inputs=context,
            g8ed_event_service=g8ed_event_service,
            llm_provider=provider,
        ):
            chunks.append(chunk)

        assert len(chunks) == 2
        assert chunks[0].type == StreamChunkFromModelType.TEXT
        assert chunks[1].type == StreamChunkFromModelType.ERROR
        assert "Network error after text" in chunks[1].data.error

    async def test_exponential_backoff_between_retries(self):
        tool_executor = MagicMock()
        tool_executor.start_invocation_context = MagicMock(return_value="token")
        tool_executor.reset_invocation_context = MagicMock()

        provider = MagicMock()
        call_count = 0
        sleep_calls = []

        def failing_stream(**kwargs):
            nonlocal call_count
            call_count += 1
            if call_count < 3:
                raise ConnectionError("service unavailable")
            async def _gen():
                yield make_provider_chunk(text="Success")
                yield make_provider_chunk(finish_reason="STOP")
            return _gen()

        provider.generate_content_stream_primary = failing_stream

        agent = make_g8e_agent(fn_handler=tool_executor)
        context = make_agent_inputs()
        gen_config = make_gen_config()
        g8ed_event_service = make_g8ed_event_service()

        with patch("asyncio.sleep") as mock_sleep:
            async def capture_sleep(duration):
                sleep_calls.append(duration)

            mock_sleep.side_effect = capture_sleep

            context.model_to_use = "test-model"
            context.generation_config = gen_config
            chunks = []
            async for chunk in agent.stream_response(
                inputs=context,
                g8ed_event_service=g8ed_event_service,
                llm_provider=provider,
            ):
                chunks.append(chunk)

            assert len(sleep_calls) == 2
            assert sleep_calls[0] == AGENT_RETRY_DELAY_SECONDS
            assert sleep_calls[1] == AGENT_RETRY_DELAY_SECONDS * AGENT_RETRY_BACKOFF_MULTIPLIER

    async def test_max_retries_respected(self):
        tool_executor = MagicMock()
        tool_executor.start_invocation_context = MagicMock(return_value="token")
        tool_executor.reset_invocation_context = MagicMock()

        provider = MagicMock()

        def always_failing_stream(**kwargs):
            async def _gen():
                raise ConnectionError("Always fails")
                yield
            return _gen()

        provider.generate_content_stream_primary = always_failing_stream

        agent = make_g8e_agent(fn_handler=tool_executor)
        context = make_agent_inputs()
        gen_config = make_gen_config()
        g8ed_event_service = make_g8ed_event_service()

        context.model_to_use = "test-model"
        context.generation_config = gen_config
        chunks = []
        async for chunk in agent.stream_response(
            inputs=context,
            g8ed_event_service=g8ed_event_service,
            llm_provider=provider,
        ):
            chunks.append(chunk)

        # Expect RETRY chunks for attempts 2, 3, 4, then ERROR
        assert len(chunks) == 4
        assert chunks[0].type == StreamChunkFromModelType.RETRY
        assert chunks[0].data.attempt == 2
        assert chunks[1].type == StreamChunkFromModelType.RETRY
        assert chunks[1].data.attempt == 3
        assert chunks[2].type == StreamChunkFromModelType.RETRY
        assert chunks[2].data.attempt == 4
        assert chunks[3].type == StreamChunkFromModelType.ERROR
        assert "Always fails" in chunks[3].data.error


# =============================================================================
# TEST: stream_response - Error Handling
# =============================================================================

@pytest.mark.asyncio(loop_scope="session")
class TestStreamResponseErrorHandling:

    async def test_non_retryable_error_returns_immediately(self):
        tool_executor = MagicMock()
        tool_executor.start_invocation_context = MagicMock(return_value="token")
        tool_executor.reset_invocation_context = MagicMock()

        provider = MagicMock()

        def auth_error_stream(**kwargs):
            async def _gen():
                raise PermissionError("Invalid API key")
                yield
            return _gen()

        provider.generate_content_stream_primary = auth_error_stream

        agent = make_g8e_agent(fn_handler=tool_executor)
        context = make_agent_inputs()
        gen_config = make_gen_config()
        g8ed_event_service = make_g8ed_event_service()

        context.model_to_use = "test-model"
        context.generation_config = gen_config
        chunks = []
        async for chunk in agent.stream_response(
            inputs=context,
            g8ed_event_service=g8ed_event_service,
            llm_provider=provider,
        ):
            chunks.append(chunk)

        assert len(chunks) == 1
        assert chunks[0].type == StreamChunkFromModelType.ERROR
        assert "Invalid API key" in chunks[0].data.error

    async def test_success_returns_without_error(self):
        tool_executor = MagicMock()
        tool_executor.start_invocation_context = MagicMock(return_value="token")
        tool_executor.reset_invocation_context = MagicMock()

        provider = MagicMock()

        def success_stream(**kwargs):
            async def _gen():
                yield make_provider_chunk(text="Success")
                yield make_provider_chunk(finish_reason="STOP")
            return _gen()

        provider.generate_content_stream_primary = success_stream

        agent = make_g8e_agent(fn_handler=tool_executor)
        context = make_agent_inputs()
        gen_config = make_gen_config()
        g8ed_event_service = make_g8ed_event_service()

        context.model_to_use = "test-model"
        context.generation_config = gen_config
        chunks = []
        async for chunk in agent.stream_response(
            inputs=context,
            g8ed_event_service=g8ed_event_service,
            llm_provider=provider,
        ):
            chunks.append(chunk)

        assert all(c.type != StreamChunkFromModelType.ERROR for c in chunks)


# =============================================================================
# TEST: run_with_sse - ContextVar Lifecycle
# =============================================================================

@pytest.mark.asyncio(loop_scope="session")
class TestRunWithSSEContextVarLifecycle:

    async def test_starts_invocation_context_before_streaming(self):
        tool_executor = MagicMock()
        tool_executor.start_invocation_context = MagicMock(return_value="context-token")
        tool_executor.reset_invocation_context = MagicMock()

        provider = MagicMock()

        def empty_stream(**kwargs):
            async def _gen():
                yield make_provider_chunk(finish_reason="STOP")
            return _gen()

        provider.generate_content_stream_primary = empty_stream

        agent = make_g8e_agent(fn_handler=tool_executor)
        context = make_agent_inputs()
        g8ed_event_service = make_g8ed_event_service()

        await agent.run_with_sse(
            inputs=context,
            state=make_agent_stream_state(),
            g8ed_event_service=g8ed_event_service,
            llm_provider=provider,
        )

        tool_executor.start_invocation_context.assert_called_once()
        call_kwargs = tool_executor.start_invocation_context.call_args.kwargs
        assert "g8e_context" in call_kwargs

    async def test_resets_invocation_context_in_finally(self):
        tool_executor = MagicMock()
        tool_executor.start_invocation_context = MagicMock(return_value="context-token")
        tool_executor.reset_invocation_context = MagicMock()

        provider = MagicMock()

        def error_stream(**kwargs):
            async def _gen():
                raise RuntimeError("Stream error")
                yield
            return _gen()

        provider.generate_content_stream_primary = error_stream

        agent = make_g8e_agent(fn_handler=tool_executor)
        context = make_agent_inputs()
        g8ed_event_service = make_g8ed_event_service()

        try:
            await agent.run_with_sse(
                inputs=context,
                state=make_agent_stream_state(),
                g8ed_event_service=g8ed_event_service,
                llm_provider=provider,
            )
        except RuntimeError:
            pass

        tool_executor.reset_invocation_context.assert_called_once_with("context-token")

    async def test_resets_invocation_context_on_success(self):
        tool_executor = MagicMock()
        tool_executor.start_invocation_context = MagicMock(return_value="context-token")
        tool_executor.reset_invocation_context = MagicMock()

        provider = MagicMock()

        def success_stream(**kwargs):
            async def _gen():
                yield make_provider_chunk(finish_reason="STOP")
            return _gen()

        provider.generate_content_stream_primary = success_stream

        agent = make_g8e_agent(fn_handler=tool_executor)
        context = make_agent_inputs()
        g8ed_event_service = make_g8ed_event_service()

        await agent.run_with_sse(
            inputs=context,
            state=make_agent_stream_state(),
            g8ed_event_service=g8ed_event_service,
            llm_provider=provider,
        )

        tool_executor.reset_invocation_context.assert_called_once_with("context-token")


# =============================================================================
# TEST: run_with_sse - G8eHttpContext Validation
# =============================================================================

@pytest.mark.asyncio(loop_scope="session")
class TestRunWithSSEValidation:

    async def test_raises_validation_error_when_g8e_context_missing(self):
        tool_executor = MagicMock()
        provider = MagicMock()
        agent = make_g8e_agent(fn_handler=tool_executor)

        context = make_agent_inputs()
        context.g8e_context = None

        g8ed_event_service = make_g8ed_event_service()

        with pytest.raises(ValidationError) as exc_info:
            await agent.run_with_sse(
                inputs=context,
                state=make_agent_stream_state(),
                g8ed_event_service=g8ed_event_service,
                llm_provider=provider,
            )

        assert "G8eHttpContext is required" in str(exc_info.value)


# =============================================================================
# TEST: _stream_with_tool_loop - ReAct Loop
# =============================================================================

@pytest.mark.asyncio(loop_scope="session")
class TestStreamWithToolLoop:

    async def test_loop_terminates_when_no_tool_calls(self):
        tool_executor = MagicMock()
        provider = MagicMock()

        def no_tool_stream(**kwargs):
            async def _gen():
                yield make_provider_chunk(text="Response without tools")
                yield make_provider_chunk(finish_reason="STOP")
            return _gen()

        provider.generate_content_stream_primary = no_tool_stream

        agent = make_g8e_agent(fn_handler=tool_executor)
        context = make_agent_inputs()
        gen_config = make_gen_config()
        g8ed_event_service = make_g8ed_event_service()

        chunks = []
        async for chunk in agent._stream_with_tool_loop(
            contents=[],
            generation_config=gen_config,
            model_name="test-model",
            inputs=context,
            g8ed_event_service=g8ed_event_service,
            llm_provider=provider,
        ):
            chunks.append(chunk)

        assert any(c.type == StreamChunkFromModelType.COMPLETE for c in chunks)

    async def test_loop_continues_when_tool_calls_present(self):
        from app.llm.llm_types import ToolCall

        tool_executor = MagicMock()
        provider = MagicMock()

        call_count = 0

        def tool_stream(**kwargs):
            nonlocal call_count
            call_count += 1
            if call_count == 1:
                async def _gen():
                    yield make_provider_chunk(text="First turn")
                    yield make_provider_chunk(
                        tool_calls=[ToolCall(name="search_web", args={"query": "test"})],
                        finish_reason="STOP",
                    )
                return _gen()
            else:
                async def _gen():
                    yield make_provider_chunk(text="Second turn")
                    yield make_provider_chunk(finish_reason="STOP")
                return _gen()

        provider.generate_content_stream_primary = tool_stream

        agent = make_g8e_agent(fn_handler=tool_executor)
        context = make_agent_inputs()
        gen_config = make_gen_config()
        g8ed_event_service = make_g8ed_event_service()

        chunks = []
        async for chunk in agent._stream_with_tool_loop(
            contents=[],
            generation_config=gen_config,
            model_name="test-model",
            inputs=context,
            g8ed_event_service=g8ed_event_service,
            llm_provider=provider,
        ):
            chunks.append(chunk)

        assert call_count == 2

    async def test_uses_provided_llm_provider(self):
        tool_executor = MagicMock()
        provider = MagicMock()

        def stream(**kwargs):
            async def _gen():
                yield make_provider_chunk(text="Response")
                yield make_provider_chunk(finish_reason="STOP")
            return _gen()

        provider.generate_content_stream_primary = MagicMock(side_effect=stream)

        agent = make_g8e_agent(fn_handler=tool_executor)
        context = make_agent_inputs()
        gen_config = make_gen_config()
        g8ed_event_service = make_g8ed_event_service()

        chunks = []
        async for chunk in agent._stream_with_tool_loop(
            contents=[],
            generation_config=gen_config,
            model_name="test-model",
            inputs=context,
            g8ed_event_service=g8ed_event_service,
            llm_provider=provider,
        ):
            chunks.append(chunk)

        provider.generate_content_stream_primary.assert_called_once()


# =============================================================================
# TEST: _stream_with_tool_loop - Max Turn Limit & Continuation Approval
# =============================================================================

@pytest.mark.asyncio(loop_scope="session")
class TestMaxTurnLimitApproval:
    """When loop_turn exceeds AGENT_MAX_TOOL_TURNS, the agent must request
    operator approval via the approval pipeline to continue (or stop cleanly)."""

    def _make_tool_calling_provider(self):
        """Provider that always emits a tool call, forcing the loop to run indefinitely."""
        from app.llm.llm_types import ToolCall

        def _stream(**kwargs):
            async def _gen():
                yield make_provider_chunk(text="turn")
                yield make_provider_chunk(
                    tool_calls=[ToolCall(name="search_web", args={"query": "x"})],
                    finish_reason="STOP",
                )
            return _gen()

        provider = MagicMock()
        provider.generate_content_stream_primary = MagicMock(side_effect=_stream)
        return provider

    async def test_requests_approval_when_max_turns_exceeded_and_stops_on_deny(self):
        from tests.fakes.fake_approval_service import FakeApprovalService

        approval_service = FakeApprovalService(approved=False)
        provider = self._make_tool_calling_provider()

        agent = make_g8e_agent(approval_service=approval_service)
        context = make_agent_inputs()
        gen_config = make_gen_config()
        g8ed_event_service = make_g8ed_event_service()

        with patch("app.services.ai.agent.AGENT_MAX_TOOL_TURNS", 2), \
             patch("app.services.ai.agent.execute_turn_tool_calls") as mock_exec:
            async def _fake_exec(*, result_out, **kwargs):
                result_out.append([])
                if False:
                    yield  # make async generator
            mock_exec.side_effect = lambda **kw: _fake_exec(**kw)

            chunks = []
            async for chunk in agent._stream_with_tool_loop(
                contents=[],
                generation_config=gen_config,
                model_name="test-model",
                inputs=context,
                g8ed_event_service=g8ed_event_service,
                llm_provider=provider,
            ):
                chunks.append(chunk)

        assert len(approval_service.agent_continue_approval_calls) == 1
        req = approval_service.agent_continue_approval_calls[0]
        assert req.turn_limit == 2
        assert req.turns_completed == 2
        assert req.task_id == "ai.agent.continue"
        assert "turns" in req.justification
        assert req.operator_id == ""
        assert req.operator_session_id == ""
        # No command_approval_calls: continuation uses dedicated approval type
        assert len(approval_service.command_approval_calls) == 0
        assert provider.generate_content_stream_primary.call_count == 2

    async def test_continues_when_approval_granted(self):
        from tests.fakes.fake_approval_service import FakeApprovalService

        approval_service = FakeApprovalService(approved=True)
        provider = self._make_tool_calling_provider()

        agent = make_g8e_agent(approval_service=approval_service)
        context = make_agent_inputs()
        gen_config = make_gen_config()
        g8ed_event_service = make_g8ed_event_service()

        with patch("app.services.ai.agent.AGENT_MAX_TOOL_TURNS", 2), \
             patch("app.services.ai.agent.execute_turn_tool_calls") as mock_exec:
            async def _fake_exec(*, result_out, **kwargs):
                result_out.append([])
                if False:
                    yield
            mock_exec.side_effect = lambda **kw: _fake_exec(**kw)

            call_budget = {"n": 0}
            original_side_effect = provider.generate_content_stream_primary.side_effect

            def limited_stream(**kwargs):
                call_budget["n"] += 1
                if call_budget["n"] > 5:
                    async def _stop():
                        yield make_provider_chunk(text="done")
                        yield make_provider_chunk(finish_reason="STOP")
                    return _stop()
                return original_side_effect(**kwargs)

            provider.generate_content_stream_primary.side_effect = limited_stream

            chunks = []
            async for chunk in agent._stream_with_tool_loop(
                contents=[],
                generation_config=gen_config,
                model_name="test-model",
                inputs=context,
                g8ed_event_service=g8ed_event_service,
                llm_provider=provider,
            ):
                chunks.append(chunk)

        # With limit=2, approval is requested after turn 2, granted; counter resets
        # to 1, so we need another approval after turn 4, etc. We exercised >= 2 approvals.
        assert len(approval_service.agent_continue_approval_calls) >= 2
        assert len(approval_service.command_approval_calls) == 0

    async def test_no_approval_service_falls_back_to_abort(self):
        provider = self._make_tool_calling_provider()

        agent = make_g8e_agent(approval_service=None)
        context = make_agent_inputs()
        gen_config = make_gen_config()
        g8ed_event_service = make_g8ed_event_service()

        with patch("app.services.ai.agent.AGENT_MAX_TOOL_TURNS", 2), \
             patch("app.services.ai.agent.execute_turn_tool_calls") as mock_exec:
            async def _fake_exec(*, result_out, **kwargs):
                result_out.append([])
                if False:
                    yield
            mock_exec.side_effect = lambda **kw: _fake_exec(**kw)

            chunks = []
            async for chunk in agent._stream_with_tool_loop(
                contents=[],
                generation_config=gen_config,
                model_name="test-model",
                inputs=context,
                g8ed_event_service=g8ed_event_service,
                llm_provider=provider,
            ):
                chunks.append(chunk)

        # Exactly max-turn provider calls, then abort (no approval possible)
        assert provider.generate_content_stream_primary.call_count == 2


# =============================================================================
# TEST: _stream_with_tool_loop - Token Accumulation
# =============================================================================

@pytest.mark.asyncio(loop_scope="session")
class TestTokenAccumulation:

    async def test_accumulates_tokens_across_turns(self):
        tool_executor = MagicMock()
        provider = MagicMock()

        def multi_turn_stream(**kwargs):
            async def _gen():
                chunk = make_provider_chunk(
                    text="Turn 1",
                    finish_reason="STOP",
                )
                chunk.usage_metadata = MagicMock()
                chunk.usage_metadata.prompt_token_count = 10
                chunk.usage_metadata.candidates_token_count = 5
                chunk.usage_metadata.total_token_count = 15
                yield chunk
            return _gen()

        provider.generate_content_stream_primary = multi_turn_stream

        agent = make_g8e_agent(fn_handler=tool_executor)
        context = make_agent_inputs()
        gen_config = make_gen_config()
        g8ed_event_service = make_g8ed_event_service()

        chunks = []
        async for chunk in agent._stream_with_tool_loop(
            contents=[],
            generation_config=gen_config,
            model_name="test-model",
            inputs=context,
            g8ed_event_service=g8ed_event_service,
            llm_provider=provider,
        ):
            chunks.append(chunk)

        complete_chunk = [c for c in chunks if c.type == StreamChunkFromModelType.COMPLETE][0]
        assert complete_chunk.data.token_usage is not None
        assert complete_chunk.data.token_usage.input_tokens == 10
        assert complete_chunk.data.token_usage.output_tokens == 5
        assert complete_chunk.data.token_usage.total_tokens == 15

    async def test_emits_none_token_usage_when_no_tokens(self):
        tool_executor = MagicMock()
        provider = MagicMock()

        def no_metadata_stream(**kwargs):
            async def _gen():
                chunk = make_provider_chunk(text="Response", finish_reason="STOP")
                chunk.usage_metadata = None
                yield chunk
            return _gen()

        provider.generate_content_stream_primary = no_metadata_stream

        agent = make_g8e_agent(fn_handler=tool_executor)
        context = make_agent_inputs()
        gen_config = make_gen_config()
        g8ed_event_service = make_g8ed_event_service()

        chunks = []
        async for chunk in agent._stream_with_tool_loop(
            contents=[],
            generation_config=gen_config,
            model_name="test-model",
            inputs=context,
            g8ed_event_service=g8ed_event_service,
            llm_provider=provider,
        ):
            chunks.append(chunk)

        complete_chunk = [c for c in chunks if c.type == StreamChunkFromModelType.COMPLETE][0]
        assert complete_chunk.data.token_usage is None


# =============================================================================
# TEST: _stream_with_tool_loop - Grounding Metadata
# =============================================================================

@pytest.mark.asyncio(loop_scope="session")
class TestGroundingMetadata:

    async def test_emits_citations_when_grounding_present(self):
        from app.llm.llm_types import ToolCall

        tool_executor = MagicMock()
        provider = MagicMock()

        call_count = 0
        def stream(**kwargs):
            nonlocal call_count
            call_count += 1
            if call_count == 1:
                async def _gen():
                    yield make_provider_chunk(text="Response")
                    yield make_provider_chunk(
                        tool_calls=[ToolCall(name="search_web", args={"query": "test"})],
                        finish_reason="STOP",
                    )
                return _gen()
            else:
                async def _gen():
                    yield make_provider_chunk(text="Done")
                    yield make_provider_chunk(finish_reason="STOP")
                return _gen()

        provider.generate_content_stream_primary = stream

        agent = make_g8e_agent(fn_handler=tool_executor)
        context = make_agent_inputs()
        gen_config = make_gen_config()
        g8ed_event_service = make_g8ed_event_service()

        async def mock_execute(**kwargs):
            grounding = GroundingMetadata(grounding_used=True, sources=[])
            result = SearchWebResult(
                success=True,
                query="test",
                results=[],
            )
            result_out = kwargs.get('result_out')
            if result_out is not None:
                result_out.append([
                    ToolCallResponse(
                        tool_name="search_web",
                        flattened_response={"success": True, "query": "test"},
                        grounding=grounding,
                    )
                ])
            yield StreamChunkFromModel(type=StreamChunkFromModelType.TOOL_RESULT, data=StreamChunkData())
            yield StreamChunkFromModel(type=StreamChunkFromModelType.TOOL_RESULT, data=StreamChunkData(result=result))

        with patch("app.services.ai.agent.execute_turn_tool_calls", side_effect=mock_execute):
            chunks = []
            async for chunk in agent._stream_with_tool_loop(
                contents=[],
                generation_config=gen_config,
                model_name="test-model",
                inputs=context,
                g8ed_event_service=g8ed_event_service,
                llm_provider=provider,
            ):
                chunks.append(chunk)

        assert any(c.type == StreamChunkFromModelType.CITATIONS for c in chunks)

    async def test_skips_citations_when_no_grounding(self):
        tool_executor = MagicMock()
        provider = MagicMock()

        def stream(**kwargs):
            async def _gen():
                yield make_provider_chunk(text="Response")
                yield make_provider_chunk(finish_reason="STOP")
            return _gen()

        provider.generate_content_stream_primary = stream

        agent = make_g8e_agent(fn_handler=tool_executor)
        context = make_agent_inputs()
        gen_config = make_gen_config()
        g8ed_event_service = make_g8ed_event_service()

        chunks = []
        async for chunk in agent._stream_with_tool_loop(
            contents=[],
            generation_config=gen_config,
            model_name="test-model",
            inputs=context,
            g8ed_event_service=g8ed_event_service,
            llm_provider=provider,
        ):
            chunks.append(chunk)

        assert not any(c.type == StreamChunkFromModelType.CITATIONS for c in chunks)


# =============================================================================
# TEST: _stream_with_tool_loop - COMPLETE Emission
# =============================================================================

@pytest.mark.asyncio(loop_scope="session")
class TestCompleteEmission:

    async def test_emits_complete_with_finish_reason(self):
        tool_executor = MagicMock()
        provider = MagicMock()

        def stream(**kwargs):
            async def _gen():
                yield make_provider_chunk(text="Response")
                yield make_provider_chunk(finish_reason="STOP")
            return _gen()

        provider.generate_content_stream_primary = stream

        agent = make_g8e_agent(fn_handler=tool_executor)
        context = make_agent_inputs()
        gen_config = make_gen_config()
        g8ed_event_service = make_g8ed_event_service()

        chunks = []
        async for chunk in agent._stream_with_tool_loop(
            contents=[],
            generation_config=gen_config,
            model_name="test-model",
            inputs=context,
            g8ed_event_service=g8ed_event_service,
            llm_provider=provider,
        ):
            chunks.append(chunk)

        complete_chunk = [c for c in chunks if c.type == StreamChunkFromModelType.COMPLETE][0]
        assert complete_chunk.data.finish_reason == "STOP"

    async def test_emits_complete_with_default_finish_reason_when_none(self):
        tool_executor = MagicMock()
        provider = MagicMock()

        def stream(**kwargs):
            async def _gen():
                chunk = make_provider_chunk(text="Response")
                chunk.finish_reason = None
                yield chunk
            return _gen()

        provider.generate_content_stream_primary = stream

        agent = make_g8e_agent(fn_handler=tool_executor)
        context = make_agent_inputs()
        gen_config = make_gen_config()
        g8ed_event_service = make_g8ed_event_service()

        chunks = []
        async for chunk in agent._stream_with_tool_loop(
            contents=[],
            generation_config=gen_config,
            model_name="test-model",
            inputs=context,
            g8ed_event_service=g8ed_event_service,
            llm_provider=provider,
        ):
            chunks.append(chunk)

        complete_chunk = [c for c in chunks if c.type == StreamChunkFromModelType.COMPLETE][0]
        assert complete_chunk.data.finish_reason == DEFAULT_FINISH_REASON
