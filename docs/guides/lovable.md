---
title: Lovable Frontend Integration
parent: Guides
---

# Lovable Frontend Integration

Last Updated: 2026-07-11
Version: v1.3.11

---

## Overview

[Lovable](https://lovable.dev) is an AI-powered frontend development platform that generates React + TypeScript + TailwindCSS applications from natural-language prompts. This guide provides detailed instructions for building a g8e Governance Console UI with Lovable, connecting it to a g8e Gateway backend via Cloudflare Tunnels.

The guide is structured as a prompt you can paste directly into your Lovable AI agent. It covers API integration, WebAuthn passkey authentication, cross-origin cookie handling, SSE live audit streaming, and the full component architecture.

### Steps at a Glance

1. **[Configure the Cloudflare Tunnel](#cloudflare-tunnel)** — Expose the local g8e Gateway to the internet via `console.g8e.ai`
2. **[Configure the Gateway](#gateway-side-configuration)** — Set CORS origins, passkey RP ID/name/origins, and public base URL
3. **[Review the API Reference](#api-reference)** — Understand the public and authenticated endpoints the Lovable app will call
4. **[Define TypeScript Types](#typescript-types)** — Mirror the g8e gateway's data models in the frontend
5. **[Build Pages and Components](#pages-and-components)** — AuthContext, Login, Dashboard, ApprovalFlow, URL hash handling
6. **[Implement WebAuthn Flows](#webauthn-implementation)** — Registration, authentication, and approval using `navigator.credentials`
7. **[Implement SSE Audit Stream](#sse-live-audit-stream)** — Live event streaming with auto-reconnect and polling fallback
8. **[Apply UI/UX Guidelines](#uiux-guidelines)** — Dark theme, monospace hashes, toast notifications, responsive layout
9. **[Handle Errors](#error-handling)** — 401 redirects, `needs_setup`, enrollment token errors, WebAuthn availability
10. **[Verify the Integration](#verification-checklist)** — CORS headers, preflight, passkey flows, cookie attributes, SSE, approvals

### Architecture

```
[Lovable App (React)] → https://console.g8e.ai → [Cloudflare Edge] → [cloudflared tunnel] → [g8e Gateway :8443]
         ↑                                                                                      ↓
    credentials: 'include'                                                          WebAuthn + Session Cookie + SSE
```

## 1. Cloudflare Tunnel

A Cloudflare Tunnel securely exposes the local g8e Gateway to the internet without opening firewall ports. `cloudflared` maintains an outbound-only encrypted connection to Cloudflare's edge, which terminates TLS with a public CA certificate and forwards traffic to the gateway's HTTPS listener on `localhost:8443`.

Key points for this integration:

- **Hostname**: `console.g8e.ai` routes through the tunnel to `https://localhost:8443`
- **Origin TLS**: The gateway uses a self-signed internal certificate; `cloudflared` connects with `originServerName: g8e.local` and the gateway's CA bundle
- **HTTP/2**: The tunnel uses HTTP/2 to the origin for multiplexed streaming (important for SSE)
- **Public URL**: The `--public-base-url` flag on the gateway must match the tunnel hostname (`https://console.g8e.ai`) so that passkey RP origins and approval links resolve correctly

#### Create the Tunnel

```bash
# Authenticate with your Cloudflare account
cloudflared tunnel login

# Create a named tunnel
cloudflared tunnel create g8e

# Route DNS to the tunnel (creates CNAME console.g8e.ai → <tunnel-id>.cfargotunnel.com)
cloudflared tunnel route dns g8e console.g8e.ai
```

Note the tunnel ID from the output. The credentials file is saved to `~/.cloudflared/<tunnel-id>.json`.

#### Configure `config.yml`

Create or edit `~/.cloudflared/config.yml`:

```yaml
tunnel: <your-tunnel-id>
credentials-file: ~/.cloudflared/<your-tunnel-id>.json

ingress:
  - hostname: console.g8e.ai
    service: https://localhost:8443
    originRequest:
      originServerName: g8e.local
      originCaPool: ./.g8e/pki/g8eg-ca-bundle.pem
      http2Origin: true
  - service: http_status:404
```

#### Run the Tunnel

```bash
# Start the tunnel (in a separate terminal from the gateway)
cloudflared tunnel run g8e

# Verify the tunnel is connected
cloudflared tunnel info g8e
```

#### Verify End-to-End

```bash
# Check the health endpoint through the tunnel
curl -s https://console.g8e.ai/api/v1/health
```

Expected response:

```json
{"status":"ok","mode":"gateway","version":"...","pid":12345,"governance_ready":true,"state_merkle_root":"..."}
```

> For full setup instructions — running `cloudflared` as a systemd service, optional Cloudflare Access policies, and troubleshooting — see the [Cloudflare Tunnel Integration guide](./cloudflare_tunnel.md).

### Prerequisites

- g8e Gateway running with CORS and passkey RP configured (see [Cloudflare Tunnel Integration](./cloudflare_tunnel.md))
- `console.g8e.ai` resolving through Cloudflare Tunnel to `localhost:8443`
- A [Lovable](https://lovable.dev) account

---

## 2. Gateway-Side Configuration

The gateway must be started with CORS and passkey RP settings that match your Lovable app's origin:

```bash
./g8e gw start \
  --public-base-url https://console.g8e.ai \
  --passkey-rp-id console.g8e.ai \
  --passkey-rp-name "g8e Console" \
  --passkey-rp-origin https://console.g8e.ai \
  --cors-origin https://your-app.lovable.app \
  --cors-origin https://your-custom-domain.com
```

Or via environment variables:

```bash
export G8E_PUBLIC_BASE_URL=https://console.g8e.ai
export G8E_PASSKEY_RP_ID=console.g8e.ai
export G8E_PASSKEY_RP_NAME="g8e Console"
export G8E_PASSKEY_RP_ORIGINS=https://console.g8e.ai
export G8E_ALLOWED_ORIGINS=https://your-app.lovable.app,https://your-custom-domain.com
```

> **Important:** Add every origin your Lovable app may use (preview URLs, production URLs, custom domains). The gateway reflects exact-match origins in CORS headers and sets `SameSite=None` on session cookies when `AllowedOrigins` is non-empty.

---

## Lovable AI Agent Prompt

Paste the following sections into your Lovable AI agent as the build instructions.

### Project Overview

Build a **g8e Governance Console** — a React + TypeScript + TailwindCSS single-page application that serves as the web UI for the g8e platform. The app communicates with the g8e gateway API at `https://console.g8e.ai` via cross-origin fetch requests with credentials.

### Critical Configuration

#### API Base URL

```typescript
const API_BASE_URL = 'https://console.g8e.ai';
```

#### Fetch Requirements

**Every** `fetch` call to the API MUST include `credentials: 'include'`. This is non-negotiable — the g8e gateway uses a `web_session` cookie (HttpOnly, Secure, SameSite=None) for browser authentication. Without `credentials: 'include'`, the cookie is not sent cross-origin and all authenticated requests will return 401.

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

## 3. API Reference

All paths are relative to `API_BASE_URL`. All authenticated routes require `credentials: 'include'`.

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

## 4. TypeScript Types

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

## 5. Pages and Components

### 5.1 Auth Context (`src/contexts/AuthContext.tsx`)

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

### 5.2 Login Page (`src/pages/Login.tsx`)

Two modes based on `bootstrapped` state:

**If NOT bootstrapped (first-time setup):**

- Show a "Register Passkey" card with a display name input
- Button: "Enroll Passkey" → calls `registerPasskey()`

**If bootstrapped (returning user):**

- Show a "Sign In" card with a User ID input
- Button: "Sign In with Passkey" → calls `authenticatePasskey()`
- Secondary button: "Register New Passkey" → reveals registration form

### 5.3 Dashboard (`src/pages/Dashboard.tsx`)

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

### 5.4 Approval Flow (`src/components/ApprovalFlow.tsx`)

When user clicks "Approve" on a transaction:

1. `GET /api/v1/approvals/{txHash}/challenge` — returns WebAuthn PublicKeyCredentialRequestOptions
2. Decode the challenge and allowedCredentials from base64url to ArrayBuffers
3. Call `navigator.credentials.get({ publicKey: decodedOptions })`
4. Encode the assertion response fields to base64url
5. `POST /api/v1/approvals/{txHash}/verify` with the encoded assertion
6. Show success or error message
7. Refresh the approvals list

### 5.5 URL Hash Handling

On app load, check `window.location.hash` for:

- `#approve={txHash}` — if user is logged in, auto-trigger the approval flow for this transaction. If not logged in, store it and trigger after login.
- `#register=1&token={enrollmentToken}` — validate the enrollment token via `POST /api/v1/auth/enrollment-token/validate`, then auto-trigger passkey registration with the returned user ID and CLI session ID.

After processing, clean the URL with `history.replaceState`.

---

## 6. WebAuthn Implementation

### 6.1 Base64url Helpers

```typescript
function base64urlToBuffer(base64url: string): ArrayBuffer {
  const base64 = base64url.replace(/-/g, '+').replace(/_/g, '/');
  const padded = base64.padEnd(base64.length + (4 - base64.length % 4) % 4, '=');
  const binary = atob(padded);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  return bytes.buffer;
}

function bufferToBase64url(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer);
  let binary = '';
  for (let i = 0; i < bytes.length; i++) binary += String.fromCharCode(bytes[i]);
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '');
}
```

### 6.2 Registration Flow

```typescript
async function registerPasskey(userId: string, displayName: string): Promise<boolean> {
  // 1. Get challenge from gateway
  const challengeRes = await fetch(`${API_BASE_URL}/api/v1/auth/passkeys/console/register/challenge`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      user_id: userId,
      user_name: displayName || userId,
      cli_session_id: 'browser',
    }),
  });
  const challengeData = await challengeRes.json();
  if (!challengeData.success) throw new Error(challengeData.error || 'Challenge failed');

  // 2. Decode options for browser WebAuthn API
  const pk = challengeData.options.publicKey;
  pk.challenge = base64urlToBuffer(pk.challenge);
  pk.user.id = base64urlToBuffer(pk.user.id);
  if (pk.excludeCredentials) {
    pk.excludeCredentials.forEach((c: any) => { c.id = base64urlToBuffer(c.id); });
  }

  // 3. Browser creates credential (triggers platform authenticator prompt)
  const credential = await navigator.credentials.create({ publicKey: pk });

  // 4. Encode response and send to gateway for verification
  const verifyRes = await fetch(`${API_BASE_URL}/api/v1/auth/passkeys/console/register/verify`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      user_id: challengeData.user_id || userId,
      cli_session_id: 'browser',
      attestation_response: {
        id: (credential as any).id,
        rawId: bufferToBase64url((credential as any).rawId),
        clientDataJSON: bufferToBase64url((credential as any).response.clientDataJSON),
        attestationObject: bufferToBase64url((credential as any).response.attestationObject),
        transports: (credential as any).response.getTransports ? (credential as any).response.getTransports() : [],
      },
    }),
  });
  const verifyData = await verifyRes.json();
  return verifyData.success;
}
```

### 6.3 Authentication Flow

```typescript
async function authenticatePasskey(userId: string): Promise<boolean> {
  // 1. Get challenge
  const challengeRes = await fetch(`${API_BASE_URL}/api/v1/auth/passkeys/console/authenticate/challenge`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ user_id: userId }),
  });
  const challengeData = await challengeRes.json();
  if (!challengeData.success) {
    if (challengeData.needs_setup) throw new Error('NO_PASSKEY_REGISTERED');
    throw new Error(challengeData.error || 'Challenge failed');
  }

  // 2. Decode options
  const pk = challengeData.options.publicKey;
  pk.challenge = base64urlToBuffer(pk.challenge);
  if (pk.allowCredentials) {
    pk.allowCredentials.forEach((c: any) => { c.id = base64urlToBuffer(c.id); });
  }

  // 3. Browser authenticator
  const assertion = await navigator.credentials.get({ publicKey: pk });

  // 4. Verify
  const verifyRes = await fetch(`${API_BASE_URL}/api/v1/auth/passkeys/console/authenticate/verify`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      user_id: userId,
      assertion_response: {
        id: (assertion as any).id,
        rawId: bufferToBase64url((assertion as any).rawId),
        clientDataJSON: bufferToBase64url((assertion as any).response.clientDataJSON),
        authenticatorData: bufferToBase64url((assertion as any).response.authenticatorData),
        signature: bufferToBase64url((assertion as any).response.signature),
        userHandle: (assertion as any).response.userHandle
          ? bufferToBase64url((assertion as any).response.userHandle) : null,
      },
    }),
  });
  const verifyData = await verifyRes.json();
  return verifyData.success;
}
```

### 6.4 Approval Flow

```typescript
async function approveTransaction(txHash: string): Promise<boolean> {
  // 1. Get approval challenge
  const challengeRes = await fetch(`${API_BASE_URL}/api/v1/approvals/${txHash}/challenge`, {
    credentials: 'include',
  });
  if (!challengeRes.ok) throw new Error('Failed to get approval challenge');
  const challengeData = await challengeRes.json();

  // 2. Decode
  const pk = challengeData.publicKey;
  pk.challenge = base64urlToBuffer(pk.challenge);
  if (pk.allowCredentials) {
    pk.allowCredentials.forEach((c: any) => { c.id = base64urlToBuffer(c.id); });
  }

  // 3. Browser authenticator
  const assertion = await navigator.credentials.get({ publicKey: pk });

  // 4. Verify
  const verifyRes = await fetch(`${API_BASE_URL}/api/v1/approvals/${txHash}/verify`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      id: (assertion as any).id,
      rawId: bufferToBase64url((assertion as any).rawId),
      clientDataJSON: bufferToBase64url((assertion as any).response.clientDataJSON),
      authenticatorData: bufferToBase64url((assertion as any).response.authenticatorData),
      signature: bufferToBase64url((assertion as any).response.signature),
      userHandle: (assertion as any).response.userHandle
        ? bufferToBase64url((assertion as any).response.userHandle) : null,
    }),
  });
  return verifyRes.ok;
}
```

---

## 7. SSE Live Audit Stream

```typescript
function useAuditStream(webSessionId: string | null) {
  const [events, setEvents] = useState<SSEEventRow[]>([]);
  const [connected, setConnected] = useState(false);
  const eventSourceRef = useRef<EventSource | null>(null);

  const connect = useCallback(() => {
    if (!webSessionId || eventSourceRef.current) return;
    const params = new URLSearchParams({ web_session_id: webSessionId });
    const es = new EventSource(`${API_BASE_URL}/api/v1/sse/stream?${params}`, { withCredentials: true });
    eventSourceRef.current = es;

    es.onopen = () => setConnected(true);
    es.onerror = () => {
      setConnected(false);
      es.close();
      eventSourceRef.current = null;
      // Auto-reconnect after 3s
      setTimeout(() => connect(), 3000);
    };
    es.onmessage = (ev) => {
      try {
        const parsed = JSON.parse(ev.data);
        const innerEvent = parsed.event ? JSON.parse(parsed.event) : parsed;
        const entry: SSEEventRow = {
          id: parsed.id || parseInt(ev.lastEventId) || 0,
          event_type: innerEvent.type || 'event',
          payload: ev.data,
          created_at: innerEvent.timestamp || new Date().toISOString(),
        } as any;
        setEvents(prev => [...prev.slice(-499), entry]);
      } catch {
        setEvents(prev => [...prev.slice(-499), {
          id: 0, event_type: 'event', payload: ev.data, created_at: new Date().toISOString(),
        } as SSEEventRow]);
      }
    };
  }, [webSessionId]);

  const disconnect = useCallback(() => {
    if (eventSourceRef.current) {
      eventSourceRef.current.close();
      eventSourceRef.current = null;
    }
    setConnected(false);
  }, []);

  const clear = useCallback(() => setEvents([]), []);

  return { events, connected, connect, disconnect, clear };
}
```

> **Note:** The SSE `EventSource` API does not support custom headers or `credentials: 'include'` in all browsers. However, `EventSource` sends cookies by default for same-origin requests. For cross-origin, the g8e CORS middleware handles it. If `EventSource` doesn't send cookies cross-origin in your testing, fall back to polling `GET /api/v1/sse/events?web_session_id={id}&since_id={lastId}` every 2 seconds as an alternative.

---

## 8. UI/UX Guidelines

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

## 9. Error Handling

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

---

## 10. Verification Checklist

After the Lovable AI agent generates the app, verify:

- [ ] **CORS headers**: Open browser DevTools > Network tab. Make any API call and confirm `Access-Control-Allow-Origin` reflects your Lovable app's origin and `Access-Control-Allow-Credentials: true` is present.
- [ ] **Preflight OPTIONS**: Confirm OPTIONS requests succeed with 200/204 status before actual POST requests.
- [ ] **Passkey registration**: Click "Enroll Passkey" and confirm the browser's WebAuthn dialog appears with RP ID `console.g8e.ai`.
- [ ] **Passkey authentication**: Click "Sign In with Passkey" and confirm the WebAuthn dialog appears and login succeeds.
- [ ] **Session cookie**: After login, check DevTools > Application > Cookies for `web_session` with `SameSite=None` and `Secure` attributes.
- [ ] **Authenticated API calls**: Confirm `GET /api/v1/users/me` returns user data (not 401) after login.
- [ ] **SSE stream**: Click "Connect" on the audit stream card and confirm live events appear.
- [ ] **Approvals**: If a suspended transaction exists, confirm the "Approve" button triggers WebAuthn and the transaction is approved.
- [ ] **Logout**: Click "Sign Out" and confirm redirect to login page and cookie cleared.
- [ ] **URL hash handling**: Navigate to `#approve={txHash}` and confirm auto-approval flow triggers.

---

## See Also

- [Cloudflare Tunnel Integration](./cloudflare_tunnel.md) — Setting up the tunnel between Cloudflare and the g8e Gateway
- [Connect Apps to Gateway](./connect_apps_to_gateway.md) — General application connectivity patterns
- [Architecture: Auth](../architecture/auth.md) — WebAuthn passkey authentication architecture
- [Architecture: Gateway](../architecture/gateway.md) — Gateway service architecture
