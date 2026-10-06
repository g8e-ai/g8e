# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import hashlib
from typing import TYPE_CHECKING

from g8e.eval.v1.trace_digest import marshal_canonical_json

from app.constants import CommandErrorType
from app.models.agent import StreamChunkData
from app.models.evaluation_trace import (
    EvaluationErrorAnalysisSummary,
    EvaluationGovernedActionRecord,
    EvaluationPolicyDecisionRecord,
    EvaluationPolicyOutcome,
    EvaluationToolCallRecord,
    EvaluationToolDecisionRecord,
)
from app.models.http_context import G8eHttpContext
from app.models.tool_results import CommandExecutionResult
from app.services.evaluation.player_steps import attach_to_call

if TYPE_CHECKING:
    from app.models.agent import AgentStreamState


def _canonical_json(value: object) -> str:
    return marshal_canonical_json(value).decode()


def _model_visible_text(result: object, field: str) -> str | None:
    """A non-empty string field of the result the model was shown, or ``None``."""
    value = getattr(result, field, None)
    return value if isinstance(value, str) and value else None


def _error_analysis_summary(result: object) -> EvaluationErrorAnalysisSummary | None:
    """Summarize the LLM error analysis attached to a failed operator command."""
    if not isinstance(result, CommandExecutionResult):
        return None
    for candidate in (result.execution_result, *(result.execution_results or [])):
        analysis = candidate.error_analysis if candidate is not None else None
        if analysis is not None:
            return EvaluationErrorAnalysisSummary(
                error_category=str(analysis.error_category),
                root_cause=analysis.root_cause,
                suggested_fix=analysis.suggested_fix,
                suggested_command=analysis.suggested_command,
                should_escalate=analysis.should_escalate,
            )
    return None


# Failures recorded as a ``deny`` policy decision. The Go grader reads the same
# set from the generated agent tool registry to tell a denied call from any
# other failed call, so this is its single source.
POLICY_DENY_ERROR_TYPES: frozenset[CommandErrorType] = frozenset(
    {
        CommandErrorType.SECURITY_VIOLATION,
        CommandErrorType.RISK_ANALYSIS_BLOCKED,
        CommandErrorType.VALIDATION_ERROR,
        CommandErrorType.G8E_RESOLUTION_ERROR,
        CommandErrorType.BLACKLIST_VIOLATION,
        CommandErrorType.WHITELIST_VIOLATION,
        CommandErrorType.PERMISSION_DENIED,
    }
)


def _policy_outcome_from_result(result: CommandExecutionResult) -> EvaluationPolicyOutcome:
    if result.success:
        return "allow"
    if result.error_type in POLICY_DENY_ERROR_TYPES:
        return "deny"
    return "refused"


def record_tool_call_started(
    state: AgentStreamState,
    g8e_context: G8eHttpContext,
    chunk: StreamChunkData,
) -> None:
    if g8e_context.evaluation_context is None:
        return
    tool_name = (chunk.tool_name or "").strip()
    if not tool_name:
        return
    execution_id = chunk.execution_id or f"{tool_name}:{len(state.tool_decisions) + 1}"
    state.tool_decisions.append(
        EvaluationToolDecisionRecord(
            decision_id=execution_id,
            tool_name=tool_name,
            recognized=True,
            selected=True,
            permission_compliant=True,
            unnecessary=False,
            outcome="pass",
        )
    )


def record_tool_call_completed(
    state: AgentStreamState,
    g8e_context: G8eHttpContext,
    chunk: StreamChunkData,
) -> None:
    if g8e_context.evaluation_context is None:
        return
    tool_name = (chunk.tool_name or "").strip()
    if not tool_name:
        return
    execution_id = chunk.execution_id or f"{tool_name}:{len(state.tool_calls) + 1}"
    success = bool(chunk.success)
    arguments_json = _canonical_json(chunk.arguments or {})
    state.tool_calls.append(
        EvaluationToolCallRecord(
            call_id=execution_id,
            tool_name=tool_name,
            arguments_json=arguments_json,
            arguments_hash=hashlib.sha256(arguments_json.encode()).hexdigest(),
            command=chunk.command or "",
            result_json=_canonical_json(chunk.result.model_dump(mode="json"))
            if chunk.result is not None
            else "",
            success=success,
            is_operator_tool=bool(chunk.is_operator_tool),
            execution_id=execution_id,
            error_type=chunk.error_type,
            loop_turn=chunk.loop_turn,
            error=_model_visible_text(chunk.result, "error"),
            suggestion=_model_visible_text(chunk.result, "suggestion"),
            error_analysis=_error_analysis_summary(chunk.result),
        )
    )
    if chunk.player_steps:
        state.player_steps.extend(
            attach_to_call(chunk.player_steps, execution_id, len(state.player_steps) + 1)
        )
    if not isinstance(chunk.result, CommandExecutionResult):
        return
    result = chunk.result
    policy_outcome = _policy_outcome_from_result(result)
    if not success:
        state.policy_decisions.append(
            EvaluationPolicyDecisionRecord(
                decision_id=execution_id,
                tool_name=tool_name,
                outcome=policy_outcome,
                detail=result.error or result.denial_reason or chunk.error_type or "",
            )
        )
        return
    if not chunk.is_operator_tool:
        return
    operator_id = ""
    operator_session_id = ""
    if g8e_context.bound_operators:
        operator = g8e_context.bound_operators[0]
        operator_id = operator.operator_id or ""
        operator_session_id = operator.operator_session_id or ""
    state.governed_actions.append(
        EvaluationGovernedActionRecord(
            binding_id=execution_id,
            transaction_id=execution_id,
            operator_id=operator_id,
            operator_session_id=operator_session_id,
            receipt_status="completed" if success else "failed",
            policy_decision="allow",
        )
    )
