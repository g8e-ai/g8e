# Docker Gateway Guide

Last Updated: 2026-09-23
Version: v2.1.12

This task guide covers the root Docker image, a standalone Gateway container, and the root Docker Compose deployment. The Gateway is the Policy Decision Point: it owns the container-local PKI and coordination state, exposes the authenticated APIs, and runs the embedded Operator for its own runtime. The outbound `g8e-operator` container is a separate Policy Execution Point. For the complete campaign and evaluation workflow, see the [Unified Docker Stack Guide](./unified_stack.md).

## Prerequisites

- Docker Engine with the Docker Compose v2 plugin.
- A checkout of this repository.
- The repository's `./g8e` binary when using CLI enrollment or lifecycle commands. Build it with `make build` if it is absent.
- Ports available for the surfaces you enable. The default stack publishes 8080, 8443, 8081, 8082, 5173, 8000, and 3000.
- A browser with WebAuthn support for passkey-based owner enrollment. Headless enrollment is available for CLI-only operation.

Run repository commands from the repository root. The `./g8e docker` commands require `docker-compose.yml` in the current directory.

## Build the Gateway image

Build the root image from the repository root:

```bash
docker build -t g8e-gateway:latest .
```

The image entrypoint is `/g8e`. The same image runs Gateway and Operator commands; the command supplied by Compose selects the mode. The Dockerfile builds the Linux `amd64` runtime binary and also copies deployment binaries for other supported targets into `/opt/g8e/bin/`. The current root `.dockerignore` excludes `dashboard/`, while the Dockerfile's `make build-all` step requires the generated Evaluation Explorer asset under that directory. Therefore, the root image build is not self-contained in the current tree; adjust the Docker build context or Dockerfile before relying on `docker build` or Compose image builds. Run the prepared image on Linux `amd64` or with a runtime that emulates that platform.

The image exposes container ports 8080 and 8443. Compose declares service-specific health checks because the image also runs the outbound-only Operator, which has no listening gateway port.

## Run a standalone Gateway

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

The host CLI identity is stored in the host checkout's `.g8e` tree. It is separate from the Gateway's `/root/.g8e` state in `g8e-data`. A standalone command does not start the Data Operator, Inference Operator, ensemble, or dashboard.

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

## Root Compose deployment

The root `docker-compose.yml` defines a single-host reference deployment on the `g8e-net` bridge network. Only `g8e-gateway` starts without a profile. The remaining services require profiles and owner-approved platform enrollment; a profile does not bypass enrollment.

| Service | Profile | Published ports | Role | Persistent volume |
| --- | --- | --- | --- | --- |
| `g8e-gateway` | default | 8080, 8443, 8081, 8082, 5173 | Gateway PDP, PKI authority, governance API, console, public spectator, and evaluation explorer | `g8e-gateway-data` |
| `g8e-operator` | `bootstrapped` | none | Data Operator with the outbound-only mTLS execution boundary | `g8e-operator-data` |
| `g8e-inference-operator` | `evaluation` | none | Inference Operator for the approved remote Ollama provider | `g8e-inference-data` |
| `ensemble` | `bootstrapped` | 8000 | g8ee FastAPI ensemble | `g8e-ensemble-data` |
| `dashboard` | `bootstrapped` | 3000 | g8ed Express dashboard host | `g8e-dashboard-data` |

The `cross-enrollment` profile adds `g8e-gateway-secondary`, which runs the same image in outbound Operator mode. The `g8ellama` profile is a separate User Gateway and Inference Node topology. It is not part of the default or evaluation stack. See the [Unified Docker Stack Guide](./unified_stack.md) before enabling evaluation, cross-enrollment, or g8ellama profiles.

The Gateway, Data Operator, and optional inference services use the root `Dockerfile`. The ensemble and dashboard build from `ensemble/Dockerfile` and `dashboard/Dockerfile`.

### Start the Gateway only

```bash
docker compose up -d --build
curl -fsS http://127.0.0.1:8080/api/v1/health
```

The HTTP endpoint is for health, bootstrap discovery, CA bundle retrieval, and platform enrollment submission. Authenticated APIs, MCP, A2A, governance envelopes, pub/sub, and the console use the HTTPS/mTLS listener.

### Manually bootstrap the platform workloads

Enroll the first owner before starting profile-gated workloads:

```bash
./g8e auth enroll user -e localhost

docker compose --profile bootstrapped up -d
./g8e auth enroll pending
```

Approve the exact pending request IDs after identifying each request's component:

```bash
./g8e auth enroll approve <data-operator-request-id> --yes
./g8e auth enroll approve <dashboard-request-id> --yes
./g8e auth enroll approve <ensemble-request-id> --yes
./g8e auth enroll deny <request-id> --yes
```

Use `deny` instead of `approve` only for a request that should not be admitted. The commands use the enrolled host CLI identity over mTLS and accept request IDs, not requester tokens, token hashes, CSRs, or certificates.

For the evaluation topology, set `G8E_OLLAMA_ENDPOINT` in `.env`, then start both profiles:

```bash
docker compose --profile bootstrapped --profile evaluation up -d
./g8e auth enroll pending
./g8e auth enroll approve <inference-operator-request-id> --yes
```

The evaluation profile's Inference Operator calls the approved remote Ollama provider. No Ollama daemon runs in this Compose stack. The full campaign workflow, provider Observer Operator, Provenance Operator, and campaign verification are documented in [Unified Docker Stack Guide](./unified_stack.md).

### Use the CLI lifecycle commands

The CLI wraps the root Compose file and prepares the host `.g8e` runtime tree before enrollment:

```bash
./g8e docker start                 # Gateway only
./g8e docker start --full          # Bootstrapped profile with interactive enrollment
./g8e docker start --full --skip-enroll
./g8e docker init                  # Build and bootstrap the evaluation stack
./g8e docker status
./g8e docker logs g8e-gateway -f
```

`docker start --full` starts only the `bootstrapped` profile: Data Operator, ensemble, and dashboard. Its walkthrough enrolls or reuses the CLI owner, then prompts for approval in ensemble, dashboard, and Data Operator order. A missing request or a skipped prompt is non-fatal; finish it with `auth enroll pending` and `auth enroll approve`.

`docker init` is the automated evaluation bootstrap. It requires a repository-root `.env` with `G8E_OLLAMA_ENDPOINT` set, builds unless `--skip-build` is supplied, starts the Gateway, enrolls the owner, starts `bootstrapped` and `evaluation`, auto-approves the platform requests, and waits for ensemble health. Useful flags are `--clean` (destructive volume wipe), `--skip-build`, `--skip-enroll`, `--skip-approvals`, and `--headless`.

The CLI walkthrough assumes host gateway ports 8080 and 8443. With remapped ports, use the manual enrollment commands and specify both endpoints:

```bash
./g8e auth enroll user -e localhost:18080 --port 18443
./g8e auth enroll pending -e localhost:18080 --port 18443
./g8e auth enroll approve <request-id> --yes -e localhost:18080 --port 18443
```

## Compose configuration

Copy `.env.example` to `.env`, or export overrides before invoking Compose:

| Variable | Default | Effect |
| --- | --- | --- |
| `G8E_PREFIX` | `g8e` | Prefixes container names. It does not rename Compose services, the network, or volumes. |
| `G8E_HTTP_PORT` | `8080` | Host port mapped to Gateway container port 8080. |
| `G8E_HTTPS_PORT` | `8443` | Host port mapped to Gateway container port 8443. |
| `G8E_PUBLIC_MIRROR_PRIVATE_PORT` | `8081` | Loopback-only host port for the Gateway private mirror ingest surface. |
| `G8E_PUBLIC_MIRROR_PUBLIC_PORT` | `8082` | Loopback-only host port for the public mirror read/SSE surface. |
| `G8E_EVAL_EXPLORER_PORT` | `5173` | Loopback-only host port for the embedded evaluation explorer. |
| `G8E_ENSEMBLE_PORT` | `8000` | Host port mapped to the ensemble container port 8000. |
| `G8E_DASHBOARD_PORT` | `3000` | Host port mapped to the dashboard container port 3000. |
| `G8E_HOSTNAME` | `localhost` | Browser-visible hostname used for the Gateway public URL, CORS, and passkey RP settings. |
| `G8E_OLLAMA_ENDPOINT` | unset | Approved remote Ollama URL required by the `evaluation` profile and `docker init`. |

For example:

```bash
G8E_HTTP_PORT=18080 G8E_HTTPS_PORT=18443 G8E_DASHBOARD_PORT=13000 docker compose up -d --build
```

Host-port overrides do not change the Gateway's internal listeners or service-network URLs. The Compose network aliases `g8e.local` and `g8eg` remain the internal names used by the workloads.

The root Compose resource settings are:

| Service | CPU limit | Memory limit | CPU reservation | Memory reservation |
| --- | --- | --- | --- | --- |
| `g8e-gateway` | 2 | 2G | 1 | 512M |
| `g8e-operator` | 2 | 1G | 0.5 | 256M |
| `g8e-inference-operator` | 4 | 4G | 1 | 1G |
| `ensemble` | 2 | 2G | 0.5 | 512M |
| `dashboard` | 1 | 512M | 0.25 | 128M |

## Dependencies and health checks

The Data Operator, ensemble, and dashboard wait for the Gateway Compose health check. The ensemble waits for the Data Operator to start, not for its health check. The inference Operator waits for the Gateway and has its own certificate-file health check.

| Service | Health check | What it means |
| --- | --- | --- |
| `g8e-gateway` | `wget --no-verbose --tries=1 --spider http://localhost:8080/api/v1/health` | The Gateway health endpoint responds successfully. |
| `g8e-operator` | `test -f /root/.g8e/pki/operator.crt` | The Data Operator certificate exists in its runtime volume. |
| `g8e-inference-operator` | `test -f /root/.g8e/pki/operator.crt` | The Inference Operator certificate exists in its runtime volume. |
| `ensemble` | HTTP request to `http://localhost:8000/health` | FastAPI startup and client initialization have completed. |
| `dashboard` | `wget --no-verbose --tries=1 --spider http://localhost:3000/` | Express is listening after startup enrollment. |

The root image has no image-level `HEALTHCHECK`; service definitions provide the appropriate signal for Gateway and Operator modes.

## Identity, storage, and trust boundaries

Each workload has its own runtime volume and submits a platform enrollment request when reusable credentials are absent. Owner approval issues the workload identity through the Gateway PKI:

- The Data Operator stores its certificate and key under `g8e-operator-data`.
- The Inference Operator stores its certificate and key under `g8e-inference-data`.
- The ensemble stores its application identity in `g8e-ensemble-data` and reads bootstrap secrets from the Data Operator volume mounted read-only at `/operator-state`.
- The dashboard stores its runtime and application identity under `/data` in `g8e-dashboard-data`.

The Gateway volume is mounted at `/root/.g8e` and contains its SQLite and ledger state under `data/`, generated PKI and trust material under `pki/`, platform secrets under `secrets/`, and vault state under `vault/`. The host CLI `.g8e` tree is not the Gateway volume. Compose does not bind-mount host campaign, mirror, inference, or provider-observation directories into the Gateway.

Removing `g8e-gateway-data` destroys the Gateway authority, owner records, and audit state. A subsequent startup creates a new trust domain and requires owner and workload enrollment again. Do not use `docker compose down -v`, `./g8e docker clean`, or `./g8e docker reset` until required evidence and credentials are backed up.

## Host identity and certificates

The root Compose Gateway mounts the host identity files read-only:

```yaml
- /etc/hosts:/etc/hosts.host:ro
- /etc/hostname:/etc/hostname.host:ro
```

In the default `full` certificate identity mode, the detector reads these mounted files before the container's own files and includes detected host IP addresses and DNS aliases. The Gateway regenerates its managed serving certificate when required SANs are absent. These mounts are Linux-oriented; other Docker hosts need an equivalent identity and certificate strategy.

`--cert-mode localhost` restricts generated serving identities to built-in local names and loopback. `--cert-mode full` enables detected network identities. `--pki-dir` changes managed PKI storage; it does not inject an externally issued serving certificate. See [Network Architecture](../architecture/network.md#8-network-identity-detection).

## Governance postures

`gw start --posture` accepts:

- `doctrine`: L1 is enforced; L2 and L3 are audited.
- `consensus`: L1 and L2 are enforced; L3 is audited.
- `ratify`: L1 and L3 are enforced; L2 is audited.
- `notary`: L1, L2, and L3 are enforced.

The root Compose Gateway does not pass `--posture`, so it uses the CLI default, `doctrine`. Selecting another posture does not create its required policy and proof inputs.

## Lifecycle, cleanup, and recovery

| Command | Behavior |
| --- | --- |
| `./g8e docker start` | Starts the default Gateway service. |
| `./g8e docker start --full` | Starts the `bootstrapped` profile and runs interactive owner/platform enrollment. |
| `./g8e docker start --full --skip-enroll` | Starts the `bootstrapped` profile without enrollment prompts; workloads wait for approval. |
| `./g8e docker init` | Builds and bootstraps the `bootstrapped` plus `evaluation` profiles. |
| `./g8e docker stop` | Removes Compose containers while preserving volumes and networks. |
| `./g8e docker status [--profile <name>]` | Displays Compose service status. |
| `./g8e docker build [--no-cache]` | Builds the `bootstrapped` Compose scope by default; `--profile` selects another profile. |
| `./g8e docker logs [service] [-f] [--profile <name>]` | Displays or follows Compose logs. |
| `./g8e docker reset [--full] [--profile <name>]` | Removes containers, volumes, and networks, then starts the Gateway or selected `bootstrapped` scope. Destructive. |
| `./g8e docker rebuild [--full] [--profile <name>]` | Stops the selected teardown scope, rebuilds the selected build scope, and starts the Gateway or selected `bootstrapped` scope. `--no-cache` defaults to true. |
| `./g8e docker clean` | Removes containers, volumes, networks, and orphans across the unified profiles. Confirmation is skipped by default; use `--yes=false` to prompt. Destructive. |

If a workload remains unhealthy, inspect the relevant service logs and `./g8e auth enroll pending`. If a volume was removed, treat all prior identities as invalid and repeat owner and workload enrollment. If a previous Docker invocation created the host `.g8e` tree as root, repair ownership before CLI enrollment, for example `sudo chown -R $(id -u):$(id -g) .g8e`.

## Headless owner enrollment

For an mTLS-only CLI identity without browser passkey registration or OS trust installation:

```bash
docker compose up -d --build
./g8e auth enroll user --headless -e localhost
```

On a new Gateway this creates the first CLI owner without a passkey. On an already bootstrapped Gateway, headless recovery requires approval by an enrolled CLI. A headless identity cannot authenticate to the browser console, so retain a passkey-enabled owner when console access or WebAuthn approval is required.

## Demo Compose environments

Healthcare, Finance, DHS, and FedRAMP deployments under `demos/` are separate demonstrations, not extensions of the root stack. Use the demos CLI or run Compose from the selected demo directory:

```bash
./g8e demos start healthcare
./g8e demos status healthcare
cd demos/healthcare
docker compose up -d --build
```

Each demo builds the shared Go image from the repository-root Dockerfile through `context: ../..`. See [Demos](../../demos/README.md) for each demo's topology, ports, enrollment, and scenarios.

## FIPS runtime mode

The root Dockerfile builds Linux binaries with Go Cryptographic Module v1.0.0 support. Strict enforcement is disabled by default so features that use non-approved primitives remain available. Verify the binary directly:

```bash
docker run --rm g8e-gateway:latest version --fips
docker run --rm -e GODEBUG=fips140=only g8e-gateway:latest version --fips
```

`GODEBUG=fips140=only` enables strict enforcement for that process and can cause features requiring non-approved primitives, such as Ed25519-based consensus, to fail closed. The image's FIPS claim is limited to the tested Linux `amd64` operating environment; do not infer runtime enforcement from the build configuration alone.

## Production notes

The root Compose file is a single-host reference deployment. A production deployment supplies its own orchestration, secret backup and recovery, resource sizing, monitoring, ingress, and browser HTTPS termination.

- Configure the Gateway public URL, CORS origin, passkey RP ID, and passkey RP origin to exactly match the browser-visible deployment.
- Persist and protect the complete Gateway runtime volume, including the vault key. `G8E_VAULT_KEY` or `--vault-key` changes the key path; it does not provide the key value.
- Treat generated authority, serving, workload, and vault keys as secrets.
- Keep `g8e.local` resolvable inside the Compose network because the Data Operator and ensemble use that name.
- Treat the public mirror ports as separate read/ingest surfaces and keep them bound to loopback unless the deployment explicitly supplies the required proxy and access controls.
- Verify FIPS mode and enforcement on the deployed process with `g8e version --fips`.
