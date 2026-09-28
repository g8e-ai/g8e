# Gateway Integration

The dashboard separates static application delivery from Gateway access. A Node.js and Express process serves the browser application over plain HTTP on port 3000, while the browser makes all authentication, session, API, and SSE requests directly to the Gateway over HTTPS. The dashboard does not proxy Gateway traffic or handle server-side authentication.

## Runtime Boundaries

`G8E_GATEWAY_URL` is required before the dashboard starts. The dashboard publishes this browser-facing origin through the `/g8e-config.js` endpoint (served with `Cache-Control: no-cache`) and includes the same origin in the browser Content Security Policy. Browser requests to the configured Gateway are sent with `credentials: 'include'` so the browser can transmit the Gateway-issued HttpOnly session cookie.

The Node.js process is a static application host only. It does not mount Express routers for platform APIs, create bearer tokens, issue API keys, or generate session headers. All feature traffic from [dashboard/public/js/utils/service-client.js](../../dashboard/public/js/utils/service-client.js) uses `ServiceName.GATEWAY` and resolves request paths against `window.G8E_GATEWAY_URL`.

## Browser Capabilities

| Capability | Behavior |
| --- | --- |
| Session restoration | On page load, the dashboard requests the Gateway's current user and public web-session identifier. A valid Gateway-issued cookie restores the in-memory dashboard session. |
| Logout | The dashboard instructs the Gateway to invalidate the session cookie, stops event handling, and clears local session state. |
| Passkey registration and sign-in | Gateway WebAuthn ceremony paths with `options.publicKey` and an explicit `user_id` on authenticate challenge. Returning users may persist `user_id` in `localStorage` under `g8e_user_id`. See [Authentication](auth.md). |
| Server-sent events | Credentialed `EventSource` to `${G8E_GATEWAY_URL}/api/v1/sse/stream` with nested Gateway push envelope normalization. After max reconnect attempts, polling falls back to `GET /api/v1/sse/events`. See [Server-Sent Events](sse.md). |
| Operator list, bind, unbind, stop | Gateway `/api/v1/operators` browser routes. |
| Chat, settings, cases, investigations | Gateway ensemble proxy: `/api/v1/chat`, `/api/v1/settings`, `/api/v1/cases`, `/api/v1/investigations`. |
| Approvals and terminal direct commands | Gateway `/api/v1/operator/approval/respond` and `/api/v1/operator/direct-command` ensemble proxy. |
| Audit log REST | Browser-readable audit endpoints: `/api/v1/audit/events`, `/api/v1/audit/summary`, `/api/v1/audit/verify`. |
| Device links and Operator API keys | Not available from the browser. |
| Operator binary download | Gateway `/.well-known/g8e/bin/{os}/{arch}` and corresponding `.../sha256` paths. |

## Deployment Requirements

A browser deployment uses different network addresses for browser traffic and container internal traffic:

1. Set `G8E_GATEWAY_URL` to the HTTPS Gateway origin reachable from the user's browser (scheme, hostname, and optional port only; no path or trailing slash).
2. Add the dashboard origin to the Gateway with `--cors-origin`. The Gateway allows cross-origin credentialed responses only to configured origins.
3. Add the dashboard origin with `--passkey-rp-origin`, and set `--passkey-rp-id` to the dashboard hostname or a valid registrable parent-domain suffix (no scheme or port).
4. Use a Gateway certificate trusted by the user's browser. The dashboard must run in a WebAuthn secure context (HTTPS or the browser's localhost development exception).
5. Keep the Gateway plain-HTTP health surface (`/api/v1/health` by default) reachable from the dashboard container.
6. Point the Gateway ensemble proxy at the running g8ee service via `--ensemble-upstream-url` or `G8E_ENSEMBLE_URL` (for example `http://ensemble:8000` in Docker Compose). Gateway default is `http://127.0.0.1:8000`.

When configured with cross-origin origins, the Gateway issues its Secure, HttpOnly web-session cookie with `SameSite=None`; without configured origins it uses `SameSite=Lax`. The dashboard host allows browser connections only to itself and `G8E_GATEWAY_URL`; it does not allow WebSocket connections. Browser events use Server-Sent Events (SSE) because the Gateway WebSocket endpoint requires mTLS.

For same-machine development:

```bash
./g8e gw connect http://localhost:3000
```

This validates health, certificate chain, and CORS against the running Gateway.

### Workstation Enrollment Commands

[dashboard/public/js/components/getting-started.js](../../dashboard/public/js/components/getting-started.js) builds binary download and enrollment commands from `window.G8E_GATEWAY_URL` and the page hostname:

- The HTTPS port comes from `G8E_GATEWAY_URL`.
- The hostname in generated commands uses `window.location.hostname`, not the configured Gateway hostname. When the dashboard and Gateway are served under different hostnames, replace the generated hostname or co-host both.
- Binary download URLs use plain HTTP port `8080` (hardcoded in the Gateway).

## Container Startup

[dashboard/entrypoint.sh](../../dashboard/entrypoint.sh) waits for the Gateway health endpoint (`GATEWAY_HEALTH_URL` + `GATEWAY_HEALTH_PATH`) for up to 30 attempts at two-second intervals before starting the Node.js process. If the health check fails after 30 attempts, the entrypoint exits with an error and prevents the dashboard from starting.

A missing `G8E_GATEWAY_URL`, an unreachable health endpoint, or incorrect health path configuration prevents the dashboard from starting.

## Configuration

| Variable | Required | Default | Purpose |
| --- | --- | --- | --- |
| `G8E_GATEWAY_URL` | Yes | None | Browser-reachable HTTPS Gateway origin |
| `GATEWAY_HEALTH_URL` | No | `http://g8eg:8080` | Container-reachable Gateway health endpoint origin |
| `GATEWAY_HEALTH_PATH` | No | `/api/v1/health` | Health check path polled on startup |
| `PORT` | No | `3000` | Dashboard static host port |

The unified Docker deployment publishes the dashboard on host port `G8E_DASHBOARD_PORT` (default `3000`) and constructs `G8E_GATEWAY_URL` from `G8E_HOSTNAME` and `G8E_HTTPS_PORT` (defaults `localhost` and `8443`). It uses the internal Docker `g8eg` network alias for `GATEWAY_HEALTH_URL` and `GATEWAY_HEALTH_PATH`. Configure the dashboard origin on the Gateway with `--cors-origin` and `--passkey-rp-origin`.

## Troubleshooting

- **Container exits before listening**: Verify `GATEWAY_HEALTH_URL` and `GATEWAY_HEALTH_PATH` point to a reachable Gateway health endpoint. Check network connectivity between the dashboard and Gateway containers.
- **CORS errors in browser**: Confirm `--cors-origin` matches the dashboard origin exactly (scheme and port). Run `./g8e gw connect <origin>` to validate CORS.
- **Passkey operations rejected**: Verify Gateway certificate trust, secure-context status (HTTPS or localhost), RP ID, RP origin, and that authentication includes an explicit `user_id`.
- **Events not arriving after successful login**: Confirm `G8E_GATEWAY_URL` in `/g8e-config.js` is correct and trusted by the browser. Verify browser connects to `/api/v1/sse/stream` (not the dashboard port). See [Server-Sent Events](sse.md).
- **Chat or settings fail with upstream errors in Docker**: Verify the Gateway has `G8E_ENSEMBLE_URL` or `--ensemble-upstream-url` pointing at the ensemble service (for example `http://ensemble:8000`).
- **Operator or approval requests return HTML or 404 from port 3000**: The request hit the static dashboard host instead of the Gateway. Verify [dashboard/public/js/utils/service-client.js](../../dashboard/public/js/utils/service-client.js) routing and `window.G8E_GATEWAY_URL`.

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
