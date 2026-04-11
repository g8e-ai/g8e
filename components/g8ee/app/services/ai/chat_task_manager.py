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

from __future__ import annotations

import asyncio
import logging

from app.constants import EventType
from app.models.vsod_client import AiProcessingStoppedPayload
from app.utils.timestamp import now
from app.models.events import SessionEvent
from app.services.infra.vsod_event_service import EventService

logger = logging.getLogger(__name__)


class ChatTaskManager:
    """Owns asyncio task tracking and cancellation for in-flight AI chat processing.

    Single responsibility: track one asyncio.Task per investigation_id so that
    the stop-processing endpoint can cancel it on demand.  No LLM, DB, or
    business logic lives here.
    """

    def __init__(self) -> None:
        self._active_tasks: dict[str, asyncio.Task[None]] = {}
        self._task_lock = asyncio.Lock()

    async def track(
        self,
        investigation_id: str,
        task: asyncio.Task[None],
        auto_cancel_previous: bool = True,
    ) -> None:
        """Register a task for an investigation, optionally cancelling any prior one."""
        async with self._task_lock:
            if auto_cancel_previous and investigation_id in self._active_tasks:
                old_task = self._active_tasks[investigation_id]
                if not old_task.done():
                    old_task.cancel()
                    logger.info(
                        "Auto-cancelled previous AI task for %s (new message)",
                        investigation_id,
                        extra={"investigation_id": investigation_id},
                    )

            self._active_tasks[investigation_id] = task
            logger.info("Tracking AI task for %s", investigation_id)

    async def untrack(self, investigation_id: str) -> None:
        """Remove a task from active tracking."""
        async with self._task_lock:
            self._active_tasks.pop(investigation_id, None)
            logger.info("Untracked AI task for %s", investigation_id)

    async def cancel(
        self,
        investigation_id: str,
        reason: str,
        web_session_id: str,
        user_id: str,
        case_id: str,
        vsod_event_service: EventService,
    ) -> bool:
        """Cancel active AI processing for an investigation.

        Returns True if a task was cancelled, False if no active task existed.
        Publishes AI_PROCESSING_STOPPED via vsod_event_service when both
        web_session_id and vsod_event_service are provided.
        """
        async with self._task_lock:
            task = self._active_tasks.get(investigation_id)
            if not task or task.done():
                logger.info(
                    "No active AI task to cancel for investigation %s",
                    investigation_id,
                    extra={"investigation_id": investigation_id, "reason": reason},
                )
                return False

            task.cancel()
            logger.info(
                "Cancelled AI processing for investigation %s",
                investigation_id,
                extra={"investigation_id": investigation_id, "reason": reason},
            )

        if web_session_id and case_id and vsod_event_service:
            try:
                await vsod_event_service.publish(
                    SessionEvent(
                        event_type=EventType.LLM_CHAT_ITERATION_STOPPED,
                        payload=AiProcessingStoppedPayload(
                            reason=reason,
                            timestamp=now(),
                        ),
                        web_session_id=web_session_id,
                        user_id=user_id,
                        case_id=case_id,
                        investigation_id=investigation_id,
                    )
                )
            except Exception as e:
                logger.warning("Failed to send stop event: %s", e)
        elif not vsod_event_service:
            logger.warning(
                "Cannot send ai.processing_stopped event - no vsod_event_service provided",
                extra={"investigation_id": investigation_id},
            )
        elif not web_session_id:
            logger.warning(
                "Cannot send ai.processing_stopped event - no web_session_id provided",
                extra={"investigation_id": investigation_id},
            )

        return True
