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
End-to-end integration test for Tribunal command generation with reputation commitment.

This test verifies the full Phase 2 pipeline:
1. Command generation via Tribunal (mocked stages)
2. Automatic reputation commitment creation on auditor success
3. SSE event emission for the commitment
4. Commitment ID propagation to the final result
"""

import pytest
from unittest.mock import AsyncMock, patch, MagicMock

from datetime import datetime, UTC
from app.constants import (
    CommandGenerationOutcome,
    EventType,
    TribunalMember,
    AuditorReason,
)
from app.models.agents.tribunal import (
    CandidateCommand,
    VoteBreakdown,
    CommandGenerationResult,
)
from app.models.reputation import ReputationCommitment, ReputationState
from app.services.ai.generator import generate_command
from tests.fakes.agent_helpers import (
    make_agent_run_args,
    make_g8ed_event_service,
)
from app.services.data.reputation_data_service import ReputationDataService

pytestmark = [pytest.mark.integration, pytest.mark.asyncio(loop_scope="session")]

_TEST_HMAC_KEY = "a" * 64

@pytest.mark.asyncio(loop_scope="session")
class TestCommandGeneratorWithCommitment:
    """Verify reputation commitment flows through the Tribunal generator pipeline."""

    async def test_successful_audit_triggers_reputation_commitment(self, fake_cache_aside_service):
        """A verified Tribunal outcome must produce a reputation commitment."""
        inputs, _ = make_agent_run_args(
            case_id="commitment-test-case-001",
            investigation_id="commitment-test-inv-001",
            web_session_id="commitment-test-sess-001",
            user_id="commitment-test-user-001",
        )
        event_svc = make_g8ed_event_service()
        
        # Use real ReputationDataService with fake cache/db
        reputation_svc = ReputationDataService(fake_cache_aside_service)
        
        # Seed some initial reputation state
        await reputation_svc.upsert_state(ReputationState(
            agent_id="axiom",
            scalar=0.5,
            updated_at=datetime.now(UTC)
        ))

        mock_candidates = [
            CandidateCommand(command="ls -la", pass_index=0, member=TribunalMember.AXIOM)
        ]
        
        with patch("app.services.ai.generator._run_generation_stage", new_callable=AsyncMock) as mock_gen, \
             patch("app.services.ai.generator._run_voting_stage", new_callable=AsyncMock) as mock_vote, \
             patch("app.services.ai.generator.run_auditor", new_callable=AsyncMock) as mock_run_auditor:
            
            mock_gen.return_value = mock_candidates
            
            # Mock voting stage result
            vote_breakdown = MagicMock(spec=VoteBreakdown)
            vote_breakdown.candidates_by_member = {"axiom": "ls -la"}
            vote_breakdown.candidates_by_command = {"ls -la": ["axiom"]}
            vote_breakdown.winner = "ls -la"
            vote_breakdown.winner_supporters = ["axiom"]
            vote_breakdown.consensus_strength = 1.0
            vote_breakdown.tie_break_reason = None
            
            mock_vote.return_value = ("ls -la", 1.0, vote_breakdown, None)
            
            # Mock auditor to pass
            mock_run_auditor.return_value = (True, "ls -la", None, AuditorReason.OK, None, None)

            # Generate command via Tribunal
            gen_result = await generate_command(
                request="list files",
                guidelines="",
                operator_context=None,
                g8ed_event_service=event_svc,
                web_session_id=inputs.web_session_id,
                user_id=inputs.user_id,
                case_id=inputs.case_id,
                investigation_id=inputs.investigation_id,
                settings=inputs.request_settings,
                reputation_data_service=reputation_svc,
                auditor_hmac_key=_TEST_HMAC_KEY,
                whitelisting_enabled=False,
                blacklisting_enabled=False,
            )

        # 1. Verify commitment was created in DB
        assert gen_result.reputation_commitment_id is not None
        commitment = await reputation_svc.get_commitment(gen_result.reputation_commitment_id)
        assert commitment is not None
        assert commitment.investigation_id == inputs.investigation_id
        assert commitment.tribunal_command_id == gen_result.correlation_id

        # 2. Verify SSE event was emitted
        published = event_svc._published_events
        commitment_events = [e for e in published if e.event_type == EventType.REPUTATION_COMMITMENT_CREATED]
        assert len(commitment_events) == 1
        payload = commitment_events[0].payload
        assert payload.commitment_id == gen_result.reputation_commitment_id
        assert payload.correlation_id == gen_result.correlation_id

    async def test_failed_audit_skips_reputation_commitment(self, fake_cache_aside_service):
        """An unverified Tribunal outcome must NOT produce a reputation commitment."""
        inputs, _ = make_agent_run_args(
            case_id="commitment-fail-case-001",
            investigation_id="commitment-fail-inv-001",
        )
        event_svc = make_g8ed_event_service()
        reputation_svc = ReputationDataService(fake_cache_aside_service)

        with patch("app.services.ai.generator._run_generation_stage", new_callable=AsyncMock) as mock_gen, \
             patch("app.services.ai.generator._run_voting_stage", new_callable=AsyncMock) as mock_vote, \
             patch("app.services.ai.generator.run_auditor", new_callable=AsyncMock) as mock_run_auditor:
            
            mock_gen.return_value = [CandidateCommand(command="rm -rf /", pass_index=0, member=TribunalMember.AXIOM)]
            
            vote_breakdown = MagicMock(spec=VoteBreakdown)
            vote_breakdown.consensus_strength = 1.0
            vote_breakdown.tie_break_reason = None
            mock_vote.return_value = ("rm -rf /", 1.0, vote_breakdown, None)
            
            # Mock auditor to REJECT
            mock_run_auditor.return_value = (False, None, None, AuditorReason.WHITELIST_VIOLATION, None, None)

            gen_result = await generate_command(
                request="delete all",
                guidelines="",
                operator_context=None,
                g8ed_event_service=event_svc,
                web_session_id=inputs.web_session_id,
                user_id=inputs.user_id,
                case_id=inputs.case_id,
                investigation_id=inputs.investigation_id,
                settings=inputs.request_settings,
                reputation_data_service=reputation_svc,
                auditor_hmac_key=_TEST_HMAC_KEY,
            )

        # Verify no commitment ID
        assert gen_result.reputation_commitment_id is None
        
        # Verify no commitment events
        published = event_svc._published_events
        commitment_events = [e for e in published if e.event_type == EventType.REPUTATION_COMMITMENT_CREATED]
        assert len(commitment_events) == 0

    async def test_commitment_failure_is_fatal_to_command_generation(self, fake_cache_aside_service):
        """A failure in the commitment step should crash the generator (prevents ghost verdicts)."""
        inputs, _ = make_agent_run_args()
        event_svc = make_g8ed_event_service()
        reputation_svc = ReputationDataService(fake_cache_aside_service)

        # Force commitment failure by mocking create_commitment to raise
        reputation_svc.create_commitment = AsyncMock(side_effect=RuntimeError("DB Offline"))

        with patch("app.services.ai.generator._run_generation_stage", new_callable=AsyncMock) as mock_gen, \
             patch("app.services.ai.generator._run_voting_stage", new_callable=AsyncMock) as mock_vote, \
             patch("app.services.ai.generator.run_auditor", new_callable=AsyncMock) as mock_run_auditor:

            mock_gen.return_value = [CandidateCommand(command="ls", pass_index=0, member=TribunalMember.AXIOM)]

            vote_breakdown = MagicMock(spec=VoteBreakdown)
            vote_breakdown.consensus_strength = 1.0
            vote_breakdown.tie_break_reason = None
            mock_vote.return_value = ("ls", 1.0, vote_breakdown, None)

            mock_run_auditor.return_value = (True, "ls", None, AuditorReason.OK, None, None)

            with pytest.raises(RuntimeError, match="Reputation commitment failed"):
                await generate_command(
                    request="list",
                    guidelines="",
                    operator_context=None,
                    g8ed_event_service=event_svc,
                    web_session_id=inputs.web_session_id,
                    user_id=inputs.user_id,
                    case_id=inputs.case_id,
                    investigation_id=inputs.investigation_id,
                    settings=inputs.request_settings,
                    reputation_data_service=reputation_svc,
                    auditor_hmac_key=_TEST_HMAC_KEY,
                )

        # Should emit a failure event
        published = event_svc._published_events
        fail_events = [e for e in published if e.event_type == EventType.REPUTATION_COMMITMENT_FAILED]
        assert len(fail_events) == 1
        assert "DB Offline" in fail_events[0].payload.error
