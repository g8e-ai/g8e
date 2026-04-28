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
from typing import Any, Literal

from pydantic import ConfigDict, Field, PrivateAttr, ValidationInfo, field_validator, model_validator

from app.models.http_context import G8eHttpContext

from app.constants import (
    ApprovalErrorType,
    ApprovalType,
    AttachmentType,
    CloudIntent,
    CloudSubtype,
    CommandErrorType,
    ComponentName,
    ExecutionStatus,
    FileOperation,
    HeartbeatType,
    OperatorStatus,
    OperatorType,
    OperatorHistoryEventType,
    VersionStability,
)
from app.models.pubsub_messages import G8eoHeartbeatPayload
from app.models.tool_results import (
    CommandInternalResult,
    CommandRiskAnalysis,
    FileOperationRiskAnalysis,
)
from app.utils.timestamp import now

from .base import G8eBaseModel, G8eIdentifiableModel, UTCDatetime

logger = logging.getLogger(__name__)


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
    timestamp: UTCDatetime = Field(default_factory=now, description="When the command completed")
    investigation_id: str | None = Field(default=None, description="Associated investigation ID")
    case_id: str | None = Field(default=None, description="Associated case ID")
    operator_session_id: str | None = Field(default=None, description="Associated operator session ID")


class OperatorHistoryEntry(G8eBaseModel):
    """Single entry in operator history trail."""
    timestamp: UTCDatetime = Field(default_factory=now, description="When this event occurred")
    event_type: OperatorHistoryEventType = Field(..., description="Type of event (canonical values from status.json)")
    summary: str = Field(..., description="Brief summary of what happened")
    actor: ComponentName = Field(default=ComponentName.G8EE, description="Who performed this action")
    details: dict[str, object] = Field(default_factory=dict, description="Detailed event information")
    prev_hash: str = Field(..., description="Hash of previous entry in the chain (hex SHA256, 64 chars)")
    entry_hash: str | None = Field(default=None, description="Hash of this entry (hex SHA256, 64 chars)")

    @model_validator(mode="after")
    def _seal_entry_hash(self) -> "OperatorHistoryEntry":
        """Auto-compute entry_hash if not provided."""
        if self.entry_hash is None:
            from app.utils.ledger_hash import compute_entry_hash
            payload = self.model_dump(mode="json", exclude={"entry_hash"})
            object.__setattr__(self, "entry_hash", compute_entry_hash(payload, self.prev_hash))
        return self

    @field_validator("entry_hash", mode="after")
    @classmethod
    def validate_entry_hash(cls, v):
        if v is None:
            raise ValueError("entry_hash must be computed and set before use")
        if len(v) != 64:
            raise ValueError("entry_hash must be 64 characters (hex SHA256)")
        return v


class OperatorDocument(G8eIdentifiableModel):
    """g8ee read-side projection of the g8ed OperatorDocument.

    Maps to operator_status_info in shared/models/operator_document.json.
    Populated from g8es KV cache keyed by KVKey.doc(Collections.OPERATORS, id) or
    GET /api/internal/operators/:id/status.
    g8ed is the authority — g8ee only reads this document.
    """

    model_config = ConfigDict(extra="ignore")

    user_id: str = Field(description="User ID who owns this operator (always set by g8ed)")
    first_deployed: UTCDatetime | None = Field(default=None, description="When the operator was first deployed")
    name: str | None = Field(default=None, description="Human-readable operator name")
    organization_id: str | None = Field(default=None, description="Organization ID")
    status: OperatorStatus = Field(default=OperatorStatus.AVAILABLE, description="Current Operator status")
    bound_web_session_id: str | None = Field(default=None, description="Bound web session ID")
    operator_session_id: str | None = Field(default=None, description="Current Operator session ID")
    claimed_at: UTCDatetime | None = Field(default=None, description="When the slot was claimed (set at claim time, not heartbeat time)")
    last_heartbeat: UTCDatetime | None = Field(default=None, description="Last heartbeat timestamp (set only on actual heartbeat ingestion)")
    terminated_at: UTCDatetime | None = Field(default=None, description="When the operator was terminated")
    latest_heartbeat_snapshot: HeartbeatSnapshot | None = Field(default=None, description="Latest heartbeat metrics")
    investigation_id: str | None = Field(default=None, description="Current investigation ID")
    case_id: str | None = Field(default=None, description="Current case ID")
    api_key: str | None = Field(default=None, description="Operator API key (authority: g8ee)")
    is_active: bool = Field(default=False, description="Whether Operator is in active status")
    operator_type: OperatorType = Field(default=OperatorType.SYSTEM, description="Operator deployment type")
    granted_intents: list[str] | None = Field(default=None, description="Granted intent permissions (cloud operators)")
    cloud_subtype: CloudSubtype | None = Field(default=None, description="Cloud provider subtype")
    current_hostname: str | None = Field(default=None, description="Denormalized hostname from latest_heartbeat_snapshot for quick access")
    session_token: str | None = Field(default=None, description="Active session token for session-based auth validation")
    session_expires_at: UTCDatetime | None = Field(default=None, description="Session expiration timestamp")
    history_trail: list[OperatorHistoryEntry] = Field(
        default_factory=list,
        description="Operator lifecycle audit trail (append-only). Authority: g8ee."
    )

    @property
    def hostname(self) -> str | None:
        """Get hostname from current_hostname for backward compatibility."""
        return self.current_hostname

    @field_validator("current_hostname", mode="before")
    @classmethod
    def sync_current_hostname(cls, v: object, info: ValidationInfo) -> str | None:
        """Ensure current_hostname stays in sync with latest_heartbeat_snapshot.system_identity.hostname."""
        if v is not None:
            return v
        if info.data.get("latest_heartbeat_snapshot") and isinstance(info.data["latest_heartbeat_snapshot"], HeartbeatSnapshot):
            return info.data["latest_heartbeat_snapshot"].system_identity.hostname
        return None

    @field_validator("latest_heartbeat_snapshot", mode="before")
    @classmethod
    def coerce_heartbeat_snapshot(cls, v: object) -> object:
        if isinstance(v, dict):
            try:
                return HeartbeatSnapshot.model_validate(v)
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
    internal_ip: str | None = Field(default=None, description="Internal/private IP address")
    interfaces: list[str] | None = Field(default=None, description="List of network interface names")
    connectivity_status: list[HeartbeatNetworkInterface] | None = Field(default=None, description="Active network interfaces with IP and MTU")


class HeartbeatUptimeInfo(G8eBaseModel):
    """Uptime information from Operator heartbeat."""
    uptime_display: str | None = Field(default=None, description="Human-readable uptime string")
    uptime_seconds: int | None = Field(default=None, description="Uptime in seconds")


class HeartbeatOSDetails(G8eBaseModel):
    """Detailed operating system information."""
    kernel: str | None = Field(default=None, description="Kernel version")
    distro: str | None = Field(default=None, description="Distribution name")
    version: str | None = Field(default=None, description="OS version")


class HeartbeatUserDetails(G8eBaseModel):
    """Details about the current OS user."""
    username: str | None = Field(default=None, description="Username")
    uid: int | None = Field(default=None, description="User ID")
    gid: int | None = Field(default=None, description="Group ID")
    home: str | None = Field(default=None, description="Home directory")
    name: str | None = Field(default=None, description="Full name")
    shell: str | None = Field(default=None, description="Default shell")


class HeartbeatDiskDetails(G8eBaseModel):
    """Root filesystem disk usage."""
    total_gb: float | None = Field(default=None, description="Total disk space in GB")
    used_gb: float | None = Field(default=None, description="Used disk space in GB")
    free_gb: float | None = Field(default=None, description="Free disk space in GB")
    percent: float | None = Field(default=None, description="Disk usage percentage")


class HeartbeatMemoryDetails(G8eBaseModel):
    """Detailed memory usage breakdown."""
    total_mb: int | None = Field(default=None, description="Total memory in MB")
    available_mb: int | None = Field(default=None, description="Available memory in MB")
    used_mb: int | None = Field(default=None, description="Used memory in MB")
    percent: float | None = Field(default=None, description="Memory usage percentage")


class HeartbeatEnvironment(G8eBaseModel):
    """Environment and container context."""
    pwd: str | None = Field(default=None, description="Current working directory")
    lang: str | None = Field(default=None, description="Language setting")
    timezone: str | None = Field(default=None, description="Timezone")
    term: str | None = Field(default=None, description="Terminal type")
    is_container: bool | None = Field(default=None, description="Running in container")
    container_runtime: str | None = Field(default=None, description="Container runtime")
    container_signals: list[str] | None = Field(default=None, description="Container signals")
    init_system: str | None = Field(default=None, description="Init system")


class HeartbeatFingerprintDetails(G8eBaseModel):
    """Sub-fields used to compute system_fingerprint."""
    os: str | None = Field(default=None, description="Operating system")
    architecture: str | None = Field(default=None, description="CPU architecture")
    cpu_count: int | None = Field(default=None, description="CPU core count")
    machine_id: str | None = Field(default=None, description="Machine ID")


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



class HeartbeatSnapshot(G8eBaseModel):
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
    timestamp: UTCDatetime = Field(default_factory=now, description="When heartbeat was received")
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
    system_fingerprint: str | None = Field(default=None, description="SHA256 fingerprint of stable host attributes")
    fingerprint_details: HeartbeatFingerprintDetails | None = Field(default=None, description="Sub-fields used to compute system_fingerprint")

    # Cloud operator flags
    is_cloud_operator: bool = Field(default=False, description="True when this is a cloud-hosted operator")
    cloud_provider: str | None = Field(default=None, description="Cloud provider identifier")

    # Operator capability flags
    local_storage_enabled: bool = Field(default=False, description="True when local (Project Chronos) storage is active")
    git_available: bool = Field(default=False, description="True when git is available on the operator host")
    ledger_enabled: bool = Field(default=False, description="True when LFAA ledger mirroring is active")

    @classmethod
    def from_wire(cls, payload: G8eoHeartbeatPayload) -> "HeartbeatSnapshot":
        """Create HeartbeatSnapshot from the typed g8eo wire payload.

        Canonical wire shape: shared/models/wire/heartbeat.json.
        Validation happens once at the pub/sub boundary in heartbeat_service.py
        before this is called.
        """
        wire_dict = payload.model_dump(mode="json", exclude={"event_type", "operator_id", "operator_session_id", "case_id", "investigation_id", "user_id", "api_key"})
        
        wire_dict["timestamp"] = now()
        wire_dict["heartbeat_type"] = _coerce_heartbeat_type(payload.heartbeat_type)
        
        wire_dict["performance"] = wire_dict.pop("performance_metrics")
        wire_dict["network"] = wire_dict.pop("network_info")
        uptime_info = wire_dict.pop("uptime_info")
        wire_dict["uptime"] = {
            "uptime_display": uptime_info.get("uptime"),
            "uptime_seconds": uptime_info.get("uptime_seconds"),
        }
        
        if wire_dict.get("network") and wire_dict["network"].get("connectivity_status"):
            wire_dict["network"]["connectivity_status"] = [
                HeartbeatNetworkInterface(name=s["name"], ip=s["ip"], mtu=s["mtu"])
                for s in wire_dict["network"]["connectivity_status"]
            ]
        
        wire_dict["is_cloud_operator"] = False
        wire_dict["cloud_provider"] = None
        
        cap = payload.capability_flags
        wire_dict["local_storage_enabled"] = cap.local_storage_enabled
        wire_dict["git_available"] = cap.git_available
        wire_dict["ledger_enabled"] = cap.ledger_enabled
        
        wire_dict.pop("capability_flags", None)
        
        return cls.model_validate(wire_dict)



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
    requested_at: UTCDatetime = Field(description="When approval was requested")
    case_id: str | None = Field(default=None)
    investigation_id: str | None = Field(default=None)
    user_id: str | None = Field(default=None)
    operator_id: str | None = Field(default=None)
    operator_session_id: str | None = Field(default=None)

    # Response state
    response_received: bool = Field(default=False)
    approved: bool | None = Field(default=None)
    reason: str = Field(default="")
    responded_at: UTCDatetime | None = Field(default=None)
    feedback: bool = Field(default=False)

    # Signalling event -- excluded from serialization
    _event: asyncio.Event = PrivateAttr(default_factory=asyncio.Event)

    def resolve(
        self,
        *,
        approved: bool,
        reason: str = "",
        responded_at: UTCDatetime | None = None,
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
    g8e_context: G8eHttpContext = Field(description="HTTP context with session/case/investigation identity")
    timeout_seconds: int = Field(description="Approval timeout in seconds")
    justification: str = Field(description="AI justification for the operation")
    execution_id: str = Field(description="Unique execution identifier for tracking")
    operator_session_id: str = Field(description="Operator session identifier")
    operator_id: str = Field(description="Operator identifier")
    batch_id: str | None = Field(
        default=None,
        description="Correlates multiple per-operator executions dispatched from a single approval.",
    )
    correlation_id: str | None = Field(
        default=None,
        description="Tribunal correlation ID linking approval to the originating Tribunal session",
    )

    model_config = ConfigDict(arbitrary_types_allowed=True)


class CommandApprovalRequest(ApprovalRequestBase):
    """Typed request for command execution approval."""
    command: str = Field(description="Command pending approval")
    risk_analysis: CommandRiskAnalysis | None = Field(default=None)
    target_systems: list[TargetSystem] = Field(default_factory=list)
    task_id: str | None = Field(default=None, description="AI task identifier")


class StreamApprovalRequest(ApprovalRequestBase):
    """Typed request for operator stream approval."""
    kind: Literal["stream"] = Field(default="stream")
    hosts: list[str] = Field(description="Hosts to stream the operator to")
    arch: str = Field(description="Binary architecture")
    endpoint: str = Field(description="g8ed endpoint for handshake")
    device_token: str = Field(description="dlk_ token (UI-redacted in event)")
    concurrency: int = Field(default=5)
    timeout: int = Field(default=300)

    @property
    def preview_command(self) -> str:
        """Derived command for display in the UI."""
        return f"g8e-operator stream --hosts {','.join(self.hosts)} --concurrency {self.concurrency}"


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
    requested_at: UTCDatetime = Field(default_factory=now, description="When the approval was requested")
    timeout_seconds: int = Field(description="Approval timeout in seconds")
    justification: str = Field(description="AI justification for the operation")
    user_id: str | None = Field(default=None)
    task_id: str | None = Field(default=None, description="AI task identifier for routing the approval response")
    batch_id: str | None = Field(
        default=None,
        description="Batch correlation ID when the approval covers multiple operators.",
    )
    correlation_id: str | None = Field(
        default=None,
        description="Tribunal correlation ID linking approval to the originating Tribunal session",
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


class StreamApprovalEvent(ApprovalContext):
    """Event payload published to g8ed when operator stream approval is requested."""
    kind: Literal["stream"] = Field(default="stream")
    hosts: list[str] = Field(description="Hosts to stream the operator to")
    concurrency: int = Field(description="Maximum parallel hosts")
    timeout: int = Field(description="Timeout per host in seconds")
    preview_command: str = Field(description="The command that will be executed")


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


class HeartbeatSSEEnvelope(G8eBaseModel):
    """Wire envelope for OPERATOR_HEARTBEAT_RECEIVED SSE events.

    Canonical shape: shared/models/wire/heartbeat_sse.json#envelope. Authorship
    boundary: g8ee owns `operator_id` and `status` (the authoritative value from
    OperatorDocument); `metrics` carries the g8eo-authored HeartbeatSnapshot
    snapshot verbatim (shared/models/wire/heartbeat.json#operator_heartbeat) —
    the same instance persisted as `latest_heartbeat_snapshot` on the operator
    document. There is no flat projection: wire, persistence, and browser
    all see the identical nested shape. Callers must never mutate fields
    after construction.
    """

    operator_id: str = Field(description="Operator ID")
    status: OperatorStatus = Field(description="Authoritative operator status from OperatorDocument")
    metrics: "HeartbeatSnapshot" = Field(description="Full HeartbeatSnapshot snapshot (nested)")

    @classmethod
    def from_heartbeat(
        cls,
        operator_id: str,
        status: OperatorStatus,
        heartbeat: "HeartbeatSnapshot",
    ) -> "HeartbeatSSEEnvelope":
        """Build the envelope from an authoritative operator_id+status plus the
        full HeartbeatSnapshot. `metrics` holds the heartbeat instance as-is."""
        return cls(
            operator_id=operator_id,
            status=status,
            metrics=heartbeat,
        )


class OperatorStatusUpdatedPayload(G8eBaseModel):
    """Wire payload for OPERATOR_STATUS_UPDATED_* SSE events.

    Mirrors components/g8ed/models/sse_models.js OperatorStatusUpdatedData.
    Emitted by HeartbeatStaleMonitorService when an operator transitions
    between BOUND/STALE or ACTIVE/OFFLINE due to heartbeat freshness.
    """

    operator_id: str = Field(description="Operator ID")
    status: OperatorStatus = Field(description="New operator status")
    hostname: str | None = Field(default=None, description="Hostname from latest_heartbeat_snapshot.system_identity")
    system_fingerprint: str | None = Field(default=None, description="SHA256 fingerprint of stable host attributes")
    timestamp: UTCDatetime | None = Field(default=None, description="Transition timestamp")


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
    timestamp: UTCDatetime = Field(default_factory=now)


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
    per_operator_execution_ids: list[str] = Field(default_factory=list)
    timestamp: UTCDatetime = Field(default_factory=now)


class CommandStatusBroadcastEvent(G8eBaseModel):
    """Broadcast payload for OPERATOR_COMMAND_STATUS."""
    execution_id: str
    command: str | None = None
    operator_session_id: str | None = None
    status: ExecutionStatus = ExecutionStatus.EXECUTING
    elapsed_seconds: float = 0
    process_alive: bool = True
    status_msg: str | None = None
    timestamp: UTCDatetime = Field(default_factory=now)


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
    timestamp: UTCDatetime = Field(default_factory=now)


class CommandCancelledBroadcastEvent(G8eBaseModel):
    """Broadcast payload for OPERATOR_COMMAND_CANCELLED."""
    execution_id: str
    command: str | None = None
    operator_session_id: str | None = None
    status: ExecutionStatus = ExecutionStatus.CANCELLED
    error: str | None = None
    error_type: CommandErrorType | None = None
    timestamp: UTCDatetime = Field(default_factory=now)


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
    timestamp: UTCDatetime = Field(default_factory=now)


class FileEditBroadcastEvent(G8eBaseModel):
    """Broadcast payload for OPERATOR_FILE_EDIT_COMPLETED / OPERATOR_FILE_EDIT_FAILED."""
    command: str | None = None
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
    timestamp: UTCDatetime = Field(default_factory=now)


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
    last_heartbeat: UTCDatetime | None = Field(default=None, description="Last heartbeat timestamp")
    session_expires_at: UTCDatetime | None = Field(default=None, description="Session expiration timestamp")
    seconds_since_heartbeat: float | None = Field(default=None, description="Seconds since last heartbeat")
    heartbeat_stale: bool = Field(default=False, description="Whether heartbeat is stale")
    session_expired: bool = Field(default=False, description="Whether session is expired")


