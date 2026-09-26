# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import pytest
from unittest.mock import AsyncMock, MagicMock

from app.constants import ExecutionStatus, FileOperation, G8EE_COMPONENT
from app.models.command_request_payloads import FileEditRequestPayload
from app.models.http_context import G8eHttpContext
from app.models.investigations import EnrichedInvestigationContext
from app.models.pubsub_messages import FileEditResultPayload
from app.models.tool_results import CommandInternalResult
from app.models.operators import OperatorDocument
from tests.fakes.builder import build_command_service


@pytest.mark.asyncio
async def test_execute_file_edit_read_returns_content():
    """Verify that execute_file_edit with READ operation returns the file content."""
    # 1. Setup
    command_service = build_command_service()
    file_service = command_service._file_service

    # Mock execution_service.execute to return a successful read result with FileEditResultPayload envelope
    mock_content = "test file content"
    internal_result = CommandInternalResult(status=ExecutionStatus.COMPLETED, output="")
    mock_envelope = MagicMock()
    mock_envelope.payload = FileEditResultPayload(
        execution_id="exec-123",
        operation="read",
        file_path="/etc/test",
        status=ExecutionStatus.COMPLETED,
        content=mock_content,
    )
    file_service.execution_service.execute = AsyncMock(
        return_value=(internal_result, mock_envelope)
    )

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
        target_operators=["all"],
    )
    g8e_context = G8eHttpContext(
        case_id="case-123",
        investigation_id="inv-123",
        web_session_id="web-123",
        user_id="user-123",
        source_component=G8EE_COMPONENT,
    )
    investigation = EnrichedInvestigationContext(
        id="inv-123",
        case_id="case-123",
        user_id="user-123",
        sentinel_mode=False,
        operator_documents=[mock_operator],
    )

    result = await file_service.execute_file_edit(args, g8e_context, investigation)

    # 3. Assert
    assert result.success is True
    # THIS IS THE BUG: result.content is currently None for READ operations
    assert result.content == mock_content


