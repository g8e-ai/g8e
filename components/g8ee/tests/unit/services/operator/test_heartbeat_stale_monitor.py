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
from datetime import datetime, timezone, timedelta
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from app.constants import EventType, OperatorStatus
from app.models.operators import OperatorDocument
from app.services.operator.heartbeat_stale_monitor import (
    HeartbeatStaleMonitorService,
    resolve_heartbeat_transition,
    operator_status_to_event_type,
)

THRESHOLD_SECONDS = 60

def make_operator(**kwargs):
    defaults = {
        "id": "op-1",
        "user_id": "user-1",
        "status": OperatorStatus.BOUND,
        "last_heartbeat": datetime.now(timezone.utc) - timedelta(seconds=10),
        "system_info": {"hostname": "node-02"},
        "system_fingerprint": "fp-1",
    }
    defaults.update(kwargs)
    return OperatorDocument.model_validate(defaults)

def test_resolve_heartbeat_transition():
    # Stale transitions
    assert resolve_heartbeat_transition(OperatorStatus.BOUND, True) == OperatorStatus.STALE
    assert resolve_heartbeat_transition(OperatorStatus.ACTIVE, True) == OperatorStatus.OFFLINE
    
    # Fresh transitions (recovery)
    assert resolve_heartbeat_transition(OperatorStatus.STALE, False) == OperatorStatus.BOUND
    assert resolve_heartbeat_transition(OperatorStatus.OFFLINE, False) == OperatorStatus.ACTIVE
    
    # No transition
    assert resolve_heartbeat_transition(OperatorStatus.BOUND, False) is None
    assert resolve_heartbeat_transition(OperatorStatus.ACTIVE, False) is None
    assert resolve_heartbeat_transition(OperatorStatus.STALE, True) is None
    assert resolve_heartbeat_transition(OperatorStatus.OFFLINE, True) is None
    
    # Ignored statuses
    assert resolve_heartbeat_transition(OperatorStatus.AVAILABLE, True) is None
    assert resolve_heartbeat_transition(OperatorStatus.TERMINATED, True) is None
    assert resolve_heartbeat_transition(OperatorStatus.STOPPED, True) is None

def test_operator_status_to_event_type():
    assert operator_status_to_event_type(OperatorStatus.STALE) == EventType.OPERATOR_STATUS_UPDATED_STALE
    assert operator_status_to_event_type(OperatorStatus.OFFLINE) == EventType.OPERATOR_STATUS_UPDATED_OFFLINE
    with pytest.raises(ValueError):
        operator_status_to_event_type("invalid-status")

@pytest.mark.asyncio
async def test_tick_transitions_stale_bound_to_stale():
    operator_data_service = AsyncMock()
    event_service = AsyncMock()
    
    op = make_operator(
        status=OperatorStatus.BOUND,
        last_heartbeat=datetime.now(timezone.utc) - timedelta(seconds=THRESHOLD_SECONDS + 30)
    )
    operator_data_service.query_operators.return_value = [op]
    operator_data_service.update_operator_status.return_value = True
    
    service = HeartbeatStaleMonitorService(
        operator_data_service=operator_data_service,
        event_service=event_service,
        threshold_seconds=THRESHOLD_SECONDS
    )
    
    await service.tick()
    
    operator_data_service.update_operator_status.assert_called_once_with(
        operator_id="op-1",
        status=OperatorStatus.STALE
    )
    event_service.publish.assert_called_once()
    event = event_service.publish.call_args[0][0]
    assert event.event_type == EventType.OPERATOR_STATUS_UPDATED_STALE
    assert event.payload.operator_id == "op-1"
    assert event.payload.status == OperatorStatus.STALE

@pytest.mark.asyncio
async def test_tick_transitions_stale_active_to_offline():
    operator_data_service = AsyncMock()
    event_service = AsyncMock()
    
    op = make_operator(
        status=OperatorStatus.ACTIVE,
        last_heartbeat=datetime.now(timezone.utc) - timedelta(seconds=THRESHOLD_SECONDS + 10)
    )
    operator_data_service.query_operators.return_value = [op]
    operator_data_service.update_operator_status.return_value = True
    
    service = HeartbeatStaleMonitorService(
        operator_data_service=operator_data_service,
        event_service=event_service,
        threshold_seconds=THRESHOLD_SECONDS
    )
    
    await service.tick()
    
    operator_data_service.update_operator_status.assert_called_once_with(
        operator_id="op-1",
        status=OperatorStatus.OFFLINE
    )

@pytest.mark.asyncio
async def test_tick_recovers_stale_to_bound():
    operator_data_service = AsyncMock()
    event_service = AsyncMock()
    
    op = make_operator(
        status=OperatorStatus.STALE,
        last_heartbeat=datetime.now(timezone.utc) - timedelta(seconds=5)
    )
    operator_data_service.query_operators.return_value = [op]
    operator_data_service.update_operator_status.return_value = True
    
    service = HeartbeatStaleMonitorService(
        operator_data_service=operator_data_service,
        event_service=event_service,
        threshold_seconds=THRESHOLD_SECONDS
    )
    
    await service.tick()
    
    operator_data_service.update_operator_status.assert_called_once_with(
        operator_id="op-1",
        status=OperatorStatus.BOUND
    )

@pytest.mark.asyncio
async def test_tick_recovers_offline_to_active():
    operator_data_service = AsyncMock()
    event_service = AsyncMock()
    
    op = make_operator(
        status=OperatorStatus.OFFLINE,
        last_heartbeat=datetime.now(timezone.utc) - timedelta(seconds=5)
    )
    operator_data_service.query_operators.return_value = [op]
    operator_data_service.update_operator_status.return_value = True
    
    service = HeartbeatStaleMonitorService(
        operator_data_service=operator_data_service,
        event_service=event_service,
        threshold_seconds=THRESHOLD_SECONDS
    )
    
    await service.tick()
    
    operator_data_service.update_operator_status.assert_called_once_with(
        operator_id="op-1",
        status=OperatorStatus.ACTIVE
    )

@pytest.mark.asyncio
async def test_tick_ignores_missing_heartbeat():
    operator_data_service = AsyncMock()
    event_service = AsyncMock()
    
    op = make_operator(status=OperatorStatus.BOUND, last_heartbeat=None)
    operator_data_service.query_operators.return_value = [op]
    
    service = HeartbeatStaleMonitorService(
        operator_data_service=operator_data_service,
        event_service=event_service,
        threshold_seconds=THRESHOLD_SECONDS
    )
    
    await service.tick()
    
    operator_data_service.update_operator_status.assert_not_called()

@pytest.mark.asyncio
async def test_tick_ignores_non_monitored_statuses():
    operator_data_service = AsyncMock()
    event_service = AsyncMock()
    
    old_ts = datetime.now(timezone.utc) - timedelta(days=1)
    operators = [
        make_operator(id="op-1", status=OperatorStatus.AVAILABLE, last_heartbeat=old_ts),
        make_operator(id="op-2", status=OperatorStatus.TERMINATED, last_heartbeat=old_ts),
        make_operator(id="op-3", status=OperatorStatus.STOPPED, last_heartbeat=old_ts),
    ]
    operator_data_service.query_operators.return_value = operators
    
    service = HeartbeatStaleMonitorService(
        operator_data_service=operator_data_service,
        event_service=event_service,
        threshold_seconds=THRESHOLD_SECONDS
    )
    
    await service.tick()
    
    operator_data_service.update_operator_status.assert_not_called()

@pytest.mark.asyncio
async def test_tick_continues_on_publish_failure():
    operator_data_service = AsyncMock()
    event_service = AsyncMock()
    event_service.publish.side_effect = Exception("SSE down")
    
    op = make_operator(
        status=OperatorStatus.BOUND,
        last_heartbeat=datetime.now(timezone.utc) - timedelta(seconds=THRESHOLD_SECONDS + 10)
    )
    operator_data_service.query_operators.return_value = [op]
    operator_data_service.update_operator_status.return_value = True
    
    service = HeartbeatStaleMonitorService(
        operator_data_service=operator_data_service,
        event_service=event_service,
        threshold_seconds=THRESHOLD_SECONDS
    )
    
    # Should not raise
    await service.tick()
    
    operator_data_service.update_operator_status.assert_called_once()

@pytest.mark.asyncio
async def test_tick_coalesces_concurrent_calls():
    operator_data_service = AsyncMock()
    event_service = AsyncMock()
    
    # Slow down query_operators
    async def slow_query(*args, **kwargs):
        await asyncio.sleep(0.1)
        return []
    
    operator_data_service.query_operators.side_effect = slow_query
    
    service = HeartbeatStaleMonitorService(
        operator_data_service=operator_data_service,
        event_service=event_service
    )
    
    await asyncio.gather(service.tick(), service.tick())
    
    assert operator_data_service.query_operators.call_count == 1

@pytest.mark.asyncio
async def test_lifecycle_idempotence():
    operator_data_service = AsyncMock()
    event_service = AsyncMock()
    
    service = HeartbeatStaleMonitorService(
        operator_data_service=operator_data_service,
        event_service=event_service,
        interval_seconds=1000
    )
    
    await service.start()
    task = service._task
    assert task is not None
    
    await service.start()
    assert service._task is task
    
    await service.stop()
    assert service._task is None
    assert task.cancelled()
