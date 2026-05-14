---
title: AI Agents
parent: Architecture
---

# AI Agent Architecture

Last Updated: 2026-05-13
Version: v0.2.5

The g8e platform utilizes a specialized multi-agent system designed for autonomous infrastructure management. The architecture enforces a strict separation between **Reasoning** (the optional g8ee AI engine) and the **Substrate** (the mandatory g8eo Operator). This ensures that no action reaches a host without cryptographic proof of intent, consensus, and human authorization.

## Core Principles

- **3-Layer Governance Bedrock**: Every action is gated by a hierarchical validation system (L1 Technical Bedrock, L2 Consensus, L3 Authorization).
- **Intent-Driven Execution**: Reasoning agents (Sage/Dash) never write shell commands directly; they articulate natural language intent to the Tribunal.
- **Ensemble Consensus**: The Tribunal uses an independent five-member ensemble with unique technical "lenses" to translate intent into commands.
- **Host Sovereignty**: The Operator (g8eo) distrusts all upstream inputs. It verifies every transaction against the protocol before execution.
- **Fail-Closed Verification**: Any missing signature, stale state root, or L1 violation results in immediate transaction rejection.
- **Interrogation Gate**: Agents can pause execution to ask clarifying questions via structured `<interrogation>` blocks, preventing "guessing" when context is missing.

## The Agent Pipeline

Every user message triggers a structured pipeline managed by the `ChatPipelineService`.

### 1. Triage (The Gatekeeper)
Incoming messages are first processed by the **Triage Agent** using a `lite` model tier. It performs a "read of the room" and emits three classifications:
- **Complexity**: `simple` (routes to **Dash**) or `complex` (routes to **Sage**).
- **Intent**: `information` (knowledge retrieval), `action` (state change), or `unknown`.
- **Posture**: Gauges user mindset (`normal`, `escalated`, `adversarial`, `confused`) to calibrate downstream behavior.

*Note: Triage is a classifier only. It does not generate questions or interact with the user.*

### 2. Context Assembly
Before a reasoning agent receives the task, the system assembles a comprehensive world view:
- **Fleet & Operator Context**: Real-time system metadata from operator heartbeats.
- **Investigation Context**: Full conversation history and active tool outputs.
- **Memory (Codex)**: Durable user preferences and investigation summaries extracted asynchronously by the **Codex** agent.

### 3. Reasoning Agents
- **Dash (Fast-path)**: Resolves simple, single-turn requests with minimal latency using the `assistant` model tier.
- **Sage (Primary Reasoner)**: The senior reasoner for complex, multi-step investigations. Operates in a **ReAct loop**, planning operations and articulating intent using the `primary` model tier.

#### The Interrogation Protocol
If Dash or Sage encounters ambiguity, they must use the Interrogation Protocol:
- Issue exactly **three targeted YES/NO questions** in parallel.
- Questions must be strictly binary to maximize information gain.
- The `<interrogation>` block must be the entire response; tool execution is suppressed until the user answers.

### 4. The Tribunal (L2 Producer)
When a reasoning agent requests an action (e.g., `run_commands_with_operator`), it sends an **Intent** to the Tribunal.
- **Generation**: Five independent members produce candidate commands in parallel, blind to each other (**Information Isolation**).
- **Voting**: The ensemble reaches consensus on the optimal command string.
- **Nemesis (The Calibrated Adversary)**: One member (Nemesis) is tasked with proposing plausible but subtly flawed commands to stress-test the Auditor.

### 5. The Warden (Circuit Breaker)
The **Warden** performs pre-execution risk analysis (`LOW`, `MEDIUM`, `HIGH`) on the consensus winner.
- **Two-Strike Circuit Breaker**: If a command is classified as `HIGH` risk twice in a single investigation, it triggers an `AI_AGENT_CONFLICT_DETECTED` and halts execution, requiring human intervention.

## Governance & Safety: The L1/L2/L3 Hierarchy

The g8eo Operator enforces the 3-layer hierarchy as the mandatory substrate.

### Layer 1: Technical Bedrock (Hard Gates)
Enforced via reflected Protobuf options in `g8eo`.
- **Forbidden Patterns**: Global blocks on `sudo`, `su`, and other prohibited shell patterns.
- **Policy Enforcement**: Denies specific dangerous commands or path substrings based on host-local configuration.

### Layer 2: Consensus (The Tribunal)
The transaction must carry a valid ED25519 signature from the trusted Tribunal. The Operator verifies the signature against the `transaction_hash`.

### Layer 3: Authorization (Human-in-the-loop)
State-changing mutations require a hardware-bound signature (FIDO2/WebAuthn).
- **Auto-Approval**: Benign commands (e.g., `uptime`, `df`) defined in `auto_approved.json` can be marked as L3-authorized automatically *only if* they have passed all L1 and L2 gates.

## Reputation & Staking

The system maintains a per-agent reputation scalar `[0.0, 1.0]` as an EMA across conversations. Agents participate in **Reputation Staking** to ensure technical integrity.

| Persona | Lens | Primary Responsibility |
|---|---|---|
| **Axiom** | Composition | Clean, efficient multi-stage pipelines. |
| **Concord** | Safety | Defensive flags and read-only discipline. |
| **Variance** | Edge Cases | Robustness against spaces, locales, and nulls. |
| **Pragma** | Convention | Idiomatic, OS-specific best practices. |
| **Nemesis** | Adversary | Calibrated stress-test; rewarded for bypassing the Auditor. |
| **Auditor** | Verification | Accurate swaps, revisions, and intent fidelity. |
| **Warden** | Defense | Precise risk classification and circuit breaking. |

### Merkle Commitments
After every verdict, the Auditor writes a signed Merkle commitment of the agent reputation scoreboard. This ensures the platform's governance history is cryptographically verifiable and tamper-proof.

## Technical Invariants

- **Wire Format**: All client-facing transactions use **Canonical JSON (protojson)** for the `GovernanceEnvelope` (UAP).
- **Signing Basis**: Signatures are computed over a deterministic **transaction_hash** (computed from normalized envelope fields).
- **State Freshness**: The `state_merkle_root` binds the command to the host state at generation time; the Operator rejects stale transactions.
- **Information Isolation**: Triage is unaware of downstream reasoning; Tribunal members are blind to each other's candidates.
