# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Regression tests for BLOCKER 12: operator service G8eMessage dispatchers
must propagate user_id and cli_session_id from g8e_context so that
pubsub_client.CommandIntent carries a non-empty requestor_user_id and the
gateway's verifyReceiptIdentity check passes.

Each test constructs a g8e_context with user_id and cli_session_id set,
invokes the service method, and asserts the G8eMessage passed to either
execution_service.execute or pubsub_service.publish_command carries both
identity fields.
"""

import asyncio

import pytest
from unittest.mock import AsyncMock, MagicMock

from app.constants import FileOperation, G8EE_COMPONENT
from app.constants.generated_status import AITaskId, EventType
from app.models.command_request_payloads import (
    FileEditRequestPayload,
    FsGrepRequestPayload,
    FsListRequestPayload,
    FsReadRequestPayload,
)
from app.models.http_context import G8eHttpContext, RequestContext
from app.models.investigations import EnrichedInvestigationContext
from app.models.internal_api import DirectCommandRequest
from app.models.operators import OperatorDocument
from app.models.pubsub_messages import FileEditResultPayload
from app.models.tool_results import CommandInternalResult
from app.constants.config import ExecutionStatus
from tests.fakes.builder import build_command_service


def _identity_context() -> G8eHttpContext:
    """Build a g8e_context with user_id and cli_session_id set."""
    return G8eHttpContext(
        case_id="case-123",
        investigation_id="inv-123",
        web_session_id="web-123",
        user_id="user-123",
        cli_session_id="cli-456",
        source_component=G8EE_COMPONENT,
    )


def _mock_operator() -> MagicMock:
    op = MagicMock(spec=OperatorDocument)
    op.id = "op-123"
    op.operator_session_id = "sess-123"
    return op


def _mock_envelope() -> MagicMock:
    envelope = MagicMock()
    envelope.payload = FileEditResultPayload(
        execution_id="exec-123",
        operation="read",
        file_path="/etc/test",
        status=ExecutionStatus.COMPLETED,
        content="test content",
    )
    return envelope


def _stub_execute_for_identity(service) -> None:
    """Wire execution_service.execute to a no-op AsyncMock returning a completed result."""
    internal_result = CommandInternalResult(status=ExecutionStatus.COMPLETED, output="")
    service.execution_service.execute = AsyncMock(
        return_value=(internal_result, _mock_envelope())
    )
    service.execution_service.event_service.publish_command_event = AsyncMock()


class TestFileServiceIdentityPropagation:
    """OperatorFileService must propagate user_id and cli_session_id to G8eMessage."""

    @pytest.mark.asyncio
    async def test_execute_file_edit_propagates_user_id_and_cli_session_id(self):
        command_service = build_command_service()
        file_service = command_service._file_service

        _stub_execute_for_identity(file_service)
        mock_op = _mock_operator()
        file_service.execution_service.resolve_operators = MagicMock(return_value=[mock_op])
        file_service.event_service.publish_command_event = AsyncMock()

        args = FileEditRequestPayload(
            file_path="/etc/test",
            operation=FileOperation.READ,
            justification="Reading test file",
            execution_id="exec-123",
            target_operators=["all"],
        )
        g8e_context = _identity_context()
        investigation = EnrichedInvestigationContext(
            id="inv-123",
            case_id="case-123",
            user_id="user-123",
            sentinel_mode=False,
            operator_documents=[mock_op],
        )

        await file_service.execute_file_edit(args, g8e_context, investigation)

        msg = file_service.execution_service.execute.call_args.kwargs["g8e_message"]
        assert msg.user_id == "user-123", "user_id must be propagated from g8e_context"
        assert msg.cli_session_id == "cli-456", "cli_session_id must be propagated from g8e_context"


class TestFilesystemServiceIdentityPropagation:
    """OperatorFilesystemService must propagate user_id and cli_session_id to G8eMessage."""

    @pytest.mark.asyncio
    async def test_execute_fs_list_propagates_user_id_and_cli_session_id(self):
        command_service = build_command_service()
        fs_service = command_service._filesystem_service

        _stub_execute_for_identity(fs_service)
        mock_op = _mock_operator()
        fs_service.execution_service.resolve_operators = MagicMock(return_value=[mock_op])

        args = FsListRequestPayload(
            path="/etc",
            execution_id="exec-123",
            target_operators=["all"],
        )
        g8e_context = _identity_context()
        investigation = EnrichedInvestigationContext(
            id="inv-123",
            case_id="case-123",
            user_id="user-123",
            sentinel_mode=False,
            operator_documents=[mock_op],
        )

        await fs_service.execute_fs_list(args, investigation, g8e_context)

        msg = fs_service.execution_service.execute.call_args.kwargs["g8e_message"]
        assert msg.user_id == "user-123"
        assert msg.cli_session_id == "cli-456"

    @pytest.mark.asyncio
    async def test_execute_fs_grep_propagates_user_id_and_cli_session_id(self):
        command_service = build_command_service()
        fs_service = command_service._filesystem_service

        _stub_execute_for_identity(fs_service)
        mock_op = _mock_operator()
        fs_service.execution_service.resolve_operators = MagicMock(return_value=[mock_op])

        args = FsGrepRequestPayload(
            path="/etc",
            pattern="test",
            execution_id="exec-123",
            target_operators=["all"],
        )
        g8e_context = _identity_context()
        investigation = EnrichedInvestigationContext(
            id="inv-123",
            case_id="case-123",
            user_id="user-123",
            sentinel_mode=False,
            operator_documents=[mock_op],
        )

        await fs_service.execute_fs_grep(args, investigation, g8e_context)

        msg = fs_service.execution_service.execute.call_args.kwargs["g8e_message"]
        assert msg.user_id == "user-123"
        assert msg.cli_session_id == "cli-456"

    @pytest.mark.asyncio
    async def test_execute_file_read_propagates_user_id_and_cli_session_id(self):
        command_service = build_command_service()
        fs_service = command_service._filesystem_service

        _stub_execute_for_identity(fs_service)
        mock_op = _mock_operator()
        fs_service.execution_service.resolve_operators = MagicMock(return_value=[mock_op])

        args = FsReadRequestPayload(
            path="/etc/test",
            execution_id="exec-123",
            target_operators=["all"],
        )
        g8e_context = _identity_context()
        investigation = EnrichedInvestigationContext(
            id="inv-123",
            case_id="case-123",
            user_id="user-123",
            sentinel_mode=False,
            operator_documents=[mock_op],
        )

        await fs_service.execute_file_read(args, investigation, g8e_context)

        msg = fs_service.execution_service.execute.call_args.kwargs["g8e_message"]
        assert msg.user_id == "user-123"
        assert msg.cli_session_id == "cli-456"


class TestExecutionServiceIdentityPropagation:
    """OperatorExecutionService must propagate user_id and cli_session_id to G8eMessage."""

    @pytest.mark.asyncio
    async def test_send_command_to_operator_propagates_user_id_and_cli_session_id(self):
        command_service = build_command_service()
        exec_service = command_service._execution_service

        # Stub pubsub so the background _wait_and_broadcast task completes fast.
        done_future: asyncio.Future = asyncio.get_event_loop().create_future()
        done_future.set_result(MagicMock())
        exec_service.pubsub_service.register_future = MagicMock(return_value=done_future)
        exec_service.pubsub_service.register_operator_session = AsyncMock()
        exec_service.pubsub_service.release_future = MagicMock()
        exec_service.pubsub_service.publish_command = AsyncMock(return_value=1)
        exec_service._event_service.publish_command_event = AsyncMock()

        mock_op = MagicMock()
        mock_op.operator_id = "op-123"
        mock_op.operator_session_id = "sess-123"

        g8e_context = _identity_context()
        g8e_context.bound_operators = [mock_op]

        command_payload = DirectCommandRequest(
            context=RequestContext(
                case_id="case-123",
                investigation_id="inv-123",
                web_session_id="web-123",
                user_id="user-123",
            ),
            command="ls -la",
            execution_id="exec-123",
        )

        await exec_service.send_command_to_operator(command_payload, g8e_context)

        msg = exec_service.pubsub_service.publish_command.call_args.kwargs["command_data"]
        assert msg.user_id == "user-123", "user_id must be propagated from g8e_context"
        assert msg.cli_session_id == "cli-456", "cli_session_id must be propagated from g8e_context"

    @pytest.mark.asyncio
    async def test_cancel_command_propagates_user_id_and_cli_session_id(self):
        command_service = build_command_service()
        exec_service = command_service._execution_service

        exec_service.pubsub_service.publish_command = AsyncMock(return_value=1)

        g8e_context = _identity_context()

        await exec_service.cancel_command(
            execution_id="exec-123",
            operator_id="op-123",
            operator_session_id="sess-123",
            g8e_context=g8e_context,
        )

        msg = exec_service.pubsub_service.publish_command.call_args.kwargs["command_data"]
        assert msg.user_id == "user-123", "user_id must be propagated from g8e_context"
        assert msg.cli_session_id == "cli-456", "cli_session_id must be propagated from g8e_context"


class TestLFAAServiceIdentityPropagation:
    """OperatorLFAAService must propagate user_id and cli_session_id to G8eMessage."""

    @pytest.mark.asyncio
    async def test_send_direct_exec_audit_event_propagates_user_id_and_cli_session_id(self):
        command_service = build_command_service()
        lfaa_service = command_service._lfaa_service

        lfaa_service.pubsub_service.publish_command = AsyncMock(return_value=1)

        g8e_context = _identity_context()
        g8e_context.bound_operators = [
            MagicMock(operator_id="op-123", operator_session_id="sess-123")
        ]

        result = await lfaa_service.send_direct_exec_audit_event(
            command="ls -la", execution_id="exec-999", g8e_context=g8e_context
        )

        assert result is True
        msg = lfaa_service.pubsub_service.publish_command.call_args[1]["command_data"]
        assert msg.user_id == "user-123", "user_id must be propagated from g8e_context"
        assert msg.cli_session_id == "cli-456", "cli_session_id must be propagated from g8e_context"
