---
title: Getting Started
parent: g8ee
---

# Getting Started

Last Updated: 2026-09-25
Version: v2.1.14

g8ee is the optional first-party Python 3.12/FastAPI application for conversational interaction with g8e. It owns triage, model reasoning, tool loops, application state, and event publication. It is outside the Gateway and Operator trust boundaries: model output, Tribunal agreement, application approvals, and memory do not authorize host or platform mutation. Host commands dispatch through the Gateway `POST /api/v1/operators/commands` endpoint with a registered request `event_type`; protected application-record writes use the Gateway governance endpoint. g8ee does not publish to Gateway pub/sub. The Gateway and executing Operator still enforce the active five-layer policy.

The unified Docker Compose stack is the supported deployment path. Use the local development path when changing or testing the ensemble source.

## Run the Unified Stack

### Prerequisites

- Docker Engine with the Docker Compose v2 plugin.
- The repository-root `g8e` binary. Build it with `make build` if it is not present.
- Host ports `8080`, `8443`, `8000`, and `3000` available when using the defaults.
- A repository-root `.env`, copied from `.env.example`, with `G8E_OLLAMA_ENDPOINT` set to the approved remote Ollama endpoint. Compose evaluates this required variable even when the evaluation profile is not selected.
- A browser with WebAuthn support for interactive owner enrollment, or a terminal for headless enrollment.

Run these commands from the repository root:

```bash
cp .env.example .env
# Edit .env and set G8E_OLLAMA_ENDPOINT to the approved remote Ollama URL.
./g8e docker build
./g8e docker start --full
```

`docker start --full` starts the Gateway and the `bootstrapped` workloads, enrolls the first CLI owner, and walks through platform enrollment approvals. It does not start the evaluation-profile inference Operator. Use `./g8e docker init` when you need the complete evaluation topology, automatic approvals, readiness checks, and the `evaluation` profile; `docker init` requires the same `.env` setting and can use `--headless` for mTLS-only owner enrollment.

The ensemble submits its own platform enrollment request, generates an app key and CSR, and remains unavailable until an enrolled owner approves the request. The interactive walkthrough may finish before a request is visible. List pending requests and approve the ensemble manually when necessary:

```bash
./g8e auth enroll pending
./g8e auth enroll approve <ensemble-request-id> --yes
# or: ./g8e auth enroll deny <ensemble-request-id> --yes
```

When approving manually, approve the Data Operator before the ensemble. The ensemble uses its own enrolled app certificate for Gateway-backed DB, KV, blob, and HTTP services. Host-command execution dispatches to the exact enrolled Operator session through Gateway HTTP; it is not executed inside the ensemble container or on the Docker host.

Check readiness and the public health endpoint:

```bash
./g8e docker status
curl -fsS http://localhost:8000/health
curl -fsS http://localhost:8000/health/live
```

The basic health response is `{"status":"ok"}`. `/health` is the Compose healthcheck and does not require application authentication. `/health/live` and `/health/details` are also public health routes; `/health/details` reports whether the ensemble's initialized services and transport clients are available.

The ensemble API is available at `http://localhost:8000` after startup and enrollment are complete. Set `G8E_ENSEMBLE_PORT` before starting Compose to publish a different host port. The container listens on port 8000; the variable changes only the host-side published port. For manual profile commands, non-default ports, logs, workload revocation, and recovery, see [Unified Docker Stack](../guides/unified_stack.md).

### Recovery

If the ensemble is waiting for approval, inspect the request and approve it with the authenticated owner CLI:

```bash
./g8e auth enroll pending
./g8e auth enroll approve <request-id> --yes
```

Enrollment state is resumable. Restarting the ensemble does not approve a pending request; it resumes an unexpired pending attempt or creates a new request when the previous attempt is no longer usable. Inspect startup failures with:

```bash
./g8e docker logs ensemble
./g8e docker status
```

A fresh enrollment requires the Gateway to be healthy and the owner CLI to have valid credentials. Do not delete runtime volumes as a troubleshooting step unless you intend to destroy the component identity and repeat enrollment. The ensemble's certificate, key, trust bundle, and pending enrollment state are stored in the `g8e-ensemble-data` volume; the executing Operator owns authoritative execution receipts and audit evidence in its own volume.

## Set Up Local Development

### Prerequisites

- Python 3.12 or later, as specified by `ensemble/pyproject.toml`.
- A running, enrolled Gateway and Operator reachable from the local process.
- An enrolled owner who can approve the ensemble workload request.
- A configured model provider and model before using chat. See [LLM Providers](llm-providers.md).

The ensemble depends on the in-tree Python protocol package. From the repository root:

```bash
cd ensemble
python3 -m venv .venv
source .venv/bin/activate
make setup
cp .env.example .env
make proto
```

`make setup` installs the in-tree `protocol/python` package and the ensemble development and test dependencies in editable mode. `make proto` checks that the generated Python protobuf bindings match the canonical protocol definitions; it does not generate new bindings.

Configure `.env` with the endpoints visible from the local process:

```dotenv
G8E_GATEWAY_HTTP_URL=http://localhost:8080
G8E_GATEWAY_URL=https://localhost:8443
G8E_OPERATOR_URL=https://localhost:8443
G8E_OPERATOR_PUBSUB_URL=wss://localhost:8443
```

`G8E_GATEWAY_HTTP_URL` is the plain-HTTP enrollment and discovery surface. If it is unset, enrollment derives it from `G8E_OPERATOR_URL` by changing `https` to `http` and port `8443` to `8080`; startup fails closed when neither value is available. `G8E_OPERATOR_URL` and `G8E_OPERATOR_PUBSUB_URL` identify the Gateway-hosted HTTPS and WebSocket services used by the ensemble transport clients. `G8E_GATEWAY_URL` is the HTTPS base URL used by the internal HTTP client for Gateway event and operator-link operations.

The process loads `.env` without replacing variables already present in its environment. The enrollment service obtains the Gateway CA bundle during enrollment and stores the resulting app identity in the configured runtime tree. Do not put private keys, API keys, or copied operator credentials in documentation or source control. Governed application-record writes use the enrolled app certificate for transport and the configured Operator session binding as delegated authority; the Gateway validates both identities and still applies the active posture. An application approval or mTLS fingerprint is not a substitute for required protocol L2 or L3 evidence.

The development entry point listens on HTTP at `0.0.0.0:8443` with reload enabled:

```bash
python -m app.main
```

Verify it from another terminal:

```bash
curl -fsS http://localhost:8443/health
```

Do not bind the local process to the same host port as a Gateway HTTPS listener. If the Gateway uses the default host port `8443`, run the local ensemble against a Gateway on another host or publish the Gateway HTTPS service on a different host port. On first startup, the process submits an ensemble enrollment request and waits for owner approval. After approval, it loads platform settings through the Gateway, connects the DB, KV, and blob transports, and starts its domain services. Startup fails if identity enrollment, transport connection, or required platform settings cannot complete.

## Validate Changes

Run the standard ensemble checks from the repository root:

```bash
make ensemble-test
make ensemble-lint
```

`make ensemble-test` runs the unit and in-process integration suites without live LLM or external API calls. `make ensemble-lint` runs Ruff and Pyright against `ensemble/app`. Tests requiring live providers or external APIs are separate and may require credentials:

```bash
make test-external
```

See [Development](devs.md) for additional component commands and [Testing](tests.md) for the ensemble test tiers and external-service gates.

## Next Steps

- [Architecture](architecture.md): System topology, service boundaries, and request flow
- [Governance](governance.md): Five-layer verification and governance postures
- [PKI and Trust](pki.md): Workload enrollment, certificates, and trust establishment
- [LLM Providers](llm-providers.md): Model roles and provider configuration
- [Platform Getting Started](../guides/getting_started.md): Platform installation and native deployment
- [Unified Docker Stack](../guides/unified_stack.md): Complete Compose workflow and operations
- [Documentation Guide](../devs/docs.md): Documentation audit and ownership rules
