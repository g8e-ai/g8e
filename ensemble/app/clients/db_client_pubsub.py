# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
PubSubDBClient - operator Document Store client using pubsub.

Replaces HTTP-based DBClient with pubsub-based communication.
All operations are sent to g8eo via pubsub channels.
"""

import logging
from typing import Any

from app.clients.pubsub_client import PubSubClient
from app.constants.channels import OperatorChannel
from app.errors import (
    DatabaseError,
    ErrorCode,
    NetworkError,
    ResourceNotFoundError,
    ValidationError,
)
from app.models.cache import (
    ArrayRemove,
    ArrayUnion,
    BatchWriteOperation,
    CacheOperationResult,
    DocumentResult,
    QueryResult,
)

logger = logging.getLogger(__name__)


class PubSubDBClient:
    """Pubsub-based operator Document Store client."""

    def __init__(
        self,
        pubsub_client: PubSubClient,
        operator_id: str,
        operator_session_id: str,
    ) -> None:
        self._pubsub_client = pubsub_client
        self._operator_id = operator_id
        self._operator_session_id = operator_session_id

    async def connect(self) -> bool:
        """Verify connectivity to the operator via pubsub."""
        try:
            return await self._pubsub_client.check_operator_online(
                self._operator_id,
                self._operator_session_id,
            )
        except Exception as e:
            logger.error("[PUBSUB-DB-CLIENT] Connection check failed: %s", e)
            return False

    async def close(self) -> None:
        """Close the pubsub client."""
        await self._pubsub_client.close()

    async def _storage_request(
        self,
        operation: str,
        payload: dict[str, Any],
    ) -> dict[str, Any]:
        """Execute a storage request via pubsub."""
        try:
            response = await self._pubsub_client.publish_storage_request(
                operator_id=self._operator_id,
                operator_session_id=self._operator_session_id,
                storage_type=OperatorChannel.STORAGE_DOCUMENT,
                operation=operation,
                payload=payload,
            )

            if response.get("success"):
                return response.get("data", {})
            error = response.get("error", "Unknown error")
            raise NetworkError(f"Storage operation failed: {error}", component="g8ee")
        except NetworkError:
            raise
        except Exception as e:
            raise DatabaseError(
                f"Storage request failed: {e}",
                code=ErrorCode.DB_QUERY_ERROR,
                component="g8ee",
                cause=e,
            ) from e

    async def create_document(
        self,
        collection: str,
        document_id: str,
        data: dict[str, object],
    ) -> CacheOperationResult:
        try:
            payload = {
                "collection": collection,
                "document_id": document_id,
                "data": data,
            }
            await self._storage_request("create", payload)
            return CacheOperationResult(success=True, document_id=document_id)
        except (NetworkError, DatabaseError):
            raise
        except Exception as e:
            raise DatabaseError(
                f"create_document failed for {collection}/{document_id}: {e}",
                code=ErrorCode.DB_WRITE_ERROR,
                component="g8ee",
                cause=e,
            ) from e

    async def get_document(
        self,
        collection: str,
        document_id: str,
    ) -> DocumentResult:
        try:
            payload = {
                "collection": collection,
                "document_id": document_id,
            }
            response = await self._storage_request("get", payload)
            return DocumentResult(success=True, data=response.get("document"))
        except NetworkError:
            raise
        except Exception as e:
            raise DatabaseError(
                f"get_document failed for {collection}/{document_id}: {e}",
                code=ErrorCode.DB_QUERY_ERROR,
                component="g8ee",
                cause=e,
            ) from e

    async def update_document(
        self,
        collection: str,
        document_id: str,
        data: dict[str, object],
        merge: bool = True,
    ) -> CacheOperationResult:
        """Update (merge) or replace a document."""
        try:
            array_ops = {k: v for k, v in data.items() if isinstance(v, (ArrayUnion, ArrayRemove))}

            if array_ops:
                existing_result = await self.get_document(collection, document_id)
                existing = existing_result.data or {}
                patch: dict[str, object] = {k: v for k, v in data.items() if k not in array_ops}
                for k, op in array_ops.items():
                    raw = existing.get(k)
                    current: list[object] = raw if isinstance(raw, list) else []
                    if isinstance(op, ArrayUnion):
                        merged = current + op.values
                        patch[k] = merged[-op.max_length :] if op.max_length is not None else merged
                    else:
                        patch[k] = [item for item in current if item not in op.values]
                data = patch

            payload = {
                "collection": collection,
                "document_id": document_id,
                "data": data,
                "merge": merge,
            }
            await self._storage_request("update", payload)
            return CacheOperationResult(success=True, document_id=document_id)
        except (NetworkError, DatabaseError):
            raise
        except Exception as e:
            raise DatabaseError(
                f"update_document failed for {collection}/{document_id}: {e}",
                code=ErrorCode.DB_WRITE_ERROR,
                component="g8ee",
                cause=e,
            ) from e

    async def delete_document(
        self,
        collection: str,
        document_id: str,
    ) -> CacheOperationResult:
        try:
            payload = {
                "collection": collection,
                "document_id": document_id,
            }
            await self._storage_request("delete", payload)
            return CacheOperationResult(success=True, document_id=document_id)
        except NetworkError:
            raise
        except Exception as e:
            raise DatabaseError(
                f"delete_document failed for {collection}/{document_id}: {e}",
                code=ErrorCode.DB_WRITE_ERROR,
                component="g8ee",
                cause=e,
            ) from e

    async def query_collection(
        self,
        collection: str,
        field_filters: list[dict[str, object]],
        order_by: dict[str, str],
        limit: int,
        select_fields: list[str],
    ) -> QueryResult:
        """Query documents in a collection."""
        try:
            body: dict[str, Any] = {}
            if field_filters:
                body["filters"] = field_filters
            if order_by:
                field, direction = next(iter(order_by.items()))
                body["order_by"] = f"{field} {direction.upper()}"
            if limit:
                body["limit"] = limit
            if select_fields:
                body["select_fields"] = select_fields

            payload = {
                "collection": collection,
                "query": body,
            }
            response = await self._storage_request("query", payload)
            docs = response.get("documents", [])
            return QueryResult(success=True, data=docs)
        except (NetworkError, DatabaseError):
            raise
        except Exception as e:
            raise DatabaseError(
                f"query_collection failed for {collection}: {e}",
                code=ErrorCode.DB_QUERY_ERROR,
                component="g8ee",
                cause=e,
            ) from e

    async def count_documents(
        self,
        collection: str,
        field_filters: list[dict[str, object]],
    ) -> int:
        result = await self.query_collection(
            collection,
            field_filters=field_filters,
            order_by={},
            limit=0,
            select_fields=[],
        )
        return len(result.data)

    async def update_with_array_union(
        self,
        collection: str,
        document_id: str,
        array_field: str,
        items_to_add: list[object],
        additional_updates: dict[str, object],
    ) -> CacheOperationResult:
        """Append items to a list field in a document."""
        existing = await self.get_document(collection, document_id)
        if existing.data is None:
            raise ResourceNotFoundError(
                message=f"Document {document_id} not found in collection {collection}",
                resource_type=collection,
                resource_id=document_id,
            )

        raw = existing.data.get(array_field)
        current: list[object] = raw if isinstance(raw, list) else []
        merged = current + list(items_to_add)

        update_data: dict[str, object] = {array_field: merged}
        if additional_updates:
            update_data.update(additional_updates)

        return await self.update_document(collection, document_id, update_data)

    async def batch_write(
        self,
        operations: list[BatchWriteOperation],
    ) -> CacheOperationResult:
        for op in operations:
            if op.op_type == BatchWriteOpType.SET:
                result = await self.create_document(op.collection, op.doc_id, op.data)
            elif op.op_type == BatchWriteOpType.UPDATE:
                result = await self.update_document(
                    op.collection, op.doc_id, op.data, merge=op.merge
                )
            elif op.op_type == BatchWriteOpType.DELETE:
                result = await self.delete_document(op.collection, op.doc_id)
            else:
                raise ValidationError(
                    f"Unknown batch_write op type: {op.op_type!r}",
                    component="g8ee",
                )

            if not result.success:
                return result

        return CacheOperationResult(success=True)
