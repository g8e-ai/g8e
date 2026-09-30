# g8e external frontend (no Lovable)

Static browser SPA that talks **directly** to a local g8e Gateway over HTTPS, following:

- `docs/guides/build_frontend.md`
- `docs/guides/connect_frontend_to_gateway.md`
- `docs/guides/lovable.md`

Same contracts as the embedded console / dashboard auth layer: WebAuthn passkeys, `credentials: "include"`, SSE with `withCredentials: true`, approvals, enrollment hash links.

## Files

| File | Purpose |
|------|---------|
| `index.html` | Shell UI |
| `styles.css` | Dark console theme |
| `app.js` | Gateway client + WebAuthn + SSE + dashboard |
| `serve.sh` | Local static server on port 3003 |
| `README.md` | This file |

## Prerequisites

1. Built `./g8e` binary and a Gateway you can start.
2. Modern browser with WebAuthn (Chrome / Firefox / Safari / Edge).
3. Browser trusts the Gateway HTTPS certificate (use `gw connect` trust flow).

## Run (local loopback — recommended)

### 1. Serve this frontend on localhost

From this directory:

```bash
./serve.sh
# or: python3 -m http.server 3003
# or: npx --yes serve -l 3003
```

Open **http://localhost:3003** in a **top-level** browser tab (not an iframe).

Do **not** open `index.html` via `file://` — WebAuthn and CORS require a real origin.

### 2. Connect the Gateway to this origin

From the g8e repository root:

```bash
./g8e gw connect http://localhost:3003
```

That command:

- validates the origin
- configures CORS + passkey RP settings for this frontend
- installs local cert trust (with consent)
- verifies HTTPS + CORS
- prints a frontend prompt (Gateway URL, usually `https://localhost:8443`)

### 3. Use the UI

1. Confirm **Gateway HTTPS origin** in the top bar (default `https://localhost:8443`).
2. Click **Health** — should show healthy.
3. If the gateway is not bootstrapped: **Enroll Passkey** (first owner only).
4. If bootstrapped: enter **User ID** → **Sign In with Passkey**.
5. After login: passkeys, pending approvals, platform enrollments, live SSE audit stream.

## Hash deep-links (CLI flows)

| Hash | Behavior |
|------|----------|
| `#enroll=1&token=…` | Token-gated passkey enrollment (cleared from URL immediately) |
| `#approve={txHash}` | After login, runs approval WebAuthn ceremony |
| `#recovery={token}` | Prompt approve/deny CLI recovery |
| `#platform-enrollment={requestId}` | Prompt approve/deny workload enrollment |

Example after `g8e auth enroll user` (CLI may open the embedded console; you can paste the token URL onto this app’s origin instead).

## Endpoints used

**Public**

- `GET /api/v1/health`
- `GET /api/v1/auth/bootstrap/status`
- `POST /api/v1/auth/passkeys/console/register/{challenge,verify}`
- `POST /api/v1/auth/passkeys/console/authenticate/{challenge,verify}`
- `POST /api/v1/auth/passkeys/enrollment/register/{challenge,verify}`
- `POST /api/v1/auth/logout`

**Authenticated** (`credentials: "include"`)

- `GET /api/v1/users/me`
- `GET /api/v1/auth/sessions/me`
- `GET|DELETE /api/v1/auth/passkeys[/{id}]`
- `GET /api/v1/approvals`, `GET|POST …/approvals/{txHash}/{challenge,verify}`
- `GET /api/v1/auth/platform-enrollments/pending`
- `POST /api/v1/auth/platform-enrollments/decision`
- `POST /api/v1/auth/cli/recovery/approve`
- `GET /api/v1/sse/stream` (`EventSource` + `withCredentials: true`)

## Invariants (must keep)

1. Browser → Gateway **directly** (no proxy / edge / service worker).
2. Every `fetch`: `credentials: "include"`.
3. Every `EventSource`: `withCredentials: true`.
4. Absolute Gateway origin (never relative `/api` on the frontend host).
5. WebAuthn challenge values base64url-decoded to `ArrayBuffer`; responses re-encoded base64url.
6. Do **not** call `/api/v1/auth/enrollment-token/validate` before enrollment (it consumes the token).

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| Failed to fetch | Gateway running? `./g8e gw status`. Rerun `./g8e gw connect http://localhost:3003`. Allow local-network access if prompted. |
| CORS errors | Origin must match exactly (scheme/host/port). Rerun `gw connect`. |
| WebAuthn SecurityError | Top-level tab, secure context (localhost HTTP or HTTPS), RP ID matches host. |
| 401 after successful passkey | Cookie blocked cross-site. Prefer same-machine `localhost` frontend + gateway as above. |
| Certificate warning | Complete `gw connect` trust install or verify fingerprint; never disable TLS checks. |

## Note on Lovable / hosted origins

Hosted frontends (e.g. `*.lovable.app`) talking to `localhost:8443` are **cross-site**. Browsers that block third-party cookies will drop `SameSite=None` session cookies → 401 after login. This local package avoids that by running the UI on loopback.

## License notice

This sample is generated for your local use against g8e. g8e itself is Business Source License 1.1 — see the main repository `LICENSE`.
