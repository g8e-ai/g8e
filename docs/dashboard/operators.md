# Operator Surfaces

## Scope

The dashboard browser components for Operator inventory, binding, deployment instructions, approval cards, status and metrics, and an anchored terminal call Gateway browser routes directly. The static host in `dashboard/server.js` serves assets only. Operator API traffic uses `ServiceName.GATEWAY` in `operator-panel-service.js` and resolves against `window.G8E_GATEWAY_URL`.

| Surface | Gateway path |
| --- | --- |
| Operator list, bind, unbind, stop, detail | `/api/v1/operators` (`RouteAuthDual`) |
| Approval response and direct terminal command | `/api/v1/operator/approval/respond`, `/api/v1/operator/direct-command` (ensemble proxy) |
| Operator binary download and checksum | `/.well-known/g8e/bin/{os}/{arch}` and `.../sha256` |
| Device links and Operator API keys | Not available from the browser; service methods reject the calls |
| Device authorization prompts | Not available from the browser; service methods reject the calls |

Chat, settings, and ensemble-backed operator workflows require the Gateway ensemble proxy to reach g8ee (`G8E_ENSEMBLE_URL` or `--ensemble-upstream-url` on the Gateway). Operator enrollment and connectivity are separate deployment concerns. See [Connect Operator to Gateway](../guides/connect_operator_to_gateway.md).

## Operator Panel

`OperatorPanel` owns the panel lifecycle, DOM rendering, authentication-state handling, Operator event subscriptions, and aggregated list state. It applies these mixins to its prototype:

- `OperatorLayoutMixin` — panel shell, resizing, and visibility.
- `OperatorListMixin` — inventory sorting and pagination, Operator cards, metrics source selection, bind/unbind/stop actions.
- `OperatorMetricsDisplayMixin` — status, CPU, memory, disk, latency, host details, environment details, and an obfuscated public IP toggle.
- `BindOperatorsMixin` — confirmation overlays and single or bulk bind/unbind operations.
- `OperatorDeviceAuthMixin` — inline device authorization UI (service calls reject when activated).
- `OperatorDownloadMixin` — platform and architecture selection, download instructions, and checksum instructions.

The panel recognizes `available`, `unavailable`, `offline`, `bound`, `stale`, `active`, `stopped`, and `terminated` Operator statuses, and distinguishes embedded and remote Operators. Inventory and status updates arrive through the Gateway SSE stream and the in-browser event bus.

`operator-panel-service.js` centralizes Operator HTTP calls through `window.serviceClient` with `ServiceName.GATEWAY`.

The operator list renders bind and stop actions per Operator. Device link and per-Operator API key actions are not rendered in the list.

## Gateway Browser Paths

| Workflow | Method and path | Browser owner |
| --- | --- | --- |
| Operator inventory | `GET /api/v1/operators` | `OperatorPanelService` |
| Operator detail | `GET /api/v1/operators/{id}` | `OperatorPanelService` |
| Bind and unbind | `POST /api/v1/operators/bind`, `/unbind` | `OperatorPanelService`, `BindOperatorsMixin` |
| Stop an Operator | `POST /api/v1/operators/{id}/stop` | `OperatorPanelService`, `OperatorListMixin` |
| Approval response | `POST /api/v1/operator/approval/respond` | `AnchoredOperatorTerminal`, `TerminalExecutionMixin` |
| Direct terminal command | `POST /api/v1/operator/direct-command` | `AnchoredOperatorTerminal` |
| Operator binary and checksum | `GET /.well-known/g8e/bin/{os}/{arch}` and `.../sha256` | `OperatorDownloadMixin` |

Platform API calls must target `window.G8E_GATEWAY_URL`, not `window.location.origin`.

## Deployment and Download UI

The deployment UI is a browser instruction surface. It offers macOS Intel and Apple Silicon choices and Linux x64, ARM64, and x86 choices, then builds curl and checksum commands for Gateway well-known binary paths.

Device links and Operator API keys are not available from the browser. `operator-panel-service.js` rejects those service calls.

`OperatorDeployment` is a separate terminal welcome component that loads the `operator-deployment` template. It is a static usage-reference panel and does not implement Operator deployment or binary retrieval.

## Anchored Terminal

`AnchoredOperatorTerminal` composes focused controller mixins:

- `anchored-terminal.js` — terminal DOM, input, history, attachments, chat handoff, and `/run <command>` dispatch.
- `anchored-terminal-operator.js` — selected or bound Operator tracking and input enablement from Operator events.
- `anchored-terminal-execution.js` — approval cards, risk information, preparing/executing indicators, command and file-edit results, and terminal approval responses.
- `anchored-terminal-output.js` — output and thinking content.
- `anchored-terminal-scroll.js` — viewport behavior.

A normal input is emitted to the chat event bus. `/run <command>` sends a direct-command Gateway request only when an Operator is bound and a web session is available. Approval cards send approval or denial through the Gateway approval proxy path. Execution results arrive via SSE events on the Gateway stream.

## Event Inputs and Security Boundary

`SSEConnectionManager` creates a credentialed `EventSource` for `${G8E_GATEWAY_URL}/api/v1/sse/stream`, normalizes the nested Gateway push envelope, and forwards application payloads to `EventBus`. Operator components consume those event names for inventory updates, heartbeats, status transitions, bind/unbind changes, approval requests, and execution results.

Event delivery is telemetry and UI transport. It does not confer execution authority, create governance proofs, or replace authenticated route checks. Any approval or mutation path remains subject to Gateway route authentication and the Gateway/Operator governance and execution boundaries described in [Governance](../architecture/governance.md) and [Operator Architecture](../architecture/operator.md).

## Tests

The dashboard unit tests cover service delegation, Operator binding, deployment UI, download instructions, terminal Operator state, terminal execution and approval rendering, static-host behavior, and architecture boundary guards. Tests use injected service clients, browser mocks, and JSDOM; they do not replace live deployment verification.

For deployment prerequisites, see [Dashboard Architecture](architecture.md), [Gateway Integration](gateway.md), and [Server-Sent Events](sse.md). For test commands, see [Dashboard Testing](tests.md).

## Related

- [Dashboard Architecture](architecture.md)
- [Gateway Integration](gateway.md)
- [Server-Sent Events](sse.md)
- [Dashboard Testing](tests.md)
- [Operator Architecture](../architecture/operator.md)
- [Governance Pipeline](../architecture/governance.md)
- [Protocol Reference](../architecture/protocol.md)
- [Connect Operator to Gateway](../guides/connect_operator_to_gateway.md)
- [Unified Docker Stack](../guides/unified_stack.md)
