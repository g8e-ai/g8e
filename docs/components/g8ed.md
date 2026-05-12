---
title: g8ed
parent: Components
---

# g8ed — g8e Dashboard

Last Updated: 2026-05-11
Version: v0.2.4

## Overview

`g8ed` is the central orchestration layer, authentication gateway, and primary entry point for the g8e platform. It serves the interactive dashboard, manages secure sessions, and coordinates communication between the user, the AI Engine (`g8ee`), and the `operator`.

**Core Responsibilities:**
- **Single Entry Point:** All inbound traffic (HTTPS, WebSocket, SSE) is terminated at `g8ed`.
- **Authentication Gateway:** Owns the Passkey (WebAuthn), API Key, and Device Link token flows. Device Link tokens are used for initial operator deployment and authentication, but execution requires subsequent binding to a human web session.
- **Session Management:** Manages stateful `WebSession` and `OperatorSession` objects using operator-backed persistence.
- **Operator Orchestration:** Manages operator "slots," handling enrollment, heartbeat monitoring, and manual user-to-operator binding.
- **Real-time Eventing:** Provides a high-performance SSE (Server-Sent Events) hub for fanning out platform events and AI streaming responses.
- **Trust Portal:** Serves workstation CA certificates and automated trust scripts over plain HTTP for initial setup.

---

## Architecture

`g8ed` is a stateless Node.js (Express) application that delegates all persistent storage to the `operator` document and KV stores.

### The "Zero-Config" Bootstrap
`g8ed` reads zero environment variables or local config files for runtime settings. Instead, it follows a two-phase bootstrap:
1. **Physical Bootstrap:** `BootstrapService` reads the `internal_auth_token` and SSL paths from the local host volume (managed by `g8e.operator --listen`).
2. **Logical Bootstrap:** `SettingsService` connects to the operator using the physical token to retrieve global platform configuration (LLM providers, allowed origins, etc.).

### Network Topology
- **Port 443 (HTTPS):** The primary secure entry point for all UI and API traffic.
- **Port 80 (HTTP):** A dedicated "Trust Portal" serving `/ca.crt` and `/trust` scripts to bootstrap SSL trust on local workstations.
- **WebSocket Tunnel:** `g8ed` proxies `/ws/pubsub` directly to the `operator` to ensure a single external endpoint for both HTTP and pub/sub traffic.

---

## Core Subsystems

### 1. Chat & Investigation Pipeline
The chat pipeline demonstrates the orchestrator role of `g8ed`:
1. **Ingress:** Browser sends a message to `POST /api/chat/send`.
2. **Context Enrichment:** `g8ed` resolves the user's bound operators and attaches a `G8eHttpContext` carrying the user identity and session state.
3. **Execution:** The request is relayed to `g8ee`.
4. **Streaming:** AI chunks and tool events are pushed from `g8ee` back to `g8ed` via internal APIs and then fanned out to the browser via the long-lived SSE stream.

### 2. Operator Slot Model
`g8ed` manages operators using a "Slot" abstraction to provide predictable resource management:
- **Available:** A slot is defined in the database but no physical operator is connected.
- **Active:** An operator has successfully registered and is providing regular heartbeats.
- **Bound:** A user has manually selected an active operator to perform tasks in their current session.
- **Stale/Offline:** Heartbeat timeout detection (monitored via `g8ee` signals and `OperatorService`).

### 3. Trust & Deployment Portal
To simplify local setup, `g8ed` serves a specialized portal on Port 80:
- **`GET /ca.crt`:** Direct download of the platform CA certificate.
- **`GET /trust`:** Platform-aware PowerShell or Bash scripts to automate certificate installation.
- **`GET /g8e`:** A unified deployment script that handles binary download and initial operator registration.

---

## Security Model

`g8ed` enforces several layers of isolation to protect the platform:

- **Identity Isolation:** All user actions are tied to a `web_session_id` stored in an `HttpOnly`, `Secure`, `SameSite=Lax` cookie.
- **Internal Origin Guard:** All cluster-internal routes (`/api/internal/*`) require a valid `X-Internal-Auth` token.
- **Context Preservation:** Every cross-component call includes a `G8eHttpContext`, ensuring that `g8ee` and `operator` can verify the originating user and session.
- **Stateless Persistence:** By delegating all state to the operator via `CacheAsideService`, `g8ed` can be restarted or scaled without data loss.

---

## Service Map

| Service | Responsibility |
|:---|:---|
| `UserService` | Manages user profiles, roles, and dev-log preferences. |
| `WebSessionService` | Manages browser-side sessions and encrypted storage for sensitive session keys. |
| `OperatorService` | The authoritative logic for operator lifecycle, heartbeats, and slot transitions. |
| `PubSubClient` | High-level wrapper for the operator's NATS-like pub/sub system. |
| `SSEService` | The platform-wide event distributor for real-time UI updates. |
| `InternalHttpClient` | Handles mutual auth and token injection for all component-to-component HTTP calls. |