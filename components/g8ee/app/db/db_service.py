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

import logging

from app.clients.db_client import DBClient
from app.models.base import VSOBaseModel
from app.models.cache import (
    CacheOperationResult,
    DocumentResult,
    QueryResult,
    BatchWriteOperation,
    FieldFilter,
)

logger = logging.getLogger(__name__)

class DBService:
    """Authoritative database service. The sole user of DBClient."""

    def __init__(self, client: DBClient):
        self.client = client

    async def create_document(
        self,
        collection: str,
        document_id: str,
        data: dict[str, object] | VSOBaseModel,
    ) -> CacheOperationResult:
        # DBClient implementation only accepts dict[str, object] for now.
        # Use flatten_for_db if it is a VSOBaseModel.
        if isinstance(data, VSOBaseModel):
            data = data.flatten_for_db()
        return await self.client.create_document(
            collection=collection, document_id=document_id, data=data
        )

    async def update_document(
        self,
        collection: str,
        document_id: str,
        data: dict[str, object] | VSOBaseModel,
        merge: bool = True,
    ) -> CacheOperationResult:
        if isinstance(data, VSOBaseModel):
            data = data.flatten_for_db()
        return await self.client.update_document(
            collection=collection, document_id=document_id, data=data, merge=merge
        )

    async def delete_document(self, collection: str, document_id: str) -> CacheOperationResult:
        return await self.client.delete_document(collection=collection, document_id=document_id)

    async def get_document(self, collection: str, document_id: str) -> DocumentResult:
        return await self.client.get_document(collection=collection, document_id=document_id)

    async def query_collection(
        self,
        collection: str,
        field_filters: list[FieldFilter],
        order_by: dict[str, str],
        limit: int,
        select_fields: list[str] | None = None,
    ) -> QueryResult:
        # DBClient expects list[dict[str, object]] for filters
        filters_raw = [f.model_dump() if hasattr(f, "model_dump") else f for f in field_filters]
        return await self.client.query_collection(
            collection=collection,
            field_filters=filters_raw,  # type: ignore
            order_by=order_by,
            limit=limit,
            select_fields=select_fields or [],
        )

    async def update_with_array_union(
        self,
        collection: str,
        document_id: str,
        array_field: str,
        items_to_add: list[object],
        additional_updates: dict[str, object],
    ) -> CacheOperationResult:
        return await self.client.update_with_array_union(
            collection=collection,
            document_id=document_id,
            array_field=array_field,
            items_to_add=items_to_add,
            additional_updates=additional_updates,
        )

    async def batch_write(self, operations: list[BatchWriteOperation]) -> CacheOperationResult:
        return await self.client.batch_write(operations)

    async def close(self) -> None:
        await self.client.close()
