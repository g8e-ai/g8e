# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Fast responders retain tool declarations and recover failed model turns."""

from types import SimpleNamespace
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from app.constants import (
    AGENT_MAX_RETRIES,
    AgentMode,
    EventType,
    ReasoningAgent,
    StreamChunkFromModelType,
)
from app.llm.llm_types import ToolDeclaration, ToolGroup
from app.llm.providers.open_ai import OpenAIProvider
from app.models.model_configs import UNKNOWN_MODEL_CONFIG, get_model_config
from app.models.tool_results import CommandExecutionResult
from app.services.ai.request_builder import AIRequestBuilder
from tests.fakes.agent_helpers import (
    make_agent_inputs,
    make_agent_stream_state,
    make_event_service,
    make_g8e_agent,
)
from tests.fakes.tool_helpers import create_tool_service_fake

pytestmark = [pytest.mark.unit, pytest.mark.asyncio]


def _response(*, calls=(), text=None, finish_reason="stop"):
    return SimpleNamespace(
        choices=[
            SimpleNamespace(
                message=SimpleNamespace(content=text, tool_calls=list(calls)),
                finish_reason=finish_reason,
            )
        ],
        usage=None,
    )


def _call(call_id):
    return SimpleNamespace(
        id=call_id,
        function=SimpleNamespace(name="get_command_constraints", arguments="{}"),
    )


def _setup(responses, active_agent=ReasoningAgent.DASH):
    with patch("app.llm.providers.open_ai.AsyncOpenAI"):
        provider = OpenAIProvider(endpoint="http://test/v1", api_key="test")
    create = AsyncMock(side_effect=responses)
    provider._client.chat.completions.create = create
    executor = MagicMock()
    executor.execute_tool_call = AsyncMock(
        return_value=CommandExecutionResult(success=True, output="ok")
    )
    agent = make_g8e_agent(fn_handler=executor)
    inputs = make_agent_inputs(active_agent=active_agent)
    assert inputs.generation_config is not None
    inputs.generation_config.tools = [
        ToolGroup(
            tools=[
                ToolDeclaration(name="get_command_constraints", description="Get constraints"),
            ]
        )
    ]
    return agent, inputs, provider, executor, create


async def test_catalog_model_name_retains_production_tools_at_provider_boundary():

    model = "google/gemini-3-flash-preview"
    agent, inputs, provider, _, create = _setup([_response(text="Done.")])
    inputs.model_to_use = model
    inputs.generation_config = AIRequestBuilder(
        tool_executor=create_tool_service_fake(auto_approve=True)
    ).get_generation_config(
        system_instructions="Check RAM on the embedded operator.",
        settings=inputs.request_settings,
        agent_mode=AgentMode.G8E_BOUND,
        model_override=model,
    )

    chunks = [
        chunk
        async for chunk in agent.stream_response(
            inputs=inputs, llm_provider=provider, event_service=make_event_service()
        )
    ]

    assert chunks[-1].type == StreamChunkFromModelType.COMPLETE
    kwargs = create.call_args.kwargs
    assert kwargs["model"] == model
    names = [tool["function"]["name"] for tool in kwargs["tools"]]
    assert "get_command_constraints" in names
    assert "run_commands_with_operator" in names


@pytest.mark.parametrize(
    "model",
    [
        "google/unregistered",
        "custom/gemini-3-flash-preview",
        "openai/gemini-3-flash-preview",
    ],
)
async def test_unknown_catalog_names_still_withhold_tools(model):
    assert get_model_config(model) is UNKNOWN_MODEL_CONFIG


@pytest.mark.parametrize("active_agent", [ReasoningAgent.DASH, ReasoningAgent.SAGE])
async def test_selected_model_receives_tools_and_valid_parallel_history(active_agent):
    agent, inputs, provider, executor, create = _setup(
        [
            _response(calls=[_call("call-1"), _call("call-2")], finish_reason="tool_calls"),
            _response(text="RAM checked."),
        ],
        active_agent,
    )

    chunks = [
        chunk
        async for chunk in agent.stream_response(
            inputs=inputs,
            llm_provider=provider,
            event_service=make_event_service(),
        )
    ]

    assert chunks[-1].type == StreamChunkFromModelType.COMPLETE
    assert executor.execute_tool_call.await_count == 2
    first, second = create.call_args_list
    assert first.kwargs["model"] == inputs.model_to_use
    assert first.kwargs["tools"][0]["function"]["name"] == "get_command_constraints"
    assert second.kwargs["tools"] == first.kwargs["tools"]
    history = second.kwargs["messages"][-3:]
    assert history[0]["role"] == "assistant"
    assert [call["id"] for call in history[0]["tool_calls"]] == ["call-1", "call-2"]
    assert all(call["type"] == "function" for call in history[0]["tool_calls"])
    assert [message["tool_call_id"] for message in history[1:]] == ["call-1", "call-2"]
    assert chunks[-1].data.model_calls[0].model_role == (
        "assistant" if active_agent == ReasoningAgent.DASH else "primary"
    )
    assert chunks[-1].data.model_calls[0].tools_declared == ["get_command_constraints"]


async def test_malformed_turn_recovers_without_replaying_completed_tools():
    agent, inputs, provider, executor, create = _setup(
        [
            _response(calls=[_call("call-1")], finish_reason="tool_calls"),
            _response(calls=[_call("invalid")], finish_reason="malformed_function_call"),
            _response(calls=[_call("call-2")], finish_reason="tool_calls"),
            _response(text="RAM checked."),
        ]
    )
    original_contents = list(inputs.contents)

    chunks = [
        chunk
        async for chunk in agent.stream_response(
            inputs=inputs,
            llm_provider=provider,
            event_service=make_event_service(),
        )
    ]

    assert chunks[-1].type == StreamChunkFromModelType.COMPLETE
    assert executor.execute_tool_call.await_count == 2
    history = create.call_args_list[-1].kwargs["messages"]
    executed_ids = [call["id"] for message in history for call in message.get("tool_calls", [])]
    assert executed_ids == ["call-1", "call-2"]
    calls = chunks[-1].data.model_calls
    assert [call.succeeded for call in calls] == [True, False, True, True]
    assert calls[1].error_type == "MalformedFunctionCall"
    assert inputs.contents == original_contents
    repair = create.call_args_list[2].kwargs["messages"][-1]
    assert repair["role"] == "user"
    assert "not executed" in repair["content"]


async def test_repeated_malformed_calls_emit_console_failure_instead_of_completion():
    agent, inputs, provider, executor, create = _setup(
        [
            _response(
                text="First, I need to check command restrictions.",
                finish_reason="malformed_function_call",
            )
            for _ in range(AGENT_MAX_RETRIES + 1)
        ]
    )
    state = make_agent_stream_state()
    events = make_event_service()

    await agent.run_with_sse(
        inputs=inputs,
        state=state,
        llm_provider=provider,
        event_service=events,
    )

    assert create.await_count == AGENT_MAX_RETRIES + 1
    executor.execute_tool_call.assert_not_awaited()
    assert state.stream_failed is True
    assert state.error is not None
    assert "did not complete" in state.error
    published_types = [event.event_type for event in events._published_events]
    assert EventType.AI_LLM_CHAT_ITERATION_FAILED in published_types
    assert EventType.AI_LLM_CHAT_ITERATION_TEXT_COMPLETED not in published_types
    assert all(call.succeeded is False for call in state.model_calls)
