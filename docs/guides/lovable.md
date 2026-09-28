---
doc_id: lovable
title: Connect a Lovable App to the Gateway
audience: frontend builders and developers
status: current
last_updated: 2026-09-28
version: v2.2.0
owners:
  - docs/guides/lovable.md
  - internal/cli/cmd/gw/gateway_connect.go
  - dashboard/g8e-adapter/
related:
  - docs/guides/build_frontend.md
  - docs/guides/build_observe_frontend.md
  - docs/guides/cloudflare_tunnel.md
when_to_read: Connecting a Lovable app or other browser-hosted frontend to a local g8e Gateway for development, testing, or secure observe workflows.
do_not_use_for:
  - Production Gateway configuration beyond the browser origin (use docs/guides/cloudflare_tunnel.md)
  - Operator or consensus configuration (use docs/devs/devs.md)
  - Contract pack generation and testing (use dashboard/g8e-adapter/contract-pack/README.md)
---

# Connect a Lovable App to the Gateway

## Purpose

Guides developers through connecting a Lovable app (or any browser-hosted frontend) running locally to a g8e Gateway on the same machine. The browser communicates directly with the Gateway over HTTPS; no tunnel is required for local development. The workflow validates the frontend origin, configures WebAuthn and CORS, manages local certificate trust, and verifies end-to-end connectivity before handing off to the frontend builder.

For the complete browser API, WebAuthn, SSE, and frontend reference, see [Build a g8e-Compatible Frontend](./build_frontend.md).

## Quick index

- [Purpose](#purpose)
- [Invariants](#invariants)
- [Before You Start](#before-you-start)
- [Connect the Frontend](#1-connect-the-frontend)
- [Apply the Frontend Prompt](#2-apply-the-emitted-frontend-prompt)
- [Observe Frontend Contract Pack](#observe-frontend-contract-pack)
- [Browser Limitations](#browser-limitations)
- [Troubleshooting](#if-it-does-not-connect)
- [Advanced Configuration](#advanced-configuration)
- [When You Need a Tunnel](#when-you-need-a-tunnel)
- [Links out](#links-out)

## Invariants

| ID | Rule |
| --- | --- |
| INV-LOVABLE-01 | The `./g8e gw connect <origin>` command MUST validate and canonicalize the frontend origin before any Gateway start or restart. The CLI MUST preserve the scheme, hostname, and non-default port while omitting path, query, fragment, and trailing slash. Non-loopback origins MUST use HTTPS; HTTP is accepted only for loopback development origins (e.g., `http://localhost:3003`). |
| INV-LOVABLE-02 | The WebAuthn RP ID MUST default to the exact host extracted from the frontend origin (e.g., `your-app.lovable.app` from `https://your-app.lovable.app`). Loopback IP origins MUST retain their URL host but use `localhost` as the RP ID. |
| INV-LOVABLE-03 | The browser MUST call the Gateway directly from the top-level page context, not from an embedded editor preview, edge function, or server proxy. Every `fetch` request MUST use `credentials: "include"`; every `EventSource` connection MUST use `withCredentials: true`. |
| INV-LOVABLE-04 | Browser restrictions (local-network access, top-level context, third-party cookies, certificate trust) are enforced by the browser and outside the CLI's control. The CLI verifies HTTPS and CORS but cannot override browser policies. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| gw connect command | [internal/cli/cmd/gw/gateway_connect.go](internal/cli/cmd/gw/gateway_connect.go) | `./g8e gw connect --help` |
| gw start flags | [internal/cli/cmd/gw/gateway.go:88-119](internal/cli/cmd/gw/gateway.go#L88-L119) | `./g8e gw start --help` |
| Contract pack generator | [dashboard/g8e-adapter/](dashboard/g8e-adapter/) | `npm run gen:contract-pack` from `dashboard/g8e-adapter/` |
| Contract pack README | [dashboard/g8e-adapter/contract-pack/README.md](dashboard/g8e-adapter/contract-pack/README.md) | Authoritative reference for contract pack contents and workflow |

## Procedures

### Before You Start

Ensure you have:

- A built `g8e` binary at the repository root: `./g8e`.
- A Lovable app deployed or previewed at a top-level browser origin. Extract the origin from the URL bar when the app is open in a standalone browser tab, not from the Lovable editor URL or an embedded preview. For example, `https://your-app.lovable.app`.
- A browser with WebAuthn support.
- The `--no-system-trust` path requires manual certificate verification; the standard path installs the Gateway root CA into the OS trust store with your consent.

The CLI accepts only the canonical origin: scheme + hostname + optional non-default port, with no path, query, fragment, or trailing slash. The CLI canonicalizes the input before using it.

### 1. Connect the Frontend

Run the command from the repository root, passing the origin where you open the Lovable app:

```bash
./g8e gw connect https://your-app.lovable.app
```

The command orchestrates the following state machine (internal services are always real; OS trust, HTTP transport, browser opening, and prompting are injectable for testing):

1. Parse and validate the frontend origin. The CLI rejects malformed URLs, invalid schemes, and paths/queries/fragments.
2. Derive the WebAuthn RP ID (exact-host default, e.g., `your-app.lovable.app` from `https://your-app.lovable.app`). Loopback IP origins use `localhost` as the RP ID. Override with `--passkey-rp-id` if required by your deployment.
3. Load configuration and check Gateway process state:
   - **Gateway stopped**: Apply the browser config to a copy of the existing launch profile (or defaults), start the Gateway, persist the new profile, then proceed to trust and verification.
   - **Gateway running with matching config**: Proceed directly to trust and verification.
   - **Gateway running with conflicting config**: Display the delta (current vs. proposed CORS origin, RP ID, RP name, RP origins), request restart consent (suppressed with `--yes`), stop the old process, start with the new config, persist, then proceed.
4. Discover the live CA bundle from the Gateway's unauthenticated discovery endpoint.
5. Inspect OS trust state:
   - Remove stale anchors from previous Gateway instances (separate consent; `--yes` does not suppress).
   - Install the current Gateway root CA if not already trusted (separate consent; `--no-system-trust` skips this and requires manual browser trust).
6. Run HTTPS health and CORS preflight verification against the running Gateway using the discovered root pool.
7. Emit the Gateway HTTPS URL and frontend prompt after all checks pass.

**Restart consent boundary**: Use `--yes` to approve a managed Gateway restart non-interactively. Certificate trust installation and stale-anchor removal have separate consent boundaries that `--yes` does not suppress.

**Manual trust path**: Skip OS trust installation:

```bash
./g8e gw connect https://your-app.lovable.app --no-system-trust
```

The CLI prints the Gateway HTTPS URL and root-CA SHA-256 fingerprint, opens the URL in your browser (unless `--no-open` is also set), and runs HTTPS and CORS verification. Verify the fingerprint before accepting the browser prompt. Do not disable TLS verification in the frontend.

**Verification report**: After all checks pass, the command prints:
- HTTPS health and certificate-chain details
- CORS origin, credentials, methods, headers
- `Vary: Origin` validation

If any check fails, the command returns an error and does not print the frontend prompt.

**No launch profile error**: If the Gateway is running but has no persisted launch profile, the command tells you to run `./g8e gw stop` and retry. This occurs only if the Gateway was started outside the `gw connect` / `gw start` workflow.

### 2. Apply the Emitted Frontend Prompt

After verification succeeds, `gw connect` prints a frontend prompt, for example:

```text
Connect this app to the g8e Gateway at https://localhost:8443. Make all Gateway requests directly from the browser, not from an edge function or server. Include credentials: "include" on every fetch request and withCredentials: true on every EventSource connection.
```

Paste that exact prompt into Lovable. Do not substitute a server-side, edge-function, or proxy integration. Critical constraints:
- The browser MUST call the Gateway directly (no server proxy, edge function, or service worker mediation).
- Every `fetch` to the Gateway MUST use `credentials: "include"`.
- Every `EventSource` connection to the Gateway MUST use `withCredentials: true`.

The emitted URL reflects the active Gateway's HTTPS port from the launch profile. `https://localhost:8443` is the default; yours may differ if you specified a different port.

Open the resulting app in a new, top-level browser tab, not embedded in the Lovable editor preview. An embedded editor iframe can block loopback requests or WebAuthn ceremonies. If the browser prompts for permission to access local-network devices, select **Allow**.

### Observe Frontend Contract Pack

For a read-only observe dashboard (agent and run lifecycle, eval summaries, downloads, live SSE narrative), use the deterministic contract pack instead of the short prompt. The pack lives at `dashboard/g8e-adapter/contract-pack/`.

**Workflow:**

1. **From `dashboard/g8e-adapter/`, regenerate the pack** (or verify it is current):
   ```bash
   npm run gen:contract-pack       # regenerate all outputs
   npm run gen:contract-pack:check # fail if committed outputs are stale
   ```

2. **Pass the contract pack to your builder** (Lovable or other). The builder consumes:
   - `contract-pack/builder-prompt.md` — encodes all constraints the generated SPA must satisfy
   - `contract-pack/models.ts` — typed models for presentation code
   - `contract-pack/observe.openapi.json` — curated OpenAPI for allowlisted browser operations
   - `contract-pack/event-schemas.json` — dashboard event payloads and sentinel definitions
   - `contract-pack/fixtures/` — typed fixtures (unauthenticated, bootstrap, live stream, reconnect, gaps, edge cases)

3. **The builder generates an SPA** that:
   - Imports the audited `g8e-adapter` package for transport, authentication, SSE, and state.
   - Uses only the allowlisted operations in `observe.openapi.json`. No arbitrary Gateway routes or adapter rewrites.
   - Validates all incoming data against the typed models.

4. **Deploy the SPA at a top-level browser origin** and connect it:
   ```bash
   ./g8e gw connect <exact-origin-where-spa-is-deployed>
   ```

See [Generator-Neutral Builder Guide](./build_observe_frontend.md) for runtime requirements and [Build a g8e-Compatible Frontend](./build_frontend.md) for the full reference. The contract-pack generator's acceptance commands are documented in [contract-pack/README.md](../../dashboard/g8e-adapter/contract-pack/README.md).

## Browser Limitations

The CLI verifies Gateway HTTPS and CORS, but these browser-controlled restrictions remain outside its control:

- **Local Network Access:** When the frontend requests the local Gateway, the browser may prompt for permission to access devices on the local network. Select **Allow**. If the prompt was dismissed, reload the page or re-enable the permission in the site's browser settings.
- **Top-level context:** Open the app in a top-level tab. The Lovable editor preview is an embedded context and may block loopback requests or WebAuthn ceremonies.
- **Third-party cookies:** A hosted Lovable origin and `localhost` are cross-site. The Gateway configures its session cookie as `SameSite=None` when a cross-origin frontend is allowed, but browsers that block third-party cookies still reject the cookie. Authenticated requests then return `401` even after a successful passkey ceremony. The CLI cannot detect or override this policy. Use a deployment where the frontend and Gateway are same-site, or proxy through the frontend origin when that deployment model is appropriate. A tunnel does not guarantee cookie acceptance.
- **Certificate trust:** The Gateway's local certificate must be trusted by the browser. Use the CLI's OS-trust flow or verify the printed fingerprint and complete the manual browser-trust flow. Never work around the warning by disabling TLS checks.

## If It Does Not Connect

- **`Failed to fetch`:** Confirm that the Gateway is running with `./g8e gw status`, open the app in a top-level tab, allow local-network access, and rerun `./g8e gw connect <origin>`.
- **CORS verification failure:** Rerun `./g8e gw connect <origin>` and inspect the verification report. The requested origin must be the exact origin where the app is opened, including its non-default port and excluding any path.
- **WebAuthn `SecurityError`:** Open the app outside the Lovable editor preview and confirm that the origin supplied to `gw connect` is the origin in the browser address bar. The CLI derives the exact-host RP ID by default.
- **Authenticated requests return `401`:** Confirm that every `fetch` uses `credentials: "include"` and every `EventSource` uses `withCredentials: true`. If login succeeds but requests still return `401`, the browser may be blocking the cross-site session cookie; see [Browser Limitations](#browser-limitations).
- **Certificate warning or manual-trust error:** Verify the SHA-256 fingerprint printed by `gw connect`, complete the browser's trust flow, and rerun the command if necessary. Do not disable TLS verification.
- **The command asks to restart:** Review the displayed configuration delta. Use `--yes` only when the proposed origin and passkey settings are correct. Trust prompts remain interactive even with `--yes`.

## When You Need a Tunnel

Use a tunnel when the Gateway must be reached from another computer or user, or when the integration must run in Lovable edge functions or cloud-side tests. A public deployment also requires publicly trusted HTTPS and explicit CORS and WebAuthn settings for the public origins. See [Cloudflare Tunnel Integration](./cloudflare_tunnel.md).

## Advanced Configuration

`gw connect` accepts one frontend origin and derives one exact-host default RP ID. For multiple origins, a registrable parent RP ID, public tunnels, custom Gateway ports, or other advanced settings, use `./g8e gw start` with explicit flags. The relevant flags are `--cors-origin`, repeatable `--passkey-rp-origin`, `--passkey-rp-id`, `--passkey-rp-name`, and `--public-base-url`. Validate that every origin is permitted for the selected RP ID. See [Build a g8e-Compatible Frontend](./build_frontend.md#gateway-side-configuration) for the complete reference.
