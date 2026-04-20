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
from app.constants import ComponentName
from app.routers.internal_router import (
    internal_chat,
    stop_ai_processing,
    operator_approval_respond,
    execute_direct_command,
    update_case,
    delete_case,
    _generate_and_update_title,
)
from app.models.internal_api import (
    ChatMessageRequest,
    StopAIRequest,
    OperatorApprovalResponse,
    DirectCommandRequest,
)
from app.models.cases import CaseUpdateRequest
from app.models.agents.title_generator import CaseTitleResult
from app.models.http_context import BoundOperator, G8eHttpContext
from app.errors import ResourceNotFoundError
from tests.fakes.factories import build_case_model, create_investigation_data

@pytest.fixture
def g8e_context():
    return G8eHttpContext(
        user_id="user-123",
        web_session_id="session-123",
        case_id="case-123",
        investigation_id="inv-123",
        organization_id="org-123",
        source_component=ComponentName.G8ED
    )

@pytest.mark.asyncio
async def test_internal_chat_new_case(g8e_context, task_tracker):
    g8e_context.new_case = True
    request = ChatMessageRequest(message="test message", sentinel_mode=True)
    
    # Mock dependencies
    mock_platform_settings = MagicMock()
    mock_user_settings = MagicMock()
    mock_chat_pipeline = MagicMock()
    mock_chat_task_manager = MagicMock()
    mock_case_service = MagicMock()
    mock_investigation_service = MagicMock()
    mock_attachment_service = MagicMock()
    mock_event_service = MagicMock()
    
    mock_case = build_case_model(case_id="case-123", user_id="user-123")
    mock_case_service.create_case = AsyncMock(return_value=mock_case)
    mock_case_service.update_case = AsyncMock(return_value=mock_case)
    mock_case_service.publish_case_update_sse = AsyncMock()
    
    mock_inv = create_investigation_data(investigation_id="inv-123", case_id="case-123")
    mock_investigation_service.create_investigation = AsyncMock(return_value=mock_inv)
    mock_investigation_service.update_investigation = AsyncMock()

    with task_tracker.patch_create_task("app.routers.internal_router"):
        response = await internal_chat(
            request=request,
            platform_settings=mock_platform_settings,
            user_settings=mock_user_settings,
            chat_pipeline=mock_chat_pipeline,
            chat_task_manager=mock_chat_task_manager,
            case_service=mock_case_service,
            investigation_service=mock_investigation_service,
            attachment_service=mock_attachment_service,
            event_service=mock_event_service,
            g8e_context=g8e_context
        )

    assert response.success is True
    assert response.case_id == "case-123"
    assert response.investigation_id == "inv-123"
    mock_case_service.create_case.assert_called_once()
    mock_investigation_service.create_investigation.assert_called_once()

@pytest.mark.asyncio
async def test_internal_chat_missing_investigation(g8e_context, task_tracker):
    g8e_context.investigation_id = None
    g8e_context.new_case = False
    request = ChatMessageRequest(message="test message", sentinel_mode=True)
    
    mock_platform_settings = MagicMock()
    mock_user_settings = MagicMock()
    mock_chat_pipeline = MagicMock()
    mock_chat_task_manager = MagicMock()
    mock_case_service = MagicMock()
    mock_investigation_service = MagicMock()
    mock_attachment_service = MagicMock()
    mock_event_service = MagicMock()

    response = await internal_chat(
        request=request,
        platform_settings=mock_platform_settings,
        user_settings=mock_user_settings,
        chat_pipeline=mock_chat_pipeline,
        chat_task_manager=mock_chat_task_manager,
        case_service=mock_case_service,
        investigation_service=mock_investigation_service,
        attachment_service=mock_attachment_service,
        event_service=mock_event_service,
        g8e_context=g8e_context
    )

    assert response.success is False
    assert response.investigation_id == ""

@pytest.mark.asyncio
async def test_stop_ai_processing(g8e_context):
    request = StopAIRequest(investigation_id="inv-123", reason="user cancel", web_session_id="session-123")
    mock_task_manager = MagicMock()
    mock_task_manager.cancel = AsyncMock(return_value=True)
    mock_pipeline = MagicMock()
    
    response = await stop_ai_processing(
        request=request,
        g8e_context=g8e_context,
        chat_task_manager=mock_task_manager,
        chat_pipeline=mock_pipeline
    )
    
    assert response.success is True

@pytest.mark.asyncio
async def test_generate_and_update_title_success():
    mock_case_service = MagicMock()
    mock_case_service.update_case = AsyncMock(return_value=MagicMock(updated_at="2023-01-01T00:00:00Z"))
    mock_case_service.publish_case_update_sse = AsyncMock()
    mock_investigation_service = MagicMock()
    mock_investigation_service.update_investigation = AsyncMock()
    mock_user_settings = MagicMock()

    with patch("app.routers.internal_router.generate_case_title", new_callable=AsyncMock) as mock_generate:
        mock_generate.return_value = CaseTitleResult(generated_title="My AI Title", fallback=False)

        await _generate_and_update_title(
            message="some message",
            case_id="case-123",
            investigation_id="inv-123",
            web_session_id="session-123",
            user_id="user-123",
            user_settings=mock_user_settings,
            case_service=mock_case_service,
            investigation_service=mock_investigation_service
        )

        mock_generate.assert_called_once_with("some message", settings=mock_user_settings)
        mock_case_service.update_case.assert_called_once()
        assert mock_case_service.update_case.call_args[0][0] == "case-123"
        assert mock_case_service.update_case.call_args[0][1].title == "My AI Title"
        mock_investigation_service.update_investigation.assert_called_once()
        assert mock_investigation_service.update_investigation.call_args[0][0] == "inv-123"
        assert mock_investigation_service.update_investigation.call_args[0][1].case_title == "My AI Title"
        mock_case_service.publish_case_update_sse.assert_called_once()

@pytest.mark.asyncio
async def test_generate_and_update_title_error():
    mock_case_service = MagicMock()
    mock_case_service.update_case = AsyncMock()
    mock_investigation_service = MagicMock()
    mock_user_settings = MagicMock()

    with patch("app.routers.internal_router.generate_case_title", new_callable=AsyncMock) as mock_generate:
        mock_generate.side_effect = Exception("Boom")

        await _generate_and_update_title(
            message="some message",
            case_id="case-123",
            investigation_id="inv-123",
            web_session_id="session-123",
            user_id="user-123",
            user_settings=mock_user_settings,
            case_service=mock_case_service,
            investigation_service=mock_investigation_service
        )

        mock_generate.assert_called_once()
        mock_case_service.update_case.assert_not_called()

@pytest.mark.asyncio
async def test_operator_approval_respond(g8e_context):
    g8e_context.bound_operators = [
        BoundOperator(operator_id="op-1", operator_session_id="opsess-1")
    ]
    request = OperatorApprovalResponse(approval_id="app-123", approved=True)
    mock_approval_service = MagicMock()
    mock_approval_service.handle_approval_response = AsyncMock()

    response = await operator_approval_respond(
        request=request,
        g8e_context=g8e_context,
        approval_service=mock_approval_service
    )

    assert response.success is True
    assert response.approval_id == "app-123"
    assert response.approved is True
    mock_approval_service.handle_approval_response.assert_called_once_with(request)
    assert request.operator_session_id == "opsess-1"
    assert request.operator_id == "op-1"

@pytest.mark.asyncio
async def test_execute_direct_command(g8e_context):
    request = DirectCommandRequest(command="ls", execution_id="exec-123")
    mock_exec_service = MagicMock()
    mock_exec_service.send_command_to_operator = AsyncMock()
    mock_exec_service.send_direct_exec_audit_event = AsyncMock()
    
    response = await execute_direct_command(
        request=request,
        g8e_context=g8e_context,
        operator_data_service=mock_exec_service
    )
    
    assert response.success is True
    mock_exec_service.send_command_to_operator.assert_called_once()
    mock_exec_service.send_direct_exec_audit_event.assert_called_once()

@pytest.mark.asyncio
async def test_get_case(g8e_context):
    from app.routers.internal_router import get_case
    mock_case_service = MagicMock()
    mock_case = build_case_model(case_id="case-123", user_id="user-123")
    mock_case_service.get_case = AsyncMock(return_value=mock_case)

    response = await get_case(
        case_id="case-123",
        case_service=mock_case_service,
        g8e_context=g8e_context
    )

    assert response.success is True
    assert response.case == mock_case
    mock_case_service.get_case.assert_called_once_with("case-123")

@pytest.mark.asyncio
async def test_update_case_with_sse(g8e_context):
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
        g8e_context=g8e_context
    )
    
    assert response.success is True
    assert response.case.title == "New Title"
    call_kwargs = mock_case_service.publish_case_update_sse.call_args.kwargs
    assert call_kwargs["user_id"] == "user-123"
    assert call_kwargs["case_id"] == case_id

@pytest.mark.asyncio
async def test_delete_case_success(g8e_context):
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
        g8e_context=g8e_context
    )
    
    mock_case_service.delete_case.assert_called_once_with(case_id)

@pytest.mark.asyncio
async def test_delete_case_not_found_idempotent(g8e_context):
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
        g8e_context=g8e_context
    )
    
    mock_case_service.delete_case.assert_not_called()
