# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import pytest

from app.models.operators import (
    OperatorDocument,
    OperatorStatus,
)

pytestmark = [pytest.mark.unit]


class TestOperatorDocumentNoSystemInfoField:
    """Tests verifying OperatorDocument no longer has a system_info field."""

    def test_operator_document_has_no_system_info_field(self):
        """OperatorDocument should not have a system_info field."""
        doc = OperatorDocument(
            id="op-123",
            user_id="user-456",
            status=OperatorStatus.OFFLINE,
            current_hostname="test-hostname",
        )
        assert not hasattr(doc, "system_info")
        assert doc.current_hostname == "test-hostname"

    def test_hostname_property_returns_current_hostname(self):
        """The hostname property should return current_hostname."""
        doc = OperatorDocument(
            id="op-123",
            user_id="user-456",
            status=OperatorStatus.OFFLINE,
            current_hostname="test-hostname",
        )
        assert doc.hostname == "test-hostname"

    def test_operator_document_differentiation_fields(self):
        """OperatorDocument supports multi-operator differentiation fields."""
        doc = OperatorDocument(
            id="op-inf-1",
            user_id="user-1",
            status=OperatorStatus.ACTIVE,
            system_fingerprint="fp-sha256-composite",
            operator_role="inference",
            local_dir="/home/bob/op-inf",
            account="bob",
            port=8444,
            runtime_config={"inference_enabled": True, "http_port": 8444},
        )
        assert doc.operator_role == "inference"
        assert doc.local_dir == "/home/bob/op-inf"
        assert doc.account == "bob"
        assert doc.port == 8444
        assert doc.system_fingerprint == "fp-sha256-composite"
        assert doc.runtime_config == {"inference_enabled": True, "http_port": 8444}
