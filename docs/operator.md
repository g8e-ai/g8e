---
title: g8e Operator
---

# g8e Operator

Last Updated: 2026-05-18

The **g8e Operator** is the platform- and language-agnostic role defined by the [g8e Protocol](protocol.md): a device or piece of software that speaks the protocol to perform remote operations under the security guarantees the protocol enables. An Operator receives signed transactions, enforces L1/L2/L3 verification, executes through a defensive boundary, and emits signed receipts anchored to a host-local ledger.

The reference Operator is **`g8eo`**, a single statically compiled Go binary. It is one implementation of many possible implementations - anything that conforms to the protocol can replace it. This document focuses on the role itself: what an Operator does, what it defends against, and how it preserves data sovereignty.

> **Two roles, one binary.** The same `g8eo` binary runs as a **Satellite** on a managed host (this document) and as a **Hub** in `--listen` mode (the data backplane - see [g8eo Service](g8eo_service.md)). Satellite mode is the focus here.

---

## What an Operator Does

An Operator is the only component that can mutate the host. It executes ordinary remote-operations work - running shell commands, editing files, listing directories, checking ports, reading file history, fetching diffs, restoring previous file revisions, performing cloud-CLI work (AWS/GCP/Azure), and managing its own lifecycle on the host - but only after every protocol invariant has been independently verified.

Concretely, the reference Operator:

- Connects outbound-only over mTLS WSS to the Hub (no inbound ports required).
- Verifies every inbound `GovernanceEnvelope` against L1, L2, L3, integrity, freshness, and state-root gates.
- Runs the **Warden** as the sole execution boundary, signing pre- and post-execution `ActionReceipt`s.
- Records every accepted mutation to a host-local, encrypted, append-only audit vault and a per-session git-backed ledger.
- Scrubs sensitive data (PII, secrets) at the host boundary so AI never sees unscrubbed data.
- Self-deploys to remote hosts (cross-compiled binaries pulled from the Hub blob store over SSH).

---

## Defense at Ingress

The Operator distrusts every upstream input. Before any code runs on the host, the `TransactionVerifier` enforces the gates in this order:

1. **Integrity** - `id == transaction_hash == SHA256(canonical_fields)`.
2. **Freshness** - `expires_at` not passed; `nonce` not in the replay store.
3. **State binding** - `state_merkle_root` matches the host's current ledger root.
4. **L1 hard gates** - Reflected `forbidden_patterns` over the typed protobuf payload, plus Sentinel pre-execution threat analysis (90+ MITRE ATT&CK patterns).
5. **L2 consensus** - Ed25519 Tribunal signature verified against the Operator-owned `SignerStore`. Missing or unknown signers → reject.
6. **L3 authorization** - WebAuthn proof verified for every mutation. Auto-approval policy may suppress the human prompt for benign verbs only after L1 and L2 have passed.

If any gate fails, the envelope is rejected, a `BLOCKED` receipt is recorded, and no execution occurs. There are no fallbacks.

---

## Defense at Execution: The Warden

The **Warden** is the Operator's execution boundary and the only service permitted to mutate host state.

1. **Pre-execution receipt** - Warden signs an `ActionReceipt` with status `EXECUTING` and writes it to the AuditVault. If the audit write fails, execution is aborted.
2. **Dispatch** - Warden routes the typed payload to its handler (shell executor, file editor, port checker, etc.).
3. **Post-execution receipt** - Warden updates the receipt with `COMPLETED` or `FAILED`, captures `state_root_after`, signs again, and publishes the result envelope.

The dual-receipt model guarantees that every attempt to change reality is cryptographically recorded - even if the process crashes mid-execution. Reputation slashing for missed risk or over-cautious blocking is anchored to these receipts.

---

## Defense of Local Data: Sentinel and the Vault Tiers

Sensitive data is scrubbed before it ever leaves the host. **AI never sees unscrubbed data.**

### Sentinel

Sentinel runs in two phases on the Operator:

- **Pre-execution defense** - Regex analysis against MITRE ATT&CK threat patterns blocks dangerous commands before dispatch.
- **Post-execution scrubbing** - Removes API keys, tokens, PII, connection strings, and credentials from output before it is stored in the Scrubbed Vault or transmitted off-host. Sentinel placeholders include `[AWS_KEY]`, `[AWS_SECRET]`, `[PII]`, `[URL_WITH_CREDENTIALS]`, `[CONN_STRING]`, `[CREDENTIAL_REFERENCE]`. Patterns live in `@/home/bob/g8e/services/g8eo/internal/services/sentinel/`.

### Three local vaults (LFAA)

Local-First Audit Architecture stores the host as the source of truth:

| Vault | Path | Purpose | Encryption |
|---|---|---|---|
| **Audit Vault** | `.g8e/data/g8e.db` | Append-only encrypted log of every event and signed `ActionReceipt`. | AES-256-GCM (DEK from local Encryption Vault) |
| **Scrubbed Vault** | `.g8e/local_state.db` | Sentinel-scrubbed execution and file-diff logs - the **only data AI ever reads**. | At rest |
| **Raw Vault** | `.g8e/raw_vault.db` | Unscrubbed forensic record. **Never readable by AI.** Customer-only access. | At rest |

The Audit Vault is fail-closed against session identity: events with missing or unknown `operator_session_id` are rejected outright. Sessions must be created explicitly by the auth lifecycle before audit writes are accepted.

### The git-backed ledger

A multi-ledger architecture under `.g8e/data/ledger/sessions/<operator_session_id>/`. Every file mutation is a two-phase commit: `LedgerHashBefore` and `LedgerHashAfter` capture pre- and post-state. Files are mirrored as encrypted `.enc` blobs when the vault is unlocked. Any file can be restored to any prior state within its session ledger. With `--no-git` the ledger is disabled; the audit vault continues operating.

---

## Identity, PKI, and mTLS

The Operator runs entirely under mTLS with workload identity bound to SPIFFE-style URI SANs. CLI and operator agent are cryptographically distinct principals issued separate certificates.

| Role | URI SAN pattern |
|---|---|
| Operator (Satellite) | `spiffe://g8e.local/operator/<organization_id>/<operator_id>/<operator_session_id>` |
| CLI (BYO client) | `spiffe://g8e.local/cli/<user_id>/<cli_session_id>` |
| Application (agent) | `spiffe://g8e.local/app/<operator_id>` |
| Hub (Listen mode) | `spiffe://g8e.local/hub/operator-listen` |

### Enrollment lifecycle

1. **Trust verification** - Client fetches the Hub's root CA fingerprint from `GET /.well-known/pki/fingerprint`.
2. **Registration** - Client presents a one-time device-link token plus a hardware-derived `system_fingerprint` to the bootstrap port.
3. **CSR submission** - Client generates two private keys (Operator, CLI) and submits two CSRs.
4. **Issuance** - Hub signs both with the Operator Intermediate CA (role-specific URI SANs) and returns the certificate chains.
5. **Steady state** - All control-plane traffic is mTLS over TLS 1.3. Older TLS versions and insecure ciphers are explicitly rejected.

Revocation is enforced on every handshake against the `revoked_certificates` collection, with signed revocation bundles available for external verification.

The Warden's Ed25519 signing key lives in `.g8e/secrets/warden_signing_key`; its public key is exported on every Hub startup to `.g8e/pki/warden_pub.pem` and `.g8e/pki/warden_pub.json` for offline receipt verification (used by evals and external auditors).

---

## Lifecycle (Satellite Mode)

1. **Discovery** - Resolve environment and trust material from `.g8e/pki` or fetch from the Hub.
2. **Fingerprinting** - Generate a stable `system_fingerprint` (OS/arch, CPU, machine ID, hostname).
3. **Enrollment** - Authenticate via `POST /api/auth/operator` with a Device Token; receive operator and CLI certs.
4. **mTLS upgrade** - Switch all transport to TLS 1.3 + mTLS WSS.
5. **Vault unlock** - API key unlocks the local Encryption Vault to retrieve the DEK.
6. **Steady state** - Subscribe to `cmd:{operator_id}:{operator_session_id}`; await governed envelopes.
7. **Verify and execute** - Run the verification gates; on success, the Warden executes; in all cases a signed receipt is published.

### Operating modes

| Mode | Purpose |
|---|---|
| **Standard (default)** | Satellite execution on a managed host. |
| **Listen** (`--listen`) | Hub mode. See [g8eo Service](g8eo_service.md). |
| **Stream** | Concurrent SSH-based fleet deployment (the binary streams itself into memory on remote hosts). |
| **OpenClaw** | Runs as a standalone capability provider behind an OpenClaw Gateway for external orchestrators. |

---

## CLI Reference

| Flag | Description |
|---|---|
| `-k`, `--key` | API key for auth and Vault unlock. |
| `-D`, `--device-token` | Device-link token for automated registration and CSR signing. |
| `-e`, `--endpoint` | Hub endpoint address. |
| `--listen` | Start in Hub mode. |
| `--wss-listen-port` | Pub/Sub port (default 9001). |
| `--http-listen-port` | mTLS API port (default 9000). |
| `--bootstrap-listen-port` | Device-link enrollment port (default 9002). |
| `--public-listen-port` | Browser/BYO public port (default 9003). |
| `--data-dir` | Persistence directory (default `.g8e/data`). |
| `--pki-dir` | PKI hierarchy directory (default `.g8e/pki`). |
| `--secrets-dir` | Platform secrets directory (default `.g8e/secrets`). |
| `-s`, `--local-storage` | Enable LFAA auditing (default on). |
| `-G`, `--no-git` | Disable the git-backed file ledger. |
| `--working-dir` | Anchor for commands and storage (default: launch dir). |
| `--log` | Log level: `info`, `error`, `debug`. |

### Exit codes

Defined in `@/home/bob/g8e/services/g8eo/internal/constants/exit_codes.go`. Stable codes let supervisors react precisely:

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | General error |
| 2 | Auth failure |
| 3 | Permission denied |
| 4 | Network error |
| 5 | Config error |
| 6 | Storage error |
| 7 | TLS / cert trust failure |
| 10 | Vault error |

For `./g8e` lifecycle commands (`platform start`, `apps`, `operator deploy`, `operator stream`, `data`, `security`, `demo`, `evals`, `test`) see [Scripts](scripts.md).

---

## Air-Gap Operation

The Operator is fully self-contained. There are no runtime internet dependencies; no telemetry leaves the host. All trust material, persistence, and secrets are local. Local LLM inference is supported through `g8ee`'s `LlamaCppProvider` for fully offline reasoning. PKI, CA hierarchy, and trust bundles are generated and rotated locally; the Hub forbids outbound dialing in `--listen` mode.

---

## Implementation Reference

| Concern | Authoritative file |
|---|---|
| Verification gates | `@/home/bob/g8e/services/g8eo/internal/services/governance/transaction_verifier.go` |
| Warden / receipts | `@/home/bob/g8e/services/g8eo/internal/services/governance/warden.go` |
| Sentinel | `@/home/bob/g8e/services/g8eo/internal/services/sentinel/sentinel.go` |
| Audit vault | `@/home/bob/g8e/services/g8eo/internal/services/storage/audit_vault.go` |
| Ledger (git) | `@/home/bob/g8e/services/g8eo/internal/services/storage/ledger.go` |
| Listen mode entry | `@/home/bob/g8e/services/g8eo/cmd/g8eo/main.go` |
| PKI / CertStore | `@/home/bob/g8e/services/g8eo/internal/services/listen/listen_certs.go` |

See also: [Protocol](protocol.md), [g8eo Service (Hub mode)](g8eo_service.md), [g8ee Service](g8ee.md).
