# Operator Surfaces

## Scope and current status

The dashboard browser components for Operator inventory, binding, deployment instructions, approval cards, status and metrics, and an anchored terminal are active against Gateway browser routes. The static host in `dashboard/server.js` serves assets only; all Operator API traffic uses `ServiceName.GATEWAY` in `operator-panel-service.js` and resolves against `window.G8E_GATEWAY_URL`.

| Surface | Status | Gateway path |
| --- | --- | --- |
| Operator list, bind, unbind, stop, detail | Active | `/api/v1/operators` (`RouteAuthDual`) |
| Approval response and direct terminal command | Active | `/api/v1/operator/approval/respond`, `/api/v1/operator/direct-command` (ensemble proxy) |
| Operator binary download and checksum | Active | `/.well-known/g8e/bin/{os}/{arch}` |
| Device links and Operator API keys from browser | Stub-reject | No Gateway browser routes; service methods return errors |
| Device authorization prompts | Stub-reject | No Gateway browser routes |

Component presence and passing unit tests do not by themselves prove a live Gateway→Operator path works in every deployment. Verify ensemble upstream configuration (`G8E_ENSEMBLE_URL`) and Operator enrollment separately.

## Operator panel

`OperatorPanel` owns the panel lifecycle, DOM rendering, authentication-state handling, Operator event subscriptions, and aggregated list state. It applies these mixins to its prototype:

- `OperatorLayoutMixin` manages the panel shell, resizing, and visibility.
- `OperatorListMixin` sorts and paginates the inventory, renders Operator cards, selects the metrics source, and exposes stop and bind actions. Device-link and API-key buttons may still render but call stub-rejecting service methods.
- `OperatorMetricsDisplayMixin` renders status, CPU, memory, disk, latency, host details, environment details, and an obfuscated public IP toggle.
- `BindOperatorsMixin` implements confirmation overlays and single or bulk bind/unbind operations.
- `OperatorDeviceAuthMixin` displays pending device authorization prompts (service calls stub-reject when activated).
- `OperatorDeviceLinkMixin` creates, lists, revokes, and deletes device-link tokens in the UI (service calls stub-reject).
- `OperatorDownloadMixin` renders platform and architecture selection, download instructions, checksum instructions, and device-link generation controls (device-link generation stub-rejects).

The panel recognizes `available`, `unavailable`, `offline`, `bound`, `stale`, `active`, `stopped`, and `terminated` Operator statuses, and distinguishes embedded and remote Operators. Inventory and status updates arrive through the Gateway SSE stream and the in-browser event bus.

`operator-panel-service.js` centralizes Operator HTTP calls. It uses `window.serviceClient` with `ServiceName.GATEWAY` for all supported operations.

## Gateway browser paths

| Workflow | Method and path | Browser owner |
| --- | --- | --- |
| Operator inventory | `GET /api/v1/operators` | `OperatorPanelService` |
| Operator detail | `GET /api/v1/operators/{id}` | `OperatorPanelService` |
| Bind and unbind | `POST /api/v1/operators/bind`, `/unbind` | `OperatorPanelService`, `BindOperatorsMixin` |
| Stop an Operator | `POST /api/v1/operators/{id}/stop` | `OperatorPanelService`, `OperatorListMixin` |
| Approval response | `POST /api/v1/operator/approval/respond` | `AnchoredOperatorTerminal`, `TerminalExecutionMixin` |
| Direct terminal command | `POST /api/v1/operator/direct-command` | `AnchoredOperatorTerminal` |
| Operator binary and checksum | `GET /.well-known/g8e/bin/{os}/{arch}` and `.../sha256` | `OperatorDownloadMixin` |

Legacy dashboard-origin paths (`/api/operators`, `/operator/download`, device-link routes) were removed with the BFF excision. The browser must not call `window.location.origin` for platform APIs.

## Deployment and download UI

The deployment UI is a browser instruction surface. It offers macOS Intel and Apple Silicon choices and Linux x64, ARM64, and x86 choices, then builds curl and checksum commands for Gateway well-known binary paths. Device-link and API-key controls may still appear but return errors from the service layer.

`OperatorDeployment` is a separate terminal welcome component that loads the `operator-deployment` template. It is a static usage-reference panel and does not implement Operator deployment or binary retrieval.

## Anchored terminal

`AnchoredOperatorTerminal` composes focused controller mixins:

- `anchored-terminal.js` owns the terminal DOM, input, history, attachments, chat handoff, and `/run <command>` dispatch.
- `anchored-terminal-operator.js` tracks the selected or bound Operator and enables input based on Operator events.
- `anchored-terminal-execution.js` renders approval cards, risk information, preparing/executing indicators, command and file-edit results, and terminal approval responses.
- `anchored-terminal-output.js` renders output and thinking content.
- `anchored-terminal-scroll.js` manages viewport behavior.

A normal input is emitted to the chat event bus. `/run <command>` sends a direct-command Gateway request only when an Operator is bound and a web session is available. Approval cards send approval or denial through the Gateway approval proxy path. Execution results arrive via SSE events on the Gateway stream.

## Event inputs and security boundary

`SSEConnectionManager` creates a credentialed `EventSource` for `${G8E_GATEWAY_URL}/api/v1/sse/stream`, normalizes the nested Gateway push envelope, and forwards application payloads to `EventBus`. Operator components consume those event names for inventory updates, heartbeats, status transitions, bind/unbind changes, approval requests, and execution results.

Event delivery is telemetry and UI transport. It does not confer execution authority, create governance proofs, or replace authenticated route checks. Any approval or mutation path remains subject to Gateway route authentication and the Gateway/Operator governance and execution boundaries described in [Governance](../architecture/governance.md) and [Operator Architecture](../architecture/operator.md).

## Tests and verification

The dashboard unit tests cover service delegation, Operator binding, deployment UI, download instructions, terminal Operator state, terminal execution and approval rendering, static-host behavior, and architecture boundary guards. Tests use injected service clients, browser mocks, and JSDOM; they do not prove every live Gateway flow without deployment verification.

For deployment prerequisites, use [Dashboard Architecture](architecture.md), [Gateway Integration](gateway.md), and [Server-Sent Events](sse.md). For test commands, use [Dashboard Testing](tests.md).

## Related

- [Dashboard Architecture](architecture.md)
- [Gateway Integration](gateway.md)
- [Server-Sent Events](sse.md)
- [Dashboard Testing](tests.md)
- [Operator Architecture](../architecture/operator.md) — Operator component design, L4 Warden, and L5 Actuator execution boundary
- [Governance Pipeline](../architecture/governance.md) — five-layer verification and posture behavior
- [Protocol Reference](../architecture/protocol.md) — canonical platform wire contracts
- [Connect Operator to Gateway](../guides/connect_operator_to_gateway.md) — Operator enrollment and deployment
- [Unified Docker Stack](../guides/unified_stack.md) — current dashboard deployment topology
