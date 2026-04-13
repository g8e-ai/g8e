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
Anthropic provider unit tests.

Covers parameter constraint enforcement, response parsing, and content
conversion. All Anthropic API calls are mocked -- no network access.
"""

from unittest.mock import patch, MagicMock, AsyncMock

import pytest

from app.constants import LLM_DEFAULT_TEMPERATURE, LLM_DEFAULT_MAX_OUTPUT_TOKENS, ThinkingLevel
from app.llm.llm_types import (
    AssistantLLMSettings,
    Candidate,
    Content,
    LiteLLMSettings,
    Part,
    PrimaryLLMSettings,
    ThinkingConfig,
    ThoughtSignature,
    ToolCall,
    ToolResponse,
    ToolDeclaration,
    ToolGroup,
    UsageMetadata,
    Schema,
    Type,
)

pytestmark = [pytest.mark.unit]


def _make_provider():
    """Create an AnthropicProvider with mocked httpx and anthropic clients."""
    with patch("app.llm.providers.anthropic.anthropic.AsyncAnthropic"):
        from app.llm.providers.anthropic import AnthropicProvider
        return AnthropicProvider(endpoint=None, api_key="test-key")


class TestBuildKwargs:
    """_build_kwargs enforces Anthropic's parameter constraints."""

    def _build(self, **overrides):
        provider = _make_provider()
        defaults = {
            "model": "claude-sonnet-4-20250514",
            "messages": [{"role": "user", "content": [{"type": "text", "text": "hi"}]}],
            "temperature": 0.4,
            "max_tokens": 8192,
        }
        defaults.update(overrides)
        return provider._build_kwargs(**defaults)

    def test_never_sends_top_p(self):
        """Anthropic: temperature and top_p are mutually exclusive. We never send top_p."""
        kwargs = self._build(temperature=0.7)
        assert "top_p" not in kwargs
        assert kwargs["temperature"] == 0.7

    def test_temperature_defaults_when_none(self):
        kwargs = self._build(temperature=None)
        assert kwargs["temperature"] == LLM_DEFAULT_TEMPERATURE

    def test_max_tokens_defaults_when_none(self):
        kwargs = self._build(max_tokens=None)
        assert kwargs["max_tokens"] == LLM_DEFAULT_MAX_OUTPUT_TOKENS

    def test_top_k_included_when_provided(self):
        kwargs = self._build(top_k=40)
        assert kwargs["top_k"] == 40

    def test_top_k_omitted_when_none(self):
        kwargs = self._build(top_k=None)
        assert "top_k" not in kwargs

    def test_system_instruction_included(self):
        kwargs = self._build(system_instruction="be helpful")
        assert kwargs["system"] == "be helpful"

    def test_system_instruction_omitted_when_empty(self):
        kwargs = self._build(system_instruction="")
        assert "system" not in kwargs

    def test_tools_included(self):
        tools = [{"name": "run_command", "description": "exec", "input_schema": {}}]
        kwargs = self._build(anthropic_tools=tools)
        assert kwargs["tools"] == tools

    def test_tools_omitted_when_none(self):
        kwargs = self._build(anthropic_tools=None)
        assert "tools" not in kwargs


class TestBuildKwargsThinkingMode:
    """Thinking mode enforces temperature=1.0 and strips sampling params."""

    def _build(self, **overrides):
        provider = _make_provider()
        defaults = {
            "model": "claude-sonnet-4-20250514",
            "messages": [],
            "temperature": 0.7,
            "max_tokens": 20000,
            "thinking_config": ThinkingConfig(
                thinking_level=ThinkingLevel.HIGH,
                include_thoughts=True,
            ),
        }
        defaults.update(overrides)
        return provider._build_kwargs(**defaults)

    def test_thinking_forces_temperature_1(self):
        kwargs = self._build()
        assert kwargs["temperature"] == 1.0

    def test_thinking_never_sends_top_p(self):
        kwargs = self._build()
        assert "top_p" not in kwargs

    def test_thinking_never_sends_top_k(self):
        """top_k must not be sent when thinking is enabled."""
        kwargs = self._build(top_k=40)
        assert "top_k" not in kwargs

    def test_thinking_sets_budget(self):
        kwargs = self._build(max_tokens=20000)
        assert kwargs["thinking"]["type"] == "enabled"
        assert kwargs["thinking"]["budget_tokens"] == 10000

    def test_thinking_overrides_user_temperature(self):
        """Even if user sets temperature=0.2, thinking forces 1.0."""
        kwargs = self._build(temperature=0.2)
        assert kwargs["temperature"] == 1.0

    def test_disabled_thinking_uses_normal_sampling(self):
        """ThinkingConfig with thinking_level=None behaves like no thinking."""
        provider = _make_provider()
        kwargs = provider._build_kwargs(
            model="claude-sonnet-4-20250514",
            messages=[],
            temperature=0.5,
            max_tokens=8192,
            top_k=40,
            thinking_config=ThinkingConfig(thinking_level=None, include_thoughts=False),
        )
        assert kwargs["temperature"] == 0.5
        assert kwargs["top_k"] == 40
        assert "thinking" not in kwargs


class TestBuildUsage:
    def test_build_usage_with_tokens(self):
        from app.llm.providers.anthropic import _build_usage
        mock_usage = MagicMock()
        mock_usage.input_tokens = 100
        mock_usage.output_tokens = 50
        result = _build_usage(mock_usage)
        assert result.prompt_token_count == 100
        assert result.candidates_token_count == 50
        assert result.total_token_count == 150

    def test_build_usage_none(self):
        from app.llm.providers.anthropic import _build_usage
        result = _build_usage(None)
        assert result.prompt_token_count == 0
        assert result.candidates_token_count == 0
        assert result.total_token_count == 0


class TestBuildResponse:
    def test_parses_text_response(self):
        provider = _make_provider()
        mock_response = MagicMock()
        mock_block = MagicMock()
        mock_block.type = "text"
        mock_block.text = "Hello world"
        mock_response.content = [mock_block]
        mock_response.stop_reason = "end_turn"
        mock_response.usage = MagicMock(input_tokens=10, output_tokens=5)

        result = provider._build_response(mock_response)
        assert len(result.candidates) == 1
        assert result.candidates[0].content.parts[0].text == "Hello world"
        assert result.candidates[0].finish_reason == "end_turn"
        assert result.usage_metadata.total_token_count == 15

    def test_parses_tool_use_response(self):
        provider = _make_provider()
        mock_block = MagicMock()
        mock_block.type = "tool_use"
        mock_block.name = "run_command"
        mock_block.input = {"command": "ls"}
        mock_block.id = "tool_123"
        mock_response = MagicMock()
        mock_response.content = [mock_block]
        mock_response.stop_reason = "tool_use"
        mock_response.usage = MagicMock(input_tokens=20, output_tokens=10)

        result = provider._build_response(mock_response)
        tc = result.candidates[0].content.parts[0].tool_call
        assert tc.name == "run_command"
        assert tc.args == {"command": "ls"}
        assert tc.id == "tool_123"


class TestContentsToAnthropic:
    def test_text_content(self):
        from app.llm.providers.anthropic import _contents_to_anthropic
        contents = [Content(role="user", parts=[Part(text="hello")])]
        result = _contents_to_anthropic(contents)
        assert result == [{"role": "user", "content": [{"type": "text", "text": "hello"}]}]

    def test_model_role_mapped_to_assistant(self):
        from app.llm.providers.anthropic import _contents_to_anthropic
        contents = [Content(role="model", parts=[Part(text="hi")])]
        result = _contents_to_anthropic(contents)
        assert result[0]["role"] == "assistant"

    def test_thinking_block_with_thought_signature_object(self):
        """ThoughtSignature object must be serialized to string via str()."""
        from app.llm.providers.anthropic import _contents_to_anthropic
        sig = ThoughtSignature(value="abc123sig")
        contents = [Content(role="model", parts=[
            Part(text="deep thought", thought=True, thought_signature=sig),
        ])]
        result = _contents_to_anthropic(contents)
        block = result[0]["content"][0]
        assert block["type"] == "thinking"
        assert block["thinking"] == "deep thought"
        assert block["signature"] == "abc123sig"

    def test_thinking_block_with_none_signature(self):
        from app.llm.providers.anthropic import _contents_to_anthropic
        contents = [Content(role="model", parts=[
            Part(text="thought", thought=True, thought_signature=None),
        ])]
        result = _contents_to_anthropic(contents)
        assert result[0]["content"][0]["signature"] == ""

    def test_tool_call_block(self):
        from app.llm.providers.anthropic import _contents_to_anthropic
        tc = ToolCall(name="run_command", args={"cmd": "ls"}, id="tc_1")
        contents = [Content(role="model", parts=[Part(tool_call=tc)])]
        result = _contents_to_anthropic(contents)
        block = result[0]["content"][0]
        assert block["type"] == "tool_use"
        assert block["id"] == "tc_1"
        assert block["name"] == "run_command"

    def test_tool_response_block(self):
        from app.llm.providers.anthropic import _contents_to_anthropic
        tr = ToolResponse(name="run_command", response={"output": "ok"}, id="tc_1")
        contents = [Content(role="user", parts=[Part(tool_response=tr)])]
        result = _contents_to_anthropic(contents)
        block = result[0]["content"][0]
        assert block["type"] == "tool_result"
        assert block["tool_use_id"] == "tc_1"

    def test_empty_parts_skipped(self):
        from app.llm.providers.anthropic import _contents_to_anthropic
        contents = [Content(role="user", parts=[])]
        result = _contents_to_anthropic(contents)
        assert result == []


    def test_tool_use_id_propagated_to_tool_result(self):
        """Regression: tool_result must carry the same ID as the preceding tool_use.

        Anthropic requires every tool_result.tool_use_id to match a tool_use.id
        in the immediately preceding assistant message. Without correct ID
        propagation, the API returns 400.
        """
        from app.llm.providers.anthropic import _contents_to_anthropic
        tool_id = "toolu_01XFDUDYJgAACzvnptvVoYEL"
        contents = [
            Content(role="user", parts=[Part(text="run uptime")]),
            Content(role="model", parts=[
                Part(text="Running the command."),
                Part(tool_call=ToolCall(name="run_commands_with_operator", args={"command": "uptime"}, id=tool_id)),
            ]),
            Content(role="user", parts=[
                Part(tool_response=ToolResponse(
                    name="run_commands_with_operator",
                    response={"output": "up 1 day"},
                    id=tool_id,
                )),
            ]),
        ]
        result = _contents_to_anthropic(contents)
        tool_use_block = result[1]["content"][1]
        tool_result_block = result[2]["content"][0]
        assert tool_use_block["id"] == tool_id
        assert tool_result_block["tool_use_id"] == tool_id
        assert tool_use_block["id"] == tool_result_block["tool_use_id"]

    def test_consecutive_same_role_messages_merged(self):
        """Anthropic requires strict user/assistant alternation.

        Consecutive Content objects with the same role must be merged into
        a single message to avoid API rejection.
        """
        from app.llm.providers.anthropic import _contents_to_anthropic
        contents = [
            Content(role="user", parts=[Part(text="first")]),
            Content(role="user", parts=[Part(text="second")]),
            Content(role="model", parts=[Part(text="reply")]),
        ]
        result = _contents_to_anthropic(contents)
        assert len(result) == 2
        assert result[0]["role"] == "user"
        assert len(result[0]["content"]) == 2
        assert result[0]["content"][0]["text"] == "first"
        assert result[0]["content"][1]["text"] == "second"
        assert result[1]["role"] == "assistant"

    def test_empty_text_blocks_dropped(self):
        """Anthropic rejects zero-length text blocks."""
        from app.llm.providers.anthropic import _contents_to_anthropic
        contents = [Content(role="user", parts=[
            Part(text=""),
            Part(text="real text"),
        ])]
        result = _contents_to_anthropic(contents)
        assert len(result[0]["content"]) == 1
        assert result[0]["content"][0]["text"] == "real text"

    def test_content_with_only_empty_text_produces_no_message(self):
        from app.llm.providers.anthropic import _contents_to_anthropic
        contents = [Content(role="user", parts=[Part(text="")])]
        result = _contents_to_anthropic(contents)
        assert result == []

    def test_tool_call_fallback_id_when_none(self):
        from app.llm.providers.anthropic import _contents_to_anthropic
        tc = ToolCall(name="run_command", args={"cmd": "ls"}, id=None)
        contents = [Content(role="model", parts=[Part(tool_call=tc)])]
        result = _contents_to_anthropic(contents)
        assert result[0]["content"][0]["id"] == "toolc_run_command"

    def test_tool_response_fallback_id_when_none(self):
        from app.llm.providers.anthropic import _contents_to_anthropic
        tr = ToolResponse(name="run_command", response={"output": "ok"}, id=None)
        contents = [Content(role="user", parts=[Part(tool_response=tr)])]
        result = _contents_to_anthropic(contents)
        assert result[0]["content"][0]["tool_use_id"] == "toolc_run_command"


class TestToolsToAnthropic:
    def test_converts_tool_declarations(self):
        from app.llm.providers.anthropic import _tools_to_anthropic
        schema = Schema(type=Type.OBJECT, properties={"cmd": Schema(type=Type.STRING)})
        tools = [ToolGroup(tools=[
            ToolDeclaration(name="run_command", description="Execute", parameters=schema),
        ])]
        result = _tools_to_anthropic(tools)
        assert len(result) == 1
        assert result[0]["name"] == "run_command"
        assert "input_schema" in result[0]

    def test_none_tools_returns_none(self):
        from app.llm.providers.anthropic import _tools_to_anthropic
        assert _tools_to_anthropic(None) is None

    def test_empty_tools_returns_none(self):
        from app.llm.providers.anthropic import _tools_to_anthropic
        assert _tools_to_anthropic([]) is None


class TestPublicMethodsDelegateCorrectly:
    """Verify public methods pass correct fields from settings to _build_kwargs."""

    @pytest.mark.asyncio
    async def test_generate_content_primary_uses_build_kwargs(self):
        provider = _make_provider()
        mock_response = MagicMock()
        mock_block = MagicMock(type="text", text="ok")
        mock_response.content = [mock_block]
        mock_response.stop_reason = "end_turn"
        mock_response.usage = MagicMock(input_tokens=5, output_tokens=3)
        provider._client.messages.create = AsyncMock(return_value=mock_response)

        settings = PrimaryLLMSettings(
            temperature=0.3,
            max_output_tokens=4096,
            top_p_nucleus_sampling=0.95,
            top_k_filtering=50,
            system_instruction="test",
        )

        result = await provider.generate_content_primary(
            model="claude-sonnet-4-20250514",
            contents=[Content(role="user", parts=[Part(text="hi")])],
            primary_llm_settings=settings,
        )

        call_kwargs = provider._client.messages.create.call_args.kwargs
        assert "top_p" not in call_kwargs
        assert call_kwargs["temperature"] == 0.3
        assert call_kwargs["top_k"] == 50
        assert call_kwargs["system"] == "test"
        assert result.candidates[0].content.parts[0].text == "ok"

    @pytest.mark.asyncio
    async def test_generate_content_assistant_uses_build_kwargs(self):
        provider = _make_provider()
        mock_response = MagicMock()
        mock_block = MagicMock(type="text", text="analysis")
        mock_response.content = [mock_block]
        mock_response.stop_reason = "end_turn"
        mock_response.usage = MagicMock(input_tokens=10, output_tokens=5)
        provider._client.messages.create = AsyncMock(return_value=mock_response)

        settings = AssistantLLMSettings(
            temperature=0.5,
            max_output_tokens=2048,
            top_p_nucleus_sampling=1.0,
            top_k_filtering=40,
            system_instruction="analyze",
        )

        await provider.generate_content_assistant(
            model="claude-sonnet-4-20250514",
            contents=[Content(role="user", parts=[Part(text="check")])],
            assistant_llm_settings=settings,
        )

        call_kwargs = provider._client.messages.create.call_args.kwargs
        assert "top_p" not in call_kwargs
        assert call_kwargs["temperature"] == 0.5

    @pytest.mark.asyncio
    async def test_generate_content_lite_uses_build_kwargs(self):
        provider = _make_provider()
        mock_response = MagicMock()
        mock_block = MagicMock(type="text", text="triage")
        mock_response.content = [mock_block]
        mock_response.stop_reason = "end_turn"
        mock_response.usage = MagicMock(input_tokens=8, output_tokens=4)
        provider._client.messages.create = AsyncMock(return_value=mock_response)

        settings = LiteLLMSettings(
            temperature=0.2,
            max_output_tokens=1024,
            top_p_nucleus_sampling=1.0,
            top_k_filtering=40,
            system_instruction="triage",
        )

        await provider.generate_content_lite(
            model="claude-sonnet-4-20250514",
            contents=[Content(role="user", parts=[Part(text="event")])],
            lite_llm_settings=settings,
        )

        call_kwargs = provider._client.messages.create.call_args.kwargs
        assert "top_p" not in call_kwargs
        assert call_kwargs["temperature"] == 0.2

    @pytest.mark.asyncio
    async def test_primary_with_thinking_forces_temperature_1(self):
        """Regression: thinking mode must force temperature=1.0 and exclude top_k."""
        provider = _make_provider()
        mock_response = MagicMock()
        mock_block = MagicMock(type="text", text="thought out")
        mock_response.content = [mock_block]
        mock_response.stop_reason = "end_turn"
        mock_response.usage = MagicMock(input_tokens=50, output_tokens=200)
        provider._client.messages.create = AsyncMock(return_value=mock_response)

        settings = PrimaryLLMSettings(
            temperature=0.3,
            max_output_tokens=20000,
            top_p_nucleus_sampling=0.95,
            top_k_filtering=50,
            system_instruction="think",
            thinking_config=ThinkingConfig(
                thinking_level=ThinkingLevel.HIGH,
                include_thoughts=True,
            ),
        )

        await provider.generate_content_primary(
            model="claude-sonnet-4-20250514",
            contents=[Content(role="user", parts=[Part(text="complex problem")])],
            primary_llm_settings=settings,
        )

        call_kwargs = provider._client.messages.create.call_args.kwargs
        assert call_kwargs["temperature"] == 1.0
        assert "top_p" not in call_kwargs
        assert "top_k" not in call_kwargs
        assert call_kwargs["thinking"]["type"] == "enabled"
