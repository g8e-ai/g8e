---
doc_id: pki-ensemble
title: Application PKI & Enrollment
audience: maintainers and coding agents
status: current
last_updated: 2026-09-28
version: v2.2.3
owners:
  - docs/ensemble/
  - internal/cli/serve/platform_enrollment_client.go
  - ensemble/app/services/infra/app_enrollment_service.py
  - internal/services/gateway/platform_enrollment_*.go
related:
  - docs/architecture/network.md
  - docs/architecture/auth.md
  - docs/architecture/governance.md
  - docs/ensemble/architecture.md
  - docs/ensemble/storage.md
when_to_read: Implementing application enrollment, certificate renewal, trust discovery, enrollment state management, or understanding identity binding between applications and the Gateway.
do_not_use_for:
  - PKI hierarchy and cryptographic profile (docs/architecture/network.md)
  - SPIFFE identity format definition (docs/architecture/network.md)
  - Route classification and authentication enforcement (docs/architecture/auth.md)
  - Governance transaction verification (docs/architecture/governance.md)
---

# Application PKI & Enrollment

## Purpose

Documents how g8e applications (dashboard, ensemble, remote Operator, delegated agents) enroll with the Gateway, manage certificates, discover trust anchors, and maintain identity credentials. This guide complements the canonical [Network Architecture](../architecture/network.md) reference by covering the client-side lifecycle: enrollment state management, renewal, revocation, and runtime storage.

The Gateway owns the deployment PKI, enrollment records, and revocation state. Applications own their enrolled certificates, private keys, trust bundles, and resumable enrollment state in their own runtime volumes. The Operator owns execution evidence. In unified Compose deployments, the read-only `/operator-state` mount supplies selected bootstrap material for initialization; it is not a general-purpose host filesystem or execution channel.

## Quick index

- [Purpose](#purpose)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

Key concepts: [Trust discovery](#trust-discovery-and-certificate-bootstrap), [Platform enrollment](#platform-application-enrollment), [CLI lifecycle](#cli-identity-lifecycle), [Renewal](#renewal-and-revocation), [Delegated applications](#delegated-application-credentials).

## Invariants

Ids are stable. Append the next free number within each group; do not renumber.

### Application enrollment (`INV-ENROLL-APP`)

| ID | Rule |
| --- | --- |
| INV-ENROLL-APP-01 | Applications MUST generate their private key locally and never transmit it to the Gateway. CSRs and public-key fingerprints are submitted over the discovery surface; private keys remain under application control. |
| INV-ENROLL-APP-02 | Platform enrollment requests MUST be token-scoped and MUST expire after 30 minutes. Requests MUST be persisted in resumable state until completion or explicit denial. |
| INV-ENROLL-APP-03 | Issued certificates MUST contain the application identity and approving-user SPIFFE SANs. Platform application certificates MUST have 7-day validity; delegated applications MUST have 1-hour validity. |
| INV-ENROLL-APP-04 | g8ee MUST load an existing certificate or enroll during FastAPI startup. It MUST NOT proceed to ready state while enrollment is pending. Certificate renewal MUST occur at the 1-day threshold during startup and through lifecycle service checks. |

### Trust and discovery (`INV-ENROLL-TRUST`)

| ID | Rule |
| --- | --- |
| INV-ENROLL-TRUST-01 | The Gateway MUST publish root CA and fingerprint at `/.well-known/g8e/pki/ca-bundle` and `/.well-known/g8e/pki/fingerprint` on the plain HTTP discovery listener. These are unauthenticated bootstrap endpoints. |
| INV-ENROLL-TRUST-02 | Applications MUST verify the root fingerprint through a secure out-of-band channel before trusting the bundle on first contact across an untrusted network. |
| INV-ENROLL-TRUST-03 | Once enrolled, applications MUST persist and reuse the Gateway root CA to validate all subsequent TLS connections without re-fetching. |

### Certificate renewal (`INV-ENROLL-RENEW`)

| ID | Rule |
| --- | --- |
| INV-ENROLL-RENEW-01 | Operator certificates MUST check validity at startup and every 24 hours, and MUST re-enroll over mTLS when less than 24 hours remain. Renewal generates a new ECDSA P-256 key. |
| INV-ENROLL-RENEW-02 | g8ee application certificates MUST be renewed at the 1-day threshold during startup and through lifecycle checks. One-hour delegated credentials MUST never be reused merely because they remain technically unexpired. |
| INV-ENROLL-RENEW-03 | Renewal logic MUST distinguish between authenticated rotation (valid certificate, < 24 hours remaining, mTLS refresh) and recovery (expired, missing, or stale certificate requiring fresh enrollment). |

### Revocation (`INV-ENROLL-REVOKE`)

| ID | Rule |
| --- | --- |
| INV-ENROLL-REVOKE-01 | The active first owner MUST be able to revoke completed platform enrollments by request ID. Revocation MUST invalidate the issued identity, delete associated application policy, and disconnect active pub/sub connections bound to that SPIFFE identity. |
| INV-ENROLL-REVOKE-02 | Operator revocation MUST also revoke the companion CLI identity, deactivate both sessions, and mark the Operator terminated. Revocation records the actor, reason, timestamp, and governance evidence. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Platform enrollment flow | `internal/services/gateway/platform_enrollment_*.go` | Lifecycle tests and request/approval/complete state machine |
| g8ee enrollment service | `ensemble/app/services/infra/app_enrollment_service.py` | FastAPI startup integration test |
| Operator enrollment client | `internal/cli/serve/platform_enrollment_client.go` | CSR generation and remote enrollment flow |
| Trust discovery endpoints | `internal/services/gateway/pki_controller.go` | HTTP router registration for `/well-known/g8e/pki/*` |
| Certificate renewal logic | `internal/services/gateway/gateway_certs.go` | Validity check and SAN drift detection |
| CLI enrollment state machine | `internal/cli/auth/enrollment.go` and `enrollment_coordinator.go` | Decision matrix and transition logic |
| Revocation state persistence | `internal/services/gateway/platform_enrollment_service.go` | Revocation record storage and identity disconnection |

## Procedures

### Trust discovery and certificate bootstrap

Applications initiate enrollment by discovering the Gateway's root CA and verifying its identity:

1. **Fetch CA Bundle**: Call `GET /.well-known/g8e/pki/ca-bundle` over plain HTTP (default port 8080) or `GET /.well-known/g8e/pki/ca-bundle` over HTTPS to retrieve the Gateway's trust bundle.
2. **Verify Fingerprint (Out-of-Band)**: If this is first contact across an untrusted network, verify the root certificate's SHA-256 fingerprint through a secure channel (e.g., a deployment script, QR code, or administrator out-of-band confirmation) before trusting the bundle.
3. **Parse and Store**: Extract the root and intermediate CA certificates from the PEM bundle and persist them in the application's runtime trust directory.
4. **Proceed to Enrollment**: With the trusted CA bundle in place, the application can now perform TLS verification for all subsequent Gateway connections, including CSR submission and identity loading.

### Platform application enrollment

The dashboard, ensemble, and remote Operator use owner-approved enrollment after the Gateway has an owner:

1. **CSR Generation**: The application generates an ECDSA P-256 private key locally and creates a certificate signing request (CSR) containing the public key.
2. **Request Submission**: The application submits the CSR and public-key fingerprint to `POST /api/v1/auth/platform-enrollments/request` over the plain HTTP discovery surface (no client certificate required). The Gateway returns an opaque token-scoped requester token that expires after 30 minutes.
3. **Request Review**: An enrolled owner views pending requests via `g8e auth enroll pending` or the Console dashboard.
4. **Approval or Denial**: The owner approves with `g8e auth enroll approve <request-id> --yes` or denies with `g8e auth enroll deny <request-id> --yes`. Approval initiates a Gateway-side policy record and sends a 202 response indicating readiness for proof-of-possession.
5. **Proof of Possession**: After approval, the application proves possession of the private key by submitting it in a request to `POST /api/v1/auth/platform-enrollments/complete` over mTLS, including a signed proof. A retry after successful completion returns the same issued identity (idempotent).
6. **Credential Installation**: The Gateway returns the issued certificate chain, trust bundle, and session or application policy. The application atomically installs the certificate, key, and trust bundle in its runtime tree and becomes ready to serve.

On completion, the application may revoke the enrollment by presenting the request ID to `POST /api/v1/auth/platform-enrollments/revoke`. Revocation invalidates the issued identity and disconnects active sessions; for Operators, it also revokes the companion CLI identity and marks the Operator terminated.

Applications persist resumable pending state under `.g8e/pki/pending-enrollment/` with owner-only permissions until completion or denial. Component-specific state files allow independent enrollment tracking.

### CLI identity lifecycle

The `g8e auth enroll user` command runs an enrollment state machine that inspects local credentials and Gateway trust anchors:

| Local and Gateway state | Action |
| --- | --- |
| No local identity, empty Gateway | Bootstrap first owner and issue a CLI certificate |
| No local identity, initialized Gateway | Request recovery via Console or `g8e auth approve-recovery` |
| Valid identity with active session, matching root | Reuse identity (no new certificate) |
| Valid certificate expiring within 24 hours | Rotate certificate through mTLS |
| Expired certificate or stale root | Recover identity or bootstrap if empty |
| Partial, corrupt, or mismatched credentials | Recover identity or bootstrap if empty |

The coordinator validates the certificate, key, session metadata, and trust bundle as one managed set. Interactive enrollment installs the Gateway root in the OS trust store (unless `--no-system-trust`) and runs passkey registration. Headless enrollment (`--headless`) skips OS trust installation and browser passkey ceremony, creating an mTLS-only identity that cannot authenticate to the Console until passkey registration completes separately.

CLI certificates and sessions have 7-day validity. Refresh (`g8e auth refresh`) renews server-side session when the certificate is still valid. Logout removes local credentials but does not revoke the Gateway-side certificate or session.

### Delegated application credentials

A locally launched agent can request a delegated certificate through an enrolled CLI at `POST /api/v1/pki/apps/delegated`:

1. **CSR Submission**: The agent generates a P-256 private key and submits a CSR over mTLS using the CLI's authenticated connection.
2. **Certificate Issuance**: The Gateway validates the CSR, creates an application policy, and returns a one-hour certificate containing both the application and requesting-user SPIFFE SANs.
3. **Credential Storage**: The agent stores the credential under its application runtime area.
4. **Renewal Threshold**: The agent re-enrolls when the certificate has 7 days or less remaining; a one-hour credential is therefore never reused merely because it remains technically unexpired at the next enrollment check.

Delegated credentials provide identity and policy admission but do not grant privileged CLI or Operator routes, nor do they grant L2 consensus signing authority.

## Anti-patterns

- **Transmitting private keys to the Gateway**: Applications MUST generate and retain private keys locally; CSRs and public-key fingerprints are the only material submitted to enrollment endpoints.
- **Skipping trust verification on first contact**: Fetching and installing the CA bundle without fingerprint validation allows MITM substitution; always verify out-of-band on untrusted networks.
- **Reusing expired delegated credentials**: One-hour delegated credentials MUST trigger re-enrollment at the 7-day threshold, never merely because unexpired time remains.
- **Enrolling without resumable state**: Applications MUST persist enrollment requests to disk; in-memory-only state cannot survive restart and causes re-submission attempts.
- **Trusting caller-supplied identity in enrollment reviews**: Gateway ownership and policy decisions MUST derive from persistent enrollment records, not from request headers or caller identity fields.
- **Treating enrollment tokens as persistent**: Enrollment tokens expire after 30 minutes and MUST NOT be cached across sessions or assumed to survive request retries without re-fetching.
- **Bypassing validation of issued certificates**: Applications MUST validate SPIFFE SANs, key correspondence, issuer chain, and component kind before installing enrolled credentials.

## Links out

- [Network Architecture](../architecture/network.md) — PKI hierarchy, cryptographic profile, SPIFFE identity format, TLS 1.3 enforcement, and port topology (canonical reference for PKI architecture).
- [Authentication & Authorization](../architecture/auth.md) — Identity binding, route classification, CLI lifecycle, and session management.
- [Governance](../architecture/governance.md) — Five-layer transaction verification and governance posture.
- [Ensemble Architecture](../architecture/ensemble.md) — g8ee startup flow, application enrollment integration, and Gateway client usage.
- [Gateway Architecture](../architecture/gateway.md) — Gateway operational modes and policy decision points.
- [Storage](storage.md) — Application runtime volume layout and credential persistence.
- [Troubleshooting](../devs/troubleshooting.md) — PKI diagnostics, enrollment recovery, and certificate debugging.
