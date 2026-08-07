# Encryption Architecture

Last Updated: 2026-08-07
Version: v1.7.0

## Overview

g8e enforces mandatory encryption for all sensitive data at rest and mutual TLS (mTLS) for all network communication. The encryption system consists of a vault providing AES-256-GCM primitives, a key hierarchy for key management, a platform keystore backed by OS keyrings for secret storage, and a Public Key Infrastructure (PKI) hierarchy for certificate-based mTLS authentication.

See [Authentication & Authorization](./auth.md) for identity enrollment and session management, and [Storage Architecture](./storage.md) for service-specific encryption behavior.

## Design Principles

- **Fail-closed**: Encryption is mandatory. Platform services fail to initialize without an unlocked vault.
- **Zero-knowledge**: The Data Encryption Key (DEK) is never persisted to disk in plaintext; only its wrapped form is stored.
- **Key rotation**: Re-keying rotates the DEK wrapper without data loss.
- **Auditability**: Encryption state is recorded in audit logs, and vault lifecycle operations produce verifiable audit entries.
- **Mutual authentication**: All network connections require client certificates verified against the platform CA chain.

## Governance Pipeline Interlock

The platform enforces a five-layer interlock sequence to guarantee that no action executes without full policy, consensus, notary, warden, and actuator verification:

- **L1 Doctrine**: Hard gates, forbidden pattern matching, and MITRE threat detection.
- **L2 Consensus**: Multi-agent consensus signature verification using Ed25519 key pairs.
- **L3 Notary**: Human-in-the-loop authorization via WebAuthn passkeys or signed CLI proofs.
- **L4 Warden**: Pre-dispatch verification including signatures, replay prevention, expiration, nonces, and Merkle root integrity.
- **L5 Actuator**: Isolated tool execution, JIT capability minting, and signed receipt production using the Actuator Ed25519 signing key.

## Key Hierarchy

The vault uses a three-tier key hierarchy:

1. **Private Key**: A 32-byte hex-encoded value, user-provided or auto-generated, used to unlock the vault.
2. **Key Encryption Key (KEK)**: Derived from the private key via HKDF-SHA256. Used to wrap and unwrap the Data Encryption Key.
3. **Data Encryption Key (DEK)**: A per-vault random key, wrapped with AES Key Wrap (RFC 3394). Used for per-record AES-256-GCM encryption with unique nonces.

## Encrypted Data

All storage services require an unlocked vault at initialization. The following data is encrypted at rest:

- **Audit store**: Event content, command stdout, and command stderr fields.
- **Execution vault**: Compressed stdout, compressed stderr, and compressed file diffs from command executions.
- **Ledger**: File content stored in the git-backed ledger with `.enc` suffix.
- **Key-value store**: Sentinel token values in the canonical key-value store.

Platform signing keys, certificate keys, and session secrets are encrypted separately via the [Platform Keystore](#platform-keystore).

## Vault Lifecycle

### Initialization

1. Generate a new vault with an auto-generated key: `g8e vault init`.
2. To use a custom vault directory, add `--vault-dir <path>`.
3. To use a custom key path, add `--key-path <path>`.
4. To import an existing key from a hex string, run `g8e vault import --key-hex <hex-string>`, or `g8e vault import` to read the key from stdin.

The gateway auto-initializes a vault on first start if no vault header exists. A random private key is generated and saved to the default key path, enabling zero-config startup for development and testing.

### Unlocking

1. Unlock the vault with a key file: `g8e vault unlock --key-path <path/to/vault/key>`.
2. For a custom vault directory, add `--vault-dir <path>`.
3. For gateway or operator startup, set `G8E_VAULT_KEY` to the key path, or pass `--vault-key <path>` to `g8e gw start`.

### Re-keying

1. Re-encrypt the vault with a new key: `g8e vault rekey --key-path <path/to/vault/key> --new-key-path <path/to/new.key>`.
2. For a custom vault directory, add `--vault-dir <path>`.
3. Update configuration to reference the new key path, then restart the gateway.

### Status

Check vault state with `g8e vault status`, or `g8e vault status --vault-dir <path>` for a custom vault directory.

### Reset

Reset destroys the vault and all encrypted data irrecoverably.

1. Run `g8e vault reset` to trigger the destructive reset.
2. For a custom vault directory, add `--vault-dir <path>`.
3. Add `--confirm` to skip interactive confirmation.

### Export

Export the private key in hex format with `g8e vault export --key-path <path/to/vault/key>`, or `g8e vault export` to use the default key path.

## Configuration

### Vault Command Flags

- `--vault-dir`: Directory for vault data (default: `.g8e/vault`).
- `--key-path`: Path to vault key (used in vault commands).
- `--new-key-path`: Path to save new vault key during rekey (default: `<vault-dir>/key.new`).
- `--confirm`: Skip interactive confirmation for vault reset.
- `--key-hex`: Vault key as hex string for the import command.

### Gateway Flags

- `--vault-dir`: Directory for vault data (default: `.g8e/vault`).
- `--vault-key`: Path to vault private key (default: `.g8e/vault/key`).

### Environment Variables

- `G8E_VAULT_DIR`: Override vault directory.
- `G8E_VAULT_KEY`: Override vault key path.

### Default Paths

The vault directory defaults to `.g8e/vault` and the vault key defaults to `.g8e/vault/key`. These paths are resolved relative to the current working directory at startup.

## Security Guarantees

### Data at Rest

- All sensitive data is encrypted with AES-256-GCM.
- Each record uses a unique nonce to prevent key reuse.
- The DEK is never written to disk in plaintext; only its wrapped form persists.
- Key material is zeroed from memory when the vault is locked.

### Key Management

- Private keys are 32-byte hex-encoded values.
- Key fingerprints are computed using SHA-256 with a domain-separation pepper for identification.
- Keys can be imported or exported for backup via `g8e vault export` and `g8e vault import`.
- Re-keying rotates the DEK wrapper without data loss because only the wrapper changes.
- Vault reset destroys all data irrecoverably.

### Fail-Closed Behavior

- Platform services fail to initialize without an unlocked vault.
- Encryption operations fail if the vault is locked.
- No silent fallback to plaintext storage occurs.
- Errors are logged and propagated to callers.

## Platform Keystore

The keystore manages platform secrets using a master encryption key stored in the OS-native credential store. Secrets are encrypted with AES-256-GCM and stored as JSON structures with embedded nonces. The keystore also provides in-memory encryption and decryption for runtime values stored in the database.

### OS Keyring Support

The keystore uses the OS-native credential store when available, with a file-based fallback:

- **Linux**: GNOME Keyring via libsecret, with file-based fallback.
- **macOS**: Keychain.
- **Windows**: File-based storage.

The file-based fallback stores the master key as a base64-encoded file with restrictive permissions. Atomic writes are performed via temporary file rename operations, and all keystore file I/O within `.g8e/` enforces restrictive file permissions.

### Encrypted Secrets

The keystore encrypts the following platform secrets at rest:

- Session encryption keys and session tokens.
- Ed25519 signing keys (actuator, notary, operator, CLI).
- CA private keys (root, hub, operator, gateway peer).
- Service certificate private keys.
- API keys for external service integrations.
- Auditor HMAC keys.

## TLS and mTLS

### PKI Hierarchy

The gateway operates a full PKI hierarchy using ECDSA P-256 certificates:

1. **Root CA**: Self-signed, 10-year validity. Signs all intermediate CAs.
2. **Hub Intermediate CA**: Signed by Root CA, 10-year validity. Signs the gateway serving certificate.
3. **Operator Intermediate CA**: Signed by Root CA, 10-year validity. Signs operator, CLI, and app leaf certificates.
4. **Gateway Peer Intermediate CA**: Signed by Root CA, 10-year validity. Signs gateway peer certificates for multi-host deployments.

CA private keys are stored encrypted in the keystore. CA certificates are written to the PKI directory with public read permissions.

### Certificate Types and Validity

| Certificate Type | Signing CA | Validity |
|------------------|-----------|----------|
| Gateway serving cert | Hub Intermediate CA | 90 days |
| Operator leaf cert | Operator Intermediate CA | 7 days |
| CLI leaf cert | Operator Intermediate CA | 7 days |
| App enrollment cert | Operator Intermediate CA | 7 days |
| Gateway peer cert | Gateway Peer Intermediate CA | 90 days |
| Delegated app credential | Operator Intermediate CA | 1 hour |

All certificates use ECDSA P-256 keys. Certificate Signing Requests (CSRs) with non-P-256 keys are rejected.

### mTLS Enforcement

The HTTPS server uses TLS 1.3 as the minimum version and accepts client certificates when presented, with application-layer middleware enforcing mTLS on all non-public routes. Client certificates are verified against a CA pool containing the Root CA and Operator Intermediate CA. The HTTP server accepts client certificates when presented but does not require them, enabling bootstrap and enrollment flows on plain HTTP.

Route-level mTLS enforcement is handled by authentication middleware, which classifies routes by auth mode. See [Authentication & Authorization](./auth.md) for details.

### SPIFFE Workload Identity

All certificates carry SPIFFE URI SANs under the `g8e.local` trust domain. The following identity formats are used:

- **Operator**: `spiffe://g8e.local/operator/<org_id>/<operator_id>/<session_id>`
- **CLI**: `spiffe://g8e.local/cli/<user_id>/<session_id>`
- **App**: `spiffe://g8e.local/app/<operator_id>`
- **Gateway peer**: `spiffe://g8e.local/gateway/<gateway_id>`
- **Hub**: `spiffe://g8e.local/hub/operator-listen`
- **User (delegated)**: `spiffe://g8e.local/user/<user_id>`

The SPIFFE URI SAN is verified during mTLS authentication to bind the certificate to a specific workload identity and session.

### Trust Bundles

The gateway generates and maintains the following trust bundles in the PKI directory:

- **Gateway bundle**: Root CA + Hub Intermediate CA + Operator Intermediate CA + Gateway Peer Intermediate CA. Used by clients connecting to the gateway.
- **Operator bundle**: Root CA + Operator Intermediate CA. Used by operator instances.
- **Root CA mirror**: Root CA only, for operator clients.

## Certificate Management

### CSR Signing

The gateway signs Certificate Signing Requests (CSRs) for operator, CLI, app, and gateway peer enrollment. CSRs must use ECDSA P-256 keys. The signing process embeds SPIFFE URI SANs based on the enrollment type and session context.

### App Enrollment

External apps can enroll via the PKI API to receive mTLS identity certificates. Enrollment is identity-only by default, giving apps certificates without consensus power. L2 signer capability requires explicit admin registration.

Delegated credentials are short-lived certificates valid for 1 hour that bind both an app identity and a requesting user identity via dual SPIFFE URI SANs. These enable user-scoped app operations without sharing long-term credentials.

### Certificate Revocation

The gateway maintains a revocation list in the canonical database. Revoked certificate serials are checked during mTLS authentication. The gateway generates standard X.509 Certificate Revocation Lists (CRLs) signed by the Operator Intermediate CA.

### Auto-Renewal

The gateway automatically regenerates its serving certificate if it is missing or within 30 days of expiry. CA private keys missing from the keystore trigger CA regeneration.

## Migration Path

### From Unencrypted to Encrypted

For existing deployments with unencrypted data:

1. Initialize the vault: `g8e vault init`.
2. Unlock the vault: `g8e vault unlock --key-path .g8e/vault/key`.
3. Restart the gateway: `g8e gw restart`.
4. New data is encrypted automatically.

### Key Rotation

To rotate vault keys, follow the [Re-keying](#re-keying) steps, then update configuration to reference the new key path and restart the gateway.

## Compliance

### FIPS 140-3

g8e uses the Go Cryptographic Module v1.0.0 (CMVP Cert #5247, CAVP A6650) in FIPS 140-3 approved mode. FIPS mode is activated at build time via `GOFIPS140=v1.0.0`; no runtime environment variable is required. The binary enters approved mode automatically on startup and runs integrity self-checks and known-answer tests at init or first use.

**Validated algorithms used by g8e**: EdDSA (Ed25519) for consensus signatures and receipts, ECDSA P-256 for PKI certificate signatures, AES-256-GCM for vault encryption, HKDF-SHA256 for key derivation, HMAC-SHA256 for auditor authentication, SHA-256 for transaction hashing, TLS 1.3 key schedule, and ML-KEM (FIPS 203) for post-quantum TLS key agreement.

**Excluded algorithms**: X25519 is removed from all TLS configurations (not SP 800-56A rev3 compliant). Ed25519 is excluded from TLS certificate signatures; g8e enforces ECDSA P-256 for all PKI-issued certificates and rejects Ed25519-signed certificates at load time.

The `g8e version --fips` command reports FIPS approved mode, enforcement state, and validated module version. It exits non-zero only if approved mode is not active.

See [FIPS 140-3 Compliance](../reference/fips140-3.md) for the complete validated boundary, operating environment matrix, and build and runtime activation details.

## Troubleshooting

### Vault Locked

If services fail with a locked vault error, check vault state with `g8e vault status`, unlock with `g8e vault unlock --key-path <path/to/vault/key>`, then restart the gateway with `g8e gw restart`.

### Invalid Key

If vault unlock fails with an invalid key error:

- Verify the key path is correct.
- Ensure the key file exists and is readable.
- Check the key format (32-byte hex-encoded).
- If the key is lost, data is unrecoverable.

### Vault Not Initialized

If services fail with an uninitialized vault error, initialize the vault with `g8e vault init`, unlock it with `g8e vault unlock --key-path .g8e/vault/key`, then restart the gateway with `g8e gw restart`.

### Certificate Expired

If mTLS connections fail with certificate errors:

- Gateway serving certificates auto-renew within 30 days of expiry.
- Operator and CLI leaf certificates expire after 7 days and require re-enrollment via `g8e auth enroll`.
- Check certificate validity using standard certificate tools.

## Receipt Signature Verification

The gateway signs action receipts with its Actuator Ed25519 private key. The actuator public key is exported to the PKI directory during gateway boot in both PEM and JSON formats with its key ID, enabling offline verification by external harnesses.

Consumers that need to cryptographically verify receipt authenticity must obtain the public key out-of-band by reading the exported files from the gateway PKI directory. Consumers can implement Ed25519 signature verification using standard cryptographic libraries with the exported public key.
