# Network Architecture

Last Updated: 2026-08-18
Version: v1.7.7

This document details the networking architecture of the g8e platform, including PKI, mTLS, identity management, and communication patterns.

## Overview

The g8e platform uses a zero-trust networking model where all communication is authenticated via mutual TLS (mTLS) with verified SPIFFE workload identities. The platform uses `g8e.local` as the SPIFFE trust domain and as the TLS ServerName for connections that resolve the gateway by IP.

### Design Goals

The use of `g8e.local` and the underlying network architecture are driven by several key goals:
1. **Canonical stability**: `g8e.local` remains the stable trust domain across all installations.
2. **Automated bootstrap**: Users do not configure DNS or host-specific addressing unless they choose to; the CLI defaults to `localhost` for HTTP discovery and can fall back to a direct IP override when a non-default host is needed.
3. **Security**: mTLS identity binding and SPIFFE URI SAN validation are preserved regardless of whether clients connect via `g8e.local`, `localhost`, or direct IP.

---

## 1. PKI Hierarchy & Trust Domain

The platform uses a four-tier PKI hierarchy issued by the Governance Gateway:

| Tier | Certificate | Purpose | Validity |
| :--- | :--- | :--- | :--- |
| **Root CA** | `g8e Root CA` | Trust anchor for the entire platform | 3650 days |
| **Hub Intermediate CA** | `g8e Hub Intermediate CA` | Signs the gateway serving certificate | 3650 days |
| **Operator Intermediate CA** | `g8e Operator Intermediate CA` | Signs all leaf certificates (operator, CLI, app) | 3650 days |
| **Peer Intermediate CA** | `g8e Gateway Peer Intermediate CA` | Signs certificates for gateway-to-gateway peering | 3650 days |
| **Serving Certificate** | operator-gateway | Gateway TLS identity for inbound connections | 90 days |
| **Leaf Certificates** | operator, CLI, app | End-entity identities for services and clients | 7 days |
| **Peer Certificates** | gateway-peer | Identity for federated gateway communication | 90 days |

### Intermediate Split Rationale

The hub and Operator intermediate CAs are kept separate to enforce a blast-radius boundary. The hub intermediate signs only the gateway's serving identity, while the Operator intermediate signs delegated workload leaves. This separation allows the operator-issuing key to be rotated or revoked without touching the gateway's serving trust, and vice versa.

### Curve Policy

All certificates (root, intermediates, serving, and leaves) use ECDSA P-256 for broad interoperability with SPIFFE/SPIRE and TLS 1.3 stacks.

### Revocation

Certificate revocation is enforced per-request during mTLS verification. A standard X.509 CRL signed by the Operator intermediate CA is served at `/.well-known/g8e/pki/crl` for external consumption.

---

## 2. Workload Identity (SPIFFE)

Each system component receives a SPIFFE ID, embedded as a Uniform Resource Identifier (URI) in the Subject Alternative Name (SAN) of its mTLS certificate.

| Workload Type | SPIFFE ID Format |
| :--- | :--- |
| **Governed Operator** | `spiffe://g8e.local/operator/<organization_id>/<operator_id>/<operator_session_id>` |
| **CLI / BYO Client** | `spiffe://g8e.local/cli/<user_id>/<cli_session_id>` |
| **Application / Agent** | `spiffe://g8e.local/app/<operator_id>` |
| **User (Human Delegator)** | `spiffe://g8e.local/user/<user_id>` |
| **Governance Gateway** | `spiffe://g8e.local/hub/operator-listen` |
| **Gateway Peer** | `spiffe://g8e.local/gateway/<gateway_id>` |

---

## 3. mTLS Enforcement

The Governance Gateway enforces TLS 1.3 for all L7 communication. Network transport provides identity assurance for the platform's five-layer interlock pipeline: L1 Doctrine, L2 Consensus, L3 Notary, L4 Warden, and L5 Actuator.

### Application-Layer mTLS Enforcement

- The gateway requests and verifies client certificates when present during the TLS handshake, allowing browser-based clients (such as the g8e Console) to connect without certificates.
- For all non-public routes, application-layer middleware acts as a strict, fail-closed gate requiring a verified client certificate.
- Certificate revocation is checked against the revoked certificates store.
- Middleware verifies that the SPIFFE ID in the client certificate matches the specific session identifier (such as `operator_session_id` or `cli_session_id`) inside the `GovernanceEnvelope`.

### Identity Binding

All peer connections enforce mTLS with SPIFFE URI SAN validation. Certificates utilize `spiffe://g8e.local/...` regardless of the host environment, ensuring identity is consistent across the mesh.

---

## 4. Certificate Enrollment & Bootstrap

### CSR-Based Enrollment

CSR-based enrollment provides cryptographic identity proof. Instead of sharing a secret such as an API key, a client generates its own key pair and submits a Certificate Signing Request (CSR) to the gateway. The Governance Gateway acts as the Certificate Authority (CA) that signs the certificate, attesting to the client identity. Starting the gateway represents platform authorization, requiring no pre-shared keys or manual approval steps. The client authenticates every subsequent call using mTLS signed with its private key.

Clients enroll in the platform using a Certificate Signing Request (CSR) bootstrap flow:

1. **CA Discovery**: Clients fetch the platform gateway trust bundle (root CA, hub intermediate, operator intermediate, and gateway peer intermediate) from the endpoint `/.well-known/g8e/pki/ca-bundle`.
2. **CSR Submission**: Clients generate a local ECDSA P-256 key pair and submit a CSR to `/api/v1/pki/csr/sign`.
3. **Registration**: The Governance Gateway validates the CSR and binds the certificate to a user identity.
4. **Session Issuance**: Upon successful enrollment, the Governance Gateway issues a specific `operator_session_id` or `cli_session_id`.

### Trusting the Self-Signed Root CA

Since the Governance Gateway acts as a self-signed CA, clients must explicitly trust the platform Root CA to allow secure HTTPS communication, especially for browser-based WebAuthn registration. The `auth enroll user` command installs the platform Root CA into the OS trust store as part of the interactive enrollment flow, before opening the browser for the passkey ceremony.

If automatic OS trust installation fails, `auth enroll user` stops before opening the browser and returns actionable remediation. Use `--no-system-trust` only when an administrator has already installed the Root CA on the host; it does not skip the passkey ceremony.

After installing the Root CA (or removing stale anchors from a previous gateway instance), close all open browser windows before clicking the enrollment link so the new trust anchor is recognized. Firefox and other browser-private trust stores may require separate handling.

### CLI Recovery and Rotation Routes

The old `handleCLIEnrollment` endpoint (`/api/v1/auth/cli/enroll`) and the trust-script routes (`/web-cert.sh`, `/web-cert.ps1`, `/.well-known/g8e/pki/trust-windows`) have been removed. They are replaced by the following public or mTLS-protected routes:

- `POST /api/v1/auth/cli/recovery/request` and `POST /api/v1/auth/cli/recovery/complete` are public discovery-surface routes; the CSR is the proof-of-possession anchor and the opaque token is the lookup key.
- `GET /api/v1/auth/cli/recovery/status` is public for token-scoped polling.
- `POST /api/v1/auth/cli/recovery/approve` is HTTPS-only and requires a browser web-session cookie so an existing user can authorize the new CLI via the Console SPA.
- `POST /api/v1/auth/cli/recovery/approve-cli` is HTTPS-only and mTLS-only; it is the headless counterpart to the browser approve endpoint. An already-enrolled CLI authorizes the new CLI via `g8e auth approve-recovery <token>`, and the approver user ID is derived from the verified mTLS certificate URI SAN by the unified auth middleware. It is never registered on the plain HTTP router.
- `POST /api/v1/auth/cli/rotate` is HTTPS-only and mTLS-only; it is never registered on the plain HTTP router. Identity is derived from the verified client certificate, and only one replacement is performed per run.

The recovery request, status, and complete endpoints are reachable over both plain HTTP and HTTPS so a new CLI without trusted TLS can initiate recovery. The approve endpoint is HTTPS-only because it requires a web-session cookie, which is only set over TLS. The approve-cli endpoint is HTTPS-only and mTLS-only because the approver must already hold a valid CLI certificate.

### Passkey Enrollment Routes

CLI-initiated passkey enrollment uses two HTTPS-only enrollment-token routes: a registration challenge step and a registration verify step under `/api/v1/auth/passkeys/enrollment/register/`. The one-time enrollment token is generated by the CLI through the mTLS-protected `/api/v1/auth/enrollment-token/generate` endpoint and passed to the browser via the `#enroll=1&token=...` URL fragment. The gateway derives `user_id` and `cli_session_id` from the token; neither is sent in the request body. The challenge step validates the token; the verify step consumes it.

### Operator Command Dispatch Routes

The command dispatch and operator session lookup endpoints are HTTPS- and mTLS-only. `POST /api/v1/operators/commands` accepts a typed `OperatorCommandRequest`, routes it through the governance pipeline, and returns a `DispatchResponse`. `GET /api/v1/operators/session/{id}` looks up an operator by session ID.

### CLI Endpoint Override Flags

When the gateway's HTTP and HTTPS ports are mapped to different host ports, such as in Docker environments where container ports `8080` and `8443` are exposed on host ports `8085` and `8448`, the CLI provides flags to independently override each endpoint:

| Flag | Purpose | Default |
| --- | --- | --- |
| `--endpoint` (`-e`) | HTTP discovery endpoint (host or host:port) for remote enrollment | empty (uses `localhost` via the configured HTTP port) |
| `--port` (`-p`) | HTTPS/mTLS port (overrides default 8443; use with `--endpoint`) | `0` (uses the configured HTTPS port, normally `8443`) |

When both flags are used together without a scheme in `--endpoint`, the CLI splits the overrides: `--endpoint` controls the HTTP discovery URL and `--port` controls the HTTPS/mTLS port. For example, running `g8e auth enroll user -e localhost:8085 --port 8448` directs HTTP discovery to port `8085` and HTTPS mTLS operations to port `8448`.

### `mcp stdio` Credential Flags

The `g8e mcp stdio` subcommand accepts command-scoped credential flags for direct IDE MCP config integration. These flags are not root-persistent; they apply only to `mcp stdio`. Cert and key are resolved as pairs per tier (app or client); supplying only one half of a pair fails closed.

| Flag | Purpose | Default when unset |
| --- | --- | --- |
| `--client-cert` | Path to CLI client certificate (mTLS) | resolved from enrolled CLI cert on disk |
| `--client-key` | Path to CLI client key (mTLS) | resolved from enrolled CLI key on disk |
| `--ca-bundle` | Path to gateway CA bundle PEM | resolved from the enrolled trust bundle in `.g8e/pki/trust/` |
| `--gateway-url` | Gateway MCP endpoint URL (https only) | `https://g8e.local:8443/mcp` with a direct-IP fallback if `g8e.local` cannot resolve |
| `--app-cert` | Path to delegated app certificate (requires `--app-key`) | none |
| `--app-key` | Path to delegated app key (requires `--app-cert`) | none |

Credentials resolve in this order: command flag, then the matching `G8E_*` environment variable, then the enrolled CLI credential on disk. App cert/key pairs are evaluated before client cert/key pairs, so a fully specified app pair takes precedence.

### No-DNS / Direct IP Configuration

The platform supports setup without requiring `/etc/hosts` changes or DNS configuration. The CLI default discovery URL uses `localhost`; the `mcp stdio` default gateway URL uses `g8e.local`. If `g8e.local` does not resolve, `mcp stdio` falls back to the machine's external IPv4 interface while continuing to use `g8e.local` as the TLS ServerName, because the gateway certificate includes `g8e.local` in its DNS SAN.

### Windows Certificate Store Enrollment

On Windows, `g8e auth enroll user` auto-detects the platform and imports the signed CLI certificate into the CurrentUser Personal store for Windows Hello and CNG access. This is part of the same `auth enroll user` flow that installs the gateway root CA and registers a passkey; it is not a separate enrollment mode.

1. The CLI generates an ECDSA P-256 key pair and CSR.
2. The CSR is submitted to the Governance Gateway, and a signed certificate with a SPIFFE URI SAN is returned.
3. The signed certificate is imported into the Windows Certificate Store.
4. The gateway extracts the SPIFFE URI SAN from the client certificate and creates a `cli_session_id` bound to the user identity.

---

## 5. Port Topology

The Governance Gateway utilizes a consolidated 2-port design. Surfaces with different TLS requirements are isolated by port.

Default ports:

| Surface | Port (default) | Auth | Purpose |
| --- | --- | --- | --- |
| **HTTP (Bootstrap)** | `8080` (plain HTTP) | No TLS | Health checks, state endpoint, CA discovery, bootstrap, CSR signing, CLI recovery request/status/complete, deploy scripts, and node binary distribution. |
| **HTTPS (Merged API + Console)** | `8443` (hybrid TLS) | mTLS / WebSession / JWT / Public | The primary execution boundary. Includes the g8e Console, browser WebAuthn endpoints, CA bundle and CRL endpoints, all mTLS-guarded operator API and MCP routes, and JWT-authenticated A2A ingress when JWKS is configured. |

### Port Constraints

- **HTTP Surface** (`8080`): Serves plain HTTP for health checks, state endpoint, CA bundle and fingerprint discovery, bootstrap, CSR signing, CLI recovery request/status/complete (token-scoped, no mTLS required), deploy scripts, and node binary distribution. The old trust-script routes (`/web-cert.sh`, `/web-cert.ps1`, `/.well-known/g8e/pki/trust-windows`) and `handleCLIEnrollment` (`/api/v1/auth/cli/enroll`) are removed; trust installation is now handled by `auth enroll user` directly, and enrollment is handled by the recovery/rotation flow.
- **HTTPS Surface** (`8443`): Accepts optional client certificates at the transport layer, allowing public access to browser-based assets while requiring application-layer mTLS verification for all governed execution routes. All governed execution endpoints and operator routes require a verified SPIFFE identity via client certificate, while public routes (the Console SPA, static assets, CA bundle, CRL, and WebAuthn browser endpoints) are accessible directly. When JWKS is configured, MCP and A2A endpoints accept JWT authentication as an alternative to mTLS for BYO clients.
- **Collision Prevention**: The gateway fails startup if multiple logical surfaces are assigned to the same port, ensuring no downgrade of the mTLS execution boundary.

### Docker Port Mapping and CLI Split Endpoints

In Docker demo environments, the gateway's internal HTTP (`8080`) and HTTPS (`8443`) ports are typically mapped to different host ports (such as `8085` for HTTP and `8448` for HTTPS). Because enrollment requires HTTP discovery and HTTPS mTLS calls, the CLI uses split endpoint overrides (`--endpoint` for HTTP discovery and `--port` for HTTPS mTLS) to reach both surfaces through their respective host ports.

---

## 6. `g8e.local` Trust Domain and Hostname

`g8e.local` serves two roles in the platform: the SPIFFE trust domain and the stable hostname used for TLS ServerName verification.

### Trust Domain

The trust domain is `g8e.local`. All SPIFFE IDs use this domain: `spiffe://g8e.local/<path>`. The trust domain is static and does not vary per installation.

### Default Gateway Hostname

The gateway serving certificate is issued to `localhost` and `g8e.local` by default, plus any identities detected by the network identity detector. The CLI defaults to `localhost` for HTTP discovery, while `g8e.local` is used as the TLS ServerName when connecting by direct IP and as the default hostname for the `mcp stdio` gateway URL.

### Gateway Peer PKI

The platform defines a dedicated gateway peer PKI tier for federated gateway-to-gateway communication. The `g8e Gateway Peer Intermediate CA` signs peer certificates with the SPIFFE ID format `spiffe://g8e.local/gateway/<gateway_id>`, where `<gateway_id>` is a persistent identifier generated at gateway installation time.

### No-DNS Fallback

When `g8e.local` does not resolve via system DNS, the `mcp stdio` path falls back to the machine's external IPv4 interface. The CLI continues to use `g8e.local` as the TLS ServerName for verification even when connecting via direct IP, since the gateway certificate includes `g8e.local` in its DNS SAN. Other CLI commands can use the `--endpoint` flag to set a non-default host or direct IP for HTTP discovery and the `--port` flag to override the HTTPS port.

---

## 7. Communication Patterns

### Outbound-Only WebSocket Connectivity

The Governed Operator uses dial-out WebSocket pub/sub connections with zero inbound port requirements. The operator establishes a persistent WebSocket connection to the gateway's `/api/v1/pubsub/stream` endpoint using mTLS. This eliminates the need to open inbound ports on managed hosts, reducing the attack surface.

### WebSocket Pub/Sub

The gateway provides a WebSocket fan-out via `/api/v1/pubsub/stream`, authenticated by mTLS. Channels are scoped per operator and session, covering command, result, and heartbeat streams. Subscribers can only access channels matching their mTLS workload identity.

### Local Operator Delivery

When a `GovernanceEnvelope` targets the local gateway, the gateway identifies the target Operator from the envelope routing metadata. If the Operator is local, the gateway delivers the envelope directly to the local command handler.

### Server-Sent Events (SSE)

The gateway provides real-time event streaming from app workloads to browser and CLI clients via dedicated push, polling, and live stream endpoints. Events are routed by session or user identity. See [SSE Streaming](./sse.md) for details.

### Agent Integration

The platform provides zero-configuration ingress for agentic CLI coding tools through:
- **MCP stdio proxy**: Bridges stdio MCP transport to the gateway mTLS HTTPS endpoint, handling L3 approval SSE notifications and browser opening.
- **A2A protocol**: Agent-to-Agent execution via `/api/v1/a2a/call`, supporting JWT authentication when JWKS is configured for BYO client integration.

---

## 8. Network Identity Detection

The gateway detects the machine's network identity (IPs, hostnames, and aliases) at startup. This detection includes:
- **Network Interface IPs**: IPv4 addresses from all non-loopback interfaces.
- **Hostnames**: From system configuration and system calls.
- **Hosts File Aliases**: Local aliases pointing to the machine's IPs.
- **mDNS names**: Local `.local` names via mDNS services.
- **DNS PTR records**: Reverse DNS lookups.
- **SSH known_hosts**: Hostnames pointing to this machine.
- **Windows Identity**: NetBIOS and AD FQDN names.

This information is used for certificate SAN generation and peer discovery.

---

## Related Documentation

- [Authentication & Authorization](./auth.md)
- [SSE Streaming](./sse.md)
- [g8e Protocol](../../protocol/docs/spec.md)
- [g8e Gateway](./gateway.md)
- [g8e Operator](./operator.md)
