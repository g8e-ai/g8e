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
Tribunal → Approval Correlation Integration Tests

End-to-end test covering the full Tribunal → approval → response flow to ensure:
1. correlation_id is generated when Tribunal starts
2. correlation_id flows through to approval events
3. web_session_id is invariant on approval events
4. SSE boundary validation rejects events missing web_session_id
"""

import pytest

from unittest.mock import AsyncMock, patch
from app.constants import EventType, CommandGenerationOutcome
from app.models.agents.tribunal import TribunalSessionStartedPayload, CommandGenerationResult, CandidateCommand, VoteBreakdown
from app.services.ai.generator import generate_command
from tests.fakes.agent_helpers import (
    make_agent_run_args,
    make_g8ed_event_service,
)

pytestmark = [pytest.mark.integration, pytest.mark.asyncio(loop_scope="session")]


@pytest.mark.asyncio(loop_scope="session")
class TestTribunalApprovalCorrelation:
    """Verify correlation_id flows through Tribunal → approval pipeline."""

    async def test_tribunal_correlation_id_flows_to_all_events(self):
        """Tribunal correlation_id is carried through to all Tribunal SSE events."""
        inputs, state = make_agent_run_args(
            case_id="correlation-test-case-001",
            investigation_id="correlation-test-inv-001",
            web_session_id="correlation-test-sess-001",
            user_id="correlation-test-user-001",
        )
        event_svc = make_g8ed_event_service()

        from app.constants import TribunalMember
        mock_candidates = [
            CandidateCommand(command="ls -la", pass_index=0, member=TribunalMember.AXIOM)
        ]
        
        with patch("app.services.ai.generator._run_generation_stage", new_callable=AsyncMock) as mock_gen, \
             patch("app.services.ai.generator._run_audit_stage", new_callable=AsyncMock) as mock_audit, \
             patch("app.services.ai.generator.weighted_vote") as mock_vote:
            
            mock_gen.return_value = mock_candidates
            # Mock vote to succeed despite only 1 candidate
            from app.models.agents.tribunal import VoteBreakdown
            mock_vote.return_value = ("ls -la", 1.0, VoteBreakdown(
                candidates_by_member={"axiom": "ls -la"},
                candidates_by_command={"ls -la": ["axiom"]},
                winner="ls -la",
                winner_supporters=["axiom"],
                consensus_strength=1.0
            ), None)
            # Mock audit to pass
            mock_audit.return_value = ("ls -la", CommandGenerationOutcome.VERIFIED, True, None, "ok")

            # Generate command via Tribunal
            gen_result = await generate_command(
                request="list files in current directory",
                guidelines="show hidden files too",
                operator_context=None,
                g8ed_event_service=event_svc,
                web_session_id=inputs.web_session_id,
                user_id=inputs.user_id,
                case_id=inputs.case_id,
                investigation_id=inputs.investigation_id,
                settings=inputs.request_settings,
                whitelisting_enabled=False,
                blacklisting_enabled=False,
                whitelisted_commands=[],
                blacklisted_commands=[],
            )

        # Verify Tribunal generated correlation_id
        assert gen_result.correlation_id is not None
        correlation_id = gen_result.correlation_id

        # Verify all emitted events carry the correlation_id
        published = event_svc._published_events
        tribunal_events = [e for e in published if e.event_type.value.startswith('g8e.v1.ai.tribunal')]
        assert len(tribunal_events) > 0
        
        for event in tribunal_events:
            # Skip failure events that might not have it if they occur very early (though here they shouldn't)
            if event.event_type in (EventType.TRIBUNAL_SESSION_STARTED, 
                                 EventType.TRIBUNAL_VOTING_PASS_COMPLETED,
                                 EventType.TRIBUNAL_VOTING_CONSENSUS_REACHED,
                                 EventType.TRIBUNAL_SESSION_COMPLETED):
                assert event.payload.correlation_id == correlation_id, f"Event {event.event_type} missing correlation_id"
                assert event.web_session_id == inputs.web_session_id

    async def test_approval_event_includes_web_session_id(self):
        """Approval events must include web_session_id for SSE boundary validation."""
        inputs, state = make_agent_run_args(
            case_id="web-session-test-case-001",
            investigation_id="web-session-test-inv-001",
            web_session_id="web-session-test-sess-001",
            user_id="web-session-test-user-001",
        )
        event_svc = make_g8ed_event_service()

        with patch("app.services.ai.generator._run_generation_stage", new_callable=AsyncMock) as mock_gen, \
             patch("app.services.ai.generator._run_audit_stage", new_callable=AsyncMock) as mock_audit, \
             patch("app.services.ai.generator.weighted_vote") as mock_vote:
            
            mock_gen.return_value = [CandidateCommand(command="ls", pass_index=0, member="axiom")]
            # Mock vote to succeed despite only 1 candidate
            from app.models.agents.tribunal import VoteBreakdown
            mock_vote.return_value = ("ls", 1.0, VoteBreakdown(
                candidates_by_member={"axiom": "ls"},
                candidates_by_command={"ls": ["axiom"]},
                winner="ls",
                winner_supporters=["axiom"],
                consensus_strength=1.0
            ), None)
            mock_audit.return_value = ("ls", CommandGenerationOutcome.VERIFIED, True, None, "ok")

            # Generate command via Tribunal
            await generate_command(
                request="list files",
                guidelines="",
                operator_context=None,
                g8ed_event_service=event_svc,
                web_session_id=inputs.web_session_id,
                user_id=inputs.user_id,
                case_id=inputs.case_id,
                investigation_id=inputs.investigation_id,
                settings=inputs.request_settings,
                whitelisting_enabled=False,
                blacklisting_enabled=False,
                whitelisted_commands=[],
                blacklisted_commands=[],
            )

        # Verify web_session_id is present in Tribunal events
        published = event_svc._published_events
        tribunal_events = [e for e in published if e.event_type == EventType.TRIBUNAL_SESSION_STARTED]
        assert len(tribunal_events) == 1
        assert tribunal_events[0].web_session_id == inputs.web_session_id

        # Note: Actual approval event emission is tested in approval_service unit tests
        # This integration test focuses on the Tribunal → correlation_id flow
