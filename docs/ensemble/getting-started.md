---
doc_id: getting_started_ensemble
title: Getting Started with g8ee
audience: developers implementing or deploying the ensemble
status: current
last_updated: 2026-10-06
version: v2.3.2
owners:
  - docs/ensemble/
  - ensemble/
  - docker-compose.yml
related:
  - docs/ensemble/architecture.md
  - docs/ensemble/governance.md
  - docs/ensemble/pki.md
  - docs/ensemble/llm-providers.md
  - docs/ensemble/devs.md
  - docs/ensemble/tests.md
  - docs/guides/unified_stack.md
when_to_read: Setting up local development, deploying the unified Docker stack, understanding ensemble bootstrap and enrollment, or verifying ensemble readiness.
do_not_use_for:
  - System architecture and trust boundaries (docs/ensemble/architecture.md)
  - Five-layer governance model (docs/ensemble/governance.md)
  - PKI, workload enrollment, and trust establishment (docs/ensemble/pki.md)
  - LLM provider configuration (docs/ensemble/llm-providers.md)
  - Component development commands (docs/ensemble/devs.md)
  - Test organization and CI scope (docs/ensemble/tests.md)
---

# Getting Started with g8ee

g8ee is the optional first-party Python 3.12 FastAPI application for conversational interaction with g8e. It owns triage, model reasoning, tool loops, application state, and event publication. g8ee runs outside the Gateway and Operator trust boundaries: model output, Tribunal agreement, application approvals, and memory do not authorize host or platform mutation. Host commands dispatch through the Gateway `POST /api/v1/operators/commands` endpoint with a registered request `event_type`; protected application-record writes use the Gateway governance endpoint. g8ee does not publish to Gateway pub/sub. The Gateway and executing Operator enforce the active five-layer policy.

The unified Docker Compose stack is the supported deployment path. Use the local development path when changing or testing the ensemble source.

## Purpose

This guide covers two ensemble paths: running the complete unified stack (recommended for first-time setup and testing) and local development against a running Gateway and Operator. Both paths include enrollment, readiness verification, and example health checks.

## Quick index

- [Purpose](#purpose)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

## Invariants

None specific to this guide. Refer to [PKI and Trust](pki.md) for workload enrollment invariants and [Testing](tests.md) for test execution scope.

## Owned surfaces

| Surface | Location | Verify |
| --- | --- | --- |
| Ensemble Docker configuration | `docker-compose.yml` services.ensemble | Port mapping, launch arguments, volume mounts |
| Ensemble startup | `ensemble/app/main.py` lifespan context manager | Bootstrap phases, service initialization order |
| Health endpoints | `ensemble/app/routers/health_router.py` | GET `/health`, `/health/live`, `/health/details` response schemas |
| Docker build | `ensemble/Dockerfile` | Multi-stage build, Python 3.12, runtime command |
| Make commands | `Makefile` targets and `ensemble/Makefile` | dev-python, ensemble-test, ensemble-lint, ensemble-test-external |

## Procedures

### Run the Unified Stack

#### Prerequisites

- Docker Engine with the Docker Compose v2 plugin.
- The repository-root `g8e` binary. Build it with `make build` if not present.
- Host ports `8080`, `8443`, `8000`, and `3000` available (or remap them with a checked-in `docker-compose.override.yml`).
- A repository-root `.env`, copied from `.env.example`, with `G8E_OLLAMA_ENDPOINT` set to the approved remote Ollama endpoint. Compose evaluates this required variable even when the evaluation profile is not selected.
- A browser with WebAuthn support for interactive owner enrollment, or a terminal for headless enrollment.

#### Startup

Run these commands from the repository root:

```bash
cp .env.example .env
# Edit .env and set G8E_OLLAMA_ENDPOINT to the approved remote Ollama URL.
./g8e docker build
./g8e docker start
```

`docker start` brings up the full unified stack — gateway, data operator, inference operator, and ensemble — enrolls the first CLI owner, and walks through platform enrollment approvals. Use `./g8e docker init` when you need the complete evaluation topology in one flow (image builds, automatic approvals, readiness checks, and the `g8e-eval` application identity); `docker init` requires the same `.env` setting and can use `--headless` for mTLS-only owner enrollment.

#### Enrollment

The ensemble submits its own platform enrollment request, generates an app key and CSR, and remains unavailable until an enrolled owner approves the request. The interactive walkthrough may finish before a request is visible. List pending requests and approve the ensemble manually when necessary:

```bash
./g8e auth enroll pending
./g8e auth enroll approve <ensemble-request-id> --yes
# or: ./g8e auth enroll deny <ensemble-request-id> --yes
```

When approving manually, approve the Data Operator before the ensemble. The ensemble uses its own enrolled app certificate for Gateway-backed DB, KV, blob, and HTTP services. Host-command execution dispatches to the exact enrolled Operator session through Gateway HTTP; it is not executed inside the ensemble container or on the Docker host.

#### Readiness

Check readiness and the public health endpoints:

```bash
./g8e docker status
curl -fsS http://localhost:8000/health
curl -fsS http://localhost:8000/health/live
curl -fsS http://localhost:8000/health/details
```

Responses:
- `/health` returns `{"status":"ok"}` and serves as the Compose healthcheck. No authentication required.
- `/health/live` returns `{"status":"alive","service":"g8ee"}`. No authentication required.
- `/health/details` returns `{"status":"ok","service":"g8ee","timestamp":"<iso-datetime>","clients":{...}}`. No authentication required. The `clients` object reports `document_service`, `internal_http_client`, `operator_command_service`, and `chat_pipeline` status.

The ensemble API listens on port 8000 inside the container. The published host port is the literal `8000:8000` in `docker-compose.yml`; change it with a checked-in `docker-compose.override.yml`. The override changes only the host-side port binding; the container always listens on `8000`. For manual profile commands, non-default ports, logs, workload revocation, and recovery, see [Unified Docker Stack](../guides/unified_stack.md).

#### Recovery

If the ensemble is waiting for approval, inspect and approve the request:

```bash
./g8e auth enroll pending
./g8e auth enroll approve <request-id> --yes
```

Enrollment state is resumable. Restarting the ensemble does not approve a pending request; it resumes an unexpired pending attempt or creates a new request when the previous attempt is no longer usable. Inspect startup failures:

```bash
./g8e docker logs ensemble
./g8e docker status
```

A fresh enrollment requires the Gateway to be healthy and the owner CLI to have valid credentials. Do not delete runtime volumes as a troubleshooting step unless you intend to destroy the component identity and repeat enrollment. The ensemble's certificate, key, trust bundle, and pending enrollment state are stored in the `g8e-ensemble-data` volume; the executing Operator owns authoritative execution receipts and audit evidence in its own volume.

### Set Up Local Development

#### Prerequisites

- Python 3.12 or later, as specified by `ensemble/pyproject.toml`.
- A running, enrolled Gateway and Operator reachable from the local process.
- An enrolled owner who can approve the ensemble workload request.
- A configured model provider and model before using chat. See [LLM Providers](llm-providers.md).

#### Bootstrap

The ensemble depends on the in-tree Python protocol package. From the repository root, `make dev-python` is the shortest path: it provisions the repository-root `.venv` with uv (which installs Python 3.12 when the system has none) and installs the editable `protocol/python` package and `ensemble[test]`. Both Makefiles prefer that `.venv`. To manage the environment by hand instead:

```bash
cd ensemble
python3 -m venv .venv
source .venv/bin/activate
make setup
cp .env.example .env
make proto
```

`make setup` installs the in-tree `protocol/python` package and ensemble development and test dependencies in editable mode. `make proto` verifies that generated Python protobuf bindings match the canonical protocol definitions; it does not generate new bindings.

#### Configuration

Platform configuration is passed as launch arguments, not environment variables (INV-ENV-04). The defaults target a local Gateway (`http://localhost:8080`, `https://localhost:8443`), so a local run needs no arguments. To point at a Gateway elsewhere, pass the arguments to `python -m app.serve`:

- `--gateway-http-url`: Plain-HTTP enrollment and discovery surface. Defaults to `http://localhost:8080`.
- `--gateway-https-url` and `--gateway-pubsub-url`: Gateway-hosted HTTPS and WebSocket services used by ensemble transport clients.
- `--gateway-url`: HTTPS base URL used by the internal HTTP client for Gateway event and operator-link operations.
- `--runtime-dir`, `--pki-dir`, `--secrets-dir`, `--ca-cert-path`: Runtime, PKI, bootstrap-secrets, and trust-bundle locations. Default to `.g8e` in the project root and its `pki` and `secrets` subdirectories.

`.env` holds only secrets and user-specific endpoints (LLM API keys and endpoints); it is loaded without replacing variables already present in the environment. Provider and model selection are Gateway-backed platform settings. The enrollment service obtains the Gateway CA bundle during enrollment and stores the app identity in the configured runtime tree. Do not put private keys, API keys, or copied operator credentials in documentation or source control. Governed application-record writes use the enrolled app certificate for transport and the configured Operator session binding as delegated authority; the Gateway validates both identities and applies the active posture. An application approval or mTLS fingerprint is not a substitute for required protocol L2 or L3 evidence.

#### Startup

Run the development server:

```bash
python -m app.serve
```

This is the same launcher the container uses: it installs the bootstrap settings from its arguments, then starts Uvicorn without TLS on `0.0.0.0:8000` (`--host` and `--port` override). `python -m app.main` remains the reload-enabled developer entry point; it runs Uvicorn without TLS on `0.0.0.0:8443`, matching the protocol HTTPS port constant but serving plain HTTP, and it takes no arguments. Do not run the local ensemble on the same host port as a Gateway TLS listener.

Verify it from another terminal:

```bash
curl -fsS http://localhost:8443/health
```

On first startup, the process submits an ensemble enrollment request and waits for owner approval. After approval, it loads platform settings through the Gateway, connects DB, KV, and blob transports, and starts domain services. Startup fails if identity enrollment, transport connection, or required platform settings cannot complete.

### Validate Changes

Run the standard ensemble checks from the repository root:

```bash
make ensemble-test
make ensemble-lint
```

`make ensemble-test` runs unit and in-process integration suites without live LLM or external API calls. `make ensemble-lint` runs Ruff and Pyright against `ensemble/app` and `ensemble/tests` (see [Python Linting](../devs/python-linting.md)). Tests requiring live providers or external APIs are separate and may require credentials:

```bash
make ensemble-test-external
```

See [Development](devs.md) for component commands and [Testing](tests.md) for test tiers and external-service gates.

## Anti-patterns

- Deleting runtime volumes (`g8e-ensemble-data`) as a first troubleshooting step. Deletion destroys component identity and requires re-enrollment.
- Assuming `G8E_G8EE_HTTPS_PORT` controls the development server port. The dev port is set to the protocol constant (8443) and determined by platform settings loaded from the Gateway, not an environment variable.
- Running the local ensemble on the same host port as a Gateway TLS listener (e.g., both on 8443).
- Putting private keys, certificates, or API credentials in `.env` files or source control. Store them securely and reference them via environment variables.

## Links out

- [Architecture](architecture.md): System topology, service boundaries, and request flow.
- [Governance](governance.md): Five-layer verification and governance postures.
- [PKI and Trust](pki.md): Workload enrollment, certificates, and trust establishment.
- [LLM Providers](llm-providers.md): Model roles and provider configuration.
- [Development](devs.md): Component development commands and environment setup.
- [Testing](tests.md): Ensemble test tiers and external-service gates.
- [Platform Getting Started](../guides/getting_started.md): Platform installation and native deployment.
- [Unified Docker Stack](../guides/unified_stack.md): Complete Compose workflow and operations.
- [Documentation Guide](../devs/docs.md): Documentation audit and ownership rules.
