# AGENTS.md

This file provides guidance and operational constraints for AI coding agents, autonomous workers, and coding assistants operating in this repository.

## What this repository is

**g8e** is a zero-trust execution and evidence platform for AI agents, human operators, and distributed target runtimes:
- A **Gateway** (Policy Decision Point, PDP) admits typed intent.
- A host-side **Operator** (Policy Execution Point, PEP) independently re-verifies and executes it.
- Every mutation is cryptographically signed and recorded at the local execution boundary.

**Core Principle**: *A model can ask, but it cannot directly act.*

### Polyglot Monorepo Structure

- **Go** (`cmd/g8e/`, `internal/`) — The single static `g8e` binary. Runs as Gateway (`g8e gw start`), Operator (`g8e operator start`), CLI, TUI, and evaluation/compliance tooling.
- **`protocol/`** — Canonical Protobuf contracts (`protocol/proto/g8e/`) plus generated Go, Python, and TypeScript bindings and JSON constant registries. Shared wire contract for every service.
- **`ensemble/` (`g8ee`)** — Optional Python 3.12 / FastAPI agentic application (Tribunal deliberation, ReAct tool loops, multi-provider LLM routing). Communicates with the Gateway over mTLS; never possesses direct execution authority.
- **`console/`** — Browser frontend: React + TypeScript (Vite) SPA, embedded in the Go binary and served by the Gateway at `/console/`. Handles passkey authentication, human approvals, Operator inventory/binding, cases, investigations, and chat. The browser communicates only with the Gateway; chat reaches `g8ee` via the Gateway's ensemble browser proxy.
- **`g8e-adapter/` and `evaluation-explorer/`** — Audited browser adapter + contract pack for generated observe frontends, and the evaluation explorer SPA (embedded during `make build`).
- **`eval/` and `demos/`** — Evaluation fixtures, campaign schemas, benchmark datasets, and sealed vertical demo environments (Healthcare, Finance, DHS, FedRAMP).

Full architectural specifications live in `docs/` — see [Where to look before making changes](#where-to-look-before-making-changes).

---

## Build and Test

> **CRITICAL**: Always use `./g8e test <suite>` or the listed `make` targets rather than invoking `go test` directly. They configure mandatory environment variables, tags, and FIPS settings.

```bash
# Build the unified binary (writes bin/g8e-<os>-<arch> and repo-root ./g8e)
make build
./g8e --help

# Go platform test suites
./g8e test unit          # Tier 1: fast, no network or disk dependencies, parallel-safe
./g8e test integration   # Tier 2: real local SQLite/PKI/pub-sub, race detector, NO t.Parallel()
./g8e test e2e           # Tier 3: against an already-running stack
./g8e test e2e-docker    # Tier 3 (Docker required): compose up, wait for health, run e2e, compose down -v
./g8e test coverage      # Coverage report
./g8e test lint          # Linting: vulncheck, doctrine/COSAiS validation, golangci-lint (also: make lint)
./g8e test chaos         # Chaos & fault injection tests
./g8e test summary       # Test run summary
./g8e test public-loop --candidate candidate.json --output evidence.json

# Narrow Go integration tests by package or test name
./g8e test integration --pkg ./internal/cli/cmd/gw --run TestGatewayConnect

# Ensemble (Python) — run from ensemble/ or via root makefile
cd ensemble && make test          # All tests under tests/, no marker exclusions
make ensemble-test                # tests/unit + tests/integration, excludes external provider markers
make ensemble-test-external       # Runs markers: ai_integration, requires_web_search, requires_api, requires_system_one
ensemble/.venv/bin/python -m pytest tests/unit/services/evaluation/test_trace_service.py -v  # Single pytest file

# Console (React/TS) — run from console/ or via root makefile
cd console && npm test             # Run Vitest once
make console-test                  # Run Vitest suite from repo root
make console-lint                  # Typecheck (tsc --noEmit) + ESLint
make console-build embed-console   # Rebuild console and refresh Gateway embedded assets (commit static/)
make console-embed-check           # Fails if committed embedded assets are stale relative to console/

# Protocol / Code Generation
make proto-generate       # Regenerate Go, Python, TS bindings + lockfiles (never hand-edit output)
make constants-check      # Verify generated event/action-type constants match protocol definitions
make swagger-generate     # Regenerate Gateway OpenAPI spec from Go Swagger annotations
make doctrines-validate   # Validate doctrine definitions
make cosais-validate      # Validate COSAiS JSON registries
```

---

## High-Level Architecture

### The Five-Layer Governance Pipeline

Every governed mutation is represented as a canonical protobuf `GovernanceEnvelope` that must pass all five layers before execution. Universal checks (hash integrity, replay nonce, expiry, state-root check, payload decode) fail closed across all postures:

| Layer | Owner | Responsibilities |
| :--- | :--- | :--- |
| **L1 Doctrine** | Gateway + Operator | Forbidden-pattern and MITRE ATT&CK-style heuristic checks. Always enforced. |
| **L2 Consensus** | Gateway (coord.), Operator (verify) | $K$-of-$N$ Ed25519 quorum signature check over `<tx_hash>\|<decision>`. |
| **L3 Notary** | Gateway (coord.), Operator (verify) | Human authorization gate (WebAuthn, or signed CLI cryptographic proof for outbound Operators). |
| **L4 Warden** | Executing Operator | Pre-dispatch gate: nonce reservation, TTL expiry, hash recalculation, state-root check, and posture-required L2/L3 proofs. |
| **L5 Actuator** | Executing Operator | Sole execution boundary: signs `EXECUTING`/`COMPLETED`/`FAILED` receipts, mints short-lived capabilities, dispatches handler, appends to local commitment ledger. |

#### Governance Postures
Set via `--posture doctrine|consensus|ratify|notary` at Gateway startup (immutable at runtime). Posture determines whether L2/L3 gate execution or are merely audited. **L1, L4, and L5 are unconditionally enforced.**

### Runtime Modes (Three Distinct Owners)

Do not conflate these three runtime execution modes:
1. **Gateway HTTP / Control Plane**: `gateway.GatewayModeService` in `internal/services/gateway/`.
2. **Embedded (In-Process) Operator Substrate**: `embedded.Service` in `internal/services/gateway/embedded/`. Used strictly for Gateway-local actions.
3. **Outbound Operator (Remote PEP)**: `services.G8eoService` in `internal/services/g8eo.go` (started from `internal/cli/serve/operator.go`). Connects strictly outbound over mTLS to `cmd:<operator_id>:<operator_session_id>`. Operators expose **no inbound listening ports**. `G8eoService` must **never** instantiate or construct `mcp.GatewayService`.

### Ingress Paths

- **Supported Ingress**: MCP (`POST /mcp` or stdio), A2A (`POST /a2a`), governed HTTP dispatch (`POST /api/v1/operators/commands`), direct envelope submission (`POST /api/v1/governance/envelopes`), and CLI `operator run`.
- **Fail-Closed Boundary**: Direct envelope submission and governed HTTP dispatch must never synthesize or forge missing L2/L3 proofs; they fail closed at L4 instead.
- **Out of Scope**: Client-native tools, external MCP wrappers, and unmonitored side channels are explicitly outside the governance boundary.

### Storage: Local-First Audit Architecture (LFAA)

Each executing runtime (Gateway, each Operator) is strictly authoritative for its own SQLite audit DB, execution vault, and state root under `.g8e/`.
- Gateway-mirrored receipts are best-effort projections only — never treat them as authoritative over an Operator's local store.
- All `.g8e/` filesystem I/O **must** route through `RuntimeFileService` (`internal/services/fs/`). Never use raw `os` file operations or hardcoded path literals.

---

## Repository Map

```text
cmd/g8e/              Binary entry point
internal/cli/cmd/     Cobra CLI command groups (gw, auth, mcp, operator, vault, test, demos,
                      docker, audit, report, public, swagger, tui, version, compliance, eval)
internal/services/    Gateway, outbound Operator, governance (L1–L5), consensus, storage, network,
                      vault, SSE, pubsub, MCP native tools, evaluation, compliance
protocol/             Canonical Protobuf contracts, constants registries, generated Go/Python/TS bindings
ensemble/             g8ee — Python 3.12 / FastAPI agentic application
console/              Browser console (React + TypeScript + Vite), embedded in Gateway at /console/
g8e-adapter/          Audited browser adapter + observe-frontend contract pack
evaluation-explorer/  Frontend SPA for live model-campaign inspection
eval/                 Evaluation fixtures, campaign schemas, benchmark datasets
demos/                Healthcare, finance, DHS, FedRAMP sealed demo environments
docs/                 Full platform documentation and developer invariant specifications
```

---

## Where to Look Before Making Changes

`docs/devs/` is written specifically for maintainers and coding agents, enforcing concrete invariant IDs (`INV-*`). Review the corresponding guide before editing code in that area:

- **`docs/devs/devs.md`** — Coding invariants: error handling conventions (`internal/constants/errors.go`), typed-contract requirements, `.g8e/` filesystem rules, dependency construction order, doctrine loading, and native MCP tool registration.
- **`docs/devs/codemap.md`** — Package architecture map detailing runtime modes, CLI groups, and persistence store ownership.
- **`docs/devs/tests.md`** — Four-tier test model, test isolation rules, fixture lifecycle (`t.Cleanup`, banning `t.Parallel()` in Tier 2/3, avoiding double teardown of `NewGatewayFixture`), and `testutil` helpers.
- **`docs/ensemble/tests.md`** — Python/Ensemble test tiers, pytest markers, external provider/credential gating, and fixture ownership (`conftest.py`, `tests/fakes/`).
- **`docs/ensemble/devs.md`** — Ensemble service wiring, coding standards, and internal architecture.
- **`docs/architecture/console.md`** — Console frontend architecture, routing, API client, and embed lifecycle.
- **`docs/devs/docs.md`** — Documentation formatting, catalogs, and audit rules for edits under `docs/`.
- **`docs/architecture/`** — Subsystem architecture documents (`gateway.md`, `operator.md`, `governance.md`, `consensus.md`, `auth.md`, `network.md`, `encryption.md`, `storage.md`, `agents.md`, `ensemble.md`, `console.md`, `protocol.md`, `evals.md`, `model-provenance.md`, `public_spectator.md`, `sse.md`).
- **CLI Flags & Defaults** — Always confirm via `./g8e <command> --help`; do not rely on static memory or older text documentation for flag inventories.

---

## Critical Invariants for Agents Editing Code

These rules prevent frequent architectural, security, and runtime regressions:

1. **Pipeline Integrity**:
   - Never bypass the envelope/L1–L5 pipeline for any new mutation.
   - Classify mutations in the action/event registries, wrap them in a `GovernanceEnvelope`, and let L4/L5 handle verification and dispatch. Never call internal mutation handlers directly.

2. **No Shims or Concealed Mutations**:
   - Do not add backwards-compatibility shims for legacy or broken paths.
   - Never write `ensure*` or `getOrCreate*` helper functions that hide a write inside a lookup/getter.
   - Search before implementing; extend existing services, utilities, and constants rather than introducing duplicate implementations.

3. **Error Handling**:
   - Sentinel errors reside exclusively in `internal/constants/errors.go`.
   - Wrap errors with `%w` and relevant operational context.
   - Never inspect or match rendered error strings (`err.Error() == ...`); always use `errors.Is` or `errors.As`.

4. **Strictly Typed Wire Contracts**:
   - Known schemas (envelopes, proofs, receipts, HTTP payloads) must remain strongly typed.
   - Never use `google.protobuf.Any` or `map[string]any` / `map[string]interface{}` when a typed contract exists or can be defined.

5. **Filesystem Isolation**:
   - All `.g8e/` runtime state operations must route through `fs.RuntimeFileService`.
   - Path constants must come from `internal/constants/paths.go`. Never construct ad-hoc paths or call raw `os` filesystem primitives directly for runtime state.

6. **Native MCP Tool Registration**:
   - Native MCP tools must implement `mcp.NativeTool`.
   - Register tools explicitly in `RegisterNativeTools` (`internal/services/mcp/native_tool_registry.go`). Never register tools inside package `init()`, and never duplicate tool lists into documentation.

7. **Generated Artifacts**:
   - Protobuf bindings, Gateway OpenAPI specs, and doctrine/COSAiS JSON registries must only change via their sources and generator commands (`make proto-generate`, `make swagger-generate`, `make doctrines-validate`, `make cosais-validate`). Never hand-edit generated files.

8. **Testing Discipline**:
   - Always execute platform test suites through `./g8e test <suite>` or the corresponding `make` targets — never run bare `go test` for platform suites.
   - **Never** call `t.Parallel()` in Go Tier 2 (integration) or Tier 3 (E2E) tests.
   - Obtain isolated runtime roots using `testutil.TempDir` or `testutil.TestPaths` (never append `.g8e` by hand).
   - `NewGatewayFixture` registers its own `t.Cleanup` teardown; do not manually stop its Gateway or close its databases a second time.
   - In Python, `make ensemble-test` runs unit and integration tests while excluding external provider markers (`ai_integration`, `requires_web_search`, `requires_api`, `requires_system_one`). Use `make ensemble-test-external` to run suites requiring real credentials or live APIs.

9. **Atomic Updates**:
   - Whenever behavior, flags, or contracts change, update documentation and regenerate relevant artifacts within the same change.

