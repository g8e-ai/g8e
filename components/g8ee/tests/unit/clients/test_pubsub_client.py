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
from app.constants import ComponentName, PubSubAction, PubSubField, PubSubMessageType, PubSubWireEventType
from tests.fakes.async_helpers import async_iter

pytestmark = pytest.mark.unit

@pytest.fixture
def disconnected_client():
    client = PubSubClient(
        pubsub_url="wss://g8es:9001",
        component_name=ComponentName.G8EE,
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
            pubsub_url="wss://g8es:9001/",
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

    def test_wire_protocol_values_match_g8es_go_constants(self):
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

        connected_client._ws.send_json.assert_called_with({
            PubSubField.ACTION: PubSubAction.SUBSCRIBE,
            PubSubField.CHANNEL: channel,
        })

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

        connected_client._ws.send_json.assert_called_with({
            PubSubField.ACTION: PubSubAction.PSUBSCRIBE,
            PubSubField.CHANNEL: pattern,
        })

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
    corresponding result message (shared/models/wire/envelope.json `id` is
    treated as a correlation token on results). Previously, pubsub_client
    tracked outbound envelope ids in `_sent_ids` and dropped any inbound
    message whose id matched -- labelling it a "self-broadcast". g8ee
    publishes only to `cmd:*` and subscribes only to `results:*` / `heartbeat:*`
    (disjoint channels), so there is no real self-broadcast scenario on this
    wire path. The filter silently ate legitimate results, causing
    `execution_registry.wait()` to time out and the UI spinner to spin forever.

    These tests pin the correct behaviour: a MESSAGE / PMESSAGE event whose
    envelope `id` was just seen on `publish()` MUST still reach the handler.
    """

    @staticmethod
    def _make_text_frame(event: dict) -> MagicMock:
        msg = MagicMock()
        msg.type = aiohttp.WSMsgType.TEXT
        msg.data = json.dumps(event)
        return msg

    async def test_result_with_matching_envelope_id_is_dispatched(self, connected_client):
        exec_id = "cmd_shared_correlation_id_1234"
        cmd_channel = "cmd:op-1:sess-1"
        results_channel = "results:op-1:sess-1"

        delivered = asyncio.Event()
        captured: list[tuple[str, object]] = []

        async def handler(channel: str, data: object) -> None:
            captured.append((channel, data))
            delivered.set()

        connected_client.on_channel_message(results_channel, handler)

        await connected_client.publish(
            cmd_channel,
            {"id": exec_id, "event_type": "g8e.v1.operator.command.requested"},
        )

        result_frame = self._make_text_frame(
            {
                "type": PubSubWireEventType.MESSAGE.value,
                "channel": results_channel,
                "data": {
                    "id": exec_id,
                    "event_type": "g8e.v1.operator.command.completed",
                    "payload": {"execution_id": exec_id, "status": "completed"},
                },
            }
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
        assert isinstance(data, dict)
        assert data["id"] == exec_id
        assert data["payload"]["execution_id"] == exec_id

    async def test_pmessage_with_matching_envelope_id_is_dispatched(self, connected_client):
        exec_id = "cmd_shared_correlation_id_pmsg"
        cmd_channel = "cmd:op-2:sess-2"
        results_pattern = "results:*"
        results_channel = "results:op-2:sess-2"

        delivered = asyncio.Event()
        captured: list[tuple[str, str, object]] = []

        async def handler(pattern: str, channel: str, data: object) -> None:
            captured.append((pattern, channel, data))
            delivered.set()

        connected_client._pmessage_handlers.setdefault(results_pattern, []).append(handler)

        await connected_client.publish(
            cmd_channel,
            {"id": exec_id, "event_type": "g8e.v1.operator.command.requested"},
        )

        pmsg_frame = self._make_text_frame(
            {
                "type": PubSubWireEventType.PMESSAGE.value,
                "pattern": results_pattern,
                "channel": results_channel,
                "data": {
                    "id": exec_id,
                    "event_type": "g8e.v1.operator.command.completed",
                    "payload": {"execution_id": exec_id, "status": "completed"},
                },
            }
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
        assert isinstance(data, dict)
        assert data["id"] == exec_id


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
    def _text_frame(payload) -> MagicMock:
        msg = MagicMock()
        msg.type = aiohttp.WSMsgType.TEXT
        msg.data = json.dumps(payload)
        return msg

    async def test_malformed_shape_is_logged_and_reader_continues(
        self, connected_client, task_tracker
    ):
        # json.loads of a list is valid JSON but not a dict, so .get() will
        # raise AttributeError inside the per-message try -- it should be
        # caught and logged, then the reader proceeds to the next frame.
        bad_frame = self._text_frame(["not", "a", "dict"])

        delivered = asyncio.Event()

        async def handler(channel: str, data: object) -> None:
            delivered.set()

        connected_client.on_channel_message("results:ok", handler)
        good_frame = self._text_frame(
            {
                "type": PubSubWireEventType.MESSAGE.value,
                "channel": "results:ok",
                "data": {"id": "x"},
            }
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

            shape_logged = any(
                "Malformed event shape" in str(call.args[0])
                for call in mock_logger.error.call_args_list
                if call.args
            )
            assert shape_logged, (
                "Shape errors (AttributeError on non-dict frame) must be logged"
            )

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

        frame = self._text_frame(
            {
                "type": PubSubWireEventType.MESSAGE.value,
                "channel": "results:op",
                "data": {"id": "x"},
            }
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
                raise ConnectionError("g8es down")

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
            raise ConnectionError("g8es down")

        connected_client._subscribed_channels.add("test-channel")
        connected_client._ensure_ws = mock_ensure_ws

        with patch("asyncio.sleep", new_callable=AsyncMock):
            await connected_client._reconnect_loop()

        assert attempts == 1
