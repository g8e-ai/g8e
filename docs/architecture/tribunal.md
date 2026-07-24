# Tribunal

Last Updated: 2026-07-23
Version: v1.6.2

## Overview

L2 Consensus is a protocol concept defined in the g8e protocol (`L2Metadata`, `L2Vote`). The Tribunal is the **reference implementation** of the L2 Consensus layer shipped with g8e — an enrolled body of agentic applications that independently evaluate each transaction's payload against the L1 Doctrine and produce signed Ed25519 votes. A transaction must collect enough affirmative votes (quorum) from distinct tribunal members to pass L2 verification. Alternative implementations can be built against the same protocol interfaces.

The Tribunal operates as a **fail-closed gate** under the `consensus` and `notary` postures. Under `doctrine` posture, L2 votes are verified if present and recorded in the receipt, but do not gate execution.

---

## TribunalPolicy: Definition and Storage

### Data Model

The `TribunalPolicy` struct (`internal/models/auth.go:521`) defines a named consensus body:

| Field | Type | Description |
|---|---|---|
| `ID` | `string` | Tribunal name/identifier (alphanumeric, hyphens, underscores only) |
| `MemberAppIDs` | `[]string` | List of member app IDs; each must resolve to an enabled `TrustedSigner` |
| `Quorum` | `int` | Minimum number of affirmative distinct signatures required (K of N) |
| `RequireDistinct` | `bool` | If true, duplicate signer key IDs in a vote set are rejected |
| `Enabled` | `bool` | Whether this tribunal is active |
| `CreatedAt` | `time.Time` | Creation timestamp (auto-set) |
| `UpdatedAt` | `time.Time` | Last update timestamp (auto-set) |

### Storage Layer

Tribunal policies are persisted in the SQLite document store under the `tribunals` collection (`internal/constants/collections.go:46`). The `TribunalStoreService` (`internal/services/gateway/tribunal_store_service.go`) provides CRUD operations:

- **`GetTribunal(id)`** — Retrieves a policy by ID; returns `nil, nil` if not found.
- **`AddTribunal(policy)`** — Creates or updates a policy with fail-closed validation.
- **`ListTribunals()`** — Returns all stored policies.
- **`DeleteTribunal(id)`** — Removes a policy.

The `TribunalStore` interface (`internal/services/governance/tribunal_store.go:19`) is the Tribunal-specific read interface. The L4 Warden now depends on the generic `L2ConsensusPolicyStore` interface, which `TribunalStoreService` satisfies via `GetConsensusPolicy` (adapting `TribunalPolicy` → `L2ConsensusPolicy`). External `TribunalStore` implementations can be adapted via `TribunalStoreAdapter`:

```go
type TribunalStore interface {
    GetTribunal(id string) (*models.TribunalPolicy, error)
}
```

### AddTribunal Validation

All validation is fail-closed at write time (`internal/services/gateway/tribunal_store_service.go:80`):

- **Tribunal ID**: non-empty, alphanumeric + hyphens + underscores only
- **Member list**: non-empty, no empty strings, no duplicates
- **Quorum**: `>= 1` and `<= len(MemberAppIDs)`
- **Trusted signer check**: every `MemberAppID` must resolve to an enabled `TrustedSigner` in the `SignerStoreService`
- **Existence check**:
  - New tribunals must be created with `Enabled=true` (rejects `Enabled=false` for non-existent IDs)
  - Existing tribunals may only be updated via `Enabled=false` (disable path)
  - Overwriting an existing tribunal with `Enabled=true` is rejected as a duplicate

---

## Tribunal Enrollment

### Declarative Bootstrap (File-Based)

Tribunals can be seeded at gateway startup via the `--tribunal-bootstrap <path>` flag (or `G8E_TRIBUNAL_BOOTSTRAP` env var). The config file is a JSON document:

```json
{
  "tribunal_id": "dhs-tribunal",
  "member_app_ids": ["auditor-ensemble"],
  "quorum": 1,
  "seed_hex": "87278693f5894d8de5d28401c923e0c3fea9ae7c35f467065954eecbc85b2e77"
}
```

| Field | Required | Description |
|---|---|---|
| `tribunal_id` | Yes | The tribunal ID to create |
| `member_app_ids` | Yes | List of member app IDs to enroll |
| `quorum` | Yes | Quorum threshold (must be `>= 1`) |
| `seed_hex` | No | Hex-encoded Ed25519 seed for deterministic key generation. If omitted, a fresh key pair is generated. |

#### Bootstrap Process

The `bootstrapTribunalPolicy` function (`internal/cli/serve/gateway.go:416`) executes the following steps:

1. **Read and parse** the JSON config file (`tribunal-bootstrap.json`)
2. **Idempotency check**: if the tribunal already exists, skip bootstrap
3. **Key derivation**: if `seed_hex` is provided, derive the Ed25519 key pair from the seed; otherwise generate a fresh key pair
4. **Trusted signer registration**: for each `member_app_id`, register the derived public key as a `TrustedSigner` (single-key ensemble pattern for demos)
5. **Member key persistence**: save each member's private key to disk via `tribunal.SaveMemberKey` so the in-process `LocalDeliberator` can sign L2 votes via `FileKeyProvider`
6. **TribunalPolicy creation**: insert the policy into the `TribunalStore` with `Enabled=true` and `RequireDistinct=true`

### Admin API (Runtime Enrollment)

Tribunals can also be created at runtime via the admin REST API (`internal/services/gateway/admin_controller.go:203`):

- **`POST /api/v1/admin/tribunals`** — Create a new tribunal policy. Requires a bootstrap user (admin-only). Accepts a `TribunalPolicy` JSON body. Returns `201 Created` or `400 Bad Request` on validation failure.
- **`GET /api/v1/admin/tribunals`** — List all tribunal policies. Requires a bootstrap user. Returns `200 OK` with a JSON array.
- **`DELETE /api/v1/admin/tribunals/{id}`** — Delete a tribunal policy. Requires a bootstrap user. Returns `200 OK` or `404 Not Found`.

### Member Key Management

Each tribunal member has its own Ed25519 private key, stored on disk as a hex-encoded seed. The `FileKeyProvider` (`internal/services/tribunal/factory.go:89`) loads keys using the naming convention:

```
{secrets_dir}/tribunal_member_{tribunalID}_{memberAppID}.key
```

The `SaveMemberKey` function (`internal/services/tribunal/factory.go:134`) writes keys with `0600` permissions to a `0700` secrets directory.

The `KeyProvider` interface (`internal/services/tribunal/factory.go:34`) abstracts key resolution:

```go
type KeyProvider interface {
    GetMemberKey(appID string) (ed25519.PrivateKey, error)
}
```

In production bootstrap (`BootstrapTribunal`), the key provider tries `FileKeyProvider` first, then falls back to the gateway's actuator signing key if the member's AppID matches the actuator's key ID. Members whose keys cannot be resolved are included in the policy without a private key — they can participate in policy but cannot sign votes, and a warning is logged.

---

## Tribunal Service

### Construction

The `TribunalService` (`internal/services/tribunal/service.go:38`) is the enrolled agentic application that deliberates on governance envelopes and produces L2 consensus votes.

```
TribunalService
├── tribunalID: string          // TribunalPolicy.ID
├── members: []TribunalMember   // Each has AppID + Ed25519 PrivateKey
├── doctrine: *L1Doctrine       // Deterministic evaluation engine
├── logger: *slog.Logger
└── responder: *response.Writer
```

The shared factory `NewTribunalFromPolicy` (`internal/services/tribunal/factory.go:54`) constructs a `TribunalService` from a `TribunalPolicy` and a `KeyProvider`. It resolves each member's private key via the provider and builds the member list. This factory is used by both production bootstrap (`BootstrapTribunal` in `internal/cli/serve/gateway.go`) and test fixtures (`SetupTribunal` in `test/fixtures/gateway_fixture.go`).

### TribunalMember

Each member (`internal/services/tribunal/member.go:29`) is an enrolled agentic app with its own Ed25519 signing key:

```go
type TribunalMember struct {
    AppID      string
    PrivateKey ed25519.PrivateKey
}
```

The member's public key is registered as a `TrustedSigner` (keyID = AppID). Members never share the gateway identity key — even in single-binary deployments, each member has a distinct key.

---

## Deliberation: How GovernanceEnvelopes Are Delivered and Processed

### Deliberation Flow

When a transaction requires L2 consensus, the `GovernanceEnvelope` is sent to the Tribunal for deliberation. The Tribunal processes the envelope through all members and returns it with L2 metadata populated (tribunal ID + signed votes).

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
│  TribunalService.Deliberate()    │
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

### Deliberate Method

The `Deliberate` method (`internal/services/tribunal/service.go:78`) performs the following:

1. **Hash verification**: recomputes the envelope's message ID via `governance.GenerateMessageID` and compares it to `env.Id`. If they don't match, returns `ErrTribunalHashMismatch` (fail-closed).

2. **Command data extraction**: extracts command data and intent from the envelope's `IntentData` (if present) or `Payload` (`internal/services/tribunal/member.go:60`).

3. **Per-member evaluation**: for each member with a non-nil `PrivateKey`:
   - **Safety evaluation**: runs MITRE checks via `L1Doctrine.AnalyzeCommand`. If any signal has `BlockRecommended=true`, the payload is deemed unsafe (`isSafe=false`). If doctrine is nil, the payload is **fail-closed** (not safe).
   - **Vote signing**: signs the string `"<transaction_hash>|<decision>"` with the member's Ed25519 private key (`internal/services/tribunal/member.go:81`). The signature is hex-encoded.

4. **L2 metadata population**: sets `env.Governance.L2.TribunalId` to the tribunal's ID and `env.Governance.L2.Votes` to the collected vote list.

5. **Return**: the envelope with L2 metadata populated, ready for submission to the gateway's L4 Warden.

### L2Vote Proto Structure

Defined in `protocol/proto/g8e/common/v1/common.proto:48`:

```protobuf
message L2Vote {
  string signer_key_id       = 1; // member appID == TrustedSigner.ID
  string consensus_signature = 2; // ed25519 over "<transaction_hash>|<decision>"
  bool   decision            = 3; // member's safe (true) / unsafe (false) vote
}

message L2Metadata {
  string tribunal_id    = 1; // TribunalPolicy.id that produced this set
  repeated L2Vote votes = 2; // independent member votes
}
```

### Delivery Mechanisms

#### 1. LocalDeliberator (In-Process, Single-Binary Deployment)

The `LocalDeliberator` (`internal/services/tribunal/service.go:179`) is an in-process adapter that satisfies the `mcp.L2ConsensusDeliberator` interface by calling `TribunalService.Deliberate` directly, without an HTTP round-trip:

```go
func (d *LocalDeliberator) Deliberate(_ context.Context, envelopeBytes []byte) ([]byte, error)
```

It unmarshals the envelope bytes, runs deliberation, and returns the marshaled envelope with L2 votes populated. This is the default mode for single-binary gateway deployments.

#### 2. HTTP Endpoint (Remote Deliberation)

The Tribunal also exposes an mTLS-guarded HTTP endpoint for remote deliberation calls:

- **Route**: `POST /tribunal/v1/deliberate` (`internal/constants/api_paths.go:241`)
- **Handler**: `TribunalService.HandleDeliberate` (`internal/services/tribunal/service.go:131`)
- **Wire format**: canonical protojson `GovernanceEnvelope` (1 MiB max body)
- **Response**: the envelope with L2 votes populated, as protojson

The route is **always registered** on the mTLS mux. The handler loads the `TribunalService` via an atomic pointer — if the tribunal is not yet wired, it returns `503 Service Unavailable` (`internal/services/gateway/governance_controller.go:88`).

#### 3. MCP Gateway Integration

Under `consensus` and `notary` postures, the MCP gateway's `processGatewayTransaction` (`internal/services/mcp/gateway.go:731`) automatically sends the envelope to the Tribunal for L2 deliberation before dispatch:

```go
if (g.posture == "consensus" || g.posture == "notary") && g.getL2ConsensusDeliberator() != nil {
    deliberatedBytes, err := g.getL2ConsensusDeliberator().Deliberate(ctx, envelopeBytes)
    // ...
    envelopeBytes = deliberatedBytes
}
```

If the deliberator is not configured, the envelope proceeds without L2 votes and will fail-closed at L4 verification under consensus/notary postures.

The `L2ConsensusDeliberator` interface (`internal/services/mcp/gateway.go:81`) is wired via `SetL2ConsensusDeliberator` after tribunal bootstrap, using `atomic.Value` for thread-safe late binding.

---

## L4 Warden: L2 Vote Verification

### Verification Flow

When a `GovernanceEnvelope` arrives at the gateway, the L4 Warden's `verifyL2Posture` (`internal/services/governance/l4_warden.go:351`) validates the L2 votes:

1. **Vote presence**: if `envelope.Governance.L2` is nil or has zero votes:
   - Under `consensus`/`notary` posture: reject with `ErrTxL2SignatureMissing`
   - Under `doctrine` posture: return `false, nil` (audited, not enforced)

2. **Store checks**: verifies `signerStore` and `consensusPolicyStore` are configured. Under enforced postures, missing stores are fail-closed.

3. **Consensus policy lookup**: loads the consensus policy by `L2.TribunalId`. Under enforced postures, a missing or disabled policy is rejected with `ErrTxL2ConsensusNotConfigured`.

4. **Member validation**: votes from `SignerKeyId` values not in the policy's `MemberAppIDs` are silently excluded from the quorum count.

5. **Duplicate signer detection**: if `RequireDistinct=true`, duplicate `SignerKeyId` values in the vote set are rejected with `ErrTxL2DuplicateSigner`.

6. **Signature verification**: for each vote, the Ed25519 signature over `"<transaction_hash>|<decision>"` is verified against the trusted public key loaded from `SignerStore.GetTrustedSigner`. Invalid signatures are excluded from the quorum count (not rejected — the vote simply doesn't count).

7. **Quorum check**: the count of affirmative votes from valid, distinct members must meet or exceed `policy.Quorum`. Under enforced postures, failure returns `ErrTxL2QuorumNotMet`.

### Signature Format

The L2 signature payload is the string `"<transaction_hash>|<decision>"` where `decision` is the Go `%v` format of a `bool` (`true` or `false`). The signature is Ed25519, hex-encoded. Verification (`internal/services/governance/l4_warden.go:548`):

```go
payload := fmt.Sprintf("%s|%v", messageID, decision)
return ed25519.Verify(pubKey, []byte(payload), sigBytes)
```

### Posture-Dependent Enforcement

| Check | Doctrine | Consensus | Notary |
|---|---|---|---|
| L2 vote presence | Audited | **Enforced** | **Enforced** |
| Signer store configured | Audited | **Enforced** | **Enforced** |
| Tribunal store configured | Audited | **Enforced** | **Enforced** |
| Tribunal policy exists + enabled | Audited | **Enforced** | **Enforced** |
| Member validation | Audited | **Enforced** | **Enforced** |
| Duplicate signer detection | Audited | **Enforced** | **Enforced** |
| Signature verification | Audited | **Enforced** | **Enforced** |
| Quorum met | Audited | **Enforced** | **Enforced** |

Under `doctrine` posture, all L2 checks return `false, nil` when stores/policies are missing — the result is recorded in the receipt but does not gate execution.

---

## Gateway Bootstrap Sequence

The full tribunal bootstrap sequence in `internal/cli/serve/gateway.go`:

1. **`--tribunal-bootstrap` flag**: if set, `bootstrapTribunalPolicy` seeds trusted signers and the `TribunalPolicy` from the JSON config file before L2 posture validation runs.

2. **L2 posture advisory check**: for `consensus`/`notary` postures, logs warnings if:
   - `--tribunal-id` is empty
   - The tribunal policy is not found or disabled
   - If the policy exists and is enabled, logs the tribunal ID, member count, and quorum

3. **Tribunal service bootstrap**: for `consensus`/`notary` postures with a non-empty `--tribunal-id`:
   - `BootstrapTribunal` loads the `TribunalPolicy` from the database
   - Constructs a `FileKeyProvider` for disk-based member keys
   - Creates a composite `KeyProvider` that tries file keys first, then falls back to the actuator key
   - Builds the `TribunalService` via `NewTribunalFromPolicy`
   - Wires it into the gateway via `svc.SetTribunal(tribunalSvc)`
   - Wires the `LocalDeliberator` into the MCP gateway via `mcpSvc.SetL2ConsensusDeliberator`

4. **Under `doctrine` posture**: the Tribunal is not constructed.

### Thread-Safe Late Binding

The `GovernanceController` uses `atomic.Pointer[tribunal.TribunalService]` for the tribunal reference (`internal/services/gateway/governance_controller.go:49`). The tribunal deliberate route is always registered on the mTLS mux — no router rebuild is needed when `SetTribunal` is called later in the boot sequence. The handler checks the atomic pointer at request time and returns `503` if not yet wired.

Similarly, the MCP gateway uses `atomic.Value` for the `L2ConsensusDeliberator` (`internal/services/mcp/gateway.go:116`), enabling thread-safe late binding after tribunal bootstrap.

---

## End-to-End Transaction Flow with Tribunal

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
   │       └── TribunalService.Deliberate()
   │           ├── Verify env.Id == recomputed hash
   │           ├── Extract command data
   │           ├── For each member with PrivateKey:
   │           │   ├── Evaluate safety via L1Doctrine (MITRE checks)
   │           │   └── Sign "<hash>|<decision>" with Ed25519
   │           ├── Set L2.TribunalId + L2.Votes
   │           └── Return envelope with L2 metadata
   │
   └── Return (hash, envelopeBytes)
       │
       ▼
3. L4 Warden VerifyEnvelope()
   ├── Stateless: hash, action type, payload, L1 doctrine
   ├── Stateful: nonce, expiry, state root
   └── Posture: verifyL2Posture()
       ├── Load TribunalPolicy by L2.TribunalId
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

- **Fail-closed by default**: if doctrine is nil, the payload is NOT safe. If tribunal stores are missing under enforced postures, transactions are rejected.
- **No key sharing**: each tribunal member has its own Ed25519 key, distinct from the gateway identity key, even in single-binary deployments.
- **Protocol ordering**: L2 (machine consensus) signs the transaction hash before L3 (human notary) is asked. The human is never bothered until all machine-checkable layers pass. L3 proof is intentionally excluded from the transaction hash to avoid circular dependencies.
- **Idempotent bootstrap**: if the tribunal already exists, `bootstrapTribunalPolicy` skips creation, enabling safe re-starts.
- **Atomic late binding**: the tribunal service and deliberator are wired after gateway construction via atomic pointers, eliminating router rebuilds.
- **Shared factory**: `NewTribunalFromPolicy` is used by both production bootstrap and test fixtures, ensuring production and test code paths exercise the same construction logic.

---

## File Reference

| Component | File |
|---|---|
| TribunalService | `internal/services/tribunal/service.go` |
| TribunalMember, safety eval, signing | `internal/services/tribunal/member.go` |
| Factory, KeyProvider, FileKeyProvider | `internal/services/tribunal/factory.go` |
| TribunalStoreService (CRUD) | `internal/services/gateway/tribunal_store_service.go` |
| TribunalStore interface | `internal/services/governance/tribunal_store.go` |
| TribunalPolicy model | `internal/models/auth.go:521` |
| L4 Warden L2 verification | `internal/services/governance/l4_warden.go:351` |
| GovernancePosture (doctrine/consensus/notary) | `internal/services/governance/posture.go` |
| GovernanceController (HTTP handlers) | `internal/services/gateway/governance_controller.go` |
| Admin API (tribunal CRUD) | `internal/services/gateway/admin_controller.go:203` |
| Bootstrap (CLI serve) | `internal/cli/serve/gateway.go:416` |
| MCP Gateway deliberation | `internal/services/mcp/gateway.go:731` |
| L2Vote / L2Metadata proto | `protocol/proto/g8e/common/v1/common.proto:48` |
| GovernanceEnvelope hash | `internal/governance/envelope.go:42` |
| Test fixture (SetupTribunal) | `test/fixtures/gateway_fixture.go:654` |
| Error sentinels | `internal/constants/errors.go:872` |
| API paths | `internal/constants/api_paths.go:237` |
| Collections | `internal/constants/collections.go:46` |
| Env vars | `internal/constants/env_vars.go:37` |
