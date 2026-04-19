from __future__ import annotations

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
Operator models for g8e system.

Defines data structures for tracking g8eo operators and their runtime configurations.
"""

import asyncio
import logging
from datetime import datetime
from typing import Optional

from pydantic import ConfigDict, Field, PrivateAttr, field_validator

from app.constants import (
    ApprovalErrorType,
    ApprovalType,
    AttachmentType,
    CloudIntent,
    CloudSubtype,
    CommandErrorType,
    ExecutionStatus,
    FileOperation,
    HeartbeatType,
    OperatorStatus,
    OperatorType,
    VersionStability,
)
from app.models.pubsub_messages import G8eoHeartbeatPayload
from app.models.tool_results import (
    CommandInternalResult,
    CommandRiskAnalysis,
    FileOperationRiskAnalysis,
)
from app.utils.timestamp import now

from .base import G8eBaseModel, G8eIdentifiableModel

logger = logging.getLogger(__name__)


class SystemInfoFingerprintDetails(G8eBaseModel):
    """Structured fingerprint sub-fields from wire/system_info.json."""
    os: str | None = Field(default=None)
    architecture: str | None = Field(default=None)
    cpu_count: int | None = Field(default=None)
    machine_id: str | None = Field(default=None)


class SystemInfoOSDetails(G8eBaseModel):
    """OS detail sub-fields from wire/system_info.json."""
    kernel: str | None = Field(default=None)
    distro: str | None = Field(default=None)
    version: str | None = Field(default=None)


class SystemInfoUserDetails(G8eBaseModel):
    """User detail sub-fields from wire/system_info.json."""
    username: str | None = Field(default=None)
    uid: int | None = Field(default=None)
    gid: int | None = Field(default=None)
    home: str | None = Field(default=None)
    name: str | None = Field(default=None)
    shell: str | None = Field(default=None)


class SystemInfoDiskDetails(G8eBaseModel):
    """Root filesystem disk usage sub-fields from wire/system_info.json."""
    total_gb: float | None = Field(default=None)
    used_gb: float | None = Field(default=None)
    free_gb: float | None = Field(default=None)
    percent: float | None = Field(default=None)


class SystemInfoMemoryDetails(G8eBaseModel):
    """Memory usage sub-fields from wire/system_info.json."""
    total_mb: int | None = Field(default=None)
    available_mb: int | None = Field(default=None)
    used_mb: int | None = Field(default=None)
    percent: float | None = Field(default=None)


class SystemInfoEnvironment(G8eBaseModel):
    """Environment and container context sub-fields from wire/system_info.json."""
    pwd: str | None = Field(default=None)
    lang: str | None = Field(default=None)
    timezone: str | None = Field(default=None)
    term: str | None = Field(default=None)
    is_container: bool | None = Field(default=None)
    container_runtime: str | None = Field(default=None)
    container_signals: list[str] | None = Field(default=None)
    init_system: str | None = Field(default=None)


class OperatorSystemInfo(G8eBaseModel):
    """System information sent by g8eo.

    Canonical shape defined in shared/models/wire/system_info.json.
    Populated from g8eo auth payload and updated on each heartbeat.
    """

    hostname: str | None = Field(default=None, description="System hostname")
    os: str | None = Field(default=None, description="Operating system name (e.g. linux)")
    architecture: str | None = Field(default=None, description="CPU architecture (e.g. amd64, arm64)")
    cpu_count: int | None = Field(default=None, description="Number of logical CPU cores")
    memory_mb: int | None = Field(default=None, description="Total system memory in MB")
    public_ip: str | None = Field(default=None, description="Public IPv4 address")
    internal_ip: str | None = Field(default=None, description="Primary internal/private IP address")
    interfaces: list[str] = Field(default_factory=list, description="Network interface names")
    current_user: str | None = Field(default=None, description="OS user running the operator")
    system_fingerprint: str | None = Field(default=None, description="SHA256 fingerprint of stable host attributes")
    fingerprint_details: SystemInfoFingerprintDetails | None = Field(default=None, description="Sub-fields used to compute system_fingerprint")
    os_details: SystemInfoOSDetails | None = Field(default=None, description="Detailed OS information")
    user_details: SystemInfoUserDetails | None = Field(default=None, description="Current OS user details")
    disk_details: SystemInfoDiskDetails | None = Field(default=None, description="Root filesystem disk usage")
    memory_details: SystemInfoMemoryDetails | None = Field(default=None, description="Detailed memory usage")
    environment: SystemInfoEnvironment | None = Field(default=None, description="Environment and container context")
    is_cloud_operator: bool | None = Field(default=False, description="True when this is a cloud-hosted operator")
    cloud_provider: str | None = Field(default=None, description="Cloud provider identifier")
    local_storage_enabled: bool | None = Field(default=True, description="True when local vault storage is active")


class AttachmentRecord(G8eIdentifiableModel):
    """Stored attachment metadata record (no binary data)."""
    filename: str | None = Field(default=None, description="Original filename")
    content_type: str | None = Field(default=None, description="MIME content type")
    size: int | None = Field(default=None, ge=0, description="File size in bytes")
    type: AttachmentType = Field(default=AttachmentType.OTHER, description="Classified attachment type")


class CommandResultRecord(G8eBaseModel):
    """Stored command execution metadata record (no stdout/stderr content)."""
    execution_id: str | None = Field(default=None, description="Unique execution identifier")
    command: str | None = Field(default=None, description="Command that was executed")
    status: ExecutionStatus = Field(default=ExecutionStatus.COMPLETED, description="Execution status")
    exit_code: int | None = Field(default=None, description="Exit code from command")
    stdout_hash: str | None = Field(default=None, description="SHA256 hash of stdout")
    stderr_hash: str | None = Field(default=None, description="SHA256 hash of stderr")
    stdout_size: int = Field(default=0, ge=0, description="Size of stdout in bytes")
    stderr_size: int = Field(default=0, ge=0, description="Size of stderr in bytes")
    stored_locally: bool = Field(default=True, description="Whether output is stored on operator")
    execution_time_seconds: float | None = Field(default=None, description="Execution duration in seconds")
    timestamp: datetime = Field(default_factory=now, description="When the command completed")
    investigation_id: str | None = Field(default=None, description="Associated investigation ID")
    case_id: str | None = Field(default=None, description="Associated case ID")
    operator_session_id: str | None = Field(default=None, description="Associated operator session ID")


class OperatorDocument(G8eBaseModel):
    """g8ee read-side projection of the g8ed OperatorDocument.

    Maps to operator_status_info in shared/models/operator_document.json.
    Populated from g8es KV cache keyed by KVKey.doc(Collections.OPERATORS, operator_id) or
    GET /api/internal/operators/:operatorId/status.
    g8ed is the authority — g8ee only reads this document.
    """

    operator_id: str = Field(description="Unique Operator identifier")
    user_id: str | None = Field(default=None, description="User ID who owns this operator")
    name: str | None = Field(default=None, description="Human-readable operator name")
    organization_id: str | None = Field(default=None, description="Organization ID")
    status: OperatorStatus = Field(default=OperatorStatus.AVAILABLE, description="Current Operator status")
    bound_web_session_id: str | None = Field(default=None, description="Bound web session ID")
    operator_session_id: str | None = Field(default=None, description="Current Operator session ID")
    last_heartbeat: datetime | None = Field(default=None, description="Last heartbeat timestamp")
    system_info: OperatorSystemInfo | None = Field(default=None, description="System information")
    latest_heartbeat_snapshot: Optional["OperatorHeartbeat"] = Field(default=None, description="Latest heartbeat metrics")
    investigation_id: str | None = Field(default=None, description="Current investigation ID")
    case_id: str | None = Field(default=None, description="Current case ID")
    is_active: bool = Field(default=False, description="Whether Operator is in active status")
    operator_type: OperatorType = Field(default=OperatorType.SYSTEM, description="Operator deployment type")
    granted_intents: list[str] | None = Field(default=None, description="Granted intent permissions (cloud operators)")
    cloud_subtype: CloudSubtype | None = Field(default=None, description="Cloud provider subtype")
    current_hostname: str | None = Field(default=None, description="Denormalized hostname from system_info for quick access")
    session_token: str | None = Field(default=None, description="Active session token for session-based auth validation")
    session_expires_at: datetime | None = Field(default=None, description="Session expiration timestamp")

    @property
    def hostname(self) -> str | None:
        """Get hostname from current_hostname for backward compatibility."""
        return self.current_hostname

    @field_validator("status", mode="before")
    @classmethod
    def coerce_status(cls, v: object) -> OperatorStatus:
        try:
            return OperatorStatus(v)
        except (ValueError, KeyError):
            logger.warning("Unknown OperatorStatus value %r — defaulting to AVAILABLE", v)
            return OperatorStatus.AVAILABLE

    @field_validator("latest_heartbeat_snapshot", mode="before")
    @classmethod
    def coerce_heartbeat_snapshot(cls, v: object) -> object:
        if isinstance(v, dict):
            try:
                return OperatorHeartbeat.model_validate(v)
            except Exception:
                return None
        return v

    @field_validator("granted_intents", mode="before")
    @classmethod
    def coerce_granted_intents(cls, v: object) -> list[str]:
        if v is None:
            return []
        if isinstance(v, list):
            return [str(item) for item in v]
        raise TypeError("granted_intents must be a list[str] or None")


# =============================================================================
# HEARTBEAT DATA MODELS
# =============================================================================
# Clean, normalized heartbeat data structure for Operator telemetry.
# Heartbeats are stored in database (operator document) and sent via SSE to g8ed.
# Last 10 heartbeats are retained in a rolling buffer for historical context.
# =============================================================================



class HeartbeatSystemIdentity(G8eBaseModel):
    """System identity information from Operator heartbeat."""
    hostname: str | None = Field(default=None, description="System hostname")
    os: str | None = Field(default=None, description="Operating system name")
    architecture: str | None = Field(default=None, description="CPU architecture (amd64, arm64)")
    pwd: str | None = Field(default=None, description="Current working directory")
    current_user: str | None = Field(default=None, description="Current logged-in user")
    cpu_count: int | None = Field(default=None, description="Number of CPU cores")
    memory_mb: int | None = Field(default=None, description="Total system memory in MB")


class HeartbeatPerformanceMetrics(G8eBaseModel):
    """Performance metrics from Operator heartbeat."""
    cpu_percent: float | None = Field(default=None, description="CPU usage percentage (0-100)")
    memory_percent: float | None = Field(default=None, description="Memory usage percentage (0-100)")
    disk_percent: float | None = Field(default=None, description="Disk usage percentage (0-100)")
    network_latency: float | None = Field(default=None, description="Network latency in milliseconds")
    memory_used_mb: float | None = Field(default=None, description="Memory used in MB")
    memory_total_mb: float | None = Field(default=None, description="Total memory in MB")
    disk_used_gb: float | None = Field(default=None, description="Disk used in GB")
    disk_total_gb: float | None = Field(default=None, description="Total disk in GB")


class HeartbeatNetworkInterface(G8eBaseModel):
    """A single active network interface reported in a heartbeat."""
    name: str | None = Field(default=None, description="Interface name (e.g. eth0)")
    ip: str | None = Field(default=None, description="IP address assigned to the interface")
    mtu: int | None = Field(default=None, description="Interface MTU")


class HeartbeatNetworkInfo(G8eBaseModel):
    """Network information from Operator heartbeat."""
    public_ip: str | None = Field(default=None, description="Public IP address")
    interfaces: list[str] | None = Field(default=None, description="List of network interface names")
    connectivity_status: list[HeartbeatNetworkInterface] | None = Field(default=None, description="Active network interfaces with IP and MTU")


class HeartbeatUptimeInfo(G8eBaseModel):
    """Uptime information from Operator heartbeat."""
    uptime_display: str | None = Field(default=None, description="Human-readable uptime string")
    uptime_seconds: int | None = Field(default=None, description="Uptime in seconds")


HeartbeatOSDetails = SystemInfoOSDetails
HeartbeatUserDetails = SystemInfoUserDetails
HeartbeatDiskDetails = SystemInfoDiskDetails
HeartbeatMemoryDetails = SystemInfoMemoryDetails
HeartbeatEnvironment = SystemInfoEnvironment


def _coerce_heartbeat_type(value: object) -> HeartbeatType:
    try:
        return HeartbeatType(value)
    except (ValueError, KeyError):
        logger.warning("Unknown HeartbeatType value %r — defaulting to AUTOMATIC", value)
        return HeartbeatType.AUTOMATIC


class HeartbeatVersionInfo(G8eBaseModel):
    """Operator version metadata from heartbeat."""
    operator_version: str | None = Field(default=None, description="Operator (g8eo) binary version")
    status: VersionStability | None = Field(default=None, description="Version stability status")



class OperatorHeartbeat(G8eBaseModel):
    """
    Clean, normalized heartbeat data structure.
    
    This is the canonical representation of Operator heartbeat data.
    g8eo sends heartbeats every 30 seconds with system telemetry.
    
    Storage:
    - Stored in database Operator document (heartbeat_history array, max 10)
    - Sent to g8ed via SSE for real-time UI updates
    - NOT stored in g8es cache
    
    Usage:
    - AI context for understanding Operator system state
    - UI display of Operator health metrics
    - Debugging and troubleshooting
    """

    # Timestamp and type
    timestamp: datetime = Field(default_factory=now, description="When heartbeat was received")
    heartbeat_type: HeartbeatType = Field(default=HeartbeatType.AUTOMATIC, description="Heartbeat type")

    # System identity (static info about the machine)
    system_identity: HeartbeatSystemIdentity = Field(
        default_factory=HeartbeatSystemIdentity,
        description="System identity information"
    )

    # Performance metrics (dynamic resource usage)
    performance: HeartbeatPerformanceMetrics = Field(
        default_factory=HeartbeatPerformanceMetrics,
        description="Current performance metrics"
    )

    # Network info
    network: HeartbeatNetworkInfo = Field(
        default_factory=HeartbeatNetworkInfo,
        description="Network information"
    )

    # Uptime
    uptime: HeartbeatUptimeInfo = Field(
        default_factory=HeartbeatUptimeInfo,
        description="Uptime information"
    )

    # OS details
    os_details: HeartbeatOSDetails = Field(
        default_factory=HeartbeatOSDetails,
        description="Operating system details"
    )

    # User details
    user_details: HeartbeatUserDetails = Field(
        default_factory=HeartbeatUserDetails,
        description="Current user details"
    )

    # Environment
    environment: HeartbeatEnvironment = Field(
        default_factory=HeartbeatEnvironment,
        description="Environment and container context"
    )

    # Operator version
    version_info: HeartbeatVersionInfo = Field(
        default_factory=HeartbeatVersionInfo,
        description="Operator version metadata"
    )

    # Detailed disk and memory sub-reports
    disk_details: HeartbeatDiskDetails = Field(
        default_factory=HeartbeatDiskDetails,
        description="Detailed root filesystem disk usage"
    )

    memory_details: HeartbeatMemoryDetails = Field(
        default_factory=HeartbeatMemoryDetails,
        description="Detailed memory usage"
    )

    # Network identity fields (top-level in g8eo payload, not nested under network_info)
    internal_ip: str | None = Field(default=None, description="Primary internal/private IP address")
    system_fingerprint: str | None = Field(default=None, description="SHA256 fingerprint of stable host attributes")
    fingerprint_details: SystemInfoFingerprintDetails | None = Field(default=None, description="Sub-fields used to compute system_fingerprint")

    # Cloud operator flags
    is_cloud_operator: bool = Field(default=False, description="True when this is a cloud-hosted operator")
    cloud_provider: str | None = Field(default=None, description="Cloud provider identifier")

    # Operator capability flags
    local_storage_enabled: bool = Field(default=False, description="True when local (Project Chronos) storage is active")
    git_available: bool = Field(default=False, description="True when git is available on the operator host")
    ledger_enabled: bool = Field(default=False, description="True when LFAA ledger mirroring is active")

    @classmethod
    def from_wire(cls, payload: G8eoHeartbeatPayload) -> "OperatorHeartbeat":
        """Create OperatorHeartbeat from the typed g8eo wire payload.

        Canonical wire shape: shared/models/wire/heartbeat.json.
        Validation happens once at the pub/sub boundary in heartbeat_service.py
        before this is called.
        """
        cap = payload.capability_flags
        return cls(
            timestamp=now(),
            heartbeat_type=_coerce_heartbeat_type(payload.heartbeat_type),
            system_identity=HeartbeatSystemIdentity(
                hostname=payload.system_identity.hostname,
                os=payload.system_identity.os,
                architecture=payload.system_identity.architecture,
                pwd=payload.system_identity.pwd,
                current_user=payload.system_identity.current_user,
                cpu_count=payload.system_identity.cpu_count,
                memory_mb=payload.system_identity.memory_mb,
            ),
            performance=HeartbeatPerformanceMetrics(
                cpu_percent=payload.performance_metrics.cpu_percent,
                memory_percent=payload.performance_metrics.memory_percent,
                disk_percent=payload.performance_metrics.disk_percent,
                network_latency=payload.performance_metrics.network_latency,
                memory_used_mb=payload.performance_metrics.memory_used_mb,
                memory_total_mb=payload.performance_metrics.memory_total_mb,
                disk_used_gb=payload.performance_metrics.disk_used_gb,
                disk_total_gb=payload.performance_metrics.disk_total_gb,
            ),
            network=HeartbeatNetworkInfo(
                public_ip=payload.network_info.public_ip,
                interfaces=payload.network_info.interfaces,
                connectivity_status=[
                    HeartbeatNetworkInterface(name=s.name, ip=s.ip, mtu=s.mtu)
                    for s in payload.network_info.connectivity_status
                ] if payload.network_info.connectivity_status is not None else None,
            ),
            uptime=HeartbeatUptimeInfo(
                uptime_display=payload.uptime_info.uptime,
                uptime_seconds=payload.uptime_info.uptime_seconds,
            ),
            os_details=HeartbeatOSDetails(
                kernel=payload.os_details.kernel,
                distro=payload.os_details.distro,
                version=payload.os_details.version,
            ),
            user_details=HeartbeatUserDetails(
                username=payload.user_details.username,
                uid=payload.user_details.uid,
                gid=payload.user_details.gid,
                home=payload.user_details.home,
                name=payload.user_details.name,
                shell=payload.user_details.shell,
            ),
            environment=HeartbeatEnvironment(
                pwd=payload.environment.pwd,
                lang=payload.environment.lang,
                timezone=payload.environment.timezone,
                term=payload.environment.term,
                is_container=payload.environment.is_container,
                container_runtime=payload.environment.container_runtime,
                container_signals=payload.environment.container_signals,
                init_system=payload.environment.init_system,
            ),
            version_info=HeartbeatVersionInfo(
                operator_version=payload.version_info.operator_version,
                status=payload.version_info.status,
            ),
            disk_details=HeartbeatDiskDetails(
                total_gb=payload.disk_details.total_gb,
                used_gb=payload.disk_details.used_gb,
                free_gb=payload.disk_details.free_gb,
                percent=payload.disk_details.percent,
            ),
            memory_details=HeartbeatMemoryDetails(
                total_mb=payload.memory_details.total_mb,
                available_mb=payload.memory_details.available_mb,
                used_mb=payload.memory_details.used_mb,
                percent=payload.memory_details.percent,
            ),
            internal_ip=payload.internal_ip,
            system_fingerprint=payload.system_fingerprint,
            fingerprint_details=SystemInfoFingerprintDetails.model_validate(
                payload.fingerprint_details
            ) if payload.fingerprint_details else None,
            is_cloud_operator=False,
            cloud_provider=None,
            local_storage_enabled=cap.local_storage_enabled,
            git_available=cap.git_available,
            ledger_enabled=cap.ledger_enabled,
        )

    def to_sse_payload(self, operator_id: str) -> "HeartbeatSSEPayload":
        """Convert to typed SSE payload for g8ed broadcasting to frontend."""
        return HeartbeatSSEPayload(
            operator_id=operator_id,
            timestamp=self.timestamp,
            heartbeat_type=self.heartbeat_type,
            status=OperatorStatus.ACTIVE,
            hostname=self.system_identity.hostname,
            os=self.system_identity.os,
            architecture=self.system_identity.architecture,
            cpu_percent=self.performance.cpu_percent,
            memory_percent=self.performance.memory_percent,
            disk_percent=self.performance.disk_percent,
            network_latency=self.performance.network_latency,
            memory_used_mb=self.performance.memory_used_mb,
            memory_total_mb=self.performance.memory_total_mb,
            disk_used_gb=self.performance.disk_used_gb,
            disk_total_gb=self.performance.disk_total_gb,
            public_ip=self.network.public_ip,
            interfaces=self.network.interfaces,
            uptime=self.uptime.uptime_display,
            uptime_seconds=self.uptime.uptime_seconds,
            operator_version=self.version_info.operator_version,
            version_status=self.version_info.status,
            local_storage_enabled=self.local_storage_enabled,
            git_available=self.git_available,
            ledger_enabled=self.ledger_enabled,
            os_details=self.os_details,
            user_details=self.user_details,
            disk_details=self.disk_details,
            memory_details=self.memory_details,
            environment=self.environment,
            cpu_count=self.system_identity.cpu_count,
        )


class PendingApproval(G8eBaseModel):
    """
    Typed state for a pending approval.

    Stored in the module-level approval registry. Waiters ``await``
    the embedded ``_event`` which is signalled by
    ``resolve()`` -- no polling required.
    """

    model_config = ConfigDict(arbitrary_types_allowed=True)

    approval_id: str = Field(description="Unique approval identifier")
    approval_type: ApprovalType = Field(description="Type of approval")

    # What is being approved
    command: str | None = Field(default=None, description="Command (for COMMAND approvals)")
    file_path: str | None = Field(default=None, description="File path (for FILE_EDIT approvals)")
    operation: FileOperation | None = Field(default=None, description="File operation (for FILE_EDIT approvals)")
    intent_name: str | None = Field(default=None, description="Intent name (for INTENT approvals)")

    # Request context
    requested_at: datetime = Field(description="When approval was requested")
    case_id: str | None = Field(default=None)
    investigation_id: str | None = Field(default=None)
    user_id: str | None = Field(default=None)
    operator_id: str | None = Field(default=None)
    operator_session_id: str | None = Field(default=None)

    # Response state
    response_received: bool = Field(default=False)
    approved: bool | None = Field(default=None)
    reason: str = Field(default="")
    responded_at: datetime | None = Field(default=None)
    feedback: bool = Field(default=False)

    # Signalling event -- excluded from serialization
    _event: asyncio.Event = PrivateAttr(default_factory=asyncio.Event)

    def resolve(
        self,
        *,
        approved: bool,
        reason: str = "",
        responded_at: datetime | None = None,
        operator_session_id: str | None = None,
        operator_id: str | None = None,
        feedback: bool = False,
    ) -> None:
        """Set the response fields and wake any waiter."""
        self.response_received = True
        self.approved = approved
        self.reason = reason
        self.responded_at = responded_at
        self.feedback = feedback
        if operator_session_id is not None:
            self.operator_session_id = operator_session_id
        if operator_id is not None:
            self.operator_id = operator_id
        self._event.set()

    async def wait(self) -> None:
        """Block until ``resolve()`` is called."""
        await self._event.wait()


class ApprovalResult(G8eBaseModel):
    """Return type for all approval request methods in ApprovalHandlerMixin."""

    approved: bool = Field(description="Whether the request was approved")
    reason: str | None = Field(default=None, description="Reason for the decision")
    approval_id: str | None = Field(default=None, description="Approval tracking ID")
    error: bool = Field(default=False, description="Whether an error occurred")
    error_type: ApprovalErrorType | None = Field(default=None, description="Error classification")
    feedback: bool = Field(default=False, description="Whether user provided feedback instead of approve/deny")
    operator_session_id: str | None = Field(default=None, description="Updated operator session ID from approval response")
    operator_id: str | None = Field(default=None, description="Updated operator ID from approval response")
    intent_name: str | None = Field(default=None, description="Normalized intent name (INTENT approvals only)")


class TargetSystem(G8eBaseModel):
    """Target system info for multi-operator batch approval display."""
    hostname: str = Field(description="Operator hostname")
    operator_id: str = Field(description="Operator identifier")
    operator_type: OperatorType = Field(description="Operator deployment type")
    operator_session_id: str = Field(default="", description="Operator session identifier")


class ApprovalRequestBase(G8eBaseModel):
    """Common context for all approval requests.

    Carries identity, routing, and timeout fields that every approval
    type needs. Type-specific subclasses add their own payload fields.
    """
    g8e_context: "G8eHttpContext" = Field(description="HTTP context with session/case/investigation identity")
    timeout_seconds: int = Field(description="Approval timeout in seconds")
    justification: str = Field(description="AI justification for the operation")
    execution_id: str = Field(description="Unique execution identifier for tracking")
    operator_session_id: str = Field(description="Operator session identifier")
    operator_id: str = Field(description="Operator identifier")
    batch_id: str | None = Field(
        default=None,
        description="Correlates multiple per-operator executions dispatched from a single approval.",
    )

    model_config = ConfigDict(arbitrary_types_allowed=True)


class CommandApprovalRequest(ApprovalRequestBase):
    """Typed request for command execution approval."""
    command: str = Field(description="Command pending approval")
    risk_analysis: CommandRiskAnalysis | None = Field(default=None)
    target_systems: list[TargetSystem] = Field(default_factory=list)
    task_id: str | None = Field(default=None, description="AI task identifier")


class FileEditApprovalRequest(ApprovalRequestBase):
    """Typed request for file edit approval."""
    file_path: str = Field(description="File path pending approval")
    operation: FileOperation = Field(description="File operation type")
    risk_analysis: FileOperationRiskAnalysis | None = Field(default=None)


class IntentApprovalRequest(ApprovalRequestBase):
    """Typed request for intent (IAM) permission approval."""
    intent_name: str | CloudIntent = Field(description="Primary intent being requested")
    all_intents: list[str | CloudIntent] = Field(description="All intents being requested (including dependencies)")
    operation_context: str | None = Field(default=None, description="Context for the operation")


class AgentContinueApprovalRequest(ApprovalRequestBase):
    """Typed request for agent turn-limit continuation approval.

    Emitted when the agent ReAct loop hits the tool-turn budget and asks the
    operator whether to reset the counter and continue, or stop the agent.
    No operator binding is required; operator_session_id/operator_id default
    to empty strings because the approval is agent-scoped, not tool-scoped.
    """
    turn_limit: int = Field(description="Tool-turn budget that triggered the approval")
    turns_completed: int = Field(description="Number of tool-use turns executed when the budget was reached")
    task_id: str | None = Field(default=None, description="AI task identifier for SSE routing")
    operator_session_id: str = Field(default="", description="Operator session identifier (unused for agent-scoped approvals)")
    operator_id: str = Field(default="", description="Operator identifier (unused for agent-scoped approvals)")


class BatchOperatorExecutionResult(G8eBaseModel):
    """Result of executing a command on a single operator within a batch execution."""
    hostname: str = Field(description="Operator hostname")
    operator_id: str = Field(description="Operator identifier")
    execution_id: str = Field(description="Unique execution ID for this operator's run")
    success: bool = Field(description="Whether the execution succeeded")
    result: CommandInternalResult | None = Field(default=None, description="Execution result payload")
    error: str | None = Field(default=None, description="Error message if failed")


class ApprovalContext(G8eBaseModel):
    """Shared routing context included in all approval event payloads."""
    approval_id: str = Field(description="Unique approval identifier")
    execution_id: str | None = Field(default=None, description="Operator execution ID at time of request")
    requested_at: datetime = Field(default_factory=now, description="When the approval was requested")
    timeout_seconds: int = Field(description="Approval timeout in seconds")
    justification: str = Field(description="AI justification for the operation")
    user_id: str | None = Field(default=None)
    task_id: str | None = Field(default=None, description="AI task identifier for routing the approval response")
    batch_id: str | None = Field(
        default=None,
        description="Batch correlation ID when the approval covers multiple operators.",
    )


class CommandApprovalEvent(ApprovalContext):
    """Event payload published to g8ed when command approval is requested."""
    command: str = Field(description="Command pending approval")
    risk_analysis: CommandRiskAnalysis | None = Field(default=None)
    target_systems: list[TargetSystem] = Field(default_factory=list)

    @property
    def is_batch_execution(self) -> bool:
        """True if targeting multiple systems."""
        return len(self.target_systems) > 1


class AgentContinueApprovalEvent(ApprovalContext):
    """Event payload published to g8ed when agent continuation approval is requested."""
    turn_limit: int = Field(description="Tool-turn budget that triggered the approval")
    turns_completed: int = Field(description="Number of tool-use turns executed when the budget was reached")


class FileEditApprovalEvent(ApprovalContext):
    """Event payload published to g8ed when file edit approval is requested."""
    file_path: str = Field(description="File path pending approval")
    operation: FileOperation = Field(description="File operation type")
    risk_analysis: FileOperationRiskAnalysis | None = Field(default=None)


class IntentApprovalEvent(ApprovalContext):
    """Event payload published to g8ed when AWS intent approval is requested."""
    intent_name: CloudIntent = Field(description="Normalized AWS intent name")
    all_intents: list[CloudIntent] = Field(description="All intents being requested")
    operation_context: str | None = Field(default=None, description="Context for the operation")
    intent_question: str = Field(description="Human-readable question for the user")
    operator_id: str | None = Field(default=None)


class TruncatedOutput(G8eBaseModel):
    """
    Output that has been truncated for storage efficiency.
    
    Keeps first and last N lines (default 20 each) for AI context.
    This allows the AI to see the beginning (headers, initial output)
    and end (final results, errors) of command output without storing
    potentially massive intermediate output.
    """
    first_lines: list[str] = Field(default_factory=list, description="First N lines of output")
    last_lines: list[str] = Field(default_factory=list, description="Last N lines of output")
    total_lines: int = Field(default=0, description="Total number of lines before truncation")
    was_truncated: bool = Field(default=False, description="Whether output was truncated")
    truncate_limit: int = Field(default=20, description="Lines kept from each end")

    @classmethod
    def from_output(cls, output: str, limit: int = 20) -> "TruncatedOutput":
        """
        Create TruncatedOutput from raw output string.
        
        Args:
            output: Raw output string (stdout/stderr)
            limit: Number of lines to keep from start and end
            
        Returns:
            TruncatedOutput with first/last lines preserved
        """
        if not output:
            return cls(
                first_lines=[],
                last_lines=[],
                total_lines=0,
                was_truncated=False,
                truncate_limit=limit
            )

        lines = output.splitlines()
        total = len(lines)

        # If output fits within 2*limit lines, no truncation needed
        if total <= limit * 2:
            return cls(
                first_lines=lines,
                last_lines=[],
                total_lines=total,
                was_truncated=False,
                truncate_limit=limit
            )

        # Truncate: keep first N and last N lines
        return cls(
            first_lines=lines[:limit],
            last_lines=lines[-limit:],
            total_lines=total,
            was_truncated=True,
            truncate_limit=limit
        )

    def format_for_prompt(self) -> str:
        if not self.first_lines and not self.last_lines:
            return "(no output)"

        if not self.was_truncated:
            return "\n".join(self.first_lines)

        result = "\n".join(self.first_lines)
        result += f"\n\n... [{self.total_lines - (len(self.first_lines) + len(self.last_lines))} lines truncated] ...\n\n"
        result += "\n".join(self.last_lines)
        return result


class HeartbeatSSEPayload(G8eBaseModel):
    """Typed SSE payload for g8ed broadcasting to frontend.

    Produced by OperatorHeartbeat.to_sse_payload(). Flattened structure
    optimized for UI display.
    """
    operator_id: str = Field(description="Operator ID")
    timestamp: datetime = Field(description="When heartbeat was received")
    heartbeat_type: HeartbeatType = Field(description="Heartbeat type")
    status: OperatorStatus = Field(description="Operator status at time of heartbeat")
    hostname: str | None = Field(default=None, description="System hostname")
    os: str | None = Field(default=None, description="Operating system name")
    architecture: str | None = Field(default=None, description="CPU architecture")
    cpu_percent: float | None = Field(default=None, description="CPU usage percentage")
    memory_percent: float | None = Field(default=None, description="Memory usage percentage")
    disk_percent: float | None = Field(default=None, description="Disk usage percentage")
    network_latency: float | None = Field(default=None, description="Network latency in ms")
    memory_used_mb: float | None = Field(default=None, description="Memory used in MB")
    memory_total_mb: float | None = Field(default=None, description="Total memory in MB")
    disk_used_gb: float | None = Field(default=None, description="Disk used in GB")
    disk_total_gb: float | None = Field(default=None, description="Total disk in GB")
    public_ip: str | None = Field(default=None, description="Public IP address")
    interfaces: list[str] | None = Field(default=None, description="Network interface names")
    uptime: str | None = Field(default=None, description="Human-readable uptime string")
    uptime_seconds: int | None = Field(default=None, description="Uptime in seconds")
    operator_version: str | None = Field(default=None, description="Operator binary version")
    version_status: VersionStability | None = Field(default=None, description="Version stability")
    local_storage_enabled: bool = Field(default=False, description="Local storage active")
    git_available: bool = Field(default=False, description="Git available on operator host")
    ledger_enabled: bool = Field(default=False, description="LFAA ledger mirroring active")
    os_details: HeartbeatOSDetails | None = Field(default=None, description="OS details")
    user_details: HeartbeatUserDetails | None = Field(default=None, description="User details")
    disk_details: HeartbeatDiskDetails | None = Field(default=None, description="Disk details")
    memory_details: HeartbeatMemoryDetails | None = Field(default=None, description="Memory details")
    environment: HeartbeatEnvironment | None = Field(default=None, description="Environment context")


# =============================================================================
# BROADCAST EVENT MODELS
# =============================================================================
# Typed payloads for _broadcast_command_event calls. Replaces raw dicts.
# All timestamp fields use datetime (not str) — serialized to ISO on the wire.
# =============================================================================

class CommandFailedBroadcastEvent(G8eBaseModel):
    """Broadcast payload for OPERATOR_COMMAND_FAILED."""
    command: str
    execution_id: str | None = None
    operator_session_id: str | None = None
    status: ExecutionStatus = ExecutionStatus.FAILED
    error: str | None = None
    stderr: str | None = None
    error_type: CommandErrorType | None = None
    denial_reason: str | None = None
    feedback_reason: str | None = None
    rule: str | None = None
    violations: list[str] | None = None
    approval_id: str | None = None
    timestamp: datetime = Field(default_factory=now)


class CommandExecutingBroadcastEvent(G8eBaseModel):
    """Broadcast payload for OPERATOR_COMMAND_EXECUTING."""
    command: str
    execution_id: str | None = None
    operator_session_id: str | None = None
    operator_id: str | None = None
    status: ExecutionStatus = ExecutionStatus.EXECUTING
    message: str | None = None
    approval_id: str | None = None
    batch_id: str | None = None
    timestamp: datetime = Field(default_factory=now)


class CommandStatusBroadcastEvent(G8eBaseModel):
    """Broadcast payload for OPERATOR_COMMAND_STATUS."""
    execution_id: str
    command: str | None = None
    operator_session_id: str | None = None
    status: ExecutionStatus = ExecutionStatus.EXECUTING
    elapsed_seconds: float = 0
    process_alive: bool = True
    status_msg: str | None = None
    timestamp: datetime = Field(default_factory=now)


class CommandResultBroadcastEvent(G8eBaseModel):
    """Broadcast payload for OPERATOR_COMMAND_COMPLETED / OPERATOR_COMMAND_FAILED (direct execution result)."""
    execution_id: str
    batch_id: str | None = None
    command: str | None = None
    status: ExecutionStatus
    output: str | None = None
    error: str | None = None
    stderr: str | None = None
    exit_code: int | None = None
    return_code: int | None = None
    execution_time_seconds: float | None = None
    operator_session_id: str | None = None
    operator_id: str | None = None
    hostname: str | None = None
    direct_execution: bool = False
    approval_id: str | None = None
    timestamp: datetime = Field(default_factory=now)


class CommandCancelledBroadcastEvent(G8eBaseModel):
    """Broadcast payload for OPERATOR_COMMAND_CANCELLED."""
    execution_id: str
    command: str | None = None
    operator_session_id: str | None = None
    status: ExecutionStatus = ExecutionStatus.CANCELLED
    error: str | None = None
    error_type: CommandErrorType | None = None
    timestamp: datetime = Field(default_factory=now)


class BatchCommandBroadcastEvent(G8eBaseModel):
    """Broadcast payload for batch OPERATOR_COMMAND_COMPLETED / OPERATOR_COMMAND_FAILED."""
    command: str
    execution_id: str
    status: ExecutionStatus
    batch_execution: bool = True
    output: str | None = None
    operators_used: int = 0
    successful_count: int = 0
    failed_count: int = 0
    approval_id: str | None = None
    timestamp: datetime = Field(default_factory=now)


class FileEditBroadcastEvent(G8eBaseModel):
    """Broadcast payload for OPERATOR_FILE_EDIT_COMPLETED / OPERATOR_FILE_EDIT_FAILED."""
    file_path: str
    operation: str | None = None
    execution_id: str | None = None
    operator_session_id: str | None = None
    status: ExecutionStatus
    error: str | None = None
    stderr: str | None = None
    error_type: CommandErrorType | None = None
    content: str | None = None
    backup_path: str | None = None
    timeout_seconds: int | None = None
    approval_id: str | None = None
    timestamp: datetime = Field(default_factory=now)


# =============================================================================
# COMMAND OPERATION RESULT MODELS
# =============================================================================
# Typed returns for cancel_command and send_command_to_operator.
# =============================================================================

class CancelCommandResult(G8eBaseModel):
    """Typed result from cancel_command."""
    execution_id: str
    status: ExecutionStatus
    message: str | None = None
    error: str | None = None


class DirectCommandResult(G8eBaseModel):
    """Typed result from send_command_to_operator (anchored terminal)."""
    execution_id: str | None = None
    status: ExecutionStatus
    message: str | None = None
    error: str | None = None


# =============================================================================
# VALIDATOR RESULT MODELS
# =============================================================================
# Pydantic models for validator return types.
# =============================================================================

class BindingValidationResult(G8eBaseModel):
    """Result model for operator binding validation."""
    valid: bool = Field(description="Whether the binding is valid")
    reason: str = Field(description="Explanation of validation result")
    operator_session_id: str | None = Field(default=None, description="Operator session ID")
    web_session_id: str | None = Field(default=None, description="Web session ID")
    operator_id: str = Field(description="Operator ID")
    operator_status: OperatorStatus | None = Field(default=None, description="Operator status")
    operator_bound_web_session_id: str | None = Field(default=None, description="Operator's bound web session ID")
    expected_operator_session_id: str | None = Field(default=None, description="Expected operator session ID")
    investigation_id: str | None = Field(default=None, description="Investigation ID for context")


class HealthCheckResultModel(G8eBaseModel):
    """Result model for operator health check.

    Replaces raw dict return in check_operator_health.
    """
    healthy: bool = Field(description="Whether operator is healthy")
    reason: str | None = Field(default=None, description="Explanation if unhealthy")
    operator_id: str = Field(description="Operator ID")
    status: OperatorStatus = Field(description="Operator status")
    last_heartbeat: datetime | None = Field(default=None, description="Last heartbeat timestamp")
    session_expires_at: datetime | None = Field(default=None, description="Session expiration timestamp")
    seconds_since_heartbeat: float | None = Field(default=None, description="Seconds since last heartbeat")
    heartbeat_stale: bool = Field(default=False, description="Whether heartbeat is stale")
    session_expired: bool = Field(default=False, description="Whether session is expired")


