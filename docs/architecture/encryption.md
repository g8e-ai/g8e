# Encryption Architecture

Last Updated: 2026-08-16
Version: v1.7.6

## Overview

g8e enforces mandatory encryption for all sensitive data at rest and mutual TLS (mTLS) for all network communication. The encryption system consists of a per-host vault that provides AES-256-GCM primitives, a three-tier key hierarchy, a platform keystore backed by OS keyrings for secret storage, and a Public Key Infrastructure (PKI) hierarchy for certificate-based mTLS authentication.

All platform mutations are protected by the five-layer governance interlock described in [Governance](./governance.md) and [Authentication & Authorization](./auth.md). See [Authentication & Authorization](./auth.md) for identity enrollment and session management, and [Storage Architecture](./storage.md) for service-specific storage behavior.

## Design Principles

- **Fail-closed**: Encryption is mandatory. The gateway fails to start unless the vault is initialized and unlocked.
- **Zero-knowledge**: The Data Encryption Key (DEK) is never persisted to disk in plaintext; only its wrapped form is stored.
- **Key rotation**: Re-keying rotates the DEK wrapper without re-encrypting data.
- **Mutual authentication**: All network connections require client certificates verified against the platform CA chain.

## Key Hierarchy

The vault uses a three-tier key hierarchy:

1. **Private Key**: A 32-byte hex-encoded value, user-provided or auto-generated, used to unlock the vault.
2. **Key Encryption Key (KEK)**: Derived from the private key using HKDF-SHA256. It wraps and unwraps the DEK.
3. **Data Encryption Key (DEK)**: A per-vault random key, wrapped by the KEK before storage. The DEK is used for per-record AES-256-GCM encryption with unique nonces.

## Encrypted Data at Rest

The audit store, execution vault, ledger, and encrypted key-value adapter all encrypt content using the vault when it is unlocked. The following data is encrypted at rest:

- **Audit store**: Event content, command stdout, and command stderr.
- **Execution vault**: Compressed stdout, compressed stderr, and compressed file diffs.
- **Ledger**: File content stored in the git-backed ledger with the `.enc` suffix.
- **Encrypted key-value store**: Sentinel token values.

The gateway refuses to start if the vault cannot be unlocked. Other components that use the vault fail closed when it is locked rather than falling back to plaintext.

Platform signing keys, certificate keys, and session secrets are encrypted separately through the [Platform Keystore](#platform-keystore).

## Vault Lifecycle

### Initialization

The `g8e vault` commands manage the lifecycle of the per-host vault:

- `g8e vault init` generates a new vault with a random key and writes it to the default key path.
- `g8e vault init --vault-dir <path>` uses a custom vault directory.
- `g8e vault init --key-path <path>` uses a custom key path.
- `g8e vault import --key-hex <hex-string>` imports an existing key; `g8e vault import` reads it from stdin.

The gateway also auto-initializes a vault on first start if no vault header exists, generating a random key and saving it to the default key path. This enables zero-config startup for development and testing.

### Unlocking

- `g8e vault unlock --key-path <path/to/vault/key>` unlocks an existing vault.
- `g8e vault unlock --vault-dir <path>` unlocks a vault in a custom directory.
- `g8e gw start` uses `--vault-key <path>` or the `G8E_VAULT_KEY` environment variable to locate the key, or defaults to the key file in the vault directory.

### Re-keying

- `g8e vault rekey --key-path <path/to/vault/key> --new-key-path <path/to/new.key>` re-wraps the DEK with a new private key.
- Update the gateway `--vault-key` flag or `G8E_VAULT_KEY` environment variable to point to the new key and restart the gateway.

### Status and Reset

- `g8e vault status` shows whether the vault is initialized and unlocked.
- `g8e vault reset` destroys the vault and all encrypted data. Use `--confirm` to skip the interactive prompt.

### Export and Import

- `g8e vault export --key-path <path/to/vault/key>` writes the private key in hex to stdout.
- `g8e vault import` stores a key provided on stdin or via `--key-hex`.

## Configuration

### Vault Command Flags

- `--vault-dir`: Directory for vault data (default: `.g8e/vault`).
- `--key-path`: Path to the vault key (default: `.g8e/vault/key`).
- `--new-key-path`: Path to save the new vault key during rekey (default: `<vault-dir>/key.new`).
- `--confirm`: Skip interactive confirmation for vault reset.
- `--key-hex`: Vault key as a hex string for the import command.

### Gateway Flags

- `--vault-dir`: Directory for vault data (default: `.g8e/vault`).
- `--vault-key`: Path to the vault private key (default: `.g8e/vault/key`).

### Environment Variables

- `G8E_VAULT_DIR`: Override the vault directory.
- `G8E_VAULT_KEY`: Override the vault key path.

## Security Guarantees

### Data at Rest

- All sensitive data is encrypted with AES-256-GCM.
- Each record uses a unique nonce to prevent key reuse.
- The DEK is never written to disk in plaintext; only its wrapped form persists.
- Private key and KEK material are zeroed from memory when the vault is locked.

### Key Management

- Private keys are 32-byte hex-encoded values.
- Key fingerprints are derived identifiers with domain separation; they are not secrets.
- Keys can be imported or exported for backup via `g8e vault export` and `g8e vault import`.
- Re-keying rotates the DEK wrapper without data loss.
- Vault reset destroys all data irrecoverably.

### Fail-Closed Behavior

- The gateway fails to start without an unlocked vault.
- Encryption operations fail if the vault is locked.
- No component silently falls back to plaintext storage.
- Errors are logged and propagated to callers.

## Platform Keystore

The keystore manages platform secrets using a master encryption key stored in the OS-native credential store. Secrets are encrypted with AES-256-GCM and stored as JSON structures with embedded nonces. It also provides in-memory encryption and decryption for runtime values stored in the database.

### OS Keyring Support

The keystore uses the OS-native credential store when available, with a file-based fallback:

- **Linux**: libsecret/GNOME Keyring, with a file-based fallback.
- **macOS**: Keychain.
- **Windows**: File-based storage.

The file-based fallback stores the master key as a base64-encoded file with restrictive permissions and uses atomic file writes.

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
2. **Hub Intermediate CA**: Signed by Root, 10-year validity. Signs the gateway serving certificate.
3. **Operator Intermediate CA**: Signed by Root, 10-year validity. Signs operator, CLI, and app leaf certificates.
4. **Gateway Peer Intermediate CA**: Signed by Root, 10-year validity. Signs gateway peer certificates for multi-host deployments.

CA private keys are stored encrypted in the keystore. CA certificates are written to the PKI directory with public read permissions.

### Certificate Types and Validity

| Certificate Type | Signing CA | Validity |
|---|---|---|
| Gateway serving cert | Hub Intermediate CA | 90 days |
| Operator leaf cert | Operator Intermediate CA | 7 days |
| CLI leaf cert | Operator Intermediate CA | 7 days |
| App enrollment cert | Operator Intermediate CA | 7 days |
| Gateway peer cert | Gateway Peer Intermediate CA | 90 days |
| Delegated app credential | Operator Intermediate CA | 1 hour |

All PKI-issued certificates use ECDSA P-256 keys. Certificate Signing Requests (CSRs) with non-P-256 keys are rejected.

### mTLS Enforcement

The gateway exposes two ports:

- **HTTP port**: Plain HTTP for bootstrap and MCP discovery flows.
- **HTTPS port**: mTLS for API, enrollment, and public surfaces. It uses TLS 1.3 as the minimum version and requires and verifies a client certificate against a CA pool containing the Root CA and Operator Intermediate CA.

Route-level mTLS enforcement is handled by authentication middleware, which classifies routes by auth mode. See [Authentication & Authorization](./auth.md) for details.

### SPIFFE Workload Identity

All certificates carry SPIFFE URI SANs under the `g8e.local` trust domain. The following identity formats are used:

- **Operator**: `spiffe://g8e.local/operator/<org_id>/<operator_id>/<session_id>`
- **CLI**: `spiffe://g8e.local/cli/<user_id>/<session_id>`
- **App**: `spiffe://g8e.local/app/<operator_id>`
- **Gateway peer**: `spiffe://g8e.local/gateway/<gateway_id>`
- **Hub**: `spiffe://g8e.local/hub/operator-listen`
- **User (delegated)**: `spiffe://g8e.local/user/<user_id>`

The SPIFFE URI SAN binds the certificate to a specific workload identity and session.

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

1. Run `g8e vault init` to create a vault, or let the gateway auto-initialize it on first start.
2. Unlock the vault with `g8e vault unlock --key-path .g8e/vault/key`.
3. Restart the gateway with `g8e gw restart`.
4. New data is encrypted automatically. Existing unencrypted data remains unchanged and is not retroactively re-encrypted.

### Key Rotation

To rotate vault keys, follow the [Re-keying](#re-keying) steps, then update the gateway to reference the new key path and restart.

## Compliance

### FIPS 140-3

g8e links against the Go Cryptographic Module v1.0.0 (CMVP Cert #5247, CAVP A6650) when built with `GOFIPS140=v1.0.0`. FIPS mode is activated at build time; no runtime environment variable is required. The binary runs integrity self-checks and known-answer tests automatically.

The validated algorithms used by g8e include EdDSA (Ed25519) for consensus signatures and receipts, ECDSA P-256 for PKI certificate signatures, AES-256-GCM for vault and keystore encryption, HKDF-SHA256 for key derivation, HMAC-SHA256 for auditor authentication, SHA-256 for transaction hashing, and the X25519MLKEM768 hybrid for FIPS 203 post-quantum TLS key agreement.

X25519 is removed from all TLS configurations because it is not SP 800-56A rev3 compliant. Ed25519 is excluded from TLS certificate signatures; g8e enforces ECDSA P-256 for all PKI-issued certificates and rejects Ed25519-signed certificates at load time.

The `g8e version --fips` command reports FIPS approved mode, enforcement state, and validated module version. It exits non-zero only if approved mode is not active.

See [FIPS 140-3 Compliance](../reference/fips140-3.md) for the complete validated boundary, operating environment matrix, and build and runtime activation details.

## Troubleshooting

### Vault Locked

If services fail with a locked vault error, run `g8e vault status`, unlock with `g8e vault unlock --key-path <path/to/vault/key>`, and restart the gateway with `g8e gw restart`.

### Invalid Key

If vault unlock fails with an invalid key error:

- Verify the key path is correct.
- Ensure the key file exists and is readable.
- Confirm the key is a 32-byte hex-encoded value.
- If the key is lost, data is unrecoverable.

### Vault Not Initialized

If services fail with an uninitialized vault error, run `g8e vault init`, unlock with `g8e vault unlock --key-path .g8e/vault/key`, and restart the gateway with `g8e gw restart`.

### Certificate Expired

If mTLS connections fail with certificate errors:

- Gateway serving certificates auto-renew within 30 days of expiry.
- Operator and CLI leaf certificates expire after 7 days and require re-enrollment via `g8e auth enroll`.
- Check certificate validity using standard certificate tools.

## Receipt Signature Verification

The gateway signs action receipts with its Actuator Ed25519 private key. The actuator public key is exported to the PKI directory during gateway boot in both PEM and JSON formats with its key ID, enabling offline verification by external harnesses.

Consumers that need to cryptographically verify receipt authenticity must obtain the public key out-of-band by reading the exported files from the gateway PKI directory. They can implement Ed25519 signature verification using standard cryptographic libraries with the exported public key.
