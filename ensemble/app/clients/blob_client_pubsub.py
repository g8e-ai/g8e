# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
PubSubBlobClient - operator Blob Store client using pubsub.

Replaces HTTP-based BlobClient with pubsub-based communication.
All operations are sent to g8eo via pubsub channels.
"""

import logging
from typing import Any

from app.clients.pubsub_client import PubSubClient
from app.constants.channels import OperatorChannel
from app.errors import NetworkError
from app.models.cache import CacheOperationResult

logger = logging.getLogger(__name__)


class PubSubBlobClient:
    """Pubsub-based operator Blob Store client."""

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
            logger.error("[PUBSUB-BLOB-CLIENT] Connection check failed: %s", e)
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
                storage_type=OperatorChannel.STORAGE_BLOB,
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
            raise NetworkError(
                f"Storage request failed: {e}",
                component="g8ee",
                cause=e,
            ) from e

    async def get(self, blob_id: str) -> bytes | None:
        """Get a blob from the Blob Store."""
        try:
            payload = {"blob_id": blob_id}
            response = await self._storage_request("get", payload)
            data = response.get("data")
            return data.encode("utf-8") if isinstance(data, str) else data
        except NetworkError:
            raise
        except Exception as e:
            raise NetworkError(
                f"get failed for blob {blob_id}: {e}",
                component="g8ee",
                cause=e,
            ) from e

    async def put(self, blob_id: str, data: bytes) -> CacheOperationResult:
        """Put a blob into the Blob Store."""
        try:
            payload = {
                "blob_id": blob_id,
                "data": data.decode("utf-8") if isinstance(data, bytes) else data,
            }
            await self._storage_request("put", payload)
            return CacheOperationResult(success=True)
        except NetworkError:
            raise
        except Exception as e:
            raise NetworkError(
                f"put failed for blob {blob_id}: {e}",
                component="g8ee",
                cause=e,
            ) from e

    async def delete(self, blob_id: str) -> CacheOperationResult:
        """Delete a blob from the Blob Store."""
        try:
            payload = {"blob_id": blob_id}
            await self._storage_request("delete", payload)
            return CacheOperationResult(success=True)
        except NetworkError:
            raise
        except Exception as e:
            raise NetworkError(
                f"delete failed for blob {blob_id}: {e}",
                component="g8ee",
                cause=e,
            ) from e
