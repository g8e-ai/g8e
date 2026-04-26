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

import hashlib
import json
import logging
from typing import Any, cast
from app.db.db_service import DBService
from app.db.kv_service import KVService
from app.services.protocols import CacheAsideProtocol, KVServiceProtocol, DBServiceProtocol
from app.constants import (
    CACHE_TTL_DEFAULT,
    CACHE_TTL_LONG,
    CACHE_TTL_MEDIUM,
    CACHE_TTL_ORGS,
    CACHE_TTL_SHORT,
    DB_COLLECTION_API_KEYS,
    DB_COLLECTION_CASES,
    DB_COLLECTION_SETTINGS,
    DB_COLLECTION_INVESTIGATIONS,
    DB_COLLECTION_MEMORIES,
    DB_COLLECTION_OPERATORS,
    DB_COLLECTION_OPERATOR_SESSIONS,
    DB_COLLECTION_ORGANIZATIONS,
    DB_COLLECTION_REPUTATION_COMMITMENTS,
    DB_COLLECTION_REPUTATION_STATE,
    DB_COLLECTION_WEB_SESSIONS,
    DB_COLLECTION_USERS,
    BatchWriteOpType,
    ComponentName,
    ErrorCode,
    KVKey,
    KVKeyPrefix,
)

from app.errors import DatabaseError
from app.models.base import recursive_serialize, G8eBaseModel
from app.models.cache import (
    BatchCreateDocumentOperation,
    BatchOperationResult,
    BatchWriteOperation,
    CacheOperationResult,
    CacheStats,
    FieldFilter,
)

logger = logging.getLogger(__name__)

TTL_STRATEGIES = {
    DB_COLLECTION_WEB_SESSIONS: None,
    DB_COLLECTION_OPERATOR_SESSIONS: None,
    DB_COLLECTION_USERS: CACHE_TTL_DEFAULT,
    DB_COLLECTION_API_KEYS: CACHE_TTL_LONG,
    DB_COLLECTION_CASES: CACHE_TTL_MEDIUM,
    DB_COLLECTION_INVESTIGATIONS: CACHE_TTL_MEDIUM,
    DB_COLLECTION_MEMORIES: CACHE_TTL_MEDIUM,
    DB_COLLECTION_ORGANIZATIONS: CACHE_TTL_ORGS,
    DB_COLLECTION_SETTINGS: CACHE_TTL_DEFAULT,
    DB_COLLECTION_OPERATORS: CACHE_TTL_MEDIUM,
    # Reputation: state is read every Auditor verdict (cache it long); commitments are
    # append-only and consulted only by the Auditor's prev_root lookup and audit replay
    # — the per-document cache adds no value, so use SHORT to keep stale reads bounded.
    DB_COLLECTION_REPUTATION_STATE: CACHE_TTL_LONG,
    DB_COLLECTION_REPUTATION_COMMITMENTS: CACHE_TTL_SHORT,
}

class CacheAsideService(CacheAsideProtocol):
    """
    Cache-aside pattern: DB-first writes, KV cache for reads.

    Create/Update/Delete: write to DB first, then invalidate or populate cache.
    Read: check KV cache first; on miss, read from DB and populate cache.
    """

    def __init__(
        self,
        kv: KVService,
        db: DBService,
        component_name: ComponentName = ComponentName.G8EE,
        default_ttl: int = CACHE_TTL_DEFAULT,
    ):
        self._kv = kv
        self._db = db
        self.component_name = component_name
        self.default_ttl = default_ttl

        logger.info(
            f"[{component_name.upper()}-CACHE] Cache-aside service initialized",
            extra={"default_ttl": default_ttl}
        )

    @property
    def kv(self) -> KVServiceProtocol:
        """Access the underlying KV service."""
        return cast(KVServiceProtocol, self._kv)

    async def close(self) -> None:
        """Close underlying KV and DB HTTP clients to prevent resource leaks."""
        try:
            if hasattr(self._kv, 'close'):
                await self._kv.close()  # type: ignore[attr-defined]
        except Exception as exc:
            logger.info("Error closing KV service: %s", exc)
        try:
            if hasattr(self._db, 'close'):
                await self._db.close()  # type: ignore[attr-defined]
        except Exception as exc:
            logger.info("Error closing DB service: %s", exc)

    @property
    def db(self) -> DBServiceProtocol:
        """Access the underlying DB service."""
        return self._db

    def _make_key(self, collection: str, document_id: str) -> str:
        return KVKey.doc(collection, document_id)

    def _get_ttl_for_collection(self, collection: str) -> int | None:
        return TTL_STRATEGIES.get(collection, self.default_ttl)

    async def create_document(
        self,
        collection: str,
        document_id: str,
        data: dict[str, Any] | G8eBaseModel,
        ttl: int | None = None
    ) -> CacheOperationResult:
        # Check if document exists first to ensure "Create" fails if it exists
        existing = await self.db.get_document(collection, document_id)
        if existing.success and existing.data is not None:
            raise DatabaseError(
                f"Document {document_id} already exists in {collection}",
                code=ErrorCode.DB_WRITE_ERROR,
                component=self.component_name,
            )

        result = await self.db.create_document(
            collection=collection,
            data=data,
            document_id=document_id
        )

        if not result.success:
            raise DatabaseError(
                f"Failed to create document {document_id} in {collection}: {result.error or 'unknown error'}",
                code=ErrorCode.DB_WRITE_ERROR,
                component=self.component_name,
            )

        logger.info(
            f"[{self.component_name.upper()}-CACHE] Document created in database",
            extra={"collection": collection, "doc_id": document_id, "operation": "create"}
        )

        # Invalidate cache instead of populating it
        key = self._make_key(collection, document_id)
        await self.kv.delete(key)
        await self.invalidate_query_cache(collection)

        logger.info(
            f"[{self.component_name.upper()}-CACHE] Cache invalidated for new document",
            extra={"collection": collection, "doc_id": document_id}
        )

        return CacheOperationResult(
            success=True,
            document_id=document_id,
            cache_invalidated=True
        )

    async def update_document(
        self,
        collection: str,
        document_id: str,
        data: dict[str, Any] | G8eBaseModel,
        merge: bool = True,
        ttl: int | None = None
    ) -> CacheOperationResult:
        result = await self.db.update_document(
            collection=collection,
            document_id=document_id,
            data=data,
            merge=merge
        )

        if not result.success:
            raise DatabaseError(
                f"Failed to update document {document_id} in {collection}: {result.error or 'unknown error'}",
                code=ErrorCode.DB_WRITE_ERROR,
                component=self.component_name,
            )

        logger.info(
            f"[{self.component_name.upper()}-CACHE] Document updated in database",
            extra={"collection": collection, "doc_id": document_id, "merge": merge}
        )

        key = self._make_key(collection, document_id)
        await self.kv.delete(key)
        await self.invalidate_query_cache(collection)

        logger.info(
            f"[{self.component_name.upper()}-CACHE] Cache invalidated",
            extra={"collection": collection, "doc_id": document_id}
        )

        return CacheOperationResult(
            success=True,
            document_id=document_id,
            cache_invalidated=True
        )

    async def delete_document(
        self,
        collection: str,
        document_id: str
    ) -> CacheOperationResult:
        result = await self.db.delete_document(
            collection=collection,
            document_id=document_id
        )

        if not result.success:
            raise DatabaseError(
                f"Failed to delete document {document_id} in {collection}: {result.error or 'unknown error'}",
                code=ErrorCode.DB_WRITE_ERROR,
                component=self.component_name,
            )

        logger.info(
            f"[{self.component_name.upper()}-CACHE] Document deleted from database",
            extra={"collection": collection, "doc_id": document_id}
        )

        key = self._make_key(collection, document_id)
        await self.kv.delete(key)
        await self.invalidate_query_cache(collection)

        logger.info(
            f"[{self.component_name.upper()}-CACHE] Document deleted from cache",
            extra={"collection": collection, "doc_id": document_id}
        )

        return CacheOperationResult(
            success=True,
            document_id=document_id
        )

    async def get_document(
        self,
        collection: str,
        document_id: str
    ) -> dict[str, Any] | None:
        key = self._make_key(collection, document_id)

        cached_data: object | None = await self.kv.get_json(key)
        if isinstance(cached_data, dict):
            logger.info(
                f"[{self.component_name.upper()}-CACHE] Cache HIT",
                extra={"collection": collection, "doc_id": document_id}
            )
            return cast(dict[str, Any], cached_data)

        logger.info(
            f"[{self.component_name.upper()}-CACHE] Cache MISS - reading from database",
            extra={"collection": collection, "doc_id": document_id}
        )

        db_response = await self.db.get_document(
            collection=collection,
            document_id=document_id
        )

        if not db_response.success or db_response.data is None:
            logger.info(
                f"[{self.component_name.upper()}-CACHE] Document not found in database",
                extra={"collection": collection, "doc_id": document_id}
            )
            return None

        db_data = recursive_serialize(db_response.data)

        ttl = self._get_ttl_for_collection(collection)
        cache_success = await self.kv.set_json(key, db_data, ex=ttl)

        if cache_success:
            logger.info(
                f"[{self.component_name.upper()}-CACHE] Cache warmed from database",
                extra={"collection": collection, "doc_id": document_id, "ttl": ttl}
            )

        return db_data

    async def cache_document(
        self,
        collection: str,
        document_id: str,
        data: dict[str, object],
        ttl: int | None = None,
    ) -> bool:
        """Write data directly to the KV cache without touching the DB."""
        key = self._make_key(collection, document_id)
        resolved_ttl = ttl if ttl is not None else self._get_ttl_for_collection(collection)
        return await self.kv.set_json(key, data, ex=resolved_ttl)

    async def invalidate_collection(self, collection: str) -> int:
        """Delete all document cache keys for a collection. Returns count deleted."""
        pattern = f"{KVKeyPrefix.CACHE_DOC}{collection}:*"
        return await self.kv.delete_pattern(pattern)

    async def clear_all(self) -> int:
        """Delete all document and query cache keys. Returns total count deleted."""
        doc_deleted = await self.kv.delete_pattern(f"{KVKeyPrefix.CACHE_DOC}*")
        query_deleted = await self.kv.delete_pattern(f"{KVKeyPrefix.CACHE_QUERY}*")
        return doc_deleted + query_deleted

    async def get_query_result(
        self,
        collection: str,
        query_params: dict[str, Any],
        ttl: int | None = CACHE_TTL_SHORT,
    ) -> list[dict[str, Any]] | None:
        query_str = json.dumps(query_params, sort_keys=True)
        filter_hash = hashlib.md5(query_str.encode()).hexdigest()
        key = KVKey.query(collection, filter_hash)

        cached_data: object | None = await self.kv.get_json(key)
        if isinstance(cached_data, list):
            result = cast(list[dict[str, Any]], cached_data)
            logger.info(
                f"[{self.component_name.upper()}-CACHE] Query cache HIT",
                extra={"collection": collection, "result_count": len(result)}
            )
            return result

        logger.info(
            f"[{self.component_name.upper()}-CACHE] Query cache MISS",
            extra={"collection": collection}
        )

        return None

    async def set_query_result(
        self,
        collection: str,
        query_params: dict[str, Any],
        results: list[dict[str, Any]],
        ttl: int | None = CACHE_TTL_SHORT
    ) -> bool:
        query_str = json.dumps(query_params, sort_keys=True)
        filter_hash = hashlib.md5(query_str.encode()).hexdigest()
        key = KVKey.query(collection, filter_hash)

        success = await self.kv.set_json(key, results, ex=ttl)
        if success:
            logger.info(
                f"[{self.component_name.upper()}-CACHE] Query results cached",
                extra={"collection": collection, "result_count": len(results), "ttl": ttl}
            )
        return success

    async def query_documents(
        self,
        collection: str,
        field_filters: list[dict[str, Any]],
        order_by: dict[str, str] | None = None,
        limit: int = 100,
        select_fields: list[str] | None = None,
        ttl: int | None = CACHE_TTL_SHORT,
        bypass_cache: bool = False,
    ) -> list[dict[str, Any]]:
        order_by = order_by or {}
        query_params: dict[str, Any] = {
            "collection": collection,
            "filters": field_filters,
            "order_by": order_by,
            "limit": limit,
            "select_fields": select_fields or [],
        }
        if not bypass_cache:
            cached = await self.get_query_result(collection, query_params, ttl=ttl)
            if cached is not None:
                return cached

        result = await self.db.query_collection(
            collection=collection,
            field_filters=[FieldFilter(**f) for f in field_filters],
            order_by=order_by,
            limit=limit,
            select_fields=select_fields or [],
        )

        data: list[dict[str, Any]] = result.data if result.success else []
        if not bypass_cache:
            await self.set_query_result(collection, query_params, data, ttl=ttl)

        logger.info(
            f"[{self.component_name.upper()}-CACHE] Query executed and cached",
            extra={"collection": collection, "result_count": len(data)}
        )
        return data

    async def invalidate_document(
        self,
        collection: str,
        document_id: str
    ) -> bool:
        key = self._make_key(collection, document_id)
        deleted = await self.kv.delete(key)
        if deleted:
            logger.info(
                f"[{self.component_name.upper()}-CACHE] Document cache invalidated",
                extra={"collection": collection, "doc_id": document_id}
            )
        return deleted > 0

    async def invalidate_query_cache(self, collection: str) -> int:
        pattern = f"{KVKeyPrefix.CACHE_QUERY}{collection}:*"
        deleted = await self.kv.delete_pattern(pattern)
        logger.info(
            f"[{self.component_name.upper()}-CACHE] Query cache invalidated",
            extra={"collection": collection, "keys_deleted": deleted}
        )
        return deleted

    async def append_to_array(
        self,
        collection: str,
        document_id: str,
        array_field: str,
        items_to_add: list[Any],
        additional_updates: dict[str, Any],
    ) -> CacheOperationResult:
        result = await self.db.update_with_array_union(
            collection=collection,
            document_id=document_id,
            array_field=array_field,
            items_to_add=items_to_add,
            additional_updates=additional_updates,
        )

        if not result.success:
            raise DatabaseError(
                f"Failed to append to {array_field} on {document_id} in {collection}: {result.error or 'unknown error'}",
                code=ErrorCode.DB_WRITE_ERROR,
                component=self.component_name,
            )

        key = self._make_key(collection, document_id)
        await self.kv.delete(key)
        await self.invalidate_query_cache(collection)

        logger.info(
            f"[{self.component_name.upper()}-CACHE] Array append completed, cache invalidated",
            extra={"collection": collection, "doc_id": document_id, "field": array_field}
        )

        return CacheOperationResult(
            success=True,
            document_id=document_id,
            cache_invalidated=True,
        )

    async def batch_create_documents(
        self,
        operations: list[BatchCreateDocumentOperation]
    ) -> BatchOperationResult:
        if not operations:
            return BatchOperationResult(success=True, count=0)

        db_operations = [
            BatchWriteOperation(
                op_type=BatchWriteOpType.SET,
                collection=op.collection,
                doc_id=op.document_id,
                data=op.data,
                merge=False,
            )
            for op in operations
        ]

        result = await self.db.batch_write(db_operations)

        if not result.success:
            raise DatabaseError(
                f"Batch write failed: {result.error or 'unknown error'}",
                code=ErrorCode.DB_WRITE_ERROR,
                component=self.component_name,
            )

        logger.info(
            f"[{self.component_name.upper()}-CACHE] Batch write completed",
            extra={"operation_count": len(operations)}
        )

        collections_touched: set[str] = set()
        for op in operations:
            key = self._make_key(op.collection, op.document_id)
            await self.kv.delete(key)
            collections_touched.add(op.collection)

        for collection in collections_touched:
            await self.invalidate_query_cache(collection)

        logger.info(
            f"[{self.component_name.upper()}-CACHE] KV batch cache invalidated",
            extra={"operation_count": len(operations)}
        )

        return BatchOperationResult(success=True, count=len(operations))

    async def get_stats(self) -> CacheStats:
        """Return a snapshot of current cache statistics."""
        try:
            doc_keys = await self.kv.keys(f"{KVKeyPrefix.CACHE_DOC}*")
            query_keys = await self.kv.keys(f"{KVKeyPrefix.CACHE_QUERY}*")
            doc_count = len(doc_keys)
            query_count = len(query_keys)
            return CacheStats(
                enabled=True,
                healthy=self.kv.is_healthy(),
                document_keys=doc_count,
                query_keys=query_count,
                total_keys=doc_count + query_count,
                default_ttl=self.default_ttl,
            )
        except Exception as exc:
            return CacheStats(
                enabled=True,
                healthy=False,
                default_ttl=self.default_ttl,
                error=str(exc),
            )
