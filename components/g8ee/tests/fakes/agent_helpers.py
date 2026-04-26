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

"""Consolidated agent streaming and turn processing helpers for g8ee tests."""

from typing import Any
from unittest.mock import AsyncMock, MagicMock

from app.models.settings import G8eeUserSettings, LLMSettings
from app.constants import AgentMode
from app.llm.llm_types import ThoughtSignature
from app.models.agent import (
    AgentInputs,
    AgentStreamState,
    StreamChunkFromModel,
    TurnResult,
)
from app.services.ai.agent import g8eEngine
from app.services.ai.agent_turn import process_provider_turn
from app.services.ai.request_builder import AIRequestBuilder
from tests.fakes.factories import build_g8e_http_context


def make_gen_config(
    settings: G8eeUserSettings = None,
    agent_mode: AgentMode = AgentMode.OPERATOR_NOT_BOUND,
    system_instructions: str = "You are a helpful assistant.",
):
    """Build an AIRequestBuilder PrimaryLLMSettings for tests."""
    fn_handler = MagicMock()
    fn_handler._tool_declarations = {}
    builder = AIRequestBuilder(tool_executor=fn_handler)
    if settings is None:
        settings = G8eeUserSettings(
            llm=LLMSettings(primary_model="test-model"),
        )
    return builder.get_generation_config(
        system_instructions=system_instructions,
        settings=settings,
        agent_mode=agent_mode,
    )


def make_agent_inputs(
    case_id: str = "case-test-001",
    investigation_id: str = "inv-test-001",
    web_session_id: str = "web-test-001",
    user_id: str = "user-test-001",
    agent_mode: AgentMode = AgentMode.OPERATOR_BOUND,
    sentinel_mode: bool = True,
    investigation=None,
    g8e_context=None,
    request_settings=None,
    model_to_use: str = "test-model",
    generation_config=None,
    **kwargs,
) -> AgentInputs:
    """Build an AgentInputs (immutable request-scoped inputs) with sensible test defaults.

    ``model_to_use`` and ``generation_config`` default to values that satisfy
    ``run_with_sse``'s field-validation (the method now reads these from inputs
    directly rather than accepting them as separate arguments).
    """
    from tests.fakes.factories import build_enriched_context

    if investigation is None:
        investigation = build_enriched_context(
            investigation_id=investigation_id,
            case_id=case_id,
            user_id=user_id,
            sentinel_mode=sentinel_mode,
        )

    if g8e_context is None:
        g8e_context = build_g8e_http_context(
            web_session_id=web_session_id,
            user_id=user_id,
        )

    if request_settings is None:
        from app.llm.factory import get_llm_settings
        llm_from_env = get_llm_settings()
        if llm_from_env:
            request_settings = G8eeUserSettings(llm=llm_from_env)
        else:
            request_settings = G8eeUserSettings(
                llm=LLMSettings(
                    primary_model="test-model",
                    assistant_model="test-model",
                    lite_model="test-model",
                    assistant_provider="ollama",
                    lite_provider="ollama",
                    primary_provider="ollama"
                )
            )

    if generation_config is None:
        generation_config = make_gen_config(
            settings=request_settings,
            agent_mode=agent_mode,
        )

    return AgentInputs(
        case_id=case_id,
        investigation_id=investigation_id,
        web_session_id=web_session_id,
        user_id=user_id,
        agent_mode=agent_mode,
        sentinel_mode=sentinel_mode,
        investigation=investigation,
        g8e_context=g8e_context,
        request_settings=request_settings,
        model_to_use=model_to_use,
        generation_config=generation_config,
        **kwargs,
    )


def make_agent_stream_state() -> AgentStreamState:
    """Build a fresh, empty AgentStreamState."""
    return AgentStreamState()


def make_agent_run_args(
    case_id: str = "case-test-001",
    investigation_id: str = "inv-test-001",
    web_session_id: str = "web-test-001",
    user_id: str = "user-test-001",
    agent_mode: AgentMode = AgentMode.OPERATOR_BOUND,
    sentinel_mode: bool = True,
    investigation=None,
    g8e_context=None,
    request_settings=None,
    **kwargs,
) -> tuple[AgentInputs, AgentStreamState]:
    """Build (inputs, state) pair for agent run tests.

    This helper enforces the pairing convention: inputs are immutable request-scoped
    data, state is the mutable sink populated during the run. Using this helper
    prevents drift in test code between the two objects.
    """
    inputs = make_agent_inputs(
        case_id=case_id,
        investigation_id=investigation_id,
        web_session_id=web_session_id,
        user_id=user_id,
        agent_mode=agent_mode,
        sentinel_mode=sentinel_mode,
        investigation=investigation,
        g8e_context=g8e_context,
        request_settings=request_settings,
        **kwargs,
    )
    state = make_agent_stream_state()
    return inputs, state


def make_g8e_agent(
    fn_handler=None,
    approval_service=None,
) -> g8eEngine:
    """Build a g8eEngine suitable for unit tests."""
    if fn_handler is None:
        fn_handler = MagicMock()
        fn_handler._tool_declarations = {}

    return g8eEngine(
        tool_executor=fn_handler,
        approval_service=approval_service,
    )


def make_provider_chunk(
    *,
    thought: bool = False,
    text: str = "",
    thought_signature: ThoughtSignature = None,
    tool_calls: list = None,
    finish_reason: str = None,
) -> MagicMock:
    """Build a minimal fake provider chunk."""
    chunk = MagicMock()
    chunk.thought = thought
    chunk.text = text
    chunk.thought_signature = thought_signature
    chunk.tool_calls = tool_calls or []
    chunk.usage_metadata
    chunk.finish_reason = finish_reason
    return chunk


class FakeStreamProvider:
    """Provider stub that yields a fixed list of chunks."""

    def __init__(self, chunks: list):
        self._chunks = chunks

    def generate_content_stream_primary(self, **kwargs):
        async def _gen():
            for c in self._chunks:
                yield c
        return _gen()


class FakeMultiTurnStreamProvider:
    """Provider stub that yields successive chunk-lists across multiple calls."""

    def __init__(self, chunks_per_call: list[list]):
        self._chunks_per_call = chunks_per_call
        self._call_idx = 0

    def generate_content_stream_primary(self, **kwargs):
        idx = self._call_idx
        self._call_idx += 1
        chunks = self._chunks_per_call[idx] if idx < len(self._chunks_per_call) else []

        async def _gen():
            for c in chunks:
                yield c

        return _gen()


def patch_stream_response(agent: g8eEngine, chunks: list[StreamChunkFromModel]) -> None:
    """Replace agent.stream_response with an async generator that yields chunks."""
    async def _fake_stream(*args, **kwargs):
        for chunk in chunks:
            yield chunk
    agent.stream_response = _fake_stream


async def collect_stream_from_model_chunks(
    agent: g8eEngine,
    inputs: AgentInputs,
    g8ed_event_service: Any = None,
    llm_provider: Any = None,
) -> list[StreamChunkFromModel]:
    """Consume agent._stream_with_tool_loop and return all yielded chunks."""
    if inputs.generation_config is None:
        inputs.generation_config = make_gen_config(agent_mode=inputs.agent_mode or AgentMode.OPERATOR_NOT_BOUND)
    if inputs.model_to_use is None:
        inputs.model_to_use = "test-model"
    chunks: list[StreamChunkFromModel] = []
    async for chunk in agent._stream_with_tool_loop(
        inputs=inputs,
        g8ed_event_service=g8ed_event_service or make_g8ed_event_service(),
        llm_provider=llm_provider,
    ):
        chunks.append(chunk)
    return chunks


def make_g8ed_event_service():
    """Build a mock EventService for g8ed SSE publishing."""
    from app.models.events import SessionEvent
    
    svc = MagicMock()
    published_events = []
    
    async def capture_publish(event):
        """Capture the published event for test inspection."""
        published_events.append(event)
        return "success"
    
    async def capture_publish_investigation_event(
        investigation_id: str,
        event_type: str,
        payload: dict | object,
        web_session_id: str,
        case_id: str,
        user_id: str,
    ):
        """Capture investigation event calls and create proper SessionEvent."""
        session_event = SessionEvent(
            event_type=event_type,
            payload=payload,
            investigation_id=investigation_id,
            web_session_id=web_session_id,
            case_id=case_id,
            user_id=user_id,
        )
        return await capture_publish(session_event)
    
    svc.publish = AsyncMock(side_effect=capture_publish)
    svc.publish_command_event = AsyncMock()
    svc.publish_investigation_event = AsyncMock(side_effect=capture_publish_investigation_event)
    
    async def capture_publish_reputation(event_type, data, g8e_context):
        """Capture reputation event calls and create proper SessionEvent."""
        session_event = SessionEvent(
            event_type=event_type,
            payload=data,
            web_session_id=g8e_context.web_session_id,
            user_id=g8e_context.user_id,
            case_id=g8e_context.case_id,
            investigation_id=g8e_context.investigation_id,
        )
        return await capture_publish(session_event)
    
    svc.publish_reputation_event = AsyncMock(side_effect=capture_publish_reputation)
    
    # Store the captured events on the service for test access
    svc._published_events = published_events
    return svc


async def run_process_provider_turn(
    provider_chunks: list,
    model_name: str = "test-model",
) -> tuple[list[StreamChunkFromModel], list]:
    """Drive process_provider_turn with the given provider chunks."""
    async def _gen():
        for c in provider_chunks:
            yield c

    result_out: list[TurnResult] = []
    stream_chunks: list[StreamChunkFromModel] = []
    async for chunk in process_provider_turn(_gen(), model_name, result_out):
        stream_chunks.append(chunk)

    return stream_chunks, result_out[0].model_response_parts
