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

import json
from unittest.mock import AsyncMock, MagicMock, call, patch

import pytest

from app.constants import EventType, HeartbeatType, OperatorStatus, PubSubChannel
from app.errors import ConfigurationError
from app.models.events import BackgroundEvent, SessionEvent
from app.models.operators import (
    HeartbeatMetrics,
    HeartbeatSSEEnvelope,
    OperatorDocument,
    OperatorHeartbeat,
)
from app.models.pubsub_messages import G8eoHeartbeatPayload
from app.services.operator.heartbeat_service import OperatorHeartbeatService
from app.utils.timestamp import now

pytestmark = [pytest.mark.unit]


def _make_payload(**kwargs) -> G8eoHeartbeatPayload:
    defaults = dict(
        event_type=EventType.OPERATOR_HEARTBEAT_SENT,
        operator_id="op-222",
        operator_session_id="op-session-111",
        timestamp=now().isoformat(),
    )
    defaults.update(kwargs)
    return G8eoHeartbeatPayload(**defaults)


def _make_mock_pubsub_client() -> MagicMock:
    client = MagicMock()
    client.ensure_connected = AsyncMock()
    client.subscribe = AsyncMock()
    client.unsubscribe = AsyncMock()
    client.on_channel_message = MagicMock()
    client.off_channel_message = MagicMock()
    client.on_disconnect = MagicMock()
    client.off_disconnect = MagicMock()
    return client


def _make_service(operator_data_service=None, event_service=None):
    return OperatorHeartbeatService(
        operator_data_service=operator_data_service or MagicMock(),
        event_service=event_service or MagicMock(),
    )


class TestSetPubsubClient:
    """set_pubsub_client — configuration guard and assignment."""

    @pytest.fixture
    def service(self):
        return _make_service()

    async def test_raises_configuration_error_when_client_is_none(self, service):
        with pytest.raises(ConfigurationError):
            service.set_pubsub_client(None)

    async def test_raises_configuration_error_when_client_is_falsy(self, service):
        with pytest.raises(ConfigurationError):
            service.set_pubsub_client(0)

    async def test_assigns_client_when_valid(self, service):
        client = _make_mock_pubsub_client()
        service.set_pubsub_client(client)
        assert service._pubsub_client is client


class TestHeartbeatServiceLifecycle:
    """start() and stop() — lifecycle management."""

    @pytest.fixture
    def service(self):
        return _make_service()

    async def test_start_raises_configuration_error_without_client(self, service):
        with pytest.raises(ConfigurationError, match="pubsub_client must be set"):
            await service.start()

    async def test_start_calls_ensure_ws_and_sets_ready(self, service):
        client = _make_mock_pubsub_client()
        service.set_pubsub_client(client)

        await service.start()

        client.ensure_connected.assert_called_once()
        assert service._ready is True

    async def test_start_is_idempotent_when_already_ready(self, service):
        client = _make_mock_pubsub_client()
        service.set_pubsub_client(client)
        await service.start()
        client.ensure_connected.reset_mock()

        await service.start()

        client.ensure_connected.assert_not_called()

    async def test_stop_unsubscribes_all_active_sessions_and_clears_ready(self, service):
        client = _make_mock_pubsub_client()
        service.set_pubsub_client(client)
        service._active_sessions = {("op-1", "sess-1"), ("op-2", "sess-2")}
        service._ready = True

        await service.stop()

        assert service._ready is False
        assert client.unsubscribe.call_count == 2

    async def test_stop_with_no_active_sessions_clears_ready(self, service):
        service._ready = True

        await service.stop()

        assert service._ready is False


class TestRegisterDeregisterSession:
    """register_operator_session / deregister_operator_session — pub/sub channel management."""

    @pytest.fixture
    def service(self):
        return _make_service()

    async def test_register_raises_configuration_error_without_client(self, service):
        with pytest.raises(ConfigurationError, match="pubsub_client is not set"):
            await service.register_operator_session("op-1", "sess-1")

    async def test_register_subscribes_to_channel(self, service):
        client = _make_mock_pubsub_client()
        service.set_pubsub_client(client)

        await service.register_operator_session("op-1", "sess-1")

        client.on_channel_message.assert_called_once()
        client.subscribe.assert_called_once()
        assert ("op-1", "sess-1") in service._active_sessions

    async def test_register_is_idempotent_for_same_session(self, service):
        client = _make_mock_pubsub_client()
        service.set_pubsub_client(client)

        await service.register_operator_session("op-1", "sess-1")
        await service.register_operator_session("op-1", "sess-1")

        assert client.subscribe.call_count == 1

    async def test_deregister_removes_session_and_calls_unsubscribe(self, service):
        client = _make_mock_pubsub_client()
        service.set_pubsub_client(client)
        await service.register_operator_session("op-1", "sess-1")

        await service.deregister_operator_session("op-1", "sess-1")

        assert ("op-1", "sess-1") not in service._active_sessions
        client.unsubscribe.assert_called_once()
        client.off_channel_message.assert_called_once()

    async def test_deregister_without_client_does_not_raise(self, service):
        service._active_sessions = {("op-1", "sess-1")}

        await service.deregister_operator_session("op-1", "sess-1")

        assert ("op-1", "sess-1") not in service._active_sessions

    async def test_deregister_nonexistent_session_does_not_raise(self, service):
        client = _make_mock_pubsub_client()
        service.set_pubsub_client(client)

        await service.deregister_operator_session("op-999", "sess-999")

        client.unsubscribe.assert_called_once()


class TestOnHeartbeatMessage:
    """_on_heartbeat_message — channel routing, data parsing, session auto-register, exception guard."""

    @pytest.fixture
    def service(self):
        svc = _make_service()
        svc._active_sessions = {("op-222", "op-session-111")}
        return svc

    async def test_on_heartbeat_message_parses_dict_data_and_calls_process(self, service):
        payload = _make_payload()
        data = payload.model_dump()

        with patch.object(service, "process_heartbeat_message", new=AsyncMock(return_value=True)) as mock_proc:
            await service._on_heartbeat_message(PubSubChannel.heartbeat("op-222", "op-session-111"), data)

        mock_proc.assert_called_once()
        call_args = mock_proc.call_args
        assert call_args.args[0] == "op-222"
        assert call_args.args[1] == "op-session-111"

    async def test_on_heartbeat_message_parses_json_string_data(self, service):
        data = json.dumps(_make_payload().model_dump(mode="json"))

        with patch.object(service, "process_heartbeat_message", new=AsyncMock(return_value=True)) as mock_proc:
            await service._on_heartbeat_message(PubSubChannel.heartbeat("op-222", "op-session-111"), data)

        mock_proc.assert_called_once()

    async def test_on_heartbeat_message_auto_registers_unknown_session(self, service):
        service._active_sessions = set()
        data = _make_payload().model_dump()

        with patch.object(service, "register_operator_session", new=AsyncMock()) as mock_reg:
            with patch.object(service, "process_heartbeat_message", new=AsyncMock(return_value=True)):
                await service._on_heartbeat_message(PubSubChannel.heartbeat("op-222", "op-session-111"), data)

        mock_reg.assert_called_once_with("op-222", "op-session-111")

    async def test_on_heartbeat_message_swallows_exceptions_silently(self, service):
        with patch.object(service, "process_heartbeat_message", new=AsyncMock(side_effect=RuntimeError("boom"))):
            await service._on_heartbeat_message(PubSubChannel.heartbeat("op-222", "op-session-111"), _make_payload().model_dump())


class TestOperatorHeartbeatServiceIdentity:
    """_validate_operator_identity — channel vs wire claim cross-check."""

    @pytest.fixture
    def service(self):
        return _make_service()

    async def test_validate_matching_operator_id(self, service):
        payload = G8eoHeartbeatPayload(operator_id="op-222")
        assert service._validate_operator_identity("op-222", payload, "sess-111") is True

    async def test_validate_mismatched_operator_id_rejected(self, service):
        payload = G8eoHeartbeatPayload(operator_id="op-EVIL")
        assert service._validate_operator_identity("op-222", payload, "sess-111") is False

    async def test_validate_no_payload_id_passes(self, service):
        payload = G8eoHeartbeatPayload(operator_id=None)
        assert service._validate_operator_identity("op-222", payload, "sess-111") is True


class TestOperatorHeartbeatServiceOperatorValidation:
    """_get_and_validate_operator — data service lookup and status gating."""

    @pytest.fixture
    def mock_operator_data_service(self):
        svc = MagicMock()
        svc.get_operator = AsyncMock()
        return svc

    @pytest.fixture
    def service(self, mock_operator_data_service):
        return _make_service(operator_data_service=mock_operator_data_service)

    async def test_cache_hit_returns_operator(self, service, mock_operator_data_service):
        operator = OperatorDocument(operator_id="op-222", status=OperatorStatus.ACTIVE, user_id="user-1", bound_web_session_id="ws-1")
        mock_operator_data_service.get_operator.return_value = operator

        result = await service._get_and_validate_operator("op-222", "sess-111", _make_payload())

        assert result is operator
        mock_operator_data_service.get_operator.assert_called_once_with("op-222")

    async def test_cache_miss_returns_none(self, service, mock_operator_data_service):
        mock_operator_data_service.get_operator.return_value = None

        result = await service._get_and_validate_operator("op-unknown", "sess-111", _make_payload())

        assert result is None

    async def test_any_known_operator_status_accepted(self, service, mock_operator_data_service):
        """Heartbeats are accepted for any OperatorDocument regardless of its status.

        The operator's status is a property of the operator doc itself; it is not
        a gate for receiving heartbeats. Unknown operators (None) are still rejected
        upstream via API-key validation — see test_cache_miss_returns_none.
        """
        for status in OperatorStatus:
            operator = OperatorDocument(operator_id="op-222", status=status, user_id="user-1", bound_web_session_id="ws-1")
            mock_operator_data_service.get_operator.return_value = operator

            result = await service._get_and_validate_operator("op-222", "sess-111", _make_payload())

            assert result is not None, f"Status {status} should not gate heartbeat acceptance"



class TestOperatorHeartbeatServiceProcessMessage:
    """process_heartbeat_message — full pipeline using G8eoHeartbeatPayload directly."""

    @pytest.fixture
    def mock_operator_data_service(self):
        svc = MagicMock()
        svc.get_operator = AsyncMock()
        svc.update_operator_heartbeat = AsyncMock(return_value=True)
        return svc

    @pytest.fixture
    def mock_event_service(self):
        svc = MagicMock()
        svc.publish = AsyncMock()
        return svc

    @pytest.fixture
    def service(self, mock_operator_data_service, mock_event_service):
        return _make_service(operator_data_service=mock_operator_data_service, event_service=mock_event_service)

    @pytest.fixture
    def bound_operator(self):
        return OperatorDocument(
            operator_id="op-222",
            status=OperatorStatus.ACTIVE,
            user_id="user-333",
            bound_web_session_id="web-999",
        )

    async def test_success_writes_cache_and_publishes_sse(
        self, service, mock_operator_data_service, mock_event_service, bound_operator
    ):
        mock_operator_data_service.get_operator.return_value = bound_operator

        result = await service.process_heartbeat_message(
            "op-222", "op-session-111", _make_payload(investigation_id="inv-789")
        )

        assert result is True
        mock_operator_data_service.update_operator_heartbeat.assert_called_once()
        mock_event_service.publish.assert_called()

    async def test_cache_write_called_with_typed_heartbeat(
        self, service, mock_operator_data_service, bound_operator
    ):
        mock_operator_data_service.get_operator.return_value = bound_operator

        await service.process_heartbeat_message(
            "op-222", "op-session-111",
            _make_payload(investigation_id="inv-789", case_id="case-456")
        )

        _, kwargs = mock_operator_data_service.update_operator_heartbeat.call_args
        assert isinstance(kwargs["heartbeat"], OperatorHeartbeat)
        assert kwargs["investigation_id"] == "inv-789"
        assert kwargs["case_id"] == "case-456"

    async def test_sse_payload_status_set_from_operator(
        self, service, mock_operator_data_service, mock_event_service
    ):
        operator = OperatorDocument(operator_id="op-222", status=OperatorStatus.BOUND, bound_web_session_id="web-999", user_id="user-1")
        mock_operator_data_service.get_operator.return_value = operator

        await service.process_heartbeat_message("op-222", "op-session-111", _make_payload())

        first_call = mock_event_service.publish.call_args_list[0]
        event = first_call.args[0]
        assert isinstance(event, SessionEvent)
        assert event.payload.status == OperatorStatus.BOUND

    async def test_stale_timestamp_never_reaches_cache(
        self, service, mock_operator_data_service
    ):
        result = await service.process_heartbeat_message(
            "op-222", "op-session-111",
            _make_payload(timestamp="2000-01-01T00:00:00+00:00")
        )

        assert result is False
        mock_operator_data_service.update_operator_heartbeat.assert_not_called()
        mock_operator_data_service.get_operator.assert_not_called()

    async def test_cache_write_failure_returns_false(
        self, service, mock_operator_data_service, bound_operator
    ):
        mock_operator_data_service.get_operator.return_value = bound_operator
        # Explicitly return False to simulate a CacheAside write failure
        mock_operator_data_service.update_operator_heartbeat = AsyncMock(return_value=False)

        result = await service.process_heartbeat_message("op-222", "op-session-111", _make_payload())

        assert result is False

    async def test_identity_mismatch_returns_false(self, service, mock_operator_data_service, bound_operator):
        mock_operator_data_service.get_operator.return_value = bound_operator

        result = await service.process_heartbeat_message(
            "op-222", "op-session-111",
            _make_payload(operator_id="op-EVIL")
        )

        assert result is False

    async def test_unknown_operator_returns_false(self, service, mock_operator_data_service):
        mock_operator_data_service.get_operator.return_value = None

        result = await service.process_heartbeat_message(
            "op-unknown", "op-session-111", _make_payload(operator_id="op-unknown")
        )

        assert result is False

    async def test_does_not_publish_panel_list_updated(
        self, service, mock_operator_data_service, mock_event_service, bound_operator
    ):
        """Heartbeat must NOT publish OPERATOR_PANEL_LIST_UPDATED.

        That event's shape is the full operator list (delivered via keepalive).
        Publishing a sparse per-heartbeat payload under the same event type
        causes the frontend to wipe its operator list — regression guard.
        """
        mock_operator_data_service.get_operator.return_value = bound_operator

        result = await service.process_heartbeat_message(
            "op-222", "op-session-111",
            _make_payload(investigation_id="inv-789", case_id="case-456")
        )

        assert result is True
        event_types = [c.args[0].event_type for c in mock_event_service.publish.call_args_list]
        assert EventType.OPERATOR_PANEL_LIST_UPDATED not in event_types
        assert EventType.OPERATOR_HEARTBEAT_RECEIVED in event_types

    async def test_sse_failure_does_not_fail_heartbeat(
        self, service, mock_operator_data_service, mock_event_service, bound_operator
    ):
        mock_operator_data_service.get_operator.return_value = bound_operator
        mock_event_service.publish.side_effect = Exception("SSE down")

        result = await service.process_heartbeat_message("op-222", "op-session-111", _make_payload())

        assert result is True


class TestPushHeartbeatSSE:
    """_push_heartbeat_sse — EventService.publish paths and no-web-session guard."""

    @pytest.fixture
    def mock_event_service(self):
        svc = MagicMock()
        svc.publish = AsyncMock()
        return svc

    @pytest.fixture
    def service(self, mock_event_service):
        return _make_service(event_service=mock_event_service)

    async def test_publishes_heartbeat_event_when_web_session_bound(
        self, service, mock_event_service
    ):
        operator = OperatorDocument(
            operator_id="op-222", status=OperatorStatus.ACTIVE, bound_web_session_id="web-999", user_id="user-1"
        )
        envelope = HeartbeatSSEEnvelope(
            operator_id="op-222",
            status=OperatorStatus.ACTIVE,
            metrics=HeartbeatMetrics(timestamp=now(), heartbeat_type=HeartbeatType.AUTOMATIC),
        )
        payload = _make_payload()

        await service._push_heartbeat_sse(envelope, payload, operator)

        mock_event_service.publish.assert_called()
        first_call = mock_event_service.publish.call_args_list[0]
        event = first_call.args[0]
        assert isinstance(event, SessionEvent)
        assert event.event_type == EventType.OPERATOR_HEARTBEAT_RECEIVED
        assert event.web_session_id == "web-999"

    async def test_skips_publish_when_operator_has_no_user_id(
        self, service, mock_event_service
    ):
        operator = OperatorDocument(
            operator_id="op-222", status=OperatorStatus.ACTIVE, bound_web_session_id=None
        )
        envelope = HeartbeatSSEEnvelope(
            operator_id="op-222",
            status=OperatorStatus.ACTIVE,
            metrics=HeartbeatMetrics(timestamp=now(), heartbeat_type=HeartbeatType.AUTOMATIC),
        )

        await service._push_heartbeat_sse(envelope, _make_payload(), operator)

        mock_event_service.publish.assert_not_called()

    async def test_publishes_background_event_when_unbound_but_has_user_id(
        self, service, mock_event_service
    ):
        operator = OperatorDocument(
            operator_id="op-222",
            status=OperatorStatus.ACTIVE,
            bound_web_session_id=None,
            user_id="user-7",
        )
        envelope = HeartbeatSSEEnvelope(
            operator_id="op-222",
            status=OperatorStatus.ACTIVE,
            metrics=HeartbeatMetrics(timestamp=now(), heartbeat_type=HeartbeatType.AUTOMATIC),
        )

        await service._push_heartbeat_sse(envelope, _make_payload(), operator)

        mock_event_service.publish.assert_called_once()
        event = mock_event_service.publish.call_args.args[0]
        assert isinstance(event, BackgroundEvent)
        assert event.user_id == "user-7"
        assert event.investigation_id is None

    async def test_background_event_preserves_investigation_id_when_present(
        self, service, mock_event_service
    ):
        operator = OperatorDocument(
            operator_id="op-222",
            status=OperatorStatus.ACTIVE,
            bound_web_session_id=None,
            user_id="user-7",
        )
        envelope = HeartbeatSSEEnvelope(
            operator_id="op-222",
            status=OperatorStatus.ACTIVE,
            metrics=HeartbeatMetrics(timestamp=now(), heartbeat_type=HeartbeatType.AUTOMATIC),
        )

        await service._push_heartbeat_sse(
            envelope, _make_payload(investigation_id="inv-42"), operator
        )

        event = mock_event_service.publish.call_args.args[0]
        assert isinstance(event, BackgroundEvent)
        assert event.investigation_id == "inv-42"

    async def test_sse_exception_does_not_propagate(
        self, service, mock_event_service
    ):
        operator = OperatorDocument(
            operator_id="op-222", status=OperatorStatus.ACTIVE, bound_web_session_id="web-999", user_id="user-1"
        )
        envelope = HeartbeatSSEEnvelope(
            operator_id="op-222",
            status=OperatorStatus.ACTIVE,
            metrics=HeartbeatMetrics(timestamp=now(), heartbeat_type=HeartbeatType.AUTOMATIC),
        )
        mock_event_service.publish.side_effect = Exception("network down")

        await service._push_heartbeat_sse(envelope, _make_payload(), operator)



class TestValidateHeartbeatTimestamp:
    """_validate_heartbeat_timestamp — reads timestamp directly from G8eoHeartbeatPayload."""

    @pytest.fixture
    def service(self):
        return _make_service()

    async def test_valid_timestamp_returns_valid(self, service):
        result = service._validate_heartbeat_timestamp(_make_payload(timestamp=now().isoformat()))
        assert result.is_valid is True

    async def test_stale_timestamp_returns_invalid(self, service):
        result = service._validate_heartbeat_timestamp(
            _make_payload(timestamp="2000-01-01T00:00:00+00:00")
        )
        assert result.is_valid is False
        assert result.error is not None

    async def test_missing_timestamp_returns_invalid(self, service):
        result = service._validate_heartbeat_timestamp(_make_payload(timestamp=None))
        assert result.is_valid is False


class TestWsDisconnectHandler:
    """_on_ws_disconnect — resets ready state and clears active sessions."""

    @pytest.fixture
    def service(self):
        return _make_service()

    async def test_disconnect_resets_ready_flag(self, service):
        service._ready = True
        service._active_sessions = {("op-1", "sess-1")}

        await service._on_ws_disconnect()

        assert service._ready is False

    async def test_disconnect_clears_active_sessions(self, service):
        service._ready = True
        service._active_sessions = {("op-1", "sess-1"), ("op-2", "sess-2")}

        await service._on_ws_disconnect()

        assert len(service._active_sessions) == 0

    async def test_disconnect_handler_registered_on_set_pubsub_client(self, service):
        client = _make_mock_pubsub_client()
        client.on_disconnect = MagicMock()

        service.set_pubsub_client(client)

        client.on_disconnect.assert_called_once_with(service._on_ws_disconnect)

    async def test_start_resets_ready_after_disconnect(self, service):
        client = _make_mock_pubsub_client()
        service.set_pubsub_client(client)
        await service.start()
        assert service._ready is True

        await service._on_ws_disconnect()
        assert service._ready is False

        await service.start()
        assert service._ready is True


class TestG8eoHeartbeatPayloadApiKey:
    """G8eoHeartbeatPayload — api_key field acceptance and default."""

    def test_api_key_field_accepted(self):
        payload = _make_payload(api_key="g8e_abc123")
        assert payload.api_key == "g8e_abc123"

    def test_api_key_defaults_to_none(self):
        payload = _make_payload()
        assert payload.api_key is None

    def test_api_key_none_explicit(self):
        payload = _make_payload(api_key=None)
        assert payload.api_key is None


class TestHeartbeatServiceConstruction:
    """Direct construction via __init__ — dependency injection contract."""

    async def test_constructs_with_required_dependencies(self):
        svc = OperatorHeartbeatService(
            operator_data_service=MagicMock(),
            event_service=MagicMock(),
        )
        assert svc._ready is False
        assert svc._pubsub_client is None
        assert len(svc._active_sessions) == 0

    async def test_distinct_instances_are_independent(self):
        svc1 = _make_service()
        svc2 = _make_service()
        assert svc1 is not svc2
