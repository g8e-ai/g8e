---
title: Connect Apps to Gateway
parent: Guides
---

# Connect Apps to g8e Gateway

Last Updated: 2026-07-13
Version: v1.5.0

---

## Overview

This guide covers connecting applications to the g8e Gateway. The g8e Gateway serves as the central Policy Decision Point (PDP) that enforces 5-layer Byzantine Fault Tolerant governance over all AI agent mutations. Applications connect via multiple protocol surfaces: MCP (Model Context Protocol), A2A (Agent-to-Agent), direct governance envelopes, WebSocket pub/sub, and JSON API.

---

## Reference g8e Gateway Connection

### Starting the g8e Gateway

Start the g8e Gateway to initialize the platform runtime:

```bash
./g8e gw start
```

This creates the `.g8e` directory structure:
- `.g8e/pki/` - PKI hierarchy (CA, certificates, keys)
- `.g8e/data/` - SQLite database for g8e Gateway persistence
- `.g8e/secrets/` - Platform secrets (session encryption keys, bootstrap digest)
- `.g8e/vault/` - Encryption vault for data at rest
- `.g8e/logs/` - Component logs

The g8e Gateway runs in the default mode (Doctrine: L1 enforced, L2/L3 audited). To run in different enforcement modes, use the `--posture` flag:

#### Doctrine Mode (Default)

Enforces L1 technical bedrock (forbidden patterns, blacklist, whitelist). L2 consensus and L3 notary signatures are audited but not required.

```bash
./g8e gw start --posture doctrine
```

#### Consensus Mode

Enforces L1 and L2 (multi-model Byzantine consensus). L3 notary signature is audited but not required.

```bash
./g8e gw start --posture consensus
```

#### Notary Mode

Enforces L1, L2, and L3 (human-in-the-loop via WebAuthn/FIDO2). This is the most secure posture.

```bash
./g8e gw start --posture notary
```

### g8e Gateway Surfaces

The g8e Gateway exposes two consolidated protocol surfaces. Each surface serves a specific purpose with distinct authentication requirements.

| Surface | Port (default) | Auth | Purpose |
|---|---|---|---|
| **HTTP Surface** | 8080 (HTTP) | None | Health checks, bootstrap enrollment, PKI discovery endpoints, catch-all redirect to HTTPS |
| **HTTPS Surface** | 8443 (TLS) | mTLS (app-layer) + URI SAN | Governance envelopes, MCP/A2A APIs, document store, WebSocket pub/sub, console SPA |

### Port Separation

The g8e Gateway enforces strict port separation for security:
- **HTTP Surface**: Plain HTTP for health checks, bootstrap enrollment, PKI discovery, and catch-all redirect to HTTPS. No MCP, A2A, governance, or mutation endpoints are exposed on this surface.
- **HTTPS Surface**: TLS with optional client certificate verification at the TLS layer. mTLS enforcement occurs at the application layer, where every route is classified into one of four auth modes: public (no auth), mTLS (client certificate required), web session (cookie-based browser auth), and dual (mTLS preferred, cookie fallback). Public routes (health, console SPA, bootstrap, passkey console, approval page) bypass mTLS; mTLS routes require a valid client certificate; web session routes validate a session cookie; dual routes try mTLS first and fall back to cookie auth.

Port mixing is prohibited. The gateway fails startup if the HTTP and HTTPS surfaces are assigned to the same port, as this would conflate plain-HTTP bootstrap routes with TLS-protected API routes.

Ports can be customized via CLI flags.

### Health Checks

Check Gateway status:

```bash
./g8e gw status
```

This reports:
- Gateway process status (running or stopped)
- Listening ports and endpoint URLs

The Gateway provides a unified health endpoint across all services for consistent health checking.

---

## Connectivity Methods

### 1. MCP (Model Context Protocol)

MCP is a JSON-RPC 2.0 protocol for AI tool invocation. The Gateway translates MCP tool calls into governance envelopes, runs them through L1/L2/L3 verification, and dispatches to downstream MCP servers or local execution.

The Gateway provides a unified MCP endpoint architecture with input validation that enforces fail-closed security for all tool inputs.

#### Native Tools

The Gateway includes a registry of native tools that execute locally with full governance enforcement. These tools are categorized by domain:

**Database Tools**
- `db_discover_topology` - Scans database schemas, tables, and column data types.
- `db_index_triage` - Queries database fragmentation statistics and index information.
- `db_isolated_read` - Executes SELECT statements in read-only mode against a SQLite database.
- `db_query_validate` - Validates SQL queries using EXPLAIN QUERY PLAN to detect full table scans.

**Filesystem Tools**
- `fs_disk_profile` - Calculates directory sizes and disk usage.
- `fs_disk_usage` - Provides df-style free space reporting for mounted filesystems.
- `fs_file_checksum` - Computes SHA256 checksums for file integrity verification.
- `read_file` - Reads file contents with path validation and safety checks.
- `log_stream_filter` - Reads log files and applies regex filtering with sensitive data scrubbing.

**Network Tools**
- `net_endpoint_ping` - Performs TCP handshake to verify network endpoint connectivity and measure latency.
- `net_http_probe` - Performs lightweight HTTP requests to probe web endpoints.
- `net_socket_audit` - Inspects active network sockets (TCP/UDP) from /proc/net.
- `net_dns_resolve` - Performs DNS resolution (dig/nslookup equivalent) for network debugging.
- `net_ssh_known_hosts` - Lists known hosts from SSH config and known_hosts files.

**Process Tools**
- `proc_metric_top` - Parses /proc to extract resource-consuming processes by CPU and memory.
- `proc_signal_safe` - Sends signals to processes with denylist enforcement for protected PIDs.
- `proc_tree` - Provides parent-child process relationships and process tree.

**System Tools**
- `sys_oom_detect` - Scans system logs for OOM (Out of Memory) killer events.
- `sys_info` - Provides system information including hostname, OS version, kernel, and uptime.
- `sys_env_vars` - Reads environment variables for configuration debugging with automatic secret redaction.
- `sys_service_status` - Checks systemd service status (operator, gateway, etc.).
- `sys_container_status` - Checks container health status (podman).
- `sys_time_clock` - Provides NTP sync status and system time verification.

**Configuration Tools**
- `config_diff_mask` - Compares configuration files with automatic secret masking.

**Security Tools**
- `tls_cert_inspect` - Parses TLS certificates, verifies chains, and checks expiration.

**Cloud Tools**
- `cloud_metadata` - Detects cloud provider (AWS, Azure, GCP) and retrieves instance metadata.

**Git Tools**
- `git_ops` - Provides git repository operations including status, log, and branch info.

**Kubernetes Tools**
- `k8s_inspect` - Provides Kubernetes cluster inspection including pods, nodes, and services.

**Shell Tools**
- `run_shell_command` - Executes shell commands with denylist enforcement for dangerous operations.

**Operator Tools**
- `operator_deploy` - Deploys the g8e operator to remote hosts via SSH.

All native tools include input validation to prevent SQL injection, SSRF attacks, path traversal, and other security vulnerabilities.

#### MCP Endpoints

| Endpoint | Method | Purpose |
|---|---|---|
| `/mcp` | POST/GET | Unified MCP JSON-RPC endpoint (POST for dispatch, GET for SSE heartbeat) |

#### MCP Unified Endpoint Tool Invocation

The `/mcp` endpoint is the sole MCP surface for AI IDEs. It implements the JSON-RPC 2.0 dispatch contract:

```bash
curl -X POST https://localhost:8443/mcp \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "run_shell_command",
      "arguments": {
        "command": "ls -la"
      }
    },
    "id": 1
  }'
```

#### MCP Special Tool: read_field

The Gateway provides a special `read_field` tool for governed field access with L1 validation and L3 session verification:

```json
{
  "jsonrpc": "2.0",
  "method": "tools/call",
  "params": {
    "name": "read_field",
    "arguments": {
      "collection": "cases",
      "document_id": "case-123",
      "field_path": "metadata.status",
      "operator_session_id": "op-session-abc"
    }
  },
  "id": 1
}
```

---

### 2. A2A (Agent-to-Agent)

A2A is an HTTP/JSON protocol for agent skill invocation. The Gateway wraps A2A skill calls in governance envelopes with the same L1/L2/L3 verification pipeline.

#### A2A Endpoints

| Endpoint | Method | Purpose |
|---|---|---|
| `/api/v1/a2a/call` | POST | Invoke an A2A skill |

#### A2A Skill Invocation

```bash
curl -X POST https://localhost:8443/api/v1/a2a/call \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "a2a/call",
    "params": {
      "skill_name": "file.read",
      "payload": {
        "path": "/etc/hosts"
      }
    },
    "id": 1
  }'
```

---

### 3. Direct Governance Envelope

Applications can submit canonical JSON `GovernanceEnvelope` transactions directly. This is the only customer-facing mutation API on the Gateway.

#### Envelope Submission

```bash
curl -X POST https://localhost:8443/api/v1/governance/envelopes \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d @envelope.json
```

The envelope `id` must match the deterministic transaction hash computed from critical envelope fields. The Gateway rejects envelopes with mismatched IDs.

---

### 4. WebSocket Pub/Sub

The Gateway provides real-time pub/sub via WebSocket for streaming events and command dispatch.

#### WebSocket Connection

```bash
wscat -c wss://localhost:8443/api/v1/pubsub/stream \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key
```

#### Pub/Sub Channels

- **Mutation channels**: `cmd:*` (governed, require envelope submission)
- **Non-mutation channels**: `heartbeat:*`, `results:*`, `sse:*`, `ws_session:*`, `internal:*`

#### Subscribe Protocol

To subscribe, send a message with the `subscribe` action and the desired channel name. The broker confirms with a `subscribed` acknowledgment frame before delivering any messages.

---

### 5. Document Store API

The Gateway provides a JSON document store with CRUD operations and query support.

#### Document Operations

```bash
# Get document
curl https://localhost:8443/api/v1/data/cases/case-123 \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key

# Set document
curl -X PUT https://localhost:8443/api/v1/data/cases/case-123 \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d '{"status": "open", "priority": "high"}'

# Update document (merge)
curl -X PATCH https://localhost:8443/api/v1/data/cases/case-123 \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d '{"priority": "critical"}'

# Delete document
curl -X DELETE https://localhost:8443/api/v1/data/cases/case-123 \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key

# Query documents
curl -X POST https://localhost:8443/api/v1/data/cases/_query \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d '{
    "filter": {"status": "open"},
    "limit": 10
  }'
```

The document store uses the `/api/v1/data/{collection}/{id}` pattern for CRUD operations and `/api/v1/data/{collection}/_query` for queries.

#### Governed Collections

Direct `/api/v1/data/` mutations are restricted to platform infrastructure collections. Governed collections (cases, investigations, tasks, memories, reputation_state, reputation_commitments, stake_resolutions, agent_activity_metadata) must use `POST /api/v1/governance/envelopes` for mutations.

---

## Protocol Library for Client Development

Applications connecting to the g8e Gateway can use the g8e Protocol Library to construct `GovernanceEnvelope` transactions, parse `ActionReceipt` responses, and access protocol constants. The library is published as both a Go module and a Python package, both sharing the same version number as the platform binary.

### Go Module

```bash
go get github.com/g8e-ai/g8e@v1.5.0
```

The Go module provides protobuf types for envelope construction and receipt parsing, SPIFFE workload identity helpers, and JSON constant registries.

### Python Package

```bash
pip install g8e==1.5.0
```

The Python package provides event type constants, request context models, and HTTP header constants for gateway communication. Requires Python 3.10+.

See the [Protocol Library documentation](../architecture/protocol.md) for the full API reference, usage examples, and package contents.

---

## Authentication

### Session Types

The Gateway enforces strict session separation to prevent cross-tenant data leakage:

| Session Type | Identifier | Authentication | Use Case |
|---|---|---|---|
| **Web Session** | `web_session_id` | WebAuthn (passkey) | Browser-based clients |
| **CLI Session** | `cli_session_id` | mTLS certificate | CLI/BYO clients |
| **Operator Session** | `operator_session_id` | mTLS certificate | In-process execution context |

### CLI Authentication (mTLS)

Generate a client certificate for CLI operations:

```bash
./g8e auth enroll
```

This:
1. Generates a CSR (Certificate Signing Request)
2. Receives a signed client certificate with SPIFFE URI SAN
3. Stores it in `.g8e/pki/client.crt`

CLI sessions use the mTLS certificate fingerprint as L3 proof.

### Browser Authentication (WebAuthn)

For web-based interactions:

1. Navigate to `https://localhost:8443`, which redirects to `/console/` (the console SPA)
2. Follow on-screen prompts to register a passkey
3. Use the passkey for subsequent authentication

Web sessions use WebAuthn signatures as L3 proof.

### CSR-Based Enrollment

**The mental model:** CSR-based enrollment is cryptographic identity proof. Instead of
sharing a secret (like an API key), a client generates its own key pair and asks the
Gateway to sign a certificate attesting "this public key belongs to this identity." The
Gateway acts as a Certificate Authority (CA). The act of starting the Gateway is itself
the Platform Owner's authorization; there are no standing invite codes, pre-shared keys,
or manual approval steps. The client then proves its identity on every subsequent call by
signing with its private key (via mTLS). No shared secrets, no API keys to leak.

All authentication to the Gateway uses CSR-based enrollment. The first human to
authenticate via `./g8e auth enroll` becomes the Platform Owner. All other entities
(operators, MCP servers, AI clients, applications) enroll via the same CSR flow.

#### Enrollment Flow

1. **Client generates key pair and CSR**: The entity (device, app, or user) creates a
   private key and a Certificate Signing Request (CSR) that states the desired identity
   (e.g., `spiffe://g8e.local/app/etl-service`)
2. **Gateway validates and signs**: The Gateway (acting as CA) issues a signed mTLS
   certificate with a SPIFFE URI SAN
3. **Client receives certificate**: The client gets `client.crt` (signed by the Gateway's CA)
   and uses it with its private key for all subsequent authentication
4. **Short-lived by design**: Leaf certificates expire after 7 days, so a
   compromised key has limited lifetime
5. **Certificate renewal**: Clients must re-enroll before certificate expiry

#### Device Enrollment

For device enrollment, use the `/api/v1/pki/devices/enroll` endpoint (see PKI section below).

#### Application Enrollment

Applications enroll via the `/api/v1/pki/apps/enroll` endpoint to obtain an app identity (`spiffe://g8e.local/app/<appname>`). For delegated credential enrollment (where a human CLI session vouches for the app), use the `/api/v1/pki/apps/delegated` endpoint with mTLS authentication.

---

## GUI Enrollment

The `g8e gui` command manages external frontend application enrollment (React, Lovable, custom apps) with the g8e Gateway. Enrollment persists the frontend's origin in a local enrollment file and verifies that the running gateway was started with the correct `--cors-origin` and `--passkey-rp-origin` flags for that origin. The gateway is not restarted during enrollment; it must be started with the right flags beforehand.

After enrollment, the frontend can:
- Authenticate users via WebAuthn passkeys
- Receive SSE (Server-Sent Events) live streams
- Make authenticated API calls with session cookies

### Prerequisites

- g8e Gateway running with CORS and passkey RP origin flags set for the frontend origin (e.g., `g8e gw start --cors-origin https://my-app.lovable.app --passkey-rp-origin https://my-app.lovable.app`)
- Frontend application served on a known origin (e.g., `http://localhost:3003`, `https://my-app.lovable.app`)

### Commands

#### `g8e gui enroll`

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

#### `g8e gui show`

Display all enrolled frontend origins and configuration snippets.

```bash
g8e gui show
g8e gui show --json    # machine-readable JSON output for scripting
g8e gui list           # alias for "show"
```

#### `g8e gui remove`

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

#### `g8e gui verify`

Verify gateway connectivity and CORS configuration for a frontend origin.

```bash
g8e gui verify --origin <url>
```

Checks enrollment status and prints a verification checklist with gateway endpoint URLs for manual testing, including health, CORS preflight, SSE, and WebAuthn passkey endpoints.

### Frontend Integration Checklist

After enrollment, the frontend developer must:

- **CORS**: All `fetch` calls must include `credentials: 'include'`
- **Passkey RP**: The RP ID must match the gateway's hostname (derived from the origin or set via `--passkey-rp-id`)
- **SSE**: `EventSource` must use `withCredentials: true`
- **Session cookie**: The gateway sets `g8e_web_session_cookie` with `SameSite=None; Secure` for cross-origin configurations (`SameSite=Lax` for same-origin)

#### Key Endpoints

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

### Example: Lovable Integration

```bash
# Enroll a Lovable app
g8e gui enroll --origin https://my-app.lovable.app

# The command outputs a TypeScript snippet:
# const API_BASE_URL = 'https://localhost:8443';
# const PASSKEY_RP_ID = 'my-app.lovable.app';
# const PASSKEY_RP_NAME = 'g8e';
```

Paste the configuration snippet into your Lovable project and follow the [Lovable Frontend Integration](lovable.md) guide for the full component architecture.

### Example: Custom React App

```bash
# Enroll a local React dev server
g8e gui enroll --origin http://localhost:3000

# Verify connectivity
g8e gui verify --origin http://localhost:3000
```

### GUI Enrollment Troubleshooting

#### CORS Errors

If the browser blocks requests with CORS errors:
- Verify the origin is enrolled: `g8e gui show`
- Verify the gateway was started with `--cors-origin` and `--passkey-rp-origin` flags for this origin
- Check that `credentials: 'include'` is set on all fetch calls

#### Passkey RP Mismatch

If WebAuthn registration fails with "RP ID does not match":
- The RP ID must be a registrable domain suffix of the origin's hostname
- Use `--passkey-rp-id` to set a custom RP ID (e.g., `g8e gui enroll --origin https://app.example.com --passkey-rp-id example.com`)

#### SSE Connection Refused

If SSE connections fail:
- Verify the session is authenticated (passkey authentication completed)
- Check that `withCredentials: true` is set on the `EventSource`
- The `web_session_id` parameter must match the session ID from authentication

---

## PKI and Trust

### Device Enrollment (CSR-based)

Enroll a device using CSR-based enrollment with mTLS authentication. The user_id is extracted from the client certificate's SPIFFE URI SAN:

```bash
curl -X POST https://localhost:8443/api/v1/pki/devices/enroll \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d '{
    "csr_pem": "-----BEGIN CERTIFICATE REQUEST-----...",
    "cli_csr_pem": "-----BEGIN CERTIFICATE REQUEST-----...",
    "system_fingerprint": "fp-123",
    "hostname": "my-host",
    "os": "linux",
    "arch": "amd64",
    "username": "user",
    "ip_address": "192.168.1.1"
  }'
```

### CSR Signing (Low-level)

Submit a CSR for low-level certificate issuance (for advanced use cases):

```bash
curl -X POST https://localhost:8443/api/v1/pki/csr/sign \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d '{
    "csr_pem": "-----BEGIN CERTIFICATE REQUEST-----...",
    "leaf_type": "operator",
    "operator_id": "op-123"
  }'
```

---

## Out-of-Band (OOB) Approval Flow

When a standard AI client requests a mutation without L3 proof, the Gateway suspends the transaction and returns an OOB approval URL.

### Suspension Flow

1. Client submits MCP/A2A request without L3 proof.
2. Gateway stores the transaction in a suspended state.
3. Gateway returns approval URL: `https://localhost:8443/api/v1/approve/{tx_hash}`.
4. User opens URL in browser and authenticates with passkey.
5. User approves transaction via WebAuthn.
6. Gateway attaches the L3 proof and resubmits the envelope through the verification pipeline.
7. Transaction proceeds to execution and the signed receipt is returned.

### Approval API

List suspended transactions (requires web session cookie):

```bash
curl https://localhost:8443/api/v1/approvals \
  --cookie "web_session=..."
```

Get WebAuthn challenge for a suspended transaction:

```bash
curl https://localhost:8443/api/v1/approvals/{tx_hash}/challenge \
  --cookie "web_session=..."
```

Verify WebAuthn assertion and resume execution:

```bash
curl -X POST https://localhost:8443/api/v1/approvals/{tx_hash}/verify \
  --cookie "web_session=..." \
  -H "Content-Type: application/json" \
  -d '{
    "id": "credential-id-base64url",
    "clientDataJSON": "...",
    "authenticatorData": "...",
    "signature": "..."
  }'
```

The Gateway attaches the L3 proof to the stored envelope and resubmits it through the verification pipeline. On success, the suspended transaction is deleted and the signed receipt is returned.

---

## Audit and Receipts

### Query Audit Receipts

```bash
curl https://localhost:8443/api/v1/audit/receipts?operator_session_id=op-session-abc \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key
```

### Export Audit Receipts

```bash
curl https://localhost:8443/api/v1/audit/receipts/export?since=2026-01-01T00:00:00Z&limit=100 \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -o audit-export.json
```

### CLI Audit Query

```bash
./g8e gw data audit list --operator-session-id <session-id> --limit 100
```

---

## Custom Gateway Implementation

For custom g8e-compatible gateway implementations, connection follows the same operational pattern:

1. **Initialize PKI**: Generate root CA and intermediate CAs with SPIFFE URI SAN support
2. **Configure Persistence**: Set up document store, KV store, and blob store with state root computation
3. **Configure Ports**: Bind the two logical surfaces (HTTP and HTTPS) to appropriate ports with correct TLS settings
4. **Start Gateway**: Launch in desired mode (doctrine, consensus, or notary)
5. **Enroll Clients**: Use CSR-based enrollment for operators and CLI clients
6. **Monitor Health**: Implement health checks for gateway process and connected operators

### Configuration Requirements

Custom gateways must support:
- CLI flags for runtime parameters (ports, mode, paths)
- Strict port separation with optional client certificate verification on the HTTPS surface and application-layer auth enforcement per route classification

---

## Troubleshooting

### Gateway Fails to Start

Check if ports are already in use:

```bash
./g8e gw status
```

Verify PKI initialization:

```bash
ls -la .g8e/pki/
```

### Authentication Failures

Verify client certificate exists:

```bash
ls -la .g8e/pki/client.crt
```

Re-run login if certificate is missing or expired:

```bash
./g8e auth enroll
```

### Operator Connection Issues

Check Gateway is listening on the HTTPS port:

```bash
curl -k https://localhost:8443/api/v1/health
```

---

## Next Steps

- **[Build Operator](build_operator.md)** - Build a custom g8e-compatible g8e Operator
- **[Connect Operator to Gateway](connect_operator_to_gateway.md)** - Deploy and use a g8e Operator
- **[Build Apps](build_apps.md)** - Build g8e-compatible applications using a Gateway
- **[Lovable Frontend Integration](lovable.md)** - Full component architecture for Lovable apps
- **[MCP Protocol](../../protocol/docs/mcp.md)** - Detailed MCP protocol specification
- **[A2A Protocol](../../protocol/docs/a2a.md)** - Detailed A2A protocol specification
- **[Gateway Architecture](../architecture/gateway.md)** - Gateway architecture and internals
- **[Protocol Library](../architecture/protocol.md)** - Go module and Python package API reference, constants, models, and usage examples
