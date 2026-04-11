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

from app.constants import (
    AGENT_MODE_PROMPT_FILES,
    ApprovalType,
    AttachmentType,
    AuthMethod,
    BatchWriteOpType,
    CloudIntent,
    CloudSubtype,
    ConversationStatus,
    EscalationRisk,
    ExecutionStatus,
    FileOperation,
    ToolCallStatus,
    ToolDisplayCategory,
    GroundingSource,
    HealthStatus,
    HeartbeatType,
    InfrastructureStatus,
    NetworkProtocol,
    OperatorToolName,
    OperatorStatus,
    OperatorType,
    Platform,
    Priority,
    PromptFile,
    PromptSection,
    ResponseType,
    RiskLevel,
    RiskThreshold,
    TaskStatus,
    ThinkingLevel,
    VersionStability,
    AgentMode,
)
from app.models.version import VersionInfo

from .agent import (
    AgentStreamContext,
    OperatorCommandArgs,
    OperatorContext,
    StreamChunkFromModel,
    StreamChunkData,
)
from .attachments import (
    AttachmentData,
    AttachmentMetadata,
    ProcessedAttachment,
)
from .base import VSOAuditableModel, VSOBaseModel, VSOIdentifiableModel, VSOTimestampedModel
from .cache import (
    BatchOperationResult,
    BatchWriteOperation,
    CacheContextWarmResult,
    CacheOperationResult,
    CacheWarmResult,
    DocumentResult,
    FieldFilter,
    QueryOrderBy,
    QueryResult,
)
from .cases import CaseCreateRequest, CaseEventPayload, CaseModel, CaseUpdateRequest, HistoryEntry
from .agents import (
    CandidateCommand,
    CommandGenerationResult,
    TriageResult,
    PrimaryRequest,
    PrimaryResult,
    VerifierRequest,
    VerifierResult,
    CaseTitleRequest,
    CaseTitleResult,
)
from .command_payloads import (
    CommandCancelPayload,
    CommandPayload,
    FetchFileDiffPayload,
    FetchFileHistoryPayload,
    FetchHistoryPayload,
    FetchLogsPayload,
    FileEditPayload,
    FsListPayload,
    FsReadPayload,
    RestoreFilePayload,
)
from .conversation import (
    Conversation,
)
from .grounding import (
    GroundingChunk,
    GroundingMetadata,
    GroundingSegment,
    GroundingSourceInfo,
    GroundingSupport,
    SearchEntryPoint,
)
from .auth import (
    AuthenticatedUser,
)
from .health import (
    DependencyStatus,
    HealthCheckResult,
    ServiceHealthResult,
    WorkflowHealthResult,
)
from .http_context import (
    BoundOperator,
    VSOHttpContext,
)
from .investigations import (
    AIResponseMetadata,
    ApprovalMetadata,
    Attachment,
    ConversationHistoryMessage,
    ConversationMessageMetadata,
    ConversationUpdateOperation,
    EnrichedInvestigationContext,
    FileEditMetadata,
    InvestigationCreateRequest,
    InvestigationCurrentState,
    InvestigationCustomerContext,
    InvestigationExecutionConstraints,
    InvestigationHistoryEntry,
    InvestigationModel,
    InvestigationQueryRequest,
    InvestigationTechnicalContext,
    InvestigationUpdateRequest,
    OperatorCommandMetadata,
    ThinkingMessage,
    UserChatMetadata,
)
from .memory import InvestigationMemory, MemoryAnalysis
from .model_configs import (
    GEMINI_3_FLASH_PREVIEW,
    GEMINI_3_PRO_PREVIEW,
    MODEL_REGISTRY,
    LLMModelConfig,
    LLMModelRegistry,
    get_available_models,
    get_model_config,
)
from .operators import (
    ApprovalContext,
    ApprovalRequestBase,
    ApprovalResult,
    AttachmentRecord,
    BatchCommandBroadcastEvent,
    BatchOperatorExecutionResult,
    BindingValidationResult,
    CancelCommandResult,
    CommandApprovalRequest,
    DirectCommandResult,
    FileEditApprovalRequest,
    IntentApprovalEvent,
    IntentApprovalRequest,
    OperatorDocument,
    OperatorSystemInfo,
    PendingApproval,
    SystemInfoDiskDetails,
    SystemInfoEnvironment,
    SystemInfoFingerprintDetails,
    SystemInfoMemoryDetails,
    SystemInfoUserDetails,
    TargetSystem,
)
from .operators_bind import (
    BindOperatorsRequest,
    BindOperatorsResponse,
    UnbindOperatorsRequest,
    UnbindOperatorsResponse,
)
from .pubsub_messages import (
    CancellationResultPayload,
    ExecutionResultsPayload,
    ExecutionStatusPayload,
    FetchFileDiffResultPayload,
    FetchFileHistoryResultPayload,
    FetchHistoryResultPayload,
    FetchLogsResultPayload,
    FileEditResultPayload,
    FsListResultPayload,
    FsReadResultPayload,
    NetworkConnectivityStatus,
    PortCheckResultPayload,
    RestoreFileResultPayload,
    ShutdownAckPayload,
    G8eoResultEnvelope,
    G8eoResultPayload,
    G8eoHeartbeatPayload,
    VSOMessage,
)
from .tool_results import (
    AuditEvent,
    AuditFileMutation,
    AuditSessionMetadata,
    CommandExecutionResult,
    CommandRiskAnalysis,
    CommandRiskContext,
    ErrorAnalysisContext,
    ErrorAnalysisResult,
    FailedIntentResult,
    FetchFileDiffToolResult,
    FetchFileHistoryToolResult,
    FetchHistoryToolResult,
    FetchLogsToolResult,
    FileDiffEntry,
    FileEditResult,
    FileHistoryEntry,
    FileOperationRiskAnalysis,
    FileOperationRiskContext,
    FsListEntry,
    FsListToolResult,
    FsReadToolResult,
    IamIntentResult,
    IntentPermissionResult,
    PortCheckToolResult,
    RestoreFileToolResult,
    SearchWebResult,
    TokenUsage,
    ToolResult,
)
# from .triage import TriageResult  # Removed - moved to .agents
from .vsod_client import (
    IntentOperationResult,
)
from .whitelist import (
    CommandValidationResult,
)

ApprovalRequestBase.model_rebuild()
CommandApprovalRequest.model_rebuild()
FileEditApprovalRequest.model_rebuild()
IntentApprovalRequest.model_rebuild()
