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

from app.models.operators import OperatorDocument, OperatorSystemInfo, OperatorStatus

pytestmark = [pytest.mark.unit]


class TestOperatorDocumentCurrentHostnameSync:
    """Tests for current_hostname denormalization sync with system_info.hostname."""

    def test_current_hostname_syncs_from_system_info_when_null(self):
        """When current_hostname is null, it should sync from system_info.hostname."""
        system_info = OperatorSystemInfo(hostname="test-hostname")
        doc = OperatorDocument(
            id="op-123",
            user_id="user-456",
            status=OperatorStatus.AVAILABLE,
            system_info=system_info,
            current_hostname=None,
        )
        assert doc.current_hostname == "test-hostname"

    def test_current_hostname_preserved_when_explicitly_set(self):
        """When current_hostname is explicitly set, it should not be overridden."""
        system_info = OperatorSystemInfo(hostname="system-info-hostname")
        doc = OperatorDocument(
            id="op-123",
            user_id="user-456",
            status=OperatorStatus.AVAILABLE,
            system_info=system_info,
            current_hostname="explicit-hostname",
        )
        assert doc.current_hostname == "explicit-hostname"

    def test_current_hostname_null_when_system_info_hostname_null(self):
        """When both current_hostname and system_info.hostname are null, current_hostname should remain null."""
        system_info = OperatorSystemInfo(hostname=None)
        doc = OperatorDocument(
            id="op-123",
            user_id="user-456",
            status=OperatorStatus.AVAILABLE,
            system_info=system_info,
            current_hostname=None,
        )
        assert doc.current_hostname is None

    def test_hostname_property_returns_current_hostname(self):
        """The hostname property should return current_hostname for backward compatibility."""
        doc = OperatorDocument(
            id="op-123",
            user_id="user-456",
            status=OperatorStatus.AVAILABLE,
            current_hostname="test-hostname",
        )
        assert doc.hostname == "test-hostname"

    def test_current_hostname_sync_with_dict_system_info(self):
        """Sync should work when system_info is provided as a dict."""
        doc = OperatorDocument(
            id="op-123",
            user_id="user-456",
            status=OperatorStatus.AVAILABLE,
            system_info={"hostname": "dict-hostname"},
            current_hostname=None,
        )
        assert doc.current_hostname == "dict-hostname"
