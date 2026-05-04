# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

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
- **Node Package Updates:** Updated dependencies in `g8ed` for security and performance.

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
- **`g8el` (LFAA) Re-introduction:** Restored the `g8el` component to provide Low-Fidelity Agentic Assistance, tuned specifically for eval performance and lightweight orchestration.
- **SSH Inventory Streaming:** New capability to stream and import operator inventory directly from local SSH configuration files.
- **Enhanced Test Fixtures:** Added `gold-set-schema.json` and `ledger-hash-fixtures.json` to improve consistency across platform evaluation suites.
- **Reputation CLI:** New administrative scripts `manage-reputation.py` and `seed-reputation-state.py` for platform governance.

### Changed
- **Tribunal 2.0 Governance:** Significant refactor of the Tribunal pipeline, implementing multi-phase consensus, detailed dissent recording, and improved safety guideline delivery.
- **Operator Authority Model:** Consolidated operator document handling and configuration delivery, positioning `g8ee` as the authoritative source for operator state.
- **Settings UX Overhaul:** Redesigned the Dashboard Settings page to match the Setup page layout, including improved command validation and status rendering.
- **Device Link Refactoring:** Streamlined device link management and added auto-approval logic for benign, non-mutating commands.
- **System Info & Heartbeat Synchronization:** Overhauled `SystemInfo` and `Heartbeat` wire models for better cross-component consistency and reduced payload size.
- **Documentation Refresh:** Comprehensive updates to all architectural and component documentation, including new guides for `g8el` and updated developer instructions.

### Fixed
- **Authentication Loops:** Resolved edge cases in operator authentication and fixed internal routing issues during high-concurrency streams.
- **Async Tooling:** Fixed `asyncio` race conditions in the `ToolService` and improved background task tracking.
- **Test Suite Stability:** Fixed unit and integration test failures in `g8ed`, `g8ee`, and the evals suite.
- **API Key Security:** Improved masking and display security for API keys within the CLI environment.
- **Iconography:** Fixed missing or incorrect icons in the Dashboard, including the Auditor and Operator status indicators.

## [0.1.4] - 2026-04-24

### Added
- **Release Synchronization:** Version bump to 0.1.4 to synchronize platform components after tagging conflict.

## [0.1.3] - 2026-04-24

### Added
- **Global Platform Refactor:** Massive synchronization of constants and models across `g8ed`, `g8ee`, and `shared` layers to ensure wire-contract stability.
- **Iteration-Scoped AI Message Persistence:** Per-tool-iteration AI commentary now lands in `conversation_history` as `MessageSender.AI_PRIMARY` rows tagged with `EventType.EVENT_SOURCE_AI_PRIMARY`, preserving the agent's running narrative across restores. The SSE delivery layer fires an `on_iteration_text` callback at each `TOOL_RESULT` boundary, which `ChatPipelineService` binds to a persistence helper. Final post-stream persistence still runs and skips whitespace-only text.
- **`InvestigationService.persist_ai_message(...)`:** New domain-layer helper that centralizes the strip-guard and `AIResponseMetadata` construction previously duplicated between the per-iteration and final AI persist paths. Accepts optional `grounding_metadata` and `token_usage` for the final-row case.
- **`BackgroundTaskManager.track_detached(...)`:** Synchronous tracking helper for fire-and-forget tasks dispatched from inside coroutines that cannot `await`. Auto-removes completed tasks via done-callback; surfaces uncaught exceptions at `WARNING` with `exc_info=True`.
- **`AgentInputs` / `AgentStreamState` split:** Request-scoped immutable inputs (`AgentInputs`) are now separate from the mutable per-run stream sinks (`AgentStreamState`). Both use `extra='forbid'`. Replaces the previous combined `AgentStreamContext` / `make_streaming_context`.
- **Evaluation Suite:** Introduced comprehensive AI evaluation tools including `accuracy`, `benchmark`, and `privacy` scorers to validate agent behavior against gold sets.
- **Tribunal Voting Breakdown:** Enhanced tribunal consensus events now include detailed voting breakdowns and dissent records.

### Changed
- **Frontend Modernization:** Overhauled `g8ed` components including `operator-panel`, `anchored-terminal`, and SSE handlers for better UX and reliability.
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
