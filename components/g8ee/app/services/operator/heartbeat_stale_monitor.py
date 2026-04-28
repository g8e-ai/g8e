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
import logging
from datetime import datetime, timezone

from app.constants import EventType, OperatorStatus, HEARTBEAT_STALE_THRESHOLD_SECONDS
from app.models.events import BackgroundEvent
from app.models.operators import OperatorStatusUpdatedPayload, OperatorDocument
from app.services.protocols import (
    OperatorDataServiceProtocol,
    EventServiceProtocol,
)

logger = logging.getLogger(__name__)

DEFAULT_MONITOR_INTERVAL_SECONDS = 15

MONITORED_STATUSES = {
    OperatorStatus.ACTIVE,
    OperatorStatus.BOUND,
    OperatorStatus.STALE,
    OperatorStatus.OFFLINE,
}

def operator_status_to_event_type(status: OperatorStatus) -> EventType:
    """Map an OperatorStatus value to its canonical OPERATOR_STATUS_UPDATED_* EventType."""
    mapping = {
        OperatorStatus.ACTIVE: EventType.OPERATOR_STATUS_UPDATED_ACTIVE,
        OperatorStatus.AVAILABLE: EventType.OPERATOR_STATUS_UPDATED_AVAILABLE,
        OperatorStatus.UNAVAILABLE: EventType.OPERATOR_STATUS_UPDATED_UNAVAILABLE,
        OperatorStatus.BOUND: EventType.OPERATOR_STATUS_UPDATED_BOUND,
        OperatorStatus.OFFLINE: EventType.OPERATOR_STATUS_UPDATED_OFFLINE,
        OperatorStatus.STALE: EventType.OPERATOR_STATUS_UPDATED_STALE,
        OperatorStatus.STOPPED: EventType.OPERATOR_STATUS_UPDATED_STOPPED,
        OperatorStatus.TERMINATED: EventType.OPERATOR_STATUS_UPDATED_TERMINATED,
    }
    if status not in mapping:
        raise ValueError(f"Unknown OperatorStatus value: {status}")
    return mapping[status]

def resolve_heartbeat_transition(current_status: OperatorStatus, is_stale: bool) -> OperatorStatus | None:
    """Compute the target status for an operator given whether its heartbeat is
    currently stale. Returns None when no transition is required.
    """
    if is_stale:
        if current_status == OperatorStatus.BOUND:
            return OperatorStatus.STALE
        if current_status == OperatorStatus.ACTIVE:
            return OperatorStatus.OFFLINE
        return None
    
    if current_status == OperatorStatus.STALE:
        return OperatorStatus.BOUND
    if current_status == OperatorStatus.OFFLINE:
        return OperatorStatus.ACTIVE
    return None

class HeartbeatStaleMonitorService:
    """Periodically scans operator documents and reconciles their `status` field
    against how recently g8eo has been phoning home (`last_heartbeat`).

    Ported from g8ed's HeartbeatMonitorService to consolidate operator ownership in g8ee.
    """

    def __init__(
        self,
        operator_data_service: OperatorDataServiceProtocol,
        event_service: EventServiceProtocol,
        threshold_seconds: int = HEARTBEAT_STALE_THRESHOLD_SECONDS,
        interval_seconds: int = DEFAULT_MONITOR_INTERVAL_SECONDS,
    ):
        self._operator_data_service = operator_data_service
        self._event_service = event_service
        self._threshold_seconds = threshold_seconds
        self._interval_seconds = interval_seconds
        self._task: asyncio.Task | None = None
        self._ticking = False

    async def start(self) -> None:
        if self._task:
            return
        
        self._task = asyncio.create_task(self._run_loop())
        logger.info(
            "[HEARTBEAT-MONITOR] Started",
            extra={
                "threshold_seconds": self._threshold_seconds,
                "interval_seconds": self._interval_seconds,
            },
        )

    async def stop(self) -> None:
        if self._task:
            self._task.cancel()
            try:
                await self._task
            except asyncio.CancelledError:
                pass
            self._task = None
            logger.info("[HEARTBEAT-MONITOR] Stopped")

    async def _run_loop(self) -> None:
        while True:
            try:
                await asyncio.sleep(self._interval_seconds)
                await self.tick()
            except asyncio.CancelledError:
                break
            except Exception:
                logger.exception("[HEARTBEAT-MONITOR] Loop encountered unexpected error")

    async def tick(self) -> None:
        """Run a single reconciliation pass. Safe to call manually; concurrent
        invocations are coalesced.
        """
        if self._ticking:
            return
        self._ticking = True
        try:
            # Bypass the query cache: monitor correctness depends on real-time last_heartbeat values.
            operators = await self._operator_data_service.query_operators(
                field_filters=[],
                bypass_cache=True
            )
            now = datetime.now(timezone.utc)

            transitions = 0
            for op in operators:
                if op.status not in MONITORED_STATUSES:
                    continue
                if not op.last_heartbeat:
                    continue

                last_hb = op.last_heartbeat
                if isinstance(last_hb, str):
                    try:
                        last_hb = datetime.fromisoformat(last_hb.replace("Z", "+00:00"))
                    except ValueError:
                        continue
                
                if not isinstance(last_hb, datetime):
                    continue

                age_seconds = (now - last_hb).total_seconds()
                is_stale = age_seconds > self._threshold_seconds
                target = resolve_heartbeat_transition(op.status, is_stale)
                
                if not target:
                    continue

                applied = await self._apply_transition(op, target, age_seconds)
                if applied:
                    transitions += 1

            if transitions > 0:
                logger.info("[HEARTBEAT-MONITOR] Reconciliation complete", extra={"transitions": transitions})
        finally:
            self._ticking = False

    async def _apply_transition(self, operator: OperatorDocument, target_status: OperatorStatus, age_seconds: float) -> bool:
        operator_id = operator.id
        from_status = operator.status
        try:
            success = await self._operator_data_service.update_operator_status(
                operator_id=operator_id,
                status=target_status,
            )
            
            if not success:
                logger.warning(
                    "[HEARTBEAT-MONITOR] Failed to persist status transition",
                    extra={
                        "operator_id": operator_id,
                        "from": from_status,
                        "to": target_status,
                    },
                )
                return False

            logger.info(
                "[HEARTBEAT-MONITOR] Operator status transitioned",
                extra={
                    "operator_id": operator_id,
                    "user_id": operator.user_id,
                    "from": from_status,
                    "to": target_status,
                    "heartbeat_age_seconds": round(age_seconds, 2),
                },
            )

            await self._publish_transition(operator, target_status)
            return True
        except Exception:
            logger.exception(
                "[HEARTBEAT-MONITOR] Transition failed",
                extra={
                    "operator_id": operator_id,
                    "from": from_status,
                    "to": target_status,
                },
            )
            return False

    async def _publish_transition(self, operator: OperatorDocument, target_status: OperatorStatus) -> None:
        if not operator.user_id:
            return
        try:
            payload = OperatorStatusUpdatedPayload(
                operator_id=operator.id,
                status=target_status,
                hostname=operator.current_hostname,
                system_fingerprint=operator.latest_heartbeat_snapshot.system_fingerprint if operator.latest_heartbeat_snapshot else None,
                timestamp=datetime.now(timezone.utc),
            )
            
            event = BackgroundEvent(
                event_type=operator_status_to_event_type(target_status),
                payload=payload,
                user_id=operator.user_id,
            )
            
            await self._event_service.publish(event)
        except Exception:
            logger.warning(
                "[HEARTBEAT-MONITOR] SSE fan-out failed (non-blocking)",
                extra={
                    "operator_id": operator.id,
                    "target": target_status,
                },
                exc_info=True,
            )
