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
PubSubKVCacheClient - operator KV store client using pubsub.

Replaces HTTP-based KVCacheClient with pubsub-based communication.
All operations are sent to g8eo via pubsub channels.
"""

import logging
from typing import Any

from app.clients.pubsub_client import PubSubClient
from app.constants.channels import OperatorChannel
from app.errors import NetworkError
from app.models.cache import CacheOperationResult

logger = logging.getLogger(__name__)


class PubSubKVCacheClient:
    """Pubsub-based operator KV store client."""

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
            logger.error("[PUBSUB-KV-CLIENT] Connection check failed: %s", e)
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
                storage_type=OperatorChannel.STORAGE_KV,
                operation=operation,
                payload=payload,
            )

            if response.get("success"):
                return response.get("data", {})
            else:
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

    async def get(self, key: str) -> str | None:
        """Get a value from the KV store."""
        try:
            payload = {"key": key}
            response = await self._storage_request("get", payload)
            return response.get("value")
        except NetworkError:
            raise
        except Exception as e:
            raise NetworkError(
                f"get failed for key {key}: {e}",
                component="g8ee",
                cause=e,
            ) from e

    async def set(self, key: str, value: str, ttl: int | None = None) -> CacheOperationResult:
        """Set a value in the KV store."""
        try:
            payload = {"key": key, "value": value}
            if ttl is not None:
                payload["ttl"] = ttl
            await self._storage_request("set", payload)
            return CacheOperationResult(success=True)
        except NetworkError:
            raise
        except Exception as e:
            raise NetworkError(
                f"set failed for key {key}: {e}",
                component="g8ee",
                cause=e,
            ) from e

    async def delete(self, key: str) -> CacheOperationResult:
        """Delete a value from the KV store."""
        try:
            payload = {"key": key}
            await self._storage_request("delete", payload)
            return CacheOperationResult(success=True)
        except NetworkError:
            raise
        except Exception as e:
            raise NetworkError(
                f"delete failed for key {key}: {e}",
                component="g8ee",
                cause=e,
            ) from e
