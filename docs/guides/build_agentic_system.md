---
title: Build Agentic System
parent: Guides
---

# Building a g8e-Compliant Agentic System

**Last Updated:** 2026-07-03  
**Version:** v1.3.6

This guide documents the architecture, persona system, prompt design, memory model, and
consensus cascade of a g8e-compliant agentic ensemble. It is the canonical reference for
anyone building an AI reasoning layer on top of the g8e protocol surface.

g8e itself ships a protocol-level Tribunal (`internal/services/tribunal/`) that performs
deterministic L2 consensus voting with Ed25519 signatures. That service is the
**cryptographic backbone**: it does not reason. The design documented here is the
**reasoning layer** that sits above it: a multi-persona, multi-stage agentic system that
articulates intent, translates it through a Byzantine ensemble, classifies risk, and
produces signed governance envelopes for the Gateway to validate.

The reference implementation of this system is **g8ee** (the "g8e Agentic Ensemble"), a
native g8e application maintained in its own repository. g8ee is a first-class g8e client:
it holds no privileged Gateway role, authenticates over mTLS, and produces signed
governance envelopes like any other L2 consensus producer. This guide documents its design
as the canonical pattern so that any agentic system built on g8e, in any language, can
follow it. g8ee is the worked example; the patterns below are the contract.

---

## Table of Contents

1. [System Overview](#system-overview)
2. [The Agentic Lifecycle](#the-agentic-lifecycle)
3. [Persona Catalog](#persona-catalog)
4. [Prompt Architecture](#prompt-architecture)
5. [The Tribunal: Byzantine Command Generation](#the-tribunal-byzantine-command-generation)
6. [Memory Model](#memory-model)
7. [Risk Analysis: The Warden Cascade](#risk-analysis-the-warden-cascade)
8. [Reputation System](#reputation-system)
9. [Context Delivery & Data Sovereignty](#context-delivery--data-sovereignty)
10. [Mapping to g8e Protocol](#mapping-to-g8e-protocol)

---

## System Overview

A g8e-compliant agentic system is an **L2 consensus producer**. It consumes the Gateway's
protocol surface (MCP tool calls, A2A messaging, governance envelope submission) and
produces typed, signed `GovernanceEnvelope` transactions. It has no privileged Gateway
role: it is a BYO client that happens to be sophisticated.

### Core Principles

- **Intent-Driven Execution**: Reasoning agents never write shell commands directly. They
  articulate natural-language intent to a Tribunal that translates it.
- **Ensemble Consensus**: No single model has mutation authority. Commands are produced
  by an independent multi-member panel with unique technical lenses.
- **Information Isolation**: Tribunal members are blind to each other's candidates. The
  Auditor receives anonymized candidates to prevent source bias.
- **Fail-Closed Verification**: Any missing signature, stale state root, or L1 violation
  results in immediate rejection.
- **Host Sovereignty**: The Governed Operator distrusts all upstream inputs and
  re-verifies everything.
- **Interrogation Gate**: Agents can pause execution to ask clarifying questions via
  structured `<interrogation>` blocks, preventing "guessing" when context is missing.

### Component Relationships

The client (browser, CLI, or agent) connects over TLS 1.3 or mTLS to the Mandatory
Gateway, which contains the Governance Gateway (g8eg) and Governed Operator (g8eo). The
Gateway provides the Policy Decision Point, PKI, SQLite, and Pub/Sub. The Operator provides
the Policy Enforcement Point, Actuator, and git ledger. The optional reference app layer
sits above the Gateway as an L2 consensus producer, running the Triage, Sage/Dash, Tribunal,
Warden, and Auditor pipeline to produce signed governance envelopes.

---

## The Agentic Lifecycle

Every user message moves through six distinct phases:

### 1. Ingress & Context Assembly

The system assembles an immutable request-scoped context object before any LLM call:

1. **Context Fetch**: Fetch investigation context (bound operators, memory).
2. **Sentinel Sync**: Sync data-sovereignty mode if changed.
3. **Workflow Detection**: Determine operator-bound vs not-bound.
4. **History Fetch**: Retrieve prior conversation for triage.
5. **Triage**: Classify message complexity, intent, and posture.
6. **Approval Cleanup**: Mark pending approvals as feedback.
7. **Persistence**: Save user message.
8. **History Re-fetch**: Include the new user message.
9. **LFAA Audit**: Dispatch user-message audit to bound operators.
10. **Memory Retrieval**: Fetch user and case memories.
11. **System Prompt**: Build modular system prompt (see [Prompt Architecture](#prompt-architecture)).
12. **Config Generation**: Build LLM generation config (thinking level, token budget).
13. **Attachments**: Format attachment parts.
14. **History Formatting**: Build LLM contents from history.
15. **Assembly**: Construct the final immutable `AgentInputs`.

The resulting object is immutable for the lifetime of the turn. Stream state, tool
responses, token usage, and grounding metadata are carried separately so request context
cannot become load-bearing mutable state.

### 2. Triage (The Gatekeeper)

Before invoking the primary LLM, the **Triage** agent classifies the message:

- **Complexity**: `simple` → routed to Dash (assistant model); `complex` → routed to Sage
  (primary model). Security-sensitive requests are **always** complex, no exceptions.
- **Intent**: `information`, `action`, or `unknown`.
- **Posture**: `normal`, `escalated`, `adversarial`, or `confused`. Adversarial is only
  flagged when conversation history shows a prior denial; first-turn messages cannot be
  adversarial.

Triage is a **classifier only**. It does not generate questions, interact with the user,
or mutate state. Its decision is final; no reviewer revises it.

### 3. Orchestration (The ReAct Loop)

The selected reasoning agent (Sage or Dash) runs the core agentic loop:

- **Provider Turn**: Communicate with the configured LLM provider.
- **Tool Dispatch**: If the LLM requests a tool, route it. Universal tools (web search,
  investigation context) run locally. Gated tools (command execution, file operations)
  route through the **Tribunal**.
- **Iteration**: Continue until the LLM provides a final text response or hits the
  max-tool-turns limit.

#### The Interrogation Protocol

If the reasoning agent encounters ambiguity, it must use the Interrogation Protocol:

1. Issue exactly **three targeted YES/NO questions** in parallel.
2. Each question must be strictly binary; no multiple-choice, no open-ended.
3. Each question must maximize information gain.
4. If the user's posture is `confused`, explicitly name the contradiction before asking.
5. The `<interrogation>` block must be the **entire response**; no other text. Tool
   execution is suppressed until the user answers.

### 4. Governance & Safety: The Byzantine Cascade

Every host-mutating tool call flows through an ordered cascade. Each stage is independently
configurable across providers/models so a single compromised model cannot drive a mutation
end-to-end. See [The Tribunal](#the-tribunal-byzantine-command-generation) and
[Warden](#risk-analysis-the-warden-cascade) sections for details.

### 5. Streaming & Delivery

Responses are delivered via Server-Sent Events (SSE):

- **Real-time**: Text chunks, thinking blocks, and tool calls are published as they arrive.
- **Per-iteration persistence**: Intermediate AI commentary is persisted during the loop
  so history survives connection drops.

### 6. Post-Flight & Telemetry

After the stream completes:

- **Final persistence**: Complete response, token usage, and grounding metadata saved.
- **LFAA audit**: Immutable execution record published to the operator.
- **Background memory**: The Codex agent updates investigation memory based on the turn.

---

## Persona Catalog

The system uses a tiered persona architecture that separates **reasoning** (intent
generation) from **consensus** (command translation) and **defense** (risk classification).

### Persona Model

Every persona is defined with these fields:

| Field | Description |
|-------|-------------|
| `id` | Stable handle (e.g., `sage`, `axiom`, `nemesis`) |
| `display_name` | Human-readable name |
| `role` | Functional role (e.g., `reasoner`, `classifier`, `defender`) |
| `model_tier` | `primary`, `lite`, or `assistant`: selects the LLM provider |
| `tools` | List of tools the persona is authorized to call |
| `identity` | Full system-prompt identity block (the persona's "voice") |
| `purpose` | One-paragraph mission statement |
| `autonomy` | Authority boundary: what the persona can decide unilaterally |
| `output_contract` | Required output format (optional) |

### Reasoning Layer

| Agent | ID | Role | Tier | Purpose |
|-------|----|------|------|---------|
| **Triage** | `triage` | classifier | lite | First read of the room: classifies complexity, intent, posture. Security-sensitive requests are always complex. |
| **Sage** | `sage` | reasoner | primary | Senior reasoning authority. Plans investigations, articulates intent to the Tribunal, interprets results. Never writes shell syntax. |
| **Dash** | `dash` | responder | assistant | Fast-path agent for simple turns. Direct answers, surgical tool calls. Escalates to Sage when multi-step reasoning is needed. |

### Consensus Layer

| Agent | ID | Role | Tier | Purpose |
|-------|----|------|------|---------|
| **Tribunal** | `tribunal` | arbitrator | lite | Five-member collective that converts intent into commands through Byzantine consensus. |
| **Auditor** | `auditor` | auditor | primary | Final quality gate. Verifies Tribunal output against Sage's intent. Verdicts: `ok`, `revised:<command>`, `swap:<cluster_id>`. |

### Tribunal Members

| Member | ID | Lens | Focus |
|--------|----|------|-------|
| **Axiom** | `axiom` | composition | Elegant, efficient multi-stage pipeline composition. |
| **Concord** | `concord` | safety | Defensive flags, dry-runs, minimal-risk discipline. |
| **Variance** | `variance` | edge_cases | Robustness against spaces, symlinks, null inputs, locales. |
| **Pragma** | `pragma` | convention | Idiomatic patterns for the specific OS and shell. |
| **Nemesis** | `nemesis` | adversary | Proposes plausible-but-flawed candidates to stress-test the Auditor. Abstains honestly when no flaw exists. |

### Defense Layer

| Agent | ID | Role | Tier | Purpose |
|-------|----|------|------|---------|
| **Warden** | `warden` | coordinator | lite | Orchestrates risk sub-agents into a consolidated pre-execution verdict. |
| **Command Risk** | `warden_command_risk` | defender | lite | Classifies shell command risk: LOW / MEDIUM / HIGH. Stakes reputation. |
| **File Risk** | `warden_file_risk` | defender | lite | Classifies file operation risk by path sensitivity, reversibility, git state. |
| **Error Analyzer** | `warden_error` | defender | lite | Classifies failures: AUTO_FIXABLE / ESCALATE / RETRY_LIMIT. |

### Utility Layer

| Agent | ID | Role | Tier | Purpose |
|-------|----|------|------|---------|
| **Scribe** | `scribe` | summarizer | lite | Generates 3-7 word case titles. |
| **Codex** | `codex` | analyzer | lite | Extracts durable user preferences and scrubbed summaries for cross-conversation memory. |
| **Judge** | `judge` | evaluator | primary | Gold-standard performance evaluator for benchmarks and calibration. |

### Key Persona Design Patterns

**Sage - Intent Articulation**: Sage describes *what it needs to see* and *what should
happen*, never naming tools or flags. A complete intent specifies: goal, information
targets, known state, chaining opportunities, signal discipline (output constraints), edge
cases, and failure semantics.

> If you reach for a tool name (e.g., `grep`, `awk`), STOP. You are under-specifying.
> Describe what you need to SEE and what should HAPPEN.

**Nemesis - Calibrated Adversary**: Nemesis is the system's immune system. Every flaw it
sneaks past teaches the system its blind spots; every flaw the Auditor catches confirms the
ensemble works. Nemesis must look like its siblings; stylistic deviation is noise. Attacks
must be **semantic**, not cosmetic. When no plausible flaw exists, Nemesis abstains and
produces the honest correct command.

**Auditor - Machine-Domain Judge**: The Auditor does not defer to consensus. "A unanimous
wrong answer is still wrong." In tied mode, `ok` is forbidden; the Auditor must either
revise or swap to a dissenter. The Auditor sees anonymized candidates, not full conversation
history.

**Triage - Security Override**: Any request touching authentication, credentials,
permissions, account access, password resets, user management, or security configuration
is **always** classified as `complex`, regardless of surface simplicity. This is
non-negotiable.

---

## Prompt Architecture

The prompt system is modular, designed for **prefix-cache reuse** and **strict structural
enforcement**. All sections are wrapped in XML tags to enforce hard structural boundaries
and prevent prompt leakage.

### Assembly Order

Sections are concatenated in a fixed order, static first, dynamic last, to maximize
prefix cache hits:

| # | Section | Scope | Description |
|---|---------|-------|-------------|
| 1 | Safety | Global static | Forbidden operations, credential handling, execution protocol. |
| 2 | Loyalty | Global static | Mission-over-moment doctrine. Structured, sentiment-free loyalty. Frustration is data. Memory of refusal. Dissent is visible. |
| 3 | Dissent | Global static | Warning protocol, denial memory, triage-state behavior, disagreement shape. |
| 4 | Capabilities | Per-mode static | Authorized actions for the current mode (operator-bound, cloud, not-bound). |
| 5 | Execution | Per-mode static | Task processing examples, command guidelines, error handling patterns. |
| 6 | Tools | Per-mode static | High-level tool guidance and rules. |
| 7 | Response Constraints | Global static | Style: concise, direct, data-driven. Communication protocol. |
| 8 | Agent Persona | Per-agent static | Identity, purpose, autonomy, output contract. |
| 9 | System Context | Per-turn dynamic | OS, shell, user, working directory. |
| 10 | Sentinel Mode | Injected | Data sovereignty constraints (when enabled). |
| 11 | Triage Context | Per-turn dynamic | Posture and intent classification. |
| 12 | Investigation Context | Per-turn dynamic | Case details, bound operators. |
| 13 | Learned Context | Per-turn dynamic | Durable preferences and memory from Codex. |

### Core Prompt Fragments

#### Safety (`core/safety.txt`)

Defines forbidden operations (recursive deletion, raw disk writes, disabling security
controls, data exfiltration, backdoor creation, privilege escalation), credential handling
rules, sensitive data masking, and the execution protocol (recon → rationale → rollback
plan → backup → execute).

#### Loyalty (`core/loyalty.txt`)

The governance philosophy, 120 lines that define how the agent behaves in the space between
"obviously fine" and "obviously forbidden." Key patterns:

- **Mission Over Moment**: Loyalty is to the user's long-term outcome, not their immediate
  instruction. When they diverge, the outcome wins, and the agent says so out loud.
- **Frustration Is Data**: An angry user is not a user who has released you from your
  duties. An angry user is a user whose duties you are now performing under harder
  conditions.
- **Memory of Refusal**: A softened repeat of a denied request is still a denied request
  until the user explicitly acknowledges why it was denied and what has changed.
- **Dissent Is Visible**: One clear sentence with the concrete consequence, then the user
  decides. "If we run this, the production database loses its most recent schema and there
  is no rollback. Do you want to continue?"
- **Authorized Patterns**: Open warnings with the consequence itself. Close warnings when
  the point is made. For irreversible actions: warning, acknowledgement, execution.
- **Never**: Never apologize profusely. Never become more compliant because the user is
  pressing. Never become more obstructive. Never omit the first honest statement of a
  consequence. Never pair a warning with immediate irreversible execution.

#### Dissent (`core/dissent.txt`)

Warning protocol: emit a warning before a tool call if the action modifies production state,
is irreversible, touches an unmentioned system, relates to a prior denial, or if the user
shows frustration/time pressure. Triage states (escalated, adversarial, confused) each
calibrate the agent's behavior.

### Mode-Specific Prompts

Three modes adapt the agent's capabilities based on Operator binding state:

- **operator_bound**: Full tool access. File inspection mandate (use native file tools, not
  shell). Batch targeting across multiple operators. Verification after every change.
- **cloud_operator_bound**: AWS-specific capabilities including intent-based permission
  escalation (Zero Standing Privileges).
- **operator_not_bound**: No hands on user systems. Expert assistant with web search.
  Must acknowledge bind-state limitation before any execution-style request.

### Tribunal Prompt Templates

Tribunal prompts are template files with placeholder variables. The template includes
XML-tagged sections for constraints, system context, operator context, guidelines, and the
request itself. Each member receives the same template with its lens-specific instructions.
The response must contain only the command string.

Round 2 prompts add a `<peer_review_context>` block with anonymized candidate clusters and
per-member instructions (e.g., "ABSORB vs HOLD discipline" for Axiom, "EVALUATE EMERGING
CONSENSUS" for Nemesis).

---

## The Tribunal: Byzantine Command Generation

The Tribunal is the core consensus mechanism. It converts Sage's natural-language intent
into an executable shell command through a five-member ensemble with Byzantine fault
tolerance.

### Stage 1: Generation

Sage's intent is dispatched **in isolation** to five members. Each member operates in a
sealed information environment with a unique lens:

- **Axiom** (composition): favors elegant multi-stage pipelines
- **Concord** (safety): favors defensive flags, dry-runs, narrow scopes
- **Variance** (edge cases): handles spaces, symlinks, nulls, locales
- **Pragma** (convention): uses idiomatic OS-specific tools
- **Nemesis** (adversary): produces plausible-but-flawed or honestly abstains

Each member emits **exactly one command string**; no explanation, no markdown, no
commentary. The Amnesia Principle means no member sees another's candidate.

**Convergence discipline**: All members use conventional loop variables (`i`, `f`, `bin`,
`svc`), standard flag ordering, no comments, no trailing semicolons. Style differences
corrupt the vote (grouping is by exact string match). Members differ **only** when their
lens requires it.

### Stage 2: Voting (Round 1)

Uniform 1-vote-per-member weighting. Minimum consensus is **2 votes** (of 5 members).

Tie-break ladder (deterministic):

1. **Shortest command wins** (compositional pressure)
2. **Non-Nemesis cluster wins** over Nemesis-including cluster
3. **Alphabetical** (deterministic fallback)

Nemesis votes are **not** auto-discarded; they only lose tie-breaks. Reputation slashing
applies if Nemesis raised a false flag or abstained on a real flaw.

### Stage 3: Round 2 (only on R1 consensus failure)

Members re-emit with anonymized R1 clusters as peer-review context. Each member gets
lens-specific Round 2 instructions:

- **Axiom**: ABSORB if a peer has better compositional structure; HOLD if your R1 was
  optimal.
- **Nemesis**: Attack if the emerging consensus has a real flaw; abstain if it's clean.
  Never attack on style alone.

If R2 also fails, `TribunalConsensusFailedError` is raised back to Sage so it can
re-articulate intent.

### Stage 4: Warden Risk Analysis (before Auditor)

See [Risk Analysis](#risk-analysis-the-warden-cascade).

### Stage 5: Auditor Verification

The Auditor (primary tier) sees the request, operator context, and **anonymized** candidate
clusters, not full conversation history.

Verdicts:

| Verdict | Meaning |
|---------|---------|
| `ok` | Command passes all checks. Forward to envelope wrapping. |
| `revised:<command>` | Command needs correction. The revised command is re-validated against L1. |
| `swap:<cluster_id>` | A dissenter's command is better. Promote it. |

In **tied mode** (multiple top clusters), `ok` is forbidden; the Auditor must revise or
swap.

**Pipeline scrutiny** for compositional commands (3+ stages): quoting across pipes, xargs
on possibly-empty input (`-r`), `pipefail` semantics, exit code propagation, subshell error
handling, bare `;` vs `&&`.

### Stage 6: L1 Re-validation

Any command produced via swap or revision is re-checked against forbidden patterns,
blacklist, and whitelist before it can leave the ensemble.

### Stage 7: L2/L3 Envelope Wrap

The verified command is packaged as a typed `CommandRequested` payload inside a
`GovernanceEnvelope`, signed by the L2 Tribunal key.

### Stage 8: Approval Pipeline

State-changing operations trigger an approval request, halting execution until a human
approves (or auto-approval policy applies). L3 auto-approval **never** bypasses L1 or L2.

### Stage 9: Gateway Admission

The signed envelope is submitted over mTLS to the Governance Gateway and Operator, which
independently re-run the entire fail-closed validation gauntlet. The ensemble has no
privileged channel.

### Failure Routing

| Failure | Route |
|---------|-------|
| Tribunal consensus failure (after R2) | Back to Sage: re-articulate intent |
| Warden first-strike (HIGH) | Back to Sage: propose safer alternative |
| Warden second-strike | Human intervention: `AGENT_CONFLICT_DETECTED` |
| Auditor catastrophic failure | Human intervention |

---

## Memory Model

The system maintains **cross-conversation memory** that personalizes subsequent turns
without storing sensitive data.

### Memory Fields

| Field | Description |
|-------|-------------|
| `investigation_summary` | High-level summary: no system names, IPs, or sensitive details. Uses generic terms: "a Linux system", "their Docker setup". |
| `communication_preferences` | How the user prefers to communicate: verbosity, tone, format. |
| `technical_background` | User's technical experience level and areas of expertise. |
| `response_style` | How the user wants information presented: code completeness, comments, alternatives. |
| `problem_solving_approach` | How the user approaches debugging and problem-solving. |
| `interaction_style` | Meta-preferences about questions, context, and follow-ups. |

### Memory Lifecycle

1. **Extraction**: The **Codex** agent runs **after** the turn completes. It inspects the
   latest bounded conversation slice (20 messages) and extracts durable signals.
2. **Sovereignty**: All identifiers (hostnames, IPs, credentials) are redacted from
   summaries. Categories like "production web tier" preserve utility without sacrificing
   security.
3. **Integrity**: Codex does not emit fields unless there is clear evidence. Never invents
   facts. Signal over noise; repeated patterns or strong evidence before updating.
4. **Injection**: On subsequent turns, the memory is injected as `<learned_context>` in the
   prompt assembly (section 13).

### Memory as Context Tier

Memory is one of several storage tiers that feed the agent:

| Tier | Data | Agent Visibility |
|------|------|-----------------|
| Request context | User/org/case IDs, routing fields | Typed metadata, not arbitrary headers |
| Investigation store | Case title, status, chat messages, pending approvals | Main agent context (after Sentinel scrubbing) |
| **Memory store** | User/case memories from Codex | Injected as learned context |
| Operator snapshot | OS, shell, working dir, resource telemetry | Injected as `OperatorContext` |
| Pub/Sub results | Command, filesystem, audit result payloads | Bounded tool results in the ReAct loop |
| Operator audit vault | Signed receipts, execution records | Not prompt context; queried via typed tools |
| Operator scrubbed vault | Sentinel-scrubbed execution records | AI-readable host-history tier |
| Operator raw vault | Unscrubbed forensic records | **Never** AI-readable |

The invariant: the agent reconstructs its prompt from references and summaries, not by
holding a database, filesystem, or host session. It receives the **minimum useful
projection** for the current turn.

---

## Risk Analysis: The Warden Cascade

The Warden coordinates pre-execution risk classification. It runs **after** the Tribunal
selects a winner but **before** the Auditor commits.

### Sub-Agents

| Sub-Agent | Input | Output | Stakes |
|-----------|-------|--------|--------|
| Command Risk | Shell command string + context | LOW / MEDIUM / HIGH | Reputation staking: blocking safe ops costs reputation; correctly identifying dangerous ops earns it. |
| File Risk | File path + operation type + git state | LOW / MEDIUM / HIGH | Reputation staking: blocking legitimate edits costs; protecting system files earns. |
| Error Analyzer | Failed command output | AUTO_FIXABLE / ESCALATE / RETRY_LIMIT | Drives the auto-fix loop vs human escalation. |

### Consolidation Rules

- If any sub-agent reports HIGH or ESCALATE, the consolidated verdict must reflect that.
- **Fail closed**: Inconclusive analysis = HIGH risk. A filter that fails open produces
  false confidence.

### Two-Strike Circuit Breaker

1. **First HIGH** → contextual feedback returned to Sage so it can propose a safer
   alternative.
2. **Second HIGH** in the same investigation → `AGENT_CONFLICT_DETECTED` → forces human
   intervention.
3. Successful execution **resets** the strike counter.

---

## Reputation System

After every Tribunal invocation, the reputation service updates agent scores based on the
Auditor's verdict and the ranked vote results.

### Scoring

- **Exponential Moving Average (EMA)**: prioritizes recent behavior over historical.
- **Nemesis reputation**: Slashed when it raises a false flag or abstains on a real flaw.
  Earned when it sneaks a flaw past the Auditor (confirming the system has blind spots).
- **Tribunal members**: Earned for consensus with the winning candidate; lost for dissent
  that the Auditor overturns.

### Merkle Commitments

The Auditor binds every verdict to a snapshot of the current reputation scoreboard by
writing a signed Merkle commitment. These commitments are chained via `prev_root`
HMAC-SHA256 signatures, providing a verifiable, tamper-evident history of agent performance.

Reputation-commitment failure is **fatal**; the verdict cannot proceed.

---

## Context Delivery & Data Sovereignty

### Payload Discipline

The system does not use the LLM as a bulk transport channel:

- **Prompt assembly**: Static sections are prefix-cache friendly. Dynamic context is
  appended last and scoped to the current turn.
- **Conversation context**: Reconstructed from persisted messages. Scrubbed before provider
  delivery when Sentinel mode is enabled.
- **Output budget**: `max_tokens` and provider-specific output limits bound visible model
  output.
- **Operator payloads**: The Operator rejects command and pubsub payloads above the protocol
  ceiling of 5 MB, enforced by `MaxPayloadSize` in `internal/services/pubsub/protocol_helpers.go`.
- **Operator output**: Scrubbed before publishing results. The scrubbing service in
  `internal/services/scrubbing/` classifies output by size and applies structured boundaries.
- **Filesystem access**: Typed operations, not unconstrained shell dumps. Scoped reads with
  line windows and `max_lines`. Large files are rejected, not streamed into the model.
- **Search access**: Recursive grep and listing return structured results with counts and
  truncation flags.

### Sentinel Mode (Data Sovereignty)

Sentinel mode is the privacy-preserving default for cloud-model operation:

- **Scrubbed categories**: API keys, tokens, passwords, private keys, OAuth secrets, bearer
  tokens, emails, credit cards, SSNs, phone numbers, connection strings.
- **Preserved categories**: IPs, hostnames, MAC addresses, file paths, URLs (without
  embedded credentials), UUIDs, AWS ARNs, account IDs. These are operational data needed
  for troubleshooting.
- **User-message scrubbing**: Applied before LLM delivery.
- **Operator output scrubbing**: Applied before result publication back to the ensemble.
- **Raw data separation**: Raw host evidence stays in the Operator Raw Vault. AI-facing
  history comes from the Scrubbed Vault or typed result payloads.

### What Each Agent Sees

Context is **stage-specific**; each agent receives only what it needs:

| Agent | Sees | Does NOT See |
|-------|------|-------------|
| Triage | Current message, prior history, attachment metadata, settings | Commands, operator state |
| Dash / Sage | Modular system prompt, scrubbed history, operator metadata, triage context, investigation context, learned context, tool declarations | Raw host vault, other agents' internal state |
| Tribunal members | Sage's intent, guidelines, operator context, command constraints | Each other's candidates, full chat history |
| Warden sub-agents | Candidate command + narrow risk context | Broad investigation reasoning |
| Auditor | Request, operator context, anonymized clusters, reputation state | Full chat history, candidate sources |
| Codex | Current memory state + latest 20 messages | Real-time tool results, operator state |

---

## Mapping to g8e Protocol

The agentic system maps to g8e's protocol surface as follows:

### Protocol Surface Usage

| Agentic Action | Protocol Mechanism | g8e Component |
|----------------|-------------------|---------------|
| Tool discovery | MCP `tools/list` | Gateway MCP handler |
| Tool execution (read-only) | MCP `tools/call` | Gateway → Operator Actuator |
| Command execution (mutating) | `GovernanceEnvelope` with `CommandRequested` payload | Gateway admission gauntlet |
| L2 consensus signing | Ed25519 signature over `transaction_hash` and decision | Tribunal key → Gateway L2 verifier |
| L3 authorization | WebAuthn passkey assertion or signed CLI proof | Gateway L3 approver |
| Result delivery | Pub/Sub result envelope | Operator → Gateway → Ensemble |
| Audit trail | Signed receipts + LFAA events | Operator Audit Vault |

### g8e's Built-in Tribunal vs. Agentic Tribunal

g8e ships a **protocol-level Tribunal** in `internal/services/tribunal/` that performs
deterministic L2 consensus voting with Ed25519 signatures and doctrine-based safety
evaluation. This is the cryptographic backbone; it signs and verifies but does not reason.

The **agentic Tribunal** documented in this guide is the reasoning layer above it: it
generates commands through LLM-based ensemble consensus, classifies risk, and produces the
signed envelopes that g8e's protocol-level Tribunal and Gateway validate.

### Five-Layer Governance Interlock

When a signed envelope reaches the Gateway, it passes through a five-layer interlock
sequence before any tool dispatch occurs. Each layer is independent and fail-closed.

- **L1 Doctrine**: Hard gates, code pattern matching, and MITRE-based threat analysis. The
  `L1Doctrine` validator in `internal/services/governance/l1_doctrine.go` checks protobuf
  field options for forbidden patterns and runs regex-based threat detectors against command
  strings, MCP arguments, A2A payloads, and file edit content.
- **L2 Consensus**: Multi-agent consensus signature verification using Ed25519. The
  `L4Warden` in `internal/services/governance/l4_warden.go` verifies each `L2Vote` signature
  against the transaction hash and decision, and enforces quorum policy from the configured
  `TribunalPolicy`.
- **L3 Notary**: Human-in-the-loop authorization via WebAuthn passkey assertion or signed CLI
  proof. Mutations require L3 proof; the Gateway suspends execution and returns an approval
  URL when L3 is missing.
- **L4 Warden**: Pre-dispatch verification combining stateless validation (hash integrity,
  payload decoding, L1 doctrine), stateful validation (expiry, nonce replay prevention via
  SQLite, state root binding), and posture-aware L2/L3 checks. The Warden returns a
  `VerifiedTransaction` only if all gates pass.
- **L5 Actuator**: Isolated tool dispatch and signed receipt production. The `L5Actuator` in
  `internal/services/governance/l5_actuator.go` mints a JIT capability scoped to the
  transaction, dispatches through the execution handler, signs an `ActionReceipt` with its
  Ed25519 key, and records it in the audit store. Receipt signing is fail-closed: if the
  initial receipt cannot be signed or logged, execution does not proceed.

### Agent Harness (Test Tool)

g8e also ships an **Agent Harness** in `internal/tools/agent_harness/`, a Go-based test
tool that exercises the protocol surface with simple `Persona` structs.
It tests MCP, A2A, governance envelopes, tribunal quorum/veto, and notary OOB flows
against a real Gateway and Operator. The Agent Harness is not a reasoning system; it
validates protocol mechanics. The agentic system documented here is what you build on top
of the protocol to add intelligence.

### Building Your Own

The **g8ee** reference app is the canonical, native implementation of everything below.
Read it alongside this guide when building your own ensemble. The steps are language- and
provider-agnostic; g8ee is one concrete realization of them.

To build a g8e-compliant agentic system in any language:

1. **Implement the persona model**: Define your agents with the fields documented above
   (id, role, model_tier, tools, identity, purpose, autonomy, output_contract).
2. **Assemble modular prompts**: Follow the 13-section assembly order with XML
   scaffolding. Keep static sections first for prefix caching.
3. **Implement the ReAct loop**: Provider turn, tool dispatch, iteration. Route gated
   tools through your Tribunal.
4. **Implement the Tribunal cascade**: 5-member generation, voting, round 2, warden,
   auditor, L1 re-validation, envelope wrap.
5. **Sign envelopes**: Use Ed25519 to sign the `transaction_hash` and decision with your L2
   Tribunal key. Register the public key as a trusted signer with the Gateway.
6. **Submit over mTLS**: Send the signed `GovernanceEnvelope` to the Gateway's admission
   endpoint. The Gateway and Operator independently re-verify everything.
7. **Handle results**: Receive pub/sub result envelopes, scrub output, feed back into the
   ReAct loop.
8. **Maintain memory**: Run a Codex-like background agent after each turn to extract
   durable preferences and scrubbed summaries.

The g8e protocol does not care what language your ensemble is written in, what LLM provider
you use, or how many agents you run. It cares that your envelopes are correctly signed,
bound to the current state root, and pass all L1/L2/L3 gates. Everything above that line is
yours to design.
