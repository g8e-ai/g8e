# Unified Docker Stack Guide

Last Updated: 2026-09-16  
Version: v2.1.8

This guide explains how to run the g8e platform from the repository root as one Docker Compose stack: Gateway, Data Operator, Inference Operator, ensemble (g8ee), and dashboard (g8ed). It also documents the evaluation campaign topology used for governed model scoring, the remote Ollama provider boundary, and the provider-boundary Observer Operator that enrolls from the Windows Ollama host.

Run all commands from the repository root unless noted otherwise.

## Prerequisites

- Docker Engine with the Docker Compose v2 plugin.
- Built `./g8e` binary (`make build`).
- Ports available on the campaign host (defaults): **8080**, **8443**, **8000**, **3000**, **8081**, **8082**, **5173**.
- Repository-root `.env` (copy from `.env.example`).
- `G8E_OLLAMA_ENDPOINT` set to the **approved remote Ollama provider** (not loopback on the campaign host when Ollama runs elsewhere). Compose fails fast when this variable is unset because the evaluation profile's Inference Operator interpolates it.
- Remote Ollama reachable from the Docker network, for example: `curl -fsS http://192.168.1.2:11434/api/version`.

For interactive owner enrollment you need a browser with WebAuthn support. For headless campaign operation use `./g8e auth enroll user --headless -e localhost`.

After source changes, rebuild images before restarting containers:

```bash
make build
./g8e docker build
```

## Compose profiles and services

The root `docker-compose.yml` defines platform services on the `g8e-net` bridge network. Profiles control which containers start; they do not bypass owner enrollment.

| Service | Profile | Published ports | Role |
| --- | --- | --- | --- |
| `g8e-gateway` | default | 8080 HTTP, 8443 HTTPS | Policy Decision Point (PDP). PKI, governance, pub/sub, console, MCP, A2A. |
| `g8e-operator` | `bootstrapped` | none | **Data Operator** — governed tool/filesystem/process boundary. |
| `g8e-inference-operator` | `evaluation` | none | **Inference Operator** — governed inference to the remote Ollama provider. Requires `G8E_OLLAMA_ENDPOINT` and campaign registry bindings. |
| `ensemble` | `bootstrapped` | 8000 | g8ee chat pipeline (`POST /api/v1/chat`). |
| `dashboard` | `bootstrapped` | 3000 | Legacy dashboard (not the evaluation acceptance UI). |
| `g8e-eval-observer` | `evaluation` | none | Short-lived networkless target observer for the native execution-boundary suite only. Not the provider-boundary Observer Operator. |

The Gateway and Operator containers use the same Go image. The evaluation acceptance topology requires **both** `bootstrapped` and `evaluation` profiles. The legacy `g8ellama` profile (separate User Gateway) is **not** used for model campaigns.

### Evaluation campaign topology

Scored assignments use one campaign Gateway with two distinct remote Operator sessions:

```text
Campaign host (Linux + Docker)
  g8e-gateway ........................ PDP, pub/sub, inference dispatch fan-out
  g8e-operator ......................... Data Operator (governed tools)
  g8e-inference-operator ............. Inference Operator → remote Ollama
  ensemble ........................... g8ee ChatPipelineService

Provider host (Windows + Ollama)
  Ollama ............................. approved provider (192.168.1.2:11434)
  g8e operator (Observer) ............ provider-boundary hardware observer
                                       (--provider-boundary-observer-enabled)
```

- Scored inference never calls Ollama directly from the campaign host CLI or ensemble.
- The Inference Operator is the only scored path to the provider.
- The Observer Operator has **no** inference backend and **no** access to Inference Operator attempt files. It samples GPU/RAM locally and receives BEGIN/FINALIZE commands over Gateway pub/sub.

## Campaign and run naming

Keep internal plan vocabulary separate from public campaign branding.

| Purpose | Campaign ID | Run ID pattern | Model inventory | Cells (3 roles × 25 scenarios) |
| --- | --- | --- | --- | --- |
| **Mini smoke** (pipeline validation) | `eval-smoke-mini` | `smoke-mini-<unix>` | `.local.dev/smoke-mini-inventory.json` | 3 models → **225** |
| **Dev full smoke** (private, all frozen models) | `phase1a-smoke` | `smoke-dev-<unix>` | `.local.dev/north-star-inventory.json` | 35 models → **2625** |
| **First public homogeneous run** | `eval-genesis-homogeneous` | `genesis-homogeneous-01` (or `-<seq>`) | fresh provider freeze at launch | all discovered models |

Rules:

- **Do not** use `north-star` in public run IDs or campaign IDs. *North Star* remains the internal scenario catalog name (`north-star-25@1.0.0`).
- Use **Genesis** for the first public homogeneous release (`eval-genesis-homogeneous`).
- Every cold start gets a **new run ID**. Never resume abandoned runs after a volume wipe.
- Set `G8E_INFERENCE_CAMPAIGN_ID` and `G8E_INFERENCE_MODEL_REGISTRY_DIGEST` in `.env` **before** starting `g8e-inference-operator`. Without them, dispatch returns HTTP 403 / `campaign binding invalid`.

### Mini smoke inventory (current)

Generated from the full freeze; three models spanning lite/mid/large:

| Model | Role in smoke |
| --- | --- |
| `qwen3:0.6b` | smallest |
| `qwen3:4b` | mid |
| `gemma3:4b` | larger |

```bash
# Regenerate after a full inventory re-freeze:
go run ./.local.dev/tools/gen-smoke-mini-inventory
```

Current bindings (2026-09-16):

- Campaign ID: `eval-smoke-mini`
- Registry digest: `ce4ce367289752cb39f5b34c431696ea5fa685825ac4392c493fc564e0be8854`
- Matrix size: **225** assignments

## Environment configuration

Copy `.env.example` to `.env` and set at minimum:

```bash
G8E_OLLAMA_ENDPOINT=http://192.168.1.2:11434

# Mini smoke (change before starting inference operator):
G8E_INFERENCE_CAMPAIGN_ID=eval-smoke-mini
G8E_INFERENCE_MODEL_REGISTRY_DIGEST=ce4ce367289752cb39f5b34c431696ea5fa685825ac4392c493fc564e0be8854
```

For the dev full matrix, use `phase1a-smoke` and digest `bf99592643c56dc78a10c0ee5de59cf4d740fae0684cda6f9f9e40363959c50c` instead.

| Variable | Default | Effect |
| --- | --- | --- |
| `G8E_PREFIX` | `g8e` | Container name prefix |
| `G8E_HTTP_PORT` | `8080` | Gateway discovery / enrollment HTTP |
| `G8E_HTTPS_PORT` | `8443` | Gateway mTLS API and pub/sub |
| `G8E_ENSEMBLE_PORT` | `8000` | Ensemble API |
| `G8E_DASHBOARD_PORT` | `3000` | Dashboard |
| `G8E_HOSTNAME` | `localhost` | Browser-visible gateway hostname (CORS, WebAuthn) |
| `G8E_OLLAMA_ENDPOINT` | — | Remote Ollama URL for Inference Operator |
| `G8E_INFERENCE_CAMPAIGN_ID` | — | Frozen campaign ID bound at Inference Operator enrollment |
| `G8E_INFERENCE_MODEL_REGISTRY_DIGEST` | — | SHA-256 of frozen model registry |

## Standard bootstrap workflow

### 1. Start the Gateway

```bash
docker compose up -d
until curl -fsS http://127.0.0.1:8080/api/v1/health >/dev/null; do sleep 2; done
```

### 2. Enroll the owner

Interactive (console access):

```bash
./g8e auth enroll user -e localhost
```

Headless (CLI-only owner):

```bash
./g8e auth enroll user --headless -e localhost
```

### 3. Start evaluation workloads

Ensure `.env` campaign bindings match the inventory you will schedule **before** this step.

```bash
docker compose --profile bootstrapped --profile evaluation up -d
```

Wait ~5s, then list pending enrollments:

```bash
./g8e auth pending-platform-enrollments
```

Approve in this order (Data Operator first):

```bash
./g8e auth approve-platform-enrollment <data-operator-request-id> --yes
./g8e auth approve-platform-enrollment <dashboard-request-id> --yes
./g8e auth approve-platform-enrollment <ensemble-request-id> --yes
./g8e auth approve-platform-enrollment <inference-operator-request-id> --yes
```

Identify requests by instance ID: `operator-<container-id>` is the **Data** Operator; `operator-inference-operator` is the **Inference** Operator.

### 4. Verify readiness

```bash
until curl -fsS http://127.0.0.1:8000/health >/dev/null; do sleep 3; done
./g8e operator list
./g8e eval inference status --json
```

Expect **two** remote Operators (data + inference) plus one embedded Gateway operator, and an active inference session ID.

Session IDs change on every volume wipe. Rediscover them after any `docker compose down -v` or `./g8e docker clean`.

### Automated alternative

```bash
./g8e docker init
```

`g8e docker init` runs the full bootstrap in one command: prepare the host `.g8e` tree, build images, start the gateway, enroll the CLI owner, start `bootstrapped` + `evaluation` workloads, auto-approve platform enrollments in order (data operator → dashboard → ensemble → inference operator), and wait for ensemble health. Requires a repository-root `.env` with `G8E_OLLAMA_ENDPOINT`, `G8E_INFERENCE_CAMPAIGN_ID`, and `G8E_INFERENCE_MODEL_REGISTRY_DIGEST` set before running.

If a prior Docker start created `.g8e` as root, fix ownership once with `sudo chown -R $(id -u):$(id -g) .g8e` and rerun init.

Useful flags:

- `--clean` — wipe containers/volumes/networks before init (cold start).
- `--skip-build` — reuse existing images.
- `--skip-enroll` — reuse an already-enrolled CLI identity.
- `--skip-approvals` — start workloads without auto-approving enrollments.
- `--headless` — mTLS-only owner enrollment without the browser passkey ceremony (default runs passkey enrollment).

For a gateway-only automated start with interactive enrollment prompts, use:

```bash
./g8e docker start --profile bootstrapped --profile evaluation --full
```

## Provider-boundary Observer Operator (Windows Ollama host)

Deploy this **on the machine that runs Ollama** (for example `192.168.1.2`), not on the Linux campaign host.

### Prerequisites on the provider host

- `g8e.exe` built for Windows (`make build-windows` or copy `bin/g8e-windows-amd64.exe`).
- Outbound TCP to the campaign Gateway on **8080** and **8443**.
- `nvidia-smi` on PATH (GPU telemetry). Host RAM via Windows-equivalent collection in the observer collector.
- A hosts-file mapping so the Gateway certificate SAN matches, for example:

```text
192.168.1.10 g8e.local
```

Replace `192.168.1.10` with the Linux campaign host's LAN address.

### Start and enroll the Observer

In a dedicated working directory on the Windows provider host:

```powershell
.\g8e.exe operator start `
  --endpoint g8e.local `
  --provider-boundary-observer-enabled `
  --provider-boundary-observer-id g8e-provider-boundary-observer
```

The process submits a platform enrollment request. **Do not** pass `--inference-enabled`; this Operator is read-only hardware observation only.

From the campaign host owner CLI:

```bash
./g8e auth pending-platform-enrollments
./g8e auth approve-platform-enrollment <observer-request-id> --yes
```

After approval, confirm the observer appears in `./g8e operator list` with `provider_boundary_observer_enabled` in its runtime config.

### What the Observer does

1. Gateway sends `ProviderBoundaryObservationCommand` (BEGIN/FINALIZE) on the observer's pub/sub cmd channel when scored inference starts and ends.
2. Observer samples GPU VRAM, utilization, temperature, power, clocks, and system RAM between BEGIN and FINALIZE.
3. Observer publishes `ProviderBoundaryObservationCompleted` on its results channel.
4. Gateway ingests windows for `g8e eval campaign verify --require-provider-observation`.

The legacy filesystem runner `g8e eval provider-observer run` is for co-located dev tests only. Production uses the enrolled Observer Operator.

## Mini smoke campaign workflow

Use this to validate the full pipeline (schedule → execute → publish → explorer) in hours instead of days.

### Phase A — Reset public feed (cold start)

The gateway owns the public mirror (`8081` private ingest, `8082` public read/SSE) and the evaluation explorer (`5173`) when started with `--public-spectator` (default). Docker Compose enables this automatically.

```bash
./g8e eval mirror stop    # stops legacy daemon mirror only; gateway-owned listeners restart with gw
rm -rf .g8e/public-feed .g8e/public-mirror
./g8e public init --source-id opendevops-local --mirror-origin http://127.0.0.1:8081
docker compose up -d g8e-gateway    # or: ./g8e gw start -f --public-spectator
```

Explorer (acceptance UI): open `http://127.0.0.1:5173/#/` after the gateway is up. Build static assets once with `cd dashboard/g8e-adapter/evaluation-explorer && npm run build` if the explorer listener logs that dist is missing. Do **not** run `npm run dev:real` for North Star acceptance — that path is legacy local supervisor only.

### Phase B — Initialize and schedule

```bash
RUN_ID=smoke-mini-$(date +%s)
INFERENCE_SESSION=$(./g8e eval inference status --json | jq -r .operator_session_id)
DATA_SESSION=$(./g8e operator list --json | jq -r '.operators[] | select(.operator_type=="remote" and .inference_enabled!=true and .provider_boundary_observer_enabled!=true) | .operator_session_id' | head -1)

./g8e eval campaign init \
  --campaign-id eval-smoke-mini \
  --run-id "$RUN_ID" \
  --inventory-file .local.dev/smoke-mini-inventory.json \
  --inference-session "$INFERENCE_SESSION" \
  --data-session "$DATA_SESSION"

./g8e eval campaign schedule --run-id "$RUN_ID" --publish
```

Wait for schedule publish to finish (~1 min for 225 cells). **Do not** start execute until schedule exits successfully.

### Phase C — Execute (serial daemon)

```bash
G8E_OLLAMA_ENDPOINT=http://192.168.1.2:11434 \
  ./g8e eval campaign execute \
  --run-id "$RUN_ID" \
  --ollama-endpoint http://192.168.1.2:11434 \
  --publish --daemon --provider-settle 8s
```

- `--daemon` runs the full matrix in one process.
- `--provider-settle 8s` waits after each assignment for Ollama to go idle.
- **Never** run `g8e eval campaign publish` concurrently with `execute --publish`.

### Phase D — Monitor

| Surface | URL |
| --- | --- |
| Explorer | `http://127.0.0.1:5173/#/` |
| Live dataset | `ds-live-<run-id>` |
| Public mirror bootstrap | `http://127.0.0.1:8082/bootstrap?source=opendevops-local` |

```bash
./g8e eval campaign status --run-id "$RUN_ID"
./g8e eval campaign account --run-id "$RUN_ID" --json
cd dashboard/g8e-adapter/evaluation-explorer && npm run health
```

After the Observer Operator is enrolled, verify hardware coverage:

```bash
./g8e eval campaign verify --run-id "$RUN_ID" --require-provider-observation
```

### Hard rules

1. **Never** call Ollama at `127.0.0.1:11434` on the campaign host for scored work when the approved provider is remote.
2. **Always** rediscover Operator session IDs after a volume wipe.
3. **Always** match `.env` campaign/registry digest to the inventory file used at `campaign init`.
4. **Never** resume archived or abandoned run IDs from prior checkpoints.

## Public spectator feed

Campaign data publishes through Go (`CampaignPublicationCoordinator` → `PublicPublisherService` outbox → mirror ingest). No Python bridge or host systemd publisher.

**Gateway-owned (landed):** `g8e gw start --public-spectator` (default) and `docker compose up -d g8e-gateway` start the in-process mirror on `8081`/`8082` and evaluation explorer on `5173`.  
**Legacy fallback:** `./g8e eval mirror {run|stop|status}` for host-only dev without a running gateway.

Do not install `deploy/systemd/opendevops-eval-publisher.service` (deleted) or rely on `g8e public mirror run` as the long-term ops interface.

## CLI stack management

| Command | Behavior |
| --- | --- |
| `./g8e docker init` | Build images, enroll owner, start full evaluation stack, auto-approve platform enrollments, and wait for readiness. |
| `./g8e docker start` | Starts default profile (Gateway only). |
| `./g8e docker start --full` | Starts `bootstrapped` profile with enrollment walkthrough. |
| `./g8e docker start --profile bootstrapped --profile evaluation` | Starts full evaluation stack. |
| `./g8e docker stop` | `docker compose down` — preserves volumes. |
| `./g8e docker status` | `docker compose ps`. |
| `./g8e docker build` | Build all stack images. |
| `./g8e docker rebuild [--full]` | Stop, rebuild, restart selected profile. |
| `./g8e docker clean` | Destructive wipe of containers, volumes, networks. |

Destructive cleanup destroys the trust domain (PKI, owner, Operator identities, campaign state). After `./g8e docker clean`, repeat owner enrollment and platform approvals.

## Health checks and resources

| Service | Health check |
| --- | --- |
| `g8e-gateway` | HTTP `GET /api/v1/health` :8080 |
| `g8e-operator` | enrolled operator certificate present |
| `g8e-inference-operator` | enrolled operator certificate present |
| `ensemble` | HTTP `GET /health` :8000 (after enrollment completes) |
| `dashboard` | HTTP `GET /` :3000 (after enrollment completes) |

Workloads remain unhealthy while enrollment is pending.

| Service | CPU limit | Memory limit |
| --- | --- | --- |
| `g8e-gateway` | 2 | 1G |
| `g8e-operator` | 2 | 1G |
| `g8e-inference-operator` | 4 | 4G |
| `ensemble` | 2 | 2G |
| `dashboard` | 1 | 512M |

## Troubleshooting

### Workload stays unhealthy

```bash
./g8e auth pending-platform-enrollments
./g8e docker logs <service>
```

### Inference dispatch returns 403 / campaign binding invalid

Set `G8E_INFERENCE_CAMPAIGN_ID` and `G8E_INFERENCE_MODEL_REGISTRY_DIGEST` in `.env` to match the inventory used at `campaign init`, then recreate the inference operator container:

```bash
docker compose --profile evaluation up -d --force-recreate g8e-inference-operator
```

### Observer not receiving commands

- Confirm Observer enrolled with `--provider-boundary-observer-enabled` (not the filesystem `eval provider-observer run` path).
- Confirm Gateway can reach the Observer session (`./g8e operator list`).
- Confirm Windows host can reach Gateway ports 8080/8443 and `g8e.local` resolves to the campaign host.

### Public feed drift or out-of-order batches

- Stop execute and any concurrent `campaign publish`.
- Do not run `campaign publish` and `execute --publish` at the same time.
- Reset public feed (Phase A above) before a new run.

### Browser TLS or WebAuthn failures

Confirm `G8E_HOSTNAME` matches the browser URL, the gateway root CA is trusted, and HTTPS port 8443 is reachable.

## Stopping safely

```bash
# Stop execute: Ctrl-C or kill the execute daemon PID
./g8e eval mirror stop
./g8e docker stop
docker compose --profile bootstrapped --profile evaluation down -v   # destroys trust domain
./g8e docker clean
```

## Relationship to other stacks

- **Demos** (`demos/`, `./g8e demos`): organization-specific scenarios; no ensemble/dashboard.
- **g8ellama profile**: legacy separate User Gateway; not used for North Star / Genesis campaigns.
- **Native execution-boundary eval** (`g8e eval run core-execution-boundary`): platform lane only; not a model campaign.

## Related documentation

- [Build Operator](./build_operator.md) — build `g8e.exe` for the Windows Observer host.
- [Connect Operator to Gateway](./connect_operator_to_gateway.md) — enrollment protocol details.
- [Docker Gateway Guide](./docker_gateway.md) — standalone gateway operation.
- [g8ee Documentation](../ensemble/index.md) — ensemble configuration and providers.
- [g8ed Documentation](../dashboard/index.md) — dashboard development.
- [Authentication and Identity](../architecture/auth.md) — mTLS, WebAuthn, PKI.
- Implementation plan: `.local.dev/docs/plans/in-progress/2026-09-15-g8e-evals-north-star-implementation.md`
