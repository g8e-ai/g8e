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
Contract test: g8ee SSEPushResponse model fields must exactly match the canonical
wire shape defined in shared/models/wire/sse_responses.json.

Prevents desynchronization between g8ed's SSEPushResponse.forWire() output
and g8ee's SSEPushResponse parsing logic.
"""

import json

import pytest

from app.models.g8ed_client import SSEPushResponse

pytestmark = pytest.mark.unit


def _load_sse_push_response_json() -> dict:
    with open("/app/shared/models/wire/sse_responses.json") as f:
        return json.load(f)


class TestSSEPushResponseFieldsMatchSharedJSON:
    """SSEPushResponse model fields must match shared/models/wire/sse_responses.json."""

    def test_success_field_exists_and_required(self):
        """success field must exist and be required in both definitions."""
        wire = _load_sse_push_response_json()["sse_push_response"]["fields"]
        assert "success" in wire
        assert wire["success"]["required"] is True
        
        # Verify g8ee model has the field
        assert "success" in SSEPushResponse.model_fields
        assert SSEPushResponse.model_fields["success"].is_required()

    def test_delivered_field_exists_and_optional_with_constraints(self):
        """delivered field must exist and be optional with default 0 and minimum 0."""
        wire = _load_sse_push_response_json()["sse_push_response"]["fields"]
        assert "delivered" in wire
        assert wire["delivered"]["required"] is False
        assert wire["delivered"]["default"] == 0
        assert wire["delivered"]["minimum"] == 0
        
        # Verify g8ee model has the field with correct constraints
        assert "delivered" in SSEPushResponse.model_fields
        assert not SSEPushResponse.model_fields["delivered"].is_required()
        assert SSEPushResponse.model_fields["delivered"].default == 0
        assert SSEPushResponse.model_fields["delivered"].ge == 0

    def test_error_field_exists_and_optional_with_default_null(self):
        """error field must exist and be optional with default null."""
        wire = _load_sse_push_response_json()["sse_push_response"]["fields"]
        assert "error" in wire
        assert wire["error"]["required"] is False
        assert wire["error"]["default"] is None
        
        # Verify g8ee model has the field and it's optional with default None
        assert "error" in SSEPushResponse.model_fields
        assert not SSEPushResponse.model_fields["error"].is_required()
        assert SSEPushResponse.model_fields["error"].default is None

    def test_all_json_fields_exist_in_g8ee_model(self):
        """All fields in shared JSON must exist in g8ee SSEPushResponse model."""
        wire = _load_sse_push_response_json()["sse_push_response"]["fields"]
        for field_name in wire.keys():
            assert field_name in SSEPushResponse.model_fields, (
                f"shared/models/wire/sse_responses.json defines field '{field_name}' "
                f"but g8ee SSEPushResponse model does not have this field"
            )

    def test_all_g8ee_model_fields_exist_in_json(self):
        """All fields in g8ee SSEPushResponse model must exist in shared JSON."""
        wire = _load_sse_push_response_json()["sse_push_response"]["fields"]
        g8ee_fields = set(SSEPushResponse.model_fields.keys())
        json_fields = set(wire.keys())
        
        extra_fields = g8ee_fields - json_fields
        assert not extra_fields, (
            f"g8ee SSEPushResponse has fields not in shared JSON: {extra_fields}. "
            "Add them to shared/models/wire/sse_responses.json first."
        )
