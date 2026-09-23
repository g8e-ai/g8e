# PKI and Trust

## Scope

g8ee is an optional platform application. It does not own the platform PKI and it is outside the Gateway and Operator execution boundaries. The Gateway owns the deployment PKI and enrollment records. g8ee owns its enrolled application certificate, private key, trust bundle, and resumable enrollment state in its own runtime volume. The remote Operator owns the credentials and execution evidence for the runtime where it runs. In the unified Compose stack, the read-only `/operator-state` mount supplies selected bootstrap material; it is not a general host filesystem or execution channel.

This document summarizes the PKI behavior that g8ee depends on. [Authentication and Authorization](../architecture/auth.md) and [Network Architecture](../architecture/network.md) are the canonical references for route classification, identity binding, and transport topology.

## Cryptographic profile

The Gateway generates a private X.509 hierarchy with ECDSA P-256 keys and ECDSA with SHA-256 signatures. The Gateway accepts only P-256 public keys in certificate signing requests. Persisted CA certificates using Ed25519 signatures are rejected when the PKI hierarchy is loaded. Ed25519 is used separately for protocol governance signatures; it is not the transport certificate algorithm.

Gateway TLS configurations require TLS 1.3 and use the shared curve preference order `X25519MLKEM768`, P-384, and P-256. Plain X25519 is excluded. The Gateway serving listener requests and verifies client certificates at the TLS layer when a client certificate is presented, while route middleware applies the actual public, browser-session, dual-authentication, or mTLS requirement. Browser routes can therefore connect without a client certificate; an mTLS route still fails closed without a valid certificate.

A valid certificate authenticates a transport principal but does not authorize a governed mutation. Governance authorization remains subject to the active five-layer posture and the Gateway or executing Operator checks described in [Governance](governance.md).

## PKI hierarchy and validity

The Gateway creates or loads one root CA and three intermediate CAs during startup:

```text
Root CA
├── Hub intermediate CA
│   └── Gateway serving certificate
├── Operator intermediate CA
│   ├── Operator certificates
│   ├── CLI certificates
│   ├── platform application certificates
│   └── delegated application certificates
└── Gateway peer intermediate CA
    └── Gateway peer certificates
```

| Authority or certificate | Validity | Purpose |
| --- | --- | --- |
| Root CA | 3,650 days | Self-signed trust anchor for the deployment |
| Hub intermediate CA | 3,650 days | Signs the Gateway serving certificate only |
| Operator intermediate CA | 3,650 days | Signs Operator, CLI, platform application, and delegated application certificates; signs the CRL |
| Gateway peer intermediate CA | 3,650 days | Signs Gateway peer certificates |
| Gateway serving certificate | 90 days | Terminates Gateway TLS and carries the Gateway service identity |
| Operator and CLI certificates | 7 days | Authenticate enrolled Operator and CLI sessions |
| Platform application certificate | 7 days | Authenticates an owner-approved dashboard, ensemble, or other platform application |
| Gateway peer certificate | 90 days | Authenticates Gateway peer identities |
| Delegated application certificate | 1 hour | Authenticates an application credential delegated by a user |

The serving certificate includes `localhost`, `g8e.local`, the literal `operator` service name, and detected or explicitly configured DNS names and IP addresses. Startup regenerates it when required SANs are missing or the certificate has less than 30 days remaining. A Gateway renewal loop checks it every 24 hours.

## SPIFFE identities

Leaf certificates carry URI SANs in the `g8e.local` trust domain. The Gateway extracts the authenticated principal from the certificate instead of trusting caller-supplied identity fields.

| Principal | SPIFFE ID |
| --- | --- |
| Governed Operator | `spiffe://g8e.local/operator/<organization_id>/<operator_id>/<operator_session_id>` |
| CLI | `spiffe://g8e.local/cli/<user_id>/<cli_session_id>` |
| Application | `spiffe://g8e.local/app/<application_id>` |
| Human delegator SAN on delegated or platform app certificates | `spiffe://g8e.local/user/<user_id>` |
| Ensemble | `spiffe://g8e.local/app/g8ee` |
| Gateway serving identity | `spiffe://g8e.local/hub/operator-listen` |
| Gateway peer | `spiffe://g8e.local/gateway/<gateway_id>` |

Delegated application certificates contain the application and requesting-user SANs and are issued for one hour. Owner-approved platform application certificates, including the g8ee certificate, also contain the application and approving-user SANs but use the seven-day platform certificate validity.

## Trust discovery and enrollment surfaces

The Gateway publishes the complete trust bundle at `/.well-known/g8e/pki/ca-bundle` and the root fingerprint at `/.well-known/g8e/pki/fingerprint`. These are discovery mechanisms, not independent proof of Gateway identity. For first contact across an untrusted network, verify the root fingerprint through a trusted channel before trusting the root CA.

The plain HTTP listener on port `8080` is limited to health, trust discovery, first-user bootstrap, token-scoped CLI recovery, token-scoped platform enrollment, deployment scripts, and platform binaries. Other HTTP requests redirect to HTTPS. The HTTPS listener uses port `8443` by default. Authenticated API and browser interaction use HTTPS; the CLI can use the plain discovery surface only for the unauthenticated steps explicitly supported by enrollment and recovery.

The canonical Gateway bundle is `.g8e/pki/trust/g8eg-ca-bundle.pem`. It contains the root, hub, Operator, and Gateway peer intermediate certificates. The Gateway also writes `.g8e/pki/trust/operator-bundle.pem` with the root and Operator intermediate, `.g8e/pki/trust/root.pem` with the root certificate alone, and `.g8e/pki/trust/trust-domain.json` as private trust-domain metadata. The Gateway serves a DER-encoded X.509 CRL at `/.well-known/g8e/pki/crl`; the CRL is signed by the Operator intermediate and has a 24-hour `NextUpdate` interval.

## CLI identity lifecycle

Run `g8e auth enroll user` to create or repair the local CLI identity. The CLI evaluates the certificate, private key, credentials record, and trust bundle as one managed set:

1. On an empty Gateway, the first enrollment creates the owner and issues a seven-day CLI certificate.
2. On an initialized Gateway without a usable local identity, enrollment creates a recovery request. An existing user approves it in the Console, or an enrolled CLI approves it with `g8e auth approve-recovery <token>`.
3. A complete identity with a matching root and valid certificate is reused. A certificate expiring within 24 hours or an explicit `--rotate-cli` request uses authenticated rotation. An expired certificate, stale root, partial identity, or corrupt credential set uses recovery instead of silently mixing files.
4. Interactive enrollment installs the Gateway root in the operating-system trust store unless `--no-system-trust` is set, asks the user to close open browser windows after a trust change, and starts the passkey registration ceremony.

Headless enrollment skips passkey registration and operating-system trust installation. It creates an mTLS-only CLI identity; that identity can use CLI surfaces but cannot authenticate to the Console until a passkey is registered. `g8e auth refresh` renews a missing or expired server-side CLI session when the certificate is still valid; it does not issue a new certificate. CLI certificates and CLI sessions each last seven days.

The CLI stages and commits the certificate, key, trust bundle, and credentials as a complete set. Local client private keys and credentials are owner-only files. Logout removes local CLI credentials, certificate, and key but does not revoke the Gateway-side certificate or session and does not remove the shared root from the operating-system trust store.

## Owner-approved platform enrollment

The dashboard, ensemble, and remote Operator use the owner-approved platform enrollment protocol after the Gateway has an owner. Each workload generates its private key and P-256 CSR locally, submits the CSR and public-key fingerprint over the plain discovery surface, and receives an opaque one-time requester token. A request expires after 30 minutes. The token is returned only to the workload; review data contains request metadata and fingerprints, not the token, private key, CSR, or certificate.

An enrolled owner reviews requests with `g8e auth enroll pending`, then approves or denies them with `g8e auth enroll approve <request-id>` or `g8e auth enroll deny <request-id>`. After approval, the workload proves possession of every requested private key and completes the request. The Gateway returns the issued certificate chain, trust bundle, and the corresponding session or application policy. A remote Operator request issues both an Operator identity and a companion CLI identity used for authenticated maintenance and renewal. Dashboard and ensemble requests are independent of Operator enrollment; startup order is operational guidance, not an authorization dependency enforced by the Gateway.

The platform enrollment clients persist resumable pending state under `.g8e/pki/pending-enrollment/` with owner-only permissions. The Operator client uses `g8eo.json`; the dashboard and ensemble use their component-specific pending files. On successful completion, each client atomically installs its certificate, key, and trust bundle in its own runtime tree. g8ee does not become ready while its enrollment is pending.

The active first owner can revoke a completed platform enrollment request with `g8e auth enroll revoke <request-id>`. Revocation invalidates the issued identity and related session or application policy and disconnects active authenticated pub/sub channels for the revoked identity. Operator revocation also invalidates the companion CLI identity and marks the Operator terminated. This lifecycle does not revoke human CLI identities, delegated application credentials, consensus signers, or Gateway peer identities.

## g8ee application credentials

g8ee loads an existing platform application identity or starts platform enrollment during its FastAPI startup. It generates a P-256 key and CSR locally, verifies the issued chain, SANs, public key, and component kind, and atomically installs the certificate, key, and trust bundle in the ensemble runtime volume. The application certificate is renewed when it has one day or less remaining. g8ee uses that certificate for Gateway-backed database, key-value, blob, HTTPS, and pub/sub clients; it does not use `/operator-state` as a general credential or execution channel.

The g8ee identity is an application identity, not an L2 consensus signer. Application policy and transport identity permit the application to use the routes and channels assigned to it, but they do not authorize host execution or replace protocol L2 or L3 evidence. Host-command requests use the exact selected remote Operator command channel, where the Gateway constructs a `GovernanceEnvelope` and the Operator independently verifies it before execution. Protected g8ee application-record writes use g8ee's dedicated Gateway client and remain subject to the active posture. See [Ensemble Architecture](../architecture/ensemble.md) for the complete application flow.

## Delegated applications

A locally launched agent can request a delegated certificate through an enrolled CLI identity at the authenticated `/api/v1/pki/apps/delegated` route. The Gateway validates the CSR, requires a P-256 key, creates an application policy, and returns a one-hour certificate containing both the application and requesting-user SPIFFE SANs. The client stores delegated credentials under its application runtime area and re-enrolls when the existing certificate has seven days or less remaining; a one-hour credential therefore is never reused merely because it is still technically unexpired at the next enrollment check.

Delegated application enrollment provides identity and policy admission only. It does not grant privileged CLI or Operator routes and does not grant L2 consensus authority. Consensus signing authority requires separate administrative enrollment.

## Renewal and revocation

The Gateway checks its serving certificate at startup and every 24 hours, renewing it within 30 days of expiry or when detected SANs have drifted. A running remote Operator checks its client certificate at startup and every 24 hours and re-enrolls over mTLS when less than 24 hours remain. Renewal generates a new P-256 key for the Operator identity. g8ee renews its platform application credential at the one-day threshold during application startup and through its certificate service lifecycle. CLI identities have no background renewal loop.

The Gateway records certificate serial numbers, reasons, and timestamps in its revoked-certificate store. Authenticated request processing checks the live store on new requests and WebSocket handshakes, so clients do not need to wait for the CRL to refresh. The CRL is an external distribution artifact and contains all serials present in the store; it does not replace live Gateway revocation checks.

## Runtime artifacts and key ownership

The Gateway's default runtime tree contains the following public or operator-relevant artifacts:

| Path | Contents |
| --- | --- |
| `.g8e/pki/root/root_ca.crt` | Root CA certificate |
| `.g8e/pki/authorities/hub_ca.crt` | Hub intermediate certificate |
| `.g8e/pki/authorities/operator_ca.crt` | Operator intermediate certificate |
| `.g8e/pki/authorities/gateway_peer_ca.crt` | Gateway peer intermediate certificate |
| `.g8e/pki/issued/hub/operator-gateway.crt` | Gateway serving certificate |
| `.g8e/pki/issued/hub/operator-gateway.chain.pem` | Serving certificate chain |
| `.g8e/pki/trust/g8eg-ca-bundle.pem` | Complete Gateway trust bundle |
| `.g8e/pki/trust/operator-bundle.pem` | Root and Operator intermediate bundle |
| `.g8e/pki/trust/root.pem` | Root certificate mirror |
| `.g8e/pki/pending-enrollment/` | Resumable platform enrollment state |
| `.g8e/pki/operator.crt` and `.g8e/pki/operator.key` | Remote Operator certificate and private key |
| `.g8e/pki/client/` | Client certificate and key fallback paths used by Operator tooling |
| `.g8e/pki/cli.crt`, `.g8e/pki/cli.key`, and the credentials record | Local CLI identity artifacts |
| `.g8e/apps/` | Delegated application certificates and private keys when the application runtime uses that layout |

CA and Gateway serving private keys are stored through the encrypted secret manager rather than as plaintext key files in the Gateway PKI tree. Workload private keys, CLI credentials, pending enrollment state, and application credentials use restrictive permissions. Public CA certificates, chains, and trust bundles are readable certificate material. Runtime file access is confined to the configured `.g8e/` root through the runtime file service.

## Related documentation

- [Authentication and Authorization](../architecture/auth.md) — Route authentication, CLI recovery and rotation, platform enrollment, sessions, and revocation.
- [Network Architecture](../architecture/network.md) — TLS surfaces, PKI hierarchy, trust discovery routes, and outbound Operator transport.
- [Gateway Architecture](../architecture/gateway.md) — Gateway PKI ownership and network services.
- [Ensemble Architecture](../architecture/ensemble.md) — g8ee startup, application enrollment, runtime volume, and Gateway clients.
- [Architecture](architecture.md) — Ensemble topology and component relationships.
- [Governance](governance.md) — Application boundaries and the five-layer governance pipeline.
- [Storage](storage.md) — g8ee storage ownership and runtime persistence.
- [Documentation Guide](../devs/docs.md) — Documentation audit and ownership rules.
