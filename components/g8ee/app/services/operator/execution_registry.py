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

"""Execution Registry Service

Manages the lifecycle of asynchronous execution events.
Provides synchronization between command dispatch and result reception
via in-memory asyncio.Event gates and a result stash.

No persistent storage — fully stateless and event-driven.
"""

import asyncio
import logging
from app.models.pubsub_messages import G8eoResultEnvelope
from app.services.protocols import ExecutionRegistryProtocol

logger = logging.getLogger(__name__)


class ExecutionRegistryService(ExecutionRegistryProtocol):
    """Manages asyncio events and in-memory result stash for command execution."""

    def __init__(self) -> None:
        self._pending_events: dict[str, asyncio.Event] = {}
        self._results: dict[str, object] = {}

    def allocate(self, execution_id: str) -> None:
        """Create a new event for tracking an execution."""
        if execution_id in self._pending_events:
            logger.warning("[REGISTRY] Re-allocating existing execution_id: %s", execution_id)
        self._pending_events[execution_id] = asyncio.Event()

    def release(self, execution_id: str) -> None:
        """Remove an event and clean up resources."""
        self._pending_events.pop(execution_id, None)
        self._results.pop(execution_id, None)

    def signal(self, execution_id: str) -> None:
        """Signal that a result has arrived for an execution."""
        event = self._pending_events.get(execution_id)
        if event:
            event.set()
        else:
            logger.info("[REGISTRY] Signal received for unknown execution_id: %s", execution_id)

    def complete(self, execution_id: str, result: object) -> None:
        """Stash a result payload and signal the waiter."""
        self._results[execution_id] = result
        self.signal(execution_id)

    def get_result(self, execution_id: str) -> G8eoResultEnvelope | None:
        """Retrieve a stashed result payload (does not remove it)."""
        result = self._results.get(execution_id)
        if result is None:
            return None
        # Cast to satisfy type checker since we store as object for flexibility
        from typing import cast
        return cast(G8eoResultEnvelope, result)

    async def wait(self, execution_id: str, timeout: float) -> bool:
        """Wait for an execution to complete or time out."""
        event = self._pending_events.get(execution_id)
        if not event:
            logger.warning("[REGISTRY] Attempted to wait on unallocated execution_id: %s", execution_id)
            return False
        try:
            await asyncio.wait_for(event.wait(), timeout=timeout)
            return True
        except (asyncio.TimeoutError, TimeoutError):
            return False
