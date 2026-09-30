---
doc_id: network
title: Network Architecture
audience: maintainers and coding agents
status: current
last_updated: 2026-09-28
version: v2.2.3
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

The platform operates an internal four-tier Public Key Infrastructure (PKI) managed by the Governance Gateway:

| Tier | Certificate Subject | Issuer | Purpose | Validity | Key Spec |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **Root CA** | `g8e Root CA` | Self-signed | Platform root trust anchor | 3650 days | ECDSA P-256 |
| **Hub Intermediate CA** | `g8e Hub Intermediate CA` | `g8e Root CA` | Signs Gateway serving certificates | 3650 days | ECDSA P-256 |
| **Operator Intermediate CA** | `g8e Operator Intermediate CA` | `g8e Root CA` | Signs workload leaves (operator, CLI, app) | 3650 days | ECDSA P-256 |
| **Peer Intermediate CA** | `g8e Gateway Peer Intermediate CA` | `g8e Root CA` | Signs federated gateway peering certificates | 3650 days | ECDSA P-256 |
| **Serving Certificate** | `operator-gateway` | `g8e Hub Intermediate CA` | TLS serving identity for Gateway listeners | 90 days | ECDSA P-256 |
| **Workload Leaves** | Dynamic (per identity) | `g8e Operator Intermediate CA` | Mutual TLS client/server authentication | 7 days | ECDSA P-256 |
| **Peer Certificates** | `gateway-peer` | `g8e Gateway Peer Intermediate CA` | Cross-gateway federated peering | 90 days | ECDSA P-256 |

The Hub and Operator intermediate CAs are strictly partitioned to enforce a blast-radius boundary. Compromise or rotation of the workload-issuing intermediate does not invalidate the Gateway's serving certificate, and rotation of the serving identity does not revoke existing client leaf certificates.

All certificates enforce ECDSA P-256. CSRs submitted to the Gateway with non-P-256 keys or invalid signatures fail verification immediately.

Revocation is checked during every mTLS request against the persistent database collection `revoked_certificates`. The Gateway generates a standard X.509 Certificate Revocation List (CRL) signed by the Operator Intermediate CA, served at `/.well-known/g8e/pki/crl` and `/api/v1/pki/revocation-bundle`. When an enrolled platform workload is revoked via `POST /api/v1/auth/platform-enrollments/revoke`, the Gateway writes the certificate serial to the revocation store, deletes associated application policies, and calls `Connections.DisconnectIdentity` to terminate active pub/sub WebSockets bound to that SPIFFE identity.

### Workload identity format (SPIFFE)

Every active component in the platform receives a cryptographically verifiable SPIFFE ID embedded as a URI SAN in its leaf certificate:

```text
spiffe://g8e.local/<type>/<identity-segments>
```

Canonical formats defined in `protocol/workload_identity.go`:

| Workload Type | SPIFFE ID Format | Description |
| :--- | :--- | :--- |
| **Governed Operator** | `spiffe://g8e.local/operator/<organization_id>/<operator_id>/<operator_session_id>` | Outbound execution worker bound to an organization, operator, and session. |
| **CLI / BYO Client** | `spiffe://g8e.local/cli/<user_id>/<cli_session_id>` | Interactive CLI tool or developer client session. |
| **Application / Agent** | `spiffe://g8e.local/app/<operator_id>` | Delegated tool, agent, or application running under an operator. |
| **Ensemble (`g8ee`)** | `spiffe://g8e.local/app/g8ee` | System-level event broker authorized to push SSE events across all sessions. |
| **User Principal** | `spiffe://g8e.local/user/<user_id>` | Approving owner identity embedded as secondary SAN on platform-enrolled application credentials. |
| **Governance Gateway** | `spiffe://g8e.local/hub/operator-listen` | Gateway hub identity embedded in the serving certificate URI SAN. |
| **Gateway Peer** | `spiffe://g8e.local/gateway/<gateway_id>` | Federated gateway peer identity. |

The ensemble identity (`spiffe://g8e.local/app/g8ee`) is a recognized platform service identity authorized to deliver Server-Sent Events to any active session regardless of specific operator binding.

### Transport security and mTLS enforcement

Network transport provides identity assurance for the platform's five-layer interlock pipeline (L1 Doctrine, L2 Consensus, L3 Notary, L4 Warden, L5 Actuator):

- **TLS 1.3 Minimum**: TLS 1.3 is required for all encrypted traffic. Older TLS versions are rejected.
- **Hybrid Client Authentication**: The Gateway's public TLS listener uses `tls.VerifyClientCertIfGiven`. The TLS handshake requests client certificates and verifies them against the Gateway CA pool when presented, but does not abort if a browser client provides no certificate.
- **Application-Layer Enforcement**: The Gateway's unified authentication middleware evaluates each route against the `RouteAuthRegistry`. Routes classified as `RouteAuthMTLS` act as fail-closed barriers, rejecting requests without a verified certificate and SPIFFE URI SAN. Dual-auth routes (`RouteAuthDual`) check mTLS first, falling back to valid browser session cookies.
- **Identity Binding**: The middleware extracts the authenticated SPIFFE ID directly from the TLS peer certificate, resolving user ID, CLI session ID, operator session ID, or app ID. Callers cannot spoof identity via request headers; headers that contradict the certificate or persisted session bindings are rejected.

### Port topology and router classification

The Governance Gateway uses a consolidated two-port architecture to separate discovery and bootstrap from the primary execution boundary:

```text
       +-------------------------------------------------------------+
       |                  g8e Governance Gateway                     |
       +-------------------------------------------------------------+
                       |                             |
              Port 8080 (Plain HTTP)        Port 8443 (Hybrid TLS 1.3)
                       |                             |
        +--------------+--------------+      +-------+--------------------+
        | Health & State Discovery    |      | Console SPA & Static Web   |
        | CA Bundle & Fingerprint     |      | Browser Passkey Auth       |
        | Unauthenticated Bootstrap   |      | Authenticated mTLS API     |
        | Token Recovery / Enrollment |      | WebSocket Pub/Sub Stream   |
        | 301 Catch-All -> HTTPS      |      | Governed MCP / A2A Ingress |
        +-----------------------------+      +----------------------------+
```

Default ports defined in `internal/constants/ports.go`:

| Surface | Port | Protocol | Classification | Purpose |
| :--- | :--- | :--- | :--- | :--- |
| **HTTP Surface** | `8080` | Plain HTTP | Public / Token-Scoped | Serves `/health`, `/state`, CA bundle discovery (`/.well-known/g8e/pki/ca-bundle`), Root CA fingerprint (`/.well-known/g8e/pki/fingerprint`), initial bootstrap (`/api/v1/auth/bootstrap`), token-scoped CLI recovery request/status/complete, token-scoped platform enrollment request/status/complete, binary downloads, and installation shell scripts. All other paths redirect to HTTPS. |
| **HTTPS Surface** | `8443` | Hybrid TLS 1.3 | mTLS / WebSession / Dual / Public | Primary execution boundary. Serves Console SPA, WebAuthn passkey registration/login, CRL (`/.well-known/g8e/pki/crl`), CSR signing (`/api/v1/pki/csr/sign`), all operator dispatch APIs, pub/sub WebSocket (`/api/v1/pubsub/stream`), SSE event streams, and governed MCP/A2A ingress. |

Startup fails if multiple logical services configure the same port (`len(usage) > 1`).

Auxiliary default ports started in Docker Compose or optional Gateway subprocesses:
- **Ensemble (`g8ee`)**: `8000` (`constants.EnsembleDefaultPort`)
- **Dashboard (`g8ed`)**: `3000` (`constants.DashboardDefaultPort`)
- **Public Spectator Private Ingest**: `8081` (`constants.PublicSpectatorPrivatePort`, loopback default)
- **Public Spectator Public Read**: `8082` (`constants.PublicSpectatorPublicPort`, loopback default)
- **Evaluation Explorer SPA**: `5173` (`constants.EvalExplorerDefaultPort`, loopback default)
- **Ollama Inference Backend**: `11434` (`constants.InferenceOllamaDefaultPort`, loopback default)

### Enrollment, bootstrap, and recovery routing

The Gateway divides authentication surfaces into public discovery endpoints and privileged owner-authenticated endpoints:

```text
Discovery Surface (Port 8080 & 8443)          Privileged Owner Surface (Port 8443 Only)
------------------------------------          -----------------------------------------
POST /api/v1/auth/bootstrap                   POST /api/v1/pki/csr/sign (mTLS only)
POST /api/v1/auth/cli/recovery/request        POST /api/v1/auth/cli/recovery/approve (WebSession)
GET  /api/v1/auth/cli/recovery/status         POST /api/v1/auth/cli/recovery/approve-cli (mTLS)
POST /api/v1/auth/cli/recovery/complete       POST /api/v1/auth/cli/rotate (mTLS only)
POST /api/v1/auth/platform-enrollments/request POST /api/v1/auth/cli/refresh (mTLS only)
GET  /api/v1/auth/platform-enrollments/status  GET  /api/v1/auth/cli/session (mTLS only)
POST /api/v1/auth/platform-enrollments/complete POST /api/v1/auth/platform-enrollments/pending (Dual)
                                              POST /api/v1/auth/platform-enrollments/decision (Dual)
                                              POST /api/v1/auth/platform-enrollments/revoke (Dual)
```

1. **CA Discovery**: Clients retrieve trust bundle certificates via `GET /.well-known/g8e/pki/ca-bundle` and verify the Root CA SHA-256 fingerprint via `GET /.well-known/g8e/pki/fingerprint`.
2. **Initial Bootstrap**: When the Gateway has no registered users, the first caller submits an ECDSA P-256 CSR to `POST /api/v1/auth/bootstrap`, provisioning the root owner and returning initial credentials.
3. **CLI Recovery**: On an already-bootstrapped Gateway, a new or recovering CLI submits a CSR to `POST /api/v1/auth/cli/recovery/request`, receiving an opaque lookup token. The CLI polls `GET /api/v1/auth/cli/recovery/status`. An active owner approves the request via the Console SPA (`POST /api/v1/auth/cli/recovery/approve` using web-session cookie) or headlessly via `g8e auth approve-recovery <token>` (`POST /api/v1/auth/cli/recovery/approve-cli` using mTLS). The recovering CLI completes issuance via `POST /api/v1/auth/cli/recovery/complete`.
4. **Platform Workload Enrollment**: Unenrolled background services (dashboard, ensemble, remote operator) submit a CSR to `POST /api/v1/auth/platform-enrollments/request`, poll status, and complete issuance after owner approval via `POST /api/v1/auth/platform-enrollments/decision`.
5. **Passkey Enrollment**: Browser WebAuthn registration is initiated by an enrolled CLI through `POST /api/v1/auth/enrollment-token/generate` (mTLS-only). The browser validates the token via `POST /api/v1/auth/enrollment-token/validate` and completes challenge/verify under `/api/v1/auth/passkeys/enrollment/register/`.
6. **Windows Certificate Store**: On Windows hosts, `g8e auth enroll user` detects the platform and automatically imports the signed client certificate into the `CurrentUser\Personal` store, enabling seamless integration with Windows Hello and CNG.

### Cross-gateway cascading topologies

Because the `g8e` binary contains both Gateway and Operator runtimes, a downstream Gateway can enroll as a Governed Operator under an upstream Gateway by running `operator start -e <upstream-gateway>`:

1. **CSR Submission**: The downstream Gateway issues an operator CSR and submits it through the upstream Gateway's platform enrollment request endpoint.
2. **Approval**: The upstream owner reviews and approves the enrollment request via the upstream Console or CLI.
3. **Identity Grant**: The downstream Gateway receives an operator certificate signed by the upstream Operator Intermediate CA, embedding `spiffe://g8e.local/operator/<org>/<op>/<session>`.
4. **Outbound Channel**: The downstream Gateway dials out to the upstream Gateway's `/api/v1/pubsub/stream` endpoint over mTLS, subscribing to its assigned `cmd:<operator_id>:<operator_session_id>` channel.
5. **Local Re-Verification**: Governed commands received from the upstream Gateway pass through the downstream Gateway's local L4 Warden and L5 Actuator, validating signatures and policy independently before execution.

This architecture enables multi-tier cascading deployments: edge gateways dial upstream to regional gateways, which dial upstream to central hubs. At each hop, the connection is strictly outbound-only mTLS, leaving no inbound ports exposed at the edge.

### Pub/sub communication patterns

The platform coordinates real-time command dispatch, execution receipts, and telemetry via persistent WebSocket pub/sub on the HTTPS listener:

- **Endpoint**: `/api/v1/pubsub/stream` on the HTTPS port (`8443`).
- **Authentication**: Strict mTLS required at WebSocket upgrade time.
- **Channel Partitioning**:
  - `cmd:<operator_id>:<operator_session_id>`: Gateway dispatches governed commands to the designated Operator session.
  - `results:<operator_id>:<operator_session_id>`: Operator publishes terminal command execution results back to the Gateway.
  - `heartbeat:<operator_id>:<operator_session_id>`: Operator sends periodic liveness pings (default interval: 30 seconds). Heartbeats are lightweight transport telemetry and do not produce compliance ledger commitments.
  - `receipts:<operator_id>:<operator_session_id>`: Operator publishes signed L5 execution receipts for audit capture.
  - `audit:<operator_id>:<operator_session_id>`: Gateway streams compliance and audit events.
- **Channel Access Control**: The WebSocket handler validates that every subscribe and publish request targets channels matching the caller's authenticated SPIFFE identity URI SAN.
- **Local Loopback Delivery**: When a transaction targets the Gateway's embedded Operator, the Gateway bypasses network sockets and routes the envelope through an in-process pub/sub client directly to the embedded L4/L5 execution pipeline.
- **Lattice Integration**: The Operator CLI provides configuration flags (`--lattice-endpoint`, `--lattice-client-id`, etc.) for an Anduril Lattice gRPC task stream. However, the Lattice adapter task handler does not provide completed governed command execution; WebSocket pub/sub remains the primary authoritative execution transport.

### Network identity detection and SAN drift

At startup, the Gateway initializes `network.Detector` to discover the local host's network identities:

1. **Network Interface IPs**: Collects all IPv4 addresses from non-loopback network interfaces.
2. **Host-Mounted Identity Bindings**: In containerized deployments (such as Docker Compose), the host's `/etc/hosts` and `/etc/hostname` are bind-mounted read-only at `/etc/hosts.host` and `/etc/hostname.host` (`internal/constants/paths.go:PathEtcHostsHost`). The detector reads these files first, merging and de-duplicating IP addresses and host aliases with container-local configurations.
3. **System Hostnames and Aliases**: Gathers machine hostnames, `/etc/hosts` aliases, and local `.local` mDNS names.
4. **DNS and Reverse Lookups**: Resolves reverse DNS PTR records and extracts hostnames from `~/.ssh/known_hosts`.
5. **Windows Identifiers**: On Windows systems, extracts NetBIOS names and Active Directory domain FQDNs.

**Additive SAN Drift Regeneration**:
The Gateway inspects its existing serving certificate against the detected identity:
- Default DNS SANs include `localhost`, `g8e.local`, and `operator`.
- Default IP SANs include `127.0.0.1`.
- If newly detected non-loopback IPs or DNS hostnames are absent from the certificate (additive drift), the Gateway regenerates the serving certificate on-the-fly and saves the updated chain to disk. Dropped or missing host aliases do not trigger regeneration, avoiding certificate churn during temporary network reconfigurations.

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
- [Dashboard Architecture](dashboard.md): Operator web console architecture and authentication.
- [Public Spectator Architecture](public_spectator.md): Read-only public mirror and evaluation explorer.
- [Unified Docker Stack](../guides/unified_stack.md): Container networking, namespace isolation, and host identity bind mounts.
- [Troubleshooting Guide](../devs/troubleshooting.md): PKI diagnostics, certificate recovery, and connection debugging.
- [Protocol Specification](../../protocol/docs/spec.md): Wire protocols, governance envelope schemas, and message types.
