# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
DBClient - Gateway Document Store shim.

Wraps the Gateway Document Store HTTP API.
No local database - every call goes to the Gateway over HTTP.

Gateway endpoints (constants.APIPaths.DataDB = "/api/v1/data/"):
    GET    /api/v1/data/{collection}/{id}   → get document
    PUT    /api/v1/data/{collection}/{id}   → set (create/replace) document
    PATCH  /api/v1/data/{collection}/{id}   → update (merge) document
    DELETE /api/v1/data/{collection}/{id}   → delete document
    POST   /api/v1/data/{collection}/_query → query documents
"""

import json
import logging
from typing import Any
from urllib.parse import quote

import aiohttp

from app.models.settings import GatewaySettings, TLSConfig
from app.services.infra.settings_service import SettingsService
from app.constants import BatchWriteOpType, AUTHORIZATION, GatewayAPIPaths
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
from app.utils.aiohttp_session import create_component_http_session

logger = logging.getLogger(__name__)


class DBClient:
    """HTTP shim over the operator Document Store API."""

    def __init__(
        self,
        tls_config: TLSConfig | None = None,
        operator_session_id: str | None = None,
        gateway_settings: GatewaySettings | None = None,
    ) -> None:
        if gateway_settings is None:
            service = SettingsService()
            gateway_settings = GatewaySettings.from_bootstrap(service)

        self._base_url = gateway_settings.http_url

        if tls_config is not None:
            self._ca_cert_path = tls_config.ca_cert_path
            self._client_cert_path = tls_config.client_cert_path
            self._client_key_path = tls_config.client_key_path
        else:
            self._ca_cert_path = None
            self._client_cert_path = None
            self._client_key_path = None

        self._operator_session_id = operator_session_id
        self._session: aiohttp.ClientSession | None = None

    async def connect(self) -> bool:
        """Verify connectivity to the Gateway Document Store service."""
        try:
            session = await self._get_http_session()
            async with session.get(f"{self._base_url}{GatewayAPIPaths.HEALTH}") as resp:
                if resp.status == 200:
                    logger.info("[DB-CLIENT] Connected to %s", self._base_url)
                    return True
                return False
        except Exception as e:
            logger.error("[DB-CLIENT] Connection failed: %s", e)
            return False

    async def _get_http_session(self) -> aiohttp.ClientSession:
        headers = {}
        if self._operator_session_id:
            headers[AUTHORIZATION] = f"Bearer {self._operator_session_id}"

        if not hasattr(self, "_session") or self._session is None:
            self._session = create_component_http_session(
                None,
                timeout=aiohttp.ClientTimeout(total=30),
                ca_cert_path=self._ca_cert_path,
                client_cert_path=self._client_cert_path,
                client_key_path=self._client_key_path,
                headers=headers,
            )
        return self._session

    async def _request_json(
        self, method: str, path: str, **kwargs: Any
    ) -> dict[str, object] | None:
        """Execute a request and return the parsed JSON object, or None on 404."""
        session = await self._get_http_session()
        url = f"{self._base_url}{path}"
        async with session.request(method, url, **kwargs) as resp:
            text = await resp.text()
            if resp.status == 404:
                return None
            if resp.status >= 400:
                raise NetworkError(f"client HTTP {resp.status}: {text}", component="g8ee")
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
                raise NetworkError(f"client HTTP {resp.status}: {text}", component="g8ee")
            return json.loads(text)

    async def _request_void(self, method: str, path: str, **kwargs: Any) -> None:
        """Execute a request and discard the response body. Raises on error."""
        session = await self._get_http_session()
        url = f"{self._base_url}{path}"
        async with session.request(method, url, **kwargs) as resp:
            if resp.status >= 400:
                text = await resp.text()
                raise NetworkError(f"client HTTP {resp.status}: {text}", component="g8ee")

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
            path = f"{GatewayAPIPaths.DATA_DB}{quote(collection, safe='')}/{quote(document_id, safe='')}"
            await self._request_void("PUT", path, json=data)
            return CacheOperationResult(success=True, document_id=document_id)
        except NetworkError:
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
            path = f"{GatewayAPIPaths.DATA_DB}{quote(collection, safe='')}/{quote(document_id, safe='')}"
            doc = await self._request_json("GET", path)
            return DocumentResult(success=True, data=doc)
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
        """PATCH (merge) or PUT (replace) a document.

        Values of type ArrayUnion or ArrayRemove trigger a read-modify-write
        cycle against the current document before sending the flat result.
        """
        try:
            path = f"{GatewayAPIPaths.DATA_DB}{quote(collection, safe='')}/{quote(document_id, safe='')}"

            array_ops = {k: v for k, v in data.items() if isinstance(v, (ArrayUnion, ArrayRemove))}

            if array_ops:
                existing = await self._request_json("GET", path) or {}
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
                component="g8ee",
                cause=e,
            ) from e

    async def delete_document(
        self,
        collection: str,
        document_id: str,
    ) -> CacheOperationResult:
        try:
            path = f"{GatewayAPIPaths.DATA_DB}{quote(collection, safe='')}/{quote(document_id, safe='')}"
            await self._request_void("DELETE", path)
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
        """POST {DataDB}{collection}/_query.

        Translates the order_by dict (e.g. {"created_at": "desc"})
        into the Gateway wire format ("created_at DESC").
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

            path = f"{GatewayAPIPaths.DATA_DB}{quote(collection, safe='')}/_query"
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
        """Append items to a list field in a document.

        Fetches the current document, merges the array, and writes back.
        The field must already be a list on the wire - callers are responsible
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
