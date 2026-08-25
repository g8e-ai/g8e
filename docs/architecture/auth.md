# Authentication & Authorization

Last Updated: 2026-08-25
Version: v2.0.0

This document explains how to authenticate and authorize actions in the g8e platform. The platform is built as a zero-trust execution environment where every action is verified before execution.

## Overview

The platform security model is built on two core principles:

1. **Identity-Bound Communication (mTLS)**: Every connection must be authenticated via mutual TLS (mTLS) with a verified identity. This applies to CLI, Console, AI Agent, and Operator connections.

2. **5-Layer Verification Sequence**: Every action (command execution, file edit, tool call) must pass through a sequential 5-layer verification pipeline before execution.

## 1. Authentication

Authentication is how you prove your identity to the platform. The g8e platform uses multiple authentication methods depending on your use case.

### 1.1 CLI Authentication

The CLI uses mTLS certificates for authentication. When you run `g8e auth enroll user`, the command inspects the local credential state and chooses the appropriate path: reuse an existing healthy identity, bootstrap a new one, recover a partial or corrupt one, or rotate a certificate. The passkey ceremony is run through a browser to prove that a human controls the enrolling identity.

**Key Concepts:**

- No shared secrets or API keys to leak.
- You prove your identity by signing with your private key on every call.
- The Gateway acts as the Certificate Authority (CA).
- A single enrollment flow owns all local CLI enrollment state transitions.
- Local credentials are managed atomically with staged writes and restricted file permissions, so torn state is not written to disk.

**Enrollment State Machine:**

The CLI classifies the local identity on disk into one of four states and takes the appropriate action. Credential state is determined purely by local file consistency. It never contacts the gateway, so a complete identity with a stale trust bundle (for example from a previous gateway instance after `gw clean`) is indistinguishable from a healthy one until a live-gateway liveness check is layered on top before the final decision.

| State | Condition | Action |
|------|-----------|--------|
| **Complete** | CLI cert, CLI key, and credentials JSON all present and valid | **Reuse** - no new certificate is issued. The existing identity is used as-is. Routed to **Recovery** instead when the local trust bundle is stale against the live gateway. |
| **Absent** | No local credentials found | **Bootstrap** - CLI connects over plain HTTP, the Gateway bootstraps itself, then runs the passkey ceremony. |
| **Partial** | Some credential files present but others missing | **Recovery** - initiates the one-time human-approved recovery flow. Does NOT silently overwrite. |
| **Corrupt** | Credential files present but fail validation (for example expired cert, key mismatch) | **Recovery** or **Rotation** depending on the nature of the corruption. |

Healthy `auth enroll user` runs with a complete identity do not rotate credentials unexpectedly. The `--rotate-cli` flag forces rotation even when the identity is complete.

**Enrollment Scenarios:**

| Scenario | When It Happens | How It Works |
|----------|----------------|--------------|
| **First-time setup** | Gateway never bootstrapped | CLI connects over plain HTTP to the gateway HTTP port, the Gateway bootstraps itself. |
| **New CLI on existing gateway** | Gateway exists, no local credentials | CLI bootstraps, generates an enrollment token, opens browser for passkey ceremony. |
| **Recovery (partial/corrupt)** | Some credentials missing or invalid | One-time human-approved recovery flow via the Console SPA, or via the mTLS approve-cli endpoint under `--headless`. |
| **Recovery (stale bundle)** | Credentials complete but local trust bundle does not match the live gateway root CA (for example after `gw clean` regenerated the gateway PKI) | One-time human-approved recovery flow - the old CLI cert cannot authenticate to the new gateway via mTLS, so rotation is impossible. Recovery issues a fresh cert signed by the new CA. |
| **Headless** | `--headless` flag used on `auth enroll user` (any of the above recovery scenarios, or bootstrap on a bootstrapped gateway) | CLI-only identity: passkey ceremony skipped, OS trust installation skipped, recovery approval delegated to an already-enrolled CLI via `g8e auth approve-recovery <token>` over the mTLS approve-cli endpoint. The resulting identity is mTLS-only and cannot authenticate to the Console SPA. See [Headless CLI Enrollment (mTLS-Only)](#headless-cli-enrollment-mtls-only). |
| **Rotation** | Credentials valid but `--rotate-cli` flag used, or cert near expiry | mTLS-protected rotation: one replacement certificate per run. |
| **Reuse** | Credentials complete and valid, and the local trust bundle matches the live gateway root CA | No new certificate is issued - existing identity is reused. The local trust bundle is refreshed from the live gateway if intermediates differ but the root is unchanged. |

**Two-Phase Enrollment & Split Endpoint Flags:**

Enrollment involves two phases that use different ports and protocols:

1. **Discovery/bootstrap phase** (plain HTTP): CA bundle fetch, live gateway CA discovery, bootstrap status check, CSR trust bundle retrieval.
2. **mTLS API phase** (HTTPS): Enrollment token generation, CSR submission, SSE stream, API client operations.

**Live Gateway CA Discovery:**

Before the state-machine switch, the CLI fetches the live gateway root CA bundle from the unauthenticated discovery surface (`GET /.well-known/g8e/pki/ca-bundle` on the plain-HTTP port) and derives its SHA-256 fingerprint locally. This is a single best-effort round-trip.

- On success, the live fingerprint is compared against the local trust bundle's primary root fingerprint. A mismatch marks the local bundle as stale and routes a complete identity to **Recovery** instead of Reuse (the old CLI cert was issued by the old CA and cannot authenticate to the new gateway via mTLS, so rotation is impossible). The live fingerprint is also used to filter stale OS anchors.
- On network failure, the CLI cannot determine whether the bundle is stale. It prints a diagnostic warning naming the `gw clean` scenario and the `--endpoint` flag, then proceeds to the existing state machine. It does NOT abort, so the air-gapped or offline case still works. If the bundle is in fact stale, the subsequent mTLS call surfaces a TLS error, but with prior context.
- Discovery runs unconditionally at the top of `Enroll` (one cheap round-trip) so the live fingerprint is available for all paths, but only the complete-reuse path uses the stale-bundle check for routing. The new-enrollment paths (bootstrap/recovery/rotation) receive a fresh bundle in their artifacts, so discovery is redundant for routing but still supplies the live fingerprint for stale-anchor detection.
- No fingerprint pin is applied - the live bundle IS the source of truth for the pin, so pinning against the local bundle would be circular.

**Stale Trust Bundle on Reused Identity:**

When the gateway PKI is regenerated (`gw clean`, PKI rotation, gateway migration to a new host with a fresh CA) and a workstation holds a complete identity from the old gateway, the local trust bundle and the OS trust store are both stale in lockstep. Without the discovery step, the CLI would trust the local bundle as the source of truth, see that the OS store matches it, conclude "already trusted," and then fail the passkey ceremony's mTLS call with a raw `x509: certificate signed by unknown authority` error - with no diagnosable cause. The discovery step surfaces this condition before any mTLS call and routes to Recovery, which issues a fresh cert signed by the new CA over plain HTTP (no mTLS required).

By default, the CLI connects to `g8e.local` (or the machine IP fallback) on the default ports (HTTP 8080, HTTPS 8443). When the gateway's HTTP and HTTPS ports are mapped to different host ports, as in Docker demos, use the split endpoint flags:

| Flags | HTTP override | HTTPS override | Use case |
|---|---|---|---|
| `-e localhost` | `localhost` (default HTTP port) | `localhost` (default HTTPS port) | Same-host, default ports |
| `-e localhost:8085` | `localhost:8085` | `localhost` (default HTTPS port) | HTTP port override only |
| `-e localhost --port 8448` | `localhost` (default HTTP port) | `localhost:8448` | HTTPS port override only |
| `-e localhost:8085 --port 8448` | `localhost:8085` | `localhost:8448` | Docker demo (split ports) |
| `--port 8448` | `localhost` (default HTTP port) | `localhost:8448` | HTTPS port override, localhost HTTP |

The `--endpoint` flag (`-e`) sets the HTTP discovery endpoint (host or host:port). The `--port` flag sets the HTTPS/mTLS port (overrides default 8443). When only `--endpoint` is provided, both HTTP and HTTPS use the same host. When both are provided, the CLI splits them into independent overrides for each phase.

**Enrollment Token Flow:**

To prevent exposing raw session identifiers in browser history, referrer headers, and screen-share surfaces, the enrollment process uses a one-time enrollment token:

1. CLI generates a one-time enrollment token via the mTLS endpoint `POST /api/v1/auth/enrollment-token/generate`.
2. CLI opens the browser with `#enroll=1&token=<token>` (no raw `user_id` or `cli_session_id` in the URL).
3. The Console SPA reads the token from the URL hash and immediately clears it from the URL.
4. SPA posts the token directly to `POST /api/v1/auth/passkeys/enrollment/register/challenge` - the gateway validates the token and derives `user_id` and `cli_session_id` from it; there is no separate token-validation round-trip and the token-derived identifiers never touch the DOM.
5. SPA performs the WebAuthn ceremony with the challenge response.
6. SPA posts the attestation plus token to `POST /api/v1/auth/passkeys/enrollment/register/verify` - the verify step consumes the token (one-time-use) and sets a web session cookie.
7. The `cli_session_id` carried by the token flows into the `passkey.registered` SSE event, which the waiting CLI receives.
8. The token is one-time-use with a 5-minute TTL.
9. Expired tokens are cleaned up periodically by the gateway.

This ensures that sensitive session identifiers are never exposed in browser history or referrer headers, while maintaining the security of the enrollment flow.

**System Trust Installation:**

By default, `auth enroll user` installs the gateway Root CA into the OS trust store **before** opening the browser for the passkey ceremony. This ensures the browser recognizes the gateway's TLS certificate during the WebAuthn flow.

- Before installation, the CLI checks for stale g8e Root CA anchors from previous gateway instances (for example after `gw clean` regenerated the CA). The active fingerprint used to filter the stale list is the live gateway root fingerprint from the discovery step - NOT the local bundle's fingerprint. Using the local bundle as the source of truth was the original bug on the reused-identity path: the local bundle and the OS store are stale in lockstep, so they agreed with each other and the detector saw nothing wrong. On the new-enrollment paths (bootstrap/recovery/rotation), the artifacts' bundle IS the live bundle, so the behavior is unchanged. When discovery was unreachable, the CLI falls back to the bundle's own fingerprint (preserving the pre-discovery behavior) after printing the diagnostic warning.
- If stale anchors are found, the user is prompted to confirm removal. Declining aborts enrollment before browser launch.
- If system trust installation fails, the CLI **stops before launching the browser**. The user sees the error and guidance to restart the browser after manually installing the Root CA.
- Use `--no-system-trust` only when an administrator has already installed the Root CA on the host. This is an administrator-managed trust opt-out - it does **not** skip the passkey step, and it does **not** enable headless enrollment. Stale-anchor detection **still runs** under `--no-system-trust` (the user may have stale anchors from a previous gateway that break the browser even when the CLI skips installation); only the installation step is skipped. When stale anchors are removed under `--no-system-trust`, the blocking browser-restart gate fires (see below).
- After trust installation or stale anchor removal, the CLI **blocks on a browser-restart gate**: it prints "close all open browser windows" and waits for the user to press Enter before proceeding to the passkey ceremony. The gate is skipped when the passkey ceremony is suppressed for internal callers, so the line does not pollute non-interactive output streams. When the trust store was not changed this run (root already trusted, no stale anchors removed), there is no prompt - the passkey ceremony proceeds directly.
- Firefox and other browsers with private trust stores may require separate handling even after OS trust is installed.
- `gw clean` removes g8e root CA anchors from the OS trust store before wiping the runtime directory. After clean there is no "current" anchor, so the stale-anchor lister uses an empty keep-fingerprint and every g8e anchor is listed and removed. This runs before the runtime wipe so that, if OS cleanup fails with an elevation error, the user can retry while the runtime state is still intact. Best-effort: on unsupported platforms or any trust-store error, the runtime wipe proceeds (the runtime wipe is the destructive primary action).

**CLI Recovery Flow (One-Time Human Approval):**

When local credentials are partial or corrupt, the CLI initiates a recovery flow that requires one-time human approval through the Console SPA:

1. CLI sends a recovery request to `POST /api/v1/auth/cli/recovery/request` (public, token-scoped).
2. Gateway creates a recovery record with an opaque token and a 10-minute TTL.
3. CLI opens the browser to the Console SPA with the token in the URL fragment (`#recovery=1&token=<token>`) - the token never appears in server logs, referrer headers, or browser history. Under `--headless`, this step is replaced: the CLI prints `g8e auth approve-recovery <token>` for an already-enrolled CLI to run instead of opening a browser (see [Headless CLI Enrollment (mTLS-Only)](#headless-cli-enrollment-mtls-only)).
4. An authenticated user (existing browser session) approves the recovery at `POST /api/v1/auth/cli/recovery/approve` (web-session protected). Under `--headless`, an already-enrolled CLI approves via `POST /api/v1/auth/cli/recovery/approve-cli` (mTLS protected) using `g8e auth approve-recovery <token>`; the approver user ID is derived from the verified CLI certificate URI SAN by the unified auth middleware.
5. CLI polls `GET /api/v1/auth/cli/recovery/status` until the recovery is approved or expires.
6. On approval, CLI calls `POST /api/v1/auth/cli/recovery/complete` to receive a new CLI certificate. Proof-of-possession of the CSR private key is required regardless of which approval path was used.
7. The recovery token is one-time-use; expired or replayed tokens are rejected on both approval paths.

The recovery flow is the only path for a CLI with partial or corrupt credentials. There is no silent overwrite or fallback to plain-HTTP enrollment.

**CLI Rotation Flow (mTLS-Protected):**

When the `--rotate-cli` flag is used or the certificate is near expiry, the CLI initiates rotation:

1. CLI uses its existing mTLS certificate to authenticate to `POST /api/v1/auth/cli/rotate`.
2. The caller's identity (user ID + active CLI session ID) is derived from the verified mTLS certificate URI - not from request body fields.
3. Gateway issues a replacement CLI certificate and revokes the old one.
4. Only **one replacement certificate** is issued per rotation run.
5. Rotation is mTLS-only and is never available on plain HTTP.

**CLI Session Refresh Flow (mTLS-Protected):**

CLI sessions have a TTL of 7 days, aligned with the CLI certificate TTL (7 days) so the session does not expire while the cert is still valid. When the session does expire (gateway restart, manual deactivation, or session record loss after a gateway volume reset) but the certificate is still valid, the CLI session refresh endpoint reissues a session using the still-valid cert as proof of identity. The cert is NOT rotated — the cert is the proof of identity, and the cert's URI SAN binds the new session to the same user.

1. CLI calls `POST /api/v1/auth/cli/refresh` (mTLS-protected, `RouteAuthMTLS`) using its existing certificate. The request body is empty — the cert is the proof of identity.
2. The unified auth middleware (`handleMTLSAuth` → `handleCLIAuth`) detects the expired or missing session and routes to `handleCLIRefreshAuth`, which extracts the user ID from the cert URI SAN (never from the request body or the expired session record), validates the cert URI SAN session ID matches the header-provided session ID, and verifies the user is still active.
3. The gateway deactivates the old session (if it still exists and is active) and issues a new CLI session bound to the same user and cert, inheriting the operator binding and cert fingerprint from the old session. When the old session is missing (for example after a gateway volume reset that wiped CLI sessions but left operator sessions intact), the gateway looks up the user's active operator session to inherit its binding. If no active operator session exists, the refresh returns 409 and the caller must re-enroll with `auth enroll user` to establish a fresh operator binding.
4. The CLI persists the new session ID atomically and prints the new session ID.

This is the recovery path for an expired CLI session with a still-valid cert. When the certificate itself has expired, an expired cert cannot authenticate via mTLS, so it can never reach this endpoint — the caller must use the recovery flow (`auth enroll user --headless`) instead, which issues a new certificate. The `g8e auth refresh` CLI subcommand wraps this endpoint.

**Logout Ownership Policy:**

`g8e auth logout` removes local CLI credential material (credentials JSON, CLI certificate, CLI key) but does **not** remove the shared OS Root CA. The Root CA is a shared trust anchor that may be used by other processes or users on the host. Removing it would break other enrolled CLIs or browser sessions on the same machine. An administrator must manually remove the Root CA if needed.

**Windows-Specific Behavior:**

On Windows, the signed CLI certificate is imported into the Windows Certificate Store for Windows Hello native API access. CLI keys are file-backed ECDSA P-256 on all platforms. This is separate from browser-based WebAuthn passkeys.

#### Headless CLI Enrollment (mTLS-Only)

The `--headless` flag on `g8e auth enroll user` opts into a CLI-only identity that completes enrollment without a browser. It is the user-facing counterpart to the internal `SkipPasskey` field: `--headless` sets `SkipPasskey: true` and additionally switches the recovery branch from the browser-approval path to the mTLS-approval path. Internal callers (`mcp agent run`, `demos`) set `SkipPasskey` directly and must NOT set `Headless`, because Headless also changes recovery output, which those callers do not want.

**What `--headless` does:**

- Skips the passkey ceremony (no WebAuthn registration).
- Skips OS trust installation (no browser means no WebAuthn TLS trust is needed; the `--no-system-trust` behavior is implied). The CLI mTLS trust bundle is still installed from the recovery artifacts.
- On the recovery branch (partial/corrupt local identity on a bootstrapped gateway), prints `g8e auth approve-recovery <token>` for an already-enrolled CLI to run instead of opening a browser, then continues polling `recovery/status` and completing recovery exactly as the browser path does.
- On the bootstrap branch (unbootstrapped gateway), enrollment proceeds over plain HTTP with no approval needed and no passkey ceremony, producing a CLI-only identity.

**The resulting identity is mTLS-only.** It can do everything the CLI could do before (MCP, A2A, governance, SSE, rotation) because mTLS is the CLI's primary auth factor. It cannot authenticate to the Console SPA because no browser passkey was registered. The user can register a browser passkey later from a browser if they want console access.

**mTLS recovery approval endpoint:**

The headless path adds a parallel recovery-approve endpoint that derives the approver identity from the verified mTLS CLI certificate instead of a browser cookie:

- `POST /api/v1/auth/cli/recovery/approve-cli` - classified `RouteAuthMTLS`. The unified auth middleware (`handleMTLSAuth` → `handleCLIAuth`) verifies the CLI certificate URI SAN via `wid.MatchesCLI`, confirms the session is active, and stamps `ContextKeyUserID` / `ContextKeyCLISessionID` into the request context. The handler reads the approver user ID from the context (never from request body fields), verifies the user is active via `userSvc.GetByID` + `IsActive()`, and calls the same `recoverySvc.Approve(token, userID)` / `recoverySvc.Deny(token, userID)` transition as the browser path. The atomic state machine is unchanged.
- The browser recovery-approve path (`POST /api/v1/auth/cli/recovery/approve`, `RouteAuthWebSession`) is unchanged. Users with a browser and a passkey still approve through the Console SPA.

**`g8e auth approve-recovery <token>` CLI subcommand:**

The CLI-side counterpart to the browser Console SPA approve action. It uses the local enrolled CLI identity (mTLS) to approve or deny a pending recovery request created by another CLI's `auth enroll user --headless` run. It posts to the mTLS approve-cli endpoint with `Approve: true` (default) or `Approve: false` (`--deny`), then prints the resulting state.

**Unchanged security properties:**

- The recovery token remains one-time-use, opaque, and TTL-bounded. The mTLS approve endpoint consumes the same `recoverySvc.Approve(token, userID)` transition as the browser path.
- Proof-of-possession on `recovery/complete` is unchanged: the new CLI must still sign the request ID with its CSR private key. Stealing the token alone is not sufficient to complete recovery.
- The approver must be an active user with a valid, non-revoked CLI certificate. A revoked cert is rejected by the middleware's `VerifyCertificate` (401, `ErrMTLSCertRevoked`) before the handler runs; a deactivated user is rejected by the handler's active-user check (403).
- Fail-closed: the mTLS approve endpoint is `RouteAuthMTLS`. Unknown routes default to `RouteAuthMTLS`. No auth-bypass path is introduced.

This is distinct from the operator-side platform enrollment in §1.5 (which covers owner-approved platform enrollment for operator, dashboard, and ensemble components); the two are cross-linked here but address different identity surfaces.

### 1.2 Browser Authentication (Passkeys)

The Console uses WebAuthn passkeys for browser authentication. Passkeys provide hardware-bound authentication (such as YubiKey, Windows Hello, Touch ID).

**How It Works:**

1. Navigate to the Console at `https://<gateway-ip>:8443/console/`.
2. Register a passkey during first-time setup.
3. Use your passkey to authenticate on subsequent visits.
4. The Console issues a session cookie (`g8e_web_session_cookie`) for authenticated sessions, valid for 24 hours.

**Passkey Management:**

Once authenticated, you can view your registered passkeys, revoke passkeys, and manage your web session.

### 1.3 Agent Authentication

When you run `g8e mcp agent run`, the agent receives a short-lived delegated credential (1-hour certificate) that binds both the agent's identity and your human identity (the requestor). This ensures every agent action is traceable to a human operator.

### 1.4 External Identity Providers (JWT)

The platform supports authentication via external Identity Providers (IdPs) for bring-your-own clients:

- JWT tokens are validated against configured JWKS endpoints.
- Users are provisioned Just-In-Time (JIT) on first successful authentication.
- JWT roles are mapped to internal authorization personas.
- JIT-provisioned users can register a WebAuthn passkey via dedicated bootstrap endpoints, allowing them to transition from JWT-only auth to browser-based passkey auth.

### 1.5 Operator Authentication

Operators (remote execution containers) authenticate via mTLS certificates. The operator does not self-enroll. Starting the gateway with zero users issues no operator certificate. The owner must explicitly approve the operator's platform enrollment request before the operator receives credentials.

**Owner-Approved Platform Enrollment:**

The operator enrollment process requires an explicit owner decision at every step:

1. The gateway starts with zero users. No operator certificate is issued at startup.
2. The owner enrolls as the first user via `g8e auth enroll user -e <gateway>`, creating the first user identity and CLI mTLS credentials.
3. The operator submits a platform enrollment request (operator + CLI CSRs, system fingerprint) to the gateway's platform enrollment request endpoint.
4. The owner inspects pending requests via the Console SPA or `g8e auth pending-platform-enrollments` and approves the operator's request by request ID via `g8e auth approve-platform-enrollment <request-id> --yes`.
5. The operator polls its request status, signs the canonical completion transcript with both private keys (operator + CLI proof-of-possession), and submits completion.
6. The gateway validates the proofs, signs an operator certificate with the session ID embedded in the identity, writes credentials to the platform PKI directory (`.g8e/pki/`), and returns them to the operator.
7. The operator atomically writes its cert, key, trust bundle, and actuator public key, then removes its pending state.

The requester token is hashed at rest and absent from logs, URLs, pending lists, approval commands, and audit records. Approval and pending discovery use request IDs over authenticated HTTPS. The completion is idempotent after success: a lost completion response returns the same artifacts on retry without issuing a second identity. The operator persists pending state (private keys, requester token, request ID, CSR fingerprints) to `pki/pending-enrollment/g8eo.json` with 0600 permissions, so a restart during pending enrollment resumes the same request and key material rather than generating a new request.

The same owner-approved platform enrollment protocol applies to the dashboard and ensemble components. Each submits its own platform enrollment request and waits for owner approval before its service becomes ready. The recommended approval order is operator, dashboard, then ensemble — this is an operational recommendation, not a security invariant. The gateway does not enforce prerequisite state between component approvals.

**Headless Gateway Deployment:**

For headless deployments that only need the gateway without platform workloads, use explicit gateway service selection:

```bash
docker compose up -d --no-deps g8e-gateway
./g8e auth enroll user --headless -e <gateway>
```

This starts only the gateway while preserving full-stack default behavior. No platform enrollment requests are submitted because no workloads are running.

**Cert Sharing:**

In demo environments, agent containers can share the operator's credentials via a read-only volume mount to avoid separate enrollment.

## 2. Authorization

Authorization determines what actions you're allowed to perform. The platform uses a 5-layer verification sequence.

### 2.1 The 5-Layer Verification Pipeline

Every action must pass through these layers before execution:

| Layer | Purpose | What It Checks |
|-------|---------|----------------|
| **L1: Doctrine** | Technical safety | Forbidden patterns, MITRE threats, critical system file protection |
| **L2: Consensus** | Multi-agent approval | Cryptographic attestation from consensus members |
| **L3: Notary** | Human authorization | Passkey or signed CLI proofs |
| **L4: Warden** | Final verification | Replay protection, state validation, posture enforcement |
| **L5: Actuator** | Execution | Dispatches action with audit commitment |

### 2.2 Layer 1: Doctrine (Technical Safety)

Doctrine enforces deterministic security rules:

- **Forbidden Patterns**: Rejects strings matching dangerous regex patterns.
- **MITRE Threat Detection**: Analyzes payloads against 16 threat categories (reverse shells, privilege escalation, credential access, etc.).
- **Critical System Protection**: Blocks modifications to critical system paths and directories.

Violations at this layer cannot be bypassed by higher layers.

### 2.3 Layer 2: Consensus (Multi-Agent Approval)

Consensus provides cryptographic attestation from multiple consensus members:

- Each consensus member independently evaluates the payload.
- Members sign their decision with their Ed25519 private key.
- The Warden verifies signatures and counts affirmative votes against a quorum threshold.
- Duplicate signers are rejected to prevent single-member vote stuffing.

### 2.4 Layer 3: Notary (Human Authorization)

Notary ensures explicit human authorization for sensitive actions.

**Browser-Based Approval Flow:**

1. CLI suspends the transaction requiring approval.
2. CLI opens browser to approval page with transaction hash.
3. User authenticates with passkey (must complete within 2 minutes).
4. Browser performs WebAuthn ceremony.
5. CLI waits for approval via SSE stream.
6. Once approved, transaction proceeds.

**Authorization Factors:**

- **Factor 1**: Passkey authorization (always required).
- **Factor 2**: mTLS transport authentication (for CLI callers).

**Approval Windows:**

- **Request window**: 2 minutes to complete the passkey ceremony after the transaction is suspended. If the passkey approval is not completed within this window, the request expires and the action must be retried.
- **Dispatch window**: 30 minutes after approval to dispatch the transaction. Transactions not dispatched within that window must be re-approved.

The gateway always requires real WebAuthn proof. The `G8E_L3_MOCK` environment variable has been removed; there is no bypass for L3 notary verification.

### 2.5 Layer 4: Warden (Final Verification)

The Warden is the final fail-closed gate before execution:

1. **In-Flight Tracking**: Prevents concurrent processing of duplicate transactions.
2. **Nonce Reservation**: Early replay protection via database.
3. **Stateless Validation**: Structural integrity, action type recognition, hash verification.
4. **Stateful Validation**: Merkle root consistency check.
5. **Posture Validation**: Enforces L2 and L3 based on configured posture.

### 2.6 Layer 5: Actuator (Execution)

The Actuator represents the execution boundary:

- Issues signed action receipts for audit.
- Rehydrates sensitive data just before execution.
- Mints scoped, single-action capabilities.
- Dispatches to downstream executors (Command, File Edit, MCP, and A2A).
- Records transaction in audit store.

## 3. Governance Postures

Postures define which verification layers are enforced. When a layer is "Audited," verification runs but failures don't block execution. When "Enforced," failures abort the transaction.

| Posture | L1 (Doctrine) | L2 (Consensus) | L3 (Notary) | Use Case |
|---------|--------------|----------------|-------------|----------|
| **Doctrine** | Enforced | Audited | Audited | Local Development / CI (Default) |
| **Consensus** | Enforced | Enforced | Audited | Automated Workflows |
| **Notary** | Enforced | Enforced | Enforced | Production |

## 4. Session & Identity Binding

Binding links platform sessions (web, CLI, operator) to the identities they authorize.

### 4.1 Web to Operator Binding

When you use the Console, you can control multiple Operators on different hosts. Session binding records which Operators are associated with your web session.

**What Binding Enables:**

- SSE events can be pushed to your web session, routed by `user_id` plus `web_session_id`.
- You can set an active target Operator.
- Commands can be dispatched from web to Operator.

### 4.2 CLI Cert Binding

CLI certificates are linked to CLI sessions, creating a chain from CLI mTLS certificate to CLI session to Operator session to Operator. This ensures CLI commands are scoped to the correct Operator.

### 4.3 JWT Persona Binding

When using external IdP authentication, JWT roles are mapped to internal personas for authorization context.

### 4.4 App Credential Binding

Agent credentials bind both the app identity and the human requestor identity, ensuring every agent action is traceable to a human.

## 5. Encryption at Rest

All sensitive data is encrypted at rest using AES-256-GCM:

- **Audit logs**: Encrypts command output and file mutations.
- **Execution results**: Encrypts stdout, stderr, and file diffs.
- **Scrubbing tokens**: Encrypts placeholders used to hide sensitive data.

**Vault Management:**

Vault operations are managed via CLI commands:

- `g8e vault init` - Initialize new vault.
- `g8e vault unlock` - Unlock vault with key.
- `g8e vault rekey` - Rotate vault keys.
- `g8e vault status` - Check vault status.
- `g8e vault export` - Export vault key.
- `g8e vault import` - Import vault key.
- `g8e vault reset` - Destroy vault (destructive).

**Configuration:**

Vault paths can be configured via:
- Gateway CLI flags: `--vault-dir`, `--vault-key`.
- Vault subcommand flags: `--vault-dir`, `--key-path`.
- Environment variables: `G8E_VAULT_DIR`, `G8E_VAULT_KEY`.
- Default paths: `.g8e/vault/` for vault data, `.g8e/secrets/key` for the vault key.

The vault is required. On first run, the gateway auto-initializes a new vault with a generated key. On subsequent starts, the vault key must be valid or the gateway fails to start.

## 6. Practical Authentication Flows

### 6.1 First-Time Setup

1. Start the Gateway.
2. Run `g8e auth enroll user` to enroll your CLI.
   - For default ports: `g8e auth enroll user`.
   - For Docker demos with split ports: `g8e auth enroll user -e localhost:<httpPort> --port <httpsPort>`.
   - The CLI installs the gateway Root CA into the OS trust store automatically before opening the browser.
   - If trust installation fails, the browser does not open - resolve the trust issue and re-run.
   - Use `--no-system-trust` only if an administrator has already installed the Root CA.
3. The CLI blocks and prompts you to close all open browser windows, then press Enter (so the browser recognizes the newly installed Root CA when it reopens).
4. Complete the passkey ceremony in the browser.
5. Your CLI identity is now enrolled and ready for use.

### 6.1.1 Recovering a CLI

If your local CLI credentials are partial, corrupt, or the local trust bundle is stale against the live gateway (for example files were accidentally deleted, cert expired, or the gateway PKI was regenerated via `gw clean`):

1. Run `g8e auth enroll user` - the CLI detects the partial/corrupt/stale-bundle state.
2. The CLI initiates the recovery flow and opens the browser.
3. An authenticated user approves the recovery in the Console SPA.
4. The CLI receives a new CLI certificate and commits it atomically.

### 6.1.2 Rotating a CLI Certificate

To manually rotate your CLI certificate (for example before expiry):

1. Run `g8e auth enroll user --rotate-cli`.
2. The CLI uses your existing mTLS certificate to request rotation.
3. A replacement certificate is issued and the old one is revoked.
4. Only one replacement is issued per run.

### 6.2 Daily Usage

**CLI:**
- Your enrolled certificate handles authentication automatically.
- Run `g8e auth enroll user` again - if your identity is complete and valid, and the local trust bundle matches the live gateway root CA, it is reused without rotation.
- Run `g8e auth enroll user --rotate-cli` to force certificate rotation.
- If credentials are partial or corrupt, or the local trust bundle is stale against the live gateway (for example after `gw clean`), the CLI automatically initiates the recovery flow.

**Browser:**
- Navigate to `https://<gateway-ip>:8443/console/`.
- Authenticate with your registered passkey.
- Session cookie maintains your authentication.

**Agents:**
- Run `g8e mcp agent run`.
- Agent receives delegated credential automatically.
- Agent actions are bound to your identity.

### 6.3 Approval Flow

When an action requires human approval:

1. CLI suspends the transaction.
2. CLI opens browser to approval page.
3. Authenticate with passkey.
4. Approve or reject the transaction.
5. CLI receives decision via SSE stream, routed by the CLI session.
6. Transaction proceeds if approved.

## 7. Security Properties

**Zero-Trust Networking:**
- All communication authenticated via mTLS.
- No shared secrets or API keys.
- Identity proven via cryptographic signatures.
- TLS key agreement restricted to FIPS-validated curves: X25519MLKEM768 (FIPS 203 hybrid), P-384, P-256. X25519 is excluded (not SP 800-56A rev3 compliant). See [FIPS 140-3 Compliance](../reference/fips140-3.md) for details.

**Fail-Closed Design:**
- Unknown routes default to strictest auth mode.
- Services fail to initialize without vault.
- Verification failures abort transactions.

**Defense in Depth:**
- Multiple independent verification layers.
- Separation between verification (L4) and execution (L5).
- Binding ensures identity chain integrity.

**Auditability:**
- Every action carries full identity chain.
- Signed action receipts provide immutable proof.
- All sensitive data encrypted at rest.

## 8. Network Security Foundation

The authentication architecture is built on a zero-trust networking model where all communication is authenticated via mutual TLS (mTLS) with verified SPIFFE workload identities.

For detailed information on PKI hierarchy, certificate management, workload identity formats, mTLS enforcement, and port topology, see [Network Architecture](./network.md).
