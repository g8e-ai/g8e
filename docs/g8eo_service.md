---
title: g8eo Service
---

# g8eo Service

Last Updated: 2026-05-18

The **g8eo Service** is our reference implementation of a [g8e Operator](operator.md) being used as the platform's data backplane. The same `g8eo` binary that runs as a Satellite on managed hosts also runs in **Listen Mode** (`--listen`) as the central Hub: protocol gateway, document/KV/blob persistence, pub/sub broker, root Certificate Authority, secrets vault, and audit authority.

This document covers the data-backplane role. For the host-side execution role see [Operator](operator.md). For the wire contract see [Protocol](protocol.md).

---

## Capabilities

A single Hub process exposes:

- **Substrate API** — `POST /api/governance/envelope` is the only customer-facing mutation entry point. Direct `/db/` writes are restricted to bootstrap/Operator-owned collections; mutations to governed collections return `409 Conflict` with `{"error":"submit via POST /api/governance/envelope"}`.
- **Document Store** — JSON document CRUD on a Collection/ID pattern with `json_extract` query support.
- **KV Store** — TTL-aware ephemeral state with `GLOB` pattern scanning and cursor-based `KVScan`. Supports a Write-Only cache policy so application adapters can populate the cache for ecosystem consumers while still reading from the authoritative DB.
- **Blob Store** — Binary persistence for attachments, large objects, and certificate material.
- **Pub/Sub Broker** — High-performance WebSocket fan-out for real-time events. Mutation channels (`cmd:*`) are governed; non-mutation fan-out (`heartbeat:*`, `results:*`, `sse:*`, `internal:*`) flows through `/pubsub/publish`.
- **SSE Buffer** — Per-session ring buffer for Server-Sent Events reconnection replay.
- **State Root Provider** — Deterministic Merkle state root across all authoritative Hub data, used to bind transactions to host state.
- **Nonce Manager** — Sliding-window replay protection for governance transactions.
- **Root CA / PKI** — Issues mTLS certificates via CSR-based enrollment with SPIFFE URI SAN workload identity.
- **Secrets Vault** — Tamper-evident bootstrap secrets with a `bootstrap_digest.json` manifest.
- **Audit Authority** — Append-only encrypted log of every event and signed `ActionReceipt`, fail-closed against session identity.

---

## The Four-Port Contract

Listen Mode exposes four distinct surfaces. Each has its own authentication model.

| Surface | Port | Auth | Purpose |
|---|---|---|---|
| **mTLS API** | 9000 | mTLS + URI SAN | `/api/governance/envelope`, `/db/*` (reads + bootstrap writes), `/kv/*`, `/blob/*`, `/pubsub/publish`, `/api/operators/*`, `/api/device-links/*`, `/api/pki/{sign-csr,revoke,revocation-bundle}`, `/api/auth/passkey/*`. |
| **Pub/Sub** | 9001 | mTLS + URI SAN | `/ws/pubsub` real-time fan-out. |
| **Bootstrap** | 9002 | None | `/.well-known/g8e/pki/hub-bundle.pem`, `/ca.crt`, `/trust`, device-link enrollment, CSR signing. |
| **Public** | 9003 | Web session (passkey) | Login challenge/verify, web-session API, PKI discovery for browser/BYO bootstrap. |

mTLS uses **TLS 1.3 only**; older versions and insecure ciphers are rejected. Revocation is enforced on every handshake against the `revoked_certificates` collection.

---

## Architecture at a Glance

```
┌─────────────────────────────────────────────────────────────────────┐
│                  Hub  (g8eo --listen)  — Data Backplane             │
│                                                                     │
│  Document Store  │  KV Store (TTL)  │  SSE Buffer  │  Blob Store   │
│  ─────────────── │  ──────────────  │  ──────────  │  ──────────   │
│  Platform docs   │  Sessions        │  SSE replay  │  Attachments  │
│  (JSON)          │  Nonces / cache  │  ring buffer │  Certs        │
│                                                                     │
│  Root CA / PKI   │  Secrets Vault   │  Pub/Sub Broker (WSS, mTLS)  │
│  State Root      │  Nonce Manager   │  Audit Vault (encrypted)     │
│                                                                     │
│                  SQLite  .g8e/data/g8e.db                           │
└─────────────────────────────────────────────────────────────────────┘
                          ▲                ▲
                 mTLS / canonical JSON     │
                          │                │
            ┌─────────────┘                └──────────────┐
            │                                              │
   ┌────────┴────────┐                          ┌──────────┴──────────┐
   │  BYO clients,   │                          │  Satellite Operators│
   │  g8ee adapter   │                          │  on managed hosts   │
   └─────────────────┘                          └─────────────────────┘
```

---

## Storage Model

All Hub state lives in a single SQLite database at `.g8e/data/g8e.db`, with separate files for the local Encryption Vault. Application adapters are stateless and rely entirely on the Coordination Store.

### State Merkle Root invariant

Hub state is anchored by a Merkle state root computed deterministically across all documents, active KV entries, and blobs. Every governance transaction carries `state_merkle_root`; the Operator rejects any transaction whose root does not match the current authoritative state. This makes it impossible for an agent to act on stale reality.

### Cache-Aside read/write contract

- **Writes** — Always go to the authoritative DB first, then invalidate the cache key.
- **Reads** — `get_document` checks the KV cache; on miss it fetches from the DB and warms the cache. `query_documents` hashes query parameters for result caching.
- **Atomic array ops** — `arrayUnion`/`arrayRemove` operate on the DB and invalidate the cache.
- **Write-Only mode** — Application adapters set `enable_cache_read: false`, ensuring every read is satisfied by the authoritative database while still populating the cache for ecosystem consumers.

### PKI and Secrets directories (root of trust)

`.g8e/pki/` stores the CA hierarchy and trust bundles:

- **Root CA** — `root/root_ca.crt`
- **Intermediate CAs** — Hub CA (signs Operator-listen service certs), Operator CA (signs Satellite operators on enrollment), Bootstrap CA (signs temporary discovery certs).
- **Trust Bundles** — `trust/hub-bundle.pem` (Root + Hub Intermediate).

`.g8e/secrets/` stores tamper-evident bootstrap material:

- `session_encryption_key`, `warden_signing_key`, `warden_key_id`.
- `bootstrap_digest.json` — SHA-256 digests of every secret. On startup, `SecretManager` validates each secret matches the manifest. Mismatch fails startup hard with actionable errors.

### Canonical collections

Defined in `@/home/bob/g8e/protocol/constants/collections.json`. Selected groups:

- **Authentication & sessions** — `users`, `web_sessions`, `operator_sessions`, `cli_sessions`, `bound_sessions`, `api_keys`, `passkey_challenges`.
- **Organizations & tenants** — `organizations`.
- **Audit & security** — `login_audit`, `auth_admin_audit`, `account_locks`, `console_audit`, `revoked_certificates`.
- **Operators** — `operators`, `operator_usage`.
- **Cases & investigations** — `cases`, `investigations`, `tasks`.
- **Governance & reputation** — `tribunal_commands`, `reputation_state`, `reputation_commitments`, `stake_resolutions`.
- **AI & context** — `memories`, `agent_activity_metadata`.
- **Configuration** — `settings`.

---

## Identity & PKI

The Hub is the only authority permitted to sign certificates. Long-lived API keys are deprecated for identity; the platform relies on short-lived, session-bound mTLS certificates.

### Workload identity

Identities follow the SPIFFE URI scheme via `protocol.WorkloadIdentity` helpers:

| Role | Helper | URI SAN |
|---|---|---|
| Operator (Satellite) | `OperatorSPIFFEID(org, op, session)` | `spiffe://g8e.local/operator/<org>/<op>/<session>` |
| CLI (BYO client) | `CLISPIFFEID(user, session)` | `spiffe://g8e.local/cli/<user>/<session>` |
| Application (agent) | `AppSPIFFEID(operator)` | `spiffe://g8e.local/app/<operator>` |
| Hub (Listen) | `HubSPIFFEID()` | `spiffe://g8e.local/hub/operator-listen` |

### CLI vs Operator separation

CLI and Operator are cryptographically distinct principals with separate keys, separate CSRs, and separate certificates:

- **Operator certificates** — Bound to `operator_session_id`. Authorize host-side mutations.
- **CLI certificates** — Bound to `cli_session_id`. Authorize BYO/CLI clients to issue commands and receive SSE.

This means CLI sessions cannot impersonate operator agents and operator sessions cannot drain another client's event stream. SSE routes are bound to CLI sessions specifically.

### Enrollment

1. Client fetches the Hub's root CA fingerprint from `GET /.well-known/pki/fingerprint`.
2. Client presents a one-time device-link token plus a hardware-derived `system_fingerprint` to the Bootstrap Port.
3. Client generates two private keys (Operator + CLI) and submits two CSRs (`csr_pem`, `cli_csr_pem`).
4. Hub signs both with the Operator Intermediate CA (with role-specific URI SANs) and returns both chains.
5. Client uses each cert for its respective role on the API and Pub/Sub ports.

### Warden public key export

The Warden's Ed25519 signing key (`.g8e/secrets/warden_signing_key`) generates a deterministic public key on every startup, exported to:

- `.g8e/pki/warden_pub.pem` (PEM)
- `.g8e/pki/warden_pub.json` (`{key_id, public_key, algorithm}`)

External verifiers (the evals harness, BYO auditors) load these to verify signed `ActionReceipt`s offline.

---

## Pub/Sub Broker

The Hub is the WSS broker and governance gate for all real-time traffic.

- **Channel format** — `{prefix}:{operator_id}:{operator_session_id}`. Always parse with a bounded split (`SplitN(channel, ":", 3)`).
- **Mutation channels** — `cmd:*` and `auditor:*` only accept envelopes via `POST /api/governance/envelope`; `/pubsub/publish` returns `409 Conflict` for these prefixes.
- **Non-mutation channels** — `heartbeat:*`, `results:*`, `sse:*`, `ws_session:*`, `internal:*` flow through `/pubsub/publish`.
- **Fail-closed** — Missing `message_id` or `operator_session_id` → reject. Unknown `event_type` → drop (no handler dispatch). Missing `TransactionVerifier` or `Warden` → reject all inbound commands.
- **Subscribe-and-wait** — Subscribers must wait for the broker's `{"type":"subscribed","channel":"..."}` ack before publishing or dispatching commands on the channel.

---

## Audit Vault (Hub side)

The Hub keeps an authoritative encrypted audit vault keyed by `transaction_hash` for every governed mutation. ActionReceipts are queryable via the protected audit API; the evals harness uses `/api/audit/receipts?tx_id=<hash>` to fetch and verify receipts offline. Audit writes are fail-closed: events with missing or unknown `operator_session_id` are rejected. Sessions must be created explicitly by the auth lifecycle before audit writes are accepted.

---

## Lifecycle

1. **Bootstrap (first boot)** — Hub generates the ECDSA P-384 CA hierarchy, intermediate CAs, trust bundles, session encryption key, Warden signing keypair, and `bootstrap_digest.json`. Server certs for the API and public ports are issued.
2. **Identity bootstrap (zero-touch)** — `./g8e platform start -a` creates a local superadmin (`superadmin@g8e.local`) and issues an mTLS cert for the loopback CLI. The first real `./g8e login --email …` permanently retires the bootstrap user; re-bootstrap requires `./g8e platform clean`.
3. **Steady state** — The Hub serves the four ports, dispatches governed envelopes through verification → Warden → audit, and fans results back via pub/sub and SSE.
4. **Reset/wipe** — `./g8e platform wipe` clears app data through the API but preserves PKI, secrets, and platform settings. `./g8e platform reset` deletes data + bootstrap secrets, preserves PKI roots. `./g8e platform clean` destructive removal of `.g8e/`.

For full lifecycle commands see [Scripts](scripts.md).

---

## Implementation Reference

| Concern | File |
|---|---|
| Listen mode entry | `@/home/bob/g8e/services/g8eo/cmd/g8eo/main.go` |
| Coordination Store | `@/home/bob/g8e/services/g8eo/internal/services/storage/` |
| Pub/Sub broker | `@/home/bob/g8e/services/g8eo/internal/services/pubsub/` |
| State Root provider | `@/home/bob/g8e/services/g8eo/internal/services/listen/listen_db.go` |
| Nonce / replay store | `@/home/bob/g8e/services/g8eo/internal/services/storage/replay_store.go` |
| PKI / CertStore | `@/home/bob/g8e/services/g8eo/internal/services/listen/listen_certs.go` |
| Secret Manager | `@/home/bob/g8e/services/g8eo/internal/services/listen/secret_manager.go` |
| Audit Vault | `@/home/bob/g8e/services/g8eo/internal/services/storage/audit_vault.go` |
| Workload identity | `@/home/bob/g8e/protocol/workload_identity.go` |
| Collections registry | `@/home/bob/g8e/protocol/constants/collections.json` |
| Channels registry | `@/home/bob/g8e/protocol/constants/channels.json` |

See also: [Protocol](protocol.md), [Operator](operator.md), [g8ee Service](g8ee_service.md).
