# PKI & Identity

The **g8eo Operator** owns the platform's Public Key Infrastructure (PKI). It acts as the root Certificate Authority (CA) for all platform participants, enforcing strict mutual TLS (mTLS) for all control-plane communication.

## Core Principles

- **Sovereign Authority**: The Operator (in Hub mode) is the only entity permitted to sign certificates.
- **CSR-Based Enrollment**: New operators and applications enroll by submitting a Certificate Signing Request (CSR). No long-lived API keys are used for identity.
- **URI SAN Identity**: Every certificate is bound to a SPIFFE-compatible URI Subject Alternative Name (SAN) that defines the component's role and session.
- **Revocation-First**: Every mTLS request is checked against the Operator's internal revocation list. Revoked certificates fail closed immediately.

## Identity Schemes

Client identities follow a fixed SPIFFE URI scheme:

| Role | URI SAN Pattern |
|---|---|
| **Operator** | `spiffe://g8e.local/operator/<organization_id>/<operator_id>/<operator_session_id>` |
| **Application** | `spiffe://g8e.local/app/<app_id>` |

## Enrollment Lifecycle

1.  **Trust Bootstrap**: A new Satellite or BYO client fetches the Hub's trust bundle from `http://<hub>/trust`.
2.  **Challenge**: The client presents a one-time device-link token to the Hub's bootstrap port (80).
3.  **CSR Submission**: The client generates a local keypair and submits a CSR to the Hub.
4.  **Issuance**: The Hub verifies the token, signs the CSR, and returns a short-lived mTLS certificate bound to the requested identity.
5.  **Steady State**: The client uses the issued certificate for all subsequent communication on ports 9000 (API) and 9001 (Pub/Sub).

## Certificate Management

The Operator manages the PKI hierarchy in the `.g8e/pki` directory:

- `ca.crt` / `ca.key`: The platform root CA.
- `hub.crt` / `hub.key`: The Hub's identity certificate.
- `trusted_signers/`: Ed25519 public keys for Layer 2 (L2) consensus verification.
- `revocation_bundle.pem`: The current list of revoked certificates.

## Hardware Fingerprinting

For Satellites, the issued certificate is cryptographically bound to a hardware fingerprint generated at enrollment. If a certificate is presented from a host with a mismatched fingerprint, the transaction is rejected at the protocol boundary.
