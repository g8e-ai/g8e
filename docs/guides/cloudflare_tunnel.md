---
title: Cloudflare Tunnel Integration
parent: Guides
---

# Cloudflare Tunnel Integration

Last Updated: 2026-07-10
Version: v1.3.11

---

## Overview

A Cloudflare Tunnel securely exposes the g8e Gateway's HTTPS console to the internet without opening firewall ports or managing public DNS records. The tunnel runs `cloudflared` as a sidecar process alongside the gateway. Cloudflare terminates TLS at the edge; the tunnel forwards traffic to the gateway's HTTPS listener on `localhost:8443`.

This guide also covers integrating a [Lovable](https://lovable.dev) frontend with the g8e Gateway backend for a data-sovereign web application architecture.

### Architecture

```
[Browser] → https://console.g8e.ai → [Cloudflare Edge (TLS + Access)] → [cloudflared tunnel] → [localhost:8443]
                                                                                              ↓
                                                              [g8e Gateway: Console SPA + WebAuthn + mTLS API]
```

### Security Layers

| Layer | Mechanism | Purpose |
|---|---|---|
| **Cloudflare Edge TLS** | Cloudflare-managed certificates | Terminates TLS with a public CA cert; browser sees valid HTTPS |
| **Cloudflare Access (optional)** | Zero-trust identity proxy | Email/OIDC/SAML authentication before traffic reaches the gateway |
| **Tunnel transport** | Encrypted QUIC/H2 connection | `cloudflared` maintains outbound-only connection to Cloudflare edge; no inbound ports required |
| **Gateway WebAuthn** | Passkey-based authentication | Browser-side passkey enrollment and login for console access |
| **Gateway mTLS** | Self-signed internal PKI | Machine-to-machine authentication for operator and API clients |
| **Origin TLS** | Self-signed cert on localhost:8443 | `cloudflared` uses `noTLSVerify: true` since the origin is local |

---

## Prerequisites

- `cloudflared` installed on the same host as the g8e Gateway ([installation guide](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/))
- A Cloudflare account with a registered domain (e.g., `g8e.ai`)
- The g8e binary built (`make build`)

---

## Step 1: Create a Cloudflare Tunnel

Authenticate `cloudflared` with your Cloudflare account:

```bash
cloudflared tunnel login
```

Create a named tunnel:

```bash
cloudflared tunnel create g8e
```

Note the tunnel ID from the output. The credentials file is saved to `~/.cloudflared/<tunnel-id>.json`.

Route DNS to the tunnel:

```bash
cloudflared tunnel route dns g8e console.g8e.ai
```

This creates a CNAME record `console.g8e.ai` → `<tunnel-id>.cfargotunnel.com`.

---

## Step 2: Configure cloudflared

Create or edit `~/.cloudflared/config.yml`:

```yaml
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

**Key settings:**

- **`noTLSVerify: true`** — The gateway uses self-signed certificates from its internal PKI. `cloudflared` does not verify the upstream cert. This is safe because the connection is local (localhost).
- **`http2Origin: true`** — Enables HTTP/2 to the origin for SSE streaming performance.
- **`http_status:404` catch-all** — Rejects requests for unmatched hostnames.

---

## Step 3: Start the Gateway

Start the gateway with flags matching the tunnel domain:

```bash
./g8e gw start -f \
  --posture doctrine \
  --passkey-rp-id console.g8e.ai \
  --passkey-rp-origin https://console.g8e.ai \
  --public-base-url https://console.g8e.ai \
  --cors-origin https://console.g8e.ai
```

**Flag explanations:**

| Flag | Value | Purpose |
|---|---|---|
| `--passkey-rp-id` | `console.g8e.ai` | WebAuthn RP ID must match the browser's registrable domain |
| `--passkey-rp-origin` | `https://console.g8e.ai` | WebAuthn origin validation (repeatable for multiple origins) |
| `--public-base-url` | `https://console.g8e.ai` | L3 approval links point to the tunnel domain |
| `--cors-origin` | `https://console.g8e.ai` | CORS headers for cross-origin browser access (repeatable) |

### Environment Variable Equivalents

All flags can be set via environment variables instead:

```bash
export G8E_PASSKEY_RP_ID=console.g8e.ai
export G8E_PASSKEY_RP_ORIGINS=https://console.g8e.ai
export G8E_PUBLIC_BASE_URL=https://console.g8e.ai
export G8E_ALLOWED_ORIGINS=https://console.g8e.ai

./g8e gw start -f --posture doctrine
```

Environment variables are resolved when the corresponding CLI flag is empty. CLI flags take precedence.

---

## Step 4: Start the Tunnel

In a separate terminal:

```bash
cloudflared tunnel run g8e
```

Verify the tunnel is connected:

```bash
cloudflared tunnel info g8e
```

You should see active edge connections (e.g., `1xsea01, 2xsea09`).

---

## Step 5: Verify End-to-End

Check the health endpoint through the tunnel:

```bash
curl -s https://console.g8e.ai/api/v1/health
```

Expected response:

```json
{"status":"ok","mode":"gateway","version":"dev","governance_ready":true,"state_merkle_root":"..."}
```

Open the console in a browser:

```
https://console.g8e.ai/console/
```

You should see the g8e Console SPA with WebAuthn passkey enrollment.

---

## Step 6: Cloudflare Access (Zero-Trust Hardening)

For maximum security, add Cloudflare Access as an identity proxy in front of the tunnel. This adds an authentication layer before traffic even reaches the gateway.

### Create an Access Application

1. Go to **Cloudflare Zero Trust Dashboard** → **Access** → **Applications**.
2. Click **Add an application** → **Self-hosted**.
3. Set the application name to `g8e Console`.
4. Set the public hostname to `console.g8e.ai`.
5. Configure an identity provider (email OTP, Google, GitHub, or SAML/OIDC for enterprise).
6. Create a policy:
   - **Action:** Allow
   - **Include:** Your email address or team domain
7. Save.

### How Access Interacts with g8e

Cloudflare Access intercepts all requests to `console.g8e.ai` and requires identity verification. Once authenticated, Cloudflare sets a JWT cookie and forwards the request to the tunnel. The gateway's own WebAuthn passkey system provides a second, independent authentication factor for console operations.

**Defense in depth:**

```
Browser → Cloudflare Access (identity check) → cloudflared tunnel → Gateway WebAuthn (passkey) → Console
```

### Service Auth for API Endpoints

For machine-to-machine API access (e.g., MCP calls from an operator), use **Service Auth** instead of interactive login:

1. Create a separate Access application for `console.g8e.ai/api/*`.
2. Set the policy action to **Service Auth**.
3. Generate a service token.
4. Pass the token headers in API requests:

```bash
curl -s https://console.g8e.ai/api/v1/health \
  -H "CF-Access-Client-Id: <service-token-id>" \
  -H "CF-Access-Client-Secret: <service-token-secret>"
```

---

## Lovable Frontend Integration

[Lovable](https://lovable.dev) generates React frontends that can connect to any backend via `fetch`. The g8e Gateway serves as a data-sovereign backend with WebAuthn authentication, governance enforcement, and audit logging.

### Architecture

```
[Lovable React App] → (fetch) → https://console.g8e.ai/api/v1/* → [Cloudflare Edge] → [g8e Gateway]
```

### Gateway-Side Configuration

The gateway must allow CORS from the Lovable app's origin. Start the gateway with the Lovable domain:

```bash
./g8e gw start -f \
  --posture doctrine \
  --passkey-rp-id console.g8e.ai \
  --passkey-rp-origin https://console.g8e.ai \
  --public-base-url https://console.g8e.ai \
  --cors-origin https://console.g8e.ai \
  --cors-origin https://your-app.lovable.app \
  --cors-origin https://your-custom-domain.com
```

Or via environment variables:

```bash
export G8E_ALLOWED_ORIGINS=https://console.g8e.ai,https://your-app.lovable.app,https://your-custom-domain.com
```

The gateway's CORS middleware:
- Reflects the request `Origin` if it matches an allowed origin
- Sets `Access-Control-Allow-Credentials: true`
- Handles `OPTIONS` preflight with `204 No Content`
- Adds `Vary: Origin` to all responses

### Lovable-Side Configuration

In your Lovable project:

1. **Set the API base URL as a secret:**
   - Go to **Cloud** tab → **Secrets**
   - Add `VITE_API_BASE_URL` = `https://console.g8e.ai`
   - Use the `VITE_` prefix so it's accessible in React code

2. **Create a config file:**
   ```typescript
   // src/config/api.ts
   export const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? '';
   ```

3. **Create an API service layer:**
   ```typescript
   // src/services/g8eApi.ts
   import { API_BASE_URL } from '@/config/api';

   class G8eApi {
     private baseUrl: string;

     constructor() {
       this.baseUrl = API_BASE_URL;
     }

     private async request<T>(path: string, options?: RequestInit): Promise<T> {
       const response = await fetch(`${this.baseUrl}${path}`, {
         ...options,
         headers: {
           'Content-Type': 'application/json',
           ...options?.headers,
         },
       });
       if (!response.ok) {
         throw new Error(`g8e API error: ${response.status} ${response.statusText}`);
       }
       return response.json();
     }

     async health() {
       return this.request<{ status: string; mode: string }>('/api/v1/health');
     }

     async listSuspendedTransactions() {
       return this.request<{ transactions: any[] }>('/api/v1/approvals');
     }
   }

   export const g8eApi = new G8eApi();
   ```

4. **Use in components:**
   ```typescript
   import { g8eApi } from '@/services/g8eApi';

   async function checkHealth() {
     const health = await g8eApi.health();
     console.log('Gateway status:', health.status);
   }
   ```

### Important Lovable Notes

- **CORS is required:** The g8e Gateway must have the Lovable app's origin in `--cors-origin` (or `G8E_ALLOWED_ORIGINS`). Without this, the browser blocks cross-origin requests.
- **WebAuthn requires same-origin:** Passkey enrollment and login only work when the browser origin matches the `--passkey-rp-id` domain. For Lovable-hosted frontends, the passkey flow must redirect to `https://console.g8e.ai/console/` for enrollment, then redirect back. Alternatively, use a custom domain on Lovable that matches the RP ID.
- **No API keys in frontend code:** For authenticated endpoints, route requests through Lovable Edge Functions (Supabase) that add credentials server-side. Store secrets without the `VITE_` prefix so they stay server-side only.
- **SSE streaming:** The gateway's SSE endpoints (`/api/v1/sse/stream`) work through Cloudflare Tunnels with `http2Origin: true`. Use `EventSource` in the Lovable app to consume live audit events.

---

## Environment Variable Reference

| Variable | CLI Flag Equivalent | Example |
|---|---|---|
| `G8E_PASSKEY_RP_ID` | `--passkey-rp-id` | `console.g8e.ai` |
| `G8E_PASSKEY_RP_NAME` | `--passkey-rp-name` | `g8e` |
| `G8E_PASSKEY_RP_ORIGINS` | `--passkey-rp-origin` (repeatable) | `https://console.g8e.ai` (comma-separated) |
| `G8E_PUBLIC_BASE_URL` | `--public-base-url` | `https://console.g8e.ai` |
| `G8E_ALLOWED_ORIGINS` | `--cors-origin` (repeatable) | `https://console.g8e.ai,https://your-app.lovable.app` (comma-separated) |

CLI flags take precedence over environment variables. Environment variables are resolved only when the corresponding CLI flag is empty.

---

## Troubleshooting

### 502 Bad Gateway

The tunnel is connected but the gateway is not running or not listening on `localhost:8443`.

```bash
# Check if the gateway is running
curl -sk https://localhost:8443/api/v1/health

# Check if cloudflared is running
cloudflared tunnel info g8e
```

### CORS Errors in Browser Console

The gateway's `--cors-origin` flag does not include the frontend's origin. Add the origin:

```bash
./g8e gw start -f --cors-origin https://your-app.lovable.app ...
```

### WebAuthn Passkey Enrollment Fails

The `--passkey-rp-id` must match the browser's registrable domain. If accessing via `console.g8e.ai`, the RP ID must be `console.g8e.ai` (not `localhost`).

### DNS Record Conflict

If `cloudflared tunnel route dns` fails with "record already exists," delete the old DNS record in the Cloudflare dashboard or use a different hostname.

### Tunnel Version Outdated

```bash
# Check version
cloudflared --version

# Update (if installed via dpkg)
wget -q https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64.deb -O /tmp/cloudflared.deb
sudo dpkg -i /tmp/cloudflared.deb
```
