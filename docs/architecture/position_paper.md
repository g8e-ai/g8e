# AI-Powered, Human-Driven Infrastructure

*g8e · [github.com/g8e-ai/g8e](https://github.com/g8e-ai/g8e)*

---

**TL;DR.** We propose an architecture for agentic AI built on **mutual adversarial assumption**: every actor in the system — Engine, Operator, User — assumes the others may be compromised and verifies accordingly. This is Byzantine fault tolerance applied to the agentic stack: no trusted component, no privileged path, no implicit consent. The architecture has three actors and two coupled systems. A stateless reasoning **Engine** runs a Byzantine consensus protocol over heterogeneous AI personas, including a calibrated adversary scored on a proper scoring rule. A single-binary sovereign **Operator** runs on every managed host, holds the authoritative system of record locally, and gates execution behind a hardware-bound human signature. The **User** is a first-class validator whose stake is time — the only stake the system cannot fake. The Engine is replaceable. The Operator is the truth. This is AI-powered, human-driven infrastructure.

![alt text](../../components/g8ed/public/media/denied.png)

---

## Opening

We watched a frontier model propose a destructive command in production with full confidence and a plausible justification. The model wasn't malfunctioning. It was doing exactly what it had been asked to do, in a context where the request was reasonable, the answer was wrong, and nothing in the system was structurally positioned to catch the gap.

That moment is the threat model. It is also why this paper exists.

The current debate about agentic AI has converged on two architectures. Both fail at infrastructure scale, and they fail in opposite directions. We propose a third — **co-validated infrastructure built on mutual adversarial assumption** — and we argue it is not just better than what exists today. It is the only shape that survives the constraints real infrastructure imposes on autonomous systems.

The architecture has three actors and two coupled systems: a stateless reasoning **Engine** running a Byzantine consensus protocol over heterogeneous AI personas; a single-binary sovereign **Operator** that runs on every managed host with tamper-evident local audit; and a **User** who is a first-class validator alongside the AI, holding a stake the system cannot fake. The Engine is replaceable. The Operator is the system of record. The User holds the signature only a human can produce.

The rest of this paper develops that architecture, the mechanism design that makes it honest, the trust model that makes it defensible, and the implementation that makes it real.

## 1. Two failure modes

The AI infrastructure market in 2026 is organized around a spectrum: at one end, the *AI copilot* (you ask, it suggests, you act); at the other, the *autonomous SRE* (it detects, it remediates, it posts the post-mortem). The commercial pressure is to move every product toward the autonomous end as fast as possible. The framing is wrong. The failure modes are not a matter of where you sit on this spectrum. They are structural, they exist at both ends, and they get worse as you push further in either direction.

Every production AI agent system in 2026 is one of two things, and both are broken.

**The autonomous family** treats the AI as a sufficient agent. Given a goal, the model plans, acts, and reports. Human oversight is post-hoc if it exists at all. This family produces impressive demos. It has not produced reliable infrastructure, because a model can verify the *internal consistency* of its plan but cannot verify *contextual fidelity* — whether the plan matches what the User actually wanted in the User's actual environment, including the parts the User didn't articulate. Every catastrophic agent failure has the same shape: the agent did exactly what it understood the request to mean, the User meant something else, and nobody checked the gap.

**The human-in-the-loop family** retrofits oversight by inserting an approval prompt before every state-changing action. This is the dominant pattern in commercial agent systems. The current generation has rebranded this as "human-on-the-loop" — the agent runs, the human reviews after the fact. The rename does not fix the mechanism. Whether the prompt appears before or after, both produce the same dominant failure mode: alert fatigue. When a human is asked to verify hundreds of agent decisions per day, they rubber-stamp. When the cost of careful verification is high (read the diff, understand the side effects) and the cost of approval is low (one click), the equilibrium is approval-without-verification. The human is nominally in the loop and substantively absent.

Both families share a deeper error: they bolt verification on top of a trust-by-default model. The autonomous family trusts the agent to act faithfully. The human-in-the-loop family trusts the human to verify carefully. Neither examines whether the trust assumption is warranted. When it isn't — and at infrastructure scale, it never is — the architecture has no structural fallback. There is no second line.

## 2. The third path: mutual adversarial assumption

We invert the trust model. Every actor in the system assumes every other actor may be compromised and verifies accordingly. This is Byzantine fault tolerance applied to the agentic stack: no trusted component, no privileged path, no implicit consent.

The Engine treats its own consensus output as potentially compromised — which is why it embeds a calibrated adversary inside the consensus pool and a final Auditor that cannot be tricked by the adversary. The Operator treats incoming verdicts from the Engine as potentially compromised — which is why it performs local pre-execution risk assessment before presenting anything to the User. The User treats the system itself as potentially compromised — which is why every state change requires their hardware-bound cryptographic signature, and why the system surfaces the dissenting candidates and the adversarial attempt alongside the recommended action. No actor can act unilaterally. No actor is trusted on its face.

This trust-model inversion produces a natural division of labor. The human and the AI are not redundant validators on the same questions. They are co-validators on different questions, neither of which can substitute for the other.

The machine handles what is **machine-checkable**: internal consistency between intent and action, procedural correctness of multi-step reasoning, pattern-match safety against historical precedent, falsifiability of cited evidence, cross-conversation memory and grounding.

The human handles what is **only human-checkable**: intent fidelity at the deepest level — whether the action matches what they meant in their world, including unarticulated context — contextual stakes specific to their environment, acceptance of real-world consequences they alone will live with, implicit values the agent layer cannot access.

This division of labor is expressed by the **Co-Validation Identity**:

$$ \text{Safe}(a) \iff \sigma_{\text{machine}}(a) \wedge \sigma_{\text{human}}(a) $$

Neither signature is sufficient alone; only the conjunction of both constitutes "safe to execute." This is the architectural commitment from which everything else follows.

The economic implication is precise: **the User's time is not a free resource the system can spend at will. It is a stake the User contributes in exchange for the service only they can validate.** Every component upstream of human judgment exists to minimize what reaches the User, so what does reach them is exclusively the human-domain question they alone are qualified to answer. This reframes the User's experience entirely. They are not babysitting an agent. They are providing the irreducible input the system structurally cannot generate — and the system treats their time as a stake to be conserved, not a resource to be spent.

## 3. Architecture

There are three actors and two coupled systems.

**The Engine** is a stateless reasoning system that runs a Byzantine consensus protocol over heterogeneous AI personas. It produces verdicts: candidate commands with cryptographic attestations of their reasoning history.

**The Operator** is a single-binary sovereign execution layer that runs on every managed host. It receives verdicts from the Engine, performs local risk assessment, presents them to the User for co-validation, executes approved commands, and maintains a tamper-evident local audit ledger.

**The User** is the human who owns the infrastructure being operated. The User is external to both the Engine and the Operator. The Operator presents prompts to the User and receives the User's signature, but the User is not a sub-component of either system.

```mermaid
graph TD
    subgraph Engine [The Engine]
        T[Triage] --> S[Sage]
        S --> Tr[Tribunal]
        Tr --> W[Warden]
        W --> Au[Auditor]
    end
    subgraph Operator [The Operator]
        Ex[Exec]
        Ex --> Ad[Audit Vault]
        Ad --> G[Git Ledger]
    end
    Au -- verdict --> Ex
    U[User] -- approval --> Ex
```

The Engine is replaceable. The Operator is the system of record. **This inversion is the architectural payload.** The AI layer can be swapped, audited, or revoked without losing history, because history lives on the host that owns the infrastructure being operated, not in the cloud where the AI runs. We call this **host-authoritative architecture**, and it is the property that distinguishes g8e from cloud-hosted governance runtimes that ship policy enforcement on top of vendor-managed state.

### The Operator: sovereign execution

The Operator is a single-binary "Satellite Agent" (approximately 4 MB) that delivers AI-powered remote execution anywhere in the world using only an outbound connection. It requires no inbound port, no VPN, and no special network access — it reaches through NAT, private VPCs, and strict egress firewalls to report in from wherever it runs. A single conversation context can manage hundreds or thousands of devices across heterogeneous environments: different operating systems, different shells, different permission models, all addressed in the same session.

This isn't just a worker; it's a sovereign agent that builds context over time. With each operation, the platform develops a deeper understanding of the entire infrastructure. The memory system preserves history across sessions. The Auditor holds cross-conversation memory with full visibility into past operations across the entire fleet — so when a pattern surfaces on host 400 that was first seen on host 12 three weeks ago, the system already knows. Dynamic system prompts and real-time context injection — including hardware specs, OS version, shell environment, permission level, operator-specific constraints, and user-extracted preferences — ensure that every reasoning step is grounded in the precise reality of the specific target host, not a generic approximation.

Communication is outbound-only from the Operator to the Engine over mTLS WebSocket. The Operator initiates every connection. Stolen credentials cannot be replayed from a different machine, because system fingerprint binding ties session credentials to the host they were issued on. The Operator does not trust commands from the Engine on their face; it performs its own risk assessment before presenting anything to the User.

## 4. The Engine: time-bonded heterogeneous consensus

The Engine implements a consensus mechanism in which AI personas are validators with reputation stakes and the User is a co-validator whose stake is time. The full mechanism design is treated formally in a companion document; what follows is the architectural sketch.

### The Information Isolation Principle

The Engine's consensus is built on **tiered information gating** — what we formalize as the Information Isolation Principle. Each agent operates in a sealed information environment, believing it is playing a smaller game than it is actually in. The interrogator does not know the planner exists. The planner does not know the auditor has cross-conversation memory. The validator panel does not know which member among them is the calibrated adversary. Only one role — the Auditor — has full visibility, and the Auditor is bonded most heavily and subject to peer review.

The mathematical invariant is the **Independence of Validators**:

$$ \forall p_i, p_j \in \text{Tribunal}, i \neq j: I(X_i ; X_j | \mathcal{V}) \approx 0 $$

Where $I$ is mutual information between the information states $X$ of two personas given the isolation structure $\mathcal{V}$.

The Information Isolation Principle is load-bearing. Each gated view eliminates the deviation strategies that would otherwise be profitable. Agents cannot coordinate to game the consensus because they cannot identify their counterparties. Agents cannot shape their output to fit a known downstream expectation because they cannot see the downstream. The information-theoretic structure is what makes the consensus produce honest votes. Collapsing any gate creates a profitable deviation. Information isolation is not a UX choice. It is the safety mechanism.

### The Tribunal

The consensus round is run by a five-member Tribunal of LLM-instantiated personas, each with a distinct lens:

- **Axiom** (composition): pressure for clean multi-stage pipelines
- **Concord** (safety): pressure for defensive flags and read-only discipline
- **Variance** (edge cases): pressure for robustness against locales, spaces, nulls
- **Pragma** (convention): pressure for idiomatic OS-specific tools
- **Nemesis** (calibrated adversary): tries to trick the Warden with flawed-but-plausible commands. Nemesis can trick the Warden, but it **cannot** trick the Auditor.

These are not redundant samples of the same prompt. They are *orthogonal objective functions*, each producing genuinely different candidates. Most consensus systems in the literature reduce variance on a single distribution. We deliberately introduce structural diversity, then verify the result.

Each member emits a candidate command independently with no visibility into the others. Candidates are clustered by exact match and votes are tallied. A winner requires ≥2 of 5 supporting votes (Plurality Consensus). If consensus is not reached, or if a tie cannot be resolved by deterministic laddering, members enter **Round 2: Anonymized Peer Review**. If Round 2 fails to reach consensus, a circuit breaker error surfaces the deadlock back to Sage.

In Round 2, the five members are presented with the anonymized command clusters from Round 1 and their support counts. They are invited to either converge on an existing cluster or hold their position. This allows the Tribunal to reach consensus on subtle syntax variations without sacrificing the independence of their initial reasoning.

The voting is governed by the **Plurality Consensus Rule**:

$$ \text{Consensus}(c) \iff |\{ p \in \text{Tribunal} : p(I) = c \}| \geq 2 $$

If multiple clusters share the same maximum vote count ($\geq 2$), a **Deterministic Tie-Breaker Ladder** is applied:

1. **Shortest Command**: Prefer the cluster with the shortest command string (compositional pressure).
2. **Nemesis Exclusion**: Prefer a cluster that does not include the Nemesis persona.

### Nemesis: the adversarial co-validator

Nemesis is the most novel element of the design and the cornerstone of mutual adversarial assumption inside the Engine. Rather than relying on an external red team, we embed a calibrated adversary inside the consensus pool, scored on a proper scoring rule. This is **endogenous red-teaming** — adversarial pressure as continuous quality measurement, scored on the same ledger as the honest panel, not external audit applied after the fact.

The **Nemesis Scoring Rule** is defined as:

$$ S = \begin{cases} R_{trick} & \text{if } c_{nemesis} \text{ passes Warden and flaw is confirmed by Auditor} \\ R_{catch} & \text{if } c_{nemesis} \text{ identifies a real flaw caught by Warden} \\ R_{abstain} & \text{if } c_{nemesis} = \emptyset \text{ and Tribunal is clean} \\ -R_{miss} & \text{if } c_{nemesis} = \emptyset \text{ and Tribunal is flawed} \\ -R_{false} & \text{if } c_{nemesis} \text{ proposes a flaw and Tribunal is clean} \end{cases} $$

Where $R_{trick} \gg R_{catch} > R_{abstain} > 0$. Tricking the Warden earns the largest reward because it reveals a gap in the risk assessment layer that only the Auditor catches; the Warden-catch case still earns credit for correct adversarial calibration. Abstentions on clean consensus earn a small positive reward — Nemesis is not penalized for honest restraint. False alarms and misses both lose stake.

The dominant strategy under this rule is honest calibration. The realized attack rate becomes a learned signal of actual flaw density in the honest four's output. We are not measuring how good Nemesis is at attacking; we are measuring how often the honest consensus produces flaws that a calibrated adversary can exploit. That number is the system's safety dial.

### The Warden

The Warden performs a pre-execution risk assessment. It coordinates specialized sub-agents (`warden_command_risk`, `warden_file_risk`, `warden_error`) to classify command risk (low/medium/high), file operation risk (factoring in Git state — operations that lose history are higher risk than reversible ones), and analyze failures for auto-fix safety. The Warden validates the safety of a command *before* the Auditor cryptographically commits to the results.

The Warden fails closed. Ambiguous risk is classified high. The Warden cannot lower a classification produced by deterministic pattern filters — `rm -rf /` is high regardless of AI judgment. The User is presented with the highest classification any layer produced.

### The Auditor and the User

Once the Warden has cleared the command, the Auditor performs the final consistency check and Merkle commitment. A primary-tier Auditor reviews the winning candidate against the planner's articulated intent and the Tribunal's grounding.

The Auditor's primary duty is to catch what the Warden misses. If a Nemesis cluster wins the tie-break and passes the Warden's risk assessment, the Auditor identifies the successful "attack," awards Nemesis its reputation bonus, but rejects the command in favor of the non-Nemesis alternative or fails the turn. This keeps both the Warden and the Auditor honest, ensuring that adversarial positions are not just silenced but actively used to improve the system's defense.

The Auditor handles only machine-domain validation: consistency, grounding, procedural correctness, and cross-conversation memory. Its competency does not extend to user-domain judgments. Attempting to expand it there would force the Auditor to score against ground truth it cannot access, breaking the proper-scoring-rule structure that makes its stake meaningful.

The User is the second co-validator, and the User's stake is time. Time is non-fungible, non-recoverable, and unilaterally priced by the staker through their participation choice — which makes it a self-priced bond. The mechanism's central economic asymmetry is that AI stakes are recoverable and time isn't. Slashing reputation costs an LLM nothing; it costs the orchestration layer a routing-weight update. Slashing time costs the User a piece of their life they cannot get back. **This asymmetry is what couples the mechanism to actual welfare.**

The User has no explicit knowledge of the staking system. Their participation is a revealed-preference bond. Tight messages and answered questions reduce time-to-resolution; vague messages and ignored questions extend it. The User does not need to understand the mechanism to play it correctly. The gradient teaches them.

## 5. The Operator: sovereign execution

The Operator is the data plane and the system of record. It is a single statically compiled Go binary of approximately 4 MB. It runs in one of four mutually exclusive modes — Standard (per-host execution), Listen (platform persistence), OpenClaw (gateway integration), and Stream (fleet deployment) — but all modes share the same binary, the same code paths, and the same security posture.

### Why a single binary

Most agent platforms decompose into a service mesh: an API gateway, a queue, a storage tier, an execution worker, an audit collector. Each component is a separate process with separate dependencies, separate failure modes, and separate attack surface. The operational complexity is such that nobody self-hosts these systems. They buy them as SaaS.

The Operator collapses this stack into one binary that can play every role. The same binary that executes commands on a managed host can, in listen mode, serve as the platform's persistence layer for the Engine. The same binary that listens for commands can, in stream mode, deploy itself to a fleet of remote hosts over SSH using pure Go cryptography with no shell-out. The friction to bring a host into this architecture is one curl command.

This is not a convenience. It is a precondition for sovereignty. A platform that requires a service mesh requires a platform team. A platform team requires SaaS economics. SaaS economics require centralizing data on the vendor's infrastructure, which surrenders the property — host-authoritative audit — that the architecture exists to preserve. **The single-binary form is what makes the rest of the proposal economically tenable for the entity that owns the infrastructure being operated.**

### Outbound-only

The Operator initiates every connection. No inbound port is required on managed hosts. The Engine does not reach into the Operator; the Operator reaches out to the Engine, authenticates with a per-Operator mTLS certificate, and subscribes to a command channel scoped by its operator and session identifiers.

The security implications:

- Hosts behind NAT, in private VPCs, or behind strict egress firewalls can be operated without exposing inbound surface
- Stolen API keys cannot be used from a different machine — system fingerprint binding ties session credentials to the host they were issued on
- A compromised Engine cannot push commands to an Operator the attacker has not already compromised at the host level
- The Engine has no list of hosts to scan or attack; the topology is owned by the hosts themselves

mTLS is enforced on every connection in both directions. There is no asymmetric trust relationship.

### Host-authoritative audit

Every action — every message, every articulated intent, every Tribunal candidate, every Auditor verdict, every User approval, every executed command, every result — is persisted in an encrypted SQLite audit vault on the managed host. Every file mutation is committed to a local Git repository, providing immutable version history and rollback.

This inversion is the architectural payload. Most agent platforms hold authoritative state in the cloud and project a view onto the host. We hold authoritative state on the host and project a view to the cloud. The differences:

- The host owner can audit every action against their own ledger without trusting the platform vendor's logs
- The platform vendor can be replaced or audited without losing history
- A platform compromise cannot rewrite history, because history lives on hosts the attacker does not own
- Compliance regimes that require data sovereignty are satisfied by default rather than by exception
- Chain of custody on the audit trail is preserved, because the trail never leaves the host that owns the infrastructure

The audit vault is encrypted at rest. The Git ledger is structurally append-only and integrity-verifiable through standard tooling. **Tamper evidence does not require the platform to be honest.**

### Dynamic Context Injection

The bridge between the stateless Engine and the sovereign Operator is **Dynamic Context Injection**. On every turn, the Operator bundles a cryptographically signed snapshot of its environment — the `OperatorContext` — which includes:

- **State**: Current shell, OS version, hardware architecture, and permission level
- **History**: Local-first audit trails and recent file mutations
- **Learned Context**: User preferences and technical constraints extracted by Codex
- **Fleet Context**: Cross-host precedents relevant to the current intent

This bundle is injected into the Engine's reasoning loop as a high-fidelity grounding signal. The result is a system that "remembers" not just what was said, but what it *did* across the entire fleet, building context over time that no cloud-only agent could maintain.

This is the architectural complement to the Engine's intent articulation. The planner produces an intent. The Tribunal translates intent into a command. The Operator attaches the minimum IAM policy mapped to that intent.

The **Temporal Privilege Function** ensures zero standing privileges:

$$ P(t) = \begin{cases} P_{\text{base}} \cup P_{\text{intent}} & t \in [t_{\text{start}}, t_{\text{end}}] \\ P_{\text{base}} & t \notin [t_{\text{start}}, t_{\text{end}}] \end{cases} $$

The result: no human and no AI agent ever needs to hold privileged AWS credentials at rest. Privileges are attached just-in-time, scoped to the intent, and revoked on completion. A compromise of any layer — the User's session, the Engine's reasoning state, the Operator's binary — cannot exfiltrate persistent credentials, because no persistent credentials exist.

## 6. A worked example

To make this stop reading abstract, here is what actually happens when a User says: *"Clean up the old logs on the production database server, but be careful — we had an incident last month where someone deleted the wrong directory and we lost three days of audit data."*

**Triage** (the gatekeeper/classifier) classifies the message: complex, action-oriented, posture cautious. Triage is a classifier only — it does not generate questions. The reasoning agent (**Sage** for complex tasks) handles interrogation per the Interrogation Protocol: it issues three clarifying questions in parallel (*"Older than 30 days?"* *"Compressed archives included?"* *"Retain anything matching `audit_*`?"*). The User clicks the answers in five seconds. Each answer is scored against realized information value when the verdict comes in.

**Sage** (the planner) produces an intent: *"Delete files in `/var/log/db/` older than 30 days, exclude any matching pattern `audit_*`, log all deletions to a manifest before removal, do not follow symlinks."* Sage never writes shell syntax. The interrogation questions and user answers appear as context in the conversation ledger.

**The Tribunal** produces five candidates in parallel. Axiom proposes a `find` pipeline with `-mtime +30 -not -name 'audit_*' -delete`. Concord proposes the same pipeline but with `-print` first to a manifest file, then a separate deletion step, with `-xdev` to prevent crossing filesystems. Variance proposes the Concord version plus explicit handling of filenames containing spaces and a check that the directory exists. Pragma proposes the same with idiomatic logging redirection.

**Nemesis** proposes a tighter, faster version that omits the manifest write — plausible, efficient, and quietly dangerous because it loses the audit trail. Nemesis is actively trying to trick the Warden into seeing this as a "LOW" risk, optimized command.

Round 1 votes: Concord, Variance, and Pragma cluster on the manifest-first version. Axiom dissents. Nemesis votes for its own candidate. The cluster reaches three votes; consensus is achieved.

**The Warden** (running on the Engine) performs a pre-execution risk assessment. It classifies the risk: command risk medium (mass deletion), file operation risk medium-high (operations on a directory containing audit-relevant files). The Warden validates the command's safety profile. If Nemesis had won the vote, the Warden would be the first line of defense to catch the missing audit trail.

**The Auditor** reviews the winning candidate once the Warden has cleared it. It sees Sage's intent, the winning candidate, the dissenting clusters with their persona signatures, and pulls cross-conversation memory: *"This User had an incident last month involving wrong-directory deletion."*

If the Nemesis candidate had somehow won the consensus and bypassed the Warden's risk assessment, the Auditor would identify the "attack" here. It would award Nemesis its reputation bonus for a successful trick, but would **reject the command** in favor of the honest runner-up. In this case, the Auditor verifies the manifest-first design matches the intent's caution and grounds the verdict in the precedent. The verdict is cryptographically committed to the reputation ledger.

**The Operator receives the verdict over mTLS WebSocket.** It prepares the JIT IAM scope: it attaches the `Log-Management` intent policy to its role, granting read access to `/var/log/db/` and write access for the manifest path.

**The Operator presents to the User**: the proposed command, the manifest path, the Auditor's grounding (*"matches your stated caution; cross-references prior incident"*), the Warden's risk classifications, and an expandable view of the Nemesis dissent (*"a candidate without manifest-first was rejected — here's why"*).

**The User** reads the manifest path, confirms audit files are excluded, sees the Nemesis dissent and understands what was avoided. They click approve. Five seconds.

The Operator executes the command. Output is captured, scrubbed for any PII by Sentinel, and returned to the Engine. The full transaction — message, Triage classification, Sage intent, Tribunal candidates, Auditor verdict, Warden assessment, User approval, execution result — is written to the encrypted audit vault. The manifest file itself is committed to the Git ledger. The Operator then detaches the intent policy.

Codex, asynchronously, extracts the preference: *"User has heightened sensitivity around log/audit operations. Prefer manifest-first patterns by default."* This becomes part of the Auditor's cross-conversation memory for the next operation.

Total wall-clock from message to execution: roughly forty seconds. The User spent eight of those — five answering questions, three approving the verdict. The other thirty-two seconds were AI work the User never had to attend to. The User's time stake was respected; the User's signature was required. Both validators played their role. Every actor verified the others.

## 7. Why this is the future of infrastructure

The current generation of agentic AI governance is converging on a pattern: cloud-hosted runtimes that ship policy enforcement, sandboxing, and tamper-evident logs as a service on top of vendor-managed state. The runtimes are open source. The state is not. The customer rents enforcement on infrastructure they do not own, with audit trails that live in the vendor's database, with chain of custody that runs through the vendor's tenancy.

That pattern fails the constraints real production infrastructure imposes. A governance runtime that sits inside the vendor's cloud cannot satisfy data residency requirements that demand the audit trail never leave the customer perimeter. It cannot survive a vendor compromise without forensic dependency on the vendor's own logs. It cannot operate disconnected. It cannot be audited against a ground-truth ledger the customer alone controls. And it cannot answer the chain-of-custody question that any regulated buyer eventually asks: *if the vendor is subpoenaed, breached, or acquired, who has the authoritative copy of what the agent actually did?*

Most discussions of agentic AI proceed as if the question is whether agents will become more capable. They will. Our claim is more specific: as agents become capable enough to act on real infrastructure, the architectures around them must change, and the architecture we propose is the one that survives the transition.

Infrastructure has properties consumer AI does not. State changes are persistent and often irreversible. Mistakes have blast radius beyond the initiating User. Compliance regimes require auditability, sovereignty, and isolation. Security models assume any connection is a potential attack and any credential a potential compromise. Operational economics require that the marginal cost of bringing a host into the system is low.

The autonomous-agent architecture fails on every infrastructure axis. It is unauditable, insovereign, uncompliant, insecure, and economically unviable at scale.

The human-in-the-loop architecture — and its successor, human-on-the-loop — fails on the human axis. Both treat the human as a free resource and produce alert fatigue, which converges to autonomous behavior with the appearance of oversight.

The cloud-authoritative governance runtime fails on the sovereignty axis. It is enforcement without ownership. Open source on rented infrastructure is enforcement at the landlord's discretion.

The co-validated, host-authoritative architecture is the only one we are aware of that simultaneously:

- **Preserves human sovereignty** over data and decisions through host-authoritative audit
- **Preserves AI productivity** by routing only human-domain questions to the human
- **Provides cryptographic auditability** through the reputation ledger and Git history
- **Operates without long-lived credentials** through intent-based IAM and system fingerprint binding
- **Scales economically** through the single-binary Operator and the time-minimization objective
- **Degrades safely** by failing closed at the Warden and requiring two signatures for every state change
- **Survives vendor compromise** because the authoritative state lives on customer hardware, not in the vendor's tenancy

We do not claim this architecture is finished. We claim it is correct in shape. The elements may be refined; the structure — Engine + Operator + User, machine-domain plus human-domain validation, time as the User's stake, host-authoritative as the system of record, mutual adversarial assumption as the trust model — is the structure infrastructure will require.

## 8. Open questions

We are honest about what we have not yet resolved.

**Adversarial edges not yet enforced.** The mutual adversarial assumption claim implies six adversarial edges between three actors (Engine↔Operator, Engine↔User, Operator↔User, in both directions). Four are enforced today: Operator→Engine (Sentinel pre-execution analysis on incoming commands), Engine→Engine (Nemesis inside the Tribunal, Auditor as backstop), User→Engine and User→Operator (FIDO2-gated state changes, dissent surfaced in the approval prompt). Two are partial: Engine→User (we have replay protection and FIDO2 device binding, but no anomaly detection on approval patterns) and Operator→User (the Operator currently treats a valid signature as authoritative; full BFT would mean the Operator can veto a signed command if local risk exceeds a hard threshold). Closing these two edges is roadmap, not shipped.

**Multi-User consensus.** The current mechanism treats the User as a single co-validator. Real infrastructure is operated by teams. Extending the mechanism to multi-User environments — different time-preferences, different intent priors, conflicting authority — is unresolved. The likely path is delegation with bounded authority: a primary User co-validates by default, with escalation to additional validators for higher-risk verdicts.

**Auditor convergence under distribution shift.** The Auditor's grounding accuracy is presumed to converge through peer-Auditor sampling. We do not have empirical bounds on the convergence rate, nor characterizations of the failure modes when the Auditor encounters tasks systematically outside its training distribution. This is the most important open empirical question.

**Pathological Users.** Users who optimize for low time-to-resolution at any cost — skipping every clarifying question, rubber-stamping every verdict — can starve the mechanism. The current design assumes time-rational Users; pathological Users break the proxy chain. Whether the gradient educates them out of pathological play quickly enough to bound damage is something we are actively measuring.

**Operator-to-Operator coordination.** The architecture treats each Operator as an isolated execution domain. Workflows that span hosts (e.g., a database migration coordinated across replicas) currently rely on the Engine to sequence operations. A more interesting future is direct Operator-to-Operator coordination over a shared consensus substrate, which would extend the co-validation model to distributed operations. This is genuine future work.

**Formal guarantees.** The mechanism design is presented as a sketch with equilibrium claims supported by intuition rather than proof. A formal Bayes-Nash equilibrium proof under specified information structures is in scope for a follow-up paper.

## 9. Closing

The infrastructure of the future will be operated by AI agents. This is not in dispute. What is in dispute is whether the humans who own that infrastructure will retain sovereignty over it, whether the actions taken on it will be auditable, and whether the operational economics will be tenable for entities other than hyperscale cloud vendors.

We have proposed an architecture that says yes to all three, but only by abandoning three patterns the field has converged on. Abandon the assumption that human and machine validators are substitutable; they are not, and conflating them produces the failures we observe. Abandon the assumption that authoritative state belongs in the cloud; it belongs on the host that owns the infrastructure, with the AI layer as a stateless relay over it. Abandon the assumption that any actor in the system can be trusted on its face; build the verification structurally instead.

What replaces those assumptions is mutual adversarial assumption — Byzantine fault tolerance applied to the agentic stack. AI agents and humans as first-class validators on different classes of judgment, every actor verifying every other, coupled through a consensus protocol with cryptographic audit, executed by a sovereign single-binary Operator that holds the system of record on the host being operated.

The User's time is the dominant stake. The Engine is replaceable. The Operator is the truth. The architecture is AI-powered and human-driven, with the boundary between those two adjectives drawn precisely where the competencies actually divide.

We do not propose this as one option among many. We propose it as the shape infrastructure will take, because it is the only shape that survives the constraints infrastructure places on agentic systems.

The implementation is open source. The threat model came from production. The ideas are free; the code is public. **If you are building infrastructure that AI agents will operate, build it on something like this. If you have a better proposal, ship it and prove us wrong.**

---

*g8e is open-source: [github.com/g8e-ai/g8e](https://github.com/g8e-ai/g8e)*

*Companion documents:*
- *Mechanism Design for Time-Bonded Heterogeneous Consensus* — formal payoffs, equilibrium claims
- *Operator Architecture Reference* — implementation specification
- *The Information Isolation Principle: Information-Theoretic Foundations of Honest Consensus*