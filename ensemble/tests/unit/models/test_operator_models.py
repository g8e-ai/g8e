# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import pytest
from pydantic import ValidationError

from app.constants import OperatorRole, OperatorStatus, OperatorType
from app.models.operators import OperatorDocument

pytestmark = [pytest.mark.unit]


class TestOperatorDocumentNoSystemInfoField:
    """Tests verifying OperatorDocument no longer has a system_info field."""

    def test_operator_document_has_no_system_info_field(self):
        """OperatorDocument should not have a system_info field."""
        doc = OperatorDocument(
            operator_type=OperatorType.REMOTE,
            id="op-123",
            user_id="user-456",
            status=OperatorStatus.OFFLINE,
            current_hostname="test-hostname",
        )
        assert not hasattr(doc, "system_info")
        assert doc.current_hostname == "test-hostname"

    def test_operator_document_differentiation_fields(self):
        """OperatorDocument supports multi-operator differentiation fields."""
        doc = OperatorDocument.model_validate(
            {
                "operator_type": OperatorType.REMOTE,
                "id": "op-inf-1",
                "user_id": "user-1",
                "status": OperatorStatus.ACTIVE,
                "system_fingerprint": "fp-sha256-composite",
                "operator_roles": ["inference"],
                "local_dir": "/home/bob/op-inf",
                "account": "bob",
                "port": 8444,
                "runtime_config": {"inference_enabled": True, "http_port": 8444},
            }
        )
        assert doc.operator_roles == ["inference"]
        assert doc.local_dir == "/home/bob/op-inf"
        assert doc.account == "bob"
        assert doc.port == 8444
        assert doc.system_fingerprint == "fp-sha256-composite"
        assert doc.runtime_config is not None
        assert doc.runtime_config.inference_enabled is True
        assert doc.runtime_config.http_port == 8444


def test_operator_document_preserves_blended_typed_roles():
    doc = OperatorDocument.model_validate(
        {
            "operator_type": OperatorType.REMOTE,
            "id": "blended",
            "user_id": "owner",
            "status": OperatorStatus.ACTIVE,
            "operator_roles": ["embedded", "data", "inference", "provenance", "observer"],
            "runtime_config": {"roles": ["data", "inference", "provenance", "observer"]},
        }
    )
    assert len(doc.operator_roles) == 5
    assert doc.runtime_config is not None
    assert len(doc.runtime_config.roles) == 4
    assert (
        OperatorDocument.model_validate_json(doc.model_dump_json()).operator_roles
        == doc.operator_roles
    )
    with pytest.raises(ValidationError):
        OperatorDocument.model_validate(
            {"id": "invalid", "user_id": "owner", "operator_roles": ["cloud"]}
        )


def _operator(**fields) -> OperatorDocument:
    defaults = {
        "id": "op",
        "user_id": "owner",
        "status": OperatorStatus.ACTIVE,
        "operator_type": OperatorType.REMOTE,
        "operator_session_id": "session",
    }
    return OperatorDocument(**{**defaults, **fields})


def test_operator_document_requires_operator_type():
    with pytest.raises(ValidationError):
        OperatorDocument.model_validate({"id": "op", "user_id": "owner"})


def test_resolved_roles_defaults_to_data_without_role_metadata():
    assert _operator().resolved_roles() == {OperatorRole.DATA}
    assert _operator(runtime_config={}).resolved_roles() == {OperatorRole.DATA}


def test_resolved_roles_prefers_runtime_config_over_stored_roles():
    doc = _operator(
        operator_roles=["data"],
        runtime_config={"inference_enabled": True, "provenance_operator_enabled": True},
    )
    assert doc.resolved_roles() == {OperatorRole.INFERENCE, OperatorRole.PROVENANCE}


def test_resolved_roles_uses_stored_roles_without_runtime_config():
    assert _operator(operator_roles=["observer"]).resolved_roles() == {OperatorRole.OBSERVER}


def test_resolved_roles_marks_embedded_type():
    doc = _operator(operator_type=OperatorType.EMBEDDED, runtime_config={"roles": ["data"]})
    assert doc.resolved_roles() == {OperatorRole.EMBEDDED, OperatorRole.DATA}


def test_resolved_roles_embedded_implies_data():
    doc = _operator(operator_type=OperatorType.EMBEDDED, runtime_config={"roles": ["embedded"]})
    assert doc.resolved_roles() == {OperatorRole.EMBEDDED, OperatorRole.DATA}
    doc = _operator(operator_type=OperatorType.EMBEDDED, runtime_config={"roles": ["embedded", "inference"]})
    assert doc.resolved_roles() == {OperatorRole.EMBEDDED, OperatorRole.DATA, OperatorRole.INFERENCE}


@pytest.mark.parametrize(
    ("fields", "expected"),
    [
        ({}, True),
        ({"status": OperatorStatus.STALE}, False),
        ({"operator_session_id": None}, False),
        ({"operator_type": OperatorType.EMBEDDED}, False),
        ({"operator_type": OperatorType.EMBEDDED, "runtime_config": {}}, True),
    ],
)
def test_has_active_role_requires_live_session(fields, expected):
    assert _operator(**fields).has_active_role(OperatorRole.DATA) is expected


def test_has_active_role_checks_role_membership():
    doc = _operator(runtime_config={"inference_enabled": True})
    assert doc.has_active_role(OperatorRole.INFERENCE)
    assert not doc.has_active_role(OperatorRole.DATA)
