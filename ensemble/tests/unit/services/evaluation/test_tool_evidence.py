# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import hashlib
import json

from app.constants import CommandErrorType, ErrorAnalysisCategory, ExecutionStatus
from app.llm.llm_dataclasses import ToolCall
from app.models.agent import AgentStreamState, StreamChunkData
from app.models.http_context import G8eHttpContext
from app.models.tool_results import (
    CommandExecutionResult,
    CommandInternalResult,
    ErrorAnalysisResult,
    FsGrepToolResult,
)
from app.services.ai.agent_tool_loop import _tribunal_error_result
from app.services.evaluation.tool_evidence import (
    record_tool_call_completed,
    record_tool_call_started,
)
from g8e.eval.v1.trace_digest import marshal_canonical_json
from g8e.models.context import BoundOperator
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


def _completed(state, context, **chunk_fields):
    record_tool_call_completed(
        state,
        context,
        StreamChunkData(
            tool_name=chunk_fields.pop("tool_name", "recursive_grep_search"),
            execution_id=chunk_fields.pop("execution_id", "exec-1"),
            **chunk_fields,
        ),
    )
    return state.tool_calls[-1]


def test_tool_call_record_carries_loop_turn_and_the_validation_guidance_shown_to_the_model():
    """A rejected call must keep the exact guidance the model saw, so a later
    corrected call can be graded as guidance followed rather than a lucky retry."""
    state = AgentStreamState()
    context = G8eHttpContext(user_id="user-1", evaluation_context=_evaluation_context())
    validation_error = (
        "1 validation error for RecursiveGrepArgs\npath\n  Field required [type=missing]"
    )

    call = _completed(
        state,
        context,
        success=False,
        loop_turn=2,
        arguments={"pattern": "AUTH_FAILURE"},
        result=CommandExecutionResult(
            success=False,
            error=validation_error,
            error_type=CommandErrorType.VALIDATION_ERROR,
            suggestion="Provide the directory to search as path",
        ),
        error_type=CommandErrorType.VALIDATION_ERROR,
    )

    assert call.loop_turn == 2
    assert call.error == validation_error
    assert call.error_type == CommandErrorType.VALIDATION_ERROR.value
    assert call.suggestion == "Provide the directory to search as path"
    assert call.error_analysis is None


def test_tool_call_record_summarizes_the_llm_error_analysis_shown_to_the_model():
    state = AgentStreamState()
    context = G8eHttpContext(user_id="user-1", evaluation_context=_evaluation_context())
    analysis = ErrorAnalysisResult(
        error_category=ErrorAnalysisCategory.DEPENDENCY,
        root_cause="deploy-healthcheck is not installed",
        can_auto_fix=False,
        suggested_fix="Install the healthcheck package",
        suggested_command="apt-get install deploy-healthcheck",
        should_escalate=True,
        reasoning="missing binary",
        user_message="command not found",
    )

    call = _completed(
        state,
        context,
        tool_name="run_commands_with_operator",
        success=False,
        is_operator_tool=True,
        loop_turn=1,
        result=CommandExecutionResult(
            success=False,
            error="command not found",
            error_type=CommandErrorType.EXECUTION_ERROR,
            execution_result=CommandInternalResult(
                status=ExecutionStatus.FAILED, error_analysis=analysis
            ),
        ),
    )

    assert call.error_analysis is not None
    assert call.error_analysis.error_category == ErrorAnalysisCategory.DEPENDENCY.value
    assert call.error_analysis.root_cause == "deploy-healthcheck is not installed"
    assert call.error_analysis.suggested_fix == "Install the healthcheck package"
    assert call.error_analysis.suggested_command == "apt-get install deploy-healthcheck"
    assert call.error_analysis.should_escalate is True


def test_tool_call_record_captures_the_error_of_non_command_tool_results():
    state = AgentStreamState()
    context = G8eHttpContext(user_id="user-1", evaluation_context=_evaluation_context())

    call = _completed(
        state,
        context,
        success=False,
        loop_turn=1,
        result=FsGrepToolResult(success=False, error="path does not exist: /missing"),
    )

    assert call.error == "path does not exist: /missing"
    assert call.suggestion is None


def test_successful_tool_call_record_has_no_guidance_fields():
    state = AgentStreamState()
    context = G8eHttpContext(user_id="user-1", evaluation_context=_evaluation_context())

    call = _completed(
        state,
        context,
        success=True,
        loop_turn=3,
        result=CommandExecutionResult(success=True, output="auth.log:3: AUTH_FAILURE"),
    )

    assert call.loop_turn == 3
    assert call.error is None
    assert call.suggestion is None
    assert call.error_analysis is None


def test_tool_call_record_without_a_loop_turn_leaves_it_unset():
    state = AgentStreamState()
    context = G8eHttpContext(user_id="user-1", evaluation_context=_evaluation_context())

    call = _completed(
        state, context, success=True, result=CommandExecutionResult(success=True, output="ok")
    )

    assert call.loop_turn is None


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


def test_successful_operator_call_binds_the_governed_action_to_the_bound_operator():
    state = AgentStreamState()
    context = G8eHttpContext(
        user_id="user-1",
        evaluation_context=_evaluation_context(),
        bound_operators=[
            BoundOperator(operator_id="op-1", operator_session_id="op-session-1"),
            BoundOperator(operator_id="op-2", operator_session_id="op-session-2"),
        ],
    )

    record_tool_call_completed(
        state,
        context,
        StreamChunkData(
            tool_name="file_read_on_operator",
            execution_id="exec-3",
            success=True,
            is_operator_tool=True,
            result=CommandExecutionResult(success=True, output="retry_limit=3"),
        ),
    )

    assert len(state.governed_actions) == 1
    action = state.governed_actions[0]
    assert action.binding_id == "exec-3"
    assert action.operator_id == "op-1"
    assert action.operator_session_id == "op-session-1"
    assert action.policy_decision == "allow"
    assert action.receipt_status == "completed"


def test_successful_operator_call_without_a_bound_operator_records_empty_operator_ids():
    state = AgentStreamState()
    context = G8eHttpContext(user_id="user-1", evaluation_context=_evaluation_context())

    record_tool_call_completed(
        state,
        context,
        StreamChunkData(
            tool_name="file_read_on_operator",
            execution_id="exec-4",
            success=True,
            is_operator_tool=True,
            result=CommandExecutionResult(success=True, output="x"),
        ),
    )

    assert [(a.operator_id, a.operator_session_id) for a in state.governed_actions] == [("", "")]


def test_requests_without_an_evaluation_context_record_nothing():
    state = AgentStreamState()
    context = G8eHttpContext(user_id="user-1")
    chunk = StreamChunkData(
        tool_name="recursive_grep_search",
        execution_id="exec-5",
        success=True,
        result=CommandExecutionResult(success=True, output="x"),
    )

    record_tool_call_started(state, context, chunk)
    record_tool_call_completed(state, context, chunk)

    assert (state.tool_decisions, state.tool_calls, state.policy_decisions, state.governed_actions) == (
        [],
        [],
        [],
        [],
    )


def test_chunks_without_a_tool_name_record_nothing():
    state = AgentStreamState()
    context = G8eHttpContext(user_id="user-1", evaluation_context=_evaluation_context())
    chunk = StreamChunkData(tool_name="  ", execution_id="exec-6", success=True)

    record_tool_call_started(state, context, chunk)
    record_tool_call_completed(state, context, chunk)

    assert (state.tool_decisions, state.tool_calls) == ([], [])


def test_non_command_results_record_the_call_but_no_policy_or_governed_evidence():
    state = AgentStreamState()
    context = G8eHttpContext(user_id="user-1", evaluation_context=_evaluation_context())

    _completed(
        state,
        context,
        success=True,
        loop_turn=1,
        result=FsGrepToolResult(success=True),
    )

    assert len(state.tool_calls) == 1
    assert state.policy_decisions == []
    assert state.governed_actions == []
