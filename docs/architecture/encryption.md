# Encryption Architecture

## Overview

g8e uses mandatory encryption for all sensitive data at rest. The encryption system is built around a vault service that provides AES-256-GCM encryption with a master key hierarchy and wrapped Data Encryption Key (DEK).

## Design Principles

- **Fail-closed**: Encryption is mandatory. Services fail to initialize without a vault.
- **Zero-knowledge**: Vault keys are never written to disk in plaintext.
- **Key rotation**: Support for re-keying without data loss.
- **Auditability**: All encryption operations are logged to the audit vault.

## Code Quality Status

### Test Coverage
- **Vault service**: Comprehensive test coverage in `internal/services/vault/vault_test.go` (930 lines)
  - Tests cover: KEK derivation, DEK generation, AES Key Wrap/Unwrap, AES-GCM encryption/decryption, vault lifecycle (init, unlock, rekey, lock, reset), concurrent access, integrity verification
  - All cryptographic primitives have dedicated test cases
  - Error paths are well-tested (invalid keys, tampered ciphertext, locked vault)
- **Storage integration**: Vault requirement tests in `internal/services/storage/vault_requirement_test.go`
  - Verifies that NewExecutionVaultService, NewTokenStoreService, and NewSQLAuditStore reject nil vault
  - Tests that locked vault operations fail appropriately
- **Integration tests**: Storage service tests use vault fixtures for end-to-end encryption validation

### Documentation Status
- ✅ Vault architecture documented
- ✅ CLI commands documented
- ✅ Configuration options documented
- ✅ Security guarantees documented
- ✅ Migration paths documented
- ⚠️ Storage service integration needs update (AuditVaultService was split in v1.0.10)

### Refactoring Opportunities

#### 1. Inconsistent Vault Integration Patterns
**Current State:**
- `SQLAuditStore`: Vault passed via `AuditStoreConfig.EncryptionVault` (required field)
- `ExecutionVaultService`: Vault passed as constructor parameter (required)
- `TokenStoreService`: Vault passed as constructor parameter (required)
- `GitLedgerService`: Vault passed via `LedgerConfig.EncryptionVault` (required)

**Smell:** Inconsistent parameter passing patterns - some use config struct, some use direct parameters.

**Refactoring Direction:**
- Standardize on config struct pattern for all storage services
- Create a common `VaultConfig` struct that can be embedded in service configs
- This aligns with the pattern used by SQLAuditStore and GitLedgerService
- Benefits: Consistency, easier configuration management, better testability

#### 2. Vault Path Configuration
**Current State:**
- Vault header path: `.g8e/vault/vault.header` (stored in dataDir)
- Vault key path: `.g8e/vault/key` (resolved relative to vault directory)
- Paths are embedded in `internal/constants/paths.go`

**Smell:** Documentation incorrectly states vault header is at `.g8e/data/vault.header`.

**Refactoring Direction:**
- Update documentation to reflect correct path (`.g8e/vault/vault.header`)
- Consider centralizing path resolution in a single utility function
- Add validation to ensure vault directory and key directory are properly secured (0700 permissions)

#### 3. Error Handling Consistency
**Current State:**
- Vault errors defined in `internal/services/vault/vault.go`
- Storage services have their own error handling for vault operations
- Some services check `vault.IsUnlocked()` before operations, others check on-demand

**Smell:** Inconsistent error handling patterns across services.

**Refactoring Direction:**
- Define common vault-related errors in `internal/constants/errors.go`
- Standardize the pattern: check vault state at service initialization, fail-fast if locked
- Add a `VaultWrapper` helper that provides consistent error wrapping for vault operations
- This aligns with the v1.0.10 consolidation of CLI & Client errors

#### 4. Thread Safety in Storage Services
**Current State:**
- Vault service uses `sync.RWMutex` for concurrent access
- SQLAuditStore uses `sync.WaitGroup` for write operations
- Other storage services have varying concurrency patterns

**Smell:** Inconsistent concurrency control across storage services.

**Refactoring Direction:**
- Audit all storage services for thread safety
- Standardize on mutex usage patterns
- Add concurrent access tests for all storage services (similar to vault's TestVaultConcurrentAccess)
- Ensure vault operations are properly synchronized in all services

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

- **Vault Header**: Metadata including key derivation parameters, wrapped DEK, and key fingerprint. Stored at `.g8e/vault/vault.header` as JSON.
- **Vault Data**: Encrypted DEK and encrypted data records (stored in vault data directory)
- **Vault Key**: Master key (32-byte hex-encoded) used to unlock the vault, stored at `.g8e/vault/key`

### Encryption Flow

1. **Vault Initialization**:
   - Generate or import master key (32-byte hex-encoded)
   - Derive Key Encryption Key (KEK) from master key using HKDF-SHA256 with info string "g8e-lfaa-kek-v1"
   - Generate Data Encryption Key (DEK) (32-byte random)
   - Wrap DEK with KEK using AES Key Wrap (RFC 3394)
   - Compute key fingerprint using Argon2id (RFC 9106)
   - Save vault header with wrapped DEK and key fingerprint to disk

2. **Data Encryption**:
   - Vault must be unlocked (DEK available in memory)
   - Generate unique nonce (12-byte) for each record
   - Encrypt data with AES-256-GCM using DEK and nonce
   - Store nonce + ciphertext

3. **Data Decryption**:
   - Vault must be unlocked
   - Read nonce from stored record
   - Decrypt ciphertext with AES-256-GCM using DEK and nonce
   - Return plaintext

## Storage Services with Encryption

All storage services require an unlocked vault at initialization:

| Service | Vault Required | Encrypted Data | Vault Integration Pattern |
|---------|---------------|----------------|-------------------------|
| SQLAuditStore | Yes | content_text, command_stdout, command_stderr fields in audit records | Config struct (AuditStoreConfig.EncryptionVault) |
| ExecutionVaultService | Yes | Execution results, command outputs, file diffs | Constructor parameter |
| TokenStoreService | Yes | Authentication tokens, session data | Constructor parameter |
| GitLedgerService | Yes | File content in ledger | Config struct (LedgerConfig.EncryptionVault) |

**Note:** The AuditVaultService was split in v1.0.10. The audit vault functionality is now integrated into SQLAuditStore with the same encryption requirements.

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

# Skip interactive confirmation (dangerous)
./g8e vault reset --confirm
```

### Export

```bash
# Export master key in hex format
./g8e vault export --key-path /path/to/vault/key

# Export with default key path
./g8e vault export
```

## Configuration

### CLI Flags

- `--vault-dir`: Directory for vault data (default: `.g8e/vault`)
- `--key-path`: Path to vault key (used in vault commands)
- `--new-key-path`: Path to save new vault key during rekey (default: `<key-path>.new`)
- `--confirm`: Skip interactive confirmation for vault reset (dangerous)
- `--key-hex`: Vault key as hex string for import command

### Environment Variables

- `G8E_VAULT_DIR`: Override vault directory
- `G8E_VAULT_KEY`: Override vault key path

### Configuration File

Vault paths are configured in the embedded paths configuration in `internal/constants/paths.go`. The default paths are:

- Vault directory: `.g8e/vault`
- Vault header file: `.g8e/vault/vault.header`
- Vault key path: `.g8e/vault/key` (resolved relative to vault directory)

These paths are resolved relative to the current working directory.

## Security Guarantees

### Data at Rest

- All sensitive data is encrypted with AES-256-GCM
- Each record uses a unique nonce to prevent key reuse
- Vault keys are never written to disk in plaintext
- Vault keys are zeroed from memory when vault is locked

### Key Management

- Master keys are 32-byte hex-encoded values
- Key fingerprints are computed using Argon2id with pepper "g8e-vault-fingerprint-v1" (16-byte output)
- Keys can be imported/exported for backup via `g8e vault export` and `g8e vault import`
- Re-keying rotates the DEK without data loss (only the DEK wrapper changes)
- Vault reset destroys all data irrecoverably

### Fail-Closed Behavior

- Services fail to initialize without a vault
- Encryption operations fail if vault is locked
- No silent fallback to plaintext storage
- Errors are logged and propagated to callers

## Implementation Details

### Vault Service

The vault service (`internal/services/vault/vault.go`) provides:

- `NewVault()`: Create new vault instance with VaultConfig
- `Unlock()`: Unwrap DEK with master key
- `Lock()`: Zero DEK from memory
- `Close()`: Lock vault and release resources
- `Encrypt()`: Encrypt data with AES-256-GCM (generates random nonce)
- `Decrypt()`: Decrypt data with AES-256-GCM (expects nonce prepended to ciphertext)
- `Rekey()`: Rotate DEK with new master key
- `GetDEK()`: Return Data Encryption Key for database operations
- `IsUnlocked()`: Check vault lock state
- `IsInitialized()`: Check if vault header exists
- `VerifyIntegrity()`: Verify vault integrity by attempting to unwrap DEK
- `Reset()`: Destroy vault and all data (requires confirmation)
- `GetDataDir()`: Return vault data directory path

### Storage Integration

Storage services integrate with vault via:

- Constructor validation: Reject nil vault with error
- Encryption checks: Verify `vault.IsUnlocked()` before encrypt/decrypt
- Error handling: Return errors on locked vault (fail-closed)

### CLI Commands

Vault management commands (`internal/cli/cmd/vault.go`):

- `init`: Initialize new vault with generated key
- `unlock`: Unlock vault with key
- `rekey`: Re-encrypt DEK with new key
- `status`: Check vault status
- `reset`: Destroy vault and all encrypted data
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

- Vault Service: `internal/services/vault/vault.go`
- Vault Header: `internal/services/vault/vault_header.go`
- Vault Cryptography: `internal/services/vault/vault_crypto.go`
- CLI Commands: `internal/cli/cmd/vault.go`
- Storage Services: `internal/services/storage/`
