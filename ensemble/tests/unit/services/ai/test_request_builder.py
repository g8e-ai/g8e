# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
Unit tests for AIRequestBuilder.
"""

import pytest
from unittest.mock import MagicMock, patch

from app.services.ai.request_builder import AIRequestBuilder, ToolExecutorProtocol
from app.models.investigations import ConversationHistoryMessage, UserChatMetadata
from app.constants.message_sender import MessageSender
from app.constants import HISTORY_OMITTED_MARKER, AgentMode
import app.llm.llm_types as types
from app.errors import ConfigurationError
from app.models.settings import G8eeUserSettings, LLMSettings
from g8e.models.internal_api import EvaluationInferenceContext, InferenceModelVariant

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
        assert builder.build_contents_from_history([]).contents == []

    def test_converts_user_chat_message(self, builder):
        history = [
            ConversationHistoryMessage(
                sender=MessageSender.USER_CHAT,
                content="hello",
                metadata=UserChatMetadata(attachment_filenames=[]),
                prev_hash="0" * 64,
            )
        ]
        contents = builder.build_contents_from_history(history, sentinel_mode=False).contents
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
                prev_hash="0" * 64,
            )
        ]
        contents = builder.build_contents_from_history(history, sentinel_mode=False).contents
        assert len(contents) == 1
        text = contents[0].parts[0].text
        assert "data.csv, logs.txt" in text
        assert "what is in these files?" in text

    def test_converts_user_terminal_message(self, builder):
        history = [
            ConversationHistoryMessage(
                sender=MessageSender.USER_TERMINAL,
                content="system output content",
                prev_hash="0" * 64,
            )
        ]
        contents = builder.build_contents_from_history(history, sentinel_mode=False).contents
        assert len(contents) == 1
        assert contents[0].role == types.Role.USER
        assert "[SYSTEM OUTPUT]" in contents[0].parts[0].text
        assert "system output content" in contents[0].parts[0].text

    def test_converts_ai_assistant_message(self, builder):
        history = [
            ConversationHistoryMessage(
                sender=MessageSender.AI_ASSISTANT,
                content="I can help with that",
                prev_hash="0" * 64,
            )
        ]
        contents = builder.build_contents_from_history(history).contents
        assert len(contents) == 1
        assert contents[0].role == types.Role.MODEL
        assert contents[0].parts[0].text == "I can help with that"

    def test_scrubs_user_messages_when_sentinel_mode_true(self, builder):
        history = [
            ConversationHistoryMessage(
                sender=MessageSender.USER_CHAT,
                content="contact admin@example.com",
                prev_hash="0" * 64,
            ),
            ConversationHistoryMessage(
                sender=MessageSender.USER_TERMINAL,
                content="call 415-555-1212",
                prev_hash="1" * 64,
            ),
        ]

        result = builder.build_contents_from_history(history, sentinel_mode=True)

        assert result.contents[0].parts[0].text == "contact [EMAIL]"
        assert result.contents[1].parts[0].text == "[SYSTEM OUTPUT]\ncall [PHONE]"

    def test_records_authoritative_scrubbing_observation(self, builder):
        history = [
            ConversationHistoryMessage(
                sender=MessageSender.USER_CHAT,
                content="contact admin@example.com",
                prev_hash="0" * 64,
            )
        ]

        result = builder.build_contents_from_history(history, sentinel_mode=True)

        assert result.contents[0].parts[0].text == "contact [EMAIL]"
        assert len(result.scrubbing_observations) == 1
        observation = result.scrubbing_observations[0]
        assert observation.source == MessageSender.USER_CHAT
        assert observation.enabled is True
        assert observation.was_modified is True
        assert observation.scrub_count == 1
        assert observation.scrub_types == ["email"]
        assert observation.monotonic_end >= observation.monotonic_start
        assert len(observation.input_artifact_hash) == 64
        assert len(observation.output_artifact_hash) == 64
        assert "admin@example.com" not in observation.model_dump_json()

    def test_skips_empty_or_whitespace_messages(self, builder):
        history = [
            ConversationHistoryMessage(
                sender=MessageSender.USER_CHAT, content="", prev_hash="0" * 64
            ),
            ConversationHistoryMessage(
                sender=MessageSender.USER_CHAT, content="   ", prev_hash="1" * 64
            ),
            ConversationHistoryMessage(
                sender=MessageSender.USER_CHAT, content="\n", prev_hash="2" * 64
            ),
        ]
        assert builder.build_contents_from_history(history).contents == []

    def test_appends_attachments_to_last_user_message(self, builder):
        history = [
            ConversationHistoryMessage(
                sender=MessageSender.USER_CHAT, content="first", prev_hash="0" * 64
            ),
            ConversationHistoryMessage(
                sender=MessageSender.AI_PRIMARY, content="ai response", prev_hash="1" * 64
            ),
            ConversationHistoryMessage(
                sender=MessageSender.USER_CHAT, content="second", prev_hash="2" * 64
            ),
        ]
        att_parts = [
            types.Part.from_text(text="att1"),
            types.Part.from_text(text="att2"),
        ]
        contents = builder.build_contents_from_history(history, attachments=att_parts).contents

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
            ConversationHistoryMessage(
                sender=MessageSender.USER_CHAT, content="user message", prev_hash="0" * 64
            ),
            ConversationHistoryMessage(
                sender=MessageSender.AI_PRIMARY, content="ai message", prev_hash="1" * 64
            ),
        ]
        att_parts = [types.Part.from_text(text="att")]
        contents = builder.build_contents_from_history(history, attachments=att_parts).contents

        assert len(contents) == 2
        assert len(contents[0].parts) == 2  # USER message got the attachment
        assert contents[0].parts[1].text == "att"
        assert len(contents[1].parts) == 1  # AI message untouched

    def test_skips_unknown_sender_types(self, builder):
        history = [
            ConversationHistoryMessage(
                sender=MessageSender.SYSTEM, content="system alert", prev_hash="0" * 64
            ),
            ConversationHistoryMessage(
                sender=MessageSender.AI_TRIAGE, content="triage note", prev_hash="1" * 64
            ),
        ]
        assert builder.build_contents_from_history(history).contents == []

    def test_attachments_not_appended_if_no_user_message(self, builder):
        history = [
            ConversationHistoryMessage(
                sender=MessageSender.AI_PRIMARY, content="ai only", prev_hash="0" * 64
            ),
        ]
        att_parts = [types.Part.from_text(text="att")]
        contents = builder.build_contents_from_history(history, attachments=att_parts).contents

        assert len(contents) == 1
        assert len(contents[0].parts) == 1
        assert contents[0].parts[0].text == "ai only"


def _msg(sender, text, n):
    return ConversationHistoryMessage(sender=sender, content=text, prev_hash=f"{n:064d}")


def _five_message_history():
    """user, model, user, model, user - each message estimates to 100 tokens."""
    return [
        _msg(MessageSender.USER_CHAT, "a" * 400, 0),
        _msg(MessageSender.AI_PRIMARY, "b" * 400, 1),
        _msg(MessageSender.USER_CHAT, "c" * 400, 2),
        _msg(MessageSender.AI_PRIMARY, "d" * 400, 3),
        _msg(MessageSender.USER_CHAT, "e" * 400, 4),
    ]


def _texts(contents):
    return [c.parts[0].text for c in contents]


class TestHistoryTokenBudget:
    """History compaction: drop oldest whole messages, never the last user message."""

    def test_under_budget_is_unchanged(self, builder):
        history = _five_message_history()
        trimmed = builder.build_contents_from_history(
            history, sentinel_mode=False, history_token_budget=500
        )
        untrimmed = builder.build_contents_from_history(history, sentinel_mode=False)

        assert trimmed.contents == untrimmed.contents
        assert HISTORY_OMITTED_MARKER not in _texts(trimmed.contents)

    def test_no_budget_never_trims(self, builder):
        result = builder.build_contents_from_history(
            _five_message_history(), sentinel_mode=False, history_token_budget=None
        )
        assert len(result.contents) == 5

    def test_over_budget_drops_oldest_first_and_prepends_marker(self, builder):
        result = builder.build_contents_from_history(
            _five_message_history(), sentinel_mode=False, history_token_budget=250
        )

        texts = _texts(result.contents)
        assert texts[0] == HISTORY_OMITTED_MARKER
        assert texts[1:] == ["d" * 400, "e" * 400]
        assert texts.count(HISTORY_OMITTED_MARKER) == 1

    def test_marker_tokens_count_against_the_budget(self, builder):
        # Two 100-token messages plus the ~24-token marker is 224: one more than fits.
        result = builder.build_contents_from_history(
            _five_message_history(), sentinel_mode=False, history_token_budget=210
        )

        assert _texts(result.contents) == [HISTORY_OMITTED_MARKER, "e" * 400]

    def test_drops_whole_messages_never_splits_one(self, builder):
        result = builder.build_contents_from_history(
            _five_message_history(), sentinel_mode=False, history_token_budget=250
        )

        for text in _texts(result.contents)[1:]:
            assert text in {"a" * 400, "b" * 400, "c" * 400, "d" * 400, "e" * 400}

    def test_last_user_message_and_attachments_always_survive(self, builder):
        attachment = types.Part.from_text("ATTACHED " * 100)
        result = builder.build_contents_from_history(
            _five_message_history(),
            attachments=[attachment],
            sentinel_mode=False,
            history_token_budget=1,
        )

        assert result.contents[0].parts[0].text == HISTORY_OMITTED_MARKER
        assert result.contents[-1].role == types.Role.USER
        assert result.contents[-1].parts[0].text == "e" * 400
        assert result.contents[-1].parts[-1] is attachment

    def test_single_oversized_last_message_is_kept_as_is(self, builder):
        history = [_msg(MessageSender.USER_CHAT, "x" * 400_000, 0)]

        result = builder.build_contents_from_history(
            history, sentinel_mode=False, history_token_budget=100
        )

        assert _texts(result.contents) == ["x" * 400_000]

    def test_oversized_last_message_drops_all_history_before_it(self, builder):
        history = [*_five_message_history()[:-1], _msg(MessageSender.USER_CHAT, "x" * 400_000, 9)]

        result = builder.build_contents_from_history(
            history, sentinel_mode=False, history_token_budget=100
        )

        assert _texts(result.contents) == [HISTORY_OMITTED_MARKER, "x" * 400_000]

    def test_marker_is_a_user_message_visible_to_the_model(self, builder):
        result = builder.build_contents_from_history(
            _five_message_history(), sentinel_mode=False, history_token_budget=250
        )

        assert result.contents[0].role == types.Role.USER
        assert "omitted" in result.contents[0].parts[0].text

    def test_dropped_messages_have_no_scrubbing_observations(self, builder):
        history = [
            _msg(MessageSender.USER_CHAT, "old " + "a" * 400, 0),
            _msg(MessageSender.AI_PRIMARY, "b" * 400, 1),
            _msg(MessageSender.USER_TERMINAL, "term " + "c" * 400, 2),
            _msg(MessageSender.USER_CHAT, "latest", 3),
        ]

        result = builder.build_contents_from_history(
            history, sentinel_mode=True, history_token_budget=120
        )

        assert _texts(result.contents)[-1] == "latest"
        assert [o.source for o in result.scrubbing_observations] == [MessageSender.USER_CHAT]
        assert len(result.contents) == 2

    def test_kept_messages_keep_their_observations(self, builder):
        history = [
            _msg(MessageSender.USER_CHAT, "a" * 400, 0),
            _msg(MessageSender.USER_TERMINAL, "t" * 40, 1),
            _msg(MessageSender.USER_CHAT, "latest", 2),
        ]

        result = builder.build_contents_from_history(
            history, sentinel_mode=True, history_token_budget=40
        )

        assert [o.source for o in result.scrubbing_observations] == [
            MessageSender.USER_TERMINAL,
            MessageSender.USER_CHAT,
        ]


class TestGetGenerationConfig:
    """Tests for get_generation_config logic."""

    def test_builds_config_successfully(self, builder, mock_tool_executor):
        settings = G8eeUserSettings(
            llm=LLMSettings(
                llm_model="gemini-1.5-pro",
                llm_parallel_tool_calls=False,
            )
        )

        config = builder.get_generation_config(
            system_instructions="instructions",
            settings=settings,
            agent_mode=AgentMode.G8E_BOUND,
        )

        assert config.system_instructions == "instructions"
        assert config.max_output_tokens is None
        assert config.parallel_tool_calls is False
        mock_tool_executor.get_tools.assert_called_once_with(
            AgentMode.G8E_BOUND, "gemini-1.5-pro", evaluation_context=None
        )

    def test_raises_configuration_error_when_model_missing(self, builder):
        settings = G8eeUserSettings(
            llm=LLMSettings(llm_model="")  # Empty model
        )

        with pytest.raises(ConfigurationError, match="No LLM model configured"):
            builder.get_generation_config(
                system_instructions="...", settings=settings, agent_mode=AgentMode.G8E_BOUND
            )

    def test_uses_model_override_if_provided(self, builder, mock_tool_executor):
        settings = G8eeUserSettings(llm=LLMSettings(llm_model="default-model"))

        builder.get_generation_config(
            system_instructions="...",
            settings=settings,
            agent_mode=AgentMode.G8E_BOUND,
            model_override="override-model",
        )

        mock_tool_executor.get_tools.assert_called_with(
            AgentMode.G8E_BOUND, "override-model", evaluation_context=None
        )

    def test_passes_evaluation_context_to_tool_executor(self, builder, mock_tool_executor):
        """The scored request's context, not an env var, drives the tool-gate bypass."""
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
        settings = G8eeUserSettings(llm=LLMSettings(llm_model="default-model"))

        builder.get_generation_config(
            system_instructions="...",
            settings=settings,
            agent_mode=AgentMode.G8E_BOUND,
            model_override="qwen3.5:4b",
            evaluation_context=evaluation_context,
        )

        mock_tool_executor.get_tools.assert_called_once_with(
            AgentMode.G8E_BOUND, "qwen3.5:4b", evaluation_context=evaluation_context
        )


class TestFormatAttachmentParts:
    """Tests for format_attachment_parts logic."""

    def test_delegates_to_attachment_provider(self, builder):
        from app.models.attachments import ProcessedAttachment
        from app.constants.config import AttachmentType

        atts = [
            ProcessedAttachment(
                filename="test.txt",
                content_type="text/plain",
                attachment_type=AttachmentType.TEXT,
                base64_data="Y29udGVudA==",
                content="content",
            )
        ]

        with patch.object(builder._attachment_provider, "format_parts") as mock_format:
            mock_format.return_value = [types.Part.from_text(text="formatted")]

            res = builder.format_attachment_parts(atts)

            assert res == [types.Part.from_text(text="formatted")]
            mock_format.assert_called_once_with(atts)
