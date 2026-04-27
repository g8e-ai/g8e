# AI-Powered, Human-Driven Infrastructure

---

## Abstract

The current debate about agentic AI has converged on two architectures, both of which fail at infrastructure scale. Fully autonomous agents act without human verification of intent or context, producing systems that are technically correct and contextually wrong. Human-in-the-loop bolt-ons treat human approval as a downstream check on machine decisions, producing alert fatigue, asymmetric cost structures, and false safety. We propose a third architecture: **co-validated infrastructure**, in which AI agents and human users are first-class validators in a Byzantine consensus protocol, each handling the class of judgment they alone are qualified to provide. We instantiate this architecture as a coupled system of two components: a stateless reasoning engine running a heterogeneous-persona consensus mechanism, and a single-binary sovereign Operator that executes verdicts locally with tamper-evident audit. The system minimizes one quantity — the user's time — and holds two cryptographic invariants — every action is verifiable from the host owner's own ledger, and no command executes without both machine-domain and human-domain validation. We argue this architecture is not just better than current agentic systems; it is the only architecture that can scale to operating real infrastructure without surrendering either human sovereignty or auditability.

---

## 1. Two failure modes

Agentic AI in 2026 is dominated by two architectural families.

**The autonomous family** treats the AI as a sufficient agent: given a goal, the model plans, acts, and reports. Human oversight is post-hoc, if it exists at all. This family has produced impressive demonstrations on benchmarks but has not produced reliable infrastructure. The reason is structural: a model can verify *internal consistency* of its plan, but it cannot verify *contextual fidelity* — whether the plan matches what the user actually wanted in the user's actual environment, including the parts the user did not articulate. Every catastrophic agent failure in production has this shape: the agent did exactly what it understood the request to mean; the user meant something else; nobody checked the gap.

**The human-in-the-loop family** retrofits oversight by inserting an approval prompt before every state-changing action. This family has produced the dominant pattern in commercial agent systems. It has also produced the dominant failure mode: *alert fatigue*. When humans are asked to verify hundreds of agent decisions per day, they rubber-stamp. When the cost of careful verification is high (read the diff, understand the side effects) and the cost of approval is low (one click), the equilibrium is approval-without-verification. The human is nominally in the loop and substantively absent.

Both families share a deeper error: they treat the human and the machine as substitutable validators on the same questions. The autonomous family says the machine can do the human's job. The human-in-the-loop family says the human can do the machine's job. Both are wrong. The human and the machine are good at different things, and a system that conflates their competencies will fail in characteristic ways depending on which it favors.

## 2. The third path: co-validation

We propose that the human and the AI are not redundant validators on the same questions, but **co-validators on different questions**, neither of which can substitute for the other.

The machine handles what is **machine-checkable**:
- Internal consistency between intent, plan, and action
- Procedural correctness of multi-step reasoning
- Pattern-match safety against historical precedent
- Falsifiability of cited evidence
- Cross-conversation memory and grounding

The human handles what is **only human-checkable**:
- Intent fidelity at the deepest level — whether the action matches what they meant in their world, including unarticulated context
- Contextual stakes specific to their environment
- Acceptance of real-world consequences they alone will live with
- Implicit values the agent layer cannot access

Both signatures are required because the union of their competencies is what constitutes "safe to execute." Neither alone is sufficient. This is the architectural commitment from which everything else follows.

The economic implication is precise: **the user's time is not a free resource the system can spend at will, but a stake the user contributes in exchange for the service only they can validate.** Every component upstream of human judgment exists to minimize what reaches the user, so that what does reach them is exclusively the human-domain question they alone are qualified to answer. This reframes the user's experience: they are not babysitting an agent; they are providing the irreducible input the system structurally cannot generate.

## 3. Architecture overview

The proposed architecture has two coupled components:

**The Engine** is a stateless reasoning system that runs a Byzantine consensus protocol over heterogeneous AI personas. It produces verdicts: candidate commands with cryptographic attestations of their reasoning history.

**The Operator** is a single-binary sovereign execution layer that runs on every managed host. It receives verdicts from the Engine, performs local risk assessment, requires human approval at the execution boundary, executes approved commands, and maintains a tamper-evident local audit ledger.

The Engine is replaceable. The Operator is the system of record. This inversion is the architectural payload of the proposal: the AI layer can be swapped, audited, or revoked without losing history, because history lives on the host that owns the infrastructure being operated, not in the cloud where the AI runs.

```
        ┌─────────────────────────┐
        │       Engine            │
        │  (stateless reasoner)   │
        │                         │
        │  Triage → Sage →        │
        │  Tribunal → Auditor     │
        └────────────┬────────────┘
                     │
        intent + verdict + grounding
                     │
                     ▼
        ┌─────────────────────────┐         ┌──────────────┐
        │       Operator          │ ──prompt──▶│              │
        │  (sovereign executor)   │         │     User     │
        │                         │ ◀─approval──│   (human)    │
        │  Warden → Exec → Audit  │         │              │
        │       Vault + Git       │         └──────────────┘
        └─────────────────────────┘
                per-host,
                local-first
```

The User is external to both the Engine and the Operator. The Operator hosts the approval prompt and receives the User's signature, but the User is not a sub-component of the Operator. Co-validation occurs at the boundary between the Operator and the User, with both signatures required for a state change.

Communication is outbound-only from the Operator to the Engine over mTLS WebSocket. The Operator initiates every connection. No inbound port is required on managed hosts. Stolen credentials cannot be used from a different machine, because system fingerprint binding ties session credentials to the host they were issued on.

The remainder of this paper develops the two halves of this architecture and the integration that couples them.

## 4. The Engine: time-bonded heterogeneous consensus

The Engine implements a consensus mechanism in which AI personas are validators with reputation stakes and the human user is a co-validator whose stake is time. The full mechanism design is treated formally in a companion document; we give here the architectural and motivational sketch.

### 4.1 The Vortex Principle

The Engine's consensus mechanism is built on **tiered information quarantine**, which we call the Vortex Principle. Each agent operates in a sealed information environment, believing it is playing a smaller game than it is actually in. The interrogator does not know the planner exists. The planner does not know the auditor has cross-conversation memory. The validator panel does not know which member among them is the calibrated adversary. Only one role — the Auditor — has full visibility, and the Auditor is bonded most heavily and subject to peer review.

The Vortex is load-bearing. Each quarantined view eliminates the deviation strategies that would otherwise be profitable: agents cannot coordinate to game the consensus because they cannot identify their counterparties; agents cannot shape their output to fit a known downstream expectation because they cannot see the downstream. The information-theoretic structure is what makes the consensus produce honest votes. Collapsing any quarantine layer creates a profitable deviation. This is why the Vortex is a *safety mechanism*, not a UX choice.

### 4.2 The Tribunal

The Engine's consensus round is run by a five-member Tribunal of LLM-instantiated personas, each with a distinct lens:

- **Axiom** (composition): pressure for clean multi-stage pipelines
- **Concord** (safety): pressure for defensive flags and read-only discipline
- **Variance** (edge cases): pressure for robustness against locales, spaces, nulls
- **Pragma** (convention): pressure for idiomatic OS-specific tools
- **Nemesis** (calibrated adversary): proposes flawed-but-plausible commands to stress the Auditor

Each member emits a candidate command independently with no visibility into the others. Candidates are clustered by exact match and votes are tallied. A winner requires ≥2 of 5 supporting votes. If consensus is weak, members enter Round 2 with anonymized peer review and may converge or hold.

The Nemesis is the system's most novel element. Rather than relying on an external red team, we embed a calibrated adversary inside the consensus pool, scored on a proper scoring rule: attacks on flawed consensus that the Auditor confirms earn large gains; abstentions on clean consensus earn small gains; false alarms and misses lose stake. The dominant strategy is honest calibration. The realized attack rate becomes a learned signal of actual flaw density in the honest four's output — adversarial pressure as continuous quality measurement.

### 4.3 The Auditor

A single primary-tier Auditor reviews the winning candidate against Sage's articulated intent and the Tribunal's grounding. The Auditor can approve, swap to a dissenting cluster's candidate, or revise minor flaws. The Auditor's verdict is cryptographically committed to the reputation ledger.

The Auditor handles only machine-domain validation: consistency, grounding, procedural correctness, pattern-match safety against cross-conversation precedent. The Auditor's competency does not extend to user-domain judgments. Attempting to expand it there would force the Auditor to score against ground truth it cannot access, breaking the proper-scoring-rule structure that makes the Auditor's stake meaningful.

The Auditor is the system's most concentrated point of trust and is bonded accordingly: 2–3× any Tribunal member's stake. An attacker seeking to capture the Auditor must outbid more than they could gain from any single corrupted verdict. Peer-Auditor re-judgment of sampled past verdicts provides Byzantine fault tolerance against a single corrupted Auditor instance.

### 4.4 Time as the user's stake

The user is the second co-validator and stakes time. Time is non-fungible, non-recoverable, and unilaterally priced by the staker through their participation choice — which makes it a self-priced bond. The mechanism's central economic asymmetry is that AI stakes are recoverable and time isn't. Slashing reputation costs an LLM nothing; it costs the orchestration layer a routing-weight update. Slashing time costs the user a piece of their life they cannot get back. This asymmetry is what couples the mechanism to actual welfare.

The user has no explicit knowledge of the staking system. Their participation is a revealed-preference bond: tight messages and answered questions reduce time-to-resolution; vague messages and ignored questions extend it. The user does not need to understand the mechanism to play it correctly. The gradient teaches them.

The system minimizes one quantity:

```
U_user = -τ_total - λ · 𝟙[execution_failure]
```

Every other reward is a gradient pointer toward this objective. The Engine is doing its job when both terms are low.

## 5. The Operator: sovereign execution

The Operator is the data plane and the system of record. It is implemented as a single statically compiled Go binary of approximately 4 MB. It runs in one of four mutually exclusive modes — Standard (per-host execution), Listen (platform persistence), OpenClaw (gateway integration), and Stream (fleet deployment) — but all modes share the same binary, the same code paths, and the same security posture.

### 5.1 Why a single binary

Most agent platforms decompose into a service mesh: an API gateway, a queue, a storage tier, an execution worker, an audit collector. Each component is a separate process with separate dependencies, separate failure modes, and separate attack surface. The operational complexity is such that nobody self-hosts these systems; they buy them as SaaS.

The Operator collapses this stack into one binary that can play every role. The same binary that executes commands on a managed host can, in listen mode, serve as the platform's persistence layer for the Engine. The same binary that listens for commands can, in stream mode, deploy itself to a fleet of remote hosts over SSH using pure Go cryptography with no shell-out. The friction to bring a host into this architecture is one curl command.

This is not a convenience. It is a precondition for sovereignty. A platform that requires a service mesh requires a platform team to operate it. A platform that requires a platform team requires SaaS economics to amortize that team. SaaS economics require centralizing data on the vendor's infrastructure, which surrenders the property — local-first audit — that the architecture exists to preserve. The single-binary form is what makes the rest of the proposal economically tenable for the entity that owns the infrastructure being operated.

### 5.2 Outbound-only architecture

The Operator initiates every connection. No inbound port is required on managed hosts. The Engine does not reach into the Operator; the Operator reaches out to the Engine, authenticates with a per-operator mTLS certificate, and subscribes to a command channel scoped by its operator and session identifiers.

The security implications:
- Hosts behind NAT, in private VPCs, or behind strict egress firewalls can be operated without exposing inbound surface
- Stolen API keys cannot be used from a different machine — the system fingerprint binding ties session credentials to the host they were issued on
- A compromised Engine cannot push commands to an Operator the attacker has not already compromised at the host level
- The Engine has no list of hosts to scan or attack; the topology is owned by the hosts themselves

mTLS is enforced on every connection in both directions. Both sides present certificates. There is no asymmetric trust relationship.

### 5.3 Local-first audit

Every action — every message, every articulated intent, every Tribunal candidate, every Auditor verdict, every human approval, every executed command, every result — is persisted in an encrypted SQLite audit vault on the managed host. Every file mutation is committed to a local Git repository, providing immutable version history and rollback capability. The Operator is the system of record; the Engine is a stateless relay.

This inversion is the architectural payload. Most agent platforms hold authoritative state in the cloud and project a view of it onto the host. We hold authoritative state on the host and project a view of it to the cloud. The differences:

- The host owner can audit every action against their own ledger without trusting the platform vendor's logs
- The platform vendor can be replaced or audited without losing history
- A platform compromise cannot rewrite history because history lives on hosts the attacker does not own
- Compliance regimes that require data sovereignty (e.g., EU GDPR, sectoral regulations) are satisfied by default rather than by exception

The audit vault is encrypted at rest with a session key derived at Operator startup. The Git ledger is structurally append-only and integrity-verifiable through standard Git tooling. Tamper evidence does not require the platform to be honest.

### 5.4 Zero Standing Privileges and intent-based IAM

For Cloud Operators on AWS, the Operator implements Zero Standing Privileges: the Operator holds no permanent IAM credentials. When the Engine articulates an intent that requires AWS access, the Operator generates a session-scoped IAM role with permissions derived from the intent itself — read-only when the intent is read-only, write-scoped to the specific resource when the intent is write — and assumes that role only for the duration of the executing command.

This is the architectural complement to the Engine's intent articulation. Sage produces an intent in natural language. The Tribunal translates intent into a candidate command. The Auditor verifies the command matches the intent. The Operator translates the intent into the minimum IAM scope sufficient to execute the command. The same intent document is the input to every layer's contract.

The result is that no human and no AI agent ever needs to hold privileged AWS credentials at rest. Privileges are minted just-in-time, scoped to the specific action, and dissolved on completion. A compromise of any layer — the human's session, the Engine's reasoning state, the Operator's binary — cannot exfiltrate persistent credentials, because no persistent credentials exist.

### 5.5 The Warden

The Operator runs the Warden, a defensive coordinator that performs pre-execution risk assessment. The Warden classifies command risk (low/medium/high), file operation risk (factoring in Git state — operations that lose history are higher risk than reversible ones), and analyzes failures for auto-fix safety. The Warden's classifications populate the human approval prompt with concrete risk indicators, making the human's co-validation more efficient.

The Warden fails closed. Ambiguous risk is classified as high. The Warden cannot lower a classification produced by deterministic pattern filters (e.g., `rm -rf /` is high regardless of AI judgment). The human is presented with the highest classification any layer produced.

## 6. The integration: from verdict to state change

The flow from user message to state change traverses both halves of the architecture.

1. **Engine — Triage and routing.** The user's message is classified for complexity, intent, and posture. Simple requests route to a fast-path responder. Complex requests route to Sage.
2. **Engine — Information acquisition.** Dash issues batches of three yes/no questions, scored on realized information value. The user's answers populate the ledger as structured context.
3. **Engine — Intent articulation.** Sage produces an intent document — goals, constraints, success criteria — in natural language only. Sage never writes shell syntax.
4. **Engine — Tribunal consensus.** Five personas produce candidate commands in parallel. Votes are tallied. If consensus is weak, Round 2 with anonymized peer review.
5. **Engine — Auditor verdict.** The winning candidate plus dissenting clusters go to the Auditor with full grounding. The Auditor approves, swaps, or revises. The verdict is cryptographically committed to the reputation ledger.
6. **Operator — Warden risk assessment.** The Operator receives the verdict and classifies command, file, and error risk locally on the target host.
7. **Operator — Presentation to User.** The Operator presents the proposed command, the Auditor's grounding, the Warden's risk assessment, and any dissenting Tribunal candidates expandable on request.
8. **User — Co-validation.** The User reviews the presentation and provides the human-domain signature: approve, reject, or request revision. This step is performed by the User, not by the Operator; the Operator hosts the prompt but does not produce the signature.
9. **Operator — Execution.** Approved commands execute in the Operator's working directory under just-in-time scoped privileges. Output is captured, scrubbed, and returned to the Engine for Sage's next reasoning step.
10. **Operator — Audit commitment.** The full transaction — message, intent, candidates, verdict, Warden assessment, User approval, execution result — is written to the encrypted audit vault. File mutations are committed to the Git ledger.
11. **Engine — Background learning.** Codex extracts user preferences and investigation summaries asynchronously, updating cross-conversation memory accessible only to the Auditor.

The actors are three: the Engine (reasoning), the Operator (sovereign execution and audit), and the User (human-domain co-validation). The User is not a sub-component of the Operator. Steps 6, 7, 9, and 10 are performed by the Operator; step 8 is performed by the User; the Operator's role at step 8 is solely to present information and receive the User's signature. Both signatures — the Auditor's at step 5 and the User's at step 8 — are required before execution at step 9.

## 7. Why this is the future of infrastructure

Most discussions of agentic AI proceed as if the question is whether AI agents will become more capable. We accept that they will. Our claim is more specific: **as agents become capable enough to act on real infrastructure, the architectures that surround them must change, and the architecture we propose is the one that survives the transition.**

Infrastructure has properties that consumer AI applications do not. State changes are persistent and often irreversible. Mistakes have blast radius beyond the initiating user. Compliance regimes require auditability, sovereignty, and isolation. Security models assume that any connection is a potential attack and that any credential is a potential compromise. Operational economics require that the marginal cost of bringing a host into the system is low.

The autonomous-agent architecture fails on every infrastructure axis. It is unauditable (no record of why decisions were made), insovereign (data flows to the AI vendor), uncompliant (no native handling of sectoral regulations), insecure (long-lived credentials, broad attack surface), and economically unviable at scale (review costs grow with action volume).

The human-in-the-loop architecture fails on the human axis. It treats the human as a free resource and produces alert fatigue, which converges to autonomous behavior with the appearance of oversight.

The co-validated architecture is the only one we are aware of that simultaneously:

- **Preserves human sovereignty** over data and decisions through local-first audit
- **Preserves AI productivity** by routing only human-domain questions to the human
- **Provides cryptographic auditability** through the reputation ledger and Git history
- **Operates without long-lived credentials** through intent-based IAM and system fingerprint binding
- **Scales economically** through the single-binary Operator and the time-minimization objective
- **Degrades safely** by failing closed at the Warden and requiring two signatures for every state change

We do not claim this architecture is finished. We claim it is correct in shape. The elements may be refined; the structure — engine + operator, machine-domain + human-domain validation, time as the user's stake, local-first as the system of record — is the structure infrastructure will require.

## 8. Open questions and future work

We are honest about what we have not yet resolved.

**Multi-user consensus.** The current mechanism treats the user as a single co-validator. Real infrastructure is operated by teams. Extending the mechanism to multi-user environments — where different users have different `λ` values, different intent priors, and conflicting authority — is unresolved. The likely path is delegation with bounded authority: a primary user co-validates by default, with escalation to additional validators for higher-risk verdicts.

**Auditor convergence under distribution shift.** The Auditor's grounding accuracy is presumed to converge through peer-Auditor sampling. We do not yet have empirical bounds on the convergence rate, nor do we have characterizations of the failure modes when the Auditor encounters tasks systematically outside its training distribution. This is the most important open empirical question.

**Pathological users.** Users who optimize for low time-to-resolution at any cost — skipping every Dash question, rubber-stamping every verdict — can starve the mechanism. The current design assumes time-rational users; pathological users break the proxy chain. Whether the gradient educates them out of pathological play quickly enough to bound damage is an empirical question we are actively measuring.

**Operator-to-Operator coordination.** The current architecture treats each Operator as an isolated execution domain. Workflows that span hosts (e.g., a database migration that must be coordinated across replicas) currently rely on the Engine to sequence operations across Operators. A more interesting future is direct Operator-to-Operator coordination over a shared consensus substrate, which would extend the co-validation model to distributed operations. This is genuine future work and would require the Byzantine consensus extension we have so far avoided.

**Formal guarantees.** The mechanism design is presented as a sketch with equilibrium claims supported by intuition rather than proof. A formal Bayes-Nash equilibrium proof under specified information structures is in scope for a follow-up paper.

## 9. Conclusion

The infrastructure of the future will be operated by AI agents. This is not in dispute. What is in dispute is whether the humans who own that infrastructure will retain sovereignty over it, whether the actions taken on it will be auditable, and whether the operational economics will be tenable for entities other than hyperscale cloud vendors.

We have proposed an architecture that says: yes to all three, but only by abandoning two architectural patterns the field has converged on. We must abandon the assumption that human and machine validators are substitutable; they are not, and conflating them produces the failures we observe in current systems. We must abandon the assumption that authoritative state belongs in the cloud; it belongs on the host that owns the infrastructure, and the AI layer should be a stateless relay over it.

What replaces those assumptions is co-validation: AI agents and humans as first-class validators on different classes of judgment, coupled through a Byzantine consensus protocol with cryptographic audit, executed by a sovereign single-binary Operator that holds the system of record on the host being operated. The user's time is the dominant stake. The Engine is replaceable. The Operator is the truth. The architecture is AI-powered and human-driven, with the boundary between those two adjectives drawn precisely where the competencies actually divide.

We do not propose this as one option among many. We propose it as the shape that infrastructure will take, because it is the only shape that survives the constraints infrastructure places on agentic systems.

---

## Companion documents

- *Mechanism Design for Time-Bonded Heterogeneous Consensus* — formal treatment of the Engine's consensus mechanism, payoff functions, and equilibrium claims.
- *Operator Architecture Reference* — implementation specification of the Operator binary, modes, security posture, and HTTP/WebSocket APIs.
- *Vortex Principle: Information-Theoretic Foundations of Honest Consensus* — derivation of the quarantine structure that makes the consensus mechanism's equilibrium hold.
