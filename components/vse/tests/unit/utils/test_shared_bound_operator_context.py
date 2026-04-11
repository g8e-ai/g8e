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
Contract test: VSE BoundOperator model fields must exactly match the canonical
wire shape defined in shared/models/wire/bound_operator_context.json.

Prevents desynchronization between VSOD's BoundOperatorContext.forWire() output
and VSE's BoundOperator parsing logic.
"""

import json

import pytest

from app.models.http_context import BoundOperator

pytestmark = pytest.mark.unit


def _load_bound_operator_context_json() -> dict:
    with open("/app/shared/models/wire/bound_operator_context.json") as f:
        return json.load(f)


class TestBoundOperatorFieldsMatchSharedJSON:
    """BoundOperator model fields must match shared/models/wire/bound_operator_context.json."""

    def test_operator_id_field_exists_and_required(self):
        """operator_id field must exist and be required in both definitions."""
        wire = _load_bound_operator_context_json()["bound_operator_context"]["fields"]
        assert "operator_id" in wire
        assert wire["operator_id"]["required"] is True
        
        # Verify VSE model has the field
        assert "operator_id" in BoundOperator.model_fields
        assert BoundOperator.model_fields["operator_id"].is_required()

    def test_operator_session_id_field_exists_and_optional(self):
        """operator_session_id field must exist and be optional in both definitions."""
        wire = _load_bound_operator_context_json()["bound_operator_context"]["fields"]
        assert "operator_session_id" in wire
        assert wire["operator_session_id"]["required"] is False
        
        # Verify VSE model has the field and it's optional
        assert "operator_session_id" in BoundOperator.model_fields
        assert not BoundOperator.model_fields["operator_session_id"].is_required()

    def test_status_field_exists_and_optional(self):
        """status field must exist and be optional in both definitions."""
        wire = _load_bound_operator_context_json()["bound_operator_context"]["fields"]
        assert "status" in wire
        assert wire["status"]["required"] is False
        
        # Verify VSE model has the field and it's optional
        assert "status" in BoundOperator.model_fields
        assert not BoundOperator.model_fields["status"].is_required()

    def test_all_json_fields_exist_in_vse_model(self):
        """All fields in shared JSON must exist in VSE BoundOperator model."""
        wire = _load_bound_operator_context_json()["bound_operator_context"]["fields"]
        for field_name in wire.keys():
            assert field_name in BoundOperator.model_fields, (
                f"shared/models/wire/bound_operator_context.json defines field '{field_name}' "
                f"but VSE BoundOperator model does not have this field"
            )

    def test_all_vse_model_fields_exist_in_json(self):
        """All fields in VSE BoundOperator model must exist in shared JSON."""
        wire = _load_bound_operator_context_json()["bound_operator_context"]["fields"]
        vse_fields = set(BoundOperator.model_fields.keys())
        json_fields = set(wire.keys())
        
        extra_fields = vse_fields - json_fields
        assert not extra_fields, (
            f"VSE BoundOperator has fields not in shared JSON: {extra_fields}. "
            "Add them to shared/models/wire/bound_operator_context.json first."
        )
