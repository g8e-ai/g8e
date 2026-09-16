# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from app.constants import CommandErrorType
from app.models.agent import AgentStreamState, StreamChunkData
from app.models.http_context import G8eHttpContext
from app.models.tool_results import CommandExecutionResult
from app.services.evaluation.tool_evidence import (
    record_tool_call_completed,
    record_tool_call_started,
)
from g8e.models.internal_api import EvaluationInferenceContext, InferenceModelVariant


def _evaluation_context() -> EvaluationInferenceContext:
    return EvaluationInferenceContext(
        campaign_id="campaign-1",
        run_id="run-1",
        assignment_id="assignment-1",
        evaluation_attempt_id="attempt-1",
        scenario_id="tool-select-grep",
        model_registry_digest="d" * 64,
        model_registry=[InferenceModelVariant(model="model-a", digest="a" * 64)],
        target_operator_session_id="session-1",
        evaluation_lane="model_role",
        designated_model_role="primary",
    )


def test_record_tool_call_started_and_completed_capture_evidence():
    state = AgentStreamState()
    context = G8eHttpContext(user_id="user-1", evaluation_context=_evaluation_context())

    record_tool_call_started(
        state,
        context,
        StreamChunkData(
            tool_name="recursive_grep_search",
            execution_id="exec-1",
            command="AUTH_FAILURE",
            is_operator_tool=False,
        ),
    )
    record_tool_call_completed(
        state,
        context,
        StreamChunkData(
            tool_name="recursive_grep_search",
            execution_id="exec-1",
            success=True,
            is_operator_tool=False,
            command="AUTH_FAILURE",
        ),
    )

    assert len(state.tool_decisions) == 1
    assert state.tool_decisions[0].tool_name == "recursive_grep_search"
    assert len(state.tool_calls) == 1
    assert state.tool_calls[0].success is True
    assert len(state.governed_actions) == 0


def test_record_tool_call_completed_captures_governed_action_and_policy_denial():
    state = AgentStreamState()
    context = G8eHttpContext(user_id="user-1", evaluation_context=_evaluation_context())

    denied = CommandExecutionResult(
        success=False,
        error="policy blocked command",
        error_type=CommandErrorType.RISK_ANALYSIS_BLOCKED,
    )
    record_tool_call_started(
        state,
        context,
        StreamChunkData(
            tool_name="run_commands_with_operator",
            execution_id="exec-2",
            command="rm -rf /",
            is_operator_tool=True,
        ),
    )
    record_tool_call_completed(
        state,
        context,
        StreamChunkData(
            tool_name="run_commands_with_operator",
            execution_id="exec-2",
            success=False,
            is_operator_tool=True,
            command="rm -rf /",
            result=denied,
            error_type=CommandErrorType.RISK_ANALYSIS_BLOCKED,
        ),
    )

    assert len(state.policy_decisions) == 1
    assert state.policy_decisions[0].outcome == "deny"
    assert len(state.governed_actions) == 0
