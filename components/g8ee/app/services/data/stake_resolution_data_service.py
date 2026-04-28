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

"""Data service for the per-verdict stake resolution log (GDD §14.5, Phase 3).

Each row is a write-once record bound to a `(tribunal_command_id, agent_id)`
pair. The composite document id provides idempotency: replaying the same
verdict's resolution is a no-op.

Authoritative writer: `app.services.ai.reputation_service`. No other module
in g8ee should import or write through this service.
"""

from __future__ import annotations

import logging

from app.constants import (
    DB_COLLECTION_STAKE_RESOLUTIONS,
    ComponentName,
    ErrorCode,
)
from app.errors import DatabaseError, ValidationError
from app.models.cache import FieldFilter
from app.models.reputation import StakeResolution
from app.services.cache.cache_aside import CacheAsideService

logger = logging.getLogger(__name__)


def stake_resolution_id(tribunal_command_id: str, agent_id: str) -> str:
    """Return the canonical composite document id for a stake resolution."""
    if not tribunal_command_id:
        raise ValidationError("tribunal_command_id is required")
    if not agent_id:
        raise ValidationError("agent_id is required")
    return f"{tribunal_command_id}:{agent_id}"


class StakeResolutionDataService:
    """CacheAside-backed CRUD for `stake_resolutions`.

    `stake_resolutions` documents are immutable and keyed by the composite
    id `{tribunal_command_id}:{agent_id}`. There is intentionally no
    update method.
    """

    def __init__(self, cache: CacheAsideService) -> None:
        self.cache = cache
        self.collection = DB_COLLECTION_STAKE_RESOLUTIONS

    async def get(self, tribunal_command_id: str, agent_id: str) -> StakeResolution | None:
        doc_id = stake_resolution_id(tribunal_command_id, agent_id)
        try:
            doc = await self.cache.get_document_with_cache(
                collection=self.collection,
                document_id=doc_id,
            )
            if not doc:
                return None
            doc.setdefault("id", doc_id)
            return StakeResolution.model_validate(doc)
        except Exception as exc:
            logger.error("Failed to get stake_resolution %s: %s", doc_id, exc, exc_info=True)
            raise DatabaseError(
                message=f"Failed to get stake_resolution: {exc}",
                code=ErrorCode.DB_QUERY_ERROR,
                details={"id": doc_id},
                cause=exc,
                component=ComponentName.G8EE,
            )

    async def create(self, resolution: StakeResolution) -> StakeResolution:
        """Append a new `stake_resolution` row.

        Returns the existing row unchanged when one is already present for
        the same composite id, preserving write-once idempotency without
        leaking storage-layer collisions to callers.
        """
        if not resolution.id:
            raise ValidationError("StakeResolution.id is required")

        existing = await self.cache.get_document_with_cache(
            collection=self.collection,
            document_id=resolution.id,
        )
        if existing:
            existing.setdefault("id", resolution.id)
            logger.info(
                "Stake resolution already exists; skipping",
                extra={"id": resolution.id},
            )
            return StakeResolution.model_validate(existing)

        try:
            await self.cache.create_document(
                collection=self.collection,
                document_id=resolution.id,
                data=resolution.model_dump(mode="json"),
            )
            logger.info(
                "Stake resolution recorded",
                extra={
                    "id": resolution.id,
                    "agent_id": resolution.agent_id,
                    "tribunal_command_id": resolution.tribunal_command_id,
                    "outcome_score": resolution.outcome_score,
                    "slash_tier": int(resolution.slash_tier) if resolution.slash_tier is not None else None,
                },
            )
            return resolution
        except DatabaseError:
            raise
        except Exception as exc:
            logger.error("Failed to create stake_resolution %s: %s", resolution.id, exc, exc_info=True)
            raise DatabaseError(
                message=f"Failed to create stake_resolution: {exc}",
                code=ErrorCode.DB_WRITE_ERROR,
                details={"id": resolution.id},
                cause=exc,
                component=ComponentName.G8EE,
            )

    async def list_for_tribunal_command(self, tribunal_command_id: str) -> list[StakeResolution]:
        if not tribunal_command_id:
            raise ValidationError("tribunal_command_id is required")
        try:
            results = await self.cache.query_documents(
                collection=self.collection,
                field_filters=[
                    FieldFilter(field="tribunal_command_id", op="==", value=tribunal_command_id).model_dump(mode="json")
                ],
                order_by={"agent_id": "asc"},
                limit=64,
            )
            return [StakeResolution.model_validate(d) for d in results]
        except Exception as exc:
            logger.error(
                "Failed to list stake_resolutions for %s: %s",
                tribunal_command_id,
                exc,
                exc_info=True,
            )
            raise DatabaseError(
                message=f"Failed to list stake_resolutions: {exc}",
                code=ErrorCode.DB_QUERY_ERROR,
                details={"tribunal_command_id": tribunal_command_id},
                cause=exc,
                component=ComponentName.G8EE,
            )
