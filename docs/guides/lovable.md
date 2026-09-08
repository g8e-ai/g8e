---
title: Connect a Lovable App
parent: Guides
---

# Connect a Lovable App

Last Updated: 2026-09-08
Version: v2.1.7

---

Connect a Lovable app directly to a g8e Gateway running on the same computer. Local development does not require Cloudflare or another tunnel.

## Before You Start

In Lovable, open the app preview in a new tab and copy its URL. This guide uses `https://your-app.lovable.app` as the example.

Use only the origin: keep `https://` and the hostname, but remove any path or trailing slash.

## 1. Start the Gateway

Replace every `your-app.lovable.app` below with your Lovable app's hostname, then run:

```bash
./g8e gw start \
  --passkey-rp-id your-app.lovable.app \
  --passkey-rp-origin https://your-app.lovable.app \
  --cors-origin https://your-app.lovable.app
```

Leave the Gateway running.

## 2. Open the Local Gateway

In the same browser, open:

[https://localhost:8443/api/v1/health](https://localhost:8443/api/v1/health)

If the browser shows a certificate warning, continue to the local site. A successful response confirms that the browser can reach the Gateway.

## 3. Tell Lovable to Connect

Paste this into Lovable:

```text
Connect this app to the g8e Gateway at https://localhost:8443. Make all Gateway requests directly from the browser, not from an edge function or server. Include credentials: "include" on every fetch request and withCredentials: true on every EventSource connection.
```

## 4. Test the App

Open the Lovable app in a new browser tab, not inside the editor preview. If the browser asks for permission to access devices on your local network, select **Allow**.

The app now connects directly from your browser to the local Gateway.

## If It Does Not Connect

- **`Failed to fetch`**: Confirm the Gateway is running, reopen the local health URL, and allow local-network access.
- **CORS error**: Confirm the URL passed to both origin flags exactly matches the Lovable app origin.
- **WebAuthn `SecurityError`**: Confirm `--passkey-rp-id` contains only the Lovable app hostname and test in a new tab.
- **Authenticated requests return `401`**: Confirm Lovable added `credentials: "include"` to every request.

## When You Need a Tunnel

Use a tunnel only when the Gateway must be reached from another computer, by another user, or by Lovable edge functions and cloud-side tests. A public deployment also requires publicly trusted HTTPS.

For production setup and complete API and WebAuthn details, see [Build a g8e-Compatible Frontend](./build_frontend.md).
