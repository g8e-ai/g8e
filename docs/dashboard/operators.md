# Operator Surfaces

## Scope and current status

The dashboard source contains browser components for Operator inventory, binding, device links, deployment and binary-download instructions, approval cards, status and metrics, and an anchored terminal. These are retained UI modules, not an active dashboard control-plane API.

The running dashboard host in `dashboard/server.js` serves static assets, `/g8e-config.js`, and the single-page fallback. `createApp()` does not mount the Operator, approval, device-link, internal, or event routers that remain in the source tree. As a result, the browser's legacy `ServiceName.g8ed` requests resolve to the dashboard origin and are not served by the standard static host. The browser-direct Gateway paths use `ServiceName.GATEWAY` instead; the Operator surfaces have not migrated to those paths.

The dashboard index therefore reports Operator management, approvals, and terminal actions as not operational in the current runtime. Component presence and passing unit tests do not establish that an endpoint is deployed or that the complete browser-to-Gateway path works.

## Operator panel

`OperatorPanel` owns the panel lifecycle, DOM rendering, authentication-state handling, Operator event subscriptions, and aggregated list state. It applies these mixins to its prototype:

- `OperatorLayoutMixin` manages the panel shell, resizing, and visibility.
- `OperatorListMixin` sorts and paginates the inventory, renders Operator cards, selects the metrics source, and exposes API-key, device-link, stop, and bind actions.
- `OperatorMetricsDisplayMixin` renders status, CPU, memory, disk, latency, host details, environment details, and an obfuscated public IP toggle.
- `BindOperatorsMixin` implements confirmation overlays and single or bulk bind/unbind operations.
- `OperatorDeviceAuthMixin` displays pending device authorization prompts, approves or rejects them, and clears expired or completed prompts.
- `OperatorDeviceLinkMixin` creates, lists, revokes, and deletes device-link tokens and displays their claim and expiry state.
- `OperatorDownloadMixin` renders platform and architecture selection, authenticated download instructions, checksum instructions, and device-link generation controls.

The panel recognizes `available`, `unavailable`, `offline`, `bound`, `stale`, `active`, `stopped`, and `terminated` Operator statuses, and distinguishes embedded and remote Operators. It prioritizes the current web session's bound Operators, then other bound Operators, active Operators, stale Operators, and the remaining statuses when it renders the list. The inventory and status state arrives through the browser event bus rather than an initial list request in `OperatorPanel`.

`operator-panel-service.js` centralizes the panel's legacy HTTP calls. It injects a service client for tests, falls back to `window.serviceClient` in the browser, and returns the underlying `Response` after the service client completes a request. The service client sends `ServiceName.g8ed` requests to `window.location.origin` with credentials and legacy session/API-key headers; this is distinct from the Gateway-direct browser client.

## Retained browser requests

The frontend path builders and components currently reference these dashboard-origin paths. They describe retained client contracts, not live endpoints in the static deployment.

| Workflow | Method and path | Browser owner |
| --- | --- | --- |
| Operator inventory and details | Event-bus list updates; `GET /api/operators/{id}/details` for card refresh | `OperatorPanel`, `OperatorListMixin` |
| Bind and unbind | `POST /api/operators/bind`, `/unbind`, `/bind-all`, and `/unbind-all` | `OperatorPanelService`, `BindOperatorsMixin` |
| Stop an Operator | `POST /api/operators/{id}/stop` | `OperatorPanelService`, `OperatorListMixin` |
| API-key display and rotation | `GET /api/operators/{id}/api-key`; `POST /api/operators/{id}/refresh-api-key` | `OperatorPanelService`, `OperatorListMixin` |
| Device-link management | `POST` and `GET /api/device-links`; `DELETE /api/device-links/{id}`; `DELETE /api/device-links/{id}?action=delete` | `OperatorPanelService`, `OperatorDeviceLinkMixin` |
| Device authorization | `POST /api/v1/auth/link/{token}/authorize` or `/reject` | `OperatorPanelService`, `OperatorDeviceAuthMixin` |
| Approval response | `POST /api/operator/approval/respond` with approval, case, investigation, and task identifiers | `AnchoredOperatorTerminal`, `TerminalExecutionMixin` |
| Direct terminal command | `POST /api/operator/approval/direct-command` with the command, execution ID, bound Operator, and web-session context | `AnchoredOperatorTerminal` |
| Operator binary and checksum | `GET /operator/download/{os}/{arch}` and `/operator/download/{os}/{arch}/sha256` | `OperatorDownloadMixin` |
| Drop-key refresh | `POST /api/user/me/refresh-drop-key` | `OperatorDownloadMixin` |

The current download and drop-key code constructs or fetches dashboard-origin paths, so those requests are subject to the same inactive-host limitation. The frontend's device-link authorization path is under `/api/v1/auth`, but it still uses `ServiceName.g8ed` and therefore does not become Gateway-direct merely because the path has a `v1` segment.

Related route implementations are split between `dashboard/routes/operator/` and `dashboard/routes/internal/`; supporting business logic is under `dashboard/services/operator/`. For example, the retained Operator download router validates platform, authenticates the download request, and serves a binary or SHA-256 response, while the approval router requires an authenticated web session and relays approval or direct-command requests downstream. None of these route modules is imported or mounted by `dashboard/server.js`.

## Deployment and download UI

The deployment UI is a browser instruction surface. It offers macOS Intel and Apple Silicon choices and Linux x64, ARM64, and x86 choices, then builds authenticated curl and checksum commands for the selected platform. It displays the session's drop key/API key with visibility and copy controls and can create device links. It does not itself start an Operator or execute a download in the browser; it builds links and commands targeting the dashboard origin.

`OperatorDeployment` is a separate terminal welcome component that loads the `operator-deployment` template. It is a static usage-reference panel and does not implement Operator deployment or binary retrieval.

## Anchored terminal

`AnchoredOperatorTerminal` composes focused controller mixins:

- `anchored-terminal.js` owns the terminal DOM, input, history, attachments, chat handoff, and `/run <command>` dispatch.
- `anchored-terminal-operator.js` tracks the selected or bound Operator and enables input based on Operator events.
- `anchored-terminal-execution.js` renders approval cards, risk information, preparing/executing indicators, command and file-edit results, and terminal approval responses.
- `anchored-terminal-output.js` renders output and thinking content.
- `anchored-terminal-scroll.js` manages viewport behavior.

A normal input is emitted to the chat event bus. `/run <command>` sends the retained direct-command HTTP request only when an Operator is bound and a web session is available. Approval cards send approval or denial through the retained approval route. Execution results are not generated locally: the terminal waits for event-bus events such as command start, output, completion, failure, file-edit status, and intent results.

The application event bus opens, minimizes, and maximizes the terminal without making the browser an execution authority. If the corresponding server path is activated, the authenticated Gateway/VSE and the target Operator remain responsible for routing, governance, verification, execution, and evidence. The terminal UI does not bypass the platform's governance boundary.

## Event inputs and security boundary

`SSEConnectionManager` creates a credentialed `EventSource` for the relative `/api/v1/sse/events` path and parses messages shaped as `{ type, data }` before forwarding the payload under the event type to `EventBus`. Operator components consume those event names for inventory updates, heartbeats, status transitions, bind/unbind changes, device authorization, approval requests, and execution results.

This is the retained client behavior, not a claim that event delivery works in the standard deployment. The relative URL resolves to the dashboard origin, while `server.js` provides no SSE handler or proxy. The current client also expects an event envelope and endpoint behavior that do not match the Gateway's active browser SSE integration; see [Server-Sent Events](sse.md#current-url-resolution-constraint).

Event delivery is telemetry and UI transport. It does not confer execution authority, create governance proofs, or replace authenticated route checks. Any activated approval or mutation path remains subject to its route authentication and the Gateway/Operator governance and execution boundaries described in [Governance](../architecture/governance.md) and [Operator Architecture](../architecture/operator.md).

## Tests and verification

The dashboard unit tests cover the retained service delegation, Operator binding, deployment and platform-selection UI, download instructions, device-link flows, terminal Operator state, terminal execution and approval rendering, and server behavior separately. The tests use injected service clients, browser mocks, JSDOM, and route-level fixtures; they do not prove that `dashboard/server.js` mounts an Operator API or that a deployed dashboard can complete a live Gateway flow.

For the current activation status and deployment prerequisites, use [Dashboard Architecture](architecture.md), [Gateway Integration](gateway.md), and [Server-Sent Events](sse.md). For the component test commands, use [Dashboard Testing](tests.md).

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
