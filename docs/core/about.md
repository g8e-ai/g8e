---
title: About
parent: Core
---

# About g8e

Last Updated: 2026-09-08
Version: v2.1.7

## Why g8e Exists

I have spent thirty years managing and protecting data across remote systems: structured data, unstructured data, and blobs; NFS, SMB, HTTPS, S3, and SSH; Linux, Unix, and Windows; WANs, LANs, applications, networks, and storage. The technical work was only part of the job. The rest included security reviews, audits, sales cycles, customer visits, root-cause analyses, mission-critical service design, and painful production conversations.

The part I value most is taking that burden away from people so they can get on with their other work and their lives.

During production incidents, the people on the call were often handling several other deadlines at once. That was reasonable. They had an expert working the problem, and they had other responsibilities competing for their attention. I learned to ask a simple question my mother taught me: how would I want to be treated in this situation?

If a production storage array were down hours before a major company event, with managers escalating and a deadline closing in, I would want someone trustworthy to take control, fix the problem, prove that it was fixed, and give me a clear record I could forward to management after the call. That is how I worked. I gathered as much context as I could directly from the affected systems, asked high-signal questions, kept meticulous notes, explained the next action, and showed my work. When customers allowed me to drive under their supervision and credentials, they could focus on everything else demanding their attention.

That trust did not come from personality or blind faith. I was the person their escalation policy required them to call, I worked inside their controls, and our incentives were aligned around the same outcome.

I wanted people to have that kind of help in their pocket: a capable expert powered by safe and reliable AI, available without depending on an escalation chain. That is why I built g8e.

## Danny as Code

g8e encodes the operating method I developed across those thirty years:

1. Gather broad context from the user and the systems involved.
2. Ask focused questions that reduce uncertainty.
3. Converge on the best next step rather than acting on the first plausible answer.
4. Present the proposed action and its justification to the person with the most at stake.
5. Bind approval to the exact action when the active policy requires human authorization.
6. Execute through a constrained boundary at the system that owns the data.
7. Verify the outcome, preserve signed evidence, and follow the work through to completion.

The optional first-party [g8ee agentic ensemble](../architecture/ensemble.md) implements one reasoning and ReAct workflow around that method. The core g8e platform does not trust a model, an ensemble, or a human interface with execution authority merely because it proposed an action. It supplies the identity, policy, verification, execution, and receipt boundary between a request and a real-world side effect.

## What g8e Is

g8e is a zero-trust execution platform between humans, AI systems, applications, and the devices they affect. It governs actions rather than actors. A user or AI can propose work, but a mutation that traverses the governed platform is represented as a typed, state-bound `GovernanceEnvelope` and reaches execution only after the checks required by the active governance posture succeed.

The platform separates policy decisions from execution authority:

| Component | Role |
| --- | --- |
| **g8e Protocol** | Defines the canonical envelope, identity and state binding, replay controls, transaction hash, governance proofs, and signed receipt model. |
| **g8eg Governance Gateway** | Acts as the Policy Decision Point for authentication, PKI, envelope construction, L1 Doctrine, required L2 Consensus coordination, L3 Notary workflows, routing, and platform APIs. It also contains an in-process Operator substrate for work executed on the Gateway host. |
| **g8eo Governed Operator** | Acts as the Policy Execution Point on a managed host. It pulls work over outbound mTLS, verifies each envelope at its local L4 Warden, executes accepted work through L5, and retains authoritative local receipts. |
| **Consensus service** | Evaluates transactions and emits Ed25519 votes over the transaction hash. Votes have protocol authority only when they come from enrolled members and satisfy the configured policy and quorum. |
| **People, applications, and interfaces** | Humans, AI clients, g8ee, g8ed, MCP clients, A2A clients, and native applications propose actions and consume results without joining the trusted execution boundary. |

The reference Gateway and Operator are two modes of the same statically linked Go binary. The repository also includes the optional Python g8ee ensemble and JavaScript g8ed dashboard. These are reference implementations of separable protocol roles, not requirements for building a compatible client, Gateway, Operator, consensus service, or interface.

## The Verification Boundary

Every governed operation reaches the L4 Warden and L5 Actuator boundary. The active [governance posture](../architecture/governance.md) determines whether L2 and L3 are enforced gates or recorded, non-gating evidence:

1. **L1 Doctrine** decodes typed payloads and applies field constraints, forbidden-pattern rules, and MITRE ATT&CK-oriented threat detection. L1 is enforced in every posture.
2. **L2 Consensus** verifies Ed25519 votes from enrolled consensus members against a configured policy and quorum when the posture requires multi-agent authorization.
3. **L3 Notary** verifies transaction-bound human authorization for mutations when the posture requires it. Gateway workflows use WebAuthn; outbound Operator workflows use signed approval proofs.
4. **L4 Warden** reserves the nonce, checks expiry and replay state, recomputes the transaction hash, validates the state root and payload, reruns Doctrine, and verifies posture-required L2 and L3 evidence before dispatch.
5. **L5 Actuator** persists signed pre-execution evidence, appends a commitment when the SQL commitment ledger is available, rehydrates explicitly registered protected values at the execution site, mints a transaction-bound capability, dispatches the handler, dissolves the capability, and persists the signed final outcome.

A remote Operator performs L4 and L5 on the managed host. Gateway MCP and A2A calls use the Gateway's in-process Operator substrate unless routed to a configured downstream service. The exact guarantees differ for direct envelopes, Operator command relay, Gateway MCP and A2A ingress, and the external MCP wrapper; [AI Agents and the g8e Governance Boundary](../architecture/agents.md) defines those limits.

## Sovereignty and Accountability

The design keeps execution authority and authoritative evidence at the data owner's boundary:

- The remote Operator initiates its connection to the Gateway over mTLS and exposes no inbound management port.
- The Operator independently verifies an envelope before changing its host; Gateway admission alone cannot force execution.
- Governed read and tool outputs pass through bounded scrubbing paths before they are returned. Explicit reversible placeholders can keep registered sensitive values out of earlier reasoning and transport stages and rehydrate them at L5. Scrubbing minimizes disclosure but does not claim that every application prompt or every value is automatically tokenized.
- Vault and keystore services protect selected persisted content and platform keys. The [Encryption Architecture](../architecture/encryption.md) defines exactly which data is encrypted and where key custody remains deployment-dependent.
- L5 persists a signed `EXECUTING` receipt before dispatch and a signed final receipt after execution. Governed file mutations also record file evidence when the file ledger is enabled.
- L5's transaction capability gates internal dispatch, but the operating-system account and external credentials available to an Operator remain explicit deployment responsibilities.

Receipts prove what crossed the governed path. g8e does not sandbox an AI process or attest to actions taken through client-native tools, other MCP servers, direct shell or filesystem access, or any side channel outside that path.

## Current Reference Implementation

The current Go implementation provides a unified MCP endpoint, a separate A2A call route, mTLS authentication, optional JWT authentication with just-in-time user provisioning when JWKS is configured, WebAuthn and CLI approval workflows, platform event streaming over SSE, an MCP streaming transport, pub/sub, PKI, and Document, KV, and Blob stores.

The native registry contains 32 governed tools for database inspection, log filtering, process and host telemetry, network and TLS diagnostics, configuration and filesystem inspection, containers and Kubernetes, cloud metadata, shell execution, remote Operator deployment, and audit receipt queries. The tools are compiled into the Go binary, but operations that invoke host facilities such as a shell, `git`, `kubectl`, service managers, or time-synchronization utilities require those facilities to exist on the target system.

See the [Platform Overview](../architecture/overview.md) for the component and data flow, [Governance](../architecture/governance.md) for exact posture semantics, [Operator Architecture](../architecture/operator.md) for execution behavior and the tool catalog, and the [Protocol Specification](../../protocol/docs/spec.md) for the wire contract.

## Related Documentation

- [Position Paper](./position_paper.md): The sovereignty argument and system model behind g8e.
- [Platform Overview](../architecture/overview.md): Components, trust boundaries, and end-to-end transaction flow.
- [Governance](../architecture/governance.md): Five-layer verification, posture behavior, and receipt flow.
- [AI Agents and the g8e Governance Boundary](../architecture/agents.md): Integration paths, governed scope, and bypass limits.
- [Gateway Architecture](../architecture/gateway.md): Gateway services, authentication surfaces, routing, and local execution.
- [Operator Architecture](../architecture/operator.md): Outbound execution, local verification, native tools, and audit evidence.
- [Consensus Architecture](../architecture/consensus.md): Enrollment, deliberation, signatures, and quorum.
- [Authentication and Authorization](../architecture/auth.md): mTLS, SPIFFE identity, sessions, WebAuthn, and CLI proofs.
- [Encryption Architecture](../architecture/encryption.md): Vault, keystore, scrubbing, rehydration, and key-custody boundaries.
- [Storage Architecture](../architecture/storage.md): Audit, commitment, execution-vault, file-ledger, and application storage ownership.
- [Network Architecture](../architecture/network.md): PKI, mTLS, enrollment, ports, and outbound Operator connectivity.
- [Protocol Specification](../../protocol/docs/spec.md): Canonical messages, hashes, proofs, and wire rules.
