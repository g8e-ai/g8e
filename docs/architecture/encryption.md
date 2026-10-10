---
doc_id: encryption
title: Encryption Architecture
audience: maintainers and coding agents
status: current
last_updated: 2026-09-28
version: v2.2.3
owners:
  - internal/services/vault/
  - internal/services/keystore/
  - internal/services/storage/
  - internal/services/scrubbing/
  - internal/services/gateway/gateway_certs.go
  - internal/services/gateway/secret_manager.go
  - internal/cli/cmd/vault/
related:
  - docs/architecture/auth.md
  - docs/architecture/governance.md
  - docs/architecture/network.md
  - docs/architecture/storage.md
  - docs/devs/troubleshooting.md
  - docs/reference/fips140-3.md
when_to_read: Implementing, operating, or auditing encryption at rest, vault lifecycle, platform keystore secret isolation, payload scrubbing, or TLS transport security.
do_not_use_for:
  - Network PKI hierarchy, trust bundles, and revocation (docs/architecture/network.md)
  - Detailed storage schema and persistence boundaries (docs/architecture/storage.md)
  - Principal identity binding and authentication routes (docs/architecture/auth.md)
  - FIPS 140-3 module compliance details (docs/reference/fips140-3.md)
---

# Encryption Architecture

## Purpose

Defines the cryptographic boundaries, key hierarchies, storage controls, and runtime lifecycle for data at rest, platform secrets, scrubbing tokens, and transport security within g8e. Describes how the encryption vault, platform keystore, scrubbing boundary, and TLS mechanisms enforce security posture across Gateway and Operator runtimes.

## Quick index

- [Purpose](#purpose)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Vault Encryption Architecture](#vault-encryption-architecture)
- [Platform Keystore Architecture](#platform-keystore-architecture)
- [Scrubbing and Execution-Site Rehydration](#scrubbing-and-execution-site-rehydration)
- [TLS and Certificate Protection](#tls-and-certificate-protection)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

Invariant groups: [Vault encryption](#vault-encryption-inv-enc-vault), [Platform keystore](#platform-keystore-inv-enc-store), [Scrubbing and rehydration](#scrubbing-and-rehydration-inv-enc-scrub), [Transport security](#transport-security-inv-enc-tls).

## Invariants

Ids are stable. Append the next free number within each group; do not renumber.

### Vault encryption (`INV-ENC-VAULT`)

| ID | Rule |
| --- | --- |
| INV-ENC-VAULT-01 | Every runtime tree maintains an independent vault. A 32-byte vault private key derives a Key Encryption Key (KEK) using HKDF-SHA256 with info `"g8e-lfaa-kek-v1"`. The KEK wraps a randomly generated 32-byte Data Encryption Key (DEK) using AES Key Wrap (RFC 3394). |
| INV-ENC-VAULT-02 | The unwrapped DEK exists only in process memory while the vault is unlocked. Calls to `Lock()` or `Close()` zero out the in-memory DEK via `SecureZero()`. |
| INV-ENC-VAULT-03 | All vault-encrypted records use AES-256-GCM with a fresh 12-byte random nonce prepended to the ciphertext (`NonceSize + len(ciphertext)`). |
| INV-ENC-VAULT-04 | Vault header verification matches the derived private key fingerprint (`SHA-256("g8e-vault-fingerprint-v1" + privateKey)[:16]`) against the header JSON before attempting DEK unwrapping. |

### Platform keystore (`INV-ENC-STORE`)

| ID | Rule |
| --- | --- |
| INV-ENC-STORE-01 | Platform secrets are encrypted at rest with a dedicated 32-byte master key stored in an OS-native keyring (`libsecret` on Linux, Keychain on macOS, DPAPI on Windows) or an explicitly provisioned external file selected by `--master-key-file` or `G8E_MASTER_KEY_FILE`. No generated plaintext fallback. |
| INV-ENC-STORE-02 | Secrets stored under `.g8e/secrets/` use AES-256-GCM authenticated encryption and JSON metadata specifying format version `1`, nonce, and ciphertext. |
| INV-ENC-STORE-03 | Keystore initialization enforces strict private filesystem permissions (`0700` for `.g8e/secrets/` and `0600` for secret files). Startup fails if master key generation or permissions enforcement fails. |

### Scrubbing and rehydration (`INV-ENC-SCRUB`)

| ID | Rule |
| --- | --- |
| INV-ENC-SCRUB-01 | The Sovereign Execution Boundary scrubs outgoing payloads with strict mode enabled by default (`StrictMode: true`), replacing sensitive tokens and credentials with irreversible labels. |
| INV-ENC-SCRUB-02 | Reversible User-Explicit Information (UEI) placeholders follow the `{{UEI_N}}` pattern. Mapped values are encrypted via the vault into the observed KV store with a 24-hour TTL. L5 Actuator rehydrates `{{UEI_N}}` placeholders immediately before action execution. |

### Transport security (`INV-ENC-TLS`)

| ID | Rule |
| --- | --- |
| INV-ENC-TLS-01 | The Gateway HTTPS listener enforces TLS 1.3. Workload and Operator ingress routes require mTLS with client certificates bound to SPIFFE URIs under the `g8e.local` trust domain. |
| INV-ENC-TLS-02 | CA private keys (Root CA, Hub CA, Operator CA, Gateway Peer CA) and Gateway serving certificate private keys are encrypted in the platform keystore. Public certificates and trust bundles persist in `.g8e/pki/`. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Vault core & AES-GCM/KW crypto | `internal/services/vault/` | `go test ./internal/services/vault/...` |
| Platform Keystore & Keyrings | `internal/services/keystore/` | `go test ./internal/services/keystore/...` |
| Secret Manager | `internal/services/gateway/secret_manager.go` | `go test ./internal/services/gateway/secret_manager_test.go` |
| Execution Vault & Diff Storage | `internal/services/storage/execution_vault.go` | `go test ./internal/services/storage/...` |
| Sovereign Execution Scrubber | `internal/services/scrubbing/` | `go test ./internal/services/scrubbing/...` |
| Encrypted KV Adapter (UEI) | `internal/services/gateway/encrypted_kv_adapter.go` | `go test ./internal/services/gateway/encrypted_kv_adapter_test.go` |
| PKI Authority & TLS | `internal/services/gateway/gateway_certs.go` | `go test ./internal/services/gateway/gateway_certs_test.go` |
| Vault CLI Commands | `internal/cli/cmd/vault/` | `./g8e vault status` |

## Vault Encryption Architecture

### Key Hierarchy and Envelope Encryption

g8e uses an envelope encryption model for vault-protected data at rest. Each runtime environment initializes one vault hierarchy:

1. **Vault Key**: A randomly generated 32-byte (256-bit) private key sealed under the master key in the platform keystore as `vault_key` (persisted under `.g8e/secrets/vault_key` using AES-256-GCM).
2. **Key Encryption Key (KEK)**: Derived from the vault key using HKDF-SHA256 (`golang.org/x/crypto/hkdf`) with salt `nil` and info string `"g8e-lfaa-kek-v1"`.
3. **Data Encryption Key (DEK)**: A randomly generated 32-byte AES-256 key (`crypto/rand`).
4. **Key Wrapping**: The DEK is wrapped by the KEK using AES Key Wrap (RFC 3394 / `aes-256-kw`).
5. **Vault Header**: Persisted to `.g8e/vault/vault.header` as JSON containing format version `1`, creation timestamp, KDF parameters, KEK algorithm identifier, base64-encoded wrapped DEK, and hex-encoded private key fingerprint (`KeyFingerprintSize = 16` bytes of `SHA-256("g8e-vault-fingerprint-v1" + vaultKey)`).
6. **Rekey Flow**: Rekeying generates a fresh vault key and rewraps the existing DEK under a new KEK, staging the new key at `.g8e/secrets/vault_key.rekey` and updating the header before atomically committing the new sealed key.

When unlocked, the unwrapped DEK remains in process memory (`Vault.dek`). Calls to `Vault.Lock()` or `Vault.Close()` clear the DEK using `vault.SecureZero()`.

### Data Encryption and Protected Surfaces

Data fields are encrypted using AES-256-GCM (`crypto/cipher.NewGCM`) with a fresh 12-byte random nonce generated per payload (`GenerateNonce()`). The 12-byte nonce is prepended directly to the GCM ciphertext before persistence.

Vault encryption protects selected high-sensitivity fields across platform services:

- **Execution Log Vault** (`internal/services/storage/execution_vault.go`): Standard output (`stdout_compressed`) and standard error (`stderr_compressed`) in `execution_log`, and file diffs (`diff_compressed`) in `file_diff_log` are encrypted with the vault before gzip compression (`sqliteutil.Compress`). Structured metadata (command, hashes, exit code, duration, file paths, session IDs) remains unencrypted for indexing.
- **Audit Event Store** (`internal/services/pubsub/vault_writer.go`): Command executions and file diff payloads are written through `VaultWriter` to the execution vault when unlocked.
- **UEI Token Store** (`internal/services/gateway/encrypted_kv_adapter.go`): Reversible User-Explicit Information (UEI) token values are encrypted via `EncryptedKVAdapter` using the vault DEK before storage in `KVStoreService` under `constants.SentinelKeyPrefix` (`"__uei_token_"`). Keys and TTL metadata remain visible.
- **File Ledger**: File snapshots managed by the file ledger service are encrypted with an `.enc` suffix when the vault is unlocked and ledger integration is active.

### Gateway Startup and Vault Lifecycle

Gateway startup reads `--vault-dir` (default: `.g8e/vault`), falling back to `G8E_VAULT_DIR`. For headless or container deployments without a usable OS keyring, configure the external master key as described below. Mode `0600` grants owner read/write; g8e treats the supplied key file as read-only input.

If no vault header exists on startup, Gateway automatically initializes a new vault header and generates the sealed vault key in the platform keystore. If an existing key cannot be read, decrypted, or matched to the header fingerprint, Gateway startup fails closed. When locked, audit, execution-vault, and UEI writes fail closed; reads return unencrypted metadata or log decryption failures. Existing installs migrating from earlier unsealed or legacy layouts must run `g8e gw clean` to recreate platform secrets and vault state.

## Platform Keystore Architecture

### Master Key and Keyring Providers

The platform keystore (`internal/services/keystore/keystore.go`) protects long-lived operational security material. It uses a dedicated 32-byte master key independent of the vault DEK.

The master key is stored using OS-native keyrings, failing closed if no protected store is available:

- **Linux**: Uses `libsecret` (`newLibsecretKeyring()`) communicating over D-Bus to the Secret Service API (e.g. `gnome-keyring-daemon`).
- **macOS**: Uses macOS Keychain (`newKeychainKeyring()`) via the OS `security` utility.
- **Windows**: Uses Windows DPAPI (`newWindowsKeyring()`) via PowerShell `System.Security.Cryptography.ProtectedData`.
- **External File**: Select with a nonblank `--master-key-file`, otherwise `G8E_MASTER_KEY_FILE`. Both values are trimmed; an empty or whitespace-only flag behaves as unset. If both are blank, g8e selects the OS keyring. A selected source that fails never falls back to another source. No home-directory filename is discovered.

The external file must contain base64 encoding of exactly 32 random bytes, be regular and at most 4096 bytes, and use an absolute path outside the actual configured runtime root. Both lexical and resolved symlink paths are checked, including symlinks in the runtime root itself. Safe external secret-mount symlinks are supported. Linux requires mode `0400` or `0600`, ownership by the service user or root, and readability by the service. Configure container secret mounts accordingly; world-readable defaults such as `0444` are rejected. Every retrieval checks the opened descriptor's metadata and validates the payload again. g8e never creates, chmods, overwrites, or deletes this external file.

Keep the containing directory and mount under trusted administration: symlink checks do not guarantee separation against path-swap races, hard links, mount aliases, or backups that include both key and ciphertext. Base64 is not encryption; protect the provisioned key and its backups separately from all ciphertext locations. Service-account or root compromise remains outside this file backend's protection.

Gateway background startup saves only the effective key **path** in its launch profile and passes it explicitly to the child. Restart reuses that path even if the invoking environment changes or disappears; a missing file fails closed. Outbound Operators use their own host's explicit path or service environment, not an implicitly forwarded developer-machine path. Vault, report, compliance, and public commands use the same selection rule; read-only keystore construction does not create keys.

OS key creation occurs only during initialization when lookup reports a missing key. It stores the new key and verifies it by reading it back and comparing in constant time; store, retrieval, and verification failures abort startup without switching backends or deleting a stored key. Backend construction performs no write probe.

Changing to a different valid key is not migration or rotation: existing ciphertext will not decrypt. Restore the original key and configuration to recover. Never overwrite an existing key to repair startup; loss of the master key loses access to its encrypted data.

g8e never generates an unencrypted file fallback key. The explicitly provisioned external key remains sensitive unencrypted input.

### Secret Storage Format

Secrets managed by `SecretManager` (`internal/services/gateway/secret_manager.go`) are written to individual JSON files under `.g8e/secrets/<name>` using AES-256-GCM. The JSON format includes `version: 1`, base64 `nonce`, and base64 `ciphertext`.

The platform keystore protects:

- Vault private key (`vault_key`, `vault_key.rekey`)
- Actuator signing key (`actuator_signing_key`) and Key ID (`actuator_key_id`)
- Auditor signing key (`auditor_signing_key`), Key ID (`auditor_key_id`), and HMAC key (`auditor_hmac_key`)
- Notary signing key (`notary_signing_key`)
- Operator private key (`operator_private_key`)
- CLI private key (`cli_private_key`)
- Session encryption key (`session_encryption_key`) and session tokens (`session_token`)
- Consensus member seeds (`consensus_member_<id>`)
- Public-feed signing keys (`public_feed_signing_key`), ingest tokens (`public_feed_ingest_token`), and rotation keys (`sealed_new_private_key`)
- Root CA, Hub CA, Operator CA, and Gateway Peer CA private keys
- Gateway serving certificate private key
- Integration API keys

Startup enforces directory permissions (`0700`) and secret file permissions (`0600`) via `Keystore.EnforcePermissions()`.

## Scrubbing and Execution-Site Rehydration

g8e combines irreversible output scrubbing with reversible execution-site rehydration:

1. **Irreversible Scrubbing**: The Sovereign Execution Boundary (`internal/services/scrubbing/boundary.go`) inspects outgoing model payloads, command output, and event streams. It redacts detected credentials, API keys, private keys, IP addresses, and personal data using fixed label replacements (e.g., `[REDACTED_API_KEY]`). Strict mode (`StrictMode: true`) is enabled by default.
2. **Reversible UEI Rehydration**: Callers register sensitive input strings to obtain a `{{UEI_N}}` token matching `regexp.MustCompile(\`\\{\\{UEI_\\d+\\}\\}\`)`. The value is encrypted into the observed KV store via `EncryptedKVAdapter` with a 24-hour expiration. Immediately prior to action execution, the L5 Actuator (`internal/services/governance/l5_actuator.go`) rehydrates `{{UEI_N}}` placeholders with original un-scrubbed values.

If a placeholder's mapped value is expired or absent, rehydration leaves the placeholder intact and logs the missing mapping without crashing.

## TLS and Certificate Protection

Network transport encryption relies on TLS 1.3 and SPIFFE identity binding managed by `PKIAuthority` (`internal/services/gateway/gateway_certs.go`):

- **CA Hierarchy**: ECDSA P-256 Root CA (`pki/root/root_ca.crt`), separate Hub CA, Operator CA, and Gateway Peer CA (`pki/authorities/`). CA private keys are stored exclusively in the platform keystore.
- **Serving Certificate**: Gateway issues a 90-day serving certificate (`pki/issued/service_cert.crt`) with DNS SANs including `g8e.local` and configured IPs. Serving private keys are stored in the platform keystore.
- **Mutual TLS (mTLS)**: Workload and Operator connections on port 8443 enforce TLS 1.3 mTLS and validate SPIFFE URI SANs (`spiffe://g8e.local/...`) against server-side session records.
- **Plain HTTP Surface**: Port 8080 serves a restricted plain-HTTP bootstrap and discovery surface.

See [Network Architecture](network.md) for PKI hierarchy details and [Authentication & Authorization Architecture](auth.md) for route authentication modes.

## Procedures

### Initializing the Vault

Generate a new vault header and seal a new vault key in the platform keystore:

```bash
./g8e vault init
# Or in headless/container environments:
./g8e vault init --master-key-file /run/secrets/g8e_master_key
```

### Unlocking and Validating the Vault

Confirm that the vault unlocks with the key held in the keystore:

```bash
./g8e vault unlock
# Or in headless/container environments:
./g8e vault unlock --master-key-file /run/secrets/g8e_master_key
```

### Checking Vault Status

Inspect whether a vault header exists and the lock state of the CLI process:

```bash
./g8e vault status
```

### Re-keying the Vault

Re-wrap the DEK with a new vault key and atomically commit the new sealed key (stop the runtime first):

```bash
./g8e vault rekey
# Or in headless/container environments:
./g8e vault rekey --master-key-file /run/secrets/g8e_master_key
```

### Resetting the Vault

Destroy vault headers and database artifacts (requires typing `destroy` or passing `--confirm`):

```bash
./g8e vault reset --confirm
```

## Anti-patterns

- **Hardcoding encryption keys**: Storing plaintext private keys or DEKs in configuration files or source code.
- **Bypassing platform keystore for CA keys**: Writing unencrypted CA or service private key files to disk outside the keystore.
- **Relying on CLI vault status for Gateway state**: Expecting `./g8e vault status` to report the running Gateway's memory state rather than the CLI process state.
- **Assuming UEI rehydration is automatic for all scrubbed data**: Expecting output scrubber labels (`[REDACTED_...]`) to rehydrate automatically at execution time.
- **Editing generated crypto code or docs out of sync**: Changing crypto algorithms without updating owning packages and documentation.

## Links out

- [Authentication & Authorization Architecture](auth.md): Route classification, identity binding, passkey enrollment, and session lifecycle.
- [Governance Architecture](governance.md): Five-layer verification interlock, L5 actuator dispatch, and receipt signing.
- [Network Architecture](network.md): PKI hierarchy, trust bundles, SPIFFE identities, and TLS surfaces.
- [Storage Architecture](storage.md): Database topology, execution vault schemas, and retention policies.
- [FIPS 140-3 Compliance](../reference/fips140-3.md): Validated cryptographic boundary, approved algorithms, and runtime verification.
- [Troubleshooting Guide](../devs/troubleshooting.md): Recovery procedures for locked vaults and keystore failures.
