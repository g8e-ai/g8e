---
title: Lovable Frontend Integration
parent: Guides
---

# Lovable Frontend Integration

Last Updated: 2026-08-14
Version: v1.7.2

---

## Overview

[Lovable](https://lovable.dev) is an AI-powered frontend development platform that generates React + TypeScript + TailwindCSS applications from natural-language prompts. This guide provides detailed instructions for building a g8e Governance Console UI with Lovable, connecting it to a g8e Gateway backend via Cloudflare Tunnels.

This guide covers Lovable-specific configuration, the AI agent prompt, and component architecture. For the comprehensive guide on building a g8e-compatible frontend (enrollment commands, API reference, WebAuthn flow requirements, SSE streaming, API data types, UI/UX guidelines), see [Build a g8e-Compatible Frontend](./build_frontend.md).

### Steps at a Glance

1. **[Configure the Cloudflare Tunnel](./cloudflare_tunnel.md)** - Expose the local g8e Gateway to the internet via `console.g8e.ai`
2. **[Configure the Gateway](./build_frontend.md#gateway-side-configuration)** - Set CORS origins, passkey RP ID/name/origins (see [Build a g8e-Compatible Frontend](./build_frontend.md))
3. **[Enroll the Frontend](./build_frontend.md#frontend-enrollment-commands)** - Run `g8e gui enroll` to validate CORS and generate a TypeScript config snippet
4. **[Review the API Reference](./build_frontend.md#api-reference)** - Understand the public and authenticated endpoints (see [Build a g8e-Compatible Frontend](./build_frontend.md))
5. **[Review API Data Types](./build_frontend.md#api-data-types)** - Use the Swagger spec for request and response schemas (see [Build a g8e-Compatible Frontend](./build_frontend.md))
6. **[Build Pages and Components](#6-pages-and-components)** - AuthContext, Login, Dashboard, ApprovalFlow, URL hash handling
7. **[Define WebAuthn Flow Requirements](./build_frontend.md#webauthn-flow-requirements)** - Registration, authentication, and approval ceremonies (see [Build a g8e-Compatible Frontend](./build_frontend.md))
8. **[Define SSE Requirements](./build_frontend.md#sse-live-audit-stream)** - Live event streaming with auto-reconnect and polling fallback (see [Build a g8e-Compatible Frontend](./build_frontend.md))
9. **[Apply UI/UX Guidelines](#9-uiux-guidelines)** - Dark theme, monospace hashes, toast notifications, responsive layout
10. **[Handle Errors](#10-error-handling)** - 401 redirects, `needs_setup`, enrollment token errors, WebAuthn availability
11. **[Verify the Integration](#11-verification-checklist)** - CORS headers, preflight, passkey flows, cookie attributes, SSE, approvals

### Architecture

```
[Lovable App (React)] → https://console.g8e.ai → [Cloudflare Edge] → [cloudflared tunnel] → [g8e Gateway :8443]
         ↑                                                                                      ↓
    credentials: 'include'                                                          WebAuthn + Session Cookie + SSE
```

## 1. Cloudflare Tunnel

A Cloudflare Tunnel securely exposes the local g8e Gateway to the internet so the Lovable frontend can reach it from the browser. The tunnel routes `console.g8e.ai` through Cloudflare's edge to the gateway's HTTPS listener on `localhost:8443`, with HTTP/2 enabled for SSE streaming performance.

**Full setup instructions** (tunnel creation, `config.yml`, systemd service, Cloudflare Access policies, and troubleshooting) are in the [Cloudflare Tunnel Integration guide](./cloudflare_tunnel.md).

### Prerequisites

- g8e Gateway running behind a Cloudflare Tunnel at `https://console.g8e.ai` (see [Cloudflare Tunnel Integration](./cloudflare_tunnel.md))
- A [Lovable](https://lovable.dev) account

---

## 2. Gateway-Side Configuration

For general gateway-side configuration (CORS, passkey RP flags, environment variables), see [Build a g8e-Compatible Frontend: Gateway-Side Configuration](./build_frontend.md#gateway-side-configuration).

### Lovable-Specific Configuration

For a Lovable app hosted at `https://your-app.lovable.app` with a Cloudflare Tunnel at `https://console.g8e.ai`:

```bash
./g8e gw start \
  --public-base-url https://console.g8e.ai \
  --passkey-rp-id lovable.app \
  --passkey-rp-name "g8e Console" \
  --passkey-rp-origin https://your-app.lovable.app \
  --cors-origin https://your-app.lovable.app \
  --cors-origin https://your-custom-domain.com
```

Use `lovable.app` as the `--passkey-rp-id` so passkeys work across Lovable subdomains. Add every origin the Lovable app may use (preview URLs, production URLs, custom domains).

---

## 3. Frontend Enrollment

For the full `g8e gui` command reference (enroll, show, verify, remove), see [Build a g8e-Compatible Frontend: Frontend Enrollment Commands](./build_frontend.md#frontend-enrollment-commands).

### Enroll the Lovable App Origin

```bash
g8e gui enroll --origin https://your-app.lovable.app --public-base-url https://console.g8e.ai
```

Copy the outputted TypeScript config snippet into the Lovable project as the starting point for API integration.

---

## 11. Verification Checklist

After the Lovable AI agent generates the app, verify:

- [ ] **CORS headers**: Open browser DevTools > Network tab. Make any API call and confirm `Access-Control-Allow-Origin` reflects your Lovable app's origin and `Access-Control-Allow-Credentials: true` is present.
- [ ] **Preflight OPTIONS**: Confirm OPTIONS requests succeed with 200/204 status before actual POST requests.
- [ ] **Passkey registration**: Click "Enroll Passkey" and confirm the browser's WebAuthn dialog appears with RP ID `lovable.app` (or your configured `--passkey-rp-id`).
- [ ] **Passkey authentication**: Click "Sign In with Passkey" and confirm the WebAuthn dialog appears and login succeeds.
- [ ] **Session cookie**: After login, check DevTools > Application > Cookies for `g8e_web_session_cookie` with `SameSite=None` and `Secure` attributes.
- [ ] **Authenticated API calls**: Confirm `GET /api/v1/users/me` returns user data (not 401) after login.
- [ ] **SSE stream**: Click "Connect" on the audit stream card and confirm live events appear.
- [ ] **Approvals**: If a suspended transaction exists, confirm the "Approve" button triggers WebAuthn and the transaction is approved.
- [ ] **Logout**: Click "Sign Out" and confirm redirect to login page and cookie cleared.
- [ ] **URL hash handling**: Navigate to `#approve={txHash}` and confirm auto-approval flow triggers.
- [ ] **`g8e gui verify`**: Run `g8e gui verify --origin https://your-app.lovable.app` and confirm all checklist items pass.

---

## See Also

- [Build a g8e-Compatible Frontend](./build_frontend.md) - Comprehensive guide for building a g8e-compatible UI (enrollment commands, API reference, WebAuthn flows, SSE, API data types, UI/UX guidelines)
- [Cloudflare Tunnel Integration](./cloudflare_tunnel.md) - Setting up the tunnel between Cloudflare and the g8e Gateway
- [Connect Apps to Gateway](./connect_apps_to_gateway.md) - General application connectivity patterns
- [Architecture: Auth](../architecture/auth.md) - WebAuthn passkey authentication architecture
- [Architecture: Gateway](../architecture/gateway.md) - Gateway service architecture
- [Protocol Library](../architecture/protocol.md) - Go module and Python package API reference for building g8e-compatible clients and services

---

<!-- ═══════════════════════════════════════════════════════════════ -->
<!-- BEGIN LOVABLE AI AGENT PROMPT                                    -->
<!-- ═══════════════════════════════════════════════════════════════ -->

## Lovable AI Agent Prompt

> **Paste everything below this line into your Lovable AI agent.**
>
> The content below describes **what is required** for the integration to work.
> Let Lovable generate the implementation code. Do not attempt to paste code from this document; paste the requirements and let Lovable build the app.

### Project Overview

Build a **g8e Governance Console** - a React + TypeScript + TailwindCSS single-page application that serves as the web UI for the g8e platform. The app communicates with the g8e gateway API at `https://console.g8e.ai` via cross-origin fetch requests with credentials.

### Critical Configuration

#### API Base URL

Set `API_BASE_URL` to `https://console.g8e.ai` (or your tunnel hostname).

#### Fetch Requirements

**Every** `fetch` call to the API MUST include `credentials: 'include'`. This is non-negotiable: the g8e gateway uses a `g8e_web_session_cookie` (HttpOnly, Secure, SameSite=None) for browser authentication. Without `credentials: 'include'`, the cookie is not sent cross-origin and all authenticated requests will return 401.

#### WebAuthn Library

Install `@simplewebauthn/browser` via npm. This library handles base64url conversions automatically.

#### Dark Theme

Use a dark theme with CSS custom properties as design tokens: `--bg`, `--surface`, `--border`, `--text`, `--muted`, `--accent`, `--success`, `--warning`, `--danger`. Use GitHub-inspired dark palette values (e.g., `#0d1117` for background, `#161b22` for surface, `#30363d` for border, `#c9d1d9` for text, `#8b949e` for muted, `#58a6ff` for accent, `#3fb950` for success, `#d29922` for warning, `#f85149` for danger).

---

## 4. API Reference

All paths are relative to `API_BASE_URL`. All authenticated routes require `credentials: 'include'`.

> **Tip:** The gateway serves a full OpenAPI/Swagger specification at `/swagger/doc.json` (and browsable UI at `/swagger/`). Lovable's AI can ingest this spec to auto-discover the API surface and generate correct integration code. Point the Lovable agent to `https://console.g8e.ai/swagger/doc.json` for the complete endpoint catalog.

### Public Routes (no auth required)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/health` | Gateway health check |
| `GET` | `/api/v1/auth/bootstrap/status` | Check if any passkey is registered |
| `POST` | `/api/v1/auth/passkeys/console/register/challenge` | Get WebAuthn registration challenge |
| `POST` | `/api/v1/auth/passkeys/console/register/verify` | Verify registration attestation |
| `POST` | `/api/v1/auth/passkeys/console/authenticate/challenge` | Get WebAuthn authentication challenge |
| `POST` | `/api/v1/auth/passkeys/console/authenticate/verify` | Verify authentication assertion |
| `POST` | `/api/v1/auth/logout` | Sign out (clears session cookie) |
| `POST` | `/api/v1/auth/enrollment-token/validate` | Validate enrollment token (for registration links) |

### Authenticated Routes (require session cookie)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/users/me` | Get current authenticated user |
| `GET` | `/api/v1/auth/sessions/me` | Get current web session info |
| `GET` | `/api/v1/auth/passkeys` | List user's passkey credentials (user derived from session) |
| `DELETE` | `/api/v1/auth/passkeys/{credentialId}` | Revoke a passkey (user derived from session) |
| `GET` | `/api/v1/approvals` | List pending suspended transactions |
| `GET` | `/api/v1/approvals/{txHash}/challenge` | Get WebAuthn approval challenge |
| `POST` | `/api/v1/approvals/{txHash}/verify` | Verify approval assertion |
| `GET` | `/api/v1/sse/stream` | SSE stream for live audit events (web_session_id from cookie) |
| `GET` | `/api/v1/sse/events?since_id={n}` | Poll SSE events (web_session_id from cookie) |

### Route Authentication

Browser clients use public routes (no auth) and authenticated routes (session cookie). The SSE endpoints accept either mTLS or a session cookie. All other gateway routes require mTLS and are not accessible from browsers.

---

## 5. API Data Types

The gateway serves a full OpenAPI/Swagger specification at `/swagger/doc.json` (and browsable UI at `/swagger/`). Use this to discover request and response schemas for all endpoints, including user, passkey, session, health, bootstrap, approval, and SSE event types. The `@simplewebauthn/browser` library provides TypeScript types for WebAuthn ceremony inputs and outputs.

---

## 6. Pages and Components

### 6.1 Auth Context

Manage global auth state:

- `user: User | null`
- `loading: boolean`
- `bootstrapped: boolean` (whether any passkey exists on the gateway)
- `webSessionId: string | null`
- `login()`, `logout()`, `registerPasskey()`, `refreshUser()` methods

On mount:

1. Call `GET /api/v1/auth/bootstrap/status` to check if any passkey is registered
2. Call `GET /api/v1/users/me` (with `credentials: 'include'`) to check if already logged in
3. If logged in, call `GET /api/v1/auth/sessions/me` to get the web session ID (needed for SSE)

### 6.2 Login Page

Two modes based on `bootstrapped` state:

**If NOT bootstrapped (first-time setup):**

- Show a "Register Passkey" card with a display name input
- Button: "Enroll Passkey" → calls `registerPasskey()`

**If bootstrapped (returning user):**

- Show a "Sign In" card with a User ID input
- Button: "Sign In with Passkey" → calls `authenticatePasskey()`
- Secondary button: "Register New Passkey" → reveals registration form

### 6.3 Dashboard

After authentication, show:

**Stats Row** (3 stat cards):

- Number of registered passkeys
- Number of pending approvals
- Gateway version (from health endpoint)

**Passkeys Card:**

- List all passkeys with credential ID (truncated, monospace), creation date, last used date
- "Revoke" button per passkey (with confirmation dialog)
- "Register New Passkey" button (expands inline form)

**Pending Approvals Card:**

- List all suspended transactions showing: tool name, transaction hash (truncated, monospace), created date, expiry countdown
- "Approve" button per transaction → triggers WebAuthn approval flow
- Empty state: "No pending approvals"

**Live Audit Stream Card:**

- Connect/Disconnect button for SSE stream
- Status indicator (green=connected, yellow=connecting, red=disconnected)
- Scrollable log area showing events as they arrive
- Each event shows: event type badge (color-coded), timestamp, event ID, expandable JSON payload
- Filter input (filters by event type, case-insensitive)
- Auto-scroll checkbox
- Clear button
- Event count display
- Auto-reconnect on disconnect (3 second delay)

**Account Card:**

- Display user ID (monospace)
- "Sign Out" button

### 6.4 Approval Flow

When user clicks "Approve" on a transaction:

1. `GET /api/v1/approvals/{txHash}/challenge` - returns WebAuthn PublicKeyCredentialRequestOptions
2. Decode the challenge and allowedCredentials from base64url to ArrayBuffers
3. Call `navigator.credentials.get({ publicKey: decodedOptions })`
4. Encode the assertion response fields to base64url
5. `POST /api/v1/approvals/{txHash}/verify` with the encoded assertion
6. Show success or error message
7. Refresh the approvals list

### 6.5 URL Hash Handling

On app load, check `window.location.hash` for:

- `#approve={txHash}` - if user is logged in, auto-trigger the approval flow for this transaction. If not logged in, store it and trigger after login.
- `#enroll=1&token={enrollmentToken}` - post the token to the enrollment register endpoints (`/api/v1/auth/passkeys/enrollment/register/challenge` then `/verify`), which validate the token and auto-trigger passkey registration. The gateway derives the user ID and CLI session ID from the token.

After processing, clean the URL with `history.replaceState`.

#### L3 Approval Redirect

When a destructive mutation is suspended by the gateway's governance gauntlet, the gateway returns an approval URL like `{publicBaseURL}/approve/{txHash}`. This URL redirects to the gateway's embedded console at `/console/#approve={txHash}`.

For the Lovable app, you have two options:

1. **Handle approvals inline** (recommended): The Lovable app implements the approval flow directly (see [Approval Flow](#64-approval-flow)). When a mutation is suspended, the API response includes the transaction hash. The Lovable app can auto-trigger the approval flow using the `#approve={txHash}` URL hash pattern.

2. **Redirect to the gateway console**: If the Lovable app does not implement the approval flow, redirect the user's browser to `{API_BASE_URL}/approve/{txHash}`. The gateway console at `/console/` handles the WebAuthn approval ceremony. Note: this requires the passkey RP ID to match the gateway's domain. If the RP ID is set to the Lovable app's domain (recommended), the console redirect will not be able to perform WebAuthn. Use option 1 instead.

---

## 7. WebAuthn Flow Requirements

The app must implement three WebAuthn ceremonies using the browser's `navigator.credentials` API. The `@simplewebauthn/browser` library is recommended as it handles base64url conversions automatically.

### 7.1 Base64url Encoding

The gateway sends and receives WebAuthn challenge data as base64url strings. The browser's WebAuthn API requires ArrayBuffers. The app must convert between base64url strings and ArrayBuffers for:

- Challenge fields
- User ID fields (registration only)
- Credential ID fields (`excludeCredentials`, `allowCredentials`)
- Assertion response fields (`rawId`, `clientDataJSON`, `authenticatorData`, `signature`, `userHandle`, `attestationObject`)

If using `@simplewebauthn/browser`, these conversions are handled automatically.

### 7.2 Registration Flow (First-Time Setup)

1. POST to `/api/v1/auth/passkeys/console/register/challenge` with JSON body `{ user_id, user_name, cli_session_id: "browser" }`. If no passkeys exist on the gateway, omit `user_id` to auto-create a user.
2. Decode the returned `options.publicKey` challenge and user ID from base64url to ArrayBuffers.
3. Call `navigator.credentials.create({ publicKey: decodedOptions })` to trigger the browser's passkey enrollment prompt.
4. Encode the credential response fields (`id`, `rawId`, `clientDataJSON`, `attestationObject`, `transports`) as base64url.
5. POST to `/api/v1/auth/passkeys/console/register/verify` with JSON body `{ user_id, cli_session_id: "browser", attestation_response: { ...encodedFields } }`.
6. On success, the gateway sets a `g8e_web_session_cookie` session cookie. The app transitions to the dashboard.

### 7.3 Authentication Flow (Returning User)

1. POST to `/api/v1/auth/passkeys/console/authenticate/challenge` with JSON body `{ user_id }`.
2. If the response has `success: false` and `needs_setup: true`, no passkey is registered. Show the registration form instead.
3. Decode the returned `options.publicKey` challenge and `allowCredentials` from base64url to ArrayBuffers.
4. Call `navigator.credentials.get({ publicKey: decodedOptions })` to trigger the browser's passkey authentication prompt.
5. Encode the assertion response fields (`id`, `rawId`, `clientDataJSON`, `authenticatorData`, `signature`, `userHandle`) as base64url.
6. POST to `/api/v1/auth/passkeys/console/authenticate/verify` with JSON body `{ user_id, assertion_response: { ...encodedFields } }`.
7. On success, the gateway sets a `g8e_web_session_cookie` session cookie. The app transitions to the dashboard.

### 7.4 Approval Flow (Suspended Transaction)

1. GET `/api/v1/approvals/{txHash}/challenge` (with `credentials: 'include'`). Returns WebAuthn `PublicKeyCredentialRequestOptions`.
2. Decode the challenge and `allowCredentials` from base64url to ArrayBuffers.
3. Call `navigator.credentials.get({ publicKey: decodedOptions })` to trigger the browser's passkey prompt.
4. Encode the assertion response fields (`id`, `rawId`, `clientDataJSON`, `authenticatorData`, `signature`, `userHandle`) as base64url.
5. POST to `/api/v1/approvals/{txHash}/verify` with the encoded assertion response as the JSON body.
6. On success, the transaction resumes execution. Refresh the approvals list.
7. On failure (403), show the error message. The transaction remains suspended.

---

## 8. SSE Live Audit Stream

The app must provide a live audit event stream with the following requirements:

### Connection

- Connect to `GET /api/v1/sse/stream` using `EventSource` with `withCredentials: true`.
- The `web_session_id` is derived from the authenticated session cookie by the gateway; do NOT pass it in the URL.
- Show connection status: green (connected), yellow (connecting), red (disconnected).
- Provide manual connect and disconnect buttons.

### Event Handling

- Parse incoming SSE messages as JSON. Each message may contain a nested `event` field that itself is JSON.
- Extract: event type, timestamp, event ID, and raw payload.
- Display events in a scrollable log with color-coded type badges (blue for events, red for errors, yellow for warnings, green for info/success).
- Show event ID, timestamp, and expandable JSON payload per entry.
- Cap the in-memory event list at 500 entries (drop oldest).
- Provide a filter input (case-insensitive filter by event type).
- Provide auto-scroll checkbox, clear button, and event count display.

### Reconnection

- Auto-reconnect 3 seconds after disconnection.

### Polling Fallback

If `EventSource` does not send cookies cross-origin, fall back to polling `GET /api/v1/sse/events?since_id={lastId}` every 2 seconds (the `web_session_id` is derived from the session cookie).

### WebSocket Note

The gateway also exposes a WebSocket pub/sub endpoint at `/ws/pubsub`, but it requires mTLS authentication and is not available to browser clients. Use SSE for all browser-based real-time telemetry.

---

## 9. UI/UX Guidelines

- **Dark theme** using the CSS variables above
- **Monospace font** for all hashes, credential IDs, and technical identifiers
- **Truncate long hashes** to first 24 characters with `...` suffix
- **Color-coded event type badges**: blue for events, red for errors, yellow for warnings, green for info/success
- **Confirmation dialogs** before destructive actions (revoke passkey)
- **Toast notifications** for success/error feedback on all async operations
- **Loading states** on all buttons during async operations
- **Responsive layout** - max-width 720px centered on desktop, full-width on mobile
- **Header bar**: "g8e Console" title (accent color) + current user display name on the right
- **Footer**: "g8e Gateway (c) 2026 Lateralus Labs, LLC."

---

## 10. Error Handling

- **401 responses**: Clear user state and redirect to login
- **`needs_setup: true`** on authenticate challenge: Show registration form automatically
- **Enrollment token validation**: Handle 410 (expired) and 409 (already used) with specific error messages
- **WebAuthn API not available**: Show a browser compatibility warning
- **Network errors**: Show a retry-able error state

---

## Recommended File Structure

```
src/
  contexts/
    AuthContext.tsx
  pages/
    Login.tsx
    Dashboard.tsx
  components/
    StatsRow.tsx
    PasskeysCard.tsx
    ApprovalsCard.tsx
    AuditStreamCard.tsx
    AccountCard.tsx
    ApprovalDialog.tsx
    RegisterPasskeyForm.tsx
    Toast.tsx
  hooks/
    useAuditStream.ts
    useApi.ts
  lib/
    api.ts          // fetch wrapper with credentials: 'include'
    webauthn.ts     // base64url helpers + registration/auth/approval flows
    types.ts        // TypeScript interfaces
  App.tsx
  main.tsx
```

<!-- ═══════════════════════════════════════════════════════════════ -->
<!-- END LOVABLE AI AGENT PROMPT                                      -->
<!-- ═══════════════════════════════════════════════════════════════ -->
