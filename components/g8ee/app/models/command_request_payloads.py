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
Command request payload models for g8eo pub/sub wire protocol.

These are the inbound payload shapes published by g8ee to the
cmd:{operator_id}:{operator_session_id} pub/sub channel. g8eo deserializes
them using Protobuf. Field names and types mirror the canonical Go structs in
components/g8eo/models/commands.go and the Protobuf definitions in
shared/proto/operator.proto.
"""

from typing import Literal, Union

from pydantic import Field

from app.constants import FileOperation
from app.models.base import G8eBaseModel
from app.proto import operator_pb2

__all__ = [
    "CheckPortRequestPayload",
    "CommandCancelRequestPayload",
    "CommandRequestPayload",
    "DirectCommandAuditRequestPayload",
    "FetchFileDiffRequestPayload",
    "FetchFileHistoryRequestPayload",
    "FetchHistoryRequestPayload",
    "FetchLogsRequestPayload",
    "FileEditRequestPayload",
    "FsGrepRequestPayload",
    "FsListRequestPayload",
    "FsReadRequestPayload",
    "G8eCommandPayload",
    "HeartbeatRequestPayload",
    "RestoreFileRequestPayload",
    "TargetedOperatorBase",
]


class TargetedOperatorBase(G8eBaseModel):
    """Base class for tool args that can be routed to specific operators."""
    target_operators: list[str] = Field(
        ...,
        min_length=1,
        description=(
            "List of operators to execute on. Each entry can be: operator_id, hostname, or index ('0', '1'). "
            "Pass ['all'] to execute on all bound operators. Required."
        ),
    )


class CommandRequestPayload(G8eBaseModel):
    """Payload for EventType.OPERATOR_COMMAND_REQUESTED."""
    payload_type: Literal["command"] = Field(default="command", description="Payload type discriminator")
    command: str = Field(..., description="Shell command string to execute")
    execution_id: str = Field(..., description="Unique execution identifier")
    justification: str | None = Field(default=None, description="Justification for running this command")
    sentinel_mode: str | None = Field(default=None, description="Vault scrubbing mode for output storage")
    timeout_seconds: int | None = Field(default=None, description="Execution timeout override in seconds")

    def to_protobuf(self) -> operator_pb2.CommandRequested:
        """Convert to protobuf CommandRequested message."""
        proto = operator_pb2.CommandRequested()
        proto.command = self.command
        proto.execution_id = self.execution_id
        if self.justification:
            proto.justification = self.justification
        if self.sentinel_mode:
            proto.sentinel_mode = self.sentinel_mode
        if self.timeout_seconds:
            proto.timeout_seconds = self.timeout_seconds
        return proto


class CommandCancelRequestPayload(G8eBaseModel):
    """Payload for EventType.OPERATOR_COMMAND_CANCEL_REQUESTED."""
    payload_type: Literal["command_cancel"] = Field(default="command_cancel", description="Payload type discriminator")
    execution_id: str = Field(..., description="execution_id of the running command to cancel")

    def to_protobuf(self) -> operator_pb2.CommandCancelRequested:
        """Convert to protobuf CommandCancelRequested message."""
        proto = operator_pb2.CommandCancelRequested()
        proto.execution_id = self.execution_id
        return proto


class FileEditRequestPayload(TargetedOperatorBase):
    """Payload for EventType.OPERATOR_FILE_EDIT_REQUESTED and typed args for _execute_file_edit."""
    payload_type: Literal["file_edit"] = Field(default="file_edit", description="Payload type discriminator")
    file_path: str = Field(..., description="Absolute path to the target file on the operator host")
    operation: FileOperation = Field(..., description="File operation type")
    justification: str | None = Field(default=None, description="Justification for this file operation")
    execution_id: str = Field(..., description="Unique execution identifier")
    sentinel_mode: str | None = Field(default=None, description="Vault scrubbing mode")
    content: str | None = Field(default=None, description="Full file content (write operation)")
    old_content: str | None = Field(default=None, description="Exact content to find and replace (replace operation)")
    new_content: str | None = Field(default=None, description="Replacement content (replace operation)")
    insert_content: str | None = Field(default=None, description="Content to insert (insert operation)")
    insert_position: int | None = Field(default=None, description="1-indexed line number for insertion point")
    start_line: int | None = Field(default=None, description="1-indexed start line (delete or read range)")
    end_line: int | None = Field(default=None, description="1-indexed end line inclusive (delete or read range)")
    patch_content: str | None = Field(default=None, description="Unified diff format patch content (patch operation)")
    create_backup: bool = Field(default=False, description="Create a backup before modifying")
    create_if_missing: bool = Field(default=False, description="Create the file if it does not exist (write operation)")

    def to_protobuf(self) -> operator_pb2.FileEditRequested:
        """Convert to protobuf FileEditRequested message."""
        proto = operator_pb2.FileEditRequested()
        proto.file_path = self.file_path
        proto.operation = self.operation
        proto.execution_id = self.execution_id
        if self.justification:
            proto.justification = self.justification
        if self.content:
            proto.content = self.content
        if self.old_content:
            proto.old_content = self.old_content
        if self.new_content:
            proto.new_content = self.new_content
        if self.insert_content:
            proto.insert_content = self.insert_content
        if self.insert_position is not None:
            proto.insert_position = self.insert_position
        if self.start_line is not None:
            proto.start_line = self.start_line
        if self.end_line is not None:
            proto.end_line = self.end_line
        if self.patch_content:
            proto.patch_content = self.patch_content
        proto.create_backup = self.create_backup
        proto.create_if_missing = self.create_if_missing
        return proto


class FsListRequestPayload(TargetedOperatorBase):
    """Payload for EventType.OPERATOR_FILESYSTEM_LIST_REQUESTED."""
    payload_type: Literal["fs_list"] = Field(default="fs_list", description="Payload type discriminator")
    path: str | None = Field(default=None, description="Directory path to list. Defaults to current working directory.")
    execution_id: str = Field(..., description="Unique execution identifier")
    max_depth: int | None = Field(default=None, description="Recursion depth. 0 = current directory only. Max 3.")
    max_entries: int | None = Field(default=None, description="Maximum number of entries to return. Max 500.")

    def to_protobuf(self) -> operator_pb2.FsListRequested:
        """Convert to protobuf FsListRequested message."""
        proto = operator_pb2.FsListRequested()
        if self.path:
            proto.path = self.path
        proto.execution_id = self.execution_id
        if self.max_depth is not None:
            proto.max_depth = self.max_depth
        if self.max_entries is not None:
            proto.max_entries = self.max_entries
        return proto


class FsGrepRequestPayload(TargetedOperatorBase):
    """Payload for EventType.OPERATOR_FILESYSTEM_GREP_REQUESTED."""
    payload_type: Literal["fs_grep"] = Field(default="fs_grep", description="Payload type discriminator")
    path: str = Field(..., description="Directory path to search. Can be absolute or relative.")
    pattern: str = Field(..., description="Regular expression pattern to search for.")
    execution_id: str = Field(..., description="Unique execution identifier")
    includes: list[str] | None = Field(default=None, description="Glob patterns to filter files.")
    max_matches: int | None = Field(default=100, description="Maximum number of matches to return. Max 500.")

    def to_protobuf(self) -> operator_pb2.FsGrepRequested:
        """Convert to protobuf FsGrepRequested message."""
        proto = operator_pb2.FsGrepRequested()
        proto.path = self.path
        proto.execution_id = self.execution_id
        proto.pattern = self.pattern
        if self.includes:
            proto.includes.extend(self.includes)
        if self.max_matches is not None:
            proto.max_matches = self.max_matches
        return proto


class FsReadRequestPayload(TargetedOperatorBase):
    """Payload for EventType.OPERATOR_FILESYSTEM_READ_REQUESTED."""
    payload_type: Literal["fs_read"] = Field(default="fs_read", description="Payload type discriminator")
    path: str = Field(..., description="Absolute or relative path to the file to read")
    execution_id: str = Field(..., description="Unique execution identifier")
    max_size: int | None = Field(default=None, description="Maximum number of bytes to read. Defaults to 100 KiB.")

    def to_protobuf(self) -> operator_pb2.FsReadRequested:
        """Convert to protobuf FsReadRequested message."""
        proto = operator_pb2.FsReadRequested()
        proto.path = self.path
        proto.execution_id = self.execution_id
        if self.max_size is not None:
            proto.max_size = self.max_size
        return proto


class FetchLogsRequestPayload(G8eBaseModel):
    """Payload for EventType.OPERATOR_LOGS_FETCH_REQUESTED."""
    payload_type: Literal["fetch_logs"] = Field(default="fetch_logs", description="Payload type discriminator")
    execution_id: str = Field(..., description="execution_id of the stored execution to fetch logs for")
    sentinel_mode: str | None = Field(default=None, description="Vault scrubbing mode to use when reading")

    def to_protobuf(self) -> operator_pb2.FetchLogsRequested:
        """Convert to protobuf FetchLogsRequested message."""
        proto = operator_pb2.FetchLogsRequested()
        proto.execution_id = self.execution_id
        if self.sentinel_mode:
            proto.sentinel_mode = self.sentinel_mode
        return proto


class FetchHistoryRequestPayload(G8eBaseModel):
    """Payload for EventType.OPERATOR_HISTORY_FETCH_REQUESTED."""
    payload_type: Literal["fetch_history"] = Field(default="fetch_history", description="Payload type discriminator")
    execution_id: str = Field(..., description="Unique execution identifier")
    operator_session_id: str | None = Field(default=None, description="Operator session ID to scope history to")
    limit: int | None = Field(default=None, description="Maximum number of history entries to return")
    offset: int | None = Field(default=None, description="Number of history entries to skip")
    include_commands: bool | None = Field(default=None, description="Include command execution entries")
    include_file_mutations: bool | None = Field(default=None, description="Include file mutation entries")

    def to_protobuf(self) -> operator_pb2.FetchHistoryRequested:
        """Convert to protobuf FetchHistoryRequested message."""
        proto = operator_pb2.FetchHistoryRequested()
        proto.execution_id = self.execution_id
        if self.operator_session_id:
            proto.operator_session_id = self.operator_session_id
        if self.limit is not None:
            proto.limit = self.limit
        if self.offset is not None:
            proto.offset = self.offset
        if self.include_commands is not None:
            proto.include_commands = self.include_commands
        if self.include_file_mutations is not None:
            proto.include_file_mutations = self.include_file_mutations
        return proto


class FetchFileHistoryRequestPayload(TargetedOperatorBase):
    """Payload for EventType.OPERATOR_FILE_HISTORY_FETCH_REQUESTED."""
    payload_type: Literal["fetch_file_history"] = Field(default="fetch_file_history", description="Payload type discriminator")
    execution_id: str = Field(..., description="Unique execution identifier")
    file_path: str = Field(..., description="Absolute path to the file to retrieve edit history for")
    limit: int | None = Field(default=None, description="Maximum number of history entries to return")

    def to_protobuf(self) -> operator_pb2.FetchFileHistoryRequested:
        """Convert to protobuf FetchFileHistoryRequested message."""
        proto = operator_pb2.FetchFileHistoryRequested()
        proto.execution_id = self.execution_id
        proto.file_path = self.file_path
        if self.limit is not None:
            proto.limit = self.limit
        return proto


class FetchFileDiffRequestPayload(TargetedOperatorBase):
    """Payload for EventType.OPERATOR_FILE_DIFF_FETCH_REQUESTED."""
    payload_type: Literal["fetch_file_diff"] = Field(default="fetch_file_diff", description="Payload type discriminator")
    execution_id: str = Field(..., description="Unique execution identifier")
    diff_id: str | None = Field(default=None, description="Specific diff entry ID to fetch")
    operator_session_id: str | None = Field(default=None, description="Fetch all diffs for an operator session")
    file_path: str | None = Field(default=None, description="Filter diffs by file path")
    limit: int | None = Field(default=None, description="Maximum number of diff entries to return")

    def to_protobuf(self) -> operator_pb2.FetchFileDiffRequested:
        """Convert to protobuf FetchFileDiffRequested message."""
        proto = operator_pb2.FetchFileDiffRequested()
        proto.execution_id = self.execution_id
        if self.diff_id:
            proto.diff_id = self.diff_id
        if self.operator_session_id:
            proto.operator_session_id = self.operator_session_id
        if self.file_path:
            proto.file_path = self.file_path
        if self.limit is not None:
            proto.limit = self.limit
        return proto


class CheckPortRequestPayload(TargetedOperatorBase):
    """Payload for EventType.OPERATOR_PORT_CHECK_REQUESTED."""
    payload_type: Literal["check_port"] = Field(default="check_port", description="Payload type discriminator")
    execution_id: str = Field(..., description="Unique execution identifier")
    port: int = Field(..., description="Port number to check (1-65535)")
    host: str = Field(default="localhost", description="Host to check (IP address or hostname)")
    protocol: str = Field(default="tcp", description="Protocol to use: 'tcp' or 'udp'")

    def to_protobuf(self) -> operator_pb2.CheckPortRequested:
        """Convert to protobuf CheckPortRequested message."""
        proto = operator_pb2.CheckPortRequested()
        proto.execution_id = self.execution_id
        proto.port = self.port
        proto.host = self.host
        proto.protocol = self.protocol
        return proto


class RestoreFileRequestPayload(G8eBaseModel):
    """Payload for EventType.OPERATOR_FILE_RESTORE_REQUESTED."""
    payload_type: Literal["restore_file"] = Field(default="restore_file", description="Payload type discriminator")
    execution_id: str = Field(..., description="Unique execution identifier")
    file_path: str = Field(..., description="Absolute path of the file to restore")
    commit_hash: str = Field(..., description="Git commit hash to restore the file to")
    operator_session_id: str | None = Field(default=None, description="Operator session ID for auditing")

    def to_protobuf(self) -> operator_pb2.RestoreFileRequested:
        """Convert to protobuf RestoreFileRequested message."""
        proto = operator_pb2.RestoreFileRequested()
        proto.execution_id = self.execution_id
        proto.file_path = self.file_path
        proto.commit_hash = self.commit_hash
        if self.operator_session_id:
            proto.operator_session_id = self.operator_session_id
        return proto


class DirectCommandAuditRequestPayload(G8eBaseModel):
    """Payload for EventType.OPERATOR_AUDIT_DIRECT_COMMAND_RECORDED."""
    payload_type: Literal["direct_command_audit"] = Field(default="direct_command_audit", description="Payload type discriminator")
    command: str = Field(..., description="Shell command that was executed")
    execution_id: str = Field(..., description="Execution identifier for the command")
    operator_session_id: str = Field(..., description="Operator session ID")
    type: Literal["direct_terminal_exec"] = Field(default="direct_terminal_exec", description="Audit event type")

    def to_protobuf(self) -> operator_pb2.DirectCommandAuditRequested:
        """Convert to protobuf DirectCommandAuditRequested message."""
        proto = operator_pb2.DirectCommandAuditRequested()
        proto.command = self.command
        proto.execution_id = self.execution_id
        proto.operator_session_id = self.operator_session_id
        proto.type = self.type
        return proto


class HeartbeatRequestPayload(G8eBaseModel):
    """Payload for EventType.OPERATOR_HEARTBEAT_REQUESTED."""
    payload_type: Literal["heartbeat"] = Field(default="heartbeat", description="Payload type discriminator")

    def to_protobuf(self) -> operator_pb2.HeartbeatRequested:
        """Convert to protobuf HeartbeatRequested message."""
        return operator_pb2.HeartbeatRequested()


# Union type for all outbound command payloads to g8eo
G8eCommandPayload = Union[
    CommandRequestPayload,
    CommandCancelRequestPayload,
    FileEditRequestPayload,
    FsListRequestPayload,
    FsGrepRequestPayload,
    FsReadRequestPayload,
    FetchLogsRequestPayload,
    FetchHistoryRequestPayload,
    FetchFileHistoryRequestPayload,
    FetchFileDiffRequestPayload,
    CheckPortRequestPayload,
    RestoreFileRequestPayload,
    DirectCommandAuditRequestPayload,
    HeartbeatRequestPayload,
]
