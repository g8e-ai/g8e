# Encryption Architecture

Last Updated: 2026-09-08
Version: v2.1.7

## Overview

g8e uses two independent encryption systems for data at rest. The vault encrypts sensitive audit, execution, ledger, and token content. The platform keystore encrypts long-lived security material such as signing keys, certificate authority keys, service certificate keys, session secrets, and integration credentials.

Network encryption is a separate control. The Gateway serves a plain HTTP discovery and bootstrap surface and a TLS 1.3 HTTPS surface. HTTPS routes apply public, browser-session, mTLS, dual-auth, or JWT authentication according to the route; protected workload routes require a verified client certificate and workload identity. See [Network Architecture](./network.md) for PKI, trust bundles, certificate lifetimes, revocation, and port topology.

All governed operations pass through the five-layer interlock after transport authentication:

1. **L1 Doctrine** applies hard gates, forbidden-pattern matching, and MITRE threat detection.
2. **L2 Consensus** verifies Ed25519 consensus signatures when the active posture requires them.
3. **L3 Notary** verifies WebAuthn or signed CLI authorization for mutations when the active posture requires it.
4. **L4 Warden** verifies signatures, replay protection, expiry, nonces, the transaction hash, and the state Merkle root before dispatch.
5. **L5 Actuator** rehydrates available protected values at the execution boundary, mints a transaction-scoped capability, dispatches the action, and produces signed receipts.

See [Governance](./governance.md) for posture-specific enforcement and [Authentication & Authorization](./auth.md) for principal and session validation.

## Vault Encryption

### Key Hierarchy

Each runtime tree has one vault. A 32-byte vault private key derives a Key Encryption Key (KEK) through HKDF-SHA256. The KEK wraps a randomly generated 32-byte Data Encryption Key (DEK) with AES Key Wrap. The vault header stores the wrapped DEK and a derived key fingerprint, while the unwrapped DEK remains in process memory only while the vault is open.

The DEK encrypts each protected value with AES-256-GCM and a fresh random nonce. Authentication failures, malformed ciphertext, a locked vault, a missing header, and an incorrect private key return errors rather than producing plaintext or accepting unauthenticated data. Closing the vault clears the in-memory DEK.

The vault private key is a separate file. Possession of the private key and vault header is sufficient to recover the DEK, so operators must back up and protect both. The key file relies on restrictive filesystem permissions and is not stored in the platform keystore.

### Protected Content

The vault encrypts these content fields before persistence:

- **Audit store:** Event content, command standard output, and command standard error. Searchable metadata such as event type, timestamps, command text, exit status, identifiers, and receipt fields remains structured in the database.
- **Execution vault:** Command standard output, command standard error, and file-diff content. Encryption occurs before compression. Execution metadata, file paths, hashes, sizes, and workflow identifiers remain structured.
- **File ledger:** Copies of governed file content are encrypted and stored with an `.enc` suffix when the ledger is enabled. Repository metadata and file-history metadata are not encrypted by the vault.
- **Scrubbing token store:** Reversible UEI token values are encrypted before entering the canonical key-value store. Token keys and expiry metadata remain visible.

These controls encrypt selected sensitive fields, not every byte in every database or runtime file. Replay nonces, suspended envelopes, state documents, SSE events, commitment records, and other structured governance data use their service-specific storage protections. See [Storage Architecture](./storage.md) for the complete persistence boundary.

### Startup and Failure Behavior

Gateway startup creates the vault header and a random private key when no header exists, then opens the vault before initializing services that require encrypted storage. The default vault directory and key path are `.g8e/vault` and `.g8e/vault/key`. `--vault-dir` and `--vault-key` override these paths; `G8E_VAULT_DIR` and `G8E_VAULT_KEY` provide environment overrides when the corresponding flags are unset.

Startup fails if an existing vault key cannot be read, decoded, or matched to the header. The audit store, execution vault, ledger, and encrypted token adapter also reject protected reads or writes while the vault is locked. Execution-vault persistence of command output and file diffs is best-effort after execution, so a persistence failure is logged but does not change the already completed action result.

Automatic initialization is convenient for a new runtime tree but is not recovery. If a vault header is missing while ciphertext from an earlier vault remains, startup creates a new key hierarchy that cannot decrypt the old content.

## Operating the Vault

### Initialize and Back Up

Run `g8e vault init` to create a header and key before the first Gateway start when explicit key custody is required. Use `--vault-dir` and `--key-path` to select paths within the active runtime tree. The command refuses to replace an existing header.

Back up the key with an approved secret-management process. `g8e vault export --key-path <path>` prints the key in hexadecimal form, so its output must be treated as secret material. `g8e vault import --key-path <path>` writes a supplied hexadecimal key but does not create or modify a vault header; the imported key must match the existing header.

### Validate Access

`g8e vault unlock --key-path <path>` opens the vault only for that command invocation and confirms that the key can unwrap the DEK. It does not unlock an already running Gateway or persist an unlocked state. The Gateway opens its own vault during startup, so change its key configuration and restart it when the key path changes.

`g8e vault status` reports whether a header exists and the lock state of the command's new process-local vault instance. It does not inspect the running Gateway, so it normally reports a configured vault as locked.

### Rotate the Private Key

Run `g8e vault rekey --key-path <current-key> --new-key-path <new-key>` while the Gateway is stopped. The command generates a new private key and re-wraps the existing DEK; it does not re-encrypt stored records. After the command succeeds, configure `--vault-key` or `G8E_VAULT_KEY` with the new path, start the Gateway, and verify encrypted reads before removing the old key backup.

Re-keying invalidates the old private key as soon as the updated header is saved. Keep the operation isolated from concurrent startup and protect the new key output because loss of the new key makes existing vault content unrecoverable.

### Reset

`g8e vault reset` requires typing `destroy`; `--confirm` skips that prompt. Reset removes the vault header and any database files located directly in the vault directory, which makes content encrypted under that header's DEK unrecoverable. It does not securely erase every ciphertext-bearing database, ledger repository, backup, or exported key, and it does not remove the configured key file.

Use reset only when abandoning the encrypted data set. Starting the Gateway afterward creates a new vault hierarchy.

## Platform Keystore

The platform keystore has its own random 32-byte AES-256-GCM master key and does not use the vault DEK. It stores each encrypted secret as an authenticated ciphertext with its nonce and format version. Gateway startup retrieves or creates the master key, enforces private permissions on the secrets directory, and validates the required bootstrap secrets before continuing.

The keystore protects:

- Actuator, Auditor, Notary, Operator, and CLI signing material managed by the Gateway.
- Root, Hub, Operator, and Gateway Peer CA private keys.
- Gateway service-certificate private keys and other managed service keys.
- Session encryption material and stored session tokens.
- Auditor authentication material.
- API keys stored for external integrations.

Public key identifiers and public certificates are not secrets. Client and workload enrollment keys generated outside the Gateway remain under the custody of the component that generated them.

### Master-Key Storage

- **Linux:** The keystore uses libsecret when available and falls back to a file in the runtime secrets directory when libsecret is unavailable.
- **macOS:** The keystore uses Keychain and fails startup if Keychain initialization fails.
- **Windows:** The keystore uses a file in the runtime secrets directory.

The file keyring stores the base64-encoded master key with private permissions and replaces it atomically. Because the file master key and encrypted secret files occupy the same runtime tree, this fallback protects against casual disclosure and partial-file exposure but does not provide cryptographic separation from an attacker who can read the complete secrets directory. Deployments that require OS-backed non-file key custody use libsecret on Linux or Keychain on macOS and protect runtime backups accordingly.

Deleting or losing the keystore master key makes keystore-encrypted platform secrets unavailable. On later startup, an absent key may cause a new master key to be generated, but that new key cannot decrypt ciphertext produced with the previous key.

## Scrubbing and Execution-Site Rehydration

Scrubbing reduces sensitive content returned from governed reads and downstream tool calls. The active scrubber replaces recognized credentials, private keys, tokens, personal data, network identifiers, and sensitive key-value fields with non-secret labels. Strict mode is enabled by default, output is bounded, and disabling scrubbing suppresses output rather than returning raw content.

Reversible UEI placeholders are a separate facility. When a caller explicitly registers a value, the scrubbing service assigns a `{{UEI_N}}` placeholder and persists the encrypted mapping for 24 hours. L5 recursively replaces mapped placeholders in text or JSON payload strings immediately before dispatch, keeping the original value out of earlier reasoning and transport stages.

The current automatic output-scrubbing paths use irreversible labels; they do not automatically convert every detected secret into a UEI placeholder. If a payload contains a placeholder whose mapping is absent or expired, rehydration leaves that placeholder unchanged and dispatch can continue. Operators must therefore treat UEI rehydration as an explicit caller-managed workflow, not as a general guarantee that every redacted value is restored.

Scrubbed observed-state evidence and encrypted raw execution content serve different purposes. Scrubbing limits disclosure across component boundaries, while vault encryption protects selected persisted content. Neither control replaces route authentication, governance authorization, or host access controls.

## TLS and Certificate Protection

The Gateway creates or loads an ECDSA P-256 Root CA, separate Hub, Operator, and Gateway Peer intermediate CAs, and a 90-day serving certificate. CA and serving private keys are encrypted through the platform keystore; public certificates and trust bundles are written to the PKI tree.

The HTTPS listener requires TLS 1.3 and verifies a client certificate when one is presented. Route middleware then enforces the route's authentication mode. Public bootstrap and browser routes can complete TLS without a client certificate, browser routes can use a secure web session, protected workload routes require mTLS, and configured MCP or A2A ingress can use JWT authentication. The plain HTTP listener exposes only its restricted discovery, bootstrap, and distribution router.

The Gateway checks certificate revocation and SPIFFE identity and session binding on authenticated requests. It renews a missing or near-expiry serving certificate and regenerates it when newly detected DNS names or IP addresses are absent from its SANs. See [Network Architecture](./network.md) for the current certificate hierarchy and transport behavior, and [Authentication & Authorization](./auth.md) for enrollment, rotation, and session checks.

## Cryptographic Compliance

Build-time module selection, runtime approved-mode reporting, strict enforcement, validated algorithms, excluded algorithms, and operating-environment limits are maintained in [FIPS 140-3 Compliance](../reference/fips140-3.md). Run `g8e version --fips` against the deployed binary to inspect its actual approved-mode and enforcement state. Do not infer strict enforcement from the presence of the linked module alone.

## Troubleshooting

### Gateway Cannot Open the Vault

1. Confirm that the configured vault directory contains the expected header.
2. Confirm that `--vault-key` or `G8E_VAULT_KEY` points to the matching 64-character hexadecimal key file.
3. Run `g8e vault unlock --vault-dir <dir> --key-path <path>` to validate the pair outside the Gateway process.
4. Restore the matching header and key from backup if either was replaced. A newly generated key cannot recover old ciphertext.

### Keystore Secrets Cannot Be Decrypted

Confirm that the operating-system credential store is available under the same account that initialized the Gateway. On Linux or Windows file-keyring deployments, restore the matching master-key file and encrypted secret files together. Recreating only the master key does not recover existing secrets.

### TLS Authentication Fails

Check the server and client certificate validity, the current Gateway trust bundle, certificate revocation state, and the SPIFFE identity's session or application policy. Public HTTPS reachability does not prove that an mTLS-protected route accepts the presented workload identity. See [Authentication & Authorization](./auth.md) for recovery and rotation workflows.

## Related Documentation

- [Storage Architecture](./storage.md): Persistence services, encrypted fields, retention, and audit flows.
- [Network Architecture](./network.md): PKI hierarchy, TLS surfaces, trust bundles, SPIFFE identities, and revocation.
- [Authentication & Authorization](./auth.md): Enrollment, sessions, route authentication, and identity binding.
- [Governance](./governance.md): Five-layer verification and posture behavior.
- [FIPS 140-3 Compliance](../reference/fips140-3.md): Validated boundary, operating environment, and runtime verification.
