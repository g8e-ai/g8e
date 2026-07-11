---
title: Lovable Deployment Guide
parent: Guides
---

# Lovable Deployment Guide

Last Updated: 2026-07-11
Version: v1.3.11

---

## Overview

This guide covers the end-to-end deployment workflow for connecting a [Lovable](https://lovable.dev) frontend application to a g8e Gateway backend. It combines Cloudflare Tunnel setup, gateway configuration, frontend enrollment, and JWT identity provider integration into a single reference.

For the detailed Lovable frontend build prompt, see [Lovable Frontend Integration](./lovable.md). For Cloudflare Tunnel specifics, see [Cloudflare Tunnel Integration](./cloudflare_tunnel.md).

---

## Architecture

```
[Lovable App (React)] → https://my-app.lovable.app → [Browser]
                                                        ↓
                                          credentials: 'include' + SameSite=None cookies
                                                        ↓
                              https://console.g8e.ai → [Cloudflare Edge] → [cloudflared] → [g8e Gateway :8443]
```

The Lovable app runs on `lovable.app`; the gateway is exposed via a Cloudflare Tunnel at a custom domain. Cross-origin requests use `credentials: 'include'` with `SameSite=None` cookies.

---

## Step 1: Start the Gateway

Start the gateway with CORS, passkey, and public base URL flags configured for both the tunnel domain and the Lovable app origin:

```bash
g8e gateway start \
  --cors-origin https://my-app.lovable.app \
  --passkey-rp-id lovable.app \
  --passkey-rp-origin https://my-app.lovable.app \
  --public-base-url https://console.g8e.ai
```

Key flags:

- `--cors-origin` — The Lovable app origin. Allows cross-origin requests with credentials.
- `--passkey-rp-id` — The WebAuthn Relying Party ID. Use the registrable domain (`lovable.app`) so passkeys work across Lovable subdomains.
- `--passkey-rp-origin` — The origin where WebAuthn ceremonies are performed. Must match the Lovable app URL.
- `--public-base-url` — The public URL of the gateway (the tunnel hostname). Used for approval redirect links and passkey origin verification.

To allow multiple frontend origins, repeat `--cors-origin` for each.

---

## Step 2: Configure the Cloudflare Tunnel

Create a Cloudflare Tunnel that routes traffic from `console.g8e.ai` to the local gateway on `localhost:8443`:

```yaml
# ~/.cloudflared/config.yml
tunnel: <tunnel-id>
credentials-file: ~/.cloudflared/<tunnel-id>.json
ingress:
  - hostname: console.g8e.ai
    service: https://localhost:8443
    originRequest:
      noTLSVerify: true
      http2Origin: true
  - service: http_status:404
```

Start the tunnel:

```bash
cloudflared tunnel run g8e
```

For detailed tunnel setup, see [Cloudflare Tunnel Integration](./cloudflare_tunnel.md).

---

## Step 3: Enroll the Frontend

Enroll the Lovable app origin with the gateway using the `g8e gui` command:

```bash
g8e gui enroll --origin https://my-app.lovable.app --public-base-url https://console.g8e.ai
```

This command:

1. Validates the origin URL
2. Sends a CORS preflight to the running gateway to verify the origin is allowed
3. Persists the origin to the local enrollment file
4. Outputs a TypeScript configuration snippet for the Lovable developer

The configuration snippet includes `apiFetch()` with `credentials: 'include'`, `connectSSE()` with `withCredentials: true`, and all key endpoint paths. Copy this snippet into the Lovable project.

---

## Step 4: L3 Approval Redirect

When a destructive mutation is suspended by the gateway's governance gauntlet, the gateway returns an approval URL. The Lovable frontend should redirect the user's browser to this URL, which lands on the gateway's embedded console with the approval token:

```typescript
// When a mutation is suspended, redirect to the gateway's console for passkey approval:
window.location.href = `${API_BASE_URL}/approve/${txHash}`;
```

The gateway console at `/console/` handles the WebAuthn approval ceremony. After the user approves with their passkey, the transaction resumes execution and an SSE event notifies the original submitting client.

---

## Step 5: Browser Telemetry via SSE

For live telemetry in Lovable dashboards, use the SSE (Server-Sent Events) endpoints. SSE supports dual authentication: mTLS for CLI/operator clients and web session cookies for browser clients.

Connect to the SSE stream from the Lovable frontend:

```typescript
const es = new EventSource(`${API_BASE_URL}/api/v1/sse/stream?web_session_id=${webSessionId}`, {
  withCredentials: true,
});
```

The SSE stream supports:

- `GET /api/v1/sse/stream` — Real-time event stream with 30-second heartbeats
- `GET /api/v1/sse/events` — Poll stored events with `since_id` and `limit` query parameters

**WebSocket pub/sub** (`/ws/v1/pubsub`) requires mTLS authentication and is not available to browser clients. Use SSE for all browser-based real-time telemetry. WebSocket pub/sub is reserved for CLI and operator clients with mTLS certificates.

---

## Step 6: JWT Identity Provider Integration (Optional)

When using an external identity provider (IdP) for JWT-based authentication, configure the gateway to validate JWTs from the IdP's JWKS endpoint:

```bash
g8e gateway start \
  --jwks-url https://your-idp.example.com/.well-known/jwks.json \
  --jwt-role-claim roles \
  --jwt-issuer https://your-idp.example.com \
  --jwt-audience g8e-gateway
```

When JWKS is configured, the gateway automatically accepts JWT bearer tokens for MCP and A2A endpoints instead of requiring mTLS. The gateway maps JWT role claims to g8e personas via the role-to-persona mapping configuration.

### Supported IdP Matrix

| IdP | JWKS URL | Role Claim | Notes |
|---|---|---|---|
| Clerk | `https://<domain>.clerk.accounts.dev/.well-known/jwks.json` | `roles` | Custom claim setup required in Clerk dashboard |
| Auth0 | `https://<domain>.auth0.com/.well-known/jwks.json` | `permissions` or custom namespace | Use `--jwt-role-claim permissions` or `--jwt-role-claim https://g8e.ai/roles` |
| Supabase | `https://<project>.supabase.co/auth/v1/.well-known/jwks.json` | `user_metadata.roles` | Requires custom JWT claims via Supabase Edge Function |

### Role-to-Persona Mapping

The gateway maps JWT role claims to g8e personas (admin, operator, observer, agent). Configure the mapping in the gateway configuration. Users with no matching role are denied access. JIT (just-in-time) provisioning creates a g8e user record on first successful JWT authentication.

---

## Verification Checklist

After completing the deployment, verify the integration:

- [ ] CORS headers present (`Access-Control-Allow-Origin`, `Access-Control-Allow-Credentials: true`)
- [ ] Passkey registration works from the Lovable app (browser WebAuthn dialog appears)
- [ ] Passkey authentication works (login succeeds, session cookie set)
- [ ] Session cookie attributes: `SameSite=None`, `Secure`, `HttpOnly`
- [ ] SSE stream connects and receives events from the Lovable app
- [ ] Authenticated API calls succeed (`GET /api/v1/users/me`)
- [ ] L3 approval redirect works (suspended transaction redirects to gateway console)
- [ ] JWT authentication works (if IdP configured; bearer token accepted)

Use `g8e gui verify --origin https://my-app.lovable.app` to run an automated verification check against the running gateway.

---

## See Also

- [Lovable Frontend Integration](./lovable.md) — Detailed Lovable build prompt with component architecture
- [Cloudflare Tunnel Integration](./cloudflare_tunnel.md) — Tunnel setup, TLS, and security layers
- [GUI Enrollment](./gui_enrollment.md) — `g8e gui` command reference
- [Gateway Architecture](../architecture/gateway.md) — Gateway internals and auth middleware
