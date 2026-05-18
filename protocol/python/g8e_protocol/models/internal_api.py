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
from .base import G8eBaseModel, Field
from .context import RequestContext

class ResourceCreationRequest(G8eBaseModel):
    """Typed request to create new case and investigation resources."""
    create_case: bool = Field(default=False)
    case_title: str | None = Field(default=None)

class SettingsGetRequest(G8eBaseModel):
    """Request model for GET /settings/user."""
    context: RequestContext = Field(..., description="Request context with session/user/organization identity")

class ChatMessageRequest(G8eBaseModel):
    """Request model for chat messages."""
    context: RequestContext = Field(...)
    message: str = Field(...)
    attachments: list[dict[str, Any]] | None = Field(default_factory=list)
    sentinel_mode: bool = Field(default=True)
    resource_creation: ResourceCreationRequest | None = Field(default=None)
    llm_primary_provider: str | None = Field(default=None)
    llm_assistant_provider: str | None = Field(default=None)
    llm_lite_provider: str | None = Field(default=None)
    llm_primary_model: str | None = Field(default=None)
    llm_assistant_model: str | None = Field(default=None)
    llm_lite_model: str | None = Field(default=None)
    llm_primary_api_key: str | None = Field(default=None)
    llm_primary_endpoint: str | None = Field(default=None)
    llm_assistant_api_key: str | None = Field(default=None)
    llm_assistant_endpoint: str | None = Field(default=None)
    llm_lite_api_key: str | None = Field(default=None)
    llm_lite_endpoint: str | None = Field(default=None)

class ChatStartedResponse(G8eBaseModel):
    """Response for POST /chat."""
    success: bool
    case_id: str
    investigation_id: str
