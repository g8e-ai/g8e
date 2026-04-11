# Dashboard

The dashboard is the primary UI surface in VSOD. It is served at `/chat` and consists of four main areas: the Header (with profile dropdown), the Operator Panel, the chat/message area, and the Terminal. All components communicate exclusively through `EventBus` — no component holds a direct reference to another.

---

## Application Bootstrap

`g8eApp` (`app.js`) owns the top-level initialization sequence.

```
g8eApp.init()
  ├── notificationService.init()
  ├── AuthManager(eventBus)               → window.authState
  ├── SSEConnectionManager(eventBus)      → window.sseConnectionManager
  ├── Header(eventBus).init()
  ├── ChatComponent(eventBus).init()
  ├── OperatorPanel(eventBus)             → window.operatorPanel
  └── Footer(eventBus).init()
```

`AuthManager.init()` is called last. After it resolves `PLATFORM_AUTH_COMPONENT_INITIALIZED_AUTHSTATE`, `g8eApp` calls `setupUI()` (URL callback handling) and, if the user is already authenticated, initializes the SSE connection. Once `PLATFORM_AUTH_COMPONENT_INITIALIZED_CHAT` fires, `OperatorPanel.init()` runs asynchronously.

Terminal visibility is controlled by the event bus: `PLATFORM_TERMINAL_OPENED` / `PLATFORM_TERMINAL_MAXIMIZED` removes the `initially-hidden` CSS class from `[data-component="terminal"]`; `PLATFORM_TERMINAL_MINIMIZED` adds it back.

---

## Setup Wizard

`SetupPage` (`setup-page.js`) handles first-run configuration. It is served at `/setup` and is completely separate from the authenticated dashboard.

### Steps

| Step | `data-panel` | `data-step` label | Content |
|---|---|---|---|
| 1 | 1 | Account | Full name (optional) + email address fields; passkey note |
| 2 | 2 | AI Provider | Provider card selection + API key; providers: Gemini, Anthropic, OpenAI, Ollama (Remote) |
| 3 | 3 | Web Search | Search provider selection (Google/None) + API key field |
| 4 | 4 | Finish | Summary table + passkey registration |

Navigation between steps is managed by `_goToStep(step)`. Forward navigation validates the current step first (`_validateStep`); back navigation skips validation. Steps are marked `active` (current) or `done` (completed) via CSS classes on `.wizard-step` elements. A `Back` / `Next` nav bar (`#wizard-nav`) is shown only for steps 1–3; both buttons are hidden on step 4.

Pressing Enter while focused on an input or select on steps 1–3 advances to the next step.

### Step Validation

| Step | Validation rule |
|---|---|
| 1 | Email is required and must match `/^[^\s@]+@[^\s@]+\.[^\s@]+$/` |
| 2 | A provider must be selected; if the provider requires an API key that field must not be empty |
| 3 | If Google is selected as search provider, Google Project ID, Vertex AI Search App ID, and API key must not be empty |

### AI Provider Selection

Provider cards (`.wizard-provider-card`) each contain a radio input. Gemini is the first-render default selection for a fresh setup. Selecting a card calls `_selectProvider(provider)`, which activates the matching card, shows the provider's config panel (`#config-<provider>`), and hides all others. The Next button on step 2 is shown only when the step is ready (`_isProviderStepReady`): true for cloud providers only when an API key value is present, and true for Ollama when an endpoint is provided.

### Preflight

On `init()`, `_loadPreflight()` calls `GET /api/setup/config` to pre-fill existing configuration (hostname radio and provider card) so re-running setup reflects the current environment state. This may override the initial Gemini default when an existing provider is already configured.

### Finish

The Finish button (`#finish-btn`) on step 4:
1. `POST /api/setup/config` — saves the collected settings to VSODB.
2. `POST /api/auth/register` — creates the admin account (email + optional name)
3. Calls `navigator.credentials.create` for passkey registration (WebAuthn challenge/verify round-trip)
4. On success, redirects to `/chat`

Settings collected by `_collectSettings()`:

| Field | Source |
|---|---|
| `provider` | Provider card selection |
| `gemini_primary_model` / `anthropic_primary_model` / `openai_primary_model` / `ollama_primary_model` | Primary model select for chosen provider |
| `gemini_assistant_model` / `anthropic_assistant_model` / `openai_assistant_model` / `ollama_assistant_model` | Assistant model select |
| `gemini_api_key` / `anthropic_api_key` / `openai_api_key` | Provider-specific key input |
| `ollama_url` | User-supplied for Ollama |
| `search_provider` | Search provider selection (Google/None) |
| `google_project_id` | Google Project ID for Vertex AI Search |
| `vertex_ai_search_app_id` | Vertex AI Search App ID |
| `search_api_key` | API key for Google Search |
| `vertex_search_enabled` | Set to `'true'` if Google Search selected |

---

## Header and Profile Dropdown

`Header` (`header.js`) owns `<header>` and `#auth-button-container`. It adds/removes the `authenticated` CSS class on the root `<header>` element in response to `PLATFORM_AUTH_USER_AUTHENTICATED` / `PLATFORM_AUTH_USER_UNAUTHENTICATED`.

`AuthManager` (`auth.js`) populates `#auth-button-container` after a successful session validation via `renderUserProfile(session)`.

### Profile Display

`renderUserProfile` inserts a `.auth-controls-wrapper` containing a `#user-profile-display` element. The display shows the user's avatar (`<img>` with `/media/default-avatar.png` fallback). Clicking the avatar toggles the profile dropdown; clicking anywhere else closes it.

### Profile Dropdown

`createProfileDropdown(session)` builds a `.profile-dropdown` div with three sections:

**Header section** (`.profile-dropdown-header`):
- Display name (`.profile-name`) from `session.getDisplayName()`
- Email (`.profile-email`) from `session.getEmail()` — omitted if absent

**Role section** (`.profile-membership-info`):
- Label: `Role`
- Value: user role string from `getUserRole(session)`

**Actions section** (`.profile-dropdown-actions`):
- `Settings` — links to `/settings`
- `Console` — links to `/console`; shown only when `session.hasRole(UserRole.SUPERADMIN)`
- `Audit Log` — links to `/audit`
- `Logout` — button, calls `this.logout()`

The dropdown uses CSS `visibility`/`opacity` transition (`.profile-dropdown.show`) rather than `display` toggling, producing a fade-in effect.

### Authentication Flow

`AuthManager` uses FIDO2/WebAuthn passkeys exclusively — no passwords.

1. On `init()`, `validateSession()` calls `GET /api/auth/session`. If a valid session exists, `_handleAuthenticatedSession` sets `this.session` (a `WebSessionModel`), emits `PLATFORM_AUTH_USER_AUTHENTICATED`, and calls `renderUserProfile`.
2. On sign-in, the browser calls `navigator.credentials.get` with decoded WebAuthn options from `POST /api/auth/passkey/authenticate/challenge`. The serialized assertion is sent to `POST /api/auth/passkey/authenticate/verify`. On success, `validateSession()` is called again to pick up the new session.
3. `logout()` calls `POST /api/auth/logout` and redirects to `/`.

`AuthManager` exposes: `getWebSessionId()`, `getWebSessionModel()`, `getState()`, `getApiKey()`, `isAuthenticated()`, `hasRole()`, `isAdmin()`, `subscribe(callback)`.

---

## Operator Panel

`OperatorPanel` (`operator-panel.js`) is the left-side panel that shows connected Operators, system metrics, and deployment tools. It is assembled from seven mixins via `Object.assign`.

```
OperatorPanel                         (operator-panel.js)
  ├── OperatorDownloadMixin           (operator-download-mixin.js)
  ├── OperatorDeviceLinkMixin         (operator-device-link-mixin.js)
  ├── BindOperatorsMixin               (operator-bind-mixin.js)
  ├── OperatorDeviceAuthMixin         (operator-device-auth-mixin.js)
  ├── OperatorLayoutMixin             (operator-layout-mixin.js)
  ├── OperatorListMixin               (operator-list-mixin.js)
  └── OperatorMetricsDisplayMixin     (operator-metrics-display-mixin.js)
```

### Initialization

`OperatorPanel.init()` is called asynchronously after `PLATFORM_AUTH_COMPONENT_INITIALIZED_CHAT` fires:
1. `preloadTemplates()` — fetches and caches all panel HTML templates via `templateLoader`
2. `render()` — injects `operator-panel-container` template into `#operator-panel-container`, caches all DOM references, initializes state, calls `_initPanelResize()`
3. `bindEvents()` — attaches scroll containment, download collapsible toggle, Escape key handler, bind-all and unbind-all button listeners
4. `setupThemeListener()` — watches `data-theme` changes to update platform download icons
5. `_setupAuthStateListener()` — subscribes to auth state changes to populate the API key and display initial operator status

Wire listeners (`_setupWireListeners`) are registered in the constructor before `init()` runs, so events received before the DOM is ready are queued as `_pendingRender` and replayed after `render()` completes.

### Wire Event Subscriptions

| EventType | Handler | Effect |
|---|---|---|
| `OPERATOR_PANEL_LIST_UPDATED` | `_onListUpdated` | Replaces `_operators` array, updates counts and `operatorSessionService`, triggers full re-render |
| `OPERATOR_STATUS_UPDATED_*` (all 8 variants) | `_onStatusUpdated` | Upserts the operator in `_operators`, recomputes counts, triggers state apply |
| `OPERATOR_HEARTBEAT_SENT` | `_onHeartbeat` | Sets `_isConnected = true`, records `_lastHeartbeat`, triggers lightweight metrics refresh |

On `_applyOperatorState`, the cause determines the update path:
- `heartbeat` — only updates metrics/status for the selected operator and its list card in-place
- `status_updated` — clears metrics panel if selected operator went offline
- All others — calls `updatePanelStatusFromOperatorCounts`, `displayOperators`, and bind/unbind button visibility

### Operator List

`OperatorListMixin.displayOperators(operators)` renders paginated operator cards. Sort priority:
1. g8e node Operators (`is_g8e_pod`)
2. Bound to current web session
3. Bound to another session
4. Active
5. Stale
6. All other statuses (alphabetical within tier)

Pagination: 10 operators per page. Each operator card (`operator-list-item`) shows:
- Name and hostname
- Status badge (color class from `operator.status_class`)
- First deployed / last heartbeat timestamps
- Expand/collapse for metrics when bound to current session

Action buttons:
- **Get Device Link Token** (`dns`)
- **Restart g8ep Operator** (`restart_alt`)
- **Copy API Key** (`vpn_key`)
- **Refresh API Key** (`key_off`)
- **Bind/Unbind** (`link` / `link_off`)
- **Stop Operator** (`stop_circle`)

### Operator Bind / Unbind

`BindOperatorsMixin` handles bind and unbind operations:

- **Single bind**: `POST /api/operator/bind` with `{ operator_id }` → emits `OPERATOR_BOUND`, updates `boundOperatorIds`, updates metrics and status display
- **Single unbind**: `POST /api/operator/unbind` (optionally with `{ operator_id }` for forced unbind) → emits `OPERATOR_UNBOUND`, removes from `boundOperatorIds`
- **Bind all / Unbind all**: show a confirmation overlay before calling the batch endpoints
- Bind and unbind all button visibility is updated after every operation

### Metrics Display

`OperatorMetricsDisplayMixin` manages the metrics panel (CPU, memory, disk, network latency, and expanded system details).

`updateMetrics(data)` wraps the raw operator data in `OperatorMetrics`, validates it, and updates:
- `hostnameElement` — hostname display
- CPU, memory, disk — percentage values and progress bar fill width/color (green/yellow/red via `getProgressLevel`)
- Network latency — ms value and `latency-indicator` quality class (from `getLatencyQuality`)

`updateExpandedDetails(operatorMetrics)` populates the expanded details panel with: OS, kernel, architecture, uptime, current user, shell, home directory, timezone, disk usage, memory usage, public IP (obfuscated by default behind `#ip-visibility-toggle`), latency, working directory, and language.

`updateStatus(status)` updates `statusElement.textContent` and toggles the `offline` class on the panel based on whether status is `ACTIVE` or `BOUND`.

`clearPanelMetrics()` resets all metric displays to their empty/default state.

`updatePanelStatusFromOperatorCounts()` updates the panel header badge (total/active counts, slot usage).

### Download and Deployment

`OperatorDownloadMixin` manages a collapsible download section in the panel footer.

Clicking the download bar (`#operator-download-collapsible-bar`) expands/collapses the section. On first expand, `populateDownloadSection()` renders the `operator-initial-download-overlay` template and calls `_populateBinaryDownloadLinks`, which injects pre-authorized `curl` download commands using the user's API key for each supported platform/architecture:

| OS | Arch | Label |
|---|---|---|
| macOS | amd64 | macOS Intel |
| macOS | arm64 | macOS Apple Silicon (M1/M2/M3) |
| Linux | amd64 | Linux x64 |
| Linux | arm64 | Linux ARM64 |
| Linux | 386 | Linux x86 |

Platform selection is a stacked overlay (`downloadMenuStack`) — users select OS then architecture. Escape key or back navigation pops the overlay stack.

### Device Links

`OperatorDeviceLinkMixin` manages deployment tokens (device links) within the download overlay.

`_initDeviceLinkDeploymentSection(overlay)` loads available slot count and existing tokens. Available slots = `maxSlots - usedSlots`. Creating a device link (`_createDeviceLink`) calls `POST /api/operator/device-link` with name, max registrations, and expiry. Token creation shows the resulting token string in a revealed panel. Existing tokens can be revoked or deleted via their respective API calls. The create form is re-shown via "Create another" after a token is generated.

### Device Authorization

`OperatorDeviceAuthMixin` handles in-band device authorization requests from Operators that require explicit user approval before they can register. Incoming `OPERATOR_DEVICE_AUTHORIZATION_REQUESTED` events are stored in `_pendingAuthRequests` and rendered as approval prompts. Approving or denying calls the relevant authorization API and clears the pending entry.

### Panel Resize

`OperatorLayoutMixin._initPanelResize()` makes the panel width adjustable via the `#panel-divider` drag handle.

Constraints:
- Minimum width: 240 px
- Maximum width: 80% of the parent container width

Supports both mouse (`mousedown`/`mousemove`/`mouseup`) and touch (`touchstart`/`touchmove`/`touchend`) drag. Drag is suppressed when `panelContainer` has the `mobile-drawer-mode` class.

---

## Terminal

The Terminal is the primary interaction surface in the dashboard. It provides a persistent input/output panel where users send messages to the AI, receive streamed responses, review and approve AI-proposed commands, and execute direct shell commands against a bound Operator.

> **Note on naming**: The Terminal class and its files are named `AnchoredOperatorTerminal` / `anchored-terminal.*` in the codebase — a legacy artifact from an earlier refactor. There is now only one terminal. The plan is to rename it to `Terminal` in the code; until then, the class name `AnchoredOperatorTerminal` refers to the Terminal described here.

### Component Structure

```
AnchoredOperatorTerminal          (anchored-terminal.js)
  ├── TerminalScrollMixin          (anchored-terminal-scroll.js)
  ├── TerminalOperatorMixin        (anchored-terminal-operator.js)
  ├── TerminalOutputMixin          (anchored-terminal-output.js)
  └── TerminalExecutionMixin       (anchored-terminal-execution.js)
```

`applyMixins` uses `Object.getOwnPropertyDescriptor` / `Object.defineProperty` to copy every non-constructor property from each mixin's prototype onto `AnchoredOperatorTerminal.prototype`, preserving getter/setter descriptors. Initialization state for each mixin (`initScrollState`, `initOperatorState`, `initExecutionState`) is called from the constructor. DOM binding and event-bus subscription happen in `init()`, which is idempotent.

### DOM References

`cacheDOMReferences()` populates the following from the document:

| Property | Element ID / Selector |
|---|---|
| `container` | `anchored-terminal-container` |
| `terminal` | `anchored-terminal` |
| `outputContainer` | `anchored-terminal-output` |
| `inputElement` | `anchored-terminal-input` |
| `sendButton` | `anchored-terminal-send` |
| `hostnameElement` | `anchored-terminal-hostname` |
| `promptElement` | `anchored-terminal-prompt` |
| `attachmentButton` | `anchored-terminal-attach` |
| `attachmentsDisplay` | `anchored-terminal-attachments` |
| `modeIndicator` | `anchored-terminal-mode` |
| `resizeHandle` | `panel-resize-handle` |
| `maximizeButton` | `anchored-terminal-maximize` |
| `inputArea` | `.anchored-terminal__input-area` (queried within `terminal`) |
| `scrollContainer` | `anchored-terminal-body` |

If `container` is not found, `init()` aborts immediately.

### Initialization Sequence

1. `constructor(eventBus)` — stores event bus; initializes `commandHistory`, `historyIndex` (`-1`), `activeStreamingResponses` (Map), `thinkingContentRaw` (Map), `_escapeDiv`, `_eventsBound`, `_initialized`; calls `initScrollState`, `initOperatorState`, `initExecutionState`
2. `init()` — idempotent (`_initialized` guard):
   - `cacheDOMReferences`
   - `bindDOMEvents` — input, keydown (Enter/ArrowUp/ArrowDown), send-button click, resize-handle mousedown, maximize-button click
   - `bindScrollListener` — passive scroll on `scrollContainer`
   - `bindEventBusListeners` — subscribes to all event-bus topics
   - `showWelcomeMessage` — mounts `OperatorDeployment` component into the output area
   - sets `_initialized = true`

The welcome component is destroyed and removed the first time any real output is appended (`_removeWelcome()`).

### Input Modes and Command Dispatch

The `#anchored-terminal-mode` element renders `Chat` or `CLI` live on every `input` event based on whether the input begins with `/run `.

`executeCommand()` is triggered by Enter (without Shift) or send-button click:
1. Trim input; abort if empty
2. Reset auto-scroll
3. Push to `commandHistory`; reset `historyIndex` to `commandHistory.length`
4. Clear input and update mode indicator
5. If input starts with `/run `: extract command, call `executeDirectCommand(command)` (CLI path)
6. Otherwise: call `sendChatMessage(input)` (Chat path)

ArrowUp/ArrowDown traverse `commandHistory` in reverse; at the end of the list the input is cleared.

### Chat Path

`sendChatMessage(input)`:
1. Guard: `currentUser` must be set
2. Collect attachments via `attachmentsUI.manager.getFormattedForBackend()`
3. Emit `LLM_CHAT_SUBMITTED` with `{ message, attachments, initiatedBySystem: false, useCurrentAttachments: false }`
4. Clear attachments

`ChatComponent` handles `LLM_CHAT_SUBMITTED` → `submitChatMessage`, which builds the full payload (including `sentinel_mode`, `llm_primary_model`, `llm_assistant_model`, `case_id`, `investigation_id`, `attachments`) and POSTs to `POST /api/chat/send`.

### CLI Path (`/run`)

`/run <command>` bypasses the AI and sends the command directly to the bound Operator.

Prerequisites: Operator must be bound (`isOperatorBound === true`); active web session required.

Flow:
1. `showExecutingIndicator(command)` — renders spinning indicator; returns indicator ID
2. `POST /api/operator/approval/direct-command` with `{ command, operator_id, web_session_id, hostname }`
3. `hideExecutingIndicator(executingId)` unconditionally on response
4. Success with `execution_id`: tracked in `activeExecutions`; output arrives via command execution events
5. Failure: results container created immediately with the error rendered as a failed result

### Streaming AI Responses

| EventType | Terminal Method | Effect |
|---|---|---|
| *(before POST)* | `showWaitingIndicator(webSessionId)` | Animated cursor while waiting |
| `OPERATOR_TERMINAL_THINKING_APPEND` | `appendThinkingContent(webSessionId, text)` | Creates/updates collapsible thinking block with dynamic title |
| `LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED` | `appendStreamingTextChunk(webSessionId, text)` | Appends raw text nodes to streaming response (accumulates plain text) |
| `LLM_CHAT_ITERATION_TEXT_COMPLETED` | `finalizeAIResponseChunk(webSessionId, finalHtml)` | Replaces streaming content with final markdown + citations; removes `streaming` class |
| `LLM_CHAT_ITERATION_CITATIONS_RECEIVED` | `applyCitationsAfterFinalize(webSessionId, groundingMetadata)` | Injects inline citation markers and sources panel |
| `LLM_CHAT_ITERATION_FAILED` | `appendErrorMessage(text)` | Renders error block |

`getOrCreateAIResponse(webSessionId)` creates `div.anchored-terminal__ai-response.streaming` on first call; subsequent calls return the existing element. On finalization the `streaming` class is removed and the element ID is suffixed with a timestamp to prevent stale lookups.

Thinking blocks (`getOrCreateThinkingEntry`) accumulate raw text in `thinkingContentRaw` (Map keyed by `webSessionId`) and re-render as markdown on each chunk. `_extractThinkingTitle` scans accumulated text bottom-up for markdown bold (`**Title**`) or heading (`## Title`) patterns and updates the header title bar in real time as new thought events arrive. The title bar shows a `+`/`–` toggle indicator and is clickable to expand/collapse the full thought stream. On completion the block is collapsed with the last title preserved.

### Tribunal Progress

When the Tribunal verifies an AI-proposed command, a live progress widget is rendered:

| EventType | Terminal Method | Effect |
|---|---|---|
| `TRIBUNAL_STARTED` | `showTribunal({ id, model, numPasses, command })` | Dot indicators for each generation pass |
| `TRIBUNAL_VOTING_PASS_COMPLETED` | `updateTribunalPass(id, { passIndex, success })` | Marks pass dot green/red |
| `TRIBUNAL_COMPLETED` | `completeTribunal({ id, finalCommand, outcome })` | Shows outcome: `Consensus`, `Verified`, or `Revised` |
| `TRIBUNALFALLBACK_TRIGGERED` | `failTribunal({ id, reason })` | Shows fallback reason label |

Fallback reason labels: `DISABLED` → `TribunalDisabled`; `PROVIDER_UNAVAILABLE` → `Tribunal unavailable`; `ALL_PASSES_FAILED` → `All passes failed — using original`; `NO_VOTE_WINNER` → `No consensus — using original`; otherwise → `Using original command`.

### Operator Binding

`TerminalOperatorMixin` tracks which Operator is connected.

| Property | Type | Description |
|---|---|---|
| `isOperatorBound` | boolean | Whether an Operator is currently bound |
| `boundOperator` | object | Full operator record from the last bind event |

`setOperatorBound(operator)` — short-circuits if same operator (by `operator_id`) is already bound; updates `hostnameElement` from `system_info.hostname` → `operator.name` → `'operator'`; sets `promptElement` to `<current_user>$` falling back to `'$'`; calls `updateInputState()`; appends `Connected to <name>` system message.

`setOperatorUnbound()` — clears `isOperatorBound`, `boundOperator`, `hostnameElement`, resets prompt to `'$'`, calls `updateInputState()`.

`updateInputState` enables/disables the input, send button, and attachment button based on `isAuthenticated`. The placeholder text reflects sign-in state.

Event-bus subscriptions:

| EventType | Handler |
|---|---|
| `OPERATOR_STATUS_UPDATED_BOUND` | `setOperatorBound(data.operator)` |
| `OPERATOR_STATUS_UPDATED_ACTIVE`, `_AVAILABLE`, `_UNAVAILABLE`, `_OFFLINE`, `_STALE`, `_STOPPED`, `_TERMINATED` | `setOperatorUnbound()` |
| `OPERATOR_PANEL_LIST_UPDATED` | `handleOperatorListUpdate` |
| `OPERATOR_BOUND` | `handleOperatorBound` |
| `OPERATOR_UNBOUND` | `handleOperatorUnbound` |
| `OPERATOR_COMMAND_APPROVAL_REQUESTED`, `OPERATOR_FILE_EDIT_APPROVAL_REQUESTED`, `OPERATOR_INTENT_APPROVAL_REQUESTED` | `handleApprovalRequest` |
| Command execution events (started, completed, failed, cancelled, approval granted/rejected) | `handleCommandExecutionEvent` |
| Intent events (granted, denied, revoked, approval granted/rejected) | `handleIntentResult` |
| `OPERATOR_TERMINAL_APPROVAL_DENIED` | `denyAllPendingApprovals` |
| `OPERATOR_TERMINAL_AUTH_STATE_CHANGED` | sets user, enables/disables input |

### Approval Flow

When the AI proposes a command, file edit, or permission escalation requiring human approval, the Terminal renders an interactive approval card.

**Approval types:**

| Condition | Header |
|---|---|
| Has `file_path` and `operation` | `File Edit` |
| Has `intent_name` and `intent_question` | `Escalation` |
| Has `is_batch_execution` with multiple `target_systems` | `Command (<N> systems)` |
| Default | `Command` |

For shell commands, a risk badge shows `risk_level` from `data.risk_analysis`:

| `risk_level` | Icon | Badge color |
|---|---|---|
| `HIGH` | `warning` | high |
| `MEDIUM` | `priority_high` | medium |
| `LOW` | `check_circle` | low |

The badge tooltip includes `risk_score/10`, destructive flag, and blast radius when present.

`handleApprovalRequest` stores the approval in `pendingApprovals` (keyed by `approval_id` or `execution_id`), renders the `approval-card` template, and attaches Approve/Deny click handlers.

`handleApprovalResponse(approvalId, approved)`:
1. Disables both buttons immediately
2. `POST /api/operator/approval/respond` with approval metadata and `approved: true|false`
3. Replaces action buttons with an approved/denied status badge
4. If approved: creates a results container anchored below the card and shows an executing indicator
5. Removes the approval from `pendingApprovals`
6. On HTTP error: re-enables buttons

`denyAllPendingApprovals` fires on `OPERATOR_TERMINAL_APPROVAL_DENIED` — iterates `pendingApprovals`, fires `POST /api/operator/approval/respond` for each with `approved: false`, and clears the map.

### Command Execution Events

After an approval or `/run` command, the Operator reports progress via pub/sub → SSE → event-bus. `handleCommandExecutionEvent` drives a state machine on `data.eventType`:

- **`OPERATOR_COMMAND_STARTED`**: looks up or creates a results container; registers in `activeExecutions`
- **Final events** (completed/failed/cancelled/approval granted/rejected for commands and file edits): retrieves output/error/exit code, hides executing indicator, appends result entry to the results container, removes from `activeExecutions`

Each result entry shows: status icon, hostname badge (if present), command string, timestamp, combined stdout+stderr (HTML-escaped; `(No output)` if both empty), exit code badge (green for 0, red otherwise). Results containers are collapsible; toggle label reads `Result` or `Results`.

`handleIntentResult` renders permission grant/deny outcomes — success/failure determined by `data.granted` or `eventType` being `OPERATOR_INTENT_GRANTED` / `OPERATOR_INTENT_APPROVAL_GRANTED`.

### Session Restore

When a case is loaded, `ChatComponent.restoreConversationHistory()` calls `restoreApprovalRequest`, `restoreCommandExecution`, and `restoreCommandResult` on the Terminal to replay prior history. These methods use the same DOM structure as live events so restored history is visually identical.

### Scroll Behaviour

`TerminalScrollMixin` manages automatic scroll-to-bottom and panel resize.

- `userHasScrolled` initializes to `true` (auto-scroll suppressed until `resetAutoScroll()` is called)
- `scrollToBottom()` schedules a `requestAnimationFrame` to set `scrollTop = scrollHeight`; skipped if `userHasScrolled` is true unless `force: true` is passed
- Passive scroll listener: sets `userHasScrolled = true` when the user scrolls away from the bottom; resets to `false` when back within 100 px of the bottom
- `resetAutoScroll()` sets `userHasScrolled = false`; called at the start of `executeCommand()` and before conversation history restore
- `scrollToBottom({ smooth: true })` uses `scrollTo({ behavior: 'smooth' })`

**Vertical resize**: the `#panel-resize-handle` drag handle adjusts the terminal height. Constraints: 180 px minimum, 600 px maximum; chat messages panel 150 px minimum.

### Input Area Maximize

The maximize button (`#anchored-terminal-maximize`) toggles the `maximized` CSS class on `.anchored-terminal__input-area`. Button title updates to `Collapse input area` / `Expand input area`. Focus is returned to the input after every toggle.

### Authentication Integration

`OPERATOR_TERMINAL_AUTH_STATE_CHANGED`:
- `isAuthenticated === true` → `setUser(user)`, `enable()`, `focus()`
- `isAuthenticated === false` → `setUser(null)`, `disable()`

`setUser(user)` updates `currentUser`, `isAuthenticated`, calls `updateInputState()`, and propagates the user to the `OperatorDeployment` welcome component if still mounted. `disable()` sets input and send button to `disabled = true` regardless of auth state. `enable()` delegates to `updateInputState()`.

### HTML Safety

All user-supplied and operator-supplied text is HTML-escaped via `escapeHtml(text)`: text is assigned to `this._escapeDiv.textContent` and the `.innerHTML` is read back. Direct HTML response rendering (`appendDirectHtmlResponse`) and finalized AI response HTML (`finalizeAIResponseChunk`) set `.innerHTML` directly from DOMPurify-sanitized markdown output, which is trusted.

---

## Cases and Conversation Management

Cases and investigations are managed by `CasesManager` (`cases-manager.js`), instantiated by `ChatComponent.initCasesManager()`. The Terminal reacts to case lifecycle events but does not own case state.

### Case Selection

1. On authentication, `CasesManager.loadUserCases` fetches investigations via `GET /api/chat/investigations`. The backend may also push `INVESTIGATION_LIST_COMPLETED` (`g8e.v1.app.investigation.list.completed`) on SSE connection.
2. `handleInvestigationQuerySuccess` populates the case dropdown and — if `?investigation=<case_id>` is in the URL — automatically calls `switchToCase` to restore the session.
3. `switchToCase` fetches the full investigation record via `GET /api/chat/investigations?case_id=<id>`, then emits `CASE_SELECTED` (carries `caseId`, `investigationId`, `caseData`, `conversationHistory`) and `CASE_SWITCHED` (carries `caseId`, `investigationId`, full investigation object).
4. **New Case**: `resetForNewCase` clears all IDs, emits `CASE_CLEARED`, removes `?investigation` from the URL.
5. On the first message in a new session, the backend emits `CASE_CREATED`; `CasesManager._applyCaseCreationResult` updates IDs and inserts the new case into the dropdown.

`CASE_SELECTED` → `ChatComponent` calls `clearChat()` then `handleCaseSelected()` to restore conversation history. `CASE_CLEARED` → `ChatComponent.handleCaseCleared()` calls `clearChat()` → `clearOutput()` on the Terminal, which destroys the welcome component, clears `outputContainer.innerHTML`, clears `activeStreamingResponses`/`pendingApprovals`/`thinkingContentRaw`/`executionResultsContainers`, and re-mounts a fresh welcome component if `currentUser` is set.

### URL State

The active investigation ID is kept in the URL as `?investigation=<case_id>`. `replaceState` is used during auto-restore; `pushState` for explicit user navigation. Browser back/forward (`popstate`) calls `switchToCase` to restore the correct session.

---

## Sentinel Mode

`SentinelModeManager` (`sentinel-mode-manager.js`) manages the Sentinel Mode toggle.

- **DOM**: `#sentinel-mode-toggle` (wrapper), `#sentinel-mode-checkbox` (input), `#sentinel-mode-container` (outer)
- **Default**: enabled (`sentinelModeEnabled = true`)
- On checkbox change, emits `PLATFORM_SENTINEL_MODE_CHANGED` (`g8e.v1.platform.sentinel.mode.changed`) with `{ enabled, investigationId }`
- `CASE_SWITCHED` → `handleCaseSwitched` restores `data.investigation.sentinel_mode`
- `INVESTIGATION_LOADED` → `handleInvestigationLoaded` restores sentinel state from the investigation document
- `CASE_CLEARED` → resets to `true`

`SentinelModeManager.getSentinelMode()` is called by `ChatComponent` when building the payload for `POST /api/chat/send`. The `sentinel_mode` boolean is included in every request body.

When Sentinel is ON, the Operator's local vault stores only redacted data and the AI receives only the redacted copy.

---

## AI Model Selection

`LlmModelManager` (`llm-model-manager.js`) manages two model pickers: primary (complex tasks) and assistant (simple tasks).

- **DOM**: `#llm-primary-model-container` + `#llm-primary-model-select`, `#llm-assistant-model-container` + `#llm-assistant-model-select`
- Provider-specific model lists delivered via `LLM_CONFIG_RECEIVED` (`g8e.v1.ai.llm.config.received`) with `{ primary_models: [...], assistant_models: [...], default_primary_model, default_assistant_model }`; each model has `id` and `label`
- `handleConfigReceived` populates both `<select>` elements and toggles `initially-hidden` on each container independently
- `CASE_SWITCHED` → restores `data.investigation.llm_primary_model` and `llm_assistant_model`; `CASE_CLEARED` → resets both to server defaults

`LlmModelManager.getPrimaryModel()` and `.getAssistantModel()` are called by `ChatComponent` when building the `POST /api/chat/send` payload. Empty string tells the server to use its configured default.

---

## Console

The Console is a superadmin-only diagnostic surface served at `/console`. It provides platform health, metrics, raw data inspection (KV store and document DB), and a live log stream. Access requires the `SUPERADMIN` role — all console routes are gated by `requireSuperAdmin` middleware and subject to `consoleRateLimiter`. The profile dropdown shows the `Console` link only when `session.hasRole(UserRole.SUPERADMIN)`.

Metrics are computed by `ConsoleMetricsService` (`services/platform/console_metrics_service.js`) and served under the `/api/console/` prefix.

### Metrics Caching

`ConsoleMetricsService` maintains an in-process `Map` (`metricsCache`) with a TTL of `CONSOLE_METRICS_CACHE_TTL_MS`. All stat methods (`getUserStats`, `getOperatorStats`, `getSessionStats`, `getLoginAuditStats`, `getAIUsageStats`) delegate to `_getCachedMetric(key, computeFn)`:
- Returns the cached entry if its age is under the TTL
- Otherwise computes fresh data via `computeFn`, stores it, and returns it
- On compute error, falls back to the stale cached value if present; otherwise returns `null`

`POST /api/console/cache/clear` calls `clearCache()` which empties the map and triggers an immediate reload.

### Tabs

The console UI is split into five tabs. Each tab panel (`#tab-<name>`) is activated on click; tab content is lazy-initialized — `initTab(name)` is called only the first time a tab is activated.

#### Overview

Activated by default on page load. Calls `loadOverview()`, `loadLoginAudit()`, and `loadAIStats()` in parallel.

**`GET /api/console/overview`** → `getPlatformOverview()` — returns a `MetricsSnapshot` combining `getUserStats`, `getOperatorStats`, `getSessionStats`, `getCacheStats`, and `getSystemHealth` in a single parallel fetch.

**Stats grid** (top of the tab):

| Card | Value | Sub-text |
|---|---|---|
| Total Users | `users.total` | `users.activity.lastWeek` active (7d) |
| Operators | `operators.total` | `operators.health.healthy` healthy |
| Active Sessions | `sessions.web` | `sessions.operator` operator sessions |
| Investigations (7d) | `ai.totalInvestigations` | `ai.activeInvestigations` active |

**System health banner** (`#health-banner`) shows overall health derived from VSODB KV and DB connectivity — `healthy` (green) or `degraded` (yellow). The banner includes latency readings for each subsystem.

**Panels** (two-column grid):

- **Operator Status** (`#operator-status-panel`): status distribution (non-zero statuses only), avg CPU/memory/latency, system vs cloud type counts
- **WebSession Overview** (`#session-stats-panel`): web sessions, operator sessions, total, bound operators
- **User Activity** (`#user-activity-panel`): total users, active (24h / 7d / 30d), new users (7d)
- **Login Activity (24h)** (`#login-audit-panel`): `GET /api/console/metrics/login-audit` → `getLoginAuditStats()` — counts `login_success`, `login_failed`, `account_locked`, `login_anomaly` events from `LOGIN_AUDIT` collection in the past 24 hours; failed/locked/anomaly counts are highlighted when non-zero

**Cache Performance** (full-width panel): hit rate, total hits/misses, cost savings, per-type breakdown (hits/misses per cache category). Data comes from `getCacheStats()` which reads `cacheMetrics.getStats()` directly — this panel is not cached through `_getCachedMetric`.

`GET /api/console/metrics/realtime` → `getRealTimeMetrics()` — returns VSOD process heap used/peak and current cache stats. This endpoint is not cached.

#### Components

`GET /api/console/components/health` → `getComponentHealth()` — performs live connectivity probes (not cached):

| Component | Probe | Details |
|---|---|---|
| VSOD | Always healthy (self-check) | uptime (seconds), heap used (MB), PID |
| VSODB KV | `GET __console_health_check__` via Redis client | round-trip latency ms |
| VSODB DB | `getDocument(COMPONENTS, 'platform_settings')` | round-trip latency ms |
| g8ee | `GET /health` via internal HTTP client | reported status from g8ee response |

Overall status: `healthy` if all components healthy; `unhealthy` if any is unhealthy; `degraded` otherwise.

Each component row renders: status icon (`check_circle` / `warning` / `error`), component name, key-value detail pairs (e.g. uptime, memory, PID), error message if present, and latency. The overall badge (`#comp-overall-badge`) reflects the aggregate status with color coding.

#### KV Store

`GET /api/console/kv/scan?pattern=<glob>&cursor=<cursor>&count=<n>` — paginated VSODB KV key scan using `SCAN` with `MATCH`. Maximum `count` per request is 200. Returns `{ cursor, keys, count }`. The UI starts with `cursor='0'` and advances using the returned cursor until `cursor === '0'` again (SCAN cursor wrap).

Clicking a key: `GET /api/console/kv/key?key=<key>` → `getKVKey(key)` — fetches value and TTL via `GET` + `TTL` in parallel. The detail panel (`#kv-detail-panel`) renders the key name, value, TTL, and existence flag.

#### Database

`GET /api/console/db/collections` — returns `Object.values(Collections)` (all known collection names). These populate the `#db-collection` select element on tab activation.

`GET /api/console/db/query?collection=<name>&limit=<n>` — queries the document store for up to `limit` documents (max 200) from the named collection. The collection must be one of the known `Collections` values; unknown collection names return 400. Results are rendered as a list of document rows; clicking a row opens the full document JSON in the detail panel (`#db-detail-panel`).

#### Logs

`GET /api/console/logs/stream?level=<level>&limit=<n>` — SSE endpoint that:
1. Replays up to `limit` (max 500) recent entries from the in-process log ring buffer, filtered to the requested level and below (`error < warn < info < debug` priority order)
2. Sends a `PLATFORM_CONSOLE_LOG_CONNECTED_CONFIRMED` event with the buffered entry count
3. Registers a live listener via `addLogListener` that forwards new log entries in real time as `PLATFORM_CONSOLE_LOG_ENTRY_RECEIVED` events
4. Removes the listener on SSE `close`

UI controls:
- **Level select** (`#logs-level`): `error`, `warn`, `info` (default), `debug` — changing reconnects the stream
- **Pause** (`#logs-pause-btn`): suspends rendering new entries without disconnecting
- **Clear** (`#logs-clear-btn`): empties the log container
- **Auto-scroll** (`#logs-scroll-btn`): toggles scroll-to-bottom on new entries

The status badge (`#logs-status-badge`) shows `Connected` / `Connecting...` / `Disconnected` based on SSE connection state.
