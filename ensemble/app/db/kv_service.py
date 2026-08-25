# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import logging

from app.clients.kv_cache_client import KVCacheClient

logger = logging.getLogger(__name__)


class KVService:
    """Authoritative key-value service. The sole user of KVCacheClient."""

    def __init__(self, client: KVCacheClient):
        self.client = client

    async def get(self, key: str) -> str | None:
        return await self.client.get(key)

    async def set(self, key: str, value: str, ex: int | None) -> bool:
        return await self.client.set(key, value, ex=ex)

    async def delete(self, *keys: str) -> int:
        return await self.client.delete(*keys)

    async def get_json(self, key: str) -> object | None:
        return await self.client.get_json(key)

    async def set_json(self, key: str, value: object, ex: int | None = None) -> bool:
        return await self.client.set_json(key, value, ex=ex)

    async def keys(self, pattern: str) -> list[str]:
        return await self.client.keys(pattern)

    async def delete_pattern(self, pattern: str) -> int:
        return await self.client.delete_pattern(pattern)

    async def lrange(self, key: str, start: int, stop: int) -> list[object]:
        return await self.client.lrange(key, start, stop)

    async def incr(self, key: str, amount: int = 1) -> int:
        return await self.client.incr(key, amount)

    async def decr(self, key: str, amount: int = 1) -> int:
        return await self.client.decr(key, amount)

    def is_healthy(self) -> bool:
        return self.client.is_healthy()
