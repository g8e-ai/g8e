# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Async iteration helpers for tests.

Tests that exercise code paths containing ``async for`` must provide iterators
that correctly implement the async iterator protocol (``__aiter__`` returning
an object with ``__anext__``). ``MagicMock(return_value=iter([...]))`` does
not satisfy this - a plain ``list_iterator`` lacks ``__anext__`` and
``async for`` raises ``TypeError`` before any loop body executes.

Use ``async_iter(...)`` to wrap a sequence of frames in a real async
generator when mocking aiohttp ``ClientWebSocketResponse`` or similar
async-iterable transports.
"""

from collections.abc import AsyncIterator, Iterable
from typing import TypeVar

T = TypeVar("T")


def async_iter(frames: Iterable[T]) -> AsyncIterator[T]:
    """Return an async iterator over ``frames`` suitable for ``async for``."""

    async def _gen() -> AsyncIterator[T]:
        for frame in frames:
            yield frame

    return _gen()
