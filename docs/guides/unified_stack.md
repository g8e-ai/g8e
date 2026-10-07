---
doc_id: unified_stack
title: Unified Docker Stack Guide
audience: platform operators and evaluators
status: current
last_updated: 2026-10-06
version: v2.3.2
owners:
  - docker-compose.yml
  - docs/guides/
related:
  - docs/guides/docker_gateway.md
  - docs/guides/build_operator.md
  - docs/guides/connect_operator_to_gateway.md
  - docs/architecture/evals.md
  - docs/architecture/model-provenance.md
  - docs/ensemble/index.md
  - docs/architecture/console.md
  - docs/architecture/auth.md
when_to_read: Running the g8e platform end-to-end in Docker Compose, bootstrapping evaluation campaigns with witness Operators, or troubleshooting stack health.
do_not_use_for:
  - CLI flag inventory — use ./g8e <command> --help
  - Evaluation execution details — see docs/architecture/evals.md
  - Authentication protocol details — see docs/architecture/auth.md
---

# Unified Docker Stack Guide

Explains how to run the g8e platform from the repository root as one Docker Compose stack: Gateway, Data Operator, Inference Operator, and ensemble (g8ee); the Gateway serves the browser console. Also documents the evaluation campaign topology used for governed model scoring, the remote Ollama provider boundary, the provider-boundary **Observer Operator** (GPU/RAM witness), and the storage-side **Provenance Operator** (model weight attestation) that enroll from the provider host.

Run all commands from the repository root unless noted otherwise.

## Purpose

This guide covers the unified evaluation stack — how to bootstrap it, enroll operator sessions, run campaigns, and observe execution. It assumes familiarity with Docker Compose and the g8e platform's governance model. For a quick start, see `./g8e docker init`. For standalone gateway operation without Docker, see [Docker Gateway Guide](docker_gateway.md).

## Quick index

- [Purpose](#purpose)
- [Quick index](#quick-index)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

## Invariants

| ID | Rule |
| --- | --- |
| INV-STACK-01 | The root docker-compose.yml defines the core unified stack (gateway, data operator, inference operator, ensemble) on the `g8e-net` bridge at subnet `172.28.0.0/16`. |
| INV-STACK-02 | Service health is determined by service-specific health checks defined in docker-compose.yml, not by simple container running state. |
| INV-STACK-03 | The Inference Operator must enroll with `--inference-enabled` before any scored campaign dispatch reaches the provider. |
| INV-STACK-04 | Observer and Provenance Operators enroll as separate sessions on the provider host and must not share the same session. |
| INV-STACK-05 | Campaign identity (campaign ID and registry digest) travels on each governed dispatch from ensemble. The Inference Operator has no startup campaign binding, so console chat and campaigns share it concurrently with nothing to switch. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Root stack service definitions | docker-compose.yml | `docker compose config` |
| Binary precedence and mounts | docker-compose.yml volumes, Dockerfile | Host `./bin:/opt/g8e/bin:ro` takes precedence |
| Service health checks | docker-compose.yml healthcheck blocks | Service-specific endpoints in [Health checks and resources](#health-checks-and-resources) |
| Environment variable schema | .env.example | `--help` for service flags and defaults |
| Profiles (cross-enrollment, g8ellama) | docker-compose.yml profiles section | `docker compose --profile <name> config` |

## Procedures

### Prerequisites

- Docker Engine with the Docker Compose v2 plugin. **Note: Local Go or `make` are NOT required on the host** — all builds can run inside Docker.
- Optional: Host Go toolchain and `make` (when developing locally and using `make build`).
- Ports available on the campaign host (defaults): **8080**, **8443**, **8000**, **8081**, **8082**, **5173**. The ensemble, mirror, and explorer ports are loopback-only in the root Compose file.
- Repository-root `.env` (copy from `.env.example`) with `G8E_OLLAMA_ENDPOINT` set.
- `G8E_OLLAMA_ENDPOINT` set to the **approved remote Ollama provider**. It has no default; Compose and `make full` fail when it is unset.
- Remote Ollama reachable from the Docker network, for example: `curl -fsS http://192.168.1.2:11434/api/version`.

For interactive owner enrollment you need a browser with WebAuthn support. For headless campaign operation use `docker compose exec g8e-gateway /g8e auth enroll user --headless -e localhost` or `./g8e auth enroll user --headless -e localhost`.

### Binary building, volume mounts, and version alignment

The platform leverages Docker volume mounts (`./bin:/opt/g8e/bin:ro`) so that a local `make build` is gospel for the entire platform without requiring Docker image rebuilds. At the same time, the host launching this is only required to have Docker — the Dockerfile uses the repository `Makefile` inside multi-stage container builds.

**Strict Order of Binary Precedence inside Containers:**
1. `G8E_BIN` explicit environment variable override (if set and executable).
2. Host-mounted binary `/opt/g8e/bin/g8e` (from local `make build` gospel).
3. Host-mounted architecture binary `/opt/g8e/bin/g8e-linux-${ARCH}` (from `make build-linux`).
4. Container image baked-in binary `/g8e` (built in container via `Makefile` during image build).

**Building and Aligning Operator Versions:**
- **Local build (Make is gospel):** Run `make build`. The host binary is written to `bin/g8e` and `./g8e`. Restart the operators to align all versions immediately:
  ```bash
  # Pure Docker:
  docker compose restart g8e-data-operator g8e-inference-operator

  # ./g8e docker CLI equivalent:
  ./g8e docker restart

  # make equivalent:
  make docker-restart-operators
  ```
- **Docker-only build (no host Make required):** Build images via Docker and export `./g8e`:
  ```bash
  # Pure Docker:
  docker compose build
  docker compose cp g8e-gateway:/g8e ./g8e && cp ./g8e bin/g8e

  # ./g8e docker CLI equivalent:
  ./g8e docker build

  # make equivalent:
  make docker-build
  ```

## Host lifecycle launcher (`make full`)

The host-native alternative to Compose runs the Gateway, the four Operator roles, and g8ee as local processes through `scripts/full.py`. The `make` targets never call Docker, and `make clean` does not reset runtime state (`g8e gw clean` does). All `make full*` targets depend on `make build`.

| Target | Behavior |
| --- | --- |
| `make full` | Unattended start: `g8e gw start --quiet` with the public URL, passkey RP ID/origin, and CORS origin derived from `G8E_HOSTNAME`, then the four Operators and g8ee. Requires `G8E_OLLAMA_ENDPOINT`. |
| `make full-setup` | Same start, but prompts for each role's system, working directory, model storage, and Ollama URL, and does not pass the hostname-derived Gateway flags. Requires interactive input. |
| `make full-reset` | `make full` with `--reset-identities`; `make full RESET_IDENTITIES=1` is equivalent. |
| `make status` (`full-status`) | Gateway, local Operator, and g8ee state. |
| `make ensemble-start`, `ensemble-stop`, `ensemble-restart`, `ensemble-status` (`g8ee-status`) | g8ee lifecycle only. |
| `make operators-stop`, `operators-restart`, `operators-status` | Local Operator lifecycle only. |
| `make down` (`stop`, `full-down`, `full-stop`) | Stops g8ee, local Operators, and the Gateway. |

`FULL_ARGS` passes extra launcher flags, for example `make full FULL_ARGS=--dry-run`. `--dry-run` prints the Gateway and Operator commands, working directories, and remote-host commands without starting processes or writing runtime state. `scripts/full.py --help` lists every flag, including `--env-file`, `--model-storage-root`, `--<role>-working-dir`, and `--keep-gateway` (leave the Gateway running when stopping workloads).

**Configuration.** The launcher reads only these variables, from the process environment or the `--env-file` (default repository-root `.env`, parsed as data and never sourced): `G8E_HOSTNAME` (default `g8e.local`), `G8E_OLLAMA_ENDPOINT` (required, validated as an HTTP(S) URL), and `G8E_PROVENANCE_HOST`, `G8E_OBSERVER_HOST`, `G8E_INFERENCE_HOST`, `G8E_DATA_HOST` (each default `localhost`). Exported variables beat `.env`; other `.env` settings are not exported to launched processes. It needs `python-dotenv` from `make ensemble-env`.

**Roles and working directories.** The roles are provenance, observer, inference, and data. Each Operator needs its own working directory, because a shared directory would share its `.g8e` runtime and credentials; the launcher rejects duplicates. The default is `~/.ollama/g8e/<role>` (override with `--<role>-working-dir`). g8ee runs from `.local.dev/full/ensemble`. Each local process writes `full.pid` and `full.log` in its directory, and the launcher records local workload directories in `.local.dev/full/workloads.json`. The provenance model storage root comes from `--model-storage-root` or is detected from common Ollama locations.

**Identity preflight.** Before launch the launcher installs the local Gateway trust bundle into each local workload directory. When a workload's saved trust no longer matches the Gateway root CA, `make full` stops and asks for an explicit reset (`make full-reset` or `RESET_IDENTITIES=1`; `make full-setup` prompts). The reset runs `g8e operator reset-identity` or `g8e ensemble reset-identity` for the stale workloads and preserves their working data, model files, vault keys, configuration, and logs; the workloads then submit fresh enrollment requests. Approve them as in [Standard bootstrap workflow](#standard-bootstrap-workflow).

**Remote roles.** A role whose host is not `localhost`, `127.0.0.1`, or `::1` is not started. The launcher prints POSIX shell and Windows PowerShell commands to run on that host with a `g8e` binary built for it, using `--trust-bundle gateway-ca-bundle.pem`. Copy the Gateway CA bundle there over a trusted channel and verify its fingerprint independently. The Gateway hostname must be reachable from that host and match the Gateway TLS certificate.

**Windows hosts with the Gateway in WSL.** `scripts/configure-gateway-lan.ps1` (`-Action Inspect|Apply|Remove`, default `Inspect`) forwards ports 8080 and 8443 from a Windows LAN address to the WSL address and manages matching firewall rules; `Apply` requires `-RemoteScope`. See [Network Architecture](../architecture/network.md) and [Connect Operator to Gateway](connect_operator_to_gateway.md).

## Compose services and unified stack

The root `docker-compose.yml` defines the core platform services on the `g8e-net` bridge network. All four core services start together in the default profile:

| Service | Profile | Published ports | Role |
| --- | --- | --- | --- |
| `g8e-gateway` | default | 8080 HTTP, 8443 HTTPS; 8081 private mirror ingest, 8082 public mirror read/SSE, 5173 evaluation explorer (loopback) | Policy Decision Point (PDP). PKI, governance, pub/sub, console, MCP, A2A, public mirror, and evaluation explorer. |
| `g8e-data-operator` | default | none | **Data Operator** (container hostname `data-operator`) — governed tool/filesystem/process boundary for the Operator container runtime. Evaluations identify it by that hostname and ignore every other enrolled data Operator. |
| `g8e-inference-operator` | default | none | **Inference Operator** — governed inference to the remote Ollama provider. |
| `ensemble` | default | 8000 (loopback) | g8ee chat pipeline (`POST /api/v1/chat`). Published on loopback for host tools; g8ee accepts proxy identity only with the Gateway's signature, wherever the request comes from. |

The Gateway and Operator containers use the same image and share the host binary mount. All four services start with `docker compose up -d`. The Observer Operator is a separately enrolled process on the remote provider host and is never a service in the unified Compose stack.

The root Compose file also defines optional profiles for specialized testing:

| Profile | Services | Purpose |
| --- | --- | --- |
| `cross-enrollment` | `g8e-gateway-secondary` | Runs a second gateway binary in outbound Operator mode against the primary Gateway. |
| `g8ellama` | `g8e-gateway-user`, `g8e-inference` | Legacy separate User Gateway and Inference Node topology for a remote Ollama provider. |

The `g8ellama` services have separate volumes and ports (`8090`/`8453` for the User Gateway by default); do not combine that profile with the campaign topology unless you intend to run both independent deployments.

### Runtime boundaries and persistent state

The Compose services run in separate containers, process namespaces, network namespaces, and named volumes on the `g8e-net` bridge (`172.28.0.0/16`). The remote Data Operator targets only the Operator container runtime; it has no Docker socket, host root filesystem, host PID namespace, or host network. The Gateway's embedded Operator targets only the Gateway container runtime. Containers reach the Gateway as `g8e.local:8080` or `g8e.local:8443`; host-side clients reach the published host ports, and `localhost` is namespace-relative. The Gateway's read-only `/etc/hosts` and `/etc/hostname` mounts are used for network identity and serving-certificate SAN detection; they do not grant host execution access.

The root stack uses `g8e-gateway-data`, `g8e-operator-data`, `g8e-inference-data`, `g8e-ensemble-data`, and the shared `g8e-shared-tmp` volume. The ensemble additionally mounts `g8e-operator-data` read-only at `/operator-state` for bootstrap material; this does not make the ensemble the Operator or share the Operator's runtime tree. Removing a component volume removes that component's credentials and local state. `docker compose down` preserves volumes; `down -v` and `./g8e docker clean` destroy them and require re-enrollment.

### Evaluation campaign topology

Scored assignments use one campaign Gateway with two remote Operator sessions on the campaign host plus one or two witness sessions on the provider host:

```text
Campaign host (Linux + Docker)
  g8e-gateway ........................ PDP, pub/sub, inference dispatch fan-out
  g8e-data-operator .................... Data Operator (governed tools)
  g8e-inference-operator ............. Inference Operator → remote Ollama
  ensemble ........................... g8ee ChatPipelineService

Provider host (Windows + Ollama example)
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

| Purpose | Campaign ID | Run ID pattern | Model inventory | Cells (27 scenarios × eligible roles) |
| --- | --- | --- | --- | --- |
| **Init campaign** (one model, tidy pipeline gate) | `eval-init-<variant_id>` | `<campaign-id>-<unix>` | `.g8e/eval/inventories/<campaign-id>.json` | 1 model → **41** |
| **Mini smoke** (multi-model pipeline validation) | `eval-smoke-mini` | `smoke-mini-<unix>` | `.g8e/eval/inventories/eval-smoke-mini.json` | 3 models → **123** |
| **Full homogeneous run** | `eval-genesis-homogeneous` | `genesis-homogeneous-<seq>` | `.g8e/eval/model-inventory.json` (from `inventory freeze`) | all discovered models |

Rules:

- Campaigns score a suite (`g8e eval campaigns create --suite <id>`); the default is the built-in `default-suite`. Campaigns frozen before the suite rename carry the legacy catalog ID `north-star-25` in their frozen spec; that identity is historical and is never used for new campaigns, run IDs, or campaign IDs.
- A campaign ID freezes its suite catalog. When the catalog changes, `g8e eval rollout run` qualifies the entry under `<campaign-id>-<8 characters of the catalog digest>` instead of failing with `campaign already exists with a different frozen spec`; `g8e eval campaigns create` still fails, so pick a new ID there.
- Use **Genesis** for the first public homogeneous release (`eval-genesis-homogeneous`).
- Every cold start gets a **new run ID**. Never resume abandoned runs after a volume wipe.
- Campaign authority travels on each governed dispatch from g8ee; the inference operator is never rebound per model or per campaign.

### Init campaign inventory (one model per campaign)

A `model-role` campaign freezes exactly one model: **one model, one campaign, 41 cells**. `g8e eval campaigns create` and `g8e eval runs start` reject any other count. This keeps runs tidy, isolates failures, and lets the provider unload each model when its campaign finishes. Use `g8e eval runs start` for one model, or `g8e eval rollout run` for many (`g8e eval rollout next` inspects the next pending entry) — no `.env` edits or operator recreate between models.

Runtime data lives under `.g8e/eval/` (gitignored). See [eval/examples/README.md](../../eval/README.md) for the public/private boundary.

```bash
# Freeze your provider's model registry (once per provider snapshot)
./g8e eval models freeze

# Single model — create campaign and start execution
./g8e eval campaigns create eval-init-qwen3-4b qwen3:4b
./g8e eval runs start eval-init-qwen3-4b \
  --publish --daemon \
  --verify \
  --require-witness
```

Optional rollout queue (multi-model tracking):

```bash
# After inventory freeze, add models to build the queue
./g8e eval rollout add --all

# Unattended strict-witness rollout (replaces private batch shell scripts).
# Defaults: --require-witness, --verify, --publish, --daemon, and --skip-verified are all true.
./g8e eval rollout run

# --skip-verified skips an entry only while it is verified on the current catalog.
# Exclude specific variants or re-run verified entries:
./g8e eval rollout skip granite3-3-2b
./g8e eval rollout run --skip-verified=false

# Or one model at a time:
./g8e eval rollout next
./g8e eval rollout run --until 1
```

To compare models side by side in the explorer, run them with the Provider Observer enrolled. Each model's run is its own dataset, and the explorer compares datasets that report the same GPU memory and system RAM, which the observer supplies once a run completes; there is nothing to configure.

List the variants available to queue:

```bash
./g8e eval models list
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
# Full provider freeze, then queue each smoke model for rollout:
./g8e eval models freeze

./g8e eval rollout add qwen3:0.6b
./g8e eval rollout add qwen3:4b
./g8e eval rollout add gemma3:4b
./g8e eval rollout run
```

Rollout runs three campaigns of **41** role-eligible scenario cells each (**123** assignments in total), one model resident at a time.

## Environment configuration

Copy `.env.example` to `.env` and set the one required value:

```bash
G8E_OLLAMA_ENDPOINT=http://192.168.1.2:11434
```

`g8e docker init` validates only `G8E_OLLAMA_ENDPOINT`. Campaign ID and registry digest are not startup settings: `g8e eval runs start` resolves them from the campaign definition and the campaign controller attaches them to each governed dispatch, so console chat and campaigns run on the same Inference Operator at the same time.

`.env` holds only secrets and user-specific endpoints or identities (INV-ENV-04 in [Developer Guidelines](../devs/devs.md)). Everything else is fixed in `docker-compose.yml` or in the `g8e` binary defaults:

| Variable | Default | Effect |
| --- | --- | --- |
| `G8E_OLLAMA_ENDPOINT` | *(required)* | Approved Ollama URL for the Inference Operator; Compose fails fast when unset (required for live inference campaigns and `docker init`). `localhost` is the container itself, so use an address the container can reach. |
| `G8E_HOSTNAME` | `localhost` | Browser-visible gateway hostname (approval links, CORS, WebAuthn); set only when you reach the stack by another name |
| `G8E_USER_HOSTNAME` | `localhost` | Same role for the User Gateway (g8ellama profile) |

Container names (`g8e-<service>`) and host ports (8080, 8443, and loopback 8000, 8081, 8082, 5173) are literals in `docker-compose.yml`. Operators use the `g8e operator start` default heartbeat interval of 30 seconds; the Gateway marks an Operator `stale` after 60 seconds without a heartbeat, so `--heartbeat-interval` accepts at most 30. The Inference Operator has no model configuration: you choose each role's model in the Console Inference view and every governed request carries it. Its keep-alive comes from the `g8e operator start` default (`--inference-keep-alive`; `./g8e operator start --help` lists it). To run different ports or keep-alive, add a checked-in `docker-compose.override.yml` that changes the published ports or appends that flag to the Inference Operator `command`; do not set them in `.env`.

## Standard bootstrap workflow

### 1. Start the unified stack

All four core services (Gateway, Data Operator, Inference Operator, and ensemble) start together in the default profile without requiring multi-stage profile bootstrapping. Workloads automatically submit platform enrollment requests and poll for owner approval.

```bash
# Pure Docker:
docker compose up -d
until curl -fsS http://127.0.0.1:8080/api/v1/health >/dev/null; do sleep 2; done

# ./g8e docker CLI equivalent:
./g8e docker start

# make equivalent:
make docker-up
```

### 2. Export host binary (optional for Docker-only hosts)

If you only have Docker installed and want the CLI binary locally on your host:

```bash
# Pure Docker:
docker compose cp g8e-gateway:/g8e ./g8e && cp ./g8e bin/g8e

# ./g8e docker CLI equivalent:
./g8e docker build

# make equivalent:
make docker-build
```

### 3. Enroll the owner

The stack services (`g8e-data-operator`, `g8e-inference-operator`, `ensemble`) submit platform enrollment requests to the Gateway and poll for approval.

Enroll the owner identity:

**Pure Docker (container execution, no host Go or Make required):**
```bash
docker compose exec g8e-gateway /g8e auth enroll user --headless -e localhost
```

**Host CLI:**
- Interactive (browser WebAuthn passkey ceremony):
  ```bash
  ./g8e auth enroll user -e localhost
  ```
- Headless (CLI-only owner):
  ```bash
  ./g8e auth enroll user --headless -e localhost
  ```

### 4. Approve platform enrollments

Wait ~5s, then list pending enrollments:

```bash
# Pure Docker:
docker compose exec g8e-gateway /g8e auth enroll pending

# Host CLI:
./g8e auth enroll pending
```

Approve in this order (Data Operator first), or deny any request that should not be admitted:

```bash
# Pure Docker:
docker compose exec g8e-gateway /g8e auth enroll approve <data-operator-request-id> --yes
docker compose exec g8e-gateway /g8e auth enroll approve <ensemble-request-id> --yes
docker compose exec g8e-gateway /g8e auth enroll approve <inference-operator-request-id> --yes
# docker compose exec g8e-gateway /g8e auth enroll deny <request-id> --yes

# Host CLI:
./g8e auth enroll approve <data-operator-request-id> --yes
./g8e auth enroll approve <ensemble-request-id> --yes
./g8e auth enroll approve <inference-operator-request-id> --yes
# ./g8e auth enroll deny <request-id> --yes
```

Identify requests by instance ID: `operator-<container-id>` is the **Data** Operator; `operator-inference-operator` is the **Inference** Operator.

### 5. Verify readiness and align versions

```bash
# Pure Docker:
until curl -fsS http://127.0.0.1:8000/health >/dev/null; do sleep 3; done
docker compose exec g8e-gateway /g8e operator list
docker compose exec g8e-gateway /g8e operator session list --json

# Host CLI:
until curl -fsS http://127.0.0.1:8000/health >/dev/null; do sleep 3; done
./g8e operator list
./g8e operator session list --json
```

Expect **two** remote Operators (data + inference) plus one embedded Gateway operator, and an active inference session ID.

Session IDs change on every volume wipe. Rediscover them after any `docker compose down -v` or `./g8e docker clean`.

**Aligning Operator versions after a local `make build` (local Make is gospel):**
Whenever you build locally with `make build`, the updated binary is installed to `bin/g8e` and `./g8e`. Because container operators mount `./bin:/opt/g8e/bin:ro`, restarting the operator containers immediately activates the new binary:

```bash
# Pure Docker:
docker compose restart g8e-data-operator g8e-inference-operator

# ./g8e docker CLI equivalent:
./g8e docker restart

# make equivalent:
make docker-restart-operators
```

### Stop or revoke an enrolled workload

Use the Operator session ID from `operator list` for a reversible process stop:

```bash
# Pure Docker:
docker compose exec g8e-gateway /g8e operator stop <operator-session-id> --reason "planned maintenance"

# Host CLI:
./g8e operator stop <operator-session-id> --reason "planned maintenance"
```

The command targets one active remote Operator owned by the authenticated user. It waits for the Operator's governed shutdown acknowledgement before the Gateway records `stopped`; the embedded Gateway Operator is never a valid target. The workload retains its certificate-backed enrollment and can start again later with its existing credentials.

Use the completed platform enrollment request ID for permanent identity revocation:

```bash
# Pure Docker:
docker compose exec g8e-gateway /g8e auth enroll revoke <request-id> --reason "host retired" --yes

# Host CLI:
./g8e auth enroll revoke <request-id> --reason "host retired" --yes
```

Operator revocation invalidates the Operator and companion CLI certificates, deactivates their sessions, marks the Operator `terminated`, and disconnects established pub/sub connections. Ensemble revocation invalidates the application certificate, removes its application policy, and disconnects established pub/sub connections. The workload must submit a new enrollment request and receive owner approval before it can authenticate again. Repeating the command for the same request is idempotent.

### Automated alternative

```bash
# Host CLI:
./g8e docker init
```

`g8e docker init` runs the full bootstrap in one command: prepare the host `.g8e` tree, build images, start the unified stack, enroll the CLI owner, auto-approve platform enrollments in order (data operator → ensemble → inference operator), and wait for ensemble health. Requires a repository-root `.env` with `G8E_OLLAMA_ENDPOINT` set before running.

**Init enrolls `g8e-eval`.** `./g8e eval` gates, probes, and campaigns dispatch inference with a separate host application identity, `g8e-eval`, and fail closed without it (console chat does not use it). As its last step init enrolls that identity unless a valid one is already installed, and approves exactly the pending request that is an application named `g8e-eval` (no other application or Operator request). It polls every 2 seconds for up to 3 minutes; a failure prints a warning and the manual steps and does not fail init. `--skip-approvals` skips this step. `./g8e docker start` does not enroll it; it only reports at the end whether the identity is enrolled. To enroll manually, in a second terminal because the command waits for approval: `./g8e auth enroll app g8e-eval`, then `./g8e auth enroll pending` and `./g8e auth enroll approve <request-id> --yes`. `docker init --clean` destroys the trust domain, so init enrolls it again afterwards.

If a prior Docker start created `.g8e` as root, fix ownership once with `sudo chown -R $(id -u):$(id -g) .g8e` and rerun init.

Useful flags:

- `--clean` — wipe containers/volumes/networks before init (cold start). Confirms first and offers an evidence backup; `--yes` skips the confirmation, `--skip-backup` skips the backup.
- `--skip-build` — reuse existing images.
- `--skip-enroll` — reuse an already-enrolled CLI identity.
- `--skip-approvals` — start workloads without auto-approving enrollments.
- `--headless` — mTLS-only owner enrollment without the browser passkey ceremony (default runs passkey enrollment).

For interactive stack startup with enrollment walkthrough prompts, use:

```bash
# Host CLI:
./g8e docker start
```

## Provider-boundary Observer Operator (provider host; Windows example)

Deploy this **on the machine that runs Ollama** (for example `192.168.1.2`), not in the campaign-host Compose stack. The commands below use the Windows provider-host build and PowerShell; the Observer Operator also has Unix host-RAM collection support.

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
4. Gateway ingests windows for `g8e eval runs verify --require-observation`.

The Observer only samples provider-boundary telemetry between BEGIN and FINALIZE. It does not manage Ollama, restart the daemon, unload models, or execute generic commands. Model residency is owned by Ollama, and campaign model release uses an approved command dispatched to the exact Inference Operator after scored work completes.

The Observer Operator is the only provider-boundary observer. There is no CLI-side observer process.

**Timing rule:** Assignments that reached a terminal state before the Observer Operator was enrolled and pub/sub-connected will fail `--require-provider-observation`. That is expected. Enroll the observer before `execute`, or accept that early assignments lack hardware windows.

For example Observer Operator console output and a healthy-output checklist, see [Evaluations — Witness operator roles](../architecture/evals.md#witness-operator-roles).

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

For architecture detail and example console output, see [Evaluations — Witness operator roles](../architecture/evals.md#witness-operator-roles) and [Model Provenance](../architecture/model-provenance.md).

## Formation smoke (`ultra-efficient-speedster`)

Use this after the evaluation stack, Inference Operator, and **both** witness Operators (Observer + Provenance) are enrolled. It validates the three-model execution-topology path (Lite → Assistant → Primary) without scheduling a full campaign matrix.

### Prerequisites

1. Observer and Provenance Operators enrolled on the provider host **before** the run (see sections above).
2. All three formation served tags present on the approved Ollama endpoint, staged through the exact Inference Operator session. The ultra-efficient-speedster formation uses:
   - `llama3.2:1b` (Lite role)
   - `gemma4:e2b` (Assistant role)
   - `qwen3.5:4b` (Primary role)
3. An enrolled `g8e-eval` application identity (`./g8e docker init` enrolls and approves it; otherwise `./g8e auth enroll app g8e-eval`, approved once by the platform owner). `./g8e eval` formation commands present this managed application credential for inference dispatch. They verify the `g8e-eval` app cert against the gateway's current trust bundle before dispatching, failing closed with an actionable error if the credential is missing, expired, or untrusted.

### Build formation inventory

Pull and freeze the catalog models:

```bash
./g8e eval models pull --formations
./g8e eval models freeze
```

Use `g8e eval formations list` to inspect the formations and their served tags.

### Run formation smoke

```bash
./g8e eval formations smoke ultra-efficient-speedster
```

Expect `"passed": true` with three role entries (lite, assistant, primary), per-role attestation, generation throughput, and peak VRAM. Observer evidence is loaded from the gateway volume via mTLS (`/api/v1/inference/provider-observations/{attempt_id}`); host `.g8e/data` does not mirror gateway witness stores.

Inspect the catalog without running:

```bash
./g8e eval formations list
./g8e eval formations show ultra-efficient-speedster
```

Add or replace a formation in the checked-in overlay (`eval/formation-catalog-overlay.json`), or remove one. Every role needs its model tag, provider, family, quantization, parameters, and VRAM estimates; see `./g8e eval formations add --help` for the full flag set.

```bash
./g8e eval formations add my-formation --display-name "My Formation" --description "..." \
  --primary-tag <tag> ... --assistant-tag <tag> ... --lite-tag <tag> ...
./g8e eval formations remove my-formation
```

### Heterogeneous campaign assignment (Phase 2)

After formation smoke passes, run through the full campaign lifecycle:

```bash
./g8e eval campaigns create eval-formations-smoke --formations ultra-efficient-speedster

./g8e eval runs start eval-formations-smoke \
  --publish --daemon \
  --verify \
  --require-witness
```

Formation-run evidence is persisted at `.g8e/data/eval/runs/<run-id>/assignments/<assignment-id>-formation-run.json`.

## Mini smoke campaign workflow

Use this to validate the full pipeline (schedule → execute → publish → explorer) in hours instead of days.

### Phase A — Reset public feed (cold start)

The gateway owns the public feed, mirror (`8081` private ingest, `8082` public read/SSE), and evaluation explorer (`5173`) in the `g8e-gateway-data` volume when started with `--public-spectator` (default). Docker Compose enables this automatically. Campaign `schedule --publish` and `execute --publish` post signed batches through the gateway API (`POST /api/v1/public-feed/batches`).

A Compose volume wipe resets the entire Docker trust domain, not only the public feed. Use it only for a cold start, then repeat owner and workload enrollment:

```bash
docker compose down -v
docker compose up -d g8e-gateway
```

For a normal restart that preserves the public feed and all credentials, use `docker compose up -d g8e-gateway` or `./g8e docker stop` followed by `./g8e docker start`.

Explorer (acceptance UI): open `http://127.0.0.1:5173/#/` after the gateway is up. Build static assets once with `cd evaluation-explorer && npm run build` if the explorer listener logs that dist is missing. Do **not** run `npm run dev:real` for campaign acceptance — that path is legacy local supervisor only.

### Phase B — Create campaign and start run

```bash
# Single model — create campaign and start execution
./g8e eval campaigns create eval-smoke-mini qwen3:4b

# Start execution (schedules the matrix and executes continuously). The run
# targets the stack's Inference Operator and its `data-operator`, binding the
# data-operator to the CLI session when it is not already bound.
./g8e eval runs start eval-smoke-mini --publish --daemon
```

Alternatively, to schedule first and execute separately:

```bash
# Persist run and assignments without executing immediately
./g8e eval runs start eval-smoke-mini --prepare-only

# Resume/execute the prepared run
./g8e eval runs resume <run-id> --publish --daemon
```

### Phase C — Execute (serial daemon)

- `--daemon` runs the full matrix in one process.
- Consecutive assignments keep the provider daemon running; no reset or stable-body `/api/ps` wait occurs between cells.
- After the queue is exhausted, the controller dispatches governed residency reads and the image-baked `/g8e operator model release <served-tag>` command through the exact Inference Operator. The Operator-owned client uses the endpoint from that session's enrolled `runtime_config` to issue `/api/generate` with `keep_alive: 0`, then governed residency reads confirm the campaign-owned tags are absent. No campaign-host provider call or external Ollama CLI is used.
- **Never** run `g8e eval runs publish` concurrently with `runs start --publish` or `runs resume --publish`.

### Phase D — Monitor

| Surface | URL |
| --- | --- |
| Explorer | `http://127.0.0.1:5173/#/` |
| Live dataset | `ds-live-<run-id>` |
| Public mirror bootstrap | `http://127.0.0.1:8082/bootstrap?source=opendevops-local` |

```bash
./g8e eval runs show "$RUN_ID"
./g8e eval runs assignments "$RUN_ID" --failed
./g8e eval runs verify "$RUN_ID" --coverage
cd evaluation-explorer && npm run health
```

`runs assignments --failed` lists terminal results that did not pass, including `INVALID_EVIDENCE`, with scenario, target (model/role or stack), repetition, duration, verdict, deterministic pass rate, and failed grades with their recorded causes. Use `--assignment <id>` to inspect every grade and its basis for one assignment, or `--json` for canonical assignment/result protojson with explicit verdict and pass rate (`0` for a scored failure, `null` when unscored). Execution logs identify each result as `Executed <id> [<scenario> | <target> | rep N, <duration>]: <lifecycle> verdict=… pass_rate=… failed=[…]`.

After the Observer Operator is enrolled, verify hardware coverage:

```bash
./g8e eval runs verify "$RUN_ID" --require-observation
```

### Hard rules

1. **Never** call Ollama at `127.0.0.1:11434` on the campaign host for scored work when the approved provider is remote.
2. **Always** rediscover Operator session IDs after a volume wipe.
3. **Always** use `g8e eval runs start` (or ensure dispatch-carried campaign authority matches the inventory) — do not rebind `.env` per model.
4. **Never** resume archived or abandoned run IDs from prior checkpoints.

## Public spectator feed

Campaign data publishes through Go (`CampaignPublicationCoordinator` → `PublicPublisherService` outbox → mirror ingest). No Python bridge or host systemd publisher.

`g8e gw start --public-spectator` (default) and `docker compose up -d g8e-gateway` start the in-process mirror on `8081`/`8082` and evaluation explorer on `5173`. Do not install `deploy/systemd/opendevops-eval-publisher.service` (deleted).

## CLI stack management

| Command | Pure Docker Equivalent | Make Equivalent | Behavior |
| --- | --- | --- | --- |
| `./g8e docker init` | `docker compose up -d` + exec enroll/approvals | `make docker-up` (manual approvals) | Build images, enroll owner, start unified stack, auto-approve platform enrollments, and wait for readiness. |
| `./g8e docker start` | `docker compose up -d` | `make docker-up` | Starts default unified stack (all 4 core services) and offers enrollment walkthrough. |
| `./g8e docker restart [service...]` | `docker compose restart g8e-data-operator g8e-inference-operator` | `make docker-restart-operators` | Restarts Data and Inference Operators to align with newly built binary from host mount (`./bin:/opt/g8e/bin:ro`). |
| `./g8e docker stop` | `docker compose down` | `make docker-down` | Stops stack, preserves volumes. |
| `./g8e docker status` | `docker compose ps` | — | Shows running containers and health status. |
| `./g8e ensemble start/stop/restart/status/logs` | Local Python process | — | Uses the active virtual environment, repository `.venv`, or `python3`, sharing the runtime started by `make full`. Run from the repository root. For containers, use `docker compose` directly. |
| `./g8e docker build` | `docker compose build && docker compose cp g8e-gateway:/g8e bin/g8e && cp bin/g8e ./g8e` | `make docker-build` | Build stack images in container via Makefile, export binary to `bin/g8e` and `./g8e`. |
| `./g8e docker rebuild` | `docker compose down && docker compose build && docker compose up -d` | — | Stop stack, rebuild images with in-container Makefile, and restart. |
| `./g8e docker clean` | `docker compose down -v --remove-orphans` | `make docker-clean` | Destructive wipe of containers, volumes, networks. |

Destructive cleanup destroys the trust domain (PKI, owner, Operator identities, campaign state). After `./g8e docker clean` or `docker compose down -v`, repeat owner enrollment and platform approvals.

`./g8e docker clean`, `docker reset`, and `docker init --clean` confirm before wiping volumes and offer to back up host evaluation evidence first. Pass `--yes` to skip the confirmation and `--skip-backup` to skip the backup. Raw `docker compose down -v` and `make docker-clean` have neither safeguard. Docker volumes are deleted outright; only host `.g8e/` wipes (`g8e gw clean`/`gw reset`) are renamed aside to `.g8e-<MMDDHHMM>` instead.

Eval runs back up their evidence to `eval/backups/` automatically when they finish. Before a destructive wipe, take a fresh copy outside `.g8e/` with `./g8e eval backup` (or `--output-dir <dir>`), and put it back with `./g8e eval restore` (newest snapshot in `eval/backups/`, or pass `<dir>/eval-backup-<timestamp>`). See [Evaluation Programs](../architecture/evals.md#evidence-and-verification). This covers host evidence only, not the Gateway volume.

## Health checks and resources

| Service | Health check |
| --- | --- |
| `g8e-gateway` | HTTP `GET /api/v1/health` :8080 |
| `g8e-data-operator` | enrolled operator certificate present |
| `g8e-inference-operator` | enrolled operator certificate present |
| `ensemble` | HTTP `GET /health` :8000 (after enrollment completes) |

Workloads remain unhealthy while enrollment is pending.

| Service | CPU limit | Memory limit |
| --- | --- | --- |
| `g8e-gateway` | 2 | 4G |
| `g8e-data-operator` | 2 | 1G |
| `g8e-inference-operator` | 4 | 4G |
| `ensemble` | 2 | 2G |

## Troubleshooting

### Workload stays unhealthy

```bash
# Pure Docker:
docker compose exec g8e-gateway /g8e auth enroll pending
docker compose logs <service>

# Host CLI:
./g8e auth enroll pending
./g8e docker logs <service>
```

### Inference dispatch returns 403

- `campaign binding invalid` or `model registry invalid`: the campaign request's assignment correlation or registry is incomplete or does not match its digest. Start runs with `g8e eval runs start` so g8ee carries the campaign's frozen registry on each dispatch.
- `model not permitted by the frozen campaign registry`: a campaign-bound request named a model outside its frozen registry. A chat request is never denied for its model; one with no model fails with `model reference invalid`, and one naming a model the Ollama provider lacks fails with `model not found`.

### Observer not receiving commands

- Confirm Observer enrolled with `--provider-boundary-observer-enabled`.
- Confirm Gateway can reach the Observer session (`./g8e operator list`).
- Confirm the provider host can reach Gateway ports 8080/8443 and `g8e.local` resolves to the campaign host.

### Provenance operator not attesting or digest mismatch

- Confirm a **separate** session enrolled with `--provenance-operator-enabled` and `--model-storage-root` pointing at the live Ollama models directory.
- Confirm `./g8e operator list --json` shows `provenance_operator_enabled: true` and the expected storage root.
- Confirm the served model tag and digest in campaign inventory match what Ollama reports (`ollama show <tag> --verbose` or `/api/tags` on the provider).
- Digest mismatch on FINALIZE is intentional fail-closed behavior when weights changed after campaign freeze.

### Campaign model release fails

- Confirm the exact Inference Operator session is active and its enrolled `runtime_config.inference_ollama_endpoint` is correct.
- Inspect typed provider residency at `/api/ps`; malformed responses, endpoint failures, and ambiguous residency fail closed.
- Do not grant the Observer or Provenance Operator command authority. They are read-only witness boundaries; retry release only after the provider owner resolves the endpoint or Ollama client failure.

### Public feed drift or out-of-order batches

- Stop execute and any concurrent `runs publish`.
- Do not run `runs publish` and `runs resume --publish` at the same time.
- Reset public feed (Phase A above) before a new run.

### Mirror empty after `docker init --clean` but host run artifacts remain

`docker init --clean` (or `docker compose down -v`) wipes the standard Docker containers, networks, and named volumes, including the gateway mirror and trust-domain state. Host campaign evidence under `.g8e/data/eval/runs/<run-id>/` and the rollout queue under `.g8e/eval/` are unchanged.

Restore every verified queue entry to the gateway-owned mirror:

```bash
./g8e public restore --queue
```

Or one run:

```bash
./g8e public restore <run-id>
```

`docker init` attempts `--queue` restore automatically when the queue and run artifacts exist. Queue entries whose `verified_run_id` directory is missing are reported as host-absent and must be re-executed — mirror restore cannot recreate inference evidence.

A plain catch-up publish now probes the gateway-owned dataset. When the dataset is absent while host `public-projection-state.json` under `.g8e/data/eval/runs/<run-id>/` still lists `published_idempotency_keys`, it clears that stale state and republishes the canonical lifecycle, result, and aggregate projections without editing JSON manually.

```bash
./g8e eval runs publish <run-id>
```

Use `--force` only when the mirror was wiped or is known to be missing records and the drift-aware path is not sufficient. It clears host idempotency keys and republishes the run; mirror restore cannot recreate missing inference evidence or make an inapplicable verification report valid.

### Browser TLS or WebAuthn failures

Confirm `G8E_HOSTNAME` matches the browser URL, the gateway root CA is trusted, and HTTPS port 8443 is reachable.

## Stopping safely

```bash
# Stop execute: Ctrl-C or kill the execute daemon PID

# Pure Docker:
docker compose down                       # preserves volumes and credentials

# ./g8e docker CLI equivalent:
./g8e docker stop

# make equivalent:
make docker-down

# For a cold reset, choose one destructive command, not both:
# Pure Docker:
docker compose down -v                    # removes standard stack containers, volumes, and networks

# ./g8e docker CLI equivalent:
./g8e docker clean

# make equivalent:
make docker-clean
```

Restarting the gateway with `docker compose up -d g8e-gateway` preserves the trust domain. A cold reset requires owner and workload re-enrollment.

## Anti-patterns

- Starting the stack without setting `G8E_OLLAMA_ENDPOINT` when inference is required. The Inference Operator fails to enroll without a valid provider endpoint.
- Rebinding `.env` campaign ID and digest per model instead of using dispatch-carried campaign authority. This couples the Inference Operator to a single campaign and breaks rollout workflow.
- Running `docker compose down -v` expecting it to preserve credentials — it destroys all volumes including PKI. Use `docker compose down` (without `-v`) to preserve state.
- Starting Formation smoke without enrolling Observer and Provenance Operators first. Early assignments will fail `--require-observation` and `--require-witness`.
- Running `g8e eval runs publish` concurrently with `runs start --publish` or `runs resume --publish`. This causes out-of-order batches in the public feed.
- Granting command execution authority to Observer or Provenance Operators. These are read-only witness boundaries. Never pass `--inference-enabled` or generic command flags to Observer/Provenance enrollments.

## Links out

- [Evaluations](../architecture/evals.md) — platform evaluation programs, Observer and Provenance Operator roles, evidence, and verification.
- [Model Provenance](../architecture/model-provenance.md) — zero-trust weight attestation and chain of custody.
- [Build Operator](./build_operator.md) — build `g8e.exe` for the Windows Observer host.
- [Connect Operator to Gateway](./connect_operator_to_gateway.md) — enrollment protocol details.
- [Docker Gateway Guide](./docker_gateway.md) — standalone gateway operation.
- [g8ee Documentation](../ensemble/index.md) — ensemble configuration and providers.
- [Console Architecture](../architecture/console.md) — the Gateway-served browser console.
- [Authentication and Identity](../architecture/auth.md) — mTLS, WebAuthn, PKI.
- [Evaluation data layout](../../eval/README.md) — public vs runtime vs private operator data.
