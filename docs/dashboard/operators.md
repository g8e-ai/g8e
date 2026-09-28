---
doc_id: operators
title: Operator Dashboard Surfaces
audience: dashboard developers and operators
status: current
last_updated: 2026-09-28
version: v2.2.3
owners:
  - dashboard/public/js/components/operator-panel.js
  - dashboard/public/js/components/anchored-terminal.js
  - dashboard/public/js/utils/operator-panel-service.js
related:
  - docs/dashboard/architecture.md
  - docs/dashboard/gateway.md
  - docs/dashboard/sse.md
  - docs/dashboard/tests.md
  - docs/architecture/operator.md
  - docs/architecture/governance.md
when_to_read: Understanding Operator inventory, binding, metrics, terminal dispatch, and event flows in the browser dashboard.
do_not_use_for:
  - Operator CLI architecture (docs/architecture/operator.md)
  - Governance enforcement and approval routing (docs/architecture/governance.md)
  - Network and enrollment flows (docs/guides/connect_operator_to_gateway.md)
---

# Operator Dashboard Surfaces

## Purpose

Describes how the dashboard browser components expose Operator inventory, binding, terminal execution, and event subscription through the Gateway. Covers component responsibilities, route ownership, mixin structure, and the security boundary between event telemetry and execution authority.

## Quick index

- [Purpose](#purpose)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

## Invariants

| ID | Rule |
| --- | --- |
| INV-OPS-PANEL-01 | `OperatorPanel` in [dashboard/public/js/components/operator-panel.js](../../dashboard/public/js/components/operator-panel.js) composes its behavior through mixin prototypes: `OperatorLayoutMixin`, `OperatorListMixin`, `OperatorMetricsDisplayMixin`, `BindOperatorsMixin`, `OperatorDeviceAuthMixin`, and `OperatorDownloadMixin`. Each mixin file MUST live in `dashboard/public/js/components/` with the suffix `-mixin.js`. |
| INV-OPS-PANEL-02 | `AnchoredOperatorTerminal` in [dashboard/public/js/components/anchored-terminal.js](../../dashboard/public/js/components/anchored-terminal.js) composes focused behavior through terminal-specific mixins: `TerminalScrollMixin`, `TerminalOperatorMixin`, `TerminalOutputMixin`, and `TerminalExecutionMixin`. Each mixin file MUST live in `dashboard/public/js/components/` with the prefix `anchored-terminal-` and suffix `.js`. |
| INV-OPS-PANEL-03 | `OperatorPanel` recognizes eight canonical statuses (from [dashboard/public/js/constants/operator-constants.js](../../dashboard/public/js/constants/operator-constants.js)): `available`, `unavailable`, `offline`, `bound`, `stale`, `active`, `stopped`, `terminated`. Status updates arrive via the event bus from Gateway SSE events. |
| INV-OPS-PANEL-04 | Device links and Operator API keys MUST be rejected at the service layer with `Promise.reject()` in [dashboard/public/js/utils/operator-panel-service.js](../../dashboard/public/js/utils/operator-panel-service.js) when called from the browser. |
| INV-OPS-PANEL-05 | All Operator HTTP calls from `OperatorPanelService` target `window.G8E_GATEWAY_URL`, not `window.location.origin`. Service method routing uses `ServiceName.GATEWAY` with endpoints from [dashboard/public/js/constants/api-paths.js](../../dashboard/public/js/constants/api-paths.js). |
| INV-OPS-PANEL-06 | Event delivery through `SSEConnectionManager` (in [dashboard/public/js/utils/sse-connection-manager.js](../../dashboard/public/js/utils/sse-connection-manager.js)) does NOT confer execution authority. Events are UI transport only. Approval and state-mutation endpoints remain subject to separate Gateway route authentication. |

## Owned surfaces

| Component | Path | Verify |
| --- | --- | --- |
| OperatorPanel component | `dashboard/public/js/components/operator-panel.js` | Class definition, mixin imports at EOF, event handler setup in `_setupWireListeners()` |
| Operator service layer | `dashboard/public/js/utils/operator-panel-service.js` | Service methods target `ApiPaths.operator.*` via `ServiceName.GATEWAY`; device link and API key methods reject |
| AnchoredOperatorTerminal | `dashboard/public/js/components/anchored-terminal.js` | Terminal class, mixin application at EOF, `/run <command>` dispatch in `executeCommand()` |
| Operator statuses | `dashboard/public/js/constants/operator-constants.js` | `OperatorStatus` enum with eight values |
| API path constants | `dashboard/public/js/constants/api-paths.js` | `ApiPaths.operator.*` and `ApiPaths.approval.*` builders |
| Event manager | `dashboard/public/js/utils/sse-connection-manager.js` | `SSEConnectionManager` class, event normalization, reconnect logic |

## Procedures

### Tracing an Operator Route from Browser to Gateway

1. Identify the browser user action (inventory load, bind, direct command) and its owning component (`OperatorPanel` or `AnchoredOperatorTerminal`).
2. Locate the mixin or core method that emits the request (e.g., `bindOperator()` in `BindOperatorsMixin`).
3. Verify the service call uses `OperatorPanelService` and the correct path from `ApiPaths.operator.*` or `ApiPaths.approval.*`.
4. Confirm the service method resolves to `window.G8E_GATEWAY_URL` and uses `ServiceName.GATEWAY` for routing.
5. Cross-check the Gateway route in [internal/services/gateway/operator_controller.go](../../internal/services/gateway/operator_controller.go) for the matching HTTP handler (e.g., `handleBindOperators()` for `POST /api/v1/operators/bind`).
6. Verify the Gateway handler obtains the user ID from the request context (web-session cookie or mTLS CLI identity).

### Testing Operator Component Behavior

1. Run dashboard unit tests: `npm test` or `npm run test:watch` in the `dashboard/` directory.
2. Tests for `OperatorPanel` cover service delegation, mixin initialization, inventory sorting, bind/unbind overlays, and metrics rendering. See [dashboard/test/unit/frontend/operator/](../../dashboard/test/unit/frontend/operator/).
3. Tests for `AnchoredOperatorTerminal` cover Operator state tracking, terminal input dispatch, approval card rendering, and execution result handling.
4. Injected service clients and JSDOM mocks prevent real Gateway calls; test assertions verify mixin method calls and DOM updates.

## Anti-patterns

- Hardcoding a route path instead of using `ApiPaths.operator.*` — all paths MUST come from the constants file (INV-OPS-PANEL-05).
- Calling device link or API key methods without catching the rejection — these are intentionally stubbed to fail in the browser (INV-OPS-PANEL-04).
- Assuming event delivery creates execution permission — events are telemetry and UI sync only; every mutation endpoint requires its own auth check (INV-OPS-PANEL-06).
- Adding new Operator mixin behavior directly to `OperatorPanel` class instead of creating a focused mixin module — mixins keep concerns isolated (INV-OPS-PANEL-01).
- Dispatching commands through `AnchoredOperatorTerminal` without an active Operator binding or web session — the terminal checks both conditions before sending (INV-OPS-PANEL-02).

## Links out

- [Dashboard Architecture](architecture.md): Component hierarchy and initialization.
- [Gateway Integration](gateway.md): HTTPS origin configuration, CORS, and credential handling.
- [Server-Sent Events](sse.md): Event stream, reconnect fallback, and payload envelope normalization.
- [Dashboard Testing](tests.md): Test structure, mocks, and coverage gates.
- [Operator Architecture](../architecture/operator.md): Operator lifecycle, session binding, and remote/embedded types.
- [Governance Pipeline](../architecture/governance.md): Approval routing, execution authority, and consent boundaries.
