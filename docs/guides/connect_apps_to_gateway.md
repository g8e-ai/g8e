---
title: Connect Apps to Gateway
parent: Guides
---

# Connect Apps to g8e Gateway

Last Updated: 2026-09-08
Version: v2.1.7

---

## Overview

This guide covers connecting applications to the g8e Gateway. The g8e Gateway serves as the central Policy Decision Point (PDP) that enforces 5-layer Byzantine Fault Tolerant governance over all AI agent mutations. Applications connect via multiple protocol surfaces: MCP (Model Context Protocol), A2A (Agent-to-Agent), direct governance envelopes, WebSocket pub/sub, and the document store API.

---

## Reference g8e Gateway Connection

### Starting the g8e Gateway

Start the g8e Gateway to initialize the platform runtime:

```bash
./g8e gw start
```

For foreground execution (Ctrl+C stops the gateway):

```bash
./g8e gw start --follow
```

For interactive onboarding (launches a setup wizard):

```bash
./g8e gw start --interactive
```

Alternatively, run the setup wizard separately before starting:

```bash
./g8e gw setup
```

Starting the gateway creates the `.g8e` directory structure:
- `.g8e/pki/` - PKI hierarchy (CA, certificates, keys)
- `.g8e/data/` - SQLite database for g8e Gateway persistence
- `.g8e/secrets/` - Platform secrets (session encryption keys, bootstrap digest)
- `.g8e/vault/` - Encryption vault for data at rest
- `.g8e/logs/` - Component logs

The g8e Gateway defaults to the `doctrine` posture. The `--posture` flag selects which of the first three governance layers are fail-closed gates and which are audit-only. Layers L4 (Warden, pre-dispatch verification) and L5 (Actuator, isolated tool dispatch with signed receipts) always run regardless of posture. The reference Gateway supports four postures:

#### Doctrine Mode (Default)

Enforces L1 technical bedrock (forbidden patterns, blacklist, whitelist). L2 consensus and L3 notary signatures are audited but not required.

```bash
./g8e gw start --posture doctrine
```

#### Consensus Mode

Enforces L1 and L2 (multi-signature Byzantine consensus). L3 notary signature is audited but not required.

```bash
./g8e gw start --posture consensus
```

#### Ratify Mode

Enforces L1 and L3 (human approval through WebAuthn or an authenticated CLI proof). L2 consensus is audited but not required.

```bash
./g8e gw start --posture ratify
```

#### Notary Mode

Enforces L1, L2, and L3. Mutations require both L2 consensus and L3 approval.

```bash
./g8e gw start --posture notary
```

### g8e Gateway Surfaces

The g8e Gateway exposes two consolidated protocol surfaces. Each surface serves a specific purpose with distinct authentication requirements.

| Surface | Port (default) | Auth | Purpose |
|---|---|---|---|
| **HTTP Surface** | 8080 (HTTP) | Public or token-scoped | Health checks, bootstrap and platform enrollment discovery, CLI recovery request/status/complete, PKI discovery endpoints, deploy scripts |
| **HTTPS Surface** | 8443 (TLS) | Per route: public, mTLS, web session, dual, or JWT for MCP/A2A when JWKS is configured | Governance envelopes, MCP/A2A APIs, document store, WebSocket pub/sub, SSE, and the console SPA |

### Port Separation

The g8e Gateway enforces strict port separation for security:
- **HTTP Surface**: Plain HTTP for health checks, initial bootstrap, token-scoped CLI recovery and platform enrollment request/status/completion, PKI discovery, and deploy scripts. No MCP, A2A, governance, document-store, pub/sub, or other authenticated mutation endpoints are exposed on this surface.
- **HTTPS Surface**: TLS-protected surface for governance, MCP, A2A, document store, WebSocket, SSE, and console routes. Every route is classified as public, mTLS-only, web-session-only, or dual (mTLS preferred with cookie fallback). When JWKS authentication is configured, the MCP and A2A handlers validate JWTs instead of relying on the main mTLS middleware.

Public routes such as health, the console SPA, bootstrap, passkey console, and the approval redirect page bypass mTLS. mTLS routes require a valid client certificate and an accepted SPIFFE identity. Web-session routes validate the `g8e_web_session_cookie` cookie. Dual routes try mTLS first and fall back to that cookie.

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

The command-line examples use an enrolled CLI identity. `./g8e auth enroll user` stores that identity under `.g8e/pki/` and normally installs the Gateway root CA in the operating-system trust store. On systems where the root is not installed, add `--cacert .g8e/pki/trust/g8eg-ca-bundle.pem` to each `curl` command. A deployed service instead uses its own app certificate and private key as described in [Application Enrollment](#application-enrollment).

The credential determines authorization as well as transport authentication:

| Credential | Primary surfaces | Important restrictions |
|---|---|---|
| CLI or Operator mTLS certificate | Direct governance envelopes, data APIs, audit, PKI management, MCP, and A2A | The certificate identity and active session must match request context. |
| App mTLS certificate | MCP, A2A, WebSocket pub/sub, SSE producer APIs, and policy-authorized data APIs | App identities cannot submit directly to `/api/v1/governance/envelopes`; the Gateway constructs envelopes for MCP, A2A, and `cmd:` pub/sub intents. App policy controls limits and access. |
| Web session cookie | Browser user, passkey, approval, and SSE consumer routes | Browser requests use `credentials: 'include'`; this credential is not accepted by mTLS-only routes. |
| JWT | MCP and A2A only when Gateway JWKS authentication is configured | The configured issuer, audience, role, and token signature are validated. |

### 1. MCP (Model Context Protocol)

MCP is a JSON-RPC 2.0 protocol for AI tool invocation. The Gateway accepts MCP tool calls, applies governance verification, and dispatches to native tools or downstream MCP servers.

The Gateway exposes a single unified MCP endpoint with fail-closed input validation for all tool calls.

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

**Audit Tools**
- `audit_receipt_list` - Lists signed ActionReceipt records from the operator audit vault scoped to an operator session.
- `audit_receipt_get` - Retrieves a single signed ActionReceipt by transaction ID from the operator audit vault.

All native tools enforce fail-closed input validation.

#### MCP Endpoints

| Endpoint | Method | Purpose |
|---|---|---|
| `/mcp` | POST, GET | Streamable HTTP MCP endpoint: POST dispatches JSON-RPC requests and GET opens an SSE connection with keepalive events. |

#### MCP Unified Endpoint Tool Invocation

The `/mcp` endpoint is the sole MCP surface. Standard clients begin with `initialize`, send `notifications/initialized`, discover tools with `tools/list`, and invoke a tool with `tools/call`. The Gateway currently negotiates MCP protocol version `2025-06-18` by default and echoes a client-supplied protocol version. The following direct request demonstrates tool invocation after client setup:

```bash
curl -X POST https://localhost:8443/mcp \
  --cert .g8e/pki/cli.crt \
  --key .g8e/pki/cli.key \
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

A2A is a JSON-RPC 2.0 HTTP protocol for agent skill invocation. The Gateway applies the same governance pipeline to A2A skill calls as it does to MCP tool calls. The reference Gateway does not provide a built-in A2A skill catalog; start it with `--a2a-downstream-url <url>` before invoking skills supplied by a downstream A2A server.

#### A2A Endpoints

| Endpoint | Method | Purpose |
|---|---|---|
| `/api/v1/a2a/call` | POST | Invoke an A2A skill |

#### A2A Skill Invocation

```bash
curl -X POST https://localhost:8443/api/v1/a2a/call \
  --cert .g8e/pki/cli.crt \
  --key .g8e/pki/cli.key \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "a2a/call",
    "params": {
      "skill_name": "<downstream-skill-name>",
      "payload": {
        "input": "<skill-input>"
      }
    },
    "id": 1
  }'
```

---

### 3. Direct Governance Envelope

Authenticated CLI and Operator clients can submit canonical protojson `GovernanceEnvelope` transactions directly. This is the direct mutation API for clients that construct complete envelopes themselves. App certificates are intentionally blocked from this route; app workloads submit MCP calls, A2A calls, or `CommandIntent` messages on authorized `cmd:` channels so the Gateway constructs and verifies the envelope.

#### Envelope Submission

```bash
curl -X POST https://localhost:8443/api/v1/governance/envelopes \
  --cert .g8e/pki/cli.crt \
  --key .g8e/pki/cli.key \
  -H "Content-Type: application/json" \
  -d @envelope.json
```

Both envelope `id` and `transaction_hash` must equal the canonical transaction hash computed from the hashed envelope fields. The Gateway rejects a missing or mismatched value. The wire body uses protobuf JSON field names and encodings; construct it with the protocol library rather than ad hoc JSON.

---

### 4. WebSocket Pub/Sub

The Gateway provides mTLS-authenticated real-time pub/sub at `wss://<gateway>:8443/api/v1/pubsub/stream`. The WebSocket wire format is binary protobuf, not JSON: clients encode and decode `g8e.pubsub.v1.PubSubMessage` frames from the protocol library.

#### Pub/Sub Actions and Channels

Clients send `subscribe`, `psubscribe`, `unsubscribe`, and `publish` actions. A successful exact or pattern subscription returns a protobuf acknowledgment before event delivery. Topic ACLs bind subscriptions and publications to the authenticated certificate identity; broad cross-operator patterns such as `heartbeat:*` are rejected.

Canonical operator channels include:

- `cmd:<operator_id>:<operator_session_id>` for app-to-Operator command intents. An authorized app publishes canonical protojson `CommandIntent` data; the Gateway validates the target session and constructs the governed envelope.
- `results:<operator_id>:<operator_session_id>` and `heartbeat:<operator_id>:<operator_session_id>` for Operator output and liveness.
- `receipts:<operator_id>:<operator_session_id>` for signed Operator receipts. The Gateway verifies, persists, and relays valid receipts.
- Session-scoped `sse:` and `ws_session:` channels used by the Gateway event bridges.

Use a protobuf-capable WebSocket client for application integration. A text-oriented client such as `wscat` can verify the TLS upgrade but cannot directly produce the required protobuf frames.

---

### 5. Document Store API

The Gateway provides a JSON document store with CRUD operations and query support.

#### Document Operations

```bash
# Get document
curl https://localhost:8443/api/v1/data/settings/platform_settings \
  --cert .g8e/pki/cli.crt \
  --key .g8e/pki/cli.key

# Set document (allowed on infrastructure collections)
curl -X PUT https://localhost:8443/api/v1/data/settings/platform_settings \
  --cert .g8e/pki/cli.crt \
  --key .g8e/pki/cli.key \
  -H "Content-Type: application/json" \
  -d '{"posture": "doctrine"}'

# Update document (merge)
curl -X PATCH https://localhost:8443/api/v1/data/settings/platform_settings \
  --cert .g8e/pki/cli.crt \
  --key .g8e/pki/cli.key \
  -H "Content-Type: application/json" \
  -d '{"posture": "consensus"}'

# Delete document
curl -X DELETE https://localhost:8443/api/v1/data/settings/platform_settings \
  --cert .g8e/pki/cli.crt \
  --key .g8e/pki/cli.key

# Query documents
curl -X POST https://localhost:8443/api/v1/data/cases/_query \
  --cert .g8e/pki/cli.crt \
  --key .g8e/pki/cli.key \
  -H "Content-Type: application/json" \
  -d '{
    "filters": [
      {
        "field": "status",
        "op": "==",
        "value": "open"
      }
    ],
    "limit": 10
  }'
```

The document store uses the `/api/v1/data/{collection}/{id}` pattern for CRUD operations and `/api/v1/data/{collection}/_query` for queries.

#### Governed Collections

Direct `/api/v1/data/` mutations (PUT, PATCH, DELETE) are restricted to platform infrastructure collections (`settings`, `users`, `operators`, `operator_sessions`, `bound_sessions`, `passkey_challenges`, `revoked_certificates`, `trusted_signers`, `console_audit`). Governed collections (`cases`, `investigations`, `tasks`, `memories`, `reputation_state`, `reputation_commitments`, `stake_resolutions`, `agent_activity_metadata`) permit reads and queries via `GET` and `POST .../_query`, but require `POST /api/v1/governance/envelopes` for mutations.

---

## Protocol Library for Client Development

Applications connecting to the g8e Gateway can use the g8e Protocol Library to construct `GovernanceEnvelope` transactions, parse `ActionReceipt` responses, and access protocol constants. The library is published as both a Go module and a Python package, both sharing the same version number as the platform binary.

### Go Module

```bash
go get github.com/g8e-ai/g8e/v2@v2.1.7
```

The Go module provides types for envelope construction, receipt parsing, and SPIFFE workload identity.

### Python Package

```bash
pip install g8e==2.1.7
```

The Python package provides constants and models for gateway communication. Requires Python 3.10+.

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
./g8e auth enroll user
```

This:
1. Generates a CSR (Certificate Signing Request)
2. Receives a signed client certificate with SPIFFE URI SAN
3. Stores the client certificate in `.g8e/pki/cli.crt` and private key in `.g8e/pki/cli.key`
4. Opens a browser to register a WebAuthn/FIDO2 passkey for web session authentication

CLI sessions use the mTLS certificate fingerprint as L3 proof. The passkey enables browser-based authentication for console and approval flows.

### Browser Authentication (WebAuthn)

For web-based interactions:

1. Navigate to `https://localhost:8443`, which redirects to `/console/` (the console SPA)
2. Follow on-screen prompts to register a passkey
3. Use the passkey for subsequent authentication

Web sessions use WebAuthn signatures as L3 proof.

### CSR-Based mTLS Enrollment

CSR-based enrollment gives CLI, Operator, and app workloads cryptographic identities without transferring their private keys to the Gateway. The client generates a P-256 key and CSR, the Gateway authorizes the enrollment path, and the issued certificate carries one or more SPIFFE URI SANs. Browser sessions use WebAuthn rather than client certificates, and MCP/A2A clients may use JWT authentication when the Gateway has JWKS configured.

The first successful `./g8e auth enroll user` against an unbootstrapped Gateway creates the first user and CLI session; that user is the platform owner. Later CLI enrollment on an already bootstrapped Gateway uses the one-time human-approved recovery flow. External apps use delegated enrollment backed by an enrolled human CLI. Reserved first-party platform components use the separate owner-approved platform enrollment protocol.

#### mTLS Enrollment Properties

1. **Private key ownership**: The client creates and retains its private key and submits only a signed CSR.
2. **SPIFFE identity**: The Gateway issues a certificate with an identity appropriate to the enrollment path, such as `spiffe://g8e.local/cli/<user_id>/<cli_session_id>` or `spiffe://g8e.local/app/<app_name>`.
3. **Trust material**: The enrollment response or runtime tree supplies the certificate chain and Gateway trust bundle needed for server verification.
4. **Short lifetimes**: Standard leaf certificates and CLI sessions have a seven-day lifetime. Delegated external-app certificates have a one-hour lifetime.
5. **Renewal**: The CLI enrollment coordinator reuses complete valid credentials and rotates an expiring identity; `--rotate-cli` forces rotation. External apps request another delegated certificate before expiry. Reserved platform components resume their owner-approved enrollment workflow as needed.

#### Device Enrollment

For device enrollment, use the `/api/v1/pki/devices/enroll` endpoint (see PKI section below).

#### Application Enrollment

External applications use delegated enrollment. An enrolled human CLI authenticates `POST /api/v1/pki/apps/delegated`, vouches for the application, and submits a P-256 CSR with `app_name`, `app_type`, and optional `organization_id`. The Gateway returns a one-hour certificate containing both the app identity and requesting-user identity, its chain, the trust bundle, the SPIFFE app ID, and the expiry time. It also creates the default `AppPolicy` required by app authentication. Delegated enrollment establishes identity only; it does not grant L2 consensus signing authority.

The following example creates an app key and CSR, builds the JSON request without flattening PEM newlines, and enrolls the app with the local CLI identity:

```bash
openssl ecparam -name prime256v1 -genkey -noout -out etl-service.key
openssl req -new -key etl-service.key -subj "/CN=etl-service" -out etl-service.csr
python3 - <<'PY'
import json
from pathlib import Path

request = {
    "csr_pem": Path("etl-service.csr").read_text(),
    "app_name": "etl-service",
    "app_type": "custom",
}
Path("etl-service-enrollment.json").write_text(json.dumps(request))
PY
curl -X POST https://localhost:8443/api/v1/pki/apps/delegated \
  --cacert .g8e/pki/trust/g8eg-ca-bundle.pem \
  --cert .g8e/pki/cli.crt \
  --key .g8e/pki/cli.key \
  -H "Content-Type: application/json" \
  -d @etl-service-enrollment.json \
  -o etl-service-enrollment-response.json
```

The app retains `etl-service.key`; the Gateway never returns the private key. Persist `app_cert`, `cert_chain`, and `trust_bundle` from the response with private-file permissions. Present the leaf certificate followed by its chain when connecting:

```bash
python3 - <<'PY'
import json
import os
from pathlib import Path

response = json.loads(Path("etl-service-enrollment-response.json").read_text())
if not response.get("success"):
    raise RuntimeError(response.get("error", "app enrollment failed"))
Path("etl-service.crt").write_text(response["app_cert"])
Path("etl-service-chain.pem").write_text(response["app_cert"] + response["cert_chain"])
Path("g8eg-ca-bundle.pem").write_text(response["trust_bundle"])
for path in ("etl-service.key", "etl-service.crt", "etl-service-chain.pem", "g8eg-ca-bundle.pem"):
    os.chmod(path, 0o600)
PY
curl -X POST https://localhost:8443/mcp \
  --cacert g8eg-ca-bundle.pem \
  --cert etl-service-chain.pem \
  --key etl-service.key \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/list","id":1}'
```

The reserved first-party names `g8ed`, `g8ee`, and `g8eo` cannot use delegated enrollment. Those components use the owner-approved platform enrollment endpoints under `/api/v1/auth/platform-enrollments/`: request, status, and completion are token-scoped discovery operations available over plain HTTP, while pending-list and decision operations require the active first owner through mTLS or a web session. The resumable client generates keys, submits its request, waits for an exact request-ID decision, signs the completion transcript, validates the issued identity, and writes credentials atomically.

The in-tree Ensemble (`g8ee`) and Dashboard (`g8ed`) clients implement that reserved-component flow during startup. See [Authentication Architecture](../architecture/auth.md), [Ensemble Architecture](../architecture/ensemble.md), [Dashboard Architecture](../architecture/dashboard.md), and [Build a g8e-Compatible Frontend](./build_frontend.md) for their component-specific behavior.

---

## GUI Enrollment

The `g8e auth enroll gui` command tree manages local integration metadata for external browser frontends such as React or Lovable applications. It does not create a server-side app identity, change Gateway configuration, or restart the Gateway. The running Gateway must already have matching CORS and WebAuthn settings.

The current `gui enroll` implementation sends its CORS preflight to the plain-HTTP health endpoint, while the plain-HTTP router does not apply the Gateway CORS middleware. Consequently, `gui enroll` fails its preflight before it persists `.g8e/gui_enrollments.json`, even when the HTTPS surface has the requested origin configured. The browser integration itself uses the HTTPS surface and can be configured with the Gateway flags below. `gui verify` only prints a manual checklist; it does not execute those checks. Treat the GUI enrollment commands as local tooling with this current limitation, not as a server-side authorization step.

With the HTTPS CORS and WebAuthn settings configured, the frontend can:
- Authenticate users via WebAuthn passkeys
- Receive SSE (Server-Sent Events) live streams
- Make authenticated API calls with session cookies

### Prerequisites

- Frontend application served on a known origin, such as `http://localhost:3003` or `https://my-app.lovable.app`.
- Gateway running with that exact origin in both `--cors-origin` and `--passkey-rp-origin`.
- For a non-localhost frontend, `--passkey-rp-id` set to the frontend hostname or a registrable parent-domain suffix. For example: `./g8e gw start --cors-origin https://my-app.lovable.app --passkey-rp-origin https://my-app.lovable.app --passkey-rp-id my-app.lovable.app`. The Gateway has one RP ID, so all configured browser origins must be valid for that RP ID.

### Commands

#### `g8e auth enroll gui enroll`

Attempt to validate and persist a frontend origin in the local enrollment file. This command currently encounters the plain-HTTP CORS limitation described above.

```bash
g8e auth enroll gui enroll --origin <url> [flags]
```

Flags:
- `--origin` (required): Frontend application origin URL (e.g., `https://my-app.lovable.app`)
- `--passkey-rp-id`: RP ID printed in the generated frontend snippet. When omitted, the command uses the parsed origin host value; specify this flag explicitly for origins containing a port. This option does not reconfigure the running Gateway, whose `--passkey-rp-id` must match.
- `--passkey-rp-name`: RP display name printed in the snippet (default: `g8e`).
- `--public-base-url`: Gateway base URL printed in the snippet. The command checks reachability and emits a warning, rather than failing, if this URL cannot be reached.

The command:
1. Validates the origin URL
2. Sends an `OPTIONS` preflight to the plain-HTTP health endpoint on the default localhost port and checks `Access-Control-Allow-Origin`.
3. If `--public-base-url` is provided, checks its health endpoint and prints a warning if the check fails.
4. Persists the origin to `.g8e/gui_enrollments.json`.
5. Outputs a TypeScript configuration snippet for the frontend developer.

The enrollment and verification commands construct their local URLs with the default localhost ports 8080 and 8443; they do not discover custom `--http-port` or `--https-port` values. Persistence and snippet output happen only after the preflight succeeds. In the current Gateway, the plain-HTTP health response has no CORS headers, so that preflight does not succeed.

#### `g8e auth enroll gui show`

Display all enrolled frontend origins and configuration snippets.

```bash
g8e auth enroll gui show
g8e auth enroll gui show --json    # machine-readable JSON output for scripting
g8e auth enroll gui list           # alias for "show"
```

#### `g8e auth enroll gui remove`

Remove an enrolled frontend application origin from the enrollment file.

```bash
g8e auth enroll gui remove --origin <url>
```

Flags:
- `--origin` (required): Frontend application origin URL to remove

The command:
1. Validates the origin URL
2. Removes the origin from `gui_enrollments.json` in the g8e runtime directory

The gateway's CORS and passkey RP configuration is unchanged. To stop accepting the origin, restart the gateway without the corresponding `--cors-origin` and `--passkey-rp-origin` flags.

Returns `not found` error if the origin is not enrolled.

#### `g8e auth enroll gui verify`

Verify gateway connectivity and CORS configuration for a frontend origin.

```bash
g8e auth enroll gui verify --origin <url>
```

Checks enrollment status and prints a verification checklist with gateway endpoint URLs for manual testing, including health, CORS preflight, SSE, and WebAuthn passkey endpoints.

### Frontend Integration Checklist

The frontend integration uses these settings:

- **CORS**: All `fetch` calls must include `credentials: 'include'`
- **Passkey RP**: The RP ID returned in WebAuthn options must equal the Gateway's configured RP ID and must be the frontend origin hostname or a registrable suffix of it. RP IDs never include a scheme or port.
- **SSE**: Construct `EventSource` with `{ withCredentials: true }`. The Gateway derives the user and web-session route from the authenticated cookie; do not send `web_session_id` in the query string.
- **Session cookie**: The Gateway sets the HttpOnly, Secure `g8e_web_session_cookie`. When any cross-origin origin is configured, the cookie uses `SameSite=None`; otherwise it uses `SameSite=Lax`.

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
| `/api/v1/sse/stream` | GET | SSE live events; the authenticated cookie supplies the user and web-session route |
| `/api/v1/approvals` | GET | List pending approvals (requires session) |

### Example: Lovable Integration

```bash
./g8e gw start \
  --cors-origin https://my-app.lovable.app \
  --passkey-rp-origin https://my-app.lovable.app \
  --passkey-rp-id my-app.lovable.app

# Configure the frontend directly; gui enroll currently fails its HTTP preflight.
# const API_BASE_URL = 'https://localhost:8443';
# const PASSKEY_RP_ID = 'my-app.lovable.app';
# const PASSKEY_RP_NAME = 'g8e';
```

Add this configuration to the Lovable project and follow [Connect a Lovable App](lovable.md) for local browser and certificate setup.

### Example: Custom React App

```bash
./g8e gw start \
  --cors-origin http://localhost:3000 \
  --passkey-rp-origin http://localhost:3000 \
  --passkey-rp-id localhost

# Configure the frontend with API_BASE_URL=https://localhost:8443 and PASSKEY_RP_ID=localhost.
# This prints the current manual checklist; it does not perform the checks.
g8e auth enroll gui verify --origin http://localhost:3000
```

### GUI Enrollment Troubleshooting

#### CORS Errors

If the browser blocks requests with CORS errors:
- Verify the Gateway was started with the frontend's exact origin in `--cors-origin`; the local `gui_enrollments.json` file does not affect server authorization.
- Verify the same origin is present in `--passkey-rp-origin` for WebAuthn ceremonies.
- Check that `credentials: 'include'` is set on all `fetch` calls.

#### Passkey RP Mismatch

If WebAuthn registration fails with "RP ID does not match":
- Verify the Gateway was started with an RP ID that is the frontend origin hostname or a registrable suffix; it must not include a scheme or port.
- Keep the generated frontend configuration aligned with the same value, for example `g8e auth enroll gui enroll --origin https://app.example.com --passkey-rp-id example.com`.
- Restart the Gateway with `--passkey-rp-id example.com --passkey-rp-origin https://app.example.com` if its active configuration differs.

#### SSE Connection Refused

If SSE connections fail:
- Verify passkey authentication completed and the browser holds `g8e_web_session_cookie`.
- Check that the `EventSource` uses `{ withCredentials: true }`.
- Do not add `web_session_id`, `cli_session_id`, or `user_id` to the browser stream URL; the Gateway derives the route from authenticated request context.
- If the frontend is cross-origin, verify the exact origin is allowed and the cookie was issued with `SameSite=None; Secure`.

---

## PKI and Trust

### Device Enrollment (CSR-based)

Enroll a device using CSR-based enrollment with mTLS authentication. The user_id is extracted from the client certificate's SPIFFE URI SAN:

```bash
curl -X POST https://localhost:8443/api/v1/pki/devices/enroll \
  --cert .g8e/pki/cli.crt \
  --key .g8e/pki/cli.key \
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

`POST /api/v1/pki/csr/sign` is an authenticated low-level platform identity endpoint. Prefer the purpose-built CLI, device, delegated-app, or platform-component enrollment flows because they create the associated session and policy records. Callers of this endpoint must supply every identity component required by the selected leaf type.

```bash
curl -X POST https://localhost:8443/api/v1/pki/csr/sign \
  --cert .g8e/pki/cli.crt \
  --key .g8e/pki/cli.key \
  -H "Content-Type: application/json" \
  -d '{
    "csr_pem": "-----BEGIN CERTIFICATE REQUEST-----...",
    "leaf_type": "operator",
    "organization_id": "org-123",
    "operator_id": "op-123",
    "workload_session_id": "op-session-123"
  }'
```

---

## Out-of-Band (OOB) Approval Flow

In `ratify` and `notary` posture, an MCP or A2A mutation without valid L3 proof is suspended for out-of-band passkey approval. In `doctrine` and `consensus` posture, L3 is audit-only and its absence does not suspend the transaction.

### Suspension Flow

1. Client submits MCP/A2A request without L3 proof.
2. Gateway stores the transaction in a suspended state.
3. Gateway returns approval URL: `https://localhost:8443/api/v1/approve/{tx_hash}` (which redirects to `/console/#approve={tx_hash}`).
4. User opens URL in browser and authenticates with passkey.
5. User approves transaction via WebAuthn.
6. Gateway attaches the L3 proof and resubmits the envelope through the verification pipeline.
7. Transaction proceeds to execution and the signed receipt is returned.

### Approval API

List suspended transactions (requires web session cookie):

```bash
curl https://localhost:8443/api/v1/approvals \
  --cookie "g8e_web_session_cookie=..."
```

Get WebAuthn challenge for a suspended transaction:

```bash
curl https://localhost:8443/api/v1/approvals/{tx_hash}/challenge \
  --cookie "g8e_web_session_cookie=..."
```

Verify WebAuthn assertion and resume execution:

```bash
curl -X POST https://localhost:8443/api/v1/approvals/{tx_hash}/verify \
  --cookie "g8e_web_session_cookie=..." \
  -H "Content-Type: application/json" \
  -d '{
    "id": "credential-id-base64url",
    "rawId": "credential-id-base64url",
    "clientDataJSON": "...",
    "authenticatorData": "...",
    "signature": "..."
  }'
```

The Gateway attaches the L3 proof and resubmits the envelope through the verification pipeline. On success, the signed receipt is returned.

---

## Audit and Receipts

### Query Audit Receipts

```bash
curl https://localhost:8443/api/v1/audit/receipts?operator_session_id=op-session-abc \
  --cert .g8e/pki/cli.crt \
  --key .g8e/pki/cli.key
```

### Export Audit Receipts

```bash
curl https://localhost:8443/api/v1/audit/receipts/export?since=2026-01-01T00:00:00Z&limit=100 \
  --cert .g8e/pki/cli.crt \
  --key .g8e/pki/cli.key \
  -o audit-export.json
```

### Response Contracts and Verification

`GET /api/v1/audit/receipts?tx_id=<transaction-id>` returns a bare canonical protojson `ActionReceipt`. Supplying `investigation_id` together with `action_type` performs a unique correlation lookup and also returns the bare receipt; multiple matches return HTTP 409. Requests without either unique selector and all `/receipts/export` requests return `AuditReceiptsResponse`, whose `receipts` array contains searchable columns and the complete canonical receipt under each record's `action_receipt` field.

Consumers verify both signatures before trusting an exported receipt. Select the Gateway or Operator actuator public key whose derived key ID matches the receipt's `signer_key_id`; a deployment that produces receipts through multiple actuators requires every producer key. The Python protocol package accepts a selected key as raw bytes, hexadecimal, or SPKI PEM:

```python
import json
from pathlib import Path

from g8e.receipts import (
    parse_action_receipt,
    verify_action_receipt_signature,
    verify_receipt_persistence_attestation,
)

record = json.loads(Path("audit-export.json").read_text())["receipts"][0]
receipt = parse_action_receipt(record["action_receipt"])
public_key = Path(".g8e/pki/Actuator_pub.pem").read_text()

if not verify_action_receipt_signature(receipt, public_key):
    raise ValueError("invalid action receipt signature")
if not verify_receipt_persistence_attestation(receipt, public_key):
    raise ValueError("invalid receipt persistence attestation")
```

The caller establishes trust in the supplied public key out of band; the verification helpers do not provide an attested key-distribution channel.

### CLI Audit Query

Query signed receipts directly using the CLI:

```bash
./g8e audit receipts --session <session-id>
```

Or query raw audit events from the Gateway audit store:

```bash
./g8e gw data audit list --operator-session-id <session-id> --limit 100
```

---

## Custom Gateway Implementation

For custom g8e-compatible gateway implementations, connection follows the same operational pattern:

1. **Initialize PKI**: Generate root CA and intermediate CAs with SPIFFE URI SAN support
2. **Configure Persistence**: Set up document store and persistence backends
3. **Configure Ports**: Bind the two logical surfaces (HTTP and HTTPS) to appropriate ports with correct TLS settings
4. **Start Gateway**: Launch in the desired posture (doctrine, consensus, ratify, or notary)
5. **Enroll Clients**: Use CSR-based enrollment for operators and CLI clients
6. **Monitor Health**: Implement health checks for gateway process and connected operators

### Configuration Requirements

Custom gateways must support:
- CLI flags for runtime parameters (ports, mode, paths)
- Strict port separation with TLS on the HTTPS surface and per-route auth classification

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

Verify client certificate and key exist:

```bash
ls -la .g8e/pki/cli.crt .g8e/pki/cli.key
```

Re-run login if certificate is missing or expired:

```bash
./g8e auth enroll user
```

### Operator Connection Issues

Check the public health endpoint without disabling TLS verification:

```bash
curl http://localhost:8080/api/v1/health
curl --cacert .g8e/pki/trust/g8eg-ca-bundle.pem https://localhost:8443/api/v1/health
```

---

## Next Steps

- **[Build Operator](build_operator.md)** - Build a custom g8e-compatible g8e Operator
- **[Connect Operator to Gateway](connect_operator_to_gateway.md)** - Deploy and use a g8e Operator
- **[Build Apps](build_apps.md)** - Build g8e-compatible applications using a Gateway
- **[Connect a Lovable App](lovable.md)** - Connect a browser-hosted Lovable app to a local Gateway
- **[MCP Protocol](../../protocol/docs/mcp.md)** - Detailed MCP protocol specification
- **[A2A Protocol](../../protocol/docs/a2a.md)** - Detailed A2A protocol specification
- **[Gateway Architecture](../architecture/gateway.md)** - Gateway architecture and internals
- **[Protocol Library](../architecture/protocol.md)** - Go module and Python package API reference, constants, models, and usage examples
