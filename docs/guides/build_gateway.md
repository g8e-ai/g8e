---
title: Build Gateway
parent: Guides
---

# Build a g8e Gateway

Last Updated: 2026-09-08
Version: v2.1.7

---

## Overview

A g8e Gateway implements the platform's Policy Decision Point (PDP). It manages PKI, canonical platform state, governance admission, messaging, and MCP/A2A protocol translation. The gateway owns L1 Doctrine, L2 Consensus, and L3 Notary decisions; an in-process Operator substrate applies L4 Warden and L5 Actuator for operations that target the gateway host.

The reference implementation is the static `g8e` binary running in gateway mode. The same binary runs as a Governed Operator, the Policy Execution Point (PEP) that connects outbound to a gateway and applies L4-L5 on a managed host. Custom implementations conform to the same protocol contracts and fail-closed invariants described in the [g8e Protocol specification](../../protocol/docs/spec.md).

---

## Reference Implementation

### Prerequisites

- **Go 1.26.6+** - Required for building the reference gateway.
- **Make** - Required to run build targets.

> **Don't have `make` or `go` installed?** Run the setup script for your platform to detect and install them automatically:
> - **Linux:** `bash scripts/linux-setup.sh`
> - **macOS:** `bash scripts/macos-setup.sh`
> - **Windows:** `pwsh scripts/windows-setup.ps1`

### Build from Source

Clone the repository and build the `g8e` binary:

```bash
git clone https://github.com/g8e-ai/g8e.git && cd g8e
make build
```

This produces the host binary as `g8e` in the repository root (`g8e.exe` on Windows), a named platform binary and SHA-256 checksum in `bin/`, and a copy at `demos/bin/g8e`. The binary is statically linked with `CGO_ENABLED=0` and has no runtime dependency on the Go toolchain, OpenSSL, or another external library.

### Build Targets

The Makefile provides several build targets:

- `make build` - Builds `g8e` for the current platform.
- `make build-all` - Builds `g8e` for every supported Linux, Windows, and macOS target.
- `make build-linux` - Builds Linux binaries for amd64, arm64, and 386.
- `make build-windows` - Builds Windows binaries for amd64 and arm64.
- `make build-darwin` - Builds macOS binaries for amd64 and arm64.
- `make build-compressed` - Builds the current-platform binary and compresses the named `bin/` artifact with UPX (requires UPX installed).
- `make build-fips` - Builds a FIPS 140-3 approved mode g8e binary for linux/amd64.
- `make verify-fips` - Builds the FIPS variant and runs its self-check with FIPS enforcement enabled.
- `make clean` - Removes `bin/`, test and coverage artifacts, the local `.g8e/` runtime tree, and the local Go build and module caches. It does not remove the repository-root `g8e` binary.

> **Warning:** `make clean` deletes the gateway state stored under the repository's `.g8e/` directory. Stop the gateway and preserve any required state before running it.

### Build in Docker (no local Go required)

Docker 24.0+ with the Docker Compose plugin can build the binaries without a local Go toolchain:

```bash
make up
```

This runs `docker compose up -d --build`. The builder stage runs `make build-all`, creates the `g8e-gateway` image, and starts only the gateway because the Operator, Dashboard, and Ensemble use the `bootstrapped` Compose profile. Copy the Linux CLI binary from the default gateway container when a host-side binary is needed:

```bash
docker cp g8e-gateway:/g8e ./g8e
chmod +x ./g8e
```

The first-owner and workload enrollment steps happen before the remaining services start. See [Docker Gateway](docker_gateway.md) for the bootstrap sequence and [Unified Stack](unified_stack.md) for the complete two-phase deployment.

The Dockerfile builder produces binaries for linux/amd64, linux/arm64, linux/386, windows/amd64, windows/arm64, darwin/amd64, and darwin/arm64. The gateway serves these artifacts through the node deployment download surface. Linux builds link the Go Cryptographic Module through `GOFIPS140=v1.0.0`; the project's FIPS compliance claim is restricted to linux/amd64, and enforcement requires `GODEBUG=fips140=only` at runtime.

### Cross-Compilation

To build for different target platforms:

```bash
GOOS=linux GOARCH=amd64 make build
GOOS=darwin GOARCH=arm64 make build
GOOS=windows GOARCH=amd64 make build
```

### Windows Build

On Windows, use the standard target to produce `g8e.exe` for the current architecture with the canonical build tags and version metadata:

```powershell
make build
```

From Linux or macOS, build both supported Windows architectures and their SHA-256 checksum files with:

```bash
make build-windows
```

The outputs are `bin/g8e-windows-amd64.exe` and `bin/g8e-windows-arm64.exe`.

### Run the Gateway

Start the gateway as a managed background process. Doctrine is the default posture:

```bash
./g8e gw start
```

Use `--follow` to run in the foreground, which is appropriate for containers and supervised services:

```bash
./g8e gw start --follow
```

On first start, the gateway creates the `.g8e/` runtime tree and PKI hierarchy. It defaults to plain HTTP on port 8080 and HTTPS on port 8443; resolved ports can shift when defaults are occupied, and startup fails if it cannot reserve a valid distinct pair. Confirm the resolved endpoints, then enroll the first owner:

```bash
./g8e gw status
./g8e auth enroll user -e localhost
```

The owner enrollment creates the local CLI identity, installs the gateway root CA into the operating system trust store, and registers a passkey. Use `--no-system-trust` only when an administrator has already installed the root CA. For a Docker deployment, start the `bootstrapped` profile after owner enrollment, then approve the pending Operator, Dashboard, and Ensemble requests that those workloads submit; [Docker Gateway](docker_gateway.md) documents that flow.

Choose a stricter posture only after configuring the proofs it requires:

```bash
./g8e gw start --posture consensus --consensus-id <policy-id>
./g8e gw start --posture ratify
./g8e gw start --posture notary --consensus-id <policy-id>
```

Consensus and notary require an enabled consensus policy, trusted signers, and an available in-process consensus service. `--consensus-bootstrap` can seed the policy and signer keys for deterministic deployments. Ratify and notary require L3 human authorization for mutations.

### Gateway Mode Flags

- `--posture <mode>` - g8e Gateway posture: doctrine (L1 enforced, L2/L3 audited, default), consensus (L1/L2 enforced, L3 audited), ratify (L1/L3 enforced, L2 audited), notary (L1/L2/L3 strictly enforced)
- `--http-port <port>` - Plain HTTP port for bootstrap, health checks, and PKI discovery (default: 8080)
- `--https-port <port>` - HTTPS port for mTLS API and public surface (default: 8443)
- `--data-dir <dir>` - Data directory for SQLite database (default: .g8e/data in working directory)
- `--pki-dir <dir>` - Directory for TLS certificates (default: .g8e/pki)
- `--secrets-dir <dir>` - Directory for platform secrets (default: .g8e/secrets)
- `--vault-dir <dir>` - Directory for vault data (default: .g8e/vault)
- `--vault-key <path>` - Path to the vault private key (default: `.g8e/vault/key`)
- `--passkey-rp-id <id>` - RP ID for passkey operations (default: localhost)
- `--passkey-rp-name <name>` - RP Name for passkey operations (default: g8e)
- `--passkey-rp-origin <origin>` - Additional RP origin for passkey operations (repeatable, e.g. http://localhost:8087)
- `--rate-limit-rps <rps>` - Gateway requests per second limit (set to 0 to disable, default: 0)
- `--rate-limit-burst <burst>` - Gateway rate limit burst size (default: 0)
- `--log <level>` - Log level: info, error, debug (default: info)
- `--cert-mode <mode>` - Certificate identity mode: `full` includes detected hostnames and IP addresses (default), while `localhost` uses only loopback identities
- `--consensus-id <id>` - ID of the enabled `ConsensusPolicy` used by the L2-enforcing `consensus` and `notary` postures
- `--consensus-url <url>` - Optional consensus service URL carried in startup configuration; the reference gateway currently deliberates through its in-process consensus service
- `--consensus-bootstrap <path>` - Path to a JSON file that seeds a ConsensusPolicy and trusted signers at startup
- `--mcp-downstream-url <url>` - URL of a downstream MCP server to proxy discovery and execution to (default: none)
- `--a2a-downstream-url <url>` - URL of a downstream A2A server to proxy execution to (default: none)
- `--public-base-url <url>` - Public base URL for approval links and host validation behind reverse proxies or Cloudflare Tunnels (e.g., `https://demo.g8e.ai`)
- `--cors-origin <origin>` - Allowed CORS origin for cross-origin browser access (repeatable, e.g., `https://lovable.dev`)
- `--doctrine-dir <dir>` - Directory containing doctrine JSON files for L1 threat detection (default: hardcoded MITRE patterns only)
- `-f, --follow` - Run gateway in foreground instead of background (Ctrl+C stops gateway)
- `-i, --interactive` - Launch the interactive onboarding wizard before starting the gateway

The root command also displays global `-e, --endpoint` and `-p, --port` flags. Those flags select the remote HTTP discovery and HTTPS/mTLS endpoints for enrollment and client commands; they do not configure the gateway's listening ports. Use `--http-port` and `--https-port` for the listeners.

When their corresponding flags are absent, the gateway reads vault, consensus, public base URL, passkey, CORS, and doctrine values from the `G8E_VAULT_DIR`, `G8E_VAULT_KEY`, `G8E_CONSENSUS_ID`, `G8E_CONSENSUS_URL`, `G8E_CONSENSUS_BOOTSTRAP`, `G8E_PUBLIC_BASE_URL`, `G8E_PASSKEY_RP_ID`, `G8E_PASSKEY_RP_NAME`, `G8E_PASSKEY_RP_ORIGINS`, `G8E_ALLOWED_ORIGINS`, and `G8E_DOCTRINE_DIR` environment variables. The origins variables accept comma-separated values.

---

## Protocol Library Dependencies

Custom gateway implementations need the g8e Protocol Library for protobuf schema definitions, SPIFFE workload identity helpers, and JSON protocol constants. The protocol is published as both a Go module and a Python package, both sharing the same version number as the platform binary.

### Go Module

The protocol is part of the root Go module `github.com/g8e-ai/g8e/v2`. Add it to your project:

```bash
go get github.com/g8e-ai/g8e/v2@v2.1.7
```

Import the protobuf types and SPIFFE workload identity helpers from the Go module. The package provides governance envelope definitions, the Operator gRPC service, pub/sub message types, and workload identity helpers for SPIFFE URI SAN generation and validation across all identity types (Operator, CLI, App, User, Hub, GatewayPeer).

See the [Protocol Library documentation](../architecture/protocol.md) for the full API reference and example programs.

### Python Package

For gateway-side tooling, testing, or Python-based services that need to consume protocol constants:

```bash
pip install g8e==2.1.7
```

The package provides `g8e.constants` (JSON protocol constants), `g8e.enums` (dynamic enums from protocol constants), and `g8e.models` (Pydantic v2 models). Requires Python 3.10+. See the [Protocol Library documentation](../architecture/protocol.md) for the full API reference.

---

## Custom Gateway Implementation

To build a custom g8e-compatible gateway, your implementation must satisfy the following protocol contracts.

### Required Capabilities

#### 1. PKI and Trust Management

The gateway must act as the platform Certificate Authority:

- **Root CA**: Generate and maintain a root CA certificate.
- **Intermediate CAs**: Maintain separate Hub, Operator, and Gateway Peer intermediate CAs. The Hub CA signs the gateway serving identity, the Operator CA signs Operator, CLI, and app leaf certificates, and the Gateway Peer CA signs peer identities.
- **CSR-Based Enrollment**: Accept Certificate Signing Requests (CSRs) and issue signed certificates with SPIFFE URI SANs.
- **Certificate Revocation**: Maintain a revocation list and enforce it at the gateway boundary.
- **Trust Bundles**: Serve trust bundles for client verification.

#### 2. Persistence Layer

The gateway must maintain canonical platform state, including JSON documents, TTL-aware ephemeral state, binary attachments, a deterministic state root across all authoritative data, and replay-protected transaction nonces.

#### 3. Messaging Broker

The gateway provides WebSocket pub/sub fan-out on identity- and session-scoped channels. Governed mutation channels accept `GovernanceEnvelope` transactions, while explicitly non-mutating traffic such as heartbeats, results, and session events uses typed direct messages. A publisher waits for the subscription acknowledgment before publishing to prevent startup message loss.

#### 4. Admission and Enrollment

The gateway exposes the protocol's bootstrap, recovery, platform enrollment, governance, PKI management, pub/sub, audit, MCP, and A2A surfaces. Public token-scoped enrollment operations remain separate from owner-authenticated decisions and mTLS-protected certificate management. See the [g8e Protocol specification](../../protocol/docs/spec.md) for canonical paths and request shapes.

#### 5. Protocol Translation

The gateway translates MCP JSON-RPC tool calls and A2A skill invocations into governed operations. Mutations use the canonical protojson representation of `GovernanceEnvelope`; read-only discovery remains typed but does not require a mutation envelope.

#### 6. Governance and Execution Integration

The gateway applies L1-L3 according to the selected posture and dispatches admitted envelopes to an in-process or remote Operator. The Operator applies L4-L5, persists signed executing and final receipts with deterministic stage evidence, and returns the result to the gateway. The gateway rejects missing, invalid, expired, replayed, stale-state, or under-authorized transactions before execution.

#### 7. Audit and Receipt Access

The gateway exposes authenticated audit event and receipt query surfaces. The execution boundary is authoritative for its local audit store, signed receipt, commitment chain, and file ledger; the gateway relays remote Operator results without replacing that host-local evidence.

### Protocol Invariants

Your implementation must enforce these core invariants:

1. **Transaction integrity**: The implementation enforces `id == transaction_hash == SHA256(canonical_fields)`.
2. **State binding**: Each mutation carries a state Merkle root that L4 compares with the current bound state.
3. **Replay defense**: L4 durably reserves each nonce before dispatch and rejects reuse.
4. **Expiry enforcement**: L4 rejects expired envelopes.
5. **Doctrine enforcement**: L1 validates the typed payload and rejects forbidden patterns and detected threats in every posture.
6. **Posture-aware proofs**: L2 signatures and L3 authorization are mandatory when the selected posture requires them; optional proof results remain audit evidence.
7. **Execution boundary**: Only L5 dispatches an admitted mutation, using a transaction-bound capability and producing signed execution evidence.
8. **Fail-closed behavior**: A malformed envelope, unknown identity or session, stale state, missing required proof, or failed verification stops execution with a typed rejection.

### Governance Modes

The gateway supports four governance postures:

- **Doctrine**: Enforces L1 technical bedrock (forbidden patterns, blacklist, whitelist). L2/L3 signatures are audited but not required. This is the default posture.
- **Consensus**: Enforces L1 and L2 using K-of-N signed votes from distinct trusted members. L3 proof is audited but not required.
- **Ratify**: Enforces L1 and L3 (human-in-the-loop via WebAuthn/FIDO2). L2 signatures are audited but not required.
- **Notary**: Enforces L1, L2, and L3.

### Session Types

The gateway must enforce strict separation between session types:

- **Operator Session**: Authenticates host-side operators via mTLS certificates bound to an operator session.
- **CLI Session**: Authenticates BYO/CLI clients via mTLS certificates bound to a CLI session.
- **Web Session**: Authenticates browser-based clients via passkey (WebAuthn) bound to a web session.

Session routing must be disjoint. A web session can never receive events intended for a CLI session.

### Two-Port Architecture

The gateway exposes two ports with distinct transport and authentication properties:

| Port | Transport | Client Cert | Purpose |
|---|---|---|---|
| **HTTP 8080** | Plain HTTP | None | Bootstrap health and state, CA discovery, initial owner bootstrap, token-scoped CLI recovery and platform enrollment, deploy scripts, and node binaries |
| **HTTPS 8443** | TLS 1.3 | Verified when present | Console and WebAuthn, public TLS routes, authenticated API and PKI management, pub/sub, MCP/A2A, governance, and audit |

The HTTP router exposes only bootstrap and discovery operations, then redirects other requests to HTTPS. It does not expose privileged CSR signing, certificate revocation, MCP/A2A, pub/sub, or governance routes.

The HTTPS listener requests and verifies client certificates when present but does not require one during the TLS handshake. This permits browser access to public Console and WebAuthn routes. Application middleware requires a verified SPIFFE identity, web session, or configured JWT on each protected route and rejects unknown routes.

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
./g8e test e2e          # Tier 3: tests against an already running, enrolled platform
./g8e test e2e-full     # Tier 3: Compose lifecycle wrapper with volume teardown
./g8e test coverage     # Integration-tagged tests with 75% coverage enforcement
./g8e test lint         # golangci-lint static analysis
./g8e test chaos        # Generate governance events for chaos testing
./g8e test summary      # View chaos test summary from test vault
```

The integration and E2E suites collectively cover pub/sub dispatch, audit and receipt persistence, commitment and ledger behavior, all five governance layers, envelope and state-root validation, nonce replay protection, PKI operations, enrollment, and MCP/A2A translation.

Run all Go unit and in-process integration tests with:

```bash
make test
```

Run the local platform, Ensemble, and Dashboard CI targets, including protocol generation, Swagger generation, lint, vulnerability checks, and component tests, with:

```bash
make ci
```

---

## Manage

Manage the gateway lifecycle and configuration:

### Gateway Stop

Stop the host gateway started as a managed background process:

```bash
./g8e gw stop
```

This command does not stop a foreground process or Docker Compose service.

### Gateway Restart

Restart the host gateway managed by the CLI:

```bash
./g8e gw restart
```

Restart preserves only the persisted posture. Other startup flags return to their defaults, so stop and start the gateway explicitly when it must retain custom ports, paths, origins, downstream URLs, or consensus bootstrap configuration.

### Gateway Settings

Display the running gateway's platform settings over the enrolled CLI's mTLS connection:

```bash
./g8e gw settings
```

### Gateway Reset

`gw reset` stops the managed gateway, runs the same full runtime cleanup used by `gw clean`, and starts a new doctrine-posture gateway:

```bash
./g8e gw reset
```

> **Warning:** The current reset path removes the complete `.g8e/` runtime tree, including the existing CA, and attempts to remove operating-system g8e trust anchors despite the command's built-in help text stating that it preserves PKI. Treat reset as destructive and enroll the owner again after it completes. Use `--force`, `--y`, or `--yes` to skip the confirmation prompt.

### Gateway Clean

Destructively remove the local CLI-managed gateway runtime state, including databases, secrets, logs, and PKI certificates:

```bash
./g8e gw clean
```

**Warning:** This permanently destroys the runtime tree and credentials and attempts to remove g8e root CA anchors from the operating system trust store. Use `--force`, `--y`, or `--yes` to skip the confirmation prompt.

### Gateway Setup

Run the interactive setup wizard to configure gateway settings such as posture, consensus, passkey, CORS, and certificate options:

```bash
./g8e gw setup
```

Flags provided to `gw setup` become initial wizard values. The command prints the resolved choices but does not save them. Start the gateway with the corresponding flags, or use `./g8e gw start --interactive` to run the wizard and apply its result to the same startup invocation.

### Security Validation

Validate the local PKI and secrets files, parse the root CA and trust bundle, and report whether the default gateway ports are available:

```bash
./g8e gw security validate
```

This command validates local state; it does not authenticate to a remote gateway. Enroll the local owner and CLI identity through the supported CSR and passkey flow with:

```bash
./g8e auth enroll user -e <gateway-host>
```

Operators and other platform workloads submit platform enrollment requests at startup. An enrolled owner reviews and decides those requests with `./g8e auth pending-platform-enrollments` and `./g8e auth approve-platform-enrollment <request-id>`.

---

## Monitor

Monitor gateway status, logs, and data:

### Gateway Status

Check the gateway health and view endpoint information:

```bash
./g8e gw status
```

This displays:
- Local gateway state and PID when available
- Endpoint URLs for bootstrap, public API, Console UI, and MCP
- Docker Compose gateway and workload status

### Gateway Logs

View logs from the host gateway started as a managed background process:

```bash
./g8e gw logs -f
```

The `-f` flag follows log output like `tail -f`. Use the command without `-f` to read the existing managed-process log. Foreground and container logs remain attached to their process supervisor or Docker.

### Data Query

Query the running gateway over mTLS for operators, users, settings, documents, and audit events. The audit summary command reads the local audit SQLite database directly:

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
./g8e gw tunnel run --name <tunnel-name>                              # Start tunnel (foreground)
./g8e gw tunnel status --name <tunnel-name> --hostname <your-domain>  # Check tunnel and public health
```

Requires `cloudflared` installed and a Cloudflare account with a registered domain. See [Cloudflare Tunnel](cloudflare_tunnel.md) for detailed setup.

---

## Next Steps

- **[Connect Apps to Gateway](connect_apps_to_gateway.md)** - Connect to, authenticate, use, maintain, and pull reports from a Gateway.
- **[Build Operator](build_operator.md)** - Build a custom g8e-compatible Operator.
- **[Protocol Library](../architecture/protocol.md)** - Go module and Python package API reference, constants, models, and usage examples.
