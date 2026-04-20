<div align="center">

# g8e

### governance architecture for trustless environments

Self-hosted. Air-gap capable. Zero cloud dependencies.<br/>
The AI reasons. You decide. The architecture enforces it.

[Architecture](docs/architecture/about.md) &#183; [Security](docs/architecture/security.md) &#183; [Quick Start](#quick-start) &#183; [Contributing](#contributing)

</div>

---

## The Problem

Give an AI an API key with write access to your infrastructure and you no longer control your infrastructure — the AI's prompt does. Prompt engineering is not a security boundary. System instructions can be overridden, context windows can be poisoned, and tool-calling agents will confidently execute destructive commands if nothing structurally prevents them.

The industry is racing to give AI agents more autonomy. We think the race should be to give them better governance.

g8e is an open-source platform that binds AI reasoning to real infrastructure through a zero-trust execution model. The AI investigates your systems, reasons about problems, and proposes actions. It cannot execute anything. Every state-changing operation requires explicit human approval, enforced cryptographically at the binary and network layers — not by a system prompt that hopes the model will comply.

---

## The Eight Directives

Every architectural decision in this platform is an expression of these principles. They are not aspirational. They are enforced in code shipping today. They are also designed to remain true when the models are a thousand times more capable than they are now.

---

**I. Human Authority is Absolute**

The AI proposes. You decide. This is not a policy — it is a cryptographic invariant. Execution and authorization are strictly separated. Every state-changing action requires explicit approval via FIDO2 WebAuthn. No API call, no prompt injection, no model behavior can bypass it. The human is the final, non-negotiable security layer.

**II. Trust is Earned, Never Inherited**

No long-lived credentials. No implicit trust from network position. Trust is mathematically bound to mTLS sessions, scoped to individual actions, and impossible for an agent to self-escalate. On AWS, the Operator launches with zero permissions and earns them one approved intent at a time, with automatic expiration.

**III. Safety is Structural, Not Verbal**

System prompts are suggestions. g8e enforces safety at the binary and network layers, where prompt injection cannot reach. Sentinel's 46 MITRE ATT&CK-mapped threat detectors block dangerous commands before any process is spawned. Privilege escalation is unconditionally forbidden. The AI cannot opt out of governance.

**IV. Data Stays Where It Belongs**

The remote host is the system of record. Raw command output never leaves the machine. The AI receives only what passes through Sentinel's 28 scrubbing patterns — credentials, PII, and secrets are replaced with safe placeholders before any data crosses a network boundary toward any model provider.

**V. Presence is Ephemeral**

The Operator is a ~4MB static binary. No dependencies. No installation. No root required. Outbound-only mTLS — it opens no inbound ports and works behind any NAT or firewall without configuration. Kill the process and it is gone. The only trace left behind is an encrypted audit ledger that belongs to you.

**VI. Accountability is Local**

Encrypted, append-only audit ledgers live at the site of execution. Every command, every file mutation, every AI interaction is recorded in local SQLite vaults with AES-256-GCM encryption. A git-backed ledger tracks every file change with cryptographic commit hashes. You do not need the platform to know exactly what happened on your machines.

**VII. Infrastructure is Yours**

No SaaS backend. No telemetry. No phone-home. No cloud dependency of any kind. The entire platform runs via `docker compose` on your hardware. The platform generates its own CA, manages its own certificates, and stores everything in local SQLite. It is fully air-gap capable. You hold every key.

**VIII. Intelligence is Replaceable**

Any model. Any provider. Any OS. Anthropic, OpenAI, Google Gemini, or a local Ollama instance running on your own hardware. The governance layer is the constant; the choice of intelligence is yours. When better models arrive — and they will — swap them in. The safety architecture does not change.

---

## How It Works

You describe what you want in natural language. The AI fans out across your bound servers, pulls real-time context via heartbeat telemetry, reasons about the problem, and proposes a plan. When it needs to execute a command, the proposal passes through a multi-stage refinement pipeline before it ever reaches you for approval.

### The Tribunal

The Tribunal is a heterogeneous multi-model consensus pipeline that refines every proposed command before human review:

1. **Parallel Generation** — Up to five independent AI passes — Axiom (The Minimalist), Concord (The Guardian), Variance (The Exhaustive), Pragma (The Conventional), Nemesis (The Adversary) — each propose a candidate command for the same intent. Diversity comes from the distinct personas, not from per-pass temperature overrides (three passes are enabled by default).
2. **Weighted Vote** — Candidates are normalized, grouped, and scored by position-decay weighting. The strongest consensus wins.
3. **Verification** — A separate convergent verifier persona (The Auditor) evaluates the winner against the original intent and either confirms it (`ok`) or emits a minimal revision.
4. **Human Approval** — The refined command halts. You see exactly what will run, on which system, and why. You approve or deny.
5. **Execution** — The Operator executes locally, records the full output to an encrypted local vault, scrubs the output through Sentinel, and returns only the sanitized result to the AI for its next reasoning step.

```
  You ──── "Fix the disk pressure on the prod nodes"
   │
   ▼
  g8ee (AI Engine) ── investigates ── reasons ── proposes command
   │
   ├── Tribunal Pass 0 · Axiom     ──┐
   ├── Tribunal Pass 1 · Concord   ──┤
   ├── Tribunal Pass 2 · Variance  ──┼── Weighted Vote ── Verifier
   ├── Tribunal Pass 3 · Pragma    ──┤         │
   └── Tribunal Pass 4 · Nemesis   ──┘         ▼
                                    ┌─── Your Approval ───┐
                                    │  "df -h /var/log"   │
                                    │  on: prod-node-3    │
                                    │  [Approve] [Deny]   │
                                    └─────────────────────┘
                                               │
                                               ▼
                                    Operator executes locally
                                    Output → Encrypted Vault (raw, local-only)
                                    Output → Sentinel scrub → AI (sanitized)
                                    AI reasons about result → next step
```

---

## Architecture

The platform is four containers and a binary. The binary is the most interesting part — depending on how you invoke it, it becomes the database server, the certificate authority, or the execution agent on your remote hosts.

| Component | Language | What It Does |
|-----------|----------|-------------|
| **g8es** | Go | The Operator binary in `--listen` mode. SQLite document store, KV store, pub/sub broker, blob store, and the platform's own certificate authority. One binary, zero external dependencies. |
| **g8ee** | Python | AI engine. ReAct reasoning loop, multi-provider LLM abstraction (Gemini, Anthropic, OpenAI, Ollama), Tribunal pipeline, Sentinel integration, and the entire tool-calling control plane. |
| **g8ed** | Node.js | Web dashboard and the single external entry point. FIDO2 passkey auth, SSE streaming, mTLS gateway for Operators, and the human approval interface. |
| **g8ep** | Multi | Test runner and build environment. Compiles Operator binaries for all architectures, runs the full test suite, and handles fleet deployment. |
| **Operator** | Go | The ~4MB static binary deployed to your servers. Executes commands, manages files, maintains encrypted local audit vaults, and communicates over outbound-only mTLS WebSocket. Streams itself to hundreds of hosts in parallel over SSH. |

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

### Security Properties

- **Authentication**: FIDO2/WebAuthn passkeys only. Passwords are not supported and never will be.
- **Transport**: TLS 1.3 everywhere. Platform-generated ECDSA P-384 CA. Per-operator mTLS client certificates issued at claim time.
- **Sentinel**: 46 pre-execution threat detectors (MITRE ATT&CK-mapped). 28 post-execution scrubbing patterns. Indirect prompt injection defense.
- **Sessions**: Encrypted cookies, idle timeout, absolute timeout, IP tracking, replay protection with timestamp + nonce validation.
- **Operator Binding**: System fingerprint permanently bound on first auth. Stolen API keys cannot be used from a different machine.
- **Compliance Alignment**: NSA Zero Trust Implementation Guidelines (exceeds requirements in 6 of 7 pillars), HIPAA-ready architecture, FedRAMP-aligned controls.

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

That is a single command. It downloads the CA, fetches the binary over HTTPS, and starts the Operator. No root, no package manager, no dependencies. The binary self-deletes when the session ends.

---

## What Can Be Built On This

g8e is a governance primitive. The current implementation covers infrastructure investigation and remediation, but the architecture is designed to be extended far beyond what exists today.

- **Fleet-scale incident response** — Bind dozens of Operators across environments. The AI correlates signals across your entire fleet in a single conversation.
- **Cloud governance** — The Cloud Operator for AWS implements Zero Standing Privileges with intent-based IAM. The AI requests permissions through you, with automatic 1-hour expiration. GCP and Azure follow the same pattern.
- **MCP gateway** — External MCP clients (Claude Code, etc.) can execute g8e tools through the full governance pipeline. Standards-based wire format, same human-in-the-loop enforcement.
- **Compliance automation** — Every action is audited locally with encrypted, append-only ledgers and git-backed file versioning. SIEM-ready threat signals with MITRE ATT&CK mapping.
- **Air-gapped environments** — Run the entire platform disconnected from the internet with a local Ollama instance. No external API calls required.
- **Custom Operators** — The Operator is a protocol, not just a binary. Any client that speaks the g8e event protocol can act as an Operator. Build specialized agents for databases, network devices, or any system that needs governed AI interaction.

We have built the foundation, but we want to be fully transparent about the current state of the codebase and platform: it is admittedly raw and the edges are rough. Because a large portion of this code was written by human-driven AI, some of it is rubbish... you will find joined ends that go nowhere, dead code, and areas needing significant cleanup.

Our primary objective right now and for the foreseeable future is hardening the platform and protocol. This means focusing on code refactors for a quality foundation, deep and robust testing, industry-standard accuracy and safety measurements, and third-party security reviews. 

We aim to someday drive the standards for secure, private, and safe human-driven, AI-powered infrastructure management that always keeps humans in control. If you share this vision and are not afraid of rough edges, we welcome you to contribute.

---

## CLI Reference

```bash
./g8e platform build       # First-time build and start
./g8e platform start       # Start without rebuilding
./g8e platform stop        # Stop (data preserved)
./g8e platform wipe        # Wipe app data, restart fresh

./g8e operator build       # Compile Operator for all architectures
./g8e test <component>     # Run component tests (g8ee, g8ed, g8eo)
./g8e test g8ee            # AI engine tests (Python/pytest)
./g8e test g8ed            # Dashboard tests (Node/Vitest)
./g8e test g8eo            # Operator tests (Go)
```

---

## Project Status

**Alpha.** This is a research project built with a paranoia-first security mindset. It has not undergone external audit. Use in production at your own risk and evaluate the [security architecture](docs/architecture/security.md) for yourself.

A significant portion of this codebase was written with AI assistance. If you have been around long enough to know what that means in practice, you already know: there are bugs. There is hallucinated logic. There are abstractions that a human would not have chosen. We built a platform to govern AI agents because we experienced firsthand how dangerous unconstrained agents are — while building this very platform with those same agents.

The irony is not lost on us. It is, in fact, the point.

---

## Contributing

We welcome all contributions. This platform is capable of far more than any single person can build, and the architecture is designed to support capabilities that do not exist yet.

**What we value:**

- Bug fixes and real-world edge cases
- Security hardening and threat model improvements
- New Operator capabilities and tool implementations
- LLM provider integrations and model-specific optimizations
- Documentation, testing, and developer experience
- Novel applications of the governance architecture

If you see something broken, fix it. If you see something missing, build it. If you have an idea for something that does not exist yet, open an issue and let's talk about it.

A good PR that improves any aspect of this platform will be merged. See [CONTRIBUTING.md](CONTRIBUTING.md) for environment setup and guidelines.

---

## Documentation

| Document | Description |
|----------|-------------|
| [Architecture Overview](docs/architecture/about.md) | Origins, governance philosophy, and core principles |
| [Security Architecture](docs/architecture/security.md) | Complete security reference — authentication, Sentinel, LFAA, threat model |
| [AI Control Plane](docs/architecture/ai_control_plane.md) | AI engine internals — ReAct loop, Tribunal, prompts, tools, providers |
| [Operator Binary](docs/architecture/operator.md) | Operator lifecycle, modes, deployment, on-host storage |
| [Developer Guide](docs/developer.md) | Setup, code quality rules, project structure |
| [Testing Guide](docs/testing.md) | Test infrastructure, component guidelines, CI workflows |
| [Glossary](docs/glossary.md) | Platform terminology |

---

## License

[Apache License, Version 2.0](LICENSE).

---

<div align="center">

*g8e is developed by [Lateralus Labs, LLC](https://lateraluslabs.com), a Certified Veteran Owned Small Business (VOSB).*

</div>
