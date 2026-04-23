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
G8eMessage models for g8es pub/sub messaging.

This module defines the standardized message format for all g8e component
communication via g8es pub/sub.
"""

from datetime import datetime
from typing import Union
from uuid import uuid4

from pydantic import Field, field_validator

from app.constants import ComponentName, EventType, ExecutionStatus, HeartbeatType

from .base import G8eBaseModel, UTCDatetime
from .tool_results import AuditEvent, AuditSessionMetadata, FileDiffEntry, FileHistoryEntry, FsListEntry
from .mcp import JSONRPCRequest

from app.utils.timestamp import now, parse_iso

# Import outbound payload types
from app.models.command_payloads import (
    CommandPayload,
    CommandCancelPayload,
    FileEditPayload,
    FsListPayload,
    FsReadPayload,
    FetchLogsPayload,
    FetchHistoryPayload,
    FetchFileHistoryPayload,
    FetchFileDiffPayload,
    RestoreFilePayload,
    DirectCommandAuditPayload,
)


class ExecutionResultsPayload(G8eBaseModel):
    """Typed payload for operator.command.completed / operator.command.failed."""
    execution_id: str = Field(..., description="Unique execution identifier")
    status: ExecutionStatus = Field(..., description="Final execution status")
    duration_seconds: float = Field(default=0.0, description="Wall-clock execution duration in seconds")
    stdout: str | None = Field(default=None, description="Standard output")
    stderr: str | None = Field(default=None, description="Standard error")
    stdout_size: int = Field(default=0, description="Size of stdout in bytes")
    stderr_size: int = Field(default=0, description="Size of stderr in bytes")
    stdout_hash: str | None = Field(default=None, description="SHA256 hash of raw stdout")
    stderr_hash: str | None = Field(default=None, description="SHA256 hash of raw stderr")
    stored_locally: bool = Field(default=False, description="True when full output is in operator local vault")
    return_code: int | None = Field(default=None, description="Process exit code")
    error_message: str | None = Field(default=None, description="Human-readable error description")
    error_type: str | None = Field(default=None, description="Machine-readable error classification")
    completed_at: UTCDatetime | None = Field(default=None, description="When the command completed (UTC)")

    @field_validator("completed_at", mode="before")
    @classmethod
    def _parse_completed_at(cls, v: object) -> datetime | None:
        if v is None:
            return None
        if isinstance(v, datetime):
            return v
        if isinstance(v, str):
            return parse_iso(v)
        raise ValueError(f"completed_at must be a datetime or ISO string, got {type(v).__name__}")


class ExecutionStatusPayload(G8eBaseModel):
    """Typed payload for operator.command.status."""
    execution_id: str = Field(..., description="Unique execution identifier")
    status: ExecutionStatus = Field(..., description="Current execution status")
    process_alive: bool = Field(default=True, description="True if the underlying process is still running")
    elapsed_seconds: float = Field(default=0.0, description="Elapsed time since execution started")
    new_output: str | None = Field(default=None, description="New stdout lines since last status update")
    new_stderr: str | None = Field(default=None, description="New stderr lines since last status update")
    message: str | None = Field(default=None, description="Human-readable status message")
    stored_locally: bool = Field(False)


class CancellationResultPayload(G8eBaseModel):
    """Typed payload for operator.command.cancelled."""
    execution_id: str = Field(..., description="Unique execution identifier of the cancelled command")
    status: ExecutionStatus = Field(ExecutionStatus.CANCELLED)
    error_message: str | None = Field(None)
    error_type: str | None = Field(None)


class FileEditResultPayload(G8eBaseModel):
    """Typed payload for operator.file.edit.completed / operator.file.edit.failed."""
    execution_id: str = Field(..., description="Unique execution identifier")
    operation: str = Field(..., description="File operation performed")
    file_path: str = Field(..., description="Absolute path of the target file")
    status: ExecutionStatus = Field(..., description="Operation status")
    duration_seconds: float = Field(0.0)
    stdout: str | None = Field(default=None, description="Standard output")
    stderr: str | None = Field(default=None, description="Standard error")
    stdout_size: int = Field(0)
    stderr_size: int = Field(0)
    stdout_hash: str | None = Field(None)
    stderr_hash: str | None = Field(None)
    stored_locally: bool = Field(False)
    return_code: int | None = Field(default=None, description="Process exit code")
    completed_at: UTCDatetime | None = Field(default=None, description="When the file operation completed (UTC)")
    content: str | None = Field(default=None, description="File content for read operations")
    bytes_written: int | None = Field(None)
    lines_changed: int | None = Field(None)
    backup_path: str | None = Field(None)
    error_message: str | None = Field(None)
    error_type: str | None = Field(None)

    @field_validator("completed_at", mode="before")
    @classmethod
    def _parse_completed_at(cls, v: object) -> datetime | None:
        if v is None:
            return None
        if isinstance(v, datetime):
            return v
        if isinstance(v, str):
            return parse_iso(v)
        raise ValueError(f"completed_at must be a datetime or ISO string, got {type(v).__name__}")


class FsListResultPayload(G8eBaseModel):
    """Typed payload for operator.fs.list.completed / operator.fs.list.failed."""
    execution_id: str = Field(..., description="Unique execution identifier")
    path: str | None = Field(default=None, description="Resolved absolute path that was listed")
    status: ExecutionStatus = Field(..., description="Operation status")
    total_count: int = Field(default=0, description="Total number of entries found before truncation")
    truncated: bool = Field(default=False, description="True if results were capped at max_entries")
    duration_seconds: float = Field(0.0)
    entries: list[FsListEntry] = Field(default_factory=list, description="Directory entries")
    stdout_size: int = Field(0)
    stderr_size: int = Field(0)
    stdout_hash: str | None = Field(None)
    stderr_hash: str | None = Field(None)
    stored_locally: bool = Field(False)
    error_message: str | None = Field(None)
    error_type: str | None = Field(None)


class FsReadResultPayload(G8eBaseModel):
    """Typed payload for operator.fs.read.completed / operator.fs.read.failed."""
    execution_id: str = Field(..., description="Unique execution identifier")
    path: str | None = Field(None)
    status: ExecutionStatus = Field(..., description="Operation status")
    content: str | None = Field(None)
    size: int = Field(0)
    truncated: bool = Field(False)
    duration_seconds: float = Field(0.0)
    stdout_size: int = Field(0)
    stderr_size: int = Field(0)
    stdout_hash: str | None = Field(None)
    stderr_hash: str | None = Field(None)
    stored_locally: bool = Field(False)
    error_message: str | None = Field(None)
    error_type: str | None = Field(None)


class FetchLogsResultPayload(G8eBaseModel):
    """Typed payload for operator.fetch.logs.completed / operator.fetch.logs.failed."""
    execution_id: str = Field(..., description="Execution identifier whose logs were fetched")
    command: str | None = Field(default=None, description="Original command string")
    exit_code: int | None = Field(default=None)
    duration_ms: int | None = Field(default=None, description="Original execution duration in milliseconds")
    stdout: str | None = Field(default=None, description="Stored stdout content")
    stderr: str | None = Field(default=None, description="Stored stderr content")
    stdout_size: int = Field(default=0)
    stderr_size: int = Field(default=0)
    timestamp: UTCDatetime | None = Field(default=None, description="When the original command was executed (UTC)")
    sentinel_mode: str | None = Field(default=None)
    error: str | None = Field(default=None, description="Error description if fetch failed")

    @field_validator("timestamp", mode="before")
    @classmethod
    def _parse_timestamp(cls, v: object) -> datetime | None:
        if v is None:
            return None
        if isinstance(v, datetime):
            return v
        if isinstance(v, str):
            return parse_iso(v)
        raise ValueError(f"timestamp must be a datetime or ISO string, got {type(v).__name__}")


class FetchHistorySuccessPayload(G8eBaseModel):
    """Typed payload for operator.fetch.history.completed."""
    execution_id: str = Field(..., description="Execution identifier for request-response correlation")
    operator_session_id: str = Field(..., description="Operator session ID that was queried")
    session: AuditSessionMetadata = Field(..., description="Session metadata")
    events: list[AuditEvent] = Field(default_factory=list, description="Audit events for the session")
    total: int = Field(0)
    limit: int = Field(50)
    offset: int = Field(0)


class FetchHistoryErrorPayload(G8eBaseModel):
    """Typed payload for operator.fetch.history.failed."""
    execution_id: str = Field(..., description="Execution identifier for request-response correlation")
    error: str = Field(..., description="Error description")


class FetchFileHistorySuccessPayload(G8eBaseModel):
    """Typed payload for operator.fetch.file.history.completed."""
    execution_id: str = Field(..., description="Execution identifier for request-response correlation")
    file_path: str = Field(..., description="Absolute path of the file queried")
    history: list[FileHistoryEntry] = Field(default_factory=list, description="Commit history entries")


class FetchFileHistoryErrorPayload(G8eBaseModel):
    """Typed payload for operator.fetch.file.history.failed."""
    execution_id: str = Field(..., description="Execution identifier for request-response correlation")
    error: str = Field(..., description="Error description")


class RestoreFileSuccessPayload(G8eBaseModel):
    """Typed payload for operator.restore.file.completed."""
    execution_id: str = Field(..., description="Execution identifier for request-response correlation")
    file_path: str = Field(..., description="Absolute path of the restored file")
    commit_hash: str = Field(..., description="Git commit hash the file was restored to")


class RestoreFileErrorPayload(G8eBaseModel):
    """Typed payload for operator.restore.file.failed."""
    execution_id: str = Field(..., description="Execution identifier for request-response correlation")
    error: str = Field(..., description="Error description")


class FetchFileDiffByIdSuccessPayload(G8eBaseModel):
    """Typed payload for operator.fetch.file.diff.completed when fetching by diff_id."""
    execution_id: str = Field(..., description="Execution identifier for request-response correlation")
    diff: FileDiffEntry = Field(..., description="Single file diff entry")


class FetchFileDiffBySessionSuccessPayload(G8eBaseModel):
    """Typed payload for operator.fetch.file.diff.completed when fetching by operator_session_id."""
    execution_id: str = Field(..., description="Execution identifier for request-response correlation")
    diffs: list[FileDiffEntry] = Field(default_factory=list, description="Multiple file diff entries")
    total: int = Field(0)
    operator_session_id: str = Field(..., description="Operator session ID")


class FetchFileDiffErrorPayload(G8eBaseModel):
    """Typed payload for operator.fetch.file.diff.failed."""
    execution_id: str = Field(..., description="Execution identifier for request-response correlation")
    error: str = Field(..., description="Error description")


class PortCheckResultPayload(G8eBaseModel):
    """Typed payload for operator.port.check.completed / operator.port.check.failed."""
    execution_id: str = Field(..., description="Execution identifier for request-response correlation")
    host: str | None = Field(default=None, description="Target host")
    port: int | None = Field(default=None, description="Target port")
    protocol: str | None = Field(default=None, description="Protocol checked")
    is_open: bool = Field(default=False, description="True if port is reachable")
    latency_ms: float | None = Field(default=None, description="Round-trip latency in milliseconds")
    error: str | None = Field(default=None, description="Error message on failure")


class ShutdownAckPayload(G8eBaseModel):
    """Typed payload for operator.shutdown.acknowledged."""
    status: str = Field("acknowledged")


class G8eoHeartbeatSystemIdentity(G8eBaseModel):
    """system_identity block from heartbeat.json."""
    hostname: str | None = None
    os: str | None = None
    architecture: str | None = None
    pwd: str | None = None
    current_user: str | None = None
    cpu_count: int | None = None
    memory_mb: int | None = None


class NetworkConnectivityStatus(G8eBaseModel):
    """Single network interface entry from connectivity_status in heartbeat.json."""
    name: str | None = None
    ip: str | None = None
    mtu: int | None = None


class G8eoHeartbeatNetworkInfo(G8eBaseModel):
    """network_info block from heartbeat.json."""
    public_ip: str | None = None
    internal_ip: str | None = None
    interfaces: list[str] | None = None
    connectivity_status: list[NetworkConnectivityStatus] | None = None


class G8eoHeartbeatVersionInfo(G8eBaseModel):
    """version_info block from heartbeat.json."""
    operator_version: str | None = None
    status: str | None = None


class G8eoHeartbeatUptimeInfo(G8eBaseModel):
    """uptime_info block from heartbeat.json."""
    uptime: str | None = None
    uptime_seconds: int | None = None


class G8eoHeartbeatPerformanceMetrics(G8eBaseModel):
    """performance_metrics block from heartbeat.json."""
    cpu_percent: float | None = None
    memory_percent: float | None = None
    disk_percent: float | None = None
    network_latency: float | None = None
    memory_used_mb: float | None = None
    memory_total_mb: float | None = None
    disk_used_gb: float | None = None
    disk_total_gb: float | None = None


class G8eoHeartbeatOSDetails(G8eBaseModel):
    """os_details block from heartbeat.json (ref: system_info.json)."""
    kernel: str | None = None
    distro: str | None = None
    version: str | None = None


class G8eoHeartbeatUserDetails(G8eBaseModel):
    """user_details block from heartbeat.json (ref: system_info.json)."""
    username: str | None = None
    uid: int | None = None
    gid: int | None = None
    home: str | None = None
    name: str | None = None
    shell: str | None = None


class G8eoHeartbeatDiskDetails(G8eBaseModel):
    """disk_details block from heartbeat.json (ref: system_info.json)."""
    total_gb: float | None = None
    used_gb: float | None = None
    free_gb: float | None = None
    percent: float | None = None


class G8eoHeartbeatMemoryDetails(G8eBaseModel):
    """memory_details block from heartbeat.json (ref: system_info.json)."""
    total_mb: int | None = None
    available_mb: int | None = None
    used_mb: int | None = None
    percent: float | None = None


class G8eoHeartbeatEnvironment(G8eBaseModel):
    """environment block from heartbeat.json (ref: system_info.json)."""
    pwd: str | None = None
    lang: str | None = None
    timezone: str | None = None
    term: str | None = None
    is_container: bool | None = None
    container_runtime: str | None = None
    container_signals: list[str] | None = None
    init_system: str | None = None


class G8eoHeartbeatFingerprintDetails(G8eBaseModel):
    """fingerprint_details block from heartbeat.json."""
    os: str | None = None
    architecture: str | None = None
    cpu_count: int | None = None
    machine_id: str | None = None


class G8eoHeartbeatCapabilityFlags(G8eBaseModel):
    """capability_flags block from heartbeat.json."""
    local_storage_enabled: bool = False
    git_available: bool = False
    ledger_enabled: bool = False


class G8eoHeartbeatPayload(G8eBaseModel):
    """Typed wire model for the g8eo heartbeat pub/sub payload.

    Canonical shape defined in shared/models/wire/heartbeat.json.
    This is the boundary model — validated once when the raw pub/sub
    message arrives in command_pubsub.py, then passed typed throughout.
    """
    event_type: EventType | None = None
    timestamp: UTCDatetime | None = None
    heartbeat_type: HeartbeatType = HeartbeatType.AUTOMATIC
    operator_id: str | None = None
    operator_session_id: str | None = None
    case_id: str | None = None
    investigation_id: str | None = None
    user_id: str | None = None
    system_fingerprint: str | None = None

    system_identity: G8eoHeartbeatSystemIdentity = Field(default_factory=G8eoHeartbeatSystemIdentity)
    network_info: G8eoHeartbeatNetworkInfo = Field(default_factory=G8eoHeartbeatNetworkInfo)
    version_info: G8eoHeartbeatVersionInfo = Field(default_factory=G8eoHeartbeatVersionInfo)
    uptime_info: G8eoHeartbeatUptimeInfo = Field(default_factory=G8eoHeartbeatUptimeInfo)
    performance_metrics: G8eoHeartbeatPerformanceMetrics = Field(default_factory=G8eoHeartbeatPerformanceMetrics)
    os_details: G8eoHeartbeatOSDetails = Field(default_factory=G8eoHeartbeatOSDetails)
    user_details: G8eoHeartbeatUserDetails = Field(default_factory=G8eoHeartbeatUserDetails)
    disk_details: G8eoHeartbeatDiskDetails = Field(default_factory=G8eoHeartbeatDiskDetails)
    memory_details: G8eoHeartbeatMemoryDetails = Field(default_factory=G8eoHeartbeatMemoryDetails)
    environment: G8eoHeartbeatEnvironment = Field(default_factory=G8eoHeartbeatEnvironment)
    fingerprint_details: G8eoHeartbeatFingerprintDetails | None = None
    capability_flags: G8eoHeartbeatCapabilityFlags = Field(default_factory=G8eoHeartbeatCapabilityFlags)
    api_key: str | None = None

    @field_validator("timestamp", mode="before")
    @classmethod
    def _parse_timestamp(cls, v: object) -> datetime | None:
        if v is None:
            return None
        if isinstance(v, datetime):
            return v
        if isinstance(v, str):
            return parse_iso(v)
        raise ValueError(f"timestamp must be a datetime or ISO string, got {type(v).__name__}")


G8eoResultPayload = Union[
    ExecutionResultsPayload,
    ExecutionStatusPayload,
    CancellationResultPayload,
    FileEditResultPayload,
    FsListResultPayload,
    FsReadResultPayload,
    FetchLogsResultPayload,
    FetchHistorySuccessPayload,
    FetchHistoryErrorPayload,
    FetchFileHistorySuccessPayload,
    FetchFileHistoryErrorPayload,
    RestoreFileSuccessPayload,
    RestoreFileErrorPayload,
    FetchFileDiffByIdSuccessPayload,
    FetchFileDiffBySessionSuccessPayload,
    FetchFileDiffErrorPayload,
    PortCheckResultPayload,
    ShutdownAckPayload,
]


# Union type for all outbound payloads from g8ee to g8eo
# Uses discriminator field 'payload_type' for type-safe parsing
G8eOutboundPayload = Union[
    CommandPayload,
    CommandCancelPayload,
    FileEditPayload,
    FsListPayload,
    FsReadPayload,
    FetchLogsPayload,
    FetchHistoryPayload,
    FetchFileHistoryPayload,
    FetchFileDiffPayload,
    RestoreFilePayload,
    DirectCommandAuditPayload,
    JSONRPCRequest,
]


class G8eoResultEnvelope(G8eBaseModel):
    """Inbound-only envelope parsed from the g8eo results pub/sub channel.

    Carries routing fields and a typed payload parsed at the wire boundary.
    This is a parse-only boundary object — not a persisted document.
    """

    id: str = Field(default_factory=lambda: str(uuid4()), description="Message ID echoed from the outbound command")
    event_type: EventType = Field(..., description="Event type from g8eo")
    operator_id: str = Field(..., description="Operator ID from channel routing")
    operator_session_id: str = Field(..., description="Operator session ID from channel routing")
    case_id: str | None = Field(default=None, description="Case ID propagated from the original command")
    investigation_id: str | None = Field(default=None, description="Investigation ID propagated from the original command")
    task_id: str | None = Field(default=None, description="Task ID propagated from the original command")
    payload: G8eoResultPayload | None = Field(default=None, description="Typed payload — always a G8eoResultPayload subclass post-parse")


class G8eMessage(G8eBaseModel):
    """Standardized message for g8es pub/sub communication between g8e components.
    
    The payload field uses a Union discriminator pattern for type-safe parsing.
    Consumers can parse inbound messages without knowing the concrete type in advance.
    """

    id: str = Field(..., description="Unique message identifier")
    timestamp: UTCDatetime = Field(default_factory=now, description="When the message was created (UTC)")

    source_component: ComponentName = Field(..., description="Source component that published this message")
    instance_id: str | None = Field(default=None, description="Optional instance identifier for the source component")
    event_type: EventType = Field(..., description="Event type identifier")
    case_id: str = Field(..., description="Case ID associated with this message")
    task_id: str = Field(..., description="Task ID associated with this message")
    investigation_id: str = Field(..., description="Investigation ID associated with this message")
    web_session_id: str = Field(..., description="Web session ID for targeted delivery to specific browser tabs")
    operator_session_id: str | None = Field(default=None, description="Operator session ID for g8eo Operator identification")
    operator_id: str | None = Field(default=None, description="Operator ID for g8eo Operator identification")
    api_key: str | None = Field(default=None, description="Operator API key carried on pub/sub messages for identity continuity")
    payload: G8eOutboundPayload | None = Field(
        default=None, 
        discriminator="payload_type",
        description="Typed payload for this message — uses discriminator for type-safe parsing"
    )
