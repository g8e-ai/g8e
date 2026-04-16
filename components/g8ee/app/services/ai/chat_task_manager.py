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
from app.models.g8ed_client import AiProcessingStoppedPayload
from app.utils.timestamp import now
from app.models.events import SessionEvent
from app.services.infra.g8ed_event_service import EventService

logger = logging.getLogger(__name__)


class BackgroundTaskManager:
    """Owns asyncio task tracking and cancellation for background operations.

    Single responsibility: track asyncio.Task objects so they can be:
    - Cancelled on demand (e.g., stop-processing endpoint)
    - Awaited during cleanup (e.g., test teardown)

    No LLM, DB, or business logic lives here.
    """

    def __init__(self) -> None:
        self._active_tasks: dict[str, asyncio.Task[None]] = {}
        self._task_lock = asyncio.Lock()

    async def track(
        self,
        task_id: str,
        task: asyncio.Task[None],
        auto_cancel_previous: bool = True,
    ) -> None:
        """Register a task by ID, optionally cancelling any prior task with the same ID."""
        async with self._task_lock:
            if auto_cancel_previous and task_id in self._active_tasks:
                old_task = self._active_tasks[task_id]
                if not old_task.done():
                    old_task.cancel()
                    logger.info(
                        "Auto-cancelled previous task for %s (new task)",
                        task_id,
                        extra={"task_id": task_id},
                    )

            self._active_tasks[task_id] = task
            logger.info("Tracking task for %s", task_id)

    async def untrack(self, task_id: str) -> None:
        """Remove a task from active tracking."""
        async with self._task_lock:
            self._active_tasks.pop(task_id, None)
            logger.info("Untracked task for %s", task_id)

    async def cancel(
        self,
        task_id: str,
        reason: str,
        web_session_id: str | None = None,
        user_id: str | None = None,
        case_id: str | None = None,
        g8ed_event_service: EventService | None = None,
    ) -> bool:
        """Cancel active task for the given ID.

        Returns True if a task was cancelled, False if no active task existed.
        Publishes AI_PROCESSING_STOPPED via g8ed_event_service when both
        web_session_id and g8ed_event_service are provided.
        """
        async with self._task_lock:
            task = self._active_tasks.get(task_id)
            if not task or task.done():
                logger.info(
                    "No active task to cancel for %s",
                    task_id,
                    extra={"task_id": task_id, "reason": reason},
                )
                return False

            task.cancel()
            logger.info(
                "Cancelled task for %s",
                task_id,
                extra={"task_id": task_id, "reason": reason},
            )

        if web_session_id and case_id and g8ed_event_service:
            try:
                await g8ed_event_service.publish(
                    SessionEvent(
                        event_type=EventType.LLM_CHAT_ITERATION_STOPPED,
                        payload=AiProcessingStoppedPayload(
                            reason=reason,
                            timestamp=now(),
                        ),
                        web_session_id=web_session_id,
                        user_id=user_id or "",
                        case_id=case_id,
                        investigation_id=task_id,
                    )
                )
            except Exception as e:
                logger.warning("Failed to send stop event: %s", e)
        elif not g8ed_event_service:
            logger.warning(
                "Cannot send ai.processing_stopped event - no g8ed_event_service provided",
                extra={"task_id": task_id},
            )
        elif not web_session_id:
            logger.warning(
                "Cannot send ai.processing_stopped event - no web_session_id provided",
                extra={"task_id": task_id},
            )

        return True

    async def wait_all(self, timeout: float | None = None) -> None:
        """Await completion of all tracked tasks.

        This is used during cleanup to ensure all background operations
        complete before resource deletion. Tasks that are already done are
        skipped. Cancelled tasks are awaited to ensure proper cleanup.

        Args:
            timeout: Optional timeout in seconds. If provided, raises TimeoutError
                    if not all tasks complete within the timeout.
        """
        async with self._task_lock:
            tasks = list(self._active_tasks.values())

        if not tasks:
            logger.debug("No tasks to await")
            return

        logger.info("Awaiting completion of %d tracked tasks", len(tasks))

        try:
            if timeout is not None:
                async with asyncio.timeout(timeout):
                    await asyncio.gather(*tasks, return_exceptions=True)
            else:
                await asyncio.gather(*tasks, return_exceptions=True)
            logger.info("All tracked tasks completed")
        except TimeoutError:
            logger.warning(
                "Timeout waiting for %d tasks to complete after %s seconds",
                len(tasks),
                timeout,
            )
            raise
        except Exception as e:
            logger.error("Error awaiting tracked tasks: %s", e, exc_info=True)
            raise


# Backward compatibility alias
ChatTaskManager = BackgroundTaskManager
