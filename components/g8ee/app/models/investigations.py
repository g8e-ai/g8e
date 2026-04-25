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

from __future__ import annotations

import hashlib
import json
import logging


from pydantic import ConfigDict, Field, field_validator

from app.constants import (
    ComponentName,
    ComponentStatus,
    EscalationRisk,
    ExecutionStatus,
    FileOperation,
    EventType,
    InvestigationStatus,
    Priority,
    RiskThreshold,
    Severity,
)
from app.utils.timestamp import now

from .base import G8eBaseModel, G8eIdentifiableModel, UTCDatetime
from .grounding import GroundingMetadata
from .http_context import BoundOperator, G8eHttpContext
from .memory import InvestigationMemory
from .operators import OperatorDocument
from .tool_results import TokenUsage


class ConversationMessageMetadata(G8eBaseModel):
    """Base typed metadata for a conversation message.

    Use a typed subclass when the message category is known:
    - UserChatMetadata       — EventType.EVENT_SOURCE_USER_CHAT messages
    - AIResponseMetadata     — EventType.EVENT_SOURCE_AI_PRIMARY / EventType.EVENT_SOURCE_AI_ASSISTANT messages
    - OperatorCommandMetadata — EventType.EVENT_SOURCE_USER_TERMINAL command execution messages
    - ApprovalMetadata       — approval request/response messages
    - FileEditMetadata       — file edit operation messages
    - SystemMetadata         — EventType.EVENT_SOURCE_SYSTEM / system notification messages

    The base class is kept for backward compat and for cases where the category
    cannot be statically determined (e.g. deserialization from DB).
    """
    event_type: EventType | None = Field(default=None, description="Event type for frontend filtering")
    execution_id: str | None = Field(default=None, description="Operator execution ID")
    command: str | None = Field(default=None, description="Operator command string")
    status: ExecutionStatus | None = Field(default=None, description="Execution status")
    is_thinking: bool | None = Field(default=None, description="Whether this is an AI thinking message")
    source: EventType | None = Field(default=None, description="AI response source (EventType.SOURCE_* only)")
    approval_id: str | None = Field(default=None, description="Approval request ID")
    hostname: str | None = Field(default=None, description="Operator hostname")
    direct_execution: bool | None = Field(default=None, description="Whether this was a direct terminal execution")
    justification: str | None = Field(default=None, description="Justification for the command")
    model: str | None = Field(default=None, description="AI model used")
    tokens: int | None = Field(default=None, description="Token count")
    has_thinking: bool | None = Field(default=None, description="Whether this message has embedded thinking content")
    thinking_content: str | None = Field(default=None, description="Embedded AI thinking content")
    response_source: EventType | None = Field(default=None, description="Source of the AI response")
    approved: bool | None = Field(default=None, description="Whether the approval was granted")
    reason: str | None = Field(default=None, description="Approval decision reason or feedback")
    feedback_reason: str | None = Field(default=None, description="User feedback reason when bypassing approval")
    is_batch_execution: bool | None = Field(default=None, description="Whether this targets multiple operators")
    batch_id: str | None = Field(default=None, description="Batch correlation ID when the approval covers multiple operators")
    file_path: str | None = Field(default=None, description="File path for file edit operations")
    operation: FileOperation | None = Field(default=None, description="File operation type")
    intent_name: str | None = Field(default=None, description="AWS intent name for intent approvals")
    intent_question: str | None = Field(default=None, description="User-facing question for intent approval")
    requested_at: UTCDatetime | None = Field(default=None, description="When the approval was requested")
    responded_at: UTCDatetime | None = Field(default=None, description="When the approval response was received")
    completed_at: UTCDatetime | None = Field(default=None, description="When the operation completed")
    backup_path: str | None = Field(default=None, description="Backup file path for file edit operations")
    error: str | None = Field(default=None, description="Error message if operation failed")
    error_type: str | None = Field(default=None, description="Structured error type classification")
    exit_code: int | None = Field(default=None, description="Command exit code")
    output: str | None = Field(default=None, description="Command output")
    execution_time_seconds: float | None = Field(default=None, description="Command execution duration in seconds")
    timeout_seconds: int | None = Field(default=None, description="Timeout value for the operation")
    grounding_metadata: GroundingMetadata | None = Field(default=None, description="Grounding metadata from AI response")
    token_usage: TokenUsage | None = Field(default=None, description="Token usage stats from AI response")
    sentinel_mode: bool | None = Field(default=None, description="Sentinel mode active when message was created")
    attachment_filenames: list[str] = Field(default_factory=list, description="Filenames of attachments sent with this message")


class UserChatMetadata(ConversationMessageMetadata):
    """Metadata for user-entered chat messages (EventType.EVENT_SOURCE_USER_CHAT).

    User chat is the source of truth for what the user typed. It must never
    carry execution IDs, commands, or AI-routing fields — those belong to
    OperatorCommandMetadata or AIResponseMetadata respectively.
    """
    sentinel_mode: bool | None = Field(default=None, description="Sentinel mode active when message was sent")
    attachment_filenames: list[str] = Field(default_factory=list, description="Filenames of attachments sent with this message")


class AIResponseMetadata(ConversationMessageMetadata):
    """Metadata for AI-generated response messages (EventType.EVENT_SOURCE_AI_PRIMARY / EVENT_SOURCE_AI_ASSISTANT).

    The `source` field here uses EventType.SOURCE_* — the attribution of which AI
    system produced the response. 
    """
    source: EventType | None = Field(default=None, description="AI response attribution (source_ai, source_tool_call)")
    response_source: EventType | None = Field(default=None, description="Source of the AI response")
    model: str | None = Field(default=None, description="AI model that produced this response")
    tokens: int | None = Field(default=None, description="Token count for this response")
    has_thinking: bool | None = Field(default=None, description="Whether this message has embedded thinking content")
    thinking_content: str | None = Field(default=None, description="Embedded AI thinking content")
    is_thinking: bool | None = Field(default=None, description="Whether this is an AI thinking message")
    grounding_metadata: GroundingMetadata | None = Field(default=None, description="Grounding metadata from AI response")
    token_usage: TokenUsage | None = Field(default=None, description="Token usage stats from AI response")


class OperatorCommandMetadata(ConversationMessageMetadata):
    """Metadata for AI-initiated operator command execution messages.

    These are commands dispatched by the AI agent, not entered by the user.
    For user-entered terminal commands, use UserRunCommandMetadata.

    The command source is implied by EventType.EVENT_SOURCE_USER_TERMINAL + direct_execution=False.
    """
    execution_id: str | None = Field(default=None, description="Operator execution ID")
    command: str | None = Field(default=None, description="Operator command string")
    status: ExecutionStatus | None = Field(default=None, description="Execution status")
    exit_code: int | None = Field(default=None, description="Command exit code")
    hostname: str | None = Field(default=None, description="Operator hostname")
    direct_execution: bool | None = Field(default=False, description="Always False — AI-initiated, not user-entered")
    approval_id: str | None = Field(default=None, description="Approval ID if approval was required")
    justification: str | None = Field(default=None, description="AI justification for this command")
    execution_time_seconds: float | None = Field(default=None, description="Command execution duration in seconds")
    error: str | None = Field(default=None, description="Error message if command failed")
    error_type: str | None = Field(default=None, description="Structured error type classification")
    requested_at: UTCDatetime | None = Field(default=None, description="When the command was requested")
    completed_at: UTCDatetime | None = Field(default=None, description="When the command completed")


class ApprovalMetadata(ConversationMessageMetadata):
    """Metadata for approval request/response messages."""
    execution_id: str | None = Field(default=None, description="Associated execution ID")
    approval_id: str | None = Field(default=None, description="Approval request ID")
    command: str | None = Field(default=None, description="Command awaiting approval")
    justification: str | None = Field(default=None, description="Justification provided for approval")
    approved: bool | None = Field(default=None, description="Whether the approval was granted")
    reason: str | None = Field(default=None, description="Approval decision reason")
    feedback_reason: str | None = Field(default=None, description="User feedback reason when bypassing approval")
    is_batch_execution: bool | None = Field(default=None, description="Whether this targets multiple operators")
    batch_id: str | None = Field(default=None, description="Batch correlation ID when the approval covers multiple operators")
    requested_at: UTCDatetime | None = Field(default=None, description="When the approval was requested")
    responded_at: UTCDatetime | None = Field(default=None, description="When the approval response was received")
    intent_name: str | None = Field(default=None, description="AWS intent name for intent approvals")
    intent_question: str | None = Field(default=None, description="User-facing question for intent approval")


class FileEditMetadata(ConversationMessageMetadata):
    """Metadata for file edit operation messages."""
    execution_id: str | None = Field(default=None, description="Associated execution ID")
    file_path: str | None = Field(default=None, description="File path being edited")
    operation: FileOperation | None = Field(default=None, description="File operation type")
    status: ExecutionStatus | None = Field(default=None, description="Operation status")
    backup_path: str | None = Field(default=None, description="Backup file path")
    error: str | None = Field(default=None, description="Error message if operation failed")
    error_type: str | None = Field(default=None, description="Structured error type classification")
    requested_at: UTCDatetime | None = Field(default=None, description="When the file edit was requested")
    completed_at: UTCDatetime | None = Field(default=None, description="When the file edit completed")
    timeout_seconds: int | None = Field(default=None, description="Timeout value for the operation")
    approval_id: str | None = Field(default=None, description="Approval ID if approval was required")


class ConversationHistoryMessage(G8eIdentifiableModel):
    """Single message in investigation conversation history."""
    sender: str = Field(..., description="Message sender path, e.g. user.chat")
    content: str = Field(default="", description="Message content")
    timestamp: UTCDatetime = Field(default_factory=now, description="When the message was sent")
    metadata: ConversationMessageMetadata = Field(default_factory=ConversationMessageMetadata, description="Message metadata")
    prev_hash: str | None = Field(default=None, description="Hash of previous entry in the chain (hex SHA256, 64 chars)")
    entry_hash: str | None = Field(default=None, description="Hash of this entry (hex SHA256, 64 chars)")
    hash: str | None = Field(default=None, description="Legacy hash field - deprecated, use entry_hash instead")

    def calculate_hash(self, previous_hash: str | None = None) -> str:
        """Calculate the cryptographic hash for this block (message).
        
        The hash is derived from:
        - Content
        - Sender
        - Timestamp
        - Metadata (serialized)
        - Previous block hash (forming the chain)
        """
        hasher = hashlib.sha256()
        
        # Consistent ordering for metadata serialization
        metadata_json = self.metadata.model_dump_json()
        
        hasher.update(str(self.content).encode('utf-8'))
        hasher.update(str(self.sender).encode('utf-8'))
        hasher.update(str(self.timestamp.isoformat()).encode('utf-8'))
        hasher.update(metadata_json.encode('utf-8'))
        
        if previous_hash:
            hasher.update(previous_hash.encode('utf-8'))
            
        return hasher.hexdigest()


class ThinkingMessage(G8eBaseModel):
    """A single AI thinking entry extracted from a conversation session."""

    timestamp: UTCDatetime = Field(..., description="Timestamp of the source message")
    sender: EventType = Field(..., description="Message sender")
    thinking_content: str = Field(..., description="Sanitized/scrubbed thinking text")
    final_response: str = Field(default="", description="Corresponding final response, if any")
    response_source: EventType = Field(..., description="Source of the response")


class Attachment(G8eIdentifiableModel):
    """File attachment metadata for investigations."""
    filename: str = Field(..., description="Original filename")
    content_type: str | None = Field(default=None, description="MIME content type")
    size: int | None = Field(default=None, ge=0, description="File size in bytes")
    uploaded_by: str | None = Field(default=None, description="User who uploaded the file")


class InvestigationExecutionConstraints(G8eBaseModel):
    """Execution constraints for investigation work."""
    max_execution_time_seconds: int = Field(default=1800, ge=60, le=7200, description="Maximum execution time")
    allowed_commands: list[str] | None = Field(default=None, description="Allowed command patterns")
    restricted_namespaces: list[str] | None = Field(default=None, description="Restricted Kubernetes namespaces")
    auto_execute_risk_threshold: RiskThreshold = Field(default=RiskThreshold.LOW, description="Auto-execution risk threshold")
    require_approval_for: list[str] = Field(default_factory=list, description="Actions requiring approval")
    enable_runtime_execution: bool = Field(default=True, description="Whether runtime execution is enabled")


class InvestigationCustomerContext(G8eBaseModel):
    """Customer context for investigation."""
    severity: Severity = Field(default=Severity.MEDIUM, description="Business impact level")
    affected_users: int | None = Field(default=None, ge=0, description="Number of affected users")
    downtime_active: bool = Field(default=False, description="Whether downtime is currently active")
    compliance_requirements: list[str] | None = Field(default=None, description="Compliance requirements")


class InvestigationTechnicalContext(G8eBaseModel):
    """Technical context for investigation."""
    primary_technology: str | None = Field(default=None, description="Primary technology stack")
    related_systems: list[str] = Field(default_factory=list, description="Related systems")
    error_patterns: list[str] = Field(default_factory=list, description="Known error patterns")
    log_sources: list[str] = Field(default_factory=list, description="Log source paths")


class InvestigationCurrentState(G8eBaseModel):
    """Current state tracking for investigation."""
    active_attempt: int = Field(default=1, ge=1, description="Current attempt number")
    pending_actions: list[str] = Field(default_factory=list, description="Pending actions")
    next_deadline: UTCDatetime | None = Field(default=None, description="Next deadline")
    escalation_risk: EscalationRisk = Field(default=EscalationRisk.LOW, description="Escalation risk level")
    collaboration_status: dict[ComponentName, ComponentStatus] = Field(default_factory=dict, description="Component collaboration status")


class ConversationUpdateOperation(G8eBaseModel):
    """Typed operation for batch_update_conversation_history."""
    investigation_id: str = Field(..., description="Target investigation ID")
    message: ConversationHistoryMessage = Field(..., description="Message to append")
    case_id: str | None = Field(default=None, description="Associated case ID")


class InvestigationHistoryEntry(G8eBaseModel):
    """Single entry in investigation history trail."""
    attempt_number: int = Field(..., ge=1, description="Attempt number for this entry")
    timestamp: UTCDatetime = Field(default_factory=now, description="When this event occurred")
    event_type: EventType = Field(..., description="Type of event")
    actor: ComponentName = Field(..., description="Who performed this action")
    summary: str = Field(..., description="Brief summary of what happened")
    investigation_attempt: G8eBaseModel | None = Field(default=None, description="Investigation attempt data")
    details: ConversationMessageMetadata = Field(default_factory=ConversationMessageMetadata, description="Detailed event information")
    prev_hash: str | None = Field(default=None, description="Hash of previous entry in the chain (hex SHA256, 64 chars)")
    entry_hash: str | None = Field(default=None, description="Hash of this entry (hex SHA256, 64 chars)")


class InvestigationModel(G8eIdentifiableModel):
    """Investigation model representing collaborative AI agent work on technical issues."""

    case_id: str = Field(..., description="Associated case ID")
    case_title: str = Field(default="", description="Associated case title")
    case_description: str = Field(default="", description="Associated case description")
    task_id: str | None = Field(default=None, description="Associated task ID that triggered this investigation (optional for modular workflow)")
    web_session_id: str | None = Field(default=None, description="Associated session ID (optional for chat-initiated investigations)")
    user_email: str | None = Field(default=None, description="Email of the user who initiated the investigation")
    user_id: str = Field(..., description="ID of the user who initiated the investigation")
    organization_id: str | None = Field(default=None, description="Organization ID for multi-tenant data isolation")
    status: InvestigationStatus = Field(default=InvestigationStatus.OPEN, description="Current investigation status")
    priority: Priority = Field(default=Priority.MEDIUM, description="Investigation priority")
    severity: Severity = Field(default=Severity.MEDIUM, description="Case severity")
    sentinel_mode: bool = Field(
        ...,
        description="Sentinel mode - when True, data is scrubbed before storage and AI sees redacted data. Must be explicitly set."
    )
    customer_context: InvestigationCustomerContext | None = Field(default=None, description="Customer context")
    technical_context: InvestigationTechnicalContext | None = Field(default=None, description="Technical context")
    created_with_case: bool = Field(default=False, description="Whether this investigation was created together with its case")
    case_source: str | None = Field(default=None, description="Source of the case that triggered this investigation")
    attachments: list[Attachment] = Field(default_factory=list, description="List of file attachment metadata")
    conversation_history: list[ConversationHistoryMessage] = Field(default_factory=list, description="Sequential chat messages managed by g8ee - includes all user and AI messages including proposed solutions")
    history_trail: list[InvestigationHistoryEntry] = Field(default_factory=list, description="Task creation/completion events managed by g8ee - does NOT include conversation data")
    current_state: InvestigationCurrentState | None = Field(default=None, description="Current state tracking")

    @field_validator("user_id", mode="before")
    @classmethod
    def validate_user_id(cls, v):
        if v is None or (isinstance(v, str) and len(v.strip()) == 0):
            # Backward compatibility for old records missing user_id
            return "unknown"
        return v

    @field_validator("sentinel_mode", mode="before")
    @classmethod
    def validate_sentinel_mode(cls, v):
        if v is None:
            # Backward compatibility for old records - default to enabled for data protection
            return True
        return bool(v)

    @field_validator("case_id", mode="before")
    @classmethod
    def validate_case_id(cls, v):
        if not v or not isinstance(v, str) or len(v.strip()) == 0:
            raise ValueError("case_id is required and cannot be empty")
        return v.strip()

    @field_validator("task_id", mode="before")
    @classmethod
    def validate_task_id(cls, v):
        if not v or (isinstance(v, str) and len(v.strip()) == 0):
            return None
        if isinstance(v, str):
            return v.strip()
        return v

    @field_validator("priority", mode="before")
    @classmethod
    def validate_priority(cls, v):
        if isinstance(v, Priority):
            return v
        if isinstance(v, int):
            try:
                return Priority(v)
            except ValueError:
                raise ValueError(f"Invalid priority integer: {v}")
        if isinstance(v, str):
            try:
                return Priority(int(v))
            except (ValueError, KeyError):
                pass
            try:
                return Priority(v)
            except ValueError:
                raise ValueError(f"Invalid priority: {v}")
        raise ValueError(f"Priority must be string, int, or Priority enum, got {type(v)}")

    @field_validator("severity", mode="before")
    @classmethod
    def validate_severity(cls, v):
        if isinstance(v, Severity):
            return v
        if isinstance(v, int):
            try:
                return Severity(v)
            except ValueError:
                raise ValueError(f"Invalid severity integer: {v}")
        if isinstance(v, str):
            try:
                return Severity(int(v))
            except (ValueError, KeyError):
                pass
            try:
                return Severity(v)
            except ValueError:
                raise ValueError(f"Invalid severity: {v}")
        raise ValueError(f"Severity must be string, int, or Severity enum, got {type(v)}")

    def add_history_entry(
        self,
        event_type: EventType,
        actor: ComponentName,
        summary: str,
        attempt_number: int | None = None,
        investigation_attempt: G8eBaseModel | None = None,
        details: ConversationMessageMetadata | None = None
    ) -> None:
        from app.utils.ledger_hash import compute_entry_hash, genesis_hash

        if attempt_number is None:
            attempt_number = (self.current_state.active_attempt if self.current_state else 1)

        prev_hash = self.history_trail[-1].entry_hash if self.history_trail else None
        if not prev_hash:
            prev_hash = genesis_hash(self.id, self.created_at.isoformat())

        entry = InvestigationHistoryEntry(
            attempt_number=attempt_number,
            event_type=event_type,
            actor=actor,
            summary=summary,
            investigation_attempt=investigation_attempt,
            details=details or ConversationMessageMetadata(),
            timestamp=now(),
            prev_hash=prev_hash,
        )

        entry_dict = entry.model_dump(mode="json", exclude={"entry_hash"})
        entry.entry_hash = compute_entry_hash(entry_dict, prev_hash)

        self.history_trail.append(entry)
        self.update_timestamp()

    def update_status(self, new_status: InvestigationStatus, actor: ComponentName, summary: str) -> None:
        old_status = self.status
        self.status = new_status

        _status_event = {
            InvestigationStatus.OPEN:      EventType.INVESTIGATION_STATUS_UPDATED_OPEN,
            InvestigationStatus.CLOSED:    EventType.INVESTIGATION_STATUS_UPDATED_CLOSED,
            InvestigationStatus.ESCALATED: EventType.INVESTIGATION_STATUS_UPDATED_ESCALATED,
            InvestigationStatus.RESOLVED:  EventType.INVESTIGATION_STATUS_UPDATED_RESOLVED,
        }

        self.add_history_entry(
            event_type=_status_event[new_status],
            actor=actor,
            summary=summary,
            details={
                "old_status": old_status,
                "new_status": new_status
            }
        )


class InvestigationCreateRequest(G8eBaseModel):
    """Request model for creating new investigations."""
    case_id: str = Field(..., description="Associated case ID")
    case_title: str = Field(..., description="Associated case title")
    case_description: str = Field(..., description="Associated case description")
    task_id: str | None = Field(default=None, description="Associated task ID that triggered this investigation")
    web_session_id: str | None = Field(default=None, description="Associated session ID (optional for modular workflow)")
    priority: Priority = Field(default=Priority.MEDIUM, description="Investigation priority")
    severity: Severity = Field(default=Severity.MEDIUM, description="Investigation severity")
    user_email: str | None = Field(default=None, description="Email of the user creating the investigation")
    user_id: str = Field(..., description="ID of the user creating the investigation")
    customer_context: InvestigationCustomerContext | None = Field(default=None, description="Customer context")
    technical_context: InvestigationTechnicalContext | None = Field(default=None, description="Technical context")
    created_with_case: bool = Field(default=False, description="Whether this investigation was created together with its case")
    case_source: str | None = Field(default=None, description="Source of the case that triggered this investigation")
    attachments: list[str] = Field(default_factory=list, description="List of Google Cloud Storage URLs for attached files")
    sentinel_mode: bool = Field(
        default=True,
        description="Sentinel mode - when True, data is scrubbed before storage and AI sees redacted data. Default: enabled for data protection."
    )


class InvestigationUpdateRequest(G8eBaseModel):
    """Request model for updating investigations."""
    status: InvestigationStatus | None = Field(default=None, description="New status")
    priority: Priority | None = Field(default=None, description="Updated priority")
    case_title: str | None = Field(default=None, description="Updated case title (synced from Case document)")
    customer_context: InvestigationCustomerContext | None = Field(default=None, description="Updated customer context")
    technical_context: InvestigationTechnicalContext | None = Field(default=None, description="Updated technical context")
    sentinel_mode: bool | None = Field(
        None,
        description="Sentinel mode - when True, data is scrubbed before storage and AI sees redacted data."
    )


class InvestigationQueryRequest(G8eBaseModel):
    """Request model for querying investigations."""
    case_id: str | None = Field(default=None, description="Filter by case ID")
    task_id: str | None = Field(default=None, description="Filter by task ID")
    web_session_id: str | None = Field(default=None, description="Filter by session ID")
    user_id: str | None = Field(default=None, description="Filter by user ID")
    status: InvestigationStatus | None = Field(default=None, description="Filter by status")
    priority: Priority | None = Field(default=None, description="Filter by priority")

    limit: int = Field(default=20, ge=1, le=100, description="Maximum results to return")
    order_by: str = Field(default="created_at", description="Field to order by")
    order_direction: str = Field(default="desc", pattern="^(asc|desc)$", description="Order direction")


class EnrichedInvestigationContext(InvestigationModel):
    """InvestigationModel enriched with runtime context for AI processing.

    Produced by ``InvestigationService.get_enriched_investigation_context()``.
    Extends the persisted ``InvestigationModel`` with transient runtime fields
    (operator details, availability, memory) that are never written to the database.
    """

    model_config = ConfigDict(arbitrary_types_allowed=True)

    operator_documents: list[OperatorDocument] = Field(default_factory=list, description="Bound OperatorDocument instances")
    memory: InvestigationMemory | None = Field(default=None, description="Attached InvestigationMemory for AI context")
    bound_operators: list[BoundOperator] = Field(default_factory=list, description="BoundOperator instances from G8eHttpContext")
    operator_session_token: str | None = Field(default=None, description="Transient operator session token for authorization validation")

    @property
    def investigation_id(self) -> str:
        """Alias for id to maintain compatibility with agent code."""
        return self.id

    @property
    def g8e_context(self) -> G8eHttpContext:
        """Create a G8eHttpContext from this investigation context for agent compatibility."""
        from app.constants import ComponentName
        
        return G8eHttpContext(
            web_session_id=self.web_session_id,
            user_id=self.user_id,
            organization_id=self.organization_id,
            case_id=self.case_id,
            investigation_id=self.id,
            task_id=self.task_id,
            bound_operators=self.bound_operators,
            source_component=ComponentName.G8EE,
        )


EnrichedInvestigationContext.model_rebuild()
