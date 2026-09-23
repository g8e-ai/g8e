---
title: Cloudflare Tunnel Integration
parent: Guides
---

# Cloudflare Tunnel Integration

Last Updated: 2026-09-23
Version: v2.1.12

---

## Scope

This guide configures `cloudflared` to publish a g8e origin through a Cloudflare-managed hostname. The g8e CLI creates or discovers a named tunnel, optionally routes DNS, and writes the `cloudflared` ingress configuration. `cloudflared` remains a separate foreground process; the g8e Gateway does not manage its lifecycle as a service.

The default origin is the Gateway HTTPS listener at `https://localhost:8443`. The `--service` flag can instead publish another local origin, such as the read-only public spectator listener. Do not use this guide to expose a private Gateway, Operator, Ensemble, Dashboard, or component-local volume unintentionally. For public spectator publication, follow [Public Spectator Operations](./public_spectator.md), which defines the permitted origin and acceptance checks.

A tunnel does not replace g8e authentication or authorization. Cloudflare TLS and optional Cloudflare Access protect the edge, while the Gateway continues to apply its own route authentication, WebAuthn sessions, mTLS requirements, and governance checks. See [Network Architecture](../architecture/network.md) and [Gateway Architecture](../architecture/gateway.md) for the trust boundaries and route classes.

For a frontend running in a browser on the same computer as the Gateway, use `./g8e gw connect <frontend-origin>` instead of a tunnel. A tunnel is appropriate when the browser or frontend is outside the Gateway host; it does not guarantee that a browser will accept cross-site session cookies. See [Connect a Lovable App](./lovable.md) and [Build a g8e-Compatible Frontend](./build_frontend.md).

### Traffic flow

```text
[Browser] -> https://console.example.com -> [Cloudflare Edge] -> [cloudflared] -> [configured local origin]
                                                                                         |
                                                                                         +-> Gateway HTTPS, or an explicitly selected public listener
```

`cloudflared` maintains the outbound connection to Cloudflare, so the Gateway host does not need an inbound firewall port for the tunnel. Cloudflare terminates the public TLS connection. Origin TLS is configured independently by the generated ingress: HTTPS origins default to `noTLSVerify: true`, or use a supplied CA bundle for verification; HTTP origins have no origin TLS.

---

## Prerequisites

- `cloudflared` is installed and on `PATH` on the same host as the process serving the origin ([installation guide](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/)).
- A Cloudflare account controls the DNS zone for the hostname. Tunnel creation changes Cloudflare tunnel and DNS state.
- The g8e binary is built. Repository examples use `./g8e`.
- The selected origin is running on the local host before external verification. The default Gateway origin listens on HTTPS port `8443`; use `--https-port` or `--service` when the origin differs.
- The operator has permission to authenticate to Cloudflare and create or update the tunnel and DNS record.

The CLI's `tunnel create` flow uses `cloudflared tunnel login` when it does not find `cert.pem` in the selected cloudflared configuration directory. Authentication opens a browser and requires interactive Cloudflare access.

---

## Step 1: Create the tunnel and generate its configuration

For the default Gateway HTTPS origin, run:

```bash
./g8e gw tunnel create --name g8e --hostname console.example.com
```

The command:

1. Checks for `cloudflared`.
2. Requires a tunnel name and public hostname.
3. Runs `cloudflared tunnel login` when it does not find the origin certificate.
4. Creates the named tunnel, or continues when the tunnel already exists.
5. Routes the hostname with `cloudflared tunnel route dns`, unless `--skip-dns` is supplied.
6. Looks up the tunnel UUID and writes `config.yml` with the tunnel credentials file, hostname ingress, origin settings, and a `http_status:404` catch-all.

The default output directory is `~/.cloudflared`. The credentials file created by `cloudflared tunnel create` is `<tunnel-id>.json` in cloudflared's default configuration directory, and the g8e command writes `config.yml` there with mode `0600`. Use `--config-dir` to choose another directory for the generated configuration and tunnel run command. The generated configuration refers to the credentials file inside that selected directory, so when using a custom directory, copy or provision the tunnel credentials there before running the tunnel. The underlying `cloudflared` login and create subprocesses use their own cloudflared default state directory rather than receiving `--config-dir` from g8e.

### `tunnel create` flags

| Flag | Default | Purpose |
| --- | --- | --- |
| `--name` | `g8e` | Named Cloudflare tunnel. |
| `--hostname` | Required | Public hostname routed to the tunnel. |
| `--https-port` | `8443` | Default Gateway HTTPS port used when `--service` is omitted. |
| `--service` | `https://localhost:<https-port>` | Origin URL. It must be an HTTP or HTTPS URL without user information, query, or fragment. |
| `--config-dir` | `~/.cloudflared` | Directory for the generated `config.yml` and the credentials path referenced by that file. |
| `--ca-bundle` | Empty | CA bundle passed to `originCaPool` for HTTPS origin verification. Without it, the generated ingress uses `noTLSVerify: true`. |
| `--origin-server-name` | Empty | Optional TLS SNI name written as `originServerName` when `--ca-bundle` is also supplied. |
| `--skip-dns` | `false` | Skip the CLI's DNS-routing step. Use when the required CNAME already exists or DNS will be managed separately. |

The command prints suggested Gateway flags using the tunnel hostname. Review those values before using them, especially when a separate frontend calls the Gateway: `--cors-origin` and `--passkey-rp-origin` must contain the frontend's exact origin, while `--public-base-url` is the public Gateway URL used for approval links and host validation. The passkey RP ID is a hostname and must be valid for the page origin; the exact frontend hostname is the default for the guided frontend workflow.

### Generated ingress

For the default HTTPS origin, the generated configuration has this shape. The implementation writes an absolute credentials path; `<home>` below represents the current user's home directory.

```yaml
tunnel: <tunnel-id>
credentials-file: <home>/.cloudflared/<tunnel-id>.json

ingress:
  - hostname: console.example.com
    service: https://localhost:8443
    originRequest:
      noTLSVerify: true
      http2Origin: true
  - service: http_status:404
```

`http2Origin: true` is emitted for HTTPS origins. If `--ca-bundle` is supplied, `originCaPool` replaces `noTLSVerify`; `originServerName` is emitted only when `--origin-server-name` is supplied. For an HTTP origin, the generated ingress contains the service and no `originRequest` block.

The default `noTLSVerify` setting disables verification of the local origin certificate. Use `--ca-bundle` when the origin is not strictly local or when origin certificate verification is required by the deployment. A CA bundle path must be readable by the `cloudflared` process.

---

## Step 2: Route DNS with the zone-specific command when needed

`cloudflared tunnel route dns` normally creates the CNAME using the zone authorized by the Cloudflare login. If that login selects the wrong zone, use the g8e `route-dns` command to upsert the record through the Cloudflare API in the zone that owns the hostname:

```bash
export CLOUDFLARE_API_TOKEN='<token-with-dns-edit-permission>'
./g8e gw tunnel route-dns --name g8e console.example.com
```

The command also accepts `--api-token`, `CLOUDFLARE_API_TOKEN`, or `CF_API_TOKEN`, in that precedence order. The token needs DNS edit permission for the target zone. Do not place the token in a document, shell history, or a committed configuration file.

`route-dns` requires an existing named tunnel and resolves its tunnel UUID with `cloudflared tunnel list`. It upserts a proxied CNAME and waits up to 30 seconds for the hostname to resolve. A DNS warning after the record update means resolution was not observed within that window; it does not roll back the record.

If the regular `create` command reports that a DNS record already exists, rerun it with `--skip-dns` when the existing record targets the intended tunnel, or use `route-dns` to update the record deliberately.

---

## Step 3: Start the Gateway

For a Gateway exposed at `console.example.com`, configure the Gateway with the public URL and the browser origins that actually use it:

```bash
./g8e gw start -f \
  --posture doctrine \
  --passkey-rp-id console.example.com \
  --passkey-rp-origin https://console.example.com \
  --public-base-url https://console.example.com \
  --cors-origin https://console.example.com
```

If a separately hosted frontend calls the Gateway, add that frontend's exact origin as a repeatable `--cors-origin` and `--passkey-rp-origin` value. Use the frontend hostname as `--passkey-rp-id` when passkey ceremonies run in that frontend page. The tunnel hostname alone is not a substitute for the frontend origin.

The corresponding environment variables are:

```bash
export G8E_PASSKEY_RP_ID=console.example.com
export G8E_PASSKEY_RP_ORIGINS=https://console.example.com
export G8E_PUBLIC_BASE_URL=https://console.example.com
export G8E_ALLOWED_ORIGINS=https://console.example.com

./g8e gw start -f --posture doctrine
```

CLI flags take precedence over environment variables. These variables configure Gateway behavior; they do not configure `cloudflared` or the tunnel commands.

The default Gateway HTTPS surface is port `8443`. The health route is public, but other HTTPS routes retain their documented g8e authentication requirements. A Cloudflare Access policy or service token does not replace a required g8e mTLS identity, web session, JWT, or governance proof.

---

## Step 4: Start the tunnel

Start the tunnel in a separate terminal after the selected origin is running:

```bash
./g8e gw tunnel run --name g8e
```

The command runs `cloudflared tunnel run g8e` in the foreground and forwards Ctrl+C or termination signals to `cloudflared`. Supply `--config-dir` when the configuration was generated outside `~/.cloudflared`:

```bash
./g8e gw tunnel run --name g8e --config-dir /path/to/cloudflared
```

You can also run `cloudflared` directly after confirming that it loads the same configuration:

```bash
cloudflared tunnel run g8e
```

---

## Step 5: Verify the tunnel and origin

The status command reports control-plane tunnel information and can optionally request the Gateway health route through the public hostname:

```bash
./g8e gw tunnel status --name g8e --hostname console.example.com
```

`--hostname` is optional. Without it, the command skips the public health request. The status command prints `ACTIVE` when `cloudflared tunnel info` succeeds, but the health request is the check that confirms the configured public hostname reaches the expected origin. The command reports failures in its output; inspect both sections.

Verify manually with the public health route:

```bash
curl -fsS https://console.example.com/api/v1/health
```

A ready Gateway returns JSON with `status`, `mode`, `version`, `pid`, `governance_ready`, `posture`, and `state_merkle_root`, for example:

```json
{"status":"ok","mode":"gateway","version":"<gateway-version>","pid":12345,"governance_ready":true,"posture":"doctrine","state_merkle_root":"<root>"}
```

The exact version, process ID, posture, and state root are runtime values. An unready Gateway returns HTTP `503` with an error such as `service initializing`, `platform_settings not ready`, or `state root calculation failed`.

For the Gateway console, open:

```text
https://console.example.com/console/
```

The console still requires its g8e WebAuthn flow. Cloudflare Edge TLS makes the public URL browser-trusted; it does not enroll a g8e user or grant console access.

---

## Optional: Cloudflare Access

Cloudflare Access is an external edge policy and is not configured by the g8e tunnel commands. To add it, create a self-hosted application in **Cloudflare Zero Trust** for the tunnel hostname and attach an allow policy for the required users or groups. An Access policy can require email OTP, an identity provider, or another Cloudflare-supported method before Cloudflare forwards a request to `cloudflared`.

The resulting flow is:

```text
Browser -> Cloudflare Access -> cloudflared tunnel -> g8e origin authentication and authorization
```

Access is an additional edge gate. It does not replace Gateway WebAuthn for browser console operations or g8e mTLS, session, JWT, and governance requirements on protected API routes. If Access service tokens are used for an API request, send the Cloudflare token headers as required by the Access application and also provide every authentication mechanism required by the g8e route. The public health route does not require g8e credentials.

---

## Frontend integration

For a hosted frontend, configure the Gateway with the frontend's exact origin in `--cors-origin` and `--passkey-rp-origin`, use the frontend hostname or valid registrable-domain suffix for `--passkey-rp-id`, and use the tunnel URL for `--public-base-url` when approval links must resolve through the tunnel. Browser cookie policy still applies when the frontend and Gateway are cross-site. See [Build a g8e-Compatible Frontend](./build_frontend.md) for WebAuthn, CORS, cookies, and API integration requirements.

For the same-machine workflow, `./g8e gw connect <frontend-origin>` derives the frontend settings, manages local Gateway startup and trust, and verifies HTTPS and CORS. A Cloudflare tunnel is not required for that workflow.

---

## Troubleshooting

### `cloudflared` is missing

The g8e commands require an executable named `cloudflared` on `PATH`:

```bash
cloudflared --version
```

Install or update it using the package method appropriate for the operating system. The Cloudflare installation guide is the source for supported packages and current release channels.

### The public endpoint returns 502

The tunnel may be connected while the configured origin is stopped, listening on another port, or using a different protocol. Check the local origin and compare it with the generated ingress:

```bash
curl -sk https://localhost:8443/api/v1/health
./g8e gw tunnel status --name g8e --hostname console.example.com
```

If `--service` points to an HTTP listener, use an `http://` local check and do not expect the Gateway HTTPS health response. For the public spectator, verify that the service points only to the configured read-only public listener; do not point the tunnel at Gateway port `8443` unless that is the intended origin.

### DNS routing fails or resolves to the wrong zone

Confirm that the hostname belongs to the intended Cloudflare zone and that the logged-in account can edit it. For a zone-selection problem, use the token-based command:

```bash
./g8e gw tunnel route-dns --name g8e console.example.com --api-token '<token-with-dns-edit-permission>'
```

Use `--skip-dns` only when the existing CNAME already points to the intended tunnel or DNS is managed by another controlled process.

### WebAuthn passkey enrollment fails

The RP ID must be a hostname valid for the page origin. Do not include a scheme or port. When the ceremony runs at `https://console.example.com`, use `console.example.com` or a valid registrable-domain suffix permitted by WebAuthn. If a hosted frontend runs the ceremony, use that frontend's hostname instead of assuming that the Gateway tunnel hostname is valid.

### CORS or authenticated requests fail

Add the browser frontend's exact scheme, hostname, and port to `--cors-origin`, and add the same frontend origin to `--passkey-rp-origin` when passkeys run there. A tunnel and Cloudflare Access do not override browser third-party-cookie policy or satisfy g8e route authentication.

### Origin certificate verification fails

By default, HTTPS origins use `noTLSVerify: true`. For verification, regenerate the configuration with `--ca-bundle` and, when the certificate requires a specific SNI name, `--origin-server-name`. Confirm that the CA bundle path is readable by the process running `cloudflared` and that the service URL uses HTTPS.

---

## See also

- [Connect a Lovable App](./lovable.md): Connect a browser-hosted frontend directly to a local Gateway.
- [Build a g8e-Compatible Frontend](./build_frontend.md): Configure public and multi-origin frontend access.
- [Public Spectator Operations](./public_spectator.md): Publish the read-only public mirror through a tunnel.
- [Gateway Architecture](../architecture/gateway.md): Gateway listeners, route authentication, and runtime boundaries.
- [Network Architecture](../architecture/network.md): TLS surfaces, PKI, identities, and transport boundaries.
- [Protocol Specification](../../protocol/docs/spec.md): Public protocol and route contract.
