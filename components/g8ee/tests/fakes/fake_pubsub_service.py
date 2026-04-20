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

"""Typed fake for PubSubServiceProtocol."""

from typing import Callable, Coroutine
from app.models.pubsub_messages import G8eMessage, G8eoResultEnvelope
from app.services.protocols import PubSubServiceProtocol


class FakePubSubService:
    """Typed fake implementing PubSubServiceProtocol.

    Records all calls for assertion in tests. Does not perform any real I/O.
    wait_for_result returns True (result ready) by default.
    """

    def __init__(self, *, wait_result: bool = True) -> None:
        self._wait_result = wait_result
        self._ready = False
        self.started = False
        self.stopped = False
        self.registered_sessions: list[tuple[str, str]] = []
        self.deregistered_sessions: list[tuple[str, str]] = []
        self.published_commands: list[G8eMessage] = []
        self.pubsub_client: object | None = None
        self._result_handlers: list[Callable[[G8eoResultEnvelope], Coroutine[object, object, None]]] = []

    def subscribe_results(self, handler: Callable[[G8eoResultEnvelope], Coroutine[object, object, None]]) -> None:
        if handler not in self._result_handlers:
            self._result_handlers.append(handler)

    def set_result_handler(self, handler: Callable[[G8eoResultEnvelope], Coroutine[object, object, None]]) -> None:
        self._result_handlers = [handler]

    @property
    def is_ready(self) -> bool:
        return self._ready

    def set_pubsub_client(self, client: object) -> None:
        self._pubsub_client = client

    def _install_msg_capture(self) -> list[G8eMessage]:
        """Capture all published commands for verification in tests."""
        return self.published_commands

    async def start(self) -> None:
        self._ready = True
        self.started = True

    async def stop(self) -> None:
        self._ready = False
        self.stopped = True

    async def wait_for_result(self, execution_id: str, timeout: float) -> bool:
        return self._wait_result

    async def register_operator_session(
        self, operator_id: str, operator_session_id: str
    ) -> None:
        self.registered_sessions.append((operator_id, operator_session_id))

    async def deregister_operator_session(
        self, operator_id: str, operator_session_id: str
    ) -> None:
        self.deregistered_sessions.append((operator_id, operator_session_id))

    async def publish_command(
        self,
        operator_id: str,
        operator_session_id: str,
        command_data: G8eMessage,
    ) -> int:
        self.published_commands.append(command_data)
        return 1


_: PubSubServiceProtocol = FakePubSubService()
