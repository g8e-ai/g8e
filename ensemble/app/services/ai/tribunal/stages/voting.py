# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import logging
from app.constants import EventType
from app.models.agents.tribunal import (
    CandidateCommand,
    VoteBreakdown,
    TribunalConsensusFailedPayload,
    TribunalVotingCompletedPayload,
    TribunalDissentRecordedPayload,
)
from app.services.ai.voter import (
    TRIBUNAL_MIN_CONSENSUS,
    weighted_vote,
)
from app.services.ai.tribunal.emitter import TribunalEmitter

logger = logging.getLogger(__name__)


async def _run_voting_stage(
    candidates: list[CandidateCommand],
    request: str,
    emitter: TribunalEmitter,
    total_members: int,
    is_final: bool = True,
) -> tuple[str | None, float, VoteBreakdown, list[CandidateCommand] | None]:
    """Stage 2: compute weighted majority vote and emit consensus event."""
    vote_winner, vote_score, vote_breakdown, tied_candidates = weighted_vote(
        candidates, total_members
    )

    if vote_winner is None:
        if vote_breakdown.consensus_strength > 0:
            logger.warning(
                "[TRIBUNAL] Consensus strength too low: %.2f < %d members",
                vote_breakdown.consensus_strength,
                TRIBUNAL_MIN_CONSENSUS,
            )
            logger.info("[TRIBUNAL-TELEMETRY] Candidate breakdown for consensus failure:")
            for member, cmd in vote_breakdown.candidates_by_member.items():
                logger.info(
                    "[TRIBUNAL-TELEMETRY]   %s: %s",
                    member,
                    cmd[:200] + "..." if len(cmd) > 200 else cmd,
                )
        else:
            logger.warning("[TRIBUNAL] Consensus failed: no agreement among members")
            logger.info("[TRIBUNAL-TELEMETRY] Candidate breakdown for consensus failure:")
            for member, cmd in vote_breakdown.candidates_by_member.items():
                logger.info(
                    "[TRIBUNAL-TELEMETRY]   %s: %s",
                    member,
                    cmd[:200] + "..." if len(cmd) > 200 else cmd,
                )

        event_type = (
            EventType.AI_CONSENSUS_VOTING_CONSENSUS_FAILED
            if is_final
            else EventType.AI_CONSENSUS_VOTING_CONSENSUS_NOT_REACHED
        )
        await emitter.emit(
            event_type,
            TribunalConsensusFailedPayload(
                request=request,
                vote_breakdown=vote_breakdown,
            ),
        )
    else:
        await emitter.emit(
            EventType.AI_CONSENSUS_VOTING_CONSENSUS_REACHED,
            TribunalVotingCompletedPayload(
                vote_winner=vote_winner,
                vote_score=vote_score,
                num_candidates=len(candidates),
                request=request,
                vote_breakdown=vote_breakdown,
            ),
        )

        for cmd, members in vote_breakdown.dissenters_by_command.items():
            await emitter.emit(
                EventType.AI_CONSENSUS_VOTING_DISSENT_RECORDED,
                TribunalDissentRecordedPayload(
                    request=request,
                    losing_command=cmd,
                    dissenting_member_ids=members,
                    winner=vote_winner,
                    vote_breakdown=vote_breakdown,
                ),
            )

    return vote_winner, vote_score, vote_breakdown, tied_candidates
