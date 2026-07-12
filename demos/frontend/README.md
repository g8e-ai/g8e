# Frontend Enrollment Demo

This demo demonstrates **third-party frontend application enrollment** with the g8e Gateway, showing how an external web app (hosted on a separate origin) can securely authenticate via WebAuthn passkeys and receive live SSE events.

## Overview

The frontend demo demonstrates:

- **CORS enrollment** — gateway accepts requests from `http://localhost:3003`
- **WebAuthn passkey registration** — browser creates a credential via `navigator.credentials.create`
- **WebAuthn passkey authentication** — browser authenticates via `navigator.credentials.get`
- **SSE live event streaming** — authenticated session receives real-time gateway events
- **Session cookie handling** — `credentials: 'include'` on all fetch calls

## Network Topology

- **net_untrusted (10.30.0.0/24)**: Reserved for bad-actor simulation
- **net_perimeter (10.31.0.0/24)**: Gateway public surface + frontend app
- **net_internal (10.32.0.0/24)**: Operator and gateway internal communication
- **net_secure (10.33.0.0/24)**: Operator actuator boundary
- **net_mgmt (10.34.0.0/24)**: Observability and audit trail

## Port Mappings

| Service | Port |
|---------|------|
| Gateway HTTP | 8083 |
| Gateway HTTPS | 8446 |
| Console | https://localhost:8446/console/ |
| Frontend App | 3003 |

## Doctrine Rules

The demo includes 3 frontend security doctrine rules:

- **Unauthorized API Access** — blocks unauthenticated API access attempts
- **CORS Origin Spoofing** — detects forged Origin/Referer headers
- **Session Hijacking** — detects session token replay/theft attempts

## Quick Start

### Prerequisites

- Docker and Docker Compose installed
- g8e binary built at repository root

### Build the g8e binary

```bash
cd /home/bob/g8e
make build
```

### Start the frontend demo

```bash
cd demos/frontend
docker compose up -d
```

### Check service status

```bash
docker compose ps
```

### Open the frontend app

Open `http://localhost:3003` in your browser to interact with the demo:

1. **Health check** — the app verifies gateway connectivity on load
2. **Register Passkey** — creates a WebAuthn credential in the browser
3. **Authenticate** — logs in with the registered passkey
4. **Connect SSE** — receives live gateway events via Server-Sent Events

## Demo Scenarios

### Scenario 1: Third-Party Frontend Enrollment

```bash
g8e demos run frontend 1
```

**Proves**: A third-party frontend application can securely connect to the g8e gateway.

The scenario runs a 5-step verification:

1. **Gateway health check** — confirms the g8e gateway is live on port 8083
2. **CORS preflight** — verifies the gateway accepts `OPTIONS` requests from `http://localhost:3003`
3. **Passkey endpoint accessible** — confirms the WebAuthn challenge endpoint responds
4. **SSE endpoint protected** — confirms the SSE stream returns 401 without a valid session (proves it is protected)
5. **Frontend app served** — confirms nginx is serving the frontend HTML on port 3003

After the automated checks pass, open `http://localhost:3003` in your browser to complete the interactive passkey registration and SSE streaming flow.

### Run all scenarios

```bash
g8e demos run frontend
```

## Architecture Notes

### Gateway Configuration

The gateway starts with CORS and passkey RP origins pre-configured for the frontend:

```
--passkey-rp-origin http://localhost:8083
--passkey-rp-origin https://localhost:8446
--passkey-rp-origin http://localhost:3003
--cors-origin http://localhost:3003
```

### Frontend App

The frontend is a single-file HTML application served by nginx on port 3003. No build step is required. The app uses:

- `credentials: 'include'` on all fetch calls
- `EventSource` with `withCredentials: true` for SSE
- Base64url helpers for WebAuthn credential encoding/decoding

### Data Sovereignty

All audit data is committed locally:
- **Git-backed ledger**: Immutable execution history on the operator container
- **SQLite audit vault**: Queryable audit trail on the gateway
- No data leaves the demo environment

## Stop and Clean Up

```bash
docker compose down
docker compose down -v  # also removes volumes
```

## License

Apache 2.0
