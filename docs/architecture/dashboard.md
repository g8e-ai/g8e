---
title: Dashboard & Relay
parent: Architecture
---

# Dashboard & Relay (g8ed)

Last Updated: 2026-05-11
Version: v0.2.3

`g8ed` (the Dashboard) is the central composition root and single external entry point for the g8e platform. It serves the human-interactive Terminal, manages authentication, and acts as a stateless relay between the browser, the Engine (`g8ee`), and the Operator (`g8eo`).

---

## Core Principles

### 1. Hub-and-Spoke Relay
`g8ed` is the only component exposed to the network. It handles:
- **HTTPS (443)**: Serves the EJS-rendered frontend and the JSON API.
- **WebSocket Proxy**: Tunnels `/ws/pubsub` traffic from remote Operators directly to the local `g8eo` listen port.
- **Internal Push**: Relays tool results and chat streams from `g8ee` to the browser via SSE.

### 2. Stateless Persistence (Cache-Aside)
`g8ed` does not own a database. It delegates all persistence to the Operator via the `CacheAsideService`.
- **Authoritative Write**: Data is written to the Operator's Document Store.
- **Optimistic Cache**: The Operator's KV store is used for high-speed session and metadata lookups.
- **In-Memory**: `g8ed` holds only transient state (e.g., active SSE connections).

### 3. Zero-Config Bootstrap
The `BootstrapService` allows `g8ed` to start with zero initial configuration by reading shared secrets (`internal_auth_token`) and SSL material from the host volume. It verifies its identity against a `bootstrap_digest.json` manifest recorded by `g8eo` at installation time.

---

## Network & Connectivity

### The Trust Portal (Port 80)
To solve the "Initial Trust" problem for local HTTPS, `g8ed` runs a plain HTTP server on Port 80. This portal serves:
- **CA Certificate**: Allows users to download and trust the platform's root CA.
- **Trust Scripts**: OS-specific scripts (sh/bat) to automate certificate installation.
- **Deployment Script**: A curl-pipe-bash target for remote operator installation.

### Internal Authentication
Communication between `g8ed` and other components is guarded by:
- **`X-Internal-Auth`**: A shared secret token required for all cross-component HTTP requests.
- **`requireInternalOrigin`**: Middleware that rejects any request not originating from the trusted platform network.

---

## Frontend Architecture (The Terminal)

The Terminal is a decoupled, EventBus-driven SPA served at `/chat`.

### 1. EventBus Communication
Components (Terminal, Operator Panel, Chat) communicate exclusively through a central `EventBus`. This enables loose coupling—the Terminal emits an `OPERATOR_COMMAND_APPROVAL_REQUESTED` event, and the interested UI component renders the Proof of Human Presence (PHP) card.

### 2. Mixin-Based Components
UI components use a mixin pattern (`Object.assign`) to compose functionality. This keeps core classes focused while sharing logic for scrolling, metrics, or execution handling.

### 3. Session Persistence
The active conversation state is anchored to the URL via `?investigation=<id>`. This ensures:
- **Refresh Resilience**: Users don't lose context on page reload.
- **History Navigation**: Browser back/forward buttons work naturally.
- **Deep Linking**: Direct links to specific investigations are shareable.

---

## Core Subsystems

### 1. SSE Service
The `SSEService` is the backbone of real-time reactivity. It manages persistent streams to browser tabs and routes events based on the **Routing Tuple** (`web_session_id`, `user_id`, `case_id`).

### 2. Auth & Passkeys
`g8ed` uses FIDO2/WebAuthn passkeys exclusively. The `PasskeyAuthService` manages the challenge/response lifecycle, ensuring phishing-resistant, hardware-bound authentication without passwords.

### 3. Operator Management
Operators are managed via a "Slot-Based" model. `g8ed` handles the binding/unbinding of specific operator slots to the current session, updating the UI prompt and execution routing in real-time.

---

## Data Flows

### Chat Request Flow
1. **Submit**: Browser POSTs message to `/api/chat/send`.
2. **Relay**: `g8ed` adds the `X-Internal-Auth` token and relays the request to `g8ee`.
3. **Stream**: `g8ee` processes the prompt and POSTs response chunks back to `g8ed`'s internal endpoint.
4. **Push**: `g8ed` identifies the target `web_session_id` and pushes the chunk to the browser via SSE.

### Direct Command Flow
1. **Input**: User types `/run <cmd>` in the Terminal.
2. **Respond**: Terminal POSTs to `/api/operator/approval/direct-command`.
3. **Verify**: `g8ed` checks the user's role and operator binding.
4. **Relay**: `g8ed` pushes the command to the Operator's command channel via Pub/Sub proxy.
5. **Result**: Operator returns the result, which `g8ed` proxies back to the UI.
