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

from pydantic import ConfigDict, Field

from app.constants import ConversationStatus

from .base import G8eTimestampedModel


class Conversation(G8eTimestampedModel):
    model_config = ConfigDict(arbitrary_types_allowed=True)

    web_session_id: str = Field(..., description="Browser session identifier — primary key for this conversation")
    case_id: str | None = Field(default=None, description="Associated case ID")
    investigation_id: str | None = Field(default=None, description="Associated investigation ID")
    user_id: str | None = Field(default=None, description="User who owns this conversation")
    status: ConversationStatus = Field(default=ConversationStatus.ACTIVE, description="Lifecycle status")
    sentinel_mode: bool = Field(default=True, description="Whether sentinel (safe-mode) is enabled for this session")
    chat: Any | None = Field(default=None, exclude=True, description="Runtime LLM chat object — not serialised")

    def deactivate(self) -> None:
        self.status = ConversationStatus.INACTIVE
        self.update_timestamp()

    def complete(self) -> None:
        self.status = ConversationStatus.COMPLETED
        self.update_timestamp()

    @property
    def is_active(self) -> bool:
        return self.status == ConversationStatus.ACTIVE
