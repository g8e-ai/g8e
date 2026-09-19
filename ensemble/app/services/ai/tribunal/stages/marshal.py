# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import logging
from typing import Any
from app.constants import EventType, RiskLevel
from app.models.agent import OperatorContext
from app.models.settings import G8eeUserSettings
from app.models.agents.tribunal import (
    TribunalMarshalBlockedError,
    TribunalMarshalBlockedPayload,
)
from app.models.tool_results import (
    CommandRiskAnalysis,
    CommandRiskContext,
    ErrorAnalysisContext,
)
from app.services.protocols import AIResponseAnalyzerProtocol
from app.services.ai.tribunal.emitter import TribunalEmitter

logger = logging.getLogger(__name__)


async def _run_marshal_stage(
    request: str,
    guidelines: str,
    vote_winner: str,
    operator_context: OperatorContext | None,
    emitter: TribunalEmitter,
    settings: G8eeUserSettings,
    investigation_id: str,
    ai_response_analyzer: AIResponseAnalyzerProtocol | None,
    investigation_state: Any | None,
    investigation_context: str = "",
) -> CommandRiskAnalysis | None:
    """Stage 3a: Marshal risk analysis on the consensus winner.

    Runs Marshal command-risk analysis before the Auditor sees the command.
    Returns the analysis (or None if no analyzer is configured or the
    analyzer returned no result).

    Raises:
        TribunalMarshalBlockedError: When Marshal classifies the command as
            HIGH risk. The Two-Strike Circuit Breaker decides the variant:
            on the first strike for an investigation, emits
            ``AI_CONSENSUS_SESSION_MARSHAL_BLOCKED`` with contextual feedback so
            Sage can propose a safer alternative; on the second strike,
            emits ``AI_AGENT_CONFLICT_DETECTED`` signalling that the AI
            agents cannot agree on a safe approach and human intervention
            is required.
    """
    if not ai_response_analyzer:
        return None

    logger.info(
        "[MARSHAL] Starting risk analysis for command: %r",
        vote_winner[:200] + "..." if len(vote_winner) > 200 else vote_winner,
    )

    justification_parts = [request.strip()] if request else []
    if guidelines and guidelines.strip():
        justification_parts.append(f"Guidelines: {guidelines.strip()}")
    justification = (
        " | ".join(justification_parts) if justification_parts else "(no justification provided)"
    )

    risk_analysis = await ai_response_analyzer.analyze_command_risk(
        command=vote_winner,
        justification=justification,
        context=CommandRiskContext(
            working_directory=operator_context.working_directory if operator_context else "",
            investigation_context=investigation_context,
        ),
        settings=settings,
    )

    if not risk_analysis:
        return None

    model_calls = [risk_analysis.model_call] if risk_analysis.model_call else []
    logger.info("[MARSHAL] Risk analysis complete: level=%s", risk_analysis.risk_level)

    if risk_analysis.risk_level != RiskLevel.HIGH:
        return risk_analysis

    block_count = investigation_state.marshal_block_count if investigation_state else 0

    if block_count >= 1:
        logger.warning(
            "[MARSHAL-CIRCUIT-BREAKER] Second marshal block detected for investigation=%s - triggering AGENT_CONFLICT",
            investigation_id,
        )
        logger.warning(
            "[MARSHAL] Blocking command due to repeated HIGH risk detection: %r",
            vote_winner,
        )
        if investigation_state:
            investigation_state.marshal_block_count = 0

        await emitter.emit(
            EventType.AI_AGENT_CONFLICT_DETECTED,
            TribunalMarshalBlockedPayload(
                request=request,
                command=vote_winner,
                risk_level=risk_analysis.risk_level,
                error="AGENT CONFLICT: Marshal blocked Sage's command twice. The AI agents cannot agree on a safe approach. Human intervention required.",
                is_conflict=True,
                model_calls=model_calls,
            ),
        )
        raise TribunalMarshalBlockedError(
            request=request,
            error_message="Agent Conflict: Marshal blocked Sage's command twice. The AI agents cannot agree on a safe approach.",
            risk_level=risk_analysis.risk_level,
        )

    logger.info(
        "[MARSHAL-CIRCUIT-BREAKER] First marshal block for investigation=%s - generating contextual feedback",
        investigation_id,
    )
    logger.info("[MARSHAL] Blocking command due to HIGH risk detection: %r", vote_winner)
    if investigation_state:
        investigation_state.marshal_block_count = block_count + 1

    error_analysis = await ai_response_analyzer.analyze_error_and_suggest_fix(
        command=vote_winner,
        exit_code=None,
        stdout="",
        stderr=f"MARSHAL BLOCK: Command classified as HIGH risk. Justification: {justification}",
        context=ErrorAnalysisContext(
            retry_count=0,
            working_directory=operator_context.working_directory if operator_context else "",
        ),
        settings=settings,
    )

    if error_analysis and error_analysis.model_call:
        model_calls.append(error_analysis.model_call)
    feedback_msg = (
        error_analysis.user_message
        if error_analysis and error_analysis.user_message
        else "Command blocked as high risk. Propose a safer alternative."
    )
    if error_analysis and error_analysis.suggested_fix:
        feedback_msg += f" Suggestion: {error_analysis.suggested_fix}"

    logger.info("[MARSHAL] Feedback for Sage: %s", feedback_msg)

    await emitter.emit(
        EventType.AI_CONSENSUS_SESSION_MARSHAL_BLOCKED,
        TribunalMarshalBlockedPayload(
            request=request,
            command=vote_winner,
            risk_level=risk_analysis.risk_level,
            error=f"MARSHAL BLOCK: {feedback_msg}",
            is_conflict=False,
            model_calls=model_calls,
        ),
    )
    raise TribunalMarshalBlockedError(
        request=request,
        error_message=f"Risk analysis blocked command: {feedback_msg}",
        risk_level=risk_analysis.risk_level,
    )
