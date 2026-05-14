# Copyright (c) 2026 Lateralus Labs, LLC.

import pytest
import asyncio
from unittest.mock import MagicMock
from app.services.operator.filesystem_service import OperatorFilesystemService
from app.models.command_request_payloads import FsGrepRequestPayload
from app.models.http_context import G8eHttpContext
from app.models.investigations import EnrichedInvestigationContext
from app.constants.status import ComponentName

@pytest.mark.asyncio
async def test_filesystem_service_grep_import_fix():
    # This test primarily verifies that the imports in filesystem_service.py are correct
    # and don't raise NameError when the methods are called/referenced.
    pubsub_service = MagicMock()
    execution_service = MagicMock()
    execution_service.client_event_service.publish_command_event = MagicMock(return_value=asyncio.Future())
    execution_service.client_event_service.publish_command_event.return_value.set_result(None)
    
    investigation_service = MagicMock()
    
    service = OperatorFilesystemService(
        pubsub_service=pubsub_service,
        execution_service=execution_service,
        investigation_service=investigation_service
    )
    
    # Mock execution_service.resolve_operators to return a mock operator
    mock_operator = MagicMock()
    mock_operator.id = "op-123"
    mock_operator.operator_session_id = "sess-456"
    execution_service.resolve_operators.return_ok = [mock_operator]
    execution_service.resolve_operators.return_value = [mock_operator]
    
    # Mock execution_service.execute to return a valid result
    mock_result = MagicMock()
    mock_result.status = "completed"
    mock_result.output = "match"
    mock_result.error = None
    
    mock_envelope = MagicMock()
    # We need to use a real class or mock that satisfies isinstance if possible,
    # but since we just want to verify NameError is gone, let's just make it return a Mock
    # that has the fields. The code does: if envelope and isinstance(envelope.payload, FsGrepResultPayload):
    # To bypass the isinstance check without importing FsGrepResultPayload in the test, 
    # we can just let it be False and verify the rest of the method.
    mock_envelope.payload = None 
    
    execution_service.execute = MagicMock(return_value=asyncio.Future())
    execution_service.execute.return_value.set_result((mock_result, mock_envelope))
    
    args = FsGrepRequestPayload(
        path="/tmp",
        pattern="test",
        execution_id="exec-1",
        target_operators=["op-123"]
    )
    
    investigation = MagicMock(spec=EnrichedInvestigationContext)
    investigation.operator_documents = []
    
    g8e_context = G8eHttpContext(
        case_id="case-1",
        investigation_id="inv-1",
        web_session_id="web-1",
        user_id="user-1",
        organization_id="org-1",
        source_component=ComponentName.CLIENT
    )
    
    # This should not raise NameError for FsGrepToolResult
    result = await service.execute_fs_grep(args, investigation, g8e_context)
    assert result is not None
    assert result.success is True
