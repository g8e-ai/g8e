# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import pytest

from app.models.operators import OperatorDocument, OperatorStatus, HeartbeatSnapshot, HeartbeatSystemIdentity

pytestmark = [pytest.mark.unit]


class TestOperatorDocumentNoSystemInfoField:
    """Tests verifying OperatorDocument no longer has a system_info field."""

    def test_operator_document_has_no_system_info_field(self):
        """OperatorDocument should not have a system_info field."""
        doc = OperatorDocument(
            id="op-123",
            user_id="user-456",
            status=OperatorStatus.AVAILABLE,
            current_hostname="test-hostname",
        )
        assert not hasattr(doc, "system_info")
        assert doc.current_hostname == "test-hostname"

    def test_hostname_property_returns_current_hostname(self):
        """The hostname property should return current_hostname."""
        doc = OperatorDocument(
            id="op-123",
            user_id="user-456",
            status=OperatorStatus.AVAILABLE,
            current_hostname="test-hostname",
        )
        assert doc.hostname == "test-hostname"
