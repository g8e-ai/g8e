# Developer Guidelines

AI agents updating documentation must follow **[docs.md](docs.md)** for stylistic rules, terminology, and source-of-truth hierarchy.

## Platform Overview

g8e is a zero-trust execution platform for agentic infrastructure. Mutations are typed, signed, state-bound, and verified through a 5-layer gauntlet before any host state changes.

- **GovernanceEnvelope**: Canonical wire format for all mutations (protojson)
- **5-layer verification**: L1 (Technical Bedrock) → L2 (Consensus) → L3 (Notary) → L4 (Warden) → L5 (Actuator)
- **Data sovereignty**: Raw data stays on the Operator host; platform state is host-native under `.g8e/`
- **BYO clients**: The CLI (`./g8e`) is the default interface; MCP stdio for AI IDE integration; Console SPA and TUI for governance management

See [g8e Protocol](../../protocol/docs/spec.md), [Gateway](../architecture/gateway.md), [Operator](../architecture/operator.md), [Codemap](codemap.md).

## Getting Started

Requires `make` and Go 1.26+ installed. If you don't have them, run the setup script for your platform to install them automatically (see [scripts.md](../architecture/scripts.md) for details):

- **Linux:** `bash scripts/linux-setup.sh`
- **macOS:** `bash scripts/macos-setup.sh`
- **Windows:** `pwsh scripts/windows-setup.ps1`

```bash
make build          # Build the g8e Operator binary
./g8e --help        # Complete command reference
```

| Command | Purpose |
|---|---|
| `./g8e gw start` | Start the Gateway (`--doctrine-dir` loads JSON doctrine files for L1 threat detection) |
| `./g8e gw status` | Gateway health and status |
| `./g8e auth enroll user` | Enroll the first owner / local CLI session with the running Gateway and register a passkey |
| `./g8e auth enroll operator` | Enroll a remote operator/device with the Gateway via CSR |
| `./g8e auth enroll gui` | Enroll an external frontend application origin with the Gateway |
| `./g8e auth pending-platform-enrollments` | List pending platform workload enrollment requests (operator, dashboard, ensemble) via authenticated mTLS |
| `./g8e auth approve-platform-enrollment <request-id>` | Approve or deny (`--deny`) a pending platform workload enrollment request by exact request ID via authenticated mTLS |
| `./g8e compliance` | FedRAMP 20x KSI evaluation and OSCAL export |
| `./g8e test` | Run test suites |

Startup sequence: binary check/build → root of trust generation (first boot) → service convergence via health checks. Platform workloads (operator, dashboard, ensemble) start not-ready and require owner-approved platform enrollment: the gateway starts with zero users, the first owner enrolls via `auth enroll user`, each workload submits a platform enrollment request at startup, and the owner approves each request by exact request ID via `auth approve-platform-enrollment` before the workload becomes ready. See [auth.md](../architecture/auth.md) and the [Docker Gateway Guide](../guides/docker_gateway.md) for the full activation flow.

## Paths & State

**Source paths** (git root):
- `protocol/` - Protobuf schemas, JSON protocol constants, model definitions, Python SDK, and conformance tests
- `cmd/g8e/` - Binary entrypoint
- `internal/cli/cmd/` - Cobra command tree
- `internal/cli/serve/` - Gateway and operator boot sequences
- `internal/cli/sse/` - Reusable SSE client (frame parsing, reconnection, mTLS headers)
- `internal/cli/tui/` - Tactical Governance Console (Bubble Tea TUI)
- `internal/` - Internal Go packages
- `internal/pkg/` - Shared internal packages (e.g., SSH utilities)
- `test/` - E2E and integration tests (gateway, MCP, consensus, native tool registry)
- `docs/` - Documentation

**Runtime paths** (`.g8e/`):
- `.g8e/pki/` - CA hierarchy and trust bundles
- `.g8e/secrets/` - Bootstrap secrets
- `.g8e/vault/` - Encryption vault (private)
- `.g8e/data/` - SQLite databases and blobs
- `.g8e/logs/` - Component logs
- `.g8e/pids/` - Process IDs

**Cleanup:** `./g8e gw reset` (delete DB + secrets, preserve CA) · `./g8e gw clean` (destructive removal of all runtime state)

## Always

- Rip and replace broken code; no compatibility shims
- Functions do one thing: reads read, writes write
- Fix root causes; no defensive guards at call sites
- Fail-closed on security checks
- Explicit over implicit; no magic or hidden side effects
- Leave codebase cleaner than you found it
- Check all errors; wrap with context: `fmt.Errorf("component: action: %w", err)`
- Define typed error constants in `internal/constants/errors.go` for any error that is checked, compared, wrapped with `errors.Is()`/`errors.As()`, or represents a distinct failure mode
- Use `fmt.Errorf()` for wrapping with context, dynamic messages with runtime values, and one-off test errors
- Search for hand-rolled strings when adding new error constants and replace them
- Return errors from production paths; no panics
- Use `context.Context` for cancellation; manage goroutines with `sync.WaitGroup` or channels
- Write table-driven tests with `testify/assert`
- Use three import blocks: standard library, external, internal
- Pass pointers for mutable/large structs; values for small/read-only structs
- Use typed model instances; no raw dicts, untyped maps, or ad-hoc JSON
- Use canonical JSON (protojson) for all client-facing surfaces
- Route all mutations through `GovernanceEnvelope` and the 5-layer verification gauntlet
- Define ALL filepath strings as constants in `internal/constants/paths.go`
- Use `RuntimeFileService` (`internal/services/fs`) as the canonical abstraction for all `.g8e/` file I/O. Call `CreateRuntimeTree` at startup, then use `fileSvc.ReadFile`/`fileSvc.WriteFile`/`fileSvc.Stat`/`fileSvc.Remove` with relative paths constructed from `constants.*` constants
- Pass `fileSvc` as an explicit parameter to services and functions that perform `.g8e/` file I/O. Do not use `os.ReadFile`/`os.WriteFile` for `.g8e/` paths
- Use `fileSvc.Resolve(constants.*)` to obtain absolute paths when needed (e.g., for `filepath.Join` in non-fileSvc APIs). Use `fileSvc.Rel()` to convert absolute `.g8e/` paths back to relative paths for `fileSvc` calls
- Use `constants.Perm*` constants for file and directory permissions. Use `constants.Err*` constants for error checking (e.g., `errors.Is(err, constants.ErrNotFound)` replaces `os.IsNotExist`)
- Wrap `fileSvcFactory()` errors with `constants.ErrFileServiceInit` in all `*WithConfig` command functions. Do not use `constants.ErrInternal`, `constants.ErrPathValidation`, or ad-hoc string wrapping for file service initialization errors
- Inject `fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error)` as a parameter in `*WithConfig` command functions. Production constructors pass `newFileSvc`; tests pass `fileSvcFactoryFor(fileSvc)` with a temp-rooted `fileSvc`. Every injection point must have a factory-error test asserting `ErrFileServiceInit` wrapping
- Do not add `DataDir`/`CredentialsDir`/`PKIDir` fields to config structs. Use `fileSvc.Resolve(constants.*)` instead. `paths.Infra` is config-only (path registration), not for file I/O
- Use `TestPaths` for isolated test environments (base directory from a constant, all sub-paths from constants)
- Reproduce bugs with failing tests before fixing
- Tier 1 (Unit) tests: mocks and stubs, no external dependencies (no files, network, or DB)
- Tier 2 (Integration) tests: real database, pub/sub, and local PKI. Tier 3 (E2E) tests: real Docker containers. Tier 4 (External) tests: real LLM providers and third-party APIs
- Keep test infrastructure separated from production code
- Run tests via `./g8e test` (unit, integration, e2e, coverage, lint, chaos, summary)
- Document what the system does, not what it should do
- Cross-link rather than repeat
- Present tense, active voice, direct and specific
- Keep PRs focused (one change per PR)
- Add tests for bug fixes and features

## Never

- No `ensure*()`, `getOrCreate*()`, `Any` types, or `map[string]interface{}` for known shapes
- No hand-rolled error strings (`errors.New("...")`) when a centralized constant exists or should exist
- No package-level error variables outside `internal/constants/errors.go`
- No panics in production paths
- No hardcoded or dynamically constructed filepath strings (including `"../../"`, `"./"`, `".g8e/"`, `"/pki/"`)
- No `filepath.Join()` with string literals (except within `TestPaths` using constants)
- No `go test` directly for platform tests; use `./g8e test`
- No emojis in documentation
- No stale docs; docs are code

## Patterns

### Error Handling

Return centralized error constants from `internal/constants/errors.go` for known failure modes. Wrap errors with context using `fmt.Errorf` and the `%w` verb for dynamic messages or chaining. Never hand-roll error strings with `errors.New` when a centralized constant exists or should exist. Never declare package-level error variables outside `internal/constants/errors.go`.

### Adding Error Constants

1. Check `internal/constants/errors.go` for existing matches
2. Add a new constant if none exists
3. Use it consistently across the codebase
4. Search for hand-rolled strings to replace

### Adding Path Constants

1. Add to `internal/constants/paths.go`
2. Update `protocol/constants/` JSON if part of the public protocol
3. Run tests to verify integration
4. Commit both Go source and JSON reference files

## Testing

- mTLS by default; test runner handles certificate injection
- Contract tests enforce alignment between components and `protocol/`
- Coverage threshold: 75%
- See [Testing Guide](tests.md) for detailed test patterns and infrastructure

**Test infrastructure separation:**
- `internal/services/storage/storagetest/` - Test-only audit storage (`TestSQLAuditStore` with Git ledger, no-op `DocSet`) and `TestTokenStore` (in-memory `TokenStore` with TTL). `TestSQLAuditStore` satisfies `compliance.AuditEvidenceReader` via `ListEvents` and `ListFileMutations` methods.
- `internal/services/pubsub/pubsubtest/` - Test-only `PubSubClient` mock (`MockOperatorPubSubClient`)
- `internal/services/governance/governancetest/` - Test-only governance store fixtures (`SimpleConsensusStore`, `SimpleAppPolicyStore`, `SimpleStateRootProvider`)
- `internal/tools/chaos/` - Chaos engineering infrastructure (uses `storagetest.TestSQLAuditStore`)
- `internal/tools/agent_harness/` - Agent test harness with scenario runner (`client/`, `config/`, `scenarios/`) for MCP/A2A gateway integration tests
- `test/` - Root-level E2E and integration tests (gateway, MCP, consensus, native tool registry, A2A)
- Production gateway mode wires `DocumentStoreService` as `TransactionAuditStore`
- Production outbound mode wires `storage.SQLAuditStore` directly as `TransactionAuditStore` via its native `DocSet` method (no adapter)

## Dependency Construction Model

`mcp.GatewayService` uses a **single-phase construction model**: all dependencies are passed to `NewGatewayService` via the `Dependencies` struct and are immutable after construction. There is no `SetRuntimeDeps`, no `atomic.Pointer`, and no `runtimeReady()` gate.

**Construction-phase** (`Dependencies` struct, immutable after `NewGatewayService`): `Logger`, `Responder`, `SuspendedStore`, `ScrubbingService`, `ThreatScanner`, `MaxPayloadBytes`, `Posture`, `A2ADownstreamURL`, `PublicBaseURL`, `AuditStore`, `FieldPathRegistryFactory`, `EnvProc`, `StateRootProvider`, `SigningKey`, `KeyID`, `DownstreamURL`, `DBService`, `SessionValidator`, `AuditLogger`, `L2ConsensusDeliberator`.

**Lazy forwarding adapters** break the circular dependency between `mcp.GatewayService` (needs `EnvProc` / `SessionValidator`) and `OperatorPubSubService` (needs `mcpGateway` for egress). Each adapter's target field is an `atomic.Pointer[OperatorPubSubService]`; `SetTarget` calls `Store` (during boot) and the request-path method calls `Load`. The `atomic.Pointer` provides the memory ordering guarantees a raw pointer lacks — the target is read on the request path and written during boot, so unsynchronized aliasing is a data race under `-race`. A `Load` returning nil means the real target is not yet wired; the adapter returns `constants.ErrGatewayNotReady` (fail-closed) in that case.

- `pubsub.GatewayEnvProcAdapter` — implements `governance.EnvelopeProcessor`, delegates to `OperatorPubSubService` once it is constructed. The adapter is pre-allocated in `GatewayModeService.build()` and passed to `NewGatewayService` as `EnvProc`. `NewGatewayOperatorPubSubService` calls `adapter.SetTarget(rs)` to wire the real target. Before `SetTarget` is called, `ProcessEnvelope` returns `constants.ErrGatewayNotReady` (fail-closed).
- `pubsub.GatewaySessionValidatorAdapter` — same pattern for `mcp.SessionValidator`.

**Narrow setters for genuinely late-bound deps** (no circular dependency, no `atomic.Pointer`, no `runtimeReady()` guard — the fields are nil-checked at their call sites):

- `SetAuditLogger` — `AuditStore` comes from `CommandServiceConfig.AuditStore`, configured in `serve/gateway.go` after `GatewayModeService` exists.
- `SetL2ConsensusDeliberator` — the deliberator requires `GatewayModeService` to exist for `ConsensusBootstrap`.

These two setters are the only post-construction mutators on `mcp.GatewayService`. All other dependencies are construction-phase only.

**`GatewayModeService.SetConsensusService`** is a narrow setter on `GatewayModeService` (not `mcp.GatewayService`) for a genuinely late-bound dependency: `ConsensusService` is built between `NewGatewayModeService` and `Start` because it reads from the DB the constructor opens (via `ConsensusBootstrap`). The `consensusSvc` field is an `atomic.Pointer[consensus.ConsensusService]`; `SetConsensusService` calls `Store`, and the `GovernanceController` (which captures `&ls.consensusSvc` at construction time) calls `Load` on the request path. The `atomic.Pointer` provides the memory ordering guarantees that a raw `**T` lacks — the cell is read on the request path and written during boot, so unsynchronized aliasing is a data race under `-race`. A `Load` returning nil means consensus is not configured for the current posture (e.g. doctrine mode); `handleConsensusDeliberate` returns 503 in that case.

## Constants & Doctrines

Go constants in `internal/constants/` are SSOT. JSON files in `protocol/constants/` are reference documentation and external protocol definitions.

**Doctrines:** Stored in `protocol/constants/doctrine/` as canonical JSON, validated by `make validate-doctrines`. L1Doctrine uses protobuf field options and hardcoded threat detectors at runtime; the JSON files are reference schemas. At gateway startup, `--doctrine-dir` (env: `G8E_DOCTRINE_DIR`) loads additional `*.json` doctrine files via `NewL1DoctrineFromDir()`, appending file-loaded detectors after hardcoded MITRE patterns. The loaded doctrine instance is shared between the MCP Gateway ThreatScanner and L4Warden.

Adding doctrines: update JSON → `make validate-doctrines` → restart Operator. For runtime doctrine loading, place JSON files in the doctrine directory and pass `--doctrine-dir`.

See [Constants Reference](../../protocol/docs/constants.md) and [Protocol Spec](../../protocol/docs/spec.md).

**Code generation:** `make proto` (protobuf from `.proto` files) · `make generate` (alias)

## Native Tools

MCP tools compiled into the g8e binary that execute within the Operator's execution boundary locally.

**Adding a new native tool:**
1. Copy `protocol/docs/mcp_tool_template.go` to `internal/services/mcp/your_tool_name.go`
2. Implement `NativeTool` interface: `Name()`, `Description()`, `InputSchema()`, `Execute()`
3. Register in `RegisterNativeTools()` at `internal/services/mcp/native_tool_registry.go`
4. Add unit tests in `internal/services/mcp/your_tool_name_test.go`
5. No `init()` function; registration is explicit

**Existing tools:** Database (discover, validate, read, index triage), log filtering, OOM detection, config diff masking, process metrics (top, tree), disk profiling (usage, profile, file checksum), signal safety, network socket audit, endpoint ping, HTTP probe, DNS resolution, TLS cert inspection, SSH known hosts, service status, container status, system info, environment variables, time clock, Git operations, cloud metadata, Kubernetes inspection, shell command execution, file read, operator deploy.

## Quick Reference

| Concern | Location |
|---|---|
| Protobuf schemas | `protocol/proto/` |
| Constants (JSON reference) | `protocol/constants/` |
| JSON model definitions | `protocol/models/` |
| Go registry files | `internal/constants/` |
| Governance layers | `internal/services/governance/` |
| Gateway service | `internal/services/gateway/` |
| MCP gateway & native tools | `internal/services/mcp/` |
| Compliance (KSI/OSCAL) | `internal/services/compliance/` |
| CLI entry points | `cmd/g8e/` → `internal/cli/cmd/` |
| Architecture docs | `docs/architecture/` |

## Contributing

- One change per PR
- Add tests for bug fixes and features
- Commit prefixes: `g8e: fix the thing`

**Contact:** danny@g8e.ai · **License:** BSL 1.1 (converts to Apache 2.0 on 2030-08-18)
