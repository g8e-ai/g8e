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

"""Service protocols for the operator services layer."""

from __future__ import annotations

from typing import Protocol, runtime_checkable, TYPE_CHECKING, Callable, Coroutine, Any

from app.constants import (
    ComponentName,
    EventType,
    FileOperation,
    OperatorStatus,
)
from app.models.base import G8eBaseModel
from app.models.cache import (
    BatchWriteOperation,
    CacheOperationResult,
    DocumentResult,
    FieldFilter,
    QueryResult,
)
from app.models.events import BackgroundEvent, SessionEvent
from app.models.http_context import G8eHttpContext
from app.models.infra import HTTPClientStatus
from app.models.investigations import (
    ConversationHistoryMessage,
    ConversationMessageMetadata,
    EnrichedInvestigationContext,
    InvestigationCreateRequest,
    InvestigationHistoryEntry,
    InvestigationModel,
    InvestigationQueryRequest,
    InvestigationUpdateRequest,
)
from app.models.memory import InvestigationMemory
from app.models.internal_api import OperatorApprovalResponse
from app.models.operators import (
    AgentContinueApprovalRequest,
    ApprovalResult,
    CommandApprovalRequest,
    CommandResultRecord,
    DirectCommandResult,
    FileEditApprovalRequest,
    IntentApprovalRequest,
    OperatorDocument,
    OperatorHeartbeat,
    PendingApproval,
    TargetSystem,
)
from app.models.internal_api import DirectCommandRequest
from app.models.pubsub_messages import G8eMessage, G8eoResultEnvelope
from app.models.tool_results import (
    CommandInternalResult,
    CommandRiskAnalysis,
    CommandRiskContext,
    ErrorAnalysisContext,
    ErrorAnalysisResult,
    FetchFileDiffToolResult,
    FetchFileHistoryToolResult,
    FileOperationRiskAnalysis,
    FileOperationRiskContext,
    FileEditResult,
    FsListToolResult,
    FsReadToolResult,
    PortCheckToolResult,
    IntentPermissionResult,
)
from app.models.tool_args import GrantIntentArgs, RevokeIntentArgs
from app.models.command_request_payloads import (
    FileEditRequestPayload,
    FetchFileDiffRequestPayload,
    FetchFileHistoryRequestPayload,
    FsListRequestPayload,
    FsReadRequestPayload,
    CheckPortRequestPayload,
)
from app.models.settings import G8eePlatformSettings, G8eeUserSettings
from app.utils.whitelist_validator import CommandWhitelistValidator
from app.utils.blacklist_validator import CommandBlacklistValidator
from app.models.tool_results import ToolResult
from app.models.g8ed_client import IntentOperationResult, SSEPushResponse
from app.constants.prompts import AgentMode
from app.llm import llm_types as types

if TYPE_CHECKING:
    from app.clients.pubsub_client import PubSubClient
    from app.clients.http_client import HTTPClient

@runtime_checkable
class SettingsServiceProtocol(Protocol):
    """Protocol for SettingsService ensuring read-only access to platform and user settings."""

    async def get_platform_settings(self) -> G8eePlatformSettings:
        """Retrieve platform-level settings from g8es with cache-aside."""
        ...

    async def get_user_settings(self, user_id: str) -> G8eeUserSettings:
        """Retrieve settings for a specific user, overlaid on platform settings."""
        ...

    def get_local_settings(self) -> G8eePlatformSettings:
        """Retrieve local bootstrap settings (bootstrap)."""
        ...


@runtime_checkable
class EventServiceProtocol(Protocol):
    """Protocol for EventService ensuring event publishing."""

    async def publish(self, event: SessionEvent | BackgroundEvent) -> str:
        """Publish a session or background event."""
        ...

    async def publish_command_event(
        self,
        event_type: EventType,
        data: G8eBaseModel,
        g8e_context: G8eHttpContext,
        *,
        task_id: str,
    ) -> None:
        """Publish a command-related event."""
        ...

    async def publish_investigation_event(
        self,
        investigation_id: str,
        event_type: EventType,
        payload: G8eBaseModel,
        web_session_id: str,
        case_id: str,
        user_id: str,
    ) -> None:
        """Publish an investigation-related event."""
        ...


@runtime_checkable
class KVServiceProtocol(Protocol):
    """Protocol for Key-Value store service."""

    async def get(self, key: str) -> str | None:
        """Retrieve a string value by key."""
        ...

    async def set(self, key: str, value: str, ex: int | None) -> bool:
        """Set a string value with optional expiration (seconds)."""
        ...

    async def delete(self, *keys: str) -> int:
        """Delete one or more keys."""
        ...

    async def get_json(self, key: str) -> object | None:
        """Retrieve and parse a JSON value by key."""
        ...

    async def set_json(self, key: str, value: object, ex: int | None = None) -> bool:
        """Serialize and set a JSON value with optional expiration."""
        ...

    async def keys(self, pattern: str = "*") -> list[str]:
        """List keys matching a pattern."""
        ...

    async def delete_pattern(self, pattern: str) -> int:
        """Delete all keys matching a pattern."""
        ...

    async def lrange(self, key: str, start: int, stop: int) -> list[object]:
        """Retrieve a range of elements from a list."""
        ...

    def is_healthy(self) -> bool:
        """Check if the service is healthy."""
        ...


@runtime_checkable
class DBServiceProtocol(Protocol):
    """Protocol for Document Store service."""

    async def create_document(
        self,
        collection: str,
        document_id: str,
        data: dict[str, object] | G8eBaseModel,
    ) -> CacheOperationResult:
        """Create a new document in a collection."""
        ...

    async def update_document(
        self,
        collection: str,
        document_id: str,
        data: dict[str, object] | G8eBaseModel,
        merge: bool = True,
    ) -> CacheOperationResult:
        """Update or replace an existing document."""
        ...

    async def delete_document(self, collection: str, document_id: str) -> CacheOperationResult:
        """Delete a document from a collection."""
        ...

    async def get_document(self, collection: str, document_id: str) -> DocumentResult:
        """Retrieve a document by ID."""
        ...

    async def query_collection(
        self,
        collection: str,
        field_filters: list[FieldFilter],
        order_by: dict[str, str],
        limit: int,
        select_fields: list[str] | None = None,
    ) -> QueryResult:
        """Query a collection with filters and ordering."""
        ...

    async def update_with_array_union(
        self,
        collection: str,
        document_id: str,
        array_field: str,
        items_to_add: list[object],
        additional_updates: dict[str, object],
    ) -> CacheOperationResult:
        """Atomically append items to an array field."""
        ...

    async def batch_write(self, operations: list[BatchWriteOperation]) -> CacheOperationResult:
        """Perform multiple write operations in batch."""
        ...

    async def close(self) -> None:
        """Close the database connection."""
        ...


@runtime_checkable
class OperatorCacheProtocol(Protocol):
    """Protocol for operator metadata caching."""

    async def get_operator(self, operator_id: str) -> OperatorDocument | None:
        """Retrieve an operator document from cache."""
        ...

    async def update_operator_status(self, operator_id: str, status: OperatorStatus) -> bool:
        """Update cached operator status."""
        ...


@runtime_checkable
class CacheAsideProtocol(Protocol):
    """Protocol for the unified CacheAside service."""

    @property
    def kv(self) -> KVServiceProtocol:
        """Access the underlying KV service."""
        ...

    @property
    def db(self) -> DBServiceProtocol:
        """Access the underlying DB service."""
        ...

    async def create_document(
        self,
        collection: str,
        document_id: str,
        data: dict[str, object] | G8eBaseModel,
        ttl: int | None = None,
    ) -> CacheOperationResult:
        """Create a document with optional caching/TTL."""
        ...

    async def update_document(
        self,
        collection: str,
        document_id: str,
        data: dict[str, object] | G8eBaseModel,
        merge: bool = True,
        ttl: int | None = None,
    ) -> CacheOperationResult:
        """Update a document with optional caching/TTL."""
        ...

    async def delete_document(self, collection: str, document_id: str) -> CacheOperationResult:
        """Delete a document from both cache and store."""
        ...

    async def get_document(self, collection: str, document_id: str) -> dict[str, object] | None:
        """Retrieve a document with cache-aside logic."""
        ...

    async def get_operator(self, operator_id: str) -> OperatorDocument | None:
        """Retrieve operator metadata with cache-aside."""
        ...

    async def update_operator_status(self, operator_id: str, status: OperatorStatus) -> bool:
        """Update operator status in both cache and store."""
        ...

    async def query_documents(
        self,
        collection: str,
        field_filters: list[dict[str, object]],
        order_by: dict[str, str],
        limit: int,
        select_fields: list[str] | None = None,
        ttl: int | None = 300,
    ) -> list[dict[str, object]]:
        """Query documents with optional result caching."""
        ...

    async def append_to_array(
        self,
        collection: str,
        document_id: str,
        array_field: str,
        items_to_add: list[object],
        additional_updates: dict[str, object],
    ) -> CacheOperationResult:
        """Append items to an array field with cache invalidation."""
        ...


@runtime_checkable
class OperatorDataServiceProtocol(Protocol):
    """Protocol for operator-specific data operations."""

    async def get_operator(self, operator_id: str) -> OperatorDocument | None:
        """Retrieve operator metadata."""
        ...

    async def query_operators(
        self,
        field_filters: list[dict[str, object]] | None = None,
        limit: int = 1000,
        bypass_cache: bool = False,
    ) -> list[OperatorDocument]:
        """Query operator documents. ``bypass_cache=True`` skips the query cache."""
        ...

    async def update_operator_status(self, operator_id: str, status: OperatorStatus) -> bool:
        """Update operator status."""
        ...

    async def update_operator_heartbeat(
        self,
        operator_id: str,
        heartbeat: OperatorHeartbeat,
        investigation_id: str | None,
        case_id: str | None,
    ) -> bool:
        """Update operator heartbeat and session status.

        investigation_id/case_id are None when the heartbeat arrives outside an
        investigation context; callers MUST NOT coerce absence to sentinel strings.
        """
        ...

    async def append_command_result(self, operator_id: str, command_result: CommandResultRecord) -> bool:
        """Append a command result to operator history."""
        ...

    async def add_operator_activity(
        self,
        operator_id: str,
        sender: str,
        content: str,
        metadata: ConversationMessageMetadata,
    ) -> bool:
        """Log operator-specific activity message."""
        ...

    async def add_operator_approval(
        self,
        operator_id: str,
        event_type: EventType,
        metadata: ConversationMessageMetadata,
    ) -> bool:
        """Log an approval lifecycle event in the operator activity log."""
        ...

    async def bind_operators(
        self,
        operator_ids: list[str],
        web_session_id: str,
        context: G8eHttpContext,
    ) -> bool:
        """Bind operators to a web session."""
        ...


@runtime_checkable
class MemoryDataServiceProtocol(Protocol):
    async def create_memory(self, investigation: InvestigationModel) -> InvestigationMemory: ...
    async def save_memory(self, memory: InvestigationMemory, is_new: bool) -> None: ...
    async def get_memory(self, investigation_id: str) -> InvestigationMemory | None: ...
    async def get_user_memories(self, user_id: str) -> list[InvestigationMemory]: ...
    async def get_case_memories(self, case_id: str, user_id: str) -> list[InvestigationMemory]: ...

@runtime_checkable
class InvestigationDataServiceProtocol(Protocol):
    async def create_investigation(self, request: InvestigationCreateRequest) -> InvestigationModel: ...
    async def get_investigation(self, investigation_id: str) -> InvestigationModel | None: ...
    async def update_investigation_raw(self, investigation_id: str, updates: dict[str, object], merge: bool = True) -> None: ...
    async def query_investigations(self, request: InvestigationQueryRequest) -> list[InvestigationModel]: ...
    async def get_case_investigations(self, case_id: str, user_id: str | None) -> list[InvestigationModel]: ...
    async def delete_investigation(self, investigation_id: str) -> None: ...
    async def get_chat_messages(self, investigation_id: str) -> list[ConversationHistoryMessage]: ...
    async def add_chat_message(
        self,
        investigation_id: str | None,
        sender: str,
        content: str,
        metadata: ConversationMessageMetadata,
    ) -> bool: ...
    async def add_history_entry(
        self,
        investigation_id: str,
        event_type: EventType,
        actor: ComponentName,
        summary: str,
        details: ConversationMessageMetadata,
    ) -> InvestigationModel: ...
    async def add_approval_record(
        self,
        investigation_id: str,
        event_type: EventType,
        metadata: ConversationMessageMetadata,
        actor: ComponentName = ComponentName.G8EE,
    ) -> InvestigationModel: ...
    async def add_command_execution_result(
        self,
        investigation_id: str,
        execution_id: str,
        command: str,
        result: CommandInternalResult,
        operator_id: str,
        operator_session_id: str,
    ) -> InvestigationModel: ...
    async def add_file_operation_result(
        self,
        investigation_id: str,
        execution_id: str,
        operator_id: str,
        event_type: EventType,
        file_path: str,
        result: FileEditResult,
        operation: FileOperation,
    ) -> InvestigationModel: ...
    async def get_command_execution_history(self, investigation_id: str) -> list[InvestigationHistoryEntry]: ...
    async def get_operator_actions_for_ai_context(self, investigation_id: str) -> str: ...

@runtime_checkable
class InvestigationServiceProtocol(Protocol):
    @property
    def investigation_data_service(self) -> InvestigationDataServiceProtocol: ...
    async def get_investigation_context(self, case_id: str | None = None, investigation_id: str | None = None, user_id: str | None = None) -> EnrichedInvestigationContext: ...
    async def get_enriched_investigation_context(self, investigation: EnrichedInvestigationContext, user_id: str, g8e_context: G8eHttpContext) -> EnrichedInvestigationContext: ...
    async def get_investigation(self, investigation_id: str) -> InvestigationModel | None: ...
    async def get_chat_messages(self, investigation_id: str) -> list[ConversationHistoryMessage]: ...
    async def update_investigation_raw(self, investigation_id: str, updates: dict[str, object], merge: bool = True) -> None: ...
    async def update_investigation(self, investigation_id: str, request: InvestigationUpdateRequest, actor: ComponentName = ComponentName.G8EE) -> InvestigationModel: ...
    async def delete_investigation(self, investigation_id: str) -> None: ...
    async def add_history_entry(
        self,
        investigation_id: str,
        event_type: EventType,
        actor: ComponentName,
        summary: str,
        details: ConversationMessageMetadata,
    ) -> InvestigationModel: ...
    async def add_command_execution_result(
        self,
        investigation_id: str,
        execution_id: str,
        command: str,
        result: CommandInternalResult,
        operator_id: str,
        operator_session_id: str,
        actor: ComponentName = ComponentName.G8EO,
    ) -> InvestigationModel: ...
    async def add_file_operation_result(
        self,
        investigation_id: str,
        execution_id: str,
        operator_id: str,
        event_type: EventType,
        file_path: str,
        result: FileEditResult,
        operation: FileOperation,
        operator_session_id: str,
    ) -> InvestigationModel: ...
    async def add_chat_message(
        self,
        investigation_id: str | None,
        sender: str,
        content: str,
        metadata: ConversationMessageMetadata,
    ) -> bool: ...
    async def add_approval_record(
        self,
        investigation_id: str,
        event_type: EventType,
        metadata: ConversationMessageMetadata,
        actor: ComponentName = ComponentName.G8EE,
    ) -> InvestigationModel: ...
    async def get_command_execution_history(self, investigation_id: str) -> list[InvestigationHistoryEntry]: ...
    async def get_operator_actions_for_ai_context(self, investigation_id: str) -> str: ...


@runtime_checkable
class G8edClientProtocol(Protocol):
    async def push_sse_event(self, event: SessionEvent | BackgroundEvent) -> SSEPushResponse: ...
    async def grant_intent(self, operator_id: str, intent: str, context: G8eHttpContext) -> IntentOperationResult: ...
    async def revoke_intent(self, operator_id: str, intent: str, context: G8eHttpContext) -> IntentOperationResult: ...

@runtime_checkable
class AIResponseAnalyzerProtocol(Protocol):
    async def analyze_command_risk(self, command: str, justification: str, context: CommandRiskContext, settings: G8eeUserSettings) -> CommandRiskAnalysis: ...
    async def analyze_error_and_suggest_fix(self, command: str, exit_code: int | None, stdout: str, stderr: str, context: ErrorAnalysisContext, settings: G8eeUserSettings) -> ErrorAnalysisResult: ...
    async def analyze_file_operation_risk(self, operation: FileOperation, file_path: str, content: str | None, context: FileOperationRiskContext, settings: G8eeUserSettings) -> FileOperationRiskAnalysis: ...

@runtime_checkable
class PubSubServiceProtocol(Protocol):
    pubsub_client: PubSubClient | None
    @property
    def is_ready(self) -> bool: ...
    def set_pubsub_client(self, client: PubSubClient) -> None: ...
    def register_future(self, execution_id: str) -> asyncio.Future[G8eoResultEnvelope]: ...
    def release_future(self, execution_id: str) -> None: ...
    async def start(self) -> None: ...
    async def stop(self) -> None: ...
    async def register_operator_session(self, operator_id: str, operator_session_id: str) -> None: ...
    async def deregister_operator_session(self, operator_id: str, operator_session_id: str) -> None: ...
    async def publish_command(self, operator_id: str, operator_session_id: str, command_data: G8eMessage) -> int: ...

@runtime_checkable
class OperatorHeartbeatServiceProtocol(Protocol):
    async def start(self) -> None: ...
    async def stop(self) -> None: ...
    async def register_operator_session(self, operator_id: str, operator_session_id: str) -> None: ...
    async def deregister_operator_session(self, operator_id: str, operator_session_id: str) -> None: ...
    def set_pubsub_client(self, client: PubSubClient) -> None: ...

@runtime_checkable
class ApprovalServiceProtocol(Protocol):
    async def request_command_approval(self, request: CommandApprovalRequest) -> ApprovalResult: ...
    async def request_file_edit_approval(self, request: FileEditApprovalRequest) -> ApprovalResult: ...
    async def request_intent_approval(self, request: IntentApprovalRequest) -> ApprovalResult: ...
    async def request_agent_continue_approval(self, request: AgentContinueApprovalRequest) -> ApprovalResult: ...
    async def handle_approval_response(self, response: OperatorApprovalResponse) -> None: ...
    def get_pending_approvals(self) -> dict[str, PendingApproval]: ...
    def mark_pending_approvals_as_feedback(self, investigation_id: str, user_message: str, user_id: str) -> int: ...

@runtime_checkable
class ExecutionServiceProtocol(Protocol):
    g8ed_event_service: EventServiceProtocol
    ai_response_analyzer: AIResponseAnalyzerProtocol
    whitelist_validator: CommandWhitelistValidator
    blacklist_validator: CommandBlacklistValidator
    async def execute(
        self,
        g8e_message: G8eMessage,
        g8e_context: G8eHttpContext,
        timeout_seconds: int = 60,
    ) -> tuple[CommandInternalResult, G8eoResultEnvelope | None]: ...
    def resolve_target_operator(self, operator_documents: list[OperatorDocument], target_operator: str | None, tool_name: str | None = None) -> OperatorDocument: ...
    def resolve_multiple_operators(self, operator_documents: list[OperatorDocument], target_operators: list[str]) -> list[OperatorDocument]: ...
    def build_target_systems_list(self, operator_documents: list[OperatorDocument]) -> list[TargetSystem]: ...
    async def send_command_to_operator(
        self,
        command_payload: DirectCommandRequest,
        g8e_context: G8eHttpContext,
    ) -> DirectCommandResult: ...

@runtime_checkable
class LFAAServiceProtocol(Protocol):
    async def send_audit_event(
        self,
        g8e_message: G8eMessage,
    ) -> bool: ...

    async def send_direct_exec_audit_event(
        self,
        command: str,
        execution_id: str,
        g8e_context: G8eHttpContext,
    ) -> bool: ...

@runtime_checkable
class FileServiceProtocol(Protocol):
    async def execute_file_edit(self, args: FileEditRequestPayload, g8e_context: G8eHttpContext, investigation: EnrichedInvestigationContext) -> FileEditResult: ...
    async def execute_fetch_file_history(self, args: FetchFileHistoryRequestPayload, g8e_context: G8eHttpContext, investigation: EnrichedInvestigationContext) -> FetchFileHistoryToolResult: ...
    async def execute_fetch_file_diff(self, args: FetchFileDiffRequestPayload, g8e_context: G8eHttpContext, investigation: EnrichedInvestigationContext) -> FetchFileDiffToolResult: ...

@runtime_checkable
class FilesystemServiceProtocol(Protocol):
    async def execute_fs_list(self, args: FsListRequestPayload, investigation: EnrichedInvestigationContext, g8e_context: G8eHttpContext) -> FsListToolResult: ...
    async def execute_file_read(self, args: FsReadRequestPayload, investigation: EnrichedInvestigationContext, g8e_context: G8eHttpContext) -> FsReadToolResult: ...

@runtime_checkable
class IntentServiceProtocol(Protocol):
    async def execute_intent_permission_request(
        self,
        *,
        args: GrantIntentArgs,
        g8e_context: G8eHttpContext,
        investigation: EnrichedInvestigationContext,
    ) -> IntentPermissionResult: ...
    async def execute_intent_revocation(
        self,
        *,
        args: RevokeIntentArgs,
        g8e_context: G8eHttpContext,
        investigation: EnrichedInvestigationContext
    ) -> IntentPermissionResult: ...
    def _resolve_intent_dependencies(self, requested_intents: list[str]) -> list[str]: ...

@runtime_checkable
class PortServiceProtocol(Protocol):
    async def execute_port_check(self, args: CheckPortRequestPayload, investigation: EnrichedInvestigationContext, g8e_context: G8eHttpContext) -> PortCheckToolResult: ...

@runtime_checkable
class ResultHandlerServiceProtocol(Protocol):
    async def handle(self, envelope: G8eoResultEnvelope) -> None: ...

@runtime_checkable
class ToolExecutorProtocol(Protocol):
    """Protocol for AI tool registration and execution."""
    def get_tools(self, agent_mode: AgentMode, model_to_use: str | None) -> list[types.ToolGroup]: ...
    async def execute_tool_call(
        self,
        tool_name: str,
        tool_args: dict[str, Any],
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
        request_settings: G8eeUserSettings,
        execution_id: str,
    ) -> ToolResult: ...
    @property
    def web_search_provider(self) -> Any: ...

@runtime_checkable
class HTTPServiceProtocol(Protocol):
    """Protocol for HTTP service managing client lifecycle and connections."""
    
    @property
    def is_ready(self) -> bool: ...
    
    def set_http_client(self, client: HTTPClient, service_name: str) -> None: ...
    
    def get_client(self, service_name: str) -> HTTPClient | None: ...
    
    async def start(self) -> None: ...
    
    async def stop(self) -> None: ...
    
    async def register_service_client(
        self, 
        service_name: str, 
        client: HTTPClient
    ) -> None: ...
    
    async def deregister_service_client(self, service_name: str) -> None: ...
    
    def list_active_clients(self) -> list[str]: ...

    def get_client_status(self) -> dict[str, HTTPClientStatus]: ...
