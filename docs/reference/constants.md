# Constants System

## Overview

The g8e constants system maintains canonical constant definitions across the platform. Constants are defined in Go source files in `internal/constants/` and referenced by JSON schemas in `protocol/constants/` for protocol documentation and external consumers.

## Constant Categories

### Database Collections (`collections.go`)
Canonical collection names for the operator's embedded SQLite database:
- `CollectionUsers`, `CollectionWebSessions`, `CollectionOperatorSessions`, `CollectionCLISessions`
- `CollectionLoginAudit`, `CollectionAuthAdminAudit`, `CollectionAccountLocks`
- `CollectionOrganizations`, `CollectionOperators`, `CollectionOperatorUsage`
- `CollectionCases`, `CollectionInvestigations`, `CollectionInvitations`, `CollectionTasks`
- `CollectionMemories`, `CollectionSettings`, `CollectionConsoleAudit`, `CollectionBoundSessions`
- `CollectionPasskeyChallenges`, `CollectionPersonas`, `CollectionAgentActivityMetadata`
- `CollectionReputationState`, `CollectionReputationCommitments`, `CollectionStakeResolutions`
- `CollectionRevokedCertificates`, `CollectionTrustedSigners`, `CollectionAppPolicies`, `CollectionChaosEvents`

### Event Types (`events.go`)
Typed event identifiers for the pub/sub system:
- App lifecycle events: `EventAppCaseCreated`, `EventAppCaseUpdated`, `EventAppTaskCreated`, `EventAppTaskCompleted`
- App investigation events: `EventAppInvestigationCreated`, `EventAppInvestigationUpdated`, `EventAppInvestigationStarted`, `EventAppInvestigationClosed`
- Operator events: `EventOperatorHeartbeatSent`, `EventOperatorHeartbeatReceived`, `EventOperatorCommandRequested`, `EventOperatorCommandCompleted`, `EventOperatorCommandFailed`
- Operator status events: `EventOperatorStatusUpdatedActive`, `EventOperatorStatusUpdatedAvailable`, `EventOperatorStatusUpdatedOffline`
- Operator approval events: `EventOperatorCommandApprovalRequested`, `EventOperatorCommandApprovalGranted`, `EventOperatorStreamApprovalRequested`
- And hundreds of additional event types across all subsystems

### API Paths (`api_paths.go`)
HTTP route paths for the Gateway REST API:
- MCP routes: `/api/v1/mcp/tools/list`, `/api/v1/mcp/tools/call`, `/api/v1/mcp/resources/list`, `/api/v1/mcp/prompts/list`
- A2A routes: `/api/v1/a2a/call`
- Governance routes: `/api/v1/governance/envelopes`, `/api/v1/governance/signers`
- Operator management: `/api/v1/operators`, `/api/v1/operators/bind`, `/api/v1/operators/reauth`
- Data routes: `/api/v1/data/settings`, `/api/v1/data/items`, `/api/v1/blobs/`
- KV routes: `/api/v1/kv/`
- PubSub routes: `/api/v1/pubsub/publish`, `/api/v1/pubsub/stream`
- SSE routes: `/api/v1/sse/push`, `/api/v1/sse/events`, `/api/v1/sse/stream`
- PKI routes: `/api/v1/pki/csr/sign`, `/api/v1/pki/devices/enroll`, `/api/v1/pki/certificates/revoke`
- Audit routes: `/api/v1/audit/receipts`, `/api/v1/audit/receipts/export`
- User routes: `/api/v1/users`, `/api/v1/users/me`
- Auth routes: `/api/v1/auth/login/verify`, `/api/v1/auth/logout`, `/api/v1/auth/bootstrap`, `/api/v1/auth/passkeys/register/challenge`, `/api/v1/auth/passkeys/authenticate/challenge`
- Approval routes: `/api/v1/approvals`, `/api/v1/approve/`
- Admin routes: `/api/v1/admin/app-policies/`, `/api/v1/admin/apps/revoke`
- Well-known routes: `/.well-known/g8e/pki/ca-bundle`, `/.well-known/g8e/pki/fingerprint`
- Health: `/api/v1/health`

### Channels (`channels.go`)
Pub/sub channel names for inter-component communication:
- Command channels: `CmdChannel(operatorID, operatorSessionID)` returns `cmd:{operator_id}:{operator_session_id}`
- Result channels: `ResultsChannel(operatorID, operatorSessionID)` returns `results:{operator_id}:{operator_session_id}`
- Heartbeat channels: `HeartbeatChannel(operatorID, operatorSessionID)` returns `heartbeat:{operator_id}:{operator_session_id}`
- Storage channels: `ChannelStorageDocument`, `ChannelStorageKv`, `ChannelStorageBlob`
- Governance channels: `ChannelGovernance`, `ChannelOperatorIntent`, `ChannelOperatorDevice`
- SSE channels: `ChannelSseEvent`
- PubSub wire protocol actions: `PubSubActionSubscribe`, `PubSubActionPSubscribe`, `PubSubActionUnsubscribe`, `PubSubActionPublish`
- PubSub wire protocol events: `PubSubEventMessage`, `PubSubEventPMessage`, `PubSubEventSubscribed`

### Intents (`intents.go`)
Cloud provider intent classification values for governance posture:
- EC2 intents: `IntentEc2Discovery`, `IntentEc2Management`, `IntentEc2SnapshotManagement`
- S3 intents: `IntentS3Read`, `IntentS3Write`, `IntentS3BucketDiscovery`, `IntentS3Delete`
- RDS intents: `IntentRdsDiscovery`, `IntentRdsManagement`, `IntentRdsSnapshotManagement`
- Aurora intents: `IntentAuroraClusterManagement`, `IntentAuroraScaling`, `IntentAuroraCloning`, `IntentAuroraGlobalDatabase`
- Lambda intents: `IntentLambdaDiscovery`, `IntentLambdaInvoke`
- ECS/EKS intents: `IntentEcsDiscovery`, `IntentEcsManagement`, `IntentEksDiscovery`
- VPC/ELB intents: `IntentVpcDiscovery`, `IntentElbDiscovery`
- Route53 intents: `IntentRoute53Discovery`, `IntentRoute53Management`
- Autoscaling intents: `IntentAutoscalingDiscovery`, `IntentAutoscalingManagement`
- CloudWatch intents: `IntentCloudwatchLogs`, `IntentCloudwatchMetrics`
- SNS/SQS intents: `IntentSnsDiscovery`, `IntentSnsPublish`, `IntentSqsDiscovery`, `IntentSqsManagement`
- DynamoDB intents: `IntentDynamodbDiscovery`, `IntentDynamodbRead`, `IntentDynamodbWrite`
- KMS intents: `IntentKmsDiscovery`, `IntentKmsCrypto`
- IAM intents: `IntentIamDiscovery`
- Additional intents for ACM, API Gateway, Step Functions, Athena, Glue, CloudFront, CodeDeploy, Cost Explorer

### Status Enums (`status.go`)
Internal enumeration constants:
- `ActionStatus`: `ActionStatusCancelled`, `ActionStatusCompleted`, `ActionStatusFailed`, `ActionStatusTimeout`, `ActionStatusUserCancelled`
- `ExecutionStatus`: `ExecutionStatusCancelRequested`, `ExecutionStatusCancelled`, `ExecutionStatusCompleted`, `ExecutionStatusDenied`, `ExecutionStatusExecuting`, `ExecutionStatusFailed`, `ExecutionStatusFeedback`, `ExecutionStatusPending`, `ExecutionStatusTimeout`
- `FileOperation`: `FileOperationCreate`, `FileOperationDelete`, `FileOperationInsert`, `FileOperationPatch`, `FileOperationRead`, `FileOperationReplace`, `FileOperationUpdate`, `FileOperationWrite`
- `ConnectionState`: `ConnectionStateClosed`, `ConnectionStateConnected`, `ConnectionStateConnecting`, `ConnectionStateDisconnected`, `ConnectionStateError`, `ConnectionStateReconnecting`
- `OperatorStatus`: `OperatorStatusActive`, `OperatorStatusAvailable`, `OperatorStatusBound`, `OperatorStatusOffline`, `OperatorStatusStale`, `OperatorStatusStopped`, `OperatorStatusTerminated`, `OperatorStatusUnavailable`
- `OperatorType`: `OperatorTypeCloud`, `OperatorTypeSystem`
- `CloudSubtype`: `CloudSubtypeAWS`, `CloudSubtypeAzure`, `CloudSubtypeGCP`, `CloudSubtypeG8EP`
- `UserStatus`: `UserStatusActive`, `UserStatusDisabled`
- `AuthProvider`: `AuthProviderJWT`, `AuthProviderLocal`, `AuthProviderPasskey`
- `ApprovalType`: `ApprovalTypeAgentContinue`, `ApprovalTypeCommand`, `ApprovalTypeFileEdit`, `ApprovalTypeIntent`, `ApprovalTypeStream`
- `SessionType`: `SessionTypeCLI`, `SessionTypeOperator`, `SessionTypeWeb`
- `StreamStatus`: `StreamStatusCancelled`, `StreamStatusCompleted`, `StreamStatusExited`, `StreamStatusFailed`, `StreamStatusSummary`
- `SentinelStatus`: `SentinelStatusError`, `SentinelStatusFailure`, `SentinelStatusInterrupted`, `SentinelStatusInvalidExit`, `SentinelStatusKilled`, `SentinelStatusMisuse`, `SentinelStatusNotExecutable`, `SentinelStatusNotFound`, `SentinelStatusSuccess`, `SentinelStatusTerminated`
- `VaultMode`: `VaultModeScrubbed`, `VaultModeRaw`
- `ToolScope`: `ToolScopeOperatorGated`, `ToolScopeUniversal`
- `Platform`: `PlatformDarwin`, `PlatformLinux`, `PlatformWindows`
- `ComponentName`: `ComponentNameClient`, `ComponentNameG8EO`, `ComponentNameG8EOGateway`
- `SystemHealth`: `SystemHealthDegraded`, `SystemHealthHealthy`, `SystemHealthUnhealthy`, `SystemHealthUnknown`
- `NetworkProtocol`: `NetworkProtocolTCP`, `NetworkProtocolUDP`
- `Environment`: `EnvironmentDev`, `EnvironmentProduction`, `EnvironmentTest`
- `VersionStability`: `VersionStabilityBeta`, `VersionStabilityDev`, `VersionStabilityStable`
- `UserRole`: `UserRoleAdmin`, `UserRoleOperator`, `UserRoleOwner`, `UserRoleUser`
- `GatewayMode`: `GatewayModeGateway`, `GatewayModeStatusOK`
- Additional enums for thinking actions, history events, heartbeat types, auth audit results, tool display categories, session key prefixes, history actors, and AI sources

### Headers (`auth.go`)
HTTP header names used across the platform:
- Standard headers: `HeaderAccept`, `HeaderAcceptLanguage`, `HeaderAuthorization`, `HeaderCacheControl`, `HeaderContentType`, `HeaderCookie`, `HeaderUserAgent`
- CORS headers: `HeaderAccessControlAllowCredentials`, `HeaderAccessControlAllowOrigin`, `HeaderAccessControlRequestHeaders`, `HeaderAccessControlRequestMethod`
- G8E identity headers: `HeaderOperatorID`, `HeaderOperatorSessionID`, `HeaderWebSessionID`, `HeaderCLISessionID`, `HeaderUserID`, `HeaderOrganizationID`
- G8E context headers: `HeaderCaseID`, `HeaderExecutionID`, `HeaderInvestigationID`, `HeaderTaskID`, `HeaderBoundOperators`
- G8E system headers: `HeaderRequestID`, `HeaderSourceComponent`, `HeaderSystemFingerprint`, `HeaderXAccelBuffering`
- Proxy headers: `HeaderXForwardedFor`, `HeaderXForwardedHost`, `HeaderXForwardedProto`, `HeaderXProxyOrganizationID`, `HeaderXProxyUserID`, `HeaderXRequestTimestamp`
- Additional headers for content disposition, language, length, last event ID, pragma, requested with, and set cookie

### Action Types (`action_types.go`)
GovernanceEnvelope action type constants:
- `ActionTypeA2aCall`, `ActionTypeEvalAnswer`, `ActionTypeExecuteBash`, `ActionTypeFetchFileDiff`, `ActionTypeFetchFileHistory`
- `ActionTypeFetchHistory`, `ActionTypeFetchLogs`, `ActionTypeFileEdit`, `ActionTypeFsGrep`, `ActionTypeFsList`, `ActionTypeFsRead`
- `ActionTypeGrantIntent`, `ActionTypeHeartbeat`, `ActionTypeInvestigationCreate`, `ActionTypeMcpCall`
- `ActionTypeMcpPromptGet`, `ActionTypeMcpPromptList`, `ActionTypeMcpResourceList`, `ActionTypeMcpResourceRead`
- `ActionTypePortCheck`, `ActionTypeRestoreFile`, `ActionTypeRevokeIntent`, `ActionTypeShutdown`

### Additional Constant Files
- `paths.go` - Filesystem paths for Operator data, certificates, ledger, runtime directories
- `ports.go` - Network port numbers: `Ports.OperatorHttp` (8080), `Ports.OperatorHttps` (8443), `Ports.InsecureMcpGateway` (18789)
- `exit_codes.go` - Process exit code constants: `ExitSuccess` (0), `ExitGeneralError` (1), `ExitAuthFailure` (2), `ExitPermissionDenied` (3), `ExitNetworkError` (4), `ExitConfigError` (5), `ExitStorageError` (6), `ExitCertTrustFailure` (7)
- `network.go` - Network-related constants: `DefaultEndpoint` (localhost)
- `output.go` - Output format constants: `TruncatedOutputFormat`
- `mappings.go` - Mapping helpers for protocol translation between event types and action types
- `auth.go` - Authentication-related constants including passkey purposes, WebAuthn algorithms, PKI leaf types, HTTP headers, and context keys
- `platform.go` - Platform-specific event constants for usage, notifications, auth, SSE, terminal, sentinel, telemetry, and console
- `prompts.go` - Agent mode constants and prompt section identifiers
- `senders.go` - Message source identifiers and message type constants
- `pubsub.go` - PubSub field name constants
- `document_ids.go` - Document ID prefixes: `DocIDPlatformSettings`, `DocIDUserSettingsPrefix`
- `kv_keys.go` - Key-value store key patterns for caching, sessions, operators, users, investigations, auth, and execution
- `env_vars.go` - Environment variable names (currently empty - g8e uses zero environment variables)
- `timestamp.go` - Go-specific format strings: `FormatRFC3339`
- `field_paths.go` - Field path access control configurations for investigations, memories, and cases collections
- `agents.go` - Agent persona details

## JSON Reference Files

The `protocol/constants/` directory contains JSON files that serve as reference documentation and external protocol definitions. These files mirror the Go constants and are used for:
- Protocol documentation generation
- External client SDK generation
- Cross-language protocol compatibility

Key JSON files:
- `collections.json` - Collection name definitions
- `events.json` - Event type definitions
- `channels.json` - Channel name definitions
- `intents.json` - Intent classification definitions
- `status.json` - Status enum definitions
- `headers.json` - HTTP header definitions
- `api_paths.json` - API path definitions
- `ports.json` - Port number definitions
- `agents.json` - Agent persona details
- `document_ids.json` - Document ID prefix definitions
- `env_vars.json` - Environment variable name definitions
- `field_paths.json` - Field path access control configurations
- `kv_keys.json` - Key-value store key pattern definitions
- `platform.json` - Platform-specific event definitions
- `prompts.json` - Prompt section identifier definitions
- `pubsub.json` - PubSub field name definitions
- `senders.json` - Message source identifier definitions
- `timestamp.json` - Timestamp format string definitions
- `doctrine/` - Doctrine rule definitions (subdirectory)

## Protocol Generation

### Generate Protocol Artifacts

```bash
make generate
```

This command generates Go Protobuf code from `.proto` files using Buf. It is an alias for `make proto`.

### Generate Go Protobuf

```bash
make proto
```

Generates Go Protobuf code from protocol definitions using Buf. This requires Buf to be installed (installed automatically via `make buf-install` if not present).

### Generate Python Protocol

```bash
make proto-python
```

Generates Python Protobuf code for the Python protocol SDK using `grpc_tools.protoc`.

### Force Regenerate Protobuf

```bash
make proto-force
```

Force regenerates Protobuf code without version checks.

## CI Integration

Constants are validated in CI via the `G8E_STRICT_CONSTANTS_LINT` environment variable. When set, the test suite enforces that all constants are properly defined and referenced. The CI pipeline includes:
- Proto verification (`make _ci-verify-proto`) - ensures generated proto files are in sync with `.proto` sources
- Doctrine validation (`make validate-doctrines`) - validates doctrine JSON schema
- Linting (`make lint`) - runs golangci-lint and other quality checks
- Vulnerability checking (`make vulncheck`) - runs govulncheck
- Testing (`make _ci-test`) - runs tests with coverage enforcement (60% threshold)

## Adding New Constants

1. **Add the constant** to the appropriate Go file in `internal/constants/`
2. **Update the corresponding JSON file** in `protocol/constants/` if the constant is part of the public protocol
3. **Run tests** to verify the constant is properly integrated
4. **Commit both** the Go source file and any updated JSON reference files

When adding new constants, follow these guidelines:
- Use typed constants (e.g., `type EventType string`) rather than raw strings
- Group related constants in the appropriate file
- Add comprehensive documentation comments
- Ensure the constant name follows Go naming conventions
- If the constant is part of the public protocol, update the corresponding JSON file in `protocol/constants/`

