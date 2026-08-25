# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from app.models.base import G8eBaseModel, Field
from app.constants.prompts import AgentMode
from app.models.attachments import AttachmentMetadata
from app.models.investigations import ConversationHistoryMessage
from app.llm.llm_dataclasses import ToolCall


class PrimaryRequest(G8eBaseModel):
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
        default=None,
        description="The current agent mode (g8e.bound, g8e.not.bound, etc.)",
    )


class PrimaryResult(G8eBaseModel):
    """The result of a Primary agent operation."""

    content: str | None = Field(default=None, description="The primary text output of the AI.")
    tool_calls: list[ToolCall] = Field(
        default_factory=list,
        description="Optional list of tool calls initiated by the Primary agent.",
    )
