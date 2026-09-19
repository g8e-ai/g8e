# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
Pydantic-to-LLM Schema Conversion Utilities.

This module provides utilities for deriving LLM tool schemas and response
formats from Pydantic models. It depends on G8eBaseModel and the canonical
LLM types.
"""

from __future__ import annotations

from copy import deepcopy
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


def inline_json_schema_refs(schema: dict[str, Any]) -> dict[str, Any]:
    """Return a self-contained JSON Schema with ``$defs`` refs expanded in place.

    Governed inference validates response schemas strictly and rejects unresolved
    ``#/$defs/...`` references produced by Pydantic model schemas.
    """
    root = deepcopy(schema)
    defs = dict(root.get("$defs") or {})

    def inline_node(node: Any, active_refs: frozenset[str]) -> Any:
        if not isinstance(node, dict):
            return node
        ref = node.get("$ref")
        if isinstance(ref, str) and ref.startswith("#/$defs/"):
            def_name = ref[len("#/$defs/") :]
            if def_name in active_refs or def_name not in defs:
                return {key: value for key, value in node.items() if key != "$ref"}
            target = deepcopy(defs[def_name])
            inlined = inline_node(target, active_refs | {def_name})
            extras = {key: value for key, value in node.items() if key != "$ref"}
            if isinstance(inlined, dict):
                merged = dict(inlined)
                merged.update(extras)
                return merged
            return extras or inlined
        return {
            key: (
                [inline_node(item, active_refs) for item in value]
                if isinstance(value, list)
                else inline_node(value, active_refs)
            )
            for key, value in node.items()
            if key != "$defs"
        }

    return inline_node(root, frozenset())


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
        properties = {k: _json_schema_to_schema(v, defs) for k, v in node["properties"].items()}
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
    from Field(description=...) on the model - no inline redeclaration needed.

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

    properties = {k: _json_schema_to_schema(v, defs) for k, v in properties_raw.items()}
    required = required_override if required_override is not None else model_required

    return Schema(
        type=Type.OBJECT,
        properties=properties,
        required=required if required else None,
    )
