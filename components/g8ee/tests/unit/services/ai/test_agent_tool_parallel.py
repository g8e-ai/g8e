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
from unittest.mock import AsyncMock, MagicMock
from app.models.agent import ToolCall, StreamChunkData, StreamChunkFromModelType
from app.models.settings import G8eeUserSettings, LLMSettings
from app.services.ai.agent_tool_loop import execute_turn_tool_calls, ToolCallResult
from app.models.http_context import G8eHttpContext
from app.models.investigations import EnrichedInvestigationContext

@pytest.mark.asyncio
async def test_execute_turn_tool_calls_parallel():
    # Setup mocks
    pending_tool_calls = [
        ToolCall(id="call_1", name="tool_1", args={}),
        ToolCall(id="call_2", name="tool_2", args={}),
    ]
    
    tool_executor = MagicMock()
    investigation = MagicMock(spec=EnrichedInvestigationContext)
    g8e_context = MagicMock(spec=G8eHttpContext)
    g8ed_event_service = AsyncMock()
    
    request_settings = MagicMock(spec=G8eeUserSettings)
    request_settings.llm = LLMSettings(llm_parallel_tool_calls=True)
    
    # Track execution order
    execution_order = []
    
    async def mock_orchestrate(tool_call, **kwargs):
        execution_order.append(f"start_{tool_call.name}")
        await asyncio.sleep(0.1)  # Simulate some work
        execution_order.append(f"end_{tool_call.name}")
        return ToolCallResult(
            tool_name=tool_call.name,
            call_info=StreamChunkData(tool_name=tool_call.name, execution_id="exec_id"),
            result_info=StreamChunkData(execution_id="exec_id", success=True),
            result=MagicMock()
        )

    with MagicMock() as mock_orchestrate_patch:
        import app.services.ai.agent_tool_loop
        app.services.ai.agent_tool_loop.orchestrate_tool_execution = mock_orchestrate

        result_out = []
        chunks = []
        async for chunk in execute_turn_tool_calls(
            pending_tool_calls=pending_tool_calls,
            tool_executor=tool_executor,
            investigation=investigation,
            g8e_context=g8e_context,
            result_out=result_out,
            request_settings=request_settings,
            g8ed_event_service=g8ed_event_service,
        ):
            chunks.append(chunk)

    # Verify results
    assert len(result_out[0]) == 2
    assert result_out[0][0].tool_name == "tool_1"
    assert result_out[0][1].tool_name == "tool_2"
    
    # Verify parallel execution: both should have started before either finished
    # The order in execution_order should be [start_tool_1, start_tool_2, end_tool_1, end_tool_2]
    # (or vice versa for 1 and 2, but both starts must precede both ends)
    assert execution_order[0].startswith("start_")
    assert execution_order[1].startswith("start_")
    assert execution_order[2].startswith("end_")
    assert execution_order[3].startswith("end_")

@pytest.mark.asyncio
async def test_execute_turn_tool_calls_sequential():
    # Setup mocks
    pending_tool_calls = [
        ToolCall(id="call_1", name="tool_1", args={}),
        ToolCall(id="call_2", name="tool_2", args={}),
    ]
    
    tool_executor = MagicMock()
    investigation = MagicMock(spec=EnrichedInvestigationContext)
    g8e_context = MagicMock(spec=G8eHttpContext)
    g8ed_event_service = AsyncMock()
    
    request_settings = MagicMock(spec=G8eeUserSettings)
    request_settings.llm = LLMSettings(llm_parallel_tool_calls=False)
    
    # Track execution order
    execution_order = []
    
    async def mock_orchestrate(tool_call, **kwargs):
        execution_order.append(f"start_{tool_call.name}")
        await asyncio.sleep(0.1)
        execution_order.append(f"end_{tool_call.name}")
        return ToolCallResult(
            tool_name=tool_call.name,
            call_info=StreamChunkData(tool_name=tool_call.name, execution_id="exec_id"),
            result_info=StreamChunkData(execution_id="exec_id", success=True),
            result=MagicMock()
        )

    import app.services.ai.agent_tool_loop
    # We need to patch it in the module
    original_orchestrate = app.services.ai.agent_tool_loop.orchestrate_tool_execution
    app.services.ai.agent_tool_loop.orchestrate_tool_execution = mock_orchestrate
    
    try:
        result_out = []
        async for _ in execute_turn_tool_calls(
            pending_tool_calls=pending_tool_calls,
            tool_executor=tool_executor,
            investigation=investigation,
            g8e_context=g8e_context,
            result_out=result_out,
            request_settings=request_settings,
            g8ed_event_service=g8ed_event_service,
        ):
            pass
    finally:
        app.services.ai.agent_tool_loop.orchestrate_tool_execution = original_orchestrate

    # Verify sequential execution: tool_1 must finish before tool_2 starts
    assert execution_order == ["start_tool_1", "end_tool_1", "start_tool_2", "end_tool_2"]
