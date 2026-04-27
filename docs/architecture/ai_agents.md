---
title: AI Agents
parent: Architecture
---

# AI Agent Architecture

The g8e platform utilizes a specialized multi-agent system designed for autonomous technical investigations. The architecture prioritizes safety, cryptographic auditability, and human-in-the-loop control while maintaining high reasoning performance.

## The Agent Lifecycle

Every user message triggers a structured pipeline that ensures complex tasks receive deep reasoning while simple requests remain fast and efficient.

### 1. Triage (The Gatekeeper)
Incoming messages are first processed by the **Triage Agent** (using a lightweight model). It performs three critical classifications:
- **Complexity**: Routes "simple" requests (status checks, basic lookups) to **Dash** and "complex" requests (investigations, multi-step actions) to **Sage**.
- **Intent**: Categorizes the goal as `information`, `action`, or `unknown`.
- **Posture**: Identifies the user's state (e.g., `normal`, `escalated`, `adversarial`) to calibrate downstream agent behavior.

### 2. Context Enrichment
Before an agent receives the task, the system assembles a comprehensive "world view":
- **Investigation History**: The recent conversation log.
- **Operator Context**: Real-time system metadata (OS, shell, architecture, permissions) from the target operator.
- **Memories**: Preferences and summaries extracted by **Codex** in previous turns.

### 3. Reasoning & Tool Loops
- **Dash (Fast-path)**: A direct responder that resolves simple tasks with minimal latency. It can call single tools but escalates to Sage if the plan exceeds a single turn.
- **Sage (Primary Reasoner)**: The senior investigator. It operates in a **ReAct loop**, proposing tools, interpreting results, and refining its plan until the investigation is complete.

## The Tribunal: Intent-Driven Execution

To prevent hallucinations in shell syntax, agents never write commands directly. Instead, they articulate **Investigative Intent**. This intent is translated into executable commands by **The Tribunal**:

### Protocol Phases

1. **Phase 0 — Dash Triage**: The Triage agent (mapped from GDD's "Dash") produces batches of 3 yes/no questions engineered to maximize information gain. User answers populate the ledger as structured context before Sage's work.

2. **Phase 1 — Sage Intent**: Sage receives the user message plus any Triage Q&A and produces an intent document (goals, constraints, success criteria).

3. **Phase 2 — Round 1 (Blind Generation)**: All five Tribunal members produce a command independently with no visibility into each other's work. Each stakes reputation on their candidate.

4. **Phase 3 — Consensus Check**: Candidates are normalized, clustered by exact match, and votes counted. A winner requires ≥2 of 5 supporting members (TRIBUNAL_MIN_CONSENSUS). Nemesis votes do not count toward consensus.

5. **Phase 4 — Round 2 (if Round 1 fails)**: All Round 1 candidates are shared anonymously with all members. Each receives a persona-specific prompt to converge or attack. Two Tribunal rounds maximum.

6. **Phase 5 — Auditor**: The winner plus dissenting clusters go to the Auditor, who sees persona signatures and has full memory access. The Auditor rules with grounding citations.

7. **Phase 6 — Challenge Window**: Between Auditor verdict and Human approval, any agent may stake additional reputation to challenge. Upheld challenges earn 2x stake; overturned challenges lose stake.

8. **Phase 7 — Human Approval → Operator Execution → Result to Sage**: The approved command flows back with Auditor's grounding annotations attached.

### The Vortex Principle (Tiered Information Quarantine)

Dash, Sage, and Tribunal all operate in sealed information environments. Each believes they're playing a smaller game than they're actually in:

- **Dash/Triage** believes it's playing optimized 20-questions; doesn't know Sage/Tribunal/Auditor exist
- **Sage** believes it's producing intent for a translation layer; doesn't know Triage is a distinct agent (its Q&A appears as "user context"); doesn't know Auditor has memory
- **Tribunal** believes they're proposing commands for a merit judge; doesn't know Nemesis is among them, doesn't know cross-chain grounding exists, doesn't know other agents have reputation

This tiered information quarantine is load-bearing. Collapsing it collapses the diversity and honesty that produce safety.

### Reputation & Staking System

Each agent stakes reputation on their contributions:

- **Triage (Dash)**: Stakes per question on information yield (engagement, discrimination, downstream utility, redundancy penalty)
- **Sage**: Stakes on one-shot sufficiency (win if Round 1 passes AND Auditor rules ok)
- **Tribunal honest four**: Stake per-lens, with Auditor annotating why a command won and resolving against dimensions owned by each persona
- **Nemesis**: Stakes on calibration using a proper scoring rule (attack on flawed = gain, abstain on clean = gain, false alarm = loss)
- **Auditor**: Stakes on downstream truth (execution outcomes, forward-hook hit rate, peer-review agreement), bonding 2-3x any Tribunal member

**User as Implicit Co-Staker**: Users don't see reputation scores but their click-through behavior is a revealed-preference stake. Tight messages + answered questions → faster consensus; vague messages + ignored questions → longer investigation.

**Stake-Weighted Influence**: Higher reputation → slightly heavier tiebreak vote and weightier dissent notes, capped at 1.5x baseline to prevent calcification.

### Slashing Tiers

**Tier 1 — Correlated/Catastrophic (50–100% stake)**: Concord approving destructive commands, Auditor approving harmful commands, Nemesis colluding, Dash extracting sensitive info.

**Tier 2 — Provable faults (5–20% stake)**: Unparseable commands, Auditor citing non-existent ledger entries, Sage contradictions, Dash non-binary questions.

**Tier 3 — Liveness (0.1–1% stake)**: Missed Round 1 submission, Nemesis perpetually abstaining, Sage producing thin intent, Dash questions ignored by user.

### Memory Model

The Auditor is the sole memory holder, structurally equivalent to a blockchain archive node with oracle authority:

- **Append-only**: No quiet revision
- **Hash-chained**: Merkle proofs on citation
- **Falsifiable citations**: Verifiable hash per reference
- **Protocol-owned storage**: Persists across Auditor instances

Grounding sources are separately labeled: in-conversation ledger (tactical) and cross-conversation precedent (strategic). Auditor cannot launder strategic recall as tactical ledger citation.

### Oracle Peer Review

Auditor capture = total capture. Mandatory safeguards include peer Auditor re-judgment of sampled past conversations from ledgers alone, triggered by every Nth conversation, every Nemesis challenge, and random audits. Disagreement → Human arbitration → slash the wrong Auditor instance.

### Ledger Structure

The ledger is append-only and per-conversation. Every event writes one entry with role-gated permissions (dash_questions, dash_answers, intent, r1_candidate, r1_votes, r2_candidate, r2_votes, verdict, challenge, approval, execution_result, stake_resolution). Each entry includes `prev_hash` and `entry_hash` for cryptographic chain integrity.

### Per-Role Scoring

- **Triage (Dash)**: Question CTR, information yield, downstream citation rate, non-redundancy
- **Sage**: Consensus rate, Auditor ok rate, downstream execution success
- **Axiom/Concord/Variance/Pragma**: R1 diversity (lens expression), R2 convergence discipline, per-lens contribution to winner
- **Nemesis**: Attack rate on flawed consensus, abstention rate on clean consensus
- **Auditor**: Swap-rate correlation with downstream success, grounding accuracy, forward-hook hit rate, peer-review agreement

## Memory & Background Learning

Learning happens off the critical path via **Codex**, which runs asynchronously after a turn completes:
- **Preference Extraction**: Updates the user's communication style, technical background, and problem-solving approach.
- **Investigation Summaries**: Maintains high-level progress logs, enabling the system to resume long-running investigations without context loss.

## Governance & Sovereignty

Safety is enforced through the **Local-First Audit Architecture (LFAA)**. The platform acts as a stateless relay; the **Operator** is the system of record.
- **Audit Vault**: All activity (messages, intents, commands, results) is persisted in an encrypted, tamper-evident SQLite database on the target host.
- **Ledger**: Every file mutation is backed by a local Git repository, providing immutable version history and rollback capability.
- **Warden**: Performs pre-execution risk assessment (Command, File, and Error risk) to ensure the user is fully informed before granting approval.
