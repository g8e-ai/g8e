# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from typing import Any
from .base import G8eBaseModel, Field
from .context import RequestContext

class ResourceCreationRequest(G8eBaseModel):
    """Typed request to create new case and investigation resources."""
    create_case: bool = Field(default=False)
    case_title: str | None = Field(default=None)

class LLMOverrides(G8eBaseModel):
    """Reusable LLM configuration override fields."""
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


class ChatMessageRequest(LLMOverrides):
    """Request model for chat messages."""
    context: RequestContext = Field(...)
    message: str = Field(...)
    attachments: list[dict[str, Any]] | None = Field(default_factory=list)
    sentinel_mode: bool = Field(default=True)
    resource_creation: ResourceCreationRequest | None = Field(default=None)

class ChatStartedResponse(G8eBaseModel):
    """Response for POST /chat."""
    success: bool
    case_id: str
    investigation_id: str
