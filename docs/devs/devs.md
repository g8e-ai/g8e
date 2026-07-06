# Developer Guidelines

AI agents updating documentation must follow **[docs.md](docs.md)** for stylistic rules, terminology, and source-of-truth hierarchy.

## Platform Overview

g8e is a zero-trust execution platform for agentic infrastructure. Mutations are typed, signed, state-bound, and verified through a 5-layer gauntlet before any host state changes.

- **GovernanceEnvelope**: Canonical wire format for all mutations (protojson)
- **5-layer verification**: L1 (Technical Bedrock) → L2 (Consensus) → L3 (Notary) → L4 (Warden) → L5 (Actuator)
- **Data sovereignty**: Raw data stays on the Operator host; platform state is host-native under `.g8e/`
- **BYO clients**: UI-less by design; the CLI (`./g8e`) is the default interface

See [g8e Protocol](../../protocol/docs/spec.md), [Gateway](../architecture/gateway.md), [Operator](../architecture/operator.md).

## Getting Started

Requires `make` and Go 1.26+ installed. If you don't have them, run the setup script for your platform to install them automatically:

- **Linux:** `bash scripts/linux-setup.sh`
- **macOS:** `bash scripts/macos-setup.sh`
- **Windows:** `pwsh scripts/windows-setup.ps1`

```bash
make build          # Build the g8e Operator binary
./g8e --help        # Complete command reference
```

| Command | Purpose |
|---|---|
| `./g8e gw start` | Start the Gateway |
| `./g8e gw status` | Gateway health and status |
| `./g8e auth enroll` | Authenticate the local CLI |
| `./g8e test` | Run test suites |

Startup sequence: binary check/build → root of trust generation (first boot) → service convergence via health checks.

## Paths & State

**Source paths** (git root):
- `protocol/` - Protobuf schemas and JSON constants (SSOT)
- `cmd/operator/` - Binary entrypoint
- `internal/cli/cmd/` - Cobra command tree
- `internal/cli/serve/` - Foreground worker bodies
- `internal/` - Internal Go packages
- `internal/pkg/` - Shared internal packages (e.g., SSH utilities)
- `docs/` - Documentation

**Runtime paths** (`.g8e/`):
- `.g8e/pki/` - CA hierarchy and trust bundles
- `.g8e/secrets/` - Bootstrap secrets
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
- Use `TestPaths` for isolated test environments (base directory from a constant, all sub-paths from constants)
- Reproduce bugs with failing tests before fixing
- Tier 1 (Unit) tests: mocks and stubs, no external dependencies (no files, network, or DB)
- Tier 2 (Integration) and Tier 3 (E2E) tests: real database, pub/sub, and LLM calls
- Keep test infrastructure separated from production code
- Run tests via `./g8e test` (unit, integration, e2e, coverage, lint, agent, chaos, summary)
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
- Coverage threshold: 70%

**Test infrastructure separation:**
- `internal/services/storage/storagetest/` - Test-only audit storage (`TestSQLAuditStore` with Git ledger, no-op `DocSet`)
- `internal/tools/chaos/` - Chaos engineering infrastructure (uses `storagetest.TestSQLAuditStore`)
- Production gateway mode wires `DocumentStoreService` as `TransactionAuditStore`
- Production outbound mode uses `auditStoreTransactionStore` adapter in `g8eo.go`

## Thread Safety for Late-Bound Dependencies

Several services have dependencies that cannot be passed to the constructor because they are created later in the boot sequence (circular or late-resolved dependency graphs). The canonical pattern for these is:

- **`atomic.Pointer[T]`** for pointer-typed late-bound deps (e.g., `HTTPHandler.tribunal`, `HTTPHandler.envProc`, `mcp.GatewayService.runtimeDeps`).
- **`atomic.Value`** for interface-typed late-bound deps (e.g., `mcp.GatewayService.tribunalDeliberator`).

**Pattern rules:**

1. The field is declared as `atomic.Pointer[T]` or `atomic.Value`, never as a raw pointer.
2. A `SetXxx` method stores via `.Store()` — it must **not** rebuild routers or mutate other state.
3. The handler/method reads via `.Load()` and checks for nil. If nil, return a fail-closed error (503 or equivalent).
4. Routes that depend on late-bound deps are **always registered** in the router. The handler checks the atomic pointer at request time, eliminating the need for a router rebuild when the dependency is wired.
5. `go test -race` must pass — the atomic access guarantees no data races even if the boot sequence changes.

**Two-phase dependency model for `mcp.GatewayService`:**

- **Construction-phase** (`Dependencies` struct, immutable after `NewGatewayService`): `Logger`, `Responder`, `SuspendedStore`, `ScrubbingService`, `MaxPayloadBytes`, `Posture`, `A2ADownstreamURL`, `PublicBaseURL`.
- **Runtime-phase** (`RuntimeDependencies` struct, set once via `SetRuntimeDeps` before first request): `EnvProc`, `StateRootProvider`, `SigningKey`, `KeyID`, `DownstreamURL`, `DBService`, `SessionValidator`, `AuditLogger`. Stored via `atomic.Pointer[RuntimeDependencies]` with `runtimeReady()` gate.

## Constants & Doctrines

Go constants in `internal/constants/` are SSOT. JSON files in `protocol/constants/` are reference documentation and external protocol definitions.

**Doctrines:** Stored in `protocol/constants/doctrine/` as canonical JSON, loaded by L1Doctrine at startup.

Adding doctrines: update JSON → `make validate-doctrines` → restart Operator.

See [Constants Reference](../../protocol/docs/constants.md) and [Protocol Spec](../../protocol/docs/spec.md).

**Code generation:** `make generate` (protobuf from `.proto` files) · `make proto` (alias)

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
| Constants (JSON SSOT) | `protocol/constants/` |
| Go registry files | `internal/constants/` |
| Governance layers | `internal/services/governance/` |
| CLI entry points | `cmd/operator/` → `internal/cli/cmd/` |
| Architecture docs | `docs/architecture/` |

## Contributing

- One change per PR
- Add tests for bug fixes and features
- Commit prefixes: `g8e: fix the thing`

**Contact:** danny@g8e.ai · **License:** Apache 2.0
