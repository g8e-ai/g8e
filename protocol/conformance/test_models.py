"""Cross-language protocol model conformance tests.

Validates that Python Pydantic models in the g8e package produce JSON that is
structurally compatible with the canonical model schemas in protocol/models/.

The JSON files in protocol/models/ define field names, types, and required
flags. The Python Pydantic models must produce JSON with matching field names
and compatible types when serialized.
"""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

import pytest

MODELS_DIR = Path(__file__).parent.parent / "models"


def _load_model_schema(filename: str) -> dict[str, Any]:
    path = MODELS_DIR / filename
    assert path.exists(), f"Model schema not found: {path}"
    with open(path, encoding="utf-8") as f:
        return json.load(f)


def _extract_field_names(schema: dict[str, Any], section: str = "settings") -> set[str]:
    """Extract field names from a model schema section."""
    fields = set()
    if section in schema:
        section_data = schema[section]
        if isinstance(section_data, dict):
            for key, val in section_data.items():
                if isinstance(val, dict) and "type" in val:
                    fields.add(key)
                elif isinstance(val, dict):
                    fields.update(_extract_field_names(val))
    return fields


class TestModelSchemaIntegrity:
    """Verify all model JSON schemas are valid and have expected structure."""

    MODEL_FILES = [
        "platform_settings.json",
        "user.json",
        "organization.json",
        "operator_session.json",
        "bound_session.json",
        "cli_session.json",
        "web_session.json",
        "case.json",
        "task.json",
        "investigation.json",
    ]

    @pytest.mark.parametrize("filename", MODEL_FILES)
    def test_model_file_loads_successfully(self, filename: str):
        data = _load_model_schema(filename)
        assert isinstance(data, dict), f"{filename} must be a JSON object"

    @pytest.mark.parametrize("filename", MODEL_FILES)
    def test_model_file_has_metadata(self, filename: str):
        data = _load_model_schema(filename)
        # All model files should have at least a top-level model definition
        assert len(data) > 0, f"{filename} is empty"


class TestPlatformSettingsConformance:
    """Verify Python PlatformSettings model matches the JSON schema."""

    def test_platform_settings_fields_match_schema(self):
        from g8e.models import PlatformSettings

        schema = _load_model_schema("platform_settings.json")
        schema_fields = _extract_field_names(schema, "platform_settings")

        # Get Python model field names (using Pydantic's model_fields)
        py_fields = set(PlatformSettings.model_fields.keys())

        # Every field in the schema's "settings" section should have a
        # corresponding field in the Python model, or be a known metadata key
        known_metadata = {"created_at", "updated_at"}
        schema_only = schema_fields - py_fields - known_metadata
        assert not schema_only, (
            f"Schema fields not in Python model: {schema_only}"
        )


class TestRequestContextConformance:
    """Verify RequestContext model produces valid JSON round-trips."""

    def test_request_context_serialization_round_trip(self):
        from g8e.models import RequestContext

        ctx = RequestContext(
            source_component="client",
            web_session_id="test-session-123",
            user_id="test-user-456",
        )
        json_str = ctx.model_dump_json()
        parsed = json.loads(json_str)

        assert parsed["source_component"] == "client"
        assert parsed["web_session_id"] == "test-session-123"
        assert parsed["user_id"] == "test-user-456"

    def test_request_context_client_requires_session_and_user(self):
        from g8e.models import RequestContext
        from pydantic import ValidationError

        with pytest.raises(ValidationError):
            RequestContext(source_component="client")

    def test_request_context_non_client_does_not_require_session(self):
        from g8e.models import RequestContext

        ctx = RequestContext(source_component="g8eo")
        assert ctx.source_component == "g8eo"


class TestBoundOperatorConformance:
    """Verify BoundOperator model round-trips correctly."""

    def test_bound_operator_serialization(self):
        from g8e.models import BoundOperator

        op = BoundOperator(
            operator_id="op-123",
            operator_session_id="sess-456",
        )
        json_str = op.model_dump_json()
        parsed = json.loads(json_str)

        assert parsed["operator_id"] == "op-123"
        assert parsed["operator_session_id"] == "sess-456"


class TestSerializationParity:
    """Verify that G8eBaseModel serialization follows canonical conventions."""

    def test_exclude_none_on_serialization(self):
        from g8e.models import RequestContext

        ctx = RequestContext(source_component="g8eo")
        dumped = ctx.model_dump()
        assert "web_session_id" not in dumped
        assert "user_id" not in dumped

    def test_extra_fields_ignored(self):
        from g8e.models import RequestContext

        ctx = RequestContext(source_component="g8eo", unknown_field="value")
        dumped = ctx.model_dump()
        assert "unknown_field" not in dumped
