---
doc_id: build_operator
title: Build and Run an Operator
audience: developers, deployers, independent implementations
status: current
last_updated: 2026-10-06
version: v2.3.2
owners:
  - docs/guides/
  - cmd/g8e
  - internal/cli/cmd/operator
  - internal/cli/cmd/gw
related:
  - docs/guides/connect_operator_to_gateway.md
  - docs/architecture/operator.md
  - docs/architecture/protocol.md
when_to_read: Building the reference Operator binary, starting a Gateway and Operator pair, configuring enrollment, understanding the public protocol surface, or implementing an independent Operator.
do_not_use_for:
  - Enrollment and day-two operations (see connect_operator_to_gateway.md)
  - Complete service architecture and thread model (see docs/architecture/operator.md)
  - Protocol library contents and package reference (see docs/architecture/protocol.md)
---

# Build and Run an Operator

## Purpose

The Governed Operator is the host-side Policy Execution Point (PEP). It opens an outbound mTLS WebSocket connection to a g8e Gateway, subscribes to its identity- and session-scoped command channel, decodes canonical protojson `GovernanceEnvelope` transactions, re-runs L1 Doctrine, verifies the universal transaction checks and posture-required L2 and L3 proofs in L4 Warden, and dispatches verified actions through L5 Actuator.

The reference Gateway and Operator compile into the same `g8e` binary. `./g8e gw start` runs the Gateway Policy Decision Point (PDP), including the client-facing MCP and A2A endpoints. `./g8e operator start` runs the outbound Operator worker. The outbound Operator does not expose an MCP or A2A listener; the Gateway translates client requests into governed transactions and publishes them to the bound Operator.

This guide covers building and running the reference Operator and identifies the public protocol surface available to independent implementations.

## Quick index

- [Purpose](#purpose)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

## Invariants

Ids are stable. Append the next free number in a topic. Do not renumber.

| ID | Rule |
| --- | --- |
| INV-BUILD-OP-01 | The reference binary compilation path is `GOOS/GOARCH` Go environment variables passed to `make build` or `make build-<platform>`. The resulting binary embeds the evaluation explorer SPA and carries provenance stamps (version, build ID, build time, platform). |
| INV-BUILD-OP-02 | The `operator start` command accepts mTLS connection parameters (`--endpoint`, `--cert`, `--key`, `--trust-bundle`) and role-specific flags (`--inference-enabled`, `--provider-boundary-observer-enabled`, `--provenance-operator-enabled`). Unknown flags and flags not copied to `ServeOperatorOptions` do not affect runtime behavior. |
| INV-BUILD-OP-03 | Multiple Operators on the same host use distinct working directories and role-specific flags to prevent fingerprint collisions. They may share a Gateway endpoint and expose no inbound ports. The Gateway differentiates instances by SHA-256 composite fingerprint incorporating system properties, directory, account, port, and role. |
| INV-BUILD-OP-04 | The canonical runtime state tree `.g8e/` contains pki/, data/, vault/, and (optionally) data/ledger/ subdirectories. Auto-initialization occurs on first `operator start` without a separate `vault init` step. |
| INV-BUILD-OP-05 | L1 Doctrine re-execution, L2/L3 posture verification, replay and expiry checks, transaction hash equality, state-root binding, and receipt signing are performed in L4 Warden before L5 Actuator execution. Verification failures produce deterministic stage evidence. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Build targets and environment | `Makefile` | `make help` for current list |
| Operator start command and flags | `internal/cli/cmd/operator/operator.go` (operatorStartCmd) | `./g8e operator start --help` |
| Gateway start command and flags | `internal/cli/cmd/gw/gateway.go` (gatewayStartCmd) | `./g8e gw start --help` |
| Operator list/bind/run/stop commands | `internal/cli/cmd/operator/operator.go` | `./g8e operator --help` |
| Vault administration | `internal/cli/cmd/vault/` | `./g8e vault --help` |
| Public protocol Go module | `github.com/g8e-ai/g8e/v2@v2.3.2` | `go get -d github.com/g8e-ai/g8e/v2@v2.3.2` |

## Procedures

### Build the Reference Operator

#### Prerequisites

- **Go 1.26.9 or later**, as declared by the root Go module.
- **Make** on Linux and macOS. Windows builds use the repository's PowerShell workflow and still invoke Make.
- **Node.js 22 or later** and **npm**, required by `make build` to build the Evaluation Explorer and Console embeds.
- **PowerShell 7 or later (`pwsh`)** for the native Windows setup script.

To refresh only the Evaluation Explorer embed, run this from the repository root:

```bash
make explorer-embed
```

The repository setup scripts validate the development tools, offer to install missing tools, build the evaluation explorer when needed, and run `make build`. The Linux and macOS scripts also install the toolchain behind `make ci` unless you pass `--build-only`:

- Linux: `bash scripts/linux-setup.sh`
- macOS: `bash scripts/macos-setup.sh`
- Windows: `pwsh scripts/windows-setup.ps1`

#### Build from Source

```bash
git clone https://github.com/g8e-ai/g8e.git
cd g8e
make build
```

`make build` copies any existing `evaluation-explorer/dist/` and `console/dist/` bundles into their Gateway embeds, using the committed embeds when those directories are absent. It then creates the platform-specific binary and checksum under `bin/` and copies the host binary to `./g8e` (or `./g8e.exe` on Windows). It does not build either frontend; explicit embed refreshes are part of [release preparation](../devs/release_process.md).

The build sets `CGO_ENABLED=0`, uses the `netgo` and `osusergo` build tags, strips symbol and debug data, and embeds the platform version, build ID, build time, and target platform. The resulting binary does not require a Go toolchain or a system SQLite library on the target host.

The binary is self-contained, but the running Operator is stateful. `--working-dir` selects both its command execution directory and the root below which it stores `.g8e/` enrollment credentials, encrypted local state, and evidence. When omitted, the process working directory is used. The Operator requires write access to its runtime root and network access to its Gateway.

#### Build Targets

| Target | Result |
| --- | --- |
| `make build` | Builds the current OS and architecture, writes `bin/g8e-<os>-<arch>`, and copies the host binary to the repository root. |
| `make build-all` | Builds Linux amd64/arm64/386, Windows amd64/arm64, and Darwin amd64/arm64 binaries, rewrites portable SHA-256 sidecars, and publishes `bin/g8e-binaries.json` last after the complete matrix validates. Linux variants use the pinned FIPS module. |
| `make build-linux` | Builds `bin/g8e-linux-{amd64,arm64,386}` and checksum files. |
| `make build-windows` | Builds `bin/g8e-windows-{amd64,arm64}.exe` and checksum files. |
| `make build-darwin` | Builds `bin/g8e-darwin-{amd64,arm64}` and checksum files. |
| `make build-compressed` | Builds the host binary and compresses it with UPX. This target requires UPX. |
| `make fips-build` | Builds `bin/g8e-fips-linux-amd64` with `GOFIPS140=v1.0.0`. |
| `make fips-verify` | Builds the FIPS variant and runs `g8e version --fips` with FIPS-only enforcement enabled. |

`make fmt`, the host-native `make up` / `make gw` / `make down` and `make full` / `make full-setup` lifecycle, the separate `make docker-up` lifecycle, and the cleanup targets are development and platform-management targets rather than Operator build variants. `make clean` removes build artifacts and Go caches while preserving `.g8e/` runtime state and workload identities. Use `./g8e gw clean` explicitly to reset Gateway state.

#### Cross-Compilation

The dedicated platform targets are the simplest cross-compilation path:

```bash
make build-linux
make build-darwin
make build-windows
```

A single target can also be selected through the Go environment consumed by `make build`:

```bash
GOOS=linux GOARCH=amd64 make build
GOOS=darwin GOARCH=arm64 make build
GOOS=windows GOARCH=amd64 make build
```

On a native Windows host, use `pwsh scripts/windows-setup.ps1` or run the Makefile targets from an environment that provides Make. The setup script invokes `make build` after validating its prerequisites.

### Start the Reference Operator

#### Start a Gateway

The Operator requires a reachable Gateway for enrollment, bootstrap configuration, pub/sub, and receipt publication. Envelope verification and authoritative execution evidence remain local and require no state-root lookup. Start the Gateway first:

```bash
./g8e gw start
```

The default Gateway posture is `doctrine`. The Gateway also supports `consensus`, `ratify`, and `notary`; the selected posture is written into each envelope and is authoritative for Operator-side L2 and L3 gating.

#### Start and Enroll the Operator

On the target host, start the Operator with the Gateway discovery endpoint:

```bash
./g8e operator start --endpoint <gateway-host>
```

When no installed Operator credentials exist, `--endpoint` starts the owner-approved platform enrollment protocol. The Operator fetches the Gateway trust bundle from the HTTP discovery endpoint, creates Operator and CLI certificate requests, persists pending enrollment state, waits for Gateway-owner approval, verifies the completion transcript, and writes the issued credentials and canonical trust bundle into `.g8e/pki/`. Restarting the process resumes the same pending enrollment request and key material.

The enrolled owner reviews pending requests with `g8e auth enroll pending`, approves with `g8e auth enroll approve <request-id>`, or denies with `g8e auth enroll deny <request-id>`. Use `g8e auth enroll list` to review completed or revoked enrollments and obtain request IDs for `g8e auth enroll revoke <request-id>`. See the [Authentication and Authorization architecture document](../architecture/auth.md) for the full platform enrollment command reference.

After enrollment, the Operator loads `.g8e/pki/operator.crt` and `.g8e/pki/operator.key`, connects to the Gateway over mTLS, requests bootstrap configuration, initializes encrypted local services, subscribes to its command channel, and starts automatic heartbeats. The canonical trust bundle is `.g8e/pki/trust/g8eg-ca-bundle.pem`.

For pre-provisioned credentials, pass explicit paths:

```bash
./g8e operator start \
  --endpoint <gateway-host> \
  --cert /path/to/operator.crt \
  --key /path/to/operator.key \
  --trust-bundle /path/to/g8eg-ca-bundle.pem
```

#### Operator Start Options

The current worker path applies these options:

| Option | Behavior |
| --- | --- |
| `-e, --endpoint <host>` | Selects the Gateway discovery host. If omitted, the default is `localhost`; a missing local trust bundle then prevents startup. |
| `--cert <path>` | Uses an explicit Operator client certificate instead of runtime-tree discovery or enrollment. |
| `-k, --key <path>` | Uses the private key paired with `--cert`. |
| `--trust-bundle <path>` | Loads an explicit CA trust bundle. With an endpoint and no local bundle, the Operator fetches the Gateway bundle from its well-known HTTP endpoint. |
| `--working-dir <path>` | Sets the command execution directory and the root for the Operator's `.g8e/` runtime state. Defaults to the process working directory. |
| `-c, --cloud` | Enables cloud Operator mode. |
| `--provider <aws\|gcp\|azure>` | Sets the cloud provider recorded in cloud Operator configuration. |
| `-s, --execution-vault` (default true) | Enables the execution vault and defaults to `true`. Outbound startup currently requires it; setting it to `false` fails closed during service initialization. |
| `-G, --no-git` | Disables the git-backed file ledger while retaining the encrypted audit store. |
| `-l, --log <level>` | Sets `info`, `error`, or `debug` logging. |
| `--heartbeat-interval <seconds>` | Sets how often the Operator sends heartbeats; the default is 30 seconds and the accepted range is 0-300 (0 selects the default). The Operator declares the interval to the Gateway at session start, and the Gateway marks it `stale` after twice that interval, never less than 60 seconds. Larger values are rejected at startup. |
| `--lattice-endpoint <url>` and related `--lattice-*` flags | These flags are exposed by Cobra but `operatorStartCmd` does not copy their values into `ServeOperatorOptions`, so the flags currently have no effect. The service-layer environment path uses `LATTICE_ENDPOINT`, `LATTICE_CLIENT_ID`, `LATTICE_CLIENT_SECRET`, `SANDBOXES_TOKEN`, `LATTICE_ENTITY_NAME`, and `LATTICE_POSTURE_FLOOR`; the adapter remains incomplete. |
| `--roles <roles>` | Enables a comma-separated, repeatable set of `embedded`, `data`, `inference`, `provenance`, and `observer` capabilities. Defaults to Data for a remote worker; Embedded runs the Gateway in process. |
| `--inference-enabled` | Enables the governed inference backend for an Inference Operator. |
| `--inference-ollama-endpoint <url>` | Selects the approved Ollama provider endpoint used by an inference-enabled Operator. The Operator holds no model configuration; each governed request names the model the user chose in the Console. |
| `--inference-keep-alive <duration>` | Sets the Ollama keep-alive duration (default: -1 for infinite). |
| `--provider-boundary-observer-enabled` | Enrolls a read-only provider-boundary hardware witness with no generic command or provider-lifecycle authority. |
| `--provider-boundary-observer-id <id>` | Sets the stable Observer Operator identity pseudonym. |
| `--provenance-operator-enabled` | Enrolls a storage-side model provenance witness. |
| `--provenance-operator-id <id>` | Sets the stable Provenance Operator identity pseudonym. |
| `--model-storage-root <path>` | Selects the local content-addressed model storage tree read by the Provenance Operator. |

One Inference Operator serves console chat and evaluation campaigns at the same time; authority is decided per request. A request without campaign authority must name the user-selected model carried in the governed request; the Operator holds no role-to-model configuration, and an unknown model is rejected by the provider. A request with campaign authority carries its campaign ID, model registry, and registry digest on the dispatch; the Operator recomputes the digest and rejects malformed registries, absent models, digest changes, and incomplete assignment correlation. Campaign registry digests are lowercase hexadecimal SHA-256 over deterministic protobuf serialization of an `InferenceRequested` containing only the campaign ID and model registry, with registry entries sorted by model and digest. Each entry binds an exact provider tag to its immutable provider digest.

Use `./g8e operator start --help` as the command-surface reference. The Lattice-named flags currently appear in Cobra help but are not copied into `ServeOperatorOptions` by `operatorStartCmd`; setting those flags does not enable the adapter. The adapter's environment-variable path exists in the service layer, but its task handler currently records receipt of a task without dispatching it. Do not treat the Lattice path as an implemented Operator execution integration.

#### Local Runtime State

The reference Operator creates and uses these runtime areas below `.g8e/`:

- `pki/` for the Operator certificate, key, trust bundle, trusted L2 signers, and enrollment state.
- `data/` for the canonical SQLite database, replay store, suspended transactions, execution vault, audit receipts, and commitment chain.
- `vault/` for the encryption vault header.
- `data/ledger/` for git-backed file history when Git integration is enabled.

The canonical runtime file service creates the tree at startup. The canonical database service auto-initializes the vault on first use, seals the vault key in the platform keystore under `.g8e/secrets/vault_key`, and unlocks the vault before opening encrypted stores. Startup fails if an existing key cannot be decrypted or cannot unlock the vault. A separate `g8e vault init` or `g8e vault unlock` step is not required before `operator start`.

The `g8e vault` commands provide explicit administration:

The Operator selects a nonblank `--master-key-file`, then `G8E_MASTER_KEY_FILE` in its own service environment, then its OS keyring. Empty or whitespace-only flags are unset. For remote deployment, explicitly supplied paths refer to the target host; the deployer's local environment path is not forwarded. Provision a separate key for each separately administered runtime. Linux secret mounts require service-readable `0400` or `0600` mode and service-user or root ownership. See [encryption](../architecture/encryption.md) for external-file validation and key continuity requirements.

- `g8e vault init [--master-key-file <path>]`
- `g8e vault unlock [--master-key-file <path>]`
- `g8e vault status`
- `g8e vault rekey [--master-key-file <path>]`
- `g8e vault reset [--confirm]`

`vault unlock` validates that the key in the keystore opens the vault in that process; it does not leave a daemon or persistent unlocked process behind. `operator start` opens and unlocks its own vault instance.

#### Running Multiple Operators on the Same System

The g8e operator is a lightweight binary designed so that multiple instances can run simultaneously on the same host for unique purposes (e.g. an Inference Operator, a Provenance Operator, an Observer Operator, and a Data Operator).

To run multiple operators on the same host without collisions:

1. **Use Distinct Local Directories**: Launch each operator from its own distinct directory (or configure `--working-dir`). Each instance maintains its own isolated `.g8e/` runtime tree, SQLite storage, and enrollment keys.
2. **Accounts and Connectivity**: Operators can run under different user accounts and share the same Gateway endpoint. They connect outbound and do not bind inbound ports.
3. **Role Determination via Flags**: Trigger the specific role using dedicated startup flags:
   - Inference Operator: `--inference-enabled` (plus `--inference-ollama-endpoint`)
   - Provenance Operator: `--provenance-operator-enabled` (plus `--model-storage-root`)
   - Observer Operator: `--provider-boundary-observer-enabled` (plus `--provider-boundary-observer-id`)
   - Data Operator: Default flags (governed tools/command execution)
4. **Collision-Free Composite Fingerprints**: The operator automatically computes a SHA-256 composite `system_fingerprint` incorporating system properties, local directory, launching account, port, and role. This prevents slot collisions on the Gateway and allows the Gateway and AI ensembles to cleanly differentiate operators running on the same host.

### Operate Remote Operators from the CLI

After enrolling the host CLI with `g8e auth enroll user` and starting one or more remote Operators with `g8e operator start --endpoint <gateway-host>`, use these commands from the owner CLI:

#### Discover operators

```bash
./g8e operator list
./g8e operator show <operator-id-or-session-id>
```

`operator list` prints operator ID, type, hostname (decoded from the Gateway-persisted latest heartbeat), session ID, and status. This hostname is live operator telemetry; it is distinct from the enrollment-time `name` metadata shown by `auth enroll list`. The status is evaluated when the list is read: an `active` Operator silent for longer than twice its declared heartbeat interval (at least 60 seconds) is listed as `stale`, and its next heartbeat restores `active`. `operator show` accepts either the operator ID or the session ID from the list and prints operator metadata, the Gateway-recorded last heartbeat time, and the same canonical latest heartbeat snapshot.

#### Bind the CLI session to operators

```bash
./g8e operator bind <operator-session-id> [<operator-session-id>...]
./g8e operator bind list
./g8e operator bind unbind
```

Binding pins the authenticated CLI session to one or more active operator sessions owned by the same user, in one call. Every target is validated before any binding changes; the first is the primary binding. A successful bind issues one replacement CLI session server-side and updates local credentials. Use `bind list` to confirm the current bindings and `bind unbind` to clear them. `g8e auth context` and `GET /api/v1/auth/cli/session` report the persisted binding for automation.

#### Run a governed shell command on one or more operators

```bash
./g8e operator run <operator-session-id> [<operator-session-id>...] \
  --cmd 'echo hello from $(hostname)'
```

`operator run` fans out governed `EXECUTE_BASH` dispatches in parallel through `POST /api/v1/operators/commands`. Each target must belong to the authenticated user and be `active`. The gateway constructs the envelope, publishes to the operator `cmd:` channel, waits for a terminal result, and returns per-target stdout, stderr, exit code, and transaction ID. Use `--timeout` to override the per-operator dispatch timeout (default 30 seconds, maximum 300).

Dispatch is bounded by `--concurrency N` (default: the number of targets, capped at 64), which also sizes the CLI's idle connection pool so a large fan-out reuses TLS connections. `--all-active` targets every `active` session owned by the authenticated user and cannot be combined with explicit session IDs. With `--json`, each result carries `started_at` and `duration_ms`, and the output adds a `summary` object (`targets`, `succeeded`, `failed`, `wall_ms`, `p50_ms`, `p95_ms`, `p99_ms`, `max_ms`, `concurrency`). Percentiles use the nearest-rank method over successful dispatches only. Text output is unchanged.

```bash
./g8e operator run --all-active --concurrency 64 --cmd 'uname -n' --timeout 60 --json
```

This path is the supported owner automation surface for multi-host shell execution. It is distinct from MCP/A2A ingress and from manual `GovernanceEnvelope` submission.

#### Stop or revoke an Operator

`./g8e operator stop <operator-session-id> --reason <reason>` sends a governed `SHUTDOWN` command to one active remote Operator owned by the authenticated user. The Gateway records `stopped` only after the exact session publishes the correlated acknowledgement. The stop retains the workload certificate and enrollment, so the process can start again with the same identity.

`./g8e auth enroll revoke <request-id> --reason <reason> --yes` permanently revokes the identity issued by a completed platform enrollment. Operator revocation invalidates the Operator and companion CLI certificates, deactivates both sessions, marks the Operator `terminated`, and disconnects their active pub/sub channels. Re-enrollment and owner approval are required before that workload can authenticate again.

### Current Operator Processing Contract

#### Ingress and Connectivity

The outbound Operator listens on no inbound application port. It dials the Gateway's mTLS WebSocket pub/sub service, receives bootstrap configuration, and subscribes to `cmd:<operator_id>:<operator_session_id>`. The Gateway owns HTTP MCP, A2A, direct-envelope ingress, policy orchestration, and publication to that scoped channel.

Governed command traffic uses canonical protobuf JSON for `g8e.common.v1.GovernanceEnvelope`. The envelope's `payload` field contains serialized bytes of the protobuf message selected by `action_type`. Unknown protojson fields, unknown action types, missing typed payloads, and non-canonical fallback transports are rejected.

#### L4 Warden Verification

The Operator reserves the nonce before expensive validation so a crash cannot reopen a replay window. It then performs:

1. Expiry and replay checks against the local replay store.
2. Envelope structure, known action type, typed payload decoding, and L1 Doctrine validation.
3. Recalculation of the transaction hash and equality checks against both `transaction_hash` and `id`.
4. State binding to the Gateway admission snapshot already carried in the envelope on the authenticated command channel. The Operator verifies its canonical hash and required proofs without fetching that root or substituting its host-local ledger root. L5 reads local state for receipt evidence.
5. Parsing of the posture carried by the envelope and verification of L2 and L3 evidence. `consensus` and `notary` require L2. `ratify` and `notary` require L3 for mutation action types. Missing optional evidence is recorded as not required rather than treated as a failed gate.

A verification rejection produces deterministic stage evidence and a signed failed receipt when the Actuator and audit dependencies are available. Universal checks and posture-required checks fail closed.

#### L5 Actuator Execution

For a verified transaction, L5 Actuator:

1. Builds, signs, and persists an `EXECUTING` `ActionReceipt`. Execution does not begin if signing or initial persistence fails.
2. Appends a signed `CommitmentAttestation` to the local SQLite commitment hash chain before execution.
3. Rehydrates locally tokenized values and mints a short-lived capability bound to the verified transaction.
4. Dispatches the typed action to the registered execution handler and dissolves the capability after the handler returns.
5. Captures the resulting state root, signs and persists the final `COMPLETED` or `FAILED` receipt, and adds a signed receipt-persistence attestation.
6. Publishes the final receipt to the Gateway on a best-effort basis. The host-local persisted receipt remains authoritative if this mirror publication fails.

Execution output and file-diff records use the encrypted execution vault. Sensitive outbound results pass through the scrubbing service, whose token store uses the encrypted canonical key-value store so token mappings survive process restarts.

#### Identity and Trust

The Operator uses a SPIFFE URI SAN in its mTLS certificate and a host-local Ed25519 Actuator key for `ActionReceipt` signatures. The local Auditor key signs commitment attestations. L2 votes are verified against the Operator's trusted signer store. The Gateway validates client certificate revocation in its authenticated request path; normal TLS chain and hostname validation also applies to the Operator's outbound connection.

### Protocol Packages and Independent Implementations

#### Public Packages

The public Go module is the repository root module:

```bash
go get github.com/g8e-ai/g8e/v2@v2.3.2
```

Generated protocol packages live under `github.com/g8e-ai/g8e/v2/protocol/proto/g8e/...`. The key packages are:

- `protocol/proto/g8e/common/v1` for `GovernanceEnvelope`, governance metadata, L2 votes, and L3 proofs.
- `protocol/proto/g8e/operator/v1` for typed action payloads, `ActionReceipt`, commitment and persistence attestations, deterministic stage evidence, and the generated `OperatorService` gRPC definitions.
- The root `protocol` package for SPIFFE workload-identity formatting, parsing, and matching.

The Python package includes generated protobuf modules, constants, dynamic enums, Pydantic models, and receipt verification helpers:

```bash
pip install g8e==2.3.2
```

See the [Protocol Library architecture document](../architecture/protocol.md) for package contents, schemas, examples, and generation commands.

#### Support Boundary

The protocol packages expose wire schemas and identity helpers; they do not expose the reference governance engine as a reusable Operator SDK. L1 Doctrine, L4 Warden, L5 Actuator, enrollment, pub/sub services, encrypted storage, and execution handlers are under Go `internal/` packages and cannot be imported by an external Go module.

The repository also does not provide a standalone third-party Operator conformance harness or a compatibility certification command. The platform test suite validates the in-tree reference implementation. An independent Operator therefore requires a separate implementation of enrollment, scoped pub/sub, canonical protojson handling, transaction hashing, replay and expiry checks, Gateway state-root verification, posture-aware L2/L3 verification, signed receipts and commitment attestations, encrypted local persistence, scrubbing, and typed action dispatch. Schema compatibility alone does not establish behavioral compatibility.

The generated `OperatorService` gRPC interface describes command operations, but the current outbound reference worker receives governed envelopes through Gateway pub/sub rather than serving that gRPC service on an inbound port. Independent implementations that interoperate with the current Gateway follow the pub/sub envelope and receipt flow implemented by the reference worker.

### Verify the Reference Implementation

Run platform tests through the `g8e test` command or the corresponding root targets:

| Command | Scope |
| --- | --- |
| `./g8e test unit` | Tier 1 unit tests. |
| `./g8e test integration` | Tier 2 in-process integration tests with SQLite, PKI, and local pub/sub. |
| `./g8e test e2e` | Tier 3 E2E tests against a running, enrolled platform. |
| `./g8e test coverage` | Unit and integration coverage with the 75% threshold. |
| `./g8e test lint` | Platform lint and quality checks. |

The matching root targets are `make test-unit`, `make test-integration`, `make test-docker`, `make test-coverage`, and `make lint`. `make test` runs unit and integration tests. `make test-docker` runs the configured steady-state E2E subset and requires a running native or Docker platform with its enrollment requests approved. `make ci` runs the full Console/adapter, platform, protocol, Ensemble, website, and script checks.

### Deployment Commands

`g8e operator cp <target>` copies the currently running binary to a local file or directory. `g8e operator scp <user@host:path>` invokes the system `scp` command and supports the flags shown by `g8e operator scp --help`.

`g8e operator deploy --local --dest-dir <dir> --count N --roles data --background --approve --endpoint <gateway-host>` deploys isolated Operators on this system. Replace `--local` with `--hosts <host[,host...]>` for SSH deployment, or use `--docker-context <context> --docker-image <image> --dest-dir </absolute/container/path>` for one persistent volume and container per Operator. `--remote-dir` remains an alias for `--dest-dir`. The command installs one shared binary per SSH/local host or resolves one immutable image ID per Docker batch, uses separate directories and identities, and supports ranges of up to 5000 Operators with `--count` and `--start-index`. `--parallel` bounds staging concurrency (default and maximum 2048, the Gateway's live Operator request budget), so a cohort up to that size stages at once. Role flags are forwarded to `operator start`; supported roles are data, provenance, inference, and observer. With `--approve`, the command stages the entire cohort, approves only its own fingerprint-bound requests in one atomic batch, and verifies all sessions are active. A staging failure prevents automatic approval; without `--approve`, it returns the staged requests for owner review. Build the shared Gateway/Operator image on an explicit context using the root Dockerfile in [Deploy to a Docker Context](connect_operator_to_gateway.md#deploy-to-a-docker-context). See [Connect Many Operators](connect_operator_to_gateway.md#connect-many-operators) for batch growth, role settings, Docker lifecycle, and staging behavior.

The current Cobra wrapper for `operator stream` parses its public flags before calling the native stream parser, so options such as `--endpoint`, `--hosts`, and `--binary-dir` are not forwarded to the implementation. Do not use `operator stream` as an automated Operator rollout path in this version.

## Anti-patterns

- Treating the Lattice adapter path as implemented (INV-BUILD-OP-02: flags parse but do not affect runtime).
- Using `./g8e gw clean` to clean build artifacts on a host whose operational state must be retained (INV-BUILD-OP-04).
- Assuming protocol package re-exports guarantee behavioral compatibility (INV-BUILD-OP-05: independent implementations require full L1-L5 stack).
- Relying on `operator stream` for production rollout (its flags are not forwarded; use `operator deploy` or an external deployment system).

## Links out

- [Connect Operator to Gateway](connect_operator_to_gateway.md): Enrollment, connection, health checks, and operation.
- [Operator Architecture](../architecture/operator.md): Service stack, execution boundary, audit stores, and tool handling.
- [Protocol Library](../architecture/protocol.md): Public Go and Python protocol packages.
- [Build Apps](build_apps.md): MCP, A2A, command-intent, and direct-envelope clients of the Gateway.
