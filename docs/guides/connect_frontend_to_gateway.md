---
doc_id: connect_frontend_to_gateway
title: Connect Frontend to Gateway
audience: frontend developers integrating with g8e
status: current
last_updated: 2026-10-06
version: v2.3.2
owners:
  - docs/guides/connect_frontend_to_gateway.md
  - internal/services/gateway/
related:
  - docs/guides/build_frontend.md
  - docs/guides/lovable.md
  - docs/guides/cloudflare_tunnel.md
  - docs/architecture/auth.md
  - docs/architecture/gateway.md
when_to_read: Connecting a browser-based frontend application to the g8e Gateway via WebAuthn passkeys, CORS, SSE, and session authentication.
do_not_use_for:
  - Building a frontend from scratch (see build_frontend.md)
  - Gateway deployment topology (see build_gateway.md)
  - Application-to-gateway connectivity (see connect_apps_to_gateway.md)
---

# Connect Frontend to Gateway

## Purpose

This guide enables frontend applications (browser-hosted, JavaScript-based) to authenticate with the g8e Gateway via WebAuthn passkeys, establish HTTPS connections, receive server-sent events (SSE), approve suspended transactions, and manage passkeys through the browser's native credential APIs.

For single-origin local development, the `./g8e gw connect <frontend-origin>` workflow automates CORS and passkey configuration, certificate trust installation, and HTTPS verification. Advanced deployments use `./g8e gw start` flags for multi-origin and public-tunnel scenarios.

## Quick index

- [Purpose](#purpose)
- [Quick index](#quick-index)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

Invariant groups: [API routing and authentication](#api-routing-and-authentication-inv-fe-auth), [Cookie and CORS behavior](#cookie-and-cors-behavior-inv-fe-cors), [WebAuthn flows](#webauthn-flows-inv-fe-wa), [Error handling](#error-handling-inv-fe-err).

## Invariants

Ids are stable. Append the next free number in a topic. Do not renumber.

### API routing and authentication (`INV-FE-AUTH`)

| ID | Rule |
| --- | --- |
| INV-FE-AUTH-01 | Browser-accessible routes (web session required) MUST use `GET /api/v1/users/me`, `GET /api/v1/auth/sessions/me`, `GET /api/v1/auth/passkeys`, `DELETE /api/v1/auth/passkeys/{credentialId}`, `GET /api/v1/approvals`, `GET /api/v1/approvals/{txHash}/challenge`, `POST /api/v1/approvals/{txHash}/verify`, `GET /api/v1/sse/stream`, and `GET /api/v1/sse/events` as the canonical set. Additional routes may exist for infrastructure or observability; consult OpenAPI spec at `/swagger/doc.json` before exposing unlisted operations. |
| INV-FE-AUTH-02 | Public endpoints (no auth required) MUST include passkey registration and authentication challenges, bootstrap status, enrollment token validation, logout, and CLI recovery approval routes. Consult `internal/services/gateway/gateway_http_router.go` for the complete canonical list. |
| INV-FE-AUTH-03 | Every session-authenticated call MUST include `credentials: 'include'` in the fetch options so the gateway's `g8e_web_session_cookie` is sent. CORS-restricted browsers block cookies without this flag. |
| INV-FE-AUTH-04 | API responses MUST be interpreted through their HTTP status code: 200 (success), 401 (not authenticated, redirect to login), 403 (forbidden by policy), 404 (resource not found), 409 (conflict, e.g. token already used), 410 (gone, e.g. token expired), 5xx (server error). |

### Cookie and CORS behavior (`INV-FE-CORS`)

| ID | Rule |
| --- | --- |
| INV-FE-CORS-01 | The gateway sets `g8e_web_session_cookie` after successful passkey registration or authentication. The cookie is always `HttpOnly` (no JavaScript access) and `Secure` (HTTPS-only). |
| INV-FE-CORS-02 | When no cross-origin origins are configured (single-origin deployment), the cookie uses `SameSite=Lax`. When `AllowedOrigins` is non-empty (`--cors-origin` flag or `G8E_ALLOWED_ORIGINS` env var), the cookie uses `SameSite=None` and requires browser support for same-site cookies across origins. |
| INV-FE-CORS-03 | The gateway CORS middleware reflects exact-match origins in `Access-Control-Allow-Origin` and sets `Access-Control-Allow-Credentials: true` when the request origin is in the allowed set. Trailing slashes and case are normalized before matching. |
| INV-FE-CORS-04 | The CORS-origin list is set at gateway startup via `--cors-origin` (repeatable) or `G8E_ALLOWED_ORIGINS` (comma-separated). Changes require a gateway restart. |

### WebAuthn flows (`INV-FE-WA`)

| ID | Rule |
| --- | --- |
| INV-FE-WA-01 | WebAuthn challenges are base64url-encoded by the gateway; the frontend MUST decode them to `ArrayBuffer` before calling `navigator.credentials.create()` and `navigator.credentials.get()`. Responses MUST be re-encoded before sending to the gateway's verify endpoints. |
| INV-FE-WA-02 | Registration and authentication challenges contain `options.publicKey`; decode `challenge` and all `allowCredentials[].id` values. The RP ID is derived from the frontend origin via `--passkey-rp-id` and MUST match the registrable domain of the page's origin or the ceremony fails in the browser. |
| INV-FE-WA-03 | The console registration route (bootstrap) permits only the first credential per user. Once the owner has a passkey, additional credentials MUST be added via the CLI token-gated enrollment flow (`g8e auth enroll user`). |
| INV-FE-WA-04 | Approval WebAuthn ceremonies (for suspended transactions) do NOT create a new session; they issue an `ActionReceipt` that resumes execution. Treat approval ceremonies as a distinct flow from initial authentication. |

### Error handling (`INV-FE-ERR`)

| ID | Rule |
| --- | --- |
| INV-FE-ERR-01 | Enrollment token errors MUST be handled distinctly: 410 Gone (token expired, 5-minute TTL), 409 Conflict (token consumed, one-time-use), 401 Unauthorized (invalid token). Expired or consumed tokens require a new `g8e auth enroll user` request. |
| INV-FE-ERR-02 | WebAuthn API unavailability (non-compliant browser or secure context not available) MUST surface a compatibility warning and offer recovery instructions. Fallback to password-based auth is not part of this flow. |
| INV-FE-ERR-03 | SSE stream disconnection (network error, browser tab backgrounded) MUST trigger automatic reconnection via `EventSource` native behavior or a custom exponential-backoff loop with `Last-Event-ID` support. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Browser credential flows | `internal/services/gateway/passkey_service_http.go` | Register and authenticate challenges, enrollment token validation, WebAuthn response verification. |
| CORS and session cookie behavior | `internal/services/gateway/passkey_service_http.go` | Cookie attributes (`HttpOnly`, `Secure`, `SameSite` selection based on `crossOrigin` flag). |
| Web session routes (browser-accessible) | `internal/services/gateway/gateway_http_router.go` | Unified middleware validates cookie before handler invocation. |
| SSE stream and poll endpoints | `internal/services/gateway/gateway_http_router.go` | Event delivery, cursor tracking, streaming vs. polling fallback. |
| API paths and constants | `internal/constants/api_paths.go` | Canonical endpoint paths for auth, users, approvals, and observability routes. |

## Procedures

### Configure the Gateway for Your Frontend Origin

For a single-origin local frontend, use the guided workflow:

```bash
./g8e gw connect https://localhost:3000
```

This command validates the origin, derives the RP ID, starts or restarts the Gateway with the correct CORS and passkey settings when needed, installs local certificate trust with consent, and verifies HTTPS and CORS against the running process.

For multi-origin or public deployments, use explicit `gw start` flags:

```bash
./g8e gw start \
  --passkey-rp-id example.com \
  --passkey-rp-name "g8e Console" \
  --passkey-rp-origin https://app.example.com \
  --cors-origin https://app.example.com
```

Or via environment variables:

```bash
export G8E_PASSKEY_RP_ID=example.com
export G8E_PASSKEY_RP_NAME="g8e Console"
export G8E_PASSKEY_RP_ORIGINS=https://app.example.com
export G8E_ALLOWED_ORIGINS=https://app.example.com
export G8E_PUBLIC_BASE_URL=https://gateway.example.com
```

Key flags and env vars:

- `--passkey-rp-id` / `G8E_PASSKEY_RP_ID`: WebAuthn Relying Party ID. Use the frontend app's hostname or a registrable domain suffix so passkeys work across subdomains (e.g., `example.com` for `https://app.example.com`).
- `--passkey-rp-origin` / `G8E_PASSKEY_RP_ORIGINS`: Exact origin(s) where WebAuthn ceremonies execute. Repeat or comma-separate for each origin. The browser matches this against the page's current origin.
- `--cors-origin` / `G8E_ALLOWED_ORIGINS`: Frontend app origin(s) for cross-origin fetch requests. Triggers `SameSite=None` cookie behavior.
- `--public-base-url` / `G8E_PUBLIC_BASE_URL`: Public URL of the gateway (for approval links and host validation). Required for tunneled or multi-region deployments.

See `./g8e gw start --help` for the full list of flags.

### API Configuration

Create a centralized fetch wrapper that includes `credentials: 'include'` for all requests:

```javascript
const API_BASE = 'https://gateway-host:8443';

async function gatewayFetch(path, options = {}) {
  const url = new URL(path, API_BASE);
  const mergedOptions = {
    ...options,
    credentials: 'include', // Always include session cookie
    headers: {
      'Content-Type': 'application/json',
      ...options.headers,
    },
  };
  const response = await fetch(url, mergedOptions);
  if (response.status === 401) {
    // Clear auth state and redirect to login
    window.location.href = '/login';
  }
  return response;
}
```

### Initialize Authentication State on App Mount

1. Call `GET /api/v1/auth/bootstrap/status` to check whether an owner user exists (this does not count existing passkeys).
2. Call `GET /api/v1/users/me` with `credentials: 'include'` to verify the browser has a valid session.
3. If the UI displays or coordinates by session ID, call `GET /api/v1/auth/sessions/me`. The SSE stream derives the session ID from the cookie automatically.

```javascript
async function initAuth() {
  const bootstrapResp = await gatewayFetch('/api/v1/auth/bootstrap/status');
  const bootstrap = await bootstrapResp.json();
  const hasOwner = bootstrap.bootstrapped;

  try {
    const userResp = await gatewayFetch('/api/v1/users/me');
    if (userResp.status === 200) {
      const user = await userResp.json();
      // User is authenticated
      return { authenticated: true, user, hasOwner };
    }
  } catch (e) {
    // Not authenticated
  }
  return { authenticated: false, hasOwner };
}
```

### Implement WebAuthn Registration (First-Time Setup)

1. Confirm `GET /api/v1/auth/bootstrap/status` returns `{ "bootstrapped": false }`.
2. POST `/api/v1/auth/passkeys/console/register/challenge` with:
   ```json
   { "user_name": "Display name", "cli_session_id": "browser" }
   ```
   (Omit `user_id` for initial bootstrap; the gateway creates the user.)
3. Decode the response's `options.publicKey` and call `navigator.credentials.create({ publicKey })`.
4. POST `/api/v1/auth/passkeys/console/register/verify` with the flat-structure attestation response (see [Encoding WebAuthn Responses](#encoding-webauthn-responses)).
5. On success (200), the gateway sets `g8e_web_session_cookie`. Load the current user from `GET /api/v1/users/me`.

The console registration route enforces first-credential-only; once an owner has a passkey, additional credentials require CLI token-gated enrollment.

### Implement WebAuthn Authentication (Returning User)

1. Collect the g8e user ID. POST `/api/v1/auth/passkeys/console/authenticate/challenge` with `{ "user_id": <user_id> }`.
2. If the response contains `success: false` and `needs_setup: true`, the user has no registered passkey; direct them to CLI token-gated enrollment.
3. Decode `options.publicKey`, including `challenge` and each `allowCredentials[].id`, and call `navigator.credentials.get({ publicKey })`.
4. POST `/api/v1/auth/passkeys/console/authenticate/verify` with the flat-structure assertion response (see [Encoding WebAuthn Responses](#encoding-webauthn-responses)).
5. On success (200), the gateway sets `g8e_web_session_cookie`. Load `GET /api/v1/users/me` to establish application state.

### Encoding WebAuthn Responses

The gateway sends WebAuthn values as unpadded base64url strings; the browser API requires `ArrayBuffer` values.

**Decoding for ceremonies:**
- Registration challenge: `base64url-decode(challenge)` → `ArrayBuffer`
- Authentication challenge: `base64url-decode(challenge)` → `ArrayBuffer`
- Credential descriptors: `base64url-decode(id)` → `ArrayBuffer` for each `allowCredentials[].id`

**Encoding for gateway verification:**
- Registration: `{ "id", "rawId": base64url(rawId), "clientDataJSON": base64url(...), "attestationObject": base64url(...), "transports": [...] }`
- Authentication & Approval: `{ "id", "rawId": base64url(rawId), "clientDataJSON": base64url(...), "authenticatorData": base64url(...), "signature": base64url(...), "userHandle": base64url(...) }`

Use a library like `@simplewebauthn/browser` to handle binary conversion; map the output to the flat gateway model.

### Implement Enrollment Token Flow (CLI-Initiated)

The CLI command `g8e auth enroll user` generates a token and opens the gateway console at `#enroll=1&token=<token>`. An external frontend can implement this flow:

1. Read the token from `window.location.hash`.
2. Clear it immediately via `history.replaceState`.
3. POST `/api/v1/auth/passkeys/enrollment/register/challenge` with `{ "enrollment_token": <token> }`. The gateway validates without consuming.
4. Perform the WebAuthn ceremony with `navigator.credentials.create()`.
5. POST `/api/v1/auth/passkeys/enrollment/register/verify` with `{ "enrollment_token": <token>, "attestation_response": {...} }`. The verify endpoint atomically consumes the token before checking the attestation.
6. On success, the gateway sets the web session cookie.

Handle errors:
- **410 Gone**: Token expired (5-minute TTL).
- **409 Conflict**: Token already consumed (one-time-use).
- **401 Unauthorized**: Invalid token.

The optional `/api/v1/auth/enrollment-token/validate` endpoint confirms a token's validity without consuming it; calling it before the challenge is safe but redundant.

### Wire Up SSE Live Event Stream

Connect to `GET /api/v1/sse/stream` using the native `EventSource` API:

```javascript
const source = new EventSource('/api/v1/sse/stream', { withCredentials: true });

source.addEventListener('message', (event) => {
  const envelope = JSON.parse(event.data);
  const { user_id, event } = envelope;
  // Handle event
});

source.addEventListener('error', () => {
  // EventSource reconnects automatically; handle as needed
  console.error('SSE connection error');
});
```

Key points:

- The `web_session_id` and `user_id` are derived from the cookie; do NOT pass them in the URL.
- The gateway replays stored events by default. Use `?since_id=<lastId>` for an explicit cursor; omit the parameter to replay from the beginning.
- `MessageEvent.lastEventId` provides the durable event ID for cursor tracking.
- The nested `event` may be an object or a JSON string; handle both.

For polling fallback (when SSE is blocked by infrastructure), use `GET /api/v1/sse/events?since_id={lastId}&limit={limit}` with `credentials: 'include'`. The response is `{ "events": [...], "count": n }`; each row contains `id`, `event_type`, `payload`, and `created_at`.

### Implement Approval Flows (Suspended Transactions)

Fetch pending transactions via `GET /api/v1/approvals` with `credentials: 'include'`. Display each showing: tool name, transaction hash (truncated, monospace), created date, expiry countdown.

When the user approves a transaction:

1. GET `/api/v1/approvals/{txHash}/challenge` with `credentials: 'include'`.
2. Decode `publicKey` (including `challenge` and `allowCredentials[].id`) and call `navigator.credentials.get({ publicKey })`.
3. POST `/api/v1/approvals/{txHash}/verify` with the flat-structure assertion as the entire JSON body (NOT nested).
4. A 200 response contains the canonical `ActionReceipt` and means execution resumed. Refresh the approval list.
5. A 403 response can contain either an error or an `ActionReceipt` describing rejection. A 404 means the transaction is missing or expired.

### Handle URL Fragments for Approval and Enrollment

On app load, check `window.location.hash` for:

- `#approve={txHash}`: If logged in, trigger the approval flow for this transaction. If not logged in, store it and trigger after login. Clear the fragment with `history.replaceState`.
- `#enroll=1&token={enrollmentToken}`: If the frontend supports token-gated enrollment, read and clear the token immediately, then post to the enrollment challenge endpoint.

Transaction hashes are not credentials; the frontend can retain or remove approval fragments per its routing behavior.

### Implement Passkey Management

**List passkeys:**

```javascript
const resp = await gatewayFetch('/api/v1/auth/passkeys');
const passkeys = await resp.json();
// Display: credential ID (truncated), creation date, last-used date
```

**Revoke a passkey:**

```javascript
await gatewayFetch(`/api/v1/auth/passkeys/{credentialId}`, { method: 'DELETE' });
// Use a confirmation dialog before revoking
```

**Register another passkey:**

Run `g8e auth enroll user`. The CLI opens the embedded gateway console, which completes token-gated registration. The public console registration endpoints reject users who already have a passkey.

### Verify the Integration Manually

- [ ] **CORS headers**: Open browser DevTools, Network tab. Make any API call and confirm `Access-Control-Allow-Origin` reflects your frontend origin and `Access-Control-Allow-Credentials: true` is present.
- [ ] **Preflight OPTIONS**: Confirm OPTIONS requests succeed with 204 No Content before actual POST requests.
- [ ] **Passkey registration**: Trigger registration and confirm the browser's WebAuthn dialog appears with the correct RP ID.
- [ ] **Passkey authentication**: Trigger authentication and confirm the WebAuthn dialog appears and login succeeds.
- [ ] **Session cookie**: After login, check DevTools for the HttpOnly, Secure `g8e_web_session_cookie`. It uses `SameSite=None` when any CORS origin is configured and `SameSite=Lax` otherwise.
- [ ] **Authenticated API calls**: Confirm `GET /api/v1/users/me` returns user data (not 401) after login.
- [ ] **SSE stream**: Connect to the SSE stream and confirm live events appear.
- [ ] **Approvals**: If a suspended transaction exists, confirm the approval flow triggers WebAuthn and the transaction is approved.
- [ ] **Enrollment token**: Navigate to `#enroll=1&token={token}` and confirm registration uses the token-gated challenge and verify endpoints without first calling `/api/v1/auth/enrollment-token/validate`.
- [ ] **URL hash approval**: Navigate to `#approve={txHash}` and confirm auto-approval flow triggers.
- [ ] **Logout**: Sign out via `POST /api/v1/auth/logout` and confirm redirect to login page and cookie cleared.

## Anti-patterns

- Not including `credentials: 'include'` on authenticated API calls, causing CORS cookie delivery to fail and 401 responses.
- Hard-coding the gateway URL as a path relative to the frontend app (e.g., `/api/`) instead of an absolute HTTPS origin; the gateway is a separate service and browser CORS blocks same-path requests.
- Ignoring 401 responses from session-authenticated routes and continuing to make requests; re-authenticate immediately.
- Treating `needs_setup: true` on an authentication challenge as a recoverable error; direct the user to CLI token-gated enrollment instead.
- Reusing enrollment tokens across failed registration attempts; each consume attempt invalidates the token and requires `g8e auth enroll user` to generate a new one.
- Relying on WebAuthn `publicKey.timeout` without implementing a UX timeout message; browsers may not surface timeout errors visibly.
- Storing or relaying WebAuthn challenges in localStorage or URL parameters across page reloads; challenges expire and browsers reject reuse.
- Hardcoding flag values or API paths in the frontend instead of deriving them from gateway help text (`./g8e gw start --help`) or OpenAPI spec (`/swagger/doc.json`).
- Calling `/api/v1/auth/enrollment-token/validate` as a mandatory prerequisite; the registration challenge validates the token and the verify endpoint consumes it, making a separate validation call redundant.

## Links out

- [Build a g8e-Compatible Frontend](./build_frontend.md) — Full reference for building a frontend from scratch.
- [Connect a Lovable App](./lovable.md) — Connect a browser-hosted Lovable app directly to a local Gateway.
- [Cloudflare Tunnel Integration](./cloudflare_tunnel.md) — Expose the gateway via a public tunnel.
- [Connect Apps to Gateway](./connect_apps_to_gateway.md) — Application-to-gateway mTLS and token-based connectivity.
- [Authentication & Authorization](../architecture/auth.md) — WebAuthn passkey authentication architecture.
- [Gateway Architecture](../architecture/gateway.md) — Gateway service architecture and posture definitions.
- [SSE Streaming](../architecture/sse.md) — SSE push, poll, and stream endpoints for real-time events.
