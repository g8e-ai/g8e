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

"""
PubSubClient — WebSocket-based Pub/Sub client for VSODB.

Talks to the Operator in --listen mode via WebSocket.
Supports: subscribe, psubscribe, publish,
publish_command, subscribe_execution_results, subscribe_heartbeats, check_operator_online.
"""

import asyncio
import json
import logging
from collections.abc import Callable, Coroutine
from typing import Any

import aiohttp

from app.models.settings import ListenSettings
from app.utils.aiohttp_session import new_pubsub_ws_session, resolve_pubsub_ssl_context
from app.constants import (
    ComponentName,
    EventType,
    PubSubAction,
    PubSubChannel,
    PubSubField,
    PubSubWireEventType,
)
from app.models.pubsub_messages import VSOMessage

logger = logging.getLogger(__name__)


class PubSubClient:
    """
    Async WebSocket client for VSODB pub/sub.
    """

    def __init__(
        self,
        pubsub_url: str | None = None,
        component_name: ComponentName = ComponentName.G8EE,
        timeout: float = 10.0,
        ca_cert_path: str | None = None,
        internal_auth_token: str | None = None,
    ):
        _settings = ListenSettings()
        self.pubsub_url = (pubsub_url or _settings.pubsub_url).rstrip("/")
        self.component_name = component_name
        self._timeout = timeout
        self._ca_cert_path = ca_cert_path
        self._internal_auth_token = internal_auth_token
        self._ws_session: aiohttp.ClientSession | None = None
        self._ws: aiohttp.ClientWebSocketResponse | None = None
        
        # Pub/sub state
        self._message_handlers: list = []
        self._channel_handlers: dict[str, list] = {}
        self._pmessage_handlers: dict[str, list] = {}
        self._pubsub_task: asyncio.Task | None = None
        self._subscribed_patterns: set[str] = set()
        self._subscribed_channels: set[str] = set()
        self._disconnect_handlers: list[Callable] = []
        self._ack_events: dict[str, asyncio.Event] = {}

    async def _get_http_ws_session(self) -> aiohttp.ClientSession:
        """Carrier session for WebSocket pub/sub."""
        self._ws_session = new_pubsub_ws_session(
            self._ws_session,
            timeout=aiohttp.ClientTimeout(total=self._timeout),
        )
        return self._ws_session

    async def _ensure_ws(self):
        if self._ws and not self._ws.closed:
            return

        ws_url = self.pubsub_url + "/ws/pubsub"
        if not ws_url.startswith("wss://"):
            logger.warning("[PUBSUB-CLIENT] Protocol override: forcing WSS for %s", self.pubsub_url)
            ws_url = ws_url.replace("ws://", "wss://", 1)
            if not ws_url.startswith("wss://"):
                ws_url = "wss://" + ws_url.split("://")[-1]

        if self._internal_auth_token:
            ws_url += f"?token={self._internal_auth_token}"

        ssl_ctx = resolve_pubsub_ssl_context(
            self._ca_cert_path,
            use_tls=True
        )

        ws_session = await self._get_http_ws_session()
        
        headers = {}
        if self._internal_auth_token:
            headers["X-Internal-Auth"] = self._internal_auth_token

        self._ws = await ws_session.ws_connect(ws_url, ssl=ssl_ctx, headers=headers)
        logger.info("[PUBSUB-CLIENT] Pub/sub WebSocket connected")

        if self._pubsub_task is None or self._pubsub_task.done():
            self._pubsub_task = asyncio.create_task(self._ws_reader())
            await asyncio.sleep(0)

        for channel in list(self._subscribed_channels):
            await self._ws.send_json({PubSubField.ACTION: PubSubAction.SUBSCRIBE, PubSubField.CHANNEL: channel})
        for pattern in list(self._subscribed_patterns):
            await self._ws.send_json({PubSubField.ACTION: PubSubAction.PSUBSCRIBE, PubSubField.CHANNEL: pattern})
        if self._subscribed_channels or self._subscribed_patterns:
            logger.info(
                f"[PUBSUB-CLIENT] Re-registered {len(self._subscribed_channels)} channels "
                f"and {len(self._subscribed_patterns)} patterns after reconnect"
            )

    async def _ws_reader(self):
        try:
            assert self._ws is not None
            async for msg in self._ws:
                if msg.type == aiohttp.WSMsgType.TEXT:
                    try:
                        event = json.loads(msg.data)
                        event_type = event.get(PubSubField.TYPE)
                        
                        if event_type == PubSubWireEventType.MESSAGE:
                            channel = event[PubSubField.CHANNEL]
                            data = event[PubSubField.DATA]
                            
                            logger.debug(
                                "[PUBSUB-CLIENT] Received MESSAGE on channel '%s'",
                                channel,
                                extra={
                                    "event_type": event_type,
                                    "channel": channel,
                                    "data_type": type(data).__name__ if data else None,
                                    "data_keys": list(data.keys()) if isinstance(data, dict) else None,
                                }
                            )
                            
                            # Call channel-specific handlers
                            channel_handlers = self._channel_handlers.get(channel, [])
                            if channel_handlers:
                                logger.debug(
                                    "[PUBSUB-CLIENT] Dispatching to %d channel handlers for '%s'",
                                    len(channel_handlers),
                                    channel,
                                    extra={"channel": channel, "handler_count": len(channel_handlers)}
                                )
                                for handler in channel_handlers:
                                    await handler(channel, data)
                            
                            # Call general message handlers
                            if self._message_handlers:
                                logger.debug(
                                    "[PUBSUB-CLIENT] Dispatching to %d general message handlers for '%s'",
                                    len(self._message_handlers),
                                    channel,
                                    extra={"channel": channel, "handler_count": len(self._message_handlers)}
                                )
                                for handler in self._message_handlers:
                                    await handler(channel, data)
                                    
                        elif event_type == PubSubWireEventType.PMESSAGE:
                            pattern = event.get(PubSubField.PATTERN, "")
                            channel = event[PubSubField.CHANNEL]
                            data = event[PubSubField.DATA]
                            
                            logger.debug(
                                "[PUBSUB-CLIENT] Received PMESSAGE for pattern '%s' on channel '%s'",
                                pattern,
                                channel,
                                extra={
                                    "event_type": event_type,
                                    "pattern": pattern,
                                    "channel": channel,
                                    "data_type": type(data).__name__ if data else None,
                                    "data_keys": list(data.keys()) if isinstance(data, dict) else None,
                                }
                            )
                            
                            pattern_handlers = self._pmessage_handlers.get(pattern, [])
                            if pattern_handlers:
                                logger.debug(
                                    "[PUBSUB-CLIENT] Dispatching to %d pattern handlers for '%s'",
                                    len(pattern_handlers),
                                    pattern,
                                    extra={"pattern": pattern, "handler_count": len(pattern_handlers)}
                                )
                                for handler in pattern_handlers:
                                    await handler(pattern, channel, data)
                                    
                        elif event_type == PubSubWireEventType.SUBSCRIBED:
                            channel = event.get(PubSubField.CHANNEL, "")
                            ack_event = self._ack_events.get(channel)
                            
                            logger.debug(
                                "[PUBSUB-CLIENT] Received SUBSCRIBED acknowledgment for channel '%s'",
                                channel,
                                extra={
                                    "event_type": event_type,
                                    "channel": channel,
                                    "has_pending_ack": ack_event is not None,
                                }
                            )
                            
                            if ack_event:
                                ack_event.set()
                            else:
                                logger.warning(
                                    "[PUBSUB-CLIENT] Received SUBSCRIBED for unknown channel '%s'",
                                    channel,
                                    extra={"channel": channel}
                                )
                        else:
                            logger.warning(
                                "[PUBSUB-CLIENT] Unknown event type '%s' received: %r",
                                event_type,
                                msg.data[:200],
                                extra={"event_type": event_type}
                            )
                            
                    except json.JSONDecodeError:
                        logger.error("[PUBSUB-CLIENT] Malformed JSON from pub/sub: %r", msg.data[:200])
                    except KeyError as exc:
                        logger.error("[PUBSUB-CLIENT] Missing field in pub/sub message: %s - %r", exc, msg.data[:200])
                        
                elif msg.type in (aiohttp.WSMsgType.CLOSED, aiohttp.WSMsgType.ERROR):
                    logger.info("[PUBSUB-CLIENT] WebSocket message type: %s", msg.type)
                    break
        except Exception as e:
            logger.error("[PUBSUB-CLIENT] WS reader crashed: %s", e, exc_info=True)
        finally:
            self._ws = None
            logger.warning("[PUBSUB-CLIENT] Pub/sub WebSocket disconnected")
            for handler in list(self._disconnect_handlers):
                try:
                    await handler()
                except Exception as exc:
                    logger.warning("[PUBSUB-CLIENT] Disconnect handler failed: %s", exc)

            # Trigger reconnection if there are active subscriptions
            if self._subscribed_channels or self._subscribed_patterns:
                logger.info("[PUBSUB-CLIENT] Scheduling reconnection due to active subscriptions")
                asyncio.create_task(self._reconnect_loop())

    async def _reconnect_loop(self):
        """Exponential backoff reconnection loop."""
        delay = 1.0
        max_delay = 60.0
        while self._subscribed_channels or self._subscribed_patterns:
            try:
                await self._ensure_ws()
                logger.info("[PUBSUB-CLIENT] Reconnection successful")
                return
            except Exception as e:
                logger.warning("[PUBSUB-CLIENT] Reconnection failed, retrying in %.1fs: %s", delay, e)
                await asyncio.sleep(delay)
                delay = min(delay * 2, max_delay)

    async def connect(self) -> bool:
        """Verify connectivity to the VSODB pub/sub service."""
        try:
            await self._ensure_ws()
            return True
        except Exception as e:
            logger.error(f"[PUBSUB-CLIENT] Connection failed: {e}")
            return False

    async def close(self):
        if self._pubsub_task and not self._pubsub_task.done():
            self._pubsub_task.cancel()
        if self._ws and not self._ws.closed:
            await self._ws.close()
        if self._ws_session and not self._ws_session.closed:
            await self._ws_session.close()

    async def ensure_connected(self) -> None:
        """Ensure the WebSocket pub/sub connection is open. Public entry point for service layer."""
        await self._ensure_ws()

    def on_disconnect(self, handler: Callable[[], Coroutine[object, object, None]]) -> None:
        """Register a coroutine to call when the WebSocket disconnects."""
        if handler not in self._disconnect_handlers:
            self._disconnect_handlers.append(handler)

    def off_disconnect(self, handler: Callable[[], Coroutine[object, object, None]]) -> None:
        """Remove a previously registered disconnect handler."""
        self._disconnect_handlers = [h for h in self._disconnect_handlers if h is not handler]

    async def subscribe(self, channel: str):
        logger.info(
            "[PUBSUB-CLIENT] Subscribing to channel '%s'",
            channel,
            extra={"channel": channel}
        )
        
        await self._ensure_ws()
        assert self._ws is not None
        ack_event = asyncio.Event()
        self._ack_events[channel] = ack_event
        self._subscribed_channels.add(channel)
        
        await self._ws.send_json({PubSubField.ACTION: PubSubAction.SUBSCRIBE, PubSubField.CHANNEL: channel})
        
        try:
            await asyncio.wait_for(ack_event.wait(), timeout=5.0)
            logger.info(
                "[PUBSUB-CLIENT] Successfully subscribed to channel '%s'",
                channel,
                extra={"channel": channel}
            )
        except asyncio.TimeoutError:
            logger.error(
                "[PUBSUB-CLIENT] Timeout waiting for subscription acknowledgment for channel '%s'",
                channel,
                extra={"channel": channel}
            )
            raise
        finally:
            self._ack_events.pop(channel, None)

    async def psubscribe(self, pattern: str):
        logger.info(
            "[PUBSUB-CLIENT] Subscribing to pattern '%s'",
            pattern,
            extra={"pattern": pattern}
        )
        
        await self._ensure_ws()
        assert self._ws is not None
        ack_event = asyncio.Event()
        self._ack_events[pattern] = ack_event
        self._subscribed_patterns.add(pattern)
        
        await self._ws.send_json({PubSubField.ACTION: PubSubAction.PSUBSCRIBE, PubSubField.CHANNEL: pattern})
        
        try:
            await asyncio.wait_for(ack_event.wait(), timeout=5.0)
            logger.info(
                "[PUBSUB-CLIENT] Successfully subscribed to pattern '%s'",
                pattern,
                extra={"pattern": pattern}
            )
        except asyncio.TimeoutError:
            logger.error(
                "[PUBSUB-CLIENT] Timeout waiting for pattern subscription acknowledgment for '%s'",
                pattern,
                extra={"pattern": pattern}
            )
            raise
        finally:
            self._ack_events.pop(pattern, None)

    async def unsubscribe(self, channel: str):
        logger.info(
            "[PUBSUB-CLIENT] Unsubscribing from channel '%s'",
            channel,
            extra={"channel": channel}
        )
        
        self._subscribed_channels.discard(channel)
        if self._ws and not self._ws.closed:
            await self._ws.send_json({PubSubField.ACTION: PubSubAction.UNSUBSCRIBE, PubSubField.CHANNEL: channel})
            logger.debug(
                "[PUBSUB-CLIENT] Unsubscribe message sent for channel '%s'",
                channel,
                extra={"channel": channel}
            )
        else:
            logger.warning(
                "[PUBSUB-CLIENT] Cannot unsubscribe from channel '%s': WebSocket not connected",
                channel,
                extra={"channel": channel}
            )

    async def punsubscribe(self, pattern: str):
        logger.info(
            "[PUBSUB-CLIENT] Unsubscribing from pattern '%s'",
            pattern,
            extra={"pattern": pattern}
        )
        
        self._subscribed_patterns.discard(pattern)
        if self._ws and not self._ws.closed:
            await self._ws.send_json({PubSubField.ACTION: PubSubAction.UNSUBSCRIBE, PubSubField.CHANNEL: pattern})
            logger.debug(
                "[PUBSUB-CLIENT] Punsubscribe message sent for pattern '%s'",
                pattern,
                extra={"pattern": pattern}
            )
        else:
            logger.warning(
                "[PUBSUB-CLIENT] Cannot punsubscribe from pattern '%s': WebSocket not connected",
                pattern,
                extra={"pattern": pattern}
            )

    async def publish(self, channel: str, data: dict[str, object]) -> int:
        """Publish a message over the shared WebSocket connection."""
        try:
            await self._ensure_ws()
            if self._ws is None or self._ws.closed:
                logger.error("[PUBSUB-CLIENT] Cannot publish: WebSocket is not connected")
                return 0

            logger.info(
                "[PUBSUB-CLIENT] Publishing message to channel '%s'",
                channel,
                extra={
                    "channel": channel,
                    "data_type": type(data).__name__,
                    "data_keys": list(data.keys()) if isinstance(data, dict) else None,
                }
            )

            await self._ws.send_json({
                PubSubField.ACTION: PubSubAction.PUBLISH,
                PubSubField.CHANNEL: channel,
                PubSubField.DATA: data,
            })
            
            logger.debug(
                "[PUBSUB-CLIENT] Message published successfully to channel '%s'",
                channel,
                extra={"channel": channel}
            )
            return 1
        except Exception as e:
            logger.error(
                "[PUBSUB-CLIENT] publish failed for channel '%s': %s",
                channel,
                e,
                extra={"channel": channel},
                exc_info=True
            )
            return 0

    def on_message(self, handler):
        self._message_handlers.append(handler)

    def on_channel_message(
        self,
        channel: str,
        handler: Callable[[str, str | dict[str, object]], Coroutine[object, object, None]],
    ) -> None:
        """Register a handler for a specific channel only."""
        self._channel_handlers.setdefault(channel, []).append(handler)

    def off_channel_message(
        self,
        channel: str,
        handler: Callable[[str, str | dict[str, object]], Coroutine[object, object, None]],
    ) -> None:
        """Remove a per-channel handler."""
        handlers = self._channel_handlers.get(channel)
        if handlers and handler in handlers:
            handlers.remove(handler)
            if not handlers:
                del self._channel_handlers[channel]

    def on_pmessage(self, pattern: str, handler):
        self._pmessage_handlers.setdefault(pattern, []).append(handler)

    # =========================================================================
    # Domain pub/sub — VSA Operator command/result/heartbeat channels
    # =========================================================================

    async def publish_command(
        self,
        operator_id: str,
        operator_session_id: str,
        command_data: VSOMessage
    ) -> int:
        channel = PubSubChannel.cmd(operator_id, operator_session_id)
        
        logger.info(
            "[PUBSUB-CLIENT] Publishing command for operator %s session %s (event_type: %s)",
            operator_id,
            operator_session_id,
            command_data.event_type,
            extra={
                "operator_id": operator_id,
                "operator_session_id": operator_session_id,
                "event_type": command_data.event_type,
                "channel": channel,
                "source_component": command_data.source_component,
            }
        )
        
        result = await self.publish(channel, command_data.flatten_for_wire())
        
        if result > 0:
            logger.debug(
                "[PUBSUB-CLIENT] Command published successfully for operator %s session %s",
                operator_id,
                operator_session_id,
                extra={
                    "operator_id": operator_id,
                    "operator_session_id": operator_session_id,
                    "event_type": command_data.event_type,
                }
            )
        else:
            logger.warning(
                "[PUBSUB-CLIENT] Failed to publish command for operator %s session %s",
                operator_id,
                operator_session_id,
                extra={
                    "operator_id": operator_id,
                    "operator_session_id": operator_session_id,
                    "event_type": command_data.event_type,
                }
            )
        
        return result

    async def subscribe_execution_results(
        self,
        operator_id: str,
        operator_session_id: str,
        callback: Callable[[str, Any], Any],
    ) -> None:
        """Subscribe to the exact results channel for one operator session."""
        channel = PubSubChannel.results(operator_id, operator_session_id)
        
        logger.info(
            "[PUBSUB-CLIENT] Subscribing to execution results for operator %s session %s",
            operator_id,
            operator_session_id,
            extra={
                "operator_id": operator_id,
                "operator_session_id": operator_session_id,
                "channel": channel,
                "subscription_type": "execution_results",
            }
        )
        
        self.on_channel_message(channel, callback)
        await self.subscribe(channel)

    async def unsubscribe_execution_results(
        self,
        operator_id: str,
        operator_session_id: str,
        callback: Callable[[str, Any], Any],
    ) -> None:
        """Unsubscribe from the exact results channel for one operator session."""
        channel = PubSubChannel.results(operator_id, operator_session_id)
        
        logger.info(
            "[PUBSUB-CLIENT] Unsubscribing from execution results for operator %s session %s",
            operator_id,
            operator_session_id,
            extra={
                "operator_id": operator_id,
                "operator_session_id": operator_session_id,
                "channel": channel,
                "subscription_type": "execution_results",
            }
        )
        
        await self.unsubscribe(channel)
        self.off_channel_message(channel, callback)

    async def subscribe_heartbeats(
        self,
        operator_id: str,
        operator_session_id: str,
        callback: Callable[[str, Any], Any],
    ) -> None:
        """Subscribe to the exact heartbeat channel for one operator session."""
        channel = PubSubChannel.heartbeat(operator_id, operator_session_id)
        
        logger.info(
            "[PUBSUB-CLIENT] Subscribing to heartbeats for operator %s session %s",
            operator_id,
            operator_session_id,
            extra={
                "operator_id": operator_id,
                "operator_session_id": operator_session_id,
                "channel": channel,
                "subscription_type": "heartbeats",
            }
        )
        
        self.on_channel_message(channel, callback)
        await self.subscribe(channel)

    async def unsubscribe_heartbeats(
        self,
        operator_id: str,
        operator_session_id: str,
        callback: Callable[[str, Any], Any],
    ) -> None:
        """Unsubscribe from the exact heartbeat channel for one operator session."""
        channel = PubSubChannel.heartbeat(operator_id, operator_session_id)
        
        logger.info(
            "[PUBSUB-CLIENT] Unsubscribing from heartbeats for operator %s session %s",
            operator_id,
            operator_session_id,
            extra={
                "operator_id": operator_id,
                "operator_session_id": operator_session_id,
                "channel": channel,
                "subscription_type": "heartbeats",
            }
        )
        
        await self.unsubscribe(channel)
        self.off_channel_message(channel, callback)

    async def check_operator_online(
        self,
        operator_id: str,
        operator_session_id: str
    ) -> bool:
        channel = PubSubChannel.cmd(operator_id, operator_session_id)
        
        logger.debug(
            "[PUBSUB-CLIENT] Checking if operator %s session %s is online",
            operator_id,
            operator_session_id,
            extra={
                "operator_id": operator_id,
                "operator_session_id": operator_session_id,
                "channel": channel,
            }
        )
        
        try:
            ping = VSOMessage(
                source_component=self.component_name,
                event_type=EventType.OPERATOR_HEARTBEAT_REQUESTED,
                operator_id=operator_id,
                operator_session_id=operator_session_id,
            )
            receivers = await self.publish(channel, ping.flatten_for_wire())
            
            is_online = receivers > 0
            logger.debug(
                "[PUBSUB-CLIENT] Operator %s session %s online check result: %s (receivers: %d)",
                operator_id,
                operator_session_id,
                "online" if is_online else "offline",
                receivers,
                extra={
                    "operator_id": operator_id,
                    "operator_session_id": operator_session_id,
                    "is_online": is_online,
                    "receivers": receivers,
                }
            )
            
            return is_online
        except Exception as e:
            logger.warning(
                "[PUBSUB-CLIENT] Failed to check if operator %s session %s is online: %s",
                operator_id,
                operator_session_id,
                e,
                extra={
                    "operator_id": operator_id,
                    "operator_session_id": operator_session_id,
                },
                exc_info=True
            )
            return False
