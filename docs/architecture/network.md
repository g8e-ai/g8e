---
doc_id: network
title: Network Architecture
audience: maintainers and coding agents
status: current
last_updated: 2026-10-06
version: v2.3.2
owners:
  - internal/services/gateway/
  - internal/services/network/
  - internal/constants/ports.go
  - internal/constants/channels.go
  - protocol/workload_identity.go
related:
  - docs/architecture/auth.md
  - docs/architecture/gateway.md
  - docs/architecture/operator.md
  - docs/architecture/sse.md
  - docs/guides/unified_stack.md
when_to_read: Understanding platform networking, PKI hierarchy, TLS/mTLS enforcement, SPIFFE identity formats, port topology, pub/sub communication, and network identity detection.
do_not_use_for:
  - Authentication and session lifecycle mechanics (docs/architecture/auth.md)
  - Gateway operational modes and policy decision points (docs/architecture/gateway.md)
  - Operator local execution and L4/L5 verification (docs/architecture/operator.md)
  - Server-Sent Events event framing and consumer routing (docs/architecture/sse.md)
  - Deployment recipes and container compose stacks (docs/guides/unified_stack.md)
---

# Network Architecture

## Purpose

Documents the networking architecture of the g8e platform, including Public Key Infrastructure (PKI), workload identities (SPIFFE), TLS 1.3 and mTLS transport security, consolidated port topology, outbound-only pub/sub communication patterns, and automated network identity detection.

## Quick index

- [Purpose](#purpose)
- [Quick index](#quick-index)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
  - [PKI hierarchy and trust anchors](#pki-hierarchy-and-trust-anchors)
  - [Workload identity format (SPIFFE)](#workload-identity-format-spiffe)
  - [Transport security and mTLS enforcement](#transport-security-and-mtls-enforcement)
  - [Port topology and router classification](#port-topology-and-router-classification)
  - [Enrollment, bootstrap, and recovery routing](#enrollment-bootstrap-and-recovery-routing)
  - [Cross-gateway cascading topologies](#cross-gateway-cascading-topologies)
  - [Pub/sub communication patterns](#pubsub-communication-patterns)
  - [Network identity detection and SAN drift](#network-identity-detection-and-san-drift)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

Key invariant groups: [PKI and Cryptography](#pki-and-cryptography-inv-net-pki), [Workload Identity](#workload-identity-inv-net-id), [Transport Security](#transport-security-inv-net-tls), [Port Topology](#port-topology-inv-net-port), [Communication Patterns](#communication-patterns-inv-net-comm).

## Invariants

Ids are stable. Append the next free number within each group; do not renumber.

### PKI and Cryptography (`INV-NET-PKI`)

| ID | Rule |
| --- | --- |
| INV-NET-PKI-01 | All platform certificates (Root CA, intermediate CAs, serving certificate, and leaf certificates) MUST use ECDSA with the NIST P-256 curve (`elliptic.P256()`). Non-P-256 CSRs MUST be rejected. |
| INV-NET-PKI-02 | The PKI hierarchy MUST separate serving issuance from workload issuance: the Hub Intermediate CA signs only the Gateway serving certificate (`operator-gateway`), while the Operator Intermediate CA signs all workload leaf certificates (`operator`, `cli`, `app`). |
| INV-NET-PKI-03 | Workload leaf certificates MUST have a maximum validity of 7 days. Serving and peer certificates MUST have a maximum validity of 90 days. Root and intermediate CAs MUST have a validity of 3650 days. |
| INV-NET-PKI-04 | Certificate revocation MUST be verified on every mTLS request and WebSocket handshake. An X.509 CRL DER binary signed by the Operator Intermediate CA MUST be exposed at `/.well-known/g8e/pki/crl`. |
| INV-NET-PKI-05 | Platform workload revocation via `POST /api/v1/auth/platform-enrollments/revoke` MUST revoke the issued certificate in persistent storage and immediately disconnect all active pub/sub connections matching the authenticated SPIFFE identity. |

### Workload Identity (`INV-NET-ID`)

| ID | Rule |
| --- | --- |
| INV-NET-ID-01 | The canonical SPIFFE trust domain MUST be `g8e.local` across all deployments. All issued workload certificates MUST embed their SPIFFE identity URI in the Subject Alternative Name (SAN). |
| INV-NET-ID-02 | SPIFFE ID URIs MUST conform to standard type paths: `operator` (`spiffe://g8e.local/operator/<org>/<op>/<session>`), `cli` (`spiffe://g8e.local/cli/<user>/<session>`), `app` (`spiffe://g8e.local/app/<id>`), `user` (`spiffe://g8e.local/user/<user>`), `hub` (`spiffe://g8e.local/hub/operator-listen`), and `gateway-peer` (`spiffe://g8e.local/gateway/<gw>`). |
| INV-NET-ID-03 | Additive SAN drift detected at Gateway startup (missing host IPs or hostnames compared to detected network identity) MUST trigger automatic regeneration of the serving certificate. Dropped host aliases MUST NOT trigger regeneration. |

### Transport Security (`INV-NET-TLS`)

| ID | Rule |
| --- | --- |
| INV-NET-TLS-01 | Inbound HTTPS and WebSocket connections MUST enforce TLS 1.3 minimum (`tls.VersionTLS13`). Plain HTTP is restricted to discovery, health checks, and unauthenticated bootstrap/recovery initiation. |
| INV-NET-TLS-02 | The HTTPS listener MUST configure `tls.VerifyClientCertIfGiven` at the transport layer to permit browser clients to access public and console routes without client certificates, while delegating mandatory mTLS enforcement to application-layer middleware. |
| INV-NET-TLS-03 | Client connections resolving the Gateway via IP address MUST provide `g8e.local` as the TLS ServerName (SNI) to satisfy hostname validation against the serving certificate DNS SAN. |

### Port Topology (`INV-NET-PORT`)

| ID | Rule |
| --- | --- |
| INV-NET-PORT-01 | The Gateway MUST expose a consolidated two-port topology: HTTP Discovery/Bootstrap (`8080` default) and HTTPS Merged API/Console (`8443` default). Startup MUST fail if multiple logical surfaces are assigned to the same port. |
| INV-NET-PORT-02 | The plain HTTP router (`buildHTTPRouter`) MUST serve only health, state root, CA bundle discovery, and token-scoped bootstrap/recovery initiation. All unhandled paths MUST redirect with HTTP 301 to HTTPS, validating the `Host` header against `isSafeHost`. |
| INV-NET-PORT-03 | Privileged CSR signing (`/api/v1/pki/csr/sign`), CLI rotation, CLI refresh, CLI session query, and enrollment approval routes MUST NOT be registered on the plain HTTP router. |

### Communication Patterns (`INV-NET-COMM`)

| ID | Rule |
| --- | --- |
| INV-NET-COMM-01 | Governed remote Operators MUST establish outbound-only mTLS WebSocket connections to the Gateway pub/sub endpoint (`/api/v1/pubsub/stream`). Operators MUST NOT open inbound listening ports for command, MCP, or A2A traffic. |
| INV-NET-COMM-02 | Pub/sub topic channels (`cmd:`, `results:`, `heartbeat:`, `receipts:`, `audit:`) MUST be strictly scoped by Operator ID and session ID, and authorization MUST be validated against the caller's verified SPIFFE identity URI SAN. |
| INV-NET-COMM-03 | The Lattice gRPC adapter is a supplementary integration; the native outbound-only WebSocket pub/sub stream is the authoritative governed command dispatch path. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| PKI Authority and TLS config | `internal/services/gateway/gateway_certs.go` | Unit tests in `internal/services/gateway/pki_authority_test.go` |
| Default port constants | `internal/constants/ports.go` | Schema in `protocol/constants/ports.json` and `internal/config/config.go` |
| HTTP & HTTPS route muxing | `internal/services/gateway/gateway_http_router.go` | Route tests in `internal/services/gateway/gateway_http_test.go` |
| SPIFFE identity formatting | `protocol/workload_identity.go` | Spec tests in `protocol/workload_identity_test.go` |
| Network identity detection | `internal/services/network/identity.go` | Interface tests in `internal/services/network/identity_test.go` |
| Operator pub/sub transport | `internal/services/pubsub/g8eg_pubsub_client.go` | Stream tests in `internal/services/gateway/gateway_pubsub_test.go` |
| Platform enrollment revocation | `internal/services/pubsub/platform_enrollment_handlers.go` | Lifecycle tests in `internal/services/gateway/platform_enrollment_controller_test.go` |

## Procedures

### PKI hierarchy and trust anchors

The current implementation builds the platform PKI in `internal/services/gateway/gateway_certs.go` under `PKIAuthority`. It does not treat the serving certificate and workload certificates as a single flat tree: the code creates separate trust roots and keeps them isolated by issuer type.

| Certificate family | Current implementation | Purpose | Validity | Key spec |
| :--- | :--- | :--- | :--- | :--- |
| **Root CA** | `g8e Root CA` | platform trust anchor | 3650 days | ECDSA P-256 |
| **Hub Intermediate CA** | `g8e Hub Intermediate CA` | signs the Gateway service certificate (`operator-gateway`) | 3650 days | ECDSA P-256 |
| **Operator Intermediate CA** | `g8e Operator Intermediate CA` | signs workload leaves (operator, CLI, app, etc.) | 3650 days | ECDSA P-256 |
| **Gateway Peer Intermediate CA** | `g8e Gateway Peer Intermediate CA` | signs cross-gateway peering identities when a gateway participates as a peer | 3650 days | ECDSA P-256 |
| **Serving cert** | `operator-gateway` | Gateway TLS identity | 90 days | ECDSA P-256 |
| **Workload leaves** | per identity | mTLS client/server auth for CLI/operator/app workloads | 7 days | ECDSA P-256 |
| **Peer certs** | `gateway-peer` | federated gateway peer auth | 90 days | ECDSA P-256 |

The behaviour matches the code: `loadOrGenerateRootCA()` and `loadOrGenerateIntermediateCAs()` create or load the root and intermediate CAs; `loadOrGenerateServiceCertWithNames()` creates the serving certificate; `SignCSR()` issues workload leaves. The service cert and workload leaves are intentionally separated by issuer, so a compromised workload intermediate does not automatically invalidate the Gateway serving identity.

All issued certificates use ECDSA P-256 and the service TLS config sets `MinVersion: tls.VersionTLS13` and FIPS curve preferences. CSRs submitted with the wrong curve or invalid signatures are rejected before issuance.

Revocation is stored in the PKI revocation state and served as a CRL. `PKIController.handlePKIRevocationBundle()` exposes the CRL at `/.well-known/g8e/pki/crl` (and the API route `/api/v1/pki/revocation-bundle`), while the explicit management route `/api/v1/pki/certificates/revoke` accepts internal requests to revoke by serial number. Platform enrollment revocation is an application-level flow: the owner-approved `POST /api/v1/auth/platform-enrollments/revoke` path marks the certificate as revoked and the corresponding auth middleware / pub/sub layer disconnects active channels associated with the same SPIFFE identity.

### Workload identity format (SPIFFE)

Every platform identity is generated from the canonical SPIFFE trust domain `g8e.local` and embedded as a URI SAN in the leaf certificate. The current canonical formats are defined in `protocol/workload_identity.go`:

```text
spiffe://g8e.local/<type>/<identity-segments>
```

| Workload type | SPIFFE ID format | Notes |
| :--- | :--- | :--- |
| **Governed Operator** | `spiffe://g8e.local/operator/<organization_id>/<operator_id>/<operator_session_id>` | outbound operator session identity |
| **CLI / BYO client** | `spiffe://g8e.local/cli/<user_id>/<cli_session_id>` | local CLI session identity |
| **Application / agent** | `spiffe://g8e.local/app/<operator_id>` | app identity for a delegated workload |
| **Ensemble (`g8ee`)** | `spiffe://g8e.local/app/g8ee` | special app identity used for event fan-out |
| **User principal** | `spiffe://g8e.local/user/<user_id>` | delegated human user SAN |
| **Gateway serving identity** | `spiffe://g8e.local/hub/operator-listen` | hub/service identity used for the hosting Gateway |
| **Gateway peer** | `spiffe://g8e.local/gateway/<gateway_id>` | peer-to-peer gateway identity |

The special `app/g8ee` identity is used as a recognized broker identity for event delivery across sessions; it is not a per-operator app ID.

### Transport security and mTLS enforcement

The active implementation in `internal/services/gateway/gateway_service.go` configures the HTTPS listener with `tls.VersionTLS13` and a client verification policy of `tls.VerifyClientCertIfGiven`:

- `PKIAuthority.TLSConfig()` creates a certificate-backed TLS config with `ClientAuth: tls.RequireAndVerifyClientCert`.
- The service overrides the listener config to `tls.VerifyClientCertIfGiven` before starting the HTTPS server, which allows browser clients and public routes to connect without a client certificate while still accepting and verifying one when presented.
- Route protection is then delegated to the application-level auth middleware in `gateway_auth.go`, which classifies routes by `RouteAuthNone`, `RouteAuthMTLS`, `RouteAuthWebSession`, and `RouteAuthDual`.
- The middleware extracts the SAN-based SPIFFE identity from the peer certificate and rejects mismatched or spoofed caller identities. This is the actual enforcement point for authentication-sensitive routes; the TLS listener alone is not the final policy gate.

This means the current code deliberately uses a hybrid TLS posture rather than requiring a client cert on every HTTPS request: browsers can reach public pages and passkey flows without a cert, but mTLS-protected paths fail closed if the client certificate is missing or invalid.

### Port topology and router classification

The current Gateway runtime still uses a two-socket layout, but the actual router split is more precise than a single "public vs private" claim. The live implementation in `internal/services/gateway/gateway_http_router.go` registers distinct public bootstrap/discovery routes on the plain HTTP listener and all app/browser/API traffic on the HTTPS listener.

```text
         +---------------------------------------------------------------+
         |                     g8e Governance Gateway                     |
         +---------------------------------------------------------------+
                             |                              |
                 HTTP listener: 8080                 HTTPS listener: 8443
                             |                              |
                 +---------------------+         +------------------------+
                 | public bootstrap    |         | browser + API + mTLS   |
                 | - health            |         | - console             |
                 | - state             |         | - passkeys            |
                 | - CA bundle         |         | - SSE                 |
                 | - bootstrap        |         | - pub/sub             |
                 | - CLI recovery req |         | - governance          |
                 | - platform enroll  |         | - observe routes      |
                 | - deploy scripts    |         | - MCP/A2A ingress     |
                 | - redirect catchall |         | - auth middleware     |
                 +---------------------+         +------------------------+
```

The actual HTTP surface is intentionally narrow but not empty:

- `buildHTTPRouter()` exposes health, state, bootstrap, PKI discovery, deploy scripts, and token-scoped recovery/enrollment request/status/complete flows.
- It does not register the browser console, passkey challenge routes, or mTLS-only route families.
- Any remaining non-public path is redirected to HTTPS using a safe host check (`isSafeHost`) with a 301 redirect; this avoids open-redirect behavior and ensures a browser on port 8080 gets moved onto the TLS endpoint.

The HTTPS router is the main application surface; it hosts the console, browser-auth flows, mTLS APIs, operator dispatch, SSE, WebSocket pub/sub, observe routes, and the MCP/A2A ingress. The exact route classifications are enforced by `NewRouteAuthRegistry()` in `gateway_auth.go`.

Default ports remain `8080` (HTTP) and `8443` (HTTPS) via `internal/constants/ports.go`; the code also reserves loopback-only secondary endpoints for the public spectator and eval explorer defaults (`8081`, `8082`, `5173`, and `11434` for Ollama), but these are auxiliary listeners rather than core Gateway protocol surfaces.

### Enrollment, bootstrap, and recovery routing

The live routing model matches the code in `gateway_http_router.go` and the route registry in `gateway_auth.go`:

```text
Public/bootstrap discovery (HTTP)                 Protected owner/app routes (HTTPS)
-----------------------------------               ------------------------------------
GET  /health                                      POST /api/v1/pki/csr/sign
GET  /state                                       POST /api/v1/auth/cli/rotate
GET  /.well-known/g8e/pki/ca-bundle              POST /api/v1/auth/cli/refresh
GET  /.well-known/g8e/pki/fingerprint            GET  /api/v1/auth/cli/session
POST /api/v1/auth/bootstrap                      POST /api/v1/auth/platform-enrollments/pending
GET  /api/v1/auth/bootstrap/status               GET  /api/v1/auth/platform-enrollments/enrolled
POST /api/v1/auth/cli/recovery/request          POST /api/v1/auth/platform-enrollments/decision
GET  /api/v1/auth/cli/recovery/status           POST /api/v1/auth/platform-enrollments/revoke
POST /api/v1/auth/cli/recovery/complete         POST /api/v1/auth/cli/recovery/approve
POST /api/v1/auth/platform-enrollments/request  POST /api/v1/auth/cli/recovery/approve-cli
GET  /api/v1/auth/platform-enrollments/status
POST /api/v1/auth/platform-enrollments/complete
```

The critical implementation detail is this split:

1. **Bootstrap + CA discovery**: the initial Gateway bootstrap and trust-bundle discovery are public and operate over the HTTP listener. These paths are intentionally plain so a new machine can establish trust before a client certificate exists.
2. **Recovery and enrollment initiation**: `/request`, `/status`, and `/complete` for CLI recovery and platform enrollment are also public over HTTP. They rely on tokens and proof-of-possession rather than a validated session cookie or mTLS cert.
3. **Approval and privileged action**: the approval, decision, revoke, rotate, refresh, bind, and session routes are not registered on the HTTP router and instead require HTTPS and the appropriate application-layer auth classification (`RouteAuthWebSession`, `RouteAuthMTLS`, or `RouteAuthDual`).
4. **Browser passkeys**: browser-facing passkey registration and login flows are served from the HTTPS surface and set browser cookies only over the TLS listener.

### Cross-gateway cascading topologies

A downstream Gateway can participate as a governed Operator of an upstream Gateway, but the current implementation does this over the normal agent/Gateway model rather than by exposing arbitrary inbound listener ports on the downstream machine. The downstream runtime dials out to the upstream service using the standard outbound mTLS flow; the upstream gateway validates the remote operator identity via the same SPIFFE identity and certificate chain used for other workload identities.

This is the current operating model: the connection is outbound-only, the operator presents a certificate signed by the upstream trust chain, and the workload then receives commands on the scoped pub/sub channel for its session. The Gateway runtime continues to perform local verification before execution; the network transport is the trust boundary, not the execution boundary.

### Pub/sub communication patterns

The platform coordinates command dispatch and receipts through the HTTPS WebSocket pub/sub path (`/api/v1/pubsub/stream`) and the auth middleware in `gateway_auth.go`:

- **Endpoint**: `/api/v1/pubsub/stream` on the HTTPS listener.
- **Authentication**: the WebSocket upgrade is validated by the TLS peer cert and the route auth registry; the handler enforces identity matching against the caller's authenticated SPIFFE SAN.
- **Channel partitioning**:
  - `cmd:<operator_id>:<operator_session_id>` for gateway-to-operator commands
  - `results:<operator_id>:<operator_session_id>` for result publication
  - `heartbeat:<operator_id>:<operator_session_id>` for liveness telemetry
  - `receipts:<operator_id>:<operator_session_id>` for signed execution receipts
  - `audit:<operator_id>:<operator_session_id>` for audit and compliance events
- **Local execution**: when the target is the embedded Gateway operator, the runtime does not require a network socket; it routes the work through the in-process Gateway operator handling path instead.
- **Supplementary adapters**: Lattice or other adapter integrations may exist for specific deployments, but the native, authoritative execution path remains the outbound mTLS WebSocket stream rather than a separately trusted inbound listener.

### Network identity detection and SAN drift

At startup, the Gateway runs `network.Detector` to discover local identity candidates. The current implementation collects:

1. **IP addresses** from non-loopback host interfaces
2. **Hostnames and aliases** from the OS and `/etc/hosts`
3. **mDNS / Bonjour names** when present
4. **DNS PTR records** for discovered addresses
5. **SSH known-host names** and Windows-specific identifiers (NetBIOS / AD FQDN) when running on those platforms

The service then compares that detected identity set with the gateway serving certificate SANs. If new non-loopback IPs or DNS names are discovered and absent from the cert, `InitializePKIWithNames()` refreshes the serving certificate on the next authorized PKI refresh path so the certificate remains aligned with the current runtime identity. Dropped aliases are intentionally ignored to avoid certificate churn caused by transient or temporary identity changes.

This is the current implementation model for SAN drift: additive identity discovery triggers regeneration; removal of a previously valid alias does not reopen certificate churn.

### Windows/WSL LAN access

`scripts/configure-gateway-lan.ps1` (`-Action Inspect|Apply|Remove`) configures Windows port forwarding and firewall rules for Gateway ports 8080 and 8443 so LAN hosts can reach a Gateway running in WSL. Inspect is the default action; Apply and Remove require an elevated session.

## Anti-patterns

- **Bypassing application-layer mTLS**: Relying on transport-level certificate checks alone without verifying the SPIFFE URI SAN and session binding in application middleware (violates INV-NET-TLS-02).
- **Hardcoding host IPs instead of using `g8e.local`**: Connecting directly to IP addresses without setting `ServerName: g8e.local`, which fails TLS certificate verification (violates INV-NET-TLS-03).
- **Registering privileged endpoints on plain HTTP**: Exposing CSR signing, session manipulation, or recovery approval on port `8080` (violates INV-NET-PORT-03).
- **Exposing inbound listening ports on remote Operators**: Attempting to implement reverse HTTP, SSH, or gRPC listener sockets on managed operator nodes instead of utilizing outbound-only pub/sub WebSockets (violates INV-NET-COMM-01).
- **Trusting caller-provided identity headers**: Allowing `X-G8E-User-ID` or `X-G8E-Operator-ID` headers from the client to override the verified SPIFFE URI SAN extracted from the client certificate (violates INV-NET-ID-02).
- **Using non-P-256 keys for platform PKI**: Generating RSA or Ed25519 keys for the platform CA hierarchy or client CSRs (violates INV-NET-PKI-01).
- **Treating Lattice gRPC as equivalent to WebSocket command dispatch**: Documenting the supplementary Lattice gRPC adapter as a fully supported alternative to native pub/sub command dispatch (violates INV-NET-COMM-03).

## Links out

- [Authentication & Authorization Architecture](auth.md): Route classification, identity extraction, and session lifecycle.
- [Gateway Architecture](gateway.md): Gateway admission control, operational modes, and policy decision points.
- [Operator Architecture](operator.md): Remote operator execution, L4 Warden verification, and L5 Actuator receipts.
- [SSE Streaming](sse.md): Real-time Server-Sent Events architecture, event types, and consumer routing.
- [Ensemble Architecture](ensemble.md): Python agentic ensemble and SSE broadcast mechanics.
- [Console Architecture](console.md): Gateway-embedded browser console architecture and authentication.
- [Public Spectator Architecture](public_spectator.md): Read-only public mirror and evaluation explorer.
- [Unified Docker Stack](../guides/unified_stack.md): Container networking, namespace isolation, and host identity bind mounts.
- [Troubleshooting Guide](../devs/troubleshooting.md): PKI diagnostics, certificate recovery, and connection debugging.
- [Protocol Specification](../../protocol/docs/spec.md): Wire protocols, governance envelope schemas, and message types.
