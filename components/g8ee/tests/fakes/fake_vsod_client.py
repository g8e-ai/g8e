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

"""Typed fake for VSODClientProtocol."""

from app.models.events import BackgroundEvent, SessionEvent
from app.models.http_context import VSOHttpContext
from app.models.vsod_client import IntentOperationResult
from app.services.protocols import VSODClientProtocol


class FakeVSODClient:
    """Typed fake implementing VSODClientProtocol.

    Records all calls for assertion in tests. Does not perform any real I/O.
    """

    def __init__(self) -> None:
        self.granted: list[dict] = []
        self.revoked: list[dict] = []
        self.pushed_events: list[SessionEvent | BackgroundEvent] = []
        self.bound_operators: list[dict] = []
        self.push_sse_event_return_value = True

    async def push_sse_event(self, event: SessionEvent | BackgroundEvent) -> bool:
        self.pushed_events.append(event)
        return self.push_sse_event_return_value

    def set_push_sse_event_return_value(self, value: bool) -> None:
        self.push_sse_event_return_value = value

    def assert_push_sse_event_called_with(self, event: SessionEvent | BackgroundEvent) -> None:
        assert event in self.pushed_events

    async def grant_intent(
        self, operator_id: str, intent: str, context: VSOHttpContext
    ) -> IntentOperationResult:
        self.granted.append({"operator_id": operator_id, "intent": intent, "context": context})
        return IntentOperationResult(success=True)

    async def revoke_intent(
        self, operator_id: str, intent: str, context: VSOHttpContext
    ) -> IntentOperationResult:
        self.revoked.append({"operator_id": operator_id, "intent": intent, "context": context})
        return IntentOperationResult(success=True)

    async def bind_operators(
        self, operator_id: str, web_session_id: str, context: VSOHttpContext
    ) -> bool:
        self.bound_operators.append(
            {"operator_id": operator_id, "web_session_id": web_session_id, "context": context}
        )
        return True


_: VSODClientProtocol = FakeVSODClient()
