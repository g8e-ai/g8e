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

"""Unit tests for ChatPipelineService per-iteration AI text persistence.

Pins the contract that ``_run_chat_impl`` wires an ``on_iteration_text``
callback into ``g8e_agent.run_with_sse`` such that:

  - every tool iteration's pre-tool commentary lands in ``conversation_history``
    as a ``MessageSender.AI_PRIMARY`` row tagged with
    ``EventType.EVENT_SOURCE_AI_PRIMARY``;
  - the final post-stream ``_persist_ai_response`` is still invoked, and
    skips the write only when ``ctx.response_text`` is whitespace-only;
  - intermediate rows carry no grounding/token-usage (those are aggregate
    totals attached only to the final row).
"""

from __future__ import annotations

import pytest
from unittest.mock import AsyncMock, MagicMock, patch

from app.constants import (
    AgentMode,
    EventType,
    LLM_DEFAULT_MAX_OUTPUT_TOKENS,
    ThinkingLevel,
    TriageComplexityClassification,
    TriageConfidence,
    TriageIntentClassification,
)
from app.constants.message_sender import MessageSender
from app.llm.llm_types import (
    PrimaryLLMSettings,
    ThinkingConfig,
    ToolCallingConfig,
    ToolConfig,
)
from app.models.agent import AgentInputs, AgentStreamState
from app.models.agents.triage import TriageResult
from app.models.investigations import AIResponseMetadata
from app.models.settings import G8eeUserSettings, LLMSettings
from app.services.ai.chat_pipeline import ChatPipelineService
from tests.fakes.fake_event_service import FakeEventService
from tests.fakes.factories import build_enriched_context, build_g8e_http_context

pytestmark = [pytest.mark.unit, pytest.mark.asyncio]


HIGH_CONFIDENCE_COMPLEX_TRIAGE = TriageResult(
    complexity=TriageComplexityClassification.COMPLEX,
    complexity_confidence=TriageConfidence.HIGH,
    intent=TriageIntentClassification.ACTION,
    intent_confidence=TriageConfidence.HIGH,
    intent_summary="run a diagnostic",
)


def _make_pipeline() -> ChatPipelineService:
    svc = ChatPipelineService.__new__(ChatPipelineService)
    svc.g8ed_event_service = FakeEventService()
    svc.g8e_agent = MagicMock()
    svc.investigation_service = MagicMock()
    svc.investigation_service.add_chat_message = AsyncMock(return_value=True)
    svc.agent_activity_data_service = MagicMock()
    svc.agent_activity_data_service.record_activity = AsyncMock()

    async def _fake_persist_ai_message(
        investigation_id,
        text,
        grounding_metadata=None,
        token_usage=None,
    ) -> bool:
        # Mirror the real helper's strip-guard so tests exercise the same
        # contract as production (whitespace-only text is not persisted).
        if not investigation_id or not text.strip():
            return False
        await svc.investigation_service.add_chat_message(
            investigation_id=investigation_id,
            sender=MessageSender.AI_PRIMARY,
            content=text,
            metadata=AIResponseMetadata(
                source=EventType.EVENT_SOURCE_AI_PRIMARY,
                grounding_metadata=grounding_metadata,
                token_usage=token_usage,
            ),
        )
        return True

    svc.investigation_service.persist_ai_message = AsyncMock(side_effect=_fake_persist_ai_message)
    svc.memory_generation_service = MagicMock()
    svc.memory_generation_service.update_memory_from_conversation = AsyncMock()
    return svc


def _make_ctx(triage_result: TriageResult = HIGH_CONFIDENCE_COMPLEX_TRIAGE) -> tuple[AgentInputs, AgentStreamState]:
    inv = build_enriched_context(investigation_id="inv-iter")
    g8e_ctx = build_g8e_http_context(investigation_id="inv-iter", user_id="user-iter")
    request_settings = G8eeUserSettings(llm=LLMSettings())

    inputs = AgentInputs(
        investigation=inv,
        g8e_context=g8e_ctx,
        request_settings=request_settings,
        case_id="case-iter",
        investigation_id="inv-iter",
        user_id="user-iter",
        web_session_id="web-iter",
        agent_mode=AgentMode.OPERATOR_BOUND,
        operator_bound=True,
        model_to_use="primary-model",
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
        triage_result=triage_result,
        user_memories=[],
        case_memories=[],
    )
    
    state = AgentStreamState()
    return inputs, state


def _ai_primary_calls(mock_add: AsyncMock) -> list[dict]:
    """Return only the AI_PRIMARY add_chat_message invocations."""
    return [
        call.kwargs
        for call in mock_add.call_args_list
        if call.kwargs.get("sender") == MessageSender.AI_PRIMARY
    ]


async def test_intermediate_iteration_text_persists_as_ai_primary_rows():
    """Every on_iteration_text invocation must produce an AI_PRIMARY row."""
    svc = _make_pipeline()
    inputs, state = _make_ctx()
    svc._prepare_chat_context = AsyncMock(return_value=inputs)

    iteration_texts = [
        "First, I'll check the service status.",
        "Now I'll inspect the recent logs.",
    ]

    async def _fake_run_with_sse(**kwargs):
        on_iter = kwargs["on_iteration_text"]
        for text in iteration_texts:
            await on_iter(text)
        # Simulate the agent ending on a final wrap-up segment that the SSE
        # delivery layer leaves in state.response_text for _persist_ai_response.
        # Mutate the *actual* state instance constructed by _run_chat_impl,
        # not the outer test fixture — otherwise the final persist path runs
        # against an empty response_text and no final row is written.
        kwargs["state"].response_text = "All checks completed — service is healthy."

    svc.g8e_agent.run_with_sse = AsyncMock(side_effect=_fake_run_with_sse)

    g8e_ctx = build_g8e_http_context(investigation_id="inv-iter", web_session_id="web-iter")
    user_settings = G8eeUserSettings(llm=LLMSettings())

    with patch("app.services.ai.chat_pipeline.get_llm_provider"):
        await svc._run_chat_impl(
            message="diagnose the service",
            g8e_context=g8e_ctx,
            attachments=[],
            sentinel_mode=True,
            llm_primary_provider=None,
            llm_assistant_provider=None,
            llm_lite_provider=None,
            llm_primary_model="",
            llm_assistant_model="",
            llm_lite_model="",
            user_settings=user_settings,
        )

    primary_calls = _ai_primary_calls(svc.investigation_service.add_chat_message)
    contents = [c["content"] for c in primary_calls]

    assert contents == [
        iteration_texts[0],
        iteration_texts[1],
        "All checks completed — service is healthy.",
    ]

    # Intermediate rows: AI_PRIMARY source, no grounding, no token usage.
    for intermediate in primary_calls[:-1]:
        meta = intermediate["metadata"]
        assert isinstance(meta, AIResponseMetadata)
        assert meta.source == EventType.EVENT_SOURCE_AI_PRIMARY
        assert meta.grounding_metadata is None
        assert meta.token_usage is None
        assert intermediate["investigation_id"] == "inv-iter"

    # Final row: still AI_PRIMARY; its grounding/token-usage come from ctx
    # (both None in this fake — real runs populate them via COMPLETE chunks).
    final = primary_calls[-1]
    assert final["sender"] == MessageSender.AI_PRIMARY
    assert isinstance(final["metadata"], AIResponseMetadata)
    assert final["metadata"].source == EventType.EVENT_SOURCE_AI_PRIMARY


async def test_final_persist_skipped_when_response_text_is_whitespace_only():
    """Agents ending on a tool result must not produce an empty AI_PRIMARY row."""
    svc = _make_pipeline()
    inputs, state = _make_ctx()
    svc._prepare_chat_context = AsyncMock(return_value=inputs)

    async def _fake_run_with_sse(**kwargs):
        on_iter = kwargs["on_iteration_text"]
        await on_iter("Iteration commentary before tool call.")
        # Stream ends on TOOL_RESULT — response_text already cleared by SSE.
        kwargs["state"].response_text = "   "

    svc.g8e_agent.run_with_sse = AsyncMock(side_effect=_fake_run_with_sse)

    g8e_ctx = build_g8e_http_context(investigation_id="inv-iter", web_session_id="web-iter")
    user_settings = G8eeUserSettings(llm=LLMSettings())

    with patch("app.services.ai.chat_pipeline.get_llm_provider"):
        await svc._run_chat_impl(
            message="run a tool",
            g8e_context=g8e_ctx,
            attachments=[],
            sentinel_mode=True,
            llm_primary_provider=None,
            llm_assistant_provider=None,
            llm_lite_provider=None,
            llm_primary_model="",
            llm_assistant_model="",
            llm_lite_model="",
            user_settings=user_settings,
        )

    primary_calls = _ai_primary_calls(svc.investigation_service.add_chat_message)
    contents = [c["content"] for c in primary_calls]

    # Only the intermediate iteration was persisted — no trailing empty row.
    assert contents == ["Iteration commentary before tool call."]


async def test_iteration_callback_skips_whitespace_only_text():
    """Whitespace-only iteration text must not produce an AI_PRIMARY row."""
    svc = _make_pipeline()
    inputs, state = _make_ctx()
    svc._prepare_chat_context = AsyncMock(return_value=inputs)

    async def _fake_run_with_sse(**kwargs):
        on_iter = kwargs["on_iteration_text"]
        # Whitespace-only — should be dropped by the closure's guard.
        await on_iter("   \n  ")
        await on_iter("Real commentary.")
        kwargs["state"].response_text = "Final answer."

    svc.g8e_agent.run_with_sse = AsyncMock(side_effect=_fake_run_with_sse)

    g8e_ctx = build_g8e_http_context(investigation_id="inv-iter", web_session_id="web-iter")
    user_settings = G8eeUserSettings(llm=LLMSettings())

    with patch("app.services.ai.chat_pipeline.get_llm_provider"):
        await svc._run_chat_impl(
            message="diagnose",
            g8e_context=g8e_ctx,
            attachments=[],
            sentinel_mode=True,
            llm_primary_provider=None,
            llm_assistant_provider=None,
            llm_lite_provider=None,
            llm_primary_model="",
            llm_assistant_model="",
            llm_lite_model="",
            user_settings=user_settings,
        )

    contents = [c["content"] for c in _ai_primary_calls(svc.investigation_service.add_chat_message)]
    assert contents == ["Real commentary.", "Final answer."]


async def test_iteration_callback_passed_to_run_with_sse():
    """Regression: _run_chat_impl must wire a callback that writes AI_PRIMARY rows.

    It is not enough that *some* callable is passed — the callable must actually
    produce an ``investigation_service.add_chat_message`` call with
    ``sender == MessageSender.AI_PRIMARY`` and
    ``metadata.source == EventType.EVENT_SOURCE_AI_PRIMARY`` when invoked with
    non-empty text. Otherwise the iteration-persistence contract can silently
    regress to a no-op.
    """
    svc = _make_pipeline()
    inputs, state = _make_ctx()
    svc._prepare_chat_context = AsyncMock(return_value=inputs)

    captured: dict = {}

    async def _fake_run_with_sse(**kwargs):
        captured.update(kwargs)
        # Drive the callback with a realistic iteration-text payload so that
        # the observable side effect (add_chat_message) is exercised here,
        # not just parameter passing.
        await kwargs["on_iteration_text"]("mid-turn commentary")
        kwargs["state"].response_text = "done"

    svc.g8e_agent.run_with_sse = AsyncMock(side_effect=_fake_run_with_sse)

    g8e_ctx = build_g8e_http_context(investigation_id="inv-iter", web_session_id="web-iter")
    user_settings = G8eeUserSettings(llm=LLMSettings())

    with patch("app.services.ai.chat_pipeline.get_llm_provider"):
        await svc._run_chat_impl(
            message="hi",
            g8e_context=g8e_ctx,
            attachments=[],
            sentinel_mode=True,
            llm_primary_provider=None,
            llm_assistant_provider=None,
            llm_lite_provider=None,
            llm_primary_model="",
            llm_assistant_model="",
            llm_lite_model="",
            user_settings=user_settings,
        )

    assert "on_iteration_text" in captured
    assert callable(captured["on_iteration_text"])

    primary_calls = _ai_primary_calls(svc.investigation_service.add_chat_message)
    # Exactly one intermediate row (the callback invocation) plus the final
    # row from _persist_ai_response.
    assert [c["content"] for c in primary_calls] == ["mid-turn commentary", "done"]

    intermediate = primary_calls[0]
    assert intermediate["sender"] == MessageSender.AI_PRIMARY
    assert intermediate["investigation_id"] == "inv-iter"
    assert isinstance(intermediate["metadata"], AIResponseMetadata)
    assert intermediate["metadata"].source == EventType.EVENT_SOURCE_AI_PRIMARY
    # Intermediate rows never carry aggregate grounding/token-usage.
    assert intermediate["metadata"].grounding_metadata is None
    assert intermediate["metadata"].token_usage is None
