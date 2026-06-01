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
| `./g8e platform start` | Start the Governance Gateway |
| `./g8e platform status` | Get Gateway health and status |
| `./g8e auth login` | Authenticate the local CLI |
| `./g8e test` | Run Go host-native tests |

The `g8e` binary is the single entry point for all platform operations. See [docs/g8e-help.md](../g8e-help.md) for complete command reference.

## Architecture

The platform consists of:

- **g8e Protocol** - Protobuf schemas and canonical JSON wire contract in `protocol/`
- **Governance Gateway (g8eg)** - Central Policy Decision Point (PDP)
- **Governed Operator (g8eo)** - Host-side Policy Execution Point (PEP) and MCP server

All components run as native Go processes. Runtime state lives in `.g8e/`.

For details, see [docs/architecture/](../architecture/).

## Build & Runtime

The platform is built via the Makefile. Run `make help` for available targets.

**Startup sequence** (`./g8e platform start`):
1. Gateway binary check/build
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
- `./g8e platform reset` - Delete database and secrets, preserve CA
- `./g8e platform clean` - Destructive removal of all runtime state

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

**Run tests via CLI:**
- `./g8e test` - Default Gateway test run
- `./g8e test g8eo` - Operator listen mode, pub/sub

Never call `go test` directly for platform tests.

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
3. Restart g8eo to load new doctrines

See [docs/architecture/g8e.md](../architecture/g8e.md) for doctrine schema details.

## Constants

Cross-component constants are stored in JSON at `protocol/constants/` (SSOT). Go consumes these via generated registry files.

**Adding constants:**
1. Add to appropriate JSON file in `protocol/constants/`
2. Run `make constants` to regenerate Go registry
3. Run `go run ./internal/constants/check_registry.go` to verify
4. Commit both JSON and generated Go files

**Commands:**
- `make constants` - Generate Go registry from JSON
- `make generate` - Generate protobuf and constants
- `make clean-constants` - Remove generated constants

See [docs/reference/constants.md](../reference/constants.md) for details.

## Native Tools

Native tools are MCP tools compiled into the Operator binary that execute within the Operator's execution boundary locally, without proxying to downstream MCP servers.

**Adding a new native tool:**
1. Create a new file in `internal/services/mcp/` (e.g., `your_tool_name.go`)
2. Implement the `NativeTool` interface with `Name()`, `Description()`, `InputSchema()`, and `Execute()` methods
3. Add your tool to the tools list in `RegisterNativeTools()` in `native_tool_registry.go`
4. No `init()` function needed - registration is explicit

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
- Use clear commit prefixes (e.g., `g8eo: fix the thing`)

**Contact:** danny@g8e.ai

**License:** Apache 2.0. By contributing, you grant us a license to use your work in g8e.
