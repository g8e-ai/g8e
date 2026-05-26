# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

# [1.0.0] - 2026-05-25

## Overview
Release **v1.0.0** represents the maturation of g8e into a production-ready zero-trust governance substrate. This release completes the substrate-first architecture by removing the g8ee application layer from the substrate, dissolving the Sentinel component into protocol layers, and establishing a comprehensive scenario-based testing framework. The platform now operates as a pure governance substrate with typed, signed, state-bound transactions enforced through fail-closed L1/L2/L3 verification gates.

## Breaking Changes

* **g8ee Removal from Substrate**: Completely removed the g8ee (Engine) application layer from the substrate. g8ee is now a standalone optional application adapter that must be run separately from the governance substrate.
* **Sentinel Dissolution**: Dissolved the Sentinel component into L1 Doctrine (Technical Bedrock) protocol layers. Threat detection logic now executes within the L4 Warden gate, and data sovereignty logic moved to a new Sovereignty Boundary Plane package.
* **Raw Vault Removal**: Removed the raw audit vault in favor of sentinel-moderated vault storage. All audit data now passes through Sentinel scrubbing/rehydration logic.
* **Cursor-Based Query Elimination**: Removed cursor-based query patterns in favor of optimized direct database access patterns.
* **Session ID Field Migration**: Migrated from `web_session_id` to `cli_session_id` across all logging, events, and constants for consistency with CLI-first architecture.
* **Constants Regeneration**: All Go constants regenerated with deterministic sorting and new collection definitions (app_policies, storage events, operator field read events).

## Added

* **Scenario Testing Framework**: Comprehensive scenario-based testing suite with 39 test fixtures covering governance gates (L1/L2/L3 validation), security scenarios (forged signatures, stale state roots), and receipt verification. Includes golden file assertions and fixture generation tooling.
* **A2A Gateway Tests**: Added A2A (Agent-to-Agent) protocol translator gateway tests with real operator integration.
* **Bulk Certificate Revocation**: Implemented bulk certificate revocation with rate limiting for large-scale fleet management.
* **SPIFFE URI SAN Parsing**: Enhanced SPIFFE URI SAN parsing with improved fragility handling and format validation.
* **External App mTLS Integration**: mTLS support for non-native applications integrating with the substrate.
* **Sentinel Encryption**: Added Sentinel encryption layer for data sovereignty with BYO client test coverage.
* **App Policies Collection**: New `app_policies` collection in constants for application-layer policy governance.
* **Clock Injection**: Test-time clock injection for deterministic testing of time-sensitive operations.
* **Deterministic Constant Generation**: Implemented deterministic sorting for Go constant exporter to ensure reproducible builds.
* **Headers Generation Tracking**: Added `headers_generated.go` tracking to constant registry check system.
* **Unit and Integration Test Subcommands**: Separated unit and integration test execution into distinct CLI subcommands for better CI/CD control.
* **Test Parallelism**: Improved test parallelism with session definition fixes and chaos test count corrections.
* **Tamper Receipt Tests**: Added comprehensive tests for receipt tampering detection and verification.
* **Gauntlet Tests**: Added Gauntlet framework tests for governance pipeline validation.
* **Chaos Tester Tests**: Enhanced chaos testing with dedicated test coverage.
* **Exporter Tests**: Added test coverage for the exporter component.
* **Platform Start Shorthand**: Added `-a` shorthand flag to `platform start` command for faster startup.

## Changed

* **Governance File Structure**: Improved governance file structure organization for better maintainability.
* **Tribunal and Consensus Separation**: Clarified separation between Tribunal (L2 consensus) and consensus mechanisms for architectural clarity.
* **Doctrine and Sentinel Separation**: Better separation between L1 Doctrine definitions and Sentinel enforcement logic.
* **DB Optimizations**: Database query optimizations including removal of cursor-based patterns and unprotected transaction fixes.
* **PKI Test Initialization**: Refactored PKI test initialization for better reliability and isolation.
* **Binary Building**: Improved binary building process with "one build, then copy" pattern for consistency.
* **Platform Reset Fix**: Fixed platform reset command to properly clean all substrate state.
* **CLI Auth Improvements**: Enhanced CLI authentication flow and error handling.
* **CLI Menu Organization**: Moved setup to top of menu, cleaned up menu structure for better UX.
* **Gateway Start and Login**: Improved gateway startup sequence and login flow reliability.
* **Test Constants Fix**: Fixed test constant generation and resolution issues.
* **Docs and Status**: Improved documentation and platform status reporting.
* **Docs Pipeline**: Complete documentation pipeline reorganization with script cleanup.
* **Mkdocs Improvements**: Migrated to built-in readthedocs theme, removing external theme dependency.
* **Dev Setup Improvements**: Enhanced developer setup documentation and tooling.
* **mTLS Bootstrap**: Fixed mTLS bootstrap process for first-time setup.
* **Operator Build Tooling**: Improved operator binary build tooling and reliability.
* **Sentinel Data Handling**: Enhanced Sentinel data handling with local store integration.
* **Concurrency Improvements**: Fixed concurrency issues in various services.
* **Constants Casing**: Standardized constants casing including acronym fixes.
* **MCP and A2A Documentation**: Improved documentation for MCP and A2A protocol integration.
* **Codemap Updates**: Updated architecture codemaps to reflect new structure.
* **Directory Structure**: Updated directory structure to align with substrate-first architecture.
* **Local Directory Fix**: Fixed local directory resolution issues.
* **Header Fix**: Fixed header generation and tracking issues.
* **Logout and Platform Tests**: Fixed logout functionality and platform test suite.
* **Protobuf Version**: Upgraded to Protobuf version 1.35.2 with toolchain mismatch fixes.
* **Go Lints**: Applied comprehensive Go linting fixes across the codebase.
* **Test Coverage**: Massive test coverage improvements across pubsub, auth, UAP, interfaces, models, responder, API clients, security, and CLI components.
* **Unprotected Transaction Fix**: Fixed unprotected database transactions that could lead to data inconsistency.
* **Decouple App Policy**: Decoupled application policy logic from substrate core.
* **Strongly Typed Mutations**: Enhanced strongly typed mutation enforcement.
* **External App mTLS**: Improved mTLS integration for external applications.
* **Wire Up Sentinel to LocalStore**: Integrated Sentinel with local storage layer.
* **Move VaultModerateRaw to SentinelModerateRaw**: Renamed vault moderation functions to reflect Sentinel ownership.
* **Remove Raw Vault**: Completely removed raw audit vault in favor of Sentinel-moderated storage.
* **Fix Hardcoded Test Cert**: Fixed hardcoded test certificate usage for proper test isolation.
* **Exclude Proto from Tests**: Excluded protobuf-generated code from test coverage calculations.
* **Exclude Mocks from Tests**: Excluded mock code from test coverage calculations.
* **Fix Gov Mock and Coverage Report**: Fixed governance mocks and coverage reporting accuracy.
* **SPIFFE Format in Tests**: Standardized SPIFFE format usage in test fixtures.
* **Fix SPIFFE Parsing Fragility**: Fixed fragile SPIFFE URI parsing logic.
* **Improve Test Session Definitions**: Enhanced test session definition handling.
* **Improve Scenario Tests**: Improved scenario test framework reliability.
* **Fix Chaos Test Count**: Fixed chaos test counting logic.
* **Mode X Truth Table Fixtures**: Added Mode X truth table test fixtures for governance validation.
* **Finish Adding Scenario Tests**: Completed scenario test suite implementation.
* **Unit and Integration Test Subcommands**: Split test execution into unit and integration subcommands.
* **Improve Governance File Structure**: Reorganized governance-related files for better structure.
* **Clear Consensus Definition**: Clarified consensus mechanism definitions.
* **Dissolve Sentinel into Proto Layers**: Moved Sentinel logic into protocol layer definitions.
* **Docs Update**: Comprehensive documentation updates reflecting architectural changes.
* **More Doc Reorgs**: Multiple documentation reorganization passes.
* **Clean Up Docs Pipeline**: Streamlined documentation generation pipeline.
* **Doc Reorg**: Major documentation reorganization effort.
* **Tribunal Cleanup**: Cleaned up Tribunal implementation and removed dead code.
* **Protocol Gen Doc Fix**: Fixed protocol documentation generation.
* **More Tribunal Cleanup**: Additional Tribunal cleanup and simplification.
* **Clearer Separation of Tribunal and Consensus**: Improved architectural separation.
* **Improve Test Session Definitions**: Enhanced test session handling.
* **Tamper Receipt Test**: Added receipt tampering detection tests.
* **Fix Gauntlet Tests**: Fixed Gauntlet framework test failures.
* **Improve GW Start and Login Fix**: Fixed gateway startup and login issues.
* **Mkdocs Improvements**: Enhanced Mkdocs configuration and theming.

## Removed

* **g8ee from Substrate**: Completely removed g8ee (Engine) from the substrate codebase and root Makefile.
* **g8ee Environment Dependencies**: Cleaned up g8ee-specific environment variable dependencies.
* **Shell Scripts**: Removed unnecessary shell scripts in favor of Go-native implementations.
* **Entrypoint.sh Remnants**: Removed remaining entrypoint.sh script remnants.
* **Raw Vault**: Removed raw audit vault storage layer.
* **Cursor-Based Queries**: Eliminated cursor-based database query patterns.
* **Unnecessary Scripts**: Cleaned up unnecessary scripts throughout the codebase.
* **Vendor Dependencies**: Removed vendored gotestsum dependencies in favor of direct tooling.

## Fixed

* **Unprotected Transactions**: Fixed database transactions that were not properly protected.
* **SPIFFE Parsing Fragility**: Fixed fragile SPIFFE URI parsing that could fail on valid inputs.
* **Hardcoded Test Cert**: Removed hardcoded test certificate for proper test isolation.
* **Platform Reset**: Fixed platform reset command to clean all state.
* **Logout and Platform Tests**: Fixed logout functionality and platform test failures.
* **Chaos Test Count**: Fixed incorrect chaos test counting logic.
* **Local Directory Resolution**: Fixed local directory path resolution issues.
* **Header Generation**: Fixed header generation and tracking in constant registry.
* **Test Constants**: Fixed test constant generation and resolution.
* **mTLS Bootstrap**: Fixed mTLS bootstrap process for first-time setup.
* **CLI Auth**: Fixed CLI authentication flow issues.
* **Gateway Start**: Fixed gateway startup sequence.
* **Concurrency Issues**: Fixed various concurrency race conditions.
* **DB Optimizations**: Fixed database performance and correctness issues.
* **PKI Test Init**: Fixed PKI test initialization for better isolation.
* **Binary Building**: Fixed binary build process issues.
* **Docs Pipeline**: Fixed documentation generation pipeline issues.
* **Protobuf Toolchain**: Fixed Protobuf toolchain version mismatch.
* **Go Lints**: Fixed Go linting issues across the codebase.
* **Test Coverage**: Fixed test coverage calculation and reporting.
* **Gov Mock**: Fixed governance mock implementations.
* **SPIFFE Format**: Fixed SPIFFE format inconsistencies in tests.
* **Scenario Tests**: Fixed scenario test framework issues.
* **Gauntlet Tests**: Fixed Gauntlet test failures.
* **Tamper Receipt Test**: Fixed receipt tampering detection tests.
* **Test Session Definitions**: Fixed test session definition handling.
* **Clock Injection**: Fixed clock injection for deterministic testing.
* **Constants Casing**: Fixed inconsistent constants casing.
* **Acronym Casing**: Fixed acronym casing in constants.
* **External App mTLS**: Fixed mTLS integration for external apps.
* **Sentinel Encryption**: Fixed Sentinel encryption layer issues.
* **BYO Client Test**: Fixed BYO client test failures.
* **Constants Generation**: Fixed constants generation and sorting.
* **Headers Generated Tracking**: Fixed tracking of generated headers.
* **App Policies Collection**: Fixed app_policies collection integration.
* **Remove g8ee References**: Fixed remaining g8ee references in root Makefile.
* **Env Var Dep Cleanup**: Fixed environment variable dependency cleanup.
* **SH Scripts Removal**: Fixed issues after shell script removal.
* **Entrypoint Remnants**: Fixed issues after entrypoint.sh removal.
* **Headers and Makefile Print**: Fixed header generation and Makefile output.
* **Pubsub and Auth Test Coverage**: Fixed test coverage for pubsub and auth.
* **UAP Test Coverage**: Fixed UAP (Universal Action Protocol) test coverage.
* **Exclude Proto from Tests**: Fixed protobuf exclusion from test coverage.
* **Exclude Mocks from Tests**: Fixed mock exclusion from test coverage.
* **Interfaces Test Coverage**: Fixed interfaces test coverage.
* **Models Test Coverage**: Fixed models test coverage.
* **Responder Test Coverage**: Fixed responder test coverage.
* **API and Auth Clients Coverage**: Fixed API and auth client test coverage.
* **Security Test Coverage**: Fixed security test coverage.
* **CLI Test Coverage**: Fixed CLI test coverage.
* **More Test Coverage**: Fixed general test coverage issues.
* **Exporter Test**: Fixed exporter test failures.
* **Chaos Tester Tests**: Fixed chaos tester test failures.
* **Logout and Platform Tests**: Fixed logout and platform test failures.
* **Fix Gov Mock and Coverage Report**: Fixed governance mock and coverage reporting.
* **Add -a Shorthand**: Fixed platform start shorthand flag.
* **SPIFFE Format in Tests**: Fixed SPIFFE format in test fixtures.
* **Bulk Revocation**: Fixed bulk certificate revocation implementation.
* **Rate Limiting**: Fixed rate limiting implementation.
* **SPIFFE Parsing**: Fixed SPIFFE URI parsing implementation.
* **Decouple App Policy**: Fixed app policy decoupling.
* **Strongly Typed Mutations**: Fixed strongly typed mutation enforcement.
* **External App mTLS Integration**: Fixed external app mTLS integration.
* **Improve Concurrency**: Fixed concurrency issues.
* **Elim Cursor Based Query**: Fixed cursor-based query removal.
* **More Constants Casing**: Fixed constants casing issues.
* **Fix Acronym Casing**: Fixed acronym casing.
* **Improve Constants**: Fixed constants generation.
* **MCP and A2A Doc**: Fixed MCP and A2A documentation.
* **Concurrency Fix**: Fixed concurrency issues.
* **MV VaultModerateRaw to SentinelModerateRaw**: Fixed vault moderation function renaming.
* **Wire Up Sentinel to LocalStore**: Fixed Sentinel local store integration.
* **RM Raw Vault**: Fixed raw vault removal.
* **DB Optimizations**: Fixed database optimizations.
* **Fix Unprotected Txns**: Fixed unprotected transaction issues.
* **Refactor PKI Test Init**: Fixed PKI test initialization.
* **mTLS for Non-Native Apps**: Fixed mTLS for non-native apps.
* **Sentinel Encrypt**: Fixed Sentinel encryption.
* **BYO Client Test**: Fixed BYO client tests.
* **Constants Generation**: Fixed constants generation.
* **One Build Then CP**: Fixed build process.
* **Platform Reset Fix**: Fixed platform reset.
* **Test Constants Fix**: Fixed test constants.
* **Improve Docs and Status**: Fixed documentation and status.
* **More CLI Improvements**: Fixed CLI improvements.
* **Exec Service Fixes**: Fixed execution service issues.
* **Docs Pipeline**: Fixed documentation pipeline.
* **RM Scripts**: Fixed script removal.
* **Cleanup Unnecessary Scripts**: Fixed unnecessary script cleanup.
* **A More Sovereign Binary**: Fixed binary sovereignty.
* **Add --path-prefix Flag**: Fixed golangci-lint path prefix.
* **Add Operator Field Read Events**: Fixed operator field read events.
* **Regenerate Constants Registry**: Fixed constants registry regeneration.
* **Commit Generated Channels Constants**: Fixed generated channels constants.
* **Update Logger Field Names**: Fixed logger field names.
* **Add Storage Events**: Fixed storage events.
* **Docs Pipeline**: Fixed documentation pipeline.
* **RM Scripts**: Fixed script removal.
* **Docs Update Scripts Updates**: Fixed documentation update scripts.
* **Migrate to Built-in ReadTheDocs Theme**: Fixed theme migration.
* **Fix Helps**: Fixed help text.
* **New Dev Setup Improvements**: Fixed dev setup.
* **Fix mTLS Bootstrap**: Fixed mTLS bootstrap.
* **Move Setup to Top of Menu**: Fixed menu organization.
* **Clean Menu**: Fixed menu cleanup.
* **Sentinel Data Handling**: Fixed Sentinel data handling.
* **Improve Operator Build Tooling**: Fixed operator build tooling.

## Security

* **Bulk Certificate Revocation**: Added bulk certificate revocation with rate limiting for rapid response to compromised credentials.
* **Sentinel Encryption**: Enhanced data sovereignty with Sentinel encryption layer for sensitive audit data.
* **mTLS for Non-Native Apps**: Extended mTLS enforcement to non-native applications integrating with the substrate.
* **SPIFFE URI SAN Hardening**: Improved SPIFFE URI SAN parsing with better validation and fragility fixes.
* **Unprotected Transaction Fix**: Fixed database transactions that could expose inconsistent state.
* **Receipt Tampering Detection**: Added comprehensive tests for receipt tampering detection.
* **Mode X Truth Table Fixtures**: Added security fixtures for Mode X governance validation.
* **Security Test Coverage**: Improved security test coverage across all components.

## Testing

* **Scenario Testing Framework**: New comprehensive scenario-based testing suite with 39 fixtures covering governance gates, security scenarios, and receipt verification.
* **A2A Gateway Tests**: Added A2A protocol translator gateway tests with real operator integration.
* **Tamper Receipt Tests**: Added receipt tampering detection and verification tests.
* **Gauntlet Tests**: Added Gauntlet framework tests for governance pipeline validation.
* **Chaos Tester Tests**: Enhanced chaos testing with dedicated test coverage.
* **Test Coverage Improvements**: Massive test coverage improvements across pubsub, auth, UAP, interfaces, models, responder, API clients, security, and CLI components.
* **Unit and Integration Test Subcommands**: Separated unit and integration test execution for better CI/CD control.
* **Test Parallelism**: Improved test parallelism with session definition fixes.
* **Clock Injection**: Added test-time clock injection for deterministic testing.
* **Golden File Assertions**: Added golden file assertions for scenario tests.
* **Fixture Generation Tooling**: Added tooling for generating test fixtures.
* **Exclude Proto and Mocks**: Excluded protobuf-generated code and mocks from test coverage calculations.

## Documentation

* **Complete Documentation Reorganization**: Major documentation reorganization reflecting substrate-first architecture.
* **Mkdocs Migration**: Migrated to built-in readthedocs theme, removing external theme dependency.
* **MCP and A2A Documentation**: Improved documentation for MCP and A2A protocol integration.
* **Dev Setup Improvements**: Enhanced developer setup documentation and tooling.
* **Codemap Updates**: Updated architecture codemaps to reflect new structure.
* **Docs Pipeline**: Streamlined documentation generation pipeline with script cleanup.
* **Governance File Structure**: Improved governance file structure documentation.
* **Tribunal and Consensus Separation**: Clarified separation between Tribunal and consensus in documentation.
* **Doctrine and Sentinel Separation**: Better documentation of L1 Doctrine and Sentinel separation.

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
- **CLI & UX Improvements:** Improved login UX, operator-side UX, and trust script stability. Enhanced build output for Mac and Linux.
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
- **Governance Envelope Hardening:** Improved UAP and proto definitions for better transaction integrity.
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
- **Evals Refactor:** Streamlined evaluation device token management to be runtime-configurable.
- **Improved Setup:** Enhanced environment variable handling and validation during bootstrap.

### Fixed
- **Documentation:** Fixed various typos and inconsistencies across architecture documentation.

## [0.2.2] - 2026-05-10

### Added
- **Ollama Model Query:** Added support for querying available Ollama models during setup with improved UI feedback.
- **Runtime Device Tokens:** Evals device tokens are now set at runtime instead of build time for improved security and flexibility.
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
- **Protobuf-Driven Architecture:** Massively migrated the platform to a robust, typed Protobuf-driven architecture for payloads, while maintaining a UAP JSON-first transport for mutation envelopes.
- **Governance Envelope:** Introduced the JSON `GovernanceEnvelope` (UAP) for all BFT transactions, binding event metadata, state roots, and hardware-bound fingerprints.
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
