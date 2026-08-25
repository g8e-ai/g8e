# PKI & Trust

## Overview

The g8e platform implements a hierarchical Public Key Infrastructure (PKI) and cryptographic identity framework to establish zero-trust communication and verifiable provenance across the Governance Gateway (`g8eg`), Governed Operators (`g8eo`), human operators via the CLI/Console, and the Agentic Ensemble (`g8ee`). The architecture separates identity verification and transport security from consensus and transaction authorization: mutual TLS (mTLS) uses X.509 certificates with elliptic-curve cryptography for transport security and device/workload identity, while consensus deliberation and transaction execution use detached signature algorithms for non-repudiation.

## Cryptographic Standards and Algorithms

The platform enforces cryptographic standards tailored to specific security layers:

- **X.509 PKI & Mutual TLS (mTLS)** — All TLS certificates across the platform (Root CA, Intermediate CAs, Service Certificates, and Leaf Certificates) use **ECDSA with the NIST P-256 (secp256r1) curve and SHA-256**. For FIPS 140-3 compliance, certificates utilizing Ed25519 signatures are explicitly rejected in the TLS trust chain (`ErrPKIEd25519CertRejected`). TLS configurations enforce TLS 1.3 as the minimum protocol version (`tls.VersionTLS13`) alongside strict curve preferences.
- **L2 Consensus & Governance Signatures** — Tribunal members, consensus nodes, and agent harnesses use dedicated **Ed25519** keypairs to sign deterministic votes over `<transaction_hash>|<decision>` and `GovernanceEnvelope` transactions.
- **L3 Human Notary** — Human authorization assertions leverage **WebAuthn / FIDO2** hardware passkeys (supporting ECDSA ES256 and Ed25519) or out-of-band Ed25519 signatures for outbound operator approvals.

## Trust Hierarchy and Certificate Authorities

The platform PKI is structured as a two-tier Certificate Authority hierarchy managed in-process by the Gateway's `PKIAuthority`:

```
                           ┌────────────────────────┐
                           │        Root CA         │
                           │  (ECDSA P-256, 10 yr)  │
                           └───────────┬────────────┘
                                       │
        ┌──────────────────────────────┼──────────────────────────────┐
        │                              │                              │
        ▼                              ▼                              ▼
┌──────────────────────┐    ┌──────────────────────┐    ┌──────────────────────┐
│  Hub Intermediate    │    │ Operator Intermediate│    │ Gateway Peer Interm. │
│ (ECDSA P-256, 10 yr) │    │ (ECDSA P-256, 10 yr) │    │ (ECDSA P-256, 10 yr) │
└──────────┬───────────┘    └──────────┬───────────┘    └──────────┬───────────┘
           │                           │                           │
           ▼                           ▼                           ▼
┌──────────────────────┐    ┌──────────────────────┐    ┌──────────────────────┐
│ Gateway Service Cert │    │ Operator Leaf Certs  │    │ Gateway Peer Certs   │
│ (ECDSA P-256, 90 d)  │    │ (ECDSA P-256, 7 d)   │    │ (ECDSA P-256, 90 d)  │
└──────────────────────┘    ├──────────────────────┤    └──────────────────────┘
                            │ CLI Client Certs     │
                            │ (ECDSA P-256, 7 d)   │
                            ├──────────────────────┤
                            │ Delegated App Certs  │
                            │ (ECDSA P-256, 1 hr)  │
                            ├──────────────────────┤
                            │ Certificate Rev. List│
                            │ (CRL, ECDSA, 24 hr)  │
                            └──────────────────────┘
```

### Certificate Authority Tiers

| Authority | Common Name | File Path | Key Algorithm | Validity | Purpose |
| --- | --- | --- | --- | --- | --- |
| **Root CA** | `g8e Root CA` | `.g8e/pki/root/root_ca.crt` | ECDSA P-256 | 10 years (3,650 days) | Self-signed trust anchor for the platform trust domain. Private key managed in `SecretManager` / KeyStore. |
| **Hub Intermediate CA** | `g8e Hub Intermediate CA` | `.g8e/pki/authorities/hub_ca.crt` | ECDSA P-256 | 10 years (3,650 days) | Issues server TLS certificates for Gateway listening surfaces. |
| **Operator Intermediate CA** | `g8e Operator Intermediate CA` | `.g8e/pki/authorities/operator_ca.crt` | ECDSA P-256 | 10 years (3,650 days) | Issues leaf certificates for Governed Operators, CLI clients, and applications. Signs the X.509 Certificate Revocation List (CRL). |
| **Gateway Peer Intermediate CA** | `g8e Gateway Peer Intermediate CA` | `.g8e/pki/authorities/gateway_peer_ca.crt` | ECDSA P-256 | 10 years (3,650 days) | Issues peer certificates for inter-gateway federation and clustering. |

### Issued Service and Leaf Certificates

| Certificate | Relative Path | Issuer CA | Validity | Description |
| --- | --- | --- | --- | --- |
| **Gateway Service Certificate** | `.g8e/pki/issued/hub/operator-gateway.crt` | Hub Intermediate | 90 days | Server TLS certificate presented on the Gateway HTTPS surface (port 8443). |
| **Operator Leaf Certificate** | `.g8e/pki/operator.crt` | Operator Intermediate | 7 days | Client certificate used by Governed Operators for outbound mTLS connections. |
| **CLI Client Certificate** | `.g8e/pki/operator-cli.crt` | Operator Intermediate | 7 days | Client certificate used by the CLI and local tools for mTLS authentication. |
| **Delegated App Certificate** | Ephemeral / in-memory | Operator Intermediate | 1 hour | Short-lived credential binding both application identity and requesting human user identity. |
| **Gateway Peer Certificate** | `.g8e/pki/issued/gateway-peer/` | Gateway Peer Intermediate | 90 days | Certificate for inter-gateway federation and mesh communication. |

## Workload Identity (SPIFFE URI SANs)

The platform embeds typed SPIFFE (Secure Production Identity Framework for Everyone) URI Subject Alternative Names (SANs) into all issued X.509 leaf certificates within the canonical trust domain `spiffe://g8e.local`.

| Identity Type | SPIFFE ID Format | Description |
| --- | --- | --- |
| **Operator** | `spiffe://g8e.local/operator/<org_id>/<operator_id>/<session_id>` | Governed Operator workload bound to an organization, operator instance, and session. |
| **CLI** | `spiffe://g8e.local/cli/<user_id>/<cli_session_id>` | Authenticated CLI client session bound to a human user and session identifier. |
| **Application** | `spiffe://g8e.local/app/<operator_id>` | Application or agent workload interacting with platform services. |
| **Ensemble (`g8ee`)** | `spiffe://g8e.local/app/g8ee` | Centralized agentic ensemble identity authorized to publish SSE events across operator sessions. |
| **Delegated App** | `spiffe://g8e.local/app/<app_name>` & `spiffe://g8e.local/user/<user_id>` | Dual-SAN certificate binding the acting application and the delegating human user. |
| **Gateway Peer** | `spiffe://g8e.local/gateway/<gateway_id>` | Gateway peer node participating in federation. |
| **User** | `spiffe://g8e.local/user/<user_id>` | Human principal identity for delegation contexts. |
| **Hub** | `spiffe://g8e.local/hub/operator-listen` | Gateway listener service identity. |

## Trust Bundles and Distribution

Trust bundles are maintained under `.g8e/pki/trust/` and distributed via unauthenticated plain HTTP discovery endpoints:

- **Gateway Trust Bundle (`g8eg-ca-bundle.pem`)** — The canonical trust bundle containing the Root CA, Hub Intermediate CA, Operator Intermediate CA, and Gateway Peer Intermediate CA certificates. Served at `GET /.well-known/g8e/pki/ca-bundle`.
- **Operator Bundle (`operator-bundle.pem`)** — Concentrated trust bundle containing the Root CA and Operator Intermediate CA certificates for operator-side verification.
- **Root Bundle Mirror (`root.pem`)** — Mirror containing the standalone Root CA certificate.
- **Trust Domain Metadata (`trust-domain.json`)** — JSON metadata declaring the SPIFFE trust domain (`g8e.local`).

### Discovery Endpoints (Plain HTTP Port 8080)

| Endpoint | Method | Output Format | Description |
| --- | --- | --- | --- |
| `/.well-known/g8e/pki/ca-bundle` | `GET` | `application/x-pem-file` | Serves the full Gateway trust bundle for bootstrapping mTLS connections. |
| `/.well-known/g8e/pki/fingerprint` | `GET` | `application/json` | Returns the SHA-256 fingerprint of the active Root CA for stale anchor detection. |
| `/.well-known/g8e/pki/crl` | `GET` | `application/pkix-crl` | Serves the active X.509 Certificate Revocation List (DER-encoded). |
| `/.well-known/g8e/bin/{filename}` | `GET` | `application/octet-stream` | Serves platform binaries for operator deployment. |
| `/g8e-deploy.sh` | `GET` | `text/plain` | Renders Linux deployment script pre-configured with Gateway connection parameters. |
| `/g8e-deploy.ps1` | `GET` | `text/plain` | Renders Windows deployment script pre-configured with Gateway connection parameters. |

## Certificate Lifecycle and Enrollment

Certificate enrollment follows a multi-phase workflow ensuring secure bootstrap and atomic credential management without pre-shared secrets.

```
┌───────────┐                     ┌───────────┐
│  Client   │                     │  Gateway  │
└─────┬─────┘                     └─────┬─────┘
      │                                 │
      │ 1. GET /.well-known/.../ca-bundle│ (HTTP 8080)
      ├────────────────────────────────>│
      │    Returns PEM Trust Bundle     │
      │<────────────────────────────────┤
      │                                 │
      │ 2. Generate ECDSA P-256 Keypair │
      │    Create CSR with SPIFFE SAN   │
      │                                 │
      │ 3. POST /api/v1/pki/csr/sign    │ (mTLS / Bootstrap)
      ├────────────────────────────────>│
      │    Validate Curve & Sign CSR    │
      │    Returns Leaf Cert + Chain    │
      │<────────────────────────────────┤
      │                                 │
      │ 4. Authenticated mTLS Session   │ (HTTPS 8443)
      │<═══════════════════════════════>│
```

### Enrollment State Machine

The CLI classifies local credential state into four operational states:

1. **Complete (Reuse)** — Local certificate, private key, and credentials JSON are present and valid. The CLI verifies that the local trust bundle matches the live Gateway root CA fingerprint. If matches, the identity is reused without minting a new certificate.
2. **Absent (Bootstrap)** — No local credentials exist. The CLI connects over plain HTTP, bootstraps trust, generates an enrollment token, and initiates the WebAuthn passkey ceremony.
3. **Partial (Recovery)** — Incomplete credentials on disk. Initiates human-approved recovery via `POST /api/v1/auth/cli/recovery/request` rather than silently overwriting.
4. **Corrupt (Recovery / Rotation)** — Invalid or mismatched credentials trigger human-approved recovery or rotation.

### Headless Enrollment

The `--headless` flag on `g8e auth enroll user` enables CLI-only enrollment for non-interactive and automated environments. Headless enrollment skips the browser passkey ceremony and OS trust store installation while delegating recovery approval to an existing enrolled CLI via `g8e auth approve-recovery <token>`.

### Management and Signing Endpoints (HTTPS Port 8443)

- `POST /api/v1/pki/csr/sign` — Internal mTLS endpoint for signing CSRs for operator, CLI, app, and gateway-peer identities. Enforces NIST P-256 curve policy.
- `POST /api/v1/pki/devices/enroll` — Device re-enrollment endpoint authenticated via existing client certificates.
- `POST /api/v1/pki/apps/delegated` — Mints short-lived (1 hour) delegated credentials binding application and requester identities.
- `POST /api/v1/auth/cli/rotate` — mTLS-protected endpoint issuing a replacement CLI certificate and revoking the prior certificate.
- `POST /api/v1/auth/cli/refresh` — mTLS-protected endpoint renewing an expired 7-day CLI session using a still-valid CLI certificate.

### Operator Certificate Renewal Loop

Governed Operators maintain an automated background renewal loop (`RunClientCertRenewalLoop`) that executes every 24 hours. When the active operator certificate enters the renewal threshold (within 24 hours of expiration), the operator generates a fresh P-256 keypair, constructs a renewal CSR, and submits a re-enrollment request to `/api/v1/pki/devices/enroll` over mTLS.

## Revocation and CRL Management

The Gateway maintains a real-time certificate revocation registry backed by the Document Store:

- **Revocation Storage** — Revoked certificate records are stored in the SQLite Document Store under the `revoked_certificates` collection, capturing serial number, revocation timestamp, and reason.
- **Revocation Endpoint** — Authorized administrators revoke certificates via `POST /api/v1/pki/certificates/revoke`.
- **CRL Generation** — The Gateway dynamically generates standard X.509 Certificate Revocation Lists (`GenerateCRL`) signed by the Operator Intermediate CA with `ECDSAWithSHA256`. CRLs carry a 24-hour validity period and are served at `GET /.well-known/g8e/pki/crl`.
- **mTLS Verification** — The Gateway TLS handshake and connection validators verify peer certificate serial numbers against the revocation store in real time via `IsRevoked`.

## Agent Key Management and Consensus Signatures

In addition to X.509 mTLS certificates, agents and Tribunal consensus members maintain cryptographic keys for transaction governance:

- **Consensus Keypairs** — Each Tribunal member (`axiom`, `concord`, `variance`, `pragma`, `nemesis`) and enrolled consensus node holds an independent Ed25519 keypair.
- **Deterministic Deliberation Signing** — Tribunal members evaluate candidate command intents against doctrine and sign their votes over `<transaction_hash>|<decision>`.
- **GovernanceEnvelope Authentication** — The ensemble signs outgoing `GovernanceEnvelope` payloads, ensuring non-repudiation and enabling the Operator L4 Warden to verify consensus quorum before host mutation.
- **Key Isolation** — Consensus member keys are isolated within the ensemble process or consensus service and never mixed with X.509 TLS private keys.

## Storage and Security Boundaries

The platform enforces strict isolation and file permissions for all PKI artifacts:

- **Private Key Storage** — Root CA and Intermediate CA private keys are stored securely in `SecretManager` / KeyStore memory and are never written to unencrypted disk files.
- **File System Permissions** — Private key files on disk (Operator and CLI keys) are written with restricted permissions (`0600` / `constants.PermFilePrivate`). Public certificates, trust bundles, and CRLs use standard read permissions (`0644` / `constants.PermFilePublic`).
- **Filesystem Confinement** — All runtime certificate file operations are mediated through `RuntimeFileService` and confined to the `.g8e/` directory structure.
- **Fail-Closed Security** — If trust bundle verification fails, certificate validation fails, or a revoked serial is detected, network connections and governance transactions immediately fail closed.

## Related

- [Architecture](architecture.md) — Multi-tier system architecture and service topology
- [Governance](governance.md) — Five-layer verification pipeline and consensus mechanics
- [Protocol](protocol.md) — Canonical wire contracts, SPIFFE identifiers, and GovernanceEnvelope specification
- [Storage](storage.md) — Data sovereignty, vault encryption, and Document Store collections
- [Agents](agents.md) — Agent persona definitions, Tribunal consensus, and role hierarchy
- [Thinking](thinking.md) — Provider reasoning, consensus deliberation, and thought signatures
- [Constants](constants.md) — Application constants, file paths, and error definitions
