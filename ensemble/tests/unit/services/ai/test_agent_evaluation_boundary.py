# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Provider-boundary evidence carried from the agent loop to the evaluation trace.

A scored request must prove the opportunity was real: the tool names that were
actually sent to the provider (captured where the request crosses the provider
boundary, never recomputed from the registry), which turn of the tool loop
issued each call, and whether the provider itself refused the tool declaration.
"""

from __future__ import annotations

from unittest.mock import AsyncMock, MagicMock

import pytest

from app.constants import StreamChunkFromModelType
from app.errors import ToolsNotSupportedError
from app.llm.llm_types import ToolCall
from app.models.agent import StreamChunkData, StreamChunkFromModel
from app.models.tool_results import CommandExecutionResult
from app.services.ai.agent_sse import deliver_via_sse
from tests.fakes.agent_helpers import (
    make_agent_inputs,
    make_agent_run_args,
    make_event_service,
    make_g8e_agent,
    make_gen_config,
    make_provider_chunk,
)

pytestmark = [pytest.mark.unit]

DECLARED = ["recursive_grep_search", "file_read_on_operator"]


class _BoundaryProvider:
    """Stub provider that records its tool declarations when called, as the
    governed provider does at the dispatch boundary."""

    def __init__(self, turns: list[list], declared: list[str], failure: Exception | None = None):
        self._turns = turns
        self._declared = declared
        self._failure = failure
        self._call = 0
        self.declared_tool_names: list[str] | None = None

    def clear_input_artifact_hash(self) -> None:
        self.input_artifact_hash = ""

    def clear_declared_tools(self) -> None:
        self.declared_tool_names = None

    def set_g8e_context(self, context) -> None:
        pass

    def set_provider_retry_count(self, retry_count: int) -> None:
        pass

    def generate_content_stream_primary(self, **kwargs):
        index = self._call
        self._call += 1
        self.declared_tool_names = list(self._declared)

        async def _gen():
            if self._failure is not None:
                raise self._failure
            for chunk in self._turns[index]:
                yield chunk

        return _gen()


def _tool_turn(call_id: str) -> list:
    return [
        make_provider_chunk(
            tool_calls=[
                ToolCall(
                    name="query_investigation_context",
                    args={"data_type": "history_trail"},
                    id=call_id,
                )
            ],
            finish_reason="STOP",
        )
    ]


def _text_turn(text: str = "done") -> list:
    return [make_provider_chunk(text=text), make_provider_chunk(finish_reason="STOP")]


def _agent_and_inputs():
    tool_executor = MagicMock()
    tool_executor.execute_tool_call = AsyncMock(
        return_value=CommandExecutionResult(success=True, output="ok")
    )
    agent = make_g8e_agent(fn_handler=tool_executor)
    inputs = make_agent_inputs()
    inputs.generation_config = make_gen_config()
    inputs.model_to_use = "test-model"
    return agent, inputs


@pytest.mark.asyncio(loop_scope="session")
class TestToolLoopBoundaryEvidence:
    async def test_tool_chunks_carry_the_loop_turn_that_issued_them(self):
        agent, inputs = _agent_and_inputs()
        provider = _BoundaryProvider(
            [_tool_turn("call-1"), _tool_turn("call-2"), _text_turn()], DECLARED
        )

        chunks = [
            chunk
            async for chunk in agent._stream_with_tool_loop(
                inputs=inputs, event_service=make_event_service(), llm_provider=provider
            )
        ]

        tool_chunks = [
            chunk
            for chunk in chunks
            if chunk.type
            in (StreamChunkFromModelType.TOOL_CALL, StreamChunkFromModelType.TOOL_RESULT)
        ]
        assert [(chunk.type, chunk.data.loop_turn) for chunk in tool_chunks] == [
            (StreamChunkFromModelType.TOOL_CALL, 1),
            (StreamChunkFromModelType.TOOL_RESULT, 1),
            (StreamChunkFromModelType.TOOL_CALL, 2),
            (StreamChunkFromModelType.TOOL_RESULT, 2),
        ]

    async def test_every_agent_call_records_the_tools_sent_to_the_provider(self):
        agent, inputs = _agent_and_inputs()
        provider = _BoundaryProvider([_tool_turn("call-1"), _text_turn()], DECLARED)

        chunks = [
            chunk
            async for chunk in agent._stream_with_tool_loop(
                inputs=inputs, event_service=make_event_service(), llm_provider=provider
            )
        ]

        complete = [c for c in chunks if c.type == StreamChunkFromModelType.COMPLETE]
        assert len(complete) == 1
        agent_calls = complete[0].data.model_calls
        assert len(agent_calls) == 2
        assert [call.tools_declared for call in agent_calls] == [DECLARED, DECLARED]

    async def test_complete_chunk_reports_unknown_when_the_provider_records_nothing(self):
        agent, inputs = _agent_and_inputs()
        provider = MagicMock()

        def stream(**kwargs):
            async def _gen():
                for chunk in _text_turn():
                    yield chunk

            return _gen()

        provider.generate_content_stream_primary = stream

        chunks = [
            chunk
            async for chunk in agent._stream_with_tool_loop(
                inputs=inputs, event_service=make_event_service(), llm_provider=provider
            )
        ]

        complete = [c for c in chunks if c.type == StreamChunkFromModelType.COMPLETE]
        assert [call.tools_declared for call in complete[0].data.model_calls] == [None]

    async def test_error_chunk_flags_a_provider_tool_declaration_rejection(self):
        agent, inputs = _agent_and_inputs()
        provider = _BoundaryProvider(
            [],
            DECLARED,
            failure=ToolsNotSupportedError(
                "Provider rejected the tool declaration: inference: requested capability unsupported",
                model="test-model",
                service_name="g8e",
            ),
        )

        chunks = [
            chunk
            async for chunk in agent.stream_response(
                inputs=inputs, event_service=make_event_service(), llm_provider=provider
            )
        ]

        assert [c.type for c in chunks] == [StreamChunkFromModelType.ERROR]
        error = chunks[0].data
        assert error.provider_tool_rejection is True
        assert "requested capability unsupported" in (error.error or "")
        # The refused call is still on the record, with the tools it declared.
        assert [(call.succeeded, call.tools_declared) for call in error.model_calls] == [
            (False, DECLARED)
        ]

    async def test_error_chunk_for_any_other_failure_is_not_a_tool_rejection(self):
        agent, inputs = _agent_and_inputs()
        provider = _BoundaryProvider([], DECLARED, failure=PermissionError("Invalid API key"))

        chunks = [
            chunk
            async for chunk in agent.stream_response(
                inputs=inputs, event_service=make_event_service(), llm_provider=provider
            )
        ]

        assert chunks[0].type == StreamChunkFromModelType.ERROR
        assert not chunks[0].data.provider_tool_rejection
        assert [call.tools_declared for call in chunks[0].data.model_calls] == [DECLARED]


async def _stream(*chunks: StreamChunkFromModel):
    for chunk in chunks:
        yield chunk


@pytest.mark.asyncio
class TestStreamStateBoundaryEvidence:
    async def test_complete_chunk_records_no_rejection(self):
        inputs, state = make_agent_run_args()

        await deliver_via_sse(
            stream=_stream(
                StreamChunkFromModel(
                    type=StreamChunkFromModelType.COMPLETE,
                    data=StreamChunkData(finish_reason="STOP"),
                )
            ),
            inputs=inputs,
            state=state,
            event_service=make_event_service(),
        )

        assert state.provider_tool_rejection is None

    async def test_tool_rejection_error_chunk_records_the_rejected_model_and_reason(self):
        inputs, state = make_agent_run_args(model_to_use="qwen3.5:4b")

        await deliver_via_sse(
            stream=_stream(
                StreamChunkFromModel(
                    type=StreamChunkFromModelType.ERROR,
                    data=StreamChunkData(
                        error="Provider rejected the tool declaration: capability unsupported",
                        provider_tool_rejection=True,
                    ),
                )
            ),
            inputs=inputs,
            state=state,
            event_service=make_event_service(),
        )

        assert state.stream_failed is True
        assert state.provider_tool_rejection is not None
        assert state.provider_tool_rejection.model == "qwen3.5:4b"
        assert "capability unsupported" in state.provider_tool_rejection.reason

    async def test_plain_error_chunk_is_not_recorded_as_a_rejection(self):
        inputs, state = make_agent_run_args()

        await deliver_via_sse(
            stream=_stream(
                StreamChunkFromModel(
                    type=StreamChunkFromModelType.ERROR,
                    data=StreamChunkData(error="backend unavailable"),
                )
            ),
            inputs=inputs,
            state=state,
            event_service=make_event_service(),
        )

        assert state.stream_failed is True
        assert state.provider_tool_rejection is None


class TestAgentRoleAttribution:
    """Every scored model call reports a persona the grader recognises."""

    def _inputs(self, *, scored: bool, active_agent):
        from g8e.models.internal_api import EvaluationInferenceContext, InferenceModelVariant

        from tests.fakes.agent_helpers import make_agent_inputs

        inputs = make_agent_inputs()
        inputs.active_agent = active_agent
        if scored:
            inputs.g8e_context.evaluation_context = EvaluationInferenceContext(
                campaign_id="c",
                run_id="r",
                assignment_id="a",
                evaluation_attempt_id="e",
                scenario_id="s",
                model_registry_digest="d" * 64,
                model_registry=[InferenceModelVariant(model="m", digest="a" * 64)],
                target_operator_session_id="op",
            )
        return inputs

    def test_reports_the_active_agent_persona(self):
        from app.constants import ReasoningAgent
        from app.services.ai.agent import _agent_role_for_telemetry

        inputs = self._inputs(scored=True, active_agent=ReasoningAgent.DASH)
        assert _agent_role_for_telemetry(inputs) == "dash"

    def test_a_scored_request_without_an_active_agent_fails_loudly(self):
        from app.errors import ValidationError
        from app.services.ai.agent import _agent_role_for_telemetry

        with pytest.raises(ValidationError):
            _agent_role_for_telemetry(self._inputs(scored=True, active_agent=None))

    def test_an_unscored_request_without_an_active_agent_fails_loudly(self):
        from app.errors import ValidationError
        from app.services.ai.agent import _agent_role_for_telemetry

        with pytest.raises(ValidationError):
            _agent_role_for_telemetry(self._inputs(scored=False, active_agent=None))
