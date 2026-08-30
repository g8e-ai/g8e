# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
Tribunal Command Generator (Orchestrator)

Implements the five-member heterogeneous AI panel (The Tribunal) as the sole
authority for shell command generation.
"""

import logging

from app.errors import ConfigurationError
from app.models.http_context import RequestContext
from app.models.agent import OperatorContext
from app.constants import (
    CommandGenerationOutcome,
    DEFAULT_OS_NAME,
    DEFAULT_SHELL,
    DEFAULT_WORKING_DIRECTORY,
    EventType,
    AuditorReason,
)
from app.services.ai.voter import TRIBUNAL_MIN_CONSENSUS
from app.llm.prompts import (
    build_command_constraints_message,
    build_tribunal_prompt_fields,
)
from app.llm.factory import get_llm_provider
from app.models.agents.tribunal import (
    CandidateCommand,
    CommandGenerationResult,
    TribunalConsensusFailedError,
    TribunalConsensusFailedPayload,
    TribunalDisabledError,
    TribunalModelNotConfiguredError,
    TribunalProviderUnavailableError,
    TribunalSessionCompletedPayload,
    TribunalSessionDisabledPayload,
    TribunalSessionModelNotConfiguredPayload,
    TribunalSessionProviderUnavailablePayload,
    TribunalSessionStartedPayload,
    TribunalVotingCompletedPayload,
    VoteBreakdown,
)
from app.models.tribunal_commands import TribunalGenerationRequest
from app.utils.ids import generate_tribunal_correlation_id
from app.models.tool_results import CommandRiskAnalysis
from app.utils.safety import validate_command_safety

from app.services.ai.tribunal.emitter import TribunalEmitter
from app.services.ai.tribunal.stages.auditor import TribunalAuditor
from app.services.ai.tribunal.stages.generation import (
    _anonymize_clusters,
    _run_generation_stage,
)
from app.services.ai.tribunal.stages.voting import _run_voting_stage
from app.services.ai.tribunal.stages.warden import _run_warden_stage
from app.services.ai.tribunal.utils import (
    member_for_pass,
    resolve_model,
)

logger = logging.getLogger(__name__)


async def _build_and_emit_result(
    request: str,
    guidelines: str,
    final_command: str | None,
    outcome: CommandGenerationOutcome,
    candidates: list[CandidateCommand],
    vote_winner: str | None,
    vote_score: float | None,
    vote_breakdown: VoteBreakdown | None,
    auditor_passed: bool | None,
    auditor_revision: str | None,
    auditor_reason: AuditorReason | None,
    emitter: TribunalEmitter,
    whitelisting_enabled: bool = False,
    blacklisting_enabled: bool = False,
    operator_context: OperatorContext | None = None,
    correlation_id: str | None = None,
    reputation_commitment_id: str | None = None,
    warden_risk_analysis: CommandRiskAnalysis | None = None,
    round_2_candidates: list[CandidateCommand] | None = None,
    round_2_vote_breakdown: VoteBreakdown | None = None,
) -> CommandGenerationResult:
    """Stage 4: assemble the result model and emit the session-completed event."""
    is_safe = True
    safety_error = None
    if final_command:
        safety_result = validate_command_safety(
            final_command,
            whitelisting_enabled=whitelisting_enabled,
            blacklisting_enabled=blacklisting_enabled,
            operator_context=operator_context,
        )
        is_safe = safety_result.is_safe
        safety_error = safety_result.error_message

    if not is_safe:
        logger.error("[TRIBUNAL] Final command safety validation failed: %s", safety_error)
        outcome = CommandGenerationOutcome.CONSENSUS_FAILED
        final_command = None

    result = CommandGenerationResult(
        request=request,
        guidelines=guidelines,
        final_command=final_command,
        outcome=outcome,
        candidates=candidates,
        vote_winner=vote_winner,
        vote_score=vote_score,
        vote_breakdown=vote_breakdown,
        auditor_passed=auditor_passed,
        auditor_revision=auditor_revision,
        auditor_reason=auditor_reason,
        warden_risk_analysis=warden_risk_analysis,
        correlation_id=correlation_id,
        reputation_commitment_id=reputation_commitment_id,
        round_2_candidates=round_2_candidates,
        round_2_vote_breakdown=round_2_vote_breakdown,
    )

    await emitter.emit(
        EventType.AI_CONSENSUS_SESSION_COMPLETED,
        TribunalSessionCompletedPayload(
            request=request,
            final_command=final_command or "",
            outcome=outcome,
            vote_score=vote_score or 0.0,
            model_calls=[warden_risk_analysis.model_call]
            if warden_risk_analysis and warden_risk_analysis.model_call
            else [],
        ),
    )
    return result


async def generate_command(request: TribunalGenerationRequest) -> CommandGenerationResult:
    """Run the Tribunal pipeline to generate a command from the caller's request."""
    request.request = request.request.strip()
    request.guidelines = request.guidelines.strip()
    fields = build_tribunal_prompt_fields(
        request.operator_context,
        request=request.request,
        guidelines=request.guidelines,
        default_os=DEFAULT_OS_NAME,
        default_shell=DEFAULT_SHELL,
        default_working_directory=DEFAULT_WORKING_DIRECTORY,
    )

    logger.info(
        "[TRIBUNAL-ENTRY] generate_command called: request_len=%d guidelines_len=%d os=%s shell=%s user=%s hostname=%s arch=%s",
        len(request.request),
        len(request.guidelines),
        fields["os"],
        fields["shell"],
        fields["user_context"],
        request.operator_context.hostname if request.operator_context else None,
        request.operator_context.architecture if request.operator_context else None,
    )

    command_constraints_message = build_command_constraints_message(
        whitelisting_enabled=request.whitelisting_enabled,
        blacklisting_enabled=request.blacklisting_enabled,
        whitelisted_commands=request.whitelisted_commands,
        blacklisted_commands=request.blacklisted_commands,
    )

    correlation_id = generate_tribunal_correlation_id()
    emitter = TribunalEmitter(
        request.event_service, request.g8e_context, correlation_id=correlation_id
    )

    if request.settings.llm is None:
        raise ConfigurationError("LLM settings are missing")

    if not request.settings.llm.llm_command_gen_enabled:
        await emitter.emit(
            EventType.AI_CONSENSUS_SESSION_DISABLED,
            TribunalSessionDisabledPayload(request=request.request),
        )
        raise TribunalDisabledError(request=request.request)

    try:
        generation_model = resolve_model(request.settings.llm, tier="lite", request=request.request)
        auditor_model = resolve_model(request.settings.llm, tier="primary", request=request.request)
    except TribunalModelNotConfiguredError as exc:
        await emitter.emit(
            EventType.AI_CONSENSUS_SESSION_MODEL_NOT_CONFIGURED,
            TribunalSessionModelNotConfiguredPayload(
                request=request.request,
                provider=exc.provider,
                error=exc.user_message,
            ),
        )
        raise

    num_passes = max(1, request.settings.llm.llm_command_gen_passes)
    members = [member_for_pass(i) for i in range(num_passes)]

    await emitter.emit(
        EventType.AI_CONSENSUS_SESSION_STARTED,
        TribunalSessionStartedPayload(
            request=request.request,
            guidelines=request.guidelines,
            model=generation_model,
            num_passes=num_passes,
            members=members,
            correlation_id=correlation_id,
        ),
    )

    try:
        generation_provider = get_llm_provider(request.settings.llm, is_lite=True)
    except Exception as exc:
        lite_provider = request.settings.llm.lite_provider
        provider_name = lite_provider.value if lite_provider else "not_configured"
        await emitter.emit(
            EventType.AI_CONSENSUS_SESSION_PROVIDER_UNAVAILABLE,
            TribunalSessionProviderUnavailablePayload(
                request=request.request,
                provider=provider_name,
                error=str(exc),
            ),
        )
        raise TribunalProviderUnavailableError(
            provider=lite_provider,
            error=str(exc),
            request=request.request,
        ) from exc

    if generation_provider is None:
        lite_provider = request.settings.llm.lite_provider
        provider_name = lite_provider.value if lite_provider else "not_configured"
        raise ConfigurationError(f"Failed to initialize generation provider for {provider_name}")

    candidates = await _run_generation_stage(
        provider=generation_provider,
        model=generation_model,
        request=request.request,
        guidelines=request.guidelines,
        operator_context=request.operator_context,
        num_passes=num_passes,
        emitter=emitter,
        command_constraints_message=command_constraints_message,
        round_num=1,
    )

    # Run Round 1 voting
    vote_winner, vote_score, vote_breakdown, tied_candidates = await _run_voting_stage(
        candidates=candidates,
        request=request.request,
        emitter=emitter,
        total_members=num_passes,
        is_final=False,
    )

    # Round 2: anonymized peer review if consensus is low
    round_2_candidates = None
    round_2_vote_breakdown = None

    if vote_winner is None:
        logger.info(
            "[TRIBUNAL] Consensus strength too low (%.2f < %d), initiating Round 2 peer review",
            vote_breakdown.consensus_strength,
            TRIBUNAL_MIN_CONSENSUS,
        )

        await emitter.emit(
            EventType.AI_CONSENSUS_VOTING_ROUND_STARTED,
            TribunalSessionStartedPayload(
                request=request.request,
                guidelines=request.guidelines,
                model=generation_model,
                num_passes=num_passes,
                members=members,
                correlation_id=correlation_id,
            ),
        )

        # Anonymize R1 clusters for peer review context
        r1_clusters, _, _ = _anonymize_clusters(candidates)

        await emitter.emit(
            EventType.AI_CONSENSUS_VOTING_ROUND_2_STARTED,
            TribunalSessionStartedPayload(
                request=request.request,
                guidelines=request.guidelines,
                model=generation_model,
                num_passes=num_passes,
                members=members,
                correlation_id=correlation_id,
            ),
        )

        # Run Round 2 generation with anonymized cluster context
        round_2_candidates = await _run_generation_stage(
            provider=generation_provider,
            model=generation_model,
            request=request.request,
            guidelines=request.guidelines,
            operator_context=request.operator_context,
            num_passes=num_passes,
            emitter=emitter,
            command_constraints_message=command_constraints_message,
            round_num=2,
            r1_clusters=r1_clusters,
        )

        # Run Round 2 voting
        vote_winner, vote_score, vote_breakdown, tied_candidates = await _run_voting_stage(
            candidates=round_2_candidates,
            request=request.request,
            emitter=emitter,
            total_members=num_passes,
        )

        if vote_winner is not None:
            await emitter.emit(
                EventType.AI_CONSENSUS_VOTING_ROUND_2_CONSENSUS_REACHED,
                TribunalVotingCompletedPayload(
                    vote_winner=vote_winner,
                    vote_score=vote_score,
                    num_candidates=len(round_2_candidates),
                    request=request.request,
                    vote_breakdown=vote_breakdown,
                ),
            )
            round_2_vote_breakdown = vote_breakdown
        else:
            await emitter.emit(
                EventType.AI_CONSENSUS_VOTING_ROUND_2_CONSENSUS_FAILED,
                TribunalConsensusFailedPayload(
                    request=request.request,
                    vote_breakdown=vote_breakdown,
                ),
            )
            round_2_vote_breakdown = vote_breakdown

        await emitter.emit(
            EventType.AI_CONSENSUS_VOTING_ROUND_COMPLETED,
            TribunalSessionCompletedPayload(
                request=request.request,
                final_command=vote_winner or "",
                outcome=CommandGenerationOutcome.CONSENSUS
                if vote_winner
                else CommandGenerationOutcome.CONSENSUS_FAILED,
                vote_score=vote_score or 0.0,
            ),
        )

    if vote_winner is None:
        await _build_and_emit_result(
            request=request.request,
            guidelines=request.guidelines,
            final_command=None,
            outcome=CommandGenerationOutcome.CONSENSUS_FAILED,
            candidates=candidates,
            vote_winner=None,
            vote_score=0.0,
            vote_breakdown=vote_breakdown,
            auditor_passed=None,
            auditor_revision=None,
            auditor_reason=None,
            emitter=emitter,
            whitelisting_enabled=request.whitelisting_enabled,
            blacklisting_enabled=request.blacklisting_enabled,
            operator_context=request.operator_context,
            correlation_id=correlation_id,
            round_2_candidates=None,
            round_2_vote_breakdown=None,
        )
        raise TribunalConsensusFailedError(request=request.request, vote_breakdown=vote_breakdown)

    try:
        auditor_provider = get_llm_provider(request.settings.llm, is_assistant=False, is_lite=False)
    except Exception as exc:
        primary_provider = request.settings.llm.primary_provider
        provider_name = primary_provider.value if primary_provider else "not_configured"
        logger.warning("[TRIBUNAL] Auditor provider unavailable: %s", exc)
        # Auditor failure is non-fatal if consensus was reached, but here we can't even start it
        auditor_provider = None

    investigation_id = request.g8e_context.investigation_id
    warden_risk_analysis = await _run_warden_stage(
        request=request.request,
        guidelines=request.guidelines,
        vote_winner=vote_winner,
        operator_context=request.operator_context,
        emitter=emitter,
        settings=request.settings,
        investigation_id=investigation_id,
        ai_response_analyzer=request.ai_response_analyzer,
        investigation_state=request.investigation_state,
        investigation_context=request.investigation_context,
    )

    auditor = TribunalAuditor(
        emitter=emitter,
        reputation_data_service=request.reputation_data_service,
        auditor_hmac_key=request.auditor_hmac_key,
    )
    audit_result = await auditor.run(
        provider=auditor_provider or generation_provider,
        model=auditor_model if auditor_provider else generation_model,
        request=request.request,
        guidelines=request.guidelines,
        vote_winner=vote_winner,
        vote_breakdown=vote_breakdown,
        tied_candidates=tied_candidates,
        operator_context=request.operator_context,
        auditor_enabled=request.settings.llm.llm_command_gen_auditor,
        command_constraints_message=command_constraints_message,
        investigation_id=investigation_id,
        context=RequestContext.from_app_context(request.g8e_context),
        whitelisting_enabled=request.whitelisting_enabled,
        blacklisting_enabled=request.blacklisting_enabled,
    )

    return await _build_and_emit_result(
        request=request.request,
        guidelines=request.guidelines,
        final_command=audit_result.final_command,
        outcome=audit_result.outcome,
        candidates=candidates,
        vote_winner=vote_winner,
        vote_score=vote_score,
        vote_breakdown=vote_breakdown,
        auditor_passed=audit_result.passed,
        auditor_revision=audit_result.revision,
        auditor_reason=audit_result.reason,
        emitter=emitter,
        whitelisting_enabled=request.whitelisting_enabled,
        blacklisting_enabled=request.blacklisting_enabled,
        operator_context=request.operator_context,
        correlation_id=correlation_id,
        reputation_commitment_id=audit_result.reputation_commitment_id,
        warden_risk_analysis=warden_risk_analysis,
        round_2_candidates=round_2_candidates,
        round_2_vote_breakdown=round_2_vote_breakdown,
    )
