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

"""
DBClient — VSODB Document Store shim.

Wraps the VSODB (Operator --listen mode) Document Store HTTP API.
No local database — every call goes to VSODB over HTTP.

VSODB endpoints:
    GET    /db/{collection}/{id}       → get document
    PUT    /db/{collection}/{id}       → set (create/replace) document
    PATCH  /db/{collection}/{id}       → update (merge) document
    DELETE /db/{collection}/{id}       → delete document
    POST   /db/{collection}/_query     → query documents
"""

import json
import logging
from typing import Any
from urllib.parse import quote

import aiohttp

from app.models.settings import ListenSettings
from app.constants import BatchWriteOpType, INTERNAL_AUTH_HEADER
from app.errors import (
    DatabaseError,
    ErrorCode,
    NetworkError,
    ResourceNotFoundError,
    ValidationError,
)
from app.models.cache import ArrayRemove, ArrayUnion, BatchWriteOperation, CacheOperationResult, DocumentResult, QueryResult
from app.utils.aiohttp_session import new_component_http_session

logger = logging.getLogger(__name__)


class DBClient:
    """HTTP shim over the VSODB Document Store API."""

    def __init__(
        self, 
        ca_cert_path: str, 
        internal_auth_token: str | None = None,
        listen_settings: ListenSettings | None = None,
    ) -> None:
        if internal_auth_token is None:
            from app.services.infra.settings_service import SettingsService
            service = SettingsService()
            local_settings = service.get_local_settings()
            internal_auth_token = local_settings.auth.internal_auth_token

        if listen_settings is None:
            from app.services.infra.settings_service import SettingsService
            service = SettingsService()
            listen_settings = ListenSettings.from_bootstrap(service)
            
        self._base_url = listen_settings.http_url
        self._ca_cert_path = ca_cert_path
        self._internal_auth_token = internal_auth_token
        self._session: aiohttp.ClientSession | None = None

    async def connect(self) -> bool:
        """Verify connectivity to the VSODB Document Store service."""
        try:
            # Document store doesn't have a dedicated /health yet, but we can check the base URL
            # or just assume session creation is enough for now, but to match KVCacheClient:
            session = await self._get_http_session()
            async with session.get(f"{self._base_url}/health") as resp:
                if resp.status == 200:
                    logger.info(f"[DB-CLIENT] Connected to {self._base_url}")
                    return True
                return False
        except Exception as e:
            logger.error(f"[DB-CLIENT] Connection failed: {e}")
            return False

    async def _get_http_session(self) -> aiohttp.ClientSession:
        headers = {}
        if self._internal_auth_token:
            headers[INTERNAL_AUTH_HEADER] = self._internal_auth_token

        if not hasattr(self, "_session") or self._session is None:
            self._session = new_component_http_session(
                None,
                timeout=aiohttp.ClientTimeout(total=30),
                ca_cert_path=self._ca_cert_path,
                headers=headers,
            )
        return self._session

    async def _request_json(self, method: str, path: str, **kwargs: Any) -> dict[str, object] | None:
        """Execute a request and return the parsed JSON object, or None on 404."""
        session = await self._get_http_session()
        url = f"{self._base_url}{path}"
        async with session.request(method, url, **kwargs) as resp:
            text = await resp.text()
            if resp.status == 404:
                return None
            if resp.status >= 400:
                raise NetworkError(f"VSODB HTTP {resp.status}: {text}", component="vse")
            return json.loads(text)

    async def _request_list(self, method: str, path: str, **kwargs: Any) -> list[dict[str, object]]:
        """Execute a request and return the parsed JSON array."""
        session = await self._get_http_session()
        url = f"{self._base_url}{path}"
        async with session.request(method, url, **kwargs) as resp:
            text = await resp.text()
            if resp.status == 404:
                return []
            if resp.status >= 400:
                raise NetworkError(f"VSODB HTTP {resp.status}: {text}", component="vse")
            return json.loads(text)

    async def _request_void(self, method: str, path: str, **kwargs: Any) -> None:
        """Execute a request and discard the response body. Raises on error."""
        session = await self._get_http_session()
        url = f"{self._base_url}{path}"
        async with session.request(method, url, **kwargs) as resp:
            if resp.status >= 400:
                text = await resp.text()
                raise NetworkError(f"VSODB HTTP {resp.status}: {text}", component="vse")

    async def close(self) -> None:
        try:
            session = self._session
        except AttributeError:
            session = None

        if session and not session.closed:
            await session.close()

    async def create_document(
        self,
        collection: str,
        document_id: str,
        data: dict[str, object],
    ) -> CacheOperationResult:
        try:
            path = f"/db/{quote(collection, safe='')}/{quote(document_id, safe='')}"
            await self._request_void("PUT", path, json=data)
            return CacheOperationResult(success=True, document_id=document_id)
        except NetworkError:
            raise
        except Exception as e:
            raise DatabaseError(
                f"create_document failed for {collection}/{document_id}: {e}",
                code=ErrorCode.DB_WRITE_ERROR,
                component="vse",
                cause=e,
            )

    async def get_document(
        self,
        collection: str,
        document_id: str,
    ) -> DocumentResult:
        try:
            path = f"/db/{quote(collection, safe='')}/{quote(document_id, safe='')}"
            doc = await self._request_json("GET", path)
            return DocumentResult(success=True, data=doc)
        except NetworkError:
            raise
        except Exception as e:
            raise DatabaseError(
                f"get_document failed for {collection}/{document_id}: {e}",
                code=ErrorCode.DB_QUERY_ERROR,
                component="vse",
                cause=e,
            )

    async def update_document(
        self,
        collection: str,
        document_id: str,
        data: dict[str, object],
        merge: bool = True,
    ) -> CacheOperationResult:
        """PATCH (merge) or PUT (replace) a document.

        Values of type ArrayUnion or ArrayRemove trigger a read-modify-write
        cycle against the current document before sending the flat result.
        """
        try:
            path = f"/db/{quote(collection, safe='')}/{quote(document_id, safe='')}"

            array_ops = {k: v for k, v in data.items() if isinstance(v, (ArrayUnion, ArrayRemove))}

            if array_ops:
                existing = await self._request_json("GET", path) or {}
                patch: dict[str, object] = {k: v for k, v in data.items() if k not in array_ops}
                for k, op in array_ops.items():
                    raw = existing.get(k)
                    current: list[object] = raw if isinstance(raw, list) else []
                    if isinstance(op, ArrayUnion):
                        merged = current + op.values
                        patch[k] = merged[-op.max_length:] if op.max_length is not None else merged
                    else:
                        patch[k] = [item for item in current if item not in op.values]
                data = patch

            if merge:
                await self._request_void("PATCH", path, json=data)
            else:
                await self._request_void("PUT", path, json=data)

            return CacheOperationResult(success=True, document_id=document_id)
        except (NetworkError, DatabaseError):
            raise
        except Exception as e:
            raise DatabaseError(
                f"update_document failed for {collection}/{document_id}: {e}",
                code=ErrorCode.DB_WRITE_ERROR,
                component="vse",
                cause=e,
            )

    async def delete_document(
        self,
        collection: str,
        document_id: str,
    ) -> CacheOperationResult:
        try:
            path = f"/db/{quote(collection, safe='')}/{quote(document_id, safe='')}"
            await self._request_void("DELETE", path)
            return CacheOperationResult(success=True, document_id=document_id)
        except NetworkError:
            raise
        except Exception as e:
            raise DatabaseError(
                f"delete_document failed for {collection}/{document_id}: {e}",
                code=ErrorCode.DB_WRITE_ERROR,
                component="vse",
                cause=e,
            )

    async def query_collection(
        self,
        collection: str,
        field_filters: list[dict[str, object]],
        order_by: dict[str, str],
        limit: int,
        select_fields: list[str],
    ) -> QueryResult:
        """POST /db/{collection}/_query.

        Translates the order_by dict (e.g. {"created_at": "desc"})
        into the VSODB wire format ("created_at DESC").
        """
        try:
            body: dict[str, object] = {}
            if field_filters:
                body["filters"] = field_filters
            if order_by:
                field, direction = next(iter(order_by.items()))
                body["order_by"] = f"{field} {direction.upper()}"
            if limit:
                body["limit"] = limit

            path = f"/db/{quote(collection, safe='')}/_query"
            docs = await self._request_list("POST", path, json=body)

            if select_fields:
                keep = set(select_fields) | {"id"}
                docs = [{k: v for k, v in doc.items() if k in keep} for doc in docs]

            return QueryResult(success=True, data=docs)
        except (NetworkError, DatabaseError):
            raise
        except Exception as e:
            raise DatabaseError(
                f"query_collection failed for {collection}: {e}",
                code=ErrorCode.DB_QUERY_ERROR,
                component="vse",
                cause=e,
            )

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
        """Append items to a list field in a document.

        Fetches the current document, merges the array, and writes back.
        The field must already be a list on the wire — callers are responsible
        for ensuring the field is written as a native JSON array.
        """
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
                result = await self.update_document(op.collection, op.doc_id, op.data, merge=op.merge)
            elif op.op_type == BatchWriteOpType.DELETE:
                result = await self.delete_document(op.collection, op.doc_id)
            else:
                raise ValidationError(
                    f"Unknown batch_write op type: {op.op_type!r}",
                    component="vse",
                )

            if not result.success:
                return result

        return CacheOperationResult(success=True)

