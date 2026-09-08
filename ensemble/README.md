# g8ee: g8e Ensemble

g8ee is the first-party Python and FastAPI reasoning service for the g8e platform. It enrolls a workload identity with the gateway, connects to gateway-hosted operator services over mTLS, runs the agent reasoning flow, and submits `GovernanceEnvelope` transactions for validation and execution through L1 Doctrine, L2 Consensus, L3 Notary, L4 Warden, and L5 Actuator. See the [Ensemble documentation](../docs/ensemble/index.md) for architecture, configuration, governance, and operating details.

## Run the unified stack

The supported Docker Compose flow starts the gateway, operator, ensemble, and dashboard. It also enrolls the first owner and guides that owner through approval of each workload enrollment request.

### Prerequisites

- Docker Engine with the Docker Compose v2 plugin
- The repository-root `g8e` binary
- Host ports 8080, 8443, 8000, and 3000 available when using the defaults
- A browser with WebAuthn support, or a terminal that can complete headless owner enrollment

From the repository root, build the images and start the full stack:

```bash
./g8e docker build
./g8e docker start --full
```

The default Compose profile starts only the gateway. The `bootstrapped` profile contains the operator, ensemble, and dashboard, and those workloads remain pending until an enrolled owner approves them. The helper above manages that flow; see the [Unified Docker Stack guide](../docs/guides/unified_stack.md) for the manual commands and troubleshooting steps.

After enrollment completes, the ensemble health endpoint is available at `http://localhost:8000/health`. Set `G8E_ENSEMBLE_PORT` before starting Compose to publish a different host port.

## Development setup

### Prerequisites

- Python 3.12 or later, as specified by `requires-python` in `pyproject.toml`
- A running and enrolled g8e gateway and operator for application startup and live integration tests

The ensemble depends on the in-tree `g8e` Python protocol package. From the repository root:

```bash
cd ensemble
python3 -m venv .venv
source .venv/bin/activate
make setup
cp .env.example .env
make proto
```

`make setup` installs `../protocol/python` and the ensemble with its development and test dependencies in editable mode. `make proto` verifies that the canonical Python protobuf stubs generated from the protocol definitions are current.

Configure `.env` for the gateway, operator, and selected LLM provider. The ensemble reads `.env` without overriding variables already present in the process environment. See [LLM Providers](../docs/ensemble/llm-providers.md) for provider settings and [PKI and Trust](../docs/ensemble/pki.md) for workload identity and certificate requirements.

## Run locally

From `ensemble/` with the virtual environment active:

```bash
python -m app.main
```

The development entry point listens on HTTP at `0.0.0.0:8443` with reload enabled. Its outbound gateway and operator connections use mTLS. On startup, the ensemble loads or requests its app identity, connects the DB, KV, pub/sub, and blob transports, loads platform settings, and starts its domain services. A new identity remains pending until an enrolled owner approves the ensemble workload request.

Governed collection mutations require an operator-bound identity. The unified Compose stack mounts the operator credentials read-only and sets `G8E_GOVERNANCE_OPERATOR_CERT` and `G8E_GOVERNANCE_OPERATOR_KEY`; a local deployment must provide equivalent paths or governed submissions fail closed.

## Test and validate

Run the standard ensemble checks from the repository root:

```bash
make ensemble-test
make ensemble-lint
```

`make ensemble-test` runs Tier 1 unit tests and Tier 2 in-process integration tests without live LLM or external API calls. `make ensemble-lint` runs Ruff and Pyright against the application. Run `make ci-ensemble` to execute both checks.

Tier 4 tests use live LLM providers or external APIs and run separately:

```bash
make test-external
```

The evaluation harness is a standalone package under `ensemble/evals/` with its own locked environment. Run `make evals-test` and `make evals-lint` from the repository root. See [Testing](../docs/ensemble/tests.md) and [Evals](../docs/ensemble/evals.md) for test tiers, markers, credential gating, and eval commands.

## Project layout

- `app/`: FastAPI application, transport clients, typed models, LLM providers, agent services, security filters, storage adapters, and route handlers.
- `config/`: Command validation allowlist, blocklist, and auto-approval configuration.
- `tests/`: Unit, in-process integration, external, and end-to-end test suites, plus shared fakes and fixtures.
- `evals/`: Standalone evaluation package, benchmark datasets, receipt verification, and reports.
- `pyproject.toml`: Package metadata, dependencies, pytest settings, coverage settings, and Ruff configuration.
- `Dockerfile`: Multi-stage runtime image built with the repository root as its build context.
- `Makefile`: Ensemble-local setup, protobuf verification, formatting, linting, and test targets.

Canonical shared protocol models and constants live in `../protocol/`; broader ensemble documentation lives in `../docs/ensemble/`.

## Contributing

See the ensemble [contribution guide](CONTRIBUTING.md), the platform [contribution guide](../.github/CONTRIBUTING.md), the [ensemble development guide](../docs/ensemble/devs.md), and the repository [documentation guide](../docs/devs/docs.md).

## Changelog

See the ensemble [changelog](CHANGELOG.md) and the platform [release notes](../docs/release_notes/).

## License

The ensemble is licensed under the Business Source License 1.1. It converts to Apache License 2.0 on 2030-08-18. See the ensemble [license](LICENSE) and the repository-root [license](../LICENSE).
