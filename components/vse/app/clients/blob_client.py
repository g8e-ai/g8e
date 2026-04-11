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
BlobClient -- VSODB Blob Store shim.

Wraps the VSODB /blob/ HTTP API for binary object storage.
Attachments are stored as raw binary keyed by namespace + id.

VSODB endpoints:
    PUT    /blob/{namespace}/{id}   -> store blob
    GET    /blob/{namespace}/{id}   -> retrieve blob
    DELETE /blob/{namespace}/{id}   -> delete blob
    DELETE /blob/{namespace}        -> delete all blobs in namespace
"""

import logging
from urllib.parse import quote

import aiohttp

from app.constants import INTERNAL_AUTH_HEADER
from app.errors import DatabaseError, ErrorCode, NetworkError
from app.models.settings import ListenSettings
from app.utils.aiohttp_session import new_component_http_session

logger = logging.getLogger(__name__)

BLOB_ATTACHMENT_NAMESPACE_PREFIX = "att"


class BlobClient:
    """HTTP shim over the VSODB Blob Store API."""

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

        self._base_url = listen_settings.blob_url
        self._ca_cert_path = ca_cert_path
        self._internal_auth_token = internal_auth_token
        self._session: aiohttp.ClientSession | None = None

    async def _get_http_session(self) -> aiohttp.ClientSession:
        headers = {}
        if self._internal_auth_token:
            headers[INTERNAL_AUTH_HEADER] = self._internal_auth_token

        if self._session is None or self._session.closed:
            self._session = new_component_http_session(
                None,
                timeout=aiohttp.ClientTimeout(total=30),
                ca_cert_path=self._ca_cert_path,
                headers=headers,
            )
        return self._session

    async def connect(self) -> bool:
        """Verify connectivity to the VSODB Blob Store service."""
        try:
            session = await self._get_http_session()
            async with session.get(f"{self._base_url}/health") as resp:
                if resp.status == 200:
                    logger.info("[BLOB-CLIENT] Connected to %s", self._base_url)
                    return True
                return False
        except Exception as e:
            logger.error("[BLOB-CLIENT] Connection failed: %s", e)
            return False

    async def close(self) -> None:
        if self._session and not self._session.closed:
            await self._session.close()

    @staticmethod
    def namespace(investigation_id: str) -> str:
        return f"{BLOB_ATTACHMENT_NAMESPACE_PREFIX}:{investigation_id}"

    @staticmethod
    def object_key(investigation_id: str, attachment_id: str) -> str:
        return f"{BLOB_ATTACHMENT_NAMESPACE_PREFIX}:{investigation_id}/{attachment_id}"

    async def put_blob(
        self,
        namespace: str,
        blob_id: str,
        data: bytes,
        content_type: str,
    ) -> None:
        """Store a binary blob."""
        session = await self._get_http_session()
        url = f"{self._base_url}/blob/{quote(namespace, safe='')}/{quote(blob_id, safe='')}"
        try:
            async with session.put(
                url,
                data=data,
                headers={"Content-Type": content_type},
            ) as resp:
                if resp.status >= 400:
                    text = await resp.text()
                    raise NetworkError(f"VSODB blob PUT {resp.status}: {text}", component="vse")
            logger.info("[BLOB-CLIENT] Stored blob %s/%s (%d bytes)", namespace, blob_id, len(data))
        except NetworkError:
            raise
        except Exception as e:
            raise DatabaseError(
                f"put_blob failed for {namespace}/{blob_id}: {e}",
                code=ErrorCode.DB_WRITE_ERROR,
                component="vse",
                cause=e,
            )

    async def get_blob(self, namespace: str, blob_id: str) -> bytes | None:
        """Retrieve a binary blob. Returns None on 404."""
        session = await self._get_http_session()
        url = f"{self._base_url}/blob/{quote(namespace, safe='')}/{quote(blob_id, safe='')}"
        try:
            async with session.get(url) as resp:
                if resp.status == 404:
                    return None
                if resp.status >= 400:
                    text = await resp.text()
                    raise NetworkError(f"VSODB blob GET {resp.status}: {text}", component="vse")
                return await resp.read()
        except NetworkError:
            raise
        except Exception as e:
            raise DatabaseError(
                f"get_blob failed for {namespace}/{blob_id}: {e}",
                code=ErrorCode.DB_WRITE_ERROR,
                component="vse",
                cause=e,
            )

    async def delete_blob(self, namespace: str, blob_id: str) -> None:
        """Delete a single blob."""
        session = await self._get_http_session()
        url = f"{self._base_url}/blob/{quote(namespace, safe='')}/{quote(blob_id, safe='')}"
        try:
            async with session.delete(url) as resp:
                if resp.status >= 400 and resp.status != 404:
                    text = await resp.text()
                    raise NetworkError(f"VSODB blob DELETE {resp.status}: {text}", component="vse")
            logger.info("[BLOB-CLIENT] Deleted blob %s/%s", namespace, blob_id)
        except NetworkError:
            raise
        except Exception as e:
            raise DatabaseError(
                f"delete_blob failed for {namespace}/{blob_id}: {e}",
                code=ErrorCode.DB_WRITE_ERROR,
                component="vse",
                cause=e,
            )

    async def delete_namespace(self, namespace: str) -> int:
        """Delete all blobs in a namespace. Returns count of deleted blobs."""
        session = await self._get_http_session()
        url = f"{self._base_url}/blob/{quote(namespace, safe='')}"
        try:
            async with session.delete(url) as resp:
                if resp.status >= 400:
                    text = await resp.text()
                    raise NetworkError(f"VSODB blob DELETE ns {resp.status}: {text}", component="vse")
                import json
                body = json.loads(await resp.text())
                count = body.get("deleted", 0)
                logger.info("[BLOB-CLIENT] Deleted namespace %s (%d blobs)", namespace, count)
                return count
        except NetworkError:
            raise
        except Exception as e:
            raise DatabaseError(
                f"delete_namespace failed for {namespace}: {e}",
                code=ErrorCode.DB_WRITE_ERROR,
                component="vse",
                cause=e,
            )
