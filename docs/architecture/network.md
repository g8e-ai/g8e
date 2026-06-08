# Network Architecture

This document details the networking architecture of the g8e platform, including PKI, mTLS, identity management, and communication patterns.

## Overview
#### Design Goals
The use of g8e.local is driven by several key goals:
1. **Canonical stability**: g8e.local remains the stable mesh-facing name across all installations
2. **Hidden complexity**: Real host identity and addressing are resolved internally by the gateway
3. **Frictionless bootstrap**: Users never configure DNS or host-specific addressing
4. **Security**: Translation preserves mTLS identity binding and SPIFFE URI SAN validation

The g8e platform uses a zero-trust networking model where all communication is authenticated via mutual TLS (mTLS) with verified SPIFFE workload identities. The platform uses `g8e.local` as the canonical internal hostname for mesh communication.

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
| **g8e Gateway** | `spiffe://g8e.local/hub/operator-listen` | `protocol/workload_identity.go:70-72` |
| **Gateway Peer** | `spiffe://g8e.local/gateway/<gateway_id>` | `protocol/workload_identity.go:139-141` |

---

## 3. mTLS Enforcement

The g8e Gateway enforces TLS 1.3 for all L7 communication.

### Strict mTLS

- The gateway requires and verifies client certificates using `tls.RequireAndVerifyClientCert`
- Revocation is checked against a database-backed revoked certificates store
- Middleware verifies that the SPIFFE ID in the client certificate matches the specific session identifier (such as `operator_session_id` or `cli_session_id`) inside the `GovernanceEnvelope`

### Identity Binding

All peer connections enforce mTLS with SPIFFE URI SAN validation. Certificates utilize `spiffe://g8e.local/...` regardless of the host environment, ensuring identity is consistent across the mesh.

---

## 4. Certificate Enrollment & Bootstrap

### CSR-Based Enrollment

Clients enroll in the platform using a Certificate Signing Request (CSR) bootstrap flow:

1. **CA Discovery**: Clients fetch the platform root CA bundle from the endpoint `/.well-known/g8e/pki/ca-bundle`
2. **CSR Submission**: Clients generate a local ECDSA P-256 key pair and submit a CSR to `/api/v1/pki/csr/sign`
3. **Registration**: The g8e Gateway validates the CSR and binds the certificate to a user identity via invitation-based Just-In-Time (JIT) provisioning
4. **Session Issuance**: Upon successful enrollment, the g8e Gateway issues a specific `operator_session_id` or `cli_session_id`

### Windows Certificate Store Enrollment

Windows users can enroll via the Windows Certificate Store for managed browser authentication:

1. **CLI Enrollment**: Run `./g8e auth enroll-windows [--tpm]` to generate an ECDSA P-256 keypair
2. **CSR Signing**: The CLI submits a CSR to the g8e Gateway and receives a signed certificate with SPIFFE URI SAN
3. **Certificate Import**: The signed certificate is imported to `Cert:\CurrentUser\My` in the Windows Certificate Store (experimental)
4. **Browser Authentication**: Chrome and Edge automatically present certificates from the Windows Personal store when the g8e Gateway issues a TLS CertificateRequest
5. **Session Binding**: The g8e Gateway extracts the SPIFFE URI SAN from the client certificate and creates a `web_session_id` bound to the user identity

**TPM-Backed Keys**: The `--tpm` flag utilizes the Microsoft Platform Crypto Provider KSP to generate keys in hardware. Currently, the implementation uses a software-backed key with TPM annotation as the full CNG API integration is pending.

---

## 5. Port Topology

The g8e Gateway exposes two logical protocol surfaces. To maintain the mTLS execution boundary, surfaces with different TLS requirements must not share a port.

Default ports are sourced from `internal/constants/ports.go:17`:

| Surface | Port (default) | Auth | Purpose |
|---|---|---|---|
| **HTTP (Bootstrap Only)** | `8080` (plain HTTP) | No TLS | Bootstrap enrollment (`/bootstrap`, `/enroll`) and CA bundle discovery (`/.well-known/g8e/pki/*`). |
| **HTTPS (mTLS API + Public + MCP)** | `8443` (mTLS) | mTLS + URI SAN or JWT | `/api/v1/governance/envelopes`, `/api/v1/db/*`, `/api/v1/kv/*`, `/api/v1/blob/*`, `/api/v1/pubsub/publish`, `/ws/v1/pubsub`, MCP endpoints (`/mcp/*`), and public mTLS surface for external app enrollment. |

### Port Constraints

- **HTTP Surface** (`8080`): Serves plain HTTP for bootstrap enrollment and PKI discovery only. Does NOT serve MCP routes. Intended for initial provisioning only.
- **HTTPS Surface** (`8443`): Requires `tls.RequireAndVerifyClientCert`. This is the primary execution boundary for mTLS API, public surface, and MCP endpoints. MCP routes are protected by `auth.Middleware` (mTLS) when JWKS is not configured, or `JWTAuthMiddleware` when JWKS is configured for external IdP auth.
- **Collision Prevention**: The gateway fails startup if incompatible surfaces are assigned to the same port.

---

## 6. g8e.local Internal Translation Layer

`g8e.local` is the canonical internal hostname for operator-to-operator communication in the g8e mesh. The gateway translates this alias to installation-specific peer identity and endpoint data, ensuring that users do not manage hostnames, IPs, or DNS records manually.

### Design Goals

1. **Canonical stability**: `g8e.local` remains the stable mesh-facing name across all installations
2. **Hidden complexity**: Real host identity and addressing are resolved internally by the gateway
3. **Frictionless bootstrap**: Users never configure DNS or host-specific addressing
4. **Security**: Translation preserves mTLS identity binding and SPIFFE URI SAN validation

### Translation Components

#### Canonical Alias

- **Alias**: `g8e.local`
- **Scope**: Internal mesh communication only
- **Visibility**: Never exposed to end users; used internally for routing and identity resolution

#### Gateway Identity Mapping

The gateway maintains a mapping from the canonical alias to installation-specific identity:

```
g8e.local -> spiffe://g8e.local/gateway/<gateway_id>
```

Where `<gateway_id>` is a persistent identifier generated at gateway installation time and stored in the data directory.

#### Peer Endpoint Resolution

When a gateway needs to communicate with a peer, it utilizes the `PeerConnectionManager` to perform resolution:

```
g8e.local -> {
  gateway_id: "gw-abc123-...",
  endpoints: ["10.0.1.5:8080", "192.168.1.100:8080"],
  certificate: <gateway peer leaf cert>,
  last_seen: <timestamp>
}
```

The endpoint set is discovered via:
- Initial federation seed configuration (via `FederationSeedURL`)
- Local network identity detection for standalone deployments

#### Certificate SAN Binding

Gateway peer certificates include the canonical alias in their SPIFFE URI SAN:

```
URI SAN: spiffe://g8e.local/gateway/<gateway_id>
```

This ensures:
- Identity is consistent across the mesh
- mTLS validation verifies the canonical namespace
- Certificate revocation operates on the canonical identity rather than host-specific names

### Routing Flow

#### Local Operator Resolution

1. Envelope arrives at the local gateway
2. Gateway identifies the target Operator via the internal pub/sub router
3. If the Operator is local, the gateway delivers the envelope via in-process dispatch
4. No alias translation is required for local delivery

#### Federation Foundations

The v1.0.6 release provides the PKI and identity foundations for remote resolution:
1. Gateway peer identity is established via `gateway-peer` intermediate CA
2. `PeerConnectionManager` maintains outbound-only connections to a federation seed
3. Envelopes are re-verified by the receiving gateway

### Implementation Notes

#### Gateway ID Generation

- Generated once at gateway installation
- Persisted in the `gateway-id` file within the gateway data directory
- Format: `gw-<hex>-<hex>-<hex>-<hex>` (16 bytes of entropy)

#### Fallback Behavior

- If no federation seed is configured, `g8e.local` utilizes localhost for service discovery
- Standalone gateway behavior is preserved through this fallback
- Federation remains opt-in via seed configuration

### Security Invariants

1. **Identity binding**: All peer connections enforce mTLS with SPIFFE URI SAN validation
2. **Canonical namespace**: Certificates utilize `spiffe://g8e.local/...` regardless of the host environment
3. **No DNS dependency**: Translation is internal to the gateway service; no external DNS is required
4. **Re-verification**: Every gateway re-verifies envelopes on receipt as mandated by the governance pipeline

---

## 7. Communication Patterns

### Outbound-Only mTLS Connectivity

The g8e Operator uses dial-out reverse tunnels with zero inbound port requirements. This eliminates the need to open inbound ports on managed hosts, reducing the attack surface.

### WebSocket Pub/Sub

The gateway provides a high-performance WebSocket fan-out via `/ws/v1/pubsub`. Mutation channels (`cmd:*`) are governed and require mTLS authentication.

### Agent Integration

The platform provides zero-config ingress for agentic CLI coding tools through:
- **Agent Wrapper**: Detects tool binaries, verifies gateway status, and injects G8E_* environment variables with MCP configuration
- **Stdio Proxy**: Bridges stdio MCP transport to the gateway HTTP endpoint with mTLS, handling L3 approval polling and browser opening

---

## 8. Network Identity Detection

The gateway detects the machine's network identity (IPs, hostnames, and aliases) using the detector in `internal/services/network/identity.go`. This information is used for:
- Federation seed configuration
- Local network identity detection for standalone deployments
- Peer endpoint resolution

---

## 9. Implementation Reference

| Concern | File |
|---|---|
| Workload identity | `protocol/workload_identity.go` |
| Network identity | `internal/services/network/identity.go` |
| PKI / CertStore | `internal/services/gateway/gateway_certs.go` |
| Peer connection manager | `internal/services/gateway/peer_connection.go` |
| Port constants | `internal/constants/ports.go` |
| Gateway pub/sub | `internal/services/gateway/gateway_pubsub.go` |
| Gateway service | `internal/services/gateway/gateway_service.go` |
| Governance envelope verification | `internal/services/gateway/governance_envelope.go` |
| Federation plan | `.local.dev/docs/plans/gateway-federation-option-a-plan.md` |

---

## Related Documentation

- [**Authentication & Authorization**](./auth.md) - Authentication and authorization architecture
- [**g8e Protocol**](./protocol.md) - The wire contract and governance hierarchy
- [**g8e Gateway**](./gateway.md) - Gateway architecture and capabilities
- [**g8e Operator**](./operator.md) - Operator architecture and execution boundary
