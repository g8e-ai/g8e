# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
Unit tests for OllamaProvider.

Tests SSL verification strategy, close behavior, construction, and content generation.
"""

from contextvars import copy_context
from types import SimpleNamespace
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from app.constants import LLM_OLLAMA_DEFAULT_NUM_CTX, ThinkingLevel
from app.errors import ContextWindowExceededError, OllamaEmptyResponseError
from app.llm.llm_types import (
    AssistantLLMSettings,
    Content,
    LiteLLMSettings,
    Part,
    PrimaryLLMSettings,
    ResponseFormat,
    ResponseJsonSchema,
    ThinkingConfig,
    ToolCallingConfig,
    ToolConfig,
)
from app.llm.model_evidence import model_boundary_hash
from app.llm.providers.ollama import OllamaProvider, _prompt_filled_context
from app.models.model_telemetry import ModelResponseArtifact

PATCH_TARGET = "app.llm.providers.ollama.AsyncClient"

pytestmark = [pytest.mark.unit]


class TestOllamaProviderClose:
    """Test that OllamaProvider properly closes its SDK client."""

    @pytest.mark.asyncio
    async def test_close_calls_close_on_client(self):
        mock_sdk_client = AsyncMock()
        with patch(PATCH_TARGET, return_value=mock_sdk_client):
            provider = OllamaProvider(
                endpoint="https://localhost:11434",
                api_key="test-key",
            )
            await provider.close()
            mock_sdk_client.close.assert_called_once()


class TestOllamaProviderConstruction:
    """Test OllamaProvider construction and initialization."""

    def test_constructor_creates_sdk_client(self):
        mock_client = MagicMock()
        with patch(PATCH_TARGET, return_value=mock_client) as mock_ctor:
            provider = OllamaProvider(
                endpoint="http://localhost:11434",
                api_key="test-key",
            )
            mock_ctor.assert_called_once_with(host="http://localhost:11434")
            assert provider._client is mock_client

    def test_constructor_strips_trailing_slash(self):
        mock_client = MagicMock()
        with patch(PATCH_TARGET, return_value=mock_client) as mock_ctor:
            OllamaProvider(
                endpoint="http://localhost:11434/",
                api_key="test-key",
            )
            mock_ctor.assert_called_once_with(host="http://localhost:11434")

    def test_constructor_rejects_v1_suffix(self):
        """Ollama endpoints must not contain '/v1'; the native API is /api/chat."""
        with pytest.raises(ValueError, match="/v1"):
            OllamaProvider(
                endpoint="http://localhost:11434/v1",
                api_key="test-key",
            )

    def test_constructor_adds_http_prefix(self):
        mock_client = MagicMock()
        with patch(PATCH_TARGET, return_value=mock_client) as mock_ctor:
            OllamaProvider(
                endpoint="localhost:11434",
                api_key="test-key",
            )
            mock_ctor.assert_called_once_with(host="http://localhost:11434")

    def test_constructor_rejects_double_v1_suffix(self):
        with pytest.raises(ValueError, match="/v1"):
            OllamaProvider(
                endpoint="http://192.168.1.2:11434/v1/v1",
                api_key="test-key",
            )

    def test_constructor_accepts_bare_ip_port(self):
        mock_client = MagicMock()
        with patch(PATCH_TARGET, return_value=mock_client) as mock_ctor:
            OllamaProvider(
                endpoint="192.168.1.100:11434",
                api_key="test-key",
            )
            mock_ctor.assert_called_once_with(host="http://192.168.1.100:11434")

    @pytest.mark.asyncio
    async def test_context_manager_support(self):
        """Test that OllamaProvider supports async context manager."""
        mock_sdk_client = AsyncMock()
        with patch(PATCH_TARGET, return_value=mock_sdk_client):
            provider = OllamaProvider(
                endpoint="http://localhost:11434",
                api_key="test-key",
            )
            async with provider:
                assert provider is not None
            mock_sdk_client.close.assert_called_once()


class TestOllamaProviderGeneration:
    """Test OllamaProvider generation methods with mocked SDK client."""

    @pytest.fixture
    def provider(self):
        mock_client = MagicMock()
        with patch(PATCH_TARGET, return_value=mock_client):
            provider = OllamaProvider(
                endpoint="http://localhost:11434",
                api_key="test-key",
            )
            yield provider, mock_client

    def test_response_timestamps_do_not_leak_between_copied_contexts(self, provider):
        provider, _ = provider
        provider._response_artifact.set(ModelResponseArtifact())
        first_context = copy_context()
        second_context = copy_context()

        first_context.run(provider._record_response, object())
        second_context.run(provider._record_response, object())

        assert len(first_context.run(provider._response_received_at.get)) == 1
        assert len(second_context.run(provider._response_received_at.get)) == 1

    @pytest.mark.asyncio
    async def test_generate_content_primary_records_exact_scrubbed_outbound_payload(self, provider):
        provider, mock_client = provider

        mock_response = MagicMock()
        mock_response.message.content = "Hello World"
        mock_response.message.thinking = "Thinking..."
        mock_response.message.tool_calls = None
        mock_response.done_reason = "stop"
        mock_response.prompt_eval_count = 10
        mock_response.eval_count = 5
        mock_client.chat = AsyncMock(return_value=mock_response)

        contents = [Content(role="user", parts=[Part(text="[REDACTED_EMAIL]")])]
        settings = PrimaryLLMSettings(
            system_instructions="You are a helpful assistant",
            max_output_tokens=1000,
            top_p_nucleus_sampling=1.0,
            top_k_filtering=40,
            stop_sequences=[],
            response_modalities=["TEXT"],
            tools=[],
            thinking_config=ThinkingConfig(
                thinking_level=ThinkingLevel.OFF, include_thoughts=False
            ),
            tool_config=ToolConfig(tool_calling_config=ToolCallingConfig(mode="AUTO")),
        )

        response = await provider.generate_content_primary("llama3", contents, settings)

        mock_client.chat.assert_called_once()
        call_kwargs = mock_client.chat.call_args.kwargs
        assert call_kwargs["options"]["num_ctx"] == LLM_OLLAMA_DEFAULT_NUM_CTX, (
            "num_ctx must be explicitly set; Ollama's 4096 default silently "
            "truncates real-world prompts and starves thinking models of output budget"
        )
        assert provider.input_artifact_hash == model_boundary_hash(call_kwargs)
        assert provider.model_boundary_privacy is not None
        assert provider.model_boundary_privacy.input_artifact_hash == provider.input_artifact_hash
        assert provider.model_boundary_privacy.raw_sensitive_occurrences == 0
        assert provider.model_boundary_privacy.raw_sensitive_types == []
        unsanitized_kwargs = {
            **call_kwargs,
            "messages": [*call_kwargs["messages"][:-1], {"role": "user", "content": "canary@example.com"}],
        }
        assert "[REDACTED_EMAIL]" in str(call_kwargs)
        assert "canary@example.com" not in str(call_kwargs)
        assert provider.input_artifact_hash != model_boundary_hash(unsanitized_kwargs)
        assert len(response.candidates) == 1
        assert response.candidates[0].content.parts[0].text == "Thinking..."
        assert response.candidates[0].content.parts[1].text == "Hello World"
        assert response.usage_metadata is not None
        assert response.usage_metadata.prompt_token_count == 10
        assert response.usage_metadata.candidates_token_count == 5
        assert response.usage_metadata.cache_token_count is None
        assert response.usage_metadata.usage_reported is True

    @pytest.mark.asyncio
    async def test_generate_content_stream_primary(self, provider):
        provider, mock_client = provider

        mock_chunk1 = MagicMock()
        mock_chunk1.message.content = "Hello"
        mock_chunk1.message.thinking = None
        mock_chunk1.message.tool_calls = None
        mock_chunk1.done = False

        mock_chunk2 = MagicMock()
        mock_chunk2.message.content = " World"
        mock_chunk2.message.thinking = None
        mock_chunk2.message.tool_calls = None
        mock_chunk2.done = True
        mock_chunk2.done_reason = "stop"
        mock_chunk2.prompt_eval_count = 10
        mock_chunk2.eval_count = 5

        async def mock_stream():
            yield mock_chunk1
            yield mock_chunk2

        mock_client.chat = AsyncMock(return_value=mock_stream())

        contents = [Content(role="user", parts=[Part(text="Hi")])]
        settings = PrimaryLLMSettings(
            system_instructions="You are a helpful assistant",
            max_output_tokens=1000,
            top_p_nucleus_sampling=1.0,
            top_k_filtering=40,
            stop_sequences=[],
            response_modalities=["TEXT"],
            tools=[],
            thinking_config=ThinkingConfig(
                thinking_level=ThinkingLevel.OFF, include_thoughts=False
            ),
            tool_config=ToolConfig(tool_calling_config=ToolCallingConfig(mode="AUTO")),
        )

        chunks = []
        async for chunk in provider.generate_content_stream_primary("llama3", contents, settings):
            chunks.append(chunk)

        mock_client.chat.assert_called_once()
        assert mock_client.chat.call_args.kwargs["options"]["num_ctx"] == LLM_OLLAMA_DEFAULT_NUM_CTX
        assert len(chunks) == 3
        assert chunks[0].text == "Hello"
        assert chunks[1].text == " World"
        assert chunks[2].finish_reason == "stop"
        assert chunks[2].usage_metadata is not None
        assert chunks[2].usage_metadata.total_token_count == 15

    @pytest.mark.asyncio
    async def test_generate_content_assistant(self, provider):
        provider, mock_client = provider

        mock_response = MagicMock()
        mock_response.message.content = "Hello World"
        mock_response.done_reason = "stop"
        mock_response.prompt_eval_count = 10
        mock_response.eval_count = 5
        mock_client.chat = AsyncMock(return_value=mock_response)

        contents = [Content(role="user", parts=[Part(text="Hi")])]
        settings = AssistantLLMSettings(
            system_instructions="You are a helpful assistant",
            max_output_tokens=1000,
            top_p_nucleus_sampling=1.0,
            top_k_filtering=40,
            stop_sequences=[],
            response_format=ResponseFormat(
                json_schema=ResponseJsonSchema(json_schema_dict={}, name="response")
            ),
        )

        response = await provider.generate_content_assistant("llama3", contents, settings)

        mock_client.chat.assert_called_once()
        assert mock_client.chat.call_args.kwargs["options"]["num_ctx"] == LLM_OLLAMA_DEFAULT_NUM_CTX
        assert len(response.candidates) == 1
        assert response.candidates[0].content.parts[0].text == "Hello World"
        assert response.usage_metadata is not None

    @pytest.mark.asyncio
    async def test_generate_content_lite(self, provider):
        provider, mock_client = provider

        mock_response = MagicMock()
        mock_response.message.content = "Hello World"
        mock_response.done_reason = "stop"
        mock_response.prompt_eval_count = 10
        mock_response.eval_count = 5
        mock_client.chat = AsyncMock(return_value=mock_response)

        contents = [Content(role="user", parts=[Part(text="Hi")])]
        settings = LiteLLMSettings(
            system_instructions="You are a helpful assistant",
            max_output_tokens=1000,
            top_p_nucleus_sampling=1.0,
            top_k_filtering=40,
            stop_sequences=[],
            response_format=ResponseFormat(
                json_schema=ResponseJsonSchema(json_schema_dict={}, name="response")
            ),
        )

        response = await provider.generate_content_lite("llama3", contents, settings)

        mock_client.chat.assert_called_once()
        assert mock_client.chat.call_args.kwargs["options"]["num_ctx"] == LLM_OLLAMA_DEFAULT_NUM_CTX
        assert len(response.candidates) == 1
        assert response.candidates[0].content.parts[0].text == "Hello World"
        assert response.usage_metadata is not None


class TestOllamaNativeDurations:
    """Ollama native nanosecond durations map to seconds on UsageMetadata.

    Ollama responses and terminal stream chunks carry eval_duration,
    prompt_eval_duration, total_duration, and load_duration in
    nanoseconds. The provider adapter converts them to seconds at the
    boundary. Absent fields stay None — never synthesized. Streaming
    calls additionally stamp time_to_first_token_seconds from first
    content/thinking/tool_calls chunk arrival.
    """

    @pytest.fixture
    def provider(self):
        mock_client = MagicMock()
        with patch(PATCH_TARGET, return_value=mock_client):
            provider = OllamaProvider(
                endpoint="http://localhost:11434",
                api_key="test-key",
            )
            yield provider, mock_client

    @staticmethod
    def _primary_settings() -> PrimaryLLMSettings:
        return PrimaryLLMSettings(
            system_instructions="You are a helpful assistant",
            max_output_tokens=1000,
            top_p_nucleus_sampling=1.0,
            top_k_filtering=40,
            stop_sequences=[],
            response_modalities=["TEXT"],
            tools=[],
            thinking_config=ThinkingConfig(
                thinking_level=ThinkingLevel.OFF, include_thoughts=False
            ),
            tool_config=ToolConfig(tool_calling_config=ToolCallingConfig(mode="AUTO")),
        )

    @staticmethod
    def _set_durations(response, *, prompt_eval_ns, eval_ns, total_ns, load_ns):
        response.prompt_eval_duration = prompt_eval_ns
        response.eval_duration = eval_ns
        response.total_duration = total_ns
        response.load_duration = load_ns

    @pytest.mark.asyncio
    async def test_generate_content_primary_maps_native_durations_to_seconds(self, provider):
        provider, mock_client = provider
        mock_response = MagicMock()
        mock_response.message.content = "Hello World"
        mock_response.message.thinking = None
        mock_response.message.tool_calls = None
        mock_response.done_reason = "stop"
        mock_response.prompt_eval_count = 10
        mock_response.eval_count = 5
        self._set_durations(
            mock_response,
            prompt_eval_ns=10_000_000,
            eval_ns=40_000_000,
            total_ns=52_000_000,
            load_ns=2_000_000,
        )
        mock_client.chat = AsyncMock(return_value=mock_response)

        contents = [Content(role="user", parts=[Part(text="Hi")])]
        response = await provider.generate_content_primary(
            "llama3", contents, self._primary_settings()
        )

        usage = response.usage_metadata
        assert usage.prompt_eval_duration_seconds == 0.01
        assert usage.eval_duration_seconds == 0.04
        assert usage.total_duration_seconds == 0.052
        assert usage.load_duration_seconds == 0.002
        # Non-streaming boundary cannot measure TTFT
        assert usage.time_to_first_token_seconds is None

    @pytest.mark.asyncio
    async def test_generate_content_primary_absent_durations_stay_none(self, provider):
        provider, mock_client = provider
        mock_response = MagicMock()
        mock_response.message.content = "Hello World"
        mock_response.message.thinking = None
        mock_response.message.tool_calls = None
        mock_response.done_reason = "stop"
        mock_response.prompt_eval_count = 10
        mock_response.eval_count = 5
        self._set_durations(
            mock_response, prompt_eval_ns=None, eval_ns=None, total_ns=None, load_ns=None
        )
        mock_client.chat = AsyncMock(return_value=mock_response)

        contents = [Content(role="user", parts=[Part(text="Hi")])]
        response = await provider.generate_content_primary(
            "llama3", contents, self._primary_settings()
        )

        usage = response.usage_metadata
        assert usage.prompt_eval_duration_seconds is None
        assert usage.eval_duration_seconds is None
        assert usage.total_duration_seconds is None
        assert usage.load_duration_seconds is None
        # Token counts still map
        assert usage.prompt_token_count == 10
        assert usage.candidates_token_count == 5
        assert usage.usage_reported is True

    @pytest.mark.asyncio
    async def test_generate_content_assistant_maps_native_durations(self, provider):
        provider, mock_client = provider
        mock_response = MagicMock()
        mock_response.message.content = "Hello World"
        mock_response.done_reason = "stop"
        mock_response.prompt_eval_count = 10
        mock_response.eval_count = 5
        self._set_durations(
            mock_response,
            prompt_eval_ns=5_000_000,
            eval_ns=20_000_000,
            total_ns=30_000_000,
            load_ns=1_000_000,
        )
        mock_client.chat = AsyncMock(return_value=mock_response)

        contents = [Content(role="user", parts=[Part(text="Hi")])]
        settings = AssistantLLMSettings(
            system_instructions="You are a helpful assistant",
            max_output_tokens=1000,
            response_format=ResponseFormat(
                json_schema=ResponseJsonSchema(json_schema_dict={}, name="response")
            ),
        )
        response = await provider.generate_content_assistant("llama3", contents, settings)

        usage = response.usage_metadata
        assert usage.prompt_eval_duration_seconds == 0.005
        assert usage.eval_duration_seconds == 0.02
        assert usage.total_duration_seconds == 0.03
        assert usage.load_duration_seconds == 0.001
        assert usage.time_to_first_token_seconds is None

    @pytest.mark.asyncio
    async def test_generate_content_lite_maps_native_durations(self, provider):
        provider, mock_client = provider
        mock_response = MagicMock()
        mock_response.message.content = "Hello World"
        mock_response.done_reason = "stop"
        mock_response.prompt_eval_count = 10
        mock_response.eval_count = 5
        self._set_durations(
            mock_response,
            prompt_eval_ns=5_000_000,
            eval_ns=20_000_000,
            total_ns=30_000_000,
            load_ns=1_000_000,
        )
        mock_client.chat = AsyncMock(return_value=mock_response)

        contents = [Content(role="user", parts=[Part(text="Hi")])]
        settings = LiteLLMSettings(
            system_instructions="You are a helpful assistant",
            max_output_tokens=1000,
            response_format=ResponseFormat(
                json_schema=ResponseJsonSchema(json_schema_dict={}, name="response")
            ),
        )
        response = await provider.generate_content_lite("llama3", contents, settings)

        usage = response.usage_metadata
        assert usage.prompt_eval_duration_seconds == 0.005
        assert usage.eval_duration_seconds == 0.02
        assert usage.total_duration_seconds == 0.03
        assert usage.load_duration_seconds == 0.001
        assert usage.time_to_first_token_seconds is None

    @pytest.mark.asyncio
    async def test_stream_primary_stamps_ttft_and_maps_durations(self, provider):
        provider, mock_client = provider

        mock_chunk1 = MagicMock()
        mock_chunk1.message.content = "Hello"
        mock_chunk1.message.thinking = None
        mock_chunk1.message.tool_calls = None
        mock_chunk1.done = False

        mock_chunk2 = MagicMock()
        mock_chunk2.message.content = " World"
        mock_chunk2.message.thinking = None
        mock_chunk2.message.tool_calls = None
        mock_chunk2.done = True
        mock_chunk2.done_reason = "stop"
        mock_chunk2.prompt_eval_count = 10
        mock_chunk2.eval_count = 5
        self._set_durations(
            mock_chunk2,
            prompt_eval_ns=10_000_000,
            eval_ns=40_000_000,
            total_ns=52_000_000,
            load_ns=2_000_000,
        )

        async def mock_stream():
            yield mock_chunk1
            yield mock_chunk2

        mock_client.chat = AsyncMock(return_value=mock_stream())

        contents = [Content(role="user", parts=[Part(text="Hi")])]
        chunks = [
            chunk
            async for chunk in provider.generate_content_stream_primary(
                "llama3", contents, self._primary_settings()
            )
        ]

        terminal = chunks[-1]
        usage = terminal.usage_metadata
        assert usage is not None
        assert usage.time_to_first_token_seconds is not None
        assert usage.time_to_first_token_seconds >= 0.0
        assert usage.prompt_eval_duration_seconds == 0.01
        assert usage.eval_duration_seconds == 0.04
        assert usage.total_duration_seconds == 0.052
        assert usage.load_duration_seconds == 0.002
        assert usage.prompt_token_count == 10
        assert usage.candidates_token_count == 5
        assert usage.usage_reported is True

    @pytest.mark.asyncio
    async def test_stream_primary_thinking_chunk_counts_as_first_token(self, provider):
        """A thinking-only first chunk is first-token evidence."""
        provider, mock_client = provider

        mock_chunk1 = MagicMock()
        mock_chunk1.message.content = None
        mock_chunk1.message.thinking = "reasoning..."
        mock_chunk1.message.tool_calls = None
        mock_chunk1.done = False

        mock_chunk2 = MagicMock()
        mock_chunk2.message.content = "answer"
        mock_chunk2.message.thinking = None
        mock_chunk2.message.tool_calls = None
        mock_chunk2.done = True
        mock_chunk2.done_reason = "stop"
        mock_chunk2.prompt_eval_count = 10
        mock_chunk2.eval_count = 5
        self._set_durations(
            mock_chunk2,
            prompt_eval_ns=10_000_000,
            eval_ns=40_000_000,
            total_ns=52_000_000,
            load_ns=2_000_000,
        )

        async def mock_stream():
            yield mock_chunk1
            yield mock_chunk2

        mock_client.chat = AsyncMock(return_value=mock_stream())

        contents = [Content(role="user", parts=[Part(text="Hi")])]
        chunks = [
            chunk
            async for chunk in provider.generate_content_stream_primary(
                "llama3", contents, self._primary_settings()
            )
        ]

        terminal = chunks[-1]
        assert terminal.usage_metadata.time_to_first_token_seconds is not None
        assert terminal.usage_metadata.time_to_first_token_seconds >= 0.0

    @pytest.mark.asyncio
    async def test_stream_primary_tool_call_chunk_counts_as_first_token(self, provider):
        """A tool_calls-only first chunk is first-token evidence."""
        provider, mock_client = provider

        tool_call = MagicMock()
        tool_call.function.name = "run_cmd"
        tool_call.function.arguments = {"cmd": "ls"}

        mock_chunk1 = MagicMock()
        mock_chunk1.message.content = None
        mock_chunk1.message.thinking = None
        mock_chunk1.message.tool_calls = [tool_call]
        mock_chunk1.done = False

        mock_chunk2 = MagicMock()
        mock_chunk2.message.content = None
        mock_chunk2.message.thinking = None
        mock_chunk2.message.tool_calls = None
        mock_chunk2.done = True
        mock_chunk2.done_reason = "stop"
        mock_chunk2.prompt_eval_count = 10
        mock_chunk2.eval_count = 0
        self._set_durations(
            mock_chunk2,
            prompt_eval_ns=10_000_000,
            eval_ns=1_000_000,
            total_ns=12_000_000,
            load_ns=1_000_000,
        )

        async def mock_stream():
            yield mock_chunk1
            yield mock_chunk2

        mock_client.chat = AsyncMock(return_value=mock_stream())

        contents = [Content(role="user", parts=[Part(text="Hi")])]
        chunks = [
            chunk
            async for chunk in provider.generate_content_stream_primary(
                "llama3", contents, self._primary_settings()
            )
        ]

        terminal = chunks[-1]
        assert terminal.usage_metadata.time_to_first_token_seconds is not None
        assert terminal.usage_metadata.time_to_first_token_seconds >= 0.0

    @pytest.mark.asyncio
    async def test_stream_primary_ttft_none_when_no_token_chunks(self, provider):
        """A stream with only a done chunk has no first-token observation."""
        provider, mock_client = provider

        mock_chunk = MagicMock()
        mock_chunk.message.content = None
        mock_chunk.message.thinking = None
        mock_chunk.message.tool_calls = None
        mock_chunk.done = True
        mock_chunk.done_reason = "stop"
        mock_chunk.prompt_eval_count = 10
        mock_chunk.eval_count = 0
        self._set_durations(
            mock_chunk,
            prompt_eval_ns=10_000_000,
            eval_ns=1_000_000,
            total_ns=12_000_000,
            load_ns=1_000_000,
        )

        async def mock_stream():
            yield mock_chunk

        mock_client.chat = AsyncMock(return_value=mock_stream())

        contents = [Content(role="user", parts=[Part(text="Hi")])]
        chunks = [
            chunk
            async for chunk in provider.generate_content_stream_primary(
                "llama3", contents, self._primary_settings()
            )
        ]

        terminal = chunks[-1]
        assert terminal.usage_metadata.time_to_first_token_seconds is None
        assert terminal.usage_metadata.eval_duration_seconds == 0.001

    @pytest.mark.asyncio
    async def test_stream_assistant_stamps_ttft_and_maps_durations(self, provider):
        provider, mock_client = provider

        mock_chunk1 = MagicMock()
        mock_chunk1.message.content = "Hello"
        mock_chunk1.done = False

        mock_chunk2 = MagicMock()
        mock_chunk2.message.content = " World"
        mock_chunk2.done = True
        mock_chunk2.done_reason = "stop"
        mock_chunk2.prompt_eval_count = 10
        mock_chunk2.eval_count = 5
        self._set_durations(
            mock_chunk2,
            prompt_eval_ns=10_000_000,
            eval_ns=40_000_000,
            total_ns=52_000_000,
            load_ns=2_000_000,
        )

        async def mock_stream():
            yield mock_chunk1
            yield mock_chunk2

        mock_client.chat = AsyncMock(return_value=mock_stream())

        contents = [Content(role="user", parts=[Part(text="Hi")])]
        settings = AssistantLLMSettings(
            system_instructions="You are a helpful assistant",
            max_output_tokens=1000,
            response_format=ResponseFormat(
                json_schema=ResponseJsonSchema(json_schema_dict={}, name="response")
            ),
        )
        chunks = [
            chunk
            async for chunk in provider.generate_content_stream_assistant(
                "llama3", contents, settings
            )
        ]

        terminal = chunks[-1]
        assert terminal.usage_metadata.time_to_first_token_seconds is not None
        assert terminal.usage_metadata.time_to_first_token_seconds >= 0.0
        assert terminal.usage_metadata.eval_duration_seconds == 0.04

    @pytest.mark.asyncio
    async def test_stream_lite_stamps_ttft_and_maps_durations(self, provider):
        provider, mock_client = provider

        mock_chunk1 = MagicMock()
        mock_chunk1.message.content = "Hello"
        mock_chunk1.done = False

        mock_chunk2 = MagicMock()
        mock_chunk2.message.content = " World"
        mock_chunk2.done = True
        mock_chunk2.done_reason = "stop"
        mock_chunk2.prompt_eval_count = 10
        mock_chunk2.eval_count = 5
        self._set_durations(
            mock_chunk2,
            prompt_eval_ns=10_000_000,
            eval_ns=40_000_000,
            total_ns=52_000_000,
            load_ns=2_000_000,
        )

        async def mock_stream():
            yield mock_chunk1
            yield mock_chunk2

        mock_client.chat = AsyncMock(return_value=mock_stream())

        contents = [Content(role="user", parts=[Part(text="Hi")])]
        settings = LiteLLMSettings(
            system_instructions="You are a helpful assistant",
            max_output_tokens=1000,
            response_format=ResponseFormat(
                json_schema=ResponseJsonSchema(json_schema_dict={}, name="response")
            ),
        )
        chunks = [
            chunk
            async for chunk in provider.generate_content_stream_lite(
                "llama3", contents, settings
            )
        ]

        terminal = chunks[-1]
        assert terminal.usage_metadata.time_to_first_token_seconds is not None
        assert terminal.usage_metadata.time_to_first_token_seconds >= 0.0
        assert terminal.usage_metadata.eval_duration_seconds == 0.04


class TestOllamaEmptyResponseError:
    """Test that OllamaEmptyResponseError is raised for empty responses."""

    @pytest.fixture
    def provider(self):
        mock_client = MagicMock()
        with patch(PATCH_TARGET, return_value=mock_client):
            provider = OllamaProvider(
                endpoint="http://localhost:11434",
                api_key="test-key",
            )
            yield provider, mock_client

    @pytest.mark.asyncio
    async def test_generate_content_primary_raises_on_empty_content(self, provider):
        provider, mock_client = provider

        mock_response = MagicMock()
        mock_response.message.content = ""
        mock_response.message.thinking = None
        mock_response.done_reason = "length"
        mock_response.prompt_eval_count = 8192
        mock_response.eval_count = 0
        mock_client.chat = AsyncMock(return_value=mock_response)

        contents = [Content(role="user", parts=[Part(text="Hi")])]
        settings = PrimaryLLMSettings(
            system_instructions="You are a helpful assistant",
            max_output_tokens=1000,
            top_p_nucleus_sampling=1.0,
            top_k_filtering=40,
            stop_sequences=[],
            response_modalities=["TEXT"],
            tools=[],
            thinking_config=ThinkingConfig(
                thinking_level=ThinkingLevel.OFF, include_thoughts=False
            ),
            tool_config=ToolConfig(tool_calling_config=ToolCallingConfig(mode="AUTO")),
        )

        with pytest.raises(OllamaEmptyResponseError) as exc_info:
            await provider.generate_content_primary("llama3", contents, settings)

        error = exc_info.value
        assert error.model == "llama3"
        assert error.channel == "primary"
        assert error.done_reason == "length"
        assert error.prompt_eval_count == 8192
        assert error.eval_count == 0
        assert error.ctx_overflow_suspected is False
        assert error.thinking_len == 0
        assert error.tool_calls_count == 0

    @pytest.mark.asyncio
    async def test_generate_content_primary_raises_context_window_exceeded_when_prompt_fills_num_ctx(
        self, provider
    ):
        provider, mock_client = provider

        mock_response = MagicMock()
        mock_response.message.content = ""
        mock_response.message.thinking = None
        mock_response.done_reason = "stop"
        mock_response.prompt_eval_count = LLM_OLLAMA_DEFAULT_NUM_CTX
        mock_response.eval_count = 0
        mock_client.chat = AsyncMock(return_value=mock_response)

        contents = [Content(role="user", parts=[Part(text="Hi")])]
        settings = PrimaryLLMSettings(
            system_instructions="You are a helpful assistant",
            max_output_tokens=1000,
            top_p_nucleus_sampling=1.0,
            top_k_filtering=40,
            stop_sequences=[],
            response_modalities=["TEXT"],
            tools=[],
            thinking_config=ThinkingConfig(
                thinking_level=ThinkingLevel.OFF, include_thoughts=False
            ),
            tool_config=ToolConfig(tool_calling_config=ToolCallingConfig(mode="AUTO")),
        )

        with pytest.raises(ContextWindowExceededError) as exc_info:
            await provider.generate_content_primary("llama3", contents, settings)

        error = exc_info.value
        assert not isinstance(error, OllamaEmptyResponseError)
        assert error.model == "llama3"
        assert error.channel == "primary"
        assert error.num_ctx == LLM_OLLAMA_DEFAULT_NUM_CTX
        assert error.prompt_tokens == LLM_OLLAMA_DEFAULT_NUM_CTX

    @staticmethod
    def _answered_response(prompt_eval_count):
        response = MagicMock()
        response.message.content = "Hello World"
        response.message.thinking = None
        response.message.tool_calls = None
        response.done_reason = "stop"
        response.prompt_eval_count = prompt_eval_count
        response.eval_count = 5
        return response

    @staticmethod
    def _lite_settings():
        return LiteLLMSettings(
            system_instructions="You are a helpful assistant",
            max_output_tokens=1000,
            response_format=ResponseFormat(
                json_schema=ResponseJsonSchema(json_schema_dict={}, name="response")
            ),
        )

    @pytest.mark.asyncio
    @pytest.mark.parametrize("prompt_eval_count", [LLM_OLLAMA_DEFAULT_NUM_CTX, LLM_OLLAMA_DEFAULT_NUM_CTX + 50])
    async def test_generate_content_lite_raises_context_window_exceeded_on_answered_prompt_at_limit(
        self, provider, prompt_eval_count
    ):
        provider, mock_client = provider
        mock_client.chat = AsyncMock(return_value=self._answered_response(prompt_eval_count))
        contents = [Content(role="user", parts=[Part(text="Hi")])]

        with pytest.raises(ContextWindowExceededError) as exc_info:
            await provider.generate_content_lite("llama3", contents, self._lite_settings())

        error = exc_info.value
        assert not isinstance(error, OllamaEmptyResponseError)
        assert error.model == "llama3"
        assert error.channel == "lite"
        assert error.num_ctx == LLM_OLLAMA_DEFAULT_NUM_CTX
        assert error.prompt_tokens == prompt_eval_count

    @pytest.mark.asyncio
    async def test_generate_content_lite_accepts_answered_prompt_below_limit(self, provider):
        provider, mock_client = provider
        mock_client.chat = AsyncMock(
            return_value=self._answered_response(LLM_OLLAMA_DEFAULT_NUM_CTX - 1)
        )
        contents = [Content(role="user", parts=[Part(text="Hi")])]

        response = await provider.generate_content_lite("llama3", contents, self._lite_settings())

        assert response.candidates[0].content.parts[0].text == "Hello World"

    @pytest.mark.asyncio
    async def test_generate_content_lite_accepts_answered_prompt_when_prompt_eval_count_unset(
        self, provider
    ):
        provider, mock_client = provider
        mock_client.chat = AsyncMock(return_value=self._answered_response(None))
        contents = [Content(role="user", parts=[Part(text="Hi")])]

        response = await provider.generate_content_lite("llama3", contents, self._lite_settings())

        assert response.candidates[0].content.parts[0].text == "Hello World"

    @pytest.mark.asyncio
    async def test_generate_content_assistant_raises_context_window_exceeded_on_answered_prompt_at_limit(
        self, provider
    ):
        provider, mock_client = provider
        mock_client.chat = AsyncMock(
            return_value=self._answered_response(LLM_OLLAMA_DEFAULT_NUM_CTX)
        )
        contents = [Content(role="user", parts=[Part(text="Hi")])]
        settings = AssistantLLMSettings(
            system_instructions="You are a helpful assistant",
            max_output_tokens=1000,
            response_format=ResponseFormat(
                json_schema=ResponseJsonSchema(json_schema_dict={}, name="response")
            ),
        )

        with pytest.raises(ContextWindowExceededError) as exc_info:
            await provider.generate_content_assistant("llama3", contents, settings)

        assert exc_info.value.channel == "assistant"
        assert exc_info.value.prompt_tokens == LLM_OLLAMA_DEFAULT_NUM_CTX

    @pytest.mark.asyncio
    async def test_generate_content_primary_raises_context_window_exceeded_on_answered_prompt_at_limit(
        self, provider
    ):
        provider, mock_client = provider
        mock_client.chat = AsyncMock(
            return_value=self._answered_response(LLM_OLLAMA_DEFAULT_NUM_CTX)
        )
        contents = [Content(role="user", parts=[Part(text="Hi")])]
        settings = PrimaryLLMSettings(
            system_instructions="You are a helpful assistant",
            max_output_tokens=1000,
            top_p_nucleus_sampling=1.0,
            top_k_filtering=40,
            stop_sequences=[],
            response_modalities=["TEXT"],
            tools=[],
            thinking_config=ThinkingConfig(
                thinking_level=ThinkingLevel.OFF, include_thoughts=False
            ),
            tool_config=ToolConfig(tool_calling_config=ToolCallingConfig(mode="AUTO")),
        )

        with pytest.raises(ContextWindowExceededError) as exc_info:
            await provider.generate_content_primary("llama3", contents, settings)

        assert exc_info.value.channel == "primary"
        assert exc_info.value.prompt_tokens == LLM_OLLAMA_DEFAULT_NUM_CTX

    @pytest.mark.asyncio
    async def test_generate_content_assistant_raises_on_empty_content(self, provider):
        provider, mock_client = provider

        mock_response = MagicMock()
        mock_response.message.content = ""
        mock_response.done_reason = "load"
        mock_response.prompt_eval_count = 100
        mock_response.eval_count = 0
        mock_client.chat = AsyncMock(return_value=mock_response)

        contents = [Content(role="user", parts=[Part(text="Hi")])]
        settings = AssistantLLMSettings(
            system_instructions="You are a helpful assistant",
            max_output_tokens=1000,
            top_p_nucleus_sampling=1.0,
            top_k_filtering=40,
            stop_sequences=[],
            response_format=ResponseFormat(
                json_schema=ResponseJsonSchema(json_schema_dict={}, name="response")
            ),
        )

        with pytest.raises(OllamaEmptyResponseError) as exc_info:
            await provider.generate_content_assistant("llama3", contents, settings)

        error = exc_info.value
        assert error.model == "llama3"
        assert error.channel == "assistant"
        assert error.done_reason == "load"
        assert error.ctx_overflow_suspected is False

    @pytest.mark.asyncio
    async def test_generate_content_lite_raises_on_empty_content(self, provider):
        provider, mock_client = provider

        mock_response = MagicMock()
        mock_response.message.content = ""
        mock_response.done_reason = "stop"
        mock_response.prompt_eval_count = 50
        mock_response.eval_count = 0
        mock_client.chat = AsyncMock(return_value=mock_response)

        contents = [Content(role="user", parts=[Part(text="Hi")])]
        settings = LiteLLMSettings(
            system_instructions="You are a helpful assistant",
            max_output_tokens=1000,
            top_p_nucleus_sampling=1.0,
            top_k_filtering=40,
            stop_sequences=[],
            response_format=ResponseFormat(
                json_schema=ResponseJsonSchema(json_schema_dict={}, name="response")
            ),
        )

        with pytest.raises(OllamaEmptyResponseError) as exc_info:
            await provider.generate_content_lite("llama3", contents, settings)

        error = exc_info.value
        assert error.model == "llama3"
        assert error.channel == "lite"
        assert error.done_reason == "stop"
        assert error.ctx_overflow_suspected is False

    @pytest.mark.asyncio
    async def test_generate_content_primary_with_thinking_only_raises_on_empty_content(
        self, provider
    ):
        provider, mock_client = provider

        mock_response = MagicMock()
        mock_response.message.content = ""
        mock_response.message.thinking = "This is my thinking process..."
        mock_response.done_reason = "stop"
        mock_response.prompt_eval_count = 100
        mock_response.eval_count = 0
        mock_client.chat = AsyncMock(return_value=mock_response)

        contents = [Content(role="user", parts=[Part(text="Hi")])]
        settings = PrimaryLLMSettings(
            system_instructions="You are a helpful assistant",
            max_output_tokens=1000,
            top_p_nucleus_sampling=1.0,
            top_k_filtering=40,
            stop_sequences=[],
            response_modalities=["TEXT"],
            tools=[],
            thinking_config=ThinkingConfig(
                thinking_level=ThinkingLevel.OFF, include_thoughts=False
            ),
            tool_config=ToolConfig(tool_calling_config=ToolCallingConfig(mode="AUTO")),
        )

        with pytest.raises(OllamaEmptyResponseError) as exc_info:
            await provider.generate_content_primary("llama3", contents, settings)

        error = exc_info.value
        assert error.thinking_len == 30
        assert error.ctx_overflow_suspected is False

    @pytest.mark.asyncio
    async def test_generate_content_primary_accepts_tool_calls_without_text(self, provider):
        provider, mock_client = provider

        mock_response = MagicMock()
        mock_response.message.content = ""
        mock_response.message.tool_calls = [
            SimpleNamespace(function=SimpleNamespace(name="inspect", arguments={"path": "a"}))
        ]
        mock_response.done_reason = "stop"
        mock_response.prompt_eval_count = 100
        mock_response.eval_count = 0
        mock_client.chat = AsyncMock(return_value=mock_response)

        contents = [Content(role="user", parts=[Part(text="Hi")])]
        settings = PrimaryLLMSettings(
            system_instructions="You are a helpful assistant",
            max_output_tokens=1000,
            top_p_nucleus_sampling=1.0,
            top_k_filtering=40,
            stop_sequences=[],
            response_modalities=["TEXT"],
            tools=[],
            thinking_config=ThinkingConfig(
                thinking_level=ThinkingLevel.OFF, include_thoughts=False
            ),
            tool_config=ToolConfig(tool_calling_config=ToolCallingConfig(mode="AUTO")),
        )

        result = await provider.generate_content_primary("llama3", contents, settings)
        call = result.candidates[0].content.parts[-1].tool_call
        assert call is not None
        assert call.name == "inspect"
        assert call.args == {"path": "a"}


class TestOllamaStreamingOverflow:
    @pytest.mark.asyncio
    @pytest.mark.parametrize("channel", ["primary", "assistant", "lite"])
    @pytest.mark.parametrize("has_text", [False, True])
    async def test_stream_rejects_overflow_before_emitting_output(self, channel, has_text):
        message = SimpleNamespace(content="answer" if has_text else "", thinking=None, tool_calls=[])
        terminal = SimpleNamespace(
            message=message, done=True, done_reason="stop",
            prompt_eval_count=LLM_OLLAMA_DEFAULT_NUM_CTX, eval_count=1,
        )

        async def upstream():
            yield terminal

        client = MagicMock()
        client.chat = AsyncMock(return_value=upstream())
        settings = {
            "primary": PrimaryLLMSettings(),
            "assistant": AssistantLLMSettings(),
            "lite": LiteLLMSettings(),
        }[channel]
        with patch(PATCH_TARGET, return_value=client):
            provider = OllamaProvider(endpoint="http://localhost:11434", api_key="")
            stream = getattr(provider, f"generate_content_stream_{channel}")(
                "llama3", [Content(role="user", parts=[Part(text="hi")])], settings,
            )
            with pytest.raises(ContextWindowExceededError) as raised:
                await anext(stream)
        assert raised.value.channel == channel
        assert raised.value.num_ctx == LLM_OLLAMA_DEFAULT_NUM_CTX


class TestPromptFilledContext:
    """The single predicate behind both the empty-content and the answered-prompt checks."""

    @pytest.mark.parametrize(
        ("prompt_eval_count", "num_ctx", "expected"),
        [
            (32768, 32768, True),
            (40000, 32768, True),
            (32767, 32768, False),
            (None, 32768, False),
            (32768, None, False),
            (32768, 0, False),
            (0, 32768, False),
        ],
    )
    def test_predicate(self, prompt_eval_count, num_ctx, expected):
        assert _prompt_filled_context(prompt_eval_count, num_ctx) is expected
