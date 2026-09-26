# Gateway Integration

The dashboard separates static application delivery from Gateway access. Its Node.js and Express process serves the browser application over plain HTTP, while the browser sends authentication, session, API, and SSE requests directly to the Gateway over HTTPS. The dashboard container also enrolls its own workload identity during startup, but that identity is separate from browser authentication.

## Runtime Boundaries

The dashboard requires `G8E_GATEWAY_URL` before it starts. It publishes that browser-facing origin through the no-cache `/g8e-config.js` endpoint, with no hardcoded fallback, and includes the same origin in the browser Content Security Policy. Browser requests to the configured Gateway include credentials so the browser can send the Gateway-issued HttpOnly session cookie; the dashboard does not create bearer tokens, API keys, or replacement session headers for those requests.

The Node.js process is a static application host, not an API proxy. It does not mount Express routers for platform APIs. All feature traffic uses `ServiceName.GATEWAY` in `service-client.js`, which resolves paths against `window.G8E_GATEWAY_URL`.

## Current Browser Capabilities

| Capability | Current behavior |
| --- | --- |
| Session restoration | Operational. On page load, the dashboard asks the Gateway for the current user and public web-session identifier. A valid Gateway cookie restores the in-memory dashboard session. |
| Logout | Operational. The dashboard asks the Gateway to invalidate the cookie-backed session, disconnects event handling, and clears local session state. |
| Passkey registration and sign-in | Operational. The browser calls Gateway console ceremony paths with `options.publicKey` and an explicit `user_id` on authenticate challenge. Returning users may persist `user_id` in `localStorage`. See [Authentication](auth.md). |
| Server-sent events | Operational. The browser opens a credentialed `EventSource` to `${G8E_GATEWAY_URL}/api/v1/sse/stream` and normalizes the nested Gateway push envelope. Polling fallback to `/api/v1/sse/events` is not yet implemented in the first-party SPA. See [Server-Sent Events](sse.md). |
| Operator list, bind, unbind, stop | Operational via Gateway `/api/v1/operators` browser routes (`RouteAuthDual`). |
| Chat, settings, cases, investigations | Operational via Gateway→g8ee ensemble proxy (`/api/v1/chat`, `/api/v1/settings`, `/api/v1/cases`, `/api/v1/investigations`). |
| Approvals and terminal direct commands | Operational via Gateway `/api/v1/operator/approval/*` and `/api/v1/operator/direct-command` proxy paths. |
| Audit log REST | Browser retargeted to Gateway `/api/v1/audit/*`, but those paths currently default to mTLS-only in the Gateway auth registry. Cookie-authenticated audit list may fail until route reclassification. |
| Device links and Operator API keys | Stub-rejected from the browser in `operator-panel-service.js`. UI may still expose controls that return errors. |
| Operator binary download | Operational via Gateway `/.well-known/g8e/bin/{os}/{arch}` paths. |

## Deployment Requirements

A browser deployment uses different addresses for browser traffic and container traffic:

1. Set `G8E_GATEWAY_URL` to the HTTPS Gateway origin reachable from the user's browser. Use an origin only, including the scheme, hostname, and optional port, without a path or trailing slash.
2. Add the exact dashboard origin to the Gateway with `--cors-origin`. Cross-origin Gateway responses allow credentials only for configured origins.
3. Add the dashboard origin with `--passkey-rp-origin`, and set `--passkey-rp-id` to the dashboard hostname or a valid registrable parent-domain suffix. The RP ID does not include a scheme or port.
4. Use a Gateway certificate trusted by the user's browser. The dashboard must run in a WebAuthn secure context, either HTTPS or the browser's localhost development exception.
5. Keep the Gateway plain-HTTP bootstrap surface reachable from the dashboard container for health checks and workload enrollment.
6. For Docker stacks with a separate ensemble service, set `G8E_ENSEMBLE_URL` on the Gateway (for example `http://ensemble:8000`) so browser chat and settings proxy paths reach g8ee.

When cross-origin access is configured, the Gateway issues its Secure, HttpOnly web-session cookie with `SameSite=None`; without configured cross-origin origins, it uses `SameSite=Lax`. The dashboard host allows browser connections only to itself and `G8E_GATEWAY_URL`; it does not allow browser WebSocket connections. Browser events use SSE because the Gateway WebSocket surface requires mTLS.

For same-machine development, `./g8e gw connect http://localhost:3000` validates health, certificate chain, and CORS against the running Gateway.

### Workstation Enrollment Instructions

The dashboard landing page uses `G8E_GATEWAY_URL` to generate binary download and user-enrollment commands. It takes the HTTPS port from that variable, but it replaces the Gateway hostname with the hostname in the dashboard page URL and always uses plain-HTTP port `8080` for the binary download. Deployments that expose the dashboard and Gateway under different hostnames produce incorrect generated commands; users must replace the generated hostname or the deployment must present both surfaces under the dashboard hostname.

## Container Startup

The container entrypoint waits for the Gateway's plain-HTTP health surface for up to 30 attempts at two-second intervals before starting Node.js. The dashboard then loads an existing `g8ed` workload certificate or resumes or creates an owner-approved enrollment through the plain-HTTP bootstrap surface. Enrollment submission retries for up to 30 minutes while the Gateway owner bootstrap completes. A missing `G8E_GATEWAY_URL`, an unavailable health surface, an unexpected identity-read failure, a denied request, or a failed enrollment prevents the static host from starting.

The enrolled workload identity is stored below `G8E_RUNTIME_DIR`. The current static host does not use it for server-to-server Gateway requests, and it never exposes the certificate or private key to the browser. See [Authentication](auth.md#container-startup-enrollment) and [PKI & Trust](../ensemble/pki.md) for the enrollment and certificate lifecycle.

## Configuration

| Variable | Required | Default | Purpose |
| --- | --- | --- | --- |
| `G8E_GATEWAY_URL` | Yes | None | Browser-reachable HTTPS Gateway origin and allowed browser connection destination |
| `G8E_GATEWAY_HTTP_URL` | When enrollment or renewal is needed | None | Container-reachable plain-HTTP Gateway bootstrap origin; the enrollment service fails closed when this is unset |
| `G8E_RUNTIME_DIR` | Yes | None | Writable, persistent root for pending and installed workload identity material; identity resolution fails when this is unset |
| `GATEWAY_HEALTH_URL` | No | `http://g8eg:8080` | Container-reachable plain-HTTP Gateway health origin |
| `GATEWAY_HEALTH_PATH` | No | `/api/v1/health` | Gateway health path polled before dashboard startup |
| `PORT` | No | `3000` | Dashboard static host port |

The unified Docker deployment publishes the dashboard on host port `G8E_DASHBOARD_PORT` (default `3000`) and sets `G8E_GATEWAY_URL` from `G8E_HOSTNAME` and `G8E_HTTPS_PORT` (defaults `localhost` and `8443`). It uses the internal `g8eg` network alias for `G8E_GATEWAY_HTTP_URL` and `GATEWAY_HEALTH_URL`, and persists `G8E_RUNTIME_DIR=/data` in the `g8e-dashboard-data` volume. The Gateway is configured separately with the dashboard origin through `--cors-origin` and `--passkey-rp-origin`.

## Troubleshooting

- If the container exits before the dashboard listens, verify the health URL, runtime directory permissions, workload enrollment status, and all required variables.
- If browser requests fail with CORS errors, confirm that `--cors-origin` exactly matches the dashboard origin, including its scheme and port. Run `./g8e gw connect <origin>` to verify CORS programmatically.
- If the browser rejects passkey operations, confirm the Gateway certificate trust, secure-context status, RP ID, RP origin, and that sign-in supplies an explicit `user_id`.
- If authentication succeeds but no events arrive, confirm `G8E_GATEWAY_URL` in `/g8e-config.js`, Gateway trust, and that the browser connects to `/api/v1/sse/stream` (not the dashboard origin). See [Server-Sent Events](sse.md).
- If chat or settings fail with upstream errors in Docker, confirm the Gateway has `G8E_ENSEMBLE_URL` pointing at the ensemble service.
- If operator, chat, or approval requests return HTML or 404 from port 3000, the request is hitting the static dashboard host instead of the Gateway. Check `service-client.js` routing and `window.G8E_GATEWAY_URL`.

## Related

- [Architecture](architecture.md)
- [Authentication](auth.md)
- [Server-Sent Events](sse.md)
- [Build a g8e-Compatible Frontend](../guides/build_frontend.md)
- [Connect Apps to Gateway](../guides/connect_apps_to_gateway.md), workload enrollment and in-tree component onboarding
- [Unified Docker Stack](../guides/unified_stack.md), Docker Compose deployment for Gateway, Operator, Ensemble, and Dashboard
- [Docker Gateway Guide](../guides/docker_gateway.md), Gateway container deployment and configuration
- [Gateway Architecture](../architecture/gateway.md), Gateway component design, protocol surfaces, and PKI authority
- [Network Architecture](../architecture/network.md), Gateway protocol surfaces, ports, and network topology
- [Protocol Reference](../architecture/protocol.md), canonical wire contracts and Gateway API surfaces
- [Governance Pipeline](../architecture/governance.md), five-layer verification pipeline governing host mutations
- [PKI & Trust](../ensemble/pki.md), platform PKI hierarchy, certificate lifecycle, and workload enrollment
