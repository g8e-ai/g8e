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

"""Unit tests for MemoryGenerationService."""

import pytest
from datetime import datetime, UTC
from unittest.mock import AsyncMock, MagicMock, patch

from app.constants import EventType, InvestigationStatus
from app.constants.message_sender import MessageSender
from app.llm.llm_types import Content, Part, Role
from app.models.investigations import (
    AIResponseMetadata,
    ConversationHistoryMessage,
    ConversationMessageMetadata,
    InvestigationModel,
)
from app.models.memory import InvestigationMemory, MemoryAnalysis
from app.models.settings import LLMSettings, VSEUserSettings
from app.services.ai.memory_generation_service import (
    MemoryGenerationService,
    CONVERSATION_HISTORY_LIMIT,
    FALLBACK_TEXT_LIMIT,
)
from tests.fakes.fake_memory_data_service import FakeMemoryDataService


class TestMemoryGenerationServiceInit:
    """Test MemoryGenerationService initialization."""

    def test_init_with_protocol(self):
        service = MemoryGenerationService(FakeMemoryDataService())
        assert service._memory_crud is not None


class TestUpdateMemoryFromConversation:
    """Test update_memory_from_conversation method."""

    @pytest.mark.asyncio
    async def test_empty_conversation_creates_new_memory(self):
        fake_memory_crud = FakeMemoryDataService()
        service = MemoryGenerationService(fake_memory_crud)

        investigation = InvestigationModel(
            id="inv-1",
            case_id="case-1",
            user_id="user-1",
            status=InvestigationStatus.OPEN,
            case_title="Test Case",
            sentinel_mode=False,
        )
        settings = VSEUserSettings(
            llm=LLMSettings(provider="gemini", assistant_model="gemini-3.1-flash")
        )

        result = await service.update_memory_from_conversation(
            conversation_history=[],
            investigation=investigation,
            settings=settings,
        )

        assert isinstance(result, InvestigationMemory)
        assert result.investigation_id == "inv-1"
        assert result.case_id == "case-1"
        assert result.user_id == "user-1"
        assert len(fake_memory_crud.create_calls) == 1
        assert len(fake_memory_crud.save_calls) == 0

    @pytest.mark.asyncio
    async def test_empty_conversation_returns_existing_memory(self):
        fake_memory_crud = FakeMemoryDataService()
        existing_memory = InvestigationMemory(
            investigation_id="inv-1",
            case_id="case-1",
            user_id="user-1",
            status=InvestigationStatus.OPEN,
            case_title="Test Case",
            investigation_summary="Existing summary",
        )
        fake_memory_crud.set_memory_to_return(existing_memory)
        service = MemoryGenerationService(fake_memory_crud)

        investigation = InvestigationModel(
            id="inv-1",
            case_id="case-1",
            user_id="user-1",
            status=InvestigationStatus.OPEN,
            case_title="Test Case",
            sentinel_mode=False,
        )
        settings = VSEUserSettings(
            llm=LLMSettings(provider="gemini", assistant_model="gemini-3.1-flash")
        )

        result = await service.update_memory_from_conversation(
            conversation_history=[],
            investigation=investigation,
            settings=settings,
        )

        assert result.investigation_summary == "Existing summary"
        assert len(fake_memory_crud.create_calls) == 0
        assert len(fake_memory_crud.save_calls) == 0

    @pytest.mark.asyncio
    async def test_conversation_truncates_to_limit(self):
        fake_memory_crud = FakeMemoryDataService()
        service = MemoryGenerationService(fake_memory_crud)

        investigation = InvestigationModel(
            id="inv-1",
            case_id="case-1",
            user_id="user-1",
            status=InvestigationStatus.OPEN,
            case_title="Test Case",
            sentinel_mode=False,
        )

        conversation_history = [
            ConversationHistoryMessage(
                id=f"msg-{i}",
                sender=EventType.EVENT_SOURCE_USER_CHAT,
                content=f"Message {i}",
                timestamp=datetime.now(UTC),
                metadata=ConversationMessageMetadata(sentinel_mode=False),
            )
            for i in range(CONVERSATION_HISTORY_LIMIT + 10)
        ]

        settings = VSEUserSettings(
            llm=LLMSettings(provider="gemini", assistant_model="gemini-3.1-flash")
        )

        # Mock _ai_update_memory to prevent actual LLM call
        async def mock_ai_update(memory, history, settings):
            pass

        service._ai_update_memory = mock_ai_update

        await service.update_memory_from_conversation(
            conversation_history=conversation_history,
            investigation=investigation,
            settings=settings,
        )

        # Verify the slice directly - the implementation uses conversation_history[-CONVERSATION_HISTORY_LIMIT:]
        # With 30 messages and limit of 20, we should get the last 20 (messages 10-29)
        memory = InvestigationMemory(
            investigation_id="inv-1",
            case_id="case-1",
            user_id="user-1",
            status=InvestigationStatus.OPEN,
            case_title="Test Case",
            sentinel_mode=False,
        )
        contents = MemoryGenerationService._conversation_to_contents(conversation_history, memory)
        # Total contents = memory context (1) + messages (20) + analysis request (1) = 22
        assert len(contents) == 22


class TestConversationToContents:
    """Test _conversation_to_contents static method."""

    def test_includes_memory_context(self):
        memory = InvestigationMemory(
            investigation_id="inv-1",
            case_id="case-1",
            user_id="user-1",
            status=InvestigationStatus.OPEN,
            case_title="Test Case",
            sentinel_mode=False,
            investigation_summary="Test summary",
            technical_background="Test background",
        )

        contents = MemoryGenerationService._conversation_to_contents([], memory)

        assert len(contents) == 2
        assert contents[0].role == Role.USER
        assert "CURRENT MEMORY STATE" in contents[0].parts[0].text
        assert "Test summary" in contents[0].parts[0].text
        assert "Test background" in contents[0].parts[0].text

    def test_filters_thinking_messages(self):
        memory = InvestigationMemory(
            investigation_id="inv-1",
            case_id="case-1",
            user_id="user-1",
            status=InvestigationStatus.OPEN,
            case_title="Test Case",
            sentinel_mode=False,
        )

        conversation_history = [
            ConversationHistoryMessage(
                id="msg-1",
                sender=EventType.EVENT_SOURCE_USER_CHAT,
                content="Normal message",
                timestamp=datetime.now(UTC),
                metadata=ConversationMessageMetadata(sentinel_mode=False),
            ),
            ConversationHistoryMessage(
                id="msg-2",
                sender=EventType.EVENT_SOURCE_AI_PRIMARY,
                content="Thinking message",
                timestamp=datetime.now(UTC),
                metadata=AIResponseMetadata(sentinel_mode=False, is_thinking=True),
            ),
        ]

        contents = MemoryGenerationService._conversation_to_contents(conversation_history, memory)

        assert len(contents) == 3  # Memory context + normal message + analysis request (thinking filtered)
        # Extract all text from content parts
        all_text = ' '.join([part.text for content in contents for part in content.parts])
        assert "Normal message" in all_text
        assert "Thinking message" not in all_text

    def test_maps_user_chat_to_user_role(self):
        memory = InvestigationMemory(
            investigation_id="inv-1",
            case_id="case-1",
            user_id="user-1",
            status=InvestigationStatus.OPEN,
            case_title="Test Case",
            sentinel_mode=False,
        )

        conversation_history = [
            ConversationHistoryMessage(
                id="msg-1",
                sender=MessageSender.USER_CHAT,
                content="User message",
                timestamp=datetime.now(UTC),
                metadata=ConversationMessageMetadata(sentinel_mode=False),
            ),
        ]

        contents = MemoryGenerationService._conversation_to_contents(conversation_history, memory)

        assert len(contents) == 3  # Memory context + user message + analysis request
        assert contents[1].role == Role.USER

    def test_maps_ai_primary_to_model_role(self):
        memory = InvestigationMemory(
            investigation_id="inv-1",
            case_id="case-1",
            user_id="user-1",
            status=InvestigationStatus.OPEN,
            case_title="Test Case",
            sentinel_mode=False,
        )

        conversation_history = [
            ConversationHistoryMessage(
                id="msg-1",
                sender=MessageSender.AI_PRIMARY,
                content="AI message",
                timestamp=datetime.now(UTC),
                metadata=ConversationMessageMetadata(sentinel_mode=False),
            ),
        ]

        contents = MemoryGenerationService._conversation_to_contents(conversation_history, memory)

        assert len(contents) == 3  # Memory context + AI message + analysis request
        assert contents[1].role == Role.MODEL

    def test_maps_ai_assistant_to_model_role(self):
        memory = InvestigationMemory(
            investigation_id="inv-1",
            case_id="case-1",
            user_id="user-1",
            status=InvestigationStatus.OPEN,
            case_title="Test Case",
            sentinel_mode=False,
        )

        conversation_history = [
            ConversationHistoryMessage(
                id="msg-1",
                sender=MessageSender.AI_ASSISTANT,
                content="Assistant message",
                timestamp=datetime.now(UTC),
                metadata=ConversationMessageMetadata(sentinel_mode=False),
            ),
        ]

        contents = MemoryGenerationService._conversation_to_contents(conversation_history, memory)

        assert len(contents) == 3  # Memory context + AI message + analysis request
        assert contents[1].role == Role.MODEL

    def test_skips_unknown_senders(self):
        memory = InvestigationMemory(
            investigation_id="inv-1",
            case_id="case-1",
            user_id="user-1",
            status=InvestigationStatus.OPEN,
            case_title="Test Case",
            sentinel_mode=False,
        )

        conversation_history = [
            ConversationHistoryMessage(
                id="msg-1",
                sender=EventType.EVENT_SOURCE_SYSTEM,
                content="System message",
                timestamp=datetime.now(UTC),
                metadata=ConversationMessageMetadata(sentinel_mode=False),
            ),
        ]

        contents = MemoryGenerationService._conversation_to_contents(conversation_history, memory)

        assert len(contents) == 2
        assert "System message" not in str(contents)

    def test_includes_analysis_request(self):
        memory = InvestigationMemory(
            investigation_id="inv-1",
            case_id="case-1",
            user_id="user-1",
            status=InvestigationStatus.OPEN,
            case_title="Test Case",
            sentinel_mode=False,
        )

        contents = MemoryGenerationService._conversation_to_contents([], memory)

        assert len(contents) == 2
        assert contents[1].role == Role.USER


class TestExtractJsonFromMarkdown:
    """Test _extract_json_from_markdown static method."""

    def test_extracts_json_from_markdown_block(self):
        text = '```json\n{"key": "value"}\n```'
        result = MemoryGenerationService._extract_json_from_markdown(text)
        assert result == '{"key": "value"}'

    def test_extracts_json_without_language_tag(self):
        text = '```\n{"key": "value"}\n```'
        result = MemoryGenerationService._extract_json_from_markdown(text)
        assert result == '{"key": "value"}'

    def test_extracts_json_from_multiline(self):
        text = '```json\n{\n  "key": "value"\n}\n```'
        result = MemoryGenerationService._extract_json_from_markdown(text)
        assert result == '{\n  "key": "value"\n}'

    def test_returns_none_when_no_json_block(self):
        text = 'Plain text without json'
        result = MemoryGenerationService._extract_json_from_markdown(text)
        assert result is None

    def test_returns_none_for_empty_string(self):
        result = MemoryGenerationService._extract_json_from_markdown('')
        assert result is None

    def test_ignores_non_object_blocks(self):
        text = '```\njust text\n```'
        result = MemoryGenerationService._extract_json_from_markdown(text)
        assert result is None

    def test_handles_array_block(self):
        text = '```json\n["item1", "item2"]\n```'
        result = MemoryGenerationService._extract_json_from_markdown(text)
        assert result == '["item1", "item2"]'


class TestExtractKeyValuePairs:
    """Test _extract_key_value_pairs static method."""

    def test_extracts_simple_key_value(self):
        text = 'key: value'
        result = MemoryGenerationService._extract_key_value_pairs(text)
        assert result == {'key': 'value'}

    def test_extracts_quoted_key(self):
        text = '"key": "value"'
        result = MemoryGenerationService._extract_key_value_pairs(text)
        assert result == {'key': 'value'}

    def test_extracts_single_quoted_value(self):
        text = "key: 'value'"
        result = MemoryGenerationService._extract_key_value_pairs(text)
        assert result == {'key': 'value'}

    def test_removes_trailing_comma(self):
        text = 'key: value,'
        result = MemoryGenerationService._extract_key_value_pairs(text)
        assert result == {'key': 'value'}

    def test_handles_multiline_values(self):
        text = 'key: value\ncontinued'
        result = MemoryGenerationService._extract_key_value_pairs(text)
        assert result == {'key': 'value continued'}

    def test_normalizes_key_to_snake_case(self):
        text = 'Key Name: value'
        result = MemoryGenerationService._extract_key_value_pairs(text)
        assert 'key_name' in result

    def test_skips_comment_lines(self):
        text = '# comment\nkey: value\n// another comment'
        result = MemoryGenerationService._extract_key_value_pairs(text)
        assert result == {'key': 'value'}

    def test_skips_empty_lines(self):
        text = '\n\nkey: value\n\n'
        result = MemoryGenerationService._extract_key_value_pairs(text)
        assert result == {'key': 'value'}

    def test_handles_multiple_pairs(self):
        text = 'key1: value1\nkey2: value2'
        result = MemoryGenerationService._extract_key_value_pairs(text)
        assert result == {'key1': 'value1', 'key2': 'value2'}

    def test_dash_prefixed_lines_treated_as_continuation(self):
        text = '- item\nkey: value\n- another item'
        result = MemoryGenerationService._extract_key_value_pairs(text)
        # Dash-prefixed lines are treated as continuation lines when they appear after a key
        # The dash is preserved in the continuation
        assert result == {'key': 'value - another item'}


class TestParseMemoryAnalysis:
    """Test _parse_memory_analysis method."""

    def setup_method(self):
        self.service = MemoryGenerationService(FakeMemoryDataService())

    def test_parses_valid_json(self):
        text = '{"investigation_summary": "Test summary"}'
        result = self.service._parse_memory_analysis(text)
        assert result.investigation_summary == "Test summary"

    def test_parses_json_from_markdown(self):
        text = '```json\n{"technical_background": "Test background"}\n```'
        result = self.service._parse_memory_analysis(text)
        assert result.technical_background == "Test background"

    def test_fallback_to_key_value_extraction(self):
        text = 'technical_background: Test background\ncommunication_preferences: email'
        result = self.service._parse_memory_analysis(text)
        assert result.technical_background == "Test background"
        assert result.communication_preferences == "email"

    def test_maps_field_aliases(self):
        text = 'summary: Test\nbackground: Test background'
        result = self.service._parse_memory_analysis(text)
        assert result.investigation_summary == "Test"
        assert result.technical_background == "Test background"

    def test_fallback_raw_text_to_summary(self):
        text = 'Plain text without structure'
        result = self.service._parse_memory_analysis(text)
        assert result.investigation_summary == "Plain text without structure"

    def test_fallback_truncates_long_text(self):
        long_text = "x" * (FALLBACK_TEXT_LIMIT + 100)
        result = self.service._parse_memory_analysis(long_text)
        assert len(result.investigation_summary) == FALLBACK_TEXT_LIMIT

    def test_fallback_strips_braces_from_raw_text(self):
        text = '{raw text inside braces}'
        result = self.service._parse_memory_analysis(text)
        assert result.investigation_summary == "raw text inside braces"

    def test_fallback_strips_brackets_from_raw_text(self):
        text = '[raw text inside brackets]'
        result = self.service._parse_memory_analysis(text)
        assert result.investigation_summary == "raw text inside brackets"

    def test_returns_empty_analysis_for_empty_string(self):
        result = self.service._parse_memory_analysis('')
        assert result.investigation_summary == ''
        assert result.technical_background == ''

    def test_returns_empty_analysis_for_whitespace_only(self):
        result = self.service._parse_memory_analysis('   ')
        assert result.investigation_summary == ''
        assert result.technical_background == ''

    def test_key_value_fallback_when_json_fails(self):
        text = 'summary: value'
        result = self.service._parse_memory_analysis(text)
        assert result.investigation_summary == "value"

    def test_all_field_aliases_mapped(self):
        text = (
            'summary: Test\n'
            'communication: email\n'
            'background: tech\n'
            'response: formal\n'
            'approach: systematic\n'
            'interaction: questions'
        )
        result = self.service._parse_memory_analysis(text)
        assert result.investigation_summary == "Test"
        assert result.communication_preferences == "email"
        assert result.technical_background == "tech"
        assert result.response_style == "formal"
        assert result.problem_solving_approach == "systematic"
        assert result.interaction_style == "questions"


class TestConstants:
    """Test module-level constants."""

    def test_conversation_history_limit_is_defined(self):
        assert CONVERSATION_HISTORY_LIMIT == 20

    def test_fallback_text_limit_is_defined(self):
        assert FALLBACK_TEXT_LIMIT == 2000
