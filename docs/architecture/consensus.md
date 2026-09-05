# Consensus

Last Updated: 2026-09-05
Version: v2.1.4

## Overview

L2 Consensus is a protocol concept in the g8e governance pipeline. The g8e Consensus is the reference implementation of the L2 layer: an enrolled body of agentic members that independently evaluate a transaction's payload and produce signed Ed25519 votes. A transaction must collect enough affirmative votes from distinct members to pass L2 verification. The Consensus is fail-closed under the `consensus` and `notary` postures. Under the `doctrine` posture, L2 votes are verified and recorded for audit, but they do not gate execution.

See [Authentication and Authorization](./auth.md) for the full five-layer verification pipeline and [Governance](./governance.md) for the envelope structure.

---

## Consensus Policy

A consensus policy names a consensus body and the rules it uses. The policy fields are:

| Field | Description |
| --- | --- |
| **ID** | Consensus name or identifier; letters, digits, hyphens, and underscores only. |
| **Member App IDs** | List of member app IDs; each must resolve to an enabled TrustedSigner. |
| **Quorum** | Minimum number of affirmative, distinct member signatures required. |
| **Require Distinct** | When true, duplicate member app IDs in a vote set are rejected. |
| **Enabled** | Whether the policy is active. |

### Validation

All consensus policy writes are validated fail-closed. The ID must be non-empty and contain only allowed characters. The member list must be non-empty and contain no duplicate or empty entries. The quorum must be at least 1 and no greater than the member count. Every member app ID must resolve to an enabled TrustedSigner. New policies must be created with `enabled=true`. Existing policies may only be updated by setting `enabled=false`; overwriting an existing enabled policy is rejected as a duplicate.

---

## Consensus Enrollment

### Declarative Bootstrap

The `--consensus-bootstrap` flag, or the `G8E_CONSENSUS_BOOTSTRAP` environment variable, points to a JSON file that seeds a consensus policy and its trusted signers at gateway startup. The file must contain `consensus_id`, `member_app_ids`, and `quorum`. It may also contain `member_seeds` and `seed_hex`.

When `member_seeds` is supplied, every listed member must have its own seed. Each seed is used to derive a per-member Ed25519 key pair. The public key is registered as a TrustedSigner for that member, and the private key is saved to the gateway secrets directory so the in-process deliberator can sign votes with distinct per-member keys.

When `member_seeds` is omitted, a single key pair is derived from `seed_hex` or generated fresh and shared across all members. The policy is always created with `require_distinct=true` and `enabled=true`. If the policy already exists, the bootstrap is skipped so restarts are safe.

### Admin API

Consensus policies can also be created, listed, and deleted at runtime by a bootstrap user through the admin REST API. `POST /api/v1/admin/consensus` creates a policy and returns `201 Created` on success. `GET /api/v1/admin/consensus` lists all policies. `DELETE /api/v1/admin/consensus/{id}` deletes a policy by ID.

### Member Key Management

Each consensus member has its own Ed25519 private key, stored on disk as a hex-encoded seed with restricted file permissions. Members never share the gateway identity key; even in single-binary deployments, each member has its own key. At runtime, the gateway first looks for a member's file-based key, then falls back to the gateway actuator key when the member's app ID matches the actuator key ID. Members whose keys cannot be resolved are included in the policy, but they cannot sign votes and a warning is logged.

---

## Deliberation

When a transaction requires L2 consensus, the governance envelope is sent to the Consensus for deliberation. The Consensus processes the envelope and returns it with L2 metadata populated: the consensus ID and the list of signed votes.

### Deliberation Process

Deliberation proceeds as follows. The envelope's message ID is recomputed and compared to the declared ID; a mismatch fails deliberation. Command data is extracted from the envelope payload or intent data. Each member that has a private key evaluates the command data against the L1 Doctrine, and if any MITRE signal recommends a block the payload is deemed unsafe. The member signs its safe or unsafe decision with its Ed25519 private key. If no members have a private key, deliberation fails closed. Finally, the consensus ID and vote list are attached to the envelope.

### Delivery Mechanisms

**Local (in-process):** In single-binary gateway deployments, deliberation runs in-process through the local deliberator, without an HTTP round-trip. This is the default for the g8e gateway.

**Remote (HTTP):** The Consensus also exposes an mTLS-guarded `POST /consensus/v1/deliberate` endpoint. The endpoint accepts a canonical protojson governance envelope with a 1 MiB body limit and returns the envelope with L2 votes populated. The route is registered only in `consensus` and `notary` postures; `doctrine` and `ratify` postures do not register the route, so requests return `404 Not Found` (there is no 503-on-nil guard — the handler is only reachable when consensus is non-nil).

**MCP Gateway Integration:** Under `consensus` and `notary` postures, the MCP gateway automatically sends the envelope to the L2 deliberator before dispatch. If the deliberator is not configured, the envelope proceeds without L2 votes and fails closed at L4 verification under enforced postures.

---

## L4 Warden: L2 Vote Verification

Before a transaction is dispatched, the L4 Warden validates the L2 votes inside the governance envelope. The checks are posture-aware: under `doctrine` they are recorded and audited; under `consensus` and `notary` they are fail-closed.

1. **Vote presence:** If L2 metadata is absent or has no votes, enforced postures reject the transaction.
2. **Store checks:** The signer store and consensus policy store must be configured under enforced postures.
3. **Policy lookup:** The consensus policy is loaded by consensus set ID. A missing or disabled policy is rejected under enforced postures.
4. **Member validation:** Votes whose signer key IDs are not in the policy's member list are silently excluded from the count.
5. **Duplicate signer detection:** If `require_distinct` is true, duplicate signer key IDs in the vote set are rejected. This check is enforced regardless of posture: a duplicate signer indicates a malformed vote set, not a policy decision, and is rejected fail-closed under all postures including `doctrine`.
6. **Signature verification:** Each vote's Ed25519 signature is verified against the trusted public key in the signer store. Invalid signatures are excluded from the quorum count.
7. **Quorum check:** The count of affirmative votes from valid, distinct members must meet or exceed the policy's quorum.

### Signature Format

The L2 signature covers the string `transaction_hash|decision`, where `decision` is `true` or `false`. The signature is Ed25519, hex-encoded. Empty or `UNSIGNED` signature strings are rejected before decoding.

### Posture-Dependent Enforcement

| Check | Doctrine | Consensus | Notary |
| --- | --- | --- | --- |
| L2 vote presence | Audited | Enforced | Enforced |
| Signer store configured | Audited | Enforced | Enforced |
| Consensus store configured | Audited | Enforced | Enforced |
| Consensus policy exists and enabled | Audited | Enforced | Enforced |
| Member validation | Audited | Enforced | Enforced |
| Duplicate signer detection | Enforced | Enforced | Enforced |
| Signature verification | Audited | Enforced | Enforced |
| Quorum met | Audited | Enforced | Enforced |

Under `doctrine`, L2 checks run only when the signer store and consensus policy store are both configured and L2 votes are present in the envelope. If the stores are not configured or no votes are present, L2 verification is skipped entirely. When checks do run, the `L2Valid` result is computed and stored in the `VerificationResult`, but it does not gate execution. The L5 Actuator records `L2_STATUS_NOT_REQUIRED` in the action receipt under doctrine, because `RequiresL2Signature()` returns false; the per-vote verification result is not surfaced as a valid/failed status in the receipt.

Duplicate signer detection is the one exception: it is enforced under all postures, including `doctrine`. A duplicate signer in a vote set is a malformed input, not a policy decision, and is rejected fail-closed regardless of posture.

---

## Gateway Bootstrap Sequence

The consensus bootstrap sequence at gateway startup is:

1. If `--consensus-bootstrap` is set, the gateway seeds trusted signers and the consensus policy from the JSON file before the L2 posture advisory check.
2. For `consensus` and `notary` postures, the gateway logs a warning if the consensus ID is empty or the policy is not found or disabled. If a valid policy exists, it logs the consensus ID, member count, and quorum.
3. For `consensus` and `notary` postures with a non-empty `--consensus-id`, the gateway loads the policy, builds the in-process consensus service with a file-based key provider and actuator fallback, and wires the local deliberator into the MCP gateway.
4. Under `doctrine` posture, the Consensus service is not constructed.

### Bootstrap Enrollment Under Enforced Consensus

Bootstrap platform enrollment actions (operator, dashboard, ensemble) no longer require an L2 tribunal that does not yet exist at first-boot. Supplied votes on bootstrap enrollment envelopes are still verified and audited, but the L2 quorum gate is not enforced for bootstrap enrollment actions. Non-bootstrap mutations continue to enforce the configured posture. This allows a fresh gateway under `consensus` or `notary` posture to accept its first platform enrollment requests before any consensus members are enrolled.

The `POST /consensus/v1/deliberate` route is registered at startup only in `consensus` and `notary` postures. `doctrine` and `ratify` postures do not register the route, so requests return `404 Not Found`.

---

## End-to-End Transaction Flow with Consensus

1. A client or agent submits an intent to the MCP gateway.
2. The gateway builds a governance envelope, computes the transaction hash, and sets the envelope ID.
3. Under `consensus` or `notary` posture, the gateway sends the envelope to the L2 deliberator. Each member with a private key evaluates the payload against the L1 Doctrine and signs its decision. The envelope returns with L2 metadata populated.
4. The L4 Warden verifies the envelope: structural and hash checks, nonce and expiry validation, state root consistency, and L2 posture enforcement.
5. The L5 Actuator executes the transaction, records the L2 and L3 status in the action receipt, and signs the receipt with the actuator key.

See [Governance](./governance.md) for the envelope structure and [Authentication and Authorization](./auth.md) for the full five-layer pipeline.

---

## Key Design Decisions

- **Fail-closed by default:** If the L1 Doctrine is not configured, the payload is not safe. If consensus stores are missing under enforced postures, transactions are rejected. If no members have signing keys, deliberation fails.
- **No key sharing:** Each consensus member has its own Ed25519 key, distinct from the gateway identity key, even in single-binary deployments.
- **Protocol ordering:** L2 machine consensus signs the transaction hash before L3 notary approval is requested, so human review only happens after all machine-checkable layers pass.
- **Idempotent bootstrap:** If the consensus policy already exists, the bootstrap skips creation so restarts are safe.
- **Generic policy interface:** The L4 Warden depends on a generic consensus policy interface, not the reference Consensus type, so alternative consensus implementations can be enrolled without changing the verification gate.
