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

from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from app.constants import ThinkingLevel
from app.llm.llm_types import (
    AssistantLLMSettings,
    Content,
    LiteLLMSettings,
    Part,
    PrimaryLLMSettings,
    ResponseFormat,
    ResponseJsonSchema,
    Schema,
    ToolCall,
    ToolDeclaration,
    ToolGroup,
    Type,
)
from app.llm.providers.open_ai import OpenAIProvider, _contents_to_messages, _tools_to_openai

pytestmark = [pytest.mark.unit]

def _make_provider():
    with patch("app.llm.providers.open_ai.AsyncOpenAI"):
        return OpenAIProvider(endpoint="http://test/v1", api_key="test-key")

class TestContentsToMessages:
    def test_basic_conversion(self):
        contents = [
            Content(role="user", parts=[Part(text="hello")]),
            Content(role="model", parts=[Part(text="hi")]),
        ]
        messages = _contents_to_messages(contents, system_instructions="be helpful")
        assert messages == [
            {"role": "system", "content": "be helpful"},
            {"role": "user", "content": "hello"},
            {"role": "assistant", "content": "hi"},
        ]

    def test_tool_call_conversion(self):
        contents = [
            Content(role="model", parts=[Part(tool_call=ToolCall(name="get_weather", args={"city": "London"}))])
        ]
        messages = _contents_to_messages(contents, system_instructions="")
        assert messages == [
            {
                "role": "assistant",
                "content": None,
                "tool_calls": [{
                    "id": "call_get_weather",
                    "type": "tool",
                    "tool": {
                        "name": "get_weather",
                        "arguments": '{"city": "London"}',
                    },
                }],
            }
        ]

    def test_tool_response_conversion(self):
        from app.llm.llm_types import ToolResponse
        contents = [
            Content(role="user", parts=[Part(tool_response=ToolResponse(name="get_weather", response={"temp": 20}))])
        ]
        messages = _contents_to_messages(contents, system_instructions="")
        assert messages == [
            {
                "role": "tool",
                "tool_call_id": "call_get_weather",
                "content": '{"temp": 20}',
            }
        ]

    def test_text_and_tool_parts(self):
        from app.llm.llm_types import ToolCall
        contents = [
            Content(role="user", parts=[Part(text="hello")]),
            Content(role="model", parts=[
                Part(text="thinking"),
                Part(tool_call=ToolCall(name="cmd", args={}))
            ]),
        ]
        messages = _contents_to_messages(contents, system_instructions="")
        assert messages == [
            {"role": "user", "content": "hello"},
            {"role": "assistant", "content": "thinking"},
            {
                "role": "assistant",
                "content": None,
                "tool_calls": [{
                    "id": "call_cmd",
                    "type": "tool",
                    "tool": {"name": "cmd", "arguments": "{}"}
                }]
            }
        ]

class TestToolsToOpenAI:
    def test_basic_conversion(self):
        schema = Schema(type=Type.OBJECT, properties={"city": Schema(type=Type.STRING)})
        tools = [ToolGroup(tools=[
            ToolDeclaration(name="get_weather", description="Get weather", parameters=schema)
        ])]
        openai_tools = _tools_to_openai(tools)
        assert len(openai_tools) == 1
        assert openai_tools[0]["type"] == "function"
        assert openai_tools[0]["function"]["name"] == "get_weather"
        assert openai_tools[0]["function"]["parameters"]["type"] == "object"

    def test_empty_tools(self):
        assert _tools_to_openai(None) is None
        assert _tools_to_openai([]) is None

class TestOpenAIProvider:
    def test_init_adds_v1(self):
        with patch("app.llm.providers.open_ai.AsyncOpenAI") as mock_client:
            OpenAIProvider(endpoint="http://test", api_key="test")
            mock_client.assert_called_once()
            assert mock_client.call_args.kwargs["base_url"] == "http://test/v1"

    @pytest.mark.asyncio
    async def test_close_resources(self):
        provider = _make_provider()
        provider._client.close = AsyncMock()
        await provider._close_resources()
        provider._client.close.assert_called_once()

    def test_build_openai_kwargs_basic(self):
        messages = [{"role": "user", "content": "hi"}]
        kwargs = OpenAIProvider._build_openai_kwargs(
            model="gpt-4",
            messages=messages,
            max_tokens=100,
            top_p=0.9,
            stop=["\n"],
        )
        assert kwargs == {
            "model": "gpt-4",
            "messages": messages,
            "max_tokens": 100,
            "stream": False,
            "top_p": 0.9,
            "stop": ["\n"],
        }

    def test_build_openai_kwargs_with_thinking(self):
        with patch("app.llm.providers.open_ai.get_model_config"), \
             patch("app.llm.providers.open_ai.translate_for_openai") as mock_translate:

            mock_translate.return_value = MagicMock(enabled=True, reasoning_effort="high")

            kwargs = OpenAIProvider._build_openai_kwargs(
                model="gpt-5",
                messages=[],
                max_tokens=100,
                top_p=None,
                stop=None,
                thinking_config=MagicMock(thinking_level=ThinkingLevel.HIGH)
            )
            assert kwargs["reasoning"] == {"effort": "high"}

    @pytest.mark.asyncio
    async def test_generate_content_primary_streaming_with_tools(self):
        provider = _make_provider()

        # Mock response for non-streaming tool call (OpenAIProvider uses non-streaming when tools are present)
        mock_response = MagicMock()
        mock_choice = MagicMock()
        mock_choice.finish_reason = "tool_calls"
        mock_choice.message.content = "I will check the weather."
        mock_choice.message.reasoning_content = "Thinking about weather..."

        mock_tool_call = MagicMock()
        mock_tool_call.function.name = "get_weather"
        mock_tool_call.function.arguments = '{"city": "London"}'
        mock_tool_call.id = "call_123"
        mock_choice.message.tool_calls = [mock_tool_call]

        mock_response.choices = [mock_choice]
        mock_response.usage = MagicMock(prompt_tokens=10, completion_tokens=20, total_tokens=30)

        provider._client.chat.completions.create = AsyncMock(return_value=mock_response)

        settings = PrimaryLLMSettings(
            max_output_tokens=100,
            tools=[ToolGroup(tools=[ToolDeclaration(name="get_weather", description="Weather", parameters=None)])],
            system_instructions="be helpful",
        )

        chunks = []
        async for chunk in provider.generate_content_stream_primary(
            model="gpt-4",
            contents=[Content(role="user", parts=[Part(text="weather in London")])],
            primary_llm_settings=settings
        ):
            chunks.append(chunk)

        assert len(chunks) == 4 # thought, content, tool_calls, finish+usage
        assert chunks[0].text == "Thinking about weather..."
        assert chunks[0].thought is True
        assert chunks[1].text == "I will check the weather."
        assert chunks[2].tool_calls[0].name == "get_weather"
        assert chunks[3].finish_reason == "tool_calls"
        assert chunks[3].usage_metadata.total_token_count == 30

    @pytest.mark.asyncio
    async def test_generate_content_primary_streaming_no_tools(self):
        provider = _make_provider()

        # Mock stream for regular text
        mock_chunk = MagicMock()
        mock_chunk.choices = [MagicMock()]
        mock_chunk.choices[0].delta.content = "hello"
        mock_chunk.choices[0].delta.reasoning_content = "thinking"
        mock_chunk.choices[0].finish_reason = None

        mock_final_chunk = MagicMock()
        mock_final_chunk.choices = [MagicMock()]
        mock_final_chunk.choices[0].delta = None
        mock_final_chunk.choices[0].finish_reason = "stop"

        async def mock_stream_iter():
            yield mock_chunk
            yield mock_final_chunk

        provider._client.chat.completions.create = AsyncMock(return_value=mock_stream_iter())

        settings = PrimaryLLMSettings(max_output_tokens=100)

        chunks = []
        async for chunk in provider.generate_content_stream_primary(
            model="gpt-4",
            contents=[Content(role="user", parts=[Part(text="hi")])],
            primary_llm_settings=settings
        ):
            chunks.append(chunk)

        # 1st chunk: reasoning content
        assert chunks[0].text == "thinking"
        assert chunks[0].thought is True

        # 2nd chunk: content
        assert chunks[1].text == "hello"

        # 3rd chunk: finish reason
        assert chunks[2].finish_reason == "stop"

    @pytest.mark.asyncio
    async def test_generate_content_primary_error_translation(self):
        provider = _make_provider()
        provider._client.chat.completions.create = AsyncMock(side_effect=Exception("OpenAI Error"))

        settings = PrimaryLLMSettings(max_output_tokens=100)

        with patch("app.llm.providers.open_ai.translate_capability_error") as mock_translate:
            with pytest.raises(Exception, match="OpenAI Error"):
                await provider.generate_content_primary(
                    model="gpt-4",
                    contents=[Content(role="user", parts=[Part(text="hi")])],
                    primary_llm_settings=settings
                )
            mock_translate.assert_called_once()

    @pytest.mark.asyncio
    async def test_generate_content_assistant(self):
        provider = _make_provider()

        mock_response = MagicMock()
        mock_choice = MagicMock()
        mock_choice.message.content = "analysis"
        mock_choice.finish_reason = "stop"
        mock_response.choices = [mock_choice]
        mock_response.usage = MagicMock(prompt_tokens=5, completion_tokens=5, total_tokens=10)

        provider._client.chat.completions.create = AsyncMock(return_value=mock_response)

        settings = AssistantLLMSettings(
            max_output_tokens=100,
            response_format=ResponseFormat(json_schema=ResponseJsonSchema(json_schema_dict={}, name="res"))
        )

        response = await provider.generate_content_assistant(
            model="gpt-4",
            contents=[Content(role="user", parts=[Part(text="analyze")])],
            assistant_llm_settings=settings
        )

        assert response.candidates[0].content.parts[0].text == "analysis"
        assert response.usage_metadata.total_token_count == 10

    @pytest.mark.asyncio
    async def test_generate_content_primary_no_streaming(self):
        provider = _make_provider()

        mock_response = MagicMock()
        mock_choice = MagicMock()
        mock_choice.message.content = "direct response"
        mock_choice.message.reasoning_content = None # Ensure no reasoning content to avoid Part(thought=True)
        mock_choice.finish_reason = "stop"
        mock_response.choices = [mock_choice]
        mock_response.usage = MagicMock(prompt_tokens=10, completion_tokens=10, total_tokens=20)

        provider._client.chat.completions.create = AsyncMock(return_value=mock_response)

        settings = PrimaryLLMSettings(max_output_tokens=100)

        response = await provider.generate_content_primary(
            model="gpt-4",
            contents=[Content(role="user", parts=[Part(text="hi")])],
            primary_llm_settings=settings
        )

        assert response.candidates[0].content.parts[0].text == "direct response"
        assert response.usage_metadata.total_token_count == 20

    @pytest.mark.asyncio
    async def test_generate_content_primary_with_reasoning(self):
        provider = _make_provider()

        mock_response = MagicMock()
        mock_choice = MagicMock()
        mock_choice.message.content = "answer"
        mock_choice.message.reasoning_content = "reasoning"
        mock_choice.finish_reason = "stop"
        mock_response.choices = [mock_choice]
        mock_response.usage = MagicMock(prompt_tokens=10, completion_tokens=10, total_tokens=20)

        provider._client.chat.completions.create = AsyncMock(return_value=mock_response)

        settings = PrimaryLLMSettings(max_output_tokens=100)

        response = await provider.generate_content_primary(
            model="gpt-4",
            contents=[Content(role="user", parts=[Part(text="hi")])],
            primary_llm_settings=settings
        )

        assert response.candidates[0].content.parts[0].text == "reasoning"
        assert response.candidates[0].content.parts[0].thought is True
        assert response.candidates[0].content.parts[1].text == "answer"

    @pytest.mark.asyncio
    async def test_generate_content_stream_assistant(self):
        provider = _make_provider()

        mock_chunk = MagicMock()
        mock_chunk.choices = [MagicMock()]
        mock_chunk.choices[0].delta.content = "assistant stream"
        mock_chunk.choices[0].finish_reason = "stop"

        async def mock_stream_iter():
            yield mock_chunk

        provider._client.chat.completions.create = AsyncMock(return_value=mock_stream_iter())

        settings = AssistantLLMSettings(max_output_tokens=100)

        chunks = []
        async for chunk in provider.generate_content_stream_assistant(
            model="gpt-4",
            contents=[Content(role="user", parts=[Part(text="hi")])],
            assistant_llm_settings=settings
        ):
            chunks.append(chunk)

        assert chunks[0].text == "assistant stream"
        assert chunks[1].finish_reason == "stop"

    @pytest.mark.asyncio
    async def test_generate_content_stream_primary_error(self):
        provider = _make_provider()
        provider._client.chat.completions.create = AsyncMock(side_effect=Exception("Stream Error"))
        settings = PrimaryLLMSettings(max_output_tokens=100)
        with patch("app.llm.providers.open_ai.translate_capability_error") as mock_translate:
            with pytest.raises(Exception, match="Stream Error"):
                async for _ in provider.generate_content_stream_primary(
                    model="gpt-4",
                    contents=[Content(role="user", parts=[Part(text="hi")])],
                    primary_llm_settings=settings
                ):
                    pass
            mock_translate.assert_called_once()

    @pytest.mark.asyncio
    async def test_generate_content_stream_assistant_error(self):
        provider = _make_provider()
        provider._client.chat.completions.create = AsyncMock(side_effect=Exception("Assistant Stream Error"))
        settings = AssistantLLMSettings(max_output_tokens=100)
        with patch("app.llm.providers.open_ai.translate_capability_error") as mock_translate:
            with pytest.raises(Exception, match="Assistant Stream Error"):
                async for _ in provider.generate_content_stream_assistant(
                    model="gpt-4",
                    contents=[Content(role="user", parts=[Part(text="hi")])],
                    assistant_llm_settings=settings
                ):
                    pass
            mock_translate.assert_called_once()

    @pytest.mark.asyncio
    async def test_generate_content_stream_lite_error(self):
        provider = _make_provider()
        provider._client.chat.completions.create = AsyncMock(side_effect=Exception("Lite Stream Error"))
        settings = LiteLLMSettings(max_output_tokens=100)
        with patch("app.llm.providers.open_ai.translate_capability_error") as mock_translate:
            with pytest.raises(Exception, match="Lite Stream Error"):
                async for _ in provider.generate_content_stream_lite(
                    model="gpt-4",
                    contents=[Content(role="user", parts=[Part(text="hi")])],
                    lite_llm_settings=settings
                ):
                    pass
            mock_translate.assert_called_once()

    @pytest.mark.asyncio
    async def test_generate_content_assistant_error(self):
        provider = _make_provider()
        provider._client.chat.completions.create = AsyncMock(side_effect=Exception("Assistant Error"))
        settings = AssistantLLMSettings(max_output_tokens=100)
        with patch("app.llm.providers.open_ai.translate_capability_error") as mock_translate:
            with pytest.raises(Exception, match="Assistant Error"):
                await provider.generate_content_assistant(
                    model="gpt-4",
                    contents=[Content(role="user", parts=[Part(text="hi")])],
                    assistant_llm_settings=settings
                )
            mock_translate.assert_called_once()

    @pytest.mark.asyncio
    async def test_generate_content_stream_lite(self):
        provider = _make_provider()

        mock_chunk = MagicMock()
        mock_chunk.choices = [MagicMock()]
        mock_chunk.choices[0].delta.content = "lite stream"
        mock_chunk.choices[0].finish_reason = "stop"

        async def mock_stream_iter():
            yield mock_chunk

        provider._client.chat.completions.create = AsyncMock(return_value=mock_stream_iter())

        settings = LiteLLMSettings(max_output_tokens=100)

        chunks = []
        async for chunk in provider.generate_content_stream_lite(
            model="gpt-4",
            contents=[Content(role="user", parts=[Part(text="hi")])],
            lite_llm_settings=settings
        ):
            chunks.append(chunk)

        assert chunks[0].text == "lite stream"
        assert chunks[1].finish_reason == "stop"

    @pytest.mark.asyncio
    async def test_generate_content_lite(self):
        provider = _make_provider()

        mock_response = MagicMock()
        mock_choice = MagicMock()
        mock_choice.message.content = "lite response"
        mock_choice.finish_reason = "stop"
        mock_response.choices = [mock_choice]
        mock_response.usage = MagicMock(prompt_tokens=5, completion_tokens=5, total_tokens=10)

        provider._client.chat.completions.create = AsyncMock(return_value=mock_response)

        settings = LiteLLMSettings(max_output_tokens=100)

        response = await provider.generate_content_lite(
            model="gpt-4",
            contents=[Content(role="user", parts=[Part(text="hi")])],
            lite_llm_settings=settings
        )

        assert response.candidates[0].content.parts[0].text == "lite response"
        assert response.usage_metadata.total_token_count == 10

    @pytest.mark.asyncio
    async def test_generate_content_lite_error(self):
        provider = _make_provider()
        provider._client.chat.completions.create = AsyncMock(side_effect=Exception("Lite Error"))
        settings = LiteLLMSettings(max_output_tokens=100)
        with patch("app.llm.providers.open_ai.translate_capability_error") as mock_translate:
            with pytest.raises(Exception, match="Lite Error"):
                await provider.generate_content_lite(
                    model="gpt-4",
                    contents=[Content(role="user", parts=[Part(text="hi")])],
                    lite_llm_settings=settings
                )
            mock_translate.assert_called_once()
