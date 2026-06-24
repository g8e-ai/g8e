# Governance Postures

Last Updated: 2026-06-23
Version: v1.1.6

## Overview

The g8e gateway operates in one of three governance postures, each determining which verification layers are enforced as **fail-closed gates** versus **audited only**. The posture is set at startup via `--posture <doctrine|consensus|notary>` and cannot be changed at runtime.

The posture interface is defined in `internal/services/governance/posture.go` and queried at two enforcement points:

1. **L4 Warden** (`internal/services/governance/l4_warden.go`) — gates transaction dispatch based on L2/L3 verification results.
2. **L5 Actuator** (`internal/services/governance/l5_actuator.go`) — records L2/L3 status in the signed `ActionReceipt`.

A third enforcement point exists at startup:

3. **Gateway startup** (`cmd/operator/gateway_cmd.go`) — validates consensus posture prerequisites before any services start.

---

## Posture Definitions

### Doctrine (default)

**Configuration**: `--posture doctrine`

**Interface implementation** (`internal/services/governance/posture.go:47-52`):
- `RequiresL2Signature()` → `false`
- `RequiresL3Proof()` → `false`

**What is enforced (fail-closed, all postures)**:
- **L1 Doctrine validation** — protobuf `forbidden_patterns` field option regex checks and MITRE-based threat detection via `L1Doctrine.ValidatePayload()` (`internal/services/governance/l1_doctrine.go:50`). Any violation returns `ErrTxL1ValidationFailed` and the transaction is rejected (`l4_warden.go:466-469`).
- **Transaction hash integrity** — `envelope.id` and `envelope.transaction_hash` must both match the recomputed hash (`l4_warden.go:477-493`).
- **Nonce replay protection** — nonces are atomically reserved in SQLite before any further checks (`l4_warden.go:325-359`).
- **Expiry enforcement** — expired transactions are rejected (`l4_warden.go:334-341`).
- **State Merkle root validation** — `envelope.state_merkle_root` must match the current state root (`l4_warden.go:500-526`).
- **Action type validation** — unknown action types are rejected (`l4_warden.go:447-450`).
- **Payload decoding** — payloads must decode to the correct protobuf type for the action (`l4_warden.go:458-462`).

**What is audited but NOT gated**:
- **L2 Consensus votes** — if L2 votes are present, they are verified and the result is recorded in the `VerifiedTransaction.L2Valid` field and the `ActionReceipt.L2Status` field, but a missing or invalid L2 does **not** reject the transaction (`l4_warden.go:550-555`).
- **L3 Notary proofs** — if an L3 proof is present, it is verified and the result is recorded in `VerifiedTransaction.L3Valid` and `ActionReceipt.L3Status`, but a missing or invalid L3 does **not** reject the transaction, even for mutations (`l4_warden.go:654-659`).

**Default posture**: Doctrine is the default for gateway mode when no `--posture` flag is provided (`internal/config/config.go:398`). Outbound (operator) mode defaults to notary (`internal/config/config.go:534`).

---

### Consensus

**Configuration**: `--posture consensus`

**Interface implementation** (`internal/services/governance/posture.go:56-61`):
- `RequiresL2Signature()` → `true`
- `RequiresL3Proof()` → `false`

**What is enforced (fail-closed)**:
- Everything from doctrine posture (L1, hash, nonce, expiry, state root, action type, payload decoding).
- **L2 Consensus signature verification** — the following checks are all fail-closed gates under consensus posture:

  - **Vote presence**: The envelope must include `L2Metadata` with at least one vote. Missing votes → `ErrTxL2SignatureMissing` (`l4_warden.go:550-554`).
  - **Signer store configured**: If `signerStore` is nil → `ErrTxL2SignerStoreNotConfigured` (`l4_warden.go:560-565`).
  - **Tribunal store configured**: If `tribunalStore` is nil → `ErrTxL2TribunalNotConfigured` (`l4_warden.go:568-573`).
  - **Tribunal policy exists and is enabled**: The `TribunalPolicy` for `L2.tribunal_id` must exist and have `Enabled = true` (`l4_warden.go:576-590`).
  - **Member validation**: Votes from `signer_key_id` values not in the tribunal policy's `MemberAppIDs` are silently excluded from quorum count (`l4_warden.go:601-603`).
  - **Duplicate signer detection**: If `policy.RequireDistinct` is true, duplicate `signer_key_id` values → `ErrTxL2DuplicateSigner` (`l4_warden.go:604-609`).
  - **Signature verification**: Each vote's `consensus_signature` (Ed25519 over `<transaction_hash>|<decision>`) is verified against the trusted public key from the `SignerStore`. Invalid signatures are excluded from quorum count (`l4_warden.go:611-623`).
  - **Quorum check**: The number of affirmative (decision = true) votes from valid, distinct members must be >= `policy.Quorum`. If not → `ErrTxL2QuorumNotMet` (`l4_warden.go:630-636`).

**Startup validation**: The gateway performs additional validation at startup for consensus posture (`cmd/operator/gateway_cmd.go:119-142`):
- `tribunalID` must be non-empty → `ErrConfigTribunalIDRequired` (`internal/config/config.go:292-293`).
- The `TribunalPolicy` must exist in the database and be enabled.
- **Quorum must be >= 2** → `ErrConfigTribunalQuorumLow` (`internal/config/config.go:295-296`). This prevents single-member tribunals from being used in consensus posture.
- The Tribunal service is bootstrapped in-process and wired as both the mTLS HTTP handler and the local deliberator (`gateway_cmd.go:243-253`).

**Tribunal bootstrap** (`gateway_cmd.go:301-338`): For single-member tribunals, the gateway's actuator Ed25519 key is reused as the member signing key (Option C). Multi-member tribunals require separate key provisioning — members without private keys are constructed but cannot sign votes, and a warning is logged. Multi-member key provisioning is not yet implemented.

**What is audited but NOT gated**:
- **L3 Notary proofs** — same behavior as doctrine: verified if present, recorded in receipt, but not required for mutations (`l4_warden.go:654-659`).

---

### Notary

**Configuration**: `--posture notary`

**Interface implementation** (`internal/services/governance/posture.go:65-70`):
- `RequiresL2Signature()` → `true`
- `RequiresL3Proof()` → `true`

**What is enforced (fail-closed)**:
- Everything from doctrine posture (L1, hash, nonce, expiry, state root, action type, payload decoding).
- Everything from consensus posture (all L2 checks: vote presence, signer store, tribunal store, tribunal policy, member validation, duplicate detection, signature verification, quorum).
- **L3 Notary proof verification for mutations** — the following checks are fail-closed gates under notary posture, but **only for mutation action types**:

  - **L3 proof presence**: For mutation actions, the envelope must include `L3Metadata` with a non-nil `Proof`. Missing proof → `ErrTxL3ProofMissing` (`l4_warden.go:654-658`).
  - **L3 notary configured**: If `l3Notary` is nil and the action is a mutation → `ErrTxL3NotaryNotConfigured` (`l4_warden.go:662-666`).
  - **L3 proof validity**: The `L3Notary.VerifyL3Proof` call must return `(true, nil)`. Any failure or error for a mutation → `ErrTxL3ProofInvalid` (`l4_warden.go:678-681`).

**Mutation classification** (`internal/constants/action_types.go:88-101`): The following action types are classified as mutations and require L3 proof under notary posture:
- `A2A_CALL`
- `CANCEL`
- `EXECUTE_BASH`
- `FILE_EDIT`
- `MCP_CALL`
- `RESTORE_FILE`
- `SHUTDOWN`

Non-mutation actions (e.g., `FS_READ`, `FS_LIST`, `FETCH_LOGS`, `HEARTBEAT`) do not require L3 proof even under notary posture. This is enforced by the `isMutation(actionType)` check that gates all L3 fail-closed paths (`l4_warden.go:655, 663, 678`).

**Default for outbound mode**: Notary is the default posture for outbound (operator) mode (`internal/config/config.go:534`). This ensures that operators running in outbound mode require full L1/L2/L3 enforcement by default, since the L3Notary is nil and mutations will fail-closed with `ErrTxL3NotaryNotConfigured` unless an L3 notary is explicitly configured.

---

## Enforcement Point Summary

The posture is checked at the following code locations. Each check is a fail-closed gate when the posture requires it:

| Check | Code Location | Doctrine | Consensus | Notary |
|---|---|---|---|---|
| L1 Doctrine validation | `l4_warden.go:466-469` | **Enforced** | **Enforced** | **Enforced** |
| Transaction hash integrity (incl. L3 proof binding) | `l4_warden.go:477-493`, `pkg/governance/types.go:107-135` | **Enforced** | **Enforced** | **Enforced** |
| Nonce replay protection | `l4_warden.go:325-359` | **Enforced** | **Enforced** | **Enforced** |
| Expiry enforcement | `l4_warden.go:334-341` | **Enforced** | **Enforced** | **Enforced** |
| State Merkle root validation | `l4_warden.go:500-526` | **Enforced** | **Enforced** | **Enforced** |
| Action type validation | `l4_warden.go:447-450` | **Enforced** | **Enforced** | **Enforced** |
| L2 vote presence | `l4_warden.go:550-554` | Audited | **Enforced** | **Enforced** |
| L2 signer store configured | `l4_warden.go:560-565` | Audited | **Enforced** | **Enforced** |
| L2 tribunal store configured | `l4_warden.go:568-573` | Audited | **Enforced** | **Enforced** |
| L2 tribunal policy exists + enabled | `l4_warden.go:576-590` | Audited | **Enforced** | **Enforced** |
| L2 duplicate signer detection | `l4_warden.go:604-609` | Audited | **Enforced** | **Enforced** |
| L2 signature verification | `l4_warden.go:611-623` | Audited | **Enforced** | **Enforced** |
| L2 quorum met | `l4_warden.go:630-636` | Audited | **Enforced** | **Enforced** |
| L3 proof present (mutations only) | `l4_warden.go:654-658` | Audited | Audited | **Enforced** |
| L3 notary configured (mutations only) | `l4_warden.go:662-666` | Audited | Audited | **Enforced** |
| L3 proof valid (mutations only) | `l4_warden.go:678-681` | Audited | Audited | **Enforced** |
| L2/L3 status in receipt | `l5_actuator.go:105-123` | Recorded | Recorded | Recorded |
| Startup: tribunal ID required | `config.go:292-293` | — | **Enforced** | — |
| Startup: quorum >= 2 | `config.go:295-296` | — | **Enforced** | — |
| Startup: tribunal policy exists + enabled | `gateway_cmd.go:127-141` | — | **Enforced** | — |
| Invalid posture name → panic | `posture.go:75-80` | **Enforced** | **Enforced** | **Enforced** |

**"Enforced"** = fail-closed gate; transaction is rejected if the check fails.
**"Audited"** = result is verified if present and recorded in the receipt, but does not gate execution.
**"Recorded"** = L2/L3 status is reflected in the `ActionReceipt` as `L2_STATUS_*` / `L3_STATUS_*` enum values.

---

## L2 Consensus Verification Detail

### Tribunal Service

The `TribunalService` (`internal/services/tribunal/service.go`) is the enrolled agentic application that deliberates on governance envelopes and produces L2 consensus votes. Each member is a distinct enrolled principal with its own Ed25519 key.

**Deliberation flow** (`tribunal/service.go:78-122`):
1. Recompute the transaction hash and verify it matches `envelope.id`. Mismatch → `ErrTribunalHashMismatch`.
2. Extract command data and intent from the envelope payload.
3. Each member independently evaluates safety via `evaluateSafety()` (`tribunal/member.go:39-49`):
   - **MITRE checks**: `L1Doctrine.AnalyzeCommand()` — if any signal has `BlockRecommended = true`, the payload is unsafe (`tribunal/member.go:53-64`).
   - **Intent validation**: `L1Doctrine.ValidateIntent()` — if the intent fails validation, the payload is unsafe.
   - **Fail-closed on nil doctrine**: If doctrine is nil, `runMITREChecks` returns `false` (unsafe) (`tribunal/member.go:54-56`).
4. Each member signs `<transaction_hash>|<decision>` with Ed25519 (`tribunal/member.go:95-101`).
5. Votes are collected into `L2Metadata.Votes` with `tribunal_id` set.

**Deliberation adapters**:
- **LocalDeliberator** (`tribunal/service.go:171-196`): In-process adapter for single-binary deployments. Calls `TribunalService.Deliberate` directly without HTTP.
- **HTTPTribunalDeliberator** (`internal/services/mcp/tribunal_deliberator.go`): HTTP client for remote tribunal services. Calls `POST /tribunal/v1/deliberate` with mTLS.

### L2 Signature Verification

The L4 Warden verifies L2 votes in `verifyL2Posture` (`l4_warden.go:549-639`). The verification is identical regardless of posture — the posture only determines whether a failure rejects the transaction or is merely recorded.

**Signature format**: Ed25519 over `<transaction_hash>|<decision>` (boolean string). Verified by `verifyL2Signature` in the L4 Warden.

### Known Gap: Intent Validation

`L1Doctrine.ValidateIntent()` always returns `true` as of v1.1.8 (`l1_doctrine.go:146-148`). Sentinel allowlist integration is not yet implemented. This is a tracked security debt item, not silent behavior. The function is called during tribunal deliberation but does not currently gate any vote decision.

---

## L3 Notary Verification Detail

### L3 Notary Implementations

The `L3Notary` interface (`internal/services/governance/l3_notary.go`) is implemented by:

- **PasskeyService** (`internal/services/gateway/passkey_service.go`): WebAuthn assertion verification for web sessions. Uses `transaction_hash` as the challenge.
- **CLIL3Notary** (`internal/services/gateway/cli_l3_notary.go`): mTLS certificate fingerprint verification for CLI sessions. Checks user active status, session ownership, session expiry, and certificate revocation.
- **outboundL3Notary** (`internal/services/governance/l3_notary.go`): Operator-side approval. Verifies the transaction exists in `SuspendedTransactionStore`, is marked approved, has a valid CLI signature, and is within the 30-minute approval window.
- **CompositeL3Verifier** (`internal/services/gateway/composite_l3_verifier.go`): Delegates to `CLIL3Notary` for `mtls_cert_fingerprint` proofs, otherwise to `PasskeyService`.

### L3 and Mutations

L3 fail-closed enforcement applies **only to mutation action types** (`l4_warden.go:655, 663, 678`). The `isMutation` check (`internal/constants/action_types.go:88-101`) classifies the following as mutations:

| Action Type | Mutation |
|---|---|
| `A2A_CALL` | Yes |
| `CANCEL` | Yes |
| `EXECUTE_BASH` | Yes |
| `FILE_EDIT` | Yes |
| `MCP_CALL` | Yes |
| `RESTORE_FILE` | Yes |
| `SHUTDOWN` | Yes |
| `FS_LIST` | No |
| `FS_READ` | No |
| `FS_GREP` | No |
| `PORT_CHECK` | No |
| `FETCH_LOGS` | No |
| `FETCH_HISTORY` | No |
| `FETCH_FILE_HISTORY` | No |
| `FETCH_FILE_DIFF` | No |
| `EVAL_ANSWER` | No |
| `MCP_RESOURCE_LIST` | No |
| `MCP_RESOURCE_READ` | No |
| `MCP_PROMPT_LIST` | No |
| `MCP_PROMPT_GET` | No |
| `GRANT_INTENT` | No |
| `REVOKE_INTENT` | No |
| `HEARTBEAT` | No |
| `INVESTIGATION_CREATE` | No |

Non-mutation actions never require L3 proof, even under notary posture.

---

## Posture Selection and Defaults

| Mode | Default Posture | Configured Via |
|---|---|---|
| Gateway mode | `doctrine` | `--posture` flag; defaults to `PostureDoctrine` (`config.go:398`) |
| Outbound (operator) mode | `notary` | Defaults to `PostureNotary` (`config.go:534`) |

**Posture selection**: The `doctrine` and `consensus` postures allow mutations to execute without human authorization (L3 proof) or multi-party consensus — below the floor defined in the position paper (§12). Choosing such a posture is itself an act of human intent; the gateway logs a warning at startup and proceeds. The `--doctrine` or `--consensus` flag is the authorization.

**Invalid posture handling**: `NewGovernancePosture()` panics on unrecognized posture names (`posture.go:75-80`). This is intentional — misconfigured deployments fail at startup rather than silently running under a weaker posture. `ParseGovernancePosture()` returns an error for CLI flag validation (`posture.go:86-97`).

---

## Receipt Metadata

The L5 Actuator records posture enforcement results in every `ActionReceipt` (`l5_actuator.go:105-123`):

| Posture | L2Status | L3Status |
|---|---|---|
| Doctrine | `L2_STATUS_NOT_REQUIRED` | `L3_STATUS_NOT_REQUIRED` |
| Consensus | `L2_STATUS_REQUIRED_VALID` or `L2_STATUS_REQUIRED_FAILED` | `L3_STATUS_NOT_REQUIRED` |
| Notary | `L2_STATUS_REQUIRED_VALID` or `L2_STATUS_REQUIRED_FAILED` | `L3_STATUS_REQUIRED_VALID` or `L3_STATUS_REQUIRED_FAILED` |

These values are part of the canonical receipt JSON (`l5_actuator.go:216-229`) and are signed by the actuator's Ed25519 key. They are also persisted in the `ActionReceiptRecord` in the SQL audit store (`l5_actuator.go:293-294`).

---

## Related Documentation

- [Transaction Process](./transaction-process.md) — End-to-end flow through all five verification layers.
- [Gateway Architecture](./gateway.md) — Gateway mode, MCP endpoints, and 5-layer verification sequence.
- [Encryption](./encryption.md) — Cryptographic primitives used throughout the pipeline.
