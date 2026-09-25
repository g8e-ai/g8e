from __future__ import annotations

# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Regression tests for Gateway-owned operator dispatch correlation."""

import base64
from unittest.mock import AsyncMock, MagicMock

import pytest

from app.constants.generated_status import EventType
from app.constants.generated_status import AITaskId
from app.constants import ExecutionStatus, G8EE_COMPONENT
from app.models.command_request_payloads import CommandRequestPayload
from app.models.pubsub_messages import G8eMessage
from app.services.operator.execution_service import OperatorExecutionService
from g8e.operator.v1 import operator_pb2
from tests.fakes.factories import build_g8e_http_context

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]


def _build_execution_service(mock_gateway_client: MagicMock) -> OperatorExecutionService:
    return OperatorExecutionService(
        pubsub_service=MagicMock(),
        approval_service=MagicMock(),
        event_service=MagicMock(),
        settings=MagicMock(),
        ai_response_analyzer=MagicMock(),
        operator_data_service=MagicMock(),
        investigation_service=MagicMock(),
        gateway_operator_client=mock_gateway_client,
    )


def _build_command_message(
    exec_id: str, *, op_id: str = "op-1", sess_id: str = "sess-1"
) -> G8eMessage:
    return G8eMessage(
        id=exec_id,
        source_component=G8EE_COMPONENT,
        event_type=EventType.OPERATOR_COMMAND_REQUESTED,
        case_id="case-1",
        task_id=AITaskId.COMMAND,
        investigation_id="inv-1",
        web_session_id="web-1",
        operator_id=op_id,
        operator_session_id=sess_id,
        payload=CommandRequestPayload(command="echo hi", execution_id=exec_id),
    )


class TestGatewayDispatchCorrelation:
    async def test_uses_payload_execution_id_not_http_request_id(self):
        mock_gateway = MagicMock()
        per_message_exec_id = "per-msg-exec-id"
        command_result = operator_pb2.CommandResult(
            execution_id=per_message_exec_id,
            status=operator_pb2.ExecutionStatus.EXECUTION_STATUS_COMPLETED,
            stdout="hi",
            return_code=0,
        )
        mock_gateway.dispatch = AsyncMock(
            return_value={
                "success": True,
                "transaction_id": "tx-1",
                "event_type": EventType.OPERATOR_COMMAND_COMPLETED,
                "result_payload": base64.b64encode(command_result.SerializeToString()).decode(
                    "ascii"
                ),
            }
        )
        svc = _build_execution_service(mock_gateway)
        g8e_context = build_g8e_http_context()
        assert g8e_context.execution_id != per_message_exec_id

        internal_result, envelope = await svc.dispatch_command(
            _build_command_message(per_message_exec_id),
            g8e_context,
            timeout_seconds=5,
        )

        assert internal_result.status == ExecutionStatus.COMPLETED
        assert internal_result.execution_id == per_message_exec_id
        assert envelope is not None
        assert envelope.payload.execution_id == per_message_exec_id

    async def test_times_out_when_gateway_dispatch_hangs(self):
        async def slow_dispatch(**kwargs):
            import asyncio

            await asyncio.sleep(1)
            return {"success": True}

        mock_gateway = MagicMock()
        mock_gateway.dispatch = slow_dispatch
        svc = _build_execution_service(mock_gateway)

        internal_result, envelope = await svc.dispatch_command(
            _build_command_message("lonely-exec-id"),
            build_g8e_http_context(),
            timeout_seconds=0.1,
        )

        assert internal_result.status == ExecutionStatus.TIMEOUT
        assert internal_result.execution_id == "lonely-exec-id"
        assert envelope is None
