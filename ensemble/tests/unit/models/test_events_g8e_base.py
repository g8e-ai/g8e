# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Regression tests for Phase 6 — SSE wire models subclassed from g8e.models.events.

Verifies:
- SessionEventWire subclasses g8e's SessionEventWire
- BackgroundEventWire subclasses g8e's BackgroundEventWire
- _SSEEventBody is re-exported from g8e
- All 11 payload classes are re-exported from g8e
- g8e's SessionEventWire has user_id as a top-level field
"""

import pytest

from g8e.models.events import (
    BackgroundEventWire as G8eBackgroundEventWire,
)
from g8e.models.events import (
    SessionEventWire as G8eSessionEventWire,
)
from g8e.models.events import (
    _SSEEventBody as G8eSSEEventBody,
    AiProcessingStoppedPayload as G8eAiProcessingStoppedPayload,
    AIToolLifecyclePayload as G8eAIToolLifecyclePayload,
    ChatCitationsReadyPayload as G8eChatCitationsReadyPayload,
    ChatErrorPayload as G8eChatErrorPayload,
    ChatProcessingStartedPayload as G8eChatProcessingStartedPayload,
    ChatResponseChunkPayload as G8eChatResponseChunkPayload,
    ChatResponseCompletePayload as G8eChatResponseCompletePayload,
    ChatRetryPayload as G8eChatRetryPayload,
    ChatThinkingPayload as G8eChatThinkingPayload,
    ChatTurnCompletePayload as G8eChatTurnCompletePayload,
    TriageClarificationQuestionsPayload as G8eTriageClarificationQuestionsPayload,
)

from app.models.events import (
    BackgroundEventWire,
    SessionEventWire,
    _SSEEventBody,
    AiProcessingStoppedPayload,
    AIToolLifecyclePayload,
    ChatCitationsReadyPayload,
    ChatErrorPayload,
    ChatProcessingStartedPayload,
    ChatResponseChunkPayload,
    ChatResponseCompletePayload,
    ChatRetryPayload,
    ChatThinkingPayload,
    ChatTurnCompletePayload,
    TriageClarificationQuestionsPayload,
)

pytestmark = pytest.mark.unit


class TestWireModelInheritance:
    """Verify g8ee wire models subclass g8e base models."""

    def test_session_event_wire_subclasses_g8e(self):
        assert issubclass(SessionEventWire, G8eSessionEventWire)

    def test_background_event_wire_subclasses_g8e(self):
        assert issubclass(BackgroundEventWire, G8eBackgroundEventWire)


class TestSSEEventBodyReExport:
    """Verify _SSEEventBody is re-exported from g8e."""

    def test_sse_event_body_is_g8e(self):
        assert _SSEEventBody is G8eSSEEventBody


class TestPayloadReExports:
    """Verify all 11 payload classes are re-exported from g8e."""

    def test_ai_processing_stopped_payload(self):
        assert AiProcessingStoppedPayload is G8eAiProcessingStoppedPayload

    def test_ai_tool_lifecycle_payload(self):
        assert AIToolLifecyclePayload is G8eAIToolLifecyclePayload

    def test_chat_citations_ready_payload(self):
        assert ChatCitationsReadyPayload is G8eChatCitationsReadyPayload

    def test_chat_error_payload(self):
        assert ChatErrorPayload is G8eChatErrorPayload

    def test_chat_processing_started_payload(self):
        assert ChatProcessingStartedPayload is G8eChatProcessingStartedPayload

    def test_chat_response_chunk_payload(self):
        assert ChatResponseChunkPayload is G8eChatResponseChunkPayload

    def test_chat_response_complete_payload(self):
        assert ChatResponseCompletePayload is G8eChatResponseCompletePayload

    def test_chat_retry_payload(self):
        assert ChatRetryPayload is G8eChatRetryPayload

    def test_chat_thinking_payload(self):
        assert ChatThinkingPayload is G8eChatThinkingPayload

    def test_chat_turn_complete_payload(self):
        assert ChatTurnCompletePayload is G8eChatTurnCompletePayload

    def test_triage_clarification_questions_payload(self):
        assert TriageClarificationQuestionsPayload is G8eTriageClarificationQuestionsPayload


class TestG8eSessionEventWireUserIdField:
    """Verify g8e's SessionEventWire has user_id as a top-level field."""

    def test_user_id_is_top_level_field(self):
        field_names = set(G8eSessionEventWire.model_fields.keys())
        assert "user_id" in field_names

    def test_user_id_is_top_level_field_in_background(self):
        field_names = set(G8eBackgroundEventWire.model_fields.keys())
        assert "user_id" in field_names
