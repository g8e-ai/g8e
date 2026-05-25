# Governed Execution Protocol for Agentic Infrastructure

## Introduction

g8e provides a zero-trust execution substrate for agentic infrastructure. It treats agentic execution as a Byzantine Fault Tolerance problem; it supplies a governed admission boundary at the host. Standard protocols like the Model Context Protocol (MCP) and Agent-to-Agent (A2A) establish capability, while g8e establishes authority.

Native g8e serves as the direct envelope-producing adoption path. Every mutation travels as a canonical JSON `GovernanceEnvelope` binding identity, intent, state, and governance proofs into a single verifiable transaction. A sovereign, host-resident Operator (`g8eo`) verifies the envelope against local state, records every decision to a local audit vault, and executes only through a single fail-closed dispatch path.

## The Lifecycle Pipeline

The execution lifecycle enforces verification before any side effects occur. The system operates on a fail-closed basis at every step.

1. **Envelope Assembly**: The client constructs a `GovernanceEnvelope` containing a typed payload, a deterministic transaction hash, a nonce, an expiry timestamp, and an expected state root.
2. **Ingress**: The `g8eo` Operator receives the envelope over an outbound-only mTLS tunnel from the `g8eg` Governance Gateway.
3. **L4 Warden Verification**: The Operator's L4 Warden subjects the transaction to a deterministic verification gauntlet.
   - **Envelope Integrity**: Validates canonical JSON parsing and required fields.
   - **Typed Payload Binding**: Ensures the payload decodes as the declared protobuf message.
   - **Hash Binding**: Verifies the envelope ID equals the SHA-256 hash of its normalized fields.
   - **Freshness**: Checks the expiry timestamp and ensures the nonce is unseen in the active replay window.
   - **State Binding**: Confirms the expected Merkle state root matches the current local root.
   - **L1 Doctrine**: Enforces static technical policy, including forbidden patterns and output scrubber rules.
   - **L2 Consensus**: Validates the Ed25519 signature from the multi-agent consensus panel over the transaction hash.
   - **L3 Notary**: Validates the WebAuthn proof or explicit auto-approval policy for human-authorized mutations.
4. **Local Audit Recording**: The Operator writes a signed executing-state receipt to the host-local vault.
5. **L5 Actuator Dispatch**: Verified payloads are dispatched to the host execution environment.
6. **Result Capture**: Output is scrubbed of sensitive material for remote clients. Raw forensic data remains local, and a final signed receipt is recorded with the post-state root.

## Core Subsystems

### The Governance Envelope

The `GovernanceEnvelope` serves as the execution contract. It forces verification to become part of the transaction rather than a property of the network. The envelope is serialized as canonical JSON (protojson) on client-facing surfaces. The protobuf schema remains the source of truth for typing and hashing. Any mutation of a bound field alters the transaction hash and invalidates the signatures.

### Governance Tiers

g8e uses a tiered governance hierarchy evaluated by the L4 Warden.

- **L1 Doctrine**: Technical bedrock. Provides deterministic policy enforcement via statically defined hard gates and forbidden patterns.
- **L2 Consensus**: Cryptographic consensus. Requires threshold agreement from independent agent seats. Heterogeneous prompts and models reduce correlated single-model failure; provider agnosticism is required to ensure independent voting.
- **L3 Notary**: Hardware-bound authorization. Uses the transaction hash as the WebAuthn challenge to bind human approval to a specific action rather than an ambient session.

### Operator and Gateway

The architecture separates the policy decision point from the policy enforcement point.

- **Governance Gateway (`g8eg`)**: Serves as the central policy decision point. It manages admission APIs, mTLS identity, replay protection, and state-root distribution.
- **Governed Operator (`g8eo`)**: Serves as the host-side policy enforcement point and MCP server. It is a single statically compiled binary with zero standing dependencies. The Operator maintains an outbound-only tunnel to the Gateway; it exposes zero inbound listening ports on the host.

## Governance & Safety

The system assumes baseline compromise of networks, clients, and models.

- **State Sovereignty**: Data and execution sovereignty remain local. The Operator evaluates requests against the local state root before any application logic runs.
- **Fail-Closed Execution**: If any verification step fails, the L4 Warden drops the payload and logs the rejection to the audit vault. The L5 Actuator is never reached.
- **Audit Immutability**: All decisions, including rejections, are permanently stored locally. File mutations are captured in a Git-backed ledger, providing a tamper-evident history and instant rollback capability.
- **Protocol-Agnostic Translation**: Standard JSON-RPC and HTTP tool calls are forced into the protojson `GovernanceEnvelope` and run through the verification gauntlet before downstream MCP or A2A execution.

