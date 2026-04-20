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

"""Typed fake for G8edClientProtocol."""

from app.models.events import BackgroundEvent, SessionEvent
from app.models.http_context import G8eHttpContext
from app.models.g8ed_client import IntentOperationResult, SSEPushResponse
from app.services.protocols import G8edClientProtocol


class FakeG8edClient:
    """Typed fake implementing G8edClientProtocol.

    Records all calls for assertion in tests. Does not perform any real I/O.
    """

    def __init__(self) -> None:
        self.granted: list[dict] = []
        self.revoked: list[dict] = []
        self.pushed_events: list[SessionEvent | BackgroundEvent] = []
        self.bound_operators: list[dict] = []
        self._push_sse_event_response: SSEPushResponse = SSEPushResponse(success=True, delivered=1)
        self._push_sse_event_exception: Exception | None = None

    async def push_sse_event(self, event: SessionEvent | BackgroundEvent) -> SSEPushResponse:
        self.pushed_events.append(event)
        if self._push_sse_event_exception is not None:
            raise self._push_sse_event_exception
        return self._push_sse_event_response

    def set_push_sse_event_response(self, response: SSEPushResponse) -> None:
        self._push_sse_event_response = response
        self._push_sse_event_exception = None

    def set_push_sse_event_exception(self, exc: Exception) -> None:
        self._push_sse_event_exception = exc

    def set_push_sse_event_return_value(self, value: bool) -> None:
        """Back-compat shim: True => success, False => success=False (inline failure)."""
        self._push_sse_event_response = SSEPushResponse(
            success=value,
            delivered=1 if value else 0,
            error=None if value else "fake failure",
        )
        self._push_sse_event_exception = None

    def assert_push_sse_event_called_with(self, event: SessionEvent | BackgroundEvent) -> None:
        assert event in self.pushed_events

    async def grant_intent(
        self, operator_id: str, intent: str, context: G8eHttpContext
    ) -> IntentOperationResult:
        self.granted.append({"operator_id": operator_id, "intent": intent, "context": context})
        return IntentOperationResult(success=True)

    async def revoke_intent(
        self, operator_id: str, intent: str, context: G8eHttpContext
    ) -> IntentOperationResult:
        self.revoked.append({"operator_id": operator_id, "intent": intent, "context": context})
        return IntentOperationResult(success=True)

    async def bind_operators(
        self, operator_id: str, web_session_id: str, context: G8eHttpContext
    ) -> bool:
        self.bound_operators.append(
            {"operator_id": operator_id, "web_session_id": web_session_id, "context": context}
        )
        return True


_: G8edClientProtocol = FakeG8edClient()
