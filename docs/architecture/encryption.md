# Encryption Architecture

## Overview

g8e uses mandatory encryption for all sensitive data at rest. The encryption system is built around a vault service that provides AES-256-GCM encryption with per-record keys and a master key hierarchy.

## Design Principles

- **Fail-closed**: Encryption is mandatory. Services fail to initialize without a vault.
- **Zero-knowledge**: Vault keys are never written to disk in plaintext.
- **Key rotation**: Support for re-keying without data loss.
- **Auditability**: All encryption operations are logged to the audit vault.

## Vault Architecture

### Key Hierarchy

```
Master Key (user-provided or generated)
    |
    +-- Data Encryption Key (DEK) - per-vault, rotated on re-key
        |
        +-- Per-record keys (derived from DEK + nonce)
```

### Vault Components

- **Vault Header**: Metadata including key derivation parameters, salt, and iteration count
- **Vault Data**: Encrypted DEK and encrypted data records
- **Vault Key**: Master key (Ed25519 private key) used to unlock the vault

### Encryption Flow

1. **Vault Initialization**:
   - Generate or import master key
   - Create vault header with cryptographic parameters
   - Generate DEK
   - Encrypt DEK with master key
   - Save vault header and encrypted DEK to disk

2. **Data Encryption**:
   - Vault must be unlocked (master key available in memory)
   - Generate unique nonce for each record
   - Derive per-record key from DEK + nonce
   - Encrypt data with AES-256-GCM
   - Store nonce + ciphertext

3. **Data Decryption**:
   - Vault must be unlocked
   - Read nonce from stored record
   - Derive per-record key from DEK + nonce
   - Decrypt ciphertext with AES-256-GCM
   - Return plaintext

## Storage Services with Encryption

All storage services require an unlocked vault at initialization:

| Service | Vault Required | Encrypted Data |
|---------|---------------|----------------|
| LocalStoreService | Yes | Command stdout/stderr, file diffs, content |
| AuditVaultService | Yes | Audit records, governance envelopes |
| ExecutionVaultService | Yes | Execution results, command outputs |
| TokenStoreService | Yes | Authentication tokens, session data |
| SQLAuditStore | Yes | Audit trail, compliance records |
| GitLedgerService | Optional (graceful degradation) | File content in ledger |

## Vault Lifecycle

### Initialization

```bash
# Generate new vault with auto-generated key
./g8e vault init

# Initialize with imported key
./g8e vault init --import-key /path/to/key.pem
```

### Unlocking

```bash
# Unlock vault with key file
./g8e vault unlock --key /path/to/vault.key

# Unlock with environment variable
export G8E_VAULT_KEY=/path/to/vault.key
./g8e gw start
```

### Re-keying

```bash
# Re-encrypt vault with new key
./g8e vault rekey --new-key /path/to/new.key --old-key /path/to/old.key
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
- `--vault-key`: Path to vault private key (default: `.g8e/secrets/vault.key`)

### Environment Variables

- `G8E_VAULT_DIR`: Override vault directory
- `G8E_VAULT_KEY`: Override vault key path

### Configuration File

Vault paths can be configured in `paths_default.json`:

```json
{
  "infra": {
    "vault_dir": ".g8e/vault",
    "vault_key_path": ".g8e/secrets/vault.key"
  }
}
```

## Security Guarantees

### Data at Rest

- All sensitive data is encrypted with AES-256-GCM
- Each record uses a unique nonce to prevent key reuse
- Vault keys are never written to disk in plaintext
- Vault keys are zeroed from memory when vault is locked

### Key Management

- Master keys are Ed25519 private keys
- Keys can be imported/exported for backup
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
- `Unlock()`: Decrypt DEK with master key
- `Lock()`: Zero DEK from memory
- `Encrypt()`: Encrypt data with AES-256-GCM
- `Decrypt()`: Decrypt data with AES-256-GCM
- `Rekey()`: Rotate DEK with new master key

### Storage Integration

Storage services integrate with vault via:

- Constructor validation: Reject nil vault with error
- Encryption checks: Verify `vault.IsUnlocked()` before encrypt/decrypt
- Error handling: Return errors on locked vault (fail-closed)

### CLI Commands

Vault management commands (`internal/cli/cmd/vault.go`):

- `init`: Initialize new vault
- `unlock`: Unlock vault with key
- `rekey`: Re-encrypt with new key
- `status`: Check vault status
- `reset`: Destroy vault
- `export-key`: Export master key
- `import-key`: Import master key

## Migration Path

### From Unencrypted to Encrypted

For existing deployments with unencrypted data:

1. Initialize vault: `./g8e vault init`
2. Unlock vault: `./g8e vault unlock`
3. Restart gateway: `./g8e gw restart`
4. New data is encrypted automatically
5. Use migration tool to encrypt existing data (planned)

### Key Rotation

To rotate vault keys:

1. Generate new key: `./g8e vault export-key --output new-key.pem`
2. Re-key vault: `./g8e vault rekey --new-key new-key.pem --old-key old-key.pem`
3. Update configuration to use new key
4. Restart gateway

## Compliance

### Encryption Standards

- AES-256-GCM (NIST-approved)
- Ed25519 for key derivation (RFC 8032)
- PBKDF2 for key stretching (RFC 2898)

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
./g8e vault unlock --key /path/to/vault.key

# Restart gateway
./g8e gw restart
```

### Invalid Key

If vault unlock fails with invalid key:

- Verify key path is correct
- Ensure key file exists and is readable
- Check key format (PEM-encoded Ed25519 private key)
- If key is lost, data is unrecoverable (by design)

### Vault Not Initialized

If services fail with "vault not initialized":

```bash
# Initialize vault
./g8e vault init

# Unlock vault
./g8e vault unlock

# Restart gateway
./g8e gw restart
```

## References

- [Vault Service](../architecture/g8e.md#vault-service)
- [Storage Services](../architecture/operator.md#storage-layer)
- [CLI Reference](../g8e-help.md#vault-commands)
- [Security Documentation](../reference/compliance-alignment.md)
