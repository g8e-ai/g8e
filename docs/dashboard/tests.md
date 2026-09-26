# Testing

## Commands

From `dashboard/`:

```bash
npm run lint
npm test
npm run test:coverage
npm run test:watch
npm run test:ui
```

From the repository root:

```bash
make dashboard-lint
make dashboard-boundary-check
make dashboard-test
```

`ci-dashboard` runs lint, boundary check, and the full Vitest suite.

## Test Runner

The dashboard uses Vitest 5. Tests run in a single fork (`pool: 'forks'`, `singleFork: true`). The default environment is Node; browser-focused tests use jsdom through helpers in `test/mocks/`.

`test/setup.js` clears mocks before each test file. Timeouts are 10 seconds for tests and hooks.

The suite currently runs 2107 tests across 60 test files.

## Configuration

`vitest.config.js` defines these aliases:

| Alias | Path |
| --- | --- |
| `@g8ed` | `dashboard/` |
| `@test` | `dashboard/test/` |

V8 coverage includes `public/js/**/*.js` and `services/**/*.js`. It excludes `node_modules/`, `test/`, and config files. Reports are written as text, HTML, and LCOV under `dashboard/coverage/`.

The `g8e-adapter/` tree has its own Vitest configuration and is excluded from the dashboard suite.

## Boundary Guards

`make dashboard-boundary-check` greps dashboard runtime paths for forbidden dashboard-origin platform API patterns:

- `ServiceName.g8ed`
- `/api/operators`
- `cache_aside`
- `operator_slot`
- `VSE_INTERNAL`

Scanned paths: `dashboard/public/`, `dashboard/server.js`, `dashboard/services/`, `dashboard/entrypoint.sh`.

`test/unit/architecture/test_g8ed_gateway_boundary.test.js` asserts:

- `server.js` has no API router mounts
- Orphan server trees are absent (`routes/`, root-level `models/`, `views/`, `middleware/`, `constants/`, `utils/`, and BFF-era `services/` subtrees)
- Production browser JS does not reference `ServiceName.g8ed`
- SSE connects to the Gateway stream URL, not the polling events URL as the primary `EventSource`

## Test Layout

```text
dashboard/test/
├── setup.js
├── fixtures/
├── mocks/                              # Browser, gateway, and fixture helpers
└── unit/
    ├── architecture/                   # Gateway boundary guards
    ├── frontend/                       # Browser SPA and static-host tests
    │   ├── cases/
    │   ├── chat/
    │   ├── models/
    │   ├── operator/
    │   ├── sse/
    │   ├── ui/
    │   └── utils/
    └── services/
        └── infra/                      # Enrollment, startup, and identity loading
```

## Static Host Tests

`test/unit/frontend/server-spa-fallback.unit.test.js` constructs `createApp()` from `server.js` and verifies:

- `/g8e-config.js` injects `window.G8E_GATEWAY_URL`
- Content Security Policy allows the configured Gateway origin
- HTML SPA fallback behavior and rate limiting

Startup enrollment paths are covered in `test/unit/services/infra/`.

## Browser Tests

Browser tests install a controlled DOM, global browser APIs, and service-client doubles. They cover application initialization, component event subscriptions, chat state, message rendering, markdown sanitization, citations, settings, operator workflows, templates, notifications, and timestamp behavior.

HTTP-facing component tests inject or replace `window.serviceClient`. They verify Gateway path construction, payloads, and response handling without a live Gateway.

## SSE Tests

`sse-connection-manager.unit.test.js` uses a fake `EventSource` and controlled timers. It verifies credentialed construction, session switching, envelope dispatch, infrastructure event handling, open and error state, active-state reporting, keepalive expiry, and disconnect cleanup.

Chat handler tests verify feature-specific interpretation of typed SSE payloads.

Polling fallback behavior is implemented in `gateway-sse-polling-fallback.js` and integrated in `sse-connection-manager.js`; dedicated unit tests for the fallback module are not yet present.

## Enrollment Tests

Infrastructure tests in `test/unit/services/infra/` cover:

- P-256 CSR generation and certificate parsing
- Installed identity reuse, expiry, malformed certificate, and missing SPIFFE identity behavior
- New, resumed, and expired-state replacement owner-approved enrollment attempts
- Required gateway and runtime configuration
- Startup load, enrollment fallback, and fail-closed fatal paths
- Native `fetch` interception for platform enrollment request, status, and completion calls

Startup tests inject an enrollment service and fatal callback so fail-closed behavior can be asserted without calling `process.exit()`.

## Verification Scope

Dashboard unit tests do not replace deployment verification. They use doubles for Gateway HTTP and do not prove live CORS, WebAuthn, ensemble upstream routing, or operator connectivity.

For end-to-end deployment verification, use [UX Smoke Test](../guides/ux_smoke_test.md).

## Related

- [Development](devs.md)
- [Architecture](architecture.md)
- [Authentication](auth.md)
- [Server-Sent Events](sse.md)
- [Platform Testing](../devs/tests.md)
- [UX Smoke Test](../guides/ux_smoke_test.md)
