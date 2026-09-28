---
doc_id: dashboard-auth
title: Dashboard Authentication
audience: platform developers deploying or extending g8ed, platform security reviewers
status: current
last_updated: 2026-09-28
version: v2.2.3
owners:
  - dashboard/public/js/components/auth.js
  - dashboard/services/infra/app-enrollment-service.js
  - docs/dashboard/auth.md
related:
  - docs/dashboard/architecture.md
  - docs/dashboard/gateway.md
  - docs/dashboard/devs.md
  - docs/architecture/auth.md
  - docs/architecture/network.md
  - docs/guides/build_frontend.md
  - docs/ensemble/pki.md
when_to_read: Building a frontend for g8e, deploying the g8ed dashboard in a new environment, understanding WebAuthn and workload enrollment flows, or debugging authentication issues.
do_not_use_for:
  - Dashboard host development (docs/dashboard/devs.md)
  - Browser SPA component architecture (docs/dashboard/architecture.md)
  - Gateway CORS and security headers (docs/dashboard/gateway.md)
  - Platform-wide identity models (docs/architecture/auth.md)
---

# Dashboard Authentication

## Purpose

Describes how the g8ed dashboard establishes browser user identities via WebAuthn passkeys and workload identities via owner-approved platform enrollment. The dashboard does not authenticate requests itself; all authentication is delegated to the Gateway. This document covers the distinct credential models, deployment prerequisites, enrollment workflows, session handling, and security boundaries.

## Quick index

- [Purpose](#purpose)
- [Quick index](#quick-index)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

Sections: [Identity Model](#identity-model), [Deployment Requirements](#deployment-requirements), [Container Startup Enrollment](#container-startup-enrollment), [Browser Session Behavior](#browser-session-behavior), [Passkey Ceremonies](#passkey-ceremonies), [URL Hash Fragments](#url-hash-fragments), [Gateway Route Authorization](#gateway-route-authorization), [Logout and Expiry](#logout-and-expiry), [Security Boundaries](#security-boundaries).

## Invariants

### Browser authentication (`INV-DASHBOARD-BROWSER-AUTH`)

| ID | Rule |
| --- | --- |
| INV-DASHBOARD-BROWSER-AUTH-01 | Browser users authenticate via WebAuthn passkeys (`POST /api/v1/auth/passkeys/console/register/challenge`, `verify`, `authenticate/challenge`, `verify`) to the Gateway at `window.G8E_GATEWAY_URL` with `credentials: 'include'`. The authenticate challenge requires an explicit `user_id`. |
| INV-DASHBOARD-BROWSER-AUTH-02 | The Gateway issues a Secure, HttpOnly `g8e_web_session_cookie` after successful passkey registration or authentication. Sessions expire in 24 hours. The browser never receives or can read the cookie value; it is sent automatically on all subsequent requests. |
| INV-DASHBOARD-BROWSER-AUTH-03 | Browser deployment requires `G8E_GATEWAY_URL` (HTTPS origin), `--cors-origin` (exact match of the dashboard origin), and `--passkey-rp-id` (dashboard hostname or parent domain suffix) configured on the Gateway. The dashboard must run in a WebAuthn secure context (HTTPS or localhost exception). |
| INV-DASHBOARD-BROWSER-AUTH-04 | When the Gateway has one or more allowed cross-origin origins configured, the session cookie uses `SameSite=None`; otherwise it uses `SameSite=Lax`. The cookie is always `Secure` and sent only over HTTPS to the Gateway origin. |

### Workload identity and enrollment (`INV-DASHBOARD-WORKLOAD-ENROLL`)

| ID | Rule |
| --- | --- |
| INV-DASHBOARD-WORKLOAD-ENROLL-01 | The dashboard generates an ECDSA P-256 key and certificate signing request (CSR) at startup via the enrollment service and submits it to `G8E_GATEWAY_HTTP_URL` (plain HTTP). The service loads a previously issued certificate if it exists, parses correctly, has > 7 days of validity remaining, and contains a URI subject alternative name. |
| INV-DASHBOARD-WORKLOAD-ENROLL-02 | The enrollment request submission retries with bounded exponential backoff for up to 30 minutes to allow the Gateway bootstrap window before owner approval is available. Pending enrollment state (token, private key, request ID, CSR fingerprint, expiry) is persisted atomically with `0600` permissions and survives process restarts. |
| INV-DASHBOARD-WORKLOAD-ENROLL-03 | After owner approval, the dashboard signs a completion transcript with the private key, submits it, validates the response (URI SAN containing `g8ed`, returned trust bundle), and installs certificates atomically. The Express server does not listen until enrollment succeeds or identity loading succeeds; startup failures exit with status 1. |
| INV-DASHBOARD-WORKLOAD-ENROLL-04 | Installed identity reuse does not verify that the private key matches the certificate, validate the certificate chain against the trust bundle, require the trust bundle to exist, or enforce the URI SAN format `spiffe://g8e.local/app/g8ed`. Enrollment completion parsing requires a URI SAN containing the component name `g8ed` but does not validate the chain or public-key match before installation. |

### Session and cookie behavior (`INV-DASHBOARD-SESSION`)

| ID | Rule |
| --- | --- |
| INV-DASHBOARD-SESSION-01 | On page load, the dashboard validates its session by calling the Gateway's current-user endpoint with the session cookie. Session metadata is held in memory (not persisted to local storage). Reloading the dashboard reconstructs display state from the Gateway. |
| INV-DASHBOARD-SESSION-02 | The Gateway returns `401` for missing, unknown, expired, or invalid session cookies. The dashboard clears in-memory state on initial validation failure. Terminal event-stream failures after authentication are treated as session expiry, though transient network failures do not prove session loss. |
| INV-DASHBOARD-SESSION-03 | Logout deletes the server-side session and expires the cookie. The dashboard disconnects the event client, clears in-memory user state, and returns to the home route. The logout endpoint is safe to call with a missing or invalid cookie. |

### Hash fragment routing (`INV-DASHBOARD-HASH-ROUTING`)

| ID | Rule |
| --- | --- |
| INV-DASHBOARD-HASH-ROUTING-01 | URL hash fragments (`#token=`, `#enroll=1&token=`, `#recovery=`, `#platform-enrollment=`, `#approve=`) are parsed and cleared via `history.replaceState` after authentication is validated. Fragment-triggered flows (passkey enrollment, recovery approval, platform enrollment decision, transaction approval) require an existing session or queue their actions until authentication succeeds. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Browser authentication flow | `dashboard/public/js/components/auth.js` | AuthManager class, passkey ceremony routes, session validation |
| Workload enrollment service | `dashboard/services/infra/app-enrollment-service.js` | Load-then-enroll logic, CSR generation, P-256 key, request submission, approval polling, completion validation |
| API path builders | `dashboard/public/js/constants/api-paths.js` | Passkey challenge/verify routes, console routes, auth paths |
| Session model and service | `dashboard/public/js/models/session-model.js`, `dashboard/public/js/utils/web-session-service.js` | Session metadata persistence in memory |
| Startup entrypoint | `dashboard/server.js` | `runStartupEnrollment()` export, enrollment-before-listen guarantee |
| Environment variables | `dashboard/package.json`, `dashboard/Dockerfile`, `dashboard/entrypoint.sh` | G8E_GATEWAY_URL, G8E_GATEWAY_HTTP_URL, G8E_RUNTIME_DIR requirements |
| Credential file paths | `dashboard/services/infra/app-enrollment-service.js` | Paths under G8E_RUNTIME_DIR: `pki/issued/apps/g8ed.*`, `pki/trust/hub-bundle.pem`, `pki/pending-enrollment/dashboard.json` |

## Procedures

### Deploy dashboard with browser authentication

Browser authentication requires:
1. A bootstrapped Gateway accessible at `G8E_GATEWAY_URL` (HTTPS, no localhost without cert).
2. The dashboard origin configured as an exact `--cors-origin` on the Gateway.
3. The dashboard hostname (or a parent domain suffix) configured as `--passkey-rp-id` on the Gateway.
4. The dashboard running in a secure context (HTTPS or `localhost` with browser's localhost exception).

Verify connectivity:
```bash
# From the browser, verify window.G8E_GATEWAY_URL is set and reachable
curl -k https://<G8E_GATEWAY_URL>/api/v1/health
```

On first user access, the browser redirects to the Gateway's passkey registration page. After registration completes, the Gateway sets the session cookie and redirects back to the dashboard.

### Enroll the dashboard workload at startup

The dashboard enrollment service runs during server startup before Express listens:

1. **Load existing identity** (if available):
   - Reads `pki/issued/apps/g8ed.crt` and `g8ed.key` from `G8E_RUNTIME_DIR`.
   - Parses the certificate and checks expiry (reuses only if > 7 days remain).
   - Extracts the URI subject alternative name and uses it as `app_id`.
   - On success, Express starts and the process continues.

2. **Enroll (if load fails or cert is near expiry)**:
   - Generates ECDSA P-256 key and CSR (component name: `g8ed`).
   - Submits enrollment request to `G8E_GATEWAY_HTTP_URL/api/v1/auth/platform-enrollments/request`.
   - Retries on 403 "platform enrollment requires a bootstrapped gateway" for up to 30 minutes.
   - Persists pending state (token, key, request ID, CSR fingerprint, expiry) with `0600` permissions.
   - Polls status with bounded exponential backoff (initial delay: 2s, max delay: 30s).
   - After approval, signs a completion transcript with the private key and calls the completion endpoint.
   - Validates the response: URI SAN contains `g8ed`, trust bundle is readable.
   - Atomically writes certificate, key, and trust bundle using temp-file-plus-rename.
   - Removes the pending state file.

3. **On failure**, the process exits with status 1. Operator approval is visible in the Gateway console at `/console/`. To check pending enrollments from the CLI:
   ```bash
   ./g8e auth enroll pending
   ./g8e auth enroll approve <request-id> --yes
   ```

### Handle URL hash fragment actions

The dashboard processes these URL hash fragments after session validation:

| Fragment | Payload | Behavior |
| --- | --- | --- |
| `#token=<enrollmentToken>` | Passkey enrollment token | Calls `/api/v1/auth/passkeys/enrollment/register/challenge` to start passkey enrollment (e.g., when invited by email) |
| `#enroll=1&token=<token>` | Enrollment mode + token | Variant of `#token=`, processed the same way |
| `#recovery=<token>` | CLI recovery token | Starts recovery approval flow (requires authenticated session; queued if not yet logged in) |
| `#platform-enrollment=<requestId>` | Platform enrollment request ID | Prompts for decision on a pending platform enrollment (requires authenticated session; queued if not yet logged in) |
| `#approve=<txHash>` | Transaction hash | Starts approval ceremony for a pending transaction (requires authenticated session; queued if not yet logged in) |

Fragments are cleared from the URL with `history.replaceState` immediately after parsing.

### Inspect runtime files during enrollment

After enrollment completes, the dashboard stores credentials under `G8E_RUNTIME_DIR`:

```bash
G8E_RUNTIME_DIR=/data
ls -la $G8E_RUNTIME_DIR/pki/issued/apps/
ls -la $G8E_RUNTIME_DIR/pki/trust/
ls -la $G8E_RUNTIME_DIR/pki/pending-enrollment/ # Empty after successful enrollment
```

Certificate files (`g8ed.crt`, `g8ed.key`) are readable only by the dashboard process (mode `0600`). The trust bundle (`hub-bundle.pem`) is readable by all but writable only by the owner (mode `0644`). Do not manually edit these files; let the enrollment service manage them.

## Anti-patterns

- Deploying without `G8E_GATEWAY_URL` set or reachable; the dashboard fails closed at startup.
- Configuring `--cors-origin` on the Gateway without also setting `--passkey-rp-id`; passkey registration will fail.
- Hard-coding a fallback Gateway origin in the browser when `window.G8E_GATEWAY_URL` is undefined; this defeats the deployment configuration model.
- Attempting cross-origin browser requests to the Gateway without `SameSite=None`; configure `--cors-origin` on the Gateway.
- Manually editing installed credentials in `G8E_RUNTIME_DIR` instead of triggering re-enrollment via the enrollment service.
- Treating transient network failures in the event stream as proof of session expiry; call the current-user endpoint to verify.
- Persisting the session cookie to local storage; it is HttpOnly and cannot be read from JavaScript.
- Skipping the enrollment phase at startup; the dashboard cannot serve traffic without a workload identity.

## Links out

- [Dashboard Architecture](architecture.md): Runtime boundaries, browser-to-gateway communication, component lifecycle.
- [Dashboard Development Guide](devs.md): Setting up the dev environment, running enrollment, npm scripts.
- [Gateway Integration](gateway.md): CORS configuration, Content Security Policy, certificate validation for mTLS.
- [Platform Authentication](../architecture/auth.md): CLI and certificate session models, multi-layer verification, identity model overview.
- [Network Architecture](../architecture/network.md): Trust boundaries, threat model, communication patterns.
- [PKI and Trust](../ensemble/pki.md): Root CA, certificate issuance, trust bundle management.
- [Build a g8e-Compatible Frontend](../guides/build_frontend.md): WebAuthn contract, passkey ceremony flow, credentials: 'include' requirements.
