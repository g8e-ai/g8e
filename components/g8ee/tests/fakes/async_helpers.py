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

"""Async iteration helpers for tests.

Tests that exercise code paths containing ``async for`` must provide iterators
that correctly implement the async iterator protocol (``__aiter__`` returning
an object with ``__anext__``). ``MagicMock(return_value=iter([...]))`` does
not satisfy this — a plain ``list_iterator`` lacks ``__anext__`` and
``async for`` raises ``TypeError`` before any loop body executes.

Use ``async_iter(...)`` to wrap a sequence of frames in a real async
generator when mocking aiohttp ``ClientWebSocketResponse`` or similar
async-iterable transports.
"""

from typing import AsyncIterator, Iterable, TypeVar

T = TypeVar("T")


def async_iter(frames: Iterable[T]) -> AsyncIterator[T]:
    """Return an async iterator over ``frames`` suitable for ``async for``."""

    async def _gen() -> AsyncIterator[T]:
        for frame in frames:
            yield frame

    return _gen()
