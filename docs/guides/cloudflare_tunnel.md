---
doc_id: cloudflare_tunnel
title: Cloudflare Tunnel Integration
audience: operators, infrastructure engineers, deployment specialists
status: current
last_updated: 2026-09-28
version: v2.2.3
owners:
  - docs/guides/cloudflare_tunnel.md
  - internal/cli/cmd/gw/tunnel.go
  - internal/cli/cmd/gw/tunnel_route_dns.go
related:
  - docs/guides/public_spectator.md
  - docs/guides/build_frontend.md
  - docs/guides/lovable.md
  - docs/architecture/network.md
  - docs/architecture/gateway.md
  - docs/architecture/auth.md
when_to_read: Publishing a local g8e Gateway to the public internet through Cloudflare's managed edge network, configuring DNS routing, or integrating external frontends with tunnel-exposed services.
do_not_use_for:
  - Frontend development and authentication (docs/guides/build_frontend.md)
  - Public spectator read-only publication (docs/guides/public_spectator.md)
  - Local same-machine frontend workflows (docs/guides/lovable.md)
  - Network trust boundaries and PKI architecture (docs/architecture/network.md)
  - Gateway authentication and authorization (docs/architecture/auth.md)
---

## Purpose

Describes how to publish a local g8e Gateway through Cloudflare Tunnel, making it accessible on the public internet without opening inbound firewall ports. Covers tunnel creation, DNS routing, configuration generation, and verification. `cloudflared` remains a separate foreground process; the g8e CLI does not manage its lifecycle as a system service.

The tunnel secures transport between Cloudflare's edge and the local origin. Authentication and authorization remain the responsibility of the Gateway; Cloudflare Tunnel and Access do not replace g8e mTLS, session, JWT, or governance requirements on protected routes.

## Quick index

- [Purpose](#purpose)
- [Quick index](#quick-index)
- [Prerequisites](#prerequisites)
- [Scope and Architecture](#scope-and-architecture)
- [Procedures](#procedures)
- [Configuration Reference](#configuration-reference)
- [Verification](#verification)
- [Troubleshooting](#troubleshooting)
- [Links out](#links-out)

## Prerequisites

- `cloudflared` installed and on `PATH` ([installation guide](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/))
- Cloudflare account that controls the DNS zone for the target hostname
- g8e binary built and available as `./g8e`
- Selected origin running on localhost before external verification (Gateway HTTPS default: `localhost:8443`)
- Permission to authenticate with Cloudflare and create or update tunnel and DNS records
- For the `route-dns` command: Cloudflare API token with DNS edit permission

## Scope and Architecture

This guide configures `cloudflared` to publish a local origin through a Cloudflare-managed hostname. The g8e CLI creates or discovers a named tunnel, optionally routes DNS, and writes the `cloudflared` ingress configuration.

**Default origin**: Gateway HTTPS listener at `https://localhost:8443`. Use `--service` to publish an HTTP origin or alternate HTTPS listener (e.g., read-only public spectator).

**Do not use this guide** to expose a private Gateway, Operator, Ensemble, Dashboard, or component-local volume unintentionally. For public spectator publication, follow [Public Spectator Operations](./public_spectator.md), which defines the permitted origin and acceptance checks.

**For a frontend on the same machine as the Gateway**, use `./g8e gw connect <frontend-origin>` instead of a tunnel. A tunnel is appropriate when the browser or frontend runs outside the Gateway host.

### Traffic flow

```
[Browser] -> https://console.example.com -> [Cloudflare Edge] -> [cloudflared] -> [configured local origin]
                                                                                    |
                                                                                    +-> Gateway HTTPS (default: localhost:8443)
                                                                                    +-> or selected HTTP/HTTPS listener
```

`cloudflared` maintains the outbound connection to Cloudflare, so the Gateway host requires no inbound firewall port for the tunnel. Cloudflare terminates the public TLS connection. Origin TLS is configured independently by the generated ingress: HTTPS origins default to `noTLSVerify: true`, or validate against a supplied CA bundle; HTTP origins have no origin TLS.

A tunnel does not replace g8e authentication or authorization. Cloudflare TLS and optional Cloudflare Access protect the edge, while the Gateway continues to apply its own route authentication, WebAuthn sessions, mTLS requirements, and governance checks. See [Network Architecture](../architecture/network.md) and [Gateway Architecture](../architecture/gateway.md) for trust boundaries and route classes.

## Procedures

### Create the tunnel and generate configuration

Create a named tunnel, optionally route DNS, and generate `config.yml` for `cloudflared`:

```bash
./g8e gw tunnel create --name g8e --hostname console.example.com
```

For the default Gateway HTTPS origin on port 8443, no additional flags are required. The command:

1. Verifies `cloudflared` is installed
2. Validates tunnel name and hostname are supplied
3. Runs `cloudflared tunnel login` if `cert.pem` is not found in the cloudflared config directory
4. Creates the named tunnel (or continues if it already exists)
5. Routes the hostname via `cloudflared tunnel route dns`, unless `--skip-dns` is supplied
6. Looks up the tunnel UUID and writes `config.yml` with ingress rules, credentials path, and a catch-all `http_status:404`

**Output location**: `~/.cloudflared/config.yml` by default. The credentials file created by `cloudflared tunnel create` (`<tunnel-id>.json`) must be in the same directory; when using `--config-dir`, copy or provision the credentials file there before running the tunnel.

**Configuration directory note**: The CLI's `cloudflared tunnel login` and `cloudflared tunnel create` subprocesses use cloudflared's default state directory (typically `~/.cloudflared`), not the directory specified by `--config-dir`. The generated `config.yml` refers to the credentials file inside your chosen `--config-dir`, so you must ensure the credentials file is present there.

#### Create command flags

| Flag | Default | Purpose |
| --- | --- | --- |
| `--name` | `g8e` | Named Cloudflare tunnel |
| `--hostname` | Required | Public hostname routed to the tunnel |
| `--https-port` | `8443` | Default Gateway HTTPS port (used when `--service` is omitted) |
| `--service` | `https://localhost:<https-port>` | Origin URL (HTTP or HTTPS, no user info, query, or fragment) |
| `--config-dir` | `~/.cloudflared` | Directory for generated `config.yml` and credentials path |
| `--ca-bundle` | Empty | CA bundle for HTTPS origin verification (default: `noTLSVerify: true`) |
| `--origin-server-name` | Empty | TLS SNI name (written as `originServerName` when `--ca-bundle` is supplied) |
| `--skip-dns` | `false` | Skip DNS routing (use when CNAME already exists or DNS is managed separately) |

The command prints suggested Gateway flags using the tunnel hostname. Review these carefully before running `gw start`, especially when a separate frontend calls the Gateway: `--cors-origin` and `--passkey-rp-origin` must contain the frontend's exact origin, while `--public-base-url` is the public Gateway URL used for approval links and host validation.

#### Generated ingress configuration

For the default HTTPS origin, the generated `config.yml` has this shape. The credentials path is absolute; `<home>` represents the current user's home directory.

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

**TLS configuration**:
- When `--ca-bundle` is supplied, `originCaPool` replaces `noTLSVerify`
- `originServerName` is written only when `--origin-server-name` is supplied alongside `--ca-bundle`
- For HTTP origins, no `originRequest` block is emitted
- `http2Origin: true` is emitted for all HTTPS origins

By default, `noTLSVerify` disables verification of the local origin certificate. Use `--ca-bundle` when the origin is not strictly local or when origin certificate verification is required. The CA bundle path must be readable by the `cloudflared` process.

### Route DNS with zone-specific API command (when needed)

`cloudflared tunnel route dns` creates the CNAME using the Cloudflare zone authorized by the login session. If that login selected the wrong zone, use the g8e `route-dns` command to upsert the record through the Cloudflare API in the zone that owns the hostname:

```bash
export CLOUDFLARE_API_TOKEN='<token-with-dns-edit-permission>'
./g8e gw tunnel route-dns --name g8e console.example.com
```

The command accepts `--api-token`, or reads `CLOUDFLARE_API_TOKEN` or `CF_API_TOKEN`, in that precedence order. The token must have DNS edit permission for the target zone. Do not place the token in a document, shell history, or committed configuration file.

`route-dns` requires an existing named tunnel and resolves its UUID via `cloudflared tunnel list`. It upserts a proxied CNAME and waits up to 30 seconds for the hostname to resolve. A DNS warning after the record update means resolution was not observed within that window; it does not roll back the record.

If the `create` command reports that a DNS record already exists, rerun it with `--skip-dns` when the existing record targets the intended tunnel, or use `route-dns` to deliberately update the record.

### Start the Gateway with tunnel-aware configuration

Configure the Gateway with the public URL and the browser origins that use it:

```bash
./g8e gw start -f \
  --posture doctrine \
  --passkey-rp-id console.example.com \
  --passkey-rp-origin https://console.example.com \
  --public-base-url https://console.example.com \
  --cors-origin https://console.example.com
```

Equivalently, use environment variables:

```bash
export G8E_PASSKEY_RP_ID=console.example.com
export G8E_PASSKEY_RP_ORIGINS=https://console.example.com
export G8E_PUBLIC_BASE_URL=https://console.example.com
export G8E_ALLOWED_ORIGINS=https://console.example.com

./g8e gw start -f --posture doctrine
```

CLI flags take precedence over environment variables.

**Multiple frontends**: If a separately hosted frontend calls the Gateway, add that frontend's exact origin as a repeatable `--cors-origin` and `--passkey-rp-origin` value. Use the frontend hostname as `--passkey-rp-id` when passkey ceremonies run in that frontend page. The tunnel hostname alone is not a substitute for the frontend origin.

**Route protection**: The default Gateway HTTPS surface is port 8443. The health route is public, but other HTTPS routes retain their documented g8e authentication requirements. A Cloudflare Access policy or service token does not replace a required g8e mTLS identity, web session, JWT, or governance proof.

### Start the tunnel

Start `cloudflared` in a separate terminal after the selected origin is running:

```bash
./g8e gw tunnel run --name g8e
```

The command runs `cloudflared tunnel run g8e` in the foreground and forwards Ctrl+C or termination signals to `cloudflared`. Supply `--config-dir` when the configuration was generated outside `~/.cloudflared`:

```bash
./g8e gw tunnel run --name g8e --config-dir /path/to/cloudflared
```

You can also run `cloudflared` directly:

```bash
cloudflared tunnel run g8e
```

### Verify the tunnel and origin

Report tunnel control-plane status and optionally verify that the public hostname reaches the configured origin:

```bash
./g8e gw tunnel status --name g8e --hostname console.example.com
```

The `--hostname` flag is optional. Without it, the command skips the public health request. The status command prints `ACTIVE` when `cloudflared tunnel info` succeeds, but the health request confirms the configured public hostname reaches the expected origin.

**Manual verification**:

```bash
curl -fsS https://console.example.com/api/v1/health
```

A ready Gateway returns HTTP 200 JSON with `status`, `mode`, `version`, `pid`, `governance_ready`, `posture`, and optionally `state_merkle_root`:

```json
{"status":"ok","mode":"gateway","version":"<version>","pid":12345,"governance_ready":true,"posture":"doctrine","state_merkle_root":"<root>"}
```

Exact version, process ID, posture, and state root are runtime values. An unready Gateway returns HTTP 503 with an error message such as `service initializing`, `platform_settings not ready`, or `state root calculation failed`.

**Console access**: Open `https://console.example.com/console/` in a browser. The console still requires its g8e WebAuthn enrollment and flow. Cloudflare Edge TLS makes the public URL browser-trusted; it does not enroll a g8e user or grant console access.

## Configuration Reference

### Cloudflare Access (optional edge gate)

Cloudflare Access is an external policy and is not configured by g8e tunnel commands. To add it:

1. Create a self-hosted application in **Cloudflare Zero Trust** for the tunnel hostname
2. Attach an allow policy requiring email OTP, identity provider, or another Cloudflare-supported authentication method

Resulting flow:

```
Browser -> Cloudflare Access gate -> cloudflared tunnel -> g8e origin authentication and authorization
```

Access is an additional edge gate. It does not replace Gateway WebAuthn for console operations or g8e mTLS, session, JWT, and governance requirements on protected API routes. If Access service tokens are used for an API request, send the Cloudflare token headers as required by the Access application AND provide every authentication mechanism required by the g8e route. The public health route does not require g8e credentials.

### Frontend integration

For a hosted frontend, configure the Gateway with the frontend's exact origin in `--cors-origin` and `--passkey-rp-origin`, use the frontend hostname or valid registrable-domain suffix for `--passkey-rp-id`, and use the tunnel URL for `--public-base-url` when approval links must resolve through the tunnel.

Browser cookie policy still applies when the frontend and Gateway are cross-site. See [Build a g8e-Compatible Frontend](./build_frontend.md) for WebAuthn, CORS, cookies, and API integration requirements.

For the same-machine workflow, `./g8e gw connect <frontend-origin>` derives the frontend settings, manages local Gateway startup and trust, and verifies HTTPS and CORS. A Cloudflare tunnel is not required for that workflow.

## Verification

### Tunnel connectivity

Verify tunnel is active and connected to Cloudflare:

```bash
./g8e gw tunnel status --name g8e
```

### Gateway health through tunnel

Verify the Gateway is reachable and ready through the public hostname:

```bash
./g8e gw tunnel status --name g8e --hostname console.example.com
```

Or manually:

```bash
curl https://console.example.com/api/v1/health
```

A ready response includes `"status":"ok"`. Any error indicates the tunnel is not passing traffic to the configured origin, or the origin is not running.

## Troubleshooting

### `cloudflared` is missing

Verify `cloudflared` is installed and on `PATH`:

```bash
cloudflared --version
```

Install or update using the package method appropriate for your operating system. The [Cloudflare installation guide](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/) is the authoritative source for supported packages and release channels.

### Public endpoint returns 502 Bad Gateway

The tunnel may be connected while the origin is stopped, listening on a different port, or using a different protocol. Verify the local origin is running and matches the generated ingress:

```bash
curl -sk https://localhost:8443/api/v1/health
./g8e gw tunnel status --name g8e --hostname console.example.com
```

If `--service` points to HTTP, use an `http://` local check and do not expect the Gateway HTTPS health response. For the public spectator, verify the service points only to the configured read-only listener; do not point the tunnel at Gateway port 8443 unless that is the intended origin.

### DNS routing fails or resolves to the wrong zone

Confirm the hostname belongs to the intended Cloudflare zone and that the logged-in account can edit it. For a zone-selection problem, use the token-based `route-dns` command:

```bash
./g8e gw tunnel route-dns --name g8e console.example.com --api-token '<token>'
```

Use `--skip-dns` only when the existing CNAME already points to the intended tunnel or DNS is managed by another controlled process.

### WebAuthn passkey enrollment fails

The RP ID must be a hostname valid for the page origin. Do not include a scheme or port. When the ceremony runs at `https://console.example.com`, use `console.example.com` or a valid registrable-domain suffix permitted by WebAuthn. If a hosted frontend runs the ceremony, use that frontend's hostname instead of assuming the Gateway tunnel hostname is valid.

### CORS or authenticated requests fail

Add the browser frontend's exact scheme, hostname, and port to `--cors-origin`, and add the same frontend origin to `--passkey-rp-origin` when passkeys run there. A tunnel and Cloudflare Access do not override browser third-party-cookie policy or satisfy g8e route authentication requirements.

### Origin certificate verification fails

By default, HTTPS origins use `noTLSVerify: true`. For verification, regenerate the configuration with `--ca-bundle` and, when the certificate requires a specific SNI name, `--origin-server-name`. Confirm that the CA bundle path is readable by the process running `cloudflared` and that the service URL uses HTTPS.

## Links out

- [Public Spectator Operations](./public_spectator.md): Publish the read-only public mirror through a tunnel
- [Build a g8e-Compatible Frontend](./build_frontend.md): Configure public and multi-origin frontend access
- [Connect a Lovable App](./lovable.md): Connect a browser-hosted frontend directly to a local Gateway
- [Gateway Architecture](../architecture/gateway.md): Gateway listeners, route authentication, and runtime boundaries
- [Network Architecture](../architecture/network.md): TLS surfaces, PKI, identities, and transport boundaries
- [Cloudflare Tunnel Documentation](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/): cloudflared installation, authentication, and tunnel operation
