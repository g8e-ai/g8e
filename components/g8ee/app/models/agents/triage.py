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
from app.models.base import G8eBaseModel
from app.constants import (
    TriageComplexityClassification,
    TriageConfidence,
    TriageIntentClassification,
    TriageRequestPosture,
)
from app.constants.prompts import AgentMode
from app.models.attachments import AttachmentMetadata
from app.models.investigations import ConversationHistoryMessage
from app.models.settings import G8eeUserSettings


class TriageRequest(G8eBaseModel):
    """Request model for triage operation."""
    message: str = Field(description="The user message to triage.")
    agent_mode: AgentMode | None = Field(default=None, description="The current agent mode.")
    conversation_history: list[ConversationHistoryMessage] = Field(
        default_factory=list, description="The recent conversation history."
    )
    attachments: list[AttachmentMetadata] = Field(
        default_factory=list, description="Metadata for any attached files."
    )
    settings: G8eeUserSettings = Field(description="The user's LLM and platform settings.")
    model_override: str | None = Field(
        default=None, description="Optional model override for the triage operation."
    )


class TriageResult(G8eBaseModel):
    """The outcome of a triage operation."""
    complexity: TriageComplexityClassification = Field(description="How complex the task is.")
    complexity_confidence: TriageConfidence = Field(
        description="Confidence in the complexity classification."
    )
    intent: TriageIntentClassification = Field(description="The category of user intent.")
    intent_confidence: TriageConfidence = Field(
        description="Confidence in the intent classification."
    )
    intent_summary: str = Field(description="A concise summary of the user's true intent / end goal.")
    follow_up_question: str | None = Field(
        default=None, description="A follow-up question if intent confidence is LOW."
    )
    request_posture: TriageRequestPosture = Field(
        default=TriageRequestPosture.NORMAL,
        description=(
            "Triage's read of the user's state for this turn. Downstream agents "
            "calibrate dissent and denial-memory behavior on this value. "
            "Defaults to `normal` when the model omits the field or when triage falls back."
        ),
    )
    posture_confidence: TriageConfidence = Field(
        default=TriageConfidence.LOW,
        description="Confidence in the request_posture classification.",
    )
    error_code: str | None = Field(
        default=None,
        description="Structured error code when triage fails (e.g., 'PROVIDER_ERROR', 'MODEL_UNAVAILABLE').",
    )
    error_class: str | None = Field(
        default=None,
        description="Exception class name when triage fails (e.g., 'HTTPError', 'ConnectionError').",
    )
    error_message: str | None = Field(
        default=None,
        description="Detailed error message for internal logging/tracking only, never exposed to AI in prompt context.",
    )
