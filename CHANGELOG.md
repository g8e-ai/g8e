# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [0.1.3] - 2026-04-23

### Added
- **Iteration-Scoped AI Message Persistence:** Per-tool-iteration AI commentary now lands in `conversation_history` as `MessageSender.AI_PRIMARY` rows tagged with `EventType.EVENT_SOURCE_AI_PRIMARY`, preserving the agent's running narrative across restores.
- **`InvestigationService.persist_ai_message(...)`:** Centralized domain-layer helper for AI message persistence with strip-guards and metadata construction.
- **`BackgroundTaskManager.track_detached(...)`:** Synchronous tracking helper for fire-and-forget tasks dispatched from coroutines.
- **`AgentInputs` / `AgentStreamState` split:** Clean separation of request-scoped immutable inputs from mutable per-run stream sinks.
- **Tribunal Voting Breakdown:** Enhanced tribunal consensus events now include detailed voting breakdowns and dissent records.

### Changed
- **Memory Generation Off The Response Path:** `update_memory_from_conversation` is now dispatched as a tracked background task, preventing memory generation from blocking SSE completion.
- **SSE Event Publishing:** Refactored `deliver_via_sse` to use a captured routing tuple, eliminating potential for dropped fields across call sites.
- **Validation Messages:** Precision field-specific guards in `deliver_via_sse` for `investigation_id`, `web_session_id`, and `case_id`.

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
- **Documentation:** `g8ed` docs updated to describe parallel batch fan-out with bounded concurrency and shared `batch_id` correlation.
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
- **g8es (Data Store):** SQLite-backed persistence layer, KV store, and pub/sub broker running within the Operator framework.
- **g8ed (Dashboard):** Node.js central management console featuring FIDO2 WebAuthn (passkey) authentication and real-time mTLS gateway proxying.
- **Security:** "Tribunal Refinement Pipeline" utilizing stochastic swarm voting to validate AI-proposed terminal commands before human review.
- **Security:** Local execution vaulting to ensure raw stdout/stderr logs are securely encrypted and retained strictly on the target host.
- **DevOps:** Comprehensive `g8e` CLI wrapper for platform setup, testing, operator deployment, and CA certificate management.
