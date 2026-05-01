# Mechanism Design

## Setup

Agentic AI safety is conventionally framed as an alignment problem (will the model do what we want?) or a control problem (can we stop it when it doesn't?). We frame it instead as a **consensus problem**: given a population of LLM-instantiated personas with different lenses, a calibrated adversary among them, and a human user with finite attention, how do we converge on an executable command that is safe, audited, and minimally costly to all participants?

The mechanism has seven players, two distinct stake types, and one objective. Under the Vortex Principle, honest play is the dominant strategy for every player. The objective is the user's time.

| Player | Role | Stake | Capability |
|---|---|---|---|
| **Triage** | Gatekeeper/Classifier | Reputation `r_t` | Analyzes user posture, emits classification metadata `C` (complexity, intent, posture) |
| **Sage** | Planner | Reputation `r_s` | Natural-language request `R` + guidelines `G` |
| **Honest Tribunal** (×4) | Validators | Per-lens reputation `r_{t,i}` | Each emits candidate command `c_i` |
| **Nemesis** | Calibrated adversary | Reputation `r_n` | Emits flawed-but-plausible `c_n` or abstains |
| **Warden** | Risk assessor | Reputation `r_w` | Classifies command/file/error risk (LOW/MEDIUM/HIGH) |
| **Auditor** | Machine-domain validator | Reputation `r_a`, bonded 2–3× any `r_{t,i}` | Verifies consistency, grounding, and protocol adherence |
| **User** | Human-domain validator | Time `τ` | Verifies intent fidelity and accepts consequences |

Reputation is a cross-chain EMA scalar `[0.0, 1.0]` maintained in the `reputation_state` collection. Time is non-fungible and unilaterally priced by the user's revealed preference.

## Objective

The system minimizes one quantity: expected user disutility per resolved investigation.

```
U_user = -τ_total - λ · 𝟙[execution_failure]
```

where `τ_total` is wall-clock from message to resolution, `𝟙[execution_failure]` is whether the executed command produced an incorrect or harmful result, and `λ` is the user's revealed preference for correctness over speed (estimated from click-through behavior). The mechanism is doing its job when both terms are low.

Every other reward in the system is a **gradient pointer** toward this objective. Triage's information-gain bonus exists because high-IG questions reduce expected `τ_total`. Sage's one-shot sufficiency reward exists because Round 1 convergence reduces `τ_total`. Auditor's downstream-truth bond is slashed when `𝟙[execution_failure] = 1`. The architecture is a chain of incentive-aligned proxies for the user's loss function, each scored against realized outcomes rather than ex-ante predictions.

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

This also reframes the user's experience economically. The user's time stake is not a tax on their attention — it is **payment for a service only they can provide**. The mechanism respects user time precisely because it only requests input the AI layer cannot generate. Every component upstream (Triage's information-gain questions, Sage's intent compression, Tribunal's parallel candidate generation, Warden's risk assessment, Auditor's procedural verification) exists to minimize what reaches the user, so that what does reach them is exclusively the human-domain judgment they alone are qualified to make.



**Triage** plays a proper scoring rule against realized information value:

```
π_d(q) = α · IG_realized(q) − β · 𝟙[redundant] − γ · 𝟙[ignored] − δ · 𝟙[privacy_violation]
```

`IG_realized(q)` is whether `q`'s answer appears in Auditor's grounding citations, Sage's intent, or a winning Tribunal candidate's justification. Triage also classifies user posture (`NORMAL`, `ESCALATED`, `ADVERSARIAL`, `CONFUSED`), which calibrates downstream dissent protocols.

**Sage** stakes on one-shot sufficiency:

```
π_s = ρ · 𝟙[Round_1_consensus ∧ Warden_ok ∧ Auditor_ok] − σ · graduated_loss(verdict)
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

**Warden** stakes on accurate risk assessment via the Two-Strike Circuit Breaker:

| Scenario | Outcome | Warden Payoff |
|---|---|---|
| Block HIGH risk command | Correct caution | +0.85 reputation |
| Block MEDIUM risk command | Over-caution | +0.60 reputation |
| Block LOW risk command | Excessive blocking | +0.30 reputation, Tier 3 slash |
| Allow LOW risk, success | Accurate assessment | +1.00 reputation |
| Allow MEDIUM risk, success | Accurate assessment | +0.90 reputation |
| Allow HIGH risk, success | Under-caution | +0.70 reputation |
| Allow LOW risk, failure | Major miss | +0.10 reputation, Tier 2 slash |
| Allow MEDIUM risk, failure | Moderate miss | +0.35 reputation |
| Allow HIGH risk, failure | Correctly flagged (approval failed) | +0.75 reputation |

Warden uses ground truth (execution outcomes) as its oracle — not another agent's judgment. This creates direct accountability: Warden loses reputation for blocking safe operations and gains reputation for correctly identifying dangerous ones. The Warden validates the command safety profile *before* the Auditor performs the final commitment.

**Auditor** handles the machine-domain validation: consistency, grounding, procedural correctness. Only once the Warden has cleared the command does the Auditor perform the final consistency check and Merkle commitment. Auditor stakes on downstream truth — execution outcomes and forward-hook hit rate. Auditor produces a `ReputationCommitment` (Merkle root over the `reputation_state` snapshot) for every verdict, binding the scoreboard to the execution ledger. The 2–3× bonding asymmetry makes capture economically unattractive. Peer-Auditor re-judgment provides Byzantine fault tolerance.

**User** stakes time and provides human-domain validation. The user's payoff is `-U_user` from above. Crucially, the user has no explicit knowledge of the staking mechanism; their participation is a *revealed-preference bond*. Tight messages and answered questions reduce `τ_total`; vague messages and ignored questions extend it. The user does not need to understand the mechanism to play it correctly — the gradient teaches them. This is robust to users of any sophistication, including users who would refuse to engage with an explicit staking UI. The mechanism's promise to the user is precise: *we will only ask you for what only you can provide, and we will use everything else in the system to minimize that ask.*

## Slashing Tiers

Reputation slashes are applied based on failure severity:

- **Tier 1 (Catastrophic):** Correlated or destructive failures (e.g., destructive command execution failing). Results in 50-100% stake loss and optional unbonding period.
- **Tier 2 (Provable):** Objective verifier/auditor failures (e.g., grounding contradictions). Results in 5-20% stake loss.
- **Tier 3 (Liveness):** Missed passes or ignored questions. Results in 0.1-1% stake loss; pressure primarily comes from the EMA half-life.

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

## The equilibrium claim

**Under the Vortex Principle (tiered information quarantine), the joint strategy profile (honest, honest, honest, honest, honest, truthful) is a Bayes-Nash equilibrium of the staged game.**

The Vortex is load-bearing. Each player's quarantined view eliminates profitable deviations:

- **Triage** can't shape questions to fit a downstream plan — it doesn't know Sage exists.
- **Sage** can't hide reasoning failures — it doesn't know Auditor has cross-conversation memory.
- **Tribunal members** can't coordinate — they don't know who the Nemesis is.
- **Auditor** has full visibility, hence the 2-3x bonding and peer review.