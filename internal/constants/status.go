// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

// ActionStatus is a typed string for action status.
type ActionStatus string

const (
	ActionStatusCancelled     ActionStatus = "cancelled"
	ActionStatusCompleted     ActionStatus = "completed"
	ActionStatusFailed        ActionStatus = "failed"
	ActionStatusTimeout       ActionStatus = "timeout"
	ActionStatusUserCancelled ActionStatus = "user.cancelled"
)

// ExecutionStatus is a typed string for execution status.
type ExecutionStatus string

const (
	ExecutionStatusCancelRequested ExecutionStatus = "cancel_requested"
	ExecutionStatusCancelled       ExecutionStatus = "cancelled"
	ExecutionStatusCompleted       ExecutionStatus = "completed"
	ExecutionStatusDenied          ExecutionStatus = "denied"
	ExecutionStatusExecuting       ExecutionStatus = "executing"
	ExecutionStatusFailed          ExecutionStatus = "failed"
	ExecutionStatusFeedback        ExecutionStatus = "feedback"
	ExecutionStatusPending         ExecutionStatus = "pending"
	ExecutionStatusTimeout         ExecutionStatus = "timeout"
)

// FileOperation is a typed string for file operations.
type FileOperation string

const (
	FileOperationCreate  FileOperation = "create"
	FileOperationDelete  FileOperation = "delete"
	FileOperationInsert  FileOperation = "insert"
	FileOperationPatch   FileOperation = "patch"
	FileOperationRead    FileOperation = "read"
	FileOperationReplace FileOperation = "replace"
	FileOperationUpdate  FileOperation = "update"
	FileOperationWrite   FileOperation = "write"
)

// ConnectionState is a typed string for connection states.
type ConnectionState string

const (
	ConnectionStateClosed       ConnectionState = "closed"
	ConnectionStateConnected    ConnectionState = "connected"
	ConnectionStateConnecting   ConnectionState = "connecting"
	ConnectionStateDisconnected ConnectionState = "disconnected"
	ConnectionStateError        ConnectionState = "error"
	ConnectionStateReconnecting ConnectionState = "reconnecting"
)

// OperatorStatus is a typed string for Operator status.
type OperatorStatus string

const (
	OperatorStatusActive      OperatorStatus = "active"
	OperatorStatusAvailable   OperatorStatus = "available"
	OperatorStatusBound       OperatorStatus = "bound"
	OperatorStatusOffline     OperatorStatus = "offline"
	OperatorStatusStale       OperatorStatus = "stale"
	OperatorStatusStopped     OperatorStatus = "stopped"
	OperatorStatusTerminated  OperatorStatus = "terminated"
	OperatorStatusUnavailable OperatorStatus = "unavailable"
)

// OperatorType is a typed string for Operator type.
type OperatorType string

const (
	OperatorTypeCloud  OperatorType = "cloud"
	OperatorTypeSystem OperatorType = "system"
)

// CloudSubtype is a typed string for cloud subtype.
type CloudSubtype string

const (
	CloudSubtypeAWS   CloudSubtype = "aws"
	CloudSubtypeAzure CloudSubtype = "azure"
	CloudSubtypeGCP   CloudSubtype = "gcp"
	CloudSubtypeG8EP  CloudSubtype = "g8ep"
)

// UserStatus is a typed string for user status.
type UserStatus string

const (
	UserStatusActive   UserStatus = "active"
	UserStatusDisabled UserStatus = "disabled"
)

// AuthProvider is a typed string for auth provider.
type AuthProvider string

const (
	AuthProviderJWT     AuthProvider = "jwt"
	AuthProviderLocal   AuthProvider = "local"
	AuthProviderPasskey AuthProvider = "passkey"
)

// ApprovalType is a typed string for approval type.
type ApprovalType string

const (
	ApprovalTypeAgentContinue ApprovalType = "agent.continue"
	ApprovalTypeCommand       ApprovalType = "command"
	ApprovalTypeFileEdit      ApprovalType = "file.edit"
	ApprovalTypeIntent        ApprovalType = "intent"
	ApprovalTypeStream        ApprovalType = "stream"
)

// SessionType is a typed string for session type.
type SessionType string

const (
	SessionTypeCLI      SessionType = "cli"
	SessionTypeOperator SessionType = "operator"
	SessionTypeWeb      SessionType = "web"
	SessionTypeApp      SessionType = "app"
)

// StreamStatus is a typed string for stream status.
type StreamStatus string

const (
	StreamStatusCancelled StreamStatus = "cancelled"
	StreamStatusCompleted StreamStatus = "completed"
	StreamStatusExited    StreamStatus = "exited"
	StreamStatusFailed    StreamStatus = "failed"
	StreamStatusSummary   StreamStatus = "summary"
)

// CommandExitStatus is a typed string for command exit status.
type CommandExitStatus string

const (
	CommandExitStatusError         CommandExitStatus = "error"
	CommandExitStatusFailure       CommandExitStatus = "failure"
	CommandExitStatusInterrupted   CommandExitStatus = "interrupted"
	CommandExitStatusInvalidExit   CommandExitStatus = "invalid_exit"
	CommandExitStatusKilled        CommandExitStatus = "killed"
	CommandExitStatusMisuse        CommandExitStatus = "misuse"
	CommandExitStatusNotExecutable CommandExitStatus = "not_executable"
	CommandExitStatusNotFound      CommandExitStatus = "not_found"
	CommandExitStatusSuccess       CommandExitStatus = "success"
	CommandExitStatusTerminated    CommandExitStatus = "terminated"
	CommandExitStatusSignal1       CommandExitStatus = "signal_1"  // SIGHUP
	CommandExitStatusSignal2       CommandExitStatus = "signal_2"  // SIGINT
	CommandExitStatusSignal3       CommandExitStatus = "signal_3"  // SIGQUIT
	CommandExitStatusSignal6       CommandExitStatus = "signal_6"  // SIGABRT
	CommandExitStatusSignal9       CommandExitStatus = "signal_9"  // SIGKILL
	CommandExitStatusSignal11      CommandExitStatus = "signal_11" // SIGSEGV
	CommandExitStatusSignal13      CommandExitStatus = "signal_13" // SIGPIPE
	CommandExitStatusSignal15      CommandExitStatus = "signal_15" // SIGTERM
)

// VaultMode is a typed string for vault mode.
type VaultMode string

const (
	VaultModeScrubbed VaultMode = "scrubbed"
	VaultModeRaw      VaultMode = "raw"
)

// ToolScope is a typed string for tool scope.
type ToolScope string

const (
	ToolScopeOperatorGated ToolScope = "operator_gated"
	ToolScopeUniversal     ToolScope = "universal"
)

// Platform is a typed string for platform.
type Platform string

const (
	PlatformDarwin  Platform = "darwin"
	PlatformLinux   Platform = "linux"
	PlatformWindows Platform = "windows"
)

// ComponentName is a typed string for component name.
type ComponentName string

const (
	ComponentNameClient      ComponentName = "client"
	ComponentNameG8EO        ComponentName = "g8eo"
	ComponentNameG8EOGateway ComponentName = "g8eo-gateway"
)

// SystemHealth is a typed string for system health.
type SystemHealth string

const (
	SystemHealthDegraded  SystemHealth = "degraded"
	SystemHealthHealthy   SystemHealth = "healthy"
	SystemHealthUnhealthy SystemHealth = "unhealthy"
	SystemHealthUnknown   SystemHealth = "unknown"
)

// NetworkProtocol is a typed string for network protocol.
type NetworkProtocol string

const (
	NetworkProtocolTCP NetworkProtocol = "tcp"
	NetworkProtocolUDP NetworkProtocol = "udp"
)

// Environment is a typed string for environment.
type Environment string

const (
	EnvironmentDev        Environment = "dev"
	EnvironmentProduction Environment = "production"
	EnvironmentTest       Environment = "test"
)

// VersionStability is a typed string for version stability.
type VersionStability string

const (
	VersionStabilityBeta       VersionStability = "beta"
	VersionStabilityDev        VersionStability = "dev"
	VersionStabilityStable     VersionStability = "stable"
	VersionStabilityUnstable   VersionStability = "unstable"
	VersionStabilityDeprecated VersionStability = "deprecated"
)

// UserRole is a typed string for user role.
type UserRole string

const (
	UserRoleAdmin    UserRole = "admin"
	UserRoleOperator UserRole = "operator"
	UserRoleOwner    UserRole = "owner"
	UserRoleUser     UserRole = "user"
)

// CAType is a typed string for certificate authority type.
type CAType string

const (
	CATypeRoot        CAType = "root"
	CATypeHub         CAType = "hub"
	CATypeOperator    CAType = "operator"
	CATypeGatewayPeer CAType = "gateway-peer"
)

// ServiceName is a typed string for service names.
type ServiceName string

const (
	ServiceNameOperatorGateway ServiceName = "operator-gateway"
)

// GatewayMode is a typed string for gateway mode.
type GatewayMode string

const (
	GatewayModeGateway  GatewayMode = "gateway"
	GatewayModeStatusOK GatewayMode = "ok"
)

// ThinkingActionType is a typed string for thinking action type.
type ThinkingActionType string

const (
	ThinkingActionTypeEnd    ThinkingActionType = "end"
	ThinkingActionTypeStart  ThinkingActionType = "start"
	ThinkingActionTypeUpdate ThinkingActionType = "update"
)

// HistoryEventType is a typed string for history event type.
type HistoryEventType string

const (
	HistoryEventTypeAPIKeyRefreshed      HistoryEventType = "api.key.refreshed"
	HistoryEventTypeAuthenticated        HistoryEventType = "authenticated"
	HistoryEventTypeBound                HistoryEventType = "bound"
	HistoryEventTypeClaimed              HistoryEventType = "claimed"
	HistoryEventTypeCreated              HistoryEventType = "created"
	HistoryEventTypeCreatedFromRefresh   HistoryEventType = "created.from.refresh"
	HistoryEventTypeDeactivated          HistoryEventType = "deactivated"
	HistoryEventTypeHeartbeatReceived    HistoryEventType = "heartbeat.received"
	HistoryEventTypeReconnected          HistoryEventType = "reconnected"
	HistoryEventTypeRegistered           HistoryEventType = "registered"
	HistoryEventTypeReset                HistoryEventType = "reset"
	HistoryEventTypeShutdownRequested    HistoryEventType = "shutdown.requested"
	HistoryEventTypeSlotConsumed         HistoryEventType = "slot.consumed"
	HistoryEventTypeSlotCreated          HistoryEventType = "slot.created"
	HistoryEventTypeSlotReleased         HistoryEventType = "slot.released"
	HistoryEventTypeStatusChanged        HistoryEventType = "status.changed"
	HistoryEventTypeStopped              HistoryEventType = "stopped"
	HistoryEventTypeTerminated           HistoryEventType = "terminated"
	HistoryEventTypeTerminatedForRefresh HistoryEventType = "terminated.for.refresh"
	HistoryEventTypeUnbound              HistoryEventType = "unbound"
)

// HeartbeatType is a typed string for heartbeat type.
type HeartbeatType string

const (
	HeartbeatTypeAutomatic HeartbeatType = "automatic"
	HeartbeatTypeBootstrap HeartbeatType = "bootstrap"
	HeartbeatTypeRequested HeartbeatType = "requested"
)

// AuthAuditResult is a typed string for auth audit result.
type AuthAuditResult string

const (
	AuthAuditResultFailure       AuthAuditResult = "failure"
	AuthAuditResultInvalidAPIKey AuthAuditResult = "invalid_api_key"
	AuthAuditResultSuccess       AuthAuditResult = "success"
)

// ToolDisplayCategory is a typed string for tool display category.
type ToolDisplayCategory string

const (
	ToolDisplayCategoryExecution ToolDisplayCategory = "execution"
	ToolDisplayCategoryFile      ToolDisplayCategory = "file"
	ToolDisplayCategoryGeneral   ToolDisplayCategory = "general"
	ToolDisplayCategoryNetwork   ToolDisplayCategory = "network"
	ToolDisplayCategorySearch    ToolDisplayCategory = "search"
)

// SessionKeyPrefix is a typed string for session key prefix.
type SessionKeyPrefix string

const (
	SessionKeyPrefixCLI      SessionKeyPrefix = "cli_session"
	SessionKeyPrefixOperator SessionKeyPrefix = "operator_session"
	SessionKeyPrefixWeb      SessionKeyPrefix = "web_session"
)

// SuspendedTxStatus is a typed string for suspended transaction status.
type SuspendedTxStatus string

const (
	SuspendedTxStatusPending           SuspendedTxStatus = "pending"
	SuspendedTxStatusApproved          SuspendedTxStatus = "approved"
	SuspendedTxStatusExpiredOrNotFound SuspendedTxStatus = "expired_or_not_found"
)

// GatewayResponseStatus is a typed string for the status field returned in
// gateway suspension responses (A2A suspension response, OOB suspension
// queries). It is the wire value clients compare against to detect that a
// mutation has been paused awaiting L3 notary approval.
type GatewayResponseStatus string

const (
	GatewayResponseStatusSuspended GatewayResponseStatus = "suspended"
)

// MCPApprovalPausedPrefix is the leading text of the directive returned to the
// calling agent when an MCP tool call is paused awaiting human passkey
// authorization under ratify or notary posture. The full message is built by
// mcp.approvalPausedMessage; tests assert the response content starts with
// this prefix rather than matching the entire dynamic string.
const MCPApprovalPausedPrefix = "Execution paused"

// HistoryActor is a typed string for history actor.
type HistoryActor string

const (
	HistoryActorNone   HistoryActor = ""
	HistoryActorG8EO   HistoryActor = "g8eo"
	HistoryActorSystem HistoryActor = "system"
	HistoryActorUser   HistoryActor = "user"
)

// AISource is a typed string for AI source.
type AISource string

const (
	AISourceTerminalAnchored AISource = "ai.terminal.anchored"
	AISourceTerminalDirect   AISource = "ai.terminal.direct"
	AISourceToolCall         AISource = "ai.tool.call"
)

// ComponentStatus is a typed string for component status.
type ComponentStatus string

const (
	ComponentStatusActive      ComponentStatus = "active"
	ComponentStatusError       ComponentStatus = "error"
	ComponentStatusInactive    ComponentStatus = "inactive"
	ComponentStatusMaintenance ComponentStatus = "maintenance"
	ComponentStatusDegraded    ComponentStatus = "degraded"
)

// WorkflowType is a typed string for workflow type.
type WorkflowType string

const (
	WorkflowTypeG8eBound      WorkflowType = "g8e.bound"
	WorkflowTypeG8eCloudBound WorkflowType = "g8e.cloud.bound"
	WorkflowTypeG8eNotBound   WorkflowType = "g8e.not.bound"
	WorkflowTypeTriage        WorkflowType = "triage"
	WorkflowTypeInvestigation WorkflowType = "investigation"
)

// AITaskId is a typed string for AI task identifiers.
type AITaskId string

const (
	AITaskIDAgentContinue      AITaskId = "ai.agent.continue"
	AITaskIDChat               AITaskId = "ai.chat"
	AITaskIDCommand            AITaskId = "ai.command"
	AITaskIDDirectCommand      AITaskId = "ai.direct.command"
	AITaskIDFetchFileDiff      AITaskId = "ai.fetch.file.diff"
	AITaskIDFetchFileHistory   AITaskId = "ai.fetch.file.history"
	AITaskIDFetchHistory       AITaskId = "ai.fetch.history"
	AITaskIDFetchLogs          AITaskId = "ai.fetch.logs"
	AITaskIDFileEdit           AITaskId = "ai.file.edit"
	AITaskIDFsList             AITaskId = "ai.fs.list"
	AITaskIDFsRead             AITaskId = "ai.fs.read"
	AITaskIDIntentGrant        AITaskId = "ai.intent.grant"
	AITaskIDIntentRevoke       AITaskId = "ai.intent.revoke"
	AITaskIDPortCheck          AITaskId = "ai.port.check"
	AITaskIDRecursiveGrep      AITaskId = "ai.recursive_grep"
	AITaskIDRestoreFile        AITaskId = "ai.restore.file"
	AITaskIdChat               AITaskId = "ai.chat"
	AITaskIdCase               AITaskId = "ai.case"
	AITaskIdMemory             AITaskId = "ai.memory"
	AITaskIdCommand            AITaskId = "ai.command"
	AITaskIdCommandExecution   AITaskId = "ai.command.execution"
	AITaskIdDirectCommand      AITaskId = "ai.direct.command"
	AITaskIdIntentGrant        AITaskId = "ai.intent.grant"
	AITaskIdIntentRevoke       AITaskId = "ai.intent.revoke"
	AITaskIdFileEdit           AITaskId = "ai.file.edit"
	AITaskIdFileOperation      AITaskId = "ai.file.operation"
	AITaskIdFsList             AITaskId = "ai.fs.list"
	AITaskIdRecursiveGrep      AITaskId = "ai.recursive.grep"
	AITaskIdPortCheck          AITaskId = "ai.port.check"
	AITaskIdAgentContinue      AITaskId = "ai.agent.continue"
	AITaskIdInvestigationQuery AITaskId = "ai.investigation.query"
)

// ConsensusMember is a typed string for consensus member identifiers.
type ConsensusMember string

const (
	ConsensusMemberAxiom    ConsensusMember = "axiom"
	ConsensusMemberConcord  ConsensusMember = "concord"
	ConsensusMemberVariance ConsensusMember = "variance"
	ConsensusMemberPragma   ConsensusMember = "pragma"
	ConsensusMemberNemesis  ConsensusMember = "nemesis"
)

// ConsensusAuditMode is a typed string for consensus audit mode.
type ConsensusAuditMode string

const (
	ConsensusAuditModeUnanimous ConsensusAuditMode = "unanimous"
	ConsensusAuditModeMajority  ConsensusAuditMode = "majority"
	ConsensusAuditModeTied      ConsensusAuditMode = "tied"
)

// ConsensusAuditStatus is a typed string for consensus audit status.
type ConsensusAuditStatus string

const (
	ConsensusAuditStatusOk      ConsensusAuditStatus = "ok"
	ConsensusAuditStatusRevised ConsensusAuditStatus = "revised"
	ConsensusAuditStatusSwap    ConsensusAuditStatus = "swap"
)

// AuditorReason is a typed string for auditor reason.
type AuditorReason string

const (
	AuditorReasonOk                 AuditorReason = "ok"
	AuditorReasonRevised            AuditorReason = "revised"
	AuditorReasonRevisedFromDissent AuditorReason = "revised_from_dissent"
	AuditorReasonSwappedToDissenter AuditorReason = "swapped_to_dissenter"
	AuditorReasonWhitelistViolation AuditorReason = "whitelist_violation"
	AuditorReasonNoValidRevision    AuditorReason = "no_valid_revision"
	AuditorReasonAuditorError       AuditorReason = "auditor_error"
	AuditorReasonEmptyResponse      AuditorReason = "empty_response"
)

// TieBreakReason is a typed string for tie-break reason.
type TieBreakReason string

const (
	TieBreakReasonShortest        TieBreakReason = "shortest"
	TieBreakReasonExcludedNemesis TieBreakReason = "excluded_nemesis"
)

// ReasoningAgent is a typed string for reasoning agent identifiers.
type ReasoningAgent string

const (
	ReasoningAgentSage ReasoningAgent = "sage"
	ReasoningAgentDash ReasoningAgent = "dash"
)

// ErrorCode is a typed string for g8e error codes.
type ErrorCode string

const (
	ErrorCodeGenericError               ErrorCode = "G8E-1000"
	ErrorCodeUnexpectedError            ErrorCode = "G8E-1001"
	ErrorCodeNotImplemented             ErrorCode = "G8E-1002"
	ErrorCodeConfigError                ErrorCode = "G8E-1100"
	ErrorCodeMissingEnvVar              ErrorCode = "G8E-1101"
	ErrorCodeInvalidSettings            ErrorCode = "G8E-1102"
	ErrorCodeServiceInitError           ErrorCode = "G8E-1103"
	ErrorCodeAuthError                  ErrorCode = "G8E-1200"
	ErrorCodeTokenExpired               ErrorCode = "G8E-1201"
	ErrorCodeInvalidToken               ErrorCode = "G8E-1202"
	ErrorCodeInsufficientPermissions    ErrorCode = "G8E-1203"
	ErrorCodeDBConnectionError          ErrorCode = "G8E-1300"
	ErrorCodeDBQueryError               ErrorCode = "G8E-1301"
	ErrorCodeDBDocumentNotFound         ErrorCode = "G8E-1302"
	ErrorCodeDBWriteError               ErrorCode = "G8E-1303"
	ErrorCodeDBTransactionError         ErrorCode = "G8E-1304"
	ErrorCodePubSubConnectionError      ErrorCode = "G8E-1400"
	ErrorCodePubSubPublishError         ErrorCode = "G8E-1401"
	ErrorCodePubSubSubscribeError       ErrorCode = "G8E-1402"
	ErrorCodePubSubTopicError           ErrorCode = "G8E-1403"
	ErrorCodeStorageConnectionError     ErrorCode = "G8E-1500"
	ErrorCodeStorageReadError           ErrorCode = "G8E-1501"
	ErrorCodeStorageWriteError          ErrorCode = "G8E-1502"
	ErrorCodeStorageDeleteError         ErrorCode = "G8E-1503"
	ErrorCodeAPIConnectionError         ErrorCode = "G8E-1600"
	ErrorCodeAPITimeoutError            ErrorCode = "G8E-1601"
	ErrorCodeAPIResponseError           ErrorCode = "G8E-1602"
	ErrorCodeAPIRequestError            ErrorCode = "G8E-1603"
	ErrorCodeAPIRateLimitError          ErrorCode = "G8E-1604"
	ErrorCodeGenericNotFound            ErrorCode = "G8E-1605"
	ErrorCodeExternalServiceError       ErrorCode = "G8E-1607"
	ErrorCodeValidationError            ErrorCode = "G8E-1700"
	ErrorCodeMissingRequiredField       ErrorCode = "G8E-1701"
	ErrorCodeInvalidFieldFormat         ErrorCode = "G8E-1702"
	ErrorCodeInvalidFieldValue          ErrorCode = "G8E-1703"
	ErrorCodeInvalidFieldType           ErrorCode = "G8E-1704"
	ErrorCodeSchemaValidationFailed     ErrorCode = "G8E-1705"
	ErrorCodeSchemaNotFound             ErrorCode = "G8E-1706"
	ErrorCodeInvalidInput               ErrorCode = "G8E-1707"
	ErrorCodeBusinessLogicError         ErrorCode = "G8E-1800"
	ErrorCodeWorkflowError              ErrorCode = "G8E-1801"
	ErrorCodeStateTransitionError       ErrorCode = "G8E-1802"
	ErrorCodeResourceConflict           ErrorCode = "G8E-1803"
	ErrorCodeTaskCreationFailed         ErrorCode = "G8E-1804"
	ErrorCodeOperationFailed            ErrorCode = "G8E-1805"
	ErrorCodeGovernanceRejected         ErrorCode = "G8E-1806"
	ErrorCodeModelCapabilityUnsupported ErrorCode = "G8E-1807"
	ErrorCodeServiceUnavailableError    ErrorCode = "G8E-1900"
)

// ErrorCategory is a typed string for error categories.
type ErrorCategory string

const (
	ErrorCategoryValidation         ErrorCategory = "validation"
	ErrorCategoryBusinessLogic      ErrorCategory = "business_logic"
	ErrorCategoryConfiguration      ErrorCategory = "configuration"
	ErrorCategoryAuth               ErrorCategory = "auth"
	ErrorCategoryPermission         ErrorCategory = "permission"
	ErrorCategoryResourceNotFound   ErrorCategory = "resource_not_found"
	ErrorCategoryConflict           ErrorCategory = "conflict"
	ErrorCategoryRateLimit          ErrorCategory = "rate_limit"
	ErrorCategoryServiceUnavailable ErrorCategory = "service_unavailable"
	ErrorCategoryExternalService    ErrorCategory = "external_service"
	ErrorCategoryTimeout            ErrorCategory = "timeout"
	ErrorCategoryDatabase           ErrorCategory = "database"
	ErrorCategoryNetwork            ErrorCategory = "network"
	ErrorCategoryPubSub             ErrorCategory = "pubsub"
	ErrorCategoryStorage            ErrorCategory = "storage"
	ErrorCategoryInternal           ErrorCategory = "internal"
	ErrorCategoryDependency         ErrorCategory = "dependency"
)

// ErrorSeverity is a typed string for error severity levels.
type ErrorSeverity string

const (
	ErrorSeverityLow      ErrorSeverity = "low"
	ErrorSeverityMedium   ErrorSeverity = "medium"
	ErrorSeverityHigh     ErrorSeverity = "high"
	ErrorSeverityCritical ErrorSeverity = "critical"
	ErrorSeverityInfo     ErrorSeverity = "info"
)

// InfrastructureStatus is a typed string for infrastructure status.
type InfrastructureStatus string

const (
	InfrastructureStatusCritical    InfrastructureStatus = "critical"
	InfrastructureStatusDegraded    InfrastructureStatus = "degraded"
	InfrastructureStatusHealthy     InfrastructureStatus = "healthy"
	InfrastructureStatusStable      InfrastructureStatus = "stable"
	InfrastructureStatusUnknown     InfrastructureStatus = "unknown"
	InfrastructureStatusOperational InfrastructureStatus = "operational"
	InfrastructureStatusDown        InfrastructureStatus = "down"
)

// AuthMethod is a typed string for authentication method.
type AuthMethod string

const (
	AuthMethodKvPubSub        AuthMethod = "kv_pubsub"
	AuthMethodSession         AuthMethod = "session"
	AuthMethodProxy           AuthMethod = "proxy"
	AuthMethodOperatorSession AuthMethod = "operator_session"
	AuthMethodTest            AuthMethod = "test"
)
