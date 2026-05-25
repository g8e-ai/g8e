---
title: g8e Operator
---

# g8e Operator

Last Updated: 2026-05-25

The **g8e Operator** is the host-side, sovereign agent role defined by the g8e Protocol: a daemon that functions as the remote execution target and universal protocol translator under the security guarantees of the platform. An Operator receives transactions, enforces L1/L2/L3 verification, executes through a defensive boundary, and emits signed receipts anchored to a host-local ledger.

The reference Operator is **`g8eo`** (built as the `g8e` binary and executed in satellite or MCP mode). It functions as a sovereign, **Governed Operator** and **Model Context Protocol (MCP) Server**, serving as the Policy Execution Point (PEP).

> **One Codebase, Two Roles.** The exact same compiled Go codebase is used to power both sides of the governance boundary. When run as `g8eg` (Gateway mode), it acts as the central Policy Decision Point (PDP). This document focuses entirely on `g8eo` - the Governed Operator running on the target host.

---

## 1. Introduction

The core invariant of the Operator is absolute defense-in-depth: a typed, signed, state-bound transaction reaches a sovereign host agent that distrusts upstream inputs and refuses to mutate reality unless every independent proof checks out. 

The Operator is the only component capable of mutating the host. It executes remote-operations work—running shell commands, editing files, interacting with cloud CLIs, and reading file history—but only after translating the request into a canonical governance transaction and verifying it locally.

---

## 2. The Lifecycle Pipeline

When a command targets an Operator, it progresses through a strict, fail-closed pipeline:

### A. Translation & Interception (MCP/A2A Gateway)
Standard AI clients (Cursor, Claude, or custom agents) speak JSON-RPC/HTTP tool calls. The Operator acts as an MCP/A2A universal protocol translator. It intercepts these standard JSON tool calls and forces them into a canonical JSON (protojson) `GovernanceEnvelope`. This ensures that generic ecosystems can interact with the Operator without sacrificing the strict typed-payload governance required by the platform.

### B. Ingress Defense (`L4Warden`)
The Operator distrusts all inputs. Before any execution happens, the `L4Warden` serves as the singular verification gate, enforcing:
1. **Integrity**: `id == transaction_hash == SHA256(canonical_fields)`. The wire format is canonical JSON, but the signature basis is always the deterministic transaction hash.
2. **Freshness**: The `expires_at` timestamp is not passed, and the `nonce` is not in the replay store.
3. **State Binding**: The `state_merkle_root` strictly matches the host's current local ledger root.
4. **L1Doctrine (Hard Gates)**: Technical Bedrock threat detection rules out forbidden patterns and executes MITRE ATT&CK heuristics on the typed payload.
5. **L2Consensus**: The 5-agent intent consensus signatures are verified against the Operator's locally trusted `SignerStore`.
6. **L3Notary**: Authorization proofs (mTLS certificate fingerprints for CLI sessions, or WebAuthn proofs for web sessions) are validated.

If any check fails, the transaction is rejected, a `BLOCKED` receipt is generated, and execution halts.

### C. Execution Boundary (`L5Actuator`)
The `L5Actuator` is the single execution boundary permitted to mutate host state. It uses a dual-receipt model to cryptographically record intent:
1. **Pre-execution receipt**: Signs an `ActionReceipt` with status `EXECUTING` and commits it to the local Audit Vault. If this write fails, execution aborts.
2. **Execution**: Dispatches the verified payload to the appropriate handler (e.g., shell, file edit).
3. **Sovereignty Boundary**: The `SovereigntyService` processes the output to scrub sensitive PII, credentials, and connection strings before the data leaves the boundary.
4. **Post-execution receipt**: Updates the receipt to `COMPLETED` or `FAILED`, captures the new `state_root_after`, signs the result, and publishes it back to the Hub.

---

## 3. Core Subsystems

### Universal Protocol Translator
By exposing standard MCP and A2A interfaces (`--mcp-serve`), the Operator acts as the admission gate for BYO (Bring-Your-Own) AI clients. It isolates the complex requirements of the `GovernanceEnvelope` (such as transaction hashing and L2/L3 signature collection) behind a standardized tool-calling facade, mapping native JSON-RPC requests directly to governed `ActionType` mutations.

### Identity, PKI, and mTLS
The Operator runs entirely over outbound mutual TLS (mTLS) over WSS. It establishes workload identity bound to SPIFFE-style URI SANs:
- **Satellite Identity**: `spiffe://g8e.local/operator/<organization_id>/<operator_id>/<operator_session_id>`
- **CLI/BYO Client**: `spiffe://g8e.local/cli/<user_id>/<cli_session_id>`

Revocation is strictly enforced on every handshake. The L5Actuator possesses a unique Ed25519 signing key used exclusively to sign `ActionReceipts`, ensuring that evals, external auditors, and the Hub can cryptographically verify that the host itself completed the mutation.

### Defense of Local Data (LFAA)
The Local-First Audit Architecture (LFAA) guarantees that the host remains the authoritative source of truth.

- **Audit Vault**: An append-only, encrypted SQLite log of every event and signed `ActionReceipt`. It is strictly fail-closed: events missing a valid `operator_session_id` are rejected outright.
- **Scrubbed Vault**: Contains only sovereignty-scrubbed execution logs. **This is the only data AI ever reads.**
- **Raw Vault**: Retains the unscrubbed forensic record. **Never readable by AI**; reserved strictly for customer security audits.
- **Git-Backed Ledger**: Implements a two-phase commit (`LedgerHashBefore` / `LedgerHashAfter`) for file mutations using a native `go-git` implementation (avoiding slow, brittle shell process forking). Files are mirrored as encrypted blobs and can be restored to any prior state within the session.

---

## 4. Governance & Safety

- **Sovereignty Boundary Plane**: Threat detection runs *before* execution (`L1Doctrine`), but data sovereignty runs *during* execution. The Sovereignty Boundary Plane rehydrates safe tokens for execution at the `L5Actuator` and aggressively scrubs the resulting outputs before publishing. Scrubbing tokens are persisted locally across restarts to prevent data leaks during crashes.
- **Strict Canonical JSON**: While schemas are defined via Protobuf, the canonical wire format for the Operator's client-facing surfaces is strictly canonical JSON (`protojson`). This guarantees ecosystem compatibility without breaking determinism for the `transaction_hash`.
- **No Backward Compatibility**: The Operator drops stale JSON formats, raw HMAC structures, and legacy relay fallbacks. A transaction either fully complies with the current strict 3-Layer governance protocol, or it is rejected. 

---

## 5. Implementation Reference

| Concern | Authoritative file |
|---|---|
| Ingress Verification (`L4Warden`) | `internal/services/governance/l4_warden.go` |
| Execution Boundary (`L5Actuator`) | `internal/services/governance/l5_actuator.go` |
| Sovereignty (Data Scrubbing) | `internal/services/sovereignty/boundary.go` |
| Threat Detection (`L1Doctrine`) | `internal/services/governance/l1_doctrine.go` |
| Local Audit Vault | `internal/services/storage/audit_vault.go` |
| Native Git Ledger | `internal/services/storage/ledger.go` |
| MCP Proxy Entrypoint | `cmd/g8eo/main.go` |

See also: [g8e Protocol](./protocol.md), [Governance Gateway (g8eg)](./gateway.md).
