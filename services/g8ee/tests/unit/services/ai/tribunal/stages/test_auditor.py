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

from unittest.mock import MagicMock
import pytest
from app.constants import CommandGenerationOutcome, AuditorReason
from app.models.agents.tribunal import (
    VoteBreakdown,
)
from app.services.ai.tribunal.emitter import TribunalEmitter
from app.services.ai.tribunal.stages.auditor import TribunalAuditor

@pytest.mark.asyncio
class TestRunAuditStage:
    async def test_auditor_disabled_returns_consensus(self, mock_g8e_context, mock_operator_context, mock_reputation_service):
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
            auditor_hmac_key="a"*64,
        )

        result = await auditor.run(
            provider=MagicMock(), model="test-model", request="list files", guidelines="",
            vote_winner="ls -la", vote_breakdown=vote_breakdown, tied_candidates=None,
            operator_context=mock_operator_context,
            auditor_enabled=False,
            command_constraints_message="No whitelist or blacklist constraints are active.",
            investigation_id="inv-1",
        )

        assert result.final_command == "ls -la"
        assert result.outcome == CommandGenerationOutcome.CONSENSUS
        assert result.passed is True
        assert result.revision is None
        assert result.reason == AuditorReason.OK

    async def test_auditor_approves_returns_verified(self, make_mock_provider, mock_g8e_context, mock_operator_context, mock_reputation_service):
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
            auditor_hmac_key="a"*64,
        )

        result = await auditor.run(
            provider=mock_provider, model="test-model", request="list files", guidelines="",
            vote_winner="ls -la", vote_breakdown=vote_breakdown, tied_candidates=None,
            operator_context=mock_operator_context,
            auditor_enabled=True,
            command_constraints_message="No whitelist or blacklist constraints are active.",
            investigation_id="inv-1",
        )

        assert result.final_command == "ls -la"
        assert result.outcome == CommandGenerationOutcome.VERIFIED
        assert result.passed is True
        assert result.revision is None
        assert result.reason == AuditorReason.OK
        assert result.reputation_commitment_id is not None
