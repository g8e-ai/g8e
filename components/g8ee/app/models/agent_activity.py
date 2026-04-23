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
Metadata models for AI agent activity tracking and data science analysis.
"""

from pydantic import Field

from app.constants import AgentMode, TriageComplexityClassification, TriageConfidence
from app.models.base import G8eIdentifiableModel, UTCDatetime
from app.models.tool_results import TokenUsage


class AgentActivityMetadata(G8eIdentifiableModel):
    """Metadata record for a single AI agent chat turn.
    
    Captures comprehensive telemetry for data science analysis including
    model usage, token consumption, tool execution patterns, triage
    classification, and performance metrics.
    """
    
    user_id: str | None = Field(default=None, description="User who initiated the request")
    user_email: str | None = Field(default=None, description="Email of the user")
    
    investigation_id: str | None = Field(default=None, description="Investigation ID")
    case_id: str | None = Field(default=None, description="Case ID")
    web_session_id: str | None = Field(default=None, description="Web session ID")
    
    agent_mode: AgentMode | None = Field(default=None, description="Agent execution mode")
    
    model_name: str | None = Field(default=None, description="LLM model used")
    provider: str | None = Field(default=None, description="LLM provider (gemini, anthropic, etc.)")
    
    token_usage: TokenUsage | None = Field(default=None, description="Token consumption metrics")
    
    finish_reason: str | None = Field(default=None, description="Why the agent stopped (stop, length, error, etc.)")
    
    triage_complexity: TriageComplexityClassification | None = Field(
        default=None, description="Triage complexity classification"
    )
    triage_complexity_confidence: TriageConfidence | None = Field(
        default=None, description="Confidence in complexity classification"
    )
    triage_intent_summary: str | None = Field(default=None, description="Triage intent classification")
    triage_intent_confidence: TriageConfidence | None = Field(
        default=None, description="Confidence in intent classification"
    )
    triage_request_posture: str | None = Field(default=None, description="Triage posture classification")
    triage_posture_confidence: TriageConfidence | None = Field(
        default=None, description="Confidence in posture classification"
    )
    
    tool_call_count: int = Field(default=0, description="Number of tool calls executed")
    tool_types_used: list[str] = Field(default_factory=list, description="Types of tools used")
    
    duration_seconds: float | None = Field(default=None, description="Total request duration in seconds")
    
    has_attachments: bool = Field(default=False, description="Whether attachments were present")
    attachment_count: int = Field(default=0, description="Number of attachments")
    
    grounding_used: bool = Field(default=False, description="Whether grounding/citations were used")
    citation_count: int = Field(default=0, description="Number of citations returned")
    
    operator_bound: bool = Field(default=False, description="Whether operators were bound")
    bound_operator_count: int = Field(default=0, description="Number of bound operators")
    
    response_length: int = Field(default=0, description="Length of AI response in characters")
    
    error: str | None = Field(default=None, description="Error message if execution failed")
    error_type: str | None = Field(default=None, description="Type of error if execution failed")
