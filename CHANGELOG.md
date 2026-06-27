# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

---

## [1.2.4] - 2026-06-26

### Overview

v1.2.4 is a console UX and passkey bootstrap refinement release. This version adds a browser-facing SSE audit stream to the console via the unified `/api/v1/sse/stream` endpoint, introduces automatic user creation during browser passkey bootstrap, hardens Windows Hello WebAuthn with HRESULT mapping and input validation, replaces hardcoded cookie name strings with a shared constant, and regenerates the Swagger/OpenAPI specification with full passkey and session management annotations.

### Added

* **Unified SSE Streaming for Browser** — The console SPA now connects to `/api/v1/sse/stream` (mTLS authenticated) with `web_session_id` as a query parameter, using the same endpoint as CLI and dashboard clients. No separate browser streaming endpoint, no mTLS bypass, no cookie-based auth for SSE.
* **Automatic User Creation on Browser Bootstrap** — Added `CreateUser` and `HasAnyUsers` methods to the `userStore` interface with `createUserOnBootstrap` config flag. Browser passkey bootstrap now auto-creates a user when no users exist, enabling true zero-config first-run enrollment.
* **Windows Hello HRESULT Mapping** — Added `mapWebAuthnHRESULT` function to translate Windows WebAuthn HRESULT codes to descriptive error messages (user cancelled, device not found, timeout, etc.). Added 8 new typed error constants in `internal/constants/errors.go`.
* **Windows Hello Input Validation** — Added input validation for challenge emptiness, user ID size (16-byte GUID for WebAuthn v4), and response pointer/size bounds checking before extraction, preventing potential memory safety issues.
* **Windows Crypto Mock Tests** — Added `internal/cli/auth/windows_crypto_mock_test.go` with comprehensive unit tests for Windows Hello WebAuthn flows using mock syscall interfaces.
* **Swagger/OpenAPI Regeneration** — Regenerated `swagger.json` and `swagger.yaml` with full annotations for all passkey endpoints, audit stream, and session management routes.
* **L3 Notary Tests** — Added `internal/services/governance/l3_notary_test.go` with comprehensive tests for L3 notary proof verification.
* **Passkey Register Challenge Response** — Added `user_id` field to `PasskeyRegisterChallengeResponse` model for client-side user tracking.

### Changed

* **WebSession Cookie Name Constant** — Replaced all hardcoded `"g8e_session"` string literals with `constants.WebSessionCookieName` across `auth_controller_session.go`, `passkey_service_http.go`, and `gateway_auth.go`.
* **Passkey Bootstrap CLI Path** — Updated `passkey_bootstrap.go` to use `constants.APIPaths.AuthPasskeysConsoleRegisterVerify` instead of hardcoded path string.
* **Gateway Serve Command Hidden** — Marked `g8e gateway serve` CLI command as hidden since it is an internal re-exec target, not user-facing.
* **Encode Challenge Helper** — Extracted `encodeChallenge` helper in `passkey_service.go` for semantic clarity over inline `base64.RawURLEncoding.EncodeToString` calls.
* **Deprecated Passkey Route Aliases** — Added `AuthPasskeysCLIRegisterChallenge`, `AuthPasskeysCLIRegisterVerify`, `AuthPasskeysCLIAuthenticateChallenge`, and `AuthPasskeysCLIAuthenticateVerify` to `PublicRouteRegistry` deprecated alias list for backward compatibility during route transition.

### Fixed

* **Console SPA Enhancements** — Updated console `index.html` with improved audit stream integration, session management, and error handling.

### Removed

* **Browser SSE Web Handler** — Deleted `internal/services/gateway/gateway_http_sse_web.go` and the `/api/v1/audit/stream` endpoint. Browser SSE streaming is now handled by the unified `/api/v1/sse/stream` endpoint with mTLS authentication.
* **Terminal Linux Package** — Deleted `internal/cli/serve/terminal_linux.go` and `terminal_linux_test.go` (262 lines). The obfuscated input reader was unused after the serve command was simplified.

---

## [1.2.3] - 2026-06-26

### Overview

v1.2.3 is a passkey architecture consolidation and dependency reduction release. This version moves all 15 passkey HTTP handlers from `AuthController` into `PasskeyService` with a typed config system, eliminates the `swaggo/swag` and `google/uuid` dependencies, adds a token bucket rate limiter, and introduces a comprehensive guide for building g8e-compliant agentic systems. The release also renames passkey routes to descriptive `bootstrap/*` and `console/*` paths with deprecated aliases for backward compatibility.

### Added

* **PasskeyService HTTP Layer** — Added `passkey_service_http.go` with four factory methods (`RegisterChallenge`, `RegisterVerify`, `AuthenticateChallenge`, `AuthenticateVerify`) using a typed `passkeyHandlerConfig` struct, plus three direct handler methods (`ListCredentials`, `RevokeCredential`, `CLIStatus`). Eliminates 15 copy-pasted handlers from `AuthController`.
* **Passkey Route Renames** — Renamed `cli-register`, `cli-browser-register`, and `browser/authenticate` passkey paths to descriptive `bootstrap/*` and `console/*` paths. Old paths retained as deprecated aliases with structured deprecation logging for one minor version.
* **Token Bucket Rate Limiter** — Added `internal/services/gateway/token_bucket.go` with a thread-safe token bucket implementation for rate limiting.
* **Passkey Credential Validation** — Added `PasskeyCredential.Validate()` method and `encodeCredID`/`decodeCredID` helpers with comprehensive encoding tests.
* **Build Agentic System Guide** — Added `docs/guides/build_agentic_system.md`, a 727-line comprehensive guide for building g8e-compliant agentic systems.
* **Demos CLI Command** — Added `g8e demos` command with subcommands for listing and running demo scenarios.
* **Gateway Startup Output** — Improved gateway startup output with clear connection information and status display.
* **Passkey Service HTTP Tests** — Added `passkey_service_http_test.go` with table-driven config matrix tests covering all four factory methods and three direct handlers.

### Changed

* **PasskeyService Constructor** — Updated `NewPasskeyService` signature to accept `userSvc`, `webSessionSvc`, `responder`, and `maxPayload` dependencies, enabling the service to own its complete HTTP layer.
* **Router Rewired** — All passkey route registrations now reference `h.passkey.*` factory methods instead of `h.authController.handleAuthPasskey*` handlers. Config constants defined at route mount time, replacing fragile `strings.Contains(r.URL.Path, "/jit-")` URL sniffing.
* **PublicRouteRegistry Simplified** — Replaced 4 exact-path entries for browser passkey endpoints with 2 prefix entries (`bootstrap/` and `console/`), with deprecated alias exact paths maintained during transition.
* **CLI Client Updated** — `PerformNativeWindowsAuth` now uses `constants.APIPaths.AuthPasskeysBootstrapAuthenticate*` constants instead of hardcoded path strings.
* **Network Identity** — Enhanced `internal/services/network/identity.go` with improved identity resolution.
* **PKI Controller** — Improved PKI controller with better error handling and response structure.

### Fixed

* **CLI Passkey Status** — Fixed CLI reporting "No passkey registered" for users who enrolled via browser. CLI now uses dedicated mTLS endpoint `/api/v1/auth/passkeys/cli/status` with explicit error classification instead of silent fallback.
* **Passkey Credential ID Comparison** — Replaced unsafe `string()` cast with `bytes.Equal` for credential ID matching in `passkey_service.go`, preventing potential timing attack vectors.
* **Challenge Replay Prevention** — Added `DeleteSession` to the `sessionStore` interface. Both `VerifyRegistration` and `VerifyAuthentication` now proactively purge the stored WebAuthn challenge after successful verification, preventing challenge replay attacks.
* **DoW Simulator** — Fixed `demos/dow/dow_simulator.py` timing and state management issues.
* **Agent Harness Client** — Fixed API path references and improved test reliability in agent harness client.

### Removed

* **`auth_controller_passkey.go`** — Deleted entirely (358 lines). All 7 handlers moved to `PasskeyService`.
* **`auth_controller_bootstrap.go` passkey handlers** — Stripped 8 passkey handlers (535 lines). Non-passkey bootstrap handlers retained.
* **`auth_controller_passkey_test.go`** — Deleted (467 lines). Replaced by `passkey_service_http_test.go`.
* **`auth_controller_bootstrap_test.go` passkey tests** — Stripped passkey test functions (506 lines). Non-passkey tests retained.
* **Swagger dependency eliminated** — Removed `github.com/swaggo/swag` and `github.com/http-swagger` imports from `docs.go`, reducing 661 lines of auto-generated swagger registration code. Swagger UI now served via native embedded HTML handler.
* **`google/uuid` dependency** — Replaced with native `internal/uuid` package using `crypto/rand`.

---

## [1.2.2] - 2026-06-25

### Overview

v1.2.2 is a security hardening, testability, and documentation consolidation release. This version introduces a `PrivilegedRouteRegistry` to prevent app certificates from submitting governance envelopes, adds a private IP allowlist for MCP HTTP probing in disconnected edge environments, and improves shell safety by moving `curl`/`wget` to command-name-based blocking. The release also refactors CLI commands for dependency injection testability, consolidates three architecture docs into two, and adds ~2000 lines of new unit test coverage.

### Added

* **PrivilegedRouteRegistry** — New gateway auth component that blocks app certificates (issued via `/api/v1/pki/apps/enroll`) from submitting governance envelopes via `POST /api/v1/governance/envelopes`. Only operator and CLI mTLS certificates may submit envelopes.
* **Private IP Allowlist for MCP** — Added `SetPrivateIPAllowlist` and `isIPAllowed` in `internal/services/mcp/validation.go` with a thread-safe CIDR allowlist. MCP HTTP probe and request validation now permit configured private/loopback addresses, supporting disconnected edge scenarios where internal endpoints must be reachable.
* **PKI CSR Sign Endpoint** — Registered `constants.APIPaths.PKICSRSign` route in the HTTP router for headless CSR signing.
* **DoW Demo: Real Governance Envelope Submission** — Converted the DoW tactical edge demo Scenario 1 from scripted theater into a genuine end-to-end exercise. The `agent-sigint` container is now a real g8e binary that submits `GovernanceEnvelope`-wrapped `run_shell_command` tool calls through the full L1/L2/L5 pipeline. Added `dow-cross-cue` and `dow-bft-veto` harness scenarios. Added mock gimbal HTTP server (`gimbal.py`), `slew.sh`, `inspect_pnt.py`, `inspect_rf.py`, and `verify_slews.py` demo artifacts.
* **Operator Cert Sharing for Agent Containers** — Agent containers in demos can now share the operator's enrolled mTLS credentials via a read-only volume mount (`operator_state:/root/.g8e:ro`), eliminating the need for a separate enrollment init container.
* **Offline Session Discovery** — `DiscoverOperator` in the agent harness now parses the operator's PEM certificate SPIFFE URI SAN to extract operator ID and session ID, enabling headless session recovery in disconnected environments without network calls.
* **Comprehensive Test Coverage** — Added ~2000 lines of unit tests: approve API/integration tests, audit command tests, data command tests, gateway/operator/report/test command extra tests, reporting csvwriter and rows tests, MCP validation tests, gateway auth registry tests, agent harness dow_cross_cue scenario tests.
* **Architecture Documentation Consolidation** — Merged `docs/architecture/binding.md`, `docs/architecture/postures.md`, and `docs/architecture/transaction-process.md` into `docs/architecture/governance.md` and `docs/architecture/auth.md`. Added Session & Identity Binding section to `auth.md`. Added `docs/media/jit-mcp-with-receipts.png` diagram.

### Changed

* **Agent Harness Refactored** — Harness now uses `io.Writer` for all output (enabling test capture), returns errors instead of calling `os.Exit(1)`, and `DiscoverOperator` returns both operator ID and session ID. `GovKit` struct updated with `OperatorSessionID` field.
* **CLI Dependency Injection** — Refactored `approve`, `audit`, and `data` CLI commands to accept injectable config loader and API client factory functions, enabling unit testing without real network or config dependencies.
* **Shell Safety: curl/wget Blocking** — Moved `curl` and `wget` from `DangerousPatterns` (pattern-matched, easily bypassed) to `DangerousCommands` (command-name-matched) in `internal/constants/shell.go` for more reliable enforcement.
* **DoW Demo: Python Extraction** — Extracted inline Python scripts from demo compose into standalone files under `demos/dow/`.

### Fixed

* **App Cert Governance Envelope Submission** — Fixed a security issue where app certificates could submit governance envelopes. Now blocked by `PrivilegedRouteRegistry`.
* **MCP HTTP Probe Private IP Blocking** — Fixed unconditional blocking of private/loopback addresses in MCP HTTP probe, preventing legitimate internal endpoint access in edge deployments.
* **OperatorSessionId Propagation** — Fixed `SubmitMaximal` in the agent harness not setting `OperatorSessionId` on `GovernanceEnvelope`, causing L5 actuator receipt recording to fail with a FOREIGN KEY constraint.
* **Agent Harness API Path Hardcoding** — All five harness client methods now use `constants.APIPaths.*` constants instead of hardcoded path strings.

### Removed

* **`docs/architecture/binding.md`** — Consolidated into `docs/architecture/governance.md` and `docs/architecture/auth.md`.
* **`docs/architecture/postures.md`** — Consolidated into `docs/architecture/governance.md`.
* **`docs/architecture/transaction-process.md`** — Consolidated into `docs/architecture/governance.md`.
* **`demos/dow/enroll.sh`** — Deleted dead code. Replaced by operator cert sharing via volume mount.
* **Non-native Go dependencies eliminated** — Replaced three non-native Go dependencies with native Go solutions:
  * `github.com/google/uuid` — Replaced with `internal/uuid` package using `crypto/rand` for UUIDv4 generation.
  * `golang.org/x/text` — Replaced `cases.Title(language.English)` with a native `titleCase()` helper using the `strings` package.
  * `github.com/swaggo/swag` + `github.com/swaggo/http-swagger` — Replaced with a native embedded HTML handler serving Swagger UI from CDN. Removed 15+ transitive indirect dependencies.
* **`internal/services/gateway/docs/docs.go`** — Deleted auto-generated Swagger registration file (zero importers, contained `init()` function).

---

## [1.2.1] - 2026-06-25

### Overview

v1.2.1 is a maintenance and stability release addressing critical routing, authentication, and UX bugs identified in the v1.2.0 g8e Console release. This update ensures seamless WebSessionAuth-guarded API routing, resolves browser WebAuthn accessibility without client certificates, and optimizes SPA interactive approval states.

### Fixed

* **WebSessionAuth Router Exact-Match Bug** — Fixed a critical routing bug where outer router `WebSessionAuth` prefix registration patterns used exact-matching patterns (without trailing slashes), causing sub-paths like `/api/v1/users/me` to fall through to the catch-all landing redirect. Now registers proper subtree-matching patterns.
* **Browser Passkey Endpoints Missing from Public Registry** — Added the four browser-facing WebAuthn endpoints to `PublicRouteRegistry`, resolving a critical bug where browser authentication/registration requests were rejected with 401 Unauthorized by the mTLS middleware.
* **Landing Page Redirect Extra Hop** — Changed landing page redirect from `/console` to `/console/` to avoid an unnecessary extra HTTP 301 hop.
* **SPA Approval Auto-Trigger on Sign-In** — Fixed a UX issue where landing-redirected transaction approvals would fail to auto-trigger upon login because the location hash was only evaluated once on initial page load.
* **SPA Unnecessary Passkeys Requests** — Optimized state loading to avoid wasteful, failing requests to passkey/approval endpoints when the user is logged out.
* **PublicRouteRegistry Excluded Prefixes** — Fixed a security issue where the broad `/api/v1/auth/passkeys` public prefix exposed mTLS-only sub-paths (register, authenticate) and JIT passkey routes as public. Added an excluded-prefixes mechanism to `PublicRouteRegistry` that protects mTLS-only sub-paths while allowing WebSessionAuth management routes to bypass mTLS.
* **Approval Page Redirect Extra Hop** — Changed approval page redirect from `/console#approve=` to `/console/#approve=` (with trailing slash) to avoid an unnecessary extra HTTP 301 hop from Go's `http.ServeMux` auto-redirect.
* **L3 Notary Test `ExpiresAt` Bug** — Fixed `TestCLIL3Notary_VerifyL3Proof_AcceptsActiveUser` and `RejectsInvalidSignature` tests where missing `ExpiresAt` caused zero-value time (0001-01-01) to fail the SQL query's `WHERE expires_at > now` filter, causing tests to pass for the wrong reason.

---

## [1.2.0] - 2026-06-24

### Overview

v1.2.0 is a major feature release introducing the g8e Console SPA, a zero-dependency, single-page application served directly from the gateway binary. To support the interactive web dashboard, this release introduces dual-auth browser passkey endpoints and implements L7 hybrid TLS / mTLS enforcement, enabling secure passkey bootstrap and transaction approval from standard web browsers. It also locks down the plain-HTTP port 8080 and unifies public and operator routing under a single HTTPS server.

### Added

* **g8e Console SPA** — A zero-dependency, single-page application (HTML+CSS+JS) served over HTTPS at `/console` using Go's `embed.FS`. Acts as the unified dashboard for WebAuthn passkey registration, authentication, management, and interactive L3 transaction approval.
* **Dual-Auth Browser Passkey Endpoints** — Added browser-specific, public endpoints (`/api/v1/auth/passkeys/browser/authenticate/challenge` and `/verify`) that permit unauthenticated WebAuthn login to obtain web sessions without carrying an mTLS client certificate.
* **Hybrid TLS / L7 mTLS Enforcement** — Transitioned the HTTPS server TLS configuration to `tls.VerifyClientCertIfGiven`, moving the fail-closed mTLS gate to the application layer (`auth.Middleware`). All non-public routes continue to require client certificates.
* **Passkey Management Under WebSessionAuth** — Passkey listing and revocation routes are now protected under WebSessionAuth, allowing users to safely manage their credentials.

### Changed

* **HTTP Port Lockdown** — Stripped non-bootstrap routes from the plain-HTTP port (`8080`), confining it exclusively to trust-establishment and CA discovery scripts. Added a catch-all redirect forcing all other requests to HTTPS (`8443`).
* **Router Consolidation** — Fully retired the old `buildRouter()` method, merging its mTLS routes into `buildPublicRouter()`. The single unified HTTPS server now handles both mTLS operator routes and public web routes.
* **Landing Page & Approval Redirects** — Replaced legacy inline HTML rendering for `/` and `/api/v1/approve/{txHash}` with 302 redirects to `/console` and `/console#approve={txHash}`.

## [1.1.9] - 2026-06-23

### Overview

v1.1.9 is a core architecture realignment release that corrects the governance verification ordering, eliminates auto-approval theater, introduces just-in-time execution capabilities, and separates bound vs observed state in the canonical database. This release also hardens CLI L3 notary verification with cryptographic proof binding, consolidates token persistence into the canonical KV store, and removes the deprecated intent grant/revoke subsystem.

### Breaking Changes

* **`auto_approved` field removed from `L3Metadata` proto** — The `bool auto_approved` field has been removed from `L3Metadata` in `common.proto`. Deployments that relied on auto-approval for doctrine/consensus postures must now configure posture-appropriate L3 verification instead. The gateway no longer auto-sets L3 metadata for any posture.
* **`GRANT_INTENT` and `REVOKE_INTENT` action types removed** — The intent grant/revoke subsystem (action types, events, proto messages, tool result models) has been fully removed. Any code referencing these action types or events will fail to compile.
* **`TokenStoreService` replaced by `EncryptedKVAdapter`** — The standalone `token_store.db` and `TokenStoreService` have been removed. Token persistence now routes through `gateway.EncryptedKVAdapter` backed by the canonical gateway DB (`g8e.db`) KV store. Existing `token_store.db` files are no longer used.

### Added

* **JIT Execution Capabilities** — New `internal/services/governance/capability.go` providing `Capability`, `MintCapability`, and `ContextWithCapability`. The L5 Actuator now mints a scoped, single-action, self-dissolving capability before execution and dissolves it immediately after — enforcing zero standing privileges.
* **Bound vs Observed State Tiering** — Added `state_tier` column to `kv_store` and `blobs` tables in the canonical gateway DB. Bound-state rows are included in the transaction freshness root; observed-state rows are hashed separately in `GetObservedStateRoot()` for audit ledger chaining without churning in-flight envelopes.
* **Observed-State Root** — `StateRootService` now computes a separate `calculateObservedStateRoot()` for observed-tier KV and blob entries, chained into the audit ledger.
* **Token Keymap Hash Binding** — The UEI token keymap hash is now bound into the bound state root (§9 binding), ensuring rehydration substitution after a transaction hash was computed produces a broken transaction rather than silent corruption.
* **Cryptographic CLI L3 Verification** — CLI L3 notary now verifies the approver's Ed25519 signature against a stored public key, not just the mTLS certificate fingerprint. `ApprovalPublicKey` field added to `SuspendedTransaction` model and `handleCLIApproval` endpoint.
* **Posture-Specific Warden Tests** — Decomposed monolithic `l4_warden_test.go` into `l4_warden_consensus_test.go`, `l4_warden_doctrine_test.go`, and `l4_warden_notary_test.go` with posture-specific coverage.
* **Capability Unit Tests** — New `internal/services/governance/capability_test.go` with comprehensive coverage of minting, verification, dissolution, and expiry.
* **L3 Notary Unit Tests** — New `internal/services/governance/l3_notary_test.go` with 241 lines covering CLI L3 verification paths.
* **L3 Proof Hash Exclusion Test** — New `TestGenerateMessageID_L3ProofNotInHash` in `pkg/governance/types_test.go` verifying L3 proof does not affect the transaction hash.
* **Architecture Documentation** — New `docs/architecture/governance.md` (absorbing `transaction-process.md`); detailed layer descriptions and transaction flow. Session & Identity Binding documentation added to `docs/architecture/auth.md`.

### Changed

* **Warden Verification Order** — `verifyPosture` in `l4_warden.go` now verifies L2 (machine consensus) before L3 (human-presence). This preserves the invariant that the human's approval bond is spent only on transactions that have already cleared tribunal consensus.
* **Tribunal Deliberation Under Notary Posture** — The gateway now sends envelopes to the Tribunal for L2 deliberation under both consensus and notary postures (previously consensus only).
* **Gateway Start Config Refactor** — `resolveStartConfig` and `runGatewayMode` refactored to accept struct parameters (`startConfig` / `GatewayStartConfig`) instead of long parameter lists.
* **Token Persistence Consolidation** — `G8eoService` now uses `gateway.NewEncryptedKVAdapter` for token storage instead of a separate `TokenStoreService` with its own SQLite DB.
* **Scrubbing Test Simplification** — Scrubbing service tests replaced real vault/tokenstore setup with `fakeTokenStore` in-memory implementation.

### Fixed

* **Warden Order Bug** — Fixed verification ordering where L3 (human-presence) was checked before L2 (consensus), causing humans to be asked to authorize content the machines had not yet vetted.
* **Proof Binding Circularity** — L3 proof is now intentionally excluded from `GenerateMessageID` transaction hash computation, resolving a circular dependency where L2 couldn't sign until the human had already acted.

### Removed

* **Auto-Approval (`acktheater`)** — Removed `auto_approved` field from `L3Metadata` proto, auto-approval logic from gateway `processGatewayTransaction`, and auto-approval bypass in `verifyL3Posture`.
* **Intent Grant/Revoke Subsystem** — Removed `ActionTypeGrantIntent`, `ActionTypeRevokeIntent`, all intent-related events (`EventOperatorIntent*`), `GrantIntentRequested/Result` and `RevokeIntentRequested/Result` proto messages, intent tool result models, and intent validation from `TribunalMember.evaluateSafety` and `L1Doctrine`.
* **Standalone Token Store** — Removed `TokenStoreService` implementation, `TokenStoreConfig`, `TokenStoreDBFilename`, `TokenStoreDBPath`, and `token_store.db` references.
* **Deprecated Integration Tests** — Removed `byo_client_e2e_test.go`, `gateway_integration_test.go`, and `tribunal_consensus_test.go` to be replaced with proper Tier 1/2 tests.

---

## [1.1.8] - 2026-06-23

### Overview

v1.1.8 is a maintenance and stability release focused on hardening the Tribunal consensus infrastructure, reorganizing development tooling for better maintainability, and improving the reliability of the end-to-end integration test suite.

### Added

* **Tribunal Store Hardening** — Expanded `TribunalStoreService` with additional validation logic and comprehensive unit test coverage to ensure robust policy management.
* **Tooling Reorganization** — Centralized development and testing tools (Agent Harness and Chaos Testing utilities) by moving them from `test/` to `internal/tools/`, improving repository structure.
* **E2E Harness Enhancements** — Significantly improved the E2E test harness (`test/e2e/harness.go`) and added new gateway test fixtures to increase test stability and coverage.

### Changed

* **Governance Cleanup** — Removed redundant test implementations in `internal/services/governance/` and consolidated testing logic within the improved harness.
* **Configuration Management** — Added new configuration options for gateway services to support improved testing and deployment scenarios.

### Fixed

* **Test Stability** — Resolved flakiness in several E2E and integration tests by improving fixture handling and harness reliability.
* **Tool Integrity** — Fixed minor issues in the agent harness to ensure consistency across testing environments.

---

## [1.1.7] - 2026-06-23

### Overview

v1.1.7 introduces the Tribunal system, replacing the gateway's L2 self-signing mechanism with a proper deliberation-based consensus model. The gateway now delegates L2 deliberation to an enrolled Tribunal service rather than signing L2 votes locally. This release also removes the deprecated `gateway_signed` field from the protocol and updates all documentation to reflect the Tribunal architecture.

### Added

* **Tribunal Service** — New `internal/services/tribunal/` package providing `TribunalService` with `Deliberate`/`HandleDeliberate` methods, `LocalDeliberator` for in-process deliberation, and `HTTPTribunalDeliberator` for remote tribunal communication over mTLS.
* **Tribunal Store Service** — New `internal/services/gateway/tribunal_store_service.go` providing `TribunalPolicy` CRUD operations with validation (quorum, member verification, duplicate detection).
* **Tribunal Admin Endpoints** — Admin-only REST endpoints for managing tribunal policies: `POST/GET /api/v1/admin/tribunals` and `DELETE /api/v1/admin/tribunals/{id}`.
* **Tribunal Deliberate Route** — mTLS-protected `POST /tribunal/v1/deliberate` route on the gateway for remote tribunal deliberation.
* **L2Vote/L2Metadata Protocol** — New protocol types replacing the old `gateway_signed` boolean with typed L2 vote structures containing `tribunal_id` and `votes` fields.
* **TribunalStore Interface** — New `TribunalStore` interface in `l4_warden.go` for loading `TribunalPolicy` by ID during L4 quorum verification.
* **CLI Flags & Env Vars** — `--tribunal-id` / `--tribunal-url` flags and `G8E_TRIBUNAL_ID` / `G8E_TRIBUNAL_URL` environment variables for configuring the gateway's tribunal integration.
* **Startup Validation** — Gateway validates that `--consensus` posture requires a non-empty `--tribunal-id`; `bootstrapTribunal` returns error if `TribunalPolicy` not found in DB.
* **Comprehensive Test Coverage** — 18 tribunal service unit tests, 11 MCP consensus integration tests, and 4 local deliberator tests covering all deliberation paths, error scenarios, and cryptographic signature verification.

### Changed

* **L2 Consensus Architecture** — Replaced `internal/services/governance/l2_consensus.go` with `internal/services/tribunal/service.go`. The gateway no longer self-signs L2 votes; instead, it calls the Tribunal's `Deliberate` method and attaches returned L2 votes to the envelope.
* **L4 Warden Quorum Verification** — `verifyL2Posture` in `l4_warden.go` now uses the `TribunalStore` interface to load the `TribunalPolicy` and verify the quorum of valid L2 votes against the policy's member list and quorum threshold.
* **Documentation** — Updated `README.md`, `docs/architecture/gateway.md`, `docs/architecture/protocol.md`, `docs/devs/codemap.md`, `docs/core/position_paper.md`, `docs/guides/build_apps.md`, `docs/guides/build_operator.md`, and `docs/protocols/mcp/mcp.md` to reflect the Tribunal architecture.

### Removed

* **`gateway_signed` Field** — Removed the `gateway_signed` boolean from `GovernanceMetadata` proto and all references in code, documentation, and examples.
* **`l2_consensus.go`** — Deleted `internal/services/governance/l2_consensus.go` and all dead `L2Consensus` wiring from `pubsub_commands.go` and `secret_manager.go`.
* **`ErrTxL2KeyNotConfigured`** — Replaced with tribunal-specific error handling.

---

## [1.1.6] - 2026-06-22

### Overview

v1.1.6 is a major reporting, compliance, and platform stability release. This version introduces a comprehensive reporting infrastructure for audit and compliance, standardizes cross-platform process management (specifically for Windows), and hardens network security by requiring HTTPS for critical operations. This release also features significant improvements to SSH error handling and retry logic, centralized architecture constants, and a massive expansion of test coverage for reporting and storage services.

### Added

* **Audit and Compliance Reporting** — Introduced a new reporting service suite in `internal/services/reporting/` capable of generating CSV exports for ledger commits, file diffs, and nonces to support governance audits.
* **Reporting CLI Capabilities** — Added `ListCommits`, `ListFileDiffs`, and `ListNonces` methods to storage services to support the new reporting infrastructure.
* **Network Error Patterns** — Added `TransientNetworkErrorPatterns` in `internal/constants/network.go` to provide more robust and intelligent retry logic for network-related operations.
* **Platform Constants** — Centralized architecture and OS constants (`ArchAMD64`, `ArchARM64`, `OSLinux`, `OSWindows`, etc.) for better maintainability and cross-platform consistency.
* **Standardized Path Constants** — Added standardized path constants for `/dev/null`, `/dev/zero`, and improved `/tmp` path handling.
* **SSH Error Types** — Added specific SSH context error types (`ErrSSHContextCancelled`, `ErrSSHRetryBackoffCancelled`) for more granular error handling in remote operations.

### Changed

* **Standardized Command Exit Codes** — Systematically replaced `nil` exit code pointers with a new `constants.ExitCodeNone` sentinel value across the storage and auditing layers for better data consistency.
* **Windows Process Management** — Refactored `ProcessManager` to use `uintptr` for Windows handles instead of `syscall.Handle`, improving abstraction and build compatibility.
* **HTTPS Requirement** — Hardened the security posture by requiring HTTPS for critical operations and removing the insecure `LocalHttpStdioGateway` (port 18789).
* **URL Constant Standardization** — Unified URL and endpoint constants into `GatewayHTTPBase` and `GatewayHTTPSBase` to reduce duplication.
* **SSH Retry Logic** — Enhanced SSH execution with improved retry logic and more descriptive error reporting, including captured remote stderr.
* **Gateway Code Quality** — Systematic refactoring of gateway components for better maintainability and performance.

### Fixed

* **Windows Builds** — Resolved critical build issues on Windows platform (#156).
* **Process Manager** — Fixed various issues in process lifecycle management across platforms.
* **Path Resolution** — Improved path handling and resolution in reporting and storage services.
* **Reporting Accuracy** — Fixed issues with data truncation and timestamp parsing in audit reports.
* **URL Cleanup** — Removed redundant and inconsistent URL definitions across the codebase.

### Removed

* **Local HTTP Stdio Gateway** — Removed the insecure `LocalHttpStdioGateway` (port 18789) as part of the move towards mandatory HTTPS.

## [1.1.5] - 2026-06-20

### Overview

v1.1.5 is a code quality and test coverage release that focuses on error typing improvements, comprehensive test coverage expansion, and codebase cleanup. This release dissolves unnecessary abstractions, centralizes posture definitions, and significantly improves test coverage across CLI, pubsub, network, storage, and history handler components.

### Changed

* **Error Typing Improvements** — Systematically improved error typing across governance and storage services for better error handling and consistency.
* **Test Coverage Expansion** — Significantly improved test coverage for CLI auth, pubsub, network operations, storage services, and history handler.
* **Emulator Reorganization** — Moved agent harness to test directory for better code organization.
* **Scenario Test Dissolution** — Dissolved scenario tests into standard integration tests for improved maintainability.
* **Codebase Cleanup** — Removed unnecessary utilities (slices, sliceutil), unused mocks, and dissolved unnecessary interfaces.
* **Posture Definitions Centralization** — Centralized posture definitions and removed duplicate code.
* **Signal Definitions Improvements** — Improved signal definitions for better clarity and consistency.
* **Constants Cleanup** — Cleaned up constants and removed deprecated entries.
* **HTTP Client Directory Cleanup** — Reorganized HTTP client directory structure.

### Fixed

* **Test Errors** — Fixed various test errors and improved test reliability across multiple test suites.
* **Lint Issues** — Addressed linting issues identified by static analysis tools.
* **Code Formatting** — Applied gofmt and standardized code formatting.

## [1.1.4] - 2026-06-19

### Overview

v1.1.4 is a code quality and test coverage release that significantly improves the reliability and maintainability of the g8e platform. This release focuses on comprehensive MCP tool test refactoring, integration test improvements, database error handling cleanup, and cross-platform path handling enhancements.

### Added

* **Comprehensive MCP Tool Tests** — Added extensive unit tests for file read operations, disk usage monitoring (including Windows-specific implementations), and network socket auditing.
* **Network Socket Audit Tool** — Added new `net_socket_audit` MCP tool for inspecting active network connections and socket states.
* **Field Parser Enhancements** — Enhanced field parsing capabilities with improved validation and error handling.

### Breaking Changes

* **`emulator` renamed to `agent-harness`** - The emulator CLI command and internal package are renamed to `agent-harness` for clarity. Directory renamed from `internal/emulator` to `internal/agent_harness`, CLI command changed from `g8e emulator` to `g8e agent-harness`, and all references in code, documentation, and configuration files are updated accordingly.
* **`local_http_stdio` (formerly `insecure_mcp`) mode removed** - The ungoverned local MCP node mode is removed entirely. It connected to an MCP gateway over WebSocket and executed `system.run`/`system.which` with no L1/L2/L3 verification — an unconditional governance bypass. All local MCP traffic now goes exclusively through the governed mTLS/HTTPS gateway surface (`g8e mcp stdio`). The `--local-http-stdio*` flags, `LocalHttpStdioGateway` port (18789), service package, config loader, and related constants are deleted.

### Changed

* **MCP Tool Refactoring** — Systematic refactoring of MCP native tools for improved testability and dependency injection support.
* **Integration Test Improvements** — Refactored integration tests for better separation of concerns and improved reliability.
* **Database Error Handling** — Cleaned up and standardized database error handling across storage services.
* **Keystore Service Improvements** — Enhanced keystore backend implementations with better error handling and cross-platform support.
* **Gateway State Root Query** — Fixed and improved gateway state root query functionality with associated test improvements.
* **Demo SOC Refactoring** — Refactored demo configurations for cleaner separation of concerns (SOC).
* **Bootstrap Function Split** — Improved bootstrap function organization by splitting responsibilities.

### Fixed

* **Windows Path Handling** — Improved Windows path handling across multiple services for better cross-platform compatibility.
* **Demo Error Handling** — Fixed error handling in demo configurations.
* **Code Quality** — Applied gofmt and addressed various code quality issues identified by linters.
* **Pubsub Commands** — Improved pubsub command handling and results processing.

## [1.1.3] - 2026-06-17

### Overview

v1.1.3 introduces a new governed migration command suite, enhances auth enrollment flows, and improves storage reliability. This release focuses on expanding migration capabilities and hardening core notary approval flows.

### Added

* **Governed Migration Suite** — New `migration` CLI command suite to manage governed bulk data transfers, including manifest signing and connectors for rclone and SharePoint.
* **Notary Approval Expiry** — Implemented automated audit and cleanup for expired suspended transactions awaiting notary approval.
* **Migration Transfer Action** — Added support for `MIGRATION_TRANSFER` action types.

### Changed

* **Auth Enrollment Flow** — Refactored and improved the agent enrollment flow for better usability.
* **L3 Posture Hardening** — Removed deprecated L3 bypass mechanisms to enforce stricter security posture.
* **Documentation** — Updated architecture and guide documentation to reflect migration suite and new auth flows.

### Fixed

* **Storage Reliability** — Fixed various issues in the storage layer related to transaction handling and auditing.
* **Swagger Documentation** — Updated OpenAPI/Swagger documentation to reflect current API endpoints.

## [1.1.2] - 2026-06-16

### Overview

v1.1.2 is a maintenance release focused on documentation cleanup, dependency updates, and build infrastructure improvements. This release removes deprecated invitation service references from documentation, reorganizes community health documentation into the standard `.github/` location, improves binary building processes, and updates critical Go dependencies for security and stability.

### Changed

* **Documentation Cleanup** — Removed all references to the deprecated invitation service from documentation files.
* **Community Health Documentation** — Moved community health documentation into `.github/` directory for standard GitHub project structure compliance.
* **Binary Building Improvements** — Enhanced binary build processes for better cross-platform compatibility and reliability.

### Fixed

* **Dependency Updates** — Bumped critical Go dependencies:
  * `golang.org/x/sys` from 0.45.0 to 0.46.0
  * `golang.org/x/crypto` from 0.52.0 to 0.53.0
* **GitHub Actions** — Updated `softprops/action-gh-release` from version 2 to 3 for improved release workflow reliability.

---

## [1.1.1] - 2026-06-15

### Overview

v1.1.1 is a comprehensive maintenance and stability release that significantly hardens the g8e platform. This release features a major architectural decomposition of the gateway's authentication controller, a systematic refactoring of MCP tools for improved testability, and a massive push for integration test coverage. Additionally, this release introduces full OpenAPI/Swagger documentation for the gateway and resolves critical data races in core services.

### Added

* **OpenAPI/Swagger Documentation** — Added complete Swagger and OpenAPI specifications for all gateway REST endpoints.
* **Docker E2E Test Harness** — Introduced a new, robust end-to-end testing harness with dedicated Docker fixtures.
* **New Test Scenarios** — Added comprehensive concurrency and scenario-based tests.

### Changed

* **Auth Controller Decomposition** — Refactored the monolithic `AuthController` into specialized components (`approvals`, `bootstrap`, `passkey`, `session`).
* **MCP Tool Testability Refactor** — Systematic refactoring of native MCP tools to support dependency injection for unit testing.
* **Test Coverage Improvement** — Significantly increased gateway test coverage to over 65%.
* **Documentation Overhaul** — Updated and refined almost all user guides.
* **PubSub Command Expansion** — Expanded internal pubsub command handlers for more robust orchestration.
* **Scrubbing Logic Enhancement** — Improved data sanitization logic in the scrubber service.

### Fixed

* **Execution Service Race Conditions** — Resolved critical data races in the execution service.
* **Gateway Data Races** — Fixed multiple race conditions in the gateway core.
* **Docker Deployment Reliability** — Resolved orchestration issues in Docker configurations.
* **Heartbeat Test Stability** — Eliminated flakiness in MCP heartbeat tests.
* **Protocol Generation** — Fixed `make proto` command and improved protocol buffer generation.

### Removed

* **Deprecated Invitation Service** — Cleaned up the legacy invitation service.
* **Database Migration Cruft** — Removed obsolete database migration logic and unused SQL utilities.

---

## [1.1.0] - 2026-06-13

### Overview

v1.1.0 is a major agentic tool expansion release that dramatically broadens g8e's compatibility with the AI agent ecosystem. This release adds support for 13 additional AI agent binaries beyond Claude Code, including Cursor, Devin, VSCode, Continue, Aider, Codeium, Tabby, Ollama, Gemini, and Goose. The MCP CLI has been significantly refactored to provide a unified `agent` command for seamless integration across all supported environments. Additionally, this release includes gateway service cleanup, URL constant standardization, comprehensive documentation updates including a new position paper, and simplified build processes.

### Added

* **Universal Agent Support** — Expanded agent binary support from 2 to 15 agents with new constants in `internal/constants/agents.go` and `protocol/constants/agents.json`:
  - **IDE Agents**: Cursor, Devin, VSCode, Continue (with `cn` alias), Aider, Codeium, Tabby
  - **Model Servers**: Ollama, Gemini
  - **Specialized Tools**: Goose, Generic
  - **Existing**: Claude, Codex
* **Agent Persona System** — Added typed constants for agent personas (`AgentNameSage`, `AgentNameDash`) to support different agent behavioral profiles.
* **Triage Classification System** — Added comprehensive triage classification constants for agent behavior analysis:
  - `TriageComplexity` (simple, complex)
  - `TriageConfidence` (high, low)
  - `TriageIntent` (information, action, unknown)
  - `TriagePosture` (normal, escalated, adversarial, confused)
* **Unified MCP Agent Command** — New `./g8e mcp agent` command provides a single entry point for running g8e as an MCP server across all supported agent environments with automatic environment variable injection and configuration.
* **Network Constants** — Added `internal/constants/network.go` with standardized network-related constants for improved consistency across the codebase.
* **PubSub Commands** — Added `internal/services/pubsub/pubsub_commands.go` with comprehensive pubsub command handling for improved service orchestration.
* **MCP Backup Commands** — Added `internal/cli/cmd/mcp_backup.go` with backup and restore functionality for MCP configurations.
* **Updated Position Paper** — Updated `docs/core/position_paper.md` (156 insertions, 73 deletions) with refined architectural philosophy and governance documentation.

### Changed

* **MCP CLI Refactoring** — Major refactoring of `internal/cli/cmd/mcp.go` (367 lines changed) to:
  - Consolidate agent-specific logic into a unified `agentCmd()`
  - Add `getSupportedAgents()` helper function to reduce code duplication
  - Improve binary path validation with `os.Stat()` checks
  - Standardize configuration output across all agent types
* **Gateway Service Cleanup** — Fixed interface mismatches and removed divergence in gateway service implementation (`internal/services/gateway/gateway_service.go`) for consistency and maintainability.
* **URL Constant Standardization** — Cleaned up and standardized URL constants across the codebase to eliminate duplication and improve maintainability.
* **Build Process Simplification** — Removed UPX compression from Makefile (86 lines removed) to simplify the build process and improve compatibility.
* **Documentation Reorganization** — Removed redundant build guides (`docs/guides/build_gateway.md`, `docs/guides/build_operator.md`) and simplified `docs/guides/getting_started.md` to reduce documentation surface area.
* **Compliance Documentation Update** — Updated `docs/reference/compliance-alignment.md` to reflect the expanded agent support and updated security posture.
* **Gateway CLI Improvements** — Enhanced `internal/cli/cmd/gateway.go` (250 lines added) with improved command structure and error handling.
* **Platform Process Handling** — Improved `internal/cli/platform/process.go` with better cross-platform process lifecycle management.

### Fixed

* **Interface Mismatches** — Fixed interface mismatches between gateway service implementations and their contracts to ensure consistent behavior across modes.
* **Gateway Service Divergence** — Resolved divergence in gateway service code that had accumulated across different operational modes.
* **Test Reliability** — Fixed various test failures across integration and unit test suites to improve CI/CD reliability.
* **CLI Bug Fixes** — Addressed multiple CLI bugs including configuration validation, path resolution, and error messaging issues.

### Security

* **Agent Binary Validation** — Added binary path validation using `os.Stat()` in agent configuration to prevent execution of non-existent or unauthorized binaries.
* **Environment Variable Injection** — Standardized and secured environment variable injection for agent sessions with explicit contracts documented in `mcp.go`.

### Testing

* **MCP Backup Tests** — Added comprehensive test coverage for MCP backup and restore functionality (`internal/cli/cmd/mcp_backup_test.go`).
* **Gateway Test Updates** — Updated gateway integration tests to reflect the refactored service architecture and fixed test fixture issues.
* **Agent Integration Tests** — Improved agent integration test coverage across the expanded set of supported agents.

### Documentation

* **Updated README** — Significantly expanded `README.md` (256 lines added) with improved getting started guidance, agent support matrix, and architectural overview.
* **CLI Documentation** — Updated CLI documentation to reflect the new `agent` command structure and expanded agent support.
* **Simplified Guides** — Removed redundant build guides and consolidated getting started information for a cleaner documentation hierarchy.

---

## [1.0.12] - 2026-06-11

### Overview

v1.0.12 is a foundational refactoring release focused on security fixes, architectural cleanup, and performance improvements. This release fixes a critical governance bypass in the `read_field` tool, completes the CanonicalDBService service extraction to eliminate delegation wrappers, consolidates duplicate interface definitions, adds auth caching for performance, and updates documentation to reflect the new architecture.

### Security

* **Fixed `read_field` Governance Bypass** — The `read_field` tool previously bypassed L4 Warden replay-protection and L5 Actuator signed receipt generation by handling execution before the full governance pipeline. Now routes through `processGatewayTransaction()` and `ProcessEnvelope()` like all other tools, ensuring replay protection applies and signed receipts are generated for all reads.
* **Removed Double-Auditing for Native Tools** — Native tools were producing two audit records (a raw `auditStore.RecordEvent` plus the L5 signed receipt). Removed the raw event so native tools produce exactly one canonical audit record per call (the L5 receipt).

### Architecture

* **Completed CanonicalDBService Service Extraction** — Reduced `gateway_db.go` from 1759 lines to 478 lines (lifecycle-only code). All domain logic extracted to dedicated service files with no delegation wrappers:
  - `DocumentStoreService` — Document CRUD operations (implements `TransactionAuditStore`)
  - `AppPolicyStoreService` — App policy retrieval (implements `AppPolicyStore`)
  - `SignerStoreService` — Trusted signer CRUD (implements `SignerStore`)
  - `StateRootService` — State merkle root calculation with caching (implements `StateRootProvider`)
  - `ReplayStoreService` — Nonce replay protection (implements `ReplayStore`)
  - `KVStoreService` — TTL-aware ephemeral state with GLOB pattern scanning
  - `SSEEventService` — Server-Sent Events fan-out
  - `BlobStoreService` — Binary persistence for attachments and certificate material
* **Consolidated SuspendedTransactionStore Interface** — Removed duplicate `SuspendedTransactionStore` interface definition from `mcp/gateway.go`. Both gateway and outbound modes now use the canonical `interfaces.SuspendedTransactionStore` from the `interfaces` package.
* **Migrated Gateway Mode to `storage.SuspendedTransactionService`** — Gateway mode now uses `storage.SuspendedTransactionService` for L3 approval workflow, consistent with outbound mode. `auth_controller.go` holds the `SuspendedTransactionStore` interface reference instead of coupling to the concrete `CanonicalDBService` type.
* **Delegated Maintenance Loop** — `CanonicalDBService.RunMaintenance` now delegates to extracted services (`KVStore.RunMaintenance()`, `BlobStore.RunMaintenance()`, `ReplayStore.CleanupExpiredNonces()`) instead of direct SQL operations.

### Performance

* **Auth Caching** — Added `sync.Map`-based cache to `AuthService` for user lookups with 5-minute TTL. Wrapped user lookups in `ValidateOperatorSession`, `handleCLIAuth`, `WebSessionAuth`, and `JWTAuthMiddleware`. Added cache invalidation hooks in `UserService.Disable` and `UserService.DeleteUser`. Benchmark results: ~40-116ns operations (far below 5ms target).

### Code Quality

* **Removed Unused Code** — Removed unused `sessionCache` field and related methods from `AuthService` (only user cache is used). Removed unused `cachedStateRoot` and `cachedStateVersion` fields from `CanonicalDBService` (state root caching moved to `StateRootService`).
* **Fixed Lint Issues** — Fixed all `golangci-lint` issues including error return value checks for `json.NewEncoder.Encode` calls and formatting issues across modified files.

### Testing

* **Integration Test Updates** — Updated all integration tests to use extracted service fields directly instead of removed delegation wrapper methods. Fixed test infrastructure to use `storage.SuspendedTransactionService` instead of fake implementations.
* **Governance Pipeline Verification** — Verified that all tools (native, `read_field`, and downstream) pass through the full L1-L5 governance pipeline with proper replay protection and signed receipt generation.

### Documentation

* **Updated `docs/architecture/gateway.md`** — Added entries for all 8 extracted services (DocumentStore, AppPolicyStore, SignerStore, StateRoot, ReplayStore, KVStore, SSEEvent, BlobStore) and SuspendedTransactionStore to the Implementation Reference table.
* **Updated `docs/devs/codemap.md`** — Updated GatewayModeService dependency tree to reflect extracted service architecture with no delegation wrappers. Updated Data Handling Convergence section to document direct field access pattern. Updated Shared Interface Implementations to remove CanonicalDBService (no longer implements interfaces). Updated Critical Data Flows to reflect consistent use of `storage.SuspendedTransactionService` in both modes.

---

## [1.0.11] - 2026-06-10

### Overview

v1.0.11 introduces a comprehensive multi-industry demo suite (Finance, Government, Healthcare) with dedicated CLI commands for orchestrated industry-specific scenarios. It significantly hardens the gateway and operator bootstrap processes, resolves certificate generation race conditions, and introduces a dedicated secret manager for secure internal credential handling. This release also includes a major expansion of end-to-end testing coverage and numerous fixes for logging and cross-process communication.

### Added

* **Multi-Industry Demo Suite** — New industry-specific demo scenarios for Healthcare (HIPAA/PHI), Finance (Trading Controls), and Government (CUI/CMMC), including Docker Compose orchestration and mock data generators.
* **Demo CLI Commands** — New `./g8e demos` command group for listing, starting, and managing demo environments.
* **Gateway Secret Manager** — Dedicated service for secure internal credential and secret handling within the gateway.
* **Enhanced E2E Tests** — Significant expansion of end-to-end testing infrastructure and coverage across gateway and operator services.
* **Gateway PubSub Capabilities** — Expanded internal pubsub functionality for better service orchestration.

### Changed

* **Operator Bootstrap Hardening** — Redesigned operator bootstrap sequence to be more resilient to environment wipes and certificate expiration.
* **Certificate Generation** — Refactored gateway certificate authority logic to eliminate race conditions during concurrent service startup.
* **Config Handling** — Improved configuration validation and default path resolution for better cross-platform reliability.

### Fixed

* **Bootstrap Failures** — Fixed critical failures when re-bootstrapping an operator after a `.g8e` directory wipe.
* **Logging Duplication** — Resolved issues where log entries were being duplicated across different output streams.
* **PEM Generation** — Fixed gateway PEM generation bugs that could lead to invalid certificate chains in certain network configurations.
* **MCP Test Stability** — Improved stability and reliability of MCP-related unit and integration tests.

---

## [1.0.10] - 2026-06-08

### Overview

v1.0.10 is a major release that hardens the platform's security posture, simplifies operational complexity, and significantly expands cross-platform support. The release introduces mandatory encryption at rest via a vault subsystem, consolidates gateway networking from four ports to two, adds comprehensive Windows Hello / passkey authentication, delivers a full suite of operator remote-management commands, and completes native Windows compatibility. Under the hood, the storage layer has been fundamentally refactored for clarity and fail-closed behavior, and the gateway architecture has been decomposed into dedicated controllers and services.

### Breaking Changes

* **Mandatory encryption at rest** — Encryption is now required for all storage services. Previously, vault parameters were optional and production deployments could run without encryption, storing sensitive data unencrypted. This is a critical security fix.
  * `NewSQLAuditStore` now requires `EncryptionVault` in config and returns an error if nil.
  * `NewExecutionVaultService` now requires a vault parameter and returns an error if nil.
  * `NewTokenStoreService` now requires a vault parameter and returns an error if nil.
  * Production initialization in `g8eo.go` and `gateway_db.go` now initializes and unlocks the vault before creating storage services.
  * All nil-checks removed from encryption paths; fail-closed behavior is enforced.

* **Port consolidation** — Gateway port configuration reduced from 4 ports to 2 ports:
  * Removed: `--bootstrap-listen-port` (8441) and `--mcp-http-port` (8442).
  * New: `--http-port` (8080) for bootstrap and MCP routes.
  * New: `--https-port` (8443) for mTLS API and public surface.

* **Audit vault refactor** — The monolithic `AuditVaultService` has been split into three cleanly separated concerns:
  * `SQLAuditStore` (`audit_store.go`) — pure SQL audit data storage.
  * `GitLedgerService` (`ledger.go`) — pure git-backed file versioning.
  * `HistoryHandler` (`history_handler.go`) — composes both for history queries.
  * `AuditVaultService` deleted from production code.

* **Dead scrubbing code removed** — Removed unused `TextScrubber` interface and `SetScrubber()` method.

* **Auditor renamed to emulator** — The CLI command was renamed from `./g8e auditor` to `./g8e emulator` for clarity.

### Migration Guide

**For existing deployments:**

1. **Initialize the vault:**
   ```bash
   ./g8e vault init
   ```

2. **Generate or import a vault key:**
   ```bash
   # Generate new key
   ./g8e vault key generate

   # Or import existing key
   ./g8e vault key import <path-to-key>
   ```

3. **Unlock the vault before starting services:**
   ```bash
   ./g8e vault unlock
   ./g8e gw start
   ```

4. **Review port configuration:**
   - If you previously customized `--bootstrap-listen-port` or `--mcp-http-port`, update your scripts to use `--http-port` (8080) instead.
   - Verify firewall rules allow traffic on the consolidated ports.

**For new deployments:**
- Vault initialization is now part of the standard setup flow.
- Follow the updated `docs/guides/build_operator.md` for complete setup instructions.

### Added

* **Vault CLI commands** — Complete vault management CLI:
  * `./g8e vault init` — Initialize vault.
  * `./g8e vault unlock` — Unlock vault with key.
  * `./g8e vault key generate` — Generate new vault key.
  * `./g8e vault key import` — Import existing vault key.
  * `./g8e vault key export` — Export vault key.
  * `./g8e vault re-key` — Re-key vault with new key.
  * `./g8e vault status` — Check vault status.
  * `./g8e vault reset` — Destructive vault reset.

* **Passkey authentication** — WebAuthn / Windows Hello passkey support for operator bootstrap:
  * `internal/cli/auth/passkey_bootstrap.go` — platform-agnostic passkey bootstrap flow.
  * `internal/services/gateway/passkey_service.go` — gateway-side passkey verification and session creation.
  * `internal/services/gateway/auth_controller.go` — dedicated controller for authentication endpoints.

* **Operator remote management commands** — New CLI commands for managing remote operators:
  * `./g8e operator cp` — Copy files to/from a remote operator.
  * `./g8e operator scp` — Secure copy with recursive support.
  * `./g8e operator ssh` — Open an SSH session to a remote operator.
  * `./g8e operator stream` — Stream logs or events from a remote operator.
  * `./g8e operator deploy` — Deploy the gateway to a remote host via SSH (new `operator_deploy` MCP tool and PowerShell/Bash deploy scripts).

* **Agent integration commands** — New `./g8e agent` command group for AI agent IDE integration:
  * `./g8e agent` — Launch the agent gateway with stdio transport.
  * Platform-specific browser helpers for automatic IDE launch (`internal/cli/platform/browser.go`).
  * Dedicated support for Claude Code and compatible agent environments.

* **Setup wizard** — New `./g8e setup` command provides an interactive, guided setup flow for first-time users, covering vault initialization, key generation, and gateway configuration.

* **Multi-Transport Configuration Command** — `./g8e mcp show` updated to output a matrix of different configurations side-by-side: `g8e.local` (mTLS), direct IP address (without DNS required), plain HTTP (for localhost access), and stdio transport.

* **Simplified Stdio MCP Config** — Simplified stdio transport configuration format output via standard JSON structure (`command`, `args`, `disabled`) compatible with standard IDE-assisted clients (Cursor, Devin).

* **Cross-Platform Setup Scripts** — New cross-platform environment bootstrapper and dependency validation scripts added under `scripts/linux-setup.sh`, `scripts/macos-setup.sh`, and `scripts/windows-setup.ps1` to easily verify Go, Buf, and Protoc compilers.

* **Consolidated CLI & Client Errors** — New error packages `internal/cli/errors/errors.go` and `internal/constants/errors.go` standardized error definitions (e.g. `ErrNotAuthenticated`, `ErrTrustBundleStale`).

* **MCP governance command** — `./g8e mcp gov` exposes governance capabilities through the MCP interface, allowing agents to inspect and interact with the governance ledger.

* **Gateway health endpoint** — `/api/v1/health` added to the HTTP router for load-balancer and orchestrator health checks.

* **Docker gateway support** — Production-ready `Dockerfile` and `docker-compose.yml` for running the gateway in containerized environments, with dedicated documentation in `docs/guides/docker_gateway.md`.

* **Windows disk usage tool** — `fs_disk_usage` now works natively on Windows via PowerShell fallback.

* **SSH known-hosts helper** — `internal/services/mcp/net_ssh_known_hosts.go` for managing and validating SSH host keys during operator remote operations.

* **Vault configuration** — Added vault settings to both Operator and Gateway modes:
  * `VaultDir` — Vault data directory path.
  * `VaultKeyPath` — Path to vault private key.
  * Environment variables: `G8E_VAULT_DIR`, `G8E_VAULT_KEY`.
  * CLI flags: `--vault-dir`, `--vault-key`, `--vault-require-unlock`.

* **Token store service** — New `internal/services/storage/token_store.go` with encrypted KV storage and comprehensive test coverage (`token_store_test.go`).

* **Execution vault service** — New `internal/services/storage/execution_vault.go` providing encrypted storage for command stdout/stderr and file diffs.

* **Suspended transaction store** — New `internal/services/storage/suspended_transaction_store.go` for durable, encrypted suspended-transaction storage.

* **Gateway session services** — Decomposed monolithic session handling into dedicated services:
  * `cli_session_service.go`
  * `web_session_service.go`
  * `operator_session_service.go`

* **Gateway state-root service** — New `internal/services/gateway/state_root_service.go` with incremental state tracking and test coverage.

* **Gateway replay store** — New `internal/services/gateway/replay_store_service.go` for deterministic replay of governance events.

* **Comprehensive encryption documentation** — New `docs/architecture/encryption.md` with complete encryption architecture overview.

* **Network architecture documentation** — New `docs/architecture/network.md` documenting the 2-port gateway networking model.

* **Service modes documentation** — New `docs/architecture/service_modes.md` explaining Operator vs Gateway service modes.

* **Reference JSON schema** — New `docs/reference/schema.json` (4,000+ lines) providing a machine-readable schema for protocol constants and models.

### Changed

* **Gateway networking** — Consolidated from 4 ports to 2 ports. Bootstrap and MCP routes now share the HTTP port (8080); mTLS API and public surface share the HTTPS port (8443).

* **Storage layer refactor** — Major refactor of the storage subsystem:
  * `TokenStoreService.KVScanPrefix` now decrypts values (previously returned encrypted ciphertext).
  * Removed dead `TextScrubber` dependency from `ExecutionVaultService`.
  * Chaos test infrastructure moved to `test/chaos/`.

* **Gateway architecture** — Decomposed gateway HTTP handling into dedicated controllers:
  * `AuthController` — authentication, enrollment, and session management.
  * `PKIController` — certificate lifecycle and revocation.
  * `DBController` — database operations and streaming.
  * `RegistrationService` — operator registration and binding.
  * `DocumentStoreService` — structured document storage.
  * `AppPolicyStoreService` — application policy management.
  * `SignerStoreService` — signing key storage.

* **Auth client** — `internal/cli/auth/client.go` expanded with passkey flows, Windows crypto integration, and improved error definitions.

* **Thread-safe Deploy Script Templates** — Parser for deployment scripts (`internal/services/mcp/operator_deploy.go`) utilizes `sync.Once` to guarantee thread-safe script compilation under concurrent execution conditions.

* **Windows compatibility** — Extensive Windows-specific improvements:
  * `internal/cli/auth/windows_crypto.go` significantly expanded for Windows Hello integration.
  * `internal/cli/platform/process_windows.go` improved for gateway process lifecycle.
  * `internal/services/system/system_utils_windows.go` updated for path and identity handling.
  * One-line enrollment flow for Windows operators.

* **CLI test command** — `./g8e test` command removed in favor of Makefile-based test execution (`make test`).

* **Build system** — Makefile improved with OS-aware builds, compressed binary support, and Windows build fixes.

* **Documentation refactor** — Major documentation reorganization:
  * `docs/architecture/g8e.md` renamed to `docs/architecture/protocol.md`.
  * `docs/devs/AGENTS.md` removed; content merged into CLI and architecture docs.
  * `docs/devs/codemap.md` significantly updated.
  * All port references updated across guides and architecture docs.
  * `docs/reference/compliance-alignment.md` updated to reflect mandatory encryption.

* **Test infrastructure** — Consolidated and renamed test utilities for clarity:
  * `internal/testutil/crypto.go` → `test_crypto.go`
  * `internal/testutil/helpers.go` → `test_infrastructure.go`
  * `internal/testutil/proto_helpers.go` → `test_proto.go`
  * `internal/testutil/pubsub.go` → `test_pubsub.go`
  * `internal/testutil/governance_mocks.go` → `test_governance_mocks.go`

* **Protocol Python package** — Simplified Python protocol package; removed redundant model files in favor of a streamlined structure.

### Fixed

* **Windows gateway startup** — Fixed remaining Windows gateway startup issues with PowerShell command syntax and process management.
* **Operator enrollment** — Fixed operator enrollment flow to properly handle trust-bundle download before certificate enrollment, eliminating race conditions.
* **PID tracking** — Fixed gateway PID tracking in `internal/cli/platform/process.go` to correctly detect running gateway processes across platforms.
* **Gateway stop** — Fixed `./g8e gw stop` to reliably terminate the gateway process on both Unix and Windows.
* **HTTP port default** — Fixed critical bug where HTTP port default was incorrectly set to `OperatorHttps` (8443) instead of `OperatorHttp` (8080).
* **MCP test errors** — Fixed multiple MCP test failures and improved test coverage for native handlers and gateway integration.
* **A2A gateway tests** — Fixed A2A gateway integration tests for compatibility with the 2-port model.
* **Unit test parallelism** — Cleaned up test parallelism issues to prevent race conditions across the suite.
* **Linting issues** — Fixed various linting issues across 467 changed files.
* **Code quality** — Extensive code quality cleanup across the entire codebase, including standardized error definitions in `internal/constants/errors.go`.
* **Stale file handles in ledger streaming** — Git ledger file-copying operations (`copyToLedger` in `ledger.go`) now leverage deferred, error-aware close hooks on files to avoid potential file descriptor leaks on errors.
* **Graceful SSH known-hosts parsing** — Skip missing or non-existent configuration or host files gracefully instead of returning blocking errors, preventing startup failures in clean environments.

### Security

* **Critical security fix — mandatory encryption** — Encryption is now mandatory for all storage services. Previous versions could run without encryption, storing sensitive data (command stdout/stderr, file diffs, content) unencrypted at rest.
* **Fail-closed behavior** — Storage services now fail to initialize if the vault is not provided or cannot be unlocked. Encryption operations fail if the vault is locked.
* **Passkey authentication** — WebAuthn / Windows Hello passkeys provide phishing-resistant authentication for operator bootstrap, replacing weaker password-based flows.
* **Input validation** — Enhanced input validation framework with additional validators for shell commands, cloud metadata, and Kubernetes operations.
* **mTLS boundary preserved** — mTLS surfaces still use `RequireAndVerifyClientCert` at the TLS layer; port consolidation does not weaken the security boundary.
* **Port collision prevention** — Gateway fails startup if incompatible surfaces are assigned to the same port.
* **PKI trust bundle handling** — Enhanced trust bundle download and validation to prevent man-in-the-middle attacks during enrollment.

* **CodeQL & Static Analysis Compliance** — Closed potential security risks identified by CodeQL. Re-engineered core execution pathways (`executeLocally`, `getContainerStatus`, `operator_deploy` remote commands) to avoid shell-interpreter injection hazards by explicitly separating binaries from arguments. Reinforced validation frameworks to verify path constructs (`validateFilePath`, `validateProcNetPath`) and URLs (`validateHTTPRequestURL`) against user-controlled manipulation.

### Removed

* **Docker container support** — Removed all Docker-related CLI commands, container status tools, and container runtime detection. The platform is now fully host-native. Note: Docker support for running the gateway itself (via Dockerfile and docker-compose.yml) remains available.
* **4-port gateway configuration** — Removed `--bootstrap-listen-port` and `--mcp-http-port` flags and their associated constants.
* **`./g8e test` command** — Use `make test` for test execution.
* **`TextScrubber` interface** — Removed dead `TextScrubber` interface and `SetScrubber()` method.
* **`internal/services/sqliteutil/migration.go`** — Removed unused migration infrastructure (directory removed).
* **Legacy local store** — Removed old `local_store.go` and `local_store_test.go` in favor of the refactored implementation.
* **Redundant Python protocol modules** — Removed redundant `__init__.py`, `constants.py`, and model files from `protocol/python/g8e_protocol/` in favor of a streamlined package structure.

---

## [1.0.9] - 2026-06-04

### Overview

v1.0.9 is a focused bug fix release addressing critical Windows startup issues and gateway audit vault functionality. This release ensures Windows users can successfully start the g8e gateway and fixes a critical issue in the operator's audit vault handling.

### Added

*   **Platform-Specific Process Handling** - Added platform-specific process implementations with `internal/cli/platform/process_unix.go` and `internal/cli/platform/process_windows.go` for improved cross-platform compatibility.

### Changed

*   **Go Version** - Bumped Go to version 1.26.4 for latest security patches and improvements.
*   **GitHub Workflows** - Updated GitHub Actions workflows for improved Windows compatibility and build reliability.
*   **Documentation** - Updated getting started guide with Windows-specific quick start instructions.

### Removed

*   **Docker Infrastructure** - Removed all Docker-related infrastructure including Docker CLI commands, Docker port constants, container status tools, and container runtime detection. The platform is now fully host-native with no container or virtualization dependencies.

### Fixed

*   **Windows Gateway Startup** - Fixed Windows gateway startup issue by correcting PowerShell command syntax from `&&` to `;` in the quick start documentation and implementation (#135). This resolves startup failures on Windows platforms.
*   **Gateway Audit Vault** - Fixed critical audit vault functionality in `cmd/operator/main.go` to ensure proper audit trail recording and vault operations (12 insertions, 1 deletion).

---

## [1.0.8] - 2026-06-02

### Overview

v1.0.8 focuses on expanding the MCP (Model Context Protocol) native tool ecosystem with 14 new tools, improving SSH streaming capabilities, standardizing naming conventions across the codebase, and addressing security and test reliability issues. This release adds comprehensive system introspection tools, enhances shell execution with multi-host support, and improves overall test coverage and documentation.

### Added

*   **Shell Execute Tool** - Added `shell_execute` native tool for secure shell command execution with multi-host support, input validation, and comprehensive error handling (`internal/services/mcp/shell_execute.go`).
*   **Cloud Metadata Tool** - Added `cloud_metadata` tool for retrieving cloud provider metadata from AWS, GCP, and Azure (`internal/services/mcp/cloud_metadata.go`).
*   **Git Operations Tool** - Added `git_ops` tool for git repository operations including status, log, diff, and branch information (`internal/services/mcp/git_ops.go`).
*   **Kubernetes Inspect Tool** - Added `k8s_inspect` tool for Kubernetes cluster inspection including pods, nodes, services, and deployments (`internal/services/mcp/k8s_inspect.go`).
*   **Filesystem Tools** - Added filesystem analysis tools:
    *   `fs_disk_usage` - Disk usage analysis and reporting
    *   `fs_file_checksum` - File integrity verification with checksums
*   **Network Tools** - Added `net_dns_resolve` tool for DNS resolution and lookup operations.
*   **Process Tools** - Added `proc_tree` tool for process tree visualization and analysis.
*   **System Tools** - Added comprehensive system monitoring tools:
    *   `sys_container_status` - Container runtime status and information
    *   `sys_env_vars` - Environment variable inspection
    *   `sys_info` - System information including OS, kernel, and hardware details
    *   `sys_service_status` - System service status checking
    *   `sys_time_clock` - System time and clock synchronization information
*   **TLS Certificate Inspection** - Added `tls_cert_inspect` tool for TLS certificate analysis and validation.
*   **SSH Stream Improvements** - Enhanced SSH streaming with improved error handling, connection management, and streaming capabilities (`internal/cmd/stream_ssh.go`).
*   **PubSub Test Coverage** - Added comprehensive test coverage for pubsub client operations and integration scenarios.

### Changed

*   **Naming Standardization** - Standardized naming conventions across 131 files in the codebase for consistency and maintainability, including documentation, internal services, and protocol models.
*   **Gateway Tool Handling** - Improved downstream gateway tool handling with better error propagation and tool registration.
*   **Native Tool Registry** - Extended native tool registry to support the expanded tool ecosystem.
*   **Validation Framework** - Enhanced input validation framework with additional validators for shell commands, cloud metadata, and Kubernetes operations.
*   **Documentation** - Updated CLI documentation to clarify auth login purpose and improved getting started guides.
*   **PKI Enrollment** - Clarified PKI enrollment process in documentation and improved error messaging.

### Fixed

*   **PKI Enrollment** - Fixed PKI enrollment issues in gateway security commands and improved operator-to-gateway connection documentation.
*   **Kubernetes Test Reliability** - Fixed `k8s_inspect` tests to skip when kubectl has no cluster access, preventing CI failures in environments without configured clusters.
*   **MCP Test Errors** - Fixed MCP test errors and improved test coverage for native handlers.
*   **String Replace Issues** - Fixed string replacement issues in various components.
*   **CodeQL Security Alerts** - Addressed CodeQL security alerts in MCP tools with additional input validation and security hardening.
*   **Test Parallelism** - Cleaned up test parallelism issues to prevent race conditions.
*   **Linting Issues** - Fixed various linting issues across the codebase.
*   **Healthcheck Improvements** - Improved healthcheck reliability and error reporting.

### Security

*   **Input Validation** - Enhanced input validation for shell_execute tool to prevent command injection attacks.
*   **Cloud Metadata Security** - Added validation for cloud metadata requests to prevent unauthorized access.
*   **Kubernetes Access Control** - Improved Kubernetes inspect tool with proper access validation and error handling.

---

## [1.0.7] - 2026-06-01

### Overview

v1.0.7 focuses on security hardening, performance optimizations, and significant expansion of the MCP (Model Context Protocol) native tool ecosystem. This release addresses CodeQL security alerts with comprehensive input validation, implements incremental state tracking for gateway performance, unifies the MCP endpoint architecture, and adds 12 new native tools covering database operations, filesystem analysis, network diagnostics, process management, and system monitoring.

### Added

*   **Input Validation Framework** - Created a comprehensive validation system (`internal/services/mcp/validation.go`) with fail-closed security principles for MCP tool inputs, including SQL query validation, URL validation, and protocol validation.
*   **Incremental State Tracking** - Implemented database schema (`internal/services/gateway/db/schema.sql`) and change tracking mechanisms to support incremental state root calculation, avoiding full recomputation and improving gateway performance.
*   **Unified MCP Endpoint** - Consolidated previously separate MCP implementations into a unified endpoint architecture (`internal/services/mcp/mcp_endpoint.go`) with improved test coverage using functional options pattern.
*   **Native Tool Registry** - Added `internal/services/mcp/native_tool_registry.go` to centralize native tool definitions and improve tool management.
*   **12 New Native Tools** - Expanded the native tool ecosystem with:
    *   Database tools: `db_discover_topology`, `db_index_triage`, `db_isolated_read`, `db_query_validate`
    *   Filesystem tools: `fs_disk_profile`, `log_stream_filter`
    *   Network tools: `net_endpoint_ping`, `net_http_probe`, `net_socket_audit`
    *   Process tools: `proc_metric_top`, `proc_signal_safe`
    *   System tools: `sys_oom_detect`
*   **Tool Template** - Added `docs/protocols/mcp/tool_template.go` as a reference for implementing new MCP tools.
*   **Comprehensive Test Coverage** - Added extensive test coverage for MCP endpoint, gateway HTTP operations, native handlers, and validation functions.

### Changed

*   **Gateway Database Operations** - Replaced materialization with streaming in gateway database operations for improved memory efficiency and performance.
*   **Health Endpoint Consolidation** - Unified health endpoint across auditor and gateway services for consistent health checking.
*   **Test Infrastructure** - Refactored `mcp_endpoint_test.go` and GatewayService test initialization to use functional options pattern for better test maintainability.
*   **Native Handlers Refactoring** - Refactored `internal/services/mcp/native_handlers.go` to use the new tool registry and modular tool implementations.
*   **Build System** - Made build OS-aware and improved binary handling with lazy copy to project root.
*   **CLI Improvements** - Simplified MCP config HTTP handling, fixed home directory path resolution bugs, and improved help text with no-flags support.
*   **Claude Code Support** - Enhanced gateway HTTP operations and added dedicated support for Claude Code integration.

### Fixed

*   **CodeQL Security Alerts** - Fixed security alerts in MCP tools by adding input validation:
    *   SQL query validation for `db_isolated_read` and `db_query_validate` tools
    *   URL validation for `net_http_probe` to prevent SSRF attacks
    *   Protocol validation for `net_socket_audit` to prevent path traversal
*   **Request Forgery Protection** - Enhanced validation tests to prevent request forgery attacks in native handlers.
*   **Tool List Handshake** - Fixed tool list handshake issues in MCP protocol communication.

---

## [1.0.6] - 2026-06-01

### Overview

v1.0.6 introduces full Windows support with native filesystem and service parity, significant improvements to gateway network identity detection, and the foundational PKI architecture for gateway-to-gateway federation. This release also standardizes `g8e.local` as the canonical internal mesh hostname and adds native support for MCP-over-HTTP.

### Added

*   **Native Windows Support** - Complete Windows implementation including native filesystem listing (`fs_list_windows.go`), process management, and service control. Added a one-line enrollment flow for Windows operators and specialized handling for Windows network identity (NetBIOS/AD FQDN).
*   **Gateway Peer Identity** - Introduced a new `gateway-peer` intermediate CA and SPIFFE URI SAN binding (`spiffe://g8e.local/gateway/<id>`) to support secure gateway-to-gateway federation.
*   **Canonical Mesh Addressing** - Standardized `g8e.local` as the internal hostname for operator-to-operator communication, supported by a new local translation layer and documentation.
*   **MCP-over-HTTP** - Added support for Model Context Protocol over HTTP on port 8442, including a new `mcp.Config` schema for environment-based TLS configuration.
*   **Network Identity Handoff** - Implemented a structured network identity detection system with cross-process handoff via `--network-identity-file`, allowing parent processes to pre-detect and pass network context to operators.

### Changed

*   **Build Pipeline** - Updated `Makefile` to support compressed builds (~15-17MB) for Linux/Windows and standard builds (~35-38MB). Refined build output for improved developer experience.
*   **Field Path Security** - Optimized `field_parser` to use a constant-backed schema registry, moving away from filesystem-dependent validation for tool call field paths.
*   **Architecture Documentation** - Updated the README and Getting Started guides to reflect current binary sizes, Windows QuickStart commands, and the 5-layer verification sequence.

### Fixed

*   **JSON Robustness** - Gateway controllers now strictly validate and reject malformed JSON bodies during Operator binding and configuration updates.
*   **Cross-Platform Nlink** - Fixed `nlink` handling in filesystem listings by implementing architecture-specific casting (uint16/uint32 to uint64) to ensure consistency across Linux, Unix, and Windows.
*   **Gateway Startup** - Resolved race conditions in gateway service initialization specifically affecting Windows environments.

---

## [1.0.5] - 2026-05-31

### Fixed

* **Version mismatch** - Fixed VERSION file to show correct version in binary. The v1.0.4 release was tagged before VERSION was updated, causing the binary to show v1.0.3. This release ensures VERSION is properly synchronized with the release tag.

---

## [1.0.4] - 2026-05-31

### Overview

v1.0.4 introduces MCP stdio transport for local agent integration, adds a complete Python protocol package, and significantly hardens PKI/certificate handling. The CLI is reorganized with gateway-focused commands, and test infrastructure is enhanced with universal gateway integration tests and flexible Docker testing options.

### Breaking Changes

* **CLI command renaming** - The `./g8e platform` command group is renamed to `./g8e gw` (gateway). All platform subcommands are now gateway subcommands (e.g., `./g8e platform start` → `./g8e gw start`). Documentation and tests updated to reflect the new command structure.
* **Scenario test fixture removal** - Hardcoded scenario test fixtures are removed in favor of programmatic fixture generation. The `test/scenario/fixtures/` directory and golden files are eliminated; fixtures are now generated dynamically via `envelope_builder.go`.

### Added

* **MCP stdio transport** - Full stdio-based MCP transport implementation with JSON-RPC handling. New `internal/cli/cmd/mcp.go` command enables local agent integration via stdio, complementing existing HTTP transport. Includes comprehensive JSON-RPC type definitions and validation.
* **Python protocol package** - Complete Python protocol implementation in `protocol/python/` with constants, models (base, context, events, internal_api, settings), and examples. Enables Python clients to use typed protocol definitions without Go dependencies.
* **Protocol examples** - Added three new protocol examples: governance envelope usage, MCP server configuration, and workload identity implementation. Examples demonstrate proper protocol usage for integration patterns.
* **Universal gateway integration test** - New `test/universal_gateway_integration_test.go` (707 lines) provides comprehensive integration testing across all gateway transports (A2A, MCP HTTP, MCP stdio) with shared test infrastructure.
* **PKI authority hardening** - Enhanced PKI authority with improved certificate enrollment, trust bundle handling, and comprehensive test coverage (490+ new test lines). Certificate fetch logic is hardened with better error handling and validation.
* **Docker flexible testing** - Added Docker-based testing options alongside native host testing. Enables CI/CD pipelines to choose between native and Docker test execution based on environment constraints.
* **CLI auth client** - New `internal/cli/auth/client.go` provides dedicated authentication client with comprehensive test coverage. Improves separation of concerns in CLI authentication flows.

### Changed

* **Gateway command reorganization** - All `platform` CLI commands renamed to `gw` (gateway) for clarity. This includes start, stop, status, and logs commands. The change better reflects the platform's gateway-first architecture.
* **mTLS auth flow** - Fixed mTLS authentication flow to properly pull down trust bundle before enrollment. Previous implementation had race conditions where enrollment could attempt before trust bundle was available.
* **Platform startup improvements** - Enhanced `./g8e gw start` with better error handling, clearer status messages, and improved process management. Startup sequence now validates prerequisites before launching services.
* **Path refactoring** - Standardized path handling across `internal/constants/paths.go`, `internal/config/config.go`, and service packages. Eliminates duplicate path resolution logic and improves consistency.
* **Test infrastructure consolidation** - Integration test helper functions consolidated into `test/integration_helper.go`. Reduces code duplication across A2A, MCP, and native Operator tests.
* **Scenario test simplification** - Scenario test framework refactored to use programmatic fixture generation instead of hardcoded JSON fixtures. Improves maintainability and reduces fixture drift.
* **Documentation cleanup** - Removed AI-focused language from documentation, improved developer guides, and clarified architectural descriptions. Protocol module now includes comprehensive LICENSE and README.

### Fixed

* **PKI regeneration bug** - Fixed certificate regeneration logic that could cause stale certificates to persist. PKI authority now properly invalidates and regenerates certificates on demand.
* **Receipts table output** - Fixed formatting and data integrity issues in receipts table output. Ledger results now display correctly with proper field alignment.
* **Authentication login flow** - Fixed CLI authentication login flow with improved error messages and better handling of edge cases in enrollment sequences.
* **Native test stability** - Fixed stability issues in native Operator tests by improving test isolation and cleanup procedures.
* **Trust script execution** - Fixed trust management script execution issues. Script now properly handles certificate trust operations across different platforms.

### Security

* **PKI trust bundle handling** - Enhanced trust bundle download and validation to prevent man-in-the-middle attacks during enrollment. Trust bundles are now verified before use.
* **Certificate enrollment hardening** - Certificate enrollment flow now strictly validates certificate chains and SANs before accepting new certificates.
* **mTLS boundary enforcement** - Improved mTLS boundary enforcement across gateway services with stricter certificate validation and cli/web/operator session management.

---

## [1.0.3] - 2026-05-29

### Overview

v1.0.3 removes all remaining g8ee application-layer coupling from the Gateway and protocol definitions. The Gateway routing layer uses dedicated controllers for admin and Operator lifecycle, and a CLI approval command enables out-of-band L3 transaction authorization. Security hardening includes fixes for outbound L3 notary verification and JIT user lockout prevention.

### Breaking Changes

* **g8ee API paths removed** - All g8ee-specific API paths removed from `protocol/constants/api_paths.json` and `internal/constants/api_paths.go`. The `g8ee` and `g8ee_full` path groups are deleted along with the `GetG8eePath()` helper function.
* **Device-link CLI commands removed** - The `g8e data device-links` command group (create, delete, list) is removed. Device-link token management is no longer exposed via CLI.
* **g8ee environment variables removed** - All g8ee-related environment variables and configuration entries removed from platform code.
* **Public endpoint renaming** - Gateway public endpoints renamed for clarity. Documentation and tests updated to reflect new endpoint names.
* **Protocol model cleanup** - Protocol models (agent_activity_metadata, case, conversation, investigation, operator_document, reputation_commitment, reputation_state, security_constraints, stake_resolution, tool_results, user, user_settings) updated to remove g8ee-specific field references.

### Added

* **CLI approval command:** The `./g8e approve <transaction_hash>` command enables out-of-band L3 transaction approval. Users sign suspended transaction hashes with their CLI private key and submit cryptographic proofs to the Gateway for authorization.
* **PublicRouteRegistry:** A centralized public route registry in `gateway_auth.go` eliminates fragile `HasPrefix` duplication across middleware. Exact paths and prefixes are registered in one location for maintainability.
* **AdminController:** A dedicated controller for admin-only endpoints, including app policy management. Separates admin concerns from auth and Operator controllers.
* **OperatorController:** A dedicated controller for Operator lifecycle endpoints (registration, binding, session management). Provides clear separation of Operator management concerns.
* **JIT user lockout defense:** A one-time valid JWT mechanism prevents JIT user lockout during enrollment. Users receive a temporary valid JWT if enrollment fails, ensuring they can recover access.
* **Enhanced gateway security:** Multiple security hardening improvements include stricter request validation, improved error handling, and enhanced authentication checks.

### Changed

* **Gateway routing refactor:** Gateway HTTP routing uses dedicated controllers. Admin, auth, and Operator concerns are separated into distinct controller packages with clear responsibilities.
* **L3 notary outbound fix:** L3 notary verification for outbound transactions is fixed. Suspended transaction handling and receipt generation correctly handle outbound mutation flows.
* **Test coverage expansion:** Extensive test coverage improvements across gateway services include comprehensive integration tests for JWT authentication, CLI approval, public route registry, and controller endpoints.
* **Build process simplification:** The `cp` command is removed from the build process in Makefile.Node Node Binary compilation is streamlined to eliminate unnecessary file operations.
* **Documentation updates:** CLI documentation, architecture docs, and guides reflect the platform-only architecture. g8ee-specific references and device-link command documentation are removed.
* **Protocol constants regeneration:** Protocol constants are regenerated after g8ee path removal. Generated Go constants are updated to match protocol JSON definitions.

### Fixed

* **Outbound L3 notary bug:** L3 notary correctly verifies and signs outbound transactions. The previous implementation did not properly handle suspended transaction states for outbound mutations.
* **Gateway security vulnerabilities:** Multiple security issues in gateway authentication and request handling are fixed. Validation of user inputs, session tokens, and request boundaries is improved.
* **CLI approval integration:** CLI approval command integration with Gateway JWT authentication is fixed. The approval flow correctly validates CLI signatures and updates suspended transaction state.
* **Test isolation issues:** Test isolation problems in gateway integration tests are fixed. Tests properly clean up database state and avoid cross-test contamination.
* **Public route matching:** Public route matching logic correctly handles both exact paths and prefixes. The previous implementation had edge cases where authenticated routes were incorrectly exposed.

### Security

* **JIT user lockout prevention:** A one-time valid JWT mechanism prevents users from being locked out during JIT enrollment failures. Ensures a recovery path for authentication errors.
* **L3 notary hardening:** Outbound transaction verification strictly enforces L3 signature requirements. Suspended transactions cannot bypass L3 checks.
* **Gateway request validation:** Enhanced request validation prevents malformed or malicious requests from reaching internal services. Stricter bounds checking on payload sizes and field values is implemented.
* **Public route registry:** Centralized public route definition eliminates accidental exposure of authenticated endpoints. All public routes are explicitly registered and auditable.

---

## [1.0.2] - 2026-05-28

### Added
- **TLS 1.3 Enforcement:** Strict requirement for TLS 1.3 across all platform communications; removed support for legacy TLS 1.2.
- **CSR-Based Enrollment:** Transitioned to Certificate Signing Requests (CSR) for all device and workload enrollment flows, enhancing identity verification.
- **Single-Port Multiplexing:** Unified HTTP/HTTPS router in the Gateway allows multiple services (Admin, MCP, A2A, PKI) to share a single port via strict SNI and mTLS routing.
- **PKI Revocation:** New `PKIController` implements certificate revocation and signed revocation bundle generation for fail-closed identity management.
- **Ecosystem Demos:** New LangChain agent demo showcasing the "Bring Your Own Agent" (BYOA) platform integration pattern.

### Changed
- **Platform Hardening:** Significant refactoring of Gateway and Operator services to improve maintainability and strictly enforce mTLS execution boundaries.
- **Bootstrap UX:** Renamed "CA Trust" to "Bootstrap" and improved startup output to better direct users toward the `g8e login` flow.
- **L1/L3 Governance:** Enhanced L1 Doctrine payload verification and unified L3 Approval brokerage for both WebAuthn and CLI sessions.
- **Legacy Cleanup:** Completed removal of legacy API-key-only authentication paths in favor of first-class PKI/mTLS.

### Fixed
- **Workload Identity:** Standardized SPIFFE-compatible URI SANs for all platform-issued certificates.
- **Audit Vault:** Hardened audit event write paths to strictly reject unattributed or malformed events.
- **Integration Tests:** Significant reliability improvements to MCP/A2A and BYO-client end-to-end test suites.

---

## [1.0.1] - 2026-05-27

### Breaking Changes

* **API keys removed** - API key authentication is fully deprecated and removed. All clients must now use JWT-based authentication. The `api_keys` collection, `APIKeyService`, and all API key-related endpoints, tests, and documentation are removed.
* **`openclaw` renamed to `insecure_mcp`** - The OpenClaw insecure MCP mode is renamed to `insecure_mcp` for clarity. Service files, tests, and documentation are updated accordingly.

### Added

* **JIT user provisioning** - Added owner-controlled just-in-time user provisioning via invitation-based enrollment. New `invitation_service.go` and `user_service.go` enable secure, time-bound user creation with invitation tokens.
* **New collections** - Added `invitations` and `users` collections to support JIT user provisioning.
* **Gateway JWT integration tests** - Added comprehensive JWT-based authentication integration tests for A2A and MCP gateways.

### Changed

* **JWT-only authentication** - Gateway now enforces JWT-based authentication exclusively. All API key paths, constants, and middleware are removed.
* **Schema organization** - Moved `schema.sql` from `internal/services/gateway/` to `internal/services/gateway/db/` for better directory structure.
* **Documentation updates** - Updated gateway architecture, Operator docs, and connection guides to reflect JWT-only auth and JIT user provisioning.

### Fixed

* **JWT integration test** - Fixed invitation setup in JWT integration test to ensure proper test isolation.
* **Documentation links** - Fixed broken position paper link and README references.

---

## [1.0.0] - 2026-05-25

### Overview

v1.0.0 completes the platform-first architecture. The g8ee application layer is excised from the
platform entirely; the Sentinel component is dissolved into the governance protocol layers; and the
codebase is restructured so `services/g8eo` is the root module. The platform is now a pure, host-sovereign
governance platform: typed, signed, state-bound transactions enforced through fail-closed L1/L2/L3/L4/L5
gates with no optional application-layer coupling in the critical path.

### Breaking Changes

* **g8ee removed from platform** - g8ee is no longer part of the platform. All `services/g8ee`
  references, root Makefile targets, and environment variable dependencies are gone. Run g8ee
  separately as an optional application adapter.
* **Sentinel dissolved** - Sentinel no longer exists as a standalone component. Threat detection
  (MITRE/pattern analysis) moved into L1 Doctrine executed within the L4 Warden gate. Data
  sovereignty (scrubbing, rehydration, encryption) moved into the new `internal/services/sovereignty`
  Boundary Plane, invoked at L5 Actuator egress and before audit publishing. The raw audit vault is
  removed; all audit data now passes through Sentinel-moderated (`SentinelModerateRaw`) storage.
* **Repository root is now the g8eo module** - `services/g8eo/` content promoted to root. `go.mod`,
  `go.sum`, and all `internal/` packages now live at the repo root. `cmd/g8eo` is the sole
  top-level `cmd/` entry. The auditor and chaos tester are now CLI subcommands (`./g8e auditor`,
  `./g8e chaos`).
* **`web_session_id` → `cli_session_id`** - Session ID field renamed across all logging, events, and
  generated constants to reflect CLI-first architecture.
* **Cursor-based queries removed** - All cursor-based database query patterns eliminated in favor of
  direct indexed access.
* **Demo profiles removed** - Docker-based demo profiles (`acme-corp`, `fleet`, `nginx`, `pnfs`) and
  the `evals/` Python harness are removed. Demos and evals are no longer bundled with the platform.

### Added

* **Scenario testing framework** (`test/scenario/`) - End-to-end governance pipeline test suite with
  fixture-driven runner, golden file assertions, receipt verification tests, concurrency tests, and
  fuzz tests. Covers L1/L2/L3 gate passes/failures, forged signatures, stale state roots, tampered
  receipts, and Mode X truth table scenarios. Fixture generation tooling included.
* **Sovereign Execution Boundary** (`internal/services/sovereignty/`) - New first-class package
  implementing data scrubbing, rehydration, and Sentinel encryption for the egress boundary.
* **App enrollment service** (`internal/services/gateway/app_enrollment_service.go`) - Operator-owned
  enrollment for non-native app mTLS integration.
* **SQLite utility package** (`internal/services/sqliteutil/`) - Extracted shared SQLite helpers.
* **CLI Go package** (`internal/cli/`) - Full Go implementation of CLI commands (`auth`, `data`,
  `platform`, `security`, `test`), config, and platform process management, replacing shell scripts.
* **A2A and MCP integration tests** (`test/a2a_gateway_test.go`, `test/a2a_real_operator_test.go`,
  `test/byo_client_test.go`, `test/mcp_gateway_test.go`) - Real-operator gateway integration tests.
* **Bulk certificate revocation** - Fleet-scale cert revocation with rate limiting.
* **`protocol/constants/field_paths.json`** - Canonical field path registry added to the protocol module.
* **Generated constant tracking** - `headers_generated.go` and `status_generated.go` added to the
  constant registry with deterministic sorting for reproducible builds.
* **`-a` shorthand** - `./g8e gw start -a` shorthand for faster invocation.
* **Unit/integration test subcommands** - `./g8e test` now exposes distinct `unit` and `integration`
  subcommands for CI granularity.

### Changed

* **Governance layer separation** - L1 Doctrine, L2 Consensus (Tribunal), L3 Notary, L4 Warden, and
  L5 Actuator are now clearly separated packages with explicit interfaces and generated mocks. Dead
  code and ambiguous shared state between Tribunal and consensus definitions removed.
* **Canonical wire format** - Formalized canonical JSON (`protojson`) as the required client-facing wire format for all Governance Envelopes instead of binary protobuf bytes, ensuring universal BYO client compatibility.
* **Local-only Protobuf generation** - Migrated `buf.gen.yaml` from BSR remote plugins to local-only generation to completely eliminate network dependencies and rate limits during compilation.
* **SPIFFE URI SAN hardening** - Parsing fragility fixed; format validation tightened across both
  code and test fixtures.
* **DB transaction safety** - Unprotected transactions fixed; cursor-based query patterns replaced.
* **mTLS bootstrap** - First-time mTLS setup sequence fixed; PKI test initialization refactored for
  isolation.
* **Node Binary build process** - "build once, then copy" pattern enforces a single compilation artifact
  per binary, eliminating race conditions during `platform start`.
* **Constants casing** - Acronym and general casing standardized across all generated Go constants.
* **Docs restructured** - Documentation reorganized to match the new root layout: `docs/architecture/`,
  `docs/core/`, `docs/devs/`, `docs/guides/`, `docs/protocols/`, `docs/reference/`. Mkdocs migrated
  to the built-in readthedocs theme.
* **Protobuf toolchain** - Upgraded to protobuf v1.35.2; toolchain mismatch resolved.

### Removed

* `services/g8ee/` - Entire Engine application layer removed from the platform repository.
* `evals/` - Python evaluation harness removed.
* `demo/` - All Docker-based demo profiles removed.
* Raw audit vault - `VaultModerateRaw` replaced by `SentinelModerateRaw`; unmoderated raw storage path eliminated.
* Vendored `gotestsum` - Removed in favor of direct tooling.
* Shell script entrypoints - `entrypoint.sh` remnants and platform shell scripts removed; replaced by the Go CLI package.

### Security

* **Sentinel encryption** - Sovereign Execution Boundary encrypts sensitive fields before audit publishing; decrypts at authorized egress.
* **Fail-closed Audit Store** - `SQLAuditStore` now strictly rejects missing/malformed session IDs and unknown sessions prior to any audit writes, preventing invalid event relationships.
* **Bulk revocation with rate limiting** - Rapid revocation of compromised credentials at fleet scale without unbounded load.
* **mTLS for non-native apps** - App enrollment service extends mTLS enforcement to heterogeneous clients.
* **SPIFFE URI SAN hardening** - Fragile SPIFFE parsing that accepted malformed URIs on valid inputs fixed.
* **Unprotected transaction fix** - DB transactions that could expose inconsistent state under concurrency now properly bounded.
* **Receipt tampering detection** - Scenario tests verify the platform rejects tampered receipts across all governance layers.

---

# [0.2.7] - 2026-05-20

## Overview
Release **v0.2.7** separates the **g8e Gateway** and the **g8e Operator** into distinct roles, introduces an **MCP & A2A protocol translator gateway**, removes external runtime dependencies (`git` and `jq`), and improves overall security and developer experience.

## Key Changes

* **Gateway Role Splitting**: The Go Gateway is now explicitly split into the **g8e Gateway** acting as the central Policy Decision Point (PDP), and the **g8e Operator** acting as the host-side Policy Execution Point (PEP).
* **MCP & A2A Gateway**: g8e Operator can now act as a standalone admission gate for standard AI clients. It translates standard tool calls into governed transactions and supports out-of-band transaction suspension with WebAuthn approval before execution.
* **Native Dependencies**: Replaced external CLI dependencies on `git` and `jq` with native Go (`go-git/v5`) and Python implementations to streamline the runtime footprint.
* **Security Enhancements**: Implemented strict mTLS client-identity verification for Server-Sent Events (SSE) push endpoints.
* **CLI Protections**: Added interactive confirmation prompts to prevent accidental data loss on destructive operations like `platform reset` and `platform clean`.
* **Air-gapped Support**: Improved protobuf generation for air-gapped environments.

## Shoutout
Special thanks to **@zhouzhou626** for their first contribution (PR #74) adding a new developer troubleshooting guide - hopefully this PR addresses the friction. If not, PRs are much appreciated.


## [0.2.6] - 2026-05-19

### Added
- **Intent Classification:** Revived intent classification to be a first-class citizen in the architecture.
- **SPIFFE URI SAN:** Refactored SPIFFE URI SAN logic to strengthen mTLS and workload identity.

### Changed
- **CLI & UX Improvements:** Improved login UX, operator-side UX, and bootstrap script stability. Enhanced build output for Mac and Linux.
- **Protocol Refinement:** Ripped out legacy protobuf definitions, refined boundary structures, and decoupled Operator auth from the app layer.
- **Session Isolation:** Improved session typing and untangled CLI chat sessions to better separate the Gateway and app layer.
- **Code Quality & Linting:** Comprehensive code quality passes including Go critic/lint fixes, Ruff, and Pyright typing improvements.
- **Eval & Testing:** Refactored the eval harness and bench tests. Improved chaos testing with better audit summaries, L1 reporting, and correct DB location.
- **Documentation:** Reorganized and updated documentation including improved diagrams and README updates.

### Fixed
- **SSL Configuration:** Addressed SSL fix.
- **Test Stability:** Fixed test stability issues including `g8ee` test fixes and model configuration improvements for tests.

## [0.2.5] - 2026-05-16

### Added
- **CLI Chat Wiring:** Implemented full CLI chat functionality (`./g8e chat`) with backend wiring to `g8ee` and unified stream handling.
- **Multi-Ledger Audit:** Implemented session-isolated Git audit ledgers for per-investigation transaction tracing.
- **Actuator Execution Boundary:** Established g8e Operator Actuator as the authoritative execution boundary with signed action receipts.
- **Governance APIs:** Added first-class governance APIs for audit export and trust management.
- **Protobuf Module:** Introduced a unified `protocol/` directory with formal Protobuf module definitions.
- **Commitment Ledger:** Added definitions for the commitment ledger to support reputation staking.
- **Internal API Routing:** Established a unified internal router for component-to-component communication within `g8ee`.

### Changed
- **RequestContext Body Migration:** Migrated business context (`web_session_id`, `user_id`, `source_component`, etc.) from HTTP headers to body-embedded `RequestContext` objects for improved security and contract stability.
- **Directory Reorganization:** Renamed `components/` to `services/` and `shared/` to `protocol/` to align with the mandatory Gateway-first architecture.
- **g8ed Decommissioning:** Completed the removal of `g8ed` (Dashboard) remnants; migrated all core logic to the g8e Operator.
- **Auth Cleanup:** Refactored `APIKeyService` and passkey authentication for better consistency and security across the Gateway.
- **CodeQL Refactor:** Optimized CodeQL workflows and addressed findings in `event_service`.
- **Exit Code Handling:** Standardized exit code handling and improved path validation in g8e Operator execution services.
- **Event Service:** Consolidated `client_event_service` into a unified `event_service` within g8e Operator.
- **Improved Chaos Output:** Enhanced chaos test reporting for better failure visibility.

### Fixed
- **Operator TLS Hardening:** Refined Operator TLS configuration and improved gateway service stability.
- **WebAuthn L3:** Fixed L3 verification issues following the `g8ed` decommissioning.
- **Path Resolution:** Improved path resolution and environment variable handling across the platform, including fixes in `paths.json`.
- **Test Stability:** Extensive fixes for unit and integration tests across `g8ee` and g8e Operator, particularly around the `RequestContext` migration and tribunal consensus.
- **Case Update Logic:** Fixed `CaseDataService.update_case` to correctly handle empty updates by ignoring the `context` field.

## [0.2.4] - 2026-05-13

### Added
- **Operator-Owned PKI/TLS:** Transitioned from legacy SSL to a robust CSR-based mTLS infrastructure owned by g8e Operator.
- **mTLS Enrollment:** New CSR and mTLS enrollment flow for operators and clients.
- **BYO Client Support:** Consolidated state root and added end-to-end support for "Bring Your Own" clients.
- **CLI Login:** Added first-class CLI login support via the operator.

### Changed
- **Gateway/App Layer Split:** Formalized g8e Operator as the mandatory Gateway and moved `client`/`g8ee` to optional application-layer adapters.
- **client Elimination:** Removed `client` Dashboard as a mandatory component; migrated data management scripts to g8e Operator API.
- **Governance Envelope Hardening:** Improved GovernanceEnvelope and proto definitions for better transaction integrity.
- **Reorganized g8e Operator:** Directory restructuring for better modularity and maintainability.
- **Passkey & Setup Refactor:** Migrated passkey and setup logic to the Operator Gateway.

### Fixed
- **Settings Model Paths:** Fixed inconsistencies in settings model resolution.
- **Split Brain Config:** Resolved configuration synchronization issues.
- **Startup Health Check:** Fixed issues with platform startup health verification.
- **PKIDir Bug:** Fixed bug in `PKIDir` path resolution.
- **Security & Testing:** Addressed CodeQL findings and improved test security headers.

## [0.2.3] - 2026-05-11

### Added
- **Interactive Platform Manager:** New interactive menu for platform management, simplifying setup, environment configuration, and e2e testing.

### Changed
- **Evals Refactor:** Streamlined evaluation management to be runtime-configurable.
- **Improved Setup:** Enhanced environment variable handling and validation during bootstrap.

### Fixed
- **Documentation:** Fixed various typos and inconsistencies across architecture documentation.

## [0.2.2] - 2026-05-10

### Added
- **Ollama Model Query:** Added support for querying available Ollama models during setup with improved UI feedback.
- **Runtime Configuration:** Evals configuration is now set at runtime instead of build time for improved security and flexibility.
- **Host-Native Testing:** Platform now runs component tests host-native without Docker, improving test reliability and CI performance.

### Changed
- **Removed Docker:** Eliminated Docker containerization across the platform. Components now run directly on the host with the Node binary in listen mode.
- **Platform Architecture:** Migrated to host-native execution model with platform runtime state in repo-local `.g8e` directory.
- **Build System:** Comprehensive updates to `build.sh` for host-native bootstrapping, improved auth token handling, and better signal handling.
- **Documentation:** Updated all documentation to reflect the removal of Docker and the new host-native architecture.
- **Constants Paths:** Fixed and standardized constants paths across all components for better consistency.

### Fixed
- **Security:** Fixed SSRF vulnerability in Ollama model query endpoint.
- **Port Conflicts:** Resolved port conflict issues during platform startup.
- **Platform Commands:** Fixed g8e platform commands for proper host-native execution.
- **Build.sh:** Fixed auth token handling and kill signal processing in build scripts.
- **Test Suite:** Fixed test failures across g8ee, client, and g8e Operator after Docker removal.
- **Chat:** Fixed chat functionality issues in the dashboard.
- **Demo Profiles:** Fixed nginx demo and cleaned up SAN configurations in demo profiles.
- **Certificate Service:** Fixed test certificate service for host-native testing.
- **Dependency:** Bumped fast-uri from 3.1.0 to 3.1.2 in client for security.

### Removed
- **Dockerfiles:** Removed all Dockerfile configurations (Dockerfile, Dockerfile.test) from components.
- **docker-compose.yml:** Removed Docker Compose configuration for platform components.

---

## [0.2.1] - 2026-05-07

### Added
- **Build System Improvements:** Optimized `build.sh` for more reliable component container builds.

### Changed
- **Heartbeat Service:** Refactored heartbeat processing to align with updated Protobuf schemas and improved error handling.
- **Envelope Builder:** Updated `EnvelopeBuilder` to ensure correct field mapping for heartbeat events.
- **Metrics Routing:** Refined console metrics routing and service interaction in `client`.

### Fixed
- **Heartbeat Proto Serialization:** Resolved serialization issues in the heartbeat service ensuring stable cross-component status updates.
- **Test Suite Cleanup:** Removed deprecated `pubsub_results` tests and modernized console metrics unit tests.
- **Cache Reliability:** Improved cache-aside service reliability in `client`.

## [0.2.0] - 2026-05-07

### Added
- **Protobuf-Driven Architecture:** Massively migrated the platform to a robust, typed Protobuf-driven architecture for payloads, while maintaining a GovernanceEnvelope JSON-first transport for mutation envelopes.
- **Governance Envelope:** Introduced the JSON `GovernanceEnvelope` for all BFT transactions, binding event metadata, state roots, and hardware-bound fingerprints.
- **L1/L2/L3 Governance:** Integrated a 3-layer command validation hierarchy (L1 Technical Bedrock, L2 Consensus/Tribunal, L3 Authorization/Human) directly into the message envelope.
- **Recursive Grep Tool:** Introduced `recursive_grep_search` for high-efficiency filesystem exploration across Operator fleets.
- **Interrogation Gate:** Implemented a new gate in the agent loop that detects `<interrogation>` blocks and suppresses pending tool calls to prioritize user input.
- **Actuator Risk Analysis:** Enhanced risk classification logic for Actuator sub-agents with improved reputation staking and file-read security.
- **LFAA Audit Enhancements:** Refactored the Low-Fidelity Agentic Assistance audit recording to use typed Protobuf schemas.

### Changed
- **G8EO Protocol Hardening:** Hardened g8e Operator to reject malformed or non-envelope command bytes and enforce L1 `forbidden_patterns` via Protobuf reflection.
- **Tribunal 2.0 Pipeline:** Refactored the Tribunal consensus pipeline into a modular, stage-based architecture utilizing strict Protobuf-typed payloads and signatures.
- **G8eHttpContext Refactor:** Centralized and enforced strict security header validation (`web_session_id`, `user_id`, `source_component`) for all internal service communication.
- **Internal API Security:** Enforced strict component-identity verification and session-binding for internal component-to-component routing.
- **Operator Lifecycle:** Hardened Operator slot management with atomic state transitions and reliable relaunch/activation logic.
- **Removed g8ep:** Eliminated the sidecar-managed `g8ep` Operator node and `SupervisorService` in favor of external operators and unified slot management.
- **Standardized Cloud Subtype:** Standardized Operator identification using `cloud_subtype` for consistency across cloud providers.

### Fixed
- **Actuator Risk Regression:** Resolved a regression where Actuator risk levels were incorrectly calculated in certain agent turns.
- **Interrogation Plumbing:** Fixed response handling and user interaction flow for the device interrogation pipeline.
- **G8EO Execution ID:** Fixed a bug where `FsGrepResultPayload` was missing `ExecutionID` propagation, breaking correlation for recursive searches.
- **Fingerprint Recording:** Resolved issues with system fingerprint recording and included missing events in the audit trail.
- **Test Coverage & Stability:** Massive increase in unit and integration test coverage for `g8ee`, g8e Operator, and `operator`, with full migration to typed payload assertions.

### Removed
- **Legacy Audit UI:** Removed the outdated Audit page and associated backend services from `client` in favor of streamlined platform logging.
- **"Available" Status:** Deprecated the "available" Operator status as it was redundant for state management.

## [0.1.9] - 2026-05-05

### Added
- **Acme Corp Demo:** Added new `acme-corp` demo profile demonstrating edge device registration and management.
- **Blog Post:** Added new blog post covering platform updates and vision (`5-5-26.md`).
- **Nginx Demo Profile:** Reorganized and enhanced the Nginx demo profile with regional deployments.

### Changed
- **Actuator Prompts & Pathing:** Improved Actuator sub-agent prompts and corrected file pathing behavior.
- **Read-Only Tools UX:** Enhanced the user experience for read-only tools and terminal results alignment.
- **Tribunal Logging:** Improved logging detail and clarity for the Tribunal consensus pipeline.
- **Tribunal Voting:** Enforced a mandatory two-round minimum for Tribunal voting to ensure rigorous consensus.
- **Model Selection:** Refined the model selection drawer UI.
- **Operator Card:** Removed unnecessary animations from the Operator card for better performance.
- **PR Template:** Updated the pull request template for better contributor guidelines.
- **Documentation:** General improvements to platform documentation, position paper, and `g8e-help`.

### Fixed
- **Interrogation Plumbing:** Fixed response handling and plumbing for the device interrogation flow.
- **Hamburger Menu:** Corrected the width and layout of the dashboard hamburger menu.
- **Fleet Demo:** Fixed configuration and deployment issues in the fleet demo profile.
- **Node Count & Bind All:** Fixed node counting logic for demos and moved the "Bind All" button to the top of the Operator list.

## [0.1.8] - 2026-05-04

### Added
- **Batch Tool Execution:** Support for fan-out execution across multiple operators with configurable concurrency and fail-fast behavior.
- **Improved Evals Suite:** Enhanced evaluation runner with support for accuracy and privacy gold sets, and improved fleet management for large-scale tests.
- **Async Scribe & Codex:** Introduced async sub-agents for case titling and preference/memory extraction.
- **Unified Batch Runner:** New `BatchRunner` service in `g8ee` for coordinating multi-operator operations.

### Changed
- **Information Isolation:** Formalized the "Information Isolation Principle" (formerly Vortex Principle) for enhanced multi-agent safety.
- **Tribunal Consensus:** Refined consensus logic (Plurality Consensus) with deterministic tie-breaking and circuit breaker for deadlocks.
- **Actuator Reputation Staking:** Actuator sub-agents now stake reputation on risk classifications.
- **Setup UX:** Improvements to the onboarding wizard, ensuring validation visibility and cleaner summary view.
- **Python Modernization:** Migrated to `StrEnum` for improved type safety and performance across `g8ee`.

### Fixed
- **Device Link Scalability:** Increased `DEVICE_LINK_MAX_USES` to 10,000 to support large fleet registrations.
- **UI Robustness:** Improved error handling and icon rendering in the setup and status components.
- **Import Optimizations:** Resolved various circular import issues and optimized imports in `g8ee`.

## [0.1.7] - 2026-05-01

### Added
- **Actuator Reputation Staking Improvements:** Enhanced reputation staking logic for Actuator's risk assessments, including file read fixes and order handling.
- **Agent Cancellation:** Added support for cancelling agent tasks with dedicated UI controls and tests.

### Changed
- **Actuator Personas & Context:** Refined Actuator's context and personas for better risk evaluation.
- **Tool Call Event Delivery:** Improved reliability and performance of tool call event delivery.
- **Onboarding UX:** Enhancements to the onboarding flow for a smoother user experience.
- **Node Package Updates:** Updated dependencies in `client` for security and performance.

### Fixed
- **Hamburger Menu & Screenshots:** Resolved issues with the dashboard hamburger menu and screenshot capture functionality.
- **File Edit Payload Handling:** Fixed bugs in how file edit payloads are processed.
- **Device Link Auth Regression:** Fixed a regression in device link authentication during interrogation.
- **Theme & Icon Fixes:** Resolved UI glitches related to theme switching and specific icons.
- **Investigation Context:** Fixed tests and handling of investigation context.

## [0.1.6] - 2026-04-29

### Added
- **Information Isolation Round 2:** Enhanced reputation staking system with improved governance and consensus mechanisms.
- **Reputation Staking:** Implemented multi-phase reputation commitment and stake resolution for Operator trust management.
- **Bug Fixes:** Resolved various issues across platform components for improved stability.

## [0.1.5] - 2026-04-28

### Added
- **Reputation System:** Introduced a multi-stage reputation and staking system, including `ReputationCommitment`, `ReputationState`, and `StakeResolution` models for trust-based Operator management.
- **SSH Inventory Streaming:** New capability to stream and import Operator inventory directly from local SSH configuration files.
- **Enhanced Test Fixtures:** Added `gold-set-schema.json` and `ledger-hash-fixtures.json` to improve consistency across platform evaluation suites.
- **Reputation CLI:** New administrative scripts `manage-reputation.py` and `seed-reputation-state.py` for platform governance.

### Changed
- **Tribunal 2.0 Governance:** Significant refactor of the Tribunal pipeline, implementing multi-phase consensus, detailed dissent recording, and improved safety guideline delivery.
- **Operator Authority Model:** Consolidated Operator document handling and configuration delivery, positioning `g8ee` as the authoritative source for Operator state.
- **Settings UX Overhaul:** Redesigned the Dashboard Settings page to match the Setup page layout, including improved command validation and status rendering.
- **Device Link Refactoring:** Streamlined device link management and added auto-approval logic for benign, non-mutating commands.
- **System Info & Heartbeat Synchronization:** Overhauled `SystemInfo` and `Heartbeat` wire models for better cross-component consistency and reduced payload size.

### Fixed
- **Authentication Loops:** Resolved edge cases in Operator authentication and fixed internal routing issues during high-concurrency streams.
- **Async Tooling:** Fixed `asyncio` race conditions in the `ToolService` and improved background task tracking.
- **Test Suite Stability:** Fixed unit and integration test failures in `client`, `g8ee`, and the evals suite.
- **API Key Security:** Improved masking and display security for API keys within the CLI environment.
- **Iconography:** Fixed missing or incorrect icons in the Dashboard, including the Auditor and Operator status indicators.

## [0.1.4] - 2026-04-24

### Added
- **Release Synchronization:** Version bump to 0.1.4 to synchronize platform components after tagging conflict.

## [0.1.3] - 2026-04-24

### Added
- **Global Platform Refactor:** Massive synchronization of constants and models across `client`, `g8ee`, and `shared` layers to ensure wire-contract stability.
- **Iteration-Scoped AI Message Persistence:** Per-tool-iteration AI commentary now lands in `conversation_history` as `MessageSender.AI_PRIMARY` rows tagged with `EventType.EVENT_SOURCE_AI_PRIMARY`, preserving the agent's running narrative across restores. The SSE delivery layer fires an `on_iteration_text` callback at each `TOOL_RESULT` boundary, which `ChatPipelineService` binds to a persistence helper. Final post-stream persistence still runs and skips whitespace-only text.
- **`InvestigationService.persist_ai_message(...)`:** New domain-layer helper that centralizes the strip-guard and `AIResponseMetadata` construction previously duplicated between the per-iteration and final AI persist paths. Accepts optional `grounding_metadata` and `token_usage` for the final-row case.
- **`BackgroundTaskManager.track_detached(...)`:** Synchronous tracking helper for fire-and-forget tasks dispatched from inside coroutines that cannot `await`. Auto-removes completed tasks via done-callback; surfaces uncaught exceptions at `WARNING` with `exc_info=True`.
- **`AgentInputs` / `AgentStreamState` split:** Request-scoped immutable inputs (`AgentInputs`) are now separate from the mutable per-run stream sinks (`AgentStreamState`). Both use `extra='forbid'`. Replaces the previous combined `AgentStreamContext` / `make_streaming_context`.
- **Evaluation Suite:** Introduced comprehensive AI evaluation tools including `accuracy`, `benchmark`, and `privacy` scorers to validate agent behavior against gold sets.
- **Tribunal Voting Breakdown:** Enhanced tribunal consensus events now include detailed voting breakdowns and dissent records.

### Changed
- **Frontend Modernization:** Overhauled `client` components including `operator-panel`, `anchored-terminal`, and SSE handlers for better UX and reliability.
- **Memory Generation Off The Response Path:** `update_memory_from_conversation` is no longer awaited inline in `_persist_ai_response`. It is dispatched as a tracked background task via `BackgroundTaskManager.track_detached`, so memory generation can no longer block SSE completion or silently swallow errors. Failures are logged at `WARNING` level with `exc_info=True` (previously `INFO`, which hid real errors).
- **SSE Event Publishing:** `deliver_via_sse` now publishes through a single `_publish(event_type, payload)` closure that captures the fixed `(investigation_id, web_session_id, case_id, user_id)` routing tuple. Eliminates 14 call sites where a new event could accidentally drop a routing field.
- **Validation Messages in `deliver_via_sse`:** Split the single multi-field guard into three precise checks with correct `field=` identifiers for `investigation_id`, `web_session_id`, and `case_id`.
- **`ExecutorCommandArgs` Cleanup:** Removed dead `execution_id` and `web_session_id` fields that were never populated from the caller surface.

### Fixed
- **`OPERATOR_COMMAND_APPROVAL_*` Render Leak:** Fixed frontend leak of approval-lifecycle system rows in `chat-history.js` using new `event_type` metadata.
- **Stale Test Fixtures:** Fixed `AgentStreamState` mutation in chat pipeline tests to ensure final persist path is correctly exercised.
- **Auditor Command Generation:** Refactored command generator and cleaned up auditor-related events.

---

## [0.1.2] - 2026-04-20

### Added
- **Tribunal Enhancements:** 5-member tribunal implementation with enhanced context and safety guidelines delivery to the tribunal pipeline
- **Operator Panel Documentation:** Comprehensive documentation for Operator panel paths and features
- **Operator Panel Tests:** Added test coverage for Operator panel path functionality

### Changed
- **Bound Session Refactoring:** Renamed `web_session_id` to `bound_web_session_id` across all services for clarity and consistency
- **SSE Validation:** Enhanced Server-Sent Events validation and wire/docs alignment
- **Heartbeat System:** Improved heartbeat data handling in g8ee and cleaned up flatten_for cruft
- **Metrics Delivery:** Enhanced metrics delivery to frontend for better Operator monitoring
- **Tribunal Error Handling:** Consolidated Tribunal error-to-event-to-tool-call-failure flow for better error tracking
- **Temperature Configuration:** Cleaned up temperature settings to be persona-specific
- **Sentinel Configuration:** Sentinel is now always-on with updated documentation

### Fixed
- **CLI Authentication:** Improved CLI login flow and authentication handling
- **CLI Security:** Enhanced CLI security for Ollama-only setups
- **Operator Panel:** Fixed Operator list display, bind/unbind all buttons, and public IP obfuscation
- **Model Selection:** Fixed model selection drawer in the dashboard
- **Platform Clean:** Fixed platform clean script for proper cleanup
- **Frontend Bugs:** General UX improvements and frontend bug fixes
- **Code Quality:** Ruff linting fixes and removal of dead AgentMetadata enum

---

## [0.1.1] - 2026-04-16

### Added
- **g8ee Model Serialization:** Introduced `UTCDatetime` type for all wire-facing datetime fields, serializing to ISO 8601 with `Z` suffix. Replaced custom `flatten_for_wire()`, `flatten_for_db()`, and `flatten_for_llm()` methods with Pydantic's native `model_dump(mode="json")` for boundary serialization. Added `SessionEventWire` and `BackgroundEventWire` models for SSE event contracts.

### Changed
- **Multi-Operator Batches:** `batch_id` correlation is now surfaced end-to-end - on `CommandExecutionResult`, approval metadata, and conversation message metadata - so agents and the dashboard can tie per-operator events and follow-up actions back to a single batched approval.
- **Task Tracking:** Task ID and TDTS tracking added for better correlation and debugging.
- **Setup Page:** Users can now reuse Gemini API key for Vertex AI search in the setup page.

### Changed
- **Batch Concurrency Safety:** `command_validation.max_batch_concurrency` is now bounded (1–64) at the model layer, preventing misconfigurations that could fan out to an unbounded number of operators.
- **Operator Selection Errors:** Multi-operator validation errors now clearly describe both single-host (`target_operator`) and batch (`target_operators`) targeting options.
- **Documentation:** `client` docs updated to describe parallel batch fan-out with bounded concurrency and shared `batch_id` correlation.
- **Agent Autonomy Language:** Updated autonomy-related language to use more empowering terminology across prompts and documentation.
- **Prompt Engineering:** Cleaned up anti-patterns from prompts, synchronized verbiage, and refactored thinking support to handle multiple definitions.
- **Capability Handling:** Improved capability handling and thinking levels for agents.
- **Operator Panel:** Added collapse functionality and increased pagination to 20 operators per page.

### Fixed
- **Ollama Provider:** Fixed model selection, `num_ctx` configuration, error handling, and thinking parameter handling. Improved context window handling from Ollama responses.
- **Gemini Models:** Fixed Gemini 3 Flash model name in configuration.
- **Model Selection:** Fixed model selection dropdowns and UI collapsing issues across the dashboard.
- **Test Infrastructure:** Improved g8ee test fixes, added test parallelism support, and fixed various integration and unit tests.

---

## [0.1.0] - 2026-04-11

### Added
- **Core Platform:** Open-source release of the `g8e` platform for AI-assisted infrastructure operations.
- **g8ee (g8e-Compliant Agentic Ensemble):** ReAct-based Python orchestration layer with support for Anthropic, OpenAI, and local Ollama models.
- **g8e Operator:** ~4MB dependency-free static Go binary for remote host execution. Features zero-inbound ports and outbound-only mTLS.
- **operator (Data Store):** SQLite-backed persistence layer, KV store, and pub/sub broker running within the Operator framework.
- **client (Dashboard):** Node.js central management console featuring FIDO2 WebAuthn (passkey) authentication and real-time mTLS gateway proxying.
- **Security:** "Tribunal Refinement Pipeline" utilizing stochastic swarm voting to validate AI-proposed terminal commands before human review.
- **Security:** Local execution vaulting to ensure raw stdout/stderr logs are securely encrypted and retained strictly on the target host.
- **DevOps:** Comprehensive `g8e` CLI wrapper for host-native platform lifecycle, testing, Operator deployment, and CA certificate management.
