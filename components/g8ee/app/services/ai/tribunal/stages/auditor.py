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
import time

from app.constants import (
    CommandGenerationOutcome,
    EventType,
    AuditorReason,
)
from app.constants.status import CommandErrorType
from app.llm.provider import LLMProvider
from app.errors import OllamaEmptyResponseError
from app.models.agent import OperatorContext
from app.models.reputation import (
    ReputationCommitmentCreatedPayload,
    ReputationCommitmentFailedPayload,
)
from app.models.agents.tribunal import (
    CandidateCommand,
    AuditorClusterInfo,
    VoteBreakdown,
    TribunalAuditorStartedPayload,
    TribunalAuditorCompletedPayload,
    TribunalAuditorFailedError,
    TribunalAuditResult,
)
from app.utils.agent_persona_loader import get_agent_persona
from app.utils.command import normalise_command
from app.utils.safety import validate_command_safety
from app.services.ai.auditor_service import (
    commit_reputation,
    build_auditor_prompt,
    call_auditor_llm,
    parse_auditor_response,
    fail_auditor,
)
from app.services.data.reputation_data_service import ReputationDataService
from app.services.ai.tribunal.emitter import TribunalEmitter

logger = logging.getLogger(__name__)

class TribunalAuditor:
    """Service for handling the Tribunal audit stage and reputation commitment.
    
    Extracts side effects (SSE events, reputation DB writes) to allow mocking 
    as a single unit in orchestrator tests.
    """

    def __init__(
        self,
        emitter: TribunalEmitter,
        reputation_data_service: ReputationDataService,
        auditor_hmac_key: str,
    ):
        self.emitter = emitter
        self.reputation_data_service = reputation_data_service
        self.auditor_hmac_key = auditor_hmac_key

    async def run(
        self,
        provider: LLMProvider,
        model: str,
        request: str,
        guidelines: str,
        vote_winner: str,
        vote_breakdown: VoteBreakdown,
        operator_context: OperatorContext | None,
        auditor_enabled: bool,
        command_constraints_message: str,
        investigation_id: str,
        tied_candidates: list[CandidateCommand] | None = None,
        whitelisting_enabled: bool = False,
        blacklisting_enabled: bool = False,
    ) -> TribunalAuditResult:
        """Execute the audit stage and handle all side effects."""
        if not auditor_enabled:
            return TribunalAuditResult(
                final_command=vote_winner,
                outcome=CommandGenerationOutcome.CONSENSUS,
                passed=True,
                revision=None,
                reason=AuditorReason.OK,
                reputation_commitment_id=None,
            )

        if vote_breakdown.consensus_strength == 1.0:
            mode = "unanimous"
        else:
            mode = "majority"

        # Prepare cluster info and mapping
        clusters: list[AuditorClusterInfo] = []
        cluster_to_cmd: dict[str, str] = {}
        cluster_to_members: dict[str, list[str]] = {}

        target_cmd = vote_winner
        cluster_to_cmd["cluster_a"] = target_cmd
        cluster_to_members["cluster_a"] = vote_breakdown.candidates_by_command[target_cmd]
        clusters.append(AuditorClusterInfo(
            cluster_id="cluster_a",
            command=target_cmd,
            support_count=len(cluster_to_members["cluster_a"])
        ))

        idx = 1
        for cmd, members in vote_breakdown.candidates_by_command.items():
            if cmd == target_cmd:
                continue
            c_id = f"cluster_{chr(ord('a') + idx)}"
            cluster_to_cmd[c_id] = cmd
            cluster_to_members[c_id] = members
            clusters.append(AuditorClusterInfo(
                cluster_id=c_id,
                command=cmd,
                support_count=len(members)
            ))
            idx += 1

        correlation_id = getattr(self.emitter, "correlation_id", None)
        await self.emitter.emit(
            EventType.TRIBUNAL_VOTING_AUDIT_STARTED,
            TribunalAuditorStartedPayload(candidate_command=target_cmd),
            correlation_id=correlation_id,
        )

        auditor_start_time = time.time()
        auditor_persona = get_agent_persona("auditor")
        prompt = build_auditor_prompt(
            request=request,
            guidelines=guidelines,
            mode=mode,
            target_cmd=target_cmd,
            clusters=clusters,
            operator_context=operator_context,
            command_constraints_message=command_constraints_message,
        )

        max_attempts = 2
        auditor_passed = False
        final_command = target_cmd
        auditor_revision = None
        auditor_reason = AuditorReason.AUDITOR_ERROR

        for attempt in range(max_attempts):
            try:
                raw_text = await call_auditor_llm(provider, model, prompt, auditor_persona, attempt)
                status, revised_raw, swap_to_cluster_id = parse_auditor_response(
                    raw_text, mode, list(cluster_to_cmd.keys())
                )

                if status == "ok":
                    total_duration_ms = (time.time() - auditor_start_time) * 1000
                    logger.info("[TRIBUNAL-AUDITOR] Completed with status=ok total_duration_ms=%.2f", total_duration_ms)
                    await self.emitter.emit(
                        EventType.TRIBUNAL_VOTING_AUDIT_COMPLETED,
                        TribunalAuditorCompletedPayload(passed=True, reason=AuditorReason.OK),
                        correlation_id=correlation_id,
                    )
                    auditor_passed, final_command, auditor_revision, auditor_reason = True, target_cmd, None, AuditorReason.OK
                    break

                if status == "swap" and swap_to_cluster_id:
                    final_cmd = cluster_to_cmd[swap_to_cluster_id]
                    swap_to_member = cluster_to_members[swap_to_cluster_id][0]
                    
                    safety_result = validate_command_safety(final_cmd, whitelisting_enabled, blacklisting_enabled, operator_context)
                    if not safety_result.is_safe:
                        reason = AuditorReason.WHITELIST_VIOLATION if safety_result.error_type == CommandErrorType.WHITELIST_VIOLATION else AuditorReason.NO_VALID_REVISION
                        await fail_auditor(self.emitter, request, reason, f"Swap target technical safety failure: {safety_result.error_message}", target_cmd)

                    total_duration_ms = (time.time() - auditor_start_time) * 1000
                    logger.info("[TRIBUNAL-AUDITOR] Completed with status=swap total_duration_ms=%.2f", total_duration_ms)
                    await self.emitter.emit(
                        EventType.TRIBUNAL_VOTING_AUDIT_COMPLETED,
                        TribunalAuditorCompletedPayload(
                            passed=True,
                            reason=AuditorReason.SWAPPED_TO_DISSENTER,
                            swap_to_cluster=swap_to_cluster_id,
                            swap_to_member=swap_to_member
                        ),
                        correlation_id=correlation_id,
                    )
                    auditor_passed, final_command, auditor_revision, auditor_reason = True, final_cmd, None, AuditorReason.SWAPPED_TO_DISSENTER
                    break

                if status == "revised" and revised_raw:
                    revised = normalise_command(revised_raw)
                    if not revised:
                        await fail_auditor(self.emitter, request, AuditorReason.NO_VALID_REVISION, "Empty revision", target_cmd)

                    safety_result = validate_command_safety(revised, whitelisting_enabled, blacklisting_enabled, operator_context)
                    if not safety_result.is_safe:
                        reason = AuditorReason.WHITELIST_VIOLATION if safety_result.error_type == CommandErrorType.WHITELIST_VIOLATION else AuditorReason.NO_VALID_REVISION
                        await fail_auditor(self.emitter, request, reason, f"Revision technical safety failure: {safety_result.error_message}", target_cmd)

                    reason = AuditorReason.REVISED_FROM_DISSENT if mode in ("majority", "tied") else AuditorReason.REVISED
                    total_duration_ms = (time.time() - auditor_start_time) * 1000
                    logger.info("[TRIBUNAL-AUDITOR] Completed with status=revised total_duration_ms=%.2f", total_duration_ms)
                    await self.emitter.emit(
                        EventType.TRIBUNAL_VOTING_AUDIT_COMPLETED,
                        TribunalAuditorCompletedPayload(passed=False, revision=revised, reason=reason),
                        correlation_id=correlation_id,
                    )
                    auditor_passed, final_command, auditor_revision, auditor_reason = False, revised, revised, reason
                    break

            except (ValueError, OllamaEmptyResponseError) as exc:
                logger.warning("[TRIBUNAL-AUDITOR] Attempt %d failed: %s", attempt + 1, exc)
                if attempt == max_attempts - 1:
                    if isinstance(exc, OllamaEmptyResponseError):
                        await fail_auditor(self.emitter, request, AuditorReason.EMPTY_RESPONSE, str(exc), target_cmd)
                    else:
                        await fail_auditor(self.emitter, request, AuditorReason.NO_VALID_REVISION, f"Failed to parse auditor response: {exc!s}", target_cmd)
                continue
            except TribunalAuditorFailedError:
                raise
            except Exception as exc:
                logger.error("[TRIBUNAL-AUDITOR] Unexpected error: %s", exc, exc_info=True)
                await fail_auditor(self.emitter, request, AuditorReason.AUDITOR_ERROR, str(exc), target_cmd)

        outcome = CommandGenerationOutcome.VERIFIED if auditor_passed else CommandGenerationOutcome.VERIFICATION_FAILED
        if not auditor_passed and auditor_reason == AuditorReason.REVISED:
            outcome = CommandGenerationOutcome.VERIFICATION_FAILED

        commitment_id: str | None = None
        if auditor_passed:
            correlation_id = getattr(self.emitter, "correlation_id", None) or "test-correlation-id"
            logger.info("[TRIBUNAL-AUDITOR] Command verified successfully, creating reputation commitment for correlation_id=%s", correlation_id)
            try:
                commitment = await commit_reputation(
                    reputation_data_service=self.reputation_data_service,
                    tribunal_command_id=correlation_id,
                    investigation_id=investigation_id,
                    hmac_key=self.auditor_hmac_key,
                )
                commitment_id = commitment.id
                logger.info("[TRIBUNAL-AUDITOR] Reputation commitment created: id=%s merkle_root=%s", commitment.id, commitment.merkle_root[:16])
                await self.emitter.emit(
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
                await self.emitter.emit(
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

        return TribunalAuditResult(
            final_command=final_command,
            outcome=outcome,
            passed=auditor_passed,
            revision=auditor_revision,
            reason=auditor_reason,
            reputation_commitment_id=commitment_id,
        )
