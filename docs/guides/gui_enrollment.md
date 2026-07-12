---
title: GUI Enrollment
parent: Guides
---

# GUI Enrollment

Last Updated: 2026-07-12

---

## Overview

The `g8e gui` command manages external frontend application enrollment (React, Lovable, custom apps) with the g8e Gateway. Enrollment persists the frontend's origin in a local enrollment file and verifies that the running gateway was started with the correct `--cors-origin` and `--passkey-rp-origin` flags for that origin. The gateway is not restarted during enrollment; it must be started with the right flags beforehand.

After enrollment, the frontend can:
- Authenticate users via WebAuthn passkeys
- Receive SSE (Server-Sent Events) live streams
- Make authenticated API calls with session cookies

## Prerequisites

- g8e Gateway running with CORS and passkey RP origin flags set for the frontend origin (e.g., `g8e gw start --cors-origin https://my-app.lovable.app --passkey-rp-origin https://my-app.lovable.app`)
- Frontend application served on a known origin (e.g., `http://localhost:3003`, `https://my-app.lovable.app`)

## Commands

### `g8e gui enroll`

Enroll a frontend application origin with the gateway.

```bash
g8e gui enroll --origin <url> [flags]
```

Flags:
- `--origin` (required) — Frontend application origin URL (e.g., `https://my-app.lovable.app`)
- `--passkey-rp-id` — Passkey RP ID (defaults to the origin's hostname)
- `--passkey-rp-name` — Passkey RP display name (default: `g8e`)
- `--public-base-url` — Public base URL for the gateway (e.g., `https://console.g8e.ai`)

The command:
1. Validates the origin URL
2. Sends a CORS preflight request to the running gateway to verify the origin is in its allowed origins
3. If `--public-base-url` is provided, verifies the gateway is reachable at that URL
4. Persists the origin to `gui_enrollments.json` in the g8e runtime directory
5. Outputs a TypeScript configuration snippet for the frontend developer

If the gateway is not running or does not have the origin configured, the command fails with an error indicating which flags to use when starting the gateway.

### `g8e gui show`

Display all enrolled frontend origins and configuration snippets.

```bash
g8e gui show
g8e gui show --json    # machine-readable JSON output for scripting
g8e gui list           # alias for "show"
```

### `g8e gui remove`

Remove an enrolled frontend application origin from the enrollment file.

```bash
g8e gui remove --origin <url>
```

Flags:
- `--origin` (required) — Frontend application origin URL to remove

The command:
1. Validates the origin URL
2. Removes the origin from `gui_enrollments.json` in the g8e runtime directory

The gateway's CORS and passkey RP configuration is unchanged. To stop accepting the origin, restart the gateway without the corresponding `--cors-origin` and `--passkey-rp-origin` flags.

Returns `not found` error if the origin is not enrolled.

### `g8e gui verify`

Verify gateway connectivity and CORS configuration for a frontend origin.

```bash
g8e gui verify --origin <url>
```

Checks enrollment status and prints a verification checklist with gateway endpoint URLs for manual testing, including health, CORS preflight, SSE, and WebAuthn passkey endpoints.

## Frontend Integration Checklist

After enrollment, the frontend developer must:

- **CORS**: All `fetch` calls must include `credentials: 'include'`
- **Passkey RP**: The RP ID must match the gateway's hostname (derived from the origin or set via `--passkey-rp-id`)
- **SSE**: `EventSource` must use `withCredentials: true`
- **Session cookie**: The gateway sets `g8e_web_session_cookie` with `SameSite=None; Secure` for cross-origin configurations (`SameSite=Lax` for same-origin)

### Key Endpoints

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/v1/health` | GET | Health check (no auth) |
| `/api/v1/auth/bootstrap/status` | GET | Check if passkey is registered |
| `/api/v1/auth/passkeys/console/register/challenge` | POST | Begin passkey registration |
| `/api/v1/auth/passkeys/console/register/verify` | POST | Verify passkey registration |
| `/api/v1/auth/passkeys/console/authenticate/challenge` | POST | Begin passkey authentication |
| `/api/v1/auth/passkeys/console/authenticate/verify` | POST | Verify passkey authentication |
| `/api/v1/users/me` | GET | Get current user (requires session) |
| `/api/v1/sse/stream?web_session_id=<id>` | GET | SSE live events (requires session) |
| `/api/v1/approvals` | GET | List pending approvals (requires session) |

## Example: Lovable Integration

```bash
# Enroll a Lovable app
g8e gui enroll --origin https://my-app.lovable.app

# The command outputs a TypeScript snippet:
# const API_BASE_URL = 'https://localhost:8443';
# const PASSKEY_RP_ID = 'my-app.lovable.app';
# const PASSKEY_RP_NAME = 'g8e';
```

Paste the configuration snippet into your Lovable project and follow the [Lovable Frontend Integration](lovable.md) guide for the full component architecture.

## Example: Custom React App

```bash
# Enroll a local React dev server
g8e gui enroll --origin http://localhost:3000

# Verify connectivity
g8e gui verify --origin http://localhost:3000
```

## Troubleshooting

### CORS Errors

If the browser blocks requests with CORS errors:
- Verify the origin is enrolled: `g8e gui show`
- Verify the gateway was started with `--cors-origin` and `--passkey-rp-origin` flags for this origin
- Check that `credentials: 'include'` is set on all fetch calls

### Passkey RP Mismatch

If WebAuthn registration fails with "RP ID does not match":
- The RP ID must be a registrable domain suffix of the origin's hostname
- Use `--passkey-rp-id` to set a custom RP ID (e.g., `g8e gui enroll --origin https://app.example.com --passkey-rp-id example.com`)

### SSE Connection Refused

If SSE connections fail:
- Verify the session is authenticated (passkey authentication completed)
- Check that `withCredentials: true` is set on the `EventSource`
- The `web_session_id` parameter must match the session ID from authentication
