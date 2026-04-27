---
title: Governance
parent: Architecture
---

# **The Tribunal Governance Model**

## **1. Core Philosophy**

The Tribunal Governance Model (TGM) replaces trust with **Heterogeneous Consensus**. Rather than relying on a single LLM to generate sensitive operator commands, g8e uses a diverse panel of specialized AI personas to arrive at a verified, robust, and safe command.

Trust is minimized through:
*   **Diverse Personas**: Different ideological lenses (minimalism, safety, edge-cases, convention, adversarial) surface distinct candidate commands.
*   **Uniform Voting**: Consensus is reached through uniform per-member voting (each member contributes exactly 1 vote per candidate) with deterministic tie-breaking.
*   **Multi-Round Peer Review**: If initial consensus is weak, the Tribunal enters an anonymized peer review round to resolve ambiguity.
*   **Independent Audit**: A primary-tier Auditor performs the final check against intent and safety constraints, cryptographically committing its verdict to a reputation scoreboard.
*   **Human-in-the-Loop**: No state-changing command is executed without explicit user approval.

## **2. The Governance Pipeline**

### **Phase 1: Triage (The Gatekeeper)**
Every user message is first processed by **Triage** (Lite-tier model). Triage classifies the message by:
*   **Complexity**: `Simple` vs. `Complex`. This determines if the request goes to **Dash** (Fast-path) or **Sage** (Senior Reasoner).
*   **Intent**: `Information` vs. `Action`.
*   **User Posture**: `Normal`, `Escalated`, `Adversarial`, or `Confused`.

### **Phase 2: Reasoning (Dash/Sage)**
*   **Dash (Assistant tier)**: Resolves simple, single-step requests with minimal latency.
*   **Sage (Primary tier)**: Handles complex, multi-step investigations. Sage articulates **Intent** in natural language to the Tribunal, never generating shell syntax directly.

### **Phase 3: The Tribunal (Consensus Engine)**
When a command is required, the Tribunal (5 Lite-tier models) generates candidates in parallel:
1.  **Generation**: Each member (Axiom, Concord, Variance, Pragma, Nemesis) produces one candidate command based on its specific lens.
2.  **Uniform Voting**: Commands are grouped. Each member contributes 1 vote to their candidate's cluster.
3.  **Consensus Check**: If a majority (3+ votes) is reached, the winner proceeds.
4.  **Round 2 (Peer Review)**: If consensus is low (strength < 0.4), members receive an anonymized list of all Round 1 candidates and are asked to review and re-generate.
5.  **Tie-Breaking**: If ties persist, the system applies deterministic rules: Shortest Command → Non-Nemesis Cluster → Alphabetical.

### **Phase 4: The Auditor (Final Judgment)**
The winner is sent to the **Auditor** (Primary-tier model) along with Sage's original intent. The Auditor can:
*   **Approve (`ok`)**: The command proceeds to the user.
*   **Swap (`swap`)**: If a dissenting cluster's command is superior, the Auditor can swap to it.
*   **Revise (`revised`)**: The Auditor can fix minor syntactic or safety flaws without changing intent.

Upon reaching a verdict, the Auditor creates a **Reputation Commitment**—a Merkle-root entry in the reputation scoreboard that cryptographically binds the Auditor's choice to the command's outcome.

## **3. The Cast of Entities**

| Persona | Role | Lens / Responsibility |
| :--- | :--- | :--- |
| **Triage** | Classifier | Message routing and posture detection. |
| **Dash** | Responder | Fast-path resolution for simple tasks. |
| **Sage** | Reasoner | Planning and natural-language intent articulation. |
| **Axiom** | Tribunal | **Composition**: Focuses on clean, multi-stage pipelines. |
| **Concord** | Tribunal | **Safety**: Focuses on read-only flags and defensive syntax. |
| **Variance** | Tribunal | **Edge Cases**: Handles spaces in paths, null input, and locales. |
| **Pragma** | Tribunal | **Convention**: Uses idiomatic OS-specific tools (e.g., `ss` vs `netstat`). |
| **Nemesis** | Tribunal | **Adversary**: Proposes subtly flawed but plausible commands to test the Auditor. |
| **Auditor** | Auditor | Final verification, dissent handling, and reputation commitment. |

## **4. Auditability & The LFAA**

Every decision in the TGM is recorded via the **Local-First Audit Architecture (LFAA)**. Audit events are published via pub/sub from the `g8ee` engine to the `g8eo` operator:
*   **User Intent Audit**: Sage's articulated request is recorded before command generation.
*   **Tribunal Audit**: Individual candidates, vote counts, and Round 2 details are preserved.
*   **Execution Audit**: The final command, human approval state, and execution output are captured immutably on the operator machine.

## **5. Safety Thresholds & Invariants**

*   **Sage-Tribunal Separation**: Sage (the "planner") never sees shell syntax; the Tribunal (the "translators") never sees the high-level plan. This prevents "hallucination spirals."
*   **Human-in-the-Loop**: The `run_commands_with_operator` tool always requires explicit user confirmation before the command hits the shell.
*   **Forbidden Patterns**: Deterministic regex filters block commands like `rm -rf /` or credential-leaking patterns regardless of AI consensus.
*   **Max Tool Turns**: The ReAct loop is capped at 25 turns to prevent infinite loops.


