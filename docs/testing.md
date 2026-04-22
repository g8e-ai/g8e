---
title: Testing
---

# Testing g8e

g8e is designed to be a **testing environment and production environment at the same time**. We do not believe in mocking the world just to get tests to pass. If it doesn't work in the test environment, it won't work in production.

This document outlines the testing architecture, core principles, and how to write and run tests across the g8e stack.

## Core Engineering Principles

- **Hermetic Execution** — Each component runs tests inside a dedicated test-runner container (`g8ee-test-runner`, `g8ed-test-runner`, `g8eo-test-runner`). Source code is volume-mounted, meaning local development and CI execution are perfectly identical.
- **Real Infrastructure** — All testing must occur against real services and real inter-component communications. This means using a real `CacheAsideService` with a real `g8es` backend, real pub/sub over WebSockets, and real network stacks.
- **The "No Mocks" Policy** — We strictly prohibit mocking internal services, database clients, or LLM providers. If a scenario is extremely difficult to test without a mock, you must define a shared test fixture in `shared/test-fixtures` and justify its necessity. Integration tests must use real services.
- **Real LLM Calls** — AI tests use real provider API calls. No `MagicMock`, `AsyncMock`, or HTTP interception on LLM clients under any circumstances.
- **Contract Enforcement** — We use AST-scanning contract tests to ensure our source-of-truth constants (events, statuses, JSON schemas) perfectly match the application code across Go, Python, and Node.js. A raw string literal where a constant should be is an instant build failure.

## AI Benchmarks & Evaluations

Evaluating non-deterministic AI models requires a multi-layered approach. g8ee includes both deterministic benchmarks and subjective evaluation suites.

### Deterministic Benchmarks

We grade the AI's **tool call payloads** against strict boolean criteria. No LLM is involved in grading. The `BenchmarkJudge` uses regex matching on the actual command arguments for a reproducible pass/fail metric.

- **Binary pass/fail**: No partial credit. All payload matchers must pass for a scenario to pass.
- **Payload grading**: Grades the actual `TOOL_CALL` arguments (e.g., the `command` field), not the text reasoning.
- **Tribunal delta tracking**: Tracks whether the internal "Tribunal" pipeline improved the primary AI's command before execution.
- **Aggregate percentage**: The true industry metric (`passed_scenarios / total_scenarios`).

```bash
# Run all AI benchmarks
./g8e login --api-key <key>
./g8e test g8ee -p <provider> -k <key> -m <primary-model> -a <assistant-model> -- -m agent_benchmark
```

### Subjective Evaluations (LLM-as-a-Judge)

For reasoning and concept application, we use an "LLM-as-a-Judge" pattern. The `EvalJudge` uses the platform's Primary Model to score the Assistant Model.

- **Deterministic thresholds**: `passed` is computed strictly from `score >= 3`. The LLM provides the score, the system determines the pass/fail.
- **Error separation**: System failures (invalid JSON, missing fields) raise an `EvalJudgeError`. A low score is a valid evaluation; a system error is a test failure.

```bash
./g8e login --api-key <key>
./g8e test g8ee -p <provider> -k <key> -m <primary-model> -a <assistant-model> -- -m agent_eval
```

## Running Tests

All tests are orchestrated via the `./g8e` CLI, which routes each component to its dedicated test-runner container. **Never call `go test`, `pytest`, or `vitest` directly on your host machine.**

| Command | Container | Framework |
|---------|-----------|-----------|
| `./g8e test g8ee` | `g8ee-test-runner` | pytest + pyright |
| `./g8e test g8ed` | `g8ed-test-runner` | vitest |
| `./g8e test g8eo` | `g8eo-test-runner` | go test |

```bash
# Start the platform infrastructure first
./g8e platform start

# Authenticate once
./g8e login --api-key <key>

# Run a specific component
./g8e test g8ee

# Run with coverage
./g8e test g8eo --coverage

# Run g8ee with strict pyright type checking
./g8e test g8ee --pyright

# Run g8ee with ruff lint check
./g8e test g8ee --ruff

# Run g8ee with E2E operator lifecycle tests
./g8e test g8ee --e2e

# Run g8ee tests in parallel
./g8e test g8ee -j auto
```

---

## Component Testing Strategies

Each component has specific testing patterns tailored to its language and ecosystem, while adhering to the global principles.

### Go (g8eo)

The Go operator enforces a strict separation between in-memory logic and wire-level integration.

- **Unit**: Pure in-memory, no I/O. Uses `MockG8esPubSubClient` for asserting pub/sub payloads.
- **Loopback**: Uses an in-process `PubSubBroker` (`loopbackFixture`) over real WebSockets. Validates the full dispatch/routing wire path without requiring a live g8es instance.
- **Integration**: Tagged with `//go:build integration`. Requires live g8es infrastructure. Exercises the full end-to-end stack, including real network, TLS, and broker behavior.
- **Contract**: AST scanners ensure Go structs exactly match our shared JSON models and constants.

**Rule:** Always use `t.TempDir()` for file operations, and always run tests with `-race`. Any goroutines spawned in tests must be stopped via `t.Cleanup()`.

### Node.js (g8ed)

The Node.js orchestration layer uses `vitest` for backend execution and `jsdom` for frontend rendering logic.

- **Integration via `getTestServices()`**: Tests requiring live services must use the global `getTestServices()` singleton. Never initialize core services manually in integration tests.
- **Collection Isolation**: Tests never write to the real `operators` or `users` collections. We use automated test collection overrides (`_test` suffix) managed by `TestCleanupHelper` to prevent state bleed.
- **SSE Testing**: Frontend Server-Sent Events are tested using a custom `MockSSEBrowser` that enforces W3C specs and simulates real-world network failures (broken pipes, slow connections, malformed frames).
- **Mocks (Exception)**: Unit tests for individual services or route handlers in `test/unit/` may use mocks for direct dependencies to isolate logic, provided they do not cross into other service domains without a mock. Integration tests must use real services.

### Python (g8ee)

The Python execution engine runs `pytest` inside the `g8ee-test-runner` container.

- **Pyright Integration**: Type checking is enforced at test time. Running `./g8e test g8ee --pyright` runs strict pyright checks before `pytest`, failing immediately on type errors.
- **Real APIs Only**: No patches or mocks on LLM clients. All tests use real provider API calls.
- **Web Search**: Tests requiring external capabilities (like Google Web Search) are isolated via markers and gracefully skip when credentials are not provided.

### Security Audits

The `g8ep` container includes a comprehensive suite of security scan scripts volume-mounted into `scripts/security/`. Tools are lazy-installed and execute against the running platform.

```bash
# Run full mTLS and configuration audit (testssl.sh, Nuclei, Trivy, nmap)
./g8e login --api-key <key>
./g8e security mtls-test
```

## Shared Test Fixtures

Shared test fixtures are stored in `shared/test-fixtures/` to ensure consistency and prevent drift across all g8e components.

### SSE Events (`sse-events.json`)

Canonical SSE event structures used by both g8ee (Python) and g8ed (Node.js) tests.

#### Usage in g8ee (Python)

```python
import json
from pathlib import Path

# Load shared fixtures
fixtures_path = Path(__file__).parent.parent / "shared" / "test-fixtures" / "sse-events.json"
with open(fixtures_path) as f:
    sse_events = json.load(f)

# Use in tests
text_chunk_event = sse_events["text_chunk_received"]
```

#### Usage in g8ed (Node.js)

```javascript
import fs from 'fs';
import path from 'path';

// Load shared fixtures
const fixturesPath = path.resolve(__dirname, '../../../shared/test-fixtures/sse-events.json');
const sseEvents = JSON.parse(fs.readFileSync(fixturesPath, 'utf8'));

// Use in tests
const textChunkEvent = sseEvents.text_chunk_received;
```

#### Fixture Structure

Each fixture includes:
- `type`: Event type constant
- `data`: Event payload with required routing fields (`investigation_id`, `case_id`, `web_session_id`)
- Realistic example values for testing

#### Contract Tests

Both g8ee and g8ed include contract tests that verify:
1. Events emitted match the shared fixture structure
2. Required routing fields are present
3. Event types match constants in `shared/constants/events.json`

#### Maintenance

When adding new SSE events:
1. Add the event structure to `sse-events.json`
2. Update any relevant contract tests
3. Document the event in component-specific testing guides

## Continuous Integration

Our GitHub workflows enforce these constraints on every commit to `main`:
- Full multi-architecture container builds.
- Matrix execution of `g8ee`, `g8ed`, and `g8eo` tests against running infrastructure.
- Real AI integration tests using live API keys.
- Contract enforcement and pyright strict type checking.
