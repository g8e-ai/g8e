---
title: Build Gateway
parent: Guides
---

# Build a g8e Gateway

Last Updated: 2026-06-01
Version: v1.0.5

---

## Overview

A g8e-compatible g8e Gateway implements the central Policy Decision Point (PDP) of the platform. It provides PKI management, persistence, messaging, admission APIs, and protocol translation for MCP/A2A requests.

The reference implementation is the g8e Node running in gateway mode. The same g8e Node operates in two modes: g8e Operator mode (connects to a remote gateway) and g8e Gateway mode (acts as the platform's central persistence and pub/sub broker). Custom gateway implementations must implement the same protocol contracts and invariants.

---

## Reference Implementation

### Prerequisites

- **Go 1.26+** — Required for building the reference gateway.
- **OpenSSL** — Required for PKI operations during runtime.
- **Git** — Required for the audit vault's Git-backed commit history.

### Build from Source

Clone the repository and build the g8e Node:

```bash
git clone https://github.com/g8e-ai/g8e.git && cd g8e
make build
```

This produces the `g8e` g8e Node in the repository root and platform-specific binaries in the `bin/` directory. The g8e Node is statically linked and requires no runtime dependencies.

### Build Targets

The Makefile provides several build targets:

- `make build` — Builds the g8e Node for all platforms (linux, windows, darwin).
- `make build-linux` — Builds g8e Node for Linux (amd64, arm64, 386).
- `make build-windows` — Builds g8e Node for Windows (amd64, arm64).
- `make build-darwin` — Builds g8e Node for Darwin (amd64, arm64).
- `make build-compressed` — Builds g8e Node for all platforms with UPX compression.
- `make clean` — Removes compiled g8e Nodes and test artifacts.

### Cross-Compilation

To build for different target platforms:

```bash
GOOS=linux GOARCH=amd64 make build
GOOS=darwin GOARCH=arm64 make build
GOOS=windows GOARCH=amd64 make build
```

### Windows Build

On Windows, use the provided PowerShell build script:

```powershell
.\build.ps1
```

For cross-compilation from Linux/macOS to Windows:

```bash
GOOS=windows GOARCH=amd64 make build
# Output: g8e (rename to g8e.exe on Windows)
```

The Makefile also includes a dedicated Windows build target:

```bash
make build-windows
```

This builds for both amd64 and arm64 architectures.

### Running in Gateway Mode

To start the gateway, use the CLI gateway command:

```bash
./g8e gw start --posture doctrine    # L1 enforced, L2/L3 audited (default)
./g8e gw start --posture consensus   # L1/L2 enforced, L3 audited
./g8e gw start --posture notary      # L1/L2/L3 strictly enforced
```

### Gateway Mode Flags

- `--posture <mode>` — g8e Gateway posture: doctrine (L1 enforced, L2/L3 audited, default), consensus (L1/L2 enforced, L3 audited), notary (L1/L2/L3 strictly enforced)
- `--http-port <port>` — Plain HTTP port for bootstrap and MCP routes (default: 8080)
- `--https-port <port>` — HTTPS port for mTLS API and public surface (default: 8443)
- `--data-dir <dir>` — Data directory for SQLite database (default: .g8e/data in working directory)
- `--pki-dir <dir>` — Directory for TLS certificates (default: .g8e/pki)
- `--secrets-dir <dir>` — Directory for platform secrets (default: .g8e/secrets)
- `--passkey-rp-id <id>` — RP ID for passkey operations (default: localhost)
- `--passkey-rp-name <name>` — RP Name for passkey operations (default: g8e)
- `--rate-limit-rps <rps>` — Gateway requests per second limit (default: 0, disabled)
- `--rate-limit-burst <burst>` — Gateway rate limit burst size (default: 10)
- `--cert-mode <mode>` — Certificate mode: full (all hostnames/IPs), localhost (only localhost)
- `--network-identity-file <path>` — Path to JSON file containing pre-detected network identity

---

## Custom Gateway Implementation

To build a custom g8e-compatible g8e Gateway, your implementation must satisfy the following protocol contracts.

### Required Capabilities

#### 1. PKI and Trust Management

The gateway must act as the platform Certificate Authority:

- **Root CA**: Generate and maintain a root CA certificate.
- **Intermediate CAs**: Issue scoped intermediate CAs for different participant types (Hub, Operator, Bootstrap).
- **CSR-Based Enrollment**: Accept Certificate Signing Requests (CSRs) and issue signed certificates with SPIFFE URI SANs.
- **Certificate Revocation**: Maintain a revocation list and enforce it at the gateway boundary.
- **Trust Bundles**: Serve trust bundles for client verification.

#### 2. Persistence Layer

The gateway must maintain canonical platform state:

- **Document Store**: JSON document CRUD on a Collection/ID pattern with query support.
- **KV Store**: TTL-aware ephemeral state with pattern scanning and cursor-based iteration.
- **Blob Store**:Node Node Binary persistence for attachments and large objects.
- **State Root Provider**: Compute and serve a deterministic Merkle state root across all authoritative data.
- **Nonce Manager**: Implement sliding-window replay protection for governance transactions.

#### 3. Messaging Broker

The gateway must serve as the Pub/Sub broker:

- **WebSocket Fan-Out**: Real-time event streaming to subscribed clients.
- **Channel Format**: Use the `{prefix}:{operator_id}:{operator_session_id}` channel format.
- **Mutation Channels**: Restrict `cmd:*` and `auditor:*` channels to envelope-based mutations only.
- **Non-Mutation Channels**: Allow direct publishing to `heartbeat:*`, `results:*`, `sse:*`, `ws_session:*`, `internal:*`.
- **Subscribe-and-Wait**: Require subscribers to wait for the broker's subscription acknowledgment before publishing.

#### 4. Admission APIs

The gateway must expose HTTP endpoints:

- **Envelope Submission**: `POST /api/v1/governance/envelopes` for canonical JSON GovernanceEnvelope transactions.
- **Device Enrollment**: `POST /api/v1/pki/devices/enroll` for CSR-based device enrollment (Operator and CLI certificates).
- **Certificate Revocation**: `POST /api/v1/pki/certificates/revoke` for certificate revocation.
- **Revocation Bundle**: `GET /api/v1/pki/revocation-bundle` for the signed revocation list.
- **MCP Endpoint**: `POST /api/v1/mcp` for JSON-RPC MCP tool calls.
- **Trust Bundle**: `GET /.well-known/g8e/pki/ca-bundle` for the CA trust bundle.

#### 5. Protocol Translation

The gateway must translate standard protocols into governed operations:

- **MCP Translation**: Accept JSON-RPC MCP tool calls and wrap them in GovernanceEnvelope format.
- **A2A Translation**: Accept HTTP/JSON A2A skill invocations and wrap them in GovernanceEnvelope format.
- **Canonical JSON**: Use protojson (canonical JSON) as the wire format for all client-facing interactions.

#### 6. Audit Authority

The gateway must maintain an authoritative audit trail:

- **Encrypted Audit Vault**: Store audit entries keyed by transaction_hash.
- **ActionReceipts**: Emit signed receipts for every governed mutation.
- **Fail-Closed Writes**: Reject events with missing or unknown operator_session_id.

### Protocol Invariants

Your implementation must enforce these core invariants:

1. **Transaction Hash Verification**: The envelope `id` must match the deterministic transaction_hash computed from its content.
2. **State Binding**: Every transaction must include a state root and be verified against the current authoritative state.
3. **Replay Defense**: Nonces must be validated against a sliding window to prevent replay attacks.
4. **Expiry Enforcement**: Transactions must be rejected if they have expired.
5. **Fail-Closed Execution**: Any verification failure must result in a typed rejection and audit entry. No fallback paths or silent retries.

### Governance Modes

The gateway must support three operating modes:

- **Doctrine Mode**: Enforce L1 technical bedrock (forbidden patterns, blacklist, whitelist). L2/L3 signatures not required. This is the default mode.
- **Consensus Mode**: Enforce L1 and L2 (multi-model Byzantine consensus). L3 signature not required.
- **Notary Mode**: Enforce L1, L2, and L3 (human-in-the-loop via WebAuthn/FIDO2).

### Session Types

The gateway must enforce strict separation between session types:

- **Operator Session**: Authenticates host-side operators via mTLS certificates bound to operator_session_id.
- **CLI Session**: Authenticates BYO/CLI clients via mTLS certificates bound to cli_session_id.
- **Web Session**: Authenticates browser-based clients via passkey (WebAuthn) bound to web_session_id.

Session routing must be disjoint. A web_session_id can never receive events intended for a cli_session_id.

### Multiplexed Port Contract

The gateway must support two logical protocol surfaces with distinct authentication requirements:

| Surface | Auth | Purpose |
|---|---|---|
| **Public Port** | TLS (no client cert) | Browser login, WebAuthn challenge, PKI discovery |
| **mTLS API + Pub/Sub** | TLS + RequireAndVerifyClientCert | Envelope submission, persistence, pub/sub |

Surfaces with different TLS client-auth requirements MUST NOT share a port. Sharing would force `tls.VerifyClientCertIfGiven` and downgrade the mTLS execution boundary to an L7 check. The reference implementation enforces this by validating port assignments during initialization.

---

## Protocol Schema

The GovernanceEnvelope schema is defined in the protocol protobuf files. Your implementation must:

1. **Use the canonical protojson wire format** for all client-facing interactions.
2. **Implement the typed payload validation** defined in the protocol schemas.
3. **Support the canonical request payload mappings** for all first-class event types.

Refer to `protocol/proto/g8e/` for the canonical schema definitions.

---

## Testing

A custom gateway implementation must pass the platform test suite to claim g8e compatibility:

```bash
./g8e test
```

This verifies:
- Pub/Sub command dispatch
- Audit vault writes
- Ledger commits
- L1/L2/L3 verification gates
- Envelope validation
- State root computation
- Nonce management
- PKI operations
- MCP/A2A protocol translation

For comprehensive testing including integration tests, use:

```bash
make test
```

For CI-quality testing with coverage enforcement:

```bash
make ci
```

---

## Manage

Manage the gateway lifecycle and configuration:

### Gateway Restart

Restart the gateway without stopping it manually:

```bash
./g8e gw restart
```

### Gateway Settings

View and manage gateway configuration:

```bash
./g8e gw settings
```

### Gateway Reset

Reset gateway data and secrets while preserving the CA:

```bash
./g8e gw reset
```

Use `--force` or `--yes` to skip the confirmation prompt.

### Gateway Clean

Destructively remove all gateway state including databases, secrets, logs, and PKI certificates:

```bash
./g8e gw clean
```

**Warning:** This permanently destroys all trust routes and credentials. Use `--force` or `--yes` to skip the confirmation prompt.

---

## Monitor

Monitor gateway status, logs, and data:

### Gateway Status

Check the gateway health and view endpoint information:

```bash
./g8e gw status
```

This displays:
- Gateway state (RUNNING/STOPPED) and PID
- Endpoint URLs for bootstrap, public API, and MCP

### Gateway Logs

View gateway logs in real-time:

```bash
./g8e gw logs -f
```

The `-f` flag follows log output (like `tail -f`). Use without `-f` to view historical logs.

### Data Query

Query the gateway's data store for operators, users, and audit events:

```bash
./g8e data operators
./g8e data users
./g8e data audit list --operator-session-id <session-id>
```

---

## Next Steps

- **[Connect Apps to Gateway](connect_apps_to_gateway.md)** — Connect to, authenticate, use, maintain, and pull reports from a Gateway.
- **[Build Operator](build_operator.md)** — Build a custom g8e-compatible g8e Operator.
