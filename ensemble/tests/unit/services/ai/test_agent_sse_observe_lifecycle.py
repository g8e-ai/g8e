# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Unit tests for observe producer agent-state and run-state projections
emitted by ``deliver_via_sse`` during the chat and tool lifecycle.

Asserts exact transition sequences for start, completion, provider error,
exception, cancellation, universal tool request/result, targetless flow,
and producer failure. The investigation run is kept non-terminal during
ordinary chat completion.
"""

from __future__ import annotations

import asyncio
from itertools import pairwise

import pytest
from g8e.models.events import AIToolLifecyclePayload

from app.constants import (
    EventType,
    OperatorToolName,
    ReasoningAgent,
    StreamChunkFromModelType,
    ToolCallStatus,
)
from app.models.agent import StreamChunkData, StreamChunkFromModel
from app.models.events import SessionEvent
from app.models.personas import get_persona
from app.models.tool_results import CommandExecutionResult, InvestigationContextResult
from app.services.ai.agent_sse import deliver_via_sse
from tests.fakes.agent_helpers import make_agent_run_args
from tests.fakes.fake_event_service import FakeEventService

pytestmark = [pytest.mark.unit, pytest.mark.asyncio]


def _text(content: str) -> StreamChunkFromModel:
    return StreamChunkFromModel(
        type=StreamChunkFromModelType.TEXT,
        data=StreamChunkData(content=content),
    )


def _tool_call(name: str = "g8e.run.command", exec_id: str = "exec-1") -> StreamChunkFromModel:
    return StreamChunkFromModel(
        type=StreamChunkFromModelType.TOOL_CALL,
        data=StreamChunkData(tool_name=name, execution_id=exec_id),
    )


def _tool_result(name: str = "g8e.run.command", exec_id: str = "exec-1") -> StreamChunkFromModel:
    return StreamChunkFromModel(
        type=StreamChunkFromModelType.TOOL_RESULT,
        data=StreamChunkData(tool_name=name, execution_id=exec_id, success=True),
    )


def _complete(reason: str = "STOP") -> StreamChunkFromModel:
    return StreamChunkFromModel(
        type=StreamChunkFromModelType.COMPLETE,
        data=StreamChunkData(finish_reason=reason),
    )


def _error(message: str = "provider error") -> StreamChunkFromModel:
    return StreamChunkFromModel(
        type=StreamChunkFromModelType.ERROR,
        data=StreamChunkData(error=message),
    )


async def _stream(*chunks: StreamChunkFromModel):
    for chunk in chunks:
        yield chunk


def _agent_state_statuses(event_svc: FakeEventService) -> list[str]:
    return [req.status for req in event_svc.agent_state_requests]


def _run_state_statuses(event_svc: FakeEventService) -> list[str]:
    return [req.status for req in event_svc.run_state_requests]


async def test_start_emits_running_agent_and_run_state():
    inputs, state = make_agent_run_args(
        case_id="case-obs-1",
        investigation_id="inv-obs-1",
        web_session_id="web-obs-1",
        user_id="user-obs-1",
        active_agent=ReasoningAgent.SAGE,
    )
    event_svc = FakeEventService()

    await deliver_via_sse(
        stream=_stream(_text("Answer."), _complete()),
        inputs=inputs,
        state=state,
        event_service=event_svc,
    )

    statuses = _agent_state_statuses(event_svc)
    assert statuses[0] == "running"
    run_statuses = _run_state_statuses(event_svc)
    assert run_statuses[0] == "running"


async def test_completion_emits_completed_agent_and_running_run():
    inputs, state = make_agent_run_args(
        case_id="case-obs-2",
        investigation_id="inv-obs-2",
        web_session_id="web-obs-2",
        user_id="user-obs-2",
        active_agent=ReasoningAgent.SAGE,
    )
    event_svc = FakeEventService()

    await deliver_via_sse(
        stream=_stream(_text("Answer."), _complete()),
        inputs=inputs,
        state=state,
        event_service=event_svc,
    )

    statuses = _agent_state_statuses(event_svc)
    # The Gateway rejects running after a terminal state, so the persona
    # resets to idle once the run ends.
    assert statuses[-2:] == ["completed", "idle"]
    run_statuses = _run_state_statuses(event_svc)
    assert run_statuses[-1] == "running"


async def test_provider_error_emits_failed_agent_state():
    inputs, state = make_agent_run_args(
        case_id="case-obs-3",
        investigation_id="inv-obs-3",
        web_session_id="web-obs-3",
        user_id="user-obs-3",
        active_agent=ReasoningAgent.SAGE,
    )
    event_svc = FakeEventService()

    await deliver_via_sse(
        stream=_stream(_text("Partial."), _error("model timeout")),
        inputs=inputs,
        state=state,
        event_service=event_svc,
    )

    statuses = _agent_state_statuses(event_svc)
    assert statuses[-2:] == ["failed", "idle"]


async def test_universal_tool_call_emits_waiting_then_running():
    inputs, state = make_agent_run_args(
        case_id="case-obs-4",
        investigation_id="inv-obs-4",
        web_session_id="web-obs-4",
        user_id="user-obs-4",
        active_agent=ReasoningAgent.SAGE,
    )
    event_svc = FakeEventService()

    await deliver_via_sse(
        stream=_stream(
            _text("Checking."),
            _tool_call(name="query_investigation_context", exec_id="exec-1"),
            _tool_result(name="query_investigation_context", exec_id="exec-1"),
            _text("Done."),
            _complete(),
        ),
        inputs=inputs,
        state=state,
        event_service=event_svc,
    )

    statuses = _agent_state_statuses(event_svc)
    assert "waiting" in statuses
    assert "running" in statuses
    assert statuses[-2:] == ["completed", "idle"]


async def test_second_run_after_terminal_state_follows_gateway_transitions():
    """Two consecutive runs of one persona never ask the Gateway for running
    straight after completed or failed (observe_producer.go agentTransitions)."""
    inputs, state = make_agent_run_args(
        case_id="case-obs-8",
        investigation_id="inv-obs-8",
        web_session_id="web-obs-8",
        user_id="user-obs-8",
        active_agent=ReasoningAgent.SAGE,
    )
    event_svc = FakeEventService()

    await deliver_via_sse(
        stream=_stream(_text("Partial."), _error("model timeout")),
        inputs=inputs,
        state=state,
        event_service=event_svc,
    )
    inputs, state = make_agent_run_args(
        case_id="case-obs-8",
        investigation_id="inv-obs-8",
        web_session_id="web-obs-8",
        user_id="user-obs-8",
        active_agent=ReasoningAgent.SAGE,
    )
    await deliver_via_sse(
        stream=_stream(_text("Answer."), _complete()),
        inputs=inputs,
        state=state,
        event_service=event_svc,
    )

    statuses = _agent_state_statuses(event_svc)
    for previous, current in pairwise(statuses):
        if previous in ("completed", "failed"):
            assert current in (previous, "idle", "offline"), statuses


async def test_cancellation_emits_idle_agent_state():

    inputs, state = make_agent_run_args(
        case_id="case-obs-5",
        investigation_id="inv-obs-5",
        web_session_id="web-obs-5",
        user_id="user-obs-5",
        active_agent=ReasoningAgent.SAGE,
    )
    event_svc = FakeEventService()

    async def _cancel_stream():
        yield _text("Working")
        raise asyncio.CancelledError

    with pytest.raises(asyncio.CancelledError):
        await deliver_via_sse(
            stream=_cancel_stream(),
            inputs=inputs,
            state=state,
            event_service=event_svc,
        )

    statuses = _agent_state_statuses(event_svc)
    assert "idle" in statuses


async def test_targetless_flow_skips_projections():
    inputs, state = make_agent_run_args(
        case_id="case-obs-6",
        investigation_id="inv-obs-6",
        web_session_id="web-obs-6",
        user_id="user-obs-6",
        active_agent=ReasoningAgent.SAGE,
    )
    event_svc = FakeEventService()

    await deliver_via_sse(
        stream=_stream(_text("No SSE."), _complete()),
        inputs=inputs,
        state=state,
        event_service=event_svc,
    )

    assert len(event_svc.agent_state_requests) == 0
    assert len(event_svc.run_state_requests) == 0


async def test_producer_failure_does_not_abort_stream():
    inputs, state = make_agent_run_args(
        case_id="case-obs-7",
        investigation_id="inv-obs-7",
        web_session_id="web-obs-7",
        user_id="user-obs-7",
        active_agent=ReasoningAgent.SAGE,
    )
    event_svc = FakeEventService()

    call_count = 0

    async def _failing_push(request):
        nonlocal call_count
        call_count += 1
        raise RuntimeError("producer unavailable")

    event_svc.publish_agent_state = _failing_push
    event_svc.publish_run_state = _failing_push

    await deliver_via_sse(
        stream=_stream(_text("Survives."), _complete()),
        inputs=inputs,
        state=state,
        event_service=event_svc,
    )

    assert call_count > 0
    assert state.response_text == "Survives."


async def test_agent_state_request_carries_registry_owned_display_name():

    inputs, state = make_agent_run_args(
        case_id="case-obs-8",
        investigation_id="inv-obs-8",
        web_session_id="web-obs-8",
        user_id="user-obs-8",
        active_agent=ReasoningAgent.SAGE,
    )
    event_svc = FakeEventService()

    await deliver_via_sse(
        stream=_stream(_text("Answer."), _complete()),
        inputs=inputs,
        state=state,
        event_service=event_svc,
    )

    assert len(event_svc.agent_state_requests) > 0
    req = event_svc.agent_state_requests[0]
    sage = get_persona("sage")
    assert req.display_name == sage.display_name
    assert req.role == sage.role
    assert req.agent_id == "user-obs-8:sage"


async def test_run_state_request_uses_investigation_case_title():
    inputs, state = make_agent_run_args(
        case_id="case-obs-9",
        investigation_id="inv-obs-9",
        web_session_id="web-obs-9",
        user_id="user-obs-9",
        active_agent=ReasoningAgent.SAGE,
    )
    if inputs.investigation:
        inputs.investigation.case_title = "My Investigation"
    event_svc = FakeEventService()

    await deliver_via_sse(
        stream=_stream(_text("Answer."), _complete()),
        inputs=inputs,
        state=state,
        event_service=event_svc,
    )

    assert len(event_svc.run_state_requests) > 0
    req = event_svc.run_state_requests[0]
    assert req.run_id == "inv-obs-9"
    assert req.run_kind == "investigation"
    assert req.display_name == "My Investigation"


async def test_no_active_agent_skips_agent_projection():
    inputs, state = make_agent_run_args(
        case_id="case-obs-10",
        investigation_id="inv-obs-10",
        web_session_id="web-obs-10",
        user_id="user-obs-10",
        active_agent=None,
    )
    event_svc = FakeEventService()

    await deliver_via_sse(
        stream=_stream(_text("Answer."), _complete()),
        inputs=inputs,
        state=state,
        event_service=event_svc,
    )

    assert len(event_svc.agent_state_requests) == 0
    assert len(event_svc.run_state_requests) > 0


async def test_operator_tool_call_lifecycle_completed():
    inputs, state = make_agent_run_args(
        case_id="case-obs-11",
        investigation_id="inv-obs-11",
        web_session_id="web-obs-11",
        user_id="user-obs-11",
        active_agent=ReasoningAgent.SAGE,
    )
    event_svc = FakeEventService()

    await deliver_via_sse(
        stream=_stream(
            _text("Running grep."),
            _tool_call(name="recursive_grep_search", exec_id="exec-grep-1"),
            _tool_result(name="recursive_grep_search", exec_id="exec-grep-1"),
            _text("Found match."),
            _complete(),
        ),
        inputs=inputs,
        state=state,
        event_service=event_svc,
    )

    event_types = [e.event_type for e in event_svc.published]
    assert EventType.OPERATOR_COMMAND_STARTED in event_types
    assert EventType.OPERATOR_COMMAND_COMPLETED in event_types

    completed_events = [
        e for e in event_svc.published if e.event_type == EventType.OPERATOR_COMMAND_COMPLETED
    ]
    assert len(completed_events) == 1
    assert isinstance(completed_events[0], SessionEvent)
    assert isinstance(completed_events[0].payload, AIToolLifecyclePayload)
    assert completed_events[0].payload.status == ToolCallStatus.COMPLETED


async def test_operator_tool_call_lifecycle_failed():
    inputs, state = make_agent_run_args(
        case_id="case-obs-12",
        investigation_id="inv-obs-12",
        web_session_id="web-obs-12",
        user_id="user-obs-12",
        active_agent=ReasoningAgent.SAGE,
    )
    event_svc = FakeEventService()

    await deliver_via_sse(
        stream=_stream(
            _text("Running grep."),
            _tool_call(name="recursive_grep_search", exec_id="exec-grep-2"),
            StreamChunkFromModel(
                type=StreamChunkFromModelType.TOOL_RESULT,
                data=StreamChunkData(
                    tool_name="recursive_grep_search",
                    execution_id="exec-grep-2",
                    error="command failed with exit code 1",
                    success=False,
                ),
            ),
            _text("Error handling."),
            _complete(),
        ),
        inputs=inputs,
        state=state,
        event_service=event_svc,
    )

    event_types = [e.event_type for e in event_svc.published]
    assert EventType.OPERATOR_COMMAND_STARTED in event_types
    assert EventType.OPERATOR_COMMAND_FAILED in event_types

    failed_events = [
        e for e in event_svc.published if e.event_type == EventType.OPERATOR_COMMAND_FAILED
    ]
    assert len(failed_events) == 1
    assert isinstance(failed_events[0].payload, AIToolLifecyclePayload)
    assert failed_events[0].payload.status == ToolCallStatus.FAILED


async def test_operator_tool_call_lifecycle_typed_result():
    inputs, state = make_agent_run_args(
        case_id="case-obs-13",
        investigation_id="inv-obs-13",
        web_session_id="web-obs-13",
        user_id="user-obs-13",
        active_agent=ReasoningAgent.SAGE,
    )
    event_svc = FakeEventService()

    await deliver_via_sse(
        stream=_stream(
            _text("Executing command."),
            _tool_call(name="run_commands_with_operator", exec_id="exec-cmd-1"),
            StreamChunkFromModel(
                type=StreamChunkFromModelType.TOOL_RESULT,
                data=StreamChunkData(
                    tool_name="run_commands_with_operator",
                    execution_id="exec-cmd-1",
                    result=CommandExecutionResult(
                        success=True,
                        output="system status ok",
                    ),
                    success=True,
                    status=ToolCallStatus.COMPLETED,
                ),
            ),
            _text("Done."),
            _complete(),
        ),
        inputs=inputs,
        state=state,
        event_service=event_svc,
    )

    completed_events = [
        e for e in event_svc.published if e.event_type == EventType.OPERATOR_COMMAND_COMPLETED
    ]
    assert len(completed_events) == 1
    assert isinstance(completed_events[0].payload, AIToolLifecyclePayload)
    assert completed_events[0].payload.status == ToolCallStatus.COMPLETED
    assert completed_events[0].payload.content == "system status ok"


async def test_universal_tool_call_lifecycle_typed_result():
    inputs, state = make_agent_run_args(
        case_id="case-obs-14",
        investigation_id="inv-obs-14",
        web_session_id="web-obs-14",
        user_id="user-obs-14",
        active_agent=ReasoningAgent.SAGE,
    )
    event_svc = FakeEventService()

    await deliver_via_sse(
        stream=_stream(
            _text("Checking investigation context."),
            _tool_call(name=OperatorToolName.QUERY_INVESTIGATION_CONTEXT, exec_id="exec-ctx-1"),
            StreamChunkFromModel(
                type=StreamChunkFromModelType.TOOL_RESULT,
                data=StreamChunkData(
                    tool_name=OperatorToolName.QUERY_INVESTIGATION_CONTEXT,
                    execution_id="exec-ctx-1",
                    result=InvestigationContextResult(
                        success=True,
                        data="enriched context data",
                    ),
                    success=True,
                    status=ToolCallStatus.COMPLETED,
                ),
            ),
            _text("Context reviewed."),
            _complete(),
        ),
        inputs=inputs,
        state=state,
        event_service=event_svc,
    )

    completed_events = [
        e
        for e in event_svc.published
        if e.event_type == EventType.AI_LLM_TOOL_G8E_INVESTIGATION_QUERY_COMPLETED
    ]
    assert len(completed_events) == 1
    assert isinstance(completed_events[0].payload, AIToolLifecyclePayload)
    assert completed_events[0].payload.status == ToolCallStatus.COMPLETED
    assert completed_events[0].payload.content == "enriched context data"


async def test_operator_tool_call_lifecycle_none_execution_id():
    inputs, state = make_agent_run_args(
        case_id="case-obs-15",
        investigation_id="inv-obs-15",
        web_session_id="web-obs-15",
        user_id="user-obs-15",
        active_agent=ReasoningAgent.SAGE,
    )
    event_svc = FakeEventService()

    await deliver_via_sse(
        stream=_stream(
            _text("Attempting tool without execution_id."),
            StreamChunkFromModel(
                type=StreamChunkFromModelType.TOOL_CALL,
                data=StreamChunkData(
                    tool_name="run_commands_with_operator",
                    execution_id=None,
                ),
            ),
            StreamChunkFromModel(
                type=StreamChunkFromModelType.TOOL_RESULT,
                data=StreamChunkData(
                    tool_name="run_commands_with_operator",
                    execution_id=None,
                    error="Tribunal generation failed",
                    success=False,
                    status=ToolCallStatus.FAILED,
                ),
            ),
            _text("Handled."),
            _complete(),
        ),
        inputs=inputs,
        state=state,
        event_service=event_svc,
    )

    started_events = [
        e for e in event_svc.published if e.event_type == EventType.OPERATOR_COMMAND_STARTED
    ]
    failed_events = [
        e for e in event_svc.published if e.event_type == EventType.OPERATOR_COMMAND_FAILED
    ]
    assert len(started_events) == 1
    assert isinstance(started_events[0].payload, AIToolLifecyclePayload)
    assert isinstance(started_events[0].payload.execution_id, str)
    assert len(started_events[0].payload.execution_id) > 0

    assert len(failed_events) == 1
    assert isinstance(failed_events[0].payload, AIToolLifecyclePayload)
    assert isinstance(failed_events[0].payload.execution_id, str)
    assert len(failed_events[0].payload.execution_id) > 0
    assert failed_events[0].payload.status == ToolCallStatus.FAILED
