# Authentication

## Two Independent Identity Surfaces

g8ed uses separate identities for browser users and the dashboard container. They do not share credentials or substitute for one another.

| Surface | Credential | Issuer and verifier | Purpose |
| --- | --- | --- | --- |
| Browser | WebAuthn passkey plus HttpOnly `g8e_web_session_cookie` | g8e Gateway | User authentication and browser API authorization |
| Container | ECDSA P-256 app certificate and private key with `spiffe://g8e.local/app/g8ed` | g8e Gateway PKI | Prepared server-to-server mTLS clients |

The browser never receives the container certificate or private key. The container does not receive or forward the browser's session cookie.

## Browser Session Startup

`AuthManager.init()` validates the existing gateway session by requesting the current user and then the public session identifier. A valid response becomes a `WebSessionModel`, updates `webSessionService`, renders the authenticated profile, and emits authentication events. A missing or invalid session clears local UI state and renders the sign-in control.

The cookie is HttpOnly. `ServiceClient` cannot read it and deliberately returns no authentication headers for `ServiceName.GATEWAY`. The browser attaches it because each request uses `credentials: 'include'`.

## Passkey Registration

`AuthManager.startPasskeyRegistration()` implements the browser half of registration:

1. Request a registration challenge from `/api/v1/auth/passkeys/console/register/challenge` with `cli_session_id` set to `browser`.
2. Decode the base64url challenge, user ID, and excluded credential IDs to `ArrayBuffer` values.
3. Call `navigator.credentials.create()` with the gateway options.
4. Serialize the credential response fields back to unpadded base64url.
5. Submit the attestation and gateway-provided user ID to `/api/v1/auth/passkeys/console/register/verify`.
6. Parse the returned user session and request its public web-session ID.

The gateway owns challenge generation, relying-party policy, credential verification, user creation, and session issuance. g8ed only translates between JSON and the browser WebAuthn API.

## Passkey Authentication

`AuthManager.passkeyLogin()` uses a discoverable-credential ceremony:

1. Request `/api/v1/auth/passkeys/console/authenticate/challenge` without a user ID.
2. Decode the challenge and allowed credential IDs.
3. Call `navigator.credentials.get()` and let the browser choose a resident passkey.
4. Recover the user ID from the challenge response or assertion `userHandle`.
5. Submit the serialized assertion to `/api/v1/auth/passkeys/console/authenticate/verify`.
6. Install the returned UI session and fetch the public web-session ID.

A cancelled browser ceremony is reported as a cancellation. Verification failures do not establish local session state.

## Logout and Session Expiry

Logout posts to `/api/v1/auth/logout`, disconnects the active SSE client, clears local session state, and navigates to the dashboard home route. The gateway invalidates and expires the authoritative cookie-backed session.

The dashboard also treats terminal SSE failure after authentication as session expiry. It clears the UI session and emits `AUTH_SESSION_EXPIRED`; the next authenticated request remains subject to gateway validation.

## Container App Enrollment

`AppEnrollmentService` resolves configuration from `G8E_GATEWAY_HTTP_URL` and `G8E_RUNTIME_DIR`. It exposes two explicit paths:

- `loadIdentity()` reads and validates installed material without network access. It rejects missing files, unparsable certificates, missing SPIFFE URI identity, expired certificates, and certificates within the seven-day renewal threshold.
- `enroll()` contacts the gateway's plain-HTTP owner-approved enrollment surface. It resumes persisted pending state when present or generates a P-256 key and CSR, fetches trust material, submits an enrollment request, polls for approval, signs the completion transcript, validates the returned certificate, and installs the identity atomically.

Installed files use the dashboard runtime tree:

| Relative path | Purpose | Permission |
| --- | --- | --- |
| `pki/issued/apps/g8ed.crt` | App leaf certificate and returned chain | `0600` |
| `pki/issued/apps/g8ed.key` | App private key | `0600` |
| `pki/trust/hub-bundle.pem` | Gateway trust bundle | `0644` |
| `pki/pending-enrollment/g8ed.json` | Resumable enrollment state while pending | `0600` |

`server.js` stores the resolved `AppIdentity` before listening. The live static host does not yet use it for outbound requests; the g8eg client classes accept the resulting certificate paths for future server-side wiring.

## Deployment Requirements

- The browser must reach the HTTPS gateway origin configured through `G8E_GATEWAY_URL`.
- The gateway must allow the dashboard origin through its CORS configuration and use matching WebAuthn relying-party settings.
- Cross-origin cookie use requires the gateway's secure cookie and allowed-origin behavior described in [Build a g8e-Compatible Frontend](../guides/build_frontend.md).
- The container must reach the plain-HTTP bootstrap origin in `G8E_GATEWAY_HTTP_URL` and have a writable `G8E_RUNTIME_DIR`.
- Enrollment failure prevents Express from listening.

## Related

- [Authentication & Authorization](../architecture/auth.md)
- [Build a g8e-Compatible Frontend](../guides/build_frontend.md)
- [Connect Apps to Gateway](../guides/connect_apps_to_gateway.md)
- [Gateway Integration](gateway.md)
