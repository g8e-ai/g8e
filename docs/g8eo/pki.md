# PKI & Identity

The **g8eo Operator** owns the platform's Public Key Infrastructure (PKI). It acts as the sovereign root Certificate Authority (CA) for all platform participants, enforcing strict mutual TLS (mTLS) for all control-plane communication.

## Core Principles

- **Sovereign Authority**: The Operator is the only entity permitted to sign certificates. It maintains a multi-layer hierarchy with intermediate CAs for isolation.
- **CSR-Based Enrollment**: Participants enroll by submitting a Certificate Signing Request (CSR). Long-lived API keys are deprecated for identity; the platform relies on short-lived, session-bound certificates.
- **Workload Identity (SPIFFE)**: Every certificate is bound to a SPIFFE-compatible URI Subject Alternative Name (SAN) defining the component's role and session.
- **Fail-Closed Revocation**: Every mTLS request is checked against a real-time revocation list. Revoked certificates are rejected at the TLS handshake or middleware boundary.
- **Hardware Binding**: Certificates are cryptographically bound to a unique `system_fingerprint` generated during enrollment.

## PKI Hierarchy

The Operator manages a structured hierarchy in `.g8e/pki` to ensure isolation between different participants:

- **Root CA**: The foundational trust anchor, used only to sign intermediate CAs.
  - Path: `.g8e/pki/root/root_ca.crt`
- **Intermediate CAs**: Scoped authorities that sign leaf certificates.
  - **Hub CA**: Signs service certificates for the Operator itself (e.g., `operator-listen`).
  - **Operator CA**: Signs certificates for Satellite operators during enrollment.
  - **Bootstrap CA**: Signs temporary certificates used during the initial discovery phase.
- **Trust Bundles**: Combinations of root and intermediate certificates used for verification.
  - Path: `.g8e/pki/trust/hub-bundle.pem` (Root + Hub Intermediate)

## Identity Schemes

Client identities follow the SPIFFE URI scheme, encoded in the certificate's URI SAN. These are generated using the `protocol.WorkloadIdentity` helper to ensure format consistency across all components:

| Role | Helper Function | URI SAN Pattern |
|---|---|---|
| **Operator (Satellite)** | `OperatorSPIFFEID(organizationID, operatorID, sessionID)` | `spiffe://g8e.local/operator/<organization_id>/<operator_id>/<operator_session_id>` |
| **CLI (BYO Client)** | `CLISPIFFEID(userID, sessionID)` | `spiffe://g8e.local/cli/<user_id>/<cli_session_id>` |
| **Application (Agent)** | `AppSPIFFEID(operatorID)` | `spiffe://g8e.local/app/<operator_id>` |
| **Hub (Operator Listen)** | `HubSPIFFEID()` | `spiffe://g8e.local/hub/operator-listen` |

### CLI vs Operator Identity Separation

The CLI is a logically separate principal from the operator agent and has its own distinct SPIFFE identity. These are generated using `protocol.WorkloadIdentity`:

- **Operator certificates** authenticate the host agent and are bound to `operator_session_id`. The operator agent represents the sovereign host that executes mutations.
  - Helper: `OperatorSPIFFEID(organizationID, operatorID, sessionID)`
  - Path: `~/.g8e/operator.crt`, `~/.g8e/operator.key`
- **CLI certificates** authenticate BYO/CLI clients and are bound to `cli_session_id`. The CLI is a client tool that issues commands and receives SSE events.
  - Helper: `CLISPIFFEID(userID, sessionID)`
  - Path: `~/.g8e/cli.crt`, `~/.g8e/cli.key`

This separation ensures that:
- CLI sessions cannot impersonate operator agents.
- Operator sessions cannot drain another client's event stream (SSERoutes are bound to CLI sessions).
- Each principal has a cryptographically distinct identity for audit and authorization.

During enrollment (via `./g8e login` or `./g8e platform start`), the client generates **two distinct private keys** and submits **two distinct CSRs**. The Operator signs both, returning separate certificate chains for each workload role.

## Enrollment Lifecycle

The enrollment process transitions a participant from "untrusted" to "mTLS-verified":

1.  **Trust Verification**: The enrolling client fetches the Hub's root CA fingerprint from `GET /.well-known/pki/fingerprint` to verify the Hub's identity.
2.  **Registration Request**: The client presents a one-time device-link token and a locally generated `system_fingerprint` to the **Bootstrap Port (9003)**.
3.  **CSR Submission**: The client generates **two private keys** (Operator and CLI) and submits **two CSRs** (`csr_pem` for Operator, `cli_csr_pem` for CLI).
4.  **Issuance**: The Hub verifies the token and fingerprint, signs both CSRs using the **Operator Intermediate CA** (with role-specific URI SANs), and returns both certificate chains (`operator_cert` and `cli_cert`).
5.  **Steady State**: The client uses the `cli_cert` for CLI-based operations (like chat or management) and the `operator_cert` for host-side agent operations. Both are used for mTLS communication on the **API Port (9000)** and **Pub/Sub Port (9001)** depending on the routing target.

## Security Controls

### Transport Security
The platform enforces **TLS 1.3 only**. Older TLS versions and insecure cipher suites are explicitly rejected.

### Mutual TLS (mTLS)
mTLS is mandatory for all mutation and control-plane routes. The Operator's `ListenService` rejects any request on ports 9000 and 9001 that does not provide a valid client certificate signed by a trusted intermediate CA. The **Bootstrap Port (9003)** is an exception and serves plain HTTP to allow for initial trust discovery and CA certificate download.

### Revocation
Revocation state is stored in the Operator's database (`revoked_certificates` collection). 
- **Check-on-Handshake**: Certificates are verified against the revocation list during the TLS handshake or by middleware.
- **Revocation Bundles**: The Operator provides a signed JSON bundle of revoked serials for external verification.

### System Fingerprinting
The `system_fingerprint` is a stable identifier generated from:
- Operating System & Architecture
- CPU Core Count
- Machine ID (from `/etc/machine-id` or similar)
- Hostname

If a valid certificate is presented from a host with a mismatched fingerprint, the transaction is rejected.

## Warden Public Key Export

The **Warden** (the governance execution boundary) signs all mutation receipts with an Ed25519 key. For external verification (e.g., by the evals harness), the Warden's public key is exported at Operator bootstrap in listen mode.

### Export Location

The Warden public key is written to the PKI directory in two formats:

- **PEM format**: `.g8e/pki/warden_pub.pem`
  - Standard PEM-encoded public key for use with cryptographic libraries
- **JSON format**: `.g8e/pki/warden_pub.json`
  - JSON object containing `key_id`, `public_key` (hex-encoded), and `algorithm` fields

### Usage

The evals harness loads the Warden public key from these files to verify signed receipts for EVAL_ANSWER actions. The public key is exported automatically when the Operator starts in listen mode (`--listen`).

### Key Lifecycle

- The Warden signing key is generated once during first boot and stored in the platform secrets (`.g8e/secrets/warden_signing_key`)
- The public key is deterministic: it is derived from the private key and exported on every startup
- The `key_id` in the JSON format is the hex-encoded public key itself, used for signer identification in governance metadata
