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
from app.services.protocols import ExecutionRegistryProtocol

class FakeExecutionRegistry(ExecutionRegistryProtocol):
    """Fake execution registry for tests."""

    def __init__(self) -> None:
        self._events: dict[str, asyncio.Event] = {}
        self._results: dict[str, object] = {}
        self.allocate_calls: list[str] = []
        self.release_calls: list[str] = []
        self.signal_calls: list[str] = []
        self.complete_calls: list[str] = []

    def allocate(self, execution_id: str) -> None:
        self.allocate_calls.append(execution_id)
        self._events[execution_id] = asyncio.Event()

    def release(self, execution_id: str) -> None:
        self.release_calls.append(execution_id)
        self._events.pop(execution_id, None)
        self._results.pop(execution_id, None)

    def signal(self, execution_id: str) -> None:
        self.signal_calls.append(execution_id)
        event = self._events.get(execution_id)
        if event:
            event.set()

    def complete(self, execution_id: str, result: object) -> None:
        self.complete_calls.append(execution_id)
        self._results[execution_id] = result
        self.signal(execution_id)

    def get_result(self, execution_id: str) -> object | None:
        return self._results.get(execution_id)

    async def wait(self, execution_id: str, timeout: float) -> bool:
        event = self._events.get(execution_id)
        if not event:
            return False
        try:
            await asyncio.wait_for(event.wait(), timeout=timeout)
            return True
        except (asyncio.TimeoutError, TimeoutError):
            return False
