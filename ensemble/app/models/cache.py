# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from typing import Literal

from app.constants import BatchWriteOpType
from app.models.base import Field, G8eBaseModel


class DocumentResult(G8eBaseModel):
    """Result of a get_document call."""

    success: bool = Field(..., description="Whether the operation succeeded")
    data: dict[str, object] | None = Field(
        default=None, description="Document data, or None if not found"
    )


class QueryResult(G8eBaseModel):
    """Result of a query_collection call."""

    success: bool = Field(..., description="Whether the operation succeeded")
    data: list[dict[str, object]] = Field(
        default_factory=list, description="List of matching documents"
    )


class FieldFilter(G8eBaseModel):
    """A single field filter for collection queries."""

    field: str = Field(..., description="Document field name to filter on")
    op: Literal["==", "!=", "<", "<=", ">", ">=", "in", "not-in", "array-contains"] = Field(
        ..., description="Comparison operator"
    )
    value: object = Field(..., description="Value to compare against")


class QueryOrderBy(G8eBaseModel):
    """Ordering clause for collection queries."""

    field: str = Field(..., description="Field to sort by")
    direction: Literal["asc", "desc"] = Field(default="asc", description="Sort direction")


class CacheOperationResult(G8eBaseModel):
    """Result of a cache operation (create, update, delete)."""

    success: bool = Field(..., description="Whether the operation succeeded")
    document_id: str | None = Field(
        default=None, description="Document ID involved in the operation"
    )
    error: str | None = Field(default=None, description="Error message if operation failed")


class BatchOperationResult(G8eBaseModel):
    """Result of a batch cache operation."""

    success: bool = Field(..., description="Whether the batch operation succeeded")
    count: int = Field(default=0, description="Number of documents processed")
    error: str | None = Field(default=None, description="Error message if operation failed")


class BatchCreateDocumentOperation(G8eBaseModel):
    """Input model for batch create document operations."""

    collection: str = Field(..., description="Target collection name")
    document_id: str = Field(..., description="Document ID")
    data: dict[str, object] = Field(..., description="Document data")


class BatchWriteOperation(G8eBaseModel):
    """A single operation entry for batch_write."""

    op_type: BatchWriteOpType = Field(default=BatchWriteOpType.SET, description="Operation type")
    collection: str = Field(..., description="Target collection name")
    doc_id: str = Field(..., description="Document ID")
    data: dict[str, object] = Field(
        default_factory=dict, description="Document data (unused for delete)"
    )
    merge: bool = Field(
        default=False, description="Use merge (PATCH) instead of replace (PUT) for update ops"
    )


class ArrayUnion:
    """Marker that tells update_document to append items to an existing array field.

    If max_length is set, the result is capped to the last max_length elements.
    """

    def __init__(self, values: list[object], max_length: int | None = None):
        self.values = values
        self.max_length = max_length


class ArrayRemove:
    """Marker that tells update_document to remove items from an existing array field."""

    def __init__(self, values: list[object]):
        self.values = values
