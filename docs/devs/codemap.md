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
├── pubsub.PubSubCommandService
│   ├── pubsub.HeartbeatService
│   ├── pubsub.CommandService
│   │   └── execution.ExecutionService
│   ├── pubsub.FileOpsService
│   │   └── execution.FileEditService
│   ├── pubsub.PortService
│   ├── pubsub.AuditService
│   ├── pubsub.HistoryService
│   ├── governance.L2Consensus
│   │   └── governance.L1Doctrine
│   ├── governance.L4Warden
│   │   ├── governance.ReplayStore (storage.SQLReplayStore)
│   │   ├── governance.StateRootProvider (gateway.CanonicalDBService) [SHARED]
│   │   ├── governance.SignerStore (governance.FilesystemSignerStore)
│   │   ├── governance.AppPolicyStore (gateway.CanonicalDBService) [SHARED]
│   │   └── governance.L3Notary (governance.outboundL3Notary implementation)
│   │       └── storage.SuspendedTransactionService
│   ├── governance.L5Actuator
│   │   ├── execution.ExecutionService
│   │   ├── storage.SQLAuditStore
│   │   ├── governance.TransactionAuditStore (auditStoreTransactionStore wrapper)
│   │   ├── scrubbing.ScrubbingService
│   │   ├── governance.StateRootProvider
│   │   └── governance.SignerStore
│   └── mcp.GatewayService [SHARED]
│       ├── response.Writer
│       └── gateway.CanonicalDBService (as SuspendedTransactionStore) [SHARED]
├── pubsub.PubSubResultsService
├── storage.ExecutionVaultService
│   ├── sqliteutil.DB
│   └── vault.Vault (shared with CanonicalDBService)
├── storage.TokenStoreService
│   ├── sqliteutil.DB
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
└── scrubbing.ScrubbingService
    └── storage.TokenStoreService

GatewayModeService (Gateway/Platform Mode) [MODE-SPECIFIC]
├── gateway.CanonicalDBService [SHARED]
│   ├── sqliteutil.DB
│   ├── storage.SQLAuditStore
│   ├── vault.Vault
│   └── keystore (embedded in schema)
├── gateway.PubSubBroker
├── gateway.AuthService
│   ├── gateway.CanonicalDBService [SHARED]
│   ├── gateway.PKIAuthority
│   ├── gateway.UserService
│   ├── gateway.PersonaService
│   └── response.Writer
├── gateway.PKIAuthority
│   ├── gateway.CanonicalDBService [SHARED]
│   └── gateway.SecretManager
├── gateway.SecretManager
│   ├── sqliteutil.DB
│   └── keystore.Keystore
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
├── gateway.SignerStoreService
│   ├── gateway.DocumentStoreService
│   └── sqliteutil.DB
├── gateway.AppPolicyStoreService
│   ├── gateway.DocumentStoreService
│   └── sqliteutil.DB
├── gateway.ReplayStoreService
│   └── sqliteutil.DB
├── gateway.DocumentStoreService
│   └── sqliteutil.DB
├── gateway.StateRootService
│   └── sqliteutil.DB
├── gateway.InvitationService
│   └── gateway.CanonicalDBService [SHARED]
├── gateway.HTTPHandler
│   ├── gateway.CanonicalDBService [SHARED]
│   ├── gateway.PubSubBroker
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
│   └── response.Writer
├── mcp.GatewayService [SHARED]
│   ├── response.Writer
│   └── gateway.CanonicalDBService (as SuspendedTransactionStore) [SHARED]
└── response.Writer
```

## Structural Observations

### Mode Bifurcation
- **Mode-specific services**: `G8eoService` (outbound mode only), `GatewayModeService` (gateway mode only)
- **Shared services**: `mcp.GatewayService` (used in both modes for MCP/A2A protocol handling), `CanonicalDBService` (used in both modes for state root calculation - full service in gateway mode, state root calculation only in outbound mode)

### Data Handling Convergence
- **`gateway.CanonicalDBService`** is the canonical SQLite root for gateway mode; it embeds `storage.SQLAuditStore` and `vault.Vault`. In outbound mode, it is used only for state root calculation and provides the shared vault instance.
- **`gateway.DocumentStoreService`** provides collection/ID-based document CRUD operations for gateway mode (delegates to CanonicalDBService).
- **`gateway.StateRootService`** provides state merkle root calculation with caching for gateway mode (delegates to CanonicalDBService).
- **`gateway.SignerStoreService`** provides trusted signer CRUD operations for gateway mode (implements governance.SignerStore).
- **`gateway.AppPolicyStoreService`** provides app policy retrieval for gateway mode (implements governance.AppPolicyStore).
- **`gateway.ReplayStoreService`** provides nonce replay protection for gateway mode (implements governance.ReplayStore).
- **`gateway.InvitationService`** handles user invitations for gateway mode.
- **`storage.ExecutionVaultService`** is the execution log and file diff storage for outbound mode.
- **`storage.TokenStoreService`** is the Sentinel token persistence store for outbound mode.
- **`storage.SuspendedTransactionService`** is the L3 approval workflow store for outbound mode.
- **`storage.SQLAuditStore`** is shared by both modes and provides the SQL-based audit storage foundation.
- **`vault.Vault`** is shared across all storage services in outbound mode (reused from CanonicalDBService).

### Dependency Flow
- `scrubbing.ScrubbingService` depends on `storage.TokenStoreService` (as `TokenStore`).
- `storage.TokenStoreService` has no dependency on `scrubbing.ScrubbingService` (circular dependency removed).
- All outbound storage services (ExecutionVaultService, TokenStoreService, SQLAuditStore, GitLedgerService) share the same `vault.Vault` instance from CanonicalDBService.

### Governance Stack (L1-L5)
- **L1**: `governance.L1Doctrine` (technical bedrock validation)
- **L2**: `governance.L2Consensus` (signing + quorum)
- **L3**: `governance.L3Notary` (gateway mode uses `gateway.CompositeL3Verifier` for WebAuthn + mTLS CLI; outbound mode uses `governance.outboundL3Notary` for CLI-based approval via suspended transactions)
- **L4**: `governance.L4Warden` (transaction verifier / fail-closed gate)
- **L5**: `governance.L5Actuator` (execution boundary, receipt signing)

### Shared Interface Implementations
- `gateway.CanonicalDBService` implements: `governance.ReplayStore`, `governance.StateRootProvider`, `governance.TransactionAuditStore`, `governance.SignerStore`, `governance.AppPolicyStore`, `governance.SuspendedTransactionStore`.
- `gateway.SignerStoreService` implements: `governance.SignerStore` (gateway mode dedicated implementation).
- `gateway.AppPolicyStoreService` implements: `governance.AppPolicyStore` (gateway mode dedicated implementation).
- `gateway.ReplayStoreService` implements: `governance.ReplayStore` (gateway mode dedicated implementation).
- `gateway.StateRootService` implements: `governance.StateRootProvider` (gateway mode dedicated implementation).
- `storage.TokenStoreService` implements: `interfaces.TokenStore`.
- `storage.SuspendedTransactionService` implements: `governance.SuspendedTransactionStore`.
- `governance.FilesystemSignerStore` implements: `governance.SignerStore` (used in outbound mode).
- `governance.outboundL3Notary` implements: `governance.L3Notary` (used in outbound mode).

### Transport & Protocol Layer
- `pubsub.PubSubCommandService` is the dispatcher for outbound mode (WebSocket pub/sub).
- `mcp.GatewayService` handles MCP/A2A protocol translation and downstream dispatch (shared between modes).
- `gateway.HTTPHandler` builds the HTTP/WebSocket surface for gateway mode.
- `gateway.PubSubBroker` is the in-process pub/sub broker for gateway mode.

## Critical Data Flows

| Flow | Path |
|------|------|
| Command execution results | `ExecutionService` → `CommandService` → `PubSubResultsService` → Pub/Sub channel |
| Audit events | `CommandService` / `FileOpsService` → `SQLAuditStore` → SQLite |
| File mutations | `FileEditService` → `GitLedgerService` → git commit |
| Suspended transactions | `L4Warden` → `SuspendedTransactionService` (outbound) or `CanonicalDBService` (gateway) |
| Action receipts | `L5Actuator` → `SQLAuditStore` (receipts table) + signed return |

## Test Infrastructure (Not Production)

The following packages are test-only and are not part of the production dependency tree:

**`internal/services/storage/storagetest/`** - Test-only audit storage implementations
- `TestSQLAuditStore` - Test-only monolithic audit service with Git ledger integration
- Used only in test code (e.g., chaos tester at `internal/test/chaos/chaos.go`)
- Implements `TransactionAuditStore` interface via a no-op `DocSet` method
- Production code uses `storage.SQLAuditStore` from `audit_store.go`

**`internal/test/chaos/`** - Chaos engineering test infrastructure
- Chaos tester uses `storagetest.TestSQLAuditStore` for audit storage
- This is intentional test infrastructure, not production code
- Located in `internal/test/` to clearly indicate test-only status

**Key distinction**: Test infrastructure is separated from production code to avoid import cycles. The `storagetest` package provides test implementations that should never be used in production code paths.
