# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software
# is released under the Apache License, Version 2.0.

"""Unit tests for the G8E governed-dispatch LLM provider.

Covers ChatPipelineService.validate_llm_config acceptance of the ``g8e``
provider, factory wiring (including the missing-client fail-closed path),
per-role dispatch, prompt conversion, usage metadata, context propagation,
unsupported-content rejection, and provider failure propagation. All tests
stub the InternalHttpClient; no network or gateway is contacted.
"""

from __future__ import annotations

from unittest.mock import AsyncMock, MagicMock

import pytest

from app.constants import LLMProvider, ThinkingLevel
from app.errors import ConfigurationError, ModelCapabilityError, NetworkError, ValidationError
from app.llm.factory import (
    clear_provider_cache,
    get_llm_provider,
    reset_settings,
    set_internal_http_client,
)
from app.llm.llm_dataclasses import (
    Content,
    InlineData,
    Part,
    ResponseFormat,
    ToolCall,
    ToolCallingConfig,
    ToolConfig,
    ToolDeclaration,
    ToolGroup,
    ToolResponse,
)
from app.llm.llm_types import (
    AssistantLLMSettings,
    LiteLLMSettings,
    PrimaryLLMSettings,
    ThinkingConfig,
)
from app.llm.providers.g8e import G8EProvider, _contents_to_messages
from app.models.http_context import G8eHttpContext
from app.models.internal_api import InferenceDispatchResponse
from g8e.models.internal_api import EvaluationInferenceContext, InferenceModelVariant
from app.models.settings import G8eeUserSettings, LLMSettings
from g8e.operator.v1.operator_pb2 import (
    EXECUTION_STATUS_COMPLETED,
    INFERENCE_MESSAGE_ROLE_ASSISTANT,
    INFERENCE_MESSAGE_ROLE_SYSTEM,
    INFERENCE_MESSAGE_ROLE_TOOL,
    INFERENCE_MESSAGE_ROLE_USER,
    INFERENCE_TOOL_CHOICE_MODE_AUTO,
    MODEL_ROLE_ASSISTANT,
    MODEL_ROLE_LITE,
    MODEL_ROLE_PRIMARY,
)

pytestmark = pytest.mark.unit


def _response(text: str = "generated output") -> InferenceDispatchResponse:
    resp = InferenceDispatchResponse(transaction_id="tx-test-001")
    if text:
        resp.result.parts.add().text = text
    resp.result.prompt_tokens = 7
    resp.result.completion_tokens = 11
    resp.result.total_tokens = 18
    resp.result.usage_reported = True
    resp.result.load_duration_ns = 2_000_000
    resp.result.prompt_eval_duration_ns = 10_000_000
    resp.result.generation_duration_ns = 40_000_000
    resp.result.total_duration_ns = 52_000_000
    resp.result.finish_reason = "stop"
    resp.result.model = "gemma3:4b"
    resp.result.requested_model = "gemma3:4b"
    resp.result.normalized_request_hash = "12" * 32
    resp.result.output_hash = "34" * 32
    resp.result.result_digest = "cd" * 32
    resp.receipt.transaction_id = "tx-test-001"
    resp.receipt.status = EXECUTION_STATUS_COMPLETED
    resp.receipt.result_summary = "cd" * 32
    resp.receipt.signer_key_id = "warden-key"
    resp.receipt.signature = "ab" * 64
    return resp


def _tool_call_response() -> InferenceDispatchResponse:
    resp = _response("")
    tool_call = resp.result.parts.add().tool_call
    tool_call.call_id = "call-2"
    tool_call.name = "inspect"
    tool_call.arguments_json = '{"path":"target.txt"}'
    resp.result.finish_reason = "tool_calls"
    return resp


def _dispatch_response(response: InferenceDispatchResponse | None = None):
    async def dispatch(request):
        result = response or _response()
        result.result.provider_attempt_id = request.provider_attempt_id
        result.result.requested_model = request.model
        result.result.model = request.model
        result.result.requested_model_digest = request.model_digest
        result.result.served_model_digest = request.model_digest
        result.result.campaign_id = request.campaign_id
        result.result.run_id = request.run_id
        result.result.assignment_id = request.assignment_id
        result.result.evaluation_attempt_id = request.evaluation_attempt_id
        result.result.scenario_id = request.scenario_id
        result.result.model_registry_digest = request.model_registry_digest
        return result

    return dispatch


def _client() -> MagicMock:
    client = MagicMock()
    client.dispatch_inference = AsyncMock(side_effect=_dispatch_response())
    return client


def _contents() -> list[Content]:
    return [
        Content(role="user", parts=[Part(text="first question")]),
        Content(
            role="model",
            parts=[
                Part(text="first answer"),
                Part(tool_call=ToolCall(name="inspect", args={"z": 2, "a": "value"}, id="call-1")),
            ],
        ),
        Content(
            role="tool",
            parts=[
                Part(
                    tool_response=ToolResponse(
                        name="inspect",
                        response={"ok": True, "detail": "done"},
                        id="call-1",
                    )
                )
            ],
        ),
        Content(role="user", parts=[Part(text="follow up")]),
    ]


class TestValidateLLMConfigAcceptsG8E:
    """ChatPipelineService.validate_llm_config must accept the g8e provider."""

    def _pipeline(self):
        from app.services.ai.chat_pipeline import ChatPipelineService

        return ChatPipelineService.__new__(ChatPipelineService)

    def test_g8e_primary_provider_passes_validation(self):
        settings = G8eeUserSettings(
            llm=LLMSettings(
                primary_provider=LLMProvider.G8E,
                primary_model="gemma3:4b",
            )
        )
        # G8E needs no endpoint or API key; validation must not reject the
        # configured role/provider pair.
        self._pipeline().validate_llm_config(settings)

    def test_g8e_provider_per_tier_passes_validation(self):
        settings = G8eeUserSettings(
            llm=LLMSettings(
                primary_provider=LLMProvider.G8E,
                primary_model="gemma3:4b",
                assistant_provider=LLMProvider.G8E,
                assistant_model="gemma3:4b",
                lite_provider=LLMProvider.G8E,
                lite_model="gemma3:4b",
            )
        )
        self._pipeline().validate_llm_config(settings)

    def test_validate_config_returns_no_errors(self):
        assert G8EProvider.validate_config(api_key=None, endpoint=None) == []


class TestG8EProviderFactoryWiring:
    """Verify get_llm_provider wiring for LLMProvider.G8E."""

    async def _reset(self):
        reset_settings()
        await clear_provider_cache()

    @pytest.mark.asyncio
    async def test_factory_returns_g8e_provider_when_client_injected(self):
        await self._reset()
        set_internal_http_client(_client())
        try:
            settings = LLMSettings(
                primary_provider=LLMProvider.G8E,
                primary_model="gemma3:4b",
            )
            provider = get_llm_provider(settings)
            assert isinstance(provider, G8EProvider)
        finally:
            await self._reset()

    @pytest.mark.asyncio
    async def test_factory_fails_closed_without_internal_client(self):
        await self._reset()
        try:
            settings = LLMSettings(
                primary_provider=LLMProvider.G8E,
                primary_model="gemma3:4b",
            )
            with pytest.raises(ConfigurationError):
                get_llm_provider(settings)
        finally:
            await self._reset()

    @pytest.mark.asyncio
    async def test_factory_caches_provider_until_clear(self):
        await self._reset()
        set_internal_http_client(_client())
        try:
            settings = LLMSettings(
                primary_provider=LLMProvider.G8E,
                primary_model="gemma3:4b",
            )
            first = get_llm_provider(settings)
            second = get_llm_provider(settings)
            assert first is second
            await clear_provider_cache()
            third = get_llm_provider(settings)
            assert third is not first
        finally:
            await self._reset()


class TestG8EProviderDispatch:
    """Exercise the provider's governed dispatch against a stub client."""

    @pytest.mark.asyncio
    async def test_primary_dispatches_primary_role(self):
        client = _client()
        provider = G8EProvider(internal_http_client=client)

        resp = await provider.generate_content_primary(
            "gemma3:4b", _contents(), PrimaryLLMSettings(system_instructions="be brief")
        )

        request = client.dispatch_inference.await_args.args[0]
        assert request.role == MODEL_ROLE_PRIMARY
        assert request.model == "gemma3:4b"
        assert request.provider_attempt_id
        assert request.max_tokens == PrimaryLLMSettings().max_output_tokens
        assert resp.candidates[0].content.parts[0].text == "generated output"
        assert resp.usage_metadata.total_token_count == 18
        assert resp.usage_metadata.usage_reported is True
        assert resp.usage_metadata.load_duration_seconds == pytest.approx(0.002)
        assert resp.usage_metadata.prompt_eval_duration_seconds == pytest.approx(0.01)
        assert resp.usage_metadata.eval_duration_seconds == pytest.approx(0.04)
        assert resp.usage_metadata.total_duration_seconds == pytest.approx(0.052)
        assert resp.usage_metadata.time_to_first_token_seconds is None

    @pytest.mark.asyncio
    async def test_usage_availability_distinguishes_unavailable_from_reported_zero(self):
        for usage_reported in (False, True):
            client = _client()
            response = _response()
            response.result.prompt_tokens = 0
            response.result.completion_tokens = 0
            response.result.total_tokens = 0
            response.result.usage_reported = usage_reported
            response.result.ClearField("load_duration_ns")
            response.result.ClearField("prompt_eval_duration_ns")
            response.result.ClearField("generation_duration_ns")
            response.result.ClearField("total_duration_ns")
            client.dispatch_inference = AsyncMock(side_effect=_dispatch_response(response))
            provider = G8EProvider(internal_http_client=client)

            result = await provider.generate_content_primary(
                "gemma3:4b", _contents(), PrimaryLLMSettings()
            )

            assert result.usage_metadata.total_token_count == 0
            assert result.usage_metadata.usage_reported is usage_reported
            assert result.usage_metadata.load_duration_seconds is None

    @pytest.mark.asyncio
    async def test_primary_dispatch_preserves_sampling_and_stop_controls(self):
        client = _client()
        provider = G8EProvider(internal_http_client=client)
        settings = PrimaryLLMSettings(
            max_output_tokens=321,
            top_p_nucleus_sampling=0.75,
            top_k_filtering=42,
            stop_sequences=["END", "STOP"],
        )

        await provider.generate_content_primary("gemma3:4b", _contents(), settings)

        request = client.dispatch_inference.await_args.args[0]
        assert request.request_schema_version == "1.0"
        assert request.HasField("top_p")
        assert request.top_p == pytest.approx(0.75)
        assert request.HasField("top_k")
        assert request.top_k == 42
        assert list(request.stop_sequences) == ["END", "STOP"]

    @pytest.mark.asyncio
    async def test_primary_dispatch_preserves_tool_parallel_thinking_and_context_controls(self):
        client = _client()
        provider = G8EProvider(internal_http_client=client)
        settings = PrimaryLLMSettings(
            thinking_config=ThinkingConfig(
                thinking_level=ThinkingLevel.HIGH,
                include_thoughts=True,
            ),
            tool_config=ToolConfig(
                tool_calling_config=ToolCallingConfig(
                    mode="AUTO",
                    allowed_tool_names=["inspect"],
                )
            ),
            parallel_tool_calls=False,
            tools=[
                ToolGroup(
                    tools=[
                        ToolDeclaration(
                            name="inspect",
                            description="Inspect a target",
                            parameters={"type": "object"},
                        )
                    ]
                )
            ],
        )

        await provider.generate_content_primary("qwen3.5:2b", _contents(), settings)

        request = client.dispatch_inference.await_args.args[0]
        assert request.tool_choice.mode == INFERENCE_TOOL_CHOICE_MODE_AUTO
        assert list(request.tool_choice.allowed_tool_names) == ["inspect"]
        assert request.HasField("parallel_tool_calls")
        assert request.parallel_tool_calls is False
        assert request.thinking.WhichOneof("mode") == "enabled"
        assert request.thinking.enabled is True
        assert request.thinking.include_thoughts is True
        assert request.HasField("context_limit")
        assert request.context_limit > 0

    @pytest.mark.asyncio
    async def test_assistant_dispatch_preserves_structured_response_format(self):
        client = _client()
        provider = G8EProvider(internal_http_client=client)
        settings = AssistantLLMSettings(
            response_format=ResponseFormat.from_pydantic_schema(
                {
                    "additionalProperties": False,
                    "properties": {"answer": {"type": "string"}},
                    "required": ["answer"],
                    "type": "object",
                },
                name="answer",
            )
        )

        await provider.generate_content_assistant("gemma3:4b", _contents(), settings)

        request = client.dispatch_inference.await_args.args[0]
        assert request.response_format.media_type == "application/json"
        assert request.response_format.json_schema == (
            '{"additionalProperties":false,"properties":{"answer":{"type":"string"}},'
            '"required":["answer"],"type":"object"}'
        )

    @pytest.mark.asyncio
    async def test_tool_only_result_normalizes_without_empty_response_failure(self):
        client = _client()
        client.dispatch_inference = AsyncMock(side_effect=_dispatch_response(_tool_call_response()))
        provider = G8EProvider(internal_http_client=client)

        resp = await provider.generate_content_primary(
            "gemma3:4b", _contents(), PrimaryLLMSettings()
        )

        assert resp.text is None
        assert len(resp.tool_calls) == 1
        assert resp.tool_calls[0] == ToolCall(
            name="inspect", args={"path": "target.txt"}, id="call-2"
        )
        assert resp.candidates[0].finish_reason == "tool_calls"

    @pytest.mark.asyncio
    async def test_assistant_dispatches_assistant_role(self):
        client = _client()
        provider = G8EProvider(internal_http_client=client)

        await provider.generate_content_assistant("gemma3:4b", _contents(), AssistantLLMSettings())

        request = client.dispatch_inference.await_args.args[0]
        assert request.role == MODEL_ROLE_ASSISTANT

    @pytest.mark.asyncio
    async def test_lite_dispatches_lite_role(self):
        client = _client()
        provider = G8EProvider(internal_http_client=client)

        await provider.generate_content_lite("gemma3:4b", _contents(), LiteLLMSettings())

        request = client.dispatch_inference.await_args.args[0]
        assert request.role == MODEL_ROLE_LITE

    @pytest.mark.asyncio
    async def test_empty_model_sends_empty_string(self):
        client = _client()
        provider = G8EProvider(internal_http_client=client)

        await provider.generate_content_lite("", _contents(), LiteLLMSettings())

        request = client.dispatch_inference.await_args.args[0]
        assert request.model == ""

    @pytest.mark.asyncio
    async def test_dispatch_preserves_ordered_typed_conversation_and_tools(self):
        client = _client()
        provider = G8EProvider(internal_http_client=client)
        settings = PrimaryLLMSettings(
            system_instructions="be brief",
            tools=[
                ToolGroup(
                    tools=[
                        ToolDeclaration(
                            name="inspect",
                            description="Inspect a target",
                            parameters={
                                "required": ["path"],
                                "properties": {"path": {"type": "string"}},
                                "type": "object",
                            },
                        )
                    ]
                )
            ],
        )

        await provider.generate_content_primary("gemma3:4b", _contents(), settings)

        request = client.dispatch_inference.await_args.args[0]
        assert [message.role for message in request.messages] == [
            INFERENCE_MESSAGE_ROLE_SYSTEM,
            INFERENCE_MESSAGE_ROLE_USER,
            INFERENCE_MESSAGE_ROLE_ASSISTANT,
            INFERENCE_MESSAGE_ROLE_TOOL,
            INFERENCE_MESSAGE_ROLE_USER,
        ]
        assert request.messages[0].parts[0].text == "be brief"
        assert request.messages[1].parts[0].text == "first question"
        assert request.messages[2].parts[0].text == "first answer"
        tool_call = request.messages[2].parts[1].tool_call
        assert (tool_call.call_id, tool_call.name, tool_call.arguments_json) == (
            "call-1",
            "inspect",
            '{"a":"value","z":2}',
        )
        tool_result = request.messages[3].parts[0].tool_result
        assert (tool_result.call_id, tool_result.name, tool_result.result_json) == (
            "call-1",
            "inspect",
            '{"detail":"done","ok":true}',
        )
        assert request.messages[4].parts[0].text == "follow up"
        assert len(request.tools) == 1
        assert request.tools[0].name == "inspect"
        assert request.tools[0].description == "Inspect a target"
        assert request.tools[0].json_schema == (
            '{"properties":{"path":{"type":"string"}},"required":["path"],"type":"object"}'
        )

    @pytest.mark.asyncio
    async def test_stream_primary_yields_single_text_chunk_then_finish(self):
        client = _client()
        provider = G8EProvider(internal_http_client=client)

        chunks = [
            chunk
            async for chunk in provider.generate_content_stream_primary(
                "gemma3:4b", _contents(), PrimaryLLMSettings()
            )
        ]

        assert chunks[0].text == "generated output"
        assert chunks[-1].finish_reason == "stop"
        assert chunks[-1].usage_metadata.total_token_count == 18

    @pytest.mark.asyncio
    async def test_response_provider_attempt_mismatch_fails_closed(self):
        client = _client()
        response = _response()
        response.result.provider_attempt_id = "different-attempt"
        client.dispatch_inference = AsyncMock(return_value=response)
        provider = G8EProvider(internal_http_client=client)

        with pytest.raises(ValidationError):
            await provider.generate_content_primary("gemma3:4b", _contents(), PrimaryLLMSettings())

        assert provider.governed_dispatch_evidence is None

    @pytest.mark.asyncio
    async def test_dispatch_failure_propagates(self):
        client = _client()
        client.dispatch_inference = AsyncMock(
            side_effect=NetworkError("[HTTP-CLIENT] Inference dispatch returned HTTP 502")
        )
        provider = G8EProvider(internal_http_client=client)

        with pytest.raises(NetworkError):
            await provider.generate_content_primary("gemma3:4b", _contents(), PrimaryLLMSettings())

    @pytest.mark.asyncio
    async def test_dispatch_failure_clears_prior_governed_evidence(self):
        client = _client()
        provider = G8EProvider(internal_http_client=client)
        await provider.generate_content_primary("gemma3:4b", _contents(), PrimaryLLMSettings())
        assert provider.governed_dispatch_evidence is not None
        client.dispatch_inference = AsyncMock(side_effect=NetworkError("dispatch failed"))

        with pytest.raises(NetworkError):
            await provider.generate_content_primary("gemma3:4b", _contents(), PrimaryLLMSettings())

        assert provider.governed_dispatch_evidence is None

    @pytest.mark.asyncio
    async def test_malformed_response_clears_prior_governed_evidence(self):
        client = _client()
        provider = G8EProvider(internal_http_client=client)
        await provider.generate_content_primary("gemma3:4b", _contents(), PrimaryLLMSettings())
        assert provider.governed_dispatch_evidence is not None
        malformed = _tool_call_response()
        malformed.result.parts[0].tool_call.arguments_json = '{"path":"a","path":"b"}'
        client.dispatch_inference = AsyncMock(side_effect=_dispatch_response(malformed))

        with pytest.raises(ValidationError):
            await provider.generate_content_primary("gemma3:4b", _contents(), PrimaryLLMSettings())

        assert provider.governed_dispatch_evidence is None

    @pytest.mark.asyncio
    async def test_cancellation_propagates(self):
        client = _client()
        client.dispatch_inference = AsyncMock(side_effect=TimeoutError())
        provider = G8EProvider(internal_http_client=client)

        with pytest.raises(TimeoutError):
            await provider.generate_content_primary("gemma3:4b", _contents(), PrimaryLLMSettings())

    @pytest.mark.asyncio
    async def test_g8e_context_is_propagated_to_dispatch_request(self):
        client = _client()
        provider = G8EProvider(internal_http_client=client)
        context = G8eHttpContext(
            web_session_id="web-1",
            cli_session_id="cli-1",
            user_id="user-1",
            case_id="case-1",
            investigation_id="inv-1",
            task_id="task-1",
        )

        provider.set_g8e_context(context)
        await provider.generate_content_primary("gemma3:4b", _contents(), PrimaryLLMSettings())

        request = client.dispatch_inference.await_args.args[0]
        assert request.case_id == "case-1"
        assert request.investigation_id == "inv-1"
        assert request.task_id == "task-1"
        assert request.web_session_id == "web-1"
        assert request.cli_session_id == "cli-1"

    @pytest.mark.asyncio
    async def test_evaluation_context_propagates_frozen_registry_and_model_digest(self):
        client = _client()
        provider = G8EProvider(internal_http_client=client)
        model_digest = "ab" * 32
        registry_digest = "cd" * 32
        context = G8eHttpContext(
            user_id="user-1",
            web_session_id="web-1",
            evaluation_context=EvaluationInferenceContext(
                campaign_id="campaign-1",
                run_id="run-1",
                assignment_id="assignment-1",
                evaluation_attempt_id="attempt-1",
                scenario_id="scenario-1",
                model_registry_digest=registry_digest,
                model_registry=[InferenceModelVariant(model="gemma3:4b", digest=model_digest)],
                target_operator_session_id="inference-session-1",
            ),
        )

        provider.set_g8e_context(context)
        await provider.generate_content_primary("gemma3:4b", _contents(), PrimaryLLMSettings())

        request = client.dispatch_inference.await_args.args[0]
        assert request.model_digest == model_digest
        assert request.model_registry_digest == registry_digest
        assert request.target_operator_session_id == "inference-session-1"
        assert request.campaign_id == "campaign-1"
        assert request.run_id == "run-1"
        assert request.assignment_id == "assignment-1"
        assert request.evaluation_attempt_id == "attempt-1"
        assert request.scenario_id == "scenario-1"
        assert [(variant.model, variant.digest) for variant in request.model_registry] == [
            ("gemma3:4b", model_digest)
        ]
        evidence = provider.governed_dispatch_evidence
        assert evidence is not None
        assert evidence.campaign_id == "campaign-1"
        assert evidence.assignment_id == "assignment-1"
        assert evidence.model_registry_digest == registry_digest

    @pytest.mark.asyncio
    async def test_missing_context_sends_empty_identities(self):
        client = _client()
        provider = G8EProvider(internal_http_client=client)

        await provider.generate_content_lite("m", _contents(), LiteLLMSettings())

        request = client.dispatch_inference.await_args.args[0]
        assert request.case_id == ""
        assert request.investigation_id == ""


class TestEvaluationInferenceContextValidation:
    def test_duplicate_model_tags_are_rejected(self):
        variant = InferenceModelVariant(model="gemma3:4b", digest="ab" * 32)

        with pytest.raises(ValueError, match="duplicate model tags"):
            EvaluationInferenceContext(
                campaign_id="campaign-1",
                run_id="run-1",
                assignment_id="assignment-1",
                evaluation_attempt_id="attempt-1",
                scenario_id="scenario-1",
                model_registry_digest="cd" * 32,
                model_registry=[variant, variant],
                target_operator_session_id="inference-session-1",
            )


class TestG8EProviderUnsupportedContent:
    def test_inline_data_part_rejected(self):
        contents = [
            Content(
                role="user",
                parts=[Part(inline_data=InlineData(mime_type="image/png", data=b"x"))],
            )
        ]
        with pytest.raises(ModelCapabilityError):
            _contents_to_messages(contents, None)

    @pytest.mark.parametrize(
        "content",
        [
            Content(role="user", parts=[]),
            Content(
                role="user", parts=[Part(text="x", tool_call=ToolCall(name="inspect", args={}))]
            ),
            Content(role="user", parts=[Part(tool_call=ToolCall(name="inspect", args={}))]),
            Content(role="tool", parts=[Part(text="not a tool result")]),
        ],
    )
    def test_invalid_part_shape_or_role_rejected(self, content: Content):
        with pytest.raises(ValidationError):
            _contents_to_messages([content], None)
