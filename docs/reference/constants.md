# Constants System

## Overview

The g8e constants system maintains canonical constant definitions across the platform. Constants are defined in Go source files in `internal/constants/` and referenced by JSON schemas in `protocol/constants/` for protocol documentation and external consumers.

## Constant Categories

### Database Collections (`collections.go`)

Canonical collection names for the operator embedded SQLite database:

- `CollectionUsers`, `CollectionWebSessions`, `CollectionOperatorSessions`, `CollectionCLISessions`
- `CollectionLoginAudit`, `CollectionAuthAdminAudit`, `CollectionAccountLocks`
- `CollectionOrganizations`, `CollectionOperators`, `CollectionOperatorUsage`
- `CollectionCases`, `CollectionInvestigations`, `CollectionTasks`
- `CollectionMemories`, `CollectionSettings`, `CollectionConsoleAudit`, `CollectionBoundSessions`
- `CollectionPasskeyChallenges`, `CollectionPersonas`, `CollectionAgentActivityMetadata`
- `CollectionReputationState`, `CollectionReputationCommitments`, `CollectionStakeResolutions`
- `CollectionRevokedCertificates`, `CollectionTrustedSigners`, `CollectionAppPolicies`

### Event Types (`events.go`)

Typed event identifiers for the pub/sub system:

- App lifecycle: `g8e.v1.app.case.created`, `g8e.v1.app.case.updated`, `g8e.v1.app.task.created`, `g8e.v1.app.task.completed`
- App investigation: `g8e.v1.app.investigation.created`, `g8e.v1.app.investigation.updated`, `g8e.v1.app.investigation.started`, `g8e.v1.app.investigation.closed`
- Operator heartbeat: `g8e.v1.operator.heartbeat.sent`, `g8e.v1.operator.heartbeat.received`, `g8e.v1.operator.heartbeat.missed`
- Operator command: `g8e.v1.operator.command.requested`, `g8e.v1.operator.command.started`, `g8e.v1.operator.command.completed`, `g8e.v1.operator.command.failed`
- Operator status: `g8e.v1.operator.status.updated.active`, `g8e.v1.operator.status.updated.available`, `g8e.v1.operator.status.updated.offline`
- Operator approval: `g8e.v1.operator.command.approval.requested`, `g8e.v1.operator.command.approval.granted`, `g8e.v1.operator.stream.approval.requested`

### API Paths (`api_paths.go`)

HTTP route paths for the Gateway REST API:

- MCP: `/api/v1/mcp/tools/list`, `/api/v1/mcp/tools/call`, `/api/v1/mcp/tools/call/sse`, `/api/v1/mcp/resources/list`, `/api/v1/mcp/resources/read`, `/api/v1/mcp/prompts/list`, `/api/v1/mcp/prompts/get`
- A2A: `/api/v1/a2a/call`
- Governance: `/api/v1/governance/envelopes`, `/api/v1/governance/signers`, `/api/v1/governance/signers/`
- Operator: `/api/v1/operators`, `/api/v1/operators/`, `/api/v1/operators/bind`, `/api/v1/operators/unbind`, `/api/v1/operators/target`, `/api/v1/operators/reauth`
- Data: `/api/v1/data/settings`, `/api/v1/data/`, `/api/v1/blobs/`, `/api/v1/data/items`
- KV: `/api/v1/kv/`
- PubSub: `/api/v1/pubsub/publish`, `/api/v1/pubsub/stream`
- SSE: `/api/v1/sse/push`, `/api/v1/sse/events`, `/api/v1/sse/stream`
- PKI: `/api/v1/pki/csr/sign`, `/api/v1/pki/devices/enroll`, `/api/v1/pki/apps/enroll`, `/api/v1/pki/apps/delegated`, `/api/v1/pki/certificates/revoke`, `/api/v1/pki/revocation-bundle`, `/.well-known/g8e/pki/crl`, `/.well-known/g8e/pki/ca-bundle`, `/.well-known/g8e/pki/fingerprint`
- Audit: `/api/v1/audit/receipts`, `/api/v1/audit/receipts/export`, `/api/v1/audit/events`, `/api/v1/audit/summary`, `/api/v1/audit/report`
- User: `/api/v1/users`, `/api/v1/users/me`
- Auth: `/api/v1/auth/login/verify`, `/api/v1/auth/logout`, `/api/v1/auth/bootstrap`, `/api/v1/auth/bootstrap/status`, `/api/v1/auth/cli/enroll`, `/api/v1/auth/device/enroll`, `/api/v1/auth/passkeys/register/challenge`, `/api/v1/auth/passkeys/register/verify`, `/api/v1/auth/passkeys/authenticate/challenge`, `/api/v1/auth/passkeys/authenticate/verify`, `/api/v1/auth/passkeys/`, `/api/v1/auth/passkeys/jit-register/challenge`, `/api/v1/auth/passkeys/jit-register/verify`, `/api/v1/auth/passkeys/cli-register/challenge`, `/api/v1/auth/passkeys/cli-register/verify`, `/api/v1/auth/passkeys/cli/authenticate/challenge`, `/api/v1/auth/passkeys/cli/authenticate/verify`, `/api/v1/auth/sessions/me`
- Approval: `/api/v1/approvals`, `/api/v1/approvals/`, `/api/v1/approve/`
- Admin: `/api/v1/admin/app-policies/`, `/api/v1/admin/apps/revoke`
- Well-known: `/.well-known/g8e/pki/ca-bundle`, `/.well-known/g8e/pki/fingerprint`, `/.well-known/g8e/bin/`
- Scripts: `/bootstrap-ca`, `/bootstrap-ca.ps1`, `/g8e-operator.sh`, `/g8e-operator.ps1`
- Health: `/api/v1/health`

### Channels (`channels.go`)

Pub/sub channel names for inter-component communication:

- Command: `cmd:{operator_id}:{operator_session_id}`
- Result: `results:{operator_id}:{operator_session_id}`
- Heartbeat: `heartbeat:{operator_id}:{operator_session_id}`
- Storage: `ChannelStorageDocument`, `ChannelStorageKv`, `ChannelStorageBlob`
- Governance: `ChannelGovernance`, `ChannelOperatorIntent`, `ChannelOperatorDevice`
- SSE: `ChannelSseEvent`
- Wire protocol actions: `PubSubActionSubscribe`, `PubSubActionPSubscribe`, `PubSubActionUnsubscribe`, `PubSubActionPublish`
- Wire protocol events: `PubSubEventMessage`, `PubSubEventPMessage`, `PubSubEventSubscribed`

### Intents (`intents.go`)

Cloud provider intent classification values for governance posture:

- EC2: `IntentEc2Discovery`, `IntentEc2Management`, `IntentEc2SnapshotManagement`
- S3: `IntentS3Read`, `IntentS3Write`, `IntentS3BucketDiscovery`, `IntentS3Delete`
- IaC: `IntentTerraformState`, `IntentCloudformationDeployment`
- Secrets: `IntentSecretsRead`
- CloudWatch: `IntentCloudwatchLogs`, `IntentCloudwatchMetrics`
- RDS: `IntentRdsDiscovery`, `IntentRdsManagement`, `IntentRdsSnapshotManagement`
- Aurora: `IntentAuroraClusterManagement`, `IntentAuroraScaling`, `IntentAuroraCloning`, `IntentAuroraGlobalDatabase`
- Compute: `IntentLambdaDiscovery`, `IntentLambdaInvoke`, `IntentEcsDiscovery`, `IntentEcsManagement`, `IntentEksDiscovery`
- Network: `IntentVpcDiscovery`, `IntentElbDiscovery`, `IntentRoute53Discovery`, `IntentRoute53Management`
- Messaging: `IntentSnsDiscovery`, `IntentSnsPublish`, `IntentSqsDiscovery`, `IntentSqsManagement`, `IntentEventbridgeDiscovery`
- Storage/Cache: `IntentDynamodbDiscovery`, `IntentDynamodbRead`, `IntentDynamodbWrite`, `IntentElasticacheDiscovery`
- Security/Identity: `IntentKmsDiscovery`, `IntentKmsCrypto`, `IntentIamDiscovery`, `IntentAcmDiscovery`
- Other: `IntentApigatewayDiscovery`, `IntentStepfunctionsDiscovery`, `IntentStepfunctionsExecution`, `IntentAthenaDiscovery`, `IntentAthenaQueryExecution`, `IntentGlueDiscovery`, `IntentCloudfrontDiscovery`, `IntentCodedeployDiscovery`, `IntentCostExplorer`

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
- `CommandExitStatus`: `CommandExitStatusError`, `CommandExitStatusFailure`, `CommandExitStatusInterrupted`, `CommandExitStatusInvalidExit`, `CommandExitStatusKilled`, `CommandExitStatusMisuse`, `CommandExitStatusNotExecutable`, `CommandExitStatusNotFound`, `CommandExitStatusSuccess`, `CommandExitStatusTerminated`
- `VaultMode`: `VaultModeScrubbed`, `VaultModeRaw`
- `ToolScope`: `ToolScopeOperatorGated`, `ToolScopeUniversal`
- `Platform`: `PlatformDarwin`, `PlatformLinux`, `PlatformWindows`
- `ComponentName`: `ComponentNameClient`, `ComponentNameG8EO`, `ComponentNameG8EOGateway`
- `SystemHealth`: `SystemHealthDegraded`, `SystemHealthHealthy`, `SystemHealthUnhealthy`, `SystemHealthUnknown`
- `NetworkProtocol`: `NetworkProtocolTCP`, `NetworkProtocolUDP`
- `Environment`: `EnvironmentDev`, `EnvironmentProduction`, `EnvironmentTest`
- `VersionStability`: `VersionStabilityBeta`, `VersionStabilityDev`, `VersionStabilityStable`
- `UserRole`: `UserRoleAdmin`, `UserRoleOperator`, `UserRoleOwner`, `UserRoleUser`
- `CAType`: `CATypeRoot`, `CATypeHub`, `CATypeOperator`, `CATypeGatewayPeer`
- `ServiceName`: `ServiceNameOperatorGateway`
- `GatewayMode`: `GatewayModeGateway`, `GatewayModeStatusOK`

### Headers (`auth.go`)

HTTP header names:

- Identity: `HeaderOperatorID`, `HeaderOperatorSessionID`, `HeaderWebSessionID`, `HeaderCLISessionID`, `HeaderUserID`, `HeaderOrganizationID`, `HeaderBoundOperators`
- Context: `HeaderCaseID`, `HeaderExecutionID`, `HeaderInvestigationID`, `HeaderTaskID`
- System: `HeaderRequestID`, `HeaderSourceComponent`, `HeaderSystemFingerprint`, `HeaderXAccelBuffering`
- Proxy: `HeaderXForwardedFor`, `HeaderXForwardedHost`, `HeaderXForwardedProto`, `HeaderXProxyOrganizationID`, `HeaderXProxyUserID`, `HeaderXRequestTimestamp`
- Security: `HeaderXContentTypeOptions`, `HeaderXFrameOptions`
- Standard: `HeaderAuthorization`, `HeaderContentType`, `HeaderAccept`, `HeaderCacheControl`, `HeaderCookie`, `HeaderUserAgent`
- CORS: `HeaderAccessControlAllowCredentials`, `HeaderAccessControlAllowOrigin`, `HeaderAccessControlRequestHeaders`, `HeaderAccessControlRequestMethod`

### Action Types (`action_types.go`)

GovernanceEnvelope action types:

- `ActionTypeA2aCall`, `ActionTypeCancel`, `ActionTypeEvalAnswer`, `ActionTypeExecuteBash`, `ActionTypeFetchFileDiff`, `ActionTypeFetchFileHistory`, `ActionTypeFetchHistory`, `ActionTypeFetchLogs`, `ActionTypeFileEdit`, `ActionTypeFsGrep`, `ActionTypeFsList`, `ActionTypeFsRead`, `ActionTypeGrantIntent`, `ActionTypeHeartbeat`, `ActionTypeInvestigationCreate`, `ActionTypeMcpCall`, `ActionTypeMcpPromptGet`, `ActionTypeMcpPromptList`, `ActionTypeMcpResourceList`, `ActionTypeMcpResourceRead`, `ActionTypePortCheck`, `ActionTypeRestoreFile`, `ActionTypeRevokeIntent`, `ActionTypeShutdown`

### Additional Constant Files

- `paths.go`: Filesystem paths for Operator data, certificates, ledger, and system paths.
- `ports.go`: Network ports `OperatorHttp` (8080), `OperatorHttps` (8443), and `InsecureMcpGateway` (18789).
- `exit_codes.go`: Process exit codes `ExitSuccess` (0), `ExitGeneralError` (1), `ExitAuthFailure` (2), `ExitPermissionDenied` (3).
- `errors.go`: Platform errors `ErrForbidden`, `ErrInternal`, `ErrNotFound`, `ErrAlreadyExists`, `ErrExpired`.
- `env_vars.go`: Typed environment variable names (currently empty).
- `field_paths.go`: Access control field paths for collections.
- `agents.go`: Agent persona details.
- `rpc_errors.go`: RPC error constants.

## JSON Reference Files

The `protocol/constants/` directory contains JSON files mirroring Go constants for protocol documentation and SDK generation.

Key JSON files: `collections.json`, `events.json`, `channels.json`, `intents.json`, `status.json`, `headers.json`, `api_paths.json`, `ports.json`, `agents.json`, `document_ids.json`, `env_vars.json`, `field_paths.json`, `kv_keys.json`, `platform.json`, `prompts.json`, `pubsub.json`, `senders.json`, `timestamp.json`.

## Protocol Generation

### Generate Protocol Artifacts

```bash
make generate
```

Generates Go Protobuf code from `.proto` files using Buf.

### Generate Python Protocol

```bash
make proto-python
```

Generates Python Protobuf code for the Python protocol SDK.

## CI Integration

Constants are validated in CI via `G8E_STRICT_CONSTANTS_LINT`. The pipeline includes:

- Proto verification (`make _ci-verify-proto`)
- Doctrine validation (`make validate-doctrines`)
- Linting (`make lint`)
- Testing (`make _ci-test`)

## Adding New Constants

1. **Add to Go file** in `internal/constants/`.
2. **Update JSON file** in `protocol/constants/` for public protocol constants.
3. **Run tests** to verify integration.
4. **Commit both** Go source and JSON reference files.

Follow these guidelines:
- Use typed constants.
- Group related constants.
- Add documentation comments.
- Follow Go naming conventions.
- Update protocol JSON for public constants.
