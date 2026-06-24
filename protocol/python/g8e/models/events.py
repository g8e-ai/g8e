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

from typing import Any
from .base import G8eBaseModel, UTCDatetime

class _SSEEventBody(G8eBaseModel):
    type: str
    data: dict[str, Any]

class SessionEventWire(G8eBaseModel):
    web_session_id: str | None = None
    cli_session_id: str | None = None
    user_id: str
    event: _SSEEventBody

class BackgroundEventWire(G8eBaseModel):
    user_id: str
    event: _SSEEventBody

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
    complexity: str
    complexity_confidence: str
    intent: str
    intent_confidence: str
    intent_summary: str
    request_posture: str
    posture_confidence: str
