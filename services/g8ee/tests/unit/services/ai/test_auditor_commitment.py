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

"""Unit tests for `auditor_service.commit_reputation` (GDD §14.4 Artifact B).

The commit function is pure with respect to the scoreboard: given a
`ReputationDataService` snapshot and an HMAC key, it must produce a
deterministic Merkle root, a verifiable HMAC signature, and a commitment
whose `prev_root` chains to the previous commitment in the deployment.

These tests exercise the real `ReputationDataService` backed by a real
`CacheAsideService` with in-memory fakes for the KV/DB clients per the
no-mocks-on-internal-services policy in `docs/testing.md`.
"""

from __future__ import annotations

import hashlib
import hmac
from datetime import UTC, datetime

import pytest

from app.errors import DatabaseError
from app.models.reputation import GENESIS_PREV_ROOT, ReputationState
from app.services.ai.auditor_service import commit_reputation
from app.services.data.reputation_data_service import ReputationDataService
from app.utils.merkle import leaf_bytes, merkle_root

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]


_FIXED_HMAC_KEY = "a" * 64


def _state(agent_id: str, scalar: float) -> ReputationState:
    return ReputationState(
        agent_id=agent_id,
        scalar=scalar,
        updated_at=datetime(2026, 4, 24, 12, 0, 0, tzinfo=UTC),
    )


async def _seed_states(service: ReputationDataService, states: list[ReputationState]) -> None:
    for s in states:
        await service.upsert_state(s)


def _recompute_root(states: list[ReputationState]) -> str:
    ordered = sorted(states, key=lambda s: s.agent_id)
    return merkle_root([leaf_bytes(s.agent_id, s.scalar) for s in ordered])


def _expected_signature(merkle_root_hex: str, prev_root: str, tribunal_command_id: str, key: str) -> str:
    return hmac.new(
        key.encode("utf-8"),
        (merkle_root_hex + prev_root + tribunal_command_id).encode("utf-8"),
        hashlib.sha256,
    ).hexdigest()


class TestCommitReputation:
    @pytest.fixture
    def service(self, fake_cache_aside_service) -> ReputationDataService:
        return ReputationDataService(fake_cache_aside_service)

    @pytest.fixture
    def seeded_states(self) -> list[ReputationState]:
        # Intentionally unsorted: the service must impose canonical order.
        return [
            _state("variance", 0.70),
            _state("axiom", 0.50),
            _state("pragma", 0.42),
            _state("concord", 0.91),
            _state("nemesis", 0.33),
            _state("sage", 0.60),
            _state("triage", 0.55),
            _state("auditor", 0.80),
        ]

    async def test_genesis_commitment_uses_genesis_prev_root(self, service, seeded_states):
        await _seed_states(service, seeded_states)

        commitment = await commit_reputation(
            reputation_data_service=service,
            tribunal_command_id="tc-1",
            investigation_id="inv-1",
            hmac_key=_FIXED_HMAC_KEY,
        )

        assert commitment.prev_root == GENESIS_PREV_ROOT
        assert commitment.leaves_count == len(seeded_states)
        assert commitment.tribunal_command_id == "tc-1"
        assert commitment.investigation_id == "inv-1"
        assert commitment.signed_by == "auditor"

    async def test_merkle_root_is_deterministic_over_fixed_state(self, service, seeded_states):
        await _seed_states(service, seeded_states)

        commitment = await commit_reputation(
            reputation_data_service=service,
            tribunal_command_id="tc-1",
            investigation_id="inv-1",
            hmac_key=_FIXED_HMAC_KEY,
        )

        assert commitment.merkle_root == _recompute_root(seeded_states)

    async def test_signature_verifies_with_key(self, service, seeded_states):
        await _seed_states(service, seeded_states)

        commitment = await commit_reputation(
            reputation_data_service=service,
            tribunal_command_id="tc-1",
            investigation_id="inv-1",
            hmac_key=_FIXED_HMAC_KEY,
        )

        expected = _expected_signature(
            commitment.merkle_root,
            commitment.prev_root,
            commitment.tribunal_command_id,
            _FIXED_HMAC_KEY,
        )
        assert commitment.signature == expected

    async def test_prev_root_chains_across_sequential_verdicts(self, service, seeded_states):
        await _seed_states(service, seeded_states)

        first = await commit_reputation(
            reputation_data_service=service,
            tribunal_command_id="tc-1",
            investigation_id="inv-1",
            hmac_key=_FIXED_HMAC_KEY,
        )
        # Mutate one scalar so the second root must differ from the first.
        await service.upsert_state(_state("axiom", 0.60))
        second = await commit_reputation(
            reputation_data_service=service,
            tribunal_command_id="tc-2",
            investigation_id="inv-1",
            hmac_key=_FIXED_HMAC_KEY,
        )

        assert second.prev_root == first.merkle_root
        assert second.merkle_root != first.merkle_root

    async def test_empty_scoreboard_has_well_defined_root(self, service):
        commitment = await commit_reputation(
            reputation_data_service=service,
            tribunal_command_id="tc-1",
            investigation_id="inv-1",
            hmac_key=_FIXED_HMAC_KEY,
        )

        assert commitment.leaves_count == 0
        assert commitment.merkle_root == hashlib.sha256(b"").hexdigest()
        assert commitment.prev_root == GENESIS_PREV_ROOT

    async def test_persisted_commitment_is_readable(self, service, seeded_states):
        await _seed_states(service, seeded_states)

        commitment = await commit_reputation(
            reputation_data_service=service,
            tribunal_command_id="tc-1",
            investigation_id="inv-1",
            hmac_key=_FIXED_HMAC_KEY,
        )

        fetched = await service.get_commitment(commitment.id)
        assert fetched is not None
        assert fetched.merkle_root == commitment.merkle_root
        assert fetched.signature == commitment.signature

    async def test_missing_hmac_key_raises(self, service, seeded_states):
        await _seed_states(service, seeded_states)

        with pytest.raises(ValueError, match="hmac_key"):
            await commit_reputation(
                reputation_data_service=service,
                tribunal_command_id="tc-1",
                investigation_id="inv-1",
                hmac_key="",
            )

    async def test_missing_tribunal_command_id_raises(self, service):
        with pytest.raises(ValueError, match="tribunal_command_id"):
            await commit_reputation(
                reputation_data_service=service,
                tribunal_command_id="",
                investigation_id="inv-1",
                hmac_key=_FIXED_HMAC_KEY,
            )

    async def test_missing_investigation_id_raises(self, service):
        with pytest.raises(ValueError, match="investigation_id"):
            await commit_reputation(
                reputation_data_service=service,
                tribunal_command_id="tc-1",
                investigation_id="",
                hmac_key=_FIXED_HMAC_KEY,
            )

    async def test_db_write_failure_propagates_and_leaves_no_commitment(
        self, service, seeded_states, fake_cache_aside_service
    ):
        await _seed_states(service, seeded_states)

        # Force the underlying DB client's create_document to raise.
        async def _boom(*_args, **_kwargs):
            raise RuntimeError("db offline")

        fake_cache_aside_service.db_client.create_document.side_effect = _boom

        with pytest.raises(DatabaseError):
            await commit_reputation(
                reputation_data_service=service,
                tribunal_command_id="tc-1",
                investigation_id="inv-1",
                hmac_key=_FIXED_HMAC_KEY,
            )

        # No commitment should have been persisted.
        latest = await service.get_latest_commitment()
        assert latest is None
