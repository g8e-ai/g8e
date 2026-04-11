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
from unittest.mock import AsyncMock, MagicMock

import pytest

from app.constants import EventType
from app.services.ai.chat_task_manager import ChatTaskManager
from app.services.infra.vsod_event_service import EventService

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]


@pytest.fixture
def manager():
    return ChatTaskManager()


def _make_event_service() -> EventService:
    svc = MagicMock(spec=EventService)
    svc.publish = AsyncMock()
    return svc


class TestTaskTracking:

    async def test_track_adds_task(self, manager, task_tracker):
        investigation_id = "inv-123"

        async def dummy():
            await asyncio.sleep(0.1)

        task = task_tracker.track(asyncio.create_task(dummy()))
        await manager.track(investigation_id, task)

        assert investigation_id in manager._active_tasks
        assert manager._active_tasks[investigation_id] == task

    async def test_track_auto_cancels_previous(self, manager, task_tracker):
        investigation_id = "inv-123"

        async def long_task():
            try:
                await asyncio.sleep(10)
            except asyncio.CancelledError:
                raise

        task1 = task_tracker.track(asyncio.create_task(long_task()))
        await manager.track(investigation_id, task1, auto_cancel_previous=True)

        task2 = task_tracker.track(asyncio.create_task(long_task()))
        await manager.track(investigation_id, task2, auto_cancel_previous=True)

        await asyncio.sleep(0.01)

        assert task1.cancelled()
        assert not task2.cancelled()
        assert manager._active_tasks[investigation_id] == task2

    async def test_track_no_auto_cancel_preserves_old_task(self, manager, task_tracker):
        investigation_id = "inv-123"

        async def dummy():
            await asyncio.sleep(0.1)

        task1 = task_tracker.track(asyncio.create_task(dummy()))
        await manager.track(investigation_id, task1, auto_cancel_previous=False)

        task2 = task_tracker.track(asyncio.create_task(dummy()))
        await manager.track(investigation_id, task2, auto_cancel_previous=False)

        await asyncio.sleep(0.01)
        assert not task1.cancelled()
        assert manager._active_tasks[investigation_id] == task2

    async def test_untrack_removes_task(self, manager, task_tracker):
        investigation_id = "inv-123"

        async def dummy():
            await asyncio.sleep(0.1)

        task = task_tracker.track(asyncio.create_task(dummy()))
        manager._active_tasks[investigation_id] = task

        await manager.untrack(investigation_id)

        assert investigation_id not in manager._active_tasks

    async def test_untrack_nonexistent_no_error(self, manager):
        await manager.untrack("inv-nonexistent")
        assert "inv-nonexistent" not in manager._active_tasks


class TestCancelBehaviour:

    async def test_cancel_returns_true_and_cancels_task(self, manager, task_tracker):
        investigation_id = "inv-123"

        async def long_task():
            try:
                await asyncio.sleep(10)
            except asyncio.CancelledError:
                raise

        task = task_tracker.track(asyncio.create_task(long_task()))
        manager._active_tasks[investigation_id] = task

        result = await manager.cancel(
            investigation_id,
            reason="Stop",
            web_session_id="web-1",
            user_id="user-1",
            case_id="case-1",
            vsod_event_service=AsyncMock()
        )

        await asyncio.sleep(0.01)

        assert result is True
        assert task.cancelled()

    async def test_cancel_no_active_task_returns_false(self, manager):
        result = await manager.cancel(
            "inv-nonexistent",
            reason="Stop",
            web_session_id="web-1",
            user_id="user-1",
            case_id="case-1",
            vsod_event_service=AsyncMock()
        )
        assert result is False

    async def test_cancel_done_task_returns_false(self, manager):
        investigation_id = "inv-123"

        async def quick():
            pass

        task = asyncio.create_task(quick())
        await task
        manager._active_tasks[investigation_id] = task

        result = await manager.cancel(
            investigation_id,
            reason="Stop",
            web_session_id="web-1",
            user_id="user-1",
            case_id="case-1",
            vsod_event_service=AsyncMock()
        )
        assert result is False

    async def test_cancel_publishes_stop_event(self, manager, task_tracker):
        investigation_id = "inv-123"
        vsod_event_service = _make_event_service()

        async def long_task():
            try:
                await asyncio.sleep(10)
            except asyncio.CancelledError:
                raise

        task = task_tracker.track(asyncio.create_task(long_task()))
        manager._active_tasks[investigation_id] = task

        result = await manager.cancel(
            investigation_id,
            reason="Test cancellation",
            web_session_id="web-session-456",
            user_id="user-456",
            case_id="case-test-123",
            vsod_event_service=vsod_event_service,
        )

        await asyncio.sleep(0.01)

        assert result is True
        assert task.cancelled()
        vsod_event_service.publish.assert_called_once()
        event = vsod_event_service.publish.call_args[0][0]
        assert event.event_type == EventType.LLM_CHAT_ITERATION_STOPPED
        assert event.payload.reason == "Test cancellation"
        from datetime import datetime
        assert isinstance(event.payload.timestamp, datetime)
        assert event.web_session_id == "web-session-456"
        assert event.investigation_id == investigation_id

    async def test_cancel_handles_event_publish_failure_gracefully(self, manager, task_tracker):
        investigation_id = "inv-123"
        vsod_event_service = _make_event_service()
        vsod_event_service.publish.side_effect = Exception("publish failed")

        async def long_task():
            try:
                await asyncio.sleep(10)
            except asyncio.CancelledError:
                raise

        task = task_tracker.track(asyncio.create_task(long_task()))
        manager._active_tasks[investigation_id] = task

        result = await manager.cancel(
            investigation_id,
            reason="Test",
            web_session_id="web-session-789",
            user_id="user-789",
            case_id="case-test-789",
            vsod_event_service=vsod_event_service,
        )

        await asyncio.sleep(0.01)

        assert result is True
        assert task.cancelled()

    async def test_cancel_warns_when_no_vsod_event_service(self, manager, caplog, task_tracker):
        investigation_id = "inv-123"

        async def long_task():
            try:
                await asyncio.sleep(10)
            except asyncio.CancelledError:
                raise

        task = task_tracker.track(asyncio.create_task(long_task()))
        manager._active_tasks[investigation_id] = task

        with caplog.at_level(logging.WARNING):
            result = await manager.cancel(
                investigation_id,
                reason="Test",
                web_session_id="web-session-123",
                user_id="user-123",
                case_id="case-123",
                vsod_event_service=None,
            )

        await asyncio.sleep(0.01)

        assert result is True
        assert task.cancelled()
        assert any("no vsod_event_service" in r.message for r in caplog.records)

    async def test_cancel_warns_when_no_web_session_id(self, manager, caplog, task_tracker):
        investigation_id = "inv-123"
        vsod_event_service = _make_event_service()

        async def long_task():
            try:
                await asyncio.sleep(10)
            except asyncio.CancelledError:
                raise

        task = task_tracker.track(asyncio.create_task(long_task()))
        manager._active_tasks[investigation_id] = task

        with caplog.at_level(logging.WARNING):
            result = await manager.cancel(
                investigation_id,
                reason="Test",
                web_session_id=None,
                user_id="user-123",
                case_id="case-123",
                vsod_event_service=vsod_event_service,
            )

        await asyncio.sleep(0.01)

        assert result is True
        assert task.cancelled()
        vsod_event_service.publish.assert_not_called()
        assert any("no web_session_id" in r.message for r in caplog.records)


class TestConcurrentTaskHandling:

    async def test_multiple_investigations_tracked_independently(self, manager, task_tracker):
        async def dummy():
            await asyncio.sleep(0.1)

        task1 = task_tracker.track(asyncio.create_task(dummy()))
        task2 = task_tracker.track(asyncio.create_task(dummy()))
        task3 = task_tracker.track(asyncio.create_task(dummy()))

        await manager.track("inv-1", task1)
        await manager.track("inv-2", task2)
        await manager.track("inv-3", task3)

        assert len(manager._active_tasks) == 3
        assert manager._active_tasks["inv-1"] == task1
        assert manager._active_tasks["inv-2"] == task2
        assert manager._active_tasks["inv-3"] == task3

    async def test_cancel_one_doesnt_affect_others(self, manager, task_tracker):
        vsod_event_service = _make_event_service()

        async def long_task():
            try:
                await asyncio.sleep(10)
            except asyncio.CancelledError:
                raise

        task1 = task_tracker.track(asyncio.create_task(long_task()))
        task2 = task_tracker.track(asyncio.create_task(long_task()))

        manager._active_tasks["inv-1"] = task1
        manager._active_tasks["inv-2"] = task2

        await manager.cancel(
            "inv-1",
            reason="Test",
            web_session_id="web-session-1",
            user_id="user-1",
            case_id="case-test-1",
            vsod_event_service=vsod_event_service,
        )
        await asyncio.sleep(0.01)

        assert task1.cancelled()
        assert not task2.cancelled()
        assert "inv-2" in manager._active_tasks

    async def test_task_lock_prevents_race_conditions(self, manager, task_tracker):
        investigation_id = "inv-race"

        async def dummy():
            await asyncio.sleep(0.05)

        track_tasks = []
        for _ in range(5):
            task = task_tracker.track(asyncio.create_task(dummy()))
            track_tasks.append(task_tracker.track(asyncio.create_task(
                manager.track(investigation_id, task, auto_cancel_previous=True)
            )))

        await asyncio.gather(*track_tasks)

        assert len([k for k in manager._active_tasks if k == investigation_id]) == 1
