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

import pytest
from unittest.mock import AsyncMock, MagicMock, patch

from app.constants import (
    EventType,
    TriageComplexityClassification,
    TriageConfidence,
    TriageIntentClassification,
    MessageSender,
)
from app.models.agents.triage import TriageResult
from app.models.g8ed_client import TriageClarificationQuestionsPayload
from tests.fakes.factories import (
    build_g8e_http_context,
    build_enriched_context,
)
from tests.fakes.fake_event_service import FakeEventService as create_mock_event_service
from .test_chat_pipeline_triage_delivery import _make_pipeline, _make_chat_context

pytestmark = [pytest.mark.unit, pytest.mark.asyncio]

CLARIFYING_QUESTIONS_RESULT = TriageResult(
    complexity=TriageComplexityClassification.COMPLEX,
    complexity_confidence=TriageConfidence.HIGH,
    intent=TriageIntentClassification.UNKNOWN,
    intent_confidence=TriageConfidence.LOW,
    intent_summary="needs clarification",
    clarifying_questions=["Question 1?", "Question 2?", "Question 3?"],
)

async def test_run_chat_impl_emits_clarifying_questions():
    """Verify that when triage returns clarifying questions, the pipeline emits them and persists to history."""
    svc = _make_pipeline()
    g8e_ctx = build_g8e_http_context(investigation_id="inv-1", web_session_id="web-1")
    inputs, state = _make_chat_context(triage_result=CLARIFYING_QUESTIONS_RESULT)
    
    svc._prepare_chat_context = AsyncMock(return_value=inputs)
    
    with patch("app.services.ai.chat_pipeline.get_llm_provider"):
        await svc._run_chat_impl(
            message="hello",
            g8e_context=g8e_ctx,
            attachments=[],
            sentinel_mode=True,
            llm_primary_provider="openai",
            llm_assistant_provider="openai",
            llm_lite_provider="openai",
            llm_primary_model="main-model",
            llm_assistant_model="assistant-model",
            llm_lite_model="lite-model",
            user_settings=MagicMock(),
        )
    
    # 1. Verify event publishing
    events = svc.g8ed_event_service.published
    clarify_events = [e for e in events if e.investigation_id == "inv-1" and e.event_type == EventType.AI_TRIAGE_CLARIFICATION_QUESTIONS]
    
    assert len(clarify_events) == 1
    assert isinstance(clarify_events[0].payload, TriageClarificationQuestionsPayload)
    assert clarify_events[0].payload.questions == CLARIFYING_QUESTIONS_RESULT.clarifying_questions
    
    # 2. Verify persistence to conversation history (ledger)
    svc.investigation_service.investigation_data_service.add_chat_message.assert_called_once()
    args, kwargs = svc.investigation_service.investigation_data_service.add_chat_message.call_args
    assert kwargs["sender"] == MessageSender.AI_TRIAGE
    assert "Question 1?" in kwargs["content"]
    assert kwargs["metadata"].event_type == EventType.AI_TRIAGE_CLARIFICATION_QUESTIONS
    assert kwargs["metadata"].clarifying_questions == CLARIFYING_QUESTIONS_RESULT.clarifying_questions

async def test_run_chat_impl_handles_both_follow_up_and_clarifying_questions():
    """Verify that both follow-up and clarifying questions are processed if both are present."""
    result_with_both = TriageResult(
        complexity=TriageComplexityClassification.COMPLEX,
        complexity_confidence=TriageConfidence.HIGH,
        intent=TriageIntentClassification.UNKNOWN,
        intent_confidence=TriageConfidence.LOW,
        intent_summary="ambiguous",
        follow_up_question="Follow up?",
        clarifying_questions=["Clarify 1?", "Clarify 2?", "Clarify 3?"],
    )
    
    svc = _make_pipeline()
    g8e_ctx = build_g8e_http_context(investigation_id="inv-1", web_session_id="web-1")
    inputs, state = _make_chat_context(triage_result=result_with_both)
    
    svc._prepare_chat_context = AsyncMock(return_value=inputs)
    
    with patch("app.services.ai.chat_pipeline.get_llm_provider"):
        await svc._run_chat_impl(
            message="hello",
            g8e_context=g8e_ctx,
            attachments=[],
            sentinel_mode=True,
            llm_primary_provider="openai",
            llm_assistant_provider="openai",
            llm_lite_provider="openai",
            llm_primary_model="main-model",
            llm_assistant_model="assistant-model",
            llm_lite_model="lite-model",
            user_settings=MagicMock(),
        )
    
    # Verify follow-up was processed (via events)
    events = svc.g8ed_event_service.published
    chunk_events = [e for e in events if e.event_type == EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED]
    assert len(chunk_events) == 1
    assert chunk_events[0].payload.content == "Follow up?"
    
    # Verify clarifying questions were also processed
    clarify_events = [e for e in events if e.event_type == EventType.AI_TRIAGE_CLARIFICATION_QUESTIONS]
    assert len(clarify_events) == 1
    assert clarify_events[0].payload.questions == result_with_both.clarifying_questions
