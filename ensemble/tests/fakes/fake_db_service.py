# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed fake for DocumentServiceProtocol."""

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
from app.services.protocols import DocumentServiceProtocol


class FakeDBService:
    """Typed fake implementing DocumentServiceProtocol.

    Records all calls for assertion in tests. Does not perform any real I/O.
    """

    def __init__(self) -> None:
        self.operator_activities: list[dict[str, object]] = []
        self.chat_messages: list[dict[str, object]] = []
        self.command_results: list[dict[str, object]] = []

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

    async def get_document_data(self, collection: str, document_id: str) -> dict[str, Any] | None:
        return None

    async def query_documents(
        self,
        collection: str,
        field_filters: list[dict[str, Any]],
        order_by: dict[str, str] | None = None,
        limit: int = 100,
        select_fields: list[str] | None = None,
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
