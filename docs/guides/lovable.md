---
title: Lovable Frontend Integration
parent: Guides
---

# Lovable Frontend Integration

Last Updated: 2026-07-12
Version: v1.5.0

---

## Overview

[Lovable](https://lovable.dev) is an AI-powered frontend development platform that generates React + TypeScript + TailwindCSS applications from natural-language prompts. This guide provides detailed instructions for building a g8e Governance Console UI with Lovable, connecting it to a g8e Gateway backend via Cloudflare Tunnels.

The guide is structured as a prompt you can paste directly into your Lovable AI agent. It covers API integration, WebAuthn passkey authentication, cross-origin cookie handling, SSE live audit streaming, and the full component architecture. The prompt describes **what is required** for the integration to work. Let Lovable generate the implementation code.

### Steps at a Glance

1. **[Configure the Cloudflare Tunnel](./cloudflare_tunnel.md)** — Expose the local g8e Gateway to the internet via `console.g8e.ai`
2. **[Configure the Gateway](#gateway-side-configuration)** — Set CORS origins, passkey RP ID/name/origins, and public base URL
3. **[Enroll the Frontend](#frontend-enrollment)** — Run `g8e gui enroll` to validate CORS and generate a TypeScript config snippet
4. **[Review the API Reference](#api-reference)** — Understand the public and authenticated endpoints (or auto-discover via `/swagger/doc.json`)
5. **[Define TypeScript Types](#typescript-types)** — Mirror the g8e gateway's data models in the frontend
6. **[Build Pages and Components](#pages-and-components)** — AuthContext, Login, Dashboard, ApprovalFlow, URL hash handling
7. **[Define WebAuthn Flow Requirements](#webauthn-flow-requirements)** — Registration, authentication, and approval ceremonies
8. **[Define SSE Requirements](#sse-live-audit-stream)** — Live event streaming with auto-reconnect and polling fallback
9. **[Apply UI/UX Guidelines](#uiux-guidelines)** — Dark theme, monospace hashes, toast notifications, responsive layout
10. **[Handle Errors](#error-handling)** — 401 redirects, `needs_setup`, enrollment token errors, WebAuthn availability
11. **[Verify the Integration](#verification-checklist)** — CORS headers, preflight, passkey flows, cookie attributes, SSE, approvals

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

The gateway must be started with CORS and passkey RP settings that match your Lovable app's origin. The passkey RP ID **must** match the domain where WebAuthn ceremonies are performed — that's the Lovable app's origin, not the gateway's domain. The browser's WebAuthn API enforces that the RP ID is a registrable domain suffix of the current page's origin.

```bash
./g8e gw start \
  --public-base-url https://console.g8e.ai \
  --passkey-rp-id lovable.app \
  --passkey-rp-name "g8e Console" \
  --passkey-rp-origin https://your-app.lovable.app \
  --cors-origin https://your-app.lovable.app \
  --cors-origin https://your-custom-domain.com
```

Or via environment variables:

```bash
export G8E_PUBLIC_BASE_URL=https://console.g8e.ai
export G8E_PASSKEY_RP_ID=lovable.app
export G8E_PASSKEY_RP_NAME="g8e Console"
export G8E_PASSKEY_RP_ORIGINS=https://your-app.lovable.app
export G8E_ALLOWED_ORIGINS=https://your-app.lovable.app,https://your-custom-domain.com
```

Key flags:

- `--passkey-rp-id` — The WebAuthn Relying Party ID. Use the Lovable app's registrable domain (`lovable.app`) so passkeys work across Lovable subdomains. The browser rejects WebAuthn ceremonies if the RP ID doesn't match the current page's origin.
- `--passkey-rp-origin` — The origin where WebAuthn ceremonies are performed (the Lovable app URL). The gateway adds this to its allowed RP origins list.
- `--cors-origin` — The Lovable app origin. Allows cross-origin requests with credentials. Repeat for each origin (preview URLs, production URLs, custom domains).
- `--public-base-url` — The public URL of the gateway (the tunnel hostname). Used for approval redirect links and host validation.

> **Important:** Add every origin your Lovable app may use (preview URLs, production URLs, custom domains). The gateway reflects exact-match origins in CORS headers and sets `SameSite=None` on session cookies when `AllowedOrigins` is non-empty.

---

## 3. Frontend Enrollment

The `g8e gui` command family provides a complete frontend enrollment workflow that validates CORS configuration and generates a copy-pasteable TypeScript config snippet for the Lovable developer.

### Enroll the Lovable App Origin

```bash
g8e gui enroll --origin https://your-app.lovable.app --public-base-url https://console.g8e.ai
```

This command:

1. Validates the origin URL
2. Sends a CORS preflight to the running gateway to verify the origin is allowed
3. Verifies the gateway is reachable at the `--public-base-url` (if provided)
4. Persists the origin to the local enrollment file (`.g8e/gui_enrollments.json`)
5. Outputs a TypeScript configuration snippet with `API_BASE_URL`, `PASSKEY_RP_ID`, `apiFetch()` helper, `connectSSE()` helper, and key endpoint paths

Copy the outputted snippet into the Lovable project as the starting point for API integration.

### Other `g8e gui` Subcommands

- **`g8e gui show`** (alias: `list`) — Lists all enrolled origins and regenerates config snippets. Supports `--json` for scripting.
- **`g8e gui verify --origin <url>`** — Checks enrollment status and prints a verification checklist (CORS headers, passkey registration, session cookie, SSE stream, authenticated API calls).
- **`g8e gui remove --origin <url>`** — Removes an origin from the enrollment file.

---

<!-- ═══════════════════════════════════════════════════════════════ -->
<!-- BEGIN LOVABLE AI AGENT PROMPT                                    -->
<!-- ═══════════════════════════════════════════════════════════════ -->

## Lovable AI Agent Prompt

> **Paste everything below this line into your Lovable AI agent.**
> **Stop at the "Verification Checklist" section.**
>
> The content below describes **what is required** for the integration to work.
> Let Lovable generate the implementation code. Do not attempt to paste code from this document; paste the requirements and let Lovable build the app.

### Project Overview

Build a **g8e Governance Console** — a React + TypeScript + TailwindCSS single-page application that serves as the web UI for the g8e platform. The app communicates with the g8e gateway API at `https://console.g8e.ai` via cross-origin fetch requests with credentials.

### Critical Configuration

#### API Base URL

```typescript
const API_BASE_URL = 'https://console.g8e.ai';
```

#### Fetch Requirements

**Every** `fetch` call to the API MUST include `credentials: 'include'`. This is non-negotiable — the g8e gateway uses a `g8e_web_session_cookie` (HttpOnly, Secure, SameSite=None) for browser authentication. Without `credentials: 'include'`, the cookie is not sent cross-origin and all authenticated requests will return 401.

#### WebAuthn Library

Install `@simplewebauthn/browser`:

```bash
npm install @simplewebauthn/browser
```

#### Dark Theme

Use a dark theme with these CSS custom properties as design tokens:

```css
--bg: #0d1117;
--surface: #161b22;
--border: #30363d;
--text: #c9d1d9;
--muted: #8b949e;
--accent: #58a6ff;
--success: #3fb950;
--warning: #d29922;
--danger: #f85149;
```

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
| `GET` | `/api/v1/sse/stream?web_session_id={id}` | SSE stream for live audit events |
| `GET` | `/api/v1/sse/events?web_session_id={id}&since_id={n}` | Poll SSE events |

---

## 5. TypeScript Types

```typescript
// User
interface User {
  id: string;
  passkey_credentials?: PasskeyCredential[];
  provider?: string;
  organization_id?: string;
  roles?: string[];
  status?: string;
  is_bootstrap?: boolean;
  local_os_user?: { domain?: string; username?: string; uid?: string };
  webauthn_user_id?: string;
}

interface UserMeResponse {
  success: boolean;
  user: User;
}

// Passkey
interface PasskeyCredential {
  id: string; // base64-encoded byte array — convert to base64url for display
  public_key: string; // base64-encoded COSE key
  attestation_type: string;
  transport?: string[];
  authenticator: {
    aaguid: string; // base64-encoded byte array
    sign_count: number;
    clone_warning: boolean;
  };
  created_at_unix_ms: number;
  last_used_at_unix_ms?: number;
}

interface PasskeyCredentialsResponse {
  success: boolean;
  credentials: PasskeyCredential[];
}

// Session
interface WebSessionResponse {
  success: boolean;
  user_id: string;
  web_session_id: string;
}

// Health
interface HealthResponse {
  status: string;
  mode: string;
  version: string;
  pid: number;
  governance_ready: boolean;
  state_merkle_root?: string;
}

// Bootstrap status
interface BootstrapStatusResponse {
  bootstrapped: boolean;
}

// Approvals (suspended transactions)
interface SuspendedTxResponse {
  transaction_hash: string;
  created_at: string;
  expires_at: string;
  tool_name: string;
  user_id: string;
  operator_id: string;
}

interface SuspendedTransactionsResponse {
  transactions: SuspendedTxResponse[];
}

// WebAuthn responses (from browser navigator.credentials)
interface WebAuthnAttestationResponse {
  id: string;
  rawId: string;
  clientDataJSON: string;
  attestationObject: string;
  transports?: string[];
}

interface WebAuthnAssertionResponse {
  id: string;
  rawId: string;
  clientDataJSON: string;
  authenticatorData: string;
  signature: string;
  userHandle?: string;
}

// SSE
interface SSEEventRow {
  id: number;
  web_session_id?: string;
  cli_session_id?: string;
  user_id?: string;
  event_type: string;
  payload: string;
  created_at: string;
}

interface SSEEventsResponse {
  events: SSEEventRow[];
  count: number;
}
```

---

## 6. Pages and Components

### 6.1 Auth Context (`src/contexts/AuthContext.tsx`)

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

### 6.2 Login Page (`src/pages/Login.tsx`)

Two modes based on `bootstrapped` state:

**If NOT bootstrapped (first-time setup):**

- Show a "Register Passkey" card with a display name input
- Button: "Enroll Passkey" → calls `registerPasskey()`

**If bootstrapped (returning user):**

- Show a "Sign In" card with a User ID input
- Button: "Sign In with Passkey" → calls `authenticatePasskey()`
- Secondary button: "Register New Passkey" → reveals registration form

### 6.3 Dashboard (`src/pages/Dashboard.tsx`)

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

### 6.4 Approval Flow (`src/components/ApprovalFlow.tsx`)

When user clicks "Approve" on a transaction:

1. `GET /api/v1/approvals/{txHash}/challenge` — returns WebAuthn PublicKeyCredentialRequestOptions
2. Decode the challenge and allowedCredentials from base64url to ArrayBuffers
3. Call `navigator.credentials.get({ publicKey: decodedOptions })`
4. Encode the assertion response fields to base64url
5. `POST /api/v1/approvals/{txHash}/verify` with the encoded assertion
6. Show success or error message
7. Refresh the approvals list

### 6.5 URL Hash Handling

On app load, check `window.location.hash` for:

- `#approve={txHash}` — if user is logged in, auto-trigger the approval flow for this transaction. If not logged in, store it and trigger after login.
- `#register=1&token={enrollmentToken}` — validate the enrollment token via `POST /api/v1/auth/enrollment-token/validate`, then auto-trigger passkey registration with the returned user ID and CLI session ID.

After processing, clean the URL with `history.replaceState`.

#### L3 Approval Redirect

When a destructive mutation is suspended by the gateway's governance gauntlet, the gateway returns an approval URL like `{publicBaseURL}/approve/{txHash}`. This URL redirects to the gateway's embedded console at `/console/#approve={txHash}`.

For the Lovable app, you have two options:

1. **Handle approvals inline** (recommended): The Lovable app implements the approval flow directly (see [Approval Flow](#64-approval-flow-srccomponentsapprovalflowtsx)). When a mutation is suspended, the API response includes the transaction hash. The Lovable app can auto-trigger the approval flow using the `#approve={txHash}` URL hash pattern.

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

- Connect to `GET /api/v1/sse/stream?web_session_id={id}` using `EventSource` with `withCredentials: true`.
- The `web_session_id` comes from `GET /api/v1/auth/sessions/me` after login.
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

If `EventSource` does not send cookies cross-origin, fall back to polling `GET /api/v1/sse/events?web_session_id={id}&since_id={lastId}` every 2 seconds.

### WebSocket Note

The gateway also exposes a WebSocket pub/sub endpoint at `/ws/v1/pubsub`, but it requires mTLS authentication and is not available to browser clients. Use SSE for all browser-based real-time telemetry.

---

## 9. UI/UX Guidelines

- **Dark theme** using the CSS variables above
- **Monospace font** for all hashes, credential IDs, and technical identifiers
- **Truncate long hashes** to first 24 characters with `...` suffix
- **Color-coded event type badges**: blue for events, red for errors, yellow for warnings, green for info/success
- **Confirmation dialogs** before destructive actions (revoke passkey)
- **Toast notifications** for success/error feedback on all async operations
- **Loading states** on all buttons during async operations
- **Responsive layout** — max-width 720px centered on desktop, full-width on mobile
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

## JWT Identity Provider Integration (Optional)

If you use an external identity provider (IdP) such as Clerk, Auth0, or Supabase for JWT-based authentication, configure the gateway to validate JWTs from the IdP's JWKS endpoint:

```bash
./g8e gw start \
  --jwks-url https://your-idp.example.com/.well-known/jwks.json \
  --jwt-role-claim roles \
  --jwt-issuer https://your-idp.example.com \
  --jwt-audience g8e-gateway
```

When JWKS is configured, the gateway automatically accepts JWT bearer tokens for MCP and A2A endpoints instead of requiring mTLS. The gateway maps JWT role claims to g8e personas (admin, operator, observer, agent) via the role-to-persona mapping configuration. JIT (just-in-time) provisioning creates a g8e user record on first successful JWT authentication.

| IdP | JWKS URL | Role Claim | Notes |
|---|---|---|---|
| Clerk | `https://<domain>.clerk.accounts.dev/.well-known/jwks.json` | `roles` | Custom claim setup required in Clerk dashboard |
| Auth0 | `https://<domain>.auth0.com/.well-known/jwks.json` | `permissions` or custom namespace | Use `--jwt-role-claim permissions` or `--jwt-role-claim https://g8e.ai/roles` |
| Supabase | `https://<project>.supabase.co/auth/v1/.well-known/jwks.json` | `user_metadata.roles` | Requires custom JWT claims via Supabase Edge Function |


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

- [Cloudflare Tunnel Integration](./cloudflare_tunnel.md) — Setting up the tunnel between Cloudflare and the g8e Gateway
- [Connect Apps to Gateway](./connect_apps_to_gateway.md) — General application connectivity patterns
- [Architecture: Auth](../architecture/auth.md) — WebAuthn passkey authentication architecture
- [Architecture: Gateway](../architecture/gateway.md) — Gateway service architecture
- [Protocol Library](../architecture/protocol.md) — Go module and Python package API reference for building g8e-compatible clients and services
