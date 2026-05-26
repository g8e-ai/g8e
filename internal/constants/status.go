// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

// OperatorStatus is a typed string for operator status.
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

// OperatorType is a typed string for operator type.
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

// DeviceLinkStatus is a typed string for device link status.
type DeviceLinkStatus string

const (
	DeviceLinkStatusActive    DeviceLinkStatus = "active"
	DeviceLinkStatusExhausted DeviceLinkStatus = "exhausted"
	DeviceLinkStatusExpired   DeviceLinkStatus = "expired"
	DeviceLinkStatusPending   DeviceLinkStatus = "pending"
	DeviceLinkStatusRevoked   DeviceLinkStatus = "revoked"
	DeviceLinkStatusUsed      DeviceLinkStatus = "used"
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

// SentinelStatus is a typed string for sentinel status.
type SentinelStatus string

const (
	SentinelStatusError         SentinelStatus = "error"
	SentinelStatusFailure       SentinelStatus = "failure"
	SentinelStatusInterrupted   SentinelStatus = "interrupted"
	SentinelStatusInvalidExit   SentinelStatus = "invalid_exit"
	SentinelStatusKilled        SentinelStatus = "killed"
	SentinelStatusMisuse        SentinelStatus = "misuse"
	SentinelStatusNotExecutable SentinelStatus = "not_executable"
	SentinelStatusNotFound      SentinelStatus = "not_found"
	SentinelStatusSuccess       SentinelStatus = "success"
	SentinelStatusTerminated    SentinelStatus = "terminated"
)

// VaultMode is a typed string for vault mode.
type VaultMode string

const (
	VaultModeScrubbed VaultMode = "scrubbed"
	VaultModeRaw      VaultMode = "raw"
)

// SentinelMode is a typed string for sentinel mode.
type SentinelMode string

const (
	SentinelModeRaw SentinelMode = "raw"
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
	VersionStabilityBeta   VersionStability = "beta"
	VersionStabilityDev    VersionStability = "dev"
	VersionStabilityStable VersionStability = "stable"
)

// UserRole is a typed string for user role.
type UserRole string

const (
	UserRoleAdmin    UserRole = "admin"
	UserRoleOperator UserRole = "operator"
	UserRoleOwner    UserRole = "owner"
	UserRoleUser     UserRole = "user"
)

// GatewayMode is a typed string for gateway mode.
type GatewayMode string

const (
	GatewayModeMode     GatewayMode = "gateway"
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
