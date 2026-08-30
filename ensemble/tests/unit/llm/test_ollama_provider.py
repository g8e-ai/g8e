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

from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from app.constants import LLM_OLLAMA_DEFAULT_NUM_CTX, ThinkingLevel
from app.errors import OllamaEmptyResponseError
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
from app.llm.providers.ollama import OllamaProvider

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
        assert response.usage_metadata.cache_token_count == 0
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
    async def test_generate_content_primary_with_tool_calls_raises_on_empty_content(self, provider):
        provider, mock_client = provider

        mock_response = MagicMock()
        mock_response.message.content = ""
        mock_response.message.tool_calls = [MagicMock()]
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
        assert error.tool_calls_count == 1
