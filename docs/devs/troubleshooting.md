# Developer Troubleshooting

Last Updated: 2026-08-25
Version: v2.0.0

This page covers common setup failures, runtime friction, and operational caveats for contributors working on g8e from a fresh checkout. The platform runs host-native. For architecture-level context, see [Authentication & Authorization](../architecture/auth.md), [Encryption Architecture](../architecture/encryption.md), [Gateway Architecture](../architecture/gateway.md), [Governance](../architecture/governance.md), and [Network Architecture](../architecture/network.md).

## First checks

Run commands from the repository root:

```bash
pwd
ls README.md g8e Makefile
```

Use a POSIX shell such as Linux, macOS Terminal, WSL, or Git Bash. The `Makefile` uses Bash, `sed`, and `curl`. The `g8e` binary at the repository root is a compiled Go executable; it does not depend on shell utilities beyond the system libc.

At minimum, install the tools for the component you are touching:

- Go 1.26.6 or later for the g8e Operator and protocol work.
- Python 3.10 or later for protocol generation and demo scripts.

## `make` targets fail with missing `curl`

The `Makefile` uses `curl` to download Buf during `make buf-install` and in demo scripts. If `curl` is not installed, install it and retry:

```bash
command -v curl
make proto
```

If the command exists in one terminal but not another, fix the shell `PATH` before changing project files.

> Note: To support sovereign, agnostic, and air-gapped deployments, `jq` is completely eliminated as a host dependency. All JSON parsing and request assembly are handled internally by the Go CLI, allowing g8e to run on virtually any modern Linux environment without extra system package requirements.

## `make proto` fails before generating files

`make proto` runs `make buf-install`, then calls Buf to generate Go Protobuf code from the schema definitions in `protocol/proto/`.

Check the local prerequisites first:

```bash
command -v go
command -v buf
```

The `buf-install` target attempts to provision Buf in the following order:

1. If Go is present on the host, it installs Buf via `go install github.com/bufbuild/buf/cmd/buf@v1.70.0`.
2. If Go is absent, it attempts to download the pre-compiled binary from Buf releases using `curl`.
3. If neither succeeds, `make proto` exits with an error. The pre-generated `.pb.go` files committed under `protocol/proto/` allow `go build` to succeed without running `make proto`, but schema changes require a working Buf installation.

If you are modifying `.proto` files in an offline environment, ensure that `buf` is installed globally on your path before running `make proto`.

For Python protocol generation, use the separate target:

```bash
make proto-python
```

## `./g8e gw start` does not become healthy

The `gw start` command launches the g8e Gateway as a background process via `gateway serve`, then waits for the process to become healthy. Start with the status command and the log:

```bash
./g8e gw status
./g8e gw logs
```

Common causes:

- One of the local ports from `protocol/constants/ports.json` (HTTP 8080, HTTPS 8443) is already in use. The process manager in `internal/cli/platform/process.go` performs a preflight port check and reports conflicting PIDs.
- The Go toolchain is missing or below the version expected by the current Developer Guidelines (Go 1.26.6).
- Runtime PKI or secrets were created by an older incompatible checkout.
- Port collision prevention: the gateway fails startup if multiple logical surfaces are assigned to the same port, ensuring no downgrade of the mTLS execution boundary. The HTTP surface (8080) serves plain HTTP for bootstrap and PKI discovery only; the HTTPS surface (8443) handles all API, MCP, console, and management routes. See [Network Architecture](../architecture/network.md) for port topology details.
- Governance posture validation: when starting in `consensus` or `notary` posture, the gateway validates consensus prerequisites at startup before any services start. If the consensus ID is empty, the consensus policy does not exist in the database, or quorum is less than 1, the gateway exits with an error. See [Governance](../architecture/governance.md) for posture startup validation details.

Stop the managed process before retrying. Use `gw restart` as a shortcut, or stop and start manually:

```bash
./g8e gw restart
```

```bash
./g8e gw stop
./g8e gw start
```

Use `./g8e gw reset` or `./g8e gw clean` only for disposable local state. They intentionally remove runtime data under `.g8e/`.

If the gateway started but the HTTPS health endpoint reports `governance_ready: false`, the gateway is running in `consensus` or `notary` posture and no trusted L2 signers are registered. In `doctrine` posture, `governance_ready` is always `true`. To resolve, register trusted signers or switch to `doctrine` posture. See [Governance](../architecture/governance.md) for signer registration details.

## Tests fail because the gateway is not running

The test suite uses a tiered structure with different infrastructure requirements:

- **Tier 1 (Unit tests)**: Run immediately without external dependencies via `make test-unit`.
- **Tier 2 (In-Process Integration)**: No external dependencies. Integration tests use in-process gateway fixtures (`test/fixtures/gateway_fixture.go`) that spin up the gateway within the test process. Run via `make test-integration`.
- **Tier 3 (Docker E2E)**: Requires a running platform. Start the platform first (`docker compose up` or `./g8e gw start`), then run `make test-docker` or `./g8e test e2e`. The test binary connects to the running platform and fails fast if it is not reachable. Use `./g8e test e2e --run <pattern>` to select specific scenario tests that require particular platform states (pending, denied, headless, approved-restart).

Tier 2 integration tests do not require a running external gateway. They construct the gateway in-process via `GatewayFixture`, which handles PKI enrollment and mTLS configuration automatically. If these tests fail, the cause is typically a port conflict or missing build dependencies, not a missing gateway process.

If a test failure mentions missing trust bundles or client certificates, confirm that the test fixture has not been modified to skip enrollment. The `EnrollClientIdentity` helper in `test/fixtures/gateway_fixture.go` generates test PKI material at runtime.

### Tier 3 E2E preflight failures

Tier 3 E2E tests (`./g8e test e2e` or `make test-docker`) require a running platform. The test binary's `TestMain` performs a bounded HTTP health check against the gateway and exits non-zero with `FATAL: E2E preflight failed` if the platform is not reachable. There is no skip and no false-green.

Common causes:
- The platform is not running. Start it first: `docker compose up` or `./g8e gw start`.
- The gateway is running but the health endpoint is not responding. Check `./g8e gw status` and `./g8e gw logs`.
- Owner credentials are missing or expired. Run `./g8e auth enroll user` to obtain a fresh CLI session. The test binary reads the owner CLI certificate, key, and session ID from the local `.g8e/` runtime tree.
- The CLI session has expired (7-day TTL). Authenticated tests fail with `CLI session expired` (401). Re-enroll to obtain a fresh session.

### Tier 3 scenario selection

Stateful E2E tests require specific platform states. Use `./g8e test e2e --run <pattern>` to select the tests that match the current platform state:

- `TestGateway|TestEnsemble|TestDashboard` — public-surface tests, any running platform with the full stack
- `TestAuth|TestOperatorRegistry|TestPubSub|TestCommandRoundtrip|TestCompliance` — authenticated tests, approved stack with valid owner session
- `TestPlatformEnrollment_PendingDiscovery` — full stack running, owner bootstrapped, no approvals
- `TestPlatformEnrollment_Denial` — full stack running, owner bootstrapped, no approvals
- `TestPlatformEnrollment_RestartDuringPending` — full stack running, owner bootstrapped, no approvals, operator restarted by user before running
- `TestPlatformEnrollment_Headless` — gateway only, owner bootstrapped, no operator/dashboard/ensemble
- `TestApprovedRestart` — full approved stack, operator restarted by user before running

## Vault and encryption failures

All sensitive data is encrypted at rest using AES-256-GCM via the vault subsystem. Services that handle sensitive content (audit store, execution vault, token store, ledger) fail closed when the vault is locked or not initialized. See [Encryption Architecture](../architecture/encryption.md) for vault lifecycle details.

### Vault not initialized

If services fail with "vault not initialized":

```bash
./g8e vault init
./g8e vault unlock --key-path .g8e/vault/key
./g8e gw restart
```

### Vault locked

If services fail with "vault is locked":

```bash
./g8e vault status
./g8e vault unlock --key-path /path/to/vault/key
./g8e gw restart
```

Without a vault key, the gateway starts but storage services fail closed on first use. Provide the vault key via `G8E_VAULT_KEY` or `--vault-key` to enable decryption.

### Invalid or lost vault key

If vault unlock fails with an invalid key error:

- Verify the key path is correct and the key file is readable.
- Check that the key is a 32-byte hex-encoded value.
- If the key is lost, all encrypted data is unrecoverable. The only option is `./g8e vault reset --confirm`, which destroys the vault and all encrypted data, followed by `./g8e vault init` and a gateway restart.

### Platform keyring fallback

The keystore uses the OS-native credential store when available, with a file-based fallback. On Linux, GNOME Keyring via libsecret is used when present; otherwise, the master key is stored as a base64-encoded file with restrictive permissions. On macOS, Keychain is used. On Windows, only the file-based fallback is available. If the keyring service is unavailable, check the file-based fallback path in `.g8e/`.

## Authentication failures after gateway start

The gateway requires explicit authentication before it can be used. After starting the gateway, enroll to bootstrap your credentials:

```bash
./g8e gw start
./g8e auth enroll user
```

If authentication fails, check the following:
- Ensure the gateway is running via `./g8e gw status`.
- Verify the external IP displayed during gateway start matches your network interface.
- For passkey authentication, ensure your hardware security key or platform authenticator is available.
- For certificate-based authentication, ensure `.g8e/cli.crt` and `.g8e/cli.key` exist. The `auth enroll user` command generates these files via the `EnrollmentCoordinator`, which drives CSR-based enrollment with the gateway CA. On all platforms (including Windows), CLI keys are file-backed EC P-256; the `--tpm` flag was removed in v1.7.2.

### CLI recovery (new CLI against an existing gateway)

If you are enrolling a new CLI against an already-bootstrapped gateway (e.g. a second workstation, or replacing a lost CLI), `auth enroll user` detects that the gateway is already bootstrapped and uses the **recovery flow** instead of bootstrap. The recovery flow requires a one-time human approval from an existing enrolled user, via either a browser or an already-enrolled CLI:

1. The new CLI posts a CSR to the gateway and receives an opaque one-time token plus a browser approval URL.
2. An existing user approves (or denies) the request via one of two paths:
   - **Browser path (default):** open the approval URL in a browser and approve via the Console SPA (`POST /api/v1/auth/cli/recovery/approve`, web-session protected).
   - **Headless path (`--headless`):** the new CLI prints `g8e auth approve-recovery <token>` for an already-enrolled CLI to run instead of opening a browser. The approver CLI posts to `POST /api/v1/auth/cli/recovery/approve-cli` (mTLS protected); the approver user ID is derived from the verified mTLS certificate URI SAN.
3. The new CLI completes the recovery by proving possession of the CSR private key (signing the request ID) and receives a new CLI certificate.

If recovery fails:
- The token expires after a bounded TTL. If the approving user does not act in time, re-run `auth enroll user` to get a fresh token.
- The opaque token is only returned once. If you lose it, re-run `auth enroll user`.
- On the browser path, the approving user must have an active web session (cookie-based auth). If approval fails with 401, the approving user should re-authenticate in their browser.
- On the headless path, the approver CLI must hold a valid, non-revoked CLI certificate bound to an active user. A revoked cert is rejected by the mTLS middleware (401) before the handler runs; a deactivated user is rejected by the handler (403).
- Recovery is not available on an unbootstrapped gateway. Use the bootstrap endpoint instead.

### `--headless` flag

`--headless` on `auth enroll user` opts into a CLI-only identity that completes enrollment without a browser. It skips the passkey ceremony and OS trust installation (the `--no-system-trust` behavior is implied), and on the recovery branch it prints `g8e auth approve-recovery <token>` for an already-enrolled CLI to run instead of opening a browser. The resulting identity is mTLS-only: it can do everything the CLI could do before (MCP, A2A, governance, SSE, rotation) but cannot authenticate to the Console SPA because no browser passkey was registered. A browser passkey can be registered later from a browser if console access is desired.

`--headless` is distinct from the internal `SkipPasskey` field used by `mcp agent run` and demos: those callers set `SkipPasskey` directly and must NOT set `Headless`, because `Headless` also changes recovery output (printing the approve-recovery command instead of opening a browser), which those callers do not want.

### `--no-system-trust` flag

`--no-system-trust` skips the OS trust store **installation** step. It is an **administrator-managed trust opt-out**, not a headless or passkey bypass. Use it only when an administrator has already installed the gateway root CA into the OS trust store. The passkey ceremony still runs. If you use `--no-system-trust` without pre-installing the root CA, browser-based WebAuthn will fail because the browser will not trust the gateway's TLS certificate.

As of v1.7.2, **stale-anchor detection still runs under `--no-system-trust`** — only the installation step is skipped. The user may have stale g8e root anchors from a previous gateway instance (e.g., after `gw clean`) that break the browser even when the CLI skips installation. When stale anchors are found, the removal prompt fires; on confirmation the stale anchors are removed and the blocking browser-restart gate fires (the user must close all browser windows and press Enter before the passkey ceremony), but no new anchor is installed.

### Browser CA trust and WebAuthn failures

The gateway uses self-signed certificates. Browser-based WebAuthn registration and console access require the platform Root CA to be trusted by the operating system. The `auth enroll user` command installs the Root CA into the OS trust store before opening the browser for the passkey ceremony. If automatic OS trust installation fails, `auth enroll user` stops before opening the browser and returns actionable remediation. Use `--no-system-trust` only when an administrator has already installed the Root CA on the host; it does not skip the passkey ceremony.

After installing the Root CA (or removing stale anchors), `auth enroll user` **blocks until the user closes all open browser windows and presses Enter**. Browsers cache certificate trust state, and WebAuthn registration will fail if the browser does not yet recognize the new platform CA. The blocking prompt ensures the browser restart happens before the passkey ceremony opens a fresh browser session. Firefox and other browser-private trust stores may require separate handling. This is the most common cause of console access failures on fresh setups.

### Certificate expiry and rotation

Leaf certificates (operator, CLI, app) have a 7-day validity period. If authentication fails with a certificate-related error, the CLI certificate may have expired. The `EnrollmentCoordinator` automatically detects expiring CLI certificates (within 24 hours of expiry) and rotates them via the mTLS-protected rotation endpoint (`/api/v1/auth/cli/rotate`).
Rotation is a single transactional replacement: the new certificate is
signed before the old session is deactivated, and the old certificate is revoked after the session replacement commits.

To force rotation of a healthy CLI certificate (e.g. after a key compromise concern), use the `--rotate-cli` flag:

```bash
./g8e auth enroll user --rotate-cli
```

If the CLI certificate has already expired and rotation is not possible, re-enroll from scratch:

```bash
./g8e auth logout && ./g8e auth enroll user
```

The gateway serving certificate has a 90-day validity period and is auto-rotated by the PKI authority. Operator certificates are auto-renewed by `RunClientCertRenewalLoop` in `internal/cli/serve/cert.go`, which periodically re-enrolls via the device-enroll handler before expiry.

### g8e.local DNS resolution

The platform uses `g8e.local` as the default hostname for gateway connections. If `g8e.local` does not resolve via system DNS, the CLI automatically falls back to the machine's external interface IP. This fallback is implemented in `internal/cli/cmd/mcp.go`. No `/etc/hosts` changes or DNS configuration are required for basic operation.

### Session TTL

Operator sessions have a 1-hour TTL by default. CLI sessions have a 7-day TTL, aligned with the CLI certificate validity period. If CLI commands fail with session-related errors after extended idle periods, re-enroll to obtain a fresh session. Operator sessions require a gateway restart to refresh.

## L3 approval timeouts and transaction rejection

Under `notary` posture, mutation actions require L3 human authorization. The approval flow has two time windows that can cause transaction rejection:

- **Request window (2 minutes)**: The passkey ceremony must be completed within 2 minutes of the transaction being suspended. If the passkey approval is not completed within this window, the request expires and the action must be retried.
- **Dispatch window (30 minutes)**: After approval, the transaction must be dispatched within 30 minutes. Transactions not dispatched within that window must be re-approved.

The CLI SSE client in `internal/cli/auth/approval_sse.go` waits for `approval.completed` events with a 3-minute timeout, which covers the 2-minute gateway request window plus margin. If the SSE wait times out, the CLI reports the failure and the transaction must be resubmitted.

Common causes of approval failures:
- The user did not complete the passkey ceremony in time.
- The browser was not restarted after the Root CA was installed by `auth enroll user` (`auth enroll user` now blocks until the user closes all browser windows and presses Enter, but if the user dismissed the prompt or restarted into a stale browser session, the WebAuthn ceremony will fail with a TLS error).
- The SSE stream was not accessible due to network or authentication issues.
- The gateway posture was changed to `notary` without a configured L3 notary.

See [Authentication & Authorization](../architecture/auth.md) for the full approval flow and [SSE Streaming](../architecture/sse.md) for SSE event delivery details.

## Governance posture startup failures

The gateway posture is set at startup via `--posture <doctrine|consensus|notary>` and cannot be changed at runtime. Each posture has different startup requirements:

- **Doctrine** (default): No consensus prerequisites. L2 and L3 are audited but not enforced.
- **Consensus**: Requires a consensus policy to exist in the database with quorum >= 1. The Consensus service is bootstrapped in-process.
- **Notary**: Same as consensus, plus L3 notary must be configured for mutation actions to succeed.

If the gateway fails to start in `consensus` or `notary` posture, check:

- The consensus ID is non-empty.
- The consensus policy exists in the database and is enabled.
- Quorum is >= 1 (valid for single-member ensembles).
- For `notary` posture, the L3 notary is available. Without it, mutations fail closed.

For declarative consensus seeding, use the `--consensus-bootstrap` flag with a JSON config file containing `consensus_id`, `member_app_ids`, `quorum`, and optional `seed_hex`. This is idempotent: if the consensus already exists, the bootstrap is skipped.

Consensus members whose keys cannot be resolved during bootstrap are included without a private key and a warning is logged. They can participate in policy but cannot sign votes. If all members lack keys, L2 deliberation produces no votes and quorum fails.

See [Governance](../architecture/governance.md) for posture definitions and consensus bootstrap details.

### Governance envelope rate limiting

The governance envelope submission endpoint (`POST /api/v1/governance/envelopes`) is rate-limited. If submissions fail with HTTP 429, reduce the submission frequency or batch multiple actions into fewer envelopes.

### Outbound mode mutations fail closed

The default posture for outbound (operator) mode is `doctrine`. When the operator is started in `notary` posture, mutations require L3 human authorization. The outbound L3 notary (`governance.NewOutboundL3Notary` in `internal/services/governance/l3_notary.go`) is not nil in outbound mode: it suspends mutation transactions and requires CLI-based approval via the suspended transaction store. If operators reject all mutations with L3-related errors under `notary` posture, either complete the CLI approval flow or switch the operator to `doctrine` or `consensus` posture.

The following action types are classified as mutations and require L3 proof under notary posture: `A2A_CALL`, `CANCEL`, `DOCUMENT_DELETE`, `DOCUMENT_UPDATE`, `EXECUTE_BASH`, `FILE_EDIT`, `MCP_CALL`, `PLATFORM_ENROLLMENT_CREATE_SESSION`, `PLATFORM_ENROLLMENT_DECIDE`, `PLATFORM_ENROLLMENT_ISSUE`, `PLATFORM_ENROLLMENT_PERSIST_POLICY`, `RESTORE_FILE`, `SHUTDOWN`. Non-mutation actions (e.g., `FS_READ`, `FS_LIST`, `FETCH_LOGS`) do not require L3 proof even under notary posture.

## State Merkle root mismatch

The L4 Warden validates that the `state_merkle_root` in the GovernanceEnvelope matches the current state root of the gateway or operator. A mismatch causes the transaction to be rejected as stale.

Common causes:
- Concurrent transactions modifying shared state between envelope creation and submission.
- The gateway was restarted between envelope creation and submission, causing the state root to change.
- The operator's git ledger HEAD changed due to external file modifications.

If this occurs frequently, ensure envelopes are submitted promptly after creation. The state root is derived from the git ledger HEAD commit hash on the operator and from the SQLite state version on the gateway. See [Storage Architecture](../architecture/storage.md) for state root details and [Governance](../architecture/governance.md) for Warden validation logic.

## Path resolution problems

The CLI resolves the project root using `config.FindProjectRoot()` in `internal/config/config.go`, which returns the current working directory (`os.Getwd()`). Run commands from the project root directory to ensure correct path resolution:

```bash
cd /path/to/g8e
./g8e gw status
```

### Cross-platform path handling

All storage services construct filesystem paths through a shared path utility layer that prevents a Windows-specific double-join issue: when two absolute paths are joined with standard library functions, the result is an invalid concatenated path (for example, `C:\temp\C:\temp\data.db`). The utility layer detects absolute paths in the joined elements and uses them as-is.

Configuration paths for databases and directories can be either relative or absolute. Relative paths are resolved against a base data directory. Absolute paths are respected and used without modification, allowing operators to place individual databases on separate volumes or drives.

The ledger additionally strips Windows drive letters and leading separators before constructing ledger-relative paths, ensuring that file history is consistent across platforms. See [Storage Architecture](../architecture/storage.md) for cross-platform path handling details.

### Ledger file size limit

The ledger enforces a 100 MB size limit on encrypted file copies to prevent out-of-memory during the full-read required by AES-256-GCM. Files larger than 100 MB cannot be encrypted and copied to the ledger. Unencrypted file copies are streamed to avoid loading entire files into memory. If a file operation fails with a size limit error, the file exceeds the encrypted copy cap.

## `./g8e` command not found

The `g8e` file at the repository root is a compiled Go binary. If you receive "command not found", ensure you are running from the repository root and the binary has execute permissions:

```bash
ls -l g8e
chmod +x g8e
./g8e gw status
```

If the binary is missing or outdated, rebuild it:

```bash
make build
```

The `make build` target compiles `cmd/g8e` and copies the resulting binary to the repository root as `g8e`. The target handles Windows builds natively, producing `g8e.exe` when run on Windows.

## SSE event delivery issues

The SSE streaming infrastructure provides real-time event delivery from app workloads to browser and CLI clients. Events are stored in the `sse_events` table and routed by `web_session_id`, `cli_session_id`, or `user_id`. See [SSE Streaming](../architecture/sse.md) for full details.

### Events not received

If SSE events are not reaching clients:

- Verify the client is authenticated. SSE consumer endpoints (`/api/v1/sse/events`, `/api/v1/sse/stream`) require dual auth: mTLS for CLI/operator clients, or web session cookie for browser clients.
- Check that the routing identifier (`web_session_id`, `cli_session_id`, or `user_id`) matches the authenticated session. The `authorizeSSERoute` helper enforces ownership checks.
- SSE endpoints are only available on the HTTPS port (8443), not on the HTTP bootstrap port (8080).

### Event drops under high load

The SSE stream uses a bounded drop-oldest buffer of 100 entries per subscriber. If the buffer is full when an event arrives, the oldest queued event is dropped (not the incoming event) with a back-pressure warning log. Consumers can recover evicted events via DB replay on reconnect. Consider batching events before pushing to reduce database load and buffer pressure.

### Event retention

The `sse_events` table is pruned automatically by the gateway maintenance loop every 30 seconds with a 1-hour retention window. Events older than 1 hour are deleted. If historical events are needed beyond 1 hour, query them before they expire.

### State root impact

SSE event inserts do not alter the state root. This is intentional to allow high-frequency event streaming without governance overhead. Events are considered ephemeral telemetry, not governance state.

## CORS and cross-origin session issues

Browser-based frontends connecting to the gateway require explicit CORS configuration. If API calls return 401 despite being logged in, or the browser console shows `Access-Control-Allow-Origin` errors, the gateway was not started with `--cors-origin` matching the frontend origin.

Restart the gateway with the correct origin:

```bash
./g8e gw start --cors-origin https://your-app.example.com --passkey-rp-origin https://your-app.example.com
```

Ensure every `fetch` call includes `credentials: 'include'`. The gateway sets `SameSite=None` on session cookies only when `--cors-origin` is configured, which is required for cross-origin cookie delivery.

For SSE connections from browser clients, construct `EventSource` with `withCredentials: true`. Without authenticated session cookies, the SSE endpoints return 401. Verify the `web_session_id` is valid via `GET /api/v1/auth/sessions/me`. See [Build a g8e-Compatible Frontend](../guides/build_frontend.md) for the full browser integration flow.

## WebAuthn passkey RP ID mismatch

If the WebAuthn ceremony fails with "RP ID is not a valid domain" or similar, the `--passkey-rp-id` flag does not match the frontend app's registrable domain. The RP ID must be a registrable domain suffix of the current page's origin. For example, when accessing the gateway via `console.g8e.ai`, the RP ID must be `console.g8e.ai`, not `localhost`.

When accessing via a Cloudflare tunnel, set `--passkey-rp-id` to the tunnel hostname. See [Cloudflare Tunnel Integration](../guides/cloudflare_tunnel.md) for tunnel-specific configuration.

## Stale trust bundle after PKI regeneration

If the gateway PKI is regenerated (`gw clean`, PKI rotation, or gateway migration to a new host with a fresh CA) while a workstation holds a complete CLI identity from the old gateway, the local trust bundle and OS trust store are both stale in lockstep. Re-running `auth enroll user` previously failed with a raw TLS error during the passkey ceremony:

```
tls: failed to verify certificate: x509: certificate signed by unknown
  authority (possibly because of "x509: ECDSA verification failure"
  while trying to verify candidate authority certificate "g8e Root CA")
```

…with no diagnosable cause, because the coordinator trusted the local bundle as the source of truth and the OS store matched it.

As of v1.7.2, `auth enroll user` now fetches the **live** gateway root CA from the unauthenticated discovery endpoint (`GET /.well-known/g8e/pki/ca-bundle` on the plain-HTTP port) before the reuse decision. When the live root fingerprint does not match the local bundle, the coordinator automatically:

1. Prints `Local trust bundle does not match the live gateway root CA; using recovery flow.`
2. Routes to the **recovery flow** (human-approved, plain-HTTP, token-scoped), which issues a fresh CLI certificate signed by the new CA. Rotation is impossible here because the old CLI cert cannot authenticate to the new gateway via mTLS.
3. Detects the stale OS root anchor using the **live** fingerprint (not the stale local one), prompts for removal, and reinstalls the new root CA.

So in the common case you no longer need to log out first — just re-run `auth enroll user` and approve the recovery in the browser:

```bash
./g8e auth enroll user
```

If the discovery endpoint is unreachable (e.g., the gateway is only reachable on the HTTPS port, or you are intentionally offline), the coordinator prints a diagnostic warning naming the `gw clean` scenario and the `--endpoint` flag, then proceeds. If the bundle is in fact stale, the subsequent mTLS call surfaces a TLS error — but with prior context. In that case, fall back to the manual flow:

```bash
./g8e auth logout && ./g8e auth enroll user
```

Note: `auth logout` removes local CLI credential material (CLI cert,
CLI key, credentials JSON) but does **not** remove the shared OS root CA. This is intentional — the OS root CA is a shared system resource that may be trusted by other applications. If the root CA itself is stale (PKI was regenerated) and the automatic recovery routing did not remove it, remove it manually from the OS trust store before re-enrolling, or rely on `auth enroll user` which will detect the stale anchor via the live fingerprint and prompt for removal.

## Cloudflare tunnel issues

When exposing the gateway via a Cloudflare tunnel, additional failure modes apply beyond local gateway troubleshooting. See [Cloudflare Tunnel Integration](../guides/cloudflare_tunnel.md) for full setup instructions.

### 502 Bad Gateway

The tunnel is connected but the gateway is not running or not listening on `localhost:8443`. Verify the gateway health endpoint and tunnel status:

```bash
curl -sk https://localhost:8443/api/v1/health
g8e gw tunnel status --hostname console.g8e.ai --name g8e
```

### DNS record conflict

If DNS routing fails with "record already exists," use `--skip-dns` with `g8e gw tunnel create` to skip the DNS step. Alternatively, delete the old DNS record in the Cloudflare dashboard or use a different hostname.

### Tunnel version outdated

If the tunnel exhibits unexpected behavior, check and update `cloudflared`:

```bash
cloudflared --version
```

If installed via dpkg, download and install the latest release from the Cloudflare GitHub releases page.

## Receipt signature verification

The Gateway signs `ActionReceipt`s with its Actuator Ed25519 private key. The actuator public key is exported to the PKI directory during gateway boot in both PEM and JSON formats.

No mechanism exists for distributing the Gateway's public key to Engine instances via an attested channel. Consumers that need to cryptographically verify receipt authenticity must obtain the public key out-of-band by reading the exported files from the gateway PKI directory. The g8e Python package does not currently expose a receipt verification utility. Consumers can implement Ed25519 verification using `nacl.signing.VerifyKey` with the exported public key.

See [Encryption Architecture](../architecture/encryption.md) for receipt signature details.

## Docker demo issues

### Demo port conflicts

If multiple demos are running simultaneously, port conflicts may occur. Check which demos are running:

```bash
docker compose ps
```

### Metabase startup failure (healthcare demo)

Metabase requires the `reporting-db` PostgreSQL container to be healthy before it can initialize. If Metabase is stuck, verify the database status and restart Metabase:

```bash
docker compose ps reporting-db
docker compose restart compliance-dashboard
```

## Known platform limitations

- **No RBAC**: Role-based access control is in development. Authentication is binary (enrolled or not) without role
differentiation.
- **No Cloud SaaS**: The platform is designed for local deployment. A cloud-hosted SaaS version is not available.
- **No external audits**: Third-party security assessments are planned but not yet completed.
- **Receipt signature distribution**: No attested channel exists for distributing the Gateway's public key to Engine instances. See the Receipt Signature Verification section above.

## Windows-specific issues

### CLI enrollment and file-backed keys

On all platforms (including Windows), `g8e auth enroll user` uses file-backed EC P-256 keys for the CLI identity. The `--tpm` flag was removed in v1.7.2. The `EnrollmentCoordinator` handles the full enrollment state machine (bootstrap, recovery, rotation, reuse) and installs the gateway root CA into the OS trust store before opening the browser for the passkey ceremony. On Windows, trust installation uses the Windows Certificate Store via PowerShell. This is distinct from the browser-based WebAuthn passkey flow, which uses a cookie-based web session.

### Keystore file-based fallback

On Windows, the keystore uses only the file-based fallback for platform secrets. The master key is stored as a base64-encoded file with restrictive permissions. If the keyring is not functioning, check the file-based fallback path in `.g8e/`.

### Path normalization

Windows paths are normalized by converting forward slashes to backslashes and removing redundant separators. The shared path utility layer prevents double-joining of absolute paths. If database or directory paths appear malformed on Windows, ensure the configuration uses consistent path separators.
