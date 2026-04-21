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

"""
Unit tests for ChatPipelineService short-circuit delivery logic.

Verifies that when triage returns LOW confidence, the pipeline:
1. Publishes an LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED event with the follow-up question.
2. Persists the follow-up question as the AI response.
3. Does NOT call the main LLM agent.
"""

import asyncio
import pytest
from unittest.mock import AsyncMock, MagicMock, patch

from app.constants import (
    EventType,
    TriageComplexityClassification,
    TriageConfidence,
    TriageIntentClassification,
    AgentMode,
    LLM_DEFAULT_MAX_OUTPUT_TOKENS,
    ThinkingLevel,
)
from app.llm.llm_types import (
    PrimaryLLMSettings,
    ThinkingConfig,
    ToolConfig,
    ToolCallingConfig,
)
from app.llm.utils import ModelOverrideResolver
from app.models.agent import AgentInputs, AgentStreamState
from app.models.agents.triage import TriageResult
from app.services.ai.chat_pipeline import ChatPipelineService
from tests.fakes.factories import (
    build_g8e_http_context,
    build_enriched_context,
)
from tests.fakes.fake_event_service import FakeEventService as create_mock_event_service

pytestmark = [pytest.mark.unit, pytest.mark.asyncio]

# ---------------------------------------------------------------------------
# Standard Triage Results for testing
# ---------------------------------------------------------------------------

LOW_CONFIDENCE_TRIAGE_RESULT = TriageResult(
    complexity=TriageComplexityClassification.COMPLEX,
    complexity_confidence=TriageConfidence.LOW,
    intent=TriageIntentClassification.UNKNOWN,
    intent_confidence=TriageConfidence.LOW,
    intent_summary="ambiguous",
    follow_up_question="Could you clarify what you mean?",
)

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _make_pipeline() -> ChatPipelineService:
    svc = ChatPipelineService.__new__(ChatPipelineService)
    svc.g8ed_event_service = create_mock_event_service()
    svc.g8e_agent = MagicMock()
    svc.g8e_agent.run_with_sse = AsyncMock()
    svc.investigation_service = MagicMock()
    svc.investigation_service.add_chat_message = AsyncMock()
    svc.investigation_service.persist_ai_message = AsyncMock(return_value=True)
    svc.memory_generation_service = MagicMock()
    svc.memory_generation_service.update_memory_from_conversation = AsyncMock()
    return svc

def _make_chat_context(triage_result: TriageResult) -> tuple[AgentInputs, AgentStreamState]:
    inv = build_enriched_context(investigation_id="inv-1")
    g8e_ctx = build_g8e_http_context(user_id="user-1")
    from app.models.settings import G8eeUserSettings, LLMSettings
    request_settings = G8eeUserSettings(llm=LLMSettings())
    
    inputs = AgentInputs(
        investigation=inv,
        g8e_context=g8e_ctx,
        request_settings=request_settings,
        case_id="case-1",
        investigation_id="inv-1",
        user_id="user-1",
        web_session_id="web-1",
        agent_mode=AgentMode.OPERATOR_NOT_BOUND,
        sentinel_mode=True,
        operator_bound=False,
        model_to_use="lite-model",
        max_tokens=None,
        conversation_history=[],
        system_instructions="",
        contents=[],
        generation_config=PrimaryLLMSettings(
            max_output_tokens=LLM_DEFAULT_MAX_OUTPUT_TOKENS,
            top_p_nucleus_sampling=1.0,
            top_k_filtering=40,
            stop_sequences=[],
            response_modalities=["TEXT"],
            tools=[],
            system_instructions="",
            thinking_config=ThinkingConfig(thinking_level=ThinkingLevel.OFF, include_thoughts=False),
            tool_config=ToolConfig(tool_calling_config=ToolCallingConfig(mode="AUTO")),
        ),
        user_memories=[],
        case_memories=[],
        triage_result=triage_result,
    )
    
    state = AgentStreamState()
    return inputs, state

# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------

async def test_run_chat_impl_short_circuits_correctly():
    svc = _make_pipeline()
    g8e_ctx = build_g8e_http_context(investigation_id="inv-1", web_session_id="web-1")
    inputs, state = _make_chat_context(triage_result=LOW_CONFIDENCE_TRIAGE_RESULT)
    
    # Mock _prepare_chat_context to return our prepared context
    svc._prepare_chat_context = AsyncMock(return_value=inputs)
    # Mock get_llm_provider to avoid actual LLM client creation
    with patch("app.services.ai.chat_pipeline.get_llm_provider"):
        await svc._run_chat_impl(
            message="hello",
            g8e_context=g8e_ctx,
            attachments=[],
            sentinel_mode=True,
            llm_primary_provider="openai",
            llm_assistant_provider="openai",
            llm_primary_model="main-model",
            llm_assistant_model="assistant-model",
            user_settings=MagicMock(),
        )
    
    # 1. Verify event publishing
    # We use our FakeEventService which records events
    events = svc.g8ed_event_service.published
    
    # Filter for our investigation and event types
    started_events = [e for e in events if e.investigation_id == "inv-1" and e.event_type == EventType.LLM_CHAT_ITERATION_STARTED]
    chunk_events = [e for e in events if e.investigation_id == "inv-1" and e.event_type == EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED]
    complete_events = [e for e in events if e.investigation_id == "inv-1" and e.event_type == EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED]
    
    assert len(started_events) == 1
    assert len(chunk_events) == 1
    assert len(complete_events) == 1
    
    # Verify STARTED arrives before CHUNK and COMPLETE
    inv_events = [e for e in events if e.investigation_id == "inv-1"]
    event_types = [e.event_type for e in inv_events]
    started_idx = event_types.index(EventType.LLM_CHAT_ITERATION_STARTED)
    chunk_idx = event_types.index(EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED)
    complete_idx = event_types.index(EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED)
    assert started_idx < chunk_idx < complete_idx
    
    # Verify Started Payload
    from app.models.g8ed_client import ChatProcessingStartedPayload, ChatResponseChunkPayload, ChatResponseCompletePayload
    assert isinstance(started_events[0].payload, ChatProcessingStartedPayload)
    assert started_events[0].payload.agent_mode == AgentMode.OPERATOR_NOT_BOUND
    
    # Verify Chunk Payload
    assert isinstance(chunk_events[0].payload, ChatResponseChunkPayload)
    assert chunk_events[0].payload.content == LOW_CONFIDENCE_TRIAGE_RESULT.follow_up_question
    
    # Verify Complete Payload
    assert isinstance(complete_events[0].payload, ChatResponseCompletePayload)
    assert complete_events[0].payload.content == LOW_CONFIDENCE_TRIAGE_RESULT.follow_up_question
    assert complete_events[0].payload.finish_reason == "stop"
    
    # 2. Verify agent was NOT called
    svc.g8e_agent.run_with_sse.assert_not_called()
    
    # 3. Verify persistence
    # _persist_ai_response calls persist_ai_message, which persists the follow-up question
    svc.investigation_service.persist_ai_message.assert_called_once()
    call_args = svc.investigation_service.persist_ai_message.call_args
    assert call_args.kwargs["text"] == LOW_CONFIDENCE_TRIAGE_RESULT.follow_up_question
    assert call_args.kwargs["investigation_id"] == "inv-1"


async def test_run_chat_exception_handler_publishes_iteration_failed():
    """Verify that when _run_chat_impl raises an exception, ITERATION_FAILED is published."""
    svc = _make_pipeline()
    g8e_ctx = build_g8e_http_context(investigation_id="inv-1", web_session_id="web-1", user_id="user-1", case_id="case-1")
    
    # Mock _run_chat_impl to raise an exception
    test_error = ValueError("Test error from _run_chat_impl")
    svc._run_chat_impl = AsyncMock(side_effect=test_error)
    
    # Mock ChatTaskManager
    from app.services.ai.chat_task_manager import ChatTaskManager
    mock_task_manager = MagicMock(spec=ChatTaskManager)
    mock_task_manager.track = AsyncMock()
    mock_task_manager.untrack = AsyncMock()
    
    from app.models.settings import G8eeUserSettings, LLMSettings
    user_settings = G8eeUserSettings(llm=LLMSettings())
    
    # Call run_chat - should catch exception and publish ITERATION_FAILED
    await svc.run_chat(
        message="hello",
        g8e_context=g8e_ctx,
        attachments=[],
        sentinel_mode=True,
        llm_primary_provider="openai",
        llm_assistant_provider="openai",
        llm_primary_model="main-model",
        llm_assistant_model="assistant-model",
        _task_manager=mock_task_manager,
        user_settings=user_settings,
        _track_task=True,
    )
    
    # Verify ITERATION_FAILED was published
    events = svc.g8ed_event_service.published
    failed_events = [e for e in events if e.investigation_id == "inv-1" and e.event_type == EventType.LLM_CHAT_ITERATION_FAILED]
    
    assert len(failed_events) == 1
    from app.models.g8ed_client import ChatErrorPayload
    assert isinstance(failed_events[0].payload, ChatErrorPayload)
    assert "Test error from _run_chat_impl" in failed_events[0].payload.error
    
    # Verify task was tracked and untracked
    mock_task_manager.track.assert_called_once_with("inv-1", asyncio.current_task())
    mock_task_manager.untrack.assert_called_once_with("inv-1")


async def test_run_chat_impl_coerces_provider_override_to_enum():
    """Regression: provider overrides must land as LLMProvider enum instances.

    model_copy(update=...) bypasses Pydantic validation, so a raw HTTP
    string would silently end up in an enum-typed field. This test pins
    the coercion at the override site by inspecting the LLMSettings
    passed to get_llm_provider.
    """
    from app.constants import LLMProvider
    from app.models.settings import G8eeUserSettings, LLMSettings

    svc = _make_pipeline()
    g8e_ctx = build_g8e_http_context(investigation_id="inv-1", web_session_id="web-1")
    inputs, state = _make_chat_context(triage_result=LOW_CONFIDENCE_TRIAGE_RESULT)
    svc._prepare_chat_context = AsyncMock(return_value=inputs)

    captured: dict = {}

    def _capture(llm_settings, is_assistant=False):
        captured["llm"] = llm_settings
        captured["is_assistant"] = is_assistant
        return MagicMock()

    user_settings = G8eeUserSettings(llm=LLMSettings())
    with patch("app.services.ai.chat_pipeline.get_llm_provider", side_effect=_capture):
        await svc._run_chat_impl(
            message="hello",
            g8e_context=g8e_ctx,
            attachments=[],
            sentinel_mode=True,
            llm_primary_provider="openai",
            llm_assistant_provider="anthropic",
            llm_primary_model="main-model",
            llm_assistant_model="assistant-model",
            user_settings=user_settings,
        )

    resolved_llm: LLMSettings = captured["llm"]
    assert isinstance(resolved_llm.primary_provider, LLMProvider)
    assert resolved_llm.primary_provider is LLMProvider.OPENAI
    assert isinstance(resolved_llm.assistant_provider, LLMProvider)
    assert resolved_llm.assistant_provider is LLMProvider.ANTHROPIC


async def test_prepare_chat_context_passes_assistant_model_to_triage():
    """Regression: triage runs on the assistant provider, so model_override
    must be the assistant model — not the primary model.

    Previously chat_pipeline passed llm_primary_model as the triage override,
    causing cross-provider mismatches (e.g. a Claude model name sent to the
    Gemini API endpoint, producing a 404 NOT_FOUND on generateContent).
    """
    from app.models.agents.triage import TriageRequest, TriageResult
    from app.models.settings import G8eeUserSettings, LLMSettings

    svc = _make_pipeline()
    svc.investigation_service.get_investigation_context = AsyncMock(
        return_value=build_enriched_context(investigation_id="inv-1")
    )
    svc.investigation_service.get_enriched_investigation_context = AsyncMock(
        return_value=build_enriched_context(investigation_id="inv-1")
    )
    svc.investigation_service.update_investigation_raw = AsyncMock()
    svc.investigation_service.get_chat_messages = AsyncMock(return_value=[])
    svc.memory_service = MagicMock()
    svc.memory_service.get_user_memories = AsyncMock(return_value=[])
    svc.memory_service.get_case_memories = AsyncMock(return_value=[])
    svc.request_builder = MagicMock()
    svc.request_builder.build_system_prompt = MagicMock(return_value="")
    svc.request_builder.format_attachment_parts = MagicMock(return_value=[])
    svc.request_builder.build_contents_from_history = MagicMock(return_value=[])

    captured: dict = {}

    async def _capture_triage(req: TriageRequest) -> TriageResult:
        captured["model_override"] = req.model_override
        return TriageResult(
            complexity=TriageComplexityClassification.COMPLEX,
            complexity_confidence=TriageConfidence.HIGH,
            intent=TriageIntentClassification.INFORMATION,
            intent_confidence=TriageConfidence.HIGH,
            intent_summary="ok",
        )

    svc.triage_agent = MagicMock()
    svc.triage_agent.triage = AsyncMock(side_effect=_capture_triage)

    g8e_ctx = build_g8e_http_context(
        investigation_id="inv-1", case_id="case-1", web_session_id="web-1", user_id="user-1"
    )
    request_settings = G8eeUserSettings(llm=LLMSettings())

    with patch("app.services.ai.chat_pipeline.resolve_model", return_value="main-model"):
        try:
            model_overrides = ModelOverrideResolver(
                primary_model="claude-opus-4-6",
                assistant_model="gemini-3-flash-preview",
            )
            await svc._prepare_chat_context(
                message="hello",
                g8e_context=g8e_ctx,
                request_settings=request_settings,
                attachments=[],
                sentinel_mode=True,
                model_overrides=model_overrides,
            )
        except Exception:
            # downstream steps may depend on additional mocked services; the
            # triage call happens early, which is all this test needs.
            pass

    assert captured["model_override"] == "gemini-3-flash-preview"


async def test_run_chat_impl_rejects_unknown_provider_override():
    """An unknown provider override surfaces as a ValueError — not a
    silent bad-value in an enum-typed field."""
    from app.models.settings import G8eeUserSettings, LLMSettings

    svc = _make_pipeline()
    g8e_ctx = build_g8e_http_context(investigation_id="inv-1", web_session_id="web-1")
    inputs, state = _make_chat_context(triage_result=LOW_CONFIDENCE_TRIAGE_RESULT)
    svc._prepare_chat_context = AsyncMock(return_value=inputs)

    user_settings = G8eeUserSettings(llm=LLMSettings())
    with patch("app.services.ai.chat_pipeline.get_llm_provider"):
        with pytest.raises(ValueError):
            await svc._run_chat_impl(
                message="hello",
                g8e_context=g8e_ctx,
                attachments=[],
                sentinel_mode=True,
                llm_primary_provider="not-a-real-provider",
                llm_assistant_provider=None,
                llm_primary_model="main-model",
                llm_assistant_model="assistant-model",
                user_settings=user_settings,
            )


async def test_run_chat_impl_selects_assistant_provider_for_simple_complexity():
    """Regression: when triage returns SIMPLE complexity, the assistant provider
    should be used — not the primary provider.

    Previously, get_llm_provider was called before triage, so is_assistant always
    defaulted to False, causing cross-provider mismatches (e.g. Gemini model sent
    to Anthropic endpoint).
    """
    from app.constants import LLMProvider
    from app.models.settings import G8eeUserSettings, LLMSettings

    svc = _make_pipeline()
    g8e_ctx = build_g8e_http_context(investigation_id="inv-1", web_session_id="web-1")
    
    # Create triage result with SIMPLE complexity (should use assistant provider)
    simple_triage_result = TriageResult(
        complexity=TriageComplexityClassification.SIMPLE,
        complexity_confidence=TriageConfidence.HIGH,
        intent=TriageIntentClassification.INFORMATION,
        intent_confidence=TriageConfidence.HIGH,
        intent_summary="ok",
    )
    inputs, state = _make_chat_context(triage_result=simple_triage_result)
    svc._prepare_chat_context = AsyncMock(return_value=inputs)

    captured: dict = {}

    def _capture(llm_settings, is_assistant=False):
        captured["llm"] = llm_settings
        captured["is_assistant"] = is_assistant
        return MagicMock()

    user_settings = G8eeUserSettings(llm=LLMSettings())
    with patch("app.services.ai.chat_pipeline.get_llm_provider", side_effect=_capture):
        await svc._run_chat_impl(
            message="hello",
            g8e_context=g8e_ctx,
            attachments=[],
            sentinel_mode=True,
            llm_primary_provider="anthropic",
            llm_assistant_provider="gemini",
            llm_primary_model="claude-opus-4-6",
            llm_assistant_model="gemini-3-flash-preview",
            user_settings=user_settings,
        )

    # Verify is_assistant=True was passed (assistant provider should be used)
    assert captured["is_assistant"] is True
