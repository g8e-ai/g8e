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

"""Tests for PubSubManagerMixin."""

import json
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from app.constants import EventType, ExecutionStatus, PubSubChannel
from app.services.operator.command_service import OperatorCommandService
from app.services.operator.heartbeat_service import OperatorHeartbeatService
from tests.fakes.builder import build_command_service

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]


def _make_pubsub_client():
    client = MagicMock()
    client.ensure_connected = AsyncMock()
    client.on_channel_message = MagicMock()
    client.off_channel_message = MagicMock()
    client.subscribe = AsyncMock()
    client.unsubscribe = AsyncMock()
    return client


def _make_service() -> OperatorCommandService:
    svc = build_command_service()
    pubsub = _make_pubsub_client()
    svc.set_pubsub_client(pubsub)
    svc._pubsub_service.subscribe_results(AsyncMock())
    return svc


@pytest.fixture
def command_service() -> OperatorCommandService:
    return _make_service()


def _make_mock_hb_pubsub_client() -> MagicMock:
    client = MagicMock()
    client.ensure_connected = AsyncMock()
    client.subscribe = AsyncMock()
    client.unsubscribe = AsyncMock()
    client.on_channel_message = MagicMock()
    client.off_channel_message = MagicMock()
    client.on_disconnect = MagicMock()
    client.off_disconnect = MagicMock()
    return client


@pytest.fixture
def heartbeat_service() -> OperatorHeartbeatService:
    svc = OperatorHeartbeatService(
        operator_data_service=MagicMock(),
        event_service=MagicMock(),
    )
    svc.set_pubsub_client(_make_mock_hb_pubsub_client())
    return svc


class TestStartPubSubListeners:
    """Test pub/sub client startup."""

    async def test_sets_pubsub_ready_flag(self, command_service):
        """Test sets _pubsub_ready to True."""
        await command_service.start_pubsub_listeners()
        assert command_service._pubsub_service._pubsub_ready is True

    async def test_no_handlers_registered_at_startup(self, command_service, heartbeat_service):
        """Test no channel handlers are registered at startup — only at register_operator_session."""
        await command_service.start_pubsub_listeners()
        command_service._pubsub_service.pubsub_client.on_channel_message.assert_not_called()
        heartbeat_service._pubsub_client.on_channel_message.assert_not_called()

    async def test_idempotent_when_called_twice(self, command_service):
        """Test second call is a no-op."""
        await command_service.start_pubsub_listeners()
        await command_service.start_pubsub_listeners()
        assert command_service._pubsub_service.pubsub_client.ensure_connected.call_count == 1

    async def test_no_channel_subscriptions_at_startup(self, command_service, heartbeat_service):
        """Test no operator channels are subscribed at startup."""
        await command_service.start_pubsub_listeners()
        command_service._pubsub_service.pubsub_client.subscribe.assert_not_called()
        heartbeat_service._pubsub_client.subscribe.assert_not_called()


class TestStopPubSubListeners:
    """Test pub/sub shutdown."""

    async def test_sets_pubsub_ready_false(self, command_service):
        """Test sets _pubsub_ready to False on stop."""
        await command_service.start_pubsub_listeners()
        await command_service.stop_pubsub_listeners()
        assert command_service._pubsub_service._pubsub_ready is False

    async def test_unsubscribes_all_active_sessions(self, command_service):
        """Test unsubscribes every registered results channel session on stop."""
        await command_service.start_pubsub_listeners()
        await command_service._pubsub_service.register_operator_session("op-1", "sess-1")
        await command_service._pubsub_service.register_operator_session("op-2", "sess-2")

        await command_service.stop_pubsub_listeners()

        assert command_service._pubsub_service.pubsub_client.unsubscribe.call_count == 2
        assert len(command_service._pubsub_service._active_operator_sessions_set) == 0

    async def test_handles_no_active_sessions(self, command_service):
        """Test stop is safe when no sessions are registered."""
        command_service._pubsub_service._pubsub_ready = True
        await command_service.stop_pubsub_listeners()
        assert command_service._pubsub_service._pubsub_ready is False


class TestRegisterOperatorSession:
    """Test per-operator exact channel subscription."""

    async def test_subscribes_exact_channels(self, command_service, heartbeat_service):
        """Test subscribes to the exact results channel for the operator session."""
        await command_service._pubsub_service.register_operator_session("op-abc", "sess-xyz")

        command_service._pubsub_service.pubsub_client.subscribe.assert_called_once_with(
            PubSubChannel.results("op-abc", "sess-xyz")
        )
        heartbeat_service._pubsub_client.subscribe.assert_not_called()

    async def test_registers_per_channel_handlers(self, command_service, heartbeat_service):
        """Test registers on_channel_message handler for the results channel only."""
        await command_service._pubsub_service.register_operator_session("op-abc", "sess-xyz")

        command_service._pubsub_service.pubsub_client.on_channel_message.assert_called_once_with(
            PubSubChannel.results("op-abc", "sess-xyz"), command_service._pubsub_service._dispatch_results_message
        )
        heartbeat_service._pubsub_client.on_channel_message.assert_not_called()

    async def test_tracks_active_session(self, command_service):
        """Test adds session to _active_operator_sessions."""
        await command_service._pubsub_service.register_operator_session("op-abc", "sess-xyz")
        assert ("op-abc", "sess-xyz") in command_service._pubsub_service._active_operator_sessions_set

    async def test_idempotent_for_same_session(self, command_service):
        """Test second registration of same session is a no-op."""
        await command_service._pubsub_service.register_operator_session("op-abc", "sess-xyz")
        await command_service._pubsub_service.register_operator_session("op-abc", "sess-xyz")
        assert command_service._pubsub_service.pubsub_client.subscribe.call_count == 1

    async def test_multiple_sessions_subscribed_independently(self, command_service):
        """Test different operator sessions each get their own channels."""
        await command_service._pubsub_service.register_operator_session("op-1", "sess-1")
        await command_service._pubsub_service.register_operator_session("op-2", "sess-2")

        subscribe_calls = [str(c) for c in command_service._pubsub_service.pubsub_client.subscribe.call_args_list]
        assert any("op-1" in c for c in subscribe_calls)
        assert any("op-2" in c for c in subscribe_calls)
        assert len(command_service._pubsub_service._active_operator_sessions_set) == 2


class TestDeregisterOperatorSession:
    """Test per-operator exact channel unsubscription."""

    async def test_unsubscribes_exact_channels(self, command_service, heartbeat_service):
        """Test unsubscribes the exact results channel for the operator session."""
        await command_service._pubsub_service.register_operator_session("op-abc", "sess-xyz")
        await command_service._pubsub_service.deregister_operator_session("op-abc", "sess-xyz")

        command_service._pubsub_service.pubsub_client.unsubscribe.assert_called_once_with(
            PubSubChannel.results("op-abc", "sess-xyz")
        )
        heartbeat_service._pubsub_client.unsubscribe.assert_not_called()

    async def test_deregisters_per_channel_handlers(self, command_service, heartbeat_service):
        """Test removes the results channel on_channel_message handler on deregister."""
        await command_service._pubsub_service.register_operator_session("op-abc", "sess-xyz")
        await command_service._pubsub_service.deregister_operator_session("op-abc", "sess-xyz")

        command_service._pubsub_service.pubsub_client.off_channel_message.assert_called_once_with(
            PubSubChannel.results("op-abc", "sess-xyz"), command_service._pubsub_service._dispatch_results_message
        )
        heartbeat_service._pubsub_client.off_channel_message.assert_not_called()

    async def test_removes_from_active_sessions(self, command_service):
        """Test removes session from _active_operator_sessions."""
        await command_service._pubsub_service.register_operator_session("op-abc", "sess-xyz")
        await command_service._pubsub_service.deregister_operator_session("op-abc", "sess-xyz")
        assert ("op-abc", "sess-xyz") not in command_service._pubsub_service._active_operator_sessions_set


class TestDispatchResultsMessage:
    """Test _dispatch_results_message routing."""

    async def test_routes_to_result_handler(self, command_service):
        """Test dispatches parsed message to _handle_pubsub_result_message."""
        with patch.object(command_service._pubsub_service, "_handle_pubsub_result_message", new=AsyncMock()) as mock_handle:
            data = json.dumps({"event_type": EventType.OPERATOR_COMMAND_COMPLETED, "payload": {}})
            await command_service._pubsub_service._dispatch_results_message(PubSubChannel.results("op-1", "sess-1"), data)
            mock_handle.assert_called_once_with(
                "op-1", "sess-1", {"event_type": EventType.OPERATOR_COMMAND_COMPLETED, "payload": {}}
            )

    async def test_ignores_invalid_channel_format(self, command_service):
        """Test silently ignores channels that cannot be parsed."""
        command_service._pubsub_service._handle_pubsub_result_message = AsyncMock()
        await command_service._pubsub_service._dispatch_results_message("bad-channel", "{}")

        command_service._pubsub_service._handle_pubsub_result_message.assert_not_called()

    async def test_handles_exception_gracefully(self, command_service):
        """Test does not raise on internal errors."""
        command_service._pubsub_service._handle_pubsub_result_message = AsyncMock(
            side_effect=Exception("boom")
        )
        await command_service._pubsub_service._dispatch_results_message(
            PubSubChannel.results("op-1", "sess-1"), json.dumps({"event_type": "x"})
        )


class TestHandlePubSubResultMessage:
    """Test _handle_pubsub_result_message processing."""

    async def test_routes_to_result_handler(self, command_service):
        """Test routes valid message to _result_handler."""
        handler = AsyncMock()
        command_service._pubsub_service.subscribe_results(handler)

        data = {
            "event_type": EventType.OPERATOR_COMMAND_COMPLETED,
            "case_id": "case-123",
            "investigation_id": "inv-456",
            "payload": {"execution_id": "exec-789", "status": ExecutionStatus.COMPLETED},
        }

        await command_service._pubsub_service._handle_pubsub_result_message("op-123", "session-456", data)

        handler.assert_called_once()

    async def test_ignores_missing_event_type(self, command_service):
        """Test ignores messages without event_type."""
        handler = AsyncMock()
        command_service._pubsub_service.subscribe_results(handler)

        await command_service._pubsub_service._handle_pubsub_result_message(
            "op-123", "session-456", {"payload": {"some": "data"}}
        )

        handler.assert_not_called()

    async def test_extracts_ids_from_payload(self, command_service):
        """Test extracts IDs from payload when not at top level."""
        handler = AsyncMock()
        command_service._pubsub_service.subscribe_results(handler)

        data = {
            "event_type": EventType.OPERATOR_COMMAND_COMPLETED,
            "payload": {
                "execution_id": "exec-001",
                "status": ExecutionStatus.COMPLETED,
                "case_id": "case-from-payload",
                "investigation_id": "inv-from-payload",
            },
        }

        await command_service._pubsub_service._handle_pubsub_result_message("op-123", "session-456", data)

        message = handler.call_args[0][0]
        assert message.case_id == "case-from-payload"
        assert message.investigation_id == "inv-from-payload"

    async def test_extracts_ids_from_top_level(self, command_service):
        """Test prefers IDs from top level over payload."""
        handler = AsyncMock()
        command_service._pubsub_service.subscribe_results(handler)

        data = {
            "event_type": EventType.OPERATOR_COMMAND_COMPLETED,
            "case_id": "case-top",
            "investigation_id": "inv-top",
            "task_id": "task-top",
            "payload": {"execution_id": "exec-002", "status": ExecutionStatus.COMPLETED},
        }

        await command_service._pubsub_service._handle_pubsub_result_message("op-123", "session-456", data)

        message = handler.call_args[0][0]
        assert message.case_id == "case-top"
        assert message.investigation_id == "inv-top"
        assert message.task_id == "task-top"

    async def test_sets_operator_ids_from_channel(self, command_service):
        """Test operator_id and operator_session_id come from the channel name."""
        handler = AsyncMock()
        command_service._pubsub_service.subscribe_results(handler)

        await command_service._pubsub_service._handle_pubsub_result_message(
            "my-operator", "my-session",
            {"event_type": EventType.OPERATOR_COMMAND_COMPLETED, "payload": {"execution_id": "exec-003", "status": ExecutionStatus.COMPLETED}}
        )

        message = handler.call_args[0][0]
        assert message.operator_id == "my-operator"
        assert message.operator_session_id == "my-session"

    async def test_handles_exception_gracefully(self, command_service):
        """Test does not raise on processing errors."""
        handler = AsyncMock(side_effect=Exception("Processing error"))
        command_service._pubsub_service.subscribe_results(handler)

        await command_service._pubsub_service._handle_pubsub_result_message(
            "op-123", "session-456",
            {"event_type": EventType.OPERATOR_COMMAND_COMPLETED, "payload": {}}
        )


class TestParseG8eoPayloadMCPReconstruction:
    """Test MCP payload reconstruction in _parse_g8eo_payload."""

    async def test_reconstructs_port_check_payload_from_mcp_metadata(self):
        """Test reconstructs PortCheckResultPayload from MCP metadata."""
        from app.services.operator.pubsub_service import _parse_g8eo_payload
        from app.models.pubsub_messages import PortCheckResultPayload

        # MCP result with structured metadata (Smell #1 Fix pattern)
        original_payload = {
            "execution_id": "exec-123",
            "host": "192.168.1.1",
            "port": 8080,
            "protocol": "tcp",
            "is_open": True,
            "latency_ms": 5.0
        }
        payload_raw = {
            "id": "exec-123",
            "result": {
                "content": [{"type": "text", "text": "Host 192.168.1.1 Port 8080 is OPEN"}],
                "isError": False,
                "_metadata": {
                    "original_payload": original_payload,
                    "event_type": EventType.OPERATOR_NETWORK_PORT_CHECK_COMPLETED
                }
            }
        }

        result = _parse_g8eo_payload(EventType.OPERATOR_MCP_TOOLS_RESULT, payload_raw)
        assert isinstance(result, PortCheckResultPayload)
        assert result.execution_id == "exec-123"
        assert result.host == "192.168.1.1"
        assert result.is_open is True

    async def test_reconstructs_fs_list_payload_from_mcp_metadata(self):
        """Test reconstructs FsListResultPayload from MCP metadata."""
        from app.services.operator.pubsub_service import _parse_g8eo_payload
        from app.models.pubsub_messages import FsListResultPayload

        original_payload = {
            "execution_id": "exec-456",
            "path": "/tmp",
            "status": "completed",
            "operator_id": "op-1",
            "operator_session_id": "sess-1",
            "entries": [
                {"name": "file1.txt", "path": "/tmp/file1.txt", "is_dir": False, "size": 123},
                {"name": "dir1", "path": "/tmp/dir1", "is_dir": True, "size": 4096}
            ],
            "total_count": 2,
            "truncated": False,
            "duration_seconds": 0.5,
            "stdout_size": 0,
            "stderr_size": 0,
            "stored_locally": False
        }
        payload_raw = {
            "id": "exec-456",
            "result": {
                "content": [{"type": "text", "text": "file1.txt\ndir1/"}],
                "isError": False,
                "_metadata": {
                    "original_payload": original_payload,
                    "event_type": EventType.OPERATOR_FILESYSTEM_LIST_COMPLETED
                }
            }
        }

        result = _parse_g8eo_payload(EventType.OPERATOR_MCP_TOOLS_RESULT, payload_raw)
        assert isinstance(result, FsListResultPayload)
        assert len(result.entries) == 2
        assert result.entries[0].name == "file1.txt"
        assert result.entries[0].path == "/tmp/file1.txt"

    async def test_falls_back_to_execution_results_on_missing_metadata(self):
        """Test falls back to ExecutionResultsPayload when metadata is missing."""
        from app.services.operator.pubsub_service import _parse_g8eo_payload
        from app.models.pubsub_messages import ExecutionResultsPayload

        # MCP result with non-JSON text content and NO metadata
        payload_raw = {
            "id": "exec-202",
            "result": {
                "_metadata": {"execution_id": "exec-202"},
                "content": [{"type": "text", "text": "Plain text output"}],
                "isError": False
            }
        }

        result = _parse_g8eo_payload(EventType.OPERATOR_MCP_TOOLS_RESULT, payload_raw)
        assert isinstance(result, ExecutionResultsPayload)
        assert result.stdout == "Plain text output"

    async def test_handles_mcp_error_with_stderr(self):
        """Test MCP errors are routed to stderr in ExecutionResultsPayload."""
        from app.services.operator.pubsub_service import _parse_g8eo_payload
        from app.models.pubsub_messages import ExecutionResultsPayload

        payload_raw = {
            "id": "exec-404",
            "result": {
                "_metadata": {"execution_id": "exec-404"},
                "content": [{"type": "text", "text": "Tool execution failed"}],
                "isError": True
            }
        }

        result = _parse_g8eo_payload(EventType.OPERATOR_MCP_TOOLS_RESULT, payload_raw)
        assert isinstance(result, ExecutionResultsPayload)
        assert result.status == ExecutionStatus.FAILED
        assert "Tool execution failed" in result.stderr


class TestOnG8eoResultCorrelation:
    """Regression coverage for _on_g8eo_result's correlation-key derivation.

    The ``_on_g8eo_result`` closure subscribed in ``OperatorCommandService.build``
    is the choke point that completes the execution-registry waiter. All result
    payloads now carry an execution_id field for request-response correlation.
    """

    async def test_lfaa_payload_with_execution_id_completes_registry(self):
        """FetchFileHistoryResultPayload has execution_id; must complete registry."""
        from tests.fakes.fake_execution_registry import FakeExecutionRegistry
        from tests.fakes.builder import build_command_service

        registry = FakeExecutionRegistry()
        svc = build_command_service(execution_registry=registry)

        execution_id = "exec-history-42"
        registry.allocate(execution_id)

        raw = {
            "id": "envelope-id",
            "event_type": EventType.OPERATOR_FILE_HISTORY_FETCH_COMPLETED,
            "case_id": "case-1",
            "investigation_id": "inv-1",
            "task_id": "fetch_file_history",
            "payload": {
                "execution_id": execution_id,
                "success": True,
                "file_path": "/etc/hosts",
                "history": [],
            },
        }

        await svc._pubsub_service._handle_pubsub_result_message(
            "op-1", "sess-1", raw
        )

        completed = await registry.wait(execution_id, timeout=0.5)
        assert completed, "LFAA payload with execution_id failed to complete registry"
        assert execution_id in registry.complete_calls

        envelope = registry.get_result(execution_id)
        assert envelope is not None
        assert envelope.payload.execution_id == execution_id

    async def test_restore_file_payload_with_execution_id_completes_registry(self):
        """RestoreFileResultPayload has execution_id; must complete registry."""
        from tests.fakes.fake_execution_registry import FakeExecutionRegistry
        from tests.fakes.builder import build_command_service

        registry = FakeExecutionRegistry()
        svc = build_command_service(execution_registry=registry)

        execution_id = "exec-restore-7"
        registry.allocate(execution_id)

        raw = {
            "id": "envelope-id",
            "event_type": EventType.OPERATOR_FILE_RESTORE_COMPLETED,
            "case_id": "case-1",
            "investigation_id": "inv-1",
            "task_id": "restore_file",
            "payload": {
                "execution_id": execution_id,
                "success": True,
                "file_path": "/etc/hosts",
                "commit_hash": "deadbeef",
            },
        }

        await svc._pubsub_service._handle_pubsub_result_message(
            "op-1", "sess-1", raw
        )

        assert await registry.wait(execution_id, timeout=0.5)
        assert execution_id in registry.complete_calls

    async def test_command_payload_with_execution_id_completes_registry(self):
        """When payload.execution_id is present, it must complete the registry."""
        from tests.fakes.fake_execution_registry import FakeExecutionRegistry
        from tests.fakes.builder import build_command_service

        registry = FakeExecutionRegistry()
        svc = build_command_service(execution_registry=registry)

        execution_id = "payload-exec-id"
        registry.allocate(execution_id)

        raw = {
            "id": "envelope-id",
            "event_type": EventType.OPERATOR_COMMAND_COMPLETED,
            "case_id": "case-1",
            "investigation_id": "inv-1",
            "task_id": "command",
            "payload": {
                "execution_id": execution_id,
                "status": ExecutionStatus.COMPLETED,
            },
        }

        await svc._pubsub_service._handle_pubsub_result_message(
            "op-1", "sess-1", raw
        )

        assert await registry.wait(execution_id, timeout=0.5)
        assert execution_id in registry.complete_calls
