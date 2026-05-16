---
title: Testing
---

# Testing g8e

Last Updated: 2026-05-16
Version: v0.3.0

g8e is designed to be a **testing environment and production environment at the same time**. We do not believe in mocking the world just to get tests to pass. If it doesn't work in the test environment, it won't work in production.

This document outlines the testing architecture, core principles, and how to write and run tests across the g8e stack.

## Core Engineering Principles

- **Hermetic Execution** — Tests run directly on the host via the `./g8e test` runner, using the Go toolchain for the substrate (`g8eo`) and repo-local Python toolchains for explicit app-layer targets (`g8ee`).
- **Real Infrastructure** — All testing occurs against real, live services. A substrate test run begins with `./g8e platform start`, which starts Operator listen mode. App-layer tests require explicit app startup through `./g8e apps start g8ee` or `./g8e platform start --with-apps`.
- **The "No Mocks" Policy** — We strictly prohibit mocking internal services, database clients, or cross-component communication. Integration tests must use the real wire paths.
- **mTLS by Default** — Most internal and substrate communication requires mTLS. The test runner automatically handles certificate injection from `.g8e/pki` if you are authenticated (via `./g8e login`).
- **Real LLM Calls** — AI integration tests use real provider API calls (Gemini, Anthropic, OpenAI, etc.). No HTTP interception is permitted for LLM clients.

## Test Harness Architecture: Substrate, App Adapters, and Evals

The platform maintains distinct test harnesses with strictly separated lifecycle patterns.

### 1. Reference Operator Tests (Protocol Path)
**Command:** `./g8e test` or `./g8e test g8eo`
**Purpose:** Validates the reference Operator (`g8eo`) and its protocol enforcement (UniversalEnvelope, 3-layer Governance, Audit Vault) without requiring Node, Python, or g8ee.
- **Pattern:** Uses Operator listen mode and unified command/result paths.
- **Rationale:** Keeps the required platform boundary small and independently verifiable.

### 2. App Adapter Tests (Explicit Opt-In)
**Command:** `./g8e test g8ee --e2e`
**Purpose:** Validates the optional bundled Engine adapter (`g8ee`).
- **Pattern:** Requires the relevant app adapter to be started explicitly.
- **Rationale:** Verifies bundled clients without making them substrate dependencies.

### 3. Evals (Application-Layer Benchmark Path)
**Command:** `./g8e evals run --operator-session-id <id> --gold-set <path>`
**Purpose:** Evaluates AI agent (Sage) reasoning and tool-calling accuracy against the product surface experienced by users.
- **Pattern:** Uses a bound operator session and hits public HTTPS endpoints.
- **Rationale:** Exercises the product exactly as a user would, asserting that the AI translates intent into safe, correct actions.

## Running Tests

All tests are orchestrated via the `./g8e` CLI, which handles environment configuration, CA certificate injection, and mTLS authentication. **Never call `pytest` or `go test` directly.**

| Command | Runner | Framework | Primary Use |
|---------|--------|-----------|-------------|
| `./g8e test` | Host Go | `go test` | Default substrate test run (g8eo) |
| `./g8e test g8eo` | Host Go | `go test` | Operator listen mode, blob store, pub/sub |
| `./g8e test g8ee` | Host venv | `pytest` | Engine adapter, AI reasoning, tool translation |

### Common Workflow

```bash
# 1. Start the reference Operator
./g8e platform start

# 2. Authenticate (required for mTLS tests)
./g8e login --email user@example.com

# 3. Run substrate tests
./g8e test
./g8e test g8eo services/pubsub

# 4. Start optional apps only when testing app-layer adapters
./g8e apps start g8ee
./g8e test g8ee --pyright --ruff
```

### LLM & Search Configuration

When running AI-integrated tests (`g8ee`), provider settings are passed via env vars or flags:

```bash
./g8e test g8ee -p anthropic -m claude-3-5-sonnet -k <api-key>
```

Available flags: `-p` (provider), `-m` (primary model), `-a` (assistant model), `-l` (lite model), `-k` (api-key), `-e` (endpoint).

## AI Benchmarks & Evaluations (Evals)

The `evals` subsystem manages a fleet of simulated operator nodes (via Docker) to test non-deterministic AI behavior at scale.

### Eval Workflow

```bash
# 1. Bring up a fleet of eval nodes (requires a device token)
./g8e evals deploy --nodes 3 --device-token dlk_...

# 2. Run the evaluation against a gold set
# Requires a bound operator_session_id from a logged-in session
./g8e evals run --operator-session-id osid_... --gold-set benchmark

# 3. View status or logs
./g8e evals status
./g8e evals logs <node-name>

# 4. Tear down the fleet
./g8e evals down
```

### Evaluation Scenarios

- **Benchmark**: Asserts that the AI generates the exact `expected_payload` for a given query (regex-based).
- **Accuracy**: Uses an `EvalJudge` (Primary Model) to score behavior against `expected_behavior`.
- **Privacy**: Asserts that Sentinel scrubber placeholders (e.g., `[PII]`) are present in egress payloads.

## Component Specifics

### Go (g8eo)
- **Tooling**: Uses `gotestsum` if available for dots-style output.
- **Race Detection**: Always enabled via `-race`.
- **Parallelism**: Runs with `-parallel 4` and a `180s` timeout by default.
- **Coverage**: Use `--coverage` to generate and display reports.

### Python (g8ee)
- **Type Safety**: `--pyright` runs strict AST-level type checking using `pyrightconfig.services.json`.
- **Linting**: `--ruff` (and `--ruff-fix`) enforces the project style guide.
- **Parallelism**: `-j auto` or `-j <N>` runs pytest in parallel via `pytest-xdist`.

## Infrastructure Ports

When debugging connectivity, reference these standard ports:
- `9000`: Operator mTLS API
- `9001`: Operator Pub/Sub (WSS)
- `9002`: Operator Bootstrap (HTTP)
- `9003`: Operator Public (BYO Client / Browser)
- `8443`: g8ee Adapter (HTTPS)

## Security & Audit

The platform includes automated verification of its own security posture:

```bash
./g8e security validate     # Verifies mTLS integrity and volume permissions
./g8e security mtls-test    # Tests connectivity between components
./g8e security scan-licenses # Scans dependencies for compliance
```

## Continuous Integration

GitHub Actions (`.github/workflows/build-and-test.yml`) enforce these standards on every PR:
- **Protocol Sync**: The `verify-proto` job ensures that generated Go and Python code is in sync with the `.proto` definitions.
- **Substrate Job**: The blocking `test-g8eo` job installs Go, starts the platform, and runs `./g8e test`.
- **Application Jobs**: `apps-g8ee` installs its own Python toolchain, starts g8ee, and runs its test suite. This job is non-blocking (`continue-on-error: true`).
