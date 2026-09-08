---
title: Build Operator
parent: Guides
---

# Build and Run a g8e Operator

Last Updated: 2026-09-08
Version: v2.1.7

---

## Overview

The Governed Operator is the host-side Policy Execution Point (PEP). It opens an outbound mTLS WebSocket connection to a g8e Gateway, subscribes to its identity- and session-scoped command channel, decodes canonical protojson `GovernanceEnvelope` transactions, re-runs L1 Doctrine, verifies the universal transaction checks and posture-required L2 and L3 proofs in L4 Warden, and dispatches verified actions through L5 Actuator.

The reference Gateway and Operator compile into the same `g8e` binary. `g8e gw start` runs the Gateway Policy Decision Point (PDP), including the client-facing MCP and A2A endpoints. `g8e operator start` runs the outbound Operator worker. The outbound Operator does not expose an MCP or A2A listener; the Gateway translates client requests into governed transactions and publishes them to the bound Operator.

This guide covers building and running the reference Operator and identifies the public protocol surface available to independent implementations. For connection, enrollment, and day-two operations, see [Connect Operator to Gateway](connect_operator_to_gateway.md). For the complete service architecture, see [Operator Architecture](../architecture/operator.md).

---

## Build the Reference Operator

### Prerequisites

- **Go 1.26.6 or later**, as declared by the root Go module.
- **Make** on Linux and macOS.
- **PowerShell** for the native Windows build script.

The repository setup scripts validate the development tools, offer to install missing tools, and run a build:

- Linux: `bash scripts/linux-setup.sh`
- macOS: `bash scripts/macos-setup.sh`
- Windows: `pwsh scripts/windows-setup.ps1`

### Build from Source

```bash
git clone https://github.com/g8e-ai/g8e.git
cd g8e
make build
```

`make build` creates the platform-specific binary and checksum under `bin/`, copies the host binary to `./g8e` (or `./g8e.exe` on Windows), and copies it to `demos/bin/g8e`.

The build sets `CGO_ENABLED=0`, uses the `netgo` and `osusergo` build tags, strips symbol and debug data, and embeds the platform version, build ID, build time, and target platform. The resulting binary does not require a Go toolchain or a system SQLite library on the target host.

The binary is self-contained, but the running Operator is stateful. It creates a `.g8e/` runtime tree below its launch directory, requires write access there, stores enrollment credentials and encrypted local state there, and requires network access to its Gateway. Use `--working-dir` to select the host execution directory; it does not relocate the `.g8e/` runtime tree.

### Build Targets

| Target | Result |
| --- | --- |
| `make build` | Builds the current OS and architecture, writes `bin/g8e-<os>-<arch>`, and copies the host binary to the repository root. |
| `make build-all` | Builds Linux amd64/arm64/386, Windows amd64/arm64, and Darwin amd64/arm64 binaries with SHA-256 checksum files. Linux variants use the pinned FIPS module. |
| `make build-linux` | Builds `bin/g8e-linux-{amd64,arm64,386}` and checksum files. |
| `make build-windows` | Builds `bin/g8e-windows-{amd64,arm64}.exe` and checksum files. |
| `make build-darwin` | Builds `bin/g8e-darwin-{amd64,arm64}` and checksum files. |
| `make build-compressed` | Builds the host binary and compresses it with UPX. This target requires UPX. |
| `make build-fips` | Builds `bin/g8e-fips-linux-amd64` with `GOFIPS140=v1.0.0`. |
| `make verify-fips` | Builds the FIPS variant and runs `g8e version --fips` with FIPS-only enforcement enabled. |

`make fmt`, `make up`, `make down`, and the cleanup targets are development and platform-management targets rather than Operator build variants. `make clean` also removes `.g8e/` runtime state, so do not use it to clean only build artifacts on a host with state that must be retained.

### Cross-Compilation

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

On a native Windows host, use the repository's `build.ps1` workflow or WSL. The root Makefile itself directs Windows users to `build.ps1`.

---

## Start the Reference Operator

### Start a Gateway

The Operator requires a reachable Gateway for enrollment, bootstrap configuration, state-root verification, pub/sub, and receipt publication. Start the Gateway first:

```bash
./g8e gw start
```

The default Gateway posture is `doctrine`. The Gateway also supports `consensus`, `ratify`, and `notary`; the selected posture is written into each envelope and is authoritative for Operator-side L2 and L3 gating.

### Start and Enroll the Operator

On the target host, start the Operator with the Gateway discovery endpoint:

```bash
./g8e operator start --endpoint <gateway-host>
```

When no installed Operator credentials exist, `--endpoint` starts the owner-approved platform enrollment protocol. The Operator fetches the Gateway trust bundle from the HTTP discovery endpoint, creates Operator and CLI certificate requests, persists pending enrollment state, waits for Gateway-owner approval, verifies the completion transcript, and writes the issued credentials and canonical trust bundle into `.g8e/pki/`. Restarting the process resumes the same pending enrollment request and key material.

After enrollment, the Operator loads `.g8e/pki/operator.crt` and `.g8e/pki/operator.key`, connects to the Gateway over mTLS, requests bootstrap configuration, initializes encrypted local services, subscribes to its command channel, and starts automatic heartbeats. The canonical trust bundle is `.g8e/pki/trust/g8eg-ca-bundle.pem`.

For pre-provisioned credentials, pass explicit paths:

```bash
./g8e operator start \
  --endpoint <gateway-host> \
  --cert /path/to/operator.crt \
  --key /path/to/operator.key \
  --trust-bundle /path/to/g8eg-ca-bundle.pem
```

### Operator Start Options

The current worker path applies these options:

| Option | Behavior |
| --- | --- |
| `-e, --endpoint <host>` | Selects the Gateway discovery host. If omitted, the default is `localhost`; a missing local trust bundle then prevents startup. |
| `--cert <path>` | Uses an explicit Operator client certificate instead of runtime-tree discovery or enrollment. |
| `-k, --key <path>` | Uses the private key paired with `--cert`. |
| `--trust-bundle <path>` | Loads an explicit CA trust bundle. With an endpoint and no local bundle, the Operator fetches the Gateway bundle from its well-known HTTP endpoint. |
| `--working-dir <path>` | Sets the working directory used by command execution. The default comes from runtime configuration and resolves to the process working directory. |
| `-c, --cloud` | Enables cloud Operator mode. |
| `--provider <aws|gcp|azure>` | Sets the cloud provider recorded in cloud Operator configuration. |
| `-s, --execution-vault` | Enables the execution vault and defaults to `true`. Outbound startup currently requires it; setting it to `false` fails closed during service initialization. |
| `-G, --no-git` | Disables the git-backed file ledger while retaining the encrypted audit store. |
| `-l, --log <level>` | Sets `info`, `error`, or `debug` logging. |
| `--heartbeat-interval <seconds>` | Sets the heartbeat interval; the default is 30 seconds. |

Use `./g8e operator start --help` as the command-surface reference. The Lattice-named flags currently appear in Cobra help but are not copied into `ServeOperatorOptions` by `operatorStartCmd`; setting those flags does not enable the adapter. The adapter's environment-variable path exists in the service layer, but its task handler currently records receipt of a task without dispatching it. Do not treat the Lattice path as an implemented Operator execution integration.

### Local Runtime State

The reference Operator creates and uses these runtime areas below `.g8e/`:

- `pki/` for the Operator certificate, key, trust bundle, trusted L2 signers, and enrollment state.
- `data/` for the canonical SQLite database, replay store, suspended transactions, execution vault, audit receipts, and commitment chain.
- `vault/` for the encryption vault header and key.
- `data/ledger/` for git-backed file history when Git integration is enabled.

The canonical runtime file service creates the tree at startup. The canonical database service auto-initializes the vault on first use, writes `.g8e/vault/key`, and unlocks the vault before opening encrypted stores. Startup fails if an existing key cannot be read or cannot unlock the vault. A separate `g8e vault init` or `g8e vault unlock` step is not required before `operator start`.

The `g8e vault` commands provide explicit administration:

- `g8e vault init [--vault-dir <dir>] [--key-path <path>]`
- `g8e vault unlock [--vault-dir <dir>] [--key-path <path>]`
- `g8e vault status [--vault-dir <dir>]`
- `g8e vault rekey [--vault-dir <dir>] [--key-path <path>] [--new-key-path <path>]`
- `g8e vault export [--key-path <path>]`
- `g8e vault import [--key-path <path>] [--key-hex <hex>]`
- `g8e vault reset [--vault-dir <dir>] [--confirm]`

`vault unlock` validates that a key opens the vault in that process; it does not leave a daemon or persistent unlocked process behind. `operator start` opens and unlocks its own vault instance.

---

## Current Operator Processing Contract

### Ingress and Connectivity

The outbound Operator listens on no inbound application port. It dials the Gateway's mTLS WebSocket pub/sub service, receives bootstrap configuration, and subscribes to `cmd:<operator_id>:<operator_session_id>`. The Gateway owns HTTP MCP, A2A, direct-envelope ingress, policy orchestration, and publication to that scoped channel.

Governed command traffic uses canonical protobuf JSON for `g8e.common.v1.GovernanceEnvelope`. The envelope's `payload` field contains serialized bytes of the protobuf message selected by `action_type`. Unknown protojson fields, unknown action types, missing typed payloads, and non-canonical fallback transports are rejected.

### L4 Warden Verification

The Operator reserves the nonce before expensive validation so a crash cannot reopen a replay window. It then performs:

1. Expiry and replay checks against the local replay store.
2. Envelope structure, known action type, typed payload decoding, and L1 Doctrine validation.
3. Recalculation of the transaction hash and equality checks against both `transaction_hash` and `id`.
4. State binding against the current Gateway state root in outbound mode. The Operator fetches this root from the Gateway rather than treating its host-local ledger root as authoritative for the envelope.
5. Parsing of the posture carried by the envelope and verification of L2 and L3 evidence. `consensus` and `notary` require L2. `ratify` and `notary` require L3 for mutation action types. Missing optional evidence is recorded as not required rather than treated as a failed gate.

A verification rejection produces deterministic stage evidence and a signed failed receipt when the Actuator and audit dependencies are available. Universal checks and posture-required checks fail closed.

### L5 Actuator Execution

For a verified transaction, L5 Actuator:

1. Builds, signs, and persists an `EXECUTING` `ActionReceipt`. Execution does not begin if signing or initial persistence fails.
2. Appends a signed `CommitmentAttestation` to the local SQLite commitment hash chain before execution.
3. Rehydrates locally tokenized values and mints a short-lived capability bound to the verified transaction.
4. Dispatches the typed action to the registered execution handler and dissolves the capability after the handler returns.
5. Captures the resulting state root, signs and persists the final `COMPLETED` or `FAILED` receipt, and adds a signed receipt-persistence attestation.
6. Publishes the final receipt to the Gateway on a best-effort basis. The host-local persisted receipt remains authoritative if this mirror publication fails.

Execution output and file-diff records use the encrypted execution vault. Sensitive outbound results pass through the scrubbing service, whose token store uses the encrypted canonical key-value store so token mappings survive process restarts.

### Identity and Trust

The Operator uses a SPIFFE URI SAN in its mTLS certificate and a host-local Ed25519 Actuator key for `ActionReceipt` signatures. The local Auditor key signs commitment attestations. L2 votes are verified against the Operator's trusted signer store. The Gateway validates client certificate revocation in its authenticated request path; normal TLS chain and hostname validation also applies to the Operator's outbound connection.

---

## Protocol Packages and Independent Implementations

### Public Packages

The public Go module is the repository root module:

```bash
go get github.com/g8e-ai/g8e/v2@v2.1.7
```

Generated protocol packages live under `github.com/g8e-ai/g8e/v2/protocol/proto/g8e/...`. The key packages are:

- `protocol/proto/g8e/common/v1` for `GovernanceEnvelope`, governance metadata, L2 votes, and L3 proofs.
- `protocol/proto/g8e/operator/v1` for typed action payloads, `ActionReceipt`, commitment and persistence attestations, deterministic stage evidence, and the generated `OperatorService` gRPC definitions.
- The root `protocol` package for SPIFFE workload-identity formatting, parsing, and matching.

The Python package includes generated protobuf modules, constants, dynamic enums, Pydantic models, and receipt verification helpers:

```bash
pip install g8e==2.1.7
```

See [Protocol Library](../architecture/protocol.md) for package contents, schemas, examples, and generation commands.

### Support Boundary

The protocol packages expose wire schemas and identity helpers; they do not expose the reference governance engine as a reusable Operator SDK. L1 Doctrine, L4 Warden, L5 Actuator, enrollment, pub/sub services, encrypted storage, and execution handlers are under Go `internal/` packages and cannot be imported by an external Go module.

The repository also does not provide a standalone third-party Operator conformance harness or a compatibility certification command. The platform test suite validates the in-tree reference implementation. An independent Operator therefore requires a separate implementation of enrollment, scoped pub/sub, canonical protojson handling, transaction hashing, replay and expiry checks, Gateway state-root verification, posture-aware L2/L3 verification, signed receipts and commitment attestations, encrypted local persistence, scrubbing, and typed action dispatch. Schema compatibility alone does not establish behavioral compatibility.

The generated `OperatorService` gRPC interface describes command operations, but the current outbound reference worker receives governed envelopes through Gateway pub/sub rather than serving that gRPC service on an inbound port. Independent implementations that interoperate with the current Gateway follow the pub/sub envelope and receipt flow implemented by the reference worker.

---

## Verify the Reference Implementation

Run platform tests through the `g8e test` command or the corresponding root targets:

| Command | Scope |
| --- | --- |
| `./g8e test unit` | Tier 1 unit tests. |
| `./g8e test integration` | Tier 2 in-process integration tests with SQLite, PKI, and local pub/sub. |
| `./g8e test e2e` | Tier 3 Docker E2E tests against a running, enrolled platform. |
| `./g8e test coverage` | Unit and integration coverage with the 75% threshold. |
| `./g8e test lint` | Platform lint and quality checks. |

The matching root targets are `make test-unit`, `make test-integration`, `make test-docker`, `make test-coverage`, and `make lint`. `make test` runs unit and integration tests. `make test-docker` runs the configured steady-state E2E subset and requires the Docker platform to be running and its enrollment requests approved. `make ci` runs the full platform, ensemble, dashboard, protocol-generation, documentation-generation, lint, vulnerability, and test pipeline.

---

## Deployment Commands

`g8e operator cp <target>` copies the currently running binary to a local file or directory. `g8e operator scp <user@host:path>` invokes the system `scp` command and supports the flags shown by `g8e operator scp --help`.

The current `operator deploy --background` implementation copies the binary and starts `gw start` on each remote host; it does not start `operator start`. The current Cobra wrapper for `operator stream` parses its public flags before calling the native stream parser, so options such as `--endpoint`, `--hosts`, and `--binary-dir` are not forwarded to the implementation. Do not use either command as an automated Operator rollout path in this version. Copy the binary with `cp`, `scp`, or an external deployment system, then run `g8e operator start --endpoint <gateway-host>` on the target.

---

## Next Steps

- [Connect Operator to Gateway](connect_operator_to_gateway.md) covers enrollment, connection, health checks, and operation.
- [Operator Architecture](../architecture/operator.md) describes the service stack, execution boundary, audit stores, and tool handling.
- [Protocol Library](../architecture/protocol.md) documents the public Go and Python protocol packages.
- [Build Apps](build_apps.md) covers MCP, A2A, command-intent, and direct-envelope clients of the Gateway.
