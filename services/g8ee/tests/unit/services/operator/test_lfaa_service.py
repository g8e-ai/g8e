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
from app.services.operator.lfaa_service import OperatorLFAAService
from app.models.pubsub_messages import G8eMessage
from app.models.command_request_payloads import DirectCommandAuditRequestPayload
from app.models.http_context import G8eHttpContext
from app.constants.events import EventType
from app.constants.status import ComponentName, AITaskId

@pytest.fixture
def mock_pubsub_service():
    service = MagicMock()
    service.pubsub_client = MagicMock()
    service.publish_command = AsyncMock()
    return service

@pytest.fixture
def lfaa_service(mock_pubsub_service):
    return OperatorLFAAService(pubsub_service=mock_pubsub_service)

@pytest.fixture
def valid_g8e_message():
    return G8eMessage(
        id="test_id",
        source_component=ComponentName.G8EE,
        event_type=EventType.OPERATOR_AUDIT_COMMAND_RECORDED,
        operator_id="op_1",
        operator_session_id="sess_1",
        case_id="case_123",
        task_id=AITaskId.DIRECT_COMMAND,
        investigation_id="inv_456",
        web_session_id="web_789",
        payload=DirectCommandAuditRequestPayload(
            command="ls",
            execution_id="exec_1",
            operator_session_id="sess_1"
        )
    )

class TestOperatorLFAAService:
    @pytest.mark.asyncio
    async def test_send_audit_event_success(self, lfaa_service, mock_pubsub_service, valid_g8e_message):
        # Setup: publish_command returns > 0 subscribers
        mock_pubsub_service.publish_command.return_value = 1

        result = await lfaa_service.send_audit_event(valid_g8e_message)

        assert result is True
        mock_pubsub_service.publish_command.assert_called_once_with(
            operator_id="op_1",
            operator_session_id="sess_1",
            command_data=valid_g8e_message
        )

    @pytest.mark.asyncio
    async def test_send_audit_event_no_subscribers(self, lfaa_service, mock_pubsub_service, valid_g8e_message):
        # Setup: publish_command returns 0 subscribers
        mock_pubsub_service.publish_command.return_value = 0

        result = await lfaa_service.send_audit_event(valid_g8e_message)

        assert result is False
        mock_pubsub_service.publish_command.assert_called_once()

    @pytest.mark.asyncio
    async def test_send_audit_event_missing_fields(self, lfaa_service, mock_pubsub_service):
        # Case 1: Missing payload
        msg_no_payload = G8eMessage(
            id="test_id",
            source_component=ComponentName.G8EE,
            event_type=EventType.OPERATOR_AUDIT_COMMAND_RECORDED,
            operator_id="op_1",
            operator_session_id="sess_1",
            case_id="case_123",
            task_id=AITaskId.DIRECT_COMMAND,
            investigation_id="inv_456",
            web_session_id="web_789",
            payload=None
        )
        assert await lfaa_service.send_audit_event(msg_no_payload) is False

        # Case 2: Missing operator_id
        msg_no_op = G8eMessage(
            id="test_id",
            source_component=ComponentName.G8EE,
            event_type=EventType.OPERATOR_AUDIT_COMMAND_RECORDED,
            operator_id=None,
            operator_session_id="sess_1",
            case_id="case_123",
            task_id=AITaskId.DIRECT_COMMAND,
            investigation_id="inv_456",
            web_session_id="web_789",
            payload=DirectCommandAuditRequestPayload(
                command="ls",
                execution_id="exec_1",
                operator_session_id="sess_1"
            )
        )
        assert await lfaa_service.send_audit_event(msg_no_op) is False

        # Case 3: Missing operator_session_id
        msg_no_sess = G8eMessage(
            id="test_id",
            source_component=ComponentName.G8EE,
            event_type=EventType.OPERATOR_AUDIT_COMMAND_RECORDED,
            operator_id="op_1",
            operator_session_id=None,
            case_id="case_123",
            task_id=AITaskId.DIRECT_COMMAND,
            investigation_id="inv_456",
            web_session_id="web_789",
            payload=DirectCommandAuditRequestPayload(
                command="ls",
                execution_id="exec_1",
                operator_session_id="sess_1"
            )
        )
        assert await lfaa_service.send_audit_event(msg_no_sess) is False

        mock_pubsub_service.publish_command.assert_not_called()

    @pytest.mark.asyncio
    async def test_send_audit_event_pubsub_not_initialized(self, lfaa_service, mock_pubsub_service, valid_g8e_message):
        # Case 1: pubsub_service is None
        lfaa_service.pubsub_service = None
        assert await lfaa_service.send_audit_event(valid_g8e_message) is False

        # Case 2: pubsub_client is None
        lfaa_service.pubsub_service = mock_pubsub_service
        mock_pubsub_service.pubsub_client = None
        assert await lfaa_service.send_audit_event(valid_g8e_message) is False

    @pytest.mark.asyncio
    async def test_send_audit_event_exception_handling(self, lfaa_service, mock_pubsub_service, valid_g8e_message):
        mock_pubsub_service.publish_command.side_effect = Exception("Pubsub error")

        result = await lfaa_service.send_audit_event(valid_g8e_message)

        assert result is False

    @pytest.mark.asyncio
    async def test_send_direct_exec_audit_event_success(self, lfaa_service, mock_pubsub_service):
        mock_pubsub_service.publish_command.return_value = 1

        g8e_context = G8eHttpContext(
            case_id="case_123",
            investigation_id="inv_456",
            source_component=ComponentName.G8EE,
            web_session_id="web_789",
            user_id="user_1"
        )
        g8e_context.bound_operators = [
            MagicMock(operator_id="op_1", operator_session_id="sess_1")
        ]

        result = await lfaa_service.send_direct_exec_audit_event(
            command="ls -la",
            execution_id="exec_999",
            g8e_context=g8e_context
        )

        assert result is True
        mock_pubsub_service.publish_command.assert_called_once()
        called_args = mock_pubsub_service.publish_command.call_args[1]
        assert called_args["operator_id"] == "op_1"
        assert called_args["operator_session_id"] == "sess_1"

        msg = called_args["command_data"]
        assert msg.id == "audit_exec_999"
        assert msg.event_type == EventType.OPERATOR_AUDIT_DIRECT_COMMAND_RECORDED
        assert msg.case_id == "case_123"
        assert msg.task_id == AITaskId.DIRECT_COMMAND
        assert isinstance(msg.payload, DirectCommandAuditRequestPayload)
        assert msg.payload.command == "ls -la"
        assert msg.payload.execution_id == "exec_999"

    @pytest.mark.asyncio
    async def test_send_direct_exec_audit_event_no_bound_operators(self, lfaa_service):
        g8e_context = G8eHttpContext(
            case_id="case_123",
            investigation_id="inv_456",
            source_component=ComponentName.G8EE,
            web_session_id="web_789",
            user_id="user_1"
        )
        g8e_context.bound_operators = []

        result = await lfaa_service.send_direct_exec_audit_event("ls", "exec_1", g8e_context)
        assert result is False

    @pytest.mark.asyncio
    async def test_send_direct_exec_audit_event_missing_session_id(self, lfaa_service):
        g8e_context = G8eHttpContext(
            case_id="case_123",
            investigation_id="inv_456",
            source_component=ComponentName.G8EE,
            web_session_id="web_789",
            user_id="user_1"
        )
        g8e_context.bound_operators = [
            MagicMock(operator_id="op_1", operator_session_id=None)
        ]

        result = await lfaa_service.send_direct_exec_audit_event("ls", "exec_1", g8e_context)
        assert result is False
