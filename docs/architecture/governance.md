# **The Tribunal Governance Model**

## **1. Core Philosophy**

The Tribunal Governance Model (TGM) replaces trust with **Heterogeneous Consensus**. Rather than relying on a single LLM to generate sensitive operator commands, g8e uses a diverse panel of specialized AI personas to arrive at a verified, robust, and safe command.

Trust is minimized through:
*   **Diverse Personas**: Different ideological lenses (minimalism, safety, edge-cases, convention, adversarial) surface distinct candidate commands.
*   **Weighted Voting**: Consensus is reached through a position-decay voting mechanism that favors early, high-confidence candidates while still considering the full panel.
*   **Independent Verification**: A separate, fast validator (The Verifier) performs the final check against the user's intent and safety constraints.
*   **Human-in-the-Loop**: No state-changing command is executed without explicit user approval.

## **2. The Cast of Entities**

### **A. Triage (The Gatekeeper)**

*   **Role**: The first line of observation. Evaluates incoming user messages.
*   **Action**: Classifies message complexity (Simple vs. Complex), intent, and **User Posture** (Normal, Escalated, Adversarial, Confused).
*   **Impact**: Complexity determines the model tier (Primary vs. Assistant); Posture calibrates the caution and tone of downstream agents.

### **B. Sage (The Architect)**

*   **Role**: The primary reasoning agent (Primary Model) responsible for diagnosis and planning.
*   **Action**: Orchestrates the ReAct loop. When a command is needed, Sage sends a natural-language `request` and `guidelines` to the Tribunal.
*   **Constraint**: Sage **never** generates shell commands directly. It only describes the desired outcome.

### **C. The Tribunal (The Consensus Engine)**

A panel of five specialized assistant-tier models that independently generate the command Sage requested:

1.  **Axiom (The Minimalist)**: Pass 0. Produces the smallest viable command. Simplicity is safety.
2.  **Concord (The Guardian)**: Pass 1. Focuses on defensive flags and non-destructive operations.
3.  **Variance (The Exhaustive)**: Pass 2. Handles edge cases, spaces in paths, and robustness.
4.  **Pragma (The Conventional)**: Pass 3. Uses idiomatic patterns for the specific OS and shell.
5.  **Nemesis (The Adversary)**: Pass 4. Acts as a red-team member, surfacing potential attack surfaces.

### **D. The Verifier (The Final Judgment)**

*   **Role**: A dedicated validator (sometimes referred to as the Auditor in persona registry).
*   **Action**: Receives the voting winner and Sage's original request.
*   **Output**: Either the literal string `ok` (approval) or a minimal syntactic revision. It cannot rewrite intent, only fix flaws.

## **3. The Command Generation Pipeline**

When a `run_commands_with_operator` tool is invoked, the following pipeline fires:

1.  **Parallel Generation**: All 5 Tribunal members receive the request + operator context (OS, Shell, User, PWD) and generate independent candidates.
2.  **Normalization**: Commands are stripped of markdown fences and surrounding whitespace.
3.  **Weighted Voting**: Unique commands are grouped. Each occurrence receives a weight based on its pass index: `weight = 1 / (index + 1)`. The command with the highest aggregate weight wins.
4.  **Verification**: The winner is sent to the Verifier. If the Verifier rejects it, the revision is used (if provided).
5.  **Guardrails**: The result is checked against deterministic `FORBIDDEN_COMMAND_PATTERNS` (e.g., `rm -rf /`).
6.  **Approval**: The final command is presented to the user for explicit approval.

## **4. Auditability & The LFAA**

Every action in the governance model is recorded via the **Local-First Audit Architecture (LFAA)**:

*   **User Message Audit**: Recorded on the operator before LLM processing starts.
*   **AI Response Audit**: The full reasoning and tool calls are recorded.
*   **Command Execution Audit**: The exact command, its approval state, and its output are captured by the `g8eo` agent on the operator machine.

This ensures a persistent, immutable trail of how a decision was reached and who approved it.

## **5. Safety Thresholds**

To prevent infinite loops or "hallucination spirals," the system enforces structural limits:

*   **Max Tool Turns**: The ReAct loop is capped (default: 25 turns). Exceeding this requires explicit "Agent Continue" approval from the user.
*   **Forbidden Patterns**: Deterministic regex blacklists that no Tribunal output can bypass.
*   **Identity & Loyalty**: Core system prompts (Identity, Safety, Loyalty, Dissent) are injected on every turn, ensuring the agent remains aligned with its mission and safety constraints.

