# Testing

## Test Runner

The dashboard uses Vitest 4. Tests run in a single fork because parts of the retained service suite historically share gateway key-value state. The default environment is Node; browser-focused tests construct a jsdom-based environment through helpers under `test/mocks/`.

Run the suite from `dashboard/`:

```bash
npm test
```

Run coverage with:

```bash
npm run test:coverage
```

The repository-level equivalent is:

```bash
make dashboard-test
```

## Configuration

`vitest.config.js` defines aliases for the dashboard root and test root, loads `test/setup.js` before each test file, uses one fork, suppresses routine output, and sets ten-second test and hook timeouts. The setup file clears mocks before each test.

V8 coverage includes:

- `public/js/**/*.js`
- `services/**/*.js`

It excludes dependencies, tests, and configuration files. Reports are emitted as text, HTML, and LCOV under `dashboard/coverage/`.

## Test Layout

```text
dashboard/test/
├── setup.js
├── mocks/                              # Browser, gateway, and fixture helpers
└── unit/
    ├── frontend/
    │   ├── cases/                      # Case state and URL behavior
    │   ├── chat/                       # Chat, rendering, models, citations, and SSE handlers
    │   ├── models/                     # Browser model validation
    │   ├── operator/                   # Binding, deployment, download, and terminal behavior
    │   ├── sse/                        # EventSource lifecycle and reconnect logic
    │   ├── ui/                         # Settings and navigation behavior
    │   └── utils/                      # Shared browser services
    └── services/infra/                 # Enrollment, startup, and mTLS client wiring
```

## Static Host Tests

`server-spa-fallback.unit.test.js` constructs the exported Express application and checks static and fallback behavior without running the executable entrypoint. `docs-routes-rate-limiting.unit.test.js` exercises retained route rate-limiting behavior as an isolated module; it does not imply that the route is mounted by `server.js`.

## Browser Tests

Browser tests install a controlled DOM, global browser APIs, and service client doubles. They cover application initialization, component event subscriptions, chat state, message rendering, markdown sanitization, citations, settings, operator workflows, templates, notifications, and timestamp behavior.

HTTP-facing component tests inject or replace `window.serviceClient`. They verify request ownership, path construction, payloads, and response handling without depending on a real gateway.

## SSE Tests

The SSE manager suite provides a fake `EventSource` and controlled timers. It verifies credentialed construction, session switching, event envelope validation, infrastructure event handling, keepalive expiry, reconnect jitter and limits, browser visibility, failure signals, and cleanup. Chat handler tests separately verify the UI interpretation of typed event payloads.

## Enrollment and mTLS Tests

Infrastructure tests cover:

- P-256 CSR generation and certificate parsing.
- Installed identity reuse, expiry, malformed certificate, and missing SPIFFE identity behavior.
- New and resumed owner-approved enrollment attempts.
- Required gateway and runtime configuration.
- Startup load, enrollment fallback, and fail-closed fatal paths.
- HTTP `undici.Agent` construction from certificate paths.
- Pub/sub TLS option construction and preservation during client duplication.

Startup tests inject an enrollment service and fatal callback, allowing fail-closed behavior to be asserted without invoking `process.exit()`.

## Verification Scope

Dashboard unit tests do not prove that retained `ServiceName.g8ed` API paths are mounted. The current runtime mounts no API routers, and component tests use doubles for those calls. End-to-end deployment verification must separately confirm browser CORS and WebAuthn configuration, owner-approved app enrollment, gateway session cookies, and any feature path expected to be live.

## Related

- [Development](development.md)
- [Architecture](architecture.md)
- [Authentication](auth.md)
- [Server-Sent Events](sse.md)
- [Platform Testing](../devs/tests.md) — g8e platform 3-tier test model, test infrastructure, and verification commands
