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
3. Otherwise, it resumes a persisted enrollment request or creates a P-256 key and certificate signing request.
4. It submits the request through the Gateway's plain-HTTP bootstrap surface and waits for owner approval. If the Gateway has no owner yet, submission retries for up to 30 minutes while bootstrap completes.
5. After approval, it proves possession of the generated private key, receives the issued credential, and installs the certificate, key, and returned trust bundle.
6. Express starts only after identity loading or enrollment succeeds. Unexpected identity-read failures, denied or expired requests, and enrollment failures stop startup.

The approval page is provided by the Gateway console because the dashboard is not available while its own enrollment is pending. Pending state survives process restarts so the dashboard can continue the same request without generating a new key. A denied or expired pending request remains on disk and requires operator intervention before a new request can be created.

The runtime files are:

| Relative path under `G8E_RUNTIME_DIR` | Purpose | Permission |
| --- | --- | --- |
| `pki/issued/apps/g8ed.crt` | App leaf certificate and returned certificate chain | `0600` |
| `pki/issued/apps/g8ed.key` | App private key | `0600` |
| `pki/trust/hub-bundle.pem` | Returned Gateway trust bundle | `0644` |
| `pki/pending-enrollment/dashboard.json` | Resumable request token, private key, request metadata, and expiry | `0600` |

Installed identity reuse does not currently verify that the private key matches the certificate, validate the certificate chain against the stored trust bundle, require the trust bundle to exist, or require the URI subject alternative name to equal the expected `g8ed` SPIFFE identity. Enrollment completion checks that the returned certificate contains a URI subject alternative name containing `g8ed`, but it does not perform those stronger validations before installation. The running static host retains the resolved file paths but does not currently construct an outbound mTLS client from them.

## Browser Session Behavior

On page startup, the dashboard asks the Gateway for the current user. If the Gateway accepts the session cookie, the dashboard also requests the public web-session identifier and keeps the returned user and session metadata in memory for display and event routing. JavaScript cannot read the HttpOnly cookie, and the dashboard does not add bearer tokens, session headers, API keys, or synthetic cookie headers to Gateway requests.

The Gateway creates a session after successful passkey registration or authentication. Sessions expire after 24 hours. On every protected browser request, the Gateway looks up the session, checks its expiry, and verifies that the associated user remains valid. Reloading the dashboard reconstructs local display state from the Gateway; no browser session is persisted in local storage.

## Current Passkey Limitation

The current dashboard sign-in control does not complete a new passkey login. It requests an authentication challenge without a user ID in an attempt to use a discoverable credential, while the current Gateway requires a g8e user ID for that request. The request therefore fails before the browser can select a passkey.

The sign-in modal is also intended to offer first-passkey setup when the Gateway reports that the selected user has no passkey. Because the dashboard does not supply a user ID, that response is not reached and the registration form is not exposed through the normal sign-in path. The dashboard source contains a first-owner registration ceremony that can create the initial user only while the Gateway has no users, but the live sign-in flow does not currently invoke it successfully.

As a result, the active browser authentication behavior is limited to restoring and using an already valid Gateway session cookie, then logging that session out. Interactive registration and returning-user sign-in are not operational in the current g8ed interface. For the Gateway's supported browser flow and current user-ID requirement, see [Build a g8e-Compatible Frontend](../guides/build_frontend.md).

## Logout and Expiry

Logout asks the Gateway to delete the session referenced by the cookie and expire the cookie. The dashboard then disconnects its event client, clears its in-memory user state, and returns to the home route. The Gateway logout route is safe to call when no cookie exists.

A protected request with a missing, unknown, expired, or otherwise invalid session receives an unauthorized response from the Gateway. The dashboard clears local state when its initial session check fails. It also treats a terminal event-stream failure after authentication as session expiry, even though an event-stream routing or network failure does not itself prove that the Gateway session expired.

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
