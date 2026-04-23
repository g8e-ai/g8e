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
them. Field names and types mirror the canonical Go structs in
components/g8eo/models/commands.go and the JSON schema in
shared/models/wire/command_payloads.json.
"""

from typing import Literal, Union

from pydantic import Field

from app.constants import FileOperation
from app.models.base import G8eBaseModel

__all__ = [
    "TargetedOperatorBase",
    "CommandRequestPayload",
    "CommandCancelRequestPayload",
    "FileEditRequestPayload",
    "FsListRequestPayload",
    "FsReadRequestPayload",
    "FetchLogsRequestPayload",
    "FetchHistoryRequestPayload",
    "FetchFileHistoryRequestPayload",
    "FetchFileDiffRequestPayload",
    "CheckPortRequestPayload",
    "RestoreFileRequestPayload",
    "DirectCommandAuditRequestPayload",
    "G8eCommandPayload",
]


class TargetedOperatorBase(G8eBaseModel):
    """Base class for tool args that can be routed to a specific operator."""
    target_operator: str | None = Field(
        default=None,
        description=(
            "Which Operator to execute on when multiple operators are bound. "
            "Can be: operator_id, hostname, or index ('0', '1'). "
            "Required when multiple operators are bound."
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


class CommandCancelRequestPayload(G8eBaseModel):
    """Payload for EventType.OPERATOR_COMMAND_CANCEL_REQUESTED."""
    payload_type: Literal["command_cancel"] = Field(default="command_cancel", description="Payload type discriminator")
    execution_id: str = Field(..., description="execution_id of the running command to cancel")


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


class FsListRequestPayload(TargetedOperatorBase):
    """Payload for EventType.OPERATOR_FILESYSTEM_LIST_REQUESTED."""
    payload_type: Literal["fs_list"] = Field(default="fs_list", description="Payload type discriminator")
    path: str | None = Field(default=None, description="Directory path to list. Defaults to current working directory.")
    execution_id: str = Field(..., description="Unique execution identifier")
    max_depth: int | None = Field(default=None, description="Recursion depth. 0 = current directory only. Max 3.")
    max_entries: int | None = Field(default=None, description="Maximum number of entries to return. Max 500.")


class FsReadRequestPayload(TargetedOperatorBase):
    """Payload for EventType.OPERATOR_FILESYSTEM_READ_REQUESTED."""
    payload_type: Literal["fs_read"] = Field(default="fs_read", description="Payload type discriminator")
    path: str = Field(..., description="Absolute or relative path to the file to read")
    execution_id: str = Field(..., description="Unique execution identifier")
    max_size: int | None = Field(default=None, description="Maximum number of bytes to read. Defaults to 100 KiB.")


class FetchLogsRequestPayload(G8eBaseModel):
    """Payload for EventType.OPERATOR_LOGS_FETCH_REQUESTED."""
    payload_type: Literal["fetch_logs"] = Field(default="fetch_logs", description="Payload type discriminator")
    execution_id: str = Field(..., description="execution_id of the stored execution to fetch logs for")
    sentinel_mode: str | None = Field(default=None, description="Vault scrubbing mode to use when reading")


class FetchHistoryRequestPayload(G8eBaseModel):
    """Payload for EventType.OPERATOR_HISTORY_FETCH_REQUESTED."""
    payload_type: Literal["fetch_history"] = Field(default="fetch_history", description="Payload type discriminator")
    execution_id: str = Field(..., description="Unique execution identifier")
    operator_session_id: str | None = Field(default=None, description="Operator session ID to scope history to")
    limit: int | None = Field(default=None, description="Maximum number of history entries to return")
    offset: int | None = Field(default=None, description="Number of history entries to skip")
    include_commands: bool | None = Field(default=None, description="Include command execution entries")
    include_file_mutations: bool | None = Field(default=None, description="Include file mutation entries")


class FetchFileHistoryRequestPayload(TargetedOperatorBase):
    """Payload for EventType.OPERATOR_FILE_HISTORY_FETCH_REQUESTED."""
    payload_type: Literal["fetch_file_history"] = Field(default="fetch_file_history", description="Payload type discriminator")
    execution_id: str = Field(..., description="Unique execution identifier")
    file_path: str = Field(..., description="Absolute path to the file to retrieve edit history for")
    limit: int | None = Field(default=None, description="Maximum number of history entries to return")


class FetchFileDiffRequestPayload(TargetedOperatorBase):
    """Payload for EventType.OPERATOR_FILE_DIFF_FETCH_REQUESTED."""
    payload_type: Literal["fetch_file_diff"] = Field(default="fetch_file_diff", description="Payload type discriminator")
    execution_id: str = Field(..., description="Unique execution identifier")
    diff_id: str | None = Field(default=None, description="Specific diff entry ID to fetch")
    operator_session_id: str | None = Field(default=None, description="Fetch all diffs for an operator session")
    file_path: str | None = Field(default=None, description="Filter diffs by file path")
    limit: int | None = Field(default=None, description="Maximum number of diff entries to return")


class CheckPortRequestPayload(TargetedOperatorBase):
    """Payload for EventType.OPERATOR_PORT_CHECK_REQUESTED."""
    payload_type: Literal["check_port"] = Field(default="check_port", description="Payload type discriminator")
    execution_id: str = Field(..., description="Unique execution identifier")
    port: int = Field(..., description="Port number to check (1-65535)")
    host: str = Field(default="localhost", description="Host to check (IP address or hostname)")
    protocol: str = Field(default="tcp", description="Protocol to use: 'tcp' or 'udp'")


class RestoreFileRequestPayload(G8eBaseModel):
    """Payload for EventType.OPERATOR_FILE_RESTORE_REQUESTED."""
    payload_type: Literal["restore_file"] = Field(default="restore_file", description="Payload type discriminator")
    execution_id: str = Field(..., description="Unique execution identifier")
    file_path: str = Field(..., description="Absolute path of the file to restore")
    commit_hash: str = Field(..., description="Git commit hash to restore the file to")


class DirectCommandAuditRequestPayload(G8eBaseModel):
    """Payload for EventType.OPERATOR_AUDIT_DIRECT_COMMAND_RECORDED."""
    payload_type: Literal["direct_command_audit"] = Field(default="direct_command_audit", description="Payload type discriminator")
    command: str = Field(..., description="Shell command that was executed")
    execution_id: str = Field(..., description="Execution identifier for the command")
    operator_session_id: str = Field(..., description="Operator session ID")
    type: Literal["direct_terminal_exec"] = Field(default="direct_terminal_exec", description="Audit event type")


# Union type for all outbound command payloads to g8eo
G8eCommandPayload = Union[
    CommandRequestPayload,
    CommandCancelRequestPayload,
    FileEditRequestPayload,
    FsListRequestPayload,
    FsReadRequestPayload,
    FetchLogsRequestPayload,
    FetchHistoryRequestPayload,
    FetchFileHistoryRequestPayload,
    FetchFileDiffRequestPayload,
    CheckPortRequestPayload,
    RestoreFileRequestPayload,
    DirectCommandAuditRequestPayload,
]
