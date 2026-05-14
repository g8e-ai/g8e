---
title: g8eo
parent: Components
---

# g8eo — g8e Operator

Last Updated: 2026-05-13
Version: v0.2.4

g8eo is the Go reference implementation of the **Operator** role defined by the g8e Protocol. A single binary acts as either a sovereign **Satellite** on a managed host (verifying signed transactions and executing them through the Warden) or as a **Hub** (`--listen`) that provides persistence, PKI, and pub/sub for a fleet. Any conforming Operator implementation can replace it; the protocol contract is the same.

> For deep-reference security documentation — CA trust bootstrap, mTLS, fingerprint binding, replay protection, Sentinel pre-execution threat detection, output scrubbing, LFAA vault encryption, and the Ledger — see [architecture/security.md](../architecture/security.md). For the wire contract, see [architecture/protocol.md](../architecture/protocol.md).

---

## Core Principles

- **Host sovereignty**: The Operator is the system of record. Every accepted mutation and its raw output are anchored to a local, append-only ledger (LFAA) before any metadata leaves the host.
- **Protocol-governed execution**: Every mutation arrives as a signed `GovernanceEnvelope` (canonical JSON / protojson) carrying a typed `operator.proto` payload and L1/L2/L3 proofs. Malformed, unsigned, or stale envelopes are rejected, never coerced.
- **Warden as the only execution boundary**: No code path mutates the host except through the Warden after L1/L2/L3 + state-root verification has passed. Every execution emits a signed `ActionReceipt`.
- **Outbound-only Satellites**: A Satellite never opens inbound ports. It dials its Hub over mTLS WebSocket; the Hub never reaches back into the managed host.

---

## Operating Modes

A single `g8e.operator` binary runs in one of the following modes. Hub and Satellite are the two protocol-level roles; `stream` and `--openclaw` are auxiliary deployment helpers built on the same binary.

### Satellite (default)
The execution role. Runs on a managed host, dials a Hub over outbound-only mTLS WebSocket, receives signed `GovernanceEnvelope` transactions, verifies them locally, executes through the Warden, and anchors results to LFAA. No inbound ports are opened.

### Hub (`--listen`)
The substrate role. Provides the platform's central persistence (Document Store, KV Store, Blob Store), pub/sub broker, PKI authority, and L3 / passkey brokerage for Satellites and BYO clients. A Hub does **not** execute host mutations; it is the rendezvous and verification surface, not an executor.

### Stream (`stream` subcommand)
Agentless fleet operations. A concurrent SSH engine that ships the binary onto remote hosts for ephemeral operations where a persistent Satellite installation is not desired.

### OpenClaw Node Host (`--openclaw`)
Adapter mode. Connects to an OpenClaw Gateway as a node host, exposing the same Warden execution boundary to a third-party orchestrator. L1/L2/L3 verification semantics are preserved.

---

## Lifecycle & Pipeline

### Satellite Startup Sequence
Satellite initialization is fail-closed: nothing executes until trust and identity are established.

1. **Phase 1 — Trust bootstrap (pre-auth)**
   - **Hub bundle**: Fetches the Hub's trust bundle from `https://<hub>/.well-known/g8e/pki/hub-bundle.pem` (or loads a pinned path via `--trust-bundle`).
   - **Enrollment**: Presents a device-link token to the Hub and submits a CSR. The Hub returns a per-operator mTLS certificate with a SPIFFE URI SAN of the form `spiffe://g8e.local/operator/<organization_id>/<operator_id>/<operator_session_id>`.
   - **Identity binding**: Pins the hardware fingerprint and Operator ID to the issued certificate.

2. **Phase 2 — Service initialization (post-auth)**
   - **Vaults**: Opens the Scrubbed, Raw, and Audit vaults and initializes the global Ledger root under `.g8e/data/ledger/`. Session-scoped git repositories under `sessions/<operator_session_id>/` are created lazily on first file mutation.
   - **Pub/sub**: Establishes the persistent mTLS WebSocket to the Hub's `/ws/pubsub` endpoint.
   - **Sentinel**: Activates pre-execution threat detection and post-execution output scrubbing (MITRE ATT&CK-mapped detectors).
   - **Warden**: Activates the on-host execution boundary; only the Warden is permitted to mutate the host.

### Transaction Verification Pipeline
Every inbound `GovernanceEnvelope` passes through a single fail-closed gate before the Warden sees it. Any failure terminates the transaction with an audit entry; nothing is coerced or auto-corrected.

1. **Envelope integrity** — Canonical JSON (protojson) is decoded into the typed `UniversalEnvelope`. The envelope `id` must equal the recomputed deterministic transaction hash; otherwise the signature basis is invalid and the envelope is rejected.
2. **L1 Technical Bedrock** — Reflected Protobuf options on the typed payload (e.g., `forbidden_patterns` on `operator.proto` fields) are enforced as hard gates. Patterns such as `sudo`, `su`, and `rm -rf /` are rejected at the boundary.
3. **L2 Consensus** — The envelope must carry a valid Ed25519 signature whose `key_id` resolves to a trusted signer under `.g8e/pki/trusted_signers/`. Missing key, unknown key, or invalid signature is fatal.
4. **L3 Authorization** — A hardware-bound human signature (FIDO2 / WebAuthn) is required for state-changing mutations. Auto-approval is an authorization *state* recorded in the envelope; it never bypasses L1 or L2.
5. **State freshness** — The mandatory `state_merkle_root` is compared to the Operator-local state root. Stale or replayed transactions are rejected.

Only after all five checks pass does the Warden execute. Every execution — success or failure — emits a signed `ActionReceipt` committed to the Audit Vault and Ledger.

---

## Storage Architecture

### Local-First Audit Architecture (LFAA)
g8eo maintains four independent local stores in the `.g8e/` directory:

| Store | Purpose | Access |
|---|---|---|
| **Scrubbed Vault** | Sentinel-scrubbed command output and file diffs. | AI Engine (g8ee) |
| **Raw Vault** | Unscrubbed full output for forensics. | Local User Only |
| **Audit Vault** | Append-only event timeline (SQLite). | Platform / Audit |
| **Ledger** | Multi-Ledger: per-session isolated git repos at `.g8e/data/ledger/sessions/<session_id>/` for cryptographic file history, diff, and rollback. | Platform / Audit |

### Hub API Surface
In `--listen` mode, g8eo exposes a single public protocol surface — there is no private back channel. Every caller (Satellite, BYO client, optional reference app) authenticates with the same mTLS contract and SPIFFE URI identity.

| Surface | Port (default) | Auth | Purpose |
|---|---|---|---|
| Bootstrap / Trust Portal | `8080` (TLS) | None | `/.well-known/g8e/pki/hub-bundle.pem`, `/ca.crt`, `/trust`, device-link enrollment, CSR signing. |
| Public browser / BYO | `8081` (TLS) | Web session (passkey) | Login challenge/verify, web-session API, PKI discovery. |
| mTLS API | `9000` | mTLS + SPIFFE URI SAN | `/db/*` (Document Store), `/kv/*` (KV with TTL), `/blob/*`, `/pubsub/publish`, `/api/operators/*`, `/api/device-links/*`, `/api/pki/{sign-csr,revoke,revocation-bundle}`, `/api/auth/passkey/*`. |
| Pub/Sub | `9001` (mTLS WSS) | mTLS + SPIFFE URI SAN | `/ws/pubsub` real-time fan-out to Satellites and clients. |

Client identities follow a fixed SPIFFE scheme: operators are `spiffe://g8e.local/operator/<organization_id>/<operator_id>/<operator_session_id>`, applications are `spiffe://g8e.local/app/<app_id>`. Revocation is checked on every request against the PKI authority.

---

## Canonical Truths

The wire contract lives in `shared/proto/`; the shared JSON registries in `shared/constants/` remain the source for event names, status values, and channel prefixes. g8eo mirrors them as compile-time Go constants so drift fails at build time, not at runtime.

- **Protocol**: Generated Go artifacts under `internal/shared/proto/` mirror `shared/proto/common.proto`, `shared/proto/operator.proto`, and `shared/proto/pubsub.proto`.
- **Wire format**: Canonical JSON (protojson) on all client-facing surfaces (HTTP, pub/sub, receipts, audit exports). Protobuf bytes are an internal storage detail only.
- **Signing basis**: A deterministic `transaction_hash` is computed from normalized envelope fields; signatures are over the hash, so wire encoding is irrelevant to the security invariant.
- **Events / Status / Channels**: `internal/constants/events.go`, `status.go`, and `channels.go` mirror their JSON counterparts under `shared/constants/`.

---

## Platform Authentication

All platform participants — Satellites, optional reference apps, and BYO clients — authenticate via the same public protocol surface. There is no privileged internal channel.

- **Operator sessions**: mTLS with URI SAN `spiffe://g8e.local/operator/<organization_id>/<operator_id>/<operator_session_id>`.
- **Applications**: mTLS with URI SAN `spiffe://g8e.local/app/<app_id>`. The optional reference Engine (`g8ee`) uses this same contract.
- **Human users**: FIDO2 / WebAuthn passkeys brokered through the Hub's `/api/auth/passkey/*` endpoints. Passwords are unsupported by design.
- **Enrollment**: New operators enroll via CSR submission on the bootstrap port (`8080`) using a one-time device-link token. The Hub signs the CSR and returns a SPIFFE-bound certificate; no long-lived API key is required after enrollment.
- **Revocation**: Every mTLS request is checked against the PKI authority's revocation list. Revoked certificates fail closed.

---

## Operational Reference

### CLI Reference
g8eo provides a comprehensive set of flags for runtime configuration. Use the `--help` flag to see all available options:

```bash
g8e.operator --help
g8e.operator stream --help
```

### Exit Codes
On a fatal condition g8eo self-terminates with a stable exit code so launcher scripts and supervisors can act precisely. Codes are defined in `internal/constants/exit_codes.go`.

| Code | Meaning | Action |
|---|---|---|
| **0** | Success | — |
| **1** | General error | Inspect logs under `.g8e/logs/` |
| **2** | Auth failure | Verify device-link token or API key; re-enroll if needed |
| **3** | Permission denied | Check filesystem permissions on `.g8e/` |
| **4** | Network error | Check Hub reachability and DNS |
| **5** | Config error | Validate CLI flags / environment |
| **6** | Storage error | Inspect SQLite vaults and git ledger init |
| **7** | TLS / cert trust failure | Refresh the Hub trust bundle; re-enroll if pinning failed |
