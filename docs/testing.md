# g8e Testing Guide

Internal testing reference for g8e engineers. This platform is **source-available** and designed to be a **testing environment and production environment at the same time**.

## Core Infrastructure Principles

- **Everything is local** — everything needed to test the platform is contained within the repository and the local environment.
- **Real communications only** — all testing must occur during real services and real inter-component communications. This means using a real `CacheAsideService` with a real `g8es`, real pub/sub, and real network stacks.
- **Mocks are prohibited** — never mock internal services, database clients, or LLM providers. If a scenario is extremely difficult to test without a mock, you must define a shared test fixture in `@/home/bob/g8e/shared/test-fixtures:1-5` and justify its necessity as being absolute.
- **Hermetic execution** — all tests run inside the **g8ep** container, ensuring consistency across local development and CI.

### AI Accuracy Evaluations (Evals)

g8ee includes a dedicated evaluation suite for measuring AI agent accuracy using the "LLM-as-a-Judge" pattern.

**Location:** `components/g8ee/tests/integration/evals/`

- `gold_set.json` — Dataset of scenarios with expected behavior, required concepts, and tool constraints.
- `test_agent_accuracy.py` — Parameterized test that runs scenarios and grades them using the `EvalJudge`.
- `app/services/ai/eval_judge.py` — The judge service that uses the platform's **Primary Model** to score the **Assistant Model**.

**Judge Design:**

- **Deterministic pass/fail** — `passed` is computed from `score >= 3` (the `PASSING_THRESHOLD` constant), never left to LLM judgment.
- **Score validation** — score is validated to 1-5 range; out-of-range responses raise `EvalJudgeError`.
- **Error separation** — system failures (LLM unreachable, invalid JSON, missing fields) raise `EvalJudgeError`. A low score is a valid evaluation; a system error is not.
- **Built-in retry** — transient LLM failures (429, 503, rate limits) are retried with exponential backoff (3 attempts, 2s initial delay, 2x multiplier).
- **Constructor** — `EvalJudge(provider, model)` takes the abstract `LLMProvider` from `app.llm.provider` and an explicit model name string.

**Running Evaluations:**

```bash
# Run all accuracy evaluations (requires LLM credentials)
./g8e test g8ee -p <provider> -k <key> -m <primary-model> -a <assistant-model> -- -m agent_eval
```

The `agent_eval` marker isolates these high-latency, high-cost tests from the regular integration suite. Every scenario is graded on a 1-5 scale, and the test fails if `grade.passed` is `False` (score < 3).

### Agent Benchmarks (Industry-Standard)

Alongside the subjective EvalJudge, g8ee includes a deterministic benchmark suite that grades the agent's **tool call payloads** against strict boolean criteria. No LLM is involved in grading -- the `BenchmarkJudge` uses regex matching on the actual command arguments for a reproducible pass/fail metric.

**Location:** `components/g8ee/tests/integration/evals/`

- `benchmark_gold_set.json` — Complex execution scenarios with `expected_tool` and `expected_payload` (regex matchers).
- `test_agent_benchmark.py` — Parameterized test that runs scenarios through the full pipeline and grades tool call payloads.
- `app/services/ai/benchmark_judge.py` — Deterministic judge: regex matching on tool call args, binary pass/fail, Tribunal delta tracking.

**Benchmark Design:**

- **Binary pass/fail** — No partial credit. All payload matchers must pass for a scenario to pass.
- **Payload grading** — Grades the actual `TOOL_CALL` arguments (e.g., the `command` field of `run_commands_with_operator`), not the text reasoning.
- **Tribunal delta** — Tracks whether the Tribunal improved the Primary Agent's command by comparing pre-Tribunal and post-Tribunal payloads against the matchers.
- **Aggregate percentage** — `passed_scenarios / total_scenarios` — the "real" industry metric.
- **Scenario categories** — `multi_step_execution`, `flag_precision`, `pipe_chain`, `security_refusal`.

**Gold Set Format:**

Each benchmark scenario specifies:
- `expected_tool` — the tool name the agent must call (e.g., `run_commands_with_operator`)
- `expected_payload` — list of `{field, pattern, description}` regex matchers
- `forbidden_tools` — tools the agent must not call (for refusal scenarios)
- `category` — for category-level reporting in the summary

**Running Benchmarks:**

```bash
# Run all agent benchmarks (requires LLM credentials + running platform)
./g8e test g8ee -p <provider> -k <key> -m <primary-model> -a <assistant-model> -- -m agent_benchmark
```

The `agent_benchmark` marker isolates these from the regular suite. Results are printed as a summary with per-category breakdowns, aggregate pass rate, and Tribunal delta statistics.

### Web Search Configuration

g8ee includes integration tests for web search capabilities. These tests are skipped by default unless web search credentials are provided.

**Running Web Search Tests:**

```bash
# Run web search integration tests (requires GCP/Google Search credentials)
./g8e test g8ee --web-search-project-id <project-id> \
               --web-search-engine-id <engine-id> \
               --web-search-api-key <api-key> \
               -- tests/integration/test_tool_search_web_integration.py
```

The `requires_web_search` marker isolates these tests. If any of the three required flags are missing, the tests will be automatically skipped with a descriptive message.

---

## Shared Principles

All tests run inside the **g8ep** container — a hermetic sidecar that bundles Python 3.12 (system, used to run `run_tests.sh` tooling), a Python venv with g8ee's dependencies pre-installed, Node.js 22, and Go 1.24.1 with all dependencies pre-installed. Source code is volume-mounted, so no rebuild is needed for code changes.

These rules apply to every component and test type.

- **Real infrastructure over mocks** — use real emulators for anything touching g8es or external systems; only mock external client boundaries if a shared fixture exists. All requests to infrastructure-backed services (g8es) must include the `X-Internal-Auth` header for authenticated routes.
- **Never mock LLM clients** — all AI tests use real provider API calls; no `MagicMock`, `AsyncMock`, or any patch on LLM clients under any circumstances.
- **Never mock internal services** — mock only external clients (HTTP, OAuth, external DB clients) when absolutely required by a shared fixture.
- **Exception: g8ed Unit Tests** — Unit tests for individual services or route handlers in g8ed may use mocks for their direct dependencies to isolate logic, provided they do not cross into other service domains without a mock. This applies to `test/unit/` only. Integration tests (`test/integration/`) must still use real services via `getTestServices()`.
- **Use constants, never raw strings** — always use enum/constant definitions for status values, event types, and channel names; raw string literals are a build failure in g8eo
- **Always clean up** — every test that writes state must clean it up; use provided cleanup helpers
- **Public API Access in Tests** — Tests should avoid accessing private-ish attributes (prefixed with `_`) of services. If a service dependency needs to be accessed in a test, it should be exposed via a public `@property` or getter.
- **No emojis** — never in application code, test code, log messages, or error strings

---

## Running Tests

All tests are executed via the `./g8e` CLI. Never call `go test`, `pytest`, `npx vitest`, or `vitest run` directly.

```bash
# All components
./g8e test

# Individual components
./g8e test g8ee
./g8e test g8ed
./g8e test g8eo

# With coverage
./g8e test g8ee --coverage
./g8e test g8ed --coverage
./g8e test g8eo --coverage

# g8ee — with pyright strict gate (required before service code changes)
./g8e test g8ee --pyright
```

The `--pyright` flag runs `python -m pyright --project pyrightconfig.services.json` against `app/services/` before pytest. Any type error in service code blocks the run. Plain `./g8e test g8ee` skips pyright — use `--pyright` when modifying anything under `app/services/`.

Infrastructure must be running before test execution:

```bash
./g8e platform start
```

---

## g8e node — Test Execution Environment

**Location:** `components/g8ep/`

- `Dockerfile` — container definition; base `ubuntu:24.04`
- `scripts/entrypoint.sh` — starts supervisord (operator binary must be built separately via `./g8e operator build`)
- `scripts/testing/run_tests.sh` — test runner invoked via `docker exec`
- `scripts/security/` — security scan scripts (volume-mounted; tools and dependencies lazy-installed at runtime)

### How tests run

`./g8e test` calls `docker exec g8ep /app/scripts/testing/run_tests.sh <args>`. The runner installs the private CA cert into the container system trust store so all tools trust internal `g8e.local` services.

| Argument | Runner | Notes |
|----------|--------|-------|
| `g8ee` | pytest | `pytest -v [--cov --cov-report=term-missing --cov-report=html --cov-report=json]` |
| `g8ed` | vitest | `npx vitest run --config ./vitest.config.js` or `npx vitest run --config ./vitest.config.js --coverage` |
| 'g8eo' | gotestsum | `gotestsum --format dots-v2 -- -race -timeout 180s ./...` |

### Rebuild triggers

Source changes do not require a rebuild. Only rebuild when:

| Changed | Action |
|---------|--------|
| `components/g8ep/Dockerfile` | `./g8e platform rebuild g8ep` |
| `components/g8ee/requirements.txt` | `./g8e platform rebuild g8ep` |
| `components/g8ed/package*.json` | `./g8e platform rebuild g8ep` |
| `components/g8ed/vitest.config.js` | `./g8e platform rebuild g8ep` |
| `components/g8eo/go.mod` / `go.sum` / `vendor/` | `./g8e platform rebuild g8ep` |

### Security scripts

All scripts live in `components/g8ep/scripts/security/` and are accessible inside the container at `/app/components/g8ep/scripts/security/`.

| Script | Purpose |
|--------|---------|
| `run-full-audit.sh` | Orchestrates testssl.sh, Nuclei, nmap, and HTTP header checks against a target |
| `scan-tls.sh` | TLS/SSL configuration audit via testssl.sh |
| `scan-nuclei.sh` | Template-based web vulnerability scan via Nuclei |
| `scan-containers.sh` | Container image CVE scan via Trivy |
| `scan-dependencies.sh` | Dependency CVE scan via Grype |
| `fetch-public-grades.sh` | Queries SSL Labs, Mozilla Observatory, SecurityHeaders.com |

Security scanning tools (Nuclei, testssl.sh, Trivy, Grype) are lazy-installed at first use by `install-scan-tools.sh`. The scan scripts require network utilities (`wget`, `unzip`, `nmap`, etc.) that are not part of the base image; install them manually inside the container before running scans.

```bash
# Platform mTLS connectivity tests
./g8e security mtls-test

# Run a specific scan
./g8e security scan-licenses
```

Scan output is written to `components/g8ep/reports/` (gitignored except `.gitkeep`).

---

## CI/CD — GitHub Workflows

Two workflows in `.github/workflows/`:

| Workflow | Trigger | Purpose |
|----------|---------|---------|
| `ci.yml` | push/PR to `main` | Run all tests (matrix: g8ee, g8ed, g8eo), build images |
| `g8ep-image.yml` | push to `main` (path-filtered) | Build and push g8ep image to GHCR |

**`ci.yml` test job:**
- Runs `./g8e platform build` (builds all images), starts `g8ep`.
- Runs `./g8e test -p gemini -k <key> -m <model> -a <assistant>` with LLM flags for `ai_integration` tests.
- Runs `./g8e platform stop` on completion (`if: always()`).
- A separate `build-images` job (requires `test` to pass) builds operator binaries for all architectures and all Docker images (`docker compose build g8ee g8ed g8es`) as a post-test smoke check; images are not pushed.

Required secrets: `GEMINI_API_KEY`, `VERTEX_SEARCH_PROJECT_ID`, `VERTEX_ENGINE_ID`, `VERTEX_SEARCH_API_KEY`, `GITHUB_TOKEN` (auto-provided). Vars: `LLM_MODEL`, `LLM_ASSISTANT_MODEL`.

**`g8ep-image.yml`** triggers on changes to: `components/g8ep/Dockerfile`, `components/g8ep/scripts/**`, `components/g8ee/requirements.txt`, `components/g8ed/package*.json`, `components/g8eo/go.mod`/`go.sum`/`vendor/**`. Also supports `workflow_dispatch`. Pushes to `ghcr.io/g8e-ai/g8e` with `latest` and `sha` tags.

---

## Shared Test Resources

The canonical `shared/constants/` and `shared/models/` definitions and per-component import patterns are documented in [developer.md — Shared Constants and Models](developer.md#shared-constants-and-models).

The core rule for tests: never duplicate or reimplement shared constant values — import them through the component's own constants layer.

---

## g8eo — Go

**Location:** `components/g8eo/`

### Test types

- **Unit** — pure in-memory, no I/O; no build tag; must always run; use `MockG8esPubSubClient` for all pub/sub assertions
- **Loopback** — in-process `PubSubBroker` via `loopbackFixture`; no build tag; no external infrastructure; exercises the real `G8esPubSubClient` + broker wire path; includes dispatch tests for commands and file operations
- **Integration** — requires live g8es; tagged `//go:build integration`; infrastructure must be running; run with `-tags integration`
- **Contract** — `contracts/` contains three enforcement tests: `constants_enforcement_test.go` AST-scans application source for raw string literals that match known constants; `shared_constants_test.go` verifies Go constants in `constants/events.go` and `constants/status.go` exactly match `shared/constants/*.json`; `shared_wire_models_test.go` verifies Go struct json tags in `models/` and MCP types in `services/mcp/` exactly match `shared/models/wire/*.json`; any of these fail the build on violation

### Unit vs integration split

Unit and integration tests are separated by **build tags**, not by file naming alone:

- Unit test files have **no build tag** — compiled and run by default with `./g8e test g8eo`
- Integration test files carry `//go:build integration` as the first non-comment, non-blank line after the copyright header — excluded from default runs, compiled only with `-tags integration`

File naming convention mirrors the tag:
- `*_integration_test.go` — always carries `//go:build integration`
- `*_test.go` without `_integration` — always unit tests, no build tag

To run integration tests (requires live g8es):

```bash
./g8e test g8eo -- -tags integration ./...
```

### Rules

See [developer.md — g8eo](developer.md#g8eo-go) for constants, models, service, and error handling rules that apply in test code as much as application code.

- Use `constants.EventType*` and `constants.Status.*` — never raw strings; the contract test will fail the build
- Never mock `ExecutionService` or `FileEditService` — construct with real config and a real `t.TempDir()`
- Use `t.TempDir()` for all file operations — never write to fixed paths
- All tests run with `-race` — goroutines spawned in tests must be stopped via `t.Cleanup()`
- Unit tests must not dial any external service — use `MockG8esPubSubClient` for logic-level assertions or `loopbackFixture` (in-process `PubSubBroker` via `httptest`) for wire-path assertions; neither requires any running infrastructure
- Integration tests: add `//go:build integration` at the top of the file; call `testutil.TestPubSubAvailable(t)` as the first statement in any test that uses pub/sub

### `testutil/` — shared test helpers

**`testutil/helpers.go`:**

- `testutil.NewTestConfig(t)` — unique `config.Config` per call, wired to test g8es; `OperatorID` and `OperatorSessionId` are unique per call (derived from `t.Name()` + a process-local `atomic.Int64` counter); parallel tests never share the same identity or pub/sub channels
- `testutil.NewTestLogger()` — silent logger (`io.Discard`)
- `testutil.NewVerboseTestLogger(t)` — bridges slog to `t.Log`; output shown only on failure
- `testutil.GetTestG8esDirectURL()` — reads `G8E_OPERATOR_PUBSUB_URL`, falls back to `wss://g8e.local:443`
- `testutil.TempFile(t, path)` — registers `os.Remove` cleanup for files outside `t.TempDir()`

**`testutil/pubsub.go`** — for integration tests (`//go:build integration`) that observe or inject pub/sub traffic against real g8es:

- `testutil.TestPubSubAvailable(t)` — calls `t.Fatalf` if the g8ed pub/sub gateway is unreachable; call as the first statement in any integration test that uses pub/sub
- `testutil.SubscribeToChannel(t, baseURL, channel)` — returns `<-chan []byte`, auto-closed on test end
- `testutil.PublishTestMessage(t, baseURL, channel, payload)`
- `testutil.WaitForMessage(t, ch, timeout)`
- `testutil.AssertMessageReceived(t, ch, timeout, substring)`
- `testutil.CreateTestChannel(t, prefix)` — returns `prefix:test:TestName:nanoseconds`

**`services/pubsub/pubsub_integration_helpers_test.go`** — `NewTestPubSubClient(t)`: creates a real `G8esPubSubClient` connected to the test g8es instance; tagged `//go:build integration`; only available to integration test files.

**`services/pubsub/g8es_pubsub_mock.go` — `MockG8esPubSubClient`:** implements `PubSubClient` for unit tests; construct with `NewMockG8esPubSubClient()`; thread-safe; supports `InjectMessage`, `PublishedCount()`, `Published()`, `LastPublished()`, `Reset()`. Use this when testing logic that depends on pub/sub without a real broker (e.g. dispatcher routing, results service serialization).

**`services/pubsub/g8es_pubsub_loopback_test.go` — loopback test framework:** tests that require the full wire path (subscribe/publish/fan-out through the broker) use a `loopbackFixture` that serves an in-process `PubSubBroker` via `httptest.NewServer` and connects `G8esPubSubClient` over plain `ws://`. No external dependencies, no build tags. Use this pattern when you need to assert that a message actually travels through the broker — e.g. `PubSubCommandService.listenForCommands` dispatch, `PubSubResultsService.Publish*` channel routing, command and file ops end-to-end. The fixture helper is private to the `pubsub` package test files:

```go
f := newLoopbackFixture(t)
client := f.newClient(t)   // G8esPubSubClient → in-process broker
sub, err := client.Subscribe(ctx, channel)
require.NoError(t, err)
// publish via broker directly, or via a second client
f.broker.Publish(channel, data)
msg := drainOne(t, sub)   // 3-second timeout helper
```

**`subscribeAndWait` — deterministic subscription registration:** when a test calls `Subscribe` then immediately publishes (or starts a service that publishes on startup), there is a race: `G8esPubSubClient.Subscribe` sends the subscribe message and returns before the broker has processed and registered the subscriber. Use `subscribeAndWait` to eliminate this race — it dials a raw WebSocket, sends subscribe, and blocks until the broker sends the `type: subscribed` ACK:

```go
sub, err := client.Subscribe(ctx, channel)
require.NoError(t, err)
f.subscribeAndWait(t, channel)  // blocks until broker confirms registration
// now safe to publish — the client's subscription is guaranteed live
f.broker.Publish(channel, data)
```

Note: `subscribeAndWait` confirms that *at least one* subscriber is registered on the channel (its own probe connection). This is sufficient to synchronize tests where the `client.Subscribe` connection races against an immediate publish, but only after `G8esPubSubClient.Subscribe` itself is fixed to wait for its own ACK. Until that fix lands, `subscribeAndWait` is a test-side workaround, not a production-side fix.

**Loopback as the foundation for command and file ops unit tests:** the loopback framework is designed to be extended. Once the `Subscribe` ACK race is resolved in `G8esPubSubClient`, the following test patterns become available without any infrastructure:

- Inject `g8e.v1.operator.command.requested` on the cmd channel → assert `g8e.v1.operator.command.completed` on results channel
- Inject a file edit command → assert results channel receives the file diff event
- Inject cancel command → assert the running command is terminated and a result published
- Multi-subscriber fan-out → same result delivered to multiple `G8esPubSubClient` instances

These tests exercise the full dispatch → service → publish path that mock-based tests cannot reach.

The three layers are complementary:
- **Unit (`MockG8esPubSubClient`)** — pure logic, dispatch routing, payload serialization; no broker, no goroutines
- **Loopback (`loopbackFixture`)** — real `G8esPubSubClient` + in-process broker; proves the full subscribe/publish/fan-out wire path without any external infrastructure
- **Integration (`//go:build integration`)** — live g8es; proves the stack end-to-end including real network, TLS, and broker behavior

For the heartbeat scheduler specifically, all three layers cover `--heartbeat-interval` flag plumbing: unit tests use `MockG8esPubSubClient` and `require.Eventually` to verify tick count and payload fields; loopback tests (`heartbeat_interval_loopback_test.go`) use `loopbackFixture` + `drainOne`/`drainNone`/`drainUntilQuiet` to verify the full wire path at short intervals; integration tests (`pubsub_commands_integration_test.go`) assert against live g8es.

### Pub/sub channel naming

Always derive channel names from `cfg.OperatorID` / `cfg.OperatorSessionId` using the constants helpers — never hardcode. The canonical channel definitions are in [components/g8es.md — Channel Naming Convention](components/g8es.md#channel-naming-convention).

### Test Coverage Status

**Well-covered:**
- AI tool registration and execution (including `fetch_file_history` and `fetch_file_diff`)
- Chat pipeline and streaming architecture
- Operator command execution and approval flows
- Investigation context assembly and memory management
- LLM provider abstraction and grounding
- SSE event delivery and lifecycle management
- Web search integration and citation processing
- Pub/sub communication patterns
- Security validation and threat detection
- Sentinel, Vault, Execution, File operations, Local storage
- Pub/sub command handling, Listen HTTP, PubSub broker, Listen DB
- Auth (TLS pinning, device link, fingerprint), System utilities
- OpenClaw node service, G8esPubSubClient, TLS error classification
- Pub/sub loopback (subscribe/publish/fan-out/command dispatch/results through in-process broker)
- Automatic heartbeats

**Known gaps:**
- `/ws/pubsub` WebSocket upgrade path inside `listen_http.go` (covered indirectly via `listen_pubsub_test.go`)
- `listen_service.go` — no tests for top-level startup/shutdown orchestration
- `device_auth.go` — HTTP polling flow against g8ed untested
- `sqliteutil/` — no direct tests; exercised indirectly

---

## g8ed — Node.js

**Location:** `components/g8ed/`

### Test types

- **Unit** — isolated logic, mocked dependencies; no g8es, no network
- **Integration** — real g8es emulators; use `getTestServices()`, never initialize services manually
- **Security** — cross-cutting security audits in `test/security/`
- **Contract** — wire-protocol and route enforcement in `test/contracts/`
- **Frontend** — jsdom environment; pure DOM rendering logic in `public/js/components/`

### Test layout

```
test/
├── setup.js                      # Per-test-file setup entry point
├── fixtures/                     # Shared test data builders
├── helpers/                      # Cleanup, service access, SSE tools
├── mocks/                        # Shared mock factories
├── unit/                         # Pure unit tests
├── integration/                  # Tests requiring live g8es
├── security/                     # Cross-cutting security audits
└── contracts/                    # Wire-protocol and constants contract tests
```

### Import aliases

| Alias | Resolves to | Use for |
|-------|-------------|---------|
| `@g8ed` | `components/g8ed/` | App source |
| `@test` | `components/g8ed/test/` | Test support |
| `@shared` | `shared/` | Workspace constants/models — ESM imports only; use `path.resolve` with `createRequire` |

### Service access

`getTestServices()` returns a singleton backed by `globalThis` — services are initialized once for the entire suite:

```javascript
const {
    webSessionService, operatorSessionService, bindingService,
    dbClient, kvClient, cacheAsideService, OperatorDataService, apiKeyService,
    userService, userModel, healthCheckService
} = await getTestServices();
```

- `getTestKVClient()` — shared `KVClient`; do not call `terminate()` on it
- `getTestG8esPubSubClient()` — shared `G8esPubSubClient`; call `.duplicate()` for per-test subscribers that need an independent connection
- **Never mock internal services** — always use real services from `getTestServices()`

`test-services.js` generates a temporary self-signed CA cert and seeds `SSL_DIR` / `CERT_DIR` env vars into the environment, and sets `OperatorDataService.collectionName = 'operators_test'` (appends `_test` to whatever the service's collectionName is) at initialization time. `userModel` is an alias for `userService`. `cacheAsideService` is the shared `CacheAsideService` instance.

### Collection isolation

Both `OperatorDataService` and `userService` expose a `collectionName` property. Tests must never write to the real `operators` or `users` collections. There are two patterns:

**Global override (unit/middleware tests via `getTestServices()`):** `test-services.js` sets `OperatorDataService.collectionName = 'operators_test'` once at initialization. All `OperatorDataService.*` calls route there automatically. `userService` has no global override — use the per-test pattern for any test that writes users.

**Per-test override (integration tests that need a unique collection per suite):** Tests set their own collection name in `beforeEach` and restore it in `afterEach`:

```javascript
const TEST_OPERATORS_COLLECTION = `${Collections.OPERATORS}_test_my_suite`;
const TEST_USERS_COLLECTION = `${Collections.USERS}_test_my_suite`;

beforeEach(() => {
    OperatorDataService.collectionName = TEST_OPERATORS_COLLECTION;
    userService.collectionName = TEST_USERS_COLLECTION;
    cleanup = new TestCleanupHelper(kvClient, cacheAsideService, {
        operatorsCollection: TEST_OPERATORS_COLLECTION,
        usersCollection: TEST_USERS_COLLECTION,
    });
});

afterEach(async () => {
    OperatorDataService.collectionName = Collections.OPERATORS;
    userService.collectionName = Collections.USERS;
    await cleanup.cleanup();
});
```

**Rule:** Any direct `dbClient.getDocument` / `dbClient.queryDocuments` call on the operators or users collection must use `OperatorDataService.collectionName` / `userService.collectionName` — never the hardcoded `Collections.*` constant. Service methods route through `this.collectionName` automatically and are unaffected.

```javascript
// CORRECT
const opDoc = await dbClient.getDocument(OperatorDataService.collectionName, operatorId);
const userDoc = await dbClient.getDocument(userService.collectionName, userId);
const kvKey = KVKey.doc(userService.collectionName, userId);

// WRONG — reads from the real collection, misses test data
const opDoc = await dbClient.getDocument(Collections.OPERATORS, operatorId);
const userDoc = await dbClient.getDocument(Collections.USERS, userId);
```

### `TestCleanupHelper`

Manages resource cleanup after each test. Initialize in `beforeEach`, call `cleanup.cleanup()` in `afterEach`.

```javascript
const cleanup = new TestCleanupHelper(kvClient, cacheAsideService);
// With non-default collections:
const cleanup = new TestCleanupHelper(kvClient, cacheAsideService, {
    operatorsCollection: TEST_OPERATORS_COLLECTION,
    usersCollection: TEST_USERS_COLLECTION,
});
```

KV tracking methods: `trackG8esKey(key)`, `trackKVPattern(pattern)`, `trackLoginAudit(userId)` — required after any failed login attempt to prevent stale rate-limit keys causing 429s in subsequent tests; `trackAccountLock(userId)`.

Document tracking methods: `trackUser(userId)`, `trackWebSession(sessionId)`, `trackOperatorSession(sessionId)`, `trackOperator(operatorId)`, `trackDBDoc(collection, docId)`, `trackOrganization(orgId)`, `trackConsoleAudit(auditId)`, `trackOperatorUsage(usageId)`, `trackSessionAuditPattern(pattern)`.

Convenience methods: `createTrackedWebSession(sessionData, requestContext)`, `createTrackedOperatorSession(sessionData, requestContext)`, `generateOperatorId(userId)` — returns `{ opId, operatorSessionId, webSessionId }` with all related KV and DB keys pre-tracked for cleanup.

Cleanup removes all tracked KV keys, matched KV patterns, and DB documents. Resources are always cleaned up even if assertions fail — place `cleanup.cleanup()` in `afterEach`, not `afterAll`.

### Fixtures (`test/fixtures/`)

All test data files live here. Import via the `@test/fixtures/` alias. Never duplicate fixture data across test files.

| File | What it provides |
|------|------------------|
| `base.fixture.js` | `now()` helper returning a consistent timestamp for the current test run |
| `operators.fixture.js` | `mockOperators` — typed `OperatorDocument` instances: `unclaimed`, `claimed`, `stale`, `active`, `differentUser`; each exposes `.forDB()` for wire-format output |
| `users.fixture.js` | `mockUsers` — typed `UserDocument` instances: `primary`, `secondary`, `basic`, `expired`, `admin`; IDs are unique per module import to prevent cross-test pollution; `makeUserDoc(overrides)` factory |
| `apikeys.fixture.js` | `mockApiKeys` — keyed entries: `validOperator`, `validOperator2`, `inactive`, `expired`, `noOperator`, `userDownload`, `invalidFormat` |
| `heartbeat.fixture.js` | `mockHeartbeatSnapshot` — typed `HeartbeatSnapshot` instance |
| `attachment.fixture.js` | `mockAttachments` — `record`, `record2`, `validInput`, `validInputPdf`, `invalidContentType` |
| `services.fixture.js` | `createOperatorServiceMock()`, `createOperatorSessionServiceMock()`, `createUserServiceMock()`, `createSSEServiceMock()`, `createPasskeyUserServiceMock(overrides)` — fresh `vi.fn()` mocks per call, pre-wired with fixture return values; `createOperatorServiceMock()` exposes `collectionName: 'operators'` and stubs `getOperator`, `claimOperatorSlot`, `updateOperatorForReconnection`, `activateOperator`, `_createOperatorSlot`; `createSSEServiceMock()` stubs `publishEvent`, `registerConnection`, `unregisterConnection`, `onConnectionEstablished` |
| `templates.fixture.js` | `TEMPLATE_FIXTURES` — Handlebars template strings used by frontend rendering unit tests |

### Mocks (`test/mocks/`)

**`mock-sse-browser.js`** — `MockSSEBrowser` and `MockSSEResponse`; simulates W3C `EventType` behavior without HTTP. Use `MockSSEBrowser.createPair()` to get a `{ response, client }` pair for route handler tests. Use `createSSETestHarness()` for a fully wired test setup with helper methods. `createMockRequest(overrides)` builds a minimal Express-compatible request object. Promise-based assertion methods: `waitForEvent`, `collectEvents`, `assertEventReceived`, `assertEventCount` — never poll.

**`mock-browser-env.js`** — Browser environment stubs for jsdom tests.
- `MockEventBus` — mirrors frontend event bus; tracks all `emit`/`on`/`off` calls
- `MockAuthState` — stub for auth state management
- `MockServiceClient` — HTTP client stub; tracks requests via `getRequestLog()`; assert calls here, never mock the service client
- `MockDOMElement` — minimal DOM element stub

**`kv.mock.js`** — `KVMock` — in-memory simulation of `KVClient`. Mirrors the real client contract: `set()` requires string values; compound structures are stored as JSON strings; `get`/`exists` enforce TTL expiry. All methods are `vi.fn()` spies. Use `createKVMock()` factory for a fresh instance.

**`cache-aside.mock.js`** — `MockCacheAsideService` — unit-test stand-in for `CacheAsideService`. Backed by in-memory DB and KV stores. All public methods are `vi.fn()` spies (write-through, read-through, delete-through). Use `createMockCacheAside()` factory.

### Contracts (`test/contracts/`)

| File | What it enforces |
|------|------------------|
| `constants-enforcement.test.js` | Scans `services/`, `routes/`, `middleware/`, `models/`, `utils/` for raw string literals that should use constants; auto-extracts known values from the constants source files; excludes test files, fixtures, frontend `public/`, views, and import statements |
| `routes-enforcement.test.js` | Scans all route files in `routes/` for structural contract violations (e.g., required middleware, response patterns) |
| `shared-definitions.test.js` | Every g8ed constant loaded from the `shared/` JSON files matches the value in the shared JSON source of truth — catches key renames, missing keys, or value drift |
| `shared-loader.test.js` | `constants/shared.js` resolves all shared JSON files (`events.json`, `status.json`, `senders.json`, `collections.json`, `kv_keys.json`, `channels.json`, `pubsub.json`, `agents.json`) correctly at the path `../../../shared/constants` relative to `constants/`; `_PUBSUB` and `_AGENTS` exports are defined |
| `shared-pubsub-constants.test.js` | Contract test: g8ed `PubSubAction` and `PubSubMessageType` values must exactly match `shared/constants/pubsub.json` wire protocol definitions — bidirectional coverage (every JSON action/event_type has a JS constant, count parity between JS and JSON) |
| `shared-agent-constants.test.js` | Contract test: g8ed `TriageComplexity`, `TriageConfidence`, `TriageIntent`, and `AgentMetadata` constants must exactly match `shared/constants/agents.json` canonical values — verifies triage enum values, agent metadata completeness (6 agents), and raw JSON value correctness |

### Async patterns

- Never use arbitrary timeouts — prefer direct seeding; use a bounded retry loop only when direct seeding is not possible
- Always close `EventType` connections in `afterEach` — unclosed connections leak
- Use 10s+ timeouts in SSE tests — CI is slower than local

### SSE testing

| Scenario | Tool |
|----------|------|
| Unit tests (no network) | `MockSSEBrowser` from `test/mocks/mock-sse-browser.js` |
| Integration tests (real HTTP) | `SSETestClient` from `test/helpers/sse-test-client.js` |
| Network-level testing | MSW handlers from `test/mocks/msw-sse-handlers.js` |
| Broken pipe scenarios | `MockSSEBrowser.simulateBrokenPipe()` |
| Connection timeout testing | `MockSSEBrowser.simulateConnectionTimeout()` |
| Testing route handlers | `createSSETestHarness()` from `test/mocks/mock-sse-browser.js` |
| Testing g8es pub/sub delivery | `SSETestClient` + real server |
| Mock event factories / flow simulation | `mockSSEEvents`, `simulateAIChatFlow`, `assertEventSequence` from `test/helpers/sse-event-helper.js` |
| Raw `EventType` utilities | `createManagedEventSource`, `waitForConnection`, `waitForClose`, `waitForEvent`, `collectEvents`, `closeAllEventSources`, `createAndConnectEventSource`, `assertEventStructure`, `createMockSSEResponse`, `simulateReconnection`, `measureEventLatency`, `createCookieHeader` from `test/unit/utils/sse-test-helpers.js` |
| Fake timers for debouncing | `vi.useFakeTimers()` in frontend tests |

**`MockSSEBrowser`** (`test/mocks/mock-sse-browser.js`) — Enhanced W3C-spec SSE wire format parser with network failure simulation. Supports `simulateBrokenPipe()`, `simulateConnectionTimeout()`, `simulateMalformedSSE()`, and `simulateSlowConnection()` for realistic network testing.

**`SSETestClient`** (`test/helpers/sse-test-client.js`) — real HTTP `EventType`. Used for integration tests against a live server. `close()` rejects any pending `waitForEvent` / `collectEvents` promises immediately — no hangs on teardown.

**MSW SSE Handlers** (`test/mocks/msw-sse-handlers.js`) — Network-level mocking using Mock Service Worker. Enables testing of actual HTTP request/response cycles, connection drops, malformed data, and timing scenarios. Provides `sseTestHelpers` for test control.

**Shared SSE Fixtures** (`shared/test-fixtures/sse-events.json`) — Canonical event structures shared across g8ee and g8ed to prevent interface drift. Used by contract tests to ensure both components emit/consume compatible event formats.

**`sse-event-helper.js`** — mock event factories (`mockSSEEvents.*`) covering all major SSE event types, `simulateAIChatFlow` for end-to-end AI chat flow scenarios, and `assertEventSequence` for ordered event assertion. Does not contain a stream parser — use `MockSSEBrowser` or `SSETestClient` for reception.

### Helpers (`test/helpers/`)

| File | What it provides |
|------|------------------|
| `test-services.js` | `getTestServices()`, `getTestKVClient()`, `getTestG8esPubSubClient()`, `cleanupTestServices()`, `initializeTestServices()` — singleton test service initialization |
| `test-cleanup.js` | `TestCleanupHelper` — resource cleanup for integration tests (see above) |
| `operator-factory.js` | `createActiveOperator(userId, email, opts, cleanup)` — seeds a fully-initialized operator in g8es (operator doc + system info + heartbeat snapshot) ready for bind/stop/unbind scenarios; uses `TestCleanupHelper` for tracking and `getTestServices()` for client access; uses `getCacheAsideService()` directly for heartbeat snapshot writes |
| `sse-test-client.js` | `SSETestClient` — real HTTP `EventType` for integration tests against a live server |
| `sse-event-helper.js` | `mockSSEEvents`, `simulateAIChatFlow`, `assertEventSequence` — mock event factories and flow simulation helpers |

### Frontend component tests

Run in jsdom — no real browser, no network. Use `MockAuthState` and `MockServiceClient` from `@test/mocks/mock-browser-env.js`. Assert HTTP calls via `serviceClient.getRequestLog()` — never mock the service client itself.

#### Chat rendering test files

| File | Coverage |
|------|----------|
| `test/unit/frontend/chat/chat-component.unit.test.js` | `ChatComponent` class — constructor initialization, `init` (auth subscription, copy button listeners), `showAIStopButton`/`hideAIStopButton`, `stopAIProcessing` (investigation check, HTTP request, approval event emission, state reset, cleanup, notifications), `render` (container reuse, template loading, DOM binding, ThinkingManager, AnchoredOperatorTerminal, CompactAttachmentsUI), `bindDOMEvents` (messagesContainer, aiStopBtn, SentinelModeManager, LlmModelManager, ScrollDelegation), `clearChat` (DOM cleanup, state reset, indicator cleanup), `destroy` (ThinkingManager destroy, container removal), `addSystemMessage` (notification service), `initCasesManager`, `attemptReconnect` (exponential backoff, max attempts, timer management), `_handleSSEConnectionClosed` (session validation, error display, reconnect trigger), `_handleSSEConnectionEstablished` (reconnection state reset, system message), `_handleLLMChatIterationFailed` (debounce cancellation, error display, state reset, content cleanup) |
| `test/unit/frontend/chat/chat-history.unit.test.js` | `ChatHistoryMixin` — `handleCaseSelected` (delegation to loadConversationHistoryFromData), `loadConversationHistoryFromData` (validation, parsing, error handling, notification service integration), `restoreConversationHistory` (early returns, auto scroll reset, thinking event filtering, user/AI/system message restoration, operator command grouping, direct execution restoration, approval flow restoration, execution deduplication), `restoreDirectCommand` (parameter mapping, default status, missing metadata handling), `restoreOperatorActivity` (message sorting, approval request/decision handling, command result restoration, container key selection, approval state tracking), `addRestoredMessage` (timestamp formatting, sender routing, anchoredTerminal delegation) |
| `test/unit/frontend/chat/chat-message-rendering.unit.test.js` | `handleAITextChunk` accumulation, `addMessage` sender routing |
| `test/unit/frontend/chat/chat-response-complete.test.js` | `handleResponseComplete` (streamed-content path, data.content fallback, pending citations, event bus routing for TEXT_COMPLETED and TEXT_TRUNCATED), `_finalizeInterTurnBubble`, investigation ID gating, indicator map cleanup on finalize, debounce cancellation on all finalize paths; `clearChat` indicator map cleanup; `restoreConversationHistory` orphaned-execution null-dereference regression |
| `test/unit/frontend/chat/terminal-output-rendering.unit.test.js` | `TerminalOutputMixin` DOM structure — `appendUserMessage`, `appendAIResponseChunk`, `appendAIResponse` (with/without citations), `appendSystemMessage`, `appendErrorMessage`, `appendThinkingContent` (dynamic title extraction from bold/heading markdown, title replacement on new chunks, toggle/collapse behavior), `completeThinkingEntry` (collapsed state, toggle indicator, title preservation), `_extractThinkingTitle` (bold, h1-h3, last-match, no-match), `applyCitations`, `showWaitingIndicator`, `hideWaitingIndicator` |
| `test/unit/frontend/chat/chat-sse-handlers.unit.test.js` | `handleChatError` (routing, state cleanup, investigation ID gating); `handleAITextChunk` edge cases (thinking safety-net, `data.content` fallback, no-text guard); `handleCitationsReady` (pending citation storage, deferred apply); `addRestoredMessage` (all three sender paths); `handleSearchWebIndicator`, `handleSearchWebCompleted`, `handleSearchWebFailed`, `handleNetworkPortCheckIndicator`, `handleNetworkPortCheckCompleted`, `handleNetworkPortCheckFailed` (event-driven indicator lifecycle, concurrent tracking, investigation ID gating) |
| `test/unit/frontend/chat/citations-handler.unit.test.js` | Citation handler lifecycle — pending citation storage, deferred application, finalization gating |
| `test/unit/frontend/models/frontend-models.unit.test.js` | Frontend model parse/validate discipline — `FrontendBaseModel` (required fields, type coercion, defaults, `forWire()` date serialization, absent fields), `FrontendIdentifiableModel` (id/timestamp inheritance), `InvestigationHistoryEntry` (backfill from context, sender-to-event-type mapping, display helpers), `InvestigationHistoryEntry` g8ee wire shape compat (parses `ConversationHistoryMessage` wire shape for all sender types, summary truncation, `parseConversationHistory` array handling), `InvestigationModel` (priority/severity coercion, history trail lifecycle, status updates), `InvestigationFactory`, wire boundary discipline |
| `test/unit/frontend/chat/llm-model-manager.unit.test.js` | LLM model manager — model selection, availability checks |
| `test/unit/frontend/chat/chat-auth.unit.test.js` | `ChatAuthMixin` — authentication state subscription, initialization waiting, user session setup, auth state change handling, UI updates based on auth status |
| `test/unit/frontend/chat/markdown-renderer.unit.test.js` | Markdown rendering — code blocks, inline formatting, sanitization |
| `test/unit/frontend/chat/message-renderer.unit.test.js` | Message rendering pipeline — sender routing, content assembly |
| `test/unit/frontend/chat/thinking-manager.unit.test.js` | Thinking content lifecycle — entry creation, accumulation, completion |
| `test/unit/frontend/setup-page.unit.test.js` | `SetupPage` — constructor initialization, `init` (button listeners, provider cards, API key inputs, reveal buttons, finish button, search provider, keyboard navigation), step navigation (`_goToStep`, `_updateNav`, `_validateStep`), status bar (`_showStatus`, `_clearStatus`), provider selection (`_selectProvider`, `_isProviderStepReady`), API key listeners and reveal buttons, summary rendering (`_renderSummary`, `_getSelectedModel`, `_getSelectedAssistantModel`), user settings collection (`_collectUserSettings` for all LLM providers and search providers), finish button flow (user registration, passkey creation/verification with navigator.credentials, API calls via window.serviceClient, error handling, redirect), keyboard navigation (Enter key in input fields) |

#### SSE connection manager test files

| File | Coverage |
|------|----------|
| `test/unit/frontend/sse/sse-connection-manager.unit.test.js` | `SSEConnectionManager.handleSSEEvent` — infrastructure event bypass (`PLATFORM_SSE_CONNECTION_ESTABLISHED`, `PLATFORM_SSE_KEEPALIVE_SENT`); missing/invalid `type` field drops; non-infrastructure events without `data` field dropped; correct `eventBus.emit(eventType, payload)` dispatch for every g8ee-emitted event type; payload fidelity (data field passed verbatim, not the top-level frame); multi-event accumulation. **EventSource regression**: `connect()` creates `EventSource` with correct URL and `withCredentials`, `onopen` sets `isConnected`/resets counters/emits `PLATFORM_SSE_CONNECTION_OPENED`, `onmessage` parses and routes through `handleSSEEvent`, reconnect closes previous `EventSource`, `onerror` clears `isConnected`; `isConnectionActive()` checks `readyState === EventSource.OPEN`; `disconnect()` closes and nulls `eventSource`, resets state; `getConnectionStatus()` returns `EventSource.OPEN`/`EventSource.CLOSED` readyState; `resetKeepaliveTimeout()` closes `EventSource` on timeout |

**Note:** SSE events correctly use `data` fields in their payload structure - this is distinct from HTTP response models, which should use explicit fields instead of generic `data` containers.

#### Operator panel test files

| File | Coverage |
|------|----------|
| `test/unit/frontend/operator/operator-deployment.unit.test.js` | `OperatorDeployment` — constructor (default values, onClose callback), `mount` (container clearing, template loading, content appending, container reference storage, _initializeg8eDeploy call), `_initializeg8eDeploy` (hostname setting from window.location, missing hostSpan guard, copy button click handler attachment, missing copyBtn guard, clipboard write with trimmed command, missing command element guard, success logging, copied class addition, icon change to check, timeout removal of copied class and icon restoration, clipboard error logging, no copied class on error), `destroy` (event listener removal, copyHandler nullification, container nullification, missing element guards, multiple destroy calls), `setUser` (no-op), integration (full lifecycle, remount after destroy, setTimeout behavior on destroy) |
| `test/unit/frontend/operator/operator-download-mixin.unit.test.js` | `OperatorDownloadMixin` — `_obfuscateApiKey` (null/empty/short/valid keys), `_bindDeployApiKey` (obfuscated display, dataset storage, visibility toggle, copy handler with explicit spy, missing element guard), `_bindDeviceLinkGeneration` (counter increment/decrement with bounds, successful generate populates curl command and token, count/TTL passed to API, error display on failure/network error, button re-enable after completion, copy handler binding with explicit spy, error clearing on retry), `_populateBinaryDownloadLinks` (click handler attachment, delegation to sub-binders, explicit spies for copyCurlCommand/handleOperatorDownload), `handleOperatorDownload` (missing API key alert, fetch failure alert, test environment skip with explicit _setTestEnvironment), `copyCurlCommand` (clipboard write, button UI state), `_isTestEnvironment` (explicit _setTestEnvironment configuration, fallback to window.__vitest__/jsdom detection), overlay management (showPlatformSelection, closeCurrentOverlay, closeAllOverlays) |
| `test/unit/frontend/operator/operator-bind-mixin.unit.test.js` | `BindOperatorsMixin` — `bindOperator` (service call, boundOperatorIds update, OPERATOR_BOUND event emission, button visibility updates, selectedMetricsOperatorId setting, metrics/status updates, error handling), `unbindOperator` (service call with/without forceWithOperatorId, boundOperatorIds removal, button visibility updates, OFFLINE status/clearPanelMetrics when last operator unbound, error handling), `bindOperatorWithConfirmation`/`unbindOperatorWithConfirmation` (modal mode delegation), `showBindAllConfirmationOverlay` (active operator filtering, no-op when none available, overlay creation), `executeBindAll` (service call, bound_operator_ids handling, fallback to input IDs, success/error feedback), `closeBindAllOverlay` (DOM removal, Escape handler cleanup), `showUnbindAllConfirmationOverlay` (bound operator filtering for web session, overlay creation), `executeUnbindAll` (service call, unbound_operator_ids handling, OFFLINE status/clearPanelMetrics when all unbound, button visibility updates, success/error feedback), `closeUnbindAllOverlay` (DOM removal, Escape handler cleanup), `updateBindAllButtonVisibility`/`updateUnbindAllButtonVisibility` (button show/hide based on operator counts, text updates), `_createBindAllOperatorItem`/`_createUnbindAllOperatorItem` (template replacement with operator data, stale status handling), `_renderFeedback` (template replacement), `_escapeHtml` (HTML special character escaping, null/undefined handling) |
| `test/unit/frontend/operator/operator-device-auth-mixin.unit.test.js` | `OperatorDeviceAuthMixin` — `handleDevicePendingAuthorization` (operator card display, hostname with XSS escaping, approve/deny button creation and click handlers, pending auth UI replacement, timeout management, missing card handling), `_clearPendingAuthUI` (timeout clearing, Map deletion, DOM cleanup, missing card handling), `clearInlineAuthUI` (Map initialization, token validation, timeout control, DOM cleanup, logging), `authorizeDevice` (API call, UI clearing, success/failure alerts, logging), `rejectDevice` (API call, UI clearing on success/failure, logging), `handleDeviceAuthorized` (SSE event handling, UI clearing, missing operator_id handling) |
| `test/unit/frontend/operator/anchored-terminal-operator.unit.test.js` | `TerminalOperatorMixin` — `initOperatorState` (initializes isOperatorBound to false, boundOperator to null, resets state on re-init); `bindEventBusListeners` (binds OPERATOR_STATUS_UPDATED_BOUND to setOperatorBound with operator data, binds all unbound status events to setOperatorUnbound, binds OPERATOR_PANEL_LIST_UPDATED to handleOperatorListUpdate, binds OPERATOR_BOUND/UNBOUND to handlers, binds all approval request events to handleApprovalRequest, binds command execution events with eventType propagation, binds intent result events with eventType propagation, binds OPERATOR_TERMINAL_APPROVAL_DENIED to denyAllPendingApprovals, binds OPERATOR_TERMINAL_AUTH_STATE_CHANGED to setUser/enable/focus or setUser/disable based on auth state, sets _eventsBound flag to prevent duplicate binding, early return when eventBus is null); `handleOperatorListUpdate` (finds BOUND status or is_bound flag operators, calls setOperatorBound when found, calls setOperatorUnbound when not found, guards against null/missing/invalid data); `handleOperatorBound` (calls setOperatorBound with operator from data, guards against null/missing operator); `handleOperatorUnbound` (calls setOperatorUnbound); `setOperatorBound` (sets isOperatorBound to true, stores boundOperator, updates hostnameElement with system_info.hostname or operator.name or 'operator', updates promptElement with system_info.current_user or '$', calls updateInputState, calls appendSystemMessage with connection message, no-op when already bound to same operator, updates when bound to different operator, guards against null elements); `setOperatorUnbound` (sets isOperatorBound to false, clears boundOperator, clears hostnameElement textContent, resets promptElement to '$', calls updateInputState, guards against null elements) |
| `test/unit/frontend/utils/operator-panel-service.unit.test.js` | `OperatorPanelService` — dependency injection (injected vs window.serviceClient fallback), operator lifecycle (bind/unbind/bindAll/unbindAll/stop), operator details & API keys (getDetails/getApiKey/refreshApiKey), device links (generate/create/list/revoke/delete), device authorization (authorize/reject) |
| `test/unit/utils/cert-installers.unit.test.js` | Certificate trust script generators — Windows (.bat), macOS (.sh), Linux (.sh): host/port baking, curl error handling (exit on download failure), certutil error handling, escalation logic, certificate cleanup, NSS database cleanup, custom port support, LAN IP support |

**Key rules for chat rendering tests:**
- Mock `anchoredTerminal` with a `vi.fn()` spy object — never let the DOM test rely on a real terminal initialising from scratch
- Always assert on `streamingContent` map state, not just terminal method call counts
- The `investigation_id` gating in `shouldProcessEvent` must be exercised — include both matching and non-matching cases
- Citation application is deferred to finalization; `handleCitationsReady` and `CITATIONS` dispatch tests must confirm `applyCitations` is NOT called at dispatch time

#### Model test files

| File | Coverage |
|------|----------|
| `test/unit/models/operator_model.unit.test.js` | Operator domain models — `GrantedIntent` (required fields, isActive() TTL check, from() factory, create() with TTL), `HeartbeatNotification` (heartbeat_data, investigation_id/case_id extraction, parse, from), `SystemInfo` (defaults, _extractInternalIp() IP filtering, mergeFromHeartbeat() field merging, forCloudOperator()), `HeartbeatSnapshot` (empty() null defaults, fromHeartbeat() metric extraction), `HistoryEntry` (required fields with defaults, custom actor/details, validation), `CertInfo` (empty() nulls, fromCertData() camelCase mapping), `OperatorStatusInfo` (required fields with defaults, is_active computed by fromOperator() based on status, fromOperator() mapping from OperatorDocument), `OperatorDocument` (parse() system_fingerprint migration, forWire() secret removal, forClient() secret removal with has_api_key boolean flag, fromDB() null handling, forCreate() AVAILABLE status, system_info handling, history entry with truncated session IDs, forSlot() is_slot flag, cloud vs system operator logic, forRefresh() cert handling, forReset() reset history), `OperatorListUpdatedEvent` (defaults, forWire() type/data split), `GeneratedCertificate` (required fields, subject default, forWire() snake_case to camelCase date conversion), `CRLDocument` (required fields with defaults, last_updated now() default), `OperatorSlotCreationResponse` (forSuccess/failure factory methods), `OperatorRefreshKeyResponse` (forSuccess/failure factory methods), `BindOperatorsResponse` (forSuccess with bound IDs, forFailure with error and status code), `UnbindOperatorsResponse` (forSuccess with unbound IDs, forFailure with error and status code, forClient() delegation), `OperatorWithSessionContext` (required fields with defaults, system_info default, create() from operator and sessions with fallback, forWire() delegation) |
| `test/unit/models/request_models.unit.test.js` | HTTP request models — `G8eHttpContext` (required fields with defaults, empty string case_id to null conversion, source_component default), `ChatMessageRequest` (required fields, message minLength validation, attachments/sentinel_mode/llm model defaults, forWire() omits identity fields), `InvestigationQueryRequest` (all optional with defaults, limit min/max constraints, forWire() omits null fields), `SessionCreateRequest` (user_id required, organization_id/metadata defaults), `ApprovalRespondRequest` (approval_id/approved required, reason default), `IntentRequest`/`IntentRequestPayload` (intent required), `UnlockAccountRequest` (user_id required), `SSEPushRequest` (web_session_id/event required), `DirectCommandRequest` (command/execution_id required, command minLength, hostname/source defaults), `CreateOperatorRequest` (operator_id/user_id/operator_session_id required, web_session_id/organization_id/api_key/operator_type/cloud_subtype defaults, system_info/runtime_config defaults), `AttestationResponseJSON` (custom parse validation for response object structure, clientDataJSON/attestationObject string checks), `AssertionResponseJSON` (custom parse validation for response object structure, clientDataJSON/authenticatorData/signature string checks), `PasskeyRegisterVerifyRequest`/`PasskeyAuthVerifyRequest` (user_id/email required, attestation/assertion response with nested model parsing), `PasskeyRegisterChallengeRequest`/`PasskeyAuthChallengeRequest` (user_id/email required), `CreateDeviceLinkRequest` (max_uses/expires_in_hours min constraints), `GenerateDeviceLinkRequest` (operator_id required), `RegisterDeviceRequest` (hostname/os/arch/system_fingerprint required, version default), `BindOperatorsRequest`/`UnbindOperatorsRequest` (operator_ids array required, custom _validate() throws on empty array with validationErrors property), `OperatorSessionRegistrationRequest`/`StopOperatorRequest`/`OperatorRegisterSessionRequest` (operator_id/operator_session_id required, StopOperatorRequest also requires user_id), `SettingsUpdateRequest` (settings required, custom _validate() throws on array), `RefreshOperatorKeyRequest` (user_id required), `InitializeOperatorSlotsRequest` (organization_id optional), `CreateUserRequest` (email/name required, email minLength, roles default), `UpdateUserRolesRequest` (role required, action default), `StopAIRequest` (investigation_id/web_session_id required, reason default), `SessionAuthResponse` (success required, all optional fields default to null or empty objects), `BoundOperatorContext` (operator_id/operator_session_id required, status/operator_type defaults, system_info with SystemInfo model parsing), `RequestModelFactory` (24 static factory methods delegating to respective Model.parse()) |
| `test/unit/models/sse_models.unit.test.js` | SSE event models — `ConnectionEstablishedEvent` (required fields, timestamp default, forWire serialization), `KeepaliveEvent` (serverTime default, forWire), `LLMConfigData`/`LLMConfigEvent` (provider required, model list defaults, nested parsing, forWire serialization), `InvestigationListData`/`InvestigationListEvent` (array defaults, count field, nested parsing), `HeartbeatSSEEvent` (operator_id required, data any type), `AuditDownloadResponse` (user_id required, events array, filters object), `OperatorStatusUpdatedData`/`OperatorStatusUpdatedEvent` (operator_id/status required, system_info object, optional fields), `OperatorPanelListUpdatedData`/`OperatorPanelListUpdatedEvent` (operator_id required, timestamp null default, event timestamp now() default), `CommandResultSSEEvent` (execution_id/status required, all optional fields, custom forWire() splitting type from data), `ApprovalResponseEvent` (success/approval_id/approved required, timestamp now() default), `DirectCommandResponseEvent` (success/execution_id required, message default), `LogStreamEvent` (type required, entry any type), `LogStreamConnectedEvent` (type required, buffered default, timestamp now() default), `G8eePassthroughEvent` (_payload required, custom _validate() checking plain object and non-empty type field, custom forWire() returning inner payload directly) |
| `test/unit/models/organization_model.unit.test.js` | Organization models and service — `OrgAdmin` (required fields, defaults, field assignment, forDB round-trip), `TeamMember` (required fields, defaults including joined_at timestamp, field assignment, forDB round-trip), `OrganizationDocument` (required fields, defaults, field assignment, org_id getter, stats getter with null team_members handling, forClient method, forDB round-trip), `OrganizationModel` service (constructor validation, getById with cacheAside mock, create with parameter defaults and org_admin handling, getByAdminUserId query, addTeamMember with duplicate detection, removeTeamMember, getTeamMembers with admin and member aggregation, incrementInvitesSent with null handling, updateName, error handling and logging) |
| `test/unit/models/response_models.unit.test.js` | HTTP response models — `OperatorListResponse` (success required, data array default, total_count/active_count defaults), `OperatorSlotsResponse` (success required, operator_ids array default, count default), `InternalUserListResponse` (success required, users array default, count default), `AuditEventResponse` (events array default, count/total_investigations defaults), `PasskeyListResponse` (user_id required, credentials array default, count/message defaults), `DBCollectionsResponse` (success required, collections array default), `DBQueryResponse` (success/collection required, documents array default, count/limit defaults), `KVScanResponse` (success/pattern required, keys array default, count/cursor/has_more defaults), `DocsTreeResponse` (success required, tree array default), `SystemNetworkInterfacesResponse` (success required, interfaces array default), `InternalHealthResponse` (success/message required, memory_usage object default, g8es_status/g8ee_status/g8eo_status/uptime_seconds defaults), `InternalSettingsResponse` (success required, settings object default with nested settings), `BindOperatorsResponse` (success required, bound_count/failed_count defaults, bound_operator_ids/failed_operator_ids/errors array defaults, statusCode/error defaults, static forSuccess() with bound IDs, static forFailure() with error and status code, forClient() delegation), `UnbindOperatorsResponse` (success required, unbound_count/failed_count defaults, unbound_operator_ids/failed_operator_ids/errors array defaults, statusCode/error defaults, static forSuccess() with unbound IDs, static forFailure() with error and status code, forClient() delegation), `OperatorBinaryAvailabilityResponse` (success/status/component/version required, platforms array default), `ErrorResponse` (error required, trace_id/execution_id defaults, forClient() delegation), `HealthResponse` (status/service required, timestamp/checks/error defaults), `Interna...[130 bytes truncated] |ationResponse` (success required, session_id/user_id/message/expires_at/validation_details defaults, valid default) |

#### Middleware unit test files

| File | Coverage |
|------|----------|
| `test/unit/middleware/rate-limit.unit.test.js` | Rate limiting middleware — all 18 rate limiters tested (globalPublicRateLimiter, authRateLimiter, chatRateLimiter, sseRateLimiter, apiRateLimiter, uploadRateLimiter, operatorRefreshRateLimiter, operatorAuthRateLimiter, operatorAuthIpBackstopLimiter, auditRateLimiter, consoleRateLimiter, operatorApiRateLimiter, deviceLinkRateLimiter, deviceLinkGenerateLimiter, deviceLinkCreateRateLimiter, deviceLinkListRateLimiter, deviceLinkRevokeRateLimiter, settingsRateLimiter, passkeyRateLimiter); handler error responses (429 status, correct RateLimitError messages); standardHeaders enabled; custom key generators (API key prefix extraction, userId fallback to IP); validate options (keyGeneratorIpFallback disabled where applicable); logging (warn/error with IP, path, method, userAgent, redacted webSessionId, truncated apiKeyPrefix, operatorSessionId); createRateLimiters factory (returns all limiters, accepts config parameter) |

### Unit test patterns

**Service unit tests** (`test/unit/services/`) use fully-mocked dependencies injected at construction time. Never use `getTestServices()` in unit tests — that is for integration tests only.

```javascript
const svc = new BindOperatorsService({
    OperatorDataService: { getOperator: vi.fn(), ... },
    bindingService:  { bind: vi.fn(), unbind: vi.fn(), getBoundOperatorSessionIds: vi.fn() },
    operatorSessionService: { validateSession: vi.fn() },
    sseService: { publishEvent: vi.fn() },
});
```

Unit tests for services that have multiple constructor dependencies must inject every dependency — never rely on default `null` values for deps that are exercised by the code path under test.

**`SSEService` unit tests** — construct with the same injection pattern:

```javascript
const svc = new SSEService({
    OperatorDataService: createOperatorServiceMock(),
    g8edSettings: { llm_model: 'test-model', llm_provider: 'openai' },
    internalHttpClient: { queryInvestigations: vi.fn() },
    boundSessionsService: { resolveBoundOperators: vi.fn().mockResolvedValue([]) },
});
```

#### Auth service unit test files

| File | Coverage |
|------|----------|
| `test/unit/services/auth/post-login-service.unit.test.js` | `PostLoginService` — `createSessionAndSetCookie` (session creation, cookie attributes, download API key fetch/create, org fallback, null key on create failure, request context forwarding); `onSuccessfulLogin` and `onSuccessfulRegistration` (correct delegation to `activateG8ENodeOperatorForUser` and `initializeOperatorSlots` with right args, org fallback when `organization_id` absent, fire-and-forget with error-level logging); `_initializeSlotsAndActivateG8eNode` (sequential ordering — `initializeOperatorSlots` completes before `activateG8ENodeOperatorForUser` runs, slot init failure prevents activation, activation errors propagate to caller) |
| `test/unit/services/auth/bound-sessions-service.unit.test.js` | `BoundSessionsService` — constructor (requires cacheAside, requires operatorService); `bind`/`unbind`/`getBoundOperatorSessionIds`/`getWebSessionForOperator` (bidirectional KV entries, binding doc create/update, empty returns); `resolveBoundOperators` (reads binding document via cacheAside, empty on missing/ended/empty doc, skips missing operators, returns BoundOperatorContext with system_info, parallel operator fetch, no validateSession dependency regression); `resolveBoundOperatorsForUser` (returns empty array when no bound sessions exist, resolves operators from all user sessions, filters out sessions for different users, filters out inactive sessions, skips operators that are not found) |

#### Operator service unit test files

| File | Coverage |
|------|----------|
| `test/unit/services/operator/operator_bind_service.unit.test.js` | `BindOperatorsService` — `bindOperators` (operator not found error, wrong user error, successful bind with G8eHttpContext relay, multiple operator bind, partial failure handling, already bound operator detection skips rebinding, operator with no active session error); `bindOperator` wrapper method (delegates to bindOperators); `unbindOperators` (successful unbind with G8eHttpContext relay, multiple operator unbind, operator not found error, wrong user authorization error, partial failure handling, operator with no session_id skips KV unbind); `unbindOperator` wrapper method (parses plain object via UnbindOperatorsRequest.parse, accepts UnbindOperatorsRequest instance directly); relay failure handling (continues bind/unbind when relayRegisterOperatorSessionToG8ee fails); broadcast failure handling (continues bind/unbind when broadcastOperatorListToSession fails); null context wrapper handling (skips relay when getOperatorWithSessionContext returns null) |
| `test/unit/services/operator/operator_slot_service.unit.test.js` | `OperatorSlotService` — `initializeOperatorSlots` (creates missing slots up to `DEFAULT_OPERATOR_SLOTS`, single DB query with no second round-trip, no API key reindexing, returns combined existing + new operator IDs, excludes TERMINATED operators from count and return value, no-op when already at capacity); `claimSlot` (uses provided status parameter, defaults to ACTIVE when status not provided); `refreshOperatorApiKey` (terminates old operator and creates new one with fresh key/cert, fails on not found, fails on unauthorized); `generateOperatorApiKey` (correct prefix and suffix format) |
| `test/unit/services/operator/operator_auth_service.unit.test.js` | `OperatorAuthService` — `_completeAuthentication` (preserves BOUND status on re-authentication when operator is already BOUND, sets ACTIVE status for non-bound operators; passes correct `claimStatus` to `claimOperatorSlot`, `updateUserOperator`, and G8eHttpContext relay to g8ee; uses `SystemInfo.parse()` to hydrate wire payload preserving all fields) |
| `test/unit/services/operator/operator_download_service.unit.test.js` | `OperatorDownloadService` — constructor validation (requires listenUrl, strips trailing slash, stores internalAuthToken, defaults token to null); `getBinary` (fetches from g8es blob store at `/blob/operator-binary/{os}-{arch}`, returns Buffer, sends X-Internal-Auth header, throws specific error on non-ok response, throws on fetch failure, omits auth header when no token); `hasBinary` (checks blob metadata at `/blob/operator-binary/{os}-{arch}/meta`, returns boolean); `getPlatformAvailability` (checks all defined platforms) |
| `test/unit/routes/operator/operator_routes.unit.test.js` | `OperatorRoutes` — health endpoint (returns healthy/degraded based on platform availability); download endpoint (streams binary on success, 400 for unsupported OS, 401 on auth failure, 503 when binary unavailable); sha256 endpoint (returns checksum for binary) |
| `test/unit/routes/operator/operator_approval_routes.unit.test.js` | `OperatorApprovalRoutes` — POST /respond (success relay via OperatorRelayService, OperatorRelayService constructed with internalHttpClient, relay body matches g8ee contract with only `approval_id`/`approved`/`reason`, context fields travel via G8eHttpContext with `bound_operators`, no dependency on req.services regression, 400 on missing context fields, 400 when no bound operators resolved); POST /direct-command (success relay via OperatorRelayService, 400 when no bound operator session) |
| `test/unit/services/operator/operator_relay_service.unit.test.js` | `OperatorRelayService` — `relayApprovalResponseToG8ee` (correct HTTP method/path/body/g8eContext, pre-validated data passed through without re-parsing); `relayDirectCommandToG8ee` (correct HTTP method/path/body/g8eContext); context validation (throws on missing g8eContext); HTTP client validation (throws when not initialized) |

#### Platform service unit test files

| File | Coverage |
|------|----------|
| `test/unit/services/initialization.unit.test.js` | Composition root tests — `initializeSettingsService` (initializes settings service and core clients, returns same instance on multiple calls); `initializeServices` (full multi-phase initialization, exercises entire composition root); services bag contract (verifies all 28 services match server.js expectations, verifies additional services not in server.js bag); accessor throw behavior (all 28 accessor functions throw with descriptive error messages when called before initialization); `resetInitialization` (nullifies all service instances, removes signal handlers); service instance consistency (returns same instance for settingsService and cacheAsideService across multiple calls) |
| `test/unit/services/platform/bootstrap_service.unit.test.js` | `BootstrapService` — constructor (default /g8es path, custom path support, null cached values); `loadInternalAuthToken` (load and cache from volume, trim whitespace, return cached on subsequent calls, null when file missing, null on read failure, null when volume nonexistent); `loadSessionEncryptionKey` (load and cache from volume, trim whitespace, return cached on subsequent calls, null when file missing, null on read failure, null when volume nonexistent); `loadCaCertPath` (find at /g8es/ca.crt, find at /g8es/ca/ca.crt legacy path, prefer /g8es/ca.crt over legacy, return cached on subsequent calls, null when neither location exists, null when volume nonexistent, continue to second path if first fails); `getSslDir` (return volume path, default /g8es); `isAvailable` (true when token exists, true when key exists, true when CA cert exists, false when volume nonexistent, false when empty volume, true when any bootstrap data available); `clearCache` (clear all cached values, allow reload after clear); `_safeListVolume` (return volume contents, empty directory message, error message when nonexistent, error message on read failure); integration scenarios (load all bootstrap data, handle partial data, cache across multiple instances) |
| `test/unit/services/platform/g8ep_operator_service.unit.test.js` | `G8ENodeOperatorService` — `getG8ENodeOperatorForUser` (returns operator+active status, null when no slot); `launchG8ENodeOperator` (persists API key to platform_settings then starts supervisor via XML-RPC, uses supervisor_port from settings, restarts if ALREADY_STARTED, throws on savePlatformSettings failure, reads settings once per launch); `relaunchG8ENodeOperatorForUser` (stops/resets/launches, failure when no slot, failure when reset has no API key); `activateG8ENodeOperatorForUser` (skips when already active, launches when available with API key, swallows errors). g8ep script validation — static analysis of `fetch-key-and-run.sh` and `entrypoint.sh` to assert all `https://g8es` URLs include port 9000; `--ca-url` is not passed to the operator binary; blob store download path exists; `--working-dir /home/g8e` is passed to the operator binary (regression test for permission denied error when operator tried to create ``). |

#### Client and cache service unit test files

| File | Coverage |
|------|----------|
| `test/unit/services/clients/g8es_kv_cache_client.unit.test.js` | `KVCacheClient` — core KV (get/set/del/setex/get_json/set_json/exists/incr/decr/expire/ttl/ping/status), set flags (EX/PX/NX), del swallows per-key errors, keys returns matches on success, keys defaults to wildcard, keys throws `KVOperationError` on failure (operation/pattern/cause preserved), scan supports MATCH/COUNT, scan throws `KVOperationError` on failure; hash (hset/hget/hgetall/hdel); list (rpush/lpush/lrange/llen/ltrim); set (sadd/srem/smembers/scard); sorted set (zadd/zrem/zrange/zrevrange); stream (xadd/xrange); lifecycle (status/quit/disconnect/terminate/isTerminated) |
| `test/unit/services/cache/cache_aside_service.unit.test.js` | `CacheAsideService` — createDocument (DB write then cache warm, DB failure skips cache); getDocument (cache HIT returns cached, cache MISS reads DB and warms cache, null when not in DB); updateDocument (DB update then cache invalidation); deleteDocument (DB delete then cache invalidation); `_invalidateQueryCache` (deletes all matching query keys, `KVOperationError` from keys() does not propagate — write operations still succeed on create and update); queryDocuments (query cache HIT returns cached, query cache MISS reads DB and caches results) |

#### Internal route unit test files

| File | Coverage |
|------|----------|
| `test/unit/routes/internal/internal-operator-routes.unit.test.js` | `POST /:operatorId/refresh-key` (success shape, correct service args, 400 on missing user_id, 400 on service failure, 500 on throw); `POST /:operatorId/reset-cache` (success shape, correct service args, 404 on failure, 500 on throw); `POST /user/:userId/reauth` (200 with user_id and operator_id, correct service args, 404 on failure, 500 on throw); `GET /user/:userId` (operator list shape, correct service args, 500 on throw); `POST /user/:userId/initialize-slots` (slot ids returned, userId fallback for missing org, 500 on throw); `GET /:operatorId/status` (200 with data, 404 on not found, 500 on throw); `GET /:operatorId` (200 with data, 404 on not found, 500 on throw); `GET /:operatorId/with-session-context` (200 with data, 404 on not found, 500 on throw) |
| `test/unit/routes/platform/chat_routes.unit.test.js` | `POST /api/chat/send` (typed ChatMessageRequest.parse, G8eHttpContext with bound_operators, typed ChatMessageResponse, error handling with 500); `GET /api/investigations` (typed InvestigationQueryRequest.parse, G8eHttpContext, URLSearchParams construction with null/undefined filtering, typed InvestigationListResponse, empty array handling, error handling with 500); `GET /api/investigations/:id` (G8eHttpContext with investigation_id from params, case_id from query, typed ChatMessageResponse, missing case_id null handling, error handling with 500); `POST /api/chat/stop` (typed StopAIRequest.parse, G8eHttpContext, typed ChatActionResponse with investigation_id/was_active data, error handling with ErrorResponse); `DELETE /api/cases/:id` (G8eHttpContext with case_id from params, 204 response on success, error handling with ErrorResponse); `GET /api/chat/health` (typed ChatHealthResponse with SystemHealth.HEALTHY/UNHEALTHY, internal_services from healthCheck, error handling with 500, requireInternalOrigin middleware) |
| `test/unit/routes/platform/health_routes.unit.test.js` | `GET /health` (basic healthy status with SystemHealth.HEALTHY and SourceComponent.G8ED); `GET /health/live` (alive status); `GET /health/store` (ready when all components up, 503 when g8es down, 503 when cacheAsideService null/not_initialized, requireInternalOrigin middleware); `GET /health/details` (full healthy details with session count and database status, 503 if database check fails, error handling with SystemHealth.UNHEALTHY) |
| `test/unit/routes/platform/mcp_routes.unit.test.js` | `POST /mcp` JSON-RPC dispatch: `initialize` (returns server info and capabilities), `notifications/initialized` (204 no body), `ping` (empty result), `tools/list` (proxies to g8ee mcpToolsList, error handling), `tools/call` (proxies to g8ee mcpToolsCall with tool_name/arguments/request_id, g8ee error field passthrough, missing tool name -32602), invalid requests (-32600 for missing jsonrpc/method fields, -32601 for unknown method), OAuth Client ID authentication (success via x-oauth-client-id header, success via oauth_client_id query param, failure on invalid key, failure on missing user_id, failure on user not found, fallback to session auth when no OAuth Client ID provided), requireAuth middleware enforcement |

#### g8ee MCP service unit test files

| File | Coverage |
|------|----------|
| `tests/unit/services/mcp/test_gateway_service.py` | `MCPGatewayService` — `list_tools` (MCP format conversion from ToolDeclarations, empty tools, multiple tools flattened); `call_tool` (success result with MCP content wrapping, error result sets isError, invocation context always reset even on exception, timeout returns graceful MCP error after MCP_TOOL_CALL_TIMEOUT_SECONDS with approval-pending message, invocation context reset on timeout); `_build_investigation_context` (resolves operator documents from bound_operators, skips non-BOUND operators, handles missing operator docs); `_tool_result_to_mcp` (success result, error result, fallback model_dump serialization) |

#### SSE event pipeline integration test files

| File | Coverage |
|------|----------|
| `test/integration/sse/sse-event-pipeline.integration.test.js` | Full wire roundtrip for every g8ee-published event type: `G8eePassthroughEvent.forWire()` → `SSEService.publishEvent()` → raw `data: <json>\n\n` SSE frame → `SSEConnectionManager.handleSSEEvent()` → `eventBus.emit(eventType, payload)` assertion. Covers all chat iteration events (`TEXT_CHUNK_RECEIVED`, `TEXT_COMPLETED`, `COMPLETED`, `FAILED`, `STOPPED`), `CITATIONS_RECEIVED` grounding shape, search web tool (`REQUESTED`, `COMPLETED`, `FAILED`), network port check (`REQUESTED`, `COMPLETED`, `FAILED`), operator command approval and lifecycle (`APPROVAL_REQUESTED`, `STARTED`, `COMPLETED`, `FAILED`), file edit approval and lifecycle, intent approval, multi-event sequence ordering, and wire shape contract assertions (`forWire()` produces the inner payload directly; SSE frame format; `type`/`data` destructuring). Only mocks: `@g8ed/utils/logger.js`. No g8es, no network, no auth. |
| `test/integration/sse/sse-event-contract.integration.test.js` | (1) Shared fixture compliance — asserts `shared/test-fixtures/sse-events.json` has all required keys with correct shape, and that every fixture `type` string matches the corresponding `EventType` constant. (2) Routing field compliance — publishes typed `G8eePassthroughEvent` instances through real `SSEService`, reads raw bytes from `MockSSEResponse.getWrittenData()`, and asserts `investigation_id`/`case_id`/`web_session_id` survive the wire round-trip for 5 fixture event types. (3) Platform event wire format — verifies `ConnectionEstablishedEvent` and `KeepaliveEvent` serialize correctly via `SSEService.publishEvent()`. No `MockSSEBrowser`, no `waitForEvent`, no string literals for event types. |
| `test/integration/sse/msw-sse-network-failures.integration.test.js` | SSE connection lifecycle via real `SSEService` + `MockSSEResponse`: delivery to registered connection (single event, burst of 10), session isolation (publish to A does not reach B, publish to both reaches each independently), unregistered connection (no write after `unregisterConnection`, stale `connectionId` guard does not evict a newer registration), destroyed response (no write), reconnect (new registration after unregister delivers correctly). Asserts on `MockSSEResponse.getWrittenData()` — no `MockSSEBrowser`, no `waitForEvent`. |

#### Auth integration test files

| File | Coverage |
|------|----------|
| `test/integration/auth/passkey_flow.integration.test.js` | Full new-user registration flow: `POST /api/auth/register` → `POST /api/auth/passkey/register-challenge` → `POST /api/auth/passkey/register-verify`. Real `UserService`, `PasskeyAuthService`, `WebSessionService`, `CacheAsideService`, `LoginSecurityService`, `OrganizationModel`. Only mock: `@simplewebauthn/server` (FIDO2 authenticator — cannot run in test env). |

### Integration test patterns

New integration tests go in `test/integration/` organized by domain (e.g., `test/integration/auth/`, `test/integration/operator/`). Use `getTestServices()` for all service access — never initialize services manually.

Some integration tests use `vi.mock` for legitimate boundary reasons. Permitted mocks in integration tests:

- **`@g8ed/utils/logger.js`** — silencing log output is acceptable in any test type
- **`@g8ed/middleware/authorization.js`** (`requireInternalOrigin`, `requireOperatorOwnership`, `requireAdmin`) — bypassing auth middleware is necessary for route-level integration tests that are testing the route logic, not the auth layer
- **`@g8ed/middleware/rate-limit.js`** — bypassing rate limiters prevents test interference
- **`@g8ed/utils/security.js`** — `redactWebSessionId` stub for deterministic output
- **`@g8ed/services/clients/internal_http_client.js`** — the HTTP boundary to g8ee; mocking this is correct for g8ed-only integration tests
- **`@g8ed/services/platform/attachment_service.js`** — mocked in chat/investigation route tests; acceptable as an external service boundary
- **`@simplewebauthn/server`** and **`@simplewebauthn/server/helpers`** — the FIDO2 authenticator library cannot run in a test environment; mocking this is correct

Mocks that are **not** permitted in integration tests (and must be converted if found):

- Any mock of core g8es service internals (`dbClient`, `KVClient`, `G8esPubSubClient`, session services, operator services)
- Any mock of `g8eNodeOperatorService` or `operatorBinaryCache` — use real service initialization via `getTestServices()`

---

## g8ee — Python

**Location:** `components/g8ee/`

### Test types

- **Unit** — business logic and translation logic in isolation; mock external client boundaries only
- **Integration** — real g8es + real service wiring; use real KV/pub/sub clients from `conftest.py`; new integration tests go in `tests/integration/`
- **AI integration** — real LLM provider calls; credentials must be present; never replace with fake outputs
- **E2E** — marker defined; `tests/e2e/` is currently empty

### Test layout

```
tests/
├── conftest.py               # Shared fixtures (unit + integration)
├── fixtures/                 # Test data builders, mock helpers
├── fakes/                    # Typed fake implementations (not MagicMock)
├── unit/                     # Unit test suites
├── integration/              # Integration tests — real service wiring, in-memory g8es
└── e2e/                      # Placeholder (empty)
```

### Test data isolation

**Critical Rule:** g8ee integration tests MUST use unique object names to prevent conflicts between tests running in session scope.

**Factory Pattern:** Use the factories in `tests/fakes/factories.py` which automatically generate unique IDs:

```python
from tests.fakes.factories import create_investigation_data, build_operator_document

# Factory automatically generates unique IDs
investigation = create_investigation_data()  # Unique investigation_id, case_id, user_id
operator = build_operator_document()      # Unique operator_id, user_id
```

**Never use hardcoded IDs** like `"inv-test-001"` or `"op-test-001"` in integration tests. Factories handle uniqueness automatically.

### Service access

Integration tests use real services via the `cache_aside_service` fixture:

```python
def test_example(self, cache_aside_service):
    investigation_service = InvestigationDataService(cache=cache_aside_service)
    # Use factories for unique test data
    investigation = create_investigation_data()
```

### Pytest configuration (`pyproject.toml`)

- Python 3.13+
- `asyncio_mode = auto` with `asyncio_default_fixture_loop_scope = session`
- Strict marker/config enforcement
- Quiet mode with short tracebacks (`-q`, `--tb=short`, `--no-header`)
- Warnings treated as errors (explicit narrow ignores for `ResourceWarning`, `PytestUnraisableExceptionWarning`, and one aiohttp deprecation)
- Timeout: 60s (signal method)

### Markers

| Marker | When to use |
|--------|-------------|
| `unit` | Pure business logic, no external I/O |
| `integration` | Real g8es, real service wiring |
| `ai_integration` | Real LLM provider calls; skipped automatically unless `--llm-provider` is passed to `./g8e test` with valid credentials |
| `e2e` | Full stack (defined, not yet populated) |
| `slow` | Tests with significant wall-clock time |
| `smoke` | Quick smoke tests for CI |
| `ai` | Tests involving AI/LLM functionality |
| `aws` | Tests requiring LocalStack AWS emulation |
| `intent_workflow` | AWS intent escalation workflow tests |
| `requires_api` | Tests that call a real external API (e.g. Vertex AI Search); skipped automatically when the required credentials are absent |

#### Chat pipeline integration test files

| File | Coverage |
|------|----------|
| `tests/integration/test_g8e_http_context_integration.py` | `get_g8e_http_context` header round-trip in 6 segments: happy-path field extraction, `bound_operators` JSON parsing to typed `BoundOperator` list, `X-G8E-New-Case` sentinel + `UNKNOWN_ID` fallbacks, missing required headers raise `AuthenticationError`, `source_component` enum validation, optional header passthrough (`organization_id`, `task_id`, `execution_id`) |
| `tests/integration/test_tool_search_web_integration.py` | `search_web` tool in 7 segments: registration gating (provider present/absent), `execute_tool_call` routing to `provider.search()`, `SearchWebResult` response shape and typed `WebSearchResultItem` list, failure and empty result handling, `build_search_web_grounding` → `GroundingMetadata` (`grounding_used`, chunks, sources count, query list), SSE events (`LLM_TOOL_SEARCH_WEB_REQUESTED`, `LLM_CHAT_ITERATION_CITATIONS_RECEIVED`), `_search_calls` observability |
| `tests/integration/test_ai_tool_calls_integration.py` | AI tool registration and execution framework in 20 tests: tool declaration consistency, payload serialization, operator-bound vs operator-not-bound modes, all 11 active tools (RUN_COMMANDS, FILE_CREATE/WRITE/READ/UPDATE, LIST_FILES, FETCH_FILE_HISTORY, FETCH_FILE_DIFF, CHECK_PORT, GRANT_INTENT/REVOKE_INTENT, G8E_SEARCH_WEB), error handling, SSE event routing, and integration with the full chat pipeline |

#### Chat pipeline unit test files

`ChatPipelineService` unit tests are in `test_chat_pipeline_triage_delivery.py` which covers triage delivery and run_chat_impl short-circuit behavior.

#### Tribunal command generator unit test file

| File | Coverage |
|------|----------|
| `tests/unit/services/ai/test_command_generator.py` | `_resolve_model` — fallback chain: returns `assistant_model` when set, falls back to `primary_model` when `assistant_model` is None, falls back to provider default model when both are None; provider defaults for all four providers (Ollama, OpenAI, Anthropic, Gemini); `assistant_model` takes priority over `primary_model`; result is always a non-empty `str`. `_infer_provider_for_model` — maps Gemini/OpenAI/Anthropic prefixes and Ollama colon convention to correct `LLMProvider` enum; returns `None` for ambiguous names; case-insensitive. `_resolve_provider_and_model` — returns coupled (provider, model) pairs; Ollama assistant with Gemini primary resolves to Ollama provider; Gemini assistant with Ollama primary resolves to Gemini provider; ambiguous model falls back to `settings.provider`; all provider defaults map to their own provider; cross-provider override (OpenAI model overrides Ollama provider). `_is_system_error` — classifies auth errors (401, 403, API key), network errors (connection refused, timeout, DNS, SSL, ECONNREFUSED), config errors (unsupported provider); non-system errors (empty response, JSON errors, content filter) return False; empty string returns False. `TribunalSystemError` — carries `pass_errors` and `original_command` attributes; is an `Exception` subclass. `TribunalFallbackPayload` — accepts `pass_errors` list; defaults to `None`. Pass errors collection — `_run_generation_pass` appends exception messages and empty response errors to `pass_errors`; successful passes do not append. `generate_command` integration — raises `TribunalSystemError` when all passes fail with system errors; raises `TribunalGenerationFailedError` on non-system failures; raises `TribunalProviderUnavailableError` when provider init fails; returns `CommandGenerationOutcome.DISABLED` when Tribunal is disabled via config; routes to correct provider via `_resolve_provider_and_model` (verified with mock factory assertion). `TestGenerateCommandOutcomes` — returns `DISABLED` outcome when `llm_command_gen_enabled=False`. `TestNewEnumValues` — `DISABLED` outcome and `SYSTEM_ERROR` outcome exist and are distinct from `FALLBACK`. `TribunalVerifierFailedError` — raised on empty verifier response, no valid revision, or exception (replaces silent pass). `Mixed errors fallback` — 1 system error + 2 non-system errors produces `TribunalGenerationFailedError` (not `TribunalSystemError`). `TribunalSessionStartedPayload` regression — rejects `None` model, accepts resolved model. `Role` import regression — `Role.USER` is importable from `llm_types`, `_run_generation_pass` and `_run_verifier` build `Content` with `Role.USER`. `TestTribunalMemberCycling` — `_member_for_pass` cycles correctly through AXIOM (pass 0), CONCORD (pass 1), VARIANCE (pass 2), then repeats; `_temperature_for_pass` returns canonical temperatures (0.0, 0.4, 0.8) for each member; temperatures match `TRIBUNAL_MEMBER_TEMPERATURES` dict from shared constants. `TestGenerateCommandHappyPath` — full happy-path integration through all four pipeline stages (generation -> voting -> verification -> result) with mocked provider: consensus path with verifier disabled (`CONSENSUS` outcome, no review events emitted), verified path with verifier approval (`VERIFIED` outcome, review events emitted), verification-failed path with verifier revision (`VERIFICATION_FAILED` outcome, revised command replaces vote winner), partial failure with surviving candidates reaching consensus (1-of-3 pass failure, 2 candidates still produce `VERIFIED`), single-pass configuration exercising all stages, SSE event emission order validation (session started < pass completed < consensus reached < review started < review completed < session completed), result model field completeness (all `CommandGenerationResult` fields populated with correct types and ranges, candidate member assignment via `_member_for_pass`), refined-command detection (`refined=True` in completed payload when `final_command != original_command`, `refined=False` when unchanged). `TestGenerateCommandVerifierFailure` — end-to-end `TribunalVerifierFailedError` propagation through `generate_command` for all three verifier failure paths: empty verifier response (reason=`empty_response`, error attribute set, `original_command` is vote winner), no valid revision (reason=`no_valid_revision`, verifier returns same command as candidate), verifier exception (reason=`exception`, error contains exception message); SSE event assertions confirm `TRIBUNAL_SESSION_STARTED`, `TRIBUNAL_VOTING_PASS_COMPLETED`, `TRIBUNAL_VOTING_CONSENSUS_REACHED`, `TRIBUNAL_VOTING_REVIEW_STARTED`, and `TRIBUNAL_VOTING_REVIEW_COMPLETED` are emitted before the exception propagates while `TRIBUNAL_SESSION_COMPLETED` is not emitted; `original_command` on the error is the vote winner (refined command) not the caller's original command; single-pass configuration (passes=1) still raises on verifier failure. |

#### Eval judge unit test file

| File | Coverage |
|------|----------|
| `tests/unit/services/ai/test_eval_judge.py` | `EvalGrade` model — score range validation (1-5 enforced via `ge`/`le`), empty/whitespace reasoning rejected, serialization roundtrip. `_extract_json` — plain JSON, markdown code fences (`json` and bare), surrounding whitespace, invalid JSON raises `JSONDecodeError`. `_is_retryable` — rate limit strings (429, 503, resource exhausted, quota), `status_code` attribute, non-retryable errors (401, generic). `EvalJudge` construction — None provider raises `EvalJudgeError`, empty/None model raises `EvalJudgeError`, valid construction stores model. `grade_turn` happy path — high score passes, low score fails, threshold score passes, `passed` is deterministic (LLM cannot override), response format config (`temperature=0.0`, `response_format` set), model forwarded to provider, markdown-fenced responses parsed, extra fields ignored. `grade_turn` error paths — empty candidates raises `EvalJudgeError`, None response raises, invalid JSON raises, missing score/reasoning raises, out-of-range score (0, 10) raises, non-retryable API error raises immediately (single call). Retry logic — retries on 429 rate limit (recovers on 2nd attempt), retries on 503 (recovers on 3rd attempt), all retries exhausted raises `EvalJudgeError`, backoff delays double (2s, 4s), `EvalJudgeError` from `_call_and_parse` propagates immediately without retry. Prompt construction — all criteria present in prompt text, None/empty tool lists handled. Score thresholds — parametrized 1-5 with deterministic `passed` from `PASSING_THRESHOLD`. |

#### Memory generation service unit test file

| File | Coverage |
|------|----------|
| `tests/unit/services/ai/test_memory_generation_service.py` | `MemoryGenerationService` — constructor accepts `MemoryDataServiceProtocol` (not concrete `MemoryDataService`), uses constants `CONVERSATION_HISTORY_LIMIT=20` and `FALLBACK_TEXT_LIMIT=2000`. `update_memory_from_conversation` — creates new memory when no existing memory, returns existing memory when conversation is empty, truncates conversation to last 20 messages before LLM call. `_conversation_to_contents` — includes memory context as first Content with all current fields, filters thinking messages (`is_thinking=True`), maps `USER_CHAT` to `Role.USER`, maps `AI_PRIMARY`/`AI_ASSISTANT` to `Role.MODEL`, skips unknown senders, adds analysis request as final Content. `_extract_json_from_markdown` — extracts JSON from markdown code blocks with and without language tags, handles multiline JSON, returns `None` for non-object blocks or no fences. `_extract_key_value_pairs` — extracts key-value pairs from plain text, normalizes keys to snake_case, handles quoted keys/values, removes trailing commas, skips comment lines (#, //, /*), skips empty lines, treats dash-prefixed lines as continuation lines (preserves dash), supports multiline values with space concatenation. `_parse_memory_analysis` — 4-fallback strategy: direct JSON parsing, JSON from markdown extraction, key-value pair extraction, raw text to `investigation_summary` (truncated to 2000 chars, strips braces/brackets). Field aliases map common names (summary→investigation_summary, background→technical_background, etc.). Returns empty `MemoryAnalysis` for empty/whitespace input. Constants validation — verifies `CONVERSATION_HISTORY_LIMIT=20` and `FALLBACK_TEXT_LIMIT=2000` |

#### Agent tool loop unit test file

| File | Coverage |
|------|----------|
| `tests/unit/services/ai/test_agent_execute_tool_call.py` | `execute_tool_call` — execution_id generation (operator tools get `exec_<12hex>_<timestamp>` format, unique per call, `None` for non-operator tools); operator tool detection (all `OperatorToolName` members detected, unregistered tools not detected); internal field injection (`execution_id` and `_web_session_id` injected for operator tools, not for non-operator tools, original args preserved); `ToolCallResult` structure (typed model with `call_info`/`result_info` as `StreamChunkData`, `tool_name` propagation, raw handler result passthrough, `execution_id` consistent across `call_info`/`result_info`, `error_type` set on failure); tool name extraction (`.name` attribute, `None` falls back to empty string); Tribunal refinement (`generate_command` called for `run_commands_with_operator` with correct `original_command`/`intent`/`os_name`, refined command replaces original in `tool_args`, unchanged command preserved, skipped for non-command operator tools, skipped when `command` arg missing, receives operator OS context from investigation); `TribunalSystemError` halt (returns failed `ToolCallResult` with joined `pass_errors` in error message and `EXECUTION_ERROR` type, prevents underlying executor from being called) |

#### LLM provider SSL unit test file

| File | Coverage |
|------|----------|
| `tests/unit/llm/test_provider_ssl.py` | SSL verification strategy for all four LLM providers — cloud endpoints (Gemini, Anthropic, OpenAI) use `certifi` (Mozilla CA bundle) bypassing `G8E_SSL_CERT_FILE`; internal endpoints (Ollama, LAN vLLM) use platform CA cert; factory passes `ca_cert_path` only to providers that may hit internal endpoints; `_is_internal_endpoint` pattern matching; Gemini `_get_client` env var override and restore (including on failure); env var cleanup when no original value existed |

#### PubSub client unit test files

| File | Coverage |
|------|----------|
| `tests/unit/clients/test_pubsub_client.py` | `PubSubClient` — constructor (explicit URL override, trailing slash stripping, default component name); wire protocol constant regression (`PubSubWireEventType` members exist with correct values, `PubSubMessageType` backward-compat alias, `PubSubAction.PSUBSCRIBE`, `PubSubField.PATTERN`, values match g8es Go constants); `subscribe` (sends action, adds channel after `_ensure_ws`, cleans up ACK event); `psubscribe` (sends action, adds pattern after `_ensure_ws`, cleans up ACK event, times out without ACK); `_ws_reader` reconnection (nulls `_ws` on exit, triggers `_reconnect_loop` with active subscriptions, skips reconnect on clean shutdown, logs disconnect handler failures at WARNING); `_reconnect_loop` (succeeds on first try, retries with exponential backoff, stops when subscriptions cleared) |

#### Shared constants contract test files

| File | Coverage |
|------|----------|
| `tests/unit/utils/test_shared_mcp_wire.py` | Contract test: MCP Pydantic models in `app/services/mcp/types.py` must exactly match canonical field names and types in `shared/models/wire/mcp.json`. |
| `tests/unit/utils/test_shared_pubsub_constants.py` | Contract test: `PubSubWireEventType`, `PubSubAction`, and `PubSubField` enum values must exactly match `shared/constants/pubsub.json` wire protocol definitions — bidirectional coverage (every JSON key has an enum member, no extra enum members beyond JSON) |
| `tests/unit/utils/test_shared_bound_operator_context.py` | Contract test: g8ee BoundOperator model fields must exactly match `shared/models/wire/bound_operator_context.json` — prevents desynchronization between g8ed's BoundOperatorContext.forWire() output and g8ee's BoundOperator parsing logic |
| `tests/unit/utils/test_shared_status_constants.py` | Contract test: `OperatorType` and `CloudSubtype` enum values must exactly match `shared/constants/status.json` canonical values — bidirectional coverage (every JSON key has an enum member, no extra enum members beyond JSON) |
| `tests/unit/utils/test_shared_intents_constants.py` | Contract test: `CloudIntent` enum loaded from `shared/constants/intents.json`, `CLOUD_INTENT_DEPENDENCIES` graph matches JSON exactly, `CLOUD_INTENT_VERIFICATION_ACTIONS` covers every intent with valid IAM action format, `OperatorCommandService._get_verification_action_for_intent` delegation, `OperatorIntentService.execute_intent_permission_request` validation (unknown intent rejection, empty intent, missing justification, comma-separated validation, dependency validation, non-cloud operator rejection) |
| `tests/unit/utils/test_shared_agent_constants.py` | Contract test: `TriageComplexityClassification`, `TriageConfidence`, `TriageIntentClassification`, `TribunalMember` enum values must exactly match `shared/constants/agents.json` canonical values — verifies triage enum alignment, agent metadata completeness (9 agents including tribunal members axiom, concord, variance as first-class agents), and tribunal temperatures match shared constants (0.0, 0.4, 0.8) |

#### `requires_api` — Vertex AI Search

When integration tests targeting Vertex AI Search are added, mark them `requires_api` and `asyncio(loop_scope="session")`. They will skip automatically when `VERTEX_SEARCH_ENABLED`, `VERTEX_SEARCH_PROJECT_ID`, `VERTEX_SEARCH_ENGINE_ID`, or `VERTEX_SEARCH_API_KEY` are absent.

To run `requires_api` tests, export the following in your shell before running `./g8e test`:

```
VERTEX_SEARCH_ENABLED=true
VERTEX_SEARCH_PROJECT_ID=your-gcp-project-id
VERTEX_SEARCH_ENGINE_ID=your-vertex-search-app-id
VERTEX_SEARCH_API_KEY=your-gcp-api-key
```

`run_tests.sh` forwards all four `VERTEX_SEARCH_*` vars from the host shell into the g8ep container automatically — no manual `docker exec` or container restart is needed.

### LLM configuration for tests

`ai_integration` tests require explicit CLI flags to enable real LLM calls. Without `--llm-provider`, all `ai_integration` tests are skipped.

| Flag | Short | Description |
|------|-------|-------------|
| `--llm-provider` | `-p` | LLM provider (`gemini`, `openai`, `anthropic`, `ollama`) |
| `--primary-model` | `-m` | Primary model name |
| `--assistant-model` | `-a` | Assistant model name |
| `--llm-endpoint-url` | `-e` | API endpoint URL |
| `--llm-api-key` | `-k` | API key |

Flags are forwarded into the g8ep container as `TEST_LLM_*` env vars. `conftest.py` reads them to build `LLMSettings`, which overrides the g8es platform settings for the test session. No g8es write occurs.

### Running g8ee tests

```bash
./g8e test g8ee
./g8e test g8ee -- -m unit
./g8e test g8ee -- -m integration
./g8e test g8ee -- -m ai_integration
./g8e test g8ee -- -m requires_api   # Vertex AI Search live API tests

# Enable ai_integration tests with Gemini
./g8e test g8ee -p gemini -k AIza...

# Specify models explicitly
./g8e test g8ee -p gemini -k AIza... -m gemini-3.1-pro-preview -a gemini-3.1-flash-lite-preview

# OpenAI with endpoint
./g8e test g8ee -p openai -k sk-... -e https://api.openai.com/v1

# Ollama (no API key needed)
./g8e test g8ee -p ollama -e https://192.168.1.100:11434/v1

# Ollama with thinking-enabled models
./g8e test g8ee -p ollama -e http://10.0.0.1:11434/v1 -m qwen3.5:4b -a qwen3.5:4b -- tests/integration/test_agent_thinking_puzzle_integration.py
```

### Fixture architecture

**`tests/conftest.py` (root shared):**

Unit test fixtures (function scope unless noted):
- `mock_kv_cache_client` — fresh `MockKVClient` instance per test
- `mock_db_client` — fresh `MockDBClient` instance per test; all CRUD methods are `AsyncMock(side_effect=...)` backed by a real in-memory dict store — assert call counts and args directly on `mock_db_client.create_document`, `.get_document`, etc.
- `mock_db_client` — `MagicMock(spec=DBClient)` with async defaults
- `mock_cache` — cache-aside mock (returns `None` on miss by default)
- `mock_cache_aside_service` — real `CacheAsideService` backed by `mock_kv_cache_client` (as `.kv`) and `mock_db_client` (as `.db_client`). **All services that accept a `CacheAsideService` parameter must receive this fixture — never pass `mock_kv_cache_client` directly.** To spy on raw KV operations, access the underlying client via `mock_cache_aside_service.kv` (a `MockKVClient` with `AsyncMock` wrappers on every method). Example: `mock_cache_aside_service.kv.set.assert_awaited_once()`. To inject a fault, replace the method directly: `mock_cache_aside_service.kv.set = AsyncMock(side_effect=Exception("down"))`. When constructing a `CacheAsideService` inline in a test (e.g. for security helpers), use `create_mock_cache_aside_service(kv_cache_client=kv_mock)` where `kv_mock` is a `MockKVClient` you control.
- `mock_storage_client` — `MagicMock` with upload/download/delete async stubs
- `mock_db_service` — real `DBService` backed by `mock_db_client`, cache disabled
- `mock_event_service` — mock with `publish` as `AsyncMock`
- `mock_settings` — `Settings()` (default-initialized settings, not a live DB load)

Domain object fixtures (function scope):
- `enriched_investigation` — `EnrichedInvestigationContext` built by `build_enriched_context()`
- `cloud_operator_doc` — `OperatorDocument` with `OperatorType.CLOUD` / `CloudSubtype.AWS`
- `binary_operator_doc` — `OperatorDocument` with `OperatorType.SYSTEM` and stubbed `OperatorSystemInfo`
- `multi_operator_investigation` — enriched context with both cloud and binary operators
- `provider_config` — `GenerateContentConfig()` instance

Integration test fixtures (session scope) — these connect to real g8es and are available to any test that needs live infrastructure:
- `cache_aside_service` — real session-scoped `CacheAsideService` backed by a live `KVClient` and `DBClient`; required by `test_settings`, `kv_cache_client`, `pubsub_client`, `db_service`
- `test_settings` — `G8eePlatformSettings` from the factory singleton (loaded from g8es during `pytest_configure`); LLM config is monkey-patched via `object.__setattr__` when `TEST_LLM_*` env vars are present — access via `getattr(test_settings, 'llm', None)`
- `kv_cache_client` — real session-scoped `KVClient` connection; `db_client` is an alias
- `pubsub_client` — real session-scoped `KVClient` connection used for pub/sub; `g8es_pubsub_client` is an alias
- `db_service` — real `DBService` backed by a `DBClient` instance and `cache_aside_service`
- `memory_crud` — session-scoped `MemoryDataService` backed by `cache_aside_service`
- `memory_service` — session-scoped `MemoryGenerationService` backed by `memory_crud`

Model config fixtures (function scope unless noted):
- `standard_model_configs` — dict mapping model name strings to `LLMModelConfig` instances: `QWEN3_1B7`, `QWEN3_CODER_30B`, `GEMINI_3_FLASH_PREVIEW`, `GEMMA3_12B`
- `lightweight_model_config` — `QWEN3_1B7`
- `coder_model_config` — `QWEN3_CODER_30B`
- `thinking_model_config` — `GEMINI_3_FLASH_PREVIEW`
- `available_test_models` — result of `get_available_models()` for the current provider
- `mock_model_config_factory` — callable returning a `LLMModelConfig` with overridable fields (`name`, `supports_tools`, `supports_thinking`, `context_window_input`, `context_window_output`, `**kwargs`)

Session output (via `pytest_sessionstart`):
- Loads platform settings from g8es, then monkey-patches any `TEST_LLM_*` env vars (from CLI flags) onto the settings singleton via `object.__setattr__` (since `G8eePlatformSettings` uses `extra="ignore"`, normal attribute assignment is blocked)
- `_show_llm_config` in `run_tests.sh` prints the active LLM configuration before tests run

**`tests/fixtures/` — importable test data builders and stubs**

Import directly in test files. Do not duplicate these — always import from the appropriate module.

| Module | What it provides |
|--------|------------------|
| `agent_streaming` | Agent streaming and SSE test infrastructure (see below) |
| `mocks` | `MockKVClient`, `MockDBClient`, `create_mock_db_client`, `create_mock_cache`, `create_mock_cache_aside_service`, `create_mock_db_service`, `create_mock_g8ed_http_client`, `create_mock_event_service`, `create_mock_llm_provider`, `make_candidate_command`, `_make_file_ops`, `MockWebSearchProvider`, `create_tool_executor`, `create_tool_executor_with_search`, `create_mock_kv_cache_client`, `create_mock_g8eo_svc`, `create_mock_heartbeat_svc`, `create_mock_lifespan_settings` |
| `status` | `status.py` — minimal status fixture helpers |
| `http_context` | `build_g8e_http_context`, `build_bound_operator`, `build_authenticated_user` |
| `headers` | `TEST_G8E_HEADERS` — complete lowercase `X-G8E-*` header dict with stable test values; use wherever a test needs to simulate an inbound request with g8e context headers |
| `cases` | `build_case_model` |
| `investigations` | `build_enriched_context`, `build_investigation_with_operators`, `create_investigation_data`, `create_investigation_request`, `create_conversation_message` |
| `memory` | `create_investigation_for_memory`, `create_memory_document`, `create_single_message_conversation`, `create_multi_turn_conversation` — test data builders for memory service tests |
| `operators` | `build_operator_document`, `build_minimal_operator_document`, `build_operator_heartbeat`, `create_operator_status_info` |


**`tests/fixtures/mocks.py` — additional detail**

- `MockKVClient` — dict-backed in-memory KV store with `AsyncMock` wrappers on every method. Seed helpers: `seed(key, value)`, `seed_json(key, data)`, `seed_model(key, model)` (calls `flatten_for_wire()` if available, else `model_dump_json()`). Use these to pre-populate state without relying on `set()` calls.
- `create_mock_event_service()` — returns a mock with `publish` as `AsyncMock` and `publish_command_event` as `AsyncMock`.
- `MockWebSearchProvider` — test double for `WebSearchProvider`. Requires no GCP credentials; `search()` returns a pre-configured `SearchWebResult`. `build_search_web_grounding` delegates to the real implementation so grounding logic is exercised, not bypassed. `_search_calls` list records all invocations.

**`tests/utils/` — test utilities**

| Module | What it provides |
|--------|------------------|
| `async_mocks` | `SafeToThreadMock`, `SafeWaitForMock`, `AsyncMockContext` — safe mocks for `asyncio.to_thread` and `asyncio.wait_for` that prevent coroutine leaks |

**`tests/utils/async_mocks.py` — safe asyncio mocking**

When patching low-level asyncio functions like `asyncio.to_thread` and `asyncio.wait_for` in tests, care must be taken to avoid coroutine leaks: coroutines created by mocks must be awaited or closed to prevent `RuntimeWarning: "coroutine was never awaited"`.

- `SafeToThreadMock` — mocks `asyncio.to_thread` returning awaitable coroutines. Use `new=SafeToThreadMock(return_value=...)` or `new=SafeToThreadMock(side_effect=...)` with `mock.patch()`.
- `SafeWaitForMock` — mocks `asyncio.wait_for` always awaiting the coroutine to prevent leaks. Use `side_effect=safe_wait.make_side_effect()` with `mock.patch()`.
- `AsyncMockContext` — context manager combining both mocks for common retry-logic testing patterns.

Example usage:
```python
from tests.utils.async_mocks import SafeToThreadMock, SafeWaitForMock

# Simple return value
with mock.patch("path.to.asyncio.to_thread", new=SafeToThreadMock(return_value=pager)):
    result = await provider.search("query")

# With retry logic testing
call_count = 0
async def wait_for_side_effect(coro, timeout):
    nonlocal call_count
    call_count += 1
    if call_count == 1:
        raise asyncio.TimeoutError()
    return pager

safe_wait = SafeWaitForMock(side_effect=wait_for_side_effect)
with mock.patch("path.to.asyncio.wait_for", side_effect=safe_wait.make_side_effect()):
    result = await provider.search("query")
```

**`tests/fakes/` — typed fake implementations**

Typed fakes are concrete implementations of service interfaces used when a `MagicMock` would be too loose. Import from the module directly; do not use via pytest fixtures.

| Module | What it provides |
|--------|------------------|
| `builder.py` | `build_command_service(**kwargs)` — builds a fully-wired `OperatorCommandService` with typed fakes for all dependencies; `build_intent_service(**kwargs)` — builds `OperatorIntentService` with typed fakes |
| `fake_ai_response_analyzer.py` | `FakeAIResponseAnalyzer` — controllable stub for `AIResponseAnalyzer`; set `.file_operation_risk_result` to override the returned `FileOperationRiskAnalysis` |
| `fake_approval_service.py` | `FakeApprovalService` — controllable stub for `ApprovalServiceProtocol`; typed request methods (`request_command_approval`, `request_file_edit_approval`, `request_intent_approval`) accepting Pydantic models; `handle_approval_response(OperatorApprovalResponse)` typed response; records calls to `command_approval_calls`, `file_edit_approval_calls`, `intent_approval_calls`, `approval_responses` |
| `fake_db_service.py` | `FakeDBService` — in-memory DB service stub |
| `fake_event_service.py` | `FakeEventService` — records all published events; inspect via `.events` |
| `fake_execution_service.py` | `FakeExecutionService` — controllable stub for `ExecutionService`; event-driven execution using `ExecutionRegistry` with `wait()`/`complete()` instead of DB polling; `execute()` returns `ExecutionResult` based on internal state; `cancel_command()` publishes cancel request |
| `fake_investigation_service.py` | `FakeInvestigationService` — controllable stub for `InvestigationService` |
| `fake_lfaa_service.py` | `FakeLFAAService` — records all LFAA audit calls |
| `fake_operator_cache.py` | `FakeOperatorCache` — in-memory operator cache stub |
| `fake_pubsub_service.py` | `FakePubSubService` — records all pub/sub publish calls |
| `fake_g8ed_client.py` | `FakeG8edClient` — controllable stub for the g8ed HTTP client |

**`tests/fixtures/agent_streaming` — agent streaming and SSE infrastructure**

Shared building blocks for any test that exercises `g8e agent` streaming, `run_with_sse`, or `_stream_with_tool_loop`. Never copy these into test files — import them.

| Export | What it provides |
|--------|-----------------|
| `make_gen_config(settings, agent_mode, system_instructions)` | `AIRequestBuilder` generation config with a mock function handler |
| `make_agent_stream_context(**kwargs)` | `AgentStreamContext` with sensible defaults; auto-builds `g8e_context` from matching IDs |
| `make_g8ed_event_service()` | Mock g8e event service with `publish` as `AsyncMock` |
| `make_g8e_agent(fn_handler)` | `g8e agent` with `fn_handler` pre-wired for `_stream_with_tool_loop` and `run_with_sse` (`_tool_declarations`, `start/reset_invocation_context` mocks — lifecycle is owned by `run_with_sse`, not `stream_response`; `llm_provider` is passed per-call to `stream_response`, not at construction time) |
| `make_provider_chunk(*, thought, text, thought_signature, tool_calls, finish_reason)` | Minimal fake provider chunk matching the interface `process_provider_turn` and `_stream_with_tool_loop` read |
| `FakeStreamProvider(chunks)` | Provider stub for single-turn scenarios; yields a fixed list from `generate_content_stream_primary` |
| `FakeMultiTurnStreamProvider(chunks_per_call)` | Provider stub for multi-turn function-call loops; yields successive chunk-lists across `generate_content_stream_primary` calls |
| `patch_stream_response(agent, chunks)` | Replaces `agent.stream_response` with an async generator yielding the given `StreamChunk` list |
| `collect_stream_from_model_chunks(agent, context, gen_config, model_name)` | Drives `_stream_with_tool_loop` and returns all yielded `StreamChunkFromModel` objects |
| `run_process_provider_turn(provider_chunks, model_name)` | Drives `process_provider_turn` directly; returns `(stream_chunks, model_response_parts)` |

`tests/fixtures/__init__.py` re-exports everything above — tests may import from either the module directly or from `tests.fixtures`.

### Rules

See [developer.md — g8ee](developer.md#g8ee-pythonfastapi) for constants, models, serialization, service, and error handling rules that apply equally in test code.

- Import enums from `app.constants` — never string literals for status-like fields
- Mock external clients only (HTTP, storage, external DB); keep service code real
- In endpoint integration tests, apply dependency overrides only for pure context/stub seams; clear overrides after teardown
- New integration tests go in `tests/integration/`; use the session-scoped fixtures from `conftest.py` (`cache_aside_service`, `kv_cache_client`, `db_service`) for infrastructure access
- Integration async modules must use a module-level async pytest mark with session loop scope — avoid mixing conflicting per-test asyncio decorators

#### CacheAsideService in unit tests

Every service that declares a `cache_aside_service: CacheAsideService` parameter must receive a real `CacheAsideService` in tests — never a raw `MockKVClient` or bare `AsyncMock`.

The production interface (`kv_get`, `kv_set`, `kv_lrange`, `get_document`, `update_document`, etc.) lives on `CacheAsideService`, not on the underlying KV client. Passing a raw KV client will fail at the first `cache_aside_service.kv_get(...)` call with `AttributeError`.

The correct pattern:

```python
@pytest.fixture
def service(self, mock_cache_aside_service, mock_settings):
    return MyService(mock_cache_aside_service, mock_settings)

async def test_something(self, service, mock_cache_aside_service):
    kv = mock_cache_aside_service.kv          # MockKVClient
    kv.seed_json(some_key, some_data)          # pre-populate store

    result = await service.do_thing(some_key)

    kv.get.assert_called_once_with(some_key)   # spy on underlying KV call
```

To inject a fault:

```python
mock_cache_aside_service.kv.set = AsyncMock(side_effect=Exception("kv down"))
```

For tests that construct a `CacheAsideService` inline (e.g. security helpers like `check_nonce_kv` that accept a `cache_aside_service` argument directly), use `create_mock_cache_aside_service` from `tests.fixtures.mocks`:

```python
from tests.fixtures.mocks import MockKVClient, create_mock_cache_aside_service

kv_mock = MockKVClient()
cache_aside_service = create_mock_cache_aside_service(kv_cache_client=kv_mock)
result = await check_nonce_kv("my-nonce", cache_aside_service)
kv_mock.set.assert_awaited_once()
```

The parameter name is always `cache_aside_service` — never `kv_cache_client` — when a function or class accepts a `CacheAsideService`. Passing a raw KV mock as `kv_cache_client=` is a type error that will fail at runtime.

### Common Pitfalls

#### LLM Provider SSL Hangs
The eval integration tests run against public cloud LLM APIs (Gemini, OpenAI, Anthropic). `g8ep` environment automatically injects CA cert variables (`G8E_SSL_CERT_FILE`, `G8E_SSL_CERT_FILE`, etc.) to point to the platform's self-signed CA cert for internal services. This can poison public cloud TLS handshakes and cause a 60s SSL hang. 

To fix this, the `_sanitize_ssl_env_for_cloud_providers` session-scoped autouse fixture in `tests/integration/evals/conftest.py` strips these environment variables for the test session so public APIs resolve correctly. Internal services still function because `aiohttp_session.py` receives the cert path explicitly via `G8E_SSL_CERT_FILE` which is retained.

#### Aggressive Global Timeouts
By default, `pyproject.toml` configures a global 60s timeout for all pytest executions. This is too aggressive for LLM integration tests (`ai_integration`), which can easily take 90-120 seconds to complete complex reasoning loops. Use `@pytest.mark.timeout(180)` on these test classes to override the global timeout.

#### The `use_enum_values` Footgun
All models inheriting from `G8eBaseModel` are configured with `use_enum_values=True`. This means enum fields (like `status: ExecutionStatus`) are stored and returned as plain `str` objects at runtime, not as enum instances. Do not call `.value` on these fields in service code (e.g., `entry.details.status.value` will raise an `AttributeError`).

### Troubleshooting

- **Async loop errors** — check for missing module-level async marks, conflicting asyncio decorators, or fixture scope mismatches
- **Flaky integration** — check for missing cleanup tracking, shared state leakage, or per-test app clients where module scope is expected
- **AI integration failures** — check `--llm`/`--m` flags, missing API key, network/provider availability

---

## Related Documentation

- [developer.md](developer.md) — Universal code quality rules, constants/model/serialization patterns, service and error handling rules for g8eo, g8ee, and g8ed
- [components/g8ep.md](components/g8ep.md) — g8e node component reference (operator lifecycle, test environment)
- [architecture/security.md](architecture/security.md) — Security architecture and controls
