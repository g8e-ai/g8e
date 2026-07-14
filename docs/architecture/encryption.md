# Encryption Architecture

Last Updated: 2026-07-13
Version: v1.5.0

## Overview

g8e uses mandatory encryption for all sensitive data at rest. The encryption system is built around a vault that provides AES-256-GCM primitives, a key hierarchy for managing encryption keys, and a keystore that reuses the vault's crypto primitives for OS keyring-backed secret storage.

## Design Principles

- **Fail-closed**: Encryption is mandatory. Services fail to initialize without a vault.
- **Zero-knowledge**: The DEK is never persisted to disk; only its wrapped form is stored.
- **Key rotation**: Support for re-keying without data loss.
- **Auditability**: Encryption state is recorded in audit records, and vault lifecycle operations are logged.

## Key Hierarchy

The vault uses a three-tier key hierarchy:

1. **Private Key**: A 32-byte hex-encoded value, user-provided or auto-generated. Used to unlock the vault.
2. **Key Encryption Key (KEK)**: Derived from the private key via HKDF-SHA256. Used to wrap and unwrap the DEK.
3. **Data Encryption Key (DEK)**: A per-vault random key, wrapped with AES Key Wrap (RFC 3394). Used for per-record AES-256-GCM encryption with unique nonces.

## Encrypted Data

All storage services require an unlocked vault at initialization. The following data is encrypted at rest:

- **Audit store**: Audit record content, command stdout, and command stderr fields.
- **Execution vault**: Compressed stdout, compressed stderr, and compressed file diffs from command executions.
- **Ledger**: File content stored in the git-backed ledger with `.enc` suffix.
- **KV store adapter**: Sentinel UEI token values in the canonical KV store.
- **Keystore**: Session encryption keys, ED25519 signing keys (actuator, notary, operator, CLI), CA private keys, service certificate private keys, API keys for external services, and auditor HMAC keys, stored via OS keyring with file-based fallback.

## Vault Lifecycle

### Initialization

```bash
# Generate new vault with auto-generated key
./g8e vault init

# Initialize with custom vault directory
./g8e vault init --vault-dir /custom/path

# Initialize with custom key path
./g8e vault init --key-path /custom/path/key

# Import key from hex string
./g8e vault import --key-hex <hex-string>

# Import key from stdin
./g8e vault import
```

### Unlocking

```bash
# Unlock vault with key file
./g8e vault unlock --key-path /path/to/vault/key

# Unlock with custom vault directory
./g8e vault unlock --vault-dir /custom/path --key-path /custom/path/key

# Unlock with environment variable (used by gateway/operator)
export G8E_VAULT_KEY=/path/to/vault/key
./g8e gw start
```

### Re-keying

```bash
# Re-encrypt vault with new key
./g8e vault rekey --key-path /path/to/vault/key --new-key-path /path/to/new.key

# Re-key with custom vault directory
./g8e vault rekey --vault-dir /custom/path --key-path /custom/path/key --new-key-path /custom/path/key.new
```

### Status

```bash
# Check vault status
./g8e vault status

# Check status with custom vault directory
./g8e vault status --vault-dir /custom/path
```

### Reset

```bash
# Destroy vault and all encrypted data (destructive)
./g8e vault reset

# Reset with custom vault directory
./g8e vault reset --vault-dir /custom/path

# Skip interactive confirmation
./g8e vault reset --confirm
```

### Export

```bash
# Export private key in hex format
./g8e vault export --key-path /path/to/vault/key

# Export with default key path
./g8e vault export
```

## Configuration

### CLI Flags

- `--vault-dir`: Directory for vault data (default: `.g8e/vault`).
- `--key-path`: Path to vault key (used in vault commands).
- `--new-key-path`: Path to save new vault key during rekey (default: `<vault-dir>/key.new`).
- `--confirm`: Skip interactive confirmation for vault reset.
- `--key-hex`: Vault key as hex string for import command.

### Environment Variables

- `G8E_VAULT_DIR`: Override vault directory.
- `G8E_VAULT_KEY`: Override vault key path.
- `G8E_VAULT_REQUIRE_UNLOCK`: Set to `true` to require the vault to be unlocked at gateway startup (fail if vault cannot be unlocked).

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
- Key fingerprints are computed using SHA-256 with a domain-separation pepper. The key material (256-bit) provides sufficient entropy; a fast hash is appropriate for identification purposes.
- Keys can be imported or exported for backup via `g8e vault export` and `g8e vault import`.
- Re-keying rotates the DEK wrapper without data loss (only the DEK wrapper changes).
- Vault reset destroys all data irrecoverably.

### Fail-Closed Behavior

- Services fail to initialize without a vault.
- Encryption operations fail if the vault is locked.
- No silent fallback to plaintext storage.
- Errors are logged and propagated to callers.

## Migration Path

### From Unencrypted to Encrypted

For existing deployments with unencrypted data:

1. Initialize vault: `./g8e vault init`
2. Unlock vault: `./g8e vault unlock --key-path .g8e/vault/key`
3. Restart gateway: `./g8e gw restart`
4. New data is encrypted automatically.

### Key Rotation

To rotate vault keys:

1. Generate new key: `./g8e vault rekey --key-path .g8e/vault/key --new-key-path .g8e/vault/key.new`
2. Update configuration to use new key.
3. Restart gateway.

## Compliance

### Encryption Standards

- AES-256-GCM (NIST-approved) for all data encryption.
- HKDF-SHA256 for Key Encryption Key derivation.
- AES Key Wrap (RFC 3394) for DEK wrapping.
- SHA-256 for key fingerprinting with domain-separation pepper.
- Key material is zeroed from memory after use in both the vault and keystore.

### Data Protection

- Sensitive data encrypted at rest.
- The DEK is never persisted in plaintext; only its wrapped form is stored.
- Encryption state tracked in audit records.
- Support for key rotation and deletion.

### Platform Keyring Support

The keystore uses the OS-native credential store when available, with a file-based fallback:

- **Linux**: GNOME Keyring via libsecret, with file-based fallback.
- **macOS**: Keychain.
- **Windows**: File-based storage.

The file-based fallback stores the master key as a base64-encoded file with restrictive permissions. Atomic writes are performed via temp file rename. All keystore file I/O within `.g8e/` uses `RuntimeFileService` (`internal/services/fs`) with relative paths constructed from `constants.*` constants, replacing direct `os.*` calls. Paths are resolved via `fileSvc.Resolve(constants.*)` instead of `DataDir` or `CredentialsDir` config fields.

## Troubleshooting

### Vault Locked

If services fail with "vault is locked":

```bash
# Check vault status
./g8e vault status

# Unlock vault
./g8e vault unlock --key-path /path/to/vault/key

# Restart gateway
./g8e gw restart
```

### Invalid Key

If vault unlock fails with invalid key:

- Verify key path is correct.
- Ensure key file exists and is readable.
- Check key format (32-byte hex-encoded).
- If key is lost, data is unrecoverable.

### Vault Not Initialized

If services fail with "vault not initialized":

```bash
# Initialize vault
./g8e vault init

# Unlock vault
./g8e vault unlock --key-path .g8e/vault/key

# Restart gateway
./g8e gw restart
```
