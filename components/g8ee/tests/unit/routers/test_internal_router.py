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
from unittest.mock import AsyncMock, MagicMock, patch
from fastapi import Request
from app.constants import ComponentName
from app.routers.internal_router import (
    internal_chat,
    stop_ai_processing,
    operator_approval_respond,
    execute_direct_command,
    update_case,
    delete_case,
)
from app.models.internal_api import (
    ChatMessageRequest,
    StopAIRequest,
    OperatorApprovalResponse,
    DirectCommandRequest,
)
from app.models.cases import CaseUpdateRequest
from app.models.http_context import BoundOperator, VSOHttpContext
from app.errors import ResourceNotFoundError
from tests.fakes.factories import build_case_model, create_investigation_data

@pytest.fixture
def vso_context():
    return VSOHttpContext(
        user_id="user-123",
        web_session_id="session-123",
        case_id="case-123",
        investigation_id="inv-123",
        organization_id="org-123",
        source_component=ComponentName.VSOD
    )

@pytest.mark.asyncio
async def test_internal_chat_new_case(vso_context, task_tracker):
    vso_context.new_case = True
    request = ChatMessageRequest(message="test message", sentinel_mode=True)
    
    # Mock dependencies
    mock_user_settings = MagicMock()
    mock_chat_pipeline = MagicMock()
    mock_chat_task_manager = MagicMock()
    mock_case_service = MagicMock()
    mock_investigation_service = MagicMock()
    mock_attachment_service = MagicMock()
    mock_event_service = MagicMock()
    
    mock_case = build_case_model(case_id="case-123", user_id="user-123")
    mock_case_service.create_case = AsyncMock(return_value=mock_case)
    
    mock_inv = create_investigation_data(investigation_id="inv-123", case_id="case-123")
    mock_investigation_service.create_investigation = AsyncMock(return_value=mock_inv)

    with task_tracker.patch_create_task("app.routers.internal_router"):
        response = await internal_chat(
            request=request,
            user_settings=mock_user_settings,
            chat_pipeline=mock_chat_pipeline,
            chat_task_manager=mock_chat_task_manager,
            case_service=mock_case_service,
            investigation_service=mock_investigation_service,
            attachment_service=mock_attachment_service,
            event_service=mock_event_service,
            vso_context=vso_context
        )

    assert response.success is True
    assert response.case_id == "case-123"
    assert response.investigation_id == "inv-123"
    mock_case_service.create_case.assert_called_once()
    mock_investigation_service.create_investigation.assert_called_once()

@pytest.mark.asyncio
async def test_stop_ai_processing(vso_context):
    request = StopAIRequest(investigation_id="inv-123", reason="user cancel", web_session_id="session-123")
    mock_task_manager = MagicMock()
    mock_task_manager.cancel = AsyncMock(return_value=True)
    mock_pipeline = MagicMock()
    
    response = await stop_ai_processing(
        request=request,
        vso_context=vso_context,
        chat_task_manager=mock_task_manager,
        chat_pipeline=mock_pipeline
    )
    
    assert response.success is True
    assert response.was_active is True
    mock_task_manager.cancel.assert_called_once()

@pytest.mark.asyncio
async def test_operator_approval_respond(vso_context):
    vso_context.bound_operators = [
        BoundOperator(operator_id="op-1", operator_session_id="opsess-1")
    ]
    request = OperatorApprovalResponse(approval_id="app-123", approved=True)
    mock_approval_service = MagicMock()
    mock_approval_service.handle_approval_response = AsyncMock()

    response = await operator_approval_respond(
        request=request,
        vso_context=vso_context,
        approval_service=mock_approval_service,
    )

    assert response.success is True
    mock_approval_service.handle_approval_response.assert_called_once_with(request)
    assert request.operator_session_id == "opsess-1"
    assert request.operator_id == "op-1"

@pytest.mark.asyncio
async def test_execute_direct_command(vso_context):
    request = DirectCommandRequest(command="ls", execution_id="exec-123")
    mock_exec_service = MagicMock()
    mock_exec_service.send_command_to_operator = AsyncMock()
    mock_exec_service.send_direct_exec_audit_event = AsyncMock()
    
    response = await execute_direct_command(
        request=request,
        vso_context=vso_context,
        operator_data_service=mock_exec_service
    )
    
    assert response.success is True
    mock_exec_service.send_command_to_operator.assert_called_once()
    mock_exec_service.send_direct_exec_audit_event.assert_called_once()

@pytest.mark.asyncio
async def test_update_case_with_sse(vso_context):
    case_id = "case-123"
    updates = CaseUpdateRequest(title="New Title")
    mock_case_service = MagicMock()
    mock_case = build_case_model(case_id=case_id, title="New Title")
    mock_case_service.update_case = AsyncMock(return_value=mock_case)
    mock_case_service.publish_case_update_sse = AsyncMock()
    
    response = await update_case(
        case_id=case_id,
        updates=updates,
        case_service=mock_case_service,
        vso_context=vso_context
    )
    
    assert response.success is True
    assert response.case.title == "New Title"
    mock_case_service.publish_case_update_sse.assert_called_once()

@pytest.mark.asyncio
async def test_delete_case_success(vso_context):
    case_id = "case-123"
    mock_case_service = MagicMock()
    mock_case_service.get_case = AsyncMock()
    mock_case_service.delete_case = AsyncMock()
    mock_inv_service = MagicMock()
    mock_inv_service.get_case_investigations = AsyncMock(return_value=[])
    mock_inv_service.delete_investigation = AsyncMock()
    mock_cache = MagicMock()
    mock_cache.query_documents = AsyncMock(return_value=[])
    mock_cache.delete_document = AsyncMock()
    
    mock_case = build_case_model(case_id=case_id, user_id="user-123")
    mock_case_service.get_case.return_value = mock_case
    
    await delete_case(
        case_id=case_id,
        case_service=mock_case_service,
        investigation_service=mock_inv_service,
        cache_aside_service=mock_cache,
        vso_context=vso_context
    )
    
    mock_case_service.delete_case.assert_called_once_with(case_id)

@pytest.mark.asyncio
async def test_delete_case_not_found_idempotent(vso_context):
    case_id = "case-missing"
    mock_case_service = MagicMock()
    mock_case_service.get_case = AsyncMock(side_effect=ResourceNotFoundError(
        message="not found",
        resource_type="case",
        resource_id=case_id
    ))
    mock_case_service.delete_case = AsyncMock()
    
    await delete_case(
        case_id=case_id,
        case_service=mock_case_service,
        investigation_service=MagicMock(),
        cache_aside_service=MagicMock(),
        vso_context=vso_context
    )
    
    mock_case_service.delete_case.assert_not_called()
