# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from typing import Any

from pydantic import Field

from .base import G8eBaseModel, UTCDatetime
from g8e.enums import EventType

class _SSEEventBody(G8eBaseModel):
    type: EventType
    data: dict[str, Any]


class SessionEventWire(G8eBaseModel):
    web_session_id: str | None = None
    cli_session_id: str | None = None
    user_id: str
    event: _SSEEventBody

    @classmethod
    def from_session_event(
        cls,
        event_type: str,
        data: dict[str, Any],
        *,
        web_session_id: str | None = None,
        cli_session_id: str | None = None,
        user_id: str | None = None,
    ) -> "SessionEventWire":
        return cls(
            web_session_id=web_session_id,
            cli_session_id=cli_session_id,
            user_id=user_id or "",
            event=_SSEEventBody(type=event_type, data=data),
        )


class BackgroundEventWire(G8eBaseModel):
    user_id: str
    event: _SSEEventBody

    @classmethod
    def from_background_event(
        cls,
        event_type: str,
        data: dict[str, Any],
        *,
        user_id: str | None = None,
    ) -> "BackgroundEventWire":
        return cls(
            user_id=user_id or "",
            event=_SSEEventBody(type=event_type, data=data),
        )
# AI SSE event payloads (Wire shapes)
class AiProcessingStoppedPayload(G8eBaseModel):
    reason: str
    timestamp: UTCDatetime

class AIToolLifecyclePayload(G8eBaseModel):
    tool_name: str
    display_label: str | None = None
    display_icon: str | None = None
    display_detail: str | None = None
    category: str | None = None
    execution_id: str
    status: str
    query: str | None = None
    content: str | None = None
    results: list[dict[str, Any]] | None = None
    error: str | None = None
    port: str | None = None
    host: str | None = None
    is_open: bool | None = None
    timestamp: str | None = None

class ChatCitationsReadyPayload(G8eBaseModel):
    grounding_metadata: dict[str, Any]
    timestamp: str | None = None

class ChatErrorPayload(G8eBaseModel):
    error: str
    timestamp: str | None = None

class ChatProcessingStartedPayload(G8eBaseModel):
    agent_mode: str
    timestamp: str | None = None

class ChatResponseChunkPayload(G8eBaseModel):
    content: str
    timestamp: str | None = None

class ChatResponseCompletePayload(G8eBaseModel):
    content: str
    finish_reason: str
    has_citations: bool
    grounding_metadata: dict[str, Any]
    token_usage: dict[str, Any]
    agent_mode: str
    model_calls: list[dict[str, Any]] = Field(default_factory=list)
    timestamp: str | None = None

class ChatRetryPayload(G8eBaseModel):
    attempt: int
    max_attempts: int
    timestamp: str | None = None

class ChatThinkingPayload(G8eBaseModel):
    thinking: str | None
    action_type: str
    timestamp: str | None = None

class ChatTurnCompletePayload(G8eBaseModel):
    turn: int
    timestamp: str | None = None

class TriageClarificationQuestionsPayload(G8eBaseModel):
    questions: list[str]
    complexity: str | None = None
    complexity_confidence: str | None = None
    intent: str | None = None
    intent_confidence: str | None = None
    intent_summary: str | None = None
    request_posture: str | None = None
    posture_confidence: str | None = None
