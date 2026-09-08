# Gateway Integration

The dashboard separates static application delivery from Gateway access. Its Node.js process serves the browser application over HTTP, while the browser sends supported authentication requests directly to the Gateway over HTTPS. The dashboard container also enrolls its own workload identity during startup, but that identity is separate from browser authentication.

## Runtime Boundaries

The dashboard requires `G8E_GATEWAY_URL` before it starts. It publishes that browser-facing origin as runtime configuration, with no hardcoded fallback, and includes the same origin in the browser Content Security Policy. Browser requests to the configured Gateway include credentials so the browser can send the Gateway-issued HttpOnly session cookie; the dashboard does not create bearer tokens, API keys, or replacement session headers for those requests.

The Node.js process is a static application host, not an API proxy. It does not provide live handlers for the retained operator, chat, approval, device-link, audit, settings, console, metrics, system, or documentation requests that the browser still directs to the dashboard origin. Those interfaces remain visible in parts of the application but are not operational in the current runtime.

## Current Browser Capabilities

| Capability | Current behavior |
| --- | --- |
| Session restoration | Operational. On page load, the dashboard asks the Gateway for the current user and public web-session identifier. A valid Gateway cookie restores the in-memory dashboard session. |
| Logout | Operational. The dashboard asks the Gateway to invalidate the cookie-backed session, disconnects event handling, and clears local session state. |
| Passkey registration and sign-in | The browser calls the Gateway directly, but the normal dashboard sign-in flow does not currently supply the user identifier required by the Gateway. Interactive sign-in and first-passkey setup are therefore not operational. See [Authentication](auth.md#current-passkey-limitation). |
| Passkey management | The current dashboard has no active controls for listing or revoking passkeys. |
| Server-sent events | Not operational. The browser uses a relative URL that resolves against the static dashboard host, selects the Gateway polling endpoint instead of its live stream, and expects a different event envelope from the Gateway stream. See [Server-Sent Events](sse.md#current-url-resolution-constraint). |
| Operator, chat, approvals, audit, settings, and console features | These requests target the dashboard origin, where the static host provides no API implementation. They are not operational in the current runtime. |

## Deployment Requirements

A browser deployment uses different addresses for browser traffic and container traffic:

1. Set `G8E_GATEWAY_URL` to the HTTPS Gateway origin reachable from the user's browser. Use an origin only, including the scheme, hostname, and optional port, without a path or trailing slash.
2. Add the exact dashboard origin to the Gateway with `--cors-origin`. Cross-origin Gateway responses allow credentials only for configured origins.
3. Add the dashboard origin with `--passkey-rp-origin`, and set `--passkey-rp-id` to the dashboard hostname or a valid registrable parent-domain suffix. The RP ID does not include a scheme or port.
4. Use a Gateway certificate trusted by the user's browser. The dashboard must run in a WebAuthn secure context, either HTTPS or the browser's localhost development exception.
5. Keep the Gateway plain-HTTP bootstrap surface reachable from the dashboard container for health checks and workload enrollment.

When cross-origin access is configured, the Gateway issues its Secure, HttpOnly web-session cookie with `SameSite=None`; without configured cross-origin origins, it uses `SameSite=Lax`. The dashboard host allows browser connections only to itself and `G8E_GATEWAY_URL`; it does not allow browser WebSocket connections. Browser events use SSE because the Gateway WebSocket surface requires mTLS.

### Workstation Enrollment Instructions

The dashboard landing page uses `G8E_GATEWAY_URL` to generate binary download and user-enrollment commands. It takes the HTTPS port from that variable, but it replaces the Gateway hostname with the hostname in the dashboard page URL and always uses plain-HTTP port `8080` for the binary download. Deployments that expose the dashboard and Gateway under different hostnames produce incorrect generated commands; users must replace the generated hostname or the deployment must present both surfaces under the dashboard hostname.

## Container Startup

The container entrypoint waits for the Gateway's plain-HTTP health surface before starting Node.js. The dashboard then loads an existing `g8ed` workload certificate or enrolls a new one through the plain-HTTP bootstrap surface. A missing `G8E_GATEWAY_URL`, an unavailable health surface, an unexpected identity-read failure, or a failed enrollment prevents the static host from starting.

The enrolled workload identity is stored below `G8E_RUNTIME_DIR`. The current static host does not use it for server-to-server Gateway requests, and it never exposes the certificate or private key to the browser. See [Startup Enrollment](architecture.md#startup-enrollment) and [PKI & Trust](../ensemble/pki.md) for the enrollment and certificate lifecycle.

## Configuration

| Variable | Required | Default | Purpose |
| --- | --- | --- | --- |
| `G8E_GATEWAY_URL` | Yes | None | Browser-reachable HTTPS Gateway origin and allowed browser connection destination |
| `G8E_GATEWAY_HTTP_URL` | When the workload identity requires enrollment or renewal | None | Container-reachable plain-HTTP Gateway bootstrap origin |
| `G8E_RUNTIME_DIR` | Yes | None | Writable, persistent root for pending and installed workload identity material |
| `GATEWAY_HEALTH_URL` | No | `http://g8eg:8080` | Container-reachable plain-HTTP Gateway health origin |
| `GATEWAY_HEALTH_PATH` | No | `/api/v1/health` | Gateway health path polled before dashboard startup |
| `PORT` | No | `3000` | Dashboard static host port |

The unified Docker deployment sets `G8E_GATEWAY_URL` from the browser-reachable hostname and HTTPS port. It uses the internal `g8eg` network alias for `G8E_GATEWAY_HTTP_URL` and `GATEWAY_HEALTH_URL`, and persists `G8E_RUNTIME_DIR` in the dashboard data volume.

## Troubleshooting

- If the container exits before the dashboard listens, verify the health URL, runtime directory permissions, workload enrollment status, and all required variables.
- If browser requests fail with CORS errors, confirm that `--cors-origin` exactly matches the dashboard origin, including its scheme and port.
- If the browser rejects passkey operations, confirm the Gateway certificate trust, secure-context status, RP ID, and RP origin before accounting for the [current dashboard sign-in limitation](auth.md#current-passkey-limitation).
- If authentication succeeds but no events arrive, see the [current event integration constraints](sse.md#current-url-resolution-constraint). The browser currently uses the wrong origin, endpoint, and event envelope for the Gateway stream.
- If operator, chat, approval, audit, settings, or console requests return the SPA document or a not-found response, the request is reaching the static dashboard host. Those retained interfaces have no live dashboard API backend.

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
