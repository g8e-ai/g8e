# g8ed Dashboard

g8ed is the first-party browser interface for g8e. The dashboard host serves a single-page application, while the browser communicates directly with the [g8e Gateway](../architecture/gateway.md) over HTTPS. The browser user and dashboard workload have separate identities and credentials.

The [platform-level dashboard architecture](../architecture/dashboard.md) describes g8ed's role and trust boundaries. This section covers dashboard setup, authentication, Gateway connectivity, event delivery, feature status, development, and testing.

## Current Runtime Status

| Capability | Status |
| --- | --- |
| Application delivery and browser-facing Gateway configuration | Active. The dashboard host serves the application and supplies the configured Gateway origin. |
| Existing browser session restoration and logout | Active. The Gateway validates and invalidates its HttpOnly browser session cookie. |
| Interactive passkey registration and sign-in | Active. Registration and sign-in use Gateway console ceremony paths with `options.publicKey` and an explicit `user_id` on authenticate challenge. |
| Dashboard workload enrollment | Active and required before the dashboard begins serving. The running host does not use the resulting workload credential after startup. |
| Server-Sent Events | Active via Gateway `/api/v1/sse/stream` (absolute URL, nested envelope normalization). |
| Chat, cases, Operator management, approvals, settings, and terminal actions | Active via Gateway browser routes and Gateway→g8ee proxy paths. |

The active dashboard is a static browser application that routes all API, SSE, and passkey traffic directly to the Gateway. The dashboard host does not proxy or implement platform APIs. Individual features may still have product gaps (for example, audit REST paths require Gateway route reclassification, and device-link UI remains visible but stub-rejects).

## Start and Connect

1. Start and bootstrap the Gateway with an owner identity.
2. Configure the Gateway to allow the exact dashboard origin for credentialed cross-origin requests and WebAuthn, and ensure the browser trusts the Gateway certificate.
3. Configure the dashboard's browser-facing HTTPS Gateway origin, container-facing plain-HTTP enrollment origin, and persistent runtime directory.
4. Start the dashboard. On first startup, it requests a `g8ed` workload identity and remains unavailable until an owner approves the request through the Gateway console.
5. After approval, the dashboard stores its workload credential in the persistent runtime directory and begins serving over plain HTTP. Use an external proxy or load balancer when the dashboard origin requires HTTPS.

See [Unified Docker Stack](../guides/unified_stack.md) for the deployment procedure and [Development](devs.md) for local setup. For same-machine deployments, `./g8e gw connect http://localhost:3000` validates CORS, health, and certificate configuration before opening the app.

## Documentation

| Document | Description |
| --- | --- |
| [Architecture](architecture.md) | Runtime boundaries, browser composition, identity separation, and current feature activation |
| [Authentication](auth.md) | Browser session behavior, Gateway-direct passkey ceremonies, and dashboard workload enrollment |
| [Gateway Integration](gateway.md) | Browser-direct connectivity, cross-origin requirements, configuration, and request ownership |
| [Server-Sent Events](sse.md) | Gateway stream lifecycle, envelope normalization, authentication, and reconnection behavior |
| [Operator Surfaces](operators.md) | Operator, deployment, approval, and terminal interfaces via Gateway browser routes |
| [Development](devs.md) | Local setup, environment, source organization, Docker startup, and development model |
| [Testing](tests.md) | Vitest configuration, test scope, browser harness, enrollment tests, and verification commands |

## Related Platform Documentation

- [Platform Architecture](../architecture/overview.md)
- [Platform Overview](../core/about.md): Application layers and component boundaries
- [Dashboard Architecture](../architecture/dashboard.md)
- [Authentication and Authorization](../architecture/auth.md)
- [SSE Streaming](../architecture/sse.md)
- [Gateway Architecture](../architecture/gateway.md)
- [Network Architecture](../architecture/network.md): Gateway protocol surfaces, ports, and network topology
- [Protocol Reference](../architecture/protocol.md): Canonical wire contracts and Gateway API surfaces
- [Governance Pipeline](../architecture/governance.md)
- [Operator Architecture](../architecture/operator.md): Operator design and the host execution boundary
- [Ensemble](../ensemble/index.md): First-party application that produces browser-visible events
- [Build a g8e-Compatible Frontend](../guides/build_frontend.md)
- [Connect Apps to Gateway](../guides/connect_apps_to_gateway.md)
- [Unified Docker Stack](../guides/unified_stack.md)
- [Documentation Guide](../devs/docs.md): Repository-wide audit, ownership, generation, cross-linking, and versioning rules
