# Authentication & Authorization

Last Updated: 2026-07-15
Version: v1.5.2

This document explains how to authenticate and authorize actions in the g8e platform. The platform is built as a zero-trust execution environment where every action is verified before execution.

## Overview

The platform security model is built on two core principles:

1. **Identity-Bound Communication (mTLS)**: Every connection must be authenticated via mutual TLS (mTLS) with a verified identity. This applies to CLI, Console, AI Agent, and Operator connections.

2. **5-Layer Verification Sequence**: Every action (command execution, file edit, tool call) must pass through a sequential 5-layer verification pipeline before execution.

---

## 1. Authentication

Authentication is how you prove your identity to the platform. The g8e platform uses multiple authentication methods depending on your use case.

### 1.1 CLI Authentication

The CLI uses mTLS certificates for authentication. When you run `g8e auth enroll`, the CLI generates its own key pair and asks the Gateway to sign a certificate attesting that "this public key belongs to this identity."

**Key Concepts:**
- No shared secrets or API keys to leak
- You prove your identity by signing with your private key on every call
- The Gateway acts as the Certificate Authority (CA)

**Enrollment Process:**

There are three enrollment scenarios:

| Scenario | When It Happens | How It Works |
|----------|----------------|--------------|
| **First-time setup** | Gateway never bootstrapped | CLI connects over plain HTTP to the gateway HTTP port, Gateway bootstraps itself |
| **New CLI on existing gateway** | Gateway exists, no local credentials | CLI connects over plain HTTP, Gateway enrolls CLI |
| **Re-enrollment** | Credentials exist, need rotation | CLI uses existing mTLS cert to request new cert |

**Two-Phase Enrollment & Split Endpoint Flags:**

Enrollment involves two phases that use different ports and protocols:

1. **Discovery/bootstrap phase** (plain HTTP) — CA bundle fetch, bootstrap status check, CSR trust bundle retrieval
2. **mTLS API phase** (HTTPS) — Enrollment token generation, CSR submission, SSE stream, API client operations

By default, the CLI connects to `g8e.local` (or the machine IP fallback) on the default ports (HTTP 8080, HTTPS 8443). When the gateway's HTTP and HTTPS ports are mapped to different host ports — as in Docker demos — use the split endpoint flags:

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

1. CLI generates a one-time enrollment token via the mTLS endpoint `/api/v1/auth/enrollment-token/generate`
2. CLI opens the browser with `#register=1&token=<token>` (no raw `user_id` or `cli_session_id` in the URL)
3. The Console SPA reads the token from the URL hash and POSTs it to the public endpoint `/api/v1/auth/enrollment-token/validate`
4. Gateway validates the token and returns the associated `user_id` and `cli_session_id`
5. SPA populates the hidden form fields and calls `registerPasskey()`
6. SPA immediately clears the token from the URL via `history.replaceState`
7. Token is one-time-use with a 5-minute TTL
8. Expired tokens are cleaned up periodically by the gateway

This ensures that sensitive session identifiers are never exposed in browser history or referrer headers, while maintaining the security of the enrollment flow.

**Trusting the Gateway Certificate:**

Since the Gateway uses self-signed certificates, you must trust the platform's Root CA before browser-based operations work:

- **Linux/macOS**: `curl -fsSL http://<gateway-ip>:8080/web-cert.sh | sh`
- **Windows**: `irm http://<gateway-ip>:8080/web-cert.ps1 | iex`

**Important**: After running a trust script, restart all browsers for the CA to be recognized.

**Windows-Specific Behavior:**

On Windows, the CLI uses the Windows Certificate Store for key generation. The `--tpm` flag requests TPM-backed keys via Windows Hello for Business, though software-backed keys are currently used as a fallback. This is separate from browser-based WebAuthn passkeys.

### 1.2 Browser Authentication (Passkeys)

The g8e Console uses WebAuthn passkeys for browser authentication. Passkeys provide hardware-bound authentication (like YubiKey, Windows Hello, Touch ID).

**How It Works:**

1. Navigate to the Console at `https://<gateway-ip>:8443/console/`
2. Register a passkey during first-time setup
3. Use your passkey to authenticate on subsequent visits
4. The Console issues a session cookie (`g8e_web_session_cookie`) for authenticated sessions, valid for 24 hours

**Passkey Management:**

Once authenticated, you can:
- View your registered passkeys
- Revoke passkeys
- Manage your web session

### 1.3 Agent Authentication

When you run `g8e mcp agent run`, the agent receives a short-lived delegated credential (1-hour certificate) that binds both:
- The agent's identity (`spiffe://g8e.local/app/<agent-name>`)
- Your human identity (the requestor)

This ensures every agent action is traceable to a human operator.

### 1.4 External Identity Providers (JWT)

The platform supports authentication via external Identity Providers (IdPs) for bring-your-own clients:

- JWT tokens are validated against configured JWKS endpoints
- Users are provisioned Just-In-Time (JIT) on first successful authentication
- JWT roles are mapped to internal authorization personas
- JIT-provisioned users can register a WebAuthn passkey via dedicated bootstrap endpoints, allowing them to transition from JWT-only auth to browser-based passkey auth

### 1.5 Operator Authentication

Operators (remote execution containers) authenticate via mTLS certificates. The operator enrollment process:

1. Creates an operator document in the database
2. Persists an operator session
3. Signs a certificate with the session ID embedded in the identity
4. Writes credentials to the platform PKI directory (`.g8e/pki/`)

**Headless Enrollment:**

For automated deployments, operators can use the `/api/v1/auth/device/enroll` endpoint for headless enrollment without manual interaction.

**Cert Sharing:**

In demo environments, agent containers can share the operator's credentials via a read-only volume mount to avoid separate enrollment.

---

## 2. Authorization

Authorization determines what actions you're allowed to perform. The platform uses a 5-layer verification sequence.

### 2.1 The 5-Layer Verification Pipeline

Every action must pass through these layers before execution:

| Layer | Purpose | What It Checks |
|-------|---------|----------------|
| **L1: Doctrine** | Technical safety | Forbidden patterns, MITRE threats, critical system file protection |
| **L2: Consensus** | Multi-agent approval | Cryptographic attestation from tribunal members |
| **L3: Notary** | Human authorization | Passkey or signed CLI proofs |
| **L4: Warden** | Final verification | Replay protection, state validation, posture enforcement |
| **L5: Actuator** | Execution | Dispatches action with audit commitment |

### 2.2 Layer 1: Doctrine (Technical Safety)

Doctrine enforces deterministic security rules:

- **Forbidden Patterns**: Rejects strings matching dangerous regex patterns
- **MITRE Threat Detection**: Analyzes payloads against 16 threat categories (reverse shells, privilege escalation, credential access, etc.)
- **Critical System Protection**: Blocks modifications to critical system paths and directories

Violations at this layer cannot be bypassed by higher layers.

### 2.3 Layer 2: Consensus (Multi-Agent Approval)

Consensus provides cryptographic attestation from multiple tribunal members:

- Each tribunal member independently evaluates the payload
- Members sign their decision with their Ed25519 private key
- The Warden verifies signatures and counts affirmative votes against a quorum threshold
- Duplicate signers are rejected to prevent single-member vote stuffing

### 2.4 Layer 3: Notary (Human Authorization)

Notary ensures explicit human authorization for sensitive actions:

**Browser-Based Approval Flow:**

1. CLI suspends the transaction requiring approval
2. CLI opens browser to approval page with transaction hash
3. User authenticates with passkey (must complete within 2 minutes)
4. Browser performs WebAuthn ceremony
5. CLI waits for approval via SSE stream
6. Once approved, transaction proceeds

**Authorization Factors:**

- **Factor 1**: Passkey authorization (always required)
- **Factor 2**: mTLS transport authentication (for CLI callers)

**Approval Windows:**

- **Request window**: 2 minutes to complete the passkey ceremony after the transaction is suspended. If the passkey approval is not completed within this window, the request expires and the action must be retried.
- **Dispatch window**: 30 minutes after approval to dispatch the transaction. Transactions not dispatched within that window must be re-approved.

### 2.5 Layer 4: Warden (Final Verification)

The Warden is the final fail-closed gate before execution:

1. **In-Flight Tracking**: Prevents concurrent processing of duplicate transactions
2. **Nonce Reservation**: Early replay protection via database
3. **Stateless Validation**: Structural integrity, action type recognition, hash verification
4. **Stateful Validation**: Merkle root consistency check
5. **Posture Validation**: Enforces L2 and L3 based on configured posture

### 2.6 Layer 5: Actuator (Execution)

The Actuator represents the execution boundary:

- Issues signed action receipts for audit
- Rehydrates sensitive data just before execution
- Mints scoped, single-action capabilities
- Dispatches to downstream executors (Shell, MCP, A2A)
- Records transaction in audit store

---

## 3. Governance Postures

Postures define which verification layers are enforced. When a layer is "Audited," verification runs but failures don't block execution. When "Enforced," failures abort the transaction.

| Posture | L1 (Doctrine) | L2 (Consensus) | L3 (Notary) | Use Case |
|---------|--------------|----------------|-------------|----------|
| **Doctrine** | Enforced | Audited | Audited | Local Development / CI (Default) |
| **Consensus** | Enforced | Enforced | Audited | Automated Workflows |
| **Notary** | Enforced | Enforced | Enforced | Production |

---

## 4. Session & Identity Binding

Binding links platform sessions (web, CLI, operator) to the identities they authorize.

### 4.1 Web ↔ Operator Binding

When you use the Console, you can control multiple Operators on different hosts. Session binding records which Operators are associated with your web session.

**What Binding Enables:**

- SSE events can be pushed to your web session
- You can set an active target Operator
- Commands can be dispatched from web to Operator

### 4.2 CLI Cert Binding

CLI certificates are linked to Operator sessions, creating a chain:

```
CLI mTLS cert → CLI session → Operator session → Operator
```

This ensures CLI commands are scoped to the correct Operator.

### 4.3 JWT Persona Binding

When using external IdP authentication, JWT roles are mapped to internal personas for authorization context.

### 4.4 App Credential Binding

Agent credentials bind both the app identity and the human requestor identity, ensuring every agent action is traceable to a human.

---

## 5. Encryption at Rest

All sensitive data is encrypted at rest using AES-256-GCM:

- **Audit logs**: Encrypts command output and file mutations
- **Execution results**: Encrypts stdout, stderr, and file diffs
- **Scrubbing tokens**: Encrypts placeholders used to hide sensitive data

**Vault Management:**

Vault operations are managed via CLI commands:

- `./g8e vault init` - Initialize new vault
- `./g8e vault unlock` - Unlock vault with key
- `./g8e vault rekey` - Rotate vault keys
- `./g8e vault status` - Check vault status
- `./g8e vault export` - Export vault key
- `./g8e vault import` - Import vault key
- `./g8e vault reset` - Destroy vault (destructive)

**Configuration:**

Vault paths can be configured via:
- CLI flags: `--vault-dir`, `--vault-key`, `--vault-require-unlock`
- Environment variables: `G8E_VAULT_DIR`, `G8E_VAULT_KEY`, `G8E_VAULT_REQUIRE_UNLOCK`
- Default paths: `.g8e/vault/` for vault data, `.g8e/secrets/key` for the vault key

When `--vault-require-unlock` is set, the gateway fails to start if the vault cannot be unlocked with the provided key. This enforces mandatory encryption at rest for production deployments.

---

## 6. Practical Authentication Flows

### 6.1 First-Time Setup

1. Start the Gateway
2. Run `g8e auth enroll` to enroll your CLI
   - For default ports: `g8e auth enroll`
   - For Docker demos with split ports: `g8e auth enroll -e localhost:<httpPort> --port <httpsPort>`
3. Trust the Gateway CA (run the appropriate script for your OS)
4. Restart your browser
5. Navigate to the Console and register a passkey

### 6.2 Daily Usage

**CLI:**
- Your enrolled certificate handles authentication automatically
- Run `g8e auth enroll` again if your certificate expires

**Browser:**
- Navigate to `https://<gateway-ip>:8443/console/`
- Authenticate with your registered passkey
- Session cookie maintains your authentication

**Agents:**
- Run `g8e mcp agent run`
- Agent receives delegated credential automatically
- Agent actions are bound to your identity

### 6.3 Approval Flow

When an action requires human approval:

1. CLI suspends the transaction
2. CLI opens browser to approval page
3. Authenticate with passkey
4. Approve or reject the transaction
5. CLI receives decision via SSE stream
6. Transaction proceeds if approved

---

## 7. Security Properties

**Zero-Trust Networking:**
- All communication authenticated via mTLS
- No shared secrets or API keys
- Identity proven via cryptographic signatures

**Fail-Closed Design:**
- Unknown routes default to strictest auth mode
- Services fail to initialize without vault
- Verification failures abort transactions

**Defense in Depth:**
- Multiple independent verification layers
- Separation between verification (L4) and execution (L5)
- Binding ensures identity chain integrity

**Auditability:**
- Every action carries full identity chain
- Signed action receipts provide immutable proof
- All sensitive data encrypted at rest

---

## 8. Network Security Foundation

The authentication architecture is built on a zero-trust networking model where all communication is authenticated via mutual TLS (mTLS) with verified SPIFFE workload identities.

For detailed information on PKI hierarchy, certificate management, workload identity formats, mTLS enforcement, and port topology, see [Network Architecture](./network.md).
