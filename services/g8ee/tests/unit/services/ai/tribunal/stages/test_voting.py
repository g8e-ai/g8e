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

import pytest
from app.constants import TieBreakReason, TribunalMember
from app.models.agents.tribunal import CandidateCommand
from app.services.ai.tribunal.emitter import TribunalEmitter
from app.services.ai.tribunal.stages.voting import _run_voting_stage

@pytest.mark.asyncio
class TestRunVotingStage:
    async def test_returns_winner_and_score_with_breakdown(self, mock_g8e_context):
        candidates = [
            CandidateCommand(command="ls -la", pass_index=0, member=TribunalMember.AXIOM),
            CandidateCommand(command="ls -la", pass_index=1, member=TribunalMember.CONCORD),
            CandidateCommand(command="ls -l", pass_index=2, member=TribunalMember.VARIANCE),
            CandidateCommand(command="ls -la", pass_index=3, member=TribunalMember.PRAGMA),
            CandidateCommand(command="ls -l", pass_index=4, member=TribunalMember.NEMESIS),
        ]
        emitter = TribunalEmitter(None, mock_g8e_context)

        winner, score, vote_breakdown, tied_candidates = await _run_voting_stage(
            candidates=candidates, request="list files", emitter=emitter, total_members=5,
        )

        assert winner == "ls -la"
        assert score == 0.6
        assert vote_breakdown is not None
        assert vote_breakdown.winner == "ls -la"
        assert vote_breakdown.consensus_strength == 0.6
        assert len(vote_breakdown.winner_supporters) == 3
        assert tied_candidates is None

    async def test_single_cluster_unanimous_wins(self, mock_g8e_context):
        candidates = [
            CandidateCommand(command="ls -la", pass_index=0, member=TribunalMember.AXIOM),
            CandidateCommand(command="ls -la", pass_index=1, member=TribunalMember.CONCORD),
            CandidateCommand(command="ls -la", pass_index=2, member=TribunalMember.VARIANCE),
            CandidateCommand(command="ls -la", pass_index=3, member=TribunalMember.PRAGMA),
            CandidateCommand(command="ls -la", pass_index=4, member=TribunalMember.NEMESIS),
        ]
        emitter = TribunalEmitter(None, mock_g8e_context)

        winner, score, vote_breakdown, tied_candidates = await _run_voting_stage(
            candidates=candidates, request="list files", emitter=emitter, total_members=5,
        )

        assert winner == "ls -la"
        assert score == 1.0
        assert vote_breakdown.consensus_strength == 1.0
        assert tied_candidates is None

    async def test_consensus_failed_returns_none_winner(self, mock_g8e_context):
        candidates = [
            CandidateCommand(command="ls -la", pass_index=0, member=TribunalMember.AXIOM),
            CandidateCommand(command="ls -l", pass_index=1, member=TribunalMember.CONCORD),
            CandidateCommand(command="ls", pass_index=2, member=TribunalMember.VARIANCE),
            CandidateCommand(command="ll", pass_index=3, member=TribunalMember.PRAGMA),
            CandidateCommand(command="rm -rf", pass_index=4, member=TribunalMember.NEMESIS),
        ]
        emitter = TribunalEmitter(None, mock_g8e_context)

        winner, score, vote_breakdown, tied_candidates = await _run_voting_stage(
            candidates=candidates, request="list files", emitter=emitter, total_members=5,
        )

        assert winner is None
        assert score == 0.0
        assert vote_breakdown.consensus_strength == 0.2
        assert vote_breakdown.winner is None
        assert tied_candidates is None

    async def test_two_two_one_tied_top_breaks_by_shortest_command(self, mock_g8e_context):
        candidates = [
            CandidateCommand(command="docker ps -a && docker images && docker volume ls && docker network ls && docker system df", pass_index=0, member=TribunalMember.AXIOM),
            CandidateCommand(command="docker ps -a && docker images && docker volume ls && docker network ls && docker system df", pass_index=1, member=TribunalMember.CONCORD),
            CandidateCommand(command="docker ps", pass_index=2, member=TribunalMember.VARIANCE),
            CandidateCommand(command="docker ps", pass_index=3, member=TribunalMember.PRAGMA),
            CandidateCommand(command="docker images", pass_index=4, member=TribunalMember.NEMESIS),
        ]
        emitter = TribunalEmitter(None, mock_g8e_context)

        winner, score, vote_breakdown, tied_candidates = await _run_voting_stage(
            candidates=candidates, request="check docker state", emitter=emitter, total_members=5,
        )

        assert winner == "docker ps"
        assert score == 0.4
        assert vote_breakdown.consensus_strength == 0.4
        assert vote_breakdown.tie_broken is True
        assert vote_breakdown.tie_break_reason == TieBreakReason.SHORTEST
        assert len(vote_breakdown.winner_supporters) == 2
        assert tied_candidates is not None
        assert len(tied_candidates) == 2
