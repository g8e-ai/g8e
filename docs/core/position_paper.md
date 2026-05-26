---
title: Position Paper
parent: Architecture
---

# The Sovereign Execution Substrate

### Co-validation, heterogeneous consensus, and the case for free and open AI runtime governance

**A position paper.** Last updated: 2026-05-25.

---

## Abstract

We argue that the architecture wrapped around autonomous agents must shift from *"trust but verify"* to *"verify, then execute."* Capability protocols — MCP, A2A, OpenAI-style tool calls — prove an agent *can* act. They establish no authority over whether it *should*. g8e supplies the missing admission boundary: a typed, signed, state-bound transaction that must clear a fail-closed verification gauntlet at the host before any side effect occurs.

The central claim of this paper is that **safe autonomous execution is a Byzantine agreement problem layered on top of a human-intent problem, and the two cannot be collapsed into one validator.** Machine-checkable *consistency* and human-checkable *intent* are orthogonal competencies. Authority safe to execute is the conjunction of both proofs, bound cryptographically to a single transaction, verified locally against sovereign state. Neither signature alone is sufficient.

We develop this with the mathematics of heterogeneous consensus, a mechanism-design account of why human time is the only validator resource that cannot be farmed, and a model of the four-role trustless dependency chain that makes entire classes of legacy attack require simultaneous, orthogonal compromise rather than a single point of failure. We close with the argument we hold most strongly: that runtime governance and audit are public goods, and that gating them behind a paywall is incompatible with a safe AI-powered, human-driven world. They must be free, open, and self-hostable.

---

## 1. The substitutability error

Every current approach to agent safety makes the same mistake in a different costume: it treats human and machine validators as substitutable. Pick one, and you inherit its failure mode.

**Full autonomy** assumes the machine is a sufficient validator. It is not. A model verifying its own intent has no independent basis for catching a misaligned-but-coherent plan; the same weights that produced the action produce its justification. Worse, a single model is a single attack surface. A prompt injection, a poisoned tool description, or a jailbreak does not need to defeat a verifier — it *becomes* the verifier. Autonomy fails by executing confident nonsense.

**Human-in-the-loop** assumes the human is a sufficient validator and is available at machine throughput. Neither holds. Route every action through a person and you manufacture alert fatigue: the marginal cost of attention collapses, approvals degrade to reflex, and the signature that was supposed to mean *"I, a responsible party, vouch for this"* comes to mean nothing. Human-in-the-loop fails by rubber-stamping at scale.

Both failures share a root cause. They ask one validator to certify two different things at once — that the action is *internally consistent and technically safe*, and that it *matches what a responsible human actually wants*. These are not the same property, they fail independently, and no single validator is competent at both. A model is good at the first and structurally blind to the second. A human is authoritative on the second and cannot keep pace on the first.

The resolution is not a better single validator. It is to stop pretending the two competencies are one.

---

## 2. Co-validation

We define authority over a state-changing action as the conjunction of two independent proofs:

- A **consistency proof** — evidence that the action is technically sound, policy-compliant, and that an independent ensemble agrees it faithfully realizes the stated request. This is machine-checkable and produced by the L2 consensus layer.
- An **intent proof** — evidence that a responsible human has authorized *this specific action*. This is human-checkable and produced by the L3 notary layer.

$$
\text{Authority}(\text{action}) \;\equiv\; \text{ConsistencyProof}(\text{action}) \;\wedge\; \text{IntentProof}(\text{action})
$$

The conjunction matters more than either conjunct. Defense in depth is usually described as a sum — more layers, more chances to catch something. Co-validation is a **product**: an unsafe action executes only if it passes a consistency check it should fail *and* an intent check it should fail, where the two checks draw their errors from different distributions because they are testing different properties. We make this precise in §4.

This is also why g8e is *agnostic by construction* — agent-agnostic, model-agnostic, platform-agnostic, domain-agnostic. The verifier never asks *who* produced an action or *what it is for*. It asks only whether the two proofs are present and valid, bound to this transaction, against this host's current state. Agnosticism is not a feature bolted on for market reach. It is the consequence of a trust model that checks mathematics rather than provenance. A substrate that trusted certain vendors more than others would have smuggled a privileged channel back in, and the whole point is that no privileged channel exists.

---

## 3. The trustless dependency chain

The system was designed as a whole, not as a gateway with accessories. Four roles cooperate, and each *layers intent* by adding a proof that the next layer can check without trusting the layer that produced it.

1. **The frontend** captures human intent. It is where a responsible party reads the proposed action and, for high-risk mutations, signs. Its output is an intent proof — a WebAuthn assertion (or, for CLI sessions, an mTLS certificate fingerprint) computed *over the transaction hash*, not over a session. The approval is bound to one action, forever.

2. **The g8e-compatible agentic ensemble** forms machine intent. It translates a request into a concrete typed payload and reaches consensus that the payload is a faithful, safe realization of the request. Its output is a consistency proof — a set of Ed25519 signatures over the transaction hash from independent reasoners.

3. **The Governance Gateway (`g8eg`)** admits. It assigns cryptographic identity (SPIFFE URI SANs over mTLS), enforces replay defense, distributes state roots, and relays envelopes to the right host. Critically, it holds **no privileged bypass**. It is a stateless coordinator, not a trusted authority.

4. **The Governed Operator (`g8eo`)** verifies and executes. It re-derives every proof from scratch against its own local state, trusts nothing upstream, and is the only component permitted to mutate the host. It writes the audit record *before* the side effect, scrubs the output at the boundary, and emits a signed receipt anchored to a host-local, git-backed ledger.

The chain is *trustless* in the precise sense that each role depends on the previous for **inputs** but not for **trust**. The Operator does not believe the Gateway that a transaction is consensus-approved; it checks the signatures itself. It does not believe the ensemble that the state is current; it compares the envelope's expected Merkle root against its own ledger root. Every claim arrives as a proof the next layer can independently falsify.

The consequence is that compromising any single layer is insufficient to cause an unauthorized mutation. This is what we mean when we say the design makes old security and privacy concerns *structurally* harder — not by patching them one by one, but by requiring that an attacker simultaneously defeat orthogonal, independently audited proofs.

| Legacy attack class | The proof that neutralizes it |
|---|---|
| Prompt injection / a jailbroken single model | L2 heterogeneous consensus — peers reject; the only residual is a *shared* blind spot (see §4) |
| Stolen session token | L3 binds approval to the transaction hash plus live human presence; a session alone authorizes nothing |
| Replay of a previously approved action | Nonce in a sliding replay window plus `expires_at` freshness |
| Time-of-check/time-of-use, approval against a stale world | `state_merkle_root` binding — the Operator rejects if its local root has moved |
| Compromised relay or man-in-the-middle gateway | The Operator re-verifies all proofs locally; the Gateway has no bypass; mTLS + SPIFFE identity |
| Data exfiltration through model context | The Sovereignty Boundary scrubs before egress; raw data and tokens are rehydrated only at execution, on-host |
| Credential theft | Zero standing privileges — there is no persistent credential to steal (§6) |
| Any single rogue actor, human or machine | Co-validation — no unilateral path exists; consensus *and* a human signature are both required |

"Impossible" is a strong word and we use it carefully. The honest claim is this: each of these attacks, which against a conventional agent integration is a single-point compromise, becomes a requirement to break two or more orthogonal proofs at once — and every attempt, successful or not, is recorded in a tamper-evident local vault before anything executes. The bar moves from *"exploit one weakness"* to *"coordinate a multi-domain compromise that also evades a fail-closed audit."*

---

## 4. The mathematics of heterogeneous consensus

The consistency proof is a Byzantine agreement among reasoners. Its strength is almost entirely a function of one quantity that the marketing of "ensemble AI" tends to hide: the *correlation* between validator errors.

### 4.1 The independent case, and why it is seductive

Let an action be either safe or unsafe. We want unsafe actions blocked. Consider $n$ validators, each independently approving an unsafe action — a false-approve — with probability $p$. We require $k$ approvals to pass. If errors are independent,

$$
\Pr[\text{unsafe passes}] \;=\; \sum_{i=k}^{n} \binom{n}{i} p^{\,i} (1-p)^{\,n-i}.
$$

For small $p$ and $k \ge 2$ this is dominated by its first term, $\binom{n}{k} p^{k}$ — it falls *polynomially in $p$ raised to the $k$*. With $p = 0.05$ and a 3-of-5 rule, the chance an unsafe action slips through is on the order of $10^{-3}$. Push to 4-of-7 and it is smaller still. This is the **Condorcet Jury Theorem** wearing a cryptographic hat: if each juror is better than a coin flip and votes *independently*, the probability the majority is correct tends to 1 as the jury grows.

This is the result every "just add more models" pitch is implicitly invoking. It is also where the theorem hides its load-bearing assumption in a single word: *independently*.

### 4.2 The common-mode floor

Models trained on overlapping corpora, with similar architectures and similar alignment procedures, share failure modes. The same injection, the same adversarial suffix, the same plausible-but-wrong reasoning pattern fools all of them together. Independence is exactly the assumption that does not hold for a homogeneous ensemble.

Model this directly. Let there be a **common-mode event** with probability $c$ — a shared blind spot that makes *every* validator false-approve simultaneously — and, conditional on no common-mode event, let each validator fail independently with probability $p$. Then

$$
\Pr[\text{unsafe passes}] \;=\; c \;+\; (1-c)\sum_{i=k}^{n}\binom{n}{i} p^{\,i}(1-p)^{\,n-i}.
$$

Now take the limit that the redundancy pitch relies on. As $n \to \infty$ with a majority rule and $p < \tfrac12$, the independent term vanishes — Condorcet still works *against idiosyncratic error*. But the common-mode term does not move:

$$
\lim_{n\to\infty}\Pr[\text{unsafe passes}] \;=\; c.
$$

**The safety of the consensus layer is bounded below by the common-mode failure rate, and no amount of redundancy reduces that floor.** Ten homogeneous models that share a blind spot are, against the attack that exploits the blind spot, barely better than one. Adding correlated validators is theater.

The only lever on $c$ is heterogeneity: different providers, different architectures, different training data, different alignment lineages, voting independently. This is why g8e's consensus layer is provider-agnostic *as a safety requirement*, not a convenience — provider agnosticism is the mechanism by which $c$ is driven down. Redundancy ($n$) fights idiosyncratic error; heterogeneity (lower $c$) fights correlated error; and since the correlated term is the one that survives the limit, **heterogeneity dominates redundancy.** An honest ensemble is measured by the diversity of its members' failure modes, not their count.

### 4.3 Why the human layer multiplies rather than adds

Let $q$ be the probability a human false-approves an unsafe action that reaches them. Because the human is checking a *different property* — intent alignment, not technical consistency — their error is, to first order, independent of the ensemble's consistency error. The two competencies are orthogonal. Then

$$
\Pr[\text{unsafe executes}] \;\approx\; \underbrace{\big(c + (1-c)\,\Sigma_{k,n,p}\big)}_{\text{consistency fails}} \;\times\; \underbrace{q}_{\text{intent fails}}.
$$

An unsafe action executes only if it survives a consistency check it should fail *and* an intent check it should fail. This product is the entire reason co-validation is not redundant with deeper machine consensus. You cannot buy the human layer's contribution by adding models, because the human covers a failure mode no model covers.

There is one way to wreck this, and it is the trap of §1. If the human is shown so many actions that they stop reading, then $q \to 1$ and, worse, the human's error becomes *correlated* with the ensemble's — they are now just echoing whatever the machines surfaced. The product collapses back to a sum, and then to a single term. **The independence of the human layer is not free; it must be protected by keeping human signatures rare and expensive.** That requirement is not a UX preference. It is a precondition for the math to hold — and it leads directly to the economics of the next section.

---

## 5. Time as a self-priced bond

A signature is only evidence if it was costly to produce. A free signature is cheap talk: it carries no information because anyone, or any compromised process, can emit it at no cost. The design question for the L3 layer is therefore: *what makes a human's approval an informative, hard-to-forge signal of genuine belief?*

Consider the two resources a validator can stake.

**Machine reputation is recoverable.** When a consensus signer stakes reputation $r$ on a decision and is slashed for a bad one, the expected cost of a dishonest approval is roughly $r \cdot \Pr[\text{caught}]$. Reputation can be re-earned over repeated rounds; slashing is a fine, and fines are a cost of doing business. A patient or well-funded adversary can absorb them. Reputation staking is a real and useful deterrent, but it is *bounded*, and it is bounded in a currency the adversary can replenish.

**Human time is not recoverable.** It is non-fungible, non-transferable, and strictly scarce — the one validator resource that cannot be farmed, delegated, or regenerated. When the protocol requires a human signature for a high-risk mutation, it conscripts that resource as a bond. The person who signs spends attention they cannot get back, on an action whose failure costs *their own* infrastructure and *their own* time to remediate. This couples the mechanism to actual welfare through revealed preference: the approver only signs what they truly believe is correct, because the cost of being wrong is paid by them, in the one currency they cannot mint.

In the language of signaling: a costly signal separates types only when the cost is higher for the dishonest type than the honest one. Here the cost structure is even cleaner — the cost is borne by the party with the most context and the most to lose, in a resource adversaries cannot acquire in bulk. That is why a *sparing* human signature, bound to a specific transaction hash, is worth more than any volume of cheap machine attestation.

And it closes the loop with §4.3. The bond only stays valuable while it stays rare. Spend it on every action and you debase the currency — attention-per-signature falls, the signal degrades to cheap talk, and independence collapses. The consensus layer exists, in part, to *protect the value of the human bond* by filtering volume so that the few actions which reach a person are exactly the ones worth a non-recoverable cost. Machine consensus is what keeps the human signature expensive enough to mean something.

---

## 6. Cryptographic binding and sovereign state

Co-validation and consensus are arguments about *who decides*. They are worthless if the proofs can be detached from the action, reused, or evaluated against the wrong world. The binding layer is what makes the proofs rigid.

Every mutation is a single `GovernanceEnvelope`. A deterministic transaction hash is computed from its normalized fields, and the verifier enforces

$$
\texttt{id} \;=\; \texttt{transaction\_hash} \;=\; \mathrm{SHA\text{-}256}(\text{canonical fields}).
$$

Every proof — every L2 signature, every L3 assertion — is computed *over that hash*. This is the property that makes approval action-specific rather than session-specific: a human's WebAuthn assertion authorizes the exact bytes of one transaction and authorizes nothing else, so it cannot be transplanted to a different action, replayed against a later request, or harvested from a live session. Consensus signatures inherit the same rigidity.

Freshness is enforced by a `nonce` checked against a sliding replay window and an `expires_at` timestamp; a stale or reused transaction is rejected before any layer runs. Causal integrity is enforced by **state binding**: the envelope carries the `state_merkle_root` the producer expected, and the Operator compares it to its own current ledger root. If the world moved between approval and execution — the classic time-of-check/time-of-use gap — the roots disagree and the transaction is dropped. Approval is therefore bound not just to an action but to the *state of reality in which that action made sense.*

Identity is SPIFFE-style and carried over mutual TLS, with revocation checked on every handshake. Execution emits an `ActionReceipt` signed by a host-unique Ed25519 key, with `state_root_before` and `state_root_after` captured around a two-phase, git-backed commit, so every mutation is a tamper-evident ledger entry that can be rolled back. The wire format is canonical JSON for ecosystem compatibility; the signing basis is the deterministic hash, so JSON cosmetics can never change what was signed.

The point of the whole binding layer is singular: **a proof is meaningless unless it is rigidly attached to one action, in one moment, on one host.** g8e attaches it.

---

## 7. Sovereignty as an architectural invariant

Authoritative state belongs on the host, not in a vendor's cloud. This is not a deployment preference; it is what the threat model requires, and it forces three design commitments.

**The single binary is a precondition, not a convenience.** The reference Operator is a statically compiled binary, roughly 7MB compressed, with zero standing dependencies. A governance layer that requires a complex service mesh implicitly requires centralized operations to run it, which reintroduces exactly the trusted third party the architecture exists to eliminate. A single, auditable, air-gap-capable artifact is what allows the entity that *owns* the infrastructure to *own the system of record*. There is no runtime to patch and no interpreter to audit; the attack surface is the binary you can read.

**Zero standing privileges.** The Operator holds no permanent administrative credentials. Permissions are minted just-in-time, derived from the verified intent inside the envelope, scoped to a single action, and dissolved on completion. A compromise of any layer — a hijacked session, a poisoned reasoning state — cannot exfiltrate persistent credentials, because none exist. You cannot steal what was never standing.

**Data never leaves; only scrubbed projections do.** The Sovereignty Boundary scrubs secrets and PII at the execution boundary, replacing them with tokens that are rehydrated only at the instant of execution, on-host. The model upstream receives a safe projection of reality, never reality itself. Raw forensic data and full execution history remain local, split into a scrubbed vault safe for AI consumption and a raw vault for human security audit. The platform vendor is reduced to a stateless relay.

Sovereignty and the SaaS economics of a paywalled governance product are simply incompatible. A vendor that holds your state, your audit log, and your execution authority is itself an unaudited single point of trust — a direct contradiction of zero-trust. Which brings us to the argument that motivates the license.

---

## 8. The free-and-open imperative

We hold this position without hedging: **runtime governance and audit for AI agents are public goods, and gating them behind a paywall is incompatible with a safe AI-powered, human-driven world.**

The reasoning is straightforward. The benefit of an agent *not* mutating reality recklessly is largely non-excludable — it accrues to everyone downstream of that infrastructure, not only to whoever paid for the governance layer. Goods with non-excludable benefits are under-provided by markets that try to sell them. If safety is a premium SKU, the cheapest path to shipping an agent will always be the ungoverned one, and economics will select for it. The safe default loses to the free default every time the safe default costs money. **For governance to be the default, governance must be free.**

There is a second, sharper reason. You cannot trust a governance layer you cannot inspect and cannot self-host. A closed, proprietary governance vendor is an unaudited authority sitting in the most sensitive position in the stack — precisely the trusted third party that zero-trust architecture is built to abolish. An auditing system that is itself unauditable is a contradiction. The only credible guarantee is source you can read, a binary you can compile, and a deployment you can run inside your own perimeter with no outbound dependency. Open source is not g8e's distribution strategy. It is the only configuration in which the security claims are checkable, and therefore the only configuration in which they are true.

Accountability for autonomous action — the ability to say *who authorized this, on what basis, against what state, and prove it later* — is becoming civic infrastructure. Civic infrastructure behind a paywall is a two-tier safety regime, where well-funded actors are governed and everyone else is not. That is not a safe world; it is an unevenly dangerous one. This is why the substrate is Apache-2.0, single-binary, and air-gap-capable, and why it will stay that way. Everyone must have access to free and open AI agent runtime governance and auditing, or the governed world is only ever a subset of the world.

---

## 9. The forward invariant

The trajectory we are building toward is explicit. As agents grow more capable, **single agents will not be permitted to make state changes.** The baseline for any mutation that touches real infrastructure becomes consensus among heterogeneous reasoners *plus* a human signoff at some upstream stage — not as a configurable nicety, but as the floor.

The future work extends co-validation outward without changing its shape: Operator-to-operator coordination, so a transaction can require independent verification across hosts and environments; and multi-user consensus, so high-consequence actions require more than one human bond. In every extension the invariant holds unchanged:

> A typed, signed, state-bound transaction reaches a sovereign host agent that refuses to mutate reality unless every independent proof — machine consistency, human intent, freshness, and state — checks out, locally, and is recorded before the fact.

g8e is not an agent. It is the substrate on which agents must run to be viable in production infrastructure, and which must be free for that infrastructure to be safe. The architecture's bet is that the future of autonomous systems is not *trust but verify*. It is *verify, then execute* — and the right to verify belongs to everyone.

---

*Built by Lateralus Labs. Licensed Apache 2.0.*