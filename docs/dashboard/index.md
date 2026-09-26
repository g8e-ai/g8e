# g8ed Dashboard

g8ed is the first-party browser interface for g8e. The dashboard host serves a single-page application over plain HTTP. The browser sends all authentication, API, and SSE traffic directly to the [g8e Gateway](../architecture/gateway.md) over HTTPS. The browser user and dashboard container have separate identities and credentials.

The [platform-level dashboard architecture](../architecture/dashboard.md) describes g8ed's role and trust boundaries. This section covers setup, authentication, Gateway connectivity, event delivery, operator surfaces, development, and testing.

## Capabilities

| Capability | Behavior |
| --- | --- |
| Application delivery | Express serves checked-in assets from `public/` and injects `window.G8E_GATEWAY_URL` through `/g8e-config.js`. |
| Browser session restore and logout | Gateway validates and invalidates the HttpOnly `g8e_web_session_cookie`. |
| Passkey registration and sign-in | Gateway console ceremony paths with `options.publicKey` and an explicit `user_id` on authenticate challenge. |
| Container workload enrollment | Required before the host listens. The enrolled `g8ed` certificate is not used for outbound Gateway requests after startup. |
| Server-Sent Events | Credentialed `EventSource` to `${G8E_GATEWAY_URL}/api/v1/sse/stream` with nested envelope normalization; polling fallback to `/api/v1/sse/events` after reconnect exhaustion. |
| Chat, cases, settings, investigations | Gateway→g8ee ensemble proxy paths with browser session auth. |
| Operator list, bind, unbind, stop | Gateway `/api/v1/operators` routes (`RouteAuthDual`). |
| Approvals and terminal commands | Gateway `/api/v1/operator/approval/*` and `/api/v1/operator/direct-command` proxy paths. |
| Audit log | Browser calls Gateway `GET /api/v1/audit/events` and `/verify` (`RouteAuthWebSession`). |
| Operator binary download | Gateway `/.well-known/g8e/bin/{os}/{arch}` paths. |
| Device links and Operator API keys | Not available from the browser; service methods reject the calls. |

The dashboard host does not proxy Gateway traffic or mount platform API routes.

## Start and Connect

1. Start and bootstrap the Gateway with an owner identity.
2. Configure the Gateway with the exact dashboard origin for credentialed CORS and WebAuthn. Ensure the browser trusts the Gateway certificate.
3. Set the dashboard's browser-facing HTTPS Gateway origin (`G8E_GATEWAY_URL`), container-facing plain-HTTP enrollment origin (`G8E_GATEWAY_HTTP_URL`), and persistent runtime directory (`G8E_RUNTIME_DIR`).
4. Start the dashboard. On first startup with an empty runtime directory, it submits a `g8ed` workload enrollment request and waits until an owner approves it through the Gateway console.
5. After approval, the dashboard stores its workload credential under `G8E_RUNTIME_DIR` and listens on plain HTTP (default port `3000`). Terminate TLS in an external proxy or load balancer when the dashboard origin requires HTTPS.

For Docker deployment, see [Unified Docker Stack](../guides/unified_stack.md). For local development, see [Development](devs.md).

Same-machine verification:

```bash
./g8e gw connect http://localhost:3000
```

This checks Gateway health, certificate trust, and CORS against the running dashboard origin.

## Documentation

| Document | Description |
| --- | --- |
| [Architecture](architecture.md) | Runtime boundaries, browser composition, identity separation, and request ownership |
| [Authentication](auth.md) | Browser sessions, passkey ceremonies, hash-fragment flows, and container enrollment |
| [Gateway Integration](gateway.md) | Browser-direct connectivity, cross-origin requirements, configuration, and troubleshooting |
| [Server-Sent Events](sse.md) | Stream lifecycle, envelope normalization, polling fallback, and reconnection |
| [Operator Surfaces](operators.md) | Operator inventory, deployment instructions, approvals, and terminal |
| [Development](devs.md) | Local setup, environment variables, source layout, Docker, and npm scripts |
| [Testing](tests.md) | Vitest configuration, boundary guards, test scope, and verification commands |

## Related Platform Documentation

- [Platform Architecture](../architecture/overview.md)
- [Platform Overview](../core/about.md)
- [Dashboard Architecture](../architecture/dashboard.md)
- [Authentication and Authorization](../architecture/auth.md)
- [SSE Streaming](../architecture/sse.md)
- [Gateway Architecture](../architecture/gateway.md)
- [Network Architecture](../architecture/network.md)
- [Protocol Reference](../architecture/protocol.md)
- [Governance Pipeline](../architecture/governance.md)
- [Operator Architecture](../architecture/operator.md)
- [Ensemble](../ensemble/index.md)
- [Build a g8e-Compatible Frontend](../guides/build_frontend.md)
- [Connect Apps to Gateway](../guides/connect_apps_to_gateway.md)
- [Unified Docker Stack](../guides/unified_stack.md)
- [Documentation Guide](../devs/docs.md)
