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

import pytest
from unittest.mock import AsyncMock, MagicMock, patch

from app.constants import (
    EventType,
    TriageComplexityClassification,
    TriageConfidence,
    TriageIntentClassification,
    AgentMode,
    LLM_DEFAULT_TEMPERATURE,
    LLM_DEFAULT_MAX_OUTPUT_TOKENS,
)
from app.llm.llm_types import PrimaryLLMSettings
from app.models.agent import AgentStreamContext
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
    svc.memory_generation_service = MagicMock()
    svc.memory_generation_service.update_memory_from_conversation = AsyncMock()
    return svc

def _make_chat_context(triage_result: TriageResult) -> AgentStreamContext:
    inv = build_enriched_context(investigation_id="inv-1")
    g8e_ctx = build_g8e_http_context(user_id="user-1")
    from app.models.settings import G8eeUserSettings, LLMSettings
    request_settings = G8eeUserSettings(llm=LLMSettings())
    
    streaming = AgentStreamContext(
        investigation=inv,
        g8e_context=g8e_ctx,
        request_settings=request_settings,
        case_id="case-1",
        investigation_id="inv-1",
        web_session_id="web-1",
        agent_mode=AgentMode.OPERATOR_NOT_BOUND,
        sentinel_mode=True,
    )
    
    agent_ctx = AgentStreamContext(
        investigation=inv,
        g8e_context=g8e_ctx,
        request_settings=request_settings,
        case_id="case-1",
        investigation_id="inv-1",
        user_id="user-1",
        web_session_id="web-1",
        agent_mode=AgentMode.OPERATOR_NOT_BOUND,
    )
    
    return AgentStreamContext(
        investigation=inv,
        g8e_context=g8e_ctx,
        request_settings=request_settings,
        agent_mode=AgentMode.OPERATOR_NOT_BOUND,
        operator_bound=False,
        model_to_use="lite-model",
        max_tokens=None,
        conversation_history=[],
        system_instructions="",
        contents=[],
        generation_config=PrimaryLLMSettings(
            temperature=LLM_DEFAULT_TEMPERATURE,
            max_output_tokens=LLM_DEFAULT_MAX_OUTPUT_TOKENS,
        ),
        streaming_context=streaming,
        agent_context=agent_ctx,
        user_memories=[],
        case_memories=[],
        triage_result=triage_result,
    )

# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------

async def test_run_chat_impl_short_circuits_correctly():
    svc = _make_pipeline()
    g8e_ctx = build_g8e_http_context(investigation_id="inv-1", web_session_id="web-1")
    ctx = _make_chat_context(triage_result=LOW_CONFIDENCE_TRIAGE_RESULT)
    
    # Mock _prepare_chat_context to return our prepared context
    svc._prepare_chat_context = AsyncMock(return_value=ctx)
    # Mock get_llm_provider to avoid actual LLM client creation
    with patch("app.services.ai.chat_pipeline.get_llm_provider"):
        await svc._run_chat_impl(
            message="hello",
            g8e_context=g8e_ctx,
            attachments=[],
            sentinel_mode=True,
            llm_primary_model="main-model",
            llm_assistant_model="assistant-model",
            user_settings=MagicMock(),
        )
    
    # 1. Verify event publishing
    # We use our FakeEventService which records events
    events = svc.g8ed_event_service.published
    
    # Filter for our investigation and event types
    chunk_events = [e for e in events if e.investigation_id == "inv-1" and e.event_type == EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED]
    complete_events = [e for e in events if e.investigation_id == "inv-1" and e.event_type == EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED]
    
    assert len(chunk_events) == 1
    assert len(complete_events) == 1
    
    # Verify Chunk Payload
    from app.models.g8ed_client import ChatResponseChunkPayload, ChatResponseCompletePayload
    assert isinstance(chunk_events[0].payload, ChatResponseChunkPayload)
    assert chunk_events[0].payload.content == LOW_CONFIDENCE_TRIAGE_RESULT.follow_up_question
    
    # Verify Complete Payload
    assert isinstance(complete_events[0].payload, ChatResponseCompletePayload)
    assert complete_events[0].payload.content == LOW_CONFIDENCE_TRIAGE_RESULT.follow_up_question
    assert complete_events[0].payload.finish_reason == "stop"
    
    # 2. Verify agent was NOT called
    svc.g8e_agent.run_with_sse.assert_not_called()
    
    # 3. Verify persistence
    # _persist_ai_response should have been called via add_chat_message on investigation_service
    svc.investigation_service.add_chat_message.assert_called_once()
    call_args = svc.investigation_service.add_chat_message.call_args
    assert call_args.kwargs["content"] == LOW_CONFIDENCE_TRIAGE_RESULT.follow_up_question
    assert call_args.kwargs["investigation_id"] == "inv-1"
