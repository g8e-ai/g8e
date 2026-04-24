---
title: Dashboard
parent: Architecture
---

# Dashboard

The dashboard is the primary UI surface in g8ed. It is served at `/chat` and consists of four main areas: the Header (with profile dropdown), the Operator Panel, the chat/message area, and the Terminal. All components communicate exclusively through `EventBus` — no component holds a direct reference to another.

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

`SetupPage` (`public/js/components/setup-page.js`) handles first-run configuration. It is served at `GET /setup` by `routes/auth/setup_routes.js` (which redirects to `/` when any user already exists) and is completely separate from the authenticated dashboard.

### Steps

| Step | `data-panel` / `data-step` | Label | Content |
|---|---|---|---|
| 1 | 1 | Account | Full name (optional) + email address; passkey note |
| 2 | 2 | AI Providers | Any combination of Gemini / Anthropic / OpenAI / Ollama keys, plus Primary / Assistant / Lite model dropdowns |
| 3 | 3 | Web Search | Search provider (Google or None) + project / app / API key fields |
| 4 | 4 | Finish | Summary + passkey registration button |

Navigation is managed by `_goToStep(step)`. Forward navigation validates the current step via `_validateStep`; back navigation skips validation. Steps are marked `active` (current) or `done` (completed) via CSS classes on `.wizard-step` elements. The `Back` / `Next` bar (`#wizard-nav`) is shown for steps 1–3 and hidden on step 4. Pressing Enter on an input or select on steps 2–3 advances to the next step.

### Step Validation (`_validateStep`)

| Step | Validation rule |
|---|---|
| 1 | Email is required and must match `/^[^\s@]+@[^\s@]+\.[^\s@]+$/` |
| 2 | At least one provider key/URL must be present, and Primary, Assistant, and Lite models must all be selected. If an Ollama URL is entered it must be `host:port` form — HTTPS and any path (e.g. `/v1`) are rejected |
| 3 | No validation (Web Search is optional) |
| 4 | No validation (passkey registration happens in the Finish handler) |

### AI Provider UI

Step 2 renders one `.wizard-provider-key-row[data-provider="<provider>"]` per supported provider, each holding its own API-key (or URL) input. The user may configure any subset of providers; there is no single "selected provider". As keys are typed, `_onProviderKeyChange` runs `_updateProviderStates` (which toggles the `has-value` class and "Configured" status label on each row) and `_updateModelDropdowns` (which rebuilds the three model menus over the union of configured providers). The Next button appears only when `_isProviderStepReady()` is true — at least one provider key is present and all three model roles are selected.

### LLM Provider Catalog (Single Source of Truth)

`components/g8ed/constants/ai.js` is the canonical catalog of `LLMProvider` values and `PROVIDER_MODELS` (model id/label per provider per tier). It is consumed:

- **Server-side** by `SSEService._getModelOptionsForProvider` (pushes the `LLM_CONFIG_RECEIVED` event to the authenticated chat UI).
- **Browser-side** at `/setup` by injecting the catalog as a JSON script tag into `views/setup.ejs`:

  ```ejs
  <script id="llm-catalog" type="application/json"><%- JSON.stringify(llmCatalog) %></script>
  ```

  `routes/auth/setup_routes.js` passes `llmCatalog = { providers: LLMProvider, providerModels: PROVIDER_MODELS }` to the render call. `setup-page.js` parses this element at module init via `_readCatalog()` and uses the result to build the model dropdowns. No browser-side duplicate of the catalog exists — adding or renaming a model requires editing only `constants/ai.js`.

Model role dropdowns are custom `.llm-model-dropdown` elements (not native `<select>`). Each role (`primary`, `assistant`, `lite`) renders one `.llm-model-dropdown__category` header per configured provider followed by one `.llm-model-dropdown__option` per model, plus a trailing `Custom…` option that prompts for a free-form model id. Selection is tracked in `_selectedModels = { primary, assistant, lite }`.

### Finish

The Finish button (`#finish-btn`) on step 4 runs a single atomic flow:

1. `POST /api/auth/register` with `{ email, name, settings }` — creates the admin user and returns `{ user_id, challenge_options }` (a WebAuthn registration challenge). The platform is blocked from accepting additional `register` calls once any user exists.
2. `navigator.credentials.create({ publicKey: options })` — prompts the browser for passkey registration.
3. `POST /api/auth/passkey/register-verify-setup` with `{ user_id, attestation_response }` — verifies the attestation, persists the credential, and returns a `session` (session cookie is set server-side).
4. On success, redirects to `/chat`.

There is no separate "save platform settings" endpoint: settings are collected by `_collectUserSettings()` and submitted inline with `register`.

### Settings Payload (`_collectUserSettings`)

The flat-keyed payload written to the user settings document (structured into the nested Pydantic shape by `g8ed/models/settings_model.js` before storage):

| Key | Source |
|---|---|
| `llm_primary_provider` / `llm_assistant_provider` / `llm_lite_provider` | Derived from the selected model id via `_modelToProvider` (falls back to the first active provider for `Custom…`) |
| `llm_model` / `llm_assistant_model` / `llm_lite_model` | Selected model id (or the custom label when `Custom…` is chosen) |
| `gemini_api_key` / `anthropic_api_key` / `openai_api_key` | Provider key inputs (omitted if empty) |
| `openai_endpoint` | Set to `https://api.openai.com/v1` when an OpenAI key is present |
| `ollama_endpoint` | `host:port` from the Ollama field |
| `vertex_search_api_key` / `vertex_search_project_id` / `vertex_search_engine_id` / `vertex_search_enabled` | Step 3 Web Search fields (only emitted when the provider is Google) |

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
| `OPERATOR_HEARTBEAT_RECEIVED` | `_onHeartbeat` | Sets `_isConnected = true`, records `_lastHeartbeat`, triggers lightweight metrics refresh |

On `_applyOperatorState`, the cause determines the update path:
- `heartbeat` — only updates metrics/status for the selected operator and its list card in-place
- `status_updated` — clears metrics panel if selected operator went offline
- All others — calls `updatePanelStatusFromOperatorCounts`, `displayOperators`, and bind/unbind button visibility

### Operator List

`OperatorListMixin.displayOperators(operators)` renders paginated operator cards. The `operators` array contains `OperatorSlot` projections — lightweight objects with only the fields needed for the operator list UI (~10 fields instead of the full `OperatorDocument`). This reduces SSE payload size significantly.

Sort priority:
1. g8e node Operators (`is_g8ep`)
2. Bound to current web session
3. Bound to another session
4. Active
5. Stale
6. All other statuses (alphabetical within tier)

Pagination: 10 operators per page. Each operator card (`operator-list-item`) shows:
- Name and hostname
- Status badge (color class from `operator.status_class`)
- First deployed / last heartbeat timestamps
- Expand/collapse to reveal the EKG-style details panel (`.operator-item-expanded-details`) with:
  - Status pill + concentric usage ring (outer=CPU, middle=MEM, inner=DISK)
  - Stats strip: LATENCY / UPTIME / USER
  - Per-metric sparkline rows for CPU / MEM / DISK (color-thresholded: `good` <65%, `warn` 65–84%, `crit` ≥85%)
  - System info grid (OS, ARCH, CPUs, MEM, public IP with obfuscation toggle, internal IP)
  - Animated EKG trace pinned to the bottom; color + scroll speed derive from overall `healthClass` (`healthy` / `loaded` / `crit` / `muted`)

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

`_initDeviceLinkDeploymentSection(overlay)` loads available slot count and existing tokens. Available slots = `maxSlots - usedSlots`. Creating a device link (`_createDeviceLink`) calls `POST /api/operator/device-link` with name, max registrations, and expiry. When a device link is created, g8ed automatically provisions any missing operator slots to fulfill the requested `max_uses` limit. Token creation shows the resulting token string in a revealed panel. Existing tokens can be revoked or deleted via their respective API calls. The create form is re-shown via "Create another" after a token is generated.

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
1. `showExecutingIndicator(command)` — creates an AI response bubble (`.anchored-terminal__ai-response--execution`) with header showing sender "g8e" and timestamp, renders spinning indicator inside the bubble content; returns indicator ID
2. `POST /api/operator/approval/direct-command` with `{ command, operator_id, web_session_id, hostname }`
3. `hideExecutingIndicator(executingId)` removes the indicator and cleans up empty parent bubbles
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

When the Tribunal verifies an AI-proposed command, a live progress widget is rendered. Event names are sourced from `components/g8ed/public/js/constants/events.js`; the session-lifecycle events are grouped under the `TRIBUNAL_SESSION_*` namespace and the per-pass/voting events under `TRIBUNAL_VOTING_*`.

| EventType | Terminal Method | Effect |
|---|---|---|
| `TRIBUNAL_SESSION_STARTED` | `showTribunal({ id, model, numPasses, request, webSessionId, correlationId })` | Renders a refining approval-compact card with dot indicators for each generation pass |
| `TRIBUNAL_VOTING_PASS_COMPLETED` | `updateTribunalPass(id, { passIndex, success })` | Marks pass dot green/red |
| `TRIBUNAL_VOTING_CONSENSUS_REACHED` / `TRIBUNAL_VOTING_AUDIT_STARTED` / `TRIBUNAL_VOTING_AUDIT_COMPLETED` | `updateTribunalStatus(id, label)` | Updates the status line inline |
| `TRIBUNAL_SESSION_COMPLETED` | `completeTribunal({ id, finalCommand, outcome })` | Shows outcome: `Consensus`, `Verified`, or `Revised` |
| `TRIBUNAL_SESSION_DISABLED` / `_MODEL_NOT_CONFIGURED` / `_PROVIDER_UNAVAILABLE` / `_SYSTEM_ERROR` / `_GENERATION_FAILED` / `_VERIFIER_FAILED` | `failTribunal({ id, reason })` | Terminal-failure labels; see `TRIBUNAL_TERMINAL_FAILURE_EVENTS` in `events.js` |

#### Refining-widget → approval upgrade contract

The refining widget rendered by `TRIBUNAL_SESSION_STARTED` carries two data attributes used to upgrade it in place when the subsequent `OPERATOR_COMMAND_APPROVAL_REQUESTED` (or file-edit / intent / agent-continue) event arrives:

- `data-approval-refining="1"` — marks the card as an unclaimed Tribunal refining widget.
- `data-correlation-id` — primary correlation key, generated by the g8ee Tribunal session (`generate_tribunal_correlation_id`) and forwarded on the approval event as `correlation_id`.
- `data-web-session-id` — fallback correlation key for legacy / non-Tribunal flows that render a refining widget without a `correlation_id`.

`handleApprovalRequest` in `anchored-terminal-execution.js` matches the refining widget in this order:

1. If the approval event carries `correlation_id`, match the refining widget whose `data-correlation-id` equals it.
2. Else if it carries `web_session_id`, match on `data-web-session-id`.
3. Else log an error and render a fresh approval card alongside the refining widget. No DOM-heuristic claim ("exactly one unclaimed widget, claim it") is performed — that masked a backend contract violation.

The `:not([data-approval-id])` guard on the selector prevents re-claiming a widget that was already upgraded by a prior approval event.

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

`handleApprovalRequest` stores the approval in `pendingApprovals` (keyed by `approval_id` or `execution_id`), creates or reuses an AI response bubble (`.anchored-terminal__ai-response--execution`) with header, renders the `approval-card` template inside the bubble content, and attaches Approve/Deny click handlers.

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
- **Final events** (completed/failed/cancelled/approval granted/rejected for commands and file edits): retrieves output/error/exit code, hides executing indicator, appends result entry to the results container (which is nested inside the AI bubble content when a parent bubble exists), removes from `activeExecutions`

Each result entry shows: status icon, hostname badge (if present), command string, timestamp, combined stdout+stderr (HTML-escaped; `(No output)` if both empty), exit code badge (green for 0, red otherwise). Results containers are collapsible; toggle label reads `Result` or `Results`.

`handleIntentResult` renders permission grant/deny outcomes — success/failure determined by `data.granted` or `eventType` being `OPERATOR_INTENT_GRANTED` / `OPERATOR_INTENT_APPROVAL_GRANTED`.

### Session Restore

When a case is loaded, `ChatComponent.restoreConversationHistory()` calls `restoreApprovalRequest`, `restoreCommandExecution`, and `restoreCommandResult` on the Terminal to replay prior history. These methods use the same DOM structure as live events (including AI bubble wrappers for approvals and results) so restored history is visually identical.

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
- Models from all configured providers are delivered via `LLM_CONFIG_RECEIVED` (`g8e.v1.ai.llm.config.received`) in a `provider_models` map grouped by provider: `{ provider_models: { gemini: { label, primary: [...], assistant: [...] }, anthropic: { ... } }, default_primary_model, default_assistant_model }`. Each model has `id` and `label`.
- `handleConfigReceived` populates both `<select>` elements with `<optgroup>` elements per provider and toggles `initially-hidden` on each container independently
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

**System health banner** (`#health-banner`) shows overall health derived from g8es KV and DB connectivity — `healthy` (green) or `degraded` (yellow). The banner includes latency readings for each subsystem.

**Panels** (two-column grid):

- **Operator Status** (`#operator-status-panel`): status distribution (non-zero statuses only), avg CPU/memory/latency, system vs cloud type counts
- **WebSession Overview** (`#session-stats-panel`): web sessions, operator sessions, total, bound operators
- **User Activity** (`#user-activity-panel`): total users, active (24h / 7d / 30d), new users (7d)
- **Login Activity (24h)** (`#login-audit-panel`): `GET /api/console/metrics/login-audit` → `getLoginAuditStats()` — counts `login_success`, `login_failed`, `account_locked`, `login_anomaly` events from `LOGIN_AUDIT` collection in the past 24 hours; failed/locked/anomaly counts are highlighted when non-zero

**Cache Performance** (full-width panel): hit rate, total hits/misses, cost savings, per-type breakdown (hits/misses per cache category). Data comes from `getCacheStats()` which reads `cacheMetrics.getStats()` directly — this panel is not cached through `_getCachedMetric`.

`GET /api/console/metrics/realtime` → `getRealTimeMetrics()` — returns g8ed process heap used/peak and current cache stats. This endpoint is not cached.

#### Components

`GET /api/console/components/health` → `getComponentHealth()` — performs live connectivity probes (not cached):

| Component | Probe | Details |
|---|---|---|
| g8ed | Always healthy (self-check) | uptime (seconds), heap used (MB), PID |
| g8es KV | `GET __console_health_check__` via Redis client | round-trip latency ms |
| g8es DB | `getDocument(COMPONENTS, 'platform_settings')` | round-trip latency ms |
| g8ee | `GET /health` via internal HTTP client | reported status from g8ee response |

Overall status: `healthy` if all components healthy; `unhealthy` if any is unhealthy; `degraded` otherwise.

Each component row renders: status icon (`check_circle` / `warning` / `error`), component name, key-value detail pairs (e.g. uptime, memory, PID), error message if present, and latency. The overall badge (`#comp-overall-badge`) reflects the aggregate status with color coding.

#### KV Store

`GET /api/console/kv/scan?pattern=<glob>&cursor=<cursor>&count=<n>` — paginated g8es KV key scan using `SCAN` with `MATCH`. Maximum `count` per request is 200. Returns `{ cursor, keys, count }`. The UI starts with `cursor='0'` and advances using the returned cursor until `cursor === '0'` again (SCAN cursor wrap).

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
