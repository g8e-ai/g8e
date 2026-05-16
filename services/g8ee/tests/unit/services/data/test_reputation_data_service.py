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

"""Unit tests for `ReputationDataService`.

The service is the only persistence boundary for the cross-chain
reputation scoreboard, so every CRUD path is pinned here. Higher-level
auditor commitment flow lives in `test_auditor_commitment.py`.
"""

from datetime import UTC, datetime

import pytest

from app.constants import (
    DB_COLLECTION_REPUTATION_COMMITMENTS,
    DB_COLLECTION_REPUTATION_STATE,
)
from app.errors import DatabaseError, ValidationError
from app.models.reputation import (
    GENESIS_PREV_ROOT,
    ReputationCommitment,
    ReputationState,
)
from app.services.data.reputation_data_service import ReputationDataService

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]


def _make_state(agent_id: str = "axiom", scalar: float = 0.5) -> ReputationState:
    return ReputationState(
        agent_id=agent_id,
        scalar=scalar,
        updated_at=datetime(2026, 4, 24, 12, 0, 0, tzinfo=UTC),
    )


def _make_commitment(
    *,
    tribunal_command_id: str = "tc-1",
    investigation_id: str = "inv-1",
    merkle_root: str = "a" * 64,
    prev_root: str = GENESIS_PREV_ROOT,
    leaves_count: int = 8,
) -> ReputationCommitment:
    return ReputationCommitment(
        investigation_id=investigation_id,
        tribunal_command_id=tribunal_command_id,
        merkle_root=merkle_root,
        prev_root=prev_root,
        leaves_count=leaves_count,
        signature="b" * 64,
    )


class TestReputationStateCrud:
    @pytest.fixture
    def service(self, mock_cache_aside_service):
        return ReputationDataService(mock_cache_aside_service)

    @pytest.fixture
    def mock_cache(self, mock_cache_aside_service):
        return mock_cache_aside_service

    async def test_get_state_returns_model(self, service, mock_cache):
        mock_cache.get_document_with_cache.return_value = {
            "agent_id": "axiom",
            "scalar": 0.5,
            "updated_at": "2026-04-24T12:00:00Z",
        }

        result = await service.get_state("axiom")

        assert isinstance(result, ReputationState)
        assert result.agent_id == "axiom"
        assert result.scalar == 0.5
        mock_cache.get_document_with_cache.assert_called_once_with(
            collection=DB_COLLECTION_REPUTATION_STATE,
            document_id="axiom",
        )

    async def test_get_state_missing_returns_none(self, service, mock_cache):
        mock_cache.get_document_with_cache.return_value = None
        assert await service.get_state("axiom") is None

    async def test_get_state_empty_id_raises(self, service):
        with pytest.raises(ValidationError):
            await service.get_state("")

    async def test_get_state_db_error_wraps(self, service, mock_cache):
        mock_cache.get_document_with_cache.side_effect = RuntimeError("boom")
        with pytest.raises(DatabaseError):
            await service.get_state("axiom")

    async def test_list_states_sorts_by_agent_id(self, service, mock_cache):
        # The Auditor's Merkle root depends on this sort; pin it explicitly.
        mock_cache.query_documents.return_value = [
            {"agent_id": "variance", "scalar": 0.5, "updated_at": "2026-04-24T12:00:00Z"},
            {"agent_id": "axiom", "scalar": 0.5, "updated_at": "2026-04-24T12:00:00Z"},
            {"agent_id": "concord", "scalar": 0.5, "updated_at": "2026-04-24T12:00:00Z"},
        ]

        results = await service.list_states()

        assert [s.agent_id for s in results] == ["axiom", "concord", "variance"]

    async def test_upsert_state_creates_when_absent(self, service, mock_cache):
        mock_cache.get_document_with_cache.return_value = None
        await service.upsert_state(_make_state())
        mock_cache.create_document.assert_called_once()
        mock_cache.update_document.assert_not_called()

    async def test_upsert_state_updates_when_present(self, service, mock_cache):
        mock_cache.get_document_with_cache.return_value = {"agent_id": "axiom", "scalar": 0.5, "updated_at": "2026-04-24T12:00:00Z"}
        await service.upsert_state(_make_state(scalar=0.6))
        mock_cache.update_document.assert_called_once()
        # Both calls go through; create is *not* called for an existing row.
        mock_cache.create_document.assert_not_called()


class TestReputationCommitmentCrud:
    @pytest.fixture
    def service(self, mock_cache_aside_service):
        return ReputationDataService(mock_cache_aside_service)

    @pytest.fixture
    def mock_cache(self, mock_cache_aside_service):
        return mock_cache_aside_service

    async def test_create_commitment_writes_to_collection(self, service, mock_cache):
        c = _make_commitment()
        await service.create_commitment(c)
        mock_cache.create_document.assert_called_once()
        kwargs = mock_cache.create_document.call_args.kwargs
        assert kwargs["collection"] == DB_COLLECTION_REPUTATION_COMMITMENTS
        assert kwargs["document_id"] == c.id

    async def test_get_commitment_round_trips(self, service, mock_cache):
        c = _make_commitment()
        mock_cache.get_document_with_cache.return_value = c.model_dump(mode="json")
        result = await service.get_commitment(c.id)
        assert isinstance(result, ReputationCommitment)
        assert result.merkle_root == c.merkle_root

    async def test_get_commitment_missing_returns_none(self, service, mock_cache):
        mock_cache.get_document_with_cache.return_value = None
        assert await service.get_commitment("missing") is None

    async def test_get_latest_commitment_picks_first_result(self, service, mock_cache):
        c = _make_commitment()
        mock_cache.query_documents.return_value = [c.model_dump(mode="json")]
        result = await service.get_latest_commitment()
        assert isinstance(result, ReputationCommitment)
        assert result.id == c.id
        # Latest is selected via DESC order_by created_at — pin the contract.
        kwargs = mock_cache.query_documents.call_args.kwargs
        assert kwargs["order_by"] == {"created_at": "desc"}
        assert kwargs["limit"] == 1

    async def test_get_latest_commitment_empty_returns_none(self, service, mock_cache):
        mock_cache.query_documents.return_value = []
        assert await service.get_latest_commitment() is None

    async def test_list_commitments_for_investigation_filters(self, service, mock_cache):
        mock_cache.query_documents.return_value = [_make_commitment().model_dump(mode="json")]
        results = await service.list_commitments_for_investigation("inv-1")
        assert len(results) == 1
        kwargs = mock_cache.query_documents.call_args.kwargs
        assert kwargs["collection"] == DB_COLLECTION_REPUTATION_COMMITMENTS
        # The filter must target investigation_id (binding the commitment to a chain).
        filters = kwargs["field_filters"]
        assert any(f["field"] == "investigation_id" and f["value"] == "inv-1" for f in filters)

    async def test_list_commitments_empty_id_raises(self, service):
        with pytest.raises(ValidationError):
            await service.list_commitments_for_investigation("")
