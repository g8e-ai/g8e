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

from typing import Literal

from pydantic import Field

from app.constants import BatchWriteOpType
from app.models.base import VSOBaseModel


class DocumentResult(VSOBaseModel):
    """Result of a get_document call."""
    success: bool = Field(..., description="Whether the operation succeeded")
    data: dict[str, object] | None = Field(default=None, description="Document data, or None if not found")


class QueryResult(VSOBaseModel):
    """Result of a query_collection call."""
    success: bool = Field(..., description="Whether the operation succeeded")
    data: list[dict[str, object]] = Field(default_factory=list, description="List of matching documents")


class FieldFilter(VSOBaseModel):
    """A single field filter for collection queries."""
    field: str = Field(..., description="Document field name to filter on")
    op: Literal["==", "!=", "<", "<=", ">", ">=", "in", "not-in", "array-contains"] = Field(..., description="Comparison operator")
    value: object = Field(..., description="Value to compare against")


class QueryOrderBy(VSOBaseModel):
    """Ordering clause for collection queries."""
    field: str = Field(..., description="Field to sort by")
    direction: Literal["asc", "desc"] = Field(default="asc", description="Sort direction")


class CacheOperationResult(VSOBaseModel):
    """Result of a cache operation (create, update, delete)."""
    success: bool = Field(..., description="Whether the operation succeeded")
    document_id: str | None = Field(default=None, description="Document ID involved in the operation")
    cached: bool | None = Field(default=None, description="Whether the item was cached")
    cache_invalidated: bool | None = Field(default=None, description="Whether cache was invalidated")
    error: str | None = Field(default=None, description="Error message if operation failed")


class BatchOperationResult(VSOBaseModel):
    """Result of a batch cache operation."""
    success: bool = Field(..., description="Whether the batch operation succeeded")
    count: int = Field(default=0, description="Number of documents processed")
    error: str | None = Field(default=None, description="Error message if operation failed")


class CacheWarmResult(VSOBaseModel):
    """Result of a full user cache warm operation."""
    user_id: str = Field(..., description="User whose cache was warmed")
    cases_count: int = Field(default=0, description="Number of cases warmed")
    investigations_count: int = Field(default=0, description="Number of investigations warmed")
    memories_count: int = Field(default=0, description="Number of memories warmed")
    success: bool = Field(default=True, description="Whether the warm operation succeeded")
    error: str | None = Field(default=None, description="Error message if operation failed")


class CacheContextWarmResult(VSOBaseModel):
    """Result of warming cache for a specific case context."""
    case: bool = Field(default=False, description="Whether the case was successfully warmed")
    investigation: bool = Field(default=False, description="Whether the investigation was successfully warmed")
    memory: bool = Field(default=False, description="Whether the memory was successfully warmed")


class BatchCreateDocumentOperation(VSOBaseModel):
    """Input model for batch create document operations."""
    collection: str = Field(..., description="Target collection name")
    document_id: str = Field(..., description="Document ID")
    data: dict[str, object] = Field(..., description="Document data")


class BatchWriteOperation(VSOBaseModel):
    """A single operation entry for batch_write."""
    op_type: BatchWriteOpType = Field(default=BatchWriteOpType.SET, description="Operation type")
    collection: str = Field(..., description="Target collection name")
    doc_id: str = Field(..., description="Document ID")
    data: dict[str, object] = Field(default_factory=dict, description="Document data (unused for delete)")
    merge: bool = Field(default=False, description="Use merge (PATCH) instead of replace (PUT) for update ops")


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


class CacheStats(VSOBaseModel):
    """Statistics snapshot for the KV cache service."""
    enabled: bool = Field(..., description="Whether caching is enabled")
    healthy: bool = Field(..., description="Whether the KV backend is healthy")
    document_keys: int = Field(default=0, description="Number of document cache keys")
    query_keys: int = Field(default=0, description="Number of query cache keys")
    total_keys: int = Field(default=0, description="Total number of cache keys")
    default_ttl: int = Field(default=0, description="Default TTL in seconds")
    error: str | None = Field(default=None, description="Error message if stats retrieval failed")
