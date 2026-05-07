---
title: Testing
---

# Testing g8e

Last Updated: 2026-05-07
Version: v0.2.0

g8e is designed to be a **testing environment and production environment at the same time**. We do not believe in mocking the world just to get tests to pass. If it doesn't work in the test environment, it won't work in production.

This document outlines the testing architecture, core principles, and how to write and run tests across the g8e stack.

## Core Engineering Principles

- **Hermetic Execution** — Each component runs tests inside a dedicated test-runner container (`g8ee-test-runner`, `g8ed-test-runner`, `g8eo-test-runner`). Source code is volume-mounted, meaning local development and CI execution are perfectly identical. For manual troubleshooting, use the `g8ep` container which includes all necessary tooling.
- **Real Infrastructure** — All testing must occur against real services and real inter-component communications. This means using a real `CacheAsideService` with a real `g8es` backend, real pub/sub over WebSockets, and real network stacks.
- **The "No Mocks" Policy** — We strictly prohibit mocking internal services, database clients, or LLM providers. Integration tests must use real services. If a scenario is extremely difficult to test without a mock, you must justify its necessity in the PR.
- **Real LLM Calls** — AI tests use real provider API calls. No `MagicMock`, `AsyncMock`, or HTTP interception on LLM clients. The system handles transient failures via exponential backoff in `EvalJudge`.
- **Contract Enforcement** — We use `scripts/testing/check_model_parity.py` to ensure our source-of-truth constants (events, statuses, JSON schemas) match across Go, Python, and Node.js. This script runs automatically before every `./g8e test` command.

## Test Harness Architecture: E2E vs Evals

The platform maintains two distinct "real operator" test harnesses with **strictly separated authentication and lifecycle patterns**. This separation is architectural and must be enforced in code review.

### 1. E2E Tests (Internal Lifecycle Path)
**Command:** `./g8e test g8ee --e2e` or `./g8e test g8ed test/integration/setup`
**Purpose:** Validate the internal operator lifecycle and platform infrastructure.
- **Pattern:** Provisions operator slots via `g8ed` internal API, uses `X-Internal-Auth`, reads API keys from `g8es`.
- **Rationale:** Validates the platform's own internal mechanisms.

### 2. Evals (Public Device-Token Path)
**Command:** `./g8e evals up -d <token>` then `./g8e evals run --gold-set <path>`
**Purpose:** Evaluate AI agent (Sage) behavior against the product surface that real users experience.
- **Pattern:** Uses public device-link tokens, hits public `g8ed` HTTPS endpoints, no internal auth.
- **Rationale:** Exercises the product exactly as users experience it.

## Running Tests

All tests are orchestrated via the `./g8e` CLI, which routes each component to its dedicated test-runner container. **Never call `go test`, `pytest`, or `vitest` directly on your host machine.**

| Command | Container | Framework | Primary Use |
|---------|-----------|-----------|-------------|
| `./g8e test g8ee` | `g8ee-test-runner` | `pytest` | Python logic, AI integration |
| `./g8e test g8ed` | `g8ed-test-runner` | `vitest` | Node.js API, Orchestration |
| `./g8e test g8eo` | `g8eo-test-runner` | `gotestsum` | Go Operator, Low-level tools |

### Common Commands

```bash
# Start the platform infrastructure first

Last Updated: 2026-05-07
Version: v0.2.0
./g8e platform start

# Authenticate once to local store

Last Updated: 2026-05-07
Version: v0.2.0
./g8e login

# Run a specific component

Last Updated: 2026-05-07
Version: v0.2.0
./g8e test g8ee

# Run with coverage

Last Updated: 2026-05-07
Version: v0.2.0
./g8e test g8eo --coverage

# Run g8ee with parallelism and strict type checking

Last Updated: 2026-05-07
Version: v0.2.0
./g8e test g8ee -j auto --pyright --ruff
```

## AI Benchmarks & Evaluations (Evals)

Evaluating non-deterministic AI models requires a multi-layered approach using the `evals` subsystem. Evals are **not** pytest-driven and run outside the standard test-runner containers.

### Eval Workflow

```bash
# 1. Start a fleet of real operator nodes linked via a device token

Last Updated: 2026-05-07
Version: v0.2.0
./g8e evals up --device-token dlk_xxx --nodes 3

# 2. Run the eval runner (executes in g8ep)

Last Updated: 2026-05-07
Version: v0.2.0
./g8e evals run --gold-set evals/gold_sets/accuracy.json --device-token dlk_xxx

# 3. View status and logs

Last Updated: 2026-05-07
Version: v0.2.0
./g8e evals status
./g8e evals logs eval-node-01

# 4. Tear down the fleet

Last Updated: 2026-05-07
Version: v0.2.0
./g8e evals down
```

### Evaluation Types

- **Deterministic Benchmarks**: Tool-call payloads graded via regex matching in `scorer.py` against `benchmark.json`.
- **Subjective Evaluations**: `EvalJudge` uses the Primary Model to score the Assistant Model (Sage) against `accuracy.json`.
- **Privacy Evaluation**: Asserts Sentinel scrubber placeholders (`[PII]`, `[AWS_KEY]`, etc.) are present in egress payloads via `privacy.json`.

## Component Testing Strategies

### Go (g8eo)
- **gotestsum**: Used for formatted output and race detection (`-race`).
- **Loopback**: Uses an in-process `PubSubBroker` over real WebSockets for full wire-path validation without a live `g8es`.
- **Integration**: Tagged with `//go:build integration`. Requires live platform infrastructure.

### Node.js (g8ed)
- **vitest**: Primary test runner for the orchestration layer.
- **Test Services**: Uses `getTestServices()` singleton to ensure consistent service initialization.
- **Isolation**: Managed by `TestCleanupHelper` using `_test` collection suffixes.

### Python (g8ee)
- **pytest**: Primary test runner for the AI engine.
- **Type Safety**: `--pyright` runs strict AST-level type checking before tests.
- **Linting**: `--ruff` (and `--ruff-fix`) enforces code style.
- **Markers**: `@pytest.mark.ai_integration` and `@pytest.mark.requires_web_search` manage tests requiring external credentials.

## Security & Audit

The platform includes automated security verification tools run via `g8ep`.

```bash
# Run mTLS and configuration audit

Last Updated: 2026-05-07
Version: v0.2.0
./g8e security mtls-test

# Validate platform security posture

Last Updated: 2026-05-07
Version: v0.2.0
./g8e security validate

# Scan for dependency licenses

Last Updated: 2026-05-07
Version: v0.2.0
./g8e security scan-licenses
```

## Shared Test Fixtures

Shared fixtures in `shared/test-fixtures/` (e.g., `sse-events.json`) are used to enforce cross-component wire compatibility. Contract tests in each component verify that emitted events match these shared structures.

## Continuous Integration

Our GitHub workflows (`.github/workflows/build-and-test.yml`) enforce:
- Multi-architecture container builds (amd64, arm64).
- Matrix execution of all component tests against real infrastructure.
- Contract enforcement via `check_model_parity.py`.
- Automated eval runs for core agent behaviors.
