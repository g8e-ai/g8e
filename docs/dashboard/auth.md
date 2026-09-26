# Authentication

## Identity Model

g8ed has separate identities for the browser user and the dashboard container. These credentials are not interchangeable.

| Surface | Credential | Authority | Current use |
| --- | --- | --- | --- |
| Browser | WebAuthn passkey and HttpOnly `g8e_web_session_cookie` | g8e Gateway | Gateway user authentication and authorization for browser-accessible routes |
| Container | ECDSA P-256 certificate and private key issued for the `g8ed` workload | [g8e Gateway PKI](../ensemble/pki.md) | Required startup enrollment; the running static host does not use the identity for outbound requests |

The browser never receives the container certificate or private key. The Express host does not read, validate, or forward the browser session cookie.

## Deployment Requirements

Browser authentication requires all of the following:

- `G8E_GATEWAY_URL` identifies an HTTPS Gateway origin that the user's browser can reach and trust.
- The dashboard origin is an exact `--cors-origin` and `--passkey-rp-origin` on the Gateway.
- `--passkey-rp-id` is the dashboard hostname or a valid registrable parent-domain suffix. It does not include a scheme or port.
- The dashboard runs in a WebAuthn secure context, either HTTPS or the browser's localhost exception for local development.
- The browser supports WebAuthn and allows credentialed cross-origin requests.

The dashboard sends browser requests directly to `G8E_GATEWAY_URL` with credentials included. When the Gateway has one or more allowed cross-origin origins, it permits exact origin matches, allows credentials, and issues the session cookie with `SameSite=None`. Without cross-origin origins, the cookie uses `SameSite=Lax`. The cookie is always `Secure` and therefore is sent only to the Gateway over HTTPS.

Container enrollment separately requires `G8E_GATEWAY_HTTP_URL`, a writable and persistent `G8E_RUNTIME_DIR`, and network access from the container to the Gateway's plain-HTTP bootstrap surface.

## Container Startup Enrollment

The dashboard resolves its workload identity before Express begins listening:

1. It checks for an installed certificate and private key under `G8E_RUNTIME_DIR`.
2. It reuses the certificate when it can parse the certificate, find a URI subject alternative name, and confirm that more than seven days remain before expiry.
3. Otherwise, it resumes an unexpired persisted enrollment request or creates a P-256 key and certificate signing request when no resumable request exists.
4. It submits the request through the Gateway's plain-HTTP bootstrap surface and waits for owner approval. If the Gateway has no owner yet, submission retries for up to 30 minutes while bootstrap completes.
5. After approval, it proves possession of the generated private key, receives the issued credential, and installs the certificate, key, and returned trust bundle.
6. Express starts only after identity loading or enrollment succeeds. Unexpected identity-read failures, denied requests, and enrollment failures stop startup.

The approval page is provided by the Gateway console because the dashboard is not available while its own enrollment is pending. Unexpired pending state survives process restarts so the dashboard can continue the same request and instance identity without generating a new key. When persisted state has expired, the dashboard replaces it with a new request. A denied request remains on disk and requires operator intervention before a new request can be created.

The runtime files are:

| Relative path under `G8E_RUNTIME_DIR` | Purpose | Permission |
| --- | --- | --- |
| `pki/issued/apps/g8ed.crt` | App leaf certificate and returned certificate chain | `0600` |
| `pki/issued/apps/g8ed.key` | App private key | `0600` |
| `pki/trust/hub-bundle.pem` | Returned Gateway trust bundle | `0644` |
| `pki/pending-enrollment/dashboard.json` | Resumable request token, private key, request metadata, and expiry | `0600` |

Installed identity reuse currently checks that the certificate and key files exist, parses the certificate, rejects certificates with 7 days or less remaining, and extracts a URI subject alternative name. It does not verify that the private key matches the certificate, validate the certificate chain against the stored trust bundle, require the trust bundle to exist, or require the URI subject alternative name to equal the exact expected `spiffe://g8e.local/app/g8ed` identity. Enrollment completion parses the returned certificate and requires a URI subject alternative name containing the component name `g8ed`; it does not validate the returned chain, trust bundle, or public-key match before installation. The running static host retains the resolved file paths but does not currently construct an outbound mTLS client from them.

## Browser Session Behavior

On page startup, the dashboard asks the Gateway for the current user. If the Gateway accepts the session cookie, the dashboard also requests the public web-session identifier and keeps the returned user and session metadata in memory for display and event routing. JavaScript cannot read the HttpOnly cookie, and the dashboard does not add bearer tokens, session headers, API keys, or synthetic cookie headers to Gateway requests.

The Gateway creates a session after successful passkey registration or authentication. Sessions expire after 24 hours. On every protected browser request, the Gateway looks up the session, checks its expiry, and verifies that the associated user remains valid. Reloading the dashboard reconstructs local display state from the Gateway; no browser session is persisted in local storage.

## Passkey Ceremonies

The dashboard sign-in flow calls Gateway console passkey routes directly:

1. **Register:** `POST /api/v1/auth/passkeys/console/register/challenge` then `verify` with `options.publicKey` decoding.
2. **Authenticate:** `POST /api/v1/auth/passkeys/console/authenticate/challenge` with a required `user_id`, then `verify`.

The authenticate challenge requires an explicit g8e user ID. The dashboard collects `user_id` from the sign-in form and persists it in `localStorage` under `g8e_user_id` for returning users. Discoverable-credential sign-in without `user_id` is not supported by the Gateway.

First-owner registration remains available while the Gateway has no users. Registration without a user ID is accepted only in that bootstrap state.

For the Gateway's supported browser flow, see [Build a g8e-Compatible Frontend](../guides/build_frontend.md).

## Gateway Route Authorization

The Gateway, rather than Express, applies browser authentication. The console passkey registration and authentication ceremony routes are public Gateway routes because the ceremony itself establishes the browser session; registration without a user ID is accepted only when the Gateway has no users and is limited to the first credential. Logout is also public and safely handles a missing cookie.

After a session exists, the Gateway's browser-session routes validate the cookie and derive the user and web-session IDs from the persisted session. These routes include the current-user and session-info endpoints, passkey management, browser approvals, observe API, ensemble browser proxy paths, and operator list/bind/unbind routes. mTLS-only routes, including workload producers, Operator dispatch commands, administrative APIs, PKI management, and direct governance-envelope submission, are not browser routes and the dashboard static host does not proxy them.

## Logout and Expiry

Logout asks the Gateway to delete the session referenced by the cookie and expire the cookie. The dashboard then disconnects its event client, clears its in-memory user state, and returns to the home route. The Gateway logout route is safe to call when no cookie exists.

A protected request with a missing, unknown, expired, or otherwise invalid session receives an unauthorized response from the Gateway. The dashboard clears local state when its initial session check fails. It also treats a terminal event-stream failure after authentication as session expiry, although an event-stream routing or network failure does not itself prove that the Gateway session expired.

## Security Boundaries

- The Gateway, not the dashboard host, authenticates browser users and authorizes browser-accessible API requests.
- The HttpOnly session cookie remains scoped to the Gateway origin and is not exposed to dashboard JavaScript.
- The workload private key remains in the dashboard runtime volume and is not published through browser configuration or static assets.
- The workload certificate does not grant the browser access to mTLS-only Gateway routes.
- Content Security Policy restricts browser connections to the dashboard origin and configured Gateway origin.
- Dashboard authentication does not bypass governance. Supported operations that enter a governed Gateway path remain subject to the active [five-layer verification pipeline](../architecture/governance.md).

## Related

- [Dashboard Architecture](architecture.md)
- [Gateway Integration](gateway.md)
- [Authentication and Authorization](../architecture/auth.md)
- [PKI and Trust](../ensemble/pki.md)
- [Build a g8e-Compatible Frontend](../guides/build_frontend.md)
- [Connect Apps to Gateway](../guides/connect_apps_to_gateway.md)
