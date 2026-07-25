# Developer Troubleshooting

Last Updated: 2026-07-19
Version: v1.5.9

This page covers common setup failures, runtime friction, and operational
caveats for contributors working on g8e from a fresh checkout. The platform
runs host-native. For architecture-level context, see
[Authentication & Authorization](../architecture/auth.md),
[Encryption Architecture](../architecture/encryption.md),
[Gateway Architecture](../architecture/gateway.md),
[Governance](../architecture/governance.md), and
[Network Architecture](../architecture/network.md).

## First checks

Run commands from the repository root:

```bash
pwd
ls README.md g8e Makefile
```

Use a POSIX shell such as Linux, macOS Terminal, WSL, or Git Bash. The
`Makefile` uses Bash, `sed`, and `curl`. The `g8e` binary at the repository
root is a compiled Go executable; it does not depend on shell utilities beyond
the system libc.

At minimum, install the tools for the component you are touching:

- Go 1.26.5 or later for the g8e Operator and protocol work.
- Python 3.10 or later for protocol generation and demo scripts.

## `make` targets fail with missing `curl`

The `Makefile` uses `curl` to download Buf during `make buf-install` and in
demo scripts. If `curl` is not installed, install it and retry:

```bash
command -v curl
make proto
```

If the command exists in one terminal but not another, fix the shell `PATH`
before changing project files.

> Note: To support sovereign, agnostic, and air-gapped deployments, `jq` is
> completely eliminated as a host dependency. All JSON parsing and request
> assembly are handled internally by the Go CLI, allowing g8e to run on
> virtually any modern Linux environment without extra system package requirements.

## `make proto` fails before generating files

`make proto` runs `make buf-install`, then calls Buf to generate Go Protobuf
code from the schema definitions in `protocol/proto/`.

Check the local prerequisites first:

```bash
command -v go
command -v buf
```

The `buf-install` target attempts to provision Buf in the following order:

1. If Go is present on the host, it installs Buf via
   `go install github.com/bufbuild/buf/cmd/buf@v1.70.0`.
2. If Go is absent, it attempts to download the pre-compiled binary from Buf
   releases using `curl`.
3. If neither succeeds, `make proto` exits with an error. The pre-generated
   `.pb.go` files committed under `protocol/proto/` allow `go build` to
   succeed without running `make proto`, but schema changes require a working
   Buf installation.

If you are modifying `.proto` files in an offline environment, ensure that
`buf` is installed globally on your path before running `make proto`.

For Python protocol generation, use the separate target:

```bash
make proto-python
```

## `./g8e gw start` does not become healthy

The `gw start` command launches the g8e Gateway as a background process via
`gateway serve`, then waits for the process to become healthy. Start with the
status command and the log:

```bash
./g8e gw status
./g8e gw logs
```

Common causes:

- One of the local ports from `protocol/constants/ports.json` (HTTP 8080, HTTPS 8443) is already in use. The process manager in `internal/cli/platform/process.go` performs a preflight port check and reports conflicting PIDs.
- The Go toolchain is missing or below the version expected by the current Developer Guidelines (Go 1.26.5).
- Runtime PKI or secrets were created by an older incompatible checkout.
- Port collision prevention: the gateway fails startup if multiple logical surfaces are assigned to the same port, ensuring no downgrade of the mTLS execution boundary. The HTTP surface (8080) serves plain HTTP for bootstrap and PKI discovery only; the HTTPS surface (8443) handles all API, MCP, console, and management routes. See [Network Architecture](../architecture/network.md) for port topology details.
- Governance posture validation: when starting in `consensus` or `notary` posture, the gateway validates tribunal prerequisites at startup before any services start. If the tribunal ID is empty, the tribunal policy does not exist in the database, or quorum is less than 1, the gateway exits with an error. See [Governance](../architecture/governance.md) for posture startup validation details.

Stop the managed process before retrying. Use `gw restart` as a shortcut, or
stop and start manually:

```bash
./g8e gw restart
```

```bash
./g8e gw stop
./g8e gw start
```

Use `./g8e gw reset` or `./g8e gw clean` only for disposable local
state. They intentionally remove runtime data under `.g8e/`.

If the gateway started but the HTTPS health endpoint reports
`governance_ready: false`, the gateway is running in `consensus` or
`notary` posture and no trusted L2 signers are registered. In `doctrine`
posture, `governance_ready` is always `true`. To resolve, register
trusted signers or switch to `doctrine` posture. See
[Governance](../architecture/governance.md) for signer registration
details.

## Tests fail because the gateway is not running

The test suite uses a tiered structure with different infrastructure requirements:

- **Tier 1 (Unit tests)**: Run immediately without external dependencies via `make test-unit`.
- **Tier 2 (In-Process Integration)**: No external dependencies. Integration tests use in-process gateway fixtures (`test/fixtures/gateway_fixture.go`) that spin up the gateway within the test process. Run via `make test-integration`.
- **Tier 3 (Docker E2E)**: Requires Docker. Run via `make test-docker`.

Tier 2 integration tests do not require a running external gateway. They
construct the gateway in-process via `GatewayFixture`, which handles PKI
enrollment and mTLS configuration automatically. If these tests fail, the cause
is typically a port conflict or missing build dependencies, not a missing
gateway process.

If a test failure mentions missing trust bundles or client certificates,
confirm that the test fixture has not been modified to skip enrollment. The
`EnrollClientIdentity` helper in `test/fixtures/gateway_fixture.go` generates
test PKI material at runtime.

## Vault and encryption failures

All sensitive data is encrypted at rest using AES-256-GCM via the vault
subsystem. Services that handle sensitive content (audit store, execution
vault, token store, ledger) fail closed when the vault is locked or not
initialized. See [Encryption Architecture](../architecture/encryption.md)
for vault lifecycle details.

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

Without a vault key, the gateway starts but storage services fail closed on first use.
Provide the vault key via `G8E_VAULT_KEY` or `--vault-key` to enable decryption.

### Invalid or lost vault key

If vault unlock fails with an invalid key error:

- Verify the key path is correct and the key file is readable.
- Check that the key is a 32-byte hex-encoded value.
- If the key is lost, all encrypted data is unrecoverable. The only option is
  `./g8e vault reset --confirm`, which destroys the vault and all encrypted
  data, followed by `./g8e vault init` and a gateway restart.

### Platform keyring fallback

The keystore uses the OS-native credential store when available, with a
file-based fallback. On Linux, GNOME Keyring via libsecret is used when
present; otherwise, the master key is stored as a base64-encoded file with
restrictive permissions. On macOS, Keychain is used. On Windows, only the
file-based fallback is available. If the keyring service is unavailable,
check the file-based fallback path in `.g8e/`.

## Authentication failures after gateway start

The gateway requires explicit authentication before it can be used. After
starting the gateway, enroll to bootstrap your credentials:

```bash
./g8e gw start
./g8e auth enroll
```

If authentication fails, check the following:
- Ensure the gateway is running via `./g8e gw status`.
- Verify the external IP displayed during gateway start matches your network interface.
- For passkey authentication, ensure your hardware security key or platform authenticator is available.
- For certificate-based authentication, ensure `.g8e/cli.crt` and `.g8e/cli.key` exist. The `auth enroll` command generates these files via CSR-based enrollment with the gateway CA. On Windows, enrollment uses the Windows Certificate Store automatically.

### Browser CA trust and WebAuthn failures

The gateway uses self-signed certificates. Browser-based WebAuthn
registration and console access require the platform Root CA to be trusted
by the operating system. Run the appropriate trust script for your platform:

- **Linux/macOS**: `curl -fsSL http://<gateway-ip>:8080/web-cert.sh | sh`
- **Windows**: `irm http://<gateway-ip>:8080/web-cert.ps1 | iex`

After running any trust script, **restart all open browsers**. Browsers
cache certificate trust state, and WebAuthn registration will fail if the
browser does not yet recognize the new platform CA. This is the most common
cause of console access failures on fresh setups.

### Certificate expiry

Leaf certificates (operator, CLI, app) have a 7-day validity period. If
authentication fails with a certificate-related error, the CLI certificate
may have expired. Re-enroll to obtain a fresh certificate:

```bash
./g8e auth enroll
```

The gateway serving certificate has a 90-day validity period and is
auto-rotated by the PKI authority. Operator and CLI certificates require
manual re-enrollment after expiry.

### g8e.local DNS resolution

The platform uses `g8e.local` as the default hostname for gateway
connections. If `g8e.local` does not resolve via system DNS, the CLI
automatically falls back to the machine's external interface IP. This
fallback is implemented in `internal/cli/cmd/mcp.go`. No `/etc/hosts`
changes or DNS configuration are required for basic operation.

### Session TTL

Operator sessions have a 24-hour TTL by default. CLI sessions have a
1-hour TTL. If CLI commands fail with session-related errors after
extended idle periods, re-enroll to obtain a fresh session. Operator
sessions require a gateway restart to refresh.

## L3 approval timeouts and transaction rejection

Under `notary` posture, mutation actions require L3 human authorization.
The approval flow has two time windows that can cause transaction rejection:

- **Request window (2 minutes)**: The passkey ceremony must be completed
  within 2 minutes of the transaction being suspended. If the passkey
  approval is not completed within this window, the request expires and the
  action must be retried.
- **Dispatch window (30 minutes)**: After approval, the transaction must be
  dispatched within 30 minutes. Transactions not dispatched within that
  window must be re-approved.

The CLI SSE client in `internal/cli/auth/approval_sse.go` waits for
`approval.completed` events with a 3-minute timeout, which covers the
2-minute gateway request window plus margin. If the SSE wait times out,
the CLI reports the failure and the transaction must be resubmitted.

Common causes of approval failures:
- The user did not complete the passkey ceremony in time.
- The browser was not restarted after running the CA trust script.
- The SSE stream was not accessible due to network or authentication issues.
- The gateway posture was changed to `notary` without a configured L3 notary.

See [Authentication & Authorization](../architecture/auth.md) for the
full approval flow and [SSE Streaming](../architecture/sse.md) for SSE
event delivery details.

## Governance posture startup failures

The gateway posture is set at startup via `--posture <doctrine|consensus|notary>`
and cannot be changed at runtime. Each posture has different startup
requirements:

- **Doctrine** (default): No tribunal prerequisites. L2 and L3 are audited
  but not enforced.
- **Consensus**: Requires a tribunal policy to exist in the database with
  quorum >= 1. The Tribunal service is bootstrapped in-process.
- **Notary**: Same as consensus, plus L3 notary must be configured for
  mutation actions to succeed.

If the gateway fails to start in `consensus` or `notary` posture, check:

- The tribunal ID is non-empty.
- The tribunal policy exists in the database and is enabled.
- Quorum is >= 1 (valid for single-member ensembles).
- For `notary` posture, the L3 notary is available. Without it, mutations
  fail closed.

For declarative tribunal seeding, use the `--tribunal-bootstrap` flag with
a JSON config file containing `tribunal_id`, `member_app_ids`, `quorum`,
and optional `seed_hex`. This is idempotent: if the tribunal already
exists, the bootstrap is skipped.

Tribunal members whose keys cannot be resolved during bootstrap are
included without a private key and a warning is logged. They can
participate in policy but cannot sign votes. If all members lack keys,
L2 deliberation produces no votes and quorum fails.

See [Governance](../architecture/governance.md) for posture definitions
and tribunal bootstrap details.

### Governance envelope rate limiting

The governance envelope submission endpoint (`POST /api/v1/governance/envelopes`)
is rate-limited. If submissions fail with HTTP 429, reduce the submission
frequency or batch multiple actions into fewer envelopes.

### Outbound mode mutations fail closed

Notary is the default posture for outbound (operator) mode. Since the
L3 notary is nil in outbound mode, mutations fail closed unless an L3
notary is explicitly configured. If operators reject all mutations with
L3-related errors, either configure an L3 notary for the operator or
switch the operator to `doctrine` or `consensus` posture.

The following action types are classified as mutations and require L3
proof under notary posture: `A2A_CALL`, `CANCEL`, `EXECUTE_BASH`,
`FILE_EDIT`, `MCP_CALL`, `RESTORE_FILE`, `SHUTDOWN`. Non-mutation
actions (e.g., `FS_READ`, `FS_LIST`, `FETCH_LOGS`) do not require L3
proof even under notary posture.

## State Merkle root mismatch

The L4 Warden validates that the `state_merkle_root` in the
GovernanceEnvelope matches the current state root of the gateway or
operator. A mismatch causes the transaction to be rejected as stale.

Common causes:
- Concurrent transactions modifying shared state between envelope
  creation and submission.
- The gateway was restarted between envelope creation and submission,
  causing the state root to change.
- The operator's git ledger HEAD changed due to external file modifications.

If this occurs frequently, ensure envelopes are submitted promptly after
creation. The state root is derived from the git ledger HEAD commit hash
on the operator and from the SQLite state version on the gateway. See
[Storage Architecture](../architecture/storage.md) for state root
details and [Governance](../architecture/governance.md) for Warden
validation logic.

## Path resolution problems

The CLI resolves the project root using `config.FindProjectRoot()` in
`internal/config/config.go`, which returns the current working directory
(`os.Getwd()`). Run commands from the project root directory to ensure
correct path resolution:

```bash
cd /path/to/g8e
./g8e gw status
```

### Cross-platform path handling

All storage services construct filesystem paths through a shared path
utility layer that prevents a Windows-specific double-join issue: when
two absolute paths are joined with standard library functions, the
result is an invalid concatenated path (for example,
`C:\temp\C:\temp\data.db`). The utility layer detects absolute paths in
the joined elements and uses them as-is.

Configuration paths for databases and directories can be either relative
or absolute. Relative paths are resolved against a base data directory.
Absolute paths are respected and used without modification, allowing
operators to place individual databases on separate volumes or drives.

The ledger additionally strips Windows drive letters and leading
separators before constructing ledger-relative paths, ensuring that file
history is consistent across platforms. See
[Storage Architecture](../architecture/storage.md) for cross-platform
path handling details.

### Ledger file size limit

The ledger enforces a 100 MB size limit on encrypted file copies to
prevent out-of-memory during the full-read required by AES-256-GCM.
Files larger than 100 MB cannot be encrypted and copied to the ledger.
Unencrypted file copies are streamed to avoid loading entire files into
memory. If a file operation fails with a size limit error, the file
exceeds the encrypted copy cap.

## `./g8e` command not found

The `g8e` file at the repository root is a compiled Go binary. If you receive
"command not found", ensure you are running from the repository root and the
binary has execute permissions:

```bash
ls -l g8e
chmod +x g8e
./g8e gw status
```

If the binary is missing or outdated, rebuild it:

```bash
make build
```

The `make build` target compiles `cmd/g8e` and copies the resulting
binary to the repository root as `g8e`. The target handles Windows builds
natively, producing `g8e.exe` when run on Windows.

## SSE event delivery issues

The SSE streaming infrastructure provides real-time event delivery from
app workloads to browser and CLI clients. Events are stored in the
`sse_events` table and routed by `web_session_id`, `cli_session_id`, or
`user_id`. See [SSE Streaming](../architecture/sse.md) for full details.

### Events not received

If SSE events are not reaching clients:

- Verify the client is authenticated. SSE consumer endpoints
  (`/api/v1/sse/events`, `/api/v1/sse/stream`) require dual auth: mTLS
  for CLI/operator clients, or web session cookie for browser clients.
- Check that the routing identifier (`web_session_id`, `cli_session_id`,
  or `user_id`) matches the authenticated session. The
  `authorizeSSERoute` helper enforces ownership checks.
- SSE endpoints are only available on the HTTPS port (8443), not on the
  HTTP bootstrap port (8080).

### Event drops under high load

The SSE stream uses a buffered channel of 100 entries per subscriber.
If the buffer is full when an event arrives, the event is dropped with a
back-pressure warning log. This can occur under high-frequency event
streaming. Consider batching events before pushing to reduce database
load and buffer pressure.

### Event retention

The `sse_events` table is pruned automatically by the gateway maintenance
loop every 30 seconds with a 1-hour retention window. Events older than
1 hour are deleted. If historical events are needed beyond 1 hour, query
them before they expire.

### State root impact

SSE event inserts do not alter the state root. This is intentional to
allow high-frequency event streaming without governance overhead. Events
are considered ephemeral telemetry, not governance state.

## CORS and cross-origin session issues

Browser-based frontends connecting to the gateway require explicit
CORS configuration. If API calls return 401 despite being logged in, or
the browser console shows `Access-Control-Allow-Origin` errors, the
gateway was not started with `--cors-origin` matching the frontend
origin.

Restart the gateway with the correct origin:

```bash
./g8e gw start --cors-origin https://your-app.example.com --passkey-rp-origin https://your-app.example.com
```

Ensure every `fetch` call includes `credentials: 'include'`. The
gateway sets `SameSite=None` on session cookies only when `--cors-origin`
is configured, which is required for cross-origin cookie delivery.

For SSE connections from browser clients, construct `EventSource` with
`withCredentials: true`. Without authenticated session cookies, the SSE
endpoints return 401. Verify the `web_session_id` is valid via
`GET /api/v1/auth/sessions/me`. See
[Build a g8e-Compatible Frontend](../guides/build_frontend.md) for the full browser
integration flow.

## WebAuthn passkey RP ID mismatch

If the WebAuthn ceremony fails with "RP ID is not a valid domain" or
similar, the `--passkey-rp-id` flag does not match the frontend app's
registrable domain. The RP ID must be a registrable domain suffix of
the current page's origin. For example, when accessing the gateway via
`console.g8e.ai`, the RP ID must be `console.g8e.ai`, not `localhost`.

When accessing via a Cloudflare tunnel, set `--passkey-rp-id` to the
tunnel hostname. See
[Cloudflare Tunnel Integration](../guides/cloudflare_tunnel.md) for
tunnel-specific configuration.

## Stale trust bundle after PKI regeneration

If authentication fails after the gateway PKI is regenerated, the local
trust bundle and client certificates may be stale. Log out and re-enroll
to obtain fresh credentials:

```bash
./g8e auth logout && ./g8e auth enroll
```

## Cloudflare tunnel issues

When exposing the gateway via a Cloudflare tunnel, additional failure
modes apply beyond local gateway troubleshooting. See
[Cloudflare Tunnel Integration](../guides/cloudflare_tunnel.md) for
full setup instructions.

### 502 Bad Gateway

The tunnel is connected but the gateway is not running or not listening
on `localhost:8443`. Verify the gateway health endpoint and tunnel
status:

```bash
curl -sk https://localhost:8443/api/v1/health
g8e gw tunnel status --hostname console.g8e.ai --name g8e
```

### DNS record conflict

If DNS routing fails with "record already exists," use `--skip-dns`
with `g8e gw tunnel create` to skip the DNS step. Alternatively, delete
the old DNS record in the Cloudflare dashboard or use a different
hostname.

### Tunnel version outdated

If the tunnel exhibits unexpected behavior, check and update
`cloudflared`:

```bash
cloudflared --version
```

If installed via dpkg, download and install the latest release from the
Cloudflare GitHub releases page.

## Receipt signature verification

The Gateway signs `ActionReceipt`s with its Actuator Ed25519 private
key. The actuator public key is exported to the PKI directory during
gateway boot in both PEM and JSON formats.

No mechanism exists for distributing the Gateway's public key to Engine
instances via an attested channel. Consumers that need to
cryptographically verify receipt authenticity must obtain the public key
out-of-band by reading the exported files from the gateway PKI
directory. The g8e Python package does not currently expose a receipt
verification utility. Consumers can implement Ed25519 verification
using `nacl.signing.VerifyKey` with the exported public key.

See [Encryption Architecture](../architecture/encryption.md) for
receipt signature details.

## Docker demo issues

### Demo port conflicts

If multiple demos are running simultaneously, port conflicts may occur. Check which demos are running:

```bash
docker compose ps
```

### Metabase startup failure (healthcare demo)

Metabase requires the `reporting-db` PostgreSQL container to be healthy
before it can initialize. If Metabase is stuck, verify the database
status and restart Metabase:

```bash
docker compose ps reporting-db
docker compose restart compliance-dashboard
```

### RabbitMQ management UI unreachable (healthcare demo)

The RabbitMQ management plugin is mapped to port 15673 on the host
(from 15672 inside the container). Use `localhost:15673` to access the
management UI.

## Known platform limitations

- **No RBAC**: Role-based access control is in development.
  Authentication is binary (enrolled or not) without role
differentiation.
- **No Cloud SaaS**: The platform is designed for local deployment. A
  cloud-hosted SaaS version is not available.
- **No external audits**: Third-party security assessments are planned
  but not yet completed.
- **Receipt signature distribution**: No attested channel exists for
  distributing the Gateway's public key to Engine instances. See the
  Receipt Signature Verification section above.

## Windows-specific issues

### CLI enrollment and the Windows Certificate Store

On Windows, `g8e auth enroll` uses the Windows Certificate Store for CLI
session enrollment. The signed certificate is imported to
`Cert:\CurrentUser\My` for Windows Hello native API access. This is
distinct from the browser-based WebAuthn passkey flow, which uses a
cookie-based web session.

### TPM-backed keys

The `--tpm` flag requests TPM-backed keys via Windows Hello for
Business. Currently, the implementation uses a software-backed key with
TPM annotation. Full CNG API integration for hardware-backed keys is
pending. If TPM enrollment fails, omit the `--tpm` flag and use
software-backed keys.

### Keystore file-based fallback

On Windows, the keystore uses only the file-based fallback for platform
secrets. The master key is stored as a base64-encoded file with
restrictive permissions. If the keyring is not functioning, check the
file-based fallback path in `.g8e/`.

### Path normalization

Windows paths are normalized by converting forward slashes to backslashes
and removing redundant separators. The shared path utility layer
prevents double-joining of absolute paths. If database or directory paths
appear malformed on Windows, ensure the configuration uses consistent
path separators.
