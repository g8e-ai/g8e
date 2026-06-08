# Developer Guidelines

This document provides practical guidance for human developers contributing to the g8e platform.

## For AI Agents

If you are an AI coding assistant, the authoritative contract is in **[AGENTS.md](AGENTS.md)**. That document contains the strict "Always" and "Never" directives you must follow. This document is for human developers.

## Platform Overview

g8e is a zero-trust execution platform for agentic infrastructure. The platform enforces typed, signed, state-bound mutations through a 5-layer verification gauntlet before any host state changes occur.

**Key concepts:**
- **GovernanceEnvelope**: The canonical wire format for all mutations (canonical JSON via protojson)
- **5-layer verification**: L1 (Technical Bedrock) → L2 (Consensus) → L3 (Notary) → L4 (Warden) → L5 (Actuator)
- **Data sovereignty**: Raw data stays on the Operator host; platform state is host-native under `.g8e/`
- **BYO clients**: The platform is UI-less by design. The CLI (`./g8e`) is the default interface.

For detailed architecture, see:
- [docs/architecture/g8e.md](../architecture/g8e.md) - Platform architecture and governance model
- [docs/architecture/protocol.md](../architecture/protocol.md) - Protocol and wire format
- [docs/architecture/operator.md](../architecture/operator.md) - Operator service details

## Getting Started

### Setup

Run the interactive setup wizard:

```bash
./g8e setup
```

This checks for Go 1.26+ dependencies, generates protocol artifacts, and builds the `g8e` binary.

**Manual GOPATH configuration** (if needed):
```bash
export GOPATH=$HOME/go
export PATH=$GOPATH/bin:$PATH
```

### Common Commands

| Command | Purpose |
|---|---|
| `./g8e` | Interactive Platform Manager |
| `./g8e gw start` | Start the g8e Gateway |
| `./g8e gw status` | Get g8e Gateway health and status |
| `./g8e auth login` | Authenticate the local CLI |
| `./g8e test` | Run Go host-native tests |

The g8e Node is the single entry point for all platform operations. See [docs/g8e-help.md](../g8e-help.md) for complete command reference.

## Architecture

The platform consists of:

- **g8e Protocol** - Protobuf schemas and canonical JSON wire contract in `protocol/`
- **g8e Gateway** - Central Policy Decision Point (PDP)
- **g8e Operator** - Host-side Policy Execution Point (PEP) and MCP server

All components run as native Go processes. Runtime state lives in `.g8e/`.

For details, see [docs/architecture/](../architecture/).

## Build & Runtime

The platform is built via the Makefile. Run `make help` for available targets.

**Startup sequence** (`./g8e gw start`):
1. g8e Node check/build
2. Root of trust generation (first boot only) - CA hierarchy in `.g8e/pki/`, secrets in `.g8e/secrets/`
3. Service convergence via health checks

## Paths & State

**Source paths** (git root):
- `protocol/` - Protobuf schemas and JSON constants (SSOT)
- `cmd/` - CLI entry points
- `internal/` - Internal Go packages
- `pkg/` - Public Go packages
- `docs/` - Documentation

**Runtime paths** (`.g8e/`):
- `.g8e/pki/` - CA hierarchy and trust bundles
- `.g8e/secrets/` - Bootstrap secrets
- `.g8e/data/` - SQLite databases and blobs
- `.g8e/logs/` - Component logs
- `.g8e/pids/` - Process IDs

**Cleanup commands:**
- `./g8e gw reset` - Delete database and secrets, preserve CA
- `./g8e gw clean` - Destructive removal of all runtime state

## Code Quality Principles

**No tech debt:**
- Rip and replace broken code; do not add compatibility shims
- No `ensure*()`, `getOrCreate*()`, `Any` types, or `map[string]interface{}` for known shapes
- Functions do one thing: reads read, writes write
- Fix root causes; do not add defensive guards at call sites

**Industry standards:**
- Use latest stable versions (Go 1.26+, Python 3.14+)
- Fail-closed on security checks
- Explicit over implicit; no magic or hidden side effects
- Leave codebase cleaner than you found it

## Go Standards

**Tooling:** `gofmt`, `goimports`, `golangci-lint` (mandatory in CI)

**Error handling:** Always check errors; wrap with context using `fmt.Errorf("component: action: %w", err)`

**Typed errors:** Define typed error constants for error reasons instead of hand-trolled strings. When adding error types, check for any hand-trolled strings that should be properly typed errors (e.g., error reason strings, status codes, rejection reasons). Define these as typed constants in `internal/constants/` and use them consistently across the codebase.

**No panics** in production paths; return errors instead

**Concurrency:** Use `context.Context` for cancellation; manage goroutines with `sync.WaitGroup` or channels

**Testing:** Table-driven tests with `testify/assert`

**Imports:** Three blocks - standard library, external, internal

**Parameters:** Pointers for mutable/large structs; values for small/read-only structs

## Data & Protocol

**Single source of truth:** The `protocol/` directory is canonical for wire-protocol values and document schemas

**Strict typing:** Use typed model instances; no raw dicts, untyped maps, or ad-hoc JSON

**Wire format:** Canonical JSON (protojson) for all client-facing surfaces

**Governance:** All mutations must pass through the `GovernanceEnvelope` and 5-layer verification gauntlet. See [AGENTS.md](AGENTS.md) for detailed layer responsibilities.

## Testing

**Principles:**
- Reproduce bugs with failing tests before fixing
- No mocks; use real database, pub/sub, and LLM calls
- Contract tests enforce alignment between components and `protocol/`
- mTLS by default; test runner handles certificate injection
- Test infrastructure separated from production code to avoid import cycles

**Run tests via CLI:**
- `./g8e test` - Full test suite (unit + integration)

Never call `go test` directly for platform tests.

### Test Infrastructure Separation

Test-only code is separated from production code to avoid import cycles and maintain clear boundaries:

**`internal/services/storage/storagetest/`** - Test-only audit storage implementations
- `TestSQLAuditStore` - Test-only monolithic audit service with Git ledger integration
- Used only in test code (e.g., chaos tester at `internal/test/chaos/chaos.go`)
- Implements `TransactionAuditStore` interface via a no-op `DocSet` method
- Production code uses `storage.SQLAuditStore` from `audit_store.go`

**`internal/test/chaos/`** - Chaos engineering test infrastructure
- Chaos tester uses `storagetest.TestSQLAuditStore` for audit storage
- This is intentional test infrastructure, not production code
- Located in `internal/test/` to clearly indicate test-only status

## Documentation

**Principles:**
- Docs are code; stale docs are bugs
- Document what the system does, not what it should do
- Cross-link rather than repeat
- Present tense, active voice, direct and specific
- No emojis

**Single source of truth:** `protocol/` for wire-protocol values

## Doctrines

Security doctrines are stored in `protocol/constants/doctrine/` as canonical JSON and loaded by L1Doctrine at startup.

**Adding doctrines:**
1. Update JSON file in `protocol/constants/doctrine/`
2. Run `make validate-doctrines`
3. Restart g8e Operator to load new doctrines

See [docs/architecture/g8e.md](../architecture/g8e.md) for doctrine schema details.

## Constants

Constants are defined in Go source files in `internal/constants/` (SSOT). JSON files in `protocol/constants/` serve as reference documentation and external protocol definitions.

**Adding constants:**
1. Add the constant to the appropriate Go file in `internal/constants/`
2. Update the corresponding JSON file in `protocol/constants/` if the constant is part of the public protocol
3. Run tests to verify the constant is properly integrated
4. Commit both the Go source file and any updated JSON reference files

**Commands:**
- `make generate` - Generate protobuf code from `.proto` files
- `make proto` - Generate Go Protobuf code (alias for generate)

See [docs/reference/constants.md](../reference/constants.md) for details.

## Native Tools

Native tools are MCP tools compiled into the Node binary that execute within the Operator's execution boundary locally, without proxying to downstream MCP servers.

**Adding a new native tool:**
1. Copy `docs/protocols/mcp/tool_template.go` to `internal/services/mcp/your_tool_name.go`
2. Implement the `NativeTool` interface with `Name()`, `Description()`, `InputSchema()`, and `Execute()` methods
3. Add your tool to the tools list in `RegisterNativeTools()` in `internal/services/mcp/native_tool_registry.go`
4. Add unit tests in `internal/services/mcp/native_handlers_test.go`
5. No `init()` function needed - registration is explicit via `RegisterNativeTools()`

**Template:** See `docs/protocols/mcp/tool_template.go` for a complete example.

**Existing tools:** Database tools (discover, validate, read, index triage), log filtering, OOM detection, config diff masking, process metrics, disk profiling, signal safety, network socket audit, endpoint ping, HTTP probe.

## Quick Reference

| Concern | Location |
|---|---|
| Protobuf schemas | `protocol/proto/` |
| Constants (JSON SSOT) | `protocol/constants/` |
| Go registry files | `internal/constants/` |
| Governance layers | `internal/services/governance/` |
| CLI entry points | `cmd/` |
| Architecture docs | `docs/architecture/` |

## Contributing

**PR guidelines:**
- Keep it focused (one change per PR)
- Add tests for bug fixes and features
- Use clear commit prefixes (e.g., `g8e: fix the thing`)

**Contact:** danny@g8e.ai

**License:** Apache 2.0. By contributing, you grant us a license to use your work in g8e.
