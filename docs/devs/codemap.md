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
│   │   ├── governance.SignerStore
│   │   ├── governance.AppPolicyStore (gateway.CanonicalDBService) [SHARED]
│   │   └── governance.L3Notary
│   │       └── gateway.CompositeL3Verifier
│   │           ├── gateway.PasskeyService
│   │           └── gateway.CLIL3Notary
│   │               ├── gateway.CanonicalDBService [SHARED]
│   │               ├── gateway.PKIAuthority
│   │               ├── gateway.UserService
│   │               └── gateway.CLISessionService
│   ├── governance.L5Actuator
│   │   ├── execution.ExecutionService
│   │   ├── storage.SQLAuditStore
│   │   ├── storage.AuditStore (auditStoreTransactionStore wrapper)
│   │   ├── scrubbing.ScrubbingService
│   │   ├── governance.StateRootProvider
│   │   └── governance.SignerStore
│   └── mcp.GatewayService [SHARED]
│       ├── response.Writer
│       └── gateway.CanonicalDBService (as SuspendedTransactionStore) [SHARED]
├── pubsub.PubSubResultsService
│   └── storage.LocalStoreService
├── storage.LocalStoreService
│   ├── sqliteutil.DB
│   └── vault.Vault
├── storage.SQLAuditStore
│   ├── sqliteutil.DB
│   └── vault.Vault
├── storage.GitLedgerService
│   └── vault.Vault
├── storage.HistoryHandler
│   ├── storage.SQLAuditStore
│   └── storage.GitLedgerService
├── governance.ReplayStore (storage.SQLReplayStore)
│   └── storage.LocalStoreService (shared DB)
└── scrubbing.ScrubbingService
    └── storage.LocalStoreService (as TokenStore)

GatewayModeService (Gateway/Platform Mode) [MODE-SPECIFIC]
├── gateway.CanonicalDBService [SHARED]
│   ├── sqliteutil.DB
│   ├── storage.SQLAuditStore
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
├── gateway.CLISessionService
│   └── gateway.CanonicalDBService [SHARED]
├── gateway.OperatorSessionService
│   └── gateway.CanonicalDBService [SHARED]
├── gateway.WebSessionService
│   └── gateway.CanonicalDBService [SHARED]
├── gateway.AppEnrollmentService
│   ├── gateway.CanonicalDBService [SHARED]
│   └── gateway.PKIAuthority
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
- **`gateway.CanonicalDBService`** is the canonical SQLite root for gateway mode; it also embeds `storage.SQLAuditStore`. In outbound mode, it is used only for state root calculation.
- **`storage.LocalStoreService`** is the consolidated execution vault for outbound mode.
- **`storage.SQLAuditStore`** is shared by both modes and provides the SQL-based audit storage foundation.

### Dependency Flow
- `scrubbing.ScrubbingService` depends on `storage.LocalStoreService` (as `TokenStore`).
- `storage.LocalStoreService` has no dependency on `scrubbing.ScrubbingService` (circular dependency removed).

### Governance Stack (L1-L5)
- **L1**: `governance.L1Doctrine` (technical bedrock validation)
- **L2**: `governance.L2Consensus` (signing + quorum)
- **L3**: `governance.L3Notary` / `gateway.CompositeL3Verifier` (WebAuthn + mTLS CLI)
- **L4**: `governance.L4Warden` (transaction verifier / fail-closed gate)
- **L5**: `governance.L5Actuator` (execution boundary, receipt signing)

### Shared Interface Implementations
- `gateway.CanonicalDBService` implements: `governance.ReplayStore`, `governance.StateRootProvider`, `governance.TransactionAuditStore`, `governance.SignerStore`, `governance.AppPolicyStore`, `governance.SuspendedTransactionStore`.
- `storage.LocalStoreService` implements: `interfaces.TokenStore`, `governance.SuspendedTransactionStore`.

### Transport & Protocol Layer
- `pubsub.PubSubCommandService` is the dispatcher for outbound mode (WebSocket pub/sub).
- `mcp.GatewayService` handles MCP/A2A protocol translation and downstream dispatch.
- `gateway.HTTPHandler` builds the HTTP/WebSocket surface for gateway mode.

## Critical Data Flows

| Flow | Path |
|------|------|
| Command execution results | `ExecutionService` → `CommandService` → `PubSubResultsService` → Pub/Sub channel |
| Audit events | `CommandService` / `FileOpsService` → `SQLAuditStore` → SQLite |
| File mutations | `FileEditService` → `LedgerService` → git commit |
| Suspended transactions | `L4Warden` → `LocalStoreService` (outbound) or `CanonicalDBService` (gateway) |
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
