# Governance

g8e implements a 3-layer governance architecture for autonomous infrastructure management. Each layer provides progressively stronger guarantees.

## L1: Technical Bedrock

Hard gates enforced via protobuf reflection. Includes forbidden pattern detection, blacklist enforcement, and whitelist validation. These are deterministic, non-negotiable checks that run before any consensus or authorization.

## L2: Consensus

Multi-agent Tribunal consensus with reputation staking and Ed25519 signatures. The ensemble produces typed, signed GovernanceEnvelope transactions that are validated by the Gateway. This is where g8ee operates as an L2 producer.

Key concepts:

- **Tribunal** — Multi-agent consensus body that evaluates and signs transactions
- **Reputation staking** — Agents stake reputation on their decisions
- **Ed25519 signatures** — Cryptographic signatures on all consensus outputs
- **GovernanceEnvelope** — Typed transaction container carrying the consensus payload

## L3: Authorization

Human-in-the-loop authorization with hardware-bound WebAuthn/FIDO2 proofs. Required for high-risk operations and policy changes. Ensures human oversight on critical infrastructure decisions.

## Enforcement Flow

```
L1 (Technical Bedrock) → L2 (Consensus) → L3 (Authorization) → Execution
```

Transactions must pass all applicable layers before execution by the Operator's Actuator stage.

## Related

- [Agents](agents.md) — Agent hierarchy and Tribunal composition
- [Thinking](thinking.md) — L2 consensus and provider reasoning
- [PKI & Trust](pki.md) — Cryptographic foundations for signatures
