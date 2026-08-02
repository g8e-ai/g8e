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
├── pubsub.PubSubClient (created in Start() if nil; used by OperatorPubSubService and PubSubResultsService)
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
│   │   ├── governance.StateRootProvider (gateway.StateRootService via Stores) [SHARED]
│   │   ├── governance.SignerStore (governance.FilesystemSignerStore)
│   │   ├── governance.L2ConsensusPolicyStore (via GovernanceDeps.ConsensusPolicyStore; NoopConsensusPolicyStore in outbound mode, gateway.ConsensusStoreService in gateway mode)
│   │   ├── governance.L1Doctrine (from GovernanceDeps.Doctrine in gateway mode; defaults to NewL1Doctrine() at pubsub_commands.go call site when nil in outbound mode)
│   │   └── governance.L3Notary (governance.outboundNotary implementation)
│   │       └── storage.SuspendedTransactionService
│   ├── governance.L5Actuator
│   │   ├── execution.ExecutionService
│   │   ├── storage.SQLAuditStore (from Stores.AuditStore) [SHARED]
│   │   ├── governance.TransactionAuditStore (auditStoreTransactionStore wrapper)
│   │   ├── scrubbing.ScrubbingService
│   │   └── governance.StateRootProvider
│   │   (L5Actuator does NOT depend on L3Notary or SignerStore; it trusts
│   │    VerifiedTransaction from L4Warden for L2/L3 status. See defense-in-depth
│   │    comment on L5Actuator struct.)
│   └── mcp.GatewayService [GATEWAY-ONLY] (declared in GatewayCommandServiceConfig, not present in CommandServiceConfig; outbound mode uses NewOperatorPubSubService, gateway mode uses NewGatewayOperatorPubSubService)
│       ├── response.Writer
│       └── storage.SuspendedTransactionService (as storage.SuspendedTransactionStore) [SHARED]
├── pubsub.PubSubResultsService
├── storage.ExecutionVaultService
│   ├── sqliteutil.DB
│   └── vault.Vault (shared with CanonicalDBService)
├── gateway.EncryptedKVAdapter (implements storage.TokenStore)
│   ├── gateway.KVStoreService (from Stores) [SHARED]
│   └── vault.Vault (shared with CanonicalDBService)
├── storage.SuspendedTransactionService
│   └── sqliteutil.DB
├── storage.GitLedgerService
│   └── vault.Vault (shared with CanonicalDBService)
├── storage.HistoryHandler
│   ├── storage.SQLAuditStore (from Stores.AuditStore) [SHARED]
│   └── storage.GitLedgerService
├── governance.ReplayStore (storage.SQLReplayStore)
│   └── sqliteutil.DB
├── scrubbing.ScrubbingService
│   └── storage.TokenStore (gateway.EncryptedKVAdapter)
├── lattice.Adapter
│   └── config.LatticeConfig
└── gateway.CanonicalDBService [SHARED] (lifecycle only: Open, Close, GetVault, schema/migrations, maintenance loop)
    ├── sqliteutil.DB
    ├── vault.Vault
    ├── keystore.Keystore (passed to OpenCanonicalDBService)
    └── gateway.Stores (returned by OpenCanonicalDBService)
        ├── gateway.DocumentStoreService
        ├── gateway.AppPolicyStoreService
        ├── gateway.SignerStoreService
        ├── gateway.ConsensusStoreService
        ├── gateway.StateRootService
        ├── gateway.ReplayStoreService
        ├── gateway.KVStoreService
        ├── gateway.SSEEventService
        ├── gateway.BlobStoreService
        ├── storage.SQLAuditStore
        └── sqliteutil.DB (raw SQLite connection for consumers needing direct DB access)

GatewayModeService (Gateway/Platform Mode) [MODE-SPECIFIC]
├── gateway.CanonicalDBService [SHARED] (lifecycle only: Open, Close, GetVault, schema/migrations, maintenance loop)
│   ├── sqliteutil.DB
│   ├── vault.Vault
│   └── gateway.Stores (returned by OpenCanonicalDBService, held as private stores field)
│       ├── gateway.DocumentStoreService
│       ├── gateway.AppPolicyStoreService
│       ├── gateway.SignerStoreService
│       ├── gateway.ConsensusStoreService
│       ├── gateway.StateRootService
│       ├── gateway.ReplayStoreService
│       ├── gateway.KVStoreService
│       ├── gateway.SSEEventService
│       ├── gateway.BlobStoreService
│       ├── storage.SQLAuditStore
│       └── sqliteutil.DB (raw SQLite connection for consumers needing direct DB access)
├── storage.SuspendedTransactionService (for L3 approval workflow)
│   └── sqliteutil.DB
├── gateway.GatewayWebSocketHandler
├── gateway.AuthService
│   ├── gateway.DocumentStoreService (from Stores [SHARED])
│   ├── gateway.PKIAuthority
│   ├── gateway.UserService
│   ├── gateway.PersonaService
│   │   └── gateway.DocumentStoreService (from Stores [SHARED])
│   ├── gateway.JWKSProvider (optional, for external IdP JWT auth)
│   └── response.Writer
├── gateway.PKIAuthority
│   ├── gateway.DocumentStoreService (from Stores [SHARED])
│   └── gateway.SecretManager (local variable in NewGatewayModeService, not a retained field)
│       ├── sqliteutil.DB
│       └── keystore.Keystore (via gateway.CanonicalDBService)
├── gateway.RegistrationService
│   ├── gateway.DocumentStoreService (from Stores [SHARED])
│   ├── gateway.KVStoreService (from Stores [SHARED])
│   ├── gateway.PKIAuthority
│   ├── gateway.UserService
│   ├── gateway.CLISessionService
│   └── gateway.OperatorSessionService
├── gateway.PasskeyService (domain logic only)
│   └── gateway.DocumentStoreService (from Stores [SHARED]) (via dbUserStore and dbSessionStore wrappers)
├── gateway.PasskeyHandler (HTTP layer, embeds *PasskeyService)
│   ├── gateway.PasskeyService [SHARED]
│   ├── gateway.WebSessionService (for session creation on browser flows)
│   ├── response.Writer (for HTTP responses)
│   └── gateway.PasskeyOrchestrator (business orchestration: MCP, suspended tx, SSE, pubsub)
│       ├── gateway.MCPServiceProvider
│       ├── storage.SuspendedTransactionStore
│       ├── gateway.SSEEventService
│       └── gateway.GatewayWebSocketHandler
├── gateway.UserService
│   └── gateway.DocumentStoreService (from Stores [SHARED])
├── gateway.CLISessionService
│   └── gateway.DocumentStoreService (from Stores [SHARED])
├── gateway.OperatorSessionService
│   └── gateway.DocumentStoreService (from Stores [SHARED])
├── gateway.WebSessionService
│   └── gateway.DocumentStoreService (from Stores [SHARED])
├── gateway.HTTPHandler (receives Stores via HTTPHandlerDependencies, delegates to controllers)
│   ├── gateway.GatewayWebSocketHandler
│   ├── gateway.AuthService
│   ├── gateway.PKIAuthority
│   ├── gateway.CLISessionService
│   ├── gateway.OperatorSessionService
│   ├── gateway.WebSessionService
│   ├── gateway.RegistrationService
│   ├── gateway.PasskeyHandler (includes approval handlers via passkey_service_approvals.go)
│   │   └── gateway.PasskeyOrchestrator (business orchestration via PasskeyHandlerDeps constructor)
│   │       ├── gateway.MCPServiceProvider
│   │       ├── storage.SuspendedTransactionStore
│   │       ├── gateway.SSEEventService
│   │       └── gateway.GatewayWebSocketHandler
│   ├── gateway.UserService
│   ├── gateway.AppEnrollmentService
│   │   ├── gateway.DocumentStoreService (from Stores [SHARED])
│   │   └── gateway.PKIAuthority
│   ├── gateway/console (Console SPA embed filesystem)
│   ├── mcp.GatewayService [SHARED]
│   ├── gateway.GovernanceController (governance envelope submission, consensus deliberation)
│   │   ├── consensus.ConsensusService (injected at construction time; nil = not configured for posture)
│   │   ├── governance.EnvelopeProcessor (injected at construction time; nil = not configured for posture)
│   │   └── response.Writer
│   ├── gateway.PKIController (PKI enrollment, CSR signing, trust scripts, deploy scripts)
│   │   ├── gateway.PKIAuthority [SHARED]
│   │   ├── gateway.AppEnrollmentService [SHARED]
│   │   ├── gateway.RegistrationService [SHARED]
│   │   └── response.Writer
│   ├── gateway.AuditController (audit receipts, audit events, audit summary, audit report)
│   │   ├── storage.SQLAuditStore (from Stores [SHARED])
│   │   └── response.Writer
│   ├── gateway.DataController (data DB, KV store, blob storage, SSE events, pub/sub publish)
│   │   ├── gateway.DocumentStoreService (from Stores [SHARED])
│   │   ├── gateway.KVStoreService (from Stores [SHARED])
│   │   ├── gateway.SSEEventService (from Stores [SHARED])
│   │   ├── gateway.BlobStoreService (from Stores [SHARED])
│   │   ├── gateway.GatewayWebSocketHandler [SHARED]
│   │   └── response.Writer
│   ├── gateway.SignerController (governance trusted signers)
│   │   ├── gateway.DocumentStoreService (from Stores [SHARED])
│   │   ├── gateway.SignerStoreService (from Stores [SHARED])
│   │   └── response.Writer
│   ├── gateway.BootstrapController (bootstrap, CLI enrollment, device enrollment, bootstrap status)
│   │   ├── gateway.UserService [SHARED]
│   │   ├── gateway.PKIAuthority [SHARED]
│   │   ├── gateway.CLISessionService [SHARED]
│   │   ├── gateway.OperatorSessionService [SHARED]
│   │   ├── gateway.DocumentStoreService (from Stores [SHARED])
│   │   ├── response.Writer
│   │   └── actuatorKeyReader (fileActuatorKeyReader, reads actuator public key from disk)
│   ├── gateway.EnrollmentTokenController (enrollment token generate, validate)
│   │   ├── gateway.EnrollmentTokenService (created in HTTPHandler constructor, manages enrollment token lifecycle with TTL-based cleanup)
│   │   ├── response.Writer
│   ├── gateway.UserController (user creation, user me)
│   │   ├── gateway.UserService [SHARED]
│   │   ├── response.Writer
│   ├── gateway.SessionController (logout, web session)
│   │   ├── gateway.DocumentStoreService (from Stores [SHARED])
│   │   ├── response.Writer
│   ├── gateway.AdminController (app policies, consensuss, app revocation)
│   │   ├── gateway.DocumentStoreService (from Stores [SHARED])
│   │   ├── gateway.SignerStoreService (from Stores [SHARED])
│   │   ├── gateway.ConsensusStoreService (from Stores [SHARED])
│   │   ├── gateway.UserService [SHARED]
│   │   └── response.Writer
│   ├── gateway.SSEController (SSE push, events, stream)
│   │   ├── gateway.DocumentStoreService (from Stores [SHARED])
│   │   ├── gateway.KVStoreService (from Stores [SHARED])
│   │   ├── gateway.SSEEventService (from Stores [SHARED])
│   │   ├── gateway.GatewayWebSocketHandler [SHARED]
│   │   ├── gateway.AuthService [SHARED]
│   │   └── response.Writer
│   ├── gateway.HealthController (health checks, state root)
│   │   ├── gateway.DocumentStoreService (from Stores [SHARED])
│   │   ├── gateway.StateRootService (from Stores [SHARED])
│   │   └── response.Writer
│   ├── gateway.OperatorController (operator list, terminate, bind/unbind, target context, reauth)
│   │   ├── gateway.RegistrationService [SHARED]
│   │   ├── gateway.AuthService [SHARED]
│   │   └── response.Writer
│   └── response.Writer
├── http.Server (HTTPS port, mTLS-enforced public router)
├── http.Server (HTTP port, bootstrap-only router)
├── governance.gatewayNotary (via governance.NewGatewayL3Notary, implements governance.L3Notary)
│   ├── gateway.cliSessionVerifier (via NewCLISessionVerifier, implements governance.CLISessionVerifier)
│   │   ├── gateway.DocumentStoreService (from Stores [SHARED])
│   │   ├── gateway.PKIAuthority
│   │   ├── gateway.UserService
│   │   └── gateway.CLISessionService
│   └── gateway.PasskeyService (as governance.L3Notary for WebAuthn proofs, domain logic only)
│       └── gateway.DocumentStoreService (from Stores [SHARED])
├── consensus.ConsensusService
│   ├── governance.L1Doctrine
│   ├── consensus.ConsensusMember (one or more enrolled members with Ed25519 keys)
│   └── response.Writer
├── mcp.GatewayService [SHARED]
│   ├── response.Writer
│   ├── storage.SuspendedTransactionService (as storage.SuspendedTransactionStore) [SHARED]
│   ├── scrubbing.ScrubbingService
│   ├── mcp.FieldPathRegistry
│   ├── mcp.NativeToolHandler
│   ├── mcp.AuditEventRecorder (interface; storage.SQLAuditStore in production via Stores.AuditStore, noopAuditEventRecorder when not wired)
│   ├── RuntimeDependencies (atomic.Pointer, set once via SetRuntimeDeps before first request):
│   │   ├── mcp.SessionValidator (set by in-process OperatorPubSubService in gateway mode)
│   │   ├── mcp.AuditLogger (pubsubAuditLogger, set by in-process OperatorPubSubService in gateway mode)
│   │   ├── governance.EnvelopeProcessor (set by in-process OperatorPubSubService in gateway mode)
│   │   ├── StateRootProvider (set by in-process OperatorPubSubService in gateway mode)
│   │   ├── Ed25519 signing key/keyID (set by in-process OperatorPubSubService in gateway mode)
│   │   ├── downstreamURL (MCP egress, set by in-process OperatorPubSubService in gateway mode)
│   │   ├── DBService (mcp.FieldReader, gateway.DocumentStoreService) [SHARED] (set by in-process OperatorPubSubService in gateway mode; not applicable in outbound mode)
│   │   └── consensus.L2ConsensusDeliberator (consensus.LocalDeliberator in gateway mode, not applicable in outbound mode)
│   ├── a2aDownstreamURL (construction-phase, immutable after NewGatewayService)
│   └── publicBaseURL (construction-phase, immutable after NewGatewayService)
└── response.Writer
```

## Structural Observations

### Mode Bifurcation
- **Mode-specific services**: `G8eoService` (outbound mode only), `GatewayModeService` (gateway mode only), `mcp.GatewayService` (gateway-only; `MCPGateway` is in `GatewayCommandServiceConfig` and wired via `NewGatewayOperatorPubSubService`; not present in outbound mode's `CommandServiceConfig`)
- **Shared services**: `CanonicalDBService` (used in both modes for state root calculation - full service in gateway mode, state root calculation only in outbound mode)
- **Governance dependencies**: `FieldReader`, `ConsensusPolicyStore`, `ReplayStore`, `StateRootProvider`, `TransactionAudit`, `L3Notary`, `SignerStore`, and `Doctrine` are consolidated in `pubsub.GovernanceDeps`, passed as a separate parameter to `NewOperatorPubSubService` and embedded via `GovDeps *GovernanceDeps` in `GatewayCommandServiceConfig`. In outbound mode, `ConsensusPolicyStore` and `FieldReader` default to no-op implementations (`NoopConsensusPolicyStore`, `NoopFieldReader`) via constructor-level defaults in `NewOperatorPubSubService`; in gateway mode, all fields are populated via `GetGovernanceDeps()`

### Data Handling Convergence
- **`gateway.CanonicalDBService`** is the canonical SQLite root for gateway mode; it now contains only lifecycle code (Open, Close, GetVault, schema/migrations, maintenance loop). All domain logic has been extracted to dedicated service fields. In outbound mode, it is used only for state root calculation and provides the shared vault instance.
- **`gateway.DocumentStoreService`** provides collection/ID-based document CRUD operations for gateway mode (implements governance.TransactionAuditStore). Callers access it directly via the `DocStore` field on `Stores` (returned by `OpenCanonicalDBService`) - no delegation wrappers.
- **`gateway.StateRootService`** provides state merkle root calculation with caching for gateway mode (implements governance.StateRootProvider). Callers access it directly via the `StateRootSvc` field on `Stores` (returned by `OpenCanonicalDBService`) - no delegation wrappers.
- **`gateway.SignerStoreService`** provides trusted signer CRUD operations for gateway mode (implements governance.SignerStore). Callers access it directly via the `SignerStore` field on `Stores` (returned by `OpenCanonicalDBService`) - no delegation wrappers.
- **`gateway.AppPolicyStoreService`** provides app policy retrieval for gateway mode (implements governance.AppPolicyStore). Callers access it directly via the `AppPolicyStore` field on `Stores` (returned by `OpenCanonicalDBService`) - no delegation wrappers.
- **`gateway.ReplayStoreService`** provides nonce replay protection for gateway mode (implements governance.ReplayStore). Callers access it directly via the `ReplayStore` field on `Stores` (returned by `OpenCanonicalDBService`) - no delegation wrappers.
- **`gateway.KVStoreService`** provides TTL-aware ephemeral state with GLOB pattern scanning for gateway mode. Callers access it directly via the `KVStore` field on `Stores` (returned by `OpenCanonicalDBService`) - no delegation wrappers.
- **`gateway.SSEEventService`** provides Server-Sent Events fan-out for gateway mode. Callers access it directly via the `SSEStore` field on `Stores` (returned by `OpenCanonicalDBService`) - no delegation wrappers.
- **`gateway.BlobStoreService`** provides binary persistence for attachments and certificate material for gateway mode. Callers access it directly via the `BlobStore` field on `Stores` (returned by `OpenCanonicalDBService`) - no delegation wrappers.
- **`Stores.DB`** provides the raw `sqliteutil.DB` connection for consumers needing direct DB access (e.g., `NewConsensusStoreService`, `NewStateRootService`). Accessible via the `DB` field on `Stores` (returned by `OpenCanonicalDBService`).
- **`gateway.Stores`** is a read-only aggregation struct returned by `OpenCanonicalDBService`. It bundles 11 store services for transport convenience. Consumers decompose it at the call site - controllers receive individual stores via `Deps` structs (e.g., `AuditControllerDeps`, `DataControllerDeps`, `SignerControllerDeps`, `BootstrapControllerDeps`), not the whole `Stores` struct.
  The trade-off: `GatewayModeService` and `G8eoService` retain the full `Stores` as a field, giving them access to all 11 stores even if they only use 2-4. This is acceptable because (1) the struct is read-only, (2) Go's type system prevents accessing the wrong store, and (3) splitting into themed groups would add types without reducing actual coupling since controllers already get individual stores via DI.
- **`storage.SuspendedTransactionService`** is the L3 approval workflow store used consistently in both gateway and outbound modes (implements `storage.SuspendedTransactionStore`). In both `GatewayModeService` and `G8eoService`, a single `suspendedTxStore` field (typed as `*storage.SuspendedTransactionService`) serves both store operations and `Close()` - no separate closer field.
- **`mcp.NewGatewayService`** fails fast on construction errors: `FieldPathRegistry` initialization errors are returned (not silently logged), making governance system initialization failures fatal. The `Dependencies.FieldPathRegistryFactory` field allows tests to inject a failing factory.
- **`mcp.AuditEventRecorder`** interface on `GatewayService` replaces a nil-in-production `*storage.SQLAuditStore` field. `NewGatewayService` defaults to `noopAuditEventRecorder` when `Dependencies.AuditStore` is nil. Production wires `stores.AuditStore` (`*storage.SQLAuditStore`) via `Dependencies.AuditStore`. This eliminates all nil guards at call sites - the field is always non-nil.
- **`storage.ExecutionVaultService`** is the execution log and file diff storage for outbound mode.
- **`gateway.EncryptedKVAdapter`** implements `storage.TokenStore` and provides Sentinel token persistence for outbound mode. It wraps `gateway.KVStoreService` (from Stores) and encrypts values at rest via `vault.Vault`.
- **`storage.SQLAuditStore`** is held in `Stores` as the `AuditStore` field and provides the SQL-based audit storage foundation for both gateway and outbound modes. In outbound mode, the standalone instance has been removed; `g8eo.go` reuses `Stores.AuditStore` for all audit writes (L5Actuator, HistoryHandler, session management), eliminating a redundant connection pool and pruner on the same `g8e.db` file.
- **`vault.Vault`** is shared across all storage services in outbound mode (reused from CanonicalDBService).

### Dependency Flow
- `scrubbing.ScrubbingService` depends on `storage.TokenStore` (interface). The outbound mode implementation is `gateway.EncryptedKVAdapter`.
- `gateway.EncryptedKVAdapter` has no dependency on `scrubbing.ScrubbingService` (circular dependency removed).
- All outbound storage services (ExecutionVaultService, EncryptedKVAdapter, SQLAuditStore, GitLedgerService) share the same `vault.Vault` instance from CanonicalDBService.
- `gateway.SecretManager` depends on `gateway.CanonicalDBService` for keystore access.

### Governance Stack (L1-L5)
- **L1**: `governance.L1Doctrine` (technical bedrock validation, threat detection, forbidden pattern matching). Constructed via `NewL1DoctrineFromDir(doctrineDir)` which loads `*.json` doctrine files from the given directory and appends them to hardcoded MITRE detectors. Empty dir falls back to `NewL1Doctrine()` (backward compatible). The doctrine instance is shared between the MCP Gateway ThreatScanner and L4Warden via `GatewayModeService.doctrine` -> `GovernanceDeps.Doctrine` -> `NewL4Warden()`.
- **L2**: `consensus.ConsensusService` (Consensus-based deliberation producing L2 votes via Ed25519 signatures; gateway delegates deliberation via `LocalDeliberator`). The `L2ConsensusPolicyStore` interface in `governance.L4Warden` loads consensus policy for quorum verification.
- **L3**: `governance.L3Notary` - composable notary design with two production implementations sharing primitives:
  - `governance.gatewayNotary` (via `governance.NewGatewayL3Notary`) - passkey authorization (`L3Notary` delegate) + optional CLI mTLS session verification (`CLISessionVerifier`). Does NOT access suspended transactions.
  - `governance.outboundNotary` (via `governance.NewOutboundL3Notary`) - suspended transaction lookup + Ed25519 signature verification (shared via `verifyOutboundProof`).
  - **Composable primitives**: `CLISessionVerifier` interface (shared by `gatewayNotary`); `verifyOutboundProof` shared function (suspended tx + signature logic used by `outboundNotary`).
- **L4**: `governance.L4Warden` (pre-dispatch verification gating, validating signatures, replay prevention, expiry, nonces, and state Merkle root)
- **L5**: `governance.L5Actuator` (isolated boundary tool dispatch via MCP/A2A, signed receipt production, audit logging). Does NOT re-verify L2/L3 proofs; trusts `VerifiedTransaction` from L4Warden. The L4->L5 separation is the defense-in-depth boundary: L4 verifies, L5 executes and records.

### Shared Interface Implementations

Governance store interfaces are defined in dedicated files under `internal/services/governance/`:
- `replay_store.go` - `ReplayStore` interface
- `state_root_provider.go` - `StateRootProvider` interface
- `signer_store.go` - `SignerStore` interface + `FailClosedSignerStore` (production fail-closed fallback) + `FilesystemSignerStore` (outbound impl)
- `l2_consensus.go` - `L2ConsensusPolicyStore` interface
- `app_policy_store.go` - `AppPolicyStore` interface
- `transaction_audit_store.go` - `TransactionAuditStore` interface

- `gateway.SignerStoreService` implements: `governance.SignerStore` (gateway mode dedicated implementation).
- `gateway.ConsensusStoreService` implements: `governance.L2ConsensusPolicyStore` (gateway mode dedicated implementation, provides ConsensusPolicy lookup for L4 Warden quorum verification).
- `gateway.AppPolicyStoreService` implements: `governance.AppPolicyStore` (gateway mode dedicated implementation).
- `gateway.ReplayStoreService` implements: `governance.ReplayStore` (gateway mode dedicated implementation).
- `gateway.StateRootService` implements: `governance.StateRootProvider` (gateway mode dedicated implementation).
- `gateway.DocumentStoreService` implements: `governance.TransactionAuditStore` (gateway mode dedicated implementation).
- `gateway.EncryptedKVAdapter` implements: `storage.TokenStore` (outbound mode).
- `storage.SuspendedTransactionService` implements: `storage.SuspendedTransactionStore` (used in both gateway and outbound modes).
- `governance.FilesystemSignerStore` implements: `governance.SignerStore` (used in outbound mode).
- `governance.gatewayNotary` implements: `governance.L3Notary` (gateway mode via `NewGatewayL3Notary(cliVerifier, passkeyVerifier, logger)` with `CLISessionVerifier` and `L3Notary` passkey delegate; no suspended store dependency).
- `governance.outboundNotary` implements: `governance.L3Notary` (outbound mode via `NewOutboundL3Notary` for suspended transaction + signature verification only).
- `gateway.cliSessionVerifier` implements: `governance.CLISessionVerifier` (used in gateway mode for mTLS CLI session verification within the L3 notary).

### PasskeyService/PasskeyHandler Domain-HTTP Split
- **`passkey_service.go`**: `PasskeyService` reduced to domain-only fields (`userStore`, `sessionStore`, `webauthn`, `logger`, `rpID`, `rpName`). `NewPasskeyService` signature simplified to `(db, logger, cfg)`. Retains domain logic: `VerifyL3Proof`, `GenerateRegistrationChallenge`, `VerifyRegistration`, `GenerateAuthenticationChallenge`, `VerifyAuthentication`, `GenerateApprovalChallenge`, `addCredential`, `listCredentials`, `revokeCredential`, `getUser`. `VerifyL3Proof` stays on `PasskeyService` (L3 binding to transaction hash per architectural guardrails).
- **`passkey_service_http.go`**: All passkey HTTP handlers now live on `*PasskeyHandler` as 4 factory methods (`RegisterChallenge`, `RegisterVerify`, `AuthenticateChallenge`, `AuthenticateVerify`) accepting a typed `passkeyHandlerConfig`, plus 3 direct handlers (`ListCredentials`, `RevokeCredential`, `CLIStatus`). All 7 methods have Swagger annotations (`@Summary`/`@Router`/`@Success`/`@Failure`).
- **`passkey_service_approvals.go`**: 6 approval functions (`handleApprovalAction` dispatcher, `handleApprovalChallenge`, `handleApprovalVerify`, `handleCLIApprovalStatus`, `handleApprovalPage`, `handleListSuspendedTransactions`) now live on `*PasskeyHandler`. All business dependencies (`mcpSvc`, `suspendedStore`, `sseStore`, `pubsub`) are encapsulated in `PasskeyOrchestrator` and accessed via `h.orchestrator.*` - no post-construction setters remain.
- **`PasskeyOrchestrator`** (`passkey_orchestrator.go`): Encapsulates cross-cutting business concerns of the passkey approval flow - MCP service provision, suspended transaction management, SSE event publishing, and WebSocket broadcasting. Methods: `GetSuspendedTransaction`, `ResumeWithL3Proof`, `ListSuspendedTransactions`, `EmitApprovalCompletedSSE`, `EmitPasskeyRegisteredSSE`. SSE emission methods no-op when `sseStore` or `pubsub` is nil.
- **`PasskeyHandler`** struct embeds `*PasskeyService` and adds HTTP concerns (`webSessionSvc`, `responder`, `maxPayload`, `crossOrigin`) plus a single `orchestrator *PasskeyOrchestrator` field. `NewPasskeyHandler(deps PasskeyHandlerDeps)` constructor wires all dependencies at construction time.
- **`gateway_service.go`**: `passkey` field -> `*PasskeyHandler`, both constructors updated, `GetGovernanceDeps` returns `*pubsub.GovernanceDeps` (consolidating all governance dependencies including `L3Notary` which wraps `ls.passkey.PasskeyService` via `NewGatewayL3Notary`).
- **`gateway_http.go`**: `HTTPHandlerDeps.Passkey` and `HTTPHandler.passkey` -> `*PasskeyHandler`. `GetPasskeyService()` renamed to `GetPasskeyHandler()`.
- **`gateway.auth_controller.go`**: `AuthController.passkey` field and `newAuthController` param -> `*PasskeyHandler`. (Historical note: `AuthController` has since been split into 4 sub-controllers - see HTTP Controller Decomposition below.)
- **`passkey_service_approvals_test.go`**: Tests all handlers on `*PasskeyHandler` with mocked dependencies.
- **`passkey_service_http_test.go`**: Tests for the 7 passkey HTTP handlers on `*PasskeyHandler`.
- **`passkey_orchestrator_test.go`**: Unit tests for `PasskeyOrchestrator` - delegation tests for `GetSuspendedTransaction`, `ResumeWithL3Proof`, `ListSuspendedTransactions`, and no-op guard tests for `EmitApprovalCompletedSSE` and `EmitPasskeyRegisteredSSE`.
- **`passkey_service_test.go`**: Tests for `PasskeyService` domain logic, including `VerifyL3Proof` WebAuthn assertion verification.
- **`internal/models/auth.go`**: `PasskeyCredential.Validate()` method performs on-disk schema validation before persistence in `addCredential`.

### Transport & Protocol Layer
- `pubsub.OperatorPubSubService` is the dispatcher for outbound mode (WebSocket pub/sub).
- `mcp.GatewayService` handles MCP/A2A protocol translation and downstream dispatch (gateway mode only; shared between HTTP ingress and OperatorPubSubService egress).
- `gateway.HTTPHandler` is a thin router and middleware shell for gateway mode. It holds only router infrastructure (rate limiting, CORS, path traversal guard), direct accessors (`passkey`, `mcp`, `pubsub`), and 13 controller fields. All domain handler logic lives on controllers (`PKIController`, `AuditController`, `DataController`, `SignerController`, `BootstrapController`, `EnrollmentTokenController`, `UserController`, `SessionController`, `AdminController`, `OperatorController`, `SSEController`, `HealthController`, `GovernanceController`).
- `gateway.GatewayWebSocketHandler` is the in-process pub/sub broker for gateway mode.
- `gateway.PKIAuthority` manages PKI hierarchy and certificate lifecycle for gateway mode.
- `network.Detector` detects host IP addresses and DNS names to configure TLS certificate identities dynamically during boot and renewal.
- `governance_envelope.go` was deleted (only contained `SetEnvelopeProcessor` delegation). The synchronous envelope-processing endpoint at `/api/v1/governance/envelopes` and all governance logic (`verifyEnvelopeIdentityBinding`, `isAppComponent`, `classifyEnvelopeError`, `handleGovernanceEnvelope`) live on `GovernanceController` in `governance_controller.go`. `GovernanceController` receives `consensus.ConsensusService` and `governance.EnvelopeProcessor` as constructor arguments via `newGovernanceController`. A nil value means the feature is not configured for the current posture (e.g. doctrine mode has no consensus). Handlers check for nil and return 503 ("not configured for this posture"), not "not yet wired during boot."
- `gateway_db.go` embeds the SQLite schema from `db/schema.sql` (via `gatewaySchema` unexported var) for database initialization. `gateway_certs.go` defines certificate validity periods and common names for all g8e CAs (root, intermediate, serving, leaf, peer).
- `gateway_pubsub.go` defines `GatewayWebSocketHandler` for WebSocket-based publish/subscribe channels, including subscriber management and in-process handlers for governance command processing and SSE streaming.

### Gateway HTTP Dual-Router Architecture
- **`buildPublicRouter`** (HTTPS port): Full API surface with mTLS middleware via `auth.Middleware`. Routes include governance envelopes, audit, PKI management, user management, MCP/A2A ingress, SSE, pub/sub, console SPA, passkey endpoints, and approval UI. WebSessionAuth-protected routes bypass mTLS and use cookie-based auth with their own middleware. The landing page (`/`) redirects to `/console/`.
- **`buildHTTPRouter`** (HTTP port): Bootstrap-only surface for CA discovery, trust scripts, deploy scripts, CLI enrollment, and state endpoint. All other paths redirect to HTTPS. Wrapped with rate limiting and path traversal guard.
- **`RouteAuthRegistry`** classifies every route by its authentication mode (`RouteAuthNone`, `RouteAuthMTLS`, `RouteAuthWebSession`, `RouteAuthDual`). Exact paths are matched with highest priority, then prefix matches. Unknown routes default to `RouteAuthMTLS` (fail-closed). When JWKS is enabled, MCP/A2A and JIT passkey routes are reclassified to `RouteAuthNone` (JWT middleware handles auth).
- **`PrivilegedRouteRegistry`** blocks app certificates from governance envelope submission and query endpoints. Only operator and CLI auth are accepted on these routes.
- **`gateway_http_middleware.go`**: `rateLimitMiddleware` applies per-IP token bucket rate limiting with 5-minute stale entry cleanup. `pathTraversalGuard` rejects requests containing `..` path segments before ServeMux normalization.
- **`gateway_http_cors.go`**: CORS middleware for handling cross-origin requests from enrolled frontend applications. Validates origins against the gateway's configured allowed origins list.
- **`health_controller.go`**: Health check endpoint (`/health`) returns platform settings and state root status. Landing page handler redirects `/` to `/console/`. Bootstrap health check on the HTTP port returns a simplified health response. (Previously in `gateway_http_health.go`, now deleted.)
- **`gateway/docs/`**: Embedded OpenAPI/Swagger specifications (`docs.go` embeds `swagger.json` and `swagger.yaml`) served at `/swagger/` with Swagger UI.
- **`gateway/scripts/`**: Thread-safe deploy script templates (`g8e-deploy.sh`, `g8e-deploy.ps1`) with Go bindings in `templates.go`, initialized via `sync.Once`. Documented centrally in [scripts.md](../architecture/scripts.md#remote-deploy-scripts-gateway-served).
- **`gateway/console/`**: Embedded Console SPA (`console.go` exposes `Handler()` serving the static filesystem from `static/`).

### HTTP Controller Decomposition
- **`gateway.PKIController`** (`pki_controller.go`): PKI enrollment, CSR signing, CA bundle, trust scripts (Linux/macOS/Windows), deploy scripts, certificate revocation, app enrollment.
- **`gateway.AuditController`** (`audit_controller.go`): Audit receipts, audit events, audit summary, audit report. 4 dependencies (cfg, logger, auditStore, responder).
- **`gateway.DataController`** (`data_controller.go`): Data DB, KV store, blob storage, SSE events, pub/sub publish. 7 dependencies (cfg, logger, docStore, kvStore, sseStore, blobStore, pubsub, responder).
- **`gateway.SignerController`** (`signer_controller.go`): Governance trusted signers. 5 dependencies (cfg, logger, docStore, signerStore, responder).
- **`gateway.BootstrapController`** (`bootstrap_controller.go`): Local bootstrap with URL, bootstrap status, CLI enrollment, device enrollment. 9 dependencies (cfg, logger, docStore, userSvc, pki, cliSessionSvc, operatorSessionSvc, responder, actuatorKeyReader).
- **`gateway.EnrollmentTokenController`** (`enrollment_token_controller.go`): Enrollment token generation (mTLS-protected) and validation (public). 4 dependencies (cfg, logger, enrollmentTokenSvc, responder).
- **`gateway.UserController`** (`user_controller.go`): User creation (mTLS-protected), user me (web session). 4 dependencies (cfg, logger, userSvc, responder).
- **`gateway.SessionController`** (`session_controller.go`): Logout (clear cookie + delete web session), web session info. 4 dependencies (logger, docStore, responder, crossOrigin).
- **`gateway.AdminController`** (`admin_controller.go`): App policy management by signer, app revocation, consensus CRUD.
- **`gateway.OperatorController`** (`operator_controller.go`): Operator list, terminate, bind/unbind operators, set target context, reauth.
- **`gateway.SSEController`** (`sse_controller.go`): SSE event push, poll, and stream endpoints. Includes `authorizeSSERoute` for dual-auth (mTLS or web session) authorization. Heartbeat interval defaults to 30s via `newSSEController`.
- **`gateway.HealthController`** (`health_controller.go`): Health check, bootstrap health, state endpoint, and landing page. Previously in `gateway_http_health.go` (deleted).
- **`gateway.GovernanceController`** (`governance_controller.go`): Governance envelope submission, consensus deliberation. Both `consensus.ConsensusService` and `governance.EnvelopeProcessor` are injected at construction time via `newGovernanceController`. A nil value means the feature is not configured for the current posture (e.g. doctrine mode has no consensus). Handlers check for nil and return 503 ("not configured for this posture").

  **Two-phase construction (eliminates circular dependency):** `GatewayModeService` construction is split into two phases. Phase 1 (`NewGatewayModeService` / `build()`) opens the DB, creates stores, PKI, auth, and MCP gateway - but does NOT create the HTTP handler. Phase 2 (`InitHTTPHandler(consensusSvc, envProc)`) creates the HTTP handler and servers with all dependencies injected. This breaks the circular dependency: Consensus needs gateway stores -> gateway stores are available after Phase 1 -> ConsensusService is constructed between phases -> HTTPHandler (including GovernanceController) is created in Phase 2 with ConsensusService already in hand.

### JWT Authentication
- **`gateway.JWKSProvider`** (`jwks.go`): Optional external IdP JWT validation via JWKS endpoint. When configured, MCP/A2A routes accept JWT auth in addition to mTLS.
- **`gateway.NativeJWT`** (`jwt_native.go`): Native RSA-SHA256 JWT validation without external dependencies. Validates `exp`, `nbf`, `iat` claims with 60-second clock skew allowance. Used when JWKS is configured but no external HTTP call is needed for key resolution.
- **`gateway.AuthService`** applies JWT middleware to MCP/A2A routes and JIT passkey bootstrap routes when JWKS is configured. App policy enforcement and rate limiting apply to JWT-authenticated requests.

### Persona Service
- **`gateway.PersonaService`** (defined in `user_service.go`): Manages role-based access control personas. Initialized with `DefaultPersonaDefinitions` during gateway startup. `AuthService` references personas for authorization decisions.

## CLI Command Tree

The `g8e` binary (`internal/cli/cmd/main.go`) registers the following subcommands on the root Cobra command:

- **`gw`** (alias `gateway`): Gateway lifecycle management. Subcommands: `start` (background process; `--follow` flag runs in foreground as the re-exec target; `--interactive`/`-i` flag launches the onboarding wizard before startup), `stop`, `status`, `restart`, `logs`, `settings`, `reset`, `clean`, `setup` (interactive onboarding wizard; standalone entry point for `g8e gw setup`). Also includes `data`, `security`, and `tunnel` subcommand groups.
  - **`data`**: Administer the local platform over mTLS. Subcommands: `users`, `operators`, `settings`, `store`, `audit`.
  - **`security`**: Security validation checks. Subcommands: `validate`, `pki` (with `enroll` subcommand).
  - **`tunnel`**: Manage Cloudflare Tunnel for public gateway access. Subcommands: `create`, `run`, `status`.
- **`auth`**: Authentication and session management. Subcommands: `enroll` (CSR-based enrollment with passkey registration), `logout`, `approve` (interactive OOB approval of suspended transactions via passkey).
- **`mcp`**: MCP protocol operations. Subcommands: `stdio` (run g8e as an MCP server over stdio), `agent` (agent integration commands for AI coding tools). `agent` subcommands: `list` (list supported agent binaries), `show` (print MCP client configuration for a specific agent), `run` (launch an agent with g8e MCP configuration and native tools disabled; includes `verifyToolInterception` pre-launch config verification via `--verify` flag, enabled by default). Supported agents: Claude Code, OpenAI Codex, Goose, Gemini CLI.
- **`operator`**: Manage Operator instances. Subcommands: `list`, `start`, `cp`, `scp`, `deploy`, `stream`.
- **`vault`**: Encryption vault management. Subcommands: `init`, `unlock`, `rekey`, `status`, `reset`, `export`, `import`.
- **`test`**: Run test suites. Subcommands: `unit`, `integration`, `e2e`, `coverage`, `lint`, `chaos`, `summary`.
- **`demos`** (alias `demo`): Demo scenario management. Subcommands: `list`, `start`, `stop`, `status`, `clean`, `rebuild`, `reset`, `run`, `pull`, `export`, `import`, `images`, `scenarios` (with `list` and `run` subcommands).
- **`audit`**: Run audit reports for compliance. Subcommands: `receipts`, `export`, `report`, `events`, `summary`.
- **`report`**: Generate CSV evidence reports. Subcommands: `all`, `verify`.
- **`compliance`**: FedRAMP 20x (CR26) compliance operations. Subcommands: `export` (generate OSCAL `component-definition` and `assessment-results` JSON artifacts), `ksi` (evaluate KSIs and print result set as JSON), `ksi-history` (list KSI snapshot history or filter by KSI ID), `overlay` (load and validate COSAiS overlay catalogs).
- **`swagger`**: Manage Swagger/OpenAPI documentation. Subcommands: `init`, `serve`, `validate`.
- **`tui`**: Launch the Tactical Governance Console (TUI). Requires a running gateway, enrolled CLI credentials, and mTLS client configuration.
- **`gui`**: Enroll external frontend applications with the g8e Gateway. Subcommands: `enroll`, `show`, `verify`, `remove`.
- **`version`**: Print g8e build version information (version, build ID, build time, platform). With `--fips` flag, also reports FIPS 140-3 module status via the native `crypto/fips140` package and exits non-zero if FIPS mode is not active. Used as an auditor/operator self-check for FIPS-compliant builds.

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

- **`internal/services/reporting/`**: Reads from database and storage backends (including decrypted execution vault, replay store, git ledger directory, and suspended transaction store) to write flat, deterministic CSV evidence files. Modules: `reporter.go` (orchestrator), `verification.go` (cryptographic verification pass), `commitments.go`, `events.go`, `executions.go`, `file_mutations.go`, `ledger.go`, `receipts.go`, `replay.go`, `rows.go`, `suspended.go`, `csvwriter.go`.
- **Cryptographic Verification**: Re-validates receipt signatures, verifies the sequential commitment hash chain, and checks the git ledger Merkle root to ensure system integrity.
- **Test Coverage**: `verification_test.go` provides 15 hermetic tests covering all 5 verification checks (commitment chain integrity, git merkle root, file mutation linkage, receipt/commitment cross-link, context cancellation) with real SQLite + vault. `verification.go` at 80.8% coverage.

## Compliance Package

**`internal/services/compliance/`** - FedRAMP 20x (CR26) KSI/OSCAL/COSAiS compliance tooling

- **`ksi.go`**: Typed KSI model with `KSI`, `KSIMethod`, `KSIResult`, `KSIResultSet`, `KSICatalog`, `Evidence`, `AutomatedMethod` structs. Typed enums for `KSICategory`, `KSIStatus`, `CertificationClass`, `ValidationCycle`, `EvidenceType`. No raw maps. Catalog loaded from `docs/reference/ksi-catalog.json` (31 KSIs across 10 categories, seeded from CR26 reference).
- **`ksi_test.go`**: 11 unit tests covering catalog loading, validation (7 edge cases), lookup, class filtering, staleness detection (6 scenarios), JSON round-trip, error paths.
- **`ksi_evaluator.go`**: `KSIEvaluator` struct with `RegisterMethods`, `RegisterDefaultMethods`, `Evaluate` methods. Derives binary KSI status from live g8e state via `EvaluatorDeps` (audit store, ledger, commitment ledger). Enforces minimum method counts per certification class (fail-closed for Class C: at least 2). `DefaultMethods` provides 8 reusable method closures bound to automatable KSIs.
- **`ksi_evaluator_test.go`**: 17 unit tests covering method registration, evaluation (all-satisfied, insufficient methods, stale, method error, nil deps, empty stores, class thresholds, context cancellation), result set helpers, default method correctness, full integration.
- **`oscal.go`**: `OSCALExporter` struct with `GenerateComponentDefinition` and `GenerateAssessmentResults` methods. Typed OSCAL structs for `component-definition` and `assessment-results` models. Evidence anchors link to receipt IDs, ledger commit hashes, and LFAA execution IDs. No `map[string]interface{}`.
- **`oscal_test.go`**: 11 unit tests covering component-definition generation, assessment-results generation, nil/empty/unknown-KSI error paths, JSON marshaling, not-applicable status mapping, deterministic UUID generation.
- **`ksi_history.go`**: `KSIHistoryStore` struct with `SaveSnapshot`, `ListSnapshots`, `GetHistoryForKSI`, `PruneOlderThan` methods. Persists `KSIResultSet` snapshots to `.g8e/data/compliance/ksi-history/` via `RuntimeFileService`. 90-day retention pruning.
- **`ksi_history_test.go`**: 10 unit tests covering write/read round-trip, multiple snapshots sorted, per-KSI history filtering, not-found errors, empty directory, nil result set, pruning, filename format.
- **`overlay_loader.go`**: `Overlay`, `OverlayCatalog`, `OverlayStatus` types. `LoadOverlayCatalog(path)`, `LoadOverlaysFromDir(dir)` (mirrors `NewL1DoctrineFromDir`), `Validate()`, `FindOverlay`/`HasOverlay`, `ValidateOverlayRefs(ksiCat, overlayCat)` for dangling reference detection.
- **`overlay_loader_test.go`**: 21 unit tests covering catalog load, validation (8 sub-tests), lookup, directory loader (8 tests), ref validation (3 tests), JSON round-trip.
- **Package coverage**: 89.4% (exceeds 75% threshold).
- **Error constants**: `ErrKSINotSatisfied`, `ErrKSIInsufficientMethods`, `ErrKSICatalogInvalid`, `ErrOverlayNotFound`, `ErrOverlayCatalogInvalid`, `ErrKSIHistoryWriteFailed`, `ErrKSIHistoryReadFailed`, `ErrKSIHistoryParseFailed`, `ErrKSIHistoryEmpty` in `internal/constants/errors.go`.
- **Path constants**: `ComplianceDirname`, `KSICatalogFilename`, `COSAiSOverlaysFilename`, `OSCALAssessmentResultsFilename`, `OSCALComponentDefFilename`, `KSIHistoryDirname`, `KSIHistoryFilenamePrefix`, `KSIHistoryRetentionDays`, `DefaultOverlayDirPath` in `internal/constants/paths.go`.

## CLI Serve Layer (Operator & Gateway Boot)

- **`internal/cli/serve/cert.go`**: PKI enrollment and certificate lifecycle: `PerformAutomaticEnrollment` (initial enrollment via CSR + trust bundle fetch, returns `(sessionID, err)` instead of calling `os.Setenv` internally), `RenewOperatorCertificate` (re-enrollment for expiring certs, decomposed into 5 testable units: `checkCertExpiry`, `fetchAndSaveTrustBundle`, `buildMTLSClient`, `submitRenewal`, `saveRenewedCerts`), `RunClientCertRenewalLoop` (periodic renewal check). `CertPaths` struct decouples path configuration from `paths.Infra`. HTTP client uses 15s timeout via `http.Client` + `http.NewRequestWithContext`. Error wrapping standardized with `ErrEmptyTrustBundle`, `ErrCAParseFailed`, `ErrMissingRequiredField`.
- **`internal/cli/serve/operator.go`**: Operator boot sequence: `RunOperator` orchestrates config loading, trust bundle setup, enrollment, and signal handling. Extracted helpers: `resolveKeyPath`, `resolveCertPath`, `loadClientCertPair`, `buildOperatorLoadOptions`.
- **`internal/cli/serve/gateway.go`**: Gateway boot sequence: `RunGateway` orchestrates config loading, `GatewayModeService` construction, in-process execution service initialization, consensus bootstrap (policy seeding via `bootstrapConsensusPolicy`, key loading via `BootstrapConsensus` with `FileKeyProvider`, `NewLocalDeliberator` for L2 consensus), in-process `OperatorPubSubService` construction via `NewGatewayOperatorPubSubService` with `GatewayCommandServiceConfig` (embedding base `CommandServiceConfig` plus `MCPGateway`, `GovDeps *GovernanceDeps`, and `L2ConsensusDeliberator`), `InitHTTPHandler(consensusSvc, cmdSvc)` two-phase construction, and graceful shutdown with 30-second timeout. `ExportActuatorPublicKey` writes the actuator public key to the PKI directory for receipt verification by external harnesses.
- **`internal/cli/serve/logger.go`**: Logger configuration: `ConfigureLogger` and `ConfigureLoggerWithOutput` produce `slog.Logger` instances with operator-friendly formatting and configurable log levels.
- **`internal/cli/serve/version.go`**: `VersionInfo` struct holds build-time metadata (version, build ID, build time, platform) set via ldflags.
- **`internal/cli/cmd/version.go`**: `versionCmd` implements the `g8e version` subcommand. With `--fips`, queries the native `crypto/fips140` package (`Enabled()`, `Enforced()`, `Version()`) to report FIPS 140-3 module status and exits non-zero if approved mode is not active. Used by `make verify-fips` and the `Dockerfile.fips` build-time self-check.
- **`internal/certs/embed.go`**: Injectable TLS primitives replacing package-level globals. `TrustStore` holds the CA trust bundle, `ClientIdentity` holds the mTLS client certificate, and `TLSConfig` combines them into a `*tls.Config` (TLS 1.3 minimum). `FIPSCurvePreferences()` returns the FIPS 140-3 compliant TLS key agreement curve set (`X25519MLKEM768`, `P-384`, `P-256`), excluding X25519. Returns a fresh slice on each call to prevent mutation of shared state.
- **`internal/cli/cmd/gateway.go`**: Gateway CLI command tree. `gatewayStartCmd` launches the gateway as a background process via `pm.StartOperator` (`ProcessManager.StartOperator`), resolving configuration from CLI flags and environment variables. With `--follow` flag, runs in foreground by calling `serve.RunGateway` directly. With `--interactive`/`-i` flag, launches the onboarding wizard (`internal/cli/wizard`) before startup; the wizard result is merged into resolved flags via `applyWizardConfig`. `gatewaySettingsCmd`, `gatewayResetCmd`, and `gatewayCleanCmd` manage gateway state over mTLS.
- **`internal/cli/cmd/tui.go`**: `tuiCmd` launches the Tactical Governance Console (TUI). Loads config, checks operator status, loads credentials, builds an mTLS client, and constructs an SSE stream URL using the CLI session ID for real-time updates.
- **`internal/cli/cmd/gwstdout.go`**: `printNextSteps` outputs guidance after the gateway starts, including CA trust instructions, CLI enrollment, operator deployment, and Console UI access.
- **`internal/services/gateway/cli_cert.go`**: `ExtractUserIDFromCert` extracts the user ID from a CLI mTLS certificate's SPIFFE URI SAN.
- **Test Coverage**: `cert_test.go` covers `PerformAutomaticEnrollment` (6 tests), `RenewOperatorCertificate` (9 tests), `RunClientCertRenewalLoop` (1 test) with hermetic `httptest.Server` and real certificate generation. `operator_test.go` covers extracted helpers at 100%. `gateway_test.go` covers gateway boot sequence. `internal/cli/serve` overall at 49.6% coverage.

## CLI Platform & Stream Packages

- **`internal/cli/platform/`**: Cross-platform process management for operator subprocesses. `process.go` provides core process lifecycle. `process_unix.go` and `process_windows.go` provide platform-specific process discovery and signal handling. `browser.go` provides cross-platform browser opening for console URLs.
- **`internal/cli/stream/`**: SSH and subprocess streaming for remote operator management. `stream.go` provides the streaming CLI command. `stream_ssh.go` provides SSH connection management for remote log streaming and command execution.
- **`internal/cli/sse/`**: Server-Sent Events client for CLI consumption of gateway SSE streams. `client.go` provides the SSE client implementation used by the TUI for real-time updates.

## CLI Wizard Package

- **`internal/cli/wizard/`**: Interactive onboarding wizard for `g8e gw start --interactive`/`-i`. Bubble Tea TUI with 4-step flow: Network & Identity → Security & Governance Posture → Agent Tooling & Routing → Review & Confirm. Uses `charmbracelet/bubbles` `textinput` for URL/string fields and arrow-key navigation for choices/toggles. Produces a focused `Config` (wizard-owned fields only); the `cmd` package owns conversion and merging via `wizardConfigFromFlags`/`applyWizardConfig`. Files: `model.go` (state struct, `NewModel`, `result()`), `update.go` (message routing, per-step key handling), `view.go` (lipgloss rendering per step), `steps.go` (step enum, ordering), `validate.go` (URL, origin, consensus, passkey validators), `styles.go` (color palette matching `tui/styles.go`), `run.go` (`Run` entry point, `Config`/`Options`/`Result` types), `messages.go` (custom message types).

## Runtime File Service

**`internal/services/fs/`** - File service for the `.g8e/` runtime directory

`RuntimeFileService` interface (`file_service.go`) is the canonical `.g8e/` file I/O abstraction for safe file operations within the `.g8e/` runtime directory. All paths are relative to the runtime directory root. The `localFS` implementation wraps `os.*` calls with atomic writes (tmp+rename), permission enforcement, and consistent error wrapping using `constants.Err*` constants.

In test verification patterns, use `fileSvc.ReadFile` or `fileSvc.Stat` to inspect runtime files, and use `errors.Is(err, constants.ErrNotFound)` as the replacement for `os.IsNotExist` in test assertions.

Interface methods:
- `Resolve(relPath)` - Converts a relative path to an absolute path within `.g8e/`
- `Rel(absPath)` - Converts an absolute `.g8e/` path back to a relative path
- `ReadFile(ctx, relPath)` - Reads a file; returns `constants.ErrNotFound` if missing
- `WriteFile(ctx, relPath, data, mode)` - Atomically writes a file with tmp+rename
- `MkdirAll(ctx, relPath, mode)` - Creates a directory tree
- `Stat(ctx, relPath)` - Returns `os.FileInfo`; returns `constants.ErrNotFound` if missing
- `FileExists(ctx, relPath)` - Returns `(bool, error)`, false for non-existent
- `Remove(ctx, relPath)` - Deletes a file; no-op if missing
- `RemoveAll(ctx, relPath)` - Deletes a directory tree; no-op if missing
- `ReadDir(ctx, relPath)` - Lists directory entries
- `Rename(ctx, oldPath, newPath)` - Atomically renames a file or directory
- `CreateRuntimeTree(ctx)` - Creates the full `.g8e/` directory tree with correct permissions. Called once at startup. Idempotent
- `EnforceDirPermissions(ctx, relPath, mode)` - Recursively enforces directory permissions
- `EnforceFilePermissions(ctx, relPath, mode)` - Enforces file permissions on a single file

Construction: `fs.NewRuntimeFileService(baseDir, logger)` creates a service scoped to `.g8e/` under `baseDir`.

## Lattice Adapter

**`internal/adapters/lattice/`** - Anduril Lattice COP gRPC adapter

- `client.go` - `Adapter` struct, `NewAdapter`, `Start`/`Stop` lifecycle, entity ID persistence, heartbeat sink registration. Uses `HeartbeatRegistrar` interface to break import cycle with `pubsub` package.
- `config/config.go` - `LatticeConfig` and `EntityConfig` structs, `Validate()`, `ValidateHeartbeatInterval()`. Extracted to sub-package to break import cycle (`config` → `lattice` → `fs`).
- `config.go` - Type aliases re-exporting `LatticeConfig`/`EntityConfig` from the `config` sub-package for backward compatibility.
- `auth.go` - `ClientCredentialsAuth` implementing `credentials.PerRPCCredentials`, OAuth2 client credentials flow with proactive token refresh and `ForceRefresh()` for `Unauthenticated` recovery.
- `retry.go` - `retryWithBackoff` with exponential backoff and jitter, `isRetryable` gRPC code classifier, `DefaultRetryOpts`.
- `interceptor.go` - `unaryRetryInterceptor` wraps unary RPCs with retry and token refresh on `Unauthenticated`.
- `presence.go` - `PublishPresence` constructs and publishes entity to Lattice EntityManager.
- `task_stream.go` - `subscribeToTasks` streaming RPC, `processStream` message handling, `isTaskAccepted` catalog filtering, `checkPostureFloor` governance gate, `reportTaskStatus` status reporting via `UpdateStatus` RPC.
- `gen/` - 46 generated `.pb.go` and `_grpc.pb.go` files from `third_party/anduril/` protos (EntityManager v1, TaskManager v1, tasks v2, ontology v1, api v1).
- `errors.go` - Package declaration only; all error sentinels live in `internal/constants/errors.go`.

Test helpers (per-package, build-tagged `integration` or test-only):
- `newTestFileSvc(t)` in `internal/services/gateway/test_setup_test.go` - Temp-backed fileSvc with full runtime tree for gateway tests
- `newTestFileSvc(t, baseDir)` in `internal/services/storage/storage_test_helpers_test.go` - Storage test fileSvc, returns fileSvc and data dir
- `NewTestFileSvc(t, baseDir)` in `internal/services/storage/storagetest/helpers.go` - Exported test fileSvc for storagetest consumers
- `newAuthTestFileSvc(t)` in `internal/cli/auth/client_test.go` - Auth client test fileSvc
- `newPlatformTestFileSvc(t, baseDir)` in `internal/cli/platform/process_test.go` - Platform test fileSvc
- `newCmdTestFileSvc(t)` in `internal/cli/cmd/vault_test.go` - CLI command test fileSvc (uses CWD as base)
- `newTestFileSvc(t)` in `internal/cli/serve/test_setup_test.go` - Serve test fileSvc

## Test Infrastructure (Not Production)

The following packages are test-only and are not part of the production dependency tree:

**`internal/services/storage/storagetest/`** - Test-only audit storage and token store implementations
- `TestSQLAuditStore` - Test-only monolithic audit service with Git ledger integration
- Implements `TransactionAuditStore` interface via a no-op `DocSet` method
- `TestTokenStore` - Thread-safe in-memory `storage.TokenStore` with TTL expiry support
- Production code uses `storage.SQLAuditStore` from `audit_store.go`

**`internal/services/pubsub/pubsubtest/`** - Test-only PubSub client mock
- `MockOperatorPubSubClient` - In-memory mock implementing the `pubsub.PubSubClient` interface
- Used by pubsub service tests and g8eo lifecycle/integration tests
- Follows the same pattern as `storagetest`, which keeps mock infrastructure out of production code

**`internal/services/governance/governancetest/`** - Test-only governance store fixtures
- `SimpleConsensusStore`, `SimpleAppPolicyStore`, `SimpleStateRootProvider` - In-memory implementations of governance store interfaces for unit tests
- Used by governance, pubsub, and chaos tests
- `FailClosedSignerStore` remains in `governance/signer_store.go` (not here) because `pubsub_commands.go` uses it as a production fail-closed fallback

**`internal/tools/chaos/`** - Test-only chaos testing for governance stack
- Generates a realistic distribution of governance events (70% good actor, 20% prompt injection, 10% MitM) against the local audit stack
- Drives `TransactionVerifier` + `Actuator` stack directly in-process, bypassing network/TLS
- Uses `storagetest.TestSQLAuditStore` and should not be used in production code paths

**Key distinction**: Test infrastructure is separated from production code to avoid import cycles. The `storagetest`, `pubsubtest`, `governancetest`, and `chaos` packages provide test implementations that should never be used in production code paths.

**`test/fixtures/gateway_fixture.go`** - In-process gateway test fixture (build tag: `integration`)
- `GatewayFixture` spins up a real `GatewayModeService` with `httptest.Server`, mTLS PKI, consensus enrollment, and in-process `OperatorPubSubService` wired with full governance dependencies
- Used by integration tests for MCP flow, A2A flow, L2 consensus, governance envelope verification, OOB suspension/approval, and downstream integration

**`test/e2e/harness.go`** - Docker-based E2E test fixture (build tag: `e2e`)
- `DockerE2EFixture` spins up docker-compose, allocates host ports, waits for gateway health, and tears down on cleanup
- Tests only what is observable from outside containers: HTTP health, CA bundle discovery, port reachability, and mTLS handshake over network
- Supports `G8E_E2E_SKIP_DOCKER` environment variable to skip when Docker is unavailable

**`test/integration_helper.go`** - Shared integration test helpers (build tag: `integration` or `e2e`)
- `NewLiveOperatorHTTPClient` creates an mTLS API client against a running g8e platform
- `ResolveRepoRootFromTestDir` finds the repository root using `go list -m`

**Integration test files** (`test/`):
- `universal_gateway_integration_test.go` - MCP/A2A flow, multi-protocol auto-detection, governance envelope verification, OOB suspension/approval, downstream integration, canonical JSON wire format
- `l2_consensus_integration_test.go` - L2 consensus idempotent enrollment, malformed CSR rejection, delegated app enrollment, quorum reached/not reached, veto by MITRE, L1-to-L5 walkthrough
- `mcp_gateway_test.go` - MCP gateway end-to-end, tools/list, tools/call, suspended transaction handling
- `mcp_gateway_config_test.go` - MCP gateway configuration validation tests
- `a2a_gateway_test.go` - A2A protocol gateway tests
- `native_tool_registry_integration_test.go` - Native tool registry integration tests
- `test/e2e/auth_e2e_test.go` - E2E authentication flow tests (build tag: `e2e`)
- `test/e2e/gateway_e2e_test.go` - E2E gateway lifecycle and health tests (build tag: `e2e`)
- `test/e2e/mcp_stdio_e2e_test.go` - E2E MCP stdio config output, JSON-RPC parsing, config template validation (build tag: `e2e`)
- `test/e2e/main_test.go` - E2E test main entry point and fixture setup (build tag: `e2e`)

## Python Protocol Package

**`protocol/python/g8e/`** - Python SDK for g8e protocol consumers (g8ee, external integrations)

- **`constants.py`**: Protocol constants loader. Loads all JSON files from `protocol/constants/` (or bundled `_data/` in PyPI installs). Exports typed dicts (`EVENTS`, `STATUS`, `COLLECTIONS`, `KV`, `CHANNELS`, `INTENTS`, `PROMPTS`, etc.) and accessor functions: `collection()`, `channel()`, `intent()`, `prompt()`, `kv_key()` (with dotted-placeholder support via regex substitution), `kv_session_type()`. Also exports `ComponentName` enum and HTTP header constants.
- **`enums.py`**: Dynamic enum generation from protocol JSON. `_build_enum()` generates `StrEnum`/`IntEnum` from `status.json` categories. `_build_enum_from_dict()` + `_EXTRA_ENUMS` registry generates enums from channels, intents, prompts, collections, kv_keys, and session_types. Access via `g8e.enums.Channel`, `g8e.enums.Intent`, etc. using `__getattr__` with `lru_cache`.
- **`models/governance.py`**: `GovernanceEnvelope` model with L1/L2/L3 governance metadata, `GovernanceL1`, `GovernanceL2Vote`, `GovernanceL2`, `GovernanceL3`, `GovernanceMetadata` sub-models. `compute_transaction_hash()` produces SHA-256 over pipe-delimited canonical fields.
- **`models/events.py`**: Event wire models including `SessionEventWire` (with `from_session_event()` factory), `BackgroundEventWire` (with `from_background_event()` factory), `TriageClarificationQuestionsPayload` (all metadata fields optional).
- **`models/internal_api.py`**: `ChatMessageRequest` with `LLMOverrides` mixin (12 override fields extracted to reusable base class).
- **`models/base.py`**: `G8eBaseModel` - Pydantic base with `extra="ignore"` and `model_dump()` excluding `None` fields.
- **`models/context.py`**: Context models for session, user, operator, and target information.
- **`models/settings.py`**: Platform settings models.
- **`_data/`**: Bundled JSON constants (populated by `make python-build` for PyPI distribution).

**Build**: `make python-build` copies `protocol/constants/*.json` to `g8e/_data/` and runs `python -m build`. Output: `protocol/python/dist/g8e-*.whl`.

**Tests**: `protocol/python/tests/` - `test_constants.py`, `test_enums.py`, `test_models.py`, `test_version.py` (151 tests). Conformance tests in `protocol/conformance/` validate `_python_const` field presence and SCREAMING_SNAKE_CASE naming across all constant files, plus model schema integrity and serialization round-trips (330 tests).

## Agent Harness & Demos

**`internal/tools/agent_harness/`** - Reference client for real governance envelope submission
- `client/client.go` - mTLS client: `StateRoot`, `RegisterSigner`, `MCPToolsCall`, `MCPToolsList`, `WaitForHumanApproval` (uses `constants.APIPaths.*`). `WaitForHumanApproval` subscribes to the gateway's SSE stream for `approval.completed` events matching the transaction hash, blocks until a human completes the WebAuthn passkey ceremony in their browser, and verifies the approval status via the mTLS status endpoint.
- `client/client_test.go` - Client unit tests (includes `TestClient_WaitForHumanApproval_Success`, `TestClient_WaitForHumanApproval_TimeoutNoMatchingEvent`, `TestClient_WaitForHumanApproval_StatusEndpointError`)
- `client/envelope.go` - `SubmitMaximal`: builds real `GovernanceEnvelope` with L1/L2/L3, submits over mTLS. `Ensemble`/`NewEnsemble`/`NewEnsembleFromSeed`/`NewEnsembleFromMemberSeeds` remain as conformance testing infrastructure.
- `client/envelope_test.go` - Envelope construction and submission tests (includes `TestSubmitMaximal`, `TestSubmitMaximal_WithL2`, ensemble tests)
- `client/audit.go` - `AuditReceipts`, `ExportReceipts`, `DiscoverOperator` (parses cert SAN for offline session discovery)
- `client/audit_test.go` - Audit client tests
- `client/protocols.go` - JSON-RPC request/response types and A2A protobuf envelope encoding for MCP/A2A protocol ingress
- `client/protocols_test.go` - Protocol encoding/decoding tests
- `client/mtls_test.go` - mTLS client setup and certificate verification tests
- `config/config.go` - Harness configuration: auth material (client cert/key/CA bundle), gateway URL, posture selection, passkey RP settings (`PasskeyRpID`, `PasskeyRpOrigin`)
- `scenarios/governance.go` - Governance scenarios: consensus, notary, delegation, OOB approval. Contains `receiptFailed` helper that parses `ActionReceipt` JSON body for `EXECUTION_STATUS_FAILED` status.
- `scenarios/governance_test.go` - Governance scenario tests, including `TestReceiptFailed` table-driven test covering FAILED, COMPLETED, UNSPECIFIED, CANCELLED, non-receipt JSON, empty body, nil body, and malformed JSON.
- `scenarios/dhs_sovereign.go` - DHS sovereign operations scenarios: multi-step governance workflow with L2 consensus
- `scenarios/dhs_sovereign_test.go` - DHS sovereign scenario tests
- `scenarios/mcp_a2a.go` - MCP and A2A protocol scenarios: plain MCP, mTLS MCP, A2A JSON, A2A mTLS, A2A protobuf
- `scenarios/mcp_a2a_test.go` - MCP/A2A protocol scenario tests
- `scenarios/gov_finance.go` - Gov/finance doctrine scenarios: `gov-cui-exfil-block` and `finance-unauthorized-trade`
- `scenarios/gov_finance_test.go` - Gov/finance scenario tests
- `scenarios/fedramp_governance.go` - FedRAMP sovereign cloud governance scenarios: `fedramp-provision` (consensus: governed cloud resource provisioning), `fedramp-deny` (doctrine: audit trail destruction blocked by L1), `fedramp-escalate` (notary: resource destruction gated on authorizing official approval), `fedramp-revert` (consensus: governed configuration revert), `fedramp-evidence-block` (doctrine: audit vault wipe rejected by L1)
- `scenarios/fedramp_governance_test.go` - FedRAMP scenario tests
- `scenarios/shell_command.go` - Shared `shellCommandArgs` helper (returns JSON string for `SubmitMaximal`) and `shellCommandMap` helper (returns `map[string]any` for `MCPToolsCall`), using typed `shellCommandJSON` struct.
- `scenarios/shell_command_test.go` - Tests for `shellCommandArgs` and `shellCommandMap`: valid JSON construction, no-args case, special character escaping, map field verification.
- `scenarios/fs_list.go` - Shared `fsListArgs` helper (returns JSON string for `SubmitMaximal`) and `fsListMap` helper (returns `map[string]any` for `MCPToolsCall`), using typed `fsListJSON` struct.
- `scenarios/fs_list_test.go` - Tests for `fsListArgs` and `fsListMap`: valid JSON construction, special character escaping, map field verification.
- `scenarios/scenario.go` - Scenario registry, `Execute`, `Posture` types
- `scenarios/scenario_test.go` - Scenario registry and execution tests

**`demos/dhs/`** - DHS sovereign data operations demo
- `datasvc.py` - Mock data service HTTP server for sovereign data access
- `dataop.sh` - Demo artifact for data operations invocation
- `verify_ops.py` - Demo artifact for verifying data operation results

**`demos/fedramp/`** - FedRAMP sovereign cloud governance demo
- `cloudsvc.py` - Sovereign Cloud Service HTTP server (L5 actuator, port 9100) for governed cloud resource operations
- `cloudop.sh` - Demo artifact for cloud operations invocation (operator execution bridge)
- `verify_ops.py` - Demo artifact for verifying cloud service operation results

**`demos/healthcare/`** - Healthcare analytics demo with Metabase integration
- `pa_api_server.py` - Prior authorization API server mock
- `setup_metabase.py` - Metabase initialization and dashboard setup
- `init.sql` - Database schema initialization for healthcare data

**`demos/frontend/`** - Third-party frontend enrollment demo
- `app/` - Frontend application source
- `compose.yml` - Docker Compose configuration for frontend demo environment

**`demos/finance/`** - Financial data governance demo

**`demos/gov/`** - Government operations demo

**CLI Demo Scenario Files** (`internal/cli/cmd/`):
- `demos.go` - Demo CLI command tree (list, start, stop, status, clean, rebuild, reset, run, pull, export, import, images, scenarios). Contains `harnessConfig` struct (connection params + `UseRun`), `defaultHarnessConfig`, `defaultGovernedHarnessConfig` (wraps `defaultHarnessConfig` with governance posture overrides), `harnessRun` helper for building docker compose exec/run commands for demo scenarios, `switchDemoPosture` (shared posture switch helper), and `runTwoLayerScenario` reusable orchestrator. `demoVerbose` flag (set by `-v`/`--verbose`), `demoStep` suppresses output when non-verbose, `demoPrintln`/`demoPrintf` (verbose-aware print helpers), `scenarioCounts` map (healthcare: 4, gov: 1, finance: 1, dhs: 5, fedramp: 5, frontend: 1), `printDemoEndpoints` (prints available endpoints per org).
- `demo_gov.go` - Gov demo scenario (uses `runTwoLayerScenario` with `harnessRun`)
- `demo_finance.go` - Finance demo scenario (uses `runTwoLayerScenario` with `harnessRun`)
- `demo_healthcare.go` - Healthcare demo scenarios (4 scenarios, each calls `harnessRun`)
- `demo_dhs.go` - DHS demo scenarios (5 scenarios, each calls `dhsHarnessRun` → `harnessRun`). Contains `dhsHarnessConfig`, `dhsHarnessRun`, `dhsScenarioStep`, `extractFirstTxHash`, `switchDHSPosture` helpers.
- `demo_fedramp.go` - FedRAMP demo scenarios (5 scenarios, each calls `harnessRun`). Contains `defaultFedRAMPHarnessConfig`, `fedrampHarnessRun`, `switchFedRAMPPosture` helpers (delegates to shared `switchDemoPosture`).
- `demo_frontend.go` - Frontend demo scenario (1 scenario: third-party frontend enrollment via `runFrontendScenario`).
- `scenarios_run.go` - `demos scenarios run` subcommand and `runAgentHarness` execution logic. Contains flag definitions, `applyAgentHarnessFlags`, `selectAgentHarnessScenarios`, `needsGovKit`, `setupGovKit` (passes `GovKit` with `{OperatorID, OperatorSessionID, UserID}` for human browser approval via `WaitForHumanApproval`), `printAgentHarnessSummary`, `failedScenariosError` helper (collects failed scenario names and returns formatted error when any `Result.OK` is false).
- `scenarios_run_test.go` - Tests for `failedScenariosError` helper: all-OK (nil), all-failed (error with all names), mixed (error with only failed names), empty results (nil), single failed (error).
- `demos_test.go` - Tests for demo CLI commands, `scenarioCounts`, `printDemoEndpoints`, `harnessRun`/`defaultHarnessConfig` unit tests, `defaultHarnessConfig`/`harnessRun` unit tests, and source-file assertions (`TestDemoScenarioFilesCallHarnessRun`, `TestNoGatewayBypassInDemoFiles`, `TestNoSqliteBackdoorInScenarioFiles`, `TestNoCopyPasteInScenarioFiles`). Also tests `TestDemoPrintln`, `TestDemosPullCmd`, `TestCheckDockerAvailable`, `TestToDockerPath`, `TestDefaultHarnessConfig`, `TestHarnessRun`.
- `demos_helpers_test.go` - Shared test helpers for demo command tests.
- `demos_integration_test.go` - Integration tests for demo CLI commands.
- `demos_docker_error_paths_test.go` - Docker error path tests for demo commands.
- `demos_run_error_paths_test.go` - Run error path tests for demo scenarios.
- `demo_dhs_test.go` - Tests for DHS demo scenario helpers.

## FIPS 140-3 Build Infrastructure

- **`Dockerfile.fips`**: FIPS 140-3 compliant Docker build. Builder stage sets `ENV GOFIPS140=v1.0.0` and `CGO_ENABLED=0`, builds the binary for `linux/amd64`, and runs `g8e version --fips` as a build-time self-check. Runtime stage uses pinned `debian:12-slim` (vendor-affirmed OE per CMVP Cert #5247 Table 3). The binary enters FIPS approved mode by default; no runtime env var is required.
- **`Makefile` `build-fips` target**: Builds `bin/g8e-fips-linux-amd64` with `GOFIPS140=v1.0.0`, `CGO_ENABLED=0`, `GOOS=linux`, `GOARCH=amd64`. Pins to the certified, frozen module version (not `certified` which floats).
- **`Makefile` `verify-fips` target**: Depends on `build-fips`, then runs `./g8e-fips version --fips` to confirm FIPS approved mode is active. Exits non-zero if the self-check fails.
- **`.github/workflows/build-and-test-fips.yml`**: CI workflow that builds with `GOFIPS140=v1.0.0`, runs `make verify-fips`, executes the test suite with the FIPS module linked, and verifies static linking.
- **`demos/fedramp/compose.fips.yml`**: FIPS-mode variant of the FedRAMP demo. All three g8e services (gateway, operator, agent-runtime) build from `Dockerfile.fips`. Uses separate container names (`g8e-fedramp-fips-*`), separate named volumes (`fedramp-fips-*`), and different host ports (8089/8452) to avoid conflicts with the standard demo.

See [FIPS 140-3 Compliance](../reference/fips140-3.md) for the validated boundary, OE matrix, and build/runtime activation details.
