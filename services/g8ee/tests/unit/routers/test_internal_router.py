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

import re
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from app.constants import ComponentName
from app.errors import ResourceNotFoundError
from app.models.agents.title_generator import CaseTitleResult
from app.models.cases import (
    CaseGetRequest,
    CaseUpdateRequest,
    CaseDeleteRequest,
)
from app.models.http_context import BoundOperator, G8eHttpContext, RequestContext
from app.models.internal_api import (
    ChatMessageRequest,
    DirectCommandRequest,
    ResourceCreationRequest,
    OperatorApprovalResponse,
    OperatorBindRequest,
    OperatorSlotClaimRequest,
    OperatorSlotCreationRequest,
    OperatorUnbindRequest,
    StopAIRequest,
)
from app.routers.internal_router import (
    _generate_and_update_title,
    bind_operators,
    claim_operator_slot,
    create_operator_slot,
    delete_case,
    execute_direct_command,
    internal_chat,
    operator_approval_respond,
    stop_ai_processing,
    unbind_operators,
    update_case,
)
from app.services.ai.chat_task_manager import BackgroundTaskManager
from tests.fakes.factories import build_case_model, create_investigation_data

# Canonical API key format from protocol/constants/api_key_patterns.json
API_KEY_OPERATOR_REGEX = re.compile(r"^g8e_[a-f0-9]{8}_[a-f0-9]{64}$")
API_KEY_REGULAR_REGEX = re.compile(r"^g8e_[a-f0-9]{64}$")

@pytest.fixture
def g8e_context():
    return G8eHttpContext(
        user_id="user-123",
        web_session_id="session-123",
        case_id="case-123",
        investigation_id="inv-123",
        organization_id="org-123",
        source_component=ComponentName.CLIENT
    )


@pytest.fixture
def request_context():
    """Body-based RequestContext mirroring g8e_context for migrated endpoints."""
    return RequestContext(
        user_id="user-123",
        web_session_id="session-123",
        case_id="case-123",
        investigation_id="inv-123",
        organization_id="org-123",
        source_component=ComponentName.CLIENT,
    )

@pytest.mark.asyncio
async def test_internal_chat_new_case(request_context, task_tracker):
    request = ChatMessageRequest(
        context=request_context,
        message="test message",
        sentinel_mode=True,
        resource_creation=ResourceCreationRequest(create_case=True)
    )

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
        )

    assert response.success is True
    assert response.case_id == "case-123"
    assert response.investigation_id == "inv-123"
    mock_case_service.create_case.assert_called_once()
    mock_investigation_service.create_investigation.assert_called_once()

@pytest.mark.asyncio
async def test_internal_chat_missing_investigation(request_context, task_tracker):
    request_context = request_context.model_copy(update={"investigation_id": ""})
    request = ChatMessageRequest(context=request_context, message="test message", sentinel_mode=True)

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
    )

    assert response.success is False
    assert response.investigation_id == ""

@pytest.mark.asyncio
async def test_stop_ai_processing(request_context):
    request = StopAIRequest(context=request_context, reason="user cancel")
    # Use spec so an incorrect kwarg name to cancel() raises TypeError in tests,
    # matching production behavior and preventing regressions like the stop-button 500.
    mock_task_manager = MagicMock(spec=BackgroundTaskManager)
    mock_task_manager.cancel = AsyncMock(return_value=True)
    mock_pipeline = MagicMock()

    response = await stop_ai_processing(
        request=request,
        chat_task_manager=mock_task_manager,
        chat_pipeline=mock_pipeline
    )

    assert response.success is True
    mock_task_manager.cancel.assert_awaited_once_with(
        task_id="inv-123",
        reason="user cancel",
        web_session_id="session-123",
        user_id=request_context.user_id,
        case_id=request_context.case_id,
        event_service=mock_pipeline.event_service,
    )

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
            organization_id="org-123",
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
            organization_id="org-123",
            user_settings=mock_user_settings,
            case_service=mock_case_service,
            investigation_service=mock_investigation_service
        )

        mock_generate.assert_called_once()
        mock_case_service.update_case.assert_not_called()

@pytest.mark.asyncio
async def test_operator_approval_respond(request_context):
    request_context = request_context.model_copy(update={
        "bound_operators": [BoundOperator(operator_id="op-1", operator_session_id="opsess-1")],
    })
    request = OperatorApprovalResponse(context=request_context, approval_id="app-123", approved=True)
    mock_approval_service = MagicMock()
    mock_approval_service.handle_approval_response = AsyncMock()

    response = await operator_approval_respond(
        request=request,
        approval_service=mock_approval_service
    )

    assert response.success is True
    assert response.approval_id == "app-123"
    assert response.approved is True
    mock_approval_service.handle_approval_response.assert_called_once_with(request)
    assert request.operator_session_id == "opsess-1"
    assert request.operator_id == "op-1"

@pytest.mark.asyncio
async def test_execute_direct_command(request_context):
    request = DirectCommandRequest(context=request_context, command="ls", execution_id="exec-123")
    mock_command_service = MagicMock()
    mock_command_service.send_command_to_operator = AsyncMock()
    mock_command_service.send_direct_exec_audit_event = AsyncMock()

    response = await execute_direct_command(
        request=request,
        operator_data_service=mock_command_service
    )

    assert response.success is True
    mock_command_service.send_command_to_operator.assert_called_once()
    mock_command_service.send_direct_exec_audit_event.assert_called_once()

@pytest.mark.asyncio
async def test_get_case(request_context):
    from app.routers.internal_router import get_case
    mock_case_service = MagicMock()
    mock_case = build_case_model(case_id="case-123", user_id="user-123")
    mock_case_service.get_case = AsyncMock(return_value=mock_case)

    request = CaseGetRequest(context=request_context)

    response = await get_case(
        case_id="case-123",
        request=request,
        case_service=mock_case_service,
    )

    assert response.success is True
    assert response.case == mock_case
    mock_case_service.get_case.assert_called_once_with("case-123")

@pytest.mark.asyncio
async def test_update_case_with_sse(request_context):
    case_id = "case-123"
    request = CaseUpdateRequest(context=request_context, title="New Title")
    mock_case_service = MagicMock()
    mock_case = build_case_model(case_id=case_id, title="New Title")
    mock_case_service.update_case = AsyncMock(return_value=mock_case)
    mock_case_service.publish_case_update_sse = AsyncMock()

    response = await update_case(
        case_id=case_id,
        request=request,
        case_service=mock_case_service,
    )

    assert response.success is True
    assert response.case.title == "New Title"
    call_kwargs = mock_case_service.publish_case_update_sse.call_args.kwargs
    assert call_kwargs["user_id"] == "user-123"
    assert call_kwargs["case_id"] == case_id

@pytest.mark.asyncio
async def test_delete_case_success(request_context):
    case_id = "case-123"
    request = CaseDeleteRequest(context=request_context)
    mock_case_service = MagicMock()
    mock_case_service.get_case = AsyncMock()
    mock_case_service.delete_case = AsyncMock()
    mock_inv_service = MagicMock()
    mock_inv_data_service = MagicMock()
    mock_inv_data_service.get_case_investigations = AsyncMock(return_value=[])
    mock_inv_service.investigation_data_service = mock_inv_data_service
    mock_inv_service.delete_investigation = AsyncMock()
    mock_cache = MagicMock()
    mock_cache.query_documents = AsyncMock(return_value=[])
    mock_cache.delete_document = AsyncMock()

    mock_case = build_case_model(case_id=case_id, user_id="user-123")
    mock_case_service.get_case.return_value = mock_case

    await delete_case(
        case_id=case_id,
        request=request,
        case_service=mock_case_service,
        investigation_service=mock_inv_service,
        cache_aside_service=mock_cache,
    )

    mock_case_service.delete_case.assert_called_once_with(case_id)

@pytest.mark.asyncio
async def test_delete_case_not_found_idempotent(request_context):
    case_id = "case-missing"
    request = CaseDeleteRequest(context=request_context)
    mock_case_service = MagicMock()
    mock_case_service.get_case = AsyncMock(side_effect=ResourceNotFoundError(
        message="not found",
        resource_type="case",
        resource_id=case_id
    ))
    mock_case_service.delete_case = AsyncMock()

    await delete_case(
        case_id=case_id,
        request=request,
        case_service=mock_case_service,
        investigation_service=MagicMock(),
        cache_aside_service=MagicMock(),
    )

    mock_case_service.delete_case.assert_not_called()

@pytest.mark.asyncio
async def test_create_operator_slot_success(request_context):
    request = OperatorSlotCreationRequest(
        context=request_context,
        slot_number=1,
        operator_type="cloud",
        cloud_subtype="aws",
        name_prefix="operator",
    )

    mock_operator_data_service = MagicMock()
    mock_operator_data_service.create_operator = AsyncMock(return_value=True)
    mock_settings_service = MagicMock()
    mock_api_key_service = MagicMock()
    mock_api_key_service.issue_operator_key = AsyncMock(return_value=True)

    response = await create_operator_slot(
        request=request,
        operator_data_service=mock_operator_data_service,
        settings_service=mock_settings_service,
        api_key_service=mock_api_key_service,
    )

    assert response.success is True
    assert response.operator_id is not None
    mock_operator_data_service.create_operator.assert_called_once()

    # Verify API key format in response matches canonical pattern
    assert API_KEY_OPERATOR_REGEX.match(response.api_key), \
        f"API key {response.api_key} does not match canonical format g8e_[8hex]_[64hex]"

@pytest.mark.asyncio
async def test_claim_operator_slot_success(request_context):
    request = OperatorSlotClaimRequest(
        context=request_context,
        operator_id="op-123",
        operator_session_id="session-123",
        bound_web_session_id="web-session-123",
        operator_type="CLOUD",
    )

    mock_operator_lifecycle_service = MagicMock()
    mock_operator_lifecycle_service.claim_operator_slot = AsyncMock(return_value=True)

    response = await claim_operator_slot(
        request=request,
        operator_lifecycle_service=mock_operator_lifecycle_service,
    )

    assert response.success is True
    mock_operator_lifecycle_service.claim_operator_slot.assert_called_once()

@pytest.mark.asyncio
async def test_bind_operators_success(request_context):
    request = OperatorBindRequest(
        context=request_context,
        operator_ids=["op-123", "op-456"],
    )

    mock_operator_data_service = MagicMock()
    mock_operator = MagicMock()
    mock_operator.user_id = "user-123"
    mock_operator.name = None
    mock_operator.latest_heartbeat_snapshot = None
    mock_operator_data_service.get_operator = AsyncMock(return_value=mock_operator)
    mock_operator_data_service.cache = MagicMock()
    mock_result = MagicMock()
    mock_result.success = True
    mock_operator_data_service.cache.update_document = AsyncMock(return_value=mock_result)

    mock_event_service = MagicMock()
    mock_event_service.publish = AsyncMock(return_value=None)

    response = await bind_operators(
        request=request,
        operator_data_service=mock_operator_data_service,
        event_service=mock_event_service,
    )

    assert response.success is True
    assert response.bound_count == 2
    assert mock_operator_data_service.cache.update_document.call_count == 2
    assert mock_event_service.publish.call_count == 2

@pytest.mark.asyncio
async def test_bind_operators_unauthorized(request_context):
    request = OperatorBindRequest(
        context=request_context,
        operator_ids=["op-123"],
    )

    mock_operator_data_service = MagicMock()
    mock_operator = MagicMock()
    mock_operator.user_id = "different-user"
    mock_operator.name = None
    mock_operator.latest_heartbeat_snapshot = None
    mock_operator_data_service.get_operator = AsyncMock(return_value=mock_operator)

    mock_event_service = MagicMock()
    mock_event_service.publish = AsyncMock(return_value=None)

    response = await bind_operators(
        request=request,
        operator_data_service=mock_operator_data_service,
        event_service=mock_event_service,
    )

    assert response.success is False
    assert response.failed_count == 1
    assert len(response.errors) == 1
    assert "Unauthorized" in response.errors[0]["error"]

@pytest.mark.asyncio
async def test_unbind_operators_success(request_context):
    request = OperatorUnbindRequest(
        context=request_context,
        operator_ids=["op-123", "op-456"],
    )

    mock_operator_data_service = MagicMock()
    mock_operator = MagicMock()
    mock_operator.user_id = "user-123"
    mock_operator.name = None
    mock_operator.latest_heartbeat_snapshot = None
    mock_operator_data_service.get_operator = AsyncMock(return_value=mock_operator)
    mock_operator_data_service.cache = MagicMock()
    mock_result = MagicMock()
    mock_result.success = True
    mock_operator_data_service.cache.update_document = AsyncMock(return_value=mock_result)

    mock_event_service = MagicMock()
    mock_event_service.publish = AsyncMock(return_value=None)

    response = await unbind_operators(
        request=request,
        operator_data_service=mock_operator_data_service,
        event_service=mock_event_service,
    )

    assert response.success is True
    assert response.unbound_count == 2
    assert response.failed_count == 0
    assert len(response.unbound_operator_ids) == 2
    assert mock_operator_data_service.cache.update_document.call_count == 2
    assert mock_event_service.publish.call_count == 2

@pytest.mark.asyncio
async def test_unbind_operators_unauthorized(request_context):
    request = OperatorUnbindRequest(
        context=request_context,
        operator_ids=["op-123"],
    )

    mock_operator_data_service = MagicMock()
    mock_operator = MagicMock()
    mock_operator.user_id = "different-user"
    mock_operator.name = None
    mock_operator.latest_heartbeat_snapshot = None
    mock_operator_data_service.get_operator = AsyncMock(return_value=mock_operator)

    mock_event_service = MagicMock()
    mock_event_service.publish = AsyncMock(return_value=None)

    response = await unbind_operators(
        request=request,
        operator_data_service=mock_operator_data_service,
        event_service=mock_event_service,
    )

    assert response.success is False
    assert response.unbound_count == 0
    assert response.failed_count == 1
    assert len(response.errors) == 1
    assert "Unauthorized" in response.errors[0]["error"]
