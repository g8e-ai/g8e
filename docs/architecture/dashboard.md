---
title: Terminal
parent: Architecture
---

# Terminal

The terminal is the primary UI surface in g8ed, served at `/chat`. It provides a unified interface for AI interaction, Operator management, command execution, and system diagnostics. All terminal components communicate exclusively through the EventBus — no component holds a direct reference to another, enabling loose coupling and testability.

## Architecture Principles

**EventBus-Driven Communication**: Components emit events to the EventBus and subscribe to events they care about. This decouples the UI from the backend and allows components to be developed, tested, and replaced independently.

**Passkey-Only Authentication**: The terminal uses FIDO2/WebAuthn passkeys exclusively — no passwords. This eliminates password-related attack vectors and provides phishing-resistant authentication.

**URL-Based Session Persistence**: The active investigation ID is stored in the URL as `?investigation=<case_id>`. This enables page refresh restoration, browser back/forward navigation, and shareable conversation links.

**Canonical Truth in Constants**: AI provider catalogs, event types, and operator statuses are defined in shared constants files (`shared/constants/*.json`). These are consumed by g8ee, g8eo, and g8ed, ensuring consistency across the platform.

## User Lifecycle

A user moves through the terminal in three phases:

### 1. First-Time Setup

The Setup Wizard (`/setup`) configures AI providers and registers the first admin user with a passkey. It is completely separate from the authenticated terminal — the route redirects to `/` once any user exists.

**Why a separate setup flow?** The platform cannot authenticate without at least one user, and it cannot create users without passkey credentials. The setup wizard bootstraps this trust anchor atomically: user creation, passkey registration, and session establishment happen in a single transaction.

The wizard reads the AI provider catalog from `components/g8ed/constants/ai.js` (the canonical source of truth), injected as a JSON script tag in the HTML. This avoids duplicating the catalog in browser code — adding a model requires editing only the constants file.

### 2. Authentication

After setup, users sign in via passkey authentication. The `AuthManager` validates the session on page load, emits `PLATFORM_AUTH_USER_AUTHENTICATED` when valid, and initializes the SSE connection. Authentication is entirely client-side: the browser calls `navigator.credentials.get` with a challenge from the server, and the assertion is verified server-side.

**Why passkeys?** They provide hardware-bound credentials that cannot be phished, replayed, or extracted. The private key never leaves the authenticator device, and the server stores only a public key credential ID.

### 3. Terminal Interaction

Once authenticated, users interact with the terminal through four main areas:

- **Header**: Profile dropdown with settings, console, audit log, and logout
- **Operator Panel**: Lists connected Operators, shows metrics, and provides deployment tools
- **Terminal**: Chat interface for AI interaction and command execution
- **Case Selector**: Dropdown for switching between conversation sessions

## Core Subsystems

### EventBus

The EventBus (`public/js/utils/eventbus.js`) is the central message bus for all terminal communication. Components emit events and subscribe to events they care about. Event type constants are defined in `constants/events.js`, sourced from `shared/constants/events.json` to ensure consistency across g8ed, g8ee, and g8eo.

**Why an EventBus?** It enables loose coupling between components. The Terminal doesn't need to know about the Operator Panel — it just emits `OPERATOR_COMMAND_APPROVAL_REQUESTED` and lets interested components react. This makes the UI easier to test, refactor, and extend.

### Terminal

The Terminal (`public/js/components/anchored-terminal.js`) is the primary interaction surface. It handles two input modes:

- **Chat mode** (default): Messages are sent to the AI via `POST /api/chat/send`. Responses stream back via SSE events (`LLM_CHAT_ITERATION_*`).
- **CLI mode** (`/run <command>`): Commands bypass the AI and execute directly on a bound Operator via `POST /api/operator/approval/direct-command`.

The Terminal is composed of four mixins via `Object.assign`:
- `TerminalScrollMixin`: Auto-scroll behavior and panel resize
- `TerminalOperatorMixin`: Operator binding state and hostname display
- `TerminalOutputMixin`: Message rendering and HTML escaping
- `TerminalExecutionMixin`: Command execution and approval handling

**Why mixins?** The Terminal's responsibilities are orthogonal. Mixing in functionality via prototype property copying keeps the core class focused while enabling code reuse without inheritance complexity.

### Operator Panel

The Operator Panel (`public/js/components/operator-panel.js`) displays connected Operators, their metrics, and deployment tools. It subscribes directly to wire events (`OPERATOR_HEARTBEAT_RECEIVED`, `OPERATOR_STATUS_UPDATED_*`) and updates the UI in real-time.

The panel is composed of seven mixins:
- `OperatorDownloadMixin`: Binary download links and device link tokens
- `OperatorDeviceLinkMixin`: Deployment token management
- `BindOperatorsMixin`: Operator bind/unbind operations
- `OperatorDeviceAuthMixin`: In-band device authorization requests
- `OperatorLayoutMixin`: Panel resize and mobile drawer mode
- `OperatorListMixin`: Paginated operator card rendering
- `OperatorMetricsDisplayMixin`: Metrics panel and expanded details

**Why wire events instead of polling?** Heartbeats arrive via pub/sub → SSE → EventBus at a fixed interval (default 5 seconds). This provides real-time updates without the overhead of HTTP polling and allows the backend to push status changes immediately.

### Chat Component

The Chat Component (`public/js/components/chat.js`) orchestrates AI interactions. It builds the chat payload (including `sentinel_mode`, model selections, case ID, and attachments), POSTs to `POST /api/chat/send`, and handles SSE streaming responses.

The component is composed of three mixins:
- `ChatAuthMixin`: Authentication state handling
- `ChatSSEHandlersMixin`: SSE event processing
- `ChatHistoryMixin`: Conversation history restoration

### Cases Manager

The Cases Manager (`public/js/components/cases-manager.js`) manages conversation sessions (investigations). It maintains a dropdown of past conversations, handles URL state (`?investigation=<case_id>`), and emits `CASE_SELECTED` / `CASE_SWITCHED` events when the user switches sessions.

**Why URL state?** Storing the investigation ID in the URL enables:
- Page refresh restoration (user doesn't lose their session on reload)
- Browser back/forward navigation (natural history navigation)
- Shareable links (user can bookmark or share a conversation)

## Safety and Governance

### Approval Flow

When the AI proposes a command, file edit, or permission escalation, the Terminal renders an interactive approval card. The user must explicitly approve or deny before execution. Risk analysis (HIGH/MEDIUM/LOW) is displayed as a badge with a tooltip explaining the score.

**Why explicit approval?** The AI operates with elevated privileges on Operators. Human-in-the-loop approval prevents accidental or malicious execution of destructive commands. The risk badge provides context without forcing the user to read through raw command text.

### Tribunal

The Tribunal is a multi-model verification system that cross-checks AI-proposed commands before requesting approval. It runs multiple generation passes, compares outputs, and only requests user approval if consensus is reached. Progress is displayed as a live widget in the Terminal.

**Why a Tribunal?** Single-model AI can hallucinate or produce unsafe commands. The Tribunal reduces this risk by requiring consensus across multiple independent models, similar to a human peer review process.

### Sentinel Mode

Sentinel Mode redacts sensitive data (passwords, keys, personal information) before sending it to the AI. When enabled, the Operator's local vault stores only the redacted copy, and the AI receives only the sanitized version.

**Why redaction instead of encryption?** Redaction is irreversible — the AI never sees sensitive data at all. Encryption requires the AI (or a downstream component) to have decryption keys, creating another attack vector. Redaction provides stronger privacy guarantees.

## Console

The Console (`/console`) is a superadmin-only diagnostic surface. It provides platform health metrics, KV store inspection, document DB queries, and a live log stream. Access requires the `SUPERADMIN` role, and all routes are gated by `requireSuperAdmin` middleware.

Metrics are computed by `ConsoleMetricsService` with in-process caching (TTL-based) to avoid expensive repeated queries. The cache can be cleared via `POST /api/console/cache/clear`.

## Data Flow

### Chat Request Flow

1. User types message in Terminal → `sendChatMessage()`
2. Terminal emits `LLM_CHAT_SUBMITTED` with message and attachments
3. ChatComponent builds payload (models, case ID, sentinel mode) → POST `/api/chat/send`
4. Backend processes request via g8ee, streams response via SSE
5. ChatComponent receives `LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED` → appends to Terminal
6. On completion, `LLM_CHAT_ITERATION_TEXT_COMPLETED` → finalizes with markdown and citations
7. If Tribunal is enabled, `TRIBUNAL_SESSION_STARTED` → renders progress widget
8. If command proposed, `OPERATOR_COMMAND_APPROVAL_REQUESTED` → renders approval card
9. User approves → `POST /api/operator/approval/respond` → command executes
10. Output arrives via `OPERATOR_COMMAND_COMPLETED` → renders in Terminal

### Operator Binding Flow

1. User clicks "Bind" on operator card → `POST /api/operator/bind`
2. Backend updates operator status to BOUND, emits `OPERATOR_STATUS_UPDATED_BOUND` via SSE
3. OperatorPanel receives event → updates operator list and metrics
4. Terminal receives event → updates hostname and prompt
5. User can now execute `/run` commands on the bound Operator

### Case Switching Flow

1. User selects case from dropdown → `switchToCase(caseId)`
2. CasesManager fetches investigation record via `GET /api/chat/investigations?case_id=<id>`
3. URL updates to `?investigation=<case_id>` via `pushState`
4. CasesManager emits `CASE_SELECTED` and `CASE_SWITCHED`
5. ChatComponent receives events → clears chat, restores conversation history
6. Terminal receives history → replays approvals and results from previous session
