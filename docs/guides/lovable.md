---
title: Connect a Lovable App
parent: Guides
---

# Connect a Lovable App

Last Updated: 2026-09-09
Version: v2.1.8

---

Connect a Lovable app directly to a g8e Gateway running on the same computer. Local development does not require Cloudflare or another tunnel.

## Before You Start

In Lovable, open the app preview in a new tab and copy its URL. This guide uses `https://your-app.lovable.app` as the example.

Use only the origin: keep `https://` and the hostname, but remove any path or trailing slash.

## 1. Connect

Run one command, replacing the origin with your Lovable app's URL:

```bash
./g8e gw connect https://your-app.lovable.app
```

The CLI validates the origin, derives the passkey RP ID (`your-app.lovable.app`, never the shared `lovable.app` parent), starts or restarts the Gateway with the correct CORS and passkey settings when needed, installs the local Gateway certificate into your OS trust store with explicit consent, and verifies HTTPS and CORS against the running process.

If the Gateway is already running with the correct configuration, the command verifies without restarting. If the configuration differs, it asks whether to restart; pass `--yes` to approve non-interactively.

If you decline system trust installation (or your platform is managed), the command prints the local Gateway URL and certificate fingerprint for manual browser trust. Add `--no-system-trust` to select that path explicitly.

## 2. Paste the Prompt

After verification succeeds, the CLI prints a prompt. Paste it into Lovable:

```text
Connect this app to the g8e Gateway at https://localhost:8443. Make all Gateway requests directly from the browser, not from an edge function or server. Include credentials: "include" on every fetch request and withCredentials: true on every EventSource connection.
```

Then open the app in a new browser tab, not inside the Lovable editor preview. The embedded editor iframe can block loopback and WebAuthn permissions; a top-level tab can request them.

If the browser asks for permission to access devices on your local network, select **Allow**.

## Observe Frontend (Generator-Neutral Contract Pack)

For a read-only observe dashboard (agent and run lifecycle projections, eval summaries, downloads, live SSE narrative), use the deterministic contract pack instead of the generic prompt above. The contract pack lives at `dashboard/g8e-adapter/contract-pack/` and contains a `builder-prompt.md` that encodes every hard constraint a generated observe SPA must satisfy.

1. From `dashboard/g8e-adapter/`, run `npm run gen:contract-pack` to regenerate the pack (or `npm run gen:contract-pack:check` to verify committed outputs are current).
2. Give Lovable the contents of `contract-pack/builder-prompt.md` plus the `contract-pack/models.ts`, `contract-pack/observe.openapi.json`, `contract-pack/event-schemas.json`, and `contract-pack/fixtures/` files.
3. The generated SPA imports the audited `g8e-adapter` package for transport, auth, SSE, and state. It does not rewrite adapter code.
4. Deploy the generated SPA at a top-level origin and connect it with `./g8e gw connect <origin>`.

See [Generator-Neutral Builder Guide](./build_observe_frontend.md) for the runtime capability requirements and [Build a g8e-Compatible Frontend](./build_frontend.md#generator-neutral-observe-frontend) for the full observe frontend reference.

## Browser Limitations

The CLI verifies Gateway HTTPS and CORS, but two browser-controlled restrictions are outside its control:

- **Local Network Access**: When the frontend requests `https://localhost:8443`, browsers may prompt for permission to access devices on your local network. Select **Allow**. The CLI cannot accept this prompt for you. If you miss the prompt, reload the page or check the browser's site permissions and re-enable local network access.
- **Third-party cookies**: The Gateway session cookie is `SameSite=None` because the frontend origin (`your-app.lovable.app`) and the Gateway origin (`localhost`) are cross-site. Browsers that block third-party cookies reject the session cookie, so authenticated requests return `401` even after a successful passkey login. The CLI cannot detect or override browser cookie policy. If your browser blocks third-party cookies, deploy the frontend and Gateway on the same site or proxy Gateway requests through the frontend origin so the cookie is first-party. A tunnel does not guarantee cookie acceptance and is not a universal fix for this limitation.

## If It Does Not Connect

- **`Failed to fetch`**: Confirm the Gateway is running (`./g8e gw status`), allow local-network access, and rerun `./g8e gw connect <origin>`.
- **CORS error**: Rerun `./g8e gw connect <origin>`; the command verifies CORS against the running Gateway and reports mismatches.
- **WebAuthn `SecurityError`**: Test in a new top-level tab, not the Lovable editor preview. The CLI derives the exact-host RP ID automatically.
- **Authenticated requests return `401`**: Confirm Lovable added `credentials: "include"` to every request. If `credentials: "include"` is set and login succeeds but authenticated calls still return `401`, your browser may be blocking the cross-site session cookie as a third-party cookie; see Browser Limitations above.
- **Certificate warning**: Approve system trust installation when `gw connect` prompts, or follow the manual trust instructions it prints when you decline.

## When You Need a Tunnel

Use a tunnel only when the Gateway must be reached from another computer, by another user, or by Lovable edge functions and cloud-side tests. A public deployment also requires publicly trusted HTTPS. See [Cloudflare Tunnel Integration](./cloudflare_tunnel.md).

## Advanced Configuration

`gw connect` accepts one origin and derives the safest defaults. For multi-origin deployments, public tunnels, custom ports, or other advanced Gateway settings, use `./g8e gw start` with explicit flags. See [Build a g8e-Compatible Frontend](./build_frontend.md) for the full reference.
