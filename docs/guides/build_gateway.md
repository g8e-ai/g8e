---
title: Build Gateway
parent: Guides
---

# Build a g8e Gateway

Last Updated: 2026-07-19
Version: v1.5.9

---

## Overview

A g8e Gateway implements the central Policy Decision Point (PDP) of the platform. It provides PKI management, persistence, messaging, admission APIs, and protocol translation for MCP/A2A requests.

The reference implementation is the g8e binary file running in gateway mode. The same g8e binary file operates in two modes: g8e Operator mode (connects to a remote gateway) and g8e Gateway mode (acts as the platform's central persistence and pub/sub broker). Custom gateway implementations must implement the same protocol contracts and invariants.

---

## Reference Implementation

### Prerequisites

- **Go 1.26.5+** - Required for building the reference gateway.
- **Make** - Required to run build targets.

> **Don't have `make` or `go` installed?** Run the setup script for your platform to detect and install them automatically:
> - **Linux:** `bash scripts/linux-setup.sh`
> - **macOS:** `bash scripts/macos-setup.sh`
> - **Windows:** `pwsh scripts/windows-setup.ps1`

### Build from Source

Clone the repository and build the g8e binary file:

```bash
git clone https://github.com/g8e-ai/g8e.git && cd g8e
make build
```

This produces the `g8e` binary in the repository root and platform-specific binaries in the `bin/` directory. All dependencies are resolved at build time; the compiled binary is statically linked (`CGO_ENABLED=0`) and has zero runtime dependencies. No Go toolchain, OpenSSL, or other external tools are needed on the target host.

### Build Targets

The Makefile provides several build targets:

- `make build` - Builds the g8e binary file for the current platform.
- `make build-all` - Builds the g8e binary file for all platforms (linux, windows, darwin).
- `make build-linux` - Builds g8e binary file for Linux (amd64, arm64, 386).
- `make build-windows` - Builds g8e binary file for Windows (amd64, arm64).
- `make build-darwin` - Builds g8e binary file for Darwin (amd64, arm64).
- `make build-compressed` - Builds g8e binary file then compresses with UPX (requires UPX installed).
- `make clean` - Removes compiled g8e binaries and test artifacts.

### Build in Docker (no local Go required)

If Go 1.26+ is not installed locally, the binary can be compiled inside Docker. Requires Docker 24.0+.

```bash
make build-docker
```

This builds a `g8e-builder` image from the `builder` stage of the Dockerfile and runs the Go compiler inside it. The output binary is placed in `bin/g8e-linux-amd64`.

Additional Docker build targets:

- `make build-docker` - Linux amd64 only.
- `make build-linux-docker` - Linux: amd64, arm64, 386.
- `make build-darwin-docker` - macOS: amd64, arm64.
- `make build-windows-docker` - Windows: amd64, arm64.
- `make build-all-docker` - All platforms.

All Docker build outputs are placed in `bin/` with a `.sha256` checksum file alongside each binary.

### Cross-Compilation

To build for different target platforms:

```bash
GOOS=linux GOARCH=amd64 make build
GOOS=darwin GOARCH=arm64 make build
GOOS=windows GOARCH=amd64 make build
```

### Windows Build

On Windows, use `make build` with Go and Make installed, or invoke `go build` directly:

```powershell
go build -o g8e.exe ./cmd/g8e
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

- `--posture <mode>` - g8e Gateway posture: doctrine (L1 enforced, L2/L3 audited, default), consensus (L1/L2 enforced, L3 audited), notary (L1/L2/L3 strictly enforced)
- `--http-port <port>` - Plain HTTP port for bootstrap, health checks, and PKI discovery (default: 8080)
- `--https-port <port>` - HTTPS port for mTLS API and public surface (default: 8443)
- `--data-dir <dir>` - Data directory for SQLite database (default: .g8e/data in working directory)
- `--pki-dir <dir>` - Directory for TLS certificates (default: .g8e/pki)
- `--secrets-dir <dir>` - Directory for platform secrets (default: .g8e/secrets)
- `--vault-dir <dir>` - Directory for vault data (default: .g8e/vault)
- `--vault-key <path>` - Path to vault private key (default: .g8e/secrets/key)
- `--passkey-rp-id <id>` - RP ID for passkey operations (default: localhost)
- `--passkey-rp-name <name>` - RP Name for passkey operations (default: g8e)
- `--passkey-rp-origin <origin>` - Additional RP origin for passkey operations (repeatable, e.g. http://localhost:8087)
- `--rate-limit-rps <rps>` - Gateway requests per second limit (set to 0 to disable, default: 0)
- `--rate-limit-burst <burst>` - Gateway rate limit burst size (default: 0)
- `--log <level>` - Log level: info, error, debug (default: info)
- `--cert-mode <mode>` - Certificate mode: full (all hostnames/IPs), localhost (only localhost)
- `--tribunal-id <id>` - ID of the TribunalPolicy for L2 consensus (required for consensus posture)
- `--tribunal-url <url>` - URL of the Tribunal service for L2 deliberation (e.g., https://localhost:8443/tribunal/v1/deliberate)
- `--tribunal-bootstrap <path>` - Path to a JSON file that seeds a TribunalPolicy and trusted signers at startup
- `--mcp-downstream-url <url>` - URL of a downstream MCP server to proxy discovery and execution to (default: none)
- `--a2a-downstream-url <url>` - URL of a downstream A2A server to proxy execution to (default: none)
- `--public-base-url <url>` - Public base URL for approval links and host validation behind reverse proxies or Cloudflare Tunnels (e.g., `https://demo.g8e.ai`)
- `--cors-origin <origin>` - Allowed CORS origin for cross-origin browser access (repeatable, e.g., `https://lovable.dev`)
- `-f, --follow` - Run gateway in foreground instead of background (Ctrl+C stops gateway)
- `-i, --interactive` - Launch the interactive onboarding wizard before starting the gateway

---

## Protocol Library Dependencies

Custom gateway implementations need the g8e Protocol Library for protobuf schema definitions, SPIFFE workload identity helpers, and JSON protocol constants. The protocol is published as both a Go module and a Python package, both sharing the same version number as the platform binary.

### Go Module

The protocol is part of the root Go module `github.com/g8e-ai/g8e`. Add it to your project:

```bash
go get github.com/g8e-ai/g8e@v1.5.8
```

Import the protobuf types and SPIFFE workload identity helpers from the Go module. The package provides governance envelope definitions, the Operator gRPC service, pub/sub message types, and workload identity helpers for SPIFFE URI SAN generation and validation across all identity types (Operator, CLI, App, User, Hub, GatewayPeer).

See the [Protocol Library documentation](../architecture/protocol.md) for the full API reference and example programs.

### Python Package

For gateway-side tooling, testing, or Python-based services that need to consume protocol constants:

```bash
pip install g8e==1.5.8
```

The package provides `g8e.constants` (JSON protocol constants), `g8e.enums` (dynamic enums from protocol constants), and `g8e.models` (Pydantic v2 models). Requires Python 3.10+. See the [Protocol Library documentation](../architecture/protocol.md) for the full API reference.

---

## Custom Gateway Implementation

To build a custom g8e-compatible gateway, your implementation must satisfy the following protocol contracts.

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
- **Mutation Channels**: Restrict `cmd:*` and `scenarios:*` channels to envelope-based mutations only.
- **Non-Mutation Channels**: Allow direct publishing to `heartbeat:*`, `results:*`, `sse:*`, `ws_session:*`, `internal:*`.
- **Subscribe-and-Wait**: Require subscribers to wait for the broker's subscription acknowledgment before publishing.

#### 4. Admission APIs

The gateway must expose HTTP endpoints:

- **Envelope Submission**: `POST /api/v1/governance/envelopes` for canonical JSON GovernanceEnvelope transactions.
- **Device Enrollment**: `POST /api/v1/pki/devices/enroll` for CSR-based device enrollment (Operator and CLI certificates).
- **Certificate Revocation**: `POST /api/v1/pki/certificates/revoke` for certificate revocation.
- **Revocation Bundle**: `GET /api/v1/pki/revocation-bundle` for the signed revocation list.
- **MCP Endpoint**: `POST /mcp` for JSON-RPC MCP tool calls (Streamable HTTP transport).
- **A2A Endpoint**: `POST /api/v1/a2a/call` for HTTP/JSON A2A skill invocations.
- **CSR Signing**: `POST /api/v1/pki/csr/sign` for certificate signing request processing.
- **CRL Distribution**: `GET /.well-known/g8e/pki/crl` for the certificate revocation list.
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

- **Doctrine Mode**: Enforce L1 technical bedrock (forbidden patterns, blacklist, whitelist). L2/L3 signatures audited but not required. This is the default mode.
- **Consensus Mode**: Enforce L1 and L2 (multi-model Byzantine consensus). L3 signature audited but not required.
- **Notary Mode**: Enforce L1, L2, and L3 (human-in-the-loop via WebAuthn/FIDO2).

### Session Types

The gateway must enforce strict separation between session types:

- **Operator Session**: Authenticates host-side operators via mTLS certificates bound to operator_session_id.
- **CLI Session**: Authenticates BYO/CLI clients via mTLS certificates bound to cli_session_id.
- **Web Session**: Authenticates browser-based clients via passkey (WebAuthn) bound to web_session_id.

Session routing must be disjoint. A web_session_id can never receive events intended for a cli_session_id.

### Two-Port Architecture

The gateway exposes two ports with distinct transport and authentication properties:

| Port | Transport | Client Cert | Purpose |
|---|---|---|---|
| **HTTP 8080** | Plain HTTP | None | Health checks, state endpoint, bootstrap, PKI discovery, CSR signing, deploy scripts |
| **HTTPS 8443** | TLS 1.3 | Verified when present | API, pub/sub, console, MCP, A2A, governance, audit |

The HTTPS port uses `tls.VerifyClientCertIfGiven`: client certificates are accepted and verified when present, but not required at the TLS layer. This allows browser clients (console, WebAuthn flows) to reach public routes without a client certificate. mTLS enforcement for protected routes happens at the application layer via route classification: each route is assigned an auth mode (none, mTLS, web session, or dual), and requests are fail-closed for unknown routes.

---

## Protocol Schema

The GovernanceEnvelope schema is defined in the protocol protobuf files. Your implementation must:

1. **Use the canonical protojson wire format** for all client-facing interactions.
2. **Implement the typed payload validation** defined in the protocol schemas.
3. **Support the canonical request payload mappings** for all first-class event types.

Refer to the [Protocol Library documentation](../architecture/protocol.md) for the canonical schema definitions.

---

## Testing

The CLI provides tiered test subcommands:

```bash
./g8e test unit         # Tier 1: unit tests (no external dependencies)
./g8e test integration  # Tier 2: in-process integration tests
./g8e test e2e          # Tier 3: live platform E2E (requires running gateway)
./g8e test coverage     # Tests with coverage report
./g8e test lint         # Static analysis
./g8e test chaos        # Generate governance events for chaos testing
./g8e test summary      # View chaos test summary from test vault
```

The integration and E2E suites verify:
- Pub/Sub command dispatch
- Audit vault writes
- Ledger commits
- L1/L2/L3 verification gates
- Envelope validation
- State root computation
- Nonce management
- PKI operations
- MCP/A2A protocol translation

For comprehensive testing including all unit and integration tests, use:

```bash
make test
```

For the full CI pipeline (lint, vulnerability scanning, proto verification, and tests with 75% coverage enforcement):

```bash
make ci
```

---

## Manage

Manage the gateway lifecycle and configuration:

### Gateway Stop

Stop the running gateway:

```bash
./g8e gw stop
```

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

### Gateway Setup

Run the interactive setup wizard to configure gateway settings such as posture, tribunal, passkey, CORS, and certificate options:

```bash
./g8e gw setup
```

Any flags provided on the command line are used as initial values in the wizard. The wizard guides you through each setting and produces a resolved configuration. Run `./g8e gw start` afterward to launch the gateway with the configured settings.

### Security Validation

Run security validation checks on the local PKI and secrets directories:

```bash
./g8e gw security validate
```

Enroll an operator with the gateway via CSR to obtain mTLS certificates:

```bash
./g8e gw security pki enroll -e <gateway-ip>
```

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
- Endpoint URLs for bootstrap, public API, Console UI, and MCP

### Gateway Logs

View gateway logs in real-time:

```bash
./g8e gw logs -f
```

The `-f` flag follows log output (like `tail -f`). Use without `-f` to view historical logs.

### Data Query

Query the gateway's data store for operators, users, settings, documents, and audit events:

```bash
./g8e gw data operators
./g8e gw data users
./g8e gw data settings
./g8e gw data store --collection <name> [--document-id <id>]
./g8e gw data audit list --operator-session-id <session-id>
./g8e gw data audit summary [--operator-session-id <session-id>]
```

### Cloudflare Tunnel

Manage a Cloudflare Tunnel to expose the local gateway to the internet without opening firewall ports:

```bash
./g8e gw tunnel create --name <tunnel-name> --hostname <your-domain>  # Create tunnel and route DNS
./g8e gw tunnel run                                                   # Start tunnel (foreground)
./g8e gw tunnel status                                                # Check tunnel connectivity
```

Requires `cloudflared` installed and a Cloudflare account with a registered domain. See [Cloudflare Tunnel](cloudflare_tunnel.md) for detailed setup.

---

## Next Steps

- **[Connect Apps to Gateway](connect_apps_to_gateway.md)** - Connect to, authenticate, use, maintain, and pull reports from a Gateway.
- **[Build Operator](build_operator.md)** - Build a custom g8e-compatible Operator.
- **[Protocol Library](../architecture/protocol.md)** - Go module and Python package API reference, constants, models, and usage examples.
