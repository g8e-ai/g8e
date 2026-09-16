# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from typing import Any, Literal
from .base import G8eBaseModel, Field, model_validator
from .context import RequestContext

EvaluationLane = Literal["model_role", "system"]
DesignatedModelRole = Literal["primary", "assistant", "lite"]


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


class InferenceModelVariant(G8eBaseModel):
    model: str = Field(..., min_length=1)
    digest: str = Field(..., pattern=r"^[0-9a-f]{64}$")


class EvaluationInferenceContext(G8eBaseModel):
    campaign_id: str = Field(..., min_length=1)
    run_id: str = Field(..., min_length=1)
    assignment_id: str = Field(..., min_length=1)
    evaluation_attempt_id: str = Field(..., min_length=1)
    scenario_id: str = Field(..., min_length=1)
    model_registry_digest: str = Field(..., pattern=r"^[0-9a-f]{64}$")
    model_registry: list[InferenceModelVariant] = Field(..., min_length=1)
    target_operator_session_id: str = Field(..., min_length=1)
    evaluation_lane: EvaluationLane = Field(default="system")
    designated_model_role: DesignatedModelRole | None = Field(default=None)

    @model_validator(mode="after")
    def validate_unique_models(self):
        models = [variant.model for variant in self.model_registry]
        if len(models) != len(set(models)):
            raise ValueError("evaluation model registry contains duplicate model tags")
        return self

    @model_validator(mode="after")
    def validate_lane_binding(self):
        if self.evaluation_lane == "model_role" and self.designated_model_role is None:
            raise ValueError(
                "designated_model_role is required when evaluation_lane is model_role"
            )
        if self.evaluation_lane == "system" and self.designated_model_role is not None:
            raise ValueError(
                "designated_model_role is only permitted for model_role evaluation lane"
            )
        return self


class ChatMessageRequest(LLMOverrides):
    """Request model for chat messages."""

    context: RequestContext = Field(...)
    evaluation_context: EvaluationInferenceContext | None = Field(default=None)
    message: str = Field(...)
    attachments: list[dict[str, Any]] | None = Field(default_factory=list)
    sentinel_mode: bool = Field(default=True)
    resource_creation: ResourceCreationRequest | None = Field(default=None)


class ChatStartedResponse(G8eBaseModel):
    """Response for POST /chat."""

    success: bool
    case_id: str
    investigation_id: str


class EvaluationTraceResponse(G8eBaseModel):
    """Read-only evaluation assignment trace lookup."""

    trace: dict[str, Any]
