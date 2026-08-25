# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import logging

from app.clients.blob_client import BlobClient

logger = logging.getLogger(__name__)


class BlobService:
    """Authoritative blob service. The sole user of BlobClient."""

    def __init__(self, client: BlobClient):
        self.client = client

    @staticmethod
    def namespace(investigation_id: str) -> str:
        return BlobClient.namespace(investigation_id)

    @staticmethod
    def object_key(investigation_id: str, attachment_id: str) -> str:
        return BlobClient.object_key(investigation_id, attachment_id)

    async def put_blob(
        self,
        namespace: str,
        blob_id: str,
        data: bytes,
        content_type: str,
    ) -> None:
        await self.client.put_blob(namespace, blob_id, data, content_type)

    async def get_blob(self, namespace: str, blob_id: str) -> bytes | None:
        return await self.client.get_blob(namespace, blob_id)

    async def delete_blob(self, namespace: str, blob_id: str) -> None:
        await self.client.delete_blob(namespace, blob_id)

    async def delete_namespace(self, namespace: str) -> int:
        return await self.client.delete_namespace(namespace)

    async def close(self) -> None:
        await self.client.close()
