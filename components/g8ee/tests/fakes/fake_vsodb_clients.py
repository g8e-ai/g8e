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

"""Typed fakes for VSODB KV, DB, and PubSub clients."""

import fnmatch
import json
from typing import Any
from unittest.mock import AsyncMock, MagicMock

from app.models.cache import CacheOperationResult, DocumentResult, QueryResult, BatchWriteOperation

class FakeKVClient:
    """In-memory fake for VSODB KV client.
    
    Provides a real dict-backed store so tests can assert on actual stored
    values. Used by KVCacheClient.
    """

    def __init__(self):
        self._store: dict[str, str] = {}
        # Expose as AsyncMock for call assertions if needed, 
        # but implementation is real in-memory.
        self.get = AsyncMock(side_effect=self._get)
        self.set = AsyncMock(side_effect=self._set)
        self.get_json = AsyncMock(side_effect=self._get_json)
        self.set_json = AsyncMock(side_effect=self._set_json)
        self.delete = AsyncMock(side_effect=self._delete)
        self.exists = AsyncMock(side_effect=self._exists)
        self.ttl = AsyncMock(return_value=-1)
        self.setex = AsyncMock(side_effect=self._setex)
        self.expire = AsyncMock(return_value=True)
        self.ping = AsyncMock(return_value=True)
        self.keys = AsyncMock(side_effect=self._keys)
        self.delete_pattern = AsyncMock(side_effect=self._delete_pattern)
        self.hget = AsyncMock(return_value=None)
        self.hset = AsyncMock(return_value=1)
        self.hgetall = AsyncMock(return_value={})
        self.hdel = AsyncMock(return_value=1)
        self.rpush = AsyncMock(return_value=1)
        self.lpush = AsyncMock(return_value=1)
        self.lrange = AsyncMock(return_value=[])
        self.llen = AsyncMock(return_value=0)
        self.ltrim = AsyncMock(return_value=True)
        self.incr = AsyncMock(return_value=1)
        self.decr = AsyncMock(return_value=0)
        self.connect = AsyncMock()
        self.disconnect = AsyncMock()
        self.close = AsyncMock()
        
    def seed(self, key: str, value: str):
        self._store[key] = value

    def seed_json(self, key: str, data: Any):
        self._store[key] = json.dumps(data)

    async def _get(self, key: str):
        return self._store.get(key)

    async def _set(self, key: str, value: str, **kwargs):
        self._store[key] = value
        return True

    async def _get_json(self, key: str):
        raw = self._store.get(key)
        return json.loads(raw) if raw is not None else None

    async def _set_json(self, key: str, value: Any, **kwargs):
        self._store[key] = json.dumps(value)
        return True

    async def _delete(self, *keys: str):
        count = 0
        for k in keys:
            if k in self._store:
                del self._store[k]
                count += 1
        return count

    async def _exists(self, *keys: str):
        return sum(1 for k in keys if k in self._store)

    async def _keys(self, pattern: str) -> list[str]:
        # Simple glob to regex or fnmatch
        return [k for k in self._store if fnmatch.fnmatch(k, pattern)]

    async def _delete_pattern(self, pattern: str) -> int:
        keys_to_delete = await self._keys(pattern)
        return await self._delete(*keys_to_delete)

    async def _setex(self, key: str, seconds: int, value: str):
        self._store[key] = value
        return True

    def is_healthy(self):
        return True


class FakePubSubClient:
    """In-memory fake for VSODB Pub/Sub client.
    
    Used by PubSubClient.
    """

    def __init__(self):
        self.publish_command = AsyncMock(return_value=1)
        self.publish = AsyncMock(return_value=1)
        self.connect = AsyncMock()
        self.disconnect = AsyncMock()
        self.close = AsyncMock()
        self.ensure_connected = AsyncMock()
        
        # Pub/Sub methods
        self.on_channel_message = MagicMock()
        self.off_channel_message = MagicMock()
        self.on_disconnect = MagicMock()
        self.off_disconnect = MagicMock()
        self.subscribe = AsyncMock()
        self.unsubscribe = AsyncMock()
        self.psubscribe = AsyncMock()
        self.punsubscribe = AsyncMock()

    def is_healthy(self):
        return True


class FakeDBClient:
    """In-memory fake for VSODB DB client."""

    def __init__(self):
        self._store: dict[str, dict[str, dict]] = {}
        self.create_document = AsyncMock(side_effect=self._create_document)
        self.get_document = AsyncMock(side_effect=self._get_document)
        self.update_document = AsyncMock(side_effect=self._update_document)
        self.delete_document = AsyncMock(side_effect=self._delete_document)
        self.update_with_array_union = AsyncMock(side_effect=self._update_with_array_union)
        self.query_collection = AsyncMock(side_effect=self._query_collection)
        self.batch_write = AsyncMock(side_effect=self._batch_write)
        self.close = AsyncMock()

    def _col(self, collection: str) -> dict:
        if collection not in self._store:
            self._store[collection] = {}
        return self._store[collection]

    async def _create_document(self, collection: str, data: dict, document_id: str, **kwargs) -> CacheOperationResult:
        self._col(collection)[document_id] = dict(data)
        return CacheOperationResult(success=True, document_id=document_id)

    async def _get_document(self, collection: str, document_id: str, **kwargs) -> DocumentResult:
        data = self._col(collection).get(document_id)
        if data is None:
            return DocumentResult(success=True, data=None)
        return DocumentResult(success=True, data=dict(data))

    async def _update_document(self, collection: str, document_id: str, data: dict, merge: bool = True, **kwargs) -> CacheOperationResult:
        col = self._col(collection)
        if merge and document_id in col:
            col[document_id].update(data)
        else:
            col[document_id] = dict(data)
        return CacheOperationResult(success=True, document_id=document_id)

    async def _delete_document(self, collection: str, document_id: str, **kwargs) -> CacheOperationResult:
        self._col(collection).pop(document_id, None)
        return CacheOperationResult(success=True, document_id=document_id)

    async def _query_collection(self, collection: str, field_filters=None, **kwargs) -> QueryResult:
        docs = list(self._col(collection).values())
        # TODO: Implement basic filtering if needed
        return QueryResult(success=True, data=[dict(d) for d in docs])

    async def _update_with_array_union(self, collection: str, document_id: str, array_field: str, items_to_add: list, **kwargs) -> CacheOperationResult:
        col = self._col(collection)
        doc = col.setdefault(document_id, {})
        existing = doc.get(array_field, [])
        doc[array_field] = existing + items_to_add
        return CacheOperationResult(success=True, document_id=document_id)

    async def _batch_write(self, operations: list[BatchWriteOperation]) -> CacheOperationResult:
        for op in operations:
            # Simplified batch write for the fake
            await self._update_document(op.collection, op.doc_id, op.data, merge=op.merge)
        return CacheOperationResult(success=True)

    def is_healthy(self) -> bool:
        return True
