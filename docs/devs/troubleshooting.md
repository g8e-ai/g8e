# Developer Troubleshooting

Last Updated: 2026-09-08
Version: v2.1.7

This guide covers common contributor setup, build, test, Gateway, authentication, governance, and local deployment failures. Run the commands from the repository root unless a section says otherwise. See the [Getting Started guide](../guides/getting_started.md) for the supported setup sequence, the [Code Map](codemap.md) for implementation ownership, and the [Documentation Guide](docs.md) for the standards used to maintain this page.

## First checks

Confirm the working directory, binary version, and Go toolchain before diagnosing a deeper failure:

```bash
pwd
ls README.md g8e Makefile VERSION
./g8e version
go version
```

The Go module currently requires Go 1.26.6. The root Makefile uses Bash and common host tools, with additional dependencies determined by the target. On Windows, use `build.ps1` where the Makefile help directs it rather than assuming every target runs in a native Windows shell.

The CLI resolves its project root from the current working directory. Running it elsewhere creates or reads a different `.g8e/` runtime tree. Return to the checkout root before inspecting status, credentials, logs, or local state.

## Build and generation failures

### `./g8e` is missing or not executable

The repository-root `g8e` file is a compiled binary. Rebuild it when it is absent or older than the current source:

```bash
make build
./g8e version
```

On Unix-like systems, restore execute permission if the file exists but the shell rejects it:

```bash
chmod +x g8e
```

`make build` compiles `cmd/g8e`, writes the platform binary under `bin/`, copies it to the repository root, and copies it into `demos/bin/`. The target uses `sha256sum` for the checksum and `pgrep` to detect a running host Gateway. Install compatible commands or use the owning platform build script when either is unavailable. Stop a running host Gateway before rebuilding if the target reports that it cannot replace the binary.

### A Make target cannot find `curl`

`curl` is used by targets that download tools or assets, including the Buf and optional protoc installation paths. Check the active shell's `PATH` before changing repository files:

```bash
command -v curl
```

The core Go CLI does not require `jq`. Individual Make targets still require the tools named in their recipes.

### `make proto` fails

`make proto` regenerates all owned protocol outputs, not only Go protobuf files. It runs the Go, Python, Node, and downstream Python lockfile stages. Check all required tools and Python modules:

```bash
command -v go
command -v buf || test -x ./buf
PYTHON=python3
test ! -x .venv/bin/python || PYTHON="$PWD/.venv/bin/python"
"$PYTHON" -c 'import grpc_tools'
command -v npm
command -v uv
```

If Buf is absent, `make buf-install` first tries `go install github.com/bufbuild/buf/cmd/buf@v1.70.0`; without Go it tries a direct download with `curl`. The Makefile selects `.venv/bin/python` when present and otherwise uses `python3`. The Node stage installs its pinned packages with `npm ci` when necessary. The Python stage requires `grpcio-tools`, and the lockfile stage requires `uv`.

A normal `make build` uses the generated files already present in the checkout and does not require `make proto`. Schema changes require the complete generation toolchain because `make proto` refreshes Go, Python, TypeScript, Markdown reference, and downstream lockfile outputs.

## Gateway startup and status

### `./g8e gw start` does not become healthy

Background startup copies the current executable to `.g8e/bin/`, starts `gw start --follow`, writes logs under `.g8e/logs/`, and polls the plain-HTTP health endpoint. Inspect both managed-process state and logs:

```bash
./g8e gw status
./g8e gw logs
```

Run in the foreground when the background command hides the first useful error:

```bash
./g8e gw stop
./g8e gw start --follow --log debug
```

Common causes include:

- The existing vault header cannot be unlocked because `.g8e/vault/key` is missing, malformed, or belongs to another vault.
- A configured data, PKI, secrets, vault, doctrine, consensus-bootstrap, or network-identity path is invalid or inaccessible.
- No free HTTP/HTTPS port pair is available in the search range, or the selected HTTP and HTTPS ports collide.
- Existing SQLite state is locked by another process.
- Runtime files came from an incompatible or partially removed local state tree.

The configuration layer searches for an available HTTP/HTTPS pair by applying the same offset to the requested ports, starting from 8080 and 8443 by default. Background process startup also searches from the requested HTTP port and verifies the corresponding HTTPS port. Do not assume a collision on 8080 always causes startup to fail; inspect the startup output and log for the selected pair. `gw status` currently displays the default endpoint URLs, so it is not authoritative for a dynamically shifted pair.

The plain-HTTP surface serves health, state binding, CA discovery, bootstrap, recovery, workload enrollment, and binary/deploy discovery routes. Other requests redirect to HTTPS. The HTTPS surface serves authenticated API, MCP, A2A, Console, management, and SSE routes. See [Network Architecture](../architecture/network.md) for the route boundary.

### `governance_ready` is false

`governance_ready` is true without trusted L2 signers in `doctrine` and `ratify` because those postures do not enforce L2. In `consensus` and `notary`, it becomes true after the signer store contains at least one trusted signer.

Missing `--consensus-id`, a missing policy, or a disabled policy does not currently stop the Gateway. Startup logs a warning, and L2-gated transactions fail closed at verification time until the policy and trusted signers exist. Check the active posture, consensus ID, policy, signer registration, and quorum rather than treating process health as proof that L2 is configured. See [Governance](../architecture/governance.md) for the canonical posture and enrollment flow.

### Restart does not preserve a non-default posture

The current `gw restart` path stops the managed process before reading the persisted posture, and stopping removes the posture file. As a result, restart falls back to `doctrine`. Restart a non-doctrine Gateway explicitly:

```bash
./g8e gw stop
./g8e gw start --posture consensus --consensus-id <consensus-id>
```

Include the other startup flags required by the deployment. Do not rely on `gw restart` to preserve custom ports, CORS origins, passkey settings, downstream URLs, or consensus bootstrap configuration.

### `gw reset` and `gw clean` are destructive

Treat both commands as destructive in the current implementation. `gw clean` stops the managed process, attempts to remove g8e root anchors from the OS trust store, and removes the entire local `.g8e/` runtime tree. `gw reset` delegates to `gw clean --force` and then starts a new Gateway, so it also removes PKI, CLI credentials, logs, databases, vault state, and trust material despite its command description saying that it preserves the CA.

Use neither command to repair state that must be retained. Back up required state through its owning export workflow before cleanup. After cleanup, the new Gateway has a new trust root and requires enrollment again.

## Test failures

The canonical test model and commands live in [Testing g8e](tests.md). The short distinction is:

- `./g8e test unit` runs Tier 1 without a live platform.
- `./g8e test integration` runs Tier 2 with local files, SQLite, PKI, pub/sub, and in-process services; it does not require an externally running Gateway.
- `./g8e test e2e` runs Tier 3 network tests against an already running platform.
- Tier 4 Ensemble tests call external providers and require their configured credentials.

Use `./g8e test` or the repository Make targets rather than invoking `go test` directly for platform suites.

### Unit or integration tests report missing Gateway state

Unit tests must not depend on the developer's `.g8e/` tree. Integration tests create isolated runtime roots and in-process services. A missing local trust bundle or developer Gateway is therefore not fixed by starting or modifying the developer's Gateway.

For integration fixture failures, inspect the fixture setup, temporary PKI enrollment, port allocation, and the first service-construction error. `GatewayFixture` creates real SQLite, PKI, pub/sub, and Gateway services and owns cleanup through `t.Cleanup`.

### Tier 3 E2E preflight fails

`./g8e test e2e` loads owner credentials from the repository-root `.g8e/` tree and performs bounded preflight checks against Gateway, Ensemble, and Dashboard. It fails non-zero if any of those services is unreachable. A host-only `./g8e gw start` is insufficient for the general E2E suite because it does not start Ensemble or Dashboard.

For an approved full stack, start the bootstrapped Compose profile according to the [Unified Stack guide](../guides/unified_stack.md), enroll or approve the workloads, and then run the steady-state subset:

```bash
make test-docker
```

Use `./g8e test e2e --run <regexp>` only after preparing the state required by the selected test. Pending, denied, restarted, and approved scenarios are mutually exclusive deployment states. The suite-level preflight still requires Gateway, Ensemble, and Dashboard, so the current E2E entry point cannot successfully run the gateway-only headless scenario even when selected by `--run`.

If configuration loading fails before preflight, confirm that `.g8e/credentials`, `.g8e/cli.crt`, `.g8e/cli.key`, and `.g8e/pki/trust/g8eg-ca-bundle.pem` belong to the running Gateway. If the CLI session expired while the certificate remains valid, run:

```bash
./g8e auth refresh
```

`./g8e test e2e-full` starts the root Compose stack with the `bootstrapped` profile and tears it down with `docker compose down -v`. It removes Compose volumes on exit and is appropriate only when discarding that stack state is intentional.

## Vault and encrypted storage

The Gateway requires an unlocked vault during construction. On the first start, if no vault header exists, it creates `.g8e/vault/`, generates a vault header and random key, stores the key at `.g8e/vault/key`, and unlocks the vault. If a header already exists, startup fails when the corresponding key cannot be read or cannot unlock it.

### Validate an existing vault key

Use the standalone command to validate the header and key pair:

```bash
./g8e vault unlock --vault-dir .g8e/vault --key-path .g8e/vault/key
```

`vault unlock` validates the pair only within that command process; it does not unlock a running or future Gateway process. Start the Gateway with the same key path, or set `G8E_VAULT_KEY`:

```bash
./g8e gw start --vault-key "$PWD/.g8e/vault/key"
```

Use an absolute `--vault-key` path for an explicit override; the Gateway resolves a relative override from its data directory. The key file contains 64 hexadecimal characters encoding 32 bytes, normally followed by a newline. A lost vault key makes data encrypted under that vault unrecoverable. Do not initialize or reset a vault over state that must be retained.

`vault status` opens a new in-process vault object and therefore reports that object as locked even when a separately running Gateway has unlocked its own vault. Use Gateway health and logs to diagnose the running process; use `vault status` only to check whether the selected vault header exists.

### Vault paths appear inconsistent

The current runtime default is `.g8e/vault/key`. Some flag help and comments still describe `.g8e/secrets/key`; use the startup log and `--vault-key` explicitly when diagnosing a non-default deployment. Vault CLI paths must resolve within the active `.g8e/` runtime root.

The keystore master key is separate from the vault key. On Linux the keystore uses libsecret when `secret-tool` is available and otherwise uses `.g8e/secrets/.master_key`; macOS uses Keychain; Windows uses the file backend. A keystore error and a vault-unlock error therefore have different recovery paths.

## Authentication and enrollment

### Local CLI credentials are missing or invalid

Start the Gateway, then enroll from the same project root:

```bash
./g8e gw start
./g8e auth enroll user
```

The local CLI identity consists of `.g8e/credentials`, `.g8e/cli.crt`, `.g8e/cli.key`, and the canonical trust bundle at `.g8e/pki/trust/g8eg-ca-bundle.pem`. CLI private keys are file-backed ECDSA P-256 keys on every supported platform.

`auth enroll user` inspects local state and chooses one path:

- No local identity and no existing Gateway owner: bootstrap the first user.
- No, partial, corrupt, expired, or stale-CA identity against a bootstrapped Gateway: human-approved recovery.
- Complete identity with a valid session: reuse it.
- Complete identity near certificate expiry, or `--rotate-cli`: rotate it over mTLS.
- Valid certificate with an expired or invalid CLI session: stop and direct the user to `auth refresh`.

### Recover a CLI against an existing Gateway

Recovery creates a CSR-bound request with a 10-minute lifetime. The opaque token is returned once and only its hash is stored. The default path opens an approval URL for an existing passkey-authenticated user. The CLI polls until approval and then proves possession of the CSR private key before receiving the new certificate.

For a browserless new CLI, run:

```bash
./g8e auth enroll user --headless
```

The command prints `g8e auth approve-recovery <token>` for an already enrolled CLI to run. `--headless` skips system-trust installation and passkey registration, so the resulting identity supports mTLS CLI access but not Console web-session authentication.

If recovery expires, is denied, or the token is lost, rerun `auth enroll user` to create a new request. Browser approval requires an active web session; CLI approval requires a valid, non-revoked CLI certificate bound to an active user.

### Session expiry and certificate rotation

Web sessions last 24 hours. CLI sessions and CLI certificates last 7 days. Operator enrollment sessions are issued for one hour. Run `auth refresh` when the CLI session is expired but the CLI certificate is still valid. Refresh creates a new session without rotating the certificate.

The enrollment coordinator rotates a CLI certificate within 24 hours of expiry. Force rotation of a still-valid identity with:

```bash
./g8e auth enroll user --rotate-cli
```

An expired certificate cannot authenticate to the refresh or rotation endpoint; rerun `auth enroll user` and complete recovery. Gateway serving certificates last 90 days and the Gateway checks them in its renewal loop. The outbound Operator also runs a client-certificate renewal loop.

`auth logout` removes local credentials, CLI certificate, and CLI key. It does not revoke the server-side session and does not remove the shared OS root CA.

### OS trust and stale CA failures

Browser passkey and Console flows require the Gateway root CA in the relevant system or browser trust store. By default, `auth enroll user` installs the live root before opening the passkey ceremony. When trust changes, enrollment waits for the user to close all browser windows and continue so a new browser process reloads trust.

`--no-system-trust` skips installation only. It does not skip passkey registration, trust-bundle validation, or stale-anchor detection. Use it only when an administrator already installed the live Gateway root. `--headless` is the browserless option.

Enrollment discovers the live CA over the plain-HTTP well-known endpoint. If the live root differs from the local bundle, it routes through recovery and removes stale g8e OS anchors before installing the new root. If discovery is unreachable, verify the discovery endpoint supplied by `--endpoint` and `--port`; an HTTPS-only route cannot replace the plain-HTTP discovery path.

Firefox or another browser with a private trust store may require separate CA installation even when the operating-system store is correct.

## Browser, CORS, and WebAuthn failures

### Cross-origin requests lose the web session

Start the Gateway with every exact allowed frontend origin and corresponding passkey origin. Both flags are repeatable:

```bash
./g8e gw start \
  --cors-origin https://app.example.com \
  --passkey-rp-origin https://app.example.com \
  --passkey-rp-id app.example.com \
  --public-base-url https://app.example.com
```

Browser `fetch` calls must use `credentials: 'include'`, and browser `EventSource` clients must use `withCredentials: true`. When CORS origins are configured, web-session cookies use `SameSite=None`; otherwise they use `SameSite=Lax`. Browser policy can still block cross-site cookies, so same-site deployment or a same-origin proxy may be required. See [Connect a Frontend](../guides/connect_frontend_to_gateway.md) for the complete browser contract.

### WebAuthn reports an RP ID or origin mismatch

The RP ID must be a registrable-domain suffix of the page origin, and the exact page origin must be configured as a passkey origin. Do not use `localhost` as the RP ID for a public hostname. Include scheme and port in `--passkey-rp-origin`, but provide only the domain in `--passkey-rp-id`.

TLS trust failures can surface as WebAuthn failures before the ceremony reaches the Gateway. Verify the live CA, close all browser processes after trust changes, and retry from a fresh browser process.

## L3 approval failures

`ratify` and `notary` enforce L3 proof for mutation-classified actions. A new suspended approval request lasts 2 minutes. After approval, the proof remains valid for up to 30 minutes for verification. The CLI SSE waiter uses a 3-minute timeout to cover the request window plus delivery margin.

If approval times out:

- Retry the original action to create a fresh suspended request.
- Confirm the browser trusts the Gateway and has an active web session.
- Confirm the CLI SSE client uses the current CLI session and HTTPS endpoint.
- Check Gateway logs for an expired suspended transaction, ownership mismatch, or failed proof verification.

Mutation classification is owned by `ActionType.IsMutation` in `internal/constants/action_types.go` and mirrored in `protocol/constants/status.json`. Do not maintain a copied action list in troubleshooting procedures; inspect those owners when a newly added action behaves unexpectedly.

## Consensus and notary transaction failures

For `consensus` and `notary`, confirm all of the following:

- `--consensus-id` selects an enabled policy.
- The policy has at least one member, quorum is at least one and does not exceed member count, and distinct-signer requirements can be met.
- Every expected member has an enabled trusted signer and an available signing key.
- `notary` also has a usable L3 approval path for mutations.

`--consensus-bootstrap <file>` seeds trusted signers, member private keys, and an enabled policy before service construction. The JSON requires `consensus_id`, non-empty `member_app_ids`, and `quorum >= 1`. Optional `member_seeds` supplies one Ed25519 seed per member. Optional `seed_hex` supplies a shared seed; if neither seed form is present, startup generates a shared key. Bootstrap is idempotent by consensus ID and skips an existing policy.

Use per-member seeds when a policy requires distinct signatures. A shared seed registers the same public key for every member and cannot provide cryptographically distinct votes.

## State-root mismatch

L4 rejects an envelope when its `state_merkle_root` is empty or differs from the current root. The in-process Gateway path carries a pre-fetched root through envelope construction and verification so concurrent mutations do not invalidate that local build window. External Operator verification fetches the current root at verification time.

A mismatch usually means an external client built an envelope from an old `/api/v1/state` response or state changed before the Operator verified it. Fetch a fresh root, rebuild and re-sign the complete envelope, and submit it promptly. Never patch only the root on an already hashed or signed envelope.

The Gateway root is persisted in SQLite; a restart alone does not imply a new root. The Operator root is derived from the embedded go-git ledger HEAD, so a ledger commit changes it while an unrelated working-tree edit does not itself change HEAD. See [Storage Architecture](../architecture/storage.md) and [Governance](../architecture/governance.md).

## Runtime paths and ledger copies

All default runtime paths are rooted at `.g8e/` under the current working directory. CLI commands operating on another checkout or directory see another runtime. Prefer explicit command flags for intentional non-default data, PKI, secrets, vault, or key paths.

The encrypted ledger mirror reads a file in full before AES-GCM encryption and rejects files larger than 100 MiB. Because the production ledger requires an encryption vault, treat 100 MiB as the effective mirror limit. The returned error is currently generic, so compare the source file size when a file mutation reaches ledger copy and fails without a more specific cause.

Ledger paths normalize absolute host paths into repository-relative forward-slash paths and remove Windows drive prefixes for consistent history keys.

## SSE delivery failures

SSE polling and streaming run on HTTPS and accept either mTLS CLI/Operator authentication or a browser web-session cookie. App certificates are not SSE consumer identities. Routing is derived from authenticated context, not caller-provided user or session query parameters.

When events are missing:

- Confirm the CLI sends `X-G8E-CLI-Session-ID`, or the browser sends its session cookie.
- Confirm the authenticated user owns the selected CLI or web session.
- Reconnect with `Last-Event-ID` or `since_id` to replay persisted rows.
- Check for `replay_failed` or `truncated` sentinel events.

Each live subscriber has a 100-entry drop-oldest buffer. Persisted replay returns at most 1,000 rows per stream connection and emits a truncation sentinel at that limit. The maintenance loop runs every 30 seconds and deletes SSE rows older than one hour. Reconnect promptly because dropped live entries are recoverable only while their persisted rows remain within that retention window. See [SSE Streaming](../architecture/sse.md).

## Rate-limit responses

The Gateway's global token-bucket limiter is disabled when `--rate-limit-rps` is zero or negative. When enabled, it applies per remote IP to the plain-HTTP router and to selected HTTPS ingress handlers, including governance-envelope and MCP/A2A submission. `--rate-limit-burst` controls immediate burst capacity.

An enrolled app can also have its own app-policy rate limit. For HTTP 429 responses, identify whether the log reports the remote-IP limiter or the app-policy limiter before changing configuration. Retrying a governance mutation is not equivalent to batching multiple actions into one envelope; preserve the intended transaction and governance semantics.

## Cloudflare Tunnel failures

Use the [Cloudflare Tunnel guide](../guides/cloudflare_tunnel.md) as the canonical setup procedure. For a local origin check:

```bash
curl -sk https://localhost:8443/api/v1/health
./g8e gw tunnel status --hostname <public-hostname> --name <tunnel-name>
```

A 502 usually means `cloudflared` cannot reach the configured local HTTPS port or cannot validate the origin certificate. `gw tunnel create` defaults to `noTLSVerify` unless `--ca-bundle` is supplied; when verification is enabled, `--origin-server-name` must match a serving-certificate SAN.

If DNS routing reports an existing record, the create command already recognizes that condition and continues. Use `--skip-dns` when DNS is managed separately. Check `cloudflared --version` and its foreground output when the tunnel exists but does not connect.

Public browser access also requires matching `--public-base-url`, `--passkey-rp-id`, `--passkey-rp-origin`, and `--cors-origin` Gateway settings.

## Receipt verification failures

Gateway and Operator L5 Actuators sign `ActionReceipt` messages with host-local Ed25519 keys and export `Actuator_pub.pem` and `Actuator_pub.json` under their runtime PKI directories. Verification requires the public key from the component that produced the receipt and must check both the receipt signature and final persistence attestation.

The Python protocol package exposes typed parsing, canonicalization, signature verification, and persistence-attestation verification helpers in `g8e.receipts`. A key copied from the producing runtime enables cryptographic verification but does not independently establish trust in that key. Bind trusted verifier keys through a separately authenticated deployment or evidence workflow. See [Build Apps](../guides/build_apps.md) for a minimal verifier and [Encryption Architecture](../architecture/encryption.md) for key ownership.

## Demo and Docker failures

Run demo Compose commands from the demo directory that owns `compose.yml`, or pass `-f` explicitly. Running `docker compose ps` at the repository root inspects the unified stack, not a demo stack.

For the healthcare Metabase dependency:

```bash
cd demos/healthcare
docker compose ps reporting-db
docker compose logs reporting-db
docker compose restart compliance-dashboard
```

Use `./g8e demos scenarios list` and the owning demo README to identify scenario prerequisites. Multiple demo environments use different host-port mappings; inspect each demo's `compose.yml` rather than assuming root ports.

## Windows-specific failures

The CLI uses file-backed ECDSA P-256 keys on Windows. OS trust installation invokes the Windows Certificate Store through PowerShell, while browser passkeys create a separate web-session identity. The keystore uses `.g8e/secrets/.master_key` because the Windows backend is file-based.

Run the Windows binary as `g8e.exe`. Use `build.ps1` for native Windows build workflows identified by the repository help. Paths are resolved through the current working directory and normalized for ledger history; avoid mixing runtime trees created from different shells or working directories.
