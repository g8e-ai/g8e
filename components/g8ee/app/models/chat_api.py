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


from pydantic import Field

from app.constants import ChatSessionStatus, EntityType
from app.models.base import G8eBaseModel, UTCDatetime
from app.models.investigations import ConversationHistoryMessage


class ChatSessionResponse(G8eBaseModel):
    """Response model for GET /chat/sessions/{web_session_id}."""
    web_session_id: str = Field(..., description="Chat session identifier")
    created_at: UTCDatetime = Field(..., description="Session creation timestamp")
    status: ChatSessionStatus = Field(..., description="Session status")
    case_id: str | None = Field(default=None, description="Associated case ID")
    entity_type: EntityType = Field(default=EntityType.MESSAGE, description="Entity type")


class ChatSessionDetailsResponse(G8eBaseModel):
    """Detailed session response including conversation history."""
    id: str = Field(..., description="Session identifier")
    case_id: str = Field(..., description="Associated case ID")
    investigation_id: str = Field(..., description="Associated investigation ID")
    conversation_history: list[ConversationHistoryMessage] = Field(default_factory=list, description="Session conversation messages")
    created_at: UTCDatetime | None = Field(default=None, description="Session creation timestamp")
    updated_at: UTCDatetime | None = Field(default=None, description="Last update timestamp")
    is_active: bool = Field(..., description="Whether the session is active")


class LatestChatSessionResponse(G8eBaseModel):
    """Response model for GET /chat/cases/{case_id}/latest-session."""
    success: bool = Field(..., description="Whether the request was successful")
    session: ChatSessionDetailsResponse | None = Field(default=None, description="Latest chat session data")
    message: str = Field(..., description="Response message")
