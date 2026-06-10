# Encryption Architecture

## Overview

g8e uses mandatory encryption for all sensitive data at rest. The encryption system is built around a vault service that provides AES-256-GCM encryption with a master key hierarchy and wrapped Data Encryption Key (DEK).

## Design Principles

- **Fail-closed**: Encryption is mandatory. Services fail to initialize without a vault.
- **Zero-knowledge**: Vault keys are never written to disk in plaintext.
- **Key rotation**: Support for re-keying without data loss.
- **Auditability**: All encryption operations are logged to the audit vault.

## Vault Architecture

### Key Hierarchy

```
Master Key (32-byte hex-encoded, user-provided or generated)
    |
    +-- Key Encryption Key (KEK) - derived via HKDF-SHA256 from master key
        |
        +-- Data Encryption Key (DEK) - per-vault, wrapped with AES Key Wrap (RFC 3394)
            |
            +-- Per-record encryption - AES-256-GCM with DEK + unique nonce
```

### Vault Components

- **Vault Header**: Metadata including key derivation parameters, salt, and iteration count
- **Vault Data**: Encrypted DEK and encrypted data records
- **Vault Key**: Master key (32-byte hex-encoded) used to unlock the vault

### Encryption Flow

1. **Vault Initialization**:
   - Generate or import master key (32-byte hex-encoded)
   - Derive Key Encryption Key (KEK) from master key using HKDF-SHA256
   - Generate Data Encryption Key (DEK)
   - Wrap DEK with KEK using AES Key Wrap (RFC 3394)
   - Save vault header with wrapped DEK to disk

2. **Data Encryption**:
   - Vault must be unlocked (DEK available in memory)
   - Generate unique nonce for each record
   - Encrypt data with AES-256-GCM using DEK and nonce
   - Store nonce + ciphertext

3. **Data Decryption**:
   - Vault must be unlocked
   - Read nonce from stored record
   - Decrypt ciphertext with AES-256-GCM using DEK and nonce
   - Return plaintext

## Storage Services with Encryption

All storage services require an unlocked vault at initialization:

| Service | Vault Required | Encrypted Data |
|---------|---------------|----------------|
| SQLAuditStore | Yes | Audit records, governance envelopes, audit trail, compliance records |
| ExecutionVaultService | Yes | Execution results, command outputs |
| TokenStoreService | Yes | Authentication tokens, session data |
| GitLedgerService | Yes | File content in ledger |

## Vault Lifecycle

### Initialization

```bash
# Generate new vault with auto-generated key
./g8e vault init

# Initialize with imported key
./g8e vault import --key-path /path/to/key
```

### Unlocking

```bash
# Unlock vault with key file
./g8e vault unlock --key-path /path/to/vault/key

# Unlock with environment variable
export G8E_VAULT_KEY=/path/to/vault/key
./g8e gw start
```

### Re-keying

```bash
# Re-encrypt vault with new key
./g8e vault rekey --key-path /path/to/vault/key --new-key-path /path/to/new.key
```

### Status

```bash
# Check vault status
./g8e vault status
```

### Reset

```bash
# Destroy vault and all encrypted data (destructive)
./g8e vault reset
```

## Configuration

### CLI Flags

- `--vault-dir`: Directory for vault data (default: `.g8e/vault`)
- `--vault-key`: Path to vault private key (default: `.g8e/vault/key`)
- `--key-path`: Path to vault key (used in vault commands)

### Environment Variables

- `G8E_VAULT_DIR`: Override vault directory
- `G8E_VAULT_KEY`: Override vault key path

### Configuration File

Vault paths are configured in the embedded `paths_default.json` in `internal/cli/config/config.go`. The default paths are:

- Vault directory: `.g8e/vault`
- Vault key path: `.g8e/vault/key`

These paths are resolved relative to the current working directory.

## Security Guarantees

### Data at Rest

- All sensitive data is encrypted with AES-256-GCM
- Each record uses a unique nonce to prevent key reuse
- Vault keys are never written to disk in plaintext
- Vault keys are zeroed from memory when vault is locked

### Key Management

- Master keys are 32-byte hex-encoded values
- Keys can be imported/exported for backup via `g8e vault export` and `g8e vault import`
- Re-keying rotates the DEK without data loss
- Vault reset destroys all data irrecoverably

### Fail-Closed Behavior

- Services fail to initialize without a vault
- Encryption operations fail if vault is locked
- No silent fallback to plaintext storage
- Errors are logged and propagated to callers

## Implementation Details

### Vault Service

The vault service (`internal/services/vault/`) provides:

- `NewVault()`: Create new vault instance
- `Unlock()`: Unwrap DEK with master key
- `Lock()`: Zero DEK from memory
- `Encrypt()`: Encrypt data with AES-256-GCM
- `Decrypt()`: Decrypt data with AES-256-GCM
- `Rekey()`: Rotate DEK with new master key
- `GetDEK()`: Return Data Encryption Key for database operations
- `IsUnlocked()`: Check vault lock state
- `IsInitialized()`: Check if vault header exists
- `VerifyIntegrity()`: Verify vault integrity
- `Reset()`: Destroy vault and all data

### Storage Integration

Storage services integrate with vault via:

- Constructor validation: Reject nil vault with error
- Encryption checks: Verify `vault.IsUnlocked()` before encrypt/decrypt
- Error handling: Return errors on locked vault (fail-closed)

### CLI Commands

Vault management commands (`internal/cli/cmd/vault.go`):

- `init`: Initialize new vault with generated key
- `unlock`: Unlock vault with key
- `rekey`: Re-encrypt with new key
- `status`: Check vault status
- `reset`: Destroy vault
- `export`: Export master key in hex format
- `import`: Import master key from hex string or stdin

## Migration Path

### From Unencrypted to Encrypted

For existing deployments with unencrypted data:

1. Initialize vault: `./g8e vault init`
2. Unlock vault: `./g8e vault unlock --key-path .g8e/vault/key`
3. Restart gateway: `./g8e gw restart`
4. New data is encrypted automatically
5. Use migration tool to encrypt existing data (planned)

### Key Rotation

To rotate vault keys:

1. Generate new key: `./g8e vault rekey --key-path .g8e/vault/key --new-key-path .g8e/vault/key.new`
2. Update configuration to use new key
3. Restart gateway

## Compliance

### Encryption Standards

- AES-256-GCM (NIST-approved) for data encryption
- HKDF-SHA256 for Key Encryption Key derivation
- AES Key Wrap (RFC 3394) for DEK wrapping
- Argon2id for key fingerprinting (RFC 9106)

### Data Protection

- Sensitive data encrypted at rest
- Vault keys never persisted in plaintext
- Audit trail of all encryption operations
- Support for key rotation and deletion

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

- Verify key path is correct
- Ensure key file exists and is readable
- Check key format (32-byte hex-encoded)
- If key is lost, data is unrecoverable (by design)

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

## References

- [Vault Service](../architecture/protocol.md#vault-service)
- [Storage Services](../architecture/operator.md#storage-layer)
- [CLI Reference](../g8e-help.md#vault-commands)
- [Security Documentation](../reference/compliance-alignment.md)
