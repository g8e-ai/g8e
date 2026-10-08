# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this repository is

g8e is a zero-trust execution and evidence platform for AI agents, human operators, and distributed target
runtimes: a **Gateway** (Policy Decision Point) admits typed intent, a host-side **Operator** (Policy Execution
Point) independently re-verifies and executes it, and every mutation is signed and recorded at the local
execution boundary. The core principle: a model can ask, but it cannot directly act.

It is a polyglot monorepo:

- **Go** (`cmd/g8e/`, `internal/`) — the single static `g8e` binary. Runs as Gateway (`g8e gw start`), Operator
  (`g8e operator start`), CLI, TUI, eval/compliance tooling.
- **`protocol/`** — canonical Protobuf contracts (`protocol/proto/g8e/`) plus generated Go/Python/TypeScript
  bindings and JSON constant registries. Shared wire contract for every service.
- **`ensemble/`** (`g8ee`) — optional Python 3.12 / FastAPI agentic app (Tribunal deliberation, ReAct tool loops,
  multi-provider LLM routing). Talks to the Gateway over mTLS; never has execution authority itself.
- **`console/`** — the browser frontend: React + TypeScript (Vite) SPA, embedded in the binary and served by the
  Gateway at `/console/`. Passkey auth, approvals, Operator inventory/binding, cases, investigations, chat. The
  browser talks only to the Gateway; chat reaches g8ee through the Gateway's ensemble browser proxy.
- **`g8e-adapter/`** and **`evaluation-explorer/`** — audited adapter + contract pack for generated observe
  frontends, and the evaluation explorer SPA (its build is embedded by `make build`).

Full architectural detail lives in `docs/`, not in this file — see "Where to look" below.

## Build and test

Always use `./g8e test <suite>` or the listed `make` targets rather than calling `go test` directly — they apply
required environment flags and FIPS configuration.

```bash
# Build the unified binary (writes bin/g8e-<os>-<arch> and a repo-root copy)
make build
./g8e --help

# Go platform tests
./g8e test unit          # Tier 1: fast, no network/disk/parallel-safe
./g8e test integration   # Tier 2: real local SQLite/PKI/pub-sub, race detector, no t.Parallel()
./g8e test e2e           # Tier 3: against an already-running stack
./g8e test e2e-docker    # Tier 3 (requires Docker): compose up, wait for health, run e2e, compose down -v
./g8e test coverage
./g8e test lint          # golangci-lint; make lint also runs vulncheck and doctrine validation
./g8e test chaos
./g8e test summary
./g8e test public-loop --candidate candidate.json --output evidence.json

# Narrow integration tests by package/name
./g8e test integration --pkg ./internal/cli/cmd/gw --run TestGatewayConnect

# Ensemble (Python) — from ensemble/, or via root make targets
cd ensemble && make test          # all tests/, no external-marker exclusion
make ensemble-test                # tests/unit + tests/integration, excludes external markers
make ensemble-test-external       # markers: ai_integration, requires_web_search, requires_api, requires_system_one
ensemble/.venv/bin/python -m pytest tests/unit/services/evaluation/test_trace_service.py -v  # single test

# Console (React/TS) — from console/, or via root make targets
cd console && npm test             # Vitest once
make console-test                  # Vitest suite from repo root
make console-lint                  # tsc --noEmit + ESLint
make console-embed   # rebuild and refresh the Gateway embed (commit static/ with the source change)
make console-embed-check           # fail if the committed embed is stale

# Claude Code plugin (claude-plugin/)
make claude-plugin-test            # hook tests, shellcheck, claude plugin validate

# Protocol / generated code
make proto-generate       # regenerates Go, Python, TS bindings + lockfiles — never hand-edit generated output
make constants-check      # verify generated event/action-type constants are current
make swagger-generate     # regenerate Gateway OpenAPI from Go Swagger annotations
make doctrines-validate
```

## High-level architecture

### The five-layer governance pipeline

Every governed mutation is a canonical protobuf `GovernanceEnvelope` that must pass all five layers before it can
execute; universal checks (hash integrity, replay nonce, expiry, state-root, payload decode) fail closed in every
posture:

| Layer | Owner | Job |
| --- | --- | --- |
| L1 Doctrine | Gateway + Operator | Forbidden-pattern / MITRE ATT&CK-style heuristics. Always enforced. |
| L2 Consensus | Gateway coord., Operator verify | K-of-N Ed25519 quorum over `<tx_hash>\|<decision>`. |
| L3 Notary | Gateway coord., Operator verify | Human authorization (WebAuthn, or signed CLI proof for outbound Operators). |
| L4 Warden | Executing Operator | Pre-dispatch gate: nonce reservation, expiry, hash recompute, state-root check, posture-required L2/L3 proof. |
| L5 Actuator | Executing Operator | Sole execution boundary: signs `EXECUTING`/`COMPLETED`/`FAILED` receipts, mints a short-lived capability, dispatches the handler, appends to the local commitment ledger. |

Governance **posture** (`--posture doctrine|consensus|ratify|notary`, set at Gateway startup, immutable at
runtime) decides whether L2/L3 gate execution or are merely audited. L1, L4, and L5 always run.

### Runtime modes — three distinct owners, do not conflate

- **Gateway HTTP/control plane** — `gateway.GatewayModeService` in `internal/services/gateway/`.
- **Embedded (in-process) Operator substrate**, used for Gateway-local actions — `embedded.Service` in
  `internal/services/gateway/embedded/`.
- **Outbound Operator**, the remote PEP — `services.G8eoService` in `internal/services/g8eo.go`, started from
  `internal/cli/serve/operator.go`. Connects outbound-only over mTLS to `cmd:<operator_id>:<operator_session_id>`;
  Operators expose **no inbound listeners**. `G8eoService` must never construct `mcp.GatewayService`.

### Ingress paths into the pipeline

MCP (`POST /mcp` or stdio), A2A (`POST /a2a`), governed HTTP dispatch (`POST /api/v1/operators/commands`), direct
envelope submission (`POST /api/v1/governance/envelopes`), and CLI `operator run`. Direct envelope submission and
governed HTTP dispatch must never synthesize missing L2/L3 proof — they fail closed at L4 instead. Client-native
tools, external MCP wrappers, and other side channels are explicitly **outside** the governance boundary.

### Storage: Local-First Audit Architecture (LFAA)

Each executing runtime (Gateway, each Operator) is authoritative for its own SQLite audit DB, execution vault, and
state root under `.g8e/`. Gateway-mirrored receipts are best-effort projections only — never treat them as the
source of truth over an Operator's local store. All `.g8e/` I/O must go through `RuntimeFileService`
(`internal/services/fs/`), never raw `os` calls or hardcoded paths.

### Repository map

```text
cmd/g8e/              Binary entry point
internal/cli/cmd/     Cobra CLI groups (gw, auth, mcp, operator, vault, test, docker, audit,
                       report, public, swagger, tui, version, compliance, ensemble, eval)
internal/services/    Gateway, outbound Operator, governance (L1-L5), consensus, storage, network,
                       vault, sse, pubsub, mcp (native tool registry), evaluation, compliance
protocol/             Protobuf contracts, constants registries, generated Go/Python/TS bindings
ensemble/             g8ee — Python/FastAPI agentic app
console/              Browser console (React/TS), embedded in the Gateway at /console/
g8e-adapter/          Audited browser adapter + observe-frontend contract pack
claude-plugin/        Claude Code plugin (marketplace manifest in .claude-plugin/)
evaluation-explorer/  Frontend for live model-campaign inspection
eval/                 Evaluation fixtures, campaign schemas, benchmark datasets
docs/                 Full platform documentation (see below)
```

## Where to look before making changes

`docs/devs/` is written specifically for maintainers *and coding agents*, and is kept current with invariant IDs
(`INV-*`) that are enforced in review. Read the relevant one before touching that area — it is more precise and
current than any summary here:

- **`docs/devs/devs.md`** — coding invariants: error handling (`internal/constants/errors.go`), typed-contract
  rules, `.g8e/` filesystem rules, dependency construction order, doctrine loading, native MCP tool registration.
- **`docs/devs/codemap.md`** — which package owns which runtime mode / CLI group / persistence store.
- **`docs/devs/tests.md`** — the four-tier test model, fixture/cleanup rules (`t.Cleanup`, no `t.Parallel()` in
  Tier 2/3, no double-closing `NewGatewayFixture`), `testutil` isolation helpers.
- **`docs/ensemble/tests.md`** — ensemble (Python) test tiers, pytest markers, external-provider/credential
  gating, and fixture ownership (`conftest.py`, `tests/fakes/`).
- **`docs/devs/docs.md`** — documentation format/audit rules if you edit anything under `docs/`.
- **`docs/architecture/`** — one file per subsystem (`gateway.md`, `operator.md`, `governance.md`, `consensus.md`,
  `auth.md`, `network.md`, `encryption.md`, `storage.md`, `agents.md`, `ensemble.md`, `console.md`, `protocol.md`,
  `evals.md`, `model-provenance.md`, `public_spectator.md`, `sse.md`).
- **`docs/ensemble/devs.md`** and **`docs/architecture/console.md`** — component-specific dev and architecture guides (service wiring,
  coding standards, source layout).
- **CLI behavior** (flags, defaults) — always confirm with `./g8e <command> --help`; do not trust older docs or
  memory for flag inventories, they drift.

## Invariants most likely to matter to an agent editing this codebase

These are pulled out because they are easy to violate accidentally; the full, authoritative invariant lists are in
the linked docs above.

- Never bypass the envelope/L1–L5 pipeline for a new mutation — classify it in the action/event registries, wrap
  it in a `GovernanceEnvelope`, and let L4/L5 handle verification and dispatch. Don't call mutation handlers
  directly.
- Don't add compatibility shims for broken paths, `ensure*`/`getOrCreate*` helpers that hide a write inside a
  lookup, or a second implementation of something that already exists — search first, extend existing
  services/utilities/constants.
- Sentinel errors live only in `internal/constants/errors.go`; wrap errors with `%w` and operation context; never
  compare rendered error strings when `errors.Is`/`errors.As` works.
- Known shapes (envelopes, proofs, receipts, HTTP contracts) must stay typed — no protobuf `Any` or
  `map[string]interface{}` for a known contract.
- `.g8e/` runtime state goes through `fs.RuntimeFileService` only; path strings come from
  `internal/constants/paths.go`.
- A new native MCP tool implements `mcp.NativeTool` and is registered explicitly in `RegisterNativeTools`
  (`internal/services/mcp/native_tool_registry.go`) — never from `init()`, and never copy the tool list into docs.
- Generated artifacts (protobuf bindings, Gateway OpenAPI, doctrine JSON) change only through their source
  + generator (`make proto-generate`, `make swagger-generate`, `make doctrines-validate`) — never
  hand-edited.
- Run platform suites only through `./g8e test <suite>` or the matching `make` target — never `go test` directly
  for a platform suite — and never call `t.Parallel()` in a Go integration or E2E test.
- Go tests get isolated runtime roots from `testutil.TempDir`/`testutil.TestPaths` (never append `.g8e` by hand).
  `NewGatewayFixture` registers its own `t.Cleanup` teardown — don't stop its Gateway or close its databases a
  second time.
- `make ensemble-test` only collects `tests/unit/` + `tests/integration/` and excludes the external-provider
  markers (`ai_integration`, `requires_web_search`, `requires_api`, `requires_system_one`); a bare `pytest tests/`
  collects everything, including those. Use `make ensemble-test-external` for that tier.
- Update documentation and generated artifacts in the same change as the behavior they describe.
