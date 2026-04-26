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

import asyncio
import logging
from typing import Any

from app.errors import OllamaEmptyResponseError
from app.models.settings import LLMSettings, G8eeUserSettings
from app.models.base import G8eBaseModel
from app.models.http_context import G8eHttpContext
from app.models.agent import OperatorContext
from app.constants import (
    CommandGenerationOutcome,
    ComponentName,
    DEFAULT_OS_NAME,
    DEFAULT_SHELL,
    DEFAULT_WORKING_DIRECTORY,
    TribunalMember,
    TieBreakReason,
    EventType,
    AuditorReason,
)
from app.llm.prompts import (
    build_command_constraints_message,
    build_tribunal_generator_prompt,
    build_tribunal_prompt_fields,
)
from app.llm.factory import get_llm_provider
from app.llm.llm_types import Content, Part, Role, LiteLLMSettings, ResponseFormat
from app.llm.provider import LLMProvider
from app.models.reputation import (
    ReputationCommitmentCreatedPayload,
    ReputationCommitmentFailedPayload,
)
from app.models.agents.tribunal import (
    CandidateCommand,
    CommandGenerationResult,
    TribunalDisabledError,
    TribunalSystemError,
    TribunalProviderUnavailableError,
    TribunalGenerationFailedError,
    TribunalModelNotConfiguredError,
    TribunalConsensusFailedError,
    TribunalPassCompletedPayload,
    TribunalSessionStartedPayload,
    TribunalSessionDisabledPayload,
    TribunalSessionModelNotConfiguredPayload,
    TribunalSessionProviderUnavailablePayload,
    TribunalSessionSystemErrorPayload,
    TribunalSessionGenerationFailedPayload,
    TribunalVotingCompletedPayload,
    TribunalConsensusFailedPayload,
    TribunalDissentRecordedPayload,
    TribunalSessionCompletedPayload,
    VoteBreakdown,
)
from app.models.events import SessionEvent
from app.models.model_configs import get_model_config
from app.services.infra.g8ed_event_service import EventService
from app.utils.agent_persona_loader import get_agent_persona
from app.utils.json_utils import extract_json_from_text
from app.utils.ids import generate_tribunal_correlation_id

from app.utils.command import normalise_command
from app.services.ai.voter import (
    TRIBUNAL_MIN_CONSENSUS,
    weighted_vote,
)
from app.services.ai.auditor_service import (
    commit_reputation,
    run_auditor,
    validate_command_safety,
)
from app.services.data.reputation_data_service import ReputationDataService

logger = logging.getLogger(__name__)

_MAX_TOKENS_GENERATION = 4096

_TERMINAL_TRIBUNAL_EVENTS = {
    EventType.TRIBUNAL_SESSION_STARTED,
    EventType.TRIBUNAL_SESSION_COMPLETED,
    EventType.TRIBUNAL_SESSION_DISABLED,
    EventType.TRIBUNAL_SESSION_MODEL_NOT_CONFIGURED,
    EventType.TRIBUNAL_SESSION_PROVIDER_UNAVAILABLE,
    EventType.TRIBUNAL_SESSION_SYSTEM_ERROR,
    EventType.TRIBUNAL_SESSION_GENERATION_FAILED,
    EventType.TRIBUNAL_SESSION_AUDITOR_FAILED,
}

class TribunalEmitter:
    """Handles emission of Tribunal SSE events via EventService."""

    def __init__(
        self,
        event_service: EventService,
        g8e_context: G8eHttpContext,
        correlation_id: str | None = None,
    ):
        self.event_service = event_service
        self.g8e_context = g8e_context
        self.correlation_id = correlation_id

    async def emit(self, event_type: EventType, payload: G8eBaseModel, correlation_id: str | None = None) -> None:
        """Emit an SSE event. Re-raises if event_type is terminal."""
        try:
            if self.event_service is None or self.g8e_context is None:
                return

            # Inject correlation_id if provided and supported by the payload
            # If not provided to emit, try to use the one stored on the emitter
            corr_id = correlation_id or getattr(self, "correlation_id", None)
            if corr_id and hasattr(payload, "correlation_id"):
                payload.correlation_id = corr_id

            event = SessionEvent(
                event_type=event_type,
                payload=payload,
                web_session_id=self.g8e_context.web_session_id,
                user_id=self.g8e_context.user_id,
                case_id=self.g8e_context.case_id,
                investigation_id=self.g8e_context.investigation_id,
                source_component=ComponentName.G8EE,
            )
            await self.event_service.publish(event)
        except Exception as exc:
            if event_type in _TERMINAL_TRIBUNAL_EVENTS:
                logger.error("[TRIBUNAL-EMIT] Terminal event %s failed: %s", event_type, exc)
                raise
            logger.warning("[TRIBUNAL-EMIT] Progress event %s failed (swallowed): %s", event_type, exc)

def _is_system_error(error_message: str) -> bool:
    """Classify an error message as a system error vs. a model error."""
    error_lower = error_message.lower()
    if "safety validation failed" in error_lower:
        return False
    system_indicators = [
        "401", "403", "unauthorized", "forbidden",
        "authentication", "api key",
        "connection refused", "connectionerror", "timeout",
        "dns", "ssl", "econnrefused",
        "unsupported llm provider",
    ]
    return any(indicator in error_lower for indicator in system_indicators)

def _member_for_pass(pass_index: int) -> TribunalMember:
    """Map a pass index to a Tribunal member."""
    members = [
        TribunalMember.AXIOM,
        TribunalMember.CONCORD,
        TribunalMember.VARIANCE,
        TribunalMember.PRAGMA,
        TribunalMember.NEMESIS,
    ]
    return members[pass_index % len(members)]

def _resolve_model(llm_settings: LLMSettings, tier: str = "assistant", request: str = "") -> str:
    """Resolve the concrete model string from settings based on tier."""
    if tier == "lite":
        resolved = llm_settings.resolved_lite_model
        if resolved:
            return resolved
    
    if tier == "assistant" and llm_settings.assistant_model:
        return llm_settings.assistant_model
    
    if tier == "primary" and llm_settings.primary_model:
        return llm_settings.primary_model

    # Fallback chain: lite -> assistant -> primary
    resolved = llm_settings.resolved_lite_model
    if resolved:
        return resolved
    if llm_settings.assistant_model:
        return llm_settings.assistant_model
    if llm_settings.primary_model:
        return llm_settings.primary_model

    provider = llm_settings.primary_provider or llm_settings.assistant_provider or llm_settings.lite_provider
    raise TribunalModelNotConfiguredError(
        provider=provider.value if provider else "unknown",
        request=request,
    )

class TribunalResponse(G8eBaseModel):
    """Structured response for Tribunal command generation."""
    command: str

async def _run_generation_pass(
    provider: LLMProvider,
    model: str,
    request: str,
    guidelines: str,
    operator_context: OperatorContext | None,
    pass_index: int,
    emitter: TribunalEmitter,
    pass_errors: list[str],
    command_constraints_message: str,
) -> str | None:
    """Run a single Tribunal generation pass."""
    member = _member_for_pass(pass_index)
    member_persona = get_agent_persona(member.value)
    fields = build_tribunal_prompt_fields(
        operator_context,
        request=request,
        guidelines=guidelines,
        default_os=DEFAULT_OS_NAME,
        default_shell=DEFAULT_SHELL,
        default_working_directory=DEFAULT_WORKING_DIRECTORY,
    )

    prompt = build_tribunal_generator_prompt(
        request=request,
        guidelines=guidelines,
        forbidden_patterns_message=fields["forbidden_patterns_message"],
        command_constraints_message=command_constraints_message,
        os=fields["os"],
        shell=fields["shell"],
        user_context=fields["user_context"],
        working_directory=fields["working_directory"],
        operator_context_str=fields["operator_context"],
    )

    logger.info(
        "[TRIBUNAL-PASS] pass=%d member=%s model=%s request_len=%d",
        pass_index, member.value, model, len(request),
    )

    model_config = get_model_config(model)

    response_format = None
    if model_config.supports_structured_output:
        response_format = ResponseFormat.from_pydantic_schema(
            TribunalResponse.model_json_schema(),
            name="TribunalResponse"
        )

    settings = LiteLLMSettings(
        max_output_tokens=_MAX_TOKENS_GENERATION,
        top_p_nucleus_sampling=model_config.top_p,
        top_k_filtering=model_config.top_k,
        stop_sequences=model_config.stop_sequences,
        system_instructions=member_persona.get_system_prompt(),
        response_format=response_format,
    )

    try:
        response = await provider.generate_content_lite(
            model=model,
            contents=[Content(role=Role.USER, parts=[Part.from_text(prompt)])],
            lite_llm_settings=settings,
        )

        if not response.text or not response.text.strip():
            error_msg = f"Pass {pass_index} ({member.value}): empty response"
            pass_errors.append(error_msg)
            logger.error("[TRIBUNAL-PASS] %s", error_msg)
            return None

        raw_command = response.text.strip()

        if model_config.supports_structured_output:
            parsed = extract_json_from_text(raw_command)
            if not (isinstance(parsed, dict) and isinstance(parsed.get("command"), str)):
                error_msg = f"Pass {pass_index} ({member.value}): structured output missing 'command' field"
                pass_errors.append(error_msg)
                logger.error("[TRIBUNAL-PASS] %s (raw=%r)", error_msg, raw_command[:100])
                return None
            raw_command = parsed["command"]

        normalised = normalise_command(raw_command)

        if not normalised:
            error_msg = f"Pass {pass_index} ({member.value}): normalisation failed"
            pass_errors.append(error_msg)
            logger.error("[TRIBUNAL-PASS] %s (raw=%r)", error_msg, raw_command[:100])
            return None

        is_safe, safety_err = validate_command_safety(normalised, False, False, operator_context)
        if not is_safe:
            error_msg = f"Pass {pass_index} ({member.value}): safety validation failed: {safety_err}"
            pass_errors.append(error_msg)
            logger.error("[TRIBUNAL-PASS] %s", error_msg)
            return None

        logger.info(
            "[TRIBUNAL-PASS] pass=%d member=%s success: cmd=%r",
            pass_index, member.value, normalised[:80],
        )

        await emitter.emit(
            EventType.TRIBUNAL_VOTING_PASS_COMPLETED,
            TribunalPassCompletedPayload(
                pass_index=pass_index,
                member=member,
                candidate=normalised,
                success=True,
            ),
        )

        return normalised

    except OllamaEmptyResponseError as exc:
        error_msg = f"Pass {pass_index} ({member.value}): {str(exc)}"
        pass_errors.append(error_msg)
        logger.error("[TRIBUNAL-PASS] %s", error_msg)
        return None
    except Exception as exc:
        error_msg = f"Pass {pass_index} ({member.value}): {str(exc)}"
        pass_errors.append(error_msg)
        logger.error("[TRIBUNAL-PASS] %s", error_msg, exc_info=True)
        return None

async def _run_generation_stage(
    provider: LLMProvider,
    model: str,
    request: str,
    guidelines: str,
    operator_context: OperatorContext | None,
    num_passes: int,
    emitter: TribunalEmitter,
    command_constraints_message: str,
) -> list[CandidateCommand]:
    """Stage 1: run N parallel generation passes and return successful candidates."""
    pass_errors: list[str] = []
    pass_tasks = [
        _run_generation_pass(
            provider=provider, model=model, request=request, guidelines=guidelines,
            operator_context=operator_context, pass_index=i, emitter=emitter, pass_errors=pass_errors,
            command_constraints_message=command_constraints_message,
        )
        for i in range(num_passes)
    ]
    raw_results = await asyncio.gather(*pass_tasks, return_exceptions=False)
    candidates = [
        CandidateCommand(command=res, pass_index=i, member=_member_for_pass(i))
        for i, res in enumerate(raw_results) if res
    ]

    if not candidates:
        if not pass_errors:
            raise AssertionError(
                "Tribunal invariant violated: all generation passes returned None but pass_errors is empty"
            )
        if all(_is_system_error(e) for e in pass_errors):
            logger.error(
                "[TRIBUNAL] All %d generation passes failed due to system errors: %s",
                num_passes, pass_errors,
            )
            await emitter.emit(
                EventType.TRIBUNAL_SESSION_SYSTEM_ERROR,
                TribunalSessionSystemErrorPayload(
                    request=request,
                    pass_errors=pass_errors,
                ),
            )
            raise TribunalSystemError(pass_errors=pass_errors, request=request)

        logger.error("[TRIBUNAL] All generation passes failed for non-system reasons; halting execution")
        await emitter.emit(
            EventType.TRIBUNAL_SESSION_GENERATION_FAILED,
            TribunalSessionGenerationFailedPayload(
                request=request,
                pass_errors=pass_errors,
            ),
        )
        raise TribunalGenerationFailedError(
            pass_errors=pass_errors,
            request=request,
        )

    return candidates

async def _run_voting_stage(
    candidates: list[CandidateCommand],
    request: str,
    emitter: TribunalEmitter,
    total_members: int,
) -> tuple[str | None, float, VoteBreakdown, list[CandidateCommand] | None]:
    """Stage 2: compute weighted majority vote and emit consensus event."""
    vote_winner, vote_score, vote_breakdown, tied_candidates = weighted_vote(candidates, total_members)

    if vote_winner is None:
        if vote_breakdown.consensus_strength > 0:
            logger.warning("[TRIBUNAL] Consensus strength too low: %.2f < %d members", vote_breakdown.consensus_strength, TRIBUNAL_MIN_CONSENSUS)
            logger.info("[TRIBUNAL-TELEMETRY] Candidate breakdown for consensus failure:")
            for member, cmd in vote_breakdown.candidates_by_member.items():
                logger.info("[TRIBUNAL-TELEMETRY]   %s: %s", member, cmd[:200] + "..." if len(cmd) > 200 else cmd)
        elif vote_breakdown.tie_break_reason == TieBreakReason.AUDITOR_DISAMBIGUATION:
            logger.info("[TRIBUNAL] Voting tied; auditor disambiguation required")
        else:
            logger.warning("[TRIBUNAL] Consensus failed: no agreement among members")
            logger.info("[TRIBUNAL-TELEMETRY] Candidate breakdown for consensus failure:")
            for member, cmd in vote_breakdown.candidates_by_member.items():
                logger.info("[TRIBUNAL-TELEMETRY]   %s: %s", member, cmd[:200] + "..." if len(cmd) > 200 else cmd)
            
        await emitter.emit(
            EventType.TRIBUNAL_VOTING_CONSENSUS_FAILED,
            TribunalConsensusFailedPayload(
                request=request,
                vote_breakdown=vote_breakdown,
            ),
        )
    else:
        await emitter.emit(
            EventType.TRIBUNAL_VOTING_CONSENSUS_REACHED,
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
                EventType.TRIBUNAL_VOTING_DISSENT_RECORDED,
                TribunalDissentRecordedPayload(
                    request=request,
                    losing_command=cmd,
                    dissenting_member_ids=members,
                    winner=vote_winner,
                    vote_breakdown=vote_breakdown,
                )
            )

    return vote_winner, vote_score, vote_breakdown, tied_candidates

async def _run_audit_stage(
    provider: LLMProvider,
    model: str,
    request: str,
    guidelines: str,
    vote_winner: str | None,
    vote_breakdown: VoteBreakdown,
    operator_context: OperatorContext | None,
    auditor_enabled: bool,
    emitter: TribunalEmitter,
    command_constraints_message: str,
    reputation_data_service: ReputationDataService,
    auditor_hmac_key: str,
    investigation_id: str,
    tied_candidates: list[CandidateCommand] | None = None,
) -> tuple[str | None, CommandGenerationOutcome, bool, str | None, AuditorReason, str | None]:
    """Stage 3: optionally audit the vote winner and determine outcome.

    On `auditor_passed=True` the auditor's verdict step also writes a
    Merkle commitment over the reputation scoreboard (GDD §14.4
    Artifact B). The returned `commitment_id` is the row id. Commitment
    failures (DB write error, etc.) emit `REPUTATION_COMMITMENT_FAILED`
    and are logged but non-fatal — the verdict still stands. The
    reputation dependencies are mandatory and have no "disabled" mode;
    operating without them is a configuration error surfaced at the
    ``generate_command`` call site.
    """
    if not auditor_enabled:
        return vote_winner, CommandGenerationOutcome.CONSENSUS, True, None, AuditorReason.OK, None

    if vote_breakdown.consensus_strength == 1.0:
        mode = "unanimous"
    elif vote_breakdown.tie_break_reason == TieBreakReason.AUDITOR_DISAMBIGUATION:
        mode = "tied"
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
    )

    outcome = CommandGenerationOutcome.VERIFIED if auditor_passed else CommandGenerationOutcome.VERIFICATION_FAILED

    if not auditor_passed and auditor_reason == AuditorReason.REVISED:
        outcome = CommandGenerationOutcome.VERIFICATION_FAILED

    commitment_id: str | None = None
    if auditor_passed:
        correlation_id = getattr(emitter, "correlation_id", None) or ""
        try:
            commitment = await commit_reputation(
                reputation_data_service=reputation_data_service,
                tribunal_command_id=correlation_id,
                investigation_id=investigation_id,
                hmac_key=auditor_hmac_key,
            )
            commitment_id = commitment.id
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
                "[TRIBUNAL-AUDITOR] reputation commitment failed (non-fatal): %s",
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

    return final_command, outcome, auditor_passed, auditor_revision, auditor_reason, commitment_id

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
) -> CommandGenerationResult:
    """Stage 4: assemble the result model and emit the session-completed event."""
    is_safe = True
    safety_error = None
    if final_command:
        is_safe, safety_error = validate_command_safety(
            final_command,
            whitelisting_enabled=whitelisting_enabled,
            blacklisting_enabled=blacklisting_enabled,
            operator_context=operator_context,
        )

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
        correlation_id=correlation_id,
        reputation_commitment_id=reputation_commitment_id,
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
    g8ed_event_service: EventService,
    web_session_id: str,
    user_id: str,
    case_id: str,
    investigation_id: str,
    settings: G8eeUserSettings,
    reputation_data_service: ReputationDataService,
    auditor_hmac_key: str,
    whitelisting_enabled: bool = False,
    blacklisting_enabled: bool = False,
    whitelisted_commands: list[dict[str, Any]] | None = None,
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

    if not request:
        raise TribunalGenerationFailedError(
            pass_errors=["Empty request submitted; cannot generate command"],
            request=request,
        )

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
    )

    vote_winner, vote_score, vote_breakdown, tied_candidates = await _run_voting_stage(
        candidates=candidates, request=request, emitter=emitter, total_members=num_passes,
    )

    if vote_winner is None and vote_breakdown.tie_break_reason != TieBreakReason.AUDITOR_DISAMBIGUATION:
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

    final_command, outcome, auditor_passed, auditor_revision, auditor_reason, commitment_id = await _run_audit_stage(
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
        reputation_commitment_id=commitment_id,
    )
