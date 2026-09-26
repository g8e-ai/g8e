# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import pytest
from unittest.mock import AsyncMock, MagicMock

from app.services.operator.lfaa_service import OperatorLFAAService
from app.constants import G8EE_COMPONENT
from app.models.pubsub_messages import G8eMessage
from app.models.command_request_payloads import DirectCommandAuditRequestPayload
from app.models.http_context import G8eHttpContext
from app.constants.generated_status import EventType
from app.constants.generated_status import AITaskId


@pytest.fixture
def mock_gateway_client():
    client = MagicMock()
    client.ingest_audit_record = AsyncMock(return_value={"seq": 1, "hash": "abc"})
    return client


@pytest.fixture
def lfaa_service(mock_gateway_client):
    return OperatorLFAAService(gateway_operator_client=mock_gateway_client)


@pytest.fixture
def valid_g8e_message():
    return G8eMessage(
        id="test_id",
        source_component=G8EE_COMPONENT,
        event_type=EventType.OPERATOR_AUDIT_COMMAND_RECORDED,
        operator_id="op_1",
        operator_session_id="sess_1",
        case_id="case_123",
        task_id=AITaskId.DIRECT_COMMAND,
        investigation_id="inv_456",
        web_session_id="web_789",
        payload=DirectCommandAuditRequestPayload(
            command="ls", execution_id="exec_1", operator_session_id="sess_1"
        ),
    )


class TestOperatorLFAAService:
    @pytest.mark.asyncio
    async def test_send_audit_event_success(
        self, lfaa_service, mock_gateway_client, valid_g8e_message
    ):
        result = await lfaa_service.send_audit_event(valid_g8e_message)

        assert result is True
        mock_gateway_client.ingest_audit_record.assert_awaited_once()
        kwargs = mock_gateway_client.ingest_audit_record.await_args.kwargs
        assert kwargs["operator_id"] == "op_1"
        assert kwargs["operator_session_id"] == "sess_1"
        assert kwargs["event_type"] == EventType.OPERATOR_AUDIT_COMMAND_RECORD_REQUESTED
        assert kwargs["idempotency_key"] == "test_id"

    @pytest.mark.asyncio
    async def test_send_audit_event_ingest_failure(
        self, lfaa_service, mock_gateway_client, valid_g8e_message
    ):
        from app.errors import NetworkError

        mock_gateway_client.ingest_audit_record.side_effect = NetworkError("gateway down")

        result = await lfaa_service.send_audit_event(valid_g8e_message)

        assert result is False
        assert mock_gateway_client.ingest_audit_record.await_count == 3

    @pytest.mark.asyncio
    async def test_send_audit_event_missing_fields(self, lfaa_service, mock_gateway_client):
        msg_no_payload = G8eMessage(
            id="test_id",
            source_component=G8EE_COMPONENT,
            event_type=EventType.OPERATOR_AUDIT_COMMAND_RECORDED,
            operator_id="op_1",
            operator_session_id="sess_1",
            case_id="case_123",
            task_id=AITaskId.DIRECT_COMMAND,
            investigation_id="inv_456",
            web_session_id="web_789",
            payload=None,
        )
        assert await lfaa_service.send_audit_event(msg_no_payload) is False
        mock_gateway_client.ingest_audit_record.assert_not_called()

    @pytest.mark.asyncio
    async def test_send_direct_exec_audit_event_success(self, lfaa_service, mock_gateway_client):
        g8e_context = G8eHttpContext(
            case_id="case_123",
            investigation_id="inv_456",
            source_component=G8EE_COMPONENT,
            web_session_id="web_789",
            user_id="user_1",
        )
        g8e_context.bound_operators = [MagicMock(operator_id="op_1", operator_session_id="sess_1")]

        result = await lfaa_service.send_direct_exec_audit_event(
            command="ls -la", execution_id="exec_999", g8e_context=g8e_context
        )

        assert result is True
        kwargs = mock_gateway_client.ingest_audit_record.await_args.kwargs
        assert kwargs["operator_id"] == "op_1"
        assert kwargs["operator_session_id"] == "sess_1"
        assert kwargs["event_type"] == EventType.OPERATOR_AUDIT_DIRECT_COMMAND_RECORD_REQUESTED
        assert kwargs["idempotency_key"] == "audit_exec_999"

    @pytest.mark.asyncio
    async def test_send_direct_exec_audit_event_no_bound_operators(self, lfaa_service):
        g8e_context = G8eHttpContext(
            case_id="case_123",
            investigation_id="inv_456",
            source_component=G8EE_COMPONENT,
            web_session_id="web_789",
            user_id="user_1",
        )
        g8e_context.bound_operators = []

        result = await lfaa_service.send_direct_exec_audit_event("ls", "exec_1", g8e_context)
        assert result is False
