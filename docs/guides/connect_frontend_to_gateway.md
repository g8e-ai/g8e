---
title: Connect Frontend to Gateway
parent: Guides
---

# Connect an Existing Frontend to g8e Gateway

Last Updated: 2026-07-25
Version: v1.6.3

---

## Overview

This guide walks through connecting an **existing** frontend UI to the g8e Gateway. If you are building a frontend from scratch, see [Build a g8e-Compatible Frontend](./build_frontend.md) for the full reference. This guide focuses on the integration steps required to wire an already-built web application into the g8e platform: gateway configuration, enrollment, authentication wiring, SSE streaming, and approval flows.

### Architecture

```
[Existing Frontend App] → https://gateway-host:8443 → [g8e Gateway]
      ↑                                              ↓
 credentials: 'include'                  WebAuthn + Session Cookie + SSE
```

The frontend communicates with the g8e Gateway over HTTPS. Authentication is via WebAuthn passkeys (no passwords, no API keys). The gateway issues an `HttpOnly` session cookie after successful passkey verification. All authenticated API calls must include `credentials: 'include'` so the cookie is sent cross-origin. Real-time telemetry is delivered via Server-Sent Events (SSE), not WebSockets (the WebSocket endpoint requires mTLS and is not available to browsers).

---

## Prerequisites

- g8e Gateway running and healthy (`./g8e gw start`)
- Existing frontend application served from a known origin (e.g., `https://your-app.example.com`, `http://localhost:3003`)
- Browser supports WebAuthn (all modern Chrome, Firefox, Safari, Edge)

---

## Step 1: Configure the Gateway for Your Frontend Origin

The gateway must be started with CORS and passkey RP settings that match your frontend app's origin. The passkey RP ID must match the domain where WebAuthn ceremonies are performed, which is your frontend app's origin, not the gateway's domain.

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

- **`--passkey-rp-id`**: The WebAuthn Relying Party ID. Use your frontend app's registrable domain so passkeys work across subdomains.
- **`--passkey-rp-origin`**: The origin where WebAuthn ceremonies are performed (your frontend app URL). Repeat for each origin.
- **`--cors-origin`**: Your frontend app origin. Allows cross-origin requests with credentials. Repeat for each origin (preview URLs, production URLs, custom domains).
- **`--public-base-url`**: The public URL of the gateway (e.g., a tunnel hostname). Used for approval redirect links and host validation.

Add every origin your frontend may use (preview URLs, production URLs, custom domains). The gateway reflects exact-match origins in CORS headers and sets `SameSite=None` on session cookies when `AllowedOrigins` is non-empty.

### How CORS Works

When `AllowedOrigins` is non-empty, the gateway's CORS middleware:

1. Checks the request `Origin` header against the allowed set (case-insensitive, trailing slashes trimmed).
2. If matched, sets `Access-Control-Allow-Origin` to the exact origin and `Access-Control-Allow-Credentials: true`.
3. Handles `OPTIONS` preflight requests with `204 No Content` and the appropriate headers.
4. Adds `Vary: Origin` to all responses so caches respect per-origin differences.
5. When `AllowedOrigins` is empty (same-origin only), the middleware is a pass-through and no CORS headers are set.

### Session Cookie Behavior

The gateway sets a `g8e_web_session_cookie` cookie after successful passkey registration or authentication:

- **`HttpOnly`**: Always true (prevents JavaScript access).
- **`Secure`**: Always true (requires HTTPS).
- **`SameSite=Lax`**: Default when no cross-origin origins are configured.
- **`SameSite=None`**: Automatically set when `AllowedOrigins` is non-empty (required for cross-origin cookie delivery).

The cookie has a 24-hour TTL. The gateway validates the cookie on every authenticated request.

---

## Step 2: Enroll Your Frontend Origin

Enroll your frontend with the gateway to validate CORS and generate a configuration snippet:

```bash
g8e gui enroll --origin https://your-app.example.com
```

Optional flags:

- `--passkey-rp-id`: Override the RP ID (defaults to the origin's hostname)
- `--passkey-rp-name`: Override the RP display name (default: `g8e`)
- `--public-base-url`: Verify the gateway is reachable at a public URL (e.g., tunnel hostname)

This command:

1. Validates the origin URL
2. Sends a CORS preflight to the running gateway to verify the origin is allowed
3. Verifies the gateway is reachable at the `--public-base-url` (if provided)
4. Persists the origin to `.g8e/gui_enrollments.json`
5. Outputs a TypeScript configuration snippet with `API_BASE_URL`, `PASSKEY_RP_ID`, `PASSKEY_RP_NAME`, `apiFetch()` helper, `connectSSE()` helper, and key endpoint paths

**Copy the outputted snippet** into your frontend project as the starting point for API integration.

### Verify Enrollment

```bash
g8e gui verify --origin https://your-app.example.com
```

This prints a verification checklist covering CORS headers, passkey registration, passkey authentication, session cookie attributes, SSE stream connectivity, and authenticated API calls.

---

## Step 3: Add the API Configuration

Take the TypeScript snippet from `g8e gui enroll` and add it to your frontend project. The key elements are:

### API Base URL and Fetch Wrapper

Every API call must include `credentials: 'include'` so the session cookie is sent cross-origin. Create a centralized fetch wrapper:

```typescript
const API_BASE_URL = "https://gateway-host:8443";

export async function apiFetch(path: string, options: RequestInit = {}): Promise<Response> {
  return fetch(`${API_BASE_URL}${path}`, {
    ...options,
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      ...options.headers,
    },
  });
}
```

### Key Endpoint Paths

| Method | Path | Auth Required |
|--------|------|---------------|
| `GET` | `/api/v1/health` | No |
| `GET` | `/api/v1/auth/bootstrap/status` | No |
| `POST` | `/api/v1/auth/passkeys/console/register/challenge` | No |
| `POST` | `/api/v1/auth/passkeys/console/register/verify` | No |
| `POST` | `/api/v1/auth/passkeys/console/authenticate/challenge` | No |
| `POST` | `/api/v1/auth/passkeys/console/authenticate/verify` | No |
| `POST` | `/api/v1/auth/logout` | No |
| `POST` | `/api/v1/auth/enrollment-token/validate` | No |
| `GET` | `/api/v1/users/me` | Yes |
| `GET` | `/api/v1/auth/sessions/me` | Yes |
| `GET` | `/api/v1/auth/passkeys` | Yes |
| `DELETE` | `/api/v1/auth/passkeys/{credentialId}` | Yes |
| `GET` | `/api/v1/approvals` | Yes |
| `GET` | `/api/v1/approvals/{txHash}/challenge` | Yes |
| `POST` | `/api/v1/approvals/{txHash}/verify` | Yes |
| `GET` | `/api/v1/sse/stream?web_session_id={id}` | Yes |
| `GET` | `/api/v1/sse/events?web_session_id={id}&since_id={n}` | Yes |

The gateway also serves a full OpenAPI/Swagger specification at `/swagger/doc.json` (and browsable UI at `/swagger/`). Use this to auto-discover the full API surface and schemas.

---

## Step 4: Wire Up WebAuthn Authentication

Your frontend must implement WebAuthn passkey flows using the browser's `navigator.credentials` API. The `@simplewebauthn/browser` library handles base64url conversions automatically; if using raw `navigator.credentials`, manual base64url encoding/decoding is required.

### Registration Flow (First-Time Setup)

1. POST to `/api/v1/auth/passkeys/console/register/challenge` with JSON body `{ user_id, user_name, cli_session_id: "browser" }`. If no passkeys exist on the gateway, omit `user_id` to auto-create a user.
2. Decode the returned `options.publicKey` challenge and user ID from base64url to ArrayBuffers.
3. Call `navigator.credentials.create({ publicKey: decodedOptions })` to trigger the browser's passkey enrollment prompt.
4. Encode the credential response fields (`id`, `rawId`, `clientDataJSON`, `attestationObject`, `transports`) as base64url.
5. POST to `/api/v1/auth/passkeys/console/register/verify` with JSON body `{ user_id, cli_session_id: "browser", attestation_response: { ...encodedFields } }`.
6. On success, the gateway sets a `g8e_web_session_cookie` session cookie. Transition to the authenticated state.

### Authentication Flow (Returning User)

1. POST to `/api/v1/auth/passkeys/console/authenticate/challenge` with JSON body `{ user_id }`.
2. If the response has `success: false` and `needs_setup: true`, no passkey is registered. Show the registration form instead.
3. Decode the returned `options.publicKey` challenge and `allowCredentials` from base64url to ArrayBuffers.
4. Call `navigator.credentials.get({ publicKey: decodedOptions })` to trigger the browser's passkey authentication prompt.
5. Encode the assertion response fields (`id`, `rawId`, `clientDataJSON`, `authenticatorData`, `signature`, `userHandle`) as base64url.
6. POST to `/api/v1/auth/passkeys/console/authenticate/verify` with JSON body `{ user_id, assertion_response: { ...encodedFields } }`.
7. On success, the gateway sets a `g8e_web_session_cookie` session cookie. Transition to the authenticated state.

### Auth Context Initialization

On app mount, check auth state:

1. Call `GET /api/v1/auth/bootstrap/status` to check if any passkey is registered
2. Call `GET /api/v1/users/me` (with `credentials: 'include'`) to check if already logged in
3. If logged in, call `GET /api/v1/auth/sessions/me` to get the web session ID (needed for SSE)

### Enrollment Token Flow (CLI-Initiated Registration)

When the CLI initiates a passkey enrollment from the terminal, it opens the browser with `#register=1&token=<token>`. Your frontend must:

1. Read the token from the URL hash (`window.location.hash`).
2. POST to `/api/v1/auth/enrollment-token/validate` with JSON body `{ token }`.
3. On success, receive `{ user_id, cli_session_id }` from the gateway.
4. Populate hidden form fields with the returned `user_id` and `cli_session_id`.
5. Auto-trigger the passkey registration flow.
6. Immediately clear the token from the URL via `history.replaceState`.

Handle error responses:
- **`410 Gone`**: Token has expired (5-minute TTL).
- **`409 Conflict`**: Token has already been used (one-time-use).
- **`401 Unauthorized`**: Invalid token.

---

## Step 5: Wire Up the SSE Live Audit Stream

### Connection

- Connect to `GET /api/v1/sse/stream?web_session_id={id}` using `EventSource` with `withCredentials: true`.
- The `web_session_id` comes from `GET /api/v1/auth/sessions/me` after login.

```typescript
export function connectSSE(
  webSessionId: string,
  onEvent: (data: string) => void,
  onStatusChange: (status: "connected" | "connecting" | "disconnected") => void
): EventSource {
  const url = `${API_BASE_URL}/api/v1/sse/stream?web_session_id=${webSessionId}`;
  const source = new EventSource(url, { withCredentials: true });
  onStatusChange("connecting");

  source.onopen = () => onStatusChange("connected");
  source.onerror = () => {
    onStatusChange("disconnected");
    // Auto-reconnect after 3 seconds
    setTimeout(() => connectSSE(webSessionId, onEvent, onStatusChange), 3000);
  };
  source.onmessage = (e) => onEvent(e.data);

  return source;
}
```

### Event Handling

- Parse incoming SSE messages as JSON. Each message may contain a nested `event` field that itself is JSON.
- Extract: event type, timestamp, event ID, and raw payload.
- Display events in a scrollable log with color-coded type badges.
- Cap the in-memory event list at 500 entries (drop oldest).
- Provide a filter input (case-insensitive filter by event type), auto-scroll checkbox, clear button, and event count display.

### Polling Fallback

If `EventSource` does not send cookies cross-origin, fall back to polling `GET /api/v1/sse/events?web_session_id={id}&since_id={lastId}` every 2 seconds.

### WebSocket Note

The gateway also exposes a WebSocket pub/sub endpoint at `/ws/pubsub`, but it requires mTLS authentication and is not available to browser clients. Use SSE for all browser-based real-time telemetry.

---

## Step 6: Wire Up Approval Flows

### Pending Approvals

Fetch pending suspended transactions:

```typescript
const res = await apiFetch("/api/v1/approvals");
const approvals = await res.json();
```

Display each transaction showing: tool name, transaction hash (truncated, monospace), created date, expiry countdown.

### Approval Flow (Suspended Transaction)

When user clicks "Approve" on a transaction:

1. `GET /api/v1/approvals/{txHash}/challenge` (with `credentials: 'include'`). Returns WebAuthn `PublicKeyCredentialRequestOptions`.
2. Decode the challenge and `allowCredentials` from base64url to ArrayBuffers.
3. Call `navigator.credentials.get({ publicKey: decodedOptions })` to trigger the browser's passkey prompt.
4. Encode the assertion response fields (`id`, `rawId`, `clientDataJSON`, `authenticatorData`, `signature`, `userHandle`) as base64url.
5. `POST /api/v1/approvals/{txHash}/verify` with the encoded assertion response as the JSON body.
6. On success, the transaction resumes execution. Refresh the approvals list.
7. On failure (403), show the error message. The transaction remains suspended.

### URL Hash Handling

On app load, check `window.location.hash` for:

- **`#approve={txHash}`**: If user is logged in, auto-trigger the approval flow for this transaction. If not logged in, store it and trigger after login.
- **`#register=1&token={enrollmentToken}`**: Validate the enrollment token, then auto-trigger passkey registration.

After processing, clean the URL with `history.replaceState`.

---

## Step 7: Add Passkey Management

### List Passkeys

```typescript
const res = await apiFetch("/api/v1/auth/passkeys");
const passkeys = await res.json();
```

Display each passkey with credential ID (truncated, monospace), creation date, last used date.

### Revoke a Passkey

```typescript
await apiFetch(`/api/v1/auth/passkeys/${credentialId}`, { method: "DELETE" });
```

Use a confirmation dialog before revoking.

### Register New Passkey

Trigger the same registration flow as Step 4 from an authenticated state.

---

## Step 8: Handle Errors and Edge Cases

- **401 responses**: Clear user state and redirect to login.
- **`needs_setup: true`** on authenticate challenge: Show registration form automatically.
- **Enrollment token validation**: Handle 410 (expired) and 409 (already used) with specific error messages.
- **WebAuthn API not available**: Show a browser compatibility warning.
- **Network errors**: Show a retry-able error state.

---

## Step 9: Verify the Integration

Run the verification command:

```bash
g8e gui verify --origin https://your-app.example.com
```

Then manually verify in the browser:

- [ ] **CORS headers**: Open browser DevTools, Network tab. Make any API call and confirm `Access-Control-Allow-Origin` reflects your frontend origin and `Access-Control-Allow-Credentials: true` is present.
- [ ] **Preflight OPTIONS**: Confirm OPTIONS requests succeed with 200/204 status before actual POST requests.
- [ ] **Passkey registration**: Trigger registration and confirm the browser's WebAuthn dialog appears with the correct RP ID.
- [ ] **Passkey authentication**: Trigger authentication and confirm the WebAuthn dialog appears and login succeeds.
- [ ] **Session cookie**: After login, check DevTools > Application > Cookies for `g8e_web_session_cookie` with `SameSite=None` and `Secure` attributes.
- [ ] **Authenticated API calls**: Confirm `GET /api/v1/users/me` returns user data (not 401) after login.
- [ ] **SSE stream**: Connect to the SSE stream and confirm live events appear.
- [ ] **Approvals**: If a suspended transaction exists, confirm the approval flow triggers WebAuthn and the transaction is approved.
- [ ] **Enrollment token**: Navigate to `#register=1&token={token}` and confirm the token is validated and registration auto-triggers.
- [ ] **URL hash approval**: Navigate to `#approve={txHash}` and confirm auto-approval flow triggers.
- [ ] **Logout**: Sign out and confirm redirect to login page and cookie cleared.

---

## Troubleshooting

### CORS Errors in Browser Console

**Symptom**: `Access-Control-Allow-Origin` header missing or does not match your frontend origin.

**Cause**: The gateway was not started with `--cors-origin` matching your frontend origin.

**Fix**: Restart the gateway with the correct flag:

```bash
./g8e gw start --cors-origin https://your-app.example.com --passkey-rp-origin https://your-app.example.com
```

Then run `g8e gui enroll --origin https://your-app.example.com` to verify.

### Passkey RP Mismatch

**Symptom**: WebAuthn ceremony fails with "RP ID is not a valid domain" or similar.

**Cause**: The `--passkey-rp-id` does not match your frontend app's registrable domain.

**Fix**: Set `--passkey-rp-id` to your frontend app's domain (e.g., `example.com` for `https://app.example.com`). The RP ID must be a registrable domain suffix of the current page's origin.

### SSE Connection Refused

**Symptom**: `EventSource` fails to connect or returns 401.

**Cause**: No authenticated session, or `withCredentials: true` not set on `EventSource`.

**Fix**: Authenticate first via the passkey flow. Ensure `new EventSource(url, { withCredentials: true })`. Verify the `web_session_id` is valid via `GET /api/v1/auth/sessions/me`.

### Session Cookie Not Sent Cross-Origin

**Symptom**: Authenticated API calls return 401 despite being logged in.

**Cause**: `credentials: 'include'` not set on `fetch` calls, or the gateway is not configured with `AllowedOrigins`.

**Fix**: Ensure every `fetch` call includes `credentials: 'include'`. Verify the gateway was started with `--cors-origin` for your frontend origin, which triggers `SameSite=None` on session cookies.

### Enrollment Token Errors

**Symptom**: Registration via `#register=1&token={token}` fails.

**Cause**: Token expired (5-minute TTL), already used (one-time-use), or invalid.

**Fix**: Generate a new enrollment token from the CLI (`g8e auth enroll` or `g8e passkey bootstrap`). Handle 410 (expired), 409 (already used), and 401 (invalid) with specific user-facing error messages.

---

## See Also

- [Build a g8e-Compatible Frontend](./build_frontend.md) - Full reference for building a frontend from scratch
- [Lovable Frontend Integration](./lovable.md) - Lovable-specific integration guide with AI agent prompt
- [Cloudflare Tunnel Integration](./cloudflare_tunnel.md) - Expose the gateway via a public tunnel
- [Connect Apps to Gateway](./connect_apps_to_gateway.md) - General application connectivity patterns
- [Architecture: Auth](../architecture/auth.md) - WebAuthn passkey authentication architecture
- [Architecture: Gateway](../architecture/gateway.md) - Gateway service architecture
