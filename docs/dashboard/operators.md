# Operator Surfaces

## Scope

The dashboard repository contains browser components for operator discovery and binding, device links, operator deployment and binary download, approvals, status and metrics, and an anchored terminal. These components express the operator experience and remain extensively unit tested.

They do not form an active end-to-end server surface in the current static-host runtime. Most HTTP calls select `ServiceName.g8ed`, which resolves to the dashboard origin, while `server.js` mounts no operator or approval routers. This page documents both the component design and that activation boundary.

## Operator Panel Composition

`OperatorPanel` composes focused mixins rather than implementing the entire workflow in one class:

- Layout and visibility logic owns the panel shell.
- List and metrics mixins render operator inventory, status, and summary data.
- Bind and device-auth mixins coordinate operator session binding and authorization prompts.
- Device-link logic creates, lists, revokes, and deletes enrollment links.
- Download and deployment components select a platform, retrieve an operator artifact, and render setup instructions.

`operator-panel-service.js` centralizes the panel's HTTP calls and returns raw `Response` objects so feature components retain status and payload handling. It supports client injection for unit tests.

## Retained API Calls

The service currently targets dashboard-origin paths for:

| Workflow | Paths |
| --- | --- |
| Bind and unbind | `/api/operators/bind`, `/api/operators/unbind`, `/api/operators/bind-all`, `/api/operators/unbind-all` |
| Operator details and stop | `/api/operators/{id}/details`, `/api/operators/{id}/stop` |
| API key display and rotation | `/api/operators/{id}/api-key`, `/api/operators/{id}/refresh-api-key` |
| Device-link lifecycle | `/api/device-links`, `/api/device-links/{id}` |
| Device authorization | `/api/v1/auth/link/*` built by the legacy g8ed path group |
| Approval response | `/api/operator/approval/respond` |

Route modules implementing related server behavior remain under `dashboard/routes/internal/`, and business services remain under `dashboard/services/operator/`. `createApp()` does not import or mount them. Documentation therefore does not promise these paths as a live g8ed API.

## Anchored Terminal

The terminal is a browser interaction surface assembled from separate controller modules:

- `anchored-terminal.js` owns top-level state and composition.
- `anchored-terminal-execution.js` handles approval and execution lifecycle actions.
- `anchored-terminal-operator.js` tracks the selected or bound operator.
- `anchored-terminal-output.js` renders command and status output.
- `anchored-terminal-scroll.js` controls viewport behavior.

The application event bus opens, minimizes, and maximizes the terminal without coupling the page shell to execution details. Approval cards and execution indicators are loaded from static component templates.

Terminal actions do not execute commands locally in the browser. They produce HTTP requests and await platform events. When the relevant API path is activated, the gateway and bound operator remain responsible for [governance](../architecture/governance.md) and host execution.

## Event Inputs

Operator and terminal components consume typed browser events representing heartbeats, binding changes, approval requests, execution state, and results. `SSEConnectionManager` forwards gateway event envelopes into `EventBus`, and specialized component handlers update rendered state.

The event channel is observational and interactive UI transport; it does not confer execution authority. Approval responses and mutations still require an authenticated gateway session and the platform's [governance checks](../architecture/governance.md).

## Tests

Operator tests cover panel service delegation, binding overlays, deployment state, platform selection, download flow, initial-install overlays, terminal operator state, and terminal execution behavior. They use injected clients and browser mocks rather than a mounted Express API.

## Related

- [Architecture](architecture.md)
- [Gateway Integration](gateway.md)
- [Server-Sent Events](sse.md)
- [Operator Architecture](../architecture/operator.md) — Operator component design, L4 Warden, and L5 Actuator execution boundary
- [Governance Pipeline](../architecture/governance.md) — Five-layer verification pipeline governing operator mutations
- [Protocol Reference](../architecture/protocol.md) — Canonical wire contracts for operator and approval endpoints
- [Connect Apps to Gateway](../guides/connect_apps_to_gateway.md) — Workload enrollment and in-tree component onboarding
- [Ensemble Agents](../ensemble/agents.md) — Ensemble agent hierarchy and Tribunal consensus that produces operator commands
- [Ensemble Governance](../ensemble/governance.md) — Tribunal consensus and governance envelope that authorizes operator commands
