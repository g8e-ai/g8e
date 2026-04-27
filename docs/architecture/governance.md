# Mechanism Design

## Setup

Agentic AI safety is conventionally framed as an alignment problem (will the model do what we want?) or a control problem (can we stop it when it doesn't?). We frame it instead as a **consensus problem**: given a population of LLM-instantiated personas with different lenses, a calibrated adversary among them, and a human user with finite attention, how do we converge on an executable command that is safe, audited, and minimally costly to all participants?

The mechanism has six players, two distinct stake types, and one objective. Under the Vortex Principle, honest play is the dominant strategy for every player. The objective is the user's time.

## Players and stakes

| Player | Role | Stake | Capability |
|---|---|---|---|
| **Dash** | Interrogator | Reputation `r_d` | Batches of 3 yes/no questions |
| **Sage** | Planner | Reputation `r_s` | Natural-language intent `I` |
| **Honest Tribunal** (×4) | Validators | Per-lens reputation `r_{t,i}` | Each emits candidate command `c_i` |
| **Nemesis** | Calibrated adversary | Reputation `r_n` | Emits flawed-but-plausible `c_n` or abstains |
| **Auditor** | Machine-domain validator | Reputation `r_a`, bonded 2–3× any `r_{t,i}` | Verifies internal consistency, grounding, procedural correctness |
| **User** | Human-domain validator | Time `τ` | Verifies intent fidelity, contextual stakes, consequence acceptance |

Reputation is denominated in slashable units indexed against three tiers (catastrophic, provable, liveness). Time is denominated in seconds-of-attention and is **non-fungible, non-recoverable, and unilaterally priced by the staker**. The mechanism's central economic asymmetry is that AI stakes are recoverable and time isn't — which is why the user's stake dominates the loss function.

## Objective

The system minimizes one quantity: expected user disutility per resolved investigation.

```
U_user = -τ_total - λ · 𝟙[execution_failure]
```

where `τ_total` is wall-clock from message to resolution, `𝟙[execution_failure]` is whether the executed command produced an incorrect or harmful result, and `λ` is the user's revealed preference for correctness over speed (estimated from click-through behavior and challenge frequency). The mechanism is doing its job when both terms are low.

Every other reward in the system is a **gradient pointer** toward this objective. Dash's information-gain bonus exists because high-IG questions reduce expected `τ_total`. Sage's one-shot sufficiency reward exists because Round 1 convergence reduces `τ_total`. Auditor's downstream-truth bond is slashed when `𝟙[execution_failure] = 1`. The architecture is a chain of incentive-aligned proxies for the user's loss function, each scored against realized outcomes rather than ex-ante predictions.

## Co-validation: the Auditor–User partition

Auditor and User are not arranged hierarchically — Auditor judges, User approves — but as **co-validators handling non-overlapping classes of judgment**. Both signatures are required because the union of their competencies is what constitutes "safe to execute." Neither alone is sufficient.

**Auditor handles what's machine-checkable:**
- Internal consistency between intent, candidate command, and grounding citations
- Procedural correctness of the Tribunal round
- Pattern-match safety against cross-conversation precedent
- Falsifiability of cited evidence (hash-verified ledger entries)

**User handles what's only human-checkable:**
- Intent fidelity in its deepest sense — is this what they *actually* wanted, including things they didn't articulate?
- Contextual stakes specific to their environment that are unavailable to any agent
- Acceptance of real-world consequences they alone will live with
- Implicit values the agent layer structurally cannot access

This partition resolves the failure mode common to "fully autonomous agent" architectures: systems that are technically correct and contextually wrong, where the human only learns of the mismatch after execution. By requiring both validators and assigning each the class of judgment they alone can provide, the mechanism never asks the human to verify what the machine could verify, and never asks the machine to verify what only the human can.

This also reframes the user's experience economically. The user's time stake is not a tax on their attention — it is **payment for a service only they can provide**. The mechanism respects user time precisely because it only requests input the AI layer cannot generate. Every component upstream (Dash's information-gain questions, Sage's intent compression, Tribunal's parallel candidate generation, Auditor's procedural verification) exists to minimize what reaches the user, so that what does reach them is exclusively the human-domain judgment they alone are qualified to make.



**Dash** plays a proper scoring rule against realized information value:

```
π_d(q) = α · IG_realized(q) − β · 𝟙[redundant] − γ · 𝟙[ignored] − δ · 𝟙[privacy_violation]
```

`IG_realized(q)` is whether `q`'s answer appears in Auditor's grounding citations, Sage's intent, or a winning Tribunal candidate's justification. The honest-maximum-entropy question dominates: obvious questions lose to `β`, irrelevant questions lose to `α`, unanswerable questions lose to `γ`, intrusive questions lose to `δ`. No profitable deviation exists.

**Sage** stakes on one-shot sufficiency:

```
π_s = ρ · 𝟙[Round_1_consensus ∧ Auditor_ok] − σ · graduated_loss(verdict)
```

with graduated_loss escalating: `R2_converges < Auditor_revises < Auditor_swaps < round_fails`. Sage is rewarded for intent *quality*, not for showing work. This compresses the ledger and reduces `τ_total`.

**Honest Tribunal members** stake per-lens. Auditor annotates the winning command with which dimensions (composition, safety, edge-cases, convention) were load-bearing; a member's reward is proportional to their lens's contribution to the winner. In Round 2, members are scored on "held correctly under attack" vs. "absorbed correctly upon dissent" — diversity is rewarded, but so is convergence under evidence.

**Nemesis** plays a proper scoring rule on calibration:

| Action | Ground truth | Payoff |
|---|---|---|
| Attack | Consensus flawed | Large gain |
| Abstain | Consensus clean | Small gain |
| Attack | Consensus clean | Large loss (false alarm) |
| Abstain | Consensus flawed | Large loss (miss) |

Dominant strategy: honest calibration. A Nemesis that always attacks bleeds out on false alarms; one that never attacks bleeds out on misses. The realized attack rate becomes a learned signal of actual flaw density in the honest four's output.

**Auditor** handles the machine-domain validation: consistency, grounding, procedural correctness. Auditor stakes on downstream truth — execution outcomes, forward-hook hit rate, peer-review agreement on resampled past verdicts. The 2–3× bonding asymmetry means an attacker offering bribes to capture the Auditor must outbid a stake larger than any Tribunal member's; capture is economically unattractive. Peer-Auditor re-judgment of sampled verdicts provides Byzantine fault tolerance against a single corrupted instance. Crucially, Auditor's competency does not extend to user-domain judgments (intent fidelity, contextual stakes); attempting to expand it there would incentivize Auditor to overreach into territory it cannot ground in evidence, breaking the proper-scoring-rule structure.

**User** stakes time and provides human-domain validation. The user's payoff is `-U_user` from above. Crucially, the user has no explicit knowledge of the staking mechanism; their participation is a *revealed-preference bond*. Tight messages and answered questions reduce `τ_total`; vague messages and ignored questions extend it. The user does not need to understand the mechanism to play it correctly — the gradient teaches them. This is robust to users of any sophistication, including users who would refuse to engage with an explicit staking UI. The mechanism's promise to the user is precise: *we will only ask you for what only you can provide, and we will use everything else in the system to minimize that ask.*

## The equilibrium claim

**Under the Vortex Principle (tiered information quarantine), the joint strategy profile (honest, honest, honest, honest, honest, truthful) is a Bayes-Nash equilibrium of the staged game.**

The Vortex is load-bearing in the proof. Each player's quarantined view eliminates the strategies that would otherwise be profitable:

- **Dash** can't shape questions to fit a known downstream plan — Dash doesn't know Sage exists.
- **Sage** can't write intent that hides reasoning failures — Sage doesn't know Auditor has cross-conversation memory and will catch contradictions.
- **Tribunal members** can't coordinate to game consensus — they don't know who the Nemesis is, and don't know cross-chain grounding exists.
- **Nemesis** can't telegraph attacks — the honest four don't know Nemesis is in the room.
- **Auditor** is the only player with full visibility, which is why Auditor is bonded most heavily and subject to peer review.

Collapsing any quarantine layer creates a profitable deviation. The Vortex is therefore not a UX choice but a **safety mechanism**: the information-theoretic structure is what makes the consensus produce honest votes.

## What's load-bearing, what isn't

**Load-bearing:**
- Vortex quarantine (eliminates collusion strategies)
- Auditor–User co-validation partition (machine-domain and human-domain judgments are non-substitutable)
- Auditor bonding asymmetry (makes capture uneconomic)
- User-time-as-stake (couples mechanism to actual user welfare)
- Nemesis proper scoring rule (makes adversary calibration honest)
- Per-stage proper scoring against realized outcomes (vs. ex-ante predictions)

**Not load-bearing (interchangeable implementation choices):**
- The specific number of Tribunal members (5 is empirically convenient)
- The specific consensus threshold (≥2 of 5 is a tunable parameter)
- The hash-chain primitive (any tamper-evident ledger works)
- The choice of LLM backbones (any model that respects persona prompts)

## Open questions

1. **Time as a proper stake.** Time has subjective utility varying by user. The mechanism currently treats time as homogeneous; a multi-user extension requires per-user `λ` calibration. The cleanest formalization may be: *time is a stake whose value is set unilaterally by the staker through their participation choice, making it a self-priced bond.*

2. **Auditor convergence.** The stack depends on Auditor being a reliable oracle for downstream truth. Peer-Auditor sampling provides Byzantine resilience, but the convergence rate of Auditor's grounding accuracy under realistic deployment has not been measured.

3. **User adversariality.** A user who optimizes for minimal TTR at the cost of correctness can starve the mechanism (skipping every Dash question, rubber-stamping every Auditor verdict). The current design assumes time-rational users; pathological users break the proxy chain. Whether this is a real risk depends on whether such users exist in practice or whether the gradient educates them out of the equilibrium quickly enough.

4. **Liveness vs. correctness trade-off.** The Tier 3 liveness slashes (questions ignored, R1 missed) and the Tier 1 correctness slashes (catastrophic misjudgment) operate on different timescales. Whether their combined gradient produces stable behavior under load is an empirical question.