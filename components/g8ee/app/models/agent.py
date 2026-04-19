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
Pydantic models for the g8e Agent streaming pipeline.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any

import app.llm.llm_types as types
from pydantic import ConfigDict, Field

from app.constants import (
    OperatorType,
    AgentMode,
)
from app.models.base import G8eBaseModel
from app.models.grounding import GroundingMetadata
from app.models.http_context import G8eHttpContext
from app.models.investigations import EnrichedInvestigationContext, ConversationHistoryMessage
from app.models.memory import InvestigationMemory
from app.models.settings import G8eeUserSettings
from app.models.agents import TriageResult
from app.models.command_payloads import TargetedOperatorArgs
from app.models.tool_results import (
    TokenUsage,
    ToolResult,
)


from app.constants import (
    StreamChunkFromModelType,
)
_TARGET_OPERATORS_DESCRIPTION = (
    "Run on MULTIPLE operators simultaneously under a SINGLE approval. "
    "STRONGLY PREFER passing ['all'] whenever the user's intent covers every bound system "
    "(e.g. 'on all systems', 'across the fleet', 'on all N hosts', or the user explicitly "
    "names a count matching the bound operator count). DO NOT enumerate individual operators "
    "for whole-fleet intent — use ['all']. Only enumerate specific hostnames/operator_ids/indices "
    "when the user is asking about a proper subset. The same Tribunal-generated command executes "
    "on all resolved systems in parallel under one approval."
)


class OperatorCommandToolSchema(TargetedOperatorArgs):
    """Sage-facing schema for run_commands_with_operator.

    Sage does NOT propose shell commands. Sage articulates what it needs to
    accomplish and any creative guidelines; the Tribunal is the sole authority
    on the exact command string. The Tribunal-produced command is then routed
    to the Operator via the internal OperatorCommandArgs payload.
    """
    request: str = Field(
        default="",
        description=(
            "Natural-language description of what Sage needs the Operator to accomplish "
            "to maximise confidence in identifying the solution. Focus on investigative "
            "intent — what you want to learn, verify, or change — NOT shell syntax. "
            "The Tribunal will translate this into a precise command for the target OS/shell."
        ),
    )
    guidelines: str = Field(
        default="",
        description=(
            "Optional creative guidance for the Tribunal: flags to favour, behaviours to avoid, "
            "edge cases to cover, output formats preferred, or tradeoffs Sage considers important. "
            "Leave empty when no guidance is needed. Never include shell syntax here — express "
            "preferences in plain language so the Tribunal can choose the correct realisation."
        ),
    )
    target_operators: list[str] | None = Field(default=None, description=_TARGET_OPERATORS_DESCRIPTION)
    expected_output_lines: int = Field(default=10, description="Approximate number of stdout lines expected (used for UI sizing).")
    timeout_seconds: int = Field(default=300, description="Maximum seconds to wait for command completion before timing out.")


class OperatorCommandArgs(TargetedOperatorArgs):
    """Internal executor payload for run_commands_with_operator.

    `command` is populated by the Tribunal after it processes Sage's `request`
    and `guidelines`. Sage never writes to `command` directly; see
    `OperatorCommandToolSchema` for the Sage-facing surface.
    """
    command: str = Field(default="", description="Shell command produced by the Tribunal (never written by Sage).")
    request: str = Field(default="", description="Sage's natural-language request passed to the Tribunal (shown to the user as justification).")
    guidelines: str = Field(default="", description="Sage's optional creative guidelines passed to the Tribunal.")
    target_operators: list[str] | None = Field(default=None, description=_TARGET_OPERATORS_DESCRIPTION)
    expected_output_lines: int = Field(default=10, description="Approximate number of stdout lines expected (used for UI sizing).")
    timeout_seconds: int = Field(default=300, description="Maximum seconds to wait for command completion before timing out.")
    execution_id: str | None = Field(default=None, alias="execution_id")
    web_session_id: str | None = Field(default=None, alias="_web_session_id")

class OperatorContext(G8eBaseModel):
    """Typed system context extracted from a single OperatorDocument model."""
    operator_id: str
    operator_session_id: str | None = None
    os: str | None = None
    hostname: str | None = None
    architecture: str | None = None
    cpu_count: int | None = None
    memory_mb: int | None = None
    public_ip: str | None = None
    operator_type: OperatorType | None = None
    cloud_subtype: str | None = None
    is_cloud_operator: bool = False
    granted_intents: list[str] | None = None
    distro: str | None = None
    kernel: str | None = None
    os_version: str | None = None
    username: str | None = None
    uid: int | None = None
    home_directory: str | None = None
    shell: str | None = None
    working_directory: str | None = None
    timezone: str | None = None
    is_container: bool = False
    container_runtime: str | None = None
    init_system: str | None = None
    disk_percent: float | None = None
    disk_total_gb: float | None = None
    disk_free_gb: float | None = None
    memory_percent: float | None = None
    memory_total_mb: float | None = None
    memory_available_mb: float | None = None


class AgentStreamContext(G8eBaseModel):
    """Typed context passed through g8eEngine stream methods. """
    model_config = ConfigDict(arbitrary_types_allowed=True)

    # Core context fields
    case_id: str | None = None
    investigation_id: str | None = None
    investigation: EnrichedInvestigationContext
    user_id: str | None = None
    g8e_context: G8eHttpContext
    web_session_id: str | None = None
    task_id: str | None = None
    agent_mode: AgentMode
    request_settings: G8eeUserSettings

    # Chat pipeline fields
    operator_bound: bool = False
    model_to_use: str | None = None
    max_tokens: int | None = None
    conversation_history: list[ConversationHistoryMessage] = Field(default_factory=list)
    system_instructions: str = ""
    contents: list[types.Content] = Field(default_factory=list)
    generation_config: types.PrimaryLLMSettings | None = None
    user_memories: list[InvestigationMemory] = Field(default_factory=list)
    case_memories: list[InvestigationMemory] = Field(default_factory=list)
    triage_result: TriageResult | None = None
    sentinel_mode: bool = True
    response_text: str = ""
    token_usage: TokenUsage | None = None
    finish_reason: str | None = None
    grounding_metadata: GroundingMetadata | None = None

    def set_thinking_started(self) -> None:
        """Mark thinking as started."""
        pass

    def set_thinking_ended(self) -> None:
        """Mark thinking as ended."""
        pass


class StreamChunkData(G8eBaseModel):
    """Typed data for StreamChunkFromModel, replacing Dict[str, Any].

    Covers all chunk types: TEXT, THINKING, TOOL_CALL, TOOL_RESULT,
    CITATIONS, COMPLETE, ERROR, RETRY.
    """
    content: str | None = None
    thinking: str | None = None
    action_type: str | None = None
    tool_name: str | None = None
    execution_id: str | None = None
    command: str | None = None
    is_operator_tool: bool | None = None
    display_label: str | None = None
    display_icon: str | None = None
    display_detail: str | None = None
    category: str | None = None
    status: str | None = None
    grounding_metadata: GroundingMetadata | None = None
    finish_reason: str | None = None
    has_citations: bool | None = None
    response_length: int | None = None
    grounding_used: bool | None = None
    token_usage: TokenUsage | None = None
    error: str | None = None
    error_type: str | None = None
    success: bool | None = None
    result: ToolResult | None = None
    attempt: int | None = None
    max_attempts: int | None = None
    investigation_id: str | None = None
    case_id: str | None = None


class StreamChunkFromModel(G8eBaseModel):
    """Typed chunk for streaming responses."""
    type: StreamChunkFromModelType
    data: StreamChunkData


@dataclass
class TurnResult:
    """Result produced by _process_provider_turn for a single LLM stream turn."""
    model_response_parts: list[types.Part]
    pending_tool_calls: list[types.ToolCall]
    finish_reason: str | None
    input_tokens: int
    output_tokens: int
    total_tokens: int


@dataclass
class ToolCallResponse:
    """Record of a single executed tool call within a turn."""
    tool_name: str
    flattened_response: dict[str, Any]
    grounding: GroundingMetadata | None
    tool_call_id: str | None = None

