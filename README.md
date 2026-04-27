<div align="center">

# g8e

**governance architecture for trustless environments**

The AI reasons. You decide. The architecture enforces it.

Self-hosted · Air-gap capable · Zero cloud dependencies

[Architecture](docs/architecture/about.md) · [Security](docs/architecture/security.md) · [Quick Start](#quick-start) · [Contributing](#contributing)

</div>

---

## The Shape of the Problem

Give an AI an API key with write access to your infrastructure and control shifts to the prompt. System instructions get overridden. Context windows get poisoned. A confident model executes a destructive command and the only thing between you and a bad afternoon is hope.

g8e removes hope from the loop.

The reasoning agent investigates your systems and proposes a plan. Execution halts at a cryptographic boundary. Every state-changing action requires a FIDO2 approval, enforced at the binary and network layer — where prompt injection cannot reach.

---

## The Core Loop

```
  You ────── "Fix the disk pressure on the prod nodes"
   │
   ▼
  Triage ─── classifies complexity · intent · posture · confidence
   │
   ├── complex turn ──►  Sage    (deep reasoning, full tool loop)
   └── simple turn  ──►  Dash    (fast path, narrow tool loop)
                          │
                          ▼
                 investigates · reasons · articulates intent
                          │
                          ▼
  The Tribunal ── heterogeneous personas, parallel generation
   │
   ├── Pass 0 · Axiom     (Composer)       ──┐
   ├── Pass 1 · Concord   (Guardian)       ──┤
   ├── Pass 2 · Variance  (Exhaustive)     ──┼──► Uniform Per-Member Vote
   ├── Pass 3 · Pragma    (Conventional)   ──┤   (deterministic tie-break)
   └── Pass 4 · Nemesis   (Adversary)      ──┘          │
                                                        ▼
                                              Auditor ── anonymized clusters
                                                         ok / revised / swap
                                                        │
                                                        ▼
                                           ┌──── Your Approval ────┐
                                           │   "df -h /var/log"    │
                                           │   on: prod-node-3     │
                                           │   [Approve]  [Deny]   │
                                           └───────────┬───────────┘
                                                       │
                                                       ▼
                                              Operator executes locally
                                                       │
                                 ┌─────────────────────┴─────────────────────┐
                                 ▼                                           ▼
                         Encrypted Vault                              Sentinel scrub
                         raw · local-only                             28 patterns, MITRE-mapped
                                                                             │
                                                                             ▼
                                                                Sage / Dash reasons about
                                                                the sanitized result
                                                                ──► next step
```

---

## The Tribunal

The reasoning agent never writes commands. It articulates intent — natural language, optional guardrails — and the Tribunal translates.

Five personas run in parallel. Each produces one candidate command from the same intent.

| Persona | Archetype |
|---|---|
| **Axiom** | The Composer — statistical probability, resource efficiency |
| **Concord** | The Guardian — harm minimization, ethical integrity |
| **Variance** | The Exhaustive — edge case hunter, adversarial simulation |
| **Pragma** | The Conventional — idiomatic, least-surprise correctness |
| **Nemesis** | The Adversary — always present, always pushing against the other four |

Each member casts one vote. Identical candidates accumulate. A two-vote consensus threshold gates the winner. Ties resolve deterministically: shortest command, then non-Nemesis cluster, then alphabetical.

The Auditor receives anonymized cluster IDs and evaluates the winner against the stated intent. Three modes by consensus strength:

- **Unanimous** — accept or revise
- **Majority** — accept, revise, or swap to a dissenting cluster
- **Tied** — must swap or revise; passive accept is forbidden

The Auditor knows one of the five is adversarial. It does not know which.

The verified command reaches you. You see it, in full, with target host and justification. You approve. You deny. That is the only path forward.

---

## Cryptoeconomic Mechanism Design

g8e aligns multi-agent behavior through a Proof of Stake reputation economy. Agents do not just propose actions—they stake their reputation on them.

- **The Genesis Block** — A user's initial prompt generates the genesis block, anchored by a Merkle root.
- **Proof of Stake Economy** — Dash, Sage, the Tribunal, and the Auditor all operate within a unified reputation market.
- **Skin in the Game** — When an agent proposes a solution, they stake reputation proportional to their confidence. 
- **Verifiable Resolution** — When you approve a command execution, the Auditor awards reputation to the agents whose proposals succeeded.
- **Immutable Ledger** — The Auditor cryptographically signs each turn, appending it as a new block in the conversation's immutable ledger.
- **Cross-Chain Reputation** — The Auditor maintains visibility across all conversations ("cross-chain"), ensuring an agent's reputation persists and compounds across different investigations.

By forcing agents to stake reputation with real consequences, their personas are economically incentivized to propose the safest, most effective solutions for an environment that voters are sworn to protect.

---

## Eight Directives

Principles engineered to hold when the models are a thousand times more capable than they are today.

```
  I.  Human Authority is Absolute       Every write gated by FIDO2. No exceptions.
 II.  Trust is Earned, Never Inherited  Zero standing credentials. Per-action scope,
                                        automatic expiration.
III.  Safety is Structural              Enforced at the binary and network layer.
                                        Prompt injection cannot reach the boundary.
 IV.  Data Stays Where It Belongs       28 scrub patterns on egress and ingress.
                                        Raw output never crosses the host.
  V.  Presence is Ephemeral             4MB static binary. Outbound-only mTLS.
                                        Kill the process and it is gone.
 VI.  Accountability is Local           Encrypted SQLite vaults plus a git-backed
                                        file ledger. Cryptographic commit chain.
VII.  Infrastructure is Yours           docker compose on your hardware. No SaaS,
                                        no telemetry, no phone-home. Air-gap capable.
VIII. Intelligence is Replaceable       Anthropic, OpenAI, Gemini, Ollama. Swap at
                                        will. Governance persists.
```

---

## Architecture

Four containers and a binary. The binary wears three hats depending on how you invoke it.

| Component | Language | Responsibility |
|---|---|---|
| **g8es** | Go | The binary in `--listen` mode. SQLite document store, KV store, pub/sub broker, blob store, platform CA. Zero external dependencies. |
| **g8ee** | Python | AI engine. ReAct loop, multi-provider abstraction (Anthropic, OpenAI, Gemini, Ollama), Tribunal pipeline, Sentinel integration. |
| **g8ed** | Node.js | Dashboard and single external entry point. FIDO2 auth, SSE streaming, mTLS gateway, human approval UI. |
| **g8ep** | Multi | Test runner, build environment, cross-arch Operator compiler, fleet deployment. |
| **Operator** | Go | The ~4MB static binary. Executes locally, maintains the encrypted audit vault, speaks outbound-only mTLS WebSocket. Streams itself over SSH to hundreds of hosts in parallel. |

```
                          ┌───────────────────────────────────────┐
                          │         Control Plane (Your Host)     │
                          │                                       │
    ┌─────────┐           │  ┌───────┐  ┌───────┐  ┌───────┐      │
    │ Browser │◄── HTTPS ─┼─►│ g8ed  │◄─┤ g8ee  │◄─┤ g8es  │      │
    │ (FIDO2) │   + SSE   │  │  :443 │  │  AI   │  │SQLite │      │
    └─────────┘           │  └───┬───┘  └───────┘  │KV/Pub │      │
                          │      │                 │  Sub  │      │
                          │      │ mTLS WebSocket  └───────┘      │
                          └──────┼────────────────────────────────┘
                                 │
                    ┌────────────┼────────────┐
                    │            │            │
               ┌────▼────┐  ┌────▼────┐  ┌────▼────┐
               │Operator │  │Operator │  │Operator │
               │ 4MB Go  │  │ 4MB Go  │  │ 4MB Go  │
               │ binary  │  │ binary  │  │ binary  │
               ├─────────┤  ├─────────┤  ├─────────┤
               │Encrypted│  │Encrypted│  │Encrypted│
               │  Audit  │  │  Audit  │  │  Audit  │
               │ Ledger  │  │ Ledger  │  │ Ledger  │
               └─────────┘  └─────────┘  └─────────┘
                 Host A      Host B      Host C
```

---

## Security at a Glance

- **Authentication** — FIDO2 / WebAuthn passkeys only. Passwords are unsupported by design.
- **Transport** — TLS 1.3 throughout. Platform-generated ECDSA P-384 CA. Per-operator mTLS client certs issued at claim time.
- **Sentinel** — 46 MITRE ATT&CK-mapped threat detectors pre-execution on the Operator. 28 scrubbing patterns applied twice (egress on the host, ingress on the engine) before any data reaches a model provider.
- **Sessions** — Encrypted cookies, idle and absolute timeouts, IP tracking, timestamp + nonce replay protection.
- **Operator Binding** — System fingerprint locked at first auth. A stolen API key is useless from a different machine.
- **Compliance Alignment** — NSA Zero Trust Guidelines (exceeds requirements in 6 of 7 pillars), HIPAA-ready architecture, FedRAMP-aligned controls.

Full threat model and control catalogue: [docs/architecture/security.md](docs/architecture/security.md).

---

## Quick Start

**Prerequisites:** Docker 24+ and Docker Compose v2.

```bash
git clone https://github.com/g8e-ai/g8e.git && cd g8e
./g8e platform build
```

Trust the platform CA on your workstation:

```bash
# macOS / Linux
curl -fsSL http://<host>/trust | sudo sh

# Windows (elevated PowerShell)
irm http://<host>/trust | iex
```

Open `https://<host>` and register your FIDO2 passkey.

Deploy an Operator to a remote host:

```bash
# Generate a device link in the dashboard, then on the target host:
curl -fsSL http://<host>/g8e | sh -s -- <device-link-token>
```

One command. It pulls the CA, fetches the binary, starts the Operator. No root, no package manager, no dependencies. The binary self-deletes when the session ends.

---

## CLI

```bash
./g8e platform build       # First-time build and start
./g8e platform start       # Start without rebuilding
./g8e platform stop        # Stop (data preserved)
./g8e platform wipe        # Wipe app data, restart fresh

./g8e operator build       # Compile Operator for all architectures
./g8e test <component>     # Run component tests (g8ee, g8ed, g8eo)
```

---

## Status

**Alpha.** A research project with a paranoia-first security posture. No external audit yet. Read the [security architecture](docs/architecture/security.md) and judge the threat model for yourself before any production use.

A significant portion of this codebase was written with AI assistance. If you have been around long enough to know what that means, you already know there are bugs, hallucinated branches, and abstractions a human would have written differently. We built a platform to govern AI agents because we lived the danger of unconstrained ones — while building this platform with those same agents.

---

## Contributing

The architecture is designed to support capabilities that do not exist yet. A good PR that improves any part of the platform gets merged.

What we value:

- Bug fixes and real-world edge cases
- Security hardening and threat model improvements
- New Operator capabilities and tool implementations
- LLM provider integrations and model-specific optimizations
- Documentation, testing, and developer experience
- Novel applications of the governance architecture

If you see something broken, fix it. If you see something missing, build it. If you have an idea nobody has built yet, open an issue.

See [CONTRIBUTING.md](CONTRIBUTING.md) for environment setup.

---

## Documentation

| Document | Description |
|---|---|
| [Architecture Overview](docs/architecture/about.md) | Origins, governance philosophy, core principles |
| [Security Architecture](docs/architecture/security.md) | Authentication, Sentinel, LFAA, threat model |
| [AI Control Plane](docs/architecture/ai_control_plane.md) | ReAct loop, Tribunal, prompts, tools, providers |
| [Operator Binary](docs/architecture/operator.md) | Lifecycle, modes, deployment, on-host storage |
| [Developer Guide](docs/developer.md) | Setup, code quality rules, project structure |
| [Testing Guide](docs/testing.md) | Test infrastructure, component guidelines, CI |
| [Glossary](docs/glossary.md) | Platform terminology |

---

## License

[Apache License, Version 2.0](LICENSE).

---

<div align="center">

*g8e is developed by [Lateralus Labs, LLC](https://lateraluslabs.com), a Certified Veteran Owned Small Business (VOSB).*

</div>
