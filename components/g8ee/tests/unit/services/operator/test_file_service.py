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
from unittest.mock import AsyncMock, MagicMock

from app.constants.status import FileOperation, ExecutionStatus, ComponentName
from app.constants.events import EventType
from app.models.command_request_payloads import FileEditRequestPayload
from app.models.http_context import G8eHttpContext
from app.models.investigations import EnrichedInvestigationContext
from app.models.pubsub_messages import FileEditResultPayload
from app.models.tool_results import CommandInternalResult
from app.models.operators import OperatorDocument
from app.services.operator.file_service import OperatorFileService
from tests.fakes.builder import build_command_service

@pytest.mark.asyncio
async def test_execute_file_edit_read_returns_content():
    """Verify that execute_file_edit with READ operation returns the file content."""
    # 1. Setup
    command_service = build_command_service()
    file_service = command_service._file_service
    
    # Mock execution_service.execute to return a successful read result with FileEditResultPayload envelope
    mock_content = "test file content"
    internal_result = CommandInternalResult(
        status=ExecutionStatus.COMPLETED,
        output=""
    )
    mock_envelope = MagicMock()
    mock_envelope.payload = FileEditResultPayload(
        execution_id="exec-123",
        operation="read",
        file_path="/etc/test",
        status=ExecutionStatus.COMPLETED,
        content=mock_content
    )
    file_service.execution_service.execute = AsyncMock(return_value=(internal_result, mock_envelope))
    
    # Mock operator resolution
    mock_operator = MagicMock(spec=OperatorDocument)
    mock_operator.id = "op-123"
    mock_operator.operator_session_id = "sess-123"
    file_service.execution_service.resolve_target_operator = MagicMock(return_value=mock_operator)
    
    # 2. Execute
    args = FileEditRequestPayload(
        file_path="/etc/test",
        operation=FileOperation.READ,
        justification="Reading test file",
        execution_id="exec-123",
        target_operators=["all"]
    )
    g8e_context = G8eHttpContext(
        case_id="case-123",
        investigation_id="inv-123",
        web_session_id="web-123",
        user_id="user-123",
        source_component=ComponentName.G8EE
    )
    investigation = EnrichedInvestigationContext(
        id="inv-123",
        case_id="case-123",
        user_id="user-123",
        sentinel_mode=False,
        operator_documents=[mock_operator]
    )
    
    result = await file_service.execute_file_edit(args, g8e_context, investigation)
    
    # 3. Assert
    assert result.success is True
    # THIS IS THE BUG: result.content is currently None for READ operations
    assert result.content == mock_content

@pytest.mark.asyncio
async def test_execute_file_edit_read_broadcasts_content():
    """Verify that execute_file_edit with READ operation broadcasts the file content."""
    # 1. Setup
    command_service = build_command_service()
    file_service = command_service._file_service
    
    # Mock execution_service.execute with FileEditResultPayload envelope
    mock_content = "test file content"
    internal_result = CommandInternalResult(
        status=ExecutionStatus.COMPLETED,
        output=""
    )
    mock_envelope = MagicMock()
    mock_envelope.payload = FileEditResultPayload(
        execution_id="exec-123",
        operation="read",
        file_path="/etc/test",
        status=ExecutionStatus.COMPLETED,
        content=mock_content
    )
    file_service.execution_service.execute = AsyncMock(return_value=(internal_result, mock_envelope))
    
    # Mock operator resolution
    mock_operator = MagicMock(spec=OperatorDocument)
    mock_operator.id = "op-123"
    mock_operator.operator_session_id = "sess-123"
    file_service.execution_service.resolve_target_operator = MagicMock(return_value=mock_operator)
    
    # 2. Execute
    args = FileEditRequestPayload(
        file_path="/etc/test",
        operation=FileOperation.READ,
        justification="Reading test file",
        execution_id="exec-123",
        target_operators=["all"]
    )
    g8e_context = G8eHttpContext(
        case_id="case-123",
        investigation_id="inv-123",
        web_session_id="web-123",
        user_id="user-123",
        source_component=ComponentName.G8EE
    )
    investigation = EnrichedInvestigationContext(
        id="inv-123",
        case_id="case-123",
        user_id="user-123",
        sentinel_mode=False,
        operator_documents=[mock_operator]
    )
    
    file_service.event_service.publish_command_event = AsyncMock()
    
    await file_service.execute_file_edit(args, g8e_context, investigation)
    
    # 3. Assert
    # Check that publish_command_event was called with FileEditBroadcastEvent containing the content
    # Find the completion event call
    completion_call = None
    for call in file_service.event_service.publish_command_event.call_args_list:
        if call[0][0] == EventType.OPERATOR_FILE_EDIT_COMPLETED:
            completion_call = call
            break
            
    assert completion_call is not None
    broadcast_event = completion_call[0][1]
    # THIS IS THE BUG: broadcast_event.content is currently None for READ operations
    assert broadcast_event.content == mock_content
