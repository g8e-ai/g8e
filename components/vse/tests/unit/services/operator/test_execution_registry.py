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
import pytest
from unittest.mock import MagicMock
from app.services.operator.execution_registry import ExecutionRegistryService

@pytest.mark.asyncio
async def test_allocate_and_release():
    registry = ExecutionRegistryService()
    execution_id = "test-exec-1"
    
    registry.allocate(execution_id)
    assert execution_id in registry._pending_events
    assert isinstance(registry._pending_events[execution_id], asyncio.Event)
    
    registry.release(execution_id)
    assert execution_id not in registry._pending_events

@pytest.mark.asyncio
async def test_reallocateexecution_id(caplog):
    registry = ExecutionRegistryService()
    execution_id = "test-exec-dup"
    
    registry.allocate(execution_id)
    event1 = registry._pending_events[execution_id]
    
    registry.allocate(execution_id)
    event2 = registry._pending_events[execution_id]
    
    assert event1 is not event2
    assert "[REGISTRY] Re-allocating existing execution_id" in caplog.text

@pytest.mark.asyncio
async def test_signal_success():
    registry = ExecutionRegistryService()
    execution_id = "test-exec-signal"
    registry.allocate(execution_id)
    
    event = registry._pending_events[execution_id]
    assert not event.is_set()
    
    registry.signal(execution_id)
    assert event.is_set()

@pytest.mark.asyncio
async def test_signal_unknown_id(caplog):
    registry = ExecutionRegistryService()
    with caplog.at_level("DEBUG"):
        registry.signal("unknown-id")
    assert "[REGISTRY] Signal received for unknown execution_id" in caplog.text

@pytest.mark.asyncio
async def test_wait_success(task_tracker):
    registry = ExecutionRegistryService()
    execution_id = "test-exec-wait"
    registry.allocate(execution_id)
    
    # Signal in background
    async def delayed_signal():
        await asyncio.sleep(0.1)
        registry.signal(execution_id)

    task = task_tracker.track(asyncio.create_task(delayed_signal()))
    result = await registry.wait(execution_id, timeout=0.5)
    await task
    assert result is True

@pytest.mark.asyncio
async def test_wait_timeout():
    registry = ExecutionRegistryService()
    execution_id = "test-exec-timeout"
    registry.allocate(execution_id)
    
    result = await registry.wait(execution_id, timeout=0.1)
    assert result is False

@pytest.mark.asyncio
async def test_wait_unallocated(caplog):
    registry = ExecutionRegistryService()
    result = await registry.wait("unallocated-id", timeout=0.1)
    assert result is False
    assert "[REGISTRY] Attempted to wait on unallocated execution_id" in caplog.text
