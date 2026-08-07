# Consensus

Last Updated: 2026-08-07
Version: v1.7.0

## Overview

L2 Consensus is a protocol concept defined in the g8e protocol. The Consensus is the reference implementation of the L2 Consensus layer shipped with g8e: an enrolled body of agentic applications that independently evaluate each transaction's payload against the L1 Doctrine and produce signed Ed25519 votes. A transaction must collect enough affirmative votes (quorum) from distinct consensus members to pass L2 verification. Alternative implementations can be built against the same protocol interfaces.

The Consensus operates as a fail-closed gate under the `consensus` and `notary` postures. Under `doctrine` posture, L2 votes are verified if present and recorded in the receipt, but do not gate execution.

See [Authentication & Authorization](./auth.md) for the full 5-layer verification pipeline and [Governance](./governance.md) for the envelope structure.

---

## Consensus Policy

A consensus policy defines a named consensus body with the following properties:

| Property | Description |
|---|---|
| **ID** | Consensus name or identifier (alphanumeric, hyphens, underscores only) |
| **Member App IDs** | List of member app IDs; each must resolve to an enabled TrustedSigner |
| **Quorum** | Minimum number of affirmative distinct signatures required (K of N) |
| **Require Distinct** | If true, duplicate signer key IDs in a vote set are rejected |
| **Enabled** | Whether this consensus is active |

### Validation

All validation is fail-closed at write time:

- Consensus ID must be non-empty and contain only alphanumeric characters, hyphens, and underscores.
- Member list must be non-empty, with no empty strings or duplicates.
- Quorum must be at least 1 and at most the member count.
- Every member app ID must resolve to an enabled TrustedSigner in the signer store.
- New policies must be created enabled. Existing policies may only be updated via the disable path. Overwriting an existing policy with `enabled=true` is rejected as a duplicate.

---

## Consensus Enrollment

### Declarative Bootstrap (File-Based)

Consensus policies can be seeded at gateway startup via the `--consensus-bootstrap` flag (or `G8E_CONSENSUS_BOOTSTRAP` environment variable). The config file is a JSON document:

```json
{
  "consensus_id": "dhs-consensus",
  "member_app_ids": ["auditor-ensemble"],
  "quorum": 1,
  "member_seeds": {
    "auditor-ensemble": "3b8237753873a5dcc78fddcdf6011ea9bea03c0ae683a8a8fb5f4cba928e8a15"
  }
}
```

For multi-member consensus with distinct per-member keys:

```json
{
  "consensus_id": "fedramp-consensus",
  "member_app_ids": ["fedramp-csp-auditor", "fedramp-3pao", "fedramp-jab"],
  "quorum": 2,
  "member_seeds": {
    "fedramp-csp-auditor": "b194523218024feacafef9acf9e557f9c2e6ed71e87c8c97e5a4fc61e624ea42",
    "fedramp-3pao": "20544a8efd3f30188095dae9d42993f320fbcdcbd924f88b2c56edfdc719e357",
    "fedramp-jab": "06946f1a26896983176f6d40b0a734136dd58b16fe502d4b5688bf7db1b97662"
  }
}
```

| Field | Required | Description |
|---|---|---|
| `consensus_id` | Yes | The consensus ID to create |
| `member_app_ids` | Yes | List of member app IDs to enroll |
| `quorum` | Yes | Quorum threshold (must be at least 1) |
| `member_seeds` | No | Map of member app IDs to hex-encoded Ed25519 seeds. Each member gets its own derived key pair, making distinct-signer enforcement and quorum cryptographically meaningful. Takes precedence over `seed_hex`. |
| `seed_hex` | No | Hex-encoded Ed25519 seed for deterministic key generation (single-key fallback). If omitted, a fresh key pair is generated. Ignored when `member_seeds` is present. |

#### Bootstrap Process

The bootstrap is idempotent: if the consensus already exists, creation is skipped, enabling safe restarts. When `member_seeds` is provided, each member gets a distinct Ed25519 key pair derived from its seed. The public key is registered as a TrustedSigner for that member, and the private key is saved to disk so the in-process deliberator can sign L2 votes with distinct per-member keys. This makes BFT quorum cryptographically meaningful: a single key cannot satisfy a multi-member quorum.

When `member_seeds` is omitted, a single key pair is derived from `seed_hex` (or freshly generated) and shared across all members. The policy is then created with `require_distinct=true` and `enabled=true`.

### Admin API (Runtime Enrollment)

Consensus policies can also be created at runtime via the admin REST API, which requires bootstrap user authentication:

- `POST /api/v1/admin/consensus`: Create a new consensus policy. Returns `201 Created` on success or `400 Bad Request` on validation failure.
- `GET /api/v1/admin/consensus`: List all consensus policies. Returns `200 OK` with a JSON array.
- `DELETE /api/v1/admin/consensus/{id}`: Delete a consensus policy. Returns `200 OK` or `404 Not Found`.

### Member Key Management

Each consensus member has its own Ed25519 private key, stored on disk as a hex-encoded seed with restricted file permissions. Members never share the gateway identity key; even in single-binary deployments, each member has a distinct key.

In production bootstrap, the key provider tries file-based keys first, then falls back to the gateway's actuator signing key if the member's app ID matches the actuator's key ID. Members whose keys cannot be resolved are included in the policy without a private key; they can participate in policy but cannot sign votes, and a warning is logged.

---

## Deliberation

When a transaction requires L2 consensus, the governance envelope is sent to the Consensus for deliberation. The Consensus processes the envelope through all members and returns it with L2 metadata populated (consensus ID plus signed votes).

### Deliberation Process

1. **Hash verification**: the envelope's message ID is recomputed and compared to the declared ID. A mismatch fails deliberation (fail-closed).
2. **Command data extraction**: command data and intent are extracted from the envelope.
3. **Per-member evaluation**: for each member with a private key, the payload is evaluated against the L1 Doctrine. If any MITRE signal recommends a block, the payload is deemed unsafe. If doctrine is not configured, the payload is fail-closed (not safe). The member signs its decision with its Ed25519 private key.
4. **No-signing-members check**: if no members have private keys, deliberation fails (fail-closed).
5. **L2 metadata population**: the consensus ID and vote list are attached to the envelope, ready for submission to the L4 Warden.

### Delivery Mechanisms

**Local (In-Process)**: In single-binary gateway deployments, deliberation runs in-process without an HTTP round-trip. This is the default mode.

**Remote (HTTP)**: The Consensus exposes an mTLS-guarded HTTP endpoint for remote deliberation:

- Route: `POST /consensus/v1/deliberate`
- Wire format: canonical protojson governance envelope (1 MiB max body)
- Response: the envelope with L2 votes populated, as protojson

The route is always registered on the mTLS mux. If the consensus service is not configured, the endpoint returns `503 Service Unavailable`.

**MCP Gateway Integration**: Under `consensus` and `notary` postures, the MCP gateway automatically sends the envelope to the Consensus for L2 deliberation before dispatch. If the deliberator is not configured, the envelope proceeds without L2 votes and will fail-closed at L4 verification under enforced postures.

---

## L4 Warden: L2 Vote Verification

When a governance envelope arrives at the gateway, the L4 Warden validates the L2 votes:

1. **Vote presence**: if L2 metadata is absent or has zero votes, enforced postures reject the transaction. Under `doctrine` posture, the result is audited but not enforced.
2. **Store checks**: the signer store and consensus policy store must be configured. Under enforced postures, missing stores fail-closed.
3. **Consensus policy lookup**: the policy is loaded by consensus set ID. Under enforced postures, a missing or disabled policy is rejected.
4. **Member validation**: votes from signer key IDs not in the policy's member list are silently excluded from the quorum count.
5. **Duplicate signer detection**: if `RequireDistinct` is true, duplicate signer key IDs in the vote set are rejected.
6. **Signature verification**: for each vote, the Ed25519 signature is verified against the trusted public key loaded from the signer store. Invalid signatures are excluded from the quorum count; the vote simply does not count.
7. **Quorum check**: the count of affirmative votes from valid, distinct members must meet or exceed the policy's quorum. Under enforced postures, failure rejects the transaction.

### Signature Format

The L2 signature payload is the string `"<transaction_hash>|<decision>"` where `decision` is `true` or `false`. The signature is Ed25519, hex-encoded. Verification rejects empty or `UNSIGNED` signature strings before attempting decode.

### Posture-Dependent Enforcement

| Check | Doctrine | Consensus | Notary |
|---|---|---|---|
| L2 vote presence | Audited | Enforced | Enforced |
| Signer store configured | Audited | Enforced | Enforced |
| Consensus store configured | Audited | Enforced | Enforced |
| Consensus policy exists and enabled | Audited | Enforced | Enforced |
| Member validation | Audited | Enforced | Enforced |
| Duplicate signer detection | Audited | Enforced | Enforced |
| Signature verification | Audited | Enforced | Enforced |
| Quorum met | Audited | Enforced | Enforced |

Under `doctrine` posture, all L2 checks are audited but do not gate execution; the result is recorded in the receipt.

---

## Gateway Bootstrap Sequence

The full consensus bootstrap sequence at gateway startup:

1. **Bootstrap flag**: if `--consensus-bootstrap` is set, trusted signers and the consensus policy are seeded from the JSON config file before L2 posture validation runs.
2. **L2 posture advisory check**: for `consensus` and `notary` postures, the gateway logs warnings if the consensus ID is empty or the policy is not found or disabled. If the policy exists and is enabled, it logs the consensus ID, member count, and quorum.
3. **Consensus service bootstrap**: for `consensus` and `notary` postures with a non-empty `--consensus-id`, the gateway loads the policy, constructs a file-based key provider with actuator key fallback, builds the consensus service, and wires the in-process deliberator into the MCP gateway.
4. **Doctrine posture**: the Consensus is not constructed.

The consensus service is wired into the governance HTTP handler at construction time. If consensus is not configured for the current posture, the deliberate endpoint returns `503 Service Unavailable`. The L2 deliberator is wired into the MCP gateway through runtime dependencies, which are set atomically before the first request.

---

## End-to-End Transaction Flow with Consensus

1. A client or agent submits an intent to the MCP gateway.
2. The gateway builds a governance envelope, computes the transaction hash, and sets the envelope ID.
3. Under `consensus` or `notary` posture, the gateway sends the envelope to the L2 deliberator. Each member with a private key evaluates the payload against the L1 Doctrine and signs its decision with Ed25519. The envelope returns with L2 metadata (consensus ID and votes) populated.
4. The L4 Warden verifies the envelope: structural and hash checks, nonce and expiry validation, state root consistency, and L2 posture enforcement (policy lookup, member validation, signature verification, quorum check).
5. The L5 Actuator executes the transaction, records L2 and L3 status in the action receipt, and signs the receipt with the actuator key.

See [Governance](./governance.md) for the envelope structure and [Authentication & Authorization](./auth.md) for the full 5-layer pipeline.

---

## Key Design Decisions

- **Fail-closed by default**: if doctrine is not configured, the payload is not safe. If consensus stores are missing under enforced postures, transactions are rejected. If no members have signing keys, deliberation fails.
- **No key sharing**: each consensus member has its own Ed25519 key, distinct from the gateway identity key, even in single-binary deployments.
- **Protocol ordering**: L2 (machine consensus) signs the transaction hash before L3 (human notary) is asked. The human is never bothered until all machine-checkable layers pass. L3 proof is intentionally excluded from the transaction hash to avoid circular dependencies.
- **Gateway-internal deliberation**: under `consensus` and `notary` postures, the MCP gateway automatically sends the envelope for L2 deliberation before dispatch. Demo scenarios use the MCP tools/call endpoint; the gateway builds the governance envelope internally, runs L2 deliberation, and suspends for L3 notary approval if required. The harness then waits for human browser approval via the SSE stream for the `approval.completed` event and verifies the approval status via the mTLS status endpoint.
- **Human browser approval for L3**: the demo harness drives the real out-of-band L3 notary flow. It prints the approval URL, subscribes to the gateway's SSE stream for `approval.completed` events matching the transaction hash, and blocks until a human completes the WebAuthn passkey ceremony in their browser. The gateway performs full real verification. The `G8E_L3_MOCK` environment variable has been removed; the gateway always requires real WebAuthn proof.
- **Idempotent bootstrap**: if the consensus already exists, the bootstrap skips creation, enabling safe restarts.
- **Construction-time injection**: the consensus service is wired into the governance HTTP handler at construction time, and the L2 deliberator is wired into the MCP gateway through runtime dependencies. This eliminates the need for individual atomic late binding or router rebuilds.
- **Shared factory**: the same construction logic is used by both production bootstrap and test fixtures, ensuring production and test code paths exercise the same code.
- **Generic policy interface**: the L4 Warden depends on a generic consensus policy interface, not on any consensus-specific type, allowing alternative consensus implementations without warden modifications.
