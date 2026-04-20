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
Unit tests for heartbeat models in operators.py and pubsub_messages.py.

Coverage:
- HeartbeatPerformanceMetrics: field names, types, defaults
- HeartbeatUptimeInfo: uptime_seconds is int, uptime_display field name
- HeartbeatSystemIdentity: all fields from wire schema
- HeartbeatNetworkInfo / HeartbeatNetworkInterface: connectivity_status shape
- HeartbeatVersionInfo: operator_version, status
- G8eoHeartbeatCapabilityFlags: nested shape, defaults
- OperatorHeartbeat.from_wire: full round-trip from a complete G8eoHeartbeatPayload
- HeartbeatSSEEnvelope.from_heartbeat / HeartbeatMetrics.from_heartbeat: all fields projected correctly
- HeartbeatSSEEnvelope.flatten_for_wire: nested models serialized to dicts
- _coerce_heartbeat_type: valid values pass through, unknown falls back to AUTOMATIC
"""

import pytest
from app.constants import HeartbeatType, OperatorStatus, VersionStability
from app.models.operators import (
    HeartbeatMetrics,
    HeartbeatNetworkInfo,
    HeartbeatNetworkInterface,
    HeartbeatPerformanceMetrics,
    HeartbeatSSEEnvelope,
    HeartbeatSystemIdentity,
    HeartbeatUptimeInfo,
    HeartbeatVersionInfo,
    OperatorHeartbeat,
    _coerce_heartbeat_type,
)
from app.models.pubsub_messages import (
    G8eoHeartbeatCapabilityFlags,
    G8eoHeartbeatPayload,
)

pytestmark = [pytest.mark.unit]


# =============================================================================
# Helpers
# =============================================================================

def _full_payload(**overrides) -> G8eoHeartbeatPayload:
    """Return a G8eoHeartbeatPayload with all sections populated."""
    from app.constants import EventType
    from app.models.pubsub_messages import (
        NetworkConnectivityStatus,
        G8eoHeartbeatDiskDetails,
        G8eoHeartbeatEnvironment,
        G8eoHeartbeatMemoryDetails,
        G8eoHeartbeatNetworkInfo,
        G8eoHeartbeatOSDetails,
        G8eoHeartbeatPerformanceMetrics,
        G8eoHeartbeatSystemIdentity,
        G8eoHeartbeatUptimeInfo,
        G8eoHeartbeatUserDetails,
        G8eoHeartbeatVersionInfo,
    )

    defaults = dict(
        event_type=EventType.OPERATOR_HEARTBEAT_SENT,
        operator_id="op-full-001",
        operator_session_id="sess-full-001",
        heartbeat_type=HeartbeatType.AUTOMATIC,
        system_identity=G8eoHeartbeatSystemIdentity(
            hostname="testhost",
            os="linux",
            architecture="amd64",
            pwd="/home/admin",
            current_user="admin",
            cpu_count=8,
            memory_mb=16384,
        ),
        network_info=G8eoHeartbeatNetworkInfo(
            public_ip="1.2.3.4",
            interfaces=["eth0", "lo"],
            connectivity_status=[
                NetworkConnectivityStatus(name="eth0", ip="192.168.1.10", mtu=1500),
                NetworkConnectivityStatus(name="lo", ip="127.0.0.1", mtu=65536),
            ],
        ),
        version_info=G8eoHeartbeatVersionInfo(
            operator_version="1.2.3",
            status="stable",
        ),
        uptime_info=G8eoHeartbeatUptimeInfo(
            uptime="2 days, 4:30:00",
            uptime_seconds=191400,
        ),
        performance_metrics=G8eoHeartbeatPerformanceMetrics(
            cpu_percent=42.5,
            memory_percent=61.0,
            disk_percent=35.0,
            network_latency=8.0,
            memory_used_mb=9830.0,
            memory_total_mb=16384.0,
            disk_used_gb=120.0,
            disk_total_gb=500.0,
        ),
        os_details=G8eoHeartbeatOSDetails(kernel="5.15.0", distro="Ubuntu", version="22.04"),
        user_details=G8eoHeartbeatUserDetails(
            username="admin", uid="1000", gid="1000",
            home="/home/admin", name="Admin User", shell="/bin/bash",
        ),
        disk_details=G8eoHeartbeatDiskDetails(total_gb=500.0, used_gb=120.0, free_gb=380.0, percent=24.0),
        memory_details=G8eoHeartbeatMemoryDetails(
            total_mb=16384, available_mb=6554, used_mb=9830, percent=60.0,
        ),
        environment=G8eoHeartbeatEnvironment(
            pwd="/home/admin", lang="en_US.UTF-8", timezone="UTC",
            term="xterm-256color", is_container=False,
        ),
        capability_flags=G8eoHeartbeatCapabilityFlags(
            local_storage_enabled=True,
            git_available=True,
            ledger_enabled=False,
        ),
    )
    defaults.update(overrides)
    return G8eoHeartbeatPayload(**defaults)


# =============================================================================
# HeartbeatPerformanceMetrics
# =============================================================================

class TestHeartbeatPerformanceMetrics:

    def test_field_network_latency_not_network_latency_ms(self):
        m = HeartbeatPerformanceMetrics(network_latency=12.0)
        assert hasattr(m, "network_latency")
        assert not hasattr(m, "network_latency_ms")

    def test_all_fields_default_to_none(self):
        m = HeartbeatPerformanceMetrics()
        assert m.cpu_percent is None
        assert m.memory_percent is None
        assert m.disk_percent is None
        assert m.network_latency is None
        assert m.memory_used_mb is None
        assert m.memory_total_mb is None
        assert m.disk_used_gb is None
        assert m.disk_total_gb is None

    def test_accepts_all_fields(self):
        m = HeartbeatPerformanceMetrics(
            cpu_percent=55.0,
            memory_percent=70.0,
            disk_percent=80.0,
            network_latency=5.0,
            memory_used_mb=4096.0,
            memory_total_mb=8192.0,
            disk_used_gb=200.0,
            disk_total_gb=500.0,
        )
        assert m.cpu_percent == 55.0
        assert m.disk_total_gb == 500.0
        assert m.network_latency == 5.0

    def test_network_latency_is_float_type(self):
        m = HeartbeatPerformanceMetrics(network_latency=7)
        assert isinstance(m.network_latency, float)

    def test_zero_values_preserved(self):
        m = HeartbeatPerformanceMetrics(cpu_percent=0.0, network_latency=0.0)
        assert m.cpu_percent == 0.0
        assert m.network_latency == 0.0


# =============================================================================
# HeartbeatUptimeInfo
# =============================================================================

class TestHeartbeatUptimeInfo:

    def test_uptime_display_field_exists(self):
        u = HeartbeatUptimeInfo(uptime_display="3 days", uptime_seconds=259200)
        assert u.uptime_display == "3 days"

    def test_uptime_seconds_is_int(self):
        u = HeartbeatUptimeInfo(uptime_seconds=3661)
        assert isinstance(u.uptime_seconds, int)
        assert u.uptime_seconds == 3661

    def test_both_fields_default_to_none(self):
        u = HeartbeatUptimeInfo()
        assert u.uptime_display is None
        assert u.uptime_seconds is None

    def test_no_uptime_seconds_float(self):
        u = HeartbeatUptimeInfo(uptime_seconds=86400)
        assert not isinstance(u.uptime_seconds, float)

    def test_large_uptime_seconds(self):
        u = HeartbeatUptimeInfo(uptime_seconds=31_536_000)
        assert u.uptime_seconds == 31_536_000


# =============================================================================
# HeartbeatSystemIdentity
# =============================================================================

class TestHeartbeatSystemIdentity:

    def test_all_fields_accepted(self):
        s = HeartbeatSystemIdentity(
            hostname="srv1",
            os="linux",
            architecture="amd64",
            pwd="/tmp",
            current_user="root",
            cpu_count=4,
            memory_mb=8192,
        )
        assert s.hostname == "srv1"
        assert s.os == "linux"
        assert s.architecture == "amd64"
        assert s.pwd == "/tmp"
        assert s.current_user == "root"
        assert s.cpu_count == 4
        assert s.memory_mb == 8192

    def test_all_fields_default_to_none(self):
        s = HeartbeatSystemIdentity()
        assert s.hostname is None
        assert s.os is None
        assert s.architecture is None
        assert s.pwd is None
        assert s.current_user is None
        assert s.cpu_count is None
        assert s.memory_mb is None


# =============================================================================
# HeartbeatNetworkInterface and HeartbeatNetworkInfo
# =============================================================================

class TestHeartbeatNetworkInterface:

    def test_has_name_ip_mtu_fields(self):
        iface = HeartbeatNetworkInterface(name="eth0", ip="10.0.0.1", mtu=1500)
        assert iface.name == "eth0"
        assert iface.ip == "10.0.0.1"
        assert iface.mtu == 1500

    def test_all_fields_default_to_none(self):
        iface = HeartbeatNetworkInterface()
        assert iface.name is None
        assert iface.ip is None
        assert iface.mtu is None

    def test_no_host_or_reachable_field(self):
        iface = HeartbeatNetworkInterface()
        assert not hasattr(iface, "host")
        assert not hasattr(iface, "reachable")
        assert not hasattr(iface, "latency_ms")


class TestHeartbeatNetworkInfo:

    def test_connectivity_status_is_list_of_interfaces(self):
        info = HeartbeatNetworkInfo(
            public_ip="1.2.3.4",
            interfaces=["eth0"],
            connectivity_status=[
                HeartbeatNetworkInterface(name="eth0", ip="192.168.1.1", mtu=1500)
            ],
        )
        assert info.connectivity_status is not None
        assert len(info.connectivity_status) == 1
        assert info.connectivity_status[0].name == "eth0"
        assert info.connectivity_status[0].ip == "192.168.1.1"
        assert info.connectivity_status[0].mtu == 1500

    def test_defaults_to_none(self):
        info = HeartbeatNetworkInfo()
        assert info.public_ip is None
        assert info.interfaces is None
        assert info.connectivity_status is None


# =============================================================================
# HeartbeatVersionInfo
# =============================================================================

class TestHeartbeatVersionInfo:

    def test_accepts_version_and_stability(self):
        v = HeartbeatVersionInfo(operator_version="2.0.0", status=VersionStability.STABLE)
        assert v.operator_version == "2.0.0"
        assert v.status == VersionStability.STABLE

    def test_defaults_to_none(self):
        v = HeartbeatVersionInfo()
        assert v.operator_version is None
        assert v.status is None


# =============================================================================
# G8eoHeartbeatCapabilityFlags
# =============================================================================

class TestG8eoHeartbeatCapabilityFlags:

    def test_defaults_all_false(self):
        flags = G8eoHeartbeatCapabilityFlags()
        assert flags.local_storage_enabled is False
        assert flags.git_available is False
        assert flags.ledger_enabled is False

    def test_accepts_all_true(self):
        flags = G8eoHeartbeatCapabilityFlags(
            local_storage_enabled=True,
            git_available=True,
            ledger_enabled=True,
        )
        assert flags.local_storage_enabled is True
        assert flags.git_available is True
        assert flags.ledger_enabled is True

    def test_mixed_values(self):
        flags = G8eoHeartbeatCapabilityFlags(
            local_storage_enabled=True,
            git_available=False,
            ledger_enabled=True,
        )
        assert flags.local_storage_enabled is True
        assert flags.git_available is False
        assert flags.ledger_enabled is True

    def test_field_name_is_local_storage_enabled_not_local_storage(self):
        flags = G8eoHeartbeatCapabilityFlags()
        assert hasattr(flags, "local_storage_enabled")
        assert not hasattr(flags, "local_storage")

    def test_field_name_is_ledger_enabled_not_ledger_mirror_enabled(self):
        flags = G8eoHeartbeatCapabilityFlags()
        assert hasattr(flags, "ledger_enabled")
        assert not hasattr(flags, "ledger_mirror_enabled")


# =============================================================================
# _coerce_heartbeat_type
# =============================================================================

class TestCoerceHeartbeatType:

    def test_automatic_string_returns_automatic(self):
        assert _coerce_heartbeat_type("automatic") == HeartbeatType.AUTOMATIC

    def test_bootstrap_string_returns_bootstrap(self):
        assert _coerce_heartbeat_type("bootstrap") == HeartbeatType.BOOTSTRAP

    def test_requested_string_returns_requested(self):
        assert _coerce_heartbeat_type("requested") == HeartbeatType.REQUESTED

    def test_enum_member_passes_through(self):
        assert _coerce_heartbeat_type(HeartbeatType.BOOTSTRAP) == HeartbeatType.BOOTSTRAP

    def test_unknown_string_falls_back_to_automatic(self):
        assert _coerce_heartbeat_type("unknown_garbage") == HeartbeatType.AUTOMATIC

    def test_empty_string_falls_back_to_automatic(self):
        assert _coerce_heartbeat_type("") == HeartbeatType.AUTOMATIC

    def test_none_falls_back_to_automatic(self):
        assert _coerce_heartbeat_type(None) == HeartbeatType.AUTOMATIC


# =============================================================================
# OperatorHeartbeat.from_wire — full round-trip
# =============================================================================

class TestOperatorHeartbeatFromWireFull:

    def test_system_identity_all_fields_mapped(self):
        hb = OperatorHeartbeat.from_wire(_full_payload())
        assert hb.system_identity.hostname == "testhost"
        assert hb.system_identity.os == "linux"
        assert hb.system_identity.architecture == "amd64"
        assert hb.system_identity.pwd == "/home/admin"
        assert hb.system_identity.current_user == "admin"
        assert hb.system_identity.cpu_count == 8
        assert hb.system_identity.memory_mb == 16384

    def test_performance_metrics_all_fields_mapped(self):
        hb = OperatorHeartbeat.from_wire(_full_payload())
        assert hb.performance.cpu_percent == 42.5
        assert hb.performance.memory_percent == 61.0
        assert hb.performance.disk_percent == 35.0
        assert hb.performance.network_latency == 8.0
        assert hb.performance.memory_used_mb == 9830.0
        assert hb.performance.memory_total_mb == 16384.0
        assert hb.performance.disk_used_gb == 120.0
        assert hb.performance.disk_total_gb == 500.0

    def test_network_latency_field_name_not_network_latency_ms(self):
        hb = OperatorHeartbeat.from_wire(_full_payload())
        assert hasattr(hb.performance, "network_latency")
        assert not hasattr(hb.performance, "network_latency_ms")

    def test_uptime_seconds_is_int(self):
        hb = OperatorHeartbeat.from_wire(_full_payload())
        assert hb.uptime.uptime_seconds == 191400
        assert isinstance(hb.uptime.uptime_seconds, int)

    def test_uptime_display_mapped_from_uptime_field(self):
        hb = OperatorHeartbeat.from_wire(_full_payload())
        assert hb.uptime.uptime_display == "2 days, 4:30:00"

    def test_network_info_mapped(self):
        hb = OperatorHeartbeat.from_wire(_full_payload())
        assert hb.network.public_ip == "1.2.3.4"
        assert hb.network.interfaces == ["eth0", "lo"]
        assert hb.network.connectivity_status is not None
        assert len(hb.network.connectivity_status) == 2
        assert hb.network.connectivity_status[0].name == "eth0"
        assert hb.network.connectivity_status[0].ip == "192.168.1.10"

    def test_version_info_mapped(self):
        hb = OperatorHeartbeat.from_wire(_full_payload())
        assert hb.version_info.operator_version == "1.2.3"

    def test_capability_flags_local_storage_enabled(self):
        hb = OperatorHeartbeat.from_wire(_full_payload())
        assert hb.local_storage_enabled is True

    def test_capability_flags_git_available(self):
        hb = OperatorHeartbeat.from_wire(_full_payload())
        assert hb.git_available is True

    def test_capability_flags_ledger_enabled_false(self):
        hb = OperatorHeartbeat.from_wire(_full_payload())
        assert hb.ledger_enabled is False

    def test_capability_flags_all_false_when_default(self):
        from app.models.pubsub_messages import G8eoHeartbeatCapabilityFlags
        payload = _full_payload(
            capability_flags=G8eoHeartbeatCapabilityFlags(
                local_storage_enabled=False,
                git_available=False,
                ledger_enabled=False,
            )
        )
        hb = OperatorHeartbeat.from_wire(payload)
        assert hb.local_storage_enabled is False
        assert hb.git_available is False
        assert hb.ledger_enabled is False

    def test_heartbeat_type_automatic(self):
        hb = OperatorHeartbeat.from_wire(_full_payload(heartbeat_type=HeartbeatType.AUTOMATIC))
        assert hb.heartbeat_type == HeartbeatType.AUTOMATIC.value

    def test_heartbeat_type_bootstrap(self):
        hb = OperatorHeartbeat.from_wire(_full_payload(heartbeat_type=HeartbeatType.BOOTSTRAP))
        assert hb.heartbeat_type == HeartbeatType.BOOTSTRAP.value

    def test_heartbeat_type_requested(self):
        hb = OperatorHeartbeat.from_wire(_full_payload(heartbeat_type=HeartbeatType.REQUESTED))
        assert hb.heartbeat_type == HeartbeatType.REQUESTED.value

    def test_os_details_mapped(self):
        hb = OperatorHeartbeat.from_wire(_full_payload())
        assert hb.os_details.kernel == "5.15.0"
        assert hb.os_details.distro == "Ubuntu"
        assert hb.os_details.version == "22.04"

    def test_user_details_mapped(self):
        hb = OperatorHeartbeat.from_wire(_full_payload())
        assert hb.user_details.username == "admin"
        assert hb.user_details.shell == "/bin/bash"

    def test_disk_details_mapped(self):
        hb = OperatorHeartbeat.from_wire(_full_payload())
        assert hb.disk_details.total_gb == 500.0
        assert hb.disk_details.used_gb == 120.0

    def test_memory_details_mapped(self):
        hb = OperatorHeartbeat.from_wire(_full_payload())
        assert hb.memory_details.total_mb == 16384
        assert hb.memory_details.used_mb == 9830

    def test_environment_mapped(self):
        hb = OperatorHeartbeat.from_wire(_full_payload())
        assert hb.environment.pwd == "/home/admin"
        assert hb.environment.timezone == "UTC"
        assert hb.environment.is_container is False

    def test_timestamp_is_set(self):
        from datetime import datetime
        hb = OperatorHeartbeat.from_wire(_full_payload())
        assert isinstance(hb.timestamp, datetime)

    def test_minimal_payload_does_not_raise(self):
        from app.constants import EventType
        payload = G8eoHeartbeatPayload(
            event_type=EventType.OPERATOR_HEARTBEAT_SENT,
            operator_id="op-min",
            operator_session_id="sess-min",
        )
        hb = OperatorHeartbeat.from_wire(payload)
        assert hb is not None
        assert hb.performance.cpu_percent is None
        assert hb.uptime.uptime_seconds is None


# =============================================================================
# HeartbeatSSEEnvelope.from_heartbeat — domain/wire boundary
# =============================================================================

class TestHeartbeatSSEEnvelope:
    """Verifies the authorship split: operator_id + status are g8ee-owned
    envelope fields; all telemetry is projected into the g8eo-authored
    ``metrics`` sub-model. Status never leaks into metrics."""

    def _make_heartbeat(self) -> OperatorHeartbeat:
        return OperatorHeartbeat.from_wire(_full_payload())

    def _make_envelope(self, status: OperatorStatus = OperatorStatus.ACTIVE) -> HeartbeatSSEEnvelope:
        return HeartbeatSSEEnvelope.from_heartbeat("op-sse-001", status, self._make_heartbeat())

    def test_operator_id_set_on_envelope(self):
        env = self._make_envelope()
        assert env.operator_id == "op-sse-001"

    def test_status_lives_on_envelope_not_metrics(self):
        env = self._make_envelope(OperatorStatus.ACTIVE)
        assert env.status == OperatorStatus.ACTIVE
        assert not hasattr(env.metrics, "status")

    def test_operator_id_not_duplicated_into_metrics(self):
        env = self._make_envelope()
        assert not hasattr(env.metrics, "operator_id")

    def test_status_bound_passed_through(self):
        assert self._make_envelope(OperatorStatus.BOUND).status == OperatorStatus.BOUND

    def test_status_offline_passed_through(self):
        assert self._make_envelope(OperatorStatus.OFFLINE).status == OperatorStatus.OFFLINE

    def test_metrics_is_typed_heartbeat_metrics(self):
        env = self._make_envelope()
        assert isinstance(env.metrics, HeartbeatMetrics)

    def test_hostname_projected_into_metrics(self):
        assert self._make_envelope().metrics.hostname == "testhost"

    def test_cpu_percent_projected_into_metrics(self):
        assert self._make_envelope().metrics.cpu_percent == 42.5

    def test_memory_percent_projected_into_metrics(self):
        assert self._make_envelope().metrics.memory_percent == 61.0

    def test_disk_percent_projected_into_metrics(self):
        assert self._make_envelope().metrics.disk_percent == 35.0

    def test_network_latency_field_name(self):
        m = self._make_envelope().metrics
        assert hasattr(m, "network_latency")
        assert not hasattr(m, "network_latency_ms")
        assert m.network_latency == 8.0

    def test_uptime_seconds_is_int(self):
        m = self._make_envelope().metrics
        assert m.uptime_seconds == 191400
        assert isinstance(m.uptime_seconds, int)

    def test_uptime_display_string_projected(self):
        assert self._make_envelope().metrics.uptime == "2 days, 4:30:00"

    def test_public_ip_projected(self):
        assert self._make_envelope().metrics.public_ip == "1.2.3.4"

    def test_operator_version_projected(self):
        assert self._make_envelope().metrics.operator_version == "1.2.3"

    def test_capability_flags_projected(self):
        m = self._make_envelope().metrics
        assert m.local_storage_enabled is True
        assert m.git_available is True
        assert m.ledger_enabled is False

    def test_heartbeat_type_preserved(self):
        hb = OperatorHeartbeat.from_wire(_full_payload(heartbeat_type=HeartbeatType.BOOTSTRAP))
        env = HeartbeatSSEEnvelope.from_heartbeat("op-sse-001", OperatorStatus.ACTIVE, hb)
        assert env.metrics.heartbeat_type == HeartbeatType.BOOTSTRAP.value

    def test_memory_used_mb_projected(self):
        assert self._make_envelope().metrics.memory_used_mb == 9830.0

    def test_disk_used_gb_projected(self):
        assert self._make_envelope().metrics.disk_used_gb == 120.0

    def test_nested_models_stay_typed_inside_boundary(self):
        """Application-boundary rule: nested fields are Pydantic instances, not dicts."""
        m = self._make_envelope().metrics
        from app.models.operators import (
            HeartbeatDiskDetails,
            HeartbeatEnvironment,
            HeartbeatMemoryDetails,
            HeartbeatOSDetails,
            HeartbeatUserDetails,
        )
        assert isinstance(m.os_details, HeartbeatOSDetails)
        assert isinstance(m.user_details, HeartbeatUserDetails)
        assert isinstance(m.disk_details, HeartbeatDiskDetails)
        assert isinstance(m.memory_details, HeartbeatMemoryDetails)
        assert isinstance(m.environment, HeartbeatEnvironment)


class TestHeartbeatSSEEnvelopeFlattenForWire:
    """Verifies serialization happens only at the wire boundary and that nested
    Pydantic models are emitted as plain dicts (no lingering model instances,
    no JSON-as-string coercion)."""

    def _make_envelope(self) -> HeartbeatSSEEnvelope:
        hb = OperatorHeartbeat.from_wire(_full_payload())
        return HeartbeatSSEEnvelope.from_heartbeat("op-sse-001", OperatorStatus.ACTIVE, hb)

    def test_envelope_top_level_shape(self):
        data = self._make_envelope().flatten_for_wire()
        assert set(data.keys()) == {"operator_id", "status", "metrics"}

    def test_status_is_string_on_wire(self):
        data = self._make_envelope().flatten_for_wire()
        assert data["status"] == OperatorStatus.ACTIVE.value

    def test_metrics_is_plain_dict(self):
        data = self._make_envelope().flatten_for_wire()
        assert isinstance(data["metrics"], dict)

    def test_nested_models_serialized_to_dicts(self):
        metrics = self._make_envelope().flatten_for_wire()["metrics"]
        assert isinstance(metrics["os_details"], dict)
        assert isinstance(metrics["user_details"], dict)
        assert isinstance(metrics["disk_details"], dict)
        assert isinstance(metrics["memory_details"], dict)
        assert isinstance(metrics["environment"], dict)

    def test_heartbeat_type_is_string_on_wire(self):
        metrics = self._make_envelope().flatten_for_wire()["metrics"]
        assert metrics["heartbeat_type"] == HeartbeatType.AUTOMATIC.value

    def test_timestamp_is_iso_string_on_wire(self):
        metrics = self._make_envelope().flatten_for_wire()["metrics"]
        assert isinstance(metrics["timestamp"], str)
        assert metrics["timestamp"].endswith("Z")

    def test_none_fields_excluded(self):
        hb = OperatorHeartbeat.from_wire(
            G8eoHeartbeatPayload(
                event_type="g8e.v1.operator.heartbeat.sent",
                operator_id="op-min",
                operator_session_id="sess-min",
            )
        )
        data = HeartbeatSSEEnvelope.from_heartbeat(
            "op-min", OperatorStatus.ACTIVE, hb
        ).flatten_for_wire()
        metrics = data["metrics"]
        assert "hostname" not in metrics
        assert "cpu_percent" not in metrics
