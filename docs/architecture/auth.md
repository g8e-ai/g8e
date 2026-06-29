# Authentication & Authorization

Last Updated: 2026-06-28
Version: v1.3.2

This document details the authentication and authorization architecture of the g8e platform. The platform is built as a zero-trust execution environment where every mutation is typed, signed, and governed via a deterministic verification pipeline.

## Overview

The platform security model is founded on two core pillars:
1. **Identity-Bound Communication (mTLS)**: Every connection within the platform, whether from a CLI, a Dashboard, or an AI Agent, must be authenticated via mutual TLS (mTLS) with a verified SPIFFE workload identity.
2. **5-Layer Verification Sequence**: Every mutation (command execution, file edit, tool call) must pass through the sequential 5-layer verification pipeline before execution.

---

## 1. Authentication & Workload Identity

The platform uses an internal Public Key Infrastructure (PKI) to issue and manage certificates. The **g8e Gateway** acts as the Certificate Authority (CA) and enforces identity validation.

### Workload Identity (SPIFFE)

Each system component receives a SPIFFE ID, embedded as a Uniform Resource Identifier (URI) in the Subject Alternative Name (SAN) of its mTLS certificate. See [Network Architecture](./network.md) for the complete SPIFFE ID format reference and implementation details.

### mTLS Enforcement

The g8e Gateway enforces TLS 1.3 for all L7 communication with strict mTLS requirements. See [Network Architecture](./network.md) for detailed mTLS enforcement policies, revocation mechanisms, and identity binding procedures.

### PKI Hierarchy & Trust Domain

The platform uses a four-tier PKI hierarchy issued by the g8e Gateway. See [Network Architecture](./network.md) for the complete PKI hierarchy, intermediate CA split rationale, curve policy, and revocation details.

### Enrollment & Bootstrap

**The mental model:** CSR-based enrollment is cryptographic identity proof. Instead of sharing a secret (like an API key), a client generates its own key pair and asks the Gateway to sign a certificate attesting "this public key belongs to this identity." The Gateway acts as a Certificate Authority (CA). The act of starting the Gateway is itself the Platform Owner's authorization: there are no standing invite codes, pre-shared keys, or manual approval steps. The Platform Owner's intent to give AI governed access to the physical world is expressed by running the Gateway; the Gateway's willingness to sign CSRs flows from that decision. The client then proves its identity on every subsequent call by signing with its private key (via mTLS). No shared secrets, no API keys to leak.

#### CLI Enrollment Paths

There are three distinct CLI enrollment paths depending on gateway and credential state. The `EnrollCLI` function (`internal/cli/auth/agent_enroll.go:35`) encapsulates the first two paths as a reusable, idempotent call used by both `g8e auth enroll` and the agent launcher.

| Path | Trigger | Transport | Function |
| :--- | :--- | :--- | :--- |
| **First-time bootstrap** | Gateway never bootstrapped | Plain HTTP (discovery port) | `BootstrapWithURL()` |
| **New CLI, existing gateway** | Gateway bootstrapped, no local credentials | Plain HTTP (discovery port) | `CLIEnroll()` |
| **Re-enrollment** | Credentials present, certificate rotation | mTLS (HTTPS port) | `ReEnroll()` or `CLIEnroll()` |

Plain HTTP is used only for the bootstrap and CLI enrollment paths because the CLI has no mTLS certificate yet. These endpoints exist on the unauthenticated discovery port and are gated by the gateway's own authorization policy. All subsequent communication uses mTLS exclusively.

**Trusting the Self-Signed CA**: Since the g8e Gateway uses self-signed certificates for its internal PKI, non-Windows clients must trust the platform's Root CA before browser-based passkey registration can succeed. The platform provides automated trust scripts for this purpose:
- **Linux**: `curl -fsSL http://<gateway-ip>:8080/bootstrap-ca | sh`
- **macOS**: `curl -fsSL http://<gateway-ip>:8080/bootstrap-ca-macos | sh`
- **Windows**: `irm http://<gateway-ip>:8080/bootstrap-ca.ps1 | iex`

**CRITICAL**: After running a trust script, the user **MUST restart all open browsers** for the newly installed CA to be recognized. Failure to do so will result in WebAuthn registration errors in the browser.

**`EnrollCLI(cfg)`** selects between `Bootstrap` and `CLIEnroll` based on `CheckBootstrapStatus`, then saves the signed certificate, trust bundle, and credential file. Callers (`g8e auth enroll`, `g8e mcp agent run`) add their own user-facing output; `EnrollCLI` itself is silent.

**Re-enrollment** (`g8e auth enroll` when credentials already exist) uses `ReEnroll` over mTLS when an operator certificate is present, or falls back to `CLIEnroll` for CLI-only deployments. It also runs `AutoRenewCertificate` to short-circuit if the existing certificate is still valid.

#### Agent App Enrollment (Delegated Credentials)

When `g8e mcp agent run` launches an AI agent, it calls `EnrollAgentApp` (`internal/cli/auth/agent_enroll.go:82`) to issue the agent a short-lived delegated credential (1-hour certificate). This certificate carries:
- A SPIFFE URI SAN identifying the agent: `spiffe://g8e.local/app/<agent-name>`
- The requestor's human identity (bound at issuance time on the gateway)

The request is made over mTLS using the CLI certificate and requires a valid `X-G8E-CLI-Session-ID` header. The gateway's `/api/v1/pki/apps/delegated` endpoint is on the mTLS-only router. The resulting credential is injected into the agent subprocess via `G8E_APP_CERT` / `G8E_APP_KEY` environment variables and is idempotent: an existing certificate with more than 7 days remaining and the correct SPIFFE URI SAN is reused without contacting the gateway.

The agent launcher (`g8e mcp agent run` in `internal/cli/cmd/mcp.go`) calls `VerifyPasskeyRegistration` (mTLS) after `EnrollAgentApp` to verify that the authenticated user has a registered passkey, enforcing that agent sessions are always backed by a human with hardware-bound authentication.

#### Windows Enrollment

On Windows, enrollment uses the Windows Certificate Store with TPM-backed keys via Windows Hello for Business. `g8e auth enroll` detects the platform and delegates to the Windows-specific path automatically. See [Network Architecture](./network.md) for details.

### External IdP Support (JWT)

The platform supports authentication via external Identity Providers (IdPs) for BYO clients on MCP and A2A endpoints.
- **JWKS Integration**: The gateway validates JWT tokens against configured JWKS endpoints.
- **JIT Provisioning**: Users are provisioned Just-In-Time (JIT) based on the JWT subject claim upon their first successful authentication, subject to platform owner authorization.
- **Persona Mapping**: JWT roles are mapped to internal binding personas via the `PersonaService` defined in `internal/services/gateway/user_service.go:386`.

### 1.5 Browser-Facing Console & L3 Passkey Brokerage

The **g8e Console** (served exclusively over HTTPS at `/console`) is a zero-dependency, single-page application (SPA) that acts as the primary WebAuthn interface for L3 passkey brokerage. It consolidates all browser-facing interactions:
- **Unified Interface**: Replaces legacy inline HTML pages across various routes with a single elegant dark-themed UI.
- **L3 Passkey Authentication**: Provides browser-based WebAuthn registration and authentication flows, allowing users to obtain web session cookies.
- **Interactive Approval**: Automatically intercepts OOB approval redirects from `/api/v1/approve/{txHash}` to `/console/#approve={url.QueryEscape(txHash)}` and handles cryptographic challenge-response signature generation directly from the browser.
- **Passkey Management**: Under `WebSessionAuth` protection, authenticated users can view their active passkeys and revoke credentials.

#### L7 mTLS Enforcement Model
To support browser-based clients that cannot hold mTLS client certificates, the HTTPS server's TLS configuration is set to `tls.VerifyClientCertIfGiven` (rather than strict `RequireAndVerifyClientCert`). Security is rigorously enforced at the application layer:
- **Centralized Public Registry**: The `PublicRouteRegistry` explicitly allowlists routes that can bypass mTLS (e.g., `/console/`, landing/redirect page, and browser-facing passkey registration/authentication endpoints under the `console/*` prefix).
- **Registered Browser Passkey Endpoint Prefix**:
  - `/api/v1/auth/passkeys/console/` - Console SPA passkey registration and authentication (public, CORS-wrapped)
- **Fail-Closed Default**: The `auth.Middleware()` acts as a strict, fail-closed gate. Any request to a non-public route that does not carry a verified mTLS certificate is immediately rejected at Layer 7.

#### WebSessionAuth Subtree Routing
Web-session authenticated routes (e.g., user profile, approvals, passkey list/revocation) are mounted on the main `mux` via `WebSessionAuth` using standard Go `http.ServeMux` subtree patterns:
- `/api/v1/users/` (matching `/api/v1/users/me`)
- `/api/v1/auth/sessions/` (matching `/api/v1/auth/sessions/me`)
- `/api/v1/approvals` & `/api/v1/approvals/` (matching pending list and action sub-paths)
- `/api/v1/auth/passkeys` & `/api/v1/auth/passkeys/` (matching listing and individual key revocation)

This subtree-match pattern (defined with trailing slashes) ensures all nested endpoints are seamlessly guarded by the `WebSessionAuth` middleware, while the outer public/mTLS routes (like exact-match `/api/v1/users` or public browser passkey handlers) continue to resolve correctly based on Go's longest-prefix routing rules.

#### Excluded Prefixes for mTLS-Protected Sub-Paths
The `/api/v1/auth/passkeys` prefix is shared between WebSessionAuth management routes (list, revoke by credential ID) and mTLS-only routes (CLI status). To prevent the broad prefix from accidentally exposing mTLS-only sub-paths as public, `PublicRouteRegistry` supports **excluded prefixes**:
- `/api/v1/auth/passkeys/cli/` - mTLS-only CLI status endpoint (`cli/status`)
- `/api/v1/approvals/status/` - mTLS-only CLI approval status endpoint (`/api/v1/approvals/status/{txHash}`)
- `/api/v1/auth/passkeys/jit-` - JIT passkey bootstrap (excluded only when JWKS is not configured)

The `IsPublic` method checks exact paths first (highest priority), then excluded prefixes (returns false), then regular prefixes (returns true). This ensures mTLS-only routes remain protected even when they share a prefix with WebSessionAuth routes.

#### Approval Page Redirect
The OOB approval redirect (`/api/v1/approve/{txHash}`) sends a 302 to `/console/#approve={url.QueryEscape(txHash)}` (with trailing slash) to avoid an extra 301 auto-redirect hop from Go's `http.ServeMux`. The console SPA detects the `#approve=` hash fragment on load and after login, automatically triggering the approval flow.

#### Passkey Service HTTP Architecture

The passkey HTTP layer is split into two components: `PasskeyService` (domain logic) and `PasskeyHandler` (HTTP layer). `PasskeyService` (`internal/services/gateway/passkey_service.go:77`) holds domain-only fields (`userStore`, `sessionStore`, `webauthn`, `logger`, `rpID`, `rpName`) and retains `VerifyL3Proof` for L3 binding to transaction hashes. `PasskeyHandler` (`internal/services/gateway/passkey_service.go:283`) embeds `*PasskeyService` and adds HTTP concerns (`webSessionSvc`, `responder`, `maxPayload`, `mcpSvc`, `suspendedStore`). All former `AuthController` passkey handlers are consolidated into 4 factory methods and 3 direct handler methods on `PasskeyHandler`, eliminating copy-pasted handler code and fragile URL-sniffing control flow.

**Factory Methods** (return `http.HandlerFunc`):
- `RegisterChallenge(cfg)` - WebAuthn registration challenge
- `RegisterVerify(cfg)` - WebAuthn registration verification
- `AuthenticateChallenge(cfg)` - WebAuthn authentication challenge
- `AuthenticateVerify(cfg)` - WebAuthn authentication verification

**Direct Handlers**:
- `ListCredentials` - `GET /api/v1/auth/passkeys` (WebSession-protected)
- `RevokeCredential` - `DELETE /api/v1/auth/passkeys/{id}` (WebSession-protected)
- `CLIStatus` - `GET /api/v1/auth/passkeys/cli/status` (mTLS-protected)

**Approval Handlers** (on `PasskeyHandler`, via `passkey_service_approvals.go`):
- `handleApprovalAction` - dispatcher for `/api/v1/approvals/{txHash}/{action}`
- `handleApprovalChallenge` - generates WebAuthn challenge for OOB approval
- `handleApprovalVerify` - verifies WebAuthn assertion for OOB approval
- `handleCLIApprovalStatus` - `GET /api/v1/approvals/status/{txHash}` (mTLS-protected)
- `handleApprovalPage` - redirects to console SPA for browser-based approval
- `handleListSuspendedTransactions` - lists pending approvals (WebSession-protected)

Dependencies for approval handlers are injected via `SetApprovalDependencies(mcpSvc, suspendedStore)` on `PasskeyHandler` after construction, since the MCP gateway is created later in the startup sequence.

**`passkeyHandlerConfig` Type**: Each factory method accepts a typed config struct that encodes the trust posture at route mount time:

| Field | Purpose |
| :--- | :--- |
| `source` | Request source enum (`sourceJWT`, `sourceBrowserBootstrap`) |
| `enforceFirstCredentialOnly` | Bootstrap protection: rejects registration if user already has credentials |
| `requireAuthenticatedUser` | Requires `ContextKeyUserID` to be present |
| `enforceSessionUserBinding` | Prevents `user_id` spoofing across an existing session |
| `createWebSession` | Mints a `g8e_session` on successful verification |
| `setCookie` | Sets the `g8e_session` browser cookie (implies `createWebSession`) |
| `createUserOnBootstrap` | Auto-creates a user record during first-time browser enrollment; gated by `userStore.HasAnyUsers()` check, only fires when no users exist, preventing unauthorized user creation (`passkey_service_http.go:120-151`) |

This design ensures the **server decides auth posture, not the client**. The route a request lands on determines whether a session cookie is minted, whether mTLS is required, and whether the first-credential check is enforced. The request body never toggles these. `SetApprovalDependencies(mcpSvc, suspendedStore)` is called on `PasskeyHandler` after both the handler and MCP GatewayService are constructed, since the MCP gateway is created later in the startup sequence.

#### CLI Status Endpoint (mTLS-Gated)

The dedicated `GET /api/v1/auth/passkeys/cli/status` endpoint allows the CLI to authoritatively answer "does this user have a passkey?" using its mTLS certificate. Identity comes exclusively from `ContextKeyUserID`; `user_id` query params are ignored. This fixes a bug where the CLI was silently pushed into re-enrollment because it couldn't reach the WebSession-protected passkey listing endpoint.

#### Passkey Route Table

| Route | Auth | Handler Config |
| :--- | :--- | :--- |
| `POST /api/v1/auth/passkeys/jit-register/challenge` | JWT | jwt / firstCred / requireAuth / sessionBinding |
| `POST /api/v1/auth/passkeys/jit-register/verify` | JWT | jwt / firstCred / requireAuth / sessionBinding |
| `POST /api/v1/auth/passkeys/console/register/challenge` | public | browserBootstrap / firstCred / createUser / cookie |
| `POST /api/v1/auth/passkeys/console/register/verify` | public | browserBootstrap / firstCred / cookie |
| `POST /api/v1/auth/passkeys/console/authenticate/challenge` | public | browserBootstrap |
| `POST /api/v1/auth/passkeys/console/authenticate/verify` | public | browserBootstrap / cookie |
| `GET /api/v1/auth/passkeys` | WebSession | list |
| `DELETE /api/v1/auth/passkeys/{id}` | WebSession | revoke |
| `GET /api/v1/auth/passkeys/cli/status` | mTLS | CLI status |

The browser-based console flow (`/api/v1/auth/passkeys/console/*`) is the sole passkey registration and authentication path. The former mTLS register/authenticate routes, localhost bootstrap server, CLI bootstrap routes, and deprecated alias routes have been removed.

#### Credential Validation & Encoding Helpers

`PasskeyCredential.Validate()` (`internal/models/auth.go:263`) performs on-disk schema validation before persistence:
- `ID` is non-empty and ≤ 1024 bytes (WebAuthn spec limit)
- `PublicKey` is non-empty and parses as a valid CBOR-encoded COSE key (via `fxamacker/cbor/v2`)
- `AttestationType` is one of `"none"`, `"indirect"`, `"direct"`, `"enterprise"`
- `CreatedAtUnixMs` is non-zero

Called in `addCredential` at `passkey_service.go:666` before writing to disk. Typed error constants: `ErrPasskeyCredentialInvalidID`, `ErrPasskeyCredentialIDTooLong`, `ErrPasskeyCredentialInvalidPublicKey`, `ErrPasskeyCredentialInvalidAttestation`, `ErrPasskeyCredentialInvalidTimestamp`.

`encodeCredID([]byte) string` and `decodeCredID(string) ([]byte, error)` helpers (`passkey_service.go:92-100`) provide centralized base64 RawURL encoding/decoding for credential IDs, replacing scattered ad-hoc `base64.RawURLEncoding` calls.

Safe byte comparisons use `bytes.Equal` instead of unsafe `string()` casts for credential ID matching (`passkey_service.go:509`).

#### Challenge Lifecycle (Purge After Verify)

The `sessionStore` interface includes `DeleteSession(userID string) error`, implemented in `dbSessionStore` via `DocDelete`. Both `VerifyRegistration` and `VerifyAuthentication` proactively purge the stored challenge after successful verification, with a warning log on deletion failure. This prevents challenge replay attacks by ensuring a consumed challenge cannot be reused.

#### Governance Envelope Authentication

Governance envelope submission (`POST /api/v1/governance/envelopes`) and query endpoints (`/_query/*`) require operator or CLI mTLS certificates. App certificates (issued via `/api/v1/pki/apps/enroll`) are explicitly blocked from these endpoints by `handleAppAuth` in `gateway_auth.go`, which checks the `PrivilegedRouteRegistry` (`NewPrivilegedRouteRegistry` in `gateway_auth.go:184`). There is no API-key or Bearer token path for governance envelopes. Only operator certs (with a valid operator session in the database) and CLI certs (with `X-G8E-CLI-Session-ID` header) can submit envelopes.

#### Operator Device Enrollment (Headless)

The `/api/v1/auth/device/enroll` endpoint is the correct headless enrollment path for operator containers. Unlike `/api/v1/pki/csr/sign` (which only signs a CSR without persisting a session), device enrollment creates an operator document, persists an operator session in the database, signs the CSR with the session ID embedded in the SPIFFE URI SAN, and writes `operator.crt`, `operator.key`, and `g8eg-ca-bundle.pem` to `/root/.g8e/pki/`. The operator's `operator run -e <endpoint>` command calls this endpoint internally.

#### Operator Cert Sharing for Agent Containers

In demo and edge deployments, agent containers that need to submit real governance envelopes can share the operator's enrolled credentials via a read-only volume mount (`operator_state:/root/.g8e:ro`). This avoids the need for a separate enrollment init container. The agent uses the operator's cert/key/CA bundle directly, and the gateway validates the operator session from the certificate's SPIFFE URI SAN.

#### Offline Session Discovery (Cert SAN Parsing)

In disconnected environments where the gateway's `/api/v1/operators` endpoint is unreachable, the `DiscoverOperator` method in `internal/tools/agent_harness/client/audit.go` parses the operator's PEM certificate locally, extracting the `operator_id` and `operator_session_id` from the SPIFFE URI SAN (`spiffe://g8e.local/operator/<user_id>/<operator_id>/<operator_session_id>`). This enables headless session recovery without network dependencies.

---

## 2. Network Security Foundation

The authentication architecture is built on a zero-trust networking model where all communication is authenticated via mutual TLS (mTLS) with verified SPIFFE workload identities. For detailed information on:

- PKI hierarchy and certificate management
- Workload identity (SPIFFE) formats
- mTLS enforcement and revocation
- Certificate enrollment and bootstrap flows
- Port topology and communication patterns
- g8e.local internal translation layer

See [Network Architecture](./network.md).

---

## 3. 5-Layer Verification Sequence (Interlock)

The platform implements a deterministic 5-layer governance sequence. Every mutation must pass through all active layers before execution. The structural schema is defined as `GovernanceEnvelope` in `protocol/proto/g8e/common/v1/common.proto:92`.

### Layer 1: Technical Bedrock (L1Doctrine)
*Implementation: `internal/services/governance/l1_doctrine.go:35`*

L1 is the foundational layer that executes deterministic security rules.
- **Forbidden Patterns**: Uses Protobuf field options (`forbidden_patterns`) to reject strings matching dangerous regex patterns on typed payload fields.
- **MITRE Threat Detection**: Analyzes payloads against MITRE ATT&CK patterns across 16 threat categories, including reverse shells, privilege escalation, credential access, data destruction, defense evasion, and cryptominer deployment. Analysis applies to `CommandRequested`, `McpCallRequested`, `A2ACallRequested`, and `FileEditRequested` payloads. MCP and A2A argument JSON is traversed recursively.
- **Critical System File Protection**: Blocks modifications to critical system paths defined in `CriticalSystemPaths` and critical directories defined in `CriticalSystemDirs`.
- **Hard Gates**: Rejects transactions immediately upon violation; cannot be bypassed by L2 or L3.

### Layer 2: Consensus (Tribunal)
*Implementation: `internal/services/tribunal/service.go:38`*

L2 provides multi-agent cryptographic attestation of payload safety through a Tribunal-based quorum system.
- **Tribunal Deliberation**: The `TribunalService` processes each `GovernanceEnvelope` through all tribunal members. Each member independently evaluates the payload and signs a vote. The envelope is returned with L2 metadata populated (`tribunal_id` + `L2Vote[]`).
- **Payload Hash Verification**: Verifies that `envelope.Id` matches the recomputed message hash before deliberation, ensuring the envelope has not been tampered with in transit.
- **MITRE Safety Evaluation**: Each member runs L1Doctrine threat detection (`AnalyzeCommand`) on extracted command data to produce a binary safety decision. If the L1Doctrine reference is absent, the payload is evaluated as unsafe (fail-closed).
- **Ed25519 Decision Signing**: Each member signs the string `{transaction_hash}|{decision}` using its Ed25519 private key and writes the hex-encoded signature to `L2Vote.consensus_signature` along with the signer's key ID (`signer_key_id`, which is the member's AppID).
- **Tribunal Quorum**: The L4 Warden verifies L2 votes by loading the `TribunalPolicy` from `TribunalStore`, checking that each signer is a tribunal member, resolving their Ed25519 public key from `SignerStore`, verifying the signature, and counting affirmative votes against the quorum threshold. The `RequireDistinct` option prevents duplicate signers.

### Layer 3: Notary (L3Notary)
*Implementation: `internal/services/governance/l3_notary.go:57` (`gatewayNotary` struct); interface at `internal/services/governance/l3_notary.go:35`*

L3 has three notary implementations:
- **`gatewayNotary`** (`internal/services/governance/l3_notary.go:57`): Gateway mode, constructed by `NewGatewayL3Notary`. Combines `cliSessionVerifier` (mTLS CLI) and `PasskeyService` (WebAuthn) as delegates. Passkey verification always runs first; `ErrPasskeyProofRequired` is returned if the proof lacks a `credential_id`. CLI mTLS session verification runs as the second layer when `MtlsCertFingerprint` is present.
- **`outboundNotary`** (`internal/services/governance/l3_notary.go:66`): Outbound mode, constructed by `NewOutboundL3Notary`. Performs suspended transaction lookup and Ed25519 signature verification only. No passkey or CLI session verification.
- **`cliNotary`** (`internal/services/governance/l3_notary.go:74`): Gateway CLI mode, constructed by `NewCLIL3Notary`. Performs CLI session verification (user active, session validity, cert revocation) before the shared suspended transaction and signature verification.

L3 ensures explicit human authorization for mutations.
- **Suspension**: The g8e Gateway suspends transactions requiring L3 approval, storing them in the suspended transaction pool.
- **Browser-Based Approval Flow**: The CLI calls `g8e approve <txHash>`, which opens a browser to `/api/v1/approve/{txHash}`. The gateway redirects to `/console/#approve={url.QueryEscape(txHash)}`, where the browser performs the WebAuthn passkey ceremony via `handleApprovalVerify`. The CLI polls `GET /api/v1/approvals/status/{txHash}` via mTLS until the transaction is approved or times out.
- **Layered Authorization Model**: Gateway mode uses a two-layer authorization model. Layer 1 (passkey authorization) is always required; proofs without a `credential_id` are rejected with `ErrPasskeyProofRequired`. The `PasskeyService` (`internal/services/gateway/passkey_service.go:77`) validates the WebAuthn assertion. Layer 2 (mTLS transport authentication) applies to CLI callers when `mtls_cert_fingerprint` is present. The `cliSessionVerifier` (`internal/services/gateway/cli_session_verifier.go:31`) verifies the CLI session as an additional transport-auth layer. Browser-only approvals skip Layer 2.
- **Outbound Mode**: When `passkeyVerifier == nil` (outbound L3 notary), only Ed25519 signature validation runs; no passkey is required. This is intentional for environments without a WebAuthn relying party.
- **Approval Window**: Approvals are valid for 30 minutes from the time of approval. Transactions not dispatched within that window are rejected and must be re-approved.
- **Unified Enrollment**: `performEnroll` replaces platform-specific enrollment paths. `RegisterPasskeyViaBrowser` opens the console UI for passkey registration and polls `VerifyPasskeyRegistration` (mTLS) until complete.
- **Gateway L3 Verification**: The `gatewayNotary` (`internal/services/governance/l3_notary.go:57`), constructed by `NewGatewayL3Notary` (`internal/services/governance/l3_notary.go:102`), implements the layered model. `NewGatewayL3Notary` accepts a `PasskeyService` (as `L3Notary`) for WebAuthn proof verification and a `CLISessionVerifier` for mTLS transport auth. The `PasskeyService` is passed as the `passkeyVerifier` delegate via `ls.passkey.PasskeyService` in `gateway_service.go:599`. Passkey verification always runs first; `ErrPasskeyProofRequired` is returned if the proof lacks a `credential_id`. CLI mTLS session verification runs as the second layer when `MtlsCertFingerprint` is present.
- **CLI Session Verifier**: The `cliSessionVerifier` (`internal/services/gateway/cli_session_verifier.go:31`) implements the `governance.CLISessionVerifier` interface and verifies that the user is active, the CLI session exists and belongs to the user, the certificate fingerprint matches the session's stored fingerprint (constant-time comparison), the session is active and not expired, and the certificate is not revoked via the PKI authority.
- **Passkey Service**: The `PasskeyService` (`internal/services/gateway/passkey_service.go:77`) handles L3 proof brokerage for WebAuthn operations, moving L3 authorization into the gateway as the sovereign authority. `VerifyL3Proof` remains on `PasskeyService` (not `PasskeyHandler`) to maintain the L3 binding to the transaction hash per architectural guardrails.
- **L3Proof**: A successful approval generates an `L3Proof` (defined in `protocol/proto/g8e/common/v1/common.proto:64`) containing the passkey WebAuthn fields (`credential_id`, `client_data_json`, `authenticator_data`, `signature`) and optional mTLS fields (`mtls_cert_fingerprint`), cryptographically bound to the `transaction_hash`.
- **Transition for Old CLI Binaries**: Old CLI binaries that send Ed25519-only L3 proofs (without `credential_id`) receive `ErrPasskeyProofRequired` from the gateway, providing a clear error rather than silent failure.

### Layer 4: Warden (L4Warden)
*Implementation: `internal/services/governance/l4_warden.go:246`*

The Warden is the final fail-closed gate before execution. It verifies in the following order:
1. **In-Flight Tracking**: Prevents concurrent processing of transactions with the same nonce via an in-memory `sync.Map` guard.
2. **Nonce Reservation**: Early durable replay protection via `ReplayStore.ReserveNonce`, committed to SQLite before any expensive cryptographic checks. Expiry is checked before reservation; expired transactions are rejected.
3. **Stateless Validation**: Structural integrity checks, action type recognition, typed payload decoding, L1Doctrine compliance, and transaction hash verification (both `id` and `transaction_hash` must equal the computed hash).
4. **Stateful Validation**: State Merkle root consistency check via `StateRootProvider`; rejects the envelope if the provided `state_merkle_root` does not match the current root.
5. **Posture Validation**: L2 and L3 enforcement based on the configured `GovernancePosture` (Doctrine, Consensus, or Notary). L2 signature verification loads the `TribunalPolicy` from `TribunalStore`, verifies each signer is a tribunal member, resolves their Ed25519 public key from `SignerStore` by `key_id`, verifies the signature over `{transaction_hash}|{decision}`, and counts affirmative votes against the quorum threshold. L3 proof verification delegates to the configured `L3Notary` implementation, typically the `gatewayNotary` constructed by `NewGatewayL3Notary`, which routes to `PasskeyService` (WebAuthn) or `cliSessionVerifier` (mTLS) based on proof type.

### Layer 5: Actuator (L5Actuator)
*Implementation: `internal/services/governance/l5_actuator.go:50`*

The Actuator represents the execution boundary and final audit commitment. L5Actuator does NOT re-verify L2 or L3 proofs. By design, L4Warden performs all pre-dispatch verification (L1 doctrine, L2 consensus, L3 notary) and embeds the results in `VerifiedTransaction`. L5 trusts that `VerifiedTransaction`, records the L2/L3 status in the `ActionReceipt` for audit, and focuses on execution safety. The separation between L4 (verification) and L5 (execution) is the defense-in-depth boundary: two independent components with distinct responsibilities.
- **Fail-Closed Pre-Execution**: Receipt signing and initial audit logging must both succeed before the execution handler is invoked. If either fails, the transaction is aborted.
- **Sensitive Data Rehydration**: Rehydrates scrubbed placeholders (such as `{{UEI_1}}`) with original sensitive data just before execution via `ScrubbingService.RehydratePayload`.
- **JIT Capability Minting**: Mints a scoped, single-action, self-dissolving `Capability` (`internal/services/governance/capability.go:39`) from the `VerifiedTransaction` via `MintCapability` (`internal/services/governance/capability.go:117`). The capability binds the action type, target resource, transaction hash, and expiry, and is injected into the execution context. The capability is dissolved immediately after execution completes or fails, ensuring zero standing privileges outside the lifetime of a single `Execute()` call.
- **Egress Dispatch**: Dispatches the verified payload to downstream executors (Shell, MCP, A2A) via `ExecutionHandler.ExecuteVerifiedTransaction`.
- **Action Receipts**: Issues a signed `ActionReceipt` using the Actuator's own Ed25519 key over a canonical JSON serialization of the receipt fields, providing immutable proof of the execution outcome.
- **Commitment**: Records the transaction in the `SQLAuditStore` (from `CanonicalDBService.AuditStore` in both gateway and outbound modes) and, where configured, in the console audit store.

---

## 4. Governance Postures

Postures define which layers of the bedrock are enforced as fail-closed gates. The `GovernancePosture` interface and its three implementations are defined in `internal/services/governance/posture.go:31`. When a layer is "Audited", verification still runs but failures do not block execution; results are recorded for audit. When a layer is "Enforced", any verification failure aborts the transaction.

| Posture | L1 (Doctrine) | L2 (Consensus) | L3 (Notary) | Use Case |
| :--- | :---: | :---: | :---: | :--- |
| **Doctrine** | Enforced | Audited | Audited | Local Dev / CI |
| **Consensus** | Enforced | Enforced | Audited | Automated Workflows |
| **Notary** | Enforced | Enforced | Enforced | **Production (Default)** |

---

## 5. Sovereign Execution Boundary

Handling sensitive data without leaking it to upstream models is managed by the Sovereign Execution Boundary:
- **Scrubbing**: Private data is replaced with opaque tokens (Uniform Element Identifiers, such as `{{UEI_1}}`) before sending to external LLMs.
- **Deterministic Rehydration**: The L5 Actuator performs local rehydration of tokens just before execution via `RehydratePayload`.
- **Data Sovereignty**: Raw secrets never leave the sovereign host environment.

---

## 6. Encryption at Rest

The platform enforces mandatory encryption for all sensitive data at rest. See [Encryption Architecture](./encryption.md) for complete details.

### Vault-Based Encryption

All storage services require an unlocked vault at initialization:
- **SQLAuditStore**: Encrypts event content fields (`content_text`, `command_stdout`, `command_stderr`) at rest and records action receipts and file mutations
- **ExecutionVaultService**: Encrypts execution results (stdout, stderr) and file diffs
- **TokenStore** (`EncryptedKVAdapter`): Encrypts scrubbing tokens (UEI placeholders) managed by the `ScrubbingService`, stored in the shared `kv_store` table of the canonical gateway database

### Encryption Guarantees

- **Fail-closed**: Services fail to initialize without a vault
- **AES-256-GCM**: All data encrypted with NIST-approved algorithm
- **Key rotation**: Support for re-keying without data loss
- **Zero-knowledge**: Vault keys never written to disk in plaintext

### Vault Management

Vault operations are managed via CLI commands:
- `./g8e vault init`: Initialize new vault
- `./g8e vault unlock`: Unlock vault with key
- `./g8e vault rekey`: Rotate vault keys
- `./g8e vault status`: Check vault status
- `./g8e vault export`: Export the vault key in hex format
- `./g8e vault import`: Import a vault key from hex string or stdin
- `./g8e vault reset`: Destroy vault (destructive)

### Configuration

Vault paths can be configured via:
- CLI flags: `--vault-dir`, `--vault-key`
- Environment variables: `G8E_VAULT_DIR`, `G8E_VAULT_KEY`
- Default configuration: embedded in binary via `config.DefaultInfraPaths()`

---

## 7. Session & Identity Binding

Binding is the cryptographic and stateful linkage between platform sessions (web, CLI, operator) and the identities, agents, and Operator instances they authorize. It is the mechanism that answers: *"Which Operator is allowed to act on behalf of which session?"* and *"Which app is allowed to push events to which target?"*

The g8e platform uses binding in five distinct contexts:

| Context | Purpose | Storage |
| :--- | :--- | :--- |
| **Session Binding (Web ↔ Operator)** | Links a browser session to one or more Operator sessions | KV store + doc store |
| **CLI Cert Binding** | Verifies a CLI mTLS cert is linked to a specific Operator session | Doc store (CLI sessions) |
| **Binding Persona** | Maps JWT roles to an internal persona for authorization context | Request context |
| **PKI Credential Binding** | Binds app + human requestor identity into a delegated credential | Certificate SANs |
| **Envelope Binding** | Stamps app/operator/user identity onto governance envelopes | Envelope fields |

---

### 7.1 Session Binding: Web ↔ Operator

#### 7.1.1 Concept

When a user interacts with the g8e Dashboard (web session), they may control multiple Operators running on different hosts. **Session binding** is the bidirectional linkage that records which Operator sessions are associated with a given web session. This linkage is required before:

- SSE events can be pushed to a web session target
- An Operator can be set as the active target context
- Commands can be dispatched from a web session to an Operator

#### 7.1.2 KV Key Scheme

Binding uses the platform KV store with three key prefixes defined in `internal/services/gateway/registration_service.go`. The web-to-operator key (`g8e:sessions:web:{web_session_id}:bind`) maps to a JSON array of operator session ID strings. The operator-to-web key (`g8e:sessions:operator:{operator_session_id}:bind`) maps to a single web session ID string. A CLI binding prefix (`g8e:sessions:cli:{cli_session_id}:bind`) is reserved for future use.

The web-to-operator direction is a **one-to-many** mapping (one web session can bind multiple operators). The operator-to-web direction is **one-to-one** (an operator session is bound to at most one web session at a time). Helper functions `sessionWebBindKey` and `sessionOperatorBindKey` construct the full KV keys from prefix, ID, and suffix constants.

#### 7.1.3 Durable Document: `BoundSessionsDocumentGo`

In addition to the KV store (which is optimized for fast lookups), binding state is persisted in the `bound_sessions` document collection as a `BoundSessionsDocumentGo` (`internal/models/auth.go`):

| Field | Type | Description |
| :--- | :--- | :--- |
| `id` | string | Document ID (equals `web_session_id`) |
| `web_session_id` | string | The browser session |
| `user_id` | string | Owning user |
| `operator_session_ids` | []string | All bound Operator session IDs |
| `operator_ids` | []string | All bound Operator IDs |
| `bound_at` | time.Time | Initial binding timestamp |
| `last_updated_at` | time.Time | Last modification timestamp |
| `status` | OperatorStatus | `active` when operators are bound, `terminated` when all unbound |

This document serves as the **durability layer**; if the KV store is lost, the bound sessions document can reconstruct bindings.

#### 7.1.4 API Endpoints

| Endpoint | Method | Description |
| :--- | :--- | :--- |
| `/api/v1/operators/bind` | POST | Bind one or more operators to a web session |
| `/api/v1/operators/unbind` | POST | Unbind one or more operators from a web session |
| `/api/v1/operators/target` | POST | Set the active target Operator for a web session (binds if not already bound) |

#### 7.1.5 Bind Operation (`BindOperators`)

Location: `internal/services/gateway/registration_service.go:448`

For each operator ID in the request:

1. **Validate**: fetch the Operator document, verify it belongs to the requesting user, and confirm it has an active `OperatorSessionID`.
2. **KV: operator→web**: set `g8e:sessions:operator:{op_session_id}:bind` → `web_session_id`.
3. **KV: web→operator**: fetch existing `g8e:sessions:web:{web_session_id}:bind`, append the operator session ID if not already present, write back.
4. **Durable doc**: create or update the `BoundSessionsDocumentGo` in the `bound_sessions` collection, adding the operator ID and session ID.
5. **Operator doc**: stamp `bound_web_session_id` on the Operator document for UI consumption.

The request type (`BindOperatorsRequest` in `internal/models/auth.go`) carries the operator IDs to bind, the requesting user ID, and the web session ID. The response type (`BindOperatorsResponse`) reports success/failure counts, lists of bound and failed operator IDs, and an optional error string.

#### 7.1.6 Unbind Operation (`UnbindOperators`)

Location: `internal/services/gateway/registration_service.go:624`

For each operator ID:

1. **Validate**: fetch the Operator document, verify ownership.
2. **KV: operator→web**: delete `g8e:sessions:operator:{op_session_id}:bind`.
3. **KV: web→operator**: fetch the web bind key, remove the operator session ID from the array. If the array is now empty, delete the key entirely; otherwise write back the reduced array.
4. **Durable doc**: remove the operator from `BoundSessionsDocumentGo`. If no operators remain, set status to `terminated`.
5. **Operator doc**: clear `bound_web_session_id` (set to empty string).

#### 7.1.7 Target Context (`SetTargetContext`)

Location: `internal/services/gateway/registration_service.go:762`

Sets the active target Operator for a web session. If the Operator is not already bound to the web session (`op.BoundWebSessionID != req.WebSessionID`), it calls `BindOperators` first to establish the binding. This ensures target context operations always operate over a bound relationship.

#### 7.1.8 Operator Document Fields

The Operator document (`internal/models/auth.go`) carries binding state:

| Field | JSON Key | Description |
| :--- | :--- | :--- |
| `BoundWebSessionID` | `bound_web_session_id` | Web session the Operator is currently bound to. Set during bind, cleared on unbind. |
| `OperatorSessionID` | `operator_session_id` | The Operator's own session ID, used as the KV key for binding lookups. |

#### 7.1.9 Operator Status: `bound`

The `bound` status (`internal/constants/status.go:74`) is one of the valid Operator lifecycle states:

```
available → bound → active → stopped → terminated
                 ↘ offline
                 ↘ stale
```

When an Operator is bound to a web session, its status transitions to `bound`. The corresponding status-updated event is `g8e.v1.operator.status.updated.bound` (`protocol/constants/events.json`).

---

### 7.2 CLI Cert Binding to Operator

#### 7.2.1 Concept

A CLI client (`g8e enroll`) receives its own mTLS certificate with a SPIFFE URI SAN identifying the CLI session. The CLI session is itself linked to an Operator session via the `OperatorSessionID` field on the `CLISession` model. This creates a chain:

```
CLI mTLS cert (SPIFFE URI) → CLI session → OperatorSessionID → Operator session
```

#### 7.2.2 `cliCertBoundToOperator`

Location: `internal/services/gateway/gateway_auth.go:753`

This function verifies that a presented client certificate belongs to a CLI session whose `OperatorSessionID` matches the claimed operator session. It is used during authentication to allow CLI clients to call internal APIs scoped by `cli_session_id` while presenting their CLI mTLS cert and the linked operator session as a Bearer token.

**Algorithm:**

1. Extract SPIFFE URIs from the client certificate's SANs.
2. Match against `WorkloadIdentity.MatchesCLI(uri, userID, cliSessionID)` or `MatchesCLISessionOnly(uri, cliSessionID)`.
3. Load the CLI session document from the `cli_sessions` collection.
4. Check that the session has not expired.
5. Return `cliSession.OperatorSessionID == operatorSessionID`.

#### 7.2.3 CLI Session Persistence

Location: `internal/services/gateway/cli_session_service.go:44`

`PersistCLISession` creates the CLI session document with the `OperatorSessionID` field that establishes the binding. The session record also stores the user ID, system fingerprint, certificate fingerprint, and certificate serial for later verification.

---

### 7.3 Binding Persona (JWT Auth Context)

#### 7.3.1 Concept

When external Identity Provider (IdP) JWT tokens are used for authentication, the JWT `roles` claim is mapped to an internal **binding persona** string. This persona is stamped into the request context and used downstream by the governance envelope builder.

#### 7.3.2 Flow

Location: `internal/services/gateway/gateway_auth.go:881-965`

1. JWT is validated against the configured JWKS endpoint.
2. `PersonaService.MapRolesToPersona(jwt.Roles)` maps the JWT roles to a persona string. If mapping fails, `"default"` is used.
3. The persona is stamped into the request context via `ContextKeyBindingPersona`.
4. The MCP gateway envelope builder (`internal/services/mcp/gateway.go:737`) reads the persona from context and sets `env.BindingPersona` on the governance envelope.

#### 7.3.3 Context Key

Defined in `protocol/constants/auth.json` as the `binding_persona` entry, which maps to the Go constant `ContextKeyBindingPersona` with the value `binding_persona`.

---

### 7.4 PKI Credential Binding (Delegated App Credentials)

#### 7.4.1 Concept

When `g8e mcp agent run` launches an AI agent, it requests a short-lived delegated credential from the Gateway's `/api/v1/pki/apps/delegated` endpoint. This credential **binds both** the app identity and the human requestor identity into the certificate's SANs.

#### 7.4.2 Implementation

Location: `internal/services/gateway/pki_controller.go:357`

The `handlePKIAppsDelegated` handler:

1. Requires mTLS authentication from a human CLI session (not an app cert).
2. Extracts the user ID from the CLI certificate.
3. Mints a short-lived (1-hour) certificate with dual URI SANs (`internal/services/gateway/gateway_certs.go:818-828`):
   - App identity: `spiffe://g8e.local/app/<app_name>`
   - Requestor identity: `spiffe://g8e.local/user/<user_id>`
4. The resulting credential is injected into the agent subprocess via `G8E_APP_CERT` / `G8E_APP_KEY` environment variables.

#### 7.4.3 Envelope Binding

Location: `internal/services/mcp/gateway.go:718-756`

The governance envelope builder binds both identities to the envelope:

- `env.OperatorId` / `env.OperatorSessionId` / `env.ActingAppId`: set from the app identity (for delegated credentials) or operator identity (for operator sessions).
- `env.RequestorUserId`: set from the human user ID.
- `env.BindingPersona`: set from the binding persona context key.
- `env.TenantId`: set from the tenant ID context key.

This ensures every governance envelope carries the full identity chain: who requested it (human), what executed it (app/operator), and under which persona/tenant.

---

### 7.5 SSE Authorization via Binding

#### 7.5.1 Push Authorization (App → Target)

Location: `internal/services/gateway/gateway_http_sse.go:129-235`

When an app workload pushes an SSE event to a target session, the Gateway verifies the app is authorized for that target by checking the binding:

**Web session target:**
1. Look up `sessionWebBindKey(web_session_id)` in KV → get list of bound operator session IDs.
2. For each bound operator session, check if the app's SPIFFE ID matches via `WorkloadIdentity.MatchesApp(appID, operatorID)`.
3. If no match, return `403 Forbidden`.

**CLI session target:**
1. Load the CLI session document.
2. Get the `OperatorSessionID` from the CLI session.
3. Load the Operator document for that session.
4. Verify `WorkloadIdentity.MatchesApp(appID, operatorID)`.
5. If no match, return `403 Forbidden`.

#### 7.5.2 Stream Authorization (Consumer → Events)

Location: `internal/services/gateway/gateway_http_sse.go:275-339`

When a consumer connects to the SSE stream, the Gateway verifies the Operator session is bound to the declared routing target:

- **CLI session target:** Load the CLI session, verify `cliSess.OperatorSessionID == operatorSessionID`.
- **Web session target:** Look up `sessionOperatorBindKey(operatorSessionID)` in KV, verify the returned `web_session_id` matches the requested `route.WebSessionID`.
- **User-scoped target:** Validate the Operator session, verify `op.UserID == route.UserID`.

---

### 7.6 Protocol Constants & Event Taxonomy

#### 7.6.1 KV Key Constants

Defined in `protocol/constants/kv_keys.json`:

| Constant | Key Pattern | Purpose |
| :--- | :--- | :--- |
| `KVKeySessionOperatorBind` | `g8e:sessions:operator:{operator.session.id}:bind` | Operator → web session |
| `KVKeySessionWebBind` | `g8e:sessions:web:{web.session.id}:bind` | Web session → operator sessions |

#### 7.6.2 HTTP Header

| Constant | Header | Purpose |
| :--- | :--- | :--- |
| `HeaderBoundOperators` | `X-G8E-Bound-Operators` | Communicates bound operator info in HTTP responses |

#### 7.6.3 Operator Events

Defined in `internal/constants/events.go` and `protocol/constants/events.json`:

| Constant | Value | Description |
| :--- | :--- | :--- |
| `EventOperatorBound` | `g8e.v1.operator.bound` | Operator bound event |
| `EventOperatorUnbound` | `g8e.v1.operator.unbound` | Operator unbound event |
| `EventOperatorStatusUpdatedBound` | `g8e.v1.operator.status.updated.bound` | Operator status changed to bound |
| `HistoryEventTypeBound` | `bound` | History event type for binding (`internal/constants/status.go`) |

#### 7.6.4 Agent Modes

Defined in `internal/constants/prompts.go`:

| Constant | Value | Description |
| :--- | :--- | :--- |
| `AgentModeG8eBound` | `g8e.bound` | Operator is bound |
| `AgentModeCloudOperatorBound` | `g8e.cloud.bound` | Cloud operator is bound |
| `AgentModeG8eNotBound` | `g8e.not.bound` | Operator is not bound |

#### 7.6.5 Agent Activity Metadata

Defined in `protocol/models/agent_activity_metadata.json`:

| Field | Type | Description |
| :--- | :--- | :--- |
| `operator_bound` | boolean | Whether operators were bound |
| `bound_operator_count` | integer | Number of bound operators |

#### 7.6.6 Reputation & Stake Binding

Both `protocol/models/reputation_commitment.json` and `protocol/models/stake_resolution.json` contain a `binding` object that links the record to a specific `investigation_id`, ensuring reputation and stake data are cryptographically bound to the investigation context.

---

### 7.7 Binding Error Constants

All binding-related errors are defined in `internal/constants/errors.go`:

| Constant | Description |
| :--- | :--- |
| `ErrIdentityBindingFailed` | Identity binding failed |
| `ErrRegistrationFailedToSetKVBinding` | Failed to set KV binding |
| `ErrRegistrationFailedToMarshalSessionIDs` | Failed to marshal session IDs |
| `ErrRegistrationFailedToGetBoundSessions` | Failed to get bound sessions document |
| `ErrRegistrationFailedToMarshalBoundSessions` | Failed to marshal bound sessions document |
| `ErrRegistrationFailedToMarshalExistingDocument` | Failed to marshal existing document |
| `ErrRegistrationFailedToSetBoundSessions` | Failed to set bound sessions document |
| `ErrRegistrationFailedToUnmarshalBoundSessions` | Failed to unmarshal bound sessions document |
| `ErrRegistrationFailedToUpdateBoundSessions` | Failed to update bound sessions document |
| `ErrRegistrationFailedToBindOperator` | Failed to bind Operator for target context |

---

### 7.8 Binding Data Flow Diagram

```
                    ┌─────────────┐
                    │  Web Browser │
                    │  (Dashboard)  │
                    └──────┬───────┘
                           │ web_session_id
                           ▼
                    ┌──────────────┐
                    │  Web Session  │
                    │  (doc store)  │
                    └──────┬───────┘
                           │
              ┌────────────┼────────────┐
              │ KV: web→op │  Doc:     │
              │ bind key   │  bound_   │
              │            │  sessions │
              ▼            └───────────┘
     ┌────────────────┐
     │ Operator Sess 1 │ ←── KV: op→web bind key
     │ (host A)        │
     └────────────────┘
     ┌────────────────┐
     │ Operator Sess 2 │ ←── KV: op→web bind key
     │ (host B)        │
     └────────────────┘

                    ┌─────────────┐
                    │  CLI Client  │
                    │  (g8e enroll) │
                    └──────┬───────┘
                           │ mTLS cert (SPIFFE URI)
                           ▼
                    ┌──────────────┐
                    │  CLI Session  │
                    │  (doc store)  │
                    │  .Operator    │
                    │  SessionID    │
                    └──────┬───────┘
                           │
                           ▼
                    ┌──────────────┐
                    │  Operator     │
                    │  Session      │
                    └──────────────┘
```

---

### 7.9 Binding Security Properties

1. **Bidirectional updates**: Both KV directions (web→operator and operator→web) are updated during bind/unbind. During bind, if the operator→web KV write succeeds but the web→operator write fails, the operator→web binding is not rolled back; the operator is added to the failed list. During unbind, KV deletion failures are logged as warnings and do not prevent the operator from being unbound.

2. **Ownership enforcement**: Bind/unbind operations verify that the Operator belongs to the requesting user (`op.UserID == req.UserID`), preventing cross-user binding attacks.

3. **Session expiry checks**: CLI cert binding checks `cliSession.ExpiresAt` before accepting the binding, preventing expired sessions from being used.

4. **SSE push authorization**: Apps cannot push events to sessions they are not bound to. The binding chain (app → operator → web/CLI session) is verified on every push.

5. **SSE stream authorization**: Consumers cannot read events from sessions their Operator is not bound to. The binding is verified at stream connection time.

6. **Durable recovery**: The `BoundSessionsDocumentGo` document provides a durability layer. If the KV store is lost, bindings can be reconstructed from the document store.

7. **Envelope identity binding**: Every governance envelope carries the full identity chain (app, operator, user, persona, tenant), ensuring auditability of all mutations.
