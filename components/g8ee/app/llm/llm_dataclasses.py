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
Simple dataclasses for LLM types.

These types have no dependencies on Pydantic models and are safe to import
from anywhere without causing circular import issues.
"""

from __future__ import annotations

import base64
from dataclasses import dataclass, field
from enum import Enum, StrEnum
from typing import Any


@dataclass(frozen=True)
class ThoughtSignature:
    """Opaque encrypted context blob from the provider thinking API.

    Canonical form inside the application is always a base64-encoded string.
    The SDK returns raw bytes at the inbound provider boundary; from_sdk()
    normalises any inbound representation to this canonical form.

    Rules for thought-signatures:
    - Must be passed back on every toolCall Part (400 if omitted).
    - Must NOT be merged: a Part with a signature cannot be combined with
      a Part that lacks one, and two signed Parts cannot be merged.
    - value is the base64 string to embed verbatim in outbound API requests.
    """

    value: str

    @classmethod
    def from_sdk(cls, raw) -> ThoughtSignature | None:
        """Normalise an inbound SDK thought_signature to ThoughtSignature.

        Accepts bytes, bytearray, or str. Returns None when raw is None or
        falsy so callers can use a simple ``if sig:`` guard.
        """
        if not raw:
            return None
        if isinstance(raw, (bytes, bytearray)):
            return cls(value=base64.b64encode(raw).decode("utf-8"))
        if isinstance(raw, str):
            return cls(value=raw)
        return cls(value=base64.b64encode(bytes(raw)).decode("utf-8"))

    def __str__(self) -> str:
        return self.value


@dataclass
class ToolCall:
    name: str
    args: dict[str, Any]
    id: str | None = None


@dataclass
class ToolResponse:
    name: str
    response: dict[str, Any]
    id: str | None = None


@dataclass
class InlineData:
    mime_type: str
    data: bytes


@dataclass
class Part:
    text: str | None = None
    tool_call: ToolCall | None = None
    tool_response: ToolResponse | None = None
    thought: bool = False
    inline_data: InlineData | None = None
    thought_signature: ThoughtSignature | None = None

    @classmethod
    def from_text(cls, text: str) -> Part:
        return cls(text=text)

    @classmethod
    def from_bytes(cls, data: bytes, mime_type: str) -> Part:
        return cls(inline_data=InlineData(mime_type=mime_type, data=data))

    @classmethod
    def from_tool_response(cls, name: str, response: dict[str, Any], id: str | None = None) -> Part:
        return cls(tool_response=ToolResponse(name=name, response=response, id=id))


@dataclass
class Content:
    role: str
    parts: list[Part] = field(default_factory=list)


@dataclass
class UsageMetadata:
    prompt_token_count: int = 0
    candidates_token_count: int = 0
    total_token_count: int = 0
    thinking_token_count: int = 0


@dataclass
class SdkGroundingWebSource:
    uri: str = ""
    title: str = ""


@dataclass
class SdkGroundingChunk:
    web: SdkGroundingWebSource | None


@dataclass
class SdkGroundingSegment:
    start_index: int = 0
    end_index: int = 0
    text: str = ""


@dataclass
class SdkGroundingSupport:
    segment: SdkGroundingSegment = field(default_factory=SdkGroundingSegment)
    grounding_chunk_indices: list[int] = field(default_factory=list)


@dataclass
class SdkSearchEntryPoint:
    rendered_content: str = ""


@dataclass
class SdkGroundingRawData:
    """Typed representation of raw SDK grounding metadata extracted at the provider boundary.

    Populated by the respective provider and attached to GenerateContentResponse.grounding_raw.
    Consumed exclusively by GroundingService — never accessed outside the grounding boundary.
    """
    web_search_queries: list[str] = field(default_factory=list)
    grounding_chunks: list[SdkGroundingChunk] = field(default_factory=list)
    grounding_supports: list[SdkGroundingSupport] = field(default_factory=list)
    search_entry_point: SdkSearchEntryPoint | None = None


@dataclass
class Candidate:
    content: Content
    finish_reason: str


@dataclass
class GenerateContentResponse:
    candidates: list[Candidate] = field(default_factory=list)
    usage_metadata: UsageMetadata = field(default_factory=UsageMetadata)
    grounding_raw: SdkGroundingRawData = field(default_factory=SdkGroundingRawData)

    @property
    def text(self) -> str | None:
        """Convenience: extract first text part from first candidate."""
        if self.candidates:
            for part in self.candidates[0].content.parts:
                if part.text and not part.thought:
                    return part.text
        return None

    @property
    def tool_calls(self) -> list[ToolCall]:
        """Convenience: extract all tool calls from first candidate."""
        calls = []
        if self.candidates:
            for part in self.candidates[0].content.parts:
                if part.tool_call:
                    calls.append(part.tool_call)
        return calls


@dataclass
class StreamChunkFromModel:
    text: str | None = None
    tool_calls: list[ToolCall] = field(default_factory=list)
    thought: bool = False
    usage_metadata: UsageMetadata = field(default_factory=UsageMetadata)
    finish_reason: str | None = None
    thought_signature: ThoughtSignature | None = None


@dataclass
class ToolCallingConfig:
    mode: str = "AUTO"
    allowed_tool_names: list[str] = field(default_factory=list)


@dataclass
class ToolConfig:
    tool_calling_config: ToolCallingConfig = field(default_factory=ToolCallingConfig)


class Role(StrEnum):
    USER = "user"
    ASSISTANT = "assistant"
    SYSTEM = "system"
    MODEL = "model"  # Provider-specific but often mapped to ASSISTANT
    TOOL = "tool"


class Type(Enum):
    STRING = "STRING"
    INTEGER = "INTEGER"
    NUMBER = "NUMBER"
    BOOLEAN = "BOOLEAN"
    ARRAY = "ARRAY"
    OBJECT = "OBJECT"


@dataclass
class Schema:
    type: Type
    description: str | None = None
    properties: dict[str, Schema] | None = None
    required: list[str] | None = None
    items: Schema | None = None
    enum: list[str] | None = None


@dataclass
class ToolDeclaration:
    """Provider-agnostic function/tool schema."""
    name: str
    description: str
    parameters: dict[str, Any] | Schema


@dataclass
class ToolGroup:
    tools: list[ToolDeclaration] = field(default_factory=list)
    google_search: bool = False


@dataclass
class ResponseJsonSchema:
    json_schema_dict: dict[str, Any]
    name: str = "response"
    strict: bool = False

    def flatten_for_ollama(self) -> dict[str, Any]:
        return self.json_schema_dict

    def flatten_for_openai(self) -> dict[str, Any]:
        return {"name": self.name, "schema": self.json_schema_dict, "strict": self.strict}


@dataclass
class ResponseFormat:
    json_schema: ResponseJsonSchema

    @classmethod
    def from_pydantic_schema(cls, json_schema: dict[str, Any], name: str = "response") -> ResponseFormat:
        return cls(json_schema=ResponseJsonSchema(json_schema_dict=json_schema, name=name))

    def flatten_for_ollama(self) -> dict[str, Any]:
        return self.json_schema.flatten_for_ollama()

    def flatten_for_openai(self) -> dict[str, Any]:
        return {"type": "json_schema", "json_schema": self.json_schema.flatten_for_openai(), "version": 1}
