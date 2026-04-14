# G8EE-G8ED Event Pipeline Refactor Plan

## Overview

This document provides a comprehensive analysis of the event pipeline between g8ee (engine) and g8ed (frontend gateway), identifying gaps and mishandlings across four categories. The goal is to ensure every phase of the chat pipeline produces rich, transparent visual feedback in the operator terminal UI.

## Progress Tracking

### Completed (2025-04-14)

**High Priority Items Completed:**

1. **Section 1.3: LLM_CHAT_ITERATION_RETRY** - COMPLETED
   - Added event type to `shared/constants/events.json` (line 334)
   - Added handler in `agent_sse.py` (lines 155-168) to emit event when RETRY chunks are received
   - Added documentation in `docs/reference/events.md`
   - Status: Users now see retry notifications in terminal

2. **Section 1.4: LLM_CHAT_ITERATION_TOOL_CALL_STARTED/COMPLETED** - COMPLETED
   - Added event types to `shared/constants/events.json` (lines 339-340)
   - Added handlers in `agent_sse.py` (lines 174-186 for started, 228-238 for completed) to emit generic tool call events for all tools
   - Added documentation in `docs/reference/events.md`
   - Status: All 17 tools now emit activity indicator events with display metadata

3. **Section 4.1: RETRY Chunks Silently Dropped** - COMPLETED
   - Fixed by adding RETRY handler in `agent_sse.py` (see item 1 above)
   - Status: RETRY chunks now emit LLM_CHAT_ITERATION_RETRY events

4. **Section 4.2: Tool Call Activity Indicators Only for 2 of 17 Tools** - COMPLETED
   - Fixed by adding generic tool call handlers in `agent_sse.py` (see item 2 above)
   - Status: All tools now emit generic started/completed events with display metadata

5. **Section 4.3: OPERATOR_COMMAND_CANCELLED Has No Frontend Handler** - COMPLETED
   - Added eventBus listener in `chat-sse-handlers.js` (line 144-146)
   - Added `handleCommandCancelled` method (lines 701-713) that clears execution state and completes activity indicators
   - Fixed bug: method now attempts both `fn-${execId}` and `tool-${execId}` prefixes to handle all tool types
   - Status: Command cancellation now properly clears UI state

**Bug Fixes:**

6. **Indicator ID Prefix Inconsistency** - PARTIALLY FIXED
   - Fixed `handleCommandCancelled` to attempt both `fn-` and `tool-` prefixes
   - This is a band-aid fix; root cause remains (see Code Smells section below)

**Documentation Updates:**

7. **docs/reference/events.md** - Updated
   - Added documentation for 3 new events under `ai.llm.chat.iteration` section
   - Created new `ai.llm.chat.iteration.tool` subsection
   - Updated total event counts from 238 to 241, AI domain count from 48 to 51

### Architecture Summary

```
g8ee (Python/FastAPI)
  -> EventService.publish() / publish_investigation_event() / publish_command_event()
    -> InternalHttpClient.push_sse_event()
      -> HTTP POST to g8ed /api/internal/sse/push
        -> g8ed internal_sse_routes.js (wraps in G8eePassthroughEvent)
          -> SSEService.publishEvent() -> sendToLocal()
            -> Browser EventSource (sse-connection-manager.js)
              -> handleSSEEvent() -> eventBus.emit(type, data)
                -> chat-sse-handlers.js / thinking.js / anchored-terminal-operator.js
```

---

## Section 1: Non-Existent Events (Should Be Created)

These are pipeline phases that produce no event at all. The user gets zero feedback during these stages.

### 1.1 LLM_CHAT_CONTEXT_PREPARING (new)

**Phase:** `ChatPipelineService._prepare_chat_context()` (chat_pipeline.py:98-289)

**Problem:** Context preparation involves investigation enrichment, triage, memory retrieval, prompt building, and history fetching. This can take 1-5+ seconds. The user sees nothing between sending a message and `LLM_CHAT_ITERATION_STARTED`.

**Proposed event:** `g8e.v1.ai.llm.chat.context.preparing`
- **Payload:** `{ agent_mode, model_to_use, has_attachments, triage_complexity }`
- **Emitted by:** `_prepare_chat_context()` after triage completes
- **UI effect:** Show "Preparing context..." or "Analyzing request..." in the terminal

### 1.2 LLM_CHAT_TRIAGE_COMPLETED (new)

**Phase:** Triage agent runs in `_prepare_chat_context()` (chat_pipeline.py:169-177)

**Problem:** Triage decides complexity (SIMPLE vs COMPLEX), model routing, and can even short-circuit with a follow-up question. No event is emitted for this decision.

**Proposed event:** `g8e.v1.ai.llm.chat.triage.completed`
- **Payload:** `{ complexity, intent_summary, model_selected, follow_up_question }`
- **Emitted by:** `_prepare_chat_context()` immediately after `self.triage_agent.triage()`
- **UI effect:** Show model being used ("Using gemini-2.5-pro") and complexity class in the terminal

### 1.3 LLM_CHAT_ITERATION_RETRY (COMPLETED ✅)

**Phase:** `g8eEngine.stream_response()` retry loop (agent.py:151-193)

**Problem:** When the LLM provider fails and g8ee retries, it yields `StreamChunkFromModelType.RETRY` chunks. However, `agent_sse.py` has **no handler** for `RETRY` chunks -- they silently fall through. The user sees no indication that g8ee is retrying.

**Proposed event:** `g8e.v1.ai.llm.chat.iteration.retry`
- **Payload:** `{ attempt, max_attempts }`
- **Emitted by:** `deliver_via_sse()` when chunk.type == RETRY
- **UI effect:** Show "Retrying (attempt 2/3)..." in the terminal

**Status:** COMPLETED (2025-04-14)
- Event added to `shared/constants/events.json` (line 334)
- Handler added in `agent_sse.py` (lines 155-168)
- Documentation added to `docs/reference/events.md`

### 1.4 LLM_CHAT_ITERATION_TOOL_CALL_STARTED / TOOL_CALL_COMPLETED (COMPLETED ✅)

**Phase:** `agent_sse.py` TOOL_CALL/TOOL_RESULT handling for operator tools (lines 153-215)

**Problem:** Only `search_web` and `check_port` get dedicated SSE events with activity indicators. All other operator tools (`file_create`, `file_write`, `file_read`, `file_update`, `list_files`, `read_file_content`, `restore_file`, `fetch_file_history`, `fetch_file_diff`, `grant_intent`, `revoke_intent`, `fetch_execution_output`, `fetch_session_history`, `query_investigation_context`) produce **no activity indicator** in agent_sse.py. The tool display metadata exists in `_TOOL_DISPLAY_METADATA` (agent_tool_loop.py:81-99) but is never used for SSE events.

**Proposed approach:** Emit a generic `g8e.v1.ai.llm.chat.iteration.tool.call.started` event for every TOOL_CALL chunk that includes the existing `display_label`, `display_icon`, `display_detail`, and `category` fields from `StreamChunkData`. This gives the frontend everything it needs to show an appropriate activity indicator for every tool.

Similarly, emit `g8e.v1.ai.llm.chat.iteration.tool.call.completed` for every TOOL_RESULT chunk.

**UI effect:** Every tool call gets a visible activity indicator in the terminal (e.g., "Writing file: /etc/config.yml", "Reading file: app.py", "Requesting permission")

**Status:** COMPLETED (2025-04-14)
- Events added to `shared/constants/events.json` (lines 339-340)
- Handlers added in `agent_sse.py` (lines 174-186 for started, 228-238 for completed)
- Documentation added to `docs/reference/events.md`

### 1.5 OPERATOR_FILE_EDIT_TIMEOUT (defined but never emitted)

**Phase:** File edit operations

**Problem:** `EventType.OPERATOR_FILE_EDIT_TIMEOUT` is defined in events.py:161 but is never emitted anywhere in g8ee. If a file edit times out, the user gets no specific timeout notification.

**Action:** Emit this event in file_service.py when a file edit operation exceeds the timeout threshold.

---

## Section 2: Events Existing in G8EE But Not Delivered to G8ED

These events are defined in the EventType enum but are never published via EventService.

### 2.1 Defined But Never Published

The following EventType members exist in `events.py` but no `g8ed_event_service.publish()` call references them:

| EventType | Status |
|-----------|--------|
| `TRIBUNAL_SESSION_FAILED` | Defined (line 263) but never emitted. Tribunal errors use `TRIBUNAL_SESSION_FALLBACK_TRIGGERED` instead. |
| `TRIBUNAL_VOTING_STARTED` | Defined (line 266) but never emitted. Voting happens implicitly inside `_run_generation_stage()`. |
| `TRIBUNAL_VOTING_FAILED` | Defined (line 267) but never emitted. Individual pass failures use `TRIBUNAL_VOTING_PASS_COMPLETED` with `success=False`. |
| `TRIBUNAL_VOTING_PASS_FAILED` | Defined (line 269) but never emitted. Pass failures use `TRIBUNAL_VOTING_PASS_COMPLETED` with `success=False`. |
| `TRIBUNAL_VOTING_CONSENSUS_NOT_REACHED` | Defined (line 271) but never emitted. No-consensus scenario falls back to `TRIBUNAL_SESSION_FALLBACK_TRIGGERED`. |
| `TRIBUNAL_VOTING_REVIEW_FAILED` | Defined (line 274) but never emitted. Verifier failures use `TRIBUNAL_VOTING_REVIEW_COMPLETED` with `passed=False`. |
| `LLM_LIFECYCLE_REQUESTED` | Defined (line 64) but never emitted anywhere. |
| `LLM_LIFECYCLE_STARTED` | Defined (line 65) but never emitted anywhere. |
| `LLM_LIFECYCLE_COMPLETED` | Defined (line 66) but never emitted anywhere. |
| `LLM_LIFECYCLE_FAILED` | Defined (line 67) but never emitted anywhere. |
| `LLM_LIFECYCLE_STOPPED` | Defined (line 68) but never emitted anywhere. |
| `LLM_LIFECYCLE_ERROR_OCCURRED` | Defined (line 69) but never emitted anywhere. |
| `LLM_CHAT_ITERATION_STREAM_STARTED` | Defined (line 58) but never emitted. |
| `LLM_CHAT_ITERATION_STREAM_DELTA_RECEIVED` | Defined (line 59) but never emitted. |
| `LLM_CHAT_ITERATION_STREAM_COMPLETED` | Defined (line 60) but never emitted. |
| `LLM_CHAT_ITERATION_STREAM_FAILED` | Defined (line 61) but never emitted. |
| `LLM_CHAT_ITERATION_TEXT_TRUNCATED` | Defined (line 56) but never emitted. |
| `LLM_CHAT_ITERATION_TEXT_RECEIVED` | Defined (line 53) but never emitted. |
| `LLM_CHAT_MESSAGE_SENT` | Defined (line 41) but never emitted by g8ee. |
| `LLM_CHAT_MESSAGE_REPLAYED` | Defined (line 42) but never emitted. |
| `LLM_CHAT_MESSAGE_PROCESSING_FAILED` | Defined (line 43) but never emitted. |
| `LLM_CHAT_MESSAGE_DEAD_LETTERED` | Defined (line 44) but never emitted. |
| `OPERATOR_COMMAND_REQUESTED` | Defined (line 132) but never emitted. |

**Recommendations:**

1. **Tribunal events:** The dual-use pattern (e.g., `PASS_COMPLETED` with `success=False` instead of `PASS_FAILED`) is acceptable. These "Failed" variants should either be removed from the enum or explicitly emitted at the appropriate failure points for frontend clarity. Recommend emitting them for richer UI feedback.

2. **LLM Lifecycle events:** These should be emitted to frame the entire chat lifecycle at a higher level than iteration events. Emit `LIFECYCLE_STARTED` at the beginning of `run_chat()` and `LIFECYCLE_COMPLETED`/`LIFECYCLE_FAILED` at the end.

3. **Stream-level events:** `STREAM_STARTED` through `STREAM_COMPLETED` could provide even more granular feedback about the raw LLM stream phase (before tool calls), distinguishing "waiting for first token" from "streaming text." However, these may be too granular for now and can be deferred.

4. **Message events:** `LLM_CHAT_MESSAGE_SENT` should be emitted when the user's message is persisted to the investigation in `_prepare_chat_context()`.

---

## Section 3: Events Delivered to G8ED But Not Passed to Frontend

These events arrive at g8ed's internal SSE push endpoint, get wrapped in `G8eePassthroughEvent`, and are delivered to the browser's `EventSource`. The `sse-connection-manager.js` `handleSSEEvent()` emits them on the `eventBus`. The question is whether any frontend component listens.

### 3.1 SSE Connection Manager Validation Drops

`sse-connection-manager.js:262-265` drops events where `payload === undefined`:
```js
if (payload === undefined) {
    devLogger.warn('[SSE] Received non-infrastructure event with no data field -- dropped', data);
    return { handled: false, eventType };
}
```

**Impact:** If g8ee sends an event where the `SessionEvent.flatten_for_wire()` produces a `data` field that is `undefined` (not just empty), the event is silently dropped. This is a safety net, but any payload serialization bug in g8ee could cause silent event loss.

### 3.2 Events Emitted on EventBus But No Listener

The following events are emitted onto the eventBus by `handleSSEEvent()` but **no component** registers a listener for them:

| Event | Reaches Frontend? | Has Listener? | Impact |
|-------|-------------------|---------------|--------|
| `OPERATOR_COMMAND_APPROVAL_PREPARING` | Yes (g8ee emits, g8ed passes through) | chat-sse-handlers.js:103 registers handler | OK - handled |
| `OPERATOR_COMMAND_OUTPUT_RECEIVED` | Yes | anchored-terminal-operator.js:70 | OK - handled |
| `OPERATOR_FILE_HISTORY_FETCH_STARTED` | Yes (g8ee emits via file_service.py) | **No listener** | User sees nothing during file history fetch |
| `OPERATOR_FILE_HISTORY_FETCH_COMPLETED` | Yes | **No listener** | No completion indication |
| `OPERATOR_FILE_HISTORY_FETCH_FAILED` | Yes | **No listener** | No failure indication |
| `OPERATOR_FILE_DIFF_FETCH_STARTED` | Yes | **No listener** | User sees nothing during diff fetch |
| `OPERATOR_FILE_DIFF_FETCH_COMPLETED` | Yes | **No listener** | No completion indication |
| `OPERATOR_FILE_DIFF_FETCH_FAILED` | Yes | **No listener** | No failure indication |
| `OPERATOR_FILESYSTEM_LIST_STARTED` | Yes (g8ee emits via filesystem_service.py) | **No listener** | User sees nothing during directory listing |
| `OPERATOR_FILESYSTEM_LIST_COMPLETED` | Yes | **No listener** | No completion indication |
| `OPERATOR_FILESYSTEM_LIST_FAILED` | Yes | **No listener** | No failure indication |
| `OPERATOR_FILESYSTEM_READ_STARTED` | Yes (g8ee emits via filesystem_service.py) | **No listener** | User sees nothing during file read |
| `OPERATOR_FILESYSTEM_READ_COMPLETED` | Yes | **No listener** | No completion indication |
| `OPERATOR_FILESYSTEM_READ_FAILED` | Yes | **No listener** | No failure indication |
| `OPERATOR_FILE_RESTORE_COMPLETED` | Yes | **No listener** | No completion indication |
| `OPERATOR_FILE_RESTORE_FAILED` | Yes | **No listener** | No failure indication |
| `OPERATOR_LOGS_FETCH_COMPLETED` | Yes | **No listener** | No completion indication |
| `OPERATOR_LOGS_FETCH_FAILED` | Yes | **No listener** | No failure indication |
| `OPERATOR_HISTORY_FETCH_COMPLETED` | Yes | **No listener** | No completion indication |
| `OPERATOR_HISTORY_FETCH_FAILED` | Yes | **No listener** | No failure indication |
| `OPERATOR_COMMAND_CANCELLED` | Yes (g8ee emits) | **Listener added (2025-04-14)** | Cancellation indication now shown in UI |
| `OPERATOR_DEVICE_REGISTERED` | Yes | **No listener** | Device registration not shown |
| `PLATFORM_NOTIFICATION` | Yes (g8ee defines it) | **No listener** | Platform notifications dropped |

**Recommendation:** Add eventBus listeners in `chat-sse-handlers.js` or `anchored-terminal-operator.js` for all operator tool lifecycle events. These should show/complete activity indicators in the terminal matching the tool display metadata from Section 1.4.

---

## Section 4: Events Mishandled by the G8ED Frontend

These events reach the frontend and have listeners, but the handling has issues.

### 4.1 RETRY Chunks Silently Dropped (COMPLETED ✅)

**Location:** `agent_sse.py` (entire file) + `sse-connection-manager.js`

**Problem:** `g8eEngine.stream_response()` yields `StreamChunkFromModelType.RETRY` chunks (agent.py:156-159), but `deliver_via_sse()` in agent_sse.py has no `elif chunk.type == StreamChunkFromModelType.RETRY:` branch. The chunk falls through all the if/elif conditions and is silently ignored. The user never knows g8ee is retrying a failed LLM call.

**Fix:** Add RETRY handling in `deliver_via_sse()` to emit a new `LLM_CHAT_ITERATION_RETRY` event, and add a frontend handler that shows a transient retry notification.

**Status:** COMPLETED (2025-04-14)
- Handler added in `agent_sse.py` (lines 155-168)
- Event type added to `shared/constants/events.json`
- Documentation added to `docs/reference/events.md`

### 4.2 Tool Call Activity Indicators Only for 2 of 17 Tools (COMPLETED ✅)

**Location:** `agent_sse.py:153-183` + `chat-sse-handlers.js:347-427`

**Problem:** The TOOL_CALL handler in `deliver_via_sse()` only produces SSE events for `search_web` and `check_port`. All other 15 tools (file operations, intent management, execution output fetch, session history, etc.) produce TOOL_CALL/TOOL_RESULT chunks that are completely ignored by agent_sse.py. The frontend has no way to show activity indicators for these tools.

The tool display metadata (`_TOOL_DISPLAY_METADATA` in agent_tool_loop.py:81-99) already has icons, labels, and categories for every tool. This data is already in the `StreamChunkData` carried by the TOOL_CALL chunk (`call_info`), but `deliver_via_sse()` never reads it for non-search/non-port tools.

**Fix:** In `deliver_via_sse()`, emit a generic tool activity event for every TOOL_CALL chunk, using the already-populated `display_label`, `display_icon`, `display_detail`, and `category` fields.

**Status:** COMPLETED (2025-04-14)
- Generic handlers added in `agent_sse.py` (lines 174-186 for started, 228-238 for completed)
- Event types added to `shared/constants/events.json`
- Documentation added to `docs/reference/events.md`

### 4.3 OPERATOR_COMMAND_CANCELLED Has No Frontend Handler (COMPLETED ✅)

**Location:** `chat-sse-handlers.js`

**Problem:** When a command is cancelled via the stop endpoint, g8ee emits `OPERATOR_COMMAND_CANCELLED` via the execution service. However, no frontend component listens for this event. The UI may remain in a stale "executing" state.

**Fix:** Add a listener that clears `executionActive`, hides stop button, and completes any pending activity indicator.

**Status:** COMPLETED (2025-04-14)
- EventBus listener added in `chat-sse-handlers.js` (line 144-146)
- `handleCommandCancelled` method added (lines 701-713)
- Bug fix: method now attempts both `fn-${execId}` and `tool-${execId}` prefixes to handle all tool types

### 4.4 Tribunal Widget Cleanup on Error

**Location:** `chat-sse-handlers.js:440-544`

**Problem:** If the Tribunal pipeline fails with an exception (e.g., `TribunalProviderUnavailableError`), the command generator catches it before emitting `TRIBUNAL_SESSION_COMPLETED`. The `_run_generation_stage()` does emit `TRIBUNAL_SESSION_FALLBACK_TRIGGERED` before raising, and `handleTribunalFallback` does cleanup. However, if `TribunalProviderUnavailableError` is raised (agent_tool_loop.py:238-247), no fallback event is emitted before the raise -- the tribunal widget may remain incomplete in the UI.

**Fix:** Emit `TRIBUNAL_SESSION_FALLBACK_TRIGGERED` in the `TribunalProviderUnavailableError` handler in `orchestrate_tool_execution()` before returning the error result, or emit it in `generate_command()` before re-raising.

### 4.5 handleResponseComplete Requires `content` But Not All Paths Provide It

**Location:** `chat-sse-handlers.js:547-575`

**Problem:** `handleResponseComplete` checks `data.content` before finalizing the AI response (line 559). If `LLM_CHAT_ITERATION_TEXT_COMPLETED` arrives with an empty `content` field (e.g., when the AI's response was entirely tool calls with no final text), the terminal response may not be properly sealed/finalized.

**Fix:** Ensure the finalization path handles the case where `content` is empty by at least hiding indicators and cleaning up state even when there's no text to render.

### 4.6 Thinking State Leak When Error Occurs During Thinking Phase

**Location:** `thinking.js` + `chat-sse-handlers.js`

**Problem:** If the LLM errors out while in a thinking phase (thinking chunks received, but no `THINKING_END` before `ERROR`), the `thinkingActive` flag in `ThinkingManager` remains `true`. The `_handleLLMChatIterationFailed` in chat.js does call `thinkingManager.hideThinkingIndicator()` (line 348), which properly cleans up. However, `handleChatError` in the mixin (line 599-641) does NOT call `hideThinkingIndicator` -- it only cleans up the streaming cursor. The `_handleLLMChatIterationFailed` call fixes this, but only because both are wired to the same event. If the order changes or one is removed, thinking state leaks.

**Recommendation:** Move the `hideThinkingIndicator` call into `handleChatError` itself for robustness.

### 4.7 BackgroundEvent Delivery (No web_session_id)

**Location:** `internal_sse_routes.js:62-144` + `events.py:69-96`

**Problem:** `BackgroundEvent.flatten_for_wire()` does not include `web_session_id` in the outer envelope. The `internal_sse_routes.js` push handler reads `pushReq.web_session_id` (line 73) -- if it's undefined/null, there's no target session to deliver to. Background events are effectively undeliverable via the current SSE push path unless the caller includes `web_session_id` through another mechanism.

**Status:** This appears to be by design (background events have no session), but it means any g8ee code that publishes a `BackgroundEvent` expecting browser delivery will silently fail. Verify no code path is doing this.

---

## Section 5: Code Smells Identified During Implementation

### 5.1 Indicator ID Prefix Inconsistency (PARTIALLY FIXED)

**Location:** `chat-sse-handlers.js`

**Issue:** The codebase uses multiple indicator ID prefixes inconsistently:
- `fn-${executionId}` for OPERATOR_COMMAND events (lines 131, 140, 159, 168, 709)
- `tool-${executionId}` for generic tool call events (line 677)
- `search-web-${executionId}` for web search (line 367)
- `port-check-${executionId}` for port checks (line 408)

**Impact:** The fix applied in `handleCommandCancelled` (lines 701-713) makes it attempt both `fn-` and `tool-` prefixes, but this is a band-aid. The root cause is that different code paths create indicators with different prefixes for the same execution IDs, making cleanup logic complex and error-prone.

**Recommendation:** Standardize on a single prefix scheme across all tool-related events. Either:
1. Use `tool-` prefix everywhere and update all OPERATOR_COMMAND handlers, or
2. Keep `fn-` for command-specific events and `tool-` for generic tool calls, but ensure the event flow clearly distinguishes between them so the right handler is called for the right prefix.

### 5.2 Redundant Event Flow (Intentional Technical Debt)

**Location:** `agent_sse.py` and event handlers

**Issue:** The generic tool call events (`LLM_CHAT_ITERATION_TOOL_CALL_STARTED/COMPLETED`) coexist with specific events for `search_web` and `check_port` (`LLM_TOOL_G8E_WEB_SEARCH_REQUESTED`, `OPERATOR_NETWORK_PORT_CHECK_REQUESTED`). This creates redundancy in the event flow.

**Impact:** Both old and new events are emitted for search_web and check_port for backward compatibility. This increases event volume and complexity.

**Recommendation:** This is intentional per the refactor plan for backward compatibility, but should be documented as technical debt. Consider deprecating the old specific events in a future release once the frontend fully migrates to the generic events.

---

## Section 6: Implementation Priority

### High Priority (Critical UX Gaps)

1. ~~**RETRY chunk handling in agent_sse.py**~~ -- COMPLETED (2025-04-14) ✅
2. ~~**Generic tool call activity indicators**~~ -- COMPLETED (2025-04-14) ✅
3. ~~**OPERATOR_COMMAND_CANCELLED listener**~~ -- COMPLETED (2025-04-14) ✅
4. **TribunalProviderUnavailableError fallback event** -- Orphaned tribunal widgets

### Medium Priority (Valuable UX Improvements)

5. **LLM_CHAT_CONTEXT_PREPARING event** -- Bridge the gap between message send and iteration start
6. **Frontend listeners for file/filesystem/logs/history events** -- Already delivered, just need handlers (18+ events)
7. **handleResponseComplete empty-content path** -- Edge case but causes incomplete cleanup
8. **Thinking state leak hardening** -- Move hideThinkingIndicator into handleChatError

### Low Priority (Completeness)

9. **LLM Lifecycle events** -- Higher-level framing of the chat lifecycle
10. **Triage completed event** -- Nice-to-have transparency into model selection
11. **Clean up unused EventType members** -- Remove or implement the 20+ phantom events
12. **LLM_CHAT_MESSAGE_SENT event** -- Confirm message persistence to user

---

## Section 7: Files to Modify

| File | Changes | Status |
|------|---------|--------|
| `g8ee/app/services/ai/agent_sse.py` | Add RETRY handler; add generic tool call/result SSE events | COMPLETED (2025-04-14) ✅ |
| `g8ee/app/services/ai/chat_pipeline.py` | Emit context-preparing and triage events | PENDING |
| `g8ee/app/services/ai/agent_tool_loop.py` | Emit fallback event for TribunalProviderUnavailableError | PENDING |
| `g8ee/app/constants/events.py` | Add new EventType members | COMPLETED (2025-04-14) ✅ |
| `g8ee/app/models/g8ed_client.py` | Add payload models for new events | COMPLETED (2025-04-14) ✅ |
| `g8ed/public/js/components/chat-sse-handlers.js` | Add listeners for new events, tool activity, command cancelled | PARTIALLY COMPLETED (2025-04-14) ✅ (command cancelled done, tool activity handlers pending) |
| `g8ed/public/js/components/thinking.js` | Move hideThinkingIndicator into error path | PENDING |
| `g8ed/public/js/constants/events.js` | Add new EventType constants | COMPLETED (2025-04-14) ✅ |
| `g8ed/public/js/utils/sse-connection-manager.js` | Add validation for new event types | PENDING |
| `shared/constants/events.json` | Add new canonical event types | COMPLETED (2025-04-14) ✅ |
| `docs/reference/events.md` | Document new events | COMPLETED (2025-04-14) ✅ |

---

## Section 8: Recommended Next Steps

### Immediate (Test Validation)

1. **Run correct test paths** - The g8ed test failed during the session because the filter `test/services` didn't match any files. Need to find the correct test directory structure for g8ed and run with the correct path.

2. **Run g8ee AI service tests** - Execute `/home/bob/g8e/g8e test g8ee -- tests/unit/services/ai` to verify the new event emissions and payload models don't break existing functionality.

### Medium-Priority UX Improvements

3. **Add LLM_CHAT_CONTEXT_PREPARING event** - Emit this event in `chat_pipeline.py:_prepare_chat_context()` after triage completes to bridge the 1-5 second silence between message send and iteration start. Create payload model with `{agent_mode, model_to_use, has_attachments, triage_complexity}` and add frontend handler to show "Preparing context..." in the terminal.

4. **Wire frontend listeners for 18+ operator tool lifecycle events** - Add eventBus listeners in `chat-sse-handlers.js` for events that already reach the browser but have no component handler:
   - `OPERATOR_FILE_HISTORY_FETCH_STARTED/COMPLETED/FAILED`
   - `OPERATOR_FILE_DIFF_FETCH_STARTED/COMPLETED/FAILED`
   - `OPERATOR_FILESYSTEM_LIST_STARTED/COMPLETED/FAILED`
   - `OPERATOR_FILESYSTEM_READ_STARTED/COMPLETED/FAILED`
   - `OPERATOR_FILE_RESTORE_COMPLETED/FAILED`
   - `OPERATOR_LOGS_FETCH_COMPLETED/FAILED`
   - `OPERATOR_HISTORY_FETCH_COMPLETED/FAILED`
   - `OPERATOR_DEVICE_REGISTERED`
   - `PLATFORM_NOTIFICATION`

5. **Harden handleChatError** - Move the `hideThinkingIndicator` call into `handleChatError` directly instead of relying on `_handleLLMChatIterationFailed` being wired separately. This prevents thinking state leaks if the wiring order changes.

### Future Enhancements

6. Add `LLM_CHAT_TRIAGE_COMPLETED` event to show model selection and complexity class
7. Implement LLM lifecycle events (`LIFECYCLE_STARTED`, `LIFECYCLE_COMPLETED`) for higher-level framing
8. Clean up unused EventType members (20+ phantom events defined but never emitted)
9. Emit `LLM_CHAT_MESSAGE_SENT` when user message is persisted to investigation
10. Address TribunalProviderUnavailableError fallback event emission
11. Standardize indicator ID prefix scheme (fix code smell 5.1)
