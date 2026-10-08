# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import logging
from typing import Any, Protocol

from app.constants import (
    G8EE_COMPONENT,
    BatchWriteOpType,
    ErrorCode,
)
from app.errors import DatabaseError
from app.models.base import G8eBaseModel, recursive_serialize
from app.models.cache import (
    BatchCreateDocumentOperation,
    BatchOperationResult,
    BatchWriteOperation,
    CacheOperationResult,
    DocumentResult,
    FieldFilter,
    QueryResult,
)
from app.services.protocols import DocumentServiceProtocol

logger = logging.getLogger(__name__)


class DBClientProtocol(Protocol):
    async def create_document(
        self, collection: str, document_id: str, data: dict[str, object]
    ) -> CacheOperationResult: ...

    async def get_document(self, collection: str, document_id: str) -> DocumentResult: ...

    async def update_document(
        self, collection: str, document_id: str, data: dict[str, object], merge: bool = True
    ) -> CacheOperationResult: ...

    async def delete_document(self, collection: str, document_id: str) -> CacheOperationResult: ...

    async def query_collection(
        self,
        collection: str,
        field_filters: list[dict[str, object]],
        order_by: dict[str, str],
        limit: int,
        select_fields: list[str],
    ) -> QueryResult: ...

    async def update_with_array_union(
        self,
        collection: str,
        document_id: str,
        array_field: str,
        items_to_add: list[object],
        additional_updates: dict[str, object],
    ) -> CacheOperationResult: ...

    async def batch_write(self, operations: list[BatchWriteOperation]) -> CacheOperationResult: ...

    async def close(self) -> None: ...


class DocumentService(DocumentServiceProtocol):
    """The single g8ee owner of Gateway document access.

    Every call is an authoritative read or write through the Gateway document API.
    g8ee keeps no document or query cache: the Gateway owns the committed state and
    a cached copy here could only be a stale one. Write failures raise DatabaseError.
    """

    def __init__(self, client: DBClientProtocol, component_name: str = G8EE_COMPONENT):
        self.client = client
        self.component_name = component_name

    async def close(self) -> None:
        try:
            await self.client.close()
        except Exception as exc:
            logger.info("Error closing document client: %s", exc)

    def _write_error(self, action: str, result: CacheOperationResult) -> DatabaseError:
        return DatabaseError(
            f"{action}: {result.error or 'unknown error'}",
            code=ErrorCode.DB_WRITE_ERROR,
            component=self.component_name,
        )

    async def create_document(
        self,
        collection: str,
        document_id: str,
        data: dict[str, Any] | G8eBaseModel,
    ) -> CacheOperationResult:
        existing = await self.client.get_document(collection=collection, document_id=document_id)
        if existing.success and existing.data is not None:
            raise DatabaseError(
                f"Document {document_id} already exists in {collection}",
                code=ErrorCode.DB_WRITE_ERROR,
                component=self.component_name,
            )

        if isinstance(data, G8eBaseModel):
            data = data.model_dump(mode="json")
        result = await self.client.create_document(
            collection=collection, document_id=document_id, data=data
        )
        if not result.success:
            raise self._write_error(
                f"Failed to create document {document_id} in {collection}", result
            )

        logger.info(
            "[%s-DOCUMENT] Document created",
            self.component_name.upper(),
            extra={"collection": collection, "doc_id": document_id},
        )
        return CacheOperationResult(success=True, document_id=document_id)

    async def update_document(
        self,
        collection: str,
        document_id: str,
        data: dict[str, Any] | G8eBaseModel,
        merge: bool = True,
    ) -> CacheOperationResult:
        if isinstance(data, G8eBaseModel):
            data = data.model_dump(mode="json")
        result = await self.client.update_document(
            collection=collection, document_id=document_id, data=data, merge=merge
        )
        if not result.success:
            raise self._write_error(
                f"Failed to update document {document_id} in {collection}", result
            )

        logger.info(
            "[%s-DOCUMENT] Document updated",
            self.component_name.upper(),
            extra={"collection": collection, "doc_id": document_id, "merge": merge},
        )
        return CacheOperationResult(success=True, document_id=document_id)

    async def delete_document(self, collection: str, document_id: str) -> CacheOperationResult:
        result = await self.client.delete_document(collection=collection, document_id=document_id)
        if not result.success:
            raise self._write_error(
                f"Failed to delete document {document_id} in {collection}", result
            )

        logger.info(
            "[%s-DOCUMENT] Document deleted",
            self.component_name.upper(),
            extra={"collection": collection, "doc_id": document_id},
        )
        return CacheOperationResult(success=True, document_id=document_id)

    async def get_document(self, collection: str, document_id: str) -> DocumentResult:
        return await self.client.get_document(collection=collection, document_id=document_id)

    async def get_document_data(self, collection: str, document_id: str) -> dict[str, Any] | None:
        """Return the document's data as a plain dict, or None when it does not exist."""
        response = await self.client.get_document(collection=collection, document_id=document_id)
        if not response.success or response.data is None:
            return None
        return recursive_serialize(response.data)

    async def query_collection(
        self,
        collection: str,
        field_filters: list[FieldFilter],
        order_by: dict[str, str],
        limit: int,
        select_fields: list[str] | None = None,
    ) -> QueryResult:
        return await self.client.query_collection(
            collection=collection,
            field_filters=[f.model_dump() for f in field_filters],
            order_by=order_by or {},
            limit=limit,
            select_fields=select_fields or [],
        )

    async def query_documents(
        self,
        collection: str,
        field_filters: list[dict[str, Any]],
        order_by: dict[str, str] | None = None,
        limit: int = 100,
        select_fields: list[str] | None = None,
    ) -> list[dict[str, Any]]:
        result = await self.query_collection(
            collection=collection,
            field_filters=[FieldFilter(**f) for f in field_filters],
            order_by=order_by or {},
            limit=limit,
            select_fields=select_fields,
        )
        return result.data if result.success else []

    async def update_with_array_union(
        self,
        collection: str,
        document_id: str,
        array_field: str,
        items_to_add: list[object],
        additional_updates: dict[str, object],
    ) -> CacheOperationResult:
        result = await self.client.update_with_array_union(
            collection=collection,
            document_id=document_id,
            array_field=array_field,
            items_to_add=items_to_add,
            additional_updates=additional_updates,
        )
        if not result.success:
            raise self._write_error(
                f"Failed to append to {array_field} on {document_id} in {collection}", result
            )

        logger.info(
            "[%s-DOCUMENT] Array append completed",
            self.component_name.upper(),
            extra={"collection": collection, "doc_id": document_id, "field": array_field},
        )
        return CacheOperationResult(success=True, document_id=document_id)

    async def append_to_array(
        self,
        collection: str,
        document_id: str,
        array_field: str,
        items_to_add: list[Any],
        additional_updates: dict[str, Any],
    ) -> CacheOperationResult:
        return await self.update_with_array_union(
            collection=collection,
            document_id=document_id,
            array_field=array_field,
            items_to_add=items_to_add,
            additional_updates=additional_updates,
        )

    async def batch_write(self, operations: list[BatchWriteOperation]) -> CacheOperationResult:
        result = await self.client.batch_write(operations)
        if not result.success:
            raise self._write_error("Batch write failed", result)

        logger.info(
            "[%s-DOCUMENT] Batch write completed",
            self.component_name.upper(),
            extra={"operation_count": len(operations)},
        )
        return CacheOperationResult(success=True)

    async def batch_create_documents(
        self, operations: list[BatchCreateDocumentOperation]
    ) -> BatchOperationResult:
        if not operations:
            return BatchOperationResult(success=True, count=0)

        await self.batch_write(
            [
                BatchWriteOperation(
                    op_type=BatchWriteOpType.SET,
                    collection=op.collection,
                    doc_id=op.document_id,
                    data=op.data,
                    merge=False,
                )
                for op in operations
            ]
        )
        return BatchOperationResult(success=True, count=len(operations))
