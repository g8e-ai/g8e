---
title: Build Operator
parent: Guides
---

# Build a g8e Operator

Last Updated: 2026-06-01
Version: v1.0.5

---

## Overview

A g8e-compatible g8e Operator implements the host-side Policy Execution Point (PEP) of the platform. It receives transactions, enforces the 5-layer verification sequence, executes through a defensive boundary, and emits signed receipts anchored to a host-local ledger.

The reference implementation is a single Go codebase that compiles into the g8e Node. The same g8e Node serves both g8e Gateway (PDP) and g8e Operator (PEP) roles, selected via command-line flags. Custom Operator implementations must implement the same protocol contracts and invariants.

---

## Reference Implementation

### Prerequisites

- **Go 1.26+** — Required for building the reference operator.

### Build from Source

Clone the repository and build the g8e Node:

```bash
git clone https://github.com/g8e-ai/g8e.git && cd g8e
make build
```

This produces the `g8e` g8e Node in the repository root. The g8e Node is statically linked and requires no runtime dependencies.

**Self-Contained Deployment**: The compiled g8e Node is fully self-sovereign and requires no source tree, configuration files, or specific directory structure. It can be copied to any directory and run from there. All paths are resolved relative to the current working directory unless explicitly overridden by flags. Path configuration is embedded directly in the g8e Node via go:embed and is the sole source of truth.

### Build Targets

The Makefile provides several build targets:

- `make build` — Builds the g8e Node for all platforms (linux, windows, darwin).
- `make build-linux` — Builds the g8e Node for Linux (amd64, arm64, 386).
- `make build-windows` — Builds the g8e Node for Windows (amd64, arm64).
- `make build-darwin` — Builds the g8e Node for Darwin (amd64, arm64).
- `make build-compressed` — Builds the g8e Node for all platforms with UPX compression.
- `make build-linux-compressed` — Builds the g8e Node for Linux with UPX compression.
- `make build-windows-compressed` — Builds the g8e Node for Windows with UPX compression.
- `make build-darwin-compressed` — Builds the g8e Node for Darwin with UPX compression.
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

- **Pre-execution Receipt**: Sign an ActionReceipt with status `EXECUTING` and commit it to the AuditVaultService. Abort execution if this write fails.
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

- **AuditVaultService**: Append-only, encrypted SQLite log of every event and signed ActionReceipt. Fail-closed: reject events missing a valid operator_session_id. Supports optional encryption vault for data-at-rest protection.
- **LedgerService**: Git-backed version control for file mutations. Implements two-phase commit (LedgerHashBefore / LedgerHashAfter) and supports restoration to any prior state within the session.
- **LocalStoreService**: SQLite storage for command execution results, file diffs, and suspended transactions. Provides token persistence for sovereignty scrubbing.
- **CanonicalDBService**: (Gateway mode only) Unified SQLite persistence for state roots, nonces, trusted signers, app policies, and suspended transactions.

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
7. **Local-First Audit**: All audit entries must be written to the host-local AuditVaultService before execution.

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
./g8e test unit
```

This runs unit tests covering:
- Pub/Sub command dispatch
- AuditVaultService writes
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
./g8e test integration
```

For the full CI pipeline:

```bash
./g8e test ci
```

---

## Next Steps

- **[Connect Operator to Gateway](connect_operator_to_gateway.md)** — Deploy and use a g8e Operator.
- **[Build Apps](build_apps.md)** — Build g8e-compatible applications using a Gateway.
