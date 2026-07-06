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
│   │   ├── governance.TribunalStore (via GovernanceDeps; nil in outbound mode, gateway.TribunalStoreService in gateway mode)
│   │   ├── governance.L1Doctrine (created internally by L4Warden when doctrine param is nil)
│   │   └── governance.L3Notary (governance.outboundNotary implementation)
│   │       └── storage.SuspendedTransactionService
│   ├── governance.L5Actuator
│   │   ├── execution.ExecutionService
│   │   ├── storage.SQLAuditStore (from CanonicalDBService.AuditStore) [SHARED]
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
│   ├── gateway.KVStoreService (from CanonicalDBService) [SHARED]
│   └── vault.Vault (shared with CanonicalDBService)
├── storage.SuspendedTransactionService
│   └── sqliteutil.DB
├── storage.GitLedgerService
│   └── vault.Vault (shared with CanonicalDBService)
├── storage.HistoryHandler
│   ├── storage.SQLAuditStore (from CanonicalDBService.AuditStore) [SHARED]
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
│   │   └── gateway.CanonicalDBService [SHARED]
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
├── gateway.PasskeyService (domain logic only)
│   └── gateway.CanonicalDBService [SHARED] (via dbUserStore and dbSessionStore wrappers)
├── gateway.PasskeyHandler (HTTP layer, embeds *PasskeyService)
│   ├── gateway.PasskeyService [SHARED]
│   ├── gateway.WebSessionService (for session creation on browser flows)
│   ├── response.Writer (for HTTP responses)
│   ├── gateway.MCPServiceProvider (via PasskeyHandlerDeps constructor)
│   ├── storage.SuspendedTransactionStore (via PasskeyHandlerDeps constructor)
│   ├── gateway.SSEEventService (via PasskeyHandlerDeps constructor)
│   └── gateway.GatewayWebSocketHandler (via PasskeyHandlerDeps constructor)
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
│   ├── gateway.PasskeyHandler (includes approval handlers via passkey_service_approvals.go)
│   │   ├── gateway.MCPServiceProvider (via PasskeyHandlerDeps constructor)
│   │   ├── storage.SuspendedTransactionStore (via PasskeyHandlerDeps constructor)
│   │   ├── gateway.SSEEventService (via PasskeyHandlerDeps constructor)
│   │   └── gateway.GatewayWebSocketHandler (via PasskeyHandlerDeps constructor)
│   ├── gateway.UserService
│   ├── gateway.AppEnrollmentService
│   │   ├── gateway.CanonicalDBService [SHARED]
│   │   └── gateway.PKIAuthority
│   ├── gateway/console (Console SPA embed filesystem)
│   ├── mcp.GatewayService [SHARED]
│   ├── tribunal.TribunalService (atomic.Pointer, set by boot sequence via SetTribunal — no router rebuild)
│   ├── governance.EnvelopeProcessor (atomic.Pointer, set by boot sequence via SetEnvelopeProcessor)
│   ├── gateway.PKIController (PKI enrollment, CSR signing, trust scripts, deploy scripts)
│   ├── gateway.DBController (audit receipts, audit events, data DB, KV, blobs, governance signers, pub/sub)
│   ├── gateway.AuthController (bootstrap, CLI enrollment, device enrollment, user management, web session)
│   │   ├── gateway.AuthService [SHARED]
│   │   ├── gateway.PasskeyHandler [SHARED]
│   │   ├── gateway.UserService [SHARED]
│   │   ├── gateway.RegistrationService [SHARED]
│   │   ├── gateway.PKIAuthority [SHARED]
│   │   ├── gateway.WebSessionService [SHARED]
│   │   ├── gateway.CLISessionService [SHARED]
│   │   ├── gateway.OperatorSessionService [SHARED]
│   │   ├── gateway.CanonicalDBService [SHARED]
│   │   ├── response.Writer
│   │   └── actuatorKeyReader (fileActuatorKeyReader, reads actuator public key from disk)
│   ├── gateway.AdminController (app policies, tribunals, app revocation)
│   ├── gateway.OperatorController (operator list, terminate, bind/unbind, target context, reauth)
│   └── response.Writer
├── http.Server (HTTPS port, mTLS-enforced public router)
├── http.Server (HTTP port, bootstrap-only router)
├── governance.gatewayNotary (via governance.NewGatewayL3Notary, implements governance.L3Notary)
│   ├── gateway.cliSessionVerifier (via NewCLISessionVerifier, implements governance.CLISessionVerifier)
│   │   ├── gateway.CanonicalDBService [SHARED]
│   │   ├── gateway.PKIAuthority
│   │   ├── gateway.UserService
│   │   └── gateway.CLISessionService
│   └── gateway.PasskeyService (as governance.L3Notary for WebAuthn proofs, domain logic only)
│       └── gateway.CanonicalDBService [SHARED]
├── tribunal.TribunalService
│   ├── governance.L1Doctrine
│   ├── tribunal.TribunalMember (one or more enrolled members with Ed25519 keys)
│   └── response.Writer
├── mcp.GatewayService [SHARED]
│   ├── response.Writer
│   ├── storage.SuspendedTransactionService (as storage.SuspendedTransactionStore) [SHARED]
│   ├── scrubbing.ScrubbingService
│   ├── mcp.FieldPathRegistry
│   ├── mcp.NativeToolHandler
│   ├── mcp.FieldReader (gateway.DocumentStoreService) [SHARED]
│   ├── RuntimeDependencies (atomic.Pointer, set once via SetRuntimeDeps before first request):
│   │   ├── mcp.SessionValidator (set by in-process OperatorPubSubService in both modes)
│   │   ├── mcp.AuditLogger (pubsubAuditLogger, set by in-process OperatorPubSubService in both modes)
│   │   ├── governance.EnvelopeProcessor (set by in-process OperatorPubSubService in both modes)
│   │   ├── StateRootProvider (set by in-process OperatorPubSubService in both modes)
│   │   ├── Ed25519 signing key/keyID (set by in-process OperatorPubSubService in both modes)
│   │   └── downstreamURL (MCP egress, set by in-process OperatorPubSubService in both modes)
│   ├── tribunal.TribunalDeliberator (atomic.Value, tribunal.LocalDeliberator in gateway mode, nil in outbound)
│   ├── a2aDownstreamURL (construction-phase, immutable after NewGatewayService)
│   └── publicBaseURL (construction-phase, immutable after NewGatewayService)
└── response.Writer
```

## Structural Observations

### Mode Bifurcation
- **Mode-specific services**: `G8eoService` (outbound mode only), `GatewayModeService` (gateway mode only), `mcp.GatewayService` (gateway-only; `MCPGateway` is in `GatewayCommandServiceConfig` and wired via `NewGatewayOperatorPubSubService`; not present in outbound mode's `CommandServiceConfig`)
- **Shared services**: `CanonicalDBService` (used in both modes for state root calculation - full service in gateway mode, state root calculation only in outbound mode)
- **Governance dependencies**: `FieldReader`, `TribunalStore`, `ReplayStore`, `StateRootProvider`, `TransactionAudit`, `L3Notary`, `SignerStore`, and `AppPolicyStore` are consolidated in `pubsub.GovernanceDeps`, passed as a separate parameter to `NewOperatorPubSubService` and embedded via `GovDeps *GovernanceDeps` in `GatewayCommandServiceConfig`. In outbound mode, `TribunalStore` and `FieldReader` are nil; in gateway mode, all fields are populated via `GetGovernanceDeps()`

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
- **`storage.SQLAuditStore`** is embedded in CanonicalDBService as the `AuditStore` field and provides the SQL-based audit storage foundation for both gateway and outbound modes. In outbound mode, the standalone instance has been removed; `g8eo.go` reuses `CanonicalDBService.AuditStore` for all audit writes (L5Actuator, HistoryHandler, session management), eliminating a redundant connection pool and pruner on the same `g8e.db` file.
- **`vault.Vault`** is shared across all storage services in outbound mode (reused from CanonicalDBService).

### Dependency Flow
- `scrubbing.ScrubbingService` depends on `storage.TokenStore` (interface). The outbound mode implementation is `gateway.EncryptedKVAdapter`.
- `gateway.EncryptedKVAdapter` has no dependency on `scrubbing.ScrubbingService` (circular dependency removed).
- All outbound storage services (ExecutionVaultService, EncryptedKVAdapter, SQLAuditStore, GitLedgerService) share the same `vault.Vault` instance from CanonicalDBService.
- `gateway.SecretManager` depends on `gateway.CanonicalDBService` for keystore access.

### Governance Stack (L1-L5)
- **L1**: `governance.L1Doctrine` (technical bedrock validation, threat detection, forbidden pattern matching)
- **L2**: `tribunal.TribunalService` (Tribunal-based deliberation producing L2 votes via Ed25519 signatures; gateway delegates deliberation via `LocalDeliberator`). The `TribunalStore` interface in `governance.L4Warden` loads `TribunalPolicy` for quorum verification.
- **L3**: `governance.L3Notary` (gateway mode uses `governance.gatewayNotary` via `governance.NewGatewayL3Notary`, combining WebAuthn passkey proofs via `PasskeyService` and mTLS CLI proofs via `cliSessionVerifier`; outbound mode uses `governance.outboundNotary` via `governance.NewOutboundL3Notary` for CLI-based approval via suspended transactions; gateway CLI mode uses `governance.cliNotary` via `governance.NewCLIL3Notary` for CLI session verification + suspended transaction checks)
- **L4**: `governance.L4Warden` (pre-dispatch verification gating, validating signatures, replay prevention, expiry, nonces, and state Merkle root)
- **L5**: `governance.L5Actuator` (isolated boundary tool dispatch via MCP/A2A, signed receipt production, audit logging). Does NOT re-verify L2/L3 proofs; trusts `VerifiedTransaction` from L4Warden. The L4→L5 separation is the defense-in-depth boundary: L4 verifies, L5 executes and records.

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
- `governance.gatewayNotary` implements: `governance.L3Notary` (gateway mode via `NewGatewayL3Notary` with both `cliSessionVerifier` and `PasskeyService` as delegates).
- `governance.outboundNotary` implements: `governance.L3Notary` (outbound mode via `NewOutboundL3Notary` for suspended transaction + signature verification only).
- `governance.cliNotary` implements: `governance.L3Notary` (gateway CLI mode via `NewCLIL3Notary` for CLI session verification + suspended transaction checks).
- `gateway.cliSessionVerifier` implements: `governance.CLISessionVerifier` (used in gateway mode for mTLS CLI session verification within the L3 notary).

### PasskeyService/PasskeyHandler Domain-HTTP Split
- **`passkey_service.go`**: `PasskeyService` reduced to domain-only fields (`userStore`, `sessionStore`, `webauthn`, `logger`, `rpID`, `rpName`). `NewPasskeyService` signature simplified to `(db, logger, cfg)`. Retains domain logic: `VerifyL3Proof`, `GenerateRegistrationChallenge`, `VerifyRegistration`, `GenerateAuthenticationChallenge`, `VerifyAuthentication`, `GenerateApprovalChallenge`, `addCredential`, `listCredentials`, `revokeCredential`, `getUser`. `VerifyL3Proof` stays on `PasskeyService` (L3 binding to transaction hash per architectural guardrails).
- **`passkey_service_http.go`**: All passkey HTTP handlers now live on `*PasskeyHandler` as 4 factory methods (`RegisterChallenge`, `RegisterVerify`, `AuthenticateChallenge`, `AuthenticateVerify`) accepting a typed `passkeyHandlerConfig`, plus 3 direct handlers (`ListCredentials`, `RevokeCredential`, `CLIStatus`). All 7 methods have Swagger annotations (`@Summary`/`@Router`/`@Success`/`@Failure`).
- **`passkey_service_approvals.go`**: 6 approval functions (`handleApprovalAction` dispatcher, `handleApprovalChallenge`, `handleApprovalVerify`, `handleCLIApprovalStatus`, `handleApprovalPage`, `handleListSuspendedTransactions`) now live on `*PasskeyHandler`. All dependencies (`mcpSvc`, `suspendedStore`, `sseStore`, `pubsub`) are passed via the `PasskeyHandlerDeps` constructor — no post-construction setters remain.
- **`PasskeyHandler`** struct embeds `*PasskeyService` and adds HTTP concerns (`webSessionSvc`, `responder`, `maxPayload`, `mcpSvc`, `suspendedStore`, `sseStore`, `pubsub`). `NewPasskeyHandler(deps PasskeyHandlerDeps)` constructor wires all dependencies at construction time.
- **`gateway_service.go`**: `passkey` field → `*PasskeyHandler`, both constructors updated, `GetGovernanceDeps` returns `*pubsub.GovernanceDeps` (consolidating all governance dependencies including `L3Notary` which wraps `ls.passkey.PasskeyService` via `NewGatewayL3Notary`).
- **`gateway_http.go`**: `HTTPHandlerDeps.Passkey` and `HTTPHandler.passkey` → `*PasskeyHandler`. `GetPasskeyService()` renamed to `GetPasskeyHandler()`.
- **`auth_controller.go`**: `AuthController.passkey` field and `newAuthController` param → `*PasskeyHandler`.
- **`passkey_service_approvals_test.go`**: Tests all handlers on `*PasskeyHandler` with mocked dependencies.
- **`passkey_service_http_test.go`**: Tests for the 7 passkey HTTP handlers on `*PasskeyHandler`.
- **`passkey_service_test.go`**: Tests for `PasskeyService` domain logic, including `VerifyL3Proof` WebAuthn assertion verification.
- **`internal/models/auth.go`**: `PasskeyCredential.Validate()` method performs on-disk schema validation before persistence in `addCredential`.

### Transport & Protocol Layer
- `pubsub.OperatorPubSubService` is the dispatcher for outbound mode (WebSocket pub/sub).
- `mcp.GatewayService` handles MCP/A2A protocol translation and downstream dispatch (shared between modes).
- `gateway.HTTPHandler` builds the HTTP/WebSocket surface for gateway mode.
- `gateway.GatewayWebSocketHandler` is the in-process pub/sub broker for gateway mode.
- `gateway.PKIAuthority` manages PKI hierarchy and certificate lifecycle for gateway mode.
- `network.Detector` detects host IP addresses and DNS names to configure TLS certificate identities dynamically during boot and renewal.
- `governance_envelope.go` provides the synchronous envelope-processing endpoint at `/api/v1/governance/envelopes`. `SetEnvelopeProcessor` wires the in-process `OperatorPubSubService` into the gateway HTTP surface via `atomic.Pointer`. `verifyEnvelopeIdentityBinding` enforces transport-to-envelope identity binding by matching mTLS certificate URI SANs against the envelope's internal identity claims. The tribunal deliberate route is always registered in the router; `handleTribunalDeliberate` checks the `atomic.Pointer` at request time and returns 503 if not yet wired, eliminating the need for a router rebuild when `SetTribunal` is called.
- `gateway_db.go` embeds the SQLite schema from `db/schema.sql` and provides `GatewaySchema()` for database initialization. `gateway_certs.go` defines certificate validity periods and common names for all g8e CAs (root, intermediate, serving, leaf, peer).
- `gateway_pubsub.go` defines `GatewayWebSocketHandler` for WebSocket-based publish/subscribe channels, including subscriber management and in-process handlers for governance command processing and SSE streaming.

### Gateway HTTP Dual-Router Architecture
- **`buildPublicRouter`** (HTTPS port): Full API surface with mTLS middleware via `auth.Middleware`. Routes include governance envelopes, audit, PKI management, user management, MCP/A2A ingress, SSE, pub/sub, console SPA, passkey endpoints, and approval UI. WebSessionAuth-protected routes bypass mTLS and use cookie-based auth with their own middleware. The landing page (`/`) redirects to `/console/`.
- **`buildHTTPRouter`** (HTTP port): Bootstrap-only surface for CA discovery, trust scripts, deploy scripts, CLI enrollment, and state endpoint. All other paths redirect to HTTPS. Wrapped with rate limiting and path traversal guard.
- **`RouteAuthRegistry`** classifies every route by its authentication mode (`RouteAuthNone`, `RouteAuthMTLS`, `RouteAuthWebSession`, `RouteAuthDual`). Exact paths are matched with highest priority, then prefix matches. Unknown routes default to `RouteAuthMTLS` (fail-closed). When JWKS is enabled, MCP/A2A and JIT passkey routes are reclassified to `RouteAuthNone` (JWT middleware handles auth).
- **`PrivilegedRouteRegistry`** blocks app certificates from governance envelope submission and query endpoints. Only operator and CLI auth are accepted on these routes.
- **`gateway_http_middleware.go`**: `rateLimitMiddleware` applies per-IP token bucket rate limiting with 5-minute stale entry cleanup. `pathTraversalGuard` rejects requests containing `..` path segments before ServeMux normalization. `isPrivateIP` reports whether an IP address is in a private network range, used by the HTTP router's safe-host check.
- **`gateway_http_sse.go`**: SSE event bridge with three endpoints. `POST /api/v1/sse/push` for event production, `GET /api/v1/sse/events` for polling, `GET /api/v1/sse/stream` for SSE streaming. Events are routed by `web_session_id`, `cli_session_id`, or `user_id`.
- **`gateway_http_health.go`**: Health check endpoint (`/health`) returns platform settings and state root status. Landing page handler redirects `/` to `/console/`. Bootstrap health check on the HTTP port returns a simplified health response.
- **`gateway/docs/`**: Embedded OpenAPI/Swagger specifications (`docs.go` embeds `swagger.json` and `swagger.yaml`) served at `/swagger/` with Swagger UI.
- **`gateway/scripts/`**: Thread-safe deploy script templates (`g8e-operator.sh`, `g8e-operator.ps1`) with Go bindings in `templates.go`, initialized via `sync.Once`.
- **`gateway/console/`**: Embedded Console SPA (`console.go` exposes `Handler()` serving the static filesystem from `static/`).

### HTTP Controller Decomposition
- **`gateway.PKIController`** (`pki_controller.go`): PKI enrollment, CSR signing, CA bundle, trust scripts (Linux/macOS/Windows), deploy scripts, certificate revocation, app enrollment.
- **`gateway.DBController`** (`db_controller.go`): Audit receipts, audit events, audit summary, audit report, data DB, KV store, blob storage, governance signers, pub/sub publish and stream.
- **`gateway.AuthController`** (`auth_controller.go`, `auth_controller_bootstrap.go`, `auth_controller_session.go`): Local bootstrap with URL, bootstrap status, CLI enrollment, device enrollment, user creation, user me, web session, logout. Decomposed into three files: core constructor, bootstrap flows, and session/user management.
- **`gateway.AdminController`** (`admin_controller.go`): App policy management by signer, app revocation, tribunal CRUD.
- **`gateway.OperatorController`** (`operator_controller.go`): Operator list, terminate, bind/unbind operators, set target context, reauth.

### JWT Authentication
- **`gateway.JWKSProvider`** (`jwks.go`): Optional external IdP JWT validation via JWKS endpoint. When configured, MCP/A2A routes accept JWT auth in addition to mTLS.
- **`gateway.NativeJWT`** (`jwt_native.go`): Native RSA-SHA256 JWT validation without external dependencies. Validates `exp`, `nbf`, `iat` claims with 60-second clock skew allowance. Used when JWKS is configured but no external HTTP call is needed for key resolution.
- **`gateway.AuthService`** applies JWT middleware to MCP/A2A routes and JIT passkey bootstrap routes when JWKS is configured. App policy enforcement and rate limiting apply to JWT-authenticated requests.

### Persona Service
- **`gateway.PersonaService`** (defined in `user_service.go`): Manages role-based access control personas. Initialized with `DefaultPersonaDefinitions` during gateway startup. `AuthService` references personas for authorization decisions.

## CLI Command Tree

The `g8e` binary (`internal/cli/cmd/main.go`) registers the following subcommands on the root Cobra command:

- **`gw`** (alias `gateway`): Gateway lifecycle management. Subcommands: `start` (background process), `serve` (foreground worker, hidden, re-exec target for `start`), `stop`, `status`, `restart`, `logs`, `settings`, `reset`, `clean`. Also includes `data` and `security` subcommand groups.
  - **`data`**: Administer the local platform over mTLS. Subcommands: `users`, `operators`, `settings`, `store`, `audit`.
  - **`security`**: Security validation checks. Subcommands: `validate`, `pki`.
- **`auth`**: Authentication and session management. Subcommands: `enroll` (CSR-based enrollment with passkey registration), `logout`, `approve` (interactive OOB approval of suspended transactions via passkey).
- **`mcp`**: MCP protocol operations. Subcommands: `stdio` (run g8e as an MCP server over stdio), `agent` (agent integration commands for AI coding tools). `agent` subcommands: `list` (list supported agent binaries), `show` (print MCP client configuration for a specific agent), `run` (launch an agent with g8e MCP configuration).
- **`operator`**: Manage Operator instances. Subcommands: `list`, `run`, `cp`, `scp`, `deploy`, `stream`.
- **`vault`**: Encryption vault management. Subcommands: `init`, `unlock`, `rekey`, `status`, `reset`, `export`, `import`.
- **`test`**: Run test suites. Subcommands: `unit`, `integration`, `e2e`, `coverage`, `lint`, `agent`, `chaos`, `summary`.
- **`demos`**: Demo scenario management. Subcommands: `list`, `start`, `stop`, `status`, `clean`, `rebuild`, `reset`, `run`, `pull`.
- **`audit`**: Run audit reports for compliance. Subcommands: `receipts`, `export`, `report`, `events`, `summary`.
- **`report`**: Generate CSV evidence reports. Subcommands: `all`, `verify`.
- **`swagger`**: Manage Swagger/OpenAPI documentation. Subcommands: `init`, `serve`, `validate`.
- **`tui`**: Launch the Tactical Governance Console (TUI). Requires a running gateway, enrolled CLI credentials, and mTLS client configuration.
- **`agent`** (alias `agent-harness`): Universal agent harness for a real g8e Gateway/Operator. Subcommands: `list` (list available scenarios), `run` (run scenarios against a real Gateway/Operator), `audit` (audit signed receipts from the Operator). Also registered as a `test` subcommand.

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

## CLI Serve Layer (Operator & Gateway Boot)

- **`internal/cli/serve/cert.go`**: PKI enrollment and certificate lifecycle: `PerformAutomaticEnrollment` (initial enrollment via CSR + trust bundle fetch, returns `(sessionID, err)` instead of calling `os.Setenv` internally), `RenewOperatorCertificate` (re-enrollment for expiring certs, decomposed into 5 testable units: `checkCertExpiry`, `fetchAndSaveTrustBundle`, `buildMTLSClient`, `submitRenewal`, `saveRenewedCerts`), `RunClientCertRenewalLoop` (periodic renewal check). `CertPaths` struct decouples path configuration from `paths.Infra`. HTTP client uses 15s timeout via `http.Client` + `http.NewRequestWithContext`. Error wrapping standardized with `ErrEmptyTrustBundle`, `ErrCAParseFailed`, `ErrMissingRequiredField`.
- **`internal/cli/serve/operator.go`**: Operator boot sequence: `RunOperator` orchestrates config loading, trust bundle setup, enrollment, and signal handling. Extracted helpers: `resolveKeyPath`, `resolveCertPath`, `loadClientCertPair`, `buildOperatorLoadOptions`.
- **`internal/cli/serve/gateway.go`**: Gateway boot sequence: `RunGateway` orchestrates config loading, `GatewayModeService` construction, in-process execution service initialization, tribunal bootstrap (policy seeding via `bootstrapTribunalPolicy`, key loading via `BootstrapTribunal` with `FileKeyProvider`), in-process `OperatorPubSubService` construction via `NewGatewayOperatorPubSubService` with `GatewayCommandServiceConfig` (embedding base `CommandServiceConfig` plus `MCPGateway` and `GovDeps *GovernanceDeps`), `SetEnvelopeProcessor` wiring, `SetTribunal` and `SetTribunalDeliberator` wiring under consensus posture, and graceful shutdown with 30-second timeout. `ExportActuatorPublicKey` writes the actuator public key to the PKI directory for receipt verification by external harnesses.
- **`internal/cli/serve/logger.go`**: Logger configuration: `ConfigureLogger` and `ConfigureLoggerWithOutput` produce `slog.Logger` instances with operator-friendly formatting and configurable log levels.
- **`internal/cli/serve/version.go`**: `VersionInfo` struct holds build-time metadata (version, build ID, build time, platform) set via ldflags.
- **`internal/cli/cmd/gateway.go`**: Gateway CLI command tree. `gatewayStartCmd` launches the gateway as a background process via `platform.StartProcess`, resolving configuration from CLI flags and environment variables. `gatewayServeCmd` (hidden) is the foreground re-exec target that calls `serve.RunGateway`. `gatewaySettingsCmd`, `gatewayResetCmd`, and `gatewayCleanCmd` manage gateway state over mTLS.
- **`internal/cli/cmd/tui.go`**: `tuiCmd` launches the Tactical Governance Console (TUI). Loads config, checks operator status, loads credentials, builds an mTLS client, and constructs an SSE stream URL using the CLI session ID for real-time updates.
- **`internal/cli/cmd/gwstdout.go`**: `printNextSteps` outputs posture-aware guidance after the gateway starts, including CA trust instructions, CLI enrollment, governance posture configuration, operator and agent connection steps, and Console UI access.
- **`internal/services/gateway/cli_cert.go`**: `VerifyCLICertificate` validates that a CLI mTLS certificate is valid for a given CLI session by checking expiry and matching SPIFFE URI SANs against the expected workload identity.
- **Test Coverage**: `cert_test.go` covers `PerformAutomaticEnrollment` (6 tests), `RenewOperatorCertificate` (9 tests), `RunClientCertRenewalLoop` (1 test) with hermetic `httptest.Server` and real certificate generation. `operator_test.go` covers extracted helpers at 100%. `gateway_test.go` covers gateway boot sequence. `internal/cli/serve` overall at 49.6% coverage.

## CLI Platform & Stream Packages

- **`internal/cli/platform/`**: Cross-platform process management for operator subprocesses. `process.go` provides core process lifecycle. `process_unix.go` and `process_windows.go` provide platform-specific process discovery and signal handling. `browser.go` provides cross-platform browser opening for console URLs.
- **`internal/cli/stream/`**: SSH and subprocess streaming for remote operator management. `stream.go` provides the streaming CLI command. `stream_ssh.go` provides SSH connection management for remote log streaming and command execution.

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

**`internal/tools/chaos/`** - Test-only chaos testing for governance stack
- Generates a realistic distribution of governance events (70% good actor, 20% prompt injection, 10% MitM) against the local audit stack
- Drives `TransactionVerifier` + `Actuator` stack directly in-process, bypassing network/TLS
- Uses `storagetest.TestSQLAuditStore` and should not be used in production code paths

**Key distinction**: Test infrastructure is separated from production code to avoid import cycles. The `storagetest`, `pubsubtest`, and `chaos` packages provide test implementations that should never be used in production code paths.

**`test/fixtures/gateway_fixture.go`** - In-process gateway test fixture (build tag: `integration`)
- `GatewayFixture` spins up a real `GatewayModeService` with `httptest.Server`, mTLS PKI, tribunal enrollment, and in-process `OperatorPubSubService` wired with full governance dependencies
- Used by integration tests for MCP flow, A2A flow, tribunal consensus, governance envelope verification, OOB suspension/approval, and downstream integration

**`test/e2e/harness.go`** - Docker-based E2E test fixture (build tag: `e2e`)
- `DockerE2EFixture` spins up docker-compose, allocates host ports, waits for gateway health, and tears down on cleanup
- Tests only what is observable from outside containers: HTTP health, CA bundle discovery, port reachability, and mTLS handshake over network
- Supports `G8E_E2E_SKIP_DOCKER` environment variable to skip when Docker is unavailable

**`test/integration_helper.go`** - Shared integration test helpers (build tag: `integration` or `e2e`)
- `NewLiveOperatorHTTPClient` creates an mTLS API client against a running g8e platform
- `ResolveRepoRootFromTestDir` finds the repository root using `go list -m`

**Integration test files** (`test/`):
- `universal_gateway_integration_test.go` - MCP/A2A flow, multi-protocol auto-detection, governance envelope verification, OOB suspension/approval, downstream integration, canonical JSON wire format
- `tribunal_consensus_integration_test.go` - Tribunal idempotent enrollment, malformed CSR rejection, delegated app enrollment, quorum reached/not reached, veto by MITRE, L1-to-L5 walkthrough
- `mcp_gateway_test.go` - MCP gateway end-to-end, tools/list, tools/call, suspended transaction handling
- `mcp_gateway_config_test.go` - MCP gateway configuration validation tests
- `a2a_gateway_test.go` - A2A protocol gateway tests
- `native_tool_registry_integration_test.go` - Native tool registry integration tests
- `test/e2e/auth_e2e_test.go` - E2E authentication flow tests (build tag: `e2e`)
- `test/e2e/gateway_e2e_test.go` - E2E gateway lifecycle and health tests (build tag: `e2e`)
- `test/e2e/mcp_stdio_e2e_test.go` - E2E MCP stdio config output, JSON-RPC parsing, config template validation (build tag: `e2e`)

## Agent Harness & Demos

**`internal/tools/agent_harness/`** - Reference client for real governance envelope submission
- `client/client.go` - mTLS client: `StateRoot`, `RegisterSigner`, `Approve` (uses `constants.APIPaths.*`)
- `client/envelope.go` - `SubmitMaximal`: builds real `GovernanceEnvelope` with L1/L2/L3, submits over mTLS
- `client/audit.go` - `AuditReceipts`, `ExportReceipts`, `DiscoverOperator` (parses cert SAN for offline session discovery)
- `client/protocols.go` - JSON-RPC request/response types and A2A protobuf envelope encoding for MCP/A2A protocol ingress
- `config/config.go` - Harness configuration: auth material (client cert/key/CA bundle), gateway URL, posture selection
- `scenarios/governance.go` - Governance scenarios: consensus, notary, delegation, veto, OOB approval
- `scenarios/dow_cross_cue.go` - DoW scenarios: `dow-cross-cue` (real slew envelope) and `dow-bft-veto` (veto envelope)
- `scenarios/dhs_sovereign.go` - DHS sovereign operations scenarios: multi-step governance workflow with tribunal consensus
- `scenarios/mcp_a2a.go` - MCP and A2A protocol scenarios: plain MCP, mTLS MCP, A2A JSON, A2A mTLS, A2A protobuf
- `scenarios/gov_finance.go` - Gov/finance doctrine scenarios: `gov-cui-exfil-block` and `finance-unauthorized-trade`
- `scenarios/secure_data.go` - Secure-data scenarios: `secure-data-migration` (consensus), `secure-data-bypass-attempt` (doctrine), `secure-data-cross-tenant` (doctrine)
- `scenarios/swarm.go` - Swarm scenarios: `swarm-recon-mission` (consensus: governed drone deployment), `swarm-weapon-release-block` (doctrine: weapon release blocked), `swarm-restricted-airspace-block` (doctrine: restricted airspace blocked)
- `scenarios/scenario.go` - Scenario registry, `Execute`, `Posture` types

**`demos/dow/`** - Department of War tactical edge demo
- `gimbal.py` - Mock gimbal HTTP server on `net_secure` (records slew commands to `/var/gimbal/slews.jsonl`)
- `slew.sh` - Demo artifact mounted at `/usr/local/bin/slew` in operator container; wraps gimbal HTTP call
- `inspect_pnt.py` - Demo artifact for PNT inspection in `agent-pnt-fusion` container
- `inspect_rf.py` - Demo artifact for RF spectrum inspection in `agent-eoir` container
- `verify_slews.py` - Demo artifact for verifying gimbal slew results
- `dow_simulator.py` - Display-only sensor narration for `agent-eoir` and `agent-pnt-fusion`

**`demos/dhs/`** - DHS sovereign data operations demo
- `datasvc.py` - Mock data service HTTP server for sovereign data access
- `dataop.sh` - Demo artifact for data operations invocation
- `verify_ops.py` - Demo artifact for verifying data operation results

**`demos/healthcare/`** - Healthcare analytics demo with Metabase integration
- `pa_api_server.py` - Prior authorization API server mock
- `setup_metabase.py` - Metabase initialization and dashboard setup
- `init.sql` - Database schema initialization for healthcare data

**`demos/swarm/`** - Drone swarm tactical demo
- `drone_simulator.py` - Mock drone telemetry and command simulation

**`demos/finance/`** - Financial data governance demo

**`demos/gov/`** - Government operations demo

**`demos/secure-data/`** - Secure data handling demo

**CLI Demo Scenario Files** (`internal/cli/cmd/`):
- `demos.go` - Demo CLI command tree (list, start, stop, status, clean, rebuild, reset, run, pull). Contains `harnessConfig` struct, `defaultHarnessConfig`, `harnessRun` helper for building docker compose exec/run commands for agent-harness scenarios, and `runTwoLayerScenario` reusable orchestrator. `demoVerbose` flag (set by `-v`/`--verbose`), `demoStep` suppresses output when non-verbose, `demoPrintln`/`demoPrintf` (verbose-aware print helpers), `scenarioCounts` map (healthcare: 4, gov: 1, finance: 1, secure-data: 3, dow: 3, dhs: 5, swarm: 3), `printDemoEndpoints` (prints available endpoints per org).
- `demo_gov.go` - Gov demo scenario (uses `runTwoLayerScenario` with `harnessRun`)
- `demo_finance.go` - Finance demo scenario (uses `runTwoLayerScenario` with `harnessRun`)
- `demo_healthcare.go` - Healthcare demo scenarios (4 scenarios, each calls `harnessRun`)
- `demo_secure_data.go` - Secure-data demo scenarios (3 scenarios, each calls `harnessRun`)
- `demo_dow.go` - DoW demo scenarios (3 scenarios, each calls `harnessRun`)
- `demo_dhs.go` - DHS demo scenarios (5 scenarios, each calls `dhsHarnessRun` → `harnessRun`). Contains `dhsHarnessConfig`, `dhsHarnessRun`, `dhsScenarioStep`, `extractFirstTxHash`, `ensureDHSPosture` helpers.
- `demo_swarm.go` - Swarm demo scenarios (3 scenarios, each calls `harnessRun`): authorized recon mission, weapons safety doctrine block, navigation boundary violation block.
- `demos_test.go` - Tests for demo CLI commands, `scenarioCounts`, `printDemoEndpoints`, `harnessRun`/`defaultHarnessConfig` unit tests, `defaultHarnessConfig`/`harnessRun` unit tests, and source-file assertions (`TestDemoScenarioFilesCallHarnessRun`, `TestNoGatewayBypassInDemoFiles`, `TestNoSqliteBackdoorInScenarioFiles`, `TestNoCopyPasteInScenarioFiles`). Also tests `TestDemoPrintln`, `TestDemosPullCmd`, `TestCheckDockerAvailable`, `TestToDockerPath`, `TestDefaultHarnessConfig`, `TestHarnessRun`.
