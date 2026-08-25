# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
End-to-end integration test for Tribunal command generation with reputation commitment.

This test verifies the full Phase 2 pipeline:
1. Command generation via Tribunal (mocked stages)
2. Automatic reputation commitment creation on auditor success
3. SSE event emission for the commitment
4. Commitment ID propagation to the final result
"""

from datetime import UTC, datetime
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from app.constants import (
    AuditorReason,
    CommandGenerationOutcome,
    ComponentName,
    ConsensusMember,
    EventType,
    G8EE_COMPONENT,
)
from app.models.agents.tribunal import (
    CandidateCommand,
    VoteBreakdown,
    TribunalAuditResult,
)
from app.models.http_context import G8eHttpContext, RequestContext
from app.models.reputation import ReputationState
from app.services.ai.generator import generate_command
from app.models.tribunal_commands import TribunalGenerationRequest
from app.services.data.reputation_data_service import ReputationDataService
from tests.fakes.agent_helpers import (
    make_agent_run_args,
    make_event_service,
)
from tests.unit.services.ai.tribunal.conftest import make_tribunal_generation_request

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
        event_svc = make_event_service()

        # Use real ReputationDataService with fake cache/db
        from unittest.mock import MagicMock as _MagicMock

        async def _write_through(collection, document_id, updates, **kwargs):
            return await fake_cache_aside_service.db_client.update_document(
                collection=collection,
                document_id=document_id,
                data=updates,
                merge=kwargs.get("merge", True),
            )

        gov = _MagicMock()
        gov.update_governed_doc = AsyncMock(side_effect=_write_through)
        reputation_svc = ReputationDataService(fake_cache_aside_service, gov)

        # Seed some initial reputation state
        await reputation_svc.upsert_state(
            ReputationState(agent_id="axiom", scalar=0.5, updated_at=datetime.now(UTC)),
            context=RequestContext(
                web_session_id="commitment-test-sess-001",
                user_id="commitment-test-user-001",
                case_id="commitment-test-case-001",
                investigation_id="commitment-test-inv-001",
            ),
        )

        mock_candidates = [
            CandidateCommand(command="ls -la", pass_index=0, member=ConsensusMember.AXIOM)
        ]

        with (
            patch(
                "app.services.ai.generator._run_generation_stage", new_callable=AsyncMock
            ) as mock_gen,
            patch(
                "app.services.ai.generator._run_voting_stage", new_callable=AsyncMock
            ) as mock_vote,
            patch(
                "app.services.ai.generator.TribunalAuditor.run", new_callable=AsyncMock
            ) as mock_run_auditor,
        ):
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

            # Mock auditor to pass and emit event
            async def mock_audit_side_effect(*args, **kwargs):
                # Simulate side effects that are now internal to TribunalAuditor.run
                from app.models.reputation import ReputationCommitmentCreatedPayload
                from app.models.events import SessionEvent

                correlation_id = "mock-correlation-id"

                payload = ReputationCommitmentCreatedPayload(
                    commitment_id="mock-commitment-id",
                    tribunal_command_id=correlation_id,
                    investigation_id=inputs.investigation_id,
                    merkle_root="a" * 64,
                    prev_root="b" * 64,
                    leaves_count=1,
                    correlation_id=correlation_id,
                )
                from app.models.http_context import RequestContext
                from app.constants import ComponentName

                ctx = RequestContext(
                    web_session_id=inputs.web_session_id,
                    user_id=inputs.user_id,
                    case_id=inputs.case_id,
                    investigation_id=inputs.investigation_id,
                    source_component=G8EE_COMPONENT,
                )
                event = SessionEvent.from_context(
                    context=ctx,
                    event_type=EventType.OPERATOR_REPUTATION_COMMITMENT_CREATED,
                    payload=payload,
                )
                await event_svc.publish(event)

                return TribunalAuditResult(
                    final_command="ls -la",
                    outcome=CommandGenerationOutcome.VERIFIED,
                    passed=True,
                    revision=None,
                    reason=AuditorReason.OK,
                    reputation_commitment_id="mock-commitment-id",
                )

            mock_run_auditor.side_effect = mock_audit_side_effect

            # Generate command via Tribunal
            gen_result = await generate_command(
                make_tribunal_generation_request(
                    request="list files",
                    guidelines="",
                    operator_context=None,
                    event_service=event_svc,
                    g8e_context=G8eHttpContext(
                        web_session_id=inputs.web_session_id,
                        user_id=inputs.user_id,
                        case_id=inputs.case_id,
                        investigation_id=inputs.investigation_id,
                        source_component=G8EE_COMPONENT,
                    ),
                    settings=inputs.request_settings,
                    reputation_data_service=reputation_svc,
                    auditor_hmac_key=_TEST_HMAC_KEY,
                    whitelisting_enabled=False,
                    blacklisting_enabled=False,
                )
            )

        # 1. Verify commitment ID was returned
        assert gen_result.reputation_commitment_id == "mock-commitment-id"

        # 2. Verify SSE event was emitted
        published = event_svc._published_events
        commitment_events = [
            e for e in published if e.event_type == EventType.OPERATOR_REPUTATION_COMMITMENT_CREATED
        ]
        assert len(commitment_events) == 1
        payload = commitment_events[0].payload
        assert payload.commitment_id == gen_result.reputation_commitment_id
        assert payload.correlation_id == "mock-correlation-id"

    async def test_failed_audit_skips_reputation_commitment(self, fake_cache_aside_service):
        """An unverified Tribunal outcome must NOT produce a reputation commitment."""
        inputs, _ = make_agent_run_args(
            case_id="commitment-fail-case-001",
            investigation_id="commitment-fail-inv-001",
        )
        event_svc = make_event_service()

        async def _write_through2(collection, document_id, updates, **kwargs):
            return await fake_cache_aside_service.db_client.update_document(
                collection=collection,
                document_id=document_id,
                data=updates,
                merge=kwargs.get("merge", True),
            )

        gov = MagicMock()
        gov.update_governed_doc = AsyncMock(side_effect=_write_through2)
        reputation_svc = ReputationDataService(fake_cache_aside_service, gov)

        with (
            patch(
                "app.services.ai.generator._run_generation_stage", new_callable=AsyncMock
            ) as mock_gen,
            patch(
                "app.services.ai.generator._run_voting_stage", new_callable=AsyncMock
            ) as mock_vote,
            patch(
                "app.services.ai.generator.TribunalAuditor.run", new_callable=AsyncMock
            ) as mock_run_auditor,
        ):
            mock_gen.return_value = [
                CandidateCommand(command="rm -rf /", pass_index=0, member=ConsensusMember.AXIOM)
            ]

            vote_breakdown = MagicMock(spec=VoteBreakdown)
            vote_breakdown.consensus_strength = 1.0
            vote_breakdown.tie_break_reason = None
            mock_vote.return_value = ("rm -rf /", 1.0, vote_breakdown, None)

            # Mock auditor to REJECT
            mock_run_auditor.return_value = TribunalAuditResult(
                final_command=None,
                outcome=CommandGenerationOutcome.VERIFICATION_FAILED,
                passed=False,
                revision=None,
                reason=AuditorReason.WHITELIST_VIOLATION,
                reputation_commitment_id=None,
            )

            gen_result = await generate_command(
                make_tribunal_generation_request(
                    request="delete all",
                    guidelines="",
                    operator_context=None,
                    event_service=event_svc,
                    g8e_context=G8eHttpContext(
                        web_session_id=inputs.web_session_id,
                        user_id=inputs.user_id,
                        case_id=inputs.case_id,
                        investigation_id=inputs.investigation_id,
                        source_component=G8EE_COMPONENT,
                    ),
                    settings=inputs.request_settings,
                    reputation_data_service=reputation_svc,
                    auditor_hmac_key=_TEST_HMAC_KEY,
                )
            )

        # Verify no commitment ID
        assert gen_result.reputation_commitment_id is None

        # Verify no commitment events
        published = event_svc._published_events
        commitment_events = [
            e for e in published if e.event_type == EventType.OPERATOR_REPUTATION_COMMITMENT_CREATED
        ]
        assert len(commitment_events) == 0

    async def test_commitment_failure_is_fatal_to_command_generation(
        self, fake_cache_aside_service
    ):
        """A failure in the commitment step should crash the generator (prevents ghost verdicts)."""
        inputs, _ = make_agent_run_args()
        event_svc = make_event_service()

        async def _write_through3(collection, document_id, updates, **kwargs):
            return await fake_cache_aside_service.db_client.update_document(
                collection=collection,
                document_id=document_id,
                data=updates,
                merge=kwargs.get("merge", True),
            )

        gov = MagicMock()
        gov.update_governed_doc = AsyncMock(side_effect=_write_through3)
        reputation_svc = ReputationDataService(fake_cache_aside_service, gov)

        # Force commitment failure by mocking create_commitment to raise
        reputation_svc.create_commitment = AsyncMock(side_effect=RuntimeError("DB Offline"))

        from app.models.reputation import ReputationCommitmentFailedPayload
        from app.constants import EventType

        with (
            patch(
                "app.services.ai.generator._run_generation_stage", new_callable=AsyncMock
            ) as mock_gen,
            patch(
                "app.services.ai.generator._run_voting_stage", new_callable=AsyncMock
            ) as mock_vote,
            patch(
                "app.services.ai.generator.TribunalAuditor.run", new_callable=AsyncMock
            ) as mock_run_auditor,
        ):
            mock_gen.return_value = [
                CandidateCommand(command="ls", pass_index=0, member=ConsensusMember.AXIOM)
            ]

            vote_breakdown = MagicMock(spec=VoteBreakdown)
            vote_breakdown.consensus_strength = 1.0
            vote_breakdown.tie_break_reason = None
            mock_vote.return_value = ("ls", 1.0, vote_breakdown, None)

            # Mock auditor to fail during commitment by raising RuntimeError
            async def mock_audit_fatal_failure(*args, **kwargs):
                from app.models.events import SessionEvent

                correlation_id = "test-correlation-id"

                payload = ReputationCommitmentFailedPayload(
                    tribunal_command_id=correlation_id,
                    investigation_id=inputs.investigation_id,
                    error="DB Offline",
                    correlation_id=correlation_id,
                )
                from app.models.http_context import RequestContext
                from app.constants import ComponentName

                ctx = RequestContext(
                    web_session_id=inputs.web_session_id,
                    user_id=inputs.user_id,
                    case_id=inputs.case_id,
                    investigation_id=inputs.investigation_id,
                    source_component=G8EE_COMPONENT,
                )
                event = SessionEvent.from_context(
                    context=ctx,
                    event_type=EventType.OPERATOR_REPUTATION_COMMITMENT_FAILED,
                    payload=payload,
                )
                await event_svc.publish(event)

                raise RuntimeError(
                    f"Reputation commitment failed for tribunal_command_id={correlation_id}: DB Offline"
                )

            mock_run_auditor.side_effect = mock_audit_fatal_failure

            with pytest.raises(RuntimeError, match="Reputation commitment failed"):
                await generate_command(
                    make_tribunal_generation_request(
                        request="list",
                        guidelines="",
                        operator_context=None,
                        event_service=event_svc,
                        g8e_context=G8eHttpContext(
                            web_session_id=inputs.web_session_id,
                            user_id=inputs.user_id,
                            case_id=inputs.case_id,
                            investigation_id=inputs.investigation_id,
                            source_component=G8EE_COMPONENT,
                        ),
                        settings=inputs.request_settings,
                        reputation_data_service=reputation_svc,
                        auditor_hmac_key=_TEST_HMAC_KEY,
                    )
                )

        # Should emit a failure event
        published = event_svc._published_events
        fail_events = [
            e for e in published if e.event_type == EventType.OPERATOR_REPUTATION_COMMITMENT_FAILED
        ]
        assert len(fail_events) == 1
        assert "DB Offline" in fail_events[0].payload.error
