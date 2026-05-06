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

"""
Tribunal Command Generator (Orchestrator)

Implements the five-member heterogeneous AI panel (The Tribunal) as the sole
authority for shell command generation.
"""

import logging
from typing import Any

from app.models.settings import G8eeUserSettings
from app.models.http_context import G8eHttpContext
from app.models.agent import OperatorContext
from app.constants import (
    CommandGenerationOutcome,
    ComponentName,
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
from app.models.whitelist import WhitelistedCommand
from app.models.agents.tribunal import (
    AuditorClusterInfo,
    CandidateCommand,
    CommandGenerationResult,
    TribunalAuditorFailedError,
    TribunalAuditorFailedPayload,
    TribunalConsensusFailedError,
    TribunalConsensusFailedPayload,
    TribunalDisabledError,
    TribunalGenerationFailedError,
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
from app.services.protocols import (
    AIResponseAnalyzerProtocol,
    EventServiceProtocol,
)
from app.utils.ids import generate_tribunal_correlation_id
from app.models.tool_results import CommandRiskAnalysis
from app.services.data.reputation_data_service import ReputationDataService
from app.utils.safety import validate_command_safety
from app.models.model_configs import get_model_config

from app.services.ai.tribunal.emitter import TribunalEmitter
from app.services.ai.tribunal.stages.auditor import _run_audit_stage
from app.services.ai.tribunal.stages.generation import (
    _anonymize_clusters,
    _run_generation_stage,
)
from app.services.ai.tribunal.stages.voting import _run_voting_stage
from app.services.ai.tribunal.stages.warden import _run_warden_stage
from app.services.ai.tribunal.utils import (
    _member_for_pass,
    _resolve_model,
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
        EventType.TRIBUNAL_SESSION_COMPLETED,
        TribunalSessionCompletedPayload(
            request=request,
            final_command=final_command or "",
            outcome=outcome,
            vote_score=vote_score or 0.0,
        ),
    )
    return result

async def generate_command(
    request: str,
    guidelines: str,
    operator_context: OperatorContext | None,
    g8ed_event_service: EventServiceProtocol,
    web_session_id: str,
    user_id: str,
    case_id: str,
    investigation_id: str,
    settings: G8eeUserSettings,
    reputation_data_service: ReputationDataService,
    auditor_hmac_key: str,
    ai_response_analyzer: AIResponseAnalyzerProtocol | None = None,
    investigation_state: Any | None = None,
    investigation_context: str = "",
    whitelisting_enabled: bool = False,
    blacklisting_enabled: bool = False,
    whitelisted_commands: list[WhitelistedCommand] | None = None,
    blacklisted_commands: list[dict[str, str]] | None = None,
) -> CommandGenerationResult:
    """Run the Tribunal pipeline to generate a command from the caller's request."""
    request = request.strip()
    guidelines = guidelines.strip()
    fields = build_tribunal_prompt_fields(
        operator_context,
        request=request,
        guidelines=guidelines,
        default_os=DEFAULT_OS_NAME,
        default_shell=DEFAULT_SHELL,
        default_working_directory=DEFAULT_WORKING_DIRECTORY,
    )

    logger.info(
        "[TRIBUNAL-ENTRY] generate_command called: request_len=%d guidelines_len=%d os=%s shell=%s user=%s hostname=%s arch=%s",
        len(request), len(guidelines), fields["os"], fields["shell"], fields["user_context"],
        operator_context.hostname if operator_context else None,
        operator_context.architecture if operator_context else None,
    )

    command_constraints_message = build_command_constraints_message(
        whitelisting_enabled=whitelisting_enabled,
        blacklisting_enabled=blacklisting_enabled,
        whitelisted_commands=whitelisted_commands,
        blacklisted_commands=blacklisted_commands,
    )

    g8e_context = G8eHttpContext(
        web_session_id=web_session_id,
        user_id=user_id,
        case_id=case_id,
        investigation_id=investigation_id,
        source_component=ComponentName.G8EE,
    )
    correlation_id = generate_tribunal_correlation_id()
    emitter = TribunalEmitter(g8ed_event_service, g8e_context, correlation_id=correlation_id)

    if not settings.llm.llm_command_gen_enabled:
        await emitter.emit(
            EventType.TRIBUNAL_SESSION_DISABLED,
            TribunalSessionDisabledPayload(request=request),
        )
        raise TribunalDisabledError(request=request)

    try:
        generation_model = _resolve_model(settings.llm, tier="lite", request=request)
        auditor_model = _resolve_model(settings.llm, tier="primary", request=request)
    except TribunalModelNotConfiguredError as exc:
        await emitter.emit(
            EventType.TRIBUNAL_SESSION_MODEL_NOT_CONFIGURED,
            TribunalSessionModelNotConfiguredPayload(
                request=request,
                provider=exc.provider,
                error=exc.user_message,
            ),
        )
        raise

    num_passes = max(1, settings.llm.llm_command_gen_passes)
    members = [_member_for_pass(i) for i in range(num_passes)]

    await emitter.emit(
        EventType.TRIBUNAL_SESSION_STARTED,
        TribunalSessionStartedPayload(
            request=request,
            guidelines=guidelines,
            model=generation_model,
            num_passes=num_passes,
            members=members,
            correlation_id=correlation_id,
        ),
    )

    try:
        generation_provider = get_llm_provider(settings.llm, is_lite=True)
    except Exception as exc:
        lite_provider = settings.llm.lite_provider
        provider_name = lite_provider.value if lite_provider else "not_configured"
        await emitter.emit(
            EventType.TRIBUNAL_SESSION_PROVIDER_UNAVAILABLE,
            TribunalSessionProviderUnavailablePayload(
                request=request,
                provider=provider_name,
                error=str(exc),
            ),
        )
        raise TribunalProviderUnavailableError(
            provider=lite_provider,
            error=str(exc),
            request=request,
        ) from exc

    candidates = await _run_generation_stage(
        provider=generation_provider, model=generation_model, request=request, guidelines=guidelines,
        operator_context=operator_context, num_passes=num_passes, emitter=emitter,
        command_constraints_message=command_constraints_message,
        round_num=1,
    )

    # Run Round 1 voting
    vote_winner, vote_score, vote_breakdown, tied_candidates = await _run_voting_stage(
        candidates=candidates, request=request, emitter=emitter, total_members=num_passes,
        is_final=False,
    )

    # Round 2: anonymized peer review if consensus is low
    round_2_candidates = None
    round_2_vote_breakdown = None
    rounds_executed = 1

    if vote_winner is None:

        logger.info("[TRIBUNAL] Consensus strength too low (%.2f < %d), initiating Round 2 peer review",
                    vote_breakdown.consensus_strength, TRIBUNAL_MIN_CONSENSUS)

        await emitter.emit(
            EventType.TRIBUNAL_VOTING_ROUND_STARTED,
            TribunalSessionStartedPayload(
                request=request,
                guidelines=guidelines,
                model=generation_model,
                num_passes=num_passes,
                members=members,
                correlation_id=correlation_id,
            ),
        )

        # Anonymize R1 clusters for peer review context
        r1_clusters, cluster_to_cmd, cluster_to_members = _anonymize_clusters(candidates)

        await emitter.emit(
            EventType.TRIBUNAL_VOTING_ROUND_2_STARTED,
            TribunalSessionStartedPayload(
                request=request,
                guidelines=guidelines,
                model=generation_model,
                num_passes=num_passes,
                members=members,
                correlation_id=correlation_id,
            ),
        )

        # Run Round 2 generation with anonymized cluster context
        round_2_candidates = await _run_generation_stage(
            provider=generation_provider, model=generation_model, request=request, guidelines=guidelines,
            operator_context=operator_context, num_passes=num_passes, emitter=emitter,
            command_constraints_message=command_constraints_message,
            round_num=2,
            r1_clusters=r1_clusters,
        )

        # Run Round 2 voting
        vote_winner, vote_score, vote_breakdown, tied_candidates = await _run_voting_stage(
            candidates=round_2_candidates, request=request, emitter=emitter, total_members=num_passes,
        )

        rounds_executed = 2

        if vote_winner is not None:
            await emitter.emit(
                EventType.TRIBUNAL_VOTING_ROUND_2_CONSENSUS_REACHED,
                TribunalVotingCompletedPayload(
                    vote_winner=vote_winner,
                    vote_score=vote_score,
                    num_candidates=len(round_2_candidates),
                    request=request,
                    vote_breakdown=vote_breakdown,
                ),
            )
            round_2_vote_breakdown = vote_breakdown
        else:
            await emitter.emit(
                EventType.TRIBUNAL_VOTING_ROUND_2_CONSENSUS_FAILED,
                TribunalConsensusFailedPayload(
                    request=request,
                    vote_breakdown=vote_breakdown,
                ),
            )
            round_2_vote_breakdown = vote_breakdown

        await emitter.emit(
            EventType.TRIBUNAL_VOTING_ROUND_COMPLETED,
            TribunalSessionCompletedPayload(
                request=request,
                final_command=vote_winner or "",
                outcome=CommandGenerationOutcome.CONSENSUS if vote_winner else CommandGenerationOutcome.CONSENSUS_FAILED,
                vote_score=vote_score or 0.0,
            ),
        )

    if vote_winner is None:
        await _build_and_emit_result(
            request=request,
            guidelines=guidelines,
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
            whitelisting_enabled=whitelisting_enabled,
            blacklisting_enabled=blacklisting_enabled,
            operator_context=operator_context,
            correlation_id=correlation_id,
            round_2_candidates=None,
            round_2_vote_breakdown=None,
        )
        raise TribunalConsensusFailedError(request=request, vote_breakdown=vote_breakdown)

    try:
        auditor_provider = get_llm_provider(settings.llm, is_assistant=False, is_lite=False)
    except Exception as exc:
        primary_provider = settings.llm.primary_provider
        provider_name = primary_provider.value if primary_provider else "not_configured"
        logger.warning("[TRIBUNAL] Auditor provider unavailable: %s", exc)
        # Auditor failure is non-fatal if consensus was reached, but here we can't even start it
        auditor_provider = None

    warden_risk_analysis = await _run_warden_stage(
        request=request,
        guidelines=guidelines,
        vote_winner=vote_winner,
        operator_context=operator_context,
        emitter=emitter,
        settings=settings,
        investigation_id=investigation_id,
        ai_response_analyzer=ai_response_analyzer,
        investigation_state=investigation_state,
        investigation_context=investigation_context,
    )

    final_command, outcome, auditor_passed, auditor_revision, auditor_reason, reputation_commitment_id = await _run_audit_stage(
        provider=auditor_provider or generation_provider,
        model=auditor_model if auditor_provider else generation_model,
        request=request,
        guidelines=guidelines,
        vote_winner=vote_winner,
        vote_breakdown=vote_breakdown,
        tied_candidates=tied_candidates,
        operator_context=operator_context,
        auditor_enabled=settings.llm.llm_command_gen_auditor,
        emitter=emitter,
        command_constraints_message=command_constraints_message,
        reputation_data_service=reputation_data_service,
        auditor_hmac_key=auditor_hmac_key,
        investigation_id=investigation_id,
        whitelisting_enabled=whitelisting_enabled,
        blacklisting_enabled=blacklisting_enabled,
    )

    return await _build_and_emit_result(
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
        emitter=emitter,
        whitelisting_enabled=whitelisting_enabled,
        blacklisting_enabled=blacklisting_enabled,
        operator_context=operator_context,
        correlation_id=correlation_id,
        reputation_commitment_id=reputation_commitment_id,
        warden_risk_analysis=warden_risk_analysis,
        round_2_candidates=round_2_candidates,
        round_2_vote_breakdown=round_2_vote_breakdown,
    )
