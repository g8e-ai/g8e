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

"""Data service for the Tribunal reputation scoreboard (GDD §14.4).

Wraps the two reputation collections behind a CacheAside-fronted CRUD
interface. Two consumers exist by design (see the cross-phase invariant
in §14.5 of the GDD progress doc):

- The Auditor reads `reputation_state` (cross-chain memory) and writes
  `reputation_commitment` inside its verdict step.
- The Phase 3 `reputation_service` writes `reputation_state` after each
  execution result lands.

No other agent persona may import this module — that boundary is what
keeps the vortex (GDD §3) intact.
"""

from __future__ import annotations

import logging
from typing import Any

from app.constants import (
    DB_COLLECTION_REPUTATION_COMMITMENTS,
    DB_COLLECTION_REPUTATION_STATE,
    ComponentName,
    ErrorCode,
)
from app.errors import DatabaseError, ValidationError
from app.models.cache import FieldFilter
from app.models.reputation import ReputationCommitment, ReputationState
from app.services.cache.cache_aside import CacheAsideService

logger = logging.getLogger(__name__)


class ReputationDataService:
    """CacheAside-backed CRUD for `reputation_state` and `reputation_commitments`.

    `reputation_state` documents are keyed by ``agent_id`` (one row per
    persona). `reputation_commitment` documents are keyed by their UUID
    ``id`` and are append-only — there is intentionally no ``update`` for
    commitments.
    """

    def __init__(self, cache: CacheAsideService) -> None:
        self.cache = cache
        self.state_collection = DB_COLLECTION_REPUTATION_STATE
        self.commitments_collection = DB_COLLECTION_REPUTATION_COMMITMENTS

    # ------------------------------------------------------------------
    # reputation_state
    # ------------------------------------------------------------------

    async def get_state(self, agent_id: str) -> ReputationState | None:
        """Return the current scalar for ``agent_id``, or None if absent."""
        if not agent_id:
            raise ValidationError("agent_id is required")
        try:
            doc = await self.cache.get_document_with_cache(
                collection=self.state_collection,
                document_id=agent_id,
            )
            if not doc:
                return None
            doc.setdefault("agent_id", agent_id)
            return ReputationState.model_validate(doc)
        except Exception as exc:
            logger.error("Failed to get reputation_state for %s: %s", agent_id, exc, exc_info=True)
            raise DatabaseError(
                message=f"Failed to get reputation_state for {agent_id}: {exc}",
                code=ErrorCode.DB_QUERY_ERROR,
                details={"agent_id": agent_id},
                cause=exc,
                component=ComponentName.G8EE,
            )

    async def list_states(self) -> list[ReputationState]:
        """Return every `reputation_state` row, ordered ASCII-ascending by ``agent_id``.

        The ordering is load-bearing: the Auditor's Merkle root is taken
        over leaves sorted by ``agent_id``, so any caller assembling that
        root reads through this method to inherit the canonical order.
        """
        try:
            results = await self.cache.query_documents(
                collection=self.state_collection,
                field_filters=[],
                order_by={"agent_id": "asc"},
                limit=1000,
            )
            states = [ReputationState.model_validate(d) for d in results]
            # Defensive re-sort: cache backends do not all honour ordering.
            states.sort(key=lambda s: s.agent_id)
            return states
        except Exception as exc:
            logger.error("Failed to list reputation_state: %s", exc, exc_info=True)
            raise DatabaseError(
                message=f"Failed to list reputation_state: {exc}",
                code=ErrorCode.DB_QUERY_ERROR,
                cause=exc,
                component=ComponentName.G8EE,
            )

    async def upsert_state(self, state: ReputationState) -> ReputationState:
        """Create or merge a `reputation_state` row keyed on ``agent_id``."""
        if not state.agent_id:
            raise ValidationError("ReputationState.agent_id is required")
        try:
            existing = await self.cache.get_document_with_cache(
                collection=self.state_collection,
                document_id=state.agent_id,
            )
            if existing is None:
                await self.cache.create_document(
                    collection=self.state_collection,
                    document_id=state.agent_id,
                    data=state.model_dump(mode="json"),
                )
            else:
                await self.cache.update_document(
                    collection=self.state_collection,
                    document_id=state.agent_id,
                    data=state.model_dump(mode="json"),
                    merge=True,
                )
            return state
        except DatabaseError:
            raise
        except Exception as exc:
            logger.error("Failed to upsert reputation_state for %s: %s", state.agent_id, exc, exc_info=True)
            raise DatabaseError(
                message=f"Failed to upsert reputation_state for {state.agent_id}: {exc}",
                code=ErrorCode.DB_WRITE_ERROR,
                details={"agent_id": state.agent_id},
                cause=exc,
                component=ComponentName.G8EE,
            )

    # ------------------------------------------------------------------
    # reputation_commitments
    # ------------------------------------------------------------------

    async def create_commitment(self, commitment: ReputationCommitment) -> ReputationCommitment:
        """Append a new `reputation_commitment` row. Commitments are immutable."""
        if not commitment.id:
            raise ValidationError("ReputationCommitment.id is required")
        try:
            await self.cache.create_document(
                collection=self.commitments_collection,
                document_id=commitment.id,
                data=commitment.model_dump(mode="json"),
            )
            logger.info(
                "Reputation commitment created",
                extra={
                    "commitment_id": commitment.id,
                    "tribunal_command_id": commitment.tribunal_command_id,
                    "investigation_id": commitment.investigation_id,
                    "merkle_root": commitment.merkle_root,
                },
            )
            return commitment
        except DatabaseError:
            raise
        except Exception as exc:
            logger.error("Failed to create reputation_commitment %s: %s", commitment.id, exc, exc_info=True)
            raise DatabaseError(
                message=f"Failed to create reputation_commitment: {exc}",
                code=ErrorCode.DB_WRITE_ERROR,
                details={"commitment_id": commitment.id},
                cause=exc,
                component=ComponentName.G8EE,
            )

    async def get_commitment(self, commitment_id: str) -> ReputationCommitment | None:
        if not commitment_id:
            raise ValidationError("commitment_id is required")
        try:
            doc = await self.cache.get_document_with_cache(
                collection=self.commitments_collection,
                document_id=commitment_id,
            )
            if not doc:
                return None
            doc.setdefault("id", commitment_id)
            return ReputationCommitment.model_validate(doc)
        except Exception as exc:
            logger.error("Failed to get reputation_commitment %s: %s", commitment_id, exc, exc_info=True)
            raise DatabaseError(
                message=f"Failed to get reputation_commitment: {exc}",
                code=ErrorCode.DB_QUERY_ERROR,
                details={"commitment_id": commitment_id},
                cause=exc,
                component=ComponentName.G8EE,
            )

    async def get_latest_commitment(self) -> ReputationCommitment | None:
        """Return the deployment's most recent commitment, or None at genesis.

        Used by the Auditor to populate `prev_root` on the next commitment
        — the cross-chain spine in GDD §14.4.
        """
        try:
            results = await self.cache.query_documents(
                collection=self.commitments_collection,
                field_filters=[],
                order_by={"created_at": "desc"},
                limit=1,
            )
            if not results:
                return None
            return ReputationCommitment.model_validate(results[0])
        except Exception as exc:
            logger.error("Failed to get latest reputation_commitment: %s", exc, exc_info=True)
            raise DatabaseError(
                message=f"Failed to get latest reputation_commitment: {exc}",
                code=ErrorCode.DB_QUERY_ERROR,
                cause=exc,
                component=ComponentName.G8EE,
            )

    async def list_commitments_for_investigation(
        self,
        investigation_id: str,
        limit: int = 100,
    ) -> list[ReputationCommitment]:
        """Return commitments tied to a specific investigation, newest first."""
        if not investigation_id:
            raise ValidationError("investigation_id is required")
        try:
            results = await self.cache.query_documents(
                collection=self.commitments_collection,
                field_filters=[
                    FieldFilter(field="investigation_id", op="==", value=investigation_id).model_dump(mode="json")
                ],
                order_by={"created_at": "desc"},
                limit=limit,
            )
            return [ReputationCommitment.model_validate(d) for d in results]
        except Exception as exc:
            logger.error(
                "Failed to list reputation_commitments for investigation %s: %s",
                investigation_id,
                exc,
                exc_info=True,
            )
            raise DatabaseError(
                message=f"Failed to list reputation_commitments: {exc}",
                code=ErrorCode.DB_QUERY_ERROR,
                details={"investigation_id": investigation_id},
                cause=exc,
                component=ComponentName.G8EE,
            )
