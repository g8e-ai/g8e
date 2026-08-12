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
- `CollectionConsensus`, `CollectionEnrollmentTokens`

### Event Types (`events.go`)

Typed event identifiers for the pub/sub system, typed as `EventType`. The file defines approximately 300 individual event constants organized across the following categories:

- App Case: `EventAppCaseCreated`, `EventAppCaseUpdated`, `EventAppCaseAssigned`, `EventAppCaseEscalated`, `EventAppCaseResolved`, `EventAppCaseClosed`, `EventAppCaseSelected`, `EventAppCaseCleared`, `EventAppCaseSwitched`, `EventAppCaseCreationRequested`, `EventAppCaseUpdateRequested`
- App Task: `EventAppTaskCreated`, `EventAppTaskUpdated`, `EventAppTaskAssigned`, `EventAppTaskStarted`, `EventAppTaskCompleted`, `EventAppTaskFailed`
- App Investigation: `EventAppInvestigationCreated`, `EventAppInvestigationUpdated`, `EventAppInvestigationLoaded`, `EventAppInvestigationRequested`, `EventAppInvestigationStarted`, `EventAppInvestigationClosed`, `EventAppInvestigationEscalated`, plus list request/response and status update variants, and chat message events (`user`, `ai`, `system`)
- Operator Heartbeat: `EventOperatorHeartbeatSent`, `EventOperatorHeartbeatRequested`, `EventOperatorHeartbeatReceived`, `EventOperatorHeartbeatMissed`
- Operator Eval: `EventOperatorEvalAnswerRequested`
- Operator Shutdown: `EventOperatorShutdownRequested`, `EventOperatorShutdownAcknowledged`
- Operator Panel/Context: `EventOperatorPanelListUpdated`, `EventOperatorContextChanged`, `EventOperatorSlotInitializationFailed`
- Operator Command: `EventOperatorCommandRequested`, `EventOperatorCommandStarted`, `EventOperatorCommandCompleted`, `EventOperatorCommandFailed`, `EventOperatorCommandCancelled`, `EventOperatorCommandExecution`, `EventOperatorCommandResult`, `EventOperatorCommandOutputReceived`, plus status update variants (queued, running, completed, failed, cancelled) and cancel lifecycle events (requested, acknowledged, failed)
- Operator Command Approval: `EventOperatorCommandApprovalRequested`, `EventOperatorCommandApprovalGranted`, `EventOperatorCommandApprovalRejected`, `EventOperatorCommandApprovalPreparing`
- Operator Stream Approval: `EventOperatorStreamApprovalRequested`, `EventOperatorStreamApprovalGranted`, `EventOperatorStreamApprovalRejected`
- Operator File Edit: requested, started, completed, failed, timeout, and approval lifecycle events (requested, granted, rejected, feedback)
- Operator Filesystem Operations: list, read, and grep events, each with started, requested, received, completed, and failed variants
- Operator File History/Diff/Restore: fetch and restore event lifecycles with started, requested, received, completed, and failed variants
- Operator Logs/History Fetch: requested, received, completed, failed
- Operator MCP/A2A: `EventOperatorMcpCallRequested`, `EventOperatorA2aCallRequested`
- Operator Network: ping and port check event lifecycles, plus `EventOperatorPortCheckRequested`
- Operator Status: `EventOperatorStatusUpdatedActive`, `EventOperatorStatusUpdatedAvailable`, `EventOperatorStatusUpdatedUnavailable`, `EventOperatorStatusUpdatedBound`, `EventOperatorStatusUpdatedOffline`, `EventOperatorStatusUpdatedStale`, `EventOperatorStatusUpdatedStopped`, `EventOperatorStatusUpdatedTerminated`
- Operator Bootstrap: requested, received, completed, failed, config.received
- Operator Audit: `EventOperatorAuditUserRecorded`, `EventOperatorAuditAiRecorded`, `EventOperatorAuditCommandRecorded`, `EventOperatorAuditDirectCommandRecorded`, `EventOperatorAuditDirectCommandResultRecorded`, `EventOperatorAuditMcpCallRecorded`
- Operator Notary: `EventOperatorNotaryApprovalRequested`, `EventOperatorNotaryTransactionExpired`
- Operator Intent: `EventOperatorIntentRequested`, `EventOperatorIntentRevokeRequested`, `EventOperatorIntentApprovalRequested`, `EventOperatorIntentDenied`, `EventOperatorIntentGranted`, `EventOperatorIntentRevoked`, `EventOperatorIntentApprovalRejected`, `EventOperatorIntentApprovalGranted`. Values follow the `g8e.v1.operator.intent.*` namespace. Accessible via `Event.Operator.Intent.*`.
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
- AI Consensus Session: started, completed, disabled, generation failed, model not configured, provider unavailable, system error, auditor failed, warden blocked
- AI Consensus Voting: pass completed, consensus reached/not reached/failed, round started/completed, round 2 started/consensus reached/consensus failed, dissent recorded, audit started/completed
- App Memory: `EventAppMemoryCreated`, `EventAppMemoryUpdated`
- App Case/Investigation Deletion: `EventAppCaseDeleted`, `EventAppInvestigationDeleted`
- AI LLM Chat Thinking Stopped: `EventAiLLMChatIterationThinkingStopped`
- AI Reputation: `EventAiReputationStateUpdated`, `EventReputationStateUpdated`
- Source: `EventSourceUserChat`, `EventSourceUserTerminal`, `EventSourceAiPrimary`, `EventSourceAiAssistant`, `EventSourceAiTriage`, `EventSourceSystem`

The file also provides a hierarchical `Event` struct accessor (`Event.Operator.*`) that groups operator event constants into nested sub-structs by domain (e.g., `Event.Operator.Command.ApprovalRequested`, `Event.Operator.FileEdit.Completed`, `Event.Operator.NetworkPing.Received`). Top-level operator fields include `Bound`, `Unbound`, `ContextChanged`, `DeviceRegistered`, `Heartbeat`, `HeartbeatMissed`, `HeartbeatReceived`, `HeartbeatRequested`, `PanelListUpdated`, `ShutdownAcknowledged`, `ShutdownRequested`, `SlotInitializationFailed`, `TerminalApprovalDenied`, `TerminalAuthStateChanged`, `TerminalThinkingAppend`, `TerminalThinkingComplete`, and `BootstrapConfigReceived`. Sub-struct domains include `A2a`, `Audit`, `Bootstrap`, `Command` (with nested `StatusUpdated`), `Eval`, `FetchFileDiff`, `FetchFileHistory`, `FetchHistory`, `FetchLogs`, `FileEdit`, `FsGrep`, `FsList`, `FsRead`, `Intent`, `Mcp`, `NetworkPing`, `Notary`, `PortCheck`, `RestoreFile`, `StatusUpdated`, and `StreamApproval`.

### API Paths (`api_paths.go`)

HTTP route paths for the Gateway REST API, defined as a struct `APIPaths` with JSON tags:

- Prefixes: `InternalPrefix` (`/api/v1`), `OperatorPrefix` (`/api`)
- Client map: `chat` (`/api/v1/chat`), `health` (`/api/v1/health`), `sse_events` (`/api/v1/internal/sse/events`), `sse_stream` (`/api/v1/internal/sse/stream`)
- MCP: `MCPEndpoint` (`/mcp`)
- A2A: `A2ACall` (`/api/v1/a2a/call`), `A2APrefix` (`/api/v1/a2a/`)
- Governance: `GovernanceEnvelopes`, `GovernanceSigners`, `GovernanceSignersByID`, `GovernanceSignersPrefix`
- Operator: `Operators`, `OperatorsByID`, `OperatorsBind`, `OperatorsUnbind`, `OperatorsTarget`, `OperatorsReauth`, `GrantIntent`, `RevokeIntent`
- Data: `DataSettings`, `DataDB`, `DataBlobs`, `DataPrefix`, `DataItems`, `DataBlobsPrefix`, `QueryPrefix` (`/_query`)
- KV: `KV` (`/api/v1/kv/`), `KVPrefix` (`/api/v1/kv/`)
- PubSub: `PubSubPublish`, `PubSubStream`, `PubSubWebSocket` (`/ws/pubsub`)
- SSE: `SSEPush`, `SSEEvents`, `SSEStream`
- PKI: `PKICSRSign`, `PKIDevicesEnroll`, `PKIAppsEnroll`, `PKIAppsDelegated`, `PKICertificatesRevoke`, `PKIRevocationBundle`, `PKICRL`, `PKICABundle`, `PKIFingerprint`
- Audit: `AuditReceipts`, `AuditReceiptsExport`, `AuditEvents`, `AuditSummary`, `AuditReport`, `AuditStream`
- User: `Users`, `UsersMe`
- Auth: `AuthLogout`, `AuthBootstrap`, `AuthBootstrapStatus`, `AuthCLIEnroll`, `AuthCLIRecoveryRequest`, `AuthCLIRecoveryStatus`, `AuthCLIRecoveryApprove`, `AuthCLIRecoveryComplete`, `AuthCLIRotate`, `AuthDeviceEnroll`, `AuthPasskeys`, `AuthPasskeysByID`, `AuthPasskeysJITRegisterChallenge`, `AuthPasskeysJITRegisterVerify`, `AuthPasskeysJITPrefix`, `AuthPasskeysPrefix`, `AuthPasskeysCLIStatus`, `AuthPasskeysConsoleRegisterChallenge`, `AuthPasskeysConsoleRegisterVerify`, `AuthPasskeysConsoleAuthenticateChallenge`, `AuthPasskeysConsoleAuthenticateVerify`, `AuthPasskeysConsolePrefix`, `AuthSessionsMe`, `AuthEnrollmentTokenGenerate`, `AuthEnrollmentTokenValidate`
- Approval: `Approvals`, `ApprovalsByID`, `ApprovalsPrefix`, `ApprovePage`, `ApprovePagePrefix`, `ApprovalsCLIStatus`, `ApprovalsCLIList`
- Admin: `AdminAppPoliciesBySigner`, `AdminAppsRevoke`, `AdminAppPoliciesPrefix`, `AdminConsensus`, `AdminConsensusByID`, `AdminConsensusPrefix`
- Consensus: `ConsensusDeliberate` (`/consensus/v1/deliberate`)
- Well-known: `WellKnownPKICABundle`, `WellKnownPKIFingerprint`, `WellKnownBinPrefix`, `WellKnownPKIPrefix`
- Deploy scripts: `DeployScriptLinux` (`/g8e-deploy.sh`), `DeployScriptWindows` (`/g8e-deploy.ps1`)
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
- Wire protocol events: `PubSubEventMessage`, `PubSubEventPMessage`, `PubSubEventSubscribed`, `PubSubEventUnsubscribed`, `PubSubEventError`
- SSE event types: `SSEEventTypeApprovalCompleted` (`approval.completed`)

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
- `ApprovalType`: `ApprovalTypeAgentContinue` (`agent.continue`), `ApprovalTypeCommand` (`command`), `ApprovalTypeFileEdit` (`file.edit`), `ApprovalTypeIntent` (`intent`), `ApprovalTypeStream` (`stream`). Values use dot-separated format consistent with `EventType` naming conventions.
- `SessionType`: `SessionTypeCLI`, `SessionTypeOperator`, `SessionTypeWeb`, `SessionTypeApp` (`app`). `SessionTypeApp` is an auto-created stub session row in the audit store for FK satisfaction when an event arrives with an unknown session ID; distinct from operator/cli/web which are first-class authenticated sessions.
- `StreamStatus`: `StreamStatusCancelled`, `StreamStatusCompleted`, `StreamStatusExited`, `StreamStatusFailed`, `StreamStatusSummary`
- `CommandExitStatus`: `CommandExitStatusError`, `CommandExitStatusFailure`, `CommandExitStatusInterrupted`, `CommandExitStatusInvalidExit`, `CommandExitStatusKilled`, `CommandExitStatusMisuse`, `CommandExitStatusNotExecutable`, `CommandExitStatusNotFound`, `CommandExitStatusSuccess`, `CommandExitStatusTerminated`, `CommandExitStatusSignal1` (SIGHUP), `CommandExitStatusSignal2` (SIGINT), `CommandExitStatusSignal3` (SIGQUIT), `CommandExitStatusSignal6` (SIGABRT), `CommandExitStatusSignal9` (SIGKILL), `CommandExitStatusSignal11` (SIGSEGV), `CommandExitStatusSignal13` (SIGPIPE), `CommandExitStatusSignal15` (SIGTERM)
- `VaultMode`: `VaultModeScrubbed`, `VaultModeRaw`
- `ToolScope`: `ToolScopeOperatorGated`, `ToolScopeUniversal`
- `Platform`: `PlatformDarwin`, `PlatformLinux`, `PlatformWindows`
- `ComponentName`: `ComponentNameClient`, `ComponentNameG8EO`, `ComponentNameG8EOGateway`
- `SystemHealth`: `SystemHealthDegraded`, `SystemHealthHealthy`, `SystemHealthUnhealthy`, `SystemHealthUnknown`
- `NetworkProtocol`: `NetworkProtocolTCP`, `NetworkProtocolUDP`
- `Environment`: `EnvironmentDev`, `EnvironmentProduction`, `EnvironmentTest`
- `VersionStability`: `VersionStabilityBeta`, `VersionStabilityDev`, `VersionStabilityStable`, `VersionStabilityUnstable`, `VersionStabilityDeprecated`
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
- `SuspendedTxStatus`: `SuspendedTxStatusPending`, `SuspendedTxStatusApproved`, `SuspendedTxStatusExpiredOrNotFound`
- `HistoryActor`: `HistoryActorNone`, `HistoryActorG8EO`, `HistoryActorSystem`, `HistoryActorUser`
- `AISource`: `AISourceTerminalAnchored`, `AISourceTerminalDirect`, `AISourceToolCall`
- `ComponentStatus`: `ComponentStatusActive`, `ComponentStatusError`, `ComponentStatusInactive`, `ComponentStatusMaintenance`, `ComponentStatusDegraded`
- `InfrastructureStatus`: `InfrastructureStatusCritical`, `InfrastructureStatusDegraded`, `InfrastructureStatusHealthy`, `InfrastructureStatusStable`, `InfrastructureStatusUnknown`, `InfrastructureStatusOperational`, `InfrastructureStatusDown`
- `AuthMethod`: `AuthMethodKvPubSub`, `AuthMethodSession`, `AuthMethodProxy`, `AuthMethodOperatorSession`, `AuthMethodTest`
- `WorkflowType`: `WorkflowTypeG8eBound`, `WorkflowTypeG8eCloudBound`, `WorkflowTypeG8eNotBound`, `WorkflowTypeTriage`, `WorkflowTypeInvestigation`
- `AITaskId`: `AITaskIDAgentContinue`, `AITaskIDChat`, `AITaskIDCommand`, `AITaskIDDirectCommand`, `AITaskIDFetchFileDiff`, `AITaskIDFetchFileHistory`, `AITaskIDFetchHistory`, `AITaskIDFetchLogs`, `AITaskIDFileEdit`, `AITaskIDFsList`, `AITaskIDFsRead`, `AITaskIDIntentGrant`, `AITaskIDIntentRevoke`, `AITaskIDPortCheck`, `AITaskIDRecursiveGrep`, `AITaskIDRestoreFile`, plus additional task ID aliases (`AITaskId*` casing) for chat, case, memory, command, command execution, direct command, intent grant, intent revoke, file edit, file operation, fs list, recursive grep, port check, agent continue, and investigation query
- `ConsensusMember`: `ConsensusMemberAxiom`, `ConsensusMemberConcord`, `ConsensusMemberVariance`, `ConsensusMemberPragma`, `ConsensusMemberNemesis`
- `ConsensusAuditMode`: `ConsensusAuditModeUnanimous`, `ConsensusAuditModeMajority`, `ConsensusAuditModeTied`
- `ConsensusAuditStatus`: `ConsensusAuditStatusOk`, `ConsensusAuditStatusRevised`, `ConsensusAuditStatusSwap`
- `AuditorReason`: `AuditorReasonOk`, `AuditorReasonRevised`, `AuditorReasonRevisedFromDissent`, `AuditorReasonSwappedToDissenter`, `AuditorReasonWhitelistViolation`, `AuditorReasonNoValidRevision`, `AuditorReasonAuditorError`, `AuditorReasonEmptyResponse`
- `TieBreakReason`: `TieBreakReasonShortest`, `TieBreakReasonExcludedNemesis`
- `ReasoningAgent`: `ReasoningAgentSage`, `ReasoningAgentDash`
- `ErrorCode`: typed string constants for g8e error codes (`G8E-1000` through `G8E-1900`), covering generic errors, config errors, auth errors, database errors, pubsub errors, storage errors, API errors, validation errors, business logic errors, and service unavailable errors
- `ErrorCategory`: `ErrorCategoryValidation`, `ErrorCategoryBusinessLogic`, `ErrorCategoryConfiguration`, `ErrorCategoryAuth`, `ErrorCategoryPermission`, `ErrorCategoryResourceNotFound`, `ErrorCategoryConflict`, `ErrorCategoryRateLimit`, `ErrorCategoryServiceUnavailable`, `ErrorCategoryExternalService`, `ErrorCategoryTimeout`, `ErrorCategoryDatabase`, `ErrorCategoryNetwork`, `ErrorCategoryPubSub`, `ErrorCategoryStorage`, `ErrorCategoryInternal`, `ErrorCategoryDependency`
- `ErrorSeverity`: `ErrorSeverityLow`, `ErrorSeverityMedium`, `ErrorSeverityHigh`, `ErrorSeverityCritical`, `ErrorSeverityInfo`

### Headers and Auth Constants (`auth.go`)

HTTP header names and authentication-related constants:

- Identity: `HeaderOperatorID`, `HeaderOperatorSessionID`, `HeaderWebSessionID`, `HeaderCLISessionID`, `HeaderUserID`, `HeaderOrganizationID`, `HeaderBoundOperators`
- Context: `HeaderCaseID`, `HeaderExecutionID`, `HeaderInvestigationID`, `HeaderTaskID`
- System: `HeaderRequestID`, `HeaderSourceComponent`, `HeaderSystemFingerprint`, `HeaderXAccelBuffering`
- Proxy: `HeaderXForwardedFor`, `HeaderXForwardedHost`, `HeaderXForwardedProto`, `HeaderXProxyOrganizationID`, `HeaderXProxyUserID`, `HeaderXRequestTimestamp`
- Security: `HeaderXContentTypeOptions`, `HeaderXFrameOptions`, `HeaderContentSecurityPolicy`
- Standard: `HeaderAuthorization`, `HeaderContentType`, `HeaderAccept`, `HeaderAcceptLanguage`, `HeaderCacheControl`, `HeaderCookie`, `HeaderUserAgent`, `HeaderPragma`, `HeaderSetCookie`, `HeaderConnection`, `HeaderVary`
- Content: `HeaderContentDisposition`, `HeaderContentLanguage`, `HeaderContentLength`
- SSE: `HeaderLastEventID`
- AJAX: `HeaderRequestedWith`
- CORS: `HeaderAccessControlAllowCredentials`, `HeaderAccessControlAllowOrigin`, `HeaderAccessControlAllowHeaders`, `HeaderAccessControlAllowMethods`, `HeaderAccessControlMaxAge`, `HeaderAccessControlRequestHeaders`, `HeaderAccessControlRequestMethod`

Additional constants in `auth.go`:

- Passkey purposes: `PasskeyPurposeRegister`, `PasskeyPurposeAuth`
- WebAuthn algorithms: `WebAuthnAlgES256` (-7), `WebAuthnAlgRS256` (-257)
- WebAuthn types: `WebAuthnTypePublicKey`, `WebAuthnAttestationNone`, `WebAuthnResidentKeyRequired`, `WebAuthnUserVerificationRequired`
- PKI leaf types: `LeafTypeOperator`, `LeafTypeApp`, `LeafTypeHub`, `LeafTypeCLI`
- JSON-RPC 2.0: `JSONRPCVersion`, `JSONRPCFieldVersion`, `JSONRPCFieldMethod`, `JSONRPCFieldParams`, `JSONRPCFieldID`, `JSONRPCFieldResult`, `JSONRPCFieldError`, `JSONRPCFieldCode`, `JSONRPCFieldMessage`, `JSONRPCFieldData`, `JSONRPCErrorCodeInternal`
- Header values: `HeaderValueNoSniff`, `HeaderValueDeny`, `HeaderValueCSPNone`, `HeaderValueKeepAlive`, `HeaderValueNoCache`, `HeaderValueTextEvent`, `HeaderValueApplicationJSON`, `HeaderValueXHTML`, `HeaderValueXML`, `HeaderValueOctetStream`, `HeaderValuePEM`, `HeaderValueCRL`, `HeaderValueShell`, `HeaderValuePowerShell`, `HeaderValueCORSPreflightMaxAge` (`3600`)
- Context keys (typed `ContextKey`): `ContextKeyUserID`, `ContextKeyAppID`, `ContextKeyTenantID`, `ContextKeyBindingPersona`, `ContextKeyOperatorID`, `ContextKeyOperatorSessionID`, `ContextKeyCapability`, `ContextKeyWebSessionID`, `ContextKeyCLISessionID`
- Auth error reasons (typed `AuthErrorReason`): `AuthErrorReasonTTLExceeded`, `AuthErrorReasonRetiredByRealLogin`, `AuthErrorReasonIdentityDisabled`, `AuthErrorReasonInvalidSession`, `AuthErrorReasonSessionExpired`, `AuthErrorReasonCertificateRevoked`, `AuthErrorReasonIdentityMismatch`, `AuthErrorReasonAppPolicyNotFound`, `AuthErrorReasonRateLimitExceeded`, `AuthErrorReasonPayloadTooLarge`, `AuthErrorReasonCollectionNotAllowed`, `AuthErrorReasonJWTInvalid`, `AuthErrorReasonJWTMissingSubject`
- Session TTL: `WebSessionTTL` (24 hours), `WebSessionCookieName` (`g8e_web_session_cookie`)
- App enrollment types: `AppTypeMCPClient`, `AppTypeA2AGateway`, `AppTypeCustom`, `AppTypeConsensusMember`
- Certificate renewal: `AppCertMinValidity` (7 days)

### Action Types (`action_types.go`)

GovernanceEnvelope action types, typed as `ActionType`. The file also defines `AllActionTypes` (a canonical slice of all valid action types) and an `IsMutation()` method that returns true for action types that modify system state:

- `ActionTypeA2aCall`, `ActionTypeCancel`, `ActionTypeEvalAnswer`, `ActionTypeExecuteBash`, `ActionTypeFetchFileDiff`, `ActionTypeFetchFileHistory`, `ActionTypeFetchHistory`, `ActionTypeFetchLogs`, `ActionTypeFileEdit`, `ActionTypeFsGrep`, `ActionTypeFsList`, `ActionTypeFsRead`, `ActionTypeHeartbeat`, `ActionTypeInvestigationCreate`, `ActionTypeMcpCall`, `ActionTypeMcpPromptGet`, `ActionTypeMcpPromptList`, `ActionTypeMcpResourceList`, `ActionTypeMcpResourceRead`, `ActionTypePortCheck`, `ActionTypeRestoreFile`, `ActionTypeShutdown`

Mutation action types: `ActionTypeA2aCall`, `ActionTypeCancel`, `ActionTypeExecuteBash`, `ActionTypeFileEdit`, `ActionTypeMcpCall`, `ActionTypeRestoreFile`, `ActionTypeShutdown`.

### Paths (`paths.go`)

Filesystem paths for Operator data, certificates, ledger, system paths, and configuration. This is the largest constant file in the package, containing:

- System paths: Linux filesystem paths (`/etc`, `/proc`, `/var`, `/dev`, `/boot`, `/tmp`, `/opt`, `/home`, `/usr`, `/bin`, `/sbin`, `/lib`, `/sys`, `/`), including specific files (`/etc/passwd`, `/etc/shadow`, `/etc/group`, `/etc/gshadow`, `/etc/sudoers`, `/etc/ssh/sshd_config`, `/etc/hosts`, `/etc/resolv.conf`, `/etc/fstab`, `/etc/hostname`, `/etc/machine-id`, `/proc/meminfo`, `/proc/loadavg`, `/proc/mounts`, `/proc/net/tcp`, `/proc/net/udp`, `/proc/uptime`, `/proc/stat`, `/proc/version`, `/proc/1/cmdline`, `/etc/os-release`, etc.), root shell paths (`/root/.ssh/`, `/root/.bashrc`, `/root/.bash_profile`, `/root/.profile`), and macOS paths (`/Library/Preferences/SystemConfiguration/preferences.plist`)
- SSH paths: `PathEtcSshKnownHosts`, `PathEtcSshSshKnownHosts`, `PathHomeSshKnownHosts`, `PathWindowsSshKnownHosts`, `PathWindowsProgramDataSsh`
- Windows paths: `PathWindowsSystemRoot`, `PathWindowsHostsFile`, `PathWindowsRegistryCryptography`, `PathWindowsRegistryMachineGuid`, Git Bash paths (`PathWindowsGitBinBash`, `PathWindowsGitUsrBinBash`, `PathWindowsGitBinSh`, `PathWindowsMsys64Bash`, `PathWindowsCygwin64Bash`), and Windows temp cert constants (`WindowsTempCertImportPrefix`, `WindowsTempCATrustPrefix`, `WindowsTempCertFilename`)
- PKI filesystem constants: directory names (`PkiDirname`, `PkiSubdirRoot`, `PkiSubdirAuthorities`, `PkiSubdirIssued`, `PkiSubdirTrust`, `PkiSubdirRevocation`, `PkiSubdirBinaries`, `PkiSubdirClient`, `PkiSubdirHub`, `PkiSubdirGatewayPeer`, `PkiSubdirApps`, `PkiSubdirTrustedSigners`), file extensions (`FileExtCert`, `FileExtKey`, `FileExtPEM`, `FileExtJSON`), CA and bundle filenames (`PkiFileRootCA`, `PkiFileRootCAKey`, `PkiFileHubCA`, `PkiFileOperatorCA`, `PkiFileGatewayPeerCA`, `PkiFileGatewayBundle`, `PkiFileRootBundle`, `PkiFileOperatorBundle`, `PkiFileTrustDomainJSON`, `PkiFileWardenPub`, `PkiFileBootstrapCA`, `PkiFileBootstrapBundle`), operator credentials (`PkiFileOperatorCert`, `PkiFileOperatorKey`, `PkiFileOperatorChain`), gateway credentials (`PkiFileGatewayCert`, `PkiFileGatewayKey`, `PkiFileGatewayChain`), peer certificates (`PeerCertFilename`, `PeerKeyFilename`, `PeerChainFilename`, `PeerSubdir`), and CLI credentials (`CliCertFilename`, `CliKeyFilename`, `CredentialsFilename`)
- Database filenames: `DbFilename` (`g8e.db`), `VaultKeyFilename`, `VaultNewKeyFilename`, `VaultHeaderFilename`, `SuspendedTxFilename`, `ReceiptsFilename`, `ReceiptsExportFilename`, `ReplayStoreDBFilename`, `ExecutionVaultDBFilename`, `LocalStateDBFilename`, `AuditVaultDBFilename`, `MasterKeyFilename`, `PublicKeySuffix`
- Secrets filenames: `SecretsFileSessionEncryptionKey`, `SecretsFileBootstrapDigest`, `SecretsFileActuatorSigningKey`, `SecretsFileActuatorKeyID`, `SecretsFileAuditorHMACKey`, `SecretsFileNotarySigningKey`, `SecretsFileOperatorPrivateKey`, `SecretsFileCLIPrivateKey`, `SecretsFileSessionToken`, `SecretsFileConsensusMemberKeyPrefix`
- Demos: `DemosDirname`, `DemosComposeFile`, `DemosBinDirname`, `DemosBinaryName`, `DemosTargetDataDir`, `DemosDoctrineDir`, `DemosPARequestsFile`, `DemosHIPAADoctrineFile`, `DemosDHSDoctrineFile`, `DemosFedRAMPDoctrineFile`, `DemosImagesManifestFile`, `DemosOrgHealthcare`, `DemosOrgFinance`, `DemosOrgDHS`, `DemosOrgFedRAMP`, `DemosOrgFrontend`
- Container paths: Docker exec paths for demo environments (`ContainerRootG8E`, `ContainerPKIDir`, `ContainerOperatorCert`, `ContainerOperatorKey`, `ContainerCABundle`, `ContainerDataDir`, `ContainerAuditVaultDB`, `ContainerExecutionVaultDB`, `ContainerLedgerFilesDir`, `ContainerDoctrineDir`, `ContainerEnsembleSeed`, and verification script paths)
- Local binary names: `LocalBinaryName` (`./g8e`), `LocalBinaryNameWindows` (`./g8e.exe`), `BinaryImageName`, `BinaryImageNameWindows`
- Deploy script filenames: `DeployScriptFilenameLinux` (`g8e-deploy.sh`), `DeployScriptFilenameWindows` (`g8e-deploy.ps1`)
- Component filenames: `SwaggerFilename`, `ComplianceReportFilename`, `GatewayIDFilename`, `ActuatorPubJSONFilename`, `ActuatorPubPEMFilename`, `NetworkIdentityFilename`, `OperatorPIDFilename`, `OperatorPostureFilename`, `OperatorBinaryFilename`, `OperatorLogFilename`
- Runtime directories: `RuntimeDirname` (`.g8e`), `DataDirname`, `VaultDirname`, `SecretsDirname`, `LedgerDirname`, `PidDirname`, `DocsDirname`, `ProtocolDirname`, `ProtocolConstantsDirname`, `ProtocolModelsDirname`, `BinDirname`, `LogDirname`
- Ledger directories: `FilesDirname`, `SessionsDirname`, `GitDirname`, `GitignoreFilename`, `GoModFilename`
- SSH config: `SshConfigFilename`, `SshDirname`, `SshConfigBasename`, `SshKnownHostsBasename`, `SshKeyEd25519`, `SshKeyECDSA`, `SshKeyRSA`
- Agent config directories: `AgentConfigDirGemini`, `AgentConfigDirGoose`
- Agent config files: `AgentConfigFileMCP`, `AgentConfigFileSettings`, `AgentConfigFileGooseYAML`
- File permission modes: `PermDirPrivate` (0700), `PermDirStandard` (0755), `PermFilePrivate` (0600), `PermFilePublic` (0644), `PermFileReadOnly` (0400)
- Filesystem listing limits: `FsListMaxDepth` (3), `FsListDefaultDepth` (0), `FsListMaxEntries` (500), `FsListDefaultEntries` (100), `FsListBatchSize` (100)
- Grep limits: `FsGrepDefaultMaxMatches` (100), `FsGrepMaxMatches` (500), `FsGrepScannerInitialBufSize` (64 KiB), `FsGrepScannerMaxBufSize` (1 MiB)
- Execution limits: `ExecutionMaxStreamSize` (10 MB), `ExecutionMaxLines` (50), `ExecutionPreviewLength` (300), `FileEditMaxSize` (50 MB)
- Reporting: `ReportsDirname` and CSV output filenames for receipts, sessions, events, file mutations, executions, file diffs, commitments, ledger commits, ledger merkle root, replay nonces, suspended transactions, verification summary, and manifest
- CLI default paths: `DefaultVaultDirDesc`, `DefaultVaultKeyDesc`, `DefaultOperatorKeyDesc`, `DefaultClientKeyDesc`, `DefaultOperatorCertDesc`, `DefaultClientCertDesc`, `DefaultDataDir`, `DefaultPKIDir`, `DefaultSecretsDir`
- Consensus bootstrap: `ConsensusBootstrapConfigFilename` (`consensus-bootstrap.json`)
- API path constants: `APIPathAuthDeviceEnroll`, `APIPathPKIDevicesEnroll`, `WellKnownPKICABundle`
- File suffixes: `TmpFileSuffix`, `BackupFileSuffixPattern`, `SQLiteWALSuffix`, `SQLiteSHMSuffix`
- Environment path: `EnvPathDefault`, `PathParentDir`
- Test-specific constants: isolated test environment filenames and paths for certs, keys, databases, and gateway config

### Ports (`ports.go`)

Network ports defined as a struct `Ports` with JSON tags:

- `OperatorHttp` (8080)
- `OperatorHttps` (8443)

### Exit Codes (`exit_codes.go`)

Operator process exit codes:

- `ExitSuccess` (0), `ExitGeneralError` (1), `ExitAuthFailure` (2), `ExitPermissionDenied` (3), `ExitNetworkError` (4), `ExitConfigError` (5), `ExitStorageError` (6), `ExitCertTrustFailure` (7)

Unix shell exit codes for command execution:

- `ExitCodeSuccess` (0), `ExitCodeGeneral` (1), `ExitCodeUsage` (2), `ExitCodeTimeout` (124), `ExitCodeCannotExecute` (126), `ExitCodeCommandNotFound` (127), `ExitCodeKilled` (137), `ExitCodeNone` (-1)

Windows process exit codes:

- `StillActiveExitCode` (259): indicates a Windows process is still running, equivalent to the Windows STILL_ACTIVE macro

### Errors (`errors.go`)

Platform error variables defined as `error` values using `errors.New()`. The file contains over 200 error variables covering standard platform errors, keystore errors, ledger errors, CLI approval errors, notary errors, CLI authentication errors, HTTP client errors, process manager errors, filesystem errors, FsGrep errors, pubsub client errors, execution service errors, MCP service errors, MCP registry errors, MCP validation errors, MCP OOM detection errors, MCP SSH known hosts errors, MCP git ops errors, MCP TLS cert inspect errors, run shell command errors, network identity detection errors, system utils errors, audit service errors, audit store errors, execution vault errors, pubsub service errors, scrubbing service errors, gateway service errors, gateway approval errors, MCP native handler errors, SQLite validation errors, SQLite utility errors, SQLite compression errors, passkey bootstrap errors, enrollment token errors, passkey credential validation errors, Windows-specific errors, data command errors, test command errors, and vault command/crypto errors.

### Environment Variables (`env_vars.go`)

Typed environment variable names, typed as `EnvVarKey` and grouped in a struct `EnvVar`:

- `ConsensusID` (`G8E_CONSENSUS_ID`), `ConsensusURL` (`G8E_CONSENSUS_URL`), `ConsensusBootstrap` (`G8E_CONSENSUS_BOOTSTRAP`), `DoctrineDir` (`G8E_DOCTRINE_DIR`)
- `VaultDir` (`G8E_VAULT_DIR`), `VaultKey` (`G8E_VAULT_KEY`)
- `OperatorSessionID` (`G8E_OPERATOR_SESSION_ID`)
- `PasskeyRpID` (`G8E_PASSKEY_RP_ID`), `PasskeyRpName` (`G8E_PASSKEY_RP_NAME`), `PasskeyRpOrigins` (`G8E_PASSKEY_RP_ORIGINS`)
- `PublicBaseURL` (`G8E_PUBLIC_BASE_URL`), `AllowedOrigins` (`G8E_ALLOWED_ORIGINS`)
- Lattice: `LatticeEndpoint` (`LATTICE_ENDPOINT`), `LatticeClientID` (`LATTICE_CLIENT_ID`), `LatticeClientSecret` (`LATTICE_CLIENT_SECRET`), `LatticeSandboxesToken` (`SANDBOXES_TOKEN`), `LatticeEntityName` (`LATTICE_ENTITY_NAME`), `LatticePostureFloor` (`LATTICE_POSTURE_FLOOR`)
- `Shell` (`SHELL`), `Lang` (`LANG`), `Term` (`TERM`), `TZ` (`TZ`)

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
- `AgentBinary` (typed): `AgentBinaryClaude`, `AgentBinaryCodex`, `AgentBinaryGemini`, `AgentBinaryGoose`

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

- Sentinel prefix: `SentinelKeyPrefix` (`g8e:sentinel:`)
- Scrubbing token prefix: `ScrubbingTokenKeyPrefix` (`uei_token_`)
- Cache prefix: `KVCachePrefix` (`g8e`)
- Cache keys: `KVKeyCacheDoc`, `KVKeyCacheQuery`
- Session keys: `KVKeySessionWeb`, `KVKeySessionOperator`, `KVKeySessionOperatorBind`, `KVKeySessionWebBind`
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
- `SSHKeepaliveRequestType` (`keepalive@g8e`), `SSHKeepaliveInterval` (15 seconds), `SSHKeepaliveMaxMissed` (3)
- `SSHMaxRetries` (3), `SSHCaptureMaxBytes` (64 KiB), `SSHPreflightVerifyCommand` (`true`), `SSHProxyAddrLabel` (`proxy`)

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
- Binary names: `BinaryNameWindows` (`g8e-windows-amd64.exe`), `BinaryNameLinux` (`g8e-linux-amd64`), `BinaryNameDarwin` (`g8e-darwin-amd64`)
- Architectures: `ArchAMD64`, `ArchARM64`, `Arch386`
- Operating systems: `OSLinux`, `OSDarwin`, `OSWindows`
- Governance posture names: `PostureDoctrine` (`doctrine`), `PostureConsensus` (`consensus`), `PostureNotary` (`notary`)
- Log level names: `LogLevelInfo`, `LogLevelError`, `LogLevelDebug`, `LogLevelDefault` (alias for `LogLevelInfo`)

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

- Terminal control characters: `CtrlC` (3), `Backspace` (8), `Delete` (127), `PrintableASCIIStart` (32), `PrintableASCIIEnd` (126)
- `DefaultShellCommandTimeout` (30 seconds), `MaxShellCommandTimeout` (300 seconds), `ShutdownTimeout` (15 seconds)
- `LocalhostHostname` (`localhost`), `LocalhostIP` (`127.0.0.1`)
- `RemoteEphemeralScriptTemplate`: bash script template for remote Operator deployment with graceful cleanup
- `RemoteInjectedBinaryMessage`, `RemoteInjectedScriptMinimal`: constants for binary injection without execution
- `DangerousCommands`: list of commands blocked by safety policy (rm, dd, mkfs, fdisk, format, del, erase, shred, wipe, killall, pkill, reboot, shutdown, halt, poweroff, init, systemctl, service, iptables, ip6tables, nft, ufw, firewall-cmd, route, ifconfig, ip, brctl, tc, modprobe, insmod, rmmod, depmod, mount, umount, swapon, swapoff, mkswap, lvcreate, lvremove, lvchange, vgcreate, vgremove, pvcreate, pvremove, cryptsetup, passwd, chpasswd, usermod, userdel, groupmod, crontab, at, batch, sudo, su, doas, runuser, curl, wget)
- `DangerousPatterns`: list of command patterns blocked by safety policy (rm -rf /, rm -rf /*, fork bomb, dd if=/dev/zero, mkfs, > /dev/sda, > /dev/vda, chmod 777 /, chown -R, nc -l, ncat -l, ssh, scp, rsync)
- `ShellInjectionPatterns`: `$(`, backtick, `|`
- `ShellMetacharacters`: `$`, backtick, `\`, `;`, `&`, `|`, `>`, `<`, newline, carriage return

### Timestamp (`timestamp.go`)

Timestamp format constants:

- `FormatRFC3339`: canonical RFC3339 format string with timezone offset (re-exported from `internal/timesvc`)
- `TimestampFormat`: RFC3339 with fixed microsecond precision (`2006-01-02T15:04:05.000000Z07:00`) for lexicographic ordering (re-exported from `internal/timesvc` as `timesvc.Format`)

### Mappings (`mappings.go`)

Functions mapping between event types and action types:

- `MapActionTypeToEventType`: Maps `ActionType` back to `EventType` using a reverse map derived from `eventToAction` at init time. Unmapped action types pass through as-is.
- `MapEventTypeToResultActionType`: Maps completion/failure events to result action types (e.g., `EXECUTE_BASH_RESULT`, `EXECUTE_BASH_CANCELLED`) via `eventToResultAction`.
- The `eventToAction` map is the single source of truth for the `EventType` to `ActionType` relationship, covering operator command, file edit, filesystem, fetch, MCP, A2A, port check, heartbeat, shutdown, eval, and investigation events.

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
