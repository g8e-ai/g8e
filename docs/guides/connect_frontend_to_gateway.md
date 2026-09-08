---
title: Connect Frontend to Gateway
parent: Guides
---

# Connect an Existing Frontend to g8e Gateway

Last Updated: 2026-09-08
Version: v2.1.7

---

## Overview

This guide connects an **existing** frontend UI to the g8e Gateway. If you are building a frontend from scratch, see [Build a g8e-Compatible Frontend](./build_frontend.md) for the full reference. This guide covers gateway configuration, frontend enrollment, WebAuthn authentication, SSE streaming, approval flows, and passkey management.

The `g8e auth enroll gui` command family records external frontend origins and generates integration configuration. The `enroll` command validates the origin, attempts to verify the local gateway's CORS response, optionally checks a public gateway URL, persists the origin locally, and prints a TypeScript configuration snippet. It does not configure or restart the gateway, and it does not verify the gateway's passkey RP configuration. Step 2 documents the current CORS-probe limitation.

### Architecture

```
[Frontend App] → https://gateway-host:8443 → [g8e Gateway]
      ↑                                          ↓
 credentials: 'include'              WebAuthn + Session Cookie + SSE
```

The frontend communicates with the g8e Gateway over HTTPS. Authentication is via WebAuthn passkeys (no passwords, no API keys). The gateway issues an `HttpOnly` session cookie after successful passkey verification. All authenticated API calls must include `credentials: 'include'` so the cookie is sent cross-origin. Real-time telemetry is delivered via Server-Sent Events (SSE), not WebSockets (the WebSocket endpoint requires mTLS and is not available to browsers).

The gateway's embedded SPA at `/console/` implements passkey registration and authentication, approvals, SSE audit streaming, passkey management, CLI recovery approval, workload enrollment approval, and URL-fragment handling. Use it as the browser reference implementation.

---

## Prerequisites

- g8e Gateway running and healthy (`./g8e gw start`)
- Existing frontend application served from a known origin (e.g., `https://your-app.example.com`, `http://localhost:3003`)
- Gateway started with `--cors-origin` and `--passkey-rp-origin` flags matching the frontend origin
- Browser supports WebAuthn (all modern Chrome, Firefox, Safari, Edge)
- Browser trusts the gateway's HTTPS certificate; the session cookie is always `Secure`
- Frontend runs in a WebAuthn secure context (HTTPS, or the browser's localhost exception for local development)

---

## Step 1: Configure the Gateway for Your Frontend Origin

The gateway starts with CORS and passkey RP settings that match the frontend app's origin. WebAuthn ceremonies execute in the frontend page, so the RP ID is the frontend hostname or a registrable domain suffix of it, not necessarily the gateway hostname. An RP ID contains only a hostname or domain; it never includes a scheme or port. The browser rejects ceremonies when the RP ID is not valid for the page's origin.

Start the gateway with CORS and passkey RP flags matching your frontend origin:

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
export G8E_PUBLIC_BASE_URL=https://gateway.example.com
```

Key flags:

- `--passkey-rp-id`: The WebAuthn Relying Party ID. Use the frontend app's hostname or a registrable domain suffix so passkeys work across subdomains.
- `--passkey-rp-origin`: The exact origin where WebAuthn ceremonies execute, including scheme and port when non-default. Repeat for each origin.
- `--cors-origin`: The frontend app origin. Allows cross-origin requests with credentials. Repeat for each origin (preview URLs, production URLs, custom domains).
- `--public-base-url`: The public URL of the gateway (for example, a tunnel hostname). The gateway uses it for approval links and redirect host validation.

The origins environment variables accept comma-separated values. Add every origin the frontend may use. The gateway reflects exact-match origins in CORS headers and sets `SameSite=None` on session cookies whenever the allowed-origin list is non-empty.

### How CORS Works

When `AllowedOrigins` is non-empty, whether populated by `--cors-origin` or `G8E_ALLOWED_ORIGINS`, the gateway CORS middleware:

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

The cookie has a 24-hour TTL. The gateway validates it on every session-authenticated request by looking up the session ID in its database and checking expiry. Browser privacy policies can still block third-party cookies even with `SameSite=None`; deploy the frontend and gateway on the same site or proxy the gateway through the frontend origin when cross-site cookies are blocked.

---

## Step 2: Enroll Your Frontend Origin

The frontend enrollment command records an origin and generates a configuration snippet:

```bash
g8e auth enroll gui enroll --origin https://your-app.example.com --passkey-rp-id example.com
```

> **Current v2.1.7 limitation:** The command sends its CORS probe to `http://localhost:8080/api/v1/health`, but the gateway attaches CORS middleware only to the HTTPS router on port 8443. A standard gateway therefore returns no `Access-Control-Allow-Origin` header, and the command exits before it saves the enrollment or prints the snippet. The probe also ignores custom gateway ports. Verify CORS against the HTTPS gateway in Step 9 and configure the frontend directly as described in Step 3 until this command is corrected.

Optional flags:

- `--passkey-rp-id`: RP ID written to the generated snippet. Pass this explicitly. When omitted, the command derives it from the parsed URL host, which includes the port for origins such as `http://localhost:3003` and is not a valid WebAuthn RP ID.
- `--passkey-rp-name`: RP display name written to the generated snippet (default: `g8e`).
- `--public-base-url`: Gateway base URL written to the generated snippet. The command performs a health check at this URL, but a failed check produces a warning and does not fail enrollment.

These flags affect generated output only. They do not change the running gateway's RP, CORS, or public URL configuration.

When its CORS probe succeeds, this command:

1. Validates the origin URL.
2. Sends a CORS preflight to the gateway's local plain-HTTP health endpoint on the default gateway port and verifies that the response allows the origin.
3. Checks gateway health at `--public-base-url`, when provided, and warns if the check fails.
4. Persists the origin to the runtime enrollment file (`.g8e/gui_enrollments.json` under the configured runtime root).
5. Outputs a TypeScript configuration snippet with `API_BASE_URL`, `PASSKEY_RP_ID`, `PASSKEY_RP_NAME`, `apiFetch()` helper, `connectSSE()` helper, and key endpoint paths.

Copy the emitted snippet into the frontend project as a starting point. Correct its L3 redirect example to `${API_BASE_URL}/api/v1/approve/${txHash}`. The current generator emits `${API_BASE_URL}/approve/${txHash}`, which is not a registered gateway route.

### Other Enrollment Commands

- `g8e auth enroll gui show` (alias: `g8e auth enroll gui list`): Displays all enrolled frontend origins and regenerated configuration snippets. Supports `--json` for scripting.
- `g8e auth enroll gui verify --origin https://your-app.example.com`: Checks whether the origin appears in the local enrollment file and prints URLs, sample commands, and a manual checklist using the default ports. It does not execute health, CORS, WebAuthn, cookie, SSE, or authenticated-route checks.
- `g8e auth enroll gui remove --origin https://your-app.example.com`: Removes an origin from the enrollment file. It does not change the running gateway configuration.

---

## Step 3: Add the API Configuration

Use the TypeScript snippet from `g8e auth enroll gui enroll` when available, or define the equivalent configuration directly while the Step 2 probe limitation applies. The key elements are:

### API Base URL and Fetch Wrapper

Every session-authenticated API call includes `credentials: 'include'` so the cookie is sent cross-origin. Create a centralized fetch wrapper that sets credentials on every request and sets `Content-Type: application/json` when a request has a JSON body. The base URL is the gateway HTTPS address (for example, `https://gateway-host:8443`).

### Key Endpoint Paths

Public endpoints (no auth required):

- `GET /api/v1/health` - Health check
- `GET /api/v1/auth/bootstrap/status` - Report whether an owner user exists; this does not count passkeys
- `POST /api/v1/auth/passkeys/console/register/challenge` - Begin console passkey registration
- `POST /api/v1/auth/passkeys/console/register/verify` - Complete console passkey registration
- `POST /api/v1/auth/passkeys/console/authenticate/challenge` - Begin console passkey authentication
- `POST /api/v1/auth/passkeys/console/authenticate/verify` - Complete console passkey authentication
- `POST /api/v1/auth/passkeys/enrollment/register/challenge` - Begin token-gated passkey registration
- `POST /api/v1/auth/passkeys/enrollment/register/verify` - Complete token-gated passkey registration
- `POST /api/v1/auth/logout` - Clear session cookie
- `POST /api/v1/auth/enrollment-token/validate` - Validate **and consume** a CLI-generated enrollment token; do not call this before token-gated registration

Browser-accessible authenticated endpoints (session cookie required for browser calls):

- `GET /api/v1/users/me` - Get current authenticated user
- `GET /api/v1/auth/sessions/me` - Get current user ID and web session ID
- `GET /api/v1/auth/passkeys` - List registered passkeys
- `DELETE /api/v1/auth/passkeys/{credentialId}` - Revoke a passkey
- `GET /api/v1/approvals` - List pending suspended transactions
- `GET /api/v1/approvals/{txHash}/challenge` - Get WebAuthn challenge for approval
- `POST /api/v1/approvals/{txHash}/verify` - Verify WebAuthn assertion to approve transaction
- `GET /api/v1/sse/stream` - SSE live event stream (web session derived from cookie)
- `GET /api/v1/sse/events?since_id={n}&limit={n}` - Poll SSE events (web session derived from cookie)
- `POST /api/v1/auth/cli/recovery/approve` - Approve or deny a token-scoped CLI recovery request
- `GET /api/v1/auth/platform-enrollments/pending` - List owner-visible pending workload enrollments
- `POST /api/v1/auth/platform-enrollments/decision` - Approve or deny a workload enrollment

The gateway also serves a full OpenAPI/Swagger specification at `/swagger/doc.json` and a browsable UI at `/swagger/`. Browser clients use public routes and routes classified for web-session or dual authentication. Unknown routes fail closed to mTLS, so consult Swagger and the route authentication registry before exposing additional gateway operations in a browser UI.

---

## Step 4: Wire Up WebAuthn Authentication

The frontend implements WebAuthn passkey flows with the browser's `navigator.credentials` API. The gateway sends binary WebAuthn values as unpadded base64url strings, while the browser API requires `ArrayBuffer` values. Decode `challenge`, registration `user.id`, and every credential descriptor `id` before calling the browser API. Encode `rawId`, `clientDataJSON`, `attestationObject`, `authenticatorData`, `signature`, and `userHandle` before verification.

Gateway verification bodies use flat credential models. Registration's `attestation_response` contains `id`, `rawId`, `clientDataJSON`, `attestationObject`, and optional `transports`. Authentication's `assertion_response`, and the entire approval verification body, contain `id`, `rawId`, `clientDataJSON`, `authenticatorData`, `signature`, and optional `userHandle`. Libraries such as `@simplewebauthn/browser` perform binary conversion but return their own response shape; map that output to these flat gateway models.

### Registration Flow (First-Time Setup)

1. Confirm that `GET /api/v1/auth/bootstrap/status` returns `{ "bootstrapped": false }`. This means no owner user exists; it does not count passkeys.
2. POST `/api/v1/auth/passkeys/console/register/challenge` with `{ "user_name": "Display name", "cli_session_id": "browser" }`, omitting `user_id` only for this initial bootstrap.
3. Save the top-level `user_id` returned by the gateway. Decode `options.publicKey` and call `navigator.credentials.create({ publicKey })`.
4. POST `/api/v1/auth/passkeys/console/register/verify` with `{ "user_id": <saved user_id>, "cli_session_id": "browser", "attestation_response": <flat encoded attestation> }`.
5. On success, the gateway sets `g8e_web_session_cookie`. Load the current user from `/api/v1/users/me` rather than relying on a user object in the verify response.

The public console registration route permits only the user's first credential. Once an owner has a passkey, add passkeys through the CLI token-gated enrollment flow.

### Authentication Flow (Returning User)

1. Collect the g8e user ID and POST `/api/v1/auth/passkeys/console/authenticate/challenge` with `{ "user_id": <user_id> }`. The endpoint requires a user ID and does not implement discoverable-credential login without it.
2. If the response has `success: false` and `needs_setup: true`, that user has no registered passkey. Direct the user to CLI token-gated enrollment rather than anonymous registration.
3. Decode `options.publicKey`, including `challenge` and each `allowCredentials[].id`, and call `navigator.credentials.get({ publicKey })`.
4. POST `/api/v1/auth/passkeys/console/authenticate/verify` with `{ "user_id": <user_id>, "assertion_response": <flat encoded assertion> }`.
5. On success, the gateway sets `g8e_web_session_cookie`. Load `/api/v1/users/me` to establish application state.

### Auth Context Initialization

On app mount, check auth state:

1. Call `GET /api/v1/auth/bootstrap/status` to check whether an owner user exists.
2. Call `GET /api/v1/users/me` with `credentials: 'include'` to check whether the browser already has a valid session.
3. If the UI displays or coordinates by session ID, call `GET /api/v1/auth/sessions/me`. SSE routing itself derives the session ID from the cookie.

### Enrollment Token Flow

`g8e auth enroll user` generates a token and opens the gateway's embedded console at `/console/#enroll=1&token=<token>`. An external frontend does not receive this redirect automatically. If the application intentionally accepts the same token-bearing fragment, it implements this flow:

1. Read the token from the URL hash (`window.location.hash`).
2. Immediately clear the token from the URL via `history.replaceState`.
3. POST the token to `/api/v1/auth/passkeys/enrollment/register/challenge` with `{ enrollment_token: <token> }` in the JSON body. The gateway validates the token and derives `user_id` and `cli_session_id` from it; there is no separate `/enrollment-token/validate` round-trip, and the token-derived identifiers never need to touch the DOM.
4. Perform the WebAuthn ceremony with the challenge response (`navigator.credentials.create`).
5. POST the flat encoded attestation and token to `/api/v1/auth/passkeys/enrollment/register/verify` with `{ "enrollment_token": <token>, "attestation_response": <flat encoded attestation> }`. The verify handler atomically consumes the token before verifying the attestation, so a failed verify attempt requires a new token. A successful verify sets the web session cookie.

Do not call `/api/v1/auth/enrollment-token/validate` as a preliminary step. Despite its name, that endpoint validates **and consumes** the token, so the registration challenge then returns `409 Conflict`.

Handle error responses from the challenge and verify endpoints:
- **`410 Gone`**: Token has expired (5-minute TTL).
- **`409 Conflict`**: Token has already been used (one-time-use).
- **`401 Unauthorized`**: Invalid token.

---

## Step 5: Wire Up the SSE Live Audit Stream

### Connection

- Connect to `GET /api/v1/sse/stream` using `EventSource` with `withCredentials: true`.
- The `web_session_id` and `user_id` are derived from the authenticated session cookie by the gateway; do NOT pass them in the URL.
- The browser's native `EventSource` reconnects automatically and sends `Last-Event-ID`. A custom loop can close the failed source and reconnect with exponential backoff.
- The stream replays stored events by default. Use `since_id=<last durable ID>` for an explicit cursor. An explicit `since_id=0` requests live events only, while omitting the parameter replays from the beginning.

### Event Handling

- Parse each `message` event's `data` as an SSE push envelope. It contains `user_id`, exactly one session ID, and an `event` value. Producers may encode the nested `event` as an object or a JSON string, so handle both forms.
- Read the durable event ID from `MessageEvent.lastEventId`. The nested event normally supplies its own type and timestamp.
- Display events in a scrollable log with color-coded type badges.
- Cap the in-memory event list at 500 entries (drop oldest).
- Provide a filter input (case-insensitive filter by event type), auto-scroll checkbox, clear button, and event count display.

### Polling Fallback

When streaming is unavailable or blocked by deployment infrastructure, poll `GET /api/v1/sse/events?since_id={lastId}&limit={limit}` with `credentials: 'include'`. The JSON response is `{ "events": [...], "count": n }`; each row contains `id`, `event_type`, `payload`, and `created_at`. `payload` is the stored push envelope serialized as a JSON string. Advance the cursor to the highest returned row ID.

### WebSocket Note

The gateway also exposes a WebSocket pub/sub endpoint at `/api/v1/pubsub/stream`, but it requires mTLS authentication and is not available to browser clients. Use SSE for all browser-based real-time telemetry.

---

## Step 6: Wire Up Approval Flows

### Pending Approvals

Fetch pending suspended transactions via `GET /api/v1/approvals` with `credentials: 'include'`.

Display each transaction showing: tool name, transaction hash (truncated, monospace), created date, expiry countdown.

### Approval Flow (Suspended Transaction)

When user clicks "Approve" on a transaction:

1. GET `/api/v1/approvals/{txHash}/challenge` with `credentials: 'include'`.
2. Decode the response's top-level `publicKey` object, including `challenge` and each `allowCredentials[].id`, then call `navigator.credentials.get({ publicKey })`.
3. POST `/api/v1/approvals/{txHash}/verify` with the flat encoded assertion as the entire JSON body, not nested under `assertion_response`.
4. A `200` response contains the canonical protojson `ActionReceipt` and means execution resumed. Refresh the approval list.
5. A `403` response can contain either an error or an `ActionReceipt` describing rejection; preserve and display it. A `404` means the transaction is missing or expired.

### L3 Approval Redirect and URL Hash Handling

The gateway's public approval URL is `{publicBaseURL}/api/v1/approve/{txHash}`. It redirects to `/console/#approve={txHash}` on the embedded gateway console. An external frontend can approve inline with the endpoints above or implement its own fragment convention.

On app load, check `window.location.hash` for:

- **`#approve={txHash}`**: If user is logged in, auto-trigger the approval flow for this transaction. If not logged in, store it and trigger after login.
- **`#enroll=1&token={enrollmentToken}`**: If the external frontend supports token-gated enrollment, clear the fragment immediately, then post the token to the enrollment registration challenge and verify endpoints. Do not call the consuming enrollment-token validation endpoint first.

Clear secret-bearing enrollment tokens with `history.replaceState` immediately after reading them. Transaction hashes are not credentials; the frontend can retain or remove approval fragments according to its routing behavior.

---

## Step 7: Add Passkey Management

### List Passkeys

Fetch registered passkeys via `GET /api/v1/auth/passkeys` with `credentials: 'include'`.

Display each passkey with credential ID (truncated, monospace), creation date, last used date.

### Revoke a Passkey

Send `DELETE /api/v1/auth/passkeys/{credentialId}` with `credentials: 'include'`.

Use a confirmation dialog before revoking.

### Register Another Passkey

Run `g8e auth enroll user`. The CLI opens the embedded gateway console, which completes token-gated registration for that user. The public console registration endpoints are first-credential-only and reject a user who already has a passkey.

---

## Step 8: Handle Errors and Edge Cases

- **401 from session-authenticated routes**: Clear user state and show the sign-in flow; handle token-gated route failures within their own flow.
- **`needs_setup: true`** on authenticate challenge: Tell the user that the supplied user ID has no passkey and direct them to CLI token-gated enrollment.
- **Enrollment token registration**: Handle 410 (expired), 409 (already used), and 401 (invalid) with specific error messages.
- **WebAuthn API not available**: Show a browser compatibility warning.
- **Network errors**: Show a retry-able error state.

---

## Step 9: Verify the Integration

Run the enrollment inspection command:

```bash
g8e auth enroll gui verify --origin https://your-app.example.com
```

This command confirms the local enrollment record and prints instructions; it does not execute the checks. Manually verify in the browser:

- [ ] **CORS headers**: Open browser DevTools, Network tab. Make any API call and confirm `Access-Control-Allow-Origin` reflects your frontend origin and `Access-Control-Allow-Credentials: true` is present.
- [ ] **Preflight OPTIONS**: Confirm OPTIONS requests succeed with `204 No Content` before actual POST requests.
- [ ] **Passkey registration**: Trigger registration and confirm the browser's WebAuthn dialog appears with the correct RP ID.
- [ ] **Passkey authentication**: Trigger authentication and confirm the WebAuthn dialog appears and login succeeds.
- [ ] **Session cookie**: After login, check DevTools for the HttpOnly, Secure `g8e_web_session_cookie`. It uses `SameSite=None` when any CORS origin is configured and `SameSite=Lax` otherwise.
- [ ] **Authenticated API calls**: Confirm `GET /api/v1/users/me` returns user data (not 401) after login.
- [ ] **SSE stream**: Connect to the SSE stream and confirm live events appear.
- [ ] **Approvals**: If a suspended transaction exists, confirm the approval flow triggers WebAuthn and the transaction is approved.
- [ ] **Enrollment token (if supported by the external frontend)**: Navigate to `#enroll=1&token={token}` and confirm registration uses the token-gated challenge and verify endpoints without first calling `/api/v1/auth/enrollment-token/validate`. The CLI normally opens this fragment on the embedded gateway console.
- [ ] **URL hash approval**: Navigate to `#approve={txHash}` and confirm auto-approval flow triggers.
- [ ] **Logout**: Sign out and confirm redirect to login page and cookie cleared.

---

## Troubleshooting

### CORS Errors in Browser Console

**Symptom**: `Access-Control-Allow-Origin` header missing or does not match your frontend origin.

**Cause**: The gateway was not started with `--cors-origin` matching your frontend origin.

**Fix**: Restart the gateway with the correct flags: `./g8e gw start --cors-origin https://your-app.example.com --passkey-rp-origin https://your-app.example.com`

Then make an allowed-origin request to the gateway's HTTPS URL and inspect the CORS response headers in browser DevTools. The v2.1.7 `gui enroll` command cannot verify the standard HTTPS router because of the Step 2 transport mismatch.

### Passkey RP Mismatch

**Symptom**: WebAuthn ceremony fails with "RP ID is not a valid domain" or similar.

**Cause**: The `--passkey-rp-id` does not match your frontend app's registrable domain.

**Fix**: Set `--passkey-rp-id` to your frontend app's domain (e.g., `example.com` for `https://app.example.com`). The RP ID must be a registrable domain suffix of the current page's origin.

### SSE Connection Refused

**Symptom**: `EventSource` fails to connect or returns 401.

**Cause**: No authenticated session, or `withCredentials: true` not set on `EventSource`.

**Fix**: Authenticate first via the passkey flow and use `new EventSource(url, { withCredentials: true })`. Verify the session through `GET /api/v1/users/me`. The gateway derives the SSE route from the cookie; do not add `web_session_id` or `user_id` to the stream URL.

### Session Cookie Not Sent Cross-Origin

**Symptom**: Authenticated API calls return 401 despite being logged in.

**Cause**: `credentials: 'include'` is absent, the gateway does not allow the frontend origin, the browser does not trust the gateway certificate, or browser policy blocks the gateway cookie as a third-party cookie.

**Fix**: Include `credentials: 'include'`, verify the gateway was started with `--cors-origin` for the exact frontend origin, and trust the gateway certificate. If browser policy still blocks the cookie, deploy both origins on the same site or proxy gateway requests through the frontend origin.

### Enrollment Token Errors

**Symptom**: Registration via `#enroll=1&token={token}` fails.

**Cause**: Token expired (5-minute TTL), was already used, or was consumed by a preliminary call to `/api/v1/auth/enrollment-token/validate`.

**Fix**: Generate a new token with `g8e auth enroll user`. Send it directly to the token-gated registration challenge and verify endpoints, and handle 410 (expired), 409 (already used), and 401 (invalid) with specific user-facing messages.

---

## See Also

- [Build a g8e-Compatible Frontend](./build_frontend.md) - Full reference for building a frontend from scratch
- [Lovable Frontend Integration](./lovable.md) - Lovable-specific integration guide with AI agent prompt
- [Cloudflare Tunnel Integration](./cloudflare_tunnel.md) - Expose the gateway via a public tunnel
- [Connect Apps to Gateway](./connect_apps_to_gateway.md) - General application connectivity patterns
- [Authentication & Authorization](../architecture/auth.md) - WebAuthn passkey authentication architecture
- [Gateway Architecture](../architecture/gateway.md) - Gateway service architecture
- [SSE Streaming](../architecture/sse.md) - SSE push, poll, and stream endpoints for agentic ensembles
