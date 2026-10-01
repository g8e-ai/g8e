# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import asyncio
from unittest.mock import MagicMock

import pytest

from app.models.agent import AgentInputs, AgentStreamState
from app.models.evaluation_trace import EvaluationProviderToolRejection, ToolGate
from app.models.http_context import G8eHttpContext
from app.models.model_telemetry import ModelCallTelemetry
from app.services.ai.chat_pipeline import ChatPipelineService
from g8e.models.internal_api import EvaluationInferenceContext, InferenceModelVariant


def _evaluation_context() -> EvaluationInferenceContext:
    return EvaluationInferenceContext(
        campaign_id="campaign-1",
        run_id="run-1",
        assignment_id="assignment-1",
        evaluation_attempt_id="attempt-1",
        scenario_id="scenario-1",
        model_registry_digest="d" * 64,
        model_registry=[InferenceModelVariant(model="model-a", digest="a" * 64)],
        target_operator_session_id="session-1",
    )


@pytest.mark.asyncio
async def test_finalize_evaluation_assignment_waits_for_memory_barrier():
    trace_service = MagicMock()
    pipeline = ChatPipelineService(
        event_service=MagicMock(),
        investigation_service=MagicMock(),
        request_builder=MagicMock(),
        g8e_agent=MagicMock(),
        memory_service=MagicMock(),
        memory_generation_service=MagicMock(),
        agent_activity_data_service=MagicMock(),
        evaluation_trace_service=trace_service,
    )

    g8e_context = G8eHttpContext(user_id="user-1", evaluation_context=_evaluation_context())
    agent_call = ModelCallTelemetry(
        agent_role="sage",
        model_role="primary",
        provider="G8EProvider",
        model="model-a",
        monotonic_start=1.0,
        monotonic_end=2.0,
    )
    memory_call = ModelCallTelemetry(
        agent_role="codex",
        model_role="lite",
        provider="G8EProvider",
        model="model-a",
        monotonic_start=3.0,
        monotonic_end=4.0,
    )
    state = AgentStreamState(model_calls=[agent_call])
    inputs = AgentInputs.model_construct(
        case_id="case-1",
        investigation_id="inv-1",
        user_id="user-1",
        g8e_context=g8e_context,
        task_id="chat",
        agent_mode="g8e_bound",
        active_agent="sage",
        operator_bound=True,
        model_to_use="model-a",
        max_tokens=1024,
        conversation_history=[],
        system_instructions="",
        contents=[],
        triage_result=None,
    )

    async def _memory_task() -> None:
        await asyncio.sleep(0.01)

    memory_holder = {
        "task": asyncio.create_task(_memory_task()),
        "model_call": memory_call,
    }

    await pipeline._finalize_evaluation_assignment(
        g8e_context=g8e_context,
        inputs=inputs,
        state=state,
        memory_holder=memory_holder,
    )

    trace_service.finalize.assert_called_once()
    kwargs = trace_service.finalize.call_args.kwargs
    assert kwargs["model_calls"] == [agent_call, memory_call]
    assert kwargs["status"] == "completed"


def _boundary_pipeline(trace_service) -> ChatPipelineService:
    return ChatPipelineService(
        event_service=MagicMock(),
        investigation_service=MagicMock(),
        request_builder=MagicMock(),
        g8e_agent=MagicMock(),
        memory_service=MagicMock(),
        memory_generation_service=MagicMock(),
        agent_activity_data_service=MagicMock(),
        evaluation_trace_service=trace_service,
    )


def _boundary_inputs(g8e_context: G8eHttpContext) -> AgentInputs:
    return AgentInputs.model_construct(
        case_id="case-1",
        investigation_id="inv-1",
        user_id="user-1",
        g8e_context=g8e_context,
        task_id="chat",
        agent_mode="g8e_bound",
        active_agent="sage",
        operator_bound=True,
        model_to_use="model-a",
        max_tokens=1024,
        conversation_history=[],
        system_instructions="",
        contents=[],
        triage_result=None,
    )


@pytest.mark.asyncio
async def test_finalize_records_the_eval_tool_gate_and_keeps_per_call_declared_tools():
    trace_service = MagicMock()
    pipeline = _boundary_pipeline(trace_service)
    g8e_context = G8eHttpContext(user_id="user-1", evaluation_context=_evaluation_context())
    agent_call = ModelCallTelemetry(
        agent_role="sage",
        model_role="primary",
        provider="G8EProvider",
        model="model-a",
        monotonic_start=1.0,
        monotonic_end=2.0,
        tools_declared=["recursive_grep_search", "file_read_on_operator"],
    )
    state = AgentStreamState(model_calls=[agent_call])

    await pipeline._finalize_evaluation_assignment(
        g8e_context=g8e_context,
        inputs=_boundary_inputs(g8e_context),
        state=state,
        memory_holder=None,
    )

    kwargs = trace_service.finalize.call_args.kwargs
    assert kwargs["model_calls"][0].tools_declared == [
        "recursive_grep_search",
        "file_read_on_operator",
    ]
    assert kwargs["tool_gate"] is ToolGate.BYPASSED_FOR_EVAL
    assert kwargs["provider_tool_rejection"] is None


@pytest.mark.asyncio
async def test_finalize_records_a_provider_tool_declaration_rejection_as_a_failed_trace():
    trace_service = MagicMock()
    pipeline = _boundary_pipeline(trace_service)
    g8e_context = G8eHttpContext(user_id="user-1", evaluation_context=_evaluation_context())
    rejection = EvaluationProviderToolRejection(
        model="model-a", reason="Provider rejected the tool declaration"
    )
    state = AgentStreamState(
        provider_tool_rejection=rejection,
        stream_failed=True,
        error="Provider rejected the tool declaration",
    )

    await pipeline._finalize_evaluation_assignment(
        g8e_context=g8e_context,
        inputs=_boundary_inputs(g8e_context),
        state=state,
        memory_holder=None,
    )

    kwargs = trace_service.finalize.call_args.kwargs
    assert kwargs["status"] == "failed"
    assert kwargs["provider_tool_rejection"] is rejection
