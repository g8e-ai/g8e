# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed fake for DocumentServiceProtocol."""

import fnmatch
import json
from typing import Any

from app.constants import EventType
from app.models.base import G8eBaseModel
from app.models.cache import (
    BatchWriteOperation,
    CacheOperationResult,
    DocumentResult,
    FieldFilter,
    QueryResult,
)
from app.models.operators import CommandResultRecord
from app.services.protocols import DocumentServiceProtocol, KVServiceProtocol


class FakeKVService:
    """In-memory fake implementing KVServiceProtocol."""

    def __init__(self) -> None:
        self._store: dict[str, str] = {}

    async def get(self, key: str) -> str | None:
        return self._store.get(key)

    async def set(self, key: str, value: str, ex: int | None) -> bool:
        self._store[key] = value
        return True

    async def delete(self, *keys: str) -> int:
        removed = [k for k in keys if self._store.pop(k, None) is not None]
        return len(removed)

    async def get_json(self, key: str) -> object | None:
        raw = self._store.get(key)
        return json.loads(raw) if raw is not None else None

    async def set_json(self, key: str, value: object, ex: int | None = None) -> bool:
        self._store[key] = json.dumps(value)
        return True

    async def keys(self, pattern: str = "*") -> list[str]:
        return [k for k in self._store if fnmatch.fnmatch(k, pattern)]

    async def delete_pattern(self, pattern: str) -> int:
        return await self.delete(*await self.keys(pattern))

    def is_healthy(self) -> bool:
        return True


class FakeDBService:
    """Typed fake implementing DocumentServiceProtocol.

    Records all calls for assertion in tests. Does not perform any real I/O.
    """

    def __init__(self) -> None:
        self.operator_activities: list[dict[str, object]] = []
        self.chat_messages: list[dict[str, object]] = []
        self.command_results: list[dict[str, object]] = []
        self._kv = FakeKVService()

    @property
    def kv(self) -> KVServiceProtocol:
        return self._kv

    @property
    def db(self) -> DocumentServiceProtocol:
        return self

    async def create_document(
        self,
        collection: str,
        document_id: str,
        data: dict[str, Any] | G8eBaseModel,
    ) -> CacheOperationResult:
        return CacheOperationResult(success=True)

    async def update_document(
        self,
        collection: str,
        document_id: str,
        data: dict[str, Any] | G8eBaseModel,
        merge: bool = True,
    ) -> CacheOperationResult:
        return CacheOperationResult(success=True)

    async def get_document(self, collection: str, document_id: str) -> DocumentResult:
        return DocumentResult(success=True)

    async def delete_document(self, collection: str, document_id: str) -> CacheOperationResult:
        return CacheOperationResult(success=True)

    async def query_collection(
        self,
        collection: str,
        field_filters: list[FieldFilter],
        order_by: dict[str, str],
        limit: int,
        select_fields: list[str] | None = None,
        ttl: int | None = 300,
    ) -> QueryResult:
        return QueryResult(success=True, data=[])

    async def update_with_array_union(
        self,
        collection: str,
        document_id: str,
        array_field: str,
        items_to_add: list[Any],
        additional_updates: dict[str, Any],
    ) -> CacheOperationResult:
        return CacheOperationResult(success=True)

    async def batch_write(self, operations: list[BatchWriteOperation]) -> CacheOperationResult:
        return CacheOperationResult(success=True)

    async def get_document_with_cache(
        self, collection: str, document_id: str
    ) -> dict[str, Any] | None:
        return None

    async def query_documents(
        self,
        collection: str,
        field_filters: list[dict[str, Any]],
        order_by: dict[str, str] | None = None,
        limit: int = 100,
        select_fields: list[str] | None = None,
        ttl: int | None = 300,
        bypass_cache: bool = False,
    ) -> list[dict[str, Any]]:
        return []

    async def append_to_array(
        self,
        collection: str,
        document_id: str,
        array_field: str,
        items_to_add: list[Any],
        additional_updates: dict[str, Any],
    ) -> CacheOperationResult:
        return CacheOperationResult(success=True)

    async def invalidate_query_cache(self, collection: str) -> int:
        return 0

    async def close(self) -> None:
        pass

    async def add_operator_activity(
        self,
        operator_id: str,
        sender: EventType,
        content: str,
        metadata: object,
        investigation_id: str,
        case_id: str,
    ) -> None:
        self.operator_activities.append(
            {
                "operator_id": operator_id,
                "sender": sender,
                "content": content,
                "metadata": metadata,
                "investigation_id": investigation_id,
                "case_id": case_id,
            }
        )

    async def add_chat_message(
        self,
        investigation_id: str | None,
        sender: EventType,
        content: str,
        metadata: object,
    ) -> None:
        self.chat_messages.append(
            {
                "investigation_id": investigation_id,
                "sender": sender,
                "content": content,
                "metadata": metadata,
            }
        )

    async def append_command_result(
        self,
        operator_id: str,
        command_result: CommandResultRecord,
    ) -> None:
        self.command_results.append(
            {
                "operator_id": operator_id,
                "command_result": command_result,
            }
        )


_: DocumentServiceProtocol = FakeDBService()
