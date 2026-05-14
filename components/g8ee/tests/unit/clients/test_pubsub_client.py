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
import json
from unittest.mock import ANY, AsyncMock, MagicMock, patch

import aiohttp
import pytest

from app.clients.pubsub_client import PubSubClient
from app.constants import (
    ComponentName,
    PubSubAction,
    PubSubField,
    PubSubMessageType,
    PubSubWireEventType,
)
from tests.fakes.async_helpers import async_iter

pytestmark = pytest.mark.unit

@pytest.fixture
def disconnected_client():
    client = PubSubClient(
        pubsub_url="wss://localhost:9001",
        component_name=ComponentName.G8EE,
        auditor_hmac_key="test-key-1234",
    )
    return client

class TestPubSubClientInit:
    def test_explicit_urls_override_defaults(self):
        client = PubSubClient(
            pubsub_url="wss://custom-host",
        )
        assert client.pubsub_url == "wss://custom-host"

    def test_trailing_slash_stripped_from_urls(self):
        client = PubSubClient(
            pubsub_url="wss://localhost:9001/",
        )
        assert not client.pubsub_url.endswith("/")

    def test_component_name_default_is_g8ee(self):
        client = PubSubClient()
        assert client.component_name is ComponentName.G8EE

class TestPubSubWireProtocolConstants:
    """Regression: PubSubWireEventType/PubSubAction/PubSubField must include
    wire protocol members that _ws_reader depends on. Without these, the
    reader crashes on first message with AttributeError and subscribe ACKs
    are never processed (TimeoutError)."""

    def test_wire_event_type_has_all_members(self):
        assert PubSubWireEventType.MESSAGE == "message"
        assert PubSubWireEventType.PMESSAGE == "pmessage"
        assert PubSubWireEventType.SUBSCRIBED == "subscribed"

    def test_backward_compat_alias_is_wire_event_type(self):
        assert PubSubMessageType is PubSubWireEventType

    def test_pubsub_action_has_psubscribe(self):
        assert PubSubAction.PSUBSCRIBE == "psubscribe"

    def test_pubsub_field_has_pattern(self):
        assert PubSubField.PATTERN == "pattern"

    def test_wire_protocol_values_match_operator_go_constants(self):
        assert PubSubWireEventType.MESSAGE.value == "message"
        assert PubSubWireEventType.PMESSAGE.value == "pmessage"
        assert PubSubWireEventType.SUBSCRIBED.value == "subscribed"
        assert PubSubAction.SUBSCRIBE.value == "subscribe"
        assert PubSubAction.PSUBSCRIBE.value == "psubscribe"
        assert PubSubAction.UNSUBSCRIBE.value == "unsubscribe"
        assert PubSubAction.PUBLISH.value == "publish"


@pytest.fixture
def connected_client(disconnected_client):
    """PubSubClient with a mocked WebSocket in place."""
    mock_ws = AsyncMock(spec=aiohttp.ClientWebSocketResponse)
    mock_ws.closed = False
    disconnected_client._ws = mock_ws
    disconnected_client._ws_session = AsyncMock(spec=aiohttp.ClientSession)
    return disconnected_client


@pytest.mark.asyncio
class TestPubSubClientSubscribe:
    async def test_subscribe_sends_action(self, connected_client, task_tracker):
        channel = "test-channel"

        async def mock_ack():
            await asyncio.sleep(0.01)
            if channel in connected_client._pending_acks:
                for ack in connected_client._pending_acks[channel]:
                    ack.set()

        task = task_tracker.track(asyncio.create_task(mock_ack()))
        await connected_client.subscribe(channel)
        await task

        connected_client._ws.send_bytes.assert_called()

    async def test_subscribe_adds_channel_after_ensure_ws(self, connected_client, task_tracker):
        """Regression: channel must be added to _subscribed_channels after
        _ensure_ws() so reconnect doesn't re-subscribe before ACK handler
        exists."""
        channel = "heartbeat:op-1:sess-1"

        async def mock_ack():
            await asyncio.sleep(0.01)
            if channel in connected_client._pending_acks:
                for ack in connected_client._pending_acks[channel]:
                    ack.set()

        task = task_tracker.track(asyncio.create_task(mock_ack()))
        await connected_client.subscribe(channel)
        await task
        assert channel in connected_client._subscribed_channels

    async def test_subscribe_cleans_up_ack_event(self, connected_client, task_tracker):
        channel = "cleanup-channel"

        async def mock_ack():
            await asyncio.sleep(0.01)
            if channel in connected_client._pending_acks:
                for ack in connected_client._pending_acks[channel]:
                    ack.set()

        task = task_tracker.track(asyncio.create_task(mock_ack()))
        await connected_client.subscribe(channel)
        await task
        assert channel not in connected_client._pending_acks


@pytest.mark.asyncio
class TestPubSubClientPsubscribe:
    async def test_psubscribe_sends_action(self, connected_client, task_tracker):
        pattern = "heartbeat:*"

        async def mock_ack():
            await asyncio.sleep(0.01)
            if pattern in connected_client._pending_acks:
                for ack in connected_client._pending_acks[pattern]:
                    ack.set()

        task = task_tracker.track(asyncio.create_task(mock_ack()))
        await connected_client.psubscribe(pattern)
        await task

        connected_client._ws.send_bytes.assert_called()

    async def test_psubscribe_adds_pattern_after_ensure_ws(self, connected_client, task_tracker):
        """Regression: pattern must be added to _subscribed_patterns after
        _ensure_ws() — same race condition as subscribe()."""
        pattern = "results:*"

        async def mock_ack():
            await asyncio.sleep(0.01)
            if pattern in connected_client._pending_acks:
                for ack in connected_client._pending_acks[pattern]:
                    ack.set()

        task = task_tracker.track(asyncio.create_task(mock_ack()))
        await connected_client.psubscribe(pattern)
        await task
        assert pattern in connected_client._subscribed_patterns

    async def test_psubscribe_cleans_up_ack_event(self, connected_client, task_tracker):
        pattern = "cleanup:*"

        async def mock_ack():
            await asyncio.sleep(0.01)
            if pattern in connected_client._pending_acks:
                for ack in connected_client._pending_acks[pattern]:
                    ack.set()

        task = task_tracker.track(asyncio.create_task(mock_ack()))
        await connected_client.psubscribe(pattern)
        await task
        assert pattern not in connected_client._pending_acks

    async def test_psubscribe_waits_for_ack(self, connected_client):
        """psubscribe should timeout if no ACK arrives within 5 seconds."""
        pattern = "never-acked:*"
        with pytest.raises(asyncio.TimeoutError):
            await connected_client.psubscribe(pattern)


@pytest.mark.asyncio
class TestWsReaderReconnection:
    async def test_reader_nulls_ws_on_exit(self, connected_client, task_tracker):
        """_ws_reader must set self._ws = None so _ensure_ws knows to reconnect."""
        mock_ws = connected_client._ws
        mock_ws.__aiter__ = MagicMock(return_value=async_iter([]))
        connected_client._pubsub_task = task_tracker.track(asyncio.create_task(connected_client._ws_reader()))
        await connected_client._pubsub_task
        assert connected_client._ws is None

    async def test_reader_triggers_reconnect_with_active_channels(self, connected_client, task_tracker):
        """When _ws_reader exits with active subscriptions, it should schedule
        _reconnect_loop to restore the connection."""
        connected_client._subscribed_channels.add("heartbeat:op-1:sess-1")
        mock_ws = connected_client._ws
        mock_ws.__aiter__ = MagicMock(return_value=async_iter([]))

        reconnect_called = asyncio.Event()
        original_reconnect = connected_client._reconnect_loop

        async def mock_reconnect():
            reconnect_called.set()

        connected_client._reconnect_loop = mock_reconnect
        connected_client._pubsub_task = task_tracker.track(asyncio.create_task(connected_client._ws_reader()))
        await connected_client._pubsub_task
        await asyncio.sleep(0.05)
        assert reconnect_called.is_set()

    async def test_reader_does_not_reconnect_without_subscriptions(self, connected_client, task_tracker):
        """No reconnect if there are no active subscriptions (clean shutdown)."""
        mock_ws = connected_client._ws
        mock_ws.__aiter__ = MagicMock(return_value=async_iter([]))

        reconnect_called = False

        async def mock_reconnect():
            nonlocal reconnect_called
            reconnect_called = True

        connected_client._reconnect_loop = mock_reconnect
        connected_client._pubsub_task = task_tracker.track(asyncio.create_task(connected_client._ws_reader()))
        await connected_client._pubsub_task
        await asyncio.sleep(0.05)
        assert not reconnect_called

    async def test_disconnect_handler_failure_is_logged_not_swallowed(self, connected_client, task_tracker):
        """Disconnect handlers that raise should log at WARNING, not silently pass."""
        mock_ws = connected_client._ws
        mock_ws.__aiter__ = MagicMock(return_value=async_iter([]))

        async def broken_handler():
            raise RuntimeError("handler bug")

        connected_client.on_disconnect(broken_handler)

        with patch("app.clients.pubsub_client.logger") as mock_logger:
            connected_client._pubsub_task = task_tracker.track(asyncio.create_task(connected_client._ws_reader()))
            await connected_client._pubsub_task
            mock_logger.error.assert_any_call(
                "[PUBSUB-CLIENT] Handler failed: %s",
                ANY,
                exc_info=True,
            )


@pytest.mark.asyncio
class TestResultDispatchDespiteMatchingEnvelopeId:
    """Regression: inbound messages must be dispatched to handlers even when
    the envelope `id` matches an id the client recently published.

    g8eo reuses the command's execution_id as the envelope `id` on the
    corresponding result message (the envelope `id` is
    treated as a correlation token on results). Previously, pubsub_client
    tracked outbound envelope ids in `_sent_ids` and dropped any inbound
    message whose id matched -- labelling it a "self-broadcast". g8ee
    publishes only to `cmd:*` and subscribes only to `results:*` / `heartbeat:*`
    (disjoint channels), so there is no real self-broadcast scenario on this
    wire path. The filter silently ate legitimate results, causing the
    awaiting Future on PubSubService to time out and the UI spinner to spin
    forever.

    These tests pin the correct behaviour: a MESSAGE / PMESSAGE event whose
    envelope `id` was just seen on `publish()` MUST still reach the handler.
    """

    @staticmethod
    def _make_binary_frame(event_type: int, channel: str, data: bytes, pattern: str = "") -> MagicMock:
        from app.proto.pubsub_pb2 import PubSubEvent
        event = PubSubEvent(type=event_type, channel=channel, data=data, pattern=pattern)
        msg = MagicMock()
        msg.type = aiohttp.WSMsgType.BINARY
        msg.data = event.SerializeToString()
        return msg

    async def test_result_with_matching_envelope_id_is_dispatched(self, connected_client):
        exec_id = "cmd_shared_correlation_id_1234"
        cmd_channel = "cmd:op-1:sess-1"
        results_channel = "results:op-1:sess-1"

        delivered = asyncio.Event()
        captured: list[tuple[str, bytes]] = []

        async def handler(channel: str, data: bytes) -> None:
            captured.append((channel, data))
            delivered.set()

        connected_client.on_channel_message(results_channel, handler)

        with patch.object(connected_client, "publish", return_value=1):
            await connected_client.publish(
                cmd_channel,
                b"some data",
            )

        result_data = b"result data"
        result_frame = self._make_binary_frame(
            PubSubWireEventType.MESSAGE.value,
            results_channel,
            result_data
        )
        mock_ws = connected_client._ws
        mock_ws.__aiter__ = MagicMock(return_value=async_iter([result_frame]))

        reader_task = asyncio.create_task(connected_client._ws_reader())
        try:
            await asyncio.wait_for(delivered.wait(), timeout=1.0)
        finally:
            reader_task.cancel()
            await asyncio.gather(reader_task, return_exceptions=True)

        assert len(captured) == 1
        channel, data = captured[0]
        assert channel == results_channel
        assert data == result_data

    async def test_pmessage_with_matching_envelope_id_is_dispatched(self, connected_client):
        exec_id = "cmd_shared_correlation_id_pmsg"
        cmd_channel = "cmd:op-2:sess-2"
        results_pattern = "results:*"
        results_channel = "results:op-2:sess-2"

        delivered = asyncio.Event()
        captured: list[tuple[str, str, bytes]] = []

        async def handler(pattern: str, channel: str, data: bytes) -> None:
            captured.append((pattern, channel, data))
            delivered.set()

        connected_client._pmessage_handlers.setdefault(results_pattern, []).append(handler)

        with patch.object(connected_client, "publish", return_value=1):
            await connected_client.publish(
                cmd_channel,
                b"some data",
            )

        result_data = b"pmsg data"
        pmsg_frame = self._make_binary_frame(
            PubSubWireEventType.PMESSAGE.value,
            results_channel,
            result_data,
            pattern=results_pattern
        )
        mock_ws = connected_client._ws
        mock_ws.__aiter__ = MagicMock(return_value=async_iter([pmsg_frame]))

        reader_task = asyncio.create_task(connected_client._ws_reader())
        try:
            await asyncio.wait_for(delivered.wait(), timeout=1.0)
        finally:
            reader_task.cancel()
            await asyncio.gather(reader_task, return_exceptions=True)

        assert len(captured) == 1
        pattern, channel, data = captured[0]
        assert pattern == results_pattern
        assert channel == results_channel
        assert data == result_data


@pytest.mark.asyncio
class TestWsReaderPerMessageErrorHandling:
    """The per-message exception handler in _ws_reader must be narrow.

    Shape errors (KeyError / ValueError / AttributeError) on a single frame
    should be logged and the reader should continue processing subsequent
    frames. Unexpected errors (e.g. a typo in dispatch code raising
    NameError / RuntimeError) MUST propagate so they are not silently eaten
    by a broad ``except Exception``.
    """

    @staticmethod
    def _binary_frame(event_type: int, channel: str = "", data: bytes = b"") -> MagicMock:
        from app.proto.pubsub_pb2 import PubSubEvent
        event = PubSubEvent(type=event_type, channel=channel, data=data)
        msg = MagicMock()
        msg.type = aiohttp.WSMsgType.BINARY
        msg.data = event.SerializeToString()
        return msg

    async def test_malformed_shape_is_logged_and_reader_continues(
        self, connected_client, task_tracker
    ):
        # We simulate a failure by making handler raise an exception
        bad_frame = MagicMock()
        bad_frame.type = aiohttp.WSMsgType.BINARY
        bad_frame.data = b"corrupt"

        delivered = asyncio.Event()

        async def handler(channel: str, data: bytes) -> None:
            delivered.set()

        connected_client.on_channel_message("results:ok", handler)
        good_frame = self._binary_frame(
            PubSubWireEventType.MESSAGE.value,
            channel="results:ok",
            data=b"ok"
        )

        mock_ws = connected_client._ws
        mock_ws.__aiter__ = MagicMock(return_value=async_iter([bad_frame, good_frame]))

        with patch("app.clients.pubsub_client.logger") as mock_logger:
            reader_task = task_tracker.track(
                asyncio.create_task(connected_client._ws_reader())
            )
            try:
                await asyncio.wait_for(delivered.wait(), timeout=1.0)
            finally:
                reader_task.cancel()
                await asyncio.gather(reader_task, return_exceptions=True)

            parse_failed = any(
                "Failed to parse binary protobuf message" in str(call.args[0])
                for call in mock_logger.error.call_args_list
                if call.args
            )
            assert parse_failed

    async def test_unexpected_exception_propagates(self, connected_client, task_tracker):
        """A RuntimeError from dispatch code (e.g. a typo surfacing as
        NameError at runtime) must NOT be swallowed by the per-message
        except block. It should propagate out of the reader so the task
        fails loudly and reconnect logic runs."""

        # Force a RuntimeError to surface during dispatch by making
        # _channel_handlers.get raise -- simulates a bug in handler code.
        broken_handlers = MagicMock()
        broken_handlers.get = MagicMock(side_effect=RuntimeError("dispatch typo"))
        broken_handlers.copy = MagicMock(return_value=broken_handlers)
        connected_client._channel_handlers = broken_handlers

        frame = self._binary_frame(
            PubSubWireEventType.MESSAGE.value,
            channel="results:op",
            data=b"x"
        )
        mock_ws = connected_client._ws
        mock_ws.__aiter__ = MagicMock(return_value=async_iter([frame]))

        reader_task = task_tracker.track(
            asyncio.create_task(connected_client._ws_reader())
        )
        with pytest.raises(RuntimeError, match="dispatch typo"):
            await asyncio.wait_for(reader_task, timeout=1.0)


@pytest.mark.asyncio
class TestReconnectLoop:
    async def test_reconnect_succeeds_on_first_try(self, connected_client):
        connected_client._subscribed_channels.add("test-channel")
        call_count = 0

        async def mock_ensure_ws():
            nonlocal call_count
            call_count += 1

        connected_client._ensure_ws = mock_ensure_ws
        await connected_client._reconnect_loop()
        assert call_count == 1

    async def test_reconnect_retries_with_backoff(self, connected_client):
        connected_client._subscribed_channels.add("test-channel")
        attempts = 0

        async def mock_ensure_ws():
            nonlocal attempts
            attempts += 1
            if attempts < 3:
                raise ConnectionError("operator down")

        delays = []
        original_sleep = asyncio.sleep

        async def mock_sleep(delay):
            delays.append(delay)

        connected_client._ensure_ws = mock_ensure_ws
        with patch("asyncio.sleep", side_effect=mock_sleep):
            await connected_client._reconnect_loop()

        assert attempts == 3
        assert delays[0] == 1.0
        assert delays[1] == 2.0

    async def test_reconnect_stops_when_no_subscriptions(self, connected_client):
        """If all subscriptions are removed while reconnecting, the loop exits."""
        attempts = 0

        async def mock_ensure_ws():
            nonlocal attempts
            attempts += 1
            connected_client._subscribed_channels.clear()
            raise ConnectionError("operator down")

        connected_client._subscribed_channels.add("test-channel")
        connected_client._ensure_ws = mock_ensure_ws

        with patch("asyncio.sleep", new_callable=AsyncMock):
            await connected_client._reconnect_loop()

        assert attempts == 1


@pytest.mark.asyncio
class TestPubSubClientCoverage:
    """Additional tests to address uncovered lines in pubsub_client.py."""

    async def test_ensure_ws_protocol_override(self, disconnected_client):
        """Test forcing WSS when ws:// is provided."""
        disconnected_client.pubsub_url = "ws://localhost:9001"
        mock_session = MagicMock(spec=aiohttp.ClientSession)
        mock_ws = AsyncMock(spec=aiohttp.ClientWebSocketResponse)
        mock_ws.closed = False
        
        # mock_session.ws_connect is called and returned value is awaited
        # so return_value must be an awaitable that returns a context manager
        # OR just mock the whole call chain.
        
        mock_cm = MagicMock()
        mock_cm.__aenter__ = AsyncMock(return_value=mock_ws)
        mock_cm.__aexit__ = AsyncMock()
        
        # await ws_session.ws_connect(...) -> returns mock_cm
        mock_session.ws_connect = AsyncMock(return_value=mock_cm)

        with patch.object(disconnected_client, "_get_http_ws_session", return_value=mock_session), \
             patch("app.clients.pubsub_client.resolve_pubsub_ssl_context"):
            await disconnected_client._ensure_ws()

        assert disconnected_client.pubsub_url == "ws://localhost:9001"
        args, kwargs = mock_session.ws_connect.call_args
        assert args[0].startswith("wss://")

    async def test_ensure_ws_uses_mtls_without_internal_token(self, disconnected_client):
        mock_session = MagicMock(spec=aiohttp.ClientSession)
        mock_ws = AsyncMock(spec=aiohttp.ClientWebSocketResponse)
        mock_ws.closed = False
        
        mock_cm = MagicMock()
        mock_cm.__aenter__ = AsyncMock(return_value=mock_ws)
        mock_cm.__aexit__ = AsyncMock()
        
        mock_session.ws_connect = AsyncMock(return_value=mock_cm)

        with patch.object(disconnected_client, "_get_http_ws_session", return_value=mock_session), \
             patch("app.clients.pubsub_client.resolve_pubsub_ssl_context"):
            await disconnected_client._ensure_ws()

        args, kwargs = mock_session.ws_connect.call_args
        assert "token=" not in args[0]
        assert kwargs["headers"] == {}

    async def test_ws_reader_unknown_event_type(self, connected_client, task_tracker):
        """Test logging of unknown wire event types."""
        from app.proto.pubsub_pb2 import PubSubEvent
        
        event = PubSubEvent(type="unknown_type") # Unknown type string
        frame = MagicMock()
        frame.type = aiohttp.WSMsgType.BINARY
        frame.data = event.SerializeToString()
        connected_client._ws.__aiter__ = MagicMock(return_value=async_iter([frame]))

        with patch("app.clients.pubsub_client.logger") as mock_logger:
            task = task_tracker.track(asyncio.create_task(connected_client._ws_reader()))
            await task
            mock_logger.warning.assert_any_call("[PUBSUB-CLIENT] Unknown wire event type '%s'", "unknown_type")

    async def test_ws_reader_malformed_proto(self, connected_client, task_tracker):
        """Test logging of malformed binary message."""
        frame = MagicMock()
        frame.type = aiohttp.WSMsgType.BINARY
        frame.data = b"invalid protobuf"
        connected_client._ws.__aiter__ = MagicMock(return_value=async_iter([frame]))

        with patch("app.clients.pubsub_client.logger") as mock_logger:
            task = task_tracker.track(asyncio.create_task(connected_client._ws_reader()))
            await task
            mock_logger.error.assert_any_call("[PUBSUB-CLIENT] Failed to parse binary protobuf message: %s", ANY)

    async def test_ws_reader_closed_type(self, connected_client, task_tracker):
        """Test reader exit on CLOSED message type."""
        frame = MagicMock()
        frame.type = aiohttp.WSMsgType.CLOSED
        connected_client._ws.__aiter__ = MagicMock(return_value=async_iter([frame]))

        task = task_tracker.track(asyncio.create_task(connected_client._ws_reader()))
        await task
        assert connected_client._ws is None

    async def test_subscribe_refcounting(self, connected_client, task_tracker):
        """Test channel subscription refcounting."""
        channel = "ref-channel"
        
        # First subscription
        async def mock_ack():
            await asyncio.sleep(0.01)
            if channel in connected_client._pending_acks:
                for ack in connected_client._pending_acks[channel]:
                    ack.set()
        
        task = task_tracker.track(asyncio.create_task(mock_ack()))
        await connected_client.subscribe(channel)
        await task
        assert connected_client._channel_refcounts[channel] == 1
        
        # Second subscription (should just increment refcount)
        await connected_client.subscribe(channel)
        assert connected_client._channel_refcounts[channel] == 2
        assert connected_client._ws.send_bytes.call_count == 1 # Only one wire call

        # Unsubscribe once
        await connected_client.unsubscribe(channel)
        assert connected_client._channel_refcounts[channel] == 1
        assert channel in connected_client._subscribed_channels
        
        # Unsubscribe twice
        await connected_client.unsubscribe(channel)
        assert connected_client._channel_refcounts[channel] == 0
        assert channel not in connected_client._subscribed_channels
        assert connected_client._ws.send_bytes.call_count == 2 # One SUBSCRIBE, one UNSUBSCRIBE

    async def test_publish_failure(self, connected_client):
        """Test publish failure logging."""
        connected_client._ws.send_bytes.side_effect = Exception("send failed")
        with patch("app.clients.pubsub_client.logger") as mock_logger:
            res = await connected_client.publish("chan", b"data")
            assert res == 0
            mock_logger.error.assert_any_call("[PUBSUB-CLIENT] publish failed for channel '%s': %s", "chan", ANY, exc_info=True)

    async def test_close_clears_state(self, connected_client):
        """Test close method clears internal state."""
        connected_client._subscribed_channels.add("chan")
        connected_client._channel_refcounts["chan"] = 1
        await connected_client.close()
        assert connected_client._closing is True
        assert connected_client._ws is None
        assert not connected_client._subscribed_channels
        assert not connected_client._channel_refcounts

    async def test_domain_methods(self, connected_client, task_tracker):
        """Test high-level domain methods: publish_command, subscribe_heartbeats, etc."""
        from app.models.pubsub_messages import G8eMessage
        from app.models.command_request_payloads import CommandRequestPayload
        from app.constants import EventType
        
        op_id = "op-1"
        sess_id = "sess-1"
        
        # publish_command
        msg = G8eMessage(
            id="msg-1",
            source_component=ComponentName.G8EE,
            event_type=EventType.OPERATOR_COMMAND_REQUESTED,
            operator_id=op_id,
            operator_session_id=sess_id,
            case_id="case-1",
            task_id="task-1",
            investigation_id="inv-1",
            web_session_id="web-1",
            payload=CommandRequestPayload(command="echo hi", execution_id="exec-1")
        )
        res = await connected_client.publish_command(op_id, sess_id, msg)
        assert res == 1
        
        # subscribe_execution_results
        async def mock_ack_results():
            await asyncio.sleep(0.01)
            channel = f"results:{op_id}:{sess_id}"
            if channel in connected_client._pending_acks:
                for ack in connected_client._pending_acks[channel]:
                    ack.set()
        
        task_res = task_tracker.track(asyncio.create_task(mock_ack_results()))
        callback_res = AsyncMock()
        await connected_client.subscribe_execution_results(op_id, sess_id, callback_res)
        await task_res
        channel_res = f"results:{op_id}:{sess_id}"
        assert channel_res in connected_client._subscribed_channels
        assert callback_res in connected_client._channel_handlers[channel_res]

        # unsubscribe_execution_results
        await connected_client.unsubscribe_execution_results(op_id, sess_id, callback_res)
        assert channel_res not in connected_client._subscribed_channels
        assert channel_res not in connected_client._channel_handlers

        # subscribe_heartbeats
        async def mock_ack():
            await asyncio.sleep(0.01)
            channel = f"heartbeat:{op_id}:{sess_id}"
            if channel in connected_client._pending_acks:
                for ack in connected_client._pending_acks[channel]:
                    ack.set()
        
        task = task_tracker.track(asyncio.create_task(mock_ack()))
        callback = AsyncMock()
        await connected_client.subscribe_heartbeats(op_id, sess_id, callback)
        await task
        channel = f"heartbeat:{op_id}:{sess_id}"
        assert channel in connected_client._subscribed_channels
        assert callback in connected_client._channel_handlers[channel]
        
        # check_operator_online
        connected_client._ws.send_bytes.reset_mock()
        
        with patch.object(connected_client, "publish", return_value=1):
            is_online = await connected_client.check_operator_online(op_id, sess_id)
            assert is_online is True

        # unsubscribe_heartbeats
        await connected_client.unsubscribe_heartbeats(op_id, sess_id, callback)
        assert channel not in connected_client._subscribed_channels
        assert channel not in connected_client._channel_handlers
