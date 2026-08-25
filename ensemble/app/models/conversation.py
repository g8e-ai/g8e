# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from typing import Any

from app.constants import ConversationStatus

from .base import G8eTimestampedModel, ConfigDict, Field


class Conversation(G8eTimestampedModel):
    model_config = ConfigDict(arbitrary_types_allowed=True)

    web_session_id: str = Field(
        ..., description="Browser session identifier - primary key for this conversation"
    )
    case_id: str | None = Field(default=None, description="Associated case ID")
    investigation_id: str | None = Field(default=None, description="Associated investigation ID")
    user_id: str | None = Field(default=None, description="User who owns this conversation")
    status: ConversationStatus = Field(
        default=ConversationStatus.ACTIVE, description="Lifecycle status"
    )
    sentinel_mode: bool = Field(
        default=True, description="Whether sentinel (safe-mode) is enabled for this session"
    )
    chat: Any | None = Field(
        default=None, exclude=True, description="Runtime LLM chat object - not serialised"
    )

    def deactivate(self) -> None:
        self.status = ConversationStatus.INACTIVE
        self.update_timestamp()

    def complete(self) -> None:
        self.status = ConversationStatus.COMPLETED
        self.update_timestamp()

    @property
    def is_active(self) -> bool:
        return self.status == ConversationStatus.ACTIVE
