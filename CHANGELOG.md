# Changelog

Release notes and updates for the g8e platform.

---

## v5.0.0 — Public Launch — 2026-04-10

Initial public release of g8e on GitHub. This release marks the platform's debut as a self-hosted governance layer for AI agents with human-in-the-loop approval gates.

### Launch Readiness
- **Fixed** — Placeholder URLs updated from `g8e.local` to `g8e.ai` across README, SECURITY.md, and CODE_OF_CONDUCT.md
- **Fixed** — Broken Markdown link in README.md corrected
- **Improved** — `.gitignore` now covers secret file patterns (.key, .pem, .crt, .secret)
- **Verified** — Security audit complete: no hardcoded secrets or leaked credentials in codebase or git history
- **Verified** — Full test suite green: 4360 tests passing across VSA, VSOD, and g8ee components

### Documentation
- **Verified** — All documentation links resolve correctly
- **Aligned** — VERSION file synchronized with CHANGELOG.md

---

## v4.3.0 — Operator Activation, Tribunal Hardening & Platform Reliability — 2026-04-07

Resolves critical production bugs across the operator activation path, the Tribunal command-safety pipeline, and the pub/sub subsystem. Introduces real-time command lifecycle events, a one-command operator g8e script, a redesigned Getting Started onboarding flow, and LLM CLI flags for AI integration testing. Includes a sweeping code quality pass removing the `PendingCommand` DB-polling model in favour of a fully event-driven execution registry.

### Operator & Execution
- **Fixed** — **Operator Activation** — `_completeAuthentication` now completes the full activation lifecycle: claims slot, updates user record, and relays to g8ee with a proper `VSOHttpContext`. Operators no longer get stuck in a permanently inactive state after authentication.
- **Fixed** — **Operator Status on Re-auth** — BOUND operators that re-authenticate now preserve BOUND status instead of being downgraded to ACTIVE.
- **Fixed** — **Post-Login Race Condition** — `initializeOperatorSlots` and `activateG8ENodeOperatorForUser` now run sequentially, preventing the g8e-pod operator from silently skipping activation.
- **Improved** — **Execution Registry** — Replaced `PendingCommand` DB-polling loop with a fully event-driven in-memory result stash. `execution_service.py` waits once via `asyncio.Event` and reads the result directly — no polling, no DB round-trips.
- **Improved** — **`resolveBoundOperators`** — Rewrote to a single cache-aside read + parallel `Promise.all` fetch, eliminating N+1 queries and the `operatorSessionService` dependency.
- **New** — **Real-Time Command Lifecycle Events** — All operator services (command, file, filesystem, intent, port) now broadcast `STARTED`, `COMPLETED`, and `FAILED` events via `EventService.publish_command_event()`, enabling real-time dashboard progress updates for every operation.

### AI & Tribunal
- **Fixed** — **Tribunal Provider/Model Mismatch** — Added `_infer_provider_for_model()` and `_resolve_provider_and_model()` to ensure the correct LLM provider is selected for the resolved model, not just the configured default.
- **Fixed** — **Tribunal Silent Fallback** — Infrastructure failures (auth errors, DNS, SSL) in the Tribunal now raise `TribunalSystemError` instead of silently falling back to the original command. Pass errors are now collected and surfaced.
- **Fixed** — **Tribunal `NameError`** — `types.Role.USER` referenced an unimported `types` module; `Role` is now imported directly.
- **Fixed** — **LLM SSL Isolation** — Cloud LLM API calls (Gemini, Anthropic, OpenAI) now use the Mozilla CA bundle instead of the internal platform CA, preventing the container-global `G8E_SSL_CERT_FILE` from breaking cloud API calls.
- **Fixed** — **`llm_command_gen_passes` TypeError** — `max(1, None)` crashed all AI tool calls; field now defaults to `3` with proper None-guards in settings service.
- **New** — **`AIGenerationConfigBuilder`** — Extracted stateless config factory for `GenerateContentConfig` objects. Supports thinking-level detection, lite configs for triage/memory/risk analysis, schema-enforced structured JSON output, and flexible JSON mode for local models.
- **Improved** — **Memory Generation** — `MemoryGenerationService` enhanced with conversation-to-memory AI analysis, structured output parsing, and proper investigation memory lifecycle management.

### Approvals
- **Improved** — **Typed Approval API** — `OperatorApprovalService` takes typed request/response models (`CommandApprovalRequest`, `FileEditApprovalRequest`, `IntentApprovalRequest`, `OperatorApprovalResponse`) replacing 18-kwarg dispatcher and 5-arg response handler.
- **Fixed** — **Approval HTTP 500** — `ApprovalRespondRequest` fields `operator_session_id`/`operator_id` were `required: true`; changed to `default: null` since the server resolves them.
- **Fixed** — **Approval route 500** — Route was accessing `req.services.operatorService` which was never attached; fixed by constructing `OperatorRelayService` directly in the route constructor.

### Pub/Sub
- **New** — **`shared/constants/pubsub.json`** — Canonical wire-protocol constants shared across g8ee, VSA, and VSOD with contract tests in all three languages.
- **New** — **`EventService`** — Extracted `vsod_event_service.py` with typed `publish_command_event()` and `publish_investigation_event()` methods for all operator service SSE broadcasting.
- **Fixed** — **PubSub Subscribe Timeout** — `PubSubMessageType` enum was missing `MESSAGE`, `PMESSAGE`, `SUBSCRIBED` wire members, causing `AttributeError` in `_ws_reader` which silently killed the task. All `subscribe()` calls timed out after 5 seconds.
- **Fixed** — **`psubscribe()` Race Condition** — Channel tracking now occurs after `_ensure_ws()` and ACK handler setup, preventing double-subscription on reconnect.
- **Improved** — **PubSub Reconnect** — `_ws_reader` now schedules exponential-backoff reconnect on disconnect when active subscriptions exist.

### Dashboard & Operator Deployment
- **New** — **Operator Drop Script** — `GET /g8e` serves a one-command POSIX shell script that installs the CA cert, fetches the operator binary (auto-detects architecture), and launches the operator.
- **Improved** — **Getting Started UI** — Replaced CLI reference text in the operator deployment panel with a structured three-step onboarding guide. Getting started content now also appears in the empty chat view.
- **Improved** — **SSL Trust Scripts** — `certInstallerScript` (Windows and macOS) now checks exit codes and exits with descriptive error messages on failure.

### Data Integrity
- **New** — **`KVOperationError`** — `keys()` and `scan()` on the KV cache client now throw `KVOperationError` on failure instead of silently returning empty results. Prevents stale query cache entries from serving empty operator lists.
- **New** — **`shared/constants/document_ids.json`** — Canonical document ID constants (`platform_settings`, `user_settings_` prefix) shared across g8ee and VSOD.

### Testing & DX
- **New** — **LLM CLI Flags** — `./g8e test` now accepts `--llm-provider`, `--primary-model`, `--assistant-model`, `--llm-endpoint-url`, `--llm-api-key` for running AI integration tests without VSODB.
- **Improved** — Test coverage expanded across Tribunal, pub/sub, provider SSL, operator activation, approval routing, port service, terminal output rendering, and KV cache error handling.

---

## v4.3.1 — Passkey Auth & Operator Panel Fixes — 2026-04-08

Hotfix release resolving two critical bugs affecting user authentication and operator panel real-time updates.

### Authentication
- **Fixed** — **Passkey Auth Response** — `PasskeyVerifyResponse` now includes `success: true` field. The response model was missing this field, causing frontend validation failures during the passkey authentication flow.
- **Fixed** — **Passkey Route Response** — Passkey routes now properly return the complete response shape including the `success` field.

### Operator Panel & SSE
- **Fixed** — **SSE Push Missing user_id** — `SSEPushRequest` now requires `user_id` as a mandatory field. Previously, SSE event pushes from g8ee to VSOD were missing the `user_id`, causing operator panel updates to fail silently.
- **Fixed** — **Heartbeat Service Validation** — `HeartbeatService` now validates both `web_session_id` and `user_id` before pushing SSE events. Missing `user_id` now logs a warning and skips the push instead of failing.
- **Fixed** — **Operator Panel List Updated Event** — Internal SSE route now properly constructs `OperatorListUpdatedEvent` from the operator list payload instead of passing the raw result object.

### g8ee Event Publishing
- **Fixed** — All g8ee services now include `user_id` when publishing events via `vsod_event_service`: `AgentSSEService`, `AgentToolLoop`, `ChatPipeline`, `ChatTaskManager`, `CommandGenerator`, `ApprovalService`, and `HeartbeatService`.

### Demo & Documentation
- **Improved** — Demo fleet SSH streaming configuration automated in `make up` with proper SSH key and hosts file setup.
- **Fixed** — Terminal CSS max-width constraints to prevent overflow on wide screens.

---

## v4.2.0 — Platform Hardening & Architecture Cleanup — 2026-04-06

Major stabilization release following the v4.0 platform rebuild. Sweeping internal refactor that eliminates architectural debt, fixes critical production bugs, and introduces a redesigned binary distribution system.

### Architecture & Refactoring
- **Removed** — **EventSource Abstraction** — Eliminated the `EventSource` constants layer entirely. All event routing now uses `EventType` directly, fixing an entire class of bugs where the browser-native `EventSource` API was confused with the internal constants object.
- **Redesigned** — **Operator Binary Distribution** — Binaries are now cross-compiled for all architectures with UPX compression at VSODB build time and distributed via the blob store. g8e-pod fetches on startup with retry logic.
- **New** — **`./g8e platform setup`** — First-time setup command that orchestrates a full build with correct startup ordering.
- **Fixed** — **Operator Version Injection** — Operator binary now reports the correct platform version instead of `dev`. Version injected via Go ldflags across all build paths (VSODB Dockerfile, Makefile container and local targets).
- **Improved** — **Code Quality** — Removed unnecessary abstractions, dead code, legacy fields, and environment-specific test config. Implemented `HttpService` protocol and `CacheAsideProtocol`.

### AI & Dashboard
- **New** — **Dual LLM Model Selection** — Split single model dropdown into Primary (complex tasks) and Assistant (simple tasks) with triage-based routing.
- **Fixed** — **Chat Pipeline** — Restored operator offline workflow; chat is fully functional end-to-end.
- **Fixed** — **SSE Connection Manager** — Fixed `TypeError` caused by EventSource refactor mangling the browser `EventSource` API.

### Bug Fixes
- **Fixed** — **Operator Execution** — Execution path failures, duplicate API key issuance, CA certificate bootstrap (chicken-and-egg TLS problem).
- **Fixed** — **VSOD** — Setup page 500 error, settings loading, investigation query construction, text completion handling, VSODB client alignment.
- **Fixed** — **Internal Auth** — KV endpoint authentication, redundant per-event operator resolution removed from SSE route.
- **Fixed** — **Logger** — Date objects rendering as `{}` in logs; `redactPii` now skips non-plain objects.

### Testing & Documentation
- **Improved** — VSOD test suite restructured and expanded. g8ee integration tests expanded with SSE error paths and retry loop coverage.
- **Improved** — Full documentation audit with corrections across security, architecture, and component docs.

---

## v4.1.0 — Execution & Intelligence Refinement — 2026-04-03

Focused on improving AI interaction reliability, execution tracing, and VSA listen mode testability.

### AI & Execution
- **Improved** — **Gemini Streaming & Multi-turn** — Fixed function call streaming and state management for multi-turn conversations.
- **Improved** — **Tool Call & Declaration Cleanup** — Standardized tool definitions across g8ee for more reliable model interactions.
- **New** — **Execution ID Tracing** — Implemented consistent `execution_id` generation and propagation for better auditability.
- **Refined** — **Payload Typing** — Strict model definitions for execution results and command payloads.

### Component Improvements
- **VSA** — Enhanced listen mode testability and internal auth token handling.
- **g8ee** — Fixed DB client token loading and settings definition synchronization.
- **VSOD** — Improved diagram generation and API endpoint alignment.

### CI/CD & DX
- **New** — GitHub Actions workflow for PRs.
- **Improved** — Simplified CI environment and local test parity.
- **Fixed** — Standardized `setup-llm.sh` and test coverage reporting.

---

## v4.0.0 — The Portable AI Ops Platform — 2026-04-02

The most significant release since launch. g8e is now 100% self-hosted with no external service dependencies. The platform has been entirely rebuilt around the 4MB Operator as the backend data plane. This release introduces the unified CLI, concurrent SSH streaming, and the complete admin console for fleet management.

### Platform & Infrastructure
- **New** — **4MB Operator as Backend** — The Operator now serves as the backend data plane for the entire platform, handling SQLite persistence, KV caching, and WebSocket pub/sub.
- **New** — **`./g8e` Unified CLI** — Single entry point for all platform operations. Only Docker is required on the host.
- **New** — **g8e-pod Execution Sandbox** — Isolated container for all toolchain operations (builds, tests, security scans).
- **New** — **Admin Console** — Complete administrative interface (`/console`) with real-time platform metrics and component health monitoring.
- **New** — **Full Documentation in Repo** — All platform documentation now ships inside the repository under `docs/`.

### AI & Execution
- **New** — **Tribunal** — 2-of-3 small language model voting for command safety validation.
- **New** — **Operator SSH Streaming** — Concurrent, ephemeral deployment via Go-native SSH with zero footprint.
- **New** — **Full Context Mode** — Dynamic system prompts that incorporate past conversations and user communication style.
- **Improved** — **LLM Provider Support** — Agnostic support for Gemini, Anthropic, OpenAI, and Ollama. Gemini 3.1+ recommended.

### Security & Governance
- **New** — **Local-First Audit Architecture (LFAA)** — Audit logs and command history stay locked in local vaults.
- **New** — **Sentinel Threat Detection** — Pre and post-execution hooks with 50+ threat detectors mapped to MITRE ATT&CK.
- **New** — **Zero-Trust Stealth** — Outbound-only connectivity over port 443 with zero listening ports.
- **New** — **Human-in-the-Loop** — Mandatory approval for all state-changing operations.
- **Enhanced** — **RBAC & Security** — Granular roles (Standard, Administrator, Super-Admin) and FIDO2 Passkey-only authentication.

---

## v3.0.0 — Sentinel Evolution — 2026-03-02

Major release introducing enhanced Sentinel capabilities and aligning with NSA Zero Trust Implementation Guidelines.

- **New** — **Sentinel Pre-Execution Threat Detection** — Command analysis mapped to MITRE ATT&CK patterns.
- **New** — **Dual-Vault Scrubbing** — Automated scrubbing of credentials and PII before AI processing.
- **New** — **Local-First Audit Architecture** — Initial implementation of raw data staying on-premise.
- **Improved** — **NSA ZIG Alignment** — Exceeding requirements in 6 of 7 Zero Trust pillars.

---

## v2.0.0 — Multi-Operator Orchestration — 2026-01-22

Introduction of multi-operator binding and industry-first zero-standing privileges for AWS.

- **New** — **Multi-Operator Binding** — Coordinated operations across multiple systems from a single chat session.
- **New** — **Zero-Standing Privileges** — 2-role approach for AWS separating execution from permission authority.
- **New** — **Unified Batch Execution** — Run commands across multiple Operators with a single human approval.

---

## v1.0.0 — Zero-Trust AI for Real Production — 2025-12-16

Initial release of g8e. First enterprise-ready platform designed as a secure execution agent with full human-in-the-loop control.

- **New** — **Human-in-the-Loop Execution** — Every command requires explicit human approval.
- **New** — **AI-Powered Command Safety** — Risk analysis (LOW/MEDIUM/HIGH) for every proposed action.
- **New** — **Zero Inbound Access** — Outbound-only communication via HTTPS; no open ports.
