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
from typing import Any
from app.constants import EventType, RiskLevel
from app.models.agent import OperatorContext
from app.models.settings import G8eeUserSettings
from app.models.agents.tribunal import (
    TribunalWardenBlockedError,
    TribunalWardenBlockedPayload,
)
from app.models.tool_results import (
    CommandRiskAnalysis,
    CommandRiskContext,
    ErrorAnalysisContext,
)
from app.services.protocols import AIResponseAnalyzerProtocol
from app.services.ai.tribunal.emitter import TribunalEmitter

logger = logging.getLogger(__name__)

async def _run_warden_stage(
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
    """Stage 3a: Warden risk analysis on the consensus winner.

    Runs Warden command-risk analysis before the Auditor sees the command.
    Returns the analysis (or None if no analyzer is configured or the
    analyzer returned no result).

    Raises:
        TribunalWardenBlockedError: When Warden classifies the command as
            HIGH risk. The Two-Strike Circuit Breaker decides the variant:
            on the first strike for an investigation, emits
            ``TRIBUNAL_SESSION_WARDEN_BLOCKED`` with contextual feedback so
            Sage can propose a safer alternative; on the second strike,
            emits ``AI_AGENT_CONFLICT_DETECTED`` signalling that the AI
            agents cannot agree on a safe approach and human intervention
            is required.
    """
    if not ai_response_analyzer:
        return None

    logger.info(
        "[TRIBUNAL-WARDEN] Starting risk analysis for command: %r",
        vote_winner[:200] + "..." if len(vote_winner) > 200 else vote_winner,
    )

    justification_parts = [request.strip()] if request else []
    if guidelines and guidelines.strip():
        justification_parts.append(f"Guidelines: {guidelines.strip()}")
    justification = " | ".join(justification_parts) if justification_parts else "(no justification provided)"

    risk_analysis = await ai_response_analyzer.analyze_command_risk(
        command=vote_winner,
        justification=justification,
        context=CommandRiskContext(
            working_directory=operator_context.working_directory if operator_context else "",
            investigation_context=investigation_context,
        ),
        settings=settings,
    )

    if risk_analysis is None:
        return None

    logger.info("[TRIBUNAL-WARDEN] Risk analysis complete: level=%s", risk_analysis.risk_level)

    if risk_analysis.risk_level != RiskLevel.HIGH:
        return risk_analysis

    block_count = investigation_state.warden_block_count if investigation_state else 0

    if block_count >= 1:
        logger.warning(
            "[WARDEN-CIRCUIT-BREAKER] Second warden block detected for investigation=%s - triggering AGENT_CONFLICT",
            investigation_id,
        )
        logger.warning("[TRIBUNAL-WARDEN] Blocking command due to repeated HIGH risk detection: %r", vote_winner)
        if investigation_state:
            investigation_state.warden_block_count = 0

        await emitter.emit(
            EventType.AI_AGENT_CONFLICT_DETECTED,
            TribunalWardenBlockedPayload(
                request=request,
                command=vote_winner,
                risk_level=risk_analysis.risk_level,
                error="AGENT CONFLICT: Warden blocked Sage's command twice. The AI agents cannot agree on a safe approach. Human intervention required.",
                is_conflict=True,
            ),
        )
        raise TribunalWardenBlockedError(
            request=request,
            error_message="Agent Conflict: Warden blocked Sage's command twice. The AI agents cannot agree on a safe approach.",
            risk_level=risk_analysis.risk_level,
        )

    logger.info(
        "[WARDEN-CIRCUIT-BREAKER] First warden block for investigation=%s - generating contextual feedback",
        investigation_id,
    )
    logger.info("[TRIBUNAL-WARDEN] Blocking command due to HIGH risk detection: %r", vote_winner)
    if investigation_state:
        investigation_state.warden_block_count = block_count + 1

    error_analysis = await ai_response_analyzer.analyze_error_and_suggest_fix(
        command=vote_winner,
        exit_code=None,
        stdout="",
        stderr=f"WARDEN BLOCK: Command classified as HIGH risk. Justification: {justification}",
        context=ErrorAnalysisContext(
            retry_count=0,
            working_directory=operator_context.working_directory if operator_context else "",
        ),
        settings=settings,
    )

    feedback_msg = (
        error_analysis.user_message
        if error_analysis and error_analysis.user_message
        else "Command blocked as high risk. Propose a safer alternative."
    )
    if error_analysis and error_analysis.suggested_fix:
        feedback_msg += f" Suggestion: {error_analysis.suggested_fix}"

    logger.info("[TRIBUNAL-WARDEN] Feedback for Sage: %s", feedback_msg)

    await emitter.emit(
        EventType.TRIBUNAL_SESSION_WARDEN_BLOCKED,
        TribunalWardenBlockedPayload(
            request=request,
            command=vote_winner,
            risk_level=risk_analysis.risk_level,
            error=f"WARDEN BLOCK: {feedback_msg}",
            is_conflict=False,
        ),
    )
    raise TribunalWardenBlockedError(
        request=request,
        error_message=f"Risk analysis blocked command: {feedback_msg}",
        risk_level=risk_analysis.risk_level,
    )
