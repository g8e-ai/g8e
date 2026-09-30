# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import hashlib
import json

from app.constants import CommandErrorType
from app.llm.llm_dataclasses import ToolCall
from app.models.agent import AgentStreamState, StreamChunkData
from app.models.http_context import G8eHttpContext
from app.models.tool_results import CommandExecutionResult
from app.services.ai.agent_tool_loop import _tribunal_error_result
from app.services.evaluation.tool_evidence import (
    record_tool_call_completed,
    record_tool_call_started,
)
from g8e.eval.v1.trace_digest import marshal_canonical_json
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
            arguments={"pattern": "AUTH_FAILURE", "path": "logs", "max_results": 2.0},
            result=CommandExecutionResult(success=True, output="auth.log:3: AUTH_FAILURE"),
        ),
    )

    assert len(state.tool_decisions) == 1
    assert state.tool_decisions[0].tool_name == "recursive_grep_search"
    assert len(state.tool_calls) == 1
    call = state.tool_calls[0]
    assert call.success is True
    assert call.command == "AUTH_FAILURE"
    assert json.loads(call.arguments_json) == {"pattern": "AUTH_FAILURE", "path": "logs", "max_results": 2.0}
    assert call.arguments_json == marshal_canonical_json(json.loads(call.arguments_json)).decode()
    assert call.arguments_hash == hashlib.sha256(call.arguments_json.encode()).hexdigest()
    assert json.loads(call.result_json)["output"] == "auth.log:3: AUTH_FAILURE"
    assert len(state.governed_actions) == 0


def test_record_tool_call_completed_uses_producer_result_chunk():
    """The TOOL_RESULT chunk built by orchestrate_tool_execution must identify
    its tool; before it did, every tool call was silently dropped from traces."""
    state = AgentStreamState()
    context = G8eHttpContext(user_id="user-1", evaluation_context=_evaluation_context())
    tool_call = ToolCall(name="recursive_grep_search", args={"pattern": "AUTH_FAILURE"}, id="fc-1")

    result = _tribunal_error_result(
        tool_name=tool_call.name,
        arguments=dict(tool_call.args),
        request="grep for auth failures",
        error_msg="tribunal unavailable",
    )
    record_tool_call_started(state, context, result.call_info)
    record_tool_call_completed(state, context, result.result_info)

    assert [call.tool_name for call in state.tool_calls] == ["recursive_grep_search"]
    assert json.loads(state.tool_calls[0].arguments_json) == {"pattern": "AUTH_FAILURE"}
    assert state.tool_calls[0].success is False
    assert len(state.policy_decisions) == 1


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
