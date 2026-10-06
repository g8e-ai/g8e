# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
Unit tests for ChatPipelineService short-circuit delivery logic.

Verifies that when triage returns LOW confidence, the pipeline:
1. Publishes an LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED event with the follow-up question.
2. Persists the follow-up question as the AI response.
3. Does NOT call the main LLM agent.
"""

import asyncio
from typing import cast
from unittest.mock import AsyncMock, MagicMock, patch

import pytest
from g8e.models.internal_api import EvaluationInferenceContext, InferenceModelVariant

from app.constants import (
    AgentMode,
    EventType,
    InvestigationStatus,
    LLMProvider,
    ReasoningAgent,
    ThinkingLevel,
    TriageComplexityClassification,
    TriageConfidence,
    TriageIntentClassification,
)
from app.llm.llm_types import (
    PrimaryLLMSettings,
    ThinkingConfig,
    ToolCallingConfig,
    ToolConfig,
)
from app.llm.utils import ModelOverrideResolver
from app.models.agent import AgentInputs, AgentStreamState
from app.models.agents.triage import TriageRequest, TriageResult
from app.models.events import ChatErrorPayload
from app.models.memory import InvestigationMemory
from app.models.settings import G8eeUserSettings, LLMSettings
from app.services.ai.chat_pipeline import ChatPipelineService
from app.services.ai.chat_task_manager import ChatTaskManager
from app.services.ai.request_builder import BuiltContents
from app.services.infra.event_service import EventService
from tests.fakes.factories import (
    build_enriched_context,
    build_g8e_http_context,
)
from tests.fakes.fake_event_service import FakeEventService

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
)

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_pipeline() -> ChatPipelineService:
    svc = ChatPipelineService.__new__(ChatPipelineService)
    fake_event_service = FakeEventService()
    svc.event_service = cast(EventService, fake_event_service)
    svc.g8e_agent = MagicMock()
    svc.g8e_agent.run_with_sse = AsyncMock()
    svc.investigation_service = MagicMock()
    svc.investigation_service.investigation_data_service.add_chat_message = AsyncMock()
    svc.investigation_service.persist_ai_message = AsyncMock(return_value=True)
    svc.memory_generation_service = MagicMock()
    svc.memory_generation_service.update_memory_from_conversation = AsyncMock()
    svc.agent_activity_data_service = MagicMock()
    svc.agent_activity_data_service.record_activity = AsyncMock()
    svc.evaluation_trace_service = MagicMock()
    svc.triage_agent = MagicMock()
    return svc


def _make_chat_context(triage_result: TriageResult) -> tuple[AgentInputs, AgentStreamState]:
    inv = build_enriched_context(investigation_id="inv-1")
    g8e_ctx = build_g8e_http_context(user_id="user-1")

    request_settings = G8eeUserSettings(llm=LLMSettings())

    inputs = AgentInputs(
        investigation=inv,
        g8e_context=g8e_ctx,
        request_settings=request_settings,
        case_id="case-1",
        investigation_id="inv-1",
        user_id="user-1",
        web_session_id="web-1",
        agent_mode=AgentMode.G8E_NOT_BOUND,
        sentinel_mode=True,
        operator_bound=False,
        model_to_use="lite-model",
        conversation_history=[],
        system_instructions="",
        contents=[],
        generation_config=PrimaryLLMSettings(
            max_output_tokens=None,
            top_p_nucleus_sampling=1.0,
            top_k_filtering=40,
            stop_sequences=[],
            response_modalities=["TEXT"],
            tools=[],
            system_instructions="",
            thinking_config=ThinkingConfig(
                thinking_level=ThinkingLevel.OFF, include_thoughts=False
            ),
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


async def test_run_chat_exception_handler_publishes_iteration_failed():
    """Verify that when _run_chat_impl raises an exception, ITERATION_FAILED is published."""
    svc = _make_pipeline()
    g8e_ctx = build_g8e_http_context(
        investigation_id="inv-1", web_session_id="web-1", user_id="user-1", case_id="case-1"
    )

    # Mock _run_chat_impl to raise an exception
    test_error = ValueError("Test error from _run_chat_impl")
    svc._run_chat_impl = AsyncMock(side_effect=test_error)

    # Mock ChatTaskManager

    mock_task_manager = MagicMock(spec=ChatTaskManager)
    mock_task_manager.track = AsyncMock()
    mock_task_manager.untrack = AsyncMock()

    user_settings = G8eeUserSettings(llm=LLMSettings())

    # Call run_chat - should catch exception and publish ITERATION_FAILED
    await svc.run_chat(
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
        _task_manager=mock_task_manager,
        user_settings=user_settings,
        _track_task=True,
    )

    # Verify ITERATION_FAILED was published
    events = cast(FakeEventService, svc.event_service).published
    failed_events = [
        e
        for e in events
        if e.investigation_id == "inv-1" and e.event_type == EventType.AI_LLM_CHAT_ITERATION_FAILED
    ]

    assert len(failed_events) == 1

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

    svc = _make_pipeline()
    g8e_ctx = build_g8e_http_context(investigation_id="inv-1", web_session_id="web-1")
    inputs, _state = _make_chat_context(triage_result=LOW_CONFIDENCE_TRIAGE_RESULT)
    svc._prepare_chat_context = AsyncMock(return_value=inputs)

    captured: dict = {}

    def _capture(llm_settings, is_assistant=False, is_lite=False):
        captured["llm"] = llm_settings
        captured["is_assistant"] = is_assistant
        captured["is_lite"] = is_lite
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
            llm_lite_provider="ollama",
            llm_primary_model="main-model",
            llm_assistant_model="assistant-model",
            llm_lite_model="lite-model",
            user_settings=user_settings,
        )

    resolved_llm: LLMSettings = captured["llm"]
    assert isinstance(resolved_llm.primary_provider, LLMProvider)
    assert resolved_llm.primary_provider is LLMProvider.OPENAI
    assert isinstance(resolved_llm.assistant_provider, LLMProvider)
    assert resolved_llm.assistant_provider is LLMProvider.ANTHROPIC


async def test_prepare_chat_context_passes_lite_model_to_triage():
    """Regression: triage runs on the lite provider, so model_override
    must be the lite model - not the primary model.

    Previously chat_pipeline passed llm_primary_model as the triage override,
    causing cross-provider mismatches (e.g. a Claude model name sent to the
    Gemini API endpoint, producing a 404 NOT_FOUND on generateContent).
    With the lite tier wiring, triage now uses the lite model.
    """

    svc = _make_pipeline()
    svc.investigation_service.get_investigation_context = AsyncMock(
        return_value=build_enriched_context(investigation_id="inv-1")
    )
    svc.investigation_service.get_enriched_investigation_context = AsyncMock(
        return_value=build_enriched_context(investigation_id="inv-1")
    )
    svc.investigation_service.investigation_data_service.update_investigation_raw = AsyncMock()
    svc.investigation_service.investigation_data_service.get_chat_messages = AsyncMock(
        return_value=[]
    )
    svc.memory_service = MagicMock()
    svc.memory_service.get_user_memories = AsyncMock(return_value=[])
    svc.memory_service.get_case_memories = AsyncMock(return_value=[])
    svc.request_builder = MagicMock()
    svc.request_builder.build_system_prompt = MagicMock(return_value="")
    svc.request_builder.format_attachment_parts = MagicMock(return_value=[])
    svc.request_builder.build_contents_from_history = MagicMock(
        return_value=BuiltContents(contents=[], scrubbing_observations=[])
    )

    svc.request_builder.get_generation_config = MagicMock(return_value=PrimaryLLMSettings())

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

    svc.triage_agent.triage = AsyncMock(side_effect=_capture_triage)

    g8e_ctx = build_g8e_http_context(
        investigation_id="inv-1", case_id="case-1", web_session_id="web-1", user_id="user-1"
    )
    request_settings = G8eeUserSettings(llm=LLMSettings())

    with patch("app.services.ai.chat_pipeline.resolve_model", return_value="main-model"):
        model_overrides = ModelOverrideResolver(
            primary_model="claude-opus-4-6",
            assistant_model="gemini-3-flash-preview",
            lite_model="gemma3:1b",
        )
        await svc._prepare_chat_context(
            message="hello",
            g8e_context=g8e_ctx,
            request_settings=request_settings,
            attachments=[],
            sentinel_mode=True,
            model_overrides=model_overrides,
        )

    assert captured["model_override"] == "gemma3:1b"


def _pipeline_for_prepare_chat_context() -> ChatPipelineService:
    """A pipeline whose collaborators are stubbed just far enough for
    ``_prepare_chat_context`` to reach ``request_builder.get_generation_config``."""

    svc = _make_pipeline()
    svc.investigation_service.get_investigation_context = AsyncMock(
        return_value=build_enriched_context(investigation_id="inv-1")
    )
    svc.investigation_service.get_enriched_investigation_context = AsyncMock(
        return_value=build_enriched_context(investigation_id="inv-1")
    )
    svc.investigation_service.investigation_data_service.update_investigation_raw = AsyncMock()
    svc.investigation_service.investigation_data_service.get_chat_messages = AsyncMock(
        return_value=[]
    )
    svc.memory_service = MagicMock()
    svc.memory_service.get_user_memories = AsyncMock(return_value=[])
    svc.memory_service.get_case_memories = AsyncMock(return_value=[])
    svc.request_builder = MagicMock()
    svc.request_builder.format_attachment_parts = MagicMock(return_value=[])
    svc.request_builder.build_contents_from_history = MagicMock(
        return_value=BuiltContents(contents=[], scrubbing_observations=[])
    )
    svc.request_builder.get_generation_config = MagicMock(return_value=PrimaryLLMSettings())
    svc.triage_agent.triage = AsyncMock(
        return_value=TriageResult(
            complexity=TriageComplexityClassification.COMPLEX,
            complexity_confidence=TriageConfidence.HIGH,
            intent=TriageIntentClassification.INFORMATION,
            intent_confidence=TriageConfidence.HIGH,
            intent_summary="ok",
        )
    )
    return svc


async def _prepare(svc: ChatPipelineService, g8e_ctx) -> AgentInputs:

    with patch("app.services.ai.chat_pipeline.resolve_model", return_value="qwen3.5:4b"):
        return await svc._prepare_chat_context(
            message="hello",
            g8e_context=g8e_ctx,
            request_settings=G8eeUserSettings(llm=LLMSettings()),
            attachments=[],
            sentinel_mode=True,
            model_overrides=ModelOverrideResolver(
                primary_model="qwen3.5:4b",
                assistant_model="qwen3.5:4b",
                lite_model="qwen3.5:4b",
            ),
        )


async def test_prepare_chat_context_hands_the_evaluation_context_to_generation_config():
    """The scored request's context is what lifts the tool gate (INV-EVAL-CAMP-07)."""

    evaluation_context = EvaluationInferenceContext(
        campaign_id="campaign-1",
        run_id="run-1",
        assignment_id="assignment-1",
        evaluation_attempt_id="attempt-1",
        scenario_id="tool-select-grep",
        model_registry_digest="d" * 64,
        model_registry=[InferenceModelVariant(model="qwen3.5:4b", digest="a" * 64)],
        target_operator_session_id="session-1",
    )
    svc = _pipeline_for_prepare_chat_context()
    g8e_ctx = build_g8e_http_context(
        investigation_id="inv-1", case_id="case-1", web_session_id="web-1", user_id="user-1"
    ).model_copy(update={"evaluation_context": evaluation_context})

    await _prepare(svc, g8e_ctx)

    kwargs = cast(MagicMock, svc.request_builder.get_generation_config).call_args.kwargs
    assert kwargs["evaluation_context"] is evaluation_context
    assert kwargs["model_override"] == "qwen3.5:4b"


async def test_prepare_chat_context_production_request_carries_no_evaluation_context():
    svc = _pipeline_for_prepare_chat_context()
    g8e_ctx = build_g8e_http_context(
        investigation_id="inv-1", case_id="case-1", web_session_id="web-1", user_id="user-1"
    )

    await _prepare(svc, g8e_ctx)

    assert cast(MagicMock, svc.request_builder.get_generation_config).call_args.kwargs["evaluation_context"] is None


@pytest.mark.parametrize(
    ("provider", "expected_budget"),
    [
        ("ollama", 32768),
        ("g8e", 32768),
        ("gemini", None),
        ("anthropic", None),
        (None, 32768),
    ],
)
async def test_prepare_chat_context_budgets_history_only_for_ollama_backed_providers(
    provider, expected_budget
):
    """Only Ollama-backed providers send num_ctx=65536; others keep untrimmed history."""

    svc = _pipeline_for_prepare_chat_context()
    g8e_ctx = build_g8e_http_context(
        investigation_id="inv-1", case_id="case-1", web_session_id="web-1", user_id="user-1"
    )
    llm = LLMSettings() if provider is None else LLMSettings(llm_primary_provider=provider)

    with patch("app.services.ai.chat_pipeline.resolve_model", return_value="qwen3.5:4b"):
        await svc._prepare_chat_context(
            message="hello",
            g8e_context=g8e_ctx,
            request_settings=G8eeUserSettings(llm=llm),
            attachments=[],
            sentinel_mode=True,
            model_overrides=ModelOverrideResolver(
                primary_model="qwen3.5:4b",
                assistant_model="qwen3.5:4b",
                lite_model="qwen3.5:4b",
            ),
        )

    kwargs = cast(MagicMock, svc.request_builder.build_contents_from_history).call_args.kwargs
    assert kwargs["history_token_budget"] == expected_budget


def _scored_context():

    return build_g8e_http_context(
        investigation_id="inv-1", case_id="case-1", web_session_id="web-1", user_id="user-1"
    ).model_copy(
        update={
            "evaluation_context": EvaluationInferenceContext(
                campaign_id="campaign-1",
                run_id="run-1",
                assignment_id="assignment-1",
                evaluation_attempt_id="attempt-1",
                scenario_id="tool-select-grep",
                model_registry_digest="d" * 64,
                model_registry=[InferenceModelVariant(model="qwen3.5:4b", digest="a" * 64)],
                target_operator_session_id="session-1",
            )
        }
    )


async def test_prepare_chat_context_scored_request_reads_case_memories_but_not_user_memories():
    """User-wide memories are artifacts of other assignments and would leak one
    scenario into the next; case memories (which the seed may have written) are
    still read. The suppression is recorded on the inputs for the trace."""

    case_memory = InvestigationMemory(
        case_id="case-1",
        investigation_id="inv-1",
        user_id="test-user-id",
        status=InvestigationStatus.OPEN,
        case_title="Checkout payment timeouts",
        investigation_summary="Customers see timeouts at checkout.",
    )
    svc = _pipeline_for_prepare_chat_context()
    svc.memory_service.get_case_memories = AsyncMock(return_value=[case_memory])

    inputs = await _prepare(svc, _scored_context())

    cast(AsyncMock, svc.memory_service.get_user_memories).assert_not_awaited()
    cast(AsyncMock, svc.memory_service.get_case_memories).assert_awaited_once()
    assert inputs.case_memories == [case_memory]
    assert inputs.user_memories == []
    assert inputs.user_memories_suppressed is True


async def test_prepare_chat_context_production_request_still_reads_user_memories():
    """Characterization: only a request carrying an evaluation_context skips the
    user-wide memory read."""
    svc = _pipeline_for_prepare_chat_context()
    g8e_ctx = build_g8e_http_context(
        investigation_id="inv-1", case_id="case-1", web_session_id="web-1", user_id="user-1"
    )

    inputs = await _prepare(svc, g8e_ctx)

    cast(AsyncMock, svc.memory_service.get_user_memories).assert_awaited_once()
    cast(AsyncMock, svc.memory_service.get_case_memories).assert_awaited_once()
    assert inputs.user_memories_suppressed is False


async def test_run_chat_impl_rejects_unknown_provider_override():
    """An unknown provider override surfaces as a ValueError - not a
    silent bad-value in an enum-typed field."""

    svc = _make_pipeline()
    g8e_ctx = build_g8e_http_context(investigation_id="inv-1", web_session_id="web-1")
    inputs, _state = _make_chat_context(triage_result=LOW_CONFIDENCE_TRIAGE_RESULT)
    svc._prepare_chat_context = AsyncMock(return_value=inputs)

    user_settings = G8eeUserSettings(llm=LLMSettings())
    with (
        patch("app.services.ai.chat_pipeline.get_llm_provider"),
        pytest.raises(ValueError, match="not-a-real-provider"),
    ):
        await svc._run_chat_impl(
            message="hello",
            g8e_context=g8e_ctx,
            attachments=[],
            sentinel_mode=True,
            llm_primary_provider="not-a-real-provider",
            llm_assistant_provider=None,
            llm_lite_provider=None,
            llm_primary_model="main-model",
            llm_assistant_model="assistant-model",
            llm_lite_model="lite-model",
            user_settings=user_settings,
        )


@pytest.mark.parametrize(
    ("active_agent", "expected_assistant", "expected_lite"),
    [
        (ReasoningAgent.DASH, True, False),
        (ReasoningAgent.SAGE, False, False),
    ],
)
async def test_run_chat_impl_selects_provider_of_scored_model_role(
    active_agent, expected_assistant, expected_lite
):
    """Regression: the scored turn's provider is the provider of the role the
    turn runs as. Dash runs as Assistant, so a simple turn must use the
    Assistant provider. Taking the Lite provider sent the Assistant model
    (here a Gemini model) to the Lite backend (Ollama), and with governed
    inference sent role Lite with the Assistant model, which the Inference
    Operator rejects.
    """

    svc = _make_pipeline()
    g8e_ctx = build_g8e_http_context(investigation_id="inv-1", web_session_id="web-1")

    # Create triage result with SIMPLE complexity (should use lite provider)
    simple_triage_result = TriageResult(
        complexity=TriageComplexityClassification.SIMPLE,
        complexity_confidence=TriageConfidence.HIGH,
        intent=TriageIntentClassification.INFORMATION,
        intent_confidence=TriageConfidence.HIGH,
        intent_summary="ok",
    )
    inputs, _state = _make_chat_context(triage_result=simple_triage_result)
    inputs = inputs.model_copy(update={"active_agent": active_agent})
    svc._prepare_chat_context = AsyncMock(return_value=inputs)

    captured: dict = {}

    def _capture(llm_settings, is_assistant=False, is_lite=False):
        captured["llm"] = llm_settings
        captured["is_assistant"] = is_assistant
        captured["is_lite"] = is_lite
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
            llm_lite_provider="ollama",
            llm_primary_model="claude-opus-4-6",
            llm_assistant_model="gemini-3-flash-preview",
            llm_lite_model="gemma3:1b",
            user_settings=user_settings,
        )

    assert captured["is_assistant"] is expected_assistant
    assert captured["is_lite"] is expected_lite


async def test_run_chat_impl_grades_with_settings_before_request_overrides():
    """Regression: the semantic judge runs on the caller's settings before
    request overrides. A campaign's overrides bind the scored model; grading
    with them made the scored model its own judge and, on a shared Inference
    Operator, sent a non-bound Lite model without campaign authority (403)."""

    svc = _make_pipeline()
    g8e_ctx = build_g8e_http_context(investigation_id="inv-1", web_session_id="web-1")
    inputs, _state = _make_chat_context(triage_result=LOW_CONFIDENCE_TRIAGE_RESULT)
    svc._prepare_chat_context = AsyncMock(return_value=inputs)
    svc._finalize_evaluation_assignment = AsyncMock()

    user_settings = G8eeUserSettings(
        llm=LLMSettings(llm_primary_provider=LLMProvider.G8E, llm_lite_model="operator-lite")
    )
    with patch("app.services.ai.chat_pipeline.get_llm_provider", return_value=MagicMock()):
        await svc._run_chat_impl(
            message="hello",
            g8e_context=g8e_ctx,
            attachments=[],
            sentinel_mode=True,
            llm_primary_provider=None,
            llm_assistant_provider=None,
            llm_lite_provider=None,
            llm_primary_model="campaign-model",
            llm_assistant_model="campaign-model",
            llm_lite_model="campaign-model",
            user_settings=user_settings,
        )

    judge_settings = svc._finalize_evaluation_assignment.call_args.kwargs["judge_settings"]
    assert judge_settings is user_settings
    assert judge_settings.llm.resolved_lite_model == "operator-lite"
