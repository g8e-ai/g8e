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
Unit tests for OllamaProvider.

Tests SSL verification strategy, close behavior, construction, and content generation.
"""

from unittest.mock import patch, MagicMock, AsyncMock
import json

import httpx
import pytest

from app.llm.providers.ollama import OllamaProvider
from app.llm.llm_types import (
    Content,
    Part,
    PrimaryLLMSettings,
    AssistantLLMSettings,
    LiteLLMSettings,
    ThinkingConfig,
    ToolConfig,
    ToolCallingConfig,
    ResponseFormat,
    ResponseJsonSchema,
)


PATCH_TARGET = "app.llm.providers.ollama.httpx.AsyncClient"
INTERNAL_CA = "/g8es/ca.crt"

pytestmark = [pytest.mark.unit]


class TestOllamaProviderSSL:
    """SSL verification strategy for Ollama provider."""

    def test_external_endpoint_uses_default_verification(self):
        with patch(PATCH_TARGET) as mock_client:
            OllamaProvider(
                endpoint="https://api.ollama.ai",
                api_key="test-key",
            )
            mock_client.assert_called_once()
            assert mock_client.call_args.kwargs.get("verify") is True

    def test_internal_localhost_uses_platform_ca(self):
        with patch(PATCH_TARGET) as mock_client:
            OllamaProvider(
                endpoint="https://localhost:11434",
                api_key="test-key",
                ca_cert_path=INTERNAL_CA,
            )
            mock_client.assert_called_once()
            import ssl
            assert isinstance(mock_client.call_args.kwargs.get("verify"), ssl.SSLContext)

    def test_internal_ip_uses_platform_ca(self):
        with patch(PATCH_TARGET) as mock_client:
            OllamaProvider(
                endpoint="https://192.168.1.50:11434",
                api_key="test-key",
                ca_cert_path=INTERNAL_CA,
            )
            mock_client.assert_called_once()
            import ssl
            assert isinstance(mock_client.call_args.kwargs.get("verify"), ssl.SSLContext)

    def test_internal_http_disables_ssl(self):
        with patch(PATCH_TARGET) as mock_client:
            OllamaProvider(
                endpoint="http://10.0.0.1:11434",
                api_key="test-key",
            )
            mock_client.assert_called_once()
            assert mock_client.call_args.kwargs.get("verify") is False

    def test_internal_without_ca_falls_back_to_true(self):
        with patch(PATCH_TARGET) as mock_client:
            OllamaProvider(
                endpoint="https://localhost:11434",
                api_key="test-key",
            )
            mock_client.assert_called_once()
            assert mock_client.call_args.kwargs.get("verify") is True


class TestOllamaProviderClose:
    """Test that OllamaProvider properly closes its httpx client."""

    @pytest.mark.asyncio
    async def test_close_calls_aclose_on_client(self):
        mock_httpx_client = AsyncMock()
        with patch(PATCH_TARGET, return_value=mock_httpx_client):
            provider = OllamaProvider(
                endpoint="https://localhost:11434",
                api_key="test-key",
            )
            await provider.close()
            mock_httpx_client.aclose.assert_called_once()


class TestOllamaProviderConstruction:
    """Test OllamaProvider construction and initialization."""

    def test_constructor_creates_async_client(self):
        mock_client = MagicMock()
        with patch(PATCH_TARGET, return_value=mock_client) as mock_ctor:
            provider = OllamaProvider(
                endpoint="https://localhost:11434",
                api_key="test-key",
            )
            mock_ctor.assert_called_once()
            assert provider._httpx_client is mock_client
            assert provider._client is not None

    def test_constructor_strips_trailing_slash(self):
        with patch(PATCH_TARGET):
            provider = OllamaProvider(
                endpoint="https://localhost:11434/",
                api_key="test-key",
            )
            assert provider._original_endpoint == "https://localhost:11434"

    def test_constructor_strips_v1_suffix(self):
        with patch(PATCH_TARGET):
            provider = OllamaProvider(
                endpoint="https://localhost:11434/v1",
                api_key="test-key",
            )
            assert provider._original_endpoint == "https://localhost:11434"

    @pytest.mark.asyncio
    async def test_context_manager_support(self):
        """Test that OllamaProvider supports async context manager."""
        mock_httpx_client = AsyncMock()
        with patch(PATCH_TARGET, return_value=mock_httpx_client):
            provider = OllamaProvider(
                endpoint="https://localhost:11434",
                api_key="test-key",
            )
            async with provider:
                assert provider is not None
            mock_httpx_client.aclose.assert_called_once()


class TestOllamaProviderGeneration:
    """Test OllamaProvider generation methods with mocked httpx transport."""

    @pytest.fixture
    def provider(self):
        with patch("app.llm.providers.ollama._InjectedAsyncClient") as mock_client_cls:
            mock_client = MagicMock()
            mock_client_cls.return_value = mock_client
            
            provider = OllamaProvider(
                endpoint="http://localhost:11434",
                api_key="test-key",
            )
            yield provider, mock_client

    @pytest.mark.asyncio
    async def test_generate_content_primary(self, provider):
        provider, mock_client = provider
        
        mock_response = MagicMock()
        mock_response.message.content = "Hello World"
        mock_response.message.thinking = "Thinking..."
        mock_response.message.tool_calls = None
        mock_response.done_reason = "stop"
        mock_response.prompt_eval_count = 10
        mock_response.eval_count = 5
        mock_client.chat = AsyncMock(return_value=mock_response)
        
        contents = [Content(role="user", parts=[Part(text="Hi")])]
        settings = PrimaryLLMSettings(
            system_instructions="You are a helpful assistant",
            temperature=0.7,
            max_output_tokens=1000,
            top_p_nucleus_sampling=1.0,
            top_k_filtering=40,
            stop_sequences=[],
            response_modalities=["TEXT"],
            tools=[],
            thinking_config=ThinkingConfig(thinking_level=None, include_thoughts=False),
            tool_config=ToolConfig(tool_calling_config=ToolCallingConfig(mode="AUTO")),
        )
        
        response = await provider.generate_content_primary("llama3", contents, settings)
        
        mock_client.chat.assert_called_once()
        assert len(response.candidates) == 1
        assert response.candidates[0].content.parts[0].text == "Thinking..."
        assert response.candidates[0].content.parts[1].text == "Hello World"
        assert response.usage_metadata is not None
        assert response.usage_metadata.prompt_token_count == 10
        assert response.usage_metadata.candidates_token_count == 5

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
            temperature=0.7,
            max_output_tokens=1000,
            top_p_nucleus_sampling=1.0,
            top_k_filtering=40,
            stop_sequences=[],
            response_modalities=["TEXT"],
            tools=[],
            thinking_config=ThinkingConfig(thinking_level=None, include_thoughts=False),
            tool_config=ToolConfig(tool_calling_config=ToolCallingConfig(mode="AUTO")),
        )
        
        chunks = []
        async for chunk in provider.generate_content_stream_primary("llama3", contents, settings):
            chunks.append(chunk)
            
        mock_client.chat.assert_called_once()
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
            temperature=0.7,
            max_output_tokens=1000,
            top_p_nucleus_sampling=1.0,
            top_k_filtering=40,
            stop_sequences=[],
            response_format=ResponseFormat(json_schema=ResponseJsonSchema(schema={}, name="response")),
        )
        
        response = await provider.generate_content_assistant("llama3", contents, settings)
        
        mock_client.chat.assert_called_once()
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
            temperature=0.7,
            max_output_tokens=1000,
            top_p_nucleus_sampling=1.0,
            top_k_filtering=40,
            stop_sequences=[],
            response_format=ResponseFormat(json_schema=ResponseJsonSchema(schema={}, name="response")),
        )
        
        response = await provider.generate_content_lite("llama3", contents, settings)
        
        mock_client.chat.assert_called_once()
        assert len(response.candidates) == 1
        assert response.candidates[0].content.parts[0].text == "Hello World"
        assert response.usage_metadata is not None
