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

import pytest

from app.constants import ReasoningAgent, StreamChunkFromModelType
from app.models.agent import StreamChunkData, StreamChunkFromModel
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


def _agent_state_statuses(event_svc) -> list[str]:
    return [req.status for req in event_svc.agent_state_requests]


def _run_state_statuses(event_svc) -> list[str]:
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
    assert statuses[-1] == "completed"
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
    assert "failed" in statuses


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
    assert statuses[-1] == "completed"


async def test_cancellation_emits_idle_agent_state():
    import asyncio

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
        raise asyncio.CancelledError()

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
        web_session_id=None,
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
    from app.models.personas import get_persona

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
