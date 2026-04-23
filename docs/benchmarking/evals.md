# g8e Evals — Real-Operator Evaluation Framework

The g8e evals framework provides end-to-end testing of the g8e platform against real operator containers, using the same public API surface and authentication flow that end users experience. This framework exercises the complete product stack — from device link token generation through chat interactions, tool execution, and approval workflows.

## Overview

The evals framework is designed to:

- **Exercise the real product surface** — Evals hit the same `g8ed` HTTPS API that browsers use, with device link tokens generated from the dashboard. No internal auth tokens, slot-provisioning shortcuts, or fake operator documents.
- **Use real operator containers** — Each scenario runs against dedicated operator containers on the `g8e-network`, mirroring the `demo/web-node` pattern. Containers are disposable and isolated from the test runner.
- **Run from the Docker host** — Evals launch via `./g8e evals run` on the host, not inside `g8ee-test-runner`. The framework does not use pytest, which remains dedicated to unit/integration/e2e tests.
- **Support deterministic and generative testing** — Gold-set JSON files define scenario inputs. The framework uses deterministic matchers for tool calls and LLM judges for behavior validation.
- **Enable rapid iteration** — The fleet stands up in seconds, scenarios run in parallel, and failures produce container logs and reproduction commands.

## Architecture

```
Host
 ├── ./g8e evals run
 │     └── components/g8ee/evals/runner/  (Python, runs on host, orchestrator)
 │           ├── docker compose -f components/g8ee/evals/docker-compose.evals.yml up
 │           │     ├── eval-node-01..NN    (real operator containers, device-token auth)
 │           │     └── (joins g8e-network, reaches g8e.local == g8ed)
 │           ├── g8ed HTTPS API            (chat + investigations + approvals)
 │           └── writes components/g8ee/reports/evals/<ts>/…
 │
 └── g8e platform (docker-compose.yml)     (g8ed, g8ee, g8es, g8ep — unchanged)
```

Key properties:

- The runner uses **only** public surfaces: device-link auth → POST chat → stream SSE → optionally POST approval. No `X-Internal-Auth`, no direct g8es document writes.
- Each eval node is a real Linux container with a real filesystem; scenarios like "tail /var/log/auth.log" actually tail a log file. Broken/healthy profiles seed the environment.
- The runner does not mint device tokens; the user provides a token generated from the dashboard, matching the end-user workflow.

## File Layout

```
components/g8ee/evals/
├── README.md
├── docker-compose.evals.yml              # Eval fleet (N operator containers)
├── containers/
│   └── eval-node/
│       ├── Dockerfile
│       └── entrypoint.sh                 # Downloads + supervises operator
├── runner/
│   ├── __init__.py
│   ├── cli.py                            # ./g8e evals run entrypoint
│   ├── fleet.py                          # compose up/down, health, discover
│   ├── client.py                         # g8ed chat/approval/SSE client
│   ├── scorer.py                         # judge + deterministic matchers
│   ├── reporter.py                       # report generation
│   └── metrics.py                        # metrics collection
└── gold_sets/
    ├── accuracy.json                     # accuracy scenarios
    ├── benchmark.json                    # benchmark scenarios
    └── privacy.json                      # privacy scenarios

docs/benchmarking/evals.md                # this file
components/g8ee/reports/evals/            # artifact output directory
```

Note: Non-operator gold-set tests (accuracy/benchmark/privacy that drive LLM+validation paths without a real operator) remain under `tests/evals/` and continue using pytest. The evals framework specifically targets the **real-operator** dimension.

## Container Design — `eval-node`

The eval-node container mirrors the `demo/containers/web-node` pattern, stripped to the minimum needed for operator coverage.

### Dockerfile

- Base: `debian:stable-slim` (closer to a real target than alpine)
- Installs: `bash curl coreutils iproute2 procps file openssh-client grep sed awk findutils less` plus any tools required by gold-set scenarios (logs, nginx, etc — profile-driven)
- Non-root `appuser` for the operator, with optional root-profile container for root-level scenarios
- Writes realistic filesystem fixtures (`/var/log/auth.log`, `/etc/app/config.json`, etc) at entrypoint so scenarios have real data to interact with

### Entrypoint Responsibilities

The entrypoint script handles:

1. **Token validation** — If `DEVICE_TOKEN` is unset, exit non-zero. Evals must always have a token.
2. **Operator download** — Download `g8e.operator` from `https://${G8E_ENDPOINT:-g8e.local}/operator/download/linux/amd64` with `Authorization: Bearer $DEVICE_TOKEN`, verifying the checksum and retrying on failure.
3. **Operator execution** — Execute the operator with `-e g8e.local -D $DEVICE_TOKEN --no-git`. The default `--http-port` and `--wss-port` settings are used to match platform expectations.
4. **Supervision** — Run a supervised restart loop with stdout prefixed with `[eval-node-NN operator]` for readable `docker logs`.

### Docker Compose Configuration

The `docker-compose.evals.yml` file defines:

- External network `g8e-network` and external volume `g8es-ssl` (same as demo)
- N services `eval-node-01..NN` using an anchor block, each with environment variables `DEVICE_TOKEN`, `EVAL_NODE_ID`, and optional `EVAL_PROFILE`
- Labels `io.g8e.evals=true` for discovery
- No exposed host ports; everything stays on `g8e-network`

## Orchestrator — `./g8e evals run`

### CLI Interface

The evals CLI runs on the Docker host:

```
./g8e evals run --device-token dlk_xxx [--suite accuracy|benchmark|privacy|all] \
                [--nodes 5] [--scenarios id1,id2] [--parallel 4] \
                [--llm-provider …] [--llm-model …] [--report-dir …]
./g8e evals up   --device-token dlk_xxx --nodes 5    # bring up fleet without running
./g8e evals down                                     # tear down fleet
./g8e evals status
./g8e evals logs eval-node-03
```

### Scenario Execution Flow

For each scenario, the orchestrator:

1. **Selects a node** — Uses round-robin or pins by `scenario.required_profile`. Waits until the operator reports BOUND status via the public API.
2. **Creates an investigation** — Uses the same POST endpoint the dashboard uses.
3. **Sends the chat message** — Streams SSE back, capturing all tool calls, approvals required, final message text, and timing. The g8ee agent is the real agent with no mocking.
4. **Auto-approves requests** — Approves any `approval_required` events using the same public approval endpoint the dashboard uses. Privacy/safety scenarios assert that certain tool calls never reach approval or that Sentinel blocked them.
5. **Scores the result** — Uses `scorer.py` with:
   - Deterministic matchers for `expected_tool`/`expected_payload` (benchmark gold-set)
   - LLM judge for `expected_behavior`/`required_concepts` (accuracy gold-set)
   - Sentinel assertions for privacy (secrets must not appear in final response)
6. **Records metrics** — Creates an `EvalRow` (same dataclass used in `tests/evals/metrics.py`).
7. **Resets node state** — Between scenarios, resets the node state (currently via `docker restart eval-node-NN`, which takes <2s).
8. **Generates reports** — After all scenarios complete, `reporter.persist_report(...)` writes `report.txt`, `results.csv`, and `summary.json` to `components/g8ee/reports/evals/<ts>/`.

### Failure Handling

On failure, the runner prints:
- The scenario ID
- The final SSE timeline
- `docker logs eval-node-NN --tail=200`
- The exact command to re-run just that scenario

### Parallelism

Scenarios are independent when each owns its own node. The `--parallel` flag is capped at `--nodes` to ensure adequate resources.

## Gold Sets

Gold sets define the test scenarios for evals. They are located in `components/g8ee/evals/gold_sets/`:

- `accuracy.json` — Accuracy scenarios with LLM judge validation
- `benchmark.json` — Benchmark scenarios with deterministic tool matching
- `privacy.json` — Privacy scenarios with Sentinel assertions

Each gold set is validated on load using schema checks from the runner module. Scenarios marked with `agent_mode: "OPERATOR_BOUND"` are first-class inputs for `./g8e evals run` and execute against real operators.

## Usage

### Prerequisites

1. **Platform running** — Ensure the g8e platform is running via `./g8e platform up`
2. **Device link token** — Generate a device link token from the dashboard. The runner does not mint tokens; this is a user action by design.
3. **Network access** — The Docker host must have access to the `g8e-network` and `g8es-ssl` volume

### Quick Start

```bash
# Bring up the eval fleet
./g8e evals up --device-token dlk_xxx --nodes 5

# Run all benchmark scenarios
./g8e evals run --device-token dlk_xxx --suite benchmark

# Run specific scenarios
./g8e evals run --device-token dlk_xxx --scenarios scenario_id_1,scenario_id_2

# Tear down the fleet
./g8e evals down
```

### Running Specific Suites

- **Accuracy suite**: `./g8e evals run --device-token dlk_xxx --suite accuracy`
- **Benchmark suite**: `./g8e evals run --device-token dlk_xxx --suite benchmark`
- **Privacy suite**: `./g8e evals run --device-token dlk_xxx --suite privacy`
- **All suites**: `./g8e evals run --device-token dlk_xxx --suite all`

### Fleet Management

```bash
# Check fleet status
./g8e evals status

# View logs for a specific node
./g8e evals logs eval-node-03

# Bring up fleet without running scenarios
./g8e evals up --device-token dlk_xxx --nodes 3
```

### Reports

After a run completes, reports are generated in `components/g8ee/reports/evals/<timestamp>/`:

- `report.txt` — Human-readable summary
- `results.csv` — Machine-readable results per scenario
- `summary.json` — Aggregated metrics and statistics

### CI Integration

Real-operator evals require a device token and are excluded from PR CI runs. They can be run as an opt-in nightly job or manually for validation.

## Design Decisions

### No Internal Auth Surface

The runner uses only public APIs — device-link auth, POST chat, SSE streaming, and approval endpoints. No `X-Internal-Auth` headers or direct g8es document writes. If a scoring need cannot be met through public APIs, that represents a product gap to be fixed in the product itself, not a shortcut in evals.

### Device Token Management

The runner does not auto-mint device tokens. Users generate tokens from the dashboard, matching the end-user workflow exactly. This ensures evals exercise the complete authentication flow.

### Node Profiles

Scenarios can specify `required_profile` to match with appropriate container configurations. Currently, a basic profile is provided. Additional profiles (e.g., nginx-broken, logs-seeded) can be added as needed.

### Approval Policy

The current implementation auto-approves all requests. Future enhancements may add policy hooks to simulate user approval behavior for privacy/safety scenarios.

## Scope

### In Scope

- End-to-end testing of the g8e platform against real operator containers
- Validation via deterministic matchers, LLM judges, and Sentinel assertions
- Fleet management and orchestration on the Docker host
- Report generation and metrics collection

### Out of Scope

- Changes to the operator binary, g8ed, g8ee, or g8es (bugs found during evals are addressed in separate PRs)
- Auto-minting of device tokens (user action by design)
- New internal auth surfaces (must use public APIs)
- Pytest-based accuracy evals (non-operator) remain under `tests/evals/`
