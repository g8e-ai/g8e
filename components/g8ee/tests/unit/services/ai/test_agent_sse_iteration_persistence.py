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

"""Unit tests for ``deliver_via_sse`` per-iteration text persistence callback.

Regression coverage for the bug where intermediate AI text produced between
tool calls was discarded by ``agent_streaming_context.response_text = ""``
on every TOOL_RESULT chunk, leaving the conversation_history with no AI
narration on restore.

These tests pin the contract that ``on_iteration_text``:
  - is awaited exactly once per tool iteration with the accumulated pre-tool
    text,
  - is awaited *before* the response_text buffer is cleared,
  - is not awaited when the iteration produced no text,
  - is not invoked at all when the stream contains no TOOL_RESULT chunks
    (final text is persisted by ``_persist_ai_response``),
  - does not abort the live SSE stream when the persistence callback raises.
"""

from __future__ import annotations

import pytest

from app.constants import StreamChunkFromModelType
from app.models.agent import StreamChunkData, StreamChunkFromModel
from app.services.ai.agent_sse import deliver_via_sse
from tests.fakes.agent_helpers import (
    make_agent_run_args,
    make_g8ed_event_service,
)

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


async def _stream(*chunks: StreamChunkFromModel):
    for chunk in chunks:
        yield chunk


async def test_callback_invoked_once_per_tool_iteration_with_accumulated_text():
    """TEXT before TOOL_RESULT should be flushed via the callback, then cleared."""
    inputs, state = make_agent_run_args(
        case_id="case-iter-1",
        investigation_id="inv-iter-1",
        web_session_id="web-iter-1",
        user_id="user-iter-1",
    )
    event_svc = make_g8ed_event_service()
    persisted: list[str] = []

    async def _on_iteration_text(text: str) -> None:
        persisted.append(text)

    await deliver_via_sse(
        stream=_stream(
            _text("Looking at the logs to understand the failure. "),
            _text("Going to run a quick check first."),
            _tool_call(),
            _tool_result(),
            _text("Now interpreting the result."),
            _complete(),
        ),
        inputs=inputs,
        state=state,
        g8ed_event_service=event_svc,
        on_iteration_text=_on_iteration_text,
    )

    assert persisted == [
        "Looking at the logs to understand the failure. Going to run a quick check first.",
    ]
    # response_text cleared after TOOL_RESULT, then accumulated again from the
    # post-tool TEXT chunk; that final segment is left for _persist_ai_response.
    assert state.response_text == "Now interpreting the result."


async def test_callback_invoked_once_per_iteration_across_multiple_tool_loops():
    """Each TOOL_RESULT flushes its own iteration's text."""
    inputs, state = make_agent_run_args(
        case_id="case-iter-2",
        investigation_id="inv-iter-2",
        web_session_id="web-iter-2",
        user_id="user-iter-2",
    )
    event_svc = make_g8ed_event_service()
    persisted: list[str] = []

    async def _on_iteration_text(text: str) -> None:
        persisted.append(text)

    await deliver_via_sse(
        stream=_stream(
            _text("Iteration 1 commentary."),
            _tool_call(exec_id="exec-1"),
            _tool_result(exec_id="exec-1"),
            _text("Iteration 2 commentary."),
            _tool_call(exec_id="exec-2"),
            _tool_result(exec_id="exec-2"),
            _text("Final wrap-up."),
            _complete(),
        ),
        inputs=inputs,
        state=state,
        g8ed_event_service=event_svc,
        on_iteration_text=_on_iteration_text,
    )

    assert persisted == ["Iteration 1 commentary.", "Iteration 2 commentary."]
    assert state.response_text == "Final wrap-up."


async def test_callback_skipped_when_iteration_text_is_whitespace_only():
    """Tool calls with no preceding narration must not produce empty AI rows."""
    inputs, state = make_agent_run_args(
        case_id="case-iter-3",
        investigation_id="inv-iter-3",
        web_session_id="web-iter-3",
        user_id="user-iter-3",
    )
    event_svc = make_g8ed_event_service()
    persisted: list[str] = []

    async def _on_iteration_text(text: str) -> None:
        persisted.append(text)

    await deliver_via_sse(
        stream=_stream(
            # Whitespace-only pre-tool text — must be dropped.
            _text("   \n  "),
            _tool_call(),
            _tool_result(),
            _text("Real narration after the tool."),
            _complete(),
        ),
        inputs=inputs,
        state=state,
        g8ed_event_service=event_svc,
        on_iteration_text=_on_iteration_text,
    )

    assert persisted == []
    assert state.response_text == "Real narration after the tool."


async def test_callback_not_invoked_when_no_tool_results():
    """Single-turn streams (no tools) leave persistence to _persist_ai_response."""
    inputs, state = make_agent_run_args(
        case_id="case-iter-4",
        investigation_id="inv-iter-4",
        web_session_id="web-iter-4",
        user_id="user-iter-4",
    )
    event_svc = make_g8ed_event_service()
    persisted: list[str] = []

    async def _on_iteration_text(text: str) -> None:
        persisted.append(text)

    await deliver_via_sse(
        stream=_stream(
            _text("Here is your direct answer."),
            _complete(),
        ),
        inputs=inputs,
        state=state,
        g8ed_event_service=event_svc,
        on_iteration_text=_on_iteration_text,
    )

    assert persisted == []
    assert state.response_text == "Here is your direct answer."


async def test_callback_failure_does_not_abort_stream():
    """Persistence errors must be swallowed so the live SSE stream continues."""
    inputs, state = make_agent_run_args(
        case_id="case-iter-5",
        investigation_id="inv-iter-5",
        web_session_id="web-iter-5",
        user_id="user-iter-5",
    )
    event_svc = make_g8ed_event_service()
    invocations: list[str] = []

    async def _on_iteration_text(text: str) -> None:
        invocations.append(text)
        raise RuntimeError("simulated persistence failure")

    await deliver_via_sse(
        stream=_stream(
            _text("Pre-tool reasoning."),
            _tool_call(),
            _tool_result(),
            _text("Post-tool wrap-up."),
            _complete(),
        ),
        inputs=inputs,
        state=state,
        g8ed_event_service=event_svc,
        on_iteration_text=_on_iteration_text,
    )

    # Callback was attempted exactly once with the pre-tool text...
    assert invocations == ["Pre-tool reasoning."]
    # ...the buffer was still cleared after the failed attempt...
    # ...and the rest of the stream was processed normally.
    assert state.response_text == "Post-tool wrap-up."


async def test_omitting_callback_preserves_legacy_behavior():
    """The new parameter is optional — passing nothing must not break the flow."""
    inputs, state = make_agent_run_args(
        case_id="case-iter-6",
        investigation_id="inv-iter-6",
        web_session_id="web-iter-6",
        user_id="user-iter-6",
    )
    event_svc = make_g8ed_event_service()

    await deliver_via_sse(
        stream=_stream(
            _text("Some narration."),
            _tool_call(),
            _tool_result(),
            _text("Final segment."),
            _complete(),
        ),
        inputs=inputs,
        state=state,
        g8ed_event_service=event_svc,
    )

    # Without the callback the intermediate text is lost (legacy behavior),
    # but the stream still completes and the final segment is preserved.
    assert state.response_text == "Final segment."
