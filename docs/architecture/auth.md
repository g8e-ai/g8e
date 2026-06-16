# Authentication & Authorization

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

**The mental model:** CSR-based enrollment is cryptographic identity proof. Instead of sharing a secret (like an API key), a client generates its own key pair and asks the Gateway to sign a certificate attesting "this public key belongs to this identity." The Gateway acts as a Certificate Authority (CA). The act of starting the Gateway is itself the Platform Owner's authorization — there are no standing invite codes, pre-shared keys, or manual approval steps. The Platform Owner's intent to give AI governed access to the physical world is expressed by running the Gateway; the Gateway's willingness to sign CSRs flows from that decision. The client then proves its identity on every subsequent call by signing with its private key (via mTLS). No shared secrets, no API keys to leak.

#### CLI Enrollment Paths

There are three distinct CLI enrollment paths depending on gateway and credential state. The `EnrollCLI` function (`internal/cli/auth/agent_enroll.go`) encapsulates the first two paths as a reusable, idempotent call used by both `g8e auth enroll` and the agent launcher.

| Path | Trigger | Transport | Function |
| :--- | :--- | :--- | :--- |
| **First-time bootstrap** | Gateway never bootstrapped | Plain HTTP (discovery port) | `Bootstrap()` |
| **New CLI, existing gateway** | Gateway bootstrapped, no local credentials | Plain HTTP (discovery port) | `CLIEnroll()` |
| **Re-enrollment** | Credentials present, certificate rotation | mTLS (HTTPS port) | `ReEnroll()` or `CLIEnroll()` |

Plain HTTP is used only for the bootstrap and CLI enrollment paths because the CLI has no mTLS certificate yet — these endpoints exist on the unauthenticated discovery port and are gated by the gateway's own authorization policy. All subsequent communication uses mTLS exclusively.

**Trusting the Self-Signed CA**: Since the g8e Gateway uses self-signed certificates for its internal PKI, non-Windows clients must trust the platform's Root CA before browser-based passkey registration can succeed. The platform provides automated trust scripts for this purpose:
- **Linux**: `curl -fsSL http://<gateway-ip>:8080/bootstrap-ca | sh`
- **macOS**: `curl -fsSL http://<gateway-ip>:8080/bootstrap-ca-macos | sh`
- **Windows**: `iwr http://<gateway-ip>:8080/bootstrap-ca.ps1 -UseBasicParsing | iex`

**CRITICAL**: After running a trust script, the user **MUST restart all open browsers** for the newly installed CA to be recognized. Failure to do so will result in WebAuthn registration errors in the browser.

**`EnrollCLI(cfg)`** selects between `Bootstrap` and `CLIEnroll` based on `CheckBootstrapStatus`, then saves the signed certificate, trust bundle, and credential file. Callers (`g8e auth enroll`, `g8e mcp agent run`) add their own user-facing output; `EnrollCLI` itself is silent.

**Re-enrollment** (`g8e auth enroll` when credentials already exist) uses `ReEnroll` over mTLS when an operator certificate is present, or falls back to `CLIEnroll` for CLI-only deployments. It also runs `AutoRenewCertificate` to short-circuit if the existing certificate is still valid.

#### Agent App Enrollment (Delegated Credentials)

When `g8e mcp agent run` launches an AI agent, it calls `EnrollAgentApp` to issue the agent a short-lived delegated credential (1-hour certificate). This certificate carries:
- A SPIFFE URI SAN identifying the agent: `spiffe://g8e.local/app/<agent-name>`
- The requestor's human identity (bound at issuance time on the gateway)

The request is made over mTLS using the CLI certificate and requires a valid `X-CLI-Session-ID` header. The gateway's `/api/v1/pki/apps/delegated` endpoint is on the mTLS-only router. The resulting credential is injected into the agent subprocess via `G8E_APP_CERT` / `G8E_APP_KEY` environment variables and is idempotent — an existing certificate with more than 7 days remaining and the correct SPIFFE URI SAN is reused without contacting the gateway.

`EnrollAgentApp` also verifies that the authenticated user has a registered passkey (`VerifyPasskeyRegistration`, mTLS) before proceeding, enforcing that agent sessions are always backed by a human with hardware-bound authentication.

#### Windows Enrollment

On Windows, enrollment uses the Windows Certificate Store with TPM-backed keys via Windows Hello for Business. `g8e auth enroll` detects the platform and delegates to the Windows-specific path automatically. See [Network Architecture](./network.md) for details.

### External IdP Support (JWT)

The platform supports authentication via external Identity Providers (IdPs) for BYO clients on MCP and A2A endpoints.
- **JWKS Integration**: The gateway validates JWT tokens against configured JWKS endpoints.
- **JIT Provisioning**: Users are provisioned Just-In-Time (JIT) based on the JWT subject claim upon their first successful authentication, subject to platform owner authorization.
- **Persona Mapping**: JWT roles are mapped to internal binding personas via the `PersonaService` defined in @../../internal/services/gateway/gateway_auth.go:866.

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

The platform implements a deterministic 5-layer governance sequence. Every mutation must pass through all active layers before execution. The structural schema is defined as `GovernanceEnvelope` in @../../protocol/proto/g8e/common/v1/common.proto:79.

### Layer 1: Technical Bedrock (L1Doctrine)
*Implementation: @../../internal/services/governance/l1_doctrine.go:50*

L1 is the foundational layer that executes deterministic security rules.
- **Forbidden Patterns**: Uses Protobuf field options (`forbidden_patterns`) to reject strings matching dangerous regex patterns on typed payload fields.
- **MITRE Threat Detection**: Analyzes payloads against MITRE ATT&CK patterns across 16 threat categories, including reverse shells, privilege escalation, credential access, data destruction, defense evasion, and cryptominer deployment. Analysis applies to `CommandRequested`, `McpCallRequested`, `A2ACallRequested`, and `FileEditRequested` payloads. MCP and A2A argument JSON is traversed recursively.
- **Critical System File Protection**: Blocks modifications to critical system paths defined in `CriticalSystemPaths` and critical directories defined in `CriticalSystemDirs`.
- **Hard Gates**: Rejects transactions immediately upon violation; cannot be bypassed by L2 or L3.

### Layer 2: Consensus (L2Consensus)
*Implementation: @../../internal/services/governance/l2_consensus.go:45*

L2 provides multi-agent cryptographic attestation of payload safety.
- **Payload Hash Verification**: Verifies that `envelope.Id` matches the computed message hash before signing, ensuring the envelope has not been tampered with in transit.
- **MITRE Safety Evaluation**: Runs L1Doctrine threat detection on extracted command data and intent fields to produce a binary safety decision.
- **Ed25519 Decision Signing**: Signs the string `{message_id}|{decision}` using the node's Ed25519 private key and writes the hex-encoded signature to `GovernanceMetadata.L2.consensus_signature` along with the signer's key ID and agent ID.
- **Fail-Closed on Missing Doctrine**: If the L1Doctrine reference is absent, L2 evaluates all payloads as unsafe and refuses to sign.

### Layer 3: Notary (L3Notary)
*Implementation: @../../internal/services/governance/l3_notary.go:32*

L3 ensures explicit human authorization for mutations.
- **Suspension**: The g8e Gateway suspends transactions requiring L3 approval, storing them in the `suspended_transactions` pool.
- **Out-of-Band (OOB) Approval**: The user approves via CLI command (`g8e approve <tx_hash>`) with a cryptographic Ed25519 signature over the transaction hash, or via WebAuthn passkey for web sessions.
- **Approval Window**: CLI-based approvals are valid for 30 minutes from the time of approval. Transactions not dispatched within that window are rejected and must be re-approved.
- **Cryptographic Binding**: The CLI proof requires a hex-encoded Ed25519 signature of exactly 64 bytes (`cli_signature`) and, when configured, an mTLS certificate fingerprint (`mtls_cert_fingerprint`) that must match the fingerprint recorded at suspension time.
- **Passkey Service**: The `PasskeyService` handles L3 proof brokerage for WebAuthn operations, moving L3 authorization into the gateway as the sovereign authority.
- **L3Proof**: A successful approval generates an `L3Proof` (defined in @../../protocol/proto/g8e/common/v1/common.proto:52) containing the cryptographic signature and certificate fingerprint, cryptographically bound to the `transaction_hash`.

### Layer 4: Warden (L4Warden)
*Implementation: @../../internal/services/governance/l4_warden.go:393*

The Warden is the final fail-closed gate before execution. It verifies in the following order:
1. **In-Flight Tracking**: Prevents concurrent processing of transactions with the same nonce via an in-memory `sync.Map` guard.
2. **Nonce Reservation**: Early durable replay protection via `ReplayStore.ReserveNonce`, committed to SQLite before any expensive cryptographic checks.
3. **Stateless Validation**: Structural integrity checks, action type recognition, typed payload decoding, L1Doctrine compliance, and transaction hash verification (both `id` and `transaction_hash` must equal the computed hash).
4. **Stateful Validation**: State Merkle root consistency check via `StateRootProvider`; rejects the envelope if the provided `state_merkle_root` does not match the current root.
5. **Posture Validation**: L2 and L3 enforcement based on the configured `GovernancePosture` (Doctrine, Consensus, or Notary). L2 signature verification resolves the signer's Ed25519 public key from `SignerStore` by `key_id` and verifies the signature over `{transaction_hash}|{decision}`. L3 proof verification delegates to the configured `L3Notary` implementation.

### Layer 5: Actuator (L5Actuator)
*Implementation: @../../internal/services/governance/l5_actuator.go:76*

The Actuator represents the execution boundary and final audit commitment.
- **Fail-Closed Pre-Execution**: Receipt signing and initial audit logging must both succeed before the execution handler is invoked. If either fails, the transaction is aborted.
- **Sensitive Data Rehydration**: Rehydrates scrubbed placeholders (such as `{{UEI_1}}`) with original sensitive data just before execution via `ScrubbingService.RehydratePayload`.
- **Egress Dispatch**: Dispatches the verified payload to downstream executors (Shell, MCP, A2A) via `ExecutionHandler.ExecuteVerifiedTransaction`.
- **Action Receipts**: Issues a signed `ActionReceipt` using the Actuator's own Ed25519 key over a canonical JSON serialization of the receipt fields, providing immutable proof of the execution outcome.
- **Commitment**: Records the transaction in the `SQLAuditStore` and, where configured, in the console audit store.

---

## 4. Governance Postures

Postures define which layers of the bedrock are enforced as fail-closed gates.

| Posture | L1 (Doctrine) | L2 (Consensus) | L3 (Notary) | Use Case |
| :--- | :---: | :---: | :---: | :--- |
| **Doctrine** | Required | Optional | Optional | Local Dev / CI |
| **Consensus** | Required | Required | Optional | Automated Workflows |
| **Notary** | Required | Required | Required | **Production (Default)** |

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
- **SQLAuditStore**: Encrypts audit records, governance envelopes, audit trail, and compliance records
- **ExecutionVaultService**: Encrypts execution results (stdout, stderr) and file diffs
- **TokenStoreService**: Encrypts authentication tokens and session data

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
- Configuration file: `paths_default.json`
