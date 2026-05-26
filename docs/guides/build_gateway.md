---
title: Build Gateway
parent: Guides
---

# Build a Governance Gateway

Last Updated: 2026-05-26
Version: v1.0.0

---

## Overview

A g8e-compatible Governance Gateway implements the central Policy Decision Point (PDP) of the substrate. It provides PKI management, persistence, messaging, admission APIs, and protocol translation for MCP/A2A requests.

The reference implementation is the g8e Operator binary running in gateway mode. The same `g8e.operator` binary operates in two modes: Operator mode (connects to a remote gateway) and Gateway mode (acts as the platform's central persistence and pub/sub broker). Custom gateway implementations must implement the same protocol contracts and invariants.

---

## Reference Implementation

### Prerequisites

- **Go 1.26+** — Required for building the reference gateway.
- **OpenSSL** — Required for PKI operations during runtime.
- **Git** — Required for the audit vault's Git-backed commit history.

### Build from Source

Clone the repository and build the operator binary:

```bash
git clone https://github.com/g8e-ai/g8e.git && cd g8e
make build
```

This produces the `g8e.operator` binary in the repository root. The binary is statically linked and requires no runtime dependencies.

### Build Targets

The Makefile provides several build targets:

- `make build` — Builds the `g8e.operator` binary.
- `make clean` — Removes compiled binaries and test artifacts.

### Cross-Compilation

To build for different target platforms:

```bash
GOOS=linux GOARCH=amd64 make build
GOOS=darwin GOARCH=arm64 make build
```

### Running in Gateway Mode

To start the gateway, run the operator binary with a gateway mode flag:

```bash
./g8e.operator --doctrine          # L1 enforced, L2/L3 audited
./g8e.operator --consensus         # L1/L2 enforced, L3 audited
./g8e.operator --notary            # L1/L2/L3 strictly enforced
```

### Gateway Mode Flags

- `--doctrine` — Gateway mode: L1 enforced, L2/L3 audited (default)
- `--consensus` — Gateway mode: L1/L2 enforced, L3 audited
- `--notary` — Gateway mode: L1/L2/L3 strictly enforced
- `--http-listen-port <port>` — HTTPS port for mTLS API (default: 8440)
- `--bootstrap-listen-port <port>` — Bootstrap TLS port for device-link enrollment (default: 8441)
- `--public-listen-port <port>` — Public browser/BYO bootstrap port (default: 8442)
- `--data-dir <dir>` — Data directory for SQLite database (default: .g8e/data in working directory)
- `--pki-dir <dir>` — Directory for TLS certificates (default: .g8e/pki)
- `--secrets-dir <dir>` — Directory for platform secrets (default: .g8e/secrets)
- `--passkey-rp-id <id>` — RP ID for passkey operations (default: localhost)
- `--passkey-rp-name <name>` — RP Name for passkey operations (default: g8e)

---

## Custom Gateway Implementation

To build a custom g8e-compatible Governance Gateway, your implementation must satisfy the following protocol contracts.

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
- **Blob Store**: Binary persistence for attachments and large objects.
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

- **Envelope Submission**: `POST /api/governance/envelope` for canonical JSON GovernanceEnvelope transactions.
- **Trust Bundle Distribution**: `GET /.well-known/g8e/pki/hub-bundle.pem` for CA certificates.
- **Root CA**: `GET /.well-known/g8e/pki/root.pem` for the root CA certificate.
- **PKI Fingerprint**: `GET /.well-known/g8e/pki/fingerprint` for the root CA SHA-256 fingerprint.
- **Device-Link Enrollment**: `POST /api/auth/device-link/request` and `POST /api/auth/device-link/register` for operator enrollment.
- **CSR Signing**: `POST /api/pki/sign-csr` for signing CSRs during enrollment.
- **App Enrollment**: `POST /api/pki/app-enroll` for external app enrollment (API key required).
- **Certificate Revocation**: `POST /api/pki/revoke` for certificate revocation.
- **Revocation Bundle**: `GET /api/pki/revocation-bundle` for the signed revocation list.

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

- **Doctrine Mode**: Enforce L1 technical bedrock (forbidden patterns, blacklist, whitelist). L2 and L3 are audited but not enforced.
- **Consensus Mode**: Enforce L1 and L2 (multi-model Byzantine consensus). L3 is audited but not enforced.
- **Notary Mode**: Enforce L1, L2, and L3 (human-in-the-loop via WebAuthn/FIDO2).

### Session Types

The gateway must enforce strict separation between session types:

- **Operator Session**: Authenticates host-side operators via mTLS certificates bound to operator_session_id.
- **CLI Session**: Authenticates BYO/CLI clients via mTLS certificates bound to cli_session_id.
- **Web Session**: Authenticates browser-based clients via passkey (WebAuthn) bound to web_session_id.

Session routing must be disjoint. A web_session_id can never receive events intended for a cli_session_id.

### Multiplexed Port Contract

The gateway must support three logical protocol surfaces with distinct authentication requirements:

| Surface | Auth | Purpose |
|---|---|---|
| **Bootstrap** | None (plain HTTP) | Trust bundle download, device-link enrollment, CSR signing |
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

A custom gateway implementation must pass the substrate test suite to claim g8e compatibility:

```bash
./g8e test g8eo
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

---

## Next Steps

- **[Connect Apps to Gateway](connect_apps_to_gateway.md)** — Connect to, authenticate, use, maintain, and pull reports from a Gateway.
- **[Build Operator](build_operator.md)** — Build a custom g8e-compatible Governed Operator.
