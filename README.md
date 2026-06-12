<div align="left">

# g8e

**Verify, then execute.**

g8e is a Reference Monitor for agentic infrastructure — a fail-closed admission boundary and a sovereign context plane in a single static Go binary. It governs every state-changing action on a host, and the tamper-evident record of those actions *is* the context your agents reason from. One proof chain. Both planes.

**2 Roles. 1 Binary. 0 Trust.**


[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE) 
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8.svg)](https://go.dev) 
[![CI](https://github.com/g8e-ai/g8e/actions/workflows/build-and-test.yml/badge.svg)](https://github.com/g8e-ai/g8e/actions/workflows/build-and-test.yml) 
[![Go Report Card](https://goreportcard.com/badge/github.com/g8e-ai/g8e)](https://goreportcard.com/report/github.com/g8e-ai/g8e) 
[![Latest Release](https://img.shields.io/github/v/release/g8e-ai/g8e)](https://github.com/g8e-ai/g8e/releases) 
[![Status](https://img.shields.io/badge/status-active%20development-orange.svg)](#status-v1010--core-platform) 
[![Compliance](https://img.shields.io/badge/compliance-SOC2%20ISO%20GDPR-006400.svg)](docs/reference/compliance-alignment.md) 
[![Secure MCP](https://img.shields.io/badge/Secure-MCP-5D3FD3.svg)](docs/protocols/mcp/mcp.md) 


[Quick start](#quick-start) · [The two planes](#the-two-planes) · [Mental model](#the-mental-model) · [Admission pipeline](#the-admission-pipeline) · [Docs](#documentation)

</div>

---

## The problem

AI agents now hold write access to terminals, cloud APIs, CI/CD, source control, and databases — wired in through MCP, A2A, and function calls. Those protocols establish **capability**: they prove an agent *can* act. They say nothing about **authority**: whether a given action, right now, on this host, is safe to execute.

The industry made a second mistake at the same time. To give agents memory, it moved context *into the provider* — managed threads, session stores, provider-side caches. Your data became someone else's state.

g8e refuses both defaults.

```
The mandatory invariant:
A state-changing action reaches the host only as a typed, signed, state-bound
transaction; the host verifies that transaction before it executes.
The record of that execution never leaves the host — and becomes the
verifiable context for the next action.
```

---

## What g8e is

A Reference Monitor in the original Anderson Report sense: tamper-evident, always invoked, small enough to verify. Implemented as one pure-Go static binary (`CGO_ENABLED=0`, zero standing dependencies) running in two roles:

- **Governance Gateway (`g8e gw`)** — the Policy Decision Point. Admits signed `GovernanceEnvelope` transactions, owns the platform PKI (mTLS, SPIFFE workload identities), enforces freshness and replay defense, and relays envelopes to operators. It holds **no privileged bypass** and no execution authority. It cannot reach into your hosts; it cannot even connect to them.

- **Governed Operator (`g8e op`)** — the Policy Execution Point and the center of gravity. It dials **outbound-only** over mTLS to the Gateway — it listens on nothing, which means it governs hosts behind firewalls, in private subnets, and in air-gapped enclaves. It re-derives every proof locally against its own state, trusts nothing upstream including the Gateway, and is the only component permitted to mutate the host.

g8e is actor-agnostic. It governs the **action**, not the actor. An AI agent, a human at a CLI, a CI/CD pipeline, and a cron job all submit through the same admission API with zero privilege difference. Any conformant `GovernanceEnvelope` producer is a Principal.

---

## The two planes

Every governance product on the market is a control plane bolted onto someone else's data plane. g8e collapses them into one object, and that is the category difference.

**The action plane.** Every mutation clears a five-layer admission pipeline at the host before it runs. Anything stale, unsigned, unauthorized, or off-policy is dropped at the boundary and recorded. The default is closed.

**The context plane.** Every admitted action writes a signed `ActionReceipt` to a host-local, git-backed, hash-chained ledger — the Local-First Audit Architecture (LFAA) — *before* the side effect occurs. That ledger is not a compliance artifact that happens to be readable. It is the only memory tier in modern agent architecture with integrity guarantees: a cryptographically provable chain of intent, interpretation, and outcome that an agent reads to form its next action. The agent doesn't *retrieve* context from a store it must trust — it *derives* context from a chain it can independently verify, and compares it against live host state through governed tools.

The same proof chain that gates execution is the substrate agents remember through. Acting and remembering are one operation on one object.

**The consequence for cloud AI:** the frontier model becomes a stateless reasoning utility. Canonical state lives in the LFAA on your host. Context is composed locally. Only tokenized, scrubbed intent material ever crosses the sovereignty boundary; rehydration happens at execution, where the data already lives. The cloud sees commitments — transaction hashes, state roots, tokenized payloads — never custody. If you know Lightning, you know this geometry: minimize the expensive shared layer to commitments, keep real state in local channels, settle cryptographically. Except here the base layer isn't merely trustless — it's *untrusted*. You get frontier reasoning with on-prem data physics.

---

## The mental model

g8e follows standard MCP topology with integrated governance:

| Reference | g8e Role | Implementation |
| --- | --- | --- |
| **MCP server** | **Governed Operator** | Tool-calling facade where every execution clears the host-local admission pipeline. No inbound ports. Runs on remote, private, or air-gapped hosts. |
| **MCP gateway** | **Governance Gateway** | Admits signed, state-bound envelopes; dispatches to operators; centralized audit authority with zero raw-data exposure. |

The Gateway proposes. The Operator disposes. Verification happens on the host that lives with the consequences.

---

## The admission pipeline

Five layers, two independent failure domains — semantic grading and mathematical verification, interleaved with no shared failure modes:

1. **L1 Doctrine** — deterministic static analysis. Forbidden patterns, MITRE ATT&CK indicators, CUI/PHI doctrine rules. Always active, for every action, no exceptions.
2. **L2 Consensus** — heterogeneous multi-model k-of-n Ed25519 signing over the canonical SHA-256 transaction hash. No single model's output executes.
3. **L3 Notary** — hardware-bound human authorization. A WebAuthn/FIDO2 passkey assertion computed over the transaction hash — bound to one action, forever. Not a session. Not a scope.
4. **L4 Warden** — the fail-closed verification authority. Re-derives every proof against local state: signatures, freshness, nonce, expiry, and the envelope's expected state Merkle root against the Operator's own ledger root.
5. **L5 Actuator** — the single dispatch path. Handler invocation with data-sovereignty enforcement: tokenized payloads are rehydrated here, at the boundary, on the host.

Compromising any single layer is insufficient to cause an unauthorized mutation. An attacker must simultaneously defeat orthogonal, independently audited proofs.

---

## Data sovereignty, enforced — not promised

- Raw data never leaves the host. Tokenization and scrubbing happen before intent material crosses the boundary; rehydration happens at L5.
- The transaction hash is computed over the **tokenized** payload, and the token keymap's integrity is bound into the state Merkle root — substitution at rehydration breaks the transaction.
- Transport credentials (OAuth tokens, bearer tokens) become **evidence in the envelope**, never bypass mechanisms.
- The audit record is written before the side effect. There is no window where something ran but nothing was recorded.

---

## Quick start

Download the binary for your platform from [g8e.ai](https://g8e.ai) (darwin/linux/windows, amd64/arm64), or build from source:

```bash
# Start the Gateway (choose your posture)
./g8e gw start

# Enroll an Operator on any host that can dial out
./g8e operator deploy --endpoint <gateway-external-address>

# Run agentic tools
./g8e gw status

# Query the audit vault
./g8e data query --collection audit_vault
```

Three postures, one enforcement model:

| Posture | L1 Doctrine | L2 Consensus | L3 Notary |
| --- | --- | --- | --- |
| `doctrine` | enforced | audited | audited |
| `consensus` | enforced | enforced | audited |
| `notary` | enforced | enforced | enforced |

L4 Warden and L5 Actuator are always active. There is no posture where the boundary is open.

---

## What g8e is not

- **Not an agent framework.** LangChain, CrewAI, and every ensemble are Principals — intent producers submitting through the same admission API as everyone else, with zero privilege advantage.
- **Not a cloud service.** Self-hosted, local-first, runs air-gapped. No phone-home, no SaaS dependency, no telemetry leaving your perimeter.
- **Not a prompt filter.** g8e governs at the execution boundary with cryptographic proofs, not at the prompt with heuristics.
- **Not paywalled.** Apache 2.0, forever. Safe AI runtime governance must never be gated by a paywall.

---

## Compliance posture

Designed for environments where "trust us" is not an acceptable answer: NIST 800-207 zero trust architecture, NIST AI RMF, CMMC/CUI handling, FedRAMP-aligned deployment paths, ISO 42001, SOC 2. The LFAA ledger produces the evidence trail these frameworks ask for as a side effect of normal operation.

---

## Status

**v1.0.0 — shipped.** Core protocol, Gateway and Operator roles, five-layer pipeline, PKI/mTLS identity, WebAuthn notary, MCP/A2A protocol translation, LFAA audit vault, 13 native tools, multi-platform binaries.

In active development: transparent interception (`g8e run <cmd>` per-process proxy and TPROXY whole-host mode), gateway federation mesh, multi-operator Byzantine fault tolerance.

---

## Documentation

- [Getting Started](docs/guides/getting_started.md)
- [Position Paper](docs/core/position_paper.md)
- [Operator Architecture](docs/architecture/operator.md)
- [Gateway Architecture](docs/architecture/gateway.md)
- [Security Model](docs/architecture/auth.md)
- [MCP Integration](docs/protocols/mcp/mcp.md) · [A2A Integration](docs/protocols/a2a/a2a.md)
- [CLI Reference](docs/guides/cli.md)
- [Glossary](docs/reference/glossary.md)

---

Apache 2.0 · Built by [Lateralus Labs](https://g8e.ai) · Veteran-owned

*This ran because it proved it should — not because nothing stopped it.*
