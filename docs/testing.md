---
title: Testing
---

# Testing g8e

Last Updated: 2026-05-10
Version: v0.2.2

g8e is designed to be a **testing environment and production environment at the same time**. We do not believe in mocking the world just to get tests to pass. If it doesn't work in the test environment, it won't work in production.

This document outlines the testing architecture, core principles, and how to write and run tests across the g8e stack.

## Core Engineering Principles

- **Hermetic Execution** — Each component runs tests directly on the host via the `./g8e test` runner, using repo-local toolchains (Python venv, Node npm, Go toolchain) and runtime state managed under the `.g8e` directory.
- **Real Infrastructure** — All testing occurs against real, live services. A test run typically begins with `./g8e platform start` to ensure a real `operator` (listen mode), `g8ed`, and `g8ee` are active and connected.
- **The "No Mocks" Policy** — We strictly prohibit mocking internal services, database clients, or cross-component communication. Integration tests must use the real wire paths. If a mock is deemed absolutely necessary, it must be justified and documented.
- **Real LLM Calls** — AI integration tests use real provider API calls (Gemini, Anthropic, OpenAI, etc.). No `MagicMock` or HTTP interception is permitted for LLM clients. Transient failures are handled via exponential backoff in the reasoning engine.

## Test Harness Architecture: E2E vs Evals

The platform maintains two distinct test harnesses with strictly separated authentication and lifecycle patterns.

### 1. E2E Tests (Internal Lifecycle Path)
**Command:** `./g8e test g8ee --e2e`
**Purpose:** Validates the internal platform infrastructure and operator lifecycle.
- **Pattern:** Uses `X-Internal-Auth` tokens and internal API paths.
- **Rationale:** Verifies that the platform can correctly provision, authenticate, and manage operator nodes.

### 2. Evals (Public Device-Token Path)
**Command:** `./g8e evals run --gold-set <name|path>`
**Purpose:** Evaluates AI agent (Sage) reasoning and tool-calling accuracy against the product surface experienced by users.
- **Pattern:** Uses public device-link tokens and hits public HTTPS endpoints.
- **Rationale:** Exercises the product exactly as a user would, asserting that the AI translates intent into safe, correct actions.

## Running Tests

All tests are orchestrated via the `./g8e` CLI, which handles environment configuration, CA certificate injection, and internal authentication. **Never call `pytest`, `vitest`, or `go test` directly.**

| Command | Runner | Framework | Primary Use |
|---------|--------|-----------|-------------|
| `./g8e test g8ee` | Host venv | `pytest` | AI reasoning, tool translation, engine logic |
| `./g8e test g8ed` | Host Node.js | `vitest` | Dashboard, API Gateway, session management |
| `./g8e test g8eo` | Host Go | `go test` | Operator listen mode, blob store, pub/sub |

### Common Workflow

```bash
# 1. Start the platform infrastructure
./g8e platform start

# 2. Run component tests
./g8e test g8ee                     # Run all g8ee unit/integration tests
./g8e test g8ee --pyright --ruff    # Run with strict type checking and linting
./g8e test g8eo --coverage          # Run Go tests with race detection and coverage
```

### LLM & Search Configuration

When running AI-integrated tests, you can override provider settings via CLI flags:

```bash
./g8e test g8ee -p anthropic -m claude-3-5-sonnet -k <api-key>
```

Available flags: `-p` (provider), `-m` (primary model), `-a` (assistant model), `-l` (lite model), `-k` (api-key), `-e` (endpoint).

## AI Benchmarks & Evaluations (Evals)

The `evals` subsystem manages a dedicated fleet of simulated operator nodes to test non-deterministic AI behavior at scale.

### Eval Workflow

```bash
# 1. Bring up and authenticate a fleet of eval nodes
./g8e evals deploy --nodes 3 --device-token <token>

# 2. Run the evaluation against a gold set
./g8e evals run --gold-set benchmark

# 3. View logs or status
./g8e evals status
./g8e evals logs evals-eval-node-1

# 4. Tear down the fleet
./g8e evals down
```

### Evaluation Scenarios

- **Benchmark**: Asserts that the AI generates the exact `expected_payload` for a given query (regex-based).
- **Accuracy**: Uses an `EvalJudge` (Primary Model) to score the `Assistant Model` behavior against `expected_behavior` and `required_concepts`.
- **Privacy**: Asserts that Sentinel scrubber placeholders (e.g., `[PII]`) are present in egress payloads.

## Component Specifics

### Go (g8eo)
- **gotestsum**: Automatically used if installed for formatted output.
- **Race Detection**: Always enabled via `-race`.
- **Parallelism**: Runs with `-parallel 4` and a `180s` timeout by default.

### Node.js (g8ed)
- **Vitest**: Runs in `forks` pool for isolation.
- **Cleanup**: Uses `TestCleanupHelper` to ensure no database pollution between runs.
- **Coverage**: Uses the `v8` provider; reports generated in `components/g8ed/coverage`.

### Python (g8ee)
- **Type Safety**: `--pyright` runs strict AST-level type checking using `pyrightconfig.services.json`.
- **Linting**: `--ruff` (and `--ruff-fix`) enforces the project style guide.
- **Markers**: Uses markers like `@pytest.mark.ai_integration` and `@pytest.mark.requires_web_search` to manage external dependencies.

## Security & Audit

The platform includes automated verification of its own security posture:

```bash
./g8e security validate     # Verifies mTLS integrity and volume permissions
./g8e security mtls-test    # Tests connectivity between components
./g8e security scan-licenses # Scans dependencies for compliance
```

## Continuous Integration

GitHub Actions (`.github/workflows/build-and-test.yml`) enforce these standards on every PR:
- **Host-Native Setup**: CI environments are configured with Go, Python 3.12, and Node 22.
- **Platform Bootstrap**: Each job starts the platform via `./g8e platform start`.
- **Strict Gating**: Failures in `pyright`, `ruff`, or any test suite block the merge.
- **Diagnostic Logging**: Logs from `.g8e/logs/*` are printed on failure to assist in debugging silent exits.
