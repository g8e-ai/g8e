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

"""
Unit tests for async_mocks utility.
"""

import asyncio
import unittest.mock as mock

import pytest

from tests.utils.async_mocks import AsyncMockContext, SafeToThreadMock, SafeWaitForMock

pytestmark = [pytest.mark.unit]


class TestSafeToThreadMock:
    """SafeToThreadMock returns awaitable coroutines."""

    @pytest.mark.asyncio
    async def test_returns_coroutine(self):
        mock_obj = SafeToThreadMock(return_value="test_value")
        result = mock_obj(lambda: None)
        assert asyncio.iscoroutine(result)
        # Clean up the coroutine to avoid leak warning
        result.close()

    @pytest.mark.asyncio
    async def test_returns_configured_value(self):
        mock_obj = SafeToThreadMock(return_value="test_value")
        coro = mock_obj(lambda: None)
        result = await coro
        assert result == "test_value"

    @pytest.mark.asyncio
    async def test_side_effect_called_with_args(self):
        captured_args = []

        def side_effect(func, *args, **kwargs):
            captured_args.append((args, kwargs))
            return "side_effect_value"

        mock_obj = SafeToThreadMock(side_effect=side_effect)
        coro = mock_obj(lambda x: x, "arg1", kwarg="kwarg1")
        result = await coro

        assert result == "side_effect_value"
        assert captured_args[0] == (("arg1",), {"kwarg": "kwarg1"})

    @pytest.mark.asyncio
    async def test_side_effect_takes_precedence_over_return_value(self):
        def side_effect(func, *args, **kwargs):
            return "side_effect_value"

        mock_obj = SafeToThreadMock(return_value="default_value", side_effect=side_effect)
        coro = mock_obj(lambda: None)
        result = await coro

        assert result == "side_effect_value"


class TestSafeWaitForMock:
    """SafeWaitForMock always awaits coroutines to prevent leaks."""

    @pytest.mark.asyncio
    async def test_make_side_effect_returns_async_function(self):
        mock_obj = SafeWaitForMock()
        side_effect = mock_obj.make_side_effect()
        assert asyncio.iscoroutinefunction(side_effect)

    @pytest.mark.asyncio
    async def test_awaits_passed_coroutine(self):
        mock_obj = SafeWaitForMock()
        side_effect = mock_obj.make_side_effect()

        async def test_coro():
            return "coro_value"

        result = await side_effect(test_coro(), 1.0)
        assert result == "coro_value"

    @pytest.mark.asyncio
    async def test_custom_side_effect_called(self):
        call_count = 0

        async def custom_side_effect(coro, timeout):
            nonlocal call_count
            call_count += 1
            return "custom_value"

        mock_obj = SafeWaitForMock(side_effect=custom_side_effect)
        side_effect = mock_obj.make_side_effect()

        async def test_coro():
            return "coro_value"

        result = await side_effect(test_coro(), 1.0)
        assert result == "custom_value"
        assert call_count == 1

    @pytest.mark.asyncio
    async def test_custom_side_effect_can_raise(self):
        async def custom_side_effect(coro, timeout):
            raise asyncio.TimeoutError()

        mock_obj = SafeWaitForMock(side_effect=custom_side_effect)
        side_effect = mock_obj.make_side_effect()

        async def test_coro():
            return "coro_value"

        with pytest.raises(asyncio.TimeoutError):
            await side_effect(test_coro(), 1.0)

    @pytest.mark.asyncio
    async def test_cleanup_closes_unawaited_coroutines(self):
        mock_obj = SafeWaitForMock()
        side_effect = mock_obj.make_side_effect()

        async def test_coro():
            return "coro_value"

        # Create a coroutine but don't await it
        coro = test_coro()
        await side_effect(coro, 1.0)

        # The coroutine should be captured
        assert len(mock_obj._captured_coros) == 1

        # Cleanup should close it
        mock_obj.cleanup()
        assert len(mock_obj._captured_coros) == 0


class TestAsyncMockContext:
    """AsyncMockContext combines SafeToThreadMock and SafeWaitForMock."""

    @pytest.mark.asyncio
    async def test_creates_both_mocks(self):
        ctx = AsyncMockContext(
            to_thread_return_value="thread_value",
        )

        assert ctx.to_thread_mock is not None
        assert ctx.wait_for_mock is not None

    @pytest.mark.asyncio
    async def test_to_thread_mock_returns_coroutine(self):
        ctx = AsyncMockContext(to_thread_return_value="test_value")
        result = ctx.to_thread_mock(lambda: None)
        assert asyncio.iscoroutine(result)
        # Clean up the coroutine to avoid leak warning
        result.close()

    @pytest.mark.asyncio
    async def test_wait_for_mock_creates_side_effect(self):
        ctx = AsyncMockContext()
        side_effect = ctx.wait_for_mock.make_side_effect()
        assert asyncio.iscoroutinefunction(side_effect)

    @pytest.mark.asyncio
    async def test_context_manager_calls_cleanup(self):
        ctx = AsyncMockContext()

        async def test_coro():
            return "value"

        # Add a coroutine to the captured list
        side_effect = ctx.wait_for_mock.make_side_effect()
        await side_effect(test_coro(), 1.0)

        assert len(ctx.wait_for_mock._captured_coros) == 1

        # Exit context should cleanup
        with ctx:
            pass

        assert len(ctx.wait_for_mock._captured_coros) == 0
