---
title: Build Operator
parent: Guides
---

# Build a g8e Operator

Last Updated: 2026-07-28
Version: v1.6.6

---

## Overview

A g8e-compatible Operator implements the host-side Policy Execution Point (PEP) of the platform. It receives transactions, enforces the 5-layer verification sequence, executes through a defensive boundary, and emits signed receipts anchored to a host-local ledger.

The reference implementation is a single Go codebase that compiles into the g8e binary. The same binary serves both g8e Gateway (PDP) and g8e Operator (PEP) roles, selected via subcommands. Custom Operator implementations must implement the same protocol contracts and invariants.

### Role Selection

The g8e binary uses a single cobra command tree. Gateway and Operator modes are invoked via subcommands.

#### Gateway Mode (PDP)
A Gateway enforces governance postures across all connected Operators. Start a gateway worker with `gw start` (background) or `gw start --follow` (foreground), specifying a posture via `--posture`. Use `gw start --interactive` to launch the onboarding wizard before starting:
- `--posture doctrine`, Enforces L1 hard gates; audits L2/L3.
- `--posture consensus`, Enforces L1/L2; audits L3. Requires `--consensus-id` and `--consensus-url` to connect to an enrolled Consensus service for L2 deliberation.
- `--posture notary`, Enforces L1/L2/L3 strictly.

Additional Gateway mode flags for consensus posture:
- `--consensus-id <id>`, ID of the ConsensusPolicy for L2 consensus.
- `--consensus-url <url>`, URL of the Consensus service for L2 deliberation.
- `--consensus-bootstrap <path>`, Path to a JSON file that seeds a ConsensusPolicy and trusted signers at startup.

#### Operator Mode (PEP)
An Operator executes tools on a host and connects back to a Gateway. Start an operator worker with `operator start`:
- `operator start -e, --endpoint <host>`, Connects to the specified Gateway.
- `operator start -k, --key <path>`, Specifies the Operator private key.
- `operator start --cert <path>`, Specifies the Operator certificate.
- `operator start --trust-bundle <path>`, Specifies the trust bundle PEM file for mTLS validation.
- `operator start --working-dir <path>`, Working directory for command execution.
- `operator start -c, --cloud`, Cloud operator mode.
- `operator start --provider <provider>`, Cloud provider (aws, gcp, azure).
- `operator start -s, --execution-vault`, Enable execution vault (data stays in working directory).
- `operator start -G, --no-git`, Disable Git integration.
- `operator start -l, --log <level>`, Log level: info, error, debug.
- `operator start --heartbeat-interval <seconds>`, Heartbeat interval in seconds (default: 30).
- `operator start --lattice-endpoint <url>`, Lattice gRPC endpoint URL. Enables the Lattice adapter when set.
- `operator start --lattice-client-id <id>`, OAuth2 client ID for Lattice authentication.
- `operator start --lattice-client-secret <secret>`, OAuth2 client secret for Lattice authentication.
- `operator start --lattice-sandboxes-token <token>`, Sandbox authorization token for Lattice.
- `operator start --lattice-entity-name <name>`, Entity display name registered with Lattice.
- `operator start --lattice-posture-floor <posture>`, Minimum governance posture (default: consensus).

Running the binary with no arguments launches the Tactical Governance Console (TUI). Use `gw start` or `operator start` subcommands to launch worker processes.

---

## Reference Implementation

### Prerequisites

- **Go 1.26.5+**, required for building the reference operator.
- **Make**, required to run build targets.

> **Don't have `make` or `go` installed?** Run the setup script for your platform to detect and install them automatically:
> - **Linux:** `bash scripts/linux-setup.sh`
> - **macOS:** `bash scripts/macos-setup.sh`
> - **Windows:** `pwsh scripts/windows-setup.ps1`

### Build from Source

Clone the repository and build the g8e binary:

```bash
git clone https://github.com/g8e-ai/g8e.git && cd g8e
make build
```

This produces the `g8e` binary in the repository root. All dependencies are resolved at build time; the compiled binary is statically linked (`CGO_ENABLED=0`) and has zero runtime dependencies. No Go toolchain, OpenSSL, or other external tools are needed on the target host.

**Self-Contained Deployment**: The compiled g8e binary is fully self-sovereign and requires no source tree, configuration files, or specific directory structure. It can be copied to any directory and run from there. All paths are resolved relative to the current working directory unless explicitly overridden by flags. Path configuration is embedded directly in the binary and is the sole source of truth.

### Build Targets

The Makefile provides several build targets:

- `make build`, Builds the g8e binary for the current platform.
- `make build-all`, Builds the g8e binary for all platforms (linux, windows, darwin).
- `make build-linux`, Builds the g8e binary for Linux (amd64, arm64, 386).
- `make build-windows`, Builds the g8e binary for Windows (amd64, arm64).
- `make build-darwin`, Builds the g8e binary for Darwin (amd64, arm64).
- `make build-compressed`, Builds the g8e binary for the current platform and compresses it with UPX (requires UPX installed).
- `make build-docker`, Builds the g8e binary for linux/amd64 inside a Docker container.
- `make build-linux-docker`, Builds the g8e binary for Linux (amd64, arm64, 386) inside a Docker container.
- `make build-windows-docker`, Builds the g8e binary for Windows (amd64, arm64) inside a Docker container.
- `make build-darwin-docker`, Builds the g8e binary for Darwin (amd64, arm64) inside a Docker container.
- `make build-all-docker`, Builds all platform binaries using Docker.
- `make clean`, Removes compiled binaries and test artifacts.

### Cross-Compilation

To build for different target platforms:

```bash
GOOS=linux GOARCH=amd64 make build
GOOS=darwin GOARCH=arm64 make build
GOOS=windows GOARCH=amd64 make build
```

### Windows Build

For cross-compilation from Linux/macOS to Windows:

```bash
GOOS=windows GOARCH=amd64 make build
# Output: bin/g8e-windows-amd64.exe
```

The Makefile includes a dedicated Windows build target:

```bash
make build-windows
```

This builds for both amd64 and arm64 architectures. On Windows hosts, use the same Makefile targets with a Windows-compatible make implementation (such as MinGW make or WSL).

### Deployment and Remote Management

The `operator` subcommand provides tools for managing remote Operator instances:

- `./g8e operator list`, Lists all Operators currently connected to the Gateway.
- `./g8e operator deploy --hosts <hosts>`, Deploys the binary to remote hosts via SSH and optionally starts it. Requires `./g8e auth enroll` first. Flags: `--hosts` (required), `--port` (`-P`), `--identity` (`-i`), `--background`.
- `./g8e operator stream [host...] [flags]`, Streams the binary to remote hosts via native Go crypto/ssh and executes it directly on each host. Supports concurrent streaming and advanced SSH configuration. Flags: `--arch` (target architecture: amd64, arm64, 386), `--hosts` (file of hosts, one per line, or `-` for stdin), `--concurrency` (max parallel SSH sessions, default: 50), `--timeout` (per-host dial and inject timeout in seconds, default: 60), `--endpoint` (platform endpoint; if set, starts Operator on each remote host), `--no-git` (disable ledger), `--ssh-config` (path to SSH config file), `--known-hosts` (path to SSH known_hosts file), `--binary-dir` (directory containing arch-specific Operator builds), `--ssh-identity-file` (SSH identity file path), `--ssh-user` (SSH username), `--ssh-passphrase` (passphrase for encrypted SSH private keys), `--preflight` (enable pre-flight SSH connectivity check).
- `./g8e operator cp <target>`, Copies the binary to a local path.
- `./g8e operator scp <user@host:path>`, Copies the binary to a remote host. Flags: `--port` (`-P`), `--identity` (`-i`), `--recursive` (`-r`), `--preserve`, `--verbose` (`-v`), `--compression` (`-C`), `--prompt`.

---

## Protocol Library Dependencies

Custom operator implementations need the g8e Protocol Library for protobuf schema definitions, SPIFFE workload identity helpers, and JSON protocol constants. The protocol is published as both a Go module and a Python package, both sharing the same version number as the platform binary.

### Go Module

The protocol is part of the root Go module `github.com/g8e-ai/g8e`. Add it to your project:

```bash
go get github.com/g8e-ai/g8e@v1.6.6
```

The Go module provides protobuf types for governance envelopes, operator service definitions, and SPIFFE workload identity helpers for mTLS identity binding. Import the common and operator protocol packages for envelope construction and verification, and the root protocol package for SPIFFE URI SAN generation and validation.

See the [Protocol Library documentation](../architecture/protocol.md) for the full API reference, and `protocol/examples/` for complete Go example programs covering governance envelope construction and SPIFFE workload identity generation.

### Python Package

For operator-side tooling, testing, or Python-based actuator services that need to consume protocol constants:

```bash
pip install g8e==1.6.6
```

The package provides `g8e.constants` (JSON protocol constants), `g8e.enums` (dynamic enums from protocol constants), and `g8e.models` (Pydantic v2 models). Requires Python 3.10+. See the [Protocol Library documentation](../architecture/protocol.md) for the full API reference.

---

## Custom Operator Implementation

To build a custom g8e-compatible Operator, your implementation must satisfy the following protocol contracts.

### Required Capabilities

#### 1. Protocol Translation

The Operator must act as a universal protocol translator:

- **MCP Translation**: Accept JSON-RPC MCP tool calls and wrap them in GovernanceEnvelope format.
- **A2A Translation**: Accept HTTP/JSON A2A skill invocations and wrap them in GovernanceEnvelope format.
- **Canonical JSON**: Use protojson (canonical JSON) as the wire format for all client-facing interactions.
- **Typed Payload Mapping**: Map native JSON-RPC requests directly to governed ActionType mutations defined in the protocol schemas.

#### 2. Verification Sequence (L1-L4)

The Operator must implement a singular verification gate that enforces:

- **Integrity**: Verify `id == transaction_hash == SHA256(canonical_fields)` computed from the GovernanceEnvelope.
- **Freshness**: Validate `expires_at` is not passed and `nonce` is not in the replay store.
- **State Binding**: Verify `state_merkle_root` matches the host's current local ledger root.
- **L1Doctrine (Hard Gates)**: Enforce technical bedrock threat detection rules, forbidden patterns, and MITRE ATT&CK heuristics on the typed payload.
- **L2Consensus**: Verify Consensus deliberation votes (Ed25519 signatures) against a locally trusted signer store and the active ConsensusPolicy. Under `consensus` posture, the gateway delegates L2 deliberation to an enrolled Consensus service rather than self-signing.
- **L3Notary**: Validate authorization proofs (mTLS certificate fingerprints for CLI sessions, WebAuthn proofs for web sessions).
- **L4Warden**: Pre-dispatch verification of all preceding proofs and state roots.

Any verification failure must result in a typed rejection and audit entry. No fallback paths or silent retries.

#### 3. Execution Boundary (L5Actuator)

The Operator must implement a single execution boundary permitted to mutate host state:

- **Pre-execution Receipt**: Sign an ActionReceipt with status `EXECUTING` and commit it to the encrypted audit log. Abort execution if this write fails.
- **Execution**: Dispatch the verified payload to the appropriate handler (shell, file edit, etc.).
- **Sovereign Execution Boundary**: Process output to scrub sensitive PII, credentials, and connection strings before data leaves the boundary.
- **Post-execution Receipt**: Update the receipt to `COMPLETED` or `FAILED`, capture the new `state_root_after`, sign the result, and publish it back to the Gateway.

#### 4. Identity and PKI

The Operator must establish workload identity via mTLS:

- **SPIFFE URI SANs**: Use SPIFFE-style URI SANs for identity binding.
- **Satellite Identity**: `spiffe://g8e.local/operator/<organization_id>/<operator_id>/<operator_session_id>`.
- **CLI/BYO Client**: `spiffe://g8e.local/cli/<user_id>/<cli_session_id>`.
- **Certificate Revocation**: Enforce revocation on every handshake.
- **Ed25519 Signing Key**: Possess a unique Ed25519 signing key used exclusively to sign ActionReceipts.

#### 5. Local-First Audit Architecture (LFAA)

The Operator must maintain the host as the authoritative source of truth:

- **Encrypted Audit Log**: Append-only, encrypted log of every event and signed ActionReceipt. Fail-closed: reject events missing a valid operator session ID. Encryption at rest is mandatory.
- **Git-backed Ledger**: Git-based version control for file mutations. Implements two-phase commit (hash before / hash after) and supports restoration to any prior state within the session. Encryption at rest is mandatory.
- **Execution Vault**: Encrypted storage for command execution results and file diffs. Encryption at rest is mandatory.
- **Canonical State Store**: (Gateway mode only) Unified encrypted persistence for state roots, nonces, trusted signers, app policies, and suspended transactions. Encryption at rest is mandatory.

#### 6. Outbound-Only Connectivity

The Operator must establish outbound-only connectivity to the Gateway:

- **mTLS Reverse Tunnel**: Dial out to the Gateway via mTLS WSS.
- **No Inbound Ports**: Listen on nothing. No NAT traversal or remote attack surface on the execution boundary.
- **Pub/Sub Subscription**: Subscribe to command events on the Gateway's Pub/Sub broker.

#### 7. MCP Server

The Operator must expose tools as a Model Context Protocol server:

- **HTTP-based MCP**: Support HTTP-based MCP for all client integrations (IDEs, direct API access).
- **Tool Registration**: Register available tools with the MCP client.

### Protocol Invariants

Your implementation must enforce these core invariants:

1. **Transaction Hash Verification**: The envelope `id` must match the deterministic transaction_hash computed from its content.
2. **State Binding**: Every transaction must include a state root and be verified against the current authoritative state.
3. **Replay Defense**: Nonces must be validated against a sliding window to prevent replay attacks.
4. **Expiry Enforcement**: Transactions must be rejected if they have expired.
5. **Fail-Closed Execution**: Any verification failure must result in a typed rejection and audit entry. No fallback paths or silent retries.
6. **Sovereignty**: Sensitive data must be scrubbed before leaving the execution boundary.
7. **Local-First Audit**: All audit entries must be written to the host-local encrypted audit log before execution.

### Sovereign Execution Boundary

The Operator must implement data sovereignty:

- **Threat Detection Before Execution**: Run L1Doctrine threat detection before execution.
- **Data Scrubbing During Execution**: Rehydrate safe tokens for execution at the L5Actuator and aggressively scrub outputs before publishing.
- **Token Persistence**: Persist scrubbing tokens locally across restarts to prevent data leaks during crashes.

### Canonical JSON Wire Format

While schemas are defined via Protobuf, the canonical wire format for the operator's client-facing surfaces must be strictly canonical JSON (protojson). This guarantees ecosystem compatibility without breaking determinism for the transaction_hash.

### Strict Protocol Enforcement

The Operator must drop stale JSON formats, raw HMAC structures, and outdated relay fallbacks. A transaction either fully complies with the current strict 5-layer verification protocol, or it is rejected.

---

## Protocol Schema

The GovernanceEnvelope schema is defined in the protocol protobuf files. Your implementation must:

1. **Use the canonical protojson wire format** for all client-facing interactions.
2. **Implement the typed payload validation** defined in the protocol schemas.
3. **Support the canonical request payload mappings** for all first-class event types.
4. **Handle L2 votes** from Consensus deliberation, verifying the quorum of Ed25519 signatures against the ConsensusPolicy.

Refer to the protocol schema definitions in the protocol protobuf files for the canonical schema definitions.

---

## Testing

A custom Operator implementation must pass the platform test suite to claim g8e compatibility:

- `make test-unit`, Runs Tier 1 unit tests.
- `make test-integration`, Runs Tier 2 in-process integration tests.
- `make test-docker`, Runs Tier 3 E2E tests requiring Docker.
- `make ci`, Runs the full CI pipeline.

---

## Vault Setup

The g8e Operator requires a vault for encryption at rest. The vault must be initialized and unlocked before the Operator can start.

### Initialize Vault

```bash
./g8e vault init
```

This creates a new vault in `.g8e/vault` and generates a private key in `.g8e/vault/key`.

### Unlock Vault

The vault is automatically unlocked when starting the Gateway or Operator. To manually unlock the vault:

```bash
./g8e vault unlock
```

### Vault Configuration

When starting the Gateway with `gw start` or `gw start --follow`, the vault can be configured via CLI flags or environment variables:

- `--vault-dir <dir>`: Directory for vault data (default: `.g8e/vault`)
- `--vault-key <path>`: Path to vault private key (default: `.g8e/secrets/key`)

Environment variables:
- `G8E_VAULT_DIR`: Override vault directory
- `G8E_VAULT_KEY`: Override vault key path

### Vault Management

Additional vault management commands:

- `init`, Initializes a new vault and saves the key to `.g8e/vault/key` (relative to the current working directory).
- `unlock`, Unlocks the vault using the private key.
- `rekey`, Re-encrypts the vault with a new key.
- `status`, Displays initialization and lock state.
- `reset`, Destroys all vault data (irreversible).
- `export`, Exports the private key to stdout.
- `import`, Imports a private key from hex string or stdin.

For detailed vault architecture and security guarantees, see [Encryption Architecture](../architecture/encryption.md).

---

## Next Steps

- **[Connect Operator to Gateway](connect_operator_to_gateway.md)**, Deploy and use a g8e Operator.
- **[Build Apps](build_apps.md)**, Build g8e-compatible applications using a Gateway.
- **[Protocol Library](../architecture/protocol.md)**, Go module and Python package API reference, constants, models, and usage examples.
