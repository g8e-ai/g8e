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

import asyncio
import pytest
from unittest.mock import MagicMock, AsyncMock

from app.models.agent import StreamChunkFromModel, StreamChunkFromModelType
from app.services.ai.agent import g8eEngine
from tests.fakes.agent_helpers import (
    make_agent_inputs,
    make_g8e_agent,
    make_client_event_service,
    make_gen_config,
    make_provider_chunk,
)

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]

class TestAgentCancellation:
    """Tests for immediate and graceful cancellation in g8eEngine."""

    async def test_stream_with_tool_loop_cancels_immediately_during_llm_call(self):
        """Verify that a cancellation during the LLM call is propagated immediately."""
        tool_executor = MagicMock()
        provider = MagicMock()
        
        # A stream that takes forever or until cancelled
        async def slow_stream(**kwargs):
            try:
                yield make_provider_chunk(text="Thinking...")
                await asyncio.sleep(10)
                yield make_provider_chunk(text="Still thinking...")
            except asyncio.CancelledError:
                # This is what we expect
                raise

        provider.generate_content_stream_primary = slow_stream
        
        agent = make_g8e_agent(fn_handler=tool_executor)
        inputs = make_agent_inputs()
        inputs.model_to_use = "test-model"
        inputs.generation_config = make_gen_config()
        client_event_service = make_client_event_service()

        # Run the stream in a task so we can cancel it
        async def run_stream():
            chunks = []
            async for chunk in agent.stream_response(
                inputs=inputs,
                client_event_service=client_event_service,
                llm_provider=provider
            ):
                chunks.append(chunk)
            return chunks

        task = asyncio.create_task(run_stream())
        
        # Wait a bit for it to start
        await asyncio.sleep(0.1)
        
        # Cancel the task - this simulates the stop button
        task.cancel()
        
        with pytest.raises(asyncio.CancelledError):
            await task

    async def test_stream_with_tool_loop_cancels_between_tool_calls(self):
        """Verify that cancellation between sequential tool calls is respected."""
        tool_executor = MagicMock()
        provider = MagicMock()
        
        # LLM turn that yields tool calls
        async def tool_calling_stream(**kwargs):
            # We need to use proper types from app.llm.llm_types
            from app.llm.llm_types import ToolCall
            yield make_provider_chunk(tool_calls=[
                ToolCall(name="tool1", args={}, id="call1"),
                ToolCall(name="tool2", args={}, id="call2")
            ])
            yield make_provider_chunk(finish_reason="STOP")

        provider.generate_content_stream_primary = tool_calling_stream

        # Tool executor that waits
        async def slow_tool(*args, **kwargs):
            await asyncio.sleep(0.5)
            # Return a minimal tool result
            from app.models.tool_results import CommandExecutionResult
            return CommandExecutionResult(success=True, output="done")

        tool_executor.execute_tool_call = AsyncMock(side_effect=slow_tool)

        agent = make_g8e_agent(fn_handler=tool_executor)
        inputs = make_agent_inputs()
        inputs.model_to_use = "test-model"
        inputs.generation_config = make_gen_config()
        # Force sequential execution for this test
        inputs.request_settings.llm.llm_parallel_tool_calls = False
        client_event_service = make_client_event_service()

        async def run_stream():
            chunks = []
            async for chunk in agent.stream_response(
                inputs=inputs,
                client_event_service=client_event_service,
                llm_provider=provider
            ):
                chunks.append(chunk)
            return chunks

        task = asyncio.create_task(run_stream())
        
        # Wait for first tool call to start
        await asyncio.sleep(0.1)
        
        # Cancel during the first tool call
        task.cancel()
        
        with pytest.raises(asyncio.CancelledError):
            await task
        
        # Verify that tool2 was NEVER called because tool1 was cancelled
        # and the loop should have broken
        # We check call_count of execute_tool_call
        # It should be 1 (for tool1)
        # Note: If the test runner environment is extremely fast, both might have started,
        # but the sequential executor should ensure we only start the second one
        # after the first one finishes. Since we cancel during the first sleep,
        # the second one should never start.
        assert tool_executor.execute_tool_call.call_count == 1
