# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import asyncio
import base64
from unittest.mock import AsyncMock, MagicMock

import pytest

from app.constants.generated_status import EventType
from app.constants import ExecutionStatus
from app.models.http_context import RequestContext
from app.models.internal_api import DirectCommandRequest
from app.models.operators import DirectCommandResult
from app.services.operator.execution_service import OperatorExecutionService
from g8e.operator.v1 import operator_pb2
from tests.fakes.factories import (
    build_bound_operator,
    build_g8e_http_context,
)

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]


def _build_execution_service(
    dispatch_result: dict,
) -> tuple[OperatorExecutionService, MagicMock]:
    gateway_client = MagicMock()
    gateway_client.dispatch = AsyncMock(return_value=dispatch_result)

    svc = OperatorExecutionService.__new__(OperatorExecutionService)
    svc._approval_service = None
    svc._settings = None
    svc._ai_response_analyzer = None
    svc._investigation_service = None
    svc._gateway_operator_client = gateway_client
    svc._background_tasks = set()

    return svc, gateway_client


class TestDirectCommandDispatch:
    async def test_send_command_to_operator_dispatches_in_background(self):
        """Direct commands dispatch through Gateway."""
        command_result = operator_pb2.CommandResult(
            execution_id="direct-exec-1",
            status=operator_pb2.ExecutionStatus.EXECUTION_STATUS_COMPLETED,
            stdout="file1\nfile2",
            return_code=0,
            execution_time_seconds=1.5,
        )
        svc, gateway_client = _build_execution_service(
            {
                "success": True,
                "transaction_id": "tx-1",
                "event_type": EventType.OPERATOR_COMMAND_COMPLETED,
                "result_payload": base64.b64encode(command_result.SerializeToString()).decode(
                    "ascii"
                ),
            }
        )

        exec_id = "direct-exec-1"
        command = "ls -la"
        bound_op = build_bound_operator(operator_id="op-1", operator_session_id="sess-1")
        g8e_context = build_g8e_http_context()
        g8e_context.bound_operators = [bound_op]

        request = DirectCommandRequest(
            command=command,
            execution_id=exec_id,
            hostname="test-host",
            context=RequestContext(
                case_id="case-1", investigation_id="inv-1", source_component="g8ee"
            ),
        )

        result = await svc.send_command_to_operator(request, g8e_context)

        assert isinstance(result, DirectCommandResult)
        assert result.status == ExecutionStatus.EXECUTING
        assert result.execution_id == exec_id

        for _ in range(5):
            await asyncio.sleep(0)

        gateway_client.dispatch.assert_called_once()
        dispatch_kwargs = gateway_client.dispatch.call_args.kwargs
        assert dispatch_kwargs["event_type"] == EventType.OPERATOR_COMMAND_REQUESTED
        assert dispatch_kwargs["operator_session_id"] == "sess-1"
        assert dispatch_kwargs["context"] == g8e_context

    async def test_send_command_to_operator_handles_gateway_failure(self):
        """Gateway dispatch failures do not crash the background task."""
        svc, gateway_client = _build_execution_service(
            {"success": False, "error": "No operator available"}
        )

        exec_id = "direct-exec-fail"
        command = "invalid-cmd"
        bound_op = build_bound_operator(operator_id="op-1", operator_session_id="sess-1")
        g8e_context = build_g8e_http_context()
        g8e_context.bound_operators = [bound_op]

        request = DirectCommandRequest(
            command=command,
            execution_id=exec_id,
            context=RequestContext(
                case_id="case-1", investigation_id="inv-1", source_component="g8ee"
            ),
        )

        result = await svc.send_command_to_operator(request, g8e_context)

        assert isinstance(result, DirectCommandResult)
        assert result.status == ExecutionStatus.EXECUTING

        for _ in range(5):
            await asyncio.sleep(0)

        gateway_client.dispatch.assert_called_once()
