# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Schema.to_json_schema is the single wire form of every tool declaration."""

import pytest

from app.llm.llm_dataclasses import Schema, ToolDeclaration, Type
from app.services.ai.tool_registry import TOOL_SPECS


class TestSchemaToJsonSchema:
    def test_renders_nested_object(self):
        schema = Schema(
            type=Type.OBJECT,
            description="outer",
            properties={
                "field": Schema(type=Type.STRING, description="inner desc"),
                "mode": Schema(type=Type.STRING, enum=["a", "b"]),
            },
            required=["field"],
        )

        assert schema.to_json_schema() == {
            "type": "object",
            "description": "outer",
            "properties": {
                "field": {"type": "string", "description": "inner desc"},
                "mode": {"type": "string", "enum": ["a", "b"]},
            },
            "required": ["field"],
        }

    def test_renders_array_items(self):
        schema = Schema(type=Type.ARRAY, items=Schema(type=Type.INTEGER))

        assert schema.to_json_schema() == {"type": "array", "items": {"type": "integer"}}

    @pytest.mark.parametrize("properties", [None, {}])
    def test_object_always_carries_properties(self, properties):
        """{"type": "object"} alone fails the whole request on strict renderers.

        Ollama's qwen3.5 template answers HTTP 500 "properties must be an
        object", which every scored qwen3.5 eval call hit through the no-argument
        get_command_constraints tool.
        """
        schema = Schema(type=Type.OBJECT, properties=properties)

        assert schema.to_json_schema() == {"type": "object", "properties": {}}

    def test_scalar_never_carries_properties(self):
        assert Schema(type=Type.BOOLEAN).to_json_schema() == {"type": "boolean"}

    def test_rendering_does_not_alias_the_declaration(self):
        schema = Schema(type=Type.STRING, enum=["a"])
        schema.to_json_schema()["enum"].append("b")

        assert schema.enum == ["a"]


class TestToolDeclaration:
    def test_defaults_to_a_no_argument_object(self):
        declaration = ToolDeclaration(name="t", description="d")

        assert declaration.parameters.to_json_schema() == {"type": "object", "properties": {}}

    @pytest.mark.parametrize("spec", TOOL_SPECS, ids=lambda spec: str(spec.name.value))
    def test_every_production_tool_renders_an_object_with_properties(self, spec):
        rendered = spec.builder().parameters.to_json_schema()

        assert rendered["type"] == "object"
        assert isinstance(rendered["properties"], dict)
