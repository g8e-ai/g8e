# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed fakes for operator DB and PubSub clients."""

from unittest.mock import AsyncMock, MagicMock

from app.clients.http_client import HTTPClient
from app.models.cache import BatchWriteOperation, CacheOperationResult, DocumentResult, QueryResult


class FakePubSubClient:
    """In-memory fake for operator Pub/Sub client.

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
    """In-memory fake for operator DB client."""

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

    async def _create_document(
        self, collection: str, data: dict, document_id: str, **kwargs
    ) -> CacheOperationResult:
        self._col(collection)[document_id] = dict(data)
        return CacheOperationResult(success=True, document_id=document_id)

    async def _get_document(self, collection: str, document_id: str, **kwargs) -> DocumentResult:
        data = self._col(collection).get(document_id)
        if data is None:
            return DocumentResult(success=True, data=None)
        return DocumentResult(success=True, data=dict(data))

    async def _update_document(
        self, collection: str, document_id: str, data: dict, merge: bool = True, **kwargs
    ) -> CacheOperationResult:
        col = self._col(collection)
        if merge and document_id in col:
            col[document_id].update(data)
        else:
            col[document_id] = dict(data)
        return CacheOperationResult(success=True, document_id=document_id)

    async def _delete_document(
        self, collection: str, document_id: str, **kwargs
    ) -> CacheOperationResult:
        self._col(collection).pop(document_id, None)
        return CacheOperationResult(success=True, document_id=document_id)

    async def _query_collection(
        self,
        collection: str,
        field_filters=None,
        order_by=None,
        limit=100,
        select_fields=None,
        **kwargs,
    ) -> QueryResult:
        docs = list(self._col(collection).values())
        for raw_filter in field_filters or []:
            field = (
                raw_filter.get("field")
                if isinstance(raw_filter, dict)
                else getattr(raw_filter, "field", None)
            )
            op = (
                raw_filter.get("op")
                if isinstance(raw_filter, dict)
                else getattr(raw_filter, "op", None)
            )
            value = (
                raw_filter.get("value")
                if isinstance(raw_filter, dict)
                else getattr(raw_filter, "value", None)
            )
            if op == "==":
                docs = [doc for doc in docs if doc.get(field) == value]
        for field, direction in reversed(list((order_by or {}).items())):
            reverse = str(direction).lower() == "desc"
            docs.sort(key=lambda doc: doc.get(field) or "", reverse=reverse)
        if limit:
            docs = docs[:limit]
        if select_fields:
            docs = [
                {field: doc.get(field) for field in select_fields if field in doc} for doc in docs
            ]
        return QueryResult(success=True, data=[dict(d) for d in docs])

    async def _update_with_array_union(
        self, collection: str, document_id: str, array_field: str, items_to_add: list, **kwargs
    ) -> CacheOperationResult:
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


class FakeG8eClient:
    """In-memory fake for g8e operator HTTP client."""

    def __init__(self):
        self.client: HTTPClient = MagicMock(spec=HTTPClient)
        self.push_sse_event = AsyncMock()
        self.grant_intent = AsyncMock()
        self.revoke_intent = AsyncMock()
        self.push_agent_state = AsyncMock()
        self.push_run_state = AsyncMock()
        self.dispatch_inference = AsyncMock()
        self.get = AsyncMock()
        self.post = AsyncMock()
        self.put = AsyncMock()
        self.delete = AsyncMock()
        self.close = AsyncMock()

    def ensure_mtls(self) -> None:
        return None

    def is_healthy(self):
        return True
