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
from unittest.mock import MagicMock, AsyncMock

import pytest

from app.constants.events import EventType
from app.constants.status import AITaskId, ComponentName, ExecutionStatus, CommandErrorType
from app.errors import BusinessLogicError, ValidationError
from app.models.command_request_payloads import CommandRequestPayload
from app.models.operators import OperatorDocument, HeartbeatSnapshot, HeartbeatSystemIdentity
from app.models.pubsub_messages import G8eMessage, G8eoResultEnvelope
from app.services.operator.execution_service import OperatorExecutionService
from tests.fakes.factories import build_g8e_http_context

pytestmark = [pytest.mark.unit]

@pytest.fixture
def mock_pubsub():
    mock = MagicMock()
    mock.is_ready = True
    mock.register_future = MagicMock()
    mock.register_operator_session = AsyncMock()
    mock.publish_command = AsyncMock(return_value=1)
    mock.release_future = MagicMock()
    return mock

@pytest.fixture
def mock_approval():
    return MagicMock()

@pytest.fixture
def mock_event_service():
    mock = MagicMock()
    mock.publish_command_event = AsyncMock()
    return mock

@pytest.fixture
def mock_settings():
    return MagicMock()

@pytest.fixture
def mock_ai_analyzer():
    return MagicMock()

@pytest.fixture
def mock_operator_data():
    return MagicMock()

@pytest.fixture
def mock_investigation():
    return MagicMock()

@pytest.fixture
def execution_service(
    mock_pubsub,
    mock_approval,
    mock_event_service,
    mock_settings,
    mock_ai_analyzer,
    mock_operator_data,
    mock_investigation
):
    return OperatorExecutionService(
        pubsub_service=mock_pubsub,
        approval_service=mock_approval,
        event_service=mock_event_service,
        settings=mock_settings,
        ai_response_analyzer=mock_ai_analyzer,
        operator_data_service=mock_operator_data,
        investigation_service=mock_investigation
    )

class TestOperatorExecutionServiceProperties:
    def test_properties(self, execution_service, mock_pubsub, mock_approval, mock_event_service, mock_operator_data, mock_ai_analyzer, mock_investigation):
        assert execution_service.pubsub_service == mock_pubsub
        assert execution_service.approval_service == mock_approval
        assert execution_service.event_service == mock_event_service
        assert execution_service.operator_data_service == mock_operator_data
        assert execution_service.ai_response_analyzer == mock_ai_analyzer
        assert execution_service.investigation_service == mock_investigation

class TestOperatorExecutionServiceFailCommand:
    @pytest.mark.asyncio
    async def test_fail_command_broadcasts_event(self, execution_service, mock_event_service):
        g8e_context = build_g8e_http_context()
        result = await execution_service._fail_command(
            error_msg="some error",
            error_type=CommandErrorType.EXECUTION_FAILED,
            command="echo hi",
            g8e_context=g8e_context,
            execution_id="exec-1",
            operator_session_id="sess-1",
            status=ExecutionStatus.FAILED,
            approval_id="app-1",
            rule="rule-1",
            violations=["v1"],
            denial_reason="denied",
            feedback_reason="feedback"
        )

        assert result.success is False
        assert result.error == "some error"
        mock_event_service.publish_command_event.assert_called_once()
        args = mock_event_service.publish_command_event.call_args
        assert args[0][0] == EventType.OPERATOR_COMMAND_FAILED
        assert args[0][1].execution_id == "exec-1"

    @pytest.mark.asyncio
    async def test_fail_command_handles_broadcast_exception(self, execution_service, mock_event_service):
        mock_event_service.publish_command_event.side_effect = Exception("broadcast failed")
        g8e_context = build_g8e_http_context()
        # Should not raise exception
        result = await execution_service._fail_command(
            error_msg="some error",
            error_type=CommandErrorType.EXECUTION_FAILED,
            command="echo hi",
            g8e_context=g8e_context,
            execution_id="exec-1",
            operator_session_id="sess-1",
            status=ExecutionStatus.FAILED,
            approval_id="app-1",
            rule="rule-1",
            violations=["v1"],
            denial_reason="denied",
            feedback_reason="feedback"
        )
        assert result.success is False

class TestOperatorExecutionServiceResolveOperators:
    def test_resolve_operators_empty_documents(self, execution_service):
        with pytest.raises(BusinessLogicError, match="No operators bound"):
            execution_service.resolve_operators([], ["op-1"])

    def test_resolve_operators_empty_targets(self, execution_service):
        docs = [OperatorDocument(id="op-1", operator_type="system", user_id="user-1")]
        with pytest.raises(ValidationError, match="target_operators list is empty"):
            execution_service.resolve_operators(docs, [])

    def test_resolve_operators_single_doc(self, execution_service):
        docs = [OperatorDocument(id="op-1", operator_type="system", user_id="user-1")]
        resolved = execution_service.resolve_operators(docs, ["something"])
        assert resolved == docs

    def test_resolve_operators_fleet_sentinels(self, execution_service):
        docs = [
            OperatorDocument(id="op-1", operator_type="system", user_id="user-1"),
            OperatorDocument(id="op-2", operator_type="system", user_id="user-1")
        ]
        for sentinel in ["all", "*", "fleet", "every", "everyone"]:
            resolved = execution_service.resolve_operators(docs, [sentinel])
            assert resolved == docs

    def test_resolve_operators_by_id(self, execution_service):
        docs = [
            OperatorDocument(id="op-1", operator_type="system", user_id="user-1"),
            OperatorDocument(id="op-2", operator_type="system", user_id="user-1")
        ]
        resolved = execution_service.resolve_operators(docs, ["op-2"])
        assert len(resolved) == 1
        assert resolved[0].id == "op-2"

    def test_resolve_operators_by_hostname(self, execution_service):
        docs = [
            OperatorDocument(id="op-1", current_hostname="host-1", operator_type="system", user_id="user-1"),
            OperatorDocument(
                id="op-2",
                operator_type="system",
                user_id="user-1",
                latest_heartbeat_snapshot=HeartbeatSnapshot(
                    system_identity=HeartbeatSystemIdentity(hostname="host-2")
                )
            )
        ]
        resolved = execution_service.resolve_operators(docs, ["host-2"])
        assert len(resolved) == 1
        assert resolved[0].id == "op-2"

    def test_resolve_operators_by_index(self, execution_service):
        docs = [
            OperatorDocument(id="op-1", operator_type="system", user_id="user-1"),
            OperatorDocument(id="op-2", operator_type="system", user_id="user-1")
        ]
        resolved = execution_service.resolve_operators(docs, ["1"])
        assert len(resolved) == 1
        assert resolved[0].id == "op-2"

    def test_resolve_operators_not_found(self, execution_service):
        docs = [
            OperatorDocument(id="op-1", operator_type="system", user_id="user-1"),
            OperatorDocument(id="op-2", operator_type="system", user_id="user-1")
        ]
        with pytest.raises(ValidationError, match="Could not resolve any operators"):
            execution_service.resolve_operators(docs, ["non-existent"])

class TestOperatorExecutionServiceTargetSystems:
    def test_build_target_systems_list(self, execution_service):
        docs = [
            OperatorDocument(id="op-1", current_hostname="host-1", operator_type="system", user_id="user-1"),
            OperatorDocument(id="op-2", operator_type="system", user_id="user-1")
        ]
        systems = execution_service.build_target_systems_list(docs)
        assert len(systems) == 2
        assert systems[0].operator_id == "op-1"
        assert systems[0].hostname == "host-1"
        assert systems[1].operator_id == "op-2"
        assert systems[1].hostname == "None"

class TestOperatorExecutionServiceDispatch:
    @pytest.mark.asyncio
    async def test_execute_calls_dispatch(self, execution_service):
        msg = G8eMessage(
            id="exec-1",
            source_component=ComponentName.G8EE,
            event_type=EventType.OPERATOR_COMMAND_REQUESTED,
            case_id="case-1",
            task_id=AITaskId.COMMAND,
            investigation_id="inv-1",
            web_session_id="web-1",
            operator_id="op-1",
            operator_session_id="sess-1",
            payload=CommandRequestPayload(command="echo hi", execution_id="exec-1")
        )
        g8e_context = build_g8e_http_context()
        # Mock dispatch_command to avoid actual execution
        execution_service.dispatch_command = AsyncMock(return_value=("result", "envelope"))
        res = await execution_service.execute(msg, g8e_context)
        assert res == ("result", "envelope")
        execution_service.dispatch_command.assert_called_once_with(msg, g8e_context, 60)

    @pytest.mark.asyncio
    async def test_dispatch_missing_payload(self, execution_service):
        msg = G8eMessage(
            id="some-id",
            source_component=ComponentName.G8EE,
            event_type=EventType.OPERATOR_COMMAND_REQUESTED,
            case_id="case-1",
            task_id=AITaskId.COMMAND,
            investigation_id="inv-1",
            web_session_id="web-1",
            operator_id="op-1",
            operator_session_id="sess-1",
            payload=None
        )
        g8e_context = build_g8e_http_context()
        # Since payload is Optional in Pydantic but required by envelope_builder,
        # dispatch_command should fail gracefully or the builder will raise ValueError.
        with pytest.raises(Exception): # Catching general Exception for now as it might be ValueError from builder
            await execution_service.dispatch_command(msg, g8e_context)

    @pytest.mark.asyncio
    async def test_dispatch_missing_operator_info(self, execution_service):
        msg = G8eMessage(
            id="exec-1",
            source_component=ComponentName.G8EE,
            event_type=EventType.OPERATOR_COMMAND_REQUESTED,
            case_id="case-1",
            task_id=AITaskId.COMMAND,
            investigation_id="inv-1",
            web_session_id="web-1",
            operator_id=None, # type: ignore
            operator_session_id="sess-1",
            payload=CommandRequestPayload(command="echo hi", execution_id="exec-1")
        )
        g8e_context = build_g8e_http_context()
        with pytest.raises(ValidationError, match="operator_id and operator_session_id are required"):
            await execution_service.dispatch_command(msg, g8e_context)

    @pytest.mark.asyncio
    async def test_dispatch_pubsub_not_ready(self, execution_service, mock_pubsub):
        mock_pubsub.is_ready = False
        msg = G8eMessage(
            id="exec-1",
            source_component=ComponentName.G8EE,
            event_type=EventType.OPERATOR_COMMAND_REQUESTED,
            case_id="case-1",
            task_id=AITaskId.COMMAND,
            investigation_id="inv-1",
            web_session_id="web-1",
            operator_id="op-1",
            operator_session_id="sess-1",
            payload=CommandRequestPayload(command="echo hi", execution_id="exec-1")
        )
        g8e_context = build_g8e_http_context()
        res, env = await execution_service.dispatch_command(msg, g8e_context)
        assert res.status == ExecutionStatus.FAILED
        assert res.error_type == CommandErrorType.PUBSUB_SUBSCRIPTION_NOT_READY

    @pytest.mark.asyncio
    async def test_dispatch_no_subscribers(self, execution_service, mock_pubsub):
        mock_pubsub.publish_command.return_value = 0
        mock_pubsub.register_future.return_value = asyncio.Future()
        msg = G8eMessage(
            id="exec-1",
            source_component=ComponentName.G8EE,
            event_type=EventType.OPERATOR_COMMAND_REQUESTED,
            case_id="case-1",
            task_id=AITaskId.COMMAND,
            investigation_id="inv-1",
            web_session_id="web-1",
            operator_id="op-1",
            operator_session_id="sess-1",
            payload=CommandRequestPayload(command="echo hi", execution_id="exec-1")
        )
        g8e_context = build_g8e_http_context()
        res, env = await execution_service.dispatch_command(msg, g8e_context)
        assert res.status == ExecutionStatus.FAILED
        assert res.error_type == CommandErrorType.NO_OPERATORS_AVAILABLE
        mock_pubsub.release_future.assert_called_once_with("exec-1")

    @pytest.mark.asyncio
    async def test_dispatch_payload_type_mismatch(self, execution_service, mock_pubsub):
        future = asyncio.Future()
        mock_pubsub.register_future.return_value = future
        from app.models.pubsub_messages import ExecutionStatusPayload
        envelope = G8eoResultEnvelope(
            operator_id="op-1",
            operator_session_id="sess-1",
            event_type=EventType.OPERATOR_COMMAND_COMPLETED,
            payload=ExecutionStatusPayload(
                execution_id="exec-1",
                status=ExecutionStatus.EXECUTING
            )
        )
        future.set_result(envelope)

        msg = G8eMessage(
            id="exec-1",
            source_component=ComponentName.G8EE,
            event_type=EventType.OPERATOR_COMMAND_REQUESTED,
            case_id="case-1",
            task_id=AITaskId.COMMAND,
            investigation_id="inv-1",
            web_session_id="web-1",
            operator_id="op-1",
            operator_session_id="sess-1",
            payload=CommandRequestPayload(command="echo hi", execution_id="exec-1")
        )
        g8e_context = build_g8e_http_context()
        res, env = await execution_service.dispatch_command(msg, g8e_context)
        assert res.status == ExecutionStatus.COMPLETED
        assert res.output == ""
        mock_pubsub.release_future.assert_called_once_with("exec-1")

class TestOperatorExecutionServiceCancel:
    @pytest.mark.asyncio
    async def test_cancel_command_success(self, execution_service, mock_pubsub):
        g8e_context = build_g8e_http_context()
        g8e_context.case_id = "case-1"
        g8e_context.investigation_id = "inv-1"
        g8e_context.web_session_id = "web-1"
        res = await execution_service.cancel_command("exec-1", "op-1", "sess-1", g8e_context)
        assert res.status == ExecutionStatus.CANCELLED
        mock_pubsub.publish_command.assert_called_once()

    @pytest.mark.asyncio
    async def test_cancel_command_failure(self, execution_service, mock_pubsub):
        mock_pubsub.publish_command.side_effect = Exception("publish failed")
        g8e_context = build_g8e_http_context()
        g8e_context.case_id = "case-1"
        g8e_context.investigation_id = "inv-1"
        g8e_context.web_session_id = "web-1"
        res = await execution_service.cancel_command("exec-1", "op-1", "sess-1", g8e_context)
        assert res.status == ExecutionStatus.FAILED
        assert "Command cancellation failed" in res.error

class TestOperatorExecutionServiceDirectCommand:
    @pytest.mark.asyncio
    async def test_send_command_no_bound_operators(self, execution_service):
        g8e_context = build_g8e_http_context()
        g8e_context.bound_operators = []
        with pytest.raises(ValidationError, match="No bound operators"):
            await execution_service.send_command_to_operator(MagicMock(), g8e_context)

    @pytest.mark.asyncio
    async def test_send_command_operator_not_bound(self, execution_service):
        g8e_context = build_g8e_http_context()
        g8e_context.bound_operators = [MagicMock(operator_session_id=None)]
        with pytest.raises(ValidationError, match="Operator not bound"):
            await execution_service.send_command_to_operator(MagicMock(), g8e_context)

    @pytest.mark.asyncio
    async def test_send_command_no_subscribers(self, execution_service, mock_pubsub):
        mock_pubsub.publish_command.return_value = 0
        mock_pubsub.register_future.return_value = asyncio.Future()
        g8e_context = build_g8e_http_context()
        g8e_context.case_id = "case-1"
        g8e_context.investigation_id = "inv-1"
        g8e_context.web_session_id = "web-1"
        bound_op = MagicMock(operator_id="op-1", operator_session_id="sess-1")
        g8e_context.bound_operators = [bound_op]

        from app.models.internal_api import DirectCommandRequest
        payload = DirectCommandRequest(execution_id="exec-1", command="echo hi", hostname="host-1")
        res = await execution_service.send_command_to_operator(payload, g8e_context)

        assert res.status == ExecutionStatus.FAILED
        assert res.error == "No Operator listening"
        mock_pubsub.release_future.assert_called_once_with("exec-1")

    @pytest.mark.asyncio
    async def test_wait_and_broadcast_payload_mismatch(self, execution_service, mock_event_service):
        future = asyncio.Future()
        from app.models.pubsub_messages import ExecutionStatusPayload
        envelope = G8eoResultEnvelope(
            operator_id="op-1",
            operator_session_id="sess-1",
            event_type=EventType.OPERATOR_COMMAND_COMPLETED,
            payload=ExecutionStatusPayload(
                execution_id="exec-1",
                status=ExecutionStatus.EXECUTING
            )
        )
        future.set_result(envelope)

        g8e_context = build_g8e_http_context()
        await execution_service._wait_and_broadcast_direct_command_result(
            "exec-1", "echo hi", future, g8e_context, "op-1", "sess-1"
        )
        mock_event_service.publish_command_event.assert_not_called()

    @pytest.mark.asyncio
    async def test_wait_and_broadcast_timeout(self, execution_service, mock_event_service):
        future = asyncio.Future()
        g8e_context = build_g8e_http_context()

        # Mock wait_for to raise TimeoutError
        from unittest.mock import patch
        with patch("asyncio.wait_for", side_effect=TimeoutError()):
            await execution_service._wait_and_broadcast_direct_command_result(
                "exec-1", "echo hi", future, g8e_context, "op-1", "sess-1"
            )

        execution_service.pubsub_service.release_future.assert_called_once_with("exec-1")

    @pytest.mark.asyncio
    async def test_wait_and_broadcast_generic_exception(self, execution_service, mock_pubsub):
        future = asyncio.Future()
        future.set_exception(Exception("unexpected"))
        g8e_context = build_g8e_http_context()

        # Should not raise
        await execution_service._wait_and_broadcast_direct_command_result(
            "exec-1", "echo hi", future, g8e_context, "op-1", "sess-1"
        )
        # Finally block should release future
        execution_service.pubsub_service.release_future.assert_called_once_with("exec-1")
