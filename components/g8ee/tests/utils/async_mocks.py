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
Standard AsyncMock utilities for asyncio.to_thread and asyncio.wait_for.

When patching low-level asyncio functions like asyncio.to_thread and
asyncio.wait_for in tests, care must be taken to avoid coroutine leaks:
coroutines created by mocks must be awaited or closed to prevent
RuntimeWarning: "coroutine was never awaited".

This module provides safe, reusable mock utilities that handle this
cleanup automatically.
"""

from typing import Any, Callable, Coroutine


class SafeWaitForMock:
    """Safe mock for asyncio.wait_for that always awaits the coroutine.

    When patching asyncio.wait_for in tests, the mock must await the
    coroutine passed to it to avoid leaks. This class provides an
    async side_effect function that does this automatically while allowing
    you to control the return value or raise exceptions.

    Usage:
        call_count = 0
        pager = _make_pager(results)

        async def wait_for_side_effect(coro, timeout):
            nonlocal call_count
            call_count += 1
            # Always await to prevent leaks
            try:
                await coro
            except Exception:
                pass

            if call_count == 1:
                raise asyncio.TimeoutError()
            return pager

        with mock.patch("path.to.asyncio.wait_for", side_effect=wait_for_side_effect):
            result = await function_under_test()

    With SafeWaitForMock:
        call_count = 0
        pager = _make_pager(results)

        safe_wait = SafeWaitForMock(
            side_effect=lambda coro, timeout: (
                raise asyncio.TimeoutError() if call_count == 1 else pager
            )
        )

        with mock.patch("path.to.asyncio.wait_for", side_effect=safe_wait.make_side_effect()):
            result = await function_under_test()
    """

    def __init__(
        self,
        side_effect: Callable[[Coroutine, float], Any] | None = None,
    ):
        """Initialize the safe wait_for mock.

        Args:
            side_effect: Optional async function that takes (coro, timeout) and
                        returns the value that wait_for should return. If not
                        provided, returns the result of awaiting the coroutine.
        """
        self._side_effect = side_effect
        self._captured_coros = []

    def make_side_effect(self) -> Callable[[Coroutine, float], Coroutine]:
        """Return an async side_effect function for patching asyncio.wait_for.

        The returned function always awaits the coroutine to prevent leaks,
        then applies the custom side_effect if provided.

        Returns:
            An async function suitable for use with mock.patch(side_effect=...)
        """

        async def _side_effect(coro: Coroutine, timeout: float) -> Any:
            self._captured_coros.append(coro)
            try:
                # Always await to prevent coroutine leaks
                result = await coro
            except Exception:
                # If the coroutine raises, that's fine - it's been awaited
                pass

            if self._side_effect:
                return await self._side_effect(coro, timeout)

            return result

        return _side_effect

    def cleanup(self):
        """Close any captured coroutines that weren't awaited."""
        for coro in self._captured_coros:
            try:
                coro.close()
            except Exception:
                pass
        self._captured_coros.clear()


class SafeToThreadMock:
    """Safe mock for asyncio.to_thread that returns awaitable coroutines.

    When patching asyncio.to_thread in tests, the mock must return a
    coroutine object (not a plain value) because the calling code awaits
    the result. This class provides a mock that returns proper coroutines
    while allowing you to control the return value.

    Usage:
        pager = _make_pager(results)

        with mock.patch("path.to.asyncio.to_thread", new=SafeToThreadMock(return_value=pager)):
            result = await provider.search("query")
    """

    def __init__(self, return_value: Any = None, side_effect: Callable | None = None):
        """Initialize the safe to_thread mock.

        Args:
            return_value: The value to return when the coroutine is awaited
            side_effect: Optional callable that takes (func, *args, **kwargs) and
                        returns the value to await. If provided, return_value is ignored.
        """
        self._return_value = return_value
        self._side_effect = side_effect

    def __call__(self, func: Callable, *args: Any, **kwargs: Any) -> Coroutine:
        """Return a coroutine that will yield the configured value.

        Args:
            func: The function that would be run in the thread pool
            *args: Positional arguments for the function
            **kwargs: Keyword arguments for the function

        Returns:
            A coroutine that yields the configured return_value
        """
        async def _mock_coro():
            if self._side_effect:
                return self._side_effect(func, *args, **kwargs)
            return self._return_value

        return _mock_coro()


class AsyncMockContext:
    """Context manager for safely patching asyncio functions.

    Combines SafeToThreadMock and SafeWaitForMock into a single
    context manager for common use cases like testing retry logic
    with asyncio.wait_for.

    Usage:
        call_count = 0
        pager = _make_pager(results)

        async def wait_for_side_effect(coro, timeout):
            nonlocal call_count
            call_count += 1
            try:
                await coro
            except Exception:
                pass
            if call_count == 1:
                raise asyncio.TimeoutError()
            return pager

        with AsyncMockContext(
            to_thread_return_value=pager,
            wait_for_side_effect=wait_for_side_effect,
        ) as ctx:
            with mock.patch("path.to.asyncio.to_thread", new=ctx.to_thread_mock):
                with mock.patch("path.to.asyncio.wait_for", side_effect=ctx.wait_for_mock.make_side_effect()):
                    result = await function_under_test()
    """

    def __init__(
        self,
        to_thread_return_value: Any = None,
        to_thread_side_effect: Callable | None = None,
        wait_for_side_effect: Callable[[Coroutine, float], Any] | None = None,
    ):
        """Initialize the async mock context.

        Args:
            to_thread_return_value: Value for SafeToThreadMock to return
            to_thread_side_effect: Optional side_effect for to_thread
            wait_for_side_effect: Optional async side_effect for wait_for
        """
        self.to_thread_mock = SafeToThreadMock(
            return_value=to_thread_return_value,
            side_effect=to_thread_side_effect,
        )
        self.wait_for_mock = SafeWaitForMock(side_effect=wait_for_side_effect)

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        self.wait_for_mock.cleanup()
