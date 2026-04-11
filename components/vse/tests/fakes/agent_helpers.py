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

"""Consolidated agent streaming and turn processing helpers for VSE tests."""

from typing import Any
from unittest.mock import AsyncMock, MagicMock

from app.models.settings import VSEPlatformSettings
from app.constants import AgentMode
from app.llm.llm_types import ThoughtSignature
from app.models.agent import AgentStreamContext, StreamChunkFromModel, TurnResult
from app.services.ai.agent import g8eAgent
from app.services.ai.agent_turn import process_provider_turn
from app.services.ai.request_builder import AIRequestBuilder
from tests.fakes.factories import build_vso_http_context


def make_gen_config(
    settings: VSEPlatformSettings = None,
    agent_mode: AgentMode = AgentMode.OPERATOR_NOT_BOUND,
    system_instructions: str = "You are a helpful assistant.",
):
    """Build an AIRequestBuilder GenerateContentConfig for tests."""
    fn_handler = MagicMock()
    fn_handler._tool_declarations = {}
    builder = AIRequestBuilder(tool_executor=fn_handler)
    return builder.get_generation_config(
        system_instructions=system_instructions,
        settings=settings or VSEPlatformSettings(
            port=443,
            llm={
                "provider": "ollama",
                "ollama_primary_model": "llama3",
                "ollama_assistant_model": "llama3",
            }
        ),
        agent_mode=agent_mode,
    )


def make_agent_stream_context(
    case_id: str = "case-test-001",
    investigation_id: str = "inv-test-001",
    web_session_id: str = "web-test-001",
    user_id: str = "user-test-001",
    agent_mode: AgentMode = AgentMode.OPERATOR_BOUND,
    investigation=None,
    vso_context=None,
    **kwargs,
) -> AgentStreamContext:
    """Build an AgentStreamContext with sensible test defaults."""
    return AgentStreamContext(
        case_id=case_id,
        investigation_id=investigation_id,
        web_session_id=web_session_id,
        user_id=user_id,
        agent_mode=agent_mode,
        investigation=investigation,
        vso_context=vso_context or build_vso_http_context(
            web_session_id=web_session_id,
            user_id=user_id,
        ),
        **kwargs,
    )


def make_agent_streaming_context(
    case_id: str = "case-test-001",
    investigation_id: str = "inv-test-001",
    web_session_id: str = "web-test-001",
    user_id: str = "user-test-001",
    agent_mode: AgentMode = AgentMode.OPERATOR_BOUND,
    sentinel_mode: bool = True,
    investigation=None,
    request_settings=None,
    **kwargs,
) -> AgentStreamContext:
    """Build a AgentStreamContext (SSE state tracker) with sensible test defaults."""
    from app.models.settings import VSEUserSettings
    from tests.fakes.factories import build_enriched_context
    
    if investigation is None:
        investigation = build_enriched_context(
            investigation_id=investigation_id,
            case_id=case_id,
            user_id=user_id,
            sentinel_mode=sentinel_mode,
        )
    
    vso_context = build_vso_http_context(
        web_session_id=web_session_id,
        user_id=user_id,
    )
    
    if request_settings is None:
        from app.models.settings import LLMSettings
        request_settings = VSEUserSettings(llm=LLMSettings())
    
    return AgentStreamContext(
        case_id=case_id,
        investigation_id=investigation_id,
        web_session_id=web_session_id,
        user_id=user_id,
        agent_mode=agent_mode,
        sentinel_mode=sentinel_mode,
        investigation=investigation,
        vso_context=vso_context,
        request_settings=request_settings,
        **kwargs,
    )


# Alias for backward compatibility
make_streaming_context = make_agent_streaming_context


def make_g8e_agent(
    provider=None,
    fn_handler=None,
) -> g8eAgent:
    """Build a g8eAgent suitable for unit tests."""
    if fn_handler is None:
        fn_handler = MagicMock()
        fn_handler._tool_declarations = {}
        fn_handler.start_invocation_context = MagicMock(return_value=None)
        fn_handler.reset_invocation_context = MagicMock()

    return g8eAgent(
        llm_provider=provider or MagicMock(),
        tool_executor=fn_handler,
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

    def generate_content_stream(self, **kwargs):
        async def _gen():
            for c in self._chunks:
                yield c
        return _gen()


class FakeMultiTurnStreamProvider:
    """Provider stub that yields successive chunk-lists across multiple calls."""

    def __init__(self, chunks_per_call: list[list]):
        self._chunks_per_call = chunks_per_call
        self._call_idx = 0

    def generate_content_stream(self, **kwargs):
        idx = self._call_idx
        self._call_idx += 1
        chunks = self._chunks_per_call[idx] if idx < len(self._chunks_per_call) else []

        async def _gen():
            for c in chunks:
                yield c

        return _gen()


def patch_stream_response(agent: g8eAgent, chunks: list[StreamChunkFromModel]) -> None:
    """Replace agent.stream_response with an async generator that yields chunks."""
    async def _fake_stream(*args, **kwargs):
        for chunk in chunks:
            yield chunk
    agent.stream_response = _fake_stream


async def collect_stream_from_model_chunks(
    agent: g8eAgent,
    context: AgentStreamContext,
    gen_config=None,
    model_name: str = "test-model",
    vsod_event_service: Any = None,
    llm_provider: Any = None,
) -> list[StreamChunkFromModel]:
    """Consume agent._stream_with_tool_loop and return all yielded chunks."""
    if gen_config is None:
        gen_config = make_gen_config(agent_mode=context.agent_mode or AgentMode.OPERATOR_NOT_BOUND)
    chunks: list[StreamChunkFromModel] = []
    async for chunk in agent._stream_with_tool_loop(
        contents=[],
        generation_config=gen_config,
        model_name=model_name,
        context=context,
        vsod_event_service=vsod_event_service or make_vsod_event_service(),
        llm_provider=llm_provider,
    ):
        chunks.append(chunk)
    return chunks


def make_vsod_event_service():
    """Build a mock EventService for VSOD SSE publishing."""
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
