---
title: Build a g8e-Compatible Frontend
parent: Guides
---

# Build a g8e-Compatible Frontend

Last Updated: 2026-08-07
Version: v1.6.10

---

## Overview

This guide describes how to build a g8e-compatible web UI. It covers gateway configuration, frontend enrollment, WebAuthn authentication, SSE streaming, approval flows, API data types, UI/UX guidelines, and the recommended project structure. It applies to custom React apps, Vue dashboards, vanilla JS consoles, and hosted platforms like Lovable.

The `g8e gui` command family enrolls external frontend applications with the g8e Gateway. Enrollment validates that the gateway is running with the correct CORS and passkey RP configuration for the frontend origin, persists the origin to a local enrollment file, and outputs a TypeScript configuration snippet for the frontend developer.

For Lovable-specific integration (AI agent prompt, Cloudflare Tunnel setup), see [Lovable Frontend Integration](./lovable.md).

### Architecture

```
[Frontend App] → https://gateway-host:8443 → [g8e Gateway]
      ↑                                          ↓
 credentials: 'include'              WebAuthn + Session Cookie + SSE
```

The frontend is a browser-based SPA that communicates with the g8e Gateway over HTTPS. Authentication is via WebAuthn passkeys (no passwords, no API keys). The gateway issues an HttpOnly session cookie after successful passkey verification. All authenticated API calls must include `credentials: 'include'` so the cookie is sent cross-origin. Real-time telemetry is delivered via Server-Sent Events (SSE), not WebSockets (the WebSocket endpoint requires mTLS and is not available to browsers).

### The Built-In Console as Reference

The g8e Gateway ships with an embedded console SPA at `/console/`. This is a single-file vanilla JS app that implements all g8e-compatible frontend requirements: passkey registration, passkey authentication, approval flows, SSE audit streaming, passkey management, and URL hash handling for enrollment tokens and approval redirects. It serves as the canonical reference implementation. Review it alongside this guide when building your own frontend.

---

## Prerequisites

- g8e Gateway running and healthy
- Frontend application served from a known origin (e.g., `https://your-app.example.com`, `http://localhost:3003`)
- Gateway started with `--cors-origin` and `--passkey-rp-origin` flags matching the frontend origin
- Browser supports WebAuthn (all modern Chrome, Firefox, Safari, Edge)

---

## Gateway-Side Configuration

The gateway must be started with CORS and passkey RP settings that match the frontend app's origin. The passkey RP ID must match the domain where WebAuthn ceremonies are performed, which is the frontend app's origin, not the gateway's domain. The browser's WebAuthn API enforces that the RP ID is a registrable domain suffix of the current page's origin.

```bash
./g8e gw start \
  --passkey-rp-id example.com \
  --passkey-rp-name "g8e Console" \
  --passkey-rp-origin https://your-app.example.com \
  --cors-origin https://your-app.example.com
```

Or via environment variables:

```bash
export G8E_PASSKEY_RP_ID=example.com
export G8E_PASSKEY_RP_NAME="g8e Console"
export G8E_PASSKEY_RP_ORIGINS=https://your-app.example.com
export G8E_ALLOWED_ORIGINS=https://your-app.example.com
```

Key flags:

- `--passkey-rp-id` - The WebAuthn Relying Party ID. Use the frontend app's registrable domain so passkeys work across subdomains. The browser rejects WebAuthn ceremonies if the RP ID does not match the current page's origin.
- `--passkey-rp-origin` - The origin where WebAuthn ceremonies are performed (the frontend app URL). The gateway adds this to its allowed RP origins list. Repeat for each origin.
- `--cors-origin` - The frontend app origin. Allows cross-origin requests with credentials. Repeat for each origin (preview URLs, production URLs, custom domains).
- `--public-base-url` - The public URL of the gateway (e.g., a tunnel hostname). Used for approval redirect links and host validation.

Add every origin the frontend app may use (preview URLs, production URLs, custom domains). The gateway reflects exact-match origins in CORS headers and sets `SameSite=None` on session cookies when `AllowedOrigins` is non-empty.

### How CORS Works

When `AllowedOrigins` is non-empty, the gateway's CORS middleware:

1. Checks the request `Origin` header against the allowed set (case-insensitive, trailing slashes trimmed).
2. If matched, sets `Access-Control-Allow-Origin` to the exact origin and `Access-Control-Allow-Credentials: true`.
3. Handles `OPTIONS` preflight requests with `204 No Content` and the appropriate `Access-Control-Allow-Methods`, `Access-Control-Allow-Headers`, and `Access-Control-Max-Age` headers.
4. Adds `Vary: Origin` to all responses so caches respect per-origin differences.
5. When `AllowedOrigins` is empty (same-origin only), the middleware is a pass-through and no CORS headers are set.

### Session Cookie Behavior

The gateway sets a `g8e_web_session_cookie` cookie after successful passkey registration or authentication:

- **`HttpOnly`**: Always true (prevents JavaScript access).
- **`Secure`**: Always true (requires HTTPS).
- **`SameSite=Lax`**: Default when no cross-origin origins are configured.
- **`SameSite=None`**: Automatically set when `AllowedOrigins` is non-empty (required for cross-origin cookie delivery).

The cookie has a 24-hour TTL. The gateway validates the cookie on every authenticated request by looking up the session ID in its database and checking expiry.

---

## Frontend Enrollment Commands

### Enroll a Frontend Origin

```bash
g8e gui enroll --origin https://your-app.example.com
```

Optional flags:

- `--passkey-rp-id` - Override the RP ID (defaults to the origin's hostname)
- `--passkey-rp-name` - Override the RP display name (default: `g8e`)
- `--public-base-url` - Verify the gateway is reachable at a public URL (e.g., tunnel hostname)

This command:

1. Validates the origin URL
2. Sends a CORS preflight to the running gateway to verify the origin is allowed
3. Verifies the gateway is reachable at the `--public-base-url` (if provided)
4. Persists the origin to the local enrollment file (`.g8e/gui_enrollments.json`)
5. Outputs a TypeScript configuration snippet with `API_BASE_URL`, `PASSKEY_RP_ID`, `PASSKEY_RP_NAME`, `apiFetch()` helper, `connectSSE()` helper, and key endpoint paths

Copy the outputted snippet into the frontend project as the starting point for API integration.

### Show Enrolled Origins

```bash
g8e gui show
```

Alias: `g8e gui list`. Lists all enrolled origins and regenerates config snippets. Supports `--json` for scripting:

```bash
g8e gui show --json
```

### Verify Enrollment

```bash
g8e gui verify --origin https://your-app.example.com
```

Checks enrollment status and prints a verification checklist covering CORS headers, passkey registration, session cookie attributes, SSE stream connectivity, and authenticated API calls.

### Remove an Origin

```bash
g8e gui remove --origin https://your-app.example.com
```

Removes an origin from the enrollment file. Does not restart the gateway. The origin remains in the gateway's CORS and passkey RP configuration until the gateway is restarted without the corresponding flags.

---

## API Reference

All paths are relative to the gateway's base URL. All authenticated routes require `credentials: 'include'`.

The gateway serves a full OpenAPI/Swagger specification at `/swagger/doc.json` (and browsable UI at `/swagger/`). Use this for auto-discovering the API surface.

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
| `GET` | `/api/v1/auth/passkeys` | List user's passkey credentials |
| `DELETE` | `/api/v1/auth/passkeys/{credentialId}` | Revoke a passkey |
| `GET` | `/api/v1/approvals` | List pending suspended transactions |
| `GET` | `/api/v1/approvals/{txHash}/challenge` | Get WebAuthn approval challenge |
| `POST` | `/api/v1/approvals/{txHash}/verify` | Verify approval assertion |
| `GET` | `/api/v1/sse/stream` | SSE stream for live audit events (web_session_id from cookie) |
| `GET` | `/api/v1/sse/events?since_id={n}` | Poll SSE events (web_session_id from cookie) |

### Route Authentication

Browser clients use public routes (no auth) and authenticated routes (session cookie). The SSE endpoints accept either mTLS or a session cookie. All other gateway routes require mTLS and are not accessible from browsers.

---

## WebAuthn Flow Requirements

The frontend must implement three WebAuthn ceremonies using the browser's `navigator.credentials` API. The `@simplewebauthn/browser` library handles base64url conversions automatically; if using raw `navigator.credentials`, manual base64url encoding/decoding is required.

### Base64url Encoding

The gateway sends and receives WebAuthn challenge data as base64url strings. The browser's WebAuthn API requires ArrayBuffers. The frontend must convert between base64url strings and ArrayBuffers for challenge fields, user ID fields (registration only), credential ID fields, and assertion response fields. The `@simplewebauthn/browser` library handles these conversions automatically. If using raw `navigator.credentials`, implement base64url-to-ArrayBuffer and ArrayBuffer-to-base64url helpers.

### Registration Flow (First-Time Setup)

1. POST to `/api/v1/auth/passkeys/console/register/challenge` with JSON body `{ user_id, user_name, cli_session_id: "browser" }`. If no passkeys exist on the gateway, omit `user_id` to auto-create a user.
2. Decode the returned `options.publicKey` challenge and user ID from base64url to ArrayBuffers.
3. Call `navigator.credentials.create({ publicKey: decodedOptions })` to trigger the browser's passkey enrollment prompt.
4. Encode the credential response fields (`id`, `rawId`, `clientDataJSON`, `attestationObject`, `transports`) as base64url.
5. POST to `/api/v1/auth/passkeys/console/register/verify` with JSON body `{ user_id, cli_session_id: "browser", attestation_response: { ...encodedFields } }`.
6. On success, the gateway sets a `g8e_web_session_cookie` session cookie. The frontend transitions to the authenticated state.

### Authentication Flow (Returning User)

1. POST to `/api/v1/auth/passkeys/console/authenticate/challenge` with JSON body `{ user_id }`.
2. If the response has `success: false` and `needs_setup: true`, no passkey is registered. Show the registration form instead.
3. Decode the returned `options.publicKey` challenge and `allowCredentials` from base64url to ArrayBuffers.
4. Call `navigator.credentials.get({ publicKey: decodedOptions })` to trigger the browser's passkey authentication prompt.
5. Encode the assertion response fields (`id`, `rawId`, `clientDataJSON`, `authenticatorData`, `signature`, `userHandle`) as base64url.
6. POST to `/api/v1/auth/passkeys/console/authenticate/verify` with JSON body `{ user_id, assertion_response: { ...encodedFields } }`.
7. On success, the gateway sets a `g8e_web_session_cookie` session cookie. The frontend transitions to the authenticated state.

### Approval Flow (Suspended Transaction)

1. GET `/api/v1/approvals/{txHash}/challenge` (with `credentials: 'include'`). Returns WebAuthn `PublicKeyCredentialRequestOptions`.
2. Decode the challenge and `allowCredentials` from base64url to ArrayBuffers.
3. Call `navigator.credentials.get({ publicKey: decodedOptions })` to trigger the browser's passkey prompt.
4. Encode the assertion response fields (`id`, `rawId`, `clientDataJSON`, `authenticatorData`, `signature`, `userHandle`) as base64url.
5. POST `/api/v1/approvals/{txHash}/verify` with the encoded assertion response as the JSON body.
6. On success, the transaction resumes execution. Refresh the approvals list.
7. On failure (403), show the error message. The transaction remains suspended.

### L3 Approval Redirect

When a destructive mutation is suspended by the gateway's governance gauntlet, the gateway returns an approval URL like `{publicBaseURL}/approve/{txHash}`. This URL redirects to the gateway's embedded console at `/console/#approve={txHash}`.

For external frontends, handle approvals inline by implementing the approval flow directly. When a mutation is suspended, the API response includes the transaction hash. The frontend can auto-trigger the approval flow using the `#approve={txHash}` URL hash pattern.

### Enrollment Token Flow

When the CLI initiates a passkey enrollment from the terminal, it generates a one-time enrollment token via the mTLS endpoint and opens the browser with `#register=1&token=<token>` (no raw `user_id` or `cli_session_id` in the URL). The frontend must:

1. Read the token from the URL hash (`window.location.hash`).
2. POST to `/api/v1/auth/enrollment-token/validate` with JSON body `{ token }`.
3. On success, receive `{ user_id, cli_session_id }` from the gateway.
4. Populate hidden form fields with the returned `user_id` and `cli_session_id`.
5. Auto-trigger the passkey registration flow.
6. Immediately clear the token from the URL via `history.replaceState`.

Handle error responses:
- `410 Gone`: Token has expired (5-minute TTL).
- `409 Conflict`: Token has already been used (one-time-use).
- `401 Unauthorized`: Invalid token.

---

## SSE Live Audit Stream

### Connection

- Connect to `GET /api/v1/sse/stream` using `EventSource` with `withCredentials: true`.
- The `web_session_id` is derived from the authenticated session cookie by the gateway; do not pass it in the URL.
- Show connection status: green (connected), yellow (connecting), red (disconnected).
- Provide manual connect and disconnect buttons.

### Event Handling

- Parse incoming SSE messages as JSON. Each message may contain a nested `event` field that itself is JSON.
- Extract: event type, timestamp, event ID, and raw payload.
- Display events in a scrollable log with color-coded type badges.
- Cap the in-memory event list at 500 entries (drop oldest).
- Provide a filter input (case-insensitive filter by event type), auto-scroll checkbox, clear button, and event count display.

### Reconnection

- Auto-reconnect 3 seconds after disconnection.

### Polling Fallback

If `EventSource` does not send cookies cross-origin, fall back to polling `GET /api/v1/sse/events?since_id={lastId}` every 2 seconds (the `web_session_id` is derived from the session cookie).

### WebSocket Note

The gateway also exposes a WebSocket pub/sub endpoint at `/ws/pubsub`, but it requires mTLS authentication and is not available to browser clients. Use SSE for all browser-based real-time telemetry.

---

## API Data Types

The gateway serves a full OpenAPI/Swagger specification at `/swagger/doc.json` (and browsable UI at `/swagger/`). Use this to discover request and response schemas for all endpoints, including user, passkey, session, health, bootstrap, approval, and SSE event types. The `@simplewebauthn/browser` library provides TypeScript types for WebAuthn ceremony inputs and outputs.

---

## Pages and Components

### Auth Context

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

### Login Page

Two modes based on `bootstrapped` state:

**If NOT bootstrapped (first-time setup):**

- Show a "Register Passkey" card with a display name input
- Button: "Enroll Passkey" -> calls `registerPasskey()`

**If bootstrapped (returning user):**

- Show a "Sign In" card with a User ID input
- Button: "Sign In with Passkey" -> calls `authenticatePasskey()`
- Secondary button: "Register New Passkey" -> reveals registration form

### Dashboard

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
- "Approve" button per transaction -> triggers WebAuthn approval flow
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

### Approval Flow Component

When the user clicks "Approve" on a transaction, run the steps described in [Approval Flow (Suspended Transaction)](#approval-flow-suspended-transaction). On success or failure, show the corresponding message and refresh the approvals list.

### URL Hash Handling

On app load, check `window.location.hash` for:

- `#approve={txHash}` - if user is logged in, auto-trigger the approval flow for this transaction. If not logged in, store it and trigger after login.
- `#register=1&token={enrollmentToken}` - validate the enrollment token via `POST /api/v1/auth/enrollment-token/validate`, then auto-trigger passkey registration with the returned user ID and CLI session ID.

After processing, clean the URL with `history.replaceState`.

---

## UI/UX Guidelines

- **Dark theme** with design tokens for background, surface, border, text, muted, accent, success, warning, and danger colors
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

## Error Handling

- **401 responses**: Clear user state and redirect to login
- **`needs_setup: true`** on authenticate challenge: Show registration form automatically
- **Enrollment token validation**: Handle 410 (expired) and 409 (already used) with specific error messages
- **WebAuthn API not available**: Show a browser compatibility warning
- **Network errors**: Show a retry-able error state

---

## Recommended Project Structure

Organize the frontend with separate concerns:

- An auth context for global state
- Page components for login and dashboard
- Card components for stats, passkeys, approvals, audit stream, and account
- A hook for SSE audit stream management
- Library modules for the API fetch wrapper (with `credentials: 'include'`), WebAuthn flow helpers, and type definitions

---

## Frontend Integration Checklist

- [ ] **CORS headers**: Open browser DevTools, Network tab. Make any API call and confirm `Access-Control-Allow-Origin` reflects the frontend origin and `Access-Control-Allow-Credentials: true` is present.
- [ ] **Preflight OPTIONS**: Confirm OPTIONS requests succeed with 200/204 status before actual POST requests.
- [ ] **Passkey registration**: Trigger registration and confirm the browser's WebAuthn dialog appears with the correct RP ID.
- [ ] **Passkey authentication**: Trigger authentication and confirm the WebAuthn dialog appears and login succeeds.
- [ ] **Session cookie**: After login, check DevTools, Application, Cookies for `g8e_web_session_cookie` with `SameSite=None` and `Secure` attributes.
- [ ] **Authenticated API calls**: Confirm `GET /api/v1/users/me` returns user data (not 401) after login.
- [ ] **SSE stream**: Connect to the SSE stream and confirm live events appear.
- [ ] **Approvals**: If a suspended transaction exists, confirm the approval flow triggers WebAuthn and the transaction is approved.
- [ ] **Enrollment token**: Navigate to `#register=1&token={token}` and confirm the token is validated and registration auto-triggers.
- [ ] **URL hash approval**: Navigate to `#approve={txHash}` and confirm auto-approval flow triggers.
- [ ] **Logout**: Sign out and confirm redirect to login page and cookie cleared.
- [ ] **`g8e gui verify`**: Run `g8e gui verify --origin <url>` and confirm all checklist items pass.

---

## Troubleshooting

### CORS Errors in Browser Console

**Symptom**: `Access-Control-Allow-Origin` header missing or does not match the frontend origin.

**Cause**: The gateway was not started with `--cors-origin` matching the frontend origin.

**Fix**: Restart the gateway with the correct flag:

```bash
./g8e gw start --cors-origin https://your-app.example.com --passkey-rp-origin https://your-app.example.com
```

Then run `g8e gui enroll --origin https://your-app.example.com` to verify.

### Passkey RP Mismatch

**Symptom**: WebAuthn ceremony fails with "RP ID is not a valid domain" or similar.

**Cause**: The `--passkey-rp-id` does not match the frontend app's registrable domain.

**Fix**: Set `--passkey-rp-id` to the frontend app's domain (e.g., `example.com` for `https://app.example.com`). The RP ID must be a registrable domain suffix of the current page's origin.

### SSE Connection Refused

**Symptom**: `EventSource` fails to connect or returns 401.

**Cause**: No authenticated session, or `withCredentials: true` not set on `EventSource`.

**Fix**: Authenticate first via the passkey flow. Ensure `new EventSource(url, { withCredentials: true })`. Verify the `web_session_id` is valid via `GET /api/v1/auth/sessions/me`.

### Session Cookie Not Sent Cross-Origin

**Symptom**: Authenticated API calls return 401 despite being logged in.

**Cause**: `credentials: 'include'` not set on `fetch` calls, or the gateway is not configured with `AllowedOrigins`.

**Fix**: Ensure every `fetch` call includes `credentials: 'include'`. Verify the gateway was started with `--cors-origin` for the frontend origin, which triggers `SameSite=None` on session cookies.

### Enrollment Token Errors

**Symptom**: Registration via `#register=1&token={token}` fails.

**Cause**: Token expired (5-minute TTL), already used (one-time-use), or invalid.

**Fix**: Generate a new enrollment token from the CLI (`g8e auth enroll`). Handle 410 (expired), 409 (already used), and 401 (invalid) with specific user-facing error messages.

---

## See Also

- [Lovable Frontend Integration](./lovable.md) - Lovable-specific integration guide with AI agent prompt
- [Cloudflare Tunnel Integration](./cloudflare_tunnel.md) - Expose the gateway via a public tunnel
- [Connect Apps to Gateway](./connect_apps_to_gateway.md) - General application connectivity patterns
- [Architecture: Auth](../architecture/auth.md) - WebAuthn passkey authentication architecture
- [Architecture: Gateway](../architecture/gateway.md) - Gateway service architecture
