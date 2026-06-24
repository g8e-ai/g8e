# g8e Codemap — Service Dependency Tree

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
│   ├── governance.L1Doctrine
│   ├── governance.L4Warden
│   │   ├── governance.ReplayStore (storage.SQLReplayStore)
│   │   ├── governance.StateRootProvider (gateway.CanonicalDBService) [SHARED]
│   │   ├── governance.SignerStore (governance.FilesystemSignerStore)
│   │   ├── governance.AppPolicyStore (gateway.CanonicalDBService) [SHARED]
│   │   ├── governance.TribunalStore (nil in outbound mode, gateway.TribunalStoreService in gateway mode)
│   │   └── governance.L3Notary (governance.outboundL3Notary implementation)
│   │       └── storage.SuspendedTransactionService
│   ├── governance.L5Actuator
│   │   ├── execution.ExecutionService
│   │   ├── storage.SQLAuditStore
│   │   ├── governance.TransactionAuditStore (auditStoreTransactionStore wrapper)
│   │   ├── governance.L3Notary
│   │   ├── scrubbing.ScrubbingService
│   │   ├── governance.StateRootProvider
│   │   ├── governance.SignerStore
│   │   └── governance.GovernancePosture
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
│   ├── gateway.BlobStoreService (extracted field)
│   └── keystore (embedded in schema, via keystore.Keystore)
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
│   └── gateway.SecretManager
├── gateway.SecretManager
│   ├── sqliteutil.DB
│   └── keystore.Keystore (via gateway.CanonicalDBService)
├── gateway.RegistrationService
│   ├── gateway.CanonicalDBService [SHARED]
│   ├── gateway.PKIAuthority
│   ├── gateway.UserService
│   ├── gateway.CLISessionService
│   └── gateway.OperatorSessionService
├── gateway.PasskeyService
│   └── gateway.CanonicalDBService [SHARED]
├── gateway.UserService
│   └── gateway.CanonicalDBService [SHARED]
├── gateway.PersonaService
│   └── gateway.CanonicalDBService [SHARED]
├── gateway.CLISessionService
│   └── gateway.CanonicalDBService [SHARED]
├── gateway.OperatorSessionService
│   └── gateway.CanonicalDBService [SHARED]
├── gateway.WebSessionService
│   └── gateway.CanonicalDBService [SHARED]
├── gateway.AppEnrollmentService
│   ├── gateway.CanonicalDBService [SHARED]
│   └── gateway.PKIAuthority
├── gateway.SignerStoreService (implements governance.SignerStore)
│   └── sqliteutil.DB
├── gateway.TribunalStoreService (implements governance.TribunalStore)
│   └── sqliteutil.DB
├── gateway.AppPolicyStoreService (implements governance.AppPolicyStore)
│   └── sqliteutil.DB
├── gateway.ReplayStoreService (implements governance.ReplayStore)
│   └── sqliteutil.DB
├── gateway.DocumentStoreService (implements governance.TransactionAuditStore)
│   └── sqliteutil.DB
├── gateway.StateRootService (implements governance.StateRootProvider)
│   └── sqliteutil.DB
├── gateway.KVStoreService (TTL-aware ephemeral state)
│   └── sqliteutil.DB
├── gateway.SSEEventService (Server-Sent Events)
│   └── sqliteutil.DB
├── gateway.BlobStoreService (Binary persistence)
│   └── sqliteutil.DB
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
│   ├── mcp.GatewayService [SHARED]
│   ├── tribunal.TribunalService [SHARED]
│   ├── storage.SuspendedTransactionService (as storage.SuspendedTransactionStore) [SHARED]
│   ├── governance.EnvelopeProcessor (set post-construction by boot sequence)
│   └── response.Writer
├── gateway.CompositeL3Verifier (implements governance.L3Notary)
│   ├── gateway.PasskeyService
│   └── gateway.CLIL3Notary
│       ├── gateway.CanonicalDBService [SHARED]
│       ├── gateway.PKIAuthority
│       ├── gateway.UserService
│       └── gateway.CLISessionService
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
│   ├── mcp.SessionValidator (OperatorPubSubService in outbound mode)
│   ├── mcp.AuditLogger (pubsubAuditLogger in outbound mode)
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
- **L2**: `tribunal.TribunalService` (Tribunal-based deliberation producing L2 votes via Ed25519 signatures; gateway delegates deliberation via `LocalDeliberator` or `HTTPTribunalDeliberator`). The `TribunalStore` interface in `governance.L4Warden` loads `TribunalPolicy` for quorum verification.
- **L3**: `governance.L3Notary` (gateway mode uses `gateway.CompositeL3Verifier` combining WebAuthn passkey and mTLS CLI proofs; outbound mode uses `governance.outboundL3Notary` for CLI-based approval via suspended transactions)
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
- `governance.outboundL3Notary` implements: `governance.L3Notary` (used in outbound mode).
- `gateway.CompositeL3Verifier` implements: `governance.L3Notary` (used in gateway mode, delegates to `PasskeyService` for WebAuthn proofs and `CLIL3Notary` for mTLS proofs).

### Transport & Protocol Layer
- `pubsub.OperatorPubSubService` is the dispatcher for outbound mode (WebSocket pub/sub).
- `mcp.GatewayService` handles MCP/A2A protocol translation and downstream dispatch (shared between modes).
- `gateway.HTTPHandler` builds the HTTP/WebSocket surface for gateway mode.
- `gateway.GatewayWebSocketHandler` is the in-process pub/sub broker for gateway mode.
- `gateway.PKIAuthority` manages PKI hierarchy and certificate lifecycle for gateway mode.

## Critical Data Flows

| Flow | Path |
|------|------|
| Command execution results | `ExecutionService` → `CommandService` → `PubSubResultsService` → Pub/Sub channel |
| Audit events | `CommandService` / `FileOpsService` → `SQLAuditStore` → SQLite |
| File mutations | `FileEditService` → `GitLedgerService` → git commit |
| Suspended transactions | `L4Warden` → `storage.SuspendedTransactionService` (consistent in both gateway and outbound modes) |
| Action receipts | `L5Actuator` → `SQLAuditStore` (receipts table) + signed return |

## Test Infrastructure (Not Production)

The following packages are test-only and are not part of the production dependency tree:

**`internal/services/storage/storagetest/`** - Test-only audit storage implementations
- `TestSQLAuditStore` - Test-only monolithic audit service with Git ledger integration
- Used only in test code (e.g., chaos tester at `test/chaos/chaos.go`)
- Implements `TransactionAuditStore` interface via a no-op `DocSet` method
- Production code uses `storage.SQLAuditStore` from `audit_store.go`

**`test/chaos/`** - Chaos engineering test infrastructure
- Chaos tester uses `storagetest.TestSQLAuditStore` for audit storage
- This is intentional test infrastructure, not production code
- Located in `test/` to clearly indicate test-only status

**Key distinction**: Test infrastructure is separated from production code to avoid import cycles. The `storagetest` package provides test implementations that should never be used in production code paths.
