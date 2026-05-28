# Developer Guidelines

This document defines the technical standards, architectural invariants, and contribution workflow for the g8e platform.

## Core Architectural Invariants

1. **Human agency is absolute.** Every state-changing operation surfaces its own approval prompt. Automatic Function Calling is permanently disabled.
2. **5-layer verification sequence.** L1 Technical Bedrock (L1Doctrine) hard gates via protobuf reflection; L2 Consensus (L2Consensus) multi-agent consensus; L3 Notary (L3Notary) human-in-the-loop WebAuthn proofs; L4 Warden (L4Warden) pre-dispatch verification; L5 Actuator (L5Actuator) sovereign execution boundary and signed receipts.
3. **Data sovereignty.** Raw command output and file contents stay on the Operator host, encrypted, and never persist platform-side. Platform state is host-native under `.g8e/`.
4. **Security by structure.** All changes adhere to the Security Review Checklist. The Operator is the only execution boundary.
5. **BYO Frontend.** The platform is UI-less by design. The **CLI (`./g8e`) is the default out-of-the-box UI**. The Operator provides a minimal bootstrap web interface, but primary interaction is via the CLI or BYO clients.

## Development Lifecycle

Components run host-native. **Do not use Docker for primary component development or testing.**

### Setup

Run the interactive setup wizard:

```bash
./g8e setup
```

The setup checks for required Go development dependencies, generates protocol artifacts, and builds the `g8e` binary.

**Manual setup requirements:**

- **Go 1.26+** (required). Configure GOPATH in your shell profile (`~/.bashrc` or `~/.zshrc`):
  ```bash
  export GOPATH=$HOME/go
  export PATH=$GOPATH/bin:$PATH
  ```
  Go tools installed via `go install` (e.g., golangci-lint, govulncheck) are placed in `$GOPATH/bin`.

### Common Commands

| Command | Purpose |
|---|---|
| `./g8e` | Interactive Platform Manager. |
| `./g8e platform start` | Start the Governance Gateway (`g8eg`). |
| `./g8e platform status` | Get Gateway health and status. |
| `./g8e auth login` | Authenticate the local CLI. |
| `./g8e test` | Run Go host-native tests. |

### CLI

The unified `g8e` binary is the single entry point for all platform operations. It serves dual purposes:
- **Daemon mode**: Runs the Governance Gateway/Operator when invoked without subcommands
- **CLI mode**: Manages platform lifecycle, auth, data, and tests when invoked with subcommands

**CLI Subcommands:**
- **Platform Management (`./g8e platform`)**: Orchestrates the Gateway lifecycle via native Go process management.
- **Auth & PKI (`./g8e auth`)**: Establishes identity, generates CSRs, and verifies certificate chains.
- **Data & Admin (`./g8e data`)**: Administers the local substrate over mTLS (users, operators, device-links, settings).
- **Test Orchestration (`./g8e test`)**: Orchestrates Go test execution suites.
- **Security (`./g8e security`)**: Validation checks.
- **Environment (`./g8e vars`)**: Environment variable management.

**Technical Invariants:**
1. **Zero Shell Scripts**: NO shell scripts are used for platform operations. All platform lifecycle, configuration, and administrative duties are handled by the unified Go binary. Constants are generated from JSON SSOT (`protocol/constants/*.json`) via `internal/constants/generate_registry.go`. Tests and code consume protocol constants directly from JSON files (`protocol/constants/*.json`), not by sourcing shell scripts.
2. **Service Readiness**: The platform is not "ready" until the Governance Gateway (`g8eg`) Gateway mode health check (`/healthz`) passes.
3. **Canonical Wire Format**: All client-facing interaction (HTTP, PubSub, receipts) must use **canonical JSON (protojson)**. Binary Protobuf is reserved for internal storage.
4. **Fail-Closed Execution**: The CLI must never mask failures or proceed with missing trust material.

**Permissible Shell Scripts:**
- Vendor scripts (third-party Go vendor scripts in `vendor/` and `tools/vendor/`) - not g8e platform code

## Architecture Philosophy

g8e is split into the **g8e Protocol**, the **Governance Gateway (g8eg)**, and the **Governed Operator (g8eo)**.

- **g8e Protocol** - Shared `.proto` schemas plus the canonical-JSON wire contract; the source of truth for what every operator and client must honor.
- **Governance Gateway (`g8eg`)** - The central, BFT-governed Policy Decision Point (PDP) running in Gateway mode (--doctrine, --consensus, or --notary). It provides the platform's central persistence, PKI, and protocol API (including a minimal bootstrap interface).
- **Governed Operator (`g8eo`)** - The host-side Policy Execution Point (PEP) and MCP Server. It enforces protocol compliance, verifies Doctrine (L1Doctrine), Consensus (L2Consensus), and Notary (L3Notary) signatures, and executes transactions via the L5Actuator stage.
- **Host-native execution** - Core components run as native processes.
- **Zero-config discovery** - Services use a standardized local runtime directory (`.g8e/`) for discovery and configuration sharing.

## Build Pipeline & Dependencies

| Component | Role | Runtime | Build |
|---|---|---|---|
| Governance Gateway (`g8eg`) / Governed Operator (`g8eo`) | Central PDP (`g8eg`) and host-side PEP (`g8eo`) | Host Go binary (compiled from single Go Gateway codebase to the `g8e` binary) | Native Go via `Makefile` |

### Host-native Startup Lifecycle

The `./g8e platform start` command manages the sequence:
1. **Gateway binary check/build** → Governance Gateway (`g8eg`) starts in Gateway mode (--doctrine, --consensus, or --notary).
2. **Root of trust generation** (first boot only) - ECDSA P-384 CA hierarchy, intermediate CAs, and trust bundles in `.g8e/pki/`; `session_encryption_key`, `Actuator_signing_key` in `.g8e/secrets/`.
3. **Asynchronous convergence** - Services and clients poll health endpoints.

## State & Data Strategy

All runtime state is rooted at `./.g8e/`.

| Path | Purpose | `wipe` | `reset` | `clean` |
|---|---|---|---|---|
| `.g8e/pki/` | CA, intermediates, trust bundles | preserve | preserve | nuke |
| `.g8e/secrets/` | Bootstrap secrets (session key) | preserve | wipe | nuke |
| `.g8e/data/` | SQLite + blobs | wipe (API) | wipe | nuke |
| `.g8e/logs/` | Component stdout/stderr | - | - | nuke |
| `.g8e/pids/` | Process IDs | clear | clear | nuke |

- **`./g8e platform reset`**: Deletes the database and secrets, but keeps the CA cert so client trust is maintained (prompts for confirmation; bypass with `-y`, `--yes`, or `--force`).
- **`./g8e platform clean`**: Destructive removal of the entire `.g8e/` directory and all running processes (prompts for confirmation; bypass with `-y`, `--yes`, or `--force`).

## Anti-Tech-Debt Directives

AI agents tend to wrap poorly understood code in new abstractions. This is strictly forbidden.

1. **Rip and replace.** When existing code violates contracts or is structurally unsound, replace it correctly. Do not route around it with a wrapper. No compatibility is maintained for broken data structures or outdated shims.
2. **Prohibited patterns.** `ensure*()`, `getOrCreate*()`, `Any` in type signatures, and `map[string]interface{}` for known shapes are hard stops. Functions do exactly one thing: reads read, writes write.
3. **No defensive guards.** Never add defensive code at the call site to handle unexpected values. Hunt down the root cause and fix it at the source.

## Code Quality Standards

### General Principles
- **Industry Standards**: We adhere to modern, strict industry standards for every language we use.
- **Latest Versions**: Always use the latest stable versions of languages (Go 1.26+, Python 3.14+), libraries, and update methods. Avoid deprecated APIs and outdated patterns.
- **Fail-Closed**: If a security check, validation, or critical dependency fails, the system must halt immediately.
- **Explicit over Implicit**: No magic, no hidden side effects, and no "guessing" user intent.
- **Zero Tech Debt**: Every PR must leave the codebase cleaner than it was found.

### Go (`g8eo`)
- **Tooling**: `gofmt`, `goimports`, and `golangci-lint` are mandatory and must pass in CI.
- **Error Handling**: Always check errors. Wrap errors with context (e.g., `fmt.Errorf("failed to do x: %w", err)`) to provide a clear trace.
- **No Panics**: Never use `panic` in production paths. Return errors instead.
- **Concurrency**: Goroutines must be managed with `context.Context` for cancellation and `sync.WaitGroup` or channels for synchronization. No orphan goroutines.
- **Testing**: Table-driven tests are the standard for unit testing. Use `testify/assert` for readability.
- **Formatting**: Group imports into three blocks: standard library, external, and internal (`g8e/g8eo/...`).
- **Parameter Passing**: Use pointers for structs with mutable fields, maps, slices, or large data to avoid copying. Use values for small, read-only structs. Most service methods should avoid taking model structs as parameters; work with them internally within service boundaries.

## Application Boundary and State Management

1. **Single source of truth.** The `protocol/` directory is canonical for all wire-protocol values and cross-component document schemas.
2. **Strict typing.** Inside the application boundary, data lives exclusively as typed model instances. Raw dicts, untyped maps, and ad-hoc JSON are prohibited.
3. **Canonical JSON wire format.** Mutation envelopes are canonical-JSON `GovernanceEnvelope` carrying a base64-encoded binary protobuf `payload`.
4. **Cache-aside discipline.** All document operations go through `CacheAsideService`. The DB is authoritative for writes; the KV store is the primary read path.

## Component Rules

### g8eg (Gateway) / g8eo (Governed Operator) (Go)
- **LFAA payload stamping** - All LFAA results include an `execution_id`.
- **Concurrency** - Goroutines have explicit cancellation contexts and clear channel ownership.
- **Protocol boundary** - Any capability needed by bundled apps or BYO clients is exposed through the public gateway protocol.
- **Execution boundary** - Actuator is the sole circuit breaker before dispatch. Every accepted mutation emits a signed `ActionReceipt`.

## Testing

g8e is designed to be a testing environment and production environment at the same time. We do not mock internal services, database clients, or cross-component communication.

### Core Principles
1. **Reproduce first.** Always reproduce a bug with a failing test before generating the fix.
2. **No mocks.** Real database, real pub/sub, real LLM calls.
3. **Contract tests.** Enforce alignment between Operator, adapters, and `protocol/` with typed protobuf assertions.
4. **mTLS by Default.** Most communication requires mTLS. The test runner handles certificate injection from `.g8e/pki`.

### Test Runners
All substrate tests are orchestrated via the `./g8e` CLI. **Never call `go test` directly for substrate tests.**

| Command | Runner | Framework | Primary Use |
|---|---|---|---|
| `./g8e test` | Host Go | `go test` | Default Gateway test run (g8eo) |
| `./g8e test g8eo` | Host Go | `go test` | Operator listen mode, pub/sub |

## Documentation Guidelines

- **Docs are code.** Documentation is maintained with the same discipline as source code; stale or inaccurate docs are bugs.
- **Authoritative, not aspirational.** Document what the system does, not what it should do.
- **No redundancy.** Each fact lives in exactly one place; cross-link rather than repeat.
- **Writing style.** Present tense, active voice, direct and specific. No filler, no emojis.
- **Single source of truth.** The `protocol/` directory is canonical for all wire-protocol values.

### The `updatedocs` Workflow
1. **Code-first discovery.** Never trust existing documentation. Verify against the implementation.
2. **High signal, low noise.** Focus on system lifecycle and request/data progression.
3. **Why vs. how.** `.md` files explain high-level concepts; implementation details belong in code.

## Doctrine Ingestion

g8e ingests industry security doctrines from OWASP CRS, Gitleaks, Semgrep, and secrets-patterns-db. Doctrines are stored in `protocol/constants/doctrine/` as canonical JSON and loaded by the L1Doctrine service at startup.

### Doctrine Schema

Each doctrine file follows this canonical schema:

```json
{
  "source": "owasp_crs",
  "version": "4.0.0",
  "last_updated": "2026-05-22",
  "license": "Apache-2.0",
  "doctrines": [
    {
      "id": "owasp_crs_932100",
      "name": "RCE: nc -e reverse shell",
      "category": "reverse_shell",
      "severity": "critical",
      "pattern": "(?i)nc\\s+.*-e\\s+(/bin/)?(sh|bash|zsh)",
      "mitre_attack": "T1059.004",
      "mitre_tactic": "Execution",
      "confidence": 0.95,
      "enabled": true
    }
  ]
}
```

### Makefile Targets

| Target | Purpose |
|---|---|
| `make validate-doctrines` | Validate JSON schema for all doctrine files |

### Adding New Doctrines

1. Create or update the doctrine JSON file in `protocol/constants/doctrine/`
2. Run `make validate-doctrines` to ensure JSON validity
3. Restart g8eo to load the new doctrines (L1Doctrine loads doctrines at startup)

### MCP/Agentic-Specific Doctrines

g8e defines unique threat doctrines for agentic execution in `mcp_vectors_doctrine.json`:
- Tool response injection
- Unsafe argument handling in MCP tools
- Prompt injection via tool outputs
- Credential exposure in tool responses
- GovernanceEnvelope field abuse
- MCP protocol misuse

## Working with Constants

g8e maintains a Single Source of Truth (SSOT) for cross-component constants in JSON at `protocol/constants/`. Go and Python consume these constants via generated registry files.

### Adding New Constants

1. Add the constant to the appropriate JSON file in `protocol/constants/`
2. Run `make constants` to regenerate Go registry files from JSON
3. Run `go run ./internal/constants/check_registry.go` to verify registration
4. Commit both the JSON source and generated Go files

### Regeneration Commands

- `make constants` - Generate Go registry files from JSON SSOT
- `make generate` - Generate both protobuf and constants
- `make clean-constants` - Remove generated constants

### Tracked vs Internal Files

The registry tracking system distinguishes exportable constants from internal-only constants. Tracked JSON files (collections.json, events.json, headers.json, channels.json, etc.) are exported to Go registry files. Internal-only Go files (status.go, platform.go, agents.go, timestamp.go) contain Go-specific enums and are not exported from JSON.

See `docs/reference/constants.md` for complete documentation of the constants pipeline.

## Where to Find Things

| Concern | Location |
|---|---|
| Protobuf schemas | `protocol/proto/` |
| Constants registries (JSON SSOT) | `protocol/constants/` |
| Constants documentation | `docs/reference/constants.md` |
| Go registry files | `internal/constants/` |
| Operator implementation | Root-level (cmd/, internal/, pkg/) |
| CLI | Unified `g8e` binary (daemon + CLI modes) |

## Submitting a PR

1. Keep it focused (one change per PR is best).
2. Add a test if you're fixing a bug or adding a feature.
3. Use a clear prefix in your commit like `g8eo: fix the thing`.
4. We'll jump in to review as soon as we can!

## Get in Touch

Have questions? Email danny@g8e.ai. It's the fastest way to get help or talk shop.

## The Fine Print (CLA)

By contributing, you grant us a license to use your work in g8e (Apache 2.0). You still own your code, but you're giving us permission to build the platform with it. Thanks for helping us grow!
