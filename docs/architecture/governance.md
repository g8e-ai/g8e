# Governance

Last Updated: 2026-09-23
Version: v2.1.12

## Overview

The g8e system governs each operation that enters a governed platform path through a five-layer verification pipeline (L1 through L5). Governed requests flow from clients through a Gateway or direct envelope path to an executing Operator, where they undergo verification before execution in that Operator's runtime. Client-native tools, external MCP wrappers, and other side channels remain outside this pipeline; see [AI Agents and the g8e Governance Boundary](./agents.md). A configurable **GovernancePosture** determines which L2 and L3 checks are enforced as fail-closed gates versus audited only.

The posture is selected at Gateway startup via `--posture <doctrine|consensus|ratify|notary>` and cannot be changed at runtime. The CLI rejects an unrecognized posture before startup. A valid posture does not by itself make the Gateway fully ready: consensus and notary transactions can remain unavailable until their required services, policies, and signers are configured. The posture is authoritative per envelope at L4; the L5 Actuator records the resulting L2 and L3 status in the signed action receipt. See [AI Agents and the g8e Governance Boundary](./agents.md) for ingress-specific differences.

The canonical transaction container is the **GovernanceEnvelope**, a typed protobuf message that binds identity, intent, state, replay-protection material, posture, and governance proofs into a single transaction. Its deterministic transaction hash covers the envelope's executable intent, target, payload, state root, replay fields, and attribution fields; L2/L3 metadata and posture are not included so L2 can sign before human approval and the Gateway can inject the posture. L4 requires both `transaction_hash` and `id` to match the recomputed hash. See [Authentication & Authorization](./auth.md) for identity and session fields, and [Protocol](./protocol.md) for the wire format.

---

## The Five-Layer Interlock Sequence

Each governed operation crosses five responsibility layers. Universal checks and proofs required by the active posture fail closed: a failed required check prevents dispatch and releases a nonce reservation when one was made. Optional L2 and L3 results are still verified when evidence is present and are recorded as audit evidence without gating execution. The Gateway is the Policy Decision Point for client-facing construction and coordination; the executing Operator, including the Gateway's embedded Operator, independently runs L4 and L5 in its own runtime.

- **L1 Doctrine**: Hard gates via forbidden pattern matching and MITRE-based threat detection. Any violation rejects the transaction. Enforced in every posture.
- **L2 Consensus**: Protocol K-of-N authorization, not application-level model voting. Each enrolled consensus member independently evaluates the payload against L1 Doctrine and signs an Ed25519 vote over the transaction hash and the member's decision. A transaction must collect enough affirmative votes from distinct members to meet quorum. See [Consensus](./consensus.md) for enrollment, deliberation, and vote verification.
- **L3 Notary**: Human-in-the-loop authorization. In gateway mode the proof is a WebAuthn passkey assertion; in outbound operator mode the proof is an Ed25519 signature over the transaction hash from an approved suspended transaction. CLI callers additionally bind the proof to an mTLS certificate fingerprint. There is no auto-approved bypass; the L4 Warden re-derives whether L3 is required from the action type and posture and demands a real proof. See [Authentication & Authorization](./auth.md) for the notary modes and the out-of-band approval flow.
- **L4 Warden**: Pre-dispatch verification. Reserves the nonce in durable storage, checks expiry, recomputes and compares the transaction hash, validates the state Merkle root, decodes and validates the payload against L1 Doctrine, and applies posture-gated L2 and L3 verification. See [Gateway Architecture](./gateway.md) for the admission checks that run before the L4 Warden.
- **L5 Actuator**: Fail-closed receipt persistence, commitment append when the SQL commitment ledger is configured, isolated tool dispatch, and final persistence attestation. It signs and persists an `EXECUTING` receipt before invoking the handler, appends a signed commitment against the current chain head when SQL audit is enabled, rehydrates the local payload, mints a transaction-bound just-in-time capability, executes the typed action, and signs and persists the final receipt plus its persistence attestation. Receipt publication from a remote Operator to the Gateway is best effort; the executing runtime's local receipt remains authoritative. See [Operator Architecture](./operator.md) for the execution boundary and local audit vault.

---

## Governance Postures

Postures define which layers are enforced as fail-closed gates and which are audited only. When a layer is audited, verification still runs if a proof is present and the result is recorded in the receipt, but a missing or invalid proof does not reject the transaction.

| Posture | L1 Doctrine | L2 Consensus | L3 Notary | Typical Use |
|---|---|---|---|---|
| **Doctrine** | Enforced | Audited | Audited | Local development and CI |
| **Consensus** | Enforced | Enforced | Audited | Automated workflows with multi-agent review |
| **Ratify** | Enforced | Audited | Enforced (mutations only) | Human-authorized workflows without multi-agent review |
| **Notary** | Enforced | Enforced | Enforced (mutations only) | Production with multi-agent review and human authorization |

The following checks are enforced as fail-closed gates in every posture: L1 Doctrine validation, transaction hash integrity, nonce replay protection, expiry enforcement, state Merkle root validation, action type validation, and payload decoding.

### Doctrine (default)

**Configuration**: `--posture doctrine`

Doctrine is the default posture for gateway mode. L1 Doctrine is enforced. L2 consensus votes and L3 notary proofs are verified if present and recorded in the receipt, but neither is required, even for mutations. Choose this posture for local development and CI where human authorization and multi-agent review are not required.

### Consensus

**Configuration**: `--posture consensus`

Consensus enforces everything from doctrine plus L2 consensus signature verification. The envelope must include L2 metadata with votes, the signer and consensus policy stores must be configured, the consensus policy must exist and be enabled, signatures must verify against trusted public keys, and the affirmative vote count from valid distinct members must meet quorum. L3 notary proofs remain audited only.

At startup, the gateway logs advisory warnings if the consensus ID is empty or the policy is missing or disabled, then boots regardless. L2-gated transactions are rejected by the L4 Warden until a consensus is properly enrolled. See [Consensus](./consensus.md) for declarative bootstrap, runtime enrollment, and member key management.

### Ratify

**Configuration**: `--posture ratify`

Ratify enforces L1 Doctrine and L3 notary proof verification for mutation action types. L2 consensus votes are verified if present and recorded in the receipt, but they are not required and do not gate execution. A mutation must include a valid L3 proof; read-only actions do not require one.

### Notary

**Configuration**: `--posture notary`

Notary enforces everything from consensus plus L3 notary proof verification for mutation action types. A mutation must include an L3 proof, the L3 notary must be configured, and the proof must verify. Any failure for a mutation rejects the transaction. Read-only actions do not require an L3 proof even under notary posture.

In gateway mode the L3 notary is a passkey-based WebAuthn verifier. In outbound operator mode the L3 notary verifies a suspended-transaction approval and an Ed25519 signature over the transaction hash. If the notary is not configured, L3-gated mutations fail closed.

### Choosing a Posture

The doctrine and consensus postures allow mutations to execute without human authorization. Doctrine and ratify allow mutations without multi-party consensus, while ratify requires human authorization. Selecting such a posture is itself an act of human intent; the `--posture` flag is the authorization and the gateway logs the chosen posture at startup. An unrecognized posture name causes startup to fail rather than silently running under a weaker posture.

| Mode | Default Posture | Configured Via |
|---|---|---|
| Gateway mode | Doctrine | `--posture` flag; defaults to doctrine when omitted |
| Outbound (operator) mode | None | Received from the gateway during enrollment; fail-closed if missing |

---

## Transaction Flow

This section describes the practical end-to-end path of a transaction, from intent to audited result. The executing runtime owns authoritative local execution evidence; governed payloads and result data may cross authenticated component boundaries, while the platform uses scrubbing, vault services, and signed receipts to limit retained and returned data.

### 1. Principal Submits Intent

A **Principal** (a human user or AI agent) submits an intent through an MCP client such as Claude Code, Codex, Goose, or Gemini CLI, through an agentic ensemble via A2A protocols, or through a native g8e integration. The intent represents what the principal wants to accomplish, such as reading a file or running a command.

### 2. Producer Wraps the Intent

The **Producer** wraps the intent in a GovernanceEnvelope carrying the typed payload, principal identity, nonce, state root, and governance proofs. Under `consensus` and `notary` postures, the envelope must include the required L2 consensus votes before the L4 Warden will admit it. Clients may obtain those votes through the gateway's deliberation endpoint or provide them along with the envelope. Under `doctrine` and `ratify` postures, L2 votes are not required.

For Gateway MCP and A2A calls under `ratify` and `notary`, the Gateway can suspend a transaction when L3 proof is missing or invalid, persist the pending approval, return an approval URL, and retry processing after the human approves through the WebAuthn flow. Direct envelope submission and the Operator `CommandIntent` relay do not synthesize or suspend for missing L3 proof; those paths must supply evidence that satisfies the active posture, and Gateway CLI dispatch rejects mutation construction when it cannot mint the proof. See [AI Agents and the g8e Governance Boundary](./agents.md) and [Gateway Architecture](./gateway.md) for path-specific behavior.

### 3. Gateway Admits the Envelope

The **Governance Gateway** acts as the Policy Decision Point and the system's PKI authority. It authenticates each route according to its configured transport mode, checks certificate and session requirements where applicable, binds mTLS transport identity to envelope identity claims on the direct-envelope route, and applies route-specific limits. Envelopes that pass admission are processed synchronously by the configured Gateway or Operator path. Malformed input and pre-verification failures can return a typed error without a receipt; failures that reach the Actuator produce signed rejection or execution evidence when the Actuator's audit dependencies are available.

### 4. Operator Retrieves the Envelope

A **Governed Operator** on a sovereign runtime establishes an outbound-only mTLS WebSocket connection to the Gateway and subscribes to its exact `cmd:<operator-id>:<operator-session-id>` channel. The Gateway publishes assigned envelopes to that session; it cannot open an inbound connection or broadcast work to the Operator. In synchronous Gateway mode, the embedded Operator handles the envelope directly without a tunnel.

### 5. Warden Verifies (L4)

The **L4 Warden** performs ordered pre-dispatch checks: it tracks the nonce in process, reserves it durably for replay protection after checking required nonce and expiry fields, performs stateless validation (known action type, typed payload decoding, L1 Doctrine, and transaction hash), verifies the current state Merkle root, reads posture from the envelope, and verifies posture-required L2 and L3 evidence. A failed check after reservation releases that reservation; a transaction is not dispatched to the handler.

### 6. Actuator Executes (L5)

The **L5 Actuator** receives L4 deterministic evidence bound to the transaction, identities, state, doctrine bundle, L2/L3 signature digests, timing, and parent stages. It signs and persists an `EXECUTING` receipt before any handler side effect. When the SQL audit store is configured, it appends a signed `CommitmentAttestation` against the current chain head and records the commitment and prior-commitment hashes in receipt evidence; signing, initial persistence, or commitment failure stops execution.

The actuator then rehydrates the sovereignty-scrubbed payload, mints a just-in-time capability scoped to the transaction, dispatches the action, and dissolves the capability immediately afterward. It adds L5 outcome evidence, state transitions, and L2/L3 status; signs and persists the final `COMPLETED` or `FAILED` receipt; then attaches a signed `ReceiptPersistenceAttestation` binding the final receipt-signature digest, audit record ID, signer key, and durable timestamp. Failure to persist the final receipt or attestation is returned as an execution failure rather than silently accepting incomplete evidence.

### 7. Audit Vault Records

The executing runtime writes the complete `ActionReceipt` to its local SQLite audit store and, when enabled, the signed `CommitmentAttestation` to its SQLite hash-chained commitment ledger. Governed file mutations can additionally create snapshots in the optional git-backed ledger. The Gateway may mirror a verified remote receipt, but that mirror is best effort and does not replace the executing Operator's local evidence. Execution payloads cross the governed transport to the target runtime; local scrubbing and vault services limit what is retained or returned. Verification failures that occur before Actuator processing may have no receipt. See [Operator Architecture](./operator.md) for the audit store and git ledger.

### 8. Receipt Returns to the Principal

The executing Operator returns the signed receipt or result envelope to the Gateway over the authenticated outbound channel. In synchronous Gateway mode the embedded Operator returns the receipt directly to the HTTP or MCP/A2A handler; in outbound mode the receipt is published on the session's result or receipt channel. Admitted execution failures still return signed failure evidence. Client-facing handlers expose only the response shape owned by that ingress: for example, MCP returns tool content plus a signed receipt reference, while the direct envelope route returns the canonical protojson `ActionReceipt`.

---

## Security Properties

### Fail-Closed Design

Universal checks and posture-required proofs fail closed. A failed required check rejects the transaction immediately and releases the nonce reservation, while optional L2 and L3 results remain audit evidence. The Actuator will not execute a mutation if it fails to sign or log the initial receipt. The posture factory rejects unrecognized posture names at startup so misconfigured deployments fail rather than silently running under a weaker posture.

### Sovereignty

The executing runtime owns authoritative local execution evidence. Operators initiate outbound-only connections to the Gateway, so the Gateway cannot reach into an Operator runtime. Governed payloads and results can cross the authenticated channel as required by the operation; scrubbing and vault services rehydrate protected values only at the execution boundary, and the receipt path returns scrubbed or summary evidence rather than assuming all raw data is absent from transport.

### Cryptographic Integrity

L2 consensus votes are Ed25519 signatures from enrolled members, produced over the transaction hash and the member's decision, and verified against the trusted signer store and configured consensus policy. Every receipt is signed by the L5 Actuator over its canonical fields and deterministic stage evidence. The final persistence attestation independently signs the final receipt-signature digest, audit record ID, signer key, and durable timestamp. Signed commitments form an insertion-ordered hash chain and bind receipt evidence to the prior chain head. Selected event content and execution output use vault encryption; SQLite metadata and complete databases are not uniformly encrypted, and file mutations may additionally use encrypted storage. mTLS protects the HTTPS port where route policy requires it, and transport-to-envelope identity binding prevents impersonation by matching certificate SPIFFE URI SANs to envelope identity claims. See [Encryption](./encryption.md) for the key hierarchy and cryptographic primitives.

The trusted signer lookup endpoint returns enabled keystore-backed signer material when a metadata document is absent, so consensus vote verification does not fail when a signer has not published a separate metadata document. The gateway receipt relay verifies both the canonical receipt signature and the final persistence attestation before accepting a relayed receipt, so a receipt with a valid signature but a missing or invalid persistence attestation is rejected.

### Defense in Depth

The five layers interlock so that each layer assumes the prior layer may be compromised. L1 provides technical bedrock validation, L2 adds multi-signature consensus, L3 adds human authorization, L4 adds replay and state-root protection plus posture gating, and L5 adds a fail-closed signed execution boundary with zero standing privileges.

### Accountability

Every admitted transaction carries a deterministic transaction hash. Governance and execution failures use typed errors or typed receipt failure codes; failures before Actuator processing may not produce a receipt. Transport identity binding and, where required, L3 proof verification establish the relevant identity and authorization checks. L2 and L3 verification status is reflected in receipts that reach the Actuator.

---

## Related Documentation

- [Gateway Architecture](./gateway.md): Gateway mode, MCP endpoints, admission control, and the 5-layer verification sequence.
- [Operator Architecture](./operator.md): Operator-side verification pipeline, execution boundary, and local audit vault.
- [Authentication & Authorization](./auth.md): mTLS identity binding, passkey enrollment, session management, and L3 notary modes.
- [Consensus](./consensus.md): Consensus policy, enrollment, deliberation, and L2 vote verification.
- [Encryption](./encryption.md): Cryptographic primitives, key hierarchy, and TLS configuration.
- [Protocol](./protocol.md): GovernanceEnvelope wire format and message types.
