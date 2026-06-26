# g8e Codemap - Service Dependency Tree

## Top-Level Service Roots

```text
G8eoService (Outbound/Operator Mode) [MODE-SPECIFIC]
├── auth.BootstrapService
│   └── *external HTTP auth endpoint*
├── gateway.SecretManager
│   └── gateway.CanonicalDBService (for keystore DB access) [SHARED]
├── execution.ExecutionService
├── execution.FileEditService
├── pubsub.OperatorPubSubService
│   ├── pubsub.HeartbeatService
│   ├── pubsub.CommandService
│   │   └── execution.ExecutionService
│   ├── pubsub.FileOpsService
│   │   └── execution.FileEditService
│   ├── pubsub.PortService
│   ├── pubsub.AuditService
│   ├── pubsub.HistoryService
│   ├── governance.L4Warden
│   │   ├── governance.ReplayStore (storage.SQLReplayStore)
│   │   ├── governance.StateRootProvider (gateway.StateRootService via CanonicalDBService) [SHARED]
│   │   ├── governance.SignerStore (governance.FilesystemSignerStore)
│   │   ├── governance.AppPolicyStore (gateway.AppPolicyStoreService via CanonicalDBService) [SHARED]
│   │   ├── governance.TribunalStore (nil in outbound mode, gateway.TribunalStoreService in gateway mode)
│   │   ├── governance.L1Doctrine (created internally by L4Warden when doctrine param is nil)
│   │   └── governance.L3Notary (governance.outboundL3Notary implementation)
│   │       └── storage.SuspendedTransactionService
│   ├── governance.L5Actuator
│   │   ├── execution.ExecutionService
│   │   ├── storage.SQLAuditStore
│   │   ├── governance.TransactionAuditStore (auditStoreTransactionStore wrapper)
│   │   ├── governance.L3Notary
│   │   ├── scrubbing.ScrubbingService
│   │   ├── governance.StateRootProvider
│   │   └── governance.SignerStore
│   └── mcp.GatewayService [SHARED] (declared in CommandServiceConfig but not wired in g8eo.go; subtree is potential wiring only)
│       ├── response.Writer
│       └── storage.SuspendedTransactionService (as storage.SuspendedTransactionStore) [SHARED]
├── pubsub.PubSubResultsService
├── storage.ExecutionVaultService
│   ├── sqliteutil.DB
│   └── vault.Vault (shared with CanonicalDBService)
├── gateway.EncryptedKVAdapter (implements storage.TokenStore)
│   ├── gateway.KVStoreService (from CanonicalDBService) [SHARED]
│   └── vault.Vault (shared with CanonicalDBService)
├── storage.SuspendedTransactionService
│   └── sqliteutil.DB
├── storage.SQLAuditStore
│   ├── sqliteutil.DB
│   └── vault.Vault (shared with CanonicalDBService)
├── storage.GitLedgerService
│   └── vault.Vault (shared with CanonicalDBService)
├── storage.HistoryHandler
│   ├── storage.SQLAuditStore
│   └── storage.GitLedgerService
├── governance.ReplayStore (storage.SQLReplayStore)
│   └── sqliteutil.DB
├── scrubbing.ScrubbingService
│   └── storage.TokenStore (gateway.EncryptedKVAdapter)
└── gateway.CanonicalDBService [SHARED]
    ├── sqliteutil.DB
    ├── storage.SQLAuditStore
    ├── vault.Vault
    ├── gateway.DocumentStoreService
    ├── gateway.AppPolicyStoreService
    ├── gateway.SignerStoreService
    ├── gateway.TribunalStoreService
    ├── gateway.StateRootService
    ├── gateway.ReplayStoreService
    ├── gateway.KVStoreService
    ├── gateway.SSEEventService
    └── gateway.BlobStoreService

GatewayModeService (Gateway/Platform Mode) [MODE-SPECIFIC]
├── gateway.CanonicalDBService [SHARED] (lifecycle only: Open, Close, Wait, GetDB, GetVault, schema/migrations, maintenance loop)
│   ├── sqliteutil.DB
│   ├── storage.SQLAuditStore
│   ├── vault.Vault
│   ├── gateway.DocumentStoreService (extracted field)
│   ├── gateway.AppPolicyStoreService (extracted field)
│   ├── gateway.SignerStoreService (extracted field)
│   ├── gateway.TribunalStoreService (extracted field)
│   ├── gateway.StateRootService (extracted field)
│   ├── gateway.ReplayStoreService (extracted field)
│   ├── gateway.KVStoreService (extracted field)
│   ├── gateway.SSEEventService (extracted field)
│   └── gateway.BlobStoreService (extracted field)
├── storage.SuspendedTransactionService (for L3 approval workflow)
│   └── sqliteutil.DB
├── gateway.GatewayWebSocketHandler
├── gateway.AuthService
│   ├── gateway.CanonicalDBService [SHARED]
│   ├── gateway.PKIAuthority
│   ├── gateway.UserService
│   ├── gateway.PersonaService
│   ├── gateway.JWKSProvider (optional, for external IdP JWT auth)
│   └── response.Writer
├── gateway.PKIAuthority
│   ├── gateway.CanonicalDBService [SHARED]
│   └── gateway.SecretManager (local variable in NewGatewayModeService, not a retained field)
│       ├── sqliteutil.DB
│       └── keystore.Keystore (via gateway.CanonicalDBService)
├── gateway.RegistrationService
│   ├── gateway.CanonicalDBService [SHARED]
│   ├── gateway.PKIAuthority
│   ├── gateway.UserService
│   ├── gateway.CLISessionService
│   └── gateway.OperatorSessionService
├── gateway.PasskeyService
│   ├── gateway.CanonicalDBService [SHARED]
│   ├── gateway.UserService (for first-credential checks)
│   ├── gateway.WebSessionService (for session creation on browser flows)
│   └── response.Writer (for HTTP responses in passkey_service_http.go)
├── gateway.UserService
│   └── gateway.CanonicalDBService [SHARED]
├── gateway.CLISessionService
│   └── gateway.CanonicalDBService [SHARED]
├── gateway.OperatorSessionService
│   └── gateway.CanonicalDBService [SHARED]
├── gateway.WebSessionService
│   └── gateway.CanonicalDBService [SHARED]
├── gateway.HTTPHandler
│   ├── gateway.CanonicalDBService [SHARED]
│   ├── gateway.GatewayWebSocketHandler
│   ├── gateway.AuthService
│   ├── gateway.PKIAuthority
│   ├── gateway.CLISessionService
│   ├── gateway.OperatorSessionService
│   ├── gateway.WebSessionService
│   ├── gateway.RegistrationService
│   ├── gateway.PasskeyService
│   ├── gateway.UserService
│   ├── gateway.AppEnrollmentService
│   │   ├── gateway.CanonicalDBService [SHARED]
│   │   └── gateway.PKIAuthority
│   ├── gateway/console (Console SPA embed filesystem)
│   ├── mcp.GatewayService [SHARED]
│   ├── tribunal.TribunalService [SHARED]
│   ├── storage.SuspendedTransactionService (as storage.SuspendedTransactionStore) [SHARED]
│   ├── governance.EnvelopeProcessor (set post-construction by boot sequence)
│   └── response.Writer
├── governance.outboundL3Notary (gateway variant via governance.NewGatewayL3Notary, implements governance.L3Notary)
│   ├── storage.SuspendedTransactionService (as storage.SuspendedTransactionStore)
│   ├── gateway.cliSessionVerifier (via NewCLISessionVerifier, implements governance.CLISessionVerifier)
│   │   ├── gateway.CanonicalDBService [SHARED]
│   │   ├── gateway.PKIAuthority
│   │   ├── gateway.UserService
│   │   └── gateway.CLISessionService
│   └── gateway.PasskeyService (as governance.L3Notary for WebAuthn proofs)
│       └── gateway.CanonicalDBService [SHARED]
├── tribunal.TribunalService
│   ├── governance.L1Doctrine
│   ├── tribunal.TribunalMember (one or more enrolled members with Ed25519 keys)
│   ├── response.Writer
│   └── tribunal.LocalDeliberator (in-process deliberation)
├── mcp.GatewayService [SHARED]
│   ├── response.Writer
│   ├── storage.SuspendedTransactionService (as storage.SuspendedTransactionStore) [SHARED]
│   ├── scrubbing.ScrubbingService
│   ├── mcp.FieldPathRegistry
│   ├── mcp.NativeToolHandler
│   ├── mcp.FieldReader (gateway.DocumentStoreService) [SHARED]
│   ├── mcp.SessionValidator (set by in-process OperatorPubSubService in both modes)
│   ├── mcp.AuditLogger (pubsubAuditLogger, set by in-process OperatorPubSubService in both modes)
│   └── tribunal.TribunalDeliberator (tribunal.LocalDeliberator in gateway mode, nil in outbound)
└── response.Writer
```

## Structural Observations

### Mode Bifurcation
- **Mode-specific services**: `G8eoService` (outbound mode only), `GatewayModeService` (gateway mode only)
- **Shared services**: `mcp.GatewayService` (used in both modes for MCP/A2A protocol handling; note: in outbound mode, `MCPGateway` is declared in `CommandServiceConfig` but not wired in `g8eo.go` Start), `CanonicalDBService` (used in both modes for state root calculation - full service in gateway mode, state root calculation only in outbound mode)

### Data Handling Convergence
- **`gateway.CanonicalDBService`** is the canonical SQLite root for gateway mode; it now contains only lifecycle code (Open, Close, Wait, GetDB, GetVault, schema/migrations, maintenance loop). All domain logic has been extracted to dedicated service fields. In outbound mode, it is used only for state root calculation and provides the shared vault instance.
- **`gateway.DocumentStoreService`** provides collection/ID-based document CRUD operations for gateway mode (implements governance.TransactionAuditStore). Callers access it directly via the `DocStore` field on CanonicalDBService - no delegation wrappers.
- **`gateway.StateRootService`** provides state merkle root calculation with caching for gateway mode (implements governance.StateRootProvider). Callers access it directly via the `StateRootSvc` field on CanonicalDBService - no delegation wrappers.
- **`gateway.SignerStoreService`** provides trusted signer CRUD operations for gateway mode (implements governance.SignerStore). Callers access it directly via the `SignerStore` field on CanonicalDBService - no delegation wrappers.
- **`gateway.AppPolicyStoreService`** provides app policy retrieval for gateway mode (implements governance.AppPolicyStore). Callers access it directly via the `AppPolicyStore` field on CanonicalDBService - no delegation wrappers.
- **`gateway.ReplayStoreService`** provides nonce replay protection for gateway mode (implements governance.ReplayStore). Callers access it directly via the `ReplayStore` field on CanonicalDBService - no delegation wrappers.
- **`gateway.KVStoreService`** provides TTL-aware ephemeral state with GLOB pattern scanning for gateway mode. Callers access it directly via the `KVStore` field on CanonicalDBService - no delegation wrappers.
- **`gateway.SSEEventService`** provides Server-Sent Events fan-out for gateway mode. Callers access it directly via the `SSEStore` field on CanonicalDBService - no delegation wrappers.
- **`gateway.BlobStoreService`** provides binary persistence for attachments and certificate material for gateway mode. Callers access it directly via the `BlobStore` field on CanonicalDBService - no delegation wrappers.
- **`storage.SuspendedTransactionService`** is the L3 approval workflow store used consistently in both gateway and outbound modes (implements `storage.SuspendedTransactionStore`).
- **`storage.ExecutionVaultService`** is the execution log and file diff storage for outbound mode.
- **`gateway.EncryptedKVAdapter`** implements `storage.TokenStore` and provides Sentinel token persistence for outbound mode. It wraps `gateway.KVStoreService` (from CanonicalDBService) and encrypts values at rest via `vault.Vault`.
- **`storage.SQLAuditStore`** is embedded in CanonicalDBService as the `AuditStore` field and provides the SQL-based audit storage foundation for gateway mode. In outbound mode, a separate SQLAuditStore instance is used for the Local-First Audit Architecture (LFAA).
- **`vault.Vault`** is shared across all storage services in outbound mode (reused from CanonicalDBService).

### Dependency Flow
- `scrubbing.ScrubbingService` depends on `storage.TokenStore` (interface). The outbound mode implementation is `gateway.EncryptedKVAdapter`.
- `gateway.EncryptedKVAdapter` has no dependency on `scrubbing.ScrubbingService` (circular dependency removed).
- All outbound storage services (ExecutionVaultService, EncryptedKVAdapter, SQLAuditStore, GitLedgerService) share the same `vault.Vault` instance from CanonicalDBService.
- `gateway.SecretManager` depends on `gateway.CanonicalDBService` for keystore access.

### Governance Stack (L1-L5)
- **L1**: `governance.L1Doctrine` (technical bedrock validation, threat detection, forbidden pattern matching)
- **L2**: `tribunal.TribunalService` (Tribunal-based deliberation producing L2 votes via Ed25519 signatures; gateway delegates deliberation via `LocalDeliberator`). The `TribunalStore` interface in `governance.L4Warden` loads `TribunalPolicy` for quorum verification.
- **L3**: `governance.L3Notary` (gateway mode uses `governance.outboundL3Notary` via `governance.NewGatewayL3Notary`, combining WebAuthn passkey proofs via `PasskeyService` and mTLS CLI proofs via `cliSessionVerifier`; outbound mode uses `governance.outboundL3Notary` via `governance.NewOutboundL3Notary` for CLI-based approval via suspended transactions)
- **L4**: `governance.L4Warden` (pre-dispatch verification gating, validating signatures, replay prevention, expiry, nonces, and state Merkle root)
- **L5**: `governance.L5Actuator` (isolated boundary tool dispatch via MCP/A2A, signed receipt production, audit logging)

### Shared Interface Implementations
- `gateway.SignerStoreService` implements: `governance.SignerStore` (gateway mode dedicated implementation).
- `gateway.TribunalStoreService` implements: `governance.TribunalStore` (gateway mode dedicated implementation, provides TribunalPolicy lookup for L4 Warden quorum verification).
- `gateway.AppPolicyStoreService` implements: `governance.AppPolicyStore` (gateway mode dedicated implementation).
- `gateway.ReplayStoreService` implements: `governance.ReplayStore` (gateway mode dedicated implementation).
- `gateway.StateRootService` implements: `governance.StateRootProvider` (gateway mode dedicated implementation).
- `gateway.DocumentStoreService` implements: `governance.TransactionAuditStore` (gateway mode dedicated implementation).
- `gateway.EncryptedKVAdapter` implements: `storage.TokenStore` (outbound mode).
- `storage.SuspendedTransactionService` implements: `storage.SuspendedTransactionStore` (used in both gateway and outbound modes).
- `governance.FilesystemSignerStore` implements: `governance.SignerStore` (used in outbound mode).
- `governance.outboundL3Notary` implements: `governance.L3Notary` (used in outbound mode via `NewOutboundL3Notary`; used in gateway mode via `NewGatewayL3Notary` with both `cliSessionVerifier` and `PasskeyService` as delegates).
- `gateway.cliSessionVerifier` implements: `governance.CLISessionVerifier` (used in gateway mode for mTLS CLI session verification within the L3 notary).

### PasskeyService HTTP Layer Consolidation
- **`passkey_service_http.go`** — All passkey HTTP handlers now live on `PasskeyService` as 4 factory methods (`RegisterChallenge`, `RegisterVerify`, `AuthenticateChallenge`, `AuthenticateVerify`) accepting a typed `passkeyHandlerConfig`, plus 3 direct handlers (`ListCredentials`, `RevokeCredential`, `CLIStatus`). The former `auth_controller_passkey.go` has been deleted entirely. Passkey handlers were stripped from `auth_controller_bootstrap.go` (non-passkey bootstrap handlers retained).
- **`passkey_service.go`** — Domain logic unchanged. Added `encodeCredID`/`decodeCredID` helpers (lines 85-92) for centralized base64 RawURL encoding of credential IDs. Added `DeleteSession` to the `sessionStore` interface for challenge purge-after-verify. Uses `bytes.Equal` for safe credential ID comparisons (line 435).
- **`internal/models/auth.go`** — `PasskeyCredential.Validate()` method (line 263) performs on-disk schema validation (COSE key parsing, ID size limits, attestation type validation, timestamp checks) before persistence in `addCredential`.

### Transport & Protocol Layer
- `pubsub.OperatorPubSubService` is the dispatcher for outbound mode (WebSocket pub/sub).
- `mcp.GatewayService` handles MCP/A2A protocol translation and downstream dispatch (shared between modes).
- `gateway.HTTPHandler` builds the HTTP/WebSocket surface for gateway mode.
- `gateway.GatewayWebSocketHandler` is the in-process pub/sub broker for gateway mode.
- `gateway.PKIAuthority` manages PKI hierarchy and certificate lifecycle for gateway mode.
- `network.Detector` detects host IP addresses and DNS names to configure TLS certificate identities dynamically during boot and renewal.

## MCP Native Tools

All Model Context Protocol (MCP) native tools are registered explicitly in `internal/services/mcp/native_tool_registry.go` inside the `RegisterNativeTools` function, avoiding global state mutation and init-based registrations. The tools are handled and dispatched via `mcp.NativeToolHandler`. Key tool categories include:

- **Database Inspection**: `DBDiscoverTopologyTool`, `DBQueryValidateTool`, `DBIsolatedReadTool`, `DBIndexTriageTool` for database schema discovery and safe read-only querying.
- **Log Analysis**: `LogStreamFilterTool` for structured log streaming and filtering.
- **System and Process Profiling**: `SysOOMDetectTool`, `ProcMetricTopTool`, `ProcSignalSafeTool`, `SysInfoTool`, `SysServiceStatusTool`, `SysContainerStatusTool`, `SysTimeClockTool`, `ProcTreeTool` for host system health and telemetry.
- **Network Inspection**: `NetSocketAuditTool`, `NetEndpointPingTool`, `NetHTTPProbeTool`, `NetDNSResolveTool`, `NetSSHKnownHostsTool` for connectivity and port auditing.
- **TLS and Security Inspection**: `TLSCertInspectTool` for certificate chain and TLS configuration verification.
- **File and Configuration Management**: `FSDiskProfileTool`, `ConfigDiffMaskTool`, `FSFileChecksumTool`, `FSDiskUsageTool`, `FileReadTool` for filesystem verification and config diff masking.
- **Environment and Cloud Integration**: `SysEnvVarsTool`, `GitOpsTool`, `CloudMetadataTool`, `K8sInspectTool`, `OperatorDeployTool` for metadata discovery and deployment workflows.
- **Shell Execution**: `RunShellCommandTool` for controlled shell command execution with scrubbing and audit logging.

## Critical Data Flows

| Flow | Path |
|------|------|
| Command execution results | `ExecutionService` → `CommandService` → `PubSubResultsService` → Pub/Sub channel |
| Audit events | `CommandService` / `FileOpsService` → `SQLAuditStore` → SQLite |
| File mutations | `FileEditService` → `GitLedgerService` → git commit |
| Suspended transactions | `L4Warden` → `storage.SuspendedTransactionService` (consistent in both gateway and outbound modes) |
| Action receipts | `L5Actuator` → `SQLAuditStore` (receipts table) + signed return |

## CLI-Invoked Verification & Reporting Service

The reporting system operates as a self-contained, offline verification utility invoked via CLI subcommands.

- **`internal/services/reporting/`**: Reads from database and storage backends (including decrypted execution vault, replay store, and git ledger directory) to write flat, deterministic CSV evidence files.
- **Cryptographic Verification**: Re-validates receipt signatures, verifies the sequential commitment hash chain, and checks the git ledger Merkle root to ensure system integrity.

## Test Infrastructure (Not Production)

The following packages are test-only and are not part of the production dependency tree:

**`internal/services/storage/storagetest/`** - Test-only audit storage implementations
- `TestSQLAuditStore` - Test-only monolithic audit service with Git ledger integration
- Implements `TransactionAuditStore` interface via a no-op `DocSet` method
- Production code uses `storage.SQLAuditStore` from `audit_store.go`

**`internal/services/pubsub/pubsubtest/`** - Test-only PubSub client mock
- `MockOperatorPubSubClient` - In-memory mock implementing the `pubsub.PubSubClient` interface
- Used by pubsub service tests and g8eo lifecycle/integration tests
- Follows the same pattern as `storagetest`, which keeps mock infrastructure out of production code

**Key distinction**: Test infrastructure is separated from production code to avoid import cycles. The `storagetest` and `pubsubtest` packages provide test implementations that should never be used in production code paths.

## Agent Harness & DoW Demo

**`internal/tools/agent_harness/`** - Reference client for real governance envelope submission
- `client/client.go` - mTLS client: `StateRoot`, `RegisterSigner`, `Approve` (uses `constants.APIPaths.*`)
- `client/envelope.go` - `SubmitMaximal`: builds real `GovernanceEnvelope` with L1/L2/L3, submits over mTLS
- `client/audit.go` - `AuditReceipts`, `ExportReceipts`, `DiscoverOperator` (parses cert SAN for offline session discovery)
- `scenarios/governance.go` - Governance scenarios: consensus, notary, delegation, veto, OOB approval
- `scenarios/dow_cross_cue.go` - DoW scenarios: `dow-cross-cue` (real slew envelope) and `dow-bft-veto` (veto envelope)
- `scenarios/scenario.go` - Scenario registry, `Execute`, `Posture` types

**`demos/dow/`** - Department of War tactical edge demo
- `gimbal.py` - Mock gimbal HTTP server on `net_secure` (records slew commands to `/var/gimbal/slews.jsonl`)
- `slew.sh` - Demo artifact mounted at `/usr/local/bin/slew` in operator container; wraps gimbal HTTP call
- `dow_simulator.py` - Display-only sensor narration for `agent-eoir` and `agent-pnt-fusion`
