# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import hashlib
import json
from typing import TYPE_CHECKING

from app.constants import CommandErrorType
from app.models.agent import StreamChunkData
from app.models.evaluation_trace import (
    EvaluationGovernedActionRecord,
    EvaluationPolicyDecisionRecord,
    EvaluationToolCallRecord,
    EvaluationToolDecisionRecord,
)
from app.models.http_context import G8eHttpContext
from app.models.tool_results import CommandExecutionResult

if TYPE_CHECKING:
    from app.models.agent import AgentStreamState


def _hash_payload(value: object) -> str:
    encoded = json.dumps(value, ensure_ascii=False, separators=(",", ":"), sort_keys=True)
    return hashlib.sha256(encoded.encode()).hexdigest()


def _arguments_hash(chunk: StreamChunkData) -> str:
    if chunk.command:
        return _hash_payload({"command": chunk.command})
    if chunk.result is not None:
        return _hash_payload(chunk.result.model_dump(mode="json"))
    return ""


def _policy_outcome_from_result(result: CommandExecutionResult) -> str:
    if result.success:
        return "allow"
    if result.error_type in {
        CommandErrorType.SECURITY_VIOLATION,
        CommandErrorType.RISK_ANALYSIS_BLOCKED,
        CommandErrorType.VALIDATION_ERROR,
        CommandErrorType.G8E_RESOLUTION_ERROR,
        CommandErrorType.BLACKLIST_VIOLATION,
        CommandErrorType.WHITELIST_VIOLATION,
        CommandErrorType.PERMISSION_DENIED,
    }:
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
    state.tool_calls.append(
        EvaluationToolCallRecord(
            call_id=execution_id,
            tool_name=tool_name,
            arguments_hash=_arguments_hash(chunk),
            success=success,
            is_operator_tool=bool(chunk.is_operator_tool),
            execution_id=execution_id,
            error_type=chunk.error_type,
        )
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
        operator_id = operator.id or ""
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
