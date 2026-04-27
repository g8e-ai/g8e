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

from __future__ import annotations

import shlex
import logging

from app.constants import (
    TribunalMember,
    TieBreakReason,
)
from app.models.agents.tribunal import (
    CandidateCommand,
    VoteBreakdown,
)

logger = logging.getLogger(__name__)

TRIBUNAL_MIN_CONSENSUS = 2

from app.utils.command import normalise_command

def weighted_vote(candidates: list[CandidateCommand], total_members: int) -> tuple[str | None, float, VoteBreakdown, list[CandidateCommand] | None]:
    """Compute uniform-weighted majority vote with deterministic tie-breaking.

    Each member contributes exactly 1 vote per candidate (no position decay).
    Tie-breaker ladder:
      1. Shortest command wins (compositional pressure)
      2. Non-Nemesis cluster wins over Nemesis-including cluster
      3. Alphabetical (deterministic fallback)

    consensus_strength is calculated as max_votes / total_members, where total_members
    is the count of members who were asked to produce (not the count of unique candidates).

    Returns (vote_winner, vote_score, vote_breakdown, tied_candidates).
    """
    if not candidates:
        return None, 0.0, VoteBreakdown(
            candidates_by_member={},
            candidates_by_command={},
            winner=None,
            winner_supporters=[],
            dissenters_by_command={},
            consensus_strength=0.0,
            tie_broken=False,
            tie_break_reason=None,
        ), None

    # Group candidates by command (uniform voting - each member = 1 vote)
    candidates_by_command: dict[str, list[str]] = {}
    candidates_by_member: dict[str, str] = {}
    for c in candidates:
        candidates_by_member[c.member.value] = c.command
        if c.command not in candidates_by_command:
            candidates_by_command[c.command] = []
        candidates_by_command[c.command].append(c.member.value)

    # Calculate vote counts (uniform - each member = 1 vote)
    vote_counts = {cmd: len(members) for cmd, members in candidates_by_command.items()}
    max_votes = max(vote_counts.values()) if vote_counts else 0

    # Check consensus threshold
    if max_votes < TRIBUNAL_MIN_CONSENSUS:
        return None, 0.0, VoteBreakdown(
            candidates_by_member=candidates_by_member,
            candidates_by_command=candidates_by_command,
            winner=None,
            winner_supporters=[],
            dissenters_by_command=candidates_by_command,
            consensus_strength=max_votes / total_members if total_members > 0 else 0.0,
            tie_broken=False,
            tie_break_reason=None,
        ), None

    # Find candidates with max votes (potential tie)
    top_candidates = [cmd for cmd, count in vote_counts.items() if count == max_votes]

    if len(top_candidates) == 1:
        winner = top_candidates[0]
        dissenters = {cmd: members for cmd, members in candidates_by_command.items() if cmd != winner}
        return winner, max_votes / total_members, VoteBreakdown(
            candidates_by_member=candidates_by_member,
            candidates_by_command=candidates_by_command,
            winner=winner,
            winner_supporters=candidates_by_command[winner],
            dissenters_by_command=dissenters,
            consensus_strength=max_votes / total_members,
            tie_broken=False,
            tie_break_reason=None,
        ), None

    # Tie detected - apply tie-breaker ladder
    # 1. Shortest command wins (compositional pressure)
    shortest_cmd = min(top_candidates, key=len)
    if len(set(len(c) for c in top_candidates)) > 1:
        # Multiple lengths, pick shortest
        winner = shortest_cmd
        dissenters = {cmd: members for cmd, members in candidates_by_command.items() if cmd != winner}
        tied_candidates = [CandidateCommand(command=cmd, pass_index=0, member=TribunalMember.AXIOM) for cmd in top_candidates]
        return winner, max_votes / total_members, VoteBreakdown(
            candidates_by_member=candidates_by_member,
            candidates_by_command=candidates_by_command,
            winner=winner,
            winner_supporters=candidates_by_command[winner],
            dissenters_by_command=dissenters,
            consensus_strength=max_votes / total_members,
            tie_broken=True,
            tie_break_reason=TieBreakReason.SHORTEST,
        ), tied_candidates

    # 2. Non-Nemesis cluster wins over Nemesis-including cluster
    nemesis_members = [TribunalMember.NEMESIS.value]
    non_nemesis_candidates = [
        cmd for cmd in top_candidates
        if not any(m in nemesis_members for m in candidates_by_command[cmd])
    ]
    nemesis_including_candidates = [
        cmd for cmd in top_candidates
        if any(m in nemesis_members for m in candidates_by_command[cmd])
    ]
    if non_nemesis_candidates and nemesis_including_candidates:
        winner = non_nemesis_candidates[0]
        dissenters = {cmd: members for cmd, members in candidates_by_command.items() if cmd != winner}
        tied_candidates = [CandidateCommand(command=cmd, pass_index=0, member=TribunalMember.AXIOM) for cmd in top_candidates]
        return winner, max_votes / total_members, VoteBreakdown(
            candidates_by_member=candidates_by_member,
            candidates_by_command=candidates_by_command,
            winner=winner,
            winner_supporters=candidates_by_command[winner],
            dissenters_by_command=dissenters,
            consensus_strength=max_votes / total_members,
            tie_broken=True,
            tie_break_reason=TieBreakReason.EXCLUDED_NEMESIS,
        ), tied_candidates

    # 3. Alphabetical fallback
    winner = sorted(top_candidates)[0]
    dissenters = {cmd: members for cmd, members in candidates_by_command.items() if cmd != winner}
    tied_candidates = [CandidateCommand(command=cmd, pass_index=0, member=TribunalMember.AXIOM) for cmd in top_candidates]
    return winner, max_votes / total_members, VoteBreakdown(
        candidates_by_member=candidates_by_member,
        candidates_by_command=candidates_by_command,
        winner=winner,
        winner_supporters=candidates_by_command[winner],
        dissenters_by_command=dissenters,
        consensus_strength=max_votes / total_members,
        tie_broken=True,
        tie_break_reason=TieBreakReason.ALPHABETICAL,
    ), tied_candidates
