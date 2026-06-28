# Constants System

## Overview

The g8e constants system maintains canonical constant definitions across the platform. Constants are defined in Go source files in `internal/constants/` and referenced by JSON schemas in `protocol/constants/` for protocol documentation and external consumers. The Go source files serve as the authoritative registry for internal usage, while the JSON files provide the single source of truth (SSOT) for protocol-level constants consumed by SDKs and external integrations.

## Constant Categories

### Database Collections (`collections.go`)

Canonical collection names for the operator embedded SQLite database, typed as `CollectionName`:

- `CollectionUsers`, `CollectionWebSessions`, `CollectionOperatorSessions`, `CollectionCLISessions`
- `CollectionLoginAudit`, `CollectionAuthAdminAudit`, `CollectionAccountLocks`
- `CollectionOrganizations`, `CollectionOperators`, `CollectionOperatorUsage`
- `CollectionCases`, `CollectionInvestigations`, `CollectionTasks`
- `CollectionMemories`, `CollectionSettings`, `CollectionConsoleAudit`, `CollectionBoundSessions`
- `CollectionPasskeyChallenges`, `CollectionPersonas`, `CollectionAgentActivityMetadata`
- `CollectionReputationState`, `CollectionReputationCommitments`, `CollectionStakeResolutions`
- `CollectionRevokedCertificates`, `CollectionTrustedSigners`, `CollectionAppPolicies`
- `CollectionTribunals`

### Event Types (`events.go`)

Typed event identifiers for the pub/sub system, typed as `EventType`. The file defines approximately 280 individual event constants organized across the following categories:

- App Case: `EventAppCaseCreated`, `EventAppCaseUpdated`, `EventAppCaseAssigned`, `EventAppCaseEscalated`, `EventAppCaseResolved`, `EventAppCaseClosed`, `EventAppCaseSelected`, `EventAppCaseCleared`, `EventAppCaseSwitched`, `EventAppCaseCreationRequested`, `EventAppCaseUpdateRequested`
- App Task: `EventAppTaskCreated`, `EventAppTaskUpdated`, `EventAppTaskAssigned`, `EventAppTaskStarted`, `EventAppTaskCompleted`, `EventAppTaskFailed`
- App Investigation: `EventAppInvestigationCreated`, `EventAppInvestigationUpdated`, `EventAppInvestigationLoaded`, `EventAppInvestigationRequested`, `EventAppInvestigationStarted`, `EventAppInvestigationClosed`, `EventAppInvestigationEscalated`, plus list request/response and status update variants, and chat message events (`user`, `ai`, `system`)
- Operator Heartbeat: `EventOperatorHeartbeatSent`, `EventOperatorHeartbeatRequested`, `EventOperatorHeartbeatReceived`, `EventOperatorHeartbeatMissed`
- Operator Command: `EventOperatorCommandRequested`, `EventOperatorCommandStarted`, `EventOperatorCommandCompleted`, `EventOperatorCommandFailed`, `EventOperatorCommandCancelled`, `EventOperatorCommandExecution`, `EventOperatorCommandResult`, `EventOperatorCommandOutputReceived`, plus status update variants (queued, running, completed, failed, cancelled) and cancel lifecycle events
- Operator Command Approval: `EventOperatorCommandApprovalRequested`, `EventOperatorCommandApprovalGranted`, `EventOperatorCommandApprovalRejected`, `EventOperatorCommandApprovalPreparing`
- Operator Stream Approval: `EventOperatorStreamApprovalRequested`, `EventOperatorStreamApprovalGranted`, `EventOperatorStreamApprovalRejected`
- Operator File Edit: requested, started, completed, failed, timeout, and approval lifecycle events (requested, granted, rejected, feedback)
- Operator Filesystem Operations: list, read, and grep events, each with started, requested, received, completed, and failed variants
- Operator File History/Diff/Restore: fetch and restore event lifecycles with started, requested, received, completed, and failed variants
- Operator Logs/History Fetch: requested, received, completed, failed
- Operator Network: ping and port check event lifecycles
- Operator Status: `EventOperatorStatusUpdatedActive`, `EventOperatorStatusUpdatedAvailable`, `EventOperatorStatusUpdatedUnavailable`, `EventOperatorStatusUpdatedBound`, `EventOperatorStatusUpdatedOffline`, `EventOperatorStatusUpdatedStale`, `EventOperatorStatusUpdatedStopped`, `EventOperatorStatusUpdatedTerminated`
- Operator Bootstrap: requested, received, completed, failed, config.received
- Operator Audit: `EventOperatorAuditUserRecorded`, `EventOperatorAuditAiRecorded`, `EventOperatorAuditCommandRecorded`, `EventOperatorAuditDirectCommandRecorded`, `EventOperatorAuditDirectCommandResultRecorded`, `EventOperatorAuditMcpCallRecorded`
- Operator Notary: `EventOperatorNotaryApprovalRequested`, `EventOperatorNotaryTransactionExpired`
- Operator Reputation: commitment created/verified/failed, state updated, slash tier1/tier2/tier3
- Operator Terminal: thinking append/complete, approval denied, auth state changed
- Operator Device: `EventOperatorDeviceRegistered`
- Operator Bound/Unbound: `EventOperatorBound`, `EventOperatorUnbound`
- Operator Field Read: access denied, access granted, requested
- AI Agent Continue Approval: requested, granted, rejected
- AI Agent Conflict: detected, resolved
- AI Triage Clarification: questions, answered, skipped, timeout
- AI LLM Config: requested, received, failed
- AI LLM Lifecycle: requested, started, completed, failed, stopped, error occurred
- AI LLM Tools: web search, investigation query, and command constraints event lifecycles
- AI LLM Chat: submitted, stop show/hide, filter event, message sent/replayed/processing failed/dead lettered, iteration lifecycle events (started, completed, failed, stopped, retry), thinking lifecycle events, citations received, text received/chunk received/completed/truncated, stream lifecycle events
- Platform: usage updated, notification
- Platform Auth: login requested/succeeded/failed, logout requested/succeeded/failed, session validation requested/succeeded/failed, session expired, user authenticated/unauthenticated, component initialized (authstate, chat, operator), auth info
- Platform SSE: keepalive sent, connection established/opened/closed/failed/error
- Platform Terminal: opened, minimized, maximized, closed
- Platform Vault: `EventPlatformVaultModeChanged` (`g8e.v1.platform.sentinel.mode.changed`)
- Platform External Service: configured
- Platform Telemetry: health reported, performance recorded, error logged, audit logged
- Platform Console Log: entry received, connected confirmed
- Source: `EventSourceUserChat`, `EventSourceUserTerminal`, `EventSourceAiPrimary`, `EventSourceAiAssistant`, `EventSourceAiTriage`, `EventSourceSystem`

The file also provides a hierarchical `Event` struct accessor (`Event.Operator.*`) that groups operator event constants into nested sub-structs by domain (e.g., `Event.Operator.Command.ApprovalRequested`, `Event.Operator.FileEdit.Completed`, `Event.Operator.NetworkPing.Received`).

### API Paths (`api_paths.go`)

HTTP route paths for the Gateway REST API, defined as a struct `APIPaths` with JSON tags:

- Prefixes: `InternalPrefix` (`/api/v1`), `OperatorPrefix` (`/api`)
- Client map: `chat` (`/api/v1/chat`), `health` (`/api/v1/health`), `sse_events` (`/api/v1/internal/sse/events`), `sse_stream` (`/api/v1/internal/sse/stream`)
- MCP: `MCPEndpoint` (`/mcp`)
- A2A: `A2ACall` (`/api/v1/a2a/call`), `A2APrefix` (`/api/v1/a2a/`)
- Governance: `GovernanceEnvelopes`, `GovernanceSigners`, `GovernanceSignersByID`, `GovernanceSignersPrefix`
- Operator: `Operators`, `OperatorsByID`, `OperatorsBind`, `OperatorsUnbind`, `OperatorsTarget`, `OperatorsReauth`
- Data: `DataSettings`, `DataDB`, `DataBlobs`, `DataPrefix`, `DataItems`, `DataBlobsPrefix`, `QueryPrefix` (`/_query`)
- KV: `KV` (`/api/v1/kv/`), `KVPrefix` (`/api/v1/kv/`)
- PubSub: `PubSubPublish`, `PubSubStream`
- SSE: `SSEPush`, `SSEEvents`, `SSEStream`
- PKI: `PKICSRSign`, `PKIDevicesEnroll`, `PKIAppsEnroll`, `PKIAppsDelegated`, `PKICertificatesRevoke`, `PKIRevocationBundle`, `PKICRL`, `PKICABundle`, `PKIFingerprint`
- Audit: `AuditReceipts`, `AuditReceiptsExport`, `AuditEvents`, `AuditSummary`, `AuditReport`
- User: `Users`, `UsersMe`
- Auth: `AuthLogout`, `AuthBootstrap`, `AuthBootstrapStatus`, `AuthCLIEnroll`, `AuthDeviceEnroll`, `AuthPasskeys`, `AuthPasskeysByID`, `AuthPasskeysJITRegisterChallenge`, `AuthPasskeysJITRegisterVerify`, `AuthPasskeysJITPrefix`, `AuthPasskeysPrefix`, `AuthPasskeysCLIStatus`, `AuthPasskeysConsoleRegisterChallenge`, `AuthPasskeysConsoleRegisterVerify`, `AuthPasskeysConsoleAuthenticateChallenge`, `AuthPasskeysConsoleAuthenticateVerify`, `AuthPasskeysConsolePrefix`, `AuthSessionsMe`
- Approval: `Approvals`, `ApprovalsByID`, `ApprovalsPrefix`, `ApprovePage`, `ApprovePagePrefix`
- Admin: `AdminAppPoliciesBySigner`, `AdminAppsRevoke`, `AdminAppPoliciesPrefix`, `AdminTribunals`, `AdminTribunalsByID`, `AdminTribunalsPrefix`
- Tribunal: `TribunalDeliberate` (`/tribunal/v1/deliberate`)
- Well-known: `WellKnownPKICABundle`, `WellKnownPKIFingerprint`, `WellKnownBinPrefix`, `WellKnownPKIPrefix`, `WellKnownTrustWindows`
- Bootstrap scripts: `BootstrapCALinux` (`/bootstrap-ca`), `BootstrapCAMacos` (`/bootstrap-ca-macos`), `BootstrapCAWindows` (`/bootstrap-ca.ps1`)
- Deploy scripts: `DeployScriptLinux` (`/g8e-operator.sh`), `DeployScriptWindows` (`/g8e-operator.ps1`)
- Health: `Health` (`/api/v1/health`)
- State: `State` (`/api/v1/state`)
- Landing: `Landing` (`/`)

### Channels (`channels.go`)

Pub/sub channel names and wire protocol constants for inter-component communication:

- Session channel prefixes: `ChannelPrefixCmd` (`cmd`), `ChannelPrefixResults` (`results`), `ChannelPrefixHeartbeat` (`heartbeat`). Channel naming convention: `cmd:{operator_id}:{operator_session_id}`, `results:{operator_id}:{operator_session_id}`, `heartbeat:{operator_id}:{operator_session_id}`.
- Storage: `ChannelStorageDocument`, `ChannelStorageKv`, `ChannelStorageBlob`
- Governance: `ChannelGovernance`, `ChannelOperatorIntent`, `ChannelOperatorDevice`
- SSE: `ChannelSseEvent`
- Wire protocol actions: `PubSubActionSubscribe`, `PubSubActionPSubscribe`, `PubSubActionUnsubscribe`, `PubSubActionPublish`
- Wire protocol events: `PubSubEventMessage`, `PubSubEventPMessage`, `PubSubEventSubscribed`

### Intents (`intents.go`)

Cloud provider intent classification values for governance posture, typed as `CloudIntent`:

- EC2: `IntentEc2Discovery`, `IntentEc2Management`, `IntentEc2SnapshotManagement`
- S3: `IntentS3Read`, `IntentS3Write`, `IntentS3BucketDiscovery`, `IntentS3Delete`
- IaC: `IntentTerraformState`, `IntentCloudformationDeployment`
- Secrets: `IntentSecretsRead`
- CloudWatch: `IntentCloudwatchLogs`, `IntentCloudwatchMetrics`
- RDS: `IntentRdsDiscovery`, `IntentRdsManagement`, `IntentRdsSnapshotManagement`
- Aurora: `IntentAuroraClusterManagement`, `IntentAuroraScaling`, `IntentAuroraCloning`, `IntentAuroraGlobalDatabase`
- Compute: `IntentLambdaDiscovery`, `IntentLambdaInvoke`, `IntentEcsDiscovery`, `IntentEcsManagement`, `IntentEksDiscovery`
- Autoscaling: `IntentAutoscalingDiscovery`, `IntentAutoscalingManagement`
- Network: `IntentVpcDiscovery`, `IntentElbDiscovery`, `IntentRoute53Discovery`, `IntentRoute53Management`
- Messaging: `IntentSnsDiscovery`, `IntentSnsPublish`, `IntentSqsDiscovery`, `IntentSqsManagement`, `IntentEventbridgeDiscovery`
- Storage/Cache: `IntentDynamodbDiscovery`, `IntentDynamodbRead`, `IntentDynamodbWrite`, `IntentElasticacheDiscovery`
- Security/Identity: `IntentKmsDiscovery`, `IntentKmsCrypto`, `IntentIamDiscovery`, `IntentAcmDiscovery`
- Other: `IntentApigatewayDiscovery`, `IntentStepfunctionsDiscovery`, `IntentStepfunctionsExecution`, `IntentAthenaDiscovery`, `IntentAthenaQueryExecution`, `IntentGlueDiscovery`, `IntentCloudfrontDiscovery`, `IntentCodedeployDiscovery`, `IntentCostExplorer`

### Status Enums (`status.go`)

Internal enumeration constants, each defined as a typed string:

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
- `CommandExitStatus`: `CommandExitStatusError`, `CommandExitStatusFailure`, `CommandExitStatusInterrupted`, `CommandExitStatusInvalidExit`, `CommandExitStatusKilled`, `CommandExitStatusMisuse`, `CommandExitStatusNotExecutable`, `CommandExitStatusNotFound`, `CommandExitStatusSuccess`, `CommandExitStatusTerminated`, `CommandExitStatusSignal1` (SIGHUP), `CommandExitStatusSignal2` (SIGINT), `CommandExitStatusSignal3` (SIGQUIT), `CommandExitStatusSignal6` (SIGABRT), `CommandExitStatusSignal9` (SIGKILL), `CommandExitStatusSignal11` (SIGSEGV), `CommandExitStatusSignal13` (SIGPIPE), `CommandExitStatusSignal15` (SIGTERM)
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
- `ThinkingActionType`: `ThinkingActionTypeEnd`, `ThinkingActionTypeStart`, `ThinkingActionTypeUpdate`
- `HistoryEventType`: `HistoryEventTypeAPIKeyRefreshed`, `HistoryEventTypeAuthenticated`, `HistoryEventTypeBound`, `HistoryEventTypeClaimed`, `HistoryEventTypeCreated`, `HistoryEventTypeCreatedFromRefresh`, `HistoryEventTypeDeactivated`, `HistoryEventTypeHeartbeatReceived`, `HistoryEventTypeReconnected`, `HistoryEventTypeRegistered`, `HistoryEventTypeReset`, `HistoryEventTypeShutdownRequested`, `HistoryEventTypeSlotConsumed`, `HistoryEventTypeSlotCreated`, `HistoryEventTypeSlotReleased`, `HistoryEventTypeStatusChanged`, `HistoryEventTypeStopped`, `HistoryEventTypeTerminated`, `HistoryEventTypeTerminatedForRefresh`, `HistoryEventTypeUnbound`
- `HeartbeatType`: `HeartbeatTypeAutomatic`, `HeartbeatTypeBootstrap`, `HeartbeatTypeRequested`
- `AuthAuditResult`: `AuthAuditResultFailure`, `AuthAuditResultInvalidAPIKey`, `AuthAuditResultSuccess`
- `ToolDisplayCategory`: `ToolDisplayCategoryExecution`, `ToolDisplayCategoryFile`, `ToolDisplayCategoryGeneral`, `ToolDisplayCategoryNetwork`, `ToolDisplayCategorySearch`
- `SessionKeyPrefix`: `SessionKeyPrefixCLI`, `SessionKeyPrefixOperator`, `SessionKeyPrefixWeb`
- `SuspendedTxStatus`: `SuspendedTxStatusPending`, `SuspendedTxStatusApproved`
- `HistoryActor`: `HistoryActorNone`, `HistoryActorG8EO`, `HistoryActorSystem`, `HistoryActorUser`
- `AISource`: `AISourceTerminalAnchored`, `AISourceTerminalDirect`, `AISourceToolCall`

### Headers and Auth Constants (`auth.go`)

HTTP header names and authentication-related constants:

- Identity: `HeaderOperatorID`, `HeaderOperatorSessionID`, `HeaderWebSessionID`, `HeaderCLISessionID`, `HeaderUserID`, `HeaderOrganizationID`, `HeaderBoundOperators`
- Context: `HeaderCaseID`, `HeaderExecutionID`, `HeaderInvestigationID`, `HeaderTaskID`
- System: `HeaderRequestID`, `HeaderSourceComponent`, `HeaderSystemFingerprint`, `HeaderXAccelBuffering`
- Proxy: `HeaderXForwardedFor`, `HeaderXForwardedHost`, `HeaderXForwardedProto`, `HeaderXProxyOrganizationID`, `HeaderXProxyUserID`, `HeaderXRequestTimestamp`
- Security: `HeaderXContentTypeOptions`, `HeaderXFrameOptions`, `HeaderContentSecurityPolicy`
- Standard: `HeaderAuthorization`, `HeaderContentType`, `HeaderAccept`, `HeaderAcceptLanguage`, `HeaderCacheControl`, `HeaderCookie`, `HeaderUserAgent`, `HeaderPragma`, `HeaderSetCookie`
- Content: `HeaderContentDisposition`, `HeaderContentLanguage`, `HeaderContentLength`
- SSE: `HeaderLastEventID`
- AJAX: `HeaderRequestedWith`
- CORS: `HeaderAccessControlAllowCredentials`, `HeaderAccessControlAllowOrigin`, `HeaderAccessControlRequestHeaders`, `HeaderAccessControlRequestMethod`

Additional constants in `auth.go`:

- Passkey purposes: `PasskeyPurposeRegister`, `PasskeyPurposeAuth`
- WebAuthn algorithms: `WebAuthnAlgES256` (-7), `WebAuthnAlgRS256` (-257)
- WebAuthn types: `WebAuthnTypePublicKey`, `WebAuthnAttestationNone`, `WebAuthnResidentKeyRequired`, `WebAuthnUserVerificationRequired`
- PKI leaf types: `LeafTypeOperator`, `LeafTypeApp`, `LeafTypeHub`, `LeafTypeCLI`
- JSON-RPC 2.0: `JSONRPCVersion`, `JSONRPCFieldVersion`, `JSONRPCFieldMethod`, `JSONRPCFieldParams`, `JSONRPCFieldID`, `JSONRPCFieldResult`, `JSONRPCFieldError`, `JSONRPCFieldCode`, `JSONRPCFieldMessage`, `JSONRPCFieldData`, `JSONRPCErrorCodeInternal`
- Header values: `HeaderValueNoSniff`, `HeaderValueDeny`, `HeaderValueCSPNone`, `HeaderValueKeepAlive`, `HeaderValueNoCache`, `HeaderValueTextEvent`, `HeaderValueApplicationJSON`, `HeaderValueXHTML`, `HeaderValueXML`, `HeaderValueOctetStream`, `HeaderValuePEM`, `HeaderValueCRL`, `HeaderValueShell`, `HeaderValuePowerShell`
- Context keys (typed `ContextKey`): `ContextKeyUserID`, `ContextKeyAppID`, `ContextKeyTenantID`, `ContextKeyBindingPersona`, `ContextKeyOperatorID`, `ContextKeyOperatorSessionID`, `ContextKeyCapability`
- Auth error reasons (typed `AuthErrorReason`): `AuthErrorReasonTTLExceeded`, `AuthErrorReasonRetiredByRealLogin`, `AuthErrorReasonIdentityDisabled`, `AuthErrorReasonInvalidSession`, `AuthErrorReasonSessionExpired`, `AuthErrorReasonCertificateRevoked`, `AuthErrorReasonIdentityMismatch`, `AuthErrorReasonAppPolicyNotFound`, `AuthErrorReasonRateLimitExceeded`, `AuthErrorReasonPayloadTooLarge`, `AuthErrorReasonCollectionNotAllowed`, `AuthErrorReasonJWTInvalid`, `AuthErrorReasonJWTMissingSubject`
- Session TTL: `WebSessionTTL` (24 hours)

### Action Types (`action_types.go`)

GovernanceEnvelope action types, typed as `ActionType`. The file also defines `AllActionTypes` (a canonical slice of all valid action types) and an `IsMutation()` method that returns true for action types that modify system state:

- `ActionTypeA2aCall`, `ActionTypeCancel`, `ActionTypeEvalAnswer`, `ActionTypeExecuteBash`, `ActionTypeFetchFileDiff`, `ActionTypeFetchFileHistory`, `ActionTypeFetchHistory`, `ActionTypeFetchLogs`, `ActionTypeFileEdit`, `ActionTypeFsGrep`, `ActionTypeFsList`, `ActionTypeFsRead`, `ActionTypeHeartbeat`, `ActionTypeInvestigationCreate`, `ActionTypeMcpCall`, `ActionTypeMcpPromptGet`, `ActionTypeMcpPromptList`, `ActionTypeMcpResourceList`, `ActionTypeMcpResourceRead`, `ActionTypePortCheck`, `ActionTypeRestoreFile`, `ActionTypeShutdown`

Mutation action types: `ActionTypeA2aCall`, `ActionTypeCancel`, `ActionTypeExecuteBash`, `ActionTypeFileEdit`, `ActionTypeMcpCall`, `ActionTypeRestoreFile`, `ActionTypeShutdown`.

### Paths (`paths.go`)

Filesystem paths for Operator data, certificates, ledger, system paths, and configuration. This is the largest constant file in the package, containing:

- System paths: Linux filesystem paths (`/etc`, `/proc`, `/var`, `/dev`, `/boot`, `/tmp`, `/opt`, `/home`, `/usr`, `/bin`, `/sbin`, `/lib`), including specific files (`/etc/passwd`, `/etc/hosts`, `/etc/resolv.conf`, `/proc/meminfo`, `/proc/loadavg`, etc.)
- SSH paths: `PathEtcSshKnownHosts`, `PathEtcSshSshKnownHosts`, `PathHomeSshKnownHosts`, `PathWindowsSshKnownHosts`, `PathWindowsProgramDataSsh`
- Windows paths: `PathWindowsSystemRoot`, `PathWindowsHostsFile`, `PathWindowsRegistryCryptography`, `PathWindowsRegistryMachineGuid`, Git Bash paths (`PathWindowsGitBinBash`, `PathWindowsGitUsrBinBash`, `PathWindowsGitBinSh`, `PathWindowsMsys64Bash`, `PathWindowsCygwin64Bash`)
- PKI filesystem constants: directory names (`PkiDirname`, `PkiSubdirRoot`, `PkiSubdirAuthorities`, `PkiSubdirIssued`, `PkiSubdirTrust`, `PkiSubdirRevocation`, `PkiSubdirBinaries`, `PkiSubdirClient`, `PkiSubdirHub`, `PkiSubdirGatewayPeer`, `PkiSubdirApps`, `PkiSubdirTrustedSigners`), file extensions (`FileExtCert`, `FileExtKey`, `FileExtPEM`, `FileExtJSON`), and filenames for CA certificates, bundles, operator credentials, gateway credentials, and bootstrap certificates
- Database filenames: `DbFilename` (`g8e.db`), `VaultKeyFilename`, `VaultHeaderFilename`, `SuspendedTxFilename`, `ReceiptsFilename`, `ReceiptsExportFilename`
- Secrets filenames: `SecretsFileSessionEncryptionKey`, `SecretsFileBootstrapDigest`, `SecretsFileActuatorSigningKey`, `SecretsFileActuatorKeyID`, `SecretsFileAuditorHMACKey`, `SecretsFileNotarySigningKey`, `SecretsFileOperatorPrivateKey`, `SecretsFileCLIPrivateKey`, `SecretsFileSessionToken`
- Demos: `DemosDirname`, `DemosComposeFile`, `DemosBinDirname`, `DemosBinaryName`, `DemosTargetDataDir`, `DemosDoctrineDir`, `DemosPARequestsFile`, `DemosHIPAADoctrineFile`, `DemosSecureDataDoctrineFile`, `DemosOrgHealthcare`, `DemosOrgFinance`, `DemosOrgGov`, `DemosOrgSecureData`
- Runtime directories: `RuntimeDirname` (`.g8e`), `DataDirname`, `VaultDirname`, `SecretsDirname`, `LedgerDirname`, `SshDirname`, `PidDirname`
- Agent config directories: `AgentConfigDirCursor`, `AgentConfigDirDevin`, `AgentConfigDirGemini`, `AgentConfigDirGoose`, `AgentConfigDirVSCode`, `AgentConfigDirCodeium`, `AgentConfigDirTabby`, `AgentConfigDirContinue`
- Agent config files: `AgentConfigFileMCP`, `AgentConfigFileMCPDevin`, `AgentConfigFileSettings`, `AgentConfigFileAider`
- File permission modes: `PermDirPrivate` (0700), `PermDirStandard` (0755), `PermFilePrivate` (0600), `PermFilePublic` (0644)
- Filesystem listing limits: `FsListMaxDepth` (3), `FsListDefaultDepth` (0), `FsListMaxEntries` (500), `FsListDefaultEntries` (100), `FsListBatchSize` (100)
- Grep limits: `FsGrepDefaultMaxMatches` (100), `FsGrepMaxMatches` (500), `FsGrepScannerInitialBufSize` (64 KiB), `FsGrepScannerMaxBufSize` (1 MiB)
- Execution limits: `ExecutionMaxStreamSize` (10 MB), `ExecutionMaxLines` (50), `ExecutionPreviewLength` (300), `FileEditMaxSize` (50 MB)
- Reporting: `ReportsDirname` and CSV output filenames for receipts, sessions, events, file mutations, executions, file diffs, commitments, ledger commits, ledger merkle root, replay nonces, suspended transactions, verification summary, and manifest
- Miscellaneous: `SwaggerFilename`, `ComplianceReportFilename`, `TmpFileSuffix`, `BackupFileSuffixPattern`, `SQLiteWALSuffix`, `SQLiteSHMSuffix`, `EnvPathDefault`, and test-specific constants

### Ports (`ports.go`)

Network ports defined as a struct `Ports` with JSON tags:

- `OperatorHttp` (8080)
- `OperatorHttps` (8443)

### Exit Codes (`exit_codes.go`)

Operator process exit codes:

- `ExitSuccess` (0), `ExitGeneralError` (1), `ExitAuthFailure` (2), `ExitPermissionDenied` (3), `ExitNetworkError` (4), `ExitConfigError` (5), `ExitStorageError` (6), `ExitCertTrustFailure` (7)

Unix shell exit codes for command execution:

- `ExitCodeSuccess` (0), `ExitCodeGeneral` (1), `ExitCodeUsage` (2), `ExitCodeTimeout` (124), `ExitCodeCannotExecute` (126), `ExitCodeCommandNotFound` (127), `ExitCodeKilled` (137), `ExitCodeNone` (-1)

### Errors (`errors.go`)

Platform error variables defined as `error` values using `errors.New()`. The file contains approximately 50 error variables including:

- `ErrUserNotFound`, `ErrNoPasskeysRegistered`, `ErrInvalidJSONBody`, `ErrUserIDRequired`, `ErrMethodNotAllowed`, `ErrForbidden`, `ErrInternal`, `ErrNotFound`, `ErrAlreadyExists`, `ErrConstraintViolation`, `ErrDatabaseLocked`, `ErrServiceUnavailable`, `ErrDatabaseReplay`, `ErrDuplicateColumn`, `ErrProcessKilled`, `ErrTrustBundleStale`, `ErrKeyNotFound`, `ErrExpired`, `ErrAgentNotFound`, `ErrAgentNotInPath`, `ErrAgentNotSupported`, `ErrConfigFileExists`, `ErrEndpointRequired`, `ErrGatewayURLRequired`, `ErrConfigLoadFailed`, `ErrCSRGenerationFailed`, `ErrEnrollmentFailed`, `ErrMissingCertificate`, `ErrDirCreateFailed`, `ErrPKIDirRequired`, `ErrCertSaveFailed`, and additional error variables for CLI operations, enrollment, and configuration validation.

### Environment Variables (`env_vars.go`)

Typed environment variable names, typed as `EnvVarKey` and grouped in a struct `EnvVar`:

- `TribunalID` (`G8E_TRIBUNAL_ID`)
- `TribunalURL` (`G8E_TRIBUNAL_URL`)

### Field Paths (`field_paths.go`)

Access control field path constants for collections:

- `FieldPathInvestigations` (`investigations`)
- `FieldPathMemories` (`memories`)
- `FieldPathCases` (`cases`)

### Agents (`agents.go`)

Agent persona and triage classification constants:

- `TriageComplexity` (typed): `TriageComplexitySimple`, `TriageComplexityComplex`
- `TriageConfidence` (typed): `TriageConfidenceHigh`, `TriageConfidenceLow`
- `TriageIntent` (typed): `TriageIntentInformation`, `TriageIntentAction`, `TriageIntentUnknown`
- `TriagePosture` (typed): `TriagePostureNormal`, `TriagePostureEscalated`, `TriagePostureAdversarial`, `TriagePostureConfused`
- `AgentName` (typed): `AgentNameSage` (`sage`), `AgentNameDash` (`dash`)
- `AgentBinary` (typed): `AgentBinaryClaude`, `AgentBinaryCodex`, `AgentBinaryCursor`, `AgentBinaryDevin`, `AgentBinaryVSCode`, `AgentBinaryContinue`, `AgentBinaryContinueAlias` (`cn`), `AgentBinaryAider`, `AgentBinaryCodeium`, `AgentBinaryTabby`, `AgentBinaryOllama`, `AgentBinaryGemini`, `AgentBinaryGoose`, `AgentBinaryGeneric`

### RPC Errors (`rpc_errors.go`)

Protocol-specific error codes for g8e, reserved in the range -32000 to -32101:

- Verification errors: `ErrCodeInvalidEnvelope` (-32000), `ErrCodeHashMismatch` (-32001), `ErrCodeExpired` (-32002), `ErrCodeReplay` (-32003), `ErrCodeStateMismatch` (-32004), `ErrCodeL1ValidationFailed` (-32005), `ErrCodeL2SignatureInvalid` (-32006), `ErrCodeL3ProofInvalid` (-32007), `ErrCodePayloadDecodeFailed` (-32008)
- Resource/state errors: `ErrCodeResourceNotFound` (-32100), `ErrCodeGatewayNotReady` (-32101)

### Document IDs (`document_ids.go`)

Canonical document ID constants, typed as `DocumentID`:

- `DocIDPlatformSettings` (`platform_settings`)
- `DocIDUserSettingsPrefix` (`user_settings_`)

### KV Keys (`kv_keys.go`)

Key-value store key patterns and session type constants:

- Cache prefix: `KVCachePrefix` (`g8e`)
- Cache keys: `KVKeyCacheDoc`, `KVKeyCacheQuery`
- Session keys: `KVKeySessionWeb`, `KVKeySessionOperatorBind`, `KVKeySessionWebBind`
- Operator keys: `KVKeyOperatorFirstDeployed`, `KVKeyOperatorTrackedStatus`
- User keys: `KVKeyUserOperators`, `KVKeyUserWebSessions`, `KVKeyUserMemories`
- Investigation keys: `KVKeyInvestigationAttachment`, `KVKeyInvestigationAttachmentIndex`
- Auth keys: `KVKeyAuthNonce`, `KVKeyAuthTokenDownload`, `KVKeyAuthTokenDevice`, `KVKeyAuthTokenDeviceUses`, `KVKeyAuthTokenDeviceFingerprints`, `KVKeyAuthTokenDeviceRegLock`, `KVKeyAuthDeviceList`, `KVKeyAuthLoginFailed`, `KVKeyAuthLoginLock`, `KVKeyAuthLoginIPAccounts`
- Execution keys: `KVKeyExecutionPendingCmd`
- Session types: `KVSessionTypeWeb` (`web`), `KVSessionTypeOperator` (`operator`)

### MCP Service Constants (`mcp.go`)

Service-level constants for MCP tool execution:

- `DefaultLogFilterLimit` (100)
- `DefaultProcessLimit` (10)
- `DefaultDiskProfileDepth` (2)
- `DefaultNetworkTimeout` (5 seconds)
- `DefaultHTTPTimeout` (10 seconds)
- `SSHKeepaliveRequestType` (`keepalive@g8e`)
- `SSHKeepaliveInterval` (15 seconds)
- `SSHKeepaliveMaxMissed` (3)

### Network (`network.go`)

Network-related constants:

- `DefaultEndpoint` (`localhost`): default hostname and TLS ServerName for raw IP connections
- `GatewayHTTPPort` (`8080`), `GatewayHTTPSPort` (`8443`), `GatewayHTTPBase`, `GatewayHTTPSBase`
- `GatewayInternalHostname` (`g8e.local`): internal hostname for Gateway TLS connections
- `TransientNetworkErrorPatterns`: list of substring patterns identifying transient network errors suitable for retry logic

### Output (`output.go`)

Output formatting constants:

- `TruncatedOutputFormat`: format string used when large command output is head/tail truncated before storage

### Platform (`platform.go`)

Platform event identifiers and binary/architecture constants:

- Platform event string constants mirroring `events.go` platform events (usage, notification, auth, SSE, terminal, vault mode, external service, telemetry, console log)
- Binary names: `BinaryNameWindows`, `BinaryNameLinux`, `BinaryNameDarwin`
- Architectures: `ArchAMD64`, `ArchARM64`, `Arch386`
- Operating systems: `OSLinux`, `OSDarwin`, `OSWindows`

### Prompts (`prompts.go`)

Agent mode and prompt section identifier constants:

- Agent modes: `AgentModeG8eBound` (`g8e.bound`), `AgentModeG8eNotBound` (`g8e.not.bound`), `AgentModeCloudOperatorBound` (`g8e.cloud.bound`)
- Prompt sections: `PromptSectionIdentity`, `PromptSectionSafety`, `PromptSectionLoyalty`, `PromptSectionDissent`, `PromptSectionCapabilities`, `PromptSectionExecution`, `PromptSectionTools`, `PromptSectionDocs`, `PromptSectionSystemContext`, `PromptSectionVaultMode`, `PromptSectionTriageContext`, `PromptSectionInvestigationContext`, `PromptSectionResponseConstraints`, `PromptSectionLearnedContext`, `PromptSectionAgentPersona`

### PubSub (`pubsub.go`)

PubSub field constants (actions and events are in `channels.go`):

- Fields: `PubSubFieldAction`, `PubSubFieldChannel`, `PubSubFieldData`, `PubSubFieldMessage`, `PubSubFieldPattern`, `PubSubFieldType`, `PubSubFieldSender`
- `ReceiptSummaryMaxBytes` (4096): maximum size for receipt summary text

### Senders (`senders.go`)

Sender and message type constants:

- Source identifiers: `SourceUserChat`, `SourceUserTerminal`, `SourceAiPrimary`, `SourceAiAssistant`, `SourceAiTriage`, `SourceSystem`
- Message types: `MessageTypeText`, `MessageTypeCode`, `MessageTypeCall`, `MessageTypeResult`, `MessageTypeError`, `MessageTypeThinking`

### Shell (`shell.go`)

Shell command execution constants:

- `DefaultShellCommandTimeout` (30 seconds), `MaxShellCommandTimeout` (300 seconds)
- `LocalhostHostname` (`localhost`), `LocalhostIP` (`127.0.0.1`)
- `RemoteEphemeralScriptTemplate`: bash script template for remote Operator deployment with graceful cleanup
- `RemoteInjectedBinaryMessage`, `RemoteInjectedScriptMinimal`: constants for binary injection without execution
- `DangerousCommands`: list of commands blocked by safety policy (rm, dd, mkfs, fdisk, format, del, erase, shred, wipe, killall, pkill, reboot, shutdown, halt, poweroff, init, systemctl, service, iptables, ip6tables, nft, ufw, firewall-cmd, route, ifconfig, ip, brctl, tc, modprobe, insmod, rmmod, depmod, mount, umount, swapon, swapoff, mkswap, lvcreate, lvremove, lvchange, vgcreate, vgremove, pvcreate, pvremove, cryptsetup, passwd, chpasswd, usermod, userdel, groupmod, crontab, at, batch, sudo, su, doas, runuser)
- `DangerousPatterns`: list of command patterns blocked by safety policy (rm -rf /, dd if=/dev/zero, mkfs, chmod 777 /, chown -R, wget, curl, nc -l, ncat -l, ssh, scp, rsync, etc.)
- `ShellInjectionPatterns`: `$(`, backtick, `|`
- `ShellMetacharacters`: `$`, backtick, `\`, `;`, `&`, `|`, `>`, `<`, newline, carriage return

### Timestamp (`timestamp.go`)

Timestamp format constants:

- `FormatRFC3339`: canonical RFC3339 format string with timezone offset
- `TimestampFormat`: RFC3339 with nanosecond precision (`time.RFC3339Nano`)

## JSON Reference Files

The `protocol/constants/` directory contains JSON files mirroring Go constants for protocol documentation and SDK generation. A `doctrine/` subdirectory holds doctrine JSON files for governance validation.

JSON files: `agents.json`, `api_paths.json`, `auth.json`, `channels.json`, `collections.json`, `document_ids.json`, `env_vars.json`, `events.json`, `exit_codes.json`, `field_paths.json`, `headers.json`, `intents.json`, `kv_keys.json`, `network.json`, `output.json`, `platform.json`, `ports.json`, `prompts.json`, `pubsub.json`, `senders.json`, `status.json`, `timestamp.json`.

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
