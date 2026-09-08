# Developer Guidelines

Last Updated: 2026-09-08
Version: v2.1.7

This guide defines the coding and maintenance rules for the g8e repository. The current working tree is the source of truth for current behavior. Use the [Code Map](codemap.md) for package and runtime ownership, the [Testing Guide](tests.md) for test infrastructure and commands, and the [Documentation Guide](docs.md) for documentation ownership, style, metadata, generation, and validation.

## Platform Boundaries

g8e contains a Governance Gateway, an in-process Operator substrate, and an outbound Governed Operator. Governed operations enter the execution boundary as typed intent or a canonical protobuf `GovernanceEnvelope`; the active posture determines whether L2 consensus and L3 notary evidence gate execution. L1 doctrine, L4 verification, and L5 actuation remain part of every governed execution path.

The governance guarantee applies only to operations that traverse a g8e ingress. An external MCP wrapper performs inline L1 screening but does not add an envelope, L2 through L5 execution, a signed receipt, or Gateway audit. Client-native tools and other side channels remain outside the governance boundary. Read [Governance](../architecture/governance.md) for the canonical five-layer and posture model and [AI Agents and the g8e Governance Boundary](../architecture/agents.md) for integration-path limits.

## Development Environment

The Go module declares Go 1.26.6, and the platform setup scripts accept Go 1.26 or newer. The scripts install missing development prerequisites interactively, run the repository build, and add the repository binary to the user path. See [Scripts](../architecture/scripts.md) for platform-specific behavior.

Run commands from the repository root unless the owning component guide says otherwise:

```bash
make build
./g8e --help
./g8e test unit
./g8e test integration
make lint
```

`make build` compiles the complete `g8e` platform CLI from `cmd/g8e`, writes a platform binary under `bin/`, copies the runnable binary to the repository root, and refreshes `demos/bin/g8e`. Use `./g8e <command> --help` as the live source for command names, arguments, flags, defaults, and destructive effects. Use the [Getting Started Guide](../guides/getting_started.md) for deployment and enrollment rather than treating this coding guide as an operations procedure.

This guide owns repository-wide invariants and Go platform conventions. Component-specific workflows live in the [Protocol README](../../protocol/README.md), [Dashboard Development](../dashboard/development.md), [Dashboard Testing](../dashboard/tests.md), [Ensemble Development](../ensemble/devs.md), and [Ensemble Testing](../ensemble/tests.md).

## Engineering Rules

### Always

- Replace broken paths instead of preserving compatibility shims for technical debt.
- Fix root causes. Do not add defensive guards at callers to conceal invalid construction or state.
- Keep functions focused: reads read, writes write, validation validates, and orchestration composes explicit dependencies.
- Make security checks fail closed and propagate their errors.
- Prefer explicit dependencies and state transitions over package globals, lazy adapters, reflection, or hidden side effects.
- Return errors from production paths. Do not panic for recoverable production failures.
- Wrap errors with operation context and preserve the cause with `%w`, for example `fmt.Errorf("gateway: load doctrine: %w", err)`.
- Use `context.Context` for cancellation and give every goroutine clear ownership, cancellation, and completion coordination through channels or `sync.WaitGroup`.
- Use typed models for known shapes. Do not replace protobuf or domain models with untyped maps or ad hoc JSON objects.
- Format Go with `gofmt` and group imports as standard library, external dependencies, and internal repository packages.
- Pass pointers for mutable or large structs and values for small read-only structs.
- Confirm a dependency is already available before importing it; add new dependencies through the owning package manager rather than editing lock or manifest entries by hand.
- Keep changes focused and leave the affected code cleaner than it was.
- Reproduce bugs with a failing regression test before changing production code, then verify the test passes with the fix.
- Update documentation and generated artifacts in the same change as the behavior they describe.

### Never

- Do not add `ensure*` or `getOrCreate*` helpers that combine reads and writes or hide creation as a lookup side effect.
- Do not use protobuf `Any`, `map[string]interface{}`, or equivalent untyped containers for a known contract.
- Do not declare package-level sentinel errors outside `internal/constants/errors.go`.
- Do not use `errors.New` for a distinct production failure mode outside `internal/constants/errors.go`.
- Do not hardcode `.g8e/` runtime paths or bypass `RuntimeFileService` with direct `os` file operations outside the file-service implementation.
- Do not invoke `go test` directly for platform suites; use `./g8e test ...` or the owning Makefile target.
- Do not use `t.Parallel()` in integration or E2E tests.
- Do not hand-edit generated README, protobuf reference, or OpenAPI output.
- Do not leave current-state documentation stale or broaden a security or evidence claim beyond the path and artifacts that support it.

## Errors

`internal/constants/errors.go` owns sentinel errors that callers compare, wrap, or inspect with `errors.Is` or `errors.As`, as well as distinct reusable platform failure modes. Before adding an error, search that file and existing callers for an equivalent constant. Add a centralized constant only when no suitable owner exists, then replace duplicate hand-written forms in the affected scope.

Use:

- A centralized `constants.Err*` value for a known failure mode.
- `fmt.Errorf("component: action: %w", err)` to add context while preserving a cause.
- `fmt.Errorf` with runtime values for dynamic errors.
- A one-off `fmt.Errorf` value in tests when the test needs an injected cause that has no production meaning.

Do not compare rendered error strings when `errors.Is`, `errors.As`, or a typed status is available.

## Typed Contracts and Serialization

Protobuf schemas under `protocol/proto/g8e/` own governance envelopes, proofs, Operator messages, pub/sub messages, receipts, and compliance messages. Preserve typed protobuf payloads through governance and transport code. Internal envelope payloads use protobuf binary encoding where the execution path expects it; protobuf-owned JSON boundaries use `protojson` so field names, enums, and canonical message semantics remain consistent. Non-protobuf HTTP surfaces use named typed request and response structs rather than raw dictionaries.

A mutation added to a governed ingress must be classified with the canonical action and event registries, represented by a typed payload, wrapped in a `GovernanceEnvelope`, verified by L4, and dispatched by L5. Do not call mutation handlers directly to bypass envelope construction or the verification gauntlet. The [Protocol Specification](../../protocol/docs/spec.md) owns wire requirements, while [Governance](../architecture/governance.md) owns posture and execution behavior.

Registry ownership is surface-specific. Runtime Go constants live under `internal/constants/`; external registries and schemas live under `protocol/constants/`, `protocol/models/`, and `protocol/schemas/`. Check the owning registry and its contract tests before changing either side. When a public registry has both Go and JSON representations, update both through the established owner and run the relevant contract or conformance tests.

## Runtime Paths and File I/O

`RuntimeFileService` in `internal/services/fs/file_service.go` is the canonical boundary for `.g8e/` state. Startup constructs the service and calls `CreateRuntimeTree`. Consumers pass relative paths assembled from constants in `internal/constants/paths.go` to methods such as `ReadFile`, `WriteFile`, `Stat`, `FileExists`, `ReadDir`, `Rename`, `Remove`, and `RemoveAll`.

Follow these rules for runtime I/O:

- Define reusable system, repository, and runtime path strings in `internal/constants/paths.go`. Do not introduce inline `.g8e/` fragments or `filepath.Join` calls with path literals in consumers.
- Pass `fs.RuntimeFileService` explicitly to services and functions that own runtime I/O.
- Use `fileSvc.Resolve` only when an API requires an absolute path.
- Use `fileSvc.Rel` to convert an absolute path inside the runtime root back to a relative service path.
- Use `fileSvc.FileExists` for existence checks and compare missing-file errors with `errors.Is(err, constants.ErrNotFound)`.
- Use `constants.Perm*` values for runtime file and directory permissions.
- Keep direct `os` operations that implement this boundary inside `internal/services/fs`; callers use the service.

Public CLI and startup configuration currently contain directory fields such as Gateway `DataDir`, `PKIDir`, `SecretsDir`, and `VaultDir`. Do not spread those absolute paths through service code or add duplicate directory fields merely to route runtime I/O. Resolve runtime locations at the boundary and pass the file service to the owner.

Command constructors under `internal/cli/cmd/` that access runtime state accept a file-service factory so tests can inject an isolated service. A factory initialization failure wraps `constants.ErrFileServiceInit` and preserves the underlying error. Every new factory injection point requires a matching case in `internal/cli/cmd/factory_error_test.go` that proves downstream dependencies are not called.

Tests use `testutil.TempDir` and `testutil.TestPaths` for isolated roots. `testutil.TempDir` returns an absolute base directory; pass it directly to `fs.NewRuntimeFileService` and path initialization rather than appending `.g8e` yourself. The [Testing Guide](tests.md#runtime-files-and-test-paths) owns the complete fixture and path rules.

## Dependency Construction

The Gateway and outbound Operator have different dependency sets. `pubsub.GovernanceCoreDeps` contains the governance dependencies shared by both modes. `pubsub.GatewayModeDeps` adds gateway-only document, consensus, field-read, platform-enrollment, and posture dependencies; `pubsub.OutboundModeDeps` exposes only the shared fields.

`NewOutboundModeDeps` validates the outbound core before `G8eoService` constructs `OperatorPubSubService`. `NewGatewayModeDeps` provides validation for a gateway bundle when called, but the current production Gateway builder constructs `GatewayModeDeps` directly and passes it to `NewGatewayOperatorPubSubService`. Code in that path must not assume the validating constructor ran; preserve the builder's explicit non-nil wiring and the command-service constructor's checks.

The Gateway builder constructs the current runtime in this order:

1. Open the canonical database and obtain typed stores.
2. Load L1 doctrine and bootstrap posture-required L2 consensus when configured.
3. Construct the gateway-mode `OperatorPubSubService` with governance and platform-enrollment dependencies.
4. Construct `PlatformEnrollmentService` with the command service as its envelope processor.
5. Construct `mcp.GatewayService` with the command service as envelope processor and session validator and with audit and L2 dependencies supplied at construction.
6. Call `OperatorPubSubService.BindMCPGateway` once before either service starts to complete the genuine egress cycle.
7. Build the HTTP handler and servers after all controller dependencies exist.

`BindMCPGateway` rejects nil, duplicate, and post-start binding with centralized typed errors. Do not reintroduce construction-time setters for audit, consensus, or session validation. Posture-dependent L2 remains optional only where the posture does not require it; required posture dependencies fail closed.

See the [Code Map](codemap.md#runtime-modes) for the broader Gateway and outbound Operator ownership model.

## Testing

The repository uses four tiers:

| Tier | Purpose | Runtime dependencies |
| --- | --- | --- |
| Tier 1: Unit | Untagged package and component tests | No running platform or third-party service |
| Tier 2: In-process integration | Integration-tagged Go tests and component integration suites | Local files, processes, SQLite, PKI, pub/sub, and in-process services |
| Tier 3: Live platform E2E | Public-interface tests against deployed components | Running Gateway, Operator, Ensemble, and Dashboard with enrolled local credentials |
| Tier 4: External | Ensemble and eval tests that call provider APIs | Explicit third-party credentials and endpoints |

Core test rules:

- Use table-driven Go tests with `testify/assert` and `testify/require` where appropriate.
- Name test functions, subtests, and files for the behavior they verify. Do not use generic `coverage`, `gap`, `edge`, `misc`, `success`, or `error` names as the scope.
- Keep Tier 1 independent of a running platform. Tier 2 and Tier 3 exercise real local boundaries rather than mocking internal services, database clients, or cross-component communication.
- Let `NewGatewayFixture` register teardown. Do not stop its Gateway, close its databases, or close its downstream server a second time.
- Register temporary credential and long-lived fixture cleanup with `t.Cleanup`, not a helper-local `defer` that runs before the test body.
- Use explicit cancellation contexts and join goroutines before the test returns.
- Use typed constants for statuses, reasons, paths, and permissions instead of duplicating their values in assertions.
- Keep the canonical trust bundle at `.g8e/pki/trust/g8eg-ca-bundle.pem`; tests do not repair failures by mutating developer PKI state.
- Do not use `os.Chdir` to align runtime state. Working-directory changes are limited to behavior that intentionally discovers source-tree or configuration files and require a file-level explanation and cleanup.

Use these primary entry points:

- `./g8e test unit`
- `./g8e test integration`
- `./g8e test e2e`
- `./g8e test e2e-full`
- `./g8e test coverage`
- `./g8e test lint`
- `./g8e test chaos`
- `./g8e test summary`
- `make test`, `make test-unit`, `make test-integration`, `make test-docker`, and `make test-coverage`
- `make ensemble-test`, `make evals-test`, `make test-external`, and `make dashboard-test`

The CLI and Makefile do not select identical package sets and timeout flags for every suite. Reproduce a CI failure through the same owning entry point. Read the [Testing Guide](tests.md) for exact selection, lifecycle, state, race, coverage, and component-specific behavior.

## Generated Artifacts

Generated output is changed through its owner:

| Output | Source | Update and validation |
| --- | --- | --- |
| Root `README.md` | `docs/templates/README.md.tmpl` and the promoted snapshot under `docs/evidence/readme/current/` | `make readme`, `make readme-test`, and `make readme-check` |
| Go, Python, TypeScript, and Markdown protobuf output | Schemas and comments under `protocol/proto/g8e/` | `make proto` (`make generate` is an alias) plus affected conformance tests |
| Gateway OpenAPI | Swagger annotations in the Go owners | `make swagger-generate` plus route and contract tests |
| Website | Generated root `README.md` | `make website-test` and `make website-build` when rendering is affected |
| Doctrine references | JSON under `protocol/constants/doctrine/` and demo doctrine inputs | `make validate-doctrines` |
| COSAiS overlays | Canonical overlay and doctrine references | `make validate-cosais` |

Do not edit the generated root README, protobuf API reference, or OpenAPI files as the source change. The [Documentation Guide](docs.md#generated-and-machine-readable-documentation) defines complete ownership and validation, and the [Release Process](release_process.md) defines attended evidence promotion.

## Doctrine Changes

`governance.NewL1DoctrineFromDir` starts with the built-in MITRE-oriented detectors and loads additional `*.json` files from the configured Gateway doctrine directory. `./g8e gw start --doctrine-dir <path>` sets that directory, and `G8E_DOCTRINE_DIR` supplies it when the flag is absent. The Gateway loads doctrine during construction, so restart the Gateway after changing runtime doctrine files.

Reference doctrine JSON under `protocol/constants/doctrine/` is validated by `make validate-doctrines`; the validator also checks compliance references. Update the owning JSON and affected Go or protocol contracts together when a doctrine identifier or public shape changes.

## Native MCP Tools

Native MCP tools implement `mcp.NativeTool` and execute through the Gateway's in-process Operator boundary after governed dispatch. The outbound `G8eoService` does not construct `mcp.GatewayService`, so the native MCP registry is not an independent outbound Operator tool server. `internal/services/mcp/native_tool_registry.go` is the current inventory and registration owner; do not duplicate its tool list in documentation.

To add a native tool:

1. Use `protocol/docs/mcp_tool_template.go` as the implementation template.
2. Implement `Name`, `Description`, `InputSchema`, and `Execute` with typed inputs and contextual errors.
3. Register the tool explicitly in `RegisterNativeTools` in `internal/services/mcp/native_tool_registry.go`.
4. Add focused unit tests and any governed integration coverage required by the execution path.
5. Do not use `init`; registration is explicit and registry construction returns errors.

## Documentation and Contribution Workflow

Treat documentation as code. Audit every changed document end to end, trace claims to current owners, update related current-state summaries, regenerate source-owned output, review relative links, and update `Last Updated` and `Version` only after the audit. Use the exact value in `VERSION` for maintained `Version:` headers. Follow [Documentation Guidelines](docs.md) for the complete process.

Keep a contribution focused on one coherent change and include tests for fixes and features. The [Contributing Guide](../../.github/CONTRIBUTING.md) owns issue, security-reporting, and contribution entry points. The repository is distributed under the [Business Source License 1.1](../../LICENSE), with the Change Date and Change License defined in that file.
