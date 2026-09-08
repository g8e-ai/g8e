# Getting Started

The g8e Agentic Ensemble (`g8ee`) is the first-party reasoning service for the g8e platform. It enrolls its own workload identity with the Gateway, connects to Gateway-hosted Operator services over mTLS, and submits governed actions through the platform verification pipeline.

The unified Docker stack is the supported path for running the complete platform. Use the local development path when changing or testing the ensemble source.

## Run the Unified Stack

### Prerequisites

- Docker Engine with the Docker Compose v2 plugin
- The repository-root `g8e` binary
- Host ports `8080`, `8443`, `8000`, and `3000` available when using the defaults
- A browser with WebAuthn support, or a terminal for headless owner enrollment

Run these commands from the repository root:

```bash
./g8e docker build
./g8e docker start --full
```

The start command launches the Gateway, Operator, ensemble, and dashboard. On a fresh deployment, it enrolls the first owner and prompts for approval of each platform workload. The ensemble generates its own key and certificate signing request, then remains unavailable until an enrolled owner approves its request.

The automated walkthrough checks each workload once. If the ensemble request is not available when checked, list and approve it manually:

```bash
./g8e auth pending-platform-enrollments
./g8e auth approve-platform-enrollment <ensemble-request-id> --yes
```

Approve the Operator before the ensemble when both are pending. Governed submissions use the enrolled Operator identity, while other ensemble connections use the ensemble workload identity.

Check container readiness and the ensemble health endpoint:

```bash
./g8e docker status
curl -fsS http://localhost:8000/health
```

The ensemble API is available at `http://localhost:8000` after startup and enrollment are complete. Set `G8E_ENSEMBLE_PORT` before starting the stack to publish a different host port. See the [Unified Docker Stack](../guides/unified_stack.md) guide for manual enrollment, non-default ports, logs, and troubleshooting.

## Set Up Local Development

### Prerequisites

- Python 3.12 or later
- A running, enrolled Gateway and Operator
- An enrolled owner who can approve the ensemble workload request
- Operator platform settings available to the ensemble

The ensemble depends on the in-tree Python protocol package. From the repository root:

```bash
cd ensemble
python3 -m venv .venv
source .venv/bin/activate
make setup
cp .env.example .env
make proto
```

`make setup` installs the in-tree protocol package and the ensemble development and test dependencies in editable mode. `make proto` verifies that the generated Python protobuf bindings match the canonical protocol definitions.

Configure `.env` with the Gateway and Operator endpoints used by the local process. The ensemble loads `.env` without replacing variables already present in the process environment. At minimum, local startup requires access to the Gateway HTTP enrollment surface and the Gateway-hosted Operator HTTPS and pub/sub surfaces:

```dotenv
G8E_GATEWAY_HTTP_URL=http://localhost:8080
G8E_GATEWAY_URL=https://localhost:8443
G8E_OPERATOR_URL=https://localhost:8443
G8E_OPERATOR_PUBSUB_URL=wss://localhost:8443
G8E_OPERATOR_BLOB_URL=https://localhost:8443
```

Configure a model provider and model before using chat. Environment variables provide bootstrap defaults, while stored platform settings and request-specific values take precedence. See [LLM Providers](llm-providers.md) for the supported providers and exact configuration keys.

Governed collection mutations require an Operator-bound identity. Set `G8E_GOVERNANCE_OPERATOR_CERT` and `G8E_GOVERNANCE_OPERATOR_KEY` to the enrolled Operator certificate and key paths. Without both values, the ensemble starts with its app identity but governed submissions fail closed at the Gateway.

Start the development server from `ensemble/` with the virtual environment active:

```bash
python -m app.main
```

The development entry point listens on HTTP at `0.0.0.0:8443` with reload enabled. On first startup, the process submits an ensemble enrollment request and waits for approval. In another terminal, approve the request with the platform enrollment commands shown above, then verify the service:

```bash
curl -fsS http://localhost:8443/health
```

On startup, the ensemble loads or enrolls its app identity, connects the DB, KV, pub/sub, and blob transports, loads platform settings through the Operator, and starts its domain services. Startup fails if identity enrollment, transport connection, or required platform settings cannot complete.

## Validate Changes

Run the standard ensemble checks from the repository root:

```bash
make ensemble-test
make ensemble-lint
```

See [Development](devs.md) for additional development commands and [Testing](tests.md) for the test tiers and external-provider checks.

## Next Steps

- [Architecture](architecture.md): System topology, service boundaries, and request flow
- [Governance](governance.md): Five-layer verification and governance postures
- [PKI and Trust](pki.md): Workload enrollment, certificates, and trust establishment
- [LLM Providers](llm-providers.md): Model roles and provider configuration
- [Platform Getting Started](../guides/getting_started.md): Platform installation and native deployment
- [Unified Docker Stack](../guides/unified_stack.md): Complete Compose workflow and operations
