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
Gemini provider unit tests.

Covers content conversion, usage metadata parsing, finish reason extraction,
grounding metadata parsing, and provider configuration.
All Google GenAI SDK calls are mocked.
"""

from unittest.mock import MagicMock, patch

import pytest
import httpx
from google.genai import types as genai_types

from app.llm.llm_types import (
    Candidate,
    Content,
    GenerateContentResponse,
    Part,
    Role,
    ThoughtSignature,
    ToolCall,
    ToolResponse,
    UsageMetadata,
    ToolGroup,
    ToolDeclaration,
    PrimaryLLMSettings,
    StreamChunkFromModel,
    AssistantLLMSettings,
    LiteLLMSettings,
    ResponseFormat,
    ResponseJsonSchema,
)
from app.llm.providers.gemini import (
    _content_to_genai,
    _usage_from_sdk,
    _finish_reason_from_candidate,
    _grounding_from_sdk_candidate,
    _parts_from_sdk_candidate,
    GeminiProvider,
)

pytestmark = [pytest.mark.unit]


class TestContentToGenai:
    """_content_to_genai converts canonical Content to SDK-compatible dicts."""

    def test_text_part(self):
        content = Content(role=Role.USER, parts=[Part(text="hello")])
        result = _content_to_genai(content)
        assert result["role"] == Role.USER
        assert result["parts"] == [{"text": "hello"}]

    def test_tool_call_part(self):
        content = Content(
            role=Role.MODEL,
            parts=[Part(tool_call=ToolCall(name="test_tool", args={"a": 1}, id="call_1"))]
        )
        result = _content_to_genai(content)
        assert result["parts"] == [{
            "function_call": {"name": "test_tool", "args": {"a": 1}, "id": "call_1"}
        }]

    def test_tool_response_part(self):
        # Gemini 3 uses USER role for tool responses
        tr = ToolResponse(name="test_tool", response={"res": "ok"}, id="call_1")
        content = Content(
            role=Role.TOOL,
            parts=[Part(tool_response=tr)]
        )
        result = _content_to_genai(content)
        assert result["role"] == Role.USER
        assert result["parts"] == [{
            "function_response": {"name": "test_tool", "response": {"res": "ok"}, "id": "call_1"}
        }]

    def test_thought_part(self):
        content = Content(role=Role.MODEL, parts=[Part(text="thinking...", thought=True)])
        result = _content_to_genai(content)
        assert result["parts"] == [{"text": "thinking...", "thought": True}]

    def test_thought_signature_part(self):
        sig = ThoughtSignature(value="base64-sig")
        content = Content(role=Role.MODEL, parts=[Part(thought_signature=sig)])
        result = _content_to_genai(content)
        assert result["parts"] == [{"text": "", "thought_signature": "base64-sig"}]

    def test_mixed_parts(self):
        sig = ThoughtSignature(value="sig123")
        content = Content(role=Role.MODEL, parts=[
            Part(text="thought", thought=True, thought_signature=sig),
            Part(text="reply")
        ])
        result = _content_to_genai(content)
        assert len(result["parts"]) == 2
        assert result["parts"][0] == {"text": "thought", "thought": True, "thought_signature": "sig123"}
        assert result["parts"][1] == {"text": "reply"}

    def test_empty_part_dropped(self):
        content = Content(role=Role.USER, parts=[Part()])
        result = _content_to_genai(content)
        assert result["parts"] == []


class TestUsageFromSdk:
    """_usage_from_sdk builds canonical UsageMetadata."""

    def test_usage_parsing(self):
        mock_um = MagicMock()
        mock_um.prompt_token_count = 10
        mock_um.candidates_token_count = 20
        mock_um.total_token_count = 30
        mock_um.thoughts_token_count = 5

        usage = _usage_from_sdk(mock_um)
        assert usage.prompt_token_count == 10
        assert usage.candidates_token_count == 20
        assert usage.total_token_count == 30
        assert usage.thinking_token_count == 5

    def test_usage_missing_fields(self):
        mock_um = MagicMock(spec=[])
        usage = _usage_from_sdk(mock_um)
        assert usage.prompt_token_count == 0
        assert usage.candidates_token_count == 0
        assert usage.total_token_count == 0
        assert usage.thinking_token_count == 0


class TestFinishReasonFromCandidate:
    """_finish_reason_from_candidate extracts finish reason string."""

    def test_finish_reason_name(self):
        mock_candidate = MagicMock()
        mock_candidate.finish_reason.name = "STOP"
        assert _finish_reason_from_candidate(mock_candidate) == "STOP"

    def test_finish_reason_str(self):
        mock_candidate = MagicMock()
        # Create an object that has no .name but can be stringified
        class FakeReason:
            def __str__(self):
                return "SAFETY"
        mock_candidate.finish_reason = FakeReason()
        assert _finish_reason_from_candidate(mock_candidate) == "SAFETY"

    def test_no_finish_reason(self):
        mock_candidate = MagicMock()
        mock_candidate.finish_reason = None
        assert _finish_reason_from_candidate(mock_candidate) is None


class TestGroundingFromSdkCandidate:
    """_grounding_from_sdk_candidate parses grounding metadata."""

    def test_no_grounding(self):
        mock_candidate = MagicMock()
        mock_candidate.grounding_metadata = None
        assert _grounding_from_sdk_candidate(mock_candidate) is None

    def test_full_grounding(self):
        mock_candidate = MagicMock()
        gm = mock_candidate.grounding_metadata
        gm.web_search_queries = ["query1"]
        
        mock_chunk = MagicMock()
        mock_chunk.web.uri = "https://example.com"
        mock_chunk.web.title = "Example"
        gm.grounding_chunks = [mock_chunk]

        mock_support = MagicMock()
        mock_support.segment.start_index = 0
        mock_support.segment.end_index = 10
        mock_support.segment.text = "segment"
        mock_support.grounding_chunk_indices = [0]
        gm.grounding_supports = [mock_support]

        gm.search_entry_point.rendered_content = "<html></html>"

        grounding = _grounding_from_sdk_candidate(mock_candidate)
        assert grounding.web_search_queries == ["query1"]
        assert grounding.grounding_chunks[0].web.uri == "https://example.com"
        assert grounding.grounding_supports[0].segment.text == "segment"
        assert grounding.search_entry_point.rendered_content == "<html></html>"

    def test_grounding_chunk_no_web(self):
        mock_candidate = MagicMock()
        gm = mock_candidate.grounding_metadata
        mock_chunk = MagicMock()
        mock_chunk.web = None
        gm.grounding_chunks = [mock_chunk]
        gm.grounding_supports = []
        gm.search_entry_point = None
        
        grounding = _grounding_from_sdk_candidate(mock_candidate)
        assert grounding.grounding_chunks[0].web is None


class TestPartsFromSdkCandidate:
    """_parts_from_sdk_candidate converts SDK parts to canonical Part."""

    def test_text_normalization(self):
        mock_candidate = MagicMock()
        mock_part = MagicMock()
        mock_part.text = "line1\\nline2"
        mock_part.thought = False
        mock_part.function_call = None
        mock_part.thought_signature = None
        mock_candidate.content.parts = [mock_part]

        parts = _parts_from_sdk_candidate(mock_candidate)
        assert parts[0].text == "line1\nline2"

    def test_thought_part(self):
        mock_candidate = MagicMock()
        mock_part = MagicMock()
        mock_part.text = "thinking"
        mock_part.thought = True
        mock_part.thought_signature = b"sig"
        mock_candidate.content.parts = [mock_part]

        parts = _parts_from_sdk_candidate(mock_candidate)
        assert parts[0].text == "thinking"
        assert parts[0].thought is True
        assert parts[0].thought_signature.value == "c2ln"

    def test_tool_call_part(self):
        mock_candidate = MagicMock()
        mock_part = MagicMock()
        mock_part.text = None
        mock_part.thought = False
        mock_part.function_call.name = "my_tool"
        mock_part.function_call.args = {"x": 1}
        mock_part.function_call.id = "call_1"
        mock_candidate.content.parts = [mock_part]

        parts = _parts_from_sdk_candidate(mock_candidate)
        assert parts[0].tool_call.name == "my_tool"
        assert parts[0].tool_call.args == {"x": 1}

    def test_signature_only_part(self):
        mock_candidate = MagicMock()
        mock_part = MagicMock()
        mock_part.text = None
        mock_part.thought = False
        mock_part.function_call = None
        mock_part.thought_signature = b"sig"
        mock_candidate.content.parts = [mock_part]

        parts = _parts_from_sdk_candidate(mock_candidate)
        assert parts[0].text is None
        assert parts[0].thought_signature.value == "c2ln"


class TestStreamParsing:
    """_sdk_chunk_to_stream_from_model_chunks handles streaming responses."""

    def test_usage_chunk(self):
        mock_chunk = MagicMock()
        mock_chunk.candidates = []
        mock_chunk.usage_metadata.prompt_token_count = 10
        
        chunks = GeminiProvider._sdk_chunk_to_stream_from_model_chunks(mock_chunk)
        assert len(chunks) == 1
        assert chunks[0].usage_metadata.prompt_token_count == 10

    def test_text_chunk(self):
        mock_chunk = MagicMock()
        mock_candidate = MagicMock()
        mock_part = MagicMock()
        mock_part.text = "hello"
        mock_part.thought = False
        mock_part.function_call = None
        mock_part.thought_signature = None
        mock_candidate.content.parts = [mock_part]
        mock_candidate.finish_reason = None
        mock_chunk.candidates = [mock_candidate]
        mock_chunk.usage_metadata = None
        
        chunks = GeminiProvider._sdk_chunk_to_stream_from_model_chunks(mock_chunk)
        assert len(chunks) == 1
        assert chunks[0].text == "hello"

    def test_tool_call_chunk(self):
        mock_chunk = MagicMock()
        mock_candidate = MagicMock()
        mock_part = MagicMock()
        mock_part.text = None
        mock_part.thought = False
        mock_part.function_call.name = "tool"
        mock_part.function_call.args = {"a": 1}
        mock_part.function_call.id = "id1"
        mock_part.thought_signature = b"sig"
        mock_candidate.content.parts = [mock_part]
        # Ensure no extra chunks from finish_reason or usage
        mock_candidate.finish_reason = None
        mock_chunk.candidates = [mock_candidate]
        mock_chunk.usage_metadata = None
        
        chunks = GeminiProvider._sdk_chunk_to_stream_from_model_chunks(mock_chunk)
        assert len(chunks) == 1
        assert chunks[0].tool_calls[0].name == "tool"
        assert chunks[0].thought_signature.value == "c2ln"

class TestResponseParsing:
    """_parse_response converts full SDK responses."""

    @patch("google.genai.Client")
    def test_parse_full_response(self, mock_client):
        provider = GeminiProvider(api_key="key")
        mock_resp = MagicMock()
        
        mock_candidate = MagicMock()
        mock_candidate.content.parts = [MagicMock(text="reply", thought=False, function_call=None, thought_signature=None)]
        mock_candidate.finish_reason.name = "STOP"
        mock_candidate.grounding_metadata = None
        mock_resp.candidates = [mock_candidate]
        
        mock_resp.usage_metadata.prompt_token_count = 100
        
        parsed = provider._parse_response(mock_resp)
        assert parsed.candidates[0].content.parts[0].text == "reply"
        assert parsed.candidates[0].finish_reason == "STOP"
        assert parsed.usage_metadata.prompt_token_count == 100
class TestGeminiProvider:
    """GeminiProvider integration and config building."""

    def test_is_retryable(self):
        assert GeminiProvider._is_retryable(httpx.ConnectTimeout("timeout")) is True
        
        exc429 = Exception("too many requests")
        setattr(exc429, "status_code", 429)
        assert GeminiProvider._is_retryable(exc429) is True

        exc500 = Exception("internal error")
        setattr(exc500, "status_code", 500)
        assert GeminiProvider._is_retryable(exc500) is False

    def test_tool_group_to_genai(self):
        from app.llm.providers.gemini import _tool_group_to_genai
        params = {"type": "OBJECT", "properties": {"loc": {"type": "STRING"}}}
        tool_decl = ToolDeclaration(
            name="get_weather",
            description="Get weather info",
            parameters=params
        )
        tool_group = ToolGroup(tools=[tool_decl], google_search=True)
        
        genai_tools = _tool_group_to_genai(tool_group)
        assert len(genai_tools) == 2
        
        # Check function declaration
        funcs = genai_tools[0].function_declarations
        assert funcs[0].name == "get_weather"
        assert funcs[0].description == "Get weather info"
        # The SDK might return a Schema object; just check we can access it
        assert funcs[0].parameters is not None
        
        # Check google search
        assert genai_tools[1].google_search is not None

    def test_build_thinking_config_gemini3(self):
        from app.constants import ThinkingLevel
        from app.llm.llm_types import ThinkingConfig
        
        tc = ThinkingConfig(thinking_level=ThinkingLevel.HIGH, include_thoughts=True)
        # Mock translate_for_gemini to avoid actual model config lookup
        with patch("app.llm.providers.gemini.translate_for_gemini") as mock_translate:
            from app.llm.thinking import GeminiThinkingTranslation
            mock_translate.return_value = GeminiThinkingTranslation(
                enabled=True,
                thinking_level="HIGH",
                include_thoughts=True
            )
            
            sdk_tc, translation = GeminiProvider._build_thinking_config_gemini3(tc, "gemini-3.0", genai_types)
            assert sdk_tc.thinking_level == "HIGH"
            assert sdk_tc.include_thoughts is True
            assert translation.enabled is True

    def test_build_thinking_config_gemini3_off(self):
        from app.llm.llm_types import ThinkingConfig
        from app.constants import ThinkingLevel
        
        tc = ThinkingConfig(thinking_level=ThinkingLevel.OFF, include_thoughts=False)
        with patch("app.llm.providers.gemini.translate_for_gemini") as mock_translate:
            from app.llm.thinking import GeminiThinkingTranslation
            mock_translate.return_value = GeminiThinkingTranslation(
                enabled=False,
                thinking_level=None,
                include_thoughts=False
            )
            
            sdk_tc, translation = GeminiProvider._build_thinking_config_gemini3(tc, "gemini-3.0", genai_types)
            assert sdk_tc is None
            assert translation.enabled is False

    @patch("google.genai.Client")
    async def test_close_resources_error(self, mock_client):
        provider = GeminiProvider(api_key="test-key")
        with patch("asyncio.to_thread", side_effect=Exception("cleanup failed")):
            # Should not raise
            await provider._close_resources()

    def test_build_genai_config_primary(self):
        settings = PrimaryLLMSettings(
            max_output_tokens=1024,
            system_instructions="be helpful",
            top_p_nucleus_sampling=0.9,
            top_k_filtering=40
        )
        tools = [genai_types.Tool(google_search=genai_types.GoogleSearch())]
        
        config = GeminiProvider._build_genai_config(settings, tools, "gemini-2.0-flash")
        assert config.max_output_tokens == 1024
        assert config.system_instruction == "be helpful"
        assert config.top_p == 0.9
        assert config.top_k == 40
        assert config.tools == tools

    @patch("google.genai.Client")
    async def test_generate_with_retry(self, mock_client):
        provider = GeminiProvider(api_key="key")
        mock_resp = MagicMock()
        mock_resp.candidates = []
        mock_resp.usage_metadata = None
        
        with patch.object(provider, "_sync_generate", return_value=mock_resp) as mock_sync:
            resp = await provider._generate_with_retry("model", [], {})
            assert isinstance(resp, GenerateContentResponse)
            mock_sync.assert_called_once()

    @patch("google.genai.Client")
    async def test_stream_with_retry(self, mock_client):
        provider = GeminiProvider(api_key="key")
        
        mock_chunk = MagicMock()
        mock_chunk.candidates = []
        mock_chunk.usage_metadata = None
        
        # mock_sync_stream returns an iterator
        mock_iter = iter([mock_chunk])
        
        with patch.object(provider, "_sync_stream", return_value=mock_iter) as mock_sync:
            chunks = []
            async for chunk in provider._stream_with_retry("model", [], {}):
                chunks.append(chunk)
            
            # Since mock_chunk has no candidates/usage, it might not yield anything 
            # depending on _sdk_chunk_to_stream_from_model_chunks logic.
            # But we verify the sync call happened.
            mock_sync.assert_called_once()

    @patch("google.genai.Client")
    async def test_generate_content_stream_primary(self, mock_client):
        provider = GeminiProvider(api_key="key")
        settings = PrimaryLLMSettings(max_output_tokens=100)
        
        async def mock_stream(*args, **kwargs):
            yield StreamChunkFromModel(text="hi")
            
        with patch.object(provider, "_stream_with_retry", side_effect=mock_stream) as mock_retry:
            chunks = []
            async for chunk in provider.generate_content_stream_primary("model", [Content(role=Role.USER, parts=[Part(text="hi")])], settings):
                chunks.append(chunk)
            assert len(chunks) == 1
            assert chunks[0].text == "hi"
            mock_retry.assert_called_once()

    @patch("google.genai.Client")
    async def test_generate_content_stream_primary_error(self, mock_client):
        provider = GeminiProvider(api_key="key")
        settings = PrimaryLLMSettings(max_output_tokens=100)
        
        async def mock_stream_error(*args, **kwargs):
            raise Exception("api error")
            yield # make it a generator
            
        with patch.object(provider, "_stream_with_retry", side_effect=mock_stream_error):
            with patch("app.llm.providers.gemini.translate_capability_error") as mock_translate:
                with pytest.raises(Exception, match="api error"):
                    async for _ in provider.generate_content_stream_primary("model", [], settings):
                        pass
                mock_translate.assert_called_once()
    @patch("google.genai.Client")
    async def test_generate_content_primary(self, mock_client):
        provider = GeminiProvider(api_key="key")
        settings = PrimaryLLMSettings(max_output_tokens=100)
        
        with patch.object(provider, "_generate_with_retry", return_value=GenerateContentResponse()) as mock_retry:
            await provider.generate_content_primary("model", [Content(role=Role.USER, parts=[Part(text="hi")])], settings)
            mock_retry.assert_called_once()

    @patch("google.genai.Client")
    async def test_generate_content_assistant(self, mock_client):
        provider = GeminiProvider(api_key="key")
        settings = AssistantLLMSettings(max_output_tokens=100)
        with patch.object(provider, "_generate_with_retry", return_value=GenerateContentResponse()) as mock_retry:
            await provider.generate_content_assistant("model", [], settings)
            mock_retry.assert_called_once()

    @patch("google.genai.Client")
    async def test_generate_content_lite(self, mock_client):
        provider = GeminiProvider(api_key="key")
        settings = LiteLLMSettings(max_output_tokens=100)
        with patch.object(provider, "_generate_with_retry", return_value=GenerateContentResponse()) as mock_retry:
            await provider.generate_content_lite("model", [], settings)
            mock_retry.assert_called_once()

    @patch("google.genai.Client")
    async def test_generate_content_stream_assistant(self, mock_client):
        provider = GeminiProvider(api_key="key")
        settings = AssistantLLMSettings(max_output_tokens=100)
        async def mock_stream(*args, **kwargs):
            yield StreamChunkFromModel(text="hi")
        with patch.object(provider, "_stream_with_retry", side_effect=mock_stream):
            chunks = []
            async for chunk in provider.generate_content_stream_assistant("model", [], settings):
                chunks.append(chunk)
            assert len(chunks) == 1

    @patch("google.genai.Client")
    async def test_generate_content_stream_lite(self, mock_client):
        provider = GeminiProvider(api_key="key")
        settings = LiteLLMSettings(max_output_tokens=100)
        async def mock_stream(*args, **kwargs):
            yield StreamChunkFromModel(text="hi")
        with patch.object(provider, "_stream_with_retry", side_effect=mock_stream):
            chunks = []
            async for chunk in provider.generate_content_stream_lite("model", [], settings):
                chunks.append(chunk)
            assert len(chunks) == 1
    def test_build_genai_config_assistant_json(self):
        schema = ResponseJsonSchema(
            json_schema_dict={"type": "object", "properties": {"res": {"type": "string"}}}
        )
        settings = AssistantLLMSettings(
            max_output_tokens=512,
            response_format=ResponseFormat(json_schema=schema)
        )
        
        config = GeminiProvider._build_genai_config(settings, None, "gemini-2.0-flash")
        assert config.max_output_tokens == 512
        assert config.response_mime_type == "application/json"
        assert config.response_json_schema == schema.json_schema_dict

    @patch("google.genai.Client")
    async def test_generate_content_primary_error(self, mock_client):
        provider = GeminiProvider(api_key="key")
        settings = PrimaryLLMSettings(max_output_tokens=100)
        with patch.object(provider, "_generate_with_retry", side_effect=Exception("api error")):
            with patch("app.llm.providers.gemini.translate_capability_error") as mock_translate:
                with pytest.raises(Exception, match="api error"):
                    await provider.generate_content_primary("model", [], settings)
                mock_translate.assert_called_once()

    @patch("google.genai.Client")
    def test_sync_wrappers(self, mock_client):
        provider = GeminiProvider(api_key="key")
        provider._sync_generate("model", [], {})
        provider._client.models.generate_content.assert_called_once()
        
        provider._sync_stream("model", [], {})
        provider._client.models.generate_content_stream.assert_called_once()

    def test_sdk_chunk_to_stream_finish_reason(self):
        mock_chunk = MagicMock()
        mock_chunk.candidates = [MagicMock(finish_reason=MagicMock(name="STOP"), content=None)]
        mock_chunk.usage_metadata = None
        
        chunks = GeminiProvider._sdk_chunk_to_stream_from_model_chunks(mock_chunk)
        assert len(chunks) == 1
        assert chunks[0].finish_reason == "STOP"

    def test_grounding_raw_indices_error(self):
        mock_candidate = MagicMock()
        gm = mock_candidate.grounding_metadata
        mock_support = MagicMock()
        mock_support.segment.text = "seg"
        # Cause TypeError in list(raw_indices)
        mock_support.grounding_chunk_indices = 123 
        gm.grounding_supports = [mock_support]
        gm.grounding_chunks = []
        gm.web_search_queries = []
        gm.search_entry_point = None
        
        grounding = _grounding_from_sdk_candidate(mock_candidate)
        assert grounding.grounding_supports[0].grounding_chunk_indices == []
