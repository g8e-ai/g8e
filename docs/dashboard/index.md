# g8ed Dashboard

g8ed is the first-party browser interface for g8e. A minimal Node.js 22 / Express 5 process serves a framework-free JavaScript single-page application, while the browser authenticates and calls the [g8e Gateway](../architecture/gateway.md) directly over HTTPS.

The dashboard source lives in `dashboard/`. The platform-level summary is [Dashboard (g8ed)](../architecture/dashboard.md); this section documents the component itself.

## Documentation

| Document | Description |
| --- | --- |
| [Architecture](architecture.md) | Runtime boundaries, browser composition, source layout, identity separation, and active versus retained surfaces |
| [Authentication](auth.md) | WebAuthn browser sessions and container app workload enrollment |
| [Gateway Integration](gateway.md) | Browser-direct request routing, endpoint ownership, CORS, CSP, and prepared mTLS clients |
| [Server-Sent Events](sse.md) | EventSource lifecycle, event dispatch, keepalive, reconnect behavior, and current routing constraint |
| [Operator Surfaces](operators.md) | Operator panel, binding, deployment, approval, and terminal components, including activation status |
| [Development](development.md) | Local setup, environment, source organization, Docker startup, and coding model |
| [Testing](tests.md) | Vitest configuration, test layout, browser harness, enrollment tests, and verification commands |

## Runtime Summary

1. The container entrypoint waits for the gateway's [plain-HTTP health endpoint](../architecture/network.md).
2. `server.js` requires `G8E_GATEWAY_URL` and resolves the dashboard's `g8ed` [workload identity](../architecture/auth.md) through `AppEnrollmentService`.
3. Express publishes the configured gateway origin through `/g8e-config.js`, sets browser security headers, serves `public/`, and supplies the SPA fallback.
4. The browser loads `window.G8E_GATEWAY_URL`, validates or establishes a gateway web session with a WebAuthn passkey, and sends gateway requests with `credentials: 'include'`.
5. Browser components coordinate through `EventBus`; `SSEConnectionManager` translates gateway event envelopes into component events.

The Express host does not currently mount `dashboard/routes/` or instantiate the prepared [g8eg HTTP and pub/sub clients](../architecture/gateway.md). Code that targets `ServiceName.g8ed` API paths therefore represents retained migration inventory, not an active dashboard backend.

## Related Platform Documentation

- [Platform Architecture](../architecture/overview.md)
- [Platform Overview](../core/about.md) — What g8e is, the application layer, and component boundaries
- [Dashboard (g8ed)](../architecture/dashboard.md)
- [Authentication & Authorization](../architecture/auth.md)
- [SSE Streaming](../architecture/sse.md)
- [Gateway Architecture](../architecture/gateway.md)
- [Network Architecture](../architecture/network.md) — Gateway protocol surfaces, ports, and network topology
- [Protocol Reference](../architecture/protocol.md) — Canonical wire contracts and Gateway API surfaces
- [Governance Pipeline](../architecture/governance.md)
- [Operator Architecture](../architecture/operator.md) — Operator component design, L4 Warden, and L5 Actuator execution boundary
- [Ensemble (g8ee)](../ensemble/index.md) — Agentic ensemble that produces SSE events consumed by the dashboard
- [Build a g8e-Compatible Frontend](../guides/build_frontend.md)
- [Connect Apps to Gateway](../guides/connect_apps_to_gateway.md)
- [Unified Docker Stack](../guides/unified_stack.md)
