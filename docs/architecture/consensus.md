# Consensus

Last Updated: 2026-07-28
Version: v1.6.6

## Overview

L2 Consensus is a protocol concept defined in the g8e protocol (`L2Metadata`, `L2Vote`). The Consensus is the **reference implementation** of the L2 Consensus layer shipped with g8e: an enrolled body of agentic applications that independently evaluate each transaction's payload against the L1 Doctrine and produce signed Ed25519 votes. A transaction must collect enough affirmative votes (quorum) from distinct consensus members to pass L2 verification. Alternative implementations can be built against the same protocol interfaces.

The Consensus operates as a **fail-closed gate** under the `consensus` and `notary` postures. Under `doctrine` posture, L2 votes are verified if present and recorded in the receipt, but do not gate execution.

---

## ConsensusPolicy

A ConsensusPolicy defines a named consensus body with the following properties:

- **ID**: Consensus name or identifier (alphanumeric, hyphens, underscores only)
- **MemberAppIDs**: List of member app IDs; each must resolve to an enabled TrustedSigner
- **Quorum**: Minimum number of affirmative distinct signatures required (K of N)
- **RequireDistinct**: If true, duplicate signer key IDs in a vote set are rejected
- **Enabled**: Whether this consensus is active

### Storage

Consensus policies are persisted in the gateway's SQLite document store under the `consensus` collection. The consensus store service provides CRUD operations: retrieve by ID (returns nil if not found), create or update (with fail-closed validation), list all policies, and delete by ID.

The L4 Warden depends on a generic `L2ConsensusPolicyStore` interface rather than any consensus-specific type, allowing alternative consensus implementations to be plugged in without modifying the warden. The consensus store service satisfies this interface by adapting ConsensusPolicy to the generic L2ConsensusPolicy struct.

### Validation

All validation is fail-closed at write time:

- **Consensus ID**: non-empty, alphanumeric + hyphens + underscores only
- **Member list**: non-empty, no empty strings, no duplicates
- **Quorum**: at least 1 and at most the member count
- **Trusted signer check**: every member app ID must resolve to an enabled TrustedSigner in the signer store
- **Existence check**:
  - New policies must be created with `enabled=true` (rejects `enabled=false` for non-existent IDs)
  - Existing policies may only be updated via `enabled=false` (disable path)
  - Overwriting an existing policy with `enabled=true` is rejected as a duplicate

---

## Consensus Enrollment

### Declarative Bootstrap (File-Based)

Consensus policies can be seeded at gateway startup via the `--consensus-bootstrap` flag (or `G8E_CONSENSUS_BOOTSTRAP` env var). The config file is a JSON document:

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
| `quorum` | Yes | Quorum threshold (must be `>= 1`) |
| `member_seeds` | No | Map of member app IDs to hex-encoded Ed25519 seeds. Each member gets its own derived key pair, making `RequireDistinct` and quorum cryptographically meaningful. Takes precedence over `seed_hex`. |
| `seed_hex` | No | Hex-encoded Ed25519 seed for deterministic key generation (single-key fallback). If omitted, a fresh key pair is generated. Ignored when `member_seeds` is present. |

#### Bootstrap Process

The bootstrap function executes the following steps:

1. **Read and parse** the JSON config file
2. **Idempotency check**: if the consensus already exists, skip bootstrap
3. **Key mode selection**: if `member_seeds` is present, use per-member key mode; otherwise fall back to `seed_hex` (single-key) or generate a fresh key pair
4. **Per-member key derivation** (when `member_seeds` is present): for each member app ID, derive a distinct Ed25519 key pair from the member's seed. The public key is registered as a TrustedSigner for that member, and the private key is saved to the secrets directory.
5. **Single-key derivation** (when `member_seeds` is absent): derive one Ed25519 key pair from `seed_hex` (or generate a fresh one) and register it as the TrustedSigner for every member
6. **Member key persistence**: save each member's private key to disk so the in-process LocalDeliberator can sign L2 votes via FileKeyProvider
7. **ConsensusPolicy creation**: insert the policy into the consensus store with `enabled=true` and `require_distinct=true`

When `member_seeds` is used, each member signs L2 votes with its own distinct Ed25519 key. The L4 Warden verifies each vote's signature against the member's registered public key, and `RequireDistinct` ensures a single key cannot satisfy a multi-member quorum. This makes BFT quorum cryptographically meaningful.

### Admin API (Runtime Enrollment)

Consensus policies can also be created at runtime via the admin REST API (requires bootstrap user authentication):

- **`POST /api/v1/admin/consensus`**: Create a new consensus policy. Returns `201 Created` or `400 Bad Request` on validation failure.
- **`GET /api/v1/admin/consensus`**: List all consensus policies. Returns `200 OK` with a JSON array.
- **`DELETE /api/v1/admin/consensus/{id}`**: Delete a consensus policy. Returns `200 OK` or `404 Not Found`.

### Member Key Management

Each consensus member has its own Ed25519 private key, stored on disk as a hex-encoded seed. The FileKeyProvider loads keys using a naming convention within the secrets directory. Keys are written with restricted file permissions to a private secrets directory.

The KeyProvider interface abstracts key resolution: implementations may load keys from disk, use an in-process actuator key, or source them from any secure backing store.

In production bootstrap, the key provider tries FileKeyProvider first, then falls back to the gateway's actuator signing key if the member's AppID matches the actuator's key ID. Members whose keys cannot be resolved are included in the policy without a private key; they can participate in policy but cannot sign votes, and a warning is logged.

---

## Consensus Service

The ConsensusService is the enrolled agentic application that deliberates on governance envelopes and produces L2 consensus votes. It holds a consensus ID, a list of consensus members (each with an AppID and Ed25519 private key), a reference to the L1 Doctrine for deterministic evaluation, a logger, and a response writer.

Each member is an enrolled agentic app with its own Ed25519 signing key. The member's public key is registered as a TrustedSigner (keyID = AppID). Members never share the gateway identity key; even in single-binary deployments, each member has a distinct key.

The shared factory `NewConsensusFromPolicy` constructs a ConsensusService from a ConsensusPolicy and a KeyProvider. It resolves each member's private key via the provider and builds the member list. This factory is used by both production bootstrap and test fixtures, ensuring production and test code paths exercise the same construction logic.

---

## Deliberation: How GovernanceEnvelopes Are Delivered and Processed

### Deliberation Flow

When a transaction requires L2 consensus, the `GovernanceEnvelope` is sent to the Consensus for deliberation. The Consensus processes the envelope through all members and returns it with L2 metadata populated (consensus ID + signed votes).

```
Client/Gateway
    │
    ▼
┌──────────────────────────────────┐
│  L2ConsensusDeliberator          │
│  (LocalDeliberator or HTTP)      │
└──────────┬───────────────────────┘
           │
           ▼
┌──────────────────────────────────┐
│  ConsensusService.Deliberate()    │
│                                  │
│  1. Verify envelope ID == hash   │
│  2. Extract command data + intent│
│  3. For each member with key:    │
│     a. Evaluate safety (L1)      │
│     b. Sign decision (Ed25519)   │
│  4. Populate L2 metadata         │
│  5. Return envelope with votes   │
└──────────────────────────────────┘
```

### Deliberation Process

The Deliberate method performs the following:

1. **Hash verification**: recomputes the envelope's message ID via `GenerateMessageID` and compares it to the envelope's ID. If they don't match, returns `ErrConsensusHashMismatch` (fail-closed).

2. **Command data extraction**: extracts command data and intent from the envelope's `IntentData` (if present) or `Payload`.

3. **Per-member evaluation**: for each member with a non-nil private key:
   - **Safety evaluation**: runs MITRE checks via L1Doctrine. If any signal has `BlockRecommended=true`, the payload is deemed unsafe. If doctrine is nil, the payload is **fail-closed** (not safe).
   - **Vote signing**: signs the string `"<transaction_hash>|<decision>"` with the member's Ed25519 private key. The signature is hex-encoded.

4. **No-signing-members check**: if no members have private keys, returns `ErrConsensusNoSigningMembers` (fail-closed).

5. **L2 metadata population**: sets the consensus ID and vote list on the envelope's L2 metadata. Also initializes L1 and L3 metadata if the governance block is nil.

6. **Return**: a DeliberateResult containing the envelope with L2 metadata populated, ready for submission to the gateway's L4 Warden.

### L2Vote Protocol Structure

The protocol defines two messages for L2 governance:

- **L2Vote**: contains the signer key ID (member AppID, matching a TrustedSigner), the consensus signature (Ed25519 over the transaction hash and decision), and the decision (safe or unsafe).
- **L2Metadata**: contains the consensus set ID and the list of independent member votes.

### Delivery Mechanisms

#### 1. LocalDeliberator (In-Process, Single-Binary Deployment)

The LocalDeliberator is an in-process adapter that satisfies the `L2ConsensusDeliberator` interface by calling `ConsensusService.Deliberate` directly, without an HTTP round-trip. It unmarshals the envelope bytes, runs deliberation, and returns the marshaled envelope with L2 votes populated. This is the default mode for single-binary gateway deployments.

#### 2. HTTP Endpoint (Remote Deliberation)

The Consensus also exposes an mTLS-guarded HTTP endpoint for remote deliberation calls:

- **Route**: `POST /consensus/v1/deliberate`
- **Wire format**: canonical protojson GovernanceEnvelope (1 MiB max body)
- **Response**: the envelope with L2 votes populated, as protojson

The route is always registered on the mTLS mux. The handler checks whether the consensus service is configured; if not, it returns `503 Service Unavailable`.

#### 3. MCP Gateway Integration

Under `consensus` and `notary` postures, the MCP gateway's `processGatewayTransaction` automatically sends the envelope to the Consensus for L2 deliberation before dispatch. If the deliberator is not configured, the envelope proceeds without L2 votes and will fail-closed at L4 verification under consensus/notary postures.

The `L2ConsensusDeliberator` interface is wired into the MCP gateway through `RuntimeDependencies`, which are set atomically before the first request via `SetRuntimeDeps`. This bundles all runtime-phase dependencies into a single atomic assignment, enabling thread-safe initialization without individual atomic fields.

---

## L4 Warden: L2 Vote Verification

### Verification Flow

When a GovernanceEnvelope arrives at the gateway, the L4 Warden's `verifyL2Posture` validates the L2 votes:

1. **Vote presence**: if L2 metadata is nil or has zero votes:
   - Under `consensus`/`notary` posture: reject with `ErrTxL2SignatureMissing`
   - Under `doctrine` posture: return `false, nil` (audited, not enforced)

2. **Store checks**: verifies the signer store and consensus policy store are configured. Under enforced postures, missing stores are fail-closed.

3. **Consensus policy lookup**: loads the consensus policy by consensus set ID. Under enforced postures, a missing or disabled policy is rejected with `ErrTxL2ConsensusNotConfigured`.

4. **Member validation**: votes from signer key IDs not in the policy's member list are silently excluded from the quorum count.

5. **Duplicate signer detection**: if `RequireDistinct=true`, duplicate signer key IDs in the vote set are rejected with `ErrTxL2DuplicateSigner`.

6. **Signature verification**: for each vote, the Ed25519 signature over `"<transaction_hash>|<decision>"` is verified against the trusted public key loaded from the signer store. Invalid signatures are excluded from the quorum count (not rejected; the vote simply doesn't count).

7. **Quorum check**: the count of affirmative votes from valid, distinct members must meet or exceed the policy's quorum. Under enforced postures, failure returns `ErrTxL2QuorumNotMet`.

### Signature Format

The L2 signature payload is the string `"<transaction_hash>|<decision>"` where `decision` is the Go `%v` format of a `bool` (`true` or `false`). The signature is Ed25519, hex-encoded. Verification rejects empty or `UNSIGNED` signature strings before attempting decode.

### Posture-Dependent Enforcement

| Check | Doctrine | Consensus | Notary |
|---|---|---|---|
| L2 vote presence | Audited | **Enforced** | **Enforced** |
| Signer store configured | Audited | **Enforced** | **Enforced** |
| Consensus store configured | Audited | **Enforced** | **Enforced** |
| Consensus policy exists + enabled | Audited | **Enforced** | **Enforced** |
| Member validation | Audited | **Enforced** | **Enforced** |
| Duplicate signer detection | Audited | **Enforced** | **Enforced** |
| Signature verification | Audited | **Enforced** | **Enforced** |
| Quorum met | Audited | **Enforced** | **Enforced** |

Under `doctrine` posture, all L2 checks return `false, nil` when stores/policies are missing; the result is recorded in the receipt but does not gate execution.

---

## Gateway Bootstrap Sequence

The full consensus bootstrap sequence:

1. **`--consensus-bootstrap` flag**: if set, the bootstrap function seeds trusted signers and the ConsensusPolicy from the JSON config file before L2 posture validation runs.

2. **L2 posture advisory check**: for `consensus`/`notary` postures, logs warnings if:
   - `--consensus-id` is empty
   - The consensus policy is not found or disabled
   - If the policy exists and is enabled, logs the consensus ID, member count, and quorum

3. **Consensus service bootstrap**: for `consensus`/`notary` postures with a non-empty `--consensus-id`:
   - `ConsensusBootstrap` loads the ConsensusPolicy from the database
   - Constructs a FileKeyProvider for disk-based member keys
   - Creates a composite KeyProvider that tries file keys first, then falls back to the actuator key
   - Builds the ConsensusService via `NewConsensusFromPolicy`
   - Creates a LocalDeliberator from the ConsensusService
   - Wires the LocalDeliberator into the MCP gateway through RuntimeDependencies (via `SetRuntimeDeps`, called by `NewGatewayOperatorPubSubService`)
   - Wires the ConsensusService into the GovernanceController at construction time via `InitHTTPHandler`

4. **Under `doctrine` posture**: the Consensus is not constructed.

### Construction-Time Injection

The GovernanceController receives the ConsensusService at construction time. If consensus is not configured for the current posture, the controller is constructed with a nil consensus, and the deliberate endpoint returns `503 Service Unavailable`. No atomic late binding or router rebuilds are needed.

The MCP gateway receives the `L2ConsensusDeliberator` through `RuntimeDependencies`, which are set atomically via `SetRuntimeDeps` before the first request. This bundles all runtime-phase dependencies (envelope processor, signing identity, audit logger, L2 deliberator, and others) into a single atomic pointer assignment.

---

## End-to-End Transaction Flow with Consensus

```
1. Client/Agent submits intent
       │
       ▼
2. MCP Gateway processGatewayTransaction()
   ├── Build GovernanceEnvelope
   ├── Compute transaction hash (GenerateMessageID)
   ├── Set env.Id = env.TransactionHash = hash
   │
   ├── If posture == consensus or notary:
   │   └── L2ConsensusDeliberator.Deliberate(envelopeBytes)
   │       └── ConsensusService.Deliberate()
   │           ├── Verify env.Id == recomputed hash
   │           ├── Extract command data
   │           ├── For each member with PrivateKey:
   │           │   ├── Evaluate safety via L1Doctrine (MITRE checks)
   │           │   └── Sign "<hash>|<decision>" with Ed25519
   │           ├── Set L2.ConsensusID + L2.Votes
   │           └── Return envelope with L2 metadata
   │
   └── Return (hash, envelopeBytes)
       │
       ▼
3. L4 Warden VerifyEnvelope()
   ├── Stateless: hash, action type, payload, L1 doctrine
   ├── Stateful: nonce, expiry, state root
   └── Posture: verifyL2Posture()
       ├── Load ConsensusPolicy by L2.ConsensusID
       ├── Validate members, check duplicates
       ├── Verify each vote's Ed25519 signature
       ├── Count affirmative votes
       └── Check quorum: affirmative >= policy.Quorum
       │
       ▼
4. L5 Actuator Execute()
   ├── Execute the transaction
   ├── Record L2/L3 status in ActionReceipt
   └── Sign receipt with actuator key
```

---

## Key Design Decisions

- **Fail-closed by default**: if doctrine is nil, the payload is not safe. If consensus stores are missing under enforced postures, transactions are rejected. If no members have signing keys, deliberation fails.
- **No key sharing**: each consensus member has its own Ed25519 key, distinct from the gateway identity key, even in single-binary deployments.
- **Protocol ordering**: L2 (machine consensus) signs the transaction hash before L3 (human notary) is asked. The human is never bothered until all machine-checkable layers pass. L3 proof is intentionally excluded from the transaction hash to avoid circular dependencies.
- **Idempotent bootstrap**: if the consensus already exists, the bootstrap function skips creation, enabling safe restarts.
- **Construction-time injection**: the ConsensusService is wired into the GovernanceController at construction time, and the L2ConsensusDeliberator is wired into the MCP gateway through RuntimeDependencies. This eliminates the need for individual atomic late binding or router rebuilds.
- **Shared factory**: `NewConsensusFromPolicy` is used by both production bootstrap and test fixtures, ensuring production and test code paths exercise the same construction logic.
- **Generic policy interface**: the L4 Warden depends on `L2ConsensusPolicyStore`, not on any consensus-specific type, allowing alternative consensus implementations without warden modifications.
