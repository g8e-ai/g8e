---
title: AI Agents and the g8e Governance Boundary
parent: Architecture
---

# AI Agents and the g8e Governance Boundary

Last Updated: 2026-08-16
Version: v1.7.6

## Overview

g8e is a zero-trust execution platform that sits between AI agents, human operators, and target hosts. An AI agent never mutates a host directly. Instead, the agent formulates intent, and the g8e platform translates that intent into a typed, signed, verifiable `GovernanceEnvelope` that passes through five fail-closed governance layers before execution.

The platform treats every AI client as an untrusted principal. The agent's role is to describe what should happen and on which hosts. The gateway validates that description against doctrine, consensus, and human authorization. The governed operator on each host re-verifies every proof locally before it executes anything. This design keeps agents honest and hosts sovereign.

---

## Architecture at a Glance

Two components define the security boundary:

- **Governance Gateway (Policy Decision Point / PDP)**: The central coordinator that admits transactions, manages PKI, enforces L1 through L3 governance, and brokers pub/sub channels to operators. The gateway runs L4 Warden and L5 Actuator in-process for operations targeting the gateway host itself.
- **Governed Operator (Policy Execution Point / PEP)**: The same static binary run in operator mode on target hosts. It requires no installation and opens no inbound ports. It initiates an outbound-only mTLS tunnel to the gateway, pulls work from a unique pub/sub channel, re-verifies every proof locally, and is the only component authorized to mutate the host.

AI clients connect to the gateway over mTLS JSON. The gateway never reaches into operators; operators pull work when it is published to their channel. See [Gateway Architecture](./gateway.md) and [Operator Architecture](./operator.md) for the full service stacks.

---

## The AI Client Surface

g8e exposes two standard protocols for AI clients.

- **MCP (Model Context Protocol)**: A unified JSON-RPC endpoint that lets standard MCP clients such as Claude Code, Codex, Goose, or Gemini CLI discover tools, call them, and receive typed results. The gateway translates each MCP `tools/call` into a canonical `GovernanceEnvelope` and routes it through the governance pipeline.
- **A2A (Agent-to-Agent)**: A JSON-RPC endpoint for direct A2A skill invocations. The gateway wraps the skill request in an envelope and either executes it through a governed operator or forwards it to a configured downstream A2A server, depending on posture and authorization.

The gateway also publishes signed execution results and out-of-band approval events over pub/sub. AI clients consume these results through MCP/A2A responses, SSE streams, or direct pub/sub channels. See [Network Architecture](./network.md) for the mTLS and port topology and [SSE Streaming](./sse.md) for the event surface.

---

## Native Tool Playbook

The governed operator ships with native, memory-safe tools that agents invoke through the MCP surface. These tools execute inside the operator's L5 boundary and return structured JSON. Examples include filesystem reads, shell command execution, database triage, log filtering, process inspection, network probes, cloud metadata lookup, Git state, and Kubernetes inspection.

Each native tool accepts a typed request, performs read-only or governed-mutation operations, and returns a scrubbed result. The operator runs L1 doctrine analysis on the tool call, verifies the envelope, and only then executes. See [Operator Architecture](./operator.md) for the complete native tool playbook.

---

## The Five-Layer Governance Pipeline

Every agent-originated action passes through the same five fail-closed layers. The gateway owns L1-L3 as policy decisions; the operator owns L4-L5 as execution gates.

### L1 Doctrine

L1 is the technical hard gate. It matches payloads against forbidden patterns and MITRE ATT&CK heuristics to detect threats such as reverse shells, privilege escalation, and destructive disk operations. Doctrine is enforced in every posture. The operator re-runs L1 validation locally before execution.

### L2 Consensus

L2 is multi-signature consensus over the transaction hash. The consensus service is an enrolled body of members, each with a distinct Ed25519 private key. The reference implementation shipped with g8e evaluates the transaction deterministically against the L1 Doctrine and signs an affirmative or negative vote over `<transaction_hash>|<decision>`. The L4 Warden verifies the votes against the configured policy and trusted signer store. A quorum of distinct, valid affirmative signatures is required under the `consensus` and `notary` postures.

The protocol is designed to support heterogeneous consensus members, but the in-platform reference implementation uses deterministic L1 doctrine evaluation. Alternative consensus implementations can be enrolled as external producers. See [Consensus](./consensus.md) for enrollment, deliberation, and member key management.

### L3 Notary

L3 enforces human-in-the-loop authorization. In gateway mode, the platform requires a WebAuthn/FIDO2 passkey assertion over the transaction hash. CLI callers additionally undergo mTLS session verification. In outbound operator mode, L3 is satisfied by a suspended-transaction approval and Ed25519 signature over the transaction hash. Mutations are blocked until a valid L3 proof is presented; read-only actions do not require L3. See [Authentication & Authorization](./auth.md) for the notary modes.

### L4 Warden

The L4 Warden runs on the operator as the final pre-dispatch gate. It recomputes and compares the transaction hash, reserves the nonce, checks expiry, validates the state Merkle root, and verifies L2 and L3 proofs. Any mismatch or missing proof fails closed.

### L5 Actuator

The L5 Actuator is the singular execution boundary. It signs an `EXECUTING` receipt, rehydrates scrubbed sensitive data at the execution site using local vault keys, mints a just-in-time capability bound to the transaction hash, dispatches the action, dissolves the capability, and signs a final `COMPLETED` or `FAILED` receipt. See [Operator Architecture](./operator.md) for the execution boundary and [Encryption](./encryption.md) for PII scrubbing and rehydration.

---

## From Intent to Execution

The practical flow for an AI client is:

1. The AI client submits an intent through the MCP or A2A endpoint.
2. The gateway translates the intent into a canonical `GovernanceEnvelope` carrying the typed payload, identity, nonce, expiry, and state root.
3. Under `consensus` or `notary` posture, the gateway sends the envelope to the enrolled consensus for L2 votes.
4. If L3 is required and missing, the gateway suspends the transaction and sends an approval challenge to the human.
5. After L1-L3 pass, the L4 Warden admits the envelope and publishes it to the unique pub/sub channel for the bound operator.
6. The bound operator pulls the envelope, re-verifies L1-L4, and the L5 Actuator executes the action.
7. The operator writes the signed receipt to the local audit vault and publishes it back to the gateway.
8. The gateway returns the receipt to the AI client through the original MCP/A2A response or SSE channel.

Only the operator bound to the envelope receives the work. The gateway binds the envelope to the authenticated operator session and publishes to that operator's unique command channel. No broadcast occurs.

---

## Doctrine: Rules the AI Must Respect

Doctrine files are JSON-defined pattern sets that specify what the platform will and will not execute. The bundled doctrine sets include blacklist, whitelist, OWASP CRS, Gitleaks, and MCP vector patterns. Doctrine is enforced regardless of agent behavior. The agent can see the doctrine as a courtesy to shape its requests, but the L1 gate rejects forbidden actions regardless of whether the agent complied. See [Governance](./governance.md) for the doctrine sources and loading pipeline.

---

## Local-First Audit and the SSE Event Bridge

Every action, command, output, error, and receipt is written to a tamper-evident, hash-chained ledger on the operator. This ledger is the source of truth for what happened on a given host. The gateway sees transaction hashes and state roots, not raw operational data.

Agentic ensembles can also push telemetry and user-facing events to the gateway through the SSE push endpoint. These events are stored, indexed, and streamed to authorized CLI sessions and browser console sessions. This lets an external ensemble surface progress, questions, and results to the human without exposing raw host data. See [SSE Streaming](./sse.md) for the push, poll, and stream semantics.

---

## MCP Stdio Credential Resolution

When `g8e mcp stdio` is invoked directly from an IDE MCP config, credentials resolve in the following order:

1. **CLI flags** (`--client-cert`/`--client-key`, `--app-cert`/`--app-key`, `--ca-bundle`, `--gateway-url`): universally supported in IDE `args` arrays.
2. **`G8E_*` environment variables** (`G8E_CLIENT_CERT`, `G8E_CLIENT_KEY`, `G8E_CA_BUNDLE`, `G8E_GATEWAY_URL`, `G8E_APP_CERT`, `G8E_APP_KEY`): injected by `g8e mcp agent run` into the agent subprocess.
3. **Enrolled CLI credentials on disk**: loaded from the local client credential store, bootstrapping enrollment if needed.

Cert and key are resolved as pairs per tier. Supplying only one half of a pair (for example, `--app-cert` without `--app-key`) fails closed instead of silently mixing tiers. The `--gateway-url` flag validates scheme (`https` only) and host to prevent plaintext traffic. See [Network Architecture](./network.md) for the full flag table.

---

## Security Boundaries Summary

| Boundary | What It Enforces |
| --- | --- |
| **AI client → Gateway** | The client speaks canonical MCP/A2A and provides intent. The gateway constructs and governs the envelope. |
| **L1 Doctrine** | Forbidden patterns and MITRE threats are rejected before consensus or execution. |
| **L2 Consensus** | Multi-signature Ed25519 votes are verified against the transaction hash and trusted signer store. |
| **L3 Notary** | Human presence or outbound approval is required for mutations. |
| **L4 Warden** | Hash, nonce, expiry, state root, and signatures are re-verified on the operator before execution. |
| **L5 Actuator** | Execution is wrapped in signed receipts, PII rehydration, and just-in-time capabilities. |
| **Operator → Host** | Only the operator mutates the host, and every action is written to the local ledger. |

---

## Key Design Principles

- **Do not trust the AI client.** The agent provides intent; the platform verifies and executes. The client has no privileged channel.
- **Do not trust the consensus layer.** Votes are verified against trusted public keys and the transaction hash. A missing or invalid signature fails closed.
- **Do not trust the gateway.** The operator re-derives every proof locally before execution.
- **Multi-signature consensus.** L2 requires K-of-N Ed25519 affirmative votes from distinct members. The reference implementation signs deterministic L1-doctrine evaluations.
- **Doctrine is enforced, not suggested.** Agents can be informed of doctrine, but the L1 gate rejects forbidden actions regardless of compliance.
- **Scope is explicit.** The envelope is bound to the authenticated operator session, and only that operator's command channel receives the dispatched work.
- **Sovereign hosts.** Every operator is authoritative for its own audit ledger and state root. The gateway never reaches into operators.

---

## Related Documentation

- [Gateway Architecture](./gateway.md): Gateway role, MCP/A2A endpoints, pub/sub brokering, and the 5-layer pipeline.
- [Operator Architecture](./operator.md): Operator execution boundary, native tools, and local audit.
- [Governance](./governance.md): Five-layer verification pipeline and posture configurations.
- [Consensus](./consensus.md): L2 consensus implementation, consensus enrollment, and deliberation flow.
- [Authentication & Authorization](./auth.md): Identity, mTLS, WebAuthn notary, and session binding.
- [Storage Architecture](./storage.md): LFAA ledger, audit store, and hash-chained commitment history.
- [Encryption](./encryption.md): Vault architecture, PII scrubbing, and rehydration at the execution boundary.
- [Network Architecture](./network.md): Outbound-only mTLS, pub/sub channels, and PKI.
- [SSE Streaming](./sse.md): SSE push, poll, and stream endpoints for agentic ensembles.
- [Build Apps](../guides/build_apps.md): Guide for building g8e-compatible applications, including maximal agentic ensembles.
