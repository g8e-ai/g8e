---
title: Cloudflare Tunnel Integration
parent: Guides
---

# Cloudflare Tunnel Integration

Last Updated: 2026-07-14
Version: v1.5.1

---

## Overview

A Cloudflare Tunnel securely exposes the g8e Gateway's HTTPS console to the internet without opening firewall ports or managing public DNS records. The tunnel runs `cloudflared` as a sidecar process alongside the gateway. Cloudflare terminates TLS at the edge; the tunnel forwards traffic to the gateway's HTTPS listener on `localhost:8443`.

For integrating a [Lovable](https://lovable.dev) frontend with the g8e Gateway, see the [Lovable Frontend Integration guide](./lovable.md).

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

The `g8e gw tunnel create` command automates tunnel creation, DNS routing, and config generation in a single step:

```bash
g8e gw tunnel create --name g8e --hostname console.g8e.ai
```

This command:
1. Authenticates with Cloudflare (runs `cloudflared tunnel login` if not already authenticated)
2. Creates a named tunnel
3. Routes DNS to the tunnel (creates a CNAME record `console.g8e.ai` → `<tunnel-id>.cfargotunnel.com`)
4. Generates `~/.cloudflared/config.yml` with the correct ingress rules

**Flags:**

| Flag | Default | Purpose |
|---|---|---|
| `--name` | `g8e` | Tunnel name |
| `--hostname` | (required) | Public hostname for the tunnel |
| `--https-port` | `8443` | Gateway HTTPS port |
| `--config-dir` | `~/.cloudflared` | cloudflared config directory |
| `--ca-bundle` | (none) | Path to CA bundle for origin TLS verification (see below) |
| `--origin-server-name` | (none) | Origin server name for TLS SNI (used with `--ca-bundle`) |
| `--skip-dns` | `false` | Skip DNS routing if the CNAME already exists |

### Origin TLS Verification

By default, the generated config uses `noTLSVerify: true` because the gateway uses self-signed certificates from its internal PKI. This is safe because the connection is local (localhost). For stricter verification, pass `--ca-bundle` with the gateway's CA bundle path. The generated config then uses `originCaPool` and `originServerName` instead of `noTLSVerify`.

### Manual Alternative

If you prefer to run `cloudflared` commands directly:

```bash
cloudflared tunnel login
cloudflared tunnel create g8e
cloudflared tunnel route dns g8e console.g8e.ai
```

The credentials file is saved to `~/.cloudflared/<tunnel-id>.json`.

---

## Step 2: Configure cloudflared

The `g8e gw tunnel create` command generates `~/.cloudflared/config.yml` automatically. The generated content looks like:

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

- **`noTLSVerify: true`**: The gateway uses self-signed certificates from its internal PKI. `cloudflared` does not verify the upstream cert. This is safe because the connection is local (localhost).
- **`http2Origin: true`**: Enables HTTP/2 to the origin for SSE streaming performance.
- **`http_status:404` catch-all**: Rejects requests for unmatched hostnames.

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

In a separate terminal, start the tunnel using the CLI wrapper:

```bash
g8e gw tunnel run --name g8e
```

This runs `cloudflared tunnel run` in the foreground with the generated config. Press Ctrl+C to stop.

Alternatively, run `cloudflared` directly:

```bash
cloudflared tunnel run g8e
```

---

## Step 5: Verify End-to-End

Check tunnel connectivity and gateway health in one step:

```bash
g8e gw tunnel status --hostname console.g8e.ai --name g8e
```

This checks `cloudflared tunnel info` and hits the gateway health endpoint through the public hostname.

Alternatively, verify manually:

```bash
curl -s https://console.g8e.ai/api/v1/health
```

Expected response:

```json
{"status":"ok","mode":"gateway","version":"v1.5.1","pid":12345,"governance_ready":true,"state_merkle_root":"..."}
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

For building a g8e Governance Console UI with [Lovable](https://lovable.dev), including gateway-side CORS and passkey RP configuration, frontend enrollment, API reference, WebAuthn flows, and SSE streaming, see the [Lovable Frontend Integration guide](./lovable.md).

When configuring the gateway for a Lovable frontend, add the Lovable app's origin to `--cors-origin` (or `G8E_ALLOWED_ORIGINS`) in addition to the tunnel hostname. The `http2Origin: true` setting in the tunnel config ensures SSE streaming works through the tunnel.

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

# Check tunnel and gateway status in one step
g8e gw tunnel status --hostname console.g8e.ai --name g8e
```

### CORS Errors in Browser Console

The gateway's `--cors-origin` flag does not include the frontend's origin. Add the origin:

```bash
./g8e gw start -f --cors-origin https://your-app.lovable.app ...
```

### WebAuthn Passkey Enrollment Fails

The `--passkey-rp-id` must match the browser's registrable domain. If accessing via `console.g8e.ai`, the RP ID must be `console.g8e.ai` (not `localhost`).

### DNS Record Conflict

If DNS routing fails with "record already exists," use `--skip-dns` with `g8e gw tunnel create` to skip the DNS step. Alternatively, delete the old DNS record in the Cloudflare dashboard or use a different hostname.

### Tunnel Version Outdated

```bash
# Check version
cloudflared --version

# Update (if installed via dpkg)
wget -q https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64.deb -O /tmp/cloudflared.deb
sudo dpkg -i /tmp/cloudflared.deb
```

---

## See Also

- **[Lovable Frontend Integration](./lovable.md)**: Build a g8e Governance Console UI with Lovable.
- **[Protocol Library](../architecture/protocol.md)**: Go module and Python package API reference for building g8e-compatible clients and services.
