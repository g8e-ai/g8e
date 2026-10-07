# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import logging
import time
from typing import Literal

from app.constants import (
    AuditorReason,
    CommandGenerationOutcome,
    ConsensusAuditMode,
    ConsensusAuditStatus,
    EventType,
)
from app.constants.generated_status import CommandErrorType
from app.errors import ContextWindowExceededError, OllamaEmptyResponseError
from app.llm.provider import LLMProvider
from app.models.agent import OperatorContext
from app.models.agents.tribunal import (
    AuditorClusterInfo,
    TribunalAuditorCompletedPayload,
    TribunalAuditorFailedError,
    TribunalAuditorStartedPayload,
    TribunalAuditResult,
    VoteBreakdown,
)
from app.models.http_context import RequestContext
from app.models.model_telemetry import ModelCallTelemetry
from app.models.reputation import (
    ReputationCommitmentCreatedPayload,
    ReputationCommitmentFailedPayload,
)
from app.services.ai.auditor_service import (
    build_auditor_prompt,
    call_auditor_llm,
    commit_reputation,
    fail_auditor,
    parse_auditor_response,
)
from app.services.ai.tribunal.emitter import TribunalEmitter
from app.services.data.reputation_data_service import ReputationDataService
from app.utils.agent_persona_loader import get_agent_persona
from app.utils.command import normalise_command
from app.utils.validation.safety import validate_command_safety

logger = logging.getLogger(__name__)


def _prepare_audit_clusters(
    vote_winner: str, vote_breakdown: VoteBreakdown
) -> tuple[
    ConsensusAuditMode,
    list[AuditorClusterInfo],
    dict[str, str],
    dict[str, list[str]],
]:
    mode = (
        ConsensusAuditMode.UNANIMOUS
        if vote_breakdown.consensus_strength == 1.0
        else ConsensusAuditMode.MAJORITY
    )
    clusters: list[AuditorClusterInfo] = []
    cluster_to_cmd: dict[str, str] = {}
    cluster_to_members: dict[str, list[str]] = {}

    target_cmd = vote_winner
    cluster_to_cmd["cluster_a"] = target_cmd
    cluster_to_members["cluster_a"] = vote_breakdown.candidates_by_command[target_cmd]
    clusters.append(
        AuditorClusterInfo(
            cluster_id="cluster_a",
            command=target_cmd,
            support_count=len(cluster_to_members["cluster_a"]),
        )
    )

    idx = 1
    for cmd, members in vote_breakdown.candidates_by_command.items():
        if cmd == target_cmd:
            continue
        c_id = f"cluster_{chr(ord('a') + idx)}"
        cluster_to_cmd[c_id] = cmd
        cluster_to_members[c_id] = members
        clusters.append(
            AuditorClusterInfo(cluster_id=c_id, command=cmd, support_count=len(members))
        )
        idx += 1
    return mode, clusters, cluster_to_cmd, cluster_to_members


class TribunalAuditor:
    """Service for handling the Tribunal audit stage and reputation commitment.

    Extracts side effects (SSE events, reputation DB writes) to allow mocking
    as a single unit in orchestrator tests.
    """

    def __init__(
        self,
        emitter: TribunalEmitter,
        reputation_data_service: ReputationDataService,
    ):
        self.emitter = emitter
        self.reputation_data_service = reputation_data_service

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
        context: RequestContext,
        whitelisting_enabled: bool = False,
        blacklisting_enabled: bool = False,
        model_role: Literal["primary", "assistant", "lite"] = "primary",
    ) -> TribunalAuditResult:
        """Execute the audit stage and handle all side effects.

        ``model_role`` is the tier ``provider`` and ``model`` were resolved from.
        """
        if not auditor_enabled:
            return TribunalAuditResult(
                final_command=vote_winner,
                outcome=CommandGenerationOutcome.CONSENSUS,
                passed=True,
                revision=None,
                reason=AuditorReason.OK,
                reputation_commitment_id=None,
            )

        mode, clusters, cluster_to_cmd, cluster_to_members = _prepare_audit_clusters(
            vote_winner, vote_breakdown
        )
        target_cmd = vote_winner

        correlation_id = getattr(self.emitter, "correlation_id", None)
        await self.emitter.emit(
            EventType.AI_CONSENSUS_VOTING_AUDIT_STARTED,
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
        model_calls: list[ModelCallTelemetry] = []

        for attempt in range(max_attempts):
            try:
                call_result = await call_auditor_llm(
                    provider,
                    model,
                    prompt,
                    auditor_persona,
                    attempt,
                    g8e_context=self.emitter.g8e_context,
                    model_role=model_role,
                )
                model_calls.append(call_result.telemetry)
                if call_result.error:
                    raise call_result.error
                status, revised_raw, swap_to_cluster_id = parse_auditor_response(
                    call_result.raw_text, mode, list(cluster_to_cmd.keys())
                )

                decision = await self._apply_audit_decision(
                    status,
                    revised_raw,
                    swap_to_cluster_id,
                    mode,
                    target_cmd,
                    cluster_to_cmd,
                    cluster_to_members,
                    whitelisting_enabled,
                    blacklisting_enabled,
                    operator_context,
                    request,
                    model_calls,
                    auditor_start_time,
                    correlation_id,
                )
                if decision:
                    auditor_passed, final_command, auditor_revision, auditor_reason = decision
                    break

            except ContextWindowExceededError as exc:
                logger.exception(
                    "[TRIBUNAL-AUDITOR] Prompt exceeded the model context window, not retrying: %s",
                    exc,
                )
                await fail_auditor(
                    self.emitter,
                    request,
                    AuditorReason.CONTEXT_OVERFLOW,
                    f"The conversation exceeded the model's context window: {exc!s}",
                    target_cmd,
                    model_calls=model_calls,
                )
            except (ValueError, OllamaEmptyResponseError) as exc:
                logger.warning("[TRIBUNAL-AUDITOR] Attempt %d failed: %s", attempt + 1, exc)
                if attempt == max_attempts - 1:
                    if isinstance(exc, OllamaEmptyResponseError):
                        await fail_auditor(
                            self.emitter,
                            request,
                            AuditorReason.EMPTY_RESPONSE,
                            str(exc),
                            target_cmd,
                            model_calls=model_calls,
                        )
                    else:
                        await fail_auditor(
                            self.emitter,
                            request,
                            AuditorReason.NO_VALID_REVISION,
                            f"Failed to parse auditor response: {exc!s}",
                            target_cmd,
                            model_calls=model_calls,
                        )
                continue
            except TribunalAuditorFailedError:
                raise
            except Exception as exc:
                logger.error("[TRIBUNAL-AUDITOR] Unexpected error: %s", exc)
                await fail_auditor(
                    self.emitter,
                    request,
                    AuditorReason.AUDITOR_ERROR,
                    str(exc),
                    target_cmd,
                    model_calls=model_calls,
                )

        outcome = (
            CommandGenerationOutcome.VERIFIED
            if auditor_passed
            else CommandGenerationOutcome.VERIFICATION_FAILED
        )
        if not auditor_passed and auditor_reason == AuditorReason.REVISED:
            outcome = CommandGenerationOutcome.VERIFICATION_FAILED

        commitment_id = await self._commit_reputation_if_verified(
            auditor_passed, investigation_id, context
        )

        return TribunalAuditResult(
            final_command=final_command,
            outcome=outcome,
            passed=auditor_passed,
            revision=auditor_revision,
            reason=auditor_reason,
            reputation_commitment_id=commitment_id,
        )

    async def _apply_audit_decision(
        self,
        status: ConsensusAuditStatus,
        revised_raw: str | None,
        swap_to_cluster_id: str | None,
        mode: ConsensusAuditMode,
        target_cmd: str,
        cluster_to_cmd: dict[str, str],
        cluster_to_members: dict[str, list[str]],
        whitelisting_enabled: bool,
        blacklisting_enabled: bool,
        operator_context: OperatorContext | None,
        request: str,
        model_calls: list[ModelCallTelemetry],
        auditor_start_time: float,
        correlation_id: str | None,
    ) -> tuple[bool, str, str | None, AuditorReason] | None:
        if status == ConsensusAuditStatus.OK:
            total_duration_ms = (time.time() - auditor_start_time) * 1000
            logger.info(
                "[TRIBUNAL-AUDITOR] Completed with status=ok total_duration_ms=%.2f",
                total_duration_ms,
            )
            await self.emitter.emit(
                EventType.AI_CONSENSUS_VOTING_AUDIT_COMPLETED,
                TribunalAuditorCompletedPayload(
                    passed=True,
                    reason=AuditorReason.OK,
                    model_calls=model_calls,
                ),
                correlation_id=correlation_id,
            )
            return True, target_cmd, None, AuditorReason.OK

        if status == ConsensusAuditStatus.SWAP and swap_to_cluster_id:
            final_cmd = cluster_to_cmd[swap_to_cluster_id]
            swap_to_member = cluster_to_members[swap_to_cluster_id][0]

            safety_result = validate_command_safety(
                final_cmd, whitelisting_enabled, blacklisting_enabled, operator_context
            )
            if not safety_result.is_safe:
                reason = (
                    AuditorReason.WHITELIST_VIOLATION
                    if safety_result.error_type == CommandErrorType.WHITELIST_VIOLATION
                    else AuditorReason.NO_VALID_REVISION
                )
                await fail_auditor(
                    self.emitter,
                    request,
                    reason,
                    f"Swap target technical safety failure: {safety_result.error_message}",
                    target_cmd,
                    model_calls=model_calls,
                )

            total_duration_ms = (time.time() - auditor_start_time) * 1000
            logger.info(
                "[TRIBUNAL-AUDITOR] Completed with status=swap total_duration_ms=%.2f",
                total_duration_ms,
            )
            await self.emitter.emit(
                EventType.AI_CONSENSUS_VOTING_AUDIT_COMPLETED,
                TribunalAuditorCompletedPayload(
                    passed=True,
                    reason=AuditorReason.SWAPPED_TO_DISSENTER,
                    swap_to_cluster=swap_to_cluster_id,
                    swap_to_member=swap_to_member,
                    model_calls=model_calls,
                ),
                correlation_id=correlation_id,
            )
            return True, final_cmd, None, AuditorReason.SWAPPED_TO_DISSENTER

        if status == ConsensusAuditStatus.REVISED and revised_raw:
            revised = normalise_command(revised_raw)
            if not revised:
                await fail_auditor(
                    self.emitter,
                    request,
                    AuditorReason.NO_VALID_REVISION,
                    "Empty revision",
                    target_cmd,
                    model_calls=model_calls,
                )

            safety_result = validate_command_safety(
                revised, whitelisting_enabled, blacklisting_enabled, operator_context
            )
            if not safety_result.is_safe:
                reason = (
                    AuditorReason.WHITELIST_VIOLATION
                    if safety_result.error_type == CommandErrorType.WHITELIST_VIOLATION
                    else AuditorReason.NO_VALID_REVISION
                )
                await fail_auditor(
                    self.emitter,
                    request,
                    reason,
                    f"Revision technical safety failure: {safety_result.error_message}",
                    target_cmd,
                    model_calls=model_calls,
                )

            reason = (
                AuditorReason.REVISED_FROM_DISSENT
                if mode in (ConsensusAuditMode.MAJORITY, ConsensusAuditMode.TIED)
                else AuditorReason.REVISED
            )
            total_duration_ms = (time.time() - auditor_start_time) * 1000
            logger.info(
                "[TRIBUNAL-AUDITOR] Completed with status=revised total_duration_ms=%.2f",
                total_duration_ms,
            )
            await self.emitter.emit(
                EventType.AI_CONSENSUS_VOTING_AUDIT_COMPLETED,
                TribunalAuditorCompletedPayload(
                    passed=False,
                    revision=revised,
                    reason=reason,
                    model_calls=model_calls,
                ),
                correlation_id=correlation_id,
            )
            return False, revised, revised, reason
        return None


    async def _commit_reputation_if_verified(
        self, auditor_passed: bool, investigation_id: str, context: RequestContext
    ) -> str | None:
        commitment_id: str | None = None
        if auditor_passed:
            correlation_id = getattr(self.emitter, "correlation_id", None) or "test-correlation-id"
            logger.info(
                "[TRIBUNAL-AUDITOR] Command verified successfully, creating reputation commitment for correlation_id=%s",
                correlation_id,
            )
            try:
                commitment = await commit_reputation(
                    reputation_data_service=self.reputation_data_service,
                    tribunal_command_id=correlation_id,
                    investigation_id=investigation_id,
                    context=context,
                )
                commitment_id = commitment.id
                logger.info(
                    "[TRIBUNAL-AUDITOR] Reputation commitment created: id=%s merkle_root=%s",
                    commitment.id,
                    commitment.merkle_root[:16],
                )
                await self.emitter.emit(
                    EventType.OPERATOR_REPUTATION_COMMITMENT_CREATED,
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
                logger.exception(
                    "[TRIBUNAL-AUDITOR] reputation commitment failed (fatal): %s",
                    exc,
                )
                await self.emitter.emit(
                    EventType.OPERATOR_REPUTATION_COMMITMENT_FAILED,
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

        return commitment_id
