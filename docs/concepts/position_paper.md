# Byzantine Fault Tolerance at the AI Execution Boundary

**A Governed Execution Protocol for Agentic Infrastructure**

*Danny Barbour — Lateralus Labs*
*May 2026*

---

## Abstract

The industry has spent two years standardizing how AI agents talk to tools. Function calling, the Model Context Protocol (MCP), agent-to-agent (A2A) messaging, and remote tool servers have made it routine to wire a model into a terminal, a cloud control plane, a CI/CD pipeline, or a production database. These protocols establish *capability*. None of them establish *authority*. A well-formed `tools/call` is a request to change something; it is not proof that the change is current, consensus-backed, authorized, bounded, or auditable at the point where it lands.

g8e treats agentic execution as a Byzantine Fault Tolerance problem and supplies the layer the tool protocols leave out: a governed admission boundary at the host. Every mutation is carried as a canonical JSON `GovernanceEnvelope` — a typed payload, a deterministic transaction hash, a nonce and expiry, an expected state root, and evidence for three independent gates: **Doctrine** (deterministic technical policy), **Quorum** (multi-party consensus), and **Notary** (hardware-bound human authorization). A host-resident Operator verifies the envelope independently against local state, records every decision to a local audit vault, and executes only through the **Actuator**, a single fail-closed dispatch path. The Operator never accepts an inbound connection; it reaches the control plane over an outbound-only tunnel.

The result composes two properties that usually trade against each other: interoperability and sovereignty. Open protocols stay useful at the edges. g8e owns the mutation boundary at the center. This paper describes the model, the verification gauntlet, the reference implementation, the threat model, and — deliberately — the things g8e does not claim to solve.

---

## 1. The execution gap

Look at how a competent team wires an agent into infrastructure today. The model runs a tool-use loop. Tools are exposed through function-calling schemas, or wrapped behind MCP servers, or reached over A2A. Increasingly the MCP server is remote, fronted by a tunnel so the agent can reach tools that live inside a private network. This plumbing is good engineering, and it has made agentic automation genuinely useful.

It is also, in authority terms, almost entirely unguarded.

The tool protocols answer one question well: *how does the agent ask for work?* They are silent on every question that matters once the work mutates real state. Is this request fresh, or a replay of one issued an hour ago against state that has since changed? Did anything other than a single model's forward pass decide it was safe? Is there cryptographic proof a human approved it, bound to *this* action and not merely to a session? When it executes, who holds the authoritative record — and can they reconstruct the before-state if it needs to be undone?

In current practice the answers are improvised. Authorization is a scoped API token the agent already holds. Human oversight is a message in Slack with an "approve" button, unbound to the specific operation and trivially fatigued into reflexive clicking. Safety is a system prompt that asks the model to be careful. None of these are bound to the action, verifiable by the host, resistant to a single compromised component, or durable as an audit record. They are conventions, and conventions do not survive contact with a confused, injected, or simply wrong agent.

The stakes are not hypothetical. Agents now hold write paths into terminals, cloud APIs, orchestration layers, source control, deployment pipelines, and the observability stack used to detect that something has gone wrong. The blast radius of a single bad mutation is large, occasionally irreversible, and rarely contained by the same protocol that delivered it.

The missing layer is an admission boundary: a place where a proposed mutation must *prove itself* before it touches the host, and where the proof and its outcome are recorded locally and permanently. g8e is that boundary.

**Contributions.** This paper makes four claims and describes the design behind each.

1. **A transaction model for agentic mutation.** The `GovernanceEnvelope` makes governance metadata travel *with* execution intent, so verification is part of the transaction rather than an ambient property of the network (§4).
2. **A deterministic admission gauntlet.** Three named gates — Doctrine (L1Doctrine), Quorum (L2Consensus), Notary (L3Notary) — reject malformed, stale, unsigned, replayed, mistyped, and locally unsafe instructions before any application code runs, fail-closed at every step (§5).
3. **Consensus as an interchangeable proof layer.** Quorum (L2Consensus) is a verifiable-evidence requirement, not a particular multi-agent design, and its value is quantifiable and bounded (§6).
4. **A sovereign, dependency-free execution boundary.** A single statically compiled Operator enforces the protocol on the host, keeps the authoritative audit record local, and reaches the control plane outbound-only — which makes air-gapped and isolated-perimeter deployment ordinary rather than exceptional (§7, §8).

---

## 2. Where the industry is already heading

g8e is not arriving in a vacuum. The most credible work in enterprise AI infrastructure over the last two years has been about *reasserting control* — and it has concentrated almost entirely on the data plane.

Hammerspace presents a single global namespace across heterogeneous storage and orchestrates data placement by performance, cost, and compliance, assimilating existing data in place rather than migrating it. Nasuni provides a governed, versioned, AI-ready file layer on cloud object storage, with immutable snapshots and ransomware resilience, positioned as a single authoritative source of truth that both people and agents can read. Different architectures, same instinct: as AI begins to consume everything, enterprises want their data sovereign, governed, and recoverable — available to the model without surrendering custody of it.

This is the right instinct applied to half the problem. Governing where data lives and how it is accessed is necessary. It is not sufficient. An agent with perfectly governed, perfectly sovereign data access can still issue a perfectly catastrophic *write*. Data sovereignty without execution sovereignty is one wall of a vault.

| Plane | Question it answers | Maturing solutions |
| --- | --- | --- |
| **Control plane** | How does the agent talk to tools? | MCP, A2A, function calling, remote tool servers |
| **Data plane** | Where does data live, who may read it, how is it recovered? | Hammerspace, Nasuni, DSPM tooling |
| **Execution plane** | What is the agent *allowed to do*, and who proved it? | **largely unaddressed — g8e** |

g8e occupies the third row. It is the execution-plane complement to the data-plane work already underway, and it is deliberately designed to sit alongside it rather than replace it: Hammerspace and Nasuni decide what an agent can *touch*; g8e decides what an agent can *change*, and forces that decision to be proven.

---

## 3. Governed execution

g8e begins from one assumption: every participant may be compromised, and the boundary must behave correctly anyway. The control plane can be confused, the transport hostile, the client obsolete, the model injected, and the host's own memory of its state stale. The boundary trusts none of them and verifies all of them.

Four roles cooperate.

- **Principal** — the human or upstream agent requesting an outcome. For mutations, the authorization path terminates in hardware-backed human proof unless policy explicitly permits auto-approval after the deterministic and consensus gates pass.
- **Protocol** — the `GovernanceEnvelope` schema, transaction-hash rules, state binding, proof model, and receipt semantics. This is the only mandatory component for interoperability; everything else is a reference implementation.
- **Governance Gateway (`g8eg`)** — the reference policy decision point: admission APIs, mTLS identity and PKI, fan-out to Operators, replay protection, and state-root distribution.
- **Governed Operator (`g8eo`)** — the reference policy enforcement point and sovereign execution boundary: local audit authority, output scrubber, Actuator, and itself an MCP server.

The mandatory invariant is narrow on purpose: *a state-changing action reaches the host only as a typed, signed, state-bound transaction, and the host verifies that transaction before it executes.* Everything in the rest of this paper is in service of that one sentence.

The difference from current practice is not incremental, and it is worth stating as a direct comparison.

| Concern | Current practice (raw MCP / function calls) | g8e |
| --- | --- | --- |
| **Action binding** | Request carries arguments; intent is implicit | Typed payload bound into a deterministic transaction hash |
| **Freshness / replay** | None; a valid call is valid indefinitely | Nonce + expiry, enforced against a replay window |
| **State assumption** | Unstated; agent acts on possibly stale context | Expected Merkle state root bound at signing; stale is rejected |
| **Decision authority** | One model's output | k-of-n heterogeneous consensus, cryptographically signed |
| **Human approval** | Out-of-band click, bound to a session at best | WebAuthn proof using the transaction hash as challenge |
| **Audit locus** | Control plane / vendor logs, after the fact | Host-local vault, written before the side effect |
| **Transport exposure** | Inbound tool endpoint, often tunnelled in | Outbound-only; zero inbound listeners on the host |
| **Failure mode** | Fail-open (acts unless something stops it) | Fail-closed (does not act unless everything proves out) |

---

## 4. The `GovernanceEnvelope`

The envelope is the execution contract. It is serialized as canonical JSON (protojson) on client-facing surfaces, so it stays native to the JSON ecosystems agents already live in, while protobuf schemas remain the source of truth for typing and hashing.

Its purpose is to make verification part of the transaction rather than something the network is trusted to have done. The binding that makes this work is the transaction hash: the envelope's identity *is* the hash of its own normalized contents.

```text
id == transaction_hash == SHA-256(canonicalize(envelope_fields))
```

Any mutation of any bound field changes the identity, which invalidates every signature taken over it. Tampering is not detected after the fact; it is structurally impossible to do without producing a different, unsigned transaction.

| Field group | Binds | Mechanism |
| --- | --- | --- |
| **Identity** | Workload identity, session, signer key IDs, target Operator scope | mTLS identity, signer resolution |
| **Typed payload** | A protobuf action message, base64-encoded; no untyped fallback is executable | Registered action-type decode |
| **Transaction hash** | Envelope `id` equals the hash over normalized fields | SHA-256 over canonical fields |
| **Freshness** | `expires_at` and nonce | Replay-window storage |
| **State root** | Expected Merkle root of target state at signing time | Local state-root comparison |
| **Doctrine evidence** | Forbidden-pattern and policy status | Static analysis / reflection |
| **Quorum evidence** | Consensus signature from a trusted producer | Ed25519 over the transaction |
| **Notary evidence** | Human-authorization proof, where required | WebAuthn/FIDO2, hash as challenge |

A transaction either proves it is current, typed, fresh, consensus-backed, and authorized — or it is not a transaction, and the host treats it as noise.

---

## 5. The verification gauntlet

The Gateway and Operator reject non-conforming transactions before any application logic runs. The order is not arbitrary. Cheap, deterministic, locally decidable checks come first, so the expensive layers — consensus and human attention — are never spent on garbage. State binding precedes the policy gates, so a stale or replayed transaction dies before a human is ever paged to approve it.

1. **Envelope integrity** — canonical JSON parses, required fields present, action type registered.
2. **Typed payload binding** — payload decodes as the protobuf message its action type declares.
3. **Hash binding** — `id == transaction_hash == SHA-256(canonical_fields)`.
4. **Freshness** — expiry valid; nonce unseen in the active replay window.
5. **State binding** — `state_merkle_root` matches the current local root.
6. **Doctrine (L1Doctrine)** — reflected forbidden-pattern checks, allow/deny policy, and output-scrubber analysis pass.
7. **Quorum (L2Consensus)** — signer resolves to the trusted store and the Ed25519 signature verifies over the transaction.
8. **Notary (L3Notary)** — WebAuthn proof validates for human-authorized mutations, or an explicit auto-approval policy applies after Doctrine (L1Doctrine) and Quorum (L2Consensus) pass.

Any failure produces a typed rejection and an audit record; the payload is dropped at the boundary and the Actuator is never reached. The default is closed. Auto-approval is not a fallback the system drifts into — it is a decision a human makes deliberately, in advance, for a scoped class of low-blast-radius actions, and it still requires Doctrine and Quorum to have passed.

> Ordering note: state binding before the policy gates is authoritative. Earlier reference diagrams that placed the state check last are superseded by this section.

---

## 6. Quorum (L2Consensus): consensus as an infrastructure control

The case for treating execution as a Byzantine problem rests on one observation that is easy to state and easy to quantify: **single-model control is undiluted exposure.**

Suppose a mutation is decided by one model, and on a given action that model produces a plausible but wrong — yet signable — output with probability *p*. Hallucination, a successful prompt injection delivered through retrieved content, drift, and silent degradation all live inside *p*. With one decision path, the probability that a bad action is admitted is exactly *p*. There is nothing to dilute it.

Quorum requires *k* of *n* independent seats to sign before an action carries consensus evidence. For consensus to be *fabricated*, at least *k* seats must fail on the same action. Under independence:

$$P(\text{fabricated consensus}) \;\le\; \sum_{i=k}^{n} \binom{n}{i}\, p^{\,i}\,(1-p)^{\,n-i}$$

and because the seats must additionally agree on the *same* faulty action — not merely each fail — this is an upper bound. The status quo is the degenerate case *n = 1*, where the sum collapses back to *p*. Moving to a *k*-of-*n* threshold converts a linear exposure into a tail probability.

The honest part — the part that separates this from multi-model theater — is the independence assumption. The bound holds only to the degree the seats fail *independently*. Shared training corpora, a shared system prompt, and above all a prompt injection riding in shared retrieved context introduce correlation. As that correlation rises toward one, the *n* seats collapse into a single effective seat and the bound degrades back toward *p*. Voting over five instances of the same model with the same context buys almost nothing.

This is precisely why the reference Quorum is heterogeneous by construction — different providers, different weights, different prompts, different tool policies across seats. Heterogeneity is not a marketing posture; it is the engineering mechanism for driving inter-seat correlation toward zero so the independence bound actually means something. Consensus is a variance-reduction mechanism, and its payoff is governed entirely by the independence of its inputs.

The reference model adds structure beyond raw voting: separation of roles (intent articulation, candidate generation, adversarial review, risk analysis, and audit verification are distinct responsibilities), a standing adversarial seat (Nemesis) that performs structured fault injection with reputation consequences rather than advisory commentary, and tamper-evident reputation state that can weight or slash future seat influence. No model vendor is assumed available, honest, stable, or sufficient — including the one writing this paragraph.

---

## 7. Reference implementation

The protocol is the product. The implementations are replaceable, and describing them concretely is what keeps the abstract roles honest.

### 7.1 g8e-Compatible Agentic Ensembles

g8e-compatible agentic ensembles are producers of governed transactions, built as ReAct loops over a layered hierarchy: **Triage/Dash** for routing and fast-path responses that never touch the mutation boundary; **Sage** as primary reasoner, which stakes reputation on proposals but cannot execute; the **Tribunal**, a five-seat panel requiring threshold consensus (2/5 or 5/5 by policy); **Actuator**, a heuristic circuit breaker that rejects off-the-wall proposals before they spend consensus budget; **Auditor**, which reviews the full investigation history before signing; and **Nemesis**, the embedded adversary. Crucially, these ensembles have no private channel to the host. They produce the same envelope, subject to the same gauntlet, as any BYO agent, MCP client, or A2A client.

### 7.2 The Gateway (`g8eg`)

`g8eg` is the reference decision point: admission APIs, mTLS and PKI, replay protection, state-root distribution, and fan-out to the Operators that make the final, independent enforcement decision.

### 7.3 The Operator (`g8eo`)

`g8eo` is the enforcement point and sovereign boundary — local audit authority, output scrubber, Actuator, and MCP server — shipped as a single statically compiled Go binary of roughly four megabytes with **zero standing dependencies**. There is no runtime to patch, no interpreter to exploit, no package tree to audit. This is what makes air-gapped and isolated-perimeter deployment ordinary: the binary either runs or it does not, and nothing else has to be present for it to enforce the protocol.

### 7.4 Transport: outbound-only, by inversion

Most teams expose tools to agents by standing up a server and letting connections in — increasingly a remote MCP endpoint reached through a tunnel punched into a private network. g8e inverts this. The Operator opens an outbound mTLS tunnel to the Gateway and the Gateway never reaches in. The Operator fetches pending envelopes and pushes signed receipts over that single egress path.

The consequence is structural, not cosmetic. The most sensitive component in the system — the one with the authority to mutate the host — has **no inbound listening port at all.** It cannot be scanned, reached, or attacked from the network, because from the network's perspective it is not there. It traverses NAT and enterprise firewalls without exception rules precisely because it asks for nothing inbound. Execution authority stays on the host; only governed, scrubbed material ever crosses the wire.

---

## 8. Local-first audit and data sovereignty

In g8e, execution is conditional on auditability, and the record is written before the side effect rather than reconstructed after it. This is where g8e's execution-plane sovereignty meets the data-plane sovereignty the industry is already pursuing: raw data, raw outputs, and forensic context never leave the host. The output scrubber emits only safe projections to the model and to remote clients, while the authoritative material stays local under the customer's control.

Before mutation, the Actuator writes a signed executing-state receipt to the host-local vault. After mutation, it signs the final status with the post-state root and result metadata. File mutations are captured in a per-session ledger with before-and-after content hashes, backed by a two-phase, Git-backed commit architecture that yields a tamper-evident history and instant rollback — the operational answer to "undo the change the agent should not have made."

The vault retains, for every admission decision including the rejections:

| Record | Contents |
| --- | --- |
| **Accepted transaction** | Envelope, proofs, signer metadata, state root |
| **Blocked transaction** | Rejection reason and boundary context |
| **Executing receipt** | Signed intent-to-execute, before any side effect |
| **Final receipt** | Completed/failed status, post-state root, signature |
| **Scrubbed output** | The AI- and client-readable projection |
| **Raw output** | Customer-controlled forensic material, host-resident |

Audit is both a precondition of execution and a durable consequence of every decision the boundary makes.

---

## 9. Threat model and security analysis

g8e assumes compromise as a baseline engineering condition.

**The adversary** can submit arbitrary well-formed messages through any ingress; compromise, confuse, or replace the client; co-opt any single model provider through hallucination, jailbreak, injection, collusion, or outage; observe and tamper with transport; compromise a bundled or BYO application and seek a private mutation channel; and replay valid transactions or present transactions signed against stale state.

**The trusted computing base** is small and stated up front: the integrity and build provenance of the Operator binary, the durability of the local audit vault, custody of the signing keys and the Notary authenticator, and the correctness of the cryptographic primitives (Ed25519, SHA-256, Merkle roots, mTLS, WebAuthn/FIDO2). Everything else — clients, models, transport, applications, and the host's own prior assumptions about its state — is untrusted by construction.

Each adversary capability is answered by a specific, verifiable mechanism rather than by trust.

| Threat | Mechanism | Gauntlet step |
| --- | --- | --- |
| Malformed / mistyped request | Envelope integrity + typed payload binding | 1–2 |
| Forged or altered intent | Hash binding | 3 |
| Replay of a valid transaction | Nonce + expiry + replay window | 4 |
| Action against stale host state | State Merkle-root binding | 5 |
| Policy-violating / dangerous action | Doctrine + output scrubber | 6 |
| Single-model failure / injection / collusion | Heterogeneous Quorum, Ed25519-signed | 7 |
| Unauthorized mutation | Notary WebAuthn/FIDO2 proof | 8 |
| Compromised transport | mTLS + envelope-level signatures | transport + 7 |
| Application seeking a side channel | No private channel; same envelope for all producers | §3, §7.1 |
| Secret / PII / credential exfiltration via output | Scrub before any external exposure | §8 |
| Remote attack surface on the host | Outbound-only tunnel; zero inbound ports | §7.4 |

The Operator's job is narrow and severe: reach the Actuator only when the transaction proves it is current, typed, fresh, consensus-backed, policy-compliant, and authorized — and otherwise fail closed and record why.

---

## 10. Limitations and non-goals

A governance layer that overclaims is worse than none, because it manufactures confidence in exactly the environments that can least afford it. g8e does not solve the following, and pretending otherwise would be the more dangerous error.

- **It does not make a model correct.** Quorum reduces correlated single-model failure; it does not make the agreed action wise. A unanimous, well-formed, fully authorized mistake will execute.
- **It does not constrain a fully authorized human.** Notary proves deliberate human presence. It does not stop an authorized operator from approving harm they understand and choose.
- **It does not defend its own TCB.** Compromise the Operator binary, the signing keys, or the vault storage at the host level and the guarantees dissolve. Build provenance and key custody are deployment responsibilities, not protocol features.
- **It does not eliminate latency.** Consensus and human-presence checks cost round trips. This is a deliberate trade: the fast path handles trivial reads, and state-changing actions pay the full cost by design.
- **It does not address model supply-chain provenance.** Quorum assumes the seats are not all compromised at the weight level simultaneously. Integrity of the model artifacts themselves is a separate, open problem.
- **Fail-closed is an availability trade.** A Gateway outage halts mutation. For environments where availability dominates integrity, this trade is explicit, not accidental — and for the environments g8e targets, it is the correct default.

---

## 11. Related work

**Byzantine fault tolerance.** g8e applies the classical line from the Byzantine Generals problem through Practical BFT to a new setting — the AI execution boundary rather than a replicated state machine. Where PBFT tolerates *f* faults among *n ≥ 3f+1* replicas of the *same* program, g8e's analogue is *k*-of-*n* agreement among deliberately *different* decision-makers, and its central design problem is maximizing their independence rather than replicating their identity. Tendermint-style economic slashing informs the reputation model, applied to a permissioned, host-sovereign setting rather than a public ledger.

**Policy engines.** OPA and AWS Cedar provide general-purpose decision points. g8e's Doctrine is adjacent but differs in two ways: it is embedded in the Operator with no external call, and its evidence travels *in the transaction*, making the decision portable and auditable without a live policy service.

**Software supply-chain integrity.** Sigstore, in-toto, and TUF secure artifacts through build and distribution. g8e secures what a trusted artifact *does* once it is running and issuing instructions. The Operator binary is itself a candidate for supply-chain verification; the concerns compose.

**Workload identity and attestation.** SPIFFE/SPIRE solve runtime workload identity, which g8e consumes rather than replaces. Hardware attestation (TPM, TDX, SEV-SNP) would strengthen the Operator's own TCB and is compatible with, not precluded by, the design.

**Human authorization.** WebAuthn and FIDO2/CTAP underlie Notary. The design choice that matters is using the transaction hash as the WebAuthn challenge, binding a hardware-backed signature to *one specific action* rather than to a session — which is what makes the human proof non-transferable and resistant to approval fatigue.

**Data platforms.** Hammerspace and Nasuni govern the data plane — sovereign placement, global namespace, versioning, recoverability for an AI-consuming enterprise. g8e is the execution-plane counterpart and is designed to sit beside them: they govern what the agent may read; g8e governs what it may change.

**AI tool protocols and guardrails.** MCP and A2A standardize capability; g8e governs authority over that capability, and their messages are valid envelope payloads once normalized. Content-level guardrails address what a model *says*; g8e addresses what a model *does*. Neither substitutes for the other.

---

## 12. Conclusion

The hard problem in autonomous infrastructure is not making agents capable. That is largely solved, and improving monthly. The hard problem is keeping humans in genuine control of capable agents that act on real systems, where a single mutation can be large and occasionally cannot be taken back.

There is a temptation to answer this with a human behind every action. It does not scale, it does not survive approval fatigue, and it quietly defeats the reason for automating in the first place. There is an opposite temptation to trust a sufficiently good model and let it act on its own recognizance. On infrastructure with real blast radius, that is not a safety posture; it is a single point of catastrophic failure wearing the costume of one.

g8e resolves the tension by making authority *graduated and provable* rather than uniform and assumed. Deterministic policy and cryptographic consensus absorb the volume of routine actions cheaply. Hardware-bound human authorization is reserved for the actions whose blast radius actually warrants a person — and when a person signs, they sign *that exact transaction*, against verified-current state, with the whole decision recorded locally before anything happens. This is what makes human control economically tractable instead of ceremonial: you spend scarce human attention only where it changes the outcome, and you can prove afterward that you spent it.

Open protocols carry capability. Heterogeneous agents produce evidence. Humans hold authority where it counts. The Operator verifies the transaction against local state, records the attempt, reaches the Actuator, and returns signed proof — and refuses, loudly and on the record, when the proof does not hold.

That is the minimum viable boundary for autonomous systems that are allowed to change reality. Everything above it can be as fast, as capable, and as experimental as the work demands, precisely because the boundary below it does not move.

---

## References

1. L. Lamport, R. Shostak, M. Pease. "The Byzantine Generals Problem." *ACM TOPLAS*, 4(3), 1982.
2. M. Castro, B. Liskov. "Practical Byzantine Fault Tolerance." *Proc. OSDI*, 1999.
3. E. Buchman. "Tendermint: Byzantine Fault Tolerance in the Age of Blockchains." M.Sc. thesis, University of Guelph, 2016.
4. J. H. Saltzer, M. D. Schroeder. "The Protection of Information in Computer Systems." *Proc. IEEE*, 63(9), 1975.
5. S. Rose, O. Borchert, S. Mitchell, S. Connelly. "Zero Trust Architecture." NIST SP 800-207, 2020.
6. W3C. "Web Authentication (WebAuthn) Level 2." W3C Recommendation, 2021.
7. J. Samuel, N. Mathewson, J. Cappos, R. Dingledine. "Survivable Key Compromise in Software Update Systems." *Proc. ACM CCS*, 2010 (TUF).
8. S. Torres-Arias et al. "in-toto: Providing Farm-to-Table Guarantees for Bits and Bytes." *Proc. USENIX Security*, 2019.
9. Open Policy Agent. https://www.openpolicyagent.org
10. SPIFFE/SPIRE. https://spiffe.io
11. Sigstore. https://sigstore.dev
12. Anthropic. "Model Context Protocol." https://modelcontextprotocol.io, 2024.
13. Google. "Agent2Agent (A2A) Protocol." 2025.
14. R. C. Merkle. "A Digital Signature Based on a Conventional Encryption Function." *Proc. CRYPTO*, 1987.
15. D. J. Bernstein et al. "High-Speed High-Security Signatures." *J. Cryptographic Engineering*, 2(2), 2012 (Ed25519).
16. Hammerspace. "The Data Platform for AI Anywhere." https://hammerspace.com
17. Nasuni. "The Unstructured Data Platform for Enterprise Teams and AI." https://nasuni.com

---

*g8e is open source under the Apache 2.0 license. Source: https://github.com/g8e-ai/g8e*
*Built by Lateralus Labs — https://lateraluslabs.com*
