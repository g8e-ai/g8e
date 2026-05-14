---
title: Testing
---

# Testing g8e

Last Updated: 2026-05-12
Version: v0.2.4

g8e is designed to be a **testing environment and production environment at the same time**. We do not believe in mocking the world just to get tests to pass. If it doesn't work in the test environment, it won't work in production.

This document outlines the testing architecture, core principles, and how to write and run tests across the g8e stack.

## Core Engineering Principles

- **Hermetic Execution** — Tests run directly on the host via the `./g8e test` runner, using the Go toolchain for the substrate and repo-local Python/Node toolchains only for explicit app-layer targets.
- **Real Infrastructure** — All testing occurs against real, live services. A substrate test run begins with `./g8e platform start`, which starts Operator listen mode only. App-layer tests require explicit app startup through `./g8e apps start ...` or `./g8e platform start --with-apps`.
- **The "No Mocks" Policy** — We strictly prohibit mocking internal services, database clients, or cross-component communication. Integration tests must use the real wire paths. If a mock is deemed absolutely necessary, it must be justified and documented.
- **Real LLM Calls** — AI integration tests use real provider API calls (Gemini, Anthropic, OpenAI, etc.). No `MagicMock` or HTTP interception is permitted for LLM clients. Transient failures are handled via exponential backoff in the reasoning engine.

## Test Harness Architecture: Substrate, App Adapters, and Evals

The platform maintains distinct test harnesses with strictly separated lifecycle patterns.

### 1. Substrate Tests (Operator/Protocol Path)
**Command:** `./g8e test` or `./g8e test g8eo`
**Purpose:** Validates the Operator/protocol substrate without requiring Node, Python, or g8ee.
- **Pattern:** Uses Operator listen mode and unified command/result paths.
- **Rationale:** Keeps the required platform boundary small and independently verifiable.

### 2. App Adapter Tests (Explicit Opt-In)
**Command:** `./g8e test g8ee --e2e`
**Purpose:** Validates optional bundled adapter behavior.
- **Pattern:** Requires the relevant app adapter to be started explicitly.
- **Rationale:** Verifies bundled clients without making them substrate dependencies.

### 3. Evals (Application-Layer Benchmark Path)
**Command:** `./g8e evals run --gold-set <name|path>`
**Purpose:** Evaluates AI agent (Sage) reasoning and tool-calling accuracy against the product surface experienced by users.
- **Pattern:** Uses public device-link tokens and hits public HTTPS endpoints.
- **Rationale:** Exercises the product exactly as a user would, asserting that the AI translates intent into safe, correct actions.

## Running Tests

All tests are orchestrated via the `./g8e` CLI, which handles environment configuration, CA certificate injection, and internal authentication. **Never call `pytest`, `vitest`, or `go test` directly.**

| Command | Runner | Framework | Primary Use |
|---------|--------|-----------|-------------|
| `./g8e test` | Host Go | `go test` | Default substrate test run |
| `./g8e test g8eo` | Host Go | `go test` | Operator listen mode, blob store, pub/sub |
| `./g8e test g8ee` | Host venv | `pytest` | Optional Engine adapter, AI reasoning, tool translation |

### Common Workflow

```bash
# 1. Start the Operator/protocol substrate
./g8e platform start

# 2. Run substrate tests
./g8e test
./g8e test g8eo --coverage

# 3. Start optional apps only when testing app-layer adapters
./g8e apps start g8ee
./g8e test g8ee --pyright --ruff
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
- **Substrate Job**: The blocking `test-g8eo` job installs Go only, starts `./g8e platform start`, and runs `./g8e test`.
- **Application Jobs**: `apps-g8ee` installs its own Python toolchain, starts only the relevant optional adapter, and is non-blocking.
- **Diagnostic Logging**: Operator logs are printed for substrate failures; app jobs also print their adapter logs.
