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
Typed result models for AI tool operations.

Canonical field definitions: shared/models/tool_results.json (authority).
Enum value sources: shared/constants/status.json (command.error.type, execution.status,
file.operation, network.protocol, risk.level).

When adding or renaming a field, update shared/models/tool_results.json first.
"""

from typing import Any, Union

from pydantic import Field

from app.constants import (
    CommandErrorType,
    ErrorAnalysisCategory,
    ExecutionStatus,
    FileOperation,
    NetworkProtocol,
    RiskLevel,
)
from app.models.base import G8eBaseModel, UTCDatetime
from app.models.ssh_inventory import SshHost
from app.models.whitelist import WhitelistedCommand


class BatchExecutionMeta(G8eBaseModel):
    """Mixin for batch execution metadata shared across all tool result types."""
    batch_execution: bool = False
    batch_id: str | None = None
    operators_used: int = 0
    successful_count: int = 0
    failed_count: int = 0


class PerOperatorResultBase(G8eBaseModel):
    """Base class for per-operator result entries."""
    execution_id: str
    operator_id: str
    hostname: str
    success: bool
    error: str | None = None


class FsListEntry(G8eBaseModel):
    """A single directory entry returned by an fs_list operation.

    Canonical shape defined in shared/proto/operator.proto (FsListEntry message).
    """
    name: str
    path: str
    is_dir: bool
    size: int = 0
    mode: str = ""
    mod_time: int = 0
    is_symlink: bool = False
    symlink_target: str | None = None
    owner: str | None = None
    group: str | None = None
    inode: int | None = None
    nlink: int | None = None


class FsGrepMatch(G8eBaseModel):
    """A single grep match record."""
    path: str
    line_number: int
    content: str
    before: list[str] = Field(default_factory=list)
    after: list[str] = Field(default_factory=list)


class AuditFileMutation(G8eBaseModel):
    """A single file mutation record embedded in an AuditEvent.

    Canonical shape defined in shared/proto/operator.proto (AuditFileMutation message).
    """
    id: int
    filepath: str
    operation: str
    ledger_hash_before: str | None = None
    ledger_hash_after: str | None = None
    diff_stat: str | None = None


class AuditEvent(G8eBaseModel):
    """A single audit event record returned by fetch_session_history.

    Canonical shape defined in shared/proto/operator.proto (AuditEvent message).
    """
    id: int | None = None
    web_session_id: str | None = None
    timestamp: str | None = None
    type: str
    content_text: str | None = None
    command_raw: str | None = None
    command_exit_code: int | None = None
    command_stdout: str | None = None
    command_stderr: str | None = None
    execution_duration_ms: int | None = None
    stored_locally: bool = False
    stdout_truncated: bool = False
    stderr_truncated: bool = False
    file_mutations: list[AuditFileMutation] = Field(default_factory=list)


class FileHistoryEntry(G8eBaseModel):
    """A single commit history entry returned by fetch_file_history.

    Canonical shape defined in shared/proto/operator.proto (FileHistoryEntry message).
    """
    commit_hash: str
    timestamp: str | None = None
    message: str


class CommandInternalResult(G8eBaseModel):
    """Typed result returned by _execute_command_internal — the pub/sub wire boundary.

    Built from the raw operator response once it arrives over the pub/sub channel.
    All fields above this boundary must be typed; this model is the conversion point.
    """
    execution_id: str | None = None
    status: ExecutionStatus
    output: str | None = None
    stderr: str | None = None
    error: str | None = None
    error_type: CommandErrorType | None = None
    error_analysis: "ErrorAnalysisResult | None" = None
    exit_code: int | None = None
    execution_time_seconds: float | None = None
    operator_id: str | None = None
    completed_at: UTCDatetime | None = None
    suggestion: str | None = None
    command: str | None = None

    def get_truncated_output(self, limit: int = 20) -> str:
        """Get output truncated to first/last N lines."""
        if not self.output:
            return ""

        lines = self.output.splitlines()
        if len(lines) <= limit * 2:
            return self.output

        first = lines[:limit]
        last = lines[-limit:]
        middle_count = len(lines) - (len(first) + len(last))

        return "\n".join(first) + f"\n\n... [{middle_count} lines truncated] ...\n\n" + "\n".join(last)


class CommandRiskContext(G8eBaseModel):
    working_directory: str = Field(default="", description="Working directory for the command")
    git_status: str = Field(default="", description="Git repository status")
    investigation_context: str = Field(default="", description="Brief description of the active investigation (e.g. case title and description) to help Warden reason about expected command scope")


class ErrorAnalysisContext(G8eBaseModel):
    retry_count: int = Field(default=0, description="Number of retry attempts so far")
    working_directory: str = Field(default="", description="Working directory when the error occurred")
    execution_id: str | None = Field(default=None, description="Execution ID for correlation")


class FileOperationRiskContext(G8eBaseModel):
    git_status: str = Field(default="", description="Git repository status")
    backup_available: bool = Field(default=False, description="Whether a backup exists for the target file")


class CommandRiskAnalysis(G8eBaseModel):
    risk_level: RiskLevel = Field(description="Classified risk level")


class ErrorAnalysisResult(G8eBaseModel):
    error_category: ErrorAnalysisCategory = Field(description="LLM-classified failure category")
    root_cause: str = Field(description="Brief root cause analysis")
    can_auto_fix: bool = Field(description="Whether the error can be automatically fixed")
    suggested_fix: str | None = Field(default=None, description="Description of the suggested fix")
    suggested_command: str | None = Field(default=None, description="Exact command to fix the issue")
    should_escalate: bool = Field(description="Whether to escalate to human intervention")
    reasoning: str = Field(description="Explanation of the decision")
    user_message: str = Field(description="Brief message to show the user")


class FileOperationRiskAnalysis(G8eBaseModel):
    risk_level: RiskLevel = Field(description="Classified risk level")
    is_system_file: bool | None = Field(default=None, description="Whether the target is a system file")
    safe_to_proceed: bool = Field(default=True, description="Whether the operation is safe to proceed")
    blocking_issues: list[str] = Field(default_factory=list, description="Issues that block the operation")
    approval_prompt: str | None = Field(default=None, description="Human-readable approval prompt")



class PerOperatorFileEditResult(PerOperatorResultBase):
    """Per-operator result for file edit operations."""
    success: bool
    content: str | None = None
    backup_path: str | None = None
    stderr: str | None = None


class FileEditResult(BatchExecutionMeta):
    """Result returned by FileEditMixin._execute_file_edit."""
    success: bool = True
    error: str | None = None
    error_type: CommandErrorType | None = None
    file_path: str | None = None
    operation: FileOperation | None = None
    content: str | None = None
    backup_path: str | None = None
    bytes_written: int | None = None
    lines_changed: int | None = None
    duration_seconds: float | None = None
    approved: bool | None = None
    blocking_issues: list[str] | None = None
    risk_analysis: FileOperationRiskAnalysis | None = None
    per_operator_results: list[PerOperatorFileEditResult] | None = None


class PerOperatorPortCheckResult(PerOperatorResultBase):
    """Per-operator result for port check operations."""
    is_open: bool | None = None
    latency_ms: float | None = None
    error_type: str | None = None
    protocol: NetworkProtocol | None = None


class PortCheckToolResult(BatchExecutionMeta):
    """Result returned by PortOperationsMixin._execute_port_check."""
    success: bool = True
    error: str | None = None
    error_type: CommandErrorType | None = None
    host: str | None = None
    port: int | None = None
    protocol: NetworkProtocol | None = None
    is_open: bool | None = None
    latency_ms: float | None = None
    per_operator_results: list[PerOperatorPortCheckResult] | None = None


class PerOperatorFsListResult(PerOperatorResultBase):
    """Per-operator result for filesystem list operations."""
    entries: list[FsListEntry] = Field(default_factory=list)
    total_count: int = 0
    truncated: bool = False


class FsListToolResult(BatchExecutionMeta):
    """Result returned by FilesystemMixin._execute_fs_list."""
    success: bool = True
    error: str | None = None
    error_type: CommandErrorType | None = None
    path: str | None = None
    entries: list[FsListEntry] = Field(default_factory=list)
    total_count: int = 0
    truncated: bool = False
    per_operator_results: list[PerOperatorFsListResult] | None = None


class PerOperatorFsReadResult(PerOperatorResultBase):
    """Per-operator result for filesystem read operations."""
    content: str | None = None
    size: int = 0
    encoding: str | None = None
    line_count: int | None = None


class FsReadToolResult(BatchExecutionMeta):
    """Result returned by FilesystemMixin._execute_file_read."""
    success: bool = True
    error: str | None = None
    error_type: CommandErrorType | None = None
    path: str | None = None
    content: str | None = None
    size: int = 0
    truncated: bool = False
    per_operator_results: list[PerOperatorFsReadResult] | None = None


class PerOperatorFsGrepResult(PerOperatorResultBase):
    """Per-operator result for filesystem grep operations."""
    matches: list[FsGrepMatch] = Field(default_factory=list)
    total_matches: int = 0
    truncated: bool = False


class FsGrepToolResult(BatchExecutionMeta):
    """Result returned by FilesystemMixin._execute_fs_grep."""
    success: bool = True
    error: str | None = None
    error_type: CommandErrorType | None = None
    path: str | None = None
    pattern: str | None = None
    matches: list[FsGrepMatch] = Field(default_factory=list)
    total_matches: int = 0
    truncated: bool = False
    per_operator_results: list[PerOperatorFsGrepResult] | None = None


class FetchLogsToolResult(G8eBaseModel):
    """Result returned by ExecutionLogMixin._execute_fetch_logs."""
    success: bool = True
    error: str | None = None
    error_type: CommandErrorType | None = None
    command: str | None = None
    exit_code: int | None = None
    stdout: str | None = None
    stderr: str | None = None
    stdout_size: int = 0
    stderr_size: int = 0
    duration_ms: int | None = None
    timestamp: UTCDatetime | None = None


class AuditSessionMetadata(G8eBaseModel):
    """Session metadata returned by fetch_session_history.

    Canonical shape defined in shared/proto/operator.proto (AuditWebSession message).
    """
    id: str
    title: str
    created_at: str | None = None
    user_identity: str


class FileDiffEntry(G8eBaseModel):
    """Single file diff record from the operator ledger.

    Canonical shape defined in shared/proto/operator.proto (FileDiffEntry message).
    """
    id: str
    timestamp: str
    file_path: str
    operation: str
    ledger_hash_before: str
    ledger_hash_after: str
    diff_stat: str
    diff_content: str | None = None
    diff_size: int
    operator_session_id: str


class FetchHistoryToolResult(G8eBaseModel):
    """Result returned by AuditHistoryMixin._execute_fetch_history."""
    success: bool = True
    error: str | None = None
    error_type: CommandErrorType | None = None
    operator_session_id: str | None = None
    session: AuditSessionMetadata | None = None
    events: list[AuditEvent] = Field(default_factory=list)
    total: int = 0
    limit: int = 50
    offset: int = 0


class PerOperatorFileHistoryResult(PerOperatorResultBase):
    """Per-operator result for file history operations."""
    entries: list[FileHistoryEntry] = Field(default_factory=list)


class FetchFileHistoryToolResult(BatchExecutionMeta):
    """Result returned by AuditHistoryMixin._execute_fetch_file_history."""
    success: bool = True
    error: str | None = None
    error_type: CommandErrorType | None = None
    file_path: str | None = None
    history: list[FileHistoryEntry] = Field(default_factory=list)
    per_operator_results: list[PerOperatorFileHistoryResult] | None = None


class RestoreFileToolResult(G8eBaseModel):
    """Result returned by LedgerMirrorMixin._execute_restore_file."""
    success: bool = True
    error: str | None = None
    error_type: CommandErrorType | None = None
    file_path: str | None = None
    commit_hash: str | None = None
    message: str | None = None


class PerOperatorFileDiffResult(PerOperatorResultBase):
    """Per-operator result for file diff operations."""
    entries: list[FileDiffEntry] = Field(default_factory=list)


class FetchFileDiffToolResult(BatchExecutionMeta):
    """Result returned by LedgerMirrorMixin._execute_fetch_file_diff."""
    success: bool = True
    error: str | None = None
    error_type: CommandErrorType | None = None
    diff: FileDiffEntry | None = None
    diffs: list[FileDiffEntry] = Field(default_factory=list)
    total: int = 0
    operator_session_id: str | None = None
    per_operator_results: list[PerOperatorFileDiffResult] | None = None


class IamIntentResult(G8eBaseModel):
    """Result of a single IAM policy attach/detach operation for one intent."""
    intent: str
    result: CommandInternalResult | None = None


class FailedIntentResult(G8eBaseModel):
    """A single intent that failed during IAM policy attach/detach."""
    intent: str
    error: str


class IntentPermissionResult(G8eBaseModel):
    """Result returned by _execute_intent_permission_request and _execute_intent_revocation."""
    success: bool = True
    error: str | None = None
    error_type: CommandErrorType | None = None
    approved: bool | None = None
    intent_name: str | None = None
    all_intents: list[str] = Field(default_factory=list)
    operation_context: str | None = None
    message: str | None = None
    approval_id: str | None = None
    feedback: bool = False
    requested_intents: list[str] | None = None
    invalid_intents: list[str] | None = None
    successful_intents: list[str] | None = None
    failed_intents: list[FailedIntentResult] | None = None
    iam_results: list[IamIntentResult] | None = None
    stderr: str | None = None
    output: str | None = None
    exit_code: int | None = None
    pending_command: str | None = None
    pending_command_result: CommandInternalResult | None = None
    timestamp: UTCDatetime | None = None
    revoked_intents: list[str] | None = None


class WebSearchResultItem(G8eBaseModel):
    """A single item returned by the Google Custom Search API."""
    title: str = Field(default="", description="Result page title")
    link: str = Field(default="", description="Result page URL")
    snippet: str = Field(default="", description="Snippet of matching content")


class SearchWebResult(G8eBaseModel):
    """Result returned by the search_web tool executor."""
    success: bool = True
    error: str | None = None
    error_type: CommandErrorType | None = None
    query: str | None = None
    results: list[WebSearchResultItem] = Field(default_factory=list)
    total_results: str | None = None


class CommandConstraintsResult(G8eBaseModel):
    """Result returned by the get_command_constraints tool.

    Surfaces whitelist/blacklist policy to the AI so it can respect
    allowed/forbidden commands before proposing them.
    """
    success: bool = True
    error: str | None = None
    error_type: CommandErrorType | None = None
    whitelisting_enabled: bool = False
    blacklisting_enabled: bool = False
    auto_approve_enabled: bool = False
    whitelisted_commands: list[WhitelistedCommand] = Field(default_factory=list)
    blacklisted_commands: list[dict[str, str]] = Field(default_factory=list)
    blacklisted_substrings: list[dict[str, str]] = Field(default_factory=list)
    blacklisted_patterns: list[dict[str, str]] = Field(default_factory=list)
    auto_approved_commands: list[str] = Field(
        default_factory=list,
        description="Base commands that bypass human approval when auto_approve_enabled is true. "
                    "These commands still must pass all hard safety gates.",
    )
    auto_approved_sources: list[dict[str, str]] = Field(
        default_factory=list,
        description="Source attribution for each auto-approved command. "
                    "Each entry has 'command' (str) and 'source' ('platform' or 'user'). "
                    "Platform sources come from JSON config; user sources come from CSV override.",
    )
    global_forbidden_patterns: list[str] = Field(default_factory=list)
    global_forbidden_directories: list[str] = Field(default_factory=list)
    message: str | None = None


class InvestigationContextResult(G8eBaseModel):
    """Result returned by the query_investigation_context tool executor."""
    success: bool = True
    error: str | None = None
    error_type: CommandErrorType | None = None
    data_type: str | None = None
    data: dict[str, Any] | list[dict[str, Any]] | str | None = None
    item_count: int | None = None
    investigation_id: str | None = None


class SshInventoryToolResult(G8eBaseModel):
    """Result returned by the list_ssh_inventory tool.

    Canonical shape: shared/models/tool_results.json ssh_inventory_result.
    """
    success: bool = True
    error: str | None = None
    error_type: CommandErrorType | None = None
    source_path: str | None = None
    hosts: list[SshHost] = Field(default_factory=list)
    total_count: int = 0


class CommandExecutionResult(BatchExecutionMeta):
    """Typed result returned by _execute_g8eo_command through the entire call chain.

    Replaces all Dict[str, Any] returns from command_executor -> tool_executor -> agent.
    """
    success: bool = Field(description="Whether execution succeeded")
    error: str | None = Field(default=None)
    error_type: CommandErrorType | None = Field(default=None)

    command_executed: str | None = Field(default=None)
    justification: str | None = Field(default=None)
    expected_lines: int | None = Field(default=None)
    timeout_seconds: int | None = Field(default=None)

    output: str | None = Field(default=None)
    stderr: str | None = Field(default=None)
    exit_code: int | None = Field(default=None)
    execution_status: ExecutionStatus | None = Field(default=None)

    execution_results: list[CommandInternalResult] | None = Field(default=None)

    execution_result: CommandInternalResult | None = Field(default=None)

    denial_reason: str | None = Field(default=None)
    feedback: bool = Field(default=False)
    feedback_reason: str | None = Field(default=None)
    execution_id: str | None = Field(default=None)
    approval_id: str | None = Field(default=None)
    available_operators: int | None = Field(default=None)
    warden_risk: RiskLevel | None = Field(
        default=None,
        description="Warden-classified risk level from the approval gate, used for Tier 1 reputation resolution."
    )
    blocked_pattern: str | None = Field(default=None)
    blocked_command: str | None = Field(default=None)
    validation_details: dict[str, Any] | None = Field(default=None)
    suggestion: str | None = Field(default=None)
    rule: str | None = Field(default=None)


class TokenUsage(G8eBaseModel):
    """Token usage reported by the LLM for a streaming response."""
    input_tokens: int = 0
    output_tokens: int = 0
    total_tokens: int = 0
    thinking_tokens: int = 0
    estimated: bool | None = None
    error: str | None = None


ToolResult = Union[
    CommandExecutionResult,
    CommandConstraintsResult,
    FileEditResult,
    PortCheckToolResult,
    FsListToolResult,
    FsGrepToolResult,
    FsReadToolResult,
    FetchLogsToolResult,
    FetchHistoryToolResult,
    FetchFileHistoryToolResult,
    RestoreFileToolResult,
    FetchFileDiffToolResult,
    IntentPermissionResult,
    SearchWebResult,
    InvestigationContextResult,
    SshInventoryToolResult,
]
