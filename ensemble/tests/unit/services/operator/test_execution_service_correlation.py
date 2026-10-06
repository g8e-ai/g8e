"""Regression tests for Gateway-owned operator dispatch correlation."""

from __future__ import annotations

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
from g8e.operator.v1 import operator_pb2

from app.constants import G8EE_COMPONENT, ExecutionStatus, FileOperation
from app.constants.generated_status import AITaskId, EventType
from app.models.command_request_payloads import CommandRequestPayload, FileEditRequestPayload
from app.models.pubsub_messages import FileEditResultPayload, G8eMessage
from app.services.operator.execution_service import OperatorExecutionService
from tests.fakes.factories import build_g8e_http_context

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]


def _build_execution_service(mock_gateway_client: MagicMock) -> OperatorExecutionService:
    return OperatorExecutionService(
        approval_service=MagicMock(),
        settings=MagicMock(),
        ai_response_analyzer=MagicMock(),
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
    @pytest.mark.parametrize("failed", [False, True])
    async def test_file_result_preserves_operator_status_and_error(self, failed):
        result = operator_pb2.FileEditResult(
            execution_id="read-1",
            operation="read",
            file_path="/tmp/status.txt",
            status=(
                operator_pb2.EXECUTION_STATUS_FAILED
                if failed
                else operator_pb2.EXECUTION_STATUS_COMPLETED
            ),
            error_message="path not found" if failed else "",
            content="ready" if not failed else "",
        )
        gateway = MagicMock()
        gateway.dispatch = AsyncMock(
            return_value={
                "success": True,
                "event_type": EventType.OPERATOR_FILE_EDIT_FAILED
                if failed
                else EventType.OPERATOR_FILE_EDIT_COMPLETED,
                "result_payload": base64.b64encode(result.SerializeToString()).decode("ascii"),
            }
        )
        message = _build_command_message("read-1")
        message.event_type = EventType.OPERATOR_FILE_EDIT_REQUESTED
        message.payload = FileEditRequestPayload(
            execution_id="read-1",
            operation=FileOperation.READ,
            file_path="/tmp/status.txt",
            justification="Read status",
            target_operators=["all"],
        )
        internal, envelope = await _build_execution_service(gateway).dispatch_command(
            message, build_g8e_http_context()
        )
        assert internal.status == (ExecutionStatus.FAILED if failed else ExecutionStatus.COMPLETED)
        assert internal.error == ("path not found" if failed else "")
        assert envelope is not None
        assert isinstance(envelope.payload, FileEditResultPayload)
        assert envelope.payload.content == (None if failed else "ready")

    async def test_missing_result_payload_is_not_success(self):
        gateway = MagicMock()
        gateway.dispatch = AsyncMock(return_value={"success": True, "transaction_id": "tx-1"})
        internal, envelope = await _build_execution_service(gateway).dispatch_command(
            _build_command_message("read-1"), build_g8e_http_context()
        )
        assert internal.status == ExecutionStatus.FAILED
        assert internal.error is not None
        assert "no operator result payload" in internal.error
        assert envelope is None

    @pytest.mark.parametrize(
        ("result_type", "event_type"),
        [
            (operator_pb2.FsListResult, EventType.OPERATOR_FILESYSTEM_LIST_FAILED),
            (operator_pb2.FsReadResult, EventType.OPERATOR_FILESYSTEM_READ_FAILED),
            (operator_pb2.FsGrepResult, EventType.OPERATOR_FILESYSTEM_GREP_FAILED),
        ],
    )
    async def test_filesystem_failure_preserves_operator_error(self, result_type, event_type):
        result = result_type(
            execution_id="fs-1",
            status=operator_pb2.EXECUTION_STATUS_FAILED,
            error_message="path not found",
        )
        gateway = MagicMock()
        gateway.dispatch = AsyncMock(
            return_value={
                "success": True,
                "event_type": event_type,
                "result_payload": base64.b64encode(result.SerializeToString()).decode("ascii"),
            }
        )
        internal, envelope = await _build_execution_service(gateway).dispatch_command(
            _build_command_message("fs-1"), build_g8e_http_context()
        )
        assert internal.status == ExecutionStatus.FAILED
        assert internal.error == "path not found"
        assert envelope is not None

    async def test_shell_failure_preserves_canonical_error(self):
        result = operator_pb2.CommandResult(
            execution_id="shell-1",
            status=operator_pb2.EXECUTION_STATUS_FAILED,
            error="execution denied",
        )
        gateway = MagicMock()
        gateway.dispatch = AsyncMock(
            return_value={
                "success": True,
                "event_type": EventType.OPERATOR_COMMAND_FAILED,
                "result_payload": base64.b64encode(result.SerializeToString()).decode("ascii"),
            }
        )
        internal, envelope = await _build_execution_service(gateway).dispatch_command(
            _build_command_message("shell-1"), build_g8e_http_context()
        )
        assert internal.status == ExecutionStatus.FAILED
        assert internal.error == "execution denied"
        assert envelope is not None

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
            await asyncio.sleep(2)
            return {"success": True}

        mock_gateway = MagicMock()
        mock_gateway.dispatch = slow_dispatch
        svc = _build_execution_service(mock_gateway)

        internal_result, envelope = await svc.dispatch_command(
            _build_command_message("lonely-exec-id"),
            build_g8e_http_context(),
            timeout_seconds=1,
        )

        assert internal_result.status == ExecutionStatus.TIMEOUT
        assert internal_result.execution_id == "lonely-exec-id"
        assert envelope is None
