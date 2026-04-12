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

import base64
from dataclasses import dataclass, field
from enum import Enum
from typing import Any, Optional

from app.constants import ThinkingLevel


@dataclass(frozen=True)
class ThoughtSignature:
    """Opaque encrypted context blob from the Gemini thinking API.

    Canonical form inside the application is always a base64-encoded string.
    The SDK returns raw bytes at the inbound provider boundary; from_sdk()
    normalises any inbound representation to this canonical form.

    Rules per Gemini 3 thought-signatures spec:
    - Must be passed back on every toolCall Part (400 if omitted).
    - Must NOT be merged: a Part with a signature cannot be combined with
      a Part that lacks one, and two signed Parts cannot be merged.
    - value is the base64 string to embed verbatim in outbound API requests.
    """

    value: str

    @classmethod
    def from_sdk(cls, raw) -> "ThoughtSignature | None":
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
    def from_text(cls, text: str) -> "Part":
        return cls(text=text)

    @classmethod
    def from_bytes(cls, data: bytes, mime_type: str) -> "Part":
        return cls(inline_data=InlineData(mime_type=mime_type, data=data))

    @classmethod
    def from_tool_response(cls, name: str, response: dict[str, Any], id: str | None = None) -> "Part":
        return cls(tool_response=ToolResponse(name=name, response=response, id=id))


@dataclass
class Content:
    role: str
    parts: list[Part] = field(default_factory=list)


class Role(str, Enum):
    USER = "user"
    ASSISTANT = "assistant"
    SYSTEM = "system"
    MODEL = "model"  # Gemini specific but mapped to ASSISTANT usually
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
    properties: dict[str, "Schema"] | None = None
    required: list[str] | None = None
    items: Optional["Schema"] = None
    enum: list[str] | None = None


_JSON_TYPE_MAP: dict[str, "Type"] = {
    "string": Type.STRING,
    "integer": Type.INTEGER,
    "number": Type.NUMBER,
    "boolean": Type.BOOLEAN,
    "array": Type.ARRAY,
    "object": Type.OBJECT,
}


def _resolve_ref(ref: str, defs: dict) -> dict:
    name = ref.split("/")[-1]
    return defs.get(name, {})


def _json_schema_to_schema(node: dict, defs: dict) -> "Schema":
    if "$ref" in node:
        node = _resolve_ref(node["$ref"], defs)

    if "anyOf" in node:
        non_null = [n for n in node["anyOf"] if n.get("type") != "null" or "$ref" in n]
        if not non_null:
            non_null = node["anyOf"]
        if len(non_null) == 1:
            resolved = dict(non_null[0])
            if "description" not in resolved and "description" in node:
                resolved["description"] = node["description"]
            return _json_schema_to_schema(resolved, defs)
        for candidate in non_null:
            if "$ref" in candidate:
                resolved = dict(_resolve_ref(candidate["$ref"], defs))
                if "description" not in resolved and "description" in node:
                    resolved["description"] = node["description"]
                return _json_schema_to_schema(resolved, defs)
        resolved = dict(non_null[0])
        if "description" not in resolved and "description" in node:
            resolved["description"] = node["description"]
        return _json_schema_to_schema(resolved, defs)

    raw_type = node.get("type", "string")
    schema_type = _JSON_TYPE_MAP.get(raw_type, Type.STRING)
    description = node.get("description")
    enum = node.get("enum")

    properties: dict[str, "Schema"] | None = None
    required: list[str] | None = None
    items: "Schema | None" = None

    if schema_type == Type.OBJECT and "properties" in node:
        properties = {
            k: _json_schema_to_schema(v, defs)
            for k, v in node["properties"].items()
        }
        req = node.get("required")
        if req:
            required = req

    if schema_type == Type.ARRAY and "items" in node:
        items = _json_schema_to_schema(node["items"], defs)

    return Schema(
        type=schema_type,
        description=description,
        properties=properties,
        required=required,
        items=items,
        enum=enum,
    )


def schema_from_model(model_cls: type, required_override: list[str] | None = None) -> "Schema":
    """Derive a types.Schema from a G8eBaseModel subclass.

    Uses model_json_schema() as the source of truth. Field descriptions come
    from Field(description=...) on the model — no inline redeclaration needed.

    Args:
        model_cls: A G8eBaseModel subclass.
        required_override: If provided, overrides the required field list. Use
            when the model has required fields that should be optional for the LLM,
            or vice versa.
    """
    json_schema = model_cls.model_json_schema()
    defs = json_schema.get("$defs", {})
    properties_raw = json_schema.get("properties", {})
    model_required = json_schema.get("required", [])

    properties = {
        k: _json_schema_to_schema(v, defs)
        for k, v in properties_raw.items()
    }
    required = required_override if required_override is not None else model_required

    return Schema(
        type=Type.OBJECT,
        properties=properties,
        required=required if required else None,
    )


@dataclass
class ToolDeclaration:
    name: str
    description: str
    parameters: Any


@dataclass
class ToolGroup:
    tools: list[ToolDeclaration] = field(default_factory=list)
    google_search: bool = False


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

    Populated by GeminiProvider and attached to GenerateContentResponse.grounding_raw.
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
class ResponseJsonSchema:
    schema: dict
    name: str = "response"
    strict: bool = False


@dataclass
class ResponseFormat:
    json_schema: ResponseJsonSchema

    @classmethod
    def from_pydantic_schema(cls, json_schema: dict, name: str = "response") -> "ResponseFormat":
        return cls(json_schema=ResponseJsonSchema(schema=json_schema, name=name))


@dataclass
class ToolCallingConfig:
    mode: str = "AUTO"
    allowed_tool_names: list[str] = field(default_factory=list)


@dataclass
class ToolConfig:
    tool_calling_config: ToolCallingConfig


@dataclass
class ThinkingConfig:
    thinking_level: ThinkingLevel | None = None
    include_thoughts: bool = False


@dataclass
class PrimaryLLMSettings:
    temperature: float
    max_output_tokens: int
    top_p_nucleus_sampling: float = 1.0
    top_k_filtering: int = 40
    stop_sequences: list[str] = field(default_factory=list)
    response_modalities: list[str] = field(default_factory=list)
    tools: list[ToolGroup] = field(default_factory=list)
    system_instruction: str = ""
    thinking_config: ThinkingConfig = field(default_factory=ThinkingConfig)
    tool_config: ToolConfig = None


@dataclass
class AssistantLLMSettings:
    temperature: float
    max_output_tokens: int
    top_p_nucleus_sampling: float = 1.0
    top_k_filtering: int = 40
    stop_sequences: list[str] = field(default_factory=list)
    system_instruction: str = ""
    response_format: ResponseFormat | None = None


@dataclass
class LiteLLMSettings:
    temperature: float
    max_output_tokens: int
    top_p_nucleus_sampling: float = 1.0
    top_k_filtering: int = 40
    stop_sequences: list[str] = field(default_factory=list)
    system_instruction: str = ""
    response_format: ResponseFormat | None = None


@dataclass
class GenerateContentConfig:
    temperature: float
    max_output_tokens: int
    top_p_nucleus_sampling: float = 1.0
    top_k_filtering: int = 40
    stop_sequences: list[str] = field(default_factory=list)
    response_modalities: list[str] = field(default_factory=list)
    tools: list[ToolGroup] = field(default_factory=list)
    system_instruction: str = ""
    thinking_config: ThinkingConfig = field(default_factory=ThinkingConfig)
    tool_config: ToolConfig | None = None
    response_format: ResponseFormat | None = None

