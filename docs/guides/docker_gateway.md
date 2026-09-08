# Docker Gateway Guide

Last Updated: 2026-09-08
Version: v2.1.7

This guide covers building the shared Gateway/Operator image, running a gateway-only container, and managing the repository's Docker Compose deployments. For the complete four-service product workflow, see the [Unified Docker Stack Guide](./unified_stack.md).

## Prerequisites

- Docker with BuildKit support
- Docker Compose v2 (`docker compose`)
- A checkout of this repository
- The `./g8e` CLI when performing owner or platform enrollment from the host

Run repository commands from the repository root unless a section explicitly changes directories.

## Build the Image

Build the shared Gateway/Operator image from the repository root:

```bash
docker build -t g8e-gateway:latest .
```

The runtime image contains a linux/amd64 binary. Run it on linux/amd64 or through a container runtime configured to emulate that platform.

## Run a Standalone Gateway

Start only the Gateway Policy Decision Point with persistent runtime state:

```bash
docker run -d \
  --name g8e-gateway \
  -p 8080:8080 \
  -p 8443:8443 \
  -v g8e-data:/root/.g8e \
  g8e-gateway:latest \
  gw start -f --posture doctrine --cert-mode localhost
```

`-f` keeps the gateway in the foreground as the container's main process. `--cert-mode localhost` makes this local-only example deterministic. Omit it to use the default `full` identity detection mode, and add the host identity mounts described in [Host identity and certificates](#host-identity-and-certificates) when the certificate must cover host names or addresses.

The standalone command does not start the operator, ensemble, or dashboard. Enroll the first owner from the host after the health endpoint responds:

```bash
until curl -fsS http://localhost:8080/api/v1/health >/dev/null 2>&1; do sleep 2; done
./g8e auth enroll user -e localhost
```

The enrollment command writes the host CLI identity to the host runtime tree; that identity is separate from the gateway state in the `g8e-data` volume.

## Unified Docker Compose Stack

The root `docker-compose.yml` defines four services on the `g8e-net` bridge network:

| Service | Role | Published ports | Persistent volume |
| --- | --- | --- | --- |
| `g8e-gateway` | Policy Decision Point, PKI authority, governance API, console, and transport services | 8080, 8443 | `g8e-gateway-data` |
| `g8e-operator` | Outbound mTLS Policy Execution Point | None | `g8e-operator-data` |
| `ensemble` | g8ee FastAPI agentic ensemble | 8000 | `g8e-ensemble-data` |
| `dashboard` | g8ed Express static SPA host | 3000 | `g8e-dashboard-data` |

Only `g8e-gateway` is in the default Compose profile. The operator, ensemble, and dashboard use the `bootstrapped` profile. Those workload containers can start before owner enrollment, but their startup enrollment blocks readiness until the first owner approves their platform enrollment requests.

The gateway and operator use the root `Dockerfile`. The ensemble uses `ensemble/Dockerfile`, and the dashboard uses `dashboard/Dockerfile`.

### Start the gateway only

```bash
docker compose up -d --build
```

This starts the unprofiled gateway service. Verify its health before enrolling:

```bash
curl -fsS http://localhost:8080/api/v1/health
```

### Complete the manual bootstrap

```bash
# Enroll the first owner and create the host CLI mTLS identity.
./g8e auth enroll user -e localhost

# Start the platform workloads.
docker compose --profile bootstrapped up -d

# Discover the requests submitted by the workloads.
./g8e auth pending-platform-enrollments

# Approve each exact request ID. Approving the operator first makes its shared transport credentials available before the ensemble finishes startup.
./g8e auth approve-platform-enrollment <operator-request-id> --yes
./g8e auth approve-platform-enrollment <dashboard-request-id> --yes
./g8e auth approve-platform-enrollment <ensemble-request-id> --yes

# Inspect readiness after enrollment completes.
docker compose --profile bootstrapped ps
```

The approval commands use the enrolled host CLI identity over mTLS. They accept request IDs, not requester tokens, token hashes, CSRs, or certificates. The gateway console at `https://localhost:8443/console/` also lists and approves pending requests after browser passkey enrollment.

### Use the CLI walkthrough

The repository CLI wraps the root Compose stack:

```bash
./g8e docker start --full
```

This starts the `bootstrapped` profile, waits for `http://127.0.0.1:8080/api/v1/health`, enrolls or reuses the CLI owner identity, and checks once for each pending request in ensemble, dashboard, then operator order. Each prompt is optional. If a workload has not submitted its request when checked, the walkthrough skips it; use the pending and approval commands above to finish manually.

The walkthrough currently assumes the default host gateway ports, 8080 and 8443. For remapped ports, use the manual workflow and pass the host endpoints to each authentication command:

```bash
./g8e auth enroll user -e localhost:18080 --port 18443
./g8e auth pending-platform-enrollments -e localhost:18080 --port 18443
./g8e auth approve-platform-enrollment <request-id> --yes -e localhost:18080 --port 18443
```

Use `./g8e docker start --full --skip-enroll` to start all four services without the walkthrough. The workload services remain pending until their requests are approved manually.

## Compose Configuration

Copy `.env.example` to `.env` or export overrides before invoking Compose:

| Variable | Default | Effect |
| --- | --- | --- |
| `G8E_PREFIX` | `g8e` | Prefixes container names, such as `g8e-gateway`. It does not rename Compose services, networks, or volumes. |
| `G8E_HTTP_PORT` | `8080` | Host port published to gateway container port 8080. |
| `G8E_HTTPS_PORT` | `8443` | Host port published to gateway container port 8443. |
| `G8E_ENSEMBLE_PORT` | `8000` | Host port published to ensemble container port 8000. |
| `G8E_DASHBOARD_PORT` | `3000` | Host port published to dashboard container port 3000. |
| `G8E_HOSTNAME` | `localhost` | Hostname used by the gateway public URL, passkey RP configuration, dashboard gateway URL, and dashboard CORS origin. |

For example:

```bash
G8E_HTTP_PORT=18080 G8E_HTTPS_PORT=18443 G8E_DASHBOARD_PORT=13000 docker compose up -d --build
```

Host-port overrides do not change container listeners. The gateway still listens on 8080 and 8443 inside the Compose network.

The root Compose file configures these resource constraints:

| Service | CPU limit | Memory limit | CPU reservation | Memory reservation |
| --- | --- | --- | --- | --- |
| `g8e-gateway` | 2 | 1G | 0.5 | 256M |
| `g8e-operator` | 2 | 1G | 0.5 | 256M |
| `ensemble` | 2 | 2G | 0.5 | 512M |
| `dashboard` | 1 | 512M | 0.25 | 128M |

## Dependencies and Health Checks

The operator, ensemble, and dashboard wait for the gateway's Compose health check. The ensemble also waits for the operator container to start, but not for the operator to become healthy.

| Service | Compose health check | Readiness meaning |
| --- | --- | --- |
| `g8e-gateway` | `wget --no-verbose --tries=1 --spider http://localhost:8080/api/v1/health` | The gateway health endpoint returns success. |
| `g8e-operator` | `test -f /root/.g8e/pki/operator.crt` | The operator certificate has been written after enrollment. |
| `ensemble` | HTTP request to `http://localhost:8000/health` | FastAPI startup, including app enrollment and client initialization, has completed. |
| `dashboard` | `wget --no-verbose --tries=1 --spider http://localhost:3000/` | Startup enrollment completed and Express is listening. |

The Dockerfile intentionally has no image-level `HEALTHCHECK` because the same image runs the listening gateway and the outbound-only operator.

## Service Identity and Storage

The workload services submit owner-approved platform enrollment requests when they do not have reusable credentials:

- The operator stores `pki/operator.crt` and `pki/operator.key` in `g8e-operator-data`.
- The ensemble stores its `spiffe://g8e.local/app/g8ee` identity under `pki/issued/apps/` in `g8e-ensemble-data`.
- The dashboard stores its `spiffe://g8e.local/app/g8ed` identity under `/data/pki/issued/apps/` in `g8e-dashboard-data` and does not listen until enrollment succeeds.

The ensemble mounts `g8e-operator-data` read-only at `/operator-state`. Its governance client uses the operator certificate and key from that mount for the privileged governance-envelope route; its other gateway clients use the ensemble app identity.

The gateway runtime volume is mounted at `/root/.g8e` and includes:

- `data/` for the canonical SQLite database and ledger state
- `pki/` for generated authorities, serving certificates, identities, and trust bundles
- `secrets/` for platform secret material
- `vault/` for encrypted vault state and the default vault key at `vault/key`

Removing `g8e-gateway-data` destroys the gateway PKI, owner records, and audit state. The next startup creates a new authority and requires owner and workload enrollment again.

## Host Identity and Certificates

The root Compose file and all four demo gateway services mount Linux host identity files read-only:

```yaml
volumes:
  - /etc/hosts:/etc/hosts.host:ro
  - /etc/hostname:/etc/hostname.host:ro
```

In the default `full` certificate identity mode, the detector reads these mounted files before the container's own files and includes detected host IP addresses and DNS aliases. The gateway regenerates its internally managed serving certificate when required SANs are absent. These mounts are Linux-oriented; deployments on other Docker hosts need an equivalent identity and certificate strategy.

`--cert-mode localhost` restricts generated serving identities to the built-in local service names and loopback address. `--cert-mode full` also enables detected network identities. `--pki-dir` changes the gateway's PKI storage directory; it is not an interface for injecting an externally issued serving certificate.

See [Network Architecture](../architecture/network.md#8-network-identity-detection) for the detector pipeline.

## Ports

The gateway has two listeners:

- **8080 HTTP**: health, bootstrap discovery, CA bundle download, platform enrollment submission, and redirects for other routes
- **8443 HTTPS/mTLS**: authenticated APIs, MCP, A2A, governance envelopes, document and blob services, pub/sub, and the console

Compose host-port mappings leave those container listeners unchanged. In a standalone container, listener flags and published ports must match. For example:

```bash
docker run -d \
  --name g8e-gateway-custom-ports \
  -p 3000:3000 \
  -p 3443:3443 \
  -v g8e-custom-data:/root/.g8e \
  g8e-gateway:latest \
  gw start -f --posture doctrine --cert-mode localhost --http-port 3000 --https-port 3443
```

To retain the default container listeners while changing only host ports, publish `3000:8080` and `3443:8443` and omit the listener flags.

## Governance Postures

`gw start --posture` accepts:

- `doctrine`: L1 enforced; L2 and L3 audited
- `consensus`: L1 and L2 enforced; L3 audited
- `ratify`: L1 and L3 enforced; L2 audited
- `notary`: L1, L2, and L3 strictly enforced

The root Compose gateway does not pass `--posture`, so the CLI default is `doctrine`. Consensus and notary deployments also require their corresponding policy and proof configuration; selecting a posture alone does not create those inputs.

## CLI Lifecycle Commands

All `./g8e docker` commands require the repository-root `docker-compose.yml` in the current directory.

| Command | Current behavior |
| --- | --- |
| `./g8e docker start` | Runs `docker compose up -d` for the default gateway-only profile. |
| `./g8e docker start --full` | Starts the `bootstrapped` profile and runs the interactive enrollment walkthrough. |
| `./g8e docker start --full --skip-enroll` | Starts the full profile without enrollment prompts. |
| `./g8e docker stop` | Runs Compose `down` against the full profile. It removes containers and the Compose network but preserves named volumes. |
| `./g8e docker status [--profile bootstrapped]` | Runs Compose `ps`. |
| `./g8e docker build [--no-cache]` | Builds images with a source build ID; the command targets the full profile by default. |
| `./g8e docker logs [service] [-f] [--profile bootstrapped]` | Prints or follows Compose logs, optionally for one Compose service name. |
| `./g8e docker reset [--full] [--profile bootstrapped]` | Removes containers, volumes, and networks, then starts the selected scope. This destroys persisted state. |
| `./g8e docker rebuild [--full] [--profile bootstrapped]` | Runs Compose down, build, and up. `--no-cache` defaults to true; use `--no-cache=false` to reuse the cache. |
| `./g8e docker clean` | Removes containers, volumes, networks, and orphans across the full profile. Confirmation is skipped by default; use `--yes=false` to request a prompt. |

`reset` and `clean` are destructive because they pass `down -v`. Back up any required runtime evidence before using them.

## Headless Gateway-Only Enrollment

For an mTLS-only CLI identity without browser passkey registration or OS trust installation:

```bash
docker compose up -d --build
./g8e auth enroll user --headless -e localhost
```

On an unbootstrapped gateway, this creates the first CLI owner identity without a passkey. On a gateway that already has an owner, headless recovery requires approval from an already enrolled CLI. A headless identity cannot authenticate to the browser console, so retain at least one passkey-enabled owner identity when console access or L3 WebAuthn approval is required.

## Demo Compose Environments

The repository contains separate Healthcare, Finance, DHS, and FedRAMP deployments under `demos/`. These are isolated compliance demonstrations, not extensions of the root unified stack. Each uses five named network tiers and excludes the root stack's ensemble and dashboard services.

Use the demos CLI for the documented lifecycle and bootstrap instructions:

```bash
./g8e demos start healthcare
./g8e demos status healthcare
./g8e demos scenarios list healthcare
```

Direct Compose usage runs from the selected demo directory:

```bash
cd demos/healthcare
docker compose up -d --build
```

Each demo builds the shared Go image from the repository-root Dockerfile through `context: ../..`. See [Demos](../../demos/README.md) for service topologies, ports, enrollment commands, and scenarios.

### Consensus demo bootstrap

The DHS and FedRAMP gateway services default `G8E_GATEWAY_POSTURE` to `consensus` and mount `config/consensus-bootstrap.json` into the gateway and scenario agent containers. Their checked-in files use `member_seeds`, which derives a distinct Ed25519 key pair for each configured member. The gateway also supports a shared `seed_hex` fallback and generates a shared key when neither seed form is present; `member_seeds` takes precedence when supplied.

These deterministic seeds are demo fixtures. Production consensus keys require deployment-specific secret management rather than checked-in bootstrap seeds.

## Image Contents and FIPS Mode

The multi-stage Dockerfile uses `golang:1.26.6` for the builder and a digest-pinned Debian 12 Bookworm runtime image. It installs the shared binary at `/g8e`, copies cross-platform deployment binaries to `/opt/g8e/bin/`, protocol constants to `/protocol/constants`, and compliance reference data to `/docs/reference`.

Linux binaries are built with Go Cryptographic Module v1.0.0 support. The linux/amd64 image enters FIPS approved mode, but strict enforcement is off by default so non-approved primitives used by features such as Ed25519 consensus remain available. Set `GODEBUG=fips140=only` only for a deployment whose selected features use approved primitives, and verify the running image directly:

```bash
docker run --rm g8e-gateway:latest version --fips
docker run --rm -e GODEBUG=fips140=only g8e-gateway:latest version --fips
```

The second command enables strict enforcement for that process. It can make features that require non-approved primitives fail closed.

## Operations and Troubleshooting

Inspect status and logs with Compose service names:

```bash
docker compose --profile bootstrapped ps
docker compose --profile bootstrapped logs -f g8e-gateway
./g8e docker logs g8e-operator -f --profile bootstrapped
```

Inspect container health with the configured container-name prefix:

```bash
docker inspect --format='{{.State.Health.Status}}' g8e-gateway
docker inspect --format='{{.State.Health.Status}}' g8e-operator
```

Query gateway process state inside the container:

```bash
docker exec g8e-gateway /g8e gw status
```

If workload enrollment is pending, list requests and inspect the corresponding service logs. If volumes were removed, discard assumptions about prior trust: the gateway has a new authority, and the owner and workloads must enroll again.

## Production Deployment Notes

The root Compose file is a single-host reference deployment. A production deployment supplies deployment-specific orchestration, secret backup and recovery, resource sizing, monitoring, and ingress controls.

- Terminate public browser traffic with trusted HTTPS and configure the gateway public URL, CORS origins, passkey RP ID, and passkey origins to exactly match the browser-visible deployment. The root Compose defaults use plain HTTP for the dashboard and are intended for localhost development.
- Persist and protect the complete gateway runtime volume. The vault key defaults to `/root/.g8e/vault/key`; `G8E_VAULT_KEY` or `--vault-key` changes that path, not the key value.
- Treat the generated root authority and serving keys as production secrets. `--cert-mode full` controls SAN discovery but does not replace the internal PKI with a public certificate authority.
- Keep `g8e.local` resolvable inside the service network because the operator and ensemble connect to that name. The Compose network alias provides resolution, and the gateway includes `g8e.local` in its serving certificate.
- Verify FIPS mode and enforcement on the deployed process with `g8e version --fips`; do not infer enforcement from build configuration alone.
