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
g8ed HTTP client request and response models.
"""

from datetime import datetime
from typing import Any

from pydantic import Field

from app.constants import ToolCallStatus, ThinkingActionType
from app.utils.timestamp import now

from .base import G8eBaseModel, UTCDatetime


class SSEPushResponse(G8eBaseModel):
    """Response from POST /api/internal/sse/push.

    Mirrors g8ed internal_sse_routes.js: { success, delivered, error }.

    ``success`` indicates the push was accepted and processed. ``delivered`` is
    the count of active SSE connections the event was fanned out to — zero is a
    legitimate outcome for a BackgroundEvent when the user has no connected
    sessions, not an error.

    Canonical wire shape: shared/models/wire/sse_responses.json (sse_push_response)
    """

    success: bool = Field(description="Whether the push was accepted and processed")
    delivered: int = Field(default=0, ge=0, description="Number of SSE connections the event was delivered to")
    error: str | None = Field(default=None, description="Error message when unsuccessful")


class GrantIntentResponse(G8eBaseModel):
    """Response from POST /api/internal/operators/:operatorId/grant-intent.

    Mirrors g8ed internal_operator_routes.js: { success, message, operator_id, granted_intents, expires_at }
    """

    success: bool = Field(description="Whether the intent was granted")
    message: str | None = Field(default=None, description="Human-readable result message")
    operator_id: str | None = Field(default=None, description="Operator ID")
    granted_intents: list[str] = Field(default_factory=list, description="All currently granted intents after this operation")
    expires_at: UTCDatetime | None = Field(default=None, description="Expiry of the granted intent")
    error: str | None = Field(default=None, description="Error message when unsuccessful")


class RevokeIntentResponse(G8eBaseModel):
    """Response from POST /api/internal/operators/:operatorId/revoke-intent.

    Mirrors g8ed internal_operator_routes.js: { success, message, operator_id, granted_intents }
    """

    success: bool = Field(description="Whether the intent was revoked")
    message: str | None = Field(default=None, description="Human-readable result message")
    operator_id: str | None = Field(default=None, description="Operator ID")
    granted_intents: list[str] = Field(default_factory=list, description="All currently granted intents after this operation")
    error: str | None = Field(default=None, description="Error message when unsuccessful")


class IntentRequestPayload(G8eBaseModel):
    """Request payload for grant-intent and revoke-intent endpoints."""

    intent: str = Field(description="Intent name to grant or revoke")


class ChatThinkingPayload(G8eBaseModel):
    """Payload for EventType.LLM_CHAT_ITERATION_THINKING_STARTED."""

    thinking: str | None = Field(default=None, description="Thinking text delta")
    action_type: ThinkingActionType = Field(description="Phase marker: start, update, or end")


class AISearchWebPayload(G8eBaseModel):
    """Payload for EventType.LLM_TOOL_G8E_WEB_SEARCH_REQUESTED/COMPLETED/FAILED."""

    query: str | None = Field(default=None, description="Search query string")
    execution_id: str | None = Field(default=None, description="Execution ID for correlation")
    status: ToolCallStatus = Field(default=ToolCallStatus.STARTED, description="Execution status")


class OperatorNetworkPortCheckPayload(G8eBaseModel):
    """Payload for EventType.OPERATOR_NETWORK_PORT_CHECK_REQUESTED/COMPLETED/FAILED."""

    port: str | None = Field(default=None, description="Port being checked")
    execution_id: str | None = Field(default=None, description="Execution ID for correlation")
    status: ToolCallStatus = Field(default=ToolCallStatus.STARTED, description="Execution status")


class ChatProcessingStartedPayload(G8eBaseModel):
    """Payload for EventType.LLM_CHAT_ITERATION_STARTED.

    Emitted at the start of SSE delivery to signal that AI processing has begun.
    The frontend uses this to show a waiting indicator before the first text chunk arrives.
    """

    agent_mode: str | None = Field(default=None, description="Workflow type (e.g., 'default', 'sentinel')")


class ChatTurnCompletePayload(G8eBaseModel):
    """Payload for EventType.LLM_CHAT_ITERATION_COMPLETED.

    Emitted at the inter-turn boundary: after a tool result is processed and
    before the next LLM turn begins. Signals the frontend to seal the current
    streaming bubble so that post-tool text opens a fresh one.
    """

    turn: int = Field(description="1-based turn index that just completed")


class ChatResponseChunkPayload(G8eBaseModel):
    """Payload for EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED."""

    content: str | None = Field(default=None, description="Streaming text token")


class ChatCitationsReadyPayload(G8eBaseModel):
    """Payload for EventType.LLM_CHAT_ITERATION_CITATIONS_RECEIVED."""

    grounding_metadata: dict[str, Any] = Field(default_factory=dict, description="Grounding metadata wire object")


class ChatResponseCompletePayload(G8eBaseModel):
    """Payload for EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED."""

    content: str | None = Field(default=None, description="Complete AI response text")
    finish_reason: str = Field(description="Finish reason from LLM")
    has_citations: bool = Field(default=False, description="Whether grounding citations are present")
    grounding_metadata: dict[str, Any] = Field(default_factory=dict, description="Grounding metadata wire object")
    token_usage: dict[str, Any] = Field(default_factory=dict, description="Token usage wire object")
    agent_mode: str | None = Field(default=None, description="Workflow type")


class ChatErrorPayload(G8eBaseModel):
    """Payload for EventType.LLM_CHAT_ITERATION_FAILED."""

    error: str = Field(description="Error message")


class ChatRetryPayload(G8eBaseModel):
    """Payload for EventType.LLM_CHAT_ITERATION_RETRY."""

    attempt: int = Field(description="Current retry attempt number (1-indexed)")
    max_attempts: int = Field(description="Maximum number of retry attempts")


class ChatToolCallPayload(G8eBaseModel):
    """Payload for EventType.LLM_CHAT_ITERATION_TOOL_CALL_STARTED/COMPLETED."""

    tool_name: str | None = Field(default=None, description="Name of the tool being called")
    display_label: str | None = Field(default=None, description="Human-readable label for the tool")
    display_icon: str | None = Field(default=None, description="Icon identifier for the tool")
    display_detail: str | None = Field(default=None, description="Additional detail (e.g., file path, query)")
    category: str | None = Field(default=None, description="Tool category (file, search, network, etc.)")
    execution_id: str | None = Field(default=None, description="Execution ID for correlation")
    status: ToolCallStatus = Field(default=ToolCallStatus.STARTED, description="Execution status")


class AiProcessingStoppedPayload(G8eBaseModel):
    """Payload for EventType.LLM_CHAT_ITERATION_STOPPED."""

    reason: str | None = Field(default=None, description="Stop reason")
    timestamp: UTCDatetime | None = Field(default=None, description="When the stop event occurred (UTC)")


class IntentDeniedPayload(G8eBaseModel):
    """Payload for EventType.OPERATOR_INTENT_DENIED."""

    execution_id: str = Field(description="Execution ID")
    intent_name: str = Field(description="Primary intent name")
    all_intents: list[str] = Field(default_factory=list, description="All intents in the request")
    operation_context: str | None = Field(default=None, description="Operation context")
    granted: bool = Field(default=False)
    status: str = Field(default="denied")
    reason: str | None = Field(default=None, description="Denial reason")
    operator_id: str | None = Field(default=None, description="Operator ID")
    timestamp: UTCDatetime = Field(default_factory=now)


class IntentGrantedPayload(G8eBaseModel):
    """Payload for EventType.OPERATOR_INTENT_GRANTED."""

    execution_id: str = Field(description="Execution ID")
    intent_name: str = Field(description="Primary intent name")
    all_intents: list[str] = Field(default_factory=list, description="All intents in the request")
    operation_context: str | None = Field(default=None, description="Operation context")
    granted: bool = Field(default=True)
    status: str = Field(default="granted")
    operator_id: str | None = Field(default=None, description="Operator ID")
    timestamp: UTCDatetime = Field(default_factory=now)


class IntentRevokedPayload(G8eBaseModel):
    """Payload for EventType.OPERATOR_INTENT_REVOKED."""

    execution_id: str = Field(description="Execution ID")
    revoked_intents: list[str] = Field(default_factory=list, description="Revoked intent names")
    justification: str | None = Field(default=None, description="Revocation justification")
    operator_id: str | None = Field(default=None, description="Operator ID")
    timestamp: UTCDatetime = Field(default_factory=now)


class IntentOperationResult(G8eBaseModel):
    """Result of a grant-intent or revoke-intent operation against g8ed."""

    success: bool = Field(description="Whether the operation succeeded")
    granted_intents: list[str] = Field(default_factory=list, description="Current granted intents after the operation")
    error: str | None = Field(default=None, description="Error message if the operation failed")
