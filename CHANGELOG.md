# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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

*   **JSON Robustness** - Gateway controllers now strictly validate and reject malformed JSON bodies during operator binding and configuration updates.
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
* **Test infrastructure consolidation** - Integration test helper functions consolidated into `test/integration_helper.go`. Reduces code duplication across A2A, MCP, and native operator tests.
* **Scenario test simplification** - Scenario test framework refactored to use programmatic fixture generation instead of hardcoded JSON fixtures. Improves maintainability and reduces fixture drift.
* **Documentation cleanup** - Removed AI-focused language from documentation, improved developer guides, and clarified architectural descriptions. Protocol module now includes comprehensive LICENSE and README.

### Fixed

* **PKI regeneration bug** - Fixed certificate regeneration logic that could cause stale certificates to persist. PKI authority now properly invalidates and regenerates certificates on demand.
* **Receipts table output** - Fixed formatting and data integrity issues in receipts table output. Ledger results now display correctly with proper field alignment.
* **Authentication login flow** - Fixed CLI authentication login flow with improved error messages and better handling of edge cases in enrollment sequences.
* **Native test stability** - Fixed stability issues in native operator tests by improving test isolation and cleanup procedures.
* **Trust script execution** - Fixed trust management script execution issues. Script now properly handles certificate trust operations across different platforms.

### Security

* **PKI trust bundle handling** - Enhanced trust bundle download and validation to prevent man-in-the-middle attacks during enrollment. Trust bundles are now verified before use.
* **Certificate enrollment hardening** - Certificate enrollment flow now strictly validates certificate chains and SANs before accepting new certificates.
* **mTLS boundary enforcement** - Improved mTLS boundary enforcement across gateway services with stricter certificate validation and session management.

---

## [1.0.3] - 2026-05-29

### Overview

v1.0.3 removes all remaining g8ee application-layer coupling from the Gateway and protocol definitions. The Gateway routing layer uses dedicated controllers for admin and operator lifecycle, and a CLI approval command enables out-of-band L3 transaction authorization. Security hardening includes fixes for outbound L3 notary verification and JIT user lockout prevention.

### Breaking Changes

* **g8ee API paths removed** - All g8ee-specific API paths removed from `protocol/constants/api_paths.json` and `internal/constants/api_paths.go`. The `g8ee` and `g8ee_full` path groups are deleted along with the `GetG8eePath()` helper function.
* **Device-link CLI commands removed** - The `g8e data device-links` command group (create, delete, list) is removed. Device-link token management is no longer exposed via CLI.
* **g8ee environment variables removed** - All g8ee-related environment variables and configuration entries removed from platform code.
* **Public endpoint renaming** - Gateway public endpoints renamed for clarity. Documentation and tests updated to reflect new endpoint names.
* **Protocol model cleanup** - Protocol models (agent_activity_metadata, case, conversation, investigation, operator_document, reputation_commitment, reputation_state, security_constraints, stake_resolution, tool_results, user, user_settings) updated to remove g8ee-specific field references.

### Added

* **CLI approval command:** The `./g8e approve <transaction_hash>` command enables out-of-band L3 transaction approval. Users sign suspended transaction hashes with their CLI private key and submit cryptographic proofs to the Gateway for authorization.
* **PublicRouteRegistry:** A centralized public route registry in `gateway_auth.go` eliminates fragile `HasPrefix` duplication across middleware. Exact paths and prefixes are registered in one location for maintainability.
* **AdminController:** A dedicated controller for admin-only endpoints, including app policy management. Separates admin concerns from auth and operator controllers.
* **OperatorController:** A dedicated controller for operator lifecycle endpoints (registration, binding, session management). Provides clear separation of operator management concerns.
* **JIT user lockout defense:** A one-time valid JWT mechanism prevents JIT user lockout during enrollment. Users receive a temporary valid JWT if enrollment fails, ensuring they can recover access.
* **Enhanced gateway security:** Multiple security hardening improvements include stricter request validation, improved error handling, and enhanced authentication checks.

### Changed

* **Gateway routing refactor:** Gateway HTTP routing uses dedicated controllers. Admin, auth, and operator concerns are separated into distinct controller packages with clear responsibilities.
* **L3 notary outbound fix:** L3 notary verification for outbound transactions is fixed. Suspended transaction handling and receipt generation correctly handle outbound mutation flows.
* **Test coverage expansion:** Extensive test coverage improvements across gateway services include comprehensive integration tests for JWT authentication, CLI approval, public route registry, and controller endpoints.
* **Build process simplification:** The `cp` command is removed from the build process in Makefile. Binary compilation is streamlined to eliminate unnecessary file operations.
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
* **Documentation updates** - Updated gateway architecture, operator docs, and connection guides to reflect JWT-only auth and JIT user provisioning.

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
* **Sovereignty Boundary Plane** (`internal/services/sovereignty/`) - New first-class package
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
* **Binary build process** - "build once, then copy" pattern enforces a single compilation artifact
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

* **Sentinel encryption** - Sovereignty Boundary Plane encrypts sensitive fields before audit publishing; decrypts at authorized egress.
* **Fail-closed Audit Vault** - `AuditVaultService` now strictly rejects missing/malformed session IDs and unknown sessions prior to any audit writes, preventing invalid event relationships.
* **Bulk revocation with rate limiting** - Rapid revocation of compromised credentials at fleet scale without unbounded load.
* **mTLS for non-native apps** - App enrollment service extends mTLS enforcement to heterogeneous clients.
* **SPIFFE URI SAN hardening** - Fragile SPIFFE parsing that accepted malformed URIs on valid inputs fixed.
* **Unprotected transaction fix** - DB transactions that could expose inconsistent state under concurrency now properly bounded.
* **Receipt tampering detection** - Scenario tests verify the platform rejects tampered receipts across all governance layers.

---

# [0.2.7] - 2026-05-20

## Overview
Release **v0.2.7** separates the **Governance Gateway (`g8eg`)** and the **Governed Operator (`g8eo`)** into distinct roles, introduces an **MCP & A2A protocol translator gateway**, removes external runtime dependencies (`git` and `jq`), and improves overall security and developer experience.

## Key Changes

* **Gateway Role Splitting**: The Go Gateway is now explicitly split into the **Governance Gateway (`g8eg`)** acting as the central Policy Decision Point (PDP), and the **Governed Operator (`g8eo`)** acting as the host-side Policy Execution Point (PEP).
* **MCP & A2A Gateway**: `g8eo` can now act as a standalone admission gate for standard AI clients. It translates standard tool calls into governed transactions and supports out-of-band transaction suspension with WebAuthn approval before execution.
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
- **Protocol Refinement:** Ripped out legacy protobuf definitions, refined boundary structures, and decoupled operator auth from the app layer.
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
- **Actuator Execution Boundary:** Established `g8eo` Actuator as the authoritative execution boundary with signed action receipts.
- **Governance APIs:** Added first-class governance APIs for audit export and trust management.
- **Protobuf Module:** Introduced a unified `protocol/` directory with formal Protobuf module definitions.
- **Commitment Ledger:** Added definitions for the commitment ledger to support reputation staking.
- **Internal API Routing:** Established a unified internal router for component-to-component communication within `g8ee`.

### Changed
- **RequestContext Body Migration:** Migrated business context (`web_session_id`, `user_id`, `source_component`, etc.) from HTTP headers to body-embedded `RequestContext` objects for improved security and contract stability.
- **Directory Reorganization:** Renamed `components/` to `services/` and `shared/` to `protocol/` to align with the mandatory Gateway-first architecture.
- **g8ed Decommissioning:** Completed the removal of `g8ed` (Dashboard) remnants; migrated all core logic to the `g8eo` operator.
- **Auth Cleanup:** Refactored `APIKeyService` and passkey authentication for better consistency and security across the Gateway.
- **CodeQL Refactor:** Optimized CodeQL workflows and addressed findings in `event_service`.
- **Exit Code Handling:** Standardized exit code handling and improved path validation in `g8eo` execution services.
- **Event Service:** Consolidated `client_event_service` into a unified `event_service` within `g8eo`.
- **Improved Chaos Output:** Enhanced chaos test reporting for better failure visibility.

### Fixed
- **Operator TLS Hardening:** Refined operator TLS configuration and improved gateway service stability.
- **WebAuthn L3:** Fixed L3 verification issues following the `g8ed` decommissioning.
- **Path Resolution:** Improved path resolution and environment variable handling across the platform, including fixes in `paths.json`.
- **Test Stability:** Extensive fixes for unit and integration tests across `g8ee` and `g8eo`, particularly around the `RequestContext` migration and tribunal consensus.
- **Case Update Logic:** Fixed `CaseDataService.update_case` to correctly handle empty updates by ignoring the `context` field.

## [0.2.4] - 2026-05-13

### Added
- **Operator-Owned PKI/TLS:** Transitioned from legacy SSL to a robust CSR-based mTLS infrastructure owned by `g8eo`.
- **mTLS Enrollment:** New CSR and mTLS enrollment flow for operators and clients.
- **BYO Client Support:** Consolidated state root and added end-to-end support for "Bring Your Own" clients.
- **CLI Login:** Added first-class CLI login support via the operator.

### Changed
- **Gateway/App Layer Split:** Formalized `g8eo` as the mandatory Gateway and moved `client`/`g8ee` to optional application-layer adapters.
- **client Elimination:** Removed `client` Dashboard as a mandatory component; migrated data management scripts to `g8eo` API.
- **Governance Envelope Hardening:** Improved GovernanceEnvelope and proto definitions for better transaction integrity.
- **Reorganized g8eo:** Directory restructuring for better modularity and maintainability.
- **Passkey & Setup Refactor:** Migrated passkey and setup logic to the operator Gateway.

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
- **Removed Docker:** Eliminated Docker containerization across the platform. Components now run directly on the host with the Operator binary in listen mode.
- **Platform Architecture:** Migrated to host-native execution model with platform runtime state in repo-local `.g8e` directory.
- **Build System:** Comprehensive updates to `build.sh` for host-native bootstrapping, improved auth token handling, and better signal handling.
- **Documentation:** Updated all documentation to reflect the removal of Docker and the new host-native architecture.
- **Constants Paths:** Fixed and standardized constants paths across all components for better consistency.

### Fixed
- **Security:** Fixed SSRF vulnerability in Ollama model query endpoint.
- **Port Conflicts:** Resolved port conflict issues during platform startup.
- **Platform Commands:** Fixed g8e platform commands for proper host-native execution.
- **Build.sh:** Fixed auth token handling and kill signal processing in build scripts.
- **Test Suite:** Fixed test failures across g8ee, client, and g8eo after Docker removal.
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
- **Recursive Grep Tool:** Introduced `recursive_grep_search` for high-efficiency filesystem exploration across operator fleets.
- **Interrogation Gate:** Implemented a new gate in the agent loop that detects `<interrogation>` blocks and suppresses pending tool calls to prioritize user input.
- **Actuator Risk Analysis:** Enhanced risk classification logic for Actuator sub-agents with improved reputation staking and file-read security.
- **LFAA Audit Enhancements:** Refactored the Low-Fidelity Agentic Assistance audit recording to use typed Protobuf schemas.

### Changed
- **G8EO Protocol Hardening:** Hardened `g8eo` to reject malformed or non-envelope command bytes and enforce L1 `forbidden_patterns` via Protobuf reflection.
- **Tribunal 2.0 Pipeline:** Refactored the Tribunal consensus pipeline into a modular, stage-based architecture utilizing strict Protobuf-typed payloads and signatures.
- **G8eHttpContext Refactor:** Centralized and enforced strict security header validation (`web_session_id`, `user_id`, `source_component`) for all internal service communication.
- **Internal API Security:** Enforced strict component-identity verification and session-binding for internal component-to-component routing.
- **Operator Lifecycle:** Hardened operator slot management with atomic state transitions and reliable relaunch/activation logic.
- **Removed g8ep:** Eliminated the sidecar-managed `g8ep` operator node and `SupervisorService` in favor of external operators and unified slot management.
- **Standardized Cloud Subtype:** Standardized operator identification using `cloud_subtype` for consistency across cloud providers.

### Fixed
- **Actuator Risk Regression:** Resolved a regression where Actuator risk levels were incorrectly calculated in certain agent turns.
- **Interrogation Plumbing:** Fixed response handling and user interaction flow for the device interrogation pipeline.
- **G8EO Execution ID:** Fixed a bug where `FsGrepResultPayload` was missing `ExecutionID` propagation, breaking correlation for recursive searches.
- **Fingerprint Recording:** Resolved issues with system fingerprint recording and included missing events in the audit trail.
- **Test Coverage & Stability:** Massive increase in unit and integration test coverage for `g8ee`, `g8eo`, and `operator`, with full migration to typed payload assertions.

### Removed
- **Legacy Audit UI:** Removed the outdated Audit page and associated backend services from `client` in favor of streamlined platform logging.
- **"Available" Status:** Deprecated the "available" operator status as it was redundant for state management.

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
- **Operator Card:** Removed unnecessary animations from the operator card for better performance.
- **PR Template:** Updated the pull request template for better contributor guidelines.
- **Documentation:** General improvements to platform documentation, position paper, and `g8e-help`.

### Fixed
- **Interrogation Plumbing:** Fixed response handling and plumbing for the device interrogation flow.
- **Hamburger Menu:** Corrected the width and layout of the dashboard hamburger menu.
- **Fleet Demo:** Fixed configuration and deployment issues in the fleet demo profile.
- **Node Count & Bind All:** Fixed node counting logic for demos and moved the "Bind All" button to the top of the operator list.

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
- **Reputation Staking:** Implemented multi-phase reputation commitment and stake resolution for operator trust management.
- **Bug Fixes:** Resolved various issues across platform components for improved stability.

## [0.1.5] - 2026-04-28

### Added
- **Reputation System:** Introduced a multi-stage reputation and staking system, including `ReputationCommitment`, `ReputationState`, and `StakeResolution` models for trust-based operator management.
- **SSH Inventory Streaming:** New capability to stream and import operator inventory directly from local SSH configuration files.
- **Enhanced Test Fixtures:** Added `gold-set-schema.json` and `ledger-hash-fixtures.json` to improve consistency across platform evaluation suites.
- **Reputation CLI:** New administrative scripts `manage-reputation.py` and `seed-reputation-state.py` for platform governance.

### Changed
- **Tribunal 2.0 Governance:** Significant refactor of the Tribunal pipeline, implementing multi-phase consensus, detailed dissent recording, and improved safety guideline delivery.
- **Operator Authority Model:** Consolidated operator document handling and configuration delivery, positioning `g8ee` as the authoritative source for operator state.
- **Settings UX Overhaul:** Redesigned the Dashboard Settings page to match the Setup page layout, including improved command validation and status rendering.
- **Device Link Refactoring:** Streamlined device link management and added auto-approval logic for benign, non-mutating commands.
- **System Info & Heartbeat Synchronization:** Overhauled `SystemInfo` and `Heartbeat` wire models for better cross-component consistency and reduced payload size.

### Fixed
- **Authentication Loops:** Resolved edge cases in operator authentication and fixed internal routing issues during high-concurrency streams.
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
- **Operator Panel Documentation:** Comprehensive documentation for operator panel paths and features
- **Operator Panel Tests:** Added test coverage for operator panel path functionality

### Changed
- **Bound Session Refactoring:** Renamed `web_session_id` to `bound_web_session_id` across all services for clarity and consistency
- **SSE Validation:** Enhanced Server-Sent Events validation and wire/docs alignment
- **Heartbeat System:** Improved heartbeat data handling in g8ee and cleaned up flatten_for cruft
- **Metrics Delivery:** Enhanced metrics delivery to frontend for better operator monitoring
- **Tribunal Error Handling:** Consolidated Tribunal error-to-event-to-tool-call-failure flow for better error tracking
- **Temperature Configuration:** Cleaned up temperature settings to be persona-specific
- **Sentinel Configuration:** Sentinel is now always-on with updated documentation

### Fixed
- **CLI Authentication:** Improved CLI login flow and authentication handling
- **CLI Security:** Enhanced CLI security for Ollama-only setups
- **Operator Panel:** Fixed operator list display, bind/unbind all buttons, and public IP obfuscation
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
- **g8eo (Operator):** ~4MB dependency-free static Go binary for remote host execution. Features zero-inbound ports and outbound-only mTLS.
- **operator (Data Store):** SQLite-backed persistence layer, KV store, and pub/sub broker running within the Operator framework.
- **client (Dashboard):** Node.js central management console featuring FIDO2 WebAuthn (passkey) authentication and real-time mTLS gateway proxying.
- **Security:** "Tribunal Refinement Pipeline" utilizing stochastic swarm voting to validate AI-proposed terminal commands before human review.
- **Security:** Local execution vaulting to ensure raw stdout/stderr logs are securely encrypted and retained strictly on the target host.
- **DevOps:** Comprehensive `g8e` CLI wrapper for host-native platform lifecycle, testing, operator deployment, and CA certificate management.
