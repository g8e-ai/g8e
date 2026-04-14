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
Command payload models for g8eo pub/sub wire protocol.

These are the inbound payload shapes published by g8ee to the
cmd:{operator_id}:{operator_session_id} pub/sub channel. g8eo deserializes
them. Field names and types mirror the canonical Go structs in
components/g8eo/models/commands.go and the JSON schema in
shared/models/wire/command_payloads.json.
"""


from pydantic import Field

from app.constants import FileOperation
from app.models.base import G8eBaseModel


class TargetedOperatorArgs(G8eBaseModel):
    """Base class for tool args that can be routed to a specific operator."""
    target_operator: str | None = Field(
        default=None,
        description=(
            "Which Operator to execute on when multiple operators are bound. "
            "Can be: operator_id, hostname, or index ('0', '1'). "
            "Required when multiple operators are bound."
        ),
    )


class CommandPayload(G8eBaseModel):
    """Payload for EventType.OPERATOR_COMMAND_REQUESTED."""
    command: str = Field(..., description="Shell command string to execute")
    execution_id: str | None = Field(default=None, description="Unique execution identifier")
    justification: str | None = Field(default=None, description="Justification for running this command")
    sentinel_mode: str | None = Field(default=None, description="Vault scrubbing mode for output storage")
    timeout_seconds: int | None = Field(default=None, description="Execution timeout override in seconds")


class CommandCancelPayload(G8eBaseModel):
    """Payload for EventType.OPERATOR_COMMAND_CANCEL_REQUESTED."""
    execution_id: str = Field(..., description="execution_id of the running command to cancel")


class FileEditPayload(TargetedOperatorArgs):
    """Payload for EventType.OPERATOR_FILE_EDIT_REQUESTED and typed args for _execute_file_edit."""
    file_path: str = Field(..., description="Absolute path to the target file on the operator host")
    operation: FileOperation = Field(..., description="File operation type")
    justification: str | None = Field(default=None, description="Justification for this file operation")
    execution_id: str | None = Field(default=None, description="Unique execution identifier")
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


class FsListPayload(G8eBaseModel):
    """Payload for EventType.OPERATOR_FILESYSTEM_LIST_REQUESTED."""
    path: str | None = Field(default=None, description="Directory path to list. Defaults to current working directory.")
    execution_id: str | None = Field(default=None, description="Unique execution identifier")
    max_depth: int | None = Field(default=None, description="Recursion depth. 0 = current directory only. Max 3.")
    max_entries: int | None = Field(default=None, description="Maximum number of entries to return. Max 500.")


class FsReadPayload(G8eBaseModel):
    """Payload for EventType.OPERATOR_FILESYSTEM_READ_REQUESTED."""
    path: str = Field(..., description="Absolute or relative path to the file to read")
    execution_id: str | None = Field(default=None, description="Unique execution identifier")
    max_size: int | None = Field(default=None, description="Maximum number of bytes to read. Defaults to 100 KiB.")


class FetchLogsPayload(G8eBaseModel):
    """Payload for EventType.OPERATOR_LOGS_FETCH_REQUESTED."""
    execution_id: str = Field(..., description="execution_id of the stored execution to fetch logs for")
    sentinel_mode: str | None = Field(default=None, description="Vault scrubbing mode to use when reading")


class FetchHistoryPayload(G8eBaseModel):
    """Payload for EventType.OPERATOR_HISTORY_FETCH_REQUESTED."""
    operator_session_id: str | None = Field(default=None, description="Operator session ID to scope history to")
    limit: int | None = Field(default=None, description="Maximum number of history entries to return")
    offset: int | None = Field(default=None, description="Number of history entries to skip")
    include_commands: bool | None = Field(default=None, description="Include command execution entries")
    include_file_mutations: bool | None = Field(default=None, description="Include file mutation entries")


class FetchFileHistoryPayload(G8eBaseModel):
    """Payload for EventType.OPERATOR_FILE_HISTORY_FETCH_REQUESTED."""
    file_path: str = Field(..., description="Absolute path to the file to retrieve edit history for")
    limit: int | None = Field(default=None, description="Maximum number of history entries to return")


class FetchFileDiffPayload(G8eBaseModel):
    """Payload for EventType.OPERATOR_FILE_DIFF_FETCH_REQUESTED."""
    diff_id: str | None = Field(default=None, description="Specific diff entry ID to fetch")
    operator_session_id: str | None = Field(default=None, description="Fetch all diffs for an operator session")
    file_path: str | None = Field(default=None, description="Filter diffs by file path")
    limit: int | None = Field(default=None, description="Maximum number of diff entries to return")


class RestoreFilePayload(G8eBaseModel):
    """Payload for EventType.OPERATOR_FILE_RESTORE_REQUESTED."""
    file_path: str = Field(..., description="Absolute path of the file to restore")
    commit_hash: str = Field(..., description="Git commit hash to restore the file to")


class FileCreateArgs(TargetedOperatorArgs):
    """LLM tool call args for OperatorToolName.FILE_CREATE."""
    file_path: str = Field(..., description="Absolute path where the file should be created.")
    content: str = Field(..., description="Content to write to the new file.")
    justification: str = Field(
        ...,
        description=(
            "REQUIRED. Clear explanation of why this file needs to be created. "
            "Examples: 'Creating backup script for database', 'Adding new configuration file'."
        ),
    )


class FileWriteArgs(TargetedOperatorArgs):
    """LLM tool call args for OperatorToolName.FILE_WRITE."""
    file_path: str = Field(..., description="Absolute path to the file to write.")
    content: str = Field(..., description="Full content to write (replaces entire file).")
    justification: str = Field(
        ...,
        description=(
            "REQUIRED. Clear explanation of why this complete replacement is needed. "
            "Examples: 'Regenerating config after corruption', 'Replacing template with actual values'."
        ),
    )
    create_if_missing: bool = Field(default=False, description="Create file if it doesn't exist. Default: false.")
    create_backup: bool = Field(default=True, description="Create backup before overwriting. Default: true.")


class FileReadArgs(TargetedOperatorArgs):
    """LLM tool call args for OperatorToolName.FILE_READ."""
    file_path: str = Field(..., description="Absolute path to the file to read.")
    justification: str = Field(
        ...,
        description=(
            "REQUIRED. Clear explanation of why this file needs to be read. "
            "Examples: 'Reading error logs to diagnose crash', 'Inspecting config before update'."
        ),
    )
    start_line: int | None = Field(default=None, description="Starting line number, 1-indexed. First line to include in output.")
    end_line: int | None = Field(default=None, description="Ending line number, 1-indexed inclusive. Last line to include.")
    max_lines: int | None = Field(default=None, description="Maximum number of lines to read. Use to limit output for large files.")


class FileUpdateArgs(TargetedOperatorArgs):
    """LLM tool call args for OperatorToolName.FILE_UPDATE."""
    file_path: str = Field(..., description="Absolute path to the file to update.")
    old_content: str = Field(
        ...,
        description=(
            "Exact text to find and replace. Must match existing content exactly "
            "including whitespace. Read file first and copy exact text."
        ),
    )
    new_content: str = Field(..., description="Text to replace old_content with.")
    justification: str = Field(
        ...,
        description=(
            "REQUIRED. Clear explanation of why this update is needed. "
            "Examples: 'Fixing typo in config', 'Updating port number', 'Enabling debug mode'."
        ),
    )
    create_backup: bool = Field(default=True, description="Create backup before modifying. Default: true.")


class SearchWebArgs(G8eBaseModel):
    """LLM tool call args for OperatorToolName.G8E_SEARCH_WEB."""
    query: str = Field(..., description="The search query. Be specific and technical for best results.")
    num: int = Field(default=5, description="Number of results to return (1-10). Default: 5.")


class CheckPortArgs(TargetedOperatorArgs):
    """LLM tool call args for OperatorToolName.CHECK_PORT."""
    port: int = Field(..., description="Port number to check (1-65535). Required.")
    host: str = Field(default="localhost", description="Host to check (IP address or hostname). Defaults to 'localhost' if not specified.")
    protocol: str = Field(default="tcp", description="Protocol to use: 'tcp' or 'udp'. Defaults to 'tcp'.")


class FsReadArgs(TargetedOperatorArgs):
    """LLM tool call args for OperatorToolName.FS_READ (low-level file read via pub/sub)."""
    path: str | None = Field(
        None,
        description="Path to the file to read. Can be absolute or relative to operator's working directory.",
    )


class FsListArgs(TargetedOperatorArgs):
    """LLM tool call args for OperatorToolName.LIST_FILES."""
    path: str | None = Field(
        None,
        description=(
            "Directory path to list. Can be absolute (e.g., '/var/log') or relative "
            "to operator's current working directory. Use '.' or empty for current directory."
        ),
    )
    max_depth: int | None = Field(
        None,
        description=(
            "Maximum depth for recursive listing. 0 = current directory only (default), "
            "1 = include immediate subdirectories, 2 = two levels deep, max 3."
        ),
    )
    max_entries: int | None = Field(
        None,
        description=(
            "Maximum number of entries to return. Default 100, max 500. "
            "Use lower values for quick scans, higher for comprehensive listings."
        ),
    )


class FetchExecutionOutputArgs(TargetedOperatorArgs):
    """LLM tool call args for OperatorToolName.FETCH_EXECUTION_OUTPUT."""
    execution_id: str = Field(..., description="The unique execution ID of the command whose output you want to retrieve.")


class FetchSessionHistoryArgs(TargetedOperatorArgs):
    """LLM tool call args for OperatorToolName.FETCH_SESSION_HISTORY."""
    operator_session_id: str = Field(..., description="The operator session ID to fetch history for.")
    limit: int | None = Field(default=None, description="Maximum number of events to return (default: 50).")
    offset: int | None = Field(default=None, description="Number of events to skip for pagination (default: 0).")
    include_commands: bool | None = Field(default=None, description="Include command execution entries")
    include_file_mutations: bool | None = Field(default=None, description="Include file mutation entries")


class FetchFileHistoryArgs(TargetedOperatorArgs):
    """LLM tool call args for OperatorToolName.FETCH_FILE_HISTORY."""
    file_path: str = Field(..., description="Absolute path to the file to get history for (e.g., /etc/nginx/nginx.conf).")
    limit: int | None = Field(default=None, description="Maximum number of history entries to return (default: 50).")


class RestoreFileArgs(TargetedOperatorArgs):
    """LLM tool call args for OperatorToolName.RESTORE_FILE."""
    file_path: str = Field(..., description="Absolute path to the file to restore (e.g., /etc/nginx/nginx.conf).")
    commit_hash: str = Field(..., description="Git commit hash to restore from (get this from fetch_file_history).")


class FetchFileDiffArgs(TargetedOperatorArgs):
    """LLM tool call args for OperatorToolName.FETCH_FILE_DIFF."""
    diff_id: str | None = Field(default=None, description="Specific diff ID to retrieve full diff content for.")
    operator_session_id: str | None = Field(default=None, description="Operator session ID to list all file diffs for.")
    file_path: str | None = Field(default=None, description="Filter results by file path (only used with operator_session_id).")
    limit: int | None = Field(default=None, description="Maximum number of diffs to return (default: 50, only used with operator_session_id).")


class GrantIntentArgs(G8eBaseModel):
    """LLM tool call args for OperatorToolName.GRANT_INTENT."""
    intent_name: str = Field(
        ...,
        description=(
            "Single intent OR comma-separated intents for complex operations. "
            "Examples: 'ec2_discovery' or 'ec2_management,ec2_discovery' or 's3_read,s3_write'"
        ),
    )
    justification: str = Field(
        ...,
        description=(
            "REQUIRED. Clear explanation of why this permission is needed. "
            "Example: 'Need to list EC2 instances and reboot them as requested by user.'"
        ),
    )
    operation_context: str | None = Field(
        None,
        description=(
            "The high-level operation being performed (e.g., 'Reboot EC2 instances', "
            "'Backup S3 to local storage', 'Deploy Lambda function'). This helps users "
            "understand the full scope of what they're approving."
        ),
    )
    pending_command: str | None = Field(
        None,
        description=(
            "The AWS CLI command to execute immediately after permission is granted. "
            "When provided, the command executes automatically upon approval - no second "
            "approval needed. Example: 'aws ec2 describe-instances --output table'"
        ),
    )
    pending_command_justification: str | None = Field(
        None,
        description=(
            "Justification for the pending command. Required if pending_command is provided. "
            "Example: 'List EC2 instances to display their status to the user.'"
        ),
    )


class RevokeIntentArgs(G8eBaseModel):
    """LLM tool call args for OperatorToolName.REVOKE_INTENT."""
    intent_name: str = Field(
        ...,
        description=(
            "The intent to revoke (e.g., 'ec2_discovery', 's3_read'). "
            "Use comma-separated values for multiple intents."
        ),
    )
    justification: str = Field(
        ...,
        description=(
            "REQUIRED. Clear explanation of why this permission is being revoked. "
            "Example: 'User requested removal of EC2 access.'"
        ),
    )


class QueryInvestigationContextArgs(G8eBaseModel):
    """LLM tool call args for OperatorToolName.QUERY_INVESTIGATION_CONTEXT."""
    data_type: str = Field(
        ...,
        description=(
            "Type of data to retrieve. One of: conversation_history, investigation_status, "
            "history_trail, operator_actions"
        ),
    )
    limit: int | None = Field(
        default=None,
        description=(
            "Maximum number of items to return (for conversation_history and history_trail). "
            "Optional. Use to limit output size for large investigations."
        ),
    )
