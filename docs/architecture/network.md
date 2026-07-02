# Network Architecture

Last Updated: 2026-07-02
Version: v1.3.5

This document details the networking architecture of the g8e platform, including PKI, mTLS, identity management, and communication patterns.

## Overview

The g8e platform uses a zero-trust networking model where all communication is authenticated via mutual TLS (mTLS) with verified SPIFFE workload identities. The platform uses `g8e.local` as the SPIFFE trust domain and default hostname for gateway connections.

### Design Goals

The use of `g8e.local` and the underlying network architecture are driven by several key goals:
1. **Canonical stability**: `g8e.local` remains the stable trust domain and default hostname across all installations.
2. **Frictionless bootstrap**: Users never configure DNS or host-specific addressing unless they choose to; the CLI falls back to direct IP automatically.
3. **Security**: mTLS identity binding and SPIFFE URI SAN validation are preserved regardless of whether clients connect via `g8e.local` or direct IP.

---

## 1. PKI Hierarchy & Trust Domain

The platform uses a four-tier PKI hierarchy issued by the g8e Gateway:

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

The hub and Operator intermediate CAs are kept separate to enforce a clean blast-radius boundary. The hub intermediate signs only the gateway's serving identity, while the Operator intermediate signs delegated workload leaves. This separation allows the operator-issuing key to be rotated or revoked without touching the gateway's serving trust, and vice versa.

### Curve Policy

All certificates (root, intermediates, serving, and leaves) use ECDSA P-256 for maximum interoperability with SPIFFE/SPIRE and TLS 1.3 stacks.

### Revocation

Certificate revocation is enforced via a database-backed denylist checked per-request in the mTLS middleware. A standard X.509 CRL signed by the Operator intermediate CA is served at `/.well-known/g8e/pki/crl` for external consumption.

---

## 2. Workload Identity (SPIFFE)

Each system component receives a SPIFFE ID, embedded as a Uniform Resource Identifier (URI) in the Subject Alternative Name (SAN) of its mTLS certificate. The generation logic is defined in `protocol/workload_identity.go`.

| Workload Type | SPIFFE ID Format | Reference |
| :--- | :--- | :--- |
| **g8e Operator** | `spiffe://g8e.local/operator/<organization_id>/<operator_id>/<operator_session_id>` | `protocol/workload_identity.go:37-39` |
| **CLI / BYO Client** | `spiffe://g8e.local/cli/<user_id>/<cli_session_id>` | `protocol/workload_identity.go:48-50` |
| **Application / Agent** | `spiffe://g8e.local/app/<operator_id>` | `protocol/workload_identity.go:59-61` |
| **User (Human Delegator)** | `spiffe://g8e.local/user/<user_id>` | `protocol/workload_identity.go:70-72` |
| **g8e Gateway** | `spiffe://g8e.local/hub/operator-listen` | `protocol/workload_identity.go:81-83` |
| **Gateway Peer** | `spiffe://g8e.local/gateway/<gateway_id>` | `protocol/workload_identity.go:165-167` |

---

## 3. mTLS Enforcement

The g8e Gateway enforces TLS 1.3 for all L7 communication.

### Application-Layer mTLS Enforcement

- The gateway accepts and verifies client certificates using `tls.VerifyClientCertIfGiven` at the TLS layer. This allows browser-based clients (such as the g8e Console) to connect without certificates.
- For all non-public routes, the `auth.Middleware()` acts as a strict, fail-closed gate at the application layer, requiring a verified client certificate.
- Revocation is checked against a database-backed revoked certificates store.
- Middleware verifies that the SPIFFE ID in the client certificate matches the specific session identifier (such as `operator_session_id` or `cli_session_id`) inside the `GovernanceEnvelope`.

### Identity Binding

All peer connections enforce mTLS with SPIFFE URI SAN validation. Certificates utilize `spiffe://g8e.local/...` regardless of the host environment, ensuring identity is consistent across the mesh.

---

## 4. Certificate Enrollment & Bootstrap

### CSR-Based Enrollment

**The mental model:** CSR-based enrollment is cryptographic identity proof. Instead of sharing a secret (like an API key), a client generates its own key pair and asks the Gateway to sign a certificate attesting "this public key belongs to this identity." The Gateway acts as a Certificate Authority (CA). The act of starting the Gateway is itself the Platform Owner's authorization; there are no standing invite codes, pre-shared keys, or manual approval steps. The client then proves its identity on every subsequent call by signing with its private key (via mTLS). No shared secrets, no API keys to leak.

Clients enroll in the platform using a Certificate Signing Request (CSR) bootstrap flow:

1. **CA Discovery**: Clients fetch the platform gateway trust bundle (root CA, hub intermediate, operator intermediate, and gateway peer intermediate) from the endpoint `/.well-known/g8e/pki/ca-bundle`.
2. **CSR Submission**: Clients generate a local ECDSA P-256 key pair and submit a CSR to `/api/v1/pki/csr/sign`.
3. **Registration**: The g8e Gateway validates the CSR and binds the certificate to a user identity.
4. **Session Issuance**: Upon successful enrollment, the g8e Gateway issues a specific `operator_session_id` or `cli_session_id`.

### Trusting the Self-Signed Root CA

Since the g8e Gateway acts as a self-signed CA, clients must explicitly trust the platform Root CA to allow secure HTTPS communication, especially for browser-based WebAuthn registration. The gateway serves platform-specific bootstrap scripts that automate the installation of the CA bundle into the system trust store.

| Platform | Endpoint | Action |
| :--- | :--- | :--- |
| **Linux** | `/bootstrap-ca` | Downloads CA and installs to system store via `update-ca-certificates`. |
| **macOS** | `/bootstrap-ca-macos` | Downloads CA and installs to System Keychain via `security add-trusted-cert`. |
| **Windows** | `/bootstrap-ca.ps1` | Downloads CA and installs to Root store via `Import-Certificate`. |

**Browser Restart**: After running any trust script, users **must restart all open browsers**. Browsers often cache certificate trust state, and WebAuthn registration will fail if the browser does not yet recognize the new platform CA.

### No-DNS / Direct IP Configuration

The platform supports setup without requiring `/etc/hosts` changes or DNS configuration. If `g8e.local` resolution fails, the system automatically falls back to direct IP access using the machine's external interface IP. This is implemented in `internal/cli/cmd/mcp.go:279-331`.

### Windows Certificate Store Enrollment

On Windows, `g8e auth enroll` auto-detects the platform and uses the Windows Certificate Store for CLI session enrollment. This is a CLI session enrollment (mTLS cert-based), distinct from the browser-based WebAuthn passkey flow (cookie-based web session).

1. **CLI Enrollment**: Run `./g8e auth enroll [--tpm]` to generate an ECDSA P-256 keypair in the Windows Certificate Store.
2. **CSR Signing**: The CLI submits a CSR to the g8e Gateway and receives a signed certificate with SPIFFE URI SAN.
3. **Certificate Import**: The signed certificate is imported to `Cert:\CurrentUser\My` in the Windows Certificate Store for Windows Hello native API access.
4. **Session Binding**: The g8e Gateway extracts the SPIFFE URI SAN from the client certificate and creates a `cli_session_id` bound to the user identity.

**TPM-Backed Keys**: The `--tpm` flag (Windows-only) utilizes the Microsoft Platform Crypto Provider KSP to generate keys in hardware. Currently, the implementation uses a software-backed key with TPM annotation as the full CNG API integration is pending.

---

## 5. Port Topology

The g8e Gateway utilizes a consolidated 2-port design. Surfaces with different TLS requirements are isolated by port.

Default ports are defined in `protocol/constants/ports.json`:

| Surface | Port (default) | Auth | Purpose |
|---|---|---|---|
| **HTTP (Bootstrap)** | `8080` (plain HTTP) | No TLS | Health checks, trust establishment scripts, CA discovery, enrollment, deploy scripts, and node binary distribution. All other requests are redirected to HTTPS. |
| **HTTPS (Merged API + Console)** | `8443` (hybrid TLS) | mTLS / WebSession / Public | The primary execution boundary. Includes the g8e Console, browser WebAuthn endpoints, CA bundle and CRL endpoints, and all mTLS-guarded operator API and MCP routes. |

### Port Constraints

- **HTTP Surface** (`8080`): Serves plain HTTP for health checks, state endpoint, trust scripts (`/bootstrap-ca` etc.), CA bundle and fingerprint discovery, enrollment endpoints, deploy scripts, and node binary distribution. All other requests are redirected to HTTPS via a permanent 301 redirect.
- **HTTPS Surface** (`8443`): Uses `tls.VerifyClientCertIfGiven`, overriding the stricter `tls.RequireAndVerifyClientCert` default from `PKIAuthority.TLSConfig()`. Operates with application-layer mTLS validation. All governed execution endpoints and operator routes require a verified SPIFFE identity via client certificate, while public routes (the Console SPA, static assets, CA bundle, CRL, and WebAuthn browser endpoints) are accessible directly.
- **Collision Prevention**: The gateway fails startup if multiple logical surfaces are assigned to the same port, ensuring no downgrade of the mTLS execution boundary.

---

## 6. g8e.local Trust Domain and Hostname

`g8e.local` serves two roles in the platform: the SPIFFE trust domain and the default hostname for gateway connections.

### Trust Domain

The trust domain is defined as the constant `TrustDomain = "g8e.local"` in `protocol/workload_identity.go`. All SPIFFE IDs use this domain: `spiffe://g8e.local/<path>`. The trust domain is static and does not vary per installation.

### Default Gateway Hostname

The constant `GatewayInternalHostname` in `internal/constants/network.go` sets `g8e.local` as the default hostname for gateway TLS connections. The gateway serving certificate includes `g8e.local` as a DNS SAN, allowing clients to connect using this name when DNS resolution is configured.

### Gateway Peer PKI

The platform defines a dedicated gateway peer PKI tier for federated gateway-to-gateway communication. The `g8e Gateway Peer Intermediate CA` signs peer certificates with the SPIFFE ID format `spiffe://g8e.local/gateway/<gateway_id>`, where `<gateway_id>` is a persistent identifier generated at gateway installation time. The peer intermediate CA and SPIFFE ID format are defined in `internal/services/gateway/gateway_certs.go` and `protocol/workload_identity.go` respectively.

### No-DNS Fallback

When `g8e.local` does not resolve via system DNS, the CLI falls back to the machine's external interface IP. This fallback is implemented in `internal/cli/cmd/mcp.go` using `network.GetExternalInterfaceIP()` from `internal/services/network/identity.go`.

### Local Operator Delivery

1. A `GovernanceEnvelope` arrives at the local gateway via the pub/sub stream.
2. The gateway identifies the target Operator via the internal pub/sub router.
3. If the Operator is local, the gateway delivers the envelope via in-process dispatch to the command service.

---

## 7. Communication Patterns

### Outbound-Only WebSocket Connectivity

The g8e Operator uses dial-out WebSocket pub/sub connections with zero inbound port requirements. The operator establishes a persistent WebSocket connection to the gateway's `/api/v1/pubsub/stream` endpoint using mTLS. This eliminates the need to open inbound ports on managed hosts, reducing the attack surface.

### WebSocket Pub/Sub

The gateway provides a WebSocket fan-out via `/api/v1/pubsub/stream`, authenticated by `WebSocketAuth` middleware. The channel naming convention is `cmd:<operator_id>:<operator_session_id>` for commands and `results:<operator_id>:<operator_session_id>` for results. All channels require mTLS authentication.

### Server-Sent Events (SSE)

The gateway provides real-time event streaming from app workloads to browser and CLI clients via three endpoints: `POST /api/v1/sse/push` (app workload producers), `GET /api/v1/sse/events` (polling), and `GET /api/v1/sse/stream` (live SSE stream). Events are routed by `web_session_id`, `cli_session_id`, or `user_id`. See [SSE Streaming](./sse.md) for full details.

### Agent Integration

The platform provides zero-config ingress for agentic CLI coding tools through:
- **MCP stdio proxy**: Bridges stdio MCP transport to the gateway mTLS HTTPS endpoint, handling L3 approval polling and browser opening.

---

## 8. Network Identity Detection

The gateway detects the machine's network identity (IPs, hostnames, and aliases) using the detector in `internal/services/network/identity.go`. This detection includes:
- **Network Interface IPs**: Both IPv4 and IPv6.
- **Hostnames**: From `/etc/hostname` and system calls.
- **Hosts File Aliases**: Local aliases pointing to the machine's IPs.
- **mDNS names**: *.local names via Avahi or Bonjour.
- **DNS PTR records**: Reverse DNS lookups.
- **SSH known_hosts**: Hostnames pointing to this machine.
- **Windows Identity**: NetBIOS and AD FQDN names.

This information is used for certificate SAN generation and peer discovery.

---

## 9. Implementation Reference

| Concern | File |
|---|---|
| Workload identity | `protocol/workload_identity.go` |
| Network identity | `internal/services/network/identity.go` |
| PKI / CertStore | `internal/services/gateway/gateway_certs.go` |
| Port constants | `protocol/constants/ports.json` |
| Gateway hostname constant | `internal/constants/network.go` |
| HTTP and HTTPS routers | `internal/services/gateway/gateway_http_router.go` |
| mTLS middleware and public routes | `internal/services/gateway/gateway_auth.go` |
| Gateway pub/sub | `internal/services/gateway/gateway_pubsub.go` |
| SSE event bridge | `internal/services/gateway/gateway_http_sse.go` |
| SSE event storage | `internal/services/gateway/sse_event_service.go` |
| Gateway service | `internal/services/gateway/gateway_service.go` |
| Governance envelope verification | `internal/services/gateway/governance_envelope.go` |
| CLI MCP Stdio | `internal/cli/cmd/mcp.go` |

---

## Related Documentation

- [**Authentication & Authorization**](./auth.md)
- [**SSE Streaming**](./sse.md)
- [**g8e Protocol**](../../protocol/docs/spec.md)
- [**g8e Gateway**](./gateway.md)
- [**g8e Operator**](./operator.md)
