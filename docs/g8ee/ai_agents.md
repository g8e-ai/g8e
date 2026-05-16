---
title: AI Agents
parent: Architecture
---

# AI Agent Architecture

Last Updated: 2026-05-16
Version: v0.3.0

The g8e platform utilizes a specialized multi-agent system designed for autonomous infrastructure management. The architecture enforces a strict separation between **Reasoning** (the application layer, e.g., g8ee) and the **Substrate** (the mandatory g8eo Operator). This ensures that no action reaches a host without cryptographic proof of intent, consensus, and human authorization.

## Core Principles

- **3-Layer Governance Bedrock**: Every action is gated by a hierarchical validation system (L1 Technical Bedrock, L2 Consensus, L3 Authorization).
- **Intent-Driven Execution**: Reasoning agents (Sage/Dash) never write shell commands directly; they articulate natural language intent to the Tribunal.
- **Ensemble Consensus**: The Tribunal uses an independent multi-member ensemble with unique technical "lenses" to translate intent into commands.
- **Host Sovereignty**: The Operator (g8eo) distrusts all upstream inputs. It verifies every transaction against the protocol before execution.
- **Fail-Closed Verification**: Any missing signature, stale state root, or L1 violation results in immediate transaction rejection.
- **Interrogation Gate**: Agents can pause execution to ask clarifying questions via structured `<interrogation>` blocks, preventing "guessing" when context is missing.

## The Agent Lifecycle

Every user request moves through a structured pipeline that terminates at the g8eo execution boundary.

### 1. Triage (The Gatekeeper)
Incoming messages are first processed by the `TriageAgent` (`@/home/bob/g8e/services/g8ee/app/services/ai/triage.py`) using a `lite` model tier. It performs a "read of the room" and emits three classifications:
- **Complexity**: `simple` (routes to **Dash**) or `complex` (routes to **Sage**).
- **Intent**: `information` (knowledge retrieval), `action` (state change), or `unknown`.
- **Posture**: Gauges user mindset (`normal`, `escalated`, `adversarial`, `confused`) to calibrate downstream behavior.

*Note: Triage is a classifier only. It does not generate questions or interact with the user.*

### 2. Context Assembly
Before a reasoning agent receives the task, the system assembles a comprehensive world view:
- **Fleet & Operator Context**: Real-time system metadata from operator heartbeats.
- **Investigation Context**: Full conversation history and active tool outputs.
- **Memory (Codex)**: Durable user preferences and investigation summaries extracted asynchronously.

### 3. Reasoning Agents
- **Dash (Fast-path)**: Resolves simple, single-turn requests with minimal latency using the `assistant` model tier.
- **Sage (Primary Reasoner)**: The senior reasoner for complex, multi-step investigations. Operates in a **ReAct loop**, planning operations and articulating intent using the `primary` model tier.

#### The Interrogation Protocol
If Dash or Sage encounters ambiguity, they must use the Interrogation Protocol:
- Issue exactly **three targeted YES/NO questions** in parallel.
- Questions must be strictly binary to maximize information gain.
- The `<interrogation>` block must be the entire response; tool execution is suppressed until the user answers.

### 4. The Tribunal (L2 Producer)
When a reasoning agent requests an action (e.g., `run_commands_with_operator`), it sends an **Intent** to the Tribunal (`@/home/bob/g8e/services/g8ee/app/services/ai/generator.py`).
- **Generation**: Five independent members produce candidate commands in parallel, blind to each other (**Information Isolation**).
- **Voting**: The ensemble reaches consensus on the optimal command string using weighted majority voting with deterministic tie-breaking.
- **Audit**: An independent **Auditor** model reviews the consensus winner against the original intent, potentially revising or swapping it for a better candidate.

### 5. The Warden (The Execution Boundary)
The **Warden** (`@/home/bob/g8e/services/g8ee/app/services/ai/tribunal/stages/warden.py`) is the final gate before a mutation is signed and dispatched to the Operator.
- **Risk Analysis**: Performs pre-execution risk analysis (`LOW`, `MEDIUM`, `HIGH`) on the audited command.
- **Two-Strike Circuit Breaker**: If a command is classified as `HIGH` risk twice in a single investigation, it triggers an `AI_AGENT_CONFLICT_DETECTED` and halts execution, requiring human intervention.
- **Receipt Issuance**: Upon successful execution, the Warden (on the g8eo side) issues a signed `ActionReceipt` (`@/home/bob/g8e/protocol/proto/operator.proto:349`) as proof of the mutation.

## Governance: The L1/L2/L3 Hierarchy

The g8eo Operator enforces the 3-layer hierarchy as the mandatory substrate.

### Layer 1: Technical Bedrock (Hard Gates)
Enforced via reflected Protobuf options in `g8eo`.
- **Forbidden Patterns**: Global blocks on `sudo`, `su`, and other prohibited shell patterns.
- **Policy Enforcement**: Denies specific dangerous commands or path substrings based on host-local configuration.

### Layer 2: Consensus (The Tribunal)
The transaction must carry a valid ED25519 signature from the trusted Tribunal. The Operator verifies the signature against the `transaction_hash`.

### Layer 3: Authorization (Human-in-the-loop)
State-changing mutations require a hardware-bound signature (WebAuthn/FIDO2).
- **Auto-Approval**: Benign commands (e.g., `uptime`, `df`) defined in `auto_approved.json` can skip manual L3 approval *only if* they have passed all L1 and L2 gates.

## Technical Invariants

- **Canonical Envelope**: All mutations are encapsulated in the `UniversalEnvelope` (`GovernanceEnvelope` in `@/home/bob/g8e/protocol/proto/common.proto:60`).
- **Wire Format**: Client-facing transactions use **Canonical JSON (protojson)** for the envelope.
- **Signing Basis**: Signatures are computed over a deterministic **transaction_hash** computed from normalized envelope fields.
- **State Freshness**: The `state_merkle_root` binds the command to the host state at generation time; the Operator rejects stale transactions.
- **Information Isolation**: Triage is unaware of downstream reasoning; Tribunal members are blind to each other's candidates.

## Persona Matrix

| Persona | Lens | Primary Responsibility |
|---|---|---|
| **Axiom** | Composition | Clean, efficient multi-stage pipelines. |
| **Concord** | Safety | Defensive flags and read-only discipline. |
| **Variance** | Edge Cases | Robustness against spaces, locales, and nulls. |
| **Pragma** | Convention | Idiomatic, OS-specific best practices. |
| **Nemesis** | Adversary | Calibrated stress-test; rewarded for bypassing the Auditor. |
| **Auditor** | Verification | Accurate swaps, revisions, and intent fidelity. |
| **Warden** | Defense | Precise risk classification and circuit breaking. |
