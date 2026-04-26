import asyncio
from contextlib import asynccontextmanager

class KeyedAsyncLock:
    """Per-key asyncio mutual exclusion.

    Guarantees that for any given key, only one coroutine holds the lock at
    a time. Locks are created lazily and reaped when no waiters remain so
    long-lived services do not leak entries for transient keys.

    Single-process / single-event-loop only. If g8ee ever runs with multiple
    workers, replace with a g8es-backed distributed lock (see followups doc).
    """

    def __init__(self) -> None:
        self._locks: dict[str, asyncio.Lock] = {}
        self._refcounts: dict[str, int] = {}
        self._guard = asyncio.Lock()

    @asynccontextmanager
    async def acquire(self, key: str):
        async with self._guard:
            lock = self._locks.get(key)
            if lock is None:
                lock = asyncio.Lock()
                self._locks[key] = lock
            self._refcounts[key] = self._refcounts.get(key, 0) + 1
        try:
            async with lock:
                yield
        finally:
            async with self._guard:
                self._refcounts[key] -= 1
                if self._refcounts[key] == 0:
                    del self._refcounts[key]
                    del self._locks[key]
