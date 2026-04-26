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

from app.constants import CommandErrorType
from app.models.investigations import EnrichedInvestigationContext, InvestigationModel
from app.models.settings import G8eeUserSettings
from app.models.http_context import G8eHttpContext
from app.models.tool_results import InvestigationContextResult
from app.services.ai.tool_service import AIToolService
from app.services.ai.tools import query_investigation_context as qic_tool
from app.services.investigation.investigation_service import InvestigationService
from app.services.operator.command_service import OperatorCommandService
from app.services.ai.grounding.web_search_provider import WebSearchProvider

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]

@pytest.fixture
def mock_investigation_service():
    return AsyncMock(spec=InvestigationService)

@pytest.fixture
def tool_service(mock_investigation_service):
    mock_op_cmd_svc = MagicMock(spec=OperatorCommandService)
    mock_web_search = MagicMock(spec=WebSearchProvider)
    return AIToolService(
        operator_command_service=mock_op_cmd_svc,
        investigation_service=mock_investigation_service,
        reputation_data_service=AsyncMock(),
        reputation_service=AsyncMock(),
        stake_resolution_data_service=AsyncMock(),
        chat_task_manager=MagicMock(),
        web_search_provider=mock_web_search,
    )

@pytest.fixture
def g8e_context():
    ctx = MagicMock(spec=G8eHttpContext)
    ctx.case_id = "test-case"
    ctx.user_id = "test-user"
    return ctx

@pytest.fixture
def investigation_context():
    inv = MagicMock(spec=EnrichedInvestigationContext)
    inv.id = "inv-123"
    return inv

@pytest.fixture
def user_settings():
    return MagicMock(spec=G8eeUserSettings)

class TestHandleQueryInvestigationContext:
    async def test_no_investigation_id(self, tool_service, g8e_context, user_settings):
        inv = MagicMock(spec=EnrichedInvestigationContext)
        inv.id = None
        
        tool_args = {"data_type": "conversation_history"}
        result = await qic_tool.handle(tool_service,
            tool_args, inv, g8e_context, user_settings, execution_id=None
        )
        
        assert isinstance(result, InvestigationContextResult)
        assert result.success is False
        assert "No investigation ID" in result.error
        assert result.error_type == CommandErrorType.VALIDATION_ERROR

    async def test_invalid_data_type(self, tool_service, investigation_context, g8e_context, user_settings):
        tool_args = {"data_type": "invalid_type"}
        result = await qic_tool.handle(tool_service,
            tool_args, investigation_context, g8e_context, user_settings, execution_id=None
        )
        
        assert result.success is False
        assert "Invalid data_type" in result.error
        assert result.error_type == CommandErrorType.VALIDATION_ERROR

    async def test_conversation_history_with_limit(self, tool_service, mock_investigation_service, investigation_context, g8e_context, user_settings):
        mock_msg = MagicMock()
        mock_msg.model_dump.return_value = {"text": "hello"}
        mock_investigation_service.get_chat_messages.return_value = [mock_msg] * 5
        
        tool_args = {"data_type": "conversation_history", "limit": 2}
        result = await qic_tool.handle(tool_service,
            tool_args, investigation_context, g8e_context, user_settings, execution_id=None
        )
        
        assert result.success is True
        assert result.data_type == "conversation_history"
        assert len(result.data) == 2
        assert result.item_count == 2
        mock_investigation_service.get_chat_messages.assert_called_once_with("inv-123")

    async def test_investigation_status_success(self, tool_service, mock_investigation_service, investigation_context, g8e_context, user_settings):
        mock_inv = MagicMock(spec=InvestigationModel)
        mock_inv.model_dump.return_value = {"id": "inv-123", "status": "Open"}
        mock_investigation_service.get_investigation.return_value = mock_inv
        
        tool_args = {"data_type": "investigation_status"}
        result = await qic_tool.handle(tool_service,
            tool_args, investigation_context, g8e_context, user_settings, execution_id=None
        )
        
        assert result.success is True
        assert result.data_type == "investigation_status"
        assert result.data["status"] == "Open"
        assert result.item_count == 1

    async def test_investigation_status_not_found(self, tool_service, mock_investigation_service, investigation_context, g8e_context, user_settings):
        mock_investigation_service.get_investigation.return_value = None
        
        tool_args = {"data_type": "investigation_status"}
        result = await qic_tool.handle(tool_service,
            tool_args, investigation_context, g8e_context, user_settings, execution_id=None
        )
        
        assert result.success is False
        assert "Investigation not found" in result.error
        assert result.error_type == CommandErrorType.VALIDATION_ERROR

    async def test_operator_actions_success(self, tool_service, mock_investigation_service, investigation_context, g8e_context, user_settings):
        mock_investigation_service.get_operator_actions_for_ai_context.return_value = "action1\naction2"
        
        tool_args = {"data_type": "operator_actions"}
        result = await qic_tool.handle(tool_service,
            tool_args, investigation_context, g8e_context, user_settings, execution_id=None
        )
        
        assert result.success is True
        assert result.data == "action1\naction2"
        assert result.item_count == 1

    async def test_service_exception_handling(self, tool_service, mock_investigation_service, investigation_context, g8e_context, user_settings):
        mock_investigation_service.get_chat_messages.side_effect = Exception("Service failure")
        
        tool_args = {"data_type": "conversation_history"}
        result = await qic_tool.handle(tool_service,
            tool_args, investigation_context, g8e_context, user_settings, execution_id=None
        )
        
        assert result.success is False
        assert "Service failure" in result.error
        assert result.error_type == CommandErrorType.EXECUTION_ERROR
