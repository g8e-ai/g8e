# Byzantine Fault Tolerance at the AI Execution Boundary

*Danny Barbour*
*May 2026*

## Abstract

Agentic systems are beginning to mutate production infrastructure through generic tool protocols. MCP, A2A, OpenAI-style tool calls, and similar interfaces improve interoperability, but they do not establish execution authority. They describe how an agent asks for work; they do not prove that the work is current, consensus-backed, authorized, bounded, or locally auditable.

g8e treats agentic execution as a Byzantine Fault Tolerance problem. Every mutation is represented as a canonical JSON `GovernanceEnvelope`: a typed payload, deterministic transaction hash, nonce, expiry, state root, L1 technical evidence, L2 consensus evidence, and L3 human authorization evidence. A host-resident Operator verifies the envelope independently, writes signed receipts to local audit state, and executes only through Warden.

The result is a protocol substrate for AI-controlled infrastructure where interoperability and sovereignty compose cleanly. Open tool protocols remain useful payloads. g8e supplies the admission boundary.

---

## 1. Operational premise

AI agents are now coupled to terminals, file systems, ticketing systems, cloud APIs, CI/CD platforms, databases, and production observability. The industry has standardized much of the conversation between models and tools, especially around JSON-RPC and HTTP payloads.

State-changing infrastructure has different requirements:

- **Intent requires binding** - a request must carry typed payload semantics, current host state, freshness, replay protection, and explicit authorization.
- **Single-model control creates correlated failure** - hallucination, prompt injection, provider outage, model drift, and supply-chain compromise can collapse through one decision path.
- **Approval requires machine-verifiable context** - human authorization must sit behind deterministic policy checks, typed payload decoding, signer trust, and state-root validation.
- **Audit authority belongs at execution** - the execution site must retain the authoritative record, including pre-state, post-state, receipts, and forensic material.
- **Protocol compatibility leaves execution authority unresolved** - an MCP `tools/call` message can be well formed and still be unsafe to execute.

The execution boundary has to distrust the AI control plane, the transport, the client, the model provider, and stale local assumptions.

---

## 2. g8e architecture

g8e defines a governed execution substrate with four cooperating roles:

- **Principal** - the human or upstream agent requesting an outcome. For mutations, the final authorization path is anchored by hardware-backed human proof unless policy explicitly permits auto-approval after L1 and L2 verification.
- **Protocol substrate** - the schemas, envelope rules, transaction hash, state binding, proof model, and receipt semantics that every conforming participant must implement.
- **Governance Gateway (`g8eg`)** - the reference policy decision point. It owns admission APIs, mTLS identity, PKI, pub/sub fan-out, transaction suspension, replay protection, state roots, and dispatch to governed Operators.
- **Governed Operator (`g8eo`)** - the reference host-side policy execution point. It is the sovereign execution boundary, local audit authority, Sentinel scrubber, Warden dispatcher, and MCP server.

Applications sit above these roles. The reference Engine (`g8ee`) is one producer of governed transactions. BYO agents, BYO frontends, MCP clients, A2A clients, and native g8e applications can produce the same envelope without privileged access.

The mandatory invariant is narrow: a state-changing action reaches the host only as a typed, signed, state-bound transaction, and the host verifies the transaction before executing.

---

## 3. The `GovernanceEnvelope`

The `GovernanceEnvelope` is the execution contract. It is serialized as canonical JSON (protojson) on client-facing surfaces so it remains compatible with JSON-native AI ecosystems while retaining protobuf schemas as the source of truth.

Each envelope binds:

- **Identity** - workload identity, session context, signer key IDs, and target Operator scope.
- **Typed payload** - a protobuf action message, base64-encoded in the envelope, with no `intent_data` fallback for execution.
- **Transaction hash** - deterministic hash over normalized fields; the envelope `id` must equal the computed hash.
- **Freshness** - required `expires_at` and nonce, enforced by replay storage.
- **State root** - expected Merkle root of the target host or gateway state at signing time.
- **L1 evidence** - technical hard gates, forbidden pattern checks, and Sentinel policy status.
- **L2 evidence** - Ed25519 consensus signature from a trusted signer or Tribunal producer.
- **L3 evidence** - WebAuthn/FIDO2 proof for human-authorized mutations, using the transaction hash as challenge.

The envelope makes governance metadata travel with execution intent. Verification is not an ambient property of the network; it is part of the transaction.

---

## 4. The verification gauntlet

The Gateway and Operator reject non-conforming transactions before application code runs. The ordered admission path is:

1. **Envelope integrity** - canonical JSON parses, required fields exist, action type is registered.
2. **Typed payload binding** - the payload decodes as the protobuf message declared by the action type.
3. **Hash binding** - `id == transaction_hash == SHA256(canonical_fields)`.
4. **Freshness** - expiry is valid and nonce has not been seen in the active replay window.
5. **State binding** - `state_merkle_root` matches the current local state root.
6. **L1 technical gates** - reflected forbidden-pattern checks, allow/deny policy, and Sentinel analysis pass.
7. **L2 consensus** - signer key resolves to the trusted store and the signature verifies over the transaction.
8. **L3 authorization** - WebAuthn proof validates for mutations, or explicit auto-approval policy applies after L1 and L2 pass.

Any failure produces a typed rejection and audit record. The payload is dropped at the boundary. Warden is not invoked.

This ordering matters. It keeps malformed, stale, unsigned, replayed, mistyped, and locally unsafe instructions out of the execution path with deterministic behavior.

---

## 5. Consensus as infrastructure control

g8e treats L2 as an interchangeable proof layer. The reference Engine uses a multi-agent Tribunal, but the substrate only requires verifiable consensus evidence from a trusted producer.

The reference model is intentionally heterogeneous:

- **Model diversity** - independent seats can use different providers, local models, tiers, prompts, and tool policies.
- **Role separation** - intent articulation, candidate generation, adversarial review, risk analysis, audit verification, and execution are separate responsibilities.
- **Adversarial pressure** - Nemesis-style review is a structured fault-injection mechanism with explicit reputation consequences.
- **Reputation accountability** - agent outcomes can be committed into tamper-evident reputation state and used for future weighting or slashing.
- **Provider agnosticism** - no single model vendor is assumed to be available, honest, stable, or sufficient.

The strategic property is independent evidence before mutation, carried cryptographically to the host boundary.

---

## 6. Universal protocol translation

Tool protocols are becoming numerous because agent ecosystems optimize for different runtimes, trust models, and integration surfaces. g8e uses a stable governance envelope so protocol churn does not become execution churn.

At ingress:

- **MCP** - JSON-RPC requests can be parsed, typed, and bound into an envelope.
- **A2A** - HTTP/JSON task messages can be normalized into the same transaction form.
- **OpenAI-style tool calls** - function-call arguments can be typed and governed before dispatch.
- **Native g8e** - applications can emit the envelope directly.

At egress:

- **Downstream MCP servers** receive MCP-shaped requests only after governance admission.
- **A2A services** receive their expected HTTP/JSON form only after verification.
- **Host tools** run only through Warden.
- **Clients** receive protocol-native responses plus signed receipt material.

This makes g8e a zero-trust universal translator: protocol agnostic at the edges, strict at the mutation boundary.

---

## 7. Native g8e adoption

Translation allows existing tools to participate. Native g8e applications remove the translation seam.

A native application emits a `GovernanceEnvelope` directly when it forms intent. Governance data is present before transport, before dispatch, and before any host-side interpretation. The application can be written in any language, use any model provider, and present any frontend, provided it can produce valid envelopes and consume signed receipts.

Native adoption gives engineering teams:

- **Governance by construction** - transaction metadata exists at intent time rather than being inferred after a tool call.
- **Audit-ready semantics** - the same transaction hash binds request, proof, execution, result, and receipt.
- **Interoperable autonomy** - any conforming Operator can verify the action without trusting the application that produced it.
- **Lower application burden** - L1/L2/L3 enforcement, replay defense, state binding, receipts, local audit, and human proof are delegated to the substrate.

This is the adoption curve: translate existing protocols first, emit governed transactions natively when the system becomes important.

---

## 8. Local-first audit authority

In g8e, execution is conditional on auditability.

Before mutation, Warden writes an executing-state receipt to the host-local audit vault. After mutation, Warden signs the final status with post-state root and result metadata. File mutations are captured in a per-session ledger with before and after hashes. Sentinel scrubs outbound material so AI systems and remote clients receive safe projections, while raw forensic context remains local.

The host keeps the authoritative record:

- **Accepted transaction** - envelope, proofs, signer metadata, and state root.
- **Blocked transaction** - rejection reason and boundary context.
- **Executing receipt** - signed intent-to-execute before side effects.
- **Final receipt** - completed or failed status, post-state root, and signature.
- **Scrubbed output** - AI-readable projection.
- **Raw output** - customer-controlled forensic material.

Audit is a prerequisite for execution and a durable consequence of every admission decision.

---

## 9. Security posture

The system assumes compromise as a baseline engineering condition:

- **Untrusted client** - the client can be confused, malicious, compromised, or obsolete.
- **Untrusted model** - any individual model can hallucinate, collude, degrade, or disappear.
- **Untrusted transport** - messages require mTLS identity and envelope-level verification.
- **Untrusted application layer** - bundled and BYO applications have no private mutation channel.
- **Untrusted host assumptions** - stale state roots and reused nonces are rejected.
- **Untrusted output** - Sentinel scrubs secrets, PII, tokens, and credential-bearing strings before external exposure.

The Operator's job is narrow and severe: mutate only when the transaction proves it is current, authorized, consensus-backed, policy-compliant, and locally auditable.

---

## 10. Conclusion

Agentic infrastructure needs a native execution governance layer. The useful abstraction is a transaction envelope with typed payloads, state binding, independent proofs, and signed receipts.

g8e defines that envelope and places a sovereign verifier at the host boundary. Open protocols carry capability. Heterogeneous agents produce evidence. Humans provide hardware-bound authorization where required. The Operator verifies the transaction against local state, records the attempt locally, executes through Warden, and returns signed proof.

That is the minimum viable substrate for autonomous systems that are allowed to change reality.
