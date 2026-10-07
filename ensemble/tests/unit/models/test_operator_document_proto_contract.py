# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
Contract tests for OperatorDocument against the proto schema.

Enforces the invariant that the pydantic OperatorDocument and OperatorRuntimeConfig
models stay pinned to their canonical protobuf definitions
(g8e.operator.v1.OperatorDocument and g8e.operator.v1.OperatorRuntimeConfig).

Three enforcement points:
1. Every field in pydantic OperatorDocument exists in operator_pb2.OperatorDocument.DESCRIPTOR.fields_by_name
   (exceptions listed explicitly with reasons).
2. Every field in pydantic OperatorRuntimeConfig exists in operator_pb2.OperatorRuntimeConfig.DESCRIPTOR.fields_by_name.
3. The golden operator-document.json fixture (shared with the Gateway) loads and validates
   via operator_document_from_gateway() and pydantic model_validate(), with key values checked.
4. Documents with empty string operator_type and status validate (proto3 default mapping).
"""

import json
from pathlib import Path

import pytest
from g8e.operator.v1 import operator_pb2

from app.models.operators import OperatorDocument, OperatorRuntimeConfig
from app.utils.gateway_decoding.gateway_operator_document import operator_document_from_gateway

pytestmark = [pytest.mark.unit]


# Path to the protocol operator-document.json fixture, resolved from this test file's
# location (ensemble/tests/unit/models/) up to the workspace root.
_OPERATOR_DOCUMENT_FIXTURE_PATH = (
    Path(__file__).resolve().parent.parent.parent.parent.parent
    / "protocol"
    / "test-fixtures"
    / "operator-document.json"
)


class TestOperatorDocumentFieldsContract:
    """Verify every pydantic OperatorDocument field exists in the proto schema."""

    def test_operator_document_fields_exist_in_proto(self):
        """Every pydantic field (except documented exceptions) must exist in operator_pb2."""
        proto_fields = set(operator_pb2.OperatorDocument.DESCRIPTOR.fields_by_name.keys())

        pydantic_model = OperatorDocument.model_json_schema()
        pydantic_properties = set(pydantic_model.get("properties", {}).keys())

        # Fields that exist in pydantic but not in proto, with explicit reasons
        exceptions = {
            # These fields are calculated/derived in g8ee, not persisted by Gateway
            "claimed_at",  # Populated from proto but used in OperatorDocument
            "latest_heartbeat_snapshot",  # Coerced/transformed (proto field is HeartbeatResult)
            # g8ee-only fields not in the Gateway proto schema
            "granted_intents",  # Kept in pydantic (used in tests/context building) but not in proto
        }

        missing_in_proto = pydantic_properties - proto_fields - exceptions

        assert not missing_in_proto, (
            f"Pydantic OperatorDocument fields {missing_in_proto} not found in operator_pb2. "
            "Either add them to the proto schema or add them to the exceptions set with a reason."
        )

    def test_operator_document_proto_fields_are_documented(self):
        """Spot check: key proto fields are present in pydantic."""
        proto_fields = set(operator_pb2.OperatorDocument.DESCRIPTOR.fields_by_name.keys())
        expected_fields = {
            "id",
            "user_id",
            "operator_type",
            "status",
            "operator_session_id",
            "organization_id",
            "system_fingerprint",
            "operator_roles",
            "local_dir",
            "account",
            "port",
            "runtime_config",
            "current_hostname",
            "created_at",
            "updated_at",
        }
        assert expected_fields.issubset(proto_fields), (
            f"Key operator_pb2 fields {expected_fields - proto_fields} missing from proto descriptor"
        )


class TestOperatorRuntimeConfigFieldsContract:
    """Verify every pydantic OperatorRuntimeConfig field exists in the proto schema."""

    def test_operator_runtime_config_fields_exist_in_proto(self):
        """Every pydantic OperatorRuntimeConfig field must exist in operator_pb2."""
        proto_fields = set(operator_pb2.OperatorRuntimeConfig.DESCRIPTOR.fields_by_name.keys())

        pydantic_model = OperatorRuntimeConfig.model_json_schema()
        pydantic_properties = set(pydantic_model.get("properties", {}).keys())

        missing_in_proto = pydantic_properties - proto_fields

        assert not missing_in_proto, (
            f"Pydantic OperatorRuntimeConfig fields {missing_in_proto} not found in operator_pb2. "
            "Update the proto schema or remove them from the pydantic model."
        )


class TestOperatorDocumentGoldenFixture:
    """Load and validate the golden operator-document.json fixture."""

    @pytest.fixture
    def golden_fixture_data(self) -> dict:
        """Load the golden operator-document.json fixture."""
        if not _OPERATOR_DOCUMENT_FIXTURE_PATH.exists():
            pytest.skip(
                f"Golden fixture {_OPERATOR_DOCUMENT_FIXTURE_PATH} not found. "
                "Ensure protocol/test-fixtures/operator-document.json is present."
            )
        with _OPERATOR_DOCUMENT_FIXTURE_PATH.open() as f:
            return json.load(f)

    def test_golden_fixture_loads_via_gateway_decoder(self, golden_fixture_data):
        """operator_document_from_gateway() must parse the golden fixture."""
        doc = operator_document_from_gateway(golden_fixture_data)
        assert isinstance(doc, OperatorDocument)
        assert doc.id == "op-golden-1"

    def test_golden_fixture_validates_via_model_validate(self, golden_fixture_data):
        """OperatorDocument.model_validate() must accept the golden fixture."""
        doc = OperatorDocument.model_validate(golden_fixture_data)
        assert doc.id == "op-golden-1"
        assert doc.user_id == "user-golden-1"

    def test_golden_fixture_key_values(self, golden_fixture_data):
        """Verify key field values in the golden fixture."""
        doc = OperatorDocument.model_validate(golden_fixture_data)

        # Core identity
        assert doc.id == "op-golden-1"
        assert doc.user_id == "user-golden-1"
        assert doc.operator_session_id == "session-golden-1"

        # Status and type
        assert doc.status == "active"
        assert doc.operator_type == "remote"

        # Heartbeat (transforms through coercion)
        assert doc.latest_heartbeat_snapshot is not None

        # Runtime configuration
        assert doc.runtime_config is not None
        assert doc.runtime_config.inference_enabled is True
        assert doc.runtime_config.http_port == 8080
        assert doc.runtime_config.roles == ["data", "inference"]

        # Operator roles
        assert "data" in doc.operator_roles
        assert "inference" in doc.operator_roles

        # Timestamps parsed (RFC3339)
        assert doc.created_at is not None
        assert doc.updated_at is not None

    def test_empty_string_operator_type_coerces_to_default(self):
        """A document with operator_type='' coerces to REMOTE (proto3 default-safe)."""
        data = {
            "id": "op-test",
            "user_id": "user-test",
            "operator_type": "",  # Empty string in proto3 default
        }
        # Should not raise ValidationError; empty string maps to REMOTE
        doc = OperatorDocument.model_validate(data)
        assert doc.operator_type == "remote"

    def test_empty_string_status_coerces_to_default(self):
        """A document with status='' coerces to OFFLINE (proto3 default-safe)."""
        data = {
            "id": "op-test",
            "user_id": "user-test",
            "operator_type": "remote",
            "status": "",  # Empty string in proto3 default
        }
        # Should not raise ValidationError; empty string maps to OFFLINE
        doc = OperatorDocument.model_validate(data)
        assert doc.status == "offline"

    def test_roundtrip_through_json_serialization(self, golden_fixture_data):
        """Document must roundtrip through JSON serialization."""
        doc = OperatorDocument.model_validate(golden_fixture_data)
        json_str = doc.model_dump_json()
        doc2 = OperatorDocument.model_validate_json(json_str)
        assert doc.id == doc2.id
        assert doc.user_id == doc2.user_id
        assert doc.operator_type == doc2.operator_type
