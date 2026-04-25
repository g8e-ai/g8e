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

"""Unit tests for `StakeResolutionDataService` (Phase 3, GDD §14.5).

Pins the load-bearing properties:

1. The composite document id is `{tribunal_command_id}:{agent_id}` —
   this is the write-once idempotency guard.
2. `create()` returns the existing row unchanged on replay (no second
   write to the cache).
3. `list_for_tribunal_command()` filters by `tribunal_command_id` and
   sorts by `agent_id`.
4. `get()` round-trip preserves all fields including `slash_tier`.
"""

from __future__ import annotations

from datetime import UTC, datetime

import pytest

from app.constants import DB_COLLECTION_STAKE_RESOLUTIONS
from app.errors import DatabaseError, ValidationError
from app.models.reputation import SlashTier, StakeResolution
from app.services.data.stake_resolution_data_service import (
    StakeResolutionDataService,
    stake_resolution_id,
)

pytestmark = [pytest.mark.unit]


def _make_resolution(
    *,
    tribunal_command_id: str = "tc-1",
    agent_id: str = "axiom",
    slash_tier: SlashTier | None = None,
) -> StakeResolution:
    return StakeResolution(
        id=stake_resolution_id(tribunal_command_id, agent_id),
        investigation_id="inv-1",
        tribunal_command_id=tribunal_command_id,
        agent_id=agent_id,
        outcome_score=1.0,
        rationale="winner_supporter_verified",
        slash_tier=slash_tier,
        scalar_before=0.5,
        scalar_after=0.51,
        half_life=50,
        created_at=datetime(2026, 4, 24, 12, 0, 0, tzinfo=UTC),
    )


class TestCompositeId:
    def test_composite_id_format(self):
        assert stake_resolution_id("tc-1", "axiom") == "tc-1:axiom"

    def test_empty_tribunal_command_id_raises(self):
        with pytest.raises(ValidationError):
            stake_resolution_id("", "axiom")

    def test_empty_agent_id_raises(self):
        with pytest.raises(ValidationError):
            stake_resolution_id("tc-1", "")


class TestCreate:
    pytestmark = [pytest.mark.asyncio(loop_scope="session")]

    @pytest.fixture
    def service(self, mock_cache_aside_service):
        return StakeResolutionDataService(mock_cache_aside_service)

    @pytest.fixture
    def mock_cache(self, mock_cache_aside_service):
        return mock_cache_aside_service

    async def test_create_writes_with_composite_id(self, service, mock_cache):
        mock_cache.get_document.return_value = None
        r = _make_resolution()
        await service.create(r)
        mock_cache.create_document.assert_called_once()
        kwargs = mock_cache.create_document.call_args.kwargs
        assert kwargs["collection"] == DB_COLLECTION_STAKE_RESOLUTIONS
        assert kwargs["document_id"] == "tc-1:axiom"
        # Persisted payload must include the slash_tier integer when present.
        assert "slash_tier" not in kwargs["data"]

    async def test_create_with_slash_tier_persists_int(self, service, mock_cache):
        mock_cache.get_document.return_value = None
        r = _make_resolution(slash_tier=SlashTier.TIER_2)
        await service.create(r)
        kwargs = mock_cache.create_document.call_args.kwargs
        assert kwargs["data"]["slash_tier"] == 2

    async def test_create_idempotent_returns_existing_no_second_write(
        self, service, mock_cache
    ):
        existing = _make_resolution(slash_tier=SlashTier.TIER_3)
        mock_cache.get_document.return_value = existing.model_dump(mode="json")

        result = await service.create(_make_resolution())

        # Existing row returned, no second write attempted.
        mock_cache.create_document.assert_not_called()
        assert isinstance(result, StakeResolution)
        assert result.id == existing.id
        assert result.slash_tier == SlashTier.TIER_3

    async def test_create_wraps_unexpected_failure_as_database_error(
        self, service, mock_cache
    ):
        mock_cache.get_document.return_value = None
        mock_cache.create_document.side_effect = RuntimeError("boom")
        with pytest.raises(DatabaseError):
            await service.create(_make_resolution())


class TestGet:
    pytestmark = [pytest.mark.asyncio(loop_scope="session")]

    @pytest.fixture
    def service(self, mock_cache_aside_service):
        return StakeResolutionDataService(mock_cache_aside_service)

    @pytest.fixture
    def mock_cache(self, mock_cache_aside_service):
        return mock_cache_aside_service

    async def test_get_round_trip_preserves_slash_tier(self, service, mock_cache):
        r = _make_resolution(slash_tier=SlashTier.TIER_1)
        mock_cache.get_document.return_value = r.model_dump(mode="json")

        result = await service.get(tribunal_command_id="tc-1", agent_id="axiom")

        assert isinstance(result, StakeResolution)
        assert result.slash_tier == SlashTier.TIER_1
        assert result.outcome_score == 1.0
        assert result.scalar_after == 0.51
        mock_cache.get_document.assert_called_once_with(
            collection=DB_COLLECTION_STAKE_RESOLUTIONS,
            document_id="tc-1:axiom",
        )

    async def test_get_missing_returns_none(self, service, mock_cache):
        mock_cache.get_document.return_value = None
        assert await service.get(tribunal_command_id="tc-1", agent_id="axiom") is None

    async def test_get_db_error_wraps(self, service, mock_cache):
        mock_cache.get_document.side_effect = RuntimeError("boom")
        with pytest.raises(DatabaseError):
            await service.get(tribunal_command_id="tc-1", agent_id="axiom")


class TestListForTribunalCommand:
    pytestmark = [pytest.mark.asyncio(loop_scope="session")]

    @pytest.fixture
    def service(self, mock_cache_aside_service):
        return StakeResolutionDataService(mock_cache_aside_service)

    @pytest.fixture
    def mock_cache(self, mock_cache_aside_service):
        return mock_cache_aside_service

    async def test_list_filters_by_tribunal_command_id_and_orders_by_agent(
        self, service, mock_cache
    ):
        mock_cache.query_documents.return_value = [
            _make_resolution(agent_id="axiom").model_dump(mode="json"),
            _make_resolution(agent_id="concord").model_dump(mode="json"),
            _make_resolution(agent_id="variance").model_dump(mode="json"),
        ]

        results = await service.list_for_tribunal_command("tc-1")

        assert [r.agent_id for r in results] == ["axiom", "concord", "variance"]
        kwargs = mock_cache.query_documents.call_args.kwargs
        assert kwargs["collection"] == DB_COLLECTION_STAKE_RESOLUTIONS
        assert kwargs["order_by"] == {"agent_id": "asc"}
        filters = kwargs["field_filters"]
        assert any(
            f["field"] == "tribunal_command_id" and f["value"] == "tc-1" for f in filters
        )

    async def test_list_empty_id_raises(self, service):
        with pytest.raises(ValidationError):
            await service.list_for_tribunal_command("")

    async def test_list_db_error_wraps(self, service, mock_cache):
        mock_cache.query_documents.side_effect = RuntimeError("boom")
        with pytest.raises(DatabaseError):
            await service.list_for_tribunal_command("tc-1")
