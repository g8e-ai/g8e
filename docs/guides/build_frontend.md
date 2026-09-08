---
title: Build a g8e-Compatible Frontend
parent: Guides
---

# Build a g8e-Compatible Frontend

Last Updated: 2026-09-08
Version: v2.1.7

---

## Overview

This guide describes how to build a g8e-compatible web UI. It covers gateway configuration, frontend enrollment, WebAuthn authentication, SSE streaming, approval flows, API data types, UI/UX guidelines, and the recommended project structure. It applies to custom React apps, Vue dashboards, vanilla JS consoles, and hosted platforms like Lovable.

The `g8e auth enroll gui` command family records external frontend origins and generates integration configuration. The `enroll` command validates the origin, verifies the local gateway's CORS response, optionally checks that a public gateway URL is healthy, persists the origin to a local enrollment file, and outputs a TypeScript configuration snippet. It does not configure or restart the gateway, and it does not verify the gateway's passkey RP configuration.

For the minimal local Lovable setup, see [Connect a Lovable App](./lovable.md).

### Architecture

```
[Frontend App] → https://gateway-host:8443 → [g8e Gateway]
      ↑                                          ↓
 credentials: 'include'              WebAuthn + Session Cookie + SSE
```

The frontend is a browser-based SPA that communicates with the g8e Gateway over HTTPS. Authentication is via WebAuthn passkeys (no passwords, no API keys). The gateway issues an HttpOnly session cookie after successful passkey verification. All authenticated API calls must include `credentials: 'include'` so the cookie is sent cross-origin. Real-time telemetry is delivered via Server-Sent Events (SSE), not WebSockets (the WebSocket endpoint requires mTLS and is not available to browsers).

### The Built-In Console as Reference

The g8e Gateway ships with an embedded, single-file vanilla JavaScript console SPA at `/console/`. It implements passkey registration and authentication, transaction approvals, SSE audit streaming, passkey management, CLI recovery approval, platform workload enrollment approval, and URL-fragment handling for enrollment and approval links. It is the canonical browser reference implementation.

---

## Prerequisites

- g8e Gateway running and healthy
- Frontend application served from a known origin (e.g., `https://your-app.example.com`, `http://localhost:3003`)
- Gateway started with `--cors-origin` and `--passkey-rp-origin` flags matching the frontend origin
- Browser supports WebAuthn (all modern Chrome, Firefox, Safari, Edge)
- Browser trusts the gateway's HTTPS certificate; the gateway session cookie is always `Secure`
- Frontend runs in a WebAuthn secure context (HTTPS, or the browser's localhost exception for local development)

---

## Gateway-Side Configuration

The gateway starts with CORS and passkey RP settings that match the frontend app's origin. WebAuthn ceremonies execute in the frontend page, so the RP ID is the frontend hostname or a registrable domain suffix of it, not necessarily the gateway hostname. An RP ID contains only a hostname or domain: it never includes a scheme or port. The browser rejects ceremonies when the RP ID is not valid for the page's origin.

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
g8e auth enroll gui enroll --origin https://your-app.example.com --passkey-rp-id example.com
```

Optional flags:

- `--passkey-rp-id` - RP ID written to the generated snippet. Pass this explicitly. When omitted, the command derives it from the parsed URL host, which includes the port for origins such as `http://localhost:3003` and is not a valid WebAuthn RP ID.
- `--passkey-rp-name` - RP display name written to the generated snippet (default: `g8e`)
- `--public-base-url` - Gateway base URL written to the generated snippet. The command performs a health check at this URL, but a failed check produces a warning and does not fail enrollment.

These flags affect generated output only. They do not change the running gateway's RP or public URL configuration.

This command:

1. Validates the origin URL
2. Sends a CORS preflight to the gateway's local plain-HTTP health endpoint on the default gateway port and verifies that the response allows the origin
3. Checks gateway health at `--public-base-url`, when provided, and warns if the check fails
4. Persists the origin to the runtime enrollment file (`.g8e/gui_enrollments.json` under the configured runtime root)
5. Outputs a TypeScript configuration snippet with `API_BASE_URL`, `PASSKEY_RP_ID`, `PASSKEY_RP_NAME`, `apiFetch()` helper, `connectSSE()` helper, and key endpoint paths

Copy the emitted snippet into the frontend project as a starting point, then correct its L3 redirect example to `${API_BASE_URL}/api/v1/approve/${txHash}`. The current generator emits `${API_BASE_URL}/approve/${txHash}`, which is not a registered gateway route.

### Show Enrolled Origins

```bash
g8e auth enroll gui show
```

Alias: `g8e auth enroll gui list`. Lists all enrolled origins and regenerates config snippets. Supports `--json` for scripting:

```bash
g8e auth enroll gui show --json
```

### Verify Enrollment

```bash
g8e auth enroll gui verify --origin https://your-app.example.com
```

Checks whether the origin appears in the local enrollment file and prints URLs, sample commands, and a manual verification checklist. It does not execute the health, CORS, WebAuthn, cookie, SSE, or authenticated-route checks.

### Remove an Origin

```bash
g8e auth enroll gui remove --origin https://your-app.example.com
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
| `GET` | `/api/v1/auth/bootstrap/status` | Report whether the gateway has an owner user |
| `POST` | `/api/v1/auth/passkeys/console/register/challenge` | Get a bootstrap WebAuthn registration challenge |
| `POST` | `/api/v1/auth/passkeys/console/register/verify` | Verify bootstrap registration attestation and create a web session |
| `POST` | `/api/v1/auth/passkeys/console/authenticate/challenge` | Get a WebAuthn authentication challenge for a user ID |
| `POST` | `/api/v1/auth/passkeys/console/authenticate/verify` | Verify authentication assertion and create a web session |
| `POST` | `/api/v1/auth/passkeys/enrollment/register/challenge` | Get a token-gated registration challenge |
| `POST` | `/api/v1/auth/passkeys/enrollment/register/verify` | Verify token-gated registration and create a web session |
| `POST` | `/api/v1/auth/logout` | Delete the current session when present and clear the session cookie |
| `POST` | `/api/v1/auth/enrollment-token/validate` | Validate **and consume** an enrollment token; do not call this before the token-gated registration flow |

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
| `GET` | `/api/v1/sse/events?since_id={n}&limit={n}` | Poll SSE events (web session from cookie) |
| `POST` | `/api/v1/auth/cli/recovery/approve` | Approve or deny a token-scoped CLI recovery request |
| `GET` | `/api/v1/auth/platform-enrollments/pending` | List owner-visible pending workload enrollments |
| `POST` | `/api/v1/auth/platform-enrollments/decision` | Approve or deny a workload enrollment |

### Route Authentication

Browser clients use public routes and routes classified for web-session or dual authentication. The SSE consumer endpoints accept either mTLS or a session cookie. The platform enrollment owner routes also accept either authentication mode; browser calls use the session cookie. Unknown routes fail closed to mTLS. Consult `/swagger/` and the route authentication registry before exposing additional gateway operations in a browser UI.

---

## WebAuthn Flow Requirements

The browser implements three WebAuthn ceremonies with `navigator.credentials`: registration, authentication, and transaction approval. Gateway challenge responses wrap browser options under `options.publicKey` for registration and authentication, while the approval challenge response places them directly under `publicKey`.

### Base64url Encoding and Wire Shape

The gateway sends WebAuthn binary values as unpadded base64url strings, while `navigator.credentials` requires `ArrayBuffer` values. Decode `challenge`, registration `user.id`, and every credential descriptor `id` before calling the browser API. Encode `rawId`, `clientDataJSON`, `attestationObject`, `authenticatorData`, `signature`, and `userHandle` before verification.

The gateway verification bodies use flat credential models. Registration's `attestation_response` contains `id`, `rawId`, `clientDataJSON`, `attestationObject`, and optional `transports`. Authentication's `assertion_response`, and the entire approval verification body, contain `id`, `rawId`, `clientDataJSON`, `authenticatorData`, `signature`, and optional `userHandle`. Libraries such as `@simplewebauthn/browser` perform binary conversion but return their own response shape; map that output to these flat gateway models.

### Registration Flow (First-Time Setup)

1. Confirm that `GET /api/v1/auth/bootstrap/status` returns `{ "bootstrapped": false }`. This means no owner user exists; it does not count passkeys.
2. POST `/api/v1/auth/passkeys/console/register/challenge` with `{ "user_name": "Display name", "cli_session_id": "browser" }`, omitting `user_id` only for this initial bootstrap.
3. Save the top-level `user_id` returned by the gateway. Decode `options.publicKey` and call `navigator.credentials.create({ publicKey })`.
4. POST `/api/v1/auth/passkeys/console/register/verify` with `{ "user_id": <saved user_id>, "cli_session_id": "browser", "attestation_response": <flat encoded attestation> }`.
5. On success, the gateway sets `g8e_web_session_cookie`. Load the current user from `/api/v1/users/me` rather than relying on a user object in the verify response.

The public console registration route enforces first-credential-only registration. Once an owner or credential exists, use the CLI token-gated enrollment flow rather than trying to bootstrap another anonymous user.

### Authentication Flow (Returning User)

1. Collect the g8e user ID and POST `/api/v1/auth/passkeys/console/authenticate/challenge` with `{ "user_id": <user_id> }`. The current gateway requires `user_id`; this endpoint does not implement discoverable-credential login without it.
2. If the response has `success: false` and `needs_setup: true`, that user has no registered passkey.
3. Decode `options.publicKey` and call `navigator.credentials.get({ publicKey })`.
4. POST `/api/v1/auth/passkeys/console/authenticate/verify` with `{ "user_id": <user_id>, "assertion_response": <flat encoded assertion> }`.
5. On success, the gateway sets `g8e_web_session_cookie`. Load `/api/v1/users/me` to establish application state.

### Approval Flow (Suspended Transaction)

1. GET `/api/v1/approvals/{txHash}/challenge` with the session cookie.
2. Decode the response's `publicKey` object and call `navigator.credentials.get({ publicKey })`.
3. POST `/api/v1/approvals/{txHash}/verify` with the flat encoded assertion as the entire JSON body, not nested under `assertion_response`.
4. A `200` response contains the canonical protojson `ActionReceipt` and means execution resumed. Refresh the approval list. A `403` response may also contain a receipt describing the rejected result; preserve and display that response rather than assuming every failure uses `{ "error": ... }`. A `404` means the transaction is missing or expired.

### L3 Approval Redirect

When a governed mutation is suspended for L3 approval, the gateway approval URL is `{publicBaseURL}/api/v1/approve/{txHash}`. That public route redirects to `/console/#approve={txHash}` on the gateway.

An external frontend handles approvals inline with the approval endpoints above. When it receives a transaction hash, it can navigate to the gateway approval URL or use its own `#approve={txHash}` fragment convention and trigger the flow after authentication. The external fragment convention is application behavior; the gateway redirect always targets the embedded console.

### Enrollment Token Flow

When the CLI initiates a passkey enrollment from the terminal, it generates a one-time enrollment token via the mTLS endpoint and opens the browser with `#enroll=1&token=<token>` (no raw `user_id` or `cli_session_id` in the URL). The frontend must:

1. Read the token from the URL hash (`window.location.hash`).
2. Immediately clear the token from the URL via `history.replaceState`.
3. POST the token to `/api/v1/auth/passkeys/enrollment/register/challenge` with JSON body `{ "enrollment_token": <token> }`. The gateway validates the token without consuming it and derives `user_id` and `cli_session_id` internally.
4. Decode `options.publicKey` and perform the registration ceremony with `navigator.credentials.create({ publicKey })`.
5. POST the flat encoded attestation and the same token to `/api/v1/auth/passkeys/enrollment/register/verify` with `{ "enrollment_token": <token>, "attestation_response": <flat encoded attestation> }`. The verify handler atomically consumes the token before it verifies the attestation, so a failed verify attempt requires a new token. A successful verify sets the web session cookie.

Do not call `/api/v1/auth/enrollment-token/validate` as a preliminary step. Despite its name, that endpoint validates **and consumes** the token, so the subsequent registration challenge returns `409 Conflict`.

Handle error responses (both challenge and verify endpoints):
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

- Parse each `message` event's `data` as an SSE push envelope. It contains `user_id`, exactly one session ID, and an `event` value. Producers may encode the nested `event` as an object or a JSON string, so handle both forms.
- Read the durable event ID from `MessageEvent.lastEventId`. The nested event normally supplies its own `type` and timestamp.
- Display events in a scrollable log with color-coded type badges.
- Cap the in-memory event list at 500 entries (drop oldest).
- Provide a filter input (case-insensitive filter by event type), auto-scroll checkbox, clear button, and event count display.

### Reconnection

- `EventSource` reconnects automatically and sends `Last-Event-ID`; a custom reconnect loop can close the failed source and reconnect with exponential backoff. The built-in console starts near 1 second, doubles each retry, caps at 30 seconds, adds up to 500ms jitter, and resets after `onopen`.
- The stream replays stored events by default. Use `since_id=<last durable ID>` for an explicit cursor. An explicit `since_id=0` requests live events only, while omitting the parameter replays from the beginning.

### Polling Fallback

When streaming is unavailable or blocked by deployment infrastructure, poll `GET /api/v1/sse/events?since_id={lastId}&limit={limit}` with `credentials: 'include'`. The JSON response is `{ "events": [...], "count": n }`; each row contains `id`, `event_type`, `payload`, and `created_at`. `payload` is the stored push envelope serialized as a JSON string. Advance the cursor to the highest returned row ID.

### WebSocket Note

The gateway also exposes a WebSocket pub/sub endpoint at `/api/v1/pubsub/stream`, but it requires mTLS authentication and is not available to browser clients. Use SSE for all browser-based real-time telemetry.

---

## API Data Types

The gateway serves a full OpenAPI/Swagger specification at `/swagger/doc.json` (and browsable UI at `/swagger/`). Use this to discover request and response schemas for all endpoints, including user, passkey, session, health, bootstrap, approval, and SSE event types. The `@simplewebauthn/browser` library provides TypeScript types for WebAuthn ceremony inputs and outputs.

---

## Pages and Components

### Auth Context

Manage global auth state:

- `user: User | null`
- `loading: boolean`
- `bootstrapped: boolean` (whether an owner user exists on the gateway)
- `webSessionId: string | null` (optional for display or application coordination; SSE routing comes from the cookie)
- `login()`, `logout()`, `registerPasskey()`, `refreshUser()` methods

On mount:

1. Call `GET /api/v1/auth/bootstrap/status` to check whether an owner user exists
2. Call `GET /api/v1/users/me` with `credentials: 'include'` to check whether the browser already has a valid session
3. If the UI displays or coordinates by session ID, call `GET /api/v1/auth/sessions/me`; SSE itself derives the session ID from the cookie

### Login Page

Two modes based on whether an owner user exists:

**If NOT bootstrapped (no owner exists):**

- Show a "Register Passkey" card with a display name input
- Button: "Enroll Passkey" -> calls `registerPasskey()`

**If bootstrapped (returning user):**

- Show a "Sign In" card with a User ID input
- Button: "Sign In with Passkey" -> calls `authenticatePasskey()`
- Direct users who need to add a passkey to `g8e auth enroll user`; anonymous console registration is first-credential-only

### Dashboard

After authentication, show:

**Stats Row:**

- Number of registered passkeys
- Number of pending transaction approvals
- Number of pending platform workload enrollments, when the UI supports owner enrollment review
- Gateway version from the health endpoint

**Passkeys Card:**

- List all passkeys with credential ID (truncated, monospace), creation date, and last-used date
- "Revoke" button per passkey with a confirmation dialog
- Direct passkey additions through the CLI token-gated enrollment flow; the public console registration endpoint cannot add another credential after the first credential exists

**Pending Approvals Card:**

- List all suspended transactions showing: tool name, transaction hash (truncated, monospace), created date, expiry countdown
- "Approve" button per transaction -> triggers WebAuthn approval flow
- Empty state: "No pending approvals"

**Platform Workload Enrollment Card (owner consoles):**

- List requests from `GET /api/v1/auth/platform-enrollments/pending`
- Display component kind, component name, instance ID, system fingerprint, CSR fingerprints, state, and expiry before the owner decides
- Submit an explicit `approve` or `deny` decision to `/api/v1/auth/platform-enrollments/decision`

**Live Audit Stream Card:**

- Connect/Disconnect button for SSE stream
- Status indicator (green=connected, yellow=connecting, red=disconnected)
- Scrollable log area showing events as they arrive
- Each event shows: event type badge (color-coded), timestamp, event ID, expandable JSON payload
- Filter input (filters by event type, case-insensitive)
- Auto-scroll checkbox
- Clear button
- Event count display
- Auto-reconnect on disconnect (exponential backoff, capped at 30 seconds)

**Account Card:**

- Display user ID (monospace)
- "Sign Out" button

### Approval Flow Component

When the user clicks "Approve" on a transaction, run the steps described in [Approval Flow (Suspended Transaction)](#approval-flow-suspended-transaction). On success or failure, show the corresponding message and refresh the approvals list.

### URL Hash Handling

On app load, check `window.location.hash` for:

- `#approve={txHash}` - if user is logged in, auto-trigger the approval flow for this transaction. If not logged in, store it and trigger after login.
- `#enroll=1&token={enrollmentToken}` - clear the fragment immediately, then post the token to the enrollment registration challenge and verify endpoints. The gateway derives the user ID and CLI session ID from the token.

The embedded console also handles `#recovery={token}` for CLI recovery approval and `#platform-enrollment={requestId}` for workload enrollment review. A custom owner console that supports those workflows follows the corresponding public/status and authenticated approval endpoints in Swagger.

Clear secret-bearing enrollment and recovery tokens with `history.replaceState` immediately after reading them. Transaction hashes and platform enrollment request IDs are not credentials; an external frontend can retain or remove those fragments according to its routing behavior.

---

## Reference UI/UX

These choices describe the embedded console and are not compatibility requirements for external frontends.

- **Dark theme** with design tokens for background, surface, border, text, muted, accent, success, warning, and danger colors
- **Monospace font** for all hashes, credential IDs, and technical identifiers
- **Truncate long hashes** to first 24 characters with `...` suffix
- **Color-coded event type badges**: blue for events, red for errors, yellow for warnings, green for info/success
- **Confirmation dialogs** before destructive actions (revoke passkey)
- **Toast notifications** for success/error feedback on all async operations
- **Loading states** on all buttons during async operations
- **Responsive layout** - max-width 720px centered on desktop, full-width on mobile
- **Header bar**: "g8e Console" title (accent color) + current user display name on the right
- **Footer**: "g8e Gateway © 2026 Lateralus Labs, LLC."

---

## Error Handling

- **401 from session-authenticated routes**: Clear user state and show the sign-in flow; handle token-gated route failures in their own flow
- **`needs_setup: true`** on authenticate challenge: Tell the user that the supplied user ID has no passkey and direct them to the CLI token-gated enrollment flow
- **Enrollment token registration**: Handle 410 (expired), 409 (already used), and 401 (invalid) with specific messages
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
- [ ] **Preflight OPTIONS**: Confirm an allowed-origin OPTIONS request returns `204 No Content` before the actual request.
- [ ] **Passkey registration**: Trigger registration and confirm the browser's WebAuthn dialog appears with the correct RP ID.
- [ ] **Passkey authentication**: Trigger authentication and confirm the WebAuthn dialog appears and login succeeds.
- [ ] **Session cookie**: After login, check DevTools for the HttpOnly, Secure `g8e_web_session_cookie`. It uses `SameSite=None` when any CORS origin is configured and `SameSite=Lax` otherwise.
- [ ] **Authenticated API calls**: Confirm `GET /api/v1/users/me` returns user data (not 401) after login.
- [ ] **SSE stream**: Connect to the SSE stream and confirm live events appear.
- [ ] **Approvals**: If a suspended transaction exists, confirm the approval flow triggers WebAuthn and the transaction is approved.
- [ ] **Enrollment token**: Navigate to `#enroll=1&token={token}` and confirm registration uses the token-gated challenge and verify endpoints without first calling `/auth/enrollment-token/validate`.
- [ ] **URL hash approval**: Navigate to `#approve={txHash}` and confirm auto-approval flow triggers.
- [ ] **Logout**: Sign out and confirm redirect to login page and cookie cleared.
- [ ] **Enrollment record**: Run `g8e auth enroll gui verify --origin <url>` to confirm the local enrollment record and print the manual checks; complete the checks above separately.

---

## Troubleshooting

### CORS Errors in Browser Console

**Symptom**: `Access-Control-Allow-Origin` header missing or does not match the frontend origin.

**Cause**: The gateway was not started with `--cors-origin` matching the frontend origin.

**Fix**: Restart the gateway with the correct flag:

```bash
./g8e gw start --cors-origin https://your-app.example.com --passkey-rp-origin https://your-app.example.com
```

Then run `g8e auth enroll gui enroll --origin https://your-app.example.com --passkey-rp-id example.com` to verify the local gateway's CORS response and generate a snippet.

### Passkey RP Mismatch

**Symptom**: WebAuthn ceremony fails with "RP ID is not a valid domain" or similar.

**Cause**: The `--passkey-rp-id` does not match the frontend app's registrable domain.

**Fix**: Set `--passkey-rp-id` to the frontend app's domain (e.g., `example.com` for `https://app.example.com`). The RP ID must be a registrable domain suffix of the current page's origin.

### SSE Connection Refused

**Symptom**: `EventSource` fails to connect or returns 401.

**Cause**: No authenticated session, or `withCredentials: true` not set on `EventSource`.

**Fix**: Authenticate first via the passkey flow and use `new EventSource(url, { withCredentials: true })`. Confirm `GET /api/v1/users/me` succeeds with credentials. The gateway derives the SSE route from the cookie; do not add `web_session_id` or `user_id` to the stream URL.

### Session Cookie Not Sent Cross-Origin

**Symptom**: Authenticated API calls return 401 despite being logged in.

**Cause**: `credentials: 'include'` not set on `fetch` calls, or the gateway is not configured with `AllowedOrigins`.

**Fix**: Ensure every `fetch` call includes `credentials: 'include'`. Verify the gateway was started with `--cors-origin` for the frontend origin, which triggers `SameSite=None` on session cookies.

### Enrollment Token Errors

**Symptom**: Registration via `#enroll=1&token={token}` fails.

**Cause**: Token expired (5-minute TTL), already used (one-time-use), or invalid.

**Fix**: Generate a new enrollment token from the CLI (`g8e auth enroll user`). Handle 410 (expired), 409 (already used), and 401 (invalid) with specific user-facing error messages.

---

## See Also

- [Connect a Lovable App](./lovable.md) - Minimal local setup for a browser-hosted Lovable app
- [Cloudflare Tunnel Integration](./cloudflare_tunnel.md) - Expose the gateway via a public tunnel
- [Connect Apps to Gateway](./connect_apps_to_gateway.md) - General application connectivity patterns
- [Architecture: Auth](../architecture/auth.md) - WebAuthn passkey authentication architecture
- [Architecture: Gateway](../architecture/gateway.md) - Gateway service architecture

---

## In-Tree Dashboard (g8ed) Server-to-Server mTLS Enrollment

The previous sections cover browser-based frontends that authenticate via WebAuthn passkeys and session cookies. The in-tree dashboard (`g8ed`, `dashboard/`) is a special case: its **browser SPA** still authenticates via WebAuthn passkeys (unchanged), but its **container** also holds its own mTLS app identity for prepared server-to-server gateway clients, mirroring the ensemble's enrollment model. `server.js` does not currently construct those clients. The two identity surfaces are independent — the browser never presents the container's mTLS cert, and the container never holds the browser's session cookie.

### Enrollment

The dashboard container enrolls at startup via the owner-approved platform enrollment protocol, the same protocol the ensemble uses. The `AppEnrollmentService` (`dashboard/services/infra/app-enrollment-service.js`) implements the nine-step resumable sequence mirroring the ensemble's `ensemble/app/services/infra/app_enrollment_service.py`:

- `loadIdentity()` — read path. Requires the existing certificate and key files, parses the certificate, rejects certificates within 7 days of expiry, and extracts the SPIFFE `app_id` from the URI SAN. It does not contact the gateway. This reuse path does not revalidate the key match or trust chain.
- `enroll()` — write path. Loads any persisted pending attempt; if none exists, generates an ECDSA P-256 key and CSR, fetches the CA bundle from the gateway's plain-HTTP discovery surface, submits a platform enrollment request with the CSR and system fingerprint, persists pending state (private key, requester token, request ID, CSR fingerprints, and expiry) to `pki/pending-enrollment/dashboard.json` with `0600` permissions, polls status with bounded backoff, signs the canonical completion transcript, validates the returned identity against the pinned trust material, expected SANs, public key, and component kind, and writes the credentials. On restart with a pending attempt, it resumes the same request without generating new keys.

The enrollment is resumable and idempotent: on restart with a valid, non-near-expiry cert, the reuse path short-circuits. On restart while a platform enrollment request is pending, the service loads the persisted pending state and resumes polling the same request. On enrollment failure, the dashboard container exits non-zero (fail-closed) so Docker's healthcheck and restart policy surface the failure. The enrolled credentials persist across container restarts in the `g8e-dashboard-data` named volume. See [auth.md](../architecture/auth.md) §1.5 for the owner-approved platform enrollment protocol.

### Environment Variables

The dashboard startup and browser configuration use these environment variables; `docker-compose.yml` supplies all three:

| Variable | Default | Description |
| --- | --- | --- |
| `G8E_GATEWAY_URL` | none (required) | Browser-facing HTTPS gateway origin injected into `/g8e-config.js`. The static server exits if it is unset. |
| `G8E_GATEWAY_HTTP_URL` | none (required) | Gateway plain-HTTP bootstrap surface URL (compose uses `http://g8eg:8080`). Enrollment uses it for CA discovery and platform enrollment requests. The service does not derive it from `G8E_GATEWAY_URL`. |
| `G8E_RUNTIME_DIR` | none (required; compose uses `/data`) | Dashboard runtime root for credentials and pending enrollment state. The non-root `g8e` user (UID 1001) owns the `g8e-dashboard-data` volume mounted at `/data`. |

### Credential Path Layout

The dashboard's runtime tree mirrors the ensemble's layout so the gateway-side cert directory structure is consistent across enrolled apps:

- `${G8E_RUNTIME_DIR}/pki/issued/apps/g8ed.crt` — enrolled app certificate followed by its certificate chain (permissions `0600`)
- `${G8E_RUNTIME_DIR}/pki/issued/apps/g8ed.key` — ECDSA P-256 private key (permissions `0600`)
- `${G8E_RUNTIME_DIR}/pki/trust/hub-bundle.pem` — trust bundle (permissions `0644`)
- `${G8E_RUNTIME_DIR}/pki/pending-enrollment/dashboard.json` — resumable pending request state, present only while needed (permissions `0600`)

### Browser vs Container Identity

The dashboard's browser SPA and container identity remain independent: the browser uses gateway-direct WebAuthn and session cookies, while `runStartupEnrollment()` resolves the container's mTLS identity before `server.js` starts the static host. `server.js` does not construct server-to-server gateway clients with that identity.

The current `dashboard/public/js/components/auth.js` and `dashboard/public/js/utils/sse-connection-manager.js` do not fully match the gateway browser contract documented above: the auth code reads challenge options without the `publicKey` wrapper, attempts authentication without the required `user_id`, serializes verification credentials in a nested browser shape instead of the gateway's flat model, and the SSE manager opens `EventSource` on the JSON polling endpoint rather than `/api/v1/sse/stream`. Treat the embedded `/console/` implementation and the gateway request models as canonical until those dashboard paths are aligned.

See [Dashboard (g8ed)](../architecture/dashboard.md) for the platform-level architecture, [Dashboard Authentication](../dashboard/auth.md) for the component enrollment flow, the [g8ed documentation](../dashboard/index.md) for the full dashboard component reference, and [Ensemble (g8ee)](../architecture/ensemble.md) for the parallel ensemble enrollment implementation.
