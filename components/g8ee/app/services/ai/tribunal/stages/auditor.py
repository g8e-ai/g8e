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

import logging
from app.constants import (
    CommandGenerationOutcome,
    EventType,
    AuditorReason,
)
from app.llm.provider import LLMProvider
from app.models.agent import OperatorContext
from app.models.reputation import (
    ReputationCommitmentCreatedPayload,
    ReputationCommitmentFailedPayload,
)
from app.models.agents.tribunal import (
    CandidateCommand,
    VoteBreakdown,
)
from app.utils.agent_persona_loader import get_agent_persona
from app.services.ai.auditor_service import (
    commit_reputation,
    run_auditor,
)
from app.services.data.reputation_data_service import ReputationDataService
from app.services.ai.tribunal.emitter import TribunalEmitter

logger = logging.getLogger(__name__)

async def _run_audit_stage(
    provider: LLMProvider,
    model: str,
    request: str,
    guidelines: str,
    vote_winner: str,
    vote_breakdown: VoteBreakdown,
    operator_context: OperatorContext | None,
    auditor_enabled: bool,
    emitter: TribunalEmitter,
    command_constraints_message: str,
    reputation_data_service: ReputationDataService,
    auditor_hmac_key: str,
    investigation_id: str,
    tied_candidates: list[CandidateCommand] | None = None,
    whitelisting_enabled: bool = False,
    blacklisting_enabled: bool = False,
) -> tuple[str | None, CommandGenerationOutcome, bool, str | None, AuditorReason, str | None]:
    """Stage 3b: Auditor verification + reputation commitment of the consensus winner.

    Runs the Auditor against the vote winner. On a verified verdict, binds
    the outcome to the reputation scoreboard via a cryptographic commitment
    -- a commitment failure is fatal and aborts the verdict.
    """
    if not auditor_enabled:
        return vote_winner, CommandGenerationOutcome.CONSENSUS, True, None, AuditorReason.OK, None

    if vote_breakdown.consensus_strength == 1.0:
        mode = "unanimous"
    else:
        mode = "majority"

    auditor_passed, final_command, auditor_revision, auditor_reason, swap_to_cluster, swap_to_member = await run_auditor(
        provider=provider,
        model=model,
        request=request,
        guidelines=guidelines,
        mode=mode,
        vote_winner=vote_winner,
        vote_breakdown=vote_breakdown,
        tied_candidates=tied_candidates,
        operator_context=operator_context,
        emitter=emitter,
        command_constraints_message=command_constraints_message,
        auditor_persona=get_agent_persona("auditor"),
        whitelisting_enabled=whitelisting_enabled,
        blacklisting_enabled=blacklisting_enabled,
    )

    outcome = CommandGenerationOutcome.VERIFIED if auditor_passed else CommandGenerationOutcome.VERIFICATION_FAILED

    if not auditor_passed and auditor_reason == AuditorReason.REVISED:
        outcome = CommandGenerationOutcome.VERIFICATION_FAILED

    commitment_id: str | None = None
    if auditor_passed:
        correlation_id = getattr(emitter, "correlation_id", None) or ""
        logger.info("[TRIBUNAL-AUDITOR] Command verified successfully, creating reputation commitment for correlation_id=%s", correlation_id)
        try:
            commitment = await commit_reputation(
                reputation_data_service=reputation_data_service,
                tribunal_command_id=correlation_id,
                investigation_id=investigation_id,
                hmac_key=auditor_hmac_key,
            )
            commitment_id = commitment.id
            logger.info("[TRIBUNAL-AUDITOR] Reputation commitment created: id=%s merkle_root=%s", commitment.id, commitment.merkle_root[:16])
            await emitter.emit(
                EventType.REPUTATION_COMMITMENT_CREATED,
                ReputationCommitmentCreatedPayload(
                    commitment_id=commitment.id,
                    tribunal_command_id=commitment.tribunal_command_id,
                    investigation_id=commitment.investigation_id,
                    merkle_root=commitment.merkle_root,
                    prev_root=commitment.prev_root,
                    leaves_count=commitment.leaves_count,
                    correlation_id=correlation_id or None,
                ),
                correlation_id=correlation_id or None,
            )
        except Exception as exc:
            logger.error(
                "[TRIBUNAL-AUDITOR] reputation commitment failed (fatal): %s",
                exc,
                exc_info=True,
            )
            await emitter.emit(
                EventType.REPUTATION_COMMITMENT_FAILED,
                ReputationCommitmentFailedPayload(
                    tribunal_command_id=correlation_id,
                    investigation_id=investigation_id,
                    error=str(exc),
                    correlation_id=correlation_id or None,
                ),
                correlation_id=correlation_id or None,
            )
            raise RuntimeError(
                f"Reputation commitment failed for tribunal_command_id={correlation_id}: {exc}. "
                "Verdict cannot proceed without cryptographic binding to reputation scoreboard."
            ) from exc

    return final_command, outcome, auditor_passed, auditor_revision, auditor_reason, commitment_id
