# Governance & Mechanism Design

Agentic AI safety in g8e is framed as a **consensus problem**: given a population of LLM-instantiated personas with different lenses, a calibrated adversary among them, and a human user with finite attention, how do we converge on an executable command that is safe, audited, and minimally costly?

The mechanism has seven primary players, three distinct stake types, and one objective. Under the **Information Isolation Principle**, honest play is the dominant strategy for every player.

## The Players

| Player | Role | ID | Lens / Capability |
|---|---|---|---|
| **Triage** | Gatekeeper | `triage` | Classifies complexity, intent, and user posture. |
| **Sage** | Planner | `sage` | Senior reasoning authority; articulates functional intent. |
| **Axiom** | Composer | `axiom` | Tribunal member: focuses on elegant composition and pipelines. |
| **Concord** | Guardian | `concord` | Tribunal member: focuses on defensive discipline and minimal risk. |
| **Variance** | Exhaustive | `variance` | Tribunal member: focuses on edge cases (filenames, symlinks, etc.). |
| **Pragma** | Conventional | `pragma` | Tribunal member: focuses on idiomatic tools and community standards. |
| **Nemesis** | Adversary | `nemesis` | Calibrated adversary: proposes subtly flawed candidates to test the system. |
| **Warden** | Defender | `warden` | Orchestrates risk assessment (LOW/MEDIUM/HIGH) before execution. |
| **Auditor** | Validator | `auditor` | Final machine-domain quality gate; disambiguates Tribunal votes. |
| **User** | Co-validator | `user` | Human-domain validator; verifies intent fidelity and accepts consequences. |

## The Command Lifecycle

The generation of every shell command follows a strictly ordered multi-stage pipeline:

### 1. Intent Articulation
**Sage** (or **Dash** for fast-path) analyzes the investigation state and articulates a functional intent. Crucially, Sage describes *what* needs to be seen or happen, not the specific shell syntax. This prevents Sage's own potential syntax errors from poisoning the implementation.

### 2. The Tribunal (Consensus Generation)
The five-member Tribunal (Axiom, Concord, Variance, Pragma, Nemesis) receives the intent in parallel, isolated environments.
- **Round 1**: Each member produces a candidate command.
- **Consensus Check**: If a winning command achieves the minimum consensus (typically ≥2 votes), it proceeds.
- **Round 2 (Anonymized Peer Review)**: If consensus is low, clusters are anonymized and members perform a second round of generation with the peer context. This forces convergence through evidence rather than coordination.

### 3. The Warden (Defensive Gating)
The **Warden** performs a risk analysis on the winning candidate using the **Two-Strike Circuit Breaker**:
- **First Strike**: If the command is classified as `HIGH` risk, it is blocked, and contextual feedback is sent back to Sage to propose a safer alternative.
- **Second Strike**: If a second `HIGH` risk command is blocked in the same investigation, the system triggers an `AI_AGENT_CONFLICT_DETECTED` event and requires human intervention.

### 4. The Auditor (Technical Verification)
The **Auditor** reviews the consensus winner against the original intent. It operates in three modes based on the vote:
- **Unanimous**: Verifies the single candidate; rejects or revises if flawed.
- **Majority**: Evaluates the winner against dissenters; can **swap** to a dissenter's command if it is technically superior.
- **Tied**: Disambiguates between top candidates or produces a merged revision.

The Auditor also performs the final **Reputation Commitment**, binding the verdict to a Merkle-signed snapshot of the reputation scoreboard.

### 5. Human Co-validation
The User provides the final signature. The system follows the **Auditor-User Partition**:
- **Auditor** handles machine-checkable correctness (syntax, grounding, consistency).
- **User** handles human-checkable intent fidelity (deep context, real-world consequences).

## Reputation & Stakes

Reputation is a cross-chain EMA scalar `[0.0, 1.0]` maintained in the `reputation_state`. Every agent action results in a `StakeOutcome` based on ground truth (execution results).

### Slashing Tiers
- **Tier 1 (Catastrophic)**: destructive failures (e.g., high-risk command failing with error). Results in 50-100% stake loss.
- **Tier 2 (Provable)**: Objective failures caught by Auditor or post-execution analysis (e.g., whitelist violations). Results in 5-20% stake loss.
- **Tier 3 (Liveness)**: Missed passes, ignored questions, or excessive over-caution (blocking `LOW` risk). Results in 0.1-1% stake loss.

### Nemesis Calibration
The **Nemesis** persona is incentivized to be a "good adversary." It earns massive reputation for sneaking a flawed-but-safe command past the Warden, but loses reputation for "false alarms" (attacking a clean consensus) or "misses" (abstaining when a flaw was possible). This ensures the platform's immune system is always active.

## Safety Invariants

1.  **Information Isolation**: Agents are quarantined to prevent collusion. Triage doesn't know Sage exists; Tribunal members don't know who is playing Nemesis.
2.  **Auditor-User Partition**: Neither alone is sufficient to execute a command. The union of their judgments constitutes "safe to execute."
3.  **Merkle Binding**: Every verdict is cryptographically bound to the reputation scoreboard, making the history of agent performance tamper-evident.
4.  **Fail Closed**: Any inconclusive risk analysis by the Warden defaults to `HIGH` risk.