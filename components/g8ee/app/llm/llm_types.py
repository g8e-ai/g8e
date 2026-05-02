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
Canonical LLM Types

Provider-agnostic type system for all LLM interactions. Every service in g8ee
uses these types instead of any provider-specific SDK types.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum
from typing import Any

from app.constants import (
    LLM_DEFAULT_MAX_OUTPUT_TOKENS,
    ThinkingLevel,
)

# Import pure dataclasses/enums from separate module
from app.llm.llm_dataclasses import (
    Candidate,
    Content,
    GenerateContentResponse,
    InlineData,
    Part,
    ResponseFormat,
    ResponseJsonSchema,
    Role,
    Schema,
    SdkGroundingChunk,
    SdkGroundingRawData,
    SdkGroundingSegment,
    SdkGroundingSupport,
    SdkGroundingWebSource,
    SdkSearchEntryPoint,
    StreamChunkFromModel,
    ThoughtSignature,
    ToolCall,
    ToolCallingConfig,
    ToolConfig,
    ToolDeclaration,
    ToolGroup,
    ToolResponse,
    Type,
    UsageMetadata,
)

# Import schema generation from dedicated module (lazy wrapper to break circular import)
def schema_from_model(model_cls: type, required_override: list[str] | None = None) -> Schema:
    """Derive a Schema from a G8eBaseModel subclass.

    This is a lazy wrapper around app.llm.llm_schema.schema_from_model to break
    circular imports between llm_types -> llm_schema -> models -> agent -> llm_types.
    """
    from app.llm.llm_schema import schema_from_model as _schema_from_model
    return _schema_from_model(model_cls, required_override)

# Re-export for backwards compatibility
__all__ = [
    "ThoughtSignature",
    "ToolCall",
    "ToolResponse",
    "InlineData",
    "Part",
    "Content",
    "Candidate",
    "GenerateContentResponse",
    "UsageMetadata",
    "SdkGroundingWebSource",
    "SdkGroundingChunk",
    "SdkGroundingSegment",
    "SdkGroundingSupport",
    "SdkSearchEntryPoint",
    "SdkGroundingRawData",
    "StreamChunkFromModel",
    "ToolCallingConfig",
    "ToolConfig",
    "ThinkingConfig",
    "Role",
    "Type",
    "Schema",
    "schema_from_model",
    "ToolDeclaration",
    "ToolGroup",
    "ResponseJsonSchema",
    "ResponseFormat",
    "PrimaryLLMSettings",
    "AssistantLLMSettings",
    "LiteLLMSettings",
    "GenerateContentConfig",
]


@dataclass
class ThinkingConfig:
    """Canonical thinking/reasoning effort request.

    thinking_level is always a ThinkingLevel (never None). ThinkingLevel.OFF
    means "do not enable thinking for this call" — providers translate OFF
    to the appropriate per-provider omission (no thinking_config key for
    Gemini, no thinking dict for Anthropic, think=False for Ollama, no
    reasoning key for OpenAI).
    """
    thinking_level: ThinkingLevel = ThinkingLevel.OFF
    include_thoughts: bool = False

    @property
    def enabled(self) -> bool:
        return self.thinking_level is not ThinkingLevel.OFF


# Module-level defaults reused by every *LLMSettings dataclass. Declaring
# them once avoids drift between sibling structs.
_DEFAULT_RESPONSE_MODALITIES: tuple[str, ...] = ("TEXT",)


@dataclass
class PrimaryLLMSettings:
    """Primary-agent generation settings.

    Only ``system_instructions`` (the prompt text) and ``tools`` (the function
    surface) are typically case-specific; everything else has a neutral
    default so callers in tests, benchmarks, and probes can construct a valid
    object without restating the same boilerplate at every site. The defaults
    map to the same values the production ``AIGenerationConfigBuilder``
    applies when platform settings omit overrides.
    """
    max_output_tokens: int = LLM_DEFAULT_MAX_OUTPUT_TOKENS
    top_p_nucleus_sampling: float | None = None
    top_k_filtering: int | None = None
    stop_sequences: list[str] | None = None
    response_modalities: list[str] = field(default_factory=lambda: list(_DEFAULT_RESPONSE_MODALITIES))
    tools: list[ToolGroup] = field(default_factory=list)
    system_instructions: str = ""
    thinking_config: ThinkingConfig = field(default_factory=ThinkingConfig)
    tool_config: ToolConfig = field(default_factory=ToolConfig)


@dataclass
class AssistantLLMSettings:
    """Assistant-tier generation settings (lighter than primary, no tools).

    Used for triage, intent classification, and other structured-output
    helpers. See PrimaryLLMSettings for the rationale behind per-field
    defaults.
    """
    max_output_tokens: int = LLM_DEFAULT_MAX_OUTPUT_TOKENS
    top_p_nucleus_sampling: float | None = None
    top_k_filtering: int | None = None
    stop_sequences: list[str] | None = None
    system_instructions: str = ""
    response_format: ResponseFormat | None = None


@dataclass
class LiteLLMSettings:
    """Lite-tier generation settings (smallest model, no tools, no thinking).

    Used for title generation, memory summarisation, and similar one-shot
    helpers. See PrimaryLLMSettings for the rationale behind per-field
    defaults.
    """
    max_output_tokens: int = LLM_DEFAULT_MAX_OUTPUT_TOKENS
    top_p_nucleus_sampling: float | None = None
    top_k_filtering: int | None = None
    stop_sequences: list[str] | None = None
    system_instructions: str = ""
    response_format: ResponseFormat | None = None


@dataclass
class GenerateContentConfig:
    max_output_tokens: int
    system_instructions: str
    top_p_nucleus_sampling: float | None = None
    top_k_filtering: int | None = None
    stop_sequences: list[str] | None = None
    response_modalities: list[str] = field(default_factory=lambda: ["TEXT"])
    tools: list[ToolGroup] = field(default_factory=list)
    thinking_config: ThinkingConfig = field(default_factory=ThinkingConfig)
    tool_config: ToolConfig = field(default_factory=lambda: ToolConfig(tool_calling_config=ToolCallingConfig(mode="AUTO")))
    response_format: ResponseFormat | None = None

