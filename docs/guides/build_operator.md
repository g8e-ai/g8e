---
title: Build Operator
parent: Guides
---

# Build a g8e Operator

Last Updated: 2026-06-10
Version: v1.0.11

---

## Overview

A g8e-compatible g8e Operator implements the host-side Policy Execution Point (PEP) of the platform. It receives transactions, enforces the 5-layer verification sequence, executes through a defensive boundary, and emits signed receipts anchored to a host-local ledger.

The reference implementation is a single Go codebase that compiles into the g8e binary. The same binary serves both g8e Gateway (PDP) and g8e Operator (PEP) roles, selected via command-line flags. Custom Operator implementations must implement the same protocol contracts and invariants.

---

## Reference Implementation

### Prerequisites

- **Go 1.26.4+** — Required for building the reference operator.

### Build from Source

Clone the repository and build the g8e binary:

```bash
git clone https://github.com/g8e-ai/g8e.git && cd g8e
make build
```

This produces the `g8e` binary in the repository root. The binary is statically linked and requires no runtime dependencies.

**Self-Contained Deployment**: The compiled g8e binary is fully self-sovereign and requires no source tree, configuration files, or specific directory structure. It can be copied to any directory and run from there. All paths are resolved relative to the current working directory unless explicitly overridden by flags. Path configuration is embedded directly in the binary via go:embed and is the sole source of truth.

### Build Targets

The Makefile provides several build targets:

- `make build` — Builds the g8e binary for the current platform.
- `make build-all` — Builds the g8e binary for all platforms (linux, windows, darwin).
- `make build-linux` — Builds the g8e binary for Linux (amd64, arm64, 386).
- `make build-windows` — Builds the g8e binary for Windows (amd64, arm64).
- `make build-darwin` — Builds the g8e binary for Darwin (amd64, arm64).
- `make clean` — Removes compiled binaries and test artifacts.

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

---

## Custom Operator Implementation

To build a custom g8e-compatible g8e Operator, your implementation must satisfy the following protocol contracts.

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
- **L2Consensus**: Verify 5-agent intent consensus signatures against a locally trusted SignerStore. In doctrine mode, the Gateway may sign locally with `gateway_signed=true` for single-agent MCP clients.
- **L3Notary**: Validate authorization proofs (mTLS certificate fingerprints for CLI sessions, WebAuthn proofs for web sessions).
- **L4Warden**: Pre-dispatch verification of all preceding proofs and state roots.

Any verification failure must result in a typed rejection and audit entry. No fallback paths or silent retries.

#### 3. Execution Boundary (L5Actuator)

The Operator must implement a single execution boundary permitted to mutate host state:

- **Pre-execution Receipt**: Sign an ActionReceipt with status `EXECUTING` and commit it to the SQLAuditStore. Abort execution if this write fails.
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

- **SQLAuditStore**: Append-only, encrypted SQLite log of every event and signed ActionReceipt. Fail-closed: reject events missing a valid operator_session_id. Requires encryption vault for data-at-rest protection (encryption is mandatory).
- **LedgerService**: Git-backed version control for file mutations. Implements two-phase commit (LedgerHashBefore / LedgerHashAfter) and supports restoration to any prior state within the session.
- **LocalStoreService**: SQLite storage for command execution results, file diffs, and suspended transactions. Provides token persistence for sovereignty scrubbing. Requires encryption vault (encryption is mandatory).
- **CanonicalDBService**: (Gateway mode only) Unified SQLite persistence for state roots, nonces, trusted signers, app policies, and suspended transactions. Requires encryption vault (encryption is mandatory).

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
7. **Local-First Audit**: All audit entries must be written to the host-local SQLAuditStore before execution.

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
4. **Handle the gateway_signed field** to distinguish between full L2 consensus and Gateway-signed transactions (single-agent MCP clients).

Refer to `protocol/proto/g8e/` for the canonical schema definitions.

---

## Testing

A custom Operator implementation must pass the platform test suite to claim g8e compatibility:

```bash
make test-unit
```

This runs unit tests covering:
- Pub/Sub command dispatch
- SQLAuditStore writes
- LedgerService commits
- L1/L2/L3 verification gates
- GovernanceEnvelope validation
- State root computation
- Nonce management
- PKI operations
- MCP/A2A translation
- Sensitive data scrubbing

For integration tests:

```bash
make test-integration
```

For the full CI pipeline:

```bash
make ci
```

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

The vault can be configured via CLI flags or environment variables:

- `--vault-dir <dir>`: Directory for vault data (default: `.g8e/vault`)
- `--vault-key <path>`: Path to vault private key (default: `.g8e/vault/key`)
- `--vault-require-unlock`: Require vault to be unlocked at startup (fail if vault cannot be unlocked)

Environment variables:
- `G8E_VAULT_DIR`: Override vault directory
- `G8E_VAULT_KEY`: Override vault key path
- `G8E_VAULT_REQUIRE_UNLOCK`: Set to `true` to require vault unlock at startup

### Vault Management

Additional vault management commands:

- `./g8e vault status`: Check vault status
- `./g8e vault rekey`: Re-encrypt vault with new private key
- `./g8e vault reset`: Reset vault (DESTROYS ALL DATA)
- `./g8e vault export`: Export vault private key
- `./g8e vault import`: Import vault private key
- `./g8e vault key generate`: Generate new vault key
- `./g8e vault key import`: Import existing vault key
- `./g8e vault key export`: Export vault key

For detailed vault architecture and security guarantees, see [Encryption Architecture](../architecture/encryption.md).

---

## Next Steps

- **[Connect Operator to Gateway](connect_operator_to_gateway.md)** — Deploy and use a g8e Operator.
- **[Build Apps](build_apps.md)** — Build g8e-compatible applications using a Gateway.
