# Contributing to g8e

Hi! We're thrilled you want to help out with g8e. Whether you're fixing a bug, cleaning up code, or suggesting a new feature, we're happy to have you here. This document defines the technical standards, architectural invariants, and contribution workflow for the g8e platform.

---

## Core Architectural Invariants

1. **Human agency is absolute.** Every state-changing operation surfaces its own approval prompt. Automatic Function Calling is permanently disabled.
2. **3-layer governance bedrock.** L1 hard gates (forbidden patterns) via protobuf reflection; L2 multi-agent Tribunal consensus with reputation staking; L3 human-in-the-loop with hardware-bound WebAuthn proofs (auto-approval for benign diagnostics only after L1+L2 pass).
3. **Data sovereignty.** Raw command output and file contents stay on the Operator host, encrypted, and never persist platform-side. Platform state is host-native under `.g8e/`.
4. **Security by structure.** All changes adhere to the Security Review Checklist. The Operator is the only execution boundary.

---

## Development Lifecycle

Components run host-native. **Do not use Docker for primary component development or testing.**

### Setup

- **Go 1.22+** (required, for `g8eo`).
- **Python 3.12+** (only when developing the optional `g8ee` adapter).
- **Node 22** (only when developing optional Dashboard/GUI).

### Common Commands

| Command | Purpose |
|---|---|
| `./g8e` | Interactive Platform Manager. |
| `./g8e platform start` | Start the reference Operator (`g8eo`) only. |
| `./g8e platform start --with-apps` | Operator plus optional bundled adapters. |
| `./g8e apps start [g8ee|all]` | Start optional application-layer adapters. |
| `./g8e platform status` | Substrate health first, optional app status separately. |
| `./g8e login` | Authenticate the local CLI. |
| `./g8e test <component>` | Host-native tests (default: g8eo). |

### Scripts & CLI

The root `./g8e` script is a Bash-based dispatcher and the single entry point for all platform operations.

- **Platform Management (`./g8e platform`)**: Orchestrates the mandatory substrate lifecycle via `scripts/core/build.sh`.
- **Application Layer (`./g8e apps`)**: Manages optional, opt-in adapters (like `g8ee`) that extend the platform's capabilities.
- **Operator Operations (`./g8e operator`)**: Lifecycle management for `g8eo` binaries and remote fleet deployment.
- **Infrastructure & Data (`./g8e data` / `./g8e security`)**: Unified interface for interacting with the substrate state and security invariants.

**Technical Invariants:**
1. **Path Resolution**: All scripts must resolve `G8E_PROJECT_ROOT` relative to their own location.
2. **Service Readiness**: The platform is not "ready" until the Operator listen-mode health check (`/healthz`) passes.
3. **Canonical Wire Format**: All client-facing interaction (HTTP, PubSub, receipts) must use **canonical JSON (protojson)**. Binary Protobuf is reserved for internal storage.
4. **Fail-Closed Execution**: Scripts must never mask failures or proceed with missing trust material.

---

## Architecture Philosophy

g8e is split into the **Protocol (substrate)**, an **Operator** that implements it on a host, and an optional **Application Layer**.

- **Protocol (substrate)** — Shared `.proto` schemas plus the canonical-JSON wire contract; the source of truth for what every Operator and client must honor.
- **Reference Operator (`g8eo`)** — In `--listen` mode it is the foundational service for the bundled deployment: generates the platform CA, foundational secrets, and exposes the public protocol API.
- **Reference Application Layer (optional)** — Reference adapters like `g8ee` consume the public protocol surface on equal footing with any BYO client.
- **Host-native execution** — Core components run as native processes.
- **Zero-config discovery** — Services use a standardized local runtime directory (`.g8e/`) for discovery and configuration sharing.

---

## Build Pipeline & Dependencies

| Component | Role | Runtime | Build |
|---|---|---|---|
| Operator (`g8eo`) | Reference Operator: Persistence, Pub/Sub, Root of Trust | Host Go binary | Native Go via `Makefile` |
| Engine (`g8ee`) | Optional Adapter: AI Backend & Workflow Orchestration | Python 3.12 venv | `pip install` into local `.venv` |

### Host-native Startup Lifecycle

The `./g8e platform start` command (invoked via `scripts/core/build.sh`) manages the sequence:
1. **Operator binary check/build** → Operator starts in `--listen mode`.
2. **Root of trust generation** (first boot only) — ECDSA P-384 CA hierarchy, intermediate CAs, and trust bundles in `.g8e/pki/`; `session_encryption_key`, `warden_signing_key` in `.g8e/secrets/`.
3. **Optional service initialization** — `g8ee` starts under its venv with mTLS + URI SAN identity.
4. **Asynchronous convergence** — Services poll health endpoints (e.g., Engine polls `https://localhost:9000/health`).

---

## State & Data Strategy

All runtime state is rooted at `./.g8e/`.

| Path | Purpose | `wipe` | `reset` | `clean` |
|---|---|---|---|---|
| `.g8e/pki/` | CA, intermediates, trust bundles | preserve | preserve | nuke |
| `.g8e/secrets/` | Bootstrap secrets (session key) | preserve | wipe | nuke |
| `.g8e/data/` | SQLite + blobs | wipe (API) | wipe | nuke |
| `.g8e/logs/` | Component stdout/stderr | — | — | nuke |
| `.g8e/pids/` | Process IDs | clear | clear | nuke |

- **`./g8e platform wipe`**: Clears application-layer data via the Operator API but preserves platform settings.
- **`./g8e platform reset`**: Deletes the database and secrets, but keeps the CA cert so client trust is maintained.
- **`./g8e platform clean`**: Destructive removal of the entire `.g8e/` directory and all running processes.

---

## Anti-Tech-Debt Directives

AI agents tend to wrap poorly understood code in new abstractions. This is strictly forbidden.

1. **Rip and replace.** When existing code violates contracts or is structurally unsound, replace it correctly. Do not route around it with a wrapper. **No backwards compatibility** is maintained for broken data structures or legacy shims.
2. **Prohibited patterns.** `ensure*()`, `getOrCreate*()`, `Any` in type signatures, and `map[string]interface{}` for known shapes are hard stops. Functions do exactly one thing: reads read, writes write.
3. **No defensive guards.** Never add defensive code at the call site to handle unexpected values. Hunt down the root cause and fix it at the source.

---

## Application Boundary and State Management

1. **Single source of truth.** The `protocol/` directory is canonical for all wire-protocol values and cross-component document schemas.
2. **Strict typing.** Inside the application boundary, data lives exclusively as typed model instances. Raw dicts, untyped maps, and ad-hoc JSON are prohibited.
3. **Canonical JSON wire format.** Mutation envelopes are canonical-JSON `GovernanceEnvelope` carrying a base64-encoded binary protobuf `payload`.
4. **Cache-aside discipline.** All document operations go through `CacheAsideService`. The DB is authoritative for writes; the KV store is the primary read path.

---

## Component Rules

### g8eo / Operator (Go)
- **LFAA payload stamping** — All LFAA results include an `execution_id`.
- **Concurrency** — Goroutines have explicit cancellation contexts and clear channel ownership.
- **Protocol boundary** — Any capability needed by bundled apps or BYO clients is exposed through the public Operator protocol.
- **Execution boundary** — Warden is the sole circuit breaker before dispatch. Every accepted mutation emits a signed `ActionReceipt`.

### g8ee (Python/FastAPI, optional adapter)
- **Pydantic enforcement** — Domain objects extend `G8eBaseModel`. Pydantic enforces type checking and rejects extra fields.
- **Async safety** — Avoid state-modifying `finally` blocks in async generators.
- **Adapter boundary** — Must produce protocol-verifiable proposals and proofs.

---

## Testing

g8e is designed to be a testing environment and production environment at the same time. We do not mock internal services, database clients, or cross-component communication.

### Core Principles
1. **Reproduce first.** Always reproduce a bug with a failing test before generating the fix.
2. **No mocks.** Real database, real pub/sub, real LLM calls.
3. **Contract tests.** Enforce alignment between Operator, adapters, and `protocol/` with typed protobuf assertions.
4. **mTLS by Default.** Most communication requires mTLS. The test runner handles certificate injection from `.g8e/pki`.

### Test Runners
All tests are orchestrated via the `./g8e` CLI. **Never call `pytest` or `go test` directly.**

| Command | Runner | Framework | Primary Use |
|---|---|---|---|
| `./g8e test` | Host Go | `go test` | Default substrate test run (g8eo) |
| `./g8e test g8eo` | Host Go | `go test` | Operator listen mode, pub/sub |
| `./g8e test g8ee` | Host venv | `pytest` | Engine adapter, AI reasoning |

---

## Evals (AI Benchmarks)

The evals harness drives the **real g8ee chat pipeline end-to-end** — Triage, Dash/Sage, Tribunal, Auditor, Warden — and fold the full agent trail into a per-task receipt.

```bash
# 1. Start the platform and login
./g8e platform start
./g8e login

# 2. Run the evaluation benchmark
./g8e evals bench --suite ifeval --model openai:gpt-4o

# 3. Verify ActionReceipts offline
./g8e evals verify-receipts reports/ifeval-<timestamp>/
```

`g8e_evals.receipts.verify.verify_receipt_signature` performs offline Ed25519 verification using the Warden public key from `.g8e/pki/warden_pub.pem`.

---

## Documentation Guidelines

- **Docs are code.** Documentation is maintained with the same discipline as source code; stale or inaccurate docs are bugs.
- **Authoritative, not aspirational.** Document what the system does, not what it should do.
- **No redundancy.** Each fact lives in exactly one place; cross-link rather than repeat.
- **Writing style.** Present tense, active voice, direct and specific. No filler, no emojis.
- **Single source of truth.** The `protocol/` directory is canonical for all wire-protocol values.

### The `updatedocs` Workflow
1. **Code-first discovery.** Never trust existing documentation. Verify against the implementation.
2. **High signal, low noise.** Focus on system lifecycle and request/data progression.
3. **Why vs. how.** `.md` files explain high-level concepts; implementation details belong in code.

---

## Where to Find Things

| Concern | Location |
|---|---|
| Protobuf schemas | `protocol/proto/` |
| Constants registries | `protocol/constants/` |
| Operator implementation | `services/g8eo/` |
| Engine implementation | `services/g8ee/` |
| Evaluation harness | `evals/` |
| CLI dispatcher | `g8e`, `scripts/` |

---

## Submitting a PR

1. Keep it focused (one change per PR is best).
2. Add a test if you're fixing a bug or adding a feature.
3. Use a clear prefix in your commit like `g8eo: fix the thing`.
4. We'll jump in to review as soon as we can!

## Get in Touch

Have questions? Email danny@g8e.ai. It's the fastest way to get help or talk shop.

## The Fine Print (CLA)

By contributing, you grant us a license to use your work in g8e (Apache 2.0). You still own your code, but you're giving us permission to build the platform with it. Thanks for helping us grow!
