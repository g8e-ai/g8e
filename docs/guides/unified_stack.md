# Unified Docker Stack Guide

Last Updated: 2026-09-22  
Version: v2.1.12

This guide explains how to run the g8e platform from the repository root as one Docker Compose stack: Gateway, Data Operator, Inference Operator, ensemble (g8ee), and dashboard (g8ed). It also documents the evaluation campaign topology used for governed model scoring, the remote Ollama provider boundary, the provider-boundary **Observer Operator** (GPU/RAM witness), and the storage-side **Provenance Operator** (model weight attestation) that enroll from the provider host.

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
| `g8e-inference-operator` | `evaluation` | none | **Inference Operator** — governed inference to the remote Ollama provider. Requires `G8E_OLLAMA_ENDPOINT`; campaign authority travels on each governed dispatch. |
| `ensemble` | `bootstrapped` | 8000 | g8ee chat pipeline (`POST /api/v1/chat`). |
| `dashboard` | `bootstrapped` | 3000 | Legacy dashboard (not the evaluation acceptance UI). |

The Gateway and Operator containers use the same Go image. The evaluation acceptance topology requires **both** `bootstrapped` and `evaluation` profiles. The Observer Operator is a separately enrolled process on the remote provider host and is never a service in the unified Compose stack. The legacy `g8ellama` profile (separate User Gateway) is **not** used for model campaigns.

### Evaluation campaign topology

Scored assignments use one campaign Gateway with two remote Operator sessions on the campaign host plus one or two witness sessions on the provider host:

```text
Campaign host (Linux + Docker)
  g8e-gateway ........................ PDP, pub/sub, inference dispatch fan-out
  g8e-operator ......................... Data Operator (governed tools)
  g8e-inference-operator ............. Inference Operator → remote Ollama
  ensemble ........................... g8ee ChatPipelineService

Provider host (Windows + Ollama)
  Ollama ............................. approved provider (192.168.1.2:11434)
  ~/.ollama/models ................... content-addressed weight blobs
  g8e operator (Observer) ............ provider-boundary hardware observer
                                       (--provider-boundary-observer-enabled;
                                        read-only telemetry only)
  g8e operator (Provenance) .......... storage-side model weight attestor
                                       (--provenance-operator-enabled;
                                        --model-storage-root ~/.ollama/models)
```

- Scored inference never calls Ollama directly from the campaign host CLI or ensemble.
- The Inference Operator is the only scored path to the provider.
- The Observer Operator has **no** inference backend and **no** access to Inference Operator attempt files. It samples GPU/RAM locally and receives `ProviderBoundaryObservationCommand` BEGIN/FINALIZE over Gateway pub/sub.
- The Provenance Operator has **no** inference backend and **no** GPU sampling. It hashes Ollama manifests and weight blobs at `--model-storage-root` and receives `ModelProvenanceObservationCommand` BEGIN/FINALIZE in parallel with the Observer.
- Observer and Provenance may run on the **same physical host** but must enroll as **separate** governed operator sessions (separate terminals, separate `operator start` processes).
- The Observer Operator is read-only telemetry. It does not manage Ollama or receive generic command execution; campaign-owned model maintenance targets the exact Inference Operator session.

## Campaign and run naming

Keep internal plan vocabulary separate from public campaign branding.

| Purpose | Campaign ID | Run ID pattern | Model inventory | Cells (3 roles × 25 scenarios) |
| --- | --- | --- | --- | --- |
| **Init campaign** (one model, tidy pipeline gate) | `eval-init-<variant_id>` | `<campaign-id>-<unix>` | `.g8e/eval/inventories/<campaign-id>.json` | 1 model → **75** |
| **Mini smoke** (multi-model pipeline validation) | `eval-smoke-mini` | `smoke-mini-<unix>` | `.g8e/eval/inventories/eval-smoke-mini.json` | 3 models → **225** |
| **Full homogeneous run** | `eval-genesis-homogeneous` | `genesis-homogeneous-<seq>` | `.g8e/eval/model-inventory.json` (from `inventory freeze`) | all discovered models |

Rules:

- **Do not** use `north-star` in public run IDs or campaign IDs. The frozen scenario catalog is `north-star-25@1.0.0` (legacy slug; content is the standard 25-scenario suite).
- Use **Genesis** for the first public homogeneous release (`eval-genesis-homogeneous`).
- Every cold start gets a **new run ID**. Never resume abandoned runs after a volume wipe.
- Leave `G8E_INFERENCE_CAMPAIGN_ID` and `G8E_INFERENCE_MODEL_REGISTRY_DIGEST` **unset** in `.env`. Campaign authority travels on each governed dispatch from g8ee; do not rebind the inference operator per model.

### Init campaign inventory (one model per campaign)

Preferred for pipeline validation and model-by-model rollout: **one model, one campaign, 75 cells**. Keeps runs tidy and isolates failures. Use `g8e eval campaign start` (or `g8e eval rollout next` to inspect the next pending entry) — no `.env` edits or operator recreate between models.

Runtime data lives under `.g8e/eval/` (gitignored). See [eval/examples/README.md](../../eval/examples/README.md) for the public/private boundary.

```bash
# Freeze your provider's model registry (once per provider snapshot)
./g8e eval models freeze \
  --campaign-id eval-genesis-homogeneous \
  --output .g8e/eval/model-inventory.json

# Single model — materializes .g8e/eval/inventories/eval-init-<variant>.json automatically
./g8e eval campaign start --model qwen3:4b \
  --publish --daemon \
  --verify \
  --require-provider-observation \
  --require-model-provenance
```

Optional rollout queue (multi-model tracking):

```bash
# After inventory freeze, materialize per-model inventories and build the queue
./g8e eval rollout init --materialize --merge

# Unattended Tier-A rollout (replaces private batch shell scripts)
./g8e eval rollout run --tier-a --skip-verified --skip-variant granite3-3-2b

# Or one model at a time
./g8e eval rollout next
./g8e eval campaign start --queue next --publish --daemon --verify --tier-a
```

List variants or materialize subsets without a queue:

```bash
./g8e eval models list
./g8e eval models materialize --tag qwen3:4b
./g8e eval models materialize --all

# Mini smoke combined inventory (3 models → 225 cells)
./g8e eval models materialize --tags qwen3:0.6b,qwen3:4b,gemma3:4b \
  --campaign-id eval-smoke-mini \
  --output .g8e/eval/inventories/eval-smoke-mini.json
```

Track per-model verification progress in `.g8e/eval/init-campaign-queue.json` (`status: verified` or `pending`, plus `verified_run_id` when complete).

### Mini smoke inventory (current)

Build a three-model smoke inventory from your own provider freeze. Tags below are illustrative — digests must come from your Ollama host.

| Model | Role in smoke |
| --- | --- |
| `qwen3:0.6b` | smallest |
| `qwen3:4b` | mid |
| `gemma3:4b` | larger |

```bash
# Full provider freeze, then build a three-model smoke inventory:
./g8e eval models freeze \
  --campaign-id eval-genesis-homogeneous \
  --output .g8e/eval/model-inventory.json

./g8e eval models materialize --tags qwen3:0.6b,qwen3:4b,gemma3:4b \
  --campaign-id eval-smoke-mini \
  --output .g8e/eval/inventories/eval-smoke-mini.json
```

Matrix size for three models: **225** assignments (3 × 3 roles × 25 scenarios).

## Environment configuration

Copy `.env.example` to `.env` and set at minimum:

```bash
G8E_OLLAMA_ENDPOINT=http://192.168.1.2:11434
```

`g8e docker init` validates only `G8E_OLLAMA_ENDPOINT`. Campaign ID and registry digest are **not** `.env` concerns — `g8e eval campaign start` resolves them from the queue or `--model` flag and g8ee attaches them to each governed dispatch.

| Variable | Default | Effect |
| --- | --- | --- |
| `G8E_PREFIX` | `g8e` | Container name prefix |
| `G8E_HTTP_PORT` | `8080` | Gateway discovery / enrollment HTTP |
| `G8E_HTTPS_PORT` | `8443` | Gateway mTLS API and pub/sub |
| `G8E_ENSEMBLE_PORT` | `8000` | Ensemble API |
| `G8E_DASHBOARD_PORT` | `3000` | Dashboard |
| `G8E_HOSTNAME` | `localhost` | Browser-visible gateway hostname (CORS, WebAuthn) |
| `G8E_OLLAMA_ENDPOINT` | — | Remote Ollama URL for Inference Operator (required for `docker init`) |
| `G8E_INFERENCE_CAMPAIGN_ID` | *(unset)* | **Leave empty.** Legacy startup binding; per-model rollout uses dispatch-carried authority instead |
| `G8E_INFERENCE_MODEL_REGISTRY_DIGEST` | *(unset)* | **Leave empty.** Same as above |

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

```bash
docker compose --profile bootstrapped --profile evaluation up -d
```

Wait ~5s, then list pending enrollments:

```bash
./g8e auth enroll pending
```

Approve in this order (Data Operator first), or deny any request that should not be admitted:

```bash
./g8e auth enroll approve <data-operator-request-id> --yes
./g8e auth enroll approve <dashboard-request-id> --yes
./g8e auth enroll approve <ensemble-request-id> --yes
./g8e auth enroll approve <inference-operator-request-id> --yes
# ./g8e auth enroll deny <request-id> --yes
```

Identify requests by instance ID: `operator-<container-id>` is the **Data** Operator; `operator-inference-operator` is the **Inference** Operator.

### 4. Verify readiness

```bash
until curl -fsS http://127.0.0.1:8000/health >/dev/null; do sleep 3; done
./g8e operator list
./g8e eval gate inference status --json
```

Expect **two** remote Operators (data + inference) plus one embedded Gateway operator, and an active inference session ID.

Session IDs change on every volume wipe. Rediscover them after any `docker compose down -v` or `./g8e docker clean`.

### Stop or revoke an enrolled workload

Use the Operator session ID from `./g8e operator list` for a reversible process stop:

```bash
./g8e operator stop <operator-session-id> --reason "planned maintenance"
```

The command targets one active remote Operator owned by the authenticated user. It waits for the Operator's governed shutdown acknowledgement before the Gateway records `stopped`; the embedded Gateway Operator is never a valid target. The workload retains its certificate-backed enrollment and can start again later with its existing credentials.

Use the completed platform enrollment request ID for permanent identity revocation:

```bash
./g8e auth enroll revoke <request-id> --reason "host retired" --yes
```

Operator revocation invalidates the Operator and companion CLI certificates, deactivates their sessions, marks the Operator `terminated`, and disconnects established pub/sub connections. Dashboard or ensemble revocation invalidates the application certificate, removes its application policy, and disconnects established pub/sub connections. The workload must submit a new enrollment request and receive owner approval before it can authenticate again. Repeating the command for the same request is idempotent.

### Automated alternative

```bash
./g8e docker init
```

`g8e docker init` runs the full bootstrap in one command: prepare the host `.g8e` tree, build images, start the gateway, enroll the CLI owner, start `bootstrapped` + `evaluation` workloads, auto-approve platform enrollments in order (data operator → dashboard → ensemble → inference operator), and wait for ensemble health. Requires a repository-root `.env` with `G8E_OLLAMA_ENDPOINT` set before running.

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
./g8e auth enroll pending
./g8e auth enroll approve <observer-request-id> --yes
```

After approval, confirm the observer appears in `./g8e operator list` with `provider_boundary_observer_enabled: true` in its runtime config. The observer is a read-only witness and must not receive generic command execution.

### What the Observer does

1. Gateway sends `ProviderBoundaryObservationCommand` (BEGIN/FINALIZE) on the observer's pub/sub cmd channel when scored inference starts and ends.
2. Observer samples GPU VRAM, utilization, temperature, power, clocks, and system RAM between BEGIN and FINALIZE.
3. Observer publishes `ProviderBoundaryObservationCompleted` on its results channel.
4. Gateway ingests windows for `g8e eval campaign verify --require-provider-observation`.

The Observer only samples provider-boundary telemetry between BEGIN and FINALIZE. It does not manage Ollama, restart the daemon, unload models, or execute generic commands. Model residency is owned by Ollama, and campaign model release uses an approved command dispatched to the exact Inference Operator after scored work completes.

The legacy filesystem runner `g8e eval dev provider-observer run` is for co-located dev tests only. Production uses the enrolled Observer Operator.

**Timing rule:** Assignments that reached a terminal state before the Observer Operator was enrolled and pub/sub-connected will fail `--require-provider-observation`. That is expected. Enroll the observer before `execute`, or accept that early assignments lack hardware windows.

For example Observer Operator console output and a healthy-output checklist, see [Evaluations — Provider-boundary Observer Operator](../architecture/evals.md#provider-boundary-observer-operator).

## Storage-side Provenance Operator

Deploy this **at the model storage site** — the directory that holds Ollama manifests and blobs. On a typical Ollama host this is the same machine as the Observer; use a **second** enrolled operator session.

### Prerequisites

- Same network and enrollment prerequisites as the Observer Operator (outbound TCP to Gateway **8080**/**8443**, `g8e.local` hosts mapping).
- Read access to the Ollama models directory (for example `C:\Users\<you>\.ollama\models` on Windows or `~/.ollama/models` on Linux).
- The frozen campaign model digest is carried on each governed dispatch; the Provenance Operator compares its locally computed manifest digest to that expected value.

### Start and enroll the Provenance Operator

In a **second** dedicated working directory on the provider host (separate from the Observer session):

```powershell
.\g8e.exe operator start `
  --endpoint g8e.local `
  --provenance-operator-enabled `
  --provenance-operator-id g8e-model-provenance-operator `
  --model-storage-root "$env:USERPROFILE\.ollama\models"
```

**Do not** pass `--inference-enabled` or `--provider-boundary-observer-enabled` on this session unless you intend a separate combined deployment; production uses distinct sessions per witness role.

From the campaign host owner CLI:

```bash
./g8e auth enroll pending
./g8e auth enroll approve <provenance-request-id> --yes
```

After approval, confirm the provenance operator appears in `./g8e operator list` with `provenance_operator_enabled: true` and the correct `provenance_operator_model_storage_root`.

### What the Provenance Operator does

1. Gateway sends `ModelProvenanceObservationCommand` (BEGIN/FINALIZE) when scored inference starts and ends, carrying `served_model_tag` and `expected_model_digest` from the frozen campaign registry.
2. On FINALIZE, the operator hashes Ollama manifest and blob files under `--model-storage-root` and fails closed when the observed digest does not match the expected campaign digest.
3. The operator publishes `ModelProvenanceObservationCompleted` on its results channel.
4. Gateway ingests attestation windows under `data/inference/model-provenance/windows/`.

**Timing rule:** Assignments that completed before the Provenance Operator was enrolled lack attestation windows. Enroll before `execute` when chain-of-custody claims are required.

For architecture detail and example console output, see [Evaluations — Storage-side Provenance Operator](../architecture/evals.md#storage-side-provenance-operator) and [Model Provenance](../architecture/model-provenance.md).

## Mini smoke campaign workflow

Use this to validate the full pipeline (schedule → execute → publish → explorer) in hours instead of days.

### Phase A — Reset public feed (cold start)

The gateway owns the public feed, mirror (`8081` private ingest, `8082` public read/SSE), and evaluation explorer (`5173`) in the `g8e-gateway-data` volume when started with `--public-spectator` (default). Docker Compose enables this automatically. Campaign `schedule --publish` and `execute --publish` post signed batches through the gateway API (`POST /api/v1/public-feed/batches`).

```bash
docker compose up -d g8e-gateway    # or: ./g8e gw start -f --public-spectator
# To wipe spectator state: docker compose down -v && docker compose up -d g8e-gateway
```

Explorer (acceptance UI): open `http://127.0.0.1:5173/#/` after the gateway is up. Build static assets once with `cd dashboard/g8e-adapter/evaluation-explorer && npm run build` if the explorer listener logs that dist is missing. Do **not** run `npm run dev:real` for campaign acceptance — that path is legacy local supervisor only.

### Phase B — Initialize and schedule

```bash
RUN_ID=smoke-mini-$(date +%s)
INFERENCE_SESSION=$(./g8e eval gate inference status --json | jq -r .operator_session_id)
DATA_SESSION=$(./g8e operator list --json | jq -r '.operators[] | select(.operator_type=="remote" and .inference_enabled!=true and .provider_boundary_observer_enabled!=true and .provenance_operator_enabled!=true) | .operator_session_id' | head -1)

./g8e eval campaign init \
  --campaign-id eval-smoke-mini \
  --run-id "$RUN_ID" \
  --inventory-file .g8e/eval/inventories/eval-smoke-mini.json \
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
  --publish --daemon
```

- `--daemon` runs the full matrix in one process.
- Consecutive assignments keep the provider daemon running; no reset or stable-body `/api/ps` wait occurs between cells.
- After the queue is exhausted, the controller reads typed `/api/ps` residency and dispatches the image-baked `/g8e operator model release <served-tag>` command through the exact Inference Operator. The governed command uses the existing `OllamaBackend` HTTP client and the Operator's approved endpoint to issue `/api/generate` with `keep_alive: 0`, then the controller confirms the campaign-owned tags are absent. No external Ollama CLI is required.
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
3. **Always** use `g8e eval campaign start` (or ensure dispatch-carried campaign authority matches the inventory) — do not rebind `.env` per model.
4. **Never** resume archived or abandoned run IDs from prior checkpoints.

## Public spectator feed

Campaign data publishes through Go (`CampaignPublicationCoordinator` → `PublicPublisherService` outbox → mirror ingest). No Python bridge or host systemd publisher.

`g8e gw start --public-spectator` (default) and `docker compose up -d g8e-gateway` start the in-process mirror on `8081`/`8082` and evaluation explorer on `5173`. Do not install `deploy/systemd/opendevops-eval-publisher.service` (deleted).

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
./g8e auth enroll pending
./g8e docker logs <service>
```

### Inference dispatch returns 403 / campaign binding invalid

Usually means the inference operator was started with a **stale startup campaign binding** (`G8E_INFERENCE_CAMPAIGN_ID` / `G8E_INFERENCE_MODEL_REGISTRY_DIGEST` set in `.env`) that no longer matches the run. Clear those keys in `.env`, recreate the operator, and use `g8e eval campaign start` so g8ee carries campaign authority on each dispatch:

```bash
# .env: only G8E_OLLAMA_ENDPOINT required; leave campaign keys unset
docker compose --profile evaluation up -d --force-recreate g8e-inference-operator
./g8e eval campaign start --queue next --publish --daemon
```

### Observer not receiving commands

- Confirm Observer enrolled with `--provider-boundary-observer-enabled` (not the filesystem `eval dev provider-observer run` path).
- Confirm Gateway can reach the Observer session (`./g8e operator list`).
- Confirm Windows host can reach Gateway ports 8080/8443 and `g8e.local` resolves to the campaign host.

### Provenance operator not attesting or digest mismatch

- Confirm a **separate** session enrolled with `--provenance-operator-enabled` and `--model-storage-root` pointing at the live Ollama models directory.
- Confirm `./g8e operator list --json` shows `provenance_operator_enabled: true` and the expected storage root.
- Confirm the served model tag and digest in campaign inventory match what Ollama reports (`ollama show <tag> --verbose` or `/api/tags` on the provider).
- Digest mismatch on FINALIZE is intentional fail-closed behavior when weights changed after campaign freeze.

### Campaign model release fails

- Confirm the exact Inference Operator session is active and its configured provider endpoint matches `G8E_OLLAMA_ENDPOINT`.
- Inspect typed provider residency at `/api/ps`; malformed responses, endpoint failures, and ambiguous residency fail closed.
- Do not grant the Observer or Provenance Operator command authority. They are read-only witness boundaries; retry release only after the provider owner resolves the endpoint or Ollama client failure.

### Public feed drift or out-of-order batches

- Stop execute and any concurrent `campaign publish`.
- Do not run `campaign publish` and `execute --publish` at the same time.
- Reset public feed (Phase A above) before a new run.

### Mirror empty after `docker init --clean` but host run artifacts remain

`docker init --clean` wipes the gateway mirror volume only. Host campaign evidence under `.g8e/data/eval/runs/<run-id>/` and the rollout queue under `.g8e/eval/` are unchanged.

Restore every verified queue entry to the gateway-owned mirror:

```bash
./g8e eval campaign mirror restore --queue
```

Or one run:

```bash
./g8e eval campaign mirror restore --run-id <run-id>
```

`docker init` attempts `--queue` restore automatically when the queue and run artifacts exist. Queue entries whose `verified_run_id` directory is missing are reported as host-absent and must be re-executed — mirror restore cannot recreate inference evidence.

A plain catch-up publish now probes the gateway-owned dataset. When the dataset is absent while host `public-projection-state.json` under `.g8e/data/eval/runs/<run-id>/` still lists `published_idempotency_keys`, it clears that stale state and republishes the canonical lifecycle, result, and aggregate projections without editing JSON manually.

```bash
./g8e eval campaign publish --run-id <run-id>
```

Use `--force` only when the mirror was wiped or is known to be missing records and the drift-aware path is not sufficient. It clears host idempotency keys and republishes the run; mirror restore cannot recreate missing inference evidence or make an inapplicable verification report valid.

### Browser TLS or WebAuthn failures

Confirm `G8E_HOSTNAME` matches the browser URL, the gateway root CA is trusted, and HTTPS port 8443 is reachable.

## Stopping safely

```bash
# Stop execute: Ctrl-C or kill the execute daemon PID
docker compose restart g8e-gateway
./g8e docker stop
docker compose --profile bootstrapped --profile evaluation down -v   # destroys trust domain
./g8e docker clean
```

## Relationship to other stacks

- **Demos** (`demos/`, `./g8e demos`): organization-specific scenarios; no ensemble/dashboard.
- **g8ellama profile**: legacy separate User Gateway; not used for Genesis campaigns.
- **Native execution-boundary eval** (`g8e eval boundary run`): platform lane only; not a model campaign.

## Related documentation

- [Evaluations](../architecture/evals.md) — platform evaluation programs, Observer and Provenance Operator roles, evidence, and verification.
- [Model Provenance](../architecture/model-provenance.md) — zero-trust weight attestation and chain of custody.
- [Build Operator](./build_operator.md) — build `g8e.exe` for the Windows Observer host.
- [Connect Operator to Gateway](./connect_operator_to_gateway.md) — enrollment protocol details.
- [Docker Gateway Guide](./docker_gateway.md) — standalone gateway operation.
- [g8ee Documentation](../ensemble/index.md) — ensemble configuration and providers.
- [g8ed Documentation](../dashboard/index.md) — dashboard development.
- [Authentication and Identity](../architecture/auth.md) — mTLS, WebAuthn, PKI.
- [Evaluation data layout](../../eval/examples/README.md) — public vs runtime vs private operator data.
