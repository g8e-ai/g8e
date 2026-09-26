# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import base64
from unittest.mock import MagicMock, AsyncMock

import pytest

from app.constants.generated_status import EventType
from app.constants.generated_status import AITaskId, CommandErrorType
from app.constants import ExecutionStatus, G8EE_COMPONENT
from app.errors import BusinessLogicError, NetworkError, ValidationError
from app.models.http_context import RequestContext
from app.models.command_request_payloads import CommandRequestPayload
from app.models.operators import OperatorDocument, HeartbeatSnapshot, HeartbeatSystemIdentity
from app.models.pubsub_messages import G8eMessage
from app.services.operator.execution_service import OperatorExecutionService
from g8e.operator.v1 import operator_pb2
from tests.fakes.factories import build_g8e_http_context

pytestmark = [pytest.mark.unit]


@pytest.fixture
def mock_approval():
    return MagicMock()


@pytest.fixture
def mock_settings():
    return MagicMock()


@pytest.fixture
def mock_ai_analyzer():
    return MagicMock()


@pytest.fixture
def mock_investigation():
    return MagicMock()


@pytest.fixture
def mock_gateway_client():
    mock = MagicMock()
    mock.dispatch = AsyncMock()
    return mock


@pytest.fixture
def execution_service(
    mock_approval,
    mock_settings,
    mock_ai_analyzer,
    mock_investigation,
    mock_gateway_client,
):
    return OperatorExecutionService(
        approval_service=mock_approval,
        settings=mock_settings,
        ai_response_analyzer=mock_ai_analyzer,
        investigation_service=mock_investigation,
        gateway_operator_client=mock_gateway_client,
    )


class TestOperatorExecutionServiceProperties:
    def test_properties(
        self,
        execution_service,
        mock_approval,
        mock_ai_analyzer,
        mock_investigation,
    ):
        assert execution_service.approval_service == mock_approval
        assert execution_service.ai_response_analyzer == mock_ai_analyzer
        assert execution_service.investigation_service == mock_investigation


class TestOperatorExecutionServiceFailCommand:
    @pytest.mark.asyncio
    async def test_fail_command_returns_failure_result(self, execution_service):
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
            feedback_reason="feedback",
        )

        assert result.success is False
        assert result.error == "some error"
        assert result.error_type == CommandErrorType.EXECUTION_FAILED
        assert result.execution_id == "exec-1"
        assert result.rule == "rule-1"
        assert result.denial_reason == "denied"
        assert result.feedback_reason == "feedback"


class TestOperatorExecutionServiceResolveOperators:
    def test_resolve_operators_empty_documents(self, execution_service):
        with pytest.raises(BusinessLogicError, match="No operators bound"):
            execution_service.resolve_operators([], ["op-1"])

    def test_resolve_operators_empty_targets(self, execution_service):
        docs = [OperatorDocument(id="op-1", operator_type="remote", user_id="user-1")]
        with pytest.raises(ValidationError, match="target_operators list is empty"):
            execution_service.resolve_operators(docs, [])

    def test_resolve_operators_single_doc(self, execution_service):
        docs = [OperatorDocument(id="op-1", operator_type="remote", user_id="user-1")]
        resolved = execution_service.resolve_operators(docs, ["something"])
        assert resolved == docs

    def test_resolve_operators_fleet_sentinels(self, execution_service):
        docs = [
            OperatorDocument(id="op-1", operator_type="remote", user_id="user-1"),
            OperatorDocument(id="op-2", operator_type="remote", user_id="user-1"),
        ]
        for sentinel in ["all", "*", "fleet", "every", "everyone"]:
            resolved = execution_service.resolve_operators(docs, [sentinel])
            assert resolved == docs

    def test_resolve_operators_by_id(self, execution_service):
        docs = [
            OperatorDocument(id="op-1", operator_type="remote", user_id="user-1"),
            OperatorDocument(id="op-2", operator_type="remote", user_id="user-1"),
        ]
        resolved = execution_service.resolve_operators(docs, ["op-2"])
        assert len(resolved) == 1
        assert resolved[0].id == "op-2"

    def test_resolve_operators_by_hostname(self, execution_service):
        docs = [
            OperatorDocument(
                id="op-1", current_hostname="host-1", operator_type="remote", user_id="user-1"
            ),
            OperatorDocument(
                id="op-2",
                operator_type="remote",
                user_id="user-1",
                latest_heartbeat_snapshot=HeartbeatSnapshot(
                    system_identity=HeartbeatSystemIdentity(hostname="host-2")
                ),
            ),
        ]
        resolved = execution_service.resolve_operators(docs, ["host-2"])
        assert len(resolved) == 1
        assert resolved[0].id == "op-2"

    def test_resolve_operators_by_index(self, execution_service):
        docs = [
            OperatorDocument(id="op-1", operator_type="remote", user_id="user-1"),
            OperatorDocument(id="op-2", operator_type="remote", user_id="user-1"),
        ]
        resolved = execution_service.resolve_operators(docs, ["1"])
        assert len(resolved) == 1
        assert resolved[0].id == "op-2"

    def test_resolve_operators_not_found(self, execution_service):
        docs = [
            OperatorDocument(id="op-1", operator_type="remote", user_id="user-1"),
            OperatorDocument(id="op-2", operator_type="remote", user_id="user-1"),
        ]
        with pytest.raises(ValidationError, match="Could not resolve any operators"):
            execution_service.resolve_operators(docs, ["non-existent"])


class TestOperatorExecutionServiceTargetSystems:
    def test_build_target_systems_list(self, execution_service):
        docs = [
            OperatorDocument(
                id="op-1", current_hostname="host-1", operator_type="remote", user_id="user-1"
            ),
            OperatorDocument(id="op-2", operator_type="remote", user_id="user-1"),
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
            source_component=G8EE_COMPONENT,
            event_type=EventType.OPERATOR_COMMAND_REQUESTED,
            case_id="case-1",
            task_id=AITaskId.COMMAND,
            investigation_id="inv-1",
            web_session_id="web-1",
            operator_id="op-1",
            operator_session_id="sess-1",
            payload=CommandRequestPayload(command="echo hi", execution_id="exec-1"),
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
            source_component=G8EE_COMPONENT,
            event_type=EventType.OPERATOR_COMMAND_REQUESTED,
            case_id="case-1",
            task_id=AITaskId.COMMAND,
            investigation_id="inv-1",
            web_session_id="web-1",
            operator_id="op-1",
            operator_session_id="sess-1",
            payload=None,
        )
        g8e_context = build_g8e_http_context()
        with pytest.raises(ValidationError, match="g8e_message.payload is required"):
            await execution_service.dispatch_command(msg, g8e_context)

    @pytest.mark.asyncio
    async def test_dispatch_missing_operator_info(self, execution_service):
        msg = G8eMessage(
            id="exec-1",
            source_component=G8EE_COMPONENT,
            event_type=EventType.OPERATOR_COMMAND_REQUESTED,
            case_id="case-1",
            task_id=AITaskId.COMMAND,
            investigation_id="inv-1",
            web_session_id="web-1",
            operator_id=None,  # type: ignore
            operator_session_id="sess-1",
            payload=CommandRequestPayload(command="echo hi", execution_id="exec-1"),
        )
        g8e_context = build_g8e_http_context()
        with pytest.raises(
            ValidationError, match="operator_id and operator_session_id are required"
        ):
            await execution_service.dispatch_command(msg, g8e_context)

    @pytest.mark.asyncio
    async def test_dispatch_gateway_client_not_configured(self):
        svc = OperatorExecutionService(
            approval_service=MagicMock(),
            settings=MagicMock(),
            ai_response_analyzer=MagicMock(),
            investigation_service=MagicMock(),
            gateway_operator_client=None,
        )
        msg = G8eMessage(
            id="exec-1",
            source_component=G8EE_COMPONENT,
            event_type=EventType.OPERATOR_COMMAND_REQUESTED,
            case_id="case-1",
            task_id=AITaskId.COMMAND,
            investigation_id="inv-1",
            web_session_id="web-1",
            operator_id="op-1",
            operator_session_id="sess-1",
            payload=CommandRequestPayload(command="echo hi", execution_id="exec-1"),
        )
        g8e_context = build_g8e_http_context()
        res, _env = await svc.dispatch_command(msg, g8e_context)
        assert res.status == ExecutionStatus.FAILED
        assert res.error_type == CommandErrorType.PUBSUB_SUBSCRIPTION_NOT_READY

    @pytest.mark.asyncio
    async def test_dispatch_gateway_failure(self, execution_service, mock_gateway_client):
        mock_gateway_client.dispatch = AsyncMock(side_effect=NetworkError("denied", component="g8ee"))
        msg = G8eMessage(
            id="exec-1",
            source_component=G8EE_COMPONENT,
            event_type=EventType.OPERATOR_COMMAND_REQUESTED,
            case_id="case-1",
            task_id=AITaskId.COMMAND,
            investigation_id="inv-1",
            web_session_id="web-1",
            operator_id="op-1",
            operator_session_id="sess-1",
            payload=CommandRequestPayload(command="echo hi", execution_id="exec-1"),
        )
        g8e_context = build_g8e_http_context()
        res, _env = await execution_service.dispatch_command(msg, g8e_context)
        assert res.status == ExecutionStatus.FAILED
        assert res.error_type == CommandErrorType.NO_OPERATORS_AVAILABLE

    @pytest.mark.asyncio
    async def test_dispatch_gateway_success(self, execution_service, mock_gateway_client):
        command_result = operator_pb2.CommandResult(
            execution_id="exec-1",
            status=operator_pb2.ExecutionStatus.EXECUTION_STATUS_COMPLETED,
            stdout="hi",
            return_code=0,
        )
        mock_gateway_client.dispatch = AsyncMock(
            return_value={
                "success": True,
                "transaction_id": "tx-1",
                "event_type": EventType.OPERATOR_COMMAND_COMPLETED,
                "result_payload": base64.b64encode(command_result.SerializeToString()).decode("ascii"),
            }
        )
        msg = G8eMessage(
            id="exec-1",
            source_component=G8EE_COMPONENT,
            event_type=EventType.OPERATOR_COMMAND_REQUESTED,
            case_id="case-1",
            task_id=AITaskId.COMMAND,
            investigation_id="inv-1",
            web_session_id="web-1",
            operator_id="op-1",
            operator_session_id="sess-1",
            payload=CommandRequestPayload(command="echo hi", execution_id="exec-1"),
        )
        g8e_context = build_g8e_http_context()
        res, envelope = await execution_service.dispatch_command(msg, g8e_context)
        assert res.status == ExecutionStatus.COMPLETED
        assert res.output == "hi"
        assert envelope is not None
        assert envelope.payload.execution_id == "exec-1"


class TestOperatorExecutionServiceCancel:
    @pytest.mark.asyncio
    async def test_cancel_command_success(self, execution_service, mock_gateway_client):
        mock_gateway_client.dispatch = AsyncMock(return_value={"success": True})
        g8e_context = build_g8e_http_context()
        g8e_context.case_id = "case-1"
        g8e_context.investigation_id = "inv-1"
        g8e_context.web_session_id = "web-1"
        res = await execution_service.cancel_command("exec-1", "op-1", "sess-1", g8e_context)
        assert res.status == ExecutionStatus.CANCELLED
        mock_gateway_client.dispatch.assert_called_once()
        dispatch_kwargs = mock_gateway_client.dispatch.call_args.kwargs
        assert dispatch_kwargs["event_type"] == EventType.OPERATOR_COMMAND_CANCEL_REQUESTED
        assert dispatch_kwargs["operator_session_id"] == "sess-1"

    @pytest.mark.asyncio
    async def test_cancel_command_failure(self, execution_service, mock_gateway_client):
        mock_gateway_client.dispatch = AsyncMock(side_effect=NetworkError("dispatch failed"))
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
    async def test_send_command_gateway_not_configured(self, execution_service):
        execution_service._gateway_operator_client = None
        g8e_context = build_g8e_http_context()
        bound_op = MagicMock(operator_id="op-1", operator_session_id="sess-1")
        g8e_context.bound_operators = [bound_op]

        from app.models.internal_api import DirectCommandRequest

        payload = DirectCommandRequest(
            execution_id="exec-1",
            command="echo hi",
            hostname="host-1",
            context=RequestContext(
                case_id="case-1", investigation_id="inv-1", source_component="g8ee"
            ),
        )
        res = await execution_service.send_command_to_operator(payload, g8e_context)

        assert res.status == ExecutionStatus.FAILED
        assert res.error == "Gateway operator client is not configured"
