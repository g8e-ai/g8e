# Gateway Integration

## Browser-Direct Contract

The active g8ed browser boundary calls the gateway directly. `server.js` injects the configured gateway origin into `/g8e-config.js` as `window.G8E_GATEWAY_URL`; `public/index.html` loads that script before the browser service client and application modules.

`ServiceClient.getServiceEndpoints(ServiceName.GATEWAY)` fails if the injected origin is absent. There is no hardcoded gateway fallback. Every request includes `credentials: 'include'`, an abort timeout, and centralized non-success handling. Gateway requests do not receive synthetic bearer, session, cookie, or API-key headers because browser authentication is the gateway-issued HttpOnly cookie.

## Active Endpoint Ownership

The current browser-direct migration covers these gateway endpoint families:

| Capability | Path family | Browser component |
| --- | --- | --- |
| Current user | `/api/v1/users/me` | `AuthManager` |
| Current web session | `/api/v1/auth/sessions/me` | `AuthManager` |
| Passkey list and revocation | `/api/v1/auth/passkeys` | Authentication and settings UI |
| Passkey registration | `/api/v1/auth/passkeys/console/register/*` | `AuthManager` |
| Passkey authentication | `/api/v1/auth/passkeys/console/authenticate/*` | `AuthManager` |
| Logout | `/api/v1/auth/logout` | `AuthManager` |
| SSE event access | `/api/v1/sse/*` | `SSEConnectionManager` |

Browser path builders live in `public/js/constants/api-paths.js`. They centralize endpoint strings but do not determine the destination origin; callers select either `ServiceName.GATEWAY` or `ServiceName.g8ed`.

## Retained g8ed-Origin Surface

The same path registry retains operator, chat, approval, device-link, audit, settings, setup, console, metrics, system, and documentation paths. Callers for those features commonly select `ServiceName.g8ed`, which resolves to `window.location.origin`.

The running Express application does not mount `dashboard/routes/`; it only serves static assets, `/g8e-config.js`, and the SPA fallback. Consequently, retained `/api/*` calls to the dashboard origin have no live API implementation in the current runtime. The route and service modules in the repository are not part of `createApp()` and are not presented as an active backend contract.

This distinction is architectural rather than cosmetic: documentation and new browser integrations use the gateway-direct surface only where the caller explicitly selects `ServiceName.GATEWAY` or otherwise constructs the gateway origin.

## Cross-Origin Requirements

The dashboard normally runs at an HTTP origin such as `http://localhost:3000`, while the gateway API runs at an HTTPS origin such as `https://localhost:8443`. The gateway configuration must therefore:

- Permit the exact dashboard origin through CORS.
- Allow credentialed requests rather than wildcard-origin responses.
- Configure the WebAuthn RP ID and RP origin for the browser deployment.
- Issue secure cross-origin session cookies with the required SameSite behavior.
- Present a certificate trusted by the user's browser.

The static host's Content Security Policy includes the configured gateway origin in `connect-src`. It does not permit arbitrary network destinations or `ws:`/`wss:` connections.

## Server-Side Gateway Clients

The dashboard container enrolls an mTLS app identity at startup (see [Startup Enrollment](./architecture.md#startup-enrollment)) but `server.js` is a static SPA host and does not construct server-to-server gateway clients. The enrolled identity is resolved and returned by `runStartupEnrollment()`; no module-global holder stores it. See [PKI & Trust](../ensemble/pki.md) for the platform certificate lifecycle that backs the enrollment credential.

## Configuration Surfaces

| Variable | Consumer | Meaning |
| --- | --- | --- |
| `G8E_GATEWAY_URL` | `server.js`, then browser | Browser-reachable HTTPS gateway origin; required with no fallback |
| `G8E_GATEWAY_HTTP_URL` | `AppEnrollmentService` | Container-reachable plain-HTTP enrollment origin; required for startup enrollment |
| `G8E_RUNTIME_DIR` | `AppEnrollmentService` | Writable root for pending and installed app identity material |
| `GATEWAY_HEALTH_URL` | `entrypoint.sh` | Container-reachable plain-HTTP health origin |
| `GATEWAY_HEALTH_PATH` | `entrypoint.sh` | Health endpoint path |
| `PORT` | `server.js` | Static host port; defaults to `3000` |

The browser-facing and container-facing gateway URLs are intentionally separate. In Docker, `G8E_GATEWAY_URL` uses a hostname reachable from the user's browser, while `G8E_GATEWAY_HTTP_URL` and `GATEWAY_HEALTH_URL` use the internal `g8eg` network alias.

## Related

- [Architecture](architecture.md)
- [Authentication](auth.md)
- [Server-Sent Events](sse.md)
- [Build a g8e-Compatible Frontend](../guides/build_frontend.md)
- [Connect Apps to Gateway](../guides/connect_apps_to_gateway.md) — Workload enrollment and in-tree component onboarding
- [Unified Docker Stack](../guides/unified_stack.md) — Docker Compose deployment for Gateway, Operator, Ensemble, and Dashboard
- [Docker Gateway Guide](../guides/docker_gateway.md) — Gateway container deployment and configuration
- [Gateway Architecture](../architecture/gateway.md) — Gateway component design, protocol surfaces, and PKI authority
- [Network Architecture](../architecture/network.md) — Gateway protocol surfaces, ports, and network topology
- [Protocol Reference](../architecture/protocol.md) — Canonical wire contracts and Gateway API surfaces
- [Governance Pipeline](../architecture/governance.md) — Five-layer verification pipeline governing host mutations
- [PKI & Trust](../ensemble/pki.md) — Platform PKI hierarchy, certificate lifecycle, and workload enrollment
