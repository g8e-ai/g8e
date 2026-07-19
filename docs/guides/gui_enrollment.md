---
title: GUI Frontend Enrollment
parent: Guides
---

# GUI Frontend Enrollment

Last Updated: 2026-07-19
Version: v1.5.8

---

## Overview

The `g8e gui` command family enrolls external frontend applications with the g8e Gateway. Enrollment validates that the gateway is running with the correct CORS and passkey RP configuration for the frontend origin, persists the origin to a local enrollment file, and outputs a TypeScript configuration snippet for the frontend developer.

This guide covers the general enrollment workflow, gateway-side configuration, API reference, WebAuthn flow requirements, and SSE streaming for any frontend application (custom React, Vue, vanilla JS, or hosted platforms like Lovable). For Lovable-specific integration, see [Lovable Frontend Integration](./lovable.md).

### Architecture

```
[Frontend App] → https://gateway-host:8443 → [g8e Gateway]
      ↑                                          ↓
 credentials: 'include'              WebAuthn + Session Cookie + SSE
```

---

## Prerequisites

- g8e Gateway running and healthy
- Frontend application served from a known origin (e.g., `https://your-app.example.com`, `http://localhost:3003`)
- Gateway started with `--cors-origin` and `--passkey-rp-origin` flags matching the frontend origin

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

---

## GUI Enrollment Commands

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
| `GET` | `/api/v1/sse/stream?web_session_id={id}` | SSE stream for live audit events |
| `GET` | `/api/v1/sse/events?web_session_id={id}&since_id={n}` | Poll SSE events |

---

## WebAuthn Flow Requirements

The frontend must implement three WebAuthn ceremonies using the browser's `navigator.credentials` API. The `@simplewebauthn/browser` library handles base64url conversions automatically; if using raw `navigator.credentials`, manual base64url encoding/decoding is required.

### Base64url Encoding

The gateway sends and receives WebAuthn challenge data as base64url strings. The browser's WebAuthn API requires ArrayBuffers. The frontend must convert between base64url strings and ArrayBuffers for:

- Challenge fields
- User ID fields (registration only)
- Credential ID fields (`excludeCredentials`, `allowCredentials`)
- Assertion response fields (`rawId`, `clientDataJSON`, `authenticatorData`, `signature`, `userHandle`, `attestationObject`)

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

---

## SSE Live Audit Stream

### Connection

- Connect to `GET /api/v1/sse/stream?web_session_id={id}` using `EventSource` with `withCredentials: true`.
- The `web_session_id` comes from `GET /api/v1/auth/sessions/me` after login.
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

If `EventSource` does not send cookies cross-origin, fall back to polling `GET /api/v1/sse/events?web_session_id={id}&since_id={lastId}` every 2 seconds.

### WebSocket Note

The gateway also exposes a WebSocket pub/sub endpoint at `/ws/pubsub`, but it requires mTLS authentication and is not available to browser clients. Use SSE for all browser-based real-time telemetry.

---

## TypeScript Types

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
  local_os_user?: { domain?: string; username?: string; uid?: string; gid?: string; sid?: string };
  webauthn_user_id?: string;
}

interface UserMeResponse {
  success: boolean;
  user: User;
}

// Passkey
interface PasskeyCredential {
  id: string;
  public_key: string;
  attestation_type: string;
  transport?: string[];
  authenticator: {
    aaguid: string;
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

## Frontend Integration Checklist

- [ ] **CORS headers**: Open browser DevTools, Network tab. Make any API call and confirm `Access-Control-Allow-Origin` reflects the frontend origin and `Access-Control-Allow-Credentials: true` is present.
- [ ] **Preflight OPTIONS**: Confirm OPTIONS requests succeed with 200/204 status before actual POST requests.
- [ ] **Passkey registration**: Trigger registration and confirm the browser's WebAuthn dialog appears with the correct RP ID.
- [ ] **Passkey authentication**: Trigger authentication and confirm the WebAuthn dialog appears and login succeeds.
- [ ] **Session cookie**: After login, check DevTools, Application, Cookies for `g8e_web_session_cookie` with `SameSite=None` and `Secure` attributes.
- [ ] **Authenticated API calls**: Confirm `GET /api/v1/users/me` returns user data (not 401) after login.
- [ ] **SSE stream**: Connect to the SSE stream and confirm live events appear.
- [ ] **Approvals**: If a suspended transaction exists, confirm the approval flow triggers WebAuthn and the transaction is approved.
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

---

## See Also

- [Lovable Frontend Integration](./lovable.md) - Lovable-specific integration guide
- [Cloudflare Tunnel Integration](./cloudflare_tunnel.md) - Expose the gateway via a public tunnel
- [Connect Apps to Gateway](./connect_apps_to_gateway.md) - General application connectivity patterns
- [Architecture: Auth](../architecture/auth.md) - WebAuthn passkey authentication architecture
- [Architecture: Gateway](../architecture/gateway.md) - Gateway service architecture
