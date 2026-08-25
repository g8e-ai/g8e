# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from unittest.mock import MagicMock
import pytest
from app.constants import CommandGenerationOutcome, AuditorReason
from app.models.agents.tribunal import (
    VoteBreakdown,
)
from app.models.http_context import RequestContext
from app.services.ai.tribunal.emitter import TribunalEmitter
from app.services.ai.tribunal.stages.auditor import TribunalAuditor


@pytest.mark.asyncio
class TestRunAuditStage:
    async def test_auditor_disabled_returns_consensus(
        self, mock_g8e_context, mock_operator_context, mock_reputation_service
    ):
        vote_breakdown = VoteBreakdown(
            candidates_by_member={},
            candidates_by_command={"ls -la": ["axiom"]},
            winner="ls -la",
            winner_supporters=["axiom"],
            dissenters_by_command={},
            consensus_strength=1.0,
        )
        emitter = TribunalEmitter(None, mock_g8e_context)
        auditor = TribunalAuditor(
            emitter=emitter,
            reputation_data_service=mock_reputation_service,
            auditor_hmac_key="a" * 64,
        )

        result = await auditor.run(
            provider=MagicMock(),
            model="test-model",
            request="list files",
            guidelines="",
            vote_winner="ls -la",
            vote_breakdown=vote_breakdown,
            tied_candidates=None,
            operator_context=mock_operator_context,
            auditor_enabled=False,
            command_constraints_message="No whitelist or blacklist constraints are active.",
            investigation_id="inv-1",
            context=RequestContext(
                web_session_id="test-web-session",
                user_id="test-user",
                investigation_id="inv-1",
            ),
        )

        assert result.final_command == "ls -la"
        assert result.outcome == CommandGenerationOutcome.CONSENSUS
        assert result.passed is True
        assert result.revision is None
        assert result.reason == AuditorReason.OK

    async def test_auditor_approves_returns_verified(
        self, make_mock_provider, mock_g8e_context, mock_operator_context, mock_reputation_service
    ):
        vote_breakdown = VoteBreakdown(
            candidates_by_member={},
            candidates_by_command={"ls -la": ["axiom"]},
            winner="ls -la",
            winner_supporters=["axiom"],
            dissenters_by_command={},
            consensus_strength=1.0,
        )
        mock_response = MagicMock()
        mock_response.text = '{"status": "ok"}'
        mock_provider = make_mock_provider(generate_content_lite_return=mock_response)
        emitter = TribunalEmitter(None, mock_g8e_context)
        emitter.correlation_id = "tribunal_test_command"
        auditor = TribunalAuditor(
            emitter=emitter,
            reputation_data_service=mock_reputation_service,
            auditor_hmac_key="a" * 64,
        )

        result = await auditor.run(
            provider=mock_provider,
            model="test-model",
            request="list files",
            guidelines="",
            vote_winner="ls -la",
            vote_breakdown=vote_breakdown,
            tied_candidates=None,
            operator_context=mock_operator_context,
            auditor_enabled=True,
            command_constraints_message="No whitelist or blacklist constraints are active.",
            investigation_id="inv-1",
            context=RequestContext(
                web_session_id="test-web-session",
                user_id="test-user",
                investigation_id="inv-1",
            ),
        )

        assert result.final_command == "ls -la"
        assert result.outcome == CommandGenerationOutcome.VERIFIED
        assert result.passed is True
        assert result.revision is None
        assert result.reason == AuditorReason.OK
        assert result.reputation_commitment_id is not None
