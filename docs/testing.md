---
title: Testing
---

# Testing g8e

Last Updated: 2026-05-10
Version: v0.2.1

g8e is designed to be a **testing environment and production environment at the same time**. We do not believe in mocking the world just to get tests to pass. If it doesn't work in the test environment, it won't work in production.

This document outlines the testing architecture, core principles, and how to write and run tests across the g8e stack.

## Core Engineering Principles

- **Hermetic Execution** — Each component runs tests through the `./g8e test` host-native runner, using managed repo-local toolchains and runtime state under `.g8e`.
- **Real Infrastructure** — All testing must occur against real services and real inter-component communications. This means using a real `CacheAsideService` with a real `operator` backend, real pub/sub over WebSockets, and real network stacks.
- **The "No Mocks" Policy** — We strictly prohibit mocking internal services, database clients, or LLM providers. Integration tests must use real services. If a scenario is extremely difficult to test without a mock, you must justify its necessity in the PR.
- **Real LLM Calls** — AI tests use real provider API calls. No `MagicMock`, `AsyncMock`, or HTTP interception on LLM clients. The system handles transient failures via exponential backoff in `EvalJudge`.

## Test Harness Architecture: E2E vs Evals

The platform maintains two distinct "real operator" test harnesses with **strictly separated authentication and lifecycle patterns**. This separation is architectural and must be enforced in code review.

### 1. E2E Tests (Internal Lifecycle Path)
**Command:** `./g8e test g8ee --e2e` or `./g8e test g8ed test/integration/setup`
**Purpose:** Validate the internal operator lifecycle and platform infrastructure.
- **Pattern:** Provisions operator slots via `g8ed` internal API, uses `X-Internal-Auth`, reads API keys from `operator`.
- **Rationale:** Validates the platform's own internal mechanisms.

### 2. Evals (Public Device-Token Path)
**Command:** `./g8e evals up -d <token>` then `./g8e evals run --gold-set <path>`
**Purpose:** Evaluate AI agent (Sage) behavior against the product surface that real users experience.
- **Pattern:** Uses public device-link tokens, hits public `g8ed` HTTPS endpoints, no internal auth.
- **Rationale:** Exercises the product exactly as users experience it.

## Running Tests

All tests are orchestrated via the `./g8e` CLI, which configures the required host-native environment for each component. **Never call `go test`, `pytest`, or `vitest` directly; use `./g8e test` so bootstrap paths, CA trust, and internal auth are configured consistently.**

| Command | Runner | Framework | Primary Use |
|---------|--------|-----------|-------------|
| `./g8e test g8ee` | Host virtualenv | `pytest` | Python logic, AI integration |
| `./g8e test g8ed` | Host Node.js | `vitest` | Node.js API, Orchestration |
| `./g8e test g8eo` | Host Go toolchain | `gotestsum` or `go test` | Go Operator, Low-level tools |

### Common Commands

```bash
# Start the platform infrastructure first

./g8e platform start

# Authenticate once to local store

./g8e login

# Run a specific component

./g8e test g8ee

# Run with coverage

./g8e test g8eo --coverage

# Run g8ee with parallelism and strict type checking

./g8e test g8ee -j auto --pyright --ruff
```

## AI Benchmarks & Evaluations (Evals)

Evaluating non-deterministic AI models requires a multi-layered approach using the `evals` subsystem. Evals are **not** pytest-driven and run outside the standard component test runner.

### Eval Workflow

```bash
# 1. Start a fleet of real operator nodes linked via a device token

./g8e evals up --device-token dlk_xxx --nodes 3

# 2. Run the eval runner

./g8e evals run --gold-set components/g8ee/evals/gold_sets/accuracy.json --device-token dlk_xxx

# 3. View status and logs

./g8e evals status
./g8e evals logs eval-node-01

# 4. Tear down the fleet

./g8e evals down
```

### Evaluation Types

- **Deterministic Benchmarks**: Tool-call payloads graded via regex matching in `scorer.py` against `benchmark.json`.
- **Subjective Evaluations**: `EvalJudge` uses the Primary Model to score the Assistant Model (Sage) against `accuracy.json`.
- **Privacy Evaluation**: Asserts Sentinel scrubber placeholders (`[PII]`, `[AWS_KEY]`, etc.) are present in egress payloads via `privacy.json`.

The `./g8e evals run` wrapper executes the Python eval runner with the repository source tree and active eval fleet configuration. Gold sets may be referenced by short name (`accuracy`) or host repo path (`components/g8ee/evals/gold_sets/accuracy.json`).

## Component Testing Strategies

### Go (g8eo)
- **gotestsum**: Used for formatted output and race detection (`-race`).
- **Loopback**: Uses an in-process `PubSubBroker` over real WebSockets for full wire-path validation without a live `operator`.
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

The platform includes automated security verification tools.

```bash
# Run mTLS and configuration audit

./g8e security mtls-test

# Validate platform security posture

./g8e security validate

# Scan for dependency licenses

./g8e security scan-licenses
```

## Shared Test Fixtures

Shared fixtures in `shared/test-fixtures/` (e.g., `sse-events.json`) are used to enforce cross-component wire compatibility. Contract tests in each component verify that emitted events match these shared structures.

## Continuous Integration

Our GitHub workflows (`.github/workflows/build-and-test.yml`) enforce:
- Host-native component test jobs with explicit Go, Python, and Node.js setup.
- Real platform startup through `./g8e platform start`; CI does not set up Docker for component tests.
- Component tests through `./g8e test g8ee`, `./g8e test g8ed`, and `./g8e test g8eo`.
- Diagnostic platform log output on failure so silent runner exits expose root-cause logs.
