# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [0.2.5] - 2026-05-16

### Added
- **CLI Chat Wiring:** Implemented full CLI chat functionality (`./g8e chat`) with backend wiring to `g8ee` and unified stream handling.
- **Multi-Ledger Audit:** Implemented session-isolated Git audit ledgers for per-investigation transaction tracing.
- **Warden Execution Boundary:** Established `g8eo` Warden as the authoritative execution boundary with signed action receipts.
- **Governance APIs:** Added first-class governance APIs for audit export and trust management.
- **Protobuf Module:** Introduced a unified `protocol/` directory with formal Protobuf module definitions.
- **Commitment Ledger:** Added definitions for the commitment ledger to support reputation staking.
- **Internal API Routing:** Established a unified internal router for component-to-component communication within `g8ee`.

### Changed
- **RequestContext Body Migration:** Migrated business context (`web_session_id`, `user_id`, `source_component`, etc.) from HTTP headers to body-embedded `RequestContext` objects for improved security and contract stability.
- **Directory Reorganization:** Renamed `components/` to `services/` and `shared/` to `protocol/` to align with the mandatory substrate-first architecture.
- **g8ed Decommissioning:** Completed the removal of `g8ed` (Dashboard) remnants; migrated all core logic to the `g8eo` operator.
- **Auth Cleanup:** Refactored `ApiKeyService` and passkey authentication for better consistency and security across the substrate.
- **CodeQL Refactor:** Optimized CodeQL workflows and addressed findings in `event_service`.
- **Exit Code Handling:** Standardized exit code handling and improved path validation in `g8eo` execution services.
- **Event Service:** Consolidated `client_event_service` into a unified `event_service` within `g8eo`.
- **Improved Chaos Output:** Enhanced chaos test reporting for better failure visibility.

### Fixed
- **Operator TLS Hardening:** Refined operator TLS configuration and improved listener service stability.
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
- **Substrate/App Layer Split:** Formalized `g8eo` as the mandatory substrate and moved `client`/`g8ee` to optional application-layer adapters.
- **client Elimination:** Removed `client` Dashboard as a mandatory component; migrated data management scripts to `g8eo` API.
- **Governance Envelope Hardening:** Improved UAP and proto definitions for better transaction integrity.
- **Reorganized g8eo:** Directory restructuring for better modularity and maintainability.
- **Passkey & Setup Refactor:** Migrated passkey and setup logic to the operator substrate.

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
- **Warden Risk Analysis:** Enhanced risk classification logic for Warden sub-agents with improved reputation staking and file-read security.
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
- **Warden Risk Regression:** Resolved a regression where Warden risk levels were incorrectly calculated in certain agent turns.
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
- **Warden Prompts & Pathing:** Improved Warden sub-agent prompts and corrected file pathing behavior.
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
- **Warden Reputation Staking:** Warden sub-agents now stake reputation on risk classifications.
- **Setup UX:** Improvements to the onboarding wizard, ensuring validation visibility and cleaner summary view.
- **Python Modernization:** Migrated to `StrEnum` for improved type safety and performance across `g8ee`.

### Fixed
- **Device Link Scalability:** Increased `DEVICE_LINK_MAX_USES` to 10,000 to support large fleet registrations.
- **UI Robustness:** Improved error handling and icon rendering in the setup and status components.
- **Import Optimizations:** Resolved various circular import issues and optimized imports in `g8ee`.

## [0.1.7] - 2026-05-01

### Added
- **Warden Reputation Staking Improvements:** Enhanced reputation staking logic for Warden's risk assessments, including file read fixes and order handling.
- **Agent Cancellation:** Added support for cancelling agent tasks with dedicated UI controls and tests.

### Changed
- **Warden Personas & Context:** Refined Warden's context and personas for better risk evaluation.
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
- **Multi-Operator Batches:** `batch_id` correlation is now surfaced end-to-end — on `CommandExecutionResult`, approval metadata, and conversation message metadata — so agents and the dashboard can tie per-operator events and follow-up actions back to a single batched approval.
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
- **g8ee (AI Engine):** ReAct-based Python orchestration layer with support for Anthropic, OpenAI, and local Ollama models.
- **g8eo (Operator):** ~4MB dependency-free static Go binary for remote host execution. Features zero-inbound ports and outbound-only mTLS.
- **operator (Data Store):** SQLite-backed persistence layer, KV store, and pub/sub broker running within the Operator framework.
- **client (Dashboard):** Node.js central management console featuring FIDO2 WebAuthn (passkey) authentication and real-time mTLS gateway proxying.
- **Security:** "Tribunal Refinement Pipeline" utilizing stochastic swarm voting to validate AI-proposed terminal commands before human review.
- **Security:** Local execution vaulting to ensure raw stdout/stderr logs are securely encrypted and retained strictly on the target host.
- **DevOps:** Comprehensive `g8e` CLI wrapper for host-native platform lifecycle, testing, operator deployment, and CA certificate management.
