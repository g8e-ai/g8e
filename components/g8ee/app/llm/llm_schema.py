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
Pydantic-to-LLM Schema Conversion Utilities.

This module provides utilities for deriving LLM tool schemas and response
formats from Pydantic models. It depends on G8eBaseModel and the canonical
LLM types.
"""

from typing import Any

from app.llm.llm_dataclasses import Type, Schema
from app.models.base import G8eBaseModel


_JSON_TYPE_MAP: dict[str, Type] = {
    "string": Type.STRING,
    "integer": Type.INTEGER,
    "number": Type.NUMBER,
    "boolean": Type.BOOLEAN,
    "array": Type.ARRAY,
    "object": Type.OBJECT,
}


def _resolve_ref(ref: str, defs: dict) -> dict:
    name = ref.rsplit("/", maxsplit=1)[-1]
    return defs.get(name, {})


def _json_schema_to_schema(node: dict, defs: dict) -> Schema:
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

    properties: dict[str, Schema] | None = None
    required: list[str] | None = None
    items: Schema | None = None

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


def schema_from_model(model_cls: type, required_override: list[str] | None = None) -> Schema:
    """Derive a types.Schema from a G8eBaseModel subclass.

    Uses model_json_schema() as the source of truth. Field descriptions come
    from Field(description=...) on the model — no inline redeclaration needed.

    Args:
        model_cls: A G8eBaseModel subclass.
        required_override: If provided, overrides the required field list. Use
            when the model has required fields that should be optional for the LLM,
            or vice versa.
    """
    if not issubclass(model_cls, G8eBaseModel):
        raise TypeError(f"model_cls must be a G8eBaseModel subclass, got {model_cls}")

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
