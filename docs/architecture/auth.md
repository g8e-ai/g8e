# Authentication & Authorization

Last Updated: 2026-06-23
Version: v1.1.9

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
- **Windows**: `iwr http://<gateway-ip>:8080/bootstrap-ca.ps1 -UseBasicParsing | iex`

**CRITICAL**: After running a trust script, the user **MUST restart all open browsers** for the newly installed CA to be recognized. Failure to do so will result in WebAuthn registration errors in the browser.

**`EnrollCLI(cfg)`** selects between `Bootstrap` and `CLIEnroll` based on `CheckBootstrapStatus`, then saves the signed certificate, trust bundle, and credential file. Callers (`g8e auth enroll`, `g8e mcp agent run`) add their own user-facing output; `EnrollCLI` itself is silent.

**Re-enrollment** (`g8e auth enroll` when credentials already exist) uses `ReEnroll` over mTLS when an operator certificate is present, or falls back to `CLIEnroll` for CLI-only deployments. It also runs `AutoRenewCertificate` to short-circuit if the existing certificate is still valid.

#### Agent App Enrollment (Delegated Credentials)

When `g8e mcp agent run` launches an AI agent, it calls `EnrollAgentApp` (`internal/cli/auth/agent_enroll.go:79`) to issue the agent a short-lived delegated credential (1-hour certificate). This certificate carries:
- A SPIFFE URI SAN identifying the agent: `spiffe://g8e.local/app/<agent-name>`
- The requestor's human identity (bound at issuance time on the gateway)

The request is made over mTLS using the CLI certificate and requires a valid `X-CLI-Session-ID` header. The gateway's `/api/v1/pki/apps/delegated` endpoint is on the mTLS-only router. The resulting credential is injected into the agent subprocess via `G8E_APP_CERT` / `G8E_APP_KEY` environment variables and is idempotent: an existing certificate with more than 7 days remaining and the correct SPIFFE URI SAN is reused without contacting the gateway.

The agent launcher (`g8e mcp agent run` in `internal/cli/cmd/mcp.go`) calls `VerifyPasskeyRegistration` (mTLS) after `EnrollAgentApp` to verify that the authenticated user has a registered passkey, enforcing that agent sessions are always backed by a human with hardware-bound authentication.

#### Windows Enrollment

On Windows, enrollment uses the Windows Certificate Store with TPM-backed keys via Windows Hello for Business. `g8e auth enroll` detects the platform and delegates to the Windows-specific path automatically. See [Network Architecture](./network.md) for details.

### External IdP Support (JWT)

The platform supports authentication via external Identity Providers (IdPs) for BYO clients on MCP and A2A endpoints.
- **JWKS Integration**: The gateway validates JWT tokens against configured JWKS endpoints.
- **JIT Provisioning**: Users are provisioned Just-In-Time (JIT) based on the JWT subject claim upon their first successful authentication, subject to platform owner authorization.
- **Persona Mapping**: JWT roles are mapped to internal binding personas via the `PersonaService` defined in `internal/services/gateway/user_service.go:392`.

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

The platform implements a deterministic 5-layer governance sequence. Every mutation must pass through all active layers before execution. The structural schema is defined as `GovernanceEnvelope` in `protocol/proto/g8e/common/v1/common.proto:83`.

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
*Implementation: `internal/services/governance/l3_notary.go:33`*

L3 ensures explicit human authorization for mutations.
- **Suspension**: The g8e Gateway suspends transactions requiring L3 approval, storing them in the suspended transaction pool.
- **Out-of-Band (OOB) Approval**: The user approves via CLI command (`g8e approve <tx_hash>`) with a cryptographic Ed25519 signature over the transaction hash, or via WebAuthn passkey for web sessions.
- **Approval Window**: CLI-based approvals are valid for 30 minutes from the time of approval. Transactions not dispatched within that window are rejected and must be re-approved.
- **Cryptographic Binding**: The CLI proof requires a hex-encoded Ed25519 signature of exactly 64 bytes (`cli_signature`) and, when configured, an mTLS certificate fingerprint (`mtls_cert_fingerprint`) that must match the fingerprint recorded at suspension time.
- **Composite L3 Verification**: The `CompositeL3Verifier` (`internal/services/gateway/composite_l3_verifier.go:35`) routes L3 proof verification based on proof type. If the proof contains `mtls_cert_fingerprint`, it delegates to `CLIL3Notary` (`internal/services/gateway/cli_l3_notary.go:38`) for CLI mTLS session verification. Otherwise, it delegates to `PasskeyService` (`internal/services/gateway/passkey_service.go:63`) for WebAuthn assertion verification.
- **CLI L3 Notary**: The `CLIL3Notary` verifies that the user is active, the CLI session exists and belongs to the user, the certificate fingerprint matches the session's stored fingerprint, the session is active and not expired, and the certificate is not revoked via the PKI authority.
- **Passkey Service**: The `PasskeyService` handles L3 proof brokerage for WebAuthn operations, moving L3 authorization into the gateway as the sovereign authority.
- **Outbound L3 Notary**: The `outboundL3Notary` (`internal/services/governance/l3_notary.go:43`) provides L3 verification for outbound mode, verifying cryptographic signatures over the transaction hash against suspended and approved transactions.
- **L3Proof**: A successful approval generates an `L3Proof` (defined in `protocol/proto/g8e/common/v1/common.proto:58`) containing the cryptographic signature and certificate fingerprint, cryptographically bound to the `transaction_hash`.

### Layer 4: Warden (L4Warden)
*Implementation: `internal/services/governance/l4_warden.go:246`*

The Warden is the final fail-closed gate before execution. It verifies in the following order:
1. **In-Flight Tracking**: Prevents concurrent processing of transactions with the same nonce via an in-memory `sync.Map` guard.
2. **Nonce Reservation**: Early durable replay protection via `ReplayStore.ReserveNonce`, committed to SQLite before any expensive cryptographic checks. Expiry is checked before reservation; expired transactions are rejected.
3. **Stateless Validation**: Structural integrity checks, action type recognition, typed payload decoding, L1Doctrine compliance, and transaction hash verification (both `id` and `transaction_hash` must equal the computed hash).
4. **Stateful Validation**: State Merkle root consistency check via `StateRootProvider`; rejects the envelope if the provided `state_merkle_root` does not match the current root.
5. **Posture Validation**: L2 and L3 enforcement based on the configured `GovernancePosture` (Doctrine, Consensus, or Notary). L2 signature verification loads the `TribunalPolicy` from `TribunalStore`, verifies each signer is a tribunal member, resolves their Ed25519 public key from `SignerStore` by `key_id`, verifies the signature over `{transaction_hash}|{decision}`, and counts affirmative votes against the quorum threshold. L3 proof verification delegates to the configured `L3Notary` implementation, typically the `CompositeL3Verifier` which routes to `PasskeyService` (WebAuthn) or `CLIL3Notary` (mTLS) based on proof type.

### Layer 5: Actuator (L5Actuator)
*Implementation: `internal/services/governance/l5_actuator.go:52`*

The Actuator represents the execution boundary and final audit commitment.
- **Fail-Closed Pre-Execution**: Receipt signing and initial audit logging must both succeed before the execution handler is invoked. If either fails, the transaction is aborted.
- **Sensitive Data Rehydration**: Rehydrates scrubbed placeholders (such as `{{UEI_1}}`) with original sensitive data just before execution via `ScrubbingService.RehydratePayload`.
- **Egress Dispatch**: Dispatches the verified payload to downstream executors (Shell, MCP, A2A) via `ExecutionHandler.ExecuteVerifiedTransaction`.
- **Action Receipts**: Issues a signed `ActionReceipt` using the Actuator's own Ed25519 key over a canonical JSON serialization of the receipt fields, providing immutable proof of the execution outcome.
- **Commitment**: Records the transaction in the `SQLAuditStore` and, where configured, in the console audit store.

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
- Default configuration: embedded in binary via `config.DefaultInfraPaths()`
