**Byzantine Fault Tolerance at the Execution Boundary: Securing Agentic Infrastructure**

*Danny Barbour*
*May 2026*

### Abstract

The industry is rapidly converging on standardized data pipes and routing protocols - such as Anthropic’s Model Context Protocol (MCP) and open Agent-to-Agent (A2A) standards - to connect Large Language Models (LLMs) to production environments. However, treating infrastructure mutation as a simple JSON-RPC tool call is operational malpractice. Autonomous systems are inherently vulnerable to auto-regressive collapse, hallucination, and sycophancy. In high-stakes environments, agentic execution is not a prompt engineering challenge; it is a Byzantine Fault Tolerance (BFT) problem.

This paper outlines a mutual-adversary governance architecture. We propose a strict physical separation between intent generation, communication protocols, and host execution. By forcing all state-changing payloads into a cryptographically signed, state-bound `GovernanceEnvelope`, we ensure that no autonomous system can access reality without structural consensus, host-local verification, and explicit, hardware-bound human authorization.

---

### 1. The Operational Reality of Agentic AI

Current agentic architectures optimize for capability and developer velocity. An LLM reasoning loop is given a set of executable tools, context is piped in via protocols like MCP, and the model autonomously invokes functions via JSON-RPC.

In read-only environments, this is highly effective. In state-changing infrastructure, it is a severe anti-pattern.

A single LLM is a non-deterministic probabilistic text generator. If it hallucinates a faulty assumption early in its reasoning chain, it will mathematically justify its own mistake across thousands of subsequent tokens. Retrofitting safety via a Human-in-the-Loop (HITL) confirmation dialog merely shifts liability to a fatigued operator who lacks the context to manually verify the blast radius of a generated script.

Furthermore, protocols like MCP and A2A are designed to standardize the *payload* (the context fetch or the tool execution). They do not provide execution governance. If an architecture pipes unstructured JSON-RPC directly into a root shell or a cloud API, it is operating entirely on implicit trust.

### 2. A Mutual-Adversary Architecture

To safely deploy autonomous agents, we must discard implicit trust. The system must operate on a **mutual-adversary model**, assuming that the AI control plane is compromised by default, that human operators are prone to fatigue, and that the execution environment must fail-closed.

#### 2.1 Mutual-Distrust Boundaries

Every component treats every other component as a potential adversary. There is no "trusted internal network", no "trusted model", no "trusted operator". Boundaries are enforced cryptographically and structurally.

**The Principal (User) does not trust:**

- *Any single AI provider or model.* The Principal binds heterogeneous providers and tiers (primary / lite / assistant) across the reasoning agents so an adversarial or hallucinating provider cannot collude with itself across the cascade. Tribunal members, Warden's risk analyzers, and Auditor are independently configurable.
- *Any host running an Operator.* mTLS with URI-SAN workload identity, system fingerprinting, device-link tokens with bounded `max_uses` and TTLs, slot-count accounting, API-key rotation, and revocation are enforced at the Operator listener, not by the AI layer.

**The Engine (g8ee) does not trust:**

- *The user.* L1 Technical Bedrock (forbidden patterns, blacklist, whitelist) blocks dangerous instructions before any model sees them. The Engine refuses to compromise infrastructure for any single user request regardless of the user's authority over the conversation.
- *The Operator.* Operator output passes through Sentinel ingress scrubbing (PII, credentials, tokens) before any AI sees it. The Engine speaks to the Operator only over mTLS, only via scoped sessions (`web_session_id` / `cli_session_id` / `operator_session_id`), and never reads raw stdout/stderr - only the scrubbed projection.

**The Operator (g8eo) does not trust:**

- *The user or the AI.* The Operator opens no inbound ports; all client-facing traffic arrives over mTLS to its `--listen` mode. Every mutation crosses the protocol admission boundary: `GovernanceEnvelope` integrity, typed-payload decode, L1 reflected forbidden patterns, hash binding, freshness (`expires_at` + nonce), state-root match, L2 Tribunal signature against a trusted signer, and (for mutations) L3 WebAuthn proof. Sentinel performs egress scrubbing on every result before any byte leaves the host.

**The Engine's internal pipeline does not trust itself.** No single agent inside g8ee holds both intent authority and execution authority. The cascade in §2.3 forces every state-changing instruction through an ensemble of independent, ideologically opposed agents before it is even *eligible* for the protocol gauntlet.

#### 2.2 The Four Architectural Components

This architecture isolates the system into four distinct components:

**I. The Principal (The Intent)**
The entity requesting the state change. Today, this is a human operator. As systems scale, the Principal will increasingly be an upstream AI agent passing a high-level request down the chain.

* **The vulnerability:** Principals provide ambiguous, context-blind, or highly dangerous instructions.
* **The constraint:** The Principal holds the hardware-bound FIDO2 passkey. The system cannot execute a mutation without this cryptographic Proof of Human Presence (PHP) anchoring the final intent.

**II. The Engine (The Orchestrator)**
The reasoning layer. Instead of a single agent, the Engine utilizes a structurally blind multi-agent ensemble (the Tribunal) operating in a ReAct loop.

* **The vulnerability:** Single agents suffer from sycophancy and auto-regressive collapse.
* **The constraint:** The Engine is strictly stateless regarding the host. It cannot execute anything. It must reach a plurality consensus across ideologically opposed agents (including a calibrated adversary) and attach an Ed25519 signature to its proposed command string before transmitting it.

**III. The Protocol (The Wire Contract)**
The governance boundary. The protocol is not a data pipe; it is an armored transport envelope (`GovernanceEnvelope`).

* **The vulnerability:** Raw tool calls lack state awareness and cryptographic proof.
* **The constraint:** The protocol binds the typed payload to a deterministic transaction hash, the L1/L2/L3 signatures, and the `state_merkle_root` of the target host.

**IV. The Operator (The Reality Portal)**
The host-side binary. The Operator is the sovereign system of record and the physical execution boundary.

* **The vulnerability:** Upstream AI or compromised transit networks injecting malicious payloads.
* **The constraint:** The Operator distrusts all inbound traffic. It enforces L1 hard-gates (e.g., regex blocking `sudo`), verifies the L2 Tribunal signature, demands the L3 Human signature, and checks the transaction's state root against the host's actual state to prevent the execution of stale context.

#### 2.3 The Engine-Internal Byzantine Cascade

Within the Engine, every turn that could touch a host flows through a layered cascade of agents that are individually replaceable and pairwise distrusted. The cascade is implemented in `services/g8ee/app/services/ai/` and orchestrated by `generator.py`:

1. **Triage** (lite tier, classifier only). Reads each user message and emits structured `complexity / intent / posture / confidence` metadata. Triage *cannot* execute, ask questions, or call tools - a confident error here only mis-routes the model tier; it cannot leak state.
2. **Dash** (assistant tier) handles `simple` turns directly. **Sage** (primary tier) handles `complex` turns. Either can pause the loop with an `<interrogation>` block of three binary YES/NO questions; tool execution is suppressed for that turn until the user answers. Sage never writes shell syntax; it articulates *intent*.
3. **Tribunal generation.** When Sage (or Dash) calls a host-mutation tool, the call is intercepted and the natural-language intent is forwarded to a five-member ensemble: **Axiom** (composition), **Concord** (safety), **Variance** (edge cases), **Pragma** (convention), **Nemesis** (calibrated adversary). Each member runs in isolation - the *Amnesia Principle*: no member sees another's candidate, no member knows which seat Nemesis occupies. Each emits exactly one shell command string. Reputation is staked on every emission.
4. **Tribunal voting.** Candidates are aggregated under uniform 1-vote-per-member weighting. Minimum consensus is 2 of N (default 5). If consensus is reached, the winner advances. Tie-breaks apply in order: shortest command, non-Nemesis cluster wins over Nemesis-including cluster, alphabetical fallback. **Note**: Nemesis votes are *not* auto-discarded - they only lose tie-breaks. Slashing applies if Nemesis raised a false flag or abstained on a real flaw.
5. **Round 2 (anonymized peer review)** runs only when Round 1 fails to reach consensus. Members see anonymized R1 clusters and re-emit. If R2 also fails, the pipeline raises `TribunalConsensusFailedError` and Sage is given the failure context to re-articulate intent.
6. **Warden risk analysis** runs *before* Auditor. The Warden coordinator orchestrates specialized sub-agents (command-risk, error, file-risk) that classify the consensus winner as LOW / MEDIUM / HIGH. **Two-Strike Circuit Breaker**: a first HIGH classification is returned to Sage with contextual feedback so Sage can re-articulate; a second HIGH in the same investigation raises `AGENT_CONFLICT_DETECTED` and forces human intervention. Successful execution resets the strike counter.
7. **Auditor verification.** If Warden clears the command, the Auditor (primary tier) sees the request, the operator context, and the anonymized candidate clusters. Auditor verdicts are `ok`, `swap:<cluster_id>` (promote a dissenter), or `revised:<command>`. Auditor's job is intent fidelity and syntactic integrity; it judges the command, not the source. On `ok` or `swap`, the verdict is bound to a SHA-256 **Merkle commitment** over the agent reputation scoreboard, chained via `prev_root` HMAC-SHA256 - a tamper-evident history of agent performance. Reputation-commitment failure is fatal; the verdict cannot proceed.
8. **L1/L2/L3 wrapping.** The Auditor-verified command is packaged as a typed `CommandRequested` payload inside a `GovernanceEnvelope`, signed by the L2 Tribunal key, and presented to the user for L3 WebAuthn approval (or auto-approval policy if the verb is in `auto_approved.json` and L1/L2 cleared).
9. **Substrate admission gauntlet.** The signed envelope is submitted over mTLS to the Operator, which independently re-runs the entire fail-closed gauntlet (§3). The Engine has no privileged channel.
10. **Warden execution + signed receipt.** Inside the Operator, the substrate Warden (distinct from the AI Warden) is the *only* execution boundary. It writes an executing-state `ActionReceipt` to the audit vault before dispatch and a final receipt with `state_root_after` after dispatch.

**Key invariant.** Any single agent in this cascade can be replaced or removed for an experiment, but the *signed-envelope substrate gauntlet* (§3) is non-negotiable. If the Engine's internal cascade is compromised end-to-end - e.g. a single adversarial provider supplied every model tier - the Operator still rejects the resulting envelope at L1 (forbidden patterns) or at the state-root / freshness / signer-trust gates.

### 3. Payload vs. Envelope: Interoperability with MCP

This architecture does not replace context standards like MCP; it secures them.

MCP is highly effective at standardizing *what* a tool call is. But an MCP server natively executing those calls possesses no structural defense against an AI logic failure.

In a mutual-adversary architecture, MCP is treated strictly as the **Payload**, and the host Operator is the **admission boundary**. The execution substrate does not "wrap" the payload as a passive courier; it ingests the payload at its perimeter and refuses to let it cross into the application layer - much less the host - until the surrounding `GovernanceEnvelope` proves itself against every conformance gate.

When an LLM client issues a standard MCP `call_tool` request, the gateway intercepts the JSON-RPC message, normalizes the arguments into a base64 Protobuf payload, and binds it to a `GovernanceEnvelope`. The Operator's `TransactionVerifier` then forces that envelope through an ordered, fail-closed gauntlet *before any application code runs*:

1. **Envelope integrity** - the JSON envelope must decode; `id` and `action_type` must be present; `action_type` must be registered.
2. **Typed payload binding** - `payload` must decode as the protobuf message declared by `action_type`. Untyped or shape-mismatched payloads are rejected.
3. **L1 forbidden patterns** - reflected scan of the typed payload's fields against the L1 denylist (e.g., `sudo`, raw destructive verbs).
4. **Hash binding** - `transaction_hash == SHA256(canonical_fields)` *and* `id == transaction_hash`. The signature basis is non-malleable.
5. **Freshness** - `expires_at` not in the past; `nonce` not in the replay store. Both are required.
6. **State binding** - `state_merkle_root` must match the Operator's current host state root. Stale-context transactions are rejected.
7. **L2 Tribunal signature** - `key_id` must resolve to a key in the trusted `SignerStore`; the Ed25519 signature over `transaction_hash` must verify. Untrusted signers are rejected even if the signature is internally valid.
8. **L3 human authorization** (mutations only) - a real WebAuthn `L3Proof` (clientDataJSON, authenticatorData, signature) using `transaction_hash` as the challenge must verify against a configured L3 verifier.

Only after every gate clears does the Operator admit the MCP payload to the Warden for native execution. If any gate fails, the verifier returns a typed `TX_*` sentinel, the substrate logs a blocked-transaction record to the audit vault, and the payload is dropped at the boundary. **The application layer (Warden, execution handlers, downstream subsystems) is never invoked, and the host is never touched.** Conformance is the price of entry - there is no permissive path.

This separation of concerns allows engineering teams to leverage open standards for capability discovery while enforcing Byzantine Fault Tolerance at the execution boundary.

### 4. Execution as a Side-Effect of the Audit Log

In standard SaaS deployments, auditability is a telemetry stream emitted after the fact. In a zero-trust substrate, execution is a side-effect of the audit log.

The Operator implements a Local-First Audit Architecture (LFAA). Before a single bit flips on the host, the intent, the consensus proof, and the human signature are anchored to a host-local, AES-256-GCM encrypted SQLite vault. Furthermore, every file mutation utilizes a multi-ledger two-phase commit into an isolated Git repository on the host, generating a cryptographic hash of the pre- and post-execution state.

The AI control plane only receives metadata scrubbed of credentials and PII. The authoritative, cryptographic truth of the state change never leaves the host.

### 5. The Agent Fabric Vision

We are moving away from monolithic assistants toward an "Agent Fabric" - an interconnected mesh of micro-agents delegating tasks to one another across organizational boundaries.

If this fabric relies on implicit trust and naked tool calls, the blast radius of a single failure will cascade unpredictably.

A mutual-adversary substrate ensures that no matter how deep the AI-to-AI delegation goes, the ultimate interaction with physical reality remains governed. Every action is cryptographically audited on the local host, validated against the immediate state, and definitively rolls up to a human Principal who authorized the overarching intent.

### 6. Conclusion

Infrastructure engineering requires properties that consumer AI does not. Mistakes are persistent, and the cost of an hallucinated action is real. We cannot bolt safety onto agentic systems after the fact by simply adding a confirmation button to an MCP tool call.

We must build safety into the substrate. By treating AI execution as a consensus problem, physically separating reasoning from execution, and requiring cryptographic proof of state and intent, we can deploy autonomous systems into high-stakes environments without surrendering operational sovereignty.