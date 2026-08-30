# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Data service for the per-verdict stake resolution log (GDD §14.5, Phase 3).

Each row is a write-once record bound to a `(tribunal_command_id, agent_id)`
pair. The composite document id provides idempotency: replaying the same
verdict's resolution is a no-op.

Authoritative writer: `app.services.ai.reputation_service`. No other module
in g8ee should import or write through this service.
"""

from __future__ import annotations

import logging

from app.clients.governance_client import GovernanceClient
from app.constants import (
    DB_COLLECTION_STAKE_RESOLUTIONS,
    ErrorCode,
    EventType,
    G8EE_COMPONENT,
)
from app.errors import DatabaseError, ValidationError
from app.models.cache import FieldFilter
from app.models.http_context import RequestContext
from app.models.reputation import StakeResolution
from app.services.protocols import DocumentServiceProtocol

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

    cache: DocumentServiceProtocol
    collection: str

    def __init__(
        self,
        cache: DocumentServiceProtocol,
        governance_client: GovernanceClient,
    ) -> None:
        self.cache = cache
        self.governance_client = governance_client
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
                component=G8EE_COMPONENT,
            ) from exc

    async def create(
        self,
        resolution: StakeResolution,
        context: RequestContext,
    ) -> StakeResolution:
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
            await self.governance_client.update_governed_doc(
                collection=self.collection,
                document_id=resolution.id,
                updates=resolution.model_dump(mode="json"),
                event_type=EventType.OPERATOR_REPUTATION_STAKE_RESOLUTION_CREATED,
                case_id=context.case_id,
                investigation_id=context.investigation_id,
                task_id=context.task_id,
                web_session_id=context.web_session_id,
                user_id=context.user_id,
                operator_id=context.operator_id,
                operator_session_id=context.operator_session_id,
                merge=False,
            )
            logger.info(
                "Stake resolution recorded",
                extra={
                    "id": resolution.id,
                    "agent_id": resolution.agent_id,
                    "tribunal_command_id": resolution.tribunal_command_id,
                    "outcome_score": resolution.outcome_score,
                    "slash_tier": int(resolution.slash_tier)
                    if resolution.slash_tier is not None
                    else None,
                },
            )
            return resolution
        except DatabaseError:
            raise
        except Exception as exc:
            logger.error(
                "Failed to create stake_resolution %s: %s", resolution.id, exc, exc_info=True
            )
            raise DatabaseError(
                message=f"Failed to create stake_resolution: {exc}",
                code=ErrorCode.DB_WRITE_ERROR,
                details={"id": resolution.id},
                cause=exc,
                component=G8EE_COMPONENT,
            ) from exc

    async def list_for_tribunal_command(self, tribunal_command_id: str) -> list[StakeResolution]:
        if not tribunal_command_id:
            raise ValidationError("tribunal_command_id is required")
        try:
            results = await self.cache.query_documents(
                collection=self.collection,
                field_filters=[
                    FieldFilter(
                        field="tribunal_command_id", op="==", value=tribunal_command_id
                    ).model_dump(mode="json")
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
                component=G8EE_COMPONENT,
            ) from exc
