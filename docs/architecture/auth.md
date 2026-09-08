# Authentication & Authorization

Last Updated: 2026-09-08
Version: v2.1.7

## Overview

g8e separates authentication from authorization. Authentication establishes the principal making a request through an mTLS workload certificate, a browser session created by WebAuthn, or a JWT from a configured identity provider. Authorization determines whether that principal may use a route and whether a governed transaction has the proofs required by the active governance posture.

The gateway exposes a small public surface for health checks, trust discovery, first-user bootstrap, browser passkey ceremonies, and token-scoped enrollment. Governed execution and administrative operations require an authenticated identity. Unknown HTTPS routes default to mTLS authentication, so an unclassified route does not become public.

Every governed transaction travels in a typed `GovernanceEnvelope` and passes through the five-layer interlock. L1, L4, and L5 always apply. L2 Consensus and L3 Notary are enforced or audited according to the active posture. See [Governance](./governance.md) for the complete transaction pipeline.

## Authentication Methods

| Principal | Credential | Primary use |
| --- | --- | --- |
| Human CLI | ECDSA P-256 client certificate and CLI session | CLI commands, approval status, enrollment administration, MCP, and A2A |
| Browser user | WebAuthn passkey and secure web-session cookie | Console access, passkey management, browser approvals, and enrollment decisions |
| Governed Operator | Workload certificate and operator session | Outbound gateway connection, governed dispatch, and receipt return |
| Platform application | Workload certificate | Dashboard, ensemble, and other owner-approved platform services |
| Delegated agent | Short-lived application certificate bound to the requesting user | Agent-specific MCP and A2A activity |
| External client | JWT validated through configured JWKS | MCP and A2A when external identity-provider authentication is enabled |

The gateway also accepts either mTLS or a browser session on selected shared surfaces, including event consumption and platform enrollment review. It prefers mTLS when a client certificate is present.

## CLI Authentication

### Enrollment Decisions

Run `g8e auth enroll user` to establish or repair a local CLI identity. The command evaluates the complete local credential set, checks the live gateway trust anchor when reachable, and selects one action:

| Local and gateway state | Action |
| --- | --- |
| No local identity and no gateway user | Bootstrap the first owner and issue a CLI identity |
| No local identity on an initialized gateway | Request human-approved recovery |
| Complete identity with a valid certificate, matching gateway root, and active session | Reuse the identity without issuing another certificate |
| Complete identity with a certificate that expires within 24 hours | Rotate the certificate through authenticated mTLS |
| Complete identity with an expired certificate or stale gateway root | Recover the identity, or bootstrap if the gateway is empty |
| Partial, corrupt, or mismatched local credentials | Recover the identity, or bootstrap if the gateway is empty |

Live CA discovery is best-effort so an offline identity can still be inspected and reused. When discovery succeeds, a root mismatch prevents attempted reuse of a certificate issued by a different gateway PKI. A matching root with changed intermediate certificates allows the local trust bundle to be refreshed.

Before reporting a reused identity as healthy, the CLI probes its server-side session when the gateway is reachable. An expired or missing session directs the user to `g8e auth refresh` instead of issuing a replacement certificate.

### Interactive Enrollment

The interactive flow performs these steps:

1. Start the gateway, then run `g8e auth enroll user`.
2. The CLI generates its private key and certificate signing request locally.
3. An empty gateway creates the first owner. An initialized gateway requires approval of a time-limited recovery request by an authenticated user.
4. The gateway issues a seven-day CLI certificate whose SPIFFE URI SAN binds the user and CLI session.
5. The CLI validates and writes the certificate, key, session metadata, and gateway trust bundle as one managed credential set.
6. Unless `--no-system-trust` is set, the CLI installs the gateway root CA in the operating-system trust store. When trust changes, the CLI asks the user to close all browser windows before continuing.
7. The CLI opens the Console for a WebAuthn registration ceremony. A short-lived, one-time enrollment token binds the browser ceremony to the new CLI user and session without placing raw session identifiers in the URL.

If browser launch fails during recovery, the CLI prints the approval URL for manual use. If passkey registration fails after the CLI credential set is issued, rerunning enrollment reuses the valid CLI identity and starts the passkey ceremony again.

The `--no-system-trust` flag skips only operating-system trust installation. It does not skip passkey registration and is appropriate only when an administrator already manages the gateway root CA. The CLI still uses its local trust bundle for mTLS.

### Endpoint Overrides

Enrollment uses plain HTTP for trust discovery and unauthenticated bootstrap or recovery, then HTTPS for authenticated API calls and browser interaction. `--endpoint` (`-e`) selects the discovery host and optional HTTP port. `--port` selects the HTTPS port.

For default ports, run `g8e auth enroll user`. For split Docker mappings, run `g8e auth enroll user -e localhost:<http-port> --port <https-port>`. When only `--endpoint` is supplied, both phases use that host and their configured default ports.

### Headless Enrollment

Run `g8e auth enroll user --headless` to create an mTLS-only CLI identity without opening a browser or changing the operating-system trust store. On an empty gateway, the command bootstraps the first owner directly. On an initialized gateway, it prints `g8e auth approve-recovery <token>` for an already-enrolled CLI to run and waits for that approval.

A headless identity can use CLI surfaces but has no browser session until a passkey is registered. The public Console bootstrap path permits only a user's first passkey. Additional passkeys can be registered by rerunning authenticated CLI enrollment, while listing and revocation require an authenticated browser session.

### Recovery, Rotation, and Refresh

CLI recovery requests expire after 10 minutes. Approval can come from an authenticated Console session or from an enrolled CLI using `g8e auth approve-recovery <token>`. The token is opaque and one-time-use, and completion requires proof that the recovering CLI controls the private key corresponding to its certificate request.

CLI certificates and CLI sessions both have a seven-day lifetime. Enrollment rotates a certificate automatically within 24 hours of expiry, and `g8e auth enroll user --rotate-cli` forces rotation while the existing certificate and session remain valid. Rotation issues one replacement certificate and revokes the old certificate.

Run `g8e auth refresh` when the CLI certificate is still valid but its server-side session is expired or missing. The gateway derives the user and prior session from the verified certificate, verifies that the user remains active, and creates a replacement session. Refresh requires an active operator binding; if none can be resolved, reenroll the CLI. An expired certificate cannot authenticate to refresh, so use `g8e auth enroll user` and complete recovery instead.

### Authentication Context and Logout

`g8e auth context` emits the local CLI identity, CLI and operator session binding, and certificate and key paths as typed JSON for automation. If local metadata lacks an operator binding, the command accepts exactly one active operator resolved through the gateway and fails closed when the binding is missing or ambiguous.

`g8e auth logout` removes the local CLI credentials, certificate, and private key. It does not revoke the gateway-side session or certificate, and it does not remove the shared gateway root CA from the operating-system trust store. Use gateway administration and certificate revocation when server-side invalidation is required.

On Windows, interactive enrollment also imports the signed CLI certificate into the current user's certificate store. CLI private keys remain file-backed ECDSA P-256 keys on every platform.

## Browser Authentication

The Console uses WebAuthn passkeys. Platform authenticators such as Windows Hello and Touch ID, roaming security keys, and synced passkeys can satisfy the ceremony when they meet the gateway's resident-key and user-verification requirements.

The normal browser flow is:

1. Register a passkey during CLI enrollment or through an allowed first-passkey bootstrap flow.
2. On later visits, enter the user identity and complete a WebAuthn assertion.
3. The gateway creates a 24-hour browser session and sets a secure, HTTP-only cookie.
4. Use the authenticated Console to review approvals, manage passkeys, inspect the current session, and review platform enrollment requests.
5. Log out through the Console to delete the browser session and clear the cookie.

A browser session authorizes only browser-classified routes. It does not substitute for a workload certificate on mTLS-only execution and administrative routes.

## External Identity Providers

When JWKS authentication is configured, the gateway requires JWT bearer authentication on MCP and A2A ingress instead of the normal mTLS application route. It verifies RS256 signatures using the token's key identifier, validates configured issuer and audience constraints and temporal claims, and requires a subject.

The first valid token for a subject provisions an internal user. The configured role claim maps to an internal persona, and the token's tenant claim supplies tenant context when present. A JWT-authenticated user with no passkey may register a first passkey through the JIT registration flow, then use that passkey for later Console authentication.

JWT authentication does not grant access to arbitrary administrative routes. It is scoped to the explicitly wrapped external-client surfaces.

## Agent and Application Authentication

`g8e mcp agent run` first relies on an authenticated human CLI, then enrolls the launched agent as a delegated application. The gateway issues a one-hour certificate containing both the application SPIFFE identity and the requesting user's SPIFFE identity. This lets policy and audit records distinguish the agent while retaining the human delegation chain.

Application certificates are accepted only with an active application policy. Application identities cannot use privileged routes reserved for CLI and operator principals, and application enrollment does not grant L2 consensus authority. Consensus signing authority requires separate administrative enrollment.

## Platform Workload Enrollment

Operators, the dashboard, and the ensemble use owner-approved platform enrollment. Starting a gateway with no users issues no platform workload certificate. The first owner must exist before a workload can submit an enrollment request.

The workload enrollment flow is:

1. The workload generates its private keys locally and submits its certificate requests with a system fingerprint.
2. The gateway creates a token-scoped request that expires after 30 minutes.
3. The active first user reviews the request in the Console or with `g8e auth pending-platform-enrollments`.
4. The owner approves or denies the request. CLI approval uses `g8e auth approve-platform-enrollment <request-id> --yes`.
5. After approval, the workload proves possession of every requested private key and receives its certificate, trust bundle, and session or application policy.
6. A retry after successful completion returns the same issued identity rather than minting a second one.

The operator, dashboard, and ensemble submit independent requests. Their recommended startup order is operational guidance, not an authorization dependency enforced by the gateway.

## Identity and Session Binding

mTLS certificates carry SPIFFE identities in URI SANs. The gateway validates certificate revocation, extracts the principal type, and matches the certificate identity to the referenced CLI, operator, or application session before accepting a request. Disabled users, terminated operators, expired sessions, revoked certificates, duplicate bindings, and identity mismatches fail closed.

A CLI command can carry a chain from CLI certificate to CLI session, user, operator session, and operator. Browser events bind to the user and browser session. Delegated application certificates bind an application identity to the human user who requested the credential. These bindings prevent caller-supplied identifiers from overriding the authenticated transport identity.

For PKI hierarchy, SPIFFE formats, trust bundles, revocation, and port topology, see [Network Architecture](./network.md).

## Authorization and the Five-Layer Interlock

Authentication admits a principal to a route. It does not by itself authorize a governed action. Every governed transaction passes through these layers:

| Layer | Authorization role |
| --- | --- |
| **L1 Doctrine** | Enforces hard gates, forbidden-pattern matching, and MITRE threat detection |
| **L2 Consensus** | Verifies a quorum of distinct Ed25519 consensus signatures over the transaction decision |
| **L3 Notary** | Verifies human authorization through WebAuthn or an approved signed CLI proof when the posture requires it |
| **L4 Warden** | Rechecks signatures, transaction hash, replay protection, expiry, nonce, state Merkle root, and posture-required proofs before dispatch |
| **L5 Actuator** | Mints a transaction-scoped capability, dispatches the isolated MCP or A2A action, and produces signed execution receipts |

Universal checks fail closed in every posture. Optional L2 and L3 evidence is verified and recorded when present but does not block execution unless the posture requires it.

### Governance Postures

| Posture | L1 Doctrine | L2 Consensus | L3 Notary |
| --- | --- | --- | --- |
| **Doctrine** | Enforced | Audited | Audited |
| **Consensus** | Enforced | Enforced | Audited |
| **Ratify** | Enforced | Audited | Enforced for mutations |
| **Notary** | Enforced | Enforced | Enforced for mutations |

Read-only actions do not require L3 proof in any posture. Under `ratify` and `notary`, mutations fail closed without valid human authorization. See [Governance](./governance.md) for posture selection and the complete L1 through L5 behavior.

### Human Approval

In gateway mode, L3 uses a WebAuthn assertion over the transaction hash. CLI-originated proofs also bind to the authenticated CLI certificate and session. In outbound operator mode, L3 verifies an approved suspended transaction and an Ed25519 signature over the transaction hash.

A suspended approval request remains available for two minutes. After approval, the proof remains dispatchable for 30 minutes. Expired requests and proofs require a new transaction and approval. There is no mock or automatic approval bypass.

## Security Properties

- **Explicit route authentication:** Public, browser-session, mTLS, dual-auth, and JWT surfaces are classified separately. Unknown HTTPS routes require mTLS.
- **Narrow bootstrap authority:** First-user bootstrap works only while the gateway has no users. Recovery and workload enrollment use short-lived tokens and private-key proof-of-possession.
- **Certificate-bound identity:** SPIFFE URI SANs bind user, session, operator, and application identities to authenticated certificates.
- **Revocation and lifecycle checks:** The gateway checks certificate revocation, user state, session state, and principal-specific policy on authenticated requests.
- **Local key generation:** CLI, workload, and delegated application private keys are generated by the requesting component and are not sent to the gateway.
- **Fail-closed governance:** Universal checks and posture-required proofs reject the transaction before execution. L5 records signed receipts around the execution boundary.
- **Separation of trust:** A browser session cannot access mTLS-only routes, an application identity cannot assume CLI or operator privileges, and transport authentication cannot bypass governance authorization.

Encryption at rest, vault operation, scrubbing, and rehydration are separate from principal authentication. See [Encryption](./encryption.md) for those controls and [FIPS 140-3 Compliance](../reference/fips140-3.md) for cryptographic compliance details.

## Related Documentation

- [Governance](./governance.md): Five-layer verification, posture behavior, and transaction flow.
- [Network Architecture](./network.md): PKI hierarchy, SPIFFE workload identities, mTLS, revocation, and ports.
- [Gateway Architecture](./gateway.md): Gateway admission, routing, and policy-decision responsibilities.
- [Operator Architecture](./operator.md): Operator session, L4 verification, and L5 execution boundary.
- [AI Agents](./agents.md): Delegated agent identity and governed MCP and A2A flows.
- [SSE Streaming](./sse.md): Browser, CLI, and application event routing.
- [Encryption](./encryption.md): Vault, encryption at rest, scrubbing, and execution-site rehydration.
