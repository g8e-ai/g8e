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
Unit tests for AIRequestBuilder.
"""

import pytest
from unittest.mock import MagicMock, patch

from app.services.ai.request_builder import AIRequestBuilder, ToolExecutorProtocol
from app.models.investigations import ConversationHistoryMessage, UserChatMetadata
from app.constants.message_sender import MessageSender
from app.constants import AgentMode
import app.llm.llm_types as types
from app.errors import ConfigurationError
from app.models.settings import G8eeUserSettings, LLMSettings

pytestmark = [pytest.mark.unit]


@pytest.fixture
def mock_tool_executor():
    executor = MagicMock(spec=ToolExecutorProtocol)
    executor.get_tools.return_value = []
    # Use a property mock for g8e_web_search_available
    type(executor).g8e_web_search_available = MagicMock(return_value=True)
    return executor


@pytest.fixture
def builder(mock_tool_executor):
    return AIRequestBuilder(tool_executor=mock_tool_executor)


class TestBuildContentsFromHistory:
    """Tests for build_contents_from_history logic."""

    def test_empty_history_returns_empty_list(self, builder):
        assert builder.build_contents_from_history([]) == []

    def test_converts_user_chat_message(self, builder):
        history = [
            ConversationHistoryMessage(
                sender=MessageSender.USER_CHAT,
                content="hello",
                metadata=UserChatMetadata(attachment_filenames=[]),
                prev_hash="0" * 64
            )
        ]
        contents = builder.build_contents_from_history(history, sentinel_mode=False)
        assert len(contents) == 1
        assert contents[0].role == types.Role.USER
        assert len(contents[0].parts) == 1
        assert contents[0].parts[0].text == "hello"

    def test_converts_user_chat_message_with_attachment_filenames(self, builder):
        history = [
            ConversationHistoryMessage(
                sender=MessageSender.USER_CHAT,
                content="what is in these files?",
                metadata=UserChatMetadata(attachment_filenames=["data.csv", "logs.txt"]),
                prev_hash="0" * 64
            )
        ]
        contents = builder.build_contents_from_history(history, sentinel_mode=False)
        assert len(contents) == 1
        text = contents[0].parts[0].text
        assert "data.csv, logs.txt" in text
        assert "what is in these files?" in text

    def test_converts_user_terminal_message(self, builder):
        history = [
            ConversationHistoryMessage(
                sender=MessageSender.USER_TERMINAL,
                content="system output content",
                prev_hash="0" * 64
            )
        ]
        contents = builder.build_contents_from_history(history, sentinel_mode=False)
        assert len(contents) == 1
        assert contents[0].role == types.Role.USER
        assert "[SYSTEM OUTPUT]" in contents[0].parts[0].text
        assert "system output content" in contents[0].parts[0].text

    def test_converts_ai_assistant_message(self, builder):
        history = [
            ConversationHistoryMessage(
                sender=MessageSender.AI_ASSISTANT,
                content="I can help with that",
                prev_hash="0" * 64
            )
        ]
        contents = builder.build_contents_from_history(history)
        assert len(contents) == 1
        assert contents[0].role == types.Role.MODEL
        assert contents[0].parts[0].text == "I can help with that"

    def test_scrubs_user_messages_when_sentinel_mode_true(self, builder):
        with patch("app.services.ai.request_builder.scrub_user_message") as mock_scrub:
            mock_scrub.side_effect = lambda x: f"scrubbed({x})"
            history = [
                ConversationHistoryMessage(
                    sender=MessageSender.USER_CHAT,
                    content="sensitive info",
                    prev_hash="0" * 64
                ),
                ConversationHistoryMessage(
                    sender=MessageSender.USER_TERMINAL,
                    content="more sensitive info",
                    prev_hash="1" * 64
                )
            ]
            contents = builder.build_contents_from_history(history, sentinel_mode=True)
            assert contents[0].parts[0].text == "scrubbed(sensitive info)"
            assert contents[1].parts[0].text == "[SYSTEM OUTPUT]\nscrubbed(more sensitive info)"

    def test_skips_empty_or_whitespace_messages(self, builder):
        history = [
            ConversationHistoryMessage(sender=MessageSender.USER_CHAT, content="", prev_hash="0" * 64),
            ConversationHistoryMessage(sender=MessageSender.USER_CHAT, content="   ", prev_hash="1" * 64),
            ConversationHistoryMessage(sender=MessageSender.USER_CHAT, content="\n", prev_hash="2" * 64),
        ]
        assert builder.build_contents_from_history(history) == []

    def test_appends_attachments_to_last_user_message(self, builder):
        history = [
            ConversationHistoryMessage(sender=MessageSender.USER_CHAT, content="first", prev_hash="0" * 64),
            ConversationHistoryMessage(sender=MessageSender.AI_PRIMARY, content="ai response", prev_hash="1" * 64),
            ConversationHistoryMessage(sender=MessageSender.USER_CHAT, content="second", prev_hash="2" * 64),
        ]
        att_parts = [
            types.Part.from_text(text="att1"),
            types.Part.from_text(text="att2"),
        ]
        contents = builder.build_contents_from_history(history, attachments=att_parts)
        
        assert len(contents) == 3
        # Last message (USER) should have original text part + 2 attachment parts
        assert contents[2].role == types.Role.USER
        assert len(contents[2].parts) == 3
        assert contents[2].parts[0].text == "second"
        assert contents[2].parts[1].text == "att1"
        assert contents[2].parts[2].text == "att2"

    def test_appends_attachments_to_correct_user_message_even_if_not_last(self, builder):
        # This case is unlikely in real usage but tests the logic of searching backwards
        history = [
            ConversationHistoryMessage(sender=MessageSender.USER_CHAT, content="user message", prev_hash="0" * 64),
            ConversationHistoryMessage(sender=MessageSender.AI_PRIMARY, content="ai message", prev_hash="1" * 64),
        ]
        att_parts = [types.Part.from_text(text="att")]
        contents = builder.build_contents_from_history(history, attachments=att_parts)
        
        assert len(contents) == 2
        assert len(contents[0].parts) == 2  # USER message got the attachment
        assert contents[0].parts[1].text == "att"
        assert len(contents[1].parts) == 1  # AI message untouched

    def test_skips_unknown_sender_types(self, builder):
        history = [
            ConversationHistoryMessage(sender=MessageSender.SYSTEM, content="system alert", prev_hash="0" * 64),
            ConversationHistoryMessage(sender=MessageSender.AI_TRIAGE, content="triage note", prev_hash="1" * 64),
        ]
        assert builder.build_contents_from_history(history) == []

    def test_attachments_not_appended_if_no_user_message(self, builder):
        history = [
            ConversationHistoryMessage(sender=MessageSender.AI_PRIMARY, content="ai only", prev_hash="0" * 64),
        ]
        att_parts = [types.Part.from_text(text="att")]
        contents = builder.build_contents_from_history(history, attachments=att_parts)
        
        assert len(contents) == 1
        assert len(contents[0].parts) == 1
        assert contents[0].parts[0].text == "ai only"


class TestGetGenerationConfig:
    """Tests for get_generation_config logic."""

    def test_builds_config_successfully(self, builder, mock_tool_executor):
        settings = G8eeUserSettings(
            llm=LLMSettings(llm_model="gemini-1.5-pro", llm_max_tokens=2048)
        )
        
        config = builder.get_generation_config(
            system_instructions="instructions",
            settings=settings,
            agent_mode=AgentMode.OPERATOR_BOUND
        )
        
        assert config.system_instructions == "instructions"
        assert config.max_output_tokens == 2048
        mock_tool_executor.get_tools.assert_called_once_with(AgentMode.OPERATOR_BOUND, "gemini-1.5-pro")

    def test_raises_configuration_error_when_model_missing(self, builder):
        settings = G8eeUserSettings(
            llm=LLMSettings(llm_model="") # Empty model
        )
        
        with pytest.raises(ConfigurationError, match="No LLM model configured"):
            builder.get_generation_config(
                system_instructions="...",
                settings=settings,
                agent_mode=AgentMode.OPERATOR_BOUND
            )

    def test_uses_model_override_if_provided(self, builder, mock_tool_executor):
        settings = G8eeUserSettings(
            llm=LLMSettings(llm_model="default-model")
        )
        
        builder.get_generation_config(
            system_instructions="...",
            settings=settings,
            agent_mode=AgentMode.OPERATOR_BOUND,
            model_override="override-model"
        )
        
        mock_tool_executor.get_tools.assert_called_with(AgentMode.OPERATOR_BOUND, "override-model")


class TestFormatAttachmentParts:
    """Tests for format_attachment_parts logic."""

    def test_delegates_to_attachment_provider(self, builder):
        from app.models.attachments import ProcessedAttachment
        from app.constants.settings import AttachmentType
        
        atts = [
            ProcessedAttachment(
                filename="test.txt",
                content_type="text/plain",
                attachment_type=AttachmentType.TEXT,
                base64_data="Y29udGVudA==",
                content="content"
            )
        ]
        
        with patch.object(builder._attachment_provider, "format_parts") as mock_format:
            mock_format.return_value = [types.Part.from_text(text="formatted")]
            
            res = builder.format_attachment_parts(atts)
            
            assert res == [types.Part.from_text(text="formatted")]
            mock_format.assert_called_once_with(atts)
