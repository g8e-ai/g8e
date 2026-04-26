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

import pytest
from unittest.mock import MagicMock, AsyncMock, patch

from app.constants.status import OperatorToolName, CommandErrorType, ComponentName
from app.models.tool_args import StreamOperatorArgs
from app.models.tool_results import CommandExecutionResult
from app.models.operators import ApprovalResult
from app.models.http_context import G8eHttpContext

@pytest.fixture
def sample_g8e_context(unique_web_session_id, unique_user_id, unique_case_id, unique_investigation_id):
    return G8eHttpContext(
        web_session_id=unique_web_session_id,
        user_id=unique_user_id,
        case_id=unique_case_id,
        investigation_id=unique_investigation_id,
        source_component=ComponentName.G8ED
    )

@pytest.fixture
def sample_investigation(enriched_investigation):
    return enriched_investigation

@pytest.fixture
def request_settings(user_settings):
    return user_settings

@pytest.mark.asyncio
async def test_stream_operator_dispatch_integration(
    tool_service,
    sample_g8e_context,
    sample_investigation,
    request_settings
):
    """End-to-end dispatch test for stream_operator tool through AIToolService."""
    
    # 1. Setup mocks
    # Mock the stream_executor.execute_stream method on the tool_service
    mock_result = CommandExecutionResult(
        success=True,
        output="Stream started on 2 hosts",
        execution_id="exec-stream-123",
        command_executed="docker exec g8ep ..."
    )
    
    # We need to mock the executor that was wired into the service
    tool_service.stream_executor.execute_stream = AsyncMock(return_value=mock_result)
    
    tool_args = {
        "hosts": ["web-1", "web-2"],
        "justification": "Deploying operators to web fleet for log analysis",
        "concurrency": 5,
        "timeout_seconds": 300
    }
    
    # 2. Execute tool through AIToolService.execute_tool_call
    # This verifies the handler(self, ...) calling convention and tool registration
    result = await tool_service.execute_tool_call(
        tool_name=OperatorToolName.STREAM_OPERATOR,
        tool_args=tool_args,
        investigation=sample_investigation,
        g8e_context=sample_g8e_context,
        request_settings=request_settings,
        execution_id="test-exec-id"
    )
    
    # 3. Verify results and contract
    assert result.success is True
    assert result.output == "Stream started on 2 hosts"
    
    # Verify the executor was called with correctly validated args
    tool_service.stream_executor.execute_stream.assert_called_once()
    call_args = tool_service.stream_executor.execute_stream.call_args[1]
    
    assert isinstance(call_args["args"], StreamOperatorArgs)
    assert call_args["args"].hosts == ["web-1", "web-2"]
    assert call_args["args"].justification == "Deploying operators to web fleet for log analysis"
    assert call_args["g8e_context"] == sample_g8e_context
    assert call_args["execution_id"] == "test-exec-id"

@pytest.mark.asyncio
async def test_stream_executor_internal_flow(
    tool_service,
    sample_g8e_context
):
    """Test the internal flow of OperatorStreamExecutor (mint -> approve -> exec)."""
    executor = tool_service.stream_executor
    
    # Mock internal dependencies
    executor._internal_http_client.post = AsyncMock(return_value={"token": "dlk_test_token"})
    executor._approval_service.request_stream_approval = AsyncMock(return_value=ApprovalResult(
        approved=True,
        approval_id="app-123"
    ))
    
    # Mock subprocess execution to avoid actual docker calls
    with patch("asyncio.create_subprocess_exec") as mock_exec:
        mock_process = AsyncMock()
        mock_process.communicate.return_value = (b"Success output", b"")
        mock_process.returncode = 0
        mock_exec.return_value = mock_process
        
        args = StreamOperatorArgs(
            hosts=["host1"],
            justification="Testing executor flow",
            arch="amd64",
            timeout_seconds=300
        )
        
        result = await executor.execute_stream(
            args=args,
            g8e_context=sample_g8e_context,
            execution_id="exec-123"
        )
        
        # Verify flow
        assert result.success is True
        executor._internal_http_client.post.assert_called_with(
            "/api/internal/tokens/mint-device-link",
            json={"user_id": sample_g8e_context.user_id}
        )
        executor._approval_service.request_stream_approval.assert_called_once()
        mock_exec.assert_called_once()
        
        # Verify dlk_ token was passed to docker exec
        cmd_args = mock_exec.call_args[0]
        assert "dlk_test_token" in cmd_args
