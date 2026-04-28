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
Unit tests for extract_single_operator_context.

Tests that the function reads exclusively from latest_heartbeat_snapshot
and handles missing snapshot gracefully.
"""

import pytest

from app.constants import OperatorType, CloudSubtype
from app.models.operators import (
    OperatorDocument,
    HeartbeatSnapshot,
    HeartbeatSystemIdentity,
    HeartbeatNetworkInfo,
    HeartbeatOSDetails,
    HeartbeatUserDetails,
    HeartbeatDiskDetails,
    HeartbeatMemoryDetails,
    HeartbeatEnvironment,
)
from app.services.investigation.investigation_service import extract_single_operator_context

pytestmark = [pytest.mark.unit]


class TestExtractSingleOperatorContext:
    """Test extract_single_operator_context reads from latest_heartbeat_snapshot."""

    def test_heartbeat_snapshot_present_all_fields_populated(self):
        """When heartbeat snapshot is present with all fields, context is fully populated."""
        operator = OperatorDocument(
            id="op-1",
            user_id="user-1",
            operator_session_id="sess-1",
            operator_type=OperatorType.SYSTEM,
            cloud_subtype=None,
            granted_intents=["read_logs"],
            latest_heartbeat_snapshot=HeartbeatSnapshot(
                system_identity=HeartbeatSystemIdentity(
                    hostname="test-host",
                    os="linux",
                    architecture="amd64",
                    cpu_count=8,
                    memory_mb=16384,
                    current_user="testuser",
                ),
                network=HeartbeatNetworkInfo(
                    public_ip="1.2.3.4",
                    internal_ip="10.0.0.1",
                    interfaces=["eth0"],
                ),
                os_details=HeartbeatOSDetails(
                    distro="ubuntu",
                    kernel="5.15.0",
                    version="22.04",
                ),
                user_details=HeartbeatUserDetails(
                    username="testuser",
                    uid="1000",
                    gid="1000",
                    home="/home/testuser",
                    shell="/bin/bash",
                ),
                environment=HeartbeatEnvironment(
                    pwd="/home/testuser",
                    timezone="UTC",
                    is_container=False,
                    container_runtime=None,
                    init_system="systemd",
                ),
                disk_details=HeartbeatDiskDetails(
                    percent=45.0,
                    total_gb=500,
                    free_gb=275,
                ),
                memory_details=HeartbeatMemoryDetails(
                    percent=60.0,
                    total_mb=16384,
                    available_mb=6553,
                ),
            ),
        )

        context = extract_single_operator_context(operator)

        assert context.operator_id == "op-1"
        assert context.operator_session_id == "sess-1"
        assert context.hostname == "test-host"
        assert context.os == "linux"
        assert context.architecture == "amd64"
        assert context.cpu_count == 8
        assert context.memory_mb == 16384
        assert context.public_ip == "1.2.3.4"
        assert context.distro == "ubuntu"
        assert context.kernel == "5.15.0"
        assert context.os_version == "22.04"
        assert context.username == "testuser"
        assert context.uid == 1000
        assert context.home_directory == "/home/testuser"
        assert context.shell == "/bin/bash"
        assert context.working_directory == "/home/testuser"
        assert context.timezone == "UTC"
        assert context.is_container is False
        assert context.container_runtime is None
        assert context.init_system == "systemd"
        assert context.disk_percent == 45.0
        assert context.disk_total_gb == 500
        assert context.disk_free_gb == 275
        assert context.memory_percent == 60.0
        assert context.memory_total_mb == 16384
        assert context.memory_available_mb == 6553

    def test_heartbeat_snapshot_present_only_system_identity(self):
        """When heartbeat snapshot has only system_identity, details fields are None."""
        operator = OperatorDocument(
            id="op-2",
            user_id="user-2",
            operator_session_id="sess-2",
            operator_type=OperatorType.CLOUD,
            cloud_subtype=CloudSubtype.AWS,
            granted_intents=["ec2_discovery"],
            latest_heartbeat_snapshot=HeartbeatSnapshot(
                system_identity=HeartbeatSystemIdentity(
                    hostname="cloud-host",
                    os="amazon-linux",
                    architecture="x86_64",
                    cpu_count=4,
                    memory_mb=8192,
                    current_user="ec2-user",
                ),
                network=HeartbeatNetworkInfo(
                    public_ip="54.123.45.67",
                ),
            ),
        )

        context = extract_single_operator_context(operator)

        assert context.operator_id == "op-2"
        assert context.operator_session_id == "sess-2"
        assert context.hostname == "cloud-host"
        assert context.os == "amazon-linux"
        assert context.architecture == "x86_64"
        assert context.cpu_count == 4
        assert context.memory_mb == 8192
        assert context.public_ip == "54.123.45.67"
        assert context.distro is None
        assert context.kernel is None
        assert context.os_version is None
        assert context.username is None
        assert context.uid is None
        assert context.home_directory is None
        assert context.shell is None
        assert context.working_directory is None
        assert context.timezone is None
        assert context.is_container is False
        assert context.container_runtime is None
        assert context.init_system is None
        assert context.disk_percent is None
        assert context.disk_total_gb is None
        assert context.disk_free_gb is None
        assert context.memory_percent is None
        assert context.memory_total_mb is None
        assert context.memory_available_mb is None

    def test_heartbeat_snapshot_none(self):
        """When heartbeat snapshot is None, all context fields are None except operator metadata."""
        operator = OperatorDocument(
            id="op-3",
            user_id="user-3",
            operator_session_id="sess-3",
            operator_type=OperatorType.SYSTEM,
            cloud_subtype=None,
            granted_intents=["read_logs"],
            latest_heartbeat_snapshot=None,
        )

        context = extract_single_operator_context(operator)

        assert context.operator_id == "op-3"
        assert context.operator_session_id == "sess-3"
        assert context.operator_type == OperatorType.SYSTEM
        assert context.granted_intents == ["read_logs"]
        assert context.hostname is None
        assert context.os is None
        assert context.architecture is None
        assert context.cpu_count is None
        assert context.memory_mb is None
        assert context.public_ip is None
        assert context.distro is None
        assert context.kernel is None
        assert context.os_version is None
        assert context.username is None
        assert context.uid is None
        assert context.home_directory is None
        assert context.shell is None
        assert context.working_directory is None
        assert context.timezone is None
        assert context.is_container is False
        assert context.container_runtime is None
        assert context.init_system is None
        assert context.disk_percent is None
        assert context.disk_total_gb is None
        assert context.disk_free_gb is None
        assert context.memory_percent is None
        assert context.memory_total_mb is None
        assert context.memory_available_mb is None

    def test_cloud_operator_context(self):
        """Cloud operator context includes cloud-specific fields from operator doc."""
        operator = OperatorDocument(
            id="op-4",
            user_id="user-4",
            operator_session_id="sess-4",
            operator_type=OperatorType.CLOUD,
            cloud_subtype=CloudSubtype.AWS,
            granted_intents=["ec2_discovery", "s3_read"],
            latest_heartbeat_snapshot=HeartbeatSnapshot(
                system_identity=HeartbeatSystemIdentity(
                    hostname="aws-host",
                    os="linux",
                    architecture="amd64",
                ),
                network=HeartbeatNetworkInfo(),
            ),
        )

        context = extract_single_operator_context(operator)

        assert context.operator_type == OperatorType.CLOUD
        assert context.is_cloud_operator is True
        assert context.granted_intents == ["ec2_discovery", "s3_read"]
