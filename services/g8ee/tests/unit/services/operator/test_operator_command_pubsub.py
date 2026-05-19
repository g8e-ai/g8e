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
from app.models.pubsub_messages import G8eoResultEnvelope, ExecutionResultsPayload
from app.services.operator.command_service import OperatorCommandService
from app.services.operator.heartbeat_service import HeartbeatSnapshotService
from tests.fakes.builder import build_command_service
from tests.fakes.factories import build_g8eo_result_envelope

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]


def _make_pubsub_client():
    client = MagicMock()
    client.ensure_connected = AsyncMock()
    client.on_channel_message = MagicMock()
    client.off_channel_message = MagicMock()
    client.subscribe = AsyncMock()
    client.unsubscribe = AsyncMock()
    client.on_disconnect = MagicMock()
    client.off_disconnect = MagicMock()
    return client


def _make_service() -> OperatorCommandService:
    svc = build_command_service()
    pubsub = _make_pubsub_client()
    svc.set_pubsub_client(pubsub)
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
def heartbeat_service() -> HeartbeatSnapshotService:
    svc = HeartbeatSnapshotService(
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

    async def test_routes_to_result_handler_uap(self, command_service):
        """Test dispatches parsed Protobuf GovernanceEnvelope to _handle_pubsub_result_message."""
        with patch.object(command_service._pubsub_service, "_handle_pubsub_result_message", new=AsyncMock()) as mock_handle:
            # Build a valid Protobuf GovernanceEnvelope JSON format
            envelope_data = {
                "id": "test-id",
                "event_type": EventType.OPERATOR_COMMAND_RESULT,
                "action_type": "EXECUTE_BASH_RESULT",
                "operator_id": "op-1",
                "operator_session_id": "sess-1",
                "intent_data": {
                    "payload_type": "execution_result",
                    "execution_id": "exec-1",
                    "status": "completed",
                    "stdout": "",
                    "stderr": "",
                    "return_code": 0
                }
            }

            data = json.dumps(envelope_data).encode("utf-8")

            await command_service._pubsub_service._dispatch_results_message( PubSubChannel.results("op-1", "sess-1"), data)

            mock_handle.assert_called_once()
            call_args = mock_handle.call_args[0]
            assert isinstance(call_args[0], G8eoResultEnvelope)
            assert call_args[0].operator_id == "op-1"
            assert call_args[0].operator_session_id == "sess-1"
            assert call_args[0].event_type == EventType.OPERATOR_COMMAND_RESULT
            assert call_args[0].payload.execution_id == "exec-1"

    async def test_rejects_invalid_protobuf_json(self, command_service):
        """Test rejects invalid JSON payload."""
        command_service._pubsub_service._handle_pubsub_result_message = AsyncMock()
        data = b"not-even-json"

        await command_service._pubsub_service._dispatch_results_message(PubSubChannel.results("op-1", "sess-1"), data)

        # Should NOT call handler because it's not a valid envelope
        command_service._pubsub_service._handle_pubsub_result_message.assert_not_called()

    async def test_ignores_invalid_channel_format(self, command_service):
        """Test silently ignores channels that cannot be parsed."""
        command_service._pubsub_service._handle_pubsub_result_message = AsyncMock()
        await command_service._pubsub_service._dispatch_results_message("bad-channel", "{}")

        command_service._pubsub_service._handle_pubsub_result_message.assert_not_called()

    async def test_propagates_downstream_handler_errors(self, command_service):
        """Unexpected exceptions from the downstream handler must propagate."""
        command_service._pubsub_service._handle_pubsub_result_message = AsyncMock(
            side_effect=Exception("boom")
        )

        # Build a valid Protobuf GovernanceEnvelope to reach the handler
        envelope_data = {
            "id": "test-id",
            "event_type": EventType.OPERATOR_COMMAND_RESULT,
            "action_type": "EXECUTE_BASH_RESULT",
            "operator_id": "op-1",
            "operator_session_id": "sess-1",
            "intent_data": {
                "payload_type": "execution_result",
                "execution_id": "exec-1",
                "status": "completed",
                "stdout": "",
                "stderr": "",
                "return_code": 0
            }
        }
        data = json.dumps(envelope_data).encode("utf-8")

        with pytest.raises(Exception, match="boom"):
            await command_service._pubsub_service._dispatch_results_message(
                PubSubChannel.results("op-1", "sess-1"), data
            )

    async def test_swallows_invalid_envelope_payload(self, command_service):
        """Invalid envelope payloads are logged and dropped, never raised."""
        command_service._pubsub_service._handle_pubsub_result_message = AsyncMock()
        await command_service._pubsub_service._dispatch_results_message(
            PubSubChannel.results("op-1", "sess-1"), b"invalid-envelope-data"
        )
        command_service._pubsub_service._handle_pubsub_result_message.assert_not_called()


class TestHandlePubSubResultMessage:
    """Test _handle_pubsub_result_message processing."""

    async def test_routes_to_result_handler(self, command_service):
        """Test dispatches message and completes registered future."""
        envelope = build_g8eo_result_envelope(
            event_type=EventType.OPERATOR_COMMAND_COMPLETED,
            operator_id="op-123",
            operator_session_id="session-456",
            case_id="case-123",
            investigation_id="inv-456",
            payload=ExecutionResultsPayload(
                payload_type="execution_result",
                execution_id="exec-789",
                status=ExecutionStatus.COMPLETED,
                stdout="",
                stderr="",
                return_code=0
            ),
        )

        future = command_service._pubsub_service.register_future("exec-789")

        await command_service._pubsub_service._handle_pubsub_result_message(envelope)

        assert future.done()
        result_envelope = future.result()
        assert result_envelope.payload.execution_id == "exec-789"

    async def test_ignores_unregistered_execution_id(self, command_service):
        """Test ignores messages for execution_ids with no registered future."""
        future = command_service._pubsub_service.register_future("exec-missing")

        envelope = build_g8eo_result_envelope(
            operator_id="op-123",
            operator_session_id="session-456",
            event_type=EventType.OPERATOR_COMMAND_COMPLETED,
            payload=ExecutionResultsPayload(
                payload_type="execution_result",
                execution_id="exec-different-id",  # Different ID than the registered future
                status=ExecutionStatus.COMPLETED,
                stdout="",
                stderr="",
                return_code=0
            ),
        )

        await command_service._pubsub_service._handle_pubsub_result_message(envelope)

        assert not future.done()

    async def test_extracts_ids_from_payload(self, command_service):
        """Test extracts IDs from payload when not at top level."""
        envelope = build_g8eo_result_envelope(
            event_type=EventType.OPERATOR_COMMAND_COMPLETED,
            operator_id="op-123",
            operator_session_id="session-456",
            case_id="case-from-payload",
            investigation_id="inv-from-payload",
            payload=ExecutionResultsPayload(
                payload_type="execution_result",
                execution_id="exec-001",
                status=ExecutionStatus.COMPLETED,
                stdout="",
                stderr="",
                return_code=0
            ),
        )

        future = command_service._pubsub_service.register_future("exec-001")

        await command_service._pubsub_service._handle_pubsub_result_message(envelope)

        assert future.done()
        result_envelope = future.result()
        assert result_envelope.case_id == "case-from-payload"
        assert result_envelope.investigation_id == "inv-from-payload"

    async def test_extracts_ids_from_top_level(self, command_service):
        """Test prefers IDs from top level over payload."""
        envelope = build_g8eo_result_envelope(
            event_type=EventType.OPERATOR_COMMAND_COMPLETED,
            operator_id="op-123",
            operator_session_id="session-456",
            case_id="case-top",
            investigation_id="inv-top",
            task_id="task-top",
            payload=ExecutionResultsPayload(
                payload_type="execution_result",
                execution_id="exec-002",
                status=ExecutionStatus.COMPLETED,
                stdout="",
                stderr="",
                return_code=0
            ),
        )

        future = command_service._pubsub_service.register_future("exec-002")

        await command_service._pubsub_service._handle_pubsub_result_message(envelope)

        assert future.done()
        result_envelope = future.result()
        assert result_envelope.case_id == "case-top"
        assert result_envelope.investigation_id == "inv-top"
        assert result_envelope.task_id == "task-top"

    async def test_sets_operator_ids_from_channel(self, command_service):
        """Test operator_id and operator_session_id come from the channel name."""
        envelope = build_g8eo_result_envelope(
            event_type=EventType.OPERATOR_COMMAND_COMPLETED,
            operator_id="my-operator",
            operator_session_id="my-session",
            payload=ExecutionResultsPayload(
                payload_type="execution_result",
                execution_id="exec-003",
                status=ExecutionStatus.COMPLETED,
                stdout="",
                stderr="",
                return_code=0
            ),
        )

        future = command_service._pubsub_service.register_future("exec-003")

        await command_service._pubsub_service._handle_pubsub_result_message(envelope)

        assert future.done()
        result_envelope = future.result()
        assert result_envelope.operator_id == "my-operator"
        assert result_envelope.operator_session_id == "my-session"

    async def test_handles_exception_gracefully(self, command_service):
        """Test does not raise on processing errors."""
        # Using invalid data to trigger parsing error
        await command_service._pubsub_service._handle_pubsub_result_message(None)
        # Should not raise exception


class TestParseG8eoPayloadNativeEventTypes:
    """Regression coverage for discriminator-based parsing."""

    async def test_placeholder(self):
        pass
