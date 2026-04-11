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
from app.models.base import VSOBaseModel
from app.constants.prompts import AgentMode
from app.models.attachments import AttachmentMetadata
from app.models.investigations import ConversationHistoryMessage
from app.llm.llm_types import ToolCall


class PrimaryRequest(VSOBaseModel):
    """Request model for the Primary agent."""
    message: str = Field(description="The user message to process.")
    investigation_id: str = Field(description="The current investigation ID.")
    conversation_history: list[ConversationHistoryMessage] = Field(
        default_factory=list, description="Full conversation history."
    )
    attachments: list[AttachmentMetadata] = Field(
        default_factory=list, description="Metadata for any attached files."
    )
    agent_mode: AgentMode | None = Field(
        default=None, description="The current agent mode (operator_bound, operator_not_bound, etc.)"
    )


class PrimaryResult(VSOBaseModel):
    """The result of a Primary agent operation."""
    content: str | None = Field(default=None, description="The primary text output of the AI.")
    tool_calls: list[ToolCall] = Field(
        default_factory=list, description="Optional list of tool calls initiated by the Primary agent."
    )
