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

Client identities follow the SPIFFE URI scheme, encoded in the certificate's URI SAN:

| Role | URI SAN Pattern |
|---|---|
| **Operator (Satellite)** | `spiffe://g8e.local/operator/<organization_id>/<operator_id>/<operator_session_id>` |
| **Application (Agent)** | `spiffe://g8e.local/app/<operator_id>` |
| **Hub (Operator Listen)** | `spiffe://g8e.local/hub/operator-listen` |

## Enrollment Lifecycle

The enrollment process transitions a participant from "untrusted" to "mTLS-verified":

1.  **Trust Verification**: The enrolling client fetches the Hub's root CA fingerprint from `GET /.well-known/pki/fingerprint` to verify the Hub's identity.
2.  **Registration Request**: The client presents a one-time device-link token and a locally generated `system_fingerprint` to the **Bootstrap Port (9002)**.
3.  **CSR Submission**: The client generates a private key (which never leaves the host) and submits a CSR.
4.  **Issuance**: The Hub verifies the token and fingerprint, signs the CSR using the **Operator Intermediate CA**, and returns the certificate chain.
5.  **Steady State**: The client uses the issued certificate for all subsequent mTLS communication on the **API Port (9000)** and **Pub/Sub Port (9001)**.

## Security Controls

### Transport Security
The platform enforces **TLS 1.3 only**. Older TLS versions and insecure cipher suites are explicitly rejected.

### Mutual TLS (mTLS)
mTLS is mandatory for all mutation and control-plane routes. The Operator's `ListenService` rejects any request on ports 9000 and 9001 that does not provide a valid client certificate signed by a trusted intermediate CA.

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
