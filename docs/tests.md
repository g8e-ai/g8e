---
title: Tests
---

# Testing g8e

Last Updated: 2026-05-18

g8e is designed to be a **testing environment and a production environment at the same time**. We do not mock the world to make tests pass. If it does not work in the test environment, it will not work in production.

---

## Core Engineering Principles

- **Hermetic execution** - Tests run directly on the host via `./g8e test`. Go for the substrate (`g8eo`); repo-local Python for explicit app-layer targets (`g8ee`).
- **Real infrastructure** - Substrate test runs begin with `./g8e platform start`. App-layer tests require explicit app startup via `./g8e apps start g8ee` or `./g8e platform start --with-apps`.
- **No mocks policy** - Mocking internal services, database clients, or cross-component communication is prohibited. Integration tests use the real wire paths.
- **mTLS by default** - Most internal and substrate communication requires mTLS. The runner injects certs from `.g8e/pki` automatically when authenticated (`./g8e login`).
- **Body-embedded context** - Business and session context is provided as a `RequestContext` in the request body. `X-G8E-*` context headers are not supported and are ignored by the substrate.
- **Real LLM calls** - AI integration tests use real provider APIs (Gemini, Anthropic, OpenAI, etc.). HTTP interception is not permitted for LLM clients.
- **Reproduce first** - Always reproduce a bug with a failing test before generating a fix.
- **Contract tests** - Enforce alignment between the Operator, optional adapters, and `protocol/` constants/models with typed protobuf assertions.

---

## Test Harness Architecture

### 1. Reference Operator Tests (Protocol Path)

```bash
./g8e test            # default
./g8e test g8eo
./g8e test g8eo services/pubsub
```

Validates the reference Operator (`g8eo`) and its protocol enforcement (`GovernanceEnvelope`, 3-layer governance, Audit Vault) without requiring Node, Python, or g8ee. Uses Operator listen mode and unified command/result paths. Keeps the required platform boundary small and independently verifiable.

### 2. App Adapter Tests (Explicit Opt-In)

```bash
./g8e test g8ee --e2e
./g8e test g8ee --pyright --ruff
```

Validates the optional bundled Engine adapter (`g8ee`). Requires the relevant app adapter to be started explicitly. Verifies bundled clients without making them substrate dependencies.

### 3. Evals (Application-Layer Benchmark Path)

```bash
./g8e evals bench --suite ifeval
```

Evaluates AI agent reasoning and tool-calling accuracy using signed `ActionReceipts`. Uses a real Operator and produces verified audit references. Exercises the product exactly as a user would. Detailed in [Evals](evals.md).

---

## Common Workflow

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

### LLM & search configuration

When running AI-integrated tests, provider settings pass via env or flags:

```bash
./g8e test g8ee -p anthropic -m claude-3-5-sonnet -k <api-key>
```

Available flags: `-p` (provider), `-m` (primary model), `-a` (assistant model), `-l` (lite model), `-k` (api key), `-e` (endpoint).

---

## Component Specifics

### Go (g8eo)

- **Tooling** - `gotestsum` if available for dots-style output.
- **Race detection** - Always enabled via `-race`.
- **Parallelism** - `-parallel 4` and a `180s` timeout by default.
- **Coverage** - `--coverage` generates and displays reports.
- **Concurrency invariants** - Goroutines must have explicit cancellation contexts and clear channel ownership. LFAA payloads must include an `execution_id`.

### Python (g8ee)

- **Type safety** - `--pyright` runs strict AST-level checking via `pyrightconfig.services.json`.
- **Linting** - `--ruff` (and `--ruff-fix`) enforces project style.
- **Parallelism** - `-j auto` or `-j <N>` runs pytest in parallel via `pytest-xdist`.
- **Pydantic enforcement** - Domain objects extend `G8eBaseModel`. Extra fields are rejected.

### Lints

- **`make lint-no-bare-session-id`** - CI-enforced lint preventing bare `sessionid` in the codebase. Excludes vendor, generated files, `.local.dev`, `.github`, and the Makefile itself.

---

## Infrastructure Ports

When debugging connectivity:

- `9000` - Operator mTLS API
- `9001` - Operator Pub/Sub (WSS)
- `9002` - Operator Bootstrap (HTTP)
- `9003` - Operator Public (BYO/Browser)
- `8443` - g8ee Adapter (HTTPS)

---

## Security & Audit

The platform includes automated verification of its own security posture:

```bash
./g8e security validate      # Verifies mTLS integrity and volume permissions
./g8e security mtls-test     # Tests connectivity between components
./g8e security scan-licenses # Scans dependencies for compliance
```

---

## Continuous Integration

GitHub Actions (`.github/workflows/build-and-test.yml`) enforces:

- **`verify-proto`** - Generated Go and Python code is in sync with `.proto` definitions.
- **`test-g8eo`** (blocking) - Installs Go, starts the platform, runs `./g8e test`.
- **`apps-g8ee`** (non-blocking, `continue-on-error: true`) - Installs Python, starts g8ee, runs its suite.

See also: [Evals](evals.md), [Scripts](scripts.md), [Contribution Guide](../CONTRIBUTING.md).
