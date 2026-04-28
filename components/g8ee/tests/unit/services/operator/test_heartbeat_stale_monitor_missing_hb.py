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

import pytest
from datetime import UTC, datetime, timedelta
from unittest.mock import AsyncMock

from app.constants import OperatorStatus
from app.models.operators import OperatorDocument
from app.services.operator.heartbeat_stale_monitor import HeartbeatStaleMonitorService

THRESHOLD_SECONDS = 60

def make_operator(**kwargs):
    defaults = {
        "id": "op-1",
        "user_id": "user-1",
        "status": OperatorStatus.BOUND,
        "last_heartbeat": None,
        "claimed_at": datetime.now(UTC) - timedelta(seconds=THRESHOLD_SECONDS + 30),
    }
    defaults.update(kwargs)
    return OperatorDocument.model_validate(defaults)

@pytest.mark.asyncio
async def test_tick_transitions_missing_heartbeat_bound_to_stale():
    operator_data_service = AsyncMock()
    event_service = AsyncMock()

    # Operator is BOUND, has NO last_heartbeat, but claimed_at is old
    op = make_operator(
        status=OperatorStatus.BOUND,
        last_heartbeat=None,
        claimed_at=datetime.now(UTC) - timedelta(seconds=THRESHOLD_SECONDS + 30)
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

@pytest.mark.asyncio
async def test_tick_transitions_missing_heartbeat_active_to_offline():
    operator_data_service = AsyncMock()
    event_service = AsyncMock()

    # Operator is ACTIVE, has NO last_heartbeat, but claimed_at is old
    # (Maybe it was activated manually or via some other flow)
    op = make_operator(
        status=OperatorStatus.ACTIVE,
        last_heartbeat=None,
        claimed_at=datetime.now(UTC) - timedelta(seconds=THRESHOLD_SECONDS + 30)
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
