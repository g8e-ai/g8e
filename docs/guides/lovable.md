---
title: Connect a Lovable App
parent: Guides
---

# Connect a Lovable App

Last Updated: 2026-09-23
Version: v2.1.12

---

Connect a Lovable app running in a browser to a g8e Gateway running on the same computer. The browser calls the Gateway directly over HTTPS; a tunnel is not required for this local workflow. For the complete browser API, WebAuthn, SSE, and frontend reference, see [Build a g8e-Compatible Frontend](./build_frontend.md).

## Before You Start

You need:

- A built `g8e` binary available from the repository root as `./g8e`.
- A Lovable app deployed or previewed at a top-level browser origin. Copy the origin from the app opened in a new browser tab, not the Lovable editor URL or an embedded preview URL. For example, `https://your-app.lovable.app`.
- A browser with WebAuthn support and permission to access local-network devices when prompted.
- A browser that accepts the local Gateway's HTTPS certificate and cross-origin session cookie. The connection workflow can install the Gateway root CA into the operating-system trust store, but it cannot change browser cookie policy.

Pass only the origin to the CLI: retain the scheme, hostname, and any non-default port, but omit a path, query, fragment, user information, and trailing slash. The CLI canonicalizes the origin before using it. Non-loopback origins must use HTTPS; HTTP is accepted only for loopback development origins such as `http://localhost:3003`.

## 1. Connect the Frontend

Run the command from the repository root, replacing the origin with the URL where you open the Lovable app:

```bash
./g8e gw connect https://your-app.lovable.app
```

`gw connect` performs these steps:

1. Validates and canonicalizes the frontend origin.
2. Derives an exact-host WebAuthn RP ID by default. For `https://your-app.lovable.app`, the default RP ID is `your-app.lovable.app`; it does not automatically use a shared parent such as `lovable.app`. Loopback IP origins retain their URL host but use `localhost` as the RP ID.
3. Applies the frontend origin to CORS and the WebAuthn RP origin, while preserving the Gateway's other launch-profile settings.
4. Starts the Gateway when it is stopped, or compares the requested browser configuration with the persisted launch profile when it is running.
5. Requests separate consent before removing stale Gateway trust anchors and before installing the current Gateway root CA into the operating-system trust store.
6. Verifies the running Gateway's HTTPS health and CORS preflight with the live Gateway CA bundle. The verification uses the validated CA roots and does not disable TLS verification.
7. Prints the exact Gateway URL and a short prompt for the frontend builder after verification succeeds.

If the running Gateway already has the requested CORS and passkey configuration, the command does not restart it. If the configuration differs, the command shows the current and proposed values and asks for restart consent. Use `--yes` only to approve that managed Gateway restart non-interactively; it does not suppress consent for trust-anchor removal or OS trust installation. If a running Gateway has no complete persisted launch profile, the command stops and tells you to run `gw stop` before retrying.

If you do not want the CLI to install system trust, select the manual browser-trust path:

```bash
./g8e gw connect https://your-app.lovable.app --no-system-trust
```

The CLI prints the Gateway HTTPS URL and live root-CA SHA-256 fingerprint, opens the URL unless `--no-open` is also set, and then runs its HTTPS and CORS verification. Verify the fingerprint before accepting the certificate in the browser. Do not disable TLS verification in the frontend. Declining an interactive OS-trust prompt returns an error after printing the same manual instructions.

A successful command prints a verification report containing HTTPS health, certificate-chain, CORS origin, CORS credentials, CORS methods, CORS headers, and `Vary: Origin` checks. If any check fails, the command returns an error and does not print the frontend handoff prompt.

## 2. Apply the Emitted Frontend Prompt

After verification succeeds, `gw connect` prints a prompt containing the actual Gateway HTTPS URL, for example:

```text
Connect this app to the g8e Gateway at https://localhost:8443. Make all Gateway requests directly from the browser, not from an edge function or server. Include credentials: "include" on every fetch request and withCredentials: true on every EventSource connection.
```

Paste that emitted prompt into Lovable. Do not replace it with a server-side, edge-function, or server-proxy integration. The browser must call the Gateway directly, every `fetch` request must use `credentials: "include"`, and every `EventSource` connection must use `withCredentials: true`. The emitted URL reflects the Gateway HTTPS port in the active launch profile; `https://localhost:8443` is only the default example.

Open the resulting app in a new top-level browser tab, not inside the Lovable editor preview. An embedded editor iframe can prevent loopback access or WebAuthn permission prompts. If the browser asks whether the app may access devices on your local network, select **Allow**.

## Observe Frontend Contract Pack

For a read-only observe dashboard showing agent and run lifecycle projections, eval summaries, downloads, and live SSE narrative, use the deterministic contract pack rather than the short prompt above. The pack is at `dashboard/g8e-adapter/contract-pack/`.

1. From `dashboard/g8e-adapter/`, run `npm run gen:contract-pack` to regenerate the pack, or `npm run gen:contract-pack:check` to verify that committed outputs are current.
2. Give Lovable `contract-pack/builder-prompt.md`, `contract-pack/models.ts`, `contract-pack/observe.openapi.json`, `contract-pack/event-schemas.json`, and the `contract-pack/fixtures/` directory.
3. Require the generated SPA to import the audited `g8e-adapter` package for runtime configuration, transport, authentication, SSE, and state. The generated presentation code must not rewrite adapter code or add arbitrary Gateway routes.
4. Deploy the generated SPA at a top-level browser origin and connect that exact origin with `./g8e gw connect <origin>`.

See [Generator-Neutral Builder Guide](./build_observe_frontend.md) for the runtime requirements and [Build a g8e-Compatible Frontend](./build_frontend.md#generator-neutral-observe-frontend) for the full observe frontend reference. The contract-pack generator's own acceptance commands are documented in [the contract-pack README](../../dashboard/g8e-adapter/contract-pack/README.md).

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
