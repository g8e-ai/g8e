# g8e Codemap — Service Dependency Tree

## Top-Level Service Roots

```text
G8eoService (Outbound/Operator Mode)
├── auth.BootstrapService
│   └── *external HTTP auth endpoint*
├── gateway.SecretManager
│   └── gateway.GatewayDBService (for keystore DB access)
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
│   │   ├── governance.StateRootProvider (gateway.GatewayDBService)
│   │   ├── governance.SignerStore
│   │   ├── governance.AppPolicyStore (gateway.GatewayDBService)
│   │   └── governance.L3Notary
│   │       └── gateway.CompositeL3Verifier
│   │           ├── gateway.PasskeyService
│   │           └── gateway.CLIL3Notary
│   │               ├── gateway.GatewayDBService
│   │               ├── gateway.PKIAuthority
│   │               ├── gateway.UserService
│   │               └── gateway.SessionsService
│   ├── governance.L5Actuator
│   │   ├── execution.ExecutionService
│   │   ├── storage.AuditVaultService
│   │   ├── storage.AuditStore (auditVaultTransactionStore wrapper)
│   │   ├── scrubbing.ScrubbingService
│   │   ├── governance.StateRootProvider
│   │   └── governance.SignerStore
│   └── mcp.GatewayService
│       ├── responder.Responder
│       └── gateway.GatewayDBService (as SuspendedTransactionStore)
├── pubsub.PubSubResultsService
│   └── storage.LocalStoreService
├── storage.LocalStoreService
│   ├── sqliteutil.DB
│   ├── vault.Vault (optional encryption)
│   └── scrubbing.ScrubbingService (wired post-init to break cycle)
├── storage.AuditVaultService
│   ├── sqliteutil.DB
│   ├── vault.Vault (optional encryption)
│   └── go-git (native, for ledger repos)
├── storage.LedgerService
│   ├── storage.AuditVaultService
│   └── vault.Vault
├── storage.HistoryHandler
│   ├── storage.AuditVaultService
│   └── storage.LedgerService
├── governance.ReplayStore (storage.SQLReplayStore)
│   └── storage.LocalStoreService (shared DB)
└── scrubbing.ScrubbingService
    └── storage.LocalStoreService (as TokenStore)

GatewayService (Gateway/Platform Mode)
├── gateway.GatewayDBService
│   ├── sqliteutil.DB
│   ├── storage.AuditVaultService
│   └── keystore (embedded in schema)
├── gateway.PubSubBroker
├── gateway.AuthService
│   ├── gateway.GatewayDBService
│   ├── gateway.PKIAuthority
│   ├── gateway.UserService
│   ├── gateway.PersonaService
│   └── responder.Responder
├── gateway.PKIAuthority
│   ├── gateway.GatewayDBService
│   └── gateway.SecretManager
├── gateway.RegistrationService
│   ├── gateway.GatewayDBService
│   ├── gateway.PKIAuthority
│   ├── gateway.UserService
│   └── gateway.SessionsService
├── gateway.PasskeyService
│   └── gateway.GatewayDBService
├── gateway.UserService
│   └── gateway.GatewayDBService
├── gateway.SessionsService
│   └── gateway.GatewayDBService
├── gateway.AppEnrollmentService
│   ├── gateway.GatewayDBService
│   └── gateway.PKIAuthority
├── mcp.GatewayService
│   ├── responder.Responder
│   └── gateway.GatewayDBService (as SuspendedTransactionStore)
└── responder.Responder
```

## Structural Observations

### Data Handling Convergence
- **`gateway.GatewayDBService`** is the canonical SQLite root for gateway mode; it also embeds `storage.AuditVaultService`.
- **`storage.LocalStoreService`** is the consolidated execution vault for outbound mode.
- **`storage.AuditVaultService`** is shared by both modes and provides the git-backed ledger foundation.

### Circular Dependency Break
- `scrubbing.ScrubbingService` depends on `storage.LocalStoreService` (as `TokenStore`).
- `storage.LocalStoreService` receives its `TextScrubber` via `SetScrubber()` post-initialization to break the cycle (`@/home/bob/g8e/internal/services/g8eo.go:297`).

### Governance Stack (L1-L5)
- **L1**: `governance.L1Doctrine` (technical bedrock validation)
- **L2**: `governance.L2Consensus` (signing + quorum)
- **L3**: `governance.L3Notary` / `gateway.CompositeL3Verifier` (WebAuthn + mTLS CLI)
- **L4**: `governance.L4Warden` (transaction verifier / fail-closed gate)
- **L5**: `governance.L5Actuator` (execution boundary, receipt signing)

### Shared Interface Implementations
- `gateway.GatewayDBService` implements: `governance.ReplayStore`, `governance.StateRootProvider`, `governance.TransactionAuditStore`, `governance.SignerStore`, `governance.AppPolicyStore`, `governance.SuspendedTransactionStore`.
- `storage.LocalStoreService` implements: `interfaces.TokenStore`, `governance.SuspendedTransactionStore`.

### Transport & Protocol Layer
- `pubsub.PubSubCommandService` is the dispatcher for outbound mode (WebSocket pub/sub).
- `mcp.GatewayService` handles MCP/A2A protocol translation and downstream dispatch.
- `gateway.HTTPHandler` builds the HTTP/WebSocket surface for gateway mode.

## Critical Data Flows

| Flow | Path |
|------|------|
| Command execution results | `ExecutionService` → `CommandService` → `PubSubResultsService` → Pub/Sub channel |
| Audit events | `CommandService` / `FileOpsService` → `AuditVaultService` → SQLite + git ledger |
| File mutations | `FileEditService` → `LedgerService` → `AuditVaultService` git commit |
| Suspended transactions | `L4Warden` → `LocalStoreService` (outbound) or `GatewayDBService` (gateway) |
| Action receipts | `L5Actuator` → `AuditVaultService` (receipts table) + signed return |
