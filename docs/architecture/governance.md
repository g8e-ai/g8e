# Governance

Last Updated: 2026-08-16
Version: v1.7.5

## Overview

The g8e system governs every transaction through a five-layer verification pipeline (L1 through L5). Transactions flow from AI clients through a governance gateway to governed operators, where they undergo verification before execution on target systems. A configurable **GovernancePosture** determines which layers are enforced as fail-closed gates versus audited only.

The posture is set at startup via `--posture <doctrine|consensus|notary>` and cannot be changed at runtime. The gateway boots regardless of posture; layer enforcement happens at transaction time. The posture is consulted at two points: the L4 Warden gates transaction dispatch based on L2 and L3 results, and the L5 Actuator records L2 and L3 status in the signed action receipt.

The canonical transaction container is the **GovernanceEnvelope**, a typed protobuf message that binds identity, intent, state, replay-protection material, and governance proofs into a single transaction. See [Authentication & Authorization](./auth.md) for the identity and session fields, and [Protocol](./protocol.md) for the wire format.

---

## The Five-Layer Interlock Sequence

Each transaction passes through five layers in order. Every layer fails closed: a failed check rejects the transaction and releases its nonce reservation. The Gateway acts as the Policy Decision Point for L1 through L3; the Operator substrate acts as the Policy Execution Point for L4 and L5.

- **L1 Doctrine**: Hard gates via forbidden pattern matching and MITRE-based threat detection. Any violation rejects the transaction. Enforced in every posture.
- **L2 Consensus**: Multi-agent consensus signature verification. Each consensus member independently evaluates the payload against L1 Doctrine and signs an Ed25519 vote over the transaction hash and the member's decision. A transaction must collect enough affirmative votes from distinct members to meet quorum. See [Consensus](./consensus.md) for enrollment, deliberation, and vote verification.
- **L3 Notary**: Human-in-the-loop authorization. In gateway mode the proof is a WebAuthn passkey assertion; in outbound operator mode the proof is an Ed25519 signature over the transaction hash from an approved suspended transaction. CLI callers additionally bind the proof to an mTLS certificate fingerprint. There is no auto-approved bypass; the Warden re-derives whether L3 is required from the action type and posture and demands a real proof. See [Authentication & Authorization](./auth.md) for the notary modes and the out-of-band approval flow.
- **L4 Warden**: Pre-dispatch verification. Reserves the nonce in durable storage, checks expiry, recomputes and compares the transaction hash, validates the state Merkle root, decodes and validates the payload against L1 Doctrine, and applies posture-gated L2 and L3 verification. See [Gateway Architecture](./gateway.md) for the admission checks that run before the Warden.
- **L5 Actuator**: Isolated tool dispatch and signed receipt production. Mints a just-in-time, single-action capability scoped to the transaction, executes the action, dissolves the capability, and signs a canonical receipt with the operator's Ed25519 key. See [Operator Architecture](./operator.md) for the execution boundary and local audit vault.

---

## Governance Postures

Postures define which layers are enforced as fail-closed gates and which are audited only. When a layer is audited, verification still runs if a proof is present and the result is recorded in the receipt, but a missing or invalid proof does not reject the transaction.

| Posture | L1 Doctrine | L2 Consensus | L3 Notary | Typical Use |
|---|---|---|---|---|
| **Doctrine** | Enforced | Audited | Audited | Local development and CI |
| **Consensus** | Enforced | Enforced | Audited | Automated workflows with multi-agent review |
| **Notary** | Enforced | Enforced | Enforced (mutations only) | Production with human authorization |

The following checks are enforced as fail-closed gates in every posture: L1 Doctrine validation, transaction hash integrity, nonce replay protection, expiry enforcement, state Merkle root validation, action type validation, and payload decoding.

### Doctrine (default)

**Configuration**: `--posture doctrine`

Doctrine is the default posture for gateway mode. L1 Doctrine is enforced. L2 consensus votes and L3 notary proofs are verified if present and recorded in the receipt, but neither is required, even for mutations. Choose this posture for local development and CI where human authorization and multi-agent review are not required.

### Consensus

**Configuration**: `--posture consensus`

Consensus enforces everything from doctrine plus L2 consensus signature verification. The envelope must include L2 metadata with votes, the signer and consensus policy stores must be configured, the consensus policy must exist and be enabled, signatures must verify against trusted public keys, and the affirmative vote count from valid distinct members must meet quorum. L3 notary proofs remain audited only.

At startup, the gateway logs advisory warnings if the consensus ID is empty or the policy is missing or disabled, then boots regardless. L2-gated transactions are rejected by the L4 Warden until a consensus is properly enrolled. See [Consensus](./consensus.md) for declarative bootstrap, runtime enrollment, and member key management.

### Notary

**Configuration**: `--posture notary`

Notary enforces everything from consensus plus L3 notary proof verification for mutation action types. A mutation must include an L3 proof, the L3 notary must be configured, and the proof must verify. Any failure for a mutation rejects the transaction. Read-only actions do not require an L3 proof even under notary posture.

In gateway mode the L3 notary is a passkey-based WebAuthn verifier. In outbound operator mode the L3 notary verifies a suspended-transaction approval and an Ed25519 signature over the transaction hash. If the notary is not configured, L3-gated mutations fail closed.

### Choosing a Posture

The doctrine and consensus postures allow mutations to execute without human authorization or, for doctrine, without multi-party consensus. Selecting such a posture is itself an act of human intent; the `--posture` flag is the authorization and the gateway logs the chosen posture at startup. An unrecognized posture name causes startup to fail rather than silently running under a weaker posture.

| Mode | Default Posture | Configured Via |
|---|---|---|
| Gateway mode | Doctrine | `--posture` flag; defaults to doctrine when omitted |
| Outbound (operator) mode | None | Received from the gateway during enrollment; fail-closed if missing |

---

## Transaction Flow

This section describes the practical end-to-end path of a transaction, from intent to audited result. The flow is designed to keep raw data and audit logs on the sovereign host while returning only scrubbed, signed evidence to the caller.

### 1. Principal Submits Intent

A **Principal** (a human user or AI agent) submits an intent through an MCP client such as Claude Code, Codex, Goose, or Gemini CLI, through an agentic ensemble via A2A protocols, or through a native g8e integration. The intent represents what the principal wants to accomplish, such as reading a file or running a command.

### 2. Producer Wraps the Intent

The **Producer** wraps the intent in a GovernanceEnvelope carrying the typed payload, principal identity, nonce, state root, and governance proofs. Under `consensus` and `notary` postures, the envelope must include the required L2 consensus votes before the Warden will admit it. Clients may obtain those votes through the gateway's deliberation endpoint or provide them along with the envelope. Under `doctrine` posture, L2 votes are not required.

If the principal cannot produce an L3 notary proof, the gateway suspends the transaction, sends an out-of-band WebAuthn challenge URL to the client, and resumes the L4 and L5 flow after the human approves via browser. See [Gateway Architecture](./gateway.md) for the suspension and approval flow.

### 3. Gateway Admits the Envelope

The **Governance Gateway** acts as the Policy Decision Point and the system's PKI authority. It enforces mTLS at the application layer for all non-public routes, checks certificate revocation, binds the transport identity to the envelope identity claims to prevent impersonation, and rate-limits the submission endpoint. Envelopes that pass admission are queued for processing; failures are rejected immediately with a typed error and audit entry.

### 4. Operator Retrieves the Envelope

A **Governed Operator** on a sovereign host establishes an outbound-only mTLS tunnel to the gateway and pulls its assigned envelopes. The operator initiates the connection; the gateway cannot reach into the operator. This keeps operators sovereign: they pull work but cannot be pushed into. In synchronous gateway mode, the in-process operator handles the envelope directly without a tunnel.

### 5. Warden Verifies (L4)

The **L4 Warden** runs the five-stage verification sequence: in-flight tracking to prevent concurrent processing of the same nonce, durable nonce reservation and expiry checks, stateless validation covering structural checks and L1 Doctrine, stateful validation of the state Merkle root, and posture-gated L2 and L3 verification. Any failed stage rejects the transaction and releases the nonce reservation.

### 6. Actuator Executes (L5)

The **L5 Actuator** signs an initial receipt with an `EXECUTING` status and logs it before starting work; if signing or logging fails, execution does not proceed. It rehydrates any sovereignty-scrubbed payload, mints a just-in-time capability scoped to the transaction, dispatches the action, and dissolves the capability immediately after execution. Execution handlers scrub sensitive host data from results before returning them. A final receipt captures the results, state root transitions, and L2 and L3 status, signed with the operator's Ed25519 key.

### 7. Audit Vault Records

The operator anchors the transaction to the **Local Audit Vault** on the sovereign host. Audit data stays on the host; raw data never leaves. Each entry is signed and chained, and even failed transactions are logged for complete transparency. See [Operator Architecture](./operator.md) for the audit store and git ledger.

### 8. Receipt Returns to the Principal

The operator returns the sovereignty-scrubbed signed receipt to the gateway. In synchronous gateway mode the receipt goes directly to the HTTP caller; in outbound mode it is pushed over the mTLS tunnel. The receipt is returned even on execution failure so callers receive cryptographic evidence of the attempt. The gateway returns the final safe output, plus an audit reference for traceability.

---

## Security Properties

### Fail-Closed Design

Every verification layer fails closed. A failed check rejects the transaction immediately and releases the nonce reservation. The Actuator will not execute a mutation if it fails to sign or log the initial receipt. The posture factory rejects unrecognized posture names at startup so misconfigured deployments fail rather than silently running under a weaker posture.

### Sovereignty

Raw data and audit logs stay on the sovereign host. Operators initiate outbound-only connections to the gateway, so the gateway cannot reach into an operator. Sensitive data is scrubbed at the execution boundary and rehydrated only locally before execution.

### Cryptographic Integrity

L2 consensus votes are Ed25519 signatures from enrolled members, produced over the transaction hash and the member's decision, and verified against the trusted signer store and the configured consensus policy. Every receipt is signed by the L5 Actuator using Ed25519 over a canonical representation of the receipt. Audit entries are stored in encrypted databases, and file mutations are optionally encrypted before storage. mTLS protects the HTTPS port with application-layer enforcement for non-public routes, and transport-to-envelope identity binding prevents impersonation by matching certificate SPIFFE URI SANs to envelope identity claims. See [Encryption](./encryption.md) for the key hierarchy and cryptographic primitives.

### Defense in Depth

The five layers interlock so that each layer assumes the prior layer may be compromised. L1 provides technical bedrock validation, L2 adds multi-signature consensus, L3 adds human authorization, L4 adds replay and state-root protection plus posture gating, and L5 adds a fail-closed signed execution boundary with zero standing privileges.

### Accountability

Every transaction is logged with a unique transaction hash. Every failure is recorded with a typed rejection. Principal identity is verified at L3, and the L2 and L3 status are reflected in every signed receipt.

---

## Related Documentation

- [Gateway Architecture](./gateway.md): Gateway mode, MCP endpoints, admission control, and the 5-layer verification sequence.
- [Operator Architecture](./operator.md): Operator-side verification pipeline, execution boundary, and local audit vault.
- [Authentication & Authorization](./auth.md): mTLS identity binding, passkey enrollment, session management, and L3 notary modes.
- [Consensus](./consensus.md): Consensus policy, enrollment, deliberation, and L2 vote verification.
- [Encryption](./encryption.md): Cryptographic primitives, key hierarchy, and TLS configuration.
- [Protocol](./protocol.md): GovernanceEnvelope wire format and message types.
