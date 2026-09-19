# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from app.llm.llm_schema import inline_json_schema_refs
from app.models.agents.triage import TriageResult


def test_inline_json_schema_refs_expands_pydantic_defs():
    schema = TriageResult.model_json_schema()
    inlined = inline_json_schema_refs(schema)

    assert "$defs" not in inlined
    assert "$ref" not in str(inlined)
    assert inlined["properties"]["complexity"]["type"] == "string"
    assert "complex" in inlined["properties"]["complexity"]["enum"]
