# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import asyncio
from unittest.mock import AsyncMock, MagicMock

import pytest

from app.models.agent import AgentInputs, AgentStreamState
from app.models.evaluation_trace import (
    EvaluationProviderToolRejection,
    EvaluationSeedApplication,
    ToolGate,
)
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
        classification="scored_chain",
        model_role="primary",
        provider="G8EProvider",
        model="model-a",
        monotonic_start=1.0,
        monotonic_end=2.0,
    )
    memory_call = ModelCallTelemetry(
        agent_role="codex",
        classification="post_turn",
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
        classification="scored_chain",
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


@pytest.mark.asyncio
async def test_finalize_records_the_eval_only_divergences_and_the_seed_application():
    trace_service = MagicMock()
    pipeline = _boundary_pipeline(trace_service)
    g8e_context = G8eHttpContext(user_id="user-1", evaluation_context=_evaluation_context())
    inputs = AgentInputs.model_construct(
        **{**_boundary_inputs(g8e_context).__dict__, "user_memories_suppressed": True}
    )
    state = AgentStreamState(tool_turn_limit_reached=True, finish_reason="tool_turn_limit")
    seed_application = EvaluationSeedApplication(turns=2, history_events=1, case_memory=True)

    await pipeline._finalize_evaluation_assignment(
        g8e_context=g8e_context,
        inputs=inputs,
        state=state,
        memory_holder=None,
        seed_application=seed_application,
    )

    kwargs = trace_service.finalize.call_args.kwargs
    assert kwargs["tool_turn_limit_reached"] is True
    assert kwargs["user_memories_suppressed"] is True
    assert kwargs["seed_application"] is seed_application
    assert kwargs["status"] == "completed"


@pytest.mark.asyncio
async def test_finalize_records_no_divergence_for_an_unseeded_run_that_finished_normally():
    trace_service = MagicMock()
    pipeline = _boundary_pipeline(trace_service)
    g8e_context = G8eHttpContext(user_id="user-1", evaluation_context=_evaluation_context())

    await pipeline._finalize_evaluation_assignment(
        g8e_context=g8e_context,
        inputs=_boundary_inputs(g8e_context),
        state=AgentStreamState(),
        memory_holder=None,
    )

    kwargs = trace_service.finalize.call_args.kwargs
    assert kwargs["tool_turn_limit_reached"] is False
    assert kwargs["user_memories_suppressed"] is False
    assert kwargs["seed_application"] is None


async def _run_a_crashing_chat(pipeline: ChatPipelineService, g8e_context: G8eHttpContext, **extra):
    pipeline._run_chat_impl = AsyncMock(side_effect=RuntimeError("pipeline exploded"))
    pipeline.event_service = MagicMock()
    pipeline.event_service.publish_investigation_event = AsyncMock()
    task_manager = MagicMock()
    task_manager.track = AsyncMock()
    task_manager.untrack = AsyncMock()

    await pipeline.run_chat(
        message="hello",
        g8e_context=g8e_context,
        attachments=[],
        sentinel_mode=True,
        llm_primary_provider=None,
        llm_assistant_provider=None,
        llm_lite_provider=None,
        llm_primary_model="model-a",
        llm_assistant_model="model-a",
        llm_lite_model="model-a",
        _task_manager=task_manager,
        user_settings=MagicMock(),
        **extra,
    )
    return pipeline.event_service.publish_investigation_event


@pytest.mark.asyncio
async def test_a_crashed_scored_chat_finalizes_its_trace_as_failed_so_the_harness_stops_waiting():
    trace_service = MagicMock()
    pipeline = _boundary_pipeline(trace_service)
    g8e_context = G8eHttpContext(
        user_id="user-1", investigation_id="inv-1", evaluation_context=_evaluation_context()
    )
    seed_application = EvaluationSeedApplication(turns=1)

    publish = await _run_a_crashing_chat(pipeline, g8e_context, seed_application=seed_application)

    trace_service.finalize_crashed.assert_called_once()
    args, kwargs = trace_service.finalize_crashed.call_args
    assert args == (g8e_context,)
    assert kwargs["error"] == "pipeline exploded"
    assert kwargs["tool_gate"] is ToolGate.BYPASSED_FOR_EVAL
    assert kwargs["seed_application"] is seed_application
    publish.assert_awaited_once()


@pytest.mark.asyncio
async def test_a_crashed_production_chat_does_not_touch_evaluation_traces():
    trace_service = MagicMock()
    pipeline = _boundary_pipeline(trace_service)
    g8e_context = G8eHttpContext(user_id="user-1", investigation_id="inv-1")

    publish = await _run_a_crashing_chat(pipeline, g8e_context)

    trace_service.finalize_crashed.assert_not_called()
    publish.assert_awaited_once()


@pytest.mark.asyncio
async def test_a_failure_to_finalize_the_crashed_trace_does_not_hide_the_original_failure_event():
    trace_service = MagicMock()
    trace_service.finalize_crashed.side_effect = OSError("disk full")
    pipeline = _boundary_pipeline(trace_service)
    g8e_context = G8eHttpContext(
        user_id="user-1", investigation_id="inv-1", evaluation_context=_evaluation_context()
    )

    publish = await _run_a_crashing_chat(pipeline, g8e_context)

    trace_service.finalize_crashed.assert_called_once()
    publish.assert_awaited_once()
    assert publish.await_args.kwargs["payload"].error == "pipeline exploded"


def _memory_barrier_fixture():
    trace_service = MagicMock()
    pipeline = _boundary_pipeline(trace_service)
    g8e_context = G8eHttpContext(user_id="user-1", evaluation_context=_evaluation_context())
    agent_call = ModelCallTelemetry(
        agent_role="sage",
        classification="scored_chain",
        model_role="primary",
        provider="G8EProvider",
        model="model-a",
        monotonic_start=1.0,
        monotonic_end=2.0,
    )
    return trace_service, pipeline, g8e_context, agent_call


@pytest.mark.asyncio
async def test_a_failing_memory_update_does_not_fail_the_scored_trace():
    trace_service, pipeline, g8e_context, agent_call = _memory_barrier_fixture()

    async def _failing_memory_task() -> None:
        raise RuntimeError("memory model unavailable")

    await pipeline._finalize_evaluation_assignment(
        g8e_context=g8e_context,
        inputs=_boundary_inputs(g8e_context),
        state=AgentStreamState(model_calls=[agent_call]),
        memory_holder={"task": asyncio.create_task(_failing_memory_task()), "model_call": None},
    )

    kwargs = trace_service.finalize.call_args.kwargs
    assert kwargs["status"] == "completed"
    assert kwargs["model_calls"] == [agent_call]


@pytest.mark.asyncio
async def test_a_timed_out_memory_update_does_not_fail_the_scored_trace(monkeypatch):
    trace_service, pipeline, g8e_context, agent_call = _memory_barrier_fixture()
    monkeypatch.setattr(
        "app.services.ai.chat_pipeline.EVALUATION_BACKGROUND_BARRIER_TIMEOUT_SECONDS", 0.01
    )

    async def _hung_memory_task() -> None:
        await asyncio.sleep(30)

    task = asyncio.create_task(_hung_memory_task())
    try:
        await pipeline._finalize_evaluation_assignment(
            g8e_context=g8e_context,
            inputs=_boundary_inputs(g8e_context),
            state=AgentStreamState(model_calls=[agent_call]),
            memory_holder={"task": task, "model_call": None},
        )
    finally:
        task.cancel()

    kwargs = trace_service.finalize.call_args.kwargs
    assert kwargs["status"] == "completed"
    assert kwargs["model_calls"] == [agent_call]
