# g8e v4.3.0 — Operator Activation, Tribunal Hardening & Platform Reliability

v4.3.0 resolves a series of critical production bugs across the operator activation path, the Tribunal command-safety pipeline, and the pub/sub subsystem. It introduces real-time command lifecycle events across all operator services, a redesigned "Getting Started" onboarding flow, a one-command operator g8e script, and LLM CLI flags for AI integration testing — alongside a sweeping code quality pass that removes the `PendingCommand` DB-polling model in favour of a fully event-driven execution registry.

## Major Changes

### Operator Activation — Full End-to-End Fix

Resolved two bugs that together prevented operators from ever appearing as **Active** in the Operator Panel after authentication.

**`_completeAuthentication` was incomplete:**
- `claimOperatorSlot` was never called, leaving operator status as `AVAILABLE` instead of `ACTIVE` or `BOUND`
- `userService.updateUserOperator` was never called, so the user-level operator record was never updated
- The g8ee relay was passed a plain object instead of a proper `G8eHttpContext` with a `bound_operators` array — g8ee rejected it and never subscribed to the heartbeat channel

**`device_registration_service.js` had the same relay bug:**
- Both callers now construct a proper `G8eHttpContext` with `BoundOperatorContext` inside `bound_operators`, matching the pattern established by `operator_bind_service.js`

**Status preservation on re-authentication:**
- A BOUND operator that re-authenticates now stays BOUND instead of being downgraded to ACTIVE
- `claimSlot` accepts an optional `status` parameter (defaults to `ACTIVE`); `_completeAuthentication` checks the existing operator status before claiming

### Tribunal — Provider/Model Decoupling & Silent Fallback Prevention

Two critical bugs in the Tribunal command-safety pipeline were resolved.

**Provider/model decoupling:**
- `generate_command` previously always routed to `settings.provider`, even when the resolved model belonged to a different provider (e.g., sending an Ollama model name to the Gemini API)
- Added `_infer_provider_for_model()` that infers provider from naming conventions for cloud providers only (`gemini-` → Gemini, `gpt-` → OpenAI, `claude-` → Anthropic)
- Ollama is a first-class provider that must be configured explicitly via `provider=ollama` in settings — it is not inferred from model names
- Added `_resolve_provider_and_model()` that returns a coupled `(provider, model)` tuple
- Added `get_llm_provider_for_provider()` to `factory.py` for explicit provider selection

**Silent fallback on infrastructure failures:**
- When all Tribunal passes failed due to auth/network/config errors, `generate_command` previously returned a `FALLBACK` outcome and executed the original command — masking infrastructure problems
- Added `_is_system_error()` classifier matching 20+ error patterns (401, connection refused, DNS, SSL, etc.)
- Added `TribunalSystemError` exception — raised instead of `FALLBACK` when all passes fail with system errors
- Added `SYSTEM_ERROR` values to `CommandGenerationOutcome` and `TribunalFallbackReason` enums
- Pass errors are now collected and surfaced via a new `pass_errors` field on `TribunalFallbackPayload`

**`NameError` fix:** `types.Role.USER` in `_run_generation_pass` and `_run_verifier` was referencing an unimported `types` module. `Role` is now imported directly from `app.llm.llm_types`.

### Execution Registry — Event-Driven Result Delivery

Replaced the `PendingCommand` DB-polling loop with a fully event-driven approach.

- `ExecutionRegistryService` now maintains an in-memory result stash alongside its asyncio event gates
- New `complete(execution_id, result)` method stashes the result payload and signals the waiter atomically
- New `get_result(execution_id)` retrieves the stashed payload without removing it
- `execution_service.py` no longer polls `operator_data_service` in a loop; it calls `execution_registry.wait()` once and reads the result directly from the stash
- `PendingCommand` model and all related DB read/write paths have been removed — execution state is fully in-memory and event-driven

### Real-Time Command Lifecycle Events

All operator services now broadcast lifecycle events (`STARTED`, `COMPLETED`, `FAILED`) to the g8ed dashboard via a new `EventService` (`g8ed_event_service.py`).

- `EventService.publish_command_event()` wraps event construction with proper `SessionEvent` routing fields (`case_id`, `investigation_id`, `web_session_id`, `task_id`)
- `EventService.publish_investigation_event()` provides investigation-scoped event publishing
- `OperatorCommandService`, `OperatorFileService`, `OperatorFilesystemService`, `OperatorIntentService`, and `OperatorPortService` all emit lifecycle events at each execution phase
- `CommandExecutingBroadcastEvent` and `CommandResultBroadcastEvent` typed payload models carry execution metadata to the frontend
- `OperatorDataService` dependency removed from file, filesystem, and port services — these services no longer touch operator document persistence directly

### Approvals — Typed Request/Response Models

The `OperatorApprovalService` API is now fully typed.

- `handle_approval_response` takes a single `OperatorApprovalResponse` typed model instead of five loose positional arguments
- `request_execution_approval` now takes typed request models (`CommandApprovalRequest`, `FileEditApprovalRequest`, `IntentApprovalRequest`) replacing the 18-kwarg dispatcher
- `OperatorApprovalService` is a first-class service on `app.state.approval_service`; `OperatorCommandService` no longer owns or creates it
- Approval route uses `resolveBoundOperators()` (which internally validates sessions) instead of the manual `getBoundOperatorSessionIds()` + `validateSession()` pattern

### Operator Drop Script

Added a one-command `drop` script for deploying operators to any Linux system.

- `GET /g8e` serves a POSIX shell script generated by new `g8eDeploy()` in `cert-installers.js`
- The script fetches the CA certificate, installs it in the system trust store, downloads the operator binary from the blob store (auto-detects architecture), and launches the operator with the provided device token
- The Operator Download panel now shows `curl http://<host>/g8e | sh -s -- <token>` instead of the multi-step binary download command

### Pub/Sub — Wire Protocol Constants & Reliability

- Added `shared/constants/pubsub.json` — canonical wire-protocol constants shared across g8ee (Python), g8eo (Go), and g8ed (JS)
- `PubSubMessageType` split into `PubSubWireEventType` (`MESSAGE`, `PMESSAGE`, `SUBSCRIBED`) and `PubSubContentType`; backward-compat alias maintained
- `PubSubMessageType` enum in `channels.py` was missing `MESSAGE`, `PMESSAGE`, `SUBSCRIBED` — the `_ws_reader` referenced these non-existent members, causing `AttributeError` on the first message received and silently killing the reader task. Subscribe ACKs were never processed, causing all `subscribe()` calls to time out after 5 seconds
- `psubscribe()` race condition fixed: `_subscribed_patterns.add(pattern)` now occurs after `_ensure_ws()` and ACK event setup, preventing double-subscription on reconnect before the ACK handler exists
- `_ws_reader` now schedules `_reconnect_loop()` on disconnect if active subscriptions exist, using exponential backoff (1 s initial, 60 s max)
- Go contract test added: `TestSharedPubSubWireMatchesGoConstants` verifies Go constants against `shared/constants/pubsub.json`

### AI Generation Config & Memory

- New `AIGenerationConfigBuilder` — stateless factory for `GenerateContentConfig` objects. Provides `build_config()` (with thinking-level detection), `get_lite_generation_config()` (triage/memory/risk analysis), `get_lite_generation_config_with_schema()` (schema-enforced structured JSON), `get_lite_generation_config_for_json()` (flexible JSON for local models), and `get_title_generation_config()`
- `MemoryGenerationService` enhanced with full conversation-to-memory AI analysis pipeline: reads existing memory, builds structured LLM request from conversation history, parses `MemoryAnalysis` structured output, and persists updated `InvestigationMemory` via `MemoryDataService`
- Separation of concerns: `AIGenerationConfigBuilder` handles stateless config construction; `AIRequestBuilder` handles stateful request assembly (tools, attachments)

### `resolveBoundOperators` — N+1 Query Elimination

Rewrote `BoundSessionsService.resolveBoundOperators` to eliminate unnecessary work and N+1 queries.

- Old path: read KV SET for session IDs → `validateSession` each one (full expiry/integrity check) → reverse-binding check per operator → sequential `getOperator` calls
- New path: read `BoundSessionsDocument` directly via `cacheAside` (one read, cache-warm) → zip `operator_ids`/`operator_session_ids` arrays → `Promise.all` parallel fetch of operator docs
- Removed `operatorSessionService` dependency from `BoundSessionsService` entirely

### Post-Login Race Condition Fix

`onSuccessfulLogin` and `onSuccessfulRegistration` in `post_login_service.js` fired `initializeOperatorSlots` and `activateG8ENodeOperatorForUser` concurrently (fire-and-forget). `activateG8ENodeOperatorForUser` queries for `is_g8ep=true` slots, but `initializeOperatorSlots` creates those slots. When activation won the race, the query returned null and activation was silently skipped.

Extracted `_initializeSlotsAndActivateG8eNode(user, session, context)` that sequentially `await`s `initializeOperatorSlots` then `activateG8ENodeOperatorForUser`.

### LLM SSL Fix — Cloud Provider TLS Isolation

`G8E_SSL_CERT_FILE=/g8es/ca.crt` (set globally in the container environment) was poisoning all outbound HTTPS requests — cloud APIs use public CAs, not the internal platform CA.

- `AnthropicProvider`: Added `_is_internal_endpoint()`, uses `certifi.where()` for cloud endpoints
- `OpenAICompatibleProvider`: Removed broken `ssl_context.load_default_certs()` block (method does not exist on `ssl.SSLContext`), uses `certifi.where()` for external endpoints
- `GeminiProvider`: Temporarily overrides `G8E_SSL_CERT_FILE` with `certifi.where()` during `genai.Client()` construction, restores in `finally` block
- `factory.py`: Only passes `ca_cert_path` to providers that may use internal endpoints (Ollama, OpenAI-compatible, Anthropic); Gemini never receives it

### LLM CLI Flags for AI Integration Testing

Added `--llm-provider`, `--primary-model`, `--assistant-model`, `--llm-endpoint-url`, and `--llm-api-key` flags to `./g8e test`. These pass `TEST_LLM_*` environment variables into the g8ep container, allowing `ai_integration` tests to run against a real LLM without writing anything to g8es.

### Getting Started UI

Replaced the plain CLI reference text in the operator deployment panel with a structured three-step onboarding guide: **Bind an Operator** → **Start a Conversation** → **Deploy to More Systems**. Includes the built-in g8ep operator note so new users know where to start. Getting started content now also appears in the empty chat view when no conversation exists.

### SSL Certificate Trust Script Improvements

- `certInstallerScript` (Windows) now checks `%errorlevel%` after each step and exits with a descriptive error message if the download or trust operation fails
- `certInstallerScript` (macOS) now checks the curl exit code and exits with a descriptive error if the CA fetch fails
- Both scripts now clean up the temporary cert file on failure

## Bug Fixes

- **Operator auth** — `_completeAuthentication` now completes the full activation lifecycle (claim slot, update user record, relay to g8ee with proper `G8eHttpContext`)
- **Tribunal** — Fixed `NameError: name 'types' is not defined` in `_run_generation_pass` and `_run_verifier`; fixed provider mismatch when assistant model belongs to a different provider than the primary
- **Approvals (HTTP 500)** — `ApprovalRespondRequest` fields `operator_session_id`/`operator_id` were `required: true` but the frontend never sends them (server resolves them). Changed to `default: null`
- **Approvals (500 on respond)** — `operator_approval_routes.js` accessed `req.services.operatorService` which was never attached. Fixed by constructing `OperatorRelayService` directly in the route constructor
- **Tribunal (500)** — `tribunal` template was missing from the preload list in `operator-panel.js`, causing a `TypeError` on `templateLoader.cache.get('tribunal')` returning `undefined`
- **Approvals (double-parse)** — `relayApprovalResponseToG8ee` called `ApprovalRespondRequest.parse()` on data already validated upstream; removed
- **ValidationError on `system_info.interfaces`** — `new SystemInfo(system_info || {})` constructor was used instead of `SystemInfo.parse()`. Constructor applies field defaults for missing/null values but `parse()` properly validates and hydrates the wire object. Fixed in both `operator_auth_service.js` and `operator_slot_service.js`
- **`llm_command_gen_passes` TypeError** — `max(1, None)` crashed all AI tool calls; `llm_command_gen_passes` now defaults to `3`, `settings_service.py` adds None-guards for all four `command_gen` fields
- **KV silent failures** — `keys()` and `scan()` on `KVCacheClient` silently returned empty results on failure, causing `_invalidateQueryCache` to no-op and stale query cache entries to serve empty operator lists. Now throw `KVOperationError`
- **Build tag display** — `build.sh` was not displaying the correct git tag in the build output
- **g8es URL port** — `fetch-key-and-run.sh` was missing port 9000 on the `G8ES_URL`; now correctly uses port 9000
- **g8ep CA cert** — `fetch-key-and-run.sh` was passing `--ca-url` to the operator, which forced a network CA fetch — a chicken-and-egg TLS problem. Removed; operator discovers the CA cert from the local `/g8es/ca.crt` mount
- **Operator launch** — `fetch-key-and-run.sh` improved with binary download from blob store, architecture auto-detection, and retry logic for transient g8es unavailability

## Code Quality

- Removed `PendingCommand` model and all associated DB read/write operations from g8ee execution path
- Removed `OperatorDataService` dependency from `OperatorFileService`, `OperatorFilesystemService`, and `OperatorPortService` — these services no longer touch operator document persistence
- Removed `operatorSessionService` dependency from `BoundSessionsService`
- Removed `_reindexOperatorApiKeys` from `operator_slot_service.js` — dead code that re-issued keys for all pre-existing operators on every login
- Eliminated double `queryOperators` call in `initializeOperatorSlots`; also fixed subtle bug where second query included `TERMINATED` operators in the return value
- Post-login error logging escalated from `warn` to `error` with `userId` in log metadata
- Removed `ApprovalRespondRequest` context field conflation (user_id, web_session_id, etc. removed from model)
- Removed unused `crypto` import from approval routes
- Removed dead `UserRole` import from `post_login_service.js`
- Extracted `AIGenerationConfigBuilder` as a stateless config factory, separating config construction from request assembly
- Added `shared/constants/document_ids.json` — canonical document ID constants shared across g8ee and g8ed
- Escalated `_invalidateQueryCache` error log from `warn` to `error` with "stale results may be served" message

## Testing

- G8EE: 41 tests in `test_command_generator.py` (was 17) — `TestInferProviderForModel`, `TestResolveProviderAndModel`, `TestIsSystemError`, `TestTribunalSystemError`, `TestPassErrorsCollection`, `TestGenerateCommandSystemError`
- G8EE: 22 tests in `test_pubsub_client.py` (was 8) — wire protocol constants, subscribe/psubscribe ACK, reconnect loop
- G8EE: 20 tests in `test_provider_ssl.py` — cloud vs. internal endpoint cert selection per provider
- G8EE: `test_operator_port_service.py` — expanded with full lifecycle event assertions and error path coverage
- G8EE: `test_agent_execute_tool_call.py` — expanded with command lifecycle event broadcasting assertions
- G8EE: `test_operator_command_service.py` — expanded with typed approval request and execution result assertions
- G8ED: Regression tests for operator activation, BoundOperatorContext relay structure, approval route fixes, g8e script generation, cert installer error handling
- G8ED: `shared-pubsub-constants.test.js` — 9 contract tests verifying JS constants against `shared/constants/pubsub.json`
- G8ED: `terminal-output-rendering.unit.test.js` — terminal output rendering tests
- G8ED: `cache_aside_service.unit.test.js` — `_invalidateQueryCache` tests for `KVOperationError` non-propagation
- G8ED: `g8es_kv_cache_client.unit.test.js` — updated for `KVOperationError` throw behavior
- G8EO: `TestSharedPubSubWireMatchesGoConstants` — Go contract tests against `shared/constants/pubsub.json`

## Component Summary

| Component | Changes |
|-----------|---------|
| **g8ee** | Execution registry redesign, real-time command lifecycle events, typed approval API, Tribunal hardening, LLM SSL isolation, pub/sub reliability, `AIGenerationConfigBuilder`, memory generation |
| **g8ed** | Operator activation fix, g8e script, getting started UI, approval route fixes, `resolveBoundOperators` rewrite, race condition fix, `KVOperationError`, terminal output rendering |
| **g8eo** | Pub/sub wire protocol contract tests |
| **g8ep** | g8es URL port fix, CA cert bootstrap fix, operator binary fetch improvements, sudoers hardening |

## Quick Start

```bash
git clone https://github.com/g8e-ai/g8e-ai/g8e.git && cd g8e
./g8e platform setup

# Then open https://localhost — the setup wizard guides you through configuration
```

## Security & Privacy

v4.3.0 continues the local-first, human-in-the-loop security model. This release hardens the operator activation path and fixes a TLS isolation gap:

- **Operator activation** — Full lifecycle is now completed correctly on auth: slot claimed, user record updated, g8ee notified with proper identity context. Operators can no longer be in a permanently inactive state after authentication.
- **TLS isolation** — Cloud LLM API calls (Gemini, Anthropic, OpenAI) now use the Mozilla CA bundle (`certifi`) instead of the internal platform CA, preventing misconfigured or expired internal CAs from silently breaking AI functionality.
- **Tribunal integrity** — Infrastructure failures (auth errors, DNS failures, SSL errors) in the Tribunal now raise `TribunalSystemError` instead of silently falling back to the original command. This prevents unvalidated commands from executing when the safety pipeline is misconfigured.
- **Data integrity** — KV cache client operations now throw `KVOperationError` on failure instead of silently returning empty results, preventing stale query cache entries from serving incorrect data to the frontend.

---

**g8e** — AI-powered, human-driven infrastructure operations. Fully self-hosted. Air-gap capable. Security and privacy by design.

[Website](https://lateraluslabs.com) | [Docs](../index.md) | [License](../../LICENSE)
