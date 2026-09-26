---
doc_id: devs
title: Developer Guidelines
audience: maintainers and coding agents
status: current
last_updated: 2026-09-26
version: v2.1.13
owners:
  - go.mod
  - Makefile
  - docker-compose.yml
  - internal/cli/cmd/main.go
  - internal/cli/cmd/test/test.go
  - internal/constants/errors.go
  - internal/constants/paths.go
  - internal/services/fs/file_service.go
  - internal/services/pubsub/mode_deps.go
  - internal/services/pubsub/pubsub_commands.go
  - internal/services/gateway/gateway_service.go
  - internal/services/gateway/embedded/operator.go
  - internal/services/governance/l1_doctrine.go
  - internal/services/mcp/registry.go
  - internal/services/mcp/native_tool_registry.go
related:
  - docs/devs/codemap.md
  - docs/devs/tests.md
  - docs/devs/docs.md
  - docs/devs/release_process.md
  - docs/architecture/governance.md
  - docs/architecture/agents.md
when_to_read: Changing Go platform code, CLI commands, runtime files, governance construction, native MCP tools, or doctrine loading.
do_not_use_for:
  - Package and runtime ownership maps (docs/devs/codemap.md)
  - Test selection, fixtures, CI, and lifecycle detail (docs/devs/tests.md)
  - Documentation audit, catalog, and generation ownership (docs/devs/docs.md)
  - Release, native evaluation acceptance, and signed evidence (docs/devs/release_process.md)
  - Deployment and enrollment procedures (docs/guides/getting_started.md)
---

# Developer Guidelines

## Purpose

Coding invariants for the g8e Go platform. The current tree is the source of truth. Command names, flags, defaults, and destructive effects come from `./g8e <command> --help`; this file is a map, not a flag dump.

## Quick index

- [Purpose](#purpose)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

Invariant groups: [Boundaries](#boundaries-inv-bound), [Environment](#environment-inv-env), [CLI layout](#cli-layout-inv-cli), [Code](#code-inv-code), [Errors](#errors-inv-err), [Typed contracts](#typed-contracts-inv-type), [Runtime files](#runtime-files-inv-fs), [Dependency construction](#dependency-construction-inv-dep), [Testing](#testing-inv-test), [Generated artifacts](#generated-artifacts-inv-gen), [Doctrine](#doctrine-inv-doctrine), [Native MCP](#native-mcp-inv-mcp), [Contribution](#contribution-inv-contrib).

## Invariants

Ids are stable. Append the next free number in a topic. Do not renumber.

### Boundaries (`INV-BOUND`)

| ID | Rule |
| --- | --- |
| INV-BOUND-01 | A governed operation MUST enter as typed intent or a canonical protobuf `GovernanceEnvelope`. The active posture determines whether L2 consensus and L3 notary evidence gate execution. L1 doctrine, L4 verification, and L5 actuation stay on every governed execution path. |
| INV-BOUND-02 | The governance guarantee MUST apply only to operations that traverse a g8e ingress. An external MCP wrapper performs inline L1 screening and MUST NOT be described as adding an envelope, L2 through L5, a signed receipt, or Gateway audit. Client-native tools and other side channels stay outside the boundary. |
| INV-BOUND-03 | A mutation added to a governed ingress MUST be classified with the canonical action and event registries, represented by a typed payload, wrapped in a `GovernanceEnvelope`, verified by L4, and dispatched by L5. MUST NOT call mutation handlers directly to skip envelope construction or the verification gauntlet. |
| INV-BOUND-04 | The Gateway in-process Operator substrate MUST stay in `internal/services/gateway/embedded/`. The outbound Operator runtime is `G8eoService` in `internal/services/g8eo.go`. `G8eoService` MUST NOT construct `mcp.GatewayService`. |

### Environment (`INV-ENV`)

| ID | Rule |
| --- | --- |
| INV-ENV-01 | The Go toolchain MUST satisfy the `go` line in `go.mod` (1.26.6). Setup scripts read that line and also check `git`, `make`, Node.js 22+, and `npm`. |
| INV-ENV-02 | Commands in this guide MUST be run from the repository root unless the owning component guide says otherwise. |
| INV-ENV-03 | CLI behavior MUST be taken from `./g8e <command> --help`. This file MUST NOT grow a flag inventory. |

### CLI layout (`INV-CLI`)

| ID | Rule |
| --- | --- |
| INV-CLI-01 | A new Cobra group MUST be a package under `internal/cli/cmd/<group>/`. The root command stays in `internal/cli/cmd/main.go`. Shared non-group helpers stay in `internal/cli/cmd/shared/`. The group inventory lives in the [Code Map](codemap.md#cli-packages). |

### Code (`INV-CODE`)

| ID | Rule |
| --- | --- |
| INV-CODE-01 | MUST replace a broken path. MUST NOT preserve a compatibility shim for technical debt. |
| INV-CODE-02 | MUST fix the root cause. MUST NOT add a caller guard that hides invalid construction or state. |
| INV-CODE-03 | A function MUST do one job: reads read, writes write, validation validates, and orchestration composes explicit dependencies. |
| INV-CODE-04 | A security check MUST fail closed and MUST propagate its error. |
| INV-CODE-05 | MUST pass explicit dependencies and state transitions. MUST NOT add package globals, lazy adapters, reflection, or hidden side effects for control flow. |
| INV-CODE-06 | A production path MUST return errors. MUST NOT panic for a recoverable production failure. |
| INV-CODE-07 | MUST use `context.Context` for cancellation. Every goroutine MUST have an owner, cancellation, and completion via channels or `sync.WaitGroup`. |
| INV-CODE-08 | Go MUST be formatted with `gofmt`. Imports MUST be grouped as standard library, external modules, then internal packages. |
| INV-CODE-09 | MUST pass a pointer for a mutable or large struct and a value for a small read-only struct. |
| INV-CODE-10 | MUST confirm a dependency is already required before importing it. A new dependency MUST be added through the owning package manager. MUST NOT hand-edit a lockfile or manifest entry. |
| INV-CODE-11 | MUST search for an existing implementation before adding code. MUST extend the existing service, utility, model, constant, or pattern. MUST NOT add a helper, shim, wrapper, or compatibility layer that duplicates one. |
| INV-CODE-12 | MUST NOT add an `ensure*` or `getOrCreate*` helper that combines a read and a write or hides creation as a lookup side effect. |
| INV-CODE-13 | MUST reproduce a bug with a failing regression test before changing production code, then show that the test passes with the fix. |
| INV-CODE-14 | MUST update the documentation and generated artifacts in the same change as the behavior they describe. |
| INV-CODE-15 | A change SHOULD stay on one coherent behavior and SHOULD leave the affected code easier to follow. |

### Errors (`INV-ERR`)

| ID | Rule |
| --- | --- |
| INV-ERR-01 | Sentinel errors and distinct reusable platform failure modes MUST live in `internal/constants/errors.go`. MUST NOT declare a package-level sentinel error anywhere else. |
| INV-ERR-02 | MUST NOT use `errors.New` for a distinct production failure mode outside `internal/constants/errors.go`. |
| INV-ERR-03 | Before adding an error, MUST search `internal/constants/errors.go` and existing callers for an equivalent constant. Add a centralized constant only when none fits, then replace duplicate hand-written forms in the affected scope. |
| INV-ERR-04 | MUST wrap an error with operation context and preserve the cause with `%w`, for example `fmt.Errorf("gateway: load doctrine: %w", err)`. |
| INV-ERR-05 | MUST use a centralized `constants.Err*` for a known failure mode, `fmt.Errorf` with runtime values for a dynamic error, and a one-off `fmt.Errorf` in tests only when the injected cause has no production meaning. |
| INV-ERR-06 | MUST NOT compare rendered error strings when `errors.Is`, `errors.As`, or a typed status is available. |

### Typed contracts (`INV-TYPE`)

| ID | Rule |
| --- | --- |
| INV-TYPE-01 | A known shape MUST use a typed model. MUST NOT replace a protobuf or domain model with an untyped map or an ad hoc JSON object. |
| INV-TYPE-02 | MUST NOT use protobuf `Any`, `map[string]interface{}`, or an equivalent untyped container for a known contract. |
| INV-TYPE-03 | Schemas under `protocol/proto/g8e/` own governance envelopes, proofs, Operator messages, pub/sub messages, receipts, and compliance messages. Governance and transport code MUST keep those payloads typed. |
| INV-TYPE-04 | An internal envelope payload MUST use protobuf binary encoding where the execution path expects it. A protobuf-owned JSON boundary MUST use `protojson`. |
| INV-TYPE-05 | A non-protobuf HTTP surface MUST use named request and response structs. |
| INV-TYPE-06 | Runtime Go constants live under `internal/constants/`. External registries and schemas live under `protocol/constants/`, `protocol/models/`, and `protocol/schemas/`. When a public registry has both a Go and a JSON representation, MUST update both through the established owner and run the owning contract or conformance tests. |

### Runtime files (`INV-FS`)

| ID | Rule |
| --- | --- |
| INV-FS-01 | `.g8e/` state MUST go through `RuntimeFileService` in `internal/services/fs/file_service.go`. MUST NOT hardcode a `.g8e/` runtime path or call `os` file operations for that state outside `internal/services/fs`. |
| INV-FS-02 | Reusable system, repository, and runtime path strings MUST be defined in `internal/constants/paths.go`. Consumers MUST NOT introduce an inline `.g8e/` fragment or a `filepath.Join` of path literals. |
| INV-FS-03 | A service or function that owns runtime I/O MUST receive `fs.RuntimeFileService` as an explicit argument. |
| INV-FS-04 | MUST use `fileSvc.Resolve` only when an API requires an absolute path. MUST use `fileSvc.Rel` to turn an absolute path inside the runtime root back into a relative service path. |
| INV-FS-05 | An existence check MUST use `fileSvc.FileExists`. A missing-file error MUST be compared with `errors.Is(err, constants.ErrNotFound)`. |
| INV-FS-06 | Runtime file and directory permissions MUST use `constants.Perm*` values. |
| INV-FS-07 | Gateway startup configuration carries `DataDir`, `PKIDir`, `SecretsDir`, and `VaultDir` (`internal/cli/serve/gateway.go`). Those absolute paths MUST stay at the CLI and startup boundary. Service code MUST receive the file service. MUST NOT add a duplicate directory field only to route runtime I/O. |
| INV-FS-08 | A command constructor under `internal/cli/cmd/<group>/` that touches runtime state MUST accept a file-service factory. Factory initialization failure MUST wrap `constants.ErrFileServiceInit` and preserve the underlying error. Every new factory injection point MUST have a matching case in `internal/cli/cmd/<group>/factory_error_<group>_test.go` that proves downstream dependencies are not called. |
| INV-FS-09 | Tests MUST use `testutil.TempDir` and `testutil.TestPaths` for isolated roots. `testutil.TempDir` returns an absolute base directory. Pass that directory to `fs.NewRuntimeFileService` and `paths.InitWithBase`. MUST NOT append `.g8e` yourself. |

Consumers pass relative paths from `internal/constants/paths.go` into `ReadFile`, `WriteFile`, `Stat`, `FileExists`, `ReadDir`, `Rename`, `Remove`, `RemoveAll`, and the other `RuntimeFileService` methods. Startup constructs the service and calls `CreateRuntimeTree` (`internal/cli/serve/gateway.go`, `internal/cli/serve/operator.go`).

### Dependency construction (`INV-DEP`)

| ID | Rule |
| --- | --- |
| INV-DEP-01 | Shared governance dependencies MUST be `pubsub.GovernanceCoreDeps`. Gateway-only document, consensus, field-read, platform-enrollment, and posture dependencies MUST be added on `pubsub.GatewayModeDeps`. Outbound mode MUST use `pubsub.OutboundModeDeps`, which embeds only the shared fields. |
| INV-DEP-02 | `G8eoService` MUST build outbound dependencies with `NewOutboundModeDeps`. That constructor validates the outbound core before `OperatorPubSubService` is constructed. |
| INV-DEP-03 | The production Gateway builder MUST construct `GatewayModeDeps` directly and pass it to `NewGatewayOperatorPubSubService`. `NewGatewayModeDeps` validates a bundle when something calls it. Code on the production builder path MUST NOT assume that validating constructor ran. Keep the builder's explicit non-nil wiring and the command-service constructor checks. |
| INV-DEP-04 | `OperatorPubSubService.BindMCPGateway` MUST run once, before either service starts. It MUST reject nil (`constants.ErrPubSubMCPGatewayNil`), duplicate (`constants.ErrPubSubMCPGatewayAlreadyBound`), and post-start (`constants.ErrPubSubMCPGatewayBindAfterStart`) binding. MUST NOT reintroduce a construction-time setter for audit, consensus, or session validation. |
| INV-DEP-05 | Posture-dependent L2 MUST stay optional only where the posture does not require it. A required posture dependency MUST fail closed. |
| INV-DEP-06 | Gateway governance wiring MUST follow the order in [Gateway governance construction](#gateway-governance-construction). |

### Testing (`INV-TEST`)

Selection, timeouts, race settings, fixtures, and CI scope live in the [Testing Guide](tests.md). These rules still bind platform changes.

| ID | Rule |
| --- | --- |
| INV-TEST-01 | A platform suite MUST run through `./g8e test ...` or the owning Makefile target. MUST NOT invoke `go test` directly for a platform suite. |
| INV-TEST-02 | MUST NOT use `t.Parallel()` in an integration or E2E test. |
| INV-TEST-03 | Go tests SHOULD be table-driven and SHOULD use `testify/assert` and `testify/require` when an assertion library helps. |
| INV-TEST-04 | Test functions, subtests, and files MUST be named for the behavior they verify. MUST NOT use a generic `coverage`, `gap`, `edge`, `misc`, `success`, or `error` name as the scope. |
| INV-TEST-05 | Tier 1 MUST stay independent of a running platform. Tier 2 and Tier 3 MUST exercise real local boundaries. MUST NOT mock an internal service, a database client, or cross-component communication in those tiers. |
| INV-TEST-06 | `NewGatewayFixture` registers teardown. MUST NOT stop its Gateway, close its databases, or close its downstream server a second time. |
| INV-TEST-07 | Temporary credential and long-lived fixture cleanup MUST be registered with `t.Cleanup`. MUST NOT use a helper-local `defer` that runs before the test body. |
| INV-TEST-08 | A test MUST use an explicit cancellation context and MUST join goroutines before it returns. |
| INV-TEST-09 | Assertions MUST use typed constants for statuses, reasons, paths, and permissions. |
| INV-TEST-10 | The canonical trust bundle path is `.g8e/pki/trust/g8eg-ca-bundle.pem`. A test MUST NOT repair a failure by mutating developer PKI state. |
| INV-TEST-11 | MUST NOT use `os.Chdir` to line up runtime state. A working-directory change is allowed only for behavior that discovers source-tree or configuration files, and that file MUST explain the change and clean it up. |
| INV-TEST-12 | `./g8e test e2e-full` MUST be described from `internal/cli/cmd/test/test.go`: it runs `docker compose up -d` with profile name `bootstrapped` (`constants.DockerBootstrappedProfile`), adds profile `cross-enrollment` when `--cross-enrollment` is set, waits up to 60 seconds for HTTP 200 from Gateway `http://localhost:8080/api/v1/health` and Ensemble `http://localhost:8000/health`, runs the same `go test` arguments as `./g8e test e2e`, and tears the stack down with `docker compose down -v`. |
| INV-TEST-13 | `docker-compose.yml` has no `bootstrapped` profile and no `evaluation` profile. Unprofiled services (gateway, data operator, inference operator, ensemble, dashboard) start on `docker compose up -d`. Named profiles are `cross-enrollment` and `g8ellama`. MUST NOT describe `bootstrapped` as a Compose profile that selects those workloads. `constants.DockerBootstrappedProfile` and `constants.DockerEvaluationProfile` still exist; the Compose file does not assign them. |

### Generated artifacts (`INV-GEN`)

| ID | Rule |
| --- | --- |
| INV-GEN-01 | Generated protobuf reference and OpenAPI output MUST change only through the source owner and its generator. MUST NOT hand-edit those outputs. |
| INV-GEN-02 | Current-state documentation MUST move with the behavior it describes. MUST NOT broaden a security or evidence claim past the path and artifacts that support it. |

| Output | Source | Update |
| --- | --- | --- |
| Root `README.md` | Handwritten product overview | Re-read the changed sections, check links, and run every command whose behavior the prose states |
| Go, Python, TypeScript, and Markdown protobuf output | `protocol/proto/g8e/` | `make proto` (`make generate` depends on `proto`) plus the affected conformance tests |
| Gateway OpenAPI | Swagger annotations in the Go owners | `make swagger-generate` plus route and contract tests |
| Website | Root `README.md` | `make website-test` and `make website-build` when rendering changes |
| Doctrine references | `protocol/constants/doctrine/` and demo doctrine inputs | `make validate-doctrines` |
| COSAiS overlays | Canonical overlay and doctrine references | `make validate-cosais` |

The [Documentation Guide](docs.md#generated-and-machine-readable-documentation) owns the full matrix. The [Release Process](release_process.md) owns native evaluation and signed compliance evidence.

### Doctrine (`INV-DOCTRINE`)

| ID | Rule |
| --- | --- |
| INV-DOCTRINE-01 | `governance.NewL1DoctrineFromDir` MUST remain the loader: built-in MITRE-oriented detectors, plus enabled entries from `*.json` files in the configured directory. An empty directory argument falls back to `NewL1Doctrine()`. |
| INV-DOCTRINE-02 | `./g8e gw start --doctrine-dir <path>` sets that directory. `G8E_DOCTRINE_DIR` supplies it when the flag is absent. The Gateway loads doctrine during construction, so a runtime doctrine change MUST be followed by a Gateway restart. |
| INV-DOCTRINE-03 | Reference JSON under `protocol/constants/doctrine/` MUST pass `make validate-doctrines`. An identifier or public-shape change MUST update the owning JSON and the affected Go or protocol contracts together. |

### Native MCP (`INV-MCP`)

| ID | Rule |
| --- | --- |
| INV-MCP-01 | A native MCP tool MUST implement `mcp.NativeTool` and MUST execute through the Gateway in-process Operator boundary after governed dispatch. |
| INV-MCP-02 | The native registry is not an outbound Operator tool server. See INV-BOUND-04. |
| INV-MCP-03 | `RegisterNativeTools` in `internal/services/mcp/native_tool_registry.go` is the inventory. Documentation MUST NOT copy the tool list. |
| INV-MCP-04 | Registration MUST be explicit in `RegisterNativeTools`. MUST NOT register a native tool from `init`. Registry construction MUST return errors. |

### Contribution (`INV-CONTRIB`)

| ID | Rule |
| --- | --- |
| INV-CONTRIB-01 | A documentation change MUST follow the [Documentation Guide](docs.md). Front matter `last_updated` and `version` (or an older `Last Updated` / `Version` header still on an unaudited file) MUST change only after that audit. `version` MUST equal the `VERSION` file. |
| INV-CONTRIB-02 | A contribution MUST be one coherent change and MUST include tests for a fix or a feature. |
| INV-CONTRIB-03 | Issue, security-reporting, and contribution entry points are the [Contributing Guide](../../.github/CONTRIBUTING.md). Distribution terms are the [Business Source License 1.1](../../LICENSE). |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Go version and module | `go.mod` | `go` directive is `1.26.6` |
| Setup prerequisites | `scripts/lib/dev-setup-common.sh`, `scripts/linux-setup.sh`, `scripts/macos-setup.sh`, `scripts/windows-setup.ps1` | Scripts read `go.mod`, check `git`, `make`, Node.js 22+, and `npm`, offer to install missing prerequisites, build `dashboard/g8e-adapter/evaluation-explorer/dist/index.html` when it is absent, run `make build`, and add the repo root to the user path |
| Platform CLI binary | `Makefile` (`MAIN_PKG := ./cmd/g8e`), `cmd/g8e` | `make build` writes `bin/g8e-<os>-<arch>` and copies a runnable binary to the repo root |
| Cobra root and groups | `internal/cli/cmd/main.go`, `internal/cli/cmd/<group>/` | `./g8e --help` |
| Sentinel errors | `internal/constants/errors.go` | `constants.Err*` declarations, including `ErrFileServiceInit` |
| Runtime path constants and Docker profile names | `internal/constants/paths.go` | `DockerBootstrappedProfile`, `DockerEvaluationProfile`, `DockerCrossEnrollProfile`, `DockerG8ellamaProfile` |
| `.g8e/` file service | `internal/services/fs/file_service.go` | `RuntimeFileService`, `NewRuntimeFileService`, `CreateRuntimeTree` |
| Trust bundle path | `internal/cli/config/config.go` | `.g8e/pki/trust/g8eg-ca-bundle.pem` |
| Mode dependency types | `internal/services/pubsub/mode_deps.go` | `GovernanceCoreDeps`, `GatewayModeDeps`, `OutboundModeDeps`, `NewOutboundModeDeps`, `NewGatewayModeDeps` |
| Outbound construction | `internal/services/g8eo.go` | Calls `pubsub.NewOutboundModeDeps` |
| Gateway builder | `internal/services/gateway/gateway_service.go` | Direct `GatewayModeDeps` composite, then `NewGatewayOperatorPubSubService`, `NewPlatformEnrollmentService`, `mcp.NewGatewayService`, `BindMCPGateway`, `initHTTPHandler` |
| MCP bind errors | `internal/services/pubsub/pubsub_commands.go` | `BindMCPGateway` |
| In-process Operator substrate | `internal/services/gateway/embedded/operator.go` | `embedded.New`, `RegisterPending`; outbound runtime is not this package |
| L1 doctrine loader | `internal/services/governance/l1_doctrine.go` | `NewL1DoctrineFromDir` |
| Doctrine flag and env | `internal/cli/cmd/gw/gateway.go`, `internal/constants/env_vars.go` | `--doctrine-dir` wins over `G8E_DOCTRINE_DIR` |
| Native tool contract and registry | `internal/services/mcp/registry.go`, `internal/services/mcp/native_tool_registry.go`, `protocol/docs/mcp_tool_template.go` | `NativeTool`, `RegisterNativeTools` |
| Compose profiles | `docker-compose.yml` | Header comment plus `profiles:` keys: `cross-enrollment`, `g8ellama` |
| `e2e-full` | `internal/cli/cmd/test/test.go` | `./g8e test e2e-full --help` |

## Procedures

### Build and inspect

1. From the repo root:

```bash
make build
./g8e --help
./g8e test unit
./g8e test integration
make lint
```

2. Read flags from `./g8e <command> --help` (INV-ENV-03). Deployment and enrollment stay in the [Getting Started Guide](../guides/getting_started.md).

### Platform test entry points

1. Go platform:

```bash
./g8e test unit
./g8e test integration
./g8e test e2e
./g8e test e2e-full
./g8e test coverage
./g8e test lint
./g8e test chaos
./g8e test summary
```

2. `./g8e test unit` delegates to `make test-unit`. Other suites keep their own package and timeout flags inside the CLI. Reproduce a CI failure through the same entry point.
3. Makefile entry points that this guide names: `make test`, `make test-unit`, `make test-integration`, `make test-docker`, `make test-coverage`, `make ensemble-test`, `make test-external`, `make dashboard-test`.
4. Apply INV-TEST-12 and INV-TEST-13 before describing `e2e-full` or a Compose profile. Further selection and lifecycle rules are in the [Testing Guide](tests.md).

### Add a runtime-file CLI command

1. Put the command in `internal/cli/cmd/<group>/` (INV-CLI-01).
2. Accept a file-service factory (INV-FS-08).
3. On factory failure, return `fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)`.
4. Add a case in `internal/cli/cmd/<group>/factory_error_<group>_test.go` that fails if a downstream dependency is called.

### Gateway governance construction

`gatewayServiceBuilder.build` in `internal/services/gateway/gateway_service.go` wires governance in this order. Other startup work sits between these milestones; do not reorder the milestones.

1. Open the canonical database and obtain typed stores.
2. After those stores exist, and before `InitializePKI` or `InitializePKIWithNames`, construct the in-process substrate with `embedded.New` and `RegisterPending`.
3. Load L1 doctrine with `NewL1DoctrineFromDir`, then bootstrap posture-required L2 consensus when configured.
4. Construct the gateway-mode `OperatorPubSubService` with governance and platform-enrollment dependencies. The builder assigns `GatewayModeDeps` literally (INV-DEP-03).
5. Construct `PlatformEnrollmentService` with the command service as its envelope processor.
6. Construct `mcp.GatewayService` with the command service as envelope processor and session validator, and with audit and L2 dependencies supplied at construction.
7. Call `OperatorPubSubService.BindMCPGateway` once before either service starts (INV-DEP-04).
8. Build the HTTP handler and servers after every controller dependency exists.

### Add a native MCP tool

1. Start from `protocol/docs/mcp_tool_template.go`.
2. Implement `Name`, `Description`, `InputSchema`, and `Execute` with typed inputs and contextual errors.
3. Register the tool in `RegisterNativeTools` in `internal/services/mcp/native_tool_registry.go`.
4. Add focused unit tests and the governed integration coverage the execution path requires.
5. Do not use `init` (INV-MCP-04). Do not copy the tool list into a doc (INV-MCP-03).

### Change runtime doctrine

1. Edit the owning JSON under `protocol/constants/doctrine/` or the Gateway doctrine directory, together with any Go or protocol contract that names the same identifier (INV-DOCTRINE-03).
2. Run `make validate-doctrines`.
3. Restart the Gateway after a runtime directory change. `./g8e gw start --doctrine-dir <path>` sets the directory; `G8E_DOCTRINE_DIR` applies only when the flag is absent (INV-DOCTRINE-02).

## Anti-patterns

- A compatibility shim that keeps a broken path alive (INV-CODE-01).
- An `ensure*` or `getOrCreate*` helper (INV-CODE-12).
- `errors.New` or a package-level sentinel outside `internal/constants/errors.go` (INV-ERR-01, INV-ERR-02).
- Protobuf `Any` or `map[string]interface{}` for a known contract (INV-TYPE-02).
- `os.ReadFile`, `os.WriteFile`, or a hardcoded `.g8e/` path outside `internal/services/fs` (INV-FS-01).
- `go test` for a platform suite, or `t.Parallel()` in integration or E2E (INV-TEST-01, INV-TEST-02).
- A second `Stop` or `Close` on a `NewGatewayFixture` resource (INV-TEST-06).
- Hand-editing generated protobuf Markdown or Gateway OpenAPI output (INV-GEN-01).
- `init` registration of a native MCP tool, or a pasted tool inventory (INV-MCP-03, INV-MCP-04).
- Calling `bootstrapped` a Compose profile that selects the operator, ensemble, or dashboard. The CLI still passes that profile name; `docker-compose.yml` does not define it (INV-TEST-12, INV-TEST-13).
- A flag dump in place of `./g8e <command> --help` (INV-ENV-03).

## Links out

- [Code Map](codemap.md): package ownership, runtime modes, and the CLI group list. Gateway and outbound mode ownership is [Runtime Modes](codemap.md#runtime-modes).
- [Testing Guide](tests.md): tiers, fixtures, `testutil` path rules, selection, race, coverage, and CI. `tests.md` still says `e2e-full` starts a `bootstrapped` Compose profile; that sentence disagrees with tip `docker-compose.yml` (INV-TEST-13).
- [Documentation Guide](docs.md): audit, catalog, style, generation, and the `docs/devs/` format.
- [Release Process](release_process.md): versioning, native evaluation acceptance, and signed evidence.
- [Governance](../architecture/governance.md) and [AI Agents and the g8e Governance Boundary](../architecture/agents.md): five-layer and posture model, and ingress limits.
- [Protocol Specification](../../protocol/docs/spec.md): wire requirements.
- [Scripts](../architecture/scripts.md): platform setup behavior.
- Component workflows: [Protocol README](../../protocol/README.md), [Dashboard Development](../dashboard/devs.md), [Dashboard Testing](../dashboard/tests.md), [Ensemble Development](../ensemble/devs.md), [Ensemble Testing](../ensemble/tests.md).
