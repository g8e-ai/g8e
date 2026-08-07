---
title: AI Agents in a g8e-Compatible Agentic Ensemble
parent: Architecture
---

# AI Agents in a g8e-Compatible Agentic Ensemble

Last Updated: 2026-08-07
Version: v1.7.0

## Overview

g8e is a zero-trust execution platform that sits between the human, AI agents, and real-world systems. A g8e-compatible agentic ensemble is a collection of AI agents that reason over a shared, hash-chained event ledger and propose actions through a strictly enforced intent-to-execution pipeline. The ensemble never executes directly; it interprets intent, and the g8e platform translates that intent into governed, signed, verifiable transactions.

---

## Architecture at a Glance

Two components define the security boundary:

- **Governance Gateway (Policy Decision Point / PDP)**: The central coordinator that admits transactions, manages PKI, enforces the 5-layer governance pipeline, and brokers pub/sub channels to operators. The gateway is where doctrine is loaded, where the Consensus deliberates, and where the L4 Warden validates every proof before dispatch. The Gateway is an Operator with additional gateway services; its in-process Operator runs L1-L5 locally for operations on the gateway host.
- **Governed Operator (Policy Execution Point / PEP)**: A single static binary deployed on target hosts. It requires no installation, opens no inbound ports, and initiates an outbound-only mTLS tunnel to the gateway. The operator pulls work from a unique pub/sub channel, re-verifies every proof locally against its own state, and is the only component authorized to mutate the host. Everything the operator does, every command, file edit, error, output, is recorded in a local, hash-chained ledger. Each remote Governed Operator runs L1-L5 locally for operations on its own host.

The ensemble communicates with the gateway over mTLS JSON. The gateway never reaches into operators; operators pull work when it is published to their channel.

---

## The Event Stream as a Hash-Chained Ledger

Every action in the g8e platform, every message, error, output, command execution, file mutation, heartbeat, and receipt, is written to a tamper-evident, hash-chained ledger on the operator. This is the Local-First Audit Architecture (LFAA). Each entry is linked to the prior entry's hash, forming a chain that is independently verifiable against the host's current state root.

The ledger is the **single source of truth** for what has happened on a given host. It is not a log file; it is a cryptographic commitment history. The gateway sees transaction hashes and state roots, never raw data.

### Context Data Path for Agents

Agents in the ensemble do not maintain their own state. Instead, they reason over **narrowly scoped sections of the hash-chained ledger** that are injected into their system prompts at runtime. This is the full context data path:

1. **An action occurs on the operator**: a command runs, a file is edited, an error is emitted.
2. **The operator writes the result to the local hash-chained ledger** and publishes a signed receipt back to the gateway via the results pub/sub channel.
3. **The gateway records the receipt** and makes it available for context retrieval.
4. **The ensemble's context assembler** selects a narrowly scoped window of relevant ledger entries, filtered by session, host, role, and recency, and injects them into the **dynamic system prompt** for the next agent invocation.
5. **The next agent receives only the context appropriate to its role**; it does not see the full ledger, only the slice that is relevant to its task and permissions.

This design keeps agents **stateless**: they carry no conversation history of their own. All state lives in the ledger. All context is derived from the ledger. An agent can be replaced, restarted, or scaled horizontally without loss of continuity, because the next agent picks up the same ledger slice. The exception is Consensus members, which maintain persistent Ed25519 signing keys on disk for vote signing (see [Consensus](./consensus.md)); however, these keys are cryptographic identities, not conversation state.

### Dynamic System Prompts

System prompts are not static. They are assembled at invocation time from:

- **Doctrine**: The applicable rules, forbidden patterns, and allowed commands for the target environment. Doctrine is enforced regardless of agent cooperation; providing it to agents is a courtesy so their suggestions are in-line with what the platform will accept.
- **Ledger context window**: A narrowly scoped section of the event history, filtered by the agent's role in the ensemble and the current task scope.
- **Role-specific instructions**: The agent's persona, responsibilities, and constraints within the ensemble.

Different agents see different slices of the same ledger. A triage agent sees recent errors and alerts. A response agent sees the triage agent's findings. A consensus juror sees the proposed action and its doctrine context. No agent sees everything.

---

## Ensemble Topology

A g8e-compatible ensemble is structured around an **intent-to-execution pipeline** that separates reasoning from action. The recommended ensemble topology includes:

### 1. Triage Agent

The entry point. The triage agent receives incoming events, alerts, errors, user requests, and classifies them. It determines whether the issue requires a lightweight response or a heavier intervention. The triage agent has read-only context: it sees recent ledger entries, system state projections, and doctrine, but it cannot propose mutations.

### 2. Lightweight Response Path

For simple, well-understood issues: restarting a known service, clearing a cache, adjusting a config value. The triage agent can route these directly to the intent-to-execution pipeline with a narrowly scoped intent. The pipeline still enforces all governance layers, but the consensus may be configured with a lower quorum for low-risk, well-known actions.

### 3. Heavier Path (Real Changes)

For issues that require actual system changes: patching code, modifying infrastructure, resolving complex incidents. These go through the full intent-to-execution pipeline described below. This is where the AI is helping solve actual problems, and where the separation of intent from execution is most critical.

---

## The Intent-to-Execution Pipeline

The core design principle: **the primary LLM only interprets intent; it does not execute.** The primary model provides instructions to the consensus layer. The consensus layer evaluates the safety of those instructions and produces signed votes.

### Step 1: Primary LLM Interprets Intent

The primary LLM receives the dynamic system prompt (doctrine + ledger context + role instructions) and the current task. It produces an **interpretation of intent**: what it believes should be done, on which hosts, and why. This is a description, not a command. The primary LLM also specifies the **hosts list**, the set of target systems where the action should be applied.

### Step 2: Consensus Layer (Consensus)

The primary LLM's interpretation of intent is passed to the consensus layer, the **Consensus**, which evaluates the safety of the proposed action and produces signed votes. The Consensus is a body of agents, each with its own Ed25519 signing key. Each juror independently:

1. Evaluates the proposed action against L1 Doctrine (forbidden patterns, MITRE threat detection).
2. Produces a signed vote: affirmative (safe) or negative (unsafe).

The protocol is designed to support [Condorcet's Jury Theorem](https://en.wikipedia.org/wiki/Condorcet%27s_jury_theorem): if each juror has a probability greater than 0.5 of making the correct judgment, and the jurors are independent, then the probability of a correct majority judgment approaches 1 as the number of jurors increases. The protocol allows each consensus member to use its own evaluation logic, enabling heterogeneous juries with independent reasoning. The reference implementation shipped with g8e uses a shared deterministic L1 Doctrine evaluation across all members with distinct signing keys; this provides multi-signature safety with cryptographic independence (each member signs with its own key) but not evaluation independence. Alternative consensus implementations can provide diverse evaluation strategies per member to achieve full Condorcet independence.

The number of jurors is configurable per consensus. Results vary depending on how much validating is needed for a specific task. A low-risk action may need a 1-of-1 consensus. A high-risk production change may require a 5-of-7 consensus. The quorum threshold is set in the consensus policy and enforced by the L4 Warden.

### Step 3: Consensus Failure → Reinterpret

**Do not trust the LLM.** If consensus is not reached (quorum is not met, or a majority of jurors vote negative), that signals a problem with the LLM's interpretation of intent. The primary LLM is asked to **reinterpret**: it receives feedback on why consensus failed and must produce a revised interpretation. The loop restarts from Step 1.

### Step 4: Consensus Reached → Hash Verification

**Do not trust the consensus layer.** After consensus is reached and the Consensus returns signed votes, the gateway verifies that the returned signatures **exactly match the hash-chained ledger**. The transaction hash in the envelope must match the recomputed hash. The state root must match the current ledger root. Every Ed25519 signature is independently verified against the trusted signer store. If any hash does not align, the system **fails closed**; the transaction is rejected.

### Step 5: Human Approval

If all hashes align, the system asks the **human** to sign the interpretation of intent and the proposed solution. This is the L3 Notary layer: a WebAuthn/FIDO2 passkey assertion computed over the transaction hash, bound to one action, one moment, and one host. It cannot be transplanted, replayed, or harvested.

- **If the user denies**: The LLM re-assesses intent and the loop restarts from Step 1.
- **If the user approves**: The signed envelope proceeds to dispatch.

### Step 6: Warden Validation and Dispatch

After human approval, the L4 Warden on the Gateway's in-process Operator performs final validation: all hashes are re-verified, nonce replay protection is enforced, expiry is checked, and the state Merkle root is confirmed. Only after the Warden validates does dispatch occur.

The request is then published to a **unique pub/sub channel for each operator**. Critically, **only the operators in scope**, those on the hosts list that the primary LLM specified and the Consensus validated, are included in the dispatch. No matter how many operators are currently enrolled in the platform, only the operators on the approved hosts list receive the work. This is a security boundary: the primary LLM passes the list of systems it wants to act on, and only those operators receive the dispatched work; no more.

### Step 7: Operator Pulls Work and Executes

The operator pulls the work from its unique pub/sub channel. Before any execution occurs, the **entire L1-L4 gauntlet is re-validated locally on the Governed Operator**; the operator does not trust the gateway. It re-derives every proof from scratch against its own local state.

Once L1-L4 pass, the **L5 Actuator** initiates execution:

1. **EXECUTING receipt**: An EXECUTING receipt is signed and persisted to the local audit vault before any execution begins. If signing or logging fails, the system fails closed and execution does not proceed.
2. **PII hydration**: Sensitive data (credentials, PII) that was scrubbed upstream is rehydrated at the execution site using local vault keys. The cloud never saw the raw values, only tokenized projections.
3. **JIT capability minting**: A scoped, single-action, self-dissolving capability token is minted, bound to the transaction hash. Zero standing privileges.
4. **Execution**: The handler dispatches the verified payload (shell command, file edit, etc.).
5. **Capability dissolution**: The JIT capability is dissolved immediately after execution.
6. **Final receipt**: A COMPLETED/FAILED receipt is signed and persisted to the local audit vault after execution.
7. **Ledger update**: The result, state root change, and signed receipt are written to the local hash-chained ledger.
8. **Result publish**: The signed receipt is published back to the gateway via the results pub/sub channel.

### Step 8: Ledger Feeds the Next Agent

Everything the agent does, every thought, action, message, error, output, is recorded in the local hash-chained ledger on the operator. A small section of that ledger is then injected into the system prompt for the next agent in the ensemble, filtered by role and relevance. This leaves agents stateless: they derive all context from the ledger, verify it against live host state through governed tools, and carry no conversation history of their own.

The loop continues: ledger → context assembly → dynamic system prompt → agent reasoning → intent → consensus → human approval → dispatch → execution → ledger.

---

## Doctrine: Rules the Agents Must Follow

All agents in the ensemble have access to **doctrines**, the JSON-defined rules that specify what can and should be run on the target systems. Doctrine files include:

- **Blacklist doctrine**: Forbidden commands and patterns (reverse shells, privilege escalation, data destruction, credential access, etc.).
- **Whitelist doctrine**: Explicitly permitted commands for operator execution.
- **OWASP CRS doctrine**: Web attack pattern detection rules.
- **Gitleaks doctrine**: Secret and credential leak detection patterns.
- **MCP vectors doctrine**: Threat patterns specific to MCP tool calls.

Doctrine is **enforced regardless** of agent cooperation; L1 Doctrine is a fail-closed gate in all postures. Providing doctrine to agents is a **courtesy**: it lets them shape their suggestions to be in-line with what the platform will accept, reducing round-trips and consensus failures. But even if an agent ignores doctrine and proposes a forbidden action, the L1 layer will reject it before execution.

See [Governance](./governance.md) for the doctrine sources and how they are loaded into the pipeline.

---

## Security Boundaries Summary

| Boundary | What It Enforces |
| --- | --- |
| **Primary LLM → Consensus** | LLM only interprets intent; Consensus votes on safety. LLM cannot execute. |
| **Consensus → Hash verification** | Consensus signatures must align to the hash-chained ledger. Fail closed on mismatch. |
| **Hash verification → Human** | Human signs intent + proposed solution. Can deny and restart the loop. |
| **Human approval → Warden** | Warden re-validates all hashes on the Gateway's in-process Operator before dispatch. |
| **Dispatch → Operator scope** | Only operators on the approved hosts list receive work. No broadcast. |
| **Operator → Execution** | L1-L4 re-validated locally. L5 signs EXECUTING receipt, hydrates PII, mints JIT capability, executes, dissolves capability, signs final receipt. |
| **Execution → Ledger** | Every action written to local hash-chained ledger. Ledger feeds next agent's context. |

---

## Key Design Principles

- **Do not trust the LLM.** The primary model only interprets intent. Consensus failure triggers reinterpretation.
- **Do not trust the consensus layer.** Returned signatures are verified against the hash-chained ledger. Mismatch means fail closed.
- **Do not trust the gateway.** The operator re-derives every proof locally before execution.
- **Agents are stateless.** All state lives in the hash-chained ledger. Context is derived from the ledger, injected into dynamic system prompts, and scoped by role. Consensus members are an exception: they maintain persistent Ed25519 signing keys for vote signing, but carry no conversation history.
- **Multi-signature consensus.** The Consensus produces K-of-N Ed25519 signed votes. The protocol is designed to support Condorcet's Jury Theorem with heterogeneous jurors; the reference implementation uses deterministic L1 Doctrine evaluation with distinct signing keys, providing cryptographic independence. Alternative implementations can provide per-member evaluation logic for full independence.
- **Doctrine is enforced, not suggested.** Providing doctrine to agents is a courtesy. L1 rejects forbidden actions regardless.
- **Scope is explicit.** The primary LLM specifies the hosts list. Only those operators receive work. No broadcast, no scope creep.

---

## MCP Stdio Credential Resolution

When `g8e mcp stdio` is invoked directly from an IDE MCP config (Windsurf, Cursor, VS Code), credentials resolve in the following order:

1. **CLI flags** (`--client-cert`/`--client-key`, `--app-cert`/`--app-key`, `--ca-bundle`, `--gateway-url`): universally supported in IDE `args` arrays
2. **`G8E_*` environment variables** (`G8E_CLIENT_CERT`, `G8E_CLIENT_KEY`, `G8E_CA_BUNDLE`, `G8E_GATEWAY_URL`, `G8E_APP_CERT`, `G8E_APP_KEY`): injected by `g8e mcp agent run` into the agent subprocess
3. **Enrolled CLI credentials on disk**: loaded from the local client credential store, bootstrapping enrollment if needed

Cert and key are resolved as **pairs per tier**: app flags → app env → client flags → client env → CLI disk. Supplying only one half of a pair (e.g. `--app-cert` without `--app-key`) fails closed instead of silently mixing tiers.

The `--gateway-url` flag validates scheme (`https` only) and host (non-empty) to prevent plaintext traffic bypassing mTLS identity binding.

These flags are command-scoped to `mcp stdio`, not root-persistent. See [Network Architecture](./network.md) for the full flag table.

---

## Related Documentation

- [Gateway Architecture](./gateway.md): Gateway role, pub/sub brokering, and the 5-layer pipeline.
- [Operator Architecture](./operator.md): Operator execution boundary, native tools, and local audit.
- [Governance](./governance.md): Five-layer verification pipeline and posture configurations.
- [Consensus](./consensus.md): L2 consensus implementation, consensus enrollment, and deliberation flow.
- [Storage Architecture](./storage.md): LFAA ledger, audit store, and hash-chained commitment history.
- [Encryption](./encryption.md): Vault architecture, PII scrubbing, and rehydration at the execution boundary.
- [Network Architecture](./network.md): Outbound-only mTLS, pub/sub channels, and PKI.
- [About g8e](../core/about.md): Platform overview and architectural differentiators.
- [Build Apps](../guides/build_apps.md): Detailed guide for building g8e-compatible applications, including the full agentic system design with persona catalog, prompt architecture, and consensus cascade.
