# Gateway Integration

The dashboard separates static application delivery from Gateway access. Its Node.js and Express process serves the browser application over plain HTTP, while the browser sends authentication, session, API, and SSE requests directly to the Gateway over HTTPS. The dashboard container enrolls its own workload identity during startup; that identity is separate from browser authentication.

## Runtime Boundaries

The dashboard requires `G8E_GATEWAY_URL` before it starts. It publishes that browser-facing origin through the no-cache `/g8e-config.js` endpoint, with no hardcoded fallback, and includes the same origin in the browser Content Security Policy. Browser requests to the configured Gateway include credentials so the browser can send the Gateway-issued HttpOnly session cookie. The dashboard does not create bearer tokens, API keys, or replacement session headers for those requests.

The Node.js process is a static application host, not an API proxy. It does not mount Express routers for platform APIs. All feature traffic uses `ServiceName.GATEWAY` in `service-client.js`, which resolves paths against `window.G8E_GATEWAY_URL`.

## Browser Capabilities

| Capability | Behavior |
| --- | --- |
| Session restoration | On page load, the dashboard asks the Gateway for the current user and public web-session identifier. A valid Gateway cookie restores the in-memory dashboard session. |
| Logout | The dashboard asks the Gateway to invalidate the cookie-backed session, disconnects event handling, and clears local session state. |
| Passkey registration and sign-in | Gateway console ceremony paths with `options.publicKey` and an explicit `user_id` on authenticate challenge. Returning users may persist `user_id` in `localStorage` under `g8e_user_id`. See [Authentication](auth.md). |
| Server-sent events | Credentialed `EventSource` to `${G8E_GATEWAY_URL}/api/v1/sse/stream` with nested Gateway push envelope normalization. After max reconnect attempts, polling fallback to `GET /api/v1/sse/events`. See [Server-Sent Events](sse.md). |
| Operator list, bind, unbind, stop | Gateway `/api/v1/operators` browser routes (`RouteAuthDual`). |
| Chat, settings, cases, investigations | Gateway→g8ee ensemble proxy (`/api/v1/chat`, `/api/v1/settings`, `/api/v1/cases`, `/api/v1/investigations`). |
| Approvals and terminal direct commands | Gateway `/api/v1/operator/approval/respond` and `/api/v1/operator/direct-command` proxy paths. |
| Audit log REST | `GET /api/v1/audit/events`, `/summary`, `/verify` (`RouteAuthWebSession`). |
| Device links and Operator API keys | Not available from the browser; `operator-panel-service.js` rejects the calls. |
| Operator binary download | Gateway `/.well-known/g8e/bin/{os}/{arch}` and `.../sha256` paths. |

## Deployment Requirements

A browser deployment uses different addresses for browser traffic and container traffic:

1. Set `G8E_GATEWAY_URL` to the HTTPS Gateway origin reachable from the user's browser. Use an origin only, including the scheme, hostname, and optional port, without a path or trailing slash.
2. Add the exact dashboard origin to the Gateway with `--cors-origin`. Cross-origin Gateway responses allow credentials only for configured origins.
3. Add the dashboard origin with `--passkey-rp-origin`, and set `--passkey-rp-id` to the dashboard hostname or a valid registrable parent-domain suffix. The RP ID does not include a scheme or port.
4. Use a Gateway certificate trusted by the user's browser. The dashboard must run in a WebAuthn secure context (HTTPS or the browser's localhost development exception).
5. Keep the Gateway plain-HTTP bootstrap surface reachable from the dashboard container for health checks and workload enrollment.
6. Point the Gateway ensemble proxy at the running g8ee service. Set `--ensemble-upstream-url` or `G8E_ENSEMBLE_URL` (for example `http://ensemble:8000` in Docker Compose). The Gateway default is `http://127.0.0.1:8000`.

When cross-origin access is configured, the Gateway issues its Secure, HttpOnly web-session cookie with `SameSite=None`; without configured cross-origin origins, it uses `SameSite=Lax`. The dashboard host allows browser connections only to itself and `G8E_GATEWAY_URL`; it does not allow browser WebSocket connections. Browser events use SSE because the Gateway WebSocket surface requires mTLS.

For same-machine development:

```bash
./g8e gw connect http://localhost:3000
```

This validates health, certificate chain, and CORS against the running Gateway.

### Workstation Enrollment Commands

The landing page `getting-started.js` module builds binary download and enrollment commands from `window.G8E_GATEWAY_URL` and the page hostname:

- The HTTPS port comes from `G8E_GATEWAY_URL`.
- The hostname in generated commands uses `window.location.hostname`, not the configured Gateway hostname.
- Binary download URLs use plain HTTP port `8080`.

When the dashboard and Gateway are served under different hostnames, replace the generated hostname or serve both surfaces under the same hostname.

## Container Startup

The container entrypoint waits for the Gateway's plain-HTTP health surface for up to 30 attempts at two-second intervals before starting Node.js. The dashboard then loads an existing `g8ed` workload certificate or resumes or creates an owner-approved enrollment through the plain-HTTP bootstrap surface. Enrollment submission retries for up to 30 minutes while the Gateway owner bootstrap completes. A missing `G8E_GATEWAY_URL`, an unavailable health surface, an unexpected identity-read failure, a denied request, or a failed enrollment prevents the static host from starting.

The enrolled workload identity is stored below `G8E_RUNTIME_DIR`. The static host does not use it for server-to-server Gateway requests, and it never exposes the certificate or private key to the browser. See [Authentication](auth.md#container-startup-enrollment) and [PKI & Trust](../ensemble/pki.md).

## Configuration

| Variable | Required | Default | Purpose |
| --- | --- | --- | --- |
| `G8E_GATEWAY_URL` | Yes | None | Browser-reachable HTTPS Gateway origin and allowed browser connection destination |
| `G8E_GATEWAY_HTTP_URL` | When enrollment or renewal is needed | None | Container-reachable plain-HTTP Gateway bootstrap origin; enrollment fails when unset |
| `G8E_RUNTIME_DIR` | Yes | None | Writable, persistent root for pending and installed workload identity material |
| `GATEWAY_HEALTH_URL` | No | `http://g8eg:8080` | Container-reachable plain-HTTP Gateway health origin |
| `GATEWAY_HEALTH_PATH` | No | `/api/v1/health` | Gateway health path polled before dashboard startup |
| `PORT` | No | `3000` | Dashboard static host port |

The unified Docker deployment publishes the dashboard on host port `G8E_DASHBOARD_PORT` (default `3000`) and sets `G8E_GATEWAY_URL` from `G8E_HOSTNAME` and `G8E_HTTPS_PORT` (defaults `localhost` and `8443`). It uses the internal `g8eg` network alias for `G8E_GATEWAY_HTTP_URL` and `GATEWAY_HEALTH_URL`, and persists `G8E_RUNTIME_DIR=/data` in the `g8e-dashboard-data` volume. Configure the dashboard origin on the Gateway with `--cors-origin` and `--passkey-rp-origin`.

## Troubleshooting

- If the container exits before the dashboard listens, verify the health URL, runtime directory permissions, workload enrollment status, and all required variables.
- If browser requests fail with CORS errors, confirm that `--cors-origin` exactly matches the dashboard origin, including scheme and port. Run `./g8e gw connect <origin>` to verify CORS.
- If the browser rejects passkey operations, confirm Gateway certificate trust, secure-context status, RP ID, RP origin, and that sign-in supplies an explicit `user_id`.
- If authentication succeeds but no events arrive, confirm `G8E_GATEWAY_URL` in `/g8e-config.js`, Gateway trust, and that the browser connects to `/api/v1/sse/stream` (not the dashboard origin). See [Server-Sent Events](sse.md).
- If chat or settings fail with upstream errors in Docker, confirm the Gateway has `G8E_ENSEMBLE_URL` or `--ensemble-upstream-url` pointing at the ensemble service (for example `http://ensemble:8000`).
- If operator, chat, or approval requests return HTML or 404 from port 3000, the request is hitting the static dashboard host instead of the Gateway. Check `service-client.js` routing and `window.G8E_GATEWAY_URL`.

## Related

- [Architecture](architecture.md)
- [Authentication](auth.md)
- [Server-Sent Events](sse.md)
- [Build a g8e-Compatible Frontend](../guides/build_frontend.md)
- [Connect Apps to Gateway](../guides/connect_apps_to_gateway.md)
- [Unified Docker Stack](../guides/unified_stack.md)
- [Docker Gateway Guide](../guides/docker_gateway.md)
- [Gateway Architecture](../architecture/gateway.md)
- [Network Architecture](../architecture/network.md)
- [Protocol Reference](../architecture/protocol.md)
- [Governance Pipeline](../architecture/governance.md)
- [PKI & Trust](../ensemble/pki.md)
