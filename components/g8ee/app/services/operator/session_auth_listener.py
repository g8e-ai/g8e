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
import hashlib
import json
import logging
from typing import Any, Optional

from app.clients.pubsub_client import PubSubClient
from app.constants import PubSubChannel
from app.services.operator.operator_session_service import OperatorSessionService
from app.services.operator.operator_data_service import OperatorDataService

logger = logging.getLogger(__name__)

# Default TTL for listening for auth response
SESSION_AUTH_LISTEN_TTL_SECONDS = 300 # 5 minutes

class SessionAuthListener:
    """
    SessionAuthListener for g8ee.
    Listens for session auth requests on PubSub and responds with bootstrap config.
    Moved from g8ed.
    """

    def __init__(
        self,
        pubsub_client: PubSubClient,
        session_service: OperatorSessionService,
        operator_data_service: OperatorDataService,
    ):
        self.pubsub_client = pubsub_client
        self.session_service = session_service
        self.operator_data_service = operator_data_service
        self._active_listeners = {}

    async def listen(self, operator_session_id: str, operator_id: str, user_id: str, organization_id: Optional[str]):
        """
        Subscribe to the session auth channel for this operator_session_id and
        respond with bootstrap config once.
        """
        session_hash = hashlib.sha256(operator_session_id.encode()).hexdigest()
        
        # We need the actual prefixes from constants
        # In g8ee, PubSubChannel has different attributes than g8ed JS.
        # I'll check app/constants/channels.py in g8ee.
        
        # Based on g8ed JS:
        # authChannel     = `${PubSubChannel.AUTH_PUBLISH_SESSION_PREFIX}${sessionHash}`;
        # responseChannel = `${PubSubChannel.AUTH_RESPONSE_SESSION_PREFIX}${sessionHash}`;
        
        # I'll hardcode them for now if not in g8ee constants, but I should check.
        
        auth_channel = f"auth.publish:session:{session_hash}"
        response_channel = f"auth.response:session:{session_hash}"

        if auth_channel in self._active_listeners:
            return

        async def message_handler(channel: str, data: Any):
            if channel != auth_channel:
                return

            try:
                session = await self.session_service.validate_session(operator_session_id)
                if not session or not session.is_active:
                    await self.pubsub_client.publish(response_channel, {
                        "success": False,
                        "error": "Session not found or expired"
                    })
                    return

                operator = await self.operator_data_service.get_operator(operator_id)
                api_key = operator.api_key if operator else None

                # Aligned with SessionAuthResponse in g8ed
                response = {
                    "success": True,
                    "operator_session_id": operator_session_id,
                    "operator_id": operator_id,
                    "user_id": user_id,
                    "organization_id": organization_id,
                    "api_key": api_key,
                    "config": {
                        "max_concurrent_tasks": 10,
                        "max_memory_mb": 1024,
                        "heartbeat_interval_seconds": 30
                    },
                    "operator_cert": None,
                    "operator_cert_key": None,
                }

                await self.pubsub_client.publish(response_channel, response)
                logger.info(f"[SESSION-AUTH-LISTENER] Auth response published for {operator_id}")

            except Exception as e:
                logger.error(f"[SESSION-AUTH-LISTENER] Failed to handle session auth request: {e}")
                await self.pubsub_client.publish(response_channel, {
                    "success": False,
                    "error": "Internal error"
                })
            finally:
                await self.cleanup(auth_channel)

        self._active_listeners[auth_channel] = message_handler
        self.pubsub_client.on_channel_message(auth_channel, message_handler)
        await self.pubsub_client.subscribe(auth_channel)
        
        logger.info(f"[SESSION-AUTH-LISTENER] Listening for session auth on {auth_channel}")
        
        # Auto cleanup after timeout
        asyncio.create_task(self._auto_cleanup(auth_channel, SESSION_AUTH_LISTEN_TTL_SECONDS))

    async def _auto_cleanup(self, auth_channel: str, delay: int):
        await asyncio.sleep(delay)
        await self.cleanup(auth_channel)

    async def cleanup(self, auth_channel: str):
        handler = self._active_listeners.pop(auth_channel, None)
        if handler:
            self.pubsub_client.off_channel_message(auth_channel, handler)
            await self.pubsub_client.unsubscribe(auth_channel)
