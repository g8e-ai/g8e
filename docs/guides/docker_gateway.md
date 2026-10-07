---
doc_id: docker_gateway
title: Docker Gateway Guide
audience: developers and operators deploying g8e in Docker
status: current
last_updated: 2026-10-01
version: v2.2.6
owners:
  - Dockerfile
  - docker-compose.yml
  - .env.example
related:
  - docs/guides/unified_stack.md
  - docs/guides/getting_started.md
  - docs/architecture/network.md
when_to_read: Building Docker images, configuring Compose deployments, enrolling gateways, and running the unified stack or g8ellama topology.
do_not_use_for:
  - Campaign and evaluation workflow (see unified_stack.md)
  - Network architecture and identity detection (see architecture/network.md)
---

# Docker Gateway Guide

## Purpose

This guide covers building the root Docker image, running the unified Docker Compose stack, enrolling the Gateway and workloads, and managing different deployment topologies. The Gateway is the Policy Decision Point: it owns the container-local PKI, coordinates workload identities, exposes the authenticated APIs, and runs the embedded Operator for its own runtime. The Data Operator and Inference Operator are separate Policy Execution Points. For campaign workflow and inference evaluation, see [Unified Docker Stack Guide](./unified_stack.md).

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
| INV-DG-IMG-01 | The Dockerfile builds only the target platform binary (linux/amd64 or linux/arm64) and does not build the full cross-platform matrix. Host cross-platform artifacts require `make build-all` on the host. |
| INV-DG-IMG-02 | The image entrypoint is `/entrypoint.sh`, which enforces binary precedence: G8E_BIN override, /opt/g8e/bin/g8e host mount, /opt/g8e/bin/g8e-linux-${ARCH} host mount, then /g8e image-baked binary. |
| INV-DG-COMPOSE-01 | The root docker-compose.yml default profile runs the unified stack: g8e-gateway, g8e-data-operator, g8e-inference-operator, and ensemble. The Gateway serves the browser console at `/console/`. No services require a profile to start. |
| INV-DG-COMPOSE-02 | The cross-enrollment profile adds g8e-gateway-secondary (operator mode, enrolls as outbound operator of primary). The g8ellama profile replaces the default gateway with g8e-gateway-user and g8e-inference (User Gateway topology). Do not combine profiles. |
| INV-DG-COMPOSE-03 | Environment variables from `.env` or inline export override defaults. See .env.example for the complete reference. Port overrides do not change internal container listeners or service-network aliases (g8e.local, g8eg). |
| INV-DG-ENROLL-01 | Workloads submit platform enrollment requests when reusable credentials are absent. Owner approval issues identity via the Gateway PKI. Each workload has its own runtime volume and enrolls independently. |
| INV-DG-ENROLL-02 | The `./g8e docker start` CLI command starts the unified stack and runs interactive owner/workload enrollment. Manual enrollment uses `./g8e auth enroll user`, `./g8e auth enroll pending`, and `./g8e auth enroll approve <request-id>`. `./g8e docker init` also enrolls and approves the host `g8e-eval` application identity that `./g8e eval` needs (skipped with `--skip-approvals`, or when a valid identity exists). `docker start` does not enroll it; it only reports at the end whether it is ready (manual path: `./g8e auth enroll app g8e-eval`). |
| INV-DG-HEALTH-01 | Gateway health endpoint is http://localhost:8080/api/v1/health. Healthchecks are declared per-service in docker-compose.yml; the image has no baked-in HEALTHCHECK. |
| INV-DG-GATEWAY-01 | The gateway `gw start` command accepts flags: --posture (doctrine/consensus/ratify/notary, default doctrine), --cert-mode (full/localhost), --public-spectator, --eval-explorer-listen, --cors-origin, --passkey-rp-* and others. See internal/cli/cmd/gw/gateway.go for complete flag list. |
| INV-DG-OPERATOR-01 | The operator `operator start` command accepts flags: -e endpoint, --heartbeat-interval, --inference-enabled, --inference-ollama-endpoint, and model/campaign configuration flags. See internal/cli/cmd/operator/operator.go for complete flag list. |
| INV-DG-PORTS-01 | Gateway exposes container ports 8080 (HTTP) and 8443 (HTTPS/mTLS). Additional gateway surfaces: 8081 (public spectator private), 8082 (public spectator public), 5173 (evaluation explorer). Ensemble: 8000. All port bindings are overridable via environment variables. |
| INV-DG-VOLUMES-01 | Each service has a persistent volume. Gateway volume at /root/.g8e contains data/, pki/, secrets/, vault/. Host CLI identity lives in the host .g8e tree, separate from container volumes. Removing gateway volume destroys the authority and requires re-enrollment. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Root Dockerfile | Dockerfile | `docker build -t g8e-gateway:latest .` builds target platform binary |
| Root Compose stack | docker-compose.yml | Services, profiles, health checks, resource limits, environment variables |
| Environment reference | .env.example | G8E_* variable defaults and descriptions |
| Gateway CLI flags | internal/cli/cmd/gw/gateway.go | --posture, --cert-mode, --public-spectator, --cors-origin, --passkey-rp-* |
| Operator CLI flags | internal/cli/cmd/operator/operator.go | -e, --heartbeat-interval, --inference-* flags |
| Port constants | internal/constants/ports.go | Container port definitions (8080, 8443, 8081, 8082, 5173) |

## Procedures

### Build the Gateway image

Build the root image from the repository root:

```bash
docker build -t g8e-gateway:latest .
```

The image entrypoint is `/entrypoint.sh`. The same image runs Gateway and Operator commands; the command supplied by Compose or `docker run` selects the mode. The Dockerfile builds only the binary for the image target platform (`linux/amd64` or `linux/arm64`) and copies that executable to `/g8e`. It does not build the full cross-platform deployment matrix on every image build. Run `make build-all` on the host when you need Linux, Windows, and macOS artifacts for remote operator deployment. Managed `g8e docker build`, `g8e docker init`, and `g8e docker rebuild` export the runtime binary to `./g8e` after a successful build. Raw `docker build` or `docker compose build` users can copy the binary with `docker cp g8e-gateway:/g8e ./g8e`.

The image exposes container ports 8080 and 8443. Compose declares service-specific health checks because the image also runs the outbound-only Operator, which has no listening gateway port.

### Prerequisites

- Docker Engine with the Docker Compose v2 plugin.
- A checkout of this repository.
- The repository's `./g8e` binary when using CLI enrollment or lifecycle commands. Build it with `make build` if it is absent.
- Ports available for the surfaces you enable. The default stack publishes 8080, 8443, 8081, 8082, 5173, 8000, and 3000.
- A browser with WebAuthn support for passkey-based owner enrollment. Headless enrollment is available for CLI-only operation.

Run repository commands from the repository root. The `./g8e docker` commands require `docker-compose.yml` in the current directory.

### Run a standalone Gateway

Start only a Gateway with a persistent named volume:

```bash
docker run -d \
  --name g8e-gateway \
  -p 8080:8080 \
  -p 8443:8443 \
  -v g8e-data:/root/.g8e \
  g8e-gateway:latest \
  gw start -f --posture doctrine --cert-mode localhost
```

`-f` keeps the Gateway in the foreground as the container's main process. `--cert-mode localhost` limits the serving identity to local names and loopback addresses, which makes this example suitable for local access. The default certificate mode is `full`; use it only when the container can read the host identity inputs described in [Host identity and certificates](#host-identity-and-certificates).

Wait for the unauthenticated health endpoint, then enroll the first owner from the repository host:

```bash
until curl -fsS http://127.0.0.1:8080/api/v1/health >/dev/null 2>&1; do sleep 2; done
./g8e auth enroll user -e localhost
```

The host CLI identity is stored in the host checkout's `.g8e` tree. It is separate from the Gateway's `/root/.g8e` state in `g8e-data`. A standalone container does not start the Data Operator, Inference Operator, or ensemble.

To change host ports while keeping the container listeners unchanged, publish different host ports:

```bash
docker run -d \
  --name g8e-gateway-custom-ports \
  -p 3000:8080 \
  -p 3443:8443 \
  -v g8e-custom-data:/root/.g8e \
  g8e-gateway:latest \
  gw start -f --posture doctrine --cert-mode localhost
```

If the listener ports themselves change, pass matching `--http-port` and `--https-port` values and publish those container ports instead.

### Start the unified Docker Compose stack

The root `docker-compose.yml` defines a single-host reference deployment on the `g8e-net` bridge network. The default profile starts the unified stack: Gateway, Data Operator, Inference Operator, and ensemble.

| Service | Profile | Published ports | Role | Persistent volume |
| --- | --- | --- | --- | --- |
| `g8e-gateway` | default | 8080, 8443, 8081, 8082, 5173 | Gateway PDP, PKI authority, MCP, A2A, governance, console, public spectator, evaluation explorer | `g8e-gateway-data` |
| `g8e-data-operator` | default | none | Data Operator (hostname `data-operator`) with outbound-only mTLS execution boundary | `g8e-operator-data` |
| `g8e-inference-operator` | default | none | Inference Operator for remote Ollama inference | `g8e-inference-data` |
| `ensemble` | default | 8000 | g8ee FastAPI agentic ensemble | `g8e-ensemble-data` |
| `g8e-gateway-secondary` | `cross-enrollment` | none | Secondary gateway in operator mode, enrolls against primary | `g8e-gateway-secondary-data` |
| `g8e-gateway-user` | `g8ellama` | 8090, 8453 | User Gateway owning state Merkle root and governance for inference | `g8e-gateway-user-data` |
| `g8e-inference` | `g8ellama` | none | Inference Node in operator mode, enrolls as outbound operator of User Gateway | `g8e-inference-data` |

#### Start the unified stack

```bash
docker compose up -d --build
curl -fsS http://127.0.0.1:8080/api/v1/health
```

The HTTP endpoint is for health, bootstrap discovery, CA bundle retrieval, and platform enrollment submission. Authenticated APIs, MCP, A2A, governance envelopes, pub/sub, and the console use the HTTPS/mTLS listener.

Enroll the first owner, then approve platform workload requests:

```bash
./g8e auth enroll user -e localhost
./g8e auth enroll pending
```

Approve or deny each workload enrollment request:

```bash
./g8e auth enroll approve <data-operator-request-id> --yes
./g8e auth enroll approve <ensemble-request-id> --yes
./g8e auth enroll approve <inference-operator-request-id> --yes
./g8e auth enroll deny <request-id> --yes
```

The commands use the enrolled host CLI identity over mTLS and accept request IDs, not requester tokens, token hashes, CSRs, or certificates.

For inference against a remote Ollama provider, set `G8E_OLLAMA_ENDPOINT` in `.env`. The Inference Operator will enroll and become ready once approved:

```bash
G8E_OLLAMA_ENDPOINT=http://approved-provider:11434 docker compose up -d --build
./g8e auth enroll user -e localhost
./g8e auth enroll pending
./g8e auth enroll approve <inference-operator-request-id> --yes
```

No Ollama daemon runs in this stack. The full campaign workflow, provider Observer Operator, Provenance Operator, and campaign verification are documented in [Unified Docker Stack Guide](./unified_stack.md).

#### Use the CLI lifecycle commands

The CLI wraps the root Compose file and prepares the host `.g8e` runtime tree before enrollment:

```bash
./g8e docker start                       # Unified stack with interactive enrollment
./g8e docker start --skip-enroll         # Unified stack without enrollment prompts
./g8e docker init                        # Build and bootstrap for evaluation
./g8e docker status
./g8e docker logs g8e-gateway -f
```

`docker start` launches the complete unified stack and walks through owner and workload enrollment. A missing request or skipped prompt is non-fatal; finish it later with `auth enroll pending` and `auth enroll approve`.

`docker init` is the automated evaluation bootstrap. It requires a repository-root `.env` with `G8E_OLLAMA_ENDPOINT` set, builds unless `--skip-build` is supplied, starts the complete stack, enrolls the owner, auto-approves all platform workloads, and waits for ensemble health. Useful flags are `--clean` (destructive volume wipe; confirms and offers an evidence backup first, with `--yes` and `--skip-backup` as for `docker clean`), `--skip-build`, `--skip-enroll`, `--skip-approvals`, and `--headless`.

The CLI walkthrough assumes host gateway ports 8080 and 8443. With remapped ports, use manual enrollment commands and specify both endpoints:

```bash
./g8e auth enroll user -e localhost:18080 --port 18443
./g8e auth enroll pending -e localhost:18080 --port 18443
./g8e auth enroll approve <request-id> --yes -e localhost:18080 --port 18443
```

### Compose configuration

Copy `.env.example` to `.env`. It holds only secrets and user-specific endpoints or identities (INV-ENV-04 in [Developer Guidelines](../devs/devs.md)):

| Variable | Default | Effect |
| --- | --- | --- |
| `G8E_OLLAMA_ENDPOINT` | *(required)* | Approved remote Ollama URL for the Inference Operator. Compose fails fast when it is unset. |
| `G8E_HOSTNAME` | `localhost` | Browser-visible hostname used for the Gateway public URL, CORS, and passkey RP settings. Set only when you reach the stack by another name. |
| `G8E_USER_HOSTNAME` | `localhost` | Public User Gateway hostname for CORS and passkey configuration (g8ellama profile). |

Container names (`g8e-<service>`) and host ports are literals in `docker-compose.yml`. The compose operators use the `g8e operator start` default heartbeat interval of 30 seconds; the Operator declares its interval to the Gateway at session start, and the Gateway marks it `stale` after twice that interval (at least 60 seconds). `--heartbeat-interval` accepts 0 through 300:

| Service | Published host ports |
| --- | --- |
| `g8e-gateway` | 8080 (HTTP), 8443 (HTTPS), and loopback-only 8081 (mirror ingest), 8082 (public mirror read/SSE), 5173 (evaluation explorer) |
| `ensemble` | 8000 |
| `g8e-gateway-user` (g8ellama profile) | 8090 (HTTP), 8453 (HTTPS) |

The Inference Operator model roles and keep-alive are the `g8e operator start` defaults; `./g8e operator start --help` lists them alongside the `--inference-*` flags. To run different ports or models, add a checked-in `docker-compose.override.yml` that changes the published ports or appends those flags to the Inference Operator `command`. Do not put them in `.env`. Campaign authority travels on each dispatch; see the [Unified Docker Stack](./unified_stack.md#environment-configuration).

Example override that remaps the gateway host ports:

```yaml
# docker-compose.override.yml
services:
  g8e-gateway:
    ports: !override
      - "18080:8080"
      - "18443:8443"
```

Host-port overrides do not change the Gateway's internal listeners or service-network URLs. The Compose network aliases `g8e.local` and `g8eg` remain the internal names used by the workloads.

### Resource limits

The root Compose resource settings are:

| Service | CPU limit | Memory limit | CPU reservation | Memory reservation |
| --- | --- | --- | --- | --- |
| `g8e-gateway` | 2 | 4G | 1 | 512M |
| `g8e-data-operator` | 2 | 1G | 0.5 | 256M |
| `g8e-inference-operator` | 4 | 4G | 1 | 1G |
| `ensemble` | 2 | 2G | 0.5 | 512M |

### Dependencies and health checks

The Data Operator and ensemble wait for the Gateway Compose health check. The ensemble waits for the Data Operator to start, not for its health check. The inference Operator waits for the Gateway and has its own certificate-file health check.

| Service | Health check | What it means |
| --- | --- | --- |
| `g8e-gateway` | `wget --no-verbose --tries=1 --spider http://localhost:8080/api/v1/health` | The Gateway health endpoint responds successfully. |
| `g8e-data-operator` | `test -f /root/.g8e/pki/operator.crt` | The Data Operator certificate exists in its runtime volume. |
| `g8e-inference-operator` | `test -f /root/.g8e/pki/operator.crt` | The Inference Operator certificate exists in its runtime volume. |
| `ensemble` | HTTP request to `http://localhost:8000/health` | FastAPI startup and client initialization have completed. |

The root image has no image-level `HEALTHCHECK`; service definitions provide the appropriate signal for Gateway and Operator modes.

### Identity, storage, and trust boundaries

Each workload has its own runtime volume and submits a platform enrollment request when reusable credentials are absent. Owner approval issues the workload identity through the Gateway PKI:

- The Data Operator stores its certificate and key under `g8e-operator-data`.
- The Inference Operator stores its certificate and key under `g8e-inference-data`.
- The ensemble stores its application identity in `g8e-ensemble-data` and reads bootstrap secrets from the Data Operator volume mounted read-only at `/operator-state`.

The Gateway volume is mounted at `/root/.g8e` and contains its SQLite and ledger state under `data/`, generated PKI and trust material under `pki/`, platform secrets under `secrets/`, and vault state under `vault/`. The host CLI `.g8e` tree is not the Gateway volume. Compose does not bind-mount host campaign, mirror, inference, or provider-observation directories into the Gateway.

Removing `g8e-gateway-data` destroys the Gateway authority, owner records, and audit state. A subsequent startup creates a new trust domain and requires owner and workload enrollment again. Do not use `docker compose down -v`, `./g8e docker clean`, or `./g8e docker reset` until required evidence and credentials are backed up.

### Host identity and certificates

The root Compose Gateway mounts the host identity files read-only:

```yaml
- /etc/hosts:/etc/hosts.host:ro
- /etc/hostname:/etc/hostname.host:ro
```

In the default `full` certificate identity mode, the detector reads these mounted files before the container's own files and includes detected host IP addresses and DNS aliases. The Gateway regenerates its managed serving certificate when required SANs are absent. These mounts are Linux-oriented; other Docker hosts need an equivalent identity and certificate strategy.

`--cert-mode localhost` restricts generated serving identities to built-in local names and loopback. `--cert-mode full` enables detected network identities. `--pki-dir` changes managed PKI storage; it does not inject an externally issued serving certificate. See [Network Architecture](../architecture/network.md#network-identity-detection).

### Governance postures

Pass `--posture` to `gw start` to select:

- `doctrine`: L1 is enforced; L2 and L3 are audited. (CLI default)
- `consensus`: L1 and L2 are enforced; L3 is audited.
- `ratify`: L1 and L3 are enforced; L2 is audited.
- `notary`: L1, L2, and L3 are enforced.

The root Compose Gateway does not override `--posture`, so it uses the default, `doctrine`. Selecting another posture does not create its required policy and proof inputs.

### g8ellama topology

The `g8ellama` profile replaces the default gateway with a User Gateway and Inference Node topology for governed remote Ollama inference. Activate it by setting `G8E_OLLAMA_ENDPOINT` and selecting the profile:

```bash
G8E_OLLAMA_ENDPOINT=http://approved-provider:11434 docker compose --profile g8ellama up -d --build
./g8e auth enroll user -e localhost:8090 --port 8453
./g8e auth enroll pending -e localhost:8090 --port 8453
./g8e auth enroll approve <user-gateway-request-id> --yes -e localhost:8090 --port 8453
./g8e auth enroll approve <inference-node-request-id> --yes -e localhost:8090 --port 8453
```

The User Gateway publishes on ports 8090 (HTTP) and 8453 (HTTPS), distinct from the default gateway. The User Gateway owns the state Merkle root and governance posture. The Inference Node runs as an outbound operator of the User Gateway and calls the approved remote Ollama provider. No Ollama daemon runs in this stack.

Environment variables for the g8ellama topology: `G8E_OLLAMA_ENDPOINT` (required) and `G8E_USER_HOSTNAME` (public User Gateway hostname for CORS and passkey RP ID/origin, default `localhost`). The Inference Node uses the `g8e operator start` default model roles and keep-alive.

The g8ellama profile is separate from the default and cross-enrollment topologies; do not combine profiles.

### Cross-enrollment topology

The `cross-enrollment` profile adds a secondary gateway that runs in operator mode and enrolls as an outbound operator of the primary gateway:

```bash
docker compose up -d --build
docker compose --profile cross-enrollment up -d
./g8e auth enroll user -e localhost
./g8e auth enroll pending
./g8e auth enroll approve <data-operator-request-id> --yes
./g8e auth enroll approve <secondary-gateway-request-id> --yes
```

The primary gateway is reachable at the default ports (8080, 8443). The secondary gateway `g8e-gateway-secondary` runs in operator mode and has no published HTTP/HTTPS ports; it communicates with the primary via mTLS at the network alias `g8e.local:8443`.

### Lifecycle management

| Command | Behavior |
| --- | --- |
| `./g8e docker start` | Builds and starts the unified stack (gateway, operator, inference-operator, ensemble); runs interactive enrollment. |
| `./g8e docker start --skip-enroll` | Starts the unified stack without enrollment prompts; workloads wait for approval. |
| `./g8e docker init` | Builds and bootstraps the unified stack with evaluation inference; auto-approves all workloads and waits for ensemble health. Requires `G8E_OLLAMA_ENDPOINT` in `.env`. |
| `./g8e docker stop` | Removes Compose containers while preserving volumes and networks. |
| `./g8e docker status [--profile <name>]` | Displays Compose service status. |
| `./g8e docker build [--no-cache]` | Builds images, exports the runtime binary to `./g8e`, and reports success only after export. |
| `./g8e docker binaries export [--image <ref>] [--output <dir>]` | Exports and validates a full g8e-binary set from an image that contains `/opt/g8e/bin`; standard Gateway images ship only the runtime binary. |
| `./g8e docker logs [service] [-f] [--profile <name>]` | Displays or follows Compose logs. |
| `./g8e docker reset [--profile <name>] [--yes] [--skip-backup]` | Removes containers, volumes, and networks, then starts the specified scope. Destructive; confirms and offers an evidence backup first (see `docker clean`). |
| `./g8e docker rebuild [--profile <name>] [--no-cache]` | Stops services, rebuilds images with provenance, exports the runtime binary to `./g8e`, and starts the specified scope. Cache reuse is the default. |
| `./g8e docker clean [--yes] [--skip-backup]` | Removes containers, volumes, networks, and orphans across all profiles. Destructive and not recoverable: Docker volumes cannot be renamed aside. It prompts for confirmation (`--yes` skips it; end of input aborts), then offers to back up host evaluation evidence to `eval/backups/` (runs automatically with `--yes` unless `--skip-backup`). A failed backup aborts. The host `.g8e/` directory is not touched. |

If a workload remains unhealthy, inspect the relevant service logs and `./g8e auth enroll pending`. If a volume was removed, treat all prior identities as invalid and repeat owner and workload enrollment. If a previous Docker invocation created the host `.g8e` tree as root, repair ownership before CLI enrollment, for example `sudo chown -R $(id -u):$(id -g) .g8e`.

### Headless owner enrollment

For an mTLS-only CLI identity without browser passkey registration or OS trust installation:

```bash
docker compose up -d --build
./g8e auth enroll user --headless -e localhost
```

On a new Gateway this creates the first CLI owner without a passkey. On an already bootstrapped Gateway, headless recovery requires approval by an enrolled CLI. A headless identity cannot authenticate to the browser console, so retain a passkey-enabled owner when console access or WebAuthn approval is required.

### FIPS runtime mode

The root Dockerfile builds Linux binaries with Go Cryptographic Module v1.0.0 support. Strict enforcement is disabled by default so features that use non-approved primitives remain available. Verify the binary directly:

```bash
docker run --rm g8e-gateway:latest version --fips
docker run --rm -e GODEBUG=fips140=only g8e-gateway:latest version --fips
```

`GODEBUG=fips140=only` enables strict enforcement for that process and can cause features requiring non-approved primitives, such as Ed25519-based consensus, to fail closed. The image's FIPS claim is limited to the tested Linux `amd64` operating environment; do not infer runtime enforcement from the build configuration alone.

### Production notes

The root Compose file is a single-host reference deployment. A production deployment supplies its own orchestration, secret backup and recovery, resource sizing, monitoring, ingress, and browser HTTPS termination.

- Configure the Gateway public URL, CORS origin, passkey RP ID, and passkey RP origin to exactly match the browser-visible deployment.
- Persist and protect the complete Gateway runtime volume, including the vault key. `G8E_VAULT_KEY` or `--vault-key` changes the key path; it does not provide the key value.
- Treat generated authority, serving, workload, and vault keys as secrets.
- Keep `g8e.local` resolvable inside the Compose network because the Data Operator and ensemble use that name.
- Treat the public mirror ports as separate read/ingest surfaces and keep them bound to loopback unless the deployment explicitly supplies the required proxy and access controls.
- Verify FIPS mode and enforcement on the deployed process with `g8e version --fips`.

## Anti-patterns

- Starting the stack without setting `G8E_OLLAMA_ENDPOINT` when using `docker init` — it requires a remote Ollama URL.
- Combining multiple profiles (e.g., `--profile cross-enrollment --profile g8ellama`) — profiles are mutually exclusive.
- Running `docker compose down -v` to remove volumes without first backing up gateway data, credentials, or evidence.
- Hand-editing container certificates or PKI files instead of using `--cert-mode` and regenerating.
- Assuming port remaps change internal container listeners — they only map host ports; internal URLs must use container ports.
- Deploying the reference Compose stack to production without adding orchestration, monitoring, secret management, and ingress.

## Links out

- [Unified Docker Stack Guide](./unified_stack.md): campaign workflow, witness operators, evaluation topology.
- [Getting Started](./getting_started.md): quick start for new users.
- [Network Architecture](../architecture/network.md): identity detection, certificate modes, PKI internals.
