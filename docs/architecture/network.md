# Network Architecture

Last Updated: 2026-07-28
Version: v1.6.6

This document details the networking architecture of the g8e platform, including PKI, mTLS, identity management, and communication patterns.

## Overview

The g8e platform uses a zero-trust networking model where all communication is authenticated via mutual TLS (mTLS) with verified SPIFFE workload identities. The platform uses `g8e.local` as the SPIFFE trust domain and default hostname for gateway connections.

### Design Goals

The use of `g8e.local` and the underlying network architecture are driven by several key goals:
1. **Canonical stability**: `g8e.local` remains the stable trust domain and default hostname across all installations.
2. **Automated bootstrap**: Users do not configure DNS or host-specific addressing unless they choose to; the CLI falls back to direct IP automatically.
3. **Security**: mTLS identity binding and SPIFFE URI SAN validation are preserved regardless of whether clients connect via `g8e.local` or direct IP.

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

The hub and Operator intermediate CAs are kept separate to enforce a clean blast-radius boundary. The hub intermediate signs only the gateway's serving identity, while the Operator intermediate signs delegated workload leaves. This separation allows the operator-issuing key to be rotated or revoked without touching the gateway's serving trust, and vice versa.

### Curve Policy

All certificates (root, intermediates, serving, and leaves) use ECDSA P-256 for maximum interoperability with SPIFFE/SPIRE and TLS 1.3 stacks.

### Revocation

Certificate revocation is enforced via a database-backed denylist checked per-request in the mTLS middleware. A standard X.509 CRL signed by the Operator intermediate CA is served at `/.well-known/g8e/pki/crl` for external consumption.

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
- Certificate revocation is checked against a database-backed revoked certificates store.
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

Since the Governance Gateway acts as a self-signed CA, clients must explicitly trust the platform Root CA to allow secure HTTPS communication, especially for browser-based WebAuthn registration. The gateway serves platform-specific bootstrap scripts that automate the installation of the CA bundle into the system trust store.

| Platform | Endpoint | Action |
| :--- | :--- | :--- |
| **Linux/macOS** | `/web-cert.sh` | Downloads CA and installs to system store via `update-ca-certificates`. |
| **Windows** | `/web-cert.ps1` | Downloads CA and installs to Root store via `Import-Certificate`. |

After running any trust script, users must restart all open browsers. Browsers cache certificate trust state, and WebAuthn registration fails if the browser does not recognize the platform CA.

### CLI Endpoint Override Flags

When the gateway's HTTP and HTTPS ports are mapped to different host ports, such as in Docker environments where container ports 8080 and 8443 are exposed on host ports 8085 and 8448, the CLI provides flags to independently override each endpoint:

| Flag | Purpose | Default |
|---|---|---|
| `--endpoint` (`-e`) | HTTP discovery endpoint (host or host:port) | `g8e.local` (or IP fallback) |
| `--port` | HTTPS/mTLS port (overrides default 8443) | `8443` |

When both flags are used together, `--endpoint` controls the HTTP discovery URL and `--port` controls the HTTPS/mTLS port. For example, running `g8e auth enroll -e localhost:8085 --port 8448` directs HTTP discovery to port 8085 and HTTPS mTLS operations to port 8448.

### No-DNS / Direct IP Configuration

The platform supports setup without requiring `/etc/hosts` changes or DNS configuration. If `g8e.local` resolution fails, the CLI automatically falls back to direct IP access using the machine's external interface IP.

### Windows Certificate Store Enrollment

On Windows, `g8e auth enroll` auto-detects the platform and uses the Windows Certificate Store for CLI session enrollment. This certificate-based CLI enrollment is distinct from the browser-based WebAuthn passkey flow.

1. **CLI Enrollment**: Run `g8e auth enroll [--tpm]` to generate an ECDSA P-256 keypair in the Windows Certificate Store.
2. **CSR Signing**: The CLI submits a CSR to the Governance Gateway and receives a signed certificate with SPIFFE URI SAN.
3. **Certificate Import**: The signed certificate is imported to `Cert:\CurrentUser\My` in the Windows Certificate Store for Windows Hello native API access.
4. **Session Binding**: The Governance Gateway extracts the SPIFFE URI SAN from the client certificate and creates a `cli_session_id` bound to the user identity.

**TPM-Backed Keys**: The `--tpm` flag (Windows-only) utilizes the Microsoft Platform Crypto Provider KSP to generate keys in hardware. Currently, the implementation uses a software-backed key with TPM annotation as full CNG API integration is pending.

---

## 5. Port Topology

The Governance Gateway utilizes a consolidated 2-port design. Surfaces with different TLS requirements are isolated by port.

Default ports:

| Surface | Port (default) | Auth | Purpose |
|---|---|---|---|
| **HTTP (Bootstrap)** | `8080` (plain HTTP) | No TLS | Health checks, state endpoint, trust establishment scripts, CA discovery, enrollment, deploy scripts, and node binary distribution. Unregistered paths return 404. |
| **HTTPS (Merged API + Console)** | `8443` (hybrid TLS) | mTLS / WebSession / JWT / Public | The primary execution boundary. Includes the g8e Console, browser WebAuthn endpoints, CA bundle and CRL endpoints, all mTLS-guarded operator API and MCP routes, and JWT-authenticated A2A ingress when JWKS is configured. |

### Port Constraints

- **HTTP Surface** (`8080`): Serves plain HTTP for health checks, state endpoint, trust scripts (`/web-cert.sh`, `/web-cert.ps1`), CA bundle and fingerprint discovery, enrollment endpoints, deploy scripts, and node binary distribution. Unregistered paths return 404; there is no redirect to HTTPS.
- **HTTPS Surface** (`8443`): Accepts optional client certificates at the transport layer, allowing public access to browser-based assets while requiring application-layer mTLS verification for all governed execution routes. All governed execution endpoints and operator routes require a verified SPIFFE identity via client certificate, while public routes (the Console SPA, static assets, CA bundle, CRL, and WebAuthn browser endpoints) are accessible directly. When JWKS is configured, MCP and A2A endpoints accept JWT authentication as an alternative to mTLS for BYO clients.
- **Collision Prevention**: The gateway fails startup if multiple logical surfaces are assigned to the same port, ensuring no downgrade of the mTLS execution boundary.

### Docker Port Mapping and CLI Split Endpoints

In Docker demo environments, the gateway's internal HTTP (8080) and HTTPS (8443) ports are typically mapped to different host ports (such as 8085 for HTTP and 8448 for HTTPS). Because enrollment requires HTTP discovery and HTTPS mTLS calls, the CLI uses split endpoint overrides (`--endpoint` for HTTP discovery and `--port` for HTTPS mTLS) to reach both surfaces through their respective host ports.

---

## 6. g8e.local Trust Domain and Hostname

`g8e.local` serves two roles in the platform: the SPIFFE trust domain and the default hostname for gateway connections.

### Trust Domain

The trust domain is `g8e.local`. All SPIFFE IDs use this domain: `spiffe://g8e.local/<path>`. The trust domain is static and does not vary per installation.

### Default Gateway Hostname

The platform sets `g8e.local` as the default hostname for gateway TLS connections. The gateway serving certificate includes `g8e.local` as a DNS SAN, allowing clients to connect using this name when DNS resolution is configured.

### Gateway Peer PKI

The platform defines a dedicated gateway peer PKI tier for federated gateway-to-gateway communication. The `g8e Gateway Peer Intermediate CA` signs peer certificates with the SPIFFE ID format `spiffe://g8e.local/gateway/<gateway_id>`, where `<gateway_id>` is a persistent identifier generated at gateway installation time.

### No-DNS Fallback

When `g8e.local` does not resolve via system DNS, the CLI falls back to the machine's external interface IP. The CLI uses `g8e.local` as the TLS ServerName for verification even when connecting via direct IP, since the gateway certificate includes `g8e.local` in its DNS SAN.

### Local Operator Delivery

1. A `GovernanceEnvelope` arrives at the local gateway via the pub/sub stream.
2. The gateway identifies the target Operator via the internal pub/sub router.
3. If the Operator is local, the gateway delivers the envelope via in-process dispatch to the command service.

---

## 7. Communication Patterns

### Outbound-Only WebSocket Connectivity

The Governed Operator uses dial-out WebSocket pub/sub connections with zero inbound port requirements. The operator establishes a persistent WebSocket connection to the gateway's `/api/v1/pubsub/stream` endpoint using mTLS. This eliminates the need to open inbound ports on managed hosts, reducing the attack surface.

### WebSocket Pub/Sub

The gateway provides a WebSocket fan-out via `/api/v1/pubsub/stream`, authenticated by mTLS middleware. The channel naming convention is `cmd:<operator_id>:<operator_session_id>` for commands, `results:<operator_id>:<operator_session_id>` for results, and `heartbeat:<operator_id>:<operator_session_id>` for operator heartbeats. All channels require mTLS authentication, and topic ACLs enforce that subscribers can only subscribe to channels matching their mTLS workload identity.

### Server-Sent Events (SSE)

The gateway provides real-time event streaming from app workloads to browser and CLI clients via three endpoints: `POST /api/v1/sse/push` (app workload producers), `GET /api/v1/sse/events` (polling), and `GET /api/v1/sse/stream` (live SSE stream). Events are routed by `web_session_id`, `cli_session_id`, or `user_id`. See [SSE Streaming](./sse.md) for details.

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

- [**Authentication & Authorization**](./auth.md)
- [**SSE Streaming**](./sse.md)
- [**g8e Protocol**](../../protocol/docs/spec.md)
- [**g8e Gateway**](./gateway.md)
- [**g8e Operator**](./operator.md)
