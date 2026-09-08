# PKI and Trust

## Overview

g8e uses a private X.509 public key infrastructure (PKI) to authenticate the Governance Gateway (`g8eg`), Governed Operators (`g8eo`), the CLI, platform workloads, and delegated applications. TLS protects network traffic and certificates bind each connection to a workload identity. Certificate authentication does not authorize host mutation by itself; governed mutations also pass the five-layer verification pipeline described in [Governance](governance.md).

The Gateway creates or loads the PKI hierarchy when it starts. It keeps certification authority (CA) and Gateway service private keys in the encrypted keystore, while public certificates and trust bundles live in the runtime directory.

## Cryptographic Profile

g8e-generated CA, service, and client certificates use ECDSA P-256 keys and ECDSA with SHA-256 signatures. Certificate signing requests must contain a P-256 public key. The Gateway rejects persisted CA certificates that use Ed25519 signatures.

TLS connections require TLS 1.3. The shared TLS key-agreement preference is X25519MLKEM768, P-384, then P-256; plain X25519 is excluded. Ed25519 remains separate from transport PKI and is used for governance signatures. WebAuthn passkeys provide human authentication and approval, as described in [Authentication and Authorization](../architecture/auth.md).

## Trust Hierarchy

The Gateway manages a root CA and three intermediate CAs:

```text
Root CA
├── Hub intermediate CA
│   └── Gateway service certificate
├── Operator intermediate CA
│   ├── Operator certificates
│   ├── CLI certificates
│   └── Application certificates
└── Gateway peer intermediate CA
    └── Gateway peer certificates
```

| Authority | Validity | Purpose |
| --- | --- | --- |
| Root CA | 3,650 days | Self-signed trust anchor for the g8e deployment |
| Hub intermediate CA | 3,650 days | Issues the Gateway service certificate |
| Operator intermediate CA | 3,650 days | Issues Operator, CLI, platform workload, and delegated application certificates; signs the certificate revocation list (CRL) |
| Gateway peer intermediate CA | 3,650 days | Issues certificates for Gateway peers |

The Gateway service certificate is valid for 90 days. Operator and CLI certificates are valid for 7 days, Gateway peer certificates are valid for 90 days, and delegated application certificates are valid for 1 hour. Owner-approved platform application certificates, including dashboard and ensemble identities, are valid for 7 days.

## Workload Identities

Issued leaf certificates carry URI Subject Alternative Names (SANs) in the `g8e.local` SPIFFE trust domain. The Gateway derives authenticated identity from these SANs rather than trusting caller-supplied identity fields.

| Identity | SPIFFE ID |
| --- | --- |
| Operator | `spiffe://g8e.local/operator/<organization_id>/<operator_id>/<operator_session_id>` |
| CLI | `spiffe://g8e.local/cli/<user_id>/<cli_session_id>` |
| Application | `spiffe://g8e.local/app/<application_id>` |
| Ensemble | `spiffe://g8e.local/app/g8ee` |
| Gateway service | `spiffe://g8e.local/hub/operator-listen` |
| Gateway peer | `spiffe://g8e.local/gateway/<gateway_id>` |

Delegated and owner-approved platform application certificates carry two URI SANs: `spiffe://g8e.local/app/<application_id>` identifies the application, and `spiffe://g8e.local/user/<user_id>` identifies the user who authorized it.

## Trust Establishment

The Gateway exposes a limited plain HTTP bootstrap surface on port `8080`. A client can retrieve the CA bundle and root fingerprint before it has a trusted certificate. Bootstrap, recovery request/status/completion, platform enrollment request/status/completion, deployment scripts, and platform binaries are also available on this surface. Other HTTP requests redirect to HTTPS.

The CA bundle and fingerprint endpoints are unauthenticated discovery mechanisms, not independent proof of the Gateway's identity. For first contact across an untrusted network, verify the root fingerprint through a trusted channel before installing the root CA. The CLI compares a complete local identity with the live root fingerprint on later enrollment runs and routes stale identities through bootstrap or recovery instead of silently reusing them.

HTTPS runs on port `8443` by default and requires TLS 1.3. Public browser routes can connect without a client certificate. Routes classified for mTLS require a verified client certificate and an active identity or session at the application layer.

The canonical Gateway trust bundle is `.g8e/pki/trust/g8eg-ca-bundle.pem`. It contains the root CA and all three intermediate CAs. The Gateway also maintains an Operator bundle containing the root and Operator intermediate, plus a standalone root mirror.

## CLI Enrollment and Recovery

Run `g8e auth enroll user` to prepare a local CLI identity. The command inspects the CLI certificate, private key, credentials record, and trust bundle as one set, then follows the matching flow:

1. On an unbootstrapped Gateway, the first enrollment creates the owner user and CLI session, receives a 7-day CLI certificate, and stores the returned trust bundle.
2. On a bootstrapped Gateway without a usable local identity, enrollment creates a recovery request. An existing user approves it in the Console, then the requester proves possession of the new private key and receives a replacement identity.
3. A complete, valid identity is reused. If the certificate is near expiry, expired, explicitly selected for rotation, or anchored to a different Gateway root, enrollment rotates it or uses recovery as appropriate.
4. After credential preparation, the normal interactive flow installs the Gateway root in the operating system trust store when needed, asks the user to restart open browsers after trust changes, and starts passkey registration.

The CLI commits newly issued credentials as a complete set, avoiding partial replacement of a certificate, key, or credentials record. It removes stale g8e root anchors only after showing them and receiving confirmation.

Use `g8e auth enroll user --headless` when no browser is available. Headless enrollment skips passkey registration and operating system trust installation. Recovery requires approval from an already enrolled CLI with `g8e auth approve-recovery <token>`, and the resulting identity supports mTLS but cannot authenticate to the Console.

Use `g8e auth refresh` when the local certificate remains valid but the server-side CLI session has expired or disappeared. Use `g8e auth enroll user --rotate-cli` to force replacement of a complete, unexpired CLI identity.

## Platform Workload Enrollment

Operators, the dashboard, and the ensemble enroll through the owner-approved platform enrollment flow after the Gateway has an owner. A workload generates its key and certificate signing request locally, submits an enrollment request with key fingerprints, and waits for a decision. The raw requester token is returned only to the workload; approval views expose request metadata and fingerprints without exposing the token, private key, or certificate.

An enrolled owner uses these commands to review and decide requests:

1. Run `g8e auth pending-platform-enrollments` to list pending requests.
2. Inspect the component, instance, and fingerprints.
3. Run `g8e auth approve-platform-enrollment <request-id>` to approve, or add `--deny` to deny it.

After approval, the workload proves possession of its private key and retrieves its certificate chain and trust bundle. Platform application certificates bind both the component identity and approving owner. Operator enrollment returns an Operator identity and a CLI identity used for authenticated renewal.

## Delegated Applications

A locally launched agent can request a delegated application certificate through an enrolled CLI identity. The Gateway binds the application SPIFFE ID and requesting user SPIFFE ID into a 1-hour certificate. The client stores delegated certificates and keys under `.g8e/apps/` and re-enrolls when an existing credential has less than 7 days remaining, which means the 1-hour delegated credential is not reused on a later enrollment check.

## Renewal

The Gateway checks its service certificate when it starts and every 24 hours. It renews a certificate with less than 30 days remaining and includes currently detected DNS names and IP addresses. Startup also replaces a still-valid service certificate when newly detected names or addresses are missing from its SANs.

A running Operator checks its client certificate at startup and every 24 hours. When less than 24 hours remain, it creates a new P-256 key and re-enrolls over mTLS with its existing CLI credential. Renewal errors leave the existing identity unchanged and are reported by the service.

CLI identities do not run a background renewal loop. Enrollment rotates a certificate near expiry, while recovery handles expired certificates that can no longer authenticate over mTLS.

## Revocation

The Gateway records revoked certificate serial numbers, reasons, and timestamps in the `revoked_certificates` collection. Authenticated request processing checks the presented certificate serial against this registry and rejects revoked identities. CLI approval and L3 verification also reject revoked CLI certificates.

The Gateway generates a DER-encoded X.509 CRL signed by the Operator intermediate CA. Each generated CRL has a 24-hour `NextUpdate` interval and is available from the public PKI path on the HTTPS surface. Revocation checking during authenticated requests uses the live registry rather than waiting for clients to refresh the CRL.

## Runtime Files

The default runtime layout contains these operator-relevant artifacts:

| Path | Contents |
| --- | --- |
| `.g8e/pki/root/root_ca.crt` | Root CA certificate |
| `.g8e/pki/authorities/` | Intermediate CA certificates |
| `.g8e/pki/issued/hub/` | Gateway service certificate and chain |
| `.g8e/pki/trust/g8eg-ca-bundle.pem` | Canonical Gateway trust bundle |
| `.g8e/cli.crt` and `.g8e/cli.key` | Local CLI certificate and private key |
| `.g8e/operator.crt` and `.g8e/operator.key` | Local Operator certificate and private key |
| `.g8e/apps/` | Delegated application certificates and private keys |

Gateway CA and service private keys do not appear as plaintext PEM files in the PKI tree. The encrypted keystore holds those keys. Local client private keys and credentials use owner-only file permissions, while public CA certificates and trust bundles are readable as public certificate material. All `.g8e/` runtime access uses the runtime file service and remains confined to the configured runtime root.

## PKI and Governance Authorization

A valid certificate establishes transport identity, but it does not bypass governance. Governed mutations pass the complete verification sequence:

1. **L1 Doctrine** applies hard gates, forbidden-pattern matching, and threat detection.
2. **L2 Consensus** verifies multi-agent consensus signatures using Ed25519.
3. **L3 Notary** verifies human authorization through WebAuthn or a signed CLI proof when required.
4. **L4 Warden** verifies signatures, expiry, nonces, replay protection, and the state Merkle root before dispatch.
5. **L5 Actuator** dispatches the approved tool call with a just-in-time capability and produces a signed receipt.

See [Governance](governance.md) for the authorization and execution model.

## Related

- [Authentication and Authorization](../architecture/auth.md) describes mTLS, WebAuthn, sessions, and route authentication.
- [Gateway Architecture](../architecture/gateway.md) describes Gateway network surfaces and services.
- [Architecture](architecture.md) describes the ensemble topology and component relationships.
- [Governance](governance.md) describes the five-layer verification pipeline.
- [Protocol](protocol.md) describes wire contracts and governance envelopes.
- [Storage](storage.md) describes runtime and document storage boundaries.
